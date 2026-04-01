package serial

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

// SerialFramer 는 시리얼 포트에서 메시지 프레이밍을 담당하는 인터페이스이다.
type SerialFramer interface {
	// Read 는 리더에서 하나의 프레임을 읽는다.
	Read(r io.Reader) ([]byte, error)
	// Write 는 라이터에 하나의 프레임을 쓴다.
	Write(w io.Writer, data []byte) error
}

// FramerOptions 는 프레이머 생성 옵션이다.
type FramerOptions struct {
	BufferSize     int           // 읽기 버퍼 크기 (rawFramer, newlineFramer, streamFramer 에 사용)
	Delimiter      byte          // 구분자 (newlineFramer 에 사용, 0이면 '\n')
	FixedSize      int           // 고정 크기 (fixedSizeFramer 에 사용)
	MaxMessageSize int           // 최대 메시지 크기 (lengthPrefixFramer, frameFramer 에 사용, 0=무제한)
	IdleTimeout    time.Duration // 유휴 타임아웃 (streamFramer 참조용, 실제 타이밍은 시리얼 포트 ReadTimeout 에 의존)

	// frame 프레이밍 전용 옵션
	STX                  []byte // 프레임 시작 마커
	ETX                  []byte // 프레임 종료 마커
	LengthOffset         int    // STX 부터 길이 필드까지 오프셋
	LengthSize           int    // 길이 필드 크기 (1 또는 2)
	LengthEndian         string // "big" 또는 "little"
	LengthIncludesHeader bool   // 길이에 헤더 포함 여부
	LengthAdjustment     int    // 디코딩된 길이에 더할 보정값
	Checksum             string // "none", "sum8", "xor"
}

// NewSerialFramer 는 프레이밍 타입에 따라 적절한 SerialFramer 구현체를 생성한다.
func NewSerialFramer(framingType string, opts FramerOptions) (SerialFramer, error) {
	switch framingType {
	case FramingRaw:
		size := opts.BufferSize
		if size <= 0 {
			size = DefaultBufferSize
		}
		return &rawFramer{bufferSize: size}, nil

	case FramingNewline:
		delim := opts.Delimiter
		if delim == 0 {
			delim = '\n'
		}
		size := opts.BufferSize
		if size <= 0 {
			size = DefaultBufferSize
		}
		return &newlineFramer{delimiter: delim, bufferSize: size}, nil

	case FramingLengthPrefix:
		return &lengthPrefixFramer{maxMessageSize: opts.MaxMessageSize}, nil

	case FramingFixedSize:
		return &fixedSizeFramer{fixedSize: opts.FixedSize}, nil

	case FramingStream:
		size := opts.BufferSize
		if size <= 0 {
			size = DefaultBufferSize
		}
		return &streamFramer{bufferSize: size}, nil

	case FramingFrame:
		return &frameFramer{
			stx:                 opts.STX,
			etx:                 opts.ETX,
			lengthOffset:        opts.LengthOffset,
			lengthSize:          opts.LengthSize,
			lengthEndian:        opts.LengthEndian,
			lengthIncludesHeader: opts.LengthIncludesHeader,
			lengthAdjustment:    opts.LengthAdjustment,
			checksum:            opts.Checksum,
			maxMessageSize:      opts.MaxMessageSize,
		}, nil

	default:
		return nil, fmt.Errorf("%w: %q", ErrInvalidFraming, framingType)
	}
}

// --- rawFramer: 원시 바이트 스트림 ---

type rawFramer struct {
	bufferSize int
}

func (f *rawFramer) Read(r io.Reader) ([]byte, error) {
	buf := make([]byte, f.bufferSize)
	n, err := r.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (f *rawFramer) Write(w io.Writer, data []byte) error {
	_, err := w.Write(data)
	return err
}

// --- newlineFramer: 구분자 기반 프레이밍 ---

type newlineFramer struct {
	delimiter  byte
	bufferSize int
}

func (f *newlineFramer) Read(r io.Reader) ([]byte, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, f.bufferSize), f.bufferSize)
	scanner.Split(f.splitFunc())

	if scanner.Scan() {
		return scanner.Bytes(), nil
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

func (f *newlineFramer) Write(w io.Writer, data []byte) error {
	buf := make([]byte, len(data)+1)
	copy(buf, data)
	buf[len(data)] = f.delimiter
	_, err := w.Write(buf)
	return err
}

// splitFunc 는 커스텀 구분자를 위한 bufio.SplitFunc 을 반환한다.
func (f *newlineFramer) splitFunc() bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		for i := 0; i < len(data); i++ {
			if data[i] == f.delimiter {
				return i + 1, data[:i], nil
			}
		}
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	}
}

// --- lengthPrefixFramer: 4바이트 빅엔디안 길이 접두사 ---

type lengthPrefixFramer struct {
	maxMessageSize int
}

func (f *lengthPrefixFramer) Read(r io.Reader) ([]byte, error) {
	// 4바이트 길이 헤더 읽기
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(header)

	// 최대 메시지 크기 검증
	if f.maxMessageSize > 0 && int(length) > f.maxMessageSize {
		return nil, fmt.Errorf("%w: %d bytes (max %d)", ErrMaxMessageSize, length, f.maxMessageSize)
	}

	// 페이로드 읽기
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (f *lengthPrefixFramer) Write(w io.Writer, data []byte) error {
	// 최대 메시지 크기 검증
	if f.maxMessageSize > 0 && len(data) > f.maxMessageSize {
		return fmt.Errorf("%w: %d bytes (max %d)", ErrMaxMessageSize, len(data), f.maxMessageSize)
	}

	// 4바이트 길이 헤더 + 페이로드 전송
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// --- fixedSizeFramer: 고정 크기 프레이밍 ---

type fixedSizeFramer struct {
	fixedSize int
}

func (f *fixedSizeFramer) Read(r io.Reader) ([]byte, error) {
	buf := make([]byte, f.fixedSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (f *fixedSizeFramer) Write(w io.Writer, data []byte) error {
	buf := make([]byte, f.fixedSize)
	copy(buf, data) // 짧으면 제로 패딩, 길면 잘림
	_, err := w.Write(buf)
	return err
}

// --- streamFramer: 유휴 타임아웃 기반 스트림 프레이밍 ---
// 수신 데이터를 누적하다가 idle timeout(기본 1ms) 동안 추가 데이터가 없으면
// 누적된 데이터를 하나의 프레임으로 반환한다.
// 실제 유휴 감지는 시리얼 포트의 ReadTimeout 설정에 의존한다.

type streamFramer struct {
	bufferSize int
}

func (f *streamFramer) Read(r io.Reader) ([]byte, error) {
	buf := make([]byte, f.bufferSize)
	var acc []byte

	for {
		n, err := r.Read(buf)
		if n > 0 {
			acc = append(acc, buf[:n]...)
			continue
		}
		// 추가 데이터 없음 (타임아웃) — 누적 데이터가 있으면 플러시
		if len(acc) > 0 {
			return acc, nil
		}
		// 누적 데이터 없이 실제 오류 발생
		if err != nil {
			return nil, err
		}
		// 순수 타임아웃, 아직 데이터 없음 — 계속 대기
	}
}

func (f *streamFramer) Write(w io.Writer, data []byte) error {
	_, err := w.Write(data)
	return err
}

// --- frameFramer: 프로토콜 수준 프레임 감지 (STX/길이/ETX/체크섬) ---

type frameFramer struct {
	stx                  []byte
	etx                  []byte
	lengthOffset         int
	lengthSize           int    // 1 또는 2
	lengthEndian         string // "big" 또는 "little"
	lengthIncludesHeader bool
	lengthAdjustment     int
	checksum             string // "none", "sum8", "xor"
	maxMessageSize       int
}

func (f *frameFramer) Read(r io.Reader) ([]byte, error) {
	// 1. STX 탐색 — 바이트 단위로 읽으며 STX 패턴 매칭
	stxBuf := make([]byte, len(f.stx))
	if err := f.findSTX(r, stxBuf); err != nil {
		return nil, err
	}

	// 2. 헤더 읽기 — STX 이후 length_offset 까지 추가 바이트 읽기
	headerExtra := f.lengthOffset - len(f.stx)
	var header []byte
	header = append(header, stxBuf...)
	if headerExtra > 0 {
		extra := make([]byte, headerExtra)
		if _, err := io.ReadFull(r, extra); err != nil {
			return nil, err
		}
		header = append(header, extra...)
	}

	// 3. 길이 필드 읽기
	lenBuf := make([]byte, f.lengthSize)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, err
	}
	header = append(header, lenBuf...)

	payloadLen := f.decodeLength(lenBuf)

	// length_includes_header 처리
	if f.lengthIncludesHeader {
		// 길이에 지금까지 읽은 바이트(header)가 포함되어 있으므로 빼준다
		payloadLen -= len(header)
		if payloadLen < 0 {
			payloadLen = 0
		}
	}

	// length_adjustment 보정
	payloadLen += f.lengthAdjustment
	if payloadLen < 0 {
		payloadLen = 0
	}

	// 최대 메시지 크기 검증
	totalSize := len(header) + payloadLen
	if f.maxMessageSize > 0 && totalSize > f.maxMessageSize {
		return nil, fmt.Errorf("%w: %d bytes (max %d)", ErrFrameTooLarge, totalSize, f.maxMessageSize)
	}

	// 4. 나머지 페이로드 읽기
	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}
	}

	frame := append(header, payload...)

	// 5. ETX 검증
	if len(f.etx) > 0 {
		etxStart := len(frame) - len(f.etx)
		if f.checksum != "none" {
			etxStart -= 1 // 체크섬 바이트 앞에 ETX
		}
		if etxStart < 0 || etxStart+len(f.etx) > len(frame) {
			return nil, fmt.Errorf("%w", ErrETXMismatch)
		}
		for i, b := range f.etx {
			if frame[etxStart+i] != b {
				return nil, fmt.Errorf("%w", ErrETXMismatch)
			}
		}
	}

	// 6. 체크섬 검증
	if f.checksum != "none" {
		csIdx := len(frame) - 1
		if csIdx < 0 {
			return nil, fmt.Errorf("%w", ErrChecksumMismatch)
		}
		expected := frame[csIdx]
		data := frame[:csIdx]
		var calculated byte
		switch f.checksum {
		case "sum8":
			calculated = checksumSum8(data)
		case "xor":
			calculated = checksumXOR(data)
		}
		if calculated != expected {
			return nil, fmt.Errorf("%w: expected 0x%02X, got 0x%02X", ErrChecksumMismatch, expected, calculated)
		}
	}

	return frame, nil
}

func (f *frameFramer) Write(w io.Writer, data []byte) error {
	_, err := w.Write(data)
	return err
}

// findSTX 는 리더에서 바이트 단위로 읽으며 STX 패턴을 찾는다.
func (f *frameFramer) findSTX(r io.Reader, buf []byte) error {
	single := make([]byte, 1)
	matchIdx := 0

	for {
		if _, err := io.ReadFull(r, single); err != nil {
			return err
		}
		if single[0] == f.stx[matchIdx] {
			buf[matchIdx] = single[0]
			matchIdx++
			if matchIdx == len(f.stx) {
				return nil
			}
		} else {
			// 매칭 실패 — 처음부터 다시
			matchIdx = 0
			if single[0] == f.stx[0] {
				buf[0] = single[0]
				matchIdx = 1
			}
		}
	}
}

// decodeLength 는 길이 필드 바이트를 정수로 디코딩한다.
func (f *frameFramer) decodeLength(buf []byte) int {
	if f.lengthSize == 1 {
		return int(buf[0])
	}
	// 2 바이트
	if f.lengthEndian == "little" {
		return int(binary.LittleEndian.Uint16(buf))
	}
	return int(binary.BigEndian.Uint16(buf))
}

// checksumSum8 는 데이터의 바이트 합을 계산한다.
func checksumSum8(data []byte) byte {
	var sum byte
	for _, b := range data {
		sum += b
	}
	return sum
}

// checksumXOR 는 데이터의 XOR 체크섬을 계산한다.
func checksumXOR(data []byte) byte {
	var result byte
	for _, b := range data {
		result ^= b
	}
	return result
}

// --- SerialConnReader: 시리얼 포트 전용 프레임 리더 ---

// SerialConnReader 는 시리얼 포트 전용 프레임 리더이다.
// newlineFramer 가 Read() 호출마다 새 bufio.Scanner 를 생성하는 문제를 해결하여
// 리더 수명 동안 하나의 scanner 를 유지한다.
type SerialConnReader struct {
	framer  SerialFramer
	reader  io.Reader
	scanner *bufio.Scanner
}

// NewSerialConnReader 는 새 SerialConnReader 를 생성한다.
// newlineFramer 인 경우 scanner 를 초기화하여 버퍼링 상태를 유지한다.
func NewSerialConnReader(framer SerialFramer, r io.Reader) *SerialConnReader {
	cr := &SerialConnReader{
		framer: framer,
		reader: r,
	}
	if nf, ok := framer.(*newlineFramer); ok {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, nf.bufferSize), nf.bufferSize)
		scanner.Split(nf.splitFunc())
		cr.scanner = scanner
	}
	return cr
}

// Read 는 리더에서 하나의 프레임을 읽는다.
// newlineFramer 인 경우 내부 scanner 를 재사용하여 버퍼 데이터 손실을 방지한다.
func (cr *SerialConnReader) Read() ([]byte, error) {
	if cr.scanner != nil {
		if cr.scanner.Scan() {
			b := cr.scanner.Bytes()
			result := make([]byte, len(b))
			copy(result, b)
			return result, nil
		}
		if err := cr.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	return cr.framer.Read(cr.reader)
}
