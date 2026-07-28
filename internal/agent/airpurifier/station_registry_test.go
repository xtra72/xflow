package airpurifier

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// portOpts 는 MQTT 브로커 없이 초기화 가능한 port 모드 옵션을 반환한다(역사 레지스트리
// 테스트는 트랜스포트와 무관하므로 브로커 배선을 피한다).
func portOpts() map[string]any {
	return map[string]any{
		"transport_mode":  "port",
		"payload_mapping": validPayloadMapping(),
	}
}

// seedST101 는 {"ST-101": {line-2, 강남, order 5}} 시드 맵을 반환한다.
func seedST101() map[string]any {
	return map[string]any{
		"ST-101": map[string]any{"line": "line-2", "display_name": "강남", "order": 5},
	}
}

// Scenario 2B.1: 설정 시드 + station→line 조회.
func TestStationRegistry_SeedLoadAndResolveLine(t *testing.T) {
	opts := portOpts()
	opts["station_registry"] = seedST101()

	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	line, err := ap.stations.ResolveLine("ST-101")
	require.NoError(t, err)
	assert.Equal(t, "line-2", line)

	entry, err := ap.stations.GetStation("ST-101")
	require.NoError(t, err)
	assert.Equal(t, "강남", entry.DisplayName)
	assert.Equal(t, 5, entry.Order)
	assert.Equal(t, "line-2", entry.Line)
}

// Scenario 2B.2: 미등록 station 조회 → ErrStationNotFound.
func TestStationRegistry_ResolveLineUnregistered(t *testing.T) {
	a, err := NewAirPurifierAgent(baseAgentConfig(portOpts()))
	require.NoError(t, err)
	ap := asAP(t, a)

	_, err = ap.stations.ResolveLine("ST-999")
	assert.ErrorIs(t, err, ErrStationNotFound)

	_, err = ap.stations.GetStation("ST-999")
	assert.ErrorIs(t, err, ErrStationNotFound)
}

// Scenario 2B.3: CRUD 라운드트립 — add_station 후 동일 경로에서 레지스트리 재구성(재시작
// 시뮬레이션) → station 복원. list_stations 는 order 순 반환.
func TestStationRegistry_CRUDRoundtripRestart(t *testing.T) {
	dir := t.TempDir()

	opts := portOpts()
	opts["station_registry"] = seedST101()
	opts["station_registry_path"] = dir

	a1, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap1 := asAP(t, a1)

	// add_station ST-102 (line-2, order 6) → 영속화.
	addReq := map[string]any{
		"command": "add_station", "station": "ST-102",
		"line": "line-2", "display_name": "역삼", "order": 6,
	}
	addRaw, _ := json.Marshal(addReq)
	resp, err := ap1.Process(addRaw)
	require.NoError(t, err)
	assertStatusOK(t, resp)

	// 동일 경로에서 새 에이전트 구성 (재시작 시뮬레이션). 시드 ST-101 + 영속 ST-102.
	a2, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap2 := asAP(t, a2)

	restored, err := ap2.stations.GetStation("ST-102")
	require.NoError(t, err)
	assert.Equal(t, "line-2", restored.Line)
	assert.Equal(t, "역삼", restored.DisplayName)
	assert.Equal(t, 6, restored.Order)

	// list_stations → order 오름차순 [ST-101(5), ST-102(6)].
	listRaw, _ := json.Marshal(map[string]any{"command": "list_stations"})
	listResp, err := ap2.Process(listRaw)
	require.NoError(t, err)

	var out struct {
		Status   string `json:"status"`
		Stations []struct {
			Station string `json:"station"`
			Line    string `json:"line"`
			Order   int    `json:"order"`
		} `json:"stations"`
	}
	require.NoError(t, json.Unmarshal(listResp, &out))
	assert.Equal(t, "ok", out.Status)
	require.Len(t, out.Stations, 2)
	assert.Equal(t, "ST-101", out.Stations[0].Station)
	assert.Equal(t, "ST-102", out.Stations[1].Station)
}

// remove_station 은 등록 station 을 제거하고, 미등록 station 은 ErrStationNotFound.
func TestStationRegistry_RemoveStation(t *testing.T) {
	opts := portOpts()
	opts["station_registry"] = seedST101()
	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	rmRaw, _ := json.Marshal(map[string]any{"command": "remove_station", "station": "ST-101"})
	resp, err := ap.Process(rmRaw)
	require.NoError(t, err)
	assertStatusOK(t, resp)

	_, err = ap.stations.GetStation("ST-101")
	assert.ErrorIs(t, err, ErrStationNotFound)

	// 미등록 station 제거 시도 → ErrStationNotFound.
	rmRaw2, _ := json.Marshal(map[string]any{"command": "remove_station", "station": "ST-999"})
	_, err = ap.Process(rmRaw2)
	assert.ErrorIs(t, err, ErrStationNotFound)
}

// Scenario 2B.4: StationsByLine 은 호선 소속 역사를 order 오름차순으로 반환.
func TestStationRegistry_StationsByLineOrdered(t *testing.T) {
	opts := portOpts()
	// 의도적으로 역순 order 로 시드해 정렬을 검증한다.
	opts["station_registry"] = map[string]any{
		"ST-102": map[string]any{"line": "line-2", "display_name": "역삼", "order": 6},
		"ST-101": map[string]any{"line": "line-2", "display_name": "강남", "order": 5},
		"ST-301": map[string]any{"line": "line-3", "display_name": "삼각지", "order": 1},
	}
	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	line2 := ap.stations.StationsByLine("line-2")
	require.Len(t, line2, 2)
	assert.Equal(t, "ST-101", line2[0].Station) // order 5
	assert.Equal(t, "ST-102", line2[1].Station) // order 6

	line3 := ap.stations.StationsByLine("line-3")
	require.Len(t, line3, 1)
	assert.Equal(t, "ST-301", line3[0].Station)
}

// Scenario 2B.5: line 은 디바이스 속성이 아니다 (SSOT 는 레지스트리). 디바이스는 Station 만
// 보유하고 line 은 ResolveLine(device.Station) 으로 도출된다.
func TestStationRegistry_LineDerivedNotStored(t *testing.T) {
	opts := portOpts()
	opts["station_registry"] = seedST101()
	opts["devices"] = []any{
		map[string]any{"device_id": "ap-101", "station": "ST-101"},
	}
	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.Equal(t, "ST-101", dev.Station)

	// line 은 디바이스에 저장되지 않고 레지스트리에서 도출된다.
	line, err := ap.stations.ResolveLine(dev.Station)
	require.NoError(t, err)
	assert.Equal(t, "line-2", line)
}

// DevicesByStation 은 로스터 속성(Device.Station)에서 device_id 를 도출한다 (정렬).
func TestDevicesByStation(t *testing.T) {
	opts := portOpts()
	opts["station_registry"] = seedST101()
	opts["devices"] = []any{
		map[string]any{"device_id": "ap-102", "station": "ST-101"},
		map[string]any{"device_id": "ap-101", "station": "ST-101"},
		map[string]any{"device_id": "ap-201", "station": "ST-201"},
	}
	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	got := ap.DevicesByStation("ST-101")
	assert.Equal(t, []string{"ap-101", "ap-102"}, got) // 정렬됨, ap-201 제외

	assert.Empty(t, ap.DevicesByStation("ST-UNKNOWN"))
}

// DevicesByLine 은 역사 레지스트리를 SSOT 로 line→stations→devices 를 도출하고, 미등록
// station 참조 디바이스를 제외하며 그 사실을 함께 보고한다 (REQ-AIRPUR-001-02-13).
func TestDevicesByLine_DerivationAndExclusion(t *testing.T) {
	opts := portOpts()
	opts["station_registry"] = map[string]any{
		"ST-101": map[string]any{"line": "line-2", "order": 5},
		"ST-102": map[string]any{"line": "line-2", "order": 6},
		"ST-301": map[string]any{"line": "line-3", "order": 1},
	}
	opts["devices"] = []any{
		map[string]any{"device_id": "ap-101", "station": "ST-101"}, // line-2 대상
		map[string]any{"device_id": "ap-102", "station": "ST-102"}, // line-2 대상
		map[string]any{"device_id": "ap-301", "station": "ST-301"}, // line-3 (비대상)
		map[string]any{"device_id": "ap-999", "station": "ST-999"}, // 미등록 station → 제외 표기
		map[string]any{"device_id": "ap-none"},                     // Station 없음 → 제외 목록 아님
	}
	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	targets, excluded := ap.DevicesByLine("line-2")
	assert.Equal(t, []string{"ap-101", "ap-102"}, targets)
	assert.Equal(t, []string{"ap-999"}, excluded, "미등록 station 참조 디바이스가 제외 목록에 표기되어야 한다")
}

// 저장소 경로 미설정 시 인메모리/시드 전용으로 동작하고 파일을 생성하지 않는다.
func TestStationRegistry_InMemoryOnlyWhenNoPath(t *testing.T) {
	opts := portOpts()
	opts["station_registry"] = seedST101()
	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	require.Nil(t, ap.stations.repo, "경로 미설정 시 저장소는 nil 이어야 한다")

	// 런타임 add_station 도 (영속 없이) 정상 동작.
	addRaw, _ := json.Marshal(map[string]any{"command": "add_station", "station": "ST-102", "line": "line-2", "order": 6})
	resp, err := ap.Process(addRaw)
	require.NoError(t, err)
	assertStatusOK(t, resp)

	entry, err := ap.stations.GetStation("ST-102")
	require.NoError(t, err)
	assert.Equal(t, "line-2", entry.Line)
}

// assertStatusOK 는 응답 JSON 의 status 필드가 "ok" 인지 확인한다.
func assertStatusOK(t *testing.T, resp []byte) {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(resp, &m))
	assert.Equal(t, "ok", m["status"])
}
