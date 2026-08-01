package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TestTokenizeInput - 입력 문자열을 토큰으로 분리하는 함수 테스트
// REQ-INT-003: 명령어 파싱 및 실행
// =============================================================================

func TestTokenizeInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "기본 명령어 분리",
			input:    "flow list",
			expected: []string{"flow", "list"},
		},
		{
			name:     "플래그가 포함된 명령어",
			input:    "flow list --format json",
			expected: []string{"flow", "list", "--format", "json"},
		},
		{
			name:     "빈 입력은 빈 슬라이스 반환",
			input:    "",
			expected: []string{},
		},
		{
			name:     "공백만 있는 입력은 빈 슬라이스 반환",
			input:    "   ",
			expected: []string{},
		},
		{
			name:     "xflow 접두사 제거",
			input:    "xflow flow list",
			expected: []string{"flow", "list"},
		},
		{
			name:     "따옴표로 감싼 경로 처리",
			input:    `create -f "/path/with spaces/file.json"`,
			expected: []string{"create", "-f", "/path/with spaces/file.json"},
		},
		{
			name:     "작은따옴표로 감싼 값 처리",
			input:    `flow create --name 'my flow'`,
			expected: []string{"flow", "create", "--name", "my flow"},
		},
		{
			name:     "단일 단어 명령어",
			input:    "version",
			expected: []string{"version"},
		},
		{
			name:     "여러 공백 구분자 처리",
			input:    "flow   list   --format   json",
			expected: []string{"flow", "list", "--format", "json"},
		},
		{
			name:     "xflow 만 입력 시 빈 슬라이스 반환",
			input:    "xflow",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenizeInput(tt.input)
			assert.Equal(t, tt.expected, result,
				"입력 '%s' 의 토큰화 결과가 일치해야 합니다", tt.input)
		})
	}
}

// =============================================================================
// TestHandleSpecialCommand - REPL 특수 명령어 인식 테스트
// REQ-INT-001, REQ-INT-004: 세션 관리 특수 명령어
// =============================================================================

func TestHandleSpecialCommand(t *testing.T) {
	// 테스트용 세션 생성
	session := newTestSession(t)

	tests := []struct {
		name            string
		input           string
		expectHandled   bool
		expectExit      bool
		expectOutputSub string // 출력에 포함될 부분 문자열
	}{
		{
			name:            "help 명령어는 특수 명령어",
			input:           "help",
			expectHandled:   true,
			expectOutputSub: "사용 가능한 명령어",
		},
		{
			name:            "? 명령어는 help 과 동일",
			input:           "?",
			expectHandled:   true,
			expectOutputSub: "사용 가능한 명령어",
		},
		{
			name:          "exit 명령어는 세션 종료",
			input:         "exit",
			expectHandled: true,
			expectExit:    true,
		},
		{
			name:          "quit 명령어는 세션 종료",
			input:         "quit",
			expectHandled: true,
			expectExit:    true,
		},
		{
			name:          "clear 명령어는 특수 명령어",
			input:         "clear",
			expectHandled: true,
		},
		{
			name:            "history 명령어는 특수 명령어",
			input:           "history",
			expectHandled:   true,
			expectOutputSub: "", // 히스토리가 비어있을 수 있음
		},
		{
			name:          "일반 명령어는 특수 명령어가 아님",
			input:         "flow list",
			expectHandled: false,
		},
		{
			name:          "version 은 일반 명령어",
			input:         "version",
			expectHandled: false,
		},
		{
			name:          "빈 입력은 특수 명령어가 아님",
			input:         "",
			expectHandled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session.writer.(*bytes.Buffer).Reset()
			handled, exit := session.handleSpecialCommand(tt.input)
			assert.Equal(t, tt.expectHandled, handled,
				"'%s' 의 handled 결과가 %v 여야 합니다", tt.input, tt.expectHandled)
			assert.Equal(t, tt.expectExit, exit,
				"'%s' 의 exit 결과가 %v 여야 합니다", tt.input, tt.expectExit)

			if tt.expectOutputSub != "" {
				output := session.writer.(*bytes.Buffer).String()
				assert.Contains(t, output, tt.expectOutputSub,
					"출력에 '%s' 가 포함되어야 합니다", tt.expectOutputSub)
			}
		})
	}
}

// =============================================================================
// TestBuildPrompt - 컨텍스트 인식 프롬프트 생성 테스트
// REQ-INT-002: 컨텍스트 인식 프롬프트
// =============================================================================

func TestBuildPrompt(t *testing.T) {
	tests := []struct {
		name     string
		client   *Client
		pingErr  error
		expected string
	}{
		{
			name:     "클라이언트가 nil 이면 기본 프롬프트",
			client:   nil,
			expected: "xflow> ",
		},
		{
			name:     "서버 연결 성공 시 호스트 표시",
			client:   NewClient("http://localhost:8080", "", 0, false),
			pingErr:  nil,
			expected: "xflow [localhost:8080]> ",
		},
		{
			name:     "서버 연결 실패 시 disconnected 표시",
			client:   NewClient("http://myserver:9090", "", 0, false),
			pingErr:  errors.New("connection refused"),
			expected: "xflow [disconnected]> ",
		},
		{
			name:     "포트 없는 호스트",
			client:   NewClient("http://example.com", "", 0, false),
			pingErr:  nil,
			expected: "xflow [example.com]> ",
		},
		{
			name:     "https URL 에서 호스트 추출",
			client:   NewClient("https://secure.example.com:443", "", 0, false),
			pingErr:  nil,
			expected: "xflow [secure.example.com:443]> ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &InteractiveSession{
				writer: &bytes.Buffer{},
			}

			if tt.client != nil {
				session.client = &tt.client
				session.pingFn = func() error { return tt.pingErr }
			}

			prompt := session.buildPrompt()
			assert.Equal(t, tt.expected, prompt,
				"프롬프트가 '%s' 여야 합니다", tt.expected)
		})
	}
}

// =============================================================================
// TestSuggestCommand - 유사 명령어 제안 테스트
// REQ-INT-005: 잘못된 명령어 처리
// =============================================================================

func TestSuggestCommand(t *testing.T) {
	session := newTestSession(t)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "flwo 는 flow 를 제안",
			input:    "flwo",
			expected: "flow",
		},
		{
			name:     "agnet 은 agent 를 제안",
			input:    "agnet",
			expected: "agent",
		},
		{
			name:     "versoin 은 version 을 제안",
			input:    "versoin",
			expected: "version",
		},
		{
			name:     "매칭되지 않는 입력은 빈 문자열 반환",
			input:    "abcxyz",
			expected: "",
		},
		{
			name:     "빈 입력은 빈 문자열 반환",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := session.suggestCommand(tt.input)
			assert.Equal(t, tt.expected, result,
				"'%s' 에 대한 제안이 '%s' 여야 합니다", tt.input, tt.expected)
		})
	}
}

// =============================================================================
// TestLevenshteinDistance - 편집 거리 계산 테스트
// REQ-INT-005: 유사 명령어 매칭 기반 알고리즘
// =============================================================================

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected int
	}{
		{
			name:     "flow 와 flwo 의 거리",
			a:        "flow",
			b:        "flwo",
			expected: 2,
		},
		{
			name:     "빈 문자열과 abc 의 거리",
			a:        "",
			b:        "abc",
			expected: 3,
		},
		{
			name:     "동일한 문자열의 거리는 0",
			a:        "same",
			b:        "same",
			expected: 0,
		},
		{
			name:     "abc 와 빈 문자열의 거리",
			a:        "abc",
			b:        "",
			expected: 3,
		},
		{
			name:     "완전히 다른 문자열",
			a:        "abc",
			b:        "xyz",
			expected: 3,
		},
		{
			name:     "한 글자 삽입",
			a:        "flow",
			b:        "flows",
			expected: 1,
		},
		{
			name:     "한 글자 삭제",
			a:        "flows",
			b:        "flow",
			expected: 1,
		},
		{
			name:     "한 글자 교체",
			a:        "flow",
			b:        "flaw",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := levenshteinDistance(tt.a, tt.b)
			assert.Equal(t, tt.expected, result,
				"'%s' 와 '%s' 의 편집 거리가 %d 여야 합니다", tt.a, tt.b, tt.expected)
		})
	}
}

// =============================================================================
// TestNewInteractiveCmd - interactive Cobra 커맨드 생성 테스트
// REQ-INT-001: REPL 세션 시작
// =============================================================================

func TestNewInteractiveCmd(t *testing.T) {
	rootCmd := NewRootCmd()

	// interactive 서브커맨드가 등록되어 있는지 확인
	var interactiveCmd *cobra.Command
	for _, sub := range rootCmd.Commands() {
		if sub.Use == "interactive" {
			interactiveCmd = sub
			break
		}
	}

	require.NotNil(t, interactiveCmd,
		"interactive 서브커맨드가 루트 커맨드에 등록되어 있어야 합니다")
	assert.Equal(t, "interactive", interactiveCmd.Use,
		"Use 필드가 'interactive' 여야 합니다")
	assert.NotEmpty(t, interactiveCmd.Short,
		"Short 설명이 비어있으면 안됩니다")
}

// =============================================================================
// TestPreventNestedInteractive - 중첩 interactive 방지 테스트
// =============================================================================

func TestPreventNestedInteractive(t *testing.T) {
	session := newTestSession(t)

	err := session.executeCommand("interactive")

	require.NoError(t, err, "중첩 interactive 호출은 에러가 아닌 안내 메시지를 보여야 합니다")

	output := session.writer.(*bytes.Buffer).String()
	assert.Contains(t, output, "이미 대화형 모드에 있습니다",
		"중첩 interactive 시 안내 메시지가 출력되어야 합니다")
}

// =============================================================================
// TestExecuteCommand - REPL 내 명령어 실행 테스트
// REQ-INT-003: 명령어 파싱 및 실행
// =============================================================================

func TestExecuteCommand(t *testing.T) {
	t.Run("빈 입력은 아무 동작 없음", func(t *testing.T) {
		session := newTestSession(t)
		err := session.executeCommand("")
		assert.NoError(t, err,
			"빈 입력은 에러를 반환하지 않아야 합니다")
	})

	t.Run("공백만 있는 입력은 아무 동작 없음", func(t *testing.T) {
		session := newTestSession(t)
		err := session.executeCommand("   ")
		assert.NoError(t, err,
			"공백 입력은 에러를 반환하지 않아야 합니다")
	})

	t.Run("version 명령어 실행", func(t *testing.T) {
		session := newTestSession(t)
		err := session.executeCommand("version")
		assert.NoError(t, err,
			"version 명령어 실행에 에러가 없어야 합니다")

		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "xflow version",
			"version 명령어 출력에 버전 정보가 포함되어야 합니다")
	})

	t.Run("xflow 접두사 포함 명령어 실행", func(t *testing.T) {
		session := newTestSession(t)
		err := session.executeCommand("xflow version")
		assert.NoError(t, err,
			"xflow 접두사 포함 명령어도 실행 가능해야 합니다")

		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "xflow version",
			"xflow 접두사 포함 명령어도 동일한 결과여야 합니다")
	})

	t.Run("존재하지 않는 명령어는 에러 메시지 출력", func(t *testing.T) {
		session := newTestSession(t)
		err := session.executeCommand("nonexistent")
		// REPL 에서는 에러를 반환하지 않고 사용자에게 메시지를 표시
		assert.NoError(t, err,
			"REPL 에서 잘못된 명령어는 에러 반환이 아닌 메시지 표시")

		output := session.writer.(*bytes.Buffer).String()
		// 에러 메시지가 출력되어야 함
		assert.NotEmpty(t, output,
			"잘못된 명령어에 대한 메시지가 출력되어야 합니다")
	})
}

// =============================================================================
// TestWelcomeMessage - 환영 메시지 테스트
// REQ-INT-001: REPL 세션 시작
// =============================================================================

func TestWelcomeMessage(t *testing.T) {
	var buf bytes.Buffer
	printWelcomeMessage(&buf)

	output := buf.String()
	assert.Contains(t, output, "xflow",
		"환영 메시지에 'xflow' 가 포함되어야 합니다")
	assert.Contains(t, output, "help",
		"환영 메시지에 'help' 안내가 포함되어야 합니다")
	assert.Contains(t, output, "?",
		"환영 메시지에 '?' 안내가 포함되어야 합니다")
}

// =============================================================================
// TestExitMessage - 종료 메시지 테스트
// REQ-INT-004: 세션 종료
// =============================================================================

func TestExitMessage(t *testing.T) {
	var buf bytes.Buffer
	printExitMessage(&buf)

	output := buf.String()
	assert.Contains(t, output, "세션을 종료합니다",
		"종료 메시지에 '세션을 종료합니다' 가 포함되어야 합니다")
}

// =============================================================================
// TestExtractHost - URL 에서 호스트:포트 추출 테스트
// REQ-INT-002: 컨텍스트 인식 프롬프트 보조 함수
// =============================================================================

func TestExtractHost(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "http URL 에서 호스트:포트 추출",
			url:      "http://localhost:8080",
			expected: "localhost:8080",
		},
		{
			name:     "https URL 에서 호스트:포트 추출",
			url:      "https://secure.example.com:443",
			expected: "secure.example.com:443",
		},
		{
			name:     "포트 없는 URL",
			url:      "http://example.com",
			expected: "example.com",
		},
		{
			name:     "빈 URL",
			url:      "",
			expected: "",
		},
		{
			name:     "잘못된 URL 은 원본 반환",
			url:      "not-a-url",
			expected: "not-a-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractHost(tt.url)
			assert.Equal(t, tt.expected, result,
				"URL '%s' 에서 추출한 호스트가 '%s' 여야 합니다", tt.url, tt.expected)
		})
	}
}

// =============================================================================
// TestHelpOutput - help 명령어 출력 내용 테스트
// =============================================================================

func TestHelpOutput(t *testing.T) {
	session := newTestSession(t)
	buf := session.writer.(*bytes.Buffer)

	session.handleSpecialCommand("help")

	output := buf.String()

	// 특수 명령어가 도움말에 포함되어야 함
	assert.Contains(t, output, "help",
		"도움말에 help 명령어가 포함되어야 합니다")
	assert.Contains(t, output, "exit",
		"도움말에 exit 명령어가 포함되어야 합니다")
	assert.Contains(t, output, "quit",
		"도움말에 quit 명령어가 포함되어야 합니다")
	assert.Contains(t, output, "clear",
		"도움말에 clear 명령어가 포함되어야 합니다")
	assert.Contains(t, output, "history",
		"도움말에 history 명령어가 포함되어야 합니다")
}

// =============================================================================
// TestNewInteractiveSession - 세션 생성 테스트
// =============================================================================

func TestNewInteractiveSession(t *testing.T) {
	rootCmd := newTestRootCmd()
	var client *Client
	var buf bytes.Buffer

	session := NewInteractiveSession(rootCmd, &client, &buf)

	require.NotNil(t, session, "세션이 nil 이면 안됩니다")
	assert.Equal(t, rootCmd, session.rootCmd, "rootCmd 가 동일해야 합니다")
	assert.NotNil(t, session.writer, "writer 가 nil 이면 안됩니다")
}

// =============================================================================
// TestSuggestCommandWithErrorMessage - 명령어 제안과 에러 메시지 결합 테스트
// REQ-INT-005: 잘못된 명령어 처리 시 유사 명령어 제안
// =============================================================================

func TestSuggestCommandWithErrorMessage(t *testing.T) {
	t.Run("유사 명령어가 있으면 제안 메시지 포함", func(t *testing.T) {
		session := newTestSession(t)
		_ = session.executeCommand("flwo")

		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "flow",
			"유사 명령어 'flow' 가 제안에 포함되어야 합니다")
	})

	t.Run("유사 명령어가 없으면 help 안내 메시지", func(t *testing.T) {
		session := newTestSession(t)
		_ = session.executeCommand("zzzzzzz")

		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "help",
			"유사 명령어가 없으면 help 안내가 표시되어야 합니다")
	})
}

// =============================================================================
// TestWizardCommandStub - wizard 명령어 스텁 등록 테스트 (P2 준비)
// =============================================================================

func TestWizardCommandStub(t *testing.T) {
	session := newTestSession(t)

	handled, _ := session.handleSpecialCommand("wizard flow deploy")
	assert.True(t, handled,
		"wizard 명령어는 특수 명령어로 처리되어야 합니다")

	output := session.writer.(*bytes.Buffer).String()
	// P2 에서 구현 예정이므로 안내 메시지만 표시
	assert.NotEmpty(t, output,
		"wizard 명령어에 대한 안내 메시지가 출력되어야 합니다")
}

// =============================================================================
// 테스트 헬퍼 함수
// =============================================================================

// newTestRootCmd 는 테스트용 루트 커맨드를 생성한다.
// PersistentPreRunE 를 제거하여 실제 서버 연결 없이 테스트할 수 있도록 한다.
func newTestRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "xflow",
		Short: "xflow - 테스트용 CLI",
	}

	// 글로벌 플래그 등록
	pflags := rootCmd.PersistentFlags()
	pflags.String("format", "table", "출력 형식")
	pflags.Bool("verbose", false, "상세 출력")
	pflags.Bool("quiet", false, "조용한 출력")
	pflags.Bool("no-color", false, "색상 비활성화")
	pflags.String("config", "", "설정 파일")
	pflags.String("server", "", "서버 URL")
	pflags.String("token", "", "인증 토큰")

	// 테스트용 서브커맨드 등록
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "버전 정보",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "xflow version dev\n")
			return nil
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "flow",
		Short: "플로우 관리",
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "agent",
		Short: "에이전트 관리",
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "config",
		Short: "설정 관리",
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "node",
		Short: "노드 관리",
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "plugin",
		Short: "플러그인 관리",
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "상태 조회",
	})

	return rootCmd
}

// newTestSession 은 테스트용 InteractiveSession 을 생성한다.
func newTestSession(t *testing.T) *InteractiveSession {
	t.Helper()
	rootCmd := newTestRootCmd()
	var client *Client
	var buf bytes.Buffer
	session := NewInteractiveSession(rootCmd, &client, &buf)
	return session
}

// mockReadCloser 는 테스트용 io.ReadCloser 구현이다.
type mockReadCloser struct {
	io.Reader
}

func (m *mockReadCloser) Close() error { return nil }

// mockReadline 은 테스트용 readline 대체 인터페이스이다.
type mockReadline struct {
	lines []string
	index int
}

func (m *mockReadline) Readline() (string, error) {
	if m.index >= len(m.lines) {
		return "", io.EOF
	}
	line := m.lines[m.index]
	m.index++
	return line, nil
}

func (m *mockReadline) SetPrompt(prompt string) {}
func (m *mockReadline) Close() error            { return nil }
func (m *mockReadline) SaveHistory(cmd string) error {
	return nil
}

// =============================================================================
// TestRunREPLLoop - REPL 루프 통합 테스트
// REQ-INT-001, REQ-INT-003, REQ-INT-004: 세션 시작/실행/종료 통합
// =============================================================================

func TestRunREPLLoop(t *testing.T) {
	t.Run("exit 으로 세션 종료", func(t *testing.T) {
		session := newTestSession(t)
		reader := &mockReadline{lines: []string{"exit"}}
		session.readlineFn = reader

		err := session.runLoop()
		assert.NoError(t, err, "exit 으로 정상 종료해야 합니다")

		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "세션을 종료합니다",
			"exit 시 종료 메시지가 출력되어야 합니다")
	})

	t.Run("quit 으로 세션 종료", func(t *testing.T) {
		session := newTestSession(t)
		reader := &mockReadline{lines: []string{"quit"}}
		session.readlineFn = reader

		err := session.runLoop()
		assert.NoError(t, err, "quit 으로 정상 종료해야 합니다")
	})

	t.Run("EOF(Ctrl+D) 로 세션 종료", func(t *testing.T) {
		session := newTestSession(t)
		reader := &mockReadline{lines: []string{}} // 빈 라인은 EOF 발생
		session.readlineFn = reader

		err := session.runLoop()
		assert.NoError(t, err, "Ctrl+D(EOF) 으로 정상 종료해야 합니다")

		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "세션을 종료합니다",
			"Ctrl+D 시 종료 메시지가 출력되어야 합니다")
	})

	t.Run("명령어 실행 후 프롬프트 반복", func(t *testing.T) {
		session := newTestSession(t)
		reader := &mockReadline{lines: []string{"version", "exit"}}
		session.readlineFn = reader

		err := session.runLoop()
		assert.NoError(t, err, "명령어 실행 후 정상 종료해야 합니다")

		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "xflow version",
			"version 명령어 출력이 포함되어야 합니다")
	})

	t.Run("빈 입력은 무시하고 다음 프롬프트", func(t *testing.T) {
		session := newTestSession(t)
		reader := &mockReadline{lines: []string{"", "   ", "exit"}}
		session.readlineFn = reader

		err := session.runLoop()
		assert.NoError(t, err, "빈 입력 후 정상 종료해야 합니다")
	})

	t.Run("유사 명령어 제안 후 프롬프트 반복", func(t *testing.T) {
		session := newTestSession(t)
		reader := &mockReadline{lines: []string{"flwo", "exit"}}
		session.readlineFn = reader

		err := session.runLoop()
		assert.NoError(t, err)

		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "flow",
			"유사 명령어 'flow' 제안이 포함되어야 합니다")
	})
}

// =============================================================================
// TestCollectAvailableCommands - 사용 가능한 명령어 수집 테스트
// =============================================================================

func TestCollectAvailableCommands(t *testing.T) {
	session := newTestSession(t)
	commands := session.collectAvailableCommands()

	assert.Contains(t, commands, "version",
		"사용 가능한 명령어에 version 이 포함되어야 합니다")
	assert.Contains(t, commands, "flow",
		"사용 가능한 명령어에 flow 가 포함되어야 합니다")
	assert.Contains(t, commands, "agent",
		"사용 가능한 명령어에 agent 가 포함되어야 합니다")
}

// =============================================================================
// TestExecuteCommandFlagReset - 명령어 실행 후 플래그 상태 초기화 테스트
// REQ-INT-006: 각 명령어별 --format 플래그 독립 적용
// =============================================================================

func TestExecuteCommandFlagReset(t *testing.T) {
	session := newTestSession(t)

	// 첫 번째 실행: --format json
	_ = session.executeCommand("version --format json")

	// 두 번째 실행: 기본 format (table)
	session.writer.(*bytes.Buffer).Reset()
	_ = session.executeCommand("version")

	// 두 번째 실행에서 format 이 기본값으로 리셋되었는지 확인
	format, err := session.rootCmd.PersistentFlags().GetString("format")
	assert.NoError(t, err)
	assert.Equal(t, "table", format,
		"명령어 실행 후 format 플래그가 기본값으로 리셋되어야 합니다")
}

// =============================================================================
// TestInteractiveAliases - interactive 명령어 별칭 테스트
// =============================================================================

func TestInteractiveAliases(t *testing.T) {
	rootCmd := NewRootCmd()

	var interactiveCmd *cobra.Command
	for _, sub := range rootCmd.Commands() {
		if sub.Use == "interactive" {
			interactiveCmd = sub
			break
		}
	}

	if interactiveCmd == nil {
		t.Skip("interactive 커맨드 미등록 (구현 전)")
	}

	// Aliases 확인 (i, repl)
	assert.Contains(t, interactiveCmd.Aliases, "i",
		"'i' 별칭이 등록되어 있어야 합니다")
	assert.Contains(t, interactiveCmd.Aliases, "repl",
		"'repl' 별칭이 등록되어 있어야 합니다")
}

// =============================================================================
// TestInputWithExtraSpaces - 다양한 공백 패턴 입력 처리 테스트
// =============================================================================

func TestInputWithExtraSpaces(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "탭 문자 포함",
			input: "flow\tlist",
			want:  []string{"flow", "list"},
		},
		{
			name:  "선행 공백",
			input: "  flow list",
			want:  []string{"flow", "list"},
		},
		{
			name:  "후행 공백",
			input: "flow list  ",
			want:  []string{"flow", "list"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenizeInput(tt.input)
			assert.Equal(t, tt.want, result)
		})
	}
}

// =============================================================================
// TestHandleSpecialCommandCaseInsensitive - 특수 명령어 대소문자 처리
// =============================================================================

func TestHandleSpecialCommandCaseInsensitive(t *testing.T) {
	session := newTestSession(t)

	// 특수 명령어는 소문자로만 인식 (REPL 관례)
	handled, _ := session.handleSpecialCommand("HELP")
	assert.False(t, handled,
		"대문자 HELP 는 특수 명령어로 인식하지 않아야 합니다")

	handled, _ = session.handleSpecialCommand("EXIT")
	assert.False(t, handled,
		"대문자 EXIT 는 특수 명령어로 인식하지 않아야 합니다")
}

// =============================================================================
// TestGetAvailableSubcommands - 서브커맨드 포함 명령어 목록 테스트
// =============================================================================

func TestGetAvailableSubcommands(t *testing.T) {
	rootCmd := newTestRootCmd()
	// flow 서브커맨드에 list 추가
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "flow" {
			cmd.AddCommand(&cobra.Command{
				Use:   "list",
				Short: "플로우 목록",
			})
			break
		}
	}

	var client *Client
	session := NewInteractiveSession(rootCmd, &client, &bytes.Buffer{})

	commands := session.collectAvailableCommands()

	assert.Contains(t, commands, "flow",
		"최상위 명령어 flow 가 포함되어야 합니다")
	// 최상위 명령어만 수집
	assert.Contains(t, commands, "version",
		"최상위 명령어 version 이 포함되어야 합니다")
}

// =============================================================================
// TestMultipleCommandExecution - 여러 명령어 연속 실행 테스트
// =============================================================================

func TestMultipleCommandExecution(t *testing.T) {
	session := newTestSession(t)
	reader := &mockReadline{lines: []string{
		"version",
		"version",
		"help",
		"exit",
	}}
	session.readlineFn = reader

	err := session.runLoop()
	assert.NoError(t, err)

	output := session.writer.(*bytes.Buffer).String()
	count := strings.Count(output, "xflow version")
	assert.GreaterOrEqual(t, count, 2,
		"version 명령어가 두 번 실행되어야 합니다")
}

// =============================================================================
// TestPrintHistoryWithEntries - 히스토리에 항목이 있을 때 출력 테스트
// =============================================================================

func TestPrintHistoryWithEntries(t *testing.T) {
	session := newTestSession(t)
	session.history = []string{"version", "flow list", "help"}

	buf := session.writer.(*bytes.Buffer)
	session.printHistory()

	output := buf.String()
	assert.Contains(t, output, "1  version",
		"히스토리에 번호와 함께 version 이 포함되어야 합니다")
	assert.Contains(t, output, "2  flow list",
		"히스토리에 번호와 함께 flow list 가 포함되어야 합니다")
	assert.Contains(t, output, "3  help",
		"히스토리에 번호와 함께 help 가 포함되어야 합니다")
}

// =============================================================================
// TestHistoryViaREPLLoop - REPL 루프에서 history 명령어 테스트
// =============================================================================

func TestHistoryViaREPLLoop(t *testing.T) {
	session := newTestSession(t)
	reader := &mockReadline{lines: []string{
		"version",
		"history",
		"exit",
	}}
	session.readlineFn = reader

	err := session.runLoop()
	assert.NoError(t, err)

	output := session.writer.(*bytes.Buffer).String()
	// history 명령어 실행 시점에서 version 이 히스토리에 있어야 함
	assert.Contains(t, output, "1  version",
		"history 출력에 이전에 실행한 version 명령어가 포함되어야 합니다")
}

// =============================================================================
// TestStopMethod - Stop 메서드 테스트
// =============================================================================

func TestStopMethod(t *testing.T) {
	session := newTestSession(t)
	mock := &mockReadline{lines: []string{}}
	session.readlineFn = mock

	// Stop 호출 시 패닉 없이 정상 동작
	session.Stop()

	// readlineFn 이 nil 인 경우도 안전해야 함
	session2 := newTestSession(t)
	session2.readlineFn = nil
	session2.Stop() // nil readlineFn 에서도 패닉 없어야 함
}

// =============================================================================
// TestHistoryClear - 히스토리 삭제 테스트
// =============================================================================

func TestHistoryClear(t *testing.T) {
	session := newTestSession(t)
	session.history = []string{"version", "flow list", "help"}

	buf := session.writer.(*bytes.Buffer)
	session.clearHistory()

	assert.Empty(t, session.history, "clearHistory 후 인메모리 히스토리가 비어있어야 합니다")
	output := buf.String()
	assert.Contains(t, output, "히스토리가 삭제되었습니다", "삭제 확인 메시지가 출력되어야 합니다")
}

// =============================================================================
// TestHistorySearch - 히스토리 검색 테스트
// =============================================================================

func TestHistorySearch(t *testing.T) {
	session := newTestSession(t)
	session.history = []string{"version", "flow list", "flow deploy my-flow", "agent list", "FLOW STATUS"}

	t.Run("패턴 일치 결과 출력", func(t *testing.T) {
		buf := session.writer.(*bytes.Buffer)
		buf.Reset()
		session.searchHistory("flow")

		output := buf.String()
		assert.Contains(t, output, "flow list", "flow list 가 검색 결과에 포함되어야 합니다")
		assert.Contains(t, output, "flow deploy", "flow deploy 가 검색 결과에 포함되어야 합니다")
		assert.Contains(t, output, "FLOW STATUS", "대소문자 무시 검색이므로 FLOW STATUS 가 포함되어야 합니다")
		assert.NotContains(t, output, "version", "version 은 검색 결과에 포함되면 안됩니다")
	})

	t.Run("일치 없음", func(t *testing.T) {
		buf := session.writer.(*bytes.Buffer)
		buf.Reset()
		session.searchHistory("nonexistent")

		output := buf.String()
		assert.Contains(t, output, "일치하는 히스토리가 없습니다", "일치 없음 메시지가 출력되어야 합니다")
	})

	t.Run("빈 패턴", func(t *testing.T) {
		buf := session.writer.(*bytes.Buffer)
		buf.Reset()
		session.searchHistory("")

		output := buf.String()
		assert.Contains(t, output, "사용법", "빈 패턴 시 사용법 안내가 출력되어야 합니다")
	})
}

// =============================================================================
// TestHistorySubcommands - history 서브명령어 테스트
// =============================================================================

func TestHistorySubcommands(t *testing.T) {
	t.Run("history clear 특수 명령어", func(t *testing.T) {
		session := newTestSession(t)
		session.history = []string{"version", "flow list"}
		handled, exit := session.handleSpecialCommand("history clear")
		assert.True(t, handled)
		assert.False(t, exit)
		assert.Empty(t, session.history)
	})

	t.Run("history search 특수 명령어", func(t *testing.T) {
		session := newTestSession(t)
		session.history = []string{"version", "flow list"}
		handled, exit := session.handleSpecialCommand("history search flow")
		assert.True(t, handled)
		assert.False(t, exit)
		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "flow list")
	})

	t.Run("history search 패턴 누락", func(t *testing.T) {
		session := newTestSession(t)
		handled, exit := session.handleSpecialCommand("history search")
		assert.True(t, handled)
		assert.False(t, exit)
		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "사용법")
	})

	t.Run("history 알 수 없는 서브명령어는 기본 히스토리 출력", func(t *testing.T) {
		session := newTestSession(t)
		session.history = []string{"version"}
		handled, exit := session.handleSpecialCommand("history unknown")
		assert.True(t, handled)
		assert.False(t, exit)
		output := session.writer.(*bytes.Buffer).String()
		assert.Contains(t, output, "version")
	})
}

// =============================================================================
// TestSubcommandFlagInREPL - REPL 에서 서브커맨드 플래그 파싱 재현 테스트
// =============================================================================

func TestSubcommandFlagInREPL(t *testing.T) {
	// 서브커맨드에 required 플래그가 있는 명령어 트리 구성
	rootCmd := &cobra.Command{
		Use: "xflow",
	}

	var filePath string
	importCmd := &cobra.Command{
		Use: "import",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "imported: %s\n", filePath)
			return nil
		},
	}
	importCmd.Flags().StringVarP(&filePath, "file", "f", "", "파일 경로")
	_ = importCmd.MarkFlagRequired("file")

	flowCmd := &cobra.Command{
		Use: "flow",
	}
	flowCmd.AddCommand(importCmd)
	rootCmd.AddCommand(flowCmd)

	var client *Client
	var buf bytes.Buffer
	session := NewInteractiveSession(rootCmd, &client, &buf)

	// 1) 플래그 없이 실행 (실패 예상)
	_ = session.executeCommand("flow import")
	firstOutput := buf.String()
	t.Logf("첫 번째 실행 출력:\n%s", firstOutput)
	buf.Reset()

	// 2) 플래그와 함께 재실행 - 이전 실행 상태가 영향을 주는지 확인
	_ = session.executeCommand("flow import -f test.yaml")
	secondOutput := buf.String()
	t.Logf("두 번째 실행 출력:\n%s", secondOutput)

	assert.Contains(t, secondOutput, "imported: test.yaml",
		"두 번째 실행에서 -f 플래그가 파싱되어야 합니다. 실제 출력: %s", secondOutput)
}

// TestSubcommandFlagWithRealRootCmd 는 실제 NewRootCmd 로 REPL 플래그 파싱을 테스트한다.
func TestSubcommandFlagWithRealRootCmd(t *testing.T) {
	rootCmd := NewRootCmd()

	var client *Client
	var buf bytes.Buffer
	session := NewInteractiveSession(rootCmd, &client, &buf)

	// flow import -f 로 실행 - 에러 메시지를 확인
	_ = session.executeCommand("flow import -f ./testdata_nonexistent.yaml")
	output := buf.String()
	t.Logf("실제 RootCmd 출력:\n%s", output)

	// Usage 가 출력되면 안 됨 (REPL 에서는 SilenceUsage)
	assert.NotContains(t, output, "Usage:",
		"REPL 에서는 런타임 에러 시 Usage 가 출력되면 안됩니다")

	// 실제 파일 에러 메시지가 표시되어야 함
	assert.Contains(t, output, "오류:",
		"REPL 에서 런타임 에러는 '오류:' 접두사로 표시되어야 합니다")
}

// TestREPL_ModbusSubcommandFlagReset 은 modbus 서브커맨드의 플래그가
// 실행 사이에 올바르게 리셋되는지 검증한다.
func TestREPL_ModbusSubcommandFlagReset(t *testing.T) {
	// mock 서버: modbus-server 타입 에이전트 반환
	handler := modbusAgentHandler("modbus-server", map[string]any{
		"values": []any{1, 2, 3},
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "", 5*time.Second, false)

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.PersistentFlags().Bool("verbose", false, "상세 출력")
	rootCmd.PersistentFlags().String("config", "", "설정 파일")
	rootCmd.PersistentFlags().String("server", "", "서버 URL")
	rootCmd.PersistentFlags().String("token", "", "인증 토큰")
	rootCmd.PersistentFlags().Bool("quiet", false, "조용한 출력")
	rootCmd.PersistentFlags().Bool("no-color", false, "색상 비활성화")
	rootCmd.AddCommand(newModbusCmd(&client))

	var buf bytes.Buffer
	session := NewInteractiveSession(rootCmd, &client, &buf)

	// 첫 번째 실행: address=100, quantity=5
	err := session.executeCommand("modbus read test-agent -r hr -a 100 -q 5")
	assert.NoError(t, err, "첫 번째 modbus read 실행이 성공해야 합니다")

	// 첫 번째 실행 출력 확인
	firstOutput := buf.String()
	assert.NotEmpty(t, firstOutput, "첫 번째 실행의 출력이 있어야 합니다")

	// 리셋 후 modbus read 서브커맨드의 플래그 값 확인
	modbusCmd, _, _ := rootCmd.Find([]string{"modbus", "read"})
	if modbusCmd != nil {
		addressFlag := modbusCmd.Flags().Lookup("address")
		if addressFlag != nil {
			assert.Equal(t, "0", addressFlag.DefValue, "address 기본값이 0이어야 합니다")
			assert.Equal(t, "0", addressFlag.Value.String(),
				"리셋 후 address 플래그가 기본값(0)으로 돌아가야 합니다")
			assert.False(t, addressFlag.Changed,
				"리셋 후 Changed 플래그가 false여야 합니다")
		}
		quantityFlag := modbusCmd.Flags().Lookup("quantity")
		if quantityFlag != nil {
			assert.Equal(t, "1", quantityFlag.Value.String(),
				"리셋 후 quantity 플래그가 기본값(1)으로 돌아가야 합니다")
			assert.False(t, quantityFlag.Changed,
				"리셋 후 Changed 플래그가 false여야 합니다")
		}
	}

	// 두 번째 실행: 다른 파라미터
	buf.Reset()
	err = session.executeCommand("modbus read test-agent -r input -a 200 -q 10")
	assert.NoError(t, err, "두 번째 modbus read 실행이 성공해야 합니다")

	secondOutput := buf.String()
	assert.NotEmpty(t, secondOutput, "두 번째 실행의 출력이 있어야 합니다")
}

// =============================================================================
// TestREPL_GlobalFlagPersistence - REPL 세션 전역 플래그 보존 재현 테스트
// 회귀: resetFlags 가 루트 퍼시스턴트 플래그(--config 등)까지 기본값으로 되돌려
// 두 번째 명령부터 런치 시점의 연결 컨텍스트(서버/설정/토큰)가 사라지는 버그.
// =============================================================================

// authFlowTestServer 는 auth login 및 flow list 핸들러를 갖춘 테스트 서버를 만든다.
// /flows 는 Bearer TOK123 헤더가 있을 때만 200 을 반환한다.
// tokenSeen 은 올바른 토큰으로 /flows 가 호출되었는지를 기록한다.
func authFlowTestServer(t *testing.T) (*httptest.Server, *bool) {
	t.Helper()
	var mu sync.Mutex
	tokenSeen := false

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"success":true,"data":{"user":{"username":"x","role":"admin"},"tokens":{"access_token":"TOK123","token_type":"Bearer","expires_in":3600}}}`)
	})
	mux.HandleFunc("/api/v1/flows", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") == "Bearer TOK123" {
			mu.Lock()
			tokenSeen = true
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"success":true,"data":[]}`)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"success":false,"error":{"code":"unauthorized","message":"no token"}}`)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &tokenSeen
}

// writeTempConfig 는 server.url 과 빈 auth.token 을 가진 임시 config 파일을 생성하고
// 그 경로를 반환한다.
func writeTempConfig(t *testing.T, serverURL string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "server:\n  url: " + serverURL + "\nauth:\n  token: \"\"\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// TestREPL_AuthLoginThenFlowListPersistsContext 는 핵심 회귀 시나리오를 재현한다.
// 런치 시 --config 로 지정한 서버/설정이 auth login 이후의 flow list 에서도
// 그대로 사용되어 인증된 요청이 되는지 검증한다.
//
// 수정 전: resetFlags 가 --config 를 기본값으로 되돌려 두 번째 명령(flow list)이
//
//	기본 config(~/.xflow)를 읽고 토큰 없이 요청 → 401/오류.
//
// 수정 후: --config 가 세션 내내 보존되어 login 이 저장한 토큰을 flow list 가 사용 → 200.
func TestREPL_AuthLoginThenFlowListPersistsContext(t *testing.T) {
	sessionToken = ""
	defer func() { sessionToken = "" }()

	srv, tokenSeen := authFlowTestServer(t)
	configPath := writeTempConfig(t, srv.URL)

	rootCmd := NewRootCmd()

	// 런치 시뮬레이션: xflow --config <path> interactive 와 동일하게
	// 루트 퍼시스턴트 --config 플래그를 설정한다.
	require.NoError(t, rootCmd.PersistentFlags().Set("config", configPath))

	var client *Client
	var buf bytes.Buffer
	session := NewInteractiveSession(rootCmd, &client, &buf)
	// 프롬프트 빌드 시 네트워크 핑을 막는다.
	session.pingFn = func() error { return nil }

	// 1) auth login: 토큰을 임시 config 에 저장하고 세션 클라이언트에 반영한다.
	err := session.executeCommand("auth login -u x -p y")
	require.NoError(t, err)
	loginOut := buf.String()
	require.Contains(t, loginOut, "로그인 성공", "auth login 이 성공해야 합니다. 출력: %s", loginOut)

	// 2) flow list: 같은 세션 컨텍스트(임시 config + 저장된 토큰)로 인증되어야 한다.
	buf.Reset()
	err = session.executeCommand("flow list")
	require.NoError(t, err)
	listOut := buf.String()

	assert.NotContains(t, listOut, "오류:",
		"flow list 가 인증 컨텍스트를 유지하여 오류 없이 실행되어야 합니다. 출력: %s", listOut)
	assert.True(t, *tokenSeen,
		"flow list 요청이 런치 config 에 저장된 Bearer TOK123 토큰을 전송해야 합니다")

	// 3) 런치 시점 --config 값이 명령 실행 후에도 보존되어야 한다.
	gotConfig, gerr := rootCmd.PersistentFlags().GetString("config")
	require.NoError(t, gerr)
	assert.Equal(t, configPath, gotConfig,
		"명령 실행 후에도 루트 --config 퍼시스턴트 플래그가 런치 값으로 유지되어야 합니다")
}

// TestREPL_AuthLoginMemoryOnlyAuthenticatesAndDoesNotPersist 는 신규 정책을 재현한다.
// 동일 프로세스/세션에서 --save 없는 auth login 후 flow list 가
// 세션 메모리 토큰(sessionToken)으로 인증되며, config 파일은 수정되지 않아야 한다.
func TestREPL_AuthLoginMemoryOnlyAuthenticatesAndDoesNotPersist(t *testing.T) {
	sessionToken = ""
	defer func() { sessionToken = "" }()

	srv, tokenSeen := authFlowTestServer(t)
	configPath := writeTempConfig(t, srv.URL)

	rootCmd := NewRootCmd()
	require.NoError(t, rootCmd.PersistentFlags().Set("config", configPath))

	var client *Client
	var buf bytes.Buffer
	session := NewInteractiveSession(rootCmd, &client, &buf)
	session.pingFn = func() error { return nil }

	// 1) auth login (--save 없음): 토큰은 세션 메모리에만 보관된다.
	require.NoError(t, session.executeCommand("auth login -u x -p y"))
	require.Contains(t, buf.String(), "로그인 성공")
	assert.Equal(t, "TOK123", sessionToken, "로그인 후 sessionToken 에 토큰이 보관되어야 합니다")

	// 2) flow list: 같은 세션 메모리 토큰으로 인증되어야 한다.
	buf.Reset()
	require.NoError(t, session.executeCommand("flow list"))
	listOut := buf.String()
	assert.NotContains(t, listOut, "오류:",
		"flow list 가 세션 메모리 토큰으로 인증되어야 합니다. 출력: %s", listOut)
	assert.True(t, *tokenSeen,
		"flow list 요청이 세션 메모리 토큰(Bearer TOK123)을 전송해야 합니다")

	// 3) config 파일은 수정되지 않아야 한다 (디스크 미저장 정책).
	assert.Equal(t, "", readConfigToken(t, configPath),
		"--save 없는 로그인은 config 파일을 수정하면 안 됩니다")
}

// TestREPL_GlobalInsecureFlagPersists 는 -k(--insecure) 가 세션 전체에 보존되는지 검증한다.
// 수정 전: resetFlags 가 insecure 를 false 로 되돌려 다음 명령부터 TLS 검증이 다시 켜진다.
func TestREPL_GlobalInsecureFlagPersists(t *testing.T) {
	rootCmd := NewRootCmd()
	require.NoError(t, rootCmd.PersistentFlags().Set("insecure", "true"))

	var client *Client
	var buf bytes.Buffer
	session := NewInteractiveSession(rootCmd, &client, &buf)
	session.pingFn = func() error { return nil }

	// 임의의 명령 실행 후 insecure 플래그가 유지되는지 확인한다.
	_ = session.executeCommand("version")

	insecure, err := rootCmd.PersistentFlags().GetBool("insecure")
	require.NoError(t, err)
	assert.True(t, insecure,
		"명령 실행 후에도 --insecure 퍼시스턴트 플래그가 보존되어야 합니다")

	changed := rootCmd.PersistentFlags().Lookup("insecure").Changed
	assert.True(t, changed,
		"--insecure 의 Changed 상태도 보존되어 PersistentPreRunE 가 런치 값을 인식해야 합니다")
}

// TestREPL_InsecureHonoredAcrossCommandsTLS 는 자체 서명 TLS 서버에 대해
// 런치 -k 가 첫 명령과 후속 명령 모두에서 PersistentPreRunE 를 통해 클라이언트에
// 반영되는지(= 재생성된 클라이언트가 TLS 검증을 건너뛰는지)를 종단 간 검증한다.
// 이는 단순 플래그 상태 보존을 넘어 플래그 상속 경로까지 확인한다.
func TestREPL_InsecureHonoredAcrossCommandsTLS(t *testing.T) {
	var mu sync.Mutex
	flowsHits := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/flows", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		flowsHits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"success":true,"data":[]}`)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	// server.url 만 가진 임시 config (HTTPS 자체 서명 서버)
	configPath := writeTempConfig(t, srv.URL)

	rootCmd := NewRootCmd()
	// 런치 시뮬레이션: xflow --config <path> -k interactive
	require.NoError(t, rootCmd.PersistentFlags().Set("config", configPath))
	require.NoError(t, rootCmd.PersistentFlags().Set("insecure", "true"))

	var client *Client
	var buf bytes.Buffer
	session := NewInteractiveSession(rootCmd, &client, &buf)
	session.pingFn = func() error { return nil }

	// 첫 번째 명령: -k 가 적용되어 자체 서명 인증서를 수용해야 한다.
	_ = session.executeCommand("flow list")
	first := buf.String()
	assert.NotContains(t, first, "오류:",
		"첫 REPL 명령에서 -k 가 적용되어 TLS 오류 없이 실행되어야 합니다. 출력: %s", first)

	// 두 번째 명령: 리셋 이후에도 -k 가 보존되어 동일하게 동작해야 한다.
	buf.Reset()
	_ = session.executeCommand("flow list")
	second := buf.String()
	assert.NotContains(t, second, "오류:",
		"두 번째 REPL 명령에서도 -k 가 보존되어 TLS 오류가 없어야 합니다. 출력: %s", second)

	mu.Lock()
	hits := flowsHits
	mu.Unlock()
	assert.Equal(t, 2, hits,
		"두 번의 flow list 가 모두 자체 서명 TLS 서버에 도달해야 합니다")
}

// TestREPL_SubcommandLocalFlagDoesNotLeak 는 서브커맨드 로컬 플래그가
// 다음 명령으로 누수되지 않는(= 기존 리셋 동작이 보존되는) 것을 검증한다.
func TestREPL_SubcommandLocalFlagDoesNotLeak(t *testing.T) {
	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("config", "", "설정 파일")
	rootCmd.PersistentFlags().String("server", "", "서버 URL")
	rootCmd.PersistentFlags().String("token", "", "인증 토큰")
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.PersistentFlags().BoolP("insecure", "k", false, "TLS 검증 건너뛰기")

	var filterVal string
	listCmd := &cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "filter=%q\n", filterVal)
			return nil
		},
	}
	listCmd.Flags().StringVar(&filterVal, "filter", "", "이름 필터")
	flowCmd := &cobra.Command{Use: "flow"}
	flowCmd.AddCommand(listCmd)
	rootCmd.AddCommand(flowCmd)

	var client *Client
	var buf bytes.Buffer
	session := NewInteractiveSession(rootCmd, &client, &buf)
	session.pingFn = func() error { return nil }

	// 1) 로컬 플래그와 함께 실행
	_ = session.executeCommand("flow list --filter prod")
	first := buf.String()
	assert.Contains(t, first, `filter="prod"`, "첫 실행에서 --filter 가 파싱되어야 합니다")

	// 2) 로컬 플래그 없이 실행 - 이전 값이 누수되면 안 된다.
	buf.Reset()
	_ = session.executeCommand("flow list")
	second := buf.String()
	assert.Contains(t, second, `filter=""`,
		"서브커맨드 로컬 플래그(--filter)는 다음 명령으로 누수되면 안 됩니다. 출력: %s", second)
}

// TestREPL_RequiredFlagMissing 은 필수 플래그 누락 시 에러 메시지를 검증한다.
func TestREPL_RequiredFlagMissing(t *testing.T) {
	rootCmd := NewRootCmd()

	var client *Client
	var buf bytes.Buffer
	session := NewInteractiveSession(rootCmd, &client, &buf)

	// 필수 플래그 없이 실행
	_ = session.executeCommand("flow import")
	output := buf.String()
	t.Logf("필수 플래그 누락 출력:\n%s", output)

	// 에러 메시지에 원인이 표시되어야 함
	assert.Contains(t, output, "오류:",
		"필수 플래그 누락 시 에러 메시지가 표시되어야 합니다")
	assert.NotContains(t, output, "Usage:",
		"REPL 에서는 Usage 대신 에러 메시지만 표시되어야 합니다")
}
