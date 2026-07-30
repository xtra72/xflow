package xsfm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 역번호(station_number) — 역사 코드와 분리된 대외 표시 번호 (SPEC-XSFM-STATION-NUMBER)
//
// 역사(station)는 내부 코드(Station, 예: st01 — 주소지정/셀렉터/영속 키)와 선택적 역번호
// (StationNumber, 예: "239"/"K215"/"2-14" — 실제 역사 표시 참조)를 함께 가진다. 디바이스 자동
// 이름의 역사 세그먼트는 역번호가 있으면 역번호를, 없으면 역사 코드를 쓴다(코드 폴백). 내부
// 주소지정/셀렉터는 항상 코드 기준으로 불변이다. 역번호 중복은 비차단 경고이다.
// ---------------------------------------------------------------------------

// attrNamingAgent 는 attribute 토픽 템플릿(composite 주소 모델)로 device 자동 이름 합성 경로를
// 활성화한 direct 모드 에이전트를 만든다(TestAddDevice_FourSegmentName 미러).
func attrNamingAgent(t *testing.T) *XSFMAgent {
	t.Helper()
	return newGroupAgent(t, map[string]any{
		"state_topic_template":   "xsfm/{station_code}/{place_code}/{device_index}/{attribute}/state",
		"command_topic_template": "xsfm/{station_code}/{place_code}/{device_index}/{attribute}/cmd",
	})
}

// deviceNameByID 는 로스터에서 device_id 로 자동 합성된 Name 을 스냅샷한다.
func deviceNameByID(t *testing.T, ap *XSFMAgent, id string) string {
	t.Helper()
	ap.mu.RLock()
	defer ap.mu.RUnlock()
	dev, ok := ap.devices[id]
	require.True(t, ok, "device %q should exist", id)
	return dev.Name
}

// addDeviceComposite 는 attribute 모드에서 device_id 미지정 add_device 를 호출하고 생성된 UUID 를
// 반환한다(자동 이름 합성 경로 검증용).
func addDeviceComposite(t *testing.T, ap *XSFMAgent, station, place string, index int) string {
	t.Helper()
	resp, err := procJSON(t, ap, map[string]any{
		"command": "add_device",
		"params":  map[string]any{"station": station, "place": place, "index": index},
	})
	require.NoError(t, err)
	var r map[string]any
	require.NoError(t, json.Unmarshal(resp, &r))
	id, _ := r["device_id"].(string)
	require.NotEmpty(t, id)
	return id
}

// ResolveStationNumber: 등록 역사의 역번호를 반환하고, 미등록/역번호 없음은 "" 로 강등한다.
// (역번호 배선은 런타임 add_station 경로 — 설정 시드는 역번호를 나르지 않는 현재 범위이므로
// 런타임 등록으로 시드한다.)
func TestStationNumber_ResolveAccessor(t *testing.T) {
	a, err := NewXSFMAgent(baseAgentConfig(portOpts()))
	require.NoError(t, err)
	ap := asAP(t, a)

	_, err = procJSON(t, ap, map[string]any{"command": "add_station", "station": "st01", "line": "line-2", "station_number": "239", "order": 1})
	require.NoError(t, err)
	_, err = procJSON(t, ap, map[string]any{"command": "add_station", "station": "st02", "line": "line-2", "order": 2}) // 역번호 없음
	require.NoError(t, err)

	assert.Equal(t, "239", ap.stations.ResolveStationNumber("st01"), "역번호가 있으면 그 값")
	assert.Equal(t, "", ap.stations.ResolveStationNumber("st02"), "역번호 없음 → 빈 문자열")
	assert.Equal(t, "", ap.stations.ResolveStationNumber("st99"), "미등록 → 빈 문자열(에러 아님)")

	// 에이전트 헬퍼: 표시 값은 역번호 우선, 없으면 코드 폴백.
	assert.Equal(t, "239", ap.stationDisplayFor("st01"))
	assert.Equal(t, "st02", ap.stationDisplayFor("st02"), "역번호 없으면 코드 폴백")
	assert.Equal(t, "st99", ap.stationDisplayFor("st99"), "미등록이면 코드 폴백")
}

// with-line + 역번호 → 4-세그먼트 이름에 역번호가 역사 세그먼트로 들어간다.
func TestStationNumber_NameUsesNumberWithLine(t *testing.T) {
	ap := attrNamingAgent(t)
	_, _ = procJSON(t, ap, map[string]any{"command": "add_line", "params": map[string]any{"code": "line_2", "name": "2"}})
	_, _ = procJSON(t, ap, map[string]any{"command": "add_station", "params": map[string]any{"station": "st01", "line": "line_2", "station_number": "239"}})

	id := addDeviceComposite(t, ap, "st01", "pump", 3)
	assert.Equal(t, "line_2:239:pump:003", deviceNameByID(t, ap, id), "4-세그먼트: 역사 세그먼트에 역번호")
}

// with-line + 역번호 없음 → 역사 세그먼트는 코드로 폴백한다(무회귀).
func TestStationNumber_NameFallsBackToCodeWhenEmpty(t *testing.T) {
	ap := attrNamingAgent(t)
	_, _ = procJSON(t, ap, map[string]any{"command": "add_line", "params": map[string]any{"code": "line_2", "name": "2"}})
	_, _ = procJSON(t, ap, map[string]any{"command": "add_station", "params": map[string]any{"station": "st01", "line": "line_2"}})

	id := addDeviceComposite(t, ap, "st01", "pump", 3)
	assert.Equal(t, "line_2:st01:pump:003", deviceNameByID(t, ap, id), "역번호 없으면 코드 세그먼트")
}

// empty-line + 역번호 → 3-세그먼트({역번호}:{place}:{index}), 라인 세그먼트 생략(RD-4).
func TestStationNumber_EmptyLineWithNumberThreeSeg(t *testing.T) {
	ap := attrNamingAgent(t)
	// 라인 미지정 역사에 역번호만 부여.
	_, _ = procJSON(t, ap, map[string]any{"command": "add_station", "params": map[string]any{"station": "st99", "station_number": "K215"}})

	id := addDeviceComposite(t, ap, "st99", "pump", 3)
	assert.Equal(t, "K215:pump:003", deviceNameByID(t, ap, id), "라인 없음 + 역번호 → 3-세그먼트")
}

// 역번호 중복은 비차단 경고이다: 두 역사 모두 등록되고, 경고 로그가 방출된다.
func TestStationNumber_DuplicateWarningNonBlocking(t *testing.T) {
	logger, buf := captureLogger()
	a, err := NewXSFMAgent(agentConfigWithLogger(portOpts(), logger))
	require.NoError(t, err)
	ap := asAP(t, a)

	resp1, err := procJSON(t, ap, map[string]any{"command": "add_station", "station": "st01", "station_number": "239"})
	require.NoError(t, err)
	assertStatusOK(t, resp1)

	// 동일 역번호 "239" 를 다른 역사에 지정 → 경고만, 등록은 계속.
	resp2, err := procJSON(t, ap, map[string]any{"command": "add_station", "station": "st02", "station_number": "239"})
	require.NoError(t, err)
	assertStatusOK(t, resp2)

	// 양쪽 역사 모두 등록 유지(비차단).
	e1, err := ap.stations.GetStation("st01")
	require.NoError(t, err)
	assert.Equal(t, "239", e1.StationNumber)
	e2, err := ap.stations.GetStation("st02")
	require.NoError(t, err)
	assert.Equal(t, "239", e2.StationNumber)

	assert.Contains(t, buf.String(), "duplicate station_number", "중복 역번호 경고 로그가 방출되어야 한다")
}

// 서로 다른 역번호는 경고를 내지 않는다(중복 판정이 값 비교 기반).
func TestStationNumber_DistinctNumbersNoWarning(t *testing.T) {
	logger, buf := captureLogger()
	a, err := NewXSFMAgent(agentConfigWithLogger(portOpts(), logger))
	require.NoError(t, err)
	ap := asAP(t, a)

	_, _ = procJSON(t, ap, map[string]any{"command": "add_station", "station": "st01", "station_number": "239"})
	_, _ = procJSON(t, ap, map[string]any{"command": "add_station", "station": "st02", "station_number": "240"})
	assert.NotContains(t, buf.String(), "duplicate station_number")

	// 같은 역사를 동일 역번호로 재-upsert 하는 것은 중복이 아니다(자기 자신 제외).
	_, _ = procJSON(t, ap, map[string]any{"command": "add_station", "station": "st01", "station_number": "239"})
	assert.NotContains(t, buf.String(), "duplicate station_number", "자기 자신 재upsert 는 중복 아님")
}

// list_stations 는 station_number 를 포함한다.
func TestStationNumber_ListStationsIncludesNumber(t *testing.T) {
	a, err := NewXSFMAgent(baseAgentConfig(portOpts()))
	require.NoError(t, err)
	ap := asAP(t, a)

	_, err = procJSON(t, ap, map[string]any{"command": "add_station", "station": "st01", "line": "line-2", "station_number": "239", "order": 1})
	require.NoError(t, err)

	listResp, err := procJSON(t, ap, map[string]any{"command": "list_stations"})
	require.NoError(t, err)

	var out struct {
		Stations []struct {
			Station       string `json:"station"`
			StationNumber string `json:"station_number"`
		} `json:"stations"`
	}
	require.NoError(t, json.Unmarshal(listResp, &out))
	require.Len(t, out.Stations, 1)
	assert.Equal(t, "st01", out.Stations[0].Station)
	assert.Equal(t, "239", out.Stations[0].StationNumber)
}

// station_registered 이벤트는 station_number 를 실어 방출한다.
func TestStationNumber_RegisteredEventIncludesNumber(t *testing.T) {
	a, err := NewXSFMAgent(baseAgentConfig(portOpts()))
	require.NoError(t, err)
	ap := asAP(t, a)

	_, err = procJSON(t, ap, map[string]any{"command": "add_station", "station": "st01", "line": "line-2", "station_number": "239"})
	require.NoError(t, err)

	evt := readEvent(t, ap)
	assert.Equal(t, "station_registered", evt["type"])
	assert.Equal(t, "st01", evt["station"])
	assert.Equal(t, "239", evt["station_number"])
	assert.Equal(t, "line-2", evt["line"])
}

// 영속 라운드트립: 역번호가 재시작 후에도 복원된다.
func TestStationNumber_PersistRoundtrip(t *testing.T) {
	dir := t.TempDir()
	opts := portOpts()
	opts["station_registry_path"] = dir

	a1, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap1 := asAP(t, a1)

	_, err = procJSON(t, ap1, map[string]any{"command": "add_station", "station": "st01", "line": "line-2", "station_number": "239", "order": 1})
	require.NoError(t, err)

	// 재시작(동일 경로) → 역번호 복원.
	a2, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap2 := asAP(t, a2)

	restored, err := ap2.stations.GetStation("st01")
	require.NoError(t, err)
	assert.Equal(t, "239", restored.StationNumber, "역번호가 재시작 후에도 복원되어야 한다")
	assert.Equal(t, "line-2", restored.Line)
}

// sticky(nameOverridden) 커스텀 이름은 역번호 지정 후 주소 변경(재계산 트리거)에도 보존된다.
func TestStationNumber_StickyPreservedAfterNumberSet(t *testing.T) {
	ap := attrNamingAgent(t)
	// 사용자 지정 이름으로 sticky 고정.
	resp, err := procJSON(t, ap, map[string]any{
		"command": "add_device", "params": map[string]any{"station": "st99", "place": "pump", "index": 3, "name": "메인펌프"},
	})
	require.NoError(t, err)
	var r map[string]any
	require.NoError(t, json.Unmarshal(resp, &r))
	id := r["device_id"].(string)

	// 역사에 역번호 부여 후 set_device 로 재계산 트리거(station 재지정).
	_, _ = procJSON(t, ap, map[string]any{"command": "add_station", "params": map[string]any{"station": "st99", "station_number": "K215"}})
	_, err = procJSON(t, ap, map[string]any{"command": "set_device", "device_id": id, "station": "st99"})
	require.NoError(t, err)

	assert.Equal(t, "메인펌프", deviceNameByID(t, ap, id), "sticky 커스텀 이름은 역번호 지정에도 재계산되지 않는다")
}

// 역번호 후지정 → 자동 이름(non-sticky) 재계산이 역번호를 반영한다(코드→역번호 전이).
func TestStationNumber_RecomputeAfterNumberSet(t *testing.T) {
	ap := attrNamingAgent(t)
	// 역번호 없는 역사에서 자동 이름 생성(코드 세그먼트).
	id := addDeviceComposite(t, ap, "st99", "pump", 3)
	assert.Equal(t, "st99:pump:003", deviceNameByID(t, ap, id))

	// 역사에 역번호 부여 후 재계산 트리거 → 이름의 역사 세그먼트가 역번호로 전이.
	_, _ = procJSON(t, ap, map[string]any{"command": "add_station", "params": map[string]any{"station": "st99", "station_number": "K215"}})
	_, err := procJSON(t, ap, map[string]any{"command": "set_device", "device_id": id, "station": "st99"})
	require.NoError(t, err)
	assert.Equal(t, "K215:pump:003", deviceNameByID(t, ap, id))
}

// 내부 주소지정/셀렉터는 역번호와 무관하게 코드 기준으로 불변이다(보조 인덱스·DevicesByStation).
func TestStationNumber_AddressingUnchangedByNumber(t *testing.T) {
	ap := attrNamingAgent(t)
	_, _ = procJSON(t, ap, map[string]any{"command": "add_station", "params": map[string]any{"station": "st01", "station_number": "239"}})
	id := addDeviceComposite(t, ap, "st01", "pump", 3)

	// 보조 인덱스는 역사 코드(st01) 기준으로 조회된다(역번호 아님).
	idFromIdx, ok := secondaryLookup(ap, "st01", "pump", 3)
	require.True(t, ok)
	assert.Equal(t, id, idFromIdx)

	// DevicesByStation 도 코드 기준.
	assert.Equal(t, []string{id}, ap.DevicesByStation("st01"))
	assert.Empty(t, ap.DevicesByStation("239"), "역번호로는 주소 조회되지 않는다(코드 전용)")
}

// HTTP exec 경로(params backfill): station_number 가 params 에서 구조체 필드로 승격된다.
func TestStationNumber_ParamsBackfill(t *testing.T) {
	var req processRequest
	raw := []byte(`{"command":"add_station","params":{"station":"st01","station_number":"239"}}`)
	require.NoError(t, json.Unmarshal(raw, &req))
	req.fillFromParams()
	assert.Equal(t, "239", req.StationNumber, "params.station_number 가 top-level 로 backfill 되어야 한다")

	// top-level 우선: top-level 이 지정되면 params 값으로 덮어쓰지 않는다.
	var req2 processRequest
	raw2 := []byte(`{"command":"add_station","station_number":"TOP","params":{"station_number":"PARAM"}}`)
	require.NoError(t, json.Unmarshal(raw2, &req2))
	req2.fillFromParams()
	assert.Equal(t, "TOP", req2.StationNumber, "top-level 이 우선")
}

// 저장소 라운드트립(storage 레이어 단위): StationNumber 가 파일에 영속되고 재로드된다.
func TestStationNumber_StorageRoundtrip(t *testing.T) {
	// station_registry 저장소 경로 라운드트립을 에이전트 재구성으로 검증(재시작 시뮬레이션).
	dir := t.TempDir()
	opts := portOpts()
	opts["station_registry_path"] = dir

	a1, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap1 := asAP(t, a1)
	_, err = procJSON(t, ap1, map[string]any{"command": "add_station", "station": "st01", "station_number": "2-14"})
	require.NoError(t, err)
	require.NoError(t, ap1.Stop(context.Background()))

	a2, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap2 := asAP(t, a2)
	got, err := ap2.stations.GetStation("st01")
	require.NoError(t, err)
	assert.Equal(t, "2-14", got.StationNumber, "자유 형식 역번호(2-14)가 파일 라운드트립을 통과해야 한다")
}
