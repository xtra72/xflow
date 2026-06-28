package cli

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// stdioReadline 은 bufio.Scanner 기반의 기본 readline 구현이다.
// 외부 의존성 없이 표준 입력에서 한 줄씩 읽는다.
type stdioReadline struct {
	scanner *bufio.Scanner
	prompt  string
	writer  io.Writer
}

func newStdioReadline(writer io.Writer) *stdioReadline {
	return &stdioReadline{
		scanner: bufio.NewScanner(os.Stdin),
		prompt:  defaultPrompt,
		writer:  writer,
	}
}

func (r *stdioReadline) Readline() (string, error) {
	fmt.Fprint(r.writer, r.prompt)
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	return r.scanner.Text(), nil
}

func (r *stdioReadline) SetPrompt(prompt string) {
	r.prompt = prompt
}

func (r *stdioReadline) Close() error {
	return nil
}

func (r *stdioReadline) SaveHistory(_ string) error {
	return nil
}

// REPL 프롬프트 및 메시지 상수
const (
	// defaultPrompt 는 클라이언트 미연결 시 기본 프롬프트이다.
	defaultPrompt = "xflow> "
	// disconnectedPrompt 는 서버 연결 실패 시 프롬프트이다.
	disconnectedPrompt = "xflow [disconnected]> "
	// maxSuggestionDistance 는 유사 명령어 제안의 최대 편집 거리이다.
	maxSuggestionDistance = 4
	// maxHistorySize 는 인메모리 히스토리의 최대 항목 수이다.
	maxHistorySize = 1000
	// defaultHistoryFile 은 히스토리 파일의 기본 상대 경로이다.
	defaultHistoryFile = ".xflow/history"
)

// readlineInterface 는 readline 의 테스트 가능한 인터페이스이다.
// 실제 readline.Instance 와 테스트용 mock 모두 이 인터페이스를 구현한다.
type readlineInterface interface {
	Readline() (string, error)
	SetPrompt(prompt string)
	Close() error
	SaveHistory(cmd string) error
}

// globalFlagState 는 루트 퍼시스턴트(전역) 플래그의 값과 Changed 상태 스냅샷이다.
type globalFlagState struct {
	value   string
	changed bool
}

// InteractiveSession 은 REPL 대화형 세션을 관리한다.
// readline 통합, 명령어 파싱, 세션 생명주기를 담당한다.
type InteractiveSession struct {
	rootCmd    *cobra.Command
	client     **Client
	writer     io.Writer
	histFile   string
	readlineFn readlineInterface
	pingFn     func() error // 서버 연결 확인 함수 (테스트 주입용)
	history    []string     // 인메모리 히스토리

	// globalFlagBaseline 은 세션 런치 시점의 루트 퍼시스턴트 플래그 스냅샷이다.
	// 매 명령 실행 후 이 값으로 전역 플래그를 복원하여, 런치 시점의 연결
	// 컨텍스트(--server/--config/--token/--insecure/--format 등)를 세션 전체에
	// 보존한다. 개별 명령의 전역 플래그 오버라이드는 다음 명령으로 누수되지 않는다.
	globalFlagBaseline map[string]globalFlagState
}

// NewInteractiveSession 은 새로운 REPL 세션을 생성한다.
// rootCmd 는 Cobra 루트 커맨드, client 는 API 클라이언트 더블 포인터,
// writer 는 출력 대상이다.
func NewInteractiveSession(rootCmd *cobra.Command, client **Client, writer io.Writer) *InteractiveSession {
	s := &InteractiveSession{
		rootCmd: rootCmd,
		client:  client,
		writer:  writer,
		history: make([]string, 0),
	}

	// 히스토리 파일 경로 설정
	if homeDir, err := os.UserHomeDir(); err == nil {
		s.histFile = filepath.Join(homeDir, defaultHistoryFile)
	}

	// TTY 환경이면 chzyer/readline 사용, 아니면 기본 stdioReadline
	if isTerminal() {
		var autoComplete readline.AutoCompleter
		if client != nil {
			ac := NewAutoCompleter(rootCmd, client, 30*time.Second)
			autoComplete = ac
		}

		rl, err := newChzyerReadline(ChzyerConfig{
			Prompt:       defaultPrompt,
			HistoryFile:  s.histFile,
			HistoryLimit: maxHistorySize,
			AutoComplete: autoComplete,
		})
		if err != nil {
			// readline 초기화 실패 시 경고 후 stdioReadline 폴백
			fmt.Fprintf(writer, "경고: readline 초기화 실패, 기본 모드로 전환합니다: %s\n", err.Error())
			s.readlineFn = newStdioReadline(writer)
		} else {
			s.readlineFn = rl
		}
	} else {
		s.readlineFn = newStdioReadline(writer)
	}

	// 기본 pingFn 설정: 실제 클라이언트의 Ping 메서드 호출
	s.pingFn = func() error {
		if s.client == nil || *s.client == nil {
			return fmt.Errorf("클라이언트 없음")
		}
		return (*s.client).Ping()
	}

	return s
}

// newInteractiveCmd 는 interactive Cobra 서브커맨드를 생성한다.
// rootCmd 와 client 를 받아 REPL 세션을 시작하는 커맨드를 반환한다.
func newInteractiveCmd(rootCmd *cobra.Command, client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "interactive",
		Aliases: []string{"i", "repl"},
		Short:   "대화형 REPL 모드 시작",
		Long:    "xflow 대화형 셸 세션을 시작합니다. 명령어를 반복적으로 입력하고 실행할 수 있습니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			writer := cmd.OutOrStdout()

			session := NewInteractiveSession(rootCmd, client, writer)

			return session.Start()
		},
	}

	return cmd
}

// Start 는 REPL 메인 루프를 시작한다.
// 환영 메시지 출력 후 사용자 입력을 반복적으로 처리한다.
func (s *InteractiveSession) Start() error {
	printWelcomeMessage(s.writer)
	return s.runLoop()
}

// Stop 은 세션 리소스를 정리한다.
func (s *InteractiveSession) Stop() {
	if s.readlineFn != nil {
		s.readlineFn.Close()
	}
}

// runLoop 는 REPL 의 핵심 읽기-실행-출력 루프이다.
// EOF 또는 exit/quit 입력까지 반복한다.
func (s *InteractiveSession) runLoop() error {
	defer func() {
		if s.readlineFn != nil {
			s.readlineFn.Close()
		}
	}()

	for {
		// 프롬프트 업데이트
		prompt := s.buildPrompt()
		if s.readlineFn != nil {
			s.readlineFn.SetPrompt(prompt)
		}

		// 입력 읽기
		line, err := s.readlineFn.Readline()
		if err != nil {
			// EOF (Ctrl+D) 로 종료
			printExitMessage(s.writer)
			return nil
		}

		// 앞뒤 공백 제거
		input := strings.TrimSpace(line)

		// 빈 입력은 무시
		if input == "" {
			continue
		}

		// 히스토리에 저장 (최대 크기 제한)
		s.history = append(s.history, input)
		if len(s.history) > maxHistorySize {
			s.history = s.history[len(s.history)-maxHistorySize:]
		}
		s.readlineFn.SaveHistory(input)

		// 특수 명령어 처리
		handled, shouldExit := s.handleSpecialCommand(input)
		if shouldExit {
			return nil
		}
		if handled {
			continue
		}

		// 일반 명령어 실행
		_ = s.executeCommand(input)
	}
}

// buildPrompt 는 현재 상태에 따른 프롬프트 문자열을 생성한다.
// 서버 연결 상태에 따라 호스트 정보 또는 disconnected 를 표시한다.
func (s *InteractiveSession) buildPrompt() string {
	// 클라이언트가 없으면 기본 프롬프트
	if s.client == nil || *s.client == nil {
		return defaultPrompt
	}

	client := *s.client
	host := extractHost(client.baseURL)

	// 서버 연결 확인
	if s.pingFn != nil {
		if err := s.pingFn(); err != nil {
			return disconnectedPrompt
		}
	}

	return fmt.Sprintf("xflow [%s]> ", host)
}

// handleSpecialCommand 는 REPL 특수 명령어를 처리한다.
// 반환값: (처리 여부, 종료 여부)
func (s *InteractiveSession) handleSpecialCommand(input string) (handled bool, shouldExit bool) {
	// 첫 단어만 추출하여 특수 명령어 판별
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false, false
	}

	cmd := parts[0]

	switch cmd {
	case "help", "?":
		s.printHelp()
		return true, false

	case "exit", "quit":
		printExitMessage(s.writer)
		return true, true

	case "clear":
		// ANSI 이스케이프로 화면 지우기
		fmt.Fprint(s.writer, "\033[2J\033[H")
		return true, false

	case "history":
		if len(parts) >= 2 {
			switch parts[1] {
			case "clear":
				s.clearHistory()
			case "search":
				if len(parts) >= 3 {
					s.searchHistory(strings.Join(parts[2:], " "))
				} else {
					fmt.Fprintln(s.writer, "사용법: history search <패턴>")
				}
			default:
				s.printHistory()
			}
		} else {
			s.printHistory()
		}
		return true, false

	case "wizard":
		wizardArgs := strings.TrimPrefix(input, "wizard ")
		wizardArgs = strings.TrimSpace(wizardArgs)
		if wizardArgs == "" || wizardArgs == "wizard" {
			fmt.Fprintln(s.writer, "사용법: wizard <명령어> (예: wizard flow deploy)")
			return true, false
		}
		runner := NewWizardRunner(s.rootCmd, s.client, s.writer)
		if err := runner.Run(wizardArgs); err != nil {
			fmt.Fprintf(s.writer, "Wizard 오류: %s\n", err.Error())
		}
		return true, false

	case "source", "run":
		if len(parts) < 2 {
			fmt.Fprintf(s.writer, "사용법: %s <파일경로>\n", cmd)
			return true, false
		}
		filePath := parts[1]
		s.executeScriptFile(filePath)
		return true, false

	default:
		return false, false
	}
}

// executeCommand 는 REPL 입력을 파싱하고 Cobra 명령어 트리에서 실행한다.
// 빈 입력은 무시하고, 잘못된 명령어는 유사 명령어를 제안한다.
func (s *InteractiveSession) executeCommand(input string) error {
	args := tokenizeInput(input)
	if len(args) == 0 {
		return nil
	}

	// 중첩 interactive 방지
	if args[0] == "interactive" || args[0] == "i" || args[0] == "repl" {
		fmt.Fprintln(s.writer, "이미 대화형 모드에 있습니다")
		return nil
	}

	// 첫 명령 실행 직전, 런치 시점의 전역 플래그 컨텍스트를 한 번만 스냅샷한다.
	// 이 시점에는 아직 어떤 명령도 실행되지 않았으므로, 현재 전역 플래그 값은
	// 곧 런치 시 전달된 --server/--config/--token/--insecure/--format 값과 같다.
	s.captureGlobalFlagBaseline()

	// REPL 출력 설정
	s.rootCmd.SetArgs(args)
	s.rootCmd.SetOut(s.writer)
	s.rootCmd.SetErr(s.writer)

	// REPL 에서는 런타임 에러 시 Usage 출력을 억제하고 에러 메시지만 표시한다.
	// Usage 는 help 명령어 또는 --help 로 명시적으로 확인할 수 있다.
	s.rootCmd.SilenceUsage = true
	s.rootCmd.SilenceErrors = true

	// 명령어 조회: 유효한 명령어인지 먼저 확인
	foundCmd, _, findErr := s.rootCmd.Find(args)
	commandFound := findErr == nil && foundCmd != nil && foundCmd != s.rootCmd

	// 명령어 실행
	err := s.rootCmd.Execute()

	// 플래그 상태 리셋: 다음 명령어에 영향을 주지 않도록
	s.resetFlags()

	if err != nil {
		if !commandFound {
			// 명령어를 찾지 못한 경우 유사 명령어 제안
			suggestion := s.suggestCommand(args[0])
			if suggestion != "" {
				fmt.Fprintf(s.writer, "혹시 '%s' 를 의미하셨나요?\n", suggestion)
			} else {
				fmt.Fprintln(s.writer, "help 또는 ?로 사용 가능한 명령어를 확인하세요")
			}
		} else {
			// 유효한 명령어의 런타임 에러는 에러 메시지를 직접 표시
			fmt.Fprintf(s.writer, "오류: %s\n", err.Error())
		}
		return nil // REPL 에서는 에러를 전파하지 않음
	}

	return nil
}

// captureGlobalFlagBaseline 은 루트 퍼시스턴트(전역) 플래그의 런치 시점 상태를
// 단 한 번만 스냅샷한다. 이미 캡처되었거나 rootCmd 가 nil 이면 아무것도 하지 않는다.
func (s *InteractiveSession) captureGlobalFlagBaseline() {
	if s.globalFlagBaseline != nil || s.rootCmd == nil {
		return
	}
	s.globalFlagBaseline = make(map[string]globalFlagState)
	s.rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		s.globalFlagBaseline[f.Name] = globalFlagState{value: f.Value.String(), changed: f.Changed}
	})
}

// resetFlags 는 서브커맨드 로컬 플래그를 기본값으로 리셋하되,
// 루트 커맨드의 퍼시스턴트(전역) 플래그는 세션 런치 시점 값으로 복원한다.
//
// REPL 은 매 명령마다 rootCmd.Execute() 를 호출하고, 그때마다 PersistentPreRunE 가
// --server/--config/--token/--insecure 를 다시 읽어 클라이언트를 재생성한다.
// 따라서 전역 플래그까지 기본값으로 되돌리면 두 번째 명령부터 런치 시점의 연결
// 컨텍스트(서버/설정/토큰/insecure/format)가 사라진다. 이를 막기 위해 전체 리셋 후
// 런치 시점 스냅샷(globalFlagBaseline)으로 전역 플래그를 복원한다.
//
// 복원 기준이 "직전 명령이 남긴 값"이 아니라 "런치 시점 값"이므로, 개별 명령의
// 전역 플래그 오버라이드(예: version --format json)는 다음 명령으로 누수되지 않으며,
// 동시에 런치 시 전달한 전역 컨텍스트는 세션 내내 유지된다.
// 서브커맨드 로컬 플래그는 그대로 리셋되어 명령 간 누수를 방지한다.
func (s *InteractiveSession) resetFlags() {
	// 안전장치: 베이스라인이 아직 없으면 현재 상태를 런치 시점으로 간주해 캡처한다.
	s.captureGlobalFlagBaseline()

	// 1) 기존 동작대로 전체 플래그 리셋 (로컬 플래그 누수 방지 포함)
	resetCommandFlags(s.rootCmd)

	// 2) 루트 퍼시스턴트 플래그를 런치 시점 값으로 복원하여 세션 전역 컨텍스트를 유지
	s.rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		base, ok := s.globalFlagBaseline[f.Name]
		if !ok {
			return
		}
		_ = f.Value.Set(base.value)
		f.Changed = base.changed
	})
}

// resetCommandFlags 는 지정된 커맨드와 하위 서브커맨드의 플래그를 재귀적으로 리셋한다.
func resetCommandFlags(cmd *cobra.Command) {
	// 퍼시스턴트 플래그 리셋
	cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
	// 로컬 플래그 리셋
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
	// 서브커맨드 재귀 리셋
	for _, sub := range cmd.Commands() {
		resetCommandFlags(sub)
	}
}

// suggestCommand 는 입력된 명령어와 가장 유사한 등록된 명령어를 제안한다.
// Levenshtein 거리 기반으로 가장 가까운 명령어를 반환한다.
// 편집 거리가 3 이하인 경우에만 제안하고, 없으면 빈 문자열을 반환한다.
func (s *InteractiveSession) suggestCommand(input string) string {
	if input == "" {
		return ""
	}

	commands := s.collectAvailableCommands()

	bestMatch := ""
	bestDist := maxSuggestionDistance

	for _, cmd := range commands {
		dist := levenshteinDistance(input, cmd)
		if dist < bestDist {
			bestDist = dist
			bestMatch = cmd
		}
	}

	return bestMatch
}

// collectAvailableCommands 는 루트 커맨드에 등록된 모든 최상위 명령어 이름을 수집한다.
func (s *InteractiveSession) collectAvailableCommands() []string {
	var commands []string
	for _, cmd := range s.rootCmd.Commands() {
		if cmd.Name() == "help" || cmd.Name() == "completion" {
			continue // Cobra 자동 생성 명령어 제외
		}
		commands = append(commands, cmd.Name())
	}
	return commands
}

// executeScriptFile 은 REPL 내에서 스크립트 파일을 실행한다.
// source/run 특수 명령어에서 호출된다.
func (s *InteractiveSession) executeScriptFile(filePath string) {
	lines, err := ParseScriptFile(filePath)
	if err != nil {
		fmt.Fprintf(s.writer, "스크립트 오류: %s\n", err.Error())
		return
	}

	executor := NewScriptExecutor(s, false, s.writer)
	result := executor.Execute(lines)
	result.PrintSummary(s.writer)
}

// printHelp 는 REPL 사용 가능한 명령어 목록을 출력한다.
func (s *InteractiveSession) printHelp() {
	fmt.Fprintln(s.writer, "사용 가능한 명령어:")
	fmt.Fprintln(s.writer, "")

	// 특수 명령어
	fmt.Fprintln(s.writer, "  REPL 특수 명령어:")
	fmt.Fprintln(s.writer, "    help, ?     사용 가능한 명령어 목록 표시")
	fmt.Fprintln(s.writer, "    exit, quit  REPL 세션 종료")
	fmt.Fprintln(s.writer, "    clear       화면 지우기")
	fmt.Fprintln(s.writer, "    history          최근 명령어 히스토리 표시")
	fmt.Fprintln(s.writer, "    history clear    히스토리 삭제")
	fmt.Fprintln(s.writer, "    history search   히스토리 검색 (예: history search flow)")
	fmt.Fprintln(s.writer, "    wizard      Wizard 모드 진입 (예: wizard flow deploy)")
	fmt.Fprintln(s.writer, "    source      스크립트 파일 실행 (예: source script.xflow)")
	fmt.Fprintln(s.writer, "    run         스크립트 파일 실행 (source 와 동일)")
	fmt.Fprintln(s.writer, "")

	// Cobra 등록 명령어
	fmt.Fprintln(s.writer, "  CLI 명령어:")
	for _, cmd := range s.rootCmd.Commands() {
		if cmd.Name() == "help" || cmd.Name() == "completion" || cmd.Name() == "interactive" {
			continue
		}
		fmt.Fprintf(s.writer, "    %-12s %s\n", cmd.Name(), cmd.Short)
	}
	fmt.Fprintln(s.writer, "")
	fmt.Fprintln(s.writer, "  'xflow' 접두사는 생략 가능합니다. (예: flow list)")
}

// printHistory 는 인메모리 히스토리를 출력한다.
func (s *InteractiveSession) printHistory() {
	if len(s.history) == 0 {
		fmt.Fprintln(s.writer, "히스토리가 비어있습니다.")
		return
	}

	for i, cmd := range s.history {
		fmt.Fprintf(s.writer, "  %d  %s\n", i+1, cmd)
	}
}

// clearHistory 는 인메모리 히스토리와 히스토리 파일을 모두 삭제한다.
func (s *InteractiveSession) clearHistory() {
	s.history = make([]string, 0)
	if s.histFile != "" {
		_ = os.Truncate(s.histFile, 0)
	}
	fmt.Fprintln(s.writer, "히스토리가 삭제되었습니다.")
}

// searchHistory 는 히스토리에서 패턴과 일치하는 항목을 검색하여 출력한다.
// 대소문자를 무시한 부분 문자열 매칭을 수행한다.
func (s *InteractiveSession) searchHistory(pattern string) {
	if pattern == "" {
		fmt.Fprintln(s.writer, "사용법: history search <패턴>")
		return
	}

	lowerPattern := strings.ToLower(pattern)
	found := false

	for i, cmd := range s.history {
		if strings.Contains(strings.ToLower(cmd), lowerPattern) {
			fmt.Fprintf(s.writer, "  %d  %s\n", i+1, cmd)
			found = true
		}
	}

	if !found {
		fmt.Fprintf(s.writer, "'%s' 와 일치하는 히스토리가 없습니다.\n", pattern)
	}
}

// printWelcomeMessage 는 REPL 시작 시 환영 메시지를 출력한다.
func printWelcomeMessage(w io.Writer) {
	fmt.Fprintln(w, "xflow 대화형 모드에 오신 것을 환영합니다!")
	fmt.Fprintln(w, "help 또는 ?로 명령어 목록을 확인하세요.")
	fmt.Fprintln(w, "")
}

// printExitMessage 는 REPL 종료 시 종료 메시지를 출력한다.
func printExitMessage(w io.Writer) {
	fmt.Fprintln(w, "세션을 종료합니다.")
}

// tokenizeInput 는 REPL 입력 문자열을 토큰 슬라이스로 분리한다.
// 따옴표로 감싼 문자열을 올바르게 처리하고, xflow 접두사를 제거한다.
func tokenizeInput(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return []string{}
	}

	tokens := shellSplit(input)

	// xflow 접두사 제거
	if len(tokens) > 0 && tokens[0] == "xflow" {
		tokens = tokens[1:]
	}

	if len(tokens) == 0 {
		return []string{}
	}

	return tokens
}

// shellSplit 은 셸 스타일로 입력을 토큰화한다.
// 작은따옴표와 큰따옴표를 처리한다.
func shellSplit(input string) []string {
	var tokens []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false

	for i := 0; i < len(input); i++ {
		ch := input[i]

		switch {
		case ch == '\'' && !inDoubleQuote:
			inSingleQuote = !inSingleQuote
		case ch == '"' && !inSingleQuote:
			inDoubleQuote = !inDoubleQuote
		case (ch == ' ' || ch == '\t') && !inSingleQuote && !inDoubleQuote:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// extractHost 는 URL 에서 호스트(:포트) 부분을 추출한다.
func extractHost(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return rawURL
	}

	return parsed.Host
}

// levenshteinDistance 는 두 문자열 사이의 편집 거리를 계산한다.
// 삽입, 삭제, 교체 연산의 최소 횟수를 반환한다.
func levenshteinDistance(a, b string) int {
	la := len(a)
	lb := len(b)

	// 기본 케이스
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// DP 테이블 생성
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(
				prev[j]+1,      // 삭제
				curr[j-1]+1,    // 삽입
				prev[j-1]+cost, // 교체
			)
		}
		prev, curr = curr, prev
	}

	return prev[lb]
}
