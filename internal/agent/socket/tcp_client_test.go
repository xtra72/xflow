package socket

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// --- 테스트 헬퍼 ---

// startTestTCPServer 는 테스트용 TCP 서버를 시작한다.
// 반환된 addr 로 클라이언트가 연결할 수 있으며, closeFn 으로 서버를 종료한다.
func startTestTCPServer(t *testing.T) (addr string, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var wg sync.WaitGroup
	conns := make(chan net.Conn, 10)
	done := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-done:
					return
				default:
					return
				}
			}
			conns <- conn
		}
	}()

	closeFn = func() {
		close(done)
		ln.Close()
		wg.Wait() // accept 고루틴 종료 대기 후 채널 닫기
		close(conns)
		for c := range conns {
			c.Close()
		}
	}

	return ln.Addr().String(), closeFn
}

// startTestTCPServerWithHandler 는 연결 핸들러를 가진 테스트 TCP 서버를 시작한다.
func startTestTCPServerWithHandler(t *testing.T, handler func(net.Conn)) (addr string, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var wg sync.WaitGroup
	done := make(chan struct{})
	var connsMu sync.Mutex
	var conns []net.Conn

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			connsMu.Lock()
			conns = append(conns, conn)
			connsMu.Unlock()

			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				handler(c)
			}(conn)
		}
	}()

	closeFn = func() {
		close(done)
		ln.Close()
		connsMu.Lock()
		for _, c := range conns {
			c.Close()
		}
		connsMu.Unlock()
		wg.Wait()
	}

	return ln.Addr().String(), closeFn
}

// makeClientConfig 는 테스트용 AgentConfig 를 생성한다.
func makeClientConfig(host string, port int) agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "test-tcp-client",
		Name: "Test TCP Client",
		Type: "tcp-client",
		Transport: agent.TransportConfig{
			Type: "tcp-client",
			Options: map[string]any{
				"host":               host,
				"port":               port,
				"reconnect_interval": "50ms",
				"connect_timeout":    "100ms",
			},
		},
	}
}

// hostPort 는 addr 문자열에서 host 와 port 를 분리한다.
func hostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	var port int
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)
	return host, port
}

// --- 테스트 케이스 ---

func TestNewTCPClientAgent_ValidConfig(t *testing.T) {
	addr, closeFn := startTestTCPServer(t)
	defer closeFn()

	host, port := hostPort(t, addr)
	cfg := makeClientConfig(host, port)

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)
	require.NotNil(t, a)

	assert.Equal(t, "test-tcp-client", a.ID())
	assert.Equal(t, "Test TCP Client", a.Name())
	assert.Equal(t, "tcp-client", a.Type())
}

func TestTCPClientAgent_ConnectToServer(t *testing.T) {
	addr, closeFn := startTestTCPServer(t)
	defer closeFn()

	host, port := hostPort(t, addr)
	cfg := makeClientConfig(host, port)

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	err = a.Start(ctx)
	require.NoError(t, err)

	// 연결이 수립될 때까지 잠시 대기
	assert.Eventually(t, func() bool {
		tc, ok := a.(*TCPClientAgent)
		if !ok {
			return false
		}
		return tc.TransportConnected()
	}, time.Second, 10*time.Millisecond, "클라이언트가 서버에 연결되어야 한다")

	err = a.Stop(ctx)
	require.NoError(t, err)
}

func TestTCPClientAgent_ReceiveData(t *testing.T) {
	testData := []byte("hello from server")

	addr, closeFn := startTestTCPServerWithHandler(t, func(conn net.Conn) {
		defer conn.Close()
		_, _ = conn.Write(testData)
		// 클라이언트가 읽을 시간을 줌
		time.Sleep(500 * time.Millisecond)
	})
	defer closeFn()

	host, port := hostPort(t, addr)
	cfg := makeClientConfig(host, port)

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)

	tcpAgent := a.(*TCPClientAgent)

	ctx := context.Background()
	err = a.Start(ctx)
	require.NoError(t, err)
	defer a.Stop(ctx)

	// ReceiveMessage 로 데이터 수신
	recvCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	data, err := tcpAgent.ReceiveMessage(recvCtx)
	require.NoError(t, err)
	assert.Equal(t, testData, data)
}

func TestTCPClientAgent_SendData(t *testing.T) {
	received := make(chan []byte, 1)

	addr, closeFn := startTestTCPServerWithHandler(t, func(conn net.Conn) {
		defer conn.Close()
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		received <- buf[:n]
	})
	defer closeFn()

	host, port := hostPort(t, addr)
	cfg := makeClientConfig(host, port)

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	err = a.Start(ctx)
	require.NoError(t, err)
	defer a.Stop(ctx)

	// 연결 대기
	tcpAgent := a.(*TCPClientAgent)
	require.Eventually(t, func() bool {
		return tcpAgent.TransportConnected()
	}, time.Second, 10*time.Millisecond)

	// Process 로 데이터 전송
	sendData := []byte("hello from client")
	_, err = a.Process(sendData)
	require.NoError(t, err)

	select {
	case data := <-received:
		assert.Equal(t, sendData, data)
	case <-time.After(2 * time.Second):
		t.Fatal("서버가 데이터를 수신하지 못했다")
	}
}

func TestTCPClientAgent_Reconnect(t *testing.T) {
	// 1단계: 서버 시작 및 연결
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().String()
	host, port := hostPort(t, addr)

	// 첫 연결 수락 고루틴
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// 바로 닫아서 재연결 트리거
		time.Sleep(50 * time.Millisecond)
		conn.Close()
	}()

	cfg := makeClientConfig(host, port)
	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)

	tcpAgent := a.(*TCPClientAgent)
	ctx := context.Background()
	err = a.Start(ctx)
	require.NoError(t, err)
	defer a.Stop(ctx)

	// 연결 확인
	require.Eventually(t, func() bool {
		return tcpAgent.TransportConnected()
	}, time.Second, 10*time.Millisecond)

	// 2단계: 서버 닫기 -> 클라이언트 연결 끊김
	ln.Close()

	// 연결 끊김 확인
	require.Eventually(t, func() bool {
		return !tcpAgent.TransportConnected()
	}, 2*time.Second, 10*time.Millisecond, "연결이 끊어져야 한다")

	// 3단계: 서버 재시작 (같은 포트)
	ln2, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	defer ln2.Close()

	go func() {
		conn, _ := ln2.Accept()
		if conn != nil {
			defer conn.Close()
			time.Sleep(2 * time.Second)
		}
	}()

	// 재연결 확인
	require.Eventually(t, func() bool {
		return tcpAgent.TransportConnected()
	}, 3*time.Second, 50*time.Millisecond, "클라이언트가 재연결되어야 한다")
}

func TestTCPClientAgent_MaxRetries(t *testing.T) {
	// 연결 불가능한 포트 사용
	cfg := agent.AgentConfig{
		ID:   "test-tcp-client",
		Name: "Test TCP Client",
		Type: "tcp-client",
		Transport: agent.TransportConfig{
			Type: "tcp-client",
			Options: map[string]any{
				"host":               "127.0.0.1",
				"port":               1, // 권한 없는 포트
				"reconnect_interval": "20ms",
				"connect_timeout":    "50ms",
				"max_retries":        3,
			},
		},
	}

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)

	tcpAgent := a.(*TCPClientAgent)
	ctx := context.Background()
	err = a.Start(ctx)
	// Start 자체는 에러를 반환하지 않고 백그라운드에서 재연결 시도
	require.NoError(t, err)
	defer a.Stop(ctx)

	// 최대 재시도 후 Error 상태로 전이
	require.Eventually(t, func() bool {
		return tcpAgent.CurrentState() == lifecycle.StateError
	}, 3*time.Second, 50*time.Millisecond, "최대 재시도 후 Error 상태여야 한다")
}

func TestTCPClientAgent_InfiniteRetries(t *testing.T) {
	// max_retries=0 (무한 재시도) 으로 설정하고, 나중에 서버 시작
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	host, port := hostPort(t, addr)
	ln.Close() // 먼저 닫아서 연결 실패하게 함

	cfg := agent.AgentConfig{
		ID:   "test-tcp-client",
		Name: "Test TCP Client",
		Type: "tcp-client",
		Transport: agent.TransportConfig{
			Type: "tcp-client",
			Options: map[string]any{
				"host":               host,
				"port":               port,
				"reconnect_interval": "30ms",
				"connect_timeout":    "50ms",
				"max_retries":        0, // 무한
			},
		},
	}

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)

	tcpAgent := a.(*TCPClientAgent)
	ctx := context.Background()
	err = a.Start(ctx)
	require.NoError(t, err)
	defer a.Stop(ctx)

	// 잠시 대기 후 서버 시작
	time.Sleep(150 * time.Millisecond)

	ln2, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	defer ln2.Close()

	go func() {
		conn, _ := ln2.Accept()
		if conn != nil {
			defer conn.Close()
			time.Sleep(2 * time.Second)
		}
	}()

	// 결국 연결 성공
	require.Eventually(t, func() bool {
		return tcpAgent.TransportConnected()
	}, 3*time.Second, 50*time.Millisecond, "무한 재시도로 결국 연결되어야 한다")
}

func TestTCPClientAgent_ConnectTimeout(t *testing.T) {
	// 연결 불가능한 주소 사용 (즉시 거부 또는 타임아웃)
	cfg := agent.AgentConfig{
		ID:   "test-tcp-client",
		Name: "Test TCP Client",
		Type: "tcp-client",
		Transport: agent.TransportConfig{
			Type: "tcp-client",
			Options: map[string]any{
				"host":               "127.0.0.1",
				"port":               1, // 권한 없는 포트 -> connection refused
				"reconnect_interval": "20ms",
				"connect_timeout":    "100ms",
				"max_retries":        1,
			},
		},
	}

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)

	tcpAgent := a.(*TCPClientAgent)
	ctx := context.Background()

	err = a.Start(ctx)
	require.NoError(t, err)
	defer a.Stop(ctx)

	// max_retries=1 이므로 첫 시도 실패 후 Error 상태로 전이
	require.Eventually(t, func() bool {
		return tcpAgent.CurrentState() == lifecycle.StateError
	}, 3*time.Second, 50*time.Millisecond, "재시도 소진 후 Error 상태여야 한다")

	// 연결되지 않았음을 확인
	assert.False(t, tcpAgent.TransportConnected())
}

func TestTCPClientAgent_GracefulShutdown(t *testing.T) {
	// 서버 없이 재연결 루프에 있는 상태에서 Stop 호출
	cfg := agent.AgentConfig{
		ID:   "test-tcp-client",
		Name: "Test TCP Client",
		Type: "tcp-client",
		Transport: agent.TransportConfig{
			Type: "tcp-client",
			Options: map[string]any{
				"host":               "127.0.0.1",
				"port":               1,
				"reconnect_interval": "100ms",
				"connect_timeout":    "50ms",
				"max_retries":        0, // 무한 재시도
			},
		},
	}

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	err = a.Start(ctx)
	require.NoError(t, err)

	// 재연결 루프가 시작될 시간
	time.Sleep(100 * time.Millisecond)

	// Stop 이 빠르게 완료되어야 함
	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err = a.Stop(stopCtx)
	require.NoError(t, err)

	tcpAgent := a.(*TCPClientAgent)
	assert.Equal(t, lifecycle.StateStopped, tcpAgent.CurrentState())
}

func TestTCPClientAgent_TransportConnected(t *testing.T) {
	addr, closeFn := startTestTCPServer(t)

	host, port := hostPort(t, addr)
	cfg := makeClientConfig(host, port)

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)

	tcpAgent := a.(*TCPClientAgent)

	// 시작 전에는 연결 안 됨
	assert.False(t, tcpAgent.TransportConnected())

	ctx := context.Background()
	err = a.Start(ctx)
	require.NoError(t, err)

	// 연결 후
	require.Eventually(t, func() bool {
		return tcpAgent.TransportConnected()
	}, time.Second, 10*time.Millisecond)

	assert.True(t, tcpAgent.TransportConnected())

	// 서버 닫기
	closeFn()

	// 연결 끊김 확인
	require.Eventually(t, func() bool {
		return !tcpAgent.TransportConnected()
	}, 2*time.Second, 10*time.Millisecond)

	a.Stop(ctx)
}

func TestTCPClientAgent_FramingNewline(t *testing.T) {
	received := make(chan []byte, 10)

	addr, closeFn := startTestTCPServerWithHandler(t, func(conn net.Conn) {
		defer conn.Close()
		// 서버가 newline 프레이밍으로 메시지 전송
		_, _ = conn.Write([]byte("line1\nline2\n"))

		// 클라이언트에서 보낸 데이터 수신
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		received <- buf[:n]

		time.Sleep(500 * time.Millisecond)
	})
	defer closeFn()

	host, port := hostPort(t, addr)
	cfg := agent.AgentConfig{
		ID:   "test-tcp-client-newline",
		Name: "Test TCP Client Newline",
		Type: "tcp-client",
		Transport: agent.TransportConfig{
			Type: "tcp-client",
			Options: map[string]any{
				"host":               host,
				"port":               port,
				"framing":            "newline",
				"reconnect_interval": "50ms",
				"connect_timeout":    "100ms",
			},
		},
	}

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)

	tcpAgent := a.(*TCPClientAgent)

	ctx := context.Background()
	err = a.Start(ctx)
	require.NoError(t, err)
	defer a.Stop(ctx)

	// 첫 번째 라인 수신
	recvCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	data1, err := tcpAgent.ReceiveMessage(recvCtx)
	require.NoError(t, err)
	assert.Equal(t, []byte("line1"), data1)

	// 두 번째 라인 수신
	data2, err := tcpAgent.ReceiveMessage(recvCtx)
	require.NoError(t, err)
	assert.Equal(t, []byte("line2"), data2)

	// Process 로 newline 프레이밍 데이터 전송
	_, err = a.Process([]byte("client msg"))
	require.NoError(t, err)

	select {
	case data := <-received:
		// newline framer 는 delimiter 를 추가한다
		assert.Equal(t, []byte("client msg\n"), data)
	case <-time.After(2 * time.Second):
		t.Fatal("서버가 데이터를 수신하지 못했다")
	}
}

// --- 인터페이스 구현 확인 ---

func TestTCPClientAgent_InterfaceCompliance(t *testing.T) {
	var _ agent.Agent = (*TCPClientAgent)(nil)
	var _ agent.MessageReceiver = (*TCPClientAgent)(nil)
	var _ agent.StatefulAgent = (*TCPClientAgent)(nil)
	var _ agent.TransportChecker = (*TCPClientAgent)(nil)
	var _ agent.BufferInfoProvider = (*TCPClientAgent)(nil)
}

func TestTCPClientAgent_State(t *testing.T) {
	addr, closeFn := startTestTCPServer(t)
	defer closeFn()

	host, port := hostPort(t, addr)
	cfg := makeClientConfig(host, port)

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)

	tcpAgent := a.(*TCPClientAgent)

	ctx := context.Background()
	err = a.Start(ctx)
	require.NoError(t, err)
	defer a.Stop(ctx)

	require.Eventually(t, func() bool {
		return tcpAgent.TransportConnected()
	}, time.Second, 10*time.Millisecond)

	state := tcpAgent.State()
	assert.Equal(t, true, state["connected"])
	assert.NotEmpty(t, state["server_addr"])
}

func TestTCPClientAgent_BufferInfo(t *testing.T) {
	addr, closeFn := startTestTCPServer(t)
	defer closeFn()

	host, port := hostPort(t, addr)
	cfg := makeClientConfig(host, port)

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)

	tcpAgent := a.(*TCPClientAgent)

	pending, capacity := tcpAgent.BufferInfo()
	assert.Equal(t, 0, pending)
	assert.Greater(t, capacity, 0)
}

func TestTCPClientAgent_Init(t *testing.T) {
	addr, closeFn := startTestTCPServer(t)
	defer closeFn()

	host, port := hostPort(t, addr)
	cfg := makeClientConfig(host, port)

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)

	// Init is already called in NewTCPClientAgent; calling it again
	// on the already-running agent should fail (invalid state transition).
	tcpAgent := a.(*TCPClientAgent)
	err = tcpAgent.Init(cfg)
	assert.Error(t, err, "Init on already-running agent should fail")
}

func TestTCPClientAgent_Pause(t *testing.T) {
	addr, closeFn := startTestTCPServer(t)
	defer closeFn()

	host, port := hostPort(t, addr)
	cfg := makeClientConfig(host, port)

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	tcpAgent := a.(*TCPClientAgent)

	// Pause from Running should succeed.
	err = a.Pause(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StatePaused, tcpAgent.CurrentState())

	// Pause again should fail (already paused).
	err = a.Pause(context.Background())
	assert.Error(t, err)
}

func TestTCPClientAgent_Resume(t *testing.T) {
	addr, closeFn := startTestTCPServer(t)
	defer closeFn()

	host, port := hostPort(t, addr)
	cfg := makeClientConfig(host, port)

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	tcpAgent := a.(*TCPClientAgent)

	// Resume from Running should fail.
	err = a.Resume(context.Background())
	assert.Error(t, err, "Resume from Running state should fail")

	// Pause then Resume should succeed.
	err = a.Pause(context.Background())
	require.NoError(t, err)

	err = a.Resume(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, tcpAgent.CurrentState())
}

func TestTCPClientAgent_Health(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T) (*TCPClientAgent, func())
		wantStatus agent.HealthState
	}{
		{
			name: "running and connected returns healthy",
			setup: func(t *testing.T) (*TCPClientAgent, func()) {
				addr, closeFn := startTestTCPServer(t)
				host, port := hostPort(t, addr)
				cfg := makeClientConfig(host, port)
				a, err := NewTCPClientAgent(cfg)
				require.NoError(t, err)
				err = a.Start(context.Background())
				require.NoError(t, err)
				tcpAgent := a.(*TCPClientAgent)
				require.Eventually(t, func() bool {
					return tcpAgent.TransportConnected()
				}, time.Second, 10*time.Millisecond)
				return tcpAgent, func() { a.Stop(context.Background()); closeFn() }
			},
			wantStatus: agent.HealthHealthy,
		},
		{
			name: "running but not connected returns degraded",
			setup: func(t *testing.T) (*TCPClientAgent, func()) {
				// Use unreachable port so connection never establishes.
				cfg := makeClientConfig("127.0.0.1", 1)
				cfg.Transport.Options["max_retries"] = 0
				cfg.Transport.Options["reconnect_interval"] = "1s"
				a, err := NewTCPClientAgent(cfg)
				require.NoError(t, err)
				tcpAgent := a.(*TCPClientAgent)
				// Don't start - agent is in Running state but not connected.
				return tcpAgent, func() { a.Stop(context.Background()) }
			},
			wantStatus: agent.HealthDegraded,
		},
		{
			name: "paused returns degraded",
			setup: func(t *testing.T) (*TCPClientAgent, func()) {
				addr, closeFn := startTestTCPServer(t)
				host, port := hostPort(t, addr)
				cfg := makeClientConfig(host, port)
				a, err := NewTCPClientAgent(cfg)
				require.NoError(t, err)
				_ = a.Pause(context.Background())
				tcpAgent := a.(*TCPClientAgent)
				return tcpAgent, func() { a.Stop(context.Background()); closeFn() }
			},
			wantStatus: agent.HealthDegraded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tcpAgent, cleanup := tt.setup(t)
			defer cleanup()

			h := tcpAgent.Health()
			assert.Equal(t, tt.wantStatus, h.Status)
			assert.NotEmpty(t, h.Message)
			assert.False(t, h.LastCheck.IsZero())
		})
	}
}

func TestTCPClientAgent_Configure(t *testing.T) {
	addr, closeFn := startTestTCPServer(t)
	defer closeFn()

	host, port := hostPort(t, addr)
	cfg := makeClientConfig(host, port)

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	// Valid config update.
	newCfg := makeClientConfig(host, port)
	newCfg.Name = "Updated TCP Client"
	err = a.Configure(newCfg)
	require.NoError(t, err)
	assert.Equal(t, "Updated TCP Client", a.Name())

	// Invalid config should return error.
	err = a.Configure(agent.AgentConfig{})
	assert.Error(t, err)
}

func TestTCPClientAgent_Info(t *testing.T) {
	addr, closeFn := startTestTCPServer(t)
	defer closeFn()

	host, port := hostPort(t, addr)
	cfg := makeClientConfig(host, port)

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	tcpAgent := a.(*TCPClientAgent)
	info := tcpAgent.Info()

	assert.Equal(t, "test-tcp-client", info.ID)
	assert.Equal(t, "Test TCP Client", info.Name)
	assert.Equal(t, "tcp-client", info.Type)
	assert.Equal(t, lifecycle.StateRunning, info.State)
	assert.False(t, info.StartedAt.IsZero())
	assert.False(t, info.CreatedAt.IsZero())
	assert.Greater(t, info.Uptime, time.Duration(0))
}

func TestTCPClientAgent_ProcessNotConnected(t *testing.T) {
	// Create agent but don't start (so it's not connected).
	cfg := makeClientConfig("127.0.0.1", 1)
	cfg.Transport.Options["max_retries"] = 1
	cfg.Transport.Options["reconnect_interval"] = "1s"
	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	// Process should fail when not connected.
	_, err = a.Process([]byte("hello"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestTCPClientAgent_StatsTracking(t *testing.T) {
	received := make(chan struct{}, 1)

	addr, closeFn := startTestTCPServerWithHandler(t, func(conn net.Conn) {
		defer conn.Close()
		// 클라이언트가 보낸 데이터 수신
		buf := make([]byte, 4096)
		_, err := conn.Read(buf)
		if err != nil {
			return
		}
		received <- struct{}{}

		// 데이터 전송
		_, _ = conn.Write([]byte("response"))
		time.Sleep(500 * time.Millisecond)
	})
	defer closeFn()

	host, port := hostPort(t, addr)
	cfg := makeClientConfig(host, port)

	a, err := NewTCPClientAgent(cfg)
	require.NoError(t, err)

	tcpAgent := a.(*TCPClientAgent)

	ctx := context.Background()
	err = a.Start(ctx)
	require.NoError(t, err)
	defer a.Stop(ctx)

	require.Eventually(t, func() bool {
		return tcpAgent.TransportConnected()
	}, time.Second, 10*time.Millisecond)

	// 데이터 전송
	_, err = a.Process([]byte("request"))
	require.NoError(t, err)

	<-received

	// 수신 데이터 대기
	recvCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err = tcpAgent.ReceiveMessage(recvCtx)
	require.NoError(t, err)

	stats := a.Stats()
	assert.Greater(t, stats.MessagesSent, int64(0), "전송 메시지 수가 0보다 커야 한다")
	assert.Greater(t, stats.BytesWritten, int64(0), "전송 바이트 수가 0보다 커야 한다")
	assert.Greater(t, stats.MessagesReceived, int64(0), "수신 메시지 수가 0보다 커야 한다")
	assert.Greater(t, stats.BytesRead, int64(0), "수신 바이트 수가 0보다 커야 한다")
}
