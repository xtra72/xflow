package century

import (
	"context"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

func TestAgent_AccessorsAndMetadata(t *testing.T) {
	t.Parallel()
	a, _, cleanup := makeTestAgent(t, nil, mustBuildReg02ResponseFrame(t, 0x3B))
	defer cleanup()

	if got := a.ID(); got != "century-test" {
		t.Errorf("ID = %q, want century-test", got)
	}
	if got := a.Name(); got != "century-test" {
		t.Errorf("Name = %q, want century-test", got)
	}
	if got := a.Type(); got != "century-hvac" {
		t.Errorf("Type = %q, want century-hvac", got)
	}

	info := a.Info()
	if info.ID != "century-test" || info.Type != "century-hvac" {
		t.Errorf("Info ID/Type = %s/%s", info.ID, info.Type)
	}

	stats := a.Stats()
	if stats.Extra == nil {
		t.Errorf("Stats.Extra = nil")
	}
	if _, ok := stats.Extra["frames_captured"]; !ok {
		t.Errorf("Stats.Extra missing frames_captured")
	}

	pending, capV := a.BufferInfo()
	if capV <= 0 {
		t.Errorf("BufferInfo cap = %d, want > 0", capV)
	}
	if pending < 0 {
		t.Errorf("BufferInfo pending = %d, want >= 0", pending)
	}

	// SetDeviceStateChangeCallback should not panic.
	a.SetDeviceStateChangeCallback(func(agentName, deviceID string) {})
}

func TestAgent_HealthByState(t *testing.T) {
	t.Parallel()
	a, _, cleanup := makeTestAgent(t, nil, mustBuildReg02ResponseFrame(t, 0x3B))
	defer cleanup()

	h := a.Health()
	if h.Status != agent.HealthHealthy {
		t.Errorf("running Health = %s, want healthy", h.Status)
	}

	if err := a.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	h = a.Health()
	if h.Status != agent.HealthDegraded {
		t.Errorf("paused Health = %s, want degraded", h.Status)
	}
}

func TestAgent_ReceiveMessage_DeliversDecodedEvent(t *testing.T) {
	t.Parallel()
	// Start with empty pipe; subscribe before delivering bytes so bridgeActive=true gates pass.
	a, rt, cleanup := makeTestAgent(t, nil, nil)
	defer cleanup()

	// Subscribe in a separate goroutine to flip bridgeActive=true.
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		data, err := a.ReceiveMessage(ctx)
		ch <- result{data, err}
	}()

	// Give the subscriber a moment to set bridgeActive then deliver bytes.
	time.Sleep(20 * time.Millisecond)
	rt.deliver(mustBuildReg02ResponseFrame(t, 0x3B))

	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("ReceiveMessage err: %v", res.err)
		}
		if len(res.data) == 0 {
			t.Errorf("ReceiveMessage returned empty payload")
		}
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("ReceiveMessage timed out")
	}
}

func TestAgent_ReceiveMessage_ContextCancel(t *testing.T) {
	t.Parallel()
	a, _, cleanup := makeTestAgent(t, nil, nil)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := a.ReceiveMessage(ctx)
	if err == nil {
		t.Fatalf("ReceiveMessage with cancelled ctx returned nil err")
	}
}

func TestAgent_Configure_UpdatesConfig(t *testing.T) {
	t.Parallel()
	a, _, cleanup := makeTestAgent(t, nil, nil)
	defer cleanup()

	newCfg := agent.AgentConfig{
		ID:   "century-test",
		Name: "century-test",
		Type: "century-hvac",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port":     "/dev/ttyTEST2",
				"offline_timeout": "10s",
			},
		},
	}
	if err := a.Configure(newCfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if a.centuryConfig.OfflineTimeout != 10*time.Second {
		t.Errorf("OfflineTimeout after Configure = %s, want 10s", a.centuryConfig.OfflineTimeout)
	}
}

func TestAgent_Configure_RejectsInvalid(t *testing.T) {
	t.Parallel()
	a, _, cleanup := makeTestAgent(t, nil, nil)
	defer cleanup()

	// v0.2.0: tcp-client is now a valid transport, but missing tcp_host/tcp_port
	// still rejects (REQ-CENTURY-028).
	bad := agent.AgentConfig{
		ID:   "century-test",
		Name: "century-test",
		Type: "century-hvac",
		Transport: agent.TransportConfig{
			Type:    "serial",
			Options: map[string]any{"transport_type": "tcp-client"},
		},
	}
	if err := a.Configure(bad); err == nil {
		t.Errorf("Configure(invalid) returned nil; want error")
	}
}

// TestAgent_Configure_AcceptsTCPClient (v0.2.0): tcp-client with valid host+port is accepted.
// Replaces the previous negative case at agent_coverage_test.go:155 (REQ-CENTURY-028, M6).
func TestAgent_Configure_AcceptsTCPClient(t *testing.T) {
	t.Parallel()
	a, _, cleanup := makeTestAgent(t, nil, nil)
	defer cleanup()

	good := agent.AgentConfig{
		ID:   "century-test",
		Name: "century-test",
		Type: "century-hvac",
		Transport: agent.TransportConfig{
			Type: "serial", // Top-level transport.type is ignored when options.transport_type is set.
			Options: map[string]any{
				"transport_type": "tcp-client",
				"tcp_host":       "192.168.1.100",
				"tcp_port":       4196,
			},
		},
	}
	if err := a.Configure(good); err != nil {
		t.Fatalf("Configure(tcp-client valid) returned err: %v", err)
	}
	if a.centuryConfig.TransportType != "tcp-client" {
		t.Errorf("TransportType = %q, want tcp-client", a.centuryConfig.TransportType)
	}
	if a.centuryConfig.TCPHost != "192.168.1.100" || a.centuryConfig.TCPPort != 4196 {
		t.Errorf("TCPHost/Port = %s:%d, want 192.168.1.100:4196",
			a.centuryConfig.TCPHost, a.centuryConfig.TCPPort)
	}
}

// v0.2.0 (M6): NewCenturyAgent now wires a production transportProvider that
// dials/listens based on cfg.TransportType. Start fails at the OS layer when
// the resource is unavailable (e.g. nonexistent serial device).
func TestAgent_NewWithProductionProvider_FailsOnMissingSerial(t *testing.T) {
	t.Parallel()
	cfg := agent.AgentConfig{
		ID:   "century-noprovider",
		Name: "century-noprovider",
		Type: "century-hvac",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port": "/dev/ttyDOES-NOT-EXIST-3f7a9b",
			},
		},
	}
	a, err := NewCenturyAgent(cfg)
	if err != nil {
		t.Fatalf("NewCenturyAgent: %v", err)
	}
	if err := a.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Start should fail because the serial port does not exist.
	if err := a.Start(context.Background()); err == nil {
		t.Fatalf("Start with nonexistent serial port returned nil err, want OS-level open failure")
	}
}

func TestAgent_StatsExtraReflectsCounters(t *testing.T) {
	t.Parallel()
	frame := mustBuildReg02ResponseFrame(t, 0x3B)
	a, _, cleanup := makeTestAgent(t, nil, frame)
	defer cleanup()

	waitUntil(t, 500*time.Millisecond, func() bool {
		return a.cStats.framesValid.Load() >= 1
	}, "no frames decoded")

	s := a.Stats()
	fc, _ := s.Extra["frames_captured"].(uint64)
	if fc < 1 {
		t.Errorf("Stats.Extra frames_captured = %v, want >= 1", s.Extra["frames_captured"])
	}
}

func TestCenturyDevice_IncrementError(t *testing.T) {
	t.Parallel()
	d := NewCenturyDevice(0x3B, "auto", time.Now())
	d.IncrementError()
	d.IncrementError()
	if d.ErrorCount != 2 {
		t.Errorf("ErrorCount = %d, want 2", d.ErrorCount)
	}
}

func TestFrameRingBuffer_Capacity(t *testing.T) {
	t.Parallel()
	rb := NewFrameRingBuffer(32)
	if got := rb.Capacity(); got != 32 {
		t.Errorf("Capacity = %d, want 32", got)
	}
}

func TestParseHexOrInt_Variants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input   any
		want    int
		wantErr bool
	}{
		{int(42), 42, false},
		{int64(42), 42, false},
		{float64(42.0), 42, false},
		{"0x2A", 42, false},
		{"0X2A", 42, false},
		{"42", 42, false},
		{"bogus", 0, true},
		{[]byte("0x2A"), 0, true}, // unsupported type
	}
	for _, tc := range cases {
		tc := tc
		t.Run("", func(t *testing.T) {
			t.Parallel()
			got, err := parseHexOrInt(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("parseHexOrInt(%v) = %d, want error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("parseHexOrInt(%v) returned err %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("parseHexOrInt(%v) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseDurationValue_Variants(t *testing.T) {
	t.Parallel()
	if d, err := parseDurationValue(500 * time.Millisecond); err != nil || d != 500*time.Millisecond {
		t.Errorf("parseDurationValue(duration): got %v err %v", d, err)
	}
	if d, err := parseDurationValue("2s"); err != nil || d != 2*time.Second {
		t.Errorf("parseDurationValue(string): got %v err %v", d, err)
	}
	if _, err := parseDurationValue(42); err == nil {
		t.Errorf("parseDurationValue(int) should error")
	}
}

func TestToInt_Variants(t *testing.T) {
	t.Parallel()
	if v := toInt(int(42)); v != 42 {
		t.Errorf("toInt(int) = %d, want 42", v)
	}
	if v := toInt(int64(42)); v != 42 {
		t.Errorf("toInt(int64) = %d, want 42", v)
	}
	if v := toInt(float64(42.7)); v != 42 {
		t.Errorf("toInt(float) = %d, want 42", v)
	}
	if v := toInt("nope"); v != 0 {
		t.Errorf("toInt(string) = %d, want 0", v)
	}
}

func TestErrIsClosedOrCanceled(t *testing.T) {
	t.Parallel()
	if !errIsClosedOrCanceled(context.Canceled) {
		t.Errorf("context.Canceled should be classified as closed")
	}
	if !errIsClosedOrCanceled(context.DeadlineExceeded) {
		t.Errorf("context.DeadlineExceeded should be classified as closed")
	}
	if errIsClosedOrCanceled(nil) {
		t.Errorf("nil should not be classified as closed")
	}
}
