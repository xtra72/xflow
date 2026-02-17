package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- 테스트 헬퍼 ---

// setupStatusTest 는 status 커맨드 테스트를 위한 공통 설정을 수행한다.
// httptest 서버, 루트 커맨드, 출력 버퍼, 정리 함수를 반환한다.
func setupStatusTest(t *testing.T, handler http.HandlerFunc) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()

	server := httptest.NewServer(handler)

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	clientPtr := &client

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.AddCommand(newStatusCmd(clientPtr))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return &buf, rootCmd, server.Close
}

// --- xflow status 테스트 (서버 상태 조회) ---

// TestStatus - 서버 상태 조회 및 텍스트 출력 검증 (REQ-CLI-001-08-01)
func TestStatus(t *testing.T) {
	statusData := map[string]any{
		"version":       "1.2.3",
		"uptime":        "3h25m",
		"running_flows": float64(5),
		"active_agents": float64(3),
		"cpu_usage":     "45.2%",
		"memory_usage":  "1.2GB",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/status", r.URL.Path,
			"요청 경로가 /api/v1/status 여야 합니다")
		assert.Equal(t, http.MethodGet, r.Method,
			"요청 메서드가 GET 이어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(statusData))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status"})
	err := cmd.Execute()
	require.NoError(t, err, "status 실행 에러가 없어야 합니다")

	output := buf.String()
	// 키-값 형식의 텍스트 출력 검증
	assert.Contains(t, output, "1.2.3", "출력에 서버 버전이 포함되어야 합니다")
	assert.Contains(t, output, "3h25m", "출력에 업타임이 포함되어야 합니다")
	assert.Contains(t, output, "5", "출력에 실행 중인 플로우 수가 포함되어야 합니다")
	assert.Contains(t, output, "3", "출력에 활성 에이전트 수가 포함되어야 합니다")
	assert.Contains(t, output, "45.2%", "출력에 CPU 사용량이 포함되어야 합니다")
	assert.Contains(t, output, "1.2GB", "출력에 메모리 사용량이 포함되어야 합니다")
}

// TestStatus_JSONFormat - JSON 출력 형식 검증
func TestStatus_JSONFormat(t *testing.T) {
	statusData := map[string]any{
		"version":       "1.2.3",
		"uptime":        "3h25m",
		"running_flows": float64(5),
		"active_agents": float64(3),
		"cpu_usage":     "45.2%",
		"memory_usage":  "1.2GB",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(statusData))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "status --format json 실행 에러가 없어야 합니다")

	output := buf.String()
	// JSON 파싱 가능 여부 검증
	var result map[string]any
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, "1.2.3", result["version"], "JSON 에 version 이 포함되어야 합니다")
}

// TestStatus_YAMLFormat - YAML 출력 형식 검증
func TestStatus_YAMLFormat(t *testing.T) {
	statusData := map[string]any{
		"version": "1.2.3",
		"uptime":  "3h25m",
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(statusData))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "--format", "yaml"})
	err := cmd.Execute()
	require.NoError(t, err, "status --format yaml 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "version", "YAML 출력에 version 키가 포함되어야 합니다")
	assert.Contains(t, output, "1.2.3", "YAML 출력에 버전 값이 포함되어야 합니다")
}

// TestStatus_APIError - API 에러 전파 검증
func TestStatus_APIError(t *testing.T) {
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

	_, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status"})
	err := cmd.Execute()
	require.Error(t, err, "API 에러가 전파되어야 합니다")
}

// --- xflow status logs <component> 테스트 (로그 스트리밍) ---

// TestStatusLogs - 컴포넌트 로그 조회 검증 (REQ-CLI-001-08-02)
func TestStatusLogs(t *testing.T) {
	logData := map[string]any{
		"component": "scheduler",
		"lines": []any{
			"2026-02-17T10:00:00Z [INFO] 스케줄러 시작",
			"2026-02-17T10:00:01Z [INFO] 작업 큐 초기화 완료",
			"2026-02-17T10:00:05Z [WARN] 큐 깊이 임계값 초과",
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/logs", r.URL.Path,
			"요청 경로가 /api/v1/logs 여야 합니다")
		assert.Equal(t, http.MethodGet, r.Method,
			"요청 메서드가 GET 이어야 합니다")
		assert.Equal(t, "scheduler", r.URL.Query().Get("component"),
			"쿼리 파라미터에 component=scheduler 가 있어야 합니다")
		assert.Equal(t, "50", r.URL.Query().Get("lines"),
			"기본 lines 값이 50 이어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(logData))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "logs", "scheduler"})
	err := cmd.Execute()
	require.NoError(t, err, "status logs 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "스케줄러 시작", "출력에 로그 내용이 포함되어야 합니다")
	assert.Contains(t, output, "큐 깊이", "출력에 경고 로그가 포함되어야 합니다")
}

// TestStatusLogs_WithLinesFlag - --lines 플래그 검증
func TestStatusLogs_WithLinesFlag(t *testing.T) {
	logData := map[string]any{
		"component": "worker",
		"lines": []any{
			"2026-02-17T10:00:00Z [INFO] 로그 항목 1",
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "100", r.URL.Query().Get("lines"),
			"--lines 100 이 쿼리 파라미터에 반영되어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(logData))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "logs", "worker", "--lines", "100"})
	err := cmd.Execute()
	require.NoError(t, err, "status logs --lines 100 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.NotEmpty(t, output, "로그 출력이 비어있으면 안됩니다")
}

// TestStatusLogs_FollowFlag - --follow 플래그 경고 메시지 검증
func TestStatusLogs_FollowFlag(t *testing.T) {
	logData := map[string]any{
		"component": "api",
		"lines": []any{
			"2026-02-17T10:00:00Z [INFO] API 서버 시작",
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(logData))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "logs", "api", "--follow"})
	err := cmd.Execute()
	require.NoError(t, err, "status logs --follow 실행 에러가 없어야 합니다")

	output := buf.String()
	// --follow 는 아직 미지원이므로 경고 메시지가 출력되어야 함
	assert.Contains(t, output, "실시간 스트리밍은 아직 지원되지 않습니다",
		"--follow 시 미지원 경고 메시지가 출력되어야 합니다")
}

// TestStatusLogs_MissingComponent - 컴포넌트 인자 누락 시 에러 검증
func TestStatusLogs_MissingComponent(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("컴포넌트 인자 누락 시 서버에 요청하면 안됩니다")
	})

	_, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "logs"})
	err := cmd.Execute()
	require.Error(t, err, "컴포넌트 인자 누락 시 에러를 반환해야 합니다")
}

// TestStatusLogs_JSONFormat - JSON 출력 형식 검증
func TestStatusLogs_JSONFormat(t *testing.T) {
	logData := map[string]any{
		"component": "scheduler",
		"lines": []any{
			"2026-02-17T10:00:00Z [INFO] 로그 1",
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(logData))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "logs", "scheduler", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "status logs --format json 실행 에러가 없어야 합니다")

	output := buf.String()
	var result map[string]any
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, "scheduler", result["component"],
		"JSON 에 component 가 포함되어야 합니다")
}

// TestStatusLogs_APIError - 로그 조회 API 에러 전파 검증
func TestStatusLogs_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		resp := map[string]any{
			"success": false,
			"error": map[string]any{
				"code":    "NOT_FOUND",
				"message": "컴포넌트를 찾을 수 없습니다",
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	_, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "logs", "nonexistent"})
	err := cmd.Execute()
	require.Error(t, err, "존재하지 않는 컴포넌트 조회 시 에러를 반환해야 합니다")
}

// --- xflow status metrics 테스트 (메트릭스 요약) ---

// TestStatusMetrics - 메트릭스 요약 조회 검증 (REQ-CLI-001-08-03)
func TestStatusMetrics(t *testing.T) {
	metricsData := map[string]any{
		"total_flows":          float64(15),
		"running_flows":        float64(8),
		"total_agents":         float64(10),
		"active_agents":        float64(6),
		"messages_per_second":  float64(1250),
		"avg_latency_ms":       float64(45.3),
		"errors_last_hour":     float64(3),
		"uptime_hours":         float64(72.5),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/metrics", r.URL.Path,
			"요청 경로가 /api/v1/metrics 여야 합니다")
		assert.Equal(t, http.MethodGet, r.Method,
			"요청 메서드가 GET 이어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(metricsData))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "metrics"})
	err := cmd.Execute()
	require.NoError(t, err, "status metrics 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "15", "출력에 총 플로우 수가 포함되어야 합니다")
	assert.Contains(t, output, "1250", "출력에 초당 메시지 수가 포함되어야 합니다")
	assert.Contains(t, output, "45.3", "출력에 평균 레이턴시가 포함되어야 합니다")
}

// TestStatusMetrics_JSONFormat - 메트릭스 JSON 출력 형식 검증
func TestStatusMetrics_JSONFormat(t *testing.T) {
	metricsData := map[string]any{
		"total_flows":   float64(15),
		"running_flows": float64(8),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(metricsData))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "metrics", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "status metrics --format json 실행 에러가 없어야 합니다")

	output := buf.String()
	var result map[string]any
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, float64(15), result["total_flows"],
		"JSON 에 total_flows 가 포함되어야 합니다")
}

// TestStatusMetrics_APIError - 메트릭스 API 에러 전파 검증
func TestStatusMetrics_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		resp := map[string]any{
			"success": false,
			"error": map[string]any{
				"code":    "SERVICE_UNAVAILABLE",
				"message": "메트릭스 서비스를 사용할 수 없습니다",
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	_, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "metrics"})
	err := cmd.Execute()
	require.Error(t, err, "메트릭스 서비스 에러가 전파되어야 합니다")
}

// --- 서브커맨드 등록 검증 ---

// TestStatusSubcommands - status 커맨드와 서브커맨드 등록 여부 검증
func TestStatusSubcommands(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)
	clientPtr := &client

	statusCmd := newStatusCmd(clientPtr)

	// status 커맨드 자체가 RunE 를 가지고 있어야 함
	require.NotNil(t, statusCmd, "newStatusCmd 가 nil 을 반환하면 안됩니다")
	assert.Equal(t, "status", statusCmd.Use, "커맨드 Use 가 'status' 여야 합니다")
	assert.NotNil(t, statusCmd.RunE, "status 커맨드는 RunE 를 가져야 합니다")

	// 서브커맨드 검증: logs, metrics
	expectedSubs := []string{"logs", "metrics"}
	subNames := make(map[string]bool)
	for _, sub := range statusCmd.Commands() {
		parts := splitFirst(sub.Use)
		subNames[parts] = true
	}

	for _, expected := range expectedSubs {
		assert.True(t, subNames[expected],
			"status 에 '%s' 서브커맨드가 등록되어 있어야 합니다", expected)
	}
}

// TestNewStatusCmd_Signature - newStatusCmd 함수 시그니처 검증
func TestNewStatusCmd_Signature(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)
	clientPtr := &client

	statusCmd := newStatusCmd(clientPtr)
	require.NotNil(t, statusCmd, "newStatusCmd 가 nil 을 반환하면 안됩니다")
	assert.Equal(t, "status", statusCmd.Use, "커맨드 Use 가 'status' 여야 합니다")
}

// --- 출력 형식 전체 검증 ---

// TestStatus_AllFormats - status 의 4가지 출력 형식 검증
func TestStatus_AllFormats(t *testing.T) {
	statusData := map[string]any{
		"version": "1.0.0",
		"uptime":  "1h",
	}

	formats := []string{"json", "yaml", "table", "text"}

	for _, format := range formats {
		t.Run(fmt.Sprintf("format_%s", format), func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(apiEnvelope(statusData))
			})

			buf, cmd, cleanup := setupStatusTest(t, handler)
			defer cleanup()

			cmd.SetArgs([]string{"status", "--format", format})
			err := cmd.Execute()
			require.NoError(t, err,
				"status --format %s 실행 에러가 없어야 합니다", format)

			output := buf.String()
			assert.NotEmpty(t, output,
				"format=%s 에서 출력이 비어있으면 안됩니다", format)
		})
	}
}

// splitFirst 는 문자열의 첫 번째 공백 구분 토큰을 반환하는 헬퍼이다.
func splitFirst(s string) string {
	for i, c := range s {
		if c == ' ' {
			return s[:i]
		}
	}
	return s
}

// --- xflow status logs: --follow 와 --lines 동시 사용 테스트 ---

// TestStatusLogs_FollowWithLines - --follow 와 --lines 동시 사용 검증
func TestStatusLogs_FollowWithLines(t *testing.T) {
	logData := map[string]any{
		"component": "worker",
		"lines": []any{
			"2026-02-17T10:00:00Z [INFO] 시작",
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "25", r.URL.Query().Get("lines"),
			"--lines 25 가 반영되어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(logData))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "logs", "worker", "--follow", "--lines", "25"})
	err := cmd.Execute()
	require.NoError(t, err, "--follow --lines 동시 사용 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "실시간 스트리밍은 아직 지원되지 않습니다",
		"--follow 경고 메시지가 출력되어야 합니다")
}
