// @spec SPEC-STORE-004
//
// store_series_reset_test.go — M4 HTTP 계층의 단일 시리즈 reset(E8/AC-15) + 식별자
// 누락 정책 + 다중 시리즈 집계 정합(시리즈별 적용) 검증.
//
// 실제 system.UserStoreAgent 를 라우터에 연결하고, SetWithMeta 로 여러 시리즈를 쓴 뒤
// DELETE /keys/{key} (metric/tags 식별자) 와 POST /query (interval_ms+aggregation) 동작을 검증한다.

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

	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api"
)

// resetSeriesResponse 는 시리즈 reset 응답(카운트)을 디코딩하는 헬퍼이다.
type resetSeriesResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Key            string `json:"key"`
		HistoryCleared int    `json:"history_cleared"`
		EntriesDeleted int    `json:"entries_deleted"`
	} `json:"data"`
}

func decodeResetSeries(t *testing.T, rec *httptest.ResponseRecorder) resetSeriesResponse {
	t.Helper()
	var r resetSeriesResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	return r
}

// remainingSeriesCount 는 key 아래 남아있는 시리즈 수를 POST /query (필터 없음)로 센다.
func remainingSeriesCount(t *testing.T, router *api.Router, agentName, key string) int {
	t.Helper()
	body := `{"key":"` + key + `","mode":"last_n","count":10}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/store/"+agentName+"/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	return queryBody(t, rec).Count
}

// AC-15: metric/tags 식별자로 단일 시리즈만 reset, 다른 시리즈 무영향.
func TestHTTP_ResetSeries_SingleSeries_OthersUnaffected(t *testing.T) {
	ag, adapter := newSeriesStoreAgent(t, "store-s")
	seedThreeSeries(t, adapter) // (room,temperature,{}), (room,humidity,{}), (room,temperature,{area:a})
	router := setupStoreQueryRouter(t, ag)

	// humidity 단일 시리즈만 삭제.
	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/store/store-s/keys/room?metric_type=humidity", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeResetSeries(t, rec)
	assert.True(t, resp.Success)
	assert.Equal(t, "room", resp.Data.Key)
	// @spec SPEC-STORE-004: 동적(auto) 시리즈 → DeleteEntry (값+레지스트리 메타 삭제).
	assert.Equal(t, 1, resp.Data.HistoryCleared+resp.Data.EntriesDeleted,
		"humidity 단일 시리즈만 처리되어야 한다")
	assert.Equal(t, 1, resp.Data.EntriesDeleted, "동적 시리즈는 삭제")

	// humidity 동적 시리즈는 삭제되고 temperature 2개만 남는다(AC-15: 다른 시리즈 무영향).
	got := remainingSeriesCount(t, router, "store-s", "room")
	assert.Equal(t, 2, got, "humidity 삭제 후 temperature 2개 시리즈만 남음")
}

// AC-15 변형: metric+tags 로 더 좁혀 단일 시리즈 reset.
func TestHTTP_ResetSeries_MetricAndTags_NarrowsToOne(t *testing.T) {
	ag, adapter := newSeriesStoreAgent(t, "store-s")
	seedThreeSeries(t, adapter)
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/store/store-s/keys/room?metric_type=temperature&tag=area:a", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeResetSeries(t, rec)
	assert.Equal(t, 1, resp.Data.HistoryCleared+resp.Data.EntriesDeleted,
		"(temperature, area=a) 단일 시리즈만 처리")
}

// 식별자 누락 정책: metric/tags 없이 DELETE → 그 key 의 모든 시리즈가 대상.
func TestHTTP_ResetSeries_NoIdentifier_AllSeriesOfKey(t *testing.T) {
	ag, adapter := newSeriesStoreAgent(t, "store-s")
	seedThreeSeries(t, adapter) // room 아래 3개 시리즈
	// 다른 key 의 시리즈는 영향받지 않아야 한다.
	ctx := context.Background()
	require.NoError(t, adapter.SetWithMeta(ctx, "other", 9, system.StoreWriteMeta{MetricType: "temperature"}))
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/store/store-s/keys/room", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeResetSeries(t, rec)
	assert.Equal(t, 3, resp.Data.HistoryCleared+resp.Data.EntriesDeleted,
		"식별자 누락 시 room 의 3개 시리즈 전부가 대상")

	// other key 는 영향받지 않는다.
	assert.Equal(t, 1, remainingSeriesCount(t, router, "store-s", "other"),
		"다른 key 의 시리즈는 영향받지 않아야 한다")
}

// plain Set(메타 없음) 으로 쓴 bare key 기본 시리즈도 reset 대상이 된다.
//
// @spec SPEC-STORE-004: auto 모드에서 plain Set 키는 SourceAuto 로 자동 등록되는 동적
// 키이다. 따라서 reset 시 DeleteEntry 경로로 값과 레지스트리 메타가 함께 제거되어
// 키가 완전히 사라진다("전체 초기화 시 동적 키 삭제"). manual(yaml) 정적 키만
// ClearHistory 로 정의를 보존한다.
func TestHTTP_ResetSeries_PlainBareKey_Deleted(t *testing.T) {
	ag, adapter := newSeriesStoreAgent(t, "store-s")
	ctx := context.Background()
	// 메타 없는 plain Set → bare key 기본 시리즈. auto 모드라 SourceAuto 로 자동 등록된다.
	require.NoError(t, adapter.Set(ctx, "plain", 7))
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/store/store-s/keys/plain", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeResetSeries(t, rec)
	assert.Equal(t, 0, resp.Data.HistoryCleared)
	assert.Equal(t, 1, resp.Data.EntriesDeleted, "동적(auto) 키 → 완전 삭제")

	// 동적 키는 값+레지스트리 메타가 삭제되어 더 이상 조회되지 않는다.
	assert.Equal(t, 0, remainingSeriesCount(t, router, "store-s", "plain"))
}

// 미일치 식별자 → 200 + 카운트 0 (S4 정합, 에러 아님).
func TestHTTP_ResetSeries_NoMatch_Zero200(t *testing.T) {
	ag, adapter := newSeriesStoreAgent(t, "store-s")
	seedThreeSeries(t, adapter)
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/store/store-s/keys/room?metric_type=pressure", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeResetSeries(t, rec)
	assert.Equal(t, 0, resp.Data.HistoryCleared+resp.Data.EntriesDeleted)

	assert.Equal(t, 3, remainingSeriesCount(t, router, "store-s", "room"),
		"미일치 reset 은 어떤 시리즈도 건드리지 않는다")
}

// 잘못된 ?tag= 형식(콜론 없음) → 400.
func TestHTTP_ResetSeries_InvalidTagFilter_400(t *testing.T) {
	ag, adapter := newSeriesStoreAgent(t, "store-s")
	seedThreeSeries(t, adapter)
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodDelete,
		"/api/v1/store/store-s/keys/room?tag=badformat", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// 집계 정합: 다중 시리즈 + interval_ms+aggregation → 시리즈별 독립 집계 + labels 유지.
func TestHTTP_QuerySeries_Aggregation_PerSeries(t *testing.T) {
	ag, adapter := newSeriesStoreAgent(t, "store-s")
	ctx := context.Background()

	// 두 시리즈를 float 으로 고정(coercion 으로 string 화되지 않게)하여 동일 버킷에 다중 포인트.
	// 같은 (key, metric) 시리즈에 여러 값을 누적 → history 가 집계 입력이 된다.
	// temperature 시리즈: 두 값.
	require.NoError(t, adapter.SetWithMeta(ctx, "room", float64(10), system.StoreWriteMeta{
		MetricType: "temperature", DataType: "float",
	}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", float64(20), system.StoreWriteMeta{
		MetricType: "temperature", DataType: "float",
	}))
	// humidity 시리즈: 두 값.
	require.NoError(t, adapter.SetWithMeta(ctx, "room", float64(40), system.StoreWriteMeta{
		MetricType: "humidity", DataType: "float",
	}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", float64(60), system.StoreWriteMeta{
		MetricType: "humidity", DataType: "float",
	}))
	router := setupStoreQueryRouter(t, ag)

	// duration 모드 + 큰 interval(최근 1시간을 단일 버킷으로) + avg.
	// 엔트리는 모두 now 근처에 기록되므로 마지막(유일) 버킷에 모인다.
	// 시리즈별로 독립 집계되어야 한다: temperature avg=15, humidity avg=50.
	body := `{"key":"room","mode":"duration","duration_sec":3600,` +
		`"interval_ms":3600000,"aggregation":"avg"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/store/store-s/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := queryBody(t, rec)

	// metric 별로 집계값을 모은다(시리즈별 독립 집계 검증).
	byMetric := map[string][]float64{}
	for _, e := range resp.Entries {
		require.NotNil(t, e.Labels, "집계 엔트리도 labels 를 유지해야 한다")
		m := e.Labels["__metric__"]
		f, ok := e.Value.(float64)
		require.True(t, ok, "집계 결과는 float64")
		byMetric[m] = append(byMetric[m], f)
	}

	require.Len(t, byMetric["temperature"], 1, "temperature 는 단일 버킷")
	require.Len(t, byMetric["humidity"], 1, "humidity 는 단일 버킷")
	assert.InDelta(t, 15.0, byMetric["temperature"][0], 0.001, "temperature avg=(10+20)/2")
	assert.InDelta(t, 50.0, byMetric["humidity"][0], 0.001, "humidity avg=(40+60)/2 — 시리즈별 독립")
}
