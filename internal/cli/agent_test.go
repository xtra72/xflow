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
)

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

	_, rootCmd, buf := setupAgentTest(t, handler)
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

			_, rootCmd, buf := setupAgentTest(t, handler)
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

		_, rootCmd, buf := setupAgentTestWithConfirm(t, handler, confirmFn)
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

		_, rootCmd, buf := setupAgentTestWithConfirm(t, handler, confirmFn)
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

	_, rootCmd, _ := setupAgentTestWithConfirm(t, handler, confirmFn)
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

	_, rootCmd, buf := setupAgentTestWithConfirm(t, handler, confirmFn)
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

	expectedSubs := []string{"list", "get", "create", "start", "stop", "restart", "delete"}

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

	_, rootCmd, _ := setupAgentTestWithConfirm(t, handler, confirmFn)
	rootCmd.SetArgs([]string{"agent", "delete", "agent-1"})

	err := rootCmd.Execute()
	require.NoError(t, err, "stdin y 입력으로 agent delete 실행 에러가 없어야 합니다")
}
