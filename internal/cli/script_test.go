package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TestParseScriptFile - 스크립트 파일 파싱 테스트
// REQ-SCR-001: 스크립트 파일 형식 (주석, 빈 줄, 공백 처리)
// REQ-SCR-008: 파일 미존재 에러
// =============================================================================

func TestParseScriptFile(t *testing.T) {
	tests := []struct {
		name        string
		content     string // 파일 내용 (빈 문자열이면 파일 생성하지 않음)
		createFile  bool
		expected    []ScriptLine
		expectError bool
		errorSubstr string // 에러 메시지에 포함될 부분 문자열
	}{
		{
			name:       "일반 명령어 파싱",
			content:    "version\nflow list\n",
			createFile: true,
			expected: []ScriptLine{
				{LineNumber: 1, Command: "version"},
				{LineNumber: 2, Command: "flow list"},
			},
		},
		{
			name:       "주석은 무시",
			content:    "# 이것은 주석입니다\nversion\n# 또 다른 주석\nflow list\n",
			createFile: true,
			expected: []ScriptLine{
				{LineNumber: 2, Command: "version"},
				{LineNumber: 4, Command: "flow list"},
			},
		},
		{
			name:       "빈 줄은 무시",
			content:    "version\n\n\nflow list\n\n",
			createFile: true,
			expected: []ScriptLine{
				{LineNumber: 1, Command: "version"},
				{LineNumber: 4, Command: "flow list"},
			},
		},
		{
			name:       "후행 공백 제거",
			content:    "version   \n  flow list  \n",
			createFile: true,
			expected: []ScriptLine{
				{LineNumber: 1, Command: "version"},
				{LineNumber: 2, Command: "flow list"},
			},
		},
		{
			name:       "주석과 빈 줄 혼합",
			content:    "# 설정 스크립트\n\nversion\n\n# 플로우 목록 조회\nflow list\n\n# 끝\n",
			createFile: true,
			expected: []ScriptLine{
				{LineNumber: 3, Command: "version"},
				{LineNumber: 6, Command: "flow list"},
			},
		},
		{
			name:       "빈 파일",
			content:    "",
			createFile: true,
			expected:   []ScriptLine{},
		},
		{
			name:       "주석만 있는 파일",
			content:    "# 주석 1\n# 주석 2\n",
			createFile: true,
			expected:   []ScriptLine{},
		},
		{
			name:        "파일 미존재 에러",
			createFile:  false,
			expectError: true,
			errorSubstr: "nonexistent_file.xflow",
		},
		{
			name:       "공백만 있는 줄은 무시",
			content:    "version\n   \n\t\nflow list\n",
			createFile: true,
			expected: []ScriptLine{
				{LineNumber: 1, Command: "version"},
				{LineNumber: 4, Command: "flow list"},
			},
		},
		{
			name:       "선행 공백 제거",
			content:    "  version\n\tflow list\n",
			createFile: true,
			expected: []ScriptLine{
				{LineNumber: 1, Command: "version"},
				{LineNumber: 2, Command: "flow list"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filePath string

			if tt.createFile {
				tmpDir := t.TempDir()
				filePath = filepath.Join(tmpDir, "test.xflow")
				err := os.WriteFile(filePath, []byte(tt.content), 0644)
				require.NoError(t, err, "테스트 파일 생성 실패")
			} else {
				filePath = filepath.Join(t.TempDir(), "nonexistent_file.xflow")
			}

			lines, err := ParseScriptFile(filePath)

			if tt.expectError {
				require.Error(t, err,
					"에러가 반환되어야 합니다")
				if tt.errorSubstr != "" {
					assert.Contains(t, err.Error(), tt.errorSubstr,
						"에러 메시지에 '%s' 가 포함되어야 합니다", tt.errorSubstr)
				}
				return
			}

			require.NoError(t, err, "파싱 에러가 없어야 합니다")
			assert.Equal(t, tt.expected, lines,
				"파싱 결과가 예상과 일치해야 합니다")
		})
	}
}

// =============================================================================
// TestScriptExecutor_Execute - 스크립트 실행 테스트
// REQ-SCR-004: 순차 실행
// REQ-SCR-005: 에러 시 중단 (기본값)
// REQ-SCR-006: 에러 시 계속 (--continue-on-error)
// =============================================================================

func TestScriptExecutor_Execute(t *testing.T) {
	t.Run("모든 명령어 성공", func(t *testing.T) {
		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, false, &buf)

		lines := []ScriptLine{
			{LineNumber: 1, Command: "version"},
			{LineNumber: 2, Command: "version"},
		}

		result := executor.Execute(lines)

		assert.Equal(t, 2, result.Total,
			"전체 명령어 수가 2여야 합니다")
		assert.Equal(t, 2, result.Success,
			"성공 명령어 수가 2여야 합니다")
		assert.Equal(t, 0, result.Failed,
			"실패 명령어 수가 0이어야 합니다")
		assert.Equal(t, 0, result.Skipped,
			"건너뛴 명령어 수가 0이어야 합니다")
		assert.Empty(t, result.Errors,
			"에러 목록이 비어있어야 합니다")
	})

	t.Run("에러 시 중단 (기본 동작)", func(t *testing.T) {
		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, false, &buf)

		lines := []ScriptLine{
			{LineNumber: 1, Command: "version"},
			{LineNumber: 2, Command: "nonexistent_command_xyz"},
			{LineNumber: 3, Command: "version"},
		}

		result := executor.Execute(lines)

		assert.Equal(t, 3, result.Total,
			"전체 명령어 수가 3이어야 합니다")
		assert.Equal(t, 1, result.Success,
			"성공 명령어 수가 1이어야 합니다")
		assert.Equal(t, 1, result.Failed,
			"실패 명령어 수가 1이어야 합니다")
		assert.Equal(t, 1, result.Skipped,
			"건너뛴 명령어 수가 1이어야 합니다")
		assert.Len(t, result.Errors, 1,
			"에러 목록에 1개의 에러가 있어야 합니다")
		assert.Equal(t, 2, result.Errors[0].Line,
			"에러가 발생한 줄 번호가 2여야 합니다")
	})

	t.Run("에러 시 계속 (--continue-on-error)", func(t *testing.T) {
		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, true, &buf)

		lines := []ScriptLine{
			{LineNumber: 1, Command: "version"},
			{LineNumber: 2, Command: "nonexistent_command_xyz"},
			{LineNumber: 3, Command: "version"},
		}

		result := executor.Execute(lines)

		assert.Equal(t, 3, result.Total,
			"전체 명령어 수가 3이어야 합니다")
		assert.Equal(t, 2, result.Success,
			"성공 명령어 수가 2여야 합니다")
		assert.Equal(t, 1, result.Failed,
			"실패 명령어 수가 1이어야 합니다")
		assert.Equal(t, 0, result.Skipped,
			"건너뛴 명령어 수가 0이어야 합니다 (계속 실행)")
		assert.Len(t, result.Errors, 1,
			"에러 목록에 1개의 에러가 있어야 합니다")
	})

	t.Run("빈 스크립트 실행", func(t *testing.T) {
		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, false, &buf)

		lines := []ScriptLine{}

		result := executor.Execute(lines)

		assert.Equal(t, 0, result.Total,
			"전체 명령어 수가 0이어야 합니다")
		assert.Equal(t, 0, result.Success,
			"성공 명령어 수가 0이어야 합니다")
		assert.Equal(t, 0, result.Failed,
			"실패 명령어 수가 0이어야 합니다")
		assert.Equal(t, 0, result.Skipped,
			"건너뛴 명령어 수가 0이어야 합니다")
	})

	t.Run("에러 시 중단 - 첫 번째 명령어 실패", func(t *testing.T) {
		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, false, &buf)

		lines := []ScriptLine{
			{LineNumber: 1, Command: "nonexistent_command_xyz"},
			{LineNumber: 2, Command: "version"},
			{LineNumber: 3, Command: "version"},
		}

		result := executor.Execute(lines)

		assert.Equal(t, 3, result.Total,
			"전체 명령어 수가 3이어야 합니다")
		assert.Equal(t, 0, result.Success,
			"성공 명령어 수가 0이어야 합니다")
		assert.Equal(t, 1, result.Failed,
			"실패 명령어 수가 1이어야 합니다")
		assert.Equal(t, 2, result.Skipped,
			"건너뛴 명령어 수가 2여야 합니다")
	})

	t.Run("에러 시 계속 - 여러 에러", func(t *testing.T) {
		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, true, &buf)

		lines := []ScriptLine{
			{LineNumber: 1, Command: "nonexistent_cmd_1"},
			{LineNumber: 2, Command: "version"},
			{LineNumber: 3, Command: "nonexistent_cmd_2"},
		}

		result := executor.Execute(lines)

		assert.Equal(t, 3, result.Total,
			"전체 명령어 수가 3이어야 합니다")
		assert.Equal(t, 1, result.Success,
			"성공 명령어 수가 1이어야 합니다")
		assert.Equal(t, 2, result.Failed,
			"실패 명령어 수가 2여야 합니다")
		assert.Equal(t, 0, result.Skipped,
			"건너뛴 명령어 수가 0이어야 합니다")
		assert.Len(t, result.Errors, 2,
			"에러 목록에 2개의 에러가 있어야 합니다")
	})

	t.Run("실행 중 진행 메시지 출력", func(t *testing.T) {
		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, false, &buf)

		lines := []ScriptLine{
			{LineNumber: 1, Command: "version"},
		}

		executor.Execute(lines)

		output := buf.String()
		assert.Contains(t, output, "[1]",
			"진행 메시지에 줄 번호가 포함되어야 합니다")
		assert.Contains(t, output, "version",
			"진행 메시지에 명령어가 포함되어야 합니다")
	})
}

// =============================================================================
// TestScriptResult_PrintSummary - 실행 요약 출력 테스트
// REQ-SCR-009: 실행 요약 출력
// =============================================================================

func TestScriptResult_PrintSummary(t *testing.T) {
	t.Run("모두 성공 요약", func(t *testing.T) {
		result := &ScriptResult{
			Total:   3,
			Success: 3,
			Failed:  0,
			Skipped: 0,
		}

		var buf bytes.Buffer
		result.PrintSummary(&buf)

		output := buf.String()
		assert.Contains(t, output, "3",
			"전체 수가 출력에 포함되어야 합니다")
		assert.Contains(t, output, "0",
			"실패 수가 출력에 포함되어야 합니다")
	})

	t.Run("실패 포함 요약", func(t *testing.T) {
		result := &ScriptResult{
			Total:   5,
			Success: 3,
			Failed:  1,
			Skipped: 1,
			Errors: []ScriptError{
				{Line: 3, Command: "bad_cmd", Error: errors.New("unknown command")},
			},
		}

		var buf bytes.Buffer
		result.PrintSummary(&buf)

		output := buf.String()
		assert.Contains(t, output, "5",
			"전체 수가 출력에 포함되어야 합니다")
		assert.Contains(t, output, "3",
			"성공 수가 출력에 포함되어야 합니다")
		assert.Contains(t, output, "1",
			"실패 수가 출력에 포함되어야 합니다")
		// 실패 상세 정보 출력
		assert.Contains(t, output, "bad_cmd",
			"실패한 명령어가 출력에 포함되어야 합니다")
	})

	t.Run("빈 스크립트 요약", func(t *testing.T) {
		result := &ScriptResult{
			Total:   0,
			Success: 0,
			Failed:  0,
			Skipped: 0,
		}

		var buf bytes.Buffer
		result.PrintSummary(&buf)

		output := buf.String()
		assert.NotEmpty(t, output,
			"빈 스크립트도 요약이 출력되어야 합니다")
	})
}

// =============================================================================
// TestScriptResult_HasErrors - 에러 존재 여부 테스트
// REQ-SCR-007: 종료 코드
// =============================================================================

func TestScriptResult_HasErrors(t *testing.T) {
	tests := []struct {
		name     string
		result   ScriptResult
		expected bool
	}{
		{
			name:     "에러 없음",
			result:   ScriptResult{Total: 3, Success: 3, Failed: 0},
			expected: false,
		},
		{
			name:     "에러 있음",
			result:   ScriptResult{Total: 3, Success: 2, Failed: 1},
			expected: true,
		},
		{
			name:     "빈 결과",
			result:   ScriptResult{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.result.HasErrors(),
				"HasErrors() 결과가 %v 여야 합니다", tt.expected)
		})
	}
}

// =============================================================================
// TestScriptExecutor_ExecuteWithFile - 파일 기반 통합 테스트
// REQ-SCR-002: CLI 스크립트 실행
// =============================================================================

func TestScriptExecutor_ExecuteWithFile(t *testing.T) {
	t.Run("스크립트 파일 파싱 후 실행", func(t *testing.T) {
		// 임시 스크립트 파일 생성
		tmpDir := t.TempDir()
		scriptPath := filepath.Join(tmpDir, "test.xflow")
		content := "# 테스트 스크립트\nversion\n\n# 다시 버전 출력\nversion\n"
		err := os.WriteFile(scriptPath, []byte(content), 0644)
		require.NoError(t, err)

		// 파싱
		lines, err := ParseScriptFile(scriptPath)
		require.NoError(t, err)
		assert.Len(t, lines, 2, "명령어 2개가 파싱되어야 합니다")

		// 실행
		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, false, &buf)
		result := executor.Execute(lines)

		assert.Equal(t, 2, result.Total)
		assert.Equal(t, 2, result.Success)
		assert.Equal(t, 0, result.Failed)
		assert.False(t, result.HasErrors())
	})
}

// =============================================================================
// TestScriptExecutorErrorInfo - 에러 정보 검증
// REQ-SCR-005: 에러 시 중단 - 실패 명령어 정보 리포트
// =============================================================================

func TestScriptExecutorErrorInfo(t *testing.T) {
	session := newTestSession(t)
	var buf bytes.Buffer
	executor := NewScriptExecutor(session, false, &buf)

	lines := []ScriptLine{
		{LineNumber: 5, Command: "nonexistent_command_abc"},
	}

	result := executor.Execute(lines)

	require.Len(t, result.Errors, 1, "에러가 1개 있어야 합니다")
	assert.Equal(t, 5, result.Errors[0].Line,
		"에러 줄 번호가 5여야 합니다")
	assert.Equal(t, "nonexistent_command_abc", result.Errors[0].Command,
		"에러 명령어가 일치해야 합니다")
	assert.NotNil(t, result.Errors[0].Error,
		"에러 객체가 nil 이 아니어야 합니다")

	// 에러 메시지 출력 확인
	output := buf.String()
	assert.Contains(t, output, "5",
		"에러 출력에 줄 번호가 포함되어야 합니다")
}

// =============================================================================
// TestParseScriptFile_LineNumbers - 줄 번호 정확성 테스트
// =============================================================================

func TestParseScriptFile_LineNumbers(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "lines.xflow")

	// 줄 1: 주석, 줄 2: 빈 줄, 줄 3: 명령어, 줄 4: 주석, 줄 5: 명령어
	content := "# 주석\n\nversion\n# 주석\nflow list\n"
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	lines, err := ParseScriptFile(filePath)
	require.NoError(t, err)

	require.Len(t, lines, 2)
	assert.Equal(t, 3, lines[0].LineNumber,
		"첫 번째 명령어의 줄 번호가 3이어야 합니다")
	assert.Equal(t, 5, lines[1].LineNumber,
		"두 번째 명령어의 줄 번호가 5여야 합니다")
}

// =============================================================================
// TestSourceCommand_REPL - REPL 에서 source 명령어 테스트
// REQ-SCR-003: REPL 스크립트 실행
// =============================================================================

func TestSourceCommand_REPL(t *testing.T) {
	t.Run("source 명령어로 스크립트 실행", func(t *testing.T) {
		// 임시 스크립트 파일 생성
		tmpDir := t.TempDir()
		scriptPath := filepath.Join(tmpDir, "test.xflow")
		content := "version\n"
		err := os.WriteFile(scriptPath, []byte(content), 0644)
		require.NoError(t, err)

		session := newTestSession(t)
		handled, exit := session.handleSpecialCommand("source " + scriptPath)

		assert.True(t, handled,
			"source 명령어는 특수 명령어로 처리되어야 합니다")
		assert.False(t, exit,
			"source 명령어는 세션을 종료하지 않아야 합니다")

		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "실행:",
			"진행 메시지가 출력되어야 합니다")
		assert.Contains(t, output, "스크립트 실행 요약",
			"요약이 출력되어야 합니다")
	})

	t.Run("run 명령어로 스크립트 실행", func(t *testing.T) {
		tmpDir := t.TempDir()
		scriptPath := filepath.Join(tmpDir, "test.xflow")
		content := "version\n"
		err := os.WriteFile(scriptPath, []byte(content), 0644)
		require.NoError(t, err)

		session := newTestSession(t)
		handled, exit := session.handleSpecialCommand("run " + scriptPath)

		assert.True(t, handled,
			"run 명령어는 특수 명령어로 처리되어야 합니다")
		assert.False(t, exit,
			"run 명령어는 세션을 종료하지 않아야 합니다")

		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "스크립트 실행 요약",
			"요약이 출력되어야 합니다")
	})

	t.Run("source 파일경로 없이 호출", func(t *testing.T) {
		session := newTestSession(t)
		handled, exit := session.handleSpecialCommand("source")

		assert.True(t, handled,
			"source 명령어는 특수 명령어로 처리되어야 합니다")
		assert.False(t, exit,
			"세션을 종료하지 않아야 합니다")

		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "사용법",
			"사용법 안내가 출력되어야 합니다")
	})

	t.Run("source 존재하지 않는 파일", func(t *testing.T) {
		session := newTestSession(t)
		handled, _ := session.handleSpecialCommand("source /nonexistent/path/script.xflow")

		assert.True(t, handled)

		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "스크립트 오류",
			"에러 메시지가 출력되어야 합니다")
	})
}

// =============================================================================
// TestScriptCmd - CLI script 서브커맨드 테스트
// REQ-SCR-002: CLI 스크립트 실행
// =============================================================================

func TestScriptCmd(t *testing.T) {
	t.Run("script 서브커맨드가 루트에 등록됨", func(t *testing.T) {
		rootCmd := NewRootCmd()

		var scriptCmd *cobra.Command
		for _, sub := range rootCmd.Commands() {
			if sub.Use == "script" {
				scriptCmd = sub
				break
			}
		}

		require.NotNil(t, scriptCmd,
			"script 서브커맨드가 루트 커맨드에 등록되어 있어야 합니다")
		assert.NotEmpty(t, scriptCmd.Short,
			"Short 설명이 비어있으면 안됩니다")
	})
}

// =============================================================================
// TestHelpIncludesSourceRun - help 출력에 source/run 포함 테스트
// REQ-SCR-003: REPL source/run 명령어 등록
// =============================================================================

func TestHelpIncludesSourceRun(t *testing.T) {
	session := newTestSession(t)
	buf := session.writer.(*bytes.Buffer)

	session.handleSpecialCommand("help")

	output := buf.String()

	assert.Contains(t, output, "source",
		"도움말에 source 명령어가 포함되어야 합니다")
	assert.Contains(t, output, "run",
		"도움말에 run 명령어가 포함되어야 합니다")
}

// =============================================================================
// TestExpandEnvInLines - 환경 변수 치환 테스트
// REQ-SCR-016, REQ-SCR-017: 환경 변수 치환
// =============================================================================

func TestExpandEnvInLines(t *testing.T) {
	t.Run("$VAR 형식 환경 변수 치환", func(t *testing.T) {
		t.Setenv("XFLOW_TEST_VAR", "hello")

		lines := []ScriptLine{
			{LineNumber: 1, Command: "node type --name $XFLOW_TEST_VAR"},
		}

		expanded := ExpandEnvInLines(lines)

		require.Len(t, expanded, 1)
		assert.Equal(t, "node type --name hello", expanded[0].Command,
			"$VAR 형식의 환경 변수가 치환되어야 합니다")
		assert.Equal(t, 1, expanded[0].LineNumber,
			"줄 번호가 보존되어야 합니다")
	})

	t.Run("${VAR} 형식 환경 변수 치환", func(t *testing.T) {
		t.Setenv("XFLOW_TEST_HOST", "localhost")

		lines := []ScriptLine{
			{LineNumber: 5, Command: "flow run --host ${XFLOW_TEST_HOST}"},
		}

		expanded := ExpandEnvInLines(lines)

		require.Len(t, expanded, 1)
		assert.Equal(t, "flow run --host localhost", expanded[0].Command,
			"${VAR} 형식의 환경 변수가 치환되어야 합니다")
	})

	t.Run("미정의 환경 변수는 빈 문자열로 치환", func(t *testing.T) {
		// XFLOW_UNDEFINED_VAR_12345 는 설정되지 않은 변수
		lines := []ScriptLine{
			{LineNumber: 1, Command: "node type --name $XFLOW_UNDEFINED_VAR_12345"},
		}

		expanded := ExpandEnvInLines(lines)

		require.Len(t, expanded, 1)
		assert.Equal(t, "node type --name ", expanded[0].Command,
			"미정의 환경 변수는 빈 문자열이 되어야 합니다")
	})

	t.Run("혼합 환경 변수 치환", func(t *testing.T) {
		t.Setenv("XFLOW_TEST_FLOW", "my-flow")
		t.Setenv("XFLOW_TEST_HOST2", "192.168.1.1")

		lines := []ScriptLine{
			{LineNumber: 3, Command: "flow run $XFLOW_TEST_FLOW --host ${XFLOW_TEST_HOST2}"},
		}

		expanded := ExpandEnvInLines(lines)

		require.Len(t, expanded, 1)
		assert.Equal(t, "flow run my-flow --host 192.168.1.1", expanded[0].Command,
			"$VAR 와 ${VAR} 혼합 치환이 되어야 합니다")
	})

	t.Run("환경 변수가 없는 명령어는 그대로 유지", func(t *testing.T) {
		lines := []ScriptLine{
			{LineNumber: 1, Command: "version"},
			{LineNumber: 2, Command: "flow list"},
		}

		expanded := ExpandEnvInLines(lines)

		require.Len(t, expanded, 2)
		assert.Equal(t, "version", expanded[0].Command)
		assert.Equal(t, "flow list", expanded[1].Command)
	})

	t.Run("빈 라인 목록", func(t *testing.T) {
		lines := []ScriptLine{}

		expanded := ExpandEnvInLines(lines)

		assert.Empty(t, expanded, "빈 입력은 빈 결과여야 합니다")
	})

	t.Run("원본 슬라이스가 변경되지 않음", func(t *testing.T) {
		t.Setenv("XFLOW_TEST_ORIG", "replaced")

		lines := []ScriptLine{
			{LineNumber: 1, Command: "node type --name $XFLOW_TEST_ORIG"},
		}

		_ = ExpandEnvInLines(lines)

		assert.Equal(t, "node type --name $XFLOW_TEST_ORIG", lines[0].Command,
			"원본 슬라이스의 Command 가 변경되면 안됩니다")
	})
}

// =============================================================================
// TestScriptExecutor_ExpandEnv - Execute 에서 환경 변수 치환 통합 테스트
// REQ-SCR-016, REQ-SCR-017
// =============================================================================

func TestScriptExecutor_ExpandEnv(t *testing.T) {
	t.Run("Execute 시 환경 변수 자동 치환", func(t *testing.T) {
		t.Setenv("XFLOW_EXEC_TEST", "version")

		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, false, &buf)

		lines := []ScriptLine{
			{LineNumber: 1, Command: "$XFLOW_EXEC_TEST"},
		}

		result := executor.Execute(lines)

		assert.Equal(t, 1, result.Success,
			"환경 변수로 치환된 명령어 'version' 이 성공해야 합니다")
		assert.Equal(t, 0, result.Failed)
	})

	t.Run("expandEnv=false 이면 치환하지 않음", func(t *testing.T) {
		t.Setenv("XFLOW_EXEC_SKIP", "version")

		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, false, &buf)
		executor.SetExpandEnv(false)

		lines := []ScriptLine{
			{LineNumber: 1, Command: "$XFLOW_EXEC_SKIP"},
		}

		result := executor.Execute(lines)

		// $XFLOW_EXEC_SKIP 가 치환되지 않으므로 존재하지 않는 명령어로 실패
		assert.Equal(t, 1, result.Failed,
			"expandEnv=false 이면 환경 변수가 치환되지 않아야 합니다")
	})
}

// =============================================================================
// TestScriptExecutor_DryRun - dry-run 모드 테스트
// REQ-SCR-018: dry-run 모드
// =============================================================================

func TestScriptExecutor_DryRun(t *testing.T) {
	t.Run("dry-run 모드에서 명령어 미실행", func(t *testing.T) {
		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, false, &buf)
		executor.SetDryRun(true)

		lines := []ScriptLine{
			{LineNumber: 1, Command: "version"},
			{LineNumber: 2, Command: "nonexistent_cmd_should_not_fail"},
			{LineNumber: 3, Command: "flow list"},
		}

		result := executor.Execute(lines)

		assert.Equal(t, 3, result.Total,
			"전체 명령어 수가 3이어야 합니다")
		assert.Equal(t, 0, result.Success,
			"dry-run 모드에서 성공 카운트는 0이어야 합니다")
		assert.Equal(t, 0, result.Failed,
			"dry-run 모드에서 실패 카운트는 0이어야 합니다")
		assert.Equal(t, 0, result.Skipped,
			"dry-run 모드에서 건너뛴 카운트는 0이어야 합니다")
		assert.Empty(t, result.Errors,
			"dry-run 모드에서 에러가 없어야 합니다")
	})

	t.Run("dry-run 출력 형식 확인", func(t *testing.T) {
		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, false, &buf)
		executor.SetDryRun(true)

		lines := []ScriptLine{
			{LineNumber: 5, Command: "version"},
		}

		executor.Execute(lines)

		output := buf.String()
		assert.Contains(t, output, "(dry-run)",
			"dry-run 출력에 '(dry-run)' 이 포함되어야 합니다")
		assert.Contains(t, output, "[5]",
			"dry-run 출력에 줄 번호가 포함되어야 합니다")
		assert.Contains(t, output, "version",
			"dry-run 출력에 명령어가 포함되어야 합니다")
	})

	t.Run("dry-run + verbose 조합 (dry-run 우선)", func(t *testing.T) {
		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, false, &buf)
		executor.SetDryRun(true)
		executor.SetVerbose(true)

		lines := []ScriptLine{
			{LineNumber: 1, Command: "version"},
		}

		result := executor.Execute(lines)

		output := buf.String()
		assert.Contains(t, output, "(dry-run)",
			"dry-run 이 verbose 보다 우선해야 합니다")
		assert.Equal(t, 0, result.Success,
			"dry-run 모드에서 명령어가 실행되지 않아야 합니다")
		assert.Equal(t, 0, result.Failed,
			"dry-run 모드에서 명령어가 실행되지 않아야 합니다")
	})
}

// =============================================================================
// TestScriptExecutor_Verbose - verbose 모드 테스트
// REQ-SCR-018: verbose 모드
// =============================================================================

func TestScriptExecutor_Verbose(t *testing.T) {
	t.Run("verbose 모드 성공 시 시작/완료 메시지", func(t *testing.T) {
		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, false, &buf)
		executor.SetVerbose(true)

		lines := []ScriptLine{
			{LineNumber: 1, Command: "version"},
		}

		executor.Execute(lines)

		output := buf.String()
		assert.Contains(t, output, "실행 시작",
			"verbose 출력에 '실행 시작' 이 포함되어야 합니다")
		assert.Contains(t, output, "실행 완료",
			"verbose 출력에 '실행 완료' 가 포함되어야 합니다")
		assert.Contains(t, output, "[1]",
			"verbose 출력에 줄 번호가 포함되어야 합니다")
	})

	t.Run("verbose 모드 실패 시 실패 메시지", func(t *testing.T) {
		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, true, &buf)
		executor.SetVerbose(true)

		lines := []ScriptLine{
			{LineNumber: 3, Command: "nonexistent_verbose_cmd"},
		}

		executor.Execute(lines)

		output := buf.String()
		assert.Contains(t, output, "실행 시작",
			"verbose 출력에 '실행 시작' 이 포함되어야 합니다")
		assert.Contains(t, output, "실패",
			"verbose 출력에 실패 정보가 포함되어야 합니다")
		assert.Contains(t, output, "[3]",
			"verbose 출력에 줄 번호가 포함되어야 합니다")
	})

	t.Run("verbose=false 이면 추가 메시지 없음", func(t *testing.T) {
		session := newTestSession(t)
		var buf bytes.Buffer
		executor := NewScriptExecutor(session, false, &buf)
		// verbose 는 기본적으로 false

		lines := []ScriptLine{
			{LineNumber: 1, Command: "version"},
		}

		executor.Execute(lines)

		output := buf.String()
		assert.NotContains(t, output, "실행 시작",
			"verbose=false 이면 '실행 시작' 이 출력되지 않아야 합니다")
		assert.NotContains(t, output, "실행 완료",
			"verbose=false 이면 '실행 완료' 가 출력되지 않아야 합니다")
	})
}

// =============================================================================
// TestScriptCmd_DryRunVerboseFlags - CLI 플래그 등록 테스트
// REQ-SCR-018: dry-run, verbose 플래그
// =============================================================================

func TestScriptCmd_DryRunVerboseFlags(t *testing.T) {
	t.Run("script 서브커맨드에 dry-run 플래그 등록", func(t *testing.T) {
		rootCmd := NewRootCmd()

		var scriptCmd *cobra.Command
		for _, sub := range rootCmd.Commands() {
			if sub.Use == "script" {
				scriptCmd = sub
				break
			}
		}

		require.NotNil(t, scriptCmd)

		dryRunFlag := scriptCmd.Flags().Lookup("dry-run")
		require.NotNil(t, dryRunFlag,
			"script 서브커맨드에 --dry-run 플래그가 등록되어야 합니다")
		assert.Equal(t, "false", dryRunFlag.DefValue,
			"--dry-run 기본값은 false 여야 합니다")
	})

	t.Run("script 서브커맨드에 verbose 플래그 등록", func(t *testing.T) {
		rootCmd := NewRootCmd()

		var scriptCmd *cobra.Command
		for _, sub := range rootCmd.Commands() {
			if sub.Use == "script" {
				scriptCmd = sub
				break
			}
		}

		require.NotNil(t, scriptCmd)

		verboseFlag := scriptCmd.Flags().Lookup("verbose")
		require.NotNil(t, verboseFlag,
			"script 서브커맨드에 --verbose 플래그가 등록되어야 합니다")
		assert.Equal(t, "false", verboseFlag.DefValue,
			"--verbose 기본값은 false 여야 합니다")
	})
}
