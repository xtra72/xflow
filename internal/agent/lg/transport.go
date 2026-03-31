package lg

import (
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------
// LGAPTransport 인터페이스
// ---------------------------------------------------------------------------

// LGAPTransport 는 LGAP 프로토콜의 트랜스포트 레이어 인터페이스이다.
type LGAPTransport interface {
	// Open 은 트랜스포트 연결을 연다.
	Open() error

	// Close 는 트랜스포트 연결을 닫는다.
	Close() error

	// Send 는 데이터를 전송한다.
	Send(data []byte) error

	// Receive 는 데이터를 수신한다. 읽은 바이트 수를 반환한다.
	Receive(buf []byte) (int, error)

	// Available 은 트랜스포트가 사용 가능한 상태인지 반환한다.
	Available() bool

	// Write 는 바이트 슬라이스를 트랜스포트로 전송하고 전송된 바이트 수를 반환한다.
	Write(data []byte) (int, error)
}

// ---------------------------------------------------------------------------
// SerialOpener: 시리얼 포트 팩토리 (테스트에서 override 가능)
// ---------------------------------------------------------------------------

// LGAPSerialOpener 는 시리얼 포트를 여는 팩토리 함수이다.
// 테스트에서 mock 으로 대체할 수 있다.
var LGAPSerialOpener func(port string, baudRate, dataBits, stopBits int, parity string) (io.ReadWriteCloser, error)

// ---------------------------------------------------------------------------
// isLGAPConnectionError: 연결 끊김 판별
// ---------------------------------------------------------------------------

// isLGAPConnectionError 는 에러가 연결 끊김을 나타내는지 판별한다.
// 타임아웃 에러는 제외하여 read deadline 만료가 연결 끊김으로 처리되지 않도록 한다.
func isLGAPConnectionError(err error) bool {
	if err == nil {
		return false
	}
	// 타임아웃 에러는 연결 끊김이 아니다.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	if errors.Is(err, io.ErrClosedPipe) {
		return true
	}
	// ENXIO: "device not configured" — 시리얼 디바이스 분리
	if errors.Is(err, syscall.ENXIO) {
		return true
	}
	// EIO: "input/output error" — USB 시리얼 디바이스 분리 시 발생
	if errors.Is(err, syscall.EIO) {
		return true
	}
	// OS 레벨 연결 끊김 (net.OpError)
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var syscallErr syscall.Errno
		if errors.As(opErr.Err, &syscallErr) {
			switch syscallErr {
			case syscall.ECONNRESET, syscall.EPIPE, syscall.ECONNABORTED:
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// lgapSerialTransport
// ---------------------------------------------------------------------------

// lgapSerialTransport 는 시리얼 포트 기반 LGAP 트랜스포트이다.
type lgapSerialTransport struct {
	port        string
	baudRate    int
	dataBits    int
	stopBits    int
	parity      string
	readTimeout time.Duration
	conn        io.ReadWriteCloser
	mu          sync.RWMutex // conn 접근 보호 (RLock=I/O 동시, Lock=conn 교체)
	open        atomic.Bool  // 연결 상태 (Available 에서 lock-free 조회)
}

// newLGAPSerialTransport 는 설정으로부터 lgapSerialTransport 를 생성한다.
func newLGAPSerialTransport(cfg LGAPConfig) *lgapSerialTransport {
	return &lgapSerialTransport{
		port:        cfg.SerialPort,
		baudRate:    cfg.BaudRate,
		dataBits:    cfg.DataBits,
		stopBits:    cfg.StopBits,
		parity:      cfg.Parity,
		readTimeout: cfg.ReadTimeout,
	}
}

// Open 은 시리얼 포트를 열고 연결을 설정한다.
func (s *lgapSerialTransport) Open() error {
	opener := LGAPSerialOpener
	if opener == nil {
		return io.ErrClosedPipe // 기본 opener 가 없으면 에러
	}

	// 시리얼 포트 연결은 블로킹될 수 있으므로 mutex 바깥에서 수행한다.
	// Available() 등 상태 조회가 블록되는 것을 방지한다.
	conn, err := opener(s.port, s.baudRate, s.dataBits, s.stopBits, s.parity)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.conn = conn
	s.open.Store(true)
	return nil
}

// Close 는 시리얼 포트 연결을 닫는다.
func (s *lgapSerialTransport) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.open.Load() || s.conn == nil {
		return nil
	}

	err := s.conn.Close()
	s.conn = nil
	s.open.Store(false)
	return err
}

// Send 는 시리얼 포트로 데이터를 전송한다.
// LGAP 는 RS-485 preamble 이 필요 없다 (동기식 master/slave 방식).
func (s *lgapSerialTransport) Send(data []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.open.Load() || s.conn == nil {
		return ErrTransportNotConnected
	}

	_, err := s.conn.Write(data)
	if err != nil && isLGAPConnectionError(err) {
		s.open.Store(false)
	}
	return err
}

// Receive 는 시리얼 포트에서 데이터를 수신한다.
// RLock 사용: Write 와 동시 실행 허용 (시리얼 포트는 전이중 I/O 지원).
func (s *lgapSerialTransport) Receive(buf []byte) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.open.Load() || s.conn == nil {
		return 0, ErrTransportNotConnected
	}

	n, err := s.conn.Read(buf)
	if err != nil && isLGAPConnectionError(err) {
		s.open.Store(false)
	}
	return n, err
}

// Write 는 시리얼 포트로 데이터를 전송하고 전송된 바이트 수를 반환한다.
// RLock 사용: Receive 와 동시 실행 허용 (시리얼 포트는 전이중 I/O 지원).
func (s *lgapSerialTransport) Write(data []byte) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.open.Load() || s.conn == nil {
		return 0, ErrTransportNotConnected
	}

	n, err := s.conn.Write(data)
	if err != nil && isLGAPConnectionError(err) {
		s.open.Store(false)
	}
	return n, err
}

// Available 은 시리얼 포트가 열려 있는지 반환한다.
// lock-free: API 에서 TransportConnected() 를 호출할 때 Send/Receive 의
// I/O 블로킹에 영향받지 않도록 atomic 으로 읽는다.
func (s *lgapSerialTransport) Available() bool {
	return s.open.Load()
}
