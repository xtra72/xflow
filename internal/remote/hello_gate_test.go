// hello_gate_test.go 는 hello 등록 게이트와 그 자가 복구 경로를 고정한다
// (@SPEC:SPEC-REMOTE-HELLO-GATE-001).
//
// 고치기 전의 결함: 등록 항목이 없는 instance 의 hello 가 무조건 수락되어, 서버
// 로그에는 online 으로 남고 인벤토리까지 미러에 쌓이지만 `ListNodes`(DB 기반)는 그
// 노드를 0건으로 보았다 — 로그에는 붙어 있고 화면에는 없는 유령. 노드는 무효한
// 토큰을 쥔 채 hello 만 되풀이하므로 스스로 빠져나오지도 못했다.
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api/ws"
	"github.com/xtra/xflow/internal/storage"
)

// readServerMessage 는 서버가 노드로 보낸 다음 메시지를 읽는다.
func readServerMessage(t *testing.T, conn *fakeConn) *ws.Message {
	t.Helper()
	select {
	case data := <-conn.outgoing:
		msg, err := DecodeMessage(data)
		require.NoError(t, err)
		return msg
	case <-time.After(time.Second):
		t.Fatal("서버 메시지 수신 타임아웃")
		return nil
	}
}

// TestHelloGate_UnregisteredRefused 는 미등록 instance 의 hello 가 거부되고
// hello_nack 이 회신되는지 검증한다(유령 노드 방지).
func TestHelloGate_UnregisteredRefused(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := newM2Server(repo, newFakeTokenIssuer())
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	hello, err := NewHelloMessage(HelloPayload{InstanceID: "ghost-1", Hostname: "xagentX", Version: "dev"})
	require.NoError(t, err)
	conn.inject(t, hello)

	msg := readServerMessage(t, conn)
	require.Equal(t, TypeHelloNack, msg.Type, "미등록 hello 는 hello_nack 으로 거부되어야 함")
	var nack HelloNackPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &nack))
	assert.Equal(t, HelloNackUnregistered, nack.Reason)

	// online 으로 잡히지 않아야 한다 — 로그와 목록이 갈라지던 자리다.
	_, ok := srv.NodeState("ghost-1")
	assert.False(t, ok, "거부된 hello 는 노드 상태를 만들지 않아야 함")
	nodes, err := srv.ListNodes(context.Background())
	require.NoError(t, err)
	assert.Empty(t, nodes)
}

// TestHelloGate_NotApprovedRefused 는 승인되지 않은 항목(pending)의 hello 가
// 거부되는지 검증한다(REQ-C06 정신 — 비승인 노드는 managed 가 아니다).
func TestHelloGate_NotApprovedRefused(t *testing.T) {
	repo := newMemManagedNodeRepo()
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: "n-pending", Status: RegStatusPending,
	}))
	srv := newM2Server(repo, newFakeTokenIssuer())
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	hello, err := NewHelloMessage(HelloPayload{InstanceID: "n-pending"})
	require.NoError(t, err)
	conn.inject(t, hello)

	msg := readServerMessage(t, conn)
	require.Equal(t, TypeHelloNack, msg.Type)
	var nack HelloNackPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &nack))
	assert.Equal(t, HelloNackNotApproved, nack.Reason)
	assert.False(t, srv.IsManaged("n-pending"))
}

// TestHelloGate_M1NoRepoStillAccepts 는 repo 미구성(M1 모드)에서 hello 가 종전처럼
// 수락되는지 검증한다(하위 호환 — 판정 근거가 없는 모드는 게이트하지 않는다).
func TestHelloGate_M1NoRepoStillAccepts(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	hello, err := NewHelloMessage(HelloPayload{InstanceID: "m1-node"})
	require.NoError(t, err)
	conn.inject(t, hello)

	require.Eventually(t, func() bool {
		st, ok := srv.NodeState("m1-node")
		return ok && st.Online
	}, time.Second, 5*time.Millisecond, "M1 모드는 hello 를 수락해야 함")
}

// TestHelloGate_RestoreRefusalSendsNack 는 토큰은 유효하나 등록 항목이 없는 재접속이
// 조용히 끊기지 않고 사유를 회신하는지 검증한다(자가 복구의 두 번째 진입점).
func TestHelloGate_RestoreRefusalSendsNack(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := newM2Server(repo, newFakeTokenIssuer())
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// authedInstanceID 가 채워진 경로(토큰 검증 통과) — 그러나 repo 에 항목이 없다.
	go func() { _ = srv.HandleConnectionAuth(ctx, conn, "vanished-1") }()

	msg := readServerMessage(t, conn)
	require.Equal(t, TypeHelloNack, msg.Type)
	var nack HelloNackPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &nack))
	assert.Equal(t, HelloNackUnregistered, nack.Reason)
}

// TestClient_HelloNackClearsTokenAndRegisters 는 노드가 hello_nack 을 받으면 토큰을
// 버리고 다음 접속에서 register 로 되돌아가는지 검증한다 — 스스로 풀려나는 경로다.
func TestClient_HelloNackClearsTokenAndRegisters(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, SaveNodeToken(dir, "stale-token"))

	first := newClientFakeConn()
	second := newClientFakeConn()
	var dialCount atomic.Int32
	// 넉넉한 버퍼 + non-blocking 송신: dial 이 이 채널에 막히면 runLoop 고루틴이
	// 멈춰 Stop() 의 wg.Wait() 가 영원히 기다린다(시험이 행에 걸린다).
	dialedURLs := make(chan string, 16)

	dialer := DialerFunc(func(_ context.Context, url string) (Conn, error) {
		select {
		case dialedURLs <- url:
		default:
		}
		// 3회차 이후는 실패를 돌려준다 — 이미 닫힌 conn 을 되돌려 주면 세션이 즉시
		// 끝나며 재연결이 폭주해 같은 판의 다른 시험 시간을 흔든다.
		switch dialCount.Add(1) {
		case 1:
			return first, nil
		case 2:
			return second, nil
		default:
			return nil, errors.New("시험 종료 — 더 이상 dial 하지 않음")
		}
	})

	cli := NewClient(ClientConfig{
		ServerURL:         "wss://example/api/remote/ws",
		InstanceID:        "node-heal",
		Hostname:          "host-heal",
		Version:           "1.0.0",
		HeartbeatInterval: time.Hour,
		DataDir:           dir,
		ReconnectInitial:  5 * time.Millisecond,
		ReconnectMax:      50 * time.Millisecond,
	}, dialer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cli.Start(ctx)
	defer cli.Stop()

	// 1회차: 토큰 보유 → hello, 그리고 dial URL 에 토큰이 실린다.
	firstURL := <-dialedURLs
	assert.Contains(t, firstURL, "token=stale-token")
	require.Equal(t, TypeHello, readClientMessage(t, first).Type)

	// 서버가 거부한다.
	nack, err := NewHelloNackMessage(HelloNackPayload{Reason: HelloNackUnregistered})
	require.NoError(t, err)
	data, err := nack.Encode()
	require.NoError(t, err)
	first.toClient <- data

	// 2회차: 토큰을 버렸으므로 URL 에 토큰이 없고 첫 메시지는 register 여야 한다.
	secondURL := <-dialedURLs
	assert.NotContains(t, secondURL, "token=", "토큰을 버린 뒤 dial 에는 토큰이 없어야 함")
	require.Equal(t, TypeRegister, readClientMessage(t, second).Type,
		"hello_nack 이후에는 register 로 되돌아가야 함")

	// 디스크의 토큰도 지워져야 한다 — 재시작이 유령 상태를 되살리지 못하게.
	_, statErr := os.Stat(filepath.Join(dir, "node_token"))
	assert.True(t, os.IsNotExist(statErr), "노드 토큰 파일이 삭제되어야 함")
}
