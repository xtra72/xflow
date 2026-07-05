package cli

import (
	"bytes"
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
// 핸들러가 호출되면 t.Fatal 로 실패하도록 하여, plugin 명령이 실제 API 를
// 호출하지 않고 미지원 안내로 단락됨을 보장한다.
func setupPluginTest(t *testing.T) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	// plugin 명령은 절대 서버를 호출하면 안 되므로, 호출 시 실패하는 서버를 둔다.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("plugin 명령은 실제 API(%s)를 호출하면 안됩니다", r.URL.Path)
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-token", 5*time.Second, false)

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")

	confirmFn := func(prompt string, reader io.Reader) bool { return true }
	rootCmd.AddCommand(newPluginCmd(&client, confirmFn))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return rootCmd, &buf
}

// --- plugin 명령 hidden 처리 검증 ---

// TestPluginCmd_Hidden - plugin 명령 그룹이 Hidden 처리되었는지 검증
func TestPluginCmd_Hidden(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)
	confirmFn := func(prompt string, reader io.Reader) bool { return false }

	pluginCmd := newPluginCmd(&client, confirmFn)
	assert.True(t, pluginCmd.Hidden, "plugin 명령은 Hidden 이어야 합니다")

	for _, sub := range pluginCmd.Commands() {
		assert.True(t, sub.Hidden,
			"plugin 하위 명령 '%s' 도 Hidden 이어야 합니다", sub.Use)
	}
}

// --- plugin 서브명령 미지원 안내 검증 ---

// TestPluginSubcommands_NotSupported - 각 서브명령이 미지원 안내 + non-zero exit 검증
func TestPluginSubcommands_NotSupported(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"list", []string{"plugin", "list"}},
		{"install", []string{"plugin", "install", "some-plugin"}},
		{"remove", []string{"plugin", "remove", "some-plugin"}},
		{"remove with --yes", []string{"plugin", "remove", "--yes", "some-plugin"}},
		{"update", []string{"plugin", "update", "some-plugin"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCmd, _ := setupPluginTest(t)
			rootCmd.SetArgs(tt.args)

			err := rootCmd.Execute()
			require.Error(t, err, "미지원 명령은 에러를 반환해야 합니다")
			assert.Contains(t, err.Error(), "지원되지 않습니다",
				"에러 메시지에 미지원 안내가 포함되어야 합니다")

			// CLIError 로 non-zero exit code 를 가져야 한다
			cliErr, ok := err.(*CLIError)
			require.True(t, ok, "에러는 *CLIError 타입이어야 합니다")
			assert.NotZero(t, cliErr.ExitCode, "ExitCode 는 non-zero 여야 합니다")
		})
	}
}

// TestPluginNotSupportedError - 안내 에러 생성 함수 검증
func TestPluginNotSupportedError(t *testing.T) {
	err := newPluginNotSupportedError()
	require.NotNil(t, err, "안내 에러는 nil 이 아니어야 합니다")
	assert.Contains(t, err.Message, "SPEC-PLUGIN-001",
		"메시지에 후속 SPEC 참조가 포함되어야 합니다")
	assert.Equal(t, 1, err.ExitCode, "ExitCode 는 1 이어야 합니다")
}

// --- 서브커맨드 등록 검증 ---

// TestPluginSubcommands - 4개 서브커맨드 등록 여부 검증 (코드 보존 확인)
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
			"plugin 에 '%s' 서브커맨드가 등록되어 있어야 합니다 (코드 보존)", expected)
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

// TestPluginRemove_YesFlagExists - remove 명령에 --yes 플래그가 보존되었는지 검증
func TestPluginRemove_YesFlagExists(t *testing.T) {
	client := NewClient("http://localhost", "", 5*time.Second, false)
	confirmFn := func(prompt string, reader io.Reader) bool { return false }

	pluginCmd := newPluginCmd(&client, confirmFn)
	for _, sub := range pluginCmd.Commands() {
		if strings.HasPrefix(sub.Use, "remove") {
			assert.NotNil(t, sub.Flags().Lookup("yes"),
				"remove 명령에 --yes 플래그가 보존되어야 합니다")
			return
		}
	}
	t.Fatal("remove 서브커맨드를 찾지 못했습니다")
}
