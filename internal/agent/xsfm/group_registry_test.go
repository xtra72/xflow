package xsfm

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// SPEC-XSFM-GROUP-001 — 그룹 1급 개념 인수 테스트 (acceptance.md Module 1~5, 7)
//
// 각 테스트는 Given-When-Then 시나리오를 기계 검증한다. 순수 방출 검증은
// control_response_timeout=0(fire-and-forget)로 응답 대기 없이 즉시 반환한다.
// ---------------------------------------------------------------------------

// newGroupAgent 는 devices 시드로 direct 모드 에이전트를 만들고 *XSFMAgent 를 반환한다.
func newGroupAgent(t *testing.T, extra map[string]any, devices ...map[string]any) *XSFMAgent {
	t.Helper()
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	for k, v := range extra {
		opts[k] = v
	}
	if len(devices) > 0 {
		opts["devices"] = groupDevices(devices...)
	}
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	return asAP(t, a)
}

// procJSON 은 map 을 JSON 으로 직렬화해 Process 에 넘긴다.
func procJSON(t *testing.T, ap *XSFMAgent, req map[string]any) ([]byte, error) {
	t.Helper()
	raw, err := json.Marshal(req)
	require.NoError(t, err)
	return ap.Process(raw)
}

// listGroupsResult 는 list_groups 응답을 파싱한다.
func listGroupsResult(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var out struct {
		Groups []map[string]any `json:"groups"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	return out.Groups
}

// findGroup 은 list_groups 결과에서 id 로 그룹을 찾는다.
func findGroup(groups []map[string]any, id string) map[string]any {
	for _, g := range groups {
		if g["id"] == id {
			return g
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Module 1 — 그룹 레지스트리 · 엔티티
// ---------------------------------------------------------------------------

// Scenario 1.1: 그룹 엔티티 생성·조회 (UpsertGroup/GetGroup/ListGroups).
func TestGroupRegistry_UpsertGetList(t *testing.T) {
	r, err := newGroupRegistry("")
	require.NoError(t, err)

	require.NoError(t, r.UpsertGroup(Group{ID: "custom:a", Name: "A", Type: groupTypeCustom, Members: []string{"d1"}}))

	got, err := r.GetGroup("custom:a")
	require.NoError(t, err)
	assert.Equal(t, "custom:a", got.ID)
	assert.Equal(t, "A", got.Name)
	assert.Equal(t, groupTypeCustom, got.Type)
	assert.Equal(t, []string{"d1"}, got.Members)

	list := r.ListGroups()
	require.Len(t, list, 1)
	assert.Equal(t, "custom:a", list[0].ID)
}

// Scenario 1.1c: 멤버 추가/제거 내부 API (AddMember/RemoveMember).
func TestGroupRegistry_AddRemoveMember(t *testing.T) {
	r, err := newGroupRegistry("")
	require.NoError(t, err)
	require.NoError(t, r.UpsertGroup(Group{ID: "custom:a", Name: "A", Type: groupTypeCustom}))

	// AddMember (중복은 무시, 정렬 유지).
	require.NoError(t, r.AddMember("custom:a", "d2"))
	require.NoError(t, r.AddMember("custom:a", "d1"))
	require.NoError(t, r.AddMember("custom:a", "d1")) // 중복.
	g, err := r.GetGroup("custom:a")
	require.NoError(t, err)
	assert.Equal(t, []string{"d1", "d2"}, g.Members)

	// RemoveMember (removeString 경로).
	require.NoError(t, r.RemoveMember("custom:a", "d1"))
	g, err = r.GetGroup("custom:a")
	require.NoError(t, err)
	assert.Equal(t, []string{"d2"}, g.Members)

	// 멤버 부재 제거는 no-op(에러 아님, 비침습).
	require.NoError(t, r.RemoveMember("custom:a", "ghost"))

	// 미등록 그룹 → ErrGroupNotFound.
	assert.ErrorIs(t, r.AddMember("custom:nope", "d1"), ErrGroupNotFound)
	assert.ErrorIs(t, r.RemoveMember("custom:nope", "d1"), ErrGroupNotFound)

	// 기본 그룹 대상 멤버 편집 → ErrGroupNotCustom.
	assert.ErrorIs(t, r.AddMember("station:S", "d1"), ErrGroupNotCustom)
	assert.ErrorIs(t, r.RemoveMember("line:L", "d1"), ErrGroupNotCustom)

	// membersOf 미등록 → ok=false.
	_, ok := r.membersOf("custom:nope")
	assert.False(t, ok)
}

// Scenario 1.1b: UpsertGroup 은 기본 그룹(비-custom id)을 거부한다.
func TestGroupRegistry_UpsertRejectsBaseGroup(t *testing.T) {
	r, err := newGroupRegistry("")
	require.NoError(t, err)
	err = r.UpsertGroup(Group{ID: "station:S", Name: "S", Type: groupTypeStation})
	assert.ErrorIs(t, err, ErrGroupNotCustom)
}

// Scenario 1.2: 별도 레이어 — 그룹 CRUD 가 station 레지스트리·로스터를 변경하지 않는다.
func TestGroupRegistry_SeparateLayerNoRegistryMutation(t *testing.T) {
	ap := newGroupAgent(t, map[string]any{
		"station_registry": map[string]any{"ST-101": map[string]any{"line": "line-2", "display_name": "강남"}},
	}, map[string]any{"device_id": "d1", "station": "ST-101"})

	// 그룹 CRUD 수행.
	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "floor2", "members": []string{"d1"}}})
	require.NoError(t, err)

	// station 레지스트리 항목 불변.
	entry, err := ap.stations.GetStation("ST-101")
	require.NoError(t, err)
	assert.Equal(t, "강남", entry.DisplayName)
	assert.Equal(t, "line-2", entry.Line)

	// 로스터 디바이스 위치 속성 불변.
	dev, err := ap.GetDevice("d1")
	require.NoError(t, err)
	assert.Equal(t, "ST-101", dev.Station)
}

// Scenario 1.3: 커스텀 그룹 영속 write-through + 재시작 복원.
func TestGroupRegistry_PersistRoundtripRestart(t *testing.T) {
	dir := t.TempDir()
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["registry_path"] = dir
	opts["devices"] = groupDevices(map[string]any{"device_id": "d1"}, map[string]any{"device_id": "d2"})

	a1, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap1 := asAP(t, a1)

	_, err = procJSON(t, ap1, map[string]any{"command": "add_group", "params": map[string]any{"name": "floor2", "members": []string{"d1", "d2"}}})
	require.NoError(t, err)

	// 재기동 (동일 경로) → 커스텀 그룹 복원.
	a2, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap2 := asAP(t, a2)

	got, err := ap2.groups.GetGroup("custom:floor2")
	require.NoError(t, err)
	assert.Equal(t, "floor2", got.Name)
	assert.Equal(t, []string{"d1", "d2"}, got.Members)
}

// Scenario 1.4: 락 중첩 없음 — 그룹 조회와 fan-out 동시 실행 (`go test -race`).
func TestGroupRegistry_ConcurrentQueryAndFanOut(t *testing.T) {
	ap, _ := directAgentWithMock(t, func() map[string]any {
		o := directOpts()
		o["control_response_timeout"] = "0s"
		o["station_registry"] = map[string]any{"ST-101": map[string]any{"line": "line-2"}}
		o["devices"] = groupDevices(
			map[string]any{"device_id": "d1", "station": "ST-101"},
			map[string]any{"device_id": "d2", "station": "ST-101"},
		)
		return o
	}())

	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "x", "members": []string{"d1", "d2"}}})
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(4)
		go func() { defer wg.Done(); _ = ap.GroupMembers("custom:x") }()
		go func() { defer wg.Done(); _ = ap.GroupsForDevice("d1") }()
		go func() { defer wg.Done(); _, _ = ap.handleListGroups() }()
		go func() {
			defer wg.Done()
			_, _ = ap.dispatchControl(processRequest{Command: "set_power", GroupID: "custom:x", Params: map[string]any{"power": true}})
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// Module 2 — 다대다 멤버십
// ---------------------------------------------------------------------------

// Scenario 2.1: 디바이스 다중 소속 (station 파생 + custom 명시).
func TestGroup_DeviceMultiMembership(t *testing.T) {
	ap := newGroupAgent(t, map[string]any{
		"station_registry": map[string]any{"S": map[string]any{"line": "L"}},
	}, map[string]any{"device_id": "d1", "station": "S"})
	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "x", "members": []string{"d1"}}})
	require.NoError(t, err)

	groups := ap.GroupsForDevice("d1")
	assert.Contains(t, groups, "station:S")
	assert.Contains(t, groups, "custom:x")
	assert.Contains(t, groups, "line:L")
}

// Scenario 2.2: 커스텀 멤버 도출 (정렬).
func TestGroup_CustomMembersSorted(t *testing.T) {
	ap := newGroupAgent(t, nil,
		map[string]any{"device_id": "d2"}, map[string]any{"device_id": "d1"})
	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "x", "members": []string{"d2", "d1"}}})
	require.NoError(t, err)

	assert.Equal(t, []string{"d1", "d2"}, ap.GroupMembers("custom:x"))
}

// Scenario 2.3: 기본 그룹 파생 도출 (station: == DevicesByStation).
func TestGroup_StationDerivedMembers(t *testing.T) {
	ap := newGroupAgent(t, map[string]any{
		"station_registry": map[string]any{"S": map[string]any{"line": "L"}},
	},
		map[string]any{"device_id": "d1", "station": "S"},
		map[string]any{"device_id": "d2", "station": "S"})

	assert.Equal(t, ap.DevicesByStation("S"), ap.GroupMembers("station:S"))
	assert.Equal(t, []string{"d1", "d2"}, ap.GroupMembers("station:S"))
}

// Scenario 2.4: 기존 group_id 마이그레이션 + primary 유지 (RD-1).
func TestGroup_MigrationPreservesPrimary(t *testing.T) {
	ap := newGroupAgent(t, nil, map[string]any{"device_id": "d1", "group_id": "legacy1"})

	// (1) custom:legacy1 그룹 생성 + d1 편입.
	g, err := ap.groups.GetGroup("custom:legacy1")
	require.NoError(t, err)
	assert.Equal(t, []string{"d1"}, g.Members)

	// (2) d1.GroupID 는 "legacy1" 로 유지(primary — 폐기되지 않음).
	dev, err := ap.GetDevice("d1")
	require.NoError(t, err)
	assert.Equal(t, "legacy1", dev.GroupID)
}

// Scenario 2.5: 위치 변경 즉시 반영 (파생).
func TestGroup_LocationChangeReflectedImmediately(t *testing.T) {
	ap := newGroupAgent(t, nil, map[string]any{"device_id": "d1", "station": "S"})

	assert.Equal(t, []string{"d1"}, ap.GroupMembers("station:S"))

	_, err := procJSON(t, ap, map[string]any{"command": "set_device", "device_id": "d1", "params": map[string]any{"station": "T"}})
	require.NoError(t, err)

	assert.Empty(t, ap.GroupMembers("station:S"))
	assert.Equal(t, []string{"d1"}, ap.GroupMembers("station:T"))
}

// Scenario 2.6: 유령 멤버 조회 필터 (RD-3) — remove_device 는 그룹 멤버십을 건드리지 않음.
func TestGroup_GhostMemberFilter(t *testing.T) {
	ap := newGroupAgent(t, nil,
		map[string]any{"device_id": "d1", "source": "bridge"},
		map[string]any{"device_id": "d2", "source": "bridge"})
	// bridge 로 등록해 remove 가능하도록 add_device 로 재등록(config 는 보호됨).
	_, err := procJSON(t, ap, map[string]any{"command": "add_device", "device_id": "b1"})
	require.NoError(t, err)
	_, err = procJSON(t, ap, map[string]any{"command": "add_device", "device_id": "b2"})
	require.NoError(t, err)
	_, err = procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "x", "members": []string{"b1", "b2"}}})
	require.NoError(t, err)

	// b2 삭제 (그룹 멤버십은 건드리지 않음).
	_, err = procJSON(t, ap, map[string]any{"command": "remove_device", "device_id": "b2"})
	require.NoError(t, err)

	// 조회는 유령 멤버 b2 를 자동 필터.
	assert.Equal(t, []string{"b1"}, ap.GroupMembers("custom:x"))

	// remove_device 는 저장된 멤버 목록을 건드리지 않는다(비침습, 여전히 b1,b2 저장).
	g, err := ap.groups.GetGroup("custom:x")
	require.NoError(t, err)
	assert.Equal(t, []string{"b1", "b2"}, g.Members)
}

// Scenario 2.7: primary 그룹 단일 태그 방출 무회귀 (RD-1).
func TestGroup_PrimarySingleTagNoRegression(t *testing.T) {
	ap := newGroupAgent(t, map[string]any{
		"station_registry": map[string]any{"S": map[string]any{"line": "L"}},
	}, map[string]any{"device_id": "d1", "group_id": "legacy1", "station": "S"})
	// 추가 커스텀 그룹에도 편입(다중 소속).
	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "extra", "members": []string{"d1"}}})
	require.NoError(t, err)

	// 다중 소속 확인.
	groups := ap.GroupsForDevice("d1")
	assert.GreaterOrEqual(t, len(groups), 3) // custom:legacy1, custom:extra, station:S, line:L

	// 단일 group_id 태그는 primary("legacy1")로 방출된다(request_state 는 status.go 경로).
	resp, err := procJSON(t, ap, map[string]any{"command": "request_state", "device_id": "d1"})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(resp, &m))
	assert.Equal(t, "legacy1", m["group_id"])
}

// Scenario 2.8: 그룹 id 접두사 인코딩 (RD-2) — 커스텀 name == station 코드여도 id 충돌 없음.
func TestGroup_PrefixEncodedIDNoCollision(t *testing.T) {
	ap := newGroupAgent(t, map[string]any{
		"station_registry": map[string]any{"S": map[string]any{"line": "L", "display_name": "S역"}},
	}, map[string]any{"device_id": "d1", "station": "S"})
	// 커스텀 그룹 이름을 station 코드 "S" 와 동일하게 생성.
	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "S", "members": []string{"d1"}}})
	require.NoError(t, err)

	resp, err := procJSON(t, ap, map[string]any{"command": "list_groups"})
	require.NoError(t, err)
	groups := listGroupsResult(t, resp)

	custom := findGroup(groups, "custom:S")
	station := findGroup(groups, "station:S")
	require.NotNil(t, custom, "custom:S 존재")
	require.NotNil(t, station, "station:S 존재(충돌 없음)")
	assert.Equal(t, groupTypeCustom, custom["type"])
	assert.Equal(t, groupTypeStation, station["type"])
}

// ---------------------------------------------------------------------------
// Module 3 — 커스텀 그룹 CRUD
// ---------------------------------------------------------------------------

// Scenario 3.1: add_group + group_registered 이벤트 + 목록 반영.
func TestGroup_AddGroup(t *testing.T) {
	ap, _ := directAgentWithMock(t, func() map[string]any {
		o := directOpts()
		o["control_response_timeout"] = "0s"
		o["devices"] = groupDevices(map[string]any{"device_id": "d1"})
		return o
	}())

	resp, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "2층", "members": []string{"d1"}}})
	require.NoError(t, err)
	var r map[string]any
	require.NoError(t, json.Unmarshal(resp, &r))
	assert.Equal(t, "ok", r["status"])
	assert.Equal(t, "custom:2층", r["group_id"])

	evt := readEvent(t, ap)
	assert.Equal(t, "group_registered", evt["type"])
	assert.Equal(t, "custom:2층", evt["group_id"])

	// 목록 반영.
	lg, err := procJSON(t, ap, map[string]any{"command": "list_groups"})
	require.NoError(t, err)
	assert.NotNil(t, findGroup(listGroupsResult(t, lg), "custom:2층"))
}

// Scenario 3.1b: add_group name 없으면 ErrInvalidCommand.
func TestGroup_AddGroupRequiresName(t *testing.T) {
	ap := newGroupAgent(t, nil)
	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{}})
	assert.ErrorIs(t, err, ErrInvalidCommand)
}

// Scenario 3.1c: 중복 add_group → ErrGroupAlreadyExists.
func TestGroup_AddGroupDuplicate(t *testing.T) {
	ap := newGroupAgent(t, nil)
	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "x"}})
	require.NoError(t, err)
	_, err = procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "x"}})
	assert.ErrorIs(t, err, ErrGroupAlreadyExists)
}

// Scenario 3.2: set_group 부분 갱신 (name 보존, members 갱신).
func TestGroup_SetGroupPartialUpdate(t *testing.T) {
	ap := newGroupAgent(t, nil,
		map[string]any{"device_id": "d1"}, map[string]any{"device_id": "d2"})
	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "A", "members": []string{"d1"}}})
	require.NoError(t, err)

	// members 만 갱신 → name "A" 보존.
	_, err = procJSON(t, ap, map[string]any{"command": "set_group", "params": map[string]any{"group_id": "custom:A", "members": []string{"d1", "d2"}}})
	require.NoError(t, err)

	g, err := ap.groups.GetGroup("custom:A")
	require.NoError(t, err)
	assert.Equal(t, "A", g.Name, "name 은 보존되어야 한다")
	assert.Equal(t, []string{"d1", "d2"}, g.Members)
}

// Scenario 3.3: remove_group + group_unregistered + 목록에서 제거.
func TestGroup_RemoveGroup(t *testing.T) {
	ap, _ := directAgentWithMock(t, func() map[string]any {
		o := directOpts()
		o["control_response_timeout"] = "0s"
		return o
	}())
	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "x"}})
	require.NoError(t, err)
	readEvent(t, ap) // group_registered 소진.

	_, err = procJSON(t, ap, map[string]any{"command": "remove_group", "params": map[string]any{"group_id": "custom:x"}})
	require.NoError(t, err)
	evt := readEvent(t, ap)
	assert.Equal(t, "group_unregistered", evt["type"])

	_, err = ap.groups.GetGroup("custom:x")
	assert.ErrorIs(t, err, ErrGroupNotFound)
}

// Scenario 3.4: list_groups (기본+커스텀) 결정적 순서.
func TestGroup_ListGroupsDeterministic(t *testing.T) {
	ap := newGroupAgent(t, map[string]any{
		"station_registry": map[string]any{"S": map[string]any{"line": "L", "display_name": "S역"}},
	}, map[string]any{"device_id": "d1", "station": "S"})
	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "custom1", "members": []string{"d1"}}})
	require.NoError(t, err)

	resp, err := procJSON(t, ap, map[string]any{"command": "list_groups"})
	require.NoError(t, err)
	groups := listGroupsResult(t, resp)

	// custom + station + line 세 종류 모두 노출.
	assert.NotNil(t, findGroup(groups, "custom:custom1"))
	assert.NotNil(t, findGroup(groups, "station:S"))
	assert.NotNil(t, findGroup(groups, "line:L"))

	// 결정적 순서(type→name→id): 두 번 호출해도 동일 순서.
	resp2, err := procJSON(t, ap, map[string]any{"command": "list_groups"})
	require.NoError(t, err)
	assert.Equal(t, string(resp), string(resp2))

	// station 그룹 필드 검증.
	st := findGroup(groups, "station:S")
	assert.Equal(t, "S역", st["name"])
	assert.Equal(t, groupTypeStation, st["type"])
	assert.EqualValues(t, 1, st["member_count"])
}

// Scenario 3.5: 기본 그룹 편집/삭제 거부 (ErrGroupNotCustom).
func TestGroup_BaseGroupEditRejected(t *testing.T) {
	ap := newGroupAgent(t, map[string]any{
		"station_registry": map[string]any{"S": map[string]any{"line": "L"}},
	}, map[string]any{"device_id": "d1", "station": "S"})

	_, err := procJSON(t, ap, map[string]any{"command": "set_group", "params": map[string]any{"group_id": "station:S", "name": "hack"}})
	assert.ErrorIs(t, err, ErrGroupNotCustom)

	_, err = procJSON(t, ap, map[string]any{"command": "remove_group", "params": map[string]any{"group_id": "line:L"}})
	assert.ErrorIs(t, err, ErrGroupNotCustom)
}

// Scenario 3.6: 미등록 그룹 거부 (ErrGroupNotFound, 부분 변경 없음).
func TestGroup_UnknownGroupRejected(t *testing.T) {
	ap := newGroupAgent(t, nil)

	_, err := procJSON(t, ap, map[string]any{"command": "set_group", "params": map[string]any{"group_id": "custom:nope", "name": "x"}})
	assert.ErrorIs(t, err, ErrGroupNotFound)

	_, err = procJSON(t, ap, map[string]any{"command": "remove_group", "params": map[string]any{"group_id": "custom:nope"}})
	assert.ErrorIs(t, err, ErrGroupNotFound)
}

// ---------------------------------------------------------------------------
// Module 4 — 기본 그룹 자동 동기화
// ---------------------------------------------------------------------------

// Scenario 4.1: 역사 추가 시 station 그룹 생성(자동 노출).
func TestGroup_AddStationCreatesStationGroup(t *testing.T) {
	ap := newGroupAgent(t, nil)
	_, err := procJSON(t, ap, map[string]any{"command": "add_station", "station": "S", "display_name": "S역"})
	require.NoError(t, err)

	resp, err := procJSON(t, ap, map[string]any{"command": "list_groups"})
	require.NoError(t, err)
	g := findGroup(listGroupsResult(t, resp), "station:S")
	require.NotNil(t, g)
	assert.Equal(t, "S역", g["name"])
	assert.Equal(t, groupTypeStation, g["type"])
}

// Scenario 4.2: 역사 삭제 시 station 그룹 제거.
func TestGroup_RemoveStationRemovesStationGroup(t *testing.T) {
	ap := newGroupAgent(t, nil)
	_, err := procJSON(t, ap, map[string]any{"command": "add_station", "station": "S", "display_name": "S역"})
	require.NoError(t, err)
	_, err = procJSON(t, ap, map[string]any{"command": "remove_station", "station": "S"})
	require.NoError(t, err)

	resp, err := procJSON(t, ap, map[string]any{"command": "list_groups"})
	require.NoError(t, err)
	assert.Nil(t, findGroup(listGroupsResult(t, resp), "station:S"))
}

// Scenario 4.3: 역사 표시명 변경 → 그룹 name 동기화.
func TestGroup_StationDisplayNameSync(t *testing.T) {
	ap := newGroupAgent(t, nil)
	_, err := procJSON(t, ap, map[string]any{"command": "add_station", "station": "S", "display_name": "옛이름"})
	require.NoError(t, err)
	_, err = procJSON(t, ap, map[string]any{"command": "add_station", "station": "S", "display_name": "새이름"})
	require.NoError(t, err)

	resp, err := procJSON(t, ap, map[string]any{"command": "list_groups"})
	require.NoError(t, err)
	g := findGroup(listGroupsResult(t, resp), "station:S")
	require.NotNil(t, g)
	assert.Equal(t, "새이름", g["name"])
}

// Scenario 4.4: line 그룹 파생 노출 + GroupMembers == DevicesByLine.
func TestGroup_LineGroupDerived(t *testing.T) {
	ap := newGroupAgent(t, map[string]any{
		"station_registry": map[string]any{
			"S1": map[string]any{"line": "L1"},
			"S2": map[string]any{"line": "L1"},
		},
	},
		map[string]any{"device_id": "d1", "station": "S1"},
		map[string]any{"device_id": "d2", "station": "S2"})

	resp, err := procJSON(t, ap, map[string]any{"command": "list_groups"})
	require.NoError(t, err)
	g := findGroup(listGroupsResult(t, resp), "line:L1")
	require.NotNil(t, g)
	assert.Equal(t, groupTypeLine, g["type"])
	assert.Equal(t, "L1", g["ref"])

	targets, _ := ap.DevicesByLine("L1")
	assert.Equal(t, targets, ap.GroupMembers("line:L1"))
	assert.Equal(t, []string{"d1", "d2"}, ap.GroupMembers("line:L1"))
}

// Scenario 4.5: 마지막 역사 제거 시 line 그룹 소멸.
func TestGroup_LastStationRemovedLineGroupGone(t *testing.T) {
	ap := newGroupAgent(t, map[string]any{
		"station_registry": map[string]any{"S1": map[string]any{"line": "L1"}},
	})
	// L1 노출 확인.
	resp, err := procJSON(t, ap, map[string]any{"command": "list_groups"})
	require.NoError(t, err)
	require.NotNil(t, findGroup(listGroupsResult(t, resp), "line:L1"))

	// 유일 역사 제거 → line 그룹 소멸.
	_, err = procJSON(t, ap, map[string]any{"command": "remove_station", "station": "S1"})
	require.NoError(t, err)
	resp, err = procJSON(t, ap, map[string]any{"command": "list_groups"})
	require.NoError(t, err)
	assert.Nil(t, findGroup(listGroupsResult(t, resp), "line:L1"))
}

// ---------------------------------------------------------------------------
// Module 5 — 그룹 셀렉터 일괄 제어
// ---------------------------------------------------------------------------

// Scenario 5.1: 커스텀 그룹 fan-out.
func TestGroup_CustomGroupFanOut(t *testing.T) {
	ap, mock := directAgentWithMock(t, func() map[string]any {
		o := directOpts()
		o["control_response_timeout"] = "0s"
		o["devices"] = groupDevices(
			map[string]any{"device_id": "d1"}, map[string]any{"device_id": "d2"})
		return o
	}())
	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "x", "members": []string{"d1", "d2"}}})
	require.NoError(t, err)

	resp, err := procJSON(t, ap, map[string]any{"command": "set_power", "group_id": "custom:x", "params": map[string]any{"power": true}})
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"xsfm/d1/cmd", "xsfm/d2/cmd"}, mock.publishedTopics())
	out := decodeFanOut(t, resp)
	assert.Equal(t, selectorRef{Type: "group_id", Value: "custom:x"}, out.Selector)
	assert.Equal(t, "ok", out.Status)
	require.Len(t, out.Results, 2)
}

// Scenario 5.2: 기본(station) 그룹 fan-out via group_id.
func TestGroup_StationGroupFanOut(t *testing.T) {
	ap, mock := directAgentWithMock(t, func() map[string]any {
		o := directOpts()
		o["control_response_timeout"] = "0s"
		o["station_registry"] = map[string]any{"S": map[string]any{"line": "L"}}
		o["devices"] = groupDevices(
			map[string]any{"device_id": "d1", "station": "S"},
			map[string]any{"device_id": "d2", "station": "S"})
		return o
	}())

	resp, err := procJSON(t, ap, map[string]any{"command": "set_power", "group_id": "station:S", "params": map[string]any{"power": true}})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"xsfm/d1/cmd", "xsfm/d2/cmd"}, mock.publishedTopics())
	out := decodeFanOut(t, resp)
	require.Len(t, out.Results, 2)
}

// Scenario 5.3: 빈 그룹 no-op (ErrEmptyGroup).
func TestGroup_EmptyCustomGroup(t *testing.T) {
	ap, mock := directAgentWithMock(t, func() map[string]any {
		o := directOpts()
		o["control_response_timeout"] = "0s"
		return o
	}())
	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "empty"}})
	require.NoError(t, err)

	_, err = procJSON(t, ap, map[string]any{"command": "set_power", "group_id": "custom:empty", "params": map[string]any{"power": true}})
	assert.ErrorIs(t, err, ErrEmptyGroup)
	assert.Equal(t, 0, mock.publishedLen())
}

// Scenario 5.4: 미등록 커스텀 그룹 거부 (ErrGroupNotFound).
func TestGroup_UnregisteredGroupFanOut(t *testing.T) {
	ap, mock := directAgentWithMock(t, func() map[string]any {
		o := directOpts()
		o["control_response_timeout"] = "0s"
		return o
	}())
	_, err := procJSON(t, ap, map[string]any{"command": "set_power", "group_id": "custom:nope", "params": map[string]any{"power": true}})
	assert.ErrorIs(t, err, ErrGroupNotFound)
	assert.Equal(t, 0, mock.publishedLen())
}

// Scenario 5.5: 셀렉터 우선순위 (device_id > group_id).
func TestGroup_SelectorPriorityDeviceOverGroup(t *testing.T) {
	ap, mock := directAgentWithMock(t, func() map[string]any {
		o := directOpts()
		o["control_response_timeout"] = "0s"
		o["devices"] = groupDevices(
			map[string]any{"device_id": "d1"}, map[string]any{"device_id": "d2"})
		return o
	}())
	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "x", "members": []string{"d1", "d2"}}})
	require.NoError(t, err)

	resp, err := procJSON(t, ap, map[string]any{"command": "set_power", "device_id": "d1", "group_id": "custom:x", "params": map[string]any{"power": true}})
	require.NoError(t, err)
	require.Equal(t, 1, mock.publishedLen())
	assert.Equal(t, "xsfm/d1/cmd", mock.published[0].topic)
	assert.NotContains(t, string(resp), `"selector"`)
}

// Scenario 5.7: 셀렉터 병존 동일 결과 (RD-4) — station 셀렉터 == group_id:station:.
func TestGroup_SelectorCoexistenceIdentical(t *testing.T) {
	mkOpts := func() map[string]any {
		o := directOpts()
		o["control_response_timeout"] = "0s"
		o["station_registry"] = map[string]any{
			"S":  map[string]any{"line": "L1"},
			"S2": map[string]any{"line": "L1"},
		}
		o["devices"] = groupDevices(
			map[string]any{"device_id": "d1", "station": "S"},
			map[string]any{"device_id": "d2", "station": "S"},
			map[string]any{"device_id": "d3", "station": "S2"})
		return o
	}

	// (a) station 셀렉터.
	apA, _ := directAgentWithMock(t, mkOpts())
	respA, err := procJSON(t, apA, map[string]any{"command": "set_power", "station": "S", "params": map[string]any{"power": true}})
	require.NoError(t, err)
	outA := decodeFanOut(t, respA)

	// (b) group_id:station:S 셀렉터.
	apB, _ := directAgentWithMock(t, mkOpts())
	respB, err := procJSON(t, apB, map[string]any{"command": "set_power", "group_id": "station:S", "params": map[string]any{"power": true}})
	require.NoError(t, err)
	outB := decodeFanOut(t, respB)

	// 두 경로 대상 집합 동일 (S 소속 d1,d2; d3 제외).
	assert.ElementsMatch(t, memberIDs(outA.Results), memberIDs(outB.Results))
	assert.ElementsMatch(t, []string{"d1", "d2"}, memberIDs(outB.Results))

	// line 셀렉터 vs group_id:line:L1 도 동일.
	apC, _ := directAgentWithMock(t, mkOpts())
	respC, err := procJSON(t, apC, map[string]any{"command": "set_power", "line": "L1", "params": map[string]any{"power": true}})
	require.NoError(t, err)
	apD, _ := directAgentWithMock(t, mkOpts())
	respD, err := procJSON(t, apD, map[string]any{"command": "set_power", "group_id": "line:L1", "params": map[string]any{"power": true}})
	require.NoError(t, err)
	assert.ElementsMatch(t, memberIDs(decodeFanOut(t, respC).Results), memberIDs(decodeFanOut(t, respD).Results))
	assert.ElementsMatch(t, []string{"d1", "d2", "d3"}, memberIDs(decodeFanOut(t, respD).Results))
}

// memberIDs 는 fan-out 결과에서 device_id 목록을 추출한다.
func memberIDs(results []groupResult) []string {
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, r.DeviceID)
	}
	return out
}

// ---------------------------------------------------------------------------
// Module 7 — 비기능 · 정합
// ---------------------------------------------------------------------------

// Scenario 7.2: 다대다 정합 — d1 이 station 그룹 + 커스텀 그룹 2개에 일관되게 나타남.
func TestGroup_MultiMembershipConsistency(t *testing.T) {
	ap := newGroupAgent(t, map[string]any{
		"station_registry": map[string]any{"S": map[string]any{"line": "L"}},
	}, map[string]any{"device_id": "d1", "station": "S"})
	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "g1", "members": []string{"d1"}}})
	require.NoError(t, err)
	_, err = procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "g2", "members": []string{"d1"}}})
	require.NoError(t, err)

	assert.Contains(t, ap.GroupMembers("custom:g1"), "d1")
	assert.Contains(t, ap.GroupMembers("custom:g2"), "d1")
	assert.Contains(t, ap.GroupMembers("station:S"), "d1")

	groups := ap.GroupsForDevice("d1")
	assert.Subset(t, groups, []string{"custom:g1", "custom:g2", "station:S", "line:L"})
}

// primary 그룹 삭제 시 리셋 (§4.5).
func TestGroup_PrimaryResetOnGroupDeletion(t *testing.T) {
	ap := newGroupAgent(t, nil, map[string]any{"device_id": "d1", "group_id": "legacy1"})
	// 마이그레이션으로 custom:legacy1 존재, d1.GroupID="legacy1".
	dev, _ := ap.GetDevice("d1")
	require.Equal(t, "legacy1", dev.GroupID)

	_, err := procJSON(t, ap, map[string]any{"command": "remove_group", "params": map[string]any{"group_id": "custom:legacy1"}})
	require.NoError(t, err)

	dev, _ = ap.GetDevice("d1")
	assert.Equal(t, "", dev.GroupID, "primary 그룹 삭제 시 primary 리셋")
}

// 커스텀 그룹 첫 편입 시 primary 설정 (§4.5).
func TestGroup_PrimarySetOnFirstEnroll(t *testing.T) {
	ap := newGroupAgent(t, nil, map[string]any{"device_id": "d1"}) // primary 없음.
	_, err := procJSON(t, ap, map[string]any{"command": "add_group", "params": map[string]any{"name": "floor2", "members": []string{"d1"}}})
	require.NoError(t, err)

	dev, _ := ap.GetDevice("d1")
	assert.Equal(t, "floor2", dev.GroupID, "primary 빈 디바이스는 첫 커스텀 편입 시 그 그룹을 primary 로 설정")
}
