// client_m2_test.go 는 M2 클라이언트 등록/토큰 흐름을 검증한다
// (@SPEC:SPEC-REMOTE-001 M2, REQ-C01/C04/C05).
//
// 토큰 미보유 시 register 송신, approved ack 시 토큰 영속, 토큰 보유 시 재접속
// 핸드셰이크에 토큰 제시, rejected 시 정지를 검증한다.
package remote

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNodeToken_PersistAndLoad 는 노드 토큰 파일의 atomic 저장/로드를 검증한다
// (REQ-C04 — 토큰 영속). 파일 권한 0600 을 확인한다(REQ-F06).
func TestNodeToken_PersistAndLoad(t *testing.T) {
	dir := t.TempDir()

	// 미존재 시 빈 토큰.
	tok, ok, err := LoadNodeToken(dir)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, tok)

	require.NoError(t, SaveNodeToken(dir, "my-node-token"))

	loaded, ok, err := LoadNodeToken(dir)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "my-node-token", loaded)

	// 권한 0600 검증.
	info, err := os.Stat(filepath.Join(dir, nodeTokenFileName))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "노드 토큰 파일은 0600 이어야 함")
}

// TestClient_SendsRegisterWhenNoToken 는 토큰이 없으면 클라이언트가 register 를
// 보내는지 검증한다(REQ-C01).
func TestClient_SendsRegisterWhenNoToken(t *testing.T) {
	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		return conn, nil
	})

	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example",
		InstanceID:        "node-reg",
		Hostname:          "h",
		Version:           "1.0.0",
		HeartbeatInterval: time.Hour,
		Exposure:          ExposureSummary{Flows: "all"},
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	msg := readClientMessage(t, conn)
	require.Equal(t, TypeRegister, msg.Type, "토큰이 없으면 첫 메시지는 register 여야 함")

	var p RegisterPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &p))
	assert.Equal(t, "node-reg", p.InstanceID)
	assert.Equal(t, "all", p.Exposure.Flows)
}

// TestClient_PersistsTokenOnApproved 는 approved ack 수신 시 토큰을 영속하는지
// 검증한다(REQ-C04).
func TestClient_PersistsTokenOnApproved(t *testing.T) {
	dir := t.TempDir()
	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		return conn, nil
	})

	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example",
		InstanceID:        "node-appr",
		HeartbeatInterval: time.Hour,
		DataDir:           dir,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	// register 소비.
	reg := readClientMessage(t, conn)
	require.Equal(t, TypeRegister, reg.Type)

	// 서버 → approved ack(토큰 포함).
	ack, _ := NewRegisterAckMessage(RegisterAckPayload{
		Status:    RegStatusApproved,
		NodeToken: "approved-token-123",
	})
	ackData, _ := ack.Encode()
	conn.toClient <- ackData

	// 토큰이 디스크에 영속되어야 한다.
	require.Eventually(t, func() bool {
		tok, ok, _ := LoadNodeToken(dir)
		return ok && tok == "approved-token-123"
	}, time.Second, 10*time.Millisecond, "approved ack 시 토큰이 영속되어야 함")
}

// TestClient_PresentsTokenOnReconnect 는 토큰이 영속되어 있으면 dial URL 에 토큰을
// 제시하는지 검증한다(REQ-C05).
func TestClient_PresentsTokenOnReconnect(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, SaveNodeToken(dir, "persisted-token"))

	var (
		mu        sync.Mutex
		dialedURL string
	)
	conn := newClientFakeConn()
	dialer := DialerFunc(func(_ context.Context, u string) (Conn, error) {
		mu.Lock()
		dialedURL = u
		mu.Unlock()
		return conn, nil
	})

	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example/api/remote/ws",
		InstanceID:        "node-reco",
		HeartbeatInterval: time.Hour,
		DataDir:           dir,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	// 토큰 보유 시 dial URL 에 토큰을 제시해야 한다(세션 복원 경로).
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return dialedURL != ""
	}, time.Second, 5*time.Millisecond)

	mu.Lock()
	got := dialedURL
	mu.Unlock()
	assert.Contains(t, got, "token=persisted-token", "재접속 dial URL 에 토큰이 포함되어야 함")
}

// TestClient_StopsOnRejected 는 rejected ack 수신 시 클라이언트가 재연결을 멈추는지
// 검증한다(REQ-C03 — 거부 시 정지).
func TestClient_StopsOnRejected(t *testing.T) {
	conn := newClientFakeConn()
	var dialCount int
	dialer := DialerFunc(func(_ context.Context, _ string) (Conn, error) {
		dialCount++
		return conn, nil
	})

	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example",
		InstanceID:        "node-rej",
		HeartbeatInterval: time.Hour,
		ReconnectInitial:  5 * time.Millisecond,
		ReconnectMax:      20 * time.Millisecond,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	reg := readClientMessage(t, conn)
	require.Equal(t, TypeRegister, reg.Type)

	// 서버 → rejected ack.
	ack, _ := NewRegisterAckMessage(RegisterAckPayload{Status: RegStatusRejected, Reason: "denied"})
	ackData, _ := ack.Encode()
	conn.toClient <- ackData

	// rejected 후에는 재연결을 멈춰야 한다. 연결을 끊어도 재dial 하지 않아야 한다.
	conn.Close()
	time.Sleep(100 * time.Millisecond)
	assert.True(t, cli.Stopped(), "rejected 후 클라이언트는 정지 상태여야 함")
}
