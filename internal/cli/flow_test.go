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

// --- 테스트 헬퍼 ---

// apiEnvelope 은 API 응답 엔벨로프를 생성하는 헬퍼이다.
func apiEnvelope(data any) []byte {
	resp := map[string]any{
		"success": true,
		"data":    data,
	}
	b, _ := json.Marshal(resp)
	return b
}

// setupFlowTest 는 mock 서버와 커맨드를 세팅하는 헬퍼이다.
// handler 에 라우팅 맵을 넘기면 요청 경로에 따라 응답을 반환한다.
func setupFlowTest(t *testing.T, handler http.HandlerFunc) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()

	server := httptest.NewServer(handler)

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	clientPtr := &client

	// confirmFn 은 항상 false (기본: 삭제 거부)
	confirmFn := func(prompt string, reader io.Reader) bool {
		return false
	}

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.AddCommand(newFlowCmd(clientPtr, confirmFn))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return &buf, rootCmd, server.Close
}

// setupFlowTestWithConfirm 은 confirmFn 을 커스텀할 수 있는 세팅 헬퍼이다.
func setupFlowTestWithConfirm(t *testing.T, handler http.HandlerFunc, confirmFn func(string, io.Reader) bool) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()

	server := httptest.NewServer(handler)

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	clientPtr := &client

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.AddCommand(newFlowCmd(clientPtr, confirmFn))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return &buf, rootCmd, server.Close
}

// --- flow list 테스트 ---

// TestFlowList - 플로우 목록 조회 및 테이블 출력 검증
func TestFlowList(t *testing.T) {
	flows := []map[string]any{
		{
			"id":         "flow-001",
			"name":       "데이터 파이프라인",
			"status":     "running",
			"node_count": float64(5),
			"created_at": "2026-01-15T10:00:00Z",
		},
		{
			"id":         "flow-002",
			"name":       "알림 워크플로우",
			"status":     "stopped",
			"node_count": float64(3),
			"created_at": "2026-01-16T12:00:00Z",
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(flows))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "list"})
	err := cmd.Execute()
	require.NoError(t, err, "flow list 실행 에러가 없어야 합니다")

	output := buf.String()
	// 테이블 헤더 검증
	assert.Contains(t, output, "ID", "테이블에 ID 헤더가 있어야 합니다")
	assert.Contains(t, output, "NAME", "테이블에 NAME 헤더가 있어야 합니다")
	assert.Contains(t, output, "STATUS", "테이블에 STATUS 헤더가 있어야 합니다")
	// 데이터 검증
	assert.Contains(t, output, "flow-001", "출력에 flow-001 이 포함되어야 합니다")
	assert.Contains(t, output, "flow-002", "출력에 flow-002 가 포함되어야 합니다")
	assert.Contains(t, output, "running", "출력에 running 상태가 포함되어야 합니다")
}

// TestFlowList_JSONFormat - JSON 출력 형식 검증
func TestFlowList_JSONFormat(t *testing.T) {
	flows := []map[string]any{
		{"id": "flow-001", "name": "테스트 플로우"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(flows))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "list", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "flow list --format json 실행 에러가 없어야 합니다")

	output := buf.String()
	// JSON 파싱 가능 여부 검증
	var result []map[string]any
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Len(t, result, 1, "JSON 배열에 1개 항목이 있어야 합니다")
}

// --- flow get 테스트 ---

// TestFlowGet - 단일 플로우 상세 조회 검증
func TestFlowGet(t *testing.T) {
	flow := map[string]any{
		"id":         "flow-001",
		"name":       "데이터 파이프라인",
		"status":     "running",
		"node_count": float64(5),
		"created_at": "2026-01-15T10:00:00Z",
		"nodes": []map[string]any{
			{"id": "node-1", "type": "trigger"},
			{"id": "node-2", "type": "processor"},
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/flow-001", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(flow))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	// 기본 형식은 table 이지만 단일 객체는 json 으로 표시
	cmd.SetArgs([]string{"flow", "get", "flow-001", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "flow get 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "flow-001", "출력에 flow ID 가 포함되어야 합니다")
	assert.Contains(t, output, "데이터 파이프라인", "출력에 플로우 이름이 포함되어야 합니다")
}

// --- flow create 테스트 ---

// TestFlowCreate - 파일에서 플로우 생성 검증
func TestFlowCreate(t *testing.T) {
	created := map[string]any{
		"id":     "flow-new",
		"name":   "새 플로우",
		"status": "created",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		// 요청 바디 검증
		body, _ := io.ReadAll(r.Body)
		assert.NotEmpty(t, body, "요청 바디가 비어있으면 안됩니다")

		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(created))
	})

	// 임시 파일 생성
	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.json")
	flowData := `{"name": "새 플로우", "nodes": []}`
	err := os.WriteFile(flowFile, []byte(flowData), 0644)
	require.NoError(t, err)

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "create", "-f", flowFile, "--format", "json"})
	err = cmd.Execute()
	require.NoError(t, err, "flow create 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "flow-new", "출력에 생성된 플로우 ID 가 포함되어야 합니다")
}

// TestFlowCreate_FileNotFound - 파일이 없을 때 에러 검증
func TestFlowCreate_FileNotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("파일이 없으면 서버에 요청하지 않아야 합니다")
	})

	_, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "create", "-f", "/nonexistent/flow.json"})
	err := cmd.Execute()
	require.Error(t, err, "존재하지 않는 파일은 에러를 반환해야 합니다")
	assert.Contains(t, err.Error(), "파일", "에러 메시지에 파일 관련 내용이 포함되어야 합니다")
}

// --- flow update 테스트 ---

// TestFlowUpdate - 파일에서 플로우 업데이트 검증
func TestFlowUpdate(t *testing.T) {
	updated := map[string]any{
		"id":     "flow-001",
		"name":   "업데이트된 플로우",
		"status": "updated",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/flow-001", r.URL.Path)
		assert.Equal(t, http.MethodPut, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(updated))
	})

	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.json")
	flowData := `{"name": "업데이트된 플로우"}`
	err := os.WriteFile(flowFile, []byte(flowData), 0644)
	require.NoError(t, err)

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "update", "flow-001", "-f", flowFile, "--format", "json"})
	err = cmd.Execute()
	require.NoError(t, err, "flow update 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "flow-001", "출력에 업데이트된 플로우 ID 가 포함되어야 합니다")
}

// --- flow delete 테스트 ---

// TestFlowDelete_WithConfirmation - 확인 프롬프트 Y 응답 시 삭제 검증
func TestFlowDelete_WithConfirmation(t *testing.T) {
	var deleteRequested bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/flow-001", r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)
		deleteRequested = true
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"deleted": true}))
	})

	// confirmFn 이 true 를 반환하도록 설정
	confirmFn := func(prompt string, reader io.Reader) bool {
		return true
	}

	buf, cmd, cleanup := setupFlowTestWithConfirm(t, handler, confirmFn)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "delete", "flow-001"})
	err := cmd.Execute()
	require.NoError(t, err, "flow delete 실행 에러가 없어야 합니다")

	assert.True(t, deleteRequested, "삭제 요청이 서버에 전송되어야 합니다")
	output := buf.String()
	assert.Contains(t, output, "삭제", "출력에 삭제 확인 메시지가 포함되어야 합니다")
}

// TestFlowDelete_Cancelled - 확인 프롬프트 N 응답 시 삭제 취소 검증
func TestFlowDelete_Cancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("삭제 취소 시 서버에 요청하지 않아야 합니다")
	})

	// confirmFn 이 false 를 반환 (삭제 거부)
	confirmFn := func(prompt string, reader io.Reader) bool {
		return false
	}

	buf, cmd, cleanup := setupFlowTestWithConfirm(t, handler, confirmFn)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "delete", "flow-001"})
	err := cmd.Execute()
	require.NoError(t, err, "삭제 취소는 에러가 아닙니다")

	output := buf.String()
	assert.Contains(t, output, "취소", "출력에 취소 메시지가 포함되어야 합니다")
}

// TestFlowDelete_WithYesFlag - --yes 플래그로 확인 생략 검증
func TestFlowDelete_WithYesFlag(t *testing.T) {
	var deleteRequested bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/flow-001", r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)
		deleteRequested = true
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"deleted": true}))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "delete", "flow-001", "--yes"})
	err := cmd.Execute()
	require.NoError(t, err, "flow delete --yes 실행 에러가 없어야 합니다")

	assert.True(t, deleteRequested, "--yes 시 확인 없이 삭제 요청을 보내야 합니다")
	output := buf.String()
	assert.Contains(t, output, "삭제", "출력에 삭제 확인 메시지가 포함되어야 합니다")
}

// --- flow deploy/start/stop/restart 테스트 ---

// TestFlowLifecycle - deploy, start, stop, restart 서브커맨드 테이블 드리븐 테스트
func TestFlowLifecycle(t *testing.T) {
	tests := []struct {
		name       string
		subcommand string
		apiPath    string
		method     string
	}{
		{
			name:       "deploy",
			subcommand: "deploy",
			apiPath:    "/api/v1/flows/flow-001/deploy",
			method:     http.MethodPost,
		},
		{
			name:       "start",
			subcommand: "start",
			apiPath:    "/api/v1/flows/flow-001/start",
			method:     http.MethodPost,
		},
		{
			name:       "stop",
			subcommand: "stop",
			apiPath:    "/api/v1/flows/flow-001/stop",
			method:     http.MethodPost,
		},
		{
			name:       "restart",
			subcommand: "restart",
			apiPath:    "/api/v1/flows/flow-001/restart",
			method:     http.MethodPost,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := map[string]any{
				"id":     "flow-001",
				"status": tt.subcommand + "ed",
			}

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tt.apiPath, r.URL.Path,
					"%s 요청 경로가 %s 여야 합니다", tt.subcommand, tt.apiPath)
				assert.Equal(t, tt.method, r.Method,
					"%s 요청 메서드가 %s 여야 합니다", tt.subcommand, tt.method)
				w.Header().Set("Content-Type", "application/json")
				w.Write(apiEnvelope(result))
			})

			buf, cmd, cleanup := setupFlowTest(t, handler)
			defer cleanup()

			cmd.SetArgs([]string{"flow", tt.subcommand, "flow-001", "--format", "json"})
			err := cmd.Execute()
			require.NoError(t, err,
				"flow %s 실행 에러가 없어야 합니다", tt.subcommand)

			output := buf.String()
			assert.Contains(t, output, "flow-001",
				"출력에 flow ID 가 포함되어야 합니다")
		})
	}
}

// --- flow export 테스트 ---

// TestFlowExport_JSON - JSON 형식으로 플로우 내보내기 검증
func TestFlowExport_JSON(t *testing.T) {
	flow := map[string]any{
		"id":     "flow-001",
		"name":   "내보내기 테스트",
		"status": "running",
		"nodes":  []any{},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/flow-001", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(flow))
	})

	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "exported.json")

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "export", "flow-001", "-o", outputFile})
	err := cmd.Execute()
	require.NoError(t, err, "flow export 실행 에러가 없어야 합니다")

	// 파일 존재 및 내용 검증
	data, err := os.ReadFile(outputFile)
	require.NoError(t, err, "내보내기 파일을 읽을 수 있어야 합니다")

	var exported map[string]any
	err = json.Unmarshal(data, &exported)
	require.NoError(t, err, "내보내기 파일이 유효한 JSON 이어야 합니다")
	assert.Equal(t, "flow-001", exported["id"], "내보내기된 플로우 ID 가 일치해야 합니다")

	output := buf.String()
	assert.Contains(t, output, outputFile, "출력에 내보내기 파일 경로가 포함되어야 합니다")
}

// TestFlowExport_YAML - YAML 형식으로 플로우 내보내기 검증
func TestFlowExport_YAML(t *testing.T) {
	flow := map[string]any{
		"id":     "flow-001",
		"name":   "YAML 내보내기",
		"status": "stopped",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/flow-001", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(flow))
	})

	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "exported.yaml")

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "export", "flow-001", "-o", outputFile})
	err := cmd.Execute()
	require.NoError(t, err, "flow export (YAML) 실행 에러가 없어야 합니다")

	// 파일 존재 및 내용 검증
	data, err := os.ReadFile(outputFile)
	require.NoError(t, err, "YAML 내보내기 파일을 읽을 수 있어야 합니다")

	content := string(data)
	assert.Contains(t, content, "flow-001", "YAML 파일에 플로우 ID 가 포함되어야 합니다")
	assert.Contains(t, content, "YAML", "YAML 파일에 플로우 이름이 포함되어야 합니다")

	output := buf.String()
	assert.Contains(t, output, outputFile, "출력에 파일 경로가 포함되어야 합니다")
}

// --- flow import 테스트 ---

// TestFlowImport - 파일에서 플로우 가져오기 검증
func TestFlowImport(t *testing.T) {
	imported := map[string]any{
		"id":     "flow-imported",
		"name":   "가져온 플로우",
		"status": "created",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(imported))
	})

	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "import.json")
	flowData := `{"name": "가져온 플로우", "nodes": []}`
	err := os.WriteFile(flowFile, []byte(flowData), 0644)
	require.NoError(t, err)

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "import", "-f", flowFile, "--format", "json"})
	err = cmd.Execute()
	require.NoError(t, err, "flow import 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "flow-imported", "출력에 가져온 플로우 ID 가 포함되어야 합니다")
}

// --- flow status 테스트 ---

// TestFlowStatus - 플로우 런타임 상태 조회 검증
func TestFlowStatus(t *testing.T) {
	status := map[string]any{
		"id":                 "flow-001",
		"state":              "running",
		"messages_processed": float64(1234),
		"errors":             float64(2),
		"uptime":             "2h30m",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/flow-001/status", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(status))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "status", "flow-001", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "flow status 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "running", "출력에 상태가 포함되어야 합니다")
	assert.Contains(t, output, "1234", "출력에 처리된 메시지 수가 포함되어야 합니다")
}

// TestFlowStatus_TableFormat - 테이블 형식의 상태 출력 검증
func TestFlowStatus_TableFormat(t *testing.T) {
	status := map[string]any{
		"id":                 "flow-001",
		"state":              "running",
		"messages_processed": float64(500),
		"errors":             float64(0),
		"uptime":             "1h00m",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(status))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "status", "flow-001"})
	err := cmd.Execute()
	require.NoError(t, err, "flow status (table) 실행 에러가 없어야 합니다")

	output := buf.String()
	// 테이블 형식 키-값 출력 검증
	assert.Contains(t, output, "flow-001", "출력에 flow ID 가 포함되어야 합니다")
	assert.Contains(t, output, "running", "출력에 상태가 포함되어야 합니다")
}

// --- flow get 인자 누락 테스트 ---

// TestFlowGet_MissingArg - ID 인자 누락 시 에러 검증
func TestFlowGet_MissingArg(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("인자 누락 시 서버 요청이 없어야 합니다")
	})

	_, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "get"})
	err := cmd.Execute()
	require.Error(t, err, "ID 인자 누락 시 에러를 반환해야 합니다")
}

// --- flow create -f 누락 테스트 ---

// TestFlowCreate_MissingFlag - -f 플래그 누락 시 에러 검증
func TestFlowCreate_MissingFlag(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("-f 플래그 누락 시 서버 요청이 없어야 합니다")
	})

	_, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "create"})
	err := cmd.Execute()
	require.Error(t, err, "-f 플래그 누락 시 에러를 반환해야 합니다")
}

// --- API 에러 전파 테스트 ---

// TestFlowList_APIError - API 에러가 사용자에게 올바르게 전파되는지 검증
func TestFlowList_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		resp := map[string]any{
			"success": false,
			"error": map[string]any{
				"code":    "INTERNAL_ERROR",
				"message": "내부 서버 오류",
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	_, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "list"})
	err := cmd.Execute()
	require.Error(t, err, "API 에러가 전파되어야 합니다")
}

// --- flow export -o 누락 테스트 ---

// TestFlowExport_MissingOutputFlag - -o 플래그 누락 시 에러 검증
func TestFlowExport_MissingOutputFlag(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("-o 플래그 누락 시 서버 요청이 없어야 합니다")
	})

	_, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "export", "flow-001"})
	err := cmd.Execute()
	require.Error(t, err, "-o 플래그 누락 시 에러를 반환해야 합니다")
}

// --- flow 서브커맨드 등록 검증 ---

// TestFlowSubcommands - 12개 서브커맨드 등록 여부 검증
func TestFlowSubcommands(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)
	clientPtr := &client
	confirmFn := func(prompt string, reader io.Reader) bool { return false }

	flowCmd := newFlowCmd(clientPtr, confirmFn)

	expectedSubcommands := []string{
		"list", "get", "create", "update", "delete",
		"deploy", "start", "stop", "restart",
		"export", "import", "status",
	}

	subNames := make(map[string]bool)
	for _, sub := range flowCmd.Commands() {
		subNames[sub.Use] = true
		// Use 필드에 인자 정보가 포함될 수 있으므로 첫 번째 단어만 검사
		parts := strings.Fields(sub.Use)
		if len(parts) > 0 {
			subNames[parts[0]] = true
		}
	}

	for _, expected := range expectedSubcommands {
		assert.True(t, subNames[expected],
			"flow 에 '%s' 서브커맨드가 등록되어 있어야 합니다", expected)
	}
}

// --- flow create YAML 파일 테스트 ---

// TestFlowCreate_YAML - YAML 파일로 플로우 생성 검증
func TestFlowCreate_YAML(t *testing.T) {
	created := map[string]any{
		"id":     "flow-yaml",
		"name":   "YAML 플로우",
		"status": "created",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(created))
	})

	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "flow.yaml")
	flowData := "name: YAML 플로우\nnodes: []\n"
	err := os.WriteFile(flowFile, []byte(flowData), 0644)
	require.NoError(t, err)

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "create", "-f", flowFile, "--format", "json"})
	err = cmd.Execute()
	require.NoError(t, err, "flow create (YAML) 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "flow-yaml", "출력에 생성된 플로우 ID 가 포함되어야 합니다")
}

// --- flow list 빈 결과 테스트 ---

// TestFlowList_Empty - 빈 플로우 목록 출력 검증
func TestFlowList_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope([]map[string]any{}))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "list"})
	err := cmd.Execute()
	require.NoError(t, err, "빈 목록 조회 에러가 없어야 합니다")

	output := buf.String()
	// 헤더는 있어야 함
	assert.Contains(t, output, "ID", "빈 목록에도 테이블 헤더가 있어야 합니다")
}

// --- getFormat 헬퍼 테스트를 위한 추가 테스트 ---

// TestFlowGet_TextFormat - text 출력 형식 검증
func TestFlowGet_TextFormat(t *testing.T) {
	flow := map[string]any{
		"id":     "flow-001",
		"name":   "텍스트 테스트",
		"status": "running",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(flow))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "get", "flow-001", "--format", "text"})
	err := cmd.Execute()
	require.NoError(t, err, "flow get --format text 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "flow-001", "text 형식에 flow ID 가 포함되어야 합니다")
}

// --- flow lifecycle 메시지 검증 ---

// TestFlowDeploy_SuccessMessage - deploy 성공 메시지 검증
func TestFlowDeploy_SuccessMessage(t *testing.T) {
	result := map[string]any{
		"id":     "flow-001",
		"status": "deployed",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/flow-001/deploy", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(result))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "deploy", "flow-001"})
	err := cmd.Execute()
	require.NoError(t, err, "flow deploy 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.NotEmpty(t, output, "deploy 성공 시 출력이 있어야 합니다")
	// 성공 메시지 또는 결과 데이터가 있어야 함
	assert.True(t,
		strings.Contains(output, "flow-001") || strings.Contains(output, "deploy"),
		"출력에 관련 정보가 포함되어야 합니다")
}

// --- 함수 시그니처 검증 ---

// TestNewFlowCmd_Signature - newFlowCmd 함수 시그니처 검증
func TestNewFlowCmd_Signature(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)
	clientPtr := &client
	confirmFn := func(prompt string, reader io.Reader) bool { return false }

	flowCmd := newFlowCmd(clientPtr, confirmFn)
	require.NotNil(t, flowCmd, "newFlowCmd 가 nil 을 반환하면 안됩니다")
	assert.Equal(t, "flow", flowCmd.Use, "flow 커맨드의 Use 가 'flow' 여야 합니다")
}

// --- 출력 형식 지원 전체 검증 ---

// TestFlowList_AllFormats - list 의 4가지 출력 형식 검증
func TestFlowList_AllFormats(t *testing.T) {
	flows := []map[string]any{
		{"id": "f-1", "name": "테스트", "status": "running", "node_count": float64(1), "created_at": "2026-01-01"},
	}

	formats := []string{"json", "yaml", "table", "text"}

	for _, format := range formats {
		t.Run(fmt.Sprintf("format_%s", format), func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(apiEnvelope(flows))
			})

			buf, cmd, cleanup := setupFlowTest(t, handler)
			defer cleanup()

			cmd.SetArgs([]string{"flow", "list", "--format", format})
			err := cmd.Execute()
			require.NoError(t, err,
				"flow list --format %s 실행 에러가 없어야 합니다", format)

			output := buf.String()
			assert.NotEmpty(t, output,
				"format=%s 에서 출력이 비어있으면 안됩니다", format)
		})
	}
}
