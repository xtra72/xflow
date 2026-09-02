// @SPEC:SPEC-DASHBOARD-004 (M2, spec.md §2.2)
// Package dashboardacl 는 (요청자, 대시보드) 쌍에 대한 view / edit / delete / grant
// 인가를 판정하는 leaf 패키지이다.
//
// leaf 로 두는 이유는 internal/rbac 와 같다 — storage 와 api 가 모두 본 판정을
// 참조하므로, 어느 한쪽에 두면 순환 참조가 생긴다. 따라서 본 패키지는
// internal/storage 도 internal/api 도 import 하지 않으며, 판정에 필요한 최소 값
// 타입(Dashboard, ACLEntry)을 자체적으로 정의한다. 권한 키만 internal/rbac 에서
// 가져온다(rbac 역시 의존성 없는 leaf 이므로 순환이 생기지 않으며, 키 문자열을
// 중복 정의하지 않아 카탈로그와의 drift 를 막는다).
//
// 판정은 단일 함수 Evaluate 로 일원화된다. 핸들러가 경로마다 조건을 재작성하면
// 한 경로만 누락되어도 타인 대시보드가 노출되므로, 모든 경로가 이 함수를 부른다.
package dashboardacl

import "github.com/xtra/xflow/internal/rbac"

// 공개 범위(visibility) 값. dashboards.visibility CHECK 제약과 동일하다.
const (
	// VisibilityPrivate 는 소유자만 접근 가능하다.
	VisibilityPrivate = "private"
	// VisibilityShared 는 인증된 모든 사용자가 조회(view) 가능하다.
	VisibilityShared = "shared"
	// VisibilityACL 은 dashboard_acl 에 등재된 대상만 접근 가능하다.
	VisibilityACL = "acl"
)

// ACL 레벨 값. dashboard_acl.level CHECK 제약과 동일하다.
//
// LevelNone 은 저장되는 값이 아니라 "매치된 ACL 행이 없음" 을 뜻하는 판정 내부
// 값이다(빈 문자열).
const (
	LevelNone = ""
	LevelView = "view"
	LevelEdit = "edit"
)

// subject 접두사. 두 접두사 외의 값은 저장될 수 없다(spec.md §2.1).
const (
	SubjectPrefixUser = "user:"
	SubjectPrefixRole = "role:"
)

// 판정에 쓰이는 권한 키. 카탈로그(internal/rbac)에서 파생한다.
var (
	permRead   = rbac.ResourceDashboard + "." + rbac.ActionRead
	permCreate = rbac.ResourceDashboard + "." + rbac.ActionCreate
	permUpdate = rbac.ResourceDashboard + "." + rbac.ActionUpdate
	permDelete = rbac.ResourceDashboard + "." + rbac.ActionDelete
)

// Subject 는 판정 대상 요청자이다.
//
// Permissions 는 요청 시점에 조회된 전역 RBAC 권한 집합이다(SPEC-AUTH-005 §4.3 —
// 토큰이 아니라 요청 시점 조회이므로 역할 변경이 즉시 반영된다).
//
// AuthEnabled 가 false 이면 서버가 권한을 검사하지 않는 배포이므로 Username /
// Role / Permissions 는 무시된다(spec.md §2.10).
type Subject struct {
	Username    string
	Role        string
	Permissions map[string]struct{}
	AuthEnabled bool
}

// Dashboard 는 판정에 필요한 대시보드 속성만 담은 값 타입이다.
// storage.Dashboard 전체를 참조하지 않는 이유는 패키지 헤더의 leaf 설명 참조.
type Dashboard struct {
	UID        string
	Owner      string
	Visibility string
}

// ACLEntry 는 판정에 필요한 ACL 행 속성만 담은 값 타입이다.
type ACLEntry struct {
	Subject string
	Level   string
}

// Access 는 (요청자, 대시보드) 쌍의 최종 판정 결과이다.
type Access struct {
	View   bool
	Edit   bool
	Delete bool
	Grant  bool
}

// allAllowed 는 4종 전부 허용 결과이다(0단계 · 1단계 조기 종료용).
var allAllowed = Access{View: true, Edit: true, Delete: true, Grant: true}

// Evaluate 는 spec.md §2.2 의 판정 순서를 그대로 따른다. 먼저 확정된 단계에서
// 종료하며, 순서를 바꾸면 결과가 달라지므로 재배열하지 않는다.
//
//	0단계: 인증 비활성        → 4종 허용, 종료
//	1단계: 대시보드 관리자     → 4종 허용, 종료
//	2단계: 소유자             → base = edit (단, 전역 상한 적용)
//	3단계: 그 외              → visibility 로 base 산출
//
// **전역 RBAC 권한은 상한(ceiling)이다**(spec.md §4.3). 소유자라도
// dashboard.update 가 없으면 편집할 수 없고, ACL 은 상한을 올리지 못한다.
// 두 축이 서로를 우회하면 어느 쪽을 봐도 실제 권한을 알 수 없기 때문이다.
func Evaluate(s Subject, d Dashboard, acl []ACLEntry) Access {
	// 0단계 — 인증 비활성 배포에서 대시보드만 잠그면 인증 도입 이전 배포가
	// 사용 불가가 된다(spec.md §2.10).
	if !s.AuthEnabled {
		return allAllowed
	}

	// 1단계 — "대시보드 관리자" 는 역할 이름이 아니라 권한 조합으로 정의한다.
	// 커스텀 역할이 생기는 순간 역할 이름 열거가 성립하지 않기 때문이다
	// (spec.md §4.2, SPEC-AUTH-006 §4.1).
	if isDashboardAdmin(s) {
		return allAllowed
	}

	// 2단계 — 소유자 판정.
	//
	// Username 이 빈 문자열이면 소유자로 보지 않는다. 인증 비활성 상태에서 만들어진
	// 대시보드의 owner 는 '' 인데, 이후 인증을 활성화하면 그 대시보드는 visibility
	// 로만 접근이 결정되어야 한다 — 소유자 특권이 없다(spec.md §2.10).
	isOwner := s.Username != "" && d.Owner == s.Username

	// 3단계 — base 산출.
	base := LevelNone
	switch {
	case isOwner:
		base = LevelEdit
	case d.Visibility == VisibilityShared:
		base = LevelView
	case d.Visibility == VisibilityACL:
		base = maxACLLevel(s, acl)
	default:
		// private (및 알 수 없는 값) — 비소유자는 접근 없음.
		base = LevelNone
	}

	return Access{
		View:   rank(base) >= rank(LevelView) && has(s, permRead),
		Edit:   base == LevelEdit && has(s, permUpdate),
		Delete: isOwner && has(s, permDelete),
		Grant:  isOwner && has(s, permUpdate),
	}
}

// Subjects 는 s 에 매치될 수 있는 ACL subject 문자열을 반환한다.
// 저장소의 ListBySubjects 질의 인자로 쓰인다(대시보드 수와 무관한 단일 질의).
func Subjects(s Subject) []string {
	out := make([]string, 0, 2)
	if s.Username != "" {
		out = append(out, SubjectPrefixUser+s.Username)
	}
	if s.Role != "" {
		out = append(out, SubjectPrefixRole+s.Role)
	}
	return out
}

// IsValidLevel 은 level 이 저장 가능한 ACL 레벨인지 판정한다.
func IsValidLevel(level string) bool {
	return level == LevelView || level == LevelEdit
}

// IsValidVisibility 는 visibility 가 저장 가능한 공개 범위인지 판정한다.
func IsValidVisibility(visibility string) bool {
	return visibility == VisibilityPrivate ||
		visibility == VisibilityShared ||
		visibility == VisibilityACL
}

// isDashboardAdmin 은 dashboard.{read,create,update,delete} 4종을 **모두** 보유한
// 사용자인지 판정한다(spec.md §2.2 1단계).
func isDashboardAdmin(s Subject) bool {
	return has(s, permRead) && has(s, permCreate) && has(s, permUpdate) && has(s, permDelete)
}

// maxACLLevel 은 s 에 매치되는 ACL 행 중 최대 레벨을 반환한다.
//
// user: 와 role: 이 동시에 매치되면 높은 레벨이 이긴다(edit > view). ACL 은
// 거부(deny) 항목을 지원하지 않으므로 부여만으로 단조 증가한다(spec.md §2.2).
func maxACLLevel(s Subject, acl []ACLEntry) string {
	best := LevelNone
	for _, e := range acl {
		if !matchesSubject(s, e.Subject) {
			continue
		}
		if rank(e.Level) > rank(best) {
			best = e.Level
		}
	}
	return best
}

// matchesSubject 는 ACL subject 가 요청자에 매치되는지 판정한다.
//
// 존재하지 않는 사용자·역할을 가리키는 잔여 행은 매치되지 않으므로 무해하다
// (plan.md §3 위험표 — 유령 권한).
func matchesSubject(s Subject, subject string) bool {
	if s.Username != "" && subject == SubjectPrefixUser+s.Username {
		return true
	}
	if s.Role != "" && subject == SubjectPrefixRole+s.Role {
		return true
	}
	return false
}

// rank 는 레벨의 서열이다. none(0) < view(1) < edit(2).
func rank(level string) int {
	switch level {
	case LevelEdit:
		return 2
	case LevelView:
		return 1
	default:
		return 0
	}
}

// has 는 전역 RBAC 권한 보유 여부를 판정한다.
func has(s Subject, key string) bool {
	if s.Permissions == nil {
		return false
	}
	_, ok := s.Permissions[key]
	return ok
}
