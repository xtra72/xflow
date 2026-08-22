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
	"github.com/xtra/xflow/internal/api"
)

// fakeInfluxManagerAgent 는 influxManager 를 구현하는 테스트용 페이크이다.
type fakeInfluxManagerAgent struct {
	*fakeAgentCommon
	listBucketsFn       func(ctx context.Context) ([]system.BucketInfo, error)
	createBucketFn      func(ctx context.Context, name string, retentionSeconds int64) (system.BucketInfo, error)
	deleteBucketFn      func(ctx context.Context, bucketRef string) error
	truncateBucketFn    func(ctx context.Context, bucketRef string) error
	listMeasurementsFn  func(ctx context.Context, bucket string) ([]string, error)
	deleteMeasurementFn func(ctx context.Context, bucket, measurement string) error
}

func (f *fakeInfluxManagerAgent) ListBuckets(ctx context.Context) ([]system.BucketInfo, error) {
	if f.listBucketsFn != nil {
		return f.listBucketsFn(ctx)
	}
	return nil, nil
}

func (f *fakeInfluxManagerAgent) CreateBucket(ctx context.Context, name string, retentionSeconds int64) (system.BucketInfo, error) {
	if f.createBucketFn != nil {
		return f.createBucketFn(ctx, name, retentionSeconds)
	}
	return system.BucketInfo{}, nil
}

func (f *fakeInfluxManagerAgent) DeleteBucket(ctx context.Context, bucketRef string) error {
	if f.deleteBucketFn != nil {
		return f.deleteBucketFn(ctx, bucketRef)
	}
	return nil
}

func (f *fakeInfluxManagerAgent) TruncateBucket(ctx context.Context, bucketRef string) error {
	if f.truncateBucketFn != nil {
		return f.truncateBucketFn(ctx, bucketRef)
	}
	return nil
}

func (f *fakeInfluxManagerAgent) ListMeasurements(ctx context.Context, bucket string) ([]string, error) {
	if f.listMeasurementsFn != nil {
		return f.listMeasurementsFn(ctx, bucket)
	}
	return nil, nil
}

func (f *fakeInfluxManagerAgent) DeleteMeasurement(ctx context.Context, bucket, measurement string) error {
	if f.deleteMeasurementFn != nil {
		return f.deleteMeasurementFn(ctx, bucket, measurement)
	}
	return nil
}

func newInfluxManagerFake(name string) *fakeInfluxManagerAgent {
	return &fakeInfluxManagerAgent{fakeAgentCommon: newFakeAgent("i1", name, "influxdb")}
}

// setupInfluxManagementRouter 는 InfluxDBManagementHandler 를 탑재한 테스트 라우터를 생성한다.
func setupInfluxManagementRouter(t *testing.T, agents ...agent.Agent) *api.Router {
	t.Helper()
	lookup := &fakeAgentLookup{agents: agents}
	router := api.NewRouter()
	h := NewInfluxDBManagementHandler(lookup, nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

// mgmtReq 는 body 문자열을 io.Reader 로 변환해 패키지 공용 doRequest 를 호출하는 헬퍼이다.
// body 가 빈 문자열이면 nil body 로 요청한다.
func mgmtReq(t *testing.T, router *api.Router, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	if body == "" {
		return doRequest(t, router, method, target, nil)
	}
	return doRequest(t, router, method, target, strings.NewReader(body))
}

// decodeManagementData 는 표준 성공 응답의 data 부분을 map 으로 디코딩한다.
func decodeManagementData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	return resp.Data
}

func TestInfluxManagement_RegisterRoutes(t *testing.T) {
	t.Parallel()
	router := api.NewRouter()
	h := NewInfluxDBManagementHandler(&fakeAgentLookup{}, nil)
	g := router.Group("/api/v1")
	before := router.RouteCount()
	h.RegisterRoutes(g)
	// @spec SPEC-TSDB-002 §2.10 (U10) — 관리 6 종 + 디스커버리 3 종(D2~D4).
	// D1(measurements)은 기존 관리 라우트를 겸하므로 신규 등록이 없다.
	assert.Equal(t, 9, router.RouteCount()-before)
}

// ===== ListBuckets =====

func TestInfluxManagement_ListBuckets_성공(t *testing.T) {
	f := newInfluxManagerFake("metrics")
	f.listBucketsFn = func(_ context.Context) ([]system.BucketInfo, error) {
		return []system.BucketInfo{
			{ID: "b1", Name: "one", OrgID: "o1", RetentionSeconds: 3600},
			{ID: "b2", Name: "two"},
		}, nil
	}
	router := setupInfluxManagementRouter(t, f)

	rec := mgmtReq(t, router, http.MethodGet, "/api/v1/influxdb/metrics/buckets", "")
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	data := decodeManagementData(t, rec)
	assert.Equal(t, float64(2), data["count"])
	buckets, ok := data["buckets"].([]any)
	require.True(t, ok)
	require.Len(t, buckets, 2)
	first := buckets[0].(map[string]any)
	assert.Equal(t, "b1", first["id"])
	assert.Equal(t, "one", first["name"])
	assert.Equal(t, float64(3600), first["retention_seconds"])
}

func TestInfluxManagement_ListBuckets_에이전트없음_404(t *testing.T) {
	router := setupInfluxManagementRouter(t /* no agents */)
	rec := mgmtReq(t, router, http.MethodGet, "/api/v1/influxdb/ghost/buckets", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestInfluxManagement_ListBuckets_잘못된타입_400(t *testing.T) {
	storeFake := &fakeStoreAgent{fakeAgentCommon: newFakeAgent("s1", "wrong", "store")}
	router := setupInfluxManagementRouter(t, storeFake)
	rec := mgmtReq(t, router, http.MethodGet, "/api/v1/influxdb/wrong/buckets", "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestInfluxManagement_ListBuckets_미지원_501(t *testing.T) {
	f := newInfluxManagerFake("metrics")
	f.listBucketsFn = func(_ context.Context) ([]system.BucketInfo, error) {
		return nil, system.ErrManagementNotSupported
	}
	router := setupInfluxManagementRouter(t, f)
	rec := mgmtReq(t, router, http.MethodGet, "/api/v1/influxdb/metrics/buckets", "")
	assert.Equal(t, http.StatusNotImplemented, rec.Code, "body=%s", rec.Body.String())
}

func TestInfluxManagement_ListBuckets_influx에러_502(t *testing.T) {
	f := newInfluxManagerFake("metrics")
	f.listBucketsFn = func(_ context.Context) ([]system.BucketInfo, error) {
		return nil, assertAnError()
	}
	router := setupInfluxManagementRouter(t, f)
	rec := mgmtReq(t, router, http.MethodGet, "/api/v1/influxdb/metrics/buckets", "")
	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

// ===== CreateBucket =====

func TestInfluxManagement_CreateBucket_성공(t *testing.T) {
	var gotName string
	var gotRetention int64
	f := newInfluxManagerFake("metrics")
	f.createBucketFn = func(_ context.Context, name string, retentionSeconds int64) (system.BucketInfo, error) {
		gotName = name
		gotRetention = retentionSeconds
		return system.BucketInfo{ID: "new", Name: name, RetentionSeconds: retentionSeconds}, nil
	}
	router := setupInfluxManagementRouter(t, f)

	rec := mgmtReq(t, router, http.MethodPost, "/api/v1/influxdb/metrics/buckets",
		`{"name":"newbucket","retentionSeconds":7200}`)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assert.Equal(t, "newbucket", gotName)
	assert.Equal(t, int64(7200), gotRetention)

	data := decodeManagementData(t, rec)
	assert.Equal(t, "new", data["id"])
	assert.Equal(t, "newbucket", data["name"])
}

func TestInfluxManagement_CreateBucket_이름누락_400(t *testing.T) {
	f := newInfluxManagerFake("metrics")
	router := setupInfluxManagementRouter(t, f)
	rec := mgmtReq(t, router, http.MethodPost, "/api/v1/influxdb/metrics/buckets", `{"retentionSeconds":10}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestInfluxManagement_CreateBucket_음수retention_400(t *testing.T) {
	f := newInfluxManagerFake("metrics")
	router := setupInfluxManagementRouter(t, f)
	rec := mgmtReq(t, router, http.MethodPost, "/api/v1/influxdb/metrics/buckets", `{"name":"b","retentionSeconds":-1}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestInfluxManagement_CreateBucket_미지원_501(t *testing.T) {
	f := newInfluxManagerFake("metrics")
	f.createBucketFn = func(_ context.Context, _ string, _ int64) (system.BucketInfo, error) {
		return system.BucketInfo{}, system.ErrManagementNotSupported
	}
	router := setupInfluxManagementRouter(t, f)
	rec := mgmtReq(t, router, http.MethodPost, "/api/v1/influxdb/metrics/buckets", `{"name":"b"}`)
	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

// ===== DeleteBucket =====

func TestInfluxManagement_DeleteBucket_성공(t *testing.T) {
	var gotRef string
	f := newInfluxManagerFake("metrics")
	f.deleteBucketFn = func(_ context.Context, bucketRef string) error {
		gotRef = bucketRef
		return nil
	}
	router := setupInfluxManagementRouter(t, f)

	rec := mgmtReq(t, router, http.MethodDelete, "/api/v1/influxdb/metrics/buckets/target", "")
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assert.Equal(t, "target", gotRef)

	data := decodeManagementData(t, rec)
	assert.Equal(t, "target", data["bucket"])
	assert.Equal(t, true, data["deleted"])
}

func TestInfluxManagement_DeleteBucket_미지원_501(t *testing.T) {
	f := newInfluxManagerFake("metrics")
	f.deleteBucketFn = func(_ context.Context, _ string) error {
		return system.ErrManagementNotSupported
	}
	router := setupInfluxManagementRouter(t, f)
	rec := mgmtReq(t, router, http.MethodDelete, "/api/v1/influxdb/metrics/buckets/x", "")
	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

// ===== TruncateBucket =====

func TestInfluxManagement_TruncateBucket_성공(t *testing.T) {
	var gotRef string
	f := newInfluxManagerFake("metrics")
	f.truncateBucketFn = func(_ context.Context, bucketRef string) error {
		gotRef = bucketRef
		return nil
	}
	router := setupInfluxManagementRouter(t, f)

	rec := mgmtReq(t, router, http.MethodPost, "/api/v1/influxdb/metrics/buckets/target/truncate", "")
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assert.Equal(t, "target", gotRef)

	data := decodeManagementData(t, rec)
	assert.Equal(t, true, data["truncated"])
}

func TestInfluxManagement_TruncateBucket_influx에러_502(t *testing.T) {
	f := newInfluxManagerFake("metrics")
	f.truncateBucketFn = func(_ context.Context, _ string) error {
		return assertAnError()
	}
	router := setupInfluxManagementRouter(t, f)
	rec := mgmtReq(t, router, http.MethodPost, "/api/v1/influxdb/metrics/buckets/x/truncate", "")
	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

// ===== ListMeasurements =====

func TestInfluxManagement_ListMeasurements_성공(t *testing.T) {
	var gotBucket string
	f := newInfluxManagerFake("metrics")
	f.listMeasurementsFn = func(_ context.Context, bucket string) ([]string, error) {
		gotBucket = bucket
		return []string{"cpu", "mem"}, nil
	}
	router := setupInfluxManagementRouter(t, f)

	rec := mgmtReq(t, router, http.MethodGet, "/api/v1/influxdb/metrics/measurements?bucket=explicit", "")
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assert.Equal(t, "explicit", gotBucket)

	data := decodeManagementData(t, rec)
	assert.Equal(t, float64(2), data["count"])
}

func TestInfluxManagement_ListMeasurements_bucket생략_기본버킷(t *testing.T) {
	// bucket 쿼리 파라미터 생략 시 빈 문자열이 그대로 전달되고, 에이전트가 기본 버킷으로 폴백한다.
	var gotBucket string
	f := newInfluxManagerFake("metrics")
	f.listMeasurementsFn = func(_ context.Context, bucket string) ([]string, error) {
		gotBucket = bucket
		return nil, nil
	}
	router := setupInfluxManagementRouter(t, f)

	rec := mgmtReq(t, router, http.MethodGet, "/api/v1/influxdb/metrics/measurements", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "", gotBucket)

	data := decodeManagementData(t, rec)
	// measurements 는 nil 이 아닌 빈 배열로 정규화된다.
	arr, ok := data["measurements"].([]any)
	require.True(t, ok)
	assert.Len(t, arr, 0)
}

func TestInfluxManagement_ListMeasurements_미지원_501(t *testing.T) {
	f := newInfluxManagerFake("metrics")
	f.listMeasurementsFn = func(_ context.Context, _ string) ([]string, error) {
		return nil, system.ErrManagementNotSupported
	}
	router := setupInfluxManagementRouter(t, f)
	rec := mgmtReq(t, router, http.MethodGet, "/api/v1/influxdb/metrics/measurements", "")
	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

// ===== DeleteMeasurement =====

func TestInfluxManagement_DeleteMeasurement_성공(t *testing.T) {
	var gotBucket, gotMeasurement string
	f := newInfluxManagerFake("metrics")
	f.deleteMeasurementFn = func(_ context.Context, bucket, measurement string) error {
		gotBucket = bucket
		gotMeasurement = measurement
		return nil
	}
	router := setupInfluxManagementRouter(t, f)

	rec := mgmtReq(t, router, http.MethodDelete, "/api/v1/influxdb/metrics/measurements/cpu?bucket=b", "")
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assert.Equal(t, "b", gotBucket)
	assert.Equal(t, "cpu", gotMeasurement)

	data := decodeManagementData(t, rec)
	assert.Equal(t, "cpu", data["measurement"])
	assert.Equal(t, true, data["deleted"])
}

func TestInfluxManagement_DeleteMeasurement_미지원_501(t *testing.T) {
	f := newInfluxManagerFake("metrics")
	f.deleteMeasurementFn = func(_ context.Context, _, _ string) error {
		return system.ErrManagementNotSupported
	}
	router := setupInfluxManagementRouter(t, f)
	rec := mgmtReq(t, router, http.MethodDelete, "/api/v1/influxdb/metrics/measurements/cpu", "")
	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

func TestInfluxManagement_DeleteMeasurement_에이전트없음_404(t *testing.T) {
	router := setupInfluxManagementRouter(t /* no agents */)
	rec := mgmtReq(t, router, http.MethodDelete, "/api/v1/influxdb/ghost/measurements/cpu", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
