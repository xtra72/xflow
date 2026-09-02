package xsfm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// SPEC-XSFM-NAMESEL-001 — 이름 기반 제어 셀렉터 (device_name / group_name)
//
// 리졸버(DeviceByName/GroupByName) 단위 검증 + 디스패치(개별/그룹) + 우선순위 체인
// (device_id > device_name > station > line > group_id > group_name) + 무회귀.
// 순수 방출 검증은 control_response_timeout=0s(fire-and-forget)를 사용한다.
// ---------------------------------------------------------------------------

// nameDeviceOpts 는 이름 셀렉터 테스트용 device 시드를 갖는 direct 옵션을 만든다(fire-and-forget).
func nameDeviceOpts(devices ...map[string]any) map[string]any {
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	seeds := make([]any, 0, len(devices))
	for _, d := range devices {
		seeds = append(seeds, d)
	}
	opts["devices"] = seeds
	return opts
}

// addCustomGroup 은 런타임 add_group 으로 커스텀 그룹(name/code/members)을 생성한다.
func addCustomGroup(t *testing.T, ap *XSFMAgent, name, code string, members ...string) {
	t.Helper()
	raw := `{"command":"add_group","name":"` + name + `","code":"` + code + `"`
	if len(members) > 0 {
		raw += `,"members":[`
		for i, m := range members {
			if i > 0 {
				raw += ","
			}
			raw += `"` + m + `"`
		}
		raw += `]`
	}
	raw += `}`
	_, err := ap.Process([]byte(raw))
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// AC 1.x — 이름 리졸버 + 모호성/부재 (Module 1, Module 4)
// ---------------------------------------------------------------------------

// AC 1.1: device_name 유일 매치 → device_id 해소.
func TestDeviceByName_UniqueMatch(t *testing.T) {
	ap, _ := directAgentWithMock(t, nameDeviceOpts(
		map[string]any{"device_id": "01", "name": "환기팬-01"},
		map[string]any{"device_id": "02", "name": "환기팬-02"},
	))

	id, err := ap.DeviceByName("환기팬-01")
	require.NoError(t, err)
	assert.Equal(t, "01", id)
}

// AC 1.2: device_name 다중 매치 → ErrAmbiguousName (무방출).
func TestDeviceByName_AmbiguousNoEmit(t *testing.T) {
	ap, mock := directAgentWithMock(t, nameDeviceOpts(
		map[string]any{"device_id": "01", "name": "환기팬"},
		map[string]any{"device_id": "02", "name": "환기팬"},
	))

	// 리졸버 직접 호출: 다중 매치 → ErrAmbiguousName.
	_, err := ap.DeviceByName("환기팬")
	assert.ErrorIs(t, err, ErrAmbiguousName)

	// 제어 명령 경로: 무방출(cmdSink 0회) fail-closed.
	_, perr := ap.Process([]byte(`{"command":"set_power","device_name":"환기팬","params":{"power":true}}`))
	assert.ErrorIs(t, perr, ErrAmbiguousName)
	assert.Equal(t, 0, mock.publishedLen(), "모호한 이름은 어떠한 발행도 하지 않아야 한다")
}

// AC 1.3: device_name 0 매치 → ErrDeviceNotFound.
func TestDeviceByName_NotFound(t *testing.T) {
	ap, _ := directAgentWithMock(t, nameDeviceOpts(
		map[string]any{"device_id": "01", "name": "환기팬-01"},
	))

	_, err := ap.DeviceByName("없는이름")
	assert.ErrorIs(t, err, ErrDeviceNotFound)
}

// AC 1.7: 공백 trim 후 정확 일치.
func TestDeviceByName_TrimExactMatch(t *testing.T) {
	ap, _ := directAgentWithMock(t, nameDeviceOpts(
		map[string]any{"device_id": "01", "name": "환기팬-01"},
	))

	id, err := ap.DeviceByName("  환기팬-01  ")
	require.NoError(t, err)
	assert.Equal(t, "01", id)
}

// AC 1.8: 대소문자 구분(RD-6) — 대소문자만 다른 이름은 매치되지 않는다.
func TestDeviceByName_CaseSensitive(t *testing.T) {
	ap, _ := directAgentWithMock(t, nameDeviceOpts(
		map[string]any{"device_id": "01", "name": "Fan-A"},
	))

	// 소문자는 case folding 을 하지 않으므로 매치 실패.
	_, err := ap.DeviceByName("fan-a")
	assert.ErrorIs(t, err, ErrDeviceNotFound)

	// 정확 케이스만 매치.
	id, err := ap.DeviceByName("Fan-A")
	require.NoError(t, err)
	assert.Equal(t, "01", id)
}

// AC 1.4: group_name 유일 매치(커스텀 그룹) → 그룹 해소.
func TestGroupByName_CustomUniqueMatch(t *testing.T) {
	ap, _ := directAgentWithMock(t, nameDeviceOpts(
		map[string]any{"device_id": "01"},
		map[string]any{"device_id": "02"},
	))
	addCustomGroup(t, ap, "2층 환기", "floor2", "01", "02")

	id, err := ap.GroupByName("2층 환기")
	require.NoError(t, err)
	assert.Equal(t, "custom:floor2", id)
}

// AC 1.4b: group_name 유일 매치(파생 station 그룹, RD-5 전 타입) → 그룹 해소.
func TestGroupByName_DerivedStationMatch(t *testing.T) {
	opts := nameDeviceOpts(map[string]any{"device_id": "01", "station": "ST-GN"})
	opts["station_registry"] = map[string]any{
		"ST-GN": map[string]any{"display_name": "강남역", "line": "2호선"},
	}
	ap, _ := directAgentWithMock(t, opts)

	// 파생 station 그룹의 표시명(display_name)으로 해소 — 커스텀뿐 아니라 파생도 매칭(RD-5).
	id, err := ap.GroupByName("강남역")
	require.NoError(t, err)
	assert.Equal(t, "station:ST-GN", id)
}

// AC 1.5: group_name 다중 매치(커스텀 + 파생 충돌, RD-5+RD-2) → ErrAmbiguousName (무방출).
func TestGroupByName_AmbiguousCustomAndDerived(t *testing.T) {
	opts := nameDeviceOpts(map[string]any{"device_id": "01", "station": "ST-GN"})
	opts["station_registry"] = map[string]any{
		"ST-GN": map[string]any{"display_name": "강남역", "line": "2호선"},
	}
	ap, mock := directAgentWithMock(t, opts)
	// 커스텀 그룹 표시명도 "강남역" → 파생 station 그룹과 타입 간 충돌(전 타입 매칭).
	addCustomGroup(t, ap, "강남역", "gangnam-custom", "01")

	_, err := ap.GroupByName("강남역")
	assert.ErrorIs(t, err, ErrAmbiguousName)

	// 제어 경로: 타입 간 충돌도 안전 거부, 어떤 멤버로도 방출 없음.
	_, perr := ap.Process([]byte(`{"command":"set_power","group_name":"강남역","params":{"power":true}}`))
	assert.ErrorIs(t, perr, ErrAmbiguousName)
	assert.Equal(t, 0, mock.publishedLen())
}

// AC 1.6: group_name 0 매치 → ErrGroupNotFound.
func TestGroupByName_NotFound(t *testing.T) {
	ap, _ := directAgentWithMock(t, nameDeviceOpts(
		map[string]any{"device_id": "01"},
	))

	_, err := ap.GroupByName("없는그룹")
	assert.ErrorIs(t, err, ErrGroupNotFound)
}

// ---------------------------------------------------------------------------
// AC 2.x — 이름 셀렉터 제어 (Module 2)
// ---------------------------------------------------------------------------

// AC 2.1: device_name → 개별 제어(controlDevice 1회, 개별 응답).
func TestControl_DeviceNameIndividual(t *testing.T) {
	ap, mock := directAgentWithMock(t, nameDeviceOpts(
		map[string]any{"device_id": "01", "name": "환기팬-01"},
		map[string]any{"device_id": "02", "name": "환기팬-02"},
	))

	resp, err := ap.Process([]byte(`{"command":"set_power","device_name":"환기팬-01","params":{"power":true}}`))
	require.NoError(t, err)

	// device_id "01" 개별 제어만 1회 발행(집계 응답 아님).
	require.Equal(t, 1, mock.publishedLen())
	assert.Equal(t, "xsfm/01/cmd", mock.published[0].topic)
	assert.Contains(t, string(resp), `"status":"ok"`)
	assert.NotContains(t, string(resp), `"selector"`)
}

// AC 2.2: group_name → 그룹 fan-out(커스텀 그룹, 집계 응답).
func TestControl_GroupNameFanOut(t *testing.T) {
	ap, mock := directAgentWithMock(t, nameDeviceOpts(
		map[string]any{"device_id": "01"},
		map[string]any{"device_id": "02"},
	))
	addCustomGroup(t, ap, "2층 환기", "floor2", "01", "02")

	resp, err := ap.Process([]byte(`{"command":"set_fan_speed","group_name":"2층 환기","params":{"fan_speed":2}}`))
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"xsfm/01/cmd", "xsfm/02/cmd"}, mock.publishedTopics())
	out := decodeFanOut(t, resp)
	assert.Equal(t, selectorRef{Type: "group_name", Value: "2층 환기"}, out.Selector)
	assert.Equal(t, "ok", out.Status)
	require.Len(t, out.Results, 2)
}

// AC 2.2b: group_name → 파생 station 그룹 fan-out(표시명 해소, RD-5).
func TestControl_GroupNameDerivedStationFanOut(t *testing.T) {
	opts := nameDeviceOpts(
		map[string]any{"device_id": "01", "station": "ST-GN"},
		map[string]any{"device_id": "02", "station": "ST-GN"},
		map[string]any{"device_id": "03", "station": "ST-OTHER"},
	)
	opts["station_registry"] = map[string]any{
		"ST-GN":    map[string]any{"display_name": "강남역", "line": "2호선"},
		"ST-OTHER": map[string]any{"display_name": "역삼역", "line": "2호선"},
	}
	ap, mock := directAgentWithMock(t, opts)

	resp, err := ap.Process([]byte(`{"command":"set_power","group_name":"강남역","params":{"power":true}}`))
	require.NoError(t, err)

	// ST-GN 소속 01,02 만 fan-out (ST-OTHER 03 제외).
	assert.ElementsMatch(t, []string{"xsfm/01/cmd", "xsfm/02/cmd"}, mock.publishedTopics())
	out := decodeFanOut(t, resp)
	assert.Equal(t, selectorRef{Type: "group_name", Value: "강남역"}, out.Selector)
	require.Len(t, out.Results, 2)
}

// AC 2.3: group_name 이 빈 그룹 해소 → ErrEmptyGroup (무방출).
func TestControl_GroupNameEmptyGroup(t *testing.T) {
	ap, mock := directAgentWithMock(t, nameDeviceOpts(
		map[string]any{"device_id": "01"},
	))
	// 멤버 없는 커스텀 그룹.
	addCustomGroup(t, ap, "빈그룹", "empty")

	_, err := ap.Process([]byte(`{"command":"set_power","group_name":"빈그룹","params":{"power":true}}`))
	assert.ErrorIs(t, err, ErrEmptyGroup)
	assert.Equal(t, 0, mock.publishedLen())
}

// AC 3.4: fillFromParams 이름 승격 — params 에 담긴 device_name 이 top-level 로 승격되어 해소.
func TestControl_DeviceNameFromParams(t *testing.T) {
	ap, mock := directAgentWithMock(t, nameDeviceOpts(
		map[string]any{"device_id": "01", "name": "환기팬-01"},
	))

	// HTTP exec 계약: device_name 이 params 에만 실림(top-level 없음) → fillFromParams 승격 후 해소.
	resp, err := ap.Process([]byte(`{"command":"set_power","params":{"device_name":"환기팬-01","power":true}}`))
	require.NoError(t, err)

	require.Equal(t, 1, mock.publishedLen())
	assert.Equal(t, "xsfm/01/cmd", mock.published[0].topic)
	assert.Contains(t, string(resp), `"status":"ok"`)
}

// ---------------------------------------------------------------------------
// AC 4.x — 우선순위 (RD-4 체인 device_id > device_name > station > line > group_id > group_name)
// ---------------------------------------------------------------------------

// AC 4.1: device_id 가 device_name 을 이긴다.
func TestPriority_DeviceIDBeatsDeviceName(t *testing.T) {
	ap, mock := directAgentWithMock(t, nameDeviceOpts(
		map[string]any{"device_id": "01", "name": "환기팬-01"},
		map[string]any{"device_id": "02", "name": "환기팬-02"},
	))

	resp, err := ap.Process([]byte(`{"command":"set_power","device_id":"01","device_name":"환기팬-02","params":{"power":true}}`))
	require.NoError(t, err)

	// device_id "01" 만 제어(device_name "환기팬-02"→02 무시).
	require.Equal(t, 1, mock.publishedLen())
	assert.Equal(t, "xsfm/01/cmd", mock.published[0].topic)
	assert.NotContains(t, string(resp), `"selector"`)
}

// AC 4.2: 명시적 group_id 가 group_name 을 이긴다.
func TestPriority_GroupIDBeatsGroupName(t *testing.T) {
	ap, mock := directAgentWithMock(t, nameDeviceOpts(
		map[string]any{"device_id": "01"},
		map[string]any{"device_id": "02"},
		map[string]any{"device_id": "03"},
	))
	addCustomGroup(t, ap, "floor2 그룹", "floor2", "01")   // group_id 대상: 멤버 01
	addCustomGroup(t, ap, "3층 환기", "floor3", "02", "03") // group_name 대상: 멤버 02,03

	resp, err := ap.Process([]byte(`{"command":"set_power","group_id":"custom:floor2","group_name":"3층 환기","params":{"power":true}}`))
	require.NoError(t, err)

	// group_id(custom:floor2) fan-out 만 실행 → 멤버 01. group_name 무시.
	assert.Equal(t, []string{"xsfm/01/cmd"}, mock.publishedTopics())
	out := decodeFanOut(t, resp)
	assert.Equal(t, "group_id", out.Selector.Type)
}

// AC 4.3: device_name 이 station/line/group_id(집계 셀렉터)보다 우선(개별 먼저).
func TestPriority_DeviceNameBeatsAggregateSelectors(t *testing.T) {
	opts := nameDeviceOpts(
		map[string]any{"device_id": "01", "name": "환기팬-01", "station": "ST-101", "group_id": "grp"},
		map[string]any{"device_id": "02", "station": "ST-101", "group_id": "grp"},
	)
	opts["station_registry"] = map[string]any{"ST-101": map[string]any{"line": "2호선"}}
	ap, mock := directAgentWithMock(t, opts)

	resp, err := ap.Process([]byte(`{"command":"set_power","device_name":"환기팬-01","station":"ST-101","line":"2호선","group_id":"grp","params":{"power":true}}`))
	require.NoError(t, err)

	// device_name → 개별 제어(device_id "01")만, 집계 셀렉터 전부 무시.
	require.Equal(t, 1, mock.publishedLen())
	assert.Equal(t, "xsfm/01/cmd", mock.published[0].topic)
	assert.NotContains(t, string(resp), `"selector"`)
}

// AC 4.4: device_name 이 station 을 이긴다(체인 인접 확인).
func TestPriority_DeviceNameBeatsStation(t *testing.T) {
	opts := nameDeviceOpts(
		map[string]any{"device_id": "01", "name": "환기팬-01", "station": "ST-101"},
		map[string]any{"device_id": "02", "station": "ST-101"},
	)
	ap, mock := directAgentWithMock(t, opts)

	_, err := ap.Process([]byte(`{"command":"set_power","device_name":"환기팬-01","station":"ST-101","params":{"power":true}}`))
	require.NoError(t, err)

	// device_name → 01 개별만, station fan-out 미수행.
	require.Equal(t, 1, mock.publishedLen())
	assert.Equal(t, "xsfm/01/cmd", mock.published[0].topic)
}

// AC 4.5: group_name 이 최하위 우선순위(station 이 group_name 을 이긴다).
func TestPriority_StationBeatsGroupName(t *testing.T) {
	opts := nameDeviceOpts(
		map[string]any{"device_id": "01", "station": "ST-101"},
		map[string]any{"device_id": "02", "station": "ST-999"},
	)
	ap, mock := directAgentWithMock(t, opts)
	addCustomGroup(t, ap, "2층 환기", "floor2", "02") // group_name 대상: 멤버 02

	resp, err := ap.Process([]byte(`{"command":"set_power","station":"ST-101","group_name":"2층 환기","params":{"power":true}}`))
	require.NoError(t, err)

	// station fan-out 만(멤버 01). group_name 무시(체인 최하위).
	assert.Equal(t, []string{"xsfm/01/cmd"}, mock.publishedTopics())
	out := decodeFanOut(t, resp)
	assert.Equal(t, "station", out.Selector.Type)
}

// ---------------------------------------------------------------------------
// AC 5.x — 무회귀 (NFR)
// ---------------------------------------------------------------------------

// AC 5.1: 기존 셀렉터(device_id/station) 무회귀 — 이름 필드 미사용 시 종전과 동일 동작.
func TestNoRegression_ExistingSelectorsUnchanged(t *testing.T) {
	opts := nameDeviceOpts(
		map[string]any{"device_id": "01", "station": "ST-101"},
		map[string]any{"device_id": "02", "station": "ST-101"},
	)
	ap, mock := directAgentWithMock(t, opts)

	// device_id 개별 경로.
	resp, err := ap.Process([]byte(`{"command":"set_power","device_id":"01","params":{"power":true}}`))
	require.NoError(t, err)
	require.Equal(t, 1, mock.publishedLen())
	assert.Equal(t, "xsfm/01/cmd", mock.published[0].topic)
	assert.NotContains(t, string(resp), `"selector"`)

	// station fan-out 경로(무회귀 selector type=station).
	resp2, err := ap.Process([]byte(`{"command":"set_power","station":"ST-101","params":{"power":true}}`))
	require.NoError(t, err)
	out := decodeFanOut(t, resp2)
	assert.Equal(t, "station", out.Selector.Type)
	require.Len(t, out.Results, 2)
}
