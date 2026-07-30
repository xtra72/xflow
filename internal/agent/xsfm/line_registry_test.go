package xsfm

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// SPEC-XSFM-LINE-001 — 라인 1급 엔티티 인수 테스트 (acceptance.md §1~§6, M1/M2/M4/M5)
// Given-When-Then 기계 검증. Go testing + testify.
// ---------------------------------------------------------------------------

// listLinesResult 는 list_lines 응답의 lines 배열을 파싱한다.
func listLinesResult(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var out struct {
		Lines []map[string]any `json:"lines"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	return out.Lines
}

// findLine 은 list_lines 결과에서 code 로 라인을 찾는다.
func findLine(lines []map[string]any, code string) map[string]any {
	for _, l := range lines {
		if l["code"] == code {
			return l
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// §1. 라인 레지스트리 — 순수 유닛 (LineRegistry)
// ---------------------------------------------------------------------------

// AC-1.4: 영속 왕복(write-through) — UpsertLine 후 재로드 복원.
func TestLineRegistry_PersistRoundtrip(t *testing.T) {
	dir := t.TempDir()
	r1, err := newLineRegistry(dir)
	require.NoError(t, err)
	require.NoError(t, r1.UpsertLine(Line{Code: "line_1", Name: "1호선", Order: 1}))

	r2, err := newLineRegistry(dir)
	require.NoError(t, err)
	got, err := r2.GetLine("line_1")
	require.NoError(t, err)
	assert.Equal(t, "line_1", got.Code)
	assert.Equal(t, "1호선", got.Name)
	assert.Equal(t, 1, got.Order)
}

// AC-1.3: ListLines Order 오름차순(동률 시 code 사전순).
func TestLineRegistry_ListOrder(t *testing.T) {
	r, err := newLineRegistry("")
	require.NoError(t, err)
	require.NoError(t, r.UpsertLine(Line{Code: "line_2", Name: "2호선", Order: 2}))
	require.NoError(t, r.UpsertLine(Line{Code: "line_1", Name: "1호선", Order: 1}))
	list := r.ListLines()
	require.Len(t, list, 2)
	assert.Equal(t, "line_1", list[0].Code)
	assert.Equal(t, "line_2", list[1].Code)
}

// RemoveLine 미존재 → ErrLineNotFound.
func TestLineRegistry_RemoveAbsent(t *testing.T) {
	r, err := newLineRegistry("")
	require.NoError(t, err)
	assert.ErrorIs(t, r.RemoveLine("nope"), ErrLineNotFound)
}

// AC-1.6: 락 동시성(-race) — 동시 add_line/list_lines.
func TestLineRegistry_ConcurrentRace(t *testing.T) {
	r, err := newLineRegistry("")
	require.NoError(t, err)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = r.UpsertLine(Line{Code: "line_x", Name: "x"}) }()
		go func() { defer wg.Done(); _ = r.ListLines() }()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// §1. add_line / remove_line / list_lines 명령 핸들러
// ---------------------------------------------------------------------------

// AC-1.1: add_line 생성 + list_lines 반영.
func TestAddLine_CreateAndList(t *testing.T) {
	ap := newGroupAgent(t, nil)
	_, err := procJSON(t, ap, map[string]any{
		"command": "add_line", "params": map[string]any{"code": "line_2", "name": "2호선"},
	})
	require.NoError(t, err)

	lg, err := procJSON(t, ap, map[string]any{"command": "list_lines"})
	require.NoError(t, err)
	lines := listLinesResult(t, lg)
	require.Len(t, lines, 1)
	assert.Equal(t, "line_2", lines[0]["code"])
	assert.Equal(t, "2호선", lines[0]["name"])
}

// AC-1.2: add_line upsert — 표시명 갱신(중복 신규 생성 아님).
func TestAddLine_Upsert(t *testing.T) {
	ap := newGroupAgent(t, nil)
	_, err := procJSON(t, ap, map[string]any{"command": "add_line", "params": map[string]any{"code": "line_2", "name": "2호선"}})
	require.NoError(t, err)
	_, err = procJSON(t, ap, map[string]any{"command": "add_line", "params": map[string]any{"code": "line_2", "name": "2호선(순환)"}})
	require.NoError(t, err)

	lg, _ := procJSON(t, ap, map[string]any{"command": "list_lines"})
	lines := listLinesResult(t, lg)
	require.Len(t, lines, 1) // 1개 유지.
	assert.Equal(t, "2호선(순환)", lines[0]["name"])
}

// AC-1.10: add_line 코드 포맷 위반 거부(RD-6).
func TestAddLine_CodeFormatReject(t *testing.T) {
	ap := newGroupAgent(t, nil)
	for _, bad := range []string{"2 호선", "Line2", "_x", ""} {
		_, err := procJSON(t, ap, map[string]any{"command": "add_line", "params": map[string]any{"code": bad, "name": "x"}})
		assert.ErrorIsf(t, err, errForAddLine(bad), "code %q must be rejected", bad)
	}
	lg, _ := procJSON(t, ap, map[string]any{"command": "list_lines"})
	assert.Empty(t, listLinesResult(t, lg))
}

// errForAddLine 은 빈 코드는 ErrInvalidCommand(code 필수), 그 외 비적합은 ErrInvalidCode 를 기대한다.
func errForAddLine(code string) error {
	if code == "" {
		return ErrInvalidCommand
	}
	return ErrInvalidCode
}

// AC-1.7: remove_line(참조 없음) → 제거.
func TestRemoveLine_NoRef(t *testing.T) {
	ap := newGroupAgent(t, nil)
	_, err := procJSON(t, ap, map[string]any{"command": "add_line", "params": map[string]any{"code": "line_9", "name": "9"}})
	require.NoError(t, err)
	_, err = procJSON(t, ap, map[string]any{"command": "remove_line", "params": map[string]any{"code": "line_9"}})
	require.NoError(t, err)
	lg, _ := procJSON(t, ap, map[string]any{"command": "list_lines"})
	assert.Nil(t, findLine(listLinesResult(t, lg), "line_9"))
}

// AC-1.8: remove_line(참조 존재) → ErrLineInUse, 라인 잔존(RD-5).
func TestRemoveLine_InUse(t *testing.T) {
	ap := newGroupAgent(t, nil)
	_, err := procJSON(t, ap, map[string]any{"command": "add_line", "params": map[string]any{"code": "line_2", "name": "2"}})
	require.NoError(t, err)
	// 라인 line_2 를 참조하는 역사 등록.
	_, err = procJSON(t, ap, map[string]any{"command": "add_station", "params": map[string]any{"station": "st01", "line": "line_2"}})
	require.NoError(t, err)

	_, err = procJSON(t, ap, map[string]any{"command": "remove_line", "params": map[string]any{"code": "line_2"}})
	assert.ErrorIs(t, err, ErrLineInUse)

	lg, _ := procJSON(t, ap, map[string]any{"command": "list_lines"})
	assert.NotNil(t, findLine(listLinesResult(t, lg), "line_2"), "line_2 잔존")
}

// AC-1.9: remove_line(참조 제거 후 삭제 가능)(RD-5).
func TestRemoveLine_AfterRefCleared(t *testing.T) {
	ap := newGroupAgent(t, nil)
	_, _ = procJSON(t, ap, map[string]any{"command": "add_line", "params": map[string]any{"code": "line_2", "name": "2"}})
	_, _ = procJSON(t, ap, map[string]any{"command": "add_station", "params": map[string]any{"station": "st01", "line": "line_2"}})
	// 참조 역사 제거 → 참조 0개.
	_, err := procJSON(t, ap, map[string]any{"command": "remove_station", "params": map[string]any{"station": "st01"}})
	require.NoError(t, err)

	_, err = procJSON(t, ap, map[string]any{"command": "remove_line", "params": map[string]any{"code": "line_2"}})
	require.NoError(t, err)
	lg, _ := procJSON(t, ap, map[string]any{"command": "list_lines"})
	assert.Nil(t, findLine(listLinesResult(t, lg), "line_2"))
}

// AC-1.5 / AC-2.1: 빈 라인 유효 + line:<code> 파생 멤버십(참조 역사 디바이스).
func TestLine_DerivedMembership(t *testing.T) {
	ap := newGroupAgent(t, nil,
		map[string]any{"device_id": "d1", "station": "st01"},
		map[string]any{"device_id": "d2", "station": "st01"})
	_, _ = procJSON(t, ap, map[string]any{"command": "add_line", "params": map[string]any{"code": "line_2", "name": "2"}})
	_, _ = procJSON(t, ap, map[string]any{"command": "add_line", "params": map[string]any{"code": "line_9", "name": "9"}})
	_, _ = procJSON(t, ap, map[string]any{"command": "add_station", "params": map[string]any{"station": "st01", "line": "line_2"}})

	// line:line_2 멤버 = 참조 역사(st01) 디바이스 파생.
	members := ap.GroupMembers("line:line_2")
	assert.Equal(t, []string{"d1", "d2"}, members)

	// AC-1.5: 빈 라인 line_9(참조 역사 0개) → 빈 멤버 집합(에러 아님).
	assert.Empty(t, ap.GroupMembers("line:line_9"))
}

// ---------------------------------------------------------------------------
// §5. 디바이스 네이밍 (composeName, M4)
// ---------------------------------------------------------------------------

// AC-5.1/5.2/5.4: 4-세그먼트(라인 있음) vs 3-세그먼트(라인 없음) + 3자리 0-채움.
func TestComposeName_Segments(t *testing.T) {
	assert.Equal(t, "line_2:st01:pump:003", composeName("line_2", "st01", "pump", 3)) // AC-5.1
	assert.Equal(t, "line_2:st01:pump:007", composeName("line_2", "st01", "pump", 7)) // AC-5.2
	assert.Equal(t, "st99:pump:003", composeName("", "st99", "pump", 3))              // AC-5.4 (라인 세그먼트 생략)
}

// AC-5.1(통합): add_device 합성 주소 경로가 라인 해석으로 4-세그먼트 이름을 만든다.
func TestAddDevice_FourSegmentName(t *testing.T) {
	ap := newGroupAgent(t, map[string]any{
		"state_topic_template":   "xsfm/{station_code}/{place_code}/{device_index}/{attribute}/state",
		"command_topic_template": "xsfm/{station_code}/{place_code}/{device_index}/{attribute}/cmd",
	})
	_, _ = procJSON(t, ap, map[string]any{"command": "add_line", "params": map[string]any{"code": "line_2", "name": "2"}})
	_, _ = procJSON(t, ap, map[string]any{"command": "add_station", "params": map[string]any{"station": "st01", "line": "line_2"}})

	resp, err := procJSON(t, ap, map[string]any{
		"command": "add_device",
		"params":  map[string]any{"station": "st01", "place": "pump", "index": 3},
	})
	require.NoError(t, err)
	var r map[string]any
	require.NoError(t, json.Unmarshal(resp, &r))
	// 응답 name 은 finishAddDevice 가 device_id 를 반환하므로, 로스터에서 이름을 조회한다.
	id, _ := r["device_id"].(string)
	require.NotEmpty(t, id)
	ap.mu.RLock()
	name := ap.devices[id].Name
	ap.mu.RUnlock()
	assert.Equal(t, "line_2:st01:pump:003", name)
}

// AC-5.4(통합): 라인 없는 역사 → 3-세그먼트 자동 이름.
func TestAddDevice_ThreeSegmentName_NoLine(t *testing.T) {
	ap := newGroupAgent(t, map[string]any{
		"state_topic_template":   "xsfm/{station_code}/{place_code}/{device_index}/{attribute}/state",
		"command_topic_template": "xsfm/{station_code}/{place_code}/{device_index}/{attribute}/cmd",
	})
	// st99 는 라인 미지정(add_station 없음) → ResolveLine=="" → 3-세그먼트.
	resp, err := procJSON(t, ap, map[string]any{
		"command": "add_device",
		"params":  map[string]any{"station": "st99", "place": "pump", "index": 3},
	})
	require.NoError(t, err)
	var r map[string]any
	require.NoError(t, json.Unmarshal(resp, &r))
	id, _ := r["device_id"].(string)
	ap.mu.RLock()
	name := ap.devices[id].Name
	ap.mu.RUnlock()
	assert.Equal(t, "st99:pump:003", name)
}

// AC-5.6: 라인 후지정 시 자동 이름 4-세그먼트 재계산(sticky 아님).
func TestSetDevice_RecomputeAfterLineAssigned(t *testing.T) {
	ap := newGroupAgent(t, map[string]any{
		"state_topic_template":   "xsfm/{station_code}/{place_code}/{device_index}/{attribute}/state",
		"command_topic_template": "xsfm/{station_code}/{place_code}/{device_index}/{attribute}/cmd",
	})
	resp, _ := procJSON(t, ap, map[string]any{
		"command": "add_device", "params": map[string]any{"station": "st99", "place": "pump", "index": 3},
	})
	var r map[string]any
	require.NoError(t, json.Unmarshal(resp, &r))
	id := r["device_id"].(string)

	// 역사 st99 에 라인 지정.
	_, _ = procJSON(t, ap, map[string]any{"command": "add_line", "params": map[string]any{"code": "line_9", "name": "9"}})
	_, _ = procJSON(t, ap, map[string]any{"command": "add_station", "params": map[string]any{"station": "st99", "line": "line_9"}})
	// set_device 로 재계산 트리거(주소 재확인 — station 재지정).
	_, err := procJSON(t, ap, map[string]any{
		"command": "set_device", "device_id": id, "station": "st99",
	})
	require.NoError(t, err)

	ap.mu.RLock()
	name := ap.devices[id].Name
	ap.mu.RUnlock()
	assert.Equal(t, "line_9:st99:pump:003", name)
}

// AC-5.7: 라인 후지정에도 sticky(nameOverridden) 이름 보존.
func TestSetDevice_StickyPreservedAfterLineAssigned(t *testing.T) {
	ap := newGroupAgent(t, map[string]any{
		"state_topic_template":   "xsfm/{station_code}/{place_code}/{device_index}/{attribute}/state",
		"command_topic_template": "xsfm/{station_code}/{place_code}/{device_index}/{attribute}/cmd",
	})
	// 사용자 지정 이름으로 sticky 고정.
	resp, _ := procJSON(t, ap, map[string]any{
		"command": "add_device", "params": map[string]any{"station": "st99", "place": "pump", "index": 3, "name": "메인펌프"},
	})
	var r map[string]any
	require.NoError(t, json.Unmarshal(resp, &r))
	id := r["device_id"].(string)

	_, _ = procJSON(t, ap, map[string]any{"command": "add_line", "params": map[string]any{"code": "line_9", "name": "9"}})
	_, _ = procJSON(t, ap, map[string]any{"command": "add_station", "params": map[string]any{"station": "st99", "line": "line_9"}})
	_, err := procJSON(t, ap, map[string]any{"command": "set_device", "device_id": id, "station": "st99"})
	require.NoError(t, err)

	ap.mu.RLock()
	name := ap.devices[id].Name
	ap.mu.RUnlock()
	assert.Equal(t, "메인펌프", name) // sticky 보존.
}

// ---------------------------------------------------------------------------
// §4. 코드 기반 통일 주소 셀렉터 fan-out (M5)
// ---------------------------------------------------------------------------

// AC-4.1/4.2/4.3: line:/custom:/station: 코드 셀렉터가 올바른 멤버로 fan-out.
func TestSelectorFanOut_CodeBased(t *testing.T) {
	ap := newGroupAgent(t, nil,
		map[string]any{"device_id": "d1", "station": "st01"},
		map[string]any{"device_id": "d2", "station": "st01"})
	_, _ = procJSON(t, ap, map[string]any{"command": "add_line", "params": map[string]any{"code": "line_2", "name": "2"}})
	_, _ = procJSON(t, ap, map[string]any{"command": "add_station", "params": map[string]any{"station": "st01", "line": "line_2"}})
	_, _ = procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"code": "gpump", "name": "펌프", "members": []string{"d1"}}})

	// AC-4.1 line:line_2 → d1,d2.
	assert.Equal(t, []string{"d1", "d2"}, ap.GroupMembers("line:line_2"))
	// AC-4.2 custom:gpump → d1.
	assert.Equal(t, []string{"d1"}, ap.GroupMembers("custom:gpump"))
	// AC-4.3 station:st01 → d1,d2(무회귀).
	assert.Equal(t, []string{"d1", "d2"}, ap.GroupMembers("station:st01"))
}

// ---------------------------------------------------------------------------
// §6. 마이그레이션 (M2)
// ---------------------------------------------------------------------------

// AC-6.1: station.Line → Line 엔티티 ensure-create(로드 마이그레이션).
func TestMigration_LinesFromStations(t *testing.T) {
	ap := newGroupAgent(t, map[string]any{
		"station_registry": []any{
			map[string]any{"station": "st01", "line": "line_2"},
			map[string]any{"station": "st02", "line": "line_2"}, // 중복 line → 1개만 생성.
			map[string]any{"station": "st03", "line": "line_3"},
		},
	})
	lg, _ := procJSON(t, ap, map[string]any{"command": "list_lines"})
	lines := listLinesResult(t, lg)
	require.Len(t, lines, 2)
	l2 := findLine(lines, "line_2")
	require.NotNil(t, l2)
	assert.Equal(t, "line_2", l2["name"]) // name = code(레거시 승격 기본값).
	assert.NotNil(t, findLine(lines, "line_3"))
}

// AC-6.2: custom:<name>(포맷 적합) → custom:<code> 승격(재기동 마이그레이션).
func TestMigration_CustomGroupCode_Conforming(t *testing.T) {
	dir := t.TempDir()
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["registry_path"] = dir
	opts["devices"] = groupDevices(map[string]any{"device_id": "d1"})

	a1, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap1 := asAP(t, a1)
	_, err = procJSON(t, ap1, map[string]any{"command": "add_group", "params": map[string]any{"name": "pumps", "members": []string{"d1"}}})
	require.NoError(t, err)

	a2, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap2 := asAP(t, a2)
	g, err := ap2.groups.GetGroup("custom:pumps")
	require.NoError(t, err)
	assert.Equal(t, "pumps", g.Code)
	assert.Equal(t, []string{"d1"}, g.Members)
}

// AC-6.2a: 레거시 포맷 비적합 name slugify 마이그레이션.
func TestMigration_CustomGroupCode_Slugify(t *testing.T) {
	dir := t.TempDir()
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["registry_path"] = dir
	opts["devices"] = groupDevices(map[string]any{"device_id": "d1"}, map[string]any{"device_id": "d2"})

	a1, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap1 := asAP(t, a1)
	// 레거시 폼(code 키 없음) → custom:"2층 창고" 생성.
	_, err = procJSON(t, ap1, map[string]any{"command": "add_group", "params": map[string]any{"name": "2층 창고", "members": []string{"d1", "d2"}}})
	require.NoError(t, err)

	a2, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap2 := asAP(t, a2)
	g, err := ap2.groups.GetGroup("custom:2cheung-changgo")
	require.NoError(t, err)
	assert.Equal(t, "2cheung-changgo", g.Code)
	assert.Equal(t, "2층 창고", g.Name) // 원문 표시값 보존.
	assert.Equal(t, []string{"d1", "d2"}, g.Members)
}

// AC-6.2b: slugify 코드 충돌 → 접미 번호 유일성.
func TestMigration_CustomGroupCode_CollisionSuffix(t *testing.T) {
	dir := t.TempDir()
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["registry_path"] = dir

	a1, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap1 := asAP(t, a1)
	// 두 레거시 그룹이 동일 slug("2cheung-changgo")로 수렴.
	_, err = procJSON(t, ap1, map[string]any{"command": "add_group", "params": map[string]any{"name": "2층 창고"}})
	require.NoError(t, err)
	_, err = procJSON(t, ap1, map[string]any{"command": "add_group", "params": map[string]any{"name": "2층-창고"}})
	require.NoError(t, err)

	a2, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap2 := asAP(t, a2)
	_, err = ap2.groups.GetGroup("custom:2cheung-changgo")
	require.NoError(t, err)
	_, err = ap2.groups.GetGroup("custom:2cheung-changgo-2")
	require.NoError(t, err, "충돌 그룹은 접미 번호로 유일성 보장")
}

// AC-6.3/6.4: 멱등성(재기동 2회 동일) + 비파괴.
func TestMigration_Idempotent(t *testing.T) {
	dir := t.TempDir()
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["registry_path"] = dir
	opts["station_registry"] = []any{map[string]any{"station": "st01", "line": "line_2"}}

	// 3회 기동 — 상태 동일해야 한다.
	var lastLines, lastGroups int
	for i := 0; i < 3; i++ {
		a, err := NewXSFMAgent(baseAgentConfig(opts))
		require.NoError(t, err)
		ap := asAP(t, a)
		if i == 0 {
			_, _ = procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "2층 창고"}})
		}
		lg, _ := procJSON(t, ap, map[string]any{"command": "list_lines"})
		lines := listLinesResult(t, lg)
		gg, _ := procJSON(t, ap, map[string]any{"command": "list_groups"})
		var gr struct {
			Groups []map[string]any `json:"groups"`
		}
		require.NoError(t, json.Unmarshal(gg, &gr))
		if i > 0 {
			assert.Equal(t, lastLines, len(lines), "라인 수 멱등")
			assert.Equal(t, lastGroups, len(gr.Groups), "그룹 수 멱등")
		}
		lastLines, lastGroups = len(lines), len(gr.Groups)
	}
}
