package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- NewRootCmd 테스트 ---

// TestNewRootCmd - 루트 커맨드 생성 및 기본 속성 검증
func TestNewRootCmd(t *testing.T) {
	cmd := NewRootCmd()

	require.NotNil(t, cmd, "루트 커맨드가 nil 이면 안됩니다")
	assert.Equal(t, "xflow", cmd.Use,
		"Use 필드가 'xflow' 여야 합니다")
	assert.NotEmpty(t, cmd.Short,
		"Short 설명이 비어있으면 안됩니다")
	assert.NotEmpty(t, cmd.Long,
		"Long 설명이 비어있으면 안됩니다")
}

// TestNewRootCmd_HasVersionSubcommand - version 서브커맨드 등록 여부 검증
func TestNewRootCmd_HasVersionSubcommand(t *testing.T) {
	cmd := NewRootCmd()

	// version 서브커맨드가 등록되어 있는지 확인
	var found bool
	for _, sub := range cmd.Commands() {
		if sub.Use == "version" {
			found = true
			break
		}
	}
	assert.True(t, found,
		"version 서브커맨드가 등록되어 있어야 합니다")
}

// --- 글로벌 플래그 테스트 ---

// TestGlobalFlags - 8개 글로벌 플래그 등록 및 기본값 검증
func TestGlobalFlags(t *testing.T) {
	cmd := NewRootCmd()
	pflags := cmd.PersistentFlags()

	tests := []struct {
		name         string
		flagName     string
		defaultValue string
	}{
		{"config 플래그", "config", ""},
		{"server 플래그", "server", ""},
		{"format 플래그", "format", "table"},
		{"token 플래그", "token", ""},
		{"verbose 플래그", "verbose", "false"},
		{"quiet 플래그", "quiet", "false"},
		{"no-color 플래그", "no-color", "false"},
		{"insecure 플래그", "insecure", "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := pflags.Lookup(tt.flagName)
			require.NotNil(t, flag,
				"--%s 플래그가 등록되어 있어야 합니다", tt.flagName)
			assert.Equal(t, tt.defaultValue, flag.DefValue,
				"--%s 의 기본값이 '%s' 여야 합니다", tt.flagName, tt.defaultValue)
		})
	}
}

// --- 서버 URL 우선순위 테스트 ---

// TestServerURLPriority - 서버 URL 결정 우선순위 검증 (플래그 > 환경변수 > 설정 > 기본값)
func TestServerURLPriority(t *testing.T) {
	tests := []struct {
		name        string
		flagValue   string
		envValue    string
		configValue string
		expected    string
	}{
		{
			name:        "기본값 사용 (아무 설정 없음)",
			flagValue:   "",
			envValue:    "",
			configValue: "",
			expected:    "http://localhost:8080",
		},
		{
			name:        "설정 파일 값 사용",
			flagValue:   "",
			envValue:    "",
			configValue: "http://config-server:9090",
			expected:    "http://config-server:9090",
		},
		{
			name:        "환경변수가 설정보다 우선",
			flagValue:   "",
			envValue:    "http://env-server:7070",
			configValue: "http://config-server:9090",
			expected:    "http://env-server:7070",
		},
		{
			name:        "플래그가 최우선",
			flagValue:   "http://flag-server:6060",
			envValue:    "http://env-server:7070",
			configValue: "http://config-server:9090",
			expected:    "http://flag-server:6060",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 환경변수 설정
			if tt.envValue != "" {
				t.Setenv("XFLOW_SERVER", tt.envValue)
			}

			// 설정 파일 생성
			configDir := t.TempDir()
			configPath := filepath.Join(configDir, "config.yaml")
			if tt.configValue != "" {
				content := "server:\n  url: " + tt.configValue + "\n"
				err := os.WriteFile(configPath, []byte(content), 0644)
				require.NoError(t, err)
			}

			cmd := NewRootCmd()

			// 플래그 설정
			args := []string{"version"}
			if tt.configValue != "" {
				args = append([]string{"--config", configPath}, args...)
			}
			if tt.flagValue != "" {
				args = append([]string{"--server", tt.flagValue}, args...)
			}
			cmd.SetArgs(args)

			// 실행 (version 서브커맨드로 실행하여 PersistentPreRunE 트리거)
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			err := cmd.Execute()
			require.NoError(t, err, "커맨드 실행 에러가 없어야 합니다")

			// resolveServerURL 직접 테스트
			result := resolveServerURL(cmd, configPath)
			assert.Equal(t, tt.expected, result,
				"서버 URL 이 '%s' 여야 합니다", tt.expected)
		})
	}
}

// --- 토큰 우선순위 테스트 ---

// TestTokenPriority - 토큰 결정 우선순위 검증 (플래그 > 환경변수 > 설정)
func TestTokenPriority(t *testing.T) {
	tests := []struct {
		name        string
		flagValue   string
		envValue    string
		configValue string
		expected    string
	}{
		{
			name:        "설정 파일 토큰 사용",
			flagValue:   "",
			envValue:    "",
			configValue: "config-token-abc",
			expected:    "config-token-abc",
		},
		{
			name:        "환경변수가 설정보다 우선",
			flagValue:   "",
			envValue:    "env-token-xyz",
			configValue: "config-token-abc",
			expected:    "env-token-xyz",
		},
		{
			name:        "플래그가 최우선",
			flagValue:   "flag-token-123",
			envValue:    "env-token-xyz",
			configValue: "config-token-abc",
			expected:    "flag-token-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 환경변수 설정
			if tt.envValue != "" {
				t.Setenv("XFLOW_TOKEN", tt.envValue)
			}

			// 설정 파일 생성
			configDir := t.TempDir()
			configPath := filepath.Join(configDir, "config.yaml")
			if tt.configValue != "" {
				content := "auth:\n  token: " + tt.configValue + "\n"
				err := os.WriteFile(configPath, []byte(content), 0644)
				require.NoError(t, err)
			}

			cmd := NewRootCmd()

			// 플래그 설정
			args := []string{"version"}
			if tt.configValue != "" {
				args = append([]string{"--config", configPath}, args...)
			}
			if tt.flagValue != "" {
				args = append([]string{"--token", tt.flagValue}, args...)
			}
			cmd.SetArgs(args)

			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			err := cmd.Execute()
			require.NoError(t, err, "커맨드 실행 에러가 없어야 합니다")

			result := resolveToken(cmd, configPath)
			assert.Equal(t, tt.expected, result,
				"토큰이 '%s' 여야 합니다", tt.expected)
		})
	}
}

// --- version 커맨드 테스트 ---

// TestVersionCommand - version 서브커맨드의 출력 형식 검증
func TestVersionCommand(t *testing.T) {
	// 빌드 변수 설정 (테스트용)
	origVersion := Version
	origCommit := Commit
	origBuildDate := BuildDate

	Version = "1.2.3"
	Commit = "abc1234"
	BuildDate = "2026-01-01T00:00:00Z"

	defer func() {
		Version = origVersion
		Commit = origCommit
		BuildDate = origBuildDate
	}()

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"version"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	require.NoError(t, err, "version 커맨드 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "1.2.3",
		"출력에 버전 정보가 포함되어야 합니다")
	assert.Contains(t, output, "abc1234",
		"출력에 커밋 해시가 포함되어야 합니다")
	assert.Contains(t, output, "2026-01-01T00:00:00Z",
		"출력에 빌드 시간이 포함되어야 합니다")
	assert.Contains(t, output, runtime.Version(),
		"출력에 Go 버전이 포함되어야 합니다")
}

// TestVersionCommand_DefaultValues - version 기본값 (dev) 출력 검증
func TestVersionCommand_DefaultValues(t *testing.T) {
	origVersion := Version
	origCommit := Commit
	origBuildDate := BuildDate

	Version = "dev"
	Commit = "unknown"
	BuildDate = "unknown"

	defer func() {
		Version = origVersion
		Commit = origCommit
		BuildDate = origBuildDate
	}()

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"version"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	require.NoError(t, err, "version 커맨드 실행 에러가 없어야 합니다")

	output := buf.String()
	assert.Contains(t, output, "dev",
		"기본 버전은 'dev' 여야 합니다")
}

// --- 설정 파일 로딩 테스트 ---

// TestConfigFileLoading - 설정 파일 로딩 검증
func TestConfigFileLoading(t *testing.T) {
	t.Run("존재하는 설정 파일 로딩", func(t *testing.T) {
		configDir := t.TempDir()
		configPath := filepath.Join(configDir, "config.yaml")
		content := "server:\n  url: http://my-server:8080\nauth:\n  token: my-token\n"
		err := os.WriteFile(configPath, []byte(content), 0644)
		require.NoError(t, err)

		cmd := NewRootCmd()
		cmd.SetArgs([]string{"--config", configPath, "version"})

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err = cmd.Execute()
		assert.NoError(t, err,
			"유효한 설정 파일로 실행 시 에러가 없어야 합니다")
	})

	t.Run("존재하지 않는 설정 파일은 무시", func(t *testing.T) {
		cmd := NewRootCmd()
		cmd.SetArgs([]string{"--config", "/nonexistent/path/config.yaml", "version"})

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.Execute()
		assert.NoError(t, err,
			"설정 파일이 없어도 기본값으로 에러 없이 실행되어야 합니다")
	})
}

// --- confirmAction 테스트 ---

// TestConfirmAction - Y/N 확인 입력 처리 검증
func TestConfirmAction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"'y' 입력은 true", "y\n", true},
		{"'Y' 입력은 true", "Y\n", true},
		{"'yes' 입력은 true", "yes\n", true},
		{"'YES' 입력은 true", "YES\n", true},
		{"'n' 입력은 false", "n\n", false},
		{"'N' 입력은 false", "N\n", false},
		{"'no' 입력은 false", "no\n", false},
		{"빈 입력은 false", "\n", false},
		{"임의의 텍스트는 false", "maybe\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			result := confirmAction("계속하시겠습니까?", reader)
			assert.Equal(t, tt.expected, result,
				"입력 '%s' 에 대한 결과가 %v 여야 합니다",
				strings.TrimSpace(tt.input), tt.expected)
		})
	}
}

// --- readFile 테스트 ---

// TestReadFile - 파일 읽기 및 에러 래핑 검증
func TestReadFile(t *testing.T) {
	t.Run("정상 파일 읽기", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.yaml")
		expected := []byte("key: value\n")
		err := os.WriteFile(filePath, expected, 0644)
		require.NoError(t, err)

		data, err := readFile(filePath)
		require.NoError(t, err, "정상 파일 읽기 에러가 없어야 합니다")
		assert.Equal(t, expected, data,
			"파일 내용이 정확하게 읽혀야 합니다")
	})

	t.Run("존재하지 않는 파일 에러", func(t *testing.T) {
		_, err := readFile("/nonexistent/path/file.yaml")
		require.Error(t, err,
			"존재하지 않는 파일 읽기는 에러를 반환해야 합니다")
		assert.Contains(t, err.Error(), "파일 읽기 실패",
			"에러 메시지에 '파일 읽기 실패' 가 포함되어야 합니다")
	})
}

// --- detectFileFormat 테스트 ---

// TestDetectFileFormat - 파일 확장자 기반 형식 감지 검증
func TestDetectFileFormat(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"JSON 파일 (.json)", "workflow.json", "json"},
		{"YAML 파일 (.yaml)", "workflow.yaml", "yaml"},
		{"YML 파일 (.yml)", "workflow.yml", "yaml"},
		{"대문자 JSON (.JSON)", "workflow.JSON", "json"},
		{"대문자 YAML (.YAML)", "workflow.YAML", "yaml"},
		{"알 수 없는 확장자 (.txt)", "readme.txt", ""},
		{"확장자 없는 파일", "Makefile", ""},
		{"경로 포함 JSON", "/path/to/workflow.json", "json"},
		{"경로 포함 YAML", "/path/to/config.yaml", "yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectFileFormat(tt.path)
			assert.Equal(t, tt.expected, result,
				"'%s' 의 형식이 '%s' 여야 합니다", tt.path, tt.expected)
		})
	}
}
