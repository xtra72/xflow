// remote_dashboard_query_test.go 는 M10(그룹 L) 노드 측 query 브리지의 대시보드 config
// (get_shared/get_mine) + 시스템 메트릭(monitor.metrics) 매핑을 검증한다
// (@SPEC:SPEC-REMOTE-001 M10, REQ-L01/L05).
//
// READ-ONLY(REQ-J03): dashboard.put/delete 등 변경 의미 action 은 client allowlist 에서
// 거부되므로 본 브리지에 도달하지 않는다. 본 테스트는 노드-로컬 read 매핑만 검증한다.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/storage"
)

// fakeDashboardReader 는 노드-로컬 대시보드 config read 소스 fake 이다(REQ-L01).
type fakeDashboardReader struct {
	snaps map[string]*storage.DashboardSnapshot // key = scope+"|"+owner
	err   error
}

func (f *fakeDashboardReader) Get(_ context.Context, scope, owner string) (*storage.DashboardSnapshot, error) {
	if f.err != nil {
		return nil, f.err
	}
	snap, ok := f.snaps[scope+"|"+owner]
	if !ok {
		return nil, storage.ErrDashboardNotFound
	}
	return snap, nil
}

// fakeMetricsReader 는 노드-로컬 시스템 메트릭 read 소스 fake 이다(REQ-L05).
type fakeMetricsReader struct {
	metrics *handler.MetricsResponse
	err     error
}

func (f *fakeMetricsReader) GetMetrics(_ context.Context) (*handler.MetricsResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.metrics, nil
}

// newDashboardTestSource 는 dashboard/monitor 소스만 바인딩한 query source 를 만든다.
func newDashboardTestSource(dash dashboardReader, metrics metricsReader) *remoteQuerySource {
	s := newRemoteQuerySource(nil, nil, nil, nil, nil)
	s.dashboard = dash
	s.metrics = metrics
	return s
}

// TestQuerySource_DashboardGetShared 는 dashboard.get_shared 가 노드의 공유(global)
// 대시보드 config 를 반환하는지 검증한다(REQ-L01 — GET /dashboards/shared 매핑).
func TestQuerySource_DashboardGetShared(t *testing.T) {
	dash := &fakeDashboardReader{snaps: map[string]*storage.DashboardSnapshot{
		"global|": {Scope: "global", Owner: "", Version: 3, Payload: json.RawMessage(`{"dashboardPages":[{"id":"p1"}]}`)},
	}}
	src := newDashboardTestSource(dash, nil)

	data, err := src.Query(context.Background(), remote.DomainDashboard, remote.QueryActionGetShared, nil)
	require.NoError(t, err)

	var snap storage.DashboardSnapshot
	require.NoError(t, json.Unmarshal(data, &snap))
	assert.Equal(t, "global", snap.Scope)
	assert.Equal(t, int64(3), snap.Version)
	assert.JSONEq(t, `{"dashboardPages":[{"id":"p1"}]}`, string(snap.Payload))
}

// TestQuerySource_DashboardGetMine 는 dashboard.get_mine 가 args.owner 로 노드-로컬
// 사용자 대시보드를 반환하는지 검증한다(REQ-L01/A17 — 노드 권위, deviceId 네임스페이싱).
func TestQuerySource_DashboardGetMine(t *testing.T) {
	dash := &fakeDashboardReader{snaps: map[string]*storage.DashboardSnapshot{
		"user|admin": {Scope: "user", Owner: "admin", Version: 1, Payload: json.RawMessage(`{"activeDashboardId":"d1"}`)},
	}}
	src := newDashboardTestSource(dash, nil)

	args := json.RawMessage(`{"owner":"admin"}`)
	data, err := src.Query(context.Background(), remote.DomainDashboard, remote.QueryActionGetMine, args)
	require.NoError(t, err)

	var snap storage.DashboardSnapshot
	require.NoError(t, json.Unmarshal(data, &snap))
	assert.Equal(t, "user", snap.Scope)
	assert.Equal(t, "admin", snap.Owner)
}

// TestQuerySource_DashboardSharedNotConfigured 는 공유(global) 대시보드 config 미설정
// 노드에서 not-found 가 오류가 아니라 EMPTY 정상 상태로 처리되는지 검증한다(REQ-L02).
//
// 로컬 GET /dashboards/shared 가 404 → UI 기본값을 쓰는 것과 동형으로, 원격 프록시도
// ErrDashboardNotFound 를 502 오류로 전파하지 않고 (nil, nil) 성공 EMPTY 결과로 환원한다.
// 노드는 query_result{ok:true, data:null} 을 전송하고 서버는 200 {data:null} 로 응답한다.
func TestQuerySource_DashboardSharedNotConfigured(t *testing.T) {
	dash := &fakeDashboardReader{snaps: map[string]*storage.DashboardSnapshot{}}
	src := newDashboardTestSource(dash, nil)

	data, err := src.Query(context.Background(), remote.DomainDashboard, remote.QueryActionGetShared, nil)
	require.NoError(t, err, "dashboard not-found 는 오류가 아니라 EMPTY 정상 상태여야 함")
	assert.Empty(t, data, "not-found 는 null/빈 data 로 응답해야 함(ok:true, data:null)")
}

// TestQuerySource_DashboardMineNotConfigured 는 개인(user) 대시보드 config 미설정 시
// not-found 가 오류가 아니라 EMPTY 정상 상태로 처리되는지 검증한다(REQ-L02).
//
// viewing admin 에게 개인(mine) 대시보드가 없는 것은 정상 빈 상태이므로 502 가 아니라
// 200 {data:null} 로 응답해야 한다(프런트는 기본/빈 대시보드를 렌더).
func TestQuerySource_DashboardMineNotConfigured(t *testing.T) {
	dash := &fakeDashboardReader{snaps: map[string]*storage.DashboardSnapshot{}}
	src := newDashboardTestSource(dash, nil)

	args := json.RawMessage(`{"owner":"admin"}`)
	data, err := src.Query(context.Background(), remote.DomainDashboard, remote.QueryActionGetMine, args)
	require.NoError(t, err, "개인 대시보드 not-found 는 오류가 아니라 EMPTY 정상 상태여야 함")
	assert.Empty(t, data, "not-found 는 null/빈 data 로 응답해야 함(ok:true, data:null)")
}

// TestQuerySource_DashboardRealErrorPropagates 는 not-found 가 아닌 실제 리더 오류는
// 그대로 전파되어 서버가 502 로 매핑하도록 보장한다(정상 실패와 빈 상태를 구분).
func TestQuerySource_DashboardRealErrorPropagates(t *testing.T) {
	readerErr := errors.New("dashboard store unavailable")
	for _, action := range []string{remote.QueryActionGetShared, remote.QueryActionGetMine} {
		dash := &fakeDashboardReader{err: readerErr}
		src := newDashboardTestSource(dash, nil)

		_, err := src.Query(context.Background(), remote.DomainDashboard, action, json.RawMessage(`{"owner":"admin"}`))
		require.Errorf(t, err, "dashboard/%s 의 실제 리더 오류는 전파되어야 함", action)
		assert.ErrorIsf(t, err, readerErr, "dashboard/%s 는 원본 리더 오류를 전파해야 함", action)
		assert.Falsef(t, errors.Is(err, storage.ErrDashboardNotFound),
			"dashboard/%s 실제 오류는 not-found 가 아님", action)
	}
}

// TestQuerySource_DashboardMissingReader 는 dashboard 소스 미바인딩 시 미지원 오류를
// 반환하는지 검증한다(노드 보호 — 패닉 금지).
func TestQuerySource_DashboardMissingReader(t *testing.T) {
	src := newRemoteQuerySource(nil, nil, nil, nil, nil)
	_, err := src.Query(context.Background(), remote.DomainDashboard, remote.QueryActionGetShared, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, remote.ErrQueryActionUnsupported))
}

// TestQuerySource_MonitorMetrics 는 monitor.metrics 가 노드의 시스템 메트릭 스냅샷을
// 반환하는지 검증한다(REQ-L05 — GET /monitor/metrics 매핑).
func TestQuerySource_MonitorMetrics(t *testing.T) {
	metrics := &fakeMetricsReader{metrics: &handler.MetricsResponse{
		MemoryUsagePct: 42.5,
		GoRoutines:     17,
		UptimeSeconds:  100,
	}}
	src := newDashboardTestSource(nil, metrics)

	data, err := src.Query(context.Background(), remote.DomainMonitor, remote.QueryActionMetrics, nil)
	require.NoError(t, err)

	var m handler.MetricsResponse
	require.NoError(t, json.Unmarshal(data, &m))
	assert.Equal(t, 42.5, m.MemoryUsagePct)
	assert.Equal(t, 17, m.GoRoutines)
}

// TestQuerySource_MonitorMissingReader 는 monitor 소스 미바인딩 시 미지원 오류를
// 반환하는지 검증한다(노드 보호).
func TestQuerySource_MonitorMissingReader(t *testing.T) {
	src := newRemoteQuerySource(nil, nil, nil, nil, nil)
	_, err := src.Query(context.Background(), remote.DomainMonitor, remote.QueryActionMetrics, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, remote.ErrQueryActionUnsupported))
}

// TestQuerySource_DashboardMutationRejected 는 dashboard 도메인의 변경 의미 action 이
// 미지원으로 거부되는지 검증한다(READ-ONLY — REQ-J03/L01/L12, 와이어 방어선).
//
// 실제로는 client allowlist(IsAllowedQueryAction)가 먼저 거부하나, 브리지 자체도
// 미열거 action 에 대해 ErrQueryActionUnsupported 를 반환해 변경을 수행하지 않는다.
func TestQuerySource_DashboardMutationRejected(t *testing.T) {
	dash := &fakeDashboardReader{snaps: map[string]*storage.DashboardSnapshot{
		"global|": {Scope: "global", Payload: json.RawMessage(`{}`)},
	}}
	src := newDashboardTestSource(dash, nil)

	for _, mut := range []string{"put", "delete", "create", "update"} {
		_, err := src.Query(context.Background(), remote.DomainDashboard, mut, json.RawMessage(`{}`))
		require.Errorf(t, err, "dashboard.%s 변경 action 은 거부되어야 함", mut)
		assert.Truef(t, errors.Is(err, remote.ErrQueryActionUnsupported),
			"dashboard.%s 는 ErrQueryActionUnsupported 여야 함", mut)
	}
}
