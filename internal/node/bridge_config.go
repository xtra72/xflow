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

	// Topics 는 Bridge 초기화 시 에이전트에 자동 구독을 요청할 토픽 목록이다.
	// 에이전트가 SubscriberAgent 인터페이스를 구현하는 경우에만 적용된다.
	Topics []string
}

// PayloadFormat 상수는 에이전트 데이터를 메시지 페이로드로 변환하는 방식을 정의한다.
const (
	// PayloadFormatAuto 는 JSON 파싱을 시도하고, 실패 시 raw 문자열로 래핑한다 (기본값).
	PayloadFormatAuto = "auto"

	// PayloadFormatJSON 은 엄격한 JSON 파싱만 허용한다. JSON이 아니면 에러를 반환한다.
	PayloadFormatJSON = "json"

	// PayloadFormatRaw 는 항상 원본 데이터를 문자열로 "raw" 키에 저장한다.
	PayloadFormatRaw = "raw"

	// PayloadFormatBinary 는 원본 바이트 데이터를 "_raw" 키에 []byte로 저장한다.
	PayloadFormatBinary = "binary"
)

// TransformConfig 는 메시지 변환 설정을 정의하는 구조체이다.
type TransformConfig struct {
	// Mode 는 변환 모드이다. "auto" 또는 "lua"를 지원한다.
	Mode string

	// PayloadFormat 은 에이전트 수신 데이터의 페이로드 변환 방식이다.
	// "auto" (기본값): JSON 파싱 시도 → 실패 시 {"raw": string}
	// "json": 엄격한 JSON 파싱, 실패 시 에러
	// "raw": 항상 {"raw": string(data)} 으로 저장
	// "binary": 항상 {"_raw": []byte(data)} 으로 저장
	PayloadFormat string

	// LuaScript 는 Lua 변환 스크립트이다. Mode가 "lua"일 때만 사용된다.
	LuaScript string
}

// DefaultBridgeConfig 는 주어진 AgentRef에 대한 기본 BridgeConfig를 반환한다.
func DefaultBridgeConfig(agentRef flow.AgentRef) BridgeConfig {
	return BridgeConfig{
		AgentRef:             agentRef,
		Direction:            agentRef.Direction,
		Transform:            TransformConfig{Mode: "auto", PayloadFormat: PayloadFormatAuto},
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

	// 8. PayloadFormat이 유효해야 한다.
	switch c.Transform.PayloadFormat {
	case "", PayloadFormatAuto, PayloadFormatJSON, PayloadFormatRaw, PayloadFormatBinary:
		// 유효한 값 ("" 는 auto로 취급)
	default:
		return fmt.Errorf("%w: Transform.PayloadFormat must be one of \"auto\", \"json\", \"raw\", \"binary\", got %q",
			ErrInvalidConfig, c.Transform.PayloadFormat)
	}

	return nil
}
