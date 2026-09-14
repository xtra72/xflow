package lg

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// 시험 하네스
// ---------------------------------------------------------------------------

// newTestAgent 는 mock 게이트웨이에 물린 에이전트를 만든다.
// 백그라운드 루프는 기동하지 않는다 — 각 테스트가 폴링·스캔을 직접 호출해
// 타이밍 의존 없이 결정적으로 검증한다.
func newTestAgent(t *testing.T, gw *mockGateway, opts map[string]any) *Hvacr03Agent {
	t.Helper()

	base := map[string]any{
		"transport_type": "rtu",
		"serial_port":    "/dev/null",
		"poll_interval":  "5s",
	}
	for k, v := range opts {
		base[k] = v
	}

	cfg, err := parseHvacr03Config(base)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}

	agentCfg := agent.AgentConfig{
		ID:   "test-agent-id",
		Name: "test-pmbus",
		Type: "lg_hvacr03",
	}

	a := &Hvacr03Agent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("lg_hvacr03-test")),
		hvacr03Config: cfg,
		transport:     gw,
		stopCh:        make(chan struct{}),
		msgCh:         make(chan []byte, cfg.MsgChannelSize),
		stats:         agent.NewAgentStats(),
		logger:        agent.ResolveLogger(agentCfg),
		createdAt:     time.Now(),
		recentEvents:  make([]pmbusEventRecord, hvacr03RecentBufferSize),
		recentNotify:  make(chan struct{}, 1),
		devices:       make(map[string]*PmbusDevice),
		lastEmitted:   make(map[string]map[string]any),
		agentConfig:   agentCfg,
	}
	a.connected.Store(true)

	t.Cleanup(func() {
		select {
		case <-a.stopCh:
		default:
			close(a.stopCh)
		}
	})
	return a
}

// exec 는 JSON 명령을 실행하고 응답을 맵으로 돌려준다.
func exec(t *testing.T, a *Hvacr03Agent, req map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := a.Process(data)
	if err != nil {
		t.Fatalf("Process(%v): %v", req["command"], err)
	}
	var m map[string]any
	if len(resp) > 0 {
		if err := json.Unmarshal(resp, &m); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
	}
	return m
}

// execErr 는 명령이 실패할 것을 기대하고 에러를 돌려준다.
func execErr(t *testing.T, a *Hvacr03Agent, req map[string]any) error {
	t.Helper()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	_, err = a.Process(data)
	return err
}

// ---------------------------------------------------------------------------
// 디바이스 발견 (AC-034 ~ AC-038)
// ---------------------------------------------------------------------------

func TestHvacr03_ScanDiscoversConnectedUnits(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(0, true)
	gw.setUnitConnected(3, true)
	gw.setUnitConnected(7, true)

	a := newTestAgent(t, gw, nil)
	a.scanDevices()

	devices := a.ListDevices()
	if len(devices) != 3 {
		t.Fatalf("discovered %d devices, want 3", len(devices))
	}
	found := map[string]bool{}
	for _, d := range devices {
		found[d.Address] = true
		if !d.Online {
			t.Errorf("device %s should be online", d.Address)
		}
		if d.Source != "auto" {
			t.Errorf("device %s source = %q, want auto", d.Address, d.Source)
		}
	}
	for _, addr := range []string{"0", "3", "7"} {
		if !found[addr] {
			t.Errorf("device %s not discovered", addr)
		}
	}
}

// TestHvacr03_ScanUsesSingleTransaction 은 전체 스캔이 1 트랜잭션인지 확인한다.
// 16대 × 16bit 를 한 번에 읽는 것이 이 설계의 핵심 이점이다.
func TestHvacr03_ScanUsesSingleTransaction(t *testing.T) {
	gw := newMockGateway()
	for n := uint16(0); n < 16; n++ {
		gw.setUnitConnected(n, true)
	}

	a := newTestAgent(t, gw, nil)
	a.scanDevices()

	if got := gw.requestCount(); got != 1 {
		t.Fatalf("scan issued %d transactions, want 1", got)
	}
	reqs := gw.requestsWithFC(pmbusFCReadDiscreteInputs)
	if len(reqs) != 1 {
		t.Fatalf("FC02 requests = %d, want 1", len(reqs))
	}
	r := reqs[0]
	addr := uint16(r[1])<<8 | uint16(r[2])
	qty := uint16(r[3])<<8 | uint16(r[4])
	if addr != 0 || qty != pmbusScanBitCount {
		t.Errorf("scan read addr=%d qty=%d, want addr=0 qty=%d", addr, qty, pmbusScanBitCount)
	}
	if len(a.ListDevices()) != 16 {
		t.Errorf("discovered %d devices, want 16", len(a.ListDevices()))
	}
}

func TestHvacr03_ScanExtractsAlarmBits(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(3, true)
	gw.setDiscreteBit(3, pmbusDiscreteAlarm, true)
	gw.setDiscreteBit(3, pmbusDiscreteFilterAlarm, false)

	a := newTestAgent(t, gw, nil)
	a.scanDevices()

	devices := a.ListDevices()
	if len(devices) != 1 {
		t.Fatalf("devices = %d, want 1", len(devices))
	}
	s := devices[0].State
	if s.Alarm == nil || !*s.Alarm {
		t.Error("alarm should be true")
	}
	if s.FilterAlarm == nil || *s.FilterAlarm {
		t.Error("filter_alarm should be false")
	}
}

func TestHvacr03_AutoDiscoveryDisabled(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(0, true)
	gw.setUnitConnected(3, true)
	gw.setUnitConnected(7, true)

	a := newTestAgent(t, gw, map[string]any{
		"auto_discovery": false,
		"devices":        []any{map[string]any{"address": "0", "name": "거실"}},
	})
	a.registerConfigDevices()
	a.scanDevices()

	devices := a.ListDevices()
	if len(devices) != 1 {
		t.Fatalf("devices = %d, want 1 (auto discovery off)", len(devices))
	}
	if devices[0].Address != "0" || devices[0].Label != "거실" {
		t.Errorf("device = %s/%s, want 0/거실", devices[0].Address, devices[0].Label)
	}
}

// TestHvacr03_ConfigDeviceSurvivesMissingScan 은 설정 디바이스가 스캔에서
// 미발견되어도 목록에 남되 오프라인으로 표시되는지 확인한다.
func TestHvacr03_ConfigDeviceSurvivesMissingScan(t *testing.T) {
	gw := newMockGateway()
	// N=5 는 연결하지 않는다.

	a := newTestAgent(t, gw, map[string]any{
		"devices": []any{map[string]any{"address": "5", "name": "안방"}},
	})
	a.registerConfigDevices()
	a.scanDevices()

	devices := a.ListDevices()
	if len(devices) != 1 {
		t.Fatalf("devices = %d, want 1", len(devices))
	}
	if devices[0].Online {
		t.Error("unconnected config device should be offline")
	}
	if devices[0].Source != "config" {
		t.Errorf("source = %q, want config", devices[0].Source)
	}
}

// TestHvacr03_HydroKitAutoPromotion 은 목표 온도 기준이 "물"인 디바이스가
// 하이드로킷으로 자동 판별되는지 확인한다.
func TestHvacr03_HydroKitAutoPromotion(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(2, true)
	gw.setDiscreteBit(2, pmbusDiscreteTempBasis, true) // 물 기준

	a := newTestAgent(t, gw, nil)
	a.scanDevices()

	devices := a.ListDevices()
	if len(devices) != 1 {
		t.Fatalf("devices = %d, want 1", len(devices))
	}
	if devices[0].Type != pmbusDeviceTypeAWHP {
		t.Errorf("device type = %q, want %q", devices[0].Type, pmbusDeviceTypeAWHP)
	}
}

// TestHvacr03_ConfiguredTypeWinsOverAutoPromotion 은 설정으로 종류가 고정된
// 디바이스가 프로토콜 신호로 승격되지 않는지 확인한다.
func TestHvacr03_ConfiguredTypeWinsOverAutoPromotion(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(2, true)
	gw.setDiscreteBit(2, pmbusDiscreteTempBasis, true)

	a := newTestAgent(t, gw, map[string]any{
		"devices": []any{map[string]any{"address": "2", "device_type": "HVACR.ERV"}},
	})
	a.registerConfigDevices()
	a.scanDevices()

	devices := a.ListDevices()
	if devices[0].Type != pmbusDeviceTypeERV {
		t.Errorf("device type = %q, want %q (config should win)", devices[0].Type, pmbusDeviceTypeERV)
	}
}

// ---------------------------------------------------------------------------
// 폴링 (AC-042 ~ AC-045)
// ---------------------------------------------------------------------------

// setupPolledAgent 는 실내기 하나를 발견·폴링한 에이전트를 만든다.
func setupPolledAgent(t *testing.T, n uint16, opts map[string]any) (*Hvacr03Agent, *mockGateway) {
	t.Helper()
	gw := newMockGateway()
	gw.setUnitConnected(n, true)
	gw.setCoil(n, pmbusCoilPower, true)
	gw.setHolding(n, pmbusHoldingMode, pmbusModeCool)
	gw.setHolding(n, pmbusHoldingFanSpeed, pmbusFanHigh)
	gw.setHolding(n, pmbusHoldingSetTemp, 240) // 24.0 °C
	gw.setInput(n, pmbusInputRoomTemp, 245)    // 24.5 °C
	gw.setInput(n, pmbusInputPipeIn, 210)
	gw.setInput(n, pmbusInputPipeOut, 232)

	a := newTestAgent(t, gw, opts)
	a.scanDevices()
	gw.resetRequests()
	a.pollAllDevices()
	return a, gw
}

func TestHvacr03_PollIssuesThreeTransactions(t *testing.T) {
	a, gw := setupPolledAgent(t, 0, nil)
	_ = a

	for _, fc := range []byte{pmbusFCReadCoils, pmbusFCReadHolding, pmbusFCReadInput} {
		if got := len(gw.requestsWithFC(fc)); got != 1 {
			t.Errorf("FC 0x%02X requests = %d, want 1", fc, got)
		}
	}
	if got := gw.requestCount(); got != 3 {
		t.Errorf("poll issued %d transactions, want 3", got)
	}
}

func TestHvacr03_PollDecodesState(t *testing.T) {
	a, _ := setupPolledAgent(t, 0, nil)

	devices := a.ListDevices()
	if len(devices) != 1 {
		t.Fatalf("devices = %d, want 1", len(devices))
	}
	props := devices[0].State.toProperties(devices[0].Type, a.projectionOpts())

	checks := []struct {
		key  string
		want any
	}{
		{"power", true},
		{"current_temperature", 24.5},
		{"target_temperature", 24.0},
		{"mode", 1},      // hvac.ModeCool
		{"fan_speed", 5}, // hvac.FanHigh
		{"pipe_in_temperature_c", 21.0},
		{"pipe_out_temperature_c", 23.2},
		{"error_code", 0},
	}
	for _, c := range checks {
		got, ok := props[c.key]
		if !ok {
			t.Errorf("%s missing from projection", c.key)
			continue
		}
		if got != c.want {
			t.Errorf("%s = %v (%T), want %v", c.key, got, got, c.want)
		}
	}
}

// TestHvacr03_PollSkipsOfflineDevices 는 오프라인 디바이스에 요청을 보내지 않는지
// 확인한다. 무응답 디바이스를 계속 폴링하면 버스 대역과 사이클 시간이 낭비된다.
func TestHvacr03_PollSkipsOfflineDevices(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(0, true)
	gw.setUnitConnected(3, false)

	a := newTestAgent(t, gw, map[string]any{
		"devices": []any{map[string]any{"address": "3"}},
	})
	a.registerConfigDevices()
	a.scanDevices()
	gw.resetRequests()
	a.pollAllDevices()

	// 온라인은 N=0 하나뿐 → 3 트랜잭션.
	if got := gw.requestCount(); got != 3 {
		t.Errorf("poll issued %d transactions, want 3 (offline device skipped)", got)
	}
	for _, r := range gw.requestsWithFC(pmbusFCReadHolding) {
		addr := uint16(r[1])<<8 | uint16(r[2])
		if addr == pmbusHoldingAddr(3, 0) {
			t.Error("offline device N=3 should not be polled")
		}
	}
}

// TestHvacr03_PollContinuesAfterDeviceFailure 는 한 디바이스의 실패가 사이클
// 전체를 중단시키지 않는지 확인한다.
func TestHvacr03_PollContinuesAfterDeviceFailure(t *testing.T) {
	gw := newMockGateway()
	for _, n := range []uint16{0, 1, 2} {
		gw.setUnitConnected(n, true)
		gw.setCoil(n, pmbusCoilPower, true)
	}

	a := newTestAgent(t, gw, nil)
	a.scanDevices()
	gw.resetRequests()

	// N=1 의 첫 읽기(FC01)를 Modbus 예외로 실패시킨다. 예외는 게이트웨이가 응답한
	// 것이므로 연결은 살아 있고, 사이클이 계속되어야 한다.
	failTarget := pmbusCoilAddr(1, 0)
	gw.interceptor = func(pdu []byte) ([]byte, error, bool) {
		if pdu[0] == pmbusFCReadCoils {
			addr := uint16(pdu[1])<<8 | uint16(pdu[2])
			if addr == failTarget {
				return []byte{pmbusFCReadCoils | 0x80, 0x04}, nil, true
			}
		}
		return nil, nil, false
	}

	a.pollAllDevices()

	// N=0, N=2 는 3 트랜잭션씩, N=1 은 실패한 1 트랜잭션 → 총 7.
	if got := gw.requestCount(); got != 7 {
		t.Errorf("transactions = %d, want 7 (cycle should continue past the failure)", got)
	}
	if a.connected.Load() != true {
		t.Error("a modbus exception should not mark the transport disconnected")
	}
	if a.pollsFailed.Load() != 1 {
		t.Errorf("polls_failed = %d, want 1", a.pollsFailed.Load())
	}
}

// TestHvacr03_TransportFailureMarksDevicesOffline 은 연결 단절 시 전 디바이스가
// 오프라인으로 전이되는지 확인한다.
func TestHvacr03_TransportFailureMarksDevicesOffline(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(0, true)

	a := newTestAgent(t, gw, nil)
	a.scanDevices()
	if !a.ListDevices()[0].Online {
		t.Fatal("device should be online after scan")
	}

	// I/O 오류를 주입한다 — Modbus 예외가 아니므로 연결 단절로 판단되어야 한다.
	gw.mu.Lock()
	gw.failNext = 10
	gw.failErr = errors.New("serial: i/o timeout")
	gw.mu.Unlock()

	a.pollAllDevices()

	if a.connected.Load() {
		t.Error("transport should be marked disconnected after an i/o failure")
	}
	if a.ListDevices()[0].Online {
		t.Error("devices should be offline after a transport failure")
	}
}

// ---------------------------------------------------------------------------
// 메시지 방출 (AC-050 ~ AC-056)
// ---------------------------------------------------------------------------

// drainEvents 는 링 버퍼의 이벤트를 파싱해 돌려준다.
func drainEvents(t *testing.T, a *Hvacr03Agent) []map[string]any {
	t.Helper()
	resp := exec(t, a, map[string]any{"command": "get_recent", "count": 0})
	framesRaw, ok := resp["frames"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(framesRaw))
	for _, f := range framesRaw {
		m, ok := f.(map[string]any)
		if !ok {
			t.Fatalf("frame is not an object: %T", f)
		}
		out = append(out, m)
	}
	return out
}

// TestHvacr03_EmitPayloadShape 은 device_state 페이로드가 lg_hvacr02 와 동일한
// 최상위 키 집합을 갖는지 확인한다. 이 형식이 깨지면 기존 플로우·대시보드가 멈춘다.
func TestHvacr03_EmitPayloadShape(t *testing.T) {
	a, _ := setupPolledAgent(t, 3, nil)

	events := drainEvents(t, a)
	if len(events) == 0 {
		t.Fatal("no device_state emitted")
	}
	ev := events[0]

	wantKeys := []string{"unit_id", "device_id", "trigger", "state", "metadata", "last_seen_ms"}
	for _, k := range wantKeys {
		if _, ok := ev[k]; !ok {
			t.Errorf("payload missing key %q", k)
		}
	}
	if len(ev) != len(wantKeys) {
		t.Errorf("payload has %d keys, want exactly %d: %v", len(ev), len(wantKeys), ev)
	}

	if ev["unit_id"] != "3" {
		t.Errorf("unit_id = %v, want \"3\"", ev["unit_id"])
	}
	if ev["trigger"] != "change" {
		t.Errorf("trigger = %v, want change", ev["trigger"])
	}

	meta, ok := ev["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata is not an object: %T", ev["metadata"])
	}
	for _, k := range []string{"name", "address", "device_type"} {
		if _, ok := meta[k]; !ok {
			t.Errorf("metadata missing key %q", k)
		}
	}

	// last_seen_ms 는 epoch milliseconds (int64) 여야 한다.
	ms, ok := ev["last_seen_ms"].(float64)
	if !ok {
		t.Fatalf("last_seen_ms is not a number: %T", ev["last_seen_ms"])
	}
	if ms < 1_600_000_000_000 || ms > 4_000_000_000_000 {
		t.Errorf("last_seen_ms = %v, does not look like epoch milliseconds", ms)
	}
}

// TestHvacr03_EmitDedupOnChange 는 상태가 같으면 change 가 방출되지 않는지 확인한다.
func TestHvacr03_EmitDedupOnChange(t *testing.T) {
	a, _ := setupPolledAgent(t, 0, nil)

	first := a.eventsEmitted.Load()
	if first == 0 {
		t.Fatal("first poll should emit a device_state")
	}

	// 상태 변화 없이 다시 폴링한다.
	a.pollAllDevices()
	if got := a.eventsEmitted.Load(); got != first {
		t.Errorf("events = %d, want %d (identical state should be deduped)", got, first)
	}
}

// TestHvacr03_ReportTriggerBypassesDedup 은 report 트리거가 dedup 을 무시하는지
// 확인한다. heartbeat 는 변경 여부와 무관하게 나가야 한다.
func TestHvacr03_ReportTriggerBypassesDedup(t *testing.T) {
	a, _ := setupPolledAgent(t, 0, nil)

	before := a.eventsEmitted.Load()
	emitted := a.emitAllDeviceStates("report")
	if emitted != 1 {
		t.Errorf("report emitted %d events, want 1", emitted)
	}
	if got := a.eventsEmitted.Load(); got != before+1 {
		t.Errorf("events = %d, want %d", got, before+1)
	}
}

// TestHvacr03_ReportEnabledGate 는 report_enabled=false 가 방출을 막는지 확인한다.
func TestHvacr03_ReportEnabledGate(t *testing.T) {
	a, gw := setupPolledAgent(t, 0, nil)

	exec(t, a, map[string]any{
		"command": "set_device",
		"address": "0",
		"params":  map[string]any{"report_enabled": false},
	})

	before := a.eventsEmitted.Load()
	gw.setHolding(0, pmbusHoldingSetTemp, 260) // 상태를 실제로 바꾼다
	a.pollAllDevices()

	if got := a.eventsEmitted.Load(); got != before {
		t.Errorf("events = %d, want %d (report_enabled=false should suppress emission)", got, before)
	}
}

// TestHvacr03_EventTempThresholdSuppresses 는 실내 온도만 미세 변동할 때
// change 방출이 억제되는지 확인한다.
func TestHvacr03_EventTempThresholdSuppresses(t *testing.T) {
	a, gw := setupPolledAgent(t, 0, map[string]any{"event_temp_threshold": 1.0})

	before := a.eventsEmitted.Load()
	gw.setInput(0, pmbusInputRoomTemp, 248) // 24.5 → 24.8, 0.3 ℃ 변화
	a.pollAllDevices()

	if got := a.eventsEmitted.Load(); got != before {
		t.Errorf("events = %d, want %d (sub-threshold temp change should be suppressed)", got, before)
	}
}

func TestHvacr03_EventTempThresholdPasses(t *testing.T) {
	a, gw := setupPolledAgent(t, 0, map[string]any{"event_temp_threshold": 1.0})

	before := a.eventsEmitted.Load()
	gw.setInput(0, pmbusInputRoomTemp, 260) // 24.5 → 26.0, 1.5 ℃ 변화
	a.pollAllDevices()

	if got := a.eventsEmitted.Load(); got != before+1 {
		t.Errorf("events = %d, want %d (1.5 ℃ change should emit)", got, before+1)
	}
}

// TestHvacr03_TemperatureThresholdDoesNotMaskOtherChanges 는 온도와 함께 다른
// 항목이 바뀌면 억제되지 않는지 확인한다.
func TestHvacr03_TemperatureThresholdDoesNotMaskOtherChanges(t *testing.T) {
	a, gw := setupPolledAgent(t, 0, map[string]any{"event_temp_threshold": 1.0})

	before := a.eventsEmitted.Load()
	gw.setInput(0, pmbusInputRoomTemp, 248)                   // 미세 온도 변화
	gw.setHolding(0, pmbusHoldingMode, uint16(pmbusModeHeat)) // + 모드 변경
	a.pollAllDevices()

	if got := a.eventsEmitted.Load(); got != before+1 {
		t.Errorf("events = %d, want %d (mode change must not be masked)", got, before+1)
	}
}

// ---------------------------------------------------------------------------
// 전원 OFF 투영 규약 (AC-047)
// ---------------------------------------------------------------------------

func TestHvacr03_PowerOffOmitsOperationalProperties(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(0, true)
	gw.setCoil(0, pmbusCoilPower, false)
	gw.setHolding(0, pmbusHoldingSetTemp, 240)
	gw.setInput(0, pmbusInputRoomTemp, 245)

	a := newTestAgent(t, gw, nil)
	a.scanDevices()
	a.pollAllDevices()

	props := a.ListDevices()[0].State.toProperties(pmbusDeviceTypeIDU, a.projectionOpts())

	if _, ok := props["target_temperature"]; ok {
		t.Error("target_temperature should be omitted when power is off")
	}
	if _, ok := props["current_temperature"]; ok {
		t.Error("current_temperature should be omitted when power is off")
	}
	if props["fan_speed"] != 0 {
		t.Errorf("fan_speed = %v, want 0 when power is off", props["fan_speed"])
	}
	if props["mode"] != 0 {
		t.Errorf("mode = %v, want 0 when power is off", props["mode"])
	}
	if props["power"] != false {
		t.Errorf("power = %v, want false", props["power"])
	}
}

// TestHvacr03_DeviceTypeGatesProjection 은 에어컨에 하이드로킷 전용 속성이
// 새어 나가지 않는지 확인한다. 미지원 레지스터도 0 으로 응답하므로 게이트가 없으면
// 대시보드에 0 °C 가 표시된다.
func TestHvacr03_DeviceTypeGatesProjection(t *testing.T) {
	a, _ := setupPolledAgent(t, 0, nil)

	props := a.ListDevices()[0].State.toProperties(pmbusDeviceTypeIDU, a.projectionOpts())
	for _, k := range []string{"water_tank_temperature_c", "solar_temperature_c", "erv_mode"} {
		if _, ok := props[k]; ok {
			t.Errorf("%s should not appear on an air-conditioner projection", k)
		}
	}
}

// ---------------------------------------------------------------------------
// 조회 명령 (AC-057 ~ AC-059)
// ---------------------------------------------------------------------------

func TestHvacr03_QueryCommands(t *testing.T) {
	a, _ := setupPolledAgent(t, 0, nil)

	commands := []string{
		"get_stats", "get_all", "request_state", "list_devices",
	}
	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			resp := exec(t, a, map[string]any{"command": cmd})
			if resp == nil {
				t.Fatal("empty response")
			}
		})
	}

	t.Run("get_state", func(t *testing.T) {
		resp := exec(t, a, map[string]any{"command": "get_state", "address": "0"})
		if resp["status"] != "ok" {
			t.Fatalf("status = %v, want ok", resp["status"])
		}
		dev, ok := resp["device"].(map[string]any)
		if !ok {
			t.Fatalf("device is not an object: %T", resp["device"])
		}
		if dev["unit_id"] != "0" {
			t.Errorf("unit_id = %v, want \"0\"", dev["unit_id"])
		}
	})

	t.Run("get_state not found", func(t *testing.T) {
		resp := exec(t, a, map[string]any{"command": "get_state", "address": "9"})
		if resp["status"] != "not_found" {
			t.Errorf("status = %v, want not_found", resp["status"])
		}
	})
}

func TestHvacr03_UnsupportedCommand(t *testing.T) {
	a, _ := setupPolledAgent(t, 0, nil)

	err := execErr(t, a, map[string]any{"command": "self_destruct"})
	if err == nil {
		t.Fatal("unsupported command should return an error")
	}
	if a.stats.Snapshot().MessagesErrored == 0 {
		t.Error("messages_errored should be incremented")
	}
}

// TestHvacr03_GetRecentLastSeqAdvances 는 get_recent 가 last_seq 를 전진시키는지
// 확인한다. 이 값이 없으면 노드가 매 조회마다 백로그를 통째로 재수신한다.
func TestHvacr03_GetRecentLastSeqAdvances(t *testing.T) {
	a, gw := setupPolledAgent(t, 0, nil)

	first := exec(t, a, map[string]any{"command": "get_recent", "count": 10})
	lastSeq, ok := first["last_seq"].(float64)
	if !ok || lastSeq == 0 {
		t.Fatalf("last_seq = %v, want a positive number", first["last_seq"])
	}

	// 같은 lastSeq 로 다시 조회하면 새 이벤트가 없어야 한다.
	second := exec(t, a, map[string]any{
		"command": "get_recent", "count": 10, "last_seq": int64(lastSeq),
	})
	if cnt := second["count"].(float64); cnt != 0 {
		t.Errorf("count = %v, want 0 (no new events since last_seq)", cnt)
	}

	// 상태를 바꾸면 새 이벤트가 나와야 한다.
	gw.setHolding(0, pmbusHoldingSetTemp, 280)
	a.pollAllDevices()
	third := exec(t, a, map[string]any{
		"command": "get_recent", "count": 10, "last_seq": int64(lastSeq),
	})
	if cnt := third["count"].(float64); cnt != 1 {
		t.Errorf("count = %v, want 1", cnt)
	}
}

// TestHvacr03_DrainIsDestructive 는 drain 이 버퍼를 비우는지 확인한다.
func TestHvacr03_DrainIsDestructive(t *testing.T) {
	a, _ := setupPolledAgent(t, 0, nil)

	first := drainEvents(t, a)
	if len(first) == 0 {
		t.Fatal("expected at least one event")
	}
	second := drainEvents(t, a)
	if len(second) != 0 {
		t.Errorf("second drain returned %d events, want 0", len(second))
	}
}

// ---------------------------------------------------------------------------
// 디바이스 관리 명령
// ---------------------------------------------------------------------------

func TestHvacr03_RemoveAndRediscover(t *testing.T) {
	a, _ := setupPolledAgent(t, 0, nil)

	exec(t, a, map[string]any{"command": "remove_device", "address": "0"})
	if len(a.ListDevices()) != 0 {
		t.Fatal("device should be removed")
	}

	// 스캔에서 다시 발견되어야 한다.
	a.scanDevices()
	if len(a.ListDevices()) != 1 {
		t.Error("device should be rediscovered by the next scan")
	}
}

func TestHvacr03_SetDeviceName(t *testing.T) {
	a, _ := setupPolledAgent(t, 0, nil)

	resp := exec(t, a, map[string]any{
		"command": "set_device",
		"address": "0",
		"params":  map[string]any{"name": "거실 에어컨"},
	})
	if resp["name"] != "거실 에어컨" {
		t.Errorf("name = %v, want 거실 에어컨", resp["name"])
	}
	if a.ListDevices()[0].Label != "거실 에어컨" {
		t.Errorf("label = %q, want 거실 에어컨", a.ListDevices()[0].Label)
	}
}

// ---------------------------------------------------------------------------
// DeviceProvider (AC-074 ~ AC-078)
// ---------------------------------------------------------------------------

func TestHvacr03_DeviceProviderReadOnly(t *testing.T) {
	a, _ := setupPolledAgent(t, 0, nil) // control_enabled 기본 false

	devices := a.DeviceProvider().Devices()
	if len(devices) != 1 {
		t.Fatalf("devices = %d, want 1", len(devices))
	}
	if _, ok := devices[0].(device.ControllableDevice); ok {
		t.Error("device should not be controllable when control_enabled is false")
	}
}

func TestHvacr03_DeviceProviderControllable(t *testing.T) {
	a, _ := setupPolledAgent(t, 0, map[string]any{"control_enabled": true})

	devices := a.DeviceProvider().Devices()
	if len(devices) != 1 {
		t.Fatalf("devices = %d, want 1", len(devices))
	}
	cd, ok := devices[0].(device.ControllableDevice)
	if !ok {
		t.Fatal("device should be controllable when control_enabled is true")
	}

	names := map[string]bool{}
	for _, spec := range cd.Commands() {
		names[spec.Name] = true
	}
	// 기본 5종 + 확장 (에어컨 대상).
	want := []string{
		"set_power", "set_mode", "set_fan_speed", "target_temperature", "set_multiple",
		"set_swing", "clear_filter_alarm", "set_lock", "set_temp_limit",
	}
	for _, n := range want {
		if !names[n] {
			t.Errorf("command %q missing from spec", n)
		}
	}
	// ERV 전용 명령은 에어컨에 노출되지 않아야 한다.
	for _, n := range []string{"set_erv_mode", "set_erv_rapid", "set_erv_eco"} {
		if names[n] {
			t.Errorf("ERV command %q should not appear on an air conditioner", n)
		}
	}
}

// TestHvacr03_ExecutorBridgesToProcess 는 어댑터 Execute 가 에이전트 Process 로
// 전달되는지 확인한다.
func TestHvacr03_ExecutorBridgesToProcess(t *testing.T) {
	a, gw := setupPolledAgent(t, 0, map[string]any{
		"control_enabled":      true,
		"control_verify_delay": "1ms",
	})

	cd := a.DeviceProvider().Devices()[0].(device.ControllableDevice)
	result, err := cd.Execute(context.Background(), "set_power", map[string]any{"power": false})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status = %v, want ok", result["status"])
	}
	if gw.getCoil(0, pmbusCoilPower) {
		t.Error("power coil should be off after the command")
	}
}
