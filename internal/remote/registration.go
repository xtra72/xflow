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
	// in-memory online 상태를 반영한다(repo 의 online 은 영속 시점 기준이므로 라이브
	// 연결 상태를 우선 적용 — 라이브 추적이 권위).
	s.mu.RLock()
	for i := range nodes {
		if st, ok := s.nodes[nodes[i].InstanceID]; ok {
			nodes[i].Online = st.Online
		}
	}
	s.mu.RUnlock()
	return nodes, nil
}

// registerConn 은 라이브 연결을 추적한다(approve ack push / revoke 종료용).
func (s *Server) registerConn(instanceID string, conn Conn, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns[instanceID] = &nodeConn{conn: conn, cancel: cancel}
}

// unregisterConn 은 라이브 연결 추적을 해제한다(동일 연결인 경우에만).
func (s *Server) unregisterConn(instanceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, instanceID)
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

// handleRegister 는 register 메시지를 처리한다(REQ-C01/C02/C08, spec §5.6).
//
// 반환: (instanceID, handled). 정상 등록 시 instanceID 가 채워진다. 부트스트랩
// 시크릿 불일치 등 거부 시 instanceID="" 이지만 연결은 유지(rejected ack 전송 후
// 클라이언트가 종료하도록).
func (s *Server) handleRegister(ctx context.Context, conn Conn, cancel context.CancelFunc, msg *ws.Message) (string, bool) {
	var p RegisterPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		s.logger.Debug("register 페이로드 디코드 실패", "error", err)
		return "", false
	}
	if p.InstanceID == "" {
		s.logger.Warn("register 에 instance_id 누락 — 무시")
		return "", false
	}

	// repo 미구성(M1 모드)에서는 등록을 처리할 수 없다 — hello 경로만 지원.
	if s.repo == nil {
		s.logger.Warn("repo 미구성 — register 무시(M1 모드)", "instance_id", p.InstanceID)
		return "", false
	}

	// 부트스트랩 시크릿 1차 신뢰 검증(REQ-C08). 구성된 경우에만 강제한다.
	if s.cfg.BootstrapSecret != "" && p.BootstrapSecret != s.cfg.BootstrapSecret {
		s.logger.Warn("부트스트랩 시크릿 불일치 — 등록 거부", "instance_id", p.InstanceID)
		s.sendRegisterAck(conn, RegisterAckPayload{
			Status: RegStatusRejected,
			Reason: "bootstrap secret mismatch",
		})
		return "", true
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
			s.registerConn(p.InstanceID, conn, cancel)
			return p.InstanceID, true
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
			return "", false
		}
		s.setNodeState(p.InstanceID, RegStatusPending, true, time.Now())
		s.registerConn(p.InstanceID, conn, cancel)
		s.sendRegisterAck(conn, RegisterAckPayload{Status: RegStatusPending})
		s.logger.Info("관리 노드 등록 요청 → pending", "instance_id", p.InstanceID)
		return p.InstanceID, true

	case err != nil:
		s.logger.Error("등록 상태 조회 실패", "instance_id", p.InstanceID, "error", err)
		return "", false

	default:
		// 기존 노드 → 현재 상태에 따라 응답(자동 상태 변경 없음 — REQ-C06).
		s.updateNodeMeta(ctx, p)
		s.setNodeState(p.InstanceID, existing.Status, true, time.Now())
		s.registerConn(p.InstanceID, conn, cancel)
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
		return p.InstanceID, true
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
	node.Hostname = p.Hostname
	node.Version = p.Version
	node.Online = true
	node.LastSeen = time.Now().UnixMilli()
	if upErr := s.repo.Upsert(ctx, node); upErr != nil {
		s.logger.Debug("노드 메타 갱신 실패", "instance_id", p.InstanceID, "error", upErr)
	}
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

// restoreSession 은 토큰이 검증된 재접속 노드의 관리 세션을 복원한다(REQ-C05).
// repo 상태가 approved 인 경우에만 true 를 반환한다.
func (s *Server) restoreSession(_ context.Context, conn Conn, cancel context.CancelFunc, instanceID string) bool {
	if s.repo == nil {
		return false
	}
	node, err := s.repo.Get(context.Background(), instanceID)
	if err != nil {
		s.logger.Warn("재접속 노드 미등록 — 세션 복원 거부", "instance_id", instanceID)
		return false
	}
	if node.Status != RegStatusApproved {
		s.logger.Warn("재접속 노드 비승인 — 세션 복원 거부",
			"instance_id", instanceID, "status", node.Status)
		return false
	}
	now := time.Now()
	// 라이브 연결을 먼저 등록한 뒤 managed(approved+online) 상태로 전이한다. 이 순서는
	// "IsManaged==true ⇒ connFor 성공" 불변을 보장하여, 디스패치/스트림/브리지 호출자가
	// IsManaged 통과 후 connFor 가 비어 있는 경합(no live connection)을 보지 않게 한다.
	s.registerConn(instanceID, conn, cancel)
	s.setNodeState(instanceID, RegStatusApproved, true, now)
	s.mu.Lock()
	if st := s.nodes[instanceID]; st != nil {
		st.Hostname = node.Hostname
		st.Version = node.Version
	}
	s.mu.Unlock()
	s.persistOnline(instanceID, true, now)
	s.logger.Info("승인 노드 재접속 — 세션 복원", "instance_id", instanceID)
	return true
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
