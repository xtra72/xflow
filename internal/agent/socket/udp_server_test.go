package socket

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// helper: valid AgentConfig for UDP server tests.
func testUDPServerConfig(port int, opts map[string]any) agent.AgentConfig {
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
		ID:   "test-udp-server",
		Name: "Test UDP Server",
		Type: "udp-server",
		Transport: agent.TransportConfig{
			Type:    "udp-server",
			Options: opts,
		},
	}
}

// helper: start a UDPServerAgent on OS-assigned port and return agent + actual address.
func startTestUDPServer(t *testing.T, opts map[string]any) (agent.Agent, string) {
	t.Helper()
	if opts == nil {
		opts = map[string]any{}
	}

	// Find a free UDP port.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := pc.LocalAddr().(*net.UDPAddr).Port
	pc.Close()

	opts["port"] = port
	opts["host"] = "127.0.0.1"

	cfg := testUDPServerConfig(port, opts)
	a, err := NewUDPServerAgent(cfg)
	if err != nil {
		t.Fatalf("NewUDPServerAgent: %v", err)
	}

	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	return a, addr
}

func TestNewUDPServerAgent_ValidConfig(t *testing.T) {
	cfg := testUDPServerConfig(9999, map[string]any{
		"host": "127.0.0.1",
		"port": 9999,
	})
	a, err := NewUDPServerAgent(cfg)
	if err != nil {
		t.Fatalf("NewUDPServerAgent: %v", err)
	}
	if a.ID() != "test-udp-server" {
		t.Errorf("ID = %q, want %q", a.ID(), "test-udp-server")
	}
	if a.Type() != "udp-server" {
		t.Errorf("Type = %q, want %q", a.Type(), "udp-server")
	}
	if a.Name() != "Test UDP Server" {
		t.Errorf("Name = %q, want %q", a.Name(), "Test UDP Server")
	}
}

func TestUDPServerAgent_StartStop(t *testing.T) {
	a, addr := startTestUDPServer(t, nil)

	// Verify we can send a UDP packet (no error expected for UDP).
	udpAddr, _ := net.ResolveUDPAddr("udp", addr)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	_, _ = conn.Write([]byte("test"))
	conn.Close()

	// Stop.
	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestUDPServerAgent_ReceiveDatagram(t *testing.T) {
	a, addr := startTestUDPServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	// Send a UDP datagram.
	udpAddr, _ := net.ResolveUDPAddr("udp", addr)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	defer conn.Close()

	msg := []byte("hello udp")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	mr := a.(agent.MessageReceiver)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	data, err := mr.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessage: %v", err)
	}
	if string(data) != string(msg) {
		t.Errorf("received %q, want %q", data, msg)
	}
}

func TestUDPServerAgent_BlockIP(t *testing.T) {
	a, addr := startTestUDPServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	// Block 127.0.0.1
	cmd := map[string]any{
		"command": "block",
		"address": "127.0.0.1",
	}
	cmdBytes, _ := json.Marshal(cmd)
	if _, err := a.Process(cmdBytes); err != nil {
		t.Fatalf("Process block: %v", err)
	}

	// Send a UDP datagram from blocked IP.
	udpAddr, _ := net.ResolveUDPAddr("udp", addr)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("blocked_data")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// ReceiveMessage should timeout -- data was dropped.
	mr := a.(agent.MessageReceiver)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	_, err = mr.ReceiveMessage(ctx)
	if err == nil {
		t.Error("expected timeout for blocked IP, but got data")
	}
}

func TestUDPServerAgent_PeerTracking(t *testing.T) {
	a, addr := startTestUDPServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	// Send from two different local ports (simulating different peers).
	udpAddr, _ := net.ResolveUDPAddr("udp", addr)

	conn1, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		t.Fatalf("DialUDP 1: %v", err)
	}
	defer conn1.Close()

	conn2, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		t.Fatalf("DialUDP 2: %v", err)
	}
	defer conn2.Close()

	if _, err := conn1.Write([]byte("peer1")); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	if _, err := conn2.Write([]byte("peer2")); err != nil {
		t.Fatalf("Write 2: %v", err)
	}

	// Drain messages.
	mr := a.(agent.MessageReceiver)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = mr.ReceiveMessage(ctx)
	_, _ = mr.ReceiveMessage(ctx)

	time.Sleep(50 * time.Millisecond)

	// Verify State() shows both peers.
	sa := a.(agent.StatefulAgent)
	state := sa.State()

	peers, ok := state["peers"].([]map[string]any)
	if !ok {
		t.Fatalf("state[peers] type: %T", state["peers"])
	}
	if len(peers) < 2 {
		t.Errorf("peer count = %d, want >= 2", len(peers))
	}
}

func TestUDPServerAgent_ProcessSend(t *testing.T) {
	a, addr := startTestUDPServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	// Create a UDP socket to receive the response.
	localAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	recvConn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	defer recvConn.Close()

	// Send a datagram so the server knows about this peer.
	serverAddr, _ := net.ResolveUDPAddr("udp", addr)
	if _, err := recvConn.WriteToUDP([]byte("hello"), serverAddr); err != nil {
		t.Fatalf("WriteToUDP: %v", err)
	}

	// Drain the received message.
	mr := a.(agent.MessageReceiver)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = mr.ReceiveMessage(ctx)

	time.Sleep(50 * time.Millisecond)

	// Send data back to the peer via Process command.
	target := recvConn.LocalAddr().String()
	payload := []byte("response from server")
	cmd := map[string]any{
		"command": "send",
		"target":  target,
		"data":    base64.StdEncoding.EncodeToString(payload),
	}
	cmdBytes, _ := json.Marshal(cmd)
	if _, err := a.Process(cmdBytes); err != nil {
		t.Fatalf("Process send: %v", err)
	}

	// Read the response on the receiver.
	buf := make([]byte, 4096)
	recvConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := recvConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP: %v", err)
	}
	if string(buf[:n]) != string(payload) {
		t.Errorf("received %q, want %q", buf[:n], payload)
	}
}

func TestUDPServerAgent_ProcessListPeers(t *testing.T) {
	a, addr := startTestUDPServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	// Send from two peers.
	udpAddr, _ := net.ResolveUDPAddr("udp", addr)
	conn1, _ := net.DialUDP("udp", nil, udpAddr)
	defer conn1.Close()
	conn2, _ := net.DialUDP("udp", nil, udpAddr)
	defer conn2.Close()

	_, _ = conn1.Write([]byte("a"))
	_, _ = conn2.Write([]byte("b"))

	// Drain messages.
	mr := a.(agent.MessageReceiver)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = mr.ReceiveMessage(ctx)
	_, _ = mr.ReceiveMessage(ctx)

	time.Sleep(50 * time.Millisecond)

	cmd := map[string]any{"command": "list_peers"}
	cmdBytes, _ := json.Marshal(cmd)
	resp, err := a.Process(cmdBytes)
	if err != nil {
		t.Fatalf("Process list_peers: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	peers, ok := result["peers"].([]any)
	if !ok {
		t.Fatalf("peers type: %T", result["peers"])
	}
	if len(peers) < 2 {
		t.Errorf("peers count = %d, want >= 2", len(peers))
	}
}

func TestUDPServerAgent_BufferInfo(t *testing.T) {
	a, _ := startTestUDPServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	bi := a.(agent.BufferInfoProvider)
	pending, capacity := bi.BufferInfo()
	if pending != 0 {
		t.Errorf("pending = %d, want 0", pending)
	}
	if capacity <= 0 {
		t.Errorf("capacity = %d, want > 0", capacity)
	}
}

func TestUDPServerAgent_PauseResume(t *testing.T) {
	a, _ := startTestUDPServer(t, nil)
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

func TestUDPServerAgent_Health(t *testing.T) {
	a, _ := startTestUDPServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	// Running state.
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

	// Paused state.
	_ = a.Pause(context.Background())
	h = a.Health()
	if h.Status != agent.HealthDegraded {
		t.Errorf("Health().Status = %q after pause, want %q", h.Status, agent.HealthDegraded)
	}
}

func TestUDPServerAgent_Configure(t *testing.T) {
	a, _ := startTestUDPServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	// Valid config update.
	newCfg := testUDPServerConfig(9999, map[string]any{
		"host": "127.0.0.1",
		"port": 9999,
	})
	newCfg.Name = "Updated UDP Server"
	if err := a.Configure(newCfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if a.Name() != "Updated UDP Server" {
		t.Errorf("Name after Configure = %q, want %q", a.Name(), "Updated UDP Server")
	}

	// Invalid config should return error.
	if err := a.Configure(agent.AgentConfig{}); err == nil {
		t.Error("Configure with invalid config should return error")
	}
}

func TestUDPServerAgent_Info(t *testing.T) {
	a, _ := startTestUDPServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	info := a.Info()
	if info.ID != "test-udp-server" {
		t.Errorf("Info().ID = %q, want %q", info.ID, "test-udp-server")
	}
	if info.Name != "Test UDP Server" {
		t.Errorf("Info().Name = %q, want %q", info.Name, "Test UDP Server")
	}
	if info.Type != "udp-server" {
		t.Errorf("Info().Type = %q, want %q", info.Type, "udp-server")
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

func TestUDPServerAgent_Stats(t *testing.T) {
	a, addr := startTestUDPServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	// Send data to generate stats.
	udpAddr, _ := net.ResolveUDPAddr("udp", addr)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("stats_test")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	mr := a.(agent.MessageReceiver)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = mr.ReceiveMessage(ctx)

	stats := a.Stats()
	if stats.MessagesReceived <= 0 {
		t.Error("Stats().MessagesReceived should be > 0 after receiving data")
	}
	if stats.BytesRead <= 0 {
		t.Error("Stats().BytesRead should be > 0 after receiving data")
	}
	if stats.MsgBufferCapacity <= 0 {
		t.Error("Stats().MsgBufferCapacity should be > 0")
	}
}
