package serial

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
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
	BufferSize     int  // 읽기 버퍼 크기 (rawFramer, newlineFramer 에 사용)
	Delimiter      byte // 구분자 (newlineFramer 에 사용, 0이면 '\n')
	FixedSize      int  // 고정 크기 (fixedSizeFramer 에 사용)
	MaxMessageSize int  // 최대 메시지 크기 (lengthPrefixFramer 에 사용, 0=무제한)
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
