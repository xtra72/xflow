package remote

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api/ws"
)

// fakeConn 은 Conn 인터페이스의 인메모리 구현으로, HTTP 없이 server 의 읽기
// 루프를 테스트한다. incoming 채널로 서버가 읽을 메시지를 주입하고, outgoing
// 으로 서버가 쓴 메시지를 캡처한다.
type fakeConn struct {
	incoming  chan []byte
	outgoing  chan []byte
	closeOnce sync.Once
	closed    chan struct{}
}

func newFakeConn() *fakeConn {
	return &fakeConn{
		incoming: make(chan []byte, 16),
		outgoing: make(chan []byte, 16),
		closed:   make(chan struct{}),
	}
}

func (f *fakeConn) ReadMessage() ([]byte, error) {
	select {
	case data, ok := <-f.incoming:
		if !ok {
			return nil, io.EOF
		}
		return data, nil
	case <-f.closed:
		return nil, io.EOF
	}
}

func (f *fakeConn) WriteMessage(data []byte) error {
	select {
	case f.outgoing <- data:
		return nil
	case <-f.closed:
		return errors.New("closed")
	}
}

func (f *fakeConn) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

// inject 는 incoming 으로 메시지를 보낸다.
func (f *fakeConn) inject(t *testing.T, msg *ws.Message) {
	t.Helper()
	data, err := msg.Encode()
	require.NoError(t, err)
	f.incoming <- data
}

// TestServer_ConnectMarksNodeOnline 는 hello 수신 시 노드가 online 으로
// 등록되는지 검증한다(REQ-REMOTE-B05).
func TestServer_ConnectMarksNodeOnline(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.HandleConnection(ctx, conn)
	}()

	hello, err := NewHelloMessage(HelloPayload{InstanceID: "node-1", Hostname: "h", Version: "v"})
	require.NoError(t, err)
	conn.inject(t, hello)

	require.Eventually(t, func() bool {
		st, ok := srv.NodeState("node-1")
		return ok && st.Online
	}, time.Second, 5*time.Millisecond, "hello 수신 후 노드는 online 이어야 함")

	cancel()
	<-done
}

// TestServer_HeartbeatRefreshesLastSeen 는 heartbeat 가 last_seen 을 갱신하는지
// 검증한다(REQ-REMOTE-B03/B05).
func TestServer_HeartbeatRefreshesLastSeen(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	hello, _ := NewHelloMessage(HelloPayload{InstanceID: "node-2"})
	conn.inject(t, hello)

	require.Eventually(t, func() bool {
		_, ok := srv.NodeState("node-2")
		return ok
	}, time.Second, 5*time.Millisecond)

	first, _ := srv.NodeState("node-2")
	time.Sleep(10 * time.Millisecond)

	hb, _ := NewHeartbeatMessage("node-2")
	conn.inject(t, hb)

	require.Eventually(t, func() bool {
		st, ok := srv.NodeState("node-2")
		return ok && st.LastSeen.After(first.LastSeen)
	}, time.Second, 5*time.Millisecond, "heartbeat 는 last_seen 을 전진시켜야 함")
}

// TestServer_DisconnectMarksOffline 는 연결 종료 시 노드가 offline 으로
// 표시되나 레지스트리에는 보존되는지 검증한다(REQ-REMOTE-B06).
func TestServer_DisconnectMarksOffline(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.HandleConnection(ctx, conn)
	}()

	hello, _ := NewHelloMessage(HelloPayload{InstanceID: "node-3"})
	conn.inject(t, hello)

	require.Eventually(t, func() bool {
		st, ok := srv.NodeState("node-3")
		return ok && st.Online
	}, time.Second, 5*time.Millisecond)

	// 연결 종료 → 읽기 루프 종료.
	conn.Close()
	<-done

	st, ok := srv.NodeState("node-3")
	require.True(t, ok, "오프라인 노드도 last-known 으로 보존되어야 함")
	assert.False(t, st.Online, "disconnect 후 노드는 offline 이어야 함")
}

// TestServer_TimeoutMarksOffline 는 heartbeat 타임아웃 시 sweep 이 노드를
// offline 으로 표시하는지 검증한다(REQ-REMOTE-B05).
func TestServer_TimeoutMarksOffline(t *testing.T) {
	srv := NewServer(ServerConfig{HeartbeatTimeout: 20 * time.Millisecond}, nil)
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	hello, _ := NewHelloMessage(HelloPayload{InstanceID: "node-4"})
	conn.inject(t, hello)

	require.Eventually(t, func() bool {
		st, ok := srv.NodeState("node-4")
		return ok && st.Online
	}, time.Second, 5*time.Millisecond)

	// heartbeat 갱신 없이 타임아웃 경과 → sweep 이 offline 처리.
	srv.sweepOnce(time.Now().Add(time.Second))

	st, _ := srv.NodeState("node-4")
	assert.False(t, st.Online, "타임아웃 경과 시 노드는 offline 이어야 함")
}

// TestServer_AuthenticatorRejectsConnection 는 Authenticator seam 이 거부하면
// 연결이 수락되지 않는지 검증한다(M2 seam — 인증/승인은 M2).
func TestServer_AuthenticatorRejectsConnection(t *testing.T) {
	rejecting := AuthenticatorFunc(func(HelloPayload) error {
		return errors.New("denied")
	})
	srv := NewServer(ServerConfig{}, rejecting)
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.HandleConnection(ctx, conn)
	}()

	hello, _ := NewHelloMessage(HelloPayload{InstanceID: "node-5"})
	conn.inject(t, hello)

	// 인증 거부 시 연결 루프는 종료되고 노드는 등록되지 않아야 한다.
	<-done
	_, ok := srv.NodeState("node-5")
	assert.False(t, ok, "인증 거부된 노드는 등록되지 않아야 함")
}

// TestServer_BootstrapSecretAuthenticator 는 기본 부트스트랩-시크릿 검증
// authenticator 의 동작을 검증한다(M1 stub, M2 에서 토큰 인증으로 확장).
func TestServer_BootstrapSecretAuthenticator(t *testing.T) {
	tests := []struct {
		name     string
		secret   string
		hello    HelloPayload
		wantPass bool
	}{
		{name: "no secret configured accepts all", secret: "", hello: HelloPayload{InstanceID: "n"}, wantPass: true},
		{name: "secret configured but spec carries none still accepts (M1 stub)", secret: "s3cr3t", hello: HelloPayload{InstanceID: "n"}, wantPass: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			auth := NewBootstrapAuthenticator(tc.secret)
			err := auth.Authenticate(tc.hello)
			if tc.wantPass {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// TestServer_NodeCount 는 다수 노드 추적을 검증한다(REQ-REMOTE-N01 보조).
func TestServer_NodeCount(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, id := range []string{"a", "b", "c"} {
		conn := newFakeConn()
		go func() { _ = srv.HandleConnection(ctx, conn) }()
		hello, _ := NewHelloMessage(HelloPayload{InstanceID: id})
		conn.inject(t, hello)
	}

	require.Eventually(t, func() bool {
		return srv.NodeCount() == 3
	}, time.Second, 5*time.Millisecond)
}

// TestServer_NodesSnapshot 는 Nodes() 가 전체 노드 스냅샷을 반환하는지 검증한다.
func TestServer_NodesSnapshot(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, id := range []string{"x", "y"} {
		conn := newFakeConn()
		go func() { _ = srv.HandleConnection(ctx, conn) }()
		hello, _ := NewHelloMessage(HelloPayload{InstanceID: id})
		conn.inject(t, hello)
	}

	require.Eventually(t, func() bool {
		return len(srv.Nodes()) == 2
	}, time.Second, 5*time.Millisecond)

	ids := map[string]bool{}
	for _, n := range srv.Nodes() {
		ids[n.InstanceID] = true
	}
	assert.True(t, ids["x"] && ids["y"])
}

// TestServer_StartSweeperMarksOffline 는 백그라운드 sweeper 가 타임아웃된 노드를
// offline 으로 표시하는지 검증한다(REQ-REMOTE-B05).
func TestServer_StartSweeperMarksOffline(t *testing.T) {
	srv := NewServer(ServerConfig{HeartbeatTimeout: 20 * time.Millisecond}, nil)
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	hello, _ := NewHelloMessage(HelloPayload{InstanceID: "node-sw"})
	conn.inject(t, hello)

	require.Eventually(t, func() bool {
		st, ok := srv.NodeState("node-sw")
		return ok && st.Online
	}, time.Second, 5*time.Millisecond)

	srv.StartSweeper(ctx)

	// heartbeat 갱신 없이 타임아웃 + sweep 주기 경과를 기다린다.
	require.Eventually(t, func() bool {
		st, ok := srv.NodeState("node-sw")
		return ok && !st.Online
	}, time.Second, 5*time.Millisecond, "sweeper 가 타임아웃 노드를 offline 처리해야 함")
}

// TestServer_IgnoresMalformedMessages 는 잘못된 메시지가 루프를 중단시키지
// 않는지 검증한다.
func TestServer_IgnoresMalformedMessages(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	conn.incoming <- []byte("{not valid json")

	hello, _ := NewHelloMessage(HelloPayload{InstanceID: "node-6"})
	conn.inject(t, hello)

	require.Eventually(t, func() bool {
		_, ok := srv.NodeState("node-6")
		return ok
	}, time.Second, 5*time.Millisecond, "malformed 메시지 후에도 정상 메시지를 처리해야 함")
}
