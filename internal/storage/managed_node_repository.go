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
// LastSeen/CreatedAt/UpdatedAt/StartedAt 은 epoch milliseconds(int64) 이다(프로젝트 규약).
// TokenID 는 발급된 노드 토큰의 식별자(폐기 매핑용)이다(REQ-C04/C07).
//
// v1.4(M9, 그룹 K):
//   - GroupName: 단일 그룹 라벨(서버 운영 메타데이터, 관리자 배정 전용 — A13/REQ-K01).
//     빈값은 가상 "전체"(All) 버킷을 의미하며 예약 라벨로 영속하지 않는다(OQ-K5).
//   - OS/Arch/StartedAt: 노드가 register/heartbeat 로 보고하는 BASIC 시스템 정보
//     (REQ-K07/K08). 자원 메트릭(CPU/메모리/디스크)은 보고하지 않는다(본 마일스톤 제외).
//     uptime 은 StartedAt 과 서버 현재 시각의 차로 파생하며 저장하지 않는다(REQ-K08).
//     하위 호환: 미보고 노드는 빈값/0(REQ-K09).
type ManagedNode struct {
	InstanceID string // PK — 노드(xflow 설치본) 식별 UUID
	Hostname   string
	Version    string
	Status     string // pending | approved | rejected | revoked (spec §5.6)
	TokenID    string // 발급된 노드 토큰 식별자(폐기 매핑). 미발급 시 빈 값.
	LastSeen   int64  // 마지막 생존 신호 시각(epoch ms)
	Online     bool   // 현재 연결 여부(오프라인 시에도 행은 보존 — REQ-E06)
	GroupName  string // 단일 그룹 라벨(빈값=전체, 관리자 전용 — REQ-K01/K02)
	OS         string // 노드 OS(runtime.GOOS, BASIC 시스템 정보 — REQ-K07)
	Arch       string // 노드 arch(runtime.GOARCH, BASIC 시스템 정보 — REQ-K07)
	StartedAt  int64  // 노드 프로세스 시작 시각(epoch ms, uptime 산출용 — REQ-K07/K08)
	// DisplayWidth/DisplayHeight 는 노드 장비 모니터(키오스크/터치스크린) 해상도이다
	// (px, v1.6 M11, 그룹 M, REQ-M01/M02). 노드가 register/heartbeat 로 보고하며(config
	// 파생 — 헤드리스 데몬), 0 은 미보고를 의미한다(관리자 뷰가 폴백 — REQ-M03). 시스템
	// 정보(os/arch/started_at)와 동일하게 SetSystemInfo 가 제공된 값만 갱신하고 미제공(0)
	// 은 기존값을 보존한다(하위 호환 — REQ-M03).
	DisplayWidth  int // 노드 장비 화면 가로 px(0=미보고 — REQ-M01/M03)
	DisplayHeight int // 노드 장비 화면 세로 px(0=미보고 — REQ-M01/M03)
	// DisplayOverrideWidth/DisplayOverrideHeight 는 관리자가 서버에서 노드 config/재시작
	// 없이 강제한 노드 해상도 오버라이드이다(px, v1.6 M11 확장, OQ-M1 보조 override).
	// group_name 과 동일하게 관리자 소유(admin-owned)이므로 register/heartbeat upsert·
	// SetSystemInfo(노드 보고)가 절대 덮어쓰지 않으며, SetNodeDisplayOverride 로만 변경한다.
	// 0,0 은 오버라이드 없음을 의미하며, 이때 effective 해상도는 노드 보고값으로 폴백한다.
	DisplayOverrideWidth  int   // 관리자 강제 가로 px(0=오버라이드 없음 — effective 폴백)
	DisplayOverrideHeight int   // 관리자 강제 세로 px(0=오버라이드 없음 — effective 폴백)
	CreatedAt             int64 // 최초 등록 시각(epoch ms)
	UpdatedAt             int64 // 마지막 갱신 시각(epoch ms)
}

// NodeGroupCount 는 distinct 그룹 라벨과 그 노드 수이다(REQ-K03).
//
// GroupName 이 빈 문자열이면 가상 "전체"(All) 버킷(그룹 미지정 노드)을 의미한다.
// UI/API 는 빈 라벨을 "전체"로 표시한다(OQ-K5 — 예약 라벨 비영속).
type NodeGroupCount struct {
	GroupName string // 그룹 라벨(빈값=전체 버킷)
	NodeCount int    // 해당 그룹의 노드 수
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

	// --- v1.4(M9, 그룹 K): 노드 그룹핑 + BASIC 시스템 정보 ---

	// SetNodeGroup 은 노드의 단일 그룹 라벨을 배정/변경/해제한다(REQ-K02). groupName
	// 이 빈 문자열이면 그룹을 해제하여 "전체" 버킷으로 환원한다(REQ-K05). 그룹은 서버
	// 운영 메타데이터이므로 노드로 명령을 전파하지 않는다(A13). 없으면 ErrManagedNodeNotFound.
	SetNodeGroup(ctx context.Context, instanceID, groupName string) error
	// RenameGroup 은 oldName 그룹의 모든 노드 group_name 을 newName 으로 일괄 변경한다.
	// 영향받은 노드 수를 반환한다(0 이면 해당 그룹이 없음 — 멤버 없는 그룹은 비존재).
	RenameGroup(ctx context.Context, oldName, newName string) (int, error)
	// DeleteGroup 은 groupName 그룹의 모든 노드를 "전체" 버킷으로 이동한다(group_name="").
	// 영향받은 노드 수를 반환한다(0 이면 해당 그룹이 없음). 노드 행 자체는 삭제하지 않는다.
	DeleteGroup(ctx context.Context, groupName string) (int, error)
	// ListGroups 는 현재 사용 중인 distinct 그룹 라벨과 노드 수를 반환한다(REQ-K03).
	// 응답은 항상 가상 "전체" 버킷(GroupName="")의 노드 수를 포함하며(그룹 미지정 노드),
	// 빈 그룹(구성원 0)은 자동으로 목록에서 사라진다(REQ-K05). 정렬: "전체" 먼저, 그
	// 다음 그룹명 오름차순.
	ListGroups(ctx context.Context) ([]NodeGroupCount, error)
	// SetSystemInfo 는 노드가 보고한 BASIC 시스템 정보(os/arch/started_at) + 노드 해상도
	// (displayWidth/displayHeight)를 저장한다(REQ-K08/M01/M02). 제공된 필드만 갱신하고
	// 미제공(빈 문자열/0) 필드는 기존값을 보존한다(하위 호환 — heartbeat 가 일부만 보내거나
	// 구버전 노드가 생략 — REQ-K09/M03). group_name 은 절대 건드리지 않는다(관리자 전용).
	// 없으면 ErrManagedNodeNotFound.
	SetSystemInfo(ctx context.Context, instanceID, os, arch string, startedAtMs int64, displayWidth, displayHeight int) error

	// --- v1.6(M11 확장): 노드 해상도 서버-측 오버라이드(관리자 전용) ---

	// SetNodeDisplayOverride 는 관리자가 서버에서 노드 해상도를 강제하는 오버라이드를
	// 설정/해제한다(OQ-M1 보조 override). width<=0 또는 height<=0 이면 오버라이드를
	// 0,0 으로 해제하여 effective 해상도가 노드 보고값으로 폴백하도록 한다. 그룹 배정과
	// 동일하게 노드로 명령을 전파하지 않는 서버 운영 메타데이터이다(A13 일관). group_name·
	// 노드 보고 해상도(display_width/height)는 절대 건드리지 않는다(관리자/노드 소유 분리).
	// 없으면 ErrManagedNodeNotFound.
	SetNodeDisplayOverride(ctx context.Context, instanceID string, width, height int) error

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
