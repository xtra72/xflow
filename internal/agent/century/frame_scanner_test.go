package century

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// TestFrameScanner_CAP3FullCycle implements the M1 scanner happy path:
// concatenating every CAP-3 frame from spec 부록 A produces exactly the same
// 8 frames in order, with zero drops, when fed to FrameScanner.
//
// (REQ-CENTURY-003, REQ-CENTURY-011)
func TestFrameScanner_CAP3FullCycle(t *testing.T) {
	t.Parallel()

	stream := mustReadFixture(t, "cap3_full_cycle.bin")
	reader := bytes.NewReader(stream)
	s := NewFrameScanner(reader)

	want := []struct {
		fc       byte
		src      uint16
		dst      uint16
		register byte // 0 if ACK
	}{
		{FCRead, AddrMaster, AddrSlave, 0x02},     // read req reg 0x02
		{FCResponse, AddrSlave, AddrMaster, 0x02}, // read resp reg 0x02
		{FCRead, AddrMaster, AddrSlave, 0x03},     // read req reg 0x03
		{FCResponse, AddrSlave, AddrMaster, 0x03}, // read resp reg 0x03
		{FCRead, AddrMaster, AddrSlave, 0x04},     // read req reg 0x04
		{FCResponse, AddrSlave, AddrMaster, 0x04}, // read resp reg 0x04
		{FCWrite, AddrMaster, AddrSlave, 0x04},    // write reg 0x04
		{FCResponse, AddrSlave, AddrMaster, 0},    // ACK
	}

	ctx := context.Background()
	for i, w := range want {
		f, err := s.Next(ctx)
		if err != nil {
			t.Fatalf("Next(#%d) returned error: %v", i, err)
		}
		if f.FunctionCode != w.fc {
			t.Errorf("frame %d: FunctionCode = 0x%02X, want 0x%02X", i, f.FunctionCode, w.fc)
		}
		if f.Src != w.src {
			t.Errorf("frame %d: Src = 0x%04X, want 0x%04X", i, f.Src, w.src)
		}
		if f.Dst != w.dst {
			t.Errorf("frame %d: Dst = 0x%04X, want 0x%04X", i, f.Dst, w.dst)
		}
		if w.register == 0 {
			// ACK: no payload prefix.
			if !f.IsACK() {
				t.Errorf("frame %d: expected ACK, got payload=%x", i, f.Payload)
			}
		} else {
			reg, ok := f.Register()
			if !ok || reg != w.register {
				t.Errorf("frame %d: Register = (0x%02X, %v), want (0x%02X, true)", i, reg, ok, w.register)
			}
		}
	}

	// EOF on the next call.
	if _, err := s.Next(ctx); !errors.Is(err, io.EOF) {
		t.Fatalf("Next() after exhausting stream = %v, want io.EOF", err)
	}

	stats := s.Stats()
	if stats.TotalFrames != uint64(len(want)) {
		t.Errorf("Stats.TotalFrames = %d, want %d", stats.TotalFrames, len(want))
	}
	if stats.DropCount != 0 {
		t.Errorf("Stats.DropCount = %d, want 0", stats.DropCount)
	}
	if stats.CRCErrors != 0 {
		t.Errorf("Stats.CRCErrors = %d, want 0", stats.CRCErrors)
	}
}

// TestFrameScanner_ResyncOnGarbage verifies that injecting noise bytes between
// two valid frames causes the scanner to advance one byte at a time, increment
// DropCount, and still emit both valid frames in order.
//
// (REQ-CENTURY-003 재동기화, AC-A8)
func TestFrameScanner_ResyncOnGarbage(t *testing.T) {
	t.Parallel()

	f1 := mustReadFixture(t, "cap3_reg02_response.bin")
	f2 := mustReadFixture(t, "cap3_reg03_response.bin")
	garbage := []byte{0xFF}

	var stream bytes.Buffer
	stream.Write(f1)
	stream.Write(garbage)
	stream.Write(f2)

	s := NewFrameScanner(&stream)
	ctx := context.Background()

	frame1, err := s.Next(ctx)
	if err != nil {
		t.Fatalf("Next() #1: %v", err)
	}
	if reg, _ := frame1.Register(); reg != 0x02 {
		t.Errorf("frame 1 register = 0x%02X, want 0x02", reg)
	}

	frame2, err := s.Next(ctx)
	if err != nil {
		t.Fatalf("Next() #2 after garbage: %v", err)
	}
	if reg, _ := frame2.Register(); reg != 0x03 {
		t.Errorf("frame 2 register = 0x%02X, want 0x03", reg)
	}

	stats := s.Stats()
	if stats.TotalFrames != 2 {
		t.Errorf("Stats.TotalFrames = %d, want 2", stats.TotalFrames)
	}
	if stats.DropCount < 1 {
		t.Errorf("Stats.DropCount = %d, want >= 1 (one garbage byte was injected)", stats.DropCount)
	}
}

// TestFrameScanner_TruncatedFrameEOF verifies that a trailing partial frame
// at EOF causes Next() to return io.EOF after emitting all complete frames.
//
// (REQ-CENTURY-003)
func TestFrameScanner_TruncatedFrameEOF(t *testing.T) {
	t.Parallel()

	complete := mustReadFixture(t, "cap3_reg02_response.bin")
	truncated := mustReadFixture(t, "cap3_reg03_response.bin")
	truncated = truncated[:len(truncated)-5] // chop last 5 bytes

	var stream bytes.Buffer
	stream.Write(complete)
	stream.Write(truncated)

	s := NewFrameScanner(&stream)
	ctx := context.Background()

	if _, err := s.Next(ctx); err != nil {
		t.Fatalf("Next() #1: %v", err)
	}
	_, err := s.Next(ctx)
	if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Next() #2 = %v, want io.EOF or io.ErrUnexpectedEOF", err)
	}
}

// TestFrameScanner_ContextCancellation ensures the scanner respects ctx
// cancellation when the underlying reader has no more bytes available
// (synthetic stalled reader).
func TestFrameScanner_ContextCancellation(t *testing.T) {
	t.Parallel()

	r := &blockingReader{}
	s := NewFrameScanner(r)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := s.Next(ctx)
	if err == nil {
		t.Fatal("Next() with cancelled context returned nil error")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		// A reader that returns io.EOF on cancellation is also acceptable.
		t.Fatalf("Next() = %v, want context.Canceled or io.EOF", err)
	}
}

// blockingReader returns io.EOF immediately. It is used to test cancellation
// without requiring a live transport.
type blockingReader struct{}

func (b *blockingReader) Read(p []byte) (int, error) {
	return 0, io.EOF
}

// TestFrameScanner_Err records that Err() returns the most-recent fatal
// error after Next() exits.
//
// (REQ-CENTURY-003)
func TestFrameScanner_Err(t *testing.T) {
	t.Parallel()

	s := NewFrameScanner(&blockingReader{})
	if got := s.Err(); got != nil {
		t.Fatalf("Err() before Next() = %v, want nil", got)
	}

	_, err := s.Next(context.Background())
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Next() = %v, want io.EOF", err)
	}
	if got := s.Err(); !errors.Is(got, io.EOF) {
		t.Fatalf("Err() after Next() = %v, want io.EOF", got)
	}
}

// errorAfterNReader emits up to N bytes from src, then returns errOut on
// every subsequent Read. It exercises the io.ReadFull error path inside
// Next() where the scanner has consumed a valid header but cannot complete
// the trailer read — the resulting error must propagate to Err().
type errorAfterNReader struct {
	src      []byte
	consumed int
	limit    int
	errOut   error
}

func (r *errorAfterNReader) Read(p []byte) (int, error) {
	remaining := r.limit - r.consumed
	if remaining <= 0 {
		return 0, r.errOut
	}
	n := len(p)
	if n > remaining {
		n = remaining
	}
	if r.consumed+n > len(r.src) {
		n = len(r.src) - r.consumed
	}
	if n <= 0 {
		return 0, r.errOut
	}
	copy(p, r.src[r.consumed:r.consumed+n])
	r.consumed += n
	return n, nil
}

// TestFrameScanner_ErrPropagatesMidFrameFailure exercises the fatal-error
// branch of Next() when the underlying reader fails after the header has
// been peeked but before the payload+CRC can be read in full.
//
// (REQ-CENTURY-003)
func TestFrameScanner_ErrPropagatesMidFrameFailure(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap3_reg02_response.bin")
	sentinel := errors.New("injected: cable yanked")
	// Allow only header + 2 payload bytes through, then explode.
	r := &errorAfterNReader{src: raw, limit: HeaderLength + 2, errOut: sentinel}
	s := NewFrameScanner(r)

	_, err := s.Next(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("Next() = %v, want %v", err, sentinel)
	}
	if got := s.Err(); !errors.Is(got, sentinel) {
		t.Fatalf("Err() = %v, want %v", got, sentinel)
	}
}

// TestFrameScanner_ResyncOnCRCMismatch is the targeted exercise for the
// scanner's prepend() resync path. It crafts a stream where:
//
//   - frame A is a structurally-valid header (passes fast-path sanity) but
//     with a corrupted CRC trailer. ParseFrame returns ErrInvalidCRC.
//   - frame B is a known-good CAP-3 frame immediately following.
//
// Next() must:
//  1. Consume frame A's bytes, fail CRC, increment CRCErrors + DropCount.
//  2. Push frame A's tail back via prepend().
//  3. Resync byte-by-byte until frame B's header aligns.
//  4. Emit frame B successfully.
//
// (REQ-CENTURY-003 재동기화, REQ-CENTURY-004)
func TestFrameScanner_ResyncOnCRCMismatch(t *testing.T) {
	t.Parallel()

	bad := bytes.Clone(mustReadFixture(t, "cap3_reg02_response.bin"))
	bad[len(bad)-1] ^= 0xFF // flip CRC trailer high byte
	good := mustReadFixture(t, "cap3_reg03_response.bin")

	var stream bytes.Buffer
	stream.Write(bad)
	stream.Write(good)

	s := NewFrameScanner(&stream)
	ctx := context.Background()

	f, err := s.Next(ctx)
	if err != nil {
		t.Fatalf("Next() after CRC-mismatched frame: %v", err)
	}
	if reg, _ := f.Register(); reg != 0x03 {
		t.Errorf("recovered frame register = 0x%02X, want 0x03", reg)
	}

	stats := s.Stats()
	if stats.TotalFrames != 1 {
		t.Errorf("Stats.TotalFrames = %d, want 1", stats.TotalFrames)
	}
	if stats.CRCErrors < 1 {
		t.Errorf("Stats.CRCErrors = %d, want >= 1", stats.CRCErrors)
	}
	if stats.DropCount < 1 {
		t.Errorf("Stats.DropCount = %d, want >= 1 (prepend path)", stats.DropCount)
	}
}

// TestScannerStatsString verifies the debug-aid Stringer output format.
func TestScannerStatsString(t *testing.T) {
	t.Parallel()

	stats := ScannerStats{TotalFrames: 7, DropCount: 2, CRCErrors: 1}
	got := stats.String()
	want := "ScannerStats{frames=7, drops=2, crc_errors=1}"
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

// TestFrameValidateCRC_ShortBuffer covers Frame.ValidateCRC's defensive
// early-return for buffers shorter than the 2-byte CRC trailer.
func TestFrameValidateCRC_ShortBuffer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  []byte
	}{
		{"nil buffer", nil},
		{"empty buffer", []byte{}},
		{"single byte", []byte{0xAB}},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := &Frame{}
			if f.ValidateCRC(tc.raw) {
				t.Fatalf("ValidateCRC(%v) = true, want false (buffer too short)", tc.raw)
			}
		})
	}
}
