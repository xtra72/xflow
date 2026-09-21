// registration.go 는 M2 등록/승인 상태 머신과 노드 토큰 발급/검증/폐기 추상화를
// 구현한다(@SPEC:SPEC-REMOTE-001 M2, spec §5.6, REQ-C01~C08, F02/F03/F07).
//
// 상태 머신(spec §5.6):
//
//	(연결) → register 수신 → [pending]
//	  [pending] --관리자 승인--> [approved] (node_token 발급 → register_ack)
//	  [pending] --관리자 거부--> [rejected]
//	  [approved] --재접속(토큰 유효)--> [approved] (세션 복원)
//	  [approved] --폐기(revoke)--> [revoked] (토큰 blacklist + 연결 종료)
//	  [rejected]/[pending] --명령 디스패치 차단--
//
// pending/rejected/revoked 노드는 managed 가 아니며 명령 대상에서 제외된다(REQ-C06).
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/xtra/xflow/internal/api/ws"
	"github.com/xtra/xflow/internal/storage"
)

// ErrNoRepo 는 등록/승인 작업에 repo 가 필요한데 주입되지 않았을 때 반환된다.
var ErrNoRepo = errors.New("remote: managed node repository not configured")

// TokenIssuer 는 노드 토큰의 발급/검증/폐기를 추상화한다(REQ-C04/C05/C07/F07).
//
// 운영 구현은 internal/auth.JWTService 를 래핑한다(NewJWTTokenIssuer). 테스트는
// 인메모리 구현으로 대체한다. 토큰 문자열 자체는 시크릿이므로 로깅 금지(REQ-F06).
type TokenIssuer interface {
	// Issue 는 subject(=instance_id) + role 로 노드 토큰을 발급한다.
	Issue(subject, role string) (token string, err error)
	// IssueWithID 는 노드 토큰과 그 jti(토큰 식별자)를 함께 발급한다(M6, REQ-F02/F07).
	// 서버는 원본 토큰이 아닌 jti 만 저장하여, DB 유출 시에도 사용 가능한 토큰이
	// 노출되지 않게 한다(노드 토큰 하드닝). 폐기는 RevokeID(jti)로 수행한다.
	IssueWithID(subject, role string) (token, jti string, err error)
	// Validate 는 토큰을 검증하고 subject/role 을 반환한다. blacklist/무효 시 에러.
	Validate(token string) (subject, role string, err error)
	// Revoke 는 토큰을 즉시 무효화한다(blacklist, REQ-F07).
	Revoke(token string)
	// IsRevoked 는 토큰이 폐기되었는지 확인한다.
	IsRevoked(token string) bool
	// RevokeID 는 jti(토큰 식별자)만으로 토큰을 즉시 무효화한다(M6, REQ-F07).
	// 서버 DB 가 jti 만 보유한 채 폐기를 수행할 수 있다(원본 토큰 불필요).
	RevokeID(jti string)
	// IsIDRevoked 는 jti 가 폐기되었는지 확인한다.
	IsIDRevoked(jti string) bool
}

// IsManaged 는 instance_id 가 관리 대상(approved + online)인지 반환한다(REQ-C06).
// pending/rejected/revoked 또는 미연결 노드는 false 이다. M3 명령 디스패치 게이트가
// 이 메서드를 사용한다.
func (s *Server) IsManaged(instanceID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.nodes[instanceID]
	if !ok {
		return false
	}
	return st.Online && st.Status == RegStatusApproved
}

// ManagedNodes 는 현재 관리 대상(approved + online) 노드 ID 목록을 반환한다(REQ-E05 토대).
func (s *Server) ManagedNodes() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0)
	for id, st := range s.nodes {
		if st.Online && st.Status == RegStatusApproved {
			out = append(out, id)
		}
	}
	return out
}

// ListNodes 는 모든 관리 노드를 영속 저장소에서 반환한다(REQ-E05/G01 토대).
// repo 가 없으면 빈 목록을 반환한다(M1 모드 안전).
func (s *Server) ListNodes(ctx context.Context) ([]storage.ManagedNode, error) {
	if s.repo == nil {
		return []storage.ManagedNode{}, nil
	}
	nodes, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	// online 판정은 아래 한 함수를 지난다(@SPEC:SPEC-REMOTE-ONLINE-001 REQ-03, K1).
	now := time.Now()
	s.mu.RLock()
	for i := range nodes {
		nodes[i].Online = s.onlineLocked(nodes[i], now)
	}
	s.mu.RUnlock()
	return nodes, nil
}

// onlineLocked 는 노드가 지금 붙어 있는가를 판정한다
// (@SPEC:SPEC-REMOTE-ONLINE-001 REQ-02, K1/K3). s.mu 를 잡은 채 호출한다.
//
// # 권위는 keep-alive 이지 항목의 유무가 아니다
//
// 종전에는 메모리 항목이 있을 때만 그 값을 덮어썼고, 없으면 **영속값이 그대로 나갔다.**
// 항목은 register/hello 에서만 생기고 노드 삭제에서만 지워지므로, "항목 없음" 은
// "연결 없음" 이 아니라 **"이번 부팅 이후 붙지 않았음"** 이다. 그 둘을 같은 것으로 쓰면
// 재시작이 진실을 지운다 — 서버가 죽을 때 online=1 이던 행이 영원히 online 으로
// 보고되고, 청소기는 항목이 없어 그 노드를 보지도 못한다(사용자 신고 2026-09-17:
// "xagent04 는 연결도 안되어 있는데 online 으로 표시됨").
//
// 그래서 항목이 없을 때는 **keep-alive 가 최근에 왔는가**로 가른다. 하트비트는 재시작
// 경계를 지나 살아남는 유일한 신호다.
//
// `last_seen == 0` 은 "본 적 없음" 이므로 오래된 것으로 읽힌다(offline). 미래 값(시계
// 왜곡)은 `Sub` 이 음수를 내므로 최근으로 읽힌다 — 어느 쪽도 패닉이 아니다(REQ-04).
func (s *Server) onlineLocked(node storage.ManagedNode, now time.Time) bool {
	// 항목이 있으면 라이브 추적이 답한다 — 하트비트가 `touch` 로 갱신하는 그 값이다.
	if st, ok := s.nodes[node.InstanceID]; ok {
		return st.Online
	}
	// 영속값이 이미 offline 이면 최근성을 묻지 않는다.
	if !node.Online {
		return false
	}
	return now.Sub(time.UnixMilli(node.LastSeen)) <= s.cfg.HeartbeatTimeout
}

// OnlineOf 는 `onlineLocked` 의 락을 잡는 겉면이다
// (@SPEC:SPEC-REMOTE-ONLINE-001 REQ-03, K1).
//
// 판정을 **두 벌로 두지 않기 위해** 있다. `ListNodes` 는 목록을 한 번의 RLock 안에서
// 돌므로 `onlineLocked` 를 직접 쓰고, 한 건만 묻는 자리(`NodeDetail`)는 이 겉면을 쓴다.
// 두 자리가 각자 `s.nodes[id]` 를 들여다보면 둘 중 하나만 고쳐지는 날이 온다 — 실제로
// 그런 날이 있었고, 그것이 이 SPEC 이 고친 결함의 둘째 사본이다.
func (s *Server) OnlineOf(node storage.ManagedNode, now time.Time) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.onlineLocked(node, now)
}

// registerConn 은 라이브 연결을 추적한다(approve ack push / revoke 종료용).
//
// 반환값은 이번에 저장된 *nodeConn(세대 핸들)이다. 호출 goroutine 은 이 포인터를
// 보관해 두었다가 연결 종료 시 unregisterConn 으로 소유권을 비교한다. 노드 프로그램
// 재기동으로 같은 instance_id 의 새 연결이 들어와 s.conns[id] 를 교체하면, 이전
// goroutine 이 보관한 핸들은 더 이상 현재 등록과 일치하지 않으므로(포인터 비교) 이전
// teardown 이 새 세션을 무너뜨리지 않게 된다(connection-identity 인지 teardown).
func (s *Server) registerConn(instanceID string, conn Conn, cancel context.CancelFunc) *nodeConn {
	s.mu.Lock()
	defer s.mu.Unlock()
	nc := &nodeConn{conn: conn, cancel: cancel}
	s.conns[instanceID] = nc
	return nc
}

// unregisterConn 은 라이브 연결 추적을 해제한다(동일 연결인 경우에만 — compare-and-delete).
//
// owned 는 이 goroutine 이 registerConn 으로 저장했던 세대 핸들이다. 현재 등록이
// 여전히 owned 와 동일할 때에만 delete 하고 true 를 반환한다. 새 연결이 이미 교체한
// 경우(superseded)에는 아무것도 하지 않고 false 를 반환한다. 호출자는 이 반환값으로
// markOffline/teardown 수행 여부를 gate 한다(살아 있는 새 세션 보호).
//
// 락 순서: s.mu 안에서는 비교/삭제만 수행하고, markOffline/teardownNodeStreams/
// teardownNodeBridges 는 각자 내부에서 락을 잡으므로 반드시 락 밖에서 호출한다(데드락 방지).
func (s *Server) unregisterConn(instanceID string, owned *nodeConn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.conns[instanceID]; ok && cur == owned {
		delete(s.conns, instanceID)
		return true
	}
	return false
}

// connFor 는 instance_id 의 현재 라이브 연결을 반환한다.
func (s *Server) connFor(instanceID string) (*nodeConn, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	nc, ok := s.conns[instanceID]
	return nc, ok
}

// syncStatusFromRepo 는 repo 에 저장된 등록 상태를 in-memory 상태에 반영한다.
func (s *Server) syncStatusFromRepo(instanceID string) {
	if s.repo == nil {
		return
	}
	node, err := s.repo.Get(context.Background(), instanceID)
	if err != nil {
		return // 미등록(아직 register 전) — 상태 없음.
	}
	s.mu.Lock()
	if st, ok := s.nodes[instanceID]; ok {
		st.Status = node.Status
	}
	s.mu.Unlock()
}

// persistOnline 은 repo 가 있으면 online/last_seen 을 영속한다(REQ-B05/E06).
func (s *Server) persistOnline(instanceID string, online bool, at time.Time) {
	if s.repo == nil {
		return
	}
	if err := s.repo.SetOnline(context.Background(), instanceID, online, at.UnixMilli()); err != nil {
		// 미등록 노드(hello-only M1 경로)는 repo 에 없을 수 있다 — 디버그만.
		s.logger.Debug("online 영속 생략", "instance_id", instanceID, "error", err)
	}
}

// admitHello 는 hello 를 받아들일지 판정한다(@SPEC:SPEC-REMOTE-HELLO-GATE-001).
//
// # 문지기가 없던 자리
//
// hello 경로의 문지기(`bootstrapAuthenticator`)는 M1 스텁이라 **무엇이든 수락**했다.
// 그래서 등록 항목이 없는 instance 가 hello 를 보내면 online 으로 로그를 남기고
// 인벤토리까지 미러에 쌓지만, `ListNodes` 는 DB만 읽으므로 그 노드를 0건으로 본다 —
// 로그에는 붙어 있고 화면에는 없는 유령이 된다(사용자 신고 2026-09-17).
//
// 그 자리를 여기서 막는다. repo 가 있는(=server 모드) 서버는 등록 항목이 없거나
// 승인 상태가 아닌 hello 를 거부한다. 거부는 조용히 끊지 않고 `hello_nack` 으로
// **사유를 돌려준다** — 노드가 그 신호로 무효한 토큰을 버리고 register 로 되돌아가
// 스스로 풀려나기 때문이다(조용한 종료는 재접속 루프만 만든다).
//
// repo 가 없는 M1 모드는 판정 근거 자체가 없으므로 종전처럼 수락한다(하위 호환).
func (s *Server) admitHello(ctx context.Context, instanceID string) (string, bool) {
	if s.repo == nil {
		return "", true // M1 모드 — 등록 개념 없음.
	}
	node, err := s.repo.Get(ctx, instanceID)
	if err != nil {
		return HelloNackUnregistered, false
	}
	if node.Status != RegStatusApproved {
		return HelloNackNotApproved, false
	}
	return "", true
}

// rejectHello 는 hello 를 거부하고 사유를 노드에 통지한 뒤 연결을 닫는다
// (@SPEC:SPEC-REMOTE-HELLO-GATE-001).
func (s *Server) rejectHello(conn Conn, instanceID, reason string) {
	s.logger.Warn("hello 거부 — 등록 경로로 되돌림",
		"instance_id", instanceID, "reason", reason)
	s.sendHelloNack(conn, reason)
	_ = conn.Close()
}

// sendHelloNack 는 hello_nack 을 전송한다(전송 실패는 치명적이지 않다 — 연결이 이미
// 끊긴 경우이며, 어느 쪽이든 노드는 재접속한다).
func (s *Server) sendHelloNack(conn Conn, reason string) {
	msg, err := NewHelloNackMessage(HelloNackPayload{Reason: reason})
	if err != nil {
		s.logger.Error("hello_nack 인코딩 실패", "error", err)
		return
	}
	if err := writeEnvelope(conn, msg); err != nil {
		s.logger.Debug("hello_nack 전송 실패", "error", err)
	}
}

// handleRegister 는 register 메시지를 처리한다(REQ-C01/C02/C08, spec §5.6).
//
// 반환: (instanceID, owned, handled). 정상 등록 시 instanceID 와 owned(이번 연결의
// 세대 핸들 — 소유권 추적용)가 채워진다. 부트스트랩 시크릿 불일치 등 거부 시
// instanceID="" / owned=nil 이지만 연결은 유지(rejected ack 전송 후 클라이언트가
// 종료하도록).
func (s *Server) handleRegister(ctx context.Context, conn Conn, cancel context.CancelFunc, msg *ws.Message) (string, *nodeConn, bool) {
	var p RegisterPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		s.logger.Debug("register 페이로드 디코드 실패", "error", err)
		return "", nil, false
	}
	if p.InstanceID == "" {
		s.logger.Warn("register 에 instance_id 누락 — 무시")
		return "", nil, false
	}

	// repo 미구성(M1 모드)에서는 등록을 처리할 수 없다 — hello 경로만 지원.
	if s.repo == nil {
		s.logger.Warn("repo 미구성 — register 무시(M1 모드)", "instance_id", p.InstanceID)
		return "", nil, false
	}

	// 부트스트랩 시크릿 1차 신뢰 검증(REQ-C08). 구성된 경우에만 강제한다.
	if s.cfg.BootstrapSecret != "" && p.BootstrapSecret != s.cfg.BootstrapSecret {
		s.logger.Warn("부트스트랩 시크릿 불일치 — 등록 거부", "instance_id", p.InstanceID)
		s.sendRegisterAck(conn, RegisterAckPayload{
			Status: RegStatusRejected,
			Reason: "bootstrap secret mismatch",
		})
		return "", nil, true
	}

	// 기존 등록 상태 확인.
	existing, err := s.repo.Get(ctx, p.InstanceID)
	switch {
	case errors.Is(err, storage.ErrManagedNodeNotFound):
		// 경로 B(enrollment 토큰): 유효한 토큰을 운반하면 신규 노드를 즉시 자동 승인한다
		// (REQ-H05). bootstrap_secret 게이트(위)를 이미 통과한 뒤이므로 두 게이트가
		// 조합된다. 무효/만료/폐기/소진 토큰은 handled=false 로 pending 폴백한다.
		if s.tryEnrollmentAutoApprove(ctx, conn, p) {
			s.setNodeState(p.InstanceID, RegStatusApproved, true, time.Now())
			owned := s.registerConn(p.InstanceID, conn, cancel)
			return p.InstanceID, owned, true
		}

		// 신규 노드 → pending 큐잉(REQ-C02). 최초 register 의 BASIC 시스템 정보
		// (os/arch/started_at)를 함께 저장한다(v1.4 M9, REQ-K07/K08). 구버전 노드가
		// 미보고하면 빈값/0 으로 저장되어 회귀가 없다(REQ-K09).
		node := storage.ManagedNode{
			InstanceID:    p.InstanceID,
			Hostname:      p.Hostname,
			Version:       p.Version,
			Status:        RegStatusPending,
			Online:        true,
			LastSeen:      time.Now().UnixMilli(),
			OS:            p.OS,
			Arch:          p.Arch,
			StartedAt:     p.StartedAt,
			DisplayWidth:  p.DisplayWidth,
			DisplayHeight: p.DisplayHeight,
		}
		if upErr := s.repo.Upsert(ctx, node); upErr != nil {
			s.logger.Error("등록 pending 저장 실패", "instance_id", p.InstanceID, "error", upErr)
			return "", nil, false
		}
		// 최초 등록 → 초기 버전을 이력에 기록한다(prev="" — 버전 관리 Phase 1).
		s.recordVersionChange(ctx, p.InstanceID, "", p.Version)
		s.setNodeState(p.InstanceID, RegStatusPending, true, time.Now())
		owned := s.registerConn(p.InstanceID, conn, cancel)
		s.sendRegisterAck(conn, RegisterAckPayload{Status: RegStatusPending})
		s.logger.Info("관리 노드 등록 요청 → pending", "instance_id", p.InstanceID)
		s.recordLifecycleAudit(ctx, p.InstanceID, storage.AuditActionRegister, RegStatusPending)
		return p.InstanceID, owned, true

	case err != nil:
		s.logger.Error("등록 상태 조회 실패", "instance_id", p.InstanceID, "error", err)
		return "", nil, false

	default:
		// 기존 노드 → 현재 상태에 따라 응답(자동 상태 변경 없음 — REQ-C06).
		s.updateNodeMeta(ctx, p)
		s.setNodeState(p.InstanceID, existing.Status, true, time.Now())
		owned := s.registerConn(p.InstanceID, conn, cancel)
		ack := RegisterAckPayload{Status: existing.Status}
		if existing.Status == RegStatusApproved {
			// 토큰 없이 재접속한 approved 노드 → 새 토큰을 발급해 전달(REQ-C04/C05).
			if tok, isErr := s.issueAndStoreToken(ctx, p.InstanceID); isErr == nil {
				ack.NodeToken = tok
				// 경로 A(사전 등록 자동 승인, REQ-H02): approved 이지만 토큰이 미발급이던
				// 노드(관리자가 사전 생성)가 처음 접속해 토큰을 받은 경우다. 정상 재접속
				// (이미 token_id 보유)과 구분해 자동 승인 1건만 감사 기록한다.
				if existing.TokenID == "" {
					s.recordPreApprovedAudit(ctx, p.InstanceID)
					s.logger.Info("사전 등록 노드 접속 — 자동 승인",
						"instance_id", p.InstanceID)
				}
			}
		}
		s.sendRegisterAck(conn, ack)
		s.logger.Info("관리 노드 재등록 요청", "instance_id", p.InstanceID, "status", existing.Status)
		return p.InstanceID, owned, true
	}
}

// updateNodeMeta 는 register 시 hostname/version 메타를 갱신한다(상태·group_name 보존).
//
// v1.4(M9): BASIC 시스템 정보(os/arch/started_at)는 SetSystemInfo 로 제공된 필드만
// 갱신한다(REQ-K08). Upsert 는 group_name/os/arch/started_at 을 ON CONFLICT 에서 보존
// 하므로(관리자/시스템 소유), 시스템 정보 갱신은 별도 경로(SetSystemInfo)로 수행한다.
func (s *Server) updateNodeMeta(ctx context.Context, p RegisterPayload) {
	node, err := s.repo.Get(ctx, p.InstanceID)
	if err != nil {
		return
	}
	prevVersion := node.Version
	node.Hostname = p.Hostname
	node.Version = p.Version
	node.Online = true
	node.LastSeen = time.Now().UnixMilli()
	if upErr := s.repo.Upsert(ctx, node); upErr != nil {
		s.logger.Debug("노드 메타 갱신 실패", "instance_id", p.InstanceID, "error", upErr)
	}
	// 버전이 직전 저장값과 달라졌으면 이력에 기록한다(버전 관리 Phase 1).
	s.recordVersionChange(ctx, p.InstanceID, prevVersion, p.Version)
	// 재기동 register 의 시스템 정보(started_at 등) + 노드 해상도를 제공 시에만 갱신한다
	// (REQ-K08/K09/M01/M03). 미제공 필드는 SetSystemInfo 가 기존값을 보존한다(preserve-on-omit).
	s.storeSystemInfo(ctx, p.InstanceID, p.OS, p.Arch, p.StartedAt, p.DisplayWidth, p.DisplayHeight)
}

// storeSystemInfo 는 노드가 보고한 BASIC 시스템 정보 + 노드 해상도를 저장한다(v1.4 M9
// / v1.6 M11, REQ-K08/M01/M02). 모든 필드가 비어 있으면(구버전 노드 — 미보고) no-op 으로
// 회귀를 피한다(REQ-K09/M03). repo 미구성(M1 모드)에서도 안전하게 무시된다. 제공된 필드만
// 갱신하고 미제공(빈값/0)은 기존값을 보존한다(preserve-on-omit — SetSystemInfo).
func (s *Server) storeSystemInfo(ctx context.Context, instanceID, osName, arch string, startedAtMs int64, displayWidth, displayHeight int) {
	if s.repo == nil {
		return
	}
	if osName == "" && arch == "" && startedAtMs == 0 && displayWidth == 0 && displayHeight == 0 {
		return // 미보고(구버전 노드) — 보존, 회귀 0.
	}
	if err := s.repo.SetSystemInfo(ctx, instanceID, osName, arch, startedAtMs, displayWidth, displayHeight); err != nil {
		s.logger.Debug("시스템 정보 저장 생략", "instance_id", instanceID, "error", err)
	}
}

// recordVersionChange 는 노드 버전이 직전 저장값과 달라졌을 때 이력에 한 줄 append 하고,
// (감사 저장소가 있으면) system actor 로 버전 변경 감사를 남긴다(버전 관리 Phase 1).
//
// verHist 미구성이거나 newVersion 이 비었거나 prev==new 이면 no-op 이다(하위 호환).
// 호출 측은 stored version 을 덮어쓰기 *전*의 prevVersion 을 전달해야 한다.
func (s *Server) recordVersionChange(ctx context.Context, instanceID, prevVersion, newVersion string) {
	if s.verHist == nil || instanceID == "" || newVersion == "" {
		return
	}
	if prevVersion == newVersion {
		return
	}
	now := time.Now().UnixMilli()
	if err := s.verHist.Append(ctx, instanceID, newVersion, now); err != nil {
		s.logger.Warn("버전 이력 기록 실패", "instance_id", instanceID, "error", err)
		return
	}
	if s.audit != nil {
		reason := "최초 " + newVersion
		if prevVersion != "" {
			reason = prevVersion + " -> " + newVersion
		}
		if err := s.audit.Append(ctx, storage.RemoteAuditRecord{
			InstanceID: instanceID,
			Actor:      "system",
			Action:     storage.AuditActionVersionUpdate,
			Result:     storage.AuditResultOK,
			Reason:     reason,
			Timestamp:  now,
		}); err != nil {
			s.logger.Debug("버전 변경 감사 기록 생략", "instance_id", instanceID, "error", err)
		}
	}
	s.logger.Info("노드 버전 변경 기록",
		"instance_id", instanceID, "from", prevVersion, "to", newVersion)
}

// NodeVersionHistory 는 노드의 버전 변경 이력을 최신순으로 반환한다(버전 관리 Phase 1).
// limit <= 0 이면 전체. VersionHistory 저장소가 미구성이면 빈 슬라이스를 반환한다.
func (s *Server) NodeVersionHistory(ctx context.Context, instanceID string, limit int) ([]storage.NodeVersionHistory, error) {
	if s.verHist == nil {
		return nil, nil
	}
	return s.verHist.List(ctx, instanceID, limit)
}

// restoreSession 은 토큰이 검증된 재접속 노드의 관리 세션을 복원한다(REQ-C05).
// repo 상태가 approved 인 경우에만 복원하며, 복원 시 이번 연결의 세대 핸들
// (*nodeConn)을 반환한다(소유권 추적용 — 호출 goroutine 의 teardown 이 비교에 사용).
// 복원하지 않으면 (nil, false) 를 반환한다.
func (s *Server) restoreSession(_ context.Context, conn Conn, cancel context.CancelFunc, instanceID string) (*nodeConn, bool) {
	if s.repo == nil {
		return nil, false
	}
	node, err := s.repo.Get(context.Background(), instanceID)
	if err != nil {
		s.logger.Warn("재접속 노드 미등록 — 세션 복원 거부", "instance_id", instanceID)
		// 토큰은 유효했으나 항목이 없다 — 노드가 토큰을 버리고 다시 등록해야 풀린다
		// (@SPEC:SPEC-REMOTE-HELLO-GATE-001 자가 복구). 사유 없이 끊으면 노드는
		// 같은 토큰으로 영원히 재접속만 되풀이한다.
		s.sendHelloNack(conn, HelloNackUnregistered)
		return nil, false
	}
	if node.Status != RegStatusApproved {
		s.logger.Warn("재접속 노드 비승인 — 세션 복원 거부",
			"instance_id", instanceID, "status", node.Status)
		s.sendHelloNack(conn, HelloNackNotApproved)
		return nil, false
	}
	now := time.Now()
	// 라이브 연결을 먼저 등록한 뒤 managed(approved+online) 상태로 전이한다. 이 순서는
	// "IsManaged==true ⇒ connFor 성공" 불변을 보장하여, 디스패치/스트림/브리지 호출자가
	// IsManaged 통과 후 connFor 가 비어 있는 경합(no live connection)을 보지 않게 한다.
	owned := s.registerConn(instanceID, conn, cancel)
	s.setNodeState(instanceID, RegStatusApproved, true, now)
	s.mu.Lock()
	if st := s.nodes[instanceID]; st != nil {
		st.Hostname = node.Hostname
		st.Version = node.Version
	}
	s.mu.Unlock()
	s.persistOnline(instanceID, true, now)
	s.logger.Info("승인 노드 재접속 — 세션 복원", "instance_id", instanceID)
	s.recordLifecycleAudit(context.Background(), instanceID, storage.AuditActionConnect, "restore")
	return owned, true
}

// Approve 는 pending 노드를 승인한다(REQ-C03/C04). approved 전이 + 노드 토큰 발급
// + token_id 저장 후, 노드가 연결되어 있으면 approved register_ack(토큰 포함)를
// 전송한다.
func (s *Server) Approve(ctx context.Context, instanceID string) error {
	if s.repo == nil {
		return ErrNoRepo
	}
	if _, err := s.repo.Get(ctx, instanceID); err != nil {
		return err // ErrManagedNodeNotFound 포함.
	}
	if err := s.repo.UpdateStatus(ctx, instanceID, RegStatusApproved); err != nil {
		return err
	}

	token, err := s.issueAndStoreToken(ctx, instanceID)
	if err != nil {
		return err
	}

	s.setNodeState(instanceID, RegStatusApproved, s.isOnline(instanceID), time.Now())

	// 연결되어 있으면 approved ack(토큰 포함)를 push 한다(REQ-C04).
	if nc, ok := s.connFor(instanceID); ok {
		s.sendRegisterAck(nc.conn, RegisterAckPayload{
			Status:    RegStatusApproved,
			NodeToken: token,
		})
	}
	s.logger.Info("관리 노드 승인", "instance_id", instanceID)
	return nil
}

// Reject 는 pending 노드를 거부한다(REQ-C03). rejected 전이 + (연결 시) rejected ack.
func (s *Server) Reject(ctx context.Context, instanceID, reason string) error {
	if s.repo == nil {
		return ErrNoRepo
	}
	if _, err := s.repo.Get(ctx, instanceID); err != nil {
		return err
	}
	if err := s.repo.UpdateStatus(ctx, instanceID, RegStatusRejected); err != nil {
		return err
	}
	s.setNodeState(instanceID, RegStatusRejected, s.isOnline(instanceID), time.Now())

	if nc, ok := s.connFor(instanceID); ok {
		s.sendRegisterAck(nc.conn, RegisterAckPayload{
			Status: RegStatusRejected,
			Reason: reason,
		})
	}
	s.logger.Info("관리 노드 거부", "instance_id", instanceID)
	return nil
}

// Revoke 는 승인된 노드를 폐기한다(REQ-C07/F07). 노드 토큰 blacklist + revoked
// 전이 + 연결 종료. 이후 그 토큰의 재인증은 거부된다.
func (s *Server) Revoke(ctx context.Context, instanceID string) error {
	if s.repo == nil {
		return ErrNoRepo
	}
	node, err := s.repo.Get(ctx, instanceID)
	if err != nil {
		return err
	}

	// 토큰 즉시 무효화(REQ-F07). token_id 에는 jti 가 저장되어 있으므로(M6 하드닝),
	// 원본 토큰 없이 jti 만으로 폐기한다(DB-안전 폐기).
	if node.TokenID != "" && s.tokens != nil {
		s.tokens.RevokeID(node.TokenID)
	}

	if err := s.repo.UpdateStatus(ctx, instanceID, RegStatusRevoked); err != nil {
		return err
	}
	s.setNodeState(instanceID, RegStatusRevoked, false, time.Now())

	// 라이브 연결 종료(REQ-C07).
	if nc, ok := s.connFor(instanceID); ok {
		nc.cancel()
		_ = nc.conn.Close()
	}
	s.logger.Info("관리 노드 폐기", "instance_id", instanceID)
	return nil
}

// issueAndStoreToken 은 instance_id 용 노드 토큰을 발급하고 token_id 를 저장한다.
//
// M6 하드닝(REQ-F02/F07): 원본 토큰이 아닌 jti(토큰 식별자)를 token_id 로 저장한다.
// 이로써 DB 가 유출되어도 사용 가능한 베어러 토큰이 노출되지 않으며, 폐기는 jti 로
// 수행된다(RevokeID). 원본 토큰은 register_ack 로 노드에만 전달되고 서버에 남지 않는다.
func (s *Server) issueAndStoreToken(ctx context.Context, instanceID string) (string, error) {
	if s.tokens == nil {
		return "", errors.New("remote: token issuer not configured")
	}
	token, jti, err := s.tokens.IssueWithID(instanceID, "node")
	if err != nil {
		return "", err
	}
	// 원본 토큰이 아닌 jti 만 저장한다(DB-안전 — M6).
	if err := s.repo.SetToken(ctx, instanceID, jti); err != nil {
		return "", err
	}
	return token, nil
}

// isOnline 은 노드의 현재 online 여부를 반환한다.
func (s *Server) isOnline(instanceID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.nodes[instanceID]
	return ok && st.Online
}

// sendRegisterAck 는 register_ack 메시지를 전송한다. 전송 실패는 로깅만 한다.
func (s *Server) sendRegisterAck(conn Conn, p RegisterAckPayload) {
	msg, err := NewRegisterAckMessage(p)
	if err != nil {
		s.logger.Error("register_ack 인코딩 실패", "error", err)
		return
	}
	if err := writeEnvelope(conn, msg); err != nil {
		s.logger.Debug("register_ack 전송 실패", "error", err)
	}
}
