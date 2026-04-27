package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// WizardQuestion 은 Wizard 단계별 프롬프트의 질문 항목이다.
type WizardQuestion struct {
	Name     string             // 파라미터 이름
	Prompt   string             // 사용자에게 표시할 프롬프트
	Default  string             // 기본값
	Required bool               // 필수 여부
	Options  []string           // Select 타입일 때 선택지
	Validate func(string) error // 유효성 검사 함수
}

// WizardRunner 는 단계별 Wizard 모드를 실행한다.
type WizardRunner struct {
	rootCmd   *cobra.Command
	client    **Client
	writer    io.Writer
	askFn     func(questions []*WizardQuestion) (map[string]string, error)
	confirmFn func(message string) (bool, error)
}

// NewWizardRunner 는 새로운 WizardRunner 를 생성한다.
func NewWizardRunner(rootCmd *cobra.Command, client **Client, writer io.Writer) *WizardRunner {
	return &WizardRunner{
		rootCmd: rootCmd,
		client:  client,
		writer:  writer,
	}
}

// Run 은 지정된 명령어 경로의 Wizard 를 실행한다.
func (w *WizardRunner) Run(commandPath string) error {
	if commandPath == "" {
		return fmt.Errorf("명령어 경로가 비어있습니다")
	}

	cmd, err := w.findCommand(commandPath)
	if err != nil {
		return err
	}

	// Wizard 헤더 출력
	header := w.buildHeader(cmd)
	fmt.Fprintln(w.writer, header)

	// 질문 생성
	questions := w.buildQuestions(cmd)

	// 사용자 입력 수집
	var answers map[string]string
	if w.askFn != nil {
		answers, err = w.askFn(questions)
		if err != nil {
			return err
		}
	}

	// 요약 표시
	summary := w.showSummary(cmd, answers)
	fmt.Fprintln(w.writer, summary)

	// 확인
	if w.confirmFn != nil {
		confirmed, err := w.confirmFn("실행하시겠습니까?")
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(w.writer, "작업이 취소되었습니다.")
			return nil
		}
	}

	// 실행
	return w.executeFromAnswers(cmd, answers)
}

// findCommand 는 명령어 경로 문자열에서 cobra.Command 를 찾는다.
func (w *WizardRunner) findCommand(path string) (*cobra.Command, error) {
	parts := strings.Fields(path)
	if len(parts) == 0 {
		return nil, fmt.Errorf("명령어 경로가 비어있습니다")
	}

	cmd := w.rootCmd
	for _, part := range parts {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == part {
				cmd = sub
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("명령어를 찾을 수 없습니다: %s", path)
		}
	}

	return cmd, nil
}

// buildQuestions 는 명령어의 인자와 플래그로부터 질문 목록을 생성한다.
// 순서: 필수 위치 인자 -> 선택적 위치 인자 -> 로컬 플래그
func (w *WizardRunner) buildQuestions(cmd *cobra.Command) []*WizardQuestion {
	var questions []*WizardQuestion

	// 1. Use 필드에서 위치 인자 추출 (예: "deploy <id>")
	useParts := strings.Fields(cmd.Use)
	for _, part := range useParts[1:] {
		name := strings.Trim(part, "<>[]")
		if name != "" {
			questions = append(questions, &WizardQuestion{
				Name:     name,
				Prompt:   fmt.Sprintf("%s 를 입력하세요", name),
				Required: strings.HasPrefix(part, "<"), // <arg> 는 필수, [arg] 는 선택
			})
		}
	}

	// 로컬 플래그에서 질문 생성
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Name == "help" {
			return
		}
		q := &WizardQuestion{
			Name:    f.Name,
			Prompt:  fmt.Sprintf("%s (%s)", f.Name, f.Usage),
			Default: f.DefValue,
		}

		// --format 플래그는 선택지 제공
		if f.Name == "format" {
			q.Options = []string{"json", "yaml", "table", "text"}
		}

		questions = append(questions, q)
	})

	return questions
}

// buildHeader 는 Wizard 헤더 문자열을 생성한다.
// 형식: "=== Flow Deploy Wizard ==="
func (w *WizardRunner) buildHeader(cmd *cobra.Command) string {
	cmdPath := w.buildCommandPath(cmd)

	// 각 경로 요소를 대문자로 시작하도록 변환
	capitalized := make([]string, len(cmdPath))
	for i, p := range cmdPath {
		capitalized[i] = capitalize(p)
	}

	title := strings.Join(capitalized, " ")
	return fmt.Sprintf("=== %s Wizard ===", title)
}

// showSummary 는 수집된 파라미터의 요약 테이블을 반환한다.
func (w *WizardRunner) showSummary(cmd *cobra.Command, answers map[string]string) string {
	var sb strings.Builder

	sb.WriteString("\n실행 요약:\n")
	sb.WriteString(strings.Repeat("-", 40) + "\n")
	sb.WriteString(fmt.Sprintf("%-15s %s\n", "파라미터", "값"))
	sb.WriteString(strings.Repeat("-", 40) + "\n")

	for k, v := range answers {
		sb.WriteString(fmt.Sprintf("%-15s %s\n", k, v))
	}
	sb.WriteString(strings.Repeat("-", 40) + "\n")

	return sb.String()
}

// executeFromAnswers 는 답변 맵을 Cobra 인자로 변환하여 명령어를 실행한다.
// answers 맵을 수정하지 않는다 (부작용 없음).
func (w *WizardRunner) executeFromAnswers(cmd *cobra.Command, answers map[string]string) error {
	// 인자 이름 집합: 플래그와 구분하기 위해 사용
	argNames := parseArgNames(cmd.Use)
	argSet := make(map[string]bool, len(argNames))
	for _, name := range argNames {
		argSet[name] = true
	}

	// 위치 인자를 Use 필드 순서대로 추가
	var positionalArgs []string
	for _, name := range argNames {
		if val, ok := answers[name]; ok {
			positionalArgs = append(positionalArgs, val)
		}
	}

	// 나머지 답변은 플래그로 추가
	var flagArgs []string
	for k, v := range answers {
		if argSet[k] {
			continue // 위치 인자는 이미 처리
		}
		flagArgs = append(flagArgs, fmt.Sprintf("--%s=%s", k, v))
	}

	// 명령어 경로 구축 (예: flow deploy -> ["flow", "deploy"])
	cmdPath := w.buildCommandPath(cmd)

	// 최종 인자: [명령어 경로] [위치 인자] [플래그]
	fullArgs := make([]string, 0, len(cmdPath)+len(positionalArgs)+len(flagArgs))
	fullArgs = append(fullArgs, cmdPath...)
	fullArgs = append(fullArgs, positionalArgs...)
	fullArgs = append(fullArgs, flagArgs...)

	w.rootCmd.SetArgs(fullArgs)
	w.rootCmd.SetOut(w.writer)
	w.rootCmd.SetErr(w.writer)

	return w.rootCmd.Execute()
}

// buildCommandPath 는 Cobra 명령어에서 루트까지의 경로를 구축한다.
// 예: deploy -> ["flow", "deploy"]
func (w *WizardRunner) buildCommandPath(cmd *cobra.Command) []string {
	var path []string
	for c := cmd; c != nil && c != w.rootCmd; c = c.Parent() {
		path = append([]string{c.Name()}, path...)
	}
	return path
}

// parseArgNames 는 Cobra Use 필드에서 위치 인자 이름을 추출한다.
// 예: "deploy <id>" -> ["id"]
// 예: "create" -> []
func parseArgNames(use string) []string {
	var names []string
	for _, part := range strings.Fields(use)[1:] { // 첫 토큰은 명령어 이름
		if strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">") {
			names = append(names, strings.Trim(part, "<>"))
		} else if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
			names = append(names, strings.Trim(part, "[]"))
		}
	}
	return names
}

// capitalize 는 문자열의 첫 글자를 대문자로 변환한다.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
