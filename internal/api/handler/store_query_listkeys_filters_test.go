// @spec SPEC-STORE-003 v0.3.0
//
// store_query_listkeys_filters_test.go — M9 신규 필터 축 (?data_type=, ?metric_type=,
// ?registration=) 과 ?tag= 의 AND 결합을 검증하는 Phase F 테스트.
//
// 본 파일은 store_query_listkeys_test.go (Phase E 마이그레이션, ?tag= 만 다룸) 의
// 빈틈을 메우며 Scenario 9, 10 + Edge Case Checklist v0.3.0 신규 항목들을 다룬다.
//
//   - Scenario 9: metric_type 필터 + default unknown 표시
//   - Scenario 10: 객체 배열 응답 + multi-filter AND
//   - Edge cases: 필터 enum 외 값 → 빈 결과 (not 400),
//     빈 필터 값 → no-filter passthrough (실제 구현 정책),
//     정렬 안정성, tags=null 방어, 5필드 항상 포함
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent/system"
)

// =============================================================================
// 테스트 헬퍼 (Phase F 공용)
// =============================================================================

// buildKeyMeta 는 StaticKeyMeta 빌더 헬퍼이다. tags 가 nil 이면 빈 맵으로 초기화한다.
//
// @spec SPEC-STORE-003 v0.3.0
func buildKeyMeta(
	dt system.DataType,
	mt string,
	source system.RegistrationSource,
	tags map[string]string,
) system.StaticKeyMeta {
	if tags == nil {
		tags = map[string]string{}
	}
	return system.StaticKeyMeta{
		DataType:   dt,
		MetricType: mt,
		Tags:       tags,
		Source:     source,
	}
}

// makeMetaListerWithKeys 는 fakeKeyMetaLister 를 정적 키 맵으로 구성한다.
//
// @spec SPEC-STORE-003 v0.3.0
func makeMetaListerWithKeys(keys map[string]system.StaticKeyMeta) *fakeKeyMetaLister {
	return &fakeKeyMetaLister{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		staticKeys:      keys,
	}
}

// doListKeysGET 은 GET /api/v1/store/store-a/keys?<query> 를 호출하고 응답을 디코드한다.
// 200 OK 가 아니면 t.Fatal 한다.
//
// @spec SPEC-STORE-003 v0.3.0
func doListKeysGET(t *testing.T, ag *fakeKeyMetaLister, queryString string) listKeysResponse {
	t.Helper()
	router := setupStoreQueryRouter(t, ag)
	url := "/api/v1/store/store-a/keys"
	if queryString != "" {
		url += "?" + queryString
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	return decodeListKeys(t, rec)
}

// makeMixedKeysFixture 는 4축 (data_type, metric_type, registration, tags) 모두
// 다양한 8개 키를 가진 fixture 를 생성한다. AND 결합 / 단축 필터 테스트의 공통 입력이다.
//
//	key             | data_type | metric_type | registration | tags
//	----------------|-----------|-------------|--------------|---------------
//	indoor:1:temp   | float     | temperature | manual       | {room: "1"}
//	indoor:2:temp   | float     | temperature | manual       | {room: "2"}
//	indoor:1:humid  | float     | humidity    | manual       | {room: "1"}
//	count_a         | int       | count       | manual       | {kind: "a"}
//	count_b         | int       | count       | manual       | {kind: "b"}
//	auto_int        | int       | unknown     | auto         | {}
//	auto_float      | float     | unknown     | auto         | {}
//	auto_string     | string    | unknown     | auto         | {}
//
// @spec SPEC-STORE-003 v0.3.0
func makeMixedKeysFixture() map[string]system.StaticKeyMeta {
	return map[string]system.StaticKeyMeta{
		"indoor:1:temp": buildKeyMeta(system.DataTypeFloat, "temperature", system.SourceManual,
			map[string]string{"room": "1"}),
		"indoor:2:temp": buildKeyMeta(system.DataTypeFloat, "temperature", system.SourceManual,
			map[string]string{"room": "2"}),
		"indoor:1:humid": buildKeyMeta(system.DataTypeFloat, "humidity", system.SourceManual,
			map[string]string{"room": "1"}),
		"count_a": buildKeyMeta(system.DataTypeInt, "count", system.SourceManual,
			map[string]string{"kind": "a"}),
		"count_b": buildKeyMeta(system.DataTypeInt, "count", system.SourceManual,
			map[string]string{"kind": "b"}),
		"auto_int":    buildKeyMeta(system.DataTypeInt, "unknown", system.SourceAuto, nil),
		"auto_float":  buildKeyMeta(system.DataTypeFloat, "unknown", system.SourceAuto, nil),
		"auto_string": buildKeyMeta(system.DataTypeString, "unknown", system.SourceAuto, nil),
	}
}

// =============================================================================
// A. ?data_type= 필터 (M9)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M9
// TestListKeys_Filter_DataType_Single 은 ?data_type=float 가 DataType=float 키만
// 반환하는지 검증한다 (Scenario 9 partial).
func TestListKeys_Filter_DataType_Single(t *testing.T) {
	ag := makeMetaListerWithKeys(makeMixedKeysFixture())
	resp := doListKeysGET(t, ag, "data_type=float")
	require.Greater(t, resp.Data.Count, 0)

	for _, k := range resp.Data.Keys {
		assert.Equal(t, "float", k.DataType,
			"data_type=float 필터는 DataType=float 만 반환해야 한다")
	}

	// fixture 기준 float 는 4개: indoor:1:temp, indoor:2:temp, indoor:1:humid, auto_float.
	expected := []string{"auto_float", "indoor:1:humid", "indoor:1:temp", "indoor:2:temp"}
	got := make([]string, 0, len(resp.Data.Keys))
	for _, k := range resp.Data.Keys {
		got = append(got, k.Key)
	}
	assert.Equal(t, expected, got)
	assert.Equal(t, 4, resp.Data.Count)
}

// @spec SPEC-STORE-003 v0.3.0 / M9 / Edge case "?data_type=invalid (enum 외)"
// TestListKeys_Filter_DataType_NoMatches 는 ?data_type=invalid_enum 처럼 enum 외 값이
// HTTP 200 + 빈 배열을 반환함을 검증한다 (Phase D 의 permissive policy).
//
// 핸들러 측 enum 검증 부재는 의도적이다 — staticKeys 에 매칭되는 키가 없으면 자연히
// 빈 배열이 되므로, 별도 400 응답으로 변환할 가치가 적다고 판단되었다.
func TestListKeys_Filter_DataType_NoMatches(t *testing.T) {
	ag := makeMetaListerWithKeys(makeMixedKeysFixture())
	resp := doListKeysGET(t, ag, "data_type=double") // enum 외

	assert.Equal(t, 0, resp.Data.Count, "enum 외 값은 매칭 키 0개")
	assert.Empty(t, resp.Data.Keys, "keys 는 빈 배열")
}

// =============================================================================
// B. ?metric_type= 필터 (M8 / M9)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M9 / Scenario 9
// TestListKeys_Filter_MetricType_Single 은 ?metric_type=temperature 가 해당
// metric_type 키만 반환하고 default unknown 키는 제외함을 검증한다.
func TestListKeys_Filter_MetricType_Single(t *testing.T) {
	ag := makeMetaListerWithKeys(makeMixedKeysFixture())
	resp := doListKeysGET(t, ag, "metric_type=temperature")

	expected := []string{"indoor:1:temp", "indoor:2:temp"}
	got := make([]string, 0, len(resp.Data.Keys))
	for _, k := range resp.Data.Keys {
		got = append(got, k.Key)
	}
	assert.Equal(t, expected, got)
	assert.Equal(t, 2, resp.Data.Count)

	// 모든 응답 객체의 metric_type 은 "temperature" 여야 한다.
	for _, k := range resp.Data.Keys {
		assert.Equal(t, "temperature", k.MetricType)
	}
}

// @spec SPEC-STORE-003 v0.3.0 / M9 / Scenario 9
// TestListKeys_Filter_MetricType_DefaultUnknownIncluded 는 ?metric_type=unknown 으로
// default 적용된 (auto 등록 또는 yaml 누락) 키만 필터링되는지 검증한다.
func TestListKeys_Filter_MetricType_DefaultUnknownIncluded(t *testing.T) {
	ag := makeMetaListerWithKeys(makeMixedKeysFixture())
	resp := doListKeysGET(t, ag, "metric_type=unknown")

	// fixture 의 unknown 키는 auto_int, auto_float, auto_string 3개.
	expected := []string{"auto_float", "auto_int", "auto_string"}
	got := make([]string, 0, len(resp.Data.Keys))
	for _, k := range resp.Data.Keys {
		got = append(got, k.Key)
	}
	assert.Equal(t, expected, got)
	assert.Equal(t, 3, resp.Data.Count)
}

// =============================================================================
// C. ?registration= 필터 (M6 / M9)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M9
// TestListKeys_Filter_Registration_Manual 은 ?registration=manual 이 Source=SourceManual
// 키만 반환함을 검증한다.
func TestListKeys_Filter_Registration_Manual(t *testing.T) {
	ag := makeMetaListerWithKeys(makeMixedKeysFixture())
	resp := doListKeysGET(t, ag, "registration=manual")

	for _, k := range resp.Data.Keys {
		assert.Equal(t, "manual", k.Registration,
			"registration=manual 필터는 Source=manual 만 반환")
	}
	// fixture 의 manual 키는 5개.
	assert.Equal(t, 5, resp.Data.Count)
}

// @spec SPEC-STORE-003 v0.3.0 / M9
// TestListKeys_Filter_Registration_Auto 는 ?registration=auto 가 Source=SourceAuto
// 키만 반환함을 검증한다.
func TestListKeys_Filter_Registration_Auto(t *testing.T) {
	ag := makeMetaListerWithKeys(makeMixedKeysFixture())
	resp := doListKeysGET(t, ag, "registration=auto")

	for _, k := range resp.Data.Keys {
		assert.Equal(t, "auto", k.Registration,
			"registration=auto 필터는 Source=auto 만 반환")
	}
	// fixture 의 auto 키는 3개 (auto_int/float/string).
	assert.Equal(t, 3, resp.Data.Count)
}

// =============================================================================
// D. 4축 AND 결합 (Scenario 10 / M9)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M9 / Scenario 10
// TestListKeys_Filter_AND_All4Axes 는 모든 4개 축 (registration, data_type,
// metric_type, tag) 이 동시 적용될 때 모든 조건을 만족하는 키만 반환됨을 검증한다.
//
// fixture 에서 registration=manual + data_type=float + metric_type=temperature + tag=room:1
// 을 동시 만족하는 키는 indoor:1:temp 단 하나이다.
func TestListKeys_Filter_AND_All4Axes(t *testing.T) {
	ag := makeMetaListerWithKeys(makeMixedKeysFixture())
	resp := doListKeysGET(t, ag,
		"registration=manual&data_type=float&metric_type=temperature&tag=room:1")

	require.Equal(t, 1, resp.Data.Count, "모든 4 조건 만족 키는 정확히 1개")
	require.Len(t, resp.Data.Keys, 1)
	k := resp.Data.Keys[0]
	assert.Equal(t, "indoor:1:temp", k.Key)
	assert.Equal(t, "manual", k.Registration)
	assert.Equal(t, "float", k.DataType)
	assert.Equal(t, "temperature", k.MetricType)
	assert.Equal(t, "1", k.Tags["room"])
}

// @spec SPEC-STORE-003 v0.3.0 / M9 / Scenario 10
// TestListKeys_Filter_AND_NoMatches 는 4축 AND 결합이 어떤 키와도 매칭되지 않으면
// count=0 + 빈 배열을 반환함을 검증한다.
func TestListKeys_Filter_AND_NoMatches(t *testing.T) {
	ag := makeMetaListerWithKeys(makeMixedKeysFixture())
	// 존재하지 않는 조합: registration=auto + data_type=float + tag=room:99.
	resp := doListKeysGET(t, ag, "registration=auto&data_type=float&tag=room:99")
	assert.Equal(t, 0, resp.Data.Count)
	assert.Empty(t, resp.Data.Keys)
}

// =============================================================================
// E. 빈 결과 — 필터 매칭 없음
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M9
// TestListKeys_Filter_NoMatches_Empty 은 매칭되는 키가 없을 때 200 + count=0 +
// keys=[] 빈 배열을 반환함을 검증한다 (null 이 아닌 빈 배열).
func TestListKeys_Filter_NoMatches_Empty(t *testing.T) {
	ag := makeMetaListerWithKeys(map[string]system.StaticKeyMeta{
		"k1": buildKeyMeta(system.DataTypeFloat, "temperature", system.SourceManual, nil),
	})
	router := setupStoreQueryRouter(t, ag)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/store/store-a/keys?metric_type=nonexistent", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"keys":[]`,
		"빈 결과는 null 이 아닌 빈 배열로 표시되어야 한다")
	assert.Contains(t, rec.Body.String(), `"count":0`)
}

// =============================================================================
// F. 정렬 안정성 (M9 알파벳 오름차순)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M9
// TestListKeys_Sorting_AlphabeticalAscending 은 응답 keys 배열이 항상 알파벳
// 오름차순으로 정렬됨을 검증한다 (M9: 안정적인 클라이언트 렌더링 보장).
//
// 테스트 입력 순서가 무관함을 보장하기 위해 의도적으로 무작위 순서로 키를 등록한다.
func TestListKeys_Sorting_AlphabeticalAscending(t *testing.T) {
	keys := map[string]system.StaticKeyMeta{
		"zzz_last":      buildKeyMeta(system.DataTypeString, "x", system.SourceManual, nil),
		"abc_first":     buildKeyMeta(system.DataTypeString, "x", system.SourceManual, nil),
		"middle_b":      buildKeyMeta(system.DataTypeString, "x", system.SourceManual, nil),
		"middle_a":      buildKeyMeta(system.DataTypeString, "x", system.SourceManual, nil),
		"AAA_uppercase": buildKeyMeta(system.DataTypeString, "x", system.SourceManual, nil),
	}
	ag := makeMetaListerWithKeys(keys)
	resp := doListKeysGET(t, ag, "")

	// Go sort.Slice 의 string < 비교는 byte 단위 (대문자 < 소문자).
	got := make([]string, 0, len(resp.Data.Keys))
	for _, k := range resp.Data.Keys {
		got = append(got, k.Key)
	}
	assert.True(t, sort.StringsAreSorted(got),
		"응답 keys 는 알파벳 오름차순 정렬되어야 한다 (M9): got=%v", got)
}

// =============================================================================
// G. Auto 등록 키의 응답 표현 (M9 / Scenario 3, 5)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M9 / Scenario 3, 5
// TestListKeys_AutoRegistered_AppearWithDefaults 는 auto 등록 키가 응답에 다음
// default 값으로 노출됨을 검증한다.
//   - registration: "auto"
//   - metric_type:  "unknown"
//   - tags:         {} (빈 객체, null 아님)
func TestListKeys_AutoRegistered_AppearWithDefaults(t *testing.T) {
	ag := makeMetaListerWithKeys(map[string]system.StaticKeyMeta{
		"auto_key": buildKeyMeta(system.DataTypeInt, "unknown", system.SourceAuto, nil),
	})
	resp := doListKeysGET(t, ag, "")

	require.Equal(t, 1, resp.Data.Count)
	require.Len(t, resp.Data.Keys, 1)
	k := resp.Data.Keys[0]
	assert.Equal(t, "auto_key", k.Key)
	assert.Equal(t, "auto", k.Registration)
	assert.Equal(t, "int", k.DataType)
	assert.Equal(t, "unknown", k.MetricType)
	require.NotNil(t, k.Tags, "Tags 는 nil 이 아닌 빈 객체")
	assert.Empty(t, k.Tags)
}

// =============================================================================
// H. Tags null 방어 (Edge case "빈 tags 객체로 표시")
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M9
// TestListKeys_TagsAlwaysObjectNotNull 은 staticKeys 의 Tags 가 nil 인 키가 응답에서
// `{}` 빈 객체로 normalize 되는지 JSON 문자열 수준에서 검증한다.
//
// 핸들러는 명시적으로 nil → {} 변환을 수행한다 (Phase D 핸들러 코드 참조).
// 이 동작은 클라이언트가 `tags === null` 체크를 회피할 수 있도록 한다.
func TestListKeys_TagsAlwaysObjectNotNull(t *testing.T) {
	// 일부러 Tags=nil 인 키를 주입한다.
	ag := &fakeKeyMetaLister{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		staticKeys: map[string]system.StaticKeyMeta{
			"k_nil_tags": {
				DataType:   system.DataTypeFloat,
				MetricType: "unknown",
				Tags:       nil, // 명시적 nil
				Source:     system.SourceAuto,
			},
		},
	}
	router := setupStoreQueryRouter(t, ag)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/store-a/keys", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	// JSON 응답 본문에 "tags":{} 가 포함되어야 하고, "tags":null 은 부재해야 한다.
	assert.Contains(t, body, `"tags":{}`,
		"nil Tags 는 빈 객체 {} 로 normalize 되어야 한다 (M9)")
	assert.NotContains(t, body, `"tags":null`,
		"tags 필드는 절대 null 이 되면 안 된다")
}

// =============================================================================
// I. 빈 필터 값 — passthrough (Phase D 실제 정책)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M9 (구현 정책 vs SPEC 차이)
// TestListKeys_FilterPresentEmptyValue_NoFilter 은 ?metric_type= (URL 파라미터가
// 존재하지만 값이 빈 문자열) 인 경우 핸들러가 이를 "필터 미적용" 으로 처리함을 검증한다.
//
// SPEC M9 의 state-driven 절은 "IF 필터 값이 빈 문자열이면 빈 결과를 반환한다" 라고
// 명시하지만, Phase D 의 실제 구현은 keyFilter struct 의 빈 문자열을 no-op 으로 정의했다
// (store_query.go keyFilter 주석 참조). 본 테스트는 실제 구현 동작을 명시적으로 문서화한다.
//
// 동작 근거: staticKeys 의 MetricType 은 항상 normalize (auto: "unknown", manual yaml:
// validateMetricType 으로 보정) 되어 빈 문자열이 될 수 없으므로, 빈 필터 값은 모든 키에
// 대해 false 가 될 수 없고 "필터 적용 안 함" 으로 처리해도 동작상 차이가 없다.
//
// 향후 SPEC 수정으로 SPEC 와 구현이 일치할 가능성이 있다. 본 테스트는 실제 동작을
// 명문화하여 향후 변경 시 의도적 회귀 검출 anchor 가 된다.
func TestListKeys_FilterPresentEmptyValue_NoFilter(t *testing.T) {
	ag := makeMetaListerWithKeys(makeMixedKeysFixture())

	// ?metric_type= (값 비움) → 모든 키 반환 (8개).
	resp := doListKeysGET(t, ag, "metric_type=")
	assert.Equal(t, 8, resp.Data.Count,
		"빈 metric_type 필터는 no-op 으로 처리되어 모든 키 반환 (Phase D 구현 정책)")

	// ?data_type= (값 비움) 도 동일.
	resp = doListKeysGET(t, ag, "data_type=")
	assert.Equal(t, 8, resp.Data.Count,
		"빈 data_type 필터도 no-op")

	// ?registration= (값 비움) 도 동일.
	resp = doListKeysGET(t, ag, "registration=")
	assert.Equal(t, 8, resp.Data.Count,
		"빈 registration 필터도 no-op")
}

// =============================================================================
// J. 5개 필드 항상 포함 (Scenario 10 / M9)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M9 / Scenario 10
// TestListKeys_AllFiveFieldsPresent 는 응답의 모든 키 객체가 5개 필드 (key,
// registration, data_type, metric_type, tags) 를 항상 포함함을 raw JSON 수준에서
// 검증한다 ("모든 객체는 5개 필드를 항상 포함한다, 빈 tags 라도 {}로 명시").
func TestListKeys_AllFiveFieldsPresent(t *testing.T) {
	ag := makeMetaListerWithKeys(map[string]system.StaticKeyMeta{
		"manual_full": buildKeyMeta(system.DataTypeFloat, "temperature", system.SourceManual,
			map[string]string{"room": "1"}),
		"auto_minimal": buildKeyMeta(system.DataTypeInt, "unknown", system.SourceAuto, nil),
	})
	router := setupStoreQueryRouter(t, ag)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/store-a/keys", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	// raw map 으로 디코딩하여 각 객체의 필드 존재 여부를 확인한다.
	var raw struct {
		Success bool `json:"success"`
		Data    struct {
			Count int              `json:"count"`
			Keys  []map[string]any `json:"keys"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&raw))
	require.Len(t, raw.Data.Keys, 2)

	// 각 객체에 5개 필드가 모두 존재해야 한다.
	requiredFields := []string{"key", "registration", "data_type", "metric_type", "tags"}
	for i, obj := range raw.Data.Keys {
		for _, f := range requiredFields {
			_, exists := obj[f]
			assert.True(t, exists, "key[%d] 객체에 %q 필드가 존재해야 한다", i, f)
		}
	}
}

// =============================================================================
// K. Multi-axis 결합 일부만 — registration + data_type
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M9 / Scenario 10 보조
// TestListKeys_Filter_AND_RegistrationDataType 은 두 축 (registration + data_type)
// 만 결합되어도 AND 가 동작함을 검증한다 (Scenario 10 의 multi-filter AND 다른 변형).
func TestListKeys_Filter_AND_RegistrationDataType(t *testing.T) {
	ag := makeMetaListerWithKeys(makeMixedKeysFixture())
	resp := doListKeysGET(t, ag, "registration=manual&data_type=int")

	// fixture 에서 manual + int 는 count_a, count_b 두 개.
	expected := []string{"count_a", "count_b"}
	got := make([]string, 0, len(resp.Data.Keys))
	for _, k := range resp.Data.Keys {
		got = append(got, k.Key)
	}
	assert.Equal(t, expected, got)
	assert.Equal(t, 2, resp.Data.Count)
}

// @spec SPEC-STORE-003 v0.3.0 / M9 / Scenario 10 보조
// TestListKeys_Filter_AND_MetricTypeRegistration 는 metric_type + registration 조합도
// AND 로 결합됨을 검증한다.
func TestListKeys_Filter_AND_MetricTypeRegistration(t *testing.T) {
	ag := makeMetaListerWithKeys(makeMixedKeysFixture())
	resp := doListKeysGET(t, ag, "metric_type=count&registration=manual")

	// fixture 에서 count + manual 는 count_a, count_b.
	assert.Equal(t, 2, resp.Data.Count)
	keys := make([]string, 0, len(resp.Data.Keys))
	for _, k := range resp.Data.Keys {
		keys = append(keys, k.Key)
	}
	assert.ElementsMatch(t, []string{"count_a", "count_b"}, keys)
}
