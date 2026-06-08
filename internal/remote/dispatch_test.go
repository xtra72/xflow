// dispatch_test.go 는 M3 서버 측 명령 디스패처를 검증한다
// (@SPEC:SPEC-REMOTE-001 M3, REQ-D01/D05/D06/D07/D08).
//
// 검증:
//   - command_id 상관: 동시 명령이 올바른 결과로 매칭(REQ-D07).
//   - 타임아웃: 결과 미수신 시 오류 + pending 정리(REQ-D06).
//   - 미승인/오프라인 노드 디스패치 거절(REQ-D08/B07).
//   - 적용 실패(ok=false) → ErrCommandFailed(REQ-D09).
//
// fakeConn / memManagedNodeRepo / fakeTokenIssuer 는 server_test.go /
// registration_test.go 에 정의된 것을 재사용한다.
package remote

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

// approvedNodeConn 은 승인+온라인 노드 1개를 가진 서버와 그 라이브 fakeConn 을
// 준비한다. 테스트가 conn.outgoing 으로 디스패치된 command 를 읽고, conn.inject
// 로 command_result 를 주입한다.
func approvedNodeConn(t *testing.T, instanceID string) (*Server, *fakeConn, context.CancelFunc) {
	t.Helper()
	repo := newMemManagedNodeRepo()
	issuer := newFakeTokenIssuer()
	srv := NewServer(ServerConfig{
		Repo:           repo,
		TokenIssuer:    issuer,
		CommandTimeout: 200 * time.Millisecond,
	}, nil)

	// 승인 노드 사전 등록 + 토큰.
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: instanceID, Status: RegStatusApproved,
	}))
	token, _ := issuer.Issue(instanceID, "node")
	require.NoError(t, repo.SetToken(context.Background(), instanceID, token))

	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.HandleConnectionAuth(ctx, conn, instanceID) }()

	require.Eventually(t, func() bool {
		return srv.IsManaged(instanceID)
	}, time.Second, 5*time.Millisecond, "노드는 승인+온라인(managed)이어야 함")

	return srv, conn, cancel
}

// readDispatchedCommand 는 서버가 conn 으로 보낸 다음 command 를 읽어 디코드한다.
func readDispatchedCommand(t *testing.T, conn *fakeConn) CommandPayload {
	t.Helper()
	select {
	case data := <-conn.outgoing:
		msg, err := DecodeMessage(data)
		require.NoError(t, err)
		require.Equal(t, TypeCommand, msg.Type)
		var cmd CommandPayload
		require.NoError(t, json.Unmarshal(msg.Payload, &cmd))
		return cmd
	case <-time.After(time.Second):
		t.Fatal("command 디스패치 수신 타임아웃")
		return CommandPayload{}
	}
}

// TestDispatch_SuccessRoundTrip 는 디스패치→결과 왕복이 성공하는지 검증한다
// (REQ-D01/D05/D07).
func TestDispatch_SuccessRoundTrip(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-d1")
	defer cancel()

	resultCh := make(chan json.RawMessage, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := srv.Dispatch(context.Background(), "node-d1", DomainFlow, "start",
			json.RawMessage(`{"id":"f1"}`))
		resultCh <- res
		errCh <- err
	}()

	// 서버가 보낸 command 를 읽고, 동일 command_id 로 결과를 주입한다.
	cmd := readDispatchedCommand(t, conn)
	assert.Equal(t, "node-d1", cmd.TargetInstanceID)
	assert.Equal(t, DomainFlow, cmd.Domain)
	assert.Equal(t, "start", cmd.Action)
	assert.NotEmpty(t, cmd.CommandID, "command 는 고유 command_id 를 가져야 함")

	ackMsg, _ := NewCommandResultMessage(CommandResultPayload{
		CommandID: cmd.CommandID,
		OK:        true,
		Result:    json.RawMessage(`{"status":"running"}`),
	})
	conn.inject(t, ackMsg)

	require.NoError(t, <-errCh)
	assert.JSONEq(t, `{"status":"running"}`, string(<-resultCh))
}

// TestDispatch_ConcurrentCorrelation 는 동시 다수 명령이 각자의 command_id 로
// 올바른 결과에 매칭되는지 검증한다(REQ-D07).
func TestDispatch_ConcurrentCorrelation(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-d2")
	defer cancel()

	const n = 8
	results := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		idx := i
		go func() {
			defer wg.Done()
			res, err := srv.Dispatch(context.Background(), "node-d2", DomainAgent, "start",
				json.RawMessage(`{}`))
			if err == nil {
				results[idx] = string(res)
			}
		}()
	}

	// 디스패치된 n 개 command 를 수집한 뒤, 각 command_id 에 고유 결과를 역순으로
	// 주입하여 상관(correlation)이 순서 무관하게 올바른지 확인한다.
	cmds := make([]CommandPayload, n)
	for i := 0; i < n; i++ {
		cmds[i] = readDispatchedCommand(t, conn)
	}
	// command_id 는 모두 고유해야 한다(REQ-D07).
	seen := map[string]bool{}
	for _, c := range cmds {
		assert.False(t, seen[c.CommandID], "command_id 는 고유해야 함")
		seen[c.CommandID] = true
	}
	// 역순 주입: 각 command_id 에 자기 command_id 를 결과로 담아 매칭을 검증.
	for i := n - 1; i >= 0; i-- {
		ack, _ := NewCommandResultMessage(CommandResultPayload{
			CommandID: cmds[i].CommandID,
			OK:        true,
			Result:    json.RawMessage(`"` + cmds[i].CommandID + `"`),
		})
		conn.inject(t, ack)
	}

	wg.Wait()
	// 각 디스패치는 자신이 보낸 command_id 를 결과로 받아야 한다.
	got := map[string]bool{}
	for _, r := range results {
		var s string
		require.NoError(t, json.Unmarshal([]byte(r), &s))
		got[s] = true
	}
	for _, c := range cmds {
		assert.True(t, got[c.CommandID], "각 명령은 자신의 결과를 받아야 함")
	}
}

// TestDispatch_Timeout 는 결과 미수신 시 타임아웃 오류 + pending 정리를 검증한다
// (REQ-D06).
func TestDispatch_Timeout(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-d3")
	defer cancel()

	res, err := srv.Dispatch(context.Background(), "node-d3", DomainFlow, "deploy",
		json.RawMessage(`{}`))
	assert.Nil(t, res)
	assert.ErrorIs(t, err, ErrCommandTimeout, "결과 미수신 시 타임아웃 오류여야 함")

	// command 는 디스패치되었지만 결과가 없었다 → pending 정리 확인.
	_ = readDispatchedCommand(t, conn)
	srv.pendingMu.Lock()
	pendingCount := len(srv.pending)
	srv.pendingMu.Unlock()
	assert.Zero(t, pendingCount, "타임아웃 후 pending 항목은 정리되어야 함")
}

// TestDispatch_LateResultAfterTimeout 는 타임아웃 후 늦게 도착한 결과가 안전하게
// 폐기되는지 검증한다(pending 누수/패닉 없음).
func TestDispatch_LateResultAfterTimeout(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-d3b")
	defer cancel()

	_, err := srv.Dispatch(context.Background(), "node-d3b", DomainFlow, "deploy",
		json.RawMessage(`{}`))
	require.ErrorIs(t, err, ErrCommandTimeout)

	cmd := readDispatchedCommand(t, conn)
	// 타임아웃 이후 동일 command_id 결과를 늦게 주입 → 폐기되어야 한다(패닉 없음).
	ack, _ := NewCommandResultMessage(CommandResultPayload{CommandID: cmd.CommandID, OK: true})
	conn.inject(t, ack)

	// 잠시 대기하여 라우팅이 처리되도록 한 뒤 pending 이 여전히 0 임을 확인.
	require.Eventually(t, func() bool {
		srv.pendingMu.Lock()
		defer srv.pendingMu.Unlock()
		return len(srv.pending) == 0
	}, time.Second, 5*time.Millisecond)
}

// TestDispatch_OfflineNodeRejected 는 오프라인(미연결) 노드 디스패치가 거절되는지
// 검증한다(REQ-B07/D08).
func TestDispatch_OfflineNodeRejected(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := newM2Server(repo, newFakeTokenIssuer())
	// 승인되었으나 연결되지 않은(오프라인) 노드.
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: "node-d4", Status: RegStatusApproved,
	}))

	res, err := srv.Dispatch(context.Background(), "node-d4", DomainFlow, "start", nil)
	assert.Nil(t, res)
	assert.ErrorIs(t, err, ErrNodeNotManaged, "오프라인 노드 디스패치는 거절되어야 함")
}

// TestDispatch_UnapprovedNodeRejected 는 미승인(pending) 노드 디스패치가 거절되는지
// 검증한다(REQ-C06/D08).
func TestDispatch_UnapprovedNodeRejected(t *testing.T) {
	repo := newMemManagedNodeRepo()
	srv := newM2Server(repo, newFakeTokenIssuer())
	conn := newFakeConn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.HandleConnection(ctx, conn) }()

	injectRegister(t, conn, RegisterPayload{InstanceID: "node-d5"})
	_ = readAck(t, conn) // pending ack

	require.Eventually(t, func() bool {
		st, ok := srv.NodeState("node-d5")
		return ok && st.Online
	}, time.Second, 5*time.Millisecond)

	// pending(미승인)이지만 온라인 → 여전히 거절되어야 한다(REQ-D08).
	res, err := srv.Dispatch(context.Background(), "node-d5", DomainFlow, "start", nil)
	assert.Nil(t, res)
	assert.ErrorIs(t, err, ErrNodeNotManaged)
}

// TestDispatch_ApplyFailure 는 클라이언트 적용 실패(ok=false)가 ErrCommandFailed 로
// 반환되는지 검증한다(REQ-D09).
func TestDispatch_ApplyFailure(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-d6")
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := srv.Dispatch(context.Background(), "node-d6", DomainDevice, "update", nil)
		errCh <- err
	}()

	cmd := readDispatchedCommand(t, conn)
	ack, _ := NewCommandResultMessage(CommandResultPayload{
		CommandID: cmd.CommandID,
		OK:        false,
		Error:     "validation failed",
	})
	conn.inject(t, ack)

	err := <-errCh
	assert.ErrorIs(t, err, ErrCommandFailed)
	assert.Contains(t, err.Error(), "validation failed")
}

// TestRouteCommandResult_Malformed 는 잘못된/누락된 command_result 페이로드가 안전하게
// 폐기되는지 검증한다(패닉 없음).
func TestRouteCommandResult_Malformed(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	// 디코드 실패.
	srv.routeCommandResult([]byte("{not json"))
	// command_id 누락.
	srv.routeCommandResult([]byte(`{"ok":true}`))
	// 매칭 pending 없음.
	srv.routeCommandResult([]byte(`{"command_id":"ghost","ok":true}`))

	srv.pendingMu.Lock()
	defer srv.pendingMu.Unlock()
	assert.Empty(t, srv.pending, "잘못된 결과는 pending 을 변경하지 않아야 함")
}

// TestCommandTimeout_DefaultWhenUnset 는 CommandTimeout 미설정 시 기본값이 적용되는지
// 검증한다.
func TestCommandTimeout_DefaultWhenUnset(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	assert.Equal(t, DefaultCommandTimeout, srv.commandTimeout())
}

// TestDispatch_ContextCancel 는 호출자 컨텍스트 취소 시 즉시 오류를 반환하는지
// 검증한다.
func TestDispatch_ContextCancel(t *testing.T) {
	srv, _, cancelSrv := approvedNodeConn(t, "node-d7")
	defer cancelSrv()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := srv.Dispatch(ctx, "node-d7", DomainFlow, "start", nil)
		errCh <- err
	}()
	cancel()

	select {
	case err := <-errCh:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("컨텍스트 취소 시 Dispatch 가 반환해야 함")
	}
}
