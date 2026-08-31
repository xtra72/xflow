package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent/system"
)

// 빈 버킷 채우기 · first/last 집계 테스트.
//
// 종전에는 store 소스가 min/max/avg 만 서버에서 처리하고 first/last 는 프런트가
// 원시 엔트리를 받아 스스로 버킷을 나눴다. 그 경로에는 채우기가 걸리지 않아 같은
// fill 설정이 집계 함수에 따라 되기도 하고 안 되기도 했다. 다섯 종을 모두 서버에서
// 처리하고 그 결과에 채우기를 적용한다.

// 버킷 0 에만 값이 있고 60_000 · 120_000 버킷은 비어 있다.
func sparseEntries() []system.HistoryEntry {
	return []system.HistoryEntry{
		{Timestamp: time.UnixMilli(1_000), Value: float64(10)},
		{Timestamp: time.UnixMilli(30_000), Value: float64(20)},
	}
}

func TestStoreQuery_집계_first(t *testing.T) {
	// 버킷 0 에 10(1s) · 20(30s) → first=10. 도착 순서가 아니라 시각 순서다.
	entries := []system.HistoryEntry{
		{Timestamp: time.UnixMilli(30_000), Value: float64(20)},
		{Timestamp: time.UnixMilli(1_000), Value: float64(10)},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	rec := doAggPOST(t, router, `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":60000,`+
		`"interval_ms":60000,"aggregation":"first"}`)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 1)
	assert.Equal(t, float64(10), resp.Data.Entries[0].Value)
}

func TestStoreQuery_집계_last(t *testing.T) {
	entries := []system.HistoryEntry{
		{Timestamp: time.UnixMilli(30_000), Value: float64(20)},
		{Timestamp: time.UnixMilli(1_000), Value: float64(10)},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	rec := doAggPOST(t, router, `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":60000,`+
		`"interval_ms":60000,"aggregation":"last"}`)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 1)
	assert.Equal(t, float64(20), resp.Data.Entries[0].Value)
}

func TestStoreQuery_채우기_미지정이면_빈버킷_생략(t *testing.T) {
	// 종전 동작 — fill 을 보내지 않은 기존 요청은 달라지지 않는다.
	router := setupStoreQueryRouter(t, makeAggFake(t, sparseEntries()))

	rec := doAggPOST(t, router, `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":180000,`+
		`"interval_ms":60000,"aggregation":"avg"}`)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 1)
	assert.Equal(t, int64(0), resp.Data.Entries[0].Timestamp)
}

func TestStoreQuery_채우기_zero(t *testing.T) {
	router := setupStoreQueryRouter(t, makeAggFake(t, sparseEntries()))

	rec := doAggPOST(t, router, `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":180000,`+
		`"interval_ms":60000,"aggregation":"avg","fill":"zero"}`)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 3)
	assert.Equal(t, float64(15), resp.Data.Entries[0].Value)
	assert.Equal(t, float64(0), resp.Data.Entries[1].Value)
	assert.Equal(t, int64(60_000), resp.Data.Entries[1].Timestamp)
	assert.Equal(t, float64(0), resp.Data.Entries[2].Value)
	assert.Equal(t, int64(120_000), resp.Data.Entries[2].Timestamp)
}

func TestStoreQuery_채우기_null(t *testing.T) {
	router := setupStoreQueryRouter(t, makeAggFake(t, sparseEntries()))

	rec := doAggPOST(t, router, `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":180000,`+
		`"interval_ms":60000,"aggregation":"avg","fill":"null"}`)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 3)
	assert.Equal(t, float64(15), resp.Data.Entries[0].Value)
	assert.Nil(t, resp.Data.Entries[1].Value)
	assert.Nil(t, resp.Data.Entries[2].Value)
}

func TestStoreQuery_채우기_previous(t *testing.T) {
	router := setupStoreQueryRouter(t, makeAggFake(t, sparseEntries()))

	rec := doAggPOST(t, router, `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":180000,`+
		`"interval_ms":60000,"aggregation":"avg","fill":"previous"}`)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 3)
	assert.Equal(t, float64(15), resp.Data.Entries[0].Value)
	assert.Equal(t, float64(15), resp.Data.Entries[1].Value)
	assert.Equal(t, float64(15), resp.Data.Entries[2].Value)
}

func TestStoreQuery_채우기_previous_첫값이전은_비워둔다(t *testing.T) {
	// 이어 쓸 직전값이 없는 구간을 0 으로 채우면 그것은 zero 전략이지 previous 가 아니다.
	entries := []system.HistoryEntry{{Timestamp: time.UnixMilli(130_000), Value: float64(7)}}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	rec := doAggPOST(t, router, `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":180000,`+
		`"interval_ms":60000,"aggregation":"avg","fill":"previous"}`)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	// 0 · 60_000 버킷은 채우지 않는다 → 120_000 하나만 남는다.
	require.Len(t, resp.Data.Entries, 1)
	assert.Equal(t, int64(120_000), resp.Data.Entries[0].Timestamp)
	assert.Equal(t, float64(7), resp.Data.Entries[0].Value)
}

func TestStoreQuery_채우기_previous_사용기간_초과시_비운다(t *testing.T) {
	// max=60s, interval=60s → 1칸만 이어 쓴다. 그 뒤 버킷은 null.
	router := setupStoreQueryRouter(t, makeAggFake(t, sparseEntries()))

	rec := doAggPOST(t, router, `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":240000,`+
		`"interval_ms":60000,"aggregation":"avg","fill":"previous","fill_previous_max_ms":60000}`)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 4)
	assert.Equal(t, float64(15), resp.Data.Entries[0].Value)
	assert.Equal(t, float64(15), resp.Data.Entries[1].Value, "1칸까지는 이어 쓴다")
	assert.Nil(t, resp.Data.Entries[2].Value, "기간을 넘기면 비운다")
	assert.Nil(t, resp.Data.Entries[3].Value)
}

func TestStoreQuery_채우기_previous_초과분_지정값(t *testing.T) {
	router := setupStoreQueryRouter(t, makeAggFake(t, sparseEntries()))

	rec := doAggPOST(t, router, `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":240000,`+
		`"interval_ms":60000,"aggregation":"avg","fill":"previous","fill_previous_max_ms":60000,`+
		`"fill_previous_overflow":"value","fill_previous_overflow_value":-1}`)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 4)
	assert.Equal(t, float64(15), resp.Data.Entries[1].Value)
	assert.Equal(t, float64(-1), resp.Data.Entries[2].Value)
	assert.Equal(t, float64(-1), resp.Data.Entries[3].Value)
}

func TestStoreQuery_채우기_값이_다시_나오면_기간이_초기화된다(t *testing.T) {
	// 0 버킷 값 → 60_000 채움 → 120_000 실제 값 → 180_000 다시 채움.
	entries := []system.HistoryEntry{
		{Timestamp: time.UnixMilli(1_000), Value: float64(10)},
		{Timestamp: time.UnixMilli(130_000), Value: float64(20)},
	}
	router := setupStoreQueryRouter(t, makeAggFake(t, entries))

	rec := doAggPOST(t, router, `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":240000,`+
		`"interval_ms":60000,"aggregation":"avg","fill":"previous","fill_previous_max_ms":60000}`)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	require.Len(t, resp.Data.Entries, 4)
	assert.Equal(t, float64(10), resp.Data.Entries[1].Value, "10 을 1칸 이어 쓴다")
	assert.Equal(t, float64(20), resp.Data.Entries[2].Value, "실제 값")
	assert.Equal(t, float64(20), resp.Data.Entries[3].Value, "기간이 초기화되어 다시 1칸")
}

func TestStoreQuery_채우기_잘못된_전략_400(t *testing.T) {
	router := setupStoreQueryRouter(t, makeAggFake(t, sparseEntries()))

	rec := doAggPOST(t, router, `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":180000,`+
		`"interval_ms":60000,"aggregation":"avg","fill":"avg"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
}

func TestStoreQuery_채우기_음수_사용기간_400(t *testing.T) {
	router := setupStoreQueryRouter(t, makeAggFake(t, sparseEntries()))

	rec := doAggPOST(t, router, `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":180000,`+
		`"interval_ms":60000,"aggregation":"avg","fill":"previous","fill_previous_max_ms":-1}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
}

func TestStoreQuery_채우기_잘못된_초과처리_400(t *testing.T) {
	router := setupStoreQueryRouter(t, makeAggFake(t, sparseEntries()))

	rec := doAggPOST(t, router, `{"key":"k","mode":"time_range","start_ms":1000,"end_ms":180000,`+
		`"interval_ms":60000,"aggregation":"avg","fill":"previous","fill_previous_overflow":"nope"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
}

func TestStoreQuery_채우기_집계가_꺼져있으면_적용하지_않는다(t *testing.T) {
	// 채울 버킷 자체가 없다 — 원시 엔트리 경로는 종전 그대로다.
	router := setupStoreQueryRouter(t, makeAggFake(t, sparseEntries()))

	rec := doAggPOST(t, router,
		`{"key":"k","mode":"time_range","start_ms":1000,"end_ms":180000,"fill":"zero"}`)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeQueryResponse(t, rec)
	assert.Len(t, resp.Data.Entries, 2)
}
