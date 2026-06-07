// editing.go 는 M7 서버 측 원격 자원 편집 보조 메서드를 구현한다
// (@SPEC:SPEC-REMOTE-001 M7, spec §4.7c 그룹 I, REQ-I05/I11/E08/A4).
//
// 원격 편집은 핸들러(internal/api/handler/remote_editing.go)가 오케스트레이션한다:
//
//  1. (update/delete) IsResourceExposed 로 대상 자원이 노드의 노출 범위 내인지 확인
//     한다(REQ-I05 — 범위 밖 편집 거부). 노출 범위는 미러 캐시 존재 여부로 판정한다
//     (REQ-E07 — 서버는 노출된 자원만 미러로 본다).
//  2. Dispatch(그룹 D)로 명령을 (온라인) 노드에 전파한다. 승인+온라인 게이트, 타임아웃
//     (504), 적용 실패(502), 미관리/오프라인(503) 매핑과 감사(F05)는 Dispatch 가 담당.
//  3. 명령 결과 수신 성공 후에만 UpsertMirror/DeleteMirror 로 미러 캐시를 갱신한다
//     (REQ-E08/A4 — 서버 단독 영속 금지; 어떤 실패에서도 캐시는 갱신되지 않음).
//
// 생성(create)은 자동 노출되지 않으므로(opt-in 보존 — spec §5.9 OPEN QUESTION 9)
// 핸들러가 미러를 강제 채우지 않는다. 따라서 본 파일의 mutator 는 update(upsert)/
// delete(remove)에서만 호출된다.
//
// redaction 책임 분리(REQ-F06): 본 패키지는 handler 의 RedactSensitiveConfig 를
// import 할 수 없으므로(import cycle), 미러에 쓸 redacted 정의는 핸들러(handler 패키지)
// 가 구성하여 storage.MirroredResource 로 전달한다. 본 mutator 는 영속만 수행한다.
package remote

import (
	"context"

	"github.com/xtra/xflow/internal/storage"
)

// IsResourceExposed 는 instance_id 의 kind(flow|agent) 자원 id 가 노드의 노출 범위
// 내(=미러 캐시에 존재)인지 반환한다(REQ-I05/E07). mirror 미구성이면 보수적으로
// false 를 반환한다(노출 범위를 알 수 없으면 편집 불가).
func (s *Server) IsResourceExposed(ctx context.Context, instanceID, kind, id string) (bool, error) {
	if s.mirror == nil {
		return false, nil
	}
	var (
		rows []storage.MirroredResource
		err  error
	)
	switch kind {
	case KindFlow:
		rows, err = s.mirror.ListFlows(ctx, instanceID)
	case KindAgent:
		rows, err = s.mirror.ListAgents(ctx, instanceID)
	case KindDevice:
		rows, err = s.mirror.ListDevices(ctx, instanceID)
	default:
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, r := range rows {
		if r.ID == id {
			return true, nil
		}
	}
	return false, nil
}

// UpsertMirror 는 명령 성공 후 단일 미러 행을 추가/갱신한다(REQ-E08 — 결과 수신
// 후에만 캐시 갱신). mirror 미구성이면 no-op(하위 호환). item.Definition 은 호출자가
// redaction(F06)한 정의여야 한다.
func (s *Server) UpsertMirror(ctx context.Context, kind string, item storage.MirroredResource) error {
	if s.mirror == nil {
		return nil
	}
	return s.mirror.UpsertResource(ctx, kind, item)
}

// DeleteMirror 는 명령 성공 후 단일 미러 행을 제거한다(REQ-E08/I03). mirror 미구성
// 이면 no-op(하위 호환).
func (s *Server) DeleteMirror(ctx context.Context, instanceID, kind, id string) error {
	if s.mirror == nil {
		return nil
	}
	return s.mirror.DeleteResource(ctx, instanceID, kind, id)
}
