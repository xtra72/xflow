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

	// --- 운영 이벤트(@SPEC:SPEC-REMOTE-LOG-001) ---
	//
	// 위의 다섯이 "관리자가 무엇을 바꿨는가" 라면, 아래 여섯은 "노드에 무슨 일이
	// 있었는가" 이다. 두 가지를 한 표에 담는 것은 의도한 것이다 — 노드 하나의
	// 시간축을 두 군데로 나눠 놓으면 "언제 끊겼고 그때 누가 무엇을 했는가" 를
	// 읽으려고 두 목록을 손으로 맞춰 봐야 한다.

	// AuditActionRegister 는 노드가 스스로 등록을 요청해 pending 으로 큐잉된 것이다.
	// actor 는 system 이다(사람이 한 일이 아니다).
	AuditActionRegister = "register"
	// AuditActionDelete 는 관리자가 노드 등록 항목 자체를 지운 것이다.
	AuditActionDelete = "delete"
	// AuditActionConnect 는 노드가 관리 서버에 세션을 맺은 것이다(system actor).
	AuditActionConnect = "connect"
	// AuditActionDisconnect 는 그 세션이 끊긴 것이다(system actor).
	AuditActionDisconnect = "disconnect"
	// AuditActionAccess 는 관리자가 노드를 원격 관리하려고 들어간 것이다.
	// 화면을 여는 동안 폴링이 계속되므로 매 요청이 아니라 **접속 한 번당 한 줄**로
	// 묶어 기록한다(창을 열어 둔 채 몇 시간이 지나도 한 줄이다).
	AuditActionAccess = "access"
	// 이미지 업데이트 **지시**는 별도 액션을 두지 않는다 — 기존 명령 디스패치를
	// 그대로 타므로 command + Domain="system" + CommandAction="update" 로 이미 1행이
	// 남는다(성공·실패·타임아웃 모두). 같은 사건에 행을 둘 만들면 목록에서 한 번의
	// 지시가 두 번 일어난 것처럼 읽힌다. 노드가 실제로 새 버전으로 올라온 것은
	// version_update 가 따로 관측한다.
)

// AuditActorSystem 은 사람이 아닌 서버 자신이 남긴 기록의 actor 이다.
const AuditActorSystem = "system"

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

// 정렬 기준 필드 화이트리스트(@SPEC:SPEC-REMOTE-LOG-001).
//
// 정렬 기준을 문자열로 받아 SQL 에 바로 붙이면 주입 통로가 된다. 허용 목록을 두고
// 그 밖의 값은 기본값으로 떨어뜨린다 — 모르는 값을 거절하는 대신 기본으로 읽는
// 이유는, 구버전 화면이 새 필드를 보내도 목록이 비어 보이지 않게 하기 위해서다.
const (
	// AuditSortTime 은 발생 시각 정렬이다(기본).
	AuditSortTime = "ts"
	// AuditSortInstance 는 노드 정렬이다.
	AuditSortInstance = "instance_id"
	// AuditSortActor 는 수행 주체 정렬이다.
	AuditSortActor = "actor"
	// AuditSortAction 은 사건 종류 정렬이다.
	AuditSortAction = "action"
)

// RemoteAuditQuery 는 감사 로그 조회 조건이다(@SPEC:SPEC-REMOTE-LOG-001).
//
// 빈 필드는 "거르지 않음" 이다. 정렬·필터를 화면이 아니라 저장소가 맡는 이유는,
// 화면이 받아 온 쪽 안에서만 정렬하면 "수행자 오름차순" 같은 결과가 전체가 아니라
// 그 쪽에만 적용되어 읽는 사람을 속이기 때문이다.
type RemoteAuditQuery struct {
	InstanceID string // 노드 필터(빈값=전체)
	Action     string // 사건 종류 필터(빈값=전체)
	Actor      string // 수행 주체 필터(빈값=전체)
	SortField  string // 정렬 기준(화이트리스트 밖은 ts 로 폴백)
	SortAsc    bool   // true=오름차순. 기본(false)은 최신순이다.
	Limit      int    // 0 이하이면 100
	Offset     int    // 음수는 0
}

// RemoteAuditRepository 는 원격 관리 감사 로그의 영속 저장소이다(spec §4.6 F05).
//
// append-only: 갱신/삭제 API 를 노출하지 않는다(감사 무결성). 조회는 노드별 필터 +
// 페이지네이션을 지원한다(관리 UI 관측성 — admin-gated 엔드포인트).
type RemoteAuditRepository interface {
	// Append 는 감사 레코드를 추가한다(추가 전용 — REQ-F05). 시크릿 미포함을 호출자가
	// 보장한다(domain/action/result 만 — REQ-F06).
	Append(ctx context.Context, rec RemoteAuditRecord) error
	// List 는 조건에 맞는 감사 레코드와 **필터를 적용한 전체 건수**를 반환한다
	// (@SPEC:SPEC-REMOTE-LOG-001).
	//
	// 전체 건수를 함께 주는 이유는 화면이 쪽 수를 알아야 하기 때문이다. 건수 없이
	// "다음 쪽이 있을지도 모른다" 로 그리면 마지막 쪽에서 빈 화면을 보게 된다.
	List(ctx context.Context, q RemoteAuditQuery) ([]RemoteAuditRecord, int64, error)

	// DeleteOlderThan 은 beforeMs 보다 오래된 레코드를 지우고 지운 건수를 반환한다
	// (@SPEC:SPEC-REMOTE-LOG-001 보존 정책).
	//
	// append-only 원칙의 예외는 **보존 기간뿐**이다 — 기록을 골라 지우는 API 는
	// 두지 않는다(감사 무결성). 연결/끊어짐까지 남기기 시작하면 불안정한 노드
	// 하나가 표를 빠르게 불리므로, 지우는 자리가 없으면 언젠가 문제가 된다.
	DeleteOlderThan(ctx context.Context, beforeMs int64) (int64, error)
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
