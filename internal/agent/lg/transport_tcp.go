package lg

import (
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// lgapTCPClientTransport: TCP 클라이언트 기반 LGAP 트랜스포트
// ---------------------------------------------------------------------------

// lgapTCPClientTransport 는 TCP 클라이언트 연결 기반 LGAP 트랜스포트이다.
// 원격 TCP 서버(예: 시리얼-TCP 컨버터)에 연결하여 LGAP 프로토콜 통신을 수행한다.
type lgapTCPClientTransport struct {
	host           string        // 연결 대상 호스트 주소
	port           int           // 연결 대상 포트 번호
	connectTimeout time.Duration // 연결 타임아웃
	readTimeout    time.Duration // 읽기 타임아웃
	writeTimeout   time.Duration // 쓰기 타임아웃
	conn           net.Conn      // TCP 연결
	mu             sync.Mutex    // conn 접근 보호
	open           atomic.Bool   // 연결 상태 (Available 에서 lock-free 조회)
}

// newLGAPTCPClientTransport 는 설정으로부터 lgapTCPClientTransport 를 생성한다.
func newLGAPTCPClientTransport(cfg LGCPConfig) *lgapTCPClientTransport {
	return &lgapTCPClientTransport{
		host:           cfg.TCPHost,
		port:           cfg.TCPPort,
		connectTimeout: cfg.TCPConnectTimeout,
		readTimeout:    cfg.TCPReadTimeout,
		writeTimeout:   cfg.TCPWriteTimeout,
	}
}

// Open 은 TCP 서버에 연결을 수립한다.
func (t *lgapTCPClientTransport) Open() error {
	addr := net.JoinHostPort(t.host, strconv.Itoa(t.port))
	conn, err := net.DialTimeout("tcp", addr, t.connectTimeout)
	if err != nil {
		return err
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.conn = conn
	t.open.Store(true)
	return nil
}

// Close 는 TCP 연결을 닫는다.
func (t *lgapTCPClientTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.open.Load() || t.conn == nil {
		return nil
	}

	err := t.conn.Close()
	t.conn = nil
	t.open.Store(false)
	return err
}

// Send 는 TCP 연결로 데이터를 전송한다.
func (t *lgapTCPClientTransport) Send(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.open.Load() || t.conn == nil {
		return ErrTransportNotConnected
	}

	if t.writeTimeout > 0 {
		_ = t.conn.SetWriteDeadline(time.Now().Add(t.writeTimeout))
	}
	_, err := t.conn.Write(data)
	if err != nil && isLGAPConnectionError(err) {
		t.open.Store(false)
	}
	return err
}

// Receive 는 TCP 연결에서 데이터를 수신한다.
func (t *lgapTCPClientTransport) Receive(buf []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.open.Load() || t.conn == nil {
		return 0, ErrTransportNotConnected
	}

	if t.readTimeout > 0 {
		_ = t.conn.SetReadDeadline(time.Now().Add(t.readTimeout))
	}
	n, err := t.conn.Read(buf)
	if err != nil && isLGAPConnectionError(err) {
		t.open.Store(false)
	}
	return n, err
}

// Write 는 TCP 연결로 데이터를 전송하고 전송된 바이트 수를 반환한다.
func (t *lgapTCPClientTransport) Write(data []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.open.Load() || t.conn == nil {
		return 0, ErrTransportNotConnected
	}

	if t.writeTimeout > 0 {
		_ = t.conn.SetWriteDeadline(time.Now().Add(t.writeTimeout))
	}
	n, err := t.conn.Write(data)
	if err != nil && isLGAPConnectionError(err) {
		t.open.Store(false)
	}
	return n, err
}

// Available 은 TCP 연결이 열려 있는지 반환한다.
// lock-free: atomic 으로 읽어 I/O 블로킹에 영향받지 않는다.
func (t *lgapTCPClientTransport) Available() bool {
	return t.open.Load()
}

// ---------------------------------------------------------------------------
// lgapTCPServerTransport: TCP 서버 기반 LGAP 트랜스포트
// ---------------------------------------------------------------------------

// lgapTCPServerTransport 는 TCP 서버 리스너 기반 LGAP 트랜스포트이다.
// 단일 클라이언트 연결을 수락하여 LGAP 프로토콜 통신을 수행한다.
// 새 연결이 수락되면 기존 연결을 교체한다 (단일 클라이언트 모드).
type lgapTCPServerTransport struct {
	host         string        // 리스닝 호스트 주소
	port         int           // 리스닝 포트 번호
	readTimeout  time.Duration // 읽기 타임아웃
	writeTimeout time.Duration // 쓰기 타임아웃
	listener     net.Listener  // TCP 리스너
	conn         net.Conn      // 현재 클라이언트 연결
	mu           sync.Mutex    // conn/listener 접근 보호
	open         atomic.Bool   // 연결 상태 (Available 에서 lock-free 조회)
	done         chan struct{} // acceptLoop 종료 시그널
}

// newLGAPTCPServerTransport 는 설정으로부터 lgapTCPServerTransport 를 생성한다.
func newLGAPTCPServerTransport(cfg LGCPConfig) *lgapTCPServerTransport {
	return &lgapTCPServerTransport{
		host:         cfg.TCPHost,
		port:         cfg.TCPPort,
		readTimeout:  cfg.TCPReadTimeout,
		writeTimeout: cfg.TCPWriteTimeout,
	}
}

// Open 은 TCP 리스너를 시작하고 acceptLoop 고루틴을 실행한다.
func (s *lgapTCPServerTransport) Open() error {
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.listener = ln
	s.done = make(chan struct{})
	s.mu.Unlock()

	go s.acceptLoop()
	return nil
}

// acceptLoop 는 새 클라이언트 연결을 지속적으로 수락한다.
// 새 연결이 들어오면 기존 연결을 닫고 교체한다 (단일 클라이언트 모드).
func (s *lgapTCPServerTransport) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// 리스너가 닫힌 경우 종료
			select {
			case <-s.done:
				return
			default:
			}
			// 임시 에러인 경우 계속 시도
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}

		s.mu.Lock()
		// 기존 연결이 있으면 닫고 교체
		if s.conn != nil {
			_ = s.conn.Close()
		}
		s.conn = conn
		s.open.Store(true)
		s.mu.Unlock()
	}
}

// Close 는 TCP 리스너와 현재 연결을 모두 닫는다.
func (s *lgapTCPServerTransport) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// done 채널 닫아 acceptLoop 종료 시그널 전송
	if s.done != nil {
		select {
		case <-s.done:
			// 이미 닫힘
		default:
			close(s.done)
		}
	}

	var firstErr error

	// 리스너 닫기 (acceptLoop 의 Accept() 를 언블록)
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			firstErr = err
		}
		s.listener = nil
	}

	// 현재 클라이언트 연결 닫기
	if s.conn != nil {
		if err := s.conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.conn = nil
	}

	s.open.Store(false)
	return firstErr
}

// Send 는 현재 클라이언트 연결로 데이터를 전송한다.
func (s *lgapTCPServerTransport) Send(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.open.Load() || s.conn == nil {
		return ErrTransportNotConnected
	}

	if s.writeTimeout > 0 {
		_ = s.conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}
	_, err := s.conn.Write(data)
	if err != nil && isLGAPConnectionError(err) {
		s.open.Store(false)
	}
	return err
}

// Receive 는 현재 클라이언트 연결에서 데이터를 수신한다.
func (s *lgapTCPServerTransport) Receive(buf []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.open.Load() || s.conn == nil {
		return 0, ErrTransportNotConnected
	}

	if s.readTimeout > 0 {
		_ = s.conn.SetReadDeadline(time.Now().Add(s.readTimeout))
	}
	n, err := s.conn.Read(buf)
	if err != nil && isLGAPConnectionError(err) {
		s.open.Store(false)
	}
	return n, err
}

// Write 는 현재 클라이언트 연결로 데이터를 전송하고 전송된 바이트 수를 반환한다.
func (s *lgapTCPServerTransport) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.open.Load() || s.conn == nil {
		return 0, ErrTransportNotConnected
	}

	if s.writeTimeout > 0 {
		_ = s.conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
	}
	n, err := s.conn.Write(data)
	if err != nil && isLGAPConnectionError(err) {
		s.open.Store(false)
	}
	return n, err
}

// Available 은 클라이언트가 연결되어 있는지 반환한다.
// lock-free: atomic 으로 읽어 I/O 블로킹에 영향받지 않는다.
func (s *lgapTCPServerTransport) Available() bool {
	return s.open.Load()
}
