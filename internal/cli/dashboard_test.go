package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- 테스트 헬퍼 ---

// dashboardEnvelope 은 대시보드 API 의 성공 응답 엔벨로프를 생성한다.
func dashboardEnvelope(data any) []byte {
	resp := map[string]any{"success": true, "data": data}
	b, _ := json.Marshal(resp)
	return b
}

// setupDashboardTest 는 mock 서버와 dashboard 커맨드 루트를 세팅하는 헬퍼이다.
func setupDashboardTest(t *testing.T, handler http.HandlerFunc) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()

	server := httptest.NewServer(handler)

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	clientPtr := &client

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.AddCommand(newDashboardCmd(clientPtr))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return &buf, rootCmd, server.Close
}

// sampleSnapshot 은 테스트용 DashboardSnapshot 응답 데이터를 생성한다.
func sampleSnapshot(scope string, owner *string) map[string]any {
	return map[string]any{
		"scope":     scope,
		"owner":     owner,
		"version":   float64(3),
		"updatedAt": float64(1700000000000),
		"payload":   map[string]any{"widgets": []any{"cpu", "mem"}},
	}
}

// --- dashboard shared get ---

func TestDashboardSharedGet(t *testing.T) {
	var gotPath, gotMethod string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.Write(dashboardEnvelope(sampleSnapshot("global", nil)))
	})

	buf, cmd, cleanup := setupDashboardTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"dashboard", "shared", "get"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Equal(t, "/api/v1/dashboards/shared", gotPath)

	out := buf.String()
	assert.Contains(t, out, "Scope")
	assert.Contains(t, out, "global")
	assert.Contains(t, out, "Version")
	// updatedAt 은 과학표기 없이 정수 문자열로 출력되어야 한다.
	assert.Contains(t, out, "1700000000000")
	assert.NotContains(t, out, "1.7e")
	assert.Contains(t, out, "Payload")
}

func TestDashboardSharedGet_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(dashboardEnvelope(sampleSnapshot("global", nil)))
	})

	buf, cmd, cleanup := setupDashboardTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "dashboard", "shared", "get"})
	require.NoError(t, cmd.Execute())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, "global", parsed["scope"])
	assert.Nil(t, parsed["owner"])
}

// --- dashboard mine get ---

func TestDashboardMineGet(t *testing.T) {
	var gotPath string
	owner := "alice"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write(dashboardEnvelope(sampleSnapshot("user", &owner)))
	})

	buf, cmd, cleanup := setupDashboardTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"dashboard", "mine", "get"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "/api/v1/dashboards/mine", gotPath)
	out := buf.String()
	assert.Contains(t, out, "user")
	assert.Contains(t, out, "alice")
}

// --- dashboard shared set ---

func TestDashboardSharedSet_InlineJSON(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(dashboardEnvelope(sampleSnapshot("global", nil)))
	})

	buf, cmd, cleanup := setupDashboardTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"dashboard", "shared", "set",
		"--payload", `{"widgets":["cpu"]}`,
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "/api/v1/dashboards/shared", gotPath)

	payload, ok := gotBody["payload"].(map[string]any)
	require.True(t, ok, "본문에 payload 객체가 있어야 합니다")
	widgets, ok := payload["widgets"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"cpu"}, widgets)

	assert.Contains(t, buf.String(), "저장되었습니다")
}

func TestDashboardSharedSet_FromFile(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(dashboardEnvelope(sampleSnapshot("global", nil)))
	})

	_, cmd, cleanup := setupDashboardTest(t, handler)
	defer cleanup()

	// 임시 JSON 파일 작성 후 @file 로 전달.
	dir := t.TempDir()
	file := filepath.Join(dir, "dash.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"layout":"grid"}`), 0o600))

	cmd.SetArgs([]string{
		"dashboard", "shared", "set",
		"--payload", "@" + file,
	})
	require.NoError(t, cmd.Execute())

	payload, ok := gotBody["payload"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "grid", payload["layout"])
}

func TestDashboardSharedSet_InvalidJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("payload JSON 이 잘못되면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupDashboardTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"dashboard", "shared", "set", "--payload", `{invalid`})
	require.Error(t, cmd.Execute())
}

func TestDashboardSharedSet_MissingPayload(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("payload 미지정 시 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupDashboardTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"dashboard", "shared", "set"})
	require.Error(t, cmd.Execute(), "--payload 는 필수 플래그입니다")
}

func TestDashboardSharedSet_FileNotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("파일이 없으면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupDashboardTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"dashboard", "shared", "set", "--payload", "@/nonexistent/path.json"})
	require.Error(t, cmd.Execute())
}

// --- dashboard mine set ---

func TestDashboardMineSet(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	owner := "alice"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(dashboardEnvelope(sampleSnapshot("user", &owner)))
	})

	buf, cmd, cleanup := setupDashboardTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"dashboard", "mine", "set", "--payload", `{"theme":"dark"}`})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "/api/v1/dashboards/mine", gotPath)

	payload, ok := gotBody["payload"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "dark", payload["theme"])

	assert.Contains(t, buf.String(), "저장되었습니다")
}

func TestDashboardMineSet_JSONFormat(t *testing.T) {
	owner := "alice"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(dashboardEnvelope(sampleSnapshot("user", &owner)))
	})

	buf, cmd, cleanup := setupDashboardTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "dashboard", "mine", "set", "--payload", `{"theme":"dark"}`})
	require.NoError(t, cmd.Execute())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, "user", parsed["scope"])
}

// --- buildDashboardPutBody 단위 테스트 ---

func TestBuildDashboardPutBody(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantErr bool
	}{
		{"valid inline object", `{"a":1}`, false},
		{"valid inline array", `[1,2,3]`, false},
		{"empty object", `{}`, false},
		{"empty payload", ``, true},
		{"whitespace only", `   `, true},
		{"invalid json", `{bad`, true},
		{"empty file ref", `@`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := buildDashboardPutBody(tt.payload)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			_, ok := body["payload"]
			assert.True(t, ok, "본문에 payload 키가 있어야 합니다")
		})
	}
}

func TestBuildDashboardPutBody_FromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "p.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"k":"v"}`), 0o600))

	body, err := buildDashboardPutBody("@" + file)
	require.NoError(t, err)

	raw, ok := body["payload"].(json.RawMessage)
	require.True(t, ok)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw, &parsed))
	assert.Equal(t, "v", parsed["k"])
}
