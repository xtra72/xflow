// @spec SPEC-STORE-003 v0.4.0
//
// 본 파일은 PUT /store/{name}/keys/{key}/meta (임의 엔트리의 metric_type/tags 설정)
// 핸들러를 검증한다. 동적 키 포함 모든 엔트리에 타입/태그를 부여할 수 있다.
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent/system"
)

// fakeKeyMetaSetter 는 storeKeyMetaSetter 인터페이스를 구현하는 페이크이다.
// setFn 으로 호출 인자를 캡처하거나 에러를 주입할 수 있다.
type fakeKeyMetaSetter struct {
	*fakeAgentCommon
	setFn func(key, metricType string, tags map[string]string) error
}

func (f *fakeKeyMetaSetter) SetKeyMeta(key, metricType string, tags map[string]string) error {
	if f.setFn != nil {
		return f.setFn(key, metricType, tags)
	}
	return nil
}

// setMetaResponse 는 PUT /meta 성공 응답 디코더이다.
type setMetaResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Key        string            `json:"key"`
		MetricType string            `json:"metric_type"`
		Tags       map[string]string `json:"tags"`
	} `json:"data"`
}

func decodeSetMeta(t *testing.T, rec *httptest.ResponseRecorder) setMetaResponse {
	t.Helper()
	var r setMetaResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	return r
}

// TestStoreQueryHandler_SetKeyMeta_성공 은 정상 요청 시 setter 가 디코드된 인자로
// 호출되고 200 + 적용 결과를 반환함을 검증한다.
func TestStoreQueryHandler_SetKeyMeta_성공(t *testing.T) {
	var gotKey, gotMetric string
	var gotTags map[string]string
	fake := &fakeKeyMetaSetter{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		setFn: func(key, metricType string, tags map[string]string) error {
			gotKey, gotMetric, gotTags = key, metricType, tags
			return nil
		},
	}
	router := setupStoreQueryRouter(t, fake)

	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/store/store-a/keys/outdoor:humidity/meta",
		strings.NewReader(`{"metric_type":"humidity","tags":{"room":"kitchen"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeSetMeta(t, rec)
	assert.True(t, resp.Success)
	assert.Equal(t, "outdoor:humidity", resp.Data.Key)
	assert.Equal(t, "humidity", resp.Data.MetricType)
	assert.Equal(t, "kitchen", resp.Data.Tags["room"])

	// setter 가 디코드된 인자로 호출되었는지 검증.
	assert.Equal(t, "outdoor:humidity", gotKey)
	assert.Equal(t, "humidity", gotMetric)
	assert.Equal(t, "kitchen", gotTags["room"])
}

// TestStoreQueryHandler_SetKeyMeta_빈metric_type_unknown 은 metric_type 생략 시
// 응답이 "unknown" 으로 normalize 됨을 검증한다.
func TestStoreQueryHandler_SetKeyMeta_빈metric_type_unknown(t *testing.T) {
	fake := &fakeKeyMetaSetter{fakeAgentCommon: newFakeAgent("s1", "store-a", "store")}
	router := setupStoreQueryRouter(t, fake)

	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/store/store-a/keys/k/meta",
		strings.NewReader(`{"tags":{}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeSetMeta(t, rec)
	assert.Equal(t, "unknown", resp.Data.MetricType)
	assert.NotNil(t, resp.Data.Tags, "tags 는 null 이 아닌 빈 객체")
}

// TestStoreQueryHandler_SetKeyMeta_잘못된metric_type_400 은 setter 가
// ErrInvalidMetricType 을 반환하면 400 으로 매핑됨을 검증한다.
func TestStoreQueryHandler_SetKeyMeta_잘못된metric_type_400(t *testing.T) {
	fake := &fakeKeyMetaSetter{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		setFn: func(_, _ string, _ map[string]string) error {
			return system.ErrInvalidMetricType
		},
	}
	router := setupStoreQueryRouter(t, fake)

	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/store/store-a/keys/k/meta",
		strings.NewReader(`{"metric_type":"bad type!"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
}

// TestStoreQueryHandler_SetKeyMeta_잘못된tag_400 은 setter 가 ErrInvalidTagKey 를
// 반환하면 400 으로 매핑됨을 검증한다.
func TestStoreQueryHandler_SetKeyMeta_잘못된tag_400(t *testing.T) {
	fake := &fakeKeyMetaSetter{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		setFn: func(_, _ string, _ map[string]string) error {
			return system.ErrInvalidTagKey
		},
	}
	router := setupStoreQueryRouter(t, fake)

	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/store/store-a/keys/k/meta",
		strings.NewReader(`{"tags":{"bad key":"v"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
}

// TestStoreQueryHandler_SetKeyMeta_에이전트없음_404 는 미존재 에이전트가 404 임을 검증한다.
func TestStoreQueryHandler_SetKeyMeta_에이전트없음_404(t *testing.T) {
	router := setupStoreQueryRouter(t /* no agents */)

	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/store/ghost/keys/k/meta",
		strings.NewReader(`{"metric_type":"gauge"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code, "body=%s", rec.Body.String())
}

// TestStoreQueryHandler_SetKeyMeta_스토어아님_400 은 SetKeyMeta 미구현 에이전트가
// 400 임을 검증한다.
func TestStoreQueryHandler_SetKeyMeta_스토어아님_400(t *testing.T) {
	// fakeStoreAgent 는 QueryHistory 만 구현하고 SetKeyMeta 는 구현하지 않는다.
	other := &fakeStoreAgent{fakeAgentCommon: newFakeAgent("s1", "store-a", "store")}
	router := setupStoreQueryRouter(t, other)

	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/store/store-a/keys/k/meta",
		strings.NewReader(`{"metric_type":"gauge"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
}

// TestStoreQueryHandler_ListKeys_동적키_필터 는 동적(SourceAuto, string) 키가
// GET /keys 의 metric_type / tag 필터 대상에 포함됨을 검증한다 (M3).
// SetKeyMeta 로 동적 키에 metric_type/tags 를 부여하면 필터로 조회 가능하다.
func TestStoreQueryHandler_ListKeys_동적키_필터(t *testing.T) {
	ag := &fakeKeyMetaLister{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		staticKeys: map[string]system.StaticKeyMeta{
			// 동적 키: SetKeyMeta 로 metric_type/tags 가 부여된 상태.
			"runtime:power": {
				DataType:   system.DataTypeString,
				MetricType: "power",
				Tags:       map[string]string{"phase": "a"},
				Source:     system.SourceAuto,
			},
			// 동적 키: 기본값(unknown, 빈 태그).
			"runtime:misc": {
				DataType:   system.DataTypeString,
				MetricType: "unknown",
				Tags:       map[string]string{},
				Source:     system.SourceAuto,
			},
		},
	}
	router := setupStoreQueryRouter(t, ag)

	// metric_type 필터로 동적 키 조회.
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/store/store-a/keys?metric_type=power", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeListKeys(t, rec)
	require.Len(t, resp.Data.Keys, 1)
	assert.Equal(t, "runtime:power", resp.Data.Keys[0].Key)
	assert.Equal(t, "auto", resp.Data.Keys[0].Registration)
	assert.Equal(t, "string", resp.Data.Keys[0].DataType)

	// tag 필터로 동적 키 조회.
	req = httptest.NewRequest(http.MethodGet,
		"/api/v1/store/store-a/keys?tag=phase:a", nil)
	rec = httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp = decodeListKeys(t, rec)
	require.Len(t, resp.Data.Keys, 1)
	assert.Equal(t, "runtime:power", resp.Data.Keys[0].Key)

	// registration=auto 필터로 동적 키 전부 조회.
	req = httptest.NewRequest(http.MethodGet,
		"/api/v1/store/store-a/keys?registration=auto", nil)
	rec = httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp = decodeListKeys(t, rec)
	assert.Equal(t, 2, resp.Data.Count, "동적 키 2개 모두 조회")
}

// TestStoreQueryHandler_ListKeys_동적키_decoder 용 listKeysResponse 디코더는
// store_query_listkeys_test.go 의 decodeListKeys 를 재사용한다.

// TestStoreQueryHandler_SetKeyMeta_콜론키_디코딩 은 %3A 인코딩된 콜론이 키로
// 올바르게 디코드되어 setter 에 전달됨을 검증한다.
func TestStoreQueryHandler_SetKeyMeta_콜론키_디코딩(t *testing.T) {
	var gotKey string
	fake := &fakeKeyMetaSetter{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		setFn: func(key, _ string, _ map[string]string) error {
			gotKey = key
			return nil
		},
	}
	router := setupStoreQueryRouter(t, fake)

	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/store/store-a/keys/indoor%3A1%3Aroom_temp/meta",
		strings.NewReader(`{"metric_type":"temperature"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assert.Equal(t, "indoor:1:room_temp", gotKey)
}
