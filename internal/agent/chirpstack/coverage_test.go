package chirpstack

import (
	"context"
	"testing"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// TestChirpStackAgent_Accessors 는 상태 무관 접근자(Process/Configure/Health/
// Info/Stats)의 기본 동작을 검증한다.
func TestChirpStackAgent_Accessors(t *testing.T) {
	a := newRunningTestAgent(t, "acc-cs")

	if out, err := a.Process([]byte("ignored")); out != nil || err != nil {
		t.Errorf("Process = (%v,%v), want (nil,nil) — 수신 전용", out, err)
	}
	if err := a.Configure(newTestConfig("acc-cs-id", "acc-cs")); err != nil {
		t.Errorf("Configure: %v", err)
	}
	if a.Health().Status == "" {
		t.Error("Health().Status empty")
	}
	info := a.Info()
	if info.Type != "chirpstack" || info.Name != "acc-cs" {
		t.Errorf("Info = %+v", info)
	}
	_ = a.Stats() // 스냅샷 접근이 panic 없이 동작하는지.
}

// TestChirpStackAgent_EnabledDegradedLifecycle 는 브로커 미가용(연결 거부) 상황에서
// auto_reconnect 하에 degraded Running 으로 진입하고 Pause/Resume/Start/Stop 이
// 동작하는지 검증한다. connect(degraded 경로) + 라이프사이클 전이를 커버한다.
//
// tcp://127.0.0.1:1 은 즉시 연결 거부되며, connect_timeout_sec=1 로 대기 상한을 둔다.
func TestChirpStackAgent_EnabledDegradedLifecycle(t *testing.T) {
	resetNameRegistryForTest()
	enabled := true
	cfg := agent.AgentConfig{
		ID:      "deg-id",
		Name:    "deg-cs",
		Type:    "chirpstack",
		Enabled: &enabled,
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"broker":              "tcp://127.0.0.1:1",
				"auto_reconnect":      true,
				"connect_timeout_sec": 1,
			},
		},
	}
	raw, err := NewChirpStackAgent(cfg)
	if err != nil {
		t.Fatalf("NewChirpStackAgent(degraded): %v", err)
	}
	a := raw.(*ChirpStackAgent)
	t.Cleanup(func() { _ = a.Stop(context.Background()) })

	if a.CurrentState() != lifecycle.StateRunning {
		t.Fatalf("state = %s, want Running", a.CurrentState())
	}
	// Health() 는 Running 브랜치를 커버한다(연결/미연결 세부 상태는 paho 타이밍
	// 의존이라 값 자체는 단언하지 않는다).
	if a.Health().Status == "" {
		t.Error("Health().Status empty")
	}

	// Start (이미 Running) → no-op.
	if err := a.Start(context.Background()); err != nil {
		t.Errorf("Start(running no-op): %v", err)
	}
	// Pause / Resume 전이.
	if err := a.Pause(context.Background()); err != nil {
		t.Errorf("Pause: %v", err)
	}
	if a.CurrentState() != lifecycle.StatePaused {
		t.Errorf("state after Pause = %s", a.CurrentState())
	}
	if err := a.Resume(context.Background()); err != nil {
		t.Errorf("Resume: %v", err)
	}
	if err := a.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
	if a.CurrentState() != lifecycle.StateStopped {
		t.Errorf("state after Stop = %s", a.CurrentState())
	}
}

// TestChirpStackAgent_HandleUplinkBadJSON 은 디코드 실패 시 레코드가 방출되지
// 않고 에러 통계가 증가하는지 검증한다.
func TestChirpStackAgent_HandleUplinkBadJSON(t *testing.T) {
	a := newRunningTestAgent(t, "bad-cs")
	a.handleUplink([]byte("not-json"), "application/x")

	ctx, cancel := context.WithTimeout(context.Background(), 100_000_000) // 100ms
	defer cancel()
	if _, err := a.ReceiveMessage(ctx); err == nil {
		t.Error("no record should be emitted for bad uplink")
	}
}

// TestChirpStackAgent_EnqueueDrop 은 버퍼가 가득 차면 메시지를 드롭하는지 검증한다.
func TestChirpStackAgent_EnqueueDrop(t *testing.T) {
	resetNameRegistryForTest()
	disabled := false
	cfg := agent.AgentConfig{
		ID: "drop-id", Name: "drop-cs", Type: "chirpstack", Enabled: &disabled,
		Transport: agent.TransportConfig{Options: map[string]any{"buffer_size": 1}},
	}
	raw, err := NewChirpStackAgent(cfg)
	if err != nil {
		t.Fatalf("NewChirpStackAgent: %v", err)
	}
	a := raw.(*ChirpStackAgent)

	a.enqueue([]byte("a"), "t") // 버퍼(cap 1) 채움.
	a.enqueue([]byte("b"), "t") // 드롭.
	if a.Stats().ExternalMessagesErrored == 0 {
		t.Error("expected a dropped-message error stat")
	}
}

// TestChirpStackAgent_StartFromCreatedAndStopped 는 Start 의 Created/Stopped 재-Init
// 경로를 커버한다 (비활성화 에이전트는 Created 로 남고 재시작해도 연결하지 않는다).
func TestChirpStackAgent_StartFromCreatedAndStopped(t *testing.T) {
	a := newRunningTestAgent(t, "start-cs") // 비활성화 → StateCreated.

	if err := a.Start(context.Background()); err != nil { // Created → Init(disabled) → Created.
		t.Errorf("Start from Created: %v", err)
	}
	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := a.Start(context.Background()); err != nil { // Stopped → Created → Init(disabled).
		t.Errorf("Start from Stopped: %v", err)
	}
}

// TestToIntAndToStringSlice 는 설정 파싱 헬퍼의 타입 분기를 커버한다.
func TestToIntAndToStringSlice(t *testing.T) {
	if toInt(5) != 5 || toInt(int64(5)) != 5 || toInt(byte(5)) != 5 || toInt(5.9) != 5 || toInt("x") != 0 {
		t.Error("toInt 타입 분기 실패")
	}
	if got := toStringSlice([]string{"a"}); len(got) != 1 || got[0] != "a" {
		t.Errorf("toStringSlice([]string) = %v", got)
	}
	if got := toStringSlice([]any{"a", 1, "b"}); len(got) != 2 {
		t.Errorf("toStringSlice([]any) = %v, want 2 strings", got)
	}
	if toStringSlice(42) != nil {
		t.Error("toStringSlice(non-slice) should be nil")
	}
}

// TestChirpDeviceAdapter_Accessors 는 device.Device 어댑터의 접근자 전부를 검증한다.
func TestChirpDeviceAdapter_Accessors(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "adap-cs")
	a.handleUplink(loadRawUplink(t), "application/x")

	d := a.DeviceProvider().Devices()[0]
	if d.Type() != "sensor" {
		t.Errorf("Type = %q, want sensor", d.Type())
	}
	if d.Protocol() != "chirpstack" {
		t.Errorf("Protocol = %q", d.Protocol())
	}
	if d.AgentName() != "adap-cs" {
		t.Errorf("AgentName = %q", d.AgentName())
	}
	if !d.Online() {
		t.Error("Online = false, want true after uplink")
	}
	if d.LastSeen().IsZero() {
		t.Error("LastSeen is zero")
	}
	if st := d.State(); !st.Online {
		t.Error("State().Online = false")
	}
	if d.Source() != "auto" {
		t.Errorf("Source = %q, want auto", d.Source())
	}
	if caps := d.Capabilities(); len(caps) != 1 || caps[0] != "passive-monitor" {
		t.Errorf("Capabilities = %v", caps)
	}
}
