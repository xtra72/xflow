// @spec SPEC-STORE-004
//
// store_series_query_test.go — M3 HTTP 계층의 시리즈 fan-out / 시리즈 행 검증.
//
// 실제 system.UserStoreAgent 를 라우터에 연결하고, SetWithMeta 로 여러 시리즈를 쓴 뒤
// POST /query (metric/tags 필터 + labels) 와 GET /keys (시리즈 행) 의 HTTP 동작을 검증한다.
// AC-5/6/7/8/13/14.

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/system"
)

// newSeriesStoreAgent 는 history 가 활성화된 실제 store 에이전트를 만들고,
// metric/tags 시리즈 쓰기용 어댑터를 함께 돌려준다.
func newSeriesStoreAgent(t *testing.T, name string) (agent.Agent, *system.NodeStoreAdapter) {
	t.Helper()
	cfg := agent.AgentConfig{
		ID:   name,
		Name: name,
		Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"registration_type": "auto",
				"max_history_size":  10,
			},
		},
	}
	ag, err := system.NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })

	usa := ag.(*system.UserStoreAgent)
	adapter := usa.NodeStoreForNamespace("default").(*system.NodeStoreAdapter)
	return ag, adapter
}

// queryBody 는 POST /query 응답을 디코딩하는 헬퍼이다.
func queryBody(t *testing.T, rec *httptest.ResponseRecorder) chartQueryResponse {
	t.Helper()
	var env struct {
		Data chartQueryResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	return env.Data
}

// seedThreeSeries 는 AC-5 의 3개 시리즈를 key=room 아래에 쓴다.
func seedThreeSeries(t *testing.T, adapter *system.NodeStoreAdapter) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 22, system.StoreWriteMeta{MetricType: "temperature"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 55, system.StoreWriteMeta{MetricType: "humidity"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 23, system.StoreWriteMeta{
		MetricType: "temperature", Tags: map[string]string{"area": "a"},
	}))
}

// AC-5: 필터 없이 키 조회 → 모든 시리즈 반환, 각 엔트리에 labels.
func TestHTTP_QuerySeries_AllSeries_WithLabels(t *testing.T) {
	ag, adapter := newSeriesStoreAgent(t, "store-s")
	seedThreeSeries(t, adapter)
	router := setupStoreQueryRouter(t, ag)

	body := `{"key":"room","mode":"last_n","count":10}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/store/store-s/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := queryBody(t, rec)
	require.Equal(t, 3, resp.Count, "3개 시리즈의 현재값이 모두 반환")

	// 각 엔트리는 __metric__ 라벨을 포함한다.
	metrics := map[string]bool{}
	for _, e := range resp.Entries {
		require.NotNil(t, e.Labels, "엔트리는 labels 를 포함해야 한다")
		metrics[e.Labels["__metric__"]] = true
	}
	assert.True(t, metrics["temperature"])
	assert.True(t, metrics["humidity"])
}

// AC-6: metric+tags 필터 → 단일 시리즈.
func TestHTTP_QuerySeries_MetricAndTagsFilter(t *testing.T) {
	ag, adapter := newSeriesStoreAgent(t, "store-s")
	seedThreeSeries(t, adapter)
	router := setupStoreQueryRouter(t, ag)

	body := `{"key":"room","mode":"last_n","count":10,"metric_type":"temperature","tags":{"area":"a"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/store/store-s/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := queryBody(t, rec)
	require.Equal(t, 1, resp.Count, "단일 시리즈만 반환")
	assert.Equal(t, "temperature", resp.Entries[0].Labels["__metric__"])
	assert.Equal(t, "a", resp.Entries[0].Labels["area"])
	assert.Equal(t, "23", resp.Entries[0].Value)
}

// AC-7: metric_type 만 필터(tags 생략) → 해당 metric 의 모든 tags 시리즈.
func TestHTTP_QuerySeries_MetricOnlyFilter(t *testing.T) {
	ag, adapter := newSeriesStoreAgent(t, "store-s")
	seedThreeSeries(t, adapter)
	router := setupStoreQueryRouter(t, ag)

	body := `{"key":"room","mode":"last_n","count":10,"metric_type":"temperature"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/store/store-s/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := queryBody(t, rec)
	require.Equal(t, 2, resp.Count, "temperature 의 두 tags 시리즈 (humidity 제외)")
	for _, e := range resp.Entries {
		assert.Equal(t, "temperature", e.Labels["__metric__"])
	}
}

// AC-8: 미일치 필터 → 200 빈 결과.
func TestHTTP_QuerySeries_NoMatch_Empty200(t *testing.T) {
	ag, adapter := newSeriesStoreAgent(t, "store-s")
	seedThreeSeries(t, adapter)
	router := setupStoreQueryRouter(t, ag)

	body := `{"key":"room","mode":"last_n","count":10,"metric_type":"pressure"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/store/store-s/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "미일치는 200")
	resp := queryBody(t, rec)
	assert.Equal(t, 0, resp.Count, "엔트리 0개")
}

// AC-13 + AC-14: GET /keys 시리즈 행 + 필터 재사용.
func TestHTTP_ListKeys_SeriesRows(t *testing.T) {
	ag, adapter := newSeriesStoreAgent(t, "store-s")
	ctx := context.Background()
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 22, system.StoreWriteMeta{MetricType: "temperature"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 55, system.StoreWriteMeta{
		MetricType: "humidity", Tags: map[string]string{"area": "a"},
	}))
	router := setupStoreQueryRouter(t, ag)

	// 전체 행: 같은 key=room 이 metric/tags 다른 2개 행으로 노출.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/store-s/keys", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var env struct {
		Data StoreKeysListResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Equal(t, 2, env.Data.Count, "2개 시리즈 행")
	for _, row := range env.Data.Keys {
		assert.Equal(t, "room", row.Key, "같은 key 가 여러 행으로 노출")
	}

	// AC-14: metric_type 필터로 temperature 행만.
	reqF := httptest.NewRequest(http.MethodGet, "/api/v1/store/store-s/keys?metric_type=temperature", nil)
	recF := httptest.NewRecorder()
	router.Handler().ServeHTTP(recF, reqF)
	require.Equal(t, http.StatusOK, recF.Code)

	var envF struct {
		Data StoreKeysListResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recF.Body.Bytes(), &envF))
	require.Equal(t, 1, envF.Data.Count, "temperature 행만")
	assert.Equal(t, "temperature", envF.Data.Keys[0].MetricType)
}
