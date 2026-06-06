// query_dispatch_test.go 는 M8(그룹 J) 서버 측 READ/QUERY 프록시 디스패처를 검증한다
// (@SPEC:SPEC-REMOTE-001 M8, REQ-J01/J02/J05/J07/J16).
//
// 검증:
//   - query happy-path: 도메인별 query_id 상관 + redacted 본문 반환(REQ-J01/J02).
//   - 미승인/오프라인 노드 → ErrNodeNotManaged(503 매핑, REQ-J05/J07).
//   - 타임아웃 → ErrQueryTimeout(504 매핑, REQ-J07).
//   - 노드 ok:false → ErrQueryFailed(502 매핑, REQ-J07).
//   - TTL 캐시: TTL 내 동일 질의는 캐시된 redacted 본문 반환(노드 미재호출 — REQ-J16).
//   - 라이브 action(device.state/agent.stats/agent.series)은 캐시 우회(REQ-J16).
//   - 변경(Dispatch) 성공 시 해당 노드 캐시 무효화(REQ-J16).
//
// fakeConn / approvedNodeConn 는 server_test.go / dispatch_test.go 에 정의된 것을
// 재사용한다.
package remote

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

// readDispatchedQuery 는 서버가 conn 으로 보낸 다음 query 를 읽어 디코드한다.
func readDispatchedQuery(t *testing.T, conn *fakeConn) QueryPayload {
	t.Helper()
	select {
	case data := <-conn.outgoing:
		msg, err := DecodeMessage(data)
		require.NoError(t, err)
		require.Equal(t, TypeQuery, msg.Type)
		var q QueryPayload
		require.NoError(t, json.Unmarshal(msg.Payload, &q))
		return q
	case <-time.After(time.Second):
		t.Fatal("query 디스패치 수신 타임아웃")
		return QueryPayload{}
	}
}

// injectQueryResult 는 query_id 에 대한 query_result 를 노드처럼 주입한다.
func injectQueryResult(t *testing.T, conn *fakeConn, queryID string, ok bool, data json.RawMessage, errMsg string) {
	t.Helper()
	msg, err := NewQueryResultMessage(QueryResultPayload{
		QueryID: queryID, OK: ok, Data: data, Error: errMsg,
	})
	require.NoError(t, err)
	conn.inject(t, msg)
}

// TestDispatchQuery_HappyPath 는 query→query_result 왕복이 성공하고 redacted 본문을
// 반환하는지 검증한다(REQ-J01/J02).
func TestDispatchQuery_HappyPath(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-q1")
	defer cancel()

	resultCh := make(chan json.RawMessage, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := srv.DispatchQuery(context.Background(), "node-q1", DomainAgent, QueryActionStats,
			json.RawMessage(`{"id":"a1"}`))
		resultCh <- res
		errCh <- err
	}()

	q := readDispatchedQuery(t, conn)
	assert.Equal(t, "node-q1", q.TargetInstanceID)
	assert.Equal(t, DomainAgent, q.Domain)
	assert.Equal(t, QueryActionStats, q.QueryAction)
	assert.NotEmpty(t, q.QueryID, "query 는 고유 query_id 를 가져야 함")

	injectQueryResult(t, conn, q.QueryID, true, json.RawMessage(`{"rx":10}`), "")

	require.NoError(t, <-errCh)
	assert.JSONEq(t, `{"rx":10}`, string(<-resultCh))
}

// TestDispatchQuery_NotManaged 는 미승인/오프라인 노드 질의가 거절되는지 검증한다
// (REQ-J05/J07 → 503).
func TestDispatchQuery_NotManaged(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	_, err := srv.DispatchQuery(context.Background(), "ghost", DomainAgent, QueryActionStats, nil)
	assert.ErrorIs(t, err, ErrNodeNotManaged)
}

// approvedNodeConnShortQuery 는 짧은 QueryTimeout 으로 승인+온라인 노드를 준비한다
// (타임아웃 검증용).
func approvedNodeConnShortQuery(t *testing.T, instanceID string) (*Server, *fakeConn, context.CancelFunc) {
	t.Helper()
	repo := newMemManagedNodeRepo()
	issuer := newFakeTokenIssuer()
	srv := NewServer(ServerConfig{
		Repo:         repo,
		TokenIssuer:  issuer,
		QueryTimeout: 150 * time.Millisecond,
	}, nil)
	require.NoError(t, repo.Upsert(context.Background(), storage.ManagedNode{
		InstanceID: instanceID, Status: RegStatusApproved,
	}))
	token, _ := issuer.Issue(instanceID, "node")
	require.NoError(t, repo.SetToken(context.Background(), instanceID, token))

	conn := newFakeConn()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.HandleConnectionAuth(ctx, conn, instanceID) }()
	require.Eventually(t, func() bool { return srv.IsManaged(instanceID) },
		time.Second, 5*time.Millisecond)
	return srv, conn, cancel
}

// TestDispatchQuery_Timeout 은 결과 미수신 시 타임아웃 오류를 반환하는지 검증한다
// (REQ-J07 → 504).
func TestDispatchQuery_Timeout(t *testing.T) {
	srv, conn, cancel := approvedNodeConnShortQuery(t, "node-q2")
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := srv.DispatchQuery(context.Background(), "node-q2", DomainFlow, QueryActionGet,
			json.RawMessage(`{"id":"f1"}`))
		errCh <- err
	}()
	_ = readDispatchedQuery(t, conn) // 결과를 주입하지 않는다 → 타임아웃.

	select {
	case err := <-errCh:
		assert.ErrorIs(t, err, ErrQueryTimeout)
	case <-time.After(2 * time.Second):
		t.Fatal("타임아웃 오류 수신 실패")
	}
}

// TestDispatchQuery_NodeError 는 노드 ok:false 응답이 ErrQueryFailed 로 매핑되는지
// 검증한다(REQ-J07 → 502).
func TestDispatchQuery_NodeError(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-q3")
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := srv.DispatchQuery(context.Background(), "node-q3", DomainDevice, QueryActionGet,
			json.RawMessage(`{"id":"d1"}`))
		errCh <- err
	}()
	q := readDispatchedQuery(t, conn)
	injectQueryResult(t, conn, q.QueryID, false, nil, "not found on node")

	select {
	case err := <-errCh:
		assert.ErrorIs(t, err, ErrQueryFailed)
	case <-time.After(2 * time.Second):
		t.Fatal("노드 오류 수신 실패")
	}
}

// TestDispatchQuery_CacheHit 은 TTL 내 동일 질의가 캐시된 redacted 본문을 반환하고
// 노드를 재호출하지 않는지 검증한다(REQ-J16).
func TestDispatchQuery_CacheHit(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-q4")
	defer cancel()

	// 1차: 노드 왕복.
	errCh := make(chan error, 1)
	resCh := make(chan json.RawMessage, 1)
	go func() {
		res, err := srv.DispatchQuery(context.Background(), "node-q4", DomainFlow, QueryActionGet,
			json.RawMessage(`{"id":"f1"}`))
		resCh <- res
		errCh <- err
	}()
	q := readDispatchedQuery(t, conn)
	injectQueryResult(t, conn, q.QueryID, true, json.RawMessage(`{"name":"flow1"}`), "")
	require.NoError(t, <-errCh)
	assert.JSONEq(t, `{"name":"flow1"}`, string(<-resCh))

	// 2차: 동일 질의 — 캐시 적중이어야 한다(노드 재호출 없음).
	res2, err2 := srv.DispatchQuery(context.Background(), "node-q4", DomainFlow, QueryActionGet,
		json.RawMessage(`{"id":"f1"}`))
	require.NoError(t, err2)
	assert.JSONEq(t, `{"name":"flow1"}`, string(res2))

	// outgoing 에 새 query 가 없어야 한다(캐시 적중 — 노드 미호출).
	select {
	case <-conn.outgoing:
		t.Fatal("캐시 적중인데 노드로 query 가 전송됨")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestDispatchQuery_LiveActionBypassesCache 는 라이브 action(agent.stats)이 캐시를
// 우회해 항상 노드를 재호출하는지 검증한다(REQ-J16).
func TestDispatchQuery_LiveActionBypassesCache(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-q5")
	defer cancel()

	for i := 0; i < 2; i++ {
		errCh := make(chan error, 1)
		go func() {
			_, err := srv.DispatchQuery(context.Background(), "node-q5", DomainAgent, QueryActionStats,
				json.RawMessage(`{"id":"a1"}`))
			errCh <- err
		}()
		q := readDispatchedQuery(t, conn) // 매 호출마다 노드로 전송되어야 한다(캐시 우회).
		injectQueryResult(t, conn, q.QueryID, true, json.RawMessage(`{"rx":1}`), "")
		require.NoError(t, <-errCh)
	}
}

// TestDispatchQuery_CacheInvalidatedOnMutation 은 Dispatch(변경) 성공 시 해당 노드의
// 캐시가 무효화되는지 검증한다(REQ-J16).
func TestDispatchQuery_CacheInvalidatedOnMutation(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-q6")
	defer cancel()

	// 캐시 채우기.
	errCh := make(chan error, 1)
	go func() {
		_, err := srv.DispatchQuery(context.Background(), "node-q6", DomainFlow, QueryActionGet,
			json.RawMessage(`{"id":"f1"}`))
		errCh <- err
	}()
	q := readDispatchedQuery(t, conn)
	injectQueryResult(t, conn, q.QueryID, true, json.RawMessage(`{"v":1}`), "")
	require.NoError(t, <-errCh)

	// 변경 명령 성공 → 캐시 무효화.
	derr := make(chan error, 1)
	go func() {
		_, err := srv.Dispatch(context.Background(), "node-q6", DomainFlow, ActionUpdate,
			json.RawMessage(`{"id":"f1"}`))
		derr <- err
	}()
	cmd := readDispatchedCommand(t, conn)
	ack, _ := NewCommandResultMessage(CommandResultPayload{CommandID: cmd.CommandID, OK: true})
	conn.inject(t, ack)
	require.NoError(t, <-derr)

	// 다음 질의는 캐시 미스여야 한다(노드 재호출).
	q2err := make(chan error, 1)
	go func() {
		_, err := srv.DispatchQuery(context.Background(), "node-q6", DomainFlow, QueryActionGet,
			json.RawMessage(`{"id":"f1"}`))
		q2err <- err
	}()
	q2 := readDispatchedQuery(t, conn) // 재호출 확인.
	injectQueryResult(t, conn, q2.QueryID, true, json.RawMessage(`{"v":2}`), "")
	require.NoError(t, <-q2err)
}

// TestQueryCache_Expiry 는 TTL 경과 후 캐시가 만료되는지 검증한다(REQ-J16).
func TestQueryCache_Expiry(t *testing.T) {
	c := newQueryCache(50*time.Millisecond, 100)
	key := queryCacheKey("n1", DomainFlow, QueryActionGet, json.RawMessage(`{"id":"f1"}`))
	c.put(key, json.RawMessage(`{"v":1}`))

	got, ok := c.get(key)
	require.True(t, ok)
	assert.JSONEq(t, `{"v":1}`, string(got))

	time.Sleep(70 * time.Millisecond)
	_, ok = c.get(key)
	assert.False(t, ok, "TTL 경과 후 캐시는 미스여야 함")
}

// TestQueryCache_Bounded 는 캐시가 최대 크기를 넘지 않도록 sweep 되는지 검증한다.
func TestQueryCache_Bounded(t *testing.T) {
	c := newQueryCache(time.Minute, 4)
	for i := 0; i < 20; i++ {
		c.put(queryCacheKey("n1", DomainFlow, QueryActionGet, json.RawMessage([]byte(`{"id":"`+string(rune('a'+i))+`"}`))), json.RawMessage(`{}`))
	}
	assert.LessOrEqual(t, c.size(), 4, "캐시는 최대 크기를 넘지 않아야 함")
}
