// mirror_repository.go 는 서버 측 인벤토리 미러 캐시 저장소 인터페이스를 정의한다
// (@SPEC:SPEC-REMOTE-001 M4, spec §5.4 mirrored_flows/agents/devices, REQ-E03/E04/E06).
//
// managed_nodes(등록/상태 머신)와 분리된 sibling 저장소로 둔다(관심사 분리): managed
// node 저장소는 노드 상태 머신을, MirrorRepository 는 노드별 자원 미러 캐시를 다룬다.
//
// 출처 태깅(REQ-E04): 모든 미러 행은 source_instance_id 를 보유한다. last-known
// (REQ-E06): 노드 오프라인 시에도 행을 삭제하지 않는다(online/last_seen 은 managed_nodes
// 가 보유). 노드 삭제 시 DeleteByNode 로 해당 노드 미러를 정리한다(orphan 방지).
package storage

import "context"

// MirroredResource 는 mirrored_flows/agents/devices 의 단일 행을 표현한다(spec §5.4).
//
// Kind 는 "flow" | "agent" | "device" 이다(테이블 구분과 더불어 응답 태깅에 사용).
// Definition 은 redaction(F06)된 정의/설정 JSON 문자열이다. UpdatedAt 은 epoch ms.
type MirroredResource struct {
	ID               string // 자원 ID(노드 로컬 ID)
	SourceInstanceID string // 출처 노드 instance_id (REQ-E04)
	Name             string
	Kind             string // flow | agent | device
	Status           string
	Definition       string // redacted JSON (F06)
	UpdatedAt        int64  // epoch ms
}

// MirrorRepository 는 노드별 인벤토리 미러 캐시의 영속 저장소이다(spec §5.4).
//
// 캐시는 오직 노드가 push 한 snapshot/delta 로만 변경된다(REQ-E08, A4 — 노드가 권위).
// 서버 측 admin 편집은 본 저장소를 직접 변경하지 않고 명령(그룹 D)으로 전파된다.
type MirrorRepository interface {
	// ReplaceFlows 는 한 노드의 flow 미러 전체를 교체한다(snapshot 수신 — REQ-E03).
	ReplaceFlows(ctx context.Context, instanceID string, items []MirroredResource) error
	// ReplaceAgents 는 한 노드의 agent 미러 전체를 교체한다(snapshot — REQ-E03).
	ReplaceAgents(ctx context.Context, instanceID string, items []MirroredResource) error
	// ReplaceDevices 는 한 노드의 device 미러 전체를 교체한다(snapshot — REQ-E03).
	ReplaceDevices(ctx context.Context, instanceID string, items []MirroredResource) error

	// UpsertResource 는 단일 미러 행을 추가/갱신한다(delta add/update — REQ-E02/E03).
	// kind 에 따라 적절한 테이블에 반영한다.
	UpsertResource(ctx context.Context, kind string, item MirroredResource) error
	// DeleteResource 는 노드+kind+id 로 단일 미러 행을 삭제한다(delta remove — REQ-E02).
	DeleteResource(ctx context.Context, instanceID, kind, id string) error

	// ListFlows / ListAgents / ListDevices 는 한 노드의 kind 별 미러를 반환한다(REQ-E05).
	ListFlows(ctx context.Context, instanceID string) ([]MirroredResource, error)
	ListAgents(ctx context.Context, instanceID string) ([]MirroredResource, error)
	ListDevices(ctx context.Context, instanceID string) ([]MirroredResource, error)

	// ListAllFlows / ListAllAgents / ListAllDevices 는 전 노드의 kind 별 미러를
	// 출처 노드 태그와 함께 반환한다(통합 목록 — REQ-E05).
	ListAllFlows(ctx context.Context) ([]MirroredResource, error)
	ListAllAgents(ctx context.Context) ([]MirroredResource, error)
	ListAllDevices(ctx context.Context) ([]MirroredResource, error)

	// DeleteByNode 는 한 노드의 모든 미러 행(flows/agents/devices)을 삭제한다(노드
	// 삭제 시 orphan 정리). 행이 없어도 에러가 아니다(멱등).
	DeleteByNode(ctx context.Context, instanceID string) error

	// Close 는 저장소 리소스를 정리한다.
	Close() error
}

// NewMirrorRepository 는 storage type 에 따라 MirrorRepository 구현을 생성한다.
// M4 는 sqlite 만 지원한다(서버 측 캐시 저장소 — §5.4).
func NewMirrorRepository(ctx context.Context, storageType, sqlitePath string) (MirrorRepository, error) {
	switch storageType {
	case "sqlite", "file":
		return NewMirrorSQLiteRepository(ctx, sqlitePath)
	default:
		return NewMirrorSQLiteRepository(ctx, sqlitePath)
	}
}
