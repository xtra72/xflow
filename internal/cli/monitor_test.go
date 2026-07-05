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

// noConfirm 은 항상 false 를 반환하는 confirmFn 스텁이다.
func noConfirm(string, io.Reader) bool { return false }

// setupMonitorTest 는 monitor 커맨드 테스트를 위한 공통 설정을 수행한다.
// httptest 서버, 루트 커맨드, 출력 버퍼, 정리 함수를 반환한다.
func setupMonitorTest(t *testing.T, handler http.HandlerFunc) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()

	server := httptest.NewServer(handler)

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	clientPtr := &client

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.AddCommand(newMonitorCmd(clientPtr, noConfirm))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return &buf, rootCmd, server.Close
}

// monitorLoglevelData 는 로그 레벨 조회 응답 데이터이다.
func monitorLoglevelData() map[string]any {
	return map[string]any{
		"default_level": "info",
		"components": map[string]any{
			"scheduler": "debug",
			"api":       "warn",
		},
	}
}

// readJSONBody 는 요청 본문을 map 으로 디코딩하는 헬퍼이다.
func readJSONBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	require.NoError(t, err, "요청 본문을 읽을 수 있어야 합니다")
	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m), "요청 본문이 유효한 JSON 이어야 합니다")
	return m
}

// --- xflow monitor metrics 테스트 ---

// TestMonitorMetrics - GET /monitor/metrics 경로/메서드 및 출력 검증
func TestMonitorMetrics(t *testing.T) {
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
		assert.Equal(t, http.MethodGet, r.Method, "요청 메서드가 GET 이어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(metricsData))
	})

	buf, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "metrics"})
	err := cmd.Execute()
	require.NoError(t, err, "monitor metrics 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "42.5", "출력에 메모리 사용률이 포함되어야 합니다")
	assert.Contains(t, output, "25", "출력에 goroutine 수가 포함되어야 합니다")
	assert.Contains(t, output, "72.5", "출력에 uptime 이 포함되어야 합니다")
}

// TestMonitorMetrics_JSONFormat - 메트릭 JSON 출력 형식 검증
func TestMonitorMetrics_JSONFormat(t *testing.T) {
	metricsData := map[string]any{
		"memory_usage_percent": float64(42.5),
		"go_routines":          float64(25),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/monitor/metrics", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(metricsData))
	})

	buf, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "metrics", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "monitor metrics --format json 실행 에러가 없어야 합니다")

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result), "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, float64(42.5), result["memory_usage_percent"],
		"JSON 에 memory_usage_percent 가 포함되어야 합니다")
}

// TestMonitorMetrics_APIError - 메트릭 API 에러 전파 검증
func TestMonitorMetrics_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   map[string]any{"code": "METRICS_ERROR", "message": "메트릭 조회에 실패했습니다"},
		})
	})

	_, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "metrics"})
	err := cmd.Execute()
	require.Error(t, err, "메트릭 API 에러가 전파되어야 합니다")
}

// --- xflow monitor loglevel (get) 테스트 ---

// TestMonitorLoglevel_DefaultList - 인자 없는 'monitor loglevel' 은 목록 조회와 동일해야 한다.
func TestMonitorLoglevel_DefaultList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/monitor/loglevel", r.URL.Path,
			"요청 경로가 /api/v1/monitor/loglevel 여야 합니다")
		assert.Equal(t, http.MethodGet, r.Method, "요청 메서드가 GET 이어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(monitorLoglevelData()))
	})

	buf, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel"})
	err := cmd.Execute()
	require.NoError(t, err, "monitor loglevel 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "info", "출력에 기본 로그 레벨이 포함되어야 합니다")
	assert.Contains(t, output, "scheduler", "출력에 컴포넌트 이름이 포함되어야 합니다")
	assert.Contains(t, output, "debug", "출력에 컴포넌트 레벨이 포함되어야 합니다")
}

// TestMonitorLoglevelGet - 'monitor loglevel get' 명시적 조회 검증
func TestMonitorLoglevelGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/monitor/loglevel", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method, "요청 메서드가 GET 이어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(monitorLoglevelData()))
	})

	buf, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel", "get"})
	err := cmd.Execute()
	require.NoError(t, err, "monitor loglevel get 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "info", "출력에 기본 로그 레벨이 포함되어야 합니다")
	assert.Contains(t, output, "scheduler", "출력에 컴포넌트 이름이 포함되어야 합니다")
}

// TestMonitorLoglevelGet_JSONFormat - 로그 레벨 JSON 출력 검증
func TestMonitorLoglevelGet_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(monitorLoglevelData()))
	})

	buf, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel", "get", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "monitor loglevel get --format json 실행 에러가 없어야 합니다")

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result), "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, "info", result["default_level"], "JSON 에 default_level 이 포함되어야 합니다")
}

// TestMonitorLoglevelGet_APIError - 조회 API 에러 전파 검증
func TestMonitorLoglevelGet_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   map[string]any{"code": "LOG_LEVEL_ERROR", "message": "로그 레벨 관리가 지원되지 않습니다"},
		})
	})

	_, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel", "get"})
	err := cmd.Execute()
	require.Error(t, err, "로그 레벨 조회 API 에러가 전파되어야 합니다")
}

// --- xflow monitor loglevel set (전역) 테스트 ---

// TestMonitorLoglevelSet_Global - 전역 로그 레벨 변경: PUT /monitor/loglevel + 본문 검증
func TestMonitorLoglevelSet_Global(t *testing.T) {
	var capturedBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/monitor/loglevel", r.URL.Path,
			"전역 변경 경로가 /api/v1/monitor/loglevel 여야 합니다")
		assert.Equal(t, http.MethodPut, r.Method, "요청 메서드가 PUT 이어야 합니다")
		capturedBody = readJSONBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"level": "debug"}))
	})

	buf, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel", "set", "debug"})
	err := cmd.Execute()
	require.NoError(t, err, "monitor loglevel set debug 실행 에러가 없어야 합니다")

	assert.Equal(t, "debug", capturedBody["level"], "요청 본문에 level=debug 가 포함되어야 합니다")
	assert.Contains(t, buf.String(), "전역 로그 레벨", "전역 변경 안내 메시지가 출력되어야 합니다")
}

// TestMonitorLoglevelSet_GlobalNormalizesLevel - 대문자/공백 입력이 정규화되어 전송되는지 검증
func TestMonitorLoglevelSet_GlobalNormalizesLevel(t *testing.T) {
	var capturedBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody = readJSONBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"level": "warn"}))
	})

	buf, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel", "set", "  WARN  "})
	err := cmd.Execute()
	require.NoError(t, err, "정규화 가능한 레벨 입력은 에러가 없어야 합니다")

	assert.Equal(t, "warn", capturedBody["level"], "레벨이 정규화(소문자/trim)되어 전송되어야 합니다")
	_ = buf
}

// TestMonitorLoglevelSet_InvalidLevel - 잘못된 레벨은 서버 호출 없이 거부되어야 한다.
func TestMonitorLoglevelSet_InvalidLevel(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("잘못된 레벨 검증 실패 시 서버에 요청하면 안됩니다")
	})

	_, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel", "set", "verbose"})
	err := cmd.Execute()
	require.Error(t, err, "유효하지 않은 로그 레벨은 에러를 반환해야 합니다")
	assert.Contains(t, err.Error(), "유효하지 않은 로그 레벨", "에러 메시지에 안내가 포함되어야 합니다")
}

// TestMonitorLoglevelSet_GlobalJSONFormat - 전역 변경 JSON 출력 검증
func TestMonitorLoglevelSet_GlobalJSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"level": "error"}))
	})

	buf, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel", "set", "error", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "monitor loglevel set error --format json 실행 에러가 없어야 합니다")

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result), "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, "error", result["level"], "JSON 결과에 변경된 레벨이 포함되어야 합니다")
}

// --- xflow monitor loglevel set (컴포넌트) 테스트 ---

// TestMonitorLoglevelSet_Component - 컴포넌트 변경: PUT /monitor/loglevel/{component} + 본문 검증
func TestMonitorLoglevelSet_Component(t *testing.T) {
	var capturedBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/monitor/loglevel/scheduler", r.URL.Path,
			"컴포넌트 변경 경로가 /api/v1/monitor/loglevel/scheduler 여야 합니다")
		assert.Equal(t, http.MethodPut, r.Method, "요청 메서드가 PUT 이어야 합니다")
		capturedBody = readJSONBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"component": "scheduler", "level": "debug"}))
	})

	buf, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel", "set", "scheduler", "debug"})
	err := cmd.Execute()
	require.NoError(t, err, "monitor loglevel set scheduler debug 실행 에러가 없어야 합니다")

	assert.Equal(t, "debug", capturedBody["level"], "요청 본문에 level=debug 가 포함되어야 합니다")
	output := buf.String()
	assert.Contains(t, output, "scheduler", "출력에 컴포넌트 이름이 포함되어야 합니다")
	assert.Contains(t, output, "debug", "출력에 변경된 레벨이 포함되어야 합니다")
}

// TestMonitorLoglevelSet_ComponentInvalidLevel - 컴포넌트 변경 시에도 레벨 검증이 동작해야 한다.
func TestMonitorLoglevelSet_ComponentInvalidLevel(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("잘못된 레벨 검증 실패 시 서버에 요청하면 안됩니다")
	})

	_, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel", "set", "scheduler", "trace"})
	err := cmd.Execute()
	require.Error(t, err, "컴포넌트 변경 시 유효하지 않은 레벨은 에러를 반환해야 합니다")
}

// TestMonitorLoglevelSet_NoArgs - set 인자 0개는 에러여야 한다.
func TestMonitorLoglevelSet_NoArgs(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("인자 검증 실패 시 서버에 요청하면 안됩니다")
	})

	_, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel", "set"})
	err := cmd.Execute()
	require.Error(t, err, "set 인자 0개는 에러를 반환해야 합니다")
}

// TestMonitorLoglevelSet_TooManyArgs - set 인자 3개 이상은 에러여야 한다.
func TestMonitorLoglevelSet_TooManyArgs(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("인자 검증 실패 시 서버에 요청하면 안됩니다")
	})

	_, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel", "set", "scheduler", "debug", "extra"})
	err := cmd.Execute()
	require.Error(t, err, "set 인자 3개 이상은 에러를 반환해야 합니다")
}

// --- xflow monitor loglevel reset 테스트 ---

// TestMonitorLoglevelReset - 컴포넌트 초기화: DELETE /monitor/loglevel/{component} 검증
func TestMonitorLoglevelReset(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/monitor/loglevel/scheduler", r.URL.Path,
			"초기화 경로가 /api/v1/monitor/loglevel/scheduler 여야 합니다")
		assert.Equal(t, http.MethodDelete, r.Method, "요청 메서드가 DELETE 여야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"component": "scheduler"}))
	})

	buf, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel", "reset", "scheduler"})
	err := cmd.Execute()
	require.NoError(t, err, "monitor loglevel reset scheduler 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "scheduler", "출력에 초기화한 컴포넌트가 포함되어야 합니다")
	assert.Contains(t, output, "초기화", "출력에 초기화 안내 메시지가 포함되어야 합니다")
}

// TestMonitorLoglevelReset_JSONFormat - 초기화 JSON 출력 검증
func TestMonitorLoglevelReset_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"component": "api"}))
	})

	buf, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel", "reset", "api", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "monitor loglevel reset api --format json 실행 에러가 없어야 합니다")

	var result map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result), "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, "api", result["component"], "JSON 결과에 초기화된 컴포넌트가 포함되어야 합니다")
}

// TestMonitorLoglevelReset_NoArgs - reset 인자 0개는 에러여야 한다.
func TestMonitorLoglevelReset_NoArgs(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("인자 검증 실패 시 서버에 요청하면 안됩니다")
	})

	_, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel", "reset"})
	err := cmd.Execute()
	require.Error(t, err, "reset 인자 0개는 에러를 반환해야 합니다")
}

// TestMonitorLoglevelReset_APIError - 초기화 API 에러 전파 검증
func TestMonitorLoglevelReset_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   map[string]any{"code": "LOG_LEVEL_ERROR", "message": "초기화 실패"},
		})
	})

	_, cmd, cleanup := setupMonitorTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"monitor", "loglevel", "reset", "scheduler"})
	err := cmd.Execute()
	require.Error(t, err, "초기화 API 에러가 전파되어야 합니다")
}

// --- 서브커맨드 등록/시그니처 검증 ---

// TestNewMonitorCmd_Signature - newMonitorCmd 시그니처 및 서브커맨드 등록 검증
func TestNewMonitorCmd_Signature(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)
	clientPtr := &client

	monitorCmd := newMonitorCmd(clientPtr, noConfirm)
	require.NotNil(t, monitorCmd, "newMonitorCmd 가 nil 을 반환하면 안됩니다")
	assert.Equal(t, "monitor", monitorCmd.Use, "커맨드 Use 가 'monitor' 여야 합니다")

	// 직속 서브커맨드: metrics, loglevel
	topSubs := make(map[string]bool)
	for _, sub := range monitorCmd.Commands() {
		topSubs[splitFirst(sub.Use)] = true
	}
	assert.True(t, topSubs["metrics"], "monitor 에 'metrics' 서브커맨드가 있어야 합니다")
	assert.True(t, topSubs["loglevel"], "monitor 에 'loglevel' 서브커맨드가 있어야 합니다")
}

// TestNewMonitorLoglevelCmd_Subcommands - loglevel 하위 get/set/reset 등록 검증
func TestNewMonitorLoglevelCmd_Subcommands(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)
	clientPtr := &client

	loglevelCmd := newMonitorLoglevelCmd(clientPtr)
	require.NotNil(t, loglevelCmd, "newMonitorLoglevelCmd 가 nil 을 반환하면 안됩니다")
	assert.Equal(t, "loglevel", splitFirst(loglevelCmd.Use), "커맨드 Use 가 'loglevel' 여야 합니다")
	assert.NotNil(t, loglevelCmd.RunE, "loglevel 은 인자 없는 목록 조회를 위해 RunE 를 가져야 합니다")

	subs := make(map[string]bool)
	for _, sub := range loglevelCmd.Commands() {
		subs[splitFirst(sub.Use)] = true
	}
	for _, expected := range []string{"get", "set", "reset"} {
		assert.True(t, subs[expected], "loglevel 에 '%s' 서브커맨드가 있어야 합니다", expected)
	}
}

// TestValidateLogLevel - 레벨 검증 헬퍼 동작 검증 (테이블 기반)
func TestValidateLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLevel string
		wantErr   bool
	}{
		{"소문자 debug", "debug", "debug", false},
		{"소문자 info", "info", "info", false},
		{"소문자 warn", "warn", "warn", false},
		{"소문자 error", "error", "error", false},
		{"대문자 정규화", "INFO", "info", false},
		{"공백 정규화", "  warn  ", "warn", false},
		{"유효하지 않은 값", "verbose", "", true},
		{"빈 문자열", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := validateLogLevel(tt.input)
			if tt.wantErr {
				require.Error(t, err, "유효하지 않은 입력은 에러여야 합니다")
				return
			}
			require.NoError(t, err, "유효한 입력은 에러가 없어야 합니다")
			assert.Equal(t, tt.wantLevel, got, "정규화된 레벨이 일치해야 합니다")
		})
	}
}
