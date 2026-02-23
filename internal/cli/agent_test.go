package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// withResolveSupport 는 기존 핸들러를 감싸서 resolveAgentID 의 이름 검색 호출을 처리한다.
// GET /api/v1/agents 가 호출되면 빈 목록을 반환하여 resolveAgentID 가 원본 ID 를 그대로 사용하게 한다.
func withResolveSupport(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agents" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []map[string]any{}})
			return
		}
		handler(w, r)
	}
}

// agentInfo 는 테스트용 에이전트 응답 구조체이다.
type agentInfo struct {
	ID        string `json:"id" yaml:"id"`
	Name      string `json:"name" yaml:"name"`
	Type      string `json:"type" yaml:"type"`
	Status    string `json:"status" yaml:"status"`
	Connected bool   `json:"connected" yaml:"connected"`
}

// setupAgentTest 는 에이전트 커맨드 테스트를 위한 공통 설정을 수행한다.
// httptest 서버, 커맨드 루트, 출력 버퍼를 반환한다.
func setupAgentTest(t *testing.T, handler http.Handler) (*httptest.Server, *cobra.Command, *bytes.Buffer) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// 클라이언트를 mock 서버로 직접 생성
	client := NewClient(srv.URL, "", 5*time.Second, false)

	// 루트 커맨드 생성 (format 플래그 포함)
	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")

	// confirmAction 을 기본적으로 false 반환하도록 설정
	confirmFn := func(prompt string, reader io.Reader) bool {
		return false
	}

	// 에이전트 커맨드 등록
	rootCmd.AddCommand(newAgentCmd(&client, confirmFn))

	// 출력 버퍼 설정
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return srv, rootCmd, &buf
}

// setupAgentTestWithConfirm 은 확인 함수를 커스텀으로 설정할 수 있는 테스트 헬퍼이다.
func setupAgentTestWithConfirm(t *testing.T, handler http.Handler, confirmFn func(string, io.Reader) bool) (*httptest.Server, *cobra.Command, *bytes.Buffer) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "", 5*time.Second, false)

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")

	rootCmd.AddCommand(newAgentCmd(&client, confirmFn))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return srv, rootCmd, &buf
}

// --- TestAgentList: 에이전트 목록 조회 ---

func TestAgentList(t *testing.T) {
	// mock 서버: GET /api/v1/agents 응답
	agents := []agentInfo{
		{ID: "agent-1", Name: "워커-A", Type: "worker", Status: "running", Connected: true},
		{ID: "agent-2", Name: "워커-B", Type: "scheduler", Status: "stopped", Connected: false},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/agents", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    agents,
		})
	})

	_, rootCmd, buf := setupAgentTest(t, handler)
	rootCmd.SetArgs([]string{"agent", "list"})

	err := rootCmd.Execute()
	require.NoError(t, err, "agent list 실행 에러가 없어야 합니다")

	output := buf.String()
	// 테이블 헤더 검증
	assert.Contains(t, output, "ID", "출력에 ID 헤더가 포함되어야 합니다")
	assert.Contains(t, output, "NAME", "출력에 NAME 헤더가 포함되어야 합니다")
	assert.Contains(t, output, "TYPE", "출력에 TYPE 헤더가 포함되어야 합니다")
	assert.Contains(t, output, "STATUS", "출력에 STATUS 헤더가 포함되어야 합니다")
	assert.Contains(t, output, "CONNECTED", "출력에 CONNECTED 헤더가 포함되어야 합니다")
	// 데이터 검증
	assert.Contains(t, output, "agent-1", "출력에 agent-1 이 포함되어야 합니다")
	assert.Contains(t, output, "agent-2", "출력에 agent-2 가 포함되어야 합니다")
	assert.Contains(t, output, "running", "출력에 running 상태가 포함되어야 합니다")
	assert.Contains(t, output, "stopped", "출력에 stopped 상태가 포함되어야 합니다")
}

// TestAgentList_JSONFormat - JSON 출력 형식 검증
func TestAgentList_JSONFormat(t *testing.T) {
	agents := []agentInfo{
		{ID: "agent-1", Name: "워커-A", Type: "worker", Status: "running", Connected: true},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    agents,
		})
	})

	_, rootCmd, buf := setupAgentTest(t, handler)
	rootCmd.SetArgs([]string{"--format", "json", "agent", "list"})

	err := rootCmd.Execute()
	require.NoError(t, err, "agent list --format json 실행 에러가 없어야 합니다")

	output := buf.String()
	// JSON 출력이 유효한지 검증
	var parsed []agentInfo
	err = json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err, "JSON 출력이 유효해야 합니다")
	assert.Len(t, parsed, 1, "에이전트가 1개여야 합니다")
	assert.Equal(t, "agent-1", parsed[0].ID)
}

// --- TestAgentGet: 에이전트 상세 조회 ---

func TestAgentGet(t *testing.T) {
	agent := agentInfo{
		ID: "agent-1", Name: "워커-A", Type: "worker", Status: "running", Connected: true,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/agents/agent-1", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    agent,
		})
	})

	_, rootCmd, buf := setupAgentTest(t, withResolveSupport(handler))
	// get 은 기본적으로 json 형식으로 출력
	rootCmd.SetArgs([]string{"--format", "json", "agent", "get", "agent-1"})

	err := rootCmd.Execute()
	require.NoError(t, err, "agent get 실행 에러가 없어야 합니다")

	output := buf.String()
	var parsed agentInfo
	err = json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err, "JSON 출력이 유효해야 합니다")
	assert.Equal(t, "agent-1", parsed.ID)
	assert.Equal(t, "running", parsed.Status)
}

// TestAgentGet_MissingID - ID 미지정 시 에러 검증
func TestAgentGet_MissingID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("ID 가 없으면 서버에 요청하면 안됩니다")
	})

	_, rootCmd, _ := setupAgentTest(t, handler)
	rootCmd.SetArgs([]string{"agent", "get"})

	err := rootCmd.Execute()
	require.Error(t, err, "ID 없이 agent get 을 실행하면 에러가 발생해야 합니다")
}

// --- TestAgentCreate: 파일 기반 에이전트 생성 ---

func TestAgentCreate(t *testing.T) {
	var receivedBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/agents", r.URL.Path)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &receivedBody)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":   "agent-new",
				"name": receivedBody["name"],
			},
		})
	})

	_, rootCmd, buf := setupAgentTest(t, handler)

	// 임시 JSON 파일 생성
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "agent.json")
	content := `{"name": "새-에이전트", "type": "worker"}`
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	rootCmd.SetArgs([]string{"--format", "json", "agent", "create", "-f", filePath})

	err = rootCmd.Execute()
	require.NoError(t, err, "agent create 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "agent-new", "생성된 에이전트 ID 가 출력에 포함되어야 합니다")

	// 전송된 본문 검증
	assert.Equal(t, "새-에이전트", receivedBody["name"],
		"요청 본문에 에이전트 이름이 포함되어야 합니다")
}

// TestAgentCreate_YAMLFile - YAML 파일 기반 에이전트 생성 검증
func TestAgentCreate_YAMLFile(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":   "agent-yaml",
				"name": "yaml-agent",
			},
		})
	})

	_, rootCmd, buf := setupAgentTest(t, handler)

	// 임시 YAML 파일 생성
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "agent.yaml")
	content := "name: yaml-에이전트\ntype: scheduler\n"
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	rootCmd.SetArgs([]string{"--format", "json", "agent", "create", "-f", filePath})

	err = rootCmd.Execute()
	require.NoError(t, err, "YAML 파일로 agent create 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "agent-yaml", "생성된 에이전트 ID 가 출력에 포함되어야 합니다")
}

// TestAgentCreate_FileNotFound - 존재하지 않는 파일 에러 검증
func TestAgentCreate_FileNotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("파일이 없으면 서버에 요청하면 안됩니다")
	})

	_, rootCmd, _ := setupAgentTest(t, handler)
	rootCmd.SetArgs([]string{"agent", "create", "-f", "/nonexistent/path/agent.json"})

	err := rootCmd.Execute()
	require.Error(t, err, "존재하지 않는 파일로 agent create 을 실행하면 에러가 발생해야 합니다")
	assert.Contains(t, err.Error(), "파일",
		"에러 메시지에 파일 관련 내용이 포함되어야 합니다")
}

// TestAgentCreate_NoFileFlag - -f 플래그 미지정 시 에러 검증
func TestAgentCreate_NoFileFlag(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("파일 플래그 없으면 서버에 요청하면 안됩니다")
	})

	_, rootCmd, _ := setupAgentTest(t, handler)
	rootCmd.SetArgs([]string{"agent", "create"})

	err := rootCmd.Execute()
	require.Error(t, err, "-f 플래그 없이 agent create 을 실행하면 에러가 발생해야 합니다")
}

// --- TestAgentLifecycle: start/stop/restart 테이블 드리븐 테스트 ---

func TestAgentLifecycle(t *testing.T) {
	tests := []struct {
		name       string
		subcommand string
		apiPath    string
	}{
		{
			name:       "agent start",
			subcommand: "start",
			apiPath:    "/api/v1/agents/agent-1/start",
		},
		{
			name:       "agent stop",
			subcommand: "stop",
			apiPath:    "/api/v1/agents/agent-1/stop",
		},
		{
			name:       "agent restart",
			subcommand: "restart",
			apiPath:    "/api/v1/agents/agent-1/restart",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method,
					"%s 는 POST 메서드를 사용해야 합니다", tt.subcommand)
				assert.Equal(t, tt.apiPath, r.URL.Path,
					"%s 의 API 경로가 올바라야 합니다", tt.subcommand)

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{
					"success": true,
					"data": map[string]any{
						"id":     "agent-1",
						"status": tt.subcommand + "ed",
					},
				})
			})

			_, rootCmd, buf := setupAgentTest(t, withResolveSupport(handler))
			rootCmd.SetArgs([]string{"agent", tt.subcommand, "agent-1"})

			err := rootCmd.Execute()
			require.NoError(t, err,
				"agent %s 실행 에러가 없어야 합니다", tt.subcommand)

			output := buf.String()
			assert.NotEmpty(t, output,
				"agent %s 의 출력이 비어있으면 안됩니다", tt.subcommand)
		})
	}
}

// TestAgentLifecycle_MissingID - ID 미지정 시 에러 검증
func TestAgentLifecycle_MissingID(t *testing.T) {
	subcommands := []string{"start", "stop", "restart"}

	for _, sub := range subcommands {
		t.Run(fmt.Sprintf("agent %s ID 미지정", sub), func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("ID 가 없으면 서버에 요청하면 안됩니다")
			})

			_, rootCmd, _ := setupAgentTest(t, handler)
			rootCmd.SetArgs([]string{"agent", sub})

			err := rootCmd.Execute()
			require.Error(t, err,
				"ID 없이 agent %s 을 실행하면 에러가 발생해야 합니다", sub)
		})
	}
}

// --- TestAgentDelete_WithConfirmation: 확인 프롬프트 테스트 ---

func TestAgentDelete_WithConfirmation(t *testing.T) {
	t.Run("사용자가 Y 입력 시 삭제 실행", func(t *testing.T) {
		var deleteCalled bool
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodDelete, r.Method)
			assert.Equal(t, "/api/v1/agents/agent-1", r.URL.Path)
			deleteCalled = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    nil,
			})
		})

		// confirmFn 이 true 를 반환
		confirmFn := func(prompt string, reader io.Reader) bool {
			return true
		}

		_, rootCmd, buf := setupAgentTestWithConfirm(t, withResolveSupport(handler), confirmFn)
		rootCmd.SetArgs([]string{"agent", "delete", "agent-1"})

		err := rootCmd.Execute()
		require.NoError(t, err, "확인 후 agent delete 실행 에러가 없어야 합니다")
		assert.True(t, deleteCalled, "DELETE 요청이 실행되어야 합니다")

		output := buf.String()
		assert.NotEmpty(t, output, "삭제 결과 메시지가 출력되어야 합니다")
	})

	t.Run("사용자가 N 입력 시 삭제 취소", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("사용자가 거부하면 서버에 요청하면 안됩니다")
		})

		// confirmFn 이 false 를 반환
		confirmFn := func(prompt string, reader io.Reader) bool {
			return false
		}

		_, rootCmd, buf := setupAgentTestWithConfirm(t, withResolveSupport(handler), confirmFn)
		rootCmd.SetArgs([]string{"agent", "delete", "agent-1"})

		err := rootCmd.Execute()
		require.NoError(t, err, "취소 시에도 에러가 없어야 합니다")

		output := buf.String()
		assert.Contains(t, output, "취소",
			"취소 메시지가 출력에 포함되어야 합니다")
	})
}

// --- TestAgentDelete_WithYesFlag: --yes 플래그로 확인 건너뛰기 ---

func TestAgentDelete_WithYesFlag(t *testing.T) {
	var deleteCalled bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v1/agents/agent-1", r.URL.Path)
		deleteCalled = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    nil,
		})
	})

	// confirmFn 은 호출되면 안됨 (--yes 플래그로 건너뜀)
	confirmFn := func(prompt string, reader io.Reader) bool {
		t.Fatal("--yes 플래그가 있으면 confirmFn 이 호출되면 안됩니다")
		return false
	}

	_, rootCmd, _ := setupAgentTestWithConfirm(t, withResolveSupport(handler), confirmFn)
	rootCmd.SetArgs([]string{"agent", "delete", "--yes", "agent-1"})

	err := rootCmd.Execute()
	require.NoError(t, err, "--yes 플래그로 agent delete 실행 에러가 없어야 합니다")
	assert.True(t, deleteCalled, "--yes 플래그 시 DELETE 요청이 실행되어야 합니다")
}

// TestAgentDelete_MissingID - 삭제 시 ID 미지정 에러 검증
func TestAgentDelete_MissingID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("ID 가 없으면 서버에 요청하면 안됩니다")
	})

	_, rootCmd, _ := setupAgentTest(t, handler)
	rootCmd.SetArgs([]string{"agent", "delete"})

	err := rootCmd.Execute()
	require.Error(t, err, "ID 없이 agent delete 을 실행하면 에러가 발생해야 합니다")
}

// --- TestAgentDelete_WithFlowWarning: 플로우 참조 경고 테스트 ---

func TestAgentDelete_WithFlowWarning(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"message": "에이전트가 삭제되었습니다",
					"warning": "이 에이전트를 참조하는 플로우가 2개 있습니다",
				},
			})
		}
	})

	confirmFn := func(prompt string, reader io.Reader) bool {
		// 확인 프롬프트에 경고 메시지가 포함되는지 검증은 하지 않음
		// (경고는 서버 응답에 포함)
		return true
	}

	_, rootCmd, buf := setupAgentTestWithConfirm(t, withResolveSupport(handler), confirmFn)
	rootCmd.SetArgs([]string{"agent", "delete", "--yes", "agent-1"})

	err := rootCmd.Execute()
	require.NoError(t, err, "경고가 포함된 agent delete 실행 에러가 없어야 합니다")

	output := buf.String()
	// 삭제 성공 메시지가 출력되어야 합니다
	assert.NotEmpty(t, output, "출력이 비어있으면 안됩니다")
}

// --- TestAgentCommand_HasSubcommands: 서브커맨드 등록 검증 ---

func TestAgentCommand_HasSubcommands(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)
	confirmFn := func(prompt string, reader io.Reader) bool { return false }
	agentCmd := newAgentCmd(&client, confirmFn)

	expectedSubs := []string{"list", "get", "create", "start", "stop", "restart", "delete", "export", "import"}

	subs := make(map[string]bool)
	for _, sub := range agentCmd.Commands() {
		subs[sub.Name()] = true
	}

	for _, expected := range expectedSubs {
		assert.True(t, subs[expected],
			"agent 커맨드에 '%s' 서브커맨드가 등록되어 있어야 합니다", expected)
	}
}

// --- 헬퍼: 인라인 confirmAction (테스트에서 stdin 을 주입하기 위함) ---

// TestAgentDelete_WithStdinConfirm - stdin 기반 confirmAction 통합 테스트
func TestAgentDelete_WithStdinConfirm(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    nil,
		})
	})

	// stdin 에서 "y" 를 읽는 confirmFn
	confirmFn := func(prompt string, reader io.Reader) bool {
		return confirmAction(prompt, strings.NewReader("y\n"))
	}

	_, rootCmd, _ := setupAgentTestWithConfirm(t, withResolveSupport(handler), confirmFn)
	rootCmd.SetArgs([]string{"agent", "delete", "agent-1"})

	err := rootCmd.Execute()
	require.NoError(t, err, "stdin y 입력으로 agent delete 실행 에러가 없어야 합니다")
}

// --- TestAgentExport: 에이전트 내보내기 테스트 ---

// TestAgentExport_JSON - 단일 에이전트를 JSON 파일로 내보내기
func TestAgentExport_JSON(t *testing.T) {
	// mock 서버: GET /api/v1/agents/agent-01 응답 (런타임 필드 포함)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/agents/agent-01", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":           "agent-01",
				"name":         "테스트-에이전트",
				"type":         "worker",
				"status":       "running",
				"connected":    true,
				"uptime":       "1h30m",
				"messages_in":  1000,
				"messages_out": 900,
				"error_count":  5,
				"config": map[string]any{
					"host": "localhost",
					"port": 8080,
				},
			},
		})
	})

	_, rootCmd, _ := setupAgentTest(t, withResolveSupport(handler))

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "agent.json")
	rootCmd.SetArgs([]string{"agent", "export", "agent-01", "-o", outputPath})

	err := rootCmd.Execute()
	require.NoError(t, err, "agent export JSON 실행 에러가 없어야 합니다")

	// 파일 존재 확인
	data, err := os.ReadFile(outputPath)
	require.NoError(t, err, "내보낸 파일을 읽을 수 있어야 합니다")

	// 유효한 JSON 인지 확인
	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err, "내보낸 파일이 유효한 JSON 이어야 합니다")

	// 에이전트 데이터 포함 확인
	assert.Equal(t, "테스트-에이전트", parsed["name"], "name 필드가 포함되어야 합니다")
	assert.Equal(t, "worker", parsed["type"], "type 필드가 포함되어야 합니다")
	assert.NotNil(t, parsed["config"], "config 필드가 포함되어야 합니다")

	// 런타임 필드 제거 확인
	assert.Nil(t, parsed["id"], "런타임 필드 id 가 제거되어야 합니다")
	assert.Nil(t, parsed["status"], "런타임 필드 status 가 제거되어야 합니다")
	assert.Nil(t, parsed["connected"], "런타임 필드 connected 가 제거되어야 합니다")
}

// TestAgentExport_YAML - 단일 에이전트를 YAML 파일로 내보내기
func TestAgentExport_YAML(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":        "agent-02",
				"name":      "yaml-에이전트",
				"type":      "scheduler",
				"status":    "stopped",
				"connected": false,
			},
		})
	})

	_, rootCmd, _ := setupAgentTest(t, withResolveSupport(handler))

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "agent.yaml")
	rootCmd.SetArgs([]string{"agent", "export", "agent-02", "-o", outputPath})

	err := rootCmd.Execute()
	require.NoError(t, err, "agent export YAML 실행 에러가 없어야 합니다")

	// 파일 존재 확인
	data, err := os.ReadFile(outputPath)
	require.NoError(t, err, "내보낸 파일을 읽을 수 있어야 합니다")

	// 유효한 YAML 인지 확인
	var parsed map[string]any
	err = yaml.Unmarshal(data, &parsed)
	require.NoError(t, err, "내보낸 파일이 유효한 YAML 이어야 합니다")

	// 에이전트 데이터 포함 확인
	assert.Equal(t, "yaml-에이전트", parsed["name"], "name 필드가 포함되어야 합니다")
	assert.Equal(t, "scheduler", parsed["type"], "type 필드가 포함되어야 합니다")
}

// TestAgentExport_RuntimeFieldsStripped - 런타임 필드가 모두 제거되는지 검증
func TestAgentExport_RuntimeFieldsStripped(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":           "agent-03",
				"name":         "필드-테스트",
				"type":         "worker",
				"status":       "running",
				"connected":    true,
				"uptime":       "5h",
				"messages_in":  10000,
				"messages_out": 9500,
				"error_count":  42,
				"config": map[string]any{
					"key": "value",
				},
			},
		})
	})

	_, rootCmd, _ := setupAgentTest(t, withResolveSupport(handler))

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "agent.json")
	rootCmd.SetArgs([]string{"agent", "export", "agent-03", "-o", outputPath})

	err := rootCmd.Execute()
	require.NoError(t, err)

	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	// 모든 런타임 필드가 제거되었는지 확인
	runtimeFields := []string{"id", "status", "connected", "uptime", "messages_in", "messages_out", "error_count"}
	for _, field := range runtimeFields {
		_, exists := parsed[field]
		assert.False(t, exists, "런타임 필드 '%s' 가 제거되어야 합니다", field)
	}

	// 비-런타임 필드는 유지되어야 함
	assert.Equal(t, "필드-테스트", parsed["name"])
	assert.Equal(t, "worker", parsed["type"])
	assert.NotNil(t, parsed["config"])
}

// TestAgentExport_MissingOutput - -o 플래그 미지정 시 에러 검증
func TestAgentExport_MissingOutput(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("-o 플래그가 없으면 서버에 요청하면 안됩니다")
	})

	_, rootCmd, _ := setupAgentTest(t, withResolveSupport(handler))
	rootCmd.SetArgs([]string{"agent", "export", "agent-01"})

	err := rootCmd.Execute()
	require.Error(t, err, "-o 플래그 없이 export 를 실행하면 에러가 발생해야 합니다")
	assert.Contains(t, err.Error(), "출력 파일 경로(-o)",
		"에러 메시지에 출력 경로 관련 내용이 포함되어야 합니다")
}

// TestAgentExport_SuccessMessage - 내보내기 성공 메시지 형식 검증
func TestAgentExport_SuccessMessage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":   "agent-01",
				"name": "테스트",
				"type": "worker",
			},
		})
	})

	_, rootCmd, buf := setupAgentTest(t, withResolveSupport(handler))

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "agent.json")
	rootCmd.SetArgs([]string{"agent", "export", "agent-01", "-o", outputPath})

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "에이전트 'agent-01'",
		"성공 메시지에 에이전트 ID 가 포함되어야 합니다")
	assert.Contains(t, output, "내보냈습니다",
		"성공 메시지에 내보냈습니다 가 포함되어야 합니다")
}

// --- TestAgentImport: 에이전트 가져오기 테스트 ---

// TestAgentImport_JSON - JSON 파일에서 에이전트 가져오기
func TestAgentImport_JSON(t *testing.T) {
	var receivedBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/agents", r.URL.Path)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &receivedBody)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":   "imported-01",
				"name": receivedBody["name"],
			},
		})
	})

	_, rootCmd, buf := setupAgentTest(t, handler)

	// 임시 JSON 파일 생성
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "agent.json")
	content := `{"name": "가져온-에이전트", "type": "worker", "config": {"host": "localhost"}}`
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	rootCmd.SetArgs([]string{"--format", "json", "agent", "import", "-f", filePath})

	err = rootCmd.Execute()
	require.NoError(t, err, "agent import JSON 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "imported-01", "가져온 에이전트 ID 가 출력에 포함되어야 합니다")

	// 전송된 본문 검증
	assert.Equal(t, "가져온-에이전트", receivedBody["name"],
		"요청 본문에 에이전트 이름이 포함되어야 합니다")
	assert.Equal(t, "worker", receivedBody["type"],
		"요청 본문에 에이전트 타입이 포함되어야 합니다")
}

// TestAgentImport_YAML - YAML 파일에서 에이전트 가져오기
func TestAgentImport_YAML(t *testing.T) {
	var receivedBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &receivedBody)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"id":   "imported-yaml",
				"name": "yaml-에이전트",
			},
		})
	})

	_, rootCmd, buf := setupAgentTest(t, handler)

	// 임시 YAML 파일 생성
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "agent.yaml")
	content := "name: yaml-에이전트\ntype: scheduler\n"
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	rootCmd.SetArgs([]string{"--format", "json", "agent", "import", "-f", filePath})

	err = rootCmd.Execute()
	require.NoError(t, err, "agent import YAML 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "imported-yaml", "가져온 에이전트 ID 가 출력에 포함되어야 합니다")

	// 전송된 본문 검증
	assert.Equal(t, "yaml-에이전트", receivedBody["name"],
		"요청 본문에 에이전트 이름이 포함되어야 합니다")
}

// TestAgentImport_MissingFile - -f 플래그 미지정 시 에러 검증
func TestAgentImport_MissingFile(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("-f 플래그가 없으면 서버에 요청하면 안됩니다")
	})

	_, rootCmd, _ := setupAgentTest(t, handler)
	rootCmd.SetArgs([]string{"agent", "import"})

	err := rootCmd.Execute()
	require.Error(t, err, "-f 플래그 없이 import 를 실행하면 에러가 발생해야 합니다")
	assert.Contains(t, err.Error(), "가져올 파일 경로(-f)",
		"에러 메시지에 파일 경로 관련 내용이 포함되어야 합니다")
}

// TestAgentImport_InvalidFormat - 유효하지 않은 파일 내용 에러 검증
func TestAgentImport_InvalidFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("잘못된 파일이면 서버에 요청하면 안됩니다")
	})

	_, rootCmd, _ := setupAgentTest(t, handler)

	// 유효하지 않은 내용의 임시 파일 생성
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "bad.json")
	err := os.WriteFile(filePath, []byte("invalid content <<<"), 0644)
	require.NoError(t, err)

	rootCmd.SetArgs([]string{"agent", "import", "-f", filePath})

	err = rootCmd.Execute()
	require.Error(t, err, "잘못된 파일 형식으로 import 를 실행하면 에러가 발생해야 합니다")
}

// --- TestAgentExport_BatchAll: 모든 에이전트 일괄 내보내기 ---

func TestAgentExport_BatchAll(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/agents", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": []map[string]any{
				{
					"id":        "agent-a",
					"name":      "에이전트-A",
					"type":      "worker",
					"status":    "running",
					"connected": true,
				},
				{
					"id":        "agent-b",
					"name":      "에이전트-B",
					"type":      "scheduler",
					"status":    "stopped",
					"connected": false,
				},
			},
		})
	})

	_, rootCmd, buf := setupAgentTest(t, handler)

	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "export")
	rootCmd.SetArgs([]string{"agent", "export", "--all", "-o", outputDir})

	err := rootCmd.Execute()
	require.NoError(t, err, "agent export --all 실행 에러가 없어야 합니다")

	// 디렉터리에 2개 파일 생성 확인
	entries, err := os.ReadDir(outputDir)
	require.NoError(t, err, "내보내기 디렉터리를 읽을 수 있어야 합니다")

	yamlFiles := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".yaml" {
			yamlFiles++
		}
	}
	assert.Equal(t, 2, yamlFiles, "2개의 .yaml 파일이 생성되어야 합니다")

	// 성공 메시지 확인
	output := buf.String()
	assert.Contains(t, output, "2개 에이전트",
		"성공 메시지에 에이전트 수가 포함되어야 합니다")
}

// --- TestAgentImport_Directory: 디렉터리에서 에이전트 일괄 가져오기 ---

func TestAgentImport_Directory(t *testing.T) {
	var importCount int

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/agents" {
			importCount++

			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			var req map[string]any
			err = json.Unmarshal(body, &req)
			require.NoError(t, err)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id":   fmt.Sprintf("imported-%d", importCount),
					"name": req["name"],
				},
			})
		}
	})

	_, rootCmd, buf := setupAgentTest(t, handler)

	// 3개 에이전트 파일이 있는 디렉터리 생성
	tmpDir := t.TempDir()
	agentDir := filepath.Join(tmpDir, "agents")
	err := os.MkdirAll(agentDir, 0755)
	require.NoError(t, err)

	// a.json
	err = os.WriteFile(filepath.Join(agentDir, "a.json"),
		[]byte(`{"name":"에이전트-A","type":"worker"}`), 0644)
	require.NoError(t, err)

	// b.yaml
	err = os.WriteFile(filepath.Join(agentDir, "b.yaml"),
		[]byte("name: 에이전트-B\ntype: scheduler\n"), 0644)
	require.NoError(t, err)

	// c.yml
	err = os.WriteFile(filepath.Join(agentDir, "c.yml"),
		[]byte("name: 에이전트-C\ntype: worker\n"), 0644)
	require.NoError(t, err)

	rootCmd.SetArgs([]string{"agent", "import", "-f", agentDir})

	err = rootCmd.Execute()
	require.NoError(t, err, "agent import (디렉터리) 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "3개 에이전트 가져오기 완료",
		"요약 메시지에 총 수가 포함되어야 합니다")
	assert.Contains(t, output, "성공: 3",
		"요약 메시지에 성공 수가 포함되어야 합니다")
	assert.Contains(t, output, "실패: 0",
		"요약 메시지에 실패 수가 포함되어야 합니다")
}

// TestAgentImport_DirectoryPartialFailure - 일괄 가져오기 부분 실패
func TestAgentImport_DirectoryPartialFailure(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/agents" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id": "imported",
				},
			})
		}
	})

	_, rootCmd, buf := setupAgentTest(t, handler)

	// 유효한 파일 2개 + 유효하지 않은 파일 1개
	tmpDir := t.TempDir()
	agentDir := filepath.Join(tmpDir, "agents")
	err := os.MkdirAll(agentDir, 0755)
	require.NoError(t, err)

	// 유효한 a.json
	err = os.WriteFile(filepath.Join(agentDir, "a.json"),
		[]byte(`{"name":"에이전트-A","type":"worker"}`), 0644)
	require.NoError(t, err)

	// 유효하지 않은 b.json
	err = os.WriteFile(filepath.Join(agentDir, "b.json"),
		[]byte("invalid json content <<<"), 0644)
	require.NoError(t, err)

	// 유효한 c.yaml
	err = os.WriteFile(filepath.Join(agentDir, "c.yaml"),
		[]byte("name: 에이전트-C\ntype: worker\n"), 0644)
	require.NoError(t, err)

	rootCmd.SetArgs([]string{"agent", "import", "-f", agentDir})

	err = rootCmd.Execute()
	require.NoError(t, err, "부분 실패 시에도 전체 프로세스는 에러 없이 완료되어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "3개 에이전트 가져오기 완료",
		"요약 메시지에 총 수가 포함되어야 합니다")
	assert.Contains(t, output, "성공: 2",
		"유효한 파일 2개가 성공해야 합니다")
	assert.Contains(t, output, "실패: 1",
		"유효하지 않은 파일 1개가 실패해야 합니다")
}

// TestAgentImport_DirectorySkipsNonAgentFiles - 에이전트 파일이 아닌 파일 건너뛰기
func TestAgentImport_DirectorySkipsNonAgentFiles(t *testing.T) {
	var importCount int

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/agents" {
			importCount++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id": "imported",
				},
			})
		}
	})

	_, rootCmd, buf := setupAgentTest(t, handler)

	tmpDir := t.TempDir()
	agentDir := filepath.Join(tmpDir, "agents")
	err := os.MkdirAll(agentDir, 0755)
	require.NoError(t, err)

	// 에이전트 파일 (처리 대상)
	err = os.WriteFile(filepath.Join(agentDir, "agent.yaml"),
		[]byte("name: 에이전트\ntype: worker\n"), 0644)
	require.NoError(t, err)

	// 비-에이전트 파일 (무시 대상)
	err = os.WriteFile(filepath.Join(agentDir, "readme.md"),
		[]byte("# README"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(agentDir, "notes.txt"),
		[]byte("some notes"), 0644)
	require.NoError(t, err)

	rootCmd.SetArgs([]string{"agent", "import", "-f", agentDir})

	err = rootCmd.Execute()
	require.NoError(t, err, "비-에이전트 파일이 있어도 에러 없이 실행되어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "1개 에이전트 가져오기 완료",
		".yaml 파일만 처리되어야 합니다")
	assert.Equal(t, 1, importCount,
		"에이전트 파일 1개만 서버에 요청되어야 합니다")
}

// --- 유틸리티 함수 단위 테스트 ---

// TestStripRuntimeFields - stripRuntimeFields 유닛 테스트
func TestStripRuntimeFields(t *testing.T) {
	input := map[string]any{
		"name":         "테스트-에이전트",
		"type":         "worker",
		"config":       map[string]any{"host": "localhost"},
		"id":           "agent-01",
		"status":       "running",
		"connected":    true,
		"uptime":       "2h",
		"messages_in":  5000,
		"messages_out": 4500,
		"error_count":  10,
	}

	result := stripRuntimeFields(input)

	// 비-런타임 필드는 유지
	assert.Equal(t, "테스트-에이전트", result["name"],
		"name 필드가 유지되어야 합니다")
	assert.Equal(t, "worker", result["type"],
		"type 필드가 유지되어야 합니다")
	assert.NotNil(t, result["config"],
		"config 필드가 유지되어야 합니다")

	// 런타임 필드는 제거
	runtimeFields := []string{"id", "status", "connected", "uptime", "messages_in", "messages_out", "error_count"}
	for _, field := range runtimeFields {
		_, exists := result[field]
		assert.False(t, exists, "런타임 필드 '%s' 가 제거되어야 합니다", field)
	}

	// 원본은 변경되지 않아야 함
	assert.NotNil(t, input["id"], "원본 맵은 변경되지 않아야 합니다")
}

// TestSanitizeFileName - sanitizeFileName 테이블 드리븐 테스트
func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "일반 문자열",
			input:    "simple",
			expected: "simple",
		},
		{
			name:     "공백 포함",
			input:    "has spaces",
			expected: "has-spaces",
		},
		{
			name:     "특수 문자 포함",
			input:    "special!@#chars",
			expected: "special---chars",
		},
		{
			name:     "한국어 이름",
			input:    "한국어-name",
			expected: "----name",
		},
		{
			name:     "유효한 문자 조합",
			input:    "valid-name_v2.0",
			expected: "valid-name_v2.0",
		},
		{
			name:     "숫자만",
			input:    "12345",
			expected: "12345",
		},
		{
			name:     "혼합 문자",
			input:    "agent/v1:latest",
			expected: "agent-v1-latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeFileName(tt.input)
			assert.Equal(t, tt.expected, result,
				"sanitizeFileName(%q) = %q, 기대값 %q", tt.input, result, tt.expected)
		})
	}
}

// TestBuildAgentCreateRequest_WithConfig - config 키가 있는 경우
func TestBuildAgentCreateRequest_WithConfig(t *testing.T) {
	input := map[string]any{
		"name": "테스트-에이전트",
		"type": "worker",
		"config": map[string]any{
			"host": "localhost",
			"port": 8080,
		},
	}

	result := buildAgentCreateRequest(input)

	assert.Equal(t, "테스트-에이전트", result["name"],
		"name 필드가 올바라야 합니다")
	assert.Equal(t, "worker", result["type"],
		"type 필드가 올바라야 합니다")

	config, ok := result["config"].(map[string]any)
	require.True(t, ok, "config 필드가 map[string]any 이어야 합니다")
	assert.Equal(t, "localhost", config["host"])
}

// --- TestAgentGet_ByName: --name 플래그로 에이전트 조회 ---

func TestAgentGet_ByName(t *testing.T) {
	agentUUID := "aaaa1111-2222-3333-4444-555566667777"
	agents := []map[string]any{
		{"id": agentUUID, "name": "mqtt-sensor"},
		{"id": "bbbb1111-2222-3333-4444-555566667777", "name": "http-receiver"},
	}
	agentDetail := map[string]any{
		"id": agentUUID, "name": "mqtt-sensor", "type": "mqtt", "status": "running",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/agents":
			// resolveAgentID 의 이름 조회 요청
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": agents})
		case "/api/v1/agents/" + agentUUID:
			// 실제 get 요청
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": agentDetail})
		default:
			t.Errorf("예상치 못한 경로: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, rootCmd, buf := setupAgentTest(t, handler)
	rootCmd.SetArgs([]string{"--format", "json", "agent", "get", "--name", "mqtt-sensor"})

	err := rootCmd.Execute()
	require.NoError(t, err, "--name 플래그로 agent get 실행 에러가 없어야 합니다")

	output := buf.String()
	var parsed map[string]any
	err = json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err, "JSON 출력이 유효해야 합니다")
	assert.Equal(t, agentUUID, parsed["id"],
		"이름으로 해석된 UUID 를 사용하여 에이전트를 조회해야 합니다")
}

// TestAgentGet_PositionalName - 이름을 positional 인자로 에이전트 조회
func TestAgentGet_PositionalName(t *testing.T) {
	agentUUID := "aaaa1111-2222-3333-4444-555566667777"
	agents := []map[string]any{
		{"id": agentUUID, "name": "mqtt-sensor"},
	}
	agentDetail := map[string]any{
		"id": agentUUID, "name": "mqtt-sensor", "type": "mqtt", "status": "running",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/agents":
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": agents})
		case "/api/v1/agents/" + agentUUID:
			json.NewEncoder(w).Encode(map[string]any{"success": true, "data": agentDetail})
		default:
			t.Errorf("예상치 못한 경로: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, rootCmd, buf := setupAgentTest(t, handler)
	rootCmd.SetArgs([]string{"--format", "json", "agent", "get", "mqtt-sensor"})

	err := rootCmd.Execute()
	require.NoError(t, err, "positional 이름으로 agent get 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, agentUUID,
		"이름이 UUID 로 해석되어 조회되어야 합니다")
}

// --- TestAgentList_NameFilter: --name 플래그로 에이전트 목록 필터링 ---

func TestAgentList_NameFilter(t *testing.T) {
	agents := []agentInfo{
		{ID: "agent-1", Name: "mqtt-sensor", Type: "mqtt", Status: "running", Connected: true},
		{ID: "agent-2", Name: "http-receiver", Type: "http", Status: "stopped", Connected: false},
		{ID: "agent-3", Name: "mqtt-publisher", Type: "mqtt", Status: "running", Connected: true},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/agents", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"success": true, "data": agents})
	})

	_, rootCmd, buf := setupAgentTest(t, handler)
	rootCmd.SetArgs([]string{"agent", "list", "--name", "mqtt"})

	err := rootCmd.Execute()
	require.NoError(t, err, "agent list --name mqtt 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "mqtt-sensor", "mqtt-sensor 가 포함되어야 합니다")
	assert.Contains(t, output, "mqtt-publisher", "mqtt-publisher 가 포함되어야 합니다")
	assert.NotContains(t, output, "http-receiver", "http-receiver 는 필터링되어야 합니다")
}

// TestAgentList_NameFilter_NoMatch - --name 필터 매칭 없음
func TestAgentList_NameFilter_NoMatch(t *testing.T) {
	agents := []agentInfo{
		{ID: "agent-1", Name: "mqtt-sensor", Type: "mqtt", Status: "running", Connected: true},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"success": true, "data": agents})
	})

	_, rootCmd, buf := setupAgentTest(t, handler)
	rootCmd.SetArgs([]string{"agent", "list", "--name", "nonexistent"})

	err := rootCmd.Execute()
	require.NoError(t, err, "매칭 없어도 에러가 없어야 합니다")

	output := buf.String()
	assert.NotContains(t, output, "mqtt-sensor", "필터링 후 mqtt-sensor 가 없어야 합니다")
}

// --- TestAgentLifecycle_ByName: --name 플래그로 start/stop/restart ---

func TestAgentLifecycle_ByName(t *testing.T) {
	agentUUID := "aaaa1111-2222-3333-4444-555566667777"
	agents := []map[string]any{
		{"id": agentUUID, "name": "mqtt-sensor"},
	}

	subcommands := []string{"start", "stop", "restart"}

	for _, sub := range subcommands {
		t.Run(fmt.Sprintf("agent %s --name", sub), func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/agents":
					json.NewEncoder(w).Encode(map[string]any{"success": true, "data": agents})
				case "/api/v1/agents/" + agentUUID + "/" + sub:
					assert.Equal(t, http.MethodPost, r.Method)
					json.NewEncoder(w).Encode(map[string]any{
						"success": true,
						"data":    map[string]any{"id": agentUUID, "status": sub + "ed"},
					})
				default:
					t.Errorf("예상치 못한 경로: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			})

			_, rootCmd, buf := setupAgentTest(t, handler)
			rootCmd.SetArgs([]string{"agent", sub, "--name", "mqtt-sensor"})

			err := rootCmd.Execute()
			require.NoError(t, err,
				"--name 플래그로 agent %s 실행 에러가 없어야 합니다", sub)

			output := buf.String()
			assert.NotEmpty(t, output,
				"agent %s --name 의 출력이 비어있으면 안됩니다", sub)
		})
	}
}

// TestBuildAgentCreateRequest_WithoutConfig - config 키가 없는 경우
func TestBuildAgentCreateRequest_WithoutConfig(t *testing.T) {
	input := map[string]any{
		"name":     "테스트-에이전트",
		"type":     "worker",
		"host":     "localhost",
		"port":     8080,
		"interval": "5s",
	}

	result := buildAgentCreateRequest(input)

	assert.Equal(t, "테스트-에이전트", result["name"],
		"name 필드가 올바라야 합니다")
	assert.Equal(t, "worker", result["type"],
		"type 필드가 올바라야 합니다")

	// config 에 name, type 외의 나머지 필드가 들어가야 함
	config, ok := result["config"].(map[string]any)
	require.True(t, ok, "config 필드가 map[string]any 이어야 합니다")
	assert.Equal(t, "localhost", config["host"],
		"config 에 host 가 포함되어야 합니다")
	assert.Equal(t, 8080, config["port"],
		"config 에 port 가 포함되어야 합니다")
	assert.Equal(t, "5s", config["interval"],
		"config 에 interval 이 포함되어야 합니다")

	// config 에 name, type 은 포함되지 않아야 함
	_, hasName := config["name"]
	_, hasType := config["type"]
	assert.False(t, hasName, "config 에 name 이 포함되면 안됩니다")
	assert.False(t, hasType, "config 에 type 이 포함되면 안됩니다")
}
