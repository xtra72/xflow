package storage

import (
	"context"
	"errors"
)

// -----------------------------------------------------------------------------
// 대시보드 1급 엔티티 모델 (SPEC-DASHBOARD-004)
// -----------------------------------------------------------------------------

// @SPEC:SPEC-DASHBOARD-004 (M2, spec.md §2.1)
// Dashboard 는 dashboards 테이블의 1행 = 대시보드 1장이다.
//
// UID 가 클라이언트에 노출되는 식별자이며 ID 는 dashboard_acl 조인 전용 내부 키다.
// Owner 는 JWT Claims.Username 으로 서버가 결정한다 — 요청 본문의 owner 는 무시된다.
// Version / CreatedAt / UpdatedAt 도 서버가 부여한다.
//
// Payload 는 **그 대시보드 한 장**의 JSON 이다
// ({ panels, layout, gridCols, showGridLines, refreshInterval }). 구 모델의
// dashboardPages 배열은 존재하지 않는다.
type Dashboard struct {
	ID         int64
	UID        string
	Name       string
	Owner      string
	Visibility string // "private" | "shared" | "acl"
	IsDefault  bool
	SortOrder  int64
	Version    int64
	CreatedAt  int64 // epoch milliseconds
	UpdatedAt  int64 // epoch milliseconds
	Payload    []byte
}

// DashboardUpdate 는 Update 의 부분 갱신 필드이다. nil 필드는 변경하지 않는다.
//
// PUT(본문 저장)과 PATCH(메타 변경)를 하나의 저장소 메서드로 처리한다 — 둘 다
// version 을 증가시키고 If-Match 검증을 받으므로 낙관적 동시성 정책이 하나로
// 유지된다(spec.md §4.5 와 같은 취지).
type DashboardUpdate struct {
	Name       *string
	Visibility *string
	IsDefault  *bool
	SortOrder  *int64
	Owner      *string
	Payload    []byte // nil 이면 변경하지 않음
}

// DashboardRepository 는 대시보드 1급 엔티티의 영속 저장소 인터페이스이다.
//
// 모든 조회·변경은 클라이언트 식별자 uid 를 키로 한다. 인가 판정은 본 계층의
// 책임이 아니다 — 저장소는 데이터 정합성만 책임지고, (요청자, 대시보드) 판정은
// internal/dashboardacl 이 담당한다(roles_sqlite.go 와 동일한 역할 분리).
type DashboardRepository interface {
	// List 는 전체 대시보드를 sort_order, uid 순으로 반환한다.
	//
	// includePayload 가 false 이면 Payload 는 nil 이다. 목록 응답은 payload 를
	// 포함하지 않으므로(spec.md §2.3) 기본은 false 이고, 레거시 호환 shim 처럼
	// 본문이 필요한 경로만 true 를 쓴다. 어느 쪽이든 질의는 1회다(§5 목록 성능).
	List(ctx context.Context, includePayload bool) ([]Dashboard, error)

	// Get 은 uid 의 대시보드를 payload 와 함께 조회한다. 없으면 ErrDashboardNotFound.
	Get(ctx context.Context, uid string) (*Dashboard, error)

	// Create 는 새 대시보드를 생성하고 서버가 부여한 값이 채워진 행을 반환한다.
	//
	// 서버 부여: ID, Version(=1), CreatedAt, UpdatedAt. 입력의 해당 필드는 무시된다.
	// uid 가 이미 존재하면 ErrDashboardUIDExists.
	Create(ctx context.Context, d Dashboard) (*Dashboard, error)

	// Update 는 uid 의 대시보드를 부분 갱신하고 새 version 을 부여한다.
	//
	// expectedVersion 의 의미(구 모델 Put 정책 승계):
	//   - < 0  : unconditional (If-Match 헤더 없음)
	//   - >= 0 : If-Match 검증. 현재 version 과 다르면 ErrDashboardVersionMismatch
	//            를 반환하고 서버 상태를 변경하지 않는다.
	//
	// 성공 시 새 version = old.version + 1, updated_at = 서버 시각.
	Update(ctx context.Context, uid string, upd DashboardUpdate, expectedVersion int64) (*Dashboard, error)

	// Delete 는 uid 의 대시보드와 그에 딸린 ACL 행을 함께 삭제한다.
	// 존재하지 않더라도 nil 을 반환한다(멱등) — 존재 확인은 인가 판정을 위해
	// 핸들러가 이미 Get 으로 수행한다.
	Delete(ctx context.Context, uid string) error
}

// -----------------------------------------------------------------------------
// 레거시 스냅샷 모델 (SPEC-DASHBOARD-001) — 호환 shim 과 원격 프록시 전용
// -----------------------------------------------------------------------------

// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-4)
// DashboardSnapshotRepository 는 공유/개인 대시보드 snapshot 의 영속 저장소
// 인터페이스이다.
//
// @SPEC:SPEC-DASHBOARD-004 (M2)
// 본 인터페이스는 SPEC-DASHBOARD-004 이전의 (scope, owner) 2행 모델이며,
// 이름만 DashboardRepository 에서 변경되었다(신규 엔티티 모델이 그 이름을 쓴다).
// 읽기 전용 호환 shim 과 원격 노드 프록시가 레거시 응답 형상을 유지해야 하므로
// 제거하지 않는다(spec.md §4.4).
//
// (scope, owner) 의미:
//   - scope="global", owner=""        → 공유 대시보드 (단일 row, DB 상 owner 컬럼 NULL)
//   - scope="user",   owner=<username> → 특정 사용자 개인 대시보드
//
// 핸들러 레이어가 URL (/shared vs /mine) 과 JWT Claims.Username 으로부터 적절한
// (scope, owner) 쌍을 결정하여 저장소를 호출한다. 클라이언트는 (scope, owner) 를
// 직접 지정할 수 없다 (UB-003 owner spoofing 차단).
type DashboardSnapshotRepository interface {
	// Get 은 (scope, owner) 의 단일 snapshot 을 조회한다. 없으면 ErrDashboardNotFound.
	Get(ctx context.Context, scope, owner string) (*DashboardSnapshot, error)

	// Put 은 snapshot 을 저장하고 새 version 을 부여한다 (server-assigned).
	//
	// expectedVersion 의 의미:
	//   - < 0 (예: -1)            : unconditional (If-Match 헤더 없음)
	//   - >= 0                    : If-Match 검증. 현재 저장된 version 과 다르면
	//                               ErrDashboardVersionMismatch 반환 (서버 상태 미변경).
	//   - 새 row (snapshot 부재)   : 현재 version 은 0 으로 간주됨.
	//
	// Put 성공 시 새 version = old.version + 1, updated_at = time.Now().UnixMilli().
	Put(ctx context.Context, scope, owner string, payload []byte, expectedVersion int64) (*DashboardSnapshot, error)

	// Delete 는 snapshot 을 삭제한다. 존재하지 않더라도 nil 을 반환한다 (멱등).
	Delete(ctx context.Context, scope, owner string) error
}

// DashboardSnapshot 은 단일 (scope, owner) 의 영속 상태를 표현한다.
//
// Payload 는 클라이언트가 PUT 한 JSON 바이트 슬라이스 (재직렬화 없이 그대로 보관).
// Scope/Owner/Version/UpdatedAt 는 서버가 부여한다 (UR-005).
type DashboardSnapshot struct {
	Scope     string // "global" | "user"
	Owner     string // global 시 "" (DB 상 NULL), user 시 username
	Version   int64
	UpdatedAt int64 // epoch milliseconds
	Payload   []byte
}

// 저장소 sentinel 에러.
var (
	// ErrDashboardNotFound 는 대시보드(구 모델: (scope, owner) snapshot / 신 모델:
	// uid) 가 없을 때 반환된다.
	ErrDashboardNotFound = errors.New("dashboard not found")

	// ErrDashboardVersionMismatch 는 저장 시 expectedVersion 이 현재 저장된 version 과
	// 일치하지 않을 때 반환된다 (낙관적 동시성 충돌 신호 — HTTP 409 매핑).
	ErrDashboardVersionMismatch = errors.New("dashboard version mismatch")

	// ErrDashboardUIDExists 는 이미 존재하는 uid 로 생성을 시도할 때 반환된다
	// (dashboards.uid UNIQUE 제약 위반 — HTTP 400/409 매핑은 핸들러 판단).
	ErrDashboardUIDExists = errors.New("dashboard uid already exists")
)
