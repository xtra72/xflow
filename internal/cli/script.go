package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// ExpandEnvInLines 는 ScriptLine 목록의 각 Command 에서 환경 변수를 치환한다.
// $VAR 및 ${VAR} 형식을 지원하며, 미정의 변수는 빈 문자열로 치환된다.
// 원본 슬라이스는 변경하지 않고 새 슬라이스를 반환한다.
func ExpandEnvInLines(lines []ScriptLine) []ScriptLine {
	result := make([]ScriptLine, len(lines))
	for i, line := range lines {
		result[i] = ScriptLine{
			LineNumber: line.LineNumber,
			Command:    os.ExpandEnv(line.Command),
		}
	}
	return result
}

// ScriptLine 은 스크립트 파일에서 파싱된 단일 명령어 줄을 나타낸다.
type ScriptLine struct {
	LineNumber int    // 원본 파일의 줄 번호
	Command    string // 파싱된 명령어 문자열
}

// ScriptError 는 단일 명령어 실행 시 발생한 에러를 나타낸다.
type ScriptError struct {
	Line    int    // 에러가 발생한 줄 번호
	Command string // 실패한 명령어
	Error   error  // 에러 상세
}

// ScriptResult 는 스크립트 실행 결과 요약을 담는다.
type ScriptResult struct {
	Total   int           // 전체 명령어 수
	Success int           // 성공한 명령어 수
	Failed  int           // 실패한 명령어 수
	Skipped int           // 건너뛴 명령어 수
	Errors  []ScriptError // 실패 상세 목록
}

// ScriptExecutor 는 스크립트 파일의 명령어를 순차 실행한다.
type ScriptExecutor struct {
	session         *InteractiveSession
	continueOnError bool
	writer          io.Writer
	expandEnv       bool // 환경 변수 치환 여부 (기본값: true)
	dryRun          bool // dry-run 모드: 명령어를 실행하지 않고 목록만 출력
	verbose         bool // verbose 모드: 각 명령어의 상세 실행 정보 출력
}

// NewScriptExecutor 는 새로운 ScriptExecutor 를 생성한다.
// session 은 명령어 실행에 사용할 InteractiveSession,
// continueOnError 가 true 이면 에러 시에도 다음 명령어를 계속 실행한다.
func NewScriptExecutor(session *InteractiveSession, continueOnError bool, writer io.Writer) *ScriptExecutor {
	return &ScriptExecutor{
		session:         session,
		continueOnError: continueOnError,
		writer:          writer,
		expandEnv:       true,
	}
}

// SetExpandEnv 는 환경 변수 치환 기능의 활성화 여부를 설정한다.
func (e *ScriptExecutor) SetExpandEnv(enabled bool) {
	e.expandEnv = enabled
}

// SetDryRun 는 dry-run 모드를 설정한다.
// dry-run 모드에서는 명령어를 실행하지 않고 목록만 출력한다.
func (e *ScriptExecutor) SetDryRun(enabled bool) {
	e.dryRun = enabled
}

// SetVerbose 는 verbose 모드를 설정한다.
// verbose 모드에서는 각 명령어의 상세 실행 정보를 출력한다.
func (e *ScriptExecutor) SetVerbose(enabled bool) {
	e.verbose = enabled
}

// ParseScriptFile 은 스크립트 파일을 파싱하여 명령어 목록을 반환한다.
// 주석(# 으로 시작), 빈 줄은 무시하고, 앞뒤 공백을 제거한다.
// 파일이 존재하지 않으면 파일 경로를 포함한 에러를 반환한다.
func ParseScriptFile(path string) ([]ScriptLine, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("스크립트 파일을 열 수 없습니다: %s: %w", path, err)
	}
	defer file.Close()

	var lines []ScriptLine
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// 빈 줄 무시
		if line == "" {
			continue
		}

		// 주석 무시
		if strings.HasPrefix(line, "#") {
			continue
		}

		lines = append(lines, ScriptLine{
			LineNumber: lineNum,
			Command:    line,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("스크립트 파일 읽기 오류: %s: %w", path, err)
	}

	if lines == nil {
		lines = []ScriptLine{}
	}

	return lines, nil
}

// Execute 는 파싱된 명령어 줄을 순차적으로 실행한다.
// continueOnError 설정에 따라 에러 발생 시 중단 또는 계속한다.
// expandEnv 가 true 이면 실행 전 환경 변수를 치환한다.
// dryRun 이 true 이면 명령어를 실행하지 않고 목록만 출력한다.
// verbose 가 true 이면 각 명령어의 상세 실행 정보를 출력한다.
func (e *ScriptExecutor) Execute(lines []ScriptLine) *ScriptResult {
	// 환경 변수 치환
	if e.expandEnv {
		lines = ExpandEnvInLines(lines)
	}

	result := &ScriptResult{
		Total:  len(lines),
		Errors: []ScriptError{},
	}

	for i, line := range lines {
		// dry-run 모드: 명령어를 실행하지 않고 목록만 출력
		if e.dryRun {
			fmt.Fprintf(e.writer, "[%d] (dry-run) %s\n", line.LineNumber, line.Command)
			continue
		}

		// verbose 모드: 실행 시작 메시지
		if e.verbose {
			fmt.Fprintf(e.writer, "[%d] 실행 시작: %s\n", line.LineNumber, line.Command)
		}

		// 진행 메시지 출력
		fmt.Fprintf(e.writer, "[%d] 실행: %s\n", line.LineNumber, line.Command)

		// 명령어 실행
		err := e.executeScriptCommand(line.Command)

		if err != nil {
			result.Failed++
			scriptErr := ScriptError{
				Line:    line.LineNumber,
				Command: line.Command,
				Error:   err,
			}
			result.Errors = append(result.Errors, scriptErr)

			fmt.Fprintf(e.writer, "  오류 (줄 %d): %s\n", line.LineNumber, err.Error())

			// verbose 모드: 실패 메시지
			if e.verbose {
				fmt.Fprintf(e.writer, "[%d] 실행 완료 (실패): %s\n", line.LineNumber, line.Command)
			}

			if !e.continueOnError {
				// 남은 명령어를 건너뛰기로 표시하고 중단
				result.Skipped = len(lines) - i - 1
				break
			}
		} else {
			result.Success++

			// verbose 모드: 성공 메시지
			if e.verbose {
				fmt.Fprintf(e.writer, "[%d] 실행 완료 (성공): %s\n", line.LineNumber, line.Command)
			}
		}
	}

	return result
}

// executeScriptCommand 는 스크립트 명령어를 실행하고 에러를 반환한다.
// REPL 의 executeCommand 와 달리 에러를 직접 반환한다.
func (e *ScriptExecutor) executeScriptCommand(input string) error {
	args := tokenizeInput(input)
	if len(args) == 0 {
		return nil
	}

	// REPL 출력 설정
	e.session.rootCmd.SetArgs(args)
	e.session.rootCmd.SetOut(e.session.writer)
	e.session.rootCmd.SetErr(e.session.writer)
	e.session.rootCmd.SilenceUsage = true
	e.session.rootCmd.SilenceErrors = true

	// 명령어 실행
	err := e.session.rootCmd.Execute()

	// 플래그 상태 리셋
	e.session.resetFlags()

	return err
}

// PrintSummary 는 스크립트 실행 결과 요약을 출력한다.
func (r *ScriptResult) PrintSummary(w io.Writer) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "--- 스크립트 실행 요약 ---")
	fmt.Fprintf(w, "전체: %d, 성공: %d, 실패: %d, 건너뜀: %d\n",
		r.Total, r.Success, r.Failed, r.Skipped)

	if len(r.Errors) > 0 {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "실패한 명령어:")
		for _, e := range r.Errors {
			fmt.Fprintf(w, "  줄 %d: %s - %s\n", e.Line, e.Command, e.Error.Error())
		}
	}
}

// HasErrors 는 실행 중 에러가 있었는지 반환한다.
func (r *ScriptResult) HasErrors() bool {
	return r.Failed > 0
}
