package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- maskToken 테스트 ---

// TestMaskToken - 토큰 마스킹 로직 검증 (빈 값, 짧은 값, 긴 값)
func TestMaskToken(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{"빈 토큰은 빈 문자열", "", ""},
		{"1글자 토큰은 전부 마스킹", "a", "****"},
		{"4글자 토큰은 전부 마스킹", "abcd", "****"},
		{"5글자 토큰은 마지막 4글자 노출", "xabcd", "****abcd"},
		{"긴 토큰은 마지막 4글자만 노출", "mytoken-abcd", "****abcd"},
		{"매우 긴 토큰", "super-secret-token-value-1234", "****1234"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskToken(tt.token)
			assert.Equal(t, tt.expected, result,
				"토큰 '%s' 의 마스킹 결과가 '%s' 여야 합니다", tt.token, tt.expected)
		})
	}
}

// --- config init 테스트 ---

// TestConfigInit_CreateNew - 빈 디렉토리에 설정 파일 신규 생성 검증
func TestConfigInit_CreateNew(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".xflow", "config.yaml")

	// confirmFn 은 호출되면 안됨 (새 파일이므로)
	confirmCalled := false
	confirmFn := func(prompt string, reader io.Reader) bool {
		confirmCalled = true
		return false
	}

	cmd := newConfigCmd(&configPath, confirmFn)
	cmd.SetArgs([]string{"init"})

	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)

	err := cmd.Execute()
	require.NoError(t, err, "config init 실행 에러가 없어야 합니다")
	assert.False(t, confirmCalled, "새 파일 생성 시 확인 프롬프트가 호출되면 안됩니다")

	// 파일이 생성되었는지 확인
	_, statErr := os.Stat(configPath)
	require.NoError(t, statErr, "설정 파일이 생성되어 있어야 합니다")

	// 내용 검증
	data, readErr := os.ReadFile(configPath)
	require.NoError(t, readErr, "설정 파일 읽기 에러가 없어야 합니다")
	content := string(data)

	assert.Contains(t, content, "server:", "설정에 server 섹션이 포함되어야 합니다")
	assert.Contains(t, content, "http://localhost:8080", "기본 서버 URL 이 포함되어야 합니다")
	assert.Contains(t, content, "auth:", "설정에 auth 섹션이 포함되어야 합니다")
	assert.Contains(t, content, "output:", "설정에 output 섹션이 포함되어야 합니다")
}

// TestConfigInit_OverwriteConfirm - 기존 파일이 있을 때 Y 확인 시 덮어쓰기 검증
func TestConfigInit_OverwriteConfirm(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// 기존 파일 생성
	existingContent := "old: config\n"
	err := os.WriteFile(configPath, []byte(existingContent), 0644)
	require.NoError(t, err)

	// Y 로 확인
	confirmFn := func(prompt string, reader io.Reader) bool {
		return true
	}

	cmd := newConfigCmd(&configPath, confirmFn)
	cmd.SetArgs([]string{"init"})

	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)

	err = cmd.Execute()
	require.NoError(t, err, "config init (덮어쓰기) 실행 에러가 없어야 합니다")

	// 덮어쓴 내용 검증
	data, readErr := os.ReadFile(configPath)
	require.NoError(t, readErr)
	content := string(data)

	assert.NotContains(t, content, "old: config",
		"기존 내용이 덮어쓰여야 합니다")
	assert.Contains(t, content, "http://localhost:8080",
		"새 기본 설정이 쓰여야 합니다")
}

// TestConfigInit_OverwriteDeny - 기존 파일이 있을 때 N 확인 시 덮어쓰기 거부 검증
func TestConfigInit_OverwriteDeny(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// 기존 파일 생성
	existingContent := "old: config\n"
	err := os.WriteFile(configPath, []byte(existingContent), 0644)
	require.NoError(t, err)

	// N 으로 거부
	confirmFn := func(prompt string, reader io.Reader) bool {
		return false
	}

	cmd := newConfigCmd(&configPath, confirmFn)
	cmd.SetArgs([]string{"init"})

	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)

	err = cmd.Execute()
	require.NoError(t, err, "config init (거부) 실행 에러가 없어야 합니다")

	// 기존 내용이 유지되었는지 검증
	data, readErr := os.ReadFile(configPath)
	require.NoError(t, readErr)
	assert.Equal(t, existingContent, string(data),
		"거부 시 기존 파일 내용이 유지되어야 합니다")
}

// --- config get 테스트 ---

// TestConfigGet - 설정 파일에서 키 값 읽기 검증
func TestConfigGet(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// 설정 파일 생성
	content := "server:\n  url: http://my-server:9090\n  timeout: 30s\nauth:\n  token: secret-token\noutput:\n  format: json\n  color: true\n"
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{"server.url 읽기", "server.url", "http://my-server:9090"},
		{"auth.token 읽기", "auth.token", "secret-token"},
		{"output.format 읽기", "output.format", "json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confirmFn := func(prompt string, reader io.Reader) bool { return false }
			cmd := newConfigCmd(&configPath, confirmFn)
			cmd.SetArgs([]string{"get", tt.key})

			var outBuf, errBuf bytes.Buffer
			cmd.SetOut(&outBuf)
			cmd.SetErr(&errBuf)

			err := cmd.Execute()
			require.NoError(t, err, "config get %s 실행 에러가 없어야 합니다", tt.key)

			output := strings.TrimSpace(outBuf.String())
			assert.Equal(t, tt.expected, output,
				"config get %s 의 결과가 '%s' 여야 합니다", tt.key, tt.expected)
		})
	}
}

// --- config set 테스트 ---

// TestConfigSet - 설정 값 쓰기 및 검증
func TestConfigSet(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// 초기 설정 파일 생성
	content := "server:\n  url: http://localhost:8080\n  timeout: 30s\nauth:\n  token: \"\"\noutput:\n  format: table\n  color: true\n"
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	confirmFn := func(prompt string, reader io.Reader) bool { return false }

	// set 실행
	cmd := newConfigCmd(&configPath, confirmFn)
	cmd.SetArgs([]string{"set", "server.url", "http://new-server:3000"})

	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)

	err = cmd.Execute()
	require.NoError(t, err, "config set 실행 에러가 없어야 합니다")

	// get 으로 변경된 값 확인
	cmd2 := newConfigCmd(&configPath, confirmFn)
	cmd2.SetArgs([]string{"get", "server.url"})

	var outBuf2 bytes.Buffer
	cmd2.SetOut(&outBuf2)
	cmd2.SetErr(&bytes.Buffer{})

	err = cmd2.Execute()
	require.NoError(t, err, "config get 실행 에러가 없어야 합니다")

	output := strings.TrimSpace(outBuf2.String())
	assert.Equal(t, "http://new-server:3000", output,
		"config set 후 값이 올바르게 변경되어야 합니다")
}

// --- config server 테스트 ---

// TestConfigServer - server.url 단축 명령어 검증
func TestConfigServer(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// 초기 설정 파일 생성
	content := "server:\n  url: http://localhost:8080\n  timeout: 30s\nauth:\n  token: \"\"\noutput:\n  format: table\n  color: true\n"
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	confirmFn := func(prompt string, reader io.Reader) bool { return false }

	// server 명령어 실행
	cmd := newConfigCmd(&configPath, confirmFn)
	cmd.SetArgs([]string{"server", "http://prod-server:8080"})

	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)

	err = cmd.Execute()
	require.NoError(t, err, "config server 실행 에러가 없어야 합니다")

	// get 으로 변경 확인
	cmd2 := newConfigCmd(&configPath, confirmFn)
	cmd2.SetArgs([]string{"get", "server.url"})

	var outBuf2 bytes.Buffer
	cmd2.SetOut(&outBuf2)
	cmd2.SetErr(&bytes.Buffer{})

	err = cmd2.Execute()
	require.NoError(t, err)

	output := strings.TrimSpace(outBuf2.String())
	assert.Equal(t, "http://prod-server:8080", output,
		"config server 명령이 server.url 을 올바르게 설정해야 합니다")
}

// --- config token 테스트 ---

// TestConfigToken - auth.token 단축 명령어 검증
func TestConfigToken(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// 초기 설정 파일 생성
	content := "server:\n  url: http://localhost:8080\n  timeout: 30s\nauth:\n  token: \"\"\noutput:\n  format: table\n  color: true\n"
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	confirmFn := func(prompt string, reader io.Reader) bool { return false }

	// token 명령어 실행
	cmd := newConfigCmd(&configPath, confirmFn)
	cmd.SetArgs([]string{"token", "my-secret-token-1234"})

	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)

	err = cmd.Execute()
	require.NoError(t, err, "config token 실행 에러가 없어야 합니다")

	// get 으로 변경 확인
	cmd2 := newConfigCmd(&configPath, confirmFn)
	cmd2.SetArgs([]string{"get", "auth.token"})

	var outBuf2 bytes.Buffer
	cmd2.SetOut(&outBuf2)
	cmd2.SetErr(&bytes.Buffer{})

	err = cmd2.Execute()
	require.NoError(t, err)

	output := strings.TrimSpace(outBuf2.String())
	assert.Equal(t, "my-secret-token-1234", output,
		"config token 명령이 auth.token 을 올바르게 설정해야 합니다")
}

// --- config list 테스트 ---

// TestConfigList - 전체 설정 출력 및 토큰 마스킹 검증
func TestConfigList(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// 토큰이 포함된 설정 파일 생성
	content := "server:\n  url: http://my-server:8080\n  timeout: 30s\nauth:\n  token: super-secret-token-abcd\noutput:\n  format: yaml\n  color: true\n"
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	confirmFn := func(prompt string, reader io.Reader) bool { return false }

	cmd := newConfigCmd(&configPath, confirmFn)
	cmd.SetArgs([]string{"list"})

	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)

	err = cmd.Execute()
	require.NoError(t, err, "config list 실행 에러가 없어야 합니다")

	output := outBuf.String()

	// 주요 키가 출력에 포함되어 있는지
	assert.Contains(t, output, "server.url",
		"출력에 server.url 키가 포함되어야 합니다")
	assert.Contains(t, output, "http://my-server:8080",
		"출력에 서버 URL 값이 포함되어야 합니다")
	assert.Contains(t, output, "auth.token",
		"출력에 auth.token 키가 포함되어야 합니다")

	// 토큰은 마스킹되어야 함
	assert.NotContains(t, output, "super-secret-token-abcd",
		"출력에 토큰 원문이 노출되면 안됩니다")
	assert.Contains(t, output, "****abcd",
		"출력에 마스킹된 토큰이 포함되어야 합니다")
}

// TestConfigList_EmptyToken - 빈 토큰일 때 list 출력 검증
func TestConfigList_EmptyToken(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	content := "server:\n  url: http://localhost:8080\n  timeout: 30s\nauth:\n  token: \"\"\noutput:\n  format: table\n  color: true\n"
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	confirmFn := func(prompt string, reader io.Reader) bool { return false }

	cmd := newConfigCmd(&configPath, confirmFn)
	cmd.SetArgs([]string{"list"})

	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)

	err = cmd.Execute()
	require.NoError(t, err, "config list (빈 토큰) 실행 에러가 없어야 합니다")

	output := outBuf.String()
	assert.Contains(t, output, "auth.token",
		"빈 토큰이어도 auth.token 키는 표시되어야 합니다")
}

// --- config get 존재하지 않는 키 테스트 ---

// TestConfigGet_NonExistentKey - 존재하지 않는 키 조회 시 빈 값 반환 검증
func TestConfigGet_NonExistentKey(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	content := "server:\n  url: http://localhost:8080\n"
	err := os.WriteFile(configPath, []byte(content), 0644)
	require.NoError(t, err)

	confirmFn := func(prompt string, reader io.Reader) bool { return false }

	cmd := newConfigCmd(&configPath, confirmFn)
	cmd.SetArgs([]string{"get", "nonexistent.key"})

	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)

	err = cmd.Execute()
	require.NoError(t, err, "존재하지 않는 키 조회 시 에러가 없어야 합니다")

	output := strings.TrimSpace(outBuf.String())
	assert.Empty(t, output,
		"존재하지 않는 키 조회 시 빈 출력이어야 합니다")
}

// --- config init 부모 디렉토리 생성 테스트 ---

// TestConfigInit_CreatesParentDir - 부모 디렉토리가 없을 때 자동 생성 검증
func TestConfigInit_CreatesParentDir(t *testing.T) {
	tmpDir := t.TempDir()
	// 깊은 경로의 설정 파일
	configPath := filepath.Join(tmpDir, "deep", "nested", "dir", "config.yaml")

	confirmFn := func(prompt string, reader io.Reader) bool { return false }

	cmd := newConfigCmd(&configPath, confirmFn)
	cmd.SetArgs([]string{"init"})

	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)

	err := cmd.Execute()
	require.NoError(t, err, "config init 이 부모 디렉토리를 자동 생성해야 합니다")

	// 파일이 생성되었는지 확인
	_, statErr := os.Stat(configPath)
	require.NoError(t, statErr, "깊은 경로에도 설정 파일이 생성되어야 합니다")
}
