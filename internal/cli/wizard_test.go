package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// 테스트 헬퍼: Wizard 테스트용 루트 커맨드 생성
// =============================================================================

// newWizardTestRootCmd 는 Wizard 테스트용 루트 커맨드를 생성한다.
// flow deploy (ExactArgs(1)), flow create (-f 필수 플래그) 등 실제와 유사한 구조를 갖는다.
func newWizardTestRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "xflow",
		Short: "xflow - 테스트용 CLI",
	}

	// 글로벌 플래그
	pflags := rootCmd.PersistentFlags()
	pflags.String("format", "table", "출력 형식 (json|yaml|table|text)")
	pflags.Bool("verbose", false, "상세 출력")

	// flow 커맨드 그룹
	flowCmd := &cobra.Command{
		Use:   "flow",
		Short: "플로우 관리",
	}

	// flow deploy <id>
	deployCmd := &cobra.Command{
		Use:   "deploy <id>",
		Short: "플로우 배포",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "플로우 '%s' deploy 완료.\n", args[0])
			return nil
		},
	}

	// flow create -f <file>
	var filePath string
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "파일에서 플로우 생성",
		RunE: func(cmd *cobra.Command, args []string) error {
			if filePath == "" {
				return fmt.Errorf("파일 경로(-f)를 지정해야 합니다")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "플로우 생성 완료: %s\n", filePath)
			return nil
		},
	}
	createCmd.Flags().StringVarP(&filePath, "file", "f", "", "플로우 정의 파일 경로 (JSON/YAML)")

	// flow list (인자 없음, --format 플래그만)
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "플로우 목록 조회",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "플로우 목록 출력")
			return nil
		},
	}

	// flow get <id>
	getCmd := &cobra.Command{
		Use:   "get <id>",
		Short: "플로우 상세 조회",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "플로우 상세: %s\n", args[0])
			return nil
		},
	}

	// flow delete <id> --yes
	deleteCmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "플로우 삭제",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "플로우 삭제: %s\n", args[0])
			return nil
		},
	}
	deleteCmd.Flags().Bool("yes", false, "확인 없이 삭제")

	flowCmd.AddCommand(deployCmd)
	flowCmd.AddCommand(createCmd)
	flowCmd.AddCommand(listCmd)
	flowCmd.AddCommand(getCmd)
	flowCmd.AddCommand(deleteCmd)

	// agent 커맨드
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "에이전트 관리",
	}

	rootCmd.AddCommand(flowCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "버전 정보",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "xflow version dev")
			return nil
		},
	})

	return rootCmd
}

// =============================================================================
// TestNewWizardRunner - WizardRunner 생성 테스트
// REQ-INT-012: Wizard 모드 진입
// =============================================================================

func TestNewWizardRunner(t *testing.T) {
	rootCmd := newWizardTestRootCmd()
	var client *Client
	var buf bytes.Buffer

	runner := NewWizardRunner(rootCmd, &client, &buf)

	require.NotNil(t, runner, "WizardRunner 가 nil 이면 안됩니다")
	assert.Equal(t, rootCmd, runner.rootCmd, "rootCmd 가 동일해야 합니다")
	assert.NotNil(t, runner.writer, "writer 가 nil 이면 안됩니다")
}

// =============================================================================
// TestFindCommand - Cobra 명령어 트리 탐색 테스트
// REQ-INT-012: Wizard 모드 진입 시 명령어 탐색
// =============================================================================

func TestFindCommand(t *testing.T) {
	rootCmd := newWizardTestRootCmd()
	var client *Client
	var buf bytes.Buffer
	runner := NewWizardRunner(rootCmd, &client, &buf)

	tests := []struct {
		name      string
		path      string
		wantUse   string
		wantError bool
	}{
		{
			name:      "flow deploy 명령어를 찾을 수 있다",
			path:      "flow deploy",
			wantUse:   "deploy <id>",
			wantError: false,
		},
		{
			name:      "flow 명령어를 찾을 수 있다",
			path:      "flow",
			wantUse:   "flow",
			wantError: false,
		},
		{
			name:      "flow create 명령어를 찾을 수 있다",
			path:      "flow create",
			wantUse:   "create",
			wantError: false,
		},
		{
			name:      "존재하지 않는 명령어는 에러를 반환한다",
			path:      "nonexistent",
			wantError: true,
		},
		{
			name:      "빈 경로는 에러를 반환한다",
			path:      "",
			wantError: true,
		},
		{
			name:      "존재하지 않는 서브커맨드는 에러를 반환한다",
			path:      "flow nonexistent",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := runner.findCommand(tt.path)
			if tt.wantError {
				assert.Error(t, err,
					"경로 '%s' 는 에러를 반환해야 합니다", tt.path)
				assert.Nil(t, cmd)
			} else {
				require.NoError(t, err,
					"경로 '%s' 는 에러 없이 명령어를 찾아야 합니다", tt.path)
				require.NotNil(t, cmd)
				assert.Equal(t, tt.wantUse, cmd.Use,
					"찾은 명령어의 Use 가 '%s' 여야 합니다", tt.wantUse)
			}
		})
	}
}

// =============================================================================
// TestBuildQuestions - Cobra 명령어에서 프롬프트 질문 생성 테스트
// REQ-INT-013: 단계별 파라미터 수집
// =============================================================================

func TestBuildQuestions(t *testing.T) {
	rootCmd := newWizardTestRootCmd()
	var client *Client
	var buf bytes.Buffer
	runner := NewWizardRunner(rootCmd, &client, &buf)

	t.Run("필수 인자가 있는 명령어는 Required 질문을 생성한다", func(t *testing.T) {
		cmd, err := runner.findCommand("flow deploy")
		require.NoError(t, err)

		questions := runner.buildQuestions(cmd)

		// deploy <id> 는 ExactArgs(1) 이므로 필수 인자 질문이 있어야 한다
		require.NotEmpty(t, questions, "질문이 1개 이상이어야 합니다")

		// 첫 번째 질문은 필수 인자
		found := false
		for _, q := range questions {
			if q.Required && q.Name == "id" {
				found = true
				break
			}
		}
		assert.True(t, found, "필수 인자 'id' 에 대한 질문이 있어야 합니다")
	})

	t.Run("플래그가 있는 명령어는 플래그 질문을 생성한다", func(t *testing.T) {
		cmd, err := runner.findCommand("flow create")
		require.NoError(t, err)

		questions := runner.buildQuestions(cmd)

		// create 는 -f 플래그가 있으므로 질문이 있어야 한다
		found := false
		for _, q := range questions {
			if q.Name == "file" {
				found = true
				break
			}
		}
		assert.True(t, found, "플래그 'file' 에 대한 질문이 있어야 합니다")
	})

	t.Run("bool 플래그는 질문에 포함된다", func(t *testing.T) {
		cmd, err := runner.findCommand("flow delete")
		require.NoError(t, err)

		questions := runner.buildQuestions(cmd)

		// delete 는 --yes bool 플래그가 있다
		found := false
		for _, q := range questions {
			if q.Name == "yes" {
				found = true
				break
			}
		}
		assert.True(t, found, "bool 플래그 'yes' 에 대한 질문이 있어야 합니다")
	})

	t.Run("인자와 플래그가 없는 명령어는 빈 질문을 반환한다", func(t *testing.T) {
		cmd, err := runner.findCommand("flow list")
		require.NoError(t, err)

		questions := runner.buildQuestions(cmd)

		// list 는 인자도 로컬 플래그도 없으므로 빈 질문
		assert.Empty(t, questions, "인자와 로컬 플래그가 없으면 질문이 비어야 합니다")
	})
}

// =============================================================================
// TestBuildQuestionsWithDefaults - 기본값 포함 질문 생성 테스트
// REQ-INT-013: 기본값 제안
// =============================================================================

func TestBuildQuestionsWithDefaults(t *testing.T) {
	rootCmd := newWizardTestRootCmd()
	var client *Client
	var buf bytes.Buffer
	runner := NewWizardRunner(rootCmd, &client, &buf)

	cmd, err := runner.findCommand("flow delete")
	require.NoError(t, err)

	questions := runner.buildQuestions(cmd)

	// --yes 플래그의 기본값은 "false"
	for _, q := range questions {
		if q.Name == "yes" {
			assert.Equal(t, "false", q.Default,
				"bool 플래그 'yes' 의 기본값이 'false' 여야 합니다")
			return
		}
	}
	t.Error("플래그 'yes' 에 대한 질문을 찾을 수 없습니다")
}

// =============================================================================
// TestShowSummary - 수집된 파라미터 요약 테이블 테스트
// REQ-INT-015: 실행 전 요약 표시
// =============================================================================

func TestShowSummary(t *testing.T) {
	rootCmd := newWizardTestRootCmd()
	var client *Client
	var buf bytes.Buffer
	runner := NewWizardRunner(rootCmd, &client, &buf)

	cmd, err := runner.findCommand("flow deploy")
	require.NoError(t, err)

	answers := map[string]string{
		"id": "flow-001",
	}

	summary := runner.showSummary(cmd, answers)

	// 요약에는 파라미터 이름과 값이 포함되어야 한다
	assert.Contains(t, summary, "id", "요약에 'id' 파라미터가 포함되어야 합니다")
	assert.Contains(t, summary, "flow-001", "요약에 값 'flow-001' 이 포함되어야 합니다")
}

// =============================================================================
// TestWizardRunConfirm - Wizard 전체 흐름 (확인 실행) 테스트
// REQ-INT-015: 실행 전 확인 후 명령어 실행
// =============================================================================

func TestWizardRunConfirm(t *testing.T) {
	rootCmd := newWizardTestRootCmd()
	var client *Client
	var buf bytes.Buffer
	runner := NewWizardRunner(rootCmd, &client, &buf)

	// 출력을 버퍼로 설정
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	// askFn 모킹: 사용자가 id 를 입력
	runner.askFn = func(questions []*WizardQuestion) (map[string]string, error) {
		return map[string]string{"id": "flow-123"}, nil
	}

	// confirmFn 모킹: 사용자가 확인
	runner.confirmFn = func(message string) (bool, error) {
		return true, nil
	}

	err := runner.Run("flow deploy")
	require.NoError(t, err, "Wizard 실행에 에러가 없어야 합니다")

	output := buf.String()
	// Wizard 헤더 출력 확인
	assert.Contains(t, output, "=== Flow Deploy Wizard ===",
		"Wizard 헤더가 출력되어야 합니다")
	// 명령어 실행 결과 확인
	assert.Contains(t, output, "flow-123",
		"명령어 실행 결과에 id 가 포함되어야 합니다")
}

// =============================================================================
// TestWizardRunCancel - Wizard 전체 흐름 (취소) 테스트
// REQ-INT-015: 취소 시 메시지 표시
// =============================================================================

func TestWizardRunCancel(t *testing.T) {
	rootCmd := newWizardTestRootCmd()
	var client *Client
	var buf bytes.Buffer
	runner := NewWizardRunner(rootCmd, &client, &buf)

	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	// askFn 모킹
	runner.askFn = func(questions []*WizardQuestion) (map[string]string, error) {
		return map[string]string{"id": "flow-456"}, nil
	}

	// confirmFn 모킹: 사용자가 취소
	runner.confirmFn = func(message string) (bool, error) {
		return false, nil
	}

	err := runner.Run("flow deploy")
	require.NoError(t, err, "취소 시에도 에러가 없어야 합니다")

	output := buf.String()
	// 취소 메시지 확인
	assert.Contains(t, output, "작업이 취소되었습니다",
		"취소 시 '작업이 취소되었습니다.' 메시지가 출력되어야 합니다")
	// 명령어가 실행되지 않았는지 확인 (deploy 완료 메시지가 없어야 함)
	assert.NotContains(t, output, "deploy 완료",
		"취소 시 명령어가 실행되면 안됩니다")
}

// =============================================================================
// TestWizardInvalidCommand - 존재하지 않는 명령어 경로 테스트
// REQ-INT-012: 존재하지 않는 명령어 에러 처리
// =============================================================================

func TestWizardInvalidCommand(t *testing.T) {
	rootCmd := newWizardTestRootCmd()
	var client *Client
	var buf bytes.Buffer
	runner := NewWizardRunner(rootCmd, &client, &buf)

	err := runner.Run("nonexistent command")
	assert.Error(t, err, "존재하지 않는 명령어는 에러를 반환해야 합니다")
}

// =============================================================================
// TestWizardEmptyPath - 빈 명령어 경로 테스트
// REQ-INT-012: 빈 경로 에러 처리
// =============================================================================

func TestWizardEmptyPath(t *testing.T) {
	rootCmd := newWizardTestRootCmd()
	var client *Client
	var buf bytes.Buffer
	runner := NewWizardRunner(rootCmd, &client, &buf)

	err := runner.Run("")
	assert.Error(t, err, "빈 경로는 에러를 반환해야 합니다")
}

// =============================================================================
// TestExecuteFromAnswers - 답변 맵에서 Cobra 인자로 변환 및 실행 테스트
// REQ-INT-015: 수집된 파라미터로 명령어 실행
// =============================================================================

func TestExecuteFromAnswers(t *testing.T) {
	rootCmd := newWizardTestRootCmd()
	var client *Client
	var buf bytes.Buffer
	runner := NewWizardRunner(rootCmd, &client, &buf)

	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	t.Run("인자와 플래그를 올바르게 변환한다", func(t *testing.T) {
		buf.Reset()
		cmd, err := runner.findCommand("flow deploy")
		require.NoError(t, err)

		answers := map[string]string{
			"id": "flow-789",
		}

		err = runner.executeFromAnswers(cmd, answers)
		require.NoError(t, err, "executeFromAnswers 에 에러가 없어야 합니다")

		output := buf.String()
		assert.Contains(t, output, "flow-789",
			"실행 결과에 인자 값이 포함되어야 합니다")
	})

	t.Run("플래그가 있는 명령어 실행", func(t *testing.T) {
		buf.Reset()
		cmd, err := runner.findCommand("flow delete")
		require.NoError(t, err)

		answers := map[string]string{
			"id":  "flow-del-001",
			"yes": "true",
		}

		err = runner.executeFromAnswers(cmd, answers)
		require.NoError(t, err, "플래그 포함 실행에 에러가 없어야 합니다")

		output := buf.String()
		assert.Contains(t, output, "flow-del-001",
			"실행 결과에 인자 값이 포함되어야 합니다")
	})
}

// =============================================================================
// TestWizardHeader - Wizard 헤더 형식 테스트
// REQ-INT-012: "=== Flow Deploy Wizard ===" 헤더 표시
// =============================================================================

func TestWizardHeader(t *testing.T) {
	rootCmd := newWizardTestRootCmd()
	var client *Client
	var buf bytes.Buffer
	runner := NewWizardRunner(rootCmd, &client, &buf)

	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	// askFn 모킹 (질문 없는 명령어도 동작해야 함)
	runner.askFn = func(questions []*WizardQuestion) (map[string]string, error) {
		return map[string]string{}, nil
	}
	runner.confirmFn = func(message string) (bool, error) {
		return true, nil
	}

	_ = runner.Run("flow list")

	output := buf.String()
	assert.Contains(t, output, "=== Flow List Wizard ===",
		"Wizard 헤더가 '=== Flow List Wizard ===' 형식이어야 합니다")
}

// =============================================================================
// TestWizardSummaryTableFormat - 요약 테이블 형식 테스트
// REQ-INT-015: 실행 전 요약 표시
// =============================================================================

func TestWizardSummaryTableFormat(t *testing.T) {
	rootCmd := newWizardTestRootCmd()
	var client *Client
	var buf bytes.Buffer
	runner := NewWizardRunner(rootCmd, &client, &buf)

	cmd, err := runner.findCommand("flow deploy")
	require.NoError(t, err)

	answers := map[string]string{
		"id": "flow-summary-test",
	}

	summary := runner.showSummary(cmd, answers)

	// 요약 테이블에 구분선이 있어야 한다
	lines := strings.Split(summary, "\n")
	hasHeader := false
	for _, line := range lines {
		if strings.Contains(line, "파라미터") || strings.Contains(line, "값") {
			hasHeader = true
			break
		}
	}
	assert.True(t, hasHeader, "요약 테이블에 헤더가 있어야 합니다")
}

// =============================================================================
// TestWizardWithNoArgsCommand - 인자 없는 명령어의 Wizard 실행 테스트
// REQ-INT-013: 인자 없는 명령어는 바로 확인 단계로
// =============================================================================

func TestWizardWithNoArgsCommand(t *testing.T) {
	rootCmd := newWizardTestRootCmd()
	var client *Client
	var buf bytes.Buffer
	runner := NewWizardRunner(rootCmd, &client, &buf)

	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	executed := false
	runner.askFn = func(questions []*WizardQuestion) (map[string]string, error) {
		// 질문이 없어야 한다
		assert.Empty(t, questions, "인자 없는 명령어는 질문이 없어야 합니다")
		return map[string]string{}, nil
	}
	runner.confirmFn = func(message string) (bool, error) {
		executed = true
		return true, nil
	}

	err := runner.Run("flow list")
	require.NoError(t, err)
	assert.True(t, executed, "confirmFn 이 호출되어야 합니다")
}

// =============================================================================
// TestParseArgNames - Use 필드에서 인자 이름 추출 테스트
// =============================================================================

func TestParseArgNames(t *testing.T) {
	tests := []struct {
		name     string
		use      string
		expected []string
	}{
		{
			name:     "필수 인자 추출",
			use:      "deploy <id>",
			expected: []string{"id"},
		},
		{
			name:     "선택적 인자 추출",
			use:      "get [name]",
			expected: []string{"name"},
		},
		{
			name:     "인자 없는 명령어",
			use:      "list",
			expected: nil,
		},
		{
			name:     "복수 인자 추출",
			use:      "copy <source> <dest>",
			expected: []string{"source", "dest"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseArgNames(tt.use)
			assert.Equal(t, tt.expected, result,
				"Use '%s' 에서 추출한 인자가 일치해야 합니다", tt.use)
		})
	}
}

// =============================================================================
// TestCapitalize - 첫 글자 대문자 변환 테스트
// =============================================================================

func TestCapitalize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "소문자 문자열",
			input:    "flow",
			expected: "Flow",
		},
		{
			name:     "빈 문자열",
			input:    "",
			expected: "",
		},
		{
			name:     "이미 대문자인 문자열",
			input:    "Deploy",
			expected: "Deploy",
		},
		{
			name:     "한 글자 문자열",
			input:    "a",
			expected: "A",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := capitalize(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// TestWizardIntegrationWithREPL - REPL 에서 wizard 통합 테스트
// =============================================================================

func TestWizardIntegrationWithREPL(t *testing.T) {
	t.Run("wizard 인자 없이 호출하면 사용법 출력", func(t *testing.T) {
		session := newTestSession(t)
		handled, _ := session.handleSpecialCommand("wizard")
		assert.True(t, handled)

		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "사용법: wizard <명령어>",
			"인자 없이 wizard 호출 시 사용법이 출력되어야 합니다")
	})

	t.Run("wizard 에 존재하지 않는 명령어 전달 시 오류 출력", func(t *testing.T) {
		session := newTestSession(t)
		handled, _ := session.handleSpecialCommand("wizard nonexistent")
		assert.True(t, handled)

		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "Wizard 오류",
			"존재하지 않는 명령어 시 오류 메시지가 출력되어야 합니다")
	})
}
