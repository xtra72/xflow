package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ParseLogOutput 테스트 ---

// TestParseLogOutput_Stdout - "stdout" 입력 시 os.Stdout 반환 확인
func TestParseLogOutput_Stdout(t *testing.T) {
	w, closer, err := ParseLogOutput("stdout")
	require.NoError(t, err)
	assert.Equal(t, os.Stdout, w)
	assert.Nil(t, closer)
}

// TestParseLogOutput_FilePath - 파일 경로 입력 시 파일에 기록 확인
func TestParseLogOutput_FilePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	w, closer, err := ParseLogOutput(path)
	require.NoError(t, err)
	require.NotNil(t, w)
	require.NotNil(t, closer)
	defer closer.Close()

	// 파일에 기록 후 내용 확인
	_, err = w.Write([]byte("hello\n"))
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(content))
}

// TestParseLogOutput_StdoutPlusFile - "stdout+파일경로" 입력 시 MultiWriter 반환 확인
func TestParseLogOutput_StdoutPlusFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	w, closer, err := ParseLogOutput("stdout+" + path)
	require.NoError(t, err)
	require.NotNil(t, w)
	require.NotNil(t, closer)
	defer closer.Close()

	// MultiWriter를 통해 파일에 기록되는지 확인
	_, err = w.Write([]byte("multi\n"))
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "multi\n", string(content))
}

// TestParseLogOutput_AutoCreateDir - 중첩 디렉토리 자동 생성 확인
func TestParseLogOutput_AutoCreateDir(t *testing.T) {
	dir := t.TempDir()
	nestedPath := filepath.Join(dir, "nested", "deep", "test.log")

	w, closer, err := ParseLogOutput(nestedPath)
	require.NoError(t, err)
	require.NotNil(t, w)
	require.NotNil(t, closer)
	defer closer.Close()

	// 디렉토리가 생성되었는지 확인
	_, err = os.Stat(filepath.Join(dir, "nested", "deep"))
	assert.NoError(t, err)
}

// TestParseLogOutput_AppendMode - 기존 파일에 append 모드로 기록 확인
func TestParseLogOutput_AppendMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.log")

	// 초기 내용 작성
	err := os.WriteFile(path, []byte("existing\n"), 0644)
	require.NoError(t, err)

	w, closer, err := ParseLogOutput(path)
	require.NoError(t, err)
	require.NotNil(t, closer)
	defer closer.Close()

	_, err = w.Write([]byte("new\n"))
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "existing\nnew\n", string(content))
}

// TestParseLogOutput_EmptyString - 빈 문자열 입력 시 에러 반환 확인
func TestParseLogOutput_EmptyString(t *testing.T) {
	_, _, err := ParseLogOutput("")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidLogOutput)
}

// TestParseLogOutput_StdoutPlusEmpty - "stdout+" (파일 경로 누락) 입력 시 에러 반환 확인
func TestParseLogOutput_StdoutPlusEmpty(t *testing.T) {
	_, _, err := ParseLogOutput("stdout+")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidLogOutput)
}

// TestParseLogOutput_InvalidPath - 존재할 수 없는 경로 입력 시 에러 반환 확인
func TestParseLogOutput_InvalidPath(t *testing.T) {
	// /dev/null/impossible 은 디렉토리 생성이 불가능한 경로
	_, _, err := ParseLogOutput("/dev/null/impossible/test.log")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidLogOutput)
}
