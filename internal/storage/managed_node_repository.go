// managed_node_repository.go 는 관리 노드(managed node) 영속 저장소 인터페이스를
// 정의한다(@SPEC:SPEC-REMOTE-001 M2, spec §5.4 managed_nodes).
//
// FlowRepository 패턴(Save/Get/List/Delete/Close)을 준용하되, 등록/승인 상태 머신과
// online/offline 추적에 맞춘 메서드(Upsert/UpdateStatus/SetToken/SetOnline)를 제공한다.
//
// 본 M2 범위는 managed_nodes 테이블만 다룬다. mirrored_flows/agents/devices 미러
// 테이블은 M4 에서 추가한다(본 파일에서 의도적으로 제외).
package storage

import (
	"context"
	"errors"
)

// ErrManagedNodeNotFound 는 instance_id 에 해당하는 관리 노드가 없을 때 반환된다.
var ErrManagedNodeNotFound = errors.New("managed node not found")

// ManagedNode 는 managed_nodes 테이블의 단일 행을 표현한다(spec §5.4).
//
// LastSeen/CreatedAt/UpdatedAt 은 epoch milliseconds(int64) 이다(프로젝트 규약).
// TokenID 는 발급된 노드 토큰의 식별자(폐기 매핑용)이다(REQ-C04/C07).
type ManagedNode struct {
	InstanceID string // PK — 노드(xflow 설치본) 식별 UUID
	Hostname   string
	Version    string
	Status     string // pending | approved | rejected | revoked (spec §5.6)
	TokenID    string // 발급된 노드 토큰 식별자(폐기 매핑). 미발급 시 빈 값.
	LastSeen   int64  // 마지막 생존 신호 시각(epoch ms)
	Online     bool   // 현재 연결 여부(오프라인 시에도 행은 보존 — REQ-E06)
	CreatedAt  int64  // 최초 등록 시각(epoch ms)
	UpdatedAt  int64  // 마지막 갱신 시각(epoch ms)
}

// ManagedNodeRepository 는 관리 노드의 영속 저장소 인터페이스이다(spec §5.4).
type ManagedNodeRepository interface {
	// Upsert 는 노드를 저장한다. 동일 instance_id 가 있으면 메타/상태를 갱신하되
	// created_at 은 보존한다(last-known 유지 — REQ-E06).
	Upsert(ctx context.Context, node ManagedNode) error
	// Get 은 instance_id 로 노드를 조회한다. 없으면 ErrManagedNodeNotFound.
	Get(ctx context.Context, instanceID string) (ManagedNode, error)
	// List 는 모든 관리 노드를 반환한다.
	List(ctx context.Context) ([]ManagedNode, error)
	// UpdateStatus 는 등록 상태를 전이한다(pending/approved/rejected/revoked).
	// 없으면 ErrManagedNodeNotFound.
	UpdateStatus(ctx context.Context, instanceID, status string) error
	// SetToken 은 노드 토큰 식별자를 저장한다(폐기 매핑). 없으면 ErrManagedNodeNotFound.
	SetToken(ctx context.Context, instanceID, tokenID string) error
	// SetOnline 은 online 상태와 last_seen 을 갱신한다(행 삭제 없음 — REQ-E06).
	// 없으면 ErrManagedNodeNotFound.
	SetOnline(ctx context.Context, instanceID string, online bool, lastSeenMs int64) error
	// Delete 는 instance_id 로 노드를 삭제한다. 없으면 ErrManagedNodeNotFound.
	Delete(ctx context.Context, instanceID string) error
	// Close 는 저장소 리소스를 정리한다.
	Close() error
}

// NewManagedNodeRepository 는 storage type 에 따라 ManagedNodeRepository 구현을
// 생성한다. M2 는 sqlite 만 지원한다(서버 측 캐시 저장소).
//
//   - "sqlite": ManagedNodeSQLiteRepository (Pure Go SQLite)
func NewManagedNodeRepository(ctx context.Context, storageType, sqlitePath string) (ManagedNodeRepository, error) {
	switch storageType {
	case "sqlite", "file":
		// file 모드 환경에서도 관리 노드 메타는 sqlite 에 저장한다(서버 캐시 — §5.4).
		return NewManagedNodeSQLiteRepository(ctx, sqlitePath)
	default:
		return NewManagedNodeSQLiteRepository(ctx, sqlitePath)
	}
}
