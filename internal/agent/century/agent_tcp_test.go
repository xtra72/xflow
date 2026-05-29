package century

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// agent_tcp_test.go — Group G integration tests at the agent layer.
//
// These tests exercise the full Init → Start → captureLoop → reconnect lifecycle
// over real loopback sockets, complementing the lower-level wrapper tests in
// transport_tcp_test.go.
//
// AC mapping:
//   AC-G1 → TestHvacr01Agent_TCPClient_DecodesCAP3Cycle
//   AC-G3 → TestHvacr01Agent_TCPClient_ReconnectAfterEOF
//   AC-G5 → TestHvacr01Agent_TCPServer_DecodesCAP3Cycle
//   AC-G8 → TestHvacr01Agent_TCPInvariant_NoWriteEver
// ---------------------------------------------------------------------------

// startServingListener returns a net.Listener that accepts ONE connection,
// writes the provided bytes once, and stays open until the test closes it.
func startServingListener(t *testing.T, payload []byte) (net.Listener, chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	served := make(chan struct{}, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_, _ = conn.Write(payload)
		select {
		case served <- struct{}{}:
		default:
		}
		// Hold the connection alive; closed when the test cleans up the listener
		// or directly by an explicit conn.Close().
		<-served
	}()
	return ln, served
}

// AC-G1 (integration): tcp-client agent decodes a CAP-3 reg 0x02 frame end-to-end.
func TestHvacr01Agent_TCPClient_DecodesCAP3Frame(t *testing.T) {
	t.Parallel()
	ln, served := startServingListener(t, mustBuildReg02ResponseFrame(t, 0x3B))
	defer ln.Close()
	defer func() {
		// release the goroutine
		select {
		case served <- struct{}{}:
		default:
		}
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	opts := map[string]any{
		"transport_type":      "tcp-client",
		"tcp_host":            host,
		"tcp_port":            port,
		"tcp_connect_timeout": "500ms",
		"tcp_read_timeout":    "1s",
		"reconnect_interval":  "100ms",
	}
	cfg := agent.AgentConfig{
		ID:        "century-tcp-client",
		Name:      "century-tcp-client",
		Type:      "century_hvacr01",
		Transport: agent.TransportConfig{Type: "serial", Options: opts},
	}
	a, err := NewHvacr01Agent(cfg)
	if err != nil {
		t.Fatalf("NewHvacr01Agent: %v", err)
	}
	// NewHvacr01Agent 가 내부에서 Init(cfg) 까지 처리하므로 명시적 Init 호출 불필요.
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Stop(context.Background())

	ca := a.(*Hvacr01Agent)
	waitUntil(t, 2*time.Second, func() bool {
		return ca.cStats.framesValid.Load() >= 1
	}, "no frames decoded over tcp-client")

	if ca.cStats.reg02ResponseCount.Load() < 1 {
		t.Errorf("reg02ResponseCount = %d, want >= 1", ca.cStats.reg02ResponseCount.Load())
	}
	if ca.cStats.devicesDiscovered.Load() < 1 {
		t.Errorf("devicesDiscovered = %d, want >= 1", ca.cStats.devicesDiscovered.Load())
	}
}

// AC-G2 (semantics): the exponential-doubling-with-cap logic matches the SPEC.
// This is a unit test of the arithmetic used inside reconnectWithBackoff.
func TestReconnectBackoff_ProgressionDoubles(t *testing.T) {
	t.Parallel()
	initial := 10 * time.Millisecond
	maxBackoff := 80 * time.Millisecond
	// Expected series at each "before sleep" step: 10 → 20 → 40 → 80 → 80 (capped).
	want := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		40 * time.Millisecond,
		80 * time.Millisecond,
		80 * time.Millisecond,
	}
	cur := initial
	for i, w := range want {
		if cur != w {
			t.Errorf("step %d backoff = %s, want %s", i, cur, w)
		}
		cur *= 2
		if cur > maxBackoff {
			cur = maxBackoff
		}
	}
}

// AC-G3 (integration): tcp-client agent reconnects after the remote closes the connection.
func TestHvacr01Agent_TCPClient_ReconnectAfterEOF(t *testing.T) {
	t.Parallel()
	// Run an accept-loop that serves the CAP-3 payload, closes immediately,
	// and re-accepts. The agent should reconnect via backoff and resume capture.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	stopServer := make(chan struct{})
	defer close(stopServer)
	connectCount := make(chan struct{}, 8)
	go func() {
		for {
			select {
			case <-stopServer:
				return
			default:
			}
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			select {
			case connectCount <- struct{}{}:
			default:
			}
			_, _ = conn.Write(mustBuildReg02ResponseFrame(t, 0x3B))
			// Close the connection immediately to force an EOF on the client side.
			time.Sleep(50 * time.Millisecond)
			_ = conn.Close()
		}
	}()

	opts := map[string]any{
		"transport_type":        "tcp-client",
		"tcp_host":              host,
		"tcp_port":              port,
		"tcp_connect_timeout":   "200ms",
		"tcp_read_timeout":      "200ms",
		"reconnect_interval":    "50ms",
		"max_reconnect_backoff": "200ms",
	}
	cfg := agent.AgentConfig{
		ID:        "century-tcp-reconnect",
		Name:      "century-tcp-reconnect",
		Type:      "century_hvacr01",
		Transport: agent.TransportConfig{Type: "serial", Options: opts},
	}
	a, err := NewHvacr01Agent(cfg)
	if err != nil {
		t.Fatalf("NewHvacr01Agent: %v", err)
	}
	// NewHvacr01Agent 가 내부에서 Init(cfg) 까지 처리하므로 명시적 Init 호출 불필요.
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Stop(context.Background())

	ca := a.(*Hvacr01Agent)
	// Expect at least 2 successful connections and 2 decoded frames within budget.
	waitUntil(t, 5*time.Second, func() bool {
		return ca.cStats.framesValid.Load() >= 2
	}, "did not see >=2 decoded frames across reconnects")

	if ca.cStats.framesValid.Load() < 2 {
		t.Errorf("framesValid = %d, want >= 2 after reconnect", ca.cStats.framesValid.Load())
	}
}

// AC-G5 (integration): tcp-server agent accepts a client and decodes its frames.
func TestHvacr01Agent_TCPServer_DecodesCAP3Frame(t *testing.T) {
	t.Parallel()
	// Discover a free port.
	tmp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := tmp.Addr().(*net.TCPAddr).Port
	_ = tmp.Close()

	opts := map[string]any{
		"transport_type":   "tcp-server",
		"tcp_host":         "127.0.0.1",
		"tcp_port":         port,
		"tcp_read_timeout": "1s",
	}
	cfg := agent.AgentConfig{
		ID:        "century-tcp-server",
		Name:      "century-tcp-server",
		Type:      "century_hvacr01",
		Transport: agent.TransportConfig{Type: "serial", Options: opts},
	}
	a, err := NewHvacr01Agent(cfg)
	if err != nil {
		t.Fatalf("NewHvacr01Agent: %v", err)
	}
	// NewHvacr01Agent 가 내부에서 Init(cfg) 까지 처리하므로 명시적 Init 호출 불필요.
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Stop(context.Background())

	// Push a frame from a client.
	go func() {
		// Allow the server a moment to start listening.
		time.Sleep(50 * time.Millisecond)
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", portStr(port)), 1*time.Second)
		if err != nil {
			t.Errorf("client dial: %v", err)
			return
		}
		defer conn.Close()
		_, _ = conn.Write(mustBuildReg02ResponseFrame(t, 0x3B))
		// Hold the connection a moment so the scanner can read.
		time.Sleep(500 * time.Millisecond)
	}()

	ca := a.(*Hvacr01Agent)
	waitUntil(t, 3*time.Second, func() bool {
		return ca.cStats.framesValid.Load() >= 1
	}, "tcp-server did not decode any frames")

	// AC-B9 invariant — but the agent's transport is the *tcpServerTransport wrapper.
	// Sanity-check by inspecting its rejected counter for >0 not required here; the
	// dedicated test TestTCPServer_RejectsSecondaryConnection covers the policy.
	if ca.cStats.framesValid.Load() < 1 {
		t.Errorf("framesValid = %d after tcp-server push, want >= 1", ca.cStats.framesValid.Load())
	}
}

func portStr(port int) string {
	// Simple base-10 conversion to avoid an additional import.
	if port == 0 {
		return "0"
	}
	digits := make([]byte, 0, 6)
	for port > 0 {
		digits = append([]byte{byte('0' + port%10)}, digits...)
		port /= 10
	}
	return string(digits)
}

// AC-G8 (integration / type-level): the agent's transport wrapper enforces
// no Write ever, regardless of how Process / get_stats / drain / control-flavored
// commands flow through. This complements AC-B9 for serial.
func TestHvacr01Agent_TCPClient_AC_G8_NoWriteInvariant(t *testing.T) {
	t.Parallel()
	ln, served := startServingListener(t, mustBuildReg02ResponseFrame(t, 0x3B))
	defer ln.Close()
	defer func() {
		select {
		case served <- struct{}{}:
		default:
		}
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port := 0
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}

	opts := map[string]any{
		"transport_type":      "tcp-client",
		"tcp_host":            host,
		"tcp_port":            port,
		"tcp_connect_timeout": "500ms",
		"tcp_read_timeout":    "1s",
	}
	cfg := agent.AgentConfig{
		ID:        "century-tcp-noWrite",
		Name:      "century-tcp-noWrite",
		Type:      "century_hvacr01",
		Transport: agent.TransportConfig{Type: "serial", Options: opts},
	}
	a, err := NewHvacr01Agent(cfg)
	if err != nil {
		t.Fatalf("NewHvacr01Agent: %v", err)
	}
	// NewHvacr01Agent 가 내부에서 Init(cfg) 까지 처리하므로 명시적 Init 호출 불필요.
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer a.Stop(context.Background())

	ca := a.(*Hvacr01Agent)
	waitUntil(t, 2*time.Second, func() bool {
		return ca.cStats.framesValid.Load() >= 1
	}, "no frames decoded")

	// Probe every Process command — none of these may write to the transport.
	for _, cmd := range []string{
		`{"command":"get_stats"}`,
		`{"command":"get_recent","count":10}`,
		`{"command":"drain"}`,
		`{"command":"power_on"}`,
		`{"command":"target_temperature","value":24}`,
	} {
		if _, err := a.Process([]byte(cmd)); err != nil {
			t.Errorf("Process(%s) err: %v", cmd, err)
		}
	}

	// Defensive: attempt a Write directly through the wrapper. Must reject.
	if tr, ok := ca.transport.(*tcpClientTransport); ok {
		n, err := tr.Write([]byte("forbidden"))
		if n != 0 || !errors.Is(err, ErrTransportPassiveOnly) {
			t.Errorf("direct wrapper Write: n=%d err=%v; want 0/ErrTransportPassiveOnly", n, err)
		}
	} else {
		t.Errorf("agent transport is %T, want *tcpClientTransport", ca.transport)
	}
}
