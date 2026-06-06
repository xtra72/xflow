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
	"runtime/debug"
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

	// DefaultInventoryPollInterval 은 인벤토리 poll+diff 델타 소스의 기본 주기이다
	// (M4, REQ-E02). 데몬에 구독 가능한 변경 이벤트 소스가 없어 poll 기반으로 델타를
	// 도출한다(inventory.go 델타 소스 결정 주석 참조).
	DefaultInventoryPollInterval = 30 * time.Second
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

	// EnrollmentToken 은 (선택) 가입 토큰이다(v1.1 그룹 H, REQ-REMOTE-H05). 설정 시
	// 토큰 미보유 register 에 실어 보내, 서버가 유효성을 검증해 관리자 수동 승인 없이
	// 노드를 자동 승인하도록 한다. 시크릿이므로 로깅 대상이 아니다(REQ-F06/H06).
	EnrollmentToken string

	// DataDir 은 노드 토큰 영속 디렉토리이다(REQ-C04/C05). 비어 있으면 토큰
	// 영속/로드를 건너뛴다(in-memory only).
	DataDir string

	// Applier 는 수신한 원격 명령을 로컬 어댑터로 적용하는 구현이다(M3, REQ-D02/D03/
	// D04). nil 이면 command 는 거부된다(미구성 노드 보호). cmd/xflowd 가 구체
	// 어댑터(FlowServiceAdapter 등)를 바인딩한 Applier 를 주입한다.
	Applier CommandApplier

	// Inventory 는 노드의 로컬 자원 인벤토리 소스이다(M4, REQ-E01/E02). nil 이면
	// 인벤토리 미러링이 비활성화된다(snapshot/delta 미송신). cmd/xflowd 가 어댑터를
	// 바인딩한 InventorySource 를 주입한다(redaction 은 소스가 수행 — F06).
	Inventory InventorySource

	// InventoryPollInterval 은 poll 기반 델타 소스의 주기이다(M4, REQ-E02). 0 이면
	// DefaultInventoryPollInterval. 데몬에 구독 가능한 변경 이벤트 소스가 없어
	// poll+diff 로 델타를 도출하므로(inventory.go 주석), 이 주기로 변경을 감지한다.
	InventoryPollInterval time.Duration

	// QuerySource 는 수신한 query 를 노드의 로컬 read 핸들러로 매핑하는 구현이다(M8,
	// REQ-J01/J04). nil 이면 query 는 거부된다(미구성 노드 보호). cmd/xflowd 가 read
	// 어댑터(FlowStatus/AgentStats/device State 등)를 바인딩한 QuerySource 를 주입한다.
	QuerySource QuerySource

	// StreamSource 는 subscribe 를 노드의 실시간 소스로 매핑하는 구현이다(M8, REQ-J08).
	// nil 이면 subscribe 는 거부된다. cmd/xflowd 가 디바이스 상태/에이전트 통계 폴러를
	// 바인딩한 StreamSource 를 주입한다.
	StreamSource StreamSource

	// QueryRedactor 는 query/stream 응답을 전송 전 마스킹한다(M8, REQ-J06). nil 이면
	// pass-through 한다. cmd/xflowd 가 secret_fields SoT 로 구성한다(노드 측 redaction).
	QueryRedactor QueryRedactor

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
	exposure  ExposureSummary

	// remirror 는 노출 설정 변경 시 재미러링을 신호하는 채널이다(REQ-A07). 버퍼 1 로
	// non-blocking; 활성 미러 루프가 없으면 신호는 다음 세션 시작 스냅샷으로 흡수된다.
	remirror chan struct{}

	// approved 는 "현재 세션 도중 노드가 승인됨(register_ack approved → node_token)"을
	// 신호하는 세션별 채널이다(REQ-E01 첫 enrollment 세션 미러 시작). runSession 이
	// 세션마다 새로 만들고(c.mu 보호), 세션 종료 시 nil 로 비운다(cross-session 누수
	// 방지). handleRegisterAck(approved)가 버퍼 1 + non-blocking send 로 정확히 한 번
	// 신호한다(double-close/패닉 없음, remirror 와 동일 관용구). 세션이 이미 끝나
	// nil 이면 신호는 무시된다(미러는 다음 세션 토큰 보유 경로로 시작).
	approved chan struct{}

	// streams 는 현재 세션의 활성 스트림 구독 레지스트리이다(M8, REQ-J08b). subscription_id
	// → 구독 핸들 매핑으로 unsubscribe 시 대상 구독을 찾고, 세션 종료 시 전 구독을
	// teardown 한다(누수 없음). runSession 이 세션마다 새로 만들고(c.mu 보호), 세션
	// 종료 시 모두 정리한 뒤 nil 로 비운다(approved 와 동일 라이프사이클).
	streams map[string]*streamSub
}

// streamSub 는 단일 활성 스트림 구독의 client 측 핸들이다(M8). cancel 로 펌프
// 고루틴을 종료하고, source.Close()는 펌프 defer 에서 호출된다(teardown).
type streamSub struct {
	cancel context.CancelFunc
	source StreamSubscription
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
	if cfg.InventoryPollInterval <= 0 {
		cfg.InventoryPollInterval = DefaultInventoryPollInterval
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if dialer == nil {
		dialer = NewGorillaDialer()
	}
	c := &Client{
		cfg:      cfg,
		dialer:   dialer,
		logger:   logger,
		exposure: cfg.Exposure,
		remirror: make(chan struct{}, 1),
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

	// 세션별 승인 신호 채널을 새로 만든다(REQ-E01). 세션 종료 시 nil 로 비워
	// cross-session 신호 누수/유효하지 않은 채널로의 송신을 방지한다. 미러 고루틴은
	// 이 로컬 참조(approvedCh)를 캡처하므로, 세션 종료 후 c.approved 가 nil 이 되어도
	// nil 채널 select 위험이 없다.
	approvedCh := make(chan struct{}, 1)
	c.mu.Lock()
	c.approved = approvedCh
	// 세션별 스트림 구독 레지스트리를 초기화한다(M8, REQ-J08b). 세션 종료 시 전 구독을
	// teardown 한다(아래 defer).
	c.streams = make(map[string]*streamSub)
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.approved = nil
		// 세션 종료 — 모든 활성 스트림 구독을 teardown 한다(노드 오프라인/연결 종료 시
		// 전 구독 정리, 누수 없음 — REQ-J08b). cancel 이 펌프 고루틴을 종료하고, 펌프
		// defer 가 source.Close()를 호출한다. 펌프는 세션 wg 로 추적되어 wg.Wait()가
		// 완료를 보장한다.
		subs := c.streams
		c.streams = nil
		c.mu.Unlock()
		for _, s := range subs {
			s.cancel()
		}
	}()

	// 핸드셰이크 송신: 노드 토큰이 있으면 hello(세션 복원, 토큰은 dial URL 로 검증
	// 완료 — REQ-C05), 없으면 register(등록 요청 — REQ-C01).
	if err := c.sendHandshake(conn); err != nil {
		c.logger.Warn("remote client 핸드셰이크 송신 실패", "error", err)
		return
	}

	var wg sync.WaitGroup

	// 읽기 루프: 서버 메시지 수신. 연결 종료를 감지해 세션을 종료한다.
	// command 수신 → 로컬 어댑터 적용 → command_result 반환(M3).
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		for {
			data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			c.handleServerMessage(sessionCtx, conn, &wg, data)
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

	// 인벤토리 미러 루프(M4): Inventory 소스가 구성된 경우에만 동작한다. 미승인 노드는
	// 미러링하지 않는다(REQ-C06/F03 정신).
	//
	//   - 세션 시작 시 이미 토큰 보유(재접속/hello 경로) → 즉시 미러 시작.
	//   - 토큰 미보유(첫 enrollment 경로) → 같은 세션의 register_ack(approved)로 토큰이
	//     도착할 때까지 대기했다가 미러를 시작한다(REQ-E01). 끝내 승인되지 않으면
	//     sessionCtx 취소로 깨끗이 종료한다(goroutine leak 방지).
	//
	// 어느 경로든 wg 로 추적되어 세션 종료/취소 시 sessionCtx 로 정리된다.
	if c.cfg.Inventory != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c.hasToken() {
				c.runMirror(sessionCtx, conn)
				return
			}
			// 미승인 — 같은 세션에서 승인될 때까지 대기. 세션이 먼저 끝나면 no-op.
			select {
			case <-sessionCtx.Done():
				return
			case <-approvedCh:
				c.runMirror(sessionCtx, conn)
			}
		}()
	}

	// ctx 취소 시 연결을 닫아 읽기 블로킹을 해제한다.
	go func() {
		<-sessionCtx.Done()
		_ = conn.Close()
	}()

	wg.Wait()
}

// handleServerMessage 는 서버가 보낸 메시지를 처리한다.
//
//   - heartbeat: 무시(생존성은 연결 자체로 확인).
//   - register_ack: 등록 응답 처리(REQ-C03/C04/C05).
//   - command: 출처/승인 검증 후 로컬 어댑터로 적용하고 command_result 반환(M3,
//     REQ-D02/D03/D04/D05/D08). 적용은 별도 고루틴에서 수행하여 읽기 루프를 막지
//     않으며, 동시 다수 명령을 병렬 처리한다(REQ-D07). 세션 WaitGroup 으로 추적하여
//     연결 종료/취소 시 고루틴 누수를 방지한다.
func (c *Client) handleServerMessage(ctx context.Context, conn Conn, wg *sync.WaitGroup, data []byte) {
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
	case TypeCommand:
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.handleCommand(ctx, conn, msg.Payload)
		}()
	case TypeQuery:
		// M8: READ 질의 → 로컬 read 매핑 → redaction → query_result. 적용은 별도
		// 고루틴에서 수행하여 읽기 루프를 막지 않으며, 세션 wg 로 추적한다(REQ-J02).
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.handleQuery(ctx, conn, msg.Payload)
		}()
	case TypeSubscribe:
		// M8: 실시간 스트림 구독 시작 → 펌프 고루틴 spawn(세션 wg 추적, REQ-J08/J08b).
		c.handleSubscribe(ctx, conn, wg, msg.Payload)
	case TypeUnsubscribe:
		// M8: 스트림 구독 해제·teardown(REQ-J08b).
		c.handleUnsubscribe(msg.Payload)
	default:
		// inventory 수신 등은 서버 측 책임이므로 클라이언트는 무시한다.
		c.logger.Debug("미처리 서버 메시지 타입", "type", msg.Type)
	}
}

// handleCommand 는 수신한 command 를 적용하고 command_result 를 같은 연결로 반환한다
// (spec §5.7 클라이언트 적용 경로).
//
//  1. 출처/승인 검증(REQ-D08): 명령은 인증된 서버 연결로만 도착하며(클라이언트는
//     자신이 dial 한 서버 연결로만 수신), 노드가 승인 상태(노드 토큰 보유)여야 한다.
//     미승인(토큰 미보유)이면 거부한다.
//  2. domain/action 에 따라 applier 로 적용(REQ-D02/D03/D04). applier 미구성이면 거부.
//  3. 결과를 command_result(ok+result | error)로 같은 연결에 반환한다(REQ-D05/D09).
func (c *Client) handleCommand(ctx context.Context, conn Conn, payload []byte) {
	// 신뢰 경계 panic 복구 가드: Applier.Apply 또는 디코드 과정의 panic 이 명령
	// 고루틴/데몬을 죽이지 않도록 복구하고, panic 을 command_result{ok:false} 로
	// 변환해 같은 연결로 반환한다(REQ-D09 정신 — 부분 적용 없이 실패 보고).
	//
	// 보안(REQ-F06): panic 값/스택은 시크릿을 담을 수 있으므로 외부 응답에는
	// 일반화된 메시지만 노출하고, 상세(값+스택)는 내부 error 로그로만 기록한다.
	//
	// commandID 는 클로저로 캡처하여, 디코드 이후 panic 이면 해당 command_id 로,
	// 디코드 전 panic 이면 빈 값으로 결과를 반환한다(상관 불가 시 서버는 타임아웃 처리).
	commandID := ""
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("원격 명령 적용 중 panic 복구",
				"command_id", commandID,
				"panic", r,
				"stack", string(debug.Stack()))
			c.sendCommandResult(conn, CommandResultPayload{
				CommandID: commandID,
				OK:        false,
				Error:     "internal error while applying command",
			})
		}
	}()

	var cmd CommandPayload
	if err := json.Unmarshal(payload, &cmd); err != nil {
		c.logger.Debug("command 디코드 실패", "error", err)
		return
	}
	if cmd.CommandID == "" {
		c.logger.Warn("command 에 command_id 누락 — 무시")
		return
	}
	commandID = cmd.CommandID

	// 출처/승인 검증(REQ-D08): 승인되지 않은 노드는 명령을 거부한다. 명령은 클라이언트가
	// 자신의 설정된 서버로 맺은 인증 연결로만 도착하므로(비-서버 출처 불가), 승인 여부는
	// 노드 토큰 보유로 판정한다(승인 시 register_ack 로 토큰 수신 — REQ-C04).
	if !c.hasToken() {
		c.logger.Warn("미승인 노드 — 원격 명령 거부",
			"command_id", cmd.CommandID, "domain", cmd.Domain, "action", cmd.Action)
		c.sendCommandResult(conn, CommandResultPayload{
			CommandID: cmd.CommandID,
			OK:        false,
			Error:     "node not approved",
		})
		return
	}

	if c.cfg.Applier == nil {
		c.logger.Warn("applier 미구성 — 원격 명령 거부", "command_id", cmd.CommandID)
		c.sendCommandResult(conn, CommandResultPayload{
			CommandID: cmd.CommandID,
			OK:        false,
			Error:     "command applier not configured",
		})
		return
	}

	// 적용(로컬 어댑터 — A5). Args 는 시크릿 가능성으로 로깅 제외(REQ-F06).
	result, applyErr := c.cfg.Applier.Apply(ctx, cmd.Domain, cmd.Action, cmd.Args)
	if applyErr != nil {
		// 적용 실패 — 부분 적용 없이 오류 보고(REQ-D09).
		c.logger.Warn("원격 명령 적용 실패",
			"command_id", cmd.CommandID, "domain", cmd.Domain,
			"action", cmd.Action, "error", applyErr)
		c.sendCommandResult(conn, CommandResultPayload{
			CommandID: cmd.CommandID,
			OK:        false,
			Error:     applyErr.Error(),
		})
		return
	}

	c.sendCommandResult(conn, CommandResultPayload{
		CommandID: cmd.CommandID,
		OK:        true,
		Result:    result,
	})
}

// sendCommandResult 는 command_result 를 연결로 전송한다(REQ-D05). 전송 실패는
// 로깅만 한다(연결 종료 시).
func (c *Client) sendCommandResult(conn Conn, res CommandResultPayload) {
	msg, err := NewCommandResultMessage(res)
	if err != nil {
		c.logger.Error("command_result 인코딩 실패", "error", err)
		return
	}
	if err := writeEnvelope(conn, msg); err != nil {
		c.logger.Debug("command_result 전송 실패", "command_id", res.CommandID, "error", err)
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
		// 현재 세션의 승인 신호를 정확히 한 번, non-blocking 으로 발사한다(REQ-E01).
		// 버퍼 1 + default 로 double-send/패닉이 없고, 세션이 이미 끝나 c.approved 가
		// nil 이면(또는 이미 신호됨) 안전하게 무시된다(remirror 와 동일 관용구).
		approvedCh := c.approved
		c.mu.Unlock()
		c.signalApproved(approvedCh)
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
		EnrollmentToken: c.cfg.EnrollmentToken,
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

// signalApproved 는 세션별 승인 채널에 non-blocking 으로 한 번 신호한다(REQ-E01).
// ch 가 nil(세션 종료) 이거나 이미 보류 신호가 있으면(coalesce) 안전하게 무시한다.
// 호출 측은 c.mu 밖에서 호출하여 락 보유 중 채널 송신을 피한다.
func (c *Client) signalApproved(ch chan struct{}) {
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
		// 이미 신호됨/대기 중 → 추가 신호 불필요.
	}
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
