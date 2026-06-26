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

// apiEnvelopeWithMeta 는 페이지네이션 메타를 포함한 응답 엔벨로프를 생성한다.
func apiEnvelopeWithMeta(data any, total int64) []byte {
	resp := map[string]any{
		"success": true,
		"data":    data,
		"meta": map[string]any{
			"pagination": map[string]any{
				"page":        1,
				"size":        100,
				"total":       total,
				"total_pages": 1,
			},
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

// statusMux 는 status 커맨드가 호출하는 실제 엔드포인트들을 모킹한다.
// version/metrics/flows/agents/health 응답을 라우팅한다.
func statusMux(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/system/version":
			w.Write(apiEnvelope(map[string]any{
				"version":        "v1.2.3",
				"commit":         "abc1234",
				"build_date":     "2026-01-01",
				"go_version":     "go1.25.0",
				"channel":        "stable",
				"os":             "linux",
				"arch":           "amd64",
				"hostname":       "node-1",
				"mode":           "server",
				"uptime_seconds": float64(12345),
			}))
		case "/api/v1/monitor/metrics":
			w.Write(apiEnvelope(map[string]any{
				"cpu_usage_percent":    float64(0),
				"memory_usage_percent": float64(42.5),
				"go_routines":          float64(25),
				"go_mem_alloc_mb":      float64(12.3),
				"go_mem_sys_mb":        float64(48.0),
				"uptime_seconds":       float64(12345),
			}))
		case "/api/v1/flows":
			w.Write(apiEnvelopeWithMeta([]map[string]any{{"id": "f1"}, {"id": "f2"}}, 5))
		case "/api/v1/agents":
			w.Write(apiEnvelopeWithMeta([]map[string]any{{"id": "a1"}}, 3))
		case "/health":
			w.Write(apiEnvelope(map[string]any{"status": "ok"}))
		default:
			t.Errorf("예상치 못한 경로 호출: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// --- xflow status 테스트 (서버 상태 조회 재배선) ---

// TestStatus - 가용 엔드포인트 조합으로 서버 상태 요약 검증
func TestStatus(t *testing.T) {
	calledPaths := make(map[string]bool)
	handler := func(w http.ResponseWriter, r *http.Request) {
		calledPaths[r.URL.Path] = true
		statusMux(t)(w, r)
	}

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status"})
	err := cmd.Execute()
	require.NoError(t, err, "status 실행 에러가 없어야 합니다")

	// 죽은 경로(/api/v1/status)가 아닌 실제 엔드포인트를 호출해야 한다
	assert.True(t, calledPaths["/api/v1/system/version"], "/api/v1/system/version 을 호출해야 합니다")
	assert.True(t, calledPaths["/api/v1/monitor/metrics"], "/api/v1/monitor/metrics 를 호출해야 합니다")
	assert.True(t, calledPaths["/api/v1/flows"], "/api/v1/flows 를 호출해야 합니다")
	assert.True(t, calledPaths["/api/v1/agents"], "/api/v1/agents 를 호출해야 합니다")
	assert.False(t, calledPaths["/api/v1/status"], "죽은 경로 /api/v1/status 를 호출하면 안됩니다")

	output := buf.String()
	assert.Contains(t, output, "v1.2.3", "출력에 서버 버전이 포함되어야 합니다")
	assert.Contains(t, output, "node-1", "출력에 hostname 이 포함되어야 합니다")
	assert.Contains(t, output, "5", "출력에 총 플로우 수(5)가 포함되어야 합니다")
	assert.Contains(t, output, "3", "출력에 총 에이전트 수(3)가 포함되어야 합니다")
	assert.Contains(t, output, "ok", "출력에 연결 상태가 포함되어야 합니다")
}

// TestStatus_JSONFormat - JSON 출력 형식 검증
func TestStatus_JSONFormat(t *testing.T) {
	buf, cmd, cleanup := setupStatusTest(t, statusMux(t))
	defer cleanup()

	cmd.SetArgs([]string{"status", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "status --format json 실행 에러가 없어야 합니다")

	output := buf.String()
	var result map[string]any
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, "v1.2.3", result["version"], "JSON 에 version 이 포함되어야 합니다")
	assert.Equal(t, float64(5), result["total_flows"], "JSON 에 total_flows 가 포함되어야 합니다")
}

// TestStatus_YAMLFormat - YAML 출력 형식 검증
func TestStatus_YAMLFormat(t *testing.T) {
	buf, cmd, cleanup := setupStatusTest(t, statusMux(t))
	defer cleanup()

	cmd.SetArgs([]string{"status", "--format", "yaml"})
	err := cmd.Execute()
	require.NoError(t, err, "status --format yaml 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "version", "YAML 출력에 version 키가 포함되어야 합니다")
	assert.Contains(t, output, "v1.2.3", "YAML 출력에 버전 값이 포함되어야 합니다")
}

// TestStatus_PartialFailure - 일부 엔드포인트 실패 시에도 상태 조회가 계속되는지 검증
func TestStatus_PartialFailure(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/system/version":
			// version 만 성공
			w.Write(apiEnvelope(map[string]any{"version": "v9.9.9", "hostname": "h"}))
		case "/health":
			w.Write(apiEnvelope(map[string]any{"status": "ok"}))
		default:
			// metrics/flows/agents 는 500 실패
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   map[string]any{"code": "ERR", "message": "fail"},
			})
		}
	}

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status"})
	err := cmd.Execute()
	require.NoError(t, err, "부분 실패 시에도 status 는 성공해야 합니다")

	output := buf.String()
	assert.Contains(t, output, "v9.9.9", "성공한 version 필드는 출력되어야 합니다")
}

// TestStatus_ConnectionUnreachable - health 실패 시 연결 상태 표시 검증
func TestStatus_ConnectionUnreachable(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		statusMux(t)(w, r)
	}

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status"})
	err := cmd.Execute()
	require.NoError(t, err, "health 실패 시에도 status 는 성공해야 합니다")

	output := buf.String()
	assert.Contains(t, output, "unreachable", "연결 불가 시 unreachable 이 표시되어야 합니다")
}

// --- xflow status logs [component] 테스트 (로그 레벨 조회 재정의) ---

// loglevelData 는 로그 레벨 조회 응답 데이터이다.
func loglevelData() map[string]any {
	return map[string]any{
		"default_level": "info",
		"components": map[string]any{
			"scheduler": "debug",
			"api":       "warn",
		},
	}
}

// TestStatusLogs - 전역/컴포넌트별 로그 레벨 조회 검증
func TestStatusLogs(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/monitor/loglevel", r.URL.Path,
			"요청 경로가 /api/v1/monitor/loglevel 여야 합니다")
		assert.Equal(t, http.MethodGet, r.Method, "요청 메서드가 GET 이어야 합니다")
		assert.NotEqual(t, "/api/v1/logs", r.URL.Path, "죽은 경로 /api/v1/logs 를 호출하면 안됩니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(loglevelData()))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "logs"})
	err := cmd.Execute()
	require.NoError(t, err, "status logs 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "info", "출력에 기본 로그 레벨이 포함되어야 합니다")
	assert.Contains(t, output, "scheduler", "출력에 컴포넌트 이름이 포함되어야 합니다")
	assert.Contains(t, output, "debug", "출력에 컴포넌트 레벨이 포함되어야 합니다")
}

// TestStatusLogs_WithComponent - 특정 컴포넌트 레벨 필터링 검증
func TestStatusLogs_WithComponent(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/monitor/loglevel", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(loglevelData()))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "logs", "scheduler"})
	err := cmd.Execute()
	require.NoError(t, err, "status logs scheduler 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "scheduler", "출력에 조회한 컴포넌트가 포함되어야 합니다")
	assert.Contains(t, output, "debug", "scheduler 의 레벨(debug)이 포함되어야 합니다")
}

// TestStatusLogs_UnknownComponent - 오버라이드 없는 컴포넌트는 기본 레벨 안내 검증
func TestStatusLogs_UnknownComponent(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(loglevelData()))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "logs", "nonexistent"})
	err := cmd.Execute()
	require.NoError(t, err, "오버라이드 없는 컴포넌트 조회는 에러가 아닙니다")

	output := buf.String()
	assert.Contains(t, output, "info", "기본 레벨(info)이 적용되어야 합니다")
	assert.Contains(t, output, "오버라이드 없음", "기본 레벨 적용 안내가 포함되어야 합니다")
}

// TestStatusLogs_FollowFlag - --follow 플래그 미지원 안내 메시지 검증
func TestStatusLogs_FollowFlag(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(loglevelData()))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "logs", "--follow"})
	err := cmd.Execute()
	require.NoError(t, err, "status logs --follow 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "실시간 로그 스트리밍은 아직 지원되지 않습니다",
		"--follow 시 미지원 안내 메시지가 출력되어야 합니다")
}

// TestStatusLogs_TooManyArgs - 인자 2개 이상 시 에러 검증
func TestStatusLogs_TooManyArgs(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("인자 검증 실패 시 서버에 요청하면 안됩니다")
	})

	_, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "logs", "a", "b"})
	err := cmd.Execute()
	require.Error(t, err, "인자 2개 이상 시 에러를 반환해야 합니다")
}

// TestStatusLogs_JSONFormat - 로그 레벨 JSON 출력 형식 검증
func TestStatusLogs_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(loglevelData()))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "logs", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "status logs --format json 실행 에러가 없어야 합니다")

	output := buf.String()
	var result map[string]any
	err = json.Unmarshal([]byte(output), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, "info", result["default_level"], "JSON 에 default_level 이 포함되어야 합니다")
}

// TestStatusLogs_APIError - 로그 레벨 조회 API 에러 전파 검증
func TestStatusLogs_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		resp := map[string]any{
			"success": false,
			"error": map[string]any{
				"code":    "LOG_LEVEL_ERROR",
				"message": "로그 레벨 관리가 지원되지 않습니다",
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	_, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "logs"})
	err := cmd.Execute()
	require.Error(t, err, "로그 레벨 조회 API 에러가 전파되어야 합니다")
}

// --- xflow status metrics 테스트 (메트릭스 요약 재배선) ---

// TestStatusMetrics - monitor/metrics 재배선 및 출력 검증
func TestStatusMetrics(t *testing.T) {
	metricsData := map[string]any{
		"cpu_usage_percent":    float64(0),
		"memory_usage_percent": float64(42.5),
		"go_routines":          float64(25),
		"go_mem_alloc_mb":      float64(12.3),
		"go_mem_sys_mb":        float64(48.0),
		"uptime_seconds":       float64(72.5),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/monitor/metrics", r.URL.Path,
			"요청 경로가 /api/v1/monitor/metrics 여야 합니다")
		assert.NotEqual(t, "/api/v1/metrics", r.URL.Path,
			"죽은 경로 /api/v1/metrics 를 호출하면 안됩니다")
		assert.Equal(t, http.MethodGet, r.Method, "요청 메서드가 GET 이어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(metricsData))
	})

	buf, cmd, cleanup := setupStatusTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"status", "metrics"})
	err := cmd.Execute()
	require.NoError(t, err, "status metrics 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "42.5", "출력에 메모리 사용률이 포함되어야 합니다")
	assert.Contains(t, output, "25", "출력에 goroutine 수가 포함되어야 합니다")
	assert.Contains(t, output, "72.5", "출력에 uptime 이 포함되어야 합니다")
}

// TestStatusMetrics_JSONFormat - 메트릭스 JSON 출력 형식 검증
func TestStatusMetrics_JSONFormat(t *testing.T) {
	metricsData := map[string]any{
		"memory_usage_percent": float64(42.5),
		"go_routines":          float64(25),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/monitor/metrics", r.URL.Path)
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
	assert.Equal(t, float64(42.5), result["memory_usage_percent"],
		"JSON 에 memory_usage_percent 가 포함되어야 합니다")
}

// TestStatusMetrics_APIError - 메트릭스 API 에러 전파 검증
func TestStatusMetrics_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		resp := map[string]any{
			"success": false,
			"error": map[string]any{
				"code":    "METRICS_ERROR",
				"message": "메트릭스 조회에 실패했습니다",
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

	require.NotNil(t, statusCmd, "newStatusCmd 가 nil 을 반환하면 안됩니다")
	assert.Equal(t, "status", statusCmd.Use, "커맨드 Use 가 'status' 여야 합니다")
	assert.NotNil(t, statusCmd.RunE, "status 커맨드는 RunE 를 가져야 합니다")

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
	formats := []string{"json", "yaml", "table", "text"}

	for _, format := range formats {
		t.Run(fmt.Sprintf("format_%s", format), func(t *testing.T) {
			buf, cmd, cleanup := setupStatusTest(t, statusMux(t))
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
