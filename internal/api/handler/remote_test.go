package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/remote"
)

// TestRemoteHandler_AcceptsNodeAndTracksOnline 는 관리 WS 핸들러가 노드 연결을
// 수락하고 hello 수신 시 online 으로 추적하는지 검증한다(REQ-REMOTE-B01/B05, N02).
func TestRemoteHandler_AcceptsNodeAndTracksOnline(t *testing.T) {
	srv := remote.NewServer(remote.ServerConfig{}, nil)
	h := NewRemoteHandler(srv, nil)

	ts := httptest.NewServer(http.HandlerFunc(h.HandleUpgrade))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	conn, _, err := websocket.DefaultDialer.DialContext(context.Background(), wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	hello, err := remote.NewHelloMessage(remote.HelloPayload{
		InstanceID: "node-h1",
		Hostname:   "host-h1",
		Version:    "1.0.0",
	})
	require.NoError(t, err)
	data, err := hello.Encode()
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, data))

	require.Eventually(t, func() bool {
		st, ok := srv.NodeState("node-h1")
		return ok && st.Online
	}, 2*time.Second, 10*time.Millisecond, "hello 수신 후 노드는 online 이어야 함")
}

// TestRemoteHandler_SeparateEndpoint 는 관리 WS 엔드포인트 경로가 모니터링 /ws
// 와 분리됨을 검증한다(REQ-REMOTE-N02). RemoteWSPattern 상수로 확인.
func TestRemoteHandler_SeparateEndpoint(t *testing.T) {
	assert.Equal(t, "GET /api/remote/ws", RemoteWSPattern)
	assert.NotEqual(t, "GET /ws", RemoteWSPattern)
}

// TestRemoteHandler_DisconnectMarksOffline 는 노드 연결 종료 시 offline 으로
// 표시되나 보존되는지 검증한다(REQ-REMOTE-B06).
func TestRemoteHandler_DisconnectMarksOffline(t *testing.T) {
	srv := remote.NewServer(remote.ServerConfig{}, nil)
	h := NewRemoteHandler(srv, nil)

	ts := httptest.NewServer(http.HandlerFunc(h.HandleUpgrade))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	conn, _, err := websocket.DefaultDialer.DialContext(context.Background(), wsURL, nil)
	require.NoError(t, err)

	hello, _ := remote.NewHelloMessage(remote.HelloPayload{InstanceID: "node-h2"})
	data, _ := hello.Encode()
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, data))

	require.Eventually(t, func() bool {
		st, ok := srv.NodeState("node-h2")
		return ok && st.Online
	}, 2*time.Second, 10*time.Millisecond)

	require.NoError(t, conn.Close())

	require.Eventually(t, func() bool {
		st, ok := srv.NodeState("node-h2")
		return ok && !st.Online
	}, 2*time.Second, 10*time.Millisecond, "disconnect 후 노드는 offline(보존)이어야 함")
}
