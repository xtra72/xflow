package lg

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// 생명주기 (AC-028 ~ AC-033)
// ---------------------------------------------------------------------------

// newLifecycleAgent 는 팩토리를 거쳐 만든 뒤 트랜스포트만 mock 으로 교체한다.
// 실제 Init 경로를 지나므로 생명주기 상태 전이를 그대로 검증할 수 있다.
func newLifecycleAgent(t *testing.T, gw *mockGateway, extra map[string]any) *Hvacr03Agent {
	t.Helper()

	opts := map[string]any{
		"transport_type":  "rtu",
		"serial_port":     "/dev/null",
		"poll_interval":   "5s",
		"scan_interval":   "5s",
		"report_interval": "5s",
	}
	for k, v := range extra {
		opts[k] = v
	}

	created, err := NewHvacr03Agent(agent.AgentConfig{
		ID:        "lifecycle-agent",
		Name:      "lifecycle-pmbus",
		Type:      "lg_hvacr03",
		Transport: agent.TransportConfig{Options: opts},
	})
	if err != nil {
		t.Fatalf("NewHvacr03Agent: %v", err)
	}

	a, ok := created.(*Hvacr03Agent)
	if !ok {
		t.Fatalf("factory returned %T, want *Hvacr03Agent", created)
	}
	// 팩토리가 만든 실제 시리얼 트랜스포트를 mock 으로 갈아 끼운다.
	a.transport = gw

	t.Cleanup(func() {
		_ = a.Stop(context.Background())
	})
	return a
}

// TestHvacr03_FactoryCreatesTransports 는 설정에 맞는 트랜스포트가 선택되는지
// 확인한다.
func TestHvacr03_FactoryCreatesTransports(t *testing.T) {
	cases := []struct {
		name string
		opts map[string]any
	}{
		{"rtu", map[string]any{"transport_type": "rtu", "serial_port": "/dev/null"}},
		{"tcp-client", map[string]any{"transport_type": "tcp-client", "tcp_host": "127.0.0.1", "tcp_port": 5020}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			created, err := NewHvacr03Agent(agent.AgentConfig{
				ID:        "factory-" + c.name,
				Name:      "factory-" + c.name,
				Type:      "lg_hvacr03",
				Transport: agent.TransportConfig{Options: c.opts},
			})
			if err != nil {
				t.Fatalf("NewHvacr03Agent: %v", err)
			}
			a := created.(*Hvacr03Agent)
			if a.transport == nil {
				t.Fatal("transport should not be nil")
			}
			if a.Type() != "lg_hvacr03" {
				t.Errorf("Type() = %q, want lg_hvacr03", a.Type())
			}
			if a.CurrentState() != lifecycle.StateRunning {
				t.Errorf("state = %v, want Running after Init", a.CurrentState())
			}
			_ = a.Stop(context.Background())
		})
	}
}

func TestHvacr03_FactoryRejectsBadConfig(t *testing.T) {
	_, err := NewHvacr03Agent(agent.AgentConfig{
		ID:        "bad",
		Name:      "bad",
		Type:      "lg_hvacr03",
		Transport: agent.TransportConfig{Options: map[string]any{"transport_type": "rtu"}},
	})
	if !errors.Is(err, ErrHvacr03SerialPortRequired) {
		t.Fatalf("error = %v, want ErrHvacr03SerialPortRequired", err)
	}
}

// TestHvacr03_StartIsIdempotent 는 Start 중복 호출이 백그라운드 루프를 중복
// 기동하지 않는지 확인한다. 중복 기동 시 같은 디바이스 report 가 한 틱에 여러 건
// 중복 발행된다.
func TestHvacr03_StartIsIdempotent(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(0, true)

	a := newLifecycleAgent(t, gw, nil)

	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if !a.bgStarted.Load() {
		t.Fatal("bgStarted should be true after Start")
	}

	// 두 번째 Start 는 no-op 이어야 한다.
	for i := 0; i < 3; i++ {
		if err := a.Start(context.Background()); err != nil {
			t.Fatalf("repeat Start: %v", err)
		}
	}

	// 루프가 여러 개 떴다면 스캔 트랜잭션이 급증한다. 기동 직후 1회 스캔이
	// 정상이므로, 여러 배수가 나오면 중복 기동이다.
	time.Sleep(50 * time.Millisecond)
	scans := len(gw.requestsWithFC(pmbusFCReadDiscreteInputs))
	if scans > 1 {
		t.Errorf("scan transactions = %d, want 1 (duplicate loops started)", scans)
	}
}

func TestHvacr03_StopClosesTransportAndResetsFlag(t *testing.T) {
	gw := newMockGateway()
	a := newLifecycleAgent(t, gw, nil)

	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if a.CurrentState() != lifecycle.StateStopped {
		t.Errorf("state = %v, want Stopped", a.CurrentState())
	}
	if a.bgStarted.Load() {
		t.Error("bgStarted should be reset so a later Start can restart the loops")
	}
	if a.connected.Load() {
		t.Error("connected should be false after Stop")
	}
	gw.mu.Lock()
	closed := gw.closed
	gw.mu.Unlock()
	if !closed {
		t.Error("transport should be closed")
	}

	// 중복 Stop 은 no-op.
	if err := a.Stop(context.Background()); err != nil {
		t.Errorf("repeat Stop: %v", err)
	}
}

// TestHvacr03_StartWithFailedConnectDoesNotError 는 연결 실패 시 에러 대신
// 재연결 루프로 진입하는지 확인한다. 기동 시점에 게이트웨이가 꺼져 있어도
// 에이전트는 살아 있어야 한다.
func TestHvacr03_StartWithFailedConnectDoesNotError(t *testing.T) {
	gw := newMockGateway()
	gw.connectErr = errors.New("serial: no such device")

	a := newLifecycleAgent(t, gw, map[string]any{"reconnect_interval": "10s"})

	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start should not fail on connect error: %v", err)
	}
	if a.connected.Load() {
		t.Error("connected should be false after a failed connect")
	}
}

func TestHvacr03_PauseResume(t *testing.T) {
	gw := newMockGateway()
	a := newLifecycleAgent(t, gw, nil)

	if err := a.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if !a.isPaused() {
		t.Error("agent should be paused")
	}
	if a.Health().Status != agent.HealthDegraded {
		t.Errorf("health = %v, want Degraded while paused", a.Health().Status)
	}

	if err := a.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if a.isPaused() {
		t.Error("agent should be running after Resume")
	}
}

// TestHvacr03_HealthReflectsConnection 은 실행 중이지만 게이트웨이가 끊긴 상태를
// Degraded 로 구분하는지 확인한다.
func TestHvacr03_HealthReflectsConnection(t *testing.T) {
	gw := newMockGateway()
	a := newLifecycleAgent(t, gw, nil)

	a.connected.Store(false)
	if got := a.Health().Status; got != agent.HealthDegraded {
		t.Errorf("health = %v, want Degraded when disconnected", got)
	}

	a.connected.Store(true)
	if got := a.Health().Status; got != agent.HealthHealthy {
		t.Errorf("health = %v, want Healthy when connected", got)
	}
}

func TestHvacr03_InfoAndStats(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(0, true)

	a := newLifecycleAgent(t, gw, nil)
	a.connected.Store(true) // Start 를 거치지 않으므로 연결 상태를 직접 세운다
	a.scanDevices()

	info := a.Info()
	if info.Type != "lg_hvacr03" {
		t.Errorf("Info().Type = %q, want lg_hvacr03", info.Type)
	}
	if info.Name != "lifecycle-pmbus" {
		t.Errorf("Info().Name = %q, want lifecycle-pmbus", info.Name)
	}
	if a.ID() != "lifecycle-agent" {
		t.Errorf("ID() = %q, want lifecycle-agent", a.ID())
	}

	stats := a.Stats()
	extra := stats.Extra
	for _, k := range []string{
		"events_emitted", "polls_total", "scans_total", "writes_total",
		"devices_total", "devices_online", "transport_connected",
	} {
		if _, ok := extra[k]; !ok {
			t.Errorf("stats.Extra missing key %q", k)
		}
	}
	if extra["devices_total"] != 1 {
		t.Errorf("devices_total = %v, want 1", extra["devices_total"])
	}
	if extra["devices_online"] != 1 {
		t.Errorf("devices_online = %v, want 1", extra["devices_online"])
	}

	state := a.State()
	if _, ok := state["reconnecting"]; !ok {
		t.Error("State() missing reconnecting")
	}
}

func TestHvacr03_BufferInfoAndNotifyChannel(t *testing.T) {
	gw := newMockGateway()
	a := newLifecycleAgent(t, gw, nil)

	pending, capacity := a.BufferInfo()
	if pending != 0 {
		t.Errorf("pending = %d, want 0", pending)
	}
	if capacity != 256 {
		t.Errorf("capacity = %d, want 256", capacity)
	}
	if a.FrameNotifyCh() == nil {
		t.Error("FrameNotifyCh should not be nil")
	}
	if !a.TransportConnected() {
		// Start 를 부르지 않았으므로 미연결이 정상이다.
		t.Log("transport not connected before Start (expected)")
	}
}

// TestHvacr03_ReceiveMessageDeliversEvents 는 bridge 경로로 이벤트가 전달되는지
// 확인한다.
func TestHvacr03_ReceiveMessageDeliversEvents(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(0, true)
	gw.setCoil(0, pmbusCoilPower, true)
	gw.setInput(0, pmbusInputRoomTemp, 245)

	a := newLifecycleAgent(t, gw, nil)
	a.connected.Store(true)
	a.bridgeActive.Store(true)

	a.scanDevices()
	a.pollAllDevices()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	data, err := a.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessage: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["unit_id"] != "0" {
		t.Errorf("unit_id = %v, want \"0\"", payload["unit_id"])
	}
}

// TestHvacr03_ReceiveMessageRespectsContext 는 컨텍스트 취소가 수신을 끊는지
// 확인한다.
func TestHvacr03_ReceiveMessageRespectsContext(t *testing.T) {
	gw := newMockGateway()
	a := newLifecycleAgent(t, gw, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if _, err := a.ReceiveMessage(ctx); err == nil {
		t.Error("ReceiveMessage should fail when the context expires")
	}
}

// TestHvacr03_Configure 는 런타임 재설정이 반영되는지 확인한다.
func TestHvacr03_Configure(t *testing.T) {
	gw := newMockGateway()
	a := newLifecycleAgent(t, gw, nil)

	err := a.Configure(agent.AgentConfig{
		ID:   "lifecycle-agent",
		Name: "renamed",
		Type: "lg_hvacr03",
		Transport: agent.TransportConfig{Options: map[string]any{
			"transport_type":  "rtu",
			"serial_port":     "/dev/null",
			"poll_interval":   "20s",
			"control_enabled": true,
		}},
	})
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if a.Name() != "renamed" {
		t.Errorf("Name() = %q, want renamed", a.Name())
	}

	a.mu.RLock()
	poll := a.hvacr03Config.PollInterval
	control := a.hvacr03Config.ControlEnabled
	a.mu.RUnlock()

	if poll != 20*time.Second {
		t.Errorf("poll_interval = %v, want 20s", poll)
	}
	if !control {
		t.Error("control_enabled should be true after Configure")
	}
}

func TestHvacr03_ConfigureRejectsBadOptions(t *testing.T) {
	gw := newMockGateway()
	a := newLifecycleAgent(t, gw, nil)

	err := a.Configure(agent.AgentConfig{
		ID:   "lifecycle-agent",
		Name: "lifecycle-pmbus",
		Type: "lg_hvacr03",
		Transport: agent.TransportConfig{Options: map[string]any{
			"transport_type": "rtu",
			"serial_port":    "/dev/null",
			"poll_interval":  "1s", // 하한 미달
		}},
	})
	if !errors.Is(err, ErrHvacr03PollIntervalTooShort) {
		t.Errorf("error = %v, want ErrHvacr03PollIntervalTooShort", err)
	}
}

// TestHvacr03_OfflineTimeout 은 LastSeen 기반 오프라인 전이를 확인한다.
func TestHvacr03_OfflineTimeout(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(0, true)

	a := newTestAgent(t, gw, map[string]any{"offline_timeout": "10ms"})
	a.scanDevices()
	if !a.ListDevices()[0].Online {
		t.Fatal("device should be online after scan")
	}

	time.Sleep(20 * time.Millisecond)
	a.checkDeviceTimeouts()

	if a.ListDevices()[0].Online {
		t.Error("device should be offline after the timeout elapsed")
	}
}

// TestHvacr03_DeviceStateCallbackFires 는 디바이스 변경 시 V2 콜백이 호출되는지
// 확인한다.
func TestHvacr03_DeviceStateCallbackFires(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(0, true)

	a := newTestAgent(t, gw, nil)

	fired := make(chan string, 4)
	a.SetDeviceStateChangeCallbackV2(func(agentName, uid, composite string) {
		fired <- composite
	})

	a.scanDevices()

	select {
	case got := <-fired:
		if got != "test-pmbus:0" {
			t.Errorf("composite id = %q, want test-pmbus:0", got)
		}
	case <-time.After(time.Second):
		t.Error("callback did not fire for a newly discovered device")
	}
}

// TestHvacr03_RegisterPinnedDevices 는 런타임 디바이스 등록을 확인한다.
func TestHvacr03_RegisterPinnedDevices(t *testing.T) {
	gw := newMockGateway()
	a := newTestAgent(t, gw, nil)

	enabled := false
	a.RegisterPinnedDevices([]agent.DeviceEntry{
		{Address: "4", Name: "pinned", Source: "bridge", ReportEnabled: &enabled},
		{Address: "99", Name: "invalid"}, // 범위 밖 — 무시되어야 한다
	})

	devices := a.ListDevices()
	if len(devices) != 1 {
		t.Fatalf("devices = %d, want 1 (invalid address must be skipped)", len(devices))
	}
	if devices[0].Address != "4" || devices[0].Source != "bridge" {
		t.Errorf("device = %s/%s, want 4/bridge", devices[0].Address, devices[0].Source)
	}
	if devices[0].ReportEnabled {
		t.Error("report_enabled should be false as configured")
	}
}
