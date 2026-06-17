// lg_hvacr02_first_obs_test.go — 최초 관측은 change 가 아니라 report 로 발행됨을 검증.
//
// 배경: 디바이스를 처음 알게 된 순간은 비교할 이전 상태가 없으므로 "변경"이 아니다.
// 이를 change 로 내보내면 에이전트 재생성/재시작 때마다 첫 프레임이 거짓 change(power:false)
// 로 새어나가 change 시리즈를 오염시킨다. 최초 관측은 report(초기 상태)로 발행한다.
package lg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUpdateDeviceState_FirstObservation_IsReportNotChange(t *testing.T) {
	a := newControllerMergeTestAgent(t)
	ts := time.Now()

	// 최초 관측 (전원 OFF) → report 여야 한다.
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("OFF")}, ts)
	trig, st := lastEmittedState(t, a)
	assert.Equal(t, "report", trig, "최초 관측은 change 가 아니라 report 여야 한다")
	assert.Equal(t, false, st["power"])

	// 이후 실제 변경 (OFF→ON) → change(델타) 여야 한다.
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("ON"), IndoorTempC: f64Ptr(24.0)}, ts.Add(time.Second))
	trig2, st2 := lastEmittedState(t, a)
	assert.Equal(t, "change", trig2, "최초 관측 이후의 실제 변경은 change 여야 한다")
	assert.Equal(t, true, st2["power"])
}

// 디바이스 상태 초기화(에이전트 재생성 모사) 후 첫 프레임이 다시 report 로 나가
// change 시리즈가 오염되지 않음을 검증.
func TestUpdateDeviceState_AfterStateReset_FirstIsReport(t *testing.T) {
	a := newControllerMergeTestAgent(t)
	ts := time.Now()

	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("OFF")}, ts)
	trig1, _ := lastEmittedState(t, a)
	assert.Equal(t, "report", trig1)

	// 에이전트 재생성 모사: 디바이스 맵 + lastStates 초기화.
	a.mu.Lock()
	a.devices = map[string]*Icp02Device{}
	a.lastStates = map[string]Icp02DeviceState{}
	a.mu.Unlock()

	// 재초기화 후 첫 관측 (다시 OFF) → change 가 아니라 report.
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("OFF")}, ts.Add(time.Second))
	trig2, _ := lastEmittedState(t, a)
	assert.Equal(t, "report", trig2,
		"상태 초기화 후 첫 관측도 change 가 아니라 report 여야 한다(거짓 change 방지)")
}
