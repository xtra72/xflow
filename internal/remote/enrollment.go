// enrollment.go 는 수동 enrollment(그룹 H)의 서버 측 로직을 구현한다
// (@SPEC:SPEC-REMOTE-001 v1.1, REQ-REMOTE-H01/H02/H05/H08).
//
// 두 가지 수동 가입 경로를 제공한다:
//
//	경로 A (사전 등록): 관리자가 노드 접속 전에 instance_id 를 approved 로 미리 생성한다
//	  (PreRegister). 해당 노드가 처음 register 하면(토큰 미발급) 관리자 개입 없이 즉시
//	  토큰을 발급받는다(handleRegister 의 approved-without-token 분기).
//
//	경로 B (enrollment 토큰): 관리자가 enrollment 토큰을 발급하고(handler), 노드는
//	  register 에 토큰을 실어 보낸다. 서버는 토큰을 검증(해시 일치·미만료·미폐기·미소진)
//	  하여 유효하면 신규 노드를 즉시 승인·토큰 발급하고 uses 를 증가시킨다. 무효/만료/
//	  소진/폐기 토큰은 pending 으로 폴백한다(보수적 — 관리자 결정 위임).
//
// 보안: enrollment 토큰 원본은 저장/로깅하지 않는다(SHA-256 해시 비교만 — REQ-H06).
// 토큰 기반 자동 승인 감사는 enrollment 토큰 id 만 기록한다(원본 토큰 금지 — REQ-H08).
package remote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/xtra/xflow/internal/storage"
)

// ErrManagedNodeExists 는 이미 존재하는 instance_id 로 사전 등록을 시도할 때 반환된다
// (REQ-H01 — 중복 사전 등록 거부, 409 매핑).
var ErrManagedNodeExists = errors.New("remote: managed node already exists")

// HashEnrollmentToken 은 원본 enrollment 토큰의 SHA-256 16진 해시를 계산한다(REQ-H06).
// 저장·비교는 항상 이 해시로만 수행하며 원본 토큰은 보관하지 않는다. handler 의 발급
// 경로와 서버의 검증 경로가 동일 규약을 공유하도록 노출한다.
func HashEnrollmentToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// PreRegister 는 노드 접속 전에 instance_id 를 approved 로 사전 생성한다(경로 A, REQ-H01).
//
// online=false, 토큰 미발급 상태로 생성한다 — 실제 토큰은 노드가 접속해 register 할 때
// 자동 승인 분기에서 발급된다. 동일 instance_id 가 이미 있으면 ErrManagedNodeExists 를
// 반환한다(상태 무관 — 중복 방지).
func (s *Server) PreRegister(ctx context.Context, instanceID, name string) error {
	if s.repo == nil {
		return ErrNoRepo
	}
	if instanceID == "" {
		return errors.New("remote: instance_id is required")
	}
	if _, err := s.repo.Get(ctx, instanceID); err == nil {
		return ErrManagedNodeExists
	} else if !errors.Is(err, storage.ErrManagedNodeNotFound) {
		return err
	}

	now := time.Now().UnixMilli()
	node := storage.ManagedNode{
		InstanceID: instanceID,
		Hostname:   name, // 사전 등록 라벨 — 접속 시 실제 hostname 으로 갱신된다.
		Status:     RegStatusApproved,
		Online:     false,
		LastSeen:   now,
	}
	if err := s.repo.Upsert(ctx, node); err != nil {
		return err
	}
	s.setNodeState(instanceID, RegStatusApproved, false, time.Now())
	s.logger.Info("관리 노드 사전 등록(approved)", "instance_id", instanceID)
	return nil
}

// RemoveNode 는 노드 등록을 완전히 제거한다(경로 A 삭제, REQ-H01).
//
// 라이브 연결이 있으면 종료하고, 노드 토큰이 있으면 폐기한 뒤(재인증 차단 — REQ-F07),
// 영속 행을 삭제한다. 미존재 시 storage.ErrManagedNodeNotFound 를 반환한다.
func (s *Server) RemoveNode(ctx context.Context, instanceID string) error {
	if s.repo == nil {
		return ErrNoRepo
	}
	node, err := s.repo.Get(ctx, instanceID)
	if err != nil {
		return err // ErrManagedNodeNotFound 포함.
	}

	// 토큰 즉시 폐기(jti 기반 — 원본 토큰 불필요, REQ-F07).
	if node.TokenID != "" && s.tokens != nil {
		s.tokens.RevokeID(node.TokenID)
	}

	// 라이브 연결 종료.
	if nc, ok := s.connFor(instanceID); ok {
		nc.cancel()
		_ = nc.conn.Close()
	}

	if err := s.repo.Delete(ctx, instanceID); err != nil {
		return err
	}
	s.dropNodeState(instanceID)
	s.logger.Info("관리 노드 삭제", "instance_id", instanceID)
	return nil
}

// dropNodeState 는 in-memory 노드 상태를 제거한다(RemoveNode 정리용).
func (s *Server) dropNodeState(instanceID string) {
	s.mu.Lock()
	delete(s.nodes, instanceID)
	s.mu.Unlock()
}

// tryEnrollmentAutoApprove 는 register 의 enrollment 토큰을 검증하고, 유효하면 신규
// 노드를 즉시 승인·토큰 발급한 뒤 approved register_ack(토큰 포함)를 전송한다(경로 B).
//
// 반환 handled=true 이면 본 함수가 등록을 완결했으므로 호출자는 pending 흐름을 건너뛴다.
// 토큰이 없거나 무효/만료/폐기/소진이면 handled=false 를 반환하여 호출자가 pending 으로
// 폴백하게 한다(보수적 invalid-token 정책 — REQ-H05).
//
// 동시 register 경합: IncrementUses 는 max_uses 조건부 원자 증가이며, 소진 경합에서
// 패자는 ErrEnrollmentTokenExhausted 를 받아 pending 으로 폴백한다(over-issue 방지 —
// REQ-H07). 승인 전에 uses 를 증가시켜, 증가 성공한 호출자만 토큰을 발급한다.
func (s *Server) tryEnrollmentAutoApprove(ctx context.Context, conn Conn, p RegisterPayload) bool {
	if s.enroll == nil || p.EnrollmentToken == "" {
		return false
	}

	tok, err := s.enroll.GetByHash(ctx, HashEnrollmentToken(p.EnrollmentToken))
	if err != nil {
		// 미존재(무효) 또는 조회 오류 — 보수적으로 pending 폴백(토큰 값 미로깅 — REQ-H06).
		s.logger.Warn("enrollment 토큰 검증 실패 — pending 폴백", "instance_id", p.InstanceID)
		return false
	}
	if !enrollmentTokenUsable(tok, time.Now().UnixMilli()) {
		s.logger.Warn("enrollment 토큰 사용 불가(만료/폐기/소진) — pending 폴백",
			"instance_id", p.InstanceID, "token_id", tok.ID)
		return false
	}

	// 사용 횟수를 먼저 원자적으로 소비한다(경합 패자는 소진으로 폴백 — REQ-H07).
	if incErr := s.enroll.IncrementUses(ctx, tok.ID); incErr != nil {
		s.logger.Warn("enrollment 토큰 소비 실패 — pending 폴백",
			"instance_id", p.InstanceID, "token_id", tok.ID)
		return false
	}

	// 신규 노드를 approved 로 생성(또는 갱신)한다. 최초 register 의 BASIC 시스템 정보
	// (os/arch/started_at)를 함께 저장한다(v1.4 M9, REQ-K07/K08, 미보고 시 빈값 — K09).
	now := time.Now().UnixMilli()
	node := storage.ManagedNode{
		InstanceID: p.InstanceID,
		Hostname:   p.Hostname,
		Version:    p.Version,
		Status:     RegStatusApproved,
		Online:     true,
		LastSeen:   now,
		OS:         p.OS,
		Arch:       p.Arch,
		StartedAt:  p.StartedAt,
	}
	if upErr := s.repo.Upsert(ctx, node); upErr != nil {
		s.logger.Error("enrollment 자동 승인 저장 실패",
			"instance_id", p.InstanceID, "error", upErr)
		return false
	}

	token, issErr := s.issueAndStoreToken(ctx, p.InstanceID)
	if issErr != nil {
		s.logger.Error("enrollment 자동 승인 토큰 발급 실패",
			"instance_id", p.InstanceID, "error", issErr)
		return false
	}

	s.setNodeState(p.InstanceID, RegStatusApproved, true, time.Now())
	s.sendRegisterAck(conn, RegisterAckPayload{Status: RegStatusApproved, NodeToken: token})

	// 감사(REQ-H08): enrollment 토큰 id 만 기록한다(원본 토큰 절대 금지). Actor 는
	// 토큰 출처를 나타내고, Reason 에 token_id 를 남겨 추적성을 제공한다.
	s.recordEnrollmentAudit(ctx, p.InstanceID, tok.ID)
	s.logger.Info("enrollment 토큰 자동 승인",
		"instance_id", p.InstanceID, "token_id", tok.ID)
	return true
}

// enrollmentTokenUsable 은 토큰이 현재 사용 가능한지(미폐기·미만료·미소진) 판정한다.
func enrollmentTokenUsable(tok storage.EnrollmentToken, nowMs int64) bool {
	if tok.Revoked {
		return false
	}
	if tok.ExpiresAt != nil && nowMs >= *tok.ExpiresAt {
		return false
	}
	if tok.MaxUses != nil && tok.Uses >= *tok.MaxUses {
		return false
	}
	return true
}

// recordEnrollmentAudit 는 enrollment 토큰 기반 자동 승인을 감사 기록한다(REQ-H08).
// 원본 토큰은 절대 포함하지 않으며 enrollment 토큰 id 만 남긴다.
func (s *Server) recordEnrollmentAudit(ctx context.Context, instanceID, tokenID string) {
	if s.audit == nil {
		return
	}
	rec := storage.RemoteAuditRecord{
		InstanceID: instanceID,
		Actor:      "enrollment-token:" + tokenID,
		Action:     storage.AuditActionApprove,
		Result:     storage.AuditResultOK,
		Reason:     "enrollment_token:" + tokenID,
		Timestamp:  time.Now().UnixMilli(),
	}
	if err := s.audit.Append(ctx, rec); err != nil {
		s.logger.Warn("enrollment 자동 승인 감사 기록 실패",
			"instance_id", instanceID, "error", err)
	}
}

// recordPreApprovedAudit 는 사전 등록 노드의 접속 시 자동 승인을 감사 기록한다(REQ-H02/H08).
func (s *Server) recordPreApprovedAudit(ctx context.Context, instanceID string) {
	if s.audit == nil {
		return
	}
	rec := storage.RemoteAuditRecord{
		InstanceID: instanceID,
		Actor:      "admin",
		Action:     storage.AuditActionApprove,
		Result:     storage.AuditResultOK,
		Reason:     "pre_registration",
		Timestamp:  time.Now().UnixMilli(),
	}
	if err := s.audit.Append(ctx, rec); err != nil {
		s.logger.Warn("사전 등록 자동 승인 감사 기록 실패",
			"instance_id", instanceID, "error", err)
	}
}
