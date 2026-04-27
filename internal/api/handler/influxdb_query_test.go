package handler

import (
	"context"
	"encoding/json"
	"errors"
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

// fakeInfluxDBAgent 는 influxFluxQueryer 를 구현하는 테스트용 페이크이다.
type fakeInfluxDBAgent struct {
	*fakeAgentCommon
	fluxFn    func(ctx context.Context, q string) ([]system.InfluxQueryResult, error)
	influxQLFn func(ctx context.Context, q string) ([]system.InfluxQueryResult, error)
}

func (f *fakeInfluxDBAgent) ExecuteFluxQuery(ctx context.Context, q string) ([]system.InfluxQueryResult, error) {
	if f.fluxFn != nil {
		return f.fluxFn(ctx, q)
	}
	return nil, nil
}

func (f *fakeInfluxDBAgent) ExecuteInfluxQLQuery(ctx context.Context, q string) ([]system.InfluxQueryResult, error) {
	if f.influxQLFn != nil {
		return f.influxQLFn(ctx, q)
	}
	return nil, nil
}

// setupInfluxDBQueryRouter 는 InfluxDBQueryHandler 를 탑재한 테스트 라우터를 생성한다.
// 가변 인자 agents 에는 agent.Agent 를 만족하는 페이크를 전달한다.
func setupInfluxDBQueryRouter(t *testing.T, agents ...agent.Agent) *api.Router {
	t.Helper()
	lookup := &fakeAgentLookup{agents: agents}
	router := api.NewRouter()
	h := NewInfluxDBQueryHandler(lookup, nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

func TestInfluxDBQueryHandler_RegisterRoutes(t *testing.T) {
	t.Parallel()
	router := api.NewRouter()
	h := NewInfluxDBQueryHandler(&fakeAgentLookup{}, nil)
	g := router.Group("/api/v1")
	before := router.RouteCount()
	h.RegisterRoutes(g)
	assert.Equal(t, 1, router.RouteCount()-before)
}

func TestInfluxDBQueryHandler_Flux_성공(t *testing.T) {
	ts := time.Date(2026, 4, 16, 10, 0, 0, 456_000_000, time.UTC)
	expectedMs := ts.UnixMilli()
	var capturedQuery string
	agentFake := &fakeInfluxDBAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		fluxFn: func(_ context.Context, q string) ([]system.InfluxQueryResult, error) {
			capturedQuery = q
			return []system.InfluxQueryResult{
				{
					Time:  ts,
					Value: 12.3,
					Labels: map[string]string{
						"_measurement": "cpu",
						"host":         "a",
					},
				},
			}, nil
		},
	}
	router := setupInfluxDBQueryRouter(t, agentFake)

	body := `{"query_language":"flux","query":"from(bucket:\"x\") |> range(start:-1h)"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/influxdb/metrics/query",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 1)
	assert.Equal(t, expectedMs, resp.Data.Entries[0].Timestamp)
	assert.Equal(t, 12.3, resp.Data.Entries[0].Value)
	assert.Equal(t, "cpu", resp.Data.Entries[0].Labels["_measurement"])
	assert.Equal(t, "a", resp.Data.Entries[0].Labels["host"])
	assert.Equal(t, 1, resp.Data.Count)
	assert.False(t, resp.Data.Truncated)
	assert.Contains(t, capturedQuery, "range")
}

func TestInfluxDBQueryHandler_InfluxQL_성공(t *testing.T) {
	var capturedQuery string
	agentFake := &fakeInfluxDBAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		influxQLFn: func(_ context.Context, q string) ([]system.InfluxQueryResult, error) {
			capturedQuery = q
			return []system.InfluxQueryResult{}, nil
		},
	}
	router := setupInfluxDBQueryRouter(t, agentFake)

	body := `{"query_language":"influxql","query":"SELECT * FROM cpu"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/influxdb/metrics/query",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "SELECT * FROM cpu", capturedQuery)
}

func TestInfluxDBQueryHandler_빈결과(t *testing.T) {
	agentFake := &fakeInfluxDBAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		fluxFn: func(_ context.Context, _ string) ([]system.InfluxQueryResult, error) {
			return []system.InfluxQueryResult{}, nil
		},
	}
	router := setupInfluxDBQueryRouter(t, agentFake)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/influxdb/metrics/query",
		strings.NewReader(`{"query_language":"flux","query":"q"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeQueryResponse(t, rec)
	assert.NotNil(t, resp.Data.Entries)
	assert.Len(t, resp.Data.Entries, 0)
	assert.Equal(t, 0, resp.Data.Count)
}

func TestInfluxDBQueryHandler_에이전트없음_404(t *testing.T) {
	router := setupInfluxDBQueryRouter(t /* no agents */)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/influxdb/ghost/query",
		strings.NewReader(`{"query_language":"flux","query":"q"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestInfluxDBQueryHandler_잘못된에이전트타입_400(t *testing.T) {
	// Store 에이전트를 InfluxDB 엔드포인트에 지정하면 400.
	storeFake := &fakeStoreAgent{fakeAgentCommon: newFakeAgent("s1", "wrong", "store")}
	router := setupInfluxDBQueryRouter(t, storeFake)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/influxdb/wrong/query",
		strings.NewReader(`{"query_language":"flux","query":"q"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestInfluxDBQueryHandler_잘못된_query_language_400(t *testing.T) {
	agentFake := &fakeInfluxDBAgent{fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb")}
	router := setupInfluxDBQueryRouter(t, agentFake)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/influxdb/metrics/query",
		strings.NewReader(`{"query_language":"sql","query":"SELECT 1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestInfluxDBQueryHandler_빈쿼리_400(t *testing.T) {
	agentFake := &fakeInfluxDBAgent{fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb")}
	router := setupInfluxDBQueryRouter(t, agentFake)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/influxdb/metrics/query",
		strings.NewReader(`{"query_language":"flux","query":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestInfluxDBQueryHandler_Context_취소_408(t *testing.T) {
	// context deadline 이 이미 지난 상태에서 호출되면 408 Request Timeout 을 반환해야 한다.
	agentFake := &fakeInfluxDBAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		fluxFn: func(ctx context.Context, _ string) ([]system.InfluxQueryResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	router := setupInfluxDBQueryRouter(t, agentFake)

	cctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	req := httptest.NewRequestWithContext(cctx, http.MethodPost,
		"/api/v1/influxdb/metrics/query",
		strings.NewReader(`{"query_language":"flux","query":"q"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestTimeout, rec.Code, "body=%s", rec.Body.String())
}

func TestInfluxDBQueryHandler_클라이언트_에러_500(t *testing.T) {
	agentFake := &fakeInfluxDBAgent{
		fakeAgentCommon: newFakeAgent("i1", "metrics", "influxdb"),
		fluxFn: func(_ context.Context, _ string) ([]system.InfluxQueryResult, error) {
			return nil, errors.New("bucket not found")
		},
	}
	router := setupInfluxDBQueryRouter(t, agentFake)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/influxdb/metrics/query",
		strings.NewReader(`{"query_language":"flux","query":"q"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	// 일반 쿼리 실패는 500 (MapDomainError 가 매핑 못하므로 internal)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, false, resp["success"])
}
