package airpurifier

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// SPEC-AIRPURIFIER-001 — device_id = UUID + 위치 계층 보조 인덱스 모델
//
// device_id 는 생성된 UUID(로스터 PRIMARY 키)이고, (station,place,index) 합성 주소는 유입 상태
// 토픽을 매칭하는 SECONDARY 인덱스이다. 아래 테스트는 미션의 필수 시나리오를 TDD 로 검증한다.
// ---------------------------------------------------------------------------

// addDeviceUUID 는 device_id 없이 add_device 를 호출하고 생성된 UUID device_id 를 반환한다.
func addDeviceUUID(t *testing.T, ap *AirPurifierAgent, station, place string, index int) string {
	t.Helper()
	body := map[string]any{"command": "add_device", "station": station, "place": place, "index": index}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := ap.Process(raw)
	require.NoError(t, err)
	var r map[string]any
	require.NoError(t, json.Unmarshal(resp, &r))
	id, _ := r["device_id"].(string)
	require.True(t, isUUID(id), "add_device(device_id 미지정)는 UUID 를 생성해야 한다")
	return id
}

// 1) add_device without device_id → UUID 생성, Name=station:place:003, UUID 조회 + 보조 인덱스 조회.
func TestIdentity_AddDeviceGeneratesUUID(t *testing.T) {
	ap, _ := attrAgentWithMock(t, attrOpts())

	resp, err := ap.Process([]byte(`{"command":"add_device","station":"st01","place":"PL-A","index":3}`))
	require.NoError(t, err)
	var r map[string]any
	require.NoError(t, json.Unmarshal(resp, &r))
	uuidID, _ := r["device_id"].(string)
	require.True(t, isUUID(uuidID), "응답 device_id 는 생성된 UUID")
	assert.Equal(t, "st01:PL-A:003", r["name"], "응답 name 은 3자리 0채움 합성")

	// UUID 로 조회.
	dev, err := ap.GetDevice(uuidID)
	require.NoError(t, err)
	assert.Equal(t, "st01:PL-A:003", dev.Name)
	assert.Equal(t, "bridge", dev.Source)
	assert.Equal(t, 3, dev.Index)

	// 보조 인덱스(합성 주소)로 조회 → 같은 UUID.
	id, ok := secondaryLookup(ap, "st01", "PL-A", 3)
	require.True(t, ok)
	assert.Equal(t, uuidID, id)
}

// 2) 유입 토픽 상태 → 사전 등록 디바이스에 보조 인덱스로 매칭(device_id 로가 아님), 상태 갱신.
func TestIdentity_TopicMatchesRegisteredViaSecondary(t *testing.T) {
	ap, mock := attrAgentWithMock(t, attrOpts())
	uuidID := addDeviceUUID(t, ap, "st01", "PL-A", 3)

	// 토픽은 device_id(UUID)가 아니라 /st01/PL-A/.../3/ 을 나른다 → 보조 인덱스로 매칭되어야 한다.
	mock.deliver("state/ui-line/st01/PL-A/bse9000/3/power", []byte("on"))

	dev, err := ap.GetDevice(uuidID)
	require.NoError(t, err)
	assert.True(t, dev.Power, "보조 인덱스로 매칭된 디바이스의 상태가 갱신되어야 한다")
	assert.Len(t, ap.ListDevices(), 1, "device_id 가 아닌 보조 인덱스 매칭이므로 신규 생성이 없어야 한다")

	idFromIdx, ok := secondaryLookup(ap, "st01", "PL-A", 3)
	require.True(t, ok)
	assert.Equal(t, uuidID, idFromIdx, "보조 인덱스가 사전 등록 UUID 를 가리켜야 한다")
}

// 3) index 정규화 — index=3 디바이스가 토픽 /003/ 과 /3/ 양쪽으로 매칭된다(둘 다 int 3 으로 정규화).
func TestIdentity_IndexNormalization(t *testing.T) {
	ap, mock := attrAgentWithMock(t, attrOpts())
	uuidID := addDeviceUUID(t, ap, "st01", "PL-A", 3)

	mock.deliver("state/ui-line/st01/PL-A/bse9000/003/power", []byte("on"))  // 0채움
	mock.deliver("state/ui-line/st01/PL-A/bse9000/3/fan_speed", []byte("2")) // 평문

	dev, err := ap.GetDevice(uuidID)
	require.NoError(t, err)
	assert.True(t, dev.Power, "/003/ 이 index 3 으로 정규화되어 매칭")
	assert.Equal(t, 2, dev.FanSpeed, "/3/ 이 index 3 으로 정규화되어 매칭")
	assert.Len(t, ap.ListDevices(), 1, "003 과 3 은 동일 int 3 으로 정규화되어 한 디바이스에 매칭")
}

// 4) auto 등록 — 미등록 (station,place,index) 토픽 → UUID + 합성 Name + Source="auto".
func TestIdentity_AutoRegisterUnknownTopic(t *testing.T) {
	ap, mock := attrAgentWithMock(t, attrOpts())

	mock.deliver("state/ui-line/st09/PL-Z/bse9000/12/power", []byte("on"))

	id, ok := secondaryLookup(ap, "st09", "PL-Z", 12)
	require.True(t, ok, "미등록 토픽은 새 디바이스를 auto 등록해야 한다")
	assert.True(t, isUUID(id), "auto 등록 device_id 는 UUID")
	dev, err := ap.GetDevice(id)
	require.NoError(t, err)
	assert.Equal(t, "auto", dev.Source)
	assert.Equal(t, "st09:PL-Z:012", dev.Name)
	assert.True(t, dev.Power)
}

// 5) 같은 (station,place,index)로 add_device 두 번 → 두 번째는 ErrDeviceAlreadyRegistered(중복 UUID 없음).
func TestIdentity_AddDeviceDuplicateComposite(t *testing.T) {
	ap, _ := attrAgentWithMock(t, attrOpts())

	_, err := ap.Process([]byte(`{"command":"add_device","station":"st01","place":"PL-A","index":3}`))
	require.NoError(t, err)
	_, err = ap.Process([]byte(`{"command":"add_device","station":"st01","place":"PL-A","index":3}`))
	assert.ErrorIs(t, err, ErrDeviceAlreadyRegistered, "같은 합성 주소 재등록은 거부")
	assert.Len(t, ap.ListDevices(), 1, "중복 UUID 를 만들지 않아야 한다")
}

// 6) set_device index 변경 → Name 재계산 + 보조 인덱스 갱신(옛 키 미매칭, 새 키 매칭).
func TestIdentity_SetDeviceChangeIndex(t *testing.T) {
	ap, _ := attrAgentWithMock(t, attrOpts())
	uuidID := addDeviceUUID(t, ap, "st01", "PL-A", 3)

	_, err := ap.Process([]byte(`{"command":"set_device","device_id":"` + uuidID + `","index":7}`))
	require.NoError(t, err)

	dev, err := ap.GetDevice(uuidID)
	require.NoError(t, err)
	assert.Equal(t, 7, dev.Index)
	assert.Equal(t, "st01:PL-A:007", dev.Name, "index 변경 시 Name 재계산")

	_, ok := secondaryLookup(ap, "st01", "PL-A", 3)
	assert.False(t, ok, "옛 compositeKey 는 더 이상 매칭되지 않아야 한다")
	newID, ok := secondaryLookup(ap, "st01", "PL-A", 7)
	require.True(t, ok)
	assert.Equal(t, uuidID, newID, "새 compositeKey 가 같은 UUID 를 가리켜야 한다")
}

// 7) set_power(device_id=UUID) → 단일 제어; station 셀렉터 fan-out → 대상들이 UUID device_id 로 도출.
func TestIdentity_ControlByUUIDAndSelector(t *testing.T) {
	ap, mock := attrAgentWithMock(t, attrOpts())
	id1 := addDeviceUUID(t, ap, "st01", "PL-A", 1)
	_ = addDeviceUUID(t, ap, "st01", "PL-B", 2)

	// device_id(UUID) 단일 제어 → 해당 디바이스만.
	_, err := ap.Process([]byte(`{"command":"set_power","device_id":"` + id1 + `","params":{"power":true}}`))
	require.NoError(t, err)
	require.Len(t, mock.published, 1)
	assert.Equal(t, "cmd/ui-line/st01/PL-A/bse9000/1/power", mock.published[0].topic,
		"명령 토픽은 device_id 가 아니라 station/place/index 로 렌더")

	// station 셀렉터 fan-out → 두 디바이스 모두, 대상 device_id 는 UUID.
	resp, err := ap.Process([]byte(`{"command":"set_power","station":"st01","params":{"power":false}}`))
	require.NoError(t, err)
	var fo map[string]any
	require.NoError(t, json.Unmarshal(resp, &fo))
	assert.Equal(t, "ok", fo["status"])
	results, ok := fo["results"].([]any)
	require.True(t, ok)
	require.Len(t, results, 2, "station 의 두 디바이스가 대상")
	for _, r := range results {
		rid, _ := r.(map[string]any)["device_id"].(string)
		assert.True(t, isUUID(rid), "fan-out 대상 device_id 는 UUID")
	}
}

// 8) 영속화 라운드트립 — add_device(UUID) → 재시작 → UUID 로 복원 + 보조 인덱스 재구축(토픽 재매칭).
func TestIdentity_PersistUUIDRoundtrip(t *testing.T) {
	dir := t.TempDir()
	opts := attrOpts()
	opts["transport_mode"] = "port" // 브로커 배선 없이 Init 복원만 검증.
	opts["registry_path"] = dir

	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	resp, err := ap.Process([]byte(`{"command":"add_device","station":"st01","place":"PL-A","index":3}`))
	require.NoError(t, err)
	var r map[string]any
	require.NoError(t, json.Unmarshal(resp, &r))
	uuidID, _ := r["device_id"].(string)
	require.True(t, isUUID(uuidID))
	require.NoError(t, ap.Stop(context.Background()))

	// 동일 설정으로 재시작.
	a2, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap2 := asAP(t, a2)

	// UUID 로 복원.
	dev, err := ap2.GetDevice(uuidID)
	require.NoError(t, err)
	assert.Equal(t, "st01", dev.Station)
	assert.Equal(t, 3, dev.Index)
	assert.Equal(t, "bridge", dev.Source)

	// 보조 인덱스 재구축 → 토픽이 여전히 같은 UUID 로 매칭.
	id, ok := secondaryLookup(ap2, "st01", "PL-A", 3)
	require.True(t, ok, "재시작 후 보조 인덱스가 재구축되어야 한다")
	assert.Equal(t, uuidID, id)

	ap2.FeedStateFromTopic("state/ui-line/st01/PL-A/bse9000/3/power", []byte("on"))
	dev2, err := ap2.GetDevice(uuidID)
	require.NoError(t, err)
	assert.True(t, dev2.Power, "복원된 디바이스가 토픽 유입으로 갱신되어야 한다")
	assert.Len(t, ap2.ListDevices(), 1, "토픽이 신규 생성 없이 복원 디바이스에 매칭")
}
