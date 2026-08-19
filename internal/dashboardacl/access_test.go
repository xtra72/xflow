package dashboardacl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @SPEC:SPEC-DASHBOARD-004 (M2, spec.md §2.2, acceptance.md AC-04~AC-08, AC-16, AC-18)
//
// 본 파일은 Evaluate 의 진리표를 전수 검증한다 —
// (visibility 3) × (소유자 여부 2) × (ACL 레벨 3) × (전역 권한 조합 6) × (subject 형태 2).
//
// 기대값은 파생 로직이 아니라 **리터럴 표**로 둔다. 기대값을 계산해 버리면 구현과
// 같은 실수를 반복하게 되어 진리표가 아무것도 검증하지 못한다. 표에 없는 조합이
// 나오면 테스트가 실패하므로 셀 누락도 검출된다 — 셀 하나가 비면 그 조합에서
// 타인 대시보드가 노출된다.

// 전역 권한 조합 이름. access.go 의 permRead/permUpdate 등 권한 키 변수와
// 이름이 겹치지 않도록 set 접두사를 쓴다.
const (
	setNone       = "none"            // 대시보드 권한 없음
	setRead       = "read"            // viewer
	setReadUpd    = "read+update"     // operator (커스텀)
	setReadCrtUpd = "read+crt+upd"    // editor
	setReadUpdDel = "read+upd+del"    // 삭제는 있으나 create 없음 (관리자 아님)
	setAdmin      = "dashboard-admin" // 4종 전량 = 1단계 관리자
)

// permSets 는 권한 조합 이름 → 권한 키 집합이다.
// 권한 키는 access.go 가 internal/rbac 카탈로그에서 파생한 값을 그대로 쓴다.
var permSets = map[string][]string{
	setNone:       {},
	setRead:       {permRead},
	setReadUpd:    {permRead, permUpdate},
	setReadCrtUpd: {permRead, permCreate, permUpdate},
	setReadUpdDel: {permRead, permUpdate, permDelete},
	setAdmin:      {permRead, permCreate, permUpdate, permDelete},
}

// truthTable 은 권한 조합별 기대 판정이다.
//
// 키 형식: "<visibility>/<owner|other>/<acl none|view|edit>"
// 값 형식: 4문자 플래그 "VEDG" (해당 판정이 허용이면 문자, 거부면 '-').
var truthTable = map[string]map[string]string{
	// 권한이 하나도 없으면 전역 상한이 0 이므로 소유자라도 아무것도 못 한다.
	setNone: {
		"private/owner/none": "----", "private/owner/view": "----", "private/owner/edit": "----",
		"private/other/none": "----", "private/other/view": "----", "private/other/edit": "----",
		"shared/owner/none": "----", "shared/owner/view": "----", "shared/owner/edit": "----",
		"shared/other/none": "----", "shared/other/view": "----", "shared/other/edit": "----",
		"acl/owner/none": "----", "acl/owner/view": "----", "acl/owner/edit": "----",
		"acl/other/none": "----", "acl/other/view": "----", "acl/other/edit": "----",
	},
	// viewer — 조회만 가능. 소유자여도 편집 불가 (spec.md §2.6 수용된 회귀).
	setRead: {
		"private/owner/none": "V---", "private/owner/view": "V---", "private/owner/edit": "V---",
		"private/other/none": "----", "private/other/view": "----", "private/other/edit": "----",
		"shared/owner/none": "V---", "shared/owner/view": "V---", "shared/owner/edit": "V---",
		"shared/other/none": "V---", "shared/other/view": "V---", "shared/other/edit": "V---",
		"acl/owner/none": "V---", "acl/owner/view": "V---", "acl/owner/edit": "V---",
		// ACL 이 edit 을 부여해도 dashboard.update 가 없으면 편집 불가 (상한 규칙).
		"acl/other/none": "----", "acl/other/view": "V---", "acl/other/edit": "V---",
	},
	// operator — read+update. 소유자는 편집·권한부여 가능, 삭제는 불가.
	setReadUpd: {
		"private/owner/none": "VE-G", "private/owner/view": "VE-G", "private/owner/edit": "VE-G",
		"private/other/none": "----", "private/other/view": "----", "private/other/edit": "----",
		"shared/owner/none": "VE-G", "shared/owner/view": "VE-G", "shared/owner/edit": "VE-G",
		"shared/other/none": "V---", "shared/other/view": "V---", "shared/other/edit": "V---",
		"acl/owner/none": "VE-G", "acl/owner/view": "VE-G", "acl/owner/edit": "VE-G",
		"acl/other/none": "----", "acl/other/view": "V---", "acl/other/edit": "VE--",
	},
	// editor — create 가 더 있어도 4종 판정에는 영향이 없다 (operator 와 동일).
	setReadCrtUpd: {
		"private/owner/none": "VE-G", "private/owner/view": "VE-G", "private/owner/edit": "VE-G",
		"private/other/none": "----", "private/other/view": "----", "private/other/edit": "----",
		"shared/owner/none": "VE-G", "shared/owner/view": "VE-G", "shared/owner/edit": "VE-G",
		"shared/other/none": "V---", "shared/other/view": "V---", "shared/other/edit": "V---",
		"acl/owner/none": "VE-G", "acl/owner/view": "VE-G", "acl/owner/edit": "VE-G",
		"acl/other/none": "----", "acl/other/view": "V---", "acl/other/edit": "VE--",
	},
	// delete 는 있으나 create 가 없어 1단계 관리자가 아니다 — 자기 것만 삭제 가능.
	setReadUpdDel: {
		"private/owner/none": "VEDG", "private/owner/view": "VEDG", "private/owner/edit": "VEDG",
		"private/other/none": "----", "private/other/view": "----", "private/other/edit": "----",
		"shared/owner/none": "VEDG", "shared/owner/view": "VEDG", "shared/owner/edit": "VEDG",
		"shared/other/none": "V---", "shared/other/view": "V---", "shared/other/edit": "V---",
		"acl/owner/none": "VEDG", "acl/owner/view": "VEDG", "acl/owner/edit": "VEDG",
		// ACL 로 edit 을 받아도 남의 대시보드는 삭제·권한부여 불가.
		"acl/other/none": "----", "acl/other/view": "V---", "acl/other/edit": "VE--",
	},
	// 1단계 대시보드 관리자 — 소유·공개범위·ACL 과 무관하게 4종 전량 허용.
	setAdmin: {
		"private/owner/none": "VEDG", "private/owner/view": "VEDG", "private/owner/edit": "VEDG",
		"private/other/none": "VEDG", "private/other/view": "VEDG", "private/other/edit": "VEDG",
		"shared/owner/none": "VEDG", "shared/owner/view": "VEDG", "shared/owner/edit": "VEDG",
		"shared/other/none": "VEDG", "shared/other/view": "VEDG", "shared/other/edit": "VEDG",
		"acl/owner/none": "VEDG", "acl/owner/view": "VEDG", "acl/owner/edit": "VEDG",
		"acl/other/none": "VEDG", "acl/other/view": "VEDG", "acl/other/edit": "VEDG",
	},
}

// parseFlags 는 "VEDG" 플래그 문자열을 Access 로 변환한다.
func parseFlags(t *testing.T, flags string) Access {
	t.Helper()
	require.Len(t, flags, 4, "플래그는 4문자여야 한다: %q", flags)
	return Access{
		View:   flags[0] == 'V',
		Edit:   flags[1] == 'E',
		Delete: flags[2] == 'D',
		Grant:  flags[3] == 'G',
	}
}

// makeSubject 는 권한 조합 이름으로 인증 활성 Subject 를 만든다.
func makeSubject(permName string) Subject {
	perms := make(map[string]struct{})
	for _, key := range permSets[permName] {
		perms[key] = struct{}{}
	}
	return Subject{
		Username:    "alice",
		Role:        "operator",
		Permissions: perms,
		AuthEnabled: true,
	}
}

// TestEvaluate_TruthTable 은 진리표 전 조합을 검증한다.
func TestEvaluate_TruthTable(t *testing.T) {
	visibilities := []string{VisibilityPrivate, VisibilityShared, VisibilityACL}
	ownerKinds := []string{"owner", "other"}
	aclKinds := []string{"none", "view", "edit"}
	// subject 형태 2종 — user: 와 role: 은 판정에서 동일하게 취급된다.
	subjectForms := []string{"user", "role"}

	permNames := []string{setNone, setRead, setReadUpd, setReadCrtUpd, setReadUpdDel, setAdmin}

	asserted := 0
	for _, permName := range permNames {
		table, ok := truthTable[permName]
		require.True(t, ok, "진리표에 권한 조합 %q 가 없다", permName)

		for _, vis := range visibilities {
			for _, ownerKind := range ownerKinds {
				for _, aclKind := range aclKinds {
					for _, form := range subjectForms {
						key := fmt.Sprintf("%s/%s/%s", vis, ownerKind, aclKind)
						want, ok := table[key]
						require.True(t, ok, "진리표 셀 누락: %s / %s", permName, key)

						s := makeSubject(permName)
						d := Dashboard{UID: "D1", Visibility: vis, Owner: "bob"}
						if ownerKind == "owner" {
							d.Owner = s.Username
						}

						var acl []ACLEntry
						if aclKind != "none" {
							subject := SubjectPrefixUser + s.Username
							if form == "role" {
								subject = SubjectPrefixRole + s.Role
							}
							acl = []ACLEntry{{Subject: subject, Level: aclKind}}
						}

						got := Evaluate(s, d, acl)
						assert.Equal(t, parseFlags(t, want), got,
							"perm=%s vis=%s owner=%s acl=%s subjectForm=%s", permName, vis, ownerKind, aclKind, form)
						asserted++
					}
				}
			}
		}
	}

	// 6 조합 × 3 visibility × 2 소유 × 3 ACL × 2 subject 형태 = 216 셀.
	assert.Equal(t, 216, asserted, "진리표 셀 수가 기대와 다르다")
}

// TestEvaluate_AuthDisabled 는 인증 비활성 배포에서 4종이 모두 허용됨을 검증한다
// (spec.md §2.10 S1, acceptance.md AC-18).
func TestEvaluate_AuthDisabled(t *testing.T) {
	for _, vis := range []string{VisibilityPrivate, VisibilityShared, VisibilityACL} {
		s := Subject{AuthEnabled: false} // 사용자·권한 정보 일절 없음
		d := Dashboard{UID: "D1", Owner: "bob", Visibility: vis}

		got := Evaluate(s, d, nil)
		assert.Equal(t, Access{View: true, Edit: true, Delete: true, Grant: true}, got,
			"인증 비활성 상태의 %s 대시보드는 4종 모두 허용되어야 한다", vis)
	}
}

// TestEvaluate_EmptyUsernameIsNotOwner 는 인증 활성 상태에서 빈 사용자명이
// 빈 owner 와 매치되지 않음을 검증한다.
//
// 인증 비활성 시 생성된 대시보드의 owner 는 ” 이며, 이후 인증을 활성화하면 그
// 대시보드는 visibility 로만 접근이 결정되어야 한다 — 소유자 특권이 없다
// (spec.md §2.10).
func TestEvaluate_EmptyUsernameIsNotOwner(t *testing.T) {
	s := Subject{
		Username:    "",
		Role:        "",
		Permissions: map[string]struct{}{permRead: {}, permUpdate: {}},
		AuthEnabled: true,
	}
	d := Dashboard{UID: "D1", Owner: "", Visibility: VisibilityPrivate}

	got := Evaluate(s, d, nil)
	assert.Equal(t, Access{}, got, "빈 사용자명은 빈 owner 의 소유자가 아니다")

	// 같은 대시보드가 shared 이면 조회만 가능해진다 (visibility 로만 결정).
	d.Visibility = VisibilityShared
	got = Evaluate(s, d, nil)
	assert.Equal(t, Access{View: true}, got, "shared 는 조회만 허용")
}

// TestEvaluate_ACLHighestLevelWins 는 user: 와 role: 이 동시에 매치될 때
// 높은 레벨이 적용됨을 검증한다 (spec.md §2.2, acceptance.md AC-07).
func TestEvaluate_ACLHighestLevelWins(t *testing.T) {
	s := Subject{
		Username:    "ops",
		Role:        "operator",
		Permissions: map[string]struct{}{permRead: {}, permUpdate: {}},
		AuthEnabled: true,
	}
	d := Dashboard{UID: "D3", Owner: "edi", Visibility: VisibilityACL}

	// user:view + role:edit → edit 이 이긴다.
	got := Evaluate(s, d, []ACLEntry{
		{Subject: SubjectPrefixUser + "ops", Level: LevelView},
		{Subject: SubjectPrefixRole + "operator", Level: LevelEdit},
	})
	assert.Equal(t, Access{View: true, Edit: true}, got, "높은 레벨(edit)이 적용되어야 한다")

	// 순서를 뒤집어도 결과가 같다.
	got = Evaluate(s, d, []ACLEntry{
		{Subject: SubjectPrefixRole + "operator", Level: LevelEdit},
		{Subject: SubjectPrefixUser + "ops", Level: LevelView},
	})
	assert.Equal(t, Access{View: true, Edit: true}, got, "ACL 행 순서는 판정에 영향을 주지 않는다")
}

// TestEvaluate_UnrelatedACLEntriesIgnored 는 매치되지 않는 잔여 ACL 행이
// 접근을 부여하지 않음을 검증한다 (삭제된 사용자·역할의 유령 권한 방지).
func TestEvaluate_UnrelatedACLEntriesIgnored(t *testing.T) {
	s := Subject{
		Username:    "ops",
		Role:        "operator",
		Permissions: map[string]struct{}{permRead: {}, permUpdate: {}},
		AuthEnabled: true,
	}
	d := Dashboard{UID: "D3", Owner: "edi", Visibility: VisibilityACL}

	got := Evaluate(s, d, []ACLEntry{
		{Subject: SubjectPrefixUser + "someone-else", Level: LevelEdit},
		{Subject: SubjectPrefixRole + "deleted-role", Level: LevelEdit},
		{Subject: "group:dev", Level: LevelEdit}, // 미지원 접두사
	})
	assert.Equal(t, Access{}, got, "매치되지 않는 ACL 행은 접근을 부여하지 않는다")
}

// TestSubjects 는 ACL 질의용 subject 문자열 생성을 검증한다.
func TestSubjects(t *testing.T) {
	assert.Equal(t, []string{"user:alice", "role:operator"},
		Subjects(Subject{Username: "alice", Role: "operator"}))
	assert.Equal(t, []string{"role:operator"},
		Subjects(Subject{Role: "operator"}))
	assert.Equal(t, []string{"user:alice"},
		Subjects(Subject{Username: "alice"}))
	assert.Empty(t, Subjects(Subject{}), "식별자가 없으면 매치 대상도 없다")
}

// TestIsValidLevel 은 저장 가능한 ACL 레벨 판정을 검증한다.
func TestIsValidLevel(t *testing.T) {
	assert.True(t, IsValidLevel(LevelView))
	assert.True(t, IsValidLevel(LevelEdit))
	assert.False(t, IsValidLevel("manage"), "미지원 레벨은 거부")
	assert.False(t, IsValidLevel(""), "빈 레벨은 거부")
}

// TestIsValidVisibility 는 저장 가능한 공개 범위 판정을 검증한다.
func TestIsValidVisibility(t *testing.T) {
	assert.True(t, IsValidVisibility(VisibilityPrivate))
	assert.True(t, IsValidVisibility(VisibilityShared))
	assert.True(t, IsValidVisibility(VisibilityACL))
	assert.False(t, IsValidVisibility("public"))
	assert.False(t, IsValidVisibility(""))
}
