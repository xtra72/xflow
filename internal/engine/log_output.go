package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xtra/xflow/pkg/flow"
)

// ---------------------------------------------------------------------------
// LogOutputTarget
// ---------------------------------------------------------------------------

// LogOutputTarget 는 파싱된 로그 출력 대상을 나타내는 구조체이다.
type LogOutputTarget struct {
	UseStdout bool   // stdout 출력 여부
	FilePath  string // 로그 파일 경로 (stdout 전용이거나 설정 없음이면 빈 문자열)
}

// ---------------------------------------------------------------------------
// ParseLogOutput
// ---------------------------------------------------------------------------

// ParseLogOutput 은 log_output 문자열을 LogOutputTarget 으로 파싱한다.
// config.ParseLogOutput 과 달리 파일을 열지 않고 구조체만 반환한다.
//
// 지원 형식:
//   - "" (빈 문자열)           -> LogOutputTarget{}, nil (설정 없음)
//   - "stdout"               -> {UseStdout: true, FilePath: ""}, nil
//   - "/path/to/file"        -> {UseStdout: false, FilePath: "/path/to/file"}, nil
//   - "stdout+/path/file"    -> {UseStdout: true, FilePath: "/path/file"}, nil
//   - "stdout+" (경로 없음)   -> error
func ParseLogOutput(raw string) (LogOutputTarget, error) {
	if raw == "" {
		return LogOutputTarget{}, nil
	}

	if raw == "stdout" {
		return LogOutputTarget{UseStdout: true}, nil
	}

	if strings.HasPrefix(raw, "stdout+") {
		filePath := strings.TrimPrefix(raw, "stdout+")
		if filePath == "" {
			return LogOutputTarget{}, fmt.Errorf("log_output 파싱 실패: stdout+ 뒤에 파일 경로가 필요합니다")
		}
		return LogOutputTarget{UseStdout: true, FilePath: filePath}, nil
	}

	// 단일 파일 경로
	return LogOutputTarget{FilePath: raw}, nil
}

// ---------------------------------------------------------------------------
// resolveNodeLogOutput
// ---------------------------------------------------------------------------

// resolveNodeLogOutput 은 계층적 우선순위로 로그 출력 대상을 결정한다.
//
//  1. 노드 config["log_output"]
//  2. 플로우 config.LogOutput
//  3. 서버 기본값
//
// 명시적 설정이 있으면 (target, true)를, 없으면 (LogOutputTarget{}, false)를 반환한다.
func resolveNodeLogOutput(nd flow.NodeDef, flowCfg flow.FlowConfig, serverDefault string) (LogOutputTarget, bool) {
	// 1. 노드 config["log_output"] 확인
	if nd.Config != nil {
		if outStr, ok := nd.Config["log_output"].(string); ok && outStr != "" {
			if target, err := ParseLogOutput(outStr); err == nil {
				return target, true
			}
		}
	}

	// 2. 플로우 config.LogOutput 확인
	if flowCfg.LogOutput != "" {
		if target, err := ParseLogOutput(flowCfg.LogOutput); err == nil {
			return target, true
		}
	}

	// 3. 서버 기본값 사용
	if serverDefault != "" {
		if target, err := ParseLogOutput(serverDefault); err == nil {
			return target, true
		}
	}

	return LogOutputTarget{}, false
}

// ---------------------------------------------------------------------------
// openLogFile
// ---------------------------------------------------------------------------

// openLogFile 은 로그 파일을 append 모드로 연다.
// 디렉토리가 없으면 os.MkdirAll로 자동 생성한다.
func openLogFile(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("로그 디렉토리 생성 실패: %w", err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("로그 파일 열기 실패: %w", err)
	}

	return file, nil
}
