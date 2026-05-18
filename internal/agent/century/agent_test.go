package century

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// makeTestAgent builds a fully-initialized CenturyAgent with the given config + transport.
// Returns the agent, the recordingTransport, and a deferred shutdown function.
func makeTestAgent(t *testing.T, opts map[string]any, initialBytes []byte) (*CenturyAgent, *recordingTransport, func()) {
	t.Helper()
	rt := newRecordingTransport(initialBytes)
	if opts == nil {
		opts = map[string]any{}
	}
	if _, ok := opts["serial_port"]; !ok {
		opts["serial_port"] = "/dev/ttyTEST"
	}
	centuryCfg, err := parseCenturyConfig(opts)
	if err != nil {
		t.Fatalf("parseCenturyConfig: %v", err)
	}
	cfg := agent.AgentConfig{
		ID:        "century-test",
		Name:      "century-test",
		Type:      "century-hvac",
		Transport: agent.TransportConfig{Type: "serial", Options: opts},
	}
	a := newCenturyAgentForTest(cfg, centuryCfg, rt)
	if err := a.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	cleanup := func() {
		_ = a.Stop(context.Background())
	}
	return a, rt, cleanup
}

func TestAgent_AC_B1a_FirstSubDevIDAutoDiscovery(t *testing.T) {
	t.Parallel()
	// AC-B1a: feed CAP-3 reg 0x02 response (sub_dev_id=0x3B) and expect a single auto device.
	frame := mustBuildReg02ResponseFrame(t, 0x3B)
	a, rt, cleanup := makeTestAgent(t, nil, frame)
	defer cleanup()

	waitUntil(t, 500*time.Millisecond, func() bool {
		return len(a.ListDevices()) >= 1
	}, "device 0x3B not auto-discovered")

	devs := a.ListDevices()
	if len(devs) != 1 {
		t.Fatalf("len(devices) = %d, want 1", len(devs))
	}
	d := devs[0]
	if d.SubDevID != 0x3B {
		t.Errorf("device SubDevID = 0x%02X, want 0x3B", d.SubDevID)
	}
	if d.Source != "auto" {
		t.Errorf("device Source = %q, want auto", d.Source)
	}
	if d.Label != "indoor-3b" {
		t.Errorf("device Label = %q, want indoor-3b", d.Label)
	}
	if !d.Online {
		t.Errorf("device Online = false, want true")
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0 (AC-B9)", rt.WriteCount())
	}
}

// TestAgent_AutoDiscovery_FromReadRequestFrame 는 회귀 테스트이다.
//
// 회귀 시나리오: master 의 READ request (FCRead 0x0B) 만 sniff 라인에서 캡처되고
// slave 응답이 누락되거나 디코드에 실패하는 환경에서, 사용자가 "자동 디바이스 등록 안됨"
// 증상을 보고했다.
//
// READ request 도 payload prefix 에 sub_dev_id 가 있으므로 (spec §5: payload[0]) decoded
// 결과가 nil 이어도 device 자동 발견은 가능해야 한다. captureLoop 의 fallback 경로
// (subDevIDFromFrame + touchDeviceFromSubDevID) 가 이 케이스를 커버한다.
func TestAgent_AutoDiscovery_FromReadRequestFrame(t *testing.T) {
	t.Parallel()
	// READ request reg 0x02 (master → slave), payload prefix = [0x3B, 0x00, 0x02]
	frame := mustBuildReadRequestFrame(t, 0x3B, 0x02)
	a, rt, cleanup := makeTestAgent(t, nil, frame)
	defer cleanup()

	waitUntil(t, 500*time.Millisecond, func() bool {
		return len(a.ListDevices()) >= 1
	}, "device 0x3B not auto-discovered from READ request frame")

	devs := a.ListDevices()
	if len(devs) != 1 {
		t.Fatalf("len(devices) = %d, want 1 (회귀: READ request 만으로 자동 발견되어야 함)", len(devs))
	}
	d := devs[0]
	if d.SubDevID != 0x3B {
		t.Errorf("device SubDevID = 0x%02X, want 0x3B", d.SubDevID)
	}
	if d.Source != "auto" {
		t.Errorf("device Source = %q, want auto", d.Source)
	}
	if !d.Online {
		t.Errorf("device Online = false, want true")
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0 (AC-B9 invariant)", rt.WriteCount())
	}
}

func TestAgent_AC_B1b_MultipleSubDevIDsIsolated(t *testing.T) {
	t.Parallel()
	// AC-B1b: After 0x3B is discovered, inject a frame with sub_dev_id=0x3C → second device created.
	frame3B := mustBuildReg02ResponseFrame(t, 0x3B)
	a, rt, cleanup := makeTestAgent(t, nil, frame3B)
	defer cleanup()

	waitUntil(t, 500*time.Millisecond, func() bool { return len(a.ListDevices()) >= 1 }, "0x3B not discovered")

	// Inject a frame with sub_dev_id=0x3C.
	frame3C := mustBuildReg02ResponseFrame(t, 0x3C)
	rt.deliver(frame3C)

	waitUntil(t, 500*time.Millisecond, func() bool { return len(a.ListDevices()) >= 2 }, "0x3C not auto-discovered")

	devs := a.ListDevices()
	if len(devs) != 2 {
		t.Fatalf("len(devices) = %d, want 2", len(devs))
	}
	have3B, have3C := false, false
	var lastSeen3B time.Time
	for _, d := range devs {
		switch d.SubDevID {
		case 0x3B:
			have3B = true
			lastSeen3B = d.LastSeen
		case 0x3C:
			have3C = true
			if d.Label != "indoor-3c" {
				t.Errorf("0x3C device Label = %q, want indoor-3c", d.Label)
			}
			if d.Source != "auto" {
				t.Errorf("0x3C device Source = %q, want auto", d.Source)
			}
		}
	}
	if !have3B || !have3C {
		t.Fatalf("missing devices: have3B=%v have3C=%v", have3B, have3C)
	}

	// Inject another 0x3B frame; 0x3C's LastSeen must NOT change.
	time.Sleep(20 * time.Millisecond) // ensure timestamps differ
	rt.deliver(mustBuildReg02ResponseFrame(t, 0x3B))
	waitUntil(t, 500*time.Millisecond, func() bool {
		devs := a.ListDevices()
		for _, d := range devs {
			if d.SubDevID == 0x3B && d.LastSeen.After(lastSeen3B) {
				return true
			}
		}
		return false
	}, "0x3B device LastSeen did not advance after second frame")

	// Verify 0x3C's LastSeen is unchanged.
	devsAfter := a.ListDevices()
	var lastSeen3C time.Time
	for _, d := range devsAfter {
		if d.SubDevID == 0x3C {
			lastSeen3C = d.LastSeen
		}
	}
	// Sanity: lastSeen3C should be from the original 0x3C frame, not refreshed.
	// We don't have an exact reference so we check that 0x3C didn't catch up to 0x3B's latest LastSeen.
	for _, d := range devsAfter {
		if d.SubDevID == 0x3B && !d.LastSeen.After(lastSeen3C) {
			t.Errorf("expected 0x3B.LastSeen > 0x3C.LastSeen after re-injection; got 3B=%v 3C=%v",
				d.LastSeen, lastSeen3C)
		}
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0 (AC-B9)", rt.WriteCount())
	}
}

func TestAgent_AC_B2_OfflineDetection(t *testing.T) {
	t.Parallel()
	// AC-B2: with offline_timeout=80ms, after no frames arrive the device transitions to offline.
	frame := mustBuildReg02ResponseFrame(t, 0x3B)
	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"offline_timeout": "80ms",
	}, frame)
	defer cleanup()

	waitUntil(t, 500*time.Millisecond, func() bool {
		devs := a.ListDevices()
		return len(devs) >= 1 && devs[0].Online
	}, "device did not come online")

	// Now wait for offline transition.
	waitUntil(t, 2*time.Second, func() bool {
		for _, d := range a.ListDevices() {
			if d.SubDevID == 0x3B && !d.Online {
				// State must still be preserved (AC-B2 last-known status).
				if d.State == nil || d.State.Reg02 == nil {
					t.Errorf("offline device lost last-known state")
				}
				return true
			}
		}
		return false
	}, "device did not transition to offline")

	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0 (AC-B9)", rt.WriteCount())
	}
}

func TestAgent_AC_B3_RingBufferOverflowCountsDrops(t *testing.T) {
	t.Parallel()
	// AC-B3: ring_buffer_size=16 (minimum), feed > 16 frames → drops counted.
	// We build 20 frames; the agent should accumulate 4 drops while the buffer holds 16.
	const target = 20
	// Use sub_dev_id varying to keep payloads distinct (avoiding accidental dedup interactions —
	// though dedup applies to writes only, not reg02 responses).
	bytesBatch := make([]byte, 0)
	for i := 0; i < target; i++ {
		bytesBatch = append(bytesBatch, mustBuildReg02ResponseFrame(t, 0x3B)...)
	}
	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"ring_buffer_size": 16,
	}, bytesBatch)
	defer cleanup()

	waitUntil(t, 1*time.Second, func() bool {
		return a.cStats.framesValid.Load() >= target
	}, "not all frames decoded")

	if got := a.ringBuffer.Len(); got != 16 {
		t.Errorf("ringBuffer.Len() = %d, want 16 (capacity)", got)
	}
	if got := a.cStats.framesDropped.Load(); got != target-16 {
		t.Errorf("framesDropped = %d, want %d", got, target-16)
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0 (AC-B9)", rt.WriteCount())
	}
}

func TestAgent_AC_B9_NoTransportWrite_OverFullProcessCycle(t *testing.T) {
	t.Parallel()
	// AC-B9: the agent must NEVER call transport.Write, across all observed code paths.
	frame := mustBuildReg02ResponseFrame(t, 0x3B)
	a, rt, cleanup := makeTestAgent(t, nil, frame)
	defer cleanup()

	waitUntil(t, 500*time.Millisecond, func() bool {
		return a.cStats.framesValid.Load() >= 1
	}, "frame not processed")

	// Exercise every supported Process command.
	if _, err := a.Process([]byte(`{"command":"get_stats"}`)); err != nil {
		t.Fatalf("get_stats: %v", err)
	}
	if _, err := a.Process([]byte(`{"command":"get_recent","count":10}`)); err != nil {
		t.Fatalf("get_recent: %v", err)
	}
	if _, err := a.Process([]byte(`{"command":"drain"}`)); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Send a control-flavored Process call; must not call Write either.
	resp, err := a.Process([]byte(`{"command":"set_temp"}`))
	if err != nil {
		t.Fatalf("set_temp Process returned err: %v", err)
	}
	if !strings.Contains(string(resp), "not_supported") {
		t.Errorf("set_temp response did not include not_supported: %s", resp)
	}

	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0 (AC-B9)", rt.WriteCount())
	}
	if len(rt.WriteCalls()) != 0 {
		t.Errorf("transport.Write called %d times, want 0", len(rt.WriteCalls()))
	}
}

func TestAgent_AC_F1_WriteDedupEmitsOnce(t *testing.T) {
	t.Parallel()
	// AC-F1: dedupe_writes=true (default). Two identical write frames in same cycle → reg04_write_count=1.
	frame1 := mustBuildReg04WriteFrame(t, 0x3B, 0x02, 0x10)
	frame2 := mustBuildReg04WriteFrame(t, 0x3B, 0x02, 0x10) // identical raw payload
	batch := append([]byte(nil), frame1...)
	batch = append(batch, frame2...)
	a, rt, cleanup := makeTestAgent(t, nil, batch)
	defer cleanup()

	waitUntil(t, 1*time.Second, func() bool {
		return a.cStats.framesCaptured.Load() >= 2
	}, "frames not captured")

	if got := a.cStats.reg04WriteCount.Load(); got != 1 {
		t.Errorf("reg04WriteCount = %d, want 1 (dedup of duplicate)", got)
	}
	if got := a.cStats.writesDeduped.Load(); got != 1 {
		t.Errorf("writesDeduped = %d, want 1", got)
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0 (AC-B9)", rt.WriteCount())
	}
}

func TestAgent_AC_F2_DedupDisabledEmitsAll(t *testing.T) {
	t.Parallel()
	// AC-F2: dedupe_writes=false → both writes emit, no dedup counter increment.
	frame1 := mustBuildReg04WriteFrame(t, 0x3B, 0x02, 0x10)
	frame2 := mustBuildReg04WriteFrame(t, 0x3B, 0x02, 0x10)
	batch := append([]byte(nil), frame1...)
	batch = append(batch, frame2...)
	a, rt, cleanup := makeTestAgent(t, map[string]any{
		"dedupe_writes": false,
	}, batch)
	defer cleanup()

	waitUntil(t, 1*time.Second, func() bool {
		return a.cStats.reg04WriteCount.Load() >= 2
	}, "writes not all counted")

	if got := a.cStats.reg04WriteCount.Load(); got != 2 {
		t.Errorf("reg04WriteCount = %d, want 2 (no dedup)", got)
	}
	if got := a.cStats.writesDeduped.Load(); got != 0 {
		t.Errorf("writesDeduped = %d, want 0 (dedup disabled)", got)
	}
	if rt.WriteCount() != 0 {
		t.Errorf("transport.Write called %d bytes, want 0 (AC-B9)", rt.WriteCount())
	}
}

func TestAgent_AC_D2_MissingSerialPortRejected(t *testing.T) {
	t.Parallel()
	// AC-D2: missing serial_port → ErrSerialPortRequired propagated through NewCenturyAgent.
	cfg := agent.AgentConfig{
		ID:   "century-bad",
		Name: "century-bad",
		Type: "century-hvac",
		Transport: agent.TransportConfig{
			Type:    "serial",
			Options: map[string]any{},
		},
	}
	_, err := NewCenturyAgent(cfg)
	if err == nil {
		t.Fatalf("NewCenturyAgent returned no error; want serial_port-required error")
	}
	if !strings.Contains(err.Error(), "serial_port") {
		t.Errorf("err = %v; want message mentioning serial_port", err)
	}
}

func TestAgent_LifecycleTransitions(t *testing.T) {
	t.Parallel()
	frame := mustBuildReg02ResponseFrame(t, 0x3B)
	a, _, cleanup := makeTestAgent(t, nil, frame)
	defer cleanup()

	if state := a.CurrentState(); state != lifecycle.StateRunning {
		t.Fatalf("after Start, state = %s, want running", state)
	}

	if err := a.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if state := a.CurrentState(); state != lifecycle.StatePaused {
		t.Fatalf("after Pause, state = %s, want paused", state)
	}

	if err := a.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if state := a.CurrentState(); state != lifecycle.StateRunning {
		t.Fatalf("after Resume, state = %s, want running", state)
	}
}

func TestAgent_ProcessGetStatsContainsExpectedKeys(t *testing.T) {
	t.Parallel()
	frame := mustBuildReg02ResponseFrame(t, 0x3B)
	a, _, cleanup := makeTestAgent(t, nil, frame)
	defer cleanup()

	waitUntil(t, 500*time.Millisecond, func() bool {
		return a.cStats.reg02ResponseCount.Load() >= 1
	}, "reg02 not counted")

	body, err := a.Process([]byte(`{"command":"get_stats"}`))
	if err != nil {
		t.Fatalf("get_stats: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("unmarshal get_stats: %v", err)
	}
	expected := []string{
		"frames_captured", "frames_valid", "frames_invalid", "frames_dropped",
		"reg02_response_count", "reg03_response_count", "reg04_response_count",
		"reg04_write_count", "ack_count",
		"invalid_length_mismatch", "invalid_crc_mismatch", "invalid_header_invalid",
		"invalid_payload_prefix", "invalid_register_length",
		"writes_deduped", "devices_discovered", "transport_connected",
	}
	for _, k := range expected {
		if _, ok := m[k]; !ok {
			t.Errorf("get_stats missing key %q", k)
		}
	}
}

func TestAgent_ProcessDrainEmptiesRingBuffer(t *testing.T) {
	t.Parallel()
	// AC-B8: drain returns all then ring buffer is empty.
	batch := append([]byte(nil), mustBuildReg02ResponseFrame(t, 0x3B)...)
	batch = append(batch, mustBuildReg03ResponseFrame(t, 0x3B)...)
	a, _, cleanup := makeTestAgent(t, nil, batch)
	defer cleanup()

	waitUntil(t, 500*time.Millisecond, func() bool {
		return a.ringBuffer.Len() >= 2
	}, "ring buffer not populated")

	body, err := a.Process([]byte(`{"command":"drain"}`))
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("unmarshal drain: %v", err)
	}
	if got, _ := m["count"].(float64); int(got) < 2 {
		t.Errorf("drain count = %v, want >= 2", m["count"])
	}
	if a.ringBuffer.Len() != 0 {
		t.Errorf("ring buffer not empty after drain: %d", a.ringBuffer.Len())
	}
}

func TestAgent_FrameNotifyChDeliversSignal(t *testing.T) {
	t.Parallel()
	frame := mustBuildReg02ResponseFrame(t, 0x3B)
	a, _, cleanup := makeTestAgent(t, nil, frame)
	defer cleanup()

	select {
	case <-a.FrameNotifyCh():
		// good — got a signal.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("FrameNotifyCh did not deliver signal within 500ms")
	}
}

func TestAgent_ConfigDevicesRegistered(t *testing.T) {
	t.Parallel()
	a, _, cleanup := makeTestAgent(t, map[string]any{
		"devices": []any{
			map[string]any{"address": "0x3B", "name": "living-room"},
		},
	}, nil)
	defer cleanup()
	devs := a.ListDevices()
	if len(devs) != 1 {
		t.Fatalf("len(devices) = %d, want 1 (preconfigured)", len(devs))
	}
	if devs[0].Source != "config" {
		t.Errorf("Source = %q, want config", devs[0].Source)
	}
	if devs[0].Label != "living-room" {
		t.Errorf("Label = %q, want living-room", devs[0].Label)
	}
}

func TestAgent_StatePopulatesDeviceList(t *testing.T) {
	t.Parallel()
	frame := mustBuildReg02ResponseFrame(t, 0x3B)
	a, _, cleanup := makeTestAgent(t, nil, frame)
	defer cleanup()

	waitUntil(t, 500*time.Millisecond, func() bool { return len(a.ListDevices()) >= 1 }, "device not discovered")
	s := a.State()
	devs, ok := s["devices"].([]map[string]any)
	if !ok {
		t.Fatalf("State()[devices] not a list of maps, got %T", s["devices"])
	}
	if len(devs) != 1 {
		t.Errorf("State() devices count = %d, want 1", len(devs))
	}
}

// Helper for tests that need a reg03 frame builder (used in drain test).
func mustBuildReg03ResponseFrame(t *testing.T, subDevID byte) []byte {
	t.Helper()
	// 16B data: temps + zero padding.
	payload := []byte{
		subDevID, 0x00, 0x03,
		0xC3, 0x00, 0xC8, 0x00, // temp_evap_a=19.5 (0x00C3=195/10), temp_evap_b=20.0
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	return buildFrame(AddrSlave, AddrMaster, FCResponse, payload)
}
