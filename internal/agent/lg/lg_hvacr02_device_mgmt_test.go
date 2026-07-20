// lg_hvacr02_device_mgmt_test.go — LG ICP-02 캡처 에이전트의 디바이스 관리 명령
// (list_devices / remove_device / set_device) 및 report_enabled 방출 게이트 검증.
//
// Samsung NASA(Hvacr01Agent) 와 동일한 DTO/동작을 미러링한다. 캡처 전용이므로
// add_device 는 없다(자동 발견 전용). report_enabled/삭제는 In-memory 만 유지한다.
package lg

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// fakeIcp02DeviceIDRepo 는 (agentName, unitID) → 결정적 UUID 를 반환하는 테스트용 저장소이다.
// device_id(UUID) 역매칭 경로(addrByDeviceUUID)를 실제 저장소 없이 검증하기 위함.
type fakeIcp02DeviceIDRepo struct{}

func (fakeIcp02DeviceIDRepo) GetOrCreate(_ context.Context, agentName, unitID string) (string, error) {
	return "uuid:" + agentName + ":" + unitID, nil
}

func (fakeIcp02DeviceIDRepo) Get(_ context.Context, agentName, unitID string) (string, error) {
	return "uuid:" + agentName + ":" + unitID, nil
}

// withFakeDeviceIDRepoIcp02 는 결정적 device_id 저장소를 설정하고 테스트 종료 시 복원한다.
func withFakeDeviceIDRepoIcp02(t *testing.T) {
	t.Helper()
	original := agent.GetDeviceIDRepository()
	agent.SetDeviceIDRepository(fakeIcp02DeviceIDRepo{})
	t.Cleanup(func() { agent.SetDeviceIDRepository(original) })
}

// newDeviceMgmtTestAgent 는 devices/lastStates 맵과 agentConfig 이름이 설정된 테스트
// 에이전트를 만든다. controller_merge 헬퍼를 재사용해 ControllerAddress 도 설정한다.
func newDeviceMgmtTestAgent(t *testing.T) *Hvacr02Agent {
	t.Helper()
	a := newControllerMergeTestAgent(t)
	a.agentConfig = agent.AgentConfig{Name: "test-lg02"}
	return a
}

// TestHvacr02_ReportEnabled_False_SuppressesEmit 는 report_enabled=false 인 디바이스가
// device_state(change/report) 방출을 모두 억제하는지 검증한다.
func TestHvacr02_ReportEnabled_False_SuppressesEmit(t *testing.T) {
	withFakeDeviceIDRepoIcp02(t)
	a := newDeviceMgmtTestAgent(t)
	ts := time.Now()

	// 최초 관측 (indoor 44550066 → controller 보고) → report 1건 방출.
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("ON"), IndoorTempC: f64Ptr(24.0)}, ts)
	a.mu.RLock()
	emitAfterFirst := a.recentIdx
	a.mu.RUnlock()
	require.Equal(t, 1, emitAfterFirst, "report_enabled=true 는 최초 관측을 report 로 방출해야 한다")

	// report_enabled off 로 전환.
	a.mu.Lock()
	a.devices["44550066"].ReportEnabled = false
	a.mu.Unlock()

	// 실제 상태 변경(온도) 유도 → off 이므로 change 방출 억제.
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("ON"), IndoorTempC: f64Ptr(26.0)}, ts.Add(time.Second))
	a.mu.RLock()
	emitAfterChange := a.recentIdx
	a.mu.RUnlock()
	assert.Equal(t, emitAfterFirst, emitAfterChange, "report_enabled=false 는 change 방출을 억제해야 한다")

	// 주기 report(heartbeat)도 억제되는지 확인.
	dev := a.devices["44550066"]
	a.mu.Lock()
	a.emitDeviceStateLocked(dev, "report")
	a.mu.Unlock()
	a.mu.RLock()
	emitAfterReport := a.recentIdx
	a.mu.RUnlock()
	assert.Equal(t, emitAfterFirst, emitAfterReport, "report_enabled=false 는 주기 report 도 억제해야 한다")
}

// TestHvacr02_ReportEnabled_True_Emits 는 report_enabled=true(기본) 디바이스가 정상
// 방출하는지 검증한다(억제 회귀 방지).
func TestHvacr02_ReportEnabled_True_Emits(t *testing.T) {
	withFakeDeviceIDRepoIcp02(t)
	a := newDeviceMgmtTestAgent(t)
	ts := time.Now()

	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("ON"), IndoorTempC: f64Ptr(24.0)}, ts)

	a.mu.RLock()
	emitted := a.recentIdx
	re := a.devices["44550066"].ReportEnabled
	a.mu.RUnlock()
	assert.Equal(t, 1, emitted, "report_enabled=true 는 device_state 를 방출해야 한다")
	assert.True(t, re, "자동 발견 디바이스는 기본 report_enabled=true 여야 한다")
}

// TestHvacr02_SetDevice_TogglesReportEnabled 는 set_device 명령이 report_enabled/name 을
// 갱신하고 응답에 반영하는지 검증한다.
func TestHvacr02_SetDevice_TogglesReportEnabled(t *testing.T) {
	withFakeDeviceIDRepoIcp02(t)
	a := newDeviceMgmtTestAgent(t)
	ts := time.Now()
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("ON")}, ts)

	b, err := a.processSetDevice(&hvacr02ProcessRequest{
		Command: "set_device",
		Address: "44550066",
		Params:  map[string]any{"report_enabled": false, "name": "거실"},
	})
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(b, &resp))
	assert.Equal(t, "ok", resp["status"])
	assert.Equal(t, false, resp["report_enabled"])
	assert.Equal(t, "거실", resp["name"])
	assert.Equal(t, "44550066", resp["address"])

	a.mu.RLock()
	dev := a.devices["44550066"]
	re, label := dev.ReportEnabled, dev.Label
	a.mu.RUnlock()
	assert.False(t, re, "dev.ReportEnabled 가 false 로 갱신되어야 한다")
	assert.Equal(t, "거실", label, "dev.Label 이 갱신되어야 한다")
}

// TestHvacr02_SetDevice_ByDeviceID 는 device_id(UUID) 로도 set_device 가 동작하는지 검증한다.
func TestHvacr02_SetDevice_ByDeviceID(t *testing.T) {
	withFakeDeviceIDRepoIcp02(t)
	a := newDeviceMgmtTestAgent(t)
	ts := time.Now()
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("ON")}, ts)

	uuid := agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, "44550066")
	require.NotEmpty(t, uuid)

	b, err := a.processSetDevice(&hvacr02ProcessRequest{
		Command: "set_device",
		Params:  map[string]any{"device_id": uuid, "report_enabled": false},
	})
	require.NoError(t, err)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(b, &resp))
	assert.Equal(t, "ok", resp["status"])
	assert.Equal(t, false, resp["report_enabled"])

	a.mu.RLock()
	re := a.devices["44550066"].ReportEnabled
	a.mu.RUnlock()
	assert.False(t, re, "device_id(UUID) 로도 report_enabled 가 갱신되어야 한다")
}

// TestHvacr02_RemoveDevice_ByAddress 는 remove_device(address) 가 In-memory 디바이스를
// 삭제하는지 검증한다.
func TestHvacr02_RemoveDevice_ByAddress(t *testing.T) {
	withFakeDeviceIDRepoIcp02(t)
	a := newDeviceMgmtTestAgent(t)
	ts := time.Now()
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("ON")}, ts)

	a.mu.RLock()
	_, existed := a.devices["44550066"]
	a.mu.RUnlock()
	require.True(t, existed, "삭제 전 디바이스가 존재해야 한다")

	b, err := a.processRemoveDevice(&hvacr02ProcessRequest{
		Command: "remove_device",
		Address: "44550066",
	})
	require.NoError(t, err)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(b, &resp))
	assert.Equal(t, "ok", resp["status"])
	assert.Equal(t, "44550066", resp["address"])

	a.mu.RLock()
	_, stillThere := a.devices["44550066"]
	_, stateGone := a.lastStates["44550066"]
	a.mu.RUnlock()
	assert.False(t, stillThere, "remove_device 후 디바이스가 삭제되어야 한다")
	assert.False(t, stateGone, "remove_device 후 lastStates 항목도 삭제되어야 한다")
}

// TestHvacr02_RemoveDevice_ByDeviceID 는 프론트엔드가 보내는 device_id(UUID) 로도
// 삭제가 동작하는지 검증한다(역매칭 헬퍼 addrByDeviceUUID 경로).
func TestHvacr02_RemoveDevice_ByDeviceID(t *testing.T) {
	withFakeDeviceIDRepoIcp02(t)
	a := newDeviceMgmtTestAgent(t)
	ts := time.Now()
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("ON")}, ts)

	uuid := agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, "44550066")
	require.NotEmpty(t, uuid)

	_, err := a.processRemoveDevice(&hvacr02ProcessRequest{
		Command: "remove_device",
		Params:  map[string]any{"device_id": uuid},
	})
	require.NoError(t, err)

	a.mu.RLock()
	_, stillThere := a.devices["44550066"]
	a.mu.RUnlock()
	assert.False(t, stillThere, "device_id(UUID) 로도 삭제되어야 한다")
}

// TestHvacr02_RemoveDevice_NotFound 는 존재하지 않는 디바이스 삭제 시 에러를 반환하는지
// 검증한다.
func TestHvacr02_RemoveDevice_NotFound(t *testing.T) {
	withFakeDeviceIDRepoIcp02(t)
	a := newDeviceMgmtTestAgent(t)

	_, err := a.processRemoveDevice(&hvacr02ProcessRequest{
		Command: "remove_device",
		Address: "deadbeef",
	})
	assert.ErrorIs(t, err, ErrDeviceNotFound)
}

// TestHvacr02_ListDevices_ReturnsReportEnabled 는 list_devices 가 각 디바이스의
// report_enabled 와 device_id 를 포함해 반환하는지 검증한다.
func TestHvacr02_ListDevices_ReturnsReportEnabled(t *testing.T) {
	withFakeDeviceIDRepoIcp02(t)
	a := newDeviceMgmtTestAgent(t)
	ts := time.Now()
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("ON")}, ts)
	a.updateDeviceState("44550067", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("ON")}, ts)

	// 한 대만 off.
	a.mu.Lock()
	a.devices["44550066"].ReportEnabled = false
	a.mu.Unlock()

	b, err := a.processListDevices()
	require.NoError(t, err)

	var resp struct {
		Status  string           `json:"status"`
		Devices []map[string]any `json:"devices"`
	}
	require.NoError(t, json.Unmarshal(b, &resp))
	assert.Equal(t, "ok", resp.Status)

	byAddr := map[string]map[string]any{}
	for _, d := range resp.Devices {
		byAddr[d["address"].(string)] = d
	}
	require.Contains(t, byAddr, "44550066")
	require.Contains(t, byAddr, "44550067")

	assert.Equal(t, false, byAddr["44550066"]["report_enabled"], "off 디바이스는 report_enabled=false")
	assert.Equal(t, true, byAddr["44550067"]["report_enabled"], "on 디바이스는 report_enabled=true")
	assert.NotEmpty(t, byAddr["44550066"]["device_id"], "device_id(UUID) 가 노출되어야 한다")
	assert.Equal(t, "HVACR.IDU", byAddr["44550066"]["device_type"])
}

// TestHvacr02_Provider_ExposesReportEnabled 는 provider→adapter 경로가 report_enabled 를
// REST DTO 로 노출하는지 검증한다(reportEnabledCarrier optional-interface).
func TestHvacr02_Provider_ExposesReportEnabled(t *testing.T) {
	withFakeDeviceIDRepoIcp02(t)
	a := newDeviceMgmtTestAgent(t)
	ts := time.Now()
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("ON")}, ts)
	a.mu.Lock()
	a.devices["44550066"].ReportEnabled = false
	a.mu.Unlock()

	prov := NewHvacr02DeviceProvider(a)
	d, err := prov.Device("test-lg02:44550066")
	require.NoError(t, err)

	rc, ok := d.(interface{ ReportEnabled() bool })
	require.True(t, ok, "adapter 는 ReportEnabled() 를 구현해야 한다")
	assert.False(t, rc.ReportEnabled(), "off 디바이스는 adapter.ReportEnabled()=false")
}
