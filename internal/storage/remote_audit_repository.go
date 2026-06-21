// remote_audit_repository.go 는 원격 관리 변경 감사(audit) 로그 저장소 인터페이스를
// 정의한다(@SPEC:SPEC-REMOTE-001 M6, REQ-F05/F06).
//
// 감사 로그는 원격 관리 mutation(승인/거부/폐기/명령)을 누가/언제/어느 노드/무엇/
// 결과로 기록한다. append-only(추가 전용)이며, 시크릿(명령 인자/토큰/페이로드)은
// 절대 저장하지 않는다(REQ-F06 — domain/action/result-status 만 기록).
package storage

import (
	"context"
	"errors"
)

// 감사 액션 상수(managed node mutation 종류).
const (
	// AuditActionApprove 는 노드 승인이다(REQ-C03).
	AuditActionApprove = "approve"
	// AuditActionReject 는 노드 거부이다(REQ-C03).
	AuditActionReject = "reject"
	// AuditActionRevoke 는 노드 폐기이다(REQ-C07).
	AuditActionRevoke = "revoke"
	// AuditActionCommand 는 원격 명령 발행이다(REQ-D01).
	AuditActionCommand = "command"
	// AuditActionVersionUpdate 는 노드 버전 변경 관측이다(버전 관리 Phase 1).
	// 노드가 보고한 version 이 직전 저장값과 달라질 때 system actor 로 기록한다.
	AuditActionVersionUpdate = "version_update"
)

// 감사 결과 상수.
const (
	// AuditResultOK 는 mutation 성공이다.
	AuditResultOK = "ok"
	// AuditResultError 는 mutation 실패이다.
	AuditResultError = "error"
)

// ErrRemoteAuditClosed 는 닫힌 저장소에 쓰기를 시도할 때 반환된다.
var ErrRemoteAuditClosed = errors.New("remote audit repository closed")

// RemoteAuditRecord 는 단일 원격 관리 감사 레코드이다(spec §4.6 REQ-F05).
//
// 시크릿 금지(REQ-F06): Action(approve/reject/revoke/command)과 명령의 경우
// Domain(flow/agent/device) + CommandAction(deploy/start/...) 만 기록한다. 명령
// 인자(Args)·토큰·페이로드는 절대 포함하지 않는다. Reason 은 거부 사유 등 비밀이
// 아닌 짧은 텍스트만 담는다.
//
// Timestamp 는 epoch milliseconds(int64) 이다(프로젝트 규약).
type RemoteAuditRecord struct {
	ID            int64  // 자동 증가 PK(조회 시 채워짐)
	InstanceID    string // 대상 노드 instance_id
	Actor         string // 수행 관리자 username(인증 Claims 출처)
	Action        string // approve | reject | revoke | command
	Domain        string // command 일 때 flow|agent|device (그 외 빈 값)
	CommandAction string // command 일 때 도메인 액션(deploy/start/...) (그 외 빈 값)
	Result        string // ok | error
	Reason        string // 선택적 비밀-아님 사유(거부 사유/오류 분류). 시크릿 금지.
	Timestamp     int64  // 수행 시각(epoch ms)
}

// RemoteAuditRepository 는 원격 관리 감사 로그의 영속 저장소이다(spec §4.6 F05).
//
// append-only: 갱신/삭제 API 를 노출하지 않는다(감사 무결성). 조회는 노드별 필터 +
// 페이지네이션을 지원한다(관리 UI 관측성 — admin-gated 엔드포인트).
type RemoteAuditRepository interface {
	// Append 는 감사 레코드를 추가한다(추가 전용 — REQ-F05). 시크릿 미포함을 호출자가
	// 보장한다(domain/action/result 만 — REQ-F06).
	Append(ctx context.Context, rec RemoteAuditRecord) error
	// List 는 감사 레코드를 최신순(ts 내림차순)으로 반환한다. instanceID 가 비어 있지
	// 않으면 해당 노드로 필터한다. limit/offset 으로 페이지네이션한다.
	List(ctx context.Context, instanceID string, limit, offset int) ([]RemoteAuditRecord, error)
	// Close 는 저장소 리소스를 정리한다.
	Close() error
}

// NewRemoteAuditRepository 는 storage type 에 따라 RemoteAuditRepository 구현을
// 생성한다. M6 은 sqlite 만 지원한다(서버 측 감사 저장소).
func NewRemoteAuditRepository(ctx context.Context, storageType, sqlitePath string) (RemoteAuditRepository, error) {
	switch storageType {
	case "sqlite", "file":
		return NewRemoteAuditSQLiteRepository(ctx, sqlitePath)
	default:
		return NewRemoteAuditSQLiteRepository(ctx, sqlitePath)
	}
}
