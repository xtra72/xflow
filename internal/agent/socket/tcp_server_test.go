package socket

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// helper: valid AgentConfig for TCP server tests.
func testTCPServerConfig(port int, opts map[string]any) agent.AgentConfig {
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
		ID:   "test-tcp-server",
		Name: "Test TCP Server",
		Type: "tcp-server",
		Transport: agent.TransportConfig{
			Type:    "tcp-server",
			Options: opts,
		},
	}
}

// helper: start a TCPServerAgent on OS-assigned port and return agent + actual address.
func startTestServer(t *testing.T, opts map[string]any) (agent.Agent, string) {
	t.Helper()
	if opts == nil {
		opts = map[string]any{}
	}
	// Use port 0 for OS-assigned port -- but ParseTCPServerConfig requires port > 0.
	// We'll use a real listener to find a free port, close it, then use that port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	opts["port"] = port
	opts["host"] = "127.0.0.1"

	cfg := testTCPServerConfig(port, opts)
	a, err := NewTCPServerAgent(cfg)
	if err != nil {
		t.Fatalf("NewTCPServerAgent: %v", err)
	}

	ctx := context.Background()
	if err := a.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Small delay for listener to be ready
	time.Sleep(50 * time.Millisecond)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	return a, addr
}

// helper: connect a TCP client.
func dialTestServer(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	return conn
}

func TestNewTCPServerAgent_ValidConfig(t *testing.T) {
	cfg := testTCPServerConfig(9999, map[string]any{
		"host":            "127.0.0.1",
		"port":            9999,
		"framing":         "raw",
		"max_connections": 10,
	})
	a, err := NewTCPServerAgent(cfg)
	if err != nil {
		t.Fatalf("NewTCPServerAgent: %v", err)
	}
	if a.ID() != "test-tcp-server" {
		t.Errorf("ID = %q, want %q", a.ID(), "test-tcp-server")
	}
	if a.Type() != "tcp-server" {
		t.Errorf("Type = %q, want %q", a.Type(), "tcp-server")
	}
	if a.Name() != "Test TCP Server" {
		t.Errorf("Name = %q, want %q", a.Name(), "Test TCP Server")
	}
}

func TestTCPServerAgent_StartStop(t *testing.T) {
	a, addr := startTestServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	// Verify we can connect (listener is active).
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close()

	// Verify TransportConnected.
	tc, ok := a.(agent.TransportChecker)
	if !ok {
		t.Fatal("agent does not implement TransportChecker")
	}
	if !tc.TransportConnected() {
		t.Error("TransportConnected() = false, want true before stop")
	}

	// Stop.
	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// After stop, connection should be refused.
	_, err = net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err == nil {
		t.Error("expected connection refused after stop, got nil error")
	}
}

func TestTCPServerAgent_AcceptClient(t *testing.T) {
	a, addr := startTestServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	conn := dialTestServer(t, addr)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	sa, ok := a.(agent.StatefulAgent)
	if !ok {
		t.Fatal("agent does not implement StatefulAgent")
	}
	state := sa.State()
	conns, ok := state["connections"].([]map[string]any)
	if !ok {
		t.Fatalf("state[connections] type: %T", state["connections"])
	}
	if len(conns) != 1 {
		t.Errorf("connection count = %d, want 1", len(conns))
	}
}

func TestTCPServerAgent_ReceiveData(t *testing.T) {
	a, addr := startTestServer(t, map[string]any{"framing": "raw"})
	defer func() { _ = a.Stop(context.Background()) }()

	conn := dialTestServer(t, addr)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	msg := []byte("hello world")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	mr, ok := a.(agent.MessageReceiver)
	if !ok {
		t.Fatal("agent does not implement MessageReceiver")
	}

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

func TestTCPServerAgent_MaxConnections(t *testing.T) {
	a, addr := startTestServer(t, map[string]any{"max_connections": 2})
	defer func() { _ = a.Stop(context.Background()) }()

	conn1 := dialTestServer(t, addr)
	defer conn1.Close()
	conn2 := dialTestServer(t, addr)
	defer conn2.Close()

	time.Sleep(100 * time.Millisecond)

	// 3rd connection should be accepted at TCP level but then closed by server.
	conn3, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		// Connection refused is also acceptable.
		return
	}
	defer conn3.Close()

	// Give server time to close the connection.
	time.Sleep(100 * time.Millisecond)

	// Verify the 3rd connection was closed by reading.
	buf := make([]byte, 1)
	conn3.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, err = conn3.Read(buf)
	if err == nil {
		t.Error("3rd connection should have been closed by server")
	}
}

func TestTCPServerAgent_BlockAddress(t *testing.T) {
	a, addr := startTestServer(t, nil)
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

	time.Sleep(50 * time.Millisecond)

	// New connection should be closed by server.
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		// Connection refused is acceptable.
		return
	}
	defer conn.Close()

	time.Sleep(100 * time.Millisecond)

	buf := make([]byte, 1)
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, err = conn.Read(buf)
	if err == nil {
		t.Error("blocked connection should have been closed")
	}
}

func TestTCPServerAgent_FramingNewline(t *testing.T) {
	a, addr := startTestServer(t, map[string]any{"framing": "newline"})
	defer func() { _ = a.Stop(context.Background()) }()

	conn := dialTestServer(t, addr)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Send two newline-delimited messages in a single write.
	if _, err := conn.Write([]byte("hello\nworld\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	mr := a.(agent.MessageReceiver)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msg1, err := mr.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessage 1: %v", err)
	}
	if string(msg1) != "hello" {
		t.Errorf("msg1 = %q, want %q", msg1, "hello")
	}

	msg2, err := mr.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessage 2: %v", err)
	}
	if string(msg2) != "world" {
		t.Errorf("msg2 = %q, want %q", msg2, "world")
	}
}

func TestTCPServerAgent_FramingLengthPrefix(t *testing.T) {
	a, addr := startTestServer(t, map[string]any{"framing": "length_prefix"})
	defer func() { _ = a.Stop(context.Background()) }()

	conn := dialTestServer(t, addr)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Send length-prefixed message.
	payload := []byte("length_prefix_test")
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(payload)))
	if _, err := conn.Write(append(header, payload...)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	mr := a.(agent.MessageReceiver)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	data, err := mr.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessage: %v", err)
	}
	if string(data) != string(payload) {
		t.Errorf("received %q, want %q", data, payload)
	}
}

func TestTCPServerAgent_GracefulShutdown(t *testing.T) {
	a, addr := startTestServer(t, nil)

	// Connect multiple clients.
	conns := make([]net.Conn, 3)
	for i := range conns {
		conns[i] = dialTestServer(t, addr)
		defer conns[i].Close()
	}
	time.Sleep(50 * time.Millisecond)

	// Stop should close all connections.
	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Verify all connections are closed.
	for i, c := range conns {
		buf := make([]byte, 1)
		c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, err := c.Read(buf)
		if err == nil {
			t.Errorf("conn[%d] should be closed after stop", i)
		}
	}
}

func TestTCPServerAgent_PauseResume(t *testing.T) {
	a, addr := startTestServer(t, map[string]any{"framing": "raw"})
	defer func() { _ = a.Stop(context.Background()) }()

	conn := dialTestServer(t, addr)
	defer conn.Close()
	time.Sleep(50 * time.Millisecond)

	// Pause
	if err := a.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	// Send data while paused -- should be buffered.
	if _, err := conn.Write([]byte("paused_data")); err != nil {
		t.Fatalf("Write during pause: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// ReceiveMessage should timeout (data is buffered, not in msgCh).
	mr := a.(agent.MessageReceiver)
	ctxShort, cancelShort := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelShort()
	_, err := mr.ReceiveMessage(ctxShort)
	if err == nil {
		t.Error("expected timeout during pause, but got data")
	}

	// Resume -- should flush buffered data.
	if err := a.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	data, err := mr.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessage after resume: %v", err)
	}
	if string(data) != "paused_data" {
		t.Errorf("received %q, want %q", data, "paused_data")
	}
}

func TestTCPServerAgent_ProcessSend(t *testing.T) {
	a, addr := startTestServer(t, map[string]any{"framing": "raw"})
	defer func() { _ = a.Stop(context.Background()) }()

	conn := dialTestServer(t, addr)
	defer conn.Close()
	time.Sleep(100 * time.Millisecond)

	// Get the connection's remote address from the server's perspective.
	sa := a.(agent.StatefulAgent)
	state := sa.State()
	conns := state["connections"].([]map[string]any)
	if len(conns) == 0 {
		t.Fatal("no connections in state")
	}
	remoteAddr := conns[0]["remote_addr"].(string)

	// Send data to specific connection.
	payload := []byte("hello from server")
	cmd := map[string]any{
		"command": "send",
		"target":  remoteAddr,
		"data":    base64.StdEncoding.EncodeToString(payload),
	}
	cmdBytes, _ := json.Marshal(cmd)
	if _, err := a.Process(cmdBytes); err != nil {
		t.Fatalf("Process send: %v", err)
	}

	// Read from client.
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(buf[:n]) != string(payload) {
		t.Errorf("received %q, want %q", buf[:n], payload)
	}
}

func TestTCPServerAgent_ProcessListConnections(t *testing.T) {
	a, addr := startTestServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	conn1 := dialTestServer(t, addr)
	defer conn1.Close()
	conn2 := dialTestServer(t, addr)
	defer conn2.Close()
	time.Sleep(100 * time.Millisecond)

	cmd := map[string]any{"command": "list_connections"}
	cmdBytes, _ := json.Marshal(cmd)
	resp, err := a.Process(cmdBytes)
	if err != nil {
		t.Fatalf("Process list_connections: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	conns, ok := result["connections"].([]any)
	if !ok {
		t.Fatalf("connections type: %T", result["connections"])
	}
	if len(conns) != 2 {
		t.Errorf("connections count = %d, want 2", len(conns))
	}
}

func TestTCPServerAgent_State(t *testing.T) {
	a, addr := startTestServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	conn := dialTestServer(t, addr)
	defer conn.Close()
	time.Sleep(50 * time.Millisecond)

	sa := a.(agent.StatefulAgent)
	state := sa.State()

	if _, ok := state["connections"]; !ok {
		t.Error("state missing 'connections' key")
	}
	if _, ok := state["blocked"]; !ok {
		t.Error("state missing 'blocked' key")
	}
	if _, ok := state["listener_addr"]; !ok {
		t.Error("state missing 'listener_addr' key")
	}
}

func TestTCPServerAgent_TransportConnected(t *testing.T) {
	a, _ := startTestServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	tc := a.(agent.TransportChecker)
	if !tc.TransportConnected() {
		t.Error("TransportConnected() = false, want true")
	}

	_ = a.Stop(context.Background())
	if tc.TransportConnected() {
		t.Error("TransportConnected() = true after stop, want false")
	}
}

func TestTCPServerAgent_BufferInfo(t *testing.T) {
	a, _ := startTestServer(t, nil)
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

func TestTCPServerAgent_Health(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T) agent.Agent
		wantStatus agent.HealthState
	}{
		{
			name: "running state returns healthy",
			setup: func(t *testing.T) agent.Agent {
				a, _ := startTestServer(t, nil)
				return a
			},
			wantStatus: agent.HealthHealthy,
		},
		{
			name: "paused state returns degraded",
			setup: func(t *testing.T) agent.Agent {
				a, _ := startTestServer(t, nil)
				_ = a.Pause(context.Background())
				return a
			},
			wantStatus: agent.HealthDegraded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.setup(t)
			defer func() { _ = a.Stop(context.Background()) }()

			h := a.Health()
			if h.Status != tt.wantStatus {
				t.Errorf("Health().Status = %q, want %q", h.Status, tt.wantStatus)
			}
			if h.Message == "" {
				t.Error("Health().Message should not be empty")
			}
			if h.LastCheck.IsZero() {
				t.Error("Health().LastCheck should not be zero")
			}
		})
	}
}

func TestTCPServerAgent_Configure(t *testing.T) {
	a, _ := startTestServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	// Valid config update.
	newCfg := testTCPServerConfig(9999, map[string]any{
		"host": "127.0.0.1",
		"port": 9999,
	})
	newCfg.Name = "Updated Name"
	if err := a.Configure(newCfg); err != nil {
		t.Fatalf("Configure with valid config: %v", err)
	}
	if a.Name() != "Updated Name" {
		t.Errorf("Name after Configure = %q, want %q", a.Name(), "Updated Name")
	}

	// Invalid config (empty ID) should return error.
	invalidCfg := agent.AgentConfig{}
	if err := a.Configure(invalidCfg); err == nil {
		t.Error("Configure with invalid config should return error")
	}
}

func TestTCPServerAgent_Info(t *testing.T) {
	a, _ := startTestServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	info := a.Info()
	if info.ID != "test-tcp-server" {
		t.Errorf("Info().ID = %q, want %q", info.ID, "test-tcp-server")
	}
	if info.Name != "Test TCP Server" {
		t.Errorf("Info().Name = %q, want %q", info.Name, "Test TCP Server")
	}
	if info.Type != "tcp-server" {
		t.Errorf("Info().Type = %q, want %q", info.Type, "tcp-server")
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

func TestTCPServerAgent_Stats(t *testing.T) {
	a, addr := startTestServer(t, map[string]any{"framing": "raw"})
	defer func() { _ = a.Stop(context.Background()) }()

	// Send some data to generate stats.
	conn := dialTestServer(t, addr)
	defer conn.Close()
	time.Sleep(50 * time.Millisecond)

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

func TestTCPServerAgent_ReceiveMessageFrom(t *testing.T) {
	a, addr := startTestServer(t, map[string]any{"framing": "raw"})
	defer func() { _ = a.Stop(context.Background()) }()

	conn := dialTestServer(t, addr)
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	msg := []byte("hello from client")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	cr, ok := a.(agent.ConnAwareReceiver)
	if !ok {
		t.Fatal("agent does not implement ConnAwareReceiver")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	data, remoteAddr, err := cr.ReceiveMessageFrom(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessageFrom: %v", err)
	}
	if string(data) != string(msg) {
		t.Errorf("received data = %q, want %q", data, msg)
	}
	if remoteAddr == "" {
		t.Error("remoteAddr should not be empty")
	}

	// remoteAddr 는 클라이언트의 로컬 주소와 일치해야 한다.
	clientAddr := conn.LocalAddr().String()
	if remoteAddr != clientAddr {
		t.Errorf("remoteAddr = %q, want %q (client local addr)", remoteAddr, clientAddr)
	}
}

func TestTCPServerAgent_ProcessSendBroadcast(t *testing.T) {
	a, addr := startTestServer(t, map[string]any{"framing": "raw"})
	defer func() { _ = a.Stop(context.Background()) }()

	conn1 := dialTestServer(t, addr)
	defer conn1.Close()
	conn2 := dialTestServer(t, addr)
	defer conn2.Close()
	time.Sleep(100 * time.Millisecond)

	// Broadcast to all connections (target="").
	payload := []byte("broadcast msg")
	cmd := map[string]any{
		"command": "send",
		"target":  "",
		"data":    base64.StdEncoding.EncodeToString(payload),
	}
	cmdBytes, _ := json.Marshal(cmd)
	resp, err := a.Process(cmdBytes)
	if err != nil {
		t.Fatalf("Process broadcast: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if result["status"] != "broadcast_sent" {
		t.Errorf("status = %q, want %q", result["status"], "broadcast_sent")
	}

	// Both clients should receive the data.
	for i, c := range []net.Conn{conn1, conn2} {
		buf := make([]byte, 1024)
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := c.Read(buf)
		if err != nil {
			t.Fatalf("conn[%d] Read: %v", i, err)
		}
		if string(buf[:n]) != string(payload) {
			t.Errorf("conn[%d] received %q, want %q", i, buf[:n], payload)
		}
	}
}

// TestTCPServerAgent_BroadcastOption_ForcesAllClients 는 broadcast=true 일 때
// 특정 Target 을 지정한 send 명령도 모든 연결 클라이언트로 전송됨을 검증한다 (RED).
// broadcast 옵션이 없으면 targeted send 는 단 1개 클라이언트에만 도달하므로 실패한다.
func TestTCPServerAgent_BroadcastOption_ForcesAllClients(t *testing.T) {
	a, addr := startTestServer(t, map[string]any{"framing": "raw", "broadcast": true})
	defer func() { _ = a.Stop(context.Background()) }()

	conn1 := dialTestServer(t, addr)
	defer conn1.Close()
	conn2 := dialTestServer(t, addr)
	defer conn2.Close()
	time.Sleep(100 * time.Millisecond)

	// 한 연결의 remote_addr 를 Target 으로 지정한다.
	sa := a.(agent.StatefulAgent)
	state := sa.State()
	conns := state["connections"].([]map[string]any)
	if len(conns) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(conns))
	}
	targetAddr := conns[0]["remote_addr"].(string)

	// Target 을 명시했지만 broadcast=true 이므로 모든 클라이언트가 받아야 한다.
	payload := []byte("forced broadcast")
	cmd := map[string]any{
		"command": "send",
		"target":  targetAddr,
		"data":    base64.StdEncoding.EncodeToString(payload),
	}
	cmdBytes, _ := json.Marshal(cmd)
	resp, err := a.Process(cmdBytes)
	if err != nil {
		t.Fatalf("Process send (broadcast forced): %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if result["status"] != "broadcast_sent" {
		t.Errorf("status = %q, want %q", result["status"], "broadcast_sent")
	}

	// 두 클라이언트 모두 payload 를 받아야 한다.
	for i, c := range []net.Conn{conn1, conn2} {
		buf := make([]byte, 1024)
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := c.Read(buf)
		if err != nil {
			t.Fatalf("conn[%d] Read (broadcast forced): %v", i, err)
		}
		if string(buf[:n]) != string(payload) {
			t.Errorf("conn[%d] received %q, want %q", i, buf[:n], payload)
		}
	}
}

// TestTCPServerAgent_BroadcastDisabled_TargetedUnicast 는 broadcast=false (기본값) 일 때
// Target 지정 send 가 해당 클라이언트에만 도달하고 다른 클라이언트에는 가지 않음을 검증한다 (회귀).
func TestTCPServerAgent_BroadcastDisabled_TargetedUnicast(t *testing.T) {
	a, addr := startTestServer(t, map[string]any{"framing": "raw", "broadcast": false})
	defer func() { _ = a.Stop(context.Background()) }()

	conn1 := dialTestServer(t, addr)
	defer conn1.Close()
	conn2 := dialTestServer(t, addr)
	defer conn2.Close()
	time.Sleep(100 * time.Millisecond)

	sa := a.(agent.StatefulAgent)
	state := sa.State()
	conns := state["connections"].([]map[string]any)
	if len(conns) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(conns))
	}

	// conn1 의 로컬 주소를 서버측 remote_addr 로 식별한다.
	addr1 := conn1.LocalAddr().String()
	addr2 := conn2.LocalAddr().String()
	targetAddr := addr1

	payload := []byte("unicast only")
	cmd := map[string]any{
		"command": "send",
		"target":  targetAddr,
		"data":    base64.StdEncoding.EncodeToString(payload),
	}
	cmdBytes, _ := json.Marshal(cmd)
	resp, err := a.Process(cmdBytes)
	if err != nil {
		t.Fatalf("Process send (unicast): %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if result["status"] != "sent" {
		t.Errorf("status = %q, want %q", result["status"], "sent")
	}

	// 대상 클라이언트(conn1)는 받아야 한다.
	buf := make([]byte, 1024)
	conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn1.Read(buf)
	if err != nil {
		t.Fatalf("target conn1 Read: %v", err)
	}
	if string(buf[:n]) != string(payload) {
		t.Errorf("conn1 received %q, want %q", buf[:n], payload)
	}

	// 비대상 클라이언트(conn2)는 받지 않아야 한다 (read timeout 기대).
	conn2.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if _, err := conn2.Read(buf); err == nil {
		t.Errorf("conn2 (addr=%s) unexpectedly received data on unicast to %s", addr2, targetAddr)
	}
}

// TestParseTCPServerConfig_Broadcast 는 broadcast 옵션 파싱을 검증한다.
func TestParseTCPServerConfig_Broadcast(t *testing.T) {
	tests := []struct {
		name string
		opts map[string]any
		want bool
	}{
		{"absent defaults false", map[string]any{"port": 9000}, false},
		{"bool true", map[string]any{"port": 9000, "broadcast": true}, true},
		{"bool false", map[string]any{"port": 9000, "broadcast": false}, false},
		{"string true", map[string]any{"port": 9000, "broadcast": "true"}, true},
		{"string false", map[string]any{"port": 9000, "broadcast": "false"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseTCPServerConfig(tt.opts)
			if err != nil {
				t.Fatalf("ParseTCPServerConfig: %v", err)
			}
			if cfg.Broadcast != tt.want {
				t.Errorf("Broadcast = %v, want %v", cfg.Broadcast, tt.want)
			}
		})
	}
}

func TestTCPServerAgent_ProcessSendUnknownTarget(t *testing.T) {
	a, _ := startTestServer(t, map[string]any{"framing": "raw"})
	defer func() { _ = a.Stop(context.Background()) }()

	// Send to a non-existent connection address.
	cmd := map[string]any{
		"command": "send",
		"target":  "192.168.1.1:99999",
		"data":    base64.StdEncoding.EncodeToString([]byte("test")),
	}
	cmdBytes, _ := json.Marshal(cmd)
	_, err := a.Process(cmdBytes)
	if err == nil {
		t.Error("Process send to unknown target should return error")
	}
}

func TestTCPServerAgent_ProcessUnknownCommand(t *testing.T) {
	a, _ := startTestServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	cmd := map[string]any{"command": "invalid_cmd"}
	cmdBytes, _ := json.Marshal(cmd)
	_, err := a.Process(cmdBytes)
	if err == nil {
		t.Error("Process unknown command should return error")
	}
}

func TestTCPServerAgent_ProcessInvalidJSON(t *testing.T) {
	a, _ := startTestServer(t, nil)
	defer func() { _ = a.Stop(context.Background()) }()

	_, err := a.Process([]byte("not json"))
	if err == nil {
		t.Error("Process invalid JSON should return error")
	}
}
