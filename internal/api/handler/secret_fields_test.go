package handler

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsSensitiveConfigKey(t *testing.T) {
	t.Parallel()

	sensitive := []string{
		"password", "token", "secret", "api_key", "apikey",
		"access_token", "auth_token", "client_secret", "private_key",
		"passphrase", "username",
	}
	for _, k := range sensitive {
		assert.Truef(t, IsSensitiveConfigKey(k), "%q 는 민감 키여야 한다", k)
	}

	// 대소문자 무시 확인
	caseVariants := []string{"PASSWORD", "Token", "Api_Key", "APIKEY", "Access_Token", "USERNAME"}
	for _, k := range caseVariants {
		assert.Truef(t, IsSensitiveConfigKey(k), "%q 는 대소문자 무시로 민감 키여야 한다", k)
	}

	// 비민감 키
	nonSensitive := []string{"host", "port", "name", "topic", "url", "interval", "user"}
	for _, k := range nonSensitive {
		assert.Falsef(t, IsSensitiveConfigKey(k), "%q 는 민감 키가 아니어야 한다", k)
	}
}

func TestRedactSensitiveConfig_NilReturnsNil(t *testing.T) {
	t.Parallel()
	assert.Nil(t, RedactSensitiveConfig(nil))
}

func TestRedactSensitiveConfig_TopLevel(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		"host":     "broker.local",
		"port":     1883,
		"password": "s3cr3t",
		"Token":    "abc",   // 대소문자 무시
		"username": "admin", // username 도 민감 키로 취급
	}
	out := RedactSensitiveConfig(in)

	// 비민감 키는 보존
	assert.Equal(t, "broker.local", out["host"])
	assert.Equal(t, 1883, out["port"])

	// 민감 키는 제거
	assert.NotContains(t, out, "password")
	assert.NotContains(t, out, "Token")
	assert.NotContains(t, out, "username")
}

func TestRedactSensitiveConfig_DoesNotMutateInput(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		"host":     "broker.local",
		"password": "s3cr3t",
		"headers": map[string]any{
			"api_key": "k123",
			"accept":  "application/json",
		},
	}
	// 입력 스냅샷
	snapshot := map[string]any{
		"host":     "broker.local",
		"password": "s3cr3t",
		"headers": map[string]any{
			"api_key": "k123",
			"accept":  "application/json",
		},
	}

	_ = RedactSensitiveConfig(in)

	// 입력은 그대로여야 한다 (라이브 config 보호)
	assert.True(t, reflect.DeepEqual(in, snapshot), "입력 맵이 변경되어서는 안 된다")
	// 중첩 맵도 그대로여야 한다
	headers := in["headers"].(map[string]any)
	assert.Equal(t, "k123", headers["api_key"], "중첩 맵의 민감 값도 입력에서는 보존되어야 한다")
}

func TestRedactSensitiveConfig_NestedMaps(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		"host": "broker.local",
		"headers": map[string]any{
			"Authorization": "Bearer x",
			"api_key":       "k123",
			"accept":        "json",
		},
		"config": map[string]any{
			"client_secret": "cs",
			"timeout":       30,
		},
	}
	out := RedactSensitiveConfig(in)

	headers := out["headers"].(map[string]any)
	assert.NotContains(t, headers, "api_key", "중첩 headers 의 api_key 가 제거되어야 한다")
	assert.Equal(t, "Bearer x", headers["Authorization"], "비민감 키는 보존")
	assert.Equal(t, "json", headers["accept"])

	nestedCfg := out["config"].(map[string]any)
	assert.NotContains(t, nestedCfg, "client_secret")
	assert.Equal(t, 30, nestedCfg["timeout"])
}

func TestRedactSensitiveConfig_NestedSlices(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		"endpoints": []any{
			map[string]any{"url": "a", "token": "t1"},
			map[string]any{"url": "b", "secret": "s2"},
		},
	}
	out := RedactSensitiveConfig(in)

	eps := out["endpoints"].([]any)
	require.Len(t, eps, 2)
	e0 := eps[0].(map[string]any)
	assert.Equal(t, "a", e0["url"])
	assert.NotContains(t, e0, "token")
	e1 := eps[1].(map[string]any)
	assert.NotContains(t, e1, "secret")
}

func TestCollectSensitiveKeys(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		"host":     "broker.local",
		"password": "s3cr3t",
		"Token":    "abc",
		"headers": map[string]any{
			"api_key": "k123",
		},
	}
	keys := CollectSensitiveKeys(in)

	// 정렬된 결과 (대소문자 무시 중복 제거, 정규 소문자 이름으로 반환)
	// 등장: password, Token, api_key → 정규화 후 정렬: api_key, password, token
	assert.Equal(t, []string{"api_key", "password", "token"}, keys)
}

func TestCollectSensitiveKeys_DeduplicatesCaseInsensitive(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		"password": "a",
		"headers": map[string]any{
			"PASSWORD": "b", // 동일 키 다른 케이스 → 한 번만
		},
	}
	keys := CollectSensitiveKeys(in)
	assert.Len(t, keys, 1)
	assert.Equal(t, "password", keys[0]) // 정규(소문자) 이름
}

func TestCollectSensitiveKeys_EmptyReturnsNil(t *testing.T) {
	t.Parallel()
	assert.Nil(t, CollectSensitiveKeys(nil))
	assert.Nil(t, CollectSensitiveKeys(map[string]any{"host": "x", "port": 1}))
}

func TestCollectSensitiveKeys_RecursesIntoSlices(t *testing.T) {
	t.Parallel()

	in := map[string]any{
		"endpoints": []any{
			map[string]any{"url": "a", "token": "t1"},
			map[string]any{"url": "b", "secret": "s2"},
			"non-map-element", // map 이 아닌 요소는 무시되어야 한다
		},
	}
	keys := CollectSensitiveKeys(in)
	assert.Equal(t, []string{"secret", "token"}, keys)
}
