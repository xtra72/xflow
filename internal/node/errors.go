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

	// ErrAggregateFieldInvalid 는 유효하지 않은 집계 필드가 설정될 때 반환된다.
	ErrAggregateFieldInvalid = errors.New("node: invalid aggregate field")

	// ErrAggregateFnInvalid 는 유효하지 않은 집계 함수가 설정될 때 반환된다.
	ErrAggregateFnInvalid = errors.New("node: invalid aggregate function")

	// ErrAggregateGroupByInvalid 는 유효하지 않은 group_by 설정일 때 반환된다.
	ErrAggregateGroupByInvalid = errors.New("node: invalid aggregate group_by: must not be empty string")

	// ErrAggregateMaxGroupsInvalid 는 유효하지 않은 max_groups 설정일 때 반환된다.
	ErrAggregateMaxGroupsInvalid = errors.New("node: invalid aggregate max_groups: must be a positive integer")

	// ErrAggregateSlideIntervalInvalid 는 slide_interval이 window_size를 초과할 때 반환된다.
	ErrAggregateSlideIntervalInvalid = errors.New("node: invalid aggregate slide_interval: must be <= window_size")

	// ErrAggregateSlideIntervalParse 는 slide_interval 파싱에 실패할 때 반환된다.
	ErrAggregateSlideIntervalParse = errors.New("node: invalid aggregate slide_interval: cannot parse duration")

	// ErrInvalidExpression 은 유효하지 않은 변환 expression이 전달될 때 반환된다.
	ErrInvalidExpression = errors.New("node: invalid expression")

	// ErrTypeMismatch 는 연산에서 타입이 일치하지 않을 때 반환된다.
	ErrTypeMismatch = errors.New("node: type mismatch")

	// ErrDivisionByZero 는 0으로 나누기를 시도할 때 반환된다.
	ErrDivisionByZero = errors.New("node: division by zero")

	// ErrUndefinedVariable 는 정의되지 않은 변수를 참조할 때 반환된다.
	ErrUndefinedVariable = errors.New("node: undefined variable")

	// ErrUndefinedFunction 은 정의되지 않은 함수를 호출할 때 반환된다.
	ErrUndefinedFunction = errors.New("node: undefined function")

	// ErrArgumentCount 는 함수 인자 수가 일치하지 않을 때 반환된다.
	ErrArgumentCount = errors.New("node: wrong number of arguments")

	// ErrMappingKeyNotFound 는 매핑 테이블에 키가 없고 기본값이 설정되지 않았을 때 반환된다.
	ErrMappingKeyNotFound = errors.New("node: mapping key not found")

	// ErrMappingFieldNotFound 는 소스 필드가 메시지에 존재하지 않을 때 반환된다.
	ErrMappingFieldNotFound = errors.New("node: mapping source field not found")

	// ErrNASAAgentNotNASA 는 resolve된 Agent가 Samsung NASA 타입이 아닐 때 반환된다.
	ErrNASAAgentNotNASA = fmt.Errorf("nasa: %w: agent is not a Samsung NASA type", ErrInvalidConfig)

	// ErrNASAMissingAgentRef 는 agent_ref 설정이 없을 때 반환된다.
	ErrNASAMissingAgentRef = fmt.Errorf("nasa: %w: agent_ref is required", ErrInvalidConfig)

	// ErrNASANoResolver 는 AgentResolver가 설정되지 않았을 때 반환된다.
	ErrNASANoResolver = fmt.Errorf("nasa: %w: agent resolver not configured", ErrNodeNotInitialized)

	// ErrNASAProcessFailed 는 Agent Process() 호출이 실패했을 때 반환된다.
	ErrNASAProcessFailed = fmt.Errorf("nasa: agent process failed")

	// ErrMQTTMissingAgentRef 는 mqtt 노드에 agent_ref 설정이 없을 때 반환된다.
	ErrMQTTMissingAgentRef = fmt.Errorf("mqtt: %w: agent_ref is required", ErrInvalidConfig)

	// ErrMQTTNoResolver 는 AgentResolver가 설정되지 않았을 때 반환된다.
	ErrMQTTNoResolver = fmt.Errorf("mqtt: %w: agent resolver not configured", ErrNodeNotInitialized)

	// ErrMQTTAgentNotSubscriber 는 resolve된 Agent가 SubscriberAgent 인터페이스를 구현하지 않을 때 반환된다.
	ErrMQTTAgentNotSubscriber = fmt.Errorf("mqtt: %w: agent does not implement SubscriberAgent", ErrInvalidConfig)

	// ErrMQTTAgentNotReceiver 는 resolve된 Agent가 MessageReceiver 인터페이스를 구현하지 않을 때 반환된다.
	ErrMQTTAgentNotReceiver = fmt.Errorf("mqtt: %w: agent does not implement MessageReceiver", ErrInvalidConfig)

	// ErrMQTTAgentNotPublisher 는 resolve된 Agent가 MessagePublisher 인터페이스를 구현하지 않을 때 반환된다.
	ErrMQTTAgentNotPublisher = fmt.Errorf("mqtt: %w: agent does not implement MessagePublisher", ErrInvalidConfig)

	// ErrMQTTPublishFailed 는 MQTT 메시지 발행이 실패했을 때 반환된다.
	ErrMQTTPublishFailed = fmt.Errorf("mqtt: publish failed")
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
