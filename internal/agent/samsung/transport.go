package samsung

import (
	"errors"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------
// NASATransport 인터페이스 (REQ-02-01)
// ---------------------------------------------------------------------------

// NASATransport 는 Samsung NASA HVAC 프로토콜의 트랜스포트 레이어 인터페이스이다.
type NASATransport interface {
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
}

// ---------------------------------------------------------------------------
// SerialOpener: 시리얼 포트 팩토리 (테스트에서 override 가능)
// ---------------------------------------------------------------------------

// SerialOpener 는 시리얼 포트를 여는 팩토리 함수이다.
// 테스트에서 mock 으로 대체할 수 있다.
var SerialOpener func(port string, baudRate, dataBits, stopBits int, parity string) (io.ReadWriteCloser, error)

// isConnectionError returns true for errors that indicate a broken connection.
// Timeout errors are excluded so that read deadline expiry does not mark the
// connection as closed.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	// Timeout errors (e.g. net.Error with Timeout() == true) are NOT connection errors.
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
	// Detect OS-level connection reset / broken pipe via net.OpError.
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
// NASASerialTransport (REQ-02-02)
// ---------------------------------------------------------------------------

// NASASerialTransport 는 시리얼 포트 기반 NASA 트랜스포트이다.
type NASASerialTransport struct {
	port     string
	baudRate int
	dataBits int
	stopBits int
	parity   string
	conn     io.ReadWriteCloser
	mu       sync.Mutex
	open     bool
}

// Open 은 시리얼 포트를 열고 연결을 설정한다.
func (s *NASASerialTransport) Open() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	opener := SerialOpener
	if opener == nil {
		return io.ErrClosedPipe // 기본 opener 가 없으면 에러
	}

	conn, err := opener(s.port, s.baudRate, s.dataBits, s.stopBits, s.parity)
	if err != nil {
		return err
	}

	s.conn = conn
	s.open = true
	return nil
}

// Close 는 시리얼 포트 연결을 닫는다.
func (s *NASASerialTransport) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.open || s.conn == nil {
		return nil
	}

	err := s.conn.Close()
	s.conn = nil
	s.open = false
	return err
}

// Send 는 시리얼 포트로 데이터를 전송한다.
func (s *NASASerialTransport) Send(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.open || s.conn == nil {
		return ErrTransportNotConnected
	}

	_, err := s.conn.Write(data)
	if err != nil && isConnectionError(err) {
		s.open = false
		s.conn = nil
	}
	return err
}

// Receive 는 시리얼 포트에서 데이터를 수신한다.
func (s *NASASerialTransport) Receive(buf []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.open || s.conn == nil {
		return 0, ErrTransportNotConnected
	}

	n, err := s.conn.Read(buf)
	if err != nil && isConnectionError(err) {
		s.open = false
		s.conn = nil
	}
	return n, err
}

// Available 은 시리얼 포트가 열려 있는지 반환한다.
func (s *NASASerialTransport) Available() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.open
}

// ---------------------------------------------------------------------------
// NASATCPTransport (REQ-02-03)
// ---------------------------------------------------------------------------

// NASATCPTransport 는 TCP 소켓 기반 NASA 트랜스포트이다.
type NASATCPTransport struct {
	address        string
	connectTimeout time.Duration
	readTimeout    time.Duration
	conn           net.Conn
	mu             sync.Mutex
	open           bool
}

// Open 은 TCP 연결을 설정한다.
func (t *NASATCPTransport) Open() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	conn, err := net.DialTimeout("tcp", t.address, t.connectTimeout)
	if err != nil {
		return err
	}

	t.conn = conn
	t.open = true
	return nil
}

// Close 는 TCP 연결을 닫는다.
func (t *NASATCPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.open || t.conn == nil {
		return nil
	}

	err := t.conn.Close()
	t.conn = nil
	t.open = false
	return err
}

// Send 는 TCP 소켓으로 데이터를 전송한다.
func (t *NASATCPTransport) Send(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.open || t.conn == nil {
		return ErrTransportNotConnected
	}

	_, err := t.conn.Write(data)
	if err != nil && isConnectionError(err) {
		t.open = false
		t.conn = nil
	}
	return err
}

// Receive 는 TCP 소켓에서 데이터를 수신한다.
func (t *NASATCPTransport) Receive(buf []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.open || t.conn == nil {
		return 0, ErrTransportNotConnected
	}

	if t.readTimeout > 0 {
		t.conn.SetReadDeadline(time.Now().Add(t.readTimeout))
	}

	n, err := t.conn.Read(buf)
	if err != nil && isConnectionError(err) {
		t.open = false
		t.conn = nil
	}
	return n, err
}

// Available 은 TCP 연결이 열려 있는지 반환한다.
func (t *NASATCPTransport) Available() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.open
}

// ---------------------------------------------------------------------------
// Transport Factory (REQ-02-04)
// ---------------------------------------------------------------------------

// NewNASATransport 는 transportType 에 따라 적절한 NASATransport 를 생성한다.
// "serial" → NASASerialTransport, "tcp" → NASATCPTransport.
func NewNASATransport(transportType string, opts map[string]any) (NASATransport, error) {
	switch transportType {
	case "serial":
		st, err := newSerialTransport(opts)
		if err != nil {
			return nil, err
		}
		return st, nil
	case "tcp":
		tt, err := newTCPTransport(opts)
		if err != nil {
			return nil, err
		}
		return tt, nil
	default:
		return nil, ErrInvalidTransportType
	}
}

// newSerialTransport 는 opts 에서 시리얼 설정을 파싱하여 NASASerialTransport 를 생성한다.
func newSerialTransport(opts map[string]any) (*NASASerialTransport, error) {
	port, _ := optString(opts, "serial_port")
	if port == "" {
		return nil, ErrSerialPortRequired
	}

	return &NASASerialTransport{
		port:     port,
		baudRate: optInt(opts, "baud_rate", 9600),
		dataBits: optInt(opts, "data_bits", 8),
		stopBits: optInt(opts, "stop_bits", 1),
		parity:   optStringDefault(opts, "parity", "even"),
	}, nil
}

// newTCPTransport 는 opts 에서 TCP 설정을 파싱하여 NASATCPTransport 를 생성한다.
func newTCPTransport(opts map[string]any) (*NASATCPTransport, error) {
	address, _ := optString(opts, "tcp_address")
	if address == "" {
		return nil, ErrTCPAddressRequired
	}

	connectTimeout := optDuration(opts, "connect_timeout", 5*time.Second)
	readTimeout := optDuration(opts, "read_timeout", 3*time.Second)

	return &NASATCPTransport{
		address:        address,
		connectTimeout: connectTimeout,
		readTimeout:    readTimeout,
	}, nil
}

// ---------------------------------------------------------------------------
// opts 헬퍼 함수
// ---------------------------------------------------------------------------

// optString 은 opts 에서 문자열 값을 추출한다.
func optString(opts map[string]any, key string) (string, bool) {
	if opts == nil {
		return "", false
	}
	v, ok := opts[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// optStringDefault 는 opts 에서 문자열 값을 추출하고, 없으면 기본값을 반환한다.
func optStringDefault(opts map[string]any, key, defaultVal string) string {
	s, ok := optString(opts, key)
	if !ok || s == "" {
		return defaultVal
	}
	return s
}

// optInt 는 opts 에서 정수 값을 추출한다.
// YAML/JSON 언마샬링 시 float64 로 올 수 있으므로 config.go 의 toInt() 를 활용한다.
// 키가 없거나 지원하지 않는 타입이면 defaultVal 을 반환한다.
func optInt(opts map[string]any, key string, defaultVal int) int {
	if opts == nil {
		return defaultVal
	}
	v, ok := opts[key]
	if !ok {
		return defaultVal
	}
	// toInt 는 int, float64 → int 변환. 미지원 타입은 0 반환.
	// 키가 존재하면 toInt 결과를 그대로 사용 (0 도 유효한 사용자 지정 값)
	switch v.(type) {
	case int, float64:
		return toInt(v)
	default:
		return defaultVal
	}
}

// optDuration 은 opts 에서 duration 문자열을 파싱한다.
func optDuration(opts map[string]any, key string, defaultVal time.Duration) time.Duration {
	s, ok := optString(opts, key)
	if !ok || s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}
