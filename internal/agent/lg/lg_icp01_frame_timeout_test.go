package lg

import (
	"bytes"
	"io"
	"testing"
)

// TimeoutSimulatingReader 는 mid-frame 에서 timeout 을 주입한다.
// 이를 통해 bufio.Peek() 가 (partial_bytes, timeout_error) 를 반환하는 상황을 시뮬레이션한다.
type TimeoutSimulatingReader struct {
	data          []byte
	offset        int
	timeoutAfter  int // 몇 번째 Read() 부터 timeout 시작
	timeoutFor    int // 몇 번 연속 timeout
	readCount     int
	partialByte   byte // timeout 시 반환할 부분 바이트
	returnPartial bool
}

func NewTimeoutSimulatingReader(data []byte, timeoutAfter int, partialByte byte) *TimeoutSimulatingReader {
	return &TimeoutSimulatingReader{
		data:          data,
		timeoutAfter:  timeoutAfter,
		timeoutFor:    2, // 2회 timeout 후 정상화
		partialByte:   partialByte,
		returnPartial: true,
	}
}

func (tr *TimeoutSimulatingReader) Read(p []byte) (int, error) {
	if tr.readCount >= tr.timeoutAfter && tr.readCount < tr.timeoutAfter+tr.timeoutFor {
		// timeout 구간: partial byte 반환 + timeout error
		tr.readCount++
		if tr.returnPartial && len(p) > 0 {
			p[0] = tr.partialByte
			return 1, &timeoutErr{} // 1 byte + timeout
		}
		return 0, &timeoutErr{}
	}

	tr.readCount++
	if tr.offset >= len(tr.data) {
		return 0, io.EOF
	}

	n := len(p)
	if n > len(tr.data)-tr.offset {
		n = len(tr.data) - tr.offset
	}
	copy(p, tr.data[tr.offset:tr.offset+n])
	tr.offset += n
	return n, nil
}

type timeoutErr struct{}

func (e *timeoutErr) Error() string   { return "i/o timeout" }
func (e *timeoutErr) Timeout() bool   { return true }
func (e *timeoutErr) Temporary() bool { return true }

// TestIDUShortMisdetectionWithTimeout 는 v0.18.17 fix 검증:
// Peek(3) 도중 timeout → partial byte 반환 → mid-frame 재시도 → 정상 파싱
//
// 시나리오:
// 1. IDU SHORT(20B) 읽기 성공: b[1..20]
// 2. Peek(3) 호출 → timeout: (0x00, timeout_error) 반환
// 3. v0.18.17 fix: timeout 감지 → 재시도
// 4. 재시도 성공 → 3바이트 정상 획득 → SHORT 정확히 감지
// 5. 다음 ODU 프레임 파싱 정상
//
// v0.18.17 fix 전:
// 2번에서 partial=0x00 받음 → default case → for i=1; i<1 (실행 안함)
// → isShort=false → io.ReadFull(raw[20:]) → 다음 프레임 20B 삼킴 → desync
func TestIDUShortMisdetectionWithTimeout(t *testing.T) {
	// IDU SHORT: 20바이트, STX=0x84
	iduShort := make([]byte, 20)
	iduShort[0] = 0x84
	iduShort[1] = 0x03
	for i := 0; i < 20; i++ {
		if iduShort[i] == 0 {
			iduShort[i] = byte(i % 256)
		}
	}

	// 패딩 0x00 (일부 디바이스에서 관찰)
	padding := []byte{0x00}

	// 다음 ODU: 20바이트
	oduNext := make([]byte, 20)
	oduNext[0] = 0x58

	stream := append(iduShort, padding...)
	stream = append(stream, oduNext...)

	// TimeoutSimulatingReader 생성:
	// - 20바이트 읽기 후 (readCount=20) 부터 timeout 시작
	// - timeout 중 0x00 반환
	timeoutReader := NewTimeoutSimulatingReader(stream, 20, 0x00)
	parser := NewIcp01FrameParser(timeoutReader)

	// 첫 번째 ReadFrame: IDU SHORT
	frameType1, _, iduFrame1, err1 := parser.ReadFrame()
	if err1 != nil {
		t.Fatalf("Frame 1 ReadFrame failed: %v", err1)
	}
	if frameType1 != 'B' {
		t.Fatalf("Expected IDU, got %c", frameType1)
	}

	// v0.18.17 fix 검증:
	// timeout 을 재시도로 흡수했으므로 IsShort=true 여야 함
	if !iduFrame1.IsShort {
		t.Errorf("FAIL: IDU frame IsShort=false (expected true after v0.18.17 fix)")
		t.Logf("  Raw[0]: 0x%02x (IDU STX)", iduFrame1.Raw[0])
		t.Logf("  Raw[20]: 0x%02x (should be 0)", iduFrame1.Raw[20])
		if iduFrame1.Raw[20] == 0x00 && iduFrame1.Raw[21] == 0x58 {
			t.Logf("  DETECTED: Parser consumed next frame (padding + ODU)")
		}
		return
	}

	// IDU 데이터 검증
	if !bytes.Equal(iduFrame1.Raw[:20], iduShort) {
		t.Errorf("IDU frame data mismatch")
	}

	// 두 번째 ReadFrame: ODU
	frameType2, oduFrame2, _, err2 := parser.ReadFrame()
	if err2 != nil {
		t.Fatalf("Frame 2 ReadFrame failed: %v", err2)
	}
	if frameType2 != 'A' {
		t.Fatalf("Expected ODU, got %c", frameType2)
	}
	if oduFrame2.Raw[0] != 0x58 {
		t.Fatalf("ODU STX mismatch: expected 0x58, got 0x%02x", oduFrame2.Raw[0])
	}

	// ODU 데이터 검증
	if !bytes.Equal(oduFrame2.Raw[:], oduNext) {
		t.Errorf("ODU frame data mismatch")
	}

	t.Logf("PASS: v0.18.17 fix verified — timeout mid-frame is recovered, framing correct")
}

// TestIDULongDetectionWithTimeout 는 LONG 프레임도 timeout 에서 정확하게 파싱됨을 검증
func TestIDULongDetectionWithTimeout(t *testing.T) {
	// IDU LONG: 40바이트
	iduLong := make([]byte, 40)
	iduLong[0] = 0x82
	iduLong[1] = 0x04
	iduLong[20] = 0x14 // b[20] > 0x05 → forces LONG detection

	// 다음 ODU
	oduNext := make([]byte, 20)
	oduNext[0] = 0x58

	stream := append(iduLong, oduNext...)

	// timeout 시 0x14 (IDU_INDEX 범위 아님) 반환
	timeoutReader := NewTimeoutSimulatingReader(stream, 20, 0x14)
	parser := NewIcp01FrameParser(timeoutReader)

	// 첫 번째 ReadFrame: IDU LONG
	frameType1, _, iduFrame1, err1 := parser.ReadFrame()
	if err1 != nil {
		t.Fatalf("Frame 1 ReadFrame failed: %v", err1)
	}
	if frameType1 != 'B' {
		t.Fatalf("Expected IDU, got %c", frameType1)
	}

	// LONG 프레임이므로 IsShort=false
	if iduFrame1.IsShort {
		t.Errorf("Expected IsShort=false for LONG frame, got %v", iduFrame1.IsShort)
	}

	// 전체 40바이트가 올바르게 파싱되었는지 확인
	if !bytes.Equal(iduFrame1.Raw[:], iduLong) {
		t.Errorf("IDU LONG data mismatch")
	}

	// 두 번째 ReadFrame: ODU
	frameType2, oduFrame2, _, err2 := parser.ReadFrame()
	if err2 != nil {
		t.Fatalf("Frame 2 ReadFrame failed: %v", err2)
	}
	if frameType2 != 'A' {
		t.Fatalf("Expected ODU, got %c", frameType2)
	}
	if !bytes.Equal(oduFrame2.Raw[:], oduNext) {
		t.Errorf("ODU frame data mismatch after LONG frame")
	}

	t.Logf("PASS: LONG frame with timeout recovery works correctly")
}

// TestMultipleTimeoutsRecovery 는 연속 timeout 도 복구됨을 검증
func TestMultipleTimeoutsRecovery(t *testing.T) {
	iduShort := make([]byte, 20)
	iduShort[0] = 0x81
	iduShort[1] = 0x02

	oduNext := make([]byte, 20)
	oduNext[0] = 0x58

	stream := append(iduShort, []byte{0x00}...)
	stream = append(stream, oduNext...)

	// timeout 중 0x00 반환 (2회 timeout)
	timeoutReader := NewTimeoutSimulatingReader(stream, 20, 0x00)
	parser := NewIcp01FrameParser(timeoutReader)

	_, _, iduFrame1, err1 := parser.ReadFrame()
	if err1 != nil {
		t.Fatalf("ReadFrame failed: %v", err1)
	}

	if !iduFrame1.IsShort {
		t.Errorf("Expected IsShort=true after timeout recovery")
		return
	}

	frameType2, oduFrame2, _, err2 := parser.ReadFrame()
	if err2 != nil {
		t.Fatalf("Second ReadFrame failed: %v", err2)
	}

	if frameType2 != 'A' || oduFrame2.Raw[0] != 0x58 {
		t.Errorf("Second frame parsing failed after timeout recovery")
	}

	t.Logf("PASS: Multiple timeouts recovered, framing correct")
}

// TestTimeoutAwareFramingTiming 은 timing 독립성 검증:
// timeout 유무와 관계없이 동일한 프레임 시퀀스 파싱
func TestTimeoutAwareFramingTiming(t *testing.T) {
	iduShort := make([]byte, 20)
	iduShort[0] = 0x83

	oduFrame := make([]byte, 20)
	oduFrame[0] = 0x58

	stream := append(iduShort, oduFrame...)

	// 케이스 1: timeout 없음 (fragmented reader)
	normalReader := NewFragmentedReader(stream, 1)
	parser1 := NewIcp01FrameParser(normalReader)

	ft1, _, idu1, _ := parser1.ReadFrame()
	ft2, odu1, _, _ := parser1.ReadFrame()

	// 케이스 2: timeout 있음
	timeoutReader := NewTimeoutSimulatingReader(stream, 20, 0x58)
	parser2 := NewIcp01FrameParser(timeoutReader)

	ft1t, _, idu1t, _ := parser2.ReadFrame()
	ft2t, odu1t, _, _ := parser2.ReadFrame()

	// 두 경우가 동일한 결과여야 함 (timing 독립성)
	if ft1 != ft1t || idu1.IsShort != idu1t.IsShort {
		t.Errorf("Timing dependency detected: timeout affects framing decision")
		t.Logf("  Normal: type=%c IsShort=%v", ft1, idu1.IsShort)
		t.Logf("  Timeout: type=%c IsShort=%v", ft1t, idu1t.IsShort)
		return
	}

	if ft2 != ft2t || !bytes.Equal(odu1.Raw[:], odu1t.Raw[:]) {
		t.Errorf("Second frame differs between normal and timeout cases")
		return
	}

	t.Logf("PASS: Framing is timing-independent (timeout or not, same result)")
}
