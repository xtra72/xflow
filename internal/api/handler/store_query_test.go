package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api"
)

// fakeStoreAgent 는 handler.storeHistoryQueryer 를 구현하는 최소 페이크이다.
// agent.Agent 인터페이스도 구현하여 agent.List() 결과로 동작한다.
type fakeStoreAgent struct {
	*fakeAgentCommon
	queryFn  func(ctx context.Context, namespace, key string, q system.HistoryQuery) ([]system.HistoryEntry, error)
	renameFn func(ctx context.Context, namespace, oldKey, newKey string) (int, error)
}

// RenameKey 는 storeKeyRenamer 계약을 만족한다(테스트용). renameFn 미지정이면 (0, nil).
func (f *fakeStoreAgent) RenameKey(ctx context.Context, namespace, oldKey, newKey string) (int, error) {
	if f.renameFn != nil {
		return f.renameFn(ctx, namespace, oldKey, newKey)
	}
	return 0, nil
}

func (f *fakeStoreAgent) QueryHistory(
	ctx context.Context,
	namespace, key string,
	q system.HistoryQuery,
) ([]system.HistoryEntry, error) {
	if f.queryFn != nil {
		return f.queryFn(ctx, namespace, key, q)
	}
	return nil, nil
}

// nonStoreAgent 는 QueryHistory 를 구현하지 않는 에이전트로, 잘못된 타입의
// 에이전트가 Store 엔드포인트에 지정되었을 때 400 을 반환하는지 확인용이다.
type nonStoreAgent struct {
	*fakeAgentCommon
}

// setupStoreQueryRouter 는 StoreQueryHandler 를 탑재한 테스트 라우터를 만든다.
func setupStoreQueryRouter(t *testing.T, agents ...agent.Agent) *api.Router {
	t.Helper()
	lookup := &fakeAgentLookup{agents: agents}
	router := api.NewRouter()
	h := NewStoreQueryHandler(lookup, nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

func TestStoreQueryHandler_RegisterRoutes(t *testing.T) {
	t.Parallel()
	router := api.NewRouter()
	h := NewStoreQueryHandler(&fakeAgentLookup{}, nil)
	g := router.Group("/api/v1")

	before := router.RouteCount()
	h.RegisterRoutes(g)
	after := router.RouteCount()

	// @spec SPEC-STORE-003:
	//   POST   /query, GET /keys, GET /tags,
	//   DELETE /keys/{key}, DELETE /keys
	// @spec SPEC-STORE-003 v0.4.0:
	//   PUT    /keys/{key}/meta (신규)
	// @spec SPEC-STORE-004:
	//   POST   /keys/{key}/rename (신규)
	// 총 7개.
	assert.Equal(t, 7, after-before)
}

func TestStoreQueryHandler_각모드_성공(t *testing.T) {
	testCases := []struct {
		name string
		body string
	}{
		{"latest", `{"key":"k","mode":"latest"}`},
		{"last_n", `{"key":"k","mode":"last_n","count":2}`},
		{"duration", `{"key":"k","mode":"duration","duration_sec":60}`},
		{"time_range", `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":2000}`},
		{"since_n", `{"key":"k","mode":"since_n","count":5,"start_ms":1500}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var captured system.HistoryQuery
			agentFake := &fakeStoreAgent{
				fakeAgentCommon: newFakeAgent("store-a", "my-store", "store"),
				queryFn: func(_ context.Context, _, _ string, q system.HistoryQuery) ([]system.HistoryEntry, error) {
					captured = q
					return []system.HistoryEntry{
						{Timestamp: time.UnixMilli(1000), Value: 1},
						{Timestamp: time.UnixMilli(2000), Value: 2},
					}, nil
				},
			}
			router := setupStoreQueryRouter(t, agentFake)
			req := httptest.NewRequest(http.MethodPost,
				"/api/v1/store/my-store/query",
				strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.Handler().ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
			resp := decodeQueryResponse(t, rec)
			assert.True(t, resp.Success)
			assert.Len(t, resp.Data.Entries, 2)
			assert.Equal(t, int64(1000), resp.Data.Entries[0].Timestamp)
			assert.Equal(t, float64(1), resp.Data.Entries[0].Value)
			assert.Equal(t, int64(2000), resp.Data.Entries[1].Timestamp)
			assert.Equal(t, 2, resp.Data.Count)
			assert.False(t, resp.Data.Truncated)
			assert.Equal(t, system.QueryMode(tc.name), captured.Mode)
		})
	}
}

func TestStoreQueryHandler_잘못된모드_400(t *testing.T) {
	agentFake := &fakeStoreAgent{fakeAgentCommon: newFakeAgent("s1", "store-a", "store")}
	router := setupStoreQueryRouter(t, agentFake)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/store/store-a/query",
		strings.NewReader(`{"key":"k","mode":"invalid"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestStoreQueryHandler_last_n_count0_400(t *testing.T) {
	agentFake := &fakeStoreAgent{fakeAgentCommon: newFakeAgent("s1", "store-a", "store")}
	router := setupStoreQueryRouter(t, agentFake)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/store/store-a/query",
		strings.NewReader(`{"key":"k","mode":"last_n","count":0}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestStoreQueryHandler_키없음_400(t *testing.T) {
	agentFake := &fakeStoreAgent{fakeAgentCommon: newFakeAgent("s1", "store-a", "store")}
	router := setupStoreQueryRouter(t, agentFake)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/store/store-a/query",
		strings.NewReader(`{"mode":"latest"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestStoreQueryHandler_키없음_빈결과_200 은 QueryHistory 가 system.ErrKeyNotFound 를
// 반환할 때(키 미존재) 핸들러가 500(INTERNAL_ERROR)이 아니라 빈 결과 200 으로
// 응답하는지 검증한다. device_id 변경으로 옛 키를 조회하거나 첫 데이터 전 차트가
// 정상 렌더되도록 하기 위함이다.
func TestStoreQueryHandler_키없음_빈결과_200(t *testing.T) {
	agentFake := &fakeStoreAgent{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		queryFn: func(_ context.Context, _, _ string, _ system.HistoryQuery) ([]system.HistoryEntry, error) {
			return nil, system.ErrKeyNotFound
		},
	}
	router := setupStoreQueryRouter(t, agentFake)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/store/store-a/query",
		strings.NewReader(`{"key":"gone-after-migration","mode":"latest"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "키 미존재는 500 이 아니라 빈 결과 200 이어야 한다")
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Count int `json:"count"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.True(t, resp.Success)
	assert.Equal(t, 0, resp.Data.Count)
}

func TestStoreQueryHandler_에이전트없음_404(t *testing.T) {
	router := setupStoreQueryRouter(t /* no agents */)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/store/missing/query",
		strings.NewReader(`{"key":"k","mode":"latest"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	var resp struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.False(t, resp.Success)
	assert.NotEmpty(t, resp.Error.Code)
}

func TestStoreQueryHandler_스토어가아님_400(t *testing.T) {
	// store 가 아닌 에이전트 (QueryHistory 미구현) 가 지정되면 400 을 반환한다.
	other := &nonStoreAgent{fakeAgentCommon: newFakeAgent("i1", "my-inf", "influxdb")}
	router := setupStoreQueryRouter(t, other)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/store/my-inf/query",
		strings.NewReader(`{"key":"k","mode":"latest"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestStoreQueryHandler_epoch_ms_변환(t *testing.T) {
	// start_ms/end_ms 가 time.UnixMilli() 로 변환되어 HistoryQuery 에 전달되는지.
	var captured system.HistoryQuery
	agentFake := &fakeStoreAgent{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		queryFn: func(_ context.Context, _, _ string, q system.HistoryQuery) ([]system.HistoryEntry, error) {
			captured = q
			return []system.HistoryEntry{}, nil
		},
	}
	router := setupStoreQueryRouter(t, agentFake)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/store/store-a/query",
		strings.NewReader(`{"key":"k","mode":"time_range","start_ms":1600000000000,"end_ms":1600000001000}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assert.Equal(t, int64(1600000000000), captured.From.UnixMilli())
	assert.Equal(t, int64(1600000001000), captured.To.UnixMilli())
}

func TestStoreQueryHandler_응답_Timestamp_epoch_ms(t *testing.T) {
	// 응답의 timestamp 는 반드시 epoch ms (int64) 이어야 한다.
	ts := time.Date(2026, 4, 16, 10, 0, 0, 123_000_000, time.UTC)
	expectedMs := ts.UnixMilli()
	agentFake := &fakeStoreAgent{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		queryFn: func(_ context.Context, _, _ string, _ system.HistoryQuery) ([]system.HistoryEntry, error) {
			return []system.HistoryEntry{
				{Timestamp: ts, Value: "hello"},
			}, nil
		},
	}
	router := setupStoreQueryRouter(t, agentFake)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/store/store-a/query",
		strings.NewReader(`{"key":"k","mode":"latest"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 1)
	assert.Equal(t, expectedMs, resp.Data.Entries[0].Timestamp)
	assert.Equal(t, "hello", resp.Data.Entries[0].Value)
}

// ============================================================================
// SPEC-WEB-005 서버측 집계 테스트.
// ============================================================================

// makeAggFake 는 주어진 엔트리 슬라이스를 반환하는 페이크 Store 에이전트를 만든다.
func makeAggFake(t *testing.T, entries []system.HistoryEntry) *fakeStoreAgent {
	t.Helper()
	return &fakeStoreAgent{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		queryFn: func(_ context.Context, _, _ string, _ system.HistoryQuery) ([]system.HistoryEntry, error) {
			return entries, nil
		},
	}
}

// doAggPOST 는 집계 쿼리 POST 요청을 실행하고 ResponseRecorder 를 반환한다.
func doAggPOST(t *testing.T, router *api.Router, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/store/store-a/query",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	return rec
}

func TestStoreQueryHandler_집계_time_range_avg(t *testing.T) {
	// epoch-zero 정렬: interval=60_000(ms=60s) 이면 버킷 경계는 0, 60_000, 120_000, ...
	// bucket 0 [0, 60_000): 값 10, 20 → avg=15
	// bucket 60_000 [60_000, 120_000): 값 30 → avg=30
	entries := []system.HistoryEntry{
		{Timestamp: time.UnixMilli(1000), Value: float64(10)},
		{Timestamp: time.UnixMilli(30_000), Value: float64(20)},
		{Timestamp: time.UnixMilli(70_000), Value: float64(30)},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	body := `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":121000,` +
		`"interval_ms":60000,"aggregation":"avg"}`
	rec := doAggPOST(t, router, body)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 2)
	assert.Equal(t, int64(0), resp.Data.Entries[0].Timestamp)
	assert.Equal(t, float64(15), resp.Data.Entries[0].Value)
	assert.Equal(t, int64(60_000), resp.Data.Entries[1].Timestamp)
	assert.Equal(t, float64(30), resp.Data.Entries[1].Value)
	assert.Equal(t, 2, resp.Data.Count)
	assert.False(t, resp.Data.Truncated)
}

func TestStoreQueryHandler_집계_time_range_min(t *testing.T) {
	// bucket 0: 값 10, 5, 20 → min=5
	// bucket 1: 값 30 → min=30
	entries := []system.HistoryEntry{
		{Timestamp: time.UnixMilli(1000), Value: float64(10)},
		{Timestamp: time.UnixMilli(30_000), Value: float64(5)},
		{Timestamp: time.UnixMilli(50_000), Value: float64(20)},
		{Timestamp: time.UnixMilli(70_000), Value: float64(30)},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	body := `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":121000,` +
		`"interval_ms":60000,"aggregation":"min"}`
	rec := doAggPOST(t, router, body)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 2)
	assert.Equal(t, float64(5), resp.Data.Entries[0].Value)
	assert.Equal(t, float64(30), resp.Data.Entries[1].Value)
}

func TestStoreQueryHandler_집계_time_range_max(t *testing.T) {
	// bucket 0: 값 10, 5, 20 → max=20
	// bucket 1: 값 25, 30 → max=30
	entries := []system.HistoryEntry{
		{Timestamp: time.UnixMilli(1000), Value: float64(10)},
		{Timestamp: time.UnixMilli(30_000), Value: float64(5)},
		{Timestamp: time.UnixMilli(50_000), Value: float64(20)},
		{Timestamp: time.UnixMilli(70_000), Value: float64(25)},
		{Timestamp: time.UnixMilli(90_000), Value: float64(30)},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	body := `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":121000,` +
		`"interval_ms":60000,"aggregation":"max"}`
	rec := doAggPOST(t, router, body)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 2)
	assert.Equal(t, float64(20), resp.Data.Entries[0].Value)
	assert.Equal(t, float64(30), resp.Data.Entries[1].Value)
}

func TestStoreQueryHandler_집계_duration_avg(t *testing.T) {
	// duration 모드에서도 epoch-zero 벽시계 정렬이 적용된다.
	// 결정성을 확보하기 위해 엔트리를 현재 시각 기준 분 경계에 정확히 배치한다.
	//   - t0 = 2분 전 (분 경계) → bucket t0
	//   - t0+30s            → bucket t0 (같은 1분 내)
	//   - t1 = 1분 전 (분 경계) → bucket t1
	// 두 버킷이 항상 분리되어 avg=15, avg=30 이 결정적으로 나온다.
	now := time.Now()
	nowTrunc := now.Truncate(time.Minute)
	t0 := nowTrunc.Add(-2 * time.Minute)
	t1 := nowTrunc.Add(-1 * time.Minute)
	entries := []system.HistoryEntry{
		{Timestamp: t0, Value: float64(10)},
		{Timestamp: t0.Add(30 * time.Second), Value: float64(20)},
		{Timestamp: t1, Value: float64(30)},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	// duration=300s 면 origin = now-5min 이 되어 t0(=now-2min), t1(=now-1min) 모두
	// origin 필터를 통과한다.
	body := `{"key":"k","mode":"duration","duration_sec":300,` +
		`"interval_ms":60000,"aggregation":"avg"}`
	rec := doAggPOST(t, router, body)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 2)
	assert.Equal(t, t0.UnixMilli(), resp.Data.Entries[0].Timestamp)
	assert.Equal(t, float64(15), resp.Data.Entries[0].Value)
	assert.Equal(t, t1.UnixMilli(), resp.Data.Entries[1].Timestamp)
	assert.Equal(t, float64(30), resp.Data.Entries[1].Value)
}

func TestStoreQueryHandler_집계_빈버킷_생략(t *testing.T) {
	// epoch-zero 정렬: 버킷 경계는 0, 60_000, 120_000, 180_000, ...
	// bucket 0     [0, 60_000)       : 값 10
	// bucket 60_000  [60_000, 120_000): (없음) ← 응답에서 생략
	// bucket 120_000 [120_000, 180_000): 값 30
	entries := []system.HistoryEntry{
		{Timestamp: time.UnixMilli(1000), Value: float64(10)},
		{Timestamp: time.UnixMilli(130_000), Value: float64(30)},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	body := `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":181000,` +
		`"interval_ms":60000,"aggregation":"avg"}`
	rec := doAggPOST(t, router, body)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	// 2개만 반환 (중간 빈 버킷은 null-fill 하지 않고 생략).
	require.Len(t, resp.Data.Entries, 2)
	assert.Equal(t, int64(0), resp.Data.Entries[0].Timestamp)
	assert.Equal(t, float64(10), resp.Data.Entries[0].Value)
	assert.Equal(t, int64(120_000), resp.Data.Entries[1].Timestamp)
	assert.Equal(t, float64(30), resp.Data.Entries[1].Value)
}

func TestStoreQueryHandler_집계_비숫자값_무시(t *testing.T) {
	// 비숫자 값(string, nil) 은 avg 의 분모에 포함되지 않아야 한다.
	// bucket 0 [1000, 61000): "abc"(skip), 10, nil(skip), 20 → avg = (10+20)/2 = 15
	entries := []system.HistoryEntry{
		{Timestamp: time.UnixMilli(1000), Value: "abc"},
		{Timestamp: time.UnixMilli(5000), Value: float64(10)},
		{Timestamp: time.UnixMilli(10_000), Value: nil},
		{Timestamp: time.UnixMilli(30_000), Value: float64(20)},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	body := `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":61000,` +
		`"interval_ms":60000,"aggregation":"avg"}`
	rec := doAggPOST(t, router, body)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 1)
	assert.Equal(t, float64(15), resp.Data.Entries[0].Value)
}

func TestStoreQueryHandler_집계_모든값_비숫자_빈응답(t *testing.T) {
	// 유효 숫자가 없는 버킷은 응답에서 생략된다.
	entries := []system.HistoryEntry{
		{Timestamp: time.UnixMilli(1000), Value: "abc"},
		{Timestamp: time.UnixMilli(5000), Value: nil},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	body := `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":61000,` +
		`"interval_ms":60000,"aggregation":"avg"}`
	rec := doAggPOST(t, router, body)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	assert.Len(t, resp.Data.Entries, 0)
	assert.Equal(t, 0, resp.Data.Count)
}

func TestStoreQueryHandler_집계_잘못된집계자_400(t *testing.T) {
	router := setupStoreQueryRouter(t, makeAggFake(t, nil))
	body := `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":2000,` +
		`"interval_ms":100,"aggregation":"median"}`
	rec := doAggPOST(t, router, body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid aggregation")
}

func TestStoreQueryHandler_집계_interval_음수_400(t *testing.T) {
	router := setupStoreQueryRouter(t, makeAggFake(t, nil))
	body := `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":2000,` +
		`"interval_ms":-1,"aggregation":"avg"}`
	rec := doAggPOST(t, router, body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	// JSON 인코더가 '>' 를 > 로 이스케이프할 수 있으므로 핵심 토큰으로 검증한다.
	assert.Contains(t, rec.Body.String(), "interval_ms must be")
}

func TestStoreQueryHandler_집계_latest모드_400(t *testing.T) {
	router := setupStoreQueryRouter(t, makeAggFake(t, nil))
	body := `{"key":"k","mode":"latest","interval_ms":60000,"aggregation":"avg"}`
	rec := doAggPOST(t, router, body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "aggregation requires mode=time_range or duration")
}

func TestStoreQueryHandler_집계_last_n모드_400(t *testing.T) {
	router := setupStoreQueryRouter(t, makeAggFake(t, nil))
	body := `{"key":"k","mode":"last_n","count":10,"interval_ms":60000,"aggregation":"avg"}`
	rec := doAggPOST(t, router, body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "aggregation requires mode=time_range or duration")
}

func TestStoreQueryHandler_집계_since_n모드_400(t *testing.T) {
	router := setupStoreQueryRouter(t, makeAggFake(t, nil))
	body := `{"key":"k","mode":"since_n","count":5,"start_ms":1500,` +
		`"interval_ms":60000,"aggregation":"avg"}`
	rec := doAggPOST(t, router, body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "aggregation requires mode=time_range or duration")
}

func TestStoreQueryHandler_집계_버킷수_상한_400(t *testing.T) {
	router := setupStoreQueryRouter(t, makeAggFake(t, nil))
	// span=200,000,000,000 ms, interval=1 ms → num_buckets > 100_000
	body := `{"key":"k","mode":"time_range","start_ms":1,"end_ms":200000000001,` +
		`"interval_ms":1,"aggregation":"avg"}`
	rec := doAggPOST(t, router, body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "too many buckets")
}

func TestStoreQueryHandler_집계_정수값_변환(t *testing.T) {
	// int / int64 / float32 등 float64 이외의 숫자 타입도 합산되어야 한다.
	entries := []system.HistoryEntry{
		{Timestamp: time.UnixMilli(1000), Value: int(10)},
		{Timestamp: time.UnixMilli(5000), Value: int64(20)},
		{Timestamp: time.UnixMilli(10_000), Value: float32(30)},
		{Timestamp: time.UnixMilli(20_000), Value: int32(40)},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	body := `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":61000,` +
		`"interval_ms":60000,"aggregation":"avg"}`
	rec := doAggPOST(t, router, body)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 1)
	// (10 + 20 + 30 + 40) / 4 = 25
	assert.Equal(t, float64(25), resp.Data.Entries[0].Value)
}

func TestStoreQueryHandler_집계_boolean값_1_0_변환(t *testing.T) {
	// boolean data_type 시리즈는 서버 집계 경로에서도 true→1 / false→0 으로
	// 변환되어 라인 차트에 표시되어야 한다. 이전에는 toFloat64 가 bool 을 처리하지
	// 못해 엔트리가 통째로 스킵되어 "boolean 타입 선택 시 출력 안 됨" 버그가 있었다.
	entries := []system.HistoryEntry{
		{Timestamp: time.UnixMilli(1000), Value: true},
		{Timestamp: time.UnixMilli(5000), Value: false},
		{Timestamp: time.UnixMilli(10_000), Value: true},
		{Timestamp: time.UnixMilli(20_000), Value: true},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	body := `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":61000,` +
		`"interval_ms":60000,"aggregation":"avg"}`
	rec := doAggPOST(t, router, body)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 1)
	// (1 + 0 + 1 + 1) / 4 = 0.75
	assert.InDelta(t, 0.75, resp.Data.Entries[0].Value, 1e-9)
}

// ---------------------------------------------------------------------------
// 벽시계 정렬 (epoch-zero alignment) 테스트.
//
// 사용자 시작 시각이 인터벌 경계와 어긋나도, 버킷은 항상 epoch 0 기준 벽시계
// 경계에 정렬되어야 한다.
//   - 1m: 초 = 0
//   - 5m: 분 = 0/5/10/.../55, 초 = 0
//   - 1h: 분 = 0, 초 = 0
//   - 1d: UTC 자정 (00:00:00 UTC)
// ---------------------------------------------------------------------------

func TestStoreQueryHandler_집계_1m_벽시계정렬(t *testing.T) {
	// start = 14:23:45 (인터벌 경계와 어긋남), interval = 1m.
	// 엔트리 14:23:45 → bucket 14:23:00 (초=0)
	// 엔트리 14:24:30 → bucket 14:24:00 (초=0)
	startTime := time.Date(2026, 4, 26, 14, 23, 45, 0, time.UTC)
	entries := []system.HistoryEntry{
		{Timestamp: time.Date(2026, 4, 26, 14, 23, 45, 0, time.UTC), Value: float64(10)},
		{Timestamp: time.Date(2026, 4, 26, 14, 24, 30, 0, time.UTC), Value: float64(20)},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	endTime := time.Date(2026, 4, 26, 14, 30, 0, 0, time.UTC)
	body := makeAggBody(startTime, endTime, 60_000, "avg")
	rec := doAggPOST(t, router, body)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 2)
	// 벽시계 분 경계: 14:23:00 와 14:24:00 (UTC).
	expected1 := time.Date(2026, 4, 26, 14, 23, 0, 0, time.UTC).UnixMilli()
	expected2 := time.Date(2026, 4, 26, 14, 24, 0, 0, time.UTC).UnixMilli()
	assert.Equal(t, expected1, resp.Data.Entries[0].Timestamp, "1m bucket should align to seconds=0")
	assert.Equal(t, float64(10), resp.Data.Entries[0].Value)
	assert.Equal(t, expected2, resp.Data.Entries[1].Timestamp, "1m bucket should align to seconds=0")
	assert.Equal(t, float64(20), resp.Data.Entries[1].Value)
}

func TestStoreQueryHandler_집계_5m_벽시계정렬(t *testing.T) {
	// 14:23:45 → bucket 14:20:00 (분 mod 5 = 0, 초 = 0)
	startTime := time.Date(2026, 4, 26, 14, 23, 45, 0, time.UTC)
	entries := []system.HistoryEntry{
		{Timestamp: time.Date(2026, 4, 26, 14, 23, 45, 0, time.UTC), Value: float64(10)},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	endTime := time.Date(2026, 4, 26, 14, 30, 0, 0, time.UTC)
	body := makeAggBody(startTime, endTime, 5*60_000, "avg")
	rec := doAggPOST(t, router, body)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 1)
	expected := time.Date(2026, 4, 26, 14, 20, 0, 0, time.UTC).UnixMilli()
	assert.Equal(t, expected, resp.Data.Entries[0].Timestamp, "5m bucket should align to minutes mod 5 = 0")
	assert.Equal(t, float64(10), resp.Data.Entries[0].Value)
}

func TestStoreQueryHandler_집계_1h_벽시계정렬(t *testing.T) {
	// 14:23:45 → bucket 14:00:00 (분 = 0, 초 = 0)
	startTime := time.Date(2026, 4, 26, 14, 23, 45, 0, time.UTC)
	entries := []system.HistoryEntry{
		{Timestamp: time.Date(2026, 4, 26, 14, 23, 45, 0, time.UTC), Value: float64(10)},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	endTime := time.Date(2026, 4, 26, 16, 0, 0, 0, time.UTC)
	body := makeAggBody(startTime, endTime, 3600_000, "avg")
	rec := doAggPOST(t, router, body)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 1)
	expected := time.Date(2026, 4, 26, 14, 0, 0, 0, time.UTC).UnixMilli()
	assert.Equal(t, expected, resp.Data.Entries[0].Timestamp, "1h bucket should align to minutes=0 seconds=0")
	assert.Equal(t, float64(10), resp.Data.Entries[0].Value)
}

// makeAggBody 는 time_range + 집계 모드의 요청 바디를 만든다.
// 테스트 가독성을 위해 time.Time 을 받아 epoch ms 로 변환한다.
func makeAggBody(start, end time.Time, intervalMs int64, agg string) string {
	return fmt.Sprintf(
		`{"key":"k","mode":"time_range","start_ms":%d,"end_ms":%d,"interval_ms":%d,"aggregation":"%s"}`,
		start.UnixMilli(), end.UnixMilli(), intervalMs, agg,
	)
}

// ---------------------------------------------------------------------------
// toFloat64 단위 테스트 (다양한 숫자 타입 분기 커버).
// ---------------------------------------------------------------------------

func TestToFloat64_숫자타입_테이블(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want float64
		ok   bool
	}{
		{"float64", float64(3.14), 3.14, true},
		{"float32", float32(2.5), 2.5, true},
		{"int", int(42), 42, true},
		{"int8", int8(7), 7, true},
		{"int16", int16(123), 123, true},
		{"int32", int32(-5), -5, true},
		{"int64", int64(1_000_000_000_000), 1_000_000_000_000, true},
		{"uint", uint(10), 10, true},
		{"uint8", uint8(255), 255, true},
		{"uint16", uint16(65535), 65535, true},
		{"uint32", uint32(1), 1, true},
		{"uint64", uint64(2), 2, true},
		{"string은_실패", "abc", 0, false},
		{"nil은_실패", nil, 0, false},
		{"bool_true는_1", true, 1, true},
		{"bool_false는_0", false, 0, true},
		{"NaN_실패", math.NaN(), 0, false},
		{"Inf_실패", math.Inf(1), 0, false},
		{"NegInf_실패", math.Inf(-1), 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := toFloat64(tc.in)
			assert.Equal(t, tc.ok, ok)
			if tc.ok {
				assert.InDelta(t, tc.want, got, 1e-9)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// buildHistoryQuery 오류 경로 테이블 (커버리지).
// ---------------------------------------------------------------------------

func TestStoreQueryHandler_buildHistoryQuery_오류경로(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"duration_음수", `{"key":"k","mode":"duration","duration_sec":0}`},
		{"time_range_start0", `{"key":"k","mode":"time_range","start_ms":0,"end_ms":100}`},
		{"time_range_end_lt_start",
			`{"key":"k","mode":"time_range","start_ms":200,"end_ms":100}`},
		{"since_n_start0",
			`{"key":"k","mode":"since_n","count":5,"start_ms":0}`},
		{"since_n_count0",
			`{"key":"k","mode":"since_n","count":0,"start_ms":1000}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agentFake := &fakeStoreAgent{fakeAgentCommon: newFakeAgent("s1", "store-a", "store")}
			router := setupStoreQueryRouter(t, agentFake)
			req := httptest.NewRequest(http.MethodPost,
				"/api/v1/store/store-a/query",
				strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.Handler().ServeHTTP(rec, req)
			assert.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
		})
	}
}

// ---------------------------------------------------------------------------
// ListKeys 엔드포인트 테스트 (기존 기능, 커버리지 확보).
// ---------------------------------------------------------------------------

// fakeKeyLister 는 storeKeyLister 를 구현하는 페이크이다.
type fakeKeyLister struct {
	*fakeAgentCommon
	listFn func(ctx context.Context, namespace, pattern string) ([]string, error)
}

func (f *fakeKeyLister) ListStoreKeys(ctx context.Context, namespace, pattern string) ([]string, error) {
	if f.listFn != nil {
		return f.listFn(ctx, namespace, pattern)
	}
	return nil, nil
}

// @spec SPEC-STORE-003 v0.3.0:
// TestStoreQueryHandler_ListKeys_성공 / _정적키없음_빈배열 은 v0.3.0 객체 배열 응답
// 검증으로 마이그레이션되어 store_query_listkeys_test.go 에서 다룬다.

func TestStoreQueryHandler_ListKeys_에이전트없음_404(t *testing.T) {
	router := setupStoreQueryRouter(t /* no agents */)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/missing/keys", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestStoreQueryHandler_ListKeys_스토어아님_400(t *testing.T) {
	other := &nonStoreAgent{fakeAgentCommon: newFakeAgent("i1", "my-inf", "influxdb")}
	router := setupStoreQueryRouter(t, other)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/my-inf/keys", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---------------------------------------------------------------------------
// @spec SPEC-STORE-003: 정적 키 + 태그 메타데이터 + 필터링 테스트.
// ---------------------------------------------------------------------------

// fakeKeyTagLister 는 storeKeyTagLister 를 구현하는 페이크이다.
// KeyTags 와 StaticTagPairs 는 생성자에서 주입된 정적 맵을 그대로 반환한다.
type fakeKeyTagLister struct {
	*fakeAgentCommon
	listFn  func(ctx context.Context, namespace, pattern string) ([]string, error)
	tags    map[string]map[string]string
	tagsErr error
}

func (f *fakeKeyTagLister) ListStoreKeys(ctx context.Context, namespace, pattern string) ([]string, error) {
	if f.listFn != nil {
		return f.listFn(ctx, namespace, pattern)
	}
	return nil, nil
}

func (f *fakeKeyTagLister) KeyTags(_ context.Context) (map[string]map[string]string, error) {
	if f.tagsErr != nil {
		return nil, f.tagsErr
	}
	// 반환 복사본 (핸들러가 원본을 변조하지 않는지 확인용).
	out := make(map[string]map[string]string, len(f.tags))
	for k, m := range f.tags {
		c := make(map[string]string, len(m))
		for tk, tv := range m {
			c[tk] = tv
		}
		out[k] = c
	}
	return out, nil
}

func (f *fakeKeyTagLister) StaticTagPairs() map[string][]string {
	pairs := map[string]map[string]struct{}{}
	for _, tags := range f.tags {
		for tk, tv := range tags {
			if _, ok := pairs[tk]; !ok {
				pairs[tk] = map[string]struct{}{}
			}
			pairs[tk][tv] = struct{}{}
		}
	}
	out := make(map[string][]string, len(pairs))
	for tk, vs := range pairs {
		list := make([]string, 0, len(vs))
		for v := range vs {
			list = append(list, v)
		}
		// 정렬하여 결정적 순서를 보장.
		for i := 1; i < len(list); i++ {
			for j := i; j > 0 && list[j-1] > list[j]; j-- {
				list[j-1], list[j] = list[j], list[j-1]
			}
		}
		out[tk] = list
	}
	return out
}

// @spec SPEC-STORE-003 v0.3.0 — Phase D BREAKING:
// listKeysResponse / decodeListKeys 는 v0.2.0 응답 형상 (keys: []string + tags: map)
// 검증용이었으므로 store_query_listkeys_phase_e_test.go 로 이전되었다.
// Phase E 가 v0.3.0 응답 객체 배열 검증으로 마이그레이션할 때 사용한다.

// listTagsResponse 는 /tags 엔드포인트 응답 구조이다.
type listTagsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Pairs []struct {
			Key    string   `json:"key"`
			Values []string `json:"values"`
		} `json:"pairs"`
	} `json:"data"`
}

func decodeListTags(t *testing.T, rec *httptest.ResponseRecorder) listTagsResponse {
	t.Helper()
	var r listTagsResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	return r
}

// @spec SPEC-STORE-003 v0.3.0:
// TestStoreQueryHandler_ListKeys_정적키_태그_포함, _태그필터_(단일/다중_AND/매칭없음/
// 잘못된형식/value내콜론) 6개 테스트는 v0.3.0 객체 배열 응답으로 마이그레이션되어
// store_query_listkeys_test.go 에서 다룬다.
// (`정적키없음_tags_생략` 은 v0.3.0 에서 더 이상 적용되지 않아 제거됨 — M9 의
// "tags 는 항상 객체" 규칙에 의해 자동 등록 키도 빈 객체 `{}` 로 응답된다.)

func TestStoreQueryHandler_ListTags_정렬된_pairs(t *testing.T) {
	ag := &fakeKeyTagLister{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		tags: map[string]map[string]string{
			"k1": {"room": "2", "type": "humidity"},
			"k2": {"room": "1", "type": "temperature"},
			"k3": {"room": "3", "type": "temperature"},
		},
	}
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/store-a/tags", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeListTags(t, rec)
	// key 오름차순: room, type
	require.Len(t, resp.Data.Pairs, 2)
	assert.Equal(t, "room", resp.Data.Pairs[0].Key)
	assert.Equal(t, []string{"1", "2", "3"}, resp.Data.Pairs[0].Values)
	assert.Equal(t, "type", resp.Data.Pairs[1].Key)
	assert.Equal(t, []string{"humidity", "temperature"}, resp.Data.Pairs[1].Values)
}

func TestStoreQueryHandler_ListTags_정적키없음_빈pairs(t *testing.T) {
	ag := &fakeKeyTagLister{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		tags:            map[string]map[string]string{},
	}
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/store-a/tags", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeListTags(t, rec)
	assert.Empty(t, resp.Data.Pairs)
}

func TestStoreQueryHandler_ListTags_에이전트없음_404(t *testing.T) {
	router := setupStoreQueryRouter(t /* no agents */)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/missing/tags", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestStoreQueryHandler_ListTags_스토어아님_400(t *testing.T) {
	// nonStoreAgent 는 storeKeyTagLister 를 구현하지 않는다.
	other := &nonStoreAgent{fakeAgentCommon: newFakeAgent("i1", "my-inf", "influxdb")}
	router := setupStoreQueryRouter(t, other)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/my-inf/tags", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestStoreQueryHandler_집계_파라미터불완전_레거시경로(t *testing.T) {
	// interval_ms 만 있고 aggregation 이 없으면 집계하지 않는다 (레거시 원시 응답).
	entries := []system.HistoryEntry{
		{Timestamp: time.UnixMilli(1000), Value: float64(10)},
		{Timestamp: time.UnixMilli(30_000), Value: float64(20)},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	body := `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":61000,"interval_ms":60000}`
	rec := doAggPOST(t, router, body)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	// 집계되지 않고 원시 엔트리가 그대로 반환되어야 한다.
	require.Len(t, resp.Data.Entries, 2)
	assert.Equal(t, int64(1000), resp.Data.Entries[0].Timestamp)
	assert.Equal(t, float64(10), resp.Data.Entries[0].Value)
	assert.Equal(t, int64(30_000), resp.Data.Entries[1].Timestamp)
	assert.Equal(t, float64(20), resp.Data.Entries[1].Value)
}

// ===========================================================================
// @spec SPEC-STORE-003: Reset 엔드포인트 (DELETE /keys, DELETE /keys/{key}).
//
// 정책:
//   - 정적 키 reset → ClearHistory (엔트리 보존, 히스토리만 비움)
//   - 동적 키 reset → DeleteEntry (엔트리+히스토리 모두 삭제)
//
// reset 은 config.staticKeys 정의 자체를 변경하지 않는다.
// ===========================================================================

// fakeStoreResetter 는 storeResetter 인터페이스를 구현하는 페이크 에이전트이다.
// agent.Agent 의 공통 메서드는 fakeAgentCommon 임베딩으로 채운다.
type fakeStoreResetter struct {
	*fakeAgentCommon

	// 정적 키 목록 (사용자 관점 키).
	staticKeys map[string]struct{}

	// 가상 저장소: namespace → key → exists.
	// 키마다 정적 여부와 무관하게 단순 존재 여부만 추적하며, ClearHistory 와 DeleteEntry 의
	// 호출 흐름을 검증하기 위한 최소 상태이다.
	store map[string]map[string]bool

	// 호출 추적.
	clearHistoryCalls []string // namespace+":"+key
	deleteEntryCalls  []string

	// 외부에서 주입한 키 목록 (ListStoreKeys 가 그대로 반환).
	listKeys []string
	listErr  error

	// ClearHistory / DeleteEntry 가 강제로 반환할 에러 (테스트별 시뮬레이션).
	forceClearErr  error
	forceDeleteErr error
}

func newFakeStoreResetter(id, name string, statics ...string) *fakeStoreResetter {
	sk := make(map[string]struct{}, len(statics))
	for _, k := range statics {
		sk[k] = struct{}{}
	}
	return &fakeStoreResetter{
		fakeAgentCommon: newFakeAgent(id, name, "store"),
		staticKeys:      sk,
		store:           map[string]map[string]bool{},
	}
}

func (f *fakeStoreResetter) seed(namespace string, keys ...string) {
	if _, ok := f.store[namespace]; !ok {
		f.store[namespace] = map[string]bool{}
	}
	for _, k := range keys {
		f.store[namespace][k] = true
	}
	f.listKeys = append(f.listKeys, keys...)
}

func (f *fakeStoreResetter) IsStaticKey(key string) bool {
	_, ok := f.staticKeys[key]
	return ok
}

func (f *fakeStoreResetter) ClearHistory(_ context.Context, namespace, key string) error {
	if f.forceClearErr != nil {
		return f.forceClearErr
	}
	if namespace == "" {
		namespace = "default"
	}
	ns := f.store[namespace]
	if ns == nil || !ns[key] {
		return system.ErrKeyNotFound
	}
	f.clearHistoryCalls = append(f.clearHistoryCalls, namespace+":"+key)
	return nil
}

func (f *fakeStoreResetter) DeleteEntry(_ context.Context, namespace, key string) error {
	if f.forceDeleteErr != nil {
		return f.forceDeleteErr
	}
	if namespace == "" {
		namespace = "default"
	}
	ns := f.store[namespace]
	if ns == nil || !ns[key] {
		return system.ErrKeyNotFound
	}
	delete(ns, key)
	f.deleteEntryCalls = append(f.deleteEntryCalls, namespace+":"+key)
	return nil
}

func (f *fakeStoreResetter) ListStoreKeys(_ context.Context, _, _ string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]string, len(f.listKeys))
	copy(out, f.listKeys)
	return out, nil
}

// resetKeyResponse 는 단일 키 reset 응답이다.
type resetKeyResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Action string `json:"action"`
		Key    string `json:"key"`
	} `json:"data"`
}

// resetAllResponse 는 bulk reset 응답이다.
type resetAllResponse struct {
	Success bool `json:"success"`
	Data    struct {
		HistoryCleared int `json:"history_cleared"`
		EntriesDeleted int `json:"entries_deleted"`
	} `json:"data"`
}

func decodeResetKey(t *testing.T, rec *httptest.ResponseRecorder) resetKeyResponse {
	t.Helper()
	var r resetKeyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	return r
}

func decodeResetAll(t *testing.T, rec *httptest.ResponseRecorder) resetAllResponse {
	t.Helper()
	var r resetAllResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	return r
}

func TestStoreQueryHandler_ResetKey_StaticKey_HistoryCleared(t *testing.T) {
	t.Parallel()
	ag := newFakeStoreResetter("s1", "store-a", "indoor:1:room_temp")
	ag.seed("default", "indoor:1:room_temp")
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/store/store-a/keys/indoor:1:room_temp", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeResetKey(t, rec)
	assert.True(t, resp.Success)
	assert.Equal(t, "history_cleared", resp.Data.Action)
	assert.Equal(t, "indoor:1:room_temp", resp.Data.Key)
	assert.Equal(t, []string{"default:indoor:1:room_temp"}, ag.clearHistoryCalls)
	assert.Empty(t, ag.deleteEntryCalls, "정적 키는 Delete 를 호출하지 않아야 한다")
}

func TestStoreQueryHandler_ResetKey_DynamicKey_EntryDeleted(t *testing.T) {
	t.Parallel()
	ag := newFakeStoreResetter("s1", "store-a" /* no static keys */)
	ag.seed("default", "dyn_key")
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/store/store-a/keys/dyn_key", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeResetKey(t, rec)
	assert.True(t, resp.Success)
	assert.Equal(t, "entry_deleted", resp.Data.Action)
	assert.Equal(t, "dyn_key", resp.Data.Key)
	assert.Equal(t, []string{"default:dyn_key"}, ag.deleteEntryCalls)
	assert.Empty(t, ag.clearHistoryCalls)
}

func TestStoreQueryHandler_ResetKey_NotFound_404(t *testing.T) {
	t.Parallel()
	ag := newFakeStoreResetter("s1", "store-a", "static_key")
	// store 가 비어있음 → ClearHistory 가 ErrKeyNotFound 반환.
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/store/store-a/keys/static_key", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code, "body=%s", rec.Body.String())
}

func TestStoreQueryHandler_ResetKey_NotStoreAgent_400(t *testing.T) {
	t.Parallel()
	other := &nonStoreAgent{fakeAgentCommon: newFakeAgent("i1", "my-inf", "influxdb")}
	router := setupStoreQueryRouter(t, other)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/store/my-inf/keys/whatever", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestStoreQueryHandler_ResetKey_AgentNotFound_404(t *testing.T) {
	t.Parallel()
	router := setupStoreQueryRouter(t /* no agents */)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/store/missing/keys/k", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestStoreQueryHandler_ResetKey_PercentEncodedColon(t *testing.T) {
	// URL 경로에서 정적 키 "indoor:1:room_temp" 가 콜론을 %3A 로 인코딩하여 전달되어도
	// 핸들러가 PathUnescape 로 디코딩해 정확한 키를 사용하는지 검증한다.
	t.Parallel()
	ag := newFakeStoreResetter("s1", "store-a", "indoor:1:room_temp")
	ag.seed("default", "indoor:1:room_temp")
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/store/store-a/keys/indoor%3A1%3Aroom_temp", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeResetKey(t, rec)
	assert.Equal(t, "history_cleared", resp.Data.Action)
	assert.Equal(t, "indoor:1:room_temp", resp.Data.Key)
}

func TestStoreQueryHandler_ResetKey_NamespaceQuery(t *testing.T) {
	t.Parallel()
	ag := newFakeStoreResetter("s1", "store-a", "k")
	ag.seed("ns1", "k")
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/store/store-a/keys/k?namespace=ns1", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assert.Equal(t, []string{"ns1:k"}, ag.clearHistoryCalls)
}

func TestStoreQueryHandler_ResetAll_Mixed(t *testing.T) {
	// 정적 2 + 동적 3 → history_cleared=2, entries_deleted=3.
	t.Parallel()
	ag := newFakeStoreResetter("s1", "store-a", "static1", "static2")
	ag.seed("default", "static1", "static2", "dyn1", "dyn2", "dyn3")
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/store/store-a/keys", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeResetAll(t, rec)
	assert.True(t, resp.Success)
	assert.Equal(t, 2, resp.Data.HistoryCleared)
	assert.Equal(t, 3, resp.Data.EntriesDeleted)
	assert.ElementsMatch(t,
		[]string{"default:static1", "default:static2"}, ag.clearHistoryCalls)
	assert.ElementsMatch(t,
		[]string{"default:dyn1", "default:dyn2", "default:dyn3"}, ag.deleteEntryCalls)
}

func TestStoreQueryHandler_ResetAll_Empty(t *testing.T) {
	t.Parallel()
	ag := newFakeStoreResetter("s1", "store-a")
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/store/store-a/keys", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeResetAll(t, rec)
	assert.True(t, resp.Success)
	assert.Equal(t, 0, resp.Data.HistoryCleared)
	assert.Equal(t, 0, resp.Data.EntriesDeleted)
}

func TestStoreQueryHandler_ResetAll_NotStoreAgent_400(t *testing.T) {
	t.Parallel()
	other := &nonStoreAgent{fakeAgentCommon: newFakeAgent("i1", "my-inf", "influxdb")}
	router := setupStoreQueryRouter(t, other)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/store/my-inf/keys", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestStoreQueryHandler_ResetAll_AgentNotFound_404(t *testing.T) {
	t.Parallel()
	router := setupStoreQueryRouter(t /* no agents */)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/store/missing/keys", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
