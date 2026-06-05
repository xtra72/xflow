// client.go 는 관리 클라이언트(mode=client)의 연결 라이프사이클을 구현한다
// (REQ-REMOTE-B01, B03, B04, spec §5.5 client).
//
// M1 범위:
//   - server_url 로 영속 WS dial(outbound, NAT 친화 — 결정 1).
//   - 연결 직후 hello 송신(instance_id + hostname + version 운반).
//   - HeartbeatInterval 주기 heartbeat 송신.
//   - 연결 종료 시 지수 백오프(+지터, cap) 재연결(socket/MQTT 패턴 준용).
//   - 재연결 성공 시 hello 재전송.
//   - ctx 취소 / Stop 으로 정상 종료(goroutine leak 방지).
//
// M2~M4 seam (본 파일에서 의도적으로 미구현):
//   - 등록 요청(register)·노드 토큰 핸드셰이크·재인증 → M2.
//   - 명령 수신 → 로컬 어댑터 적용 → command_result 반환 → M3.
//   - 인벤토리 스냅샷/델타 송신 → M4.
//   - 재연결 후 인벤토리 재동기화(REQ-B04) → M4 (현재는 hello 재전송 hook 만).
package remote

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// DefaultHeartbeatInterval 은 heartbeat 기본 주기이다(REQ-A05 안전 기본값).
	DefaultHeartbeatInterval = 30 * time.Second

	// DefaultReconnectInitial 은 재연결 백오프의 초기 간격이다.
	DefaultReconnectInitial = 1 * time.Second

	// DefaultReconnectMax 는 재연결 백오프의 상한이다(폭주 방지 — 위험표 "재연결 폭주").
	DefaultReconnectMax = 60 * time.Second
)

// Dialer 는 server_url 로 관리 WS 연결을 수립하는 추상화이다(테스트 주입용).
type Dialer interface {
	// Dial 은 url 로 연결을 수립한다. 실패 시 에러를 반환한다.
	Dial(ctx context.Context, url string) (Conn, error)
}

// DialerFunc 는 함수를 Dialer 로 어댑트한다.
type DialerFunc func(ctx context.Context, url string) (Conn, error)

// Dial 은 Dialer 를 구현한다.
func (f DialerFunc) Dial(ctx context.Context, url string) (Conn, error) { return f(ctx, url) }

// gorillaDialer 는 gorilla/websocket 기반 Dialer 이다(프로덕션 기본).
type gorillaDialer struct {
	dialer *websocket.Dialer
}

// NewGorillaDialer 는 gorilla/websocket 기반 Dialer 를 생성한다.
// TLS(wss) 는 url scheme 으로 결정된다(REQ-F01).
func NewGorillaDialer() Dialer {
	return &gorillaDialer{
		dialer: &websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		},
	}
}

func (d *gorillaDialer) Dial(ctx context.Context, url string) (Conn, error) {
	conn, _, err := d.dialer.DialContext(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	return newGorillaConn(conn), nil
}

// ClientConfig 는 관리 클라이언트의 동작 파라미터이다.
type ClientConfig struct {
	// ServerURL 은 관리 서버 WS 엔드포인트(wss URL)이다(REQ-A02).
	// 비어 있으면 클라이언트는 dial 하지 않는다(설정 오류).
	ServerURL string

	// InstanceID 는 이 노드의 영속 instance_id 이다(REQ-A03).
	InstanceID string

	// Hostname / Version 은 hello 에 실어 보내는 노드 메타이다.
	Hostname string
	Version  string

	// HeartbeatInterval 은 heartbeat 주기이다(REQ-A05). 0 이면 기본값.
	HeartbeatInterval time.Duration

	// ReconnectInitial / ReconnectMax 는 지수 백오프 파라미터이다(REQ-B04).
	// 0 이면 기본값.
	ReconnectInitial time.Duration
	ReconnectMax     time.Duration

	// Exposure 는 register 요청에 실리는 노출 범위 요약이다(REQ-C01/A04). 실제
	// 미러링 평가는 M4 이며, M2 는 등록 요약 운반 용도로만 사용한다.
	Exposure ExposureSummary

	// BootstrapSecret 은 (선택) enrollment 사전 공유 시크릿이다(REQ-C08). 서버에
	// 동일 시크릿이 구성된 경우 register 의 1차 신뢰 검증에 사용된다. 시크릿이므로
	// 로깅/커밋 대상이 아니다(REQ-F06).
	BootstrapSecret string

	// DataDir 은 노드 토큰 영속 디렉토리이다(REQ-C04/C05). 비어 있으면 토큰
	// 영속/로드를 건너뛴다(in-memory only).
	DataDir string

	// Logger 는 선택적 로거이다.
	Logger *slog.Logger
}

// Client 는 관리 클라이언트의 dial/heartbeat/재연결 루프를 소유한다.
type Client struct {
	cfg    ClientConfig
	dialer Dialer
	logger *slog.Logger

	stopOnce sync.Once
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	mu        sync.Mutex
	conn      Conn
	nodeToken string // 영속/메모리의 현재 노드 토큰(REQ-C04/C05). 미승인 시 빈 값.
	rejected  bool   // rejected ack 수신 시 true → 재연결 중단(REQ-C03).
}

// NewClient 는 Client 를 생성한다. dialer 가 nil 이면 gorilla dialer 를 사용한다.
func NewClient(cfg ClientConfig, dialer Dialer) *Client {
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if cfg.ReconnectInitial <= 0 {
		cfg.ReconnectInitial = DefaultReconnectInitial
	}
	if cfg.ReconnectMax <= 0 {
		cfg.ReconnectMax = DefaultReconnectMax
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if dialer == nil {
		dialer = NewGorillaDialer()
	}
	c := &Client{
		cfg:    cfg,
		dialer: dialer,
		logger: logger,
	}
	// 영속된 노드 토큰을 로드한다(REQ-C05 — 재접속 인증).
	if cfg.DataDir != "" {
		if tok, ok, err := LoadNodeToken(cfg.DataDir); err != nil {
			logger.Warn("노드 토큰 로드 실패", "error", err)
		} else if ok {
			c.nodeToken = tok
		}
	}
	return c
}

// Stopped 는 클라이언트가 정지(rejected 또는 Stop 호출)되었는지 반환한다.
func (c *Client) Stopped() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rejected
}

// Start 는 연결 라이프사이클 고루틴을 시작한다(비블로킹).
// ServerURL 이 비어 있으면 설정 오류를 기록하고 dial 하지 않는다(REQ-A02).
func (c *Client) Start(ctx context.Context) {
	if c.cfg.ServerURL == "" {
		c.logger.Error("remote client: server_url 미설정 — 접속 시도 안 함",
			"instance_id", c.cfg.InstanceID)
		return
	}

	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	c.wg.Add(1)
	go c.runLoop(runCtx)
}

// Stop 은 클라이언트를 정상 종료한다(고루틴 정리 + 연결 종료). 멱등하다.
func (c *Client) Stop() {
	c.stopOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		c.closeConn()
	})
	c.wg.Wait()
}

// runLoop 는 dial → 세션 → (종료 시) 백오프 재연결을 반복한다(REQ-B01/B04).
func (c *Client) runLoop(ctx context.Context) {
	defer c.wg.Done()

	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// rejected ack 를 받았으면 재연결을 멈춘다(REQ-C03 — 거부 시 정지).
		if c.isRejected() {
			c.logger.Error("원격 등록 거부됨 — 재연결 중단", "instance_id", c.cfg.InstanceID)
			return
		}

		if attempt > 0 {
			backoff := c.calculateBackoff(attempt)
			c.logger.Debug("remote client 재연결 대기",
				"attempt", attempt, "backoff", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}

		conn, err := c.dialer.Dial(ctx, c.dialURL())
		if err != nil {
			attempt++
			c.logger.Warn("remote client dial 실패",
				"attempt", attempt, "error", err)
			continue
		}

		c.logger.Info("remote client 연결 수립",
			"instance_id", c.cfg.InstanceID, "server_url", c.cfg.ServerURL)

		// 세션 실행(연결 종료 또는 ctx 취소 시 반환).
		c.runSession(ctx, conn)

		select {
		case <-ctx.Done():
			return
		default:
			// 연결이 끊김 → 재연결 시도. 성공 세션 직후이므로 백오프 카운터를
			// 1 로 리셋해 첫 재시도를 빠르게 한다(지수 백오프는 연속 실패에서만 증가).
			attempt = 1
		}
	}
}

// runSession 은 단일 연결 세션을 실행한다: hello 송신 → heartbeat 루프 + 읽기 루프.
// 연결 종료 또는 ctx 취소 시 반환한다.
func (c *Client) runSession(ctx context.Context, conn Conn) {
	c.setConn(conn)
	defer c.closeConn()

	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 핸드셰이크 송신: 노드 토큰이 있으면 hello(세션 복원, 토큰은 dial URL 로 검증
	// 완료 — REQ-C05), 없으면 register(등록 요청 — REQ-C01).
	if err := c.sendHandshake(conn); err != nil {
		c.logger.Warn("remote client 핸드셰이크 송신 실패", "error", err)
		return
	}

	var wg sync.WaitGroup

	// 읽기 루프: 서버 메시지 수신. 연결 종료를 감지해 세션을 종료한다.
	// M3 seam: command 수신 → 로컬 어댑터 적용 → command_result 반환.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		for {
			data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			c.handleServerMessage(data)
		}
	}()

	// heartbeat 루프.
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(c.cfg.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-sessionCtx.Done():
				return
			case <-ticker.C:
				if err := c.sendHeartbeat(conn); err != nil {
					c.logger.Debug("remote client heartbeat 송신 실패", "error", err)
					cancel()
					return
				}
			}
		}
	}()

	// ctx 취소 시 연결을 닫아 읽기 블로킹을 해제한다.
	go func() {
		<-sessionCtx.Done()
		_ = conn.Close()
	}()

	wg.Wait()
}

// handleServerMessage 는 서버가 보낸 메시지를 처리한다.
// M1 은 수신만 하고(연결 생존성 확인), command/register_ack 처리는 M2/M3 seam.
func (c *Client) handleServerMessage(data []byte) {
	msg, err := DecodeMessage(data)
	if err != nil {
		c.logger.Debug("remote client 메시지 디코드 실패", "error", err)
		return
	}
	switch msg.Type {
	case TypeHeartbeat:
		// 서버 heartbeat — 무시(생존성은 연결 자체로 확인).
	case TypeRegisterAck:
		c.handleRegisterAck(msg.Payload)
	default:
		// M3 seam: command 처리 진입점.
		c.logger.Debug("미처리 서버 메시지 타입(M3 seam)", "type", msg.Type)
	}
}

// handleRegisterAck 는 register_ack 를 처리한다(REQ-C03/C04/C05).
//
//   - approved: node_token 을 영속하고 메모리에 보관(이후 재접속 인증).
//   - pending:  대기(별도 동작 없음; 재연결 시 재시도/heartbeat 유지).
//   - rejected: 정지 플래그를 세워 재연결을 멈추고 토큰을 정리한다.
func (c *Client) handleRegisterAck(payload []byte) {
	var ack RegisterAckPayload
	if err := json.Unmarshal(payload, &ack); err != nil {
		c.logger.Debug("register_ack 디코드 실패", "error", err)
		return
	}
	switch ack.Status {
	case RegStatusApproved:
		c.mu.Lock()
		c.nodeToken = ack.NodeToken
		c.rejected = false
		c.mu.Unlock()
		if c.cfg.DataDir != "" && ack.NodeToken != "" {
			if err := SaveNodeToken(c.cfg.DataDir, ack.NodeToken); err != nil {
				c.logger.Error("노드 토큰 영속 실패", "error", err)
			}
		}
		// 토큰 값은 로깅하지 않는다(REQ-F06).
		c.logger.Info("원격 등록 승인됨 — 노드 토큰 수신", "instance_id", c.cfg.InstanceID)
	case RegStatusPending:
		c.logger.Info("원격 등록 대기 중(pending) — 관리자 승인 대기", "instance_id", c.cfg.InstanceID)
	case RegStatusRejected:
		c.setRejected()
		if c.cfg.DataDir != "" {
			_ = ClearNodeToken(c.cfg.DataDir)
		}
		c.logger.Error("원격 등록 거부됨", "instance_id", c.cfg.InstanceID, "reason", ack.Reason)
	default:
		c.logger.Warn("알 수 없는 register_ack 상태", "status", ack.Status)
	}
}

// sendHandshake 는 첫 핸드셰이크 메시지를 송신한다. 토큰 보유 시 hello(세션 복원),
// 미보유 시 register(등록 요청).
func (c *Client) sendHandshake(conn Conn) error {
	if c.hasToken() {
		return c.sendHello(conn)
	}
	return c.sendRegister(conn)
}

// sendHello 는 hello 메시지를 송신한다(REQ-B01, 토큰 보유 재접속 경로).
func (c *Client) sendHello(conn Conn) error {
	msg, err := NewHelloMessage(HelloPayload{
		InstanceID: c.cfg.InstanceID,
		Hostname:   c.cfg.Hostname,
		Version:    c.cfg.Version,
	})
	if err != nil {
		return err
	}
	return writeEnvelope(conn, msg)
}

// sendRegister 는 register 메시지를 송신한다(REQ-C01, 토큰 미보유 등록 경로).
func (c *Client) sendRegister(conn Conn) error {
	msg, err := NewRegisterMessage(RegisterPayload{
		InstanceID:      c.cfg.InstanceID,
		Hostname:        c.cfg.Hostname,
		Version:         c.cfg.Version,
		Exposure:        c.cfg.Exposure,
		BootstrapSecret: c.cfg.BootstrapSecret,
	})
	if err != nil {
		return err
	}
	return writeEnvelope(conn, msg)
}

// sendHeartbeat 는 heartbeat 메시지를 송신한다(REQ-B03).
func (c *Client) sendHeartbeat(conn Conn) error {
	msg, err := NewHeartbeatMessage(c.cfg.InstanceID)
	if err != nil {
		return err
	}
	return writeEnvelope(conn, msg)
}

// calculateBackoff 는 지수 백오프 + 지터(+-20%)를 계산한다(REQ-B04).
// ReconnectInitial * 2^(attempt-1), ReconnectMax 상한.
// socket TCP 클라이언트 패턴(calculateBackoff)을 준용한다.
func (c *Client) calculateBackoff(attempt int) time.Duration {
	base := c.cfg.ReconnectInitial
	multiplier := math.Pow(2, float64(attempt-1))
	backoff := time.Duration(float64(base) * multiplier)

	if backoff > c.cfg.ReconnectMax || backoff <= 0 {
		backoff = c.cfg.ReconnectMax
	}

	// +-20% 지터.
	jitter := float64(backoff) * 0.2 * (2*rand.Float64() - 1) //nolint:gosec // 지터는 보안 민감 정보 아님
	backoff = time.Duration(float64(backoff) + jitter)
	if backoff < time.Millisecond {
		backoff = time.Millisecond
	}
	return backoff
}

// hasToken 은 노드 토큰을 보유 중인지 반환한다.
func (c *Client) hasToken() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.nodeToken != ""
}

// isRejected 는 rejected ack 로 정지되었는지 반환한다.
func (c *Client) isRejected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rejected
}

// setRejected 는 정지 플래그를 세우고 진행 중 세션을 종료한다(재연결 중단).
func (c *Client) setRejected() {
	c.mu.Lock()
	c.rejected = true
	c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
	}
}

// dialURL 은 dial 에 사용할 URL 을 구성한다. 노드 토큰이 있으면 ?token= 쿼리
// 파라미터로 제시한다(REQ-C05, websocket.go 의 ?token= 패턴 준용). 토큰 값은
// 로깅하지 않는다(REQ-F06).
func (c *Client) dialURL() string {
	c.mu.Lock()
	token := c.nodeToken
	c.mu.Unlock()
	if token == "" {
		return c.cfg.ServerURL
	}
	sep := "?"
	if strings.Contains(c.cfg.ServerURL, "?") {
		sep = "&"
	}
	return c.cfg.ServerURL + sep + "token=" + url.QueryEscape(token)
}

func (c *Client) setConn(conn Conn) {
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
}

func (c *Client) closeConn() {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}
