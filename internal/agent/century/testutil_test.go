package century

import (
	"encoding/binary"
	"io"
	"sync"
	"testing"
	"time"
)

// recordingTransport 는 agent_test 전용 mock io.ReadWriteCloser 이다.
//
// 핵심 invariant (AC-B9): WriteCount 는 0 이어야 한다 — agent 는 어떠한 경로로도
// transport.Write() 를 호출하지 않아야 한다.
//
// Read 는 reads 슬라이스의 바이트를 순차적으로 반환한다. 모두 소진되면 blockOnEmpty=true
// 이면 ctx done 까지 block, 아니면 io.EOF 를 반환한다.
type recordingTransport struct {
	mu sync.Mutex

	reads         []byte
	readPos       int
	blockOnEmpty  bool
	closed        bool
	closedCh      chan struct{}
	releaseSignal chan struct{} // optional: external delivery of more bytes

	writeCalls [][]byte
	writeCount int

	// hooks for tests to deliver bytes at controlled times.
	pendingReads chan []byte
}

func newRecordingTransport(initial []byte) *recordingTransport {
	rt := &recordingTransport{
		reads:        append([]byte(nil), initial...),
		closedCh:     make(chan struct{}),
		pendingReads: make(chan []byte, 32),
		blockOnEmpty: true,
	}
	return rt
}

func (t *recordingTransport) Read(p []byte) (int, error) {
	for {
		t.mu.Lock()
		if t.closed {
			t.mu.Unlock()
			return 0, io.EOF
		}
		if t.readPos < len(t.reads) {
			n := copy(p, t.reads[t.readPos:])
			t.readPos += n
			t.mu.Unlock()
			return n, nil
		}
		t.mu.Unlock()

		if !t.blockOnEmpty {
			return 0, io.EOF
		}
		// Wait for more bytes or close.
		select {
		case more := <-t.pendingReads:
			t.mu.Lock()
			t.reads = append(t.reads, more...)
			t.mu.Unlock()
		case <-t.closedCh:
			return 0, io.EOF
		}
	}
}

func (t *recordingTransport) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	cp := make([]byte, len(p))
	copy(cp, p)
	t.writeCalls = append(t.writeCalls, cp)
	t.writeCount += len(p)
	// Even if writes happen unexpectedly, we record but do not error so the test can
	// assert post-hoc rather than crashing the capture loop.
	return len(p), nil
}

func (t *recordingTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	close(t.closedCh)
	return nil
}

// deliver pushes additional bytes to the read stream and unblocks any pending Read.
func (t *recordingTransport) deliver(data []byte) {
	cp := append([]byte(nil), data...)
	select {
	case t.pendingReads <- cp:
	default:
		// channel full — append directly under lock to ensure no data loss.
		t.mu.Lock()
		t.reads = append(t.reads, cp...)
		t.mu.Unlock()
	}
}

// WriteCount returns the total bytes written via Write (AC-B9: must be 0).
func (t *recordingTransport) WriteCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.writeCount
}

// WriteCalls returns the recorded Write invocations (defensive copy).
func (t *recordingTransport) WriteCalls() [][]byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([][]byte, len(t.writeCalls))
	for i, c := range t.writeCalls {
		out[i] = append([]byte(nil), c...)
	}
	return out
}

var _ io.ReadWriteCloser = (*recordingTransport)(nil)

// buildFrame constructs a fully valid Century frame on the wire with computed CRC-16/ARC.
//
// fc / src / dst are the wire header fields; payload is the data portion including the
// sub_dev_id / reserved2 / register prefix for non-ACK frames. The returned bytes are
// header(8) + payload(N) + CRC(2).
func buildFrame(src, dst uint16, fc byte, payload []byte) []byte {
	pl := len(payload)
	if pl > MaxPayloadLength {
		panic("buildFrame: payload too long")
	}
	raw := make([]byte, HeaderLength+pl+CRCLength)
	binary.LittleEndian.PutUint16(raw[0:2], src)
	binary.LittleEndian.PutUint16(raw[2:4], dst)
	binary.LittleEndian.PutUint16(raw[4:6], uint16(pl))
	raw[6] = 0x00
	raw[7] = fc
	copy(raw[8:8+pl], payload)
	crc := CRC16ARC(raw[:HeaderLength+pl])
	binary.LittleEndian.PutUint16(raw[HeaderLength+pl:], crc)
	return raw
}

// mustBuildReg02ResponseFrame builds a synthetic slave-to-master reg 0x02 response
// frame for the given sub_dev_id. data is 17B; we use the CAP-3 baseline pattern.
func mustBuildReg02ResponseFrame(t *testing.T, subDevID byte) []byte {
	t.Helper()
	// Prefix: sub_dev_id, 0x00, register=0x02
	// Data (17B): pattern from CAP-3 (cooling, fan 17, setpoint 25.0℃).
	payload := []byte{
		subDevID, 0x00, 0x02,
		// data starts here:
		0x00, 0x01, 0x11, 0x00, 0x00, 0x00, 0x00, 0xFA, 0x00,
		0x00, 0x00, 0xFA, 0x00, 0x1B, 0x39, 0x39, 0x00,
	}
	return buildFrame(AddrSlave, AddrMaster, FCResponse, payload)
}

// mustBuildReg04ResponseFrame builds a synthetic slave-to-master reg 0x04 read response (14B data).
func mustBuildReg04ResponseFrame(t *testing.T, subDevID byte) []byte {
	t.Helper()
	payload := []byte{
		subDevID, 0x00, 0x04,
		// 14B data:
		0x39, 0xF6, 0x09, 0x00, 0x00, 0x00, 0x00, 0x2C,
		0xE4, 0x03, 0xFC, 0x00, 0xE0, 0x04,
	}
	return buildFrame(AddrSlave, AddrMaster, FCResponse, payload)
}

// mustBuildReg04WriteFrame builds a synthetic master-to-slave reg 0x04 write request (16B data).
// payloadByte0/15 control the variable parts so callers can make duplicates or unique writes.
func mustBuildReg04WriteFrame(t *testing.T, subDevID, payloadByte0, payloadByte15 byte) []byte {
	t.Helper()
	payload := []byte{
		subDevID, 0x00, 0x04,
		// 16B data:
		payloadByte0, 0x04, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xC0, payloadByte15,
	}
	return buildFrame(AddrMaster, AddrSlave, FCWrite, payload)
}

// mustBuildAckFrame builds a slave-to-master ACK frame (payload_length=1, [0x00]).
func mustBuildAckFrame(t *testing.T) []byte {
	t.Helper()
	return buildFrame(AddrSlave, AddrMaster, FCResponse, []byte{0x00})
}

// mustBuildReadRequestFrame builds a synthetic master-to-slave READ request frame
// (FCRead=0x0B). Payload contains only the 3-byte prefix (sub_dev_id, reserved, register)
// with no data. spec 부록 A 의 "Read Request reg 0x02/0x03/0x04" 와 동일한 형식.
func mustBuildReadRequestFrame(t *testing.T, subDevID, register byte) []byte {
	t.Helper()
	payload := []byte{subDevID, 0x00, register}
	return buildFrame(AddrMaster, AddrSlave, FCRead, payload)
}

// waitUntil polls cond every 5ms until true or deadline. Fails the test on deadline.
func waitUntil(t *testing.T, deadline time.Duration, cond func() bool, msg string) {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !cond() {
		t.Fatalf("waitUntil timed out: %s", msg)
	}
}
