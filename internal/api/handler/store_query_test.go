package handler

import (
	"context"
	"encoding/json"
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
	queryFn func(ctx context.Context, namespace, key string, q system.HistoryQuery) ([]system.HistoryEntry, error)
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

	assert.Equal(t, 1, after-before)
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
