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

	"github.com/xtra/xflow/internal/api/ws"
	"github.com/xtra/xflow/internal/storage"
)

// DefaultHeartbeatTimeout 은 heartbeat 미수신 시 노드를 offline 으로 간주하기까지의
// 기본 임계 시간이다. heartbeat 주기보다 충분히 길어야 한다.
const DefaultHeartbeatTimeout = 90 * time.Second

// DefaultCommandTimeout 은 디스패치된 명령의 결과 대기 기본 제한 시간이다(REQ-D06).
// 결과가 이 시간 내 도착하지 않으면 명령은 타임아웃 처리되고 미적용으로 간주된다.
const DefaultCommandTimeout = 30 * time.Second

// DefaultBridgeOpenTimeout 은 bridge_open 후 bridge_open_ack 대기 기본 제한 시간이다
// (SPEC-SUBFLOW-001 RB05). 이 시간 내 ack 가 도착하지 않으면 OpenBridge 는 타임아웃
// 오류를 반환한다(노드 미응답/실행 시작 실패로 간주).
const DefaultBridgeOpenTimeout = 30 * time.Second

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
	// Status 는 등록 상태(pending/approved/rejected/revoked)이다(M2). repo 가
	// 주입되지 않은 M1 모드에서는 빈 문자열일 수 있다.
	Status string
}

// ServerConfig 는 관리 서버의 동작 파라미터이다.
type ServerConfig struct {
	// HeartbeatTimeout 은 heartbeat 미수신 시 offline 으로 간주하는 임계 시간이다.
	// 0 이면 DefaultHeartbeatTimeout 을 사용한다.
	HeartbeatTimeout time.Duration

	// Repo 는 관리 노드 영속 저장소이다(M2). nil 이면 등록/승인 상태 머신은
	// 비활성화되고 M1 의 in-memory online/offline 추적만 동작한다(하위 호환).
	Repo storage.ManagedNodeRepository

	// Mirror 는 인벤토리 미러 캐시 저장소이다(M4, REQ-E03). nil 이면 inventory_snapshot/
	// inventory_delta 수신은 무시된다(미러링 비활성). 노드가 push 한 snapshot/delta
	// 로만 변경되며, 서버 admin 편집은 본 저장소를 직접 쓰지 않는다(REQ-E08/A4).
	Mirror storage.MirrorRepository

	// TokenIssuer 는 노드 토큰 발급/검증/폐기를 담당한다(M2, REQ-C04/C05/C07/F07).
	// Repo 와 함께 주입되어야 등록/승인 흐름이 완전 동작한다.
	TokenIssuer TokenIssuer

	// BootstrapSecret 은 (선택) enrollment 사전 공유 시크릿이다(REQ-C08). 비어
	// 있으면 순수 관리자 승인 흐름이다. 설정 시 register 요청의 시크릿과 일치해야
	// pending 큐잉된다.
	BootstrapSecret string

	// CommandTimeout 은 디스패치된 명령의 결과 대기 제한 시간이다(REQ-D06). 0 이면
	// DefaultCommandTimeout 을 사용한다.
	CommandTimeout time.Duration

	// QueryTimeout 은 디스패치된 READ 질의의 결과 대기 제한 시간이다(M8, REQ-J07). 0 이면
	// DefaultQueryTimeout 을 사용한다. 타임아웃 시 질의는 미응답으로 간주되어 504 로
	// 매핑된다(mapRemoteQueryError).
	QueryTimeout time.Duration

	// QueryCacheTTL 은 READ 응답 단기 캐시 TTL 이다(M8, REQ-J16). 0 이면 DefaultQueryCacheTTL.
	// 라이브 action(IsStreamableAction)은 캐시를 우회한다.
	QueryCacheTTL time.Duration

	// Audit 는 원격 변경 감사 로그 저장소이다(M6, REQ-F05). nil 이면 감사는 구조화
	// 로그로만 남고 영속화되지 않는다(하위 호환). 명령 디스패치 결과(성공/실패/타임
	// 아웃)를 누가/언제/어느 노드/도메인·액션/결과로 기록한다. 시크릿은 기록하지
	// 않는다(REQ-F06).
	Audit storage.RemoteAuditRepository

	// Enrollment 는 enrollment 토큰 저장소이다(v1.1 그룹 H, REQ-REMOTE-H05). nil 이면
	// enrollment 토큰 기반 자동 승인은 비활성화되고(register 의 enrollment_token 무시)
	// 기존 pending 흐름만 동작한다(하위 호환). 토큰은 SHA-256 해시로만 저장된다(REQ-H06).
	Enrollment storage.EnrollmentTokenRepository

	// VersionHistory 는 노드 버전 변경 이력 저장소이다(버전 관리 Phase 1). nil 이면
	// 버전 이력 기록은 비활성화된다(하위 호환). 노드가 보고한 version 이 직전 저장값과
	// 달라질 때마다 (instance_id, version, changed_at) 한 줄을 append 한다.
	VersionHistory storage.NodeVersionHistoryRepository

	// Logger 는 선택적 로거이다. nil 이면 slog.Default() 를 사용한다.
	Logger *slog.Logger
}

// nodeConn 은 라이브 노드 연결을 추적한다(approve 시 ack push, revoke 시 종료).
type nodeConn struct {
	conn   Conn
	cancel context.CancelFunc
}

// Server 는 관리 서버 측 노드 연결/상태 추적 + 등록/승인 상태 머신을 담당한다.
type Server struct {
	cfg     ServerConfig
	auth    Authenticator
	repo    storage.ManagedNodeRepository
	mirror  storage.MirrorRepository
	tokens  TokenIssuer
	audit   storage.RemoteAuditRepository
	enroll  storage.EnrollmentTokenRepository
	verHist storage.NodeVersionHistoryRepository
	logger  *slog.Logger

	mu    sync.RWMutex
	nodes map[string]*NodeState
	conns map[string]*nodeConn // instance_id -> 라이브 연결(M2)

	cmdTimeout time.Duration
	pendingMu  sync.Mutex
	pending    map[string]chan CommandResultPayload // command_id -> 결과 채널(M3)

	// M8(그룹 J) READ/QUERY 프록시 상태.
	queryTimeout  time.Duration
	pendingQMu    sync.Mutex
	pendingQuery  map[string]chan QueryResultPayload // query_id -> 결과 채널(M8)
	queryCache    *queryCache                        // 단기 TTL READ 캐시(REQ-J16)
	streamManager *streamManager                     // 브라우저-노드 스트림 팬아웃(REQ-J08)

	// P3(SPEC-SUBFLOW-001 그룹 RB) 라이브 브리지 상태. open 대기 상관(bridge_id →
	// ack 채널)과 활성 브리지 레지스트리(bridge_id → ServerBridge)를 보유한다.
	bridgeOpenTimeout time.Duration
	bridgeMu          sync.Mutex
	pendingBridge     map[string]chan BridgeOpenAckPayload // bridge_id -> open ack 채널(RB05)
	bridges           map[string]*ServerBridge             // bridge_id -> 활성 브리지(RB07/RB12)
}

// NewServer 는 Server 를 생성한다. auth 가 nil 이면 부트스트랩 authenticator(빈
// 시크릿 — 전부 수락 stub)를 사용한다. cfg.Repo/cfg.TokenIssuer 가 주입되면 M2
// 등록/승인 상태 머신이 활성화된다.
func NewServer(cfg ServerConfig, auth Authenticator) *Server {
	if cfg.HeartbeatTimeout <= 0 {
		cfg.HeartbeatTimeout = DefaultHeartbeatTimeout
	}
	if cfg.CommandTimeout <= 0 {
		cfg.CommandTimeout = DefaultCommandTimeout
	}
	if cfg.QueryTimeout <= 0 {
		cfg.QueryTimeout = DefaultQueryTimeout
	}
	bridgeOpenTimeout := DefaultBridgeOpenTimeout
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if auth == nil {
		auth = NewBootstrapAuthenticator("")
	}
	s := &Server{
		cfg:          cfg,
		auth:         auth,
		repo:         cfg.Repo,
		mirror:       cfg.Mirror,
		tokens:       cfg.TokenIssuer,
		audit:        cfg.Audit,
		enroll:       cfg.Enrollment,
		verHist:      cfg.VersionHistory,
		logger:       logger,
		nodes:        make(map[string]*NodeState),
		conns:        make(map[string]*nodeConn),
		cmdTimeout:   cfg.CommandTimeout,
		pending:      make(map[string]chan CommandResultPayload),
		queryTimeout: cfg.QueryTimeout,
		pendingQuery: make(map[string]chan QueryResultPayload),
		queryCache:   newQueryCache(cfg.QueryCacheTTL, DefaultQueryCacheMaxEntries),

		bridgeOpenTimeout: bridgeOpenTimeout,
		pendingBridge:     make(map[string]chan BridgeOpenAckPayload),
		bridges:           make(map[string]*ServerBridge),
	}
	s.streamManager = newStreamManager(s)
	return s
}

// HandleConnection 은 단일 노드 연결의 읽기 루프를 실행한다(블로킹).
//
// 핸드셰이크에 노드 토큰이 없는 경로(등록/재등록)이다. 핸들러가 토큰을 검증한
// 재접속 경로는 HandleConnectionAuth 를 사용한다.
//
// 흐름:
//  1. 첫 메시지로 hello 또는 register 를 기대한다(instance_id 식별).
//  2. hello → M1 호환 online 추적(repo 없으면 in-memory 만). register → M2 등록
//     상태 머신(pending 큐잉 + register_ack).
//  3. Authenticator 로 hello 를 검증한다. 실패 시 연결을 닫고 반환한다.
//  4. 이후 heartbeat/status 로 last_seen 을 갱신한다.
//  5. ctx 취소 또는 읽기 에러(연결 종료) 시 노드를 offline 으로 표시하고 반환한다
//     (last-known 보존 — REQ-B06).
func (s *Server) HandleConnection(ctx context.Context, conn Conn) error {
	return s.handleConnection(ctx, conn, "")
}

// HandleConnectionAuth 는 핸드셰이크에서 노드 토큰이 검증된 재접속 연결을 처리한다
// (REQ-C05/F02). authedInstanceID 가 비어 있지 않으면, 노드가 승인 상태인 경우
// register 없이 관리 세션을 복원한다.
func (s *Server) HandleConnectionAuth(ctx context.Context, conn Conn, authedInstanceID string) error {
	return s.handleConnection(ctx, conn, authedInstanceID)
}

func (s *Server) handleConnection(ctx context.Context, conn Conn, authedInstanceID string) error {
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// connCtx 취소(또는 부모 ctx 취소, 또는 revoke) 시 읽기 블로킹을 해제한다.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-connCtx.Done():
			_ = conn.Close()
		case <-stop:
		}
	}()

	instanceID := ""
	// ownedConn 은 이 goroutine 이 등록한 라이브 연결의 세대 핸들이다(restoreSession/
	// handleHandshakeMessage → registerConn 가 반환). 연결 종료 시 teardown defer 는
	// 현재 등록이 여전히 이 핸들과 동일할 때에만(=superseded 되지 않았을 때) 자원을
	// 정리한다. 노드 프로그램 재기동으로 새 연결이 s.conns[id] 를 교체했다면 이 이전
	// goroutine 은 teardown 을 전부 건너뛴다(살아 있는 새 세션 보호 — connection-identity
	// 인지 teardown, "동일 연결인 경우에만").
	var ownedConn *nodeConn
	defer func() {
		if instanceID == "" || ownedConn == nil {
			return
		}
		// compare-and-delete: 현재 등록이 이 goroutine 의 핸들과 동일할 때에만 소유권을
		// 인정한다. s.mu 안에서는 비교/삭제만 하고, teardown/markOffline 은 락 밖에서
		// 호출한다(각자 내부에서 s.mu 를 잡으므로 데드락 방지).
		if !s.unregisterConn(instanceID, ownedConn) {
			// superseded: 새 연결이 이미 conns/streams/bridges/online 을 소유한다 —
			// 아무것도 정리하지 않는다(새 세션을 무너뜨리지 않음).
			return
		}
		// M8(REQ-J08b): 세션 종료 시 이 노드의 모든 브라우저 스트림을 teardown 한다
		// (노드 오프라인/연결 종료 → 해당 노드 스트림 전부 종료, 누수 없음).
		s.teardownNodeStreams(instanceID)
		// P3(SUBFLOW RB09): 세션 종료 시 이 노드의 모든 라이브 브리지를 teardown 한다
		// (최종 offline status 방출 + 채널 close — 매니저 엔진 노드가 무출력으로 전이).
		s.teardownNodeBridges(instanceID)
		s.markOffline(instanceID)
	}()

	// 재접속 경로: 토큰이 검증된 노드를 곧바로 세션 복원한다(REQ-C05).
	if authedInstanceID != "" {
		if owned, restored := s.restoreSession(connCtx, conn, cancel, authedInstanceID); restored {
			instanceID = authedInstanceID
			ownedConn = owned
		} else {
			// 승인 상태가 아니면(거부/폐기/미존재) 세션을 복원하지 않고 종료한다.
			_ = conn.Close()
			return nil
		}
	}

	for {
		data, err := conn.ReadMessage()
		if err != nil {
			if instanceID == "" {
				return nil // 식별 전 종료 — 조용히 반환.
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
			id, owned, handled := s.handleHandshakeMessage(connCtx, conn, cancel, msg)
			if handled && id != "" {
				instanceID = id
				ownedConn = owned
			} else if handled {
				// 인증/검증 실패로 연결을 닫아야 하는 경우(hello authenticator 거부).
				return nil
			}
			continue
		}

		// 식별 후: heartbeat/status 로 생존성 갱신, command_result 로 명령 상관.
		switch msg.Type {
		case TypeHeartbeat:
			// 생존성 갱신 + BASIC 시스템 정보 갱신(v1.4 M9, REQ-K07/K08). 시스템 정보는
			// 제공된 필드만 갱신하고 미제공은 보존한다(구버전 노드 하위 호환 — REQ-K09).
			s.touch(instanceID)
			s.handleHeartbeat(connCtx, instanceID, msg.Payload)
		case TypeStatus:
			s.touch(instanceID)
		case TypeCommandResult:
			// 명령 결과를 대기 중인 Dispatch 호출로 라우팅한다(REQ-D05/D07).
			// 생존성도 함께 갱신한다(결과 수신 = 노드 활성).
			s.touch(instanceID)
			s.routeCommandResult(msg.Payload)
		case TypeQueryResult:
			// READ 질의 결과를 대기 중인 DispatchQuery 호출로 라우팅한다(M8, REQ-J02).
			s.touch(instanceID)
			s.routeQueryResult(msg.Payload)
		case TypeStreamData:
			// 스트림 갱신/터미널 오류 프레임을 브라우저 소비자로 팬아웃한다(M8, REQ-J08).
			s.touch(instanceID)
			s.routeStreamData(msg.Payload)
		case TypeBridgeOpenAck:
			// 라이브 브리지 개설 결과를 대기 중인 OpenBridge 호출로 상관한다(SUBFLOW RB05/RB06).
			s.touch(instanceID)
			s.routeBridgeOpenAck(msg.Payload)
		case TypeBridgeOutput:
			// 원격 출력 경계 메시지를 소유 브리지의 Outputs 채널로 라우팅한다(SUBFLOW RB07).
			s.touch(instanceID)
			s.routeBridgeOutput(msg.Payload)
		case TypeBridgeStatus:
			// 브리지 라이프사이클/헬스 신호를 소유 브리지의 Status 채널로 라우팅한다(SUBFLOW RB09).
			s.touch(instanceID)
			s.routeBridgeStatus(msg.Payload)
		case TypeInventorySnapshot:
			// 접속 시 전체 인벤토리 — 노드별 미러를 종류별로 교체한다(REQ-E01/E03).
			s.touch(instanceID)
			s.handleInventorySnapshot(connCtx, instanceID, msg.Payload)
		case TypeInventoryDelta:
			// 변경 델타 — add/update/remove 를 미러에 적용한다(REQ-E02/E03).
			s.touch(instanceID)
			s.handleInventoryDelta(connCtx, instanceID, msg.Payload)
		default:
			// M5 seam: 추가 텔레메트리 등 향후 메시지 타입 처리 진입점.
			s.logger.Debug("미처리 관리 메시지 타입(M5 seam)",
				"type", msg.Type, "instance_id", instanceID)
		}
	}
}

// handleHandshakeMessage 는 식별 전 첫 메시지(hello 또는 register)를 처리한다.
// 반환: (instanceID, owned, handled). owned 는 이번 연결이 등록한 라이브 연결의 세대
// 핸들이다(소유권 추적용 — teardown defer 가 비교에 사용). handled=true 이고
// instanceID="" 이면 연결을 닫아야 한다(인증/검증 실패).
func (s *Server) handleHandshakeMessage(ctx context.Context, conn Conn, cancel context.CancelFunc, msg *ws.Message) (string, *nodeConn, bool) {
	switch msg.Type {
	case TypeHello:
		var hello HelloPayload
		if err := json.Unmarshal(msg.Payload, &hello); err != nil {
			s.logger.Debug("hello 페이로드 디코드 실패", "error", err)
			return "", nil, false
		}
		if hello.InstanceID == "" {
			s.logger.Warn("hello 에 instance_id 누락 — 무시")
			return "", nil, false
		}
		if authErr := s.auth.Authenticate(hello); authErr != nil {
			s.logger.Warn("관리 노드 인증 거부",
				"instance_id", hello.InstanceID, "error", authErr)
			_ = conn.Close()
			return "", nil, true
		}
		s.markOnline(hello)
		owned := s.registerConn(hello.InstanceID, conn, cancel)
		s.logger.Info("관리 노드 online", "instance_id", hello.InstanceID,
			"hostname", hello.Hostname, "version", hello.Version)
		return hello.InstanceID, owned, true

	case TypeRegister:
		return s.handleRegister(ctx, conn, cancel, msg)

	default:
		// 식별 전 다른 타입은 무시한다.
		return "", nil, false
	}
}

// markOnline 은 노드를 online 으로 등록(또는 갱신)한다.
func (s *Server) markOnline(hello HelloPayload) {
	now := time.Now()
	s.mu.Lock()
	st, ok := s.nodes[hello.InstanceID]
	if !ok {
		st = &NodeState{InstanceID: hello.InstanceID}
		s.nodes[hello.InstanceID] = st
	}
	st.Hostname = hello.Hostname
	st.Version = hello.Version
	st.Online = true
	st.LastSeen = now
	s.mu.Unlock()

	// 저장된 등록 상태가 있으면 in-memory 상태에 반영한다(repo 가 권위 — M2).
	s.syncStatusFromRepo(hello.InstanceID)
	s.persistOnline(hello.InstanceID, true, now)
}

// setNodeState 는 노드의 in-memory 상태를 직접 설정/갱신한다(재접속 복원용).
func (s *Server) setNodeState(instanceID, status string, online bool, lastSeen time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.nodes[instanceID]
	if !ok {
		st = &NodeState{InstanceID: instanceID}
		s.nodes[instanceID] = st
	}
	if status != "" {
		st.Status = status
	}
	st.Online = online
	st.LastSeen = lastSeen
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
	wasOnline := false
	if st, ok := s.nodes[instanceID]; ok && st.Online {
		st.Online = false
		wasOnline = true
	}
	s.mu.Unlock()
	if wasOnline {
		s.logger.Info("관리 노드 offline", "instance_id", instanceID)
		s.persistOnline(instanceID, false, time.Now())
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
