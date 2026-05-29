package samsung

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------
// NasaTransport 인터페이스 (REQ-02-01)
// ---------------------------------------------------------------------------

// NasaTransport 는 Samsung NASA HVAC 프로토콜의 트랜스포트 레이어 인터페이스이다.
type NasaTransport interface {
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
// Preamble: RS-485 버스 동기화용 0x55 바이트 시퀀스
// ---------------------------------------------------------------------------

const preambleLen = 100

// preamble 은 RS-485 UART 동기화를 위해 프레임 전송 전에 붙이는 0x55 바이트열이다.
var preamble [preambleLen]byte

func init() {
	for i := range preamble {
		preamble[i] = 0x55
	}
}

// prependPreamble 은 프레임 앞에 preamble 을 붙인 버퍼를 반환한다.
func prependPreamble(frame []byte) []byte {
	buf := make([]byte, preambleLen+len(frame))
	copy(buf, preamble[:])
	copy(buf[preambleLen:], frame)
	return buf
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
	// ENXIO: "device not configured" — 시리얼 디바이스 분리 (os.PathError 래핑)
	if errors.Is(err, syscall.ENXIO) {
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
// NasaSerialTransport (REQ-02-02)
// ---------------------------------------------------------------------------

// NasaSerialTransport 는 시리얼 포트 기반 NASA 트랜스포트이다.
type NasaSerialTransport struct {
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
func (s *NasaSerialTransport) Open() error {
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
func (s *NasaSerialTransport) Close() error {
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
// RS-485 버스 동기화를 위해 프레임 앞에 0x55 preamble 100바이트를 붙인다.
func (s *NasaSerialTransport) Send(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.open || s.conn == nil {
		return ErrTransportNotConnected
	}

	_, err := s.conn.Write(prependPreamble(data))
	if err != nil && isConnectionError(err) {
		s.open = false
		s.conn = nil
	}
	return err
}

// Receive 는 시리얼 포트에서 데이터를 수신한다.
func (s *NasaSerialTransport) Receive(buf []byte) (int, error) {
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
func (s *NasaSerialTransport) Available() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.open
}

// ---------------------------------------------------------------------------
// NasaTCPTransport (REQ-02-03)
// ---------------------------------------------------------------------------

// NasaTCPTransport 는 TCP 소켓 기반 NASA 트랜스포트이다.
type NasaTCPTransport struct {
	address        string
	connectTimeout time.Duration
	readTimeout    time.Duration
	conn           net.Conn
	mu             sync.Mutex
	open           bool
}

// Open 은 TCP 연결을 설정한다.
func (t *NasaTCPTransport) Open() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	conn, err := net.DialTimeout("tcp", t.address, t.connectTimeout)
	if err != nil {
		return err
	}

	// TCP_NODELAY 활성화 (Nagle 알고리즘 비활성화).
	// 2026-05-14 hotfix: serial-to-ethernet 어댑터(EW11 등) 경유 시 Nagle 이 작은
	// 제어 프레임을 지연/병합하여 RS-485 버스 타이밍을 깨뜨리면 제어 명령이 디바이스에
	// 도달하지 못한다. 모니터링(수신)은 영향이 없으나 timing-sensitive 한 제어 명령은
	// 즉시 전송되어야 하므로 NoDelay 를 켠다.
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}

	t.conn = conn
	t.open = true
	return nil
}

// Close 는 TCP 연결을 닫는다.
func (t *NasaTCPTransport) Close() error {
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
// RS-485 변환기(EW11 등) 경유 시에도 preamble 이 필요하므로 동일하게 적용한다.
func (t *NasaTCPTransport) Send(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.open || t.conn == nil {
		return ErrTransportNotConnected
	}

	_, err := t.conn.Write(prependPreamble(data))
	if err != nil && isConnectionError(err) {
		t.open = false
		t.conn = nil
	}
	return err
}

// Receive 는 TCP 소켓에서 데이터를 수신한다.
func (t *NasaTCPTransport) Receive(buf []byte) (int, error) {
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
func (t *NasaTCPTransport) Available() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.open
}

// ---------------------------------------------------------------------------
// NasaTCPServerTransport (REQ-02-06, 2026-05-29)
// ---------------------------------------------------------------------------

// NasaTCPServerTransport 는 TCP 서버 리스너 기반 NASA 트랜스포트이다 (LG/Century 통일).
//
// 단일 활성 연결 정책 (LG lgapTCPServerTransport 패턴):
//   - net.Listen 으로 host:port 에 바인드 후 acceptLoop goroutine 시작.
//   - 첫 클라이언트 접속을 활성 conn 으로 채택.
//   - 활성 conn 이 살아있는 동안 추가 accept 결과는 기존 conn 을 close 하고 새 conn 으로 교체
//     (제어 디바이스가 재접속하는 시나리오 — 사용자 가시 동작).
//   - 활성 conn 종료 시 다음 accept 결과가 자동으로 활성으로 승격.
//
// Send 는 활성 conn 으로 preamble + frame 전송 (RS-485 변환기 통과 시에도 preamble 필요).
// Receive 는 활성 conn 에서 read deadline 적용 후 수신; 클라이언트 미접속 시
// 짧게 대기 후 transient 에러 반환 (receiveLoop 의 Available()==true 체크에 의해 폴링 지속).
type NasaTCPServerTransport struct {
	host        string
	port        int
	readTimeout time.Duration

	mu       sync.Mutex
	listener net.Listener
	conn     net.Conn
	open     bool // listener 가 살아있고 (또는 active conn 보유) Available 신호 — Available 은 mutex 보호
	doneCh   chan struct{}

	logger serverInfoLogger // INFO 이벤트 (연결 교체 등) 용 — 미연결 시 noopServerLogger
}

// serverInfoLogger 는 NasaTCPServerTransport 가 사용하는 narrow logger 인터페이스이다.
// *slog.Logger 가 암시적으로 만족한다. agent 가 SetLogger 로 wire 한다 (선택).
type serverInfoLogger interface {
	Info(msg string, args ...any)
}

// noopServerLogger 는 logger 미설정 시 폴백이다.
type noopServerLogger struct{}

func (noopServerLogger) Info(string, ...any) {}

// errNoActiveClient 는 tcp-server 의 Receive 가 활성 conn 부재로 잠시 대기 후
// 반환하는 transient 에러이다. isConnectionError 에서 connection error 로 분류되지 않으므로
// receiveLoop 는 이를 일시적 에러로 간주하고 폴링을 계속한다.
var errNoActiveClient = errors.New("samsung_hvacr01: tcp-server: no client connected")

// newTCPServerTransport 는 opts 에서 TCP 서버 설정을 파싱하여 NasaTCPServerTransport 를 생성한다.
// tcp_host 미지정 시 "0.0.0.0" (모든 인터페이스 바인드) 을 기본값으로 사용한다.
func newTCPServerTransport(opts map[string]any) (*NasaTCPServerTransport, error) {
	host, _ := optString(opts, "tcp_host")
	if host == "" {
		host = "0.0.0.0"
	}

	port := optInt(opts, "tcp_port", 0)
	if port == 0 {
		return nil, ErrTCPPortRequired
	}

	readTimeout := optDuration(opts, "read_timeout", 3*time.Second)

	return &NasaTCPServerTransport{
		host:        host,
		port:        port,
		readTimeout: readTimeout,
		logger:      noopServerLogger{},
	}, nil
}

// Addr 는 바인드된 주소를 반환한다 (테스트 헬퍼 — port=0 으로 OS 할당 시 실제 포트 조회).
func (s *NasaTCPServerTransport) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// SetLogger 는 INFO 이벤트용 logger 를 등록한다 (선택).
func (s *NasaTCPServerTransport) SetLogger(l serverInfoLogger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l != nil {
		s.logger = l
	}
}

// Open 은 TCP 리스너를 시작하고 acceptLoop 고루틴을 실행한다.
func (s *NasaTCPServerTransport) Open() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.open {
		return nil
	}

	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTCPListenFailed, err)
	}

	s.listener = ln
	s.doneCh = make(chan struct{})
	s.open = true

	go s.acceptLoop(s.doneCh)
	return nil
}

// acceptLoop 는 listener 가 close 될 때까지 새 연결을 수락한다.
// 새 연결은 기존 활성 conn 을 close 하고 교체한다 (LG lgapTCPServerTransport 패턴).
func (s *NasaTCPServerTransport) acceptLoop(doneCh chan struct{}) {
	for {
		s.mu.Lock()
		ln := s.listener
		s.mu.Unlock()
		if ln == nil {
			return
		}

		conn, err := ln.Accept()
		if err != nil {
			// listener 가 close 됨 → 정상 종료.
			select {
			case <-doneCh:
				return
			default:
			}
			// 일시적 (Timeout) 에러는 계속 시도.
			var nerr net.Error
			if errors.As(err, &nerr) && nerr.Timeout() {
				continue
			}
			// 영구 에러: accept loop 종료.
			s.mu.Lock()
			s.open = false
			s.mu.Unlock()
			return
		}

		// TCP_NODELAY: 시리얼-Ethernet 어댑터 경유 시 timing-sensitive 한
		// 제어 frame 이 Nagle 로 지연되지 않도록 한다 (NasaTCPTransport 와 동일 정책).
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.SetNoDelay(true)
		}

		s.mu.Lock()
		if s.conn != nil {
			peer := s.conn.RemoteAddr().String()
			_ = s.conn.Close()
			s.logger.Info("samsung_hvacr01: tcp-server: 기존 연결 교체",
				"old_peer", peer,
				"new_peer", conn.RemoteAddr().String(),
			)
		}
		s.conn = conn
		s.mu.Unlock()
	}
}

// Close 는 리스너와 활성 conn 을 모두 닫는다.
func (s *NasaTCPServerTransport) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.open {
		return nil
	}

	// doneCh 닫아 accept loop 종료 시그널.
	if s.doneCh != nil {
		select {
		case <-s.doneCh:
		default:
			close(s.doneCh)
		}
	}

	var firstErr error
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			firstErr = err
		}
		s.listener = nil
	}
	if s.conn != nil {
		if err := s.conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.conn = nil
	}
	s.open = false
	return firstErr
}

// Send 는 활성 conn 으로 preamble + data 를 전송한다.
// 활성 conn 이 없으면 ErrTransportNotConnected 반환.
func (s *NasaTCPServerTransport) Send(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.open || s.conn == nil {
		return ErrTransportNotConnected
	}

	_, err := s.conn.Write(prependPreamble(data))
	if err != nil && isConnectionError(err) {
		// conn 만 정리 — 리스너는 그대로 유지 (다음 클라이언트 대기).
		_ = s.conn.Close()
		s.conn = nil
	}
	return err
}

// Receive 는 활성 conn 에서 데이터를 수신한다.
// 활성 conn 이 없으면 짧게 대기 후 errNoActiveClient 를 반환한다.
// 연결 에러 시 conn 만 정리하고 (리스너는 유지) 에러를 반환한다.
func (s *NasaTCPServerTransport) Receive(buf []byte) (int, error) {
	s.mu.Lock()
	if !s.open {
		s.mu.Unlock()
		return 0, ErrTransportNotConnected
	}
	c := s.conn
	doneCh := s.doneCh
	rt := s.readTimeout
	s.mu.Unlock()

	if c == nil {
		// 클라이언트 미연결 — 짧게 대기 후 transient 에러 반환.
		// receiveLoop 는 Available()==true 이면 transient 로 간주하고 폴링을 계속한다.
		select {
		case <-doneCh:
			return 0, io.EOF
		case <-time.After(20 * time.Millisecond):
		}
		return 0, errNoActiveClient
	}

	if rt > 0 {
		_ = c.SetReadDeadline(time.Now().Add(rt))
	}

	n, err := c.Read(buf)
	if err != nil && isConnectionError(err) {
		// 활성 conn 만 닫는다 — 리스너 (s.open) 는 유지.
		s.mu.Lock()
		if s.conn == c {
			_ = s.conn.Close()
			s.conn = nil
		}
		s.mu.Unlock()
	}
	return n, err
}

// Available 은 리스너가 살아있는지 반환한다.
// tcp-server 는 클라이언트 미연결 상태에서도 listener 가 살아있으면 true 를 반환하여
// receiveLoop 가 reconnectLoop 로 진입하지 않도록 한다 (수동 대기 모드).
func (s *NasaTCPServerTransport) Available() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.open
}

// ---------------------------------------------------------------------------
// Transport Factory (REQ-02-04)
// ---------------------------------------------------------------------------

// NewNasaTransport 는 transportType 에 따라 적절한 NasaTransport 를 생성한다.
//
//   - "serial"     → NasaSerialTransport (RS-485 직결)
//   - "tcp-client" → NasaTCPTransport (외부 컨버터/서버에 능동 접속)
//   - "tcp-server" → NasaTCPServerTransport (수신 대기 후 클라이언트 접속 수락)
//
// 2026-05-29 breaking: "tcp" 값은 더 이상 허용되지 않는다 (LG/Century 통일).
// 이전 설정은 "tcp-client" 로 명시적으로 마이그레이션해야 한다.
func NewNasaTransport(transportType string, opts map[string]any) (NasaTransport, error) {
	switch transportType {
	case "serial":
		st, err := newSerialTransport(opts)
		if err != nil {
			return nil, err
		}
		return st, nil
	case "tcp-client":
		tt, err := newTCPTransport(opts)
		if err != nil {
			return nil, err
		}
		return tt, nil
	case "tcp-server":
		ts, err := newTCPServerTransport(opts)
		if err != nil {
			return nil, err
		}
		return ts, nil
	case "tcp":
		// 2026-05-29 breaking change: explicit migration error.
		return nil, ErrDeprecatedTCPTransport
	default:
		return nil, ErrInvalidTransportType
	}
}

// newSerialTransport 는 opts 에서 시리얼 설정을 파싱하여 NasaSerialTransport 를 생성한다.
func newSerialTransport(opts map[string]any) (*NasaSerialTransport, error) {
	port, _ := optString(opts, "serial_port")
	if port == "" {
		return nil, ErrSerialPortRequired
	}

	return &NasaSerialTransport{
		port:     port,
		baudRate: optInt(opts, "baud_rate", 9600),
		dataBits: optInt(opts, "data_bits", 8),
		stopBits: optInt(opts, "stop_bits", 1),
		parity:   optStringDefault(opts, "parity", "even"),
	}, nil
}

// newTCPTransport 는 opts 에서 TCP 설정을 파싱하여 NasaTCPTransport 를 생성한다.
// tcp_host (string) + tcp_port (int) 두 키를 사용하며 (LG ICP-01/LGCP 패턴과 통일),
// 내부적으로 "host:port" 형식의 address 를 합성한다.
func newTCPTransport(opts map[string]any) (*NasaTCPTransport, error) {
	host, _ := optString(opts, "tcp_host")
	if host == "" {
		return nil, ErrTCPHostRequired
	}

	port := optInt(opts, "tcp_port", 0)
	if port == 0 {
		return nil, ErrTCPPortRequired
	}

	address := fmt.Sprintf("%s:%d", host, port)
	connectTimeout := optDuration(opts, "connect_timeout", 5*time.Second)
	readTimeout := optDuration(opts, "read_timeout", 3*time.Second)

	return &NasaTCPTransport{
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
