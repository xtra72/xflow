// lg_hvacr02_emit_dedup_test.go — 발행 단일 길목(emitDeviceStatePayloadLocked)의
// 변경-시에만-발행 dedup 검증. report(주기)·중복 루프라도 같은 값은 한 번만 나간다.
package lg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 주기 report 는 heartbeat 이므로 변경 여부와 무관하게 항상 발행되어야 한다.
func TestEmitDedup_PeriodicReport_AlwaysEmits(t *testing.T) {
	a := newControllerMergeTestAgent(t)
	ts := time.Now()

	// 최초 관측 (OFF) → 1건 발행.
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("OFF")}, ts)
	a.mu.RLock()
	after1 := a.recentIdx
	a.mu.RUnlock()
	require.Equal(t, 1, after1)

	dev := a.devices["44550066"]

	// 같은 상태라도 주기 report 는 매번 발행된다(heartbeat).
	a.mu.Lock()
	a.emitDeviceStateLocked(dev, "report")
	a.emitDeviceStateLocked(dev, "report")
	a.emitDeviceStateLocked(dev, "report")
	a.mu.Unlock()
	a.mu.RLock()
	afterReports := a.recentIdx
	a.mu.RUnlock()
	assert.Equal(t, 4, afterReports, "주기 report 는 변경 없어도 매번 발행되어야 한다")
}

// change 는 변경 시에만 발행된다(동일 상태 프레임은 미발행).
func TestEmitDedup_Change_OnlyOnChange(t *testing.T) {
	a := newControllerMergeTestAgent(t)
	ts := time.Now()

	// 최초 관측 (OFF) → report 1건.
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("OFF")}, ts)
	a.mu.RLock()
	after1 := a.recentIdx
	a.mu.RUnlock()
	require.Equal(t, 1, after1)

	// 동일 상태(OFF) 프레임 → 변경 없음 → 미발행.
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("OFF")}, ts.Add(time.Second))
	a.mu.RLock()
	afterSame := a.recentIdx
	a.mu.RUnlock()
	assert.Equal(t, 1, afterSame, "동일 상태 프레임은 change 를 발행하지 않아야 한다")

	// 실제 변경(OFF→ON) → change 1건.
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("ON"), IndoorTempC: f64Ptr(24.0)}, ts.Add(2*time.Second))
	a.mu.RLock()
	afterChange := a.recentIdx
	a.mu.RUnlock()
	assert.Equal(t, 2, afterChange, "실제 변경은 change 로 발행되어야 한다")
}

// "response"(노드의 inactivity request_state 내부 응답)도 dedup 대상이다.
// 무수신 중에는 상태가 안 바뀌므로 항상 직전 발행과 동일 → 플로우로 전달되지 않는다.
func TestEmitDedup_Response_DedupedWhenUnchanged(t *testing.T) {
	a := newControllerMergeTestAgent(t)
	ts := time.Now()
	a.updateDeviceState("44550066", "44550000", "0204", "",
		&Icp02DecodedPayload{PowerState: strPtr("OFF")}, ts)
	dev := a.devices["44550066"]

	a.mu.RLock()
	before := a.recentIdx
	a.mu.RUnlock()

	// 동일 상태의 response 는 dedup 되어 다운스트림으로 흐르지 않는다.
	a.mu.Lock()
	a.emitDeviceStateLocked(dev, "response")
	a.emitDeviceStateLocked(dev, "response")
	a.mu.Unlock()
	a.mu.RLock()
	after := a.recentIdx
	a.mu.RUnlock()
	assert.Equal(t, before, after, "변경 없는 response 는 dedup 되어 발행되지 않아야 한다")
}
