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

// testFlowUUID 는 테스트에서 사용하는 플로우 ID이다.
const testFlowUUID = "f47ac10b-58cc-4372-a567-0e02b2c3d479"

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

// --- flow list --name 필터 테스트 ---

// TestFlowList_NameFilter - --name 플래그로 플로우 목록 필터링 검증
func TestFlowList_NameFilter(t *testing.T) {
	flows := []map[string]any{
		{"id": "flow-001", "name": "데이터 파이프라인", "status": "running", "node_count": float64(5), "created_at": "2026-01-15T10:00:00Z"},
		{"id": "flow-002", "name": "알림 워크플로우", "status": "stopped", "node_count": float64(3), "created_at": "2026-01-16T12:00:00Z"},
		{"id": "flow-003", "name": "데이터 수집기", "status": "running", "node_count": float64(2), "created_at": "2026-01-17T08:00:00Z"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(flows))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "list", "--name", "데이터"})
	err := cmd.Execute()
	require.NoError(t, err, "flow list --name 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "flow-001", "데이터 파이프라인 이 포함되어야 합니다")
	assert.Contains(t, output, "flow-003", "데이터 수집기 가 포함되어야 합니다")
	assert.NotContains(t, output, "flow-002", "알림 워크플로우 는 필터링되어야 합니다")
}

// TestFlowList_NameFilter_CaseInsensitive - --name 필터 대소문자 무시 검증
func TestFlowList_NameFilter_CaseInsensitive(t *testing.T) {
	flows := []map[string]any{
		{"id": "flow-001", "name": "MQTT-Pipeline", "status": "running", "node_count": float64(3), "created_at": "2026-01-15T10:00:00Z"},
		{"id": "flow-002", "name": "http-service", "status": "stopped", "node_count": float64(2), "created_at": "2026-01-16T12:00:00Z"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(flows))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "list", "--name", "mqtt"})
	err := cmd.Execute()
	require.NoError(t, err, "flow list --name (대소문자 무시) 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "flow-001", "MQTT-Pipeline 이 대소문자 무시로 매칭되어야 합니다")
	assert.NotContains(t, output, "flow-002", "http-service 는 필터링되어야 합니다")
}

// --- flow get 테스트 ---

// TestFlowGet - 단일 플로우 상세 조회 검증
func TestFlowGet(t *testing.T) {
	flow := map[string]any{
		"id":         testFlowUUID,
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
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID, r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(flow))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	// 기본 형식은 table 이지만 단일 객체는 json 으로 표시
	cmd.SetArgs([]string{"flow", "get", testFlowUUID, "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "flow get 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, testFlowUUID, "출력에 flow ID 가 포함되어야 합니다")
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
		"id":     testFlowUUID,
		"name":   "업데이트된 플로우",
		"status": "updated",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID, r.URL.Path)
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

	cmd.SetArgs([]string{"flow", "update", testFlowUUID, "-f", flowFile, "--format", "json"})
	err = cmd.Execute()
	require.NoError(t, err, "flow update 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, testFlowUUID, "출력에 업데이트된 플로우 ID 가 포함되어야 합니다")
}

// --- flow delete 테스트 ---

// TestFlowDelete_WithConfirmation - 확인 프롬프트 Y 응답 시 삭제 검증
func TestFlowDelete_WithConfirmation(t *testing.T) {
	var deleteRequested bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID, r.URL.Path)
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

	cmd.SetArgs([]string{"flow", "delete", testFlowUUID})
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

	cmd.SetArgs([]string{"flow", "delete", testFlowUUID})
	err := cmd.Execute()
	require.NoError(t, err, "삭제 취소는 에러가 아닙니다")

	output := buf.String()
	assert.Contains(t, output, "취소", "출력에 취소 메시지가 포함되어야 합니다")
}

// TestFlowDelete_WithYesFlag - --yes 플래그로 확인 생략 검증
func TestFlowDelete_WithYesFlag(t *testing.T) {
	var deleteRequested bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID, r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)
		deleteRequested = true
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"deleted": true}))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "delete", testFlowUUID, "--yes"})
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
			apiPath:    "/api/v1/flows/" + testFlowUUID + "/deploy",
			method:     http.MethodPost,
		},
		{
			name:       "start",
			subcommand: "start",
			apiPath:    "/api/v1/flows/" + testFlowUUID + "/start",
			method:     http.MethodPost,
		},
		{
			name:       "stop",
			subcommand: "stop",
			apiPath:    "/api/v1/flows/" + testFlowUUID + "/stop",
			method:     http.MethodPost,
		},
		{
			name:       "restart",
			subcommand: "restart",
			apiPath:    "/api/v1/flows/" + testFlowUUID + "/restart",
			method:     http.MethodPost,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := map[string]any{
				"id":     testFlowUUID,
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

			cmd.SetArgs([]string{"flow", tt.subcommand, testFlowUUID, "--format", "json"})
			err := cmd.Execute()
			require.NoError(t, err,
				"flow %s 실행 에러가 없어야 합니다", tt.subcommand)

			output := buf.String()
			assert.Contains(t, output, testFlowUUID,
				"출력에 flow ID 가 포함되어야 합니다")
		})
	}
}

// --- flow export 테스트 ---

// TestFlowExport_JSON - JSON 형식으로 플로우 내보내기 검증
func TestFlowExport_JSON(t *testing.T) {
	flow := map[string]any{
		"id":     testFlowUUID,
		"name":   "내보내기 테스트",
		"status": "running",
		"nodes":  []any{},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID, r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(flow))
	})

	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "exported.json")

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "export", testFlowUUID, "-o", outputFile})
	err := cmd.Execute()
	require.NoError(t, err, "flow export 실행 에러가 없어야 합니다")

	// 파일 존재 및 내용 검증
	data, err := os.ReadFile(outputFile)
	require.NoError(t, err, "내보내기 파일을 읽을 수 있어야 합니다")

	var exported map[string]any
	err = json.Unmarshal(data, &exported)
	require.NoError(t, err, "내보내기 파일이 유효한 JSON 이어야 합니다")
	assert.Equal(t, testFlowUUID, exported["id"], "내보내기된 플로우 ID 가 일치해야 합니다")

	output := buf.String()
	assert.Contains(t, output, outputFile, "출력에 내보내기 파일 경로가 포함되어야 합니다")
}

// TestFlowExport_YAML - YAML 형식으로 플로우 내보내기 검증
func TestFlowExport_YAML(t *testing.T) {
	flow := map[string]any{
		"id":     testFlowUUID,
		"name":   "YAML 내보내기",
		"status": "stopped",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(flow))
	})

	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "exported.yaml")

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "export", testFlowUUID, "-o", outputFile})
	err := cmd.Execute()
	require.NoError(t, err, "flow export (YAML) 실행 에러가 없어야 합니다")

	// 파일 존재 및 내용 검증
	data, err := os.ReadFile(outputFile)
	require.NoError(t, err, "YAML 내보내기 파일을 읽을 수 있어야 합니다")

	content := string(data)
	assert.Contains(t, content, testFlowUUID, "YAML 파일에 플로우 ID 가 포함되어야 합니다")
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
		"id":                 testFlowUUID,
		"state":              "running",
		"messages_processed": float64(1234),
		"errors":             float64(2),
		"uptime":             "2h30m",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID+"/status", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(status))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "status", testFlowUUID, "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "flow status 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "running", "출력에 상태가 포함되어야 합니다")
	assert.Contains(t, output, "1234", "출력에 처리된 메시지 수가 포함되어야 합니다")
}

// TestFlowStatus_TableFormat - 테이블 형식의 상태 출력 검증
func TestFlowStatus_TableFormat(t *testing.T) {
	status := map[string]any{
		"id":                 testFlowUUID,
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

	cmd.SetArgs([]string{"flow", "status", testFlowUUID})
	err := cmd.Execute()
	require.NoError(t, err, "flow status (table) 실행 에러가 없어야 합니다")

	output := buf.String()
	// 테이블 형식 키-값 출력 검증
	assert.Contains(t, output, testFlowUUID, "출력에 flow ID 가 포함되어야 합니다")
	assert.Contains(t, output, "running", "출력에 상태가 포함되어야 합니다")
}

// TestFlowStatus_WithNodeStats - node_stats 포함 시 미니 테이블 출력 검증
func TestFlowStatus_WithNodeStats(t *testing.T) {
	status := map[string]any{
		"status":        "running",
		"id":            testFlowUUID,
		"uptime":        "1m50s",
		"message_count": float64(55),
		"error_count":   float64(0),
		"node_stats": []any{
			map[string]any{"node_id": "aaa-111", "node_type": "filter", "errors": float64(0), "processed": float64(10)},
			map[string]any{"node_id": "bbb-222", "node_type": "transform", "errors": float64(1), "processed": float64(5)},
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(status))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "status", testFlowUUID})
	err := cmd.Execute()
	require.NoError(t, err, "flow status 실행 에러가 없어야 합니다")

	output := buf.String()
	// 스칼라 필드 라벨 검증
	assert.Contains(t, output, "Status:", "Status 라벨이 포함되어야 합니다")
	assert.Contains(t, output, "running", "상태 값이 포함되어야 합니다")
	assert.Contains(t, output, "Messages:", "Messages 라벨이 포함되어야 합니다")
	assert.Contains(t, output, "55", "메시지 수가 포함되어야 합니다")
	// node_stats 섹션 검증
	assert.Contains(t, output, "Node Stats:", "Node Stats 섹션 헤더가 포함되어야 합니다")
	assert.Contains(t, output, "NODE_TYPE", "미니 테이블 헤더가 포함되어야 합니다")
	assert.Contains(t, output, "filter", "노드 타입이 포함되어야 합니다")
	assert.Contains(t, output, "transform", "노드 타입이 포함되어야 합니다")
	// Go map 원시 표현이 없어야 함
	assert.NotContains(t, output, "map[", "Go map 원시 표현이 없어야 합니다")
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

	cmd.SetArgs([]string{"flow", "export", testFlowUUID})
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
		"deploy", "start", "stop", "restart", "undeploy",
		"export", "import", "status",
		"config", "subflow-stats", "tap", "taps", "node-configure",
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
		"id":     testFlowUUID,
		"name":   "텍스트 테스트",
		"status": "running",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(flow))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "get", testFlowUUID, "--format", "text"})
	err := cmd.Execute()
	require.NoError(t, err, "flow get --format text 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, testFlowUUID, "text 형식에 flow ID 가 포함되어야 합니다")
}

// --- flow lifecycle 메시지 검증 ---

// TestFlowDeploy_SuccessMessage - deploy 성공 메시지 검증
func TestFlowDeploy_SuccessMessage(t *testing.T) {
	result := map[string]any{
		"id":     testFlowUUID,
		"status": "deployed",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID+"/deploy", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(result))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "deploy", testFlowUUID})
	err := cmd.Execute()
	require.NoError(t, err, "flow deploy 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.NotEmpty(t, output, "deploy 성공 시 출력이 있어야 합니다")
	// 성공 메시지 또는 결과 데이터가 있어야 함
	assert.True(t,
		strings.Contains(output, testFlowUUID) || strings.Contains(output, "deploy"),
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

// --- resolveFlowID 테스트 ---

// TestResolveFlowID_UUIDPassthrough - UUID 입력 시 API 호출 없이 바로 반환
func TestResolveFlowID_UUIDPassthrough(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("UUID 패스스루 시 서버 요청이 없어야 합니다")
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	id, err := resolveFlowID(client, "f47ac10b-58cc-4372-a567-0e02b2c3d479")
	require.NoError(t, err)
	assert.Equal(t, "f47ac10b-58cc-4372-a567-0e02b2c3d479", id,
		"UUID 는 API 호출 없이 그대로 반환되어야 합니다")
}

// TestResolveFlowID_NameMatch - 이름으로 플로우 ID 해석 검증
func TestResolveFlowID_NameMatch(t *testing.T) {
	flows := []map[string]any{
		{"id": "aaaa1111-2222-3333-4444-555566667777", "name": "my-pipeline"},
		{"id": "bbbb1111-2222-3333-4444-555566667777", "name": "other-flow"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(flows))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	id, err := resolveFlowID(client, "my-pipeline")
	require.NoError(t, err)
	assert.Equal(t, "aaaa1111-2222-3333-4444-555566667777", id,
		"이름으로 매칭된 UUID 가 반환되어야 합니다")
}

// TestResolveFlowID_NoMatch - 매칭 없을 때 원본 반환 검증
func TestResolveFlowID_NoMatch(t *testing.T) {
	flows := []map[string]any{
		{"id": "aaaa1111-2222-3333-4444-555566667777", "name": "my-pipeline"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(flows))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	id, err := resolveFlowID(client, "nonexistent")
	require.NoError(t, err)
	assert.Equal(t, "nonexistent", id,
		"매칭 없으면 원본 문자열이 반환되어야 합니다")
}

// TestResolveFlowID_DuplicateName - 중복 이름 시 에러 검증
func TestResolveFlowID_DuplicateName(t *testing.T) {
	flows := []map[string]any{
		{"id": "aaaa1111-2222-3333-4444-555566667777", "name": "dup-name"},
		{"id": "bbbb1111-2222-3333-4444-555566667777", "name": "dup-name"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(flows))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	_, err := resolveFlowID(client, "dup-name")
	require.Error(t, err, "중복 이름은 에러를 반환해야 합니다")
	assert.Contains(t, err.Error(), "2", "에러 메시지에 중복 개수가 포함되어야 합니다")
}

// TestResolveFlowID_APIError - API 에러 시 원본 반환 검증
func TestResolveFlowID_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	id, err := resolveFlowID(client, "some-name")
	require.NoError(t, err)
	assert.Equal(t, "some-name", id,
		"API 에러 시 원본 문자열이 반환되어야 합니다")
}

// TestResolveFlowID_PaginatedResponse - 실제 서버의 페이지네이션 응답 형식 검증
func TestResolveFlowID_PaginatedResponse(t *testing.T) {
	flowUUID := "dddd1111-2222-3333-4444-555566667777"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 실제 서버의 NewPaginatedResponse 와 동일한 형식
		resp := map[string]any{
			"success": true,
			"data": []map[string]any{
				{
					"id":          flowUUID,
					"name":        "simple-pipeline",
					"description": "테스트 파이프라인",
					"status":      "stored",
					"node_count":  float64(3),
					"created_at":  "2026-01-01T00:00:00Z",
					"updated_at":  "2026-01-01T00:00:00Z",
				},
			},
			"meta": map[string]any{
				"pagination": map[string]any{
					"page":        1,
					"size":        20,
					"total":       1,
					"total_pages": 1,
				},
			},
		}
		b, _ := json.Marshal(resp)
		w.Write(b)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	id, err := resolveFlowID(client, "simple-pipeline")
	require.NoError(t, err)
	assert.Equal(t, flowUUID, id,
		"페이지네이션 응답에서도 이름 기반 ID 해석이 동작해야 합니다")
}

// TestFlowDeploy_ByName - 이름 기반 deploy E2E 검증
func TestFlowDeploy_ByName(t *testing.T) {
	flowUUID := "cccc1111-2222-3333-4444-555566667777"
	flows := []map[string]any{
		{"id": flowUUID, "name": "my-pipeline"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/flows":
			// resolveFlowID 의 이름 조회 요청
			w.Write(apiEnvelope(flows))
		case "/api/v1/flows/" + flowUUID + "/deploy":
			// deploy 요청
			assert.Equal(t, http.MethodPost, r.Method)
			w.Write(apiEnvelope(map[string]any{"id": flowUUID, "status": "deployed"}))
		default:
			t.Errorf("예상치 못한 경로: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "deploy", "my-pipeline"})
	err := cmd.Execute()
	require.NoError(t, err, "이름 기반 deploy 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.NotEmpty(t, output, "deploy 성공 시 출력이 있어야 합니다")
}

// --- 출력 형식 지원 전체 검증 ---

// --- 플로우 파일 형식 라운드트립 / 데이터 손실 회귀 방지 테스트 ---

// captureRequestBody 는 핸들러가 받은 마지막 요청의 JSON 바디를 디코딩하여 반환한다.
// 단일 POST 호출을 가정한다.
func captureRequestBody(t *testing.T, captured *map[string]any) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var body map[string]any
		require.NoError(t, json.Unmarshal(data, &body))
		*captured = body
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"id": "flow-x", "name": body["name"]}))
	}
}

// TestFlowImport_FlatLegacyFormat - 평면 레거시 형식({name, nodes, wires}) 가져오기 검증
// 레거시 계약 보존: body 전체를 definition 으로 취급하지만 name/description 은 top-level 로 분리한다.
func TestFlowImport_FlatLegacyFormat(t *testing.T) {
	flatFile := `{
		"name": "legacy-flow",
		"description": "flat format",
		"nodes": [{"id": "n1", "type": "mqtt-in"}, {"id": "n2", "type": "filter"}],
		"wires": [{"source_node_id": "n1", "target_node_id": "n2"}]
	}`

	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "legacy.json")
	require.NoError(t, os.WriteFile(flowFile, []byte(flatFile), 0644))

	var captured map[string]any
	_, cmd, cleanup := setupFlowTest(t, captureRequestBody(t, &captured))
	defer cleanup()

	cmd.SetArgs([]string{"flow", "import", "-f", flowFile, "--format", "json"})
	require.NoError(t, cmd.Execute(), "flow import (flat) 실행 에러가 없어야 합니다")

	assert.Equal(t, "legacy-flow", captured["name"], "top-level name 이 전달되어야 합니다")
	assert.Equal(t, "flat format", captured["description"], "top-level description 이 전달되어야 합니다")

	definition, ok := captured["definition"].(map[string]any)
	require.True(t, ok, "definition 이 map 이어야 합니다")

	nodes, ok := definition["nodes"].([]any)
	require.True(t, ok, "definition.nodes 가 배열이어야 합니다")
	assert.Len(t, nodes, 2, "definition.nodes 길이가 보존되어야 합니다")

	wires, ok := definition["wires"].([]any)
	require.True(t, ok, "definition.wires 가 배열이어야 합니다")
	assert.Len(t, wires, 1, "definition.wires 길이가 보존되어야 합니다")

	// 이중 래핑 방지: definition 안에 또 다른 definition 키가 있으면 안 된다
	_, hasNestedDef := definition["definition"]
	assert.False(t, hasNestedDef, "definition 내부에 nested definition 키가 없어야 합니다")
}

// TestFlowImport_ExportedNestedFormat - 서버 내보내기 형식({name, definition:{nodes, edges}}) 가져오기 검증
// 회귀 방지(데이터 손실): 내보내기 파일을 그대로 다시 import 했을 때 노드/엣지가 보존되어야 한다.
func TestFlowImport_ExportedNestedFormat(t *testing.T) {
	exportedFile := `{
		"name": "Capture",
		"description": "exported flow",
		"definition": {
			"nodes": [
				{"id": "n1"}, {"id": "n2"}, {"id": "n3"}, {"id": "n4"}, {"id": "n5"},
				{"id": "n6"}, {"id": "n7"}, {"id": "n8"}, {"id": "n9"}, {"id": "n10"},
				{"id": "n11"}, {"id": "n12"}, {"id": "n13"}, {"id": "n14"}, {"id": "n15"},
				{"id": "n16"}, {"id": "n17"}, {"id": "n18"}, {"id": "n19"}, {"id": "n20"}
			],
			"edges": [
				{"id": "e1"}, {"id": "e2"}, {"id": "e3"}, {"id": "e4"},
				{"id": "e5"}, {"id": "e6"}, {"id": "e7"}, {"id": "e8"},
				{"id": "e9"}, {"id": "e10"}, {"id": "e11"}, {"id": "e12"},
				{"id": "e13"}, {"id": "e14"}, {"id": "e15"}, {"id": "e16"}
			]
		}
	}`

	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "exported.json")
	require.NoError(t, os.WriteFile(flowFile, []byte(exportedFile), 0644))

	var captured map[string]any
	_, cmd, cleanup := setupFlowTest(t, captureRequestBody(t, &captured))
	defer cleanup()

	cmd.SetArgs([]string{"flow", "import", "-f", flowFile, "--format", "json"})
	require.NoError(t, cmd.Execute(), "flow import (exported) 실행 에러가 없어야 합니다")

	assert.Equal(t, "Capture", captured["name"], "top-level name 이 전달되어야 합니다")
	assert.Equal(t, "exported flow", captured["description"], "top-level description 이 전달되어야 합니다")

	definition, ok := captured["definition"].(map[string]any)
	require.True(t, ok, "definition 이 map 이어야 합니다")

	nodes, ok := definition["nodes"].([]any)
	require.True(t, ok, "definition.nodes 가 배열이어야 합니다")
	assert.Len(t, nodes, 20, "내보내기 파일의 20개 노드가 모두 보존되어야 합니다")

	edges, ok := definition["edges"].([]any)
	require.True(t, ok, "definition.edges 가 배열이어야 합니다")
	assert.Len(t, edges, 16, "내보내기 파일의 16개 엣지가 모두 보존되어야 합니다")

	// 이중 래핑 방지 핵심 검증
	_, hasNestedName := definition["name"]
	assert.False(t, hasNestedName, "definition 내부에 name 키가 없어야 합니다 (이중 래핑 방지)")
	_, hasNestedDef := definition["definition"]
	assert.False(t, hasNestedDef, "definition 내부에 nested definition 키가 없어야 합니다 (이중 래핑 방지)")
}

// TestFlowCreate_ExportedNestedFormat - flow create 도 동일하게 내보내기 형식 데이터 손실 방지 검증
func TestFlowCreate_ExportedNestedFormat(t *testing.T) {
	exportedFile := `{
		"name": "Capture",
		"definition": {
			"nodes": [{"id": "n1"}, {"id": "n2"}, {"id": "n3"}],
			"edges": [{"id": "e1"}, {"id": "e2"}]
		}
	}`

	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "exported.json")
	require.NoError(t, os.WriteFile(flowFile, []byte(exportedFile), 0644))

	var captured map[string]any
	_, cmd, cleanup := setupFlowTest(t, captureRequestBody(t, &captured))
	defer cleanup()

	cmd.SetArgs([]string{"flow", "create", "-f", flowFile, "--format", "json"})
	require.NoError(t, cmd.Execute(), "flow create (exported) 실행 에러가 없어야 합니다")

	assert.Equal(t, "Capture", captured["name"])

	definition, ok := captured["definition"].(map[string]any)
	require.True(t, ok, "definition 이 map 이어야 합니다")

	nodes, _ := definition["nodes"].([]any)
	edges, _ := definition["edges"].([]any)
	assert.Len(t, nodes, 3, "노드 3개가 보존되어야 합니다")
	assert.Len(t, edges, 2, "엣지 2개가 보존되어야 합니다")

	// 이중 래핑 방지
	_, hasNestedName := definition["name"]
	assert.False(t, hasNestedName, "definition 내부에 name 키가 없어야 합니다")
}

// TestFlowImport_RoundTrip - export 핸들러가 생성하는 형식을 import 가 손실 없이 받아들이는지 검증
// 서버의 Export 핸들러는 {name, description, definition:{nodes, edges, ...}, required_agents?} 를 생성한다.
func TestFlowImport_RoundTrip(t *testing.T) {
	// 서버 Export 형식을 그대로 모사
	exported := map[string]any{
		"name":        "round-trip-flow",
		"description": "exported from server",
		"definition": map[string]any{
			"nodes": []any{
				map[string]any{"id": "n1", "type": "mqtt-in"},
				map[string]any{"id": "n2", "type": "filter"},
				map[string]any{"id": "n3", "type": "mqtt-out"},
			},
			"edges": []any{
				map[string]any{"id": "e1", "source": "n1", "target": "n2"},
				map[string]any{"id": "e2", "source": "n2", "target": "n3"},
			},
		},
		"required_agents": []any{
			map[string]any{"name": "mqtt-broker", "type": "mqtt"},
		},
	}

	exportedBytes, err := json.Marshal(exported)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	flowFile := filepath.Join(tmpDir, "roundtrip.json")
	require.NoError(t, os.WriteFile(flowFile, exportedBytes, 0644))

	var captured map[string]any
	_, cmd, cleanup := setupFlowTest(t, captureRequestBody(t, &captured))
	defer cleanup()

	cmd.SetArgs([]string{"flow", "import", "-f", flowFile, "--format", "json"})
	require.NoError(t, cmd.Execute(), "round-trip import 실행 에러가 없어야 합니다")

	assert.Equal(t, "round-trip-flow", captured["name"])
	assert.Equal(t, "exported from server", captured["description"])

	definition, ok := captured["definition"].(map[string]any)
	require.True(t, ok)

	nodes, _ := definition["nodes"].([]any)
	edges, _ := definition["edges"].([]any)
	assert.Len(t, nodes, 3, "라운드트립 시 노드 수가 보존되어야 합니다")
	assert.Len(t, edges, 2, "라운드트립 시 엣지 수가 보존되어야 합니다")
}

// TestExtractDefinition_Unit - extractDefinition 헬퍼의 단위 테스트
func TestExtractDefinition_Unit(t *testing.T) {
	t.Run("exported_format", func(t *testing.T) {
		body := map[string]any{
			"name":        "f1",
			"description": "d1",
			"definition": map[string]any{
				"nodes": []any{1, 2, 3},
			},
		}
		name, desc, def := extractDefinition(body)
		assert.Equal(t, "f1", name)
		assert.Equal(t, "d1", desc)
		require.NotNil(t, def)
		nodes, _ := def["nodes"].([]any)
		assert.Len(t, nodes, 3)
		_, hasName := def["name"]
		assert.False(t, hasName, "정의 내부에 name 키가 없어야 합니다")
	})

	t.Run("flat_legacy_format", func(t *testing.T) {
		body := map[string]any{
			"name":  "f2",
			"nodes": []any{1, 2},
			"wires": []any{1},
		}
		name, desc, def := extractDefinition(body)
		assert.Equal(t, "f2", name)
		assert.Equal(t, "", desc)
		require.NotNil(t, def)
		nodes, _ := def["nodes"].([]any)
		wires, _ := def["wires"].([]any)
		assert.Len(t, nodes, 2)
		assert.Len(t, wires, 1)
		_, hasName := def["name"]
		assert.False(t, hasName, "name 은 definition 에서 분리되어야 합니다")
	})

	t.Run("empty_body", func(t *testing.T) {
		body := map[string]any{}
		name, desc, def := extractDefinition(body)
		assert.Equal(t, "", name)
		assert.Equal(t, "", desc)
		require.NotNil(t, def)
		assert.Empty(t, def)
	})

	t.Run("definition_wins_over_flat", func(t *testing.T) {
		// 두 형식이 모두 있으면 export 형식(definition 키)이 우선한다 (결정적 규칙)
		body := map[string]any{
			"name":  "f3",
			"nodes": []any{1, 2, 3, 4, 5}, // 무시되어야 함
			"definition": map[string]any{
				"nodes": []any{1, 2},
			},
		}
		_, _, def := extractDefinition(body)
		require.NotNil(t, def)
		nodes, _ := def["nodes"].([]any)
		assert.Len(t, nodes, 2, "definition 키가 있으면 top-level nodes 는 무시되어야 합니다")
	})

	t.Run("definition_not_a_map_falls_back_to_flat", func(t *testing.T) {
		// definition 값이 map 이 아니면 flat 으로 폴백
		body := map[string]any{
			"name":       "f4",
			"definition": "not-a-map",
			"nodes":      []any{1},
		}
		_, _, def := extractDefinition(body)
		require.NotNil(t, def)
		nodes, _ := def["nodes"].([]any)
		assert.Len(t, nodes, 1, "definition 이 map 이 아니면 flat 형식으로 처리해야 합니다")
	})
}

// --- flow undeploy 테스트 ---

// TestFlowUndeploy - undeploy 서브커맨드 경로/메서드 검증
func TestFlowUndeploy(t *testing.T) {
	result := map[string]any{
		"id":     testFlowUUID,
		"status": "undeployed",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID+"/undeploy", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(result))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "undeploy", testFlowUUID, "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "flow undeploy 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, testFlowUUID, "출력에 flow ID 가 포함되어야 합니다")
}

// TestFlowUndeploy_ByName - 이름 기반 undeploy 검증 (id|name resolve)
func TestFlowUndeploy_ByName(t *testing.T) {
	flowUUID := "eeee1111-2222-3333-4444-555566667777"
	flows := []map[string]any{
		{"id": flowUUID, "name": "my-pipeline"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/flows":
			w.Write(apiEnvelope(flows))
		case "/api/v1/flows/" + flowUUID + "/undeploy":
			assert.Equal(t, http.MethodPost, r.Method)
			w.Write(apiEnvelope(map[string]any{"id": flowUUID, "status": "undeployed"}))
		default:
			t.Errorf("예상치 못한 경로: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "undeploy", "my-pipeline"})
	err := cmd.Execute()
	require.NoError(t, err, "이름 기반 undeploy 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.NotEmpty(t, output, "undeploy 성공 시 출력이 있어야 합니다")
}

// --- flow config 테스트 ---

// TestFlowConfig_KeyValue - key=value 인자로 플로우 구성 수정 검증
func TestFlowConfig_KeyValue(t *testing.T) {
	var captured map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID+"/config", r.URL.Path)
		assert.Equal(t, http.MethodPut, r.Method)
		data, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(data, &captured))
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"id": testFlowUUID, "status": "configured"}))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "config", testFlowUUID, "log_level=debug", "max_retries=3"})
	err := cmd.Execute()
	require.NoError(t, err, "flow config (key=value) 실행 에러가 없어야 합니다")

	// 요청 본문은 {"config": {...}} 형식이어야 한다
	config, ok := captured["config"].(map[string]any)
	require.True(t, ok, "요청 본문에 config 키가 있어야 합니다")
	assert.Equal(t, "debug", config["log_level"], "log_level 값이 전달되어야 합니다")
	assert.Equal(t, float64(3), config["max_retries"], "max_retries 정수 값이 전달되어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "구성", "출력에 구성 수정 메시지가 포함되어야 합니다")
}

// TestFlowConfig_File - 파일(-f)로 플로우 구성 수정 검증
func TestFlowConfig_File(t *testing.T) {
	var captured map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID+"/config", r.URL.Path)
		assert.Equal(t, http.MethodPut, r.Method)
		data, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(data, &captured))
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"id": testFlowUUID, "status": "configured"}))
	})

	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.json")
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"log_level": "info", "buffer_size": 100}`), 0644))

	_, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "config", testFlowUUID, "-f", cfgFile})
	err := cmd.Execute()
	require.NoError(t, err, "flow config (-f) 실행 에러가 없어야 합니다")

	config, ok := captured["config"].(map[string]any)
	require.True(t, ok, "요청 본문에 config 키가 있어야 합니다")
	assert.Equal(t, "info", config["log_level"], "파일의 log_level 이 전달되어야 합니다")
	assert.Equal(t, float64(100), config["buffer_size"], "파일의 buffer_size 가 전달되어야 합니다")
}

// TestFlowConfig_NoValue - 설정값 없이 호출 시 에러 검증
func TestFlowConfig_NoValue(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("설정값 없으면 서버에 요청하지 않아야 합니다")
	})

	_, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "config", testFlowUUID})
	err := cmd.Execute()
	require.Error(t, err, "설정값 누락 시 에러를 반환해야 합니다")
}

// TestFlowConfig_InvalidKeyValue - 잘못된 key=value 형식 에러 검증
func TestFlowConfig_InvalidKeyValue(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("잘못된 형식이면 서버에 요청하지 않아야 합니다")
	})

	_, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "config", testFlowUUID, "invalid-no-equals"})
	err := cmd.Execute()
	require.Error(t, err, "key=value 형식이 아니면 에러를 반환해야 합니다")
}

// --- flow subflow-stats 테스트 ---

// TestFlowSubflowStats - 서브플로우 통계 조회 검증 (JSON)
func TestFlowSubflowStats(t *testing.T) {
	stats := map[string]any{
		"flow_id": testFlowUUID,
		"nodes": []any{
			map[string]any{"node_id": "n1", "name": "필터", "type": "filter", "processed": float64(10), "errors": float64(0)},
			map[string]any{"node_id": "n2", "name": "변환", "type": "transform", "processed": float64(5), "errors": float64(1)},
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID+"/subflow-stats", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(stats))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "subflow-stats", testFlowUUID, "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "flow subflow-stats 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, testFlowUUID, "출력에 flow ID 가 포함되어야 합니다")
	assert.Contains(t, output, "filter", "출력에 노드 타입이 포함되어야 합니다")
}

// TestFlowSubflowStats_TableFormat - 서브플로우 통계 테이블 출력 검증
func TestFlowSubflowStats_TableFormat(t *testing.T) {
	stats := map[string]any{
		"flow_id": testFlowUUID,
		"nodes":   []any{},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(stats))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "subflow-stats", testFlowUUID})
	err := cmd.Execute()
	require.NoError(t, err, "flow subflow-stats (table) 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, testFlowUUID, "출력에 flow ID 가 포함되어야 합니다")
}

// --- flow tap 테스트 ---

// TestFlowTap_Enable - tap 활성화(기본값) 검증
func TestFlowTap_Enable(t *testing.T) {
	const nodeID = "node-abc"
	var captured map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID+"/nodes/"+nodeID+"/tap", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		data, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(data, &captured))
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"flow_id": testFlowUUID, "node_id": nodeID, "enabled": true}))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "tap", testFlowUUID, nodeID})
	err := cmd.Execute()
	require.NoError(t, err, "flow tap 실행 에러가 없어야 합니다")

	// 기본값은 enabled=true 여야 한다
	assert.Equal(t, true, captured["enabled"], "기본값으로 tap 이 활성화되어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "활성화", "출력에 tap 활성화 메시지가 포함되어야 합니다")
}

// TestFlowTap_Disable - --disable 플래그로 tap 비활성화 검증
func TestFlowTap_Disable(t *testing.T) {
	const nodeID = "node-abc"
	var captured map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID+"/nodes/"+nodeID+"/tap", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		data, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(data, &captured))
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"flow_id": testFlowUUID, "node_id": nodeID, "enabled": false}))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "tap", testFlowUUID, nodeID, "--disable"})
	err := cmd.Execute()
	require.NoError(t, err, "flow tap --disable 실행 에러가 없어야 합니다")

	assert.Equal(t, false, captured["enabled"], "--disable 시 tap 이 비활성화되어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "비활성화", "출력에 tap 비활성화 메시지가 포함되어야 합니다")
}

// TestFlowTap_MissingNodeID - nodeID 인자 누락 시 에러 검증
func TestFlowTap_MissingNodeID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("인자 누락 시 서버 요청이 없어야 합니다")
	})

	_, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "tap", testFlowUUID})
	err := cmd.Execute()
	require.Error(t, err, "nodeID 인자 누락 시 에러를 반환해야 합니다")
}

// --- flow taps 테스트 ---

// TestFlowTaps - 활성 탭 목록 조회 검증 (JSON)
func TestFlowTaps(t *testing.T) {
	taps := map[string]any{
		"flow_id":  testFlowUUID,
		"node_ids": []any{"node-1", "node-2"},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID+"/taps", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(taps))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "taps", testFlowUUID, "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "flow taps 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "node-1", "출력에 tap 중인 노드가 포함되어야 합니다")
	assert.Contains(t, output, "node-2", "출력에 tap 중인 노드가 포함되어야 합니다")
}

// TestFlowTaps_TableFormat - 활성 탭 목록 테이블 출력 검증
func TestFlowTaps_TableFormat(t *testing.T) {
	taps := map[string]any{
		"flow_id":  testFlowUUID,
		"node_ids": []any{},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(taps))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "taps", testFlowUUID})
	err := cmd.Execute()
	require.NoError(t, err, "flow taps (table) 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, testFlowUUID, "출력에 flow ID 가 포함되어야 합니다")
}

// --- flow node-configure 테스트 ---

// TestFlowNodeConfigure_KeyValue - key=value 인자로 노드 구성 수정 검증
func TestFlowNodeConfigure_KeyValue(t *testing.T) {
	const nodeID = "node-xyz"
	var captured map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID+"/nodes/"+nodeID+"/configure", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		data, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(data, &captured))
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"id": testFlowUUID, "node_id": nodeID, "status": "configured"}))
	})

	buf, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "node-configure", testFlowUUID, nodeID, "output_enabled=false"})
	err := cmd.Execute()
	require.NoError(t, err, "flow node-configure (key=value) 실행 에러가 없어야 합니다")

	// 요청 본문은 {"config": {...}} 형식이어야 한다
	config, ok := captured["config"].(map[string]any)
	require.True(t, ok, "요청 본문에 config 키가 있어야 합니다")
	assert.Equal(t, false, config["output_enabled"], "output_enabled bool 값이 전달되어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "구성", "출력에 구성 수정 메시지가 포함되어야 합니다")
}

// TestFlowNodeConfigure_File - 파일(-f)로 노드 구성 수정 검증
func TestFlowNodeConfigure_File(t *testing.T) {
	const nodeID = "node-xyz"
	var captured map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/flows/"+testFlowUUID+"/nodes/"+nodeID+"/configure", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		data, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(data, &captured))
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"id": testFlowUUID, "node_id": nodeID, "status": "configured"}))
	})

	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "node.json")
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"threshold": 50}`), 0644))

	_, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "node-configure", testFlowUUID, nodeID, "-f", cfgFile})
	err := cmd.Execute()
	require.NoError(t, err, "flow node-configure (-f) 실행 에러가 없어야 합니다")

	config, ok := captured["config"].(map[string]any)
	require.True(t, ok, "요청 본문에 config 키가 있어야 합니다")
	assert.Equal(t, float64(50), config["threshold"], "파일의 threshold 가 전달되어야 합니다")
}

// TestFlowNodeConfigure_MissingNodeID - nodeID 인자 누락 시 에러 검증
func TestFlowNodeConfigure_MissingNodeID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("인자 누락 시 서버 요청이 없어야 합니다")
	})

	_, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "node-configure", testFlowUUID})
	err := cmd.Execute()
	require.Error(t, err, "nodeID 인자 누락 시 에러를 반환해야 합니다")
}

// TestFlowNodeConfigure_NoValue - 설정값 없이 호출 시 에러 검증
func TestFlowNodeConfigure_NoValue(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("설정값 없으면 서버에 요청하지 않아야 합니다")
	})

	_, cmd, cleanup := setupFlowTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"flow", "node-configure", testFlowUUID, "node-xyz"})
	err := cmd.Execute()
	require.Error(t, err, "설정값 누락 시 에러를 반환해야 합니다")
}

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
