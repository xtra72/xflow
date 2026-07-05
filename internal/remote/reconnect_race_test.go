// reconnect_race_test.go 는 노드 프로그램 재기동 시 발생하는 stale-cleanup 경합을
// 검증한다. 핵심: 새 연결이 s.conns[id] 를 교체한 뒤, 뒤늦게 종료되는 이전 연결의
// teardown 이 살아 있는 새 세션을 절대 무너뜨리지 않아야 한다(connection-identity
// 인지 teardown — "동일 연결인 경우에만").
//
// 회귀 전 동작(버그): 이전 goroutine 의 teardown defer 가 instanceID 만으로
// unregisterConn(무조건 delete) + markOffline + teardownNodeStreams/Bridges 를
// 수행해, 방금 재접속한 새 연결을 삭제하고 라이브 노드를 offline 으로 만들었다.
package remote

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

// TestReconnectRace_SupersededTeardownDoesNotEvictNewConn 는 노드 재기동 경합을
// 재현한다:
//  1. approved 노드의 OLD 연결이 세션 복원되어 managed/online.
//  2. 노드 재기동 → NEW 연결이 도착해 restoreSession 으로 s.conns[id] 를 교체.
//  3. OLD 연결의 읽기 루프가 뒤늦게 종료(TCP close) → OLD goroutine 의 teardown defer 실행.
//
// 기대(수정 후): OLD teardown 이 끝난 뒤에도 connFor 는 NEW 연결을 반환하고, 노드는
// 여전히 online/managed 이며, NEW 세션에 등록된 브리지는 teardown 되지 않는다.
func TestReconnectRace_SupersededTeardownDoesNotEvictNewConn(t *testing.T) {
	repo := newMemManagedNodeRepo()
	issuer := newFakeTokenIssuer()
	srv := newM2Server(repo, issuer)

	const id = "node-race"
	// 사전 조건: 승인된 노드 + 발급 토큰.
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{InstanceID: id, Status: RegStatusApproved}))
	token, jti, _ := issuer.IssueWithID(id, "node")
	require.NoError(t, repo.SetToken(context.Background(), id, jti))
	_ = token

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// (1) OLD 연결: 토큰 검증된 재접속 경로로 세션 복원.
	oldConn := newFakeConn()
	oldDone := make(chan struct{})
	go func() {
		defer close(oldDone)
		_ = srv.HandleConnectionAuth(ctx, oldConn, id)
	}()

	require.Eventually(t, func() bool {
		nc, ok := srv.connFor(id)
		return ok && nc.conn == oldConn && srv.IsManaged(id)
	}, time.Second, 5*time.Millisecond, "OLD 연결이 managed online 이어야 함")

	// (2) 노드 재기동 → NEW 연결 도착. 새 세션 등록(restoreSession)을 OLD teardown 보다
	// 먼저 결정적으로 수행한다(실제 경합에서 새 연결 등록이 이전 읽기 루프 종료보다
	// 앞서는 순간을 모사). NEW 연결도 살아 있는 읽기 루프를 갖도록 goroutine 으로 구동한다.
	newConn := newFakeConn()
	newCancelCtx, newCancel := context.WithCancel(ctx)
	defer newCancel()
	_, newRestored := srv.restoreSession(newCancelCtx, newConn, newCancel, id)
	require.True(t, newRestored, "NEW 연결 세션 복원이 성공해야 함")

	// NEW 세션이 s.conns[id] 를 차지했는지 확인.
	require.Eventually(t, func() bool {
		nc, ok := srv.connFor(id)
		return ok && nc.conn == newConn
	}, time.Second, 5*time.Millisecond, "NEW 연결이 conns 를 차지해야 함")

	// NEW 세션에 라이브 브리지를 하나 등록한다(teardown 회귀 검출용).
	br := &ServerBridge{
		srv:        srv,
		bridgeID:   "br-1",
		instanceID: id,
		outputs:    make(chan BridgeOutputPayload, 1),
		status:     make(chan BridgeStatusPayload, 1),
	}
	srv.bridgeMu.Lock()
	srv.bridges["br-1"] = br
	srv.bridgeMu.Unlock()

	// (3) OLD 연결의 읽기 루프 종료 → OLD goroutine 의 teardown defer 실행.
	oldConn.Close()
	select {
	case <-oldDone:
	case <-time.After(time.Second):
		t.Fatal("OLD 연결 goroutine 이 종료되어야 함")
	}

	// 기대: superseded OLD teardown 은 NEW 세션의 자원을 건드리지 않는다.
	nc, ok := srv.connFor(id)
	assert.True(t, ok, "NEW 연결이 conns 에 남아 있어야 함")
	if ok {
		assert.Equal(t, newConn, nc.conn, "connFor 는 NEW 연결을 반환해야 함(삭제 금지)")
	}

	assert.True(t, srv.IsManaged(id), "노드는 여전히 managed online 이어야 함")
	st, stOK := srv.NodeState(id)
	require.True(t, stOK)
	assert.True(t, st.Online, "노드는 여전히 online 이어야 함")

	// NEW 세션 브리지는 teardown 되지 않아야 한다.
	srv.bridgeMu.Lock()
	_, bridgeAlive := srv.bridges["br-1"]
	srv.bridgeMu.Unlock()
	assert.True(t, bridgeAlive, "NEW 세션의 브리지는 살아 있어야 함(superseded teardown 이 건드리면 안 됨)")
}
