package storage

import (
	"context"
	"errors"
)

// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-4)
// DashboardRepository 는 공유/개인 대시보드 snapshot 의 영속 저장소 인터페이스이다.
//
// (scope, owner) 의미:
//   - scope="global", owner=""        → 공유 대시보드 (단일 row, DB 상 owner 컬럼 NULL)
//   - scope="user",   owner=<username> → 특정 사용자 개인 대시보드
//
// 핸들러 레이어가 URL (/shared vs /mine) 과 JWT Claims.Username 으로부터 적절한
// (scope, owner) 쌍을 결정하여 저장소를 호출한다. 클라이언트는 (scope, owner) 를
// 직접 지정할 수 없다 (UB-003 owner spoofing 차단).
type DashboardRepository interface {
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
	// ErrDashboardNotFound 는 (scope, owner) 에 해당하는 snapshot 이 없을 때 반환된다.
	ErrDashboardNotFound = errors.New("dashboard snapshot not found")

	// ErrDashboardVersionMismatch 는 Put 시 expectedVersion 이 현재 저장된 version 과
	// 일치하지 않을 때 반환된다 (last-write-wins 정책의 충돌 신호 — HTTP 409 매핑).
	ErrDashboardVersionMismatch = errors.New("dashboard version mismatch")
)
