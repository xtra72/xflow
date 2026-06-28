// lg_hvacr02_controller_merge_test.go — controller(실외기) 디바이스 상태 진동 회귀 방지.
//
// 배경(2026-06-16 진단): controller(SA=44550000)는 실내기(DA)별로 서로 다른 실외
// 서비스 상태(outdoor_active/refrigerant_on/op_mode/compressor)를 0204 status 프레임
// 으로 보낸다. 기존 updateDeviceState 는 0201 외 모든 status 를 SA(=controller) 단일
// 디바이스에 병합하여, 실내기 3대의 서로 다른 실외 컨텍스트가 controller 에서 충돌하며
// ~5초 간격으로 진동했다.
//
// 모델 B(전역만 갱신): controller→특정 실내기(DA≠broadcast) 0204 프레임은 controller
// 상태 병합에서 제외한다. broadcast(ffffffff) 등 전역 프레임과 실내기→controller 보고만
// 기존대로 병합한다.
package lg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newControllerMergeTestAgent 는 controller_address 가 설정되고 device/state 맵이
// 초기화된 테스트 에이전트를 만든다.
func newControllerMergeTestAgent(t *testing.T) *Hvacr02Agent {
	t.Helper()
	a := newTestHvacr02Agent(t, &hvacr02MockTransport{})
	a.hvacr02Config.ControllerAddress = "44550000"
	a.devices = make(map[string]*Icp02Device)
	a.lastStates = make(map[string]Icp02DeviceState)
	return a
}

func boolPtr(b bool) *bool { return &b }

// @spec SPEC-REMOTE-001 (무관) — 진단 회귀 방지.
// 재현: controller→실내기(DA) 0204 프레임은 controller 디바이스 상태를 오염시키면
// 안 된다. 서로 다른 실내기(65: 실외 active, 66: 실외 inactive)로 가는 프레임이
// controller 에 병합되면 outdoor_active 가 진동한다.
func TestUpdateDeviceState_ControllerToIndoor_DoesNotPolluteController(t *testing.T) {
	a := newControllerMergeTestAgent(t)
	ts := time.Now()

	// controller → 실내기 65: 해당 실내기 가동 (실외 active)
	a.updateDeviceState("44550000", "44550065", "0204", "",
		&Icp02DecodedPayload{OutdoorActive: boolPtr(true), RefrigerantOn: boolPtr(true)}, ts)
	// controller → 실내기 66: 해당 실내기 idle (실외 inactive)
	a.updateDeviceState("44550000", "44550066", "0204", "",
		&Icp02DecodedPayload{OutdoorActive: boolPtr(false), RefrigerantOn: boolPtr(false)}, ts)

	a.mu.RLock()
	defer a.mu.RUnlock()

	ctrl := a.devices["44550000"]
	require.NotNil(t, ctrl, "controller 디바이스는 liveness 로 등록되어야 한다")
	require.NotNil(t, ctrl.State)
	// 모델 B: controller→실내기 프레임은 controller 상태를 갱신하지 않는다.
	assert.Nil(t, ctrl.State.OutdoorActive,
		"controller→실내기 status 는 controller 의 outdoor_active 를 오염시키면 안 된다")
	assert.Nil(t, ctrl.State.RefrigerantOn,
		"controller→실내기 status 는 controller 의 refrigerant_on 을 오염시키면 안 된다")
}

// 실내기→controller(자체 보고) status 는 기존대로 실내기(SA) 디바이스에 병합된다.
func TestUpdateDeviceState_IndoorToController_StillUpdatesIndoor(t *testing.T) {
	a := newControllerMergeTestAgent(t)
	ts := time.Now()
	temp := 24.5

	a.updateDeviceState("44550065", "44550000", "0204", "",
		&Icp02DecodedPayload{IndoorTempC: &temp}, ts)

	a.mu.RLock()
	defer a.mu.RUnlock()
	indoor := a.devices["44550065"]
	require.NotNil(t, indoor)
	require.NotNil(t, indoor.State.IndoorTempC)
	assert.Equal(t, 24.5, *indoor.State.IndoorTempC, "실내기 자체 보고는 실내기 상태를 갱신해야 한다")
}

// controller broadcast(ffffffff) status 는 전역으로 controller 를 갱신한다.
func TestUpdateDeviceState_ControllerBroadcast_UpdatesController(t *testing.T) {
	a := newControllerMergeTestAgent(t)
	ts := time.Now()

	a.updateDeviceState("44550000", "ffffffff", "0204", "",
		&Icp02DecodedPayload{OutdoorActive: boolPtr(true)}, ts)

	a.mu.RLock()
	defer a.mu.RUnlock()
	ctrl := a.devices["44550000"]
	require.NotNil(t, ctrl)
	require.NotNil(t, ctrl.State.OutdoorActive, "broadcast 전역 프레임은 controller 를 갱신해야 한다")
	assert.True(t, *ctrl.State.OutdoorActive)
}

// op_mode 0x4 는 가동 중(실외 active) 상태로, "cooling" 으로 매핑되어야 한다(잠정).
// 기존엔 미매핑이라 "unknown" 으로 새어나갔다.
func TestDecodeOpMode_0x4_Cooling(t *testing.T) {
	assert.Equal(t, "cooling", decodeOpMode(0x4),
		"op_mode 0x4 는 가동 중(cooling)으로 매핑되어야 한다")
}
