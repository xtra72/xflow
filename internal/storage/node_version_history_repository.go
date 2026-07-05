// node_version_history_repository.go 는 관리 노드(managed node)의 버전 변경 이력
// 영속 저장소 인터페이스를 정의한다(@SPEC:SPEC-REMOTE-001 버전 관리 Phase 1).
//
// 노드가 보고한 version 이 직전 저장값과 달라질 때마다 (instance_id, version,
// changed_at) 한 줄을 append 하여, 관리 서버가 각 노드의 버전 변천을 타임라인으로
// 조회할 수 있게 한다. managed_nodes.version 은 "현재값"만 보존하므로 이력은 별도
// 테이블로 분리한다.
//
// ManagedNodeRepository 패턴(인터페이스 + sqlite 구현 + factory)을 준용한다.
// changed_at 은 epoch milliseconds(int64)이다(프로젝트 규약).
package storage

import "context"

// NodeVersionHistory 는 node_version_history 테이블의 단일 행이다.
type NodeVersionHistory struct {
	InstanceID string // 노드 식별 UUID
	Version    string // 변경 후 버전 문자열(노드 보고값)
	ChangedAt  int64  // 변경 감지 시각(epoch ms)
}

// NodeVersionHistoryRepository 는 노드 버전 변경 이력의 영속 저장소 인터페이스이다.
type NodeVersionHistoryRepository interface {
	// Append 는 한 노드의 새 버전 관측을 이력에 추가한다(항상 INSERT — 호출 측이
	// "직전과 다를 때만" 호출하여 중복을 막는다).
	Append(ctx context.Context, instanceID, version string, changedAtMs int64) error
	// List 는 instance_id 의 버전 이력을 최신순(changed_at DESC)으로 반환한다.
	// limit <= 0 이면 전체를 반환한다.
	List(ctx context.Context, instanceID string, limit int) ([]NodeVersionHistory, error)
	// Close 는 저장소 리소스를 정리한다.
	Close() error
}

// NewNodeVersionHistoryRepository 는 storage type 에 따라 구현을 생성한다.
// 현재는 sqlite 만 지원한다(서버 측 저장소). 알 수 없는 type 은 sqlite 로 폴백한다.
func NewNodeVersionHistoryRepository(ctx context.Context, storageType, sqlitePath string) (NodeVersionHistoryRepository, error) {
	switch storageType {
	case "sqlite", "file":
		return NewNodeVersionHistorySQLiteRepository(ctx, sqlitePath)
	default:
		return NewNodeVersionHistorySQLiteRepository(ctx, sqlitePath)
	}
}
