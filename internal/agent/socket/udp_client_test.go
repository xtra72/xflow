package socket

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// helper: valid AgentConfig for UDP client tests.
func testUDPClientConfig(port int, opts map[string]any) agent.AgentConfig {
	if opts == nil {
		opts = map[string]any{}
	}
	if _, ok := opts["port"]; !ok {
		opts["port"] = port
	}
	if _, ok := opts["host"]; !ok {
		opts["host"] = "127.0.0.1"
	}
	return agent.AgentConfig{
		ID:   "test-udp-client",
		Name: "Test UDP Client",
		Type: "udp-client",
		Transport: agent.TransportConfig{
			Type:    "udp-client",
			Options: opts,
		},
	}
}

// helper: start a UDP echo server for testing and return its address.
func startTestUDPEchoServer(t *testing.T) (*net.UDPConn, string) {
	t.Helper()
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	actualAddr := conn.LocalAddr().String()
	return conn, actualAddr
}

func TestNewUDPClientAgent_ValidConfig(t *testing.T) {
	cfg := testUDPClientConfig(9999, map[string]any{
		"host": "127.0.0.1",
		"port": 9999,
	})
	a, err := NewUDPClientAgent(cfg)
	if err != nil {
		t.Fatalf("NewUDPClientAgent: %v", err)
	}
	if a.ID() != "test-udp-client" {
		t.Errorf("ID = %q, want %q", a.ID(), "test-udp-client")
	}
	if a.Type() != "udp-client" {
		t.Errorf("Type = %q, want %q", a.Type(), "udp-client")
	}
	if a.Name() != "Test UDP Client" {
		t.Errorf("Name = %q, want %q", a.Name(), "Test UDP Client")
	}
}

func TestUDPClientAgent_StartStop(t *testing.T) {
	// Start an echo server so the client has something to connect to.
	echoConn, echoAddr := startTestUDPEchoServer(t)
	defer echoConn.Close()

	udpAddr, _ := net.ResolveUDPAddr("udp", echoAddr)
	cfg := testUDPClientConfig(udpAddr.Port, map[string]any{
		"host": "127.0.0.1",
		"port": udpAddr.Port,
	})
	a, err := NewUDPClientAgent(cfg)
	if err != nil {
		t.Fatalf("NewUDPClientAgent: %v", err)
	}

	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if err := a.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestUDPClientAgent_SendDatagram(t *testing.T) {
	echoConn, echoAddr := startTestUDPEchoServer(t)
	defer echoConn.Close()

	udpAddr, _ := net.ResolveUDPAddr("udp", echoAddr)
	cfg := testUDPClientConfig(udpAddr.Port, map[string]any{
		"host": "127.0.0.1",
		"port": udpAddr.Port,
	})
	a, err := NewUDPClientAgent(cfg)
	if err != nil {
		t.Fatalf("NewUDPClientAgent: %v", err)
	}

	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = a.Stop(ctx) }()

	time.Sleep(50 * time.Millisecond)

	// Send data via Process.
	payload := []byte("hello from client")
	cmd := map[string]any{
		"command": "send",
		"data":    base64.StdEncoding.EncodeToString(payload),
	}
	cmdBytes, _ := json.Marshal(cmd)
	if _, err := a.Process(cmdBytes); err != nil {
		t.Fatalf("Process send: %v", err)
	}

	// Read from echo server.
	buf := make([]byte, 4096)
	echoConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := echoConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP: %v", err)
	}
	if string(buf[:n]) != string(payload) {
		t.Errorf("received %q, want %q", buf[:n], payload)
	}
}

func TestUDPClientAgent_ReceiveResponse(t *testing.T) {
	echoConn, echoAddr := startTestUDPEchoServer(t)
	defer echoConn.Close()

	udpAddr, _ := net.ResolveUDPAddr("udp", echoAddr)
	cfg := testUDPClientConfig(udpAddr.Port, map[string]any{
		"host": "127.0.0.1",
		"port": udpAddr.Port,
	})
	a, err := NewUDPClientAgent(cfg)
	if err != nil {
		t.Fatalf("NewUDPClientAgent: %v", err)
	}

	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = a.Stop(ctx) }()

	time.Sleep(50 * time.Millisecond)

	// Client sends a packet so the echo server knows the client's address.
	sendCmd := map[string]any{
		"command": "send",
		"data":    base64.StdEncoding.EncodeToString([]byte("ping")),
	}
	sendBytes, _ := json.Marshal(sendCmd)
	if _, err := a.Process(sendBytes); err != nil {
		t.Fatalf("Process send: %v", err)
	}

	// Echo server reads and sends response back.
	buf := make([]byte, 4096)
	echoConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, clientAddr, err := echoConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP: %v", err)
	}

	response := []byte("pong")
	if _, err := echoConn.WriteToUDP(response, clientAddr); err != nil {
		t.Fatalf("WriteToUDP: %v", err)
	}
	_ = n

	// Client should receive the response via ReceiveMessage.
	mr := a.(agent.MessageReceiver)
	recvCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	data, err := mr.ReceiveMessage(recvCtx)
	if err != nil {
		t.Fatalf("ReceiveMessage: %v", err)
	}
	if string(data) != string(response) {
		t.Errorf("received %q, want %q", data, response)
	}
}

func TestUDPClientAgent_BufferInfo(t *testing.T) {
	echoConn, echoAddr := startTestUDPEchoServer(t)
	defer echoConn.Close()

	udpAddr, _ := net.ResolveUDPAddr("udp", echoAddr)
	cfg := testUDPClientConfig(udpAddr.Port, map[string]any{
		"host": "127.0.0.1",
		"port": udpAddr.Port,
	})
	a, err := NewUDPClientAgent(cfg)
	if err != nil {
		t.Fatalf("NewUDPClientAgent: %v", err)
	}

	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = a.Stop(ctx) }()

	bi := a.(agent.BufferInfoProvider)
	pending, capacity := bi.BufferInfo()
	if pending != 0 {
		t.Errorf("pending = %d, want 0", pending)
	}
	if capacity <= 0 {
		t.Errorf("capacity = %d, want > 0", capacity)
	}
}

func TestUDPClientAgent_Init(t *testing.T) {
	echoConn, echoAddr := startTestUDPEchoServer(t)
	defer echoConn.Close()

	udpAddr, _ := net.ResolveUDPAddr("udp", echoAddr)
	cfg := testUDPClientConfig(udpAddr.Port, map[string]any{
		"host": "127.0.0.1",
		"port": udpAddr.Port,
	})

	a, err := NewUDPClientAgent(cfg)
	if err != nil {
		t.Fatalf("NewUDPClientAgent: %v", err)
	}
	defer func() { _ = a.Stop(context.Background()) }()

	// Init on already-running agent should fail (invalid state transition).
	err = a.Init(cfg)
	if err == nil {
		t.Error("Init on already-running agent should fail")
	}
}

func TestUDPClientAgent_PauseResume(t *testing.T) {
	echoConn, echoAddr := startTestUDPEchoServer(t)
	defer echoConn.Close()

	udpAddr, _ := net.ResolveUDPAddr("udp", echoAddr)
	cfg := testUDPClientConfig(udpAddr.Port, map[string]any{
		"host": "127.0.0.1",
		"port": udpAddr.Port,
	})

	a, err := NewUDPClientAgent(cfg)
	if err != nil {
		t.Fatalf("NewUDPClientAgent: %v", err)
	}
	defer func() { _ = a.Stop(context.Background()) }()

	// Pause from Running should succeed.
	if err := a.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	// Pause again should fail.
	if err := a.Pause(context.Background()); err == nil {
		t.Error("Pause from Paused state should fail")
	}

	// Resume should succeed.
	if err := a.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	// Resume again should fail (already running).
	if err := a.Resume(context.Background()); err == nil {
		t.Error("Resume from Running state should fail")
	}
}

func TestUDPClientAgent_Health(t *testing.T) {
	echoConn, echoAddr := startTestUDPEchoServer(t)
	defer echoConn.Close()

	udpAddr, _ := net.ResolveUDPAddr("udp", echoAddr)
	cfg := testUDPClientConfig(udpAddr.Port, map[string]any{
		"host": "127.0.0.1",
		"port": udpAddr.Port,
	})

	a, err := NewUDPClientAgent(cfg)
	if err != nil {
		t.Fatalf("NewUDPClientAgent: %v", err)
	}
	defer func() { _ = a.Stop(context.Background()) }()

	// Running state should be healthy.
	h := a.Health()
	if h.Status != agent.HealthHealthy {
		t.Errorf("Health().Status = %q, want %q", h.Status, agent.HealthHealthy)
	}
	if h.Message == "" {
		t.Error("Health().Message should not be empty")
	}
	if h.LastCheck.IsZero() {
		t.Error("Health().LastCheck should not be zero")
	}

	// Paused state should be degraded.
	_ = a.Pause(context.Background())
	h = a.Health()
	if h.Status != agent.HealthDegraded {
		t.Errorf("Health().Status = %q after pause, want %q", h.Status, agent.HealthDegraded)
	}
}

func TestUDPClientAgent_Configure(t *testing.T) {
	echoConn, echoAddr := startTestUDPEchoServer(t)
	defer echoConn.Close()

	udpAddr, _ := net.ResolveUDPAddr("udp", echoAddr)
	cfg := testUDPClientConfig(udpAddr.Port, map[string]any{
		"host": "127.0.0.1",
		"port": udpAddr.Port,
	})

	a, err := NewUDPClientAgent(cfg)
	if err != nil {
		t.Fatalf("NewUDPClientAgent: %v", err)
	}
	defer func() { _ = a.Stop(context.Background()) }()

	// Valid config update.
	newCfg := testUDPClientConfig(udpAddr.Port, map[string]any{
		"host": "127.0.0.1",
		"port": udpAddr.Port,
	})
	newCfg.Name = "Updated UDP Client"
	if err := a.Configure(newCfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if a.Name() != "Updated UDP Client" {
		t.Errorf("Name after Configure = %q, want %q", a.Name(), "Updated UDP Client")
	}

	// Invalid config should return error.
	if err := a.Configure(agent.AgentConfig{}); err == nil {
		t.Error("Configure with invalid config should return error")
	}
}

func TestUDPClientAgent_Info(t *testing.T) {
	echoConn, echoAddr := startTestUDPEchoServer(t)
	defer echoConn.Close()

	udpAddr, _ := net.ResolveUDPAddr("udp", echoAddr)
	cfg := testUDPClientConfig(udpAddr.Port, map[string]any{
		"host": "127.0.0.1",
		"port": udpAddr.Port,
	})

	a, err := NewUDPClientAgent(cfg)
	if err != nil {
		t.Fatalf("NewUDPClientAgent: %v", err)
	}
	defer func() { _ = a.Stop(context.Background()) }()

	info := a.Info()
	if info.ID != "test-udp-client" {
		t.Errorf("Info().ID = %q, want %q", info.ID, "test-udp-client")
	}
	if info.Name != "Test UDP Client" {
		t.Errorf("Info().Name = %q, want %q", info.Name, "Test UDP Client")
	}
	if info.Type != "udp-client" {
		t.Errorf("Info().Type = %q, want %q", info.Type, "udp-client")
	}
	if info.Health.Status != agent.HealthHealthy {
		t.Errorf("Info().Health.Status = %q, want %q", info.Health.Status, agent.HealthHealthy)
	}
	if info.StartedAt.IsZero() {
		t.Error("Info().StartedAt should not be zero")
	}
	if info.CreatedAt.IsZero() {
		t.Error("Info().CreatedAt should not be zero")
	}
	if info.Uptime <= 0 {
		t.Error("Info().Uptime should be > 0 for running agent")
	}
}

func TestUDPClientAgent_Stats(t *testing.T) {
	echoConn, echoAddr := startTestUDPEchoServer(t)
	defer echoConn.Close()

	udpAddr, _ := net.ResolveUDPAddr("udp", echoAddr)
	cfg := testUDPClientConfig(udpAddr.Port, map[string]any{
		"host": "127.0.0.1",
		"port": udpAddr.Port,
	})

	a, err := NewUDPClientAgent(cfg)
	if err != nil {
		t.Fatalf("NewUDPClientAgent: %v", err)
	}

	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = a.Stop(ctx) }()

	time.Sleep(50 * time.Millisecond)

	// Send data to generate stats.
	payload := []byte("stats_test")
	cmd := map[string]any{
		"command": "send",
		"data":    base64.StdEncoding.EncodeToString(payload),
	}
	cmdBytes, _ := json.Marshal(cmd)
	if _, err := a.Process(cmdBytes); err != nil {
		t.Fatalf("Process send: %v", err)
	}

	stats := a.Stats()
	if stats.MessagesSent <= 0 {
		t.Error("Stats().MessagesSent should be > 0 after sending data")
	}
	if stats.BytesWritten <= 0 {
		t.Error("Stats().BytesWritten should be > 0 after sending data")
	}
	if stats.MsgBufferCapacity <= 0 {
		t.Error("Stats().MsgBufferCapacity should be > 0")
	}
}

func TestUDPClientAgent_State(t *testing.T) {
	echoConn, echoAddr := startTestUDPEchoServer(t)
	defer echoConn.Close()

	udpAddr, _ := net.ResolveUDPAddr("udp", echoAddr)
	cfg := testUDPClientConfig(udpAddr.Port, map[string]any{
		"host": "127.0.0.1",
		"port": udpAddr.Port,
	})
	a, err := NewUDPClientAgent(cfg)
	if err != nil {
		t.Fatalf("NewUDPClientAgent: %v", err)
	}

	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = a.Stop(ctx) }()

	sa := a.(agent.StatefulAgent)
	state := sa.State()

	if _, ok := state["server_addr"]; !ok {
		t.Error("state missing 'server_addr' key")
	}
	if _, ok := state["connected"]; !ok {
		t.Error("state missing 'connected' key")
	}
}
