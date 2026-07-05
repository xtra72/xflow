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

// setupSystemTest 는 system 커맨드 테스트를 위한 공통 설정을 수행한다.
// httptest 서버, 루트 커맨드, 출력 버퍼, 정리 함수를 반환한다.
// confirmFn 은 항상 true 를 반환한다(롤백 확인 통과).
func setupSystemTest(t *testing.T, handler http.HandlerFunc) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()
	return setupSystemTestWithConfirm(t, handler, func(string, io.Reader) bool { return true })
}

// setupSystemTestWithConfirm 은 confirmFn 을 커스텀할 수 있는 세팅 헬퍼이다.
func setupSystemTestWithConfirm(t *testing.T, handler http.HandlerFunc, confirmFn func(string, io.Reader) bool) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()

	server := httptest.NewServer(handler)

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	clientPtr := &client

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.AddCommand(newSystemCmd(clientPtr, confirmFn))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return &buf, rootCmd, server.Close
}

// --- system version 테스트 ---

// versionData 는 GET /system/version 응답 데이터이다.
func versionData() map[string]any {
	return map[string]any{
		"version":          "v1.2.3",
		"commit":           "abc1234",
		"build_date":       "2026-01-01",
		"go_version":       "go1.25.0",
		"channel":          "stable",
		"os":               "linux",
		"arch":             "amd64",
		"hostname":         "node-1",
		"mode":             "server",
		"uptime_seconds":   float64(12345),
		"update_available": true,
		"latest_version":   "v1.3.0",
	}
}

// TestSystemVersion - 원격 서버 버전 정보 조회 검증 (경로/메서드/출력)
func TestSystemVersion(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/system/version", r.URL.Path, "요청 경로가 /api/v1/system/version 여야 합니다")
		assert.Equal(t, http.MethodGet, r.Method, "요청 메서드가 GET 이어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(versionData()))
	})

	buf, cmd, cleanup := setupSystemTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"system", "version"})
	err := cmd.Execute()
	require.NoError(t, err, "system version 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "v1.2.3", "출력에 현재 버전이 포함되어야 합니다")
	assert.Contains(t, output, "node-1", "출력에 hostname 이 포함되어야 합니다")
	assert.Contains(t, output, "server", "출력에 mode 가 포함되어야 합니다")
	assert.Contains(t, output, "v1.3.0", "출력에 마지막 체크 결과(latest_version)가 포함되어야 합니다")
}

// TestSystemVersion_JSONFormat - 버전 정보 JSON 출력 형식 검증
func TestSystemVersion_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(versionData()))
	})

	buf, cmd, cleanup := setupSystemTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"system", "version", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "system version --format json 실행 에러가 없어야 합니다")

	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, "v1.2.3", result["version"], "JSON 에 version 이 포함되어야 합니다")
	assert.Equal(t, true, result["update_available"], "JSON 에 update_available 이 포함되어야 합니다")
}

// TestSystemVersion_APIError - 버전 조회 API 에러 전파 검증
func TestSystemVersion_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   map[string]any{"code": "ERR", "message": "fail"},
		})
	})

	_, cmd, cleanup := setupSystemTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"system", "version"})
	err := cmd.Execute()
	require.Error(t, err, "버전 조회 API 에러가 전파되어야 합니다")
}

// --- system update check 테스트 ---

// TestSystemUpdateCheck - 신규 버전 확인 검증 (경로/메서드/출력)
func TestSystemUpdateCheck(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/system/update/check", r.URL.Path,
			"요청 경로가 /api/v1/system/update/check 여야 합니다")
		assert.Equal(t, http.MethodPost, r.Method, "요청 메서드가 POST 여야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{
			"current":      "v1.2.3",
			"latest":       "v1.3.0",
			"available":    true,
			"channel":      "stable",
			"release_url":  "https://example.com/release",
			"published_at": "2026-01-02T00:00:00Z",
		}))
	})

	buf, cmd, cleanup := setupSystemTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"system", "update", "check"})
	err := cmd.Execute()
	require.NoError(t, err, "system update check 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "v1.3.0", "출력에 latest 버전이 포함되어야 합니다")
	assert.Contains(t, output, "stable", "출력에 채널이 포함되어야 합니다")
}

// TestSystemUpdateCheck_JSONFormat - 신규 버전 확인 JSON 출력 검증
func TestSystemUpdateCheck_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{
			"current":   "v1.2.3",
			"latest":    "v1.3.0",
			"available": true,
			"channel":   "stable",
		}))
	})

	buf, cmd, cleanup := setupSystemTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"system", "update", "check", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "system update check --format json 실행 에러가 없어야 합니다")

	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, "v1.3.0", result["latest"], "JSON 에 latest 가 포함되어야 합니다")
}

// --- system update apply 테스트 ---

// TestSystemUpdateApply - 업데이트 적용 시작 검증 (경로/메서드/빈 바디)
func TestSystemUpdateApply(t *testing.T) {
	var receivedBody map[string]any
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/system/update/apply", r.URL.Path,
			"요청 경로가 /api/v1/system/update/apply 여야 합니다")
		assert.Equal(t, http.MethodPost, r.Method, "요청 메서드가 POST 여야 합니다")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{
			"operation_id": "op-123",
			"status":       "starting",
			"from_version": "v1.2.3",
		}))
	})

	buf, cmd, cleanup := setupSystemTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"system", "update", "apply"})
	err := cmd.Execute()
	require.NoError(t, err, "system update apply 실행 에러가 없어야 합니다")

	// 플래그 없이 호출하면 version/force 가 바디에 포함되지 않아야 한다.
	_, hasVersion := receivedBody["version"]
	_, hasForce := receivedBody["force"]
	assert.False(t, hasVersion, "플래그 없으면 version 필드가 바디에 없어야 합니다")
	assert.False(t, hasForce, "플래그 없으면 force 필드가 바디에 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "op-123", "출력에 operation_id 가 포함되어야 합니다")
	assert.Contains(t, output, "starting", "출력에 상태가 포함되어야 합니다")
}

// TestSystemUpdateApply_VersionAndForce - --version/--force 바디 전달 검증
func TestSystemUpdateApply_VersionAndForce(t *testing.T) {
	var receivedBody map[string]any
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method, "요청 메서드가 POST 여야 합니다")
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &receivedBody), "요청 바디가 유효한 JSON 이어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{
			"operation_id": "op-456",
			"status":       "starting",
			"from_version": "v1.3.0",
			"to_version":   "v1.2.0",
		}))
	})

	buf, cmd, cleanup := setupSystemTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"system", "update", "apply", "--version", "v1.2.0", "--force"})
	err := cmd.Execute()
	require.NoError(t, err, "system update apply --version --force 실행 에러가 없어야 합니다")

	assert.Equal(t, "v1.2.0", receivedBody["version"], "바디에 지정한 version 이 포함되어야 합니다")
	assert.Equal(t, true, receivedBody["force"], "바디에 force=true 가 포함되어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "op-456", "출력에 operation_id 가 포함되어야 합니다")
}

// TestSystemUpdateApply_JSONFormat - 적용 응답 JSON 출력 검증
func TestSystemUpdateApply_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{
			"operation_id": "op-789",
			"status":       "starting",
		}))
	})

	buf, cmd, cleanup := setupSystemTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"system", "update", "apply", "--format", "json"})
	err := cmd.Execute()
	require.NoError(t, err, "system update apply --format json 실행 에러가 없어야 합니다")

	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err, "출력이 유효한 JSON 이어야 합니다")
	assert.Equal(t, "op-789", result["operation_id"], "JSON 에 operation_id 가 포함되어야 합니다")
}

// --- system update status 테스트 ---

// TestSystemUpdateStatus - 업데이트 작업 상태 조회 검증 (경로/메서드/출력)
func TestSystemUpdateStatus(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/system/update/status", r.URL.Path,
			"요청 경로가 /api/v1/system/update/status 여야 합니다")
		assert.Equal(t, http.MethodGet, r.Method, "요청 메서드가 GET 이어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{
			"operation_id": "op-123",
			"status":       "downloading",
			"from_version": "v1.2.3",
			"to_version":   "v1.3.0",
		}))
	})

	buf, cmd, cleanup := setupSystemTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"system", "update", "status"})
	err := cmd.Execute()
	require.NoError(t, err, "system update status 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "downloading", "출력에 작업 상태가 포함되어야 합니다")
	assert.Contains(t, output, "op-123", "출력에 operation_id 가 포함되어야 합니다")
}

// TestSystemUpdateStatus_Idle - idle 상태 조회 검증
func TestSystemUpdateStatus_Idle(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "요청 메서드가 GET 이어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"status": "idle"}))
	})

	buf, cmd, cleanup := setupSystemTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"system", "update", "status"})
	err := cmd.Execute()
	require.NoError(t, err, "system update status (idle) 실행 에러가 없어야 합니다")

	assert.Contains(t, buf.String(), "idle", "출력에 idle 상태가 포함되어야 합니다")
}

// --- system update rollback 테스트 ---

// TestSystemUpdateRollback_Confirmed - 확인 후 롤백 검증 (경로/메서드)
func TestSystemUpdateRollback_Confirmed(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		assert.Equal(t, "/api/v1/system/update/rollback", r.URL.Path,
			"요청 경로가 /api/v1/system/update/rollback 여야 합니다")
		assert.Equal(t, http.MethodPost, r.Method, "요청 메서드가 POST 여야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{
			"from_version":   "v1.3.0",
			"to_version":     "v1.2.3",
			"rolled_back_at": "2026-01-03T00:00:00Z",
		}))
	})

	// confirmFn 이 true 를 반환하도록 설정
	buf, cmd, cleanup := setupSystemTestWithConfirm(t, handler, func(string, io.Reader) bool { return true })
	defer cleanup()

	cmd.SetArgs([]string{"system", "update", "rollback"})
	err := cmd.Execute()
	require.NoError(t, err, "system update rollback 실행 에러가 없어야 합니다")

	assert.True(t, called, "확인 시 롤백 API 가 호출되어야 합니다")
	assert.Contains(t, buf.String(), "v1.2.3", "출력에 복원된 버전이 포함되어야 합니다")
}

// TestSystemUpdateRollback_Cancelled - 확인 거부 시 API 미호출 검증
func TestSystemUpdateRollback_Cancelled(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{}))
	})

	// confirmFn 이 false 를 반환하도록 설정 (취소)
	buf, cmd, cleanup := setupSystemTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"system", "update", "rollback"})
	err := cmd.Execute()
	require.NoError(t, err, "취소 시에도 에러가 아니어야 합니다")

	assert.False(t, called, "확인 거부 시 롤백 API 가 호출되면 안됩니다")
	assert.Contains(t, buf.String(), "취소", "취소 안내 메시지가 출력되어야 합니다")
}

// TestSystemUpdateRollback_YesFlag - --yes 플래그 시 확인 없이 롤백 검증
func TestSystemUpdateRollback_YesFlag(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		assert.Equal(t, http.MethodPost, r.Method, "요청 메서드가 POST 여야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{
			"from_version":   "v1.3.0",
			"to_version":     "v1.2.3",
			"rolled_back_at": "2026-01-03T00:00:00Z",
		}))
	})

	// confirmFn 이 false 를 반환해도 --yes 면 호출되어야 한다.
	confirmCalled := false
	_, cmd, cleanup := setupSystemTestWithConfirm(t, handler, func(string, io.Reader) bool {
		confirmCalled = true
		return false
	})
	defer cleanup()

	cmd.SetArgs([]string{"system", "update", "rollback", "--yes"})
	err := cmd.Execute()
	require.NoError(t, err, "system update rollback --yes 실행 에러가 없어야 합니다")

	assert.True(t, called, "--yes 시 롤백 API 가 호출되어야 합니다")
	assert.False(t, confirmCalled, "--yes 시 confirmFn 이 호출되면 안됩니다")
}

// --- system channel 테스트 ---

// channelData 는 GET /system/update/channel 응답 데이터이다.
func channelData() map[string]any {
	return map[string]any{
		"current":   "stable",
		"available": []any{"stable", "beta", "nightly"},
	}
}

// TestSystemChannel_NoArgs - 인자 없는 channel 호출 시 현재 채널 조회 검증
func TestSystemChannel_NoArgs(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/system/update/channel", r.URL.Path,
			"요청 경로가 /api/v1/system/update/channel 여야 합니다")
		assert.Equal(t, http.MethodGet, r.Method, "요청 메서드가 GET 이어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(channelData()))
	})

	buf, cmd, cleanup := setupSystemTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"system", "channel"})
	err := cmd.Execute()
	require.NoError(t, err, "system channel 실행 에러가 없어야 합니다")

	assert.Contains(t, buf.String(), "stable", "출력에 현재 채널이 포함되어야 합니다")
}

// TestSystemChannelGet - channel get 시 현재 채널 조회 검증 (경로/메서드)
func TestSystemChannelGet(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/system/update/channel", r.URL.Path,
			"요청 경로가 /api/v1/system/update/channel 여야 합니다")
		assert.Equal(t, http.MethodGet, r.Method, "요청 메서드가 GET 이어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(channelData()))
	})

	buf, cmd, cleanup := setupSystemTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"system", "channel", "get"})
	err := cmd.Execute()
	require.NoError(t, err, "system channel get 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "stable", "출력에 현재 채널이 포함되어야 합니다")
	assert.Contains(t, output, "nightly", "출력에 선택 가능한 채널 목록이 포함되어야 합니다")
}

// TestSystemChannelSet - channel set 시 채널 변경 검증 (경로/메서드/바디)
func TestSystemChannelSet(t *testing.T) {
	var receivedBody map[string]any
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/system/update/channel", r.URL.Path,
			"요청 경로가 /api/v1/system/update/channel 여야 합니다")
		assert.Equal(t, http.MethodPut, r.Method, "요청 메서드가 PUT 이어야 합니다")
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &receivedBody), "요청 바디가 유효한 JSON 이어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{
			"previous": "stable",
			"current":  "beta",
			"message":  "채널 변경 완료",
		}))
	})

	buf, cmd, cleanup := setupSystemTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"system", "channel", "set", "beta"})
	err := cmd.Execute()
	require.NoError(t, err, "system channel set beta 실행 에러가 없어야 합니다")

	assert.Equal(t, "beta", receivedBody["channel"], "바디에 새 채널(beta)이 포함되어야 합니다")
	assert.Contains(t, buf.String(), "beta", "출력에 변경된 채널이 포함되어야 합니다")
}

// TestSystemChannelSet_InvalidChannel - 잘못된 채널 값 거부 검증 (서버 미호출)
func TestSystemChannelSet_InvalidChannel(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	_, cmd, cleanup := setupSystemTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"system", "channel", "set", "experimental"})
	err := cmd.Execute()
	require.Error(t, err, "잘못된 채널 값은 에러를 반환해야 합니다")
	assert.Contains(t, err.Error(), "experimental", "에러 메시지에 잘못된 채널 값이 포함되어야 합니다")
	assert.False(t, called, "잘못된 채널 값은 서버에 요청하면 안됩니다")
}

// TestSystemChannelSet_AllValidChannels - 허용 채널 값 모두 통과 검증
func TestSystemChannelSet_AllValidChannels(t *testing.T) {
	for _, channel := range []string{"stable", "beta", "nightly"} {
		t.Run(channel, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPut, r.Method)
				w.Header().Set("Content-Type", "application/json")
				w.Write(apiEnvelope(map[string]any{"current": channel}))
			})

			_, cmd, cleanup := setupSystemTest(t, handler)
			defer cleanup()

			cmd.SetArgs([]string{"system", "channel", "set", channel})
			err := cmd.Execute()
			require.NoError(t, err, "허용 채널 %q 는 통과해야 합니다", channel)
		})
	}
}

// TestSystemChannelSet_MissingArg - 인자 없는 set 호출 시 에러 검증
func TestSystemChannelSet_MissingArg(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("인자 검증 실패 시 서버에 요청하면 안됩니다")
	})

	_, cmd, cleanup := setupSystemTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"system", "channel", "set"})
	err := cmd.Execute()
	require.Error(t, err, "인자 없는 set 호출은 에러를 반환해야 합니다")
}

// --- 서브커맨드 등록 검증 ---

// TestSystemSubcommands - system 커맨드와 서브커맨드 구조 등록 검증
func TestSystemSubcommands(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)
	clientPtr := &client
	confirmFn := func(string, io.Reader) bool { return true }

	systemCmd := newSystemCmd(clientPtr, confirmFn)

	require.NotNil(t, systemCmd, "newSystemCmd 가 nil 을 반환하면 안됩니다")
	assert.Equal(t, "system", systemCmd.Use, "커맨드 Use 가 'system' 여야 합니다")

	// 직속 서브커맨드: version, update, channel
	directSubs := subcommandNames(systemCmd)
	for _, expected := range []string{"version", "update", "channel"} {
		assert.True(t, directSubs[expected],
			"system 에 '%s' 서브커맨드가 등록되어 있어야 합니다", expected)
	}

	// update 하위: check, apply, status, rollback
	updateCmd := findSubcommand(systemCmd, "update")
	require.NotNil(t, updateCmd, "update 서브커맨드를 찾아야 합니다")
	updateSubs := subcommandNames(updateCmd)
	for _, expected := range []string{"check", "apply", "status", "rollback"} {
		assert.True(t, updateSubs[expected],
			"update 에 '%s' 서브커맨드가 등록되어 있어야 합니다", expected)
	}

	// channel 하위: get, set
	channelCmd := findSubcommand(systemCmd, "channel")
	require.NotNil(t, channelCmd, "channel 서브커맨드를 찾아야 합니다")
	channelSubs := subcommandNames(channelCmd)
	for _, expected := range []string{"get", "set"} {
		assert.True(t, channelSubs[expected],
			"channel 에 '%s' 서브커맨드가 등록되어 있어야 합니다", expected)
	}
}

// TestSystemHelpMentionsRemote - 도움말에 원격 서버 대상임과 xflowd update 차이 명시 검증
func TestSystemHelpMentionsRemote(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)
	clientPtr := &client
	confirmFn := func(string, io.Reader) bool { return true }

	systemCmd := newSystemCmd(clientPtr, confirmFn)

	help := systemCmd.Long
	assert.Contains(t, help, "원격", "도움말에 '원격 서버' 대상임이 명시되어야 합니다")
	assert.Contains(t, help, "xflowd update", "도움말에 xflowd update(로컬)와의 차이가 명시되어야 합니다")
}

// --- 헬퍼 ---

// subcommandNames 는 커맨드의 직속 서브커맨드 이름 집합을 반환한다.
func subcommandNames(cmd *cobra.Command) map[string]bool {
	names := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		names[splitFirst(sub.Use)] = true
	}
	return names
}

// findSubcommand 는 커맨드의 직속 서브커맨드 중 이름이 일치하는 것을 반환한다.
func findSubcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if splitFirst(sub.Use) == name {
			return sub
		}
	}
	return nil
}
