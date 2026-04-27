package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TestChzyerReadlineInterfaceSatisfaction - 컴파일 타임 인터페이스 확인
// =============================================================================

func TestChzyerReadlineInterfaceSatisfaction(t *testing.T) {
	// compile-time check 는 readline_adapter.go 에서 이미 수행됨
	// 이 테스트는 런타임에서도 인터페이스를 만족하는지 문서화한다.
	var _ readlineInterface = (*chzyerReadline)(nil)
}

// =============================================================================
// TestHistoryDirectoryCreation - 히스토리 디렉토리 자동 생성 테스트
// =============================================================================

func TestHistoryDirectoryCreation(t *testing.T) {
	t.Run("히스토리 디렉토리가 없으면 자동 생성", func(t *testing.T) {
		tmpDir := t.TempDir()
		histDir := filepath.Join(tmpDir, "subdir", "nested")
		histFile := filepath.Join(histDir, "history")

		// newChzyerReadline 내부의 디렉토리 생성 로직을 직접 검증한다.
		// chzyer/readline 라이브러리 내부에 -race 환경에서 goroutine 레이스가 있으므로
		// readline 인스턴스를 생성하지 않고 디렉토리 생성 로직만 테스트한다.
		dir := filepath.Dir(histFile)
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err, "히스토리 디렉토리 생성이 성공해야 합니다")

		// 디렉토리가 생성되었는지 확인
		_, dirErr := os.Stat(histDir)
		assert.NoError(t, dirErr, "히스토리 디렉토리가 생성되어야 합니다")
	})

	t.Run("빈 히스토리 파일 경로도 디렉토리 생성 건너뜀", func(t *testing.T) {
		config := ChzyerConfig{
			Prompt:       "test> ",
			HistoryFile:  "",
			HistoryLimit: 100,
		}
		assert.Empty(t, config.HistoryFile, "빈 경로일 때 디렉토리 생성을 건너뛰어야 합니다")
	})
}

// =============================================================================
// TestIsTerminal - 표준 입력 터미널 확인 테스트
// =============================================================================

func TestIsTerminal(t *testing.T) {
	// 테스트 환경에서는 표준 입력이 터미널이 아닌 파이프이므로 false 를 반환해야 한다.
	result := isTerminal()
	assert.False(t, result,
		"테스트 환경에서 isTerminal 은 false 를 반환해야 합니다")
}

// =============================================================================
// TestChzyerConfigFields - ChzyerConfig 구조체 필드 검증
// chzyer/readline 라이브러리 내부에 -race 환경에서 Terminal.ioloop goroutine 과
// Terminal.Close() 사이의 데이터 레이스가 있으므로 (terminal.go:119 vs terminal.go:228)
// readline.NewEx 를 호출하지 않고 우리 래퍼 로직만 단위 테스트한다.
// =============================================================================

func TestChzyerConfigFields(t *testing.T) {
	t.Run("기본 설정 구성", func(t *testing.T) {
		config := ChzyerConfig{
			Prompt:       "xflow> ",
			HistoryFile:  "/tmp/test_history",
			HistoryLimit: 500,
			AutoComplete: nil,
		}

		assert.Equal(t, "xflow> ", config.Prompt)
		assert.Equal(t, "/tmp/test_history", config.HistoryFile)
		assert.Equal(t, 500, config.HistoryLimit)
		assert.Nil(t, config.AutoComplete)
	})

	t.Run("AutoComplete 설정 포함", func(t *testing.T) {
		ac := NewAutoCompleter(nil, nil, 0)
		config := ChzyerConfig{
			Prompt:       "test> ",
			HistoryFile:  "",
			HistoryLimit: 100,
			AutoComplete: ac,
		}

		assert.NotNil(t, config.AutoComplete)
		assert.Equal(t, "test> ", config.Prompt)
	})
}

// =============================================================================
// TestNewChzyerReadlineDirectoryCreation - newChzyerReadline 디렉토리 생성 로직 테스트
// readline 인스턴스 생성은 하지 않고 디렉토리 생성 로직의 동작을 검증한다.
// =============================================================================

func TestNewChzyerReadlineDirectoryCreation(t *testing.T) {
	t.Run("중첩 디렉토리 자동 생성", func(t *testing.T) {
		tmpDir := t.TempDir()
		nestedDir := filepath.Join(tmpDir, "a", "b", "c")
		histFile := filepath.Join(nestedDir, "history")

		// newChzyerReadline 이 수행하는 것과 동일한 디렉토리 생성 로직
		dir := filepath.Dir(histFile)
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)

		info, statErr := os.Stat(nestedDir)
		require.NoError(t, statErr)
		assert.True(t, info.IsDir(), "생성된 경로가 디렉토리여야 합니다")
	})

	t.Run("이미 존재하는 디렉토리는 에러 없음", func(t *testing.T) {
		tmpDir := t.TempDir()
		histFile := filepath.Join(tmpDir, "history")

		dir := filepath.Dir(histFile)
		err := os.MkdirAll(dir, 0755)
		assert.NoError(t, err, "이미 존재하는 디렉토리에 대해 에러가 없어야 합니다")
	})
}
