package xsfm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addPlaceReq 는 add_place 명령 JSON 을 만든다.
func addPlaceReq(station, place, displayName string, order int) []byte {
	b, _ := json.Marshal(map[string]any{
		"command": "add_place", "station": station, "place": place,
		"display_name": displayName, "order": order,
	})
	return b
}

// Scenario Place.1: 역사 내 위치 등록 → list_places 가 Order 순으로 반환.
func TestPlace_AddAndListOrdered(t *testing.T) {
	opts := portOpts()
	opts["station_registry"] = seedST101()
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	// 의도적으로 역순 order 로 추가해 정렬을 검증한다.
	resp, err := ap.Process(addPlaceReq("ST-101", "PL-B", "승강장 B", 2))
	require.NoError(t, err)
	assertStatusOK(t, resp)
	resp, err = ap.Process(addPlaceReq("ST-101", "PL-A", "승강장 A", 1))
	require.NoError(t, err)
	assertStatusOK(t, resp)

	listRaw, _ := json.Marshal(map[string]any{"command": "list_places", "station": "ST-101"})
	listResp, err := ap.Process(listRaw)
	require.NoError(t, err)

	var out struct {
		Status string `json:"status"`
		Places []struct {
			Place       string `json:"place"`
			DisplayName string `json:"display_name"`
			Order       int    `json:"order"`
		} `json:"places"`
	}
	require.NoError(t, json.Unmarshal(listResp, &out))
	assert.Equal(t, "ok", out.Status)
	require.Len(t, out.Places, 2)
	assert.Equal(t, "PL-A", out.Places[0].Place) // order 1
	assert.Equal(t, "PL-B", out.Places[1].Place) // order 2
	assert.Equal(t, "승강장 A", out.Places[0].DisplayName)
}

// Scenario Place.2: 미등록 역사에 위치 추가 → ErrStationNotFound.
func TestPlace_AddToMissingStation(t *testing.T) {
	a, err := NewXSFMAgent(baseAgentConfig(portOpts()))
	require.NoError(t, err)
	ap := asAP(t, a)

	_, err = ap.Process(addPlaceReq("ST-999", "PL-A", "", 0))
	assert.ErrorIs(t, err, ErrStationNotFound)

	// list_places 도 미등록 역사면 ErrStationNotFound.
	listRaw, _ := json.Marshal(map[string]any{"command": "list_places", "station": "ST-999"})
	_, err = ap.Process(listRaw)
	assert.ErrorIs(t, err, ErrStationNotFound)
}

// Scenario Place.3: remove_place — 등록 위치 제거, 미등록 위치는 ErrPlaceNotFound.
func TestPlace_Remove(t *testing.T) {
	opts := portOpts()
	opts["station_registry"] = seedST101()
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	require.NoError(t, ap.stations.UpsertPlace("ST-101", PlaceEntry{Place: "PL-A"}))

	rmRaw, _ := json.Marshal(map[string]any{"command": "remove_place", "station": "ST-101", "place": "PL-A"})
	resp, err := ap.Process(rmRaw)
	require.NoError(t, err)
	assertStatusOK(t, resp)

	places, err := ap.stations.ListPlaces("ST-101")
	require.NoError(t, err)
	assert.Empty(t, places)

	// 미등록 위치 제거 → ErrPlaceNotFound.
	_, err = ap.Process(rmRaw)
	assert.ErrorIs(t, err, ErrPlaceNotFound)

	// 미등록 역사에서 제거 → ErrStationNotFound.
	rmMissing, _ := json.Marshal(map[string]any{"command": "remove_place", "station": "ST-999", "place": "PL-A"})
	_, err = ap.Process(rmMissing)
	assert.ErrorIs(t, err, ErrStationNotFound)
}

// Scenario Place.4: list_stations 가 각 역사의 위치를 Order 순으로 함께 반환.
func TestPlace_ListStationsIncludesPlaces(t *testing.T) {
	opts := portOpts()
	opts["station_registry"] = seedST101()
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	require.NoError(t, ap.stations.UpsertPlace("ST-101", PlaceEntry{Place: "PL-B", Order: 2}))
	require.NoError(t, ap.stations.UpsertPlace("ST-101", PlaceEntry{Place: "PL-A", Order: 1}))

	listRaw, _ := json.Marshal(map[string]any{"command": "list_stations"})
	listResp, err := ap.Process(listRaw)
	require.NoError(t, err)

	var out struct {
		Status   string `json:"status"`
		Stations []struct {
			Station string `json:"station"`
			Places  []struct {
				Place string `json:"place"`
				Order int    `json:"order"`
			} `json:"places"`
		} `json:"stations"`
	}
	require.NoError(t, json.Unmarshal(listResp, &out))
	require.Len(t, out.Stations, 1)
	assert.Equal(t, "ST-101", out.Stations[0].Station)
	require.Len(t, out.Stations[0].Places, 2)
	assert.Equal(t, "PL-A", out.Stations[0].Places[0].Place) // order 1
	assert.Equal(t, "PL-B", out.Stations[0].Places[1].Place) // order 2
}

// Scenario Place.5: 영속 라운드트립 — add_place 후 동일 경로 재구성 시 위치 복원.
func TestPlace_PersistRoundtripRestart(t *testing.T) {
	dir := t.TempDir()
	opts := portOpts()
	opts["station_registry"] = seedST101()
	opts["station_registry_path"] = dir

	a1, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap1 := asAP(t, a1)

	resp, err := ap1.Process(addPlaceReq("ST-101", "PL-A", "승강장 A", 3))
	require.NoError(t, err)
	assertStatusOK(t, resp)

	// 재시작 시뮬레이션: 동일 경로에서 새 에이전트 구성.
	a2, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap2 := asAP(t, a2)

	restored, err := ap2.stations.GetPlace("ST-101", "PL-A")
	require.NoError(t, err)
	assert.Equal(t, "승강장 A", restored.DisplayName)
	assert.Equal(t, 3, restored.Order)
}

// Scenario Place.6: 하위호환 — places 필드가 없는 레거시 station_registry.json 이 무에러 로드되고
// 빈 위치 목록을 갖는다.
func TestPlace_BackwardCompatLegacyFile(t *testing.T) {
	dir := t.TempDir()
	// places 필드가 없는 레거시 레지스트리 파일을 직접 기록한다.
	legacy := `{"ST-101":{"station":"ST-101","line":"line-2","display_name":"강남","order":5}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "station_registry.json"), []byte(legacy), 0644))

	opts := portOpts()
	opts["station_registry_path"] = dir
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	entry, err := ap.stations.GetStation("ST-101")
	require.NoError(t, err)
	assert.Equal(t, "line-2", entry.Line)
	assert.Empty(t, entry.Places, "레거시 파일은 빈 위치 목록으로 로드되어야 한다")

	places, err := ap.stations.ListPlaces("ST-101")
	require.NoError(t, err)
	assert.Empty(t, places)

	// 이후 위치 추가도 정상 동작.
	resp, err := ap.Process(addPlaceReq("ST-101", "PL-A", "", 1))
	require.NoError(t, err)
	assertStatusOK(t, resp)
}

// Place 명령 검증 에러: station/place 누락은 ErrInvalidCommand, UpsertPlace 빈 place 는 ErrPlaceNotFound.
func TestPlace_ValidationErrors(t *testing.T) {
	opts := portOpts()
	opts["station_registry"] = seedST101()
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	// add_place: station 누락.
	noStation, _ := json.Marshal(map[string]any{"command": "add_place", "place": "PL-A"})
	_, err = ap.Process(noStation)
	assert.ErrorIs(t, err, ErrInvalidCommand)

	// add_place: place 누락.
	noPlace, _ := json.Marshal(map[string]any{"command": "add_place", "station": "ST-101"})
	_, err = ap.Process(noPlace)
	assert.ErrorIs(t, err, ErrInvalidCommand)

	// remove_place: place 누락.
	rmNoPlace, _ := json.Marshal(map[string]any{"command": "remove_place", "station": "ST-101"})
	_, err = ap.Process(rmNoPlace)
	assert.ErrorIs(t, err, ErrInvalidCommand)

	// list_places: station 누락.
	lsNoStation, _ := json.Marshal(map[string]any{"command": "list_places"})
	_, err = ap.Process(lsNoStation)
	assert.ErrorIs(t, err, ErrInvalidCommand)

	// UpsertPlace 빈 place → ErrPlaceNotFound.
	err = ap.stations.UpsertPlace("ST-101", PlaceEntry{Place: ""})
	assert.ErrorIs(t, err, ErrPlaceNotFound)
}

// GetPlace: 등록 위치 조회 성공, 미등록 위치는 ErrPlaceNotFound, 미등록 역사는 ErrStationNotFound.
func TestPlace_GetPlace(t *testing.T) {
	opts := portOpts()
	opts["station_registry"] = seedST101()
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	require.NoError(t, ap.stations.UpsertPlace("ST-101", PlaceEntry{Place: "PL-A", DisplayName: "승강장 A", Order: 1}))

	got, err := ap.stations.GetPlace("ST-101", "PL-A")
	require.NoError(t, err)
	assert.Equal(t, "승강장 A", got.DisplayName)

	_, err = ap.stations.GetPlace("ST-101", "PL-Z")
	assert.ErrorIs(t, err, ErrPlaceNotFound)

	_, err = ap.stations.GetPlace("ST-999", "PL-A")
	assert.ErrorIs(t, err, ErrStationNotFound)
}

// UpsertPlace 는 동일 place 를 교체(update)하며 목록이 중복되지 않는다.
func TestPlace_UpsertReplaces(t *testing.T) {
	opts := portOpts()
	opts["station_registry"] = seedST101()
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	require.NoError(t, ap.stations.UpsertPlace("ST-101", PlaceEntry{Place: "PL-A", DisplayName: "old", Order: 1}))
	require.NoError(t, ap.stations.UpsertPlace("ST-101", PlaceEntry{Place: "PL-A", DisplayName: "new", Order: 2}))

	places, err := ap.stations.ListPlaces("ST-101")
	require.NoError(t, err)
	require.Len(t, places, 1)
	assert.Equal(t, "new", places[0].DisplayName)
	assert.Equal(t, 2, places[0].Order)
}

// Scenario Place.7: 디바이스 place 는 등록된 위치를 강제 참조하지 않는다 (느슨한 연결, REQ-02-13).
func TestPlace_DeviceLinkageNonStrict(t *testing.T) {
	opts := portOpts()
	opts["station_registry"] = seedST101()
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	// 미등록 place 로 디바이스 등록 → 거부되지 않는다.
	addDev, _ := json.Marshal(map[string]any{
		"command": "add_device", "device_id": "dev-1",
		"station": "ST-101", "place": "UNREGISTERED-PLACE",
	})
	resp, err := ap.Process(addDev)
	require.NoError(t, err)
	assertStatusOK(t, resp)

	dev, err := ap.GetDevice("dev-1")
	require.NoError(t, err)
	assert.Equal(t, "UNREGISTERED-PLACE", dev.Place)
}
