// @spec SPEC-TSDB-002 §2.10 (U10) — 스키마 디스커버리 라우트 D2~D4.
package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api"
)

// fakeInfluxDiscovererAgent 는 influxSchemaDiscoverer 를 구현하는 테스트용 페이크다.
//
// influxManager 는 구현하지 않는다 — 두 계약이 실제로 분리되어 있는지를 이 타입의
// 존재가 확인한다. 프로덕션의 *system.InfluxDBAgent 는 둘 다 만족한다.
type fakeInfluxDiscovererAgent struct {
	*fakeAgentCommon
	listTagKeysFn   func(ctx context.Context, bucket, measurement string) ([]string, error)
	listTagValuesFn func(ctx context.Context, bucket, measurement, tagKey string) ([]string, error)
	listFieldKeysFn func(ctx context.Context, bucket, measurement string) ([]string, error)
}

func (f *fakeInfluxDiscovererAgent) ListTagKeys(ctx context.Context, bucket, measurement string) ([]string, error) {
	if f.listTagKeysFn != nil {
		return f.listTagKeysFn(ctx, bucket, measurement)
	}
	return nil, nil
}

func (f *fakeInfluxDiscovererAgent) ListTagValues(ctx context.Context, bucket, measurement, tagKey string, _ map[string]string, _ system.SchemaWindow) ([]string, error) {
	if f.listTagValuesFn != nil {
		return f.listTagValuesFn(ctx, bucket, measurement, tagKey)
	}
	return nil, nil
}

func (f *fakeInfluxDiscovererAgent) ListFieldKeys(ctx context.Context, bucket, measurement string) ([]string, error) {
	if f.listFieldKeysFn != nil {
		return f.listFieldKeysFn(ctx, bucket, measurement)
	}
	return nil, nil
}

func newInfluxDiscovererFake(name string) *fakeInfluxDiscovererAgent {
	return &fakeInfluxDiscovererAgent{fakeAgentCommon: newFakeAgent("i1", name, "influxdb")}
}

// discoveryGet 은 디스커버리 GET 요청 헬퍼다.
func discoveryGet(t *testing.T, agents []agent.Agent, target string) *httptest.ResponseRecorder {
	t.Helper()
	router := setupInfluxManagementRouter(t, agents...)
	return mgmtReq(t, router, http.MethodGet, target, "")
}

// ===== 라우트 등록 =====

// TestInfluxSchema_라우트_등록 은 D2~D4 3 종이 추가로 등록됨을 확인한다(AC-29).
// D1(measurements)은 기존 라우트를 겸하므로 신규 등록이 없다.
func TestInfluxSchema_라우트_등록(t *testing.T) {
	t.Parallel()
	router := api.NewRouter()
	h := NewInfluxDBManagementHandler(&fakeAgentLookup{}, nil)
	g := router.Group("/api/v1")
	before := router.RouteCount()
	h.RegisterRoutes(g)
	// @spec SPEC-TSDB-003 §2.2 (U2) — 열거 D5 가 더해져 10 이다.
	assert.Equal(t, 10, router.RouteCount()-before, "관리 6 종 + 디스커버리 3 종 + 열거 1 종")
}

// TestInfluxSchema_디스커버리_권한 은 3 종 모두 store.read 임을 확인한다.
// 읽기 전용 경로가 store.update 를 요구하면 조회 전용 사용자가 시리즈를 고를 수
// 없게 된다.
func TestInfluxSchema_디스커버리_권한(t *testing.T) {
	t.Parallel()
	router := api.NewRouter()
	h := NewInfluxDBManagementHandler(&fakeAgentLookup{}, nil)
	h.RegisterRoutes(router.Group("/api/v1"))

	want := map[string]string{
		"/api/v1/influxdb/{agent_name}/tag-keys":   "store.read",
		"/api/v1/influxdb/{agent_name}/tag-values": "store.read",
		"/api/v1/influxdb/{agent_name}/field-keys": "store.read",
	}
	seen := 0
	for _, r := range router.Routes() {
		perm, ok := want[r.Pattern]
		if !ok {
			continue
		}
		seen++
		assert.Equal(t, http.MethodGet, r.Method, r.Pattern)
		assert.Equal(t, perm, r.Permission, r.Pattern)
	}
	assert.Equal(t, len(want), seen, "디스커버리 라우트가 전부 등록되지 않았다")
}

// ===== D2 tag-keys =====

func TestInfluxSchema_TagKeys_성공(t *testing.T) {
	var gotBucket, gotMeasurement string
	f := newInfluxDiscovererFake("metrics")
	f.listTagKeysFn = func(_ context.Context, bucket, measurement string) ([]string, error) {
		gotBucket, gotMeasurement = bucket, measurement
		return []string{"host", "region"}, nil
	}

	rec := discoveryGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/tag-keys?measurement=cpu&bucket=b1")
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	data := decodeManagementData(t, rec)
	assert.Equal(t, []any{"host", "region"}, data["tag_keys"])
	assert.Equal(t, float64(2), data["count"])
	assert.Equal(t, "b1", gotBucket)
	assert.Equal(t, "cpu", gotMeasurement)
}

func TestInfluxSchema_TagKeys_bucket생략(t *testing.T) {
	var gotBucket string
	f := newInfluxDiscovererFake("metrics")
	f.listTagKeysFn = func(_ context.Context, bucket, _ string) ([]string, error) {
		gotBucket = bucket
		return nil, nil
	}

	rec := discoveryGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/tag-keys?measurement=cpu")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, gotBucket, "빈 bucket 을 그대로 넘겨 에이전트가 기본값을 채우게 한다")

	// nil 결과는 null 이 아니라 빈 배열로 직렬화한다 — 클라이언트가 length 를
	// 바로 읽을 수 있어야 한다.
	data := decodeManagementData(t, rec)
	assert.Equal(t, []any{}, data["tag_keys"])
	assert.Equal(t, float64(0), data["count"])
}

func TestInfluxSchema_TagKeys_measurement누락_400(t *testing.T) {
	f := newInfluxDiscovererFake("metrics")
	f.listTagKeysFn = func(_ context.Context, _, _ string) ([]string, error) {
		t.Fatal("measurement 없이 조회를 실행해서는 안 된다")
		return nil, nil
	}

	rec := discoveryGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/tag-keys")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "measurement is required")
}

// ===== D3 tag-values =====

func TestInfluxSchema_TagValues_성공(t *testing.T) {
	var gotMeasurement, gotTagKey string
	f := newInfluxDiscovererFake("metrics")
	f.listTagValuesFn = func(_ context.Context, _, measurement, tagKey string) ([]string, error) {
		gotMeasurement, gotTagKey = measurement, tagKey
		return []string{"a", "b"}, nil
	}

	rec := discoveryGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/tag-values?measurement=cpu&tag_key=host")
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	data := decodeManagementData(t, rec)
	assert.Equal(t, []any{"a", "b"}, data["tag_values"])
	assert.Equal(t, float64(2), data["count"])
	assert.Equal(t, "cpu", gotMeasurement)
	assert.Equal(t, "host", gotTagKey)
}

// TestInfluxSchema_TagValues_tagKey누락_400 은 tag_key 폴백이 없음을 잠근다.
// 키 없이 값 목록을 돌려주면 어느 키의 값인지 알 수 없다.
func TestInfluxSchema_TagValues_tagKey누락_400(t *testing.T) {
	f := newInfluxDiscovererFake("metrics")
	f.listTagValuesFn = func(_ context.Context, _, _, _ string) ([]string, error) {
		t.Fatal("tag_key 없이 조회를 실행해서는 안 된다")
		return nil, nil
	}

	rec := discoveryGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/tag-values?measurement=cpu")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "tag_key is required")
}

func TestInfluxSchema_TagValues_measurement누락_400(t *testing.T) {
	f := newInfluxDiscovererFake("metrics")
	rec := discoveryGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/tag-values?tag_key=host")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "measurement is required")
}

// ===== D4 field-keys =====

func TestInfluxSchema_FieldKeys_성공(t *testing.T) {
	var gotMeasurement string
	f := newInfluxDiscovererFake("metrics")
	f.listFieldKeysFn = func(_ context.Context, _, measurement string) ([]string, error) {
		gotMeasurement = measurement
		return []string{"usage", "load"}, nil
	}

	rec := discoveryGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/field-keys?measurement=cpu")
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	data := decodeManagementData(t, rec)
	assert.Equal(t, []any{"usage", "load"}, data["field_keys"])
	assert.Equal(t, float64(2), data["count"])
	assert.Equal(t, "cpu", gotMeasurement)
}

func TestInfluxSchema_FieldKeys_measurement누락_400(t *testing.T) {
	f := newInfluxDiscovererFake("metrics")
	rec := discoveryGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/field-keys")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "measurement is required")
}

// ===== 에이전트 해석 =====

func TestInfluxSchema_에이전트없음_404(t *testing.T) {
	rec := discoveryGet(t, nil, "/api/v1/influxdb/ghost/field-keys?measurement=cpu")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "agent_not_found")
}

func TestInfluxSchema_잘못된타입_400(t *testing.T) {
	storeFake := &fakeStoreAgent{fakeAgentCommon: newFakeAgent("s1", "wrong", "store")}
	rec := discoveryGet(t, []agent.Agent{storeFake}, "/api/v1/influxdb/wrong/tag-keys?measurement=cpu")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "not_an_influxdb_agent")
}

// ===== 오류 매핑 =====

// TestInfluxSchema_이스케이프불가_400 은 이스케이프 불가 식별자가 502 가 아니라
// 400 으로 매핑됨을 확인한다. 사용자가 고칠 수 있는 입력 오류를 업스트림 장애로
// 보고하면 클라이언트가 재시도를 한다.
func TestInfluxSchema_이스케이프불가_400(t *testing.T) {
	f := newInfluxDiscovererFake("metrics")
	f.listTagKeysFn = func(_ context.Context, _, _ string) ([]string, error) {
		return nil, system.ErrUnescapableIdentifier
	}

	rec := discoveryGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/tag-keys?measurement=cpu")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestInfluxSchema_업스트림오류_502 는 그 외 오류가 관리 조작과 같은 매핑을
// 따름을 확인한다.
func TestInfluxSchema_업스트림오류_502(t *testing.T) {
	f := newInfluxDiscovererFake("metrics")
	f.listFieldKeysFn = func(_ context.Context, _, _ string) ([]string, error) {
		return nil, errors.New("upstream down")
	}

	rec := discoveryGet(t, []agent.Agent{f}, "/api/v1/influxdb/metrics/field-keys?measurement=cpu")
	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

// TestInfluxSchema_타임아웃_408 은 context 취소가 408 로 매핑됨을 확인한다.
func TestInfluxSchema_타임아웃_408(t *testing.T) {
	f := newInfluxDiscovererFake("metrics")
	f.listTagValuesFn = func(_ context.Context, _, _, _ string) ([]string, error) {
		return nil, context.DeadlineExceeded
	}

	rec := discoveryGet(t, []agent.Agent{f},
		"/api/v1/influxdb/metrics/tag-values?measurement=cpu&tag_key=host")
	assert.Equal(t, http.StatusRequestTimeout, rec.Code)
}

// TestInfluxSchema_핸들러_NoCache 는 핸들러가 응답을 캐시하지 않음을 확인한다
// (§2.10 · UB1-12 · AC-33). 같은 요청을 두 번 보내면 에이전트도 두 번 불린다.
func TestInfluxSchema_핸들러_NoCache(t *testing.T) {
	calls := 0
	f := newInfluxDiscovererFake("metrics")
	f.listFieldKeysFn = func(_ context.Context, _, _ string) ([]string, error) {
		calls++
		return []string{"usage"}, nil
	}
	router := setupInfluxManagementRouter(t, f)

	for range 2 {
		rec := mgmtReq(t, router, http.MethodGet, "/api/v1/influxdb/metrics/field-keys?measurement=cpu", "")
		require.Equal(t, http.StatusOK, rec.Code)
	}
	assert.Equal(t, 2, calls, "디스커버리 응답이 캐시되었다")
}
