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
	ErrNASAAgentNotNASA = fmt.Errorf("samsung_nasa: %w: agent is not a Samsung NASA type", ErrInvalidConfig)

	// ErrNASAMissingAgentRef 는 agent_ref 설정이 없을 때 반환된다.
	ErrNASAMissingAgentRef = fmt.Errorf("samsung_nasa: %w: agent_ref is required", ErrInvalidConfig)

	// ErrNASANoResolver 는 AgentResolver가 설정되지 않았을 때 반환된다.
	ErrNASANoResolver = fmt.Errorf("samsung_nasa: %w: agent resolver not configured", ErrNodeNotInitialized)

	// ErrNASAProcessFailed 는 Agent Process() 호출이 실패했을 때 반환된다.
	ErrNASAProcessFailed = fmt.Errorf("samsung_nasa: agent process failed")

	// LGAP 노드 에러

	// ErrLGAPAgentNotLGAP 는 resolve된 Agent가 LG LGAP 타입이 아닐 때 반환된다.
	ErrLGAPAgentNotLGAP = fmt.Errorf("lgap: %w: agent is not an LG LGAP type", ErrInvalidConfig)

	// ErrLGAPMissingAgentRef 는 agent_ref 설정이 없을 때 반환된다.
	ErrLGAPMissingAgentRef = fmt.Errorf("lgap: %w: agent_ref is required", ErrInvalidConfig)

	// ErrLGAPNoResolver 는 AgentResolver가 설정되지 않았을 때 반환된다.
	ErrLGAPNoResolver = fmt.Errorf("lgap: %w: agent resolver not configured", ErrNodeNotInitialized)

	// ErrLGAPProcessFailed 는 Agent Process() 호출이 실패했을 때 반환된다.
	ErrLGAPProcessFailed = fmt.Errorf("lgap: agent process failed")

	// LG HVACR-02 노드 에러

	// ErrLGHvacr02AgentNotLGHvacr02 는 resolve된 Agent가 LG HVACR-02 타입이 아닐 때 반환된다.
	ErrLGHvacr02AgentNotLGHvacr02 = fmt.Errorf("lg_hvacr02: %w: agent is not an LG HVACR-02 type", ErrInvalidConfig)

	// ErrLGHvacr02MissingAgentRef 는 agent_ref 설정이 없을 때 반환된다.
	ErrLGHvacr02MissingAgentRef = fmt.Errorf("lg_hvacr02: %w: agent_ref is required", ErrInvalidConfig)

	// ErrLGHvacr02NoResolver 는 AgentResolver가 설정되지 않았을 때 반환된다.
	ErrLGHvacr02NoResolver = fmt.Errorf("lg_hvacr02: %w: agent resolver not configured", ErrNodeNotInitialized)

	// ErrLGHvacr02ProcessFailed 는 Agent Process() 호출이 실패했을 때 반환된다.
	ErrLGHvacr02ProcessFailed = fmt.Errorf("lg_hvacr02: agent process failed")

	// ErrLGHvacr02MissingAddress 는 제어 명령에 address가 누락되었을 때 반환된다.
	ErrLGHvacr02MissingAddress = fmt.Errorf("lg_hvacr02: %w: address is required for control commands", ErrInvalidConfig)

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

	// ErrSerialMissingAgentRef 는 시리얼 노드에 agent_ref 설정이 없을 때 반환된다.
	ErrSerialMissingAgentRef = fmt.Errorf("serial: %w: agent_ref is required", ErrInvalidConfig)

	// ErrSerialNoResolver 는 AgentResolver가 설정되지 않았을 때 반환된다.
	ErrSerialNoResolver = fmt.Errorf("serial: %w: agent resolver not configured", ErrNodeNotInitialized)

	// ErrSerialAgentNotReceiver 는 resolve된 Agent가 MessageReceiver 인터페이스를 구현하지 않을 때 반환된다.
	ErrSerialAgentNotReceiver = fmt.Errorf("serial-in: %w: agent does not implement MessageReceiver", ErrInvalidConfig)

	// TCP 노드 에러

	// ErrTCPMissingAgentRef 는 tcp 노드에 agent_ref 설정이 없을 때 반환된다.
	ErrTCPMissingAgentRef = fmt.Errorf("tcp: %w: agent_ref is required", ErrInvalidConfig)

	// ErrTCPNoResolver 는 AgentResolver가 설정되지 않았을 때 반환된다.
	ErrTCPNoResolver = fmt.Errorf("tcp: %w: agent resolver not configured", ErrNodeNotInitialized)

	// ErrTCPAgentNotReceiver 는 resolve된 Agent가 MessageReceiver 또는 ConnAwareReceiver 인터페이스를 구현하지 않을 때 반환된다.
	ErrTCPAgentNotReceiver = fmt.Errorf("tcp-in: %w: agent does not implement MessageReceiver or ConnAwareReceiver", ErrInvalidConfig)

	// Thingplus 게이트웨이 노드 에러 (thingplus-uplink / thingplus-downlink)

	// ErrThingplusMissingAgentRef 는 thingplus 노드에 agent_ref 설정이 없을 때 반환된다.
	ErrThingplusMissingAgentRef = fmt.Errorf("thingplus: %w: agent_ref is required", ErrInvalidConfig)

	// ErrThingplusNoResolver 는 AgentResolver가 설정되지 않았을 때 반환된다.
	ErrThingplusNoResolver = fmt.Errorf("thingplus: %w: agent resolver not configured", ErrNodeNotInitialized)

	// ErrThingplusAgentNotReceiver 는 resolve된 Agent가 MessageReceiver 인터페이스를 구현하지 않을 때 반환된다 (다운링크 전용).
	ErrThingplusAgentNotReceiver = fmt.Errorf("thingplus-downlink: %w: agent does not implement MessageReceiver", ErrInvalidConfig)

	// ErrThingplusAgentNotSubscriber 는 topics 가 지정되었으나 resolve된 Agent가 SubscriberAgent 인터페이스를 구현하지 않을 때 반환된다.
	ErrThingplusAgentNotSubscriber = fmt.Errorf("thingplus-downlink: %w: agent does not implement SubscriberAgent", ErrInvalidConfig)

	// ErrThingplusAgentUnsupported 는 resolve된 Agent가 업링크 Process 진입점(agent.Agent)을 제공하지 않을 때 반환된다 (업링크 전용).
	ErrThingplusAgentUnsupported = fmt.Errorf("thingplus-uplink: %w: agent does not support Process entrypoint", ErrInvalidConfig)

	// ErrThingplusPublishFailed 는 업링크 Process() 호출이 실패했을 때 반환된다.
	ErrThingplusPublishFailed = fmt.Errorf("thingplus-uplink: publish failed")
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
