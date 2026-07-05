// query_dispatch_m10_test.go 는 M10(그룹 L) 서버 측 dashboard/metrics READ 디스패치의
// 왕복·TTL 캐시·라이브 우회를 검증한다(@SPEC:SPEC-REMOTE-001 M10, REQ-L01/L02/L05/J16).
//
// dashboard.get_shared/get_mine·monitor.metrics 는 완만 변동이므로 단기 TTL 캐시 대상
// (IsStreamableAction=false). chart/monitor.logs 는 스트림 경로(streamManager)이므로
// query 디스패치/캐시를 거치지 않는다(별도 검증 — stream_proxy 경로).
package remote

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDispatchQuery_DashboardSharedRoundTrip 은 dashboard.get_shared 가 노드 왕복으로
// config 를 반환하는지 검증한다(REQ-L01).
func TestDispatchQuery_DashboardSharedRoundTrip(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-l1")
	defer cancel()

	errCh := make(chan error, 1)
	resCh := make(chan json.RawMessage, 1)
	go func() {
		res, err := srv.DispatchQuery(context.Background(), "node-l1",
			DomainDashboard, QueryActionGetShared, nil)
		resCh <- res
		errCh <- err
	}()
	q := readDispatchedQuery(t, conn)
	assert.Equal(t, DomainDashboard, q.Domain)
	assert.Equal(t, QueryActionGetShared, q.QueryAction)
	injectQueryResult(t, conn, q.QueryID, true, json.RawMessage(`{"scope":"global"}`), "")
	require.NoError(t, <-errCh)
	assert.JSONEq(t, `{"scope":"global"}`, string(<-resCh))
}

// TestDispatchQuery_DashboardCacheHit 은 dashboard.get_shared 가 TTL 내 캐시되어
// 노드를 재호출하지 않는지 검증한다(REQ-J16/L02 — config 는 완만 변동, 캐시 대상).
func TestDispatchQuery_DashboardCacheHit(t *testing.T) {
	srv, conn, cancel := approvedNodeConn(t, "node-l2")
	defer cancel()

	// 1차: 노드 왕복.
	errCh := make(chan error, 1)
	go func() {
		_, err := srv.DispatchQuery(context.Background(), "node-l2",
			DomainDashboard, QueryActionGetShared, nil)
		errCh <- err
	}()
	q := readDispatchedQuery(t, conn)
	injectQueryResult(t, conn, q.QueryID, true, json.RawMessage(`{"scope":"global"}`), "")
	require.NoError(t, <-errCh)

	// 2차: 동일 질의 — 캐시 적중(노드 미호출).
	res2, err2 := srv.DispatchQuery(context.Background(), "node-l2",
		DomainDashboard, QueryActionGetShared, nil)
	require.NoError(t, err2)
	assert.JSONEq(t, `{"scope":"global"}`, string(res2))

	select {
	case <-conn.outgoing:
		t.Fatal("dashboard config 캐시 적중인데 노드로 query 가 전송됨")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestDispatchQuery_MetricsCacheable 은 monitor.metrics 가 캐시 대상(완만 변동)인지
// 검증한다(REQ-L05/J16). 라이브 action(stream)이 아니므로 IsStreamableAction=false.
func TestDispatchQuery_MetricsCacheable(t *testing.T) {
	assert.False(t, IsStreamableAction(DomainMonitor, QueryActionMetrics),
		"monitor.metrics 는 캐시 대상(완만 변동) — 스트림이 아님")

	srv, conn, cancel := approvedNodeConn(t, "node-l3")
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := srv.DispatchQuery(context.Background(), "node-l3",
			DomainMonitor, QueryActionMetrics, nil)
		errCh <- err
	}()
	q := readDispatchedQuery(t, conn)
	injectQueryResult(t, conn, q.QueryID, true, json.RawMessage(`{"go_routines":7}`), "")
	require.NoError(t, <-errCh)

	// 2차: 캐시 적중.
	res2, err2 := srv.DispatchQuery(context.Background(), "node-l3",
		DomainMonitor, QueryActionMetrics, nil)
	require.NoError(t, err2)
	assert.Contains(t, string(res2), "go_routines")
	select {
	case <-conn.outgoing:
		t.Fatal("metrics 캐시 적중인데 노드로 query 가 전송됨")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestDispatchQuery_DashboardNotManaged 은 미관리 노드 dashboard 질의가 거절되는지
// 검증한다(REQ-L02 → 503).
func TestDispatchQuery_DashboardNotManaged(t *testing.T) {
	srv := NewServer(ServerConfig{}, nil)
	_, err := srv.DispatchQuery(context.Background(), "ghost-l",
		DomainDashboard, QueryActionGetShared, nil)
	require.ErrorIs(t, err, ErrNodeNotManaged)
}
