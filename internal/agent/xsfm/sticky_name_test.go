package xsfm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// composite 디바이스 커스텀 이름 sticky (nameOverridden) — 요구 동작 1~7
//
// composite 디바이스(device_id=UUID, Name=station:place:index 파생)에 사용자가 커스텀 이름을
// 지정하면, 이후 station/place/index 를 바꿔도(그 요청에 name 미포함) 재계산되지 않고 커스텀
// 이름이 유지되어야 한다(sticky). 명시적 빈 이름은 override 를 해제해 파생 이름으로 복귀시킨다.
// ---------------------------------------------------------------------------

// addCompositeWithName 는 attribute 모드에서 name 을 포함해 device_id 미지정 add_device 를 호출하고
// 생성된 UUID 를 반환한다(요구 동작 1 검증용 헬퍼).
func addCompositeWithName(t *testing.T, ap *XSFMAgent, station, place string, index int, name string) string {
	t.Helper()
	body := map[string]any{"command": "add_device", "station": station, "place": place, "index": index, "name": name}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := ap.Process(raw)
	require.NoError(t, err)
	var r map[string]any
	require.NoError(t, json.Unmarshal(resp, &r))
	id, _ := r["device_id"].(string)
	require.True(t, isUUID(id))
	return id
}

// 요구 동작 1: composite add + 비어있지 않은 name → Name=커스텀, nameOverridden=true.
func TestStickyName_CompositeAddWithCustomName(t *testing.T) {
	ap, _ := attrAgentWithMock(t, attrOpts())

	resp, err := ap.Process([]byte(`{"command":"add_device","station":"st01","place":"PL-A","index":3,"name":"대합실환기구"}`))
	require.NoError(t, err)
	var r map[string]any
	require.NoError(t, json.Unmarshal(resp, &r))
	assert.Equal(t, "대합실환기구", r["name"], "응답 name 은 사용자 지정 커스텀 이름")

	uuidID, _ := r["device_id"].(string)
	dev, err := ap.GetDevice(uuidID)
	require.NoError(t, err)
	assert.Equal(t, "대합실환기구", dev.Name)
	assert.True(t, dev.composite, "합성 주소 모델 디바이스")
	assert.True(t, dev.nameOverridden, "커스텀 이름이 sticky override 로 표시되어야 한다")
}

// 요구 동작 2: composite add + name 미제공 → Name=composeName, nameOverridden=false (기존 동작 보존).
func TestStickyName_CompositeAddWithoutName(t *testing.T) {
	ap, _ := attrAgentWithMock(t, attrOpts())
	uuidID := addDeviceUUID(t, ap, "st01", "PL-A", 3)

	dev, err := ap.GetDevice(uuidID)
	require.NoError(t, err)
	assert.Equal(t, "st01:PL-A:003", dev.Name, "name 미제공 → 파생 이름")
	assert.False(t, dev.nameOverridden, "파생 이름은 override 아님")
}

// 요구 동작 2 보강: composite add + 빈 name → composeName, nameOverridden=false.
func TestStickyName_CompositeAddWithEmptyName(t *testing.T) {
	ap, _ := attrAgentWithMock(t, attrOpts())

	resp, err := ap.Process([]byte(`{"command":"add_device","station":"st01","place":"PL-A","index":3,"name":""}`))
	require.NoError(t, err)
	var r map[string]any
	require.NoError(t, json.Unmarshal(resp, &r))
	uuidID, _ := r["device_id"].(string)

	dev, err := ap.GetDevice(uuidID)
	require.NoError(t, err)
	assert.Equal(t, "st01:PL-A:003", dev.Name, "빈 name → 파생 이름")
	assert.False(t, dev.nameOverridden)
}

// 요구 동작 3: set_device + present(name) && name != "" → Name=커스텀, nameOverridden=true.
func TestStickyName_SetDeviceCustomName(t *testing.T) {
	ap, _ := attrAgentWithMock(t, attrOpts())
	uuidID := addDeviceUUID(t, ap, "st01", "PL-A", 3)

	_, err := ap.Process([]byte(`{"command":"set_device","device_id":"` + uuidID + `","name":"승강장송풍기"}`))
	require.NoError(t, err)

	dev, err := ap.GetDevice(uuidID)
	require.NoError(t, err)
	assert.Equal(t, "승강장송풍기", dev.Name)
	assert.True(t, dev.nameOverridden, "set_device 커스텀 이름 → override")
}

// 요구 동작 4: set_device + present(name) && name == "" → nameOverridden=false, Name=composeName 복귀.
func TestStickyName_SetDeviceEmptyNameClearsOverride(t *testing.T) {
	ap, _ := attrAgentWithMock(t, attrOpts())
	uuidID := addCompositeWithName(t, ap, "st01", "PL-A", 3, "커스텀이름")

	// 먼저 override 상태 확인.
	dev, err := ap.GetDevice(uuidID)
	require.NoError(t, err)
	require.True(t, dev.nameOverridden)

	// 명시적 빈 이름 → override 해제 + 파생 이름 복귀.
	_, err = ap.Process([]byte(`{"command":"set_device","device_id":"` + uuidID + `","name":""}`))
	require.NoError(t, err)

	dev, err = ap.GetDevice(uuidID)
	require.NoError(t, err)
	assert.False(t, dev.nameOverridden, "빈 이름은 override 해제")
	assert.Equal(t, "st01:PL-A:003", dev.Name, "override 해제 후 composite 파생 이름으로 복귀")
}

// 요구 동작 4 보강: override 해제가 같은 요청의 새 주소를 반영해 재계산한다.
func TestStickyName_SetDeviceEmptyNameWithAddressChange(t *testing.T) {
	ap, _ := attrAgentWithMock(t, attrOpts())
	uuidID := addCompositeWithName(t, ap, "st01", "PL-A", 3, "커스텀이름")

	// 같은 set_device 요청에서 index 변경 + 빈 이름 → 새 index 로 파생 이름 재계산.
	_, err := ap.Process([]byte(`{"command":"set_device","device_id":"` + uuidID + `","index":9,"name":""}`))
	require.NoError(t, err)

	dev, err := ap.GetDevice(uuidID)
	require.NoError(t, err)
	assert.False(t, dev.nameOverridden)
	assert.Equal(t, "st01:PL-A:009", dev.Name, "override 해제 시 새 주소로 재계산")
}

// 요구 동작 5: sticky across address change — 커스텀 이름 set 후 station/place/index 변경(name
// 미제공) → nameOverridden==true 이면 재계산 안 함(커스텀 이름 유지). false 면 재계산.
func TestStickyName_StickyAcrossAddressChange(t *testing.T) {
	ap, _ := attrAgentWithMock(t, attrOpts())

	// (a) sticky=true 경로: 커스텀 이름 → index 변경(name 미포함) → 이름 유지.
	stickyID := addCompositeWithName(t, ap, "st01", "PL-A", 3, "고정이름")
	_, err := ap.Process([]byte(`{"command":"set_device","device_id":"` + stickyID + `","index":7}`))
	require.NoError(t, err)
	dev, err := ap.GetDevice(stickyID)
	require.NoError(t, err)
	assert.Equal(t, 7, dev.Index, "주소는 변경됨")
	assert.Equal(t, "고정이름", dev.Name, "sticky 커스텀 이름은 주소 변경에도 유지되어야 한다")
	assert.True(t, dev.nameOverridden)

	// 보조 인덱스도 새 주소로 갱신됐는지 확인(주소 변경 자체는 정상 처리).
	idFromIdx, ok := secondaryLookup(ap, "st01", "PL-A", 7)
	require.True(t, ok)
	assert.Equal(t, stickyID, idFromIdx)

	// (b) sticky=false 경로: 파생 이름 → index 변경 → 재계산.
	plainID := addDeviceUUID(t, ap, "st02", "PL-B", 3)
	_, err = ap.Process([]byte(`{"command":"set_device","device_id":"` + plainID + `","index":8}`))
	require.NoError(t, err)
	dev2, err := ap.GetDevice(plainID)
	require.NoError(t, err)
	assert.Equal(t, "st02:PL-B:008", dev2.Name, "override 아닌 composite 는 주소 변경 시 재계산")
	assert.False(t, dev2.nameOverridden)
}

// 요구 동작 6: persist round-trip — 커스텀 이름과 sticky 상태가 재시작 후에도 유지되고, 재시작
// 후 주소 변경에도 커스텀 이름이 재계산되지 않는다.
func TestStickyName_PersistRoundtripSticky(t *testing.T) {
	dir := t.TempDir()
	opts := attrOpts()
	opts["transport_mode"] = "port" // 브로커 배선 없이 Init 복원만 검증.
	opts["registry_path"] = dir

	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	uuidID := addCompositeWithName(t, ap, "st01", "PL-A", 3, "영속커스텀")
	require.NoError(t, ap.Stop(context.Background()))

	// 재시작 → 커스텀 이름 + sticky 상태 복원.
	a2, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap2 := asAP(t, a2)

	dev, err := ap2.GetDevice(uuidID)
	require.NoError(t, err)
	assert.Equal(t, "영속커스텀", dev.Name, "커스텀 이름 복원")
	assert.True(t, dev.composite, "UUID 디바이스는 composite 로 복원")
	assert.True(t, dev.nameOverridden, "sticky 상태가 재시작 후에도 복원되어야 한다")

	// 재시작 후 주소 변경(name 미포함) → 여전히 커스텀 이름 유지(재계산 안 함).
	_, err = ap2.Process([]byte(`{"command":"set_device","device_id":"` + uuidID + `","index":11}`))
	require.NoError(t, err)
	dev2, err := ap2.GetDevice(uuidID)
	require.NoError(t, err)
	assert.Equal(t, 11, dev2.Index)
	assert.Equal(t, "영속커스텀", dev2.Name, "재시작 후에도 sticky 이름 유지")
}

// 요구 동작 6 보강: 파생 이름(non-sticky) composite 도 라운드트립되며 재시작 후 주소 변경 시 재계산.
func TestStickyName_PersistRoundtripNonSticky(t *testing.T) {
	dir := t.TempDir()
	opts := attrOpts()
	opts["transport_mode"] = "port"
	opts["registry_path"] = dir

	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	uuidID := addDeviceUUID(t, ap, "st01", "PL-A", 3)
	require.NoError(t, ap.Stop(context.Background()))

	a2, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap2 := asAP(t, a2)

	dev, err := ap2.GetDevice(uuidID)
	require.NoError(t, err)
	assert.False(t, dev.nameOverridden, "파생 이름은 override 아님으로 복원")

	_, err = ap2.Process([]byte(`{"command":"set_device","device_id":"` + uuidID + `","index":5}`))
	require.NoError(t, err)
	dev2, err := ap2.GetDevice(uuidID)
	require.NoError(t, err)
	assert.Equal(t, "st01:PL-A:005", dev2.Name, "재시작 후 override 아닌 composite 는 재계산")
}

// 요구 동작 7: blob(non-composite) 디바이스 — Name 은 항상 사용자 관리, 재계산 없음(기존과 동일).
func TestStickyName_BlobDeviceUnchanged(t *testing.T) {
	ap, _ := directAgentWithMock(t, directOpts())

	// 명시적 device_id(blob) 등록 + 커스텀 이름.
	_, err := ap.Process([]byte(`{"command":"add_device","device_id":"ap-101","name":"블롭이름","station":"ST-1","place":"p","index":2}`))
	require.NoError(t, err)
	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.False(t, dev.composite, "blob 디바이스")
	assert.Equal(t, "블롭이름", dev.Name)

	// station/place/index 변경(name 미포함) → blob 은 Name 재계산 없음(사용자 관리값 보존).
	_, err = ap.Process([]byte(`{"command":"set_device","device_id":"ap-101","index":9}`))
	require.NoError(t, err)
	dev, err = ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.Equal(t, 9, dev.Index)
	assert.Equal(t, "블롭이름", dev.Name, "blob 은 주소 변경에도 Name 재계산 없음")

	// blob + 명시적 빈 이름 → 준 값(빈값) 그대로 반영(기존 동작 보존).
	_, err = ap.Process([]byte(`{"command":"set_device","device_id":"ap-101","name":""}`))
	require.NoError(t, err)
	dev, err = ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.Equal(t, "", dev.Name, "blob 명시적 빈 이름 → 빈값 반영(사용자 관리)")
}
