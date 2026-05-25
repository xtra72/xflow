package century

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

// ScannerStats 는 FrameScanner 누적 통계이다.
// 카운터는 atomic 으로 갱신되므로 Stats() 호출자는 락 없이 안전하게 스냅샷을 얻을 수 있다.
type ScannerStats struct {
	// TotalFrames 는 다층 검증을 통과한 유효 프레임 누적 수이다.
	TotalFrames uint64

	// DropCount 는 재동기화 과정에서 1바이트씩 폐기된 횟수이다.
	// (헤더 검증 / 길이 검증 / payload prefix 검증 실패로 인한 ParseFrame 에러 포함)
	DropCount uint64

	// CRCErrors 는 ErrInvalidCRC 발생 횟수이다. DropCount 의 부분집합이다.
	CRCErrors uint64
}

// FrameScanner 는 io.Reader 로부터 Century 프레임을 스트리밍 추출한다.
//
// 동작 원리:
//   - 내부 버퍼에 바이트를 적재한다 (bufio.Reader 기반).
//   - 매 Next() 호출에서 헤더 8바이트를 사전 읽기 → payload_length 파싱
//     → 나머지(payload + CRC 2B)를 읽은 뒤 ParseFrame 으로 다층 검증.
//   - 검증 실패 시 첫 바이트를 1개 폐기 (DropCount++) 하고 재동기화한다.
//   - 검증 성공 시 TotalFrames 를 증가시키고 *Frame 을 반환한다.
//
// FrameScanner 는 concurrent-safe 하지 않다 — 단일 goroutine 에서만 Next 를 호출하라.
// Stats() 은 별도 goroutine 에서 호출 가능하다.
//
// (REQ-CENTURY-003)
type FrameScanner struct {
	br *bufio.Reader

	// ReadTimeout 은 in-frame inter-byte gap 의 상한이다. 0 이면 비활성.
	// SPEC §5.1: 폴링 주기 ~512ms 에 비해 충분한 ~50ms 가 권장 기본값.
	// io.Reader 자체가 timeout 을 지원하지 않으므로, 실제 적용은 호출자가
	// 자기만의 timeout-aware reader 를 주입해야 한다. 이 필드는 향후 사용을
	// 위한 후크이며 현재 구현에서는 별도 강제하지 않는다.
	ReadTimeout time.Duration

	totalFrames atomic.Uint64
	dropCount   atomic.Uint64
	crcErrors   atomic.Uint64

	// lastErr 는 가장 최근 fatal error (io.EOF 등) 를 보관한다.
	lastErr error
}

// NewFrameScanner 는 새 FrameScanner 를 생성한다.
// r 가 *bufio.Reader 이면 그대로 사용하고, 아니면 wrap 한다.
func NewFrameScanner(r io.Reader) *FrameScanner {
	br, ok := r.(*bufio.Reader)
	if !ok {
		// 4 KB 는 한 사이클(약 165 B) 의 약 24 배. 단일 frame 의 최대 270 B 를 충분히 수용한다.
		br = bufio.NewReaderSize(r, 4096)
	}
	return &FrameScanner{
		br:          br,
		ReadTimeout: 50 * time.Millisecond,
	}
}

// Stats 는 통계 스냅샷을 반환한다 (concurrent-safe).
func (s *FrameScanner) Stats() ScannerStats {
	return ScannerStats{
		TotalFrames: s.totalFrames.Load(),
		DropCount:   s.dropCount.Load(),
		CRCErrors:   s.crcErrors.Load(),
	}
}

// Err 는 가장 최근 fatal error (io.EOF, context cancel 등) 를 반환한다.
func (s *FrameScanner) Err() error {
	return s.lastErr
}

// Next 는 다음 유효 프레임을 반환한다.
//
// 동작 흐름:
//  1. peek 으로 첫 8바이트(헤더 사전 읽기)를 확인한다. 부족하면 io.EOF.
//  2. payload_length 가 합리 범위 안인지 빠르게 검증한다.
//     비합리면 (혹은 reserved / fc 가 명백히 깨졌으면) 1바이트 폐기 후 재시도.
//  3. 전체 frame (= header + payload + CRC) 만큼을 한 번에 읽고 ParseFrame.
//  4. ParseFrame 실패 시 1바이트 폐기 후 재시도, 통계 갱신.
//
// ctx 가 cancel 되면 io.EOF 또는 ctx.Err() 가 반환된다.
//
// (REQ-CENTURY-003, REQ-CENTURY-011)
func (s *FrameScanner) Next(ctx context.Context) (*Frame, error) {
	for {
		if err := ctx.Err(); err != nil {
			s.lastErr = err
			return nil, err
		}

		// --- 헤더 peek ---
		header, err := s.peekHeader()
		if err != nil {
			s.lastErr = err
			return nil, err
		}

		// 빠른 sanity check: reserved / fc / payload_length 가 명백히 깨져 있으면
		// CRC 까지 가지 말고 즉시 1바이트 폐기로 재동기화한다.
		// (이는 단순한 fast-path; 최종 검증은 ParseFrame 이 수행한다.)
		payloadLen := binary.LittleEndian.Uint16(header[4:6])
		reserved := header[6]
		fc := header[7]
		if payloadLen == 0 ||
			int(payloadLen) > MaxPayloadLength ||
			reserved != 0x00 ||
			!isValidFunctionCode(fc) {
			s.dropOne()
			continue
		}

		// --- 전체 frame 읽기 ---
		total := HeaderLength + int(payloadLen) + CRCLength
		raw := make([]byte, total)
		if _, err := io.ReadFull(s.br, raw); err != nil {
			// 헤더는 가지고 있었지만 트레일이 끊겼다 → fatal (caller 가 io.EOF 처리)
			s.lastErr = err
			return nil, err
		}

		// --- 다층 검증 ---
		f, perr := ParseFrame(raw)
		if perr == nil {
			s.totalFrames.Add(1)
			return f, nil
		}
		if errors.Is(perr, ErrInvalidCRC) {
			s.crcErrors.Add(1)
		}
		// ParseFrame 이 실패했지만 이미 raw 만큼 reader 에서 소비했다.
		// 재동기화를 위해, 다음 검색은 raw[1:] 부터 다시 후보로 보아야 한다.
		// bufio.Reader 의 unread 는 1바이트만 가능하므로, 대신 직접 unread queue 로
		// 처리한다 — 가장 안전한 방법: raw[1:] 을 push-back 하고 1바이트만 폐기로 카운트.
		s.dropCount.Add(1)
		if len(raw) > 1 {
			// raw[1:] 의 모든 바이트를 다시 reader 앞에 prepend 한다.
			s.prepend(raw[1:])
		}
	}
}

// peekHeader 는 다음 8바이트 헤더 후보를 peek 한다 (소비하지 않는다).
func (s *FrameScanner) peekHeader() ([]byte, error) {
	header, err := s.br.Peek(HeaderLength)
	if err != nil {
		return nil, err
	}
	if len(header) < HeaderLength {
		return nil, io.ErrUnexpectedEOF
	}
	// peek 결과는 bufio 내부 버퍼를 참조한다 — 호출자가 보존하려면 복사해야 한다.
	// 여기서는 즉시 LE 디코딩만 하므로 사본은 불필요하다.
	out := make([]byte, HeaderLength)
	copy(out, header)
	return out, nil
}

// dropOne 은 reader 에서 1바이트 폐기하고 dropCount 를 증가시킨다 (재동기화).
func (s *FrameScanner) dropOne() {
	if _, err := s.br.Discard(1); err == nil {
		s.dropCount.Add(1)
	}
}

// prepend 는 이미 읽어둔 바이트를 reader 앞에 다시 push 한다.
// bufio.Reader 는 push-back 을 직접 지원하지 않으므로, 새 reader 로 교체한다.
func (s *FrameScanner) prepend(data []byte) {
	if len(data) == 0 {
		return
	}
	// 남은 bufio 내부 데이터를 모두 꺼낸 뒤, data 와 결합하여 새 reader 를 만든다.
	remaining := s.br.Buffered()
	tail := make([]byte, remaining)
	if remaining > 0 {
		if _, err := io.ReadFull(s.br, tail); err != nil {
			// 사실상 발생하지 않음 (Buffered() == 실제 사용 가능량). 안전망.
			return
		}
	}
	combined := make([]byte, 0, len(data)+len(tail))
	combined = append(combined, data...)
	combined = append(combined, tail...)
	s.br = bufio.NewReaderSize(io.MultiReader(&sliceReader{b: combined}, s.br), 4096)
}

// sliceReader exposes a byte slice as an io.Reader. It is used by
// FrameScanner.prepend to push previously-consumed bytes back onto the front
// of the read stream via io.MultiReader.
type sliceReader struct {
	b []byte
}

func (r *sliceReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}

// String formats stats for logging (debug aid).
func (s ScannerStats) String() string {
	return fmt.Sprintf("ScannerStats{frames=%d, drops=%d, crc_errors=%d}",
		s.TotalFrames, s.DropCount, s.CRCErrors)
}
