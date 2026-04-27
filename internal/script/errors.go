// Package script 는 GopherLua 기반 Lua 스크립트 엔진을 제공한다.
package script

import (
	"errors"
	"fmt"
)

// sentinel error 정의
var (
	// ErrVMPoolExhausted 는 VM 풀에 사용 가능한 VM이 없을 때 반환된다.
	ErrVMPoolExhausted = errors.New("script: VM pool exhausted")

	// ErrScriptCompileFailed 는 스크립트 컴파일에 실패했을 때 반환된다.
	ErrScriptCompileFailed = errors.New("script: compile failed")

	// ErrScriptExecutionFailed 는 스크립트 실행 중 에러가 발생했을 때 반환된다.
	ErrScriptExecutionFailed = errors.New("script: execution failed")

	// ErrScriptTimeout 는 스크립트 실행이 제한 시간을 초과했을 때 반환된다.
	ErrScriptTimeout = errors.New("script: execution timeout")

	// ErrSandboxViolation 는 샌드박스 정책을 위반했을 때 반환된다.
	ErrSandboxViolation = errors.New("script: sandbox violation")

	// ErrScriptNotFound 는 스크립트를 캐시에서 찾을 수 없을 때 반환된다.
	ErrScriptNotFound = errors.New("script: script not found")

	// ErrInvalidScriptSource 는 잘못된 스크립트 소스가 전달되었을 때 반환된다.
	ErrInvalidScriptSource = errors.New("script: invalid script source")

	// ErrHotReloadFailed 는 핫 리로드에 실패했을 때 반환된다.
	ErrHotReloadFailed = errors.New("script: hot reload failed")
)

// ScriptError 는 스크립트 관련 에러의 상세 정보를 포함하는 구조체이다.
type ScriptError struct {
	// Err 는 원본 sentinel error이다.
	Err error
	// ScriptID 는 에러가 발생한 스크립트의 식별자이다.
	ScriptID string
	// Line 는 에러가 발생한 줄 번호이다 (0이면 줄 정보 없음).
	Line int
	// Detail 는 에러에 대한 상세 설명이다.
	Detail string
}

// Error 는 에러 메시지를 반환한다.
// Line이 0보다 크면 "[script:<ScriptID>:<Line>] <Detail>" 형식,
// 아니면 "[script:<ScriptID>] <Detail>" 형식이다.
func (e *ScriptError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("[script:%s:%d] %s", e.ScriptID, e.Line, e.Detail)
	}
	return fmt.Sprintf("[script:%s] %s", e.ScriptID, e.Detail)
}

// Unwrap 은 원본 에러를 반환한다. errors.Is/errors.As를 지원한다.
func (e *ScriptError) Unwrap() error {
	return e.Err
}
