package socket

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

// Framer 는 소켓 연결에서 메시지 프레이밍을 담당하는 인터페이스이다.
type Framer interface {
	// Read 는 연결에서 하나의 프레임을 읽는다.
	Read(conn net.Conn) ([]byte, error)
	// Write 는 연결에 하나의 프레임을 쓴다.
	Write(conn net.Conn, data []byte) error
}

// FramerOptions 는 프레이머 생성 옵션이다.
type FramerOptions struct {
	BufferSize     int           // 읽기 버퍼 크기 (RawFramer, NewlineFramer 에 사용)
	Delimiter      byte          // 구분자 (NewlineFramer 에 사용, 0이면 '\n')
	FixedSize      int           // 고정 크기 (FixedSizeFramer 에 사용)
	MaxMessageSize int           // 최대 메시지 크기 (LengthPrefixFramer 에 사용, 0=무제한)
	WriteTimeout   time.Duration // conn.Write 쓰기 데드라인 (0이면 데드라인 미설정)
}

// setWriteDeadline 은 conn.Write 직전에 쓰기 데드라인을 설정한다.
// timeout <= 0 이면 no-op (데드라인 미설정, 기존 동작 보존).
//
// 데드라인이 없으면 stale 클라이언트가 수신을 멈춰 커널 송신버퍼가 포화될 때
// conn.Write 가 무한 블록되어 상위(processSend/Process)와 flow 를 정지시킨다.
// 데드라인을 걸면 초과 시 timeout 에러가 반환되어 정상적으로 상위로 전파된다.
func setWriteDeadline(conn net.Conn, timeout time.Duration) {
	if timeout > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(timeout))
	}
}

// NewFramer 는 프레이밍 타입에 따라 적절한 Framer 구현체를 생성한다.
func NewFramer(framingType string, opts FramerOptions) (Framer, error) {
	switch framingType {
	case FramingRaw:
		size := opts.BufferSize
		if size <= 0 {
			size = DefaultBufferSize
		}
		return &rawFramer{bufferSize: size, writeTimeout: opts.WriteTimeout}, nil

	case FramingNewline:
		delim := opts.Delimiter
		if delim == 0 {
			delim = '\n'
		}
		size := opts.BufferSize
		if size <= 0 {
			size = DefaultBufferSize
		}
		return &newlineFramer{delimiter: delim, bufferSize: size, writeTimeout: opts.WriteTimeout}, nil

	case FramingLengthPrefix:
		return &lengthPrefixFramer{maxMessageSize: opts.MaxMessageSize, writeTimeout: opts.WriteTimeout}, nil

	case FramingFixedSize:
		return &fixedSizeFramer{fixedSize: opts.FixedSize, writeTimeout: opts.WriteTimeout}, nil

	default:
		return nil, fmt.Errorf("%w: %q", ErrInvalidFraming, framingType)
	}
}

// --- rawFramer: 원시 바이트 스트림 ---

type rawFramer struct {
	bufferSize   int
	writeTimeout time.Duration
}

func (f *rawFramer) Read(conn net.Conn) ([]byte, error) {
	buf := make([]byte, f.bufferSize)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	// 2026-05-14 hotfix: three-index slice 로 cap 을 n 으로 제한한다.
	// cap == bufferSize 이면 downstream 의 append 가 공유 backing 배열에
	// 써넣어 다른 프레임을 변조할 수 있다.
	return buf[:n:n], nil
}

func (f *rawFramer) Write(conn net.Conn, data []byte) error {
	setWriteDeadline(conn, f.writeTimeout)
	_, err := conn.Write(data)
	return err
}

// --- newlineFramer: 구분자 기반 프레이밍 ---

type newlineFramer struct {
	delimiter    byte
	bufferSize   int
	writeTimeout time.Duration
}

func (f *newlineFramer) Read(conn net.Conn) ([]byte, error) {
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, f.bufferSize), f.bufferSize)
	scanner.Split(f.splitFunc())

	if scanner.Scan() {
		// 2026-05-14 hotfix: bufio.Scanner.Bytes() 는 다음 Scan() 호출에 의해
		// 무효화되는 슬라이스를 반환하므로, 호출자에게 넘기기 전 복사한다.
		b := scanner.Bytes()
		out := make([]byte, len(b))
		copy(out, b)
		return out, nil
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

func (f *newlineFramer) Write(conn net.Conn, data []byte) error {
	buf := make([]byte, len(data)+1)
	copy(buf, data)
	buf[len(data)] = f.delimiter
	setWriteDeadline(conn, f.writeTimeout)
	_, err := conn.Write(buf)
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
	writeTimeout   time.Duration
}

func (f *lengthPrefixFramer) Read(conn net.Conn) ([]byte, error) {
	// 4바이트 길이 헤더 읽기
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(header)

	// 최대 메시지 크기 검증
	if f.maxMessageSize > 0 && int(length) > f.maxMessageSize {
		return nil, fmt.Errorf("%w: %d bytes (max %d)", ErrMaxMessageSize, length, f.maxMessageSize)
	}

	// 페이로드 읽기
	payload := make([]byte, length)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (f *lengthPrefixFramer) Write(conn net.Conn, data []byte) error {
	// 최대 메시지 크기 검증
	if f.maxMessageSize > 0 && len(data) > f.maxMessageSize {
		return fmt.Errorf("%w: %d bytes (max %d)", ErrMaxMessageSize, len(data), f.maxMessageSize)
	}

	// 4바이트 길이 헤더 + 페이로드 전송
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	setWriteDeadline(conn, f.writeTimeout)
	if _, err := conn.Write(header); err != nil {
		return err
	}
	setWriteDeadline(conn, f.writeTimeout)
	_, err := conn.Write(data)
	return err
}

// --- fixedSizeFramer: 고정 크기 프레이밍 ---

type fixedSizeFramer struct {
	fixedSize    int
	writeTimeout time.Duration
}

func (f *fixedSizeFramer) Read(conn net.Conn) ([]byte, error) {
	buf := make([]byte, f.fixedSize)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (f *fixedSizeFramer) Write(conn net.Conn, data []byte) error {
	buf := make([]byte, f.fixedSize)
	copy(buf, data) // 짧으면 제로 패딩, 길면 잘림
	setWriteDeadline(conn, f.writeTimeout)
	_, err := conn.Write(buf)
	return err
}

// --- ConnReader: 연결별 프레임 리더 ---

// ConnReader 는 연결별 프레임 리더이다.
// newlineFramer 가 Read() 호출마다 새 bufio.Scanner 를 생성하는 문제를 해결하여
// 연결 수명 동안 하나의 scanner 를 유지한다.
type ConnReader struct {
	framer  Framer
	conn    net.Conn
	scanner *bufio.Scanner
}

// NewConnReader 는 새 ConnReader 를 생성한다.
// newlineFramer 인 경우 scanner 를 초기화하여 버퍼링 상태를 유지한다.
func NewConnReader(framer Framer, conn net.Conn) *ConnReader {
	cr := &ConnReader{
		framer: framer,
		conn:   conn,
	}
	if nf, ok := framer.(*newlineFramer); ok {
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, nf.bufferSize), nf.bufferSize)
		scanner.Split(nf.splitFunc())
		cr.scanner = scanner
	}
	return cr
}

// Read 는 연결에서 하나의 프레임을 읽는다.
// newlineFramer 인 경우 내부 scanner 를 재사용하여 버퍼 데이터 손실을 방지한다.
func (cr *ConnReader) Read() ([]byte, error) {
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
	return cr.framer.Read(cr.conn)
}
