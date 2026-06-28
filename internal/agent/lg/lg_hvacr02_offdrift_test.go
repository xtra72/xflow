// lg_hvacr02_offdrift_test.go — 전원 OFF 상태에서 센서값만 흔들릴 때 device_state.change
// 가 새어나가던 회귀 방지.
//
// 증상(2026-06-16): 에이전트가 type="device_state.change" 메시지를 보냈으나 payload 는
// {power:false} 로 직전과 동일 — 실제 관측 가능한 변화가 없음. 원인: stateChanged 는
// 내부 누적 상태(배관/실내 온도 등 센서 raw 필드)의 변화를 감지하지만, 전원 OFF 시
// 그 필드들은 toProperties 투영에서 제외되어 payload 는 {power:false} 로 고정된다.
// fix: emit 게이트를 투영(toProperties) 동일 여부로 판단.
package lg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func strPtr(s string) *string   { return &s }
func f64Ptr(f float64) *float64 { return &f }

// OFF 상태에서 OFF 투영에 노출되지 않는 필드(target_temperature 등)만 변하면
// device_state.change 가 emit 되지 않아야 한다. (투영 {power:false} 동일 → 관측 변화 없음)
//
// SetTempC 는 stateChanged 가 감지하지만 OFF 투영(indoorProperties powerOn=false)에는
// 포함되지 않으며, nonTempField 라 EventTempThreshold 게이트도 통과한다. 따라서 fix 가
// 없으면 반드시 누수(emit)되는 견고한 재현 케이스다.
func TestUpdateDeviceState_PowerOff_HiddenFieldDrift_NoEmit(t *testing.T) {
	a := newControllerMergeTestAgent(t)
	ts := time.Now()

	// 프레임 1: 실내기(44550066) → controller 보고. 전원 OFF + 목표온도 24.0.
	// 최초 관측 → power:false 가 투영에 추가되므로 1건 emit.
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("OFF"), SetTempC: f64Ptr(24.0)}, ts)

	a.mu.RLock()
	afterFirst := a.recentIdx
	a.mu.RUnlock()
	assert.Equal(t, 1, afterFirst, "최초 OFF 관측은 1건 emit 되어야 한다")

	// 프레임 2: 전원 OFF 동일, 목표온도만 24.0→26.0 변경. 내부 상태는 변하지만
	// OFF 투영은 {power:false} 로 동일 → emit 되면 안 된다.
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("OFF"), SetTempC: f64Ptr(26.0)}, ts.Add(time.Second))

	a.mu.RLock()
	afterSecond := a.recentIdx
	a.mu.RUnlock()
	assert.Equal(t, 1, afterSecond,
		"OFF 상태에서 비투영 필드만 흔들리면 device_state.change 가 새어나가면 안 된다")
}

// 전원 OFF→ON 전이는 투영이 바뀌므로 정상적으로 emit 되어야 한다(과억제 방지).
func TestUpdateDeviceState_PowerOffToOn_Emits(t *testing.T) {
	a := newControllerMergeTestAgent(t)
	ts := time.Now()

	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("OFF")}, ts)
	a.mu.RLock()
	afterOff := a.recentIdx
	a.mu.RUnlock()
	assert.Equal(t, 1, afterOff)

	// 전원 ON 전이 → 투영에 power:true 등장 → emit.
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("ON"), IndoorTempC: f64Ptr(24.0)}, ts.Add(time.Second))
	a.mu.RLock()
	afterOn := a.recentIdx
	a.mu.RUnlock()
	assert.Equal(t, 2, afterOn, "전원 OFF→ON 전이는 emit 되어야 한다")
}
