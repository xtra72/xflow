// @spec SPEC-STORE-003 v0.3.0
//
// 본 파일은 v0.2.0 의 ListKeys 핸들러 테스트를 v0.3.0 객체 배열 응답
// (`keys: [{key, registration, data_type, field, tags}, ...]`) 으로 마이그레이션한 결과이다.
// v0.2.0 의 검증된 동작 (목록 정상 반환, nil/빈배열, 정적키 태그 포함, 다중 ?tag= 필터, 잘못된
// 형식 거부) 은 그대로 보존되며, 응답 형상만 v0.3.0 envelope (StoreKeysListResponse) 로
// 진화한다.
//
// 또한 v0.3.0 신규 필터 (?data_type=, ?field=, ?registration=) 은 Phase F 의 별도
// 추가 테스트에서 다룬다 (본 파일은 Phase E 마이그레이션 범위로 제한).

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent/system"
)

// @spec SPEC-STORE-003 v0.3.0
// listKeysResponse 는 v0.3.0 응답 envelope 의 디코더이다.
// data: { count: int, keys: [{key, registration, data_type, field, tags}, ...] }.
type listKeysResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Count int                `json:"count"`
		Keys  []StoreKeyResponse `json:"keys"`
	} `json:"data"`
}

func decodeListKeys(t *testing.T, rec *httptest.ResponseRecorder) listKeysResponse {
	t.Helper()
	var r listKeysResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&r))
	return r
}

// keyByName 은 응답 keys 배열에서 key 이름이 일치하는 첫 항목 포인터를 반환한다.
// 디코딩 후 검증 helper.
func keyByName(keys []StoreKeyResponse, name string) *StoreKeyResponse {
	for i := range keys {
		if keys[i].Key == name {
			return &keys[i]
		}
	}
	return nil
}

// @spec SPEC-STORE-003 v0.3.0
// fakeKeyMetaLister 는 storeKeyMetaLister 인터페이스를 구현하는 페이크이다.
// StaticKeysSnapshot 은 생성자에서 주입된 정적 맵의 깊은 복사본을 반환한다.
//
// agent.Agent 인터페이스의 공통 no-op 메서드는 fakeAgentCommon 임베딩으로 제공된다.
// listFn 은 v0.2.0 ListKeys 가 사용했지만 v0.3.0 에서는 핸들러가 호출하지 않으므로 본
// 페이크에서는 의도적으로 생략한다 (불필요한 의존성 제거).
type fakeKeyMetaLister struct {
	*fakeAgentCommon
	staticKeys map[string]system.StaticKeyMeta
	// liveKeys 가 nil 이면 모든 등록 키를 live 로 간주한다(auto 유령 필터 미적용).
	liveKeys map[string]struct{}
}

func (f *fakeKeyMetaLister) StaticKeysSnapshot() map[string]system.StaticKeyMeta {
	if f.staticKeys == nil {
		return map[string]system.StaticKeyMeta{}
	}
	out := make(map[string]system.StaticKeyMeta, len(f.staticKeys))
	for k, meta := range f.staticKeys {
		// Tags 깊은 복사 (snapshot 계약).
		tagsCopy := make(map[string]string, len(meta.Tags))
		for tk, tv := range meta.Tags {
			tagsCopy[tk] = tv
		}
		out[k] = system.StaticKeyMeta{
			DataType: meta.DataType,
			Field:    meta.Field,
			Tags:     tagsCopy,
			Source:   meta.Source,
		}
	}
	return out
}

// LiveSeriesKeys 는 실데이터가 있는 시리즈 키 집합을 반환한다.
// liveKeys 가 nil 이면 모든 등록 키를 live 로 간주한다(auto 유령 필터 미적용 — 기존 테스트 호환).
func (f *fakeKeyMetaLister) LiveSeriesKeys() map[string]struct{} {
	if f.liveKeys != nil {
		return f.liveKeys
	}
	all := make(map[string]struct{}, len(f.staticKeys))
	for k := range f.staticKeys {
		all[k] = struct{}{}
	}
	return all
}

// ---------------------------------------------------------------------------
// 기본 응답 형상 / 빈 응답
// ---------------------------------------------------------------------------

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도 (Scenario 1 정상 동작): 정적 키가 있으면 응답에 객체 배열로 노출된다.
// v0.3.0 진화: 응답이 string 배열이 아니라 객체 배열로 변경되었으며, registration/data_type/
// field/tags 가 함께 노출된다. namespace/pattern 쿼리 파라미터는 v0.3.0 응답에 영향을
// 주지 않는다 (StaticKeysSnapshot 만 사용).
func TestStoreQueryHandler_ListKeys_성공(t *testing.T) {
	agentFake := &fakeKeyMetaLister{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		staticKeys: map[string]system.StaticKeyMeta{
			"a": {
				DataType: system.DataTypeString,
				Field:    "unknown",
				Tags:     map[string]string{},
				Source:   system.SourceManual,
			},
			"b": {
				DataType: system.DataTypeString,
				Field:    "unknown",
				Tags:     map[string]string{},
				Source:   system.SourceManual,
			},
			"c": {
				DataType: system.DataTypeString,
				Field:    "unknown",
				Tags:     map[string]string{},
				Source:   system.SourceManual,
			},
		},
	}
	router := setupStoreQueryRouter(t, agentFake)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/store/store-a/keys?namespace=ns1&pattern=foo*", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	resp := decodeListKeys(t, rec)
	assert.True(t, resp.Success)
	assert.Equal(t, 3, resp.Data.Count)
	require.Len(t, resp.Data.Keys, 3)
	// 알파벳순 정렬 보장 (M9).
	assert.Equal(t, "a", resp.Data.Keys[0].Key)
	assert.Equal(t, "b", resp.Data.Keys[1].Key)
	assert.Equal(t, "c", resp.Data.Keys[2].Key)
	// 각 객체는 v0.3.0 필드 5종을 모두 포함해야 한다.
	for _, k := range resp.Data.Keys {
		assert.Equal(t, "manual", k.Registration)
		assert.Equal(t, "string", k.DataType)
		assert.Equal(t, "unknown", k.Field)
		assert.NotNil(t, k.Tags, "tags 는 nil 이 아닌 빈 객체여야 한다 (M9)")
	}
}

// 실데이터 없는 시리즈(auto 유령 + bare 정적 정의)는 제외하고, 실데이터 있는 시리즈만
// 노출한다. (라인차트 Store 선택기 ↔ 저장소 탭 일치)
func TestStoreQueryHandler_ListKeys_필터_데이터없는시리즈제외(t *testing.T) {
	agentFake := &fakeKeyMetaLister{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		staticKeys: map[string]system.StaticKeyMeta{
			"m-empty": { // manual(bare 정적 정의) + 실데이터 없음: 제외.
				DataType: system.DataTypeBoolean, Field: "power",
				Tags: map[string]string{}, Source: system.SourceManual,
			},
			"m-live": { // manual + 실데이터 있음: 노출.
				DataType: system.DataTypeBoolean, Field: "power",
				Tags: map[string]string{}, Source: system.SourceManual,
			},
			"a-live": { // auto + 실데이터 있음: 노출.
				DataType: system.DataTypeBoolean, Field: "power",
				Tags: map[string]string{}, Source: system.SourceAuto,
			},
			"a-phantom": { // auto + 실데이터 없음: 유령 → 제외.
				DataType: system.DataTypeBoolean, Field: "power",
				Tags: map[string]string{}, Source: system.SourceAuto,
			},
		},
		liveKeys: map[string]struct{}{"m-live": {}, "a-live": {}},
	}
	router := setupStoreQueryRouter(t, agentFake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/store-a/keys", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeListKeys(t, rec)
	assert.Equal(t, 2, resp.Data.Count, "실데이터 있는 m-live + a-live 만 노출")
	assert.NotNil(t, keyByName(resp.Data.Keys, "m-live"), "실데이터 있는 manual 노출")
	assert.NotNil(t, keyByName(resp.Data.Keys, "a-live"), "실데이터 있는 auto 노출")
	assert.Nil(t, keyByName(resp.Data.Keys, "m-empty"), "데이터 없는 bare 정적 정의 제외")
	assert.Nil(t, keyByName(resp.Data.Keys, "a-phantom"), "유령 auto 제외")
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 정적 키가 하나도 없으면 count=0 + keys=[] 빈 배열을 반환한다.
// v0.3.0 진화: top-level tags 맵은 더 이상 존재하지 않으며, keys 배열의 각 객체에 tags 가
// 들어간다.
func TestStoreQueryHandler_ListKeys_정적키없음_빈배열(t *testing.T) {
	agentFake := &fakeKeyMetaLister{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		staticKeys:      nil,
	}
	router := setupStoreQueryRouter(t, agentFake)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/store-a/keys", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"keys":[]`)
	assert.Contains(t, rec.Body.String(), `"count":0`)
}

// ---------------------------------------------------------------------------
// 정적 키 태그 노출 (v0.2.0 _정적키_태그_포함 의 v0.3.0 진화)
// ---------------------------------------------------------------------------

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 정적 키의 태그가 응답에 포함된다. 자동 등록 키 (Source=auto) 도
// staticKeys 맵에 들어가므로 응답에 노출되지만 registration 필드가 "auto" 로 표시된다.
//
// v0.3.0 진화:
//   - top-level "tags" map → keys[].tags 객체로 이동.
//   - 동적/자동 등록 키도 keys 배열에 항상 등장 (v0.2.0 의 "tags 생략" 동작과 다름).
//   - 각 객체에 registration/data_type/field 가 추가 노출.
func TestStoreQueryHandler_ListKeys_정적키_태그_포함(t *testing.T) {
	ag := &fakeKeyMetaLister{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		staticKeys: map[string]system.StaticKeyMeta{
			"indoor:1:room_temp": {
				DataType: system.DataTypeFloat,
				Field:    "temperature",
				Tags:     map[string]string{"room": "1", "type": "temperature"},
				Source:   system.SourceManual,
			},
			"outdoor:temperature": {
				DataType: system.DataTypeFloat,
				Field:    "temperature",
				Tags:     map[string]string{"location": "outside", "type": "temperature"},
				Source:   system.SourceManual,
			},
			"dynamic_key": {
				DataType: system.DataTypeString,
				Field:    "unknown",
				Tags:     map[string]string{},
				Source:   system.SourceAuto,
			},
		},
	}
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/store-a/keys", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeListKeys(t, rec)
	assert.True(t, resp.Success)
	assert.Equal(t, 3, resp.Data.Count)

	// 알파벳순 검증.
	keys := resp.Data.Keys
	require.Len(t, keys, 3)
	assert.Equal(t, "dynamic_key", keys[0].Key)
	assert.Equal(t, "indoor:1:room_temp", keys[1].Key)
	assert.Equal(t, "outdoor:temperature", keys[2].Key)

	// 정적 키의 태그/메타 검증.
	indoor := keyByName(keys, "indoor:1:room_temp")
	require.NotNil(t, indoor)
	assert.Equal(t, "manual", indoor.Registration)
	assert.Equal(t, "float", indoor.DataType)
	assert.Equal(t, "temperature", indoor.Field)
	assert.Equal(t, "1", indoor.Tags["room"])
	assert.Equal(t, "temperature", indoor.Tags["type"])

	outdoor := keyByName(keys, "outdoor:temperature")
	require.NotNil(t, outdoor)
	assert.Equal(t, "manual", outdoor.Registration)

	// 자동 등록된 동적 키도 응답에 포함되며, Tags 는 nil 이 아닌 빈 객체로 응답되어야 한다 (M9).
	dyn := keyByName(keys, "dynamic_key")
	require.NotNil(t, dyn)
	assert.Equal(t, "auto", dyn.Registration)
	assert.Equal(t, "string", dyn.DataType)
	assert.Equal(t, "unknown", dyn.Field)
	assert.NotNil(t, dyn.Tags)
	assert.Empty(t, dyn.Tags)
}

// ---------------------------------------------------------------------------
// ?tag= 필터 (v0.2.0 와 동일 의도, 응답 형상만 진화)
// ---------------------------------------------------------------------------

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도 (Scenario 4): ?tag=room:1 한 개로 정확히 일치하는 키만 반환된다.
func TestStoreQueryHandler_ListKeys_태그필터_단일_매칭(t *testing.T) {
	ag := &fakeKeyMetaLister{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		staticKeys: map[string]system.StaticKeyMeta{
			"indoor:1:room_temp": {
				DataType: system.DataTypeFloat,
				Field:    "temperature",
				Tags:     map[string]string{"room": "1", "type": "temperature"},
				Source:   system.SourceManual,
			},
			"indoor:2:room_temp": {
				DataType: system.DataTypeFloat,
				Field:    "temperature",
				Tags:     map[string]string{"room": "2", "type": "temperature"},
				Source:   system.SourceManual,
			},
			"outdoor:temperature": {
				DataType: system.DataTypeFloat,
				Field:    "temperature",
				Tags:     map[string]string{"location": "outside", "type": "temperature"},
				Source:   system.SourceManual,
			},
		},
	}
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/store-a/keys?tag=room:1", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeListKeys(t, rec)
	assert.Equal(t, 1, resp.Data.Count)
	require.Len(t, resp.Data.Keys, 1)
	assert.Equal(t, "indoor:1:room_temp", resp.Data.Keys[0].Key)
	assert.Equal(t, "1", resp.Data.Keys[0].Tags["room"])
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도 (Scenario 4): 다중 ?tag= 는 AND 결합되어 모든 조건 만족 키만 반환된다.
func TestStoreQueryHandler_ListKeys_태그필터_다중_AND_매칭(t *testing.T) {
	ag := &fakeKeyMetaLister{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		staticKeys: map[string]system.StaticKeyMeta{
			"indoor:1:room_temp": {
				DataType: system.DataTypeFloat, Field: "temperature",
				Tags:   map[string]string{"room": "1", "type": "temperature"},
				Source: system.SourceManual,
			},
			"indoor:2:room_temp": {
				DataType: system.DataTypeFloat, Field: "temperature",
				Tags:   map[string]string{"room": "2", "type": "temperature"},
				Source: system.SourceManual,
			},
			"outdoor:temperature": {
				DataType: system.DataTypeFloat, Field: "temperature",
				Tags:   map[string]string{"location": "outside", "type": "temperature"},
				Source: system.SourceManual,
			},
		},
	}
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/store/store-a/keys?tag=type:temperature&tag=room:2", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeListKeys(t, rec)
	assert.Equal(t, 1, resp.Data.Count)
	require.Len(t, resp.Data.Keys, 1)
	assert.Equal(t, "indoor:2:room_temp", resp.Data.Keys[0].Key)
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 매칭되는 키가 없으면 count=0 + 빈 배열을 반환한다.
func TestStoreQueryHandler_ListKeys_태그필터_매칭없음_빈결과(t *testing.T) {
	ag := &fakeKeyMetaLister{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		staticKeys: map[string]system.StaticKeyMeta{
			"indoor:1:room_temp": {
				DataType: system.DataTypeFloat, Field: "temperature",
				Tags:   map[string]string{"room": "1"},
				Source: system.SourceManual,
			},
		},
	}
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/store/store-a/keys?tag=room:999", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeListKeys(t, rec)
	assert.Equal(t, 0, resp.Data.Count)
	assert.Empty(t, resp.Data.Keys)
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 콜론이 없는 ?tag= 값은 400 BadRequest 로 거부된다.
func TestStoreQueryHandler_ListKeys_태그필터_잘못된형식_400(t *testing.T) {
	ag := &fakeKeyMetaLister{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
	}
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/store/store-a/keys?tag=justkey", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid tag format: expected key:value")
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: ?tag=key:value:extra 형태에서 첫 콜론까지만 key 로, 나머지 전체가
// value 로 해석된다 (URL 등 콜론이 포함된 값 허용).
func TestStoreQueryHandler_ListKeys_태그필터_value내콜론_허용(t *testing.T) {
	ag := &fakeKeyMetaLister{
		fakeAgentCommon: newFakeAgent("s1", "store-a", "store"),
		staticKeys: map[string]system.StaticKeyMeta{
			"url_key": {
				DataType: system.DataTypeString, Field: "unknown",
				Tags:   map[string]string{"scheme": "https://example"},
				Source: system.SourceManual,
			},
		},
	}
	router := setupStoreQueryRouter(t, ag)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/store/store-a/keys?tag=scheme:https://example", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	resp := decodeListKeys(t, rec)
	assert.Equal(t, 1, resp.Data.Count)
	require.Len(t, resp.Data.Keys, 1)
	assert.Equal(t, "url_key", resp.Data.Keys[0].Key)
	assert.Equal(t, "https://example", resp.Data.Keys[0].Tags["scheme"])
}
