package node

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/modbus"
	"github.com/xtra/xflow/internal/agent/modbusserver"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 센티널 에러 정의 (공통)
// ---------------------------------------------------------------------------

var (
	// ErrModbusAgentNotMODBUS 는 resolve된 Agent가 MODBUS 타입이 아닐 때 반환된다.
	ErrModbusAgentNotMODBUS = fmt.Errorf("modbus: %w: agent is not a MODBUS type", ErrInvalidConfig)

	// ErrModbusMissingAgentRef 는 agent_ref 설정이 없을 때 반환된다.
	ErrModbusMissingAgentRef = fmt.Errorf("modbus: %w: agent_ref is required", ErrInvalidConfig)

	// ErrModbusInvalidOperation 은 operation이 read/write가 아닐 때 반환된다.
	ErrModbusInvalidOperation = fmt.Errorf("modbus: %w: operation must be \"read\" or \"write\"", ErrInvalidConfig)

	// ErrModbusInvalidRegisterArea 는 지원하지 않는 register_area 값일 때 반환된다.
	ErrModbusInvalidRegisterArea = fmt.Errorf("modbus: %w: unsupported register_area", ErrInvalidConfig)

	// ErrModbusReadOnlyArea 는 읽기 전용 영역에 쓰기를 시도할 때 반환된다.
	ErrModbusReadOnlyArea = fmt.Errorf("modbus: %w: cannot write to read-only register area", ErrInvalidConfig)

	// ErrModbusInvalidCount 는 count가 0 이하일 때 반환된다.
	ErrModbusInvalidCount = fmt.Errorf("modbus: %w: count must be positive", ErrInvalidConfig)

	// ErrModbusNoResolver 는 AgentResolver가 설정되지 않았을 때 반환된다.
	ErrModbusNoResolver = fmt.Errorf("modbus: %w: agent resolver not configured", ErrNodeNotInitialized)

	// ErrModbusWriteValueMissing 는 쓰기 연산 시 value/values가 없을 때 반환된다.
	ErrModbusWriteValueMissing = fmt.Errorf("modbus: write value missing: payload must contain \"value\" or \"values\" key")

	// ErrModbusProcessFailed 는 Agent Process() 호출이 실패했을 때 반환된다.
	ErrModbusProcessFailed = fmt.Errorf("modbus: agent process failed")
)

// ---------------------------------------------------------------------------
// 상수 정의 (공통)
// ---------------------------------------------------------------------------

const (
	// 레지스터 영역
	areaCoils            = "coils"
	areaDiscreteInputs   = "discrete_inputs"
	areaHoldingRegisters = "holding_registers"
	areaInputRegisters   = "input_registers"

	// 데이터 타입 기본값
	defaultDataType  = "uint16"
	defaultByteOrder = "big_endian"
	defaultTimeout   = 5 * time.Second

	// Agent 타입 식별자
	agentTypeServer = "server"
	agentTypeClient = "client"
)

// validRegisterAreas 는 유효한 레지스터 영역 목록이다.
var validRegisterAreas = map[string]bool{
	areaCoils:            true,
	areaDiscreteInputs:   true,
	areaHoldingRegisters: true,
	areaInputRegisters:   true,
}

// readOnlyAreas 는 읽기 전용 레지스터 영역이다.
var readOnlyAreas = map[string]bool{
	areaDiscreteInputs: true,
	areaInputRegisters: true,
}

// booleanAreas 는 boolean 타입 레지스터 영역이다 (data_type 무시).
var booleanAreas = map[string]bool{
	areaCoils:          true,
	areaDiscreteInputs: true,
}

// areaToFunctionCode 는 레지스터 영역을 Client Agent function_code로 매핑한다.
var areaToFunctionCode = map[string]byte{
	areaCoils:            1, // FC01
	areaDiscreteInputs:   2, // FC02
	areaHoldingRegisters: 3, // FC03
	areaInputRegisters:   4, // FC04
}

// ---------------------------------------------------------------------------
// ModbusConfig 구조체 (공통)
// ---------------------------------------------------------------------------

// ModbusConfig 는 Modbus 노드의 공통 설정 구조체이다.
// 명령 빌더/오버라이드 헬퍼가 이 구조체를 입력으로 받아 재사용한다.
type ModbusConfig struct {
	AgentRef     string `json:"agent_ref"`     // 대상 Agent 이름/ID
	Operation    string `json:"operation"`     // "read" | "write"
	RegisterArea string `json:"register_area"` // "coils" | "discrete_inputs" | "holding_registers" | "input_registers"
	Address      uint16 `json:"address"`       // 시작 레지스터 주소
	Count        uint16 `json:"count"`         // 레지스터 수 (기본값 1)
	DataType     string `json:"data_type"`     // "uint16" | "int16" | "float32" | "uint32" | "int32"
	ByteOrder    string `json:"byte_order"`    // "big_endian" | "little_endian"
	DeviceID     uint8  `json:"device_id"`     // Client Agent 전용 (기본값 1)
	Timeout      string `json:"timeout"`       // Process 호출 타임아웃 (기본값 "5s")

	// EmitMetadata 는 metadata 그룹 emit 정책을 제어한다 (P3).
	// 디바이스 노드가 아니므로 Agent(에이전트 그룹)만 사용한다.
	// parseEmitMetadata 가 Agent 기본 ON — emit_agent:false 로 비활성화.
	EmitMetadata MetadataEmitOptions `json:"emit_metadata"`
}

// ---------------------------------------------------------------------------
// modbusNodeBase - Modbus 노드 공통 기반 구조체
// ---------------------------------------------------------------------------

// modbusNodeBase 는 Modbus 노드 공통 기반 구조체이다.
// mqttNodeBase 패턴을 따르며, Agent 해석/호출/종료 로직을 공유한다.
type modbusNodeBase struct {
	*BaseNode
	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent   // 원본 Agent 객체
	agentType string        // "server" | "client"
	timeout   time.Duration // Process 호출 타임아웃
	mu        sync.RWMutex  // 설정 보호 뮤텍스
}

// ---------------------------------------------------------------------------
// AgentResolver 초기화
// ---------------------------------------------------------------------------

// initModbusResolver 는 BaseNode config에서 AgentResolver를 추출한다.
// BridgeNode, MQTTNode 등과 동일한 패턴으로 _agent_resolver 키를 사용한다.
func (mb *modbusNodeBase) initModbusResolver() {
	if mb.BaseNode.config != nil {
		if r, ok := mb.BaseNode.config["_agent_resolver"]; ok {
			if resolver, ok := r.(AgentResolver); ok {
				mb.resolver = resolver
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Agent Resolve & 타입 감지
// ---------------------------------------------------------------------------

// resolveModbusAgent 는 AgentResolver를 통해 Modbus 에이전트를 resolve하고
// Server/Client 타입을 자동 감지한다.
// 성공 시 mb.transport, mb.agent, mb.agentType 이 설정된다.
func (mb *modbusNodeBase) resolveModbusAgent(ctx context.Context, agentRef string) error {
	if mb.resolver == nil {
		return ErrModbusNoResolver
	}

	ref := flow.AgentRef{AgentID: agentRef, AgentName: agentRef}
	transport, err := mb.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("modbus init: agent resolve failed: %w", err)
	}
	mb.transport = transport

	// AgentAccessor를 통해 원본 Agent 객체 획득 및 타입 감지
	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return ErrModbusAgentNotMODBUS
	}

	underlyingAgent := accessor.UnderlyingAgent()
	switch underlyingAgent.(type) {
	case *modbusserver.ModbusServerAgent:
		mb.agentType = agentTypeServer
		mb.agent = underlyingAgent
	case *modbus.ModbusAgent:
		mb.agentType = agentTypeClient
		mb.agent = underlyingAgent
	default:
		return ErrModbusAgentNotMODBUS
	}

	return nil
}

// ---------------------------------------------------------------------------
// Agent Process 호출
// ---------------------------------------------------------------------------

// callModbusAgentProcess 는 Agent.Process()를 context timeout과 함께 호출한다.
func (mb *modbusNodeBase) callModbusAgentProcess(ctx context.Context, cmdBytes []byte) ([]byte, error) {
	mb.mu.RLock()
	a := mb.agent
	mb.mu.RUnlock()

	if a == nil {
		return nil, ErrModbusNoResolver
	}

	// context timeout 설정
	timeoutCtx, cancel := context.WithTimeout(ctx, mb.timeout)
	defer cancel()

	// 채널 기반 timeout 처리
	type processResult struct {
		data []byte
		err  error
	}
	ch := make(chan processResult, 1)

	go func() {
		data, err := a.Process(cmdBytes)
		ch <- processResult{data: data, err: err}
	}()

	select {
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("modbus: %w", timeoutCtx.Err())
	case result := <-ch:
		return result.data, result.err
	}
}

// ---------------------------------------------------------------------------
// 공통 종료 로직
// ---------------------------------------------------------------------------

// modbusShutdown 은 공통 종료 로직을 수행한다.
func (mb *modbusNodeBase) modbusShutdown() error {
	return mb.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// ---------------------------------------------------------------------------
// 읽기 명령 빌더 (Server/Client) — package-level, 재사용 가능
// ---------------------------------------------------------------------------

// buildModbusServerReadCommand 는 Server Agent 읽기 명령 JSON을 생성한다.
func buildModbusServerReadCommand(cfg ModbusConfig) ([]byte, error) {
	params := map[string]any{
		"address": cfg.Address,
	}

	var command string

	switch cfg.RegisterArea {
	case areaCoils:
		command = "get_coils"
		params["quantity"] = cfg.Count
	case areaDiscreteInputs:
		command = "get_discrete_inputs"
		params["quantity"] = cfg.Count
	case areaHoldingRegisters:
		if cfg.DataType != defaultDataType && !booleanAreas[cfg.RegisterArea] {
			command = "get_register_typed"
			params["area"] = cfg.RegisterArea
			params["data_type"] = cfg.DataType
			params["byte_order"] = cfg.ByteOrder
		} else {
			command = "get_holding_registers"
			params["quantity"] = cfg.Count
		}
	case areaInputRegisters:
		if cfg.DataType != defaultDataType && !booleanAreas[cfg.RegisterArea] {
			command = "get_register_typed"
			params["area"] = cfg.RegisterArea
			params["data_type"] = cfg.DataType
			params["byte_order"] = cfg.ByteOrder
		} else {
			command = "get_input_registers"
			params["quantity"] = cfg.Count
		}
	}

	req := map[string]any{
		"command": command,
		"params":  params,
	}
	return json.Marshal(req)
}

// buildModbusClientReadCommand 는 Client Agent 읽기 명령 JSON을 생성한다.
// read_raw 명령을 사용하여 개별 주소 읽기를 지원한다.
func buildModbusClientReadCommand(cfg ModbusConfig) ([]byte, error) {
	deviceID := fmt.Sprintf("%d", cfg.DeviceID)
	fc := areaToFunctionCode[cfg.RegisterArea]

	params := map[string]any{
		"function_code": fc,
		"address":       cfg.Address,
		"quantity":      cfg.Count,
	}

	req := map[string]any{
		"command":   "read_raw",
		"device_id": deviceID,
		"params":    params,
	}
	return json.Marshal(req)
}

// ---------------------------------------------------------------------------
// 쓰기 명령 빌더 (Server/Client) — package-level, 재사용 가능
// ---------------------------------------------------------------------------

// buildModbusServerWriteCommand 는 Server Agent 쓰기 명령 JSON을 생성한다.
func buildModbusServerWriteCommand(cfg ModbusConfig, value any, hasValue bool, values any, hasValues bool) ([]byte, error) {
	params := map[string]any{
		"address": cfg.Address,
	}

	var command string

	switch cfg.RegisterArea {
	case areaCoils:
		if cfg.Count > 1 && hasValues {
			command = "set_coils"
			params["values"] = values
		} else {
			command = "set_coil"
			params["value"] = value
		}

	case areaHoldingRegisters:
		if !booleanAreas[cfg.RegisterArea] && cfg.DataType != "" {
			params["data_type"] = cfg.DataType
			params["byte_order"] = cfg.ByteOrder
		}

		if cfg.Count > 1 && hasValues {
			command = "set_registers"
			params["values"] = values
		} else {
			command = "set_register"
			params["value"] = value
		}
	}

	req := map[string]any{
		"command": command,
		"params":  params,
	}
	return json.Marshal(req)
}

// buildModbusClientWriteCommand 는 Client Agent 쓰기 명령 JSON을 생성한다.
func buildModbusClientWriteCommand(cfg ModbusConfig, value any, hasValue bool, values any, hasValues bool) ([]byte, error) {
	deviceID := fmt.Sprintf("%d", cfg.DeviceID)
	params := map[string]any{
		"address": cfg.Address,
	}

	var command string

	switch cfg.RegisterArea {
	case areaCoils:
		if cfg.Count > 1 && hasValues {
			command = "write_coils"
			params["values"] = values
		} else {
			command = "write_coil"
			params["value"] = value
		}

	case areaHoldingRegisters:
		if !booleanAreas[cfg.RegisterArea] && cfg.DataType != "" {
			params["data_type"] = cfg.DataType
			params["byte_order"] = cfg.ByteOrder
		}

		if cfg.Count > 1 && hasValues {
			command = "write_registers"
			params["values"] = values
		} else {
			command = "write_register"
			params["value"] = value
		}
	}

	req := map[string]any{
		"command":   command,
		"device_id": deviceID,
		"params":    params,
	}
	return json.Marshal(req)
}

// ---------------------------------------------------------------------------
// 읽기 결과 추출
// ---------------------------------------------------------------------------

// extractReadResult 는 Agent 응답에서 읽기 결과를 추출한다.
func extractReadResult(resp map[string]any, agentType string, cfg ModbusConfig) any {
	switch agentType {
	case agentTypeServer:
		// Server Agent 응답: {ok, values, ...} 또는 {ok, value, ...}
		if values, ok := resp["values"]; ok {
			return values
		}
		if value, ok := resp["value"]; ok {
			return value
		}
		return resp
	case agentTypeClient:
		// Client Agent read_raw 응답: {data: "base64encoded"}
		if data, ok := resp["data"]; ok {
			return data
		}
		return resp
	}
	return resp
}

// ---------------------------------------------------------------------------
// 메시지 오버라이드 / 파라미터 변환 유틸리티
// ---------------------------------------------------------------------------

// applyMessageOverrides 는 입력 메시지 payload에서 설정값을 오버라이드한다.
// payload에 operation, register_area, address, count, data_type, byte_order, device_id 키가
// 있으면 해당 값으로 노드 설정을 덮어쓴다. 노드 config은 기본값, 메시지는 런타임 오버라이드이다.
func applyMessageOverrides(msg message.Message, cfg ModbusConfig) ModbusConfig {
	if msg.Payload() == nil {
		return cfg
	}

	if v, ok := msg.Payload().Get("operation"); ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.Operation = s
		}
	}
	if v, ok := msg.Payload().Get("register_area"); ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.RegisterArea = s
		}
	}
	if v, ok := msg.Payload().Get("address"); ok {
		cfg.Address = toUint16FromAny(v)
	}
	if v, ok := msg.Payload().Get("count"); ok {
		if c := toUint16FromAny(v); c > 0 {
			cfg.Count = c
		}
	}
	if v, ok := msg.Payload().Get("data_type"); ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.DataType = s
		}
	}
	if v, ok := msg.Payload().Get("byte_order"); ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.ByteOrder = s
		}
	}
	if v, ok := msg.Payload().Get("device_id"); ok {
		switch dv := v.(type) {
		case string:
			if dv != "" {
				if n, err := strconv.ParseUint(dv, 10, 8); err == nil {
					cfg.DeviceID = uint8(n)
				}
			}
		default:
			if d := toByte(v); d > 0 {
				cfg.DeviceID = d
			}
		}
	}

	return cfg
}

// toUint16FromAny 는 any 타입 값을 uint16으로 변환한다.
// JSON 디코딩 시 숫자가 float64로 전달되는 경우를 처리한다.
func toUint16FromAny(v any) uint16 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return uint16(n)
	case int:
		return uint16(n)
	case int64:
		return uint16(n)
	case uint16:
		return n
	case uint32:
		return uint16(n)
	case uint8:
		return uint16(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return uint16(i)
		}
	}
	return 0
}

// toByte 는 any 타입 값을 uint8로 변환한다.
func toByte(v any) uint8 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return uint8(n)
	case int:
		return uint8(n)
	case int64:
		return uint8(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return uint8(i)
		}
	}
	return 0
}
