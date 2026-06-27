package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- 테스트 헬퍼 ---

// settingsEnvelope 은 settings API 의 성공 응답 엔벨로프(`{success, data:{key, value}}`)를
// 생성하는 헬퍼이다. value 는 서버가 저장한 불투명 JSON 원본을 그대로 담는다.
func settingsEnvelope(key string, rawValue string) []byte {
	resp := map[string]any{
		"success": true,
		"data": map[string]any{
			"key":   key,
			"value": json.RawMessage(rawValue),
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

// setupSettingsTest 는 mock 서버와 settings 커맨드를 세팅하는 헬퍼이다.
func setupSettingsTest(t *testing.T, handler http.HandlerFunc) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()

	server := httptest.NewServer(handler)

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	clientPtr := &client

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.AddCommand(newSettingsCmd(clientPtr))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return &buf, rootCmd, server.Close
}

// --- settings get 테스트 ---

// TestSettingsGet_PathAndMethod - get 의 경로/메서드 및 value 출력 검증
func TestSettingsGet_PathAndMethod(t *testing.T) {
	var gotPath, gotMethod string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.Write(settingsEnvelope("device-list-columns", `{"columns":["id","name"]}`))
	})

	buf, cmd, cleanup := setupSettingsTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"settings", "get", "device-list-columns", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "settings get 실행 에러가 없어야 합니다")

	// 경로/메서드 검증
	assert.Equal(t, "/api/v1/settings/device-list-columns", gotPath, "GET 경로가 올바라야 합니다")
	assert.Equal(t, http.MethodGet, gotMethod, "메서드는 GET 이어야 합니다")

	// value(JSON) 가 그대로 파싱되어 출력되는지 검증
	var decoded map[string]any
	err = json.Unmarshal(buf.Bytes(), &decoded)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	cols, ok := decoded["columns"].([]any)
	require.True(t, ok, "columns 필드가 있어야 합니다")
	assert.Len(t, cols, 2, "columns 에 2개 항목이 있어야 합니다")
}

// TestSettingsGet_StringValue - 문자열 value 조회 시 따옴표 없이 출력되는지 검증
func TestSettingsGet_StringValue(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 서버에 저장된 value 가 JSON 문자열("hello")인 경우
		w.Write(settingsEnvelope("greeting", `"hello"`))
	})

	buf, cmd, cleanup := setupSettingsTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"settings", "get", "greeting", "--format", "text"})
	err := cmd.Execute()
	require.NoError(t, err, "settings get 실행 에러가 없어야 합니다")

	// text 형식에서는 디코딩된 문자열 값(hello)이 출력되어야 한다.
	assert.Contains(t, buf.String(), "hello", "출력에 hello 가 포함되어야 합니다")
}

// TestSettingsGet_NotFound - 404 응답 시 에러 반환 검증
func TestSettingsGet_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"success":false,"error":{"code":"NOT_FOUND","message":"setting \"missing\" not found"}}`))
	})

	_, cmd, cleanup := setupSettingsTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"settings", "get", "missing"})
	err := cmd.Execute()
	require.Error(t, err, "존재하지 않는 키는 에러를 반환해야 합니다")
}

// TestSettingsGet_MissingArg - key 인자 누락 시 에러 검증
func TestSettingsGet_MissingArg(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("핸들러가 호출되면 안됩니다 (인자 검증 단계에서 실패해야 함)")
	})

	_, cmd, cleanup := setupSettingsTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"settings", "get"})
	err := cmd.Execute()
	require.Error(t, err, "key 인자가 없으면 에러를 반환해야 합니다")
}

// --- settings set 테스트 ---

// TestSettingsSet_PathMethodAndStringBody - set 의 경로/메서드 및 문자열 value 바디 검증
//
// 기본 모드에서는 value 인자가 JSON 문자열로 인코딩되어야 한다(hello → "hello").
func TestSettingsSet_PathMethodAndStringBody(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody []byte

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write(settingsEnvelope("greeting", string(gotBody)))
	})

	buf, cmd, cleanup := setupSettingsTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"settings", "set", "greeting", "hello"})
	err := cmd.Execute()
	require.NoError(t, err, "settings set 실행 에러가 없어야 합니다")

	// 경로/메서드 검증
	assert.Equal(t, "/api/v1/settings/greeting", gotPath, "PUT 경로가 올바라야 합니다")
	assert.Equal(t, http.MethodPut, gotMethod, "메서드는 PUT 이어야 합니다")

	// 바디 구조 검증: 문자열은 JSON 문자열로 인코딩되어야 한다 ("hello")
	assert.JSONEq(t, `"hello"`, string(gotBody), "바디는 JSON 문자열 \"hello\" 여야 합니다")

	// 저장 완료 메시지 검증 (기본 table 형식)
	assert.Contains(t, buf.String(), "greeting", "출력에 키 이름이 포함되어야 합니다")
}

// TestSettingsSet_JSONFlag - --json 플래그 시 원본 JSON 이 그대로 전달되는지 검증
func TestSettingsSet_JSONFlag(t *testing.T) {
	var gotBody []byte

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write(settingsEnvelope("device-list-columns", string(gotBody)))
	})

	_, cmd, cleanup := setupSettingsTest(t, handler)
	defer cleanup()

	jsonValue := `{"columns":["id","name"]}`
	cmd.SetArgs([]string{"settings", "set", "device-list-columns", jsonValue, "--json"})
	err := cmd.Execute()
	require.NoError(t, err, "settings set --json 실행 에러가 없어야 합니다")

	// 바디 구조 검증: 원본 JSON 객체가 그대로 전달되어야 한다(이중 인코딩 없음)
	assert.JSONEq(t, jsonValue, string(gotBody), "바디는 원본 JSON 객체 그대로여야 합니다")
}

// TestSettingsSet_JSONFlag_Invalid - --json 값이 잘못된 JSON 이면 에러 검증
func TestSettingsSet_JSONFlag_Invalid(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("핸들러가 호출되면 안됩니다 (클라이언트 측 JSON 검증에서 실패해야 함)")
	})

	_, cmd, cleanup := setupSettingsTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"settings", "set", "k", "{invalid", "--json"})
	err := cmd.Execute()
	require.Error(t, err, "잘못된 JSON 은 에러를 반환해야 합니다")
}

// TestSettingsSet_JSONFormatOutput - set 후 --format json 출력 검증
func TestSettingsSet_JSONFormatOutput(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write(settingsEnvelope("device-list-columns", string(body)))
	})

	buf, cmd, cleanup := setupSettingsTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"settings", "set", "device-list-columns", `[1,2,3]`, "--json", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "settings set --json --format json 실행 에러가 없어야 합니다")

	// 저장된 value(배열)가 JSON 으로 출력되는지 검증
	var decoded []any
	err = json.Unmarshal(buf.Bytes(), &decoded)
	require.NoError(t, err, "출력이 유효한 JSON 배열이어야 합니다")
	assert.Len(t, decoded, 3, "배열에 3개 항목이 있어야 합니다")
}

// TestSettingsSet_MissingArgs - value 인자 누락 시 에러 검증
func TestSettingsSet_MissingArgs(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("핸들러가 호출되면 안됩니다 (인자 검증 단계에서 실패해야 함)")
	})

	_, cmd, cleanup := setupSettingsTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"settings", "set", "only-key"})
	err := cmd.Execute()
	require.Error(t, err, "value 인자가 없으면 에러를 반환해야 합니다")
}

// TestSettingsSet_APIError - 서버 에러(413 등) 응답 시 에러 전파 검증
func TestSettingsSet_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"success":false,"error":{"code":"BAD_REQUEST","message":"value must be valid JSON"}}`))
	})

	_, cmd, cleanup := setupSettingsTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"settings", "set", "k", "v"})
	err := cmd.Execute()
	require.Error(t, err, "서버 에러는 전파되어야 합니다")
}

// --- 구조 검증 테스트 ---

// TestSettingsSubcommands - settings 커맨드에 get/set 서브커맨드가 등록되어 있는지 검증
func TestSettingsSubcommands(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)
	clientPtr := &client

	settingsCmd := newSettingsCmd(clientPtr)

	wantSub := map[string]bool{"get": false, "set": false}
	for _, c := range settingsCmd.Commands() {
		name := c.Name()
		if _, ok := wantSub[name]; ok {
			wantSub[name] = true
		}
	}

	for name, found := range wantSub {
		assert.True(t, found, "서브커맨드 '%s' 가 등록되어 있어야 합니다", name)
	}
}

// TestSettingsHelp_MentionsServerGlobalAndConfigDifference - 도움말에 "서버 전역" 및
// config(로컬)와의 차이가 명시되어 있는지 검증한다.
func TestSettingsHelp_MentionsServerGlobalAndConfigDifference(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)
	clientPtr := &client

	settingsCmd := newSettingsCmd(clientPtr)

	help := settingsCmd.Short + "\n" + settingsCmd.Long
	assert.Contains(t, help, "전역", "도움말에 '전역' 설정임이 명시되어야 합니다")
	assert.Contains(t, help, "config", "도움말에 'config'(로컬)와의 차이가 명시되어야 합니다")
}

// TestNewSettingsCmd_Signature - newSettingsCmd 함수 시그니처 검증.
// flow.go 패턴(client **Client)을 따르되, settings 는 확인 프롬프트가 필요 없으므로
// confirmFn 을 받지 않는다.
func TestNewSettingsCmd_Signature(t *testing.T) {
	var _ func(**Client) *cobra.Command = newSettingsCmd
}
