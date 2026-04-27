package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- 테스트 헬퍼 ---

// setupPluginTest 는 플러그인 커맨드 테스트를 위한 공통 설정을 수행한다.
// httptest 서버, 커맨드 루트, 출력 버퍼를 반환한다.
func setupPluginTest(t *testing.T, handler http.Handler) (*httptest.Server, *cobra.Command, *bytes.Buffer) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// 클라이언트를 mock 서버로 직접 생성
	client := NewClient(srv.URL, "test-token", 5*time.Second, false)

	// 루트 커맨드 생성 (format 플래그 포함)
	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")

	// confirmFn 은 기본적으로 false 반환 (삭제 거부)
	confirmFn := func(prompt string, reader io.Reader) bool {
		return false
	}

	// 플러그인 커맨드 등록
	rootCmd.AddCommand(newPluginCmd(&client, confirmFn))

	// 출력 버퍼 설정
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return srv, rootCmd, &buf
}

// setupPluginTestWithConfirm 은 확인 함수를 커스텀으로 설정할 수 있는 테스트 헬퍼이다.
func setupPluginTestWithConfirm(t *testing.T, handler http.Handler, confirmFn func(string, io.Reader) bool) (*httptest.Server, *cobra.Command, *bytes.Buffer) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-token", 5*time.Second, false)

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")

	rootCmd.AddCommand(newPluginCmd(&client, confirmFn))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return srv, rootCmd, &buf
}

// --- plugin list 테스트 ---

// TestPluginList - 플러그인 목록 조회 및 테이블 출력 검증
func TestPluginList(t *testing.T) {
	plugins := []map[string]any{
		{
			"name":    "slack-notifier",
			"version": "1.2.0",
			"type":    "Go",
			"status":  "active",
			"nodes":   float64(3),
		},
		{
			"name":    "csv-parser",
			"version": "0.9.1",
			"type":    "WASM",
			"status":  "inactive",
			"nodes":   float64(1),
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/plugins", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    plugins,
		})
	})

	_, rootCmd, buf := setupPluginTest(t, handler)
	rootCmd.SetArgs([]string{"plugin", "list"})

	err := rootCmd.Execute()
	require.NoError(t, err, "plugin list 실행 에러가 없어야 합니다")

	output := buf.String()
	// 테이블 헤더 검증
	assert.Contains(t, output, "NAME", "테이블에 NAME 헤더가 있어야 합니다")
	assert.Contains(t, output, "VERSION", "테이블에 VERSION 헤더가 있어야 합니다")
	assert.Contains(t, output, "TYPE", "테이블에 TYPE 헤더가 있어야 합니다")
	assert.Contains(t, output, "STATUS", "테이블에 STATUS 헤더가 있어야 합니다")
	assert.Contains(t, output, "NODES", "테이블에 NODES 헤더가 있어야 합니다")
	// 데이터 검증
	assert.Contains(t, output, "slack-notifier", "출력에 slack-notifier 가 포함되어야 합니다")
	assert.Contains(t, output, "csv-parser", "출력에 csv-parser 가 포함되어야 합니다")
	assert.Contains(t, output, "active", "출력에 active 상태가 포함되어야 합니다")
	assert.Contains(t, output, "WASM", "출력에 WASM 타입이 포함되어야 합니다")
}

// TestPluginList_JSONFormat - JSON 출력 형식 검증
func TestPluginList_JSONFormat(t *testing.T) {
	plugins := []map[string]any{
		{"name": "slack-notifier", "version": "1.2.0", "type": "Go", "status": "active", "nodes": float64(3)},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    plugins,
		})
	})

	_, rootCmd, buf := setupPluginTest(t, handler)
	rootCmd.SetArgs([]string{"--format", "json", "plugin", "list"})

	err := rootCmd.Execute()
	require.NoError(t, err, "plugin list --format json 실행 에러가 없어야 합니다")

	output := buf.String()
	// JSON 파싱 가능 여부 검증
	var result []map[string]any
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Len(t, result, 1, "JSON 배열에 1개 항목이 있어야 합니다")
}

// TestPluginList_Empty - 빈 플러그인 목록 출력 검증
func TestPluginList_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    []map[string]any{},
		})
	})

	_, rootCmd, buf := setupPluginTest(t, handler)
	rootCmd.SetArgs([]string{"plugin", "list"})

	err := rootCmd.Execute()
	require.NoError(t, err, "빈 목록 조회 에러가 없어야 합니다")

	output := buf.String()
	// 헤더는 있어야 함
	assert.Contains(t, output, "NAME", "빈 목록에도 테이블 헤더가 있어야 합니다")
}

// TestPluginList_APIError - API 에러가 사용자에게 올바르게 전파되는지 검증
func TestPluginList_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error": map[string]any{
				"code":    "INTERNAL_ERROR",
				"message": "내부 서버 오류",
			},
		})
	})

	_, rootCmd, _ := setupPluginTest(t, handler)
	rootCmd.SetArgs([]string{"plugin", "list"})

	err := rootCmd.Execute()
	require.Error(t, err, "API 에러가 전파되어야 합니다")
}

// --- plugin install 테스트 ---

// TestPluginInstall_ByName - 이름으로 플러그인 설치 검증
func TestPluginInstall_ByName(t *testing.T) {
	var receivedBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/plugins", r.URL.Path)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &receivedBody)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"name":    "slack-notifier",
				"version": "1.2.0",
				"status":  "installed",
			},
		})
	})

	_, rootCmd, buf := setupPluginTest(t, handler)
	rootCmd.SetArgs([]string{"plugin", "install", "slack-notifier"})

	err := rootCmd.Execute()
	require.NoError(t, err, "plugin install 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "slack-notifier", "출력에 플러그인 이름이 포함되어야 합니다")

	// 요청 바디에 name 이 포함되어야 함
	assert.Equal(t, "slack-notifier", receivedBody["name"],
		"요청 바디에 플러그인 이름이 포함되어야 합니다")
}

// TestPluginInstall_ByPath - 경로로 플러그인 설치 검증
func TestPluginInstall_ByPath(t *testing.T) {
	var receivedBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/plugins", r.URL.Path)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &receivedBody)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"name":    "my-plugin",
				"version": "0.1.0",
				"status":  "installed",
			},
		})
	})

	_, rootCmd, buf := setupPluginTest(t, handler)
	rootCmd.SetArgs([]string{"plugin", "install", "/path/to/my-plugin.wasm"})

	err := rootCmd.Execute()
	require.NoError(t, err, "plugin install (path) 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "my-plugin", "출력에 플러그인 이름이 포함되어야 합니다")

	// 경로인 경우 path 필드로 전송
	assert.Equal(t, "/path/to/my-plugin.wasm", receivedBody["path"],
		"요청 바디에 플러그인 경로가 포함되어야 합니다")
}

// TestPluginInstall_MissingArg - 인자 누락 시 에러 검증
func TestPluginInstall_MissingArg(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("인자 누락 시 서버에 요청하면 안됩니다")
	})

	_, rootCmd, _ := setupPluginTest(t, handler)
	rootCmd.SetArgs([]string{"plugin", "install"})

	err := rootCmd.Execute()
	require.Error(t, err, "인자 없이 plugin install 을 실행하면 에러가 발생해야 합니다")
}

// TestPluginInstall_JSONFormat - 설치 결과 JSON 출력 검증
func TestPluginInstall_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"name":    "slack-notifier",
				"version": "1.2.0",
				"status":  "installed",
			},
		})
	})

	_, rootCmd, buf := setupPluginTest(t, handler)
	rootCmd.SetArgs([]string{"--format", "json", "plugin", "install", "slack-notifier"})

	err := rootCmd.Execute()
	require.NoError(t, err, "plugin install --format json 실행 에러가 없어야 합니다")

	output := buf.String()
	var result map[string]any
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, "slack-notifier", result["name"])
}

// --- plugin remove 테스트 ---

// TestPluginRemove_WithConfirmation - 확인 프롬프트 Y 응답 시 삭제 검증
func TestPluginRemove_WithConfirmation(t *testing.T) {
	var deleteCalled bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v1/plugins/slack-notifier", r.URL.Path)
		deleteCalled = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    map[string]any{"deleted": true},
		})
	})

	// confirmFn 이 true 를 반환
	confirmFn := func(prompt string, reader io.Reader) bool {
		return true
	}

	_, rootCmd, buf := setupPluginTestWithConfirm(t, handler, confirmFn)
	rootCmd.SetArgs([]string{"plugin", "remove", "slack-notifier"})

	err := rootCmd.Execute()
	require.NoError(t, err, "plugin remove 실행 에러가 없어야 합니다")

	assert.True(t, deleteCalled, "DELETE 요청이 서버에 전송되어야 합니다")
	output := buf.String()
	assert.Contains(t, output, "삭제", "출력에 삭제 확인 메시지가 포함되어야 합니다")
}

// TestPluginRemove_Cancelled - 확인 프롬프트 N 응답 시 삭제 취소 검증
func TestPluginRemove_Cancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("삭제 취소 시 서버에 요청하지 않아야 합니다")
	})

	// confirmFn 이 false 를 반환 (삭제 거부)
	confirmFn := func(prompt string, reader io.Reader) bool {
		return false
	}

	_, rootCmd, buf := setupPluginTestWithConfirm(t, handler, confirmFn)
	rootCmd.SetArgs([]string{"plugin", "remove", "slack-notifier"})

	err := rootCmd.Execute()
	require.NoError(t, err, "삭제 취소는 에러가 아닙니다")

	output := buf.String()
	assert.Contains(t, output, "취소", "출력에 취소 메시지가 포함되어야 합니다")
}

// TestPluginRemove_WithYesFlag - --yes 플래그로 확인 생략 검증
func TestPluginRemove_WithYesFlag(t *testing.T) {
	var deleteCalled bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v1/plugins/slack-notifier", r.URL.Path)
		deleteCalled = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    map[string]any{"deleted": true},
		})
	})

	// confirmFn 은 호출되면 안됨 (--yes 플래그로 건너뜀)
	confirmFn := func(prompt string, reader io.Reader) bool {
		t.Fatal("--yes 플래그가 있으면 confirmFn 이 호출되면 안됩니다")
		return false
	}

	_, rootCmd, _ := setupPluginTestWithConfirm(t, handler, confirmFn)
	rootCmd.SetArgs([]string{"plugin", "remove", "--yes", "slack-notifier"})

	err := rootCmd.Execute()
	require.NoError(t, err, "plugin remove --yes 실행 에러가 없어야 합니다")
	assert.True(t, deleteCalled, "--yes 시 확인 없이 삭제 요청을 보내야 합니다")
}

// TestPluginRemove_MissingArg - 이름 인자 누락 시 에러 검증
func TestPluginRemove_MissingArg(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("인자 누락 시 서버에 요청하면 안됩니다")
	})

	_, rootCmd, _ := setupPluginTest(t, handler)
	rootCmd.SetArgs([]string{"plugin", "remove"})

	err := rootCmd.Execute()
	require.Error(t, err, "이름 없이 plugin remove 를 실행하면 에러가 발생해야 합니다")
}

// TestPluginRemove_WithWarning - 사용 중인 노드가 있을 때 경고 메시지 검증
func TestPluginRemove_WithWarning(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"deleted": true,
				"warning": "이 플러그인의 노드가 2개 플로우에서 사용 중입니다",
			},
		})
	})

	confirmFn := func(prompt string, reader io.Reader) bool {
		return true
	}

	_, rootCmd, buf := setupPluginTestWithConfirm(t, handler, confirmFn)
	rootCmd.SetArgs([]string{"plugin", "remove", "slack-notifier"})

	err := rootCmd.Execute()
	require.NoError(t, err, "경고가 포함된 plugin remove 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "경고", "출력에 경고 메시지가 포함되어야 합니다")
}

// --- plugin update 테스트 ---

// TestPluginUpdate - 플러그인 업데이트 검증
func TestPluginUpdate(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/v1/plugins/slack-notifier", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"name":    "slack-notifier",
				"version": "1.3.0",
				"status":  "updated",
			},
		})
	})

	_, rootCmd, buf := setupPluginTest(t, handler)
	rootCmd.SetArgs([]string{"plugin", "update", "slack-notifier"})

	err := rootCmd.Execute()
	require.NoError(t, err, "plugin update 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "slack-notifier", "출력에 플러그인 이름이 포함되어야 합니다")
}

// TestPluginUpdate_JSONFormat - 업데이트 결과 JSON 출력 검증
func TestPluginUpdate_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/v1/plugins/slack-notifier", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"name":    "slack-notifier",
				"version": "1.3.0",
				"status":  "updated",
			},
		})
	})

	_, rootCmd, buf := setupPluginTest(t, handler)
	rootCmd.SetArgs([]string{"--format", "json", "plugin", "update", "slack-notifier"})

	err := rootCmd.Execute()
	require.NoError(t, err, "plugin update --format json 실행 에러가 없어야 합니다")

	output := buf.String()
	var result map[string]any
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, "slack-notifier", result["name"])
	assert.Equal(t, "1.3.0", result["version"])
}

// TestPluginUpdate_MissingArg - 이름 인자 누락 시 에러 검증
func TestPluginUpdate_MissingArg(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("인자 누락 시 서버에 요청하면 안됩니다")
	})

	_, rootCmd, _ := setupPluginTest(t, handler)
	rootCmd.SetArgs([]string{"plugin", "update"})

	err := rootCmd.Execute()
	require.Error(t, err, "이름 없이 plugin update 를 실행하면 에러가 발생해야 합니다")
}

// --- 서브커맨드 등록 검증 ---

// TestPluginSubcommands - 4개 서브커맨드 등록 여부 검증
func TestPluginSubcommands(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)
	confirmFn := func(prompt string, reader io.Reader) bool { return false }

	pluginCmd := newPluginCmd(&client, confirmFn)

	expectedSubs := []string{"list", "install", "remove", "update"}

	subs := make(map[string]bool)
	for _, sub := range pluginCmd.Commands() {
		parts := strings.Fields(sub.Use)
		if len(parts) > 0 {
			subs[parts[0]] = true
		}
	}

	for _, expected := range expectedSubs {
		assert.True(t, subs[expected],
			"plugin 에 '%s' 서브커맨드가 등록되어 있어야 합니다", expected)
	}
}

// --- 함수 시그니처 검증 ---

// TestNewPluginCmd_Signature - newPluginCmd 함수 시그니처 검증
func TestNewPluginCmd_Signature(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)
	confirmFn := func(prompt string, reader io.Reader) bool { return false }

	pluginCmd := newPluginCmd(&client, confirmFn)
	require.NotNil(t, pluginCmd, "newPluginCmd 가 nil 을 반환하면 안됩니다")
	assert.Equal(t, "plugin", pluginCmd.Use, "plugin 커맨드의 Use 가 'plugin' 이어야 합니다")
}

// --- 출력 형식 지원 전체 검증 ---

// TestPluginList_AllFormats - list 의 4가지 출력 형식 검증
func TestPluginList_AllFormats(t *testing.T) {
	plugins := []map[string]any{
		{"name": "test-plugin", "version": "1.0.0", "type": "Go", "status": "active", "nodes": float64(1)},
	}

	formats := []string{"json", "yaml", "table", "text"}

	for _, format := range formats {
		t.Run(fmt.Sprintf("format_%s", format), func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{
					"success": true,
					"data":    plugins,
				})
			})

			_, rootCmd, buf := setupPluginTest(t, handler)
			rootCmd.SetArgs([]string{"--format", format, "plugin", "list"})

			err := rootCmd.Execute()
			require.NoError(t, err,
				"plugin list --format %s 실행 에러가 없어야 합니다", format)

			output := buf.String()
			assert.NotEmpty(t, output,
				"format=%s 에서 출력이 비어있으면 안됩니다", format)
		})
	}
}

// --- plugin install 경로 판별 테스트 ---

// TestPluginInstall_PathDetection - / 또는 . 으로 시작하면 path, 아니면 name 으로 전송
func TestPluginInstall_PathDetection(t *testing.T) {
	tests := []struct {
		name       string
		arg        string
		expectKey  string
		expectVal  string
	}{
		{
			name:      "절대 경로",
			arg:       "/usr/local/plugins/my-plugin.so",
			expectKey: "path",
			expectVal: "/usr/local/plugins/my-plugin.so",
		},
		{
			name:      "상대 경로",
			arg:       "./plugins/my-plugin.wasm",
			expectKey: "path",
			expectVal: "./plugins/my-plugin.wasm",
		},
		{
			name:      "플러그인 이름",
			arg:       "slack-notifier",
			expectKey: "name",
			expectVal: "slack-notifier",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedBody map[string]any

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				json.Unmarshal(body, &receivedBody)

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{
					"success": true,
					"data":    map[string]any{"name": "test", "status": "installed"},
				})
			})

			_, rootCmd, _ := setupPluginTest(t, handler)
			rootCmd.SetArgs([]string{"plugin", "install", tt.arg})

			err := rootCmd.Execute()
			require.NoError(t, err)

			assert.Equal(t, tt.expectVal, receivedBody[tt.expectKey],
				"%s 인자는 %s 키로 전송되어야 합니다", tt.arg, tt.expectKey)
		})
	}
}
