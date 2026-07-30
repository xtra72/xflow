package xsfm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// SPEC-XSFM-LINE-001 — 통일 코드 포맷 & slugify 인수 테스트 (§4.6, RD-6, M2/M3)
// ---------------------------------------------------------------------------

// AC-1.10/AC-3.3: 통일 코드 포맷 검증 — 적합/비적합 집합.
func TestValidCode(t *testing.T) {
	valid := []string{"gpump", "line_2", "2cheung-changgo", "st01", "a", "0", "g1a2b3c4d"}
	for _, c := range valid {
		assert.Truef(t, validCode(c), "expected %q valid", c)
	}
	// AC-3.3 검증 노트의 거부 집합: 공백·한글, 대문자, 선두 언더스코어, 빈 문자열.
	invalid := []string{"펌프 군", "Pumps", "_pump", "", "2 호선", "Line2", "-x", "a:b", "café"}
	for _, c := range invalid {
		assert.Falsef(t, validCode(c), "expected %q invalid", c)
	}
}

// AC-6.2a: slugify 워크드 예시 — "2층 창고" → "2cheung-changgo".
func TestSlugify_WorkedExample(t *testing.T) {
	assert.Equal(t, "2cheung-changgo", slugify("2층 창고"))
}

// slugify 추가 케이스: 한글 Revised Romanization 자모 매핑·트림·하이픈 축약.
func TestSlugify_Cases(t *testing.T) {
	cases := map[string]string{
		"2층 창고":      "2cheung-changgo",
		"2층-창고":      "2cheung-changgo", // 하이픈 입력도 동일 slug(충돌 유발 케이스).
		"펌프 군":       "peompeu-gun",     // ㅍ=p, ㅓ=eo, ㅁ tail=m / ㄱ=g, ㅜ=u, ㄴ tail=n.
		"Pumps":      "pumps",           // 대문자 → 소문자.
		"  spaced  ": "spaced",          // 양끝 공백/하이픈 트림.
		"a--b":       "a-b",             // 연속 하이픈 축약.
		"강남":         "gangnam",         // ㄱㅏㅇ=gang, ㄴㅏㅁ=nam.
		"line_2":     "line_2",          // 언더스코어 보존.
	}
	for in, want := range cases {
		assert.Equalf(t, want, slugify(in), "slugify(%q)", in)
	}
}

// slugify 해시 폴백: 로마자화 불가한 순수 비-ASCII 는 g<hex8> 폴백을 쓴다(§4.6 규칙 5).
func TestSlugify_HashFallback(t *testing.T) {
	got := slugify("日本語")
	require.NotEmpty(t, got)
	assert.Equal(t, byte('g'), got[0])
	assert.Len(t, got, 9) // "g" + 8 hex.
	assert.True(t, validCode(got), "hash fallback must satisfy code format")
	// 결정적: 동일 입력은 동일 폴백.
	assert.Equal(t, got, slugify("日本語"))
}

// ---------------------------------------------------------------------------
// M3 — 커스텀 그룹 코드 (add_group{code,name,members}) 인수 테스트
// ---------------------------------------------------------------------------

// AC-3.1: add_group 코드 기반 id (custom:<code>) + name 표시 전용.
func TestAddGroup_CodeBasedID(t *testing.T) {
	ap := newGroupAgent(t, nil,
		map[string]any{"device_id": "d1"}, map[string]any{"device_id": "d2"})

	resp, err := procJSON(t, ap, map[string]any{
		"command": "add_group",
		"params":  map[string]any{"code": "gpump", "name": "펌프군", "members": []string{"d1", "d2"}},
	})
	require.NoError(t, err)
	assert.Contains(t, string(resp), "custom:gpump")

	g, err := ap.groups.GetGroup("custom:gpump")
	require.NoError(t, err)
	assert.Equal(t, "custom:gpump", g.ID)
	assert.Equal(t, "gpump", g.Code)
	assert.Equal(t, "펌프군", g.Name) // 표시 전용.
	assert.Equal(t, []string{"d1", "d2"}, g.Members)
}

// AC-3.2: 코드 중복 거부 → ErrGroupAlreadyExists.
func TestAddGroup_DuplicateCode(t *testing.T) {
	ap := newGroupAgent(t, nil, map[string]any{"device_id": "d1"})
	_, err := procJSON(t, ap, map[string]any{
		"command": "add_group", "params": map[string]any{"code": "gpump", "name": "펌프군"},
	})
	require.NoError(t, err)

	_, err = procJSON(t, ap, map[string]any{
		"command": "add_group", "params": map[string]any{"code": "gpump", "name": "재생성"},
	})
	assert.ErrorIs(t, err, ErrGroupAlreadyExists)
}

// AC-3.3: 코드 포맷 위반 거부 (RD-6). code 키가 존재하지만 비적합이면 거부.
func TestAddGroup_CodeFormatReject(t *testing.T) {
	ap := newGroupAgent(t, nil, map[string]any{"device_id": "d1"})
	for _, bad := range []string{"펌프 군", "Pumps", "_pump", ""} {
		_, err := procJSON(t, ap, map[string]any{
			"command": "add_group", "params": map[string]any{"code": bad, "name": "x"},
		})
		assert.ErrorIsf(t, err, ErrInvalidCode, "code %q must be rejected", bad)
	}
	// 그룹이 생성되지 않았음을 확인(레지스트리 비어있음).
	assert.Empty(t, ap.groups.ListGroups())
}

// 무회귀: code 키 없는 레거시 add_group{name} 은 custom:<name> 폴백을 유지한다(GROUP-001 보존).
func TestAddGroup_LegacyNameFallback(t *testing.T) {
	ap := newGroupAgent(t, nil, map[string]any{"device_id": "d1"})
	resp, err := procJSON(t, ap, map[string]any{
		"command": "add_group", "params": map[string]any{"name": "floor2", "members": []string{"d1"}},
	})
	require.NoError(t, err)
	assert.Contains(t, string(resp), "custom:floor2")
	g, err := ap.groups.GetGroup("custom:floor2")
	require.NoError(t, err)
	assert.Equal(t, "floor2", g.Code)
}
