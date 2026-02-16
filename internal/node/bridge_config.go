package node

import (
	"fmt"
	"time"

	"github.com/xtra/xflow/pkg/flow"
)

// BridgeConfig 는 BridgeNode의 설정을 정의하는 구조체이다.
type BridgeConfig struct {
	// AgentRef 는 연결 대상 에이전트 참조 정보이다.
	AgentRef flow.AgentRef

	// Direction 은 에이전트와 플로우 간의 데이터 흐름 방향이다.
	Direction flow.BridgeDirection

	// Transform 은 메시지 변환 설정이다.
	Transform TransformConfig

	// RequestTimeout 은 요청-응답 패턴의 타임아웃 시간이다.
	// BridgeRequestReply 모드에서만 필수이다.
	RequestTimeout time.Duration

	// ReconnectInterval 은 에이전트 재연결 시도 간격이다.
	ReconnectInterval time.Duration

	// MaxReconnectAttempts 는 최대 재연결 시도 횟수이다. 0이면 재연결하지 않는다.
	MaxReconnectAttempts int

	// BufferSize 는 수신 버퍼 크기이다. 최소 1이어야 한다.
	BufferSize int

	// ResponseTarget 은 응답을 보낼 대상 노드 ID이다. 비어있으면 기본 출력 포트를 사용한다.
	ResponseTarget string
}

// TransformConfig 는 메시지 변환 설정을 정의하는 구조체이다.
type TransformConfig struct {
	// Mode 는 변환 모드이다. "auto" 또는 "lua"를 지원한다.
	Mode string

	// LuaScript 는 Lua 변환 스크립트이다. Mode가 "lua"일 때만 사용된다.
	LuaScript string
}

// DefaultBridgeConfig 는 주어진 AgentRef에 대한 기본 BridgeConfig를 반환한다.
func DefaultBridgeConfig(agentRef flow.AgentRef) BridgeConfig {
	return BridgeConfig{
		AgentRef:             agentRef,
		Direction:            agentRef.Direction,
		Transform:            TransformConfig{Mode: "auto"},
		RequestTimeout:       30 * time.Second,
		ReconnectInterval:    5 * time.Second,
		MaxReconnectAttempts: 10,
		BufferSize:           256,
	}
}

// Validate 는 BridgeConfig의 유효성을 검증한다.
// 유효하지 않은 필드가 있으면 해당 필드를 설명하는 에러를 반환한다.
func (c BridgeConfig) Validate() error {
	// 1. AgentRef에 AgentName 또는 AgentID가 있어야 한다.
	if c.AgentRef.AgentName == "" && c.AgentRef.AgentID == "" {
		return fmt.Errorf("%w: AgentRef must have AgentName or AgentID", ErrInvalidConfig)
	}

	// 2. Direction이 유효해야 한다.
	switch c.Direction {
	case flow.BridgeIn, flow.BridgeOut, flow.BridgeInOut, flow.BridgeRequestReply:
		// 유효한 direction
	default:
		return fmt.Errorf("%w: invalid direction %q", ErrInvalidConfig, c.Direction)
	}

	// 3. RequestReply 모드에서 RequestTimeout이 양수여야 한다.
	if c.Direction == flow.BridgeRequestReply && c.RequestTimeout <= 0 {
		return fmt.Errorf("%w: RequestTimeout must be positive for RequestReply mode", ErrInvalidConfig)
	}

	// 4. MaxReconnectAttempts >= 0
	if c.MaxReconnectAttempts < 0 {
		return fmt.Errorf("%w: MaxReconnectAttempts must be >= 0", ErrInvalidConfig)
	}

	// 5. BufferSize >= 1
	if c.BufferSize < 1 {
		return fmt.Errorf("%w: BufferSize must be >= 1", ErrInvalidConfig)
	}

	// 6. Transform.Mode가 "auto" 또는 "lua"여야 한다.
	if c.Transform.Mode != "auto" && c.Transform.Mode != "lua" {
		return fmt.Errorf("%w: Transform.Mode must be \"auto\" or \"lua\", got %q", ErrInvalidConfig, c.Transform.Mode)
	}

	// 7. Lua 모드에서 LuaScript가 비어있으면 안 된다.
	if c.Transform.Mode == "lua" && c.Transform.LuaScript == "" {
		return fmt.Errorf("%w: LuaScript must not be empty when Transform.Mode is \"lua\"", ErrInvalidConfig)
	}

	return nil
}
