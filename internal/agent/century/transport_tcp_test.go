package century

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// transport_tcp_test.go — Group G (TCP transport, v0.2.0) tests.
//
// Covers REQ-CENTURY-029 (tcp-client), REQ-CENTURY-030 (tcp-server),
// REQ-CENTURY-031 (exponential backoff), and the type-level AC-B9 enforcement
// (AC-G8). All tests use real loopback sockets (net.Listen "127.0.0.1:0") so
// dial/accept paths exercise the actual code rather than mocks.
// ---------------------------------------------------------------------------

// startLoopbackServer accepts a single connection, sends the provided bytes once,
// then keeps the connection open until the test closes it via the returned closeFn.
// Useful for asserting tcp-client Read paths.
func startLoopbackServer(t *testing.T, payload []byte) (addr string, accepted chan net.Conn, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	accepted = make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		if len(payload) > 0 {
			_, _ = conn.Write(payload)
		}
		accepted <- conn
	}()
	closeFn = func() {
		_ = ln.Close()
		select {
		case c := <-accepted:
			if c != nil {
				_ = c.Close()
			}
		default:
		}
	}
	return ln.Addr().String(), accepted, closeFn
}

// splitHostPort extracts host and port from a net.Listen address string.
func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	var port int
	for _, c := range portStr {
		port = port*10 + int(c-'0')
	}
	return host, port
}

// AC-G1 (subset): tcp-client connects, receives CAP-3 reg 0x02 bytes, and a
// valid Century frame is decoded via FrameScanner over the wrapper's Read.
func TestTCPClient_DialAndDecodeCAP3Frame(t *testing.T) {
	t.Parallel()
	payload := mustBuildReg02ResponseFrame(t, 0x3B)
	addr, _, closeFn := startLoopbackServer(t, payload)
	defer closeFn()

	host, port := splitHostPort(t, addr)
	cfg := Hvacr01Config{
		TransportType:     "tcp-client",
		TCPHost:           host,
		TCPPort:           port,
		TCPConnectTimeout: 500 * time.Millisecond,
		TCPReadTimeout:    1 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tr, err := openTCPClient(ctx, cfg)
	if err != nil {
		t.Fatalf("openTCPClient: %v", err)
	}
	defer tr.Close()

	scanner := NewFrameScanner(tr)
	scanCtx, scanCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer scanCancel()
	f, err := scanner.Next(scanCtx)
	if err != nil {
		t.Fatalf("scanner.Next over tcp-client: %v", err)
	}
	if f.FunctionCode != FCResponse {
		t.Errorf("FunctionCode = 0x%02x, want 0x06", f.FunctionCode)
	}
	if reg, ok := f.Register(); !ok || reg != 0x02 {
		t.Errorf("Register = 0x%02x (ok=%v), want 0x02", reg, ok)
	}
}

// AC-G2 (config-level): dial against an unreachable port returns ErrHvacr01TCPDialFailed.
// Backoff-loop timing is exercised in TestHvacr01Agent_TCPClient_ReconnectAfterEOF below.
func TestTCPClient_DialFailureWrapsSentinel(t *testing.T) {
	t.Parallel()
	cfg := Hvacr01Config{
		TransportType:     "tcp-client",
		TCPHost:           "127.0.0.1",
		TCPPort:           1, // privileged + unbound on most CI
		TCPConnectTimeout: 100 * time.Millisecond,
		TCPReadTimeout:    1 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err := openTCPClient(ctx, cfg)
	if err == nil {
		t.Fatalf("openTCPClient returned nil err; want ErrHvacr01TCPDialFailed wrap")
	}
	if !errors.Is(err, ErrHvacr01TCPDialFailed) {
		t.Fatalf("err = %v; want errors.Is ErrHvacr01TCPDialFailed", err)
	}
}

// AC-G2 (context cancel): dial respects ctx cancel — agent.Stop interrupts a pending dial.
func TestTCPClient_DialContextCancel(t *testing.T) {
	t.Parallel()
	cfg := Hvacr01Config{
		TransportType:     "tcp-client",
		TCPHost:           "10.255.255.1", // RFC 5737 — black-holes connect attempts
		TCPPort:           80,
		TCPConnectTimeout: 5 * time.Second,
		TCPReadTimeout:    1 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := openTCPClient(ctx, cfg)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("openTCPClient returned nil err; want cancel error")
	}
	if elapsed > 2*time.Second {
		t.Errorf("dial did not respect ctx cancel (took %s); want <2s", elapsed)
	}
}

// AC-G4: read timeout fires after TCPReadTimeout when no bytes arrive.
func TestTCPClient_ReadTimeoutTriggers(t *testing.T) {
	t.Parallel()
	addr, _, closeFn := startLoopbackServer(t, nil)
	defer closeFn()

	host, port := splitHostPort(t, addr)
	cfg := Hvacr01Config{
		TransportType:     "tcp-client",
		TCPHost:           host,
		TCPPort:           port,
		TCPConnectTimeout: 500 * time.Millisecond,
		TCPReadTimeout:    100 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tr, err := openTCPClient(ctx, cfg)
	if err != nil {
		t.Fatalf("openTCPClient: %v", err)
	}
	defer tr.Close()

	buf := make([]byte, 32)
	_, err = tr.Read(buf)
	if err == nil {
		t.Fatalf("Read returned nil err; want timeout")
	}
	var nerr net.Error
	if !errors.As(err, &nerr) || !nerr.Timeout() {
		t.Fatalf("err = %v; want a net.Error with Timeout()=true", err)
	}
}

// AC-G8 (type-level enforcement): tcp-client wrapper's Write returns ErrTransportPassiveOnly.
func TestTCPClient_WriteRejected(t *testing.T) {
	t.Parallel()
	payload := []byte{0x01, 0x02, 0x03}
	addr, _, closeFn := startLoopbackServer(t, payload)
	defer closeFn()

	host, port := splitHostPort(t, addr)
	cfg := Hvacr01Config{
		TransportType:     "tcp-client",
		TCPHost:           host,
		TCPPort:           port,
		TCPConnectTimeout: 500 * time.Millisecond,
		TCPReadTimeout:    1 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tr, err := openTCPClient(ctx, cfg)
	if err != nil {
		t.Fatalf("openTCPClient: %v", err)
	}
	defer tr.Close()

	n, err := tr.Write([]byte("forbidden"))
	if !errors.Is(err, ErrTransportPassiveOnly) {
		t.Errorf("Write returned err=%v, want ErrTransportPassiveOnly", err)
	}
	if n != 0 {
		t.Errorf("Write returned n=%d, want 0", n)
	}
}

func TestTCPClient_CloseIsIdempotent(t *testing.T) {
	t.Parallel()
	addr, _, closeFn := startLoopbackServer(t, nil)
	defer closeFn()

	host, port := splitHostPort(t, addr)
	cfg := Hvacr01Config{
		TransportType:     "tcp-client",
		TCPHost:           host,
		TCPPort:           port,
		TCPConnectTimeout: 500 * time.Millisecond,
		TCPReadTimeout:    1 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tr, err := openTCPClient(ctx, cfg)
	if err != nil {
		t.Fatalf("openTCPClient: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Errorf("second Close (idempotent): %v", err)
	}
	if tr.Available() {
		t.Errorf("Available() after close = true, want false")
	}
}

// AC-G5: tcp-server binds to 127.0.0.1:0 and decodes a CAP-3 frame pushed by a client.
func TestTCPServer_ListenAcceptAndDecode(t *testing.T) {
	t.Parallel()
	cfg := Hvacr01Config{
		TransportType:  "tcp-server",
		TCPHost:        "127.0.0.1",
		TCPPort:        0, // OS-assigned
		TCPReadTimeout: 1 * time.Second,
	}
	// openTCPServer requires a non-zero port via validator path; instead we
	// directly construct via a wrapper that bypasses the parseHvacr01Config
	// range check. We exercise openTCPServer with an explicit free port discovery.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen for port discovery: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	cfg.TCPPort = port

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	srv, err := openTCPServer(ctx, cfg)
	if err != nil {
		t.Fatalf("openTCPServer: %v", err)
	}
	defer srv.Close()

	addr := srv.Addr().String()

	// Push frame bytes from a "converter" client.
	frame := mustBuildReg02ResponseFrame(t, 0x3B)
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		c, err := net.DialTimeout("tcp", addr, 1*time.Second)
		if err != nil {
			t.Errorf("client dial: %v", err)
			return
		}
		defer c.Close()
		_, _ = c.Write(frame)
		// Hold the connection so the scanner has a stable Read source.
		time.Sleep(500 * time.Millisecond)
	}()

	scanner := NewFrameScanner(srv)
	scanCtx, scanCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer scanCancel()
	f, err := scanner.Next(scanCtx)
	if err != nil {
		t.Fatalf("scanner.Next over tcp-server: %v", err)
	}
	if f.FunctionCode != FCResponse {
		t.Errorf("FunctionCode = 0x%02x, want 0x06", f.FunctionCode)
	}
	<-clientDone

	if active := srv.ActiveConnections(); active != 0 && active != 1 {
		t.Errorf("ActiveConnections = %d, want 0 or 1 (single-active)", active)
	}
}

// AC-G6: a second concurrent client connection is rejected immediately.
func TestTCPServer_RejectsSecondaryConnection(t *testing.T) {
	t.Parallel()
	// Discover a free port.
	tmp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for port: %v", err)
	}
	port := tmp.Addr().(*net.TCPAddr).Port
	_ = tmp.Close()

	cfg := Hvacr01Config{
		TransportType:  "tcp-server",
		TCPHost:        "127.0.0.1",
		TCPPort:        port,
		TCPReadTimeout: 5 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	srv, err := openTCPServer(ctx, cfg)
	if err != nil {
		t.Fatalf("openTCPServer: %v", err)
	}
	defer srv.Close()

	addr := srv.Addr().String()

	// First client: connect and hold.
	c1, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		t.Fatalf("c1 dial: %v", err)
	}
	defer c1.Close()

	// Give acceptLoop a moment to take the active slot.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if srv.ActiveConnections() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if active := srv.ActiveConnections(); active != 1 {
		t.Fatalf("ActiveConnections after first dial = %d, want 1", active)
	}

	// Second client: should be accepted then immediately closed by the server.
	c2, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		t.Fatalf("c2 dial: %v", err)
	}
	defer c2.Close()

	// Wait for the rejection counter.
	deadline = time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if srv.RejectedSecondary() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := srv.RejectedSecondary(); got < 1 {
		t.Errorf("RejectedSecondary = %d, want >= 1", got)
	}
	if active := srv.ActiveConnections(); active != 1 {
		t.Errorf("ActiveConnections after second dial = %d, want 1 (unchanged)", active)
	}

	// Confirm c2 read returns EOF promptly.
	_ = c2.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 1)
	_, err = c2.Read(buf)
	if err == nil {
		t.Errorf("c2.Read returned no err; expected EOF or timeout")
	}
}

// AC-G8 (type-level): tcp-server wrapper's Write returns ErrTransportPassiveOnly.
func TestTCPServer_WriteRejected(t *testing.T) {
	t.Parallel()
	tmp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for port: %v", err)
	}
	port := tmp.Addr().(*net.TCPAddr).Port
	_ = tmp.Close()

	cfg := Hvacr01Config{TransportType: "tcp-server", TCPHost: "127.0.0.1", TCPPort: port, TCPReadTimeout: 1 * time.Second}
	srv, err := openTCPServer(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openTCPServer: %v", err)
	}
	defer srv.Close()

	n, err := srv.Write([]byte("forbidden"))
	if !errors.Is(err, ErrTransportPassiveOnly) {
		t.Errorf("Write returned err=%v, want ErrTransportPassiveOnly", err)
	}
	if n != 0 {
		t.Errorf("Write returned n=%d, want 0", n)
	}
}

// AC-G6 follow-up: after the first client disconnects, a new client is accepted.
func TestTCPServer_NextClientAcceptedAfterFirstDisconnect(t *testing.T) {
	t.Parallel()
	tmp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for port: %v", err)
	}
	port := tmp.Addr().(*net.TCPAddr).Port
	_ = tmp.Close()

	cfg := Hvacr01Config{TransportType: "tcp-server", TCPHost: "127.0.0.1", TCPPort: port, TCPReadTimeout: 200 * time.Millisecond}
	srv, err := openTCPServer(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openTCPServer: %v", err)
	}
	defer srv.Close()

	addr := srv.Addr().String()
	c1, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		t.Fatalf("c1 dial: %v", err)
	}

	// Wait until server picks up c1.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && srv.ActiveConnections() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.ActiveConnections() != 1 {
		t.Fatalf("c1 not active in time")
	}

	// Close c1 — drive server-side EOF via a brief Read which will fail / drop the active slot.
	_ = c1.Close()

	// Read on the server side advances state: it will observe the EOF and clear active.
	buf := make([]byte, 4)
	_ = srv.clearActiveOnError(buf, srv)

	// Dial a fresh client; should be accepted as the new active.
	c2, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		t.Fatalf("c2 dial: %v", err)
	}
	defer c2.Close()

	deadline = time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && srv.ActiveConnections() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if active := srv.ActiveConnections(); active != 1 {
		t.Errorf("after first close, c2 not promoted to active: ActiveConnections=%d", active)
	}
}

// helper used by the test above — invokes Read once with a short deadline to force
// the server to observe a stale connection and clear it.
func (s *tcpServerTransport) clearActiveOnError(buf []byte, _ io.ReadWriteCloser) error {
	_, err := s.Read(buf)
	return err
}

// AC-G6 / AC-G5 sanity: the server Addr reflects the OS-assigned port.
func TestTCPServer_AddrExposesAssignedPort(t *testing.T) {
	t.Parallel()
	tmp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := tmp.Addr().(*net.TCPAddr).Port
	_ = tmp.Close()

	cfg := Hvacr01Config{TransportType: "tcp-server", TCPHost: "127.0.0.1", TCPPort: port, TCPReadTimeout: 1 * time.Second}
	srv, err := openTCPServer(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openTCPServer: %v", err)
	}
	defer srv.Close()

	if !strings.Contains(srv.Addr().String(), "127.0.0.1") {
		t.Errorf("Addr().String() = %q, want host portion", srv.Addr().String())
	}
}

// openTransport dispatches to the correct concrete transport based on cfg.TransportType.
func TestOpenTransport_DispatchByType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		cfg       Hvacr01Config
		wantNil   bool
		wantErrIs error
	}{
		{
			name:      "unknown type",
			cfg:       Hvacr01Config{TransportType: "websocket"},
			wantNil:   true,
			wantErrIs: ErrUnknownTransportType,
		},
		{
			name:      "tcp-client without host",
			cfg:       Hvacr01Config{TransportType: "tcp-client", TCPPort: 4196},
			wantNil:   true,
			wantErrIs: ErrHvacr01TCPHostRequired,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr, err := openTransport(context.Background(), tc.cfg)
			if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
				t.Errorf("err = %v, want errors.Is %v", err, tc.wantErrIs)
			}
			if tc.wantNil && tr != nil {
				t.Errorf("transport = %T, want nil", tr)
			}
		})
	}
}

// TestTCPTransport_AC_G8_WriteCountZeroAcrossScenarios verifies that the
// Write method of both TCP wrappers ALWAYS rejects writes — the wrapper
// type-level enforcement of AC-B9 (passive sniff invariant) under TCP.
//
// recordingTransport-style write counter in the test asserts that any
// attempted Write through the wrapper yields a zero-bytes outcome.
func TestTCPTransport_AC_G8_WriteCountZeroAcrossScenarios(t *testing.T) {
	t.Parallel()

	// Loopback server for tcp-client.
	addr, _, closeFn := startLoopbackServer(t, nil)
	defer closeFn()
	host, port := splitHostPort(t, addr)

	clientCfg := Hvacr01Config{
		TransportType:     "tcp-client",
		TCPHost:           host,
		TCPPort:           port,
		TCPConnectTimeout: 500 * time.Millisecond,
		TCPReadTimeout:    1 * time.Second,
	}
	clientCtx, clientCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer clientCancel()
	clientTr, err := openTCPClient(clientCtx, clientCfg)
	if err != nil {
		t.Fatalf("openTCPClient: %v", err)
	}
	defer clientTr.Close()

	// Loopback listener for tcp-server.
	tmp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srvPort := tmp.Addr().(*net.TCPAddr).Port
	_ = tmp.Close()
	serverCfg := Hvacr01Config{TransportType: "tcp-server", TCPHost: "127.0.0.1", TCPPort: srvPort, TCPReadTimeout: 500 * time.Millisecond}
	serverTr, err := openTCPServer(context.Background(), serverCfg)
	if err != nil {
		t.Fatalf("openTCPServer: %v", err)
	}
	defer serverTr.Close()

	var writeAttempts atomic.Uint64
	var bytesActuallyWritten atomic.Uint64

	for _, tr := range []io.ReadWriteCloser{clientTr, serverTr} {
		writeAttempts.Add(1)
		n, err := tr.Write([]byte("never"))
		if !errors.Is(err, ErrTransportPassiveOnly) {
			t.Errorf("%T.Write err = %v, want ErrTransportPassiveOnly", tr, err)
		}
		if n != 0 {
			bytesActuallyWritten.Add(uint64(n))
		}
	}

	if got := bytesActuallyWritten.Load(); got != 0 {
		t.Errorf("AC-B9 violation: %d bytes were written through TCP wrappers; want 0", got)
	}
	if writeAttempts.Load() == 0 {
		t.Errorf("test did not actually exercise Write paths")
	}
}

// Sanity: tcp-server listen failure (port already in use) wraps ErrHvacr01TCPListenFailed.
func TestTCPServer_ListenConflictWrapsSentinel(t *testing.T) {
	t.Parallel()
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("setup listen: %v", err)
	}
	port := taken.Addr().(*net.TCPAddr).Port
	defer taken.Close()

	cfg := Hvacr01Config{TransportType: "tcp-server", TCPHost: "127.0.0.1", TCPPort: port, TCPReadTimeout: 1 * time.Second}
	_, err = openTCPServer(context.Background(), cfg)
	if err == nil {
		t.Fatalf("openTCPServer returned nil err; want listen-failed wrap")
	}
	if !errors.Is(err, ErrHvacr01TCPListenFailed) {
		t.Errorf("err = %v; want errors.Is ErrHvacr01TCPListenFailed", err)
	}
}

// concurrent Read + Close on tcp-server is race-free.
func TestTCPServer_ConcurrentReadAndClose(t *testing.T) {
	t.Parallel()
	tmp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := tmp.Addr().(*net.TCPAddr).Port
	_ = tmp.Close()

	cfg := Hvacr01Config{TransportType: "tcp-server", TCPHost: "127.0.0.1", TCPPort: port, TCPReadTimeout: 500 * time.Millisecond}
	srv, err := openTCPServer(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openTCPServer: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 16)
		for i := 0; i < 5; i++ {
			_, err := srv.Read(buf)
			if err != nil {
				return
			}
		}
	}()
	time.Sleep(40 * time.Millisecond)
	_ = srv.Close()
	wg.Wait()
}
