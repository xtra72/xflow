package century

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// transport_tcp.go — Century TCP transport (REQ-CENTURY-029, REQ-CENTURY-030)
//
// 두 변종을 제공한다:
//   - tcpClientTransport: net.Dialer.DialContext 로 원격 컨버터에 능동 접속
//   - tcpServerTransport: net.Listen 으로 단일 활성 연결을 accept
//
// 양 변종 모두 io.ReadWriteCloser 를 만족하지만, Write 메서드는 ErrTransportPassiveOnly
// 를 반환하여 AC-B9 invariant (transport.Write 0회) 를 type-level 에서 강제한다 (AC-G8).
// captureLoop 는 절대 Write 를 호출하지 않으나, 방어적 구현으로 wrapper 자체가
// 송신을 거부하므로 회귀가 발생해도 회선에 byte 가 흘러가지 않는다.
// ---------------------------------------------------------------------------

// openTransport 는 cfg.TransportType 에 따라 serial / tcp-client / tcp-server 트랜스포트를 연다.
// ctx 는 tcp-* 변종의 dial / listen 단계에서 cancel 처리에 사용된다.
//
// 에러 시 io.ReadWriteCloser 는 명시적으로 nil 로 반환된다 (typed-nil 회피).
func openTransport(ctx context.Context, cfg Hvacr01Config) (io.ReadWriteCloser, error) {
	switch cfg.TransportType {
	case "serial":
		t, err := openSerialTransport(cfg)
		if err != nil {
			return nil, err
		}
		return t, nil
	case "tcp-client":
		t, err := openTCPClient(ctx, cfg)
		if err != nil {
			return nil, err
		}
		return t, nil
	case "tcp-server":
		t, err := openTCPServer(ctx, cfg)
		if err != nil {
			return nil, err
		}
		return t, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownTransportType, cfg.TransportType)
	}
}

// ---------------------------------------------------------------------------
// tcpClientTransport
// ---------------------------------------------------------------------------

// tcpClientTransport 는 RX-only TCP 클라이언트 wrapper 이다.
//
// io.ReadWriteCloser 를 만족하지만 Write 는 ErrTransportPassiveOnly 를 반환한다.
// 매 Read 직전에 SetReadDeadline 을 갱신하여 readTimeout 을 강제한다 (REQ-CENTURY-029).
type tcpClientTransport struct {
	conn        net.Conn
	readTimeout time.Duration

	closed atomic.Bool
}

// openTCPClient 는 net.Dialer.DialContext 를 사용해 원격 호스트에 능동 접속한다.
//
// (lg_hvacr01 의 lgapTCPClientTransport.Open() 의 net.DialTimeout 패턴을 ctx-aware 로 개선 —
// agent.Stop 의 context cancel 이 dial 도중 즉시 적용된다.)
func openTCPClient(ctx context.Context, cfg Hvacr01Config) (*tcpClientTransport, error) {
	if cfg.TCPHost == "" {
		return nil, ErrHvacr01TCPHostRequired
	}
	if cfg.TCPPort < 1 || cfg.TCPPort > 65535 {
		return nil, fmt.Errorf("%w: %d", ErrHvacr01TCPPortRequired, cfg.TCPPort)
	}
	timeout := cfg.TCPConnectTimeout
	if timeout <= 0 {
		timeout = DefaultTCPConnectTimeout
	}
	d := &net.Dialer{Timeout: timeout}
	addr := net.JoinHostPort(cfg.TCPHost, strconv.Itoa(cfg.TCPPort))
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHvacr01TCPDialFailed, err)
	}
	rt := cfg.TCPReadTimeout
	if rt <= 0 {
		rt = DefaultTCPReadTimeout
	}
	return &tcpClientTransport{conn: conn, readTimeout: rt}, nil
}

// Read implements io.Reader. SetReadDeadline is refreshed on every call so
// the TCP read timeout is enforced per-read regardless of how often the
// underlying capture loop calls Read.
func (t *tcpClientTransport) Read(p []byte) (int, error) {
	if t.closed.Load() {
		return 0, io.EOF
	}
	if t.readTimeout > 0 {
		_ = t.conn.SetReadDeadline(time.Now().Add(t.readTimeout))
	}
	return t.conn.Read(p)
}

// Write enforces AC-B9 at the type level — the agent must never transmit on a
// passive sniff. Even a defensive write attempt is refused with a sentinel error.
func (t *tcpClientTransport) Write(p []byte) (int, error) {
	return 0, ErrTransportPassiveOnly
}

// Close releases the underlying connection. Idempotent.
func (t *tcpClientTransport) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}
	return t.conn.Close()
}

// Available indicates whether the connection is still considered usable.
// Mirrors lg.lgapTCPClientTransport.Available semantics for parity.
func (t *tcpClientTransport) Available() bool {
	return !t.closed.Load()
}

// ---------------------------------------------------------------------------
// tcpServerTransport
// ---------------------------------------------------------------------------

// tcpServerTransport 는 TCP 서버 wrapper 이다. 단일 활성 연결 정책을 따른다 (A11).
//
//   - net.Listen 으로 endpoint 를 열고 acceptLoop goroutine 을 시작한다.
//   - 첫 연결은 활성 conn 으로 채택되고, 활성 연결이 살아있는 동안 추가 accept 결과는
//     즉시 close 된다 (AC-G6).
//   - 활성 연결이 종료되면 다음 accept 결과가 자동으로 활성으로 승격된다 (AC-G6 두번째 시나리오).
//   - Write 는 항상 ErrTransportPassiveOnly 반환.
type tcpServerTransport struct {
	listener net.Listener
	logger   serverLogger

	mu     sync.Mutex
	active net.Conn

	closed atomic.Bool
	doneCh chan struct{}

	readTimeout time.Duration

	activeConnections atomic.Uint64
	rejectedSecondary atomic.Uint64
}

// serverLogger is a narrow interface so tcpServerTransport can log INFO events
// (secondary connection rejected) without coupling to slog directly in tests.
// *slog.Logger satisfies this implicitly via its Info(msg, args...) method.
type serverLogger interface {
	Info(msg string, args ...any)
}

// noopLogger is a fallback when no logger is wired (unit tests).
type noopLogger struct{}

func (noopLogger) Info(string, ...any) {}

// Compile-time interface check (keeps the noop usable as a fallback).
var _ serverLogger = noopLogger{}

// openTCPServer 는 net.Listen 으로 endpoint 를 열고 accept loop 를 시작한다.
//
// ctx 는 listen 단계의 cancel 처리에 사용된다 (현재 net.Listen 자체는 즉시 반환되므로
// 주로 향후 확장 hook 으로 보관된다; accept loop 의 정지는 Close 또는 ctx.Done 으로 처리).
func openTCPServer(ctx context.Context, cfg Hvacr01Config) (*tcpServerTransport, error) {
	if cfg.TCPPort < 1 || cfg.TCPPort > 65535 {
		return nil, fmt.Errorf("%w: %d", ErrHvacr01TCPPortRequired, cfg.TCPPort)
	}
	host := cfg.TCPHost
	if host == "" {
		host = "0.0.0.0"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(cfg.TCPPort))

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHvacr01TCPListenFailed, err)
	}
	rt := cfg.TCPReadTimeout
	if rt <= 0 {
		rt = DefaultTCPReadTimeout
	}
	s := &tcpServerTransport{
		listener:    ln,
		logger:      noopLogger{},
		doneCh:      make(chan struct{}),
		readTimeout: rt,
	}
	go s.acceptLoop()
	return s, nil
}

// SetLogger registers a structured logger for INFO events (e.g. secondary rejections).
func (s *tcpServerTransport) SetLogger(l serverLogger) {
	if l != nil {
		s.logger = l
	}
}

// Addr returns the bound address (test helper — listener.Addr can be used to learn
// the OS-assigned port when configured with port 0).
func (s *tcpServerTransport) Addr() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// acceptLoop 는 listener 가 close 될 때까지 새 연결을 받아들인다.
//
// 활성 연결이 살아있는 동안 추가 accept 결과는 즉시 close + rejectedSecondary++ (AC-G6).
// 활성 연결이 종료되면 자동으로 다음 accept 결과가 활성으로 승격된다.
func (s *tcpServerTransport) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// listener closed → exit gracefully.
			if s.closed.Load() {
				return
			}
			// Temporary errors: keep going.
			var nerr net.Error
			if errors.As(err, &nerr) && nerr.Timeout() {
				continue
			}
			// On a persistent error treat the listener as dead.
			return
		}

		s.mu.Lock()
		if s.active != nil {
			// Single-active policy: reject secondary connections immediately (AC-G6).
			peer := conn.RemoteAddr().String()
			_ = conn.Close()
			s.rejectedSecondary.Add(1)
			s.mu.Unlock()
			s.logger.Info("century_hvacr01: tcp-server: rejected secondary connection", "peer", peer)
			continue
		}
		s.active = conn
		s.activeConnections.Store(1)
		s.mu.Unlock()
	}
}

// Read pulls from the current active connection. If no client is connected it
// blocks until one accepts. Close releases the wait.
func (s *tcpServerTransport) Read(p []byte) (int, error) {
	for {
		if s.closed.Load() {
			return 0, io.EOF
		}
		s.mu.Lock()
		c := s.active
		s.mu.Unlock()
		if c == nil {
			// Wait briefly for an accept; pull again. We avoid a condition variable
			// here because the captureLoop already polls with a buffered scanner.
			select {
			case <-s.doneCh:
				return 0, io.EOF
			case <-time.After(20 * time.Millisecond):
				continue
			}
		}
		if s.readTimeout > 0 {
			_ = c.SetReadDeadline(time.Now().Add(s.readTimeout))
		}
		n, err := c.Read(p)
		if err != nil {
			// Active connection died — clear it so the next accept can take over.
			s.clearActive(c)
			if n > 0 {
				return n, nil
			}
			// EOF / timeout / network errors: surface to caller which decides how
			// to handle (frame scanner sees io.EOF / timeout and tears down).
			return 0, err
		}
		return n, nil
	}
}

// clearActive drops the given connection if it is still the active one.
func (s *tcpServerTransport) clearActive(c net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == c {
		_ = s.active.Close()
		s.active = nil
		s.activeConnections.Store(0)
	}
}

// Write rejects every call to preserve AC-B9 (passive sniff invariant).
func (s *tcpServerTransport) Write(p []byte) (int, error) {
	return 0, ErrTransportPassiveOnly
}

// Close shuts down the listener and any active client connection.
func (s *tcpServerTransport) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(s.doneCh)
	var firstErr error
	if err := s.listener.Close(); err != nil {
		firstErr = err
	}
	s.mu.Lock()
	if s.active != nil {
		if err := s.active.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		s.active = nil
	}
	s.activeConnections.Store(0)
	s.mu.Unlock()
	return firstErr
}

// ActiveConnections returns the current active-connection count (0 or 1 in single-active mode).
func (s *tcpServerTransport) ActiveConnections() uint64 {
	return s.activeConnections.Load()
}

// RejectedSecondary returns the cumulative count of secondary connections refused.
func (s *tcpServerTransport) RejectedSecondary() uint64 {
	return s.rejectedSecondary.Load()
}
