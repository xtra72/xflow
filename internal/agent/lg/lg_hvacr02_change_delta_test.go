// lg_hvacr02_change_delta_test.go — device_state.change 는 변경된 필드만,
// report 는 전체 상태를 전송하는지 검증.
//
// 배경: change 메시지가 변경되지 않은 필드까지 전체 전송하여 다운스트림에 중복
// 데이터가 누적되던 문제. report=전체 / change=델타 로 분리한다.
package lg

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lastEmittedState 는 링 버퍼의 가장 최근 device_state 이벤트에서 state 맵을 디코드한다.
func lastEmittedState(t *testing.T, a *Hvacr02Agent) (string, map[string]any) {
	t.Helper()
	a.recentMu.RLock()
	defer a.recentMu.RUnlock()
	idx := (a.recentIdx - 1 + hvacr02RecentBufferSize) % hvacr02RecentBufferSize
	raw := a.recentFrames[idx].Event
	require.NotEmpty(t, raw, "emit 된 프레임이 있어야 한다")
	var payload struct {
		Trigger string         `json:"trigger"`
		State   map[string]any `json:"state"`
	}
	require.NoError(t, json.Unmarshal(raw, &payload))
	return payload.Trigger, payload.State
}

// change 메시지는 직전 대비 변경된 필드만 담아야 한다.
func TestUpdateDeviceState_Change_OnlyChangedFields(t *testing.T) {
	a := newControllerMergeTestAgent(t)
	ts := time.Now()

	// 프레임 1: 가동(ON) 전체 상태 — 최초 관측.
	a.updateDeviceState("44550066", "44550000", "0204", "", &Icp02DecodedPayload{
		PowerState:  strPtr("ON"),
		IndoorTempC: f64Ptr(24.0),
		SetTempC:    f64Ptr(26.0),
		Mode:        strPtr("cool"),
		FanSpeed:    strPtr("high"),
	}, ts)

	// 프레임 2: 실내 온도만 24.0 → 25.0 변경. change 는 current_temperature 만 담아야 한다.
	a.updateDeviceState("44550066", "44550000", "0204", "", &Icp02DecodedPayload{
		PowerState:  strPtr("ON"),
		IndoorTempC: f64Ptr(25.0),
		SetTempC:    f64Ptr(26.0),
		Mode:        strPtr("cool"),
		FanSpeed:    strPtr("high"),
	}, ts.Add(time.Second))

	trigger, state := lastEmittedState(t, a)
	assert.Equal(t, "change", trigger)
	assert.Equal(t, map[string]any{"current_temperature": 25.0}, state,
		"change 메시지는 변경된 필드(current_temperature)만 담아야 한다")
}

// report 메시지는 전체 상태를 담아야 한다(부분 전송 아님).
func TestEmitDeviceState_Report_FullState(t *testing.T) {
	a := newControllerMergeTestAgent(t)
	ts := time.Now()

	a.updateDeviceState("44550066", "44550000", "0204", "", &Icp02DecodedPayload{
		PowerState:  strPtr("ON"),
		IndoorTempC: f64Ptr(24.0),
		SetTempC:    f64Ptr(26.0),
		Mode:        strPtr("cool"),
		FanSpeed:    strPtr("high"),
	}, ts)

	// 해당 실내기에 대해 report 트리거로 전체 상태 emit.
	a.mu.Lock()
	a.emitDeviceStateLocked(a.devices["44550066"], "report")
	a.mu.Unlock()

	trigger, state := lastEmittedState(t, a)
	assert.Equal(t, "report", trigger)
	// 전체 상태: power + 운전 필드 모두 포함.
	assert.Contains(t, state, "power")
	assert.Contains(t, state, "current_temperature")
	assert.Contains(t, state, "target_temperature")
	assert.Contains(t, state, "mode")
	assert.Contains(t, state, "fan_speed")
}
