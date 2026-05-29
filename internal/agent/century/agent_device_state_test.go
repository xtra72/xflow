package century

// Group H acceptance scenarios — v0.3.0 (M7) device-centric output (REQ-CENTURY-033/034/035).
//
// These tests cover the breaking change in v0.3.0 where msgCh's default emit is the
// device-centric DeviceStateEvent (snake_case JSON, epoch ms) instead of the v0.2.x
// register-decoded messages. The change detector compares the 5 core fields
// (power / mode / fan / set_temp_c / current_temp_c) plus online state transitions,
// and the keepalive ticker emits trigger="keepalive" when KeepaliveInterval elapses
// without change.
//
// See acceptance.md §"그룹 H: Device-centric output (v0.3.0)" for the gherkin specs.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// drainMsgCh collects all currently-available payloads from a.msgCh until the
// deadline elapses. It does not subscribe via ReceiveMessage, since msgCh emit
// is bridgeActive-gated; we set bridgeActive=true manually via subscribeOnce.
func drainMsgCh(t *testing.T, a *Hvacr01Agent, deadline time.Duration) []map[string]any {
	t.Helper()
	a.bridgeActive.Store(true)
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	var out []map[string]any
	for {
		select {
		case data := <-a.msgCh:
			var m map[string]any
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("unmarshal msgCh payload: %v (raw=%s)", err, data)
			}
			out = append(out, m)
		case <-ctx.Done():
			// Drain anything left non-blocking.
			for {
				select {
				case data := <-a.msgCh:
					var m map[string]any
					if err := json.Unmarshal(data, &m); err == nil {
						out = append(out, m)
					}
				default:
					return out
				}
			}
		}
	}
}

// waitForMsgCount polls until the cumulative collected list reaches `want` or
// deadline elapses. Returns the collected payloads.
func waitForMsgCount(t *testing.T, a *Hvacr01Agent, want int, deadline time.Duration) []map[string]any {
	t.Helper()
	a.bridgeActive.Store(true)
	end := time.Now().Add(deadline)
	var out []map[string]any
	for time.Now().Before(end) {
		select {
		case data := <-a.msgCh:
			var m map[string]any
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("unmarshal msgCh payload: %v (raw=%s)", err, data)
			}
			out = append(out, m)
			if len(out) >= want {
				return out
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
	return out
}

// deviceStateGroup 은 v0.4.0 의 nested "state" 그룹을 추출하는 헬퍼이다.
// device_state 이벤트에서 5 핵심 필드 + online 은 모두 m["state"] 아래로 이동됨.
func deviceStateGroup(m map[string]any) map[string]any {
	if s, ok := m["state"].(map[string]any); ok {
		return s
	}
	return map[string]any{}
}

// AC-H1: emit_device_state=true (default) + emit_register_decoded=false (default).
// All observed msgCh payloads must have type="device_state"; none should be
// register-decoded message types.
func TestAgent_AC_H1_RegisterDecodedOptOutByDefault(t *testing.T) {
	t.Parallel()
	// Build a small batch: one frame of each register type to exercise the
	// dispatcher. The default config should yield ONLY device_state events.
	batch := append([]byte{}, mustBuildReg02ResponseFrame(t, 0x3B)...)
	batch = append(batch, mustBuildReg04ResponseFrame(t, 0x3B)...)
	batch = append(batch, mustBuildReg04WriteFrame(t, 0x3B, 0x02, 0x10)...)
	batch = append(batch, mustBuildAckFrame(t)...)

	a, rt, cleanup := makeTestAgent(t, nil, batch)
	defer cleanup()

	// Wait for all 4 frames to be processed (ringBuffer push proves processing).
	waitUntil(t, 2*time.Second, func() bool {
		return a.cStats.framesCaptured.Load() >= 4
	}, "frames not all captured")

	// Drain everything that hit msgCh in a short window.
	msgs := drainMsgCh(t, a, 200*time.Millisecond)

	// Every message must be device_state (no register-decoded leaked through).
	// v0.9.0: device_state 는 type 필드가 없음 (metadata.message_type 에 인코딩).
	// register-decoded 만 "type": "century_reg02_response" 등을 보유. 따라서
	// type 필드가 있으면서 century_ prefix 이면 leak 으로 판정.
	for _, m := range msgs {
		tp, _ := m["type"].(string)
		if tp == "" {
			// device_state (no type field — v0.9.0 schema)
			// v0.18.6: device_state 의 디바이스 식별자는 unit_id (글로벌 UUID 는 device_id).
			if _, hasUnitID := m["unit_id"]; !hasUnitID {
				t.Errorf("AC-H1: untyped message without unit_id (likely register-decoded leak): %v", m)
			}
			continue
		}
		switch tp {
		case "century_reg02_response",
			"century_reg03_response",
			"century_reg04_response",
			"century_reg04_write_request",
			"century_ack":
			t.Errorf("AC-H1: register-decoded message leaked through default config: %s", tp)
		default:
			t.Errorf("AC-H1: unexpected typed message: type=%q msg=%v", tp, m)
		}
	}
	// At minimum the reg02 frame should have produced one device_state emit.
	if len(msgs) < 1 {
		t.Fatalf("AC-H1: no device_state emit observed; got %d msgs", len(msgs))
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0 (AC-B9)", rt.WriteCount())
	}
}

// AC-H2: Reg02 + Reg04 모두 수신 후 첫 emit (v0.4.2 갱신).
// v0.4.2 부터 5 핵심 필드 의 모든 원천 register 가 관측된 후에만 emit 한다 —
// 초기값 fallback (0/off) 노출을 방지하기 위한 정책. 따라서 첫 emit 은
// trigger="change" + 5 핵심 모두 정상값으로 나타난다.
func TestAgent_AC_H2_FirstEmitAfterReg02AndReg04(t *testing.T) {
	t.Parallel()
	// v0.4.2: Reg02 + Reg04 모두 주입해야 첫 emit 발생.
	batch := append([]byte{}, mustBuildReg02ResponseFrame(t, 0x3B)...)
	batch = append(batch, mustBuildReg04ResponseFrame(t, 0x3B)...)
	a, rt, cleanup := makeTestAgent(t, nil, batch)
	defer cleanup()

	msgs := waitForMsgCount(t, a, 1, 2*time.Second)
	if len(msgs) < 1 {
		t.Fatalf("no device_state emit observed")
	}
	m := msgs[0]
	// v0.9.0: device_state 는 type 필드가 없음 — dev_id 존재 + type 부재 로 식별.
	if _, hasType := m["type"]; hasType {
		t.Errorf("v0.9.0: device_state 는 type 필드가 없어야 함: %v", m)
	}
	// v0.18.6: device_state 의 프로토콜 식별자는 unit_id (이전 device_id).
	if got, _ := m["unit_id"].(string); got != "0x3B" {
		t.Errorf("unit_id = %q, want 0x3B", got)
	}
	// v0.5.0: label 은 metadata 그룹 안으로 이동.
	if meta, ok := m["metadata"].(map[string]any); !ok {
		t.Errorf("metadata group missing in %v", m)
	} else if got, _ := meta["label"].(string); got != "indoor-3b" {
		t.Errorf("metadata.label = %q, want indoor-3b", got)
	}
	if _, exists := m["label"]; exists {
		t.Errorf("v0.5.0: label must NOT be at top-level (moved to metadata.label)")
	}
	// v0.4.0: 5 핵심 + online 은 nested "state" 그룹으로 이동.
	st := deviceStateGroup(m)
	if got, _ := st["online"].(bool); !got {
		t.Errorf("state.online = false, want true")
	}
	if got, _ := st["power"].(bool); !got {
		t.Errorf("state.power = false, want true (mode=cooling)")
	}
	if got, _ := st["mode"].(float64); got != 1 {
		t.Errorf("state.mode = %v, want 1 (hvac.ModeCool)", got)
	}
	if got, _ := st["fan_speed"].(float64); got != 1 {
		t.Errorf("state.fan_speed = %v, want 1 (hvac.FanAuto)", got)
	}
	if got, _ := st["target_temperature"].(float64); got != 25.0 {
		t.Errorf("state.target_temp = %v, want 25.0", got)
	}
	// v0.4.2: Reg04 도 수신했으므로 current_temp 가 정상값으로 나와야 한다.
	if got, _ := st["current_temperature"].(float64); got != 25.2 {
		t.Errorf("state.current_temp = %v, want 25.2 (Reg04 정상값)", got)
	}
	// v0.3.1: evap 필드는 device state schema 에서 제거됨 (register-decoded 로 이동).
	if _, exists := st["evap_temp_a_c"]; exists {
		t.Errorf("evap_temp_a_c must not be present in state group (moved to register-decoded)")
	}
	if _, exists := st["evap_temp_b_c"]; exists {
		t.Errorf("evap_temp_b_c must not be present in state group")
	}
	if got, _ := m["trigger"].(string); got != TriggerChange {
		t.Errorf("trigger = %q, want change (first emit)", got)
	}
	// v0.5.0: timestamp_ms 제거, last_seen_ms 단일 timestamp.
	if got, ok := m["last_seen_ms"].(float64); !ok || got < 1_000_000_000_000 {
		t.Errorf("last_seen_ms = %v ok=%v, want epoch ms > 1e12", got, ok)
	}
	if _, exists := m["timestamp_ms"]; exists {
		t.Errorf("v0.5.0: timestamp_ms must NOT be present (removed)")
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0 (AC-B9)", rt.WriteCount())
	}
}

// AC-H3 (v0.4.2 갱신): reg 0x02 + reg 0x04 read → 단일 통합 emit 발생.
//
// v0.4.1 까지는 Reg02 만 봐도 emit 후 Reg04 후 두 번째 emit 이었으나,
// v0.4.2 의 Reg02+Reg04 gate 정책으로 둘 다 도착 후 1회 emit 으로 변경.
// 첫 emit 부터 5 핵심 모두 정상값 (mode=cool/fan=17/target=25/current=25.2).
func TestAgent_AC_H3_Reg04UpdatesCurrentTemp(t *testing.T) {
	t.Parallel()
	batch := append([]byte{}, mustBuildReg02ResponseFrame(t, 0x3B)...)
	batch = append(batch, mustBuildReg04ResponseFrame(t, 0x3B)...)

	a, rt, cleanup := makeTestAgent(t, nil, batch)
	defer cleanup()

	msgs := waitForMsgCount(t, a, 1, 2*time.Second)
	if len(msgs) < 1 {
		t.Fatalf("want >=1 device_state emit (Reg02+Reg04 통합), got %d", len(msgs))
	}
	first := msgs[0]
	firstSt := deviceStateGroup(first)
	// 5 핵심 모두 정상값.
	if got, _ := firstSt["current_temperature"].(float64); got != 25.2 {
		t.Errorf("state.current_temp = %v, want 25.2 (Reg04 정상값)", got)
	}
	if got, _ := firstSt["mode"].(float64); got != 1 {
		t.Errorf("state.mode = %v, want 1 (cool)", got)
	}
	if got, _ := firstSt["fan_speed"].(float64); got != 1 {
		t.Errorf("state.fan_speed = %v, want 1 (hvac.FanAuto)", got)
	}
	if got, _ := firstSt["target_temperature"].(float64); got != 25.0 {
		t.Errorf("state.target_temp = %v, want 25.0", got)
	}
	if got, _ := first["trigger"].(string); got != TriggerChange {
		t.Errorf("first.trigger = %q, want change", got)
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0 (AC-B9)", rt.WriteCount())
	}
}

// AC-H4: same frame three times → emit count must stay at 1 (5 core unchanged).
//
// v0.4.2: Reg04 frame 도 함께 주입해야 첫 emit 이 발생 (Reg02+Reg04 gate).
func TestAgent_AC_H4_UnchangedFramesDoNotReemit(t *testing.T) {
	t.Parallel()
	// Three identical reg02 frames + 1 reg04 (v0.4.2 gate 통과용).
	frame := mustBuildReg02ResponseFrame(t, 0x3B)
	batch := append([]byte{}, frame...)
	batch = append(batch, mustBuildReg04ResponseFrame(t, 0x3B)...)
	batch = append(batch, frame...)
	batch = append(batch, frame...)

	// Use a very long keepalive_interval so the test window doesn't trigger one.
	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"report_interval": "10m",
	}, batch)
	defer cleanup()

	waitUntil(t, 2*time.Second, func() bool {
		return a.cStats.framesCaptured.Load() >= 4
	}, "frames not all captured")

	msgs := drainMsgCh(t, a, 300*time.Millisecond)
	if len(msgs) != 1 {
		t.Errorf("AC-H4: got %d emits, want exactly 1 (unchanged value should not re-emit)", len(msgs))
		for i, m := range msgs {
			t.Logf("  msg[%d] = %v", i, m)
		}
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0", rt.WriteCount())
	}
}

// mustBuildReg02ResponseFrameMode is a test helper that overrides the mode byte
// in a CAP-3-style reg 0x02 response frame (default mode = 0x01 cooling).
func mustBuildReg02ResponseFrameMode(t *testing.T, subDevID, mode byte) []byte {
	t.Helper()
	// Prefix: sub_dev_id, 0x00, register=0x02. Data (17B) — mode is data[1].
	payload := []byte{
		subDevID, 0x00, 0x02,
		0x00, mode, 0x11, 0x00, 0x00, 0x00, 0x00, 0xFA, 0x00,
		0x00, 0x00, 0xFA, 0x00, 0x1B, 0x39, 0x39, 0x00,
	}
	return buildFrame(AddrSlave, AddrMaster, FCResponse, payload)
}

// AC-H5: mode 0x00 → 0x01 transition emits power false → true change.
//
// v0.4.2: Reg04 도 한 번 주입해야 emit gate 가 통과한다.
func TestAgent_AC_H5_ModeTransitionEmitsPowerChange(t *testing.T) {
	t.Parallel()
	offFrame := mustBuildReg02ResponseFrameMode(t, 0x3B, 0x00)
	onFrame := mustBuildReg02ResponseFrameMode(t, 0x3B, 0x01)
	// v0.4.2: Reg04 가 있어야 emit 됨. mode 변경 두 번 만으로는 첫 emit 도 안 발생.
	batch := append([]byte{}, offFrame...)
	batch = append(batch, mustBuildReg04ResponseFrame(t, 0x3B)...)
	batch = append(batch, onFrame...)

	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"report_interval": "10m",
	}, batch)
	defer cleanup()

	msgs := waitForMsgCount(t, a, 2, 2*time.Second)
	if len(msgs) < 2 {
		t.Fatalf("want >=2 emits (off + on transition), got %d: %v", len(msgs), msgs)
	}
	first, second := msgs[0], msgs[1]
	firstSt := deviceStateGroup(first)
	secondSt := deviceStateGroup(second)
	if got, _ := firstSt["power"].(bool); got {
		t.Errorf("first.state.power = true, want false (mode=off)")
	}
	if got, _ := firstSt["mode"].(float64); got != 0 {
		t.Errorf("first.state.mode = %v, want 0 (off)", got)
	}
	if got, _ := secondSt["power"].(bool); !got {
		t.Errorf("second.state.power = false, want true (mode=cooling)")
	}
	if got, _ := secondSt["mode"].(float64); got != 1 {
		t.Errorf("second.state.mode = %v, want 1 (cool)", got)
	}
	if got, _ := second["trigger"].(string); got != TriggerChange {
		t.Errorf("second.trigger = %q, want change", got)
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0", rt.WriteCount())
	}
}

// AC-H6: keepalive_interval=200ms — without further frames, a keepalive emit
// arrives after the interval. Uses real time with a generous deadline.
//
// v0.4.2: Reg02 + Reg04 모두 주입해야 emit gate 통과.
func TestAgent_AC_H6_KeepaliveAfterInterval(t *testing.T) {
	t.Parallel()
	// Reg02 + Reg04 (v0.4.2 gate), 그 후 더 이상 변경 없음 → keepalive 가 200ms 후 fire.
	batch := append([]byte{}, mustBuildReg02ResponseFrame(t, 0x3B)...)
	batch = append(batch, mustBuildReg04ResponseFrame(t, 0x3B)...)
	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"report_interval": "200ms",
	}, batch)
	defer cleanup()

	// Expect the initial change emit, then at least one keepalive within 2s budget.
	msgs := waitForMsgCount(t, a, 2, 2500*time.Millisecond)
	if len(msgs) < 2 {
		t.Fatalf("AC-H6: want >=2 emits (change + keepalive), got %d: %v", len(msgs), msgs)
	}
	first := msgs[0]
	if got, _ := first["trigger"].(string); got != TriggerChange {
		t.Errorf("first.trigger = %q, want change", got)
	}
	// At least one subsequent emit must be keepalive.
	sawKeepalive := false
	for _, m := range msgs[1:] {
		if got, _ := m["trigger"].(string); got == TriggerReport {
			sawKeepalive = true
			// Sanity: core fields preserved (state group).
			if got, _ := deviceStateGroup(m)["mode"].(float64); got != 1 {
				t.Errorf("keepalive emit dropped state.mode: got %v, want 1 (cool)", got)
			}
		}
	}
	if !sawKeepalive {
		t.Errorf("AC-H6: no keepalive emit observed in %d messages", len(msgs))
	}
	// Stats counter must show at least one keepalive.
	if got := a.cStats.reportEmits.Load(); got < 1 {
		t.Errorf("reportEmits = %d, want >= 1", got)
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0", rt.WriteCount())
	}
}

// AC-H7: offline transition triggers immediate device_state with online=false.
//
// v0.4.2: Reg02 + Reg04 모두 주입해야 첫 emit (gate) 발생.
func TestAgent_AC_H7_OfflineTransitionEmitsChange(t *testing.T) {
	t.Parallel()
	// Short offline_timeout, long keepalive (so keepalive doesn't interfere).
	batch := append([]byte{}, mustBuildReg02ResponseFrame(t, 0x3B)...)
	batch = append(batch, mustBuildReg04ResponseFrame(t, 0x3B)...)
	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"offline_timeout": "120ms",
		"report_interval": "10m",
	}, batch)
	defer cleanup()

	// Initial change emit on first reg02.
	msgs := waitForMsgCount(t, a, 1, 1*time.Second)
	if len(msgs) < 1 || deviceStateGroup(msgs[0])["online"] != true {
		t.Fatalf("AC-H7: initial state.online=true emit missing: %v", msgs)
	}
	// Wait for offline transition emit (state.online=false).
	more := waitForMsgCount(t, a, 1, 2*time.Second)
	if len(more) < 1 {
		t.Fatalf("AC-H7: no offline transition emit observed")
	}
	offEvt := more[len(more)-1]
	offSt := deviceStateGroup(offEvt)
	if online, _ := offSt["online"].(bool); online {
		t.Errorf("AC-H7: offline emit has state.online=true, want false: %v", offEvt)
	}
	if got, _ := offEvt["trigger"].(string); got != TriggerChange {
		t.Errorf("AC-H7: offline emit trigger = %q, want change", got)
	}
	// Stale snapshot — mode/fan should still reflect the last known state.
	if got, _ := offSt["mode"].(float64); got != 1 {
		t.Errorf("AC-H7: offline emit dropped stale state.mode: got %v, want 1 (cool)", got)
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0", rt.WriteCount())
	}
}

// AC-H8 (v0.5.1 갱신): register-decoded stream 폐기로 본 테스트 케이스 삭제.
// 이전 시나리오 ("emit_device_state=false + emit_register_decoded=true → only
// register-decoded") 는 v0.5.1 에서 의미가 사라졌다 (register-decoded 자체 emit 안 됨).

// AC-H9 (v0.5.1 갱신): emit_device_state=false → ErrHvacr01NoOutputEnabled.
// v0.5.1: register-decoded 옵션 제거. EmitDeviceState 가 유일한 emit stream 이므로
// false 로 설정 시 즉시 에러.
func TestAgent_AC_H9_EmitDeviceStateOffReturnsErrHvacr01NoOutputEnabled(t *testing.T) {
	t.Parallel()
	cfg := agent.AgentConfig{
		ID:   "century-no-output",
		Name: "century-no-output",
		Type: "century_hvacr01",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port":       "/dev/ttyTEST",
				"emit_device_state": false,
			},
		},
	}
	_, err := NewHvacr01Agent(cfg)
	if !errors.Is(err, ErrHvacr01NoOutputEnabled) {
		t.Fatalf("NewHvacr01Agent err = %v, want ErrHvacr01NoOutputEnabled", err)
	}
}

// AC-H10: multiple sub_device_ids — independent change detection per device.
// Initial frames for 0x3B and 0x3C each trigger a change emit. A second identical
// 0x3B frame must not re-emit. Sub_dev_id 0x3C must not be affected.
//
// v0.4.2: 두 디바이스 모두 Reg02 + Reg04 가 있어야 첫 emit 가 발생.
func TestAgent_AC_H10_MultiSubDevIDIndependent(t *testing.T) {
	t.Parallel()
	batch := append([]byte{}, mustBuildReg02ResponseFrame(t, 0x3B)...)
	batch = append(batch, mustBuildReg04ResponseFrame(t, 0x3B)...) // v0.4.2 gate (0x3B)
	batch = append(batch, mustBuildReg02ResponseFrame(t, 0x3C)...)
	batch = append(batch, mustBuildReg04ResponseFrame(t, 0x3C)...) // v0.4.2 gate (0x3C)
	batch = append(batch, mustBuildReg02ResponseFrame(t, 0x3B)...) // duplicate of 3B

	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"report_interval": "10m",
	}, batch)
	defer cleanup()

	waitUntil(t, 2*time.Second, func() bool {
		return len(a.ListDevices()) >= 2
	}, "two devices not discovered")
	waitUntil(t, 2*time.Second, func() bool {
		return a.cStats.framesCaptured.Load() >= 5
	}, "frames not all captured")

	msgs := drainMsgCh(t, a, 300*time.Millisecond)
	count3B, count3C := 0, 0
	for _, m := range msgs {
		// v0.18.6: device_state 의 프로토콜 식별자는 unit_id.
		switch m["unit_id"] {
		case "0x3B":
			count3B++
		case "0x3C":
			count3C++
		}
	}
	if count3B != 1 {
		t.Errorf("AC-H10: 0x3B emit count = %d, want exactly 1 (second frame is duplicate)", count3B)
	}
	if count3C != 1 {
		t.Errorf("AC-H10: 0x3C emit count = %d, want exactly 1", count3C)
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0", rt.WriteCount())
	}
}

// TestTransformDecodedPayload_Defaults 는 v0.3.3 transform 의 default 동작 검증:
//   - confirmed 필드 → "status" 그룹으로 value 평탄화
//   - inferred 필드 → include_inferred_fields=false 시 제외
//   - unknown 필드  → include_unknown_fields=false 시 제외
//   - 비-nested 필드 (register, sub_dev_id, ...) → top-level 보존
func TestTransformDecodedPayload_Defaults(t *testing.T) {
	t.Parallel()
	input := []byte(`{
		"register": 3,
		"device_id": 59,
		"temp_evap_a_c": {"status":"confirmed","value":26.5,"raw":265},
		"temp_evap_b_c": {"status":"confirmed","value":27.0,"raw":270},
		"reg03_pad_4": {"status":"unknown","value":0},
		"reg03_pad_12": {"status":"unknown","value":0},
		"op_val_1": {"status":"inferred","value":996},
		"timestamp_ms": 1779150443359
	}`)
	out, err := transformDecodedPayload(input, false, false, true)
	if err != nil {
		t.Fatalf("transformDecodedPayload: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	// status 그룹: confirmed 필드의 value 만 평탄화.
	state, ok := m["state"].(map[string]any)
	if !ok {
		t.Fatalf("state group missing or not object: %v", m["state"])
	}
	if state["temp_evap_a_c"] != 26.5 {
		t.Errorf("state.temp_evap_a_c = %v, want 26.5", state["temp_evap_a_c"])
	}
	if state["temp_evap_b_c"] != 27.0 {
		t.Errorf("status.temp_evap_b_c = %v, want 27.0", state["temp_evap_b_c"])
	}
	// inferred / unknown 그룹은 없어야 한다.
	if _, ok := m["inferred"]; ok {
		t.Errorf("inferred group must not appear when include_inferred_fields=false")
	}
	if _, ok := m["unknown"]; ok {
		t.Errorf("unknown group must not appear when include_unknown_fields=false")
	}
	// top-level 비-nested 필드는 보존.
	if m["register"] == nil || m["device_id"] == nil || m["timestamp_ms"] == nil {
		t.Errorf("top-level register/dev_id/timestamp_ms must be preserved: %v", m)
	}
	// 원본 nested 필드 (raw/status 메타 포함) 는 top-level 에 남아 있으면 안 됨.
	for _, k := range []string{"temp_evap_a_c", "temp_evap_b_c", "reg03_pad_4", "op_val_1"} {
		if _, exists := m[k]; exists {
			t.Errorf("%q must be moved into a group (not at top-level)", k)
		}
	}
}

// TestTransformDecodedPayload_IncludeInferred 는 include_inferred_fields=true 일 때
// inferred 필드들이 별도 "inferred" 그룹으로 출력됨을 검증한다.
func TestTransformDecodedPayload_IncludeInferred(t *testing.T) {
	t.Parallel()
	input := []byte(`{
		"register": 4,
		"temp_A_c": {"status":"inferred","value":25.2,"raw":252},
		"op_val_1": {"status":"inferred","value":996},
		"status_bits": {"status":"inferred","value":54},
		"mode": {"status":"confirmed","value":"cool","raw":1}
	}`)
	out, err := transformDecodedPayload(input, true, false, true)
	if err != nil {
		t.Fatalf("transformDecodedPayload: %v", err)
	}
	var m map[string]any
	json.Unmarshal(out, &m)
	state := m["state"].(map[string]any)
	// SPEC-DEVICE-IDENTITY-001 후속: mode 는 hvac 통일 ID (int). "cool" → 1 (ModeCool).
	// JSON unmarshal 후에는 float64 로 저장됨.
	if state["mode"] != float64(1) {
		t.Errorf("state.mode = %v, want 1 (ModeCool)", state["mode"])
	}
	inferred, ok := m["inferred"].(map[string]any)
	if !ok {
		t.Fatalf("inferred group missing")
	}
	if inferred["temp_A_c"] != 25.2 {
		t.Errorf("inferred.temp_A_c = %v, want 25.2", inferred["temp_A_c"])
	}
	if inferred["op_val_1"] != float64(996) {
		t.Errorf("inferred.op_val_1 = %v, want 996", inferred["op_val_1"])
	}
}

// v0.5.1: register-decoded stream 제거로 다음 테스트들이 삭제됨:
//   - TestAgent_DefaultOutput_StatusGroupOnly
//   - TestAgent_IncludeAllFields_AllGroupsPresent
//   - TestAgent_RegisterDecoded_ChangeDetectionDeduplicates
//   - TestAgent_RegisterDecoded_ACKNotEmitted
//
// 모두 emit_register_decoded=true 옵션을 전제로 했으나, v0.5.1 에서 본 옵션과
// register-decoded 별도 stream 자체가 제거되었다. temp_evap_a_c / temp_evap_b_c
// 의 노출은 이제 device_state.state 그룹에서 직접 검증한다 (AC-H3 등).

// TestAgent_ProcessDrain_AppliesChangeDetection 는 v0.3.8 회귀 테스트이다.
//
// 사용자 보고: century-status 노드의 polling 결과 (processDrain) 에서 같은 state
// 메시지가 반복 emit. captureLoop msgCh 와 별개 path 라 v0.3.7 의 dedup 이 안 됨.
// 이번 fix 로 processDrain 에도 frameToEventIfChanged 적용.
func TestAgent_ProcessDrain_AppliesChangeDetection(t *testing.T) {
	t.Parallel()
	// 동일 reg02 5 회 — drain 결과의 frame 개수가 1 (첫 emit 만 통과) 이어야 한다.
	frame := mustBuildReg02ResponseFrame(t, 0x3B)
	stream := append([]byte{}, frame...)
	for i := 0; i < 4; i++ {
		stream = append(stream, frame...)
	}
	a, rt, cleanup := makeTestAgent(t, nil, stream)
	defer cleanup()

	// 모든 5 frame 이 ringBuffer 에 쌓이도록 대기.
	waitUntil(t, 2*time.Second, func() bool {
		return a.cStats.framesCaptured.Load() >= 5
	}, "frames not all captured")

	// processDrain 호출.
	resp, err := a.processDrain()
	if err != nil {
		t.Fatalf("processDrain: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// drain 결과의 frames 배열 길이 < 5 이어야 한다 (dedup 동작).
	frames, _ := result["frames"].([]any)
	if len(frames) >= 5 {
		t.Errorf("processDrain frames count = %d, want <5 (5 동일 frame 중 dedup 동작 안 함)", len(frames))
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0", rt.WriteCount())
	}
}

// TestAgent_ProcessDrain_SkipsACKFrames 는 ACK frame 이 processDrain 결과에서 제외됨을 검증.
func TestAgent_ProcessDrain_SkipsACKFrames(t *testing.T) {
	t.Parallel()
	// ACK + reg02 frame 1 개씩.
	stream := append([]byte{}, mustBuildAckFrame(t)...)
	stream = append(stream, mustBuildReg02ResponseFrame(t, 0x3B)...)
	a, _, cleanup := makeTestAgent(t, nil, stream)
	defer cleanup()

	waitUntil(t, 2*time.Second, func() bool {
		return a.cStats.framesCaptured.Load() >= 2
	}, "frames not all captured")

	resp, err := a.processDrain()
	if err != nil {
		t.Fatalf("processDrain: %v", err)
	}
	var result map[string]any
	json.Unmarshal(resp, &result)
	frames, _ := result["frames"].([]any)
	// ACK 가 제외되고 reg02 1 개만 남아야 한다.
	if len(frames) != 1 {
		t.Errorf("processDrain frames count = %d, want 1 (ACK skip + reg02 1개)", len(frames))
	}
}
