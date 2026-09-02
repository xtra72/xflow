package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// --- 쓰기 커맨드 회귀 방지 ---

// TestDashboardCmd_NoWriteSubcommands 는 shared / mine 그룹에 쓰기 하위 커맨드가
// 등록되지 않았음을 고정한다.
//
// @SPEC:SPEC-DASHBOARD-004 (M4, spec.md §2.3)
// 서버가 PUT/DELETE /api/v1/dashboards/{shared,mine} 를 등록하지 않으므로(호출 시
// 404) 그 경로를 때리는 CLI 커맨드를 다시 추가하면 런타임에서만 깨진다. mock 서버를
// 쓰는 테스트는 그 사실을 잡아내지 못하므로, "커맨드가 없다" 를 직접 고정한다.
func TestDashboardCmd_NoWriteSubcommands(t *testing.T) {
	var client *Client
	dashboardCmd := newDashboardCmd(&client)

	for _, group := range []string{"shared", "mine"} {
		t.Run(group, func(t *testing.T) {
			var groupCmd *cobra.Command
			for _, c := range dashboardCmd.Commands() {
				if c.Name() == group {
					groupCmd = c
					break
				}
			}
			require.NotNil(t, groupCmd, "%s 그룹은 남아 있어야 한다", group)

			names := make([]string, 0, len(groupCmd.Commands()))
			for _, c := range groupCmd.Commands() {
				names = append(names, c.Name())
			}
			assert.Equal(t, []string{"get"}, names,
				"%s 그룹은 읽기 전용이어야 한다 (set 재추가 금지 — 서버 라우트가 없다)", group)
		})
	}
}

// TestDashboardCmd_OnlyTargetsReadRoutes 는 등록된 하위 커맨드가 실제로 GET 만
// 발생시키는지 확인한다.
func TestDashboardCmd_OnlyTargetsReadRoutes(t *testing.T) {
	var methods []string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(dashboardEnvelope(sampleSnapshot("global", nil)))
	})

	for _, group := range []string{"shared", "mine"} {
		_, cmd, cleanup := setupDashboardTest(t, handler)
		cmd.SetArgs([]string{"dashboard", group, "get"})
		require.NoError(t, cmd.Execute())
		cleanup()
	}

	assert.Equal(t, []string{http.MethodGet, http.MethodGet}, methods,
		"대시보드 CLI 는 읽기 요청만 발생시켜야 한다")
}
