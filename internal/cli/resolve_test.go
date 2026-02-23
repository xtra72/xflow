package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- resolveEntityArg 테스트 ---

func TestResolveEntityArg_PositionalOnly(t *testing.T) {
	result, err := resolveEntityArg([]string{"agent-001"}, "")
	require.NoError(t, err)
	assert.Equal(t, "agent-001", result)
}

func TestResolveEntityArg_NameFlagOnly(t *testing.T) {
	result, err := resolveEntityArg([]string{}, "mqtt-sensor")
	require.NoError(t, err)
	assert.Equal(t, "mqtt-sensor", result)
}

func TestResolveEntityArg_BothError(t *testing.T) {
	_, err := resolveEntityArg([]string{"agent-001"}, "mqtt-sensor")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "동시에 사용할 수 없습니다")
}

func TestResolveEntityArg_NeitherError(t *testing.T) {
	_, err := resolveEntityArg([]string{}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "대상을 지정해주세요")
}

// --- filterByName 테스트 ---

func TestFilterByName_SubstringMatch(t *testing.T) {
	items := []map[string]any{
		{"name": "mqtt-sensor", "type": "mqtt"},
		{"name": "http-receiver", "type": "http"},
		{"name": "mqtt-publisher", "type": "mqtt"},
	}

	result := filterByName(items, "mqtt", "name")
	assert.Len(t, result, 2, "mqtt 를 포함하는 항목이 2개여야 합니다")
	assert.Equal(t, "mqtt-sensor", result[0]["name"])
	assert.Equal(t, "mqtt-publisher", result[1]["name"])
}

func TestFilterByName_CaseInsensitive(t *testing.T) {
	items := []map[string]any{
		{"name": "MQTT-Sensor"},
		{"name": "http-receiver"},
	}

	result := filterByName(items, "mqtt", "name")
	assert.Len(t, result, 1)
	assert.Equal(t, "MQTT-Sensor", result[0]["name"])
}

func TestFilterByName_EmptyFilter(t *testing.T) {
	items := []map[string]any{
		{"name": "a"},
		{"name": "b"},
	}

	result := filterByName(items, "", "name")
	assert.Len(t, result, 2, "빈 필터는 전체를 반환해야 합니다")
}

func TestFilterByName_NoMatch(t *testing.T) {
	items := []map[string]any{
		{"name": "mqtt-sensor"},
	}

	result := filterByName(items, "http", "name")
	assert.Len(t, result, 0)
}

// --- isUUID 테스트 ---

func TestIsUUID(t *testing.T) {
	assert.True(t, isUUID("f47ac10b-58cc-4372-a567-0e02b2c3d479"), "유효한 UUID")
	assert.True(t, isUUID("aaaa1111-2222-3333-4444-555566667777"), "유효한 UUID")
	assert.True(t, isUUID("AAAA1111-2222-3333-4444-555566667777"), "대문자 UUID")
	assert.False(t, isUUID("agent-mqtt-sensor-1771853399404"), "타임스탬프 기반 ID")
	assert.False(t, isUUID("mqtt-sensor"), "일반 이름")
	assert.False(t, isUUID(""), "빈 문자열")
	assert.False(t, isUUID("not-a-uuid"), "형식 불일치")
}

// --- resolveAgentID 테스트 ---

func TestResolveAgentID_UUIDPassthrough(t *testing.T) {
	// UUID 형식이면 API 호출 없이 바로 반환
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("UUID 패스스루 시 서버 요청이 없어야 합니다")
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	id, err := resolveAgentID(client, "aaaa1111-2222-3333-4444-555566667777")
	require.NoError(t, err)
	assert.Equal(t, "aaaa1111-2222-3333-4444-555566667777", id,
		"UUID 는 API 호출 없이 그대로 반환되어야 합니다")
}

func TestResolveAgentID_NameMatch(t *testing.T) {
	agents := []map[string]any{
		{"id": "aaaa1111-2222-3333-4444-555566667777", "name": "mqtt-sensor"},
		{"id": "bbbb1111-2222-3333-4444-555566667777", "name": "http-receiver"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"success": true, "data": agents}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	id, err := resolveAgentID(client, "mqtt-sensor")
	require.NoError(t, err)
	assert.Equal(t, "aaaa1111-2222-3333-4444-555566667777", id,
		"이름으로 매칭된 UUID 가 반환되어야 합니다")
}

func TestResolveAgentID_NoMatch(t *testing.T) {
	agents := []map[string]any{
		{"id": "aaaa1111-2222-3333-4444-555566667777", "name": "mqtt-sensor"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"success": true, "data": agents}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	id, err := resolveAgentID(client, "nonexistent")
	require.NoError(t, err)
	assert.Equal(t, "nonexistent", id, "매칭 없으면 원본이 반환되어야 합니다")
}

func TestResolveAgentID_Duplicate(t *testing.T) {
	agents := []map[string]any{
		{"id": "aaaa1111-2222-3333-4444-555566667777", "name": "dup-name"},
		{"id": "bbbb1111-2222-3333-4444-555566667777", "name": "dup-name"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{"success": true, "data": agents}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	_, err := resolveAgentID(client, "dup-name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "동일한 이름의 에이전트가 2개 있습니다")
}

func TestResolveAgentID_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	id, err := resolveAgentID(client, "some-name")
	require.NoError(t, err)
	assert.Equal(t, "some-name", id, "서버 오류 시 원본이 반환되어야 합니다")
}
