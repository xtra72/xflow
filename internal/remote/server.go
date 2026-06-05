// server.go 는 관리 서버(mode=server)의 노드 연결 수락과 online/offline 추적을
// 구현한다(REQ-REMOTE-B05, B06, spec §5.5 server).
//
// M1 범위:
//   - 노드 연결 수락 + hello/heartbeat 파싱으로 instance_id 식별.
//   - in-memory NodeRegistry 로 online/offline/last_seen 추적.
//   - heartbeat 타임아웃 sweep.
//   - Authenticator seam(M1 은 부트스트랩-시크릿 stub; M2 에서 토큰 인증/등록
//     승인 상태 머신으로 확장).
//
// M2~M4 seam (본 파일에서 의도적으로 미구현):
//   - 등록/승인 상태 머신(pending/approved/rejected/revoked) → M2.
//   - 명령 디스패처(상관 id·타임아웃) → M3.
//   - 인벤토리 수신 → ManagedNodeRepository 영속 캐시 → M4.
//
// 전송 비의존: HandleConnection 은 최소 Conn 인터페이스에서 동작하여 HTTP 없이
// 테스트 가능하다. HTTP 업그레이드는 internal/api/handler/remote.go 가 담당한다.
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// DefaultHeartbeatTimeout 은 heartbeat 미수신 시 노드를 offline 으로 간주하기까지의
// 기본 임계 시간이다. heartbeat 주기보다 충분히 길어야 한다.
const DefaultHeartbeatTimeout = 90 * time.Second

// Conn 은 관리 채널의 메시지 단위 양방향 연결 추상화이다.
// gorilla/websocket 연결을 래핑하거나, 테스트에서 인메모리로 구현한다.
type Conn interface {
	// ReadMessage 는 다음 메시지를 블로킹으로 읽는다. 연결 종료 시 에러를 반환한다.
	ReadMessage() ([]byte, error)
	// WriteMessage 는 메시지를 전송한다.
	WriteMessage(data []byte) error
	// Close 는 연결을 닫는다. 멱등이어야 한다.
	Close() error
}

// Authenticator 는 핸드셰이크 시 노드 hello 를 검증하는 seam 이다.
//
// M1 은 부트스트랩-시크릿 stub 만 제공한다(NewBootstrapAuthenticator). M2 에서
// 노드 토큰(JWT) 검증·등록 승인 게이팅으로 확장한다(REQ-B02, C04/C05/F02/F03).
type Authenticator interface {
	// Authenticate 는 hello 페이로드를 검증한다. nil 이면 연결을 수락한다.
	Authenticate(hello HelloPayload) error
}

// AuthenticatorFunc 는 함수를 Authenticator 로 어댑트한다.
type AuthenticatorFunc func(HelloPayload) error

// Authenticate 는 Authenticator 를 구현한다.
func (f AuthenticatorFunc) Authenticate(hello HelloPayload) error { return f(hello) }

// bootstrapAuthenticator 는 M1 의 기본 authenticator 이다.
//
// M1 동작(stub): 항상 수락한다. 부트스트랩 시크릿이 구성되어 있어도, M1 의
// hello 페이로드는 시크릿을 운반하지 않으므로(시크릿 운반·검증은 M2 register
// 흐름) 연결 자체는 허용한다. 이는 spec §C08 "미구성 시 순수 관리자 승인 흐름"
// 과 일관된 보수적 seam 이다(승인 게이팅은 M2 에서 강제).
type bootstrapAuthenticator struct {
	secret string
}

// NewBootstrapAuthenticator 는 부트스트랩-시크릿 기반 authenticator stub 을
// 생성한다(M1 seam, M2 에서 토큰 인증으로 대체/보강).
func NewBootstrapAuthenticator(secret string) Authenticator {
	return &bootstrapAuthenticator{secret: secret}
}

func (a *bootstrapAuthenticator) Authenticate(_ HelloPayload) error {
	// M1: accept-with-bootstrap-secret-check-stub.
	// M2 seam: 여기서 노드 토큰 검증 / 등록 상태(pending/approved) 게이팅을 수행한다.
	return nil
}

// NodeState 는 한 관리 노드의 런타임 상태 스냅샷이다.
type NodeState struct {
	InstanceID string
	Hostname   string
	Version    string
	Online     bool
	LastSeen   time.Time
}

// ServerConfig 는 관리 서버의 동작 파라미터이다.
type ServerConfig struct {
	// HeartbeatTimeout 은 heartbeat 미수신 시 offline 으로 간주하는 임계 시간이다.
	// 0 이면 DefaultHeartbeatTimeout 을 사용한다.
	HeartbeatTimeout time.Duration

	// Logger 는 선택적 로거이다. nil 이면 slog.Default() 를 사용한다.
	Logger *slog.Logger
}

// Server 는 관리 서버 측 노드 연결/상태 추적을 담당한다.
type Server struct {
	cfg    ServerConfig
	auth   Authenticator
	logger *slog.Logger

	mu    sync.RWMutex
	nodes map[string]*NodeState
}

// NewServer 는 Server 를 생성한다. auth 가 nil 이면 부트스트랩 authenticator(빈
// 시크릿 — 전부 수락 stub)를 사용한다.
func NewServer(cfg ServerConfig, auth Authenticator) *Server {
	if cfg.HeartbeatTimeout <= 0 {
		cfg.HeartbeatTimeout = DefaultHeartbeatTimeout
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if auth == nil {
		auth = NewBootstrapAuthenticator("")
	}
	return &Server{
		cfg:    cfg,
		auth:   auth,
		logger: logger,
		nodes:  make(map[string]*NodeState),
	}
}

// HandleConnection 은 단일 노드 연결의 읽기 루프를 실행한다(블로킹).
//
// 흐름:
//  1. 첫 메시지로 hello 를 기대한다(instance_id 식별). hello 외 메시지는 식별
//     전까지 무시한다.
//  2. Authenticator 로 검증한다. 실패 시 연결을 닫고 반환한다(노드 미등록).
//  3. 노드를 online 으로 등록한다.
//  4. 이후 heartbeat/status 로 last_seen 을 갱신한다.
//  5. ctx 취소 또는 읽기 에러(연결 종료) 시 노드를 offline 으로 표시하고 반환한다
//     (last-known 보존 — REQ-B06).
func (s *Server) HandleConnection(ctx context.Context, conn Conn) error {
	// ctx 취소 시 읽기 블로킹을 해제하기 위해 conn 을 닫는다.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stop:
		}
	}()

	instanceID := ""
	defer func() {
		if instanceID != "" {
			s.markOffline(instanceID)
		}
	}()

	for {
		data, err := conn.ReadMessage()
		if err != nil {
			if instanceID == "" {
				// 식별 전 종료 — 조용히 반환.
				return nil
			}
			s.logger.Debug("관리 노드 연결 종료", "instance_id", instanceID, "error", err)
			return nil
		}

		msg, decodeErr := DecodeMessage(data)
		if decodeErr != nil {
			s.logger.Debug("관리 메시지 디코드 실패", "error", decodeErr)
			continue
		}

		if instanceID == "" {
			// 식별 전: hello 만 처리한다.
			if msg.Type != TypeHello {
				continue
			}
			var hello HelloPayload
			if err := json.Unmarshal(msg.Payload, &hello); err != nil {
				s.logger.Debug("hello 페이로드 디코드 실패", "error", err)
				continue
			}
			if hello.InstanceID == "" {
				s.logger.Warn("hello 에 instance_id 누락 — 무시")
				continue
			}
			if authErr := s.auth.Authenticate(hello); authErr != nil {
				s.logger.Warn("관리 노드 인증 거부",
					"instance_id", hello.InstanceID, "error", authErr)
				_ = conn.Close()
				return authErr
			}
			instanceID = hello.InstanceID
			s.markOnline(hello)
			s.logger.Info("관리 노드 online", "instance_id", instanceID,
				"hostname", hello.Hostname, "version", hello.Version)
			continue
		}

		// 식별 후: heartbeat/status 로 생존성 갱신.
		switch msg.Type {
		case TypeHeartbeat, TypeStatus:
			s.touch(instanceID)
		default:
			// 그 외 타입(register/command/inventory 등)은 후속 마일스톤에서 처리.
			s.logger.Debug("M1 미처리 관리 메시지 타입",
				"type", msg.Type, "instance_id", instanceID)
		}
	}
}

// markOnline 은 노드를 online 으로 등록(또는 갱신)한다.
func (s *Server) markOnline(hello HelloPayload) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.nodes[hello.InstanceID]
	if !ok {
		st = &NodeState{InstanceID: hello.InstanceID}
		s.nodes[hello.InstanceID] = st
	}
	st.Hostname = hello.Hostname
	st.Version = hello.Version
	st.Online = true
	st.LastSeen = now
}

// touch 는 노드의 last_seen 을 현재 시각으로 갱신하고 online 으로 유지한다.
func (s *Server) touch(instanceID string) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.nodes[instanceID]; ok {
		st.LastSeen = now
		st.Online = true
	}
}

// markOffline 은 노드를 offline 으로 표시한다(레지스트리에서 삭제하지 않음 —
// last-known 보존, REQ-B06).
func (s *Server) markOffline(instanceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.nodes[instanceID]; ok && st.Online {
		st.Online = false
		s.logger.Info("관리 노드 offline", "instance_id", instanceID)
	}
}

// NodeState 는 instance_id 의 현재 상태 사본을 반환한다.
func (s *Server) NodeState(instanceID string) (NodeState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.nodes[instanceID]
	if !ok {
		return NodeState{}, false
	}
	return *st, true
}

// Nodes 는 모든 노드 상태의 스냅샷을 반환한다.
func (s *Server) Nodes() []NodeState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]NodeState, 0, len(s.nodes))
	for _, st := range s.nodes {
		out = append(out, *st)
	}
	return out
}

// NodeCount 는 추적 중인 노드 수(online+offline)를 반환한다.
func (s *Server) NodeCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.nodes)
}

// sweepOnce 는 now 기준으로 heartbeat 타임아웃을 넘긴 online 노드를 offline 으로
// 표시한다(REQ-B05). StartSweeper 가 주기적으로 호출한다.
func (s *Server) sweepOnce(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, st := range s.nodes {
		if st.Online && now.Sub(st.LastSeen) > s.cfg.HeartbeatTimeout {
			st.Online = false
			s.logger.Info("관리 노드 heartbeat 타임아웃 → offline", "instance_id", id)
		}
	}
}

// StartSweeper 는 heartbeat 타임아웃 sweep 고루틴을 시작한다. ctx 취소 시 종료한다.
// 반환된 함수를 호출하거나 ctx 를 취소하면 정리된다(goroutine leak 방지).
func (s *Server) StartSweeper(ctx context.Context) {
	interval := s.cfg.HeartbeatTimeout / 2
	if interval <= 0 {
		interval = DefaultHeartbeatTimeout / 2
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				s.sweepOnce(t)
			}
		}
	}()
}

// ErrConnClosed 는 연결이 종료되었을 때 Conn 구현이 사용할 수 있는 센티널이다.
var ErrConnClosed = errors.New("remote: connection closed")
