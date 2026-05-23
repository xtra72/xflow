package century

import (
	"testing"
	"time"
)

// TestAgent_DeviceStateGatedByReg02 는 v0.4.1 의 회귀 테스트이다.
//
// 사용자 보고: AC 가 켜진 상태인데 첫 device_state emit 이 mode="off",
// power=false, fan_speed=0, target_temp=0 으로 표시되어 "AC 가 꺼진 것처럼"
// 보임. 원인은 Reg04 가 먼저 도착하면 Reg02 미수신 상태로 fallback 값 (0/off)
// 으로 emit 되는 것이었음.
//
// v0.4.1 수정: maybeEmitDeviceState 가 Reg02 미수신이면 emit skip.
//
// 본 테스트는 Reg04 frame 만 먼저 주입했을 때 device_state emit 이 0개임을
// 확인하고, 그 후 Reg02 frame 을 주입하면 첫 emit 이 정상 power/mode 로
// 나타나는지 검증한다.
func TestAgent_DeviceStateGatedByReg02(t *testing.T) {
	t.Parallel()

	// Step 1: Reg04 만 먼저 주입.
	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"keepalive_interval": "10m", // keepalive 가 테스트 도중 fire 하지 않도록.
	}, mustBuildReg04ResponseFrame(t, 0x3B))
	defer cleanup()

	// Reg04 frame 이 처리될 때까지 잠깐 대기.
	waitUntil(t, 500*time.Millisecond, func() bool {
		return a.cStats.framesCaptured.Load() >= 1
	}, "reg04 frame not captured")

	// device_state emit 은 0개여야 한다 (Reg02 미수신으로 gate).
	msgs := drainMsgCh(t, a, 200*time.Millisecond)
	deviceStateCount := 0
	for _, m := range msgs {
		// v0.9.0: device_state 는 type 필드가 없음 — dev_id 존재 로 식별.
		if _, hasUnitID := m["unit_id"]; hasUnitID {
			deviceStateCount++
		}
	}
	if deviceStateCount != 0 {
		t.Errorf("v0.4.1: device_state emitted %d times before Reg02 (want 0). msgs=%v",
			deviceStateCount, msgs)
	}

	// Step 2: 이제 Reg02 frame 주입 → 첫 device_state emit 이 발생해야 한다.
	rt.deliver(mustBuildReg02ResponseFrame(t, 0x3B))

	more := waitForMsgCount(t, a, 1, 2*time.Second)
	// device_state 만 골라낸다.
	var deviceStateMsgs []map[string]any
	for _, m := range more {
		// v0.18.6: device_state 의 프로토콜 식별자는 unit_id (이전 device_id).
		if _, hasUnitID := m["unit_id"]; hasUnitID {
			deviceStateMsgs = append(deviceStateMsgs, m)
		}
	}
	if len(deviceStateMsgs) < 1 {
		t.Fatalf("v0.4.1: no device_state emit observed after Reg02 frame; got %d total msgs", len(more))
	}

	first := deviceStateMsgs[0]
	if got, _ := first["trigger"].(string); got != TriggerChange {
		t.Errorf("first.trigger = %q, want change", got)
	}
	st := deviceStateGroup(first)
	// 첫 emit 은 Reg02 의 정상 state 를 반영해야 한다 (fallback 0/off 가 아님).
	if got, _ := st["mode"].(float64); got != 1 {
		t.Errorf("first.state.mode = %v, want 1 (Reg02 정상값)", got)
	}
	if got, _ := st["power"].(bool); !got {
		t.Errorf("first.state.power = false, want true (mode=cool)")
	}
	if got, _ := st["fan_speed"].(float64); got != 1 {
		t.Errorf("first.state.fan_speed = %v, want 1 (hvac.FanAuto)", got)
	}
	if got, _ := st["target_temperature"].(float64); got != 25.0 {
		t.Errorf("first.state.target_temp = %v, want 25.0", got)
	}
	// current_temp 는 Reg04 가 이미 도착했으므로 25.2 여야 한다.
	if got, _ := st["current_temperature"].(float64); got != 25.2 {
		t.Errorf("first.state.current_temp = %v, want 25.2 (Reg04 이미 수신)", got)
	}

	// AC-B9 invariant.
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0", rt.WriteCount())
	}
}

// TestAgent_DeviceStateGatedByReg04 는 v0.4.2 의 대칭 시나리오 회귀 테스트이다.
//
// 사용자 보고 v0.4.1 후속: Reg02 만 먼저 도착하면 첫 emit 이 current_temp=0
// (Reg04 미수신 fallback) 으로 나타나 그 직후 정상값으로 정정되는 결함.
//
// v0.4.2 수정으로 Reg02 도 단독으로는 emit 안 됨; Reg04 도 함께 와야 첫 emit.
func TestAgent_DeviceStateGatedByReg04(t *testing.T) {
	t.Parallel()

	// Step 1: Reg02 만 먼저 주입.
	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"keepalive_interval": "10m",
	}, mustBuildReg02ResponseFrame(t, 0x3B))
	defer cleanup()

	waitUntil(t, 500*time.Millisecond, func() bool {
		return a.cStats.framesCaptured.Load() >= 1
	}, "reg02 frame not captured")

	// device_state emit 은 0개여야 한다 (Reg04 미수신으로 gate).
	msgs := drainMsgCh(t, a, 200*time.Millisecond)
	deviceStateCount := 0
	for _, m := range msgs {
		// v0.9.0: device_state 는 type 필드가 없음 — dev_id 존재 로 식별.
		if _, hasUnitID := m["unit_id"]; hasUnitID {
			deviceStateCount++
		}
	}
	if deviceStateCount != 0 {
		t.Errorf("v0.4.2: device_state emitted %d times before Reg04 (want 0). msgs=%v",
			deviceStateCount, msgs)
	}

	// Step 2: Reg04 주입 → 첫 device_state emit 발생.
	rt.deliver(mustBuildReg04ResponseFrame(t, 0x3B))

	more := waitForMsgCount(t, a, 1, 2*time.Second)
	var deviceStateMsgs []map[string]any
	for _, m := range more {
		// v0.18.6: device_state 의 프로토콜 식별자는 unit_id (이전 device_id).
		if _, hasUnitID := m["unit_id"]; hasUnitID {
			deviceStateMsgs = append(deviceStateMsgs, m)
		}
	}
	if len(deviceStateMsgs) < 1 {
		t.Fatalf("v0.4.2: no device_state emit observed after Reg04; got %d total msgs", len(more))
	}

	first := deviceStateMsgs[0]
	st := deviceStateGroup(first)
	// 첫 emit 부터 5 핵심 모두 정상값 (Reg02+Reg04 통합 결과).
	if got, _ := st["mode"].(float64); got != 1 {
		t.Errorf("first.state.mode = %v, want 1 (cool)", got)
	}
	if got, _ := st["fan_speed"].(float64); got != 1 {
		t.Errorf("first.state.fan_speed = %v, want 1 (hvac.FanAuto)", got)
	}
	if got, _ := st["target_temperature"].(float64); got != 25.0 {
		t.Errorf("first.state.target_temp = %v, want 25.0", got)
	}
	// 핵심 invariant: Reg04 fallback 0 이 아닌 25.2 가 첫 emit 부터 나와야 한다.
	if got, _ := st["current_temperature"].(float64); got != 25.2 {
		t.Errorf("v0.4.2 regression: first.state.current_temp = %v, want 25.2 (Reg04 fallback 0 노출 금지)", got)
	}

	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0", rt.WriteCount())
	}
}
