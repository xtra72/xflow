// enrollment_token_repository.go 는 enrollment 토큰(가입 토큰) 영속 저장소 인터페이스를
// 정의한다(@SPEC:SPEC-REMOTE-001 v1.1 그룹 H, REQ-REMOTE-H03~H07).
//
// enrollment 토큰은 관리자가 사전 발급하여, 노드가 register 시 제시하면 관리자 수동
// 승인 없이 자동 승인되도록 하는 1회성(또는 횟수/기간 제한) 시크릿이다. 보안 불변식:
//   - 원본 토큰은 절대 저장하지 않는다 — SHA-256 해시(token_hash)만 영속한다(REQ-H06).
//   - List 는 메타데이터만 반환하고 token_hash 를 노출하지 않는다(REQ-H06).
//   - 폐기(revoke)는 즉시 후속 사용을 거부한다(REQ-H04).
//
// 시각 컬럼은 epoch milliseconds(int64) 이다(프로젝트 규약).
package storage

import (
	"context"
	"errors"
)

// ErrEnrollmentTokenNotFound 는 해당 토큰이 없을 때 반환된다.
var ErrEnrollmentTokenNotFound = errors.New("enrollment token not found")

// ErrEnrollmentTokenExhausted 는 max_uses 에 도달하여 더 이상 사용할 수 없을 때
// IncrementUses 가 반환한다(REQ-H07).
var ErrEnrollmentTokenExhausted = errors.New("enrollment token exhausted")

// EnrollmentToken 은 enrollment_tokens 테이블의 단일 행을 표현한다(spec §5.8).
//
// ExpiresAt / MaxUses 는 nullable 이다(미설정 시 nil = 무기한/무제한). TokenHash 는
// 원본 토큰의 SHA-256 16진 해시이며, GetByHash 외 조회(List)에서는 노출되지 않는다.
type EnrollmentToken struct {
	ID        string // PK — 토큰 식별 UUID(원본 토큰 아님)
	TokenHash string // 원본 토큰의 SHA-256 16진 해시(REQ-H06 — 원본 미저장)
	Label     string // 관리 편의용 라벨(선택)
	CreatedAt int64  // 생성 시각(epoch ms)
	ExpiresAt *int64 // 만료 시각(epoch ms). nil 이면 무기한.
	MaxUses   *int   // 최대 사용 횟수. nil 이면 무제한.
	Uses      int    // 현재 사용 횟수
	Revoked   bool   // 폐기 여부(REQ-H04)
}

// IntPtr 는 정수 리터럴을 *int 로 만드는 헬퍼이다(MaxUses 등 nullable 필드 구성용).
func IntPtr(v int) *int { return &v }

// Int64Ptr 는 정수 리터럴을 *int64 로 만드는 헬퍼이다(ExpiresAt 등 nullable 필드 구성용).
func Int64Ptr(v int64) *int64 { return &v }

// EnrollmentTokenRepository 는 enrollment 토큰의 영속 저장소 인터페이스이다(spec §5.8).
type EnrollmentTokenRepository interface {
	// Create 는 새 토큰을 저장한다(token_hash 만 — 원본 미저장, REQ-H06).
	Create(ctx context.Context, tok EnrollmentToken) error
	// GetByHash 는 token_hash 로 토큰을 조회한다. 없으면 ErrEnrollmentTokenNotFound.
	// 검증 경로 전용이므로 TokenHash 를 포함한 전체 행을 반환한다.
	GetByHash(ctx context.Context, tokenHash string) (EnrollmentToken, error)
	// List 는 모든 토큰의 메타데이터를 반환한다(TokenHash 는 빈 값으로 가려짐 — REQ-H06).
	List(ctx context.Context) ([]EnrollmentToken, error)
	// Revoke 는 토큰을 폐기한다(즉시 후속 사용 거부 — REQ-H04). 없으면 ErrEnrollmentTokenNotFound.
	Revoke(ctx context.Context, id string) error
	// IncrementUses 는 사용 횟수를 1 증가시킨다. max_uses 가 설정되어 있고 이미 도달했으면
	// 증가하지 않고 ErrEnrollmentTokenExhausted 를 반환한다(원자적 — 동시 경합 안전, REQ-H07).
	// 없으면 ErrEnrollmentTokenNotFound.
	IncrementUses(ctx context.Context, id string) error
	// Delete 는 토큰을 삭제한다. 없으면 ErrEnrollmentTokenNotFound.
	Delete(ctx context.Context, id string) error
	// Close 는 저장소 리소스를 정리한다.
	Close() error
}

// NewEnrollmentTokenRepository 는 storage type 에 따라 EnrollmentTokenRepository
// 구현을 생성한다. v1.1 은 sqlite 만 지원한다(서버 측 저장소).
func NewEnrollmentTokenRepository(ctx context.Context, storageType, sqlitePath string) (EnrollmentTokenRepository, error) {
	switch storageType {
	case "sqlite", "file":
		return NewEnrollmentTokenSQLiteRepository(ctx, sqlitePath)
	default:
		return NewEnrollmentTokenSQLiteRepository(ctx, sqlitePath)
	}
}
