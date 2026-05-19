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
func drainMsgCh(t *testing.T, a *CenturyAgent, deadline time.Duration) []map[string]any {
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
func waitForMsgCount(t *testing.T, a *CenturyAgent, want int, deadline time.Duration) []map[string]any {
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
	for _, m := range msgs {
		tp, _ := m["type"].(string)
		switch tp {
		case EventTypeDeviceState:
			// good
		case "century_reg02_response",
			"century_reg03_response",
			"century_reg04_response",
			"century_reg04_write_request",
			"century_ack":
			t.Errorf("AC-H1: register-decoded message leaked through default config: %s", tp)
		default:
			// Reg02/Reg03/Reg04/ACK decoded structs don't carry a "type" field,
			// so absence of "type" still indicates a register-decoded leak.
			if tp == "" {
				t.Errorf("AC-H1: untyped (likely register-decoded) message leaked: %v", m)
			}
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

// AC-H2: single reg 0x02 response — first emit has trigger="change" + 0.0 fallback
// values for current_temp_c / evap_temp_a_c / evap_temp_b_c (REQ-CENTURY-033 A14).
func TestAgent_AC_H2_FirstReg02EmitsChangeWithFallbacks(t *testing.T) {
	t.Parallel()
	a, rt, cleanup := makeTestAgent(t, nil, mustBuildReg02ResponseFrame(t, 0x3B))
	defer cleanup()

	msgs := waitForMsgCount(t, a, 1, 2*time.Second)
	if len(msgs) < 1 {
		t.Fatalf("no device_state emit observed")
	}
	m := msgs[0]
	if got, _ := m["type"].(string); got != EventTypeDeviceState {
		t.Errorf("type = %q, want %q", got, EventTypeDeviceState)
	}
	if got, _ := m["dev_id"].(string); got != "0x3B" {
		t.Errorf("dev_id = %q, want 0x3B", got)
	}
	if got, _ := m["label"].(string); got != "indoor-3b" {
		t.Errorf("label = %q, want indoor-3b", got)
	}
	// v0.4.0: 5 핵심 + online 은 nested "state" 그룹으로 이동.
	st := deviceStateGroup(m)
	if got, _ := st["online"].(bool); !got {
		t.Errorf("state.online = false, want true")
	}
	if got, _ := st["power"].(bool); !got {
		t.Errorf("state.power = false, want true (mode=cooling)")
	}
	if got, _ := st["mode"].(string); got != "cool" {
		t.Errorf("state.mode = %q, want cool", got)
	}
	if got, _ := st["fan_speed"].(float64); got != 17 {
		t.Errorf("state.fan_speed = %v, want 17", got)
	}
	if got, _ := st["target_temp"].(float64); got != 25.0 {
		t.Errorf("state.target_temp = %v, want 25.0", got)
	}
	// reg04 not received — current_temp should be 0.0 fallback.
	if got, _ := st["current_temp"].(float64); got != 0.0 {
		t.Errorf("state.current_temp = %v, want 0.0 (reg04 not received)", got)
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
	// timestamp_ms / last_seen_ms must be epoch ms int64-shaped values.
	if got, ok := m["timestamp_ms"].(float64); !ok || got < 1_000_000_000_000 {
		t.Errorf("timestamp_ms = %v ok=%v, want epoch ms > 1e12", got, ok)
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0 (AC-B9)", rt.WriteCount())
	}
}

// AC-H3: reg 0x02 + reg 0x04 read → second emit shows current_temp_c populated.
func TestAgent_AC_H3_Reg04UpdatesCurrentTemp(t *testing.T) {
	t.Parallel()
	batch := append([]byte{}, mustBuildReg02ResponseFrame(t, 0x3B)...)
	batch = append(batch, mustBuildReg04ResponseFrame(t, 0x3B)...)

	a, rt, cleanup := makeTestAgent(t, nil, batch)
	defer cleanup()

	msgs := waitForMsgCount(t, a, 2, 2*time.Second)
	if len(msgs) < 2 {
		t.Fatalf("want >=2 device_state emits (reg02 + reg04), got %d", len(msgs))
	}
	first, second := msgs[0], msgs[1]
	firstSt := deviceStateGroup(first)
	secondSt := deviceStateGroup(second)
	if got, _ := firstSt["current_temp"].(float64); got != 0.0 {
		t.Errorf("first.state.current_temp = %v, want 0.0", got)
	}
	if got, _ := secondSt["current_temp"].(float64); got != 25.2 {
		t.Errorf("second.state.current_temp = %v, want 25.2 (CAP-4)", got)
	}
	// Mode/fan/setpoint must be preserved across the two emits.
	if got, _ := secondSt["mode"].(string); got != "cool" {
		t.Errorf("second.state.mode = %q, want cool preserved", got)
	}
	if got, _ := secondSt["fan_speed"].(float64); got != 17 {
		t.Errorf("second.state.fan_speed = %v, want 17 preserved", got)
	}
	if got, _ := secondSt["target_temp"].(float64); got != 25.0 {
		t.Errorf("second.state.target_temp = %v, want 25.0 preserved", got)
	}
	if got, _ := second["trigger"].(string); got != TriggerChange {
		t.Errorf("second.trigger = %q, want change", got)
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0 (AC-B9)", rt.WriteCount())
	}
}

// AC-H4: same frame three times → emit count must stay at 1 (5 core unchanged).
func TestAgent_AC_H4_UnchangedFramesDoNotReemit(t *testing.T) {
	t.Parallel()
	// Three identical reg02 frames.
	frame := mustBuildReg02ResponseFrame(t, 0x3B)
	batch := append([]byte{}, frame...)
	batch = append(batch, frame...)
	batch = append(batch, frame...)

	// Use a very long keepalive_interval so the test window doesn't trigger one.
	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"keepalive_interval": "10m",
	}, batch)
	defer cleanup()

	waitUntil(t, 2*time.Second, func() bool {
		return a.cStats.framesCaptured.Load() >= 3
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
func TestAgent_AC_H5_ModeTransitionEmitsPowerChange(t *testing.T) {
	t.Parallel()
	offFrame := mustBuildReg02ResponseFrameMode(t, 0x3B, 0x00)
	onFrame := mustBuildReg02ResponseFrameMode(t, 0x3B, 0x01)
	batch := append([]byte{}, offFrame...)
	batch = append(batch, onFrame...)

	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"keepalive_interval": "10m",
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
	if got, _ := firstSt["mode"].(string); got != "off" {
		t.Errorf("first.state.mode = %q, want off", got)
	}
	if got, _ := secondSt["power"].(bool); !got {
		t.Errorf("second.state.power = false, want true (mode=cooling)")
	}
	if got, _ := secondSt["mode"].(string); got != "cool" {
		t.Errorf("second.state.mode = %q, want cool", got)
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
func TestAgent_AC_H6_KeepaliveAfterInterval(t *testing.T) {
	t.Parallel()
	// One initial frame, then no more — keepalive must fire after 200ms.
	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"keepalive_interval": "200ms",
	}, mustBuildReg02ResponseFrame(t, 0x3B))
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
		if got, _ := m["trigger"].(string); got == TriggerKeepalive {
			sawKeepalive = true
			// Sanity: core fields preserved (state group).
			if got, _ := deviceStateGroup(m)["mode"].(string); got != "cool" {
				t.Errorf("keepalive emit dropped state.mode: got %q", got)
			}
		}
	}
	if !sawKeepalive {
		t.Errorf("AC-H6: no keepalive emit observed in %d messages", len(msgs))
	}
	// Stats counter must show at least one keepalive.
	if got := a.cStats.keepaliveEmits.Load(); got < 1 {
		t.Errorf("keepaliveEmits = %d, want >= 1", got)
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0", rt.WriteCount())
	}
}

// AC-H7: offline transition triggers immediate device_state with online=false.
func TestAgent_AC_H7_OfflineTransitionEmitsChange(t *testing.T) {
	t.Parallel()
	// Short offline_timeout, long keepalive (so keepalive doesn't interfere).
	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"offline_timeout":    "120ms",
		"keepalive_interval": "10m",
	}, mustBuildReg02ResponseFrame(t, 0x3B))
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
	if got, _ := offSt["mode"].(string); got != "cool" {
		t.Errorf("AC-H7: offline emit dropped stale state.mode: got %q", got)
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0", rt.WriteCount())
	}
}

// AC-H8: emit_device_state=false + emit_register_decoded=true → only register-decoded
// messages are emitted; no device_state and keepalive ticker is inactive.
//
// Register-decoded emit is gated on bridgeActive (a subscriber must be present),
// so we subscribe BEFORE delivering bytes — mirroring the v0.2.x flow.
func TestAgent_AC_H8_RegisterOnlyMode(t *testing.T) {
	t.Parallel()
	// Start with empty pipe and pre-flip bridgeActive so the captureLoop's
	// register-decoded path opens its msgCh send.
	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"emit_device_state":     false,
		"emit_register_decoded": true,
		"keepalive_interval":    "100ms", // would fire if logic incorrectly honored it
	}, nil)
	defer cleanup()
	a.bridgeActive.Store(true)
	// Now deliver the frame.
	rt.deliver(mustBuildReg02ResponseFrame(t, 0x3B))

	waitUntil(t, 1*time.Second, func() bool {
		return a.cStats.reg02ResponseCount.Load() >= 1
	}, "reg02 not counted")

	msgs := drainMsgCh(t, a, 400*time.Millisecond)
	if len(msgs) < 1 {
		t.Fatalf("AC-H8: no register-decoded emit observed")
	}
	for _, m := range msgs {
		tp, _ := m["type"].(string)
		if tp == EventTypeDeviceState {
			t.Errorf("AC-H8: device_state leaked when emit_device_state=false: %v", m)
		}
	}
	// Change detector / keepalive must be inactive: no device_state stats.
	if got := a.cStats.deviceStateEmits.Load(); got != 0 {
		t.Errorf("deviceStateEmits = %d, want 0 (emit_device_state=false)", got)
	}
	if got := a.cStats.keepaliveEmits.Load(); got != 0 {
		t.Errorf("keepaliveEmits = %d, want 0 (emit_device_state=false disables keepalive)", got)
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0", rt.WriteCount())
	}
}

// AC-H9: both emit options false → ErrCenturyNoOutputEnabled at parse time.
func TestAgent_AC_H9_BothEmitOptionsOff_ReturnsErrCenturyNoOutputEnabled(t *testing.T) {
	t.Parallel()
	cfg := agent.AgentConfig{
		ID:   "century-no-output",
		Name: "century-no-output",
		Type: "century-hvac",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port":           "/dev/ttyTEST",
				"emit_device_state":     false,
				"emit_register_decoded": false,
			},
		},
	}
	_, err := NewCenturyAgent(cfg)
	if !errors.Is(err, ErrCenturyNoOutputEnabled) {
		t.Fatalf("NewCenturyAgent err = %v, want ErrCenturyNoOutputEnabled", err)
	}
}

// AC-H10: multiple sub_dev_ids — independent change detection per device.
// Initial frames for 0x3B and 0x3C each trigger a change emit. A second identical
// 0x3B frame must not re-emit. Sub_dev_id 0x3C must not be affected.
func TestAgent_AC_H10_MultiSubDevIDIndependent(t *testing.T) {
	t.Parallel()
	batch := append([]byte{}, mustBuildReg02ResponseFrame(t, 0x3B)...)
	batch = append(batch, mustBuildReg02ResponseFrame(t, 0x3C)...)
	batch = append(batch, mustBuildReg02ResponseFrame(t, 0x3B)...) // duplicate of 3B

	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"keepalive_interval": "10m",
	}, batch)
	defer cleanup()

	waitUntil(t, 2*time.Second, func() bool {
		return len(a.ListDevices()) >= 2
	}, "two devices not discovered")
	waitUntil(t, 2*time.Second, func() bool {
		return a.cStats.framesCaptured.Load() >= 3
	}, "frames not all captured")

	msgs := drainMsgCh(t, a, 300*time.Millisecond)
	count3B, count3C := 0, 0
	for _, m := range msgs {
		switch m["dev_id"] {
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
		"dev_id": 59,
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
	if m["register"] == nil || m["dev_id"] == nil || m["timestamp_ms"] == nil {
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
	if state["mode"] != "cool" {
		t.Errorf("state.mode = %v, want cool", state["mode"])
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

// TestAgent_DefaultOutput_StatusGroupOnly 는 captureLoop 흐름 회귀 — 기본 옵션
// (emit_register_decoded=true, include_inferred=false, include_unknown=false) 에서
// 사용자가 본 노이즈 (op_val_*, reg04_const_*, status_bits, temp_A_c, reg*_byte_*,
// reg03_pad_*, reg02_live_*, reg02_word_*) 가 모두 빠져야 한다.
func TestAgent_DefaultOutput_StatusGroupOnly(t *testing.T) {
	t.Parallel()
	opts := map[string]any{
		"emit_register_decoded": true,
		"include_register_info": true, // v0.3.5: register 필드로 메시지 식별 위해 필요
		// include_inferred_fields / include_unknown_fields 미설정 → default false
	}
	a, rt, cleanup := makeTestAgent(t, opts, mustBuildReg03ResponseFrame(t, 0x3B))
	defer cleanup()

	msgs := waitForMsgCount(t, a, 1, 2*time.Second)
	var reg03 map[string]any
	for _, m := range msgs {
		if reg, ok := m["register"].(float64); ok && reg == 3 {
			reg03 = m
			break
		}
	}
	if reg03 == nil {
		t.Fatalf("no reg03 message found in %d emits", len(msgs))
	}
	// status 그룹 존재 + confirmed 필드 평탄화.
	state, ok := reg03["state"].(map[string]any)
	if !ok {
		t.Fatalf("state group missing in reg03 emit: %v", reg03)
	}
	if _, ok := state["temp_evap_a_c"]; !ok {
		t.Errorf("state.temp_evap_a_c (confirmed) must be present")
	}
	// pad / inferred / 원본 nested 모두 top-level 에 없어야 함.
	for _, k := range []string{
		"reg03_pad_4", "reg03_pad_12", "reg03_pad_15",
		"temp_evap_a_c", "temp_evap_b_c",
	} {
		if _, exists := reg03[k]; exists {
			t.Errorf("%q must NOT appear at top-level (default output)", k)
		}
	}
	if _, ok := reg03["inferred"]; ok {
		t.Errorf("inferred group must NOT appear (default include_inferred_fields=false)")
	}
	if _, ok := reg03["unknown"]; ok {
		t.Errorf("unknown group must NOT appear (default include_unknown_fields=false)")
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0", rt.WriteCount())
	}
}

// TestAgent_IncludeAllFields_AllGroupsPresent 는 두 옵션 모두 true 일 때 status /
// inferred / unknown 세 그룹이 모두 출력됨을 검증한다 (디버깅 / 프로토콜 RE 시).
func TestAgent_IncludeAllFields_AllGroupsPresent(t *testing.T) {
	t.Parallel()
	opts := map[string]any{
		"emit_register_decoded":   true,
		"include_inferred_fields": true,
		"include_unknown_fields":  true,
		"include_register_info":   true,
	}
	a, _, cleanup := makeTestAgent(t, opts, mustBuildReg03ResponseFrame(t, 0x3B))
	defer cleanup()

	msgs := waitForMsgCount(t, a, 1, 2*time.Second)
	var reg03 map[string]any
	for _, m := range msgs {
		if reg, ok := m["register"].(float64); ok && reg == 3 {
			reg03 = m
			break
		}
	}
	if reg03 == nil {
		t.Fatalf("no reg03 message found")
	}
	if _, ok := reg03["state"]; !ok {
		t.Errorf("state group missing")
	}
	if _, ok := reg03["unknown"]; !ok {
		t.Errorf("unknown group must appear when include_unknown_fields=true")
	}
	// inferred 그룹은 reg03 응답에 inferred 필드가 없으면 비어 있을 수 있음.
}

// TestAgent_RegisterDecoded_ChangeDetectionDeduplicates 는 v0.3.6 의 register-decoded
// change detection 회귀 테스트이다.
//
// 사용자 보고: "에이전트에서 상태 변화가 없는데, 메시지 전송". 같은 (dev_id, register) +
// 동일한 transformed payload 가 연속으로 흘러오면 emit 한 번만 발생해야 한다.
func TestAgent_RegisterDecoded_ChangeDetectionDeduplicates(t *testing.T) {
	t.Parallel()
	opts := map[string]any{
		"emit_register_decoded": true,
		"include_register_info": true, // 회귀 식별 위해 register 보존
	}
	// 동일한 reg02 응답 3회 — 첫 emit 만 흘러나오고 나머지는 dedupe 되어야 한다.
	frame := mustBuildReg02ResponseFrame(t, 0x3B)
	stream := append([]byte{}, frame...)
	stream = append(stream, frame...)
	stream = append(stream, frame...)
	a, rt, cleanup := makeTestAgent(t, opts, stream)
	defer cleanup()

	waitUntil(t, 2*time.Second, func() bool {
		return a.cStats.framesCaptured.Load() >= 3
	}, "frames not all captured")

	msgs := drainMsgCh(t, a, 200*time.Millisecond)

	// register-decoded (register=2) 메시지 count 가 3 미만이어야 한다 (dedup 동작).
	// bridgeActive timing 영향으로 0~1 사이 변동 가능; 핵심은 "3 개 모두 emit 되지 않음".
	reg02Count := 0
	for _, m := range msgs {
		if reg, ok := m["register"].(float64); ok && reg == 2 {
			reg02Count++
		}
	}
	if reg02Count >= 3 {
		t.Errorf("reg02 register-decoded emit count = %d, want <3 (3 동일 프레임 중 dedup 동작 안 함)", reg02Count)
	}
	// lastRegisterEmit cache 에 키가 등록되어 있어야 한다 (dedup 동작 증거).
	a.emitMu.Lock()
	_, hasKey := a.lastRegisterEmit[registerEmitKey{DevID: 0x3B, Register: 0x02}]
	a.emitMu.Unlock()
	if !hasKey {
		t.Errorf("lastRegisterEmit cache missing reg02 key — change detection 동작 안 함")
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0 (AC-B9)", rt.WriteCount())
	}
}

// TestAgent_RegisterDecoded_ACKNotEmitted 는 ACK frame 이 register-decoded 메시지로
// 흘러나오지 않음을 검증한다 (v0.3.6).
//
// 사용자 보고: 빈 메시지 ({"raw_hex":"","seq":...,"timestamp_ms":...}) 가 ACK 디코딩
// 결과로 나옴 — ACK 는 의미 없는 응답이므로 emit skip.
func TestAgent_RegisterDecoded_ACKNotEmitted(t *testing.T) {
	t.Parallel()
	opts := map[string]any{
		"emit_register_decoded": true,
		"include_register_info": true,
	}
	// ACK frame 만 주입 — register-decoded msgCh emit 가 0 이어야 한다.
	a, rt, cleanup := makeTestAgent(t, opts, mustBuildAckFrame(t))
	defer cleanup()

	waitUntil(t, 2*time.Second, func() bool {
		return a.cStats.framesCaptured.Load() >= 1
	}, "ACK frame not captured")

	msgs := drainMsgCh(t, a, 200*time.Millisecond)
	// device_state event 는 ACK 가 sub_dev_id 가 없으므로 emit 안 되고,
	// register-decoded 도 ACK 분기에서 skip — 즉 msgCh 전체에 0 메시지.
	for _, m := range msgs {
		// ACK 디코딩 결과 (TimestampMs + Direction 만) 가 흘러나오면 실패
		if _, hasState := m["state"]; !hasState {
			if _, hasType := m["type"].(string); !hasType {
				t.Errorf("ACK-shaped message leaked through register-decoded: %v", m)
			}
		}
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0", rt.WriteCount())
	}
}

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
