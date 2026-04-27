package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chzyer/readline"
)

// chzyerReadline 은 chzyer/readline 라이브러리 기반의 readlineInterface 구현체이다.
// TTY 환경에서 화살표 키 히스토리 탐색, Ctrl+R 검색, 히스토리 파일 영속화를 지원한다.
type chzyerReadline struct {
	instance *readline.Instance
}

// ChzyerConfig 은 chzyer/readline 초기화 설정이다.
type ChzyerConfig struct {
	Prompt       string
	HistoryFile  string
	HistoryLimit int
	AutoComplete readline.AutoCompleter
}

// newChzyerReadline 은 chzyer/readline 기반 readlineInterface 를 생성한다.
// 히스토리 파일 디렉토리가 없으면 자동으로 생성한다.
func newChzyerReadline(config ChzyerConfig) (*chzyerReadline, error) {
	if config.HistoryFile != "" {
		dir := filepath.Dir(config.HistoryFile)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("히스토리 디렉토리 생성 실패: %w", err)
		}
	}

	rlConfig := &readline.Config{
		Prompt:            config.Prompt,
		HistoryFile:       config.HistoryFile,
		HistoryLimit:      config.HistoryLimit,
		AutoComplete:      config.AutoComplete,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
	}

	instance, err := readline.NewEx(rlConfig)
	if err != nil {
		return nil, fmt.Errorf("readline 초기화 실패: %w", err)
	}

	return &chzyerReadline{instance: instance}, nil
}

func (r *chzyerReadline) Readline() (string, error) {
	return r.instance.Readline()
}

func (r *chzyerReadline) SetPrompt(prompt string) {
	r.instance.SetPrompt(prompt)
}

func (r *chzyerReadline) Close() error {
	return r.instance.Close()
}

func (r *chzyerReadline) SaveHistory(cmd string) error {
	return r.instance.SaveHistory(cmd)
}

// isTerminal 은 표준 입력이 터미널인지 확인한다.
func isTerminal() bool {
	return readline.IsTerminal(int(os.Stdin.Fd()))
}

// compile-time interface check
var _ readlineInterface = (*chzyerReadline)(nil)
