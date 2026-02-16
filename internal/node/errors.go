package node

import (
	"errors"
	"fmt"
)

// 센티널 에러 정의 - Node 패키지에서 발생할 수 있는 모든 에러 타입
var (
	// ErrNodeTypeAlreadyRegistered 는 이미 등록된 노드 타입을 재등록할 때 반환된다.
	ErrNodeTypeAlreadyRegistered = errors.New("node: type already registered")

	// ErrNodeTypeNotFound 는 등록되지 않은 노드 타입을 조회할 때 반환된다.
	ErrNodeTypeNotFound = errors.New("node: type not found")

	// ErrPortNotFound 는 존재하지 않는 포트를 조회할 때 반환된다.
	ErrPortNotFound = errors.New("node: port not found")

	// ErrInvalidConfig 는 유효하지 않은 설정이 전달될 때 반환된다.
	ErrInvalidConfig = errors.New("node: invalid configuration")

	// ErrAgentNotFound 는 에이전트를 찾을 수 없을 때 반환된다.
	ErrAgentNotFound = errors.New("node: agent not found")

	// ErrRequestTimeout 은 요청-응답 타임아웃이 발생할 때 반환된다.
	ErrRequestTimeout = errors.New("node: request-reply timeout")

	// ErrScriptCompileFailed 는 스크립트 컴파일에 실패할 때 반환된다.
	ErrScriptCompileFailed = errors.New("node: script compile failed")

	// ErrScriptExecutionFailed 는 스크립트 실행에 실패할 때 반환된다.
	ErrScriptExecutionFailed = errors.New("node: script execution failed")

	// ErrScriptTimeout 은 스크립트 실행 타임아웃이 발생할 때 반환된다.
	ErrScriptTimeout = errors.New("node: script execution timeout")

	// ErrNodeNotInitialized 는 초기화되지 않은 노드에서 연산을 수행할 때 반환된다.
	ErrNodeNotInitialized = errors.New("node: not initialized")

	// ErrNodeAlreadyInitialized 는 이미 초기화된 노드를 재초기화할 때 반환된다.
	ErrNodeAlreadyInitialized = errors.New("node: already initialized")

	// ErrAggregateWindowInvalid 는 유효하지 않은 집계 윈도우가 설정될 때 반환된다.
	ErrAggregateWindowInvalid = errors.New("node: invalid aggregate window")
)

// NodeError 는 노드에서 발생한 에러를 래핑하는 구조체이다.
// 노드 ID와 타입 정보를 포함하여 에러 추적을 용이하게 한다.
type NodeError struct {
	NodeID   string // 에러가 발생한 노드의 ID
	NodeType string // 에러가 발생한 노드의 타입
	Err      error  // 실제 에러
}

// Error 는 "node [<NodeID>] (<NodeType>): <Err>" 포맷의 에러 메시지를 반환한다.
func (e *NodeError) Error() string {
	return fmt.Sprintf("node [%s] (%s): %s", e.NodeID, e.NodeType, e.Err)
}

// Unwrap 은 내부 에러를 반환하여 errors.Is/errors.As 체인을 지원한다.
func (e *NodeError) Unwrap() error {
	return e.Err
}
