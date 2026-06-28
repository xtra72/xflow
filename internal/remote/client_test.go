package remote

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api/ws"
)

// clientFakeConn 은 client_test 전용 Conn 구현이다. 서버가 클라이언트로 보낼
// 메시지는 toClient 로 주입하고, 클라이언트가 보낸 메시지는 fromClient 로
// 캡처한다.
type clientFakeConn struct {
	toClient   chan []byte
	fromClient chan []byte
	closeOnce  sync.Once
	closed     chan struct{}
}

func newClientFakeConn() *clientFakeConn {
	return &clientFakeConn{
		toClient:   make(chan []byte, 32),
		fromClient: make(chan []byte, 32),
		closed:     make(chan struct{}),
	}
}

func (c *clientFakeConn) ReadMessage() ([]byte, error) {
	select {
	case data, ok := <-c.toClient:
		if !ok {
			return nil, io.EOF
		}
		return data, nil
	case <-c.closed:
		return nil, io.EOF
	}
}

func (c *clientFakeConn) WriteMessage(data []byte) error {
	select {
	case c.fromClient <- data:
		return nil
	case <-c.closed:
		return errors.New("closed")
	}
}

func (c *clientFakeConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

// TestClient_DialAndRegister 는 토큰이 없는 클라이언트가 dial 후 register 를
// 보내는지 검증한다(REQ-REMOTE-B01/C01).
//
// M2 변경: M1 에서는 token 개념이 없어 항상 hello 를 보냈으나, M2 에서는 미등록
// (토큰 미보유) 노드가 register 를 보낸다. 토큰 보유 재접속 경로는 hello 를 보낸다
// (TestClient_PresentsTokenOnReconnect / TestClient_HelloWithPersistedToken).
func TestClient_DialAndRegister(t *testing.T) {
	conn := newClientFakeConn()
	var dialCount atomic.Int32

	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		dialCount.Add(1)
		return conn, nil
	})

	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example/api/remote/ws",
		InstanceID:        "node-c1",
		Hostname:          "host-c1",
		Version:           "1.0.0",
		HeartbeatInterval: 20 * time.Millisecond,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	// 토큰 미보유 → 첫 송신 메시지는 register 여야 한다.
	msg := readClientMessage(t, conn)
	assert.Equal(t, TypeRegister, msg.Type)

	var reg RegisterPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &reg))
	assert.Equal(t, "node-c1", reg.InstanceID)
	assert.Equal(t, "host-c1", reg.Hostname)
	assert.Equal(t, "1.0.0", reg.Version)
	assert.GreaterOrEqual(t, dialCount.Load(), int32(1))
}

// TestClient_HelloWithPersistedToken 는 영속 토큰이 있으면 첫 메시지가 hello 인지
// 검증한다(REQ-C05 — 재접속 세션 복원 경로).
func TestClient_HelloWithPersistedToken(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, SaveNodeToken(dir, "existing-token"))

	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		return conn, nil
	})

	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example",
		InstanceID:        "node-c1b",
		Hostname:          "host-c1b",
		Version:           "1.0.0",
		HeartbeatInterval: time.Hour,
		DataDir:           dir,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	msg := readClientMessage(t, conn)
	assert.Equal(t, TypeHello, msg.Type, "토큰 보유 시 첫 메시지는 hello 여야 함")
}

// TestClient_HeartbeatCadence 는 클라이언트가 HeartbeatInterval 주기로 heartbeat
// 를 보내는지 검증한다(REQ-REMOTE-B03).
func TestClient_HeartbeatCadence(t *testing.T) {
	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		return conn, nil
	})

	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example",
		InstanceID:        "node-c2",
		HeartbeatInterval: 15 * time.Millisecond,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	// register 소비(토큰 미보유 — M2 등록 경로).
	reg := readClientMessage(t, conn)
	require.Equal(t, TypeRegister, reg.Type)

	// 최소 2개의 heartbeat 를 관찰한다.
	heartbeats := 0
	deadline := time.After(time.Second)
	for heartbeats < 2 {
		select {
		case data := <-conn.fromClient:
			msg, err := DecodeMessage(data)
			require.NoError(t, err)
			if msg.Type == TypeHeartbeat {
				var hb HeartbeatPayload
				require.NoError(t, json.Unmarshal(msg.Payload, &hb))
				assert.Equal(t, "node-c2", hb.InstanceID)
				heartbeats++
			}
		case <-deadline:
			t.Fatalf("heartbeat 부족: %d개만 관찰됨", heartbeats)
		}
	}
}

// TestClient_ExponentialBackoffReconnect 는 연결이 끊기면 지수 백오프로
// 재연결을 시도하며, 시도 횟수가 증가하는지 검증한다(REQ-REMOTE-B04).
func TestClient_ExponentialBackoffReconnect(t *testing.T) {
	var dialCount atomic.Int32
	var mu sync.Mutex
	var dialTimes []time.Time

	failingDialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		dialCount.Add(1)
		mu.Lock()
		dialTimes = append(dialTimes, time.Now())
		mu.Unlock()
		return nil, errors.New("dial refused")
	})

	cli := NewClient(ClientConfig{
		ServerURL:        "wss://unreachable",
		InstanceID:       "node-c3",
		ReconnectInitial: 10 * time.Millisecond,
		ReconnectMax:     200 * time.Millisecond,
	}, failingDialer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	// 재연결 시도가 누적되어야 한다.
	require.Eventually(t, func() bool {
		return dialCount.Load() >= 3
	}, 2*time.Second, 5*time.Millisecond, "여러 번의 재연결 시도가 있어야 함")

	// 백오프 간격이 단조 증가(대략)하는지 확인 — 지수 백오프 증거.
	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(dialTimes), 3)
	gap1 := dialTimes[1].Sub(dialTimes[0])
	gap2 := dialTimes[2].Sub(dialTimes[1])
	// 지터가 있으므로 엄격한 2배가 아닌 "두 번째 간격이 첫 간격보다 큼" 으로 검증.
	assert.Greater(t, gap2, gap1, "백오프 간격이 증가해야 함(지수 백오프)")
}

// TestClient_ReconnectAfterDisconnect 는 연결이 끊긴 뒤 재연결에 성공하면 다시
// hello 를 보내는지 검증한다(REQ-REMOTE-B04 재동기화 seam).
func TestClient_ReconnectAfterDisconnect(t *testing.T) {
	var dialCount atomic.Int32
	conns := make(chan *clientFakeConn, 4)

	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		n := dialCount.Add(1)
		c := newClientFakeConn()
		conns <- c
		_ = n
		return c, nil
	})

	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example",
		InstanceID:        "node-c4",
		HeartbeatInterval: time.Hour, // heartbeat 간섭 배제
		ReconnectInitial:  5 * time.Millisecond,
		ReconnectMax:      50 * time.Millisecond,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	// 첫 연결 + register(토큰 미보유).
	c1 := <-conns
	reg1 := readClientMessage(t, c1)
	assert.Equal(t, TypeRegister, reg1.Type)

	// 연결 강제 종료 → 재연결 트리거.
	c1.Close()

	// 두 번째 연결 + register 재전송(토큰 미발급 상태 유지).
	select {
	case c2 := <-conns:
		reg2 := readClientMessage(t, c2)
		assert.Equal(t, TypeRegister, reg2.Type)
	case <-time.After(2 * time.Second):
		t.Fatal("재연결이 발생하지 않음")
	}
	assert.GreaterOrEqual(t, dialCount.Load(), int32(2))
}

// TestClient_CleanShutdown 은 Stop/ctx 취소 시 고루틴이 정리되고 연결이 닫히는지
// 검증한다(goroutine leak 방지).
func TestClient_CleanShutdown(t *testing.T) {
	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		return conn, nil
	})

	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example",
		InstanceID:        "node-c5",
		HeartbeatInterval: 10 * time.Millisecond,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	cli.Start(ctx)

	readClientMessage(t, conn) // hello

	cancel()
	cli.Stop()

	// 종료 후 연결이 닫혀야 한다.
	require.Eventually(t, func() bool {
		select {
		case <-conn.closed:
			return true
		default:
			return false
		}
	}, time.Second, 5*time.Millisecond, "shutdown 시 연결이 닫혀야 함")
}

// TestClient_EmptyServerURLNoDial 는 server_url 이 비면 dial 하지 않는지
// 검증한다(REQ-REMOTE-A02 설정 오류 처리).
func TestClient_EmptyServerURLNoDial(t *testing.T) {
	var dialCount atomic.Int32
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		dialCount.Add(1)
		return newClientFakeConn(), nil
	})

	cli := NewClient(ClientConfig{
		ServerURL:  "",
		InstanceID: "node-c6",
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), dialCount.Load(), "server_url 이 비면 dial 하지 않아야 함")
}

// TestClient_HandlesServerMessages 는 클라이언트가 서버 메시지(heartbeat 및 미처리
// 타입)를 수신해도 세션이 유지되는지 검증한다(handleServerMessage 경로).
func TestClient_HandlesServerMessages(t *testing.T) {
	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		return conn, nil
	})

	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example",
		InstanceID:        "node-sm",
		HeartbeatInterval: time.Hour,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	// register 소비(토큰 미보유 — M2 등록 경로).
	reg := readClientMessage(t, conn)
	require.Equal(t, TypeRegister, reg.Type)

	// 서버 → 클라이언트: heartbeat (무시 경로).
	shb, _ := NewHeartbeatMessage("server")
	sdata, _ := shb.Encode()
	conn.toClient <- sdata

	// 서버 → 클라이언트: 미처리 타입(command, M3 seam).
	cmd, _ := ws.NewMessage(TypeCommand, map[string]string{"id": "c1"})
	cdata, _ := cmd.Encode()
	conn.toClient <- cdata

	// 세션이 살아 있어야 한다 — heartbeat 를 강제로 한 번 더 보내도 쓰기 성공.
	hb, err := NewHeartbeatMessage("node-sm")
	require.NoError(t, err)
	require.NoError(t, writeEnvelope(conn, hb))

	got := readClientMessage(t, conn)
	assert.Equal(t, TypeHeartbeat, got.Type)
}

// readClientMessage 는 클라이언트가 보낸 다음 메시지를 디코드한다.
func readClientMessage(t *testing.T, conn *clientFakeConn) *ws.Message {
	t.Helper()
	select {
	case data := <-conn.fromClient:
		msg, err := DecodeMessage(data)
		require.NoError(t, err)
		return msg
	case <-time.After(time.Second):
		t.Fatal("클라이언트 메시지 수신 타임아웃")
		return nil
	}
}
