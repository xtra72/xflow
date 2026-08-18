// @SPEC:SPEC-AUTH-005 (M1)
// Package rbac 는 권한 키 카탈로그와 빌트인 역할 정의를 보유하는 leaf 패키지이다.
//
// 본 패키지는 xflow 의 다른 internal 패키지를 import 하지 않는다.
// internal/auth 가 이미 internal/storage 를 import 하고 있으므로 (credentials.go),
// 카탈로그를 auth 패키지에 두면 storage 의 마이그레이션 시드가 auth 를 참조할 때
// storage → auth → storage 순환 참조가 발생한다. 카탈로그를 의존성 없는 leaf 로
// 분리하여 storage(시드) 와 auth(인가) 가 모두 동일한 단일 정의를 참조하게 한다.
//
// internal/auth/rbac.go 는 본 패키지의 얇은 facade 이며, spec.md §3 트레이서빌리티
// 표의 파일 경로를 유지한다.
package rbac

import "sort"

// 권한 액션 상수 (spec.md §2.1).
const (
	ActionRead    = "read"
	ActionCreate  = "create"
	ActionUpdate  = "update"
	ActionDelete  = "delete"
	ActionExecute = "execute"
)

// 권한 리소스 상수 (spec.md §2.1).
const (
	ResourceAgent      = "agent"
	ResourceDevice     = "device"
	ResourceFlow       = "flow"
	ResourceDashboard  = "dashboard"
	ResourceNode       = "node"
	ResourceSchedule   = "schedule"
	ResourceMonitoring = "monitoring"
	ResourceStore      = "store"
	ResourceRemote     = "remote"
	ResourceSystem     = "system"
	ResourceUser       = "user"
	ResourceRole       = "role"
)

// 빌트인 역할 이름 상수. 기존 users.role 값과 정확히 일치하므로 사용자 데이터
// 마이그레이션이 불필요하다 (spec.md §2.1).
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

// maxRoleNameLen 은 역할 이름의 최대 길이이다 (spec.md §6.3).
const maxRoleNameLen = 32

// resourceActions 는 spec.md §2.1 리소스·액션 표의 유일한 정의이다.
// 카탈로그와 빌트인 역할 3종이 모두 본 표에서 파생되므로, 표를 고치면 전체가
// 함께 갱신된다 (수기 목록 중복으로 인한 drift 방지).
var resourceActions = []struct {
	Resource string
	Actions  []string
}{
	// execute = start/stop/restart/enable/disable
	{ResourceAgent, []string{ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionExecute}},
	// execute = 제어 명령
	{ResourceDevice, []string{ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionExecute}},
	// execute = 배포/시작/정지
	{ResourceFlow, []string{ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionExecute}},
	// 패널 레이아웃 저장
	{ResourceDashboard, []string{ActionRead, ActionUpdate}},
	// 노드 타입 카탈로그
	{ResourceNode, []string{ActionRead}},
	{ResourceSchedule, []string{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
	// 로그·메트릭 조회
	{ResourceMonitoring, []string{ActionRead}},
	// 스토어 키/값
	{ResourceStore, []string{ActionRead, ActionUpdate}},
	// 원격 노드 관리 (하위 API 세분 권한은 범위 밖 — 단일 키)
	{ResourceRemote, []string{ActionRead, ActionUpdate}},
	// 시스템 설정
	{ResourceSystem, []string{ActionRead, ActionUpdate}},
	// 사용자 관리
	{ResourceUser, []string{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
	// 역할 관리
	{ResourceRole, []string{ActionRead, ActionCreate, ActionUpdate, ActionDelete}},
}

// managementResources 는 사용자·역할 관리 리소스이다.
// editor/viewer 는 본 리소스의 read 도 보유하지 않는다.
//
// spec.md §2.1 의 "전 리소스 read" 문구를 문자 그대로 적용하면 viewer 가
// user.read 를 얻어 acceptance.md AC-07 (viewer 의 GET /api/v1/users → 403) 과
// 충돌한다. AC 를 만족하면서 "user·role 관리 제외" 취지를 지키도록 관리 리소스는
// read 에서도 제외한다.
var managementResources = map[string]bool{
	ResourceUser: true,
	ResourceRole: true,
}

// editorWritableResources 는 editor 가 쓰기 액션을 수행할 수 있는 리소스이다
// (spec.md §2.1 editor 행).
var editorWritableResources = map[string]bool{
	ResourceAgent:     true,
	ResourceDevice:    true,
	ResourceFlow:      true,
	ResourceSchedule:  true,
	ResourceDashboard: true,
	ResourceStore:     true,
}

// editorWritableActions 는 editor 에게 허용되는 쓰기 액션이다. delete 는 제외된다.
var editorWritableActions = map[string]bool{
	ActionCreate:  true,
	ActionUpdate:  true,
	ActionExecute: true,
}

// 카탈로그 파생 결과. 프로세스 수명 동안 불변이다.
var (
	allPermissions     []string
	permissionSet      map[string]struct{}
	adminPermissions   []string
	editorPermissions  []string
	viewerPermissions  []string
	builtinRoleNameSet = map[string]bool{
		RoleAdmin:  true,
		RoleEditor: true,
		RoleViewer: true,
	}
)

func init() {
	permissionSet = make(map[string]struct{})
	for _, ra := range resourceActions {
		for _, action := range ra.Actions {
			key := ra.Resource + "." + action
			allPermissions = append(allPermissions, key)
			permissionSet[key] = struct{}{}

			// admin = 카탈로그 전체.
			adminPermissions = append(adminPermissions, key)

			isReadable := action == ActionRead && !managementResources[ra.Resource]
			if isReadable {
				viewerPermissions = append(viewerPermissions, key)
				editorPermissions = append(editorPermissions, key)
				continue
			}
			if editorWritableResources[ra.Resource] && editorWritableActions[action] {
				editorPermissions = append(editorPermissions, key)
			}
		}
	}
	sort.Strings(allPermissions)
	sort.Strings(adminPermissions)
	sort.Strings(editorPermissions)
	sort.Strings(viewerPermissions)
}

// BuiltinRole 은 마이그레이션 시 시드되는 빌트인 역할 정의이다.
type BuiltinRole struct {
	Name        string
	Description string
	Permissions []string
}

// Permissions 는 카탈로그 전체 권한 키를 사전순으로 정렬해 반환한다.
// 호출자가 슬라이스를 변형해도 카탈로그가 오염되지 않도록 복사본을 돌려준다.
func Permissions() []string {
	return append([]string(nil), allPermissions...)
}

// IsValidPermission 은 key 가 카탈로그에 정의된 권한인지 판정한다.
// 카탈로그에 없는 키는 역할에 저장될 수 없다 (spec.md §2.1).
func IsValidPermission(key string) bool {
	_, ok := permissionSet[key]
	return ok
}

// BuiltinRoles 는 빌트인 역할 3종을 admin → editor → viewer 순서로 반환한다.
// 각 역할의 Permissions 는 사전순 정렬된 복사본이다.
func BuiltinRoles() []BuiltinRole {
	return []BuiltinRole{
		{
			Name:        RoleAdmin,
			Description: "전체 권한",
			Permissions: append([]string(nil), adminPermissions...),
		},
		{
			Name:        RoleEditor,
			Description: "조회 + 에이전트/장치/플로우/스케줄/대시보드/스토어 편집",
			Permissions: append([]string(nil), editorPermissions...),
		},
		{
			Name:        RoleViewer,
			Description: "조회 전용",
			Permissions: append([]string(nil), viewerPermissions...),
		},
	}
}

// IsBuiltinRole 은 name 이 빌트인 역할인지 판정한다 (삭제·수정 금지 불변식용).
func IsBuiltinRole(name string) bool {
	return builtinRoleNameSet[name]
}

// IsValidRoleName 은 역할 이름 규칙을 검증한다: 소문자·숫자·하이픈, 1~32자
// (spec.md §6.3).
func IsValidRoleName(name string) bool {
	if len(name) < 1 || len(name) > maxRoleNameLen {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return true
}
