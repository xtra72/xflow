package remote

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
)

// TestGorillaConn_RoundTrip 는 gorilla 기반 Conn 래퍼의 read/write/close 가
// 실제 WS 연결에서 동작하는지 검증한다.
func TestGorillaConn_RoundTrip(t *testing.T) {
	var serverUpgrader = websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}

	// echo 서버: 받은 메시지를 그대로 되돌려준다.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := serverUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			mt, data, err := c.ReadMessage()
			if err != nil {
				return
			}
			if err := c.WriteMessage(mt, data); err != nil {
				return
			}
		}
	}))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	// NewGorillaDialer 로 dial → NewGorillaConn 래퍼 경유.
	dialer := NewGorillaDialer()
	conn, err := dialer.Dial(context.Background(), wsURL)
	require.NoError(t, err)
	defer conn.Close()

	// hello 송신 → echo 수신.
	hello, err := NewHelloMessage(HelloPayload{InstanceID: "rt-1"})
	require.NoError(t, err)
	require.NoError(t, writeEnvelope(conn, hello))

	// 첫 echo 읽기(hello).
	_, err = conn.ReadMessage()
	require.NoError(t, err)

	// 두 번째 왕복으로 ReadMessage 경로 검증.
	hb, err := NewHeartbeatMessage("rt-1")
	require.NoError(t, err)
	require.NoError(t, writeEnvelope(conn, hb))

	data, err := conn.ReadMessage()
	require.NoError(t, err)
	msg, err := DecodeMessage(data)
	require.NoError(t, err)
	assert.Equal(t, TypeHeartbeat, msg.Type)
}

// TestGorillaDialer_DialError 는 dial 실패 시 에러를 반환하는지 검증한다.
func TestGorillaDialer_DialError(t *testing.T) {
	dialer := NewGorillaDialer()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := dialer.Dial(ctx, "ws://127.0.0.1:1/nonexistent")
	assert.Error(t, err, "도달 불가 주소 dial 은 에러여야 함")
}

// TestNewGorillaConn_Exported 는 NewGorillaConn 이 Conn 을 반환하는지(핸들러
// 사용 경로) 검증한다.
func TestNewGorillaConn_Exported(t *testing.T) {
	var serverUpgrader = websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}
	captured := make(chan Conn, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := serverUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		captured <- NewGorillaConn(c)
	}))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	cConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer cConn.Close()

	select {
	case server := <-captured:
		require.NotNil(t, server)
		assert.NoError(t, server.Close())
	case <-time.After(time.Second):
		t.Fatal("서버 측 Conn 캡처 실패")
	}
}
