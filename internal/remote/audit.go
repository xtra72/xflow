// audit.go 는 원격 변경 감사(audit) 영속화 배선을 담당한다
// (@SPEC:SPEC-REMOTE-001 M6, REQ-F05/F06).
//
// 명령 디스패치(dispatch.go)의 actor(관리자 username)는 핸들러가 컨텍스트에 실어
// 전달한다(ContextWithActor). 이렇게 하면 Dispatch 시그니처를 바꾸지 않고도(하위
// 호환) 감사에 actor 를 기록할 수 있다. 감사 저장소는 storage.RemoteAuditRepository
// 이며, remote 패키지는 이미 storage 를 import 하므로 사이클이 없다.
//
// 시크릿 금지(REQ-F06): 감사에는 domain/action/result 만 기록하고, 명령 인자(Args)·
// 토큰·페이로드는 절대 기록하지 않는다.
package remote

import (
	"context"
	"time"

	"github.com/xtra/xflow/internal/storage"
)

// actorContextKey 는 컨텍스트에 actor(관리자 username)를 싣는 키이다.
type actorContextKey struct{}

// ContextWithActor 는 ctx 에 actor(관리자 username)를 부착한다. 핸들러가 Dispatch
// 호출 전 인증 Claims 의 username 을 실어 보낸다(감사 actor 기록용).
func ContextWithActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, actorContextKey{}, actor)
}

// actorFromContext 는 ctx 의 actor 를 반환한다. 없으면 빈 문자열(시스템/미상 출처).
func actorFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(actorContextKey{}).(string); ok {
		return v
	}
	return ""
}

// ActorFromContext 는 ctx 에 부착된 actor(관리자 username)를 반환한다(공개 reader).
// 없으면 빈 문자열이다. 핸들러/소비자가 actor 전파를 확인하는 데 사용한다.
func ActorFromContext(ctx context.Context) string {
	return actorFromContext(ctx)
}

// recordLifecycleAudit 는 노드 생애 사건(연결·끊어짐·등록)을 감사 저장소에 기록한다
// (@SPEC:SPEC-REMOTE-LOG-001).
//
// 이 사건들의 주체는 사람이 아니라 서버가 관측한 사실이므로 actor 는 system 이다.
// reason 에는 비밀이 아닌 짧은 분류만 담는다(REQ-F06 — 토큰·페이로드 금지).
//
// 기록 실패는 연결 경로를 막지 않는다. 감사가 끊겼다고 노드를 끊으면 관측 장치가
// 서비스의 가용성을 끌어내린다.
func (s *Server) recordLifecycleAudit(ctx context.Context, instanceID, action, reason string) {
	if s.audit == nil {
		return
	}
	rec := storage.RemoteAuditRecord{
		InstanceID: instanceID,
		Actor:      storage.AuditActorSystem,
		Action:     action,
		Result:     storage.AuditResultOK,
		Reason:     reason,
		Timestamp:  time.Now().UnixMilli(),
	}
	if err := s.audit.Append(ctx, rec); err != nil {
		s.logger.Warn("감사 레코드 기록 실패",
			"instance_id", instanceID, "action", action, "error", err)
	}
}

// recordActorAudit 는 사람이 일으킨 사건(삭제·원격 접속)을 감사 저장소에 기록한다
// (@SPEC:SPEC-REMOTE-LOG-001). actor 는 ctx 에서 꺼낸다 — 핸들러가 ContextWithActor
// 로 넣지 않았으면 빈 값이 되며, 그 경우에도 사건 자체는 남긴다(주체 미상 기록이
// 기록 없음보다 낫다).
func (s *Server) recordActorAudit(ctx context.Context, instanceID, action, reason string) {
	s.recordAuditAs(ctx, instanceID, action, actorFromContext(ctx), reason)
}

// recordAuditAs 는 주체를 **인자로 받아** 기록한다.
//
// 주체가 ctx 에 실려 오지 않는 경로가 있다 — 핸들러가 `ctx.UserID()` 를 꺼내 그대로
// 넘기는 TouchAccess 가 그렇다. 그 경로에서 ctx 에서 주체를 다시 꺼내려 하면 빈
// 문자열이 되어, 접속 로그가 "누가" 없이 쌓인다. 주체를 아는 쪽이 넘기게 한다.
func (s *Server) recordAuditAs(ctx context.Context, instanceID, action, actor, reason string) {
	if s.audit == nil {
		return
	}
	rec := storage.RemoteAuditRecord{
		InstanceID: instanceID,
		Actor:      actor,
		Action:     action,
		Result:     storage.AuditResultOK,
		Reason:     reason,
		Timestamp:  time.Now().UnixMilli(),
	}
	if err := s.audit.Append(ctx, rec); err != nil {
		s.logger.Warn("감사 레코드 기록 실패",
			"instance_id", instanceID, "action", action, "error", err)
	}
}

// recordCommandAudit 는 명령 디스패치 결과를 감사 저장소에 기록한다(REQ-F05).
// audit 미구성이면 no-op. 시크릿(Args/토큰)은 기록하지 않는다(REQ-F06).
func (s *Server) recordCommandAudit(ctx context.Context, instanceID, domain, action, result, reason string) {
	if s.audit == nil {
		return
	}
	rec := storage.RemoteAuditRecord{
		InstanceID:    instanceID,
		Actor:         actorFromContext(ctx),
		Action:        storage.AuditActionCommand,
		Domain:        domain,
		CommandAction: action,
		Result:        result,
		Reason:        reason,
		Timestamp:     time.Now().UnixMilli(),
	}
	// 감사 기록 실패는 명령 경로를 막지 않는다(구조화 로그로 보강).
	if err := s.audit.Append(ctx, rec); err != nil {
		s.logger.Warn("감사 레코드 기록 실패",
			"instance_id", instanceID, "action", action, "error", err)
	}
}
