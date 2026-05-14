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
// 센티널 에러 정의
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
// 상수 정의
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
// ModbusConfig 구조체
// ---------------------------------------------------------------------------

// ModbusConfig 는 ModbusNode의 설정 구조체이다.
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
}

// ---------------------------------------------------------------------------
// ModbusNode 구조체
// ---------------------------------------------------------------------------

// ModbusNode 는 MODBUS 에이전트의 레지스터를 읽기/쓰기하는 처리 노드이다.
// Server Agent와 Client Agent를 자동 감지하여 적절한 Process() 명령을 전송한다.
type ModbusNode struct {
	*BaseNode
	modbusConfig ModbusConfig
	resolver     AgentResolver
	transport    AgentTransport
	agent        agent.Agent   // 원본 Agent 객체
	agentType    string        // "server" | "client"
	timeout      time.Duration // Process 호출 타임아웃
	mu           sync.RWMutex  // 설정 보호 뮤텍스
}

// 인터페이스 컴파일 체크
var _ Node = (*ModbusNode)(nil)

// ---------------------------------------------------------------------------
// 팩토리 함수
// ---------------------------------------------------------------------------

// NewModbusNode 는 새로운 ModbusNode를 생성하는 팩토리 함수이다.
func NewModbusNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &ModbusNode{
		BaseNode: base,
	}

	// 옵션에서 AgentResolver 추출 (BridgeNode 패턴과 동일)
	if base.config != nil {
		if r, ok := base.config["_agent_resolver"]; ok {
			if resolver, ok := r.(AgentResolver); ok {
				n.resolver = resolver
			}
		}
	}

	return n, nil
}

// ---------------------------------------------------------------------------
// Configure
// ---------------------------------------------------------------------------

// Configure 는 ModbusNode의 설정을 적용한다.
// agent_ref(필수), operation(필수), register_area(필수), address(필수)를 검증한다.
func (n *ModbusNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg ModbusConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrModbusMissingAgentRef
	}

	// operation (필수)
	if v, ok := config["operation"]; ok {
		if s, ok := v.(string); ok {
			cfg.Operation = s
		}
	}
	if cfg.Operation != "read" && cfg.Operation != "write" {
		return ErrModbusInvalidOperation
	}

	// register_area (필수)
	if v, ok := config["register_area"]; ok {
		if s, ok := v.(string); ok {
			cfg.RegisterArea = s
		}
	}
	if !validRegisterAreas[cfg.RegisterArea] {
		return ErrModbusInvalidRegisterArea
	}

	// 읽기 전용 영역에 쓰기 시도 검증 (R-MBRW-004)
	if cfg.Operation == "write" && readOnlyAreas[cfg.RegisterArea] {
		return ErrModbusReadOnlyArea
	}

	// address (필수)
	cfg.Address = toUint16FromAny(config["address"])

	// count (선택, 기본값 1)
	cfg.Count = 1
	if v, ok := config["count"]; ok {
		if c := toUint16FromAny(v); c > 0 {
			cfg.Count = c
		}
	}
	if cfg.Count == 0 {
		return ErrModbusInvalidCount
	}

	// data_type (선택, 기본값 "uint16")
	cfg.DataType = defaultDataType
	if v, ok := config["data_type"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.DataType = s
		}
	}

	// byte_order (선택, 기본값 "big_endian")
	cfg.ByteOrder = defaultByteOrder
	if v, ok := config["byte_order"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.ByteOrder = s
		}
	}

	// device_id (선택, 기본값 1) - Client Agent 전용
	cfg.DeviceID = 1
	if v, ok := config["device_id"]; ok {
		if d := toByte(v); d > 0 {
			cfg.DeviceID = d
		}
	}

	// timeout (선택, 기본값 "5s")
	n.timeout = defaultTimeout
	if v, ok := config["timeout"]; ok {
		if s, ok := v.(string); ok && s != "" {
			if d, err := time.ParseDuration(s); err == nil {
				n.timeout = d
			}
		}
	}
	cfg.Timeout = n.timeout.String()

	// 원자적 설정 적용
	n.mu.Lock()
	n.modbusConfig = cfg
	n.mu.Unlock()

	return nil
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

// Init 은 ModbusNode를 초기화한다.
// AgentResolver를 통해 에이전트를 resolve하고, 타입을 자동 감지한다.
func (n *ModbusNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	// AgentResolver 확인 (NewModbusNode에서 옵션으로 설정됨)
	if n.resolver == nil {
		return ErrModbusNoResolver
	}

	// 에이전트 resolve (R-MBRW-007)
	n.mu.RLock()
	agentRef := n.modbusConfig.AgentRef
	n.mu.RUnlock()

	ref := flow.AgentRef{
		AgentName: agentRef,
	}
	transport, err := n.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("modbus init: agent resolve failed: %w", err)
	}
	n.transport = transport

	// AgentAccessor를 통해 원본 Agent 객체 획득 및 타입 감지 (R-MBRW-008)
	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return ErrModbusAgentNotMODBUS
	}

	underlyingAgent := accessor.UnderlyingAgent()
	switch underlyingAgent.(type) {
	case *modbusserver.ModbusServerAgent:
		n.agentType = agentTypeServer
		n.agent = underlyingAgent
	case *modbus.ModbusAgent:
		n.agentType = agentTypeClient
		n.agent = underlyingAgent
	default:
		// 비-MODBUS Agent (R-MBRW-009)
		return ErrModbusAgentNotMODBUS
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// ---------------------------------------------------------------------------
// Process
// ---------------------------------------------------------------------------

// Process 는 MODBUS 에이전트에 레지스터 읽기/쓰기 명령을 전달하고 결과를 반환한다.
func (n *ModbusNode) Process(ctx context.Context, msg message.Message) (result []message.Message, retErr error) {
	// 패닉 방지 (R-MBRW-032)
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("modbus: panic recovered: %v", r)
		}
	}()

	n.mu.RLock()
	cfg := n.modbusConfig
	n.mu.RUnlock()

	// 입력 메시지 payload로 설정값 오버라이드
	cfg = applyMessageOverrides(msg, cfg)

	switch cfg.Operation {
	case "read":
		return n.processRead(ctx, msg, cfg)
	case "write":
		return n.processWrite(ctx, msg, cfg)
	default:
		return nil, ErrModbusInvalidOperation
	}
}

// ---------------------------------------------------------------------------
// 읽기 연산
// ---------------------------------------------------------------------------

// processRead 는 읽기 연산을 수행한다.
func (n *ModbusNode) processRead(ctx context.Context, msg message.Message, cfg ModbusConfig) ([]message.Message, error) {
	var cmdBytes []byte
	var err error

	switch n.agentType {
	case agentTypeServer:
		cmdBytes, err = n.buildServerReadCommand(cfg)
	case agentTypeClient:
		cmdBytes, err = n.buildClientReadCommand(msg, cfg)
	default:
		return nil, ErrModbusAgentNotMODBUS
	}
	if err != nil {
		return nil, fmt.Errorf("modbus read: command build failed: %w", err)
	}

	// Agent Process() 호출 (R-MBRW-010, R-MBRW-012)
	respBytes, err := n.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrModbusProcessFailed, err)
	}

	// 응답 파싱
	var resp map[string]any
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("modbus read: response parse failed: %w", err)
	}

	// 결과 추출
	result := extractReadResult(resp, n.agentType, cfg)

	// 출력 메시지 구성 (R-MBRW-015, R-MBRW-017: 원본 payload 보존)
	outMsg := msg.Clone()
	outMsg.Payload().Set("result", result)
	outMsg.Payload().Set("register_area", cfg.RegisterArea)
	outMsg.Payload().Set("address", cfg.Address)
	outMsg.Payload().Set("count", cfg.Count)
	outMsg.Payload().Set("data_type", cfg.DataType)
	outMsg.Payload().Set("agent_type", n.agentType)
	outMsg.Metadata().Set("message_type", "response")

	return []message.Message{outMsg}, nil
}

// buildServerReadCommand 는 Server Agent 읽기 명령 JSON을 생성한다.
func (n *ModbusNode) buildServerReadCommand(cfg ModbusConfig) ([]byte, error) {
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
		// 타입 변환 읽기 (R-MBRW-016)
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

// buildClientReadCommand 는 Client Agent 읽기 명령 JSON을 생성한다.
// read_raw 명령을 사용하여 개별 주소 읽기를 지원한다.
// device_id는 applyMessageOverrides에서 이미 오버라이드 처리됨.
func (n *ModbusNode) buildClientReadCommand(msg message.Message, cfg ModbusConfig) ([]byte, error) {
	deviceID := fmt.Sprintf("%d", cfg.DeviceID)

	// function_code 결정
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
// 쓰기 연산
// ---------------------------------------------------------------------------

// processWrite 는 쓰기 연산을 수행한다.
func (n *ModbusNode) processWrite(ctx context.Context, msg message.Message, cfg ModbusConfig) ([]message.Message, error) {
	// 읽기 전용 영역 쓰기 차단 (R-MBRW-034)
	if readOnlyAreas[cfg.RegisterArea] {
		return nil, ErrModbusReadOnlyArea
	}

	// 쓰기 값 추출 (R-MBRW-023, R-MBRW-024)
	value, hasValue := msg.Payload().Get("value")
	values, hasValues := msg.Payload().Get("values")

	if !hasValue && !hasValues {
		return nil, ErrModbusWriteValueMissing
	}

	var cmdBytes []byte
	var err error

	switch n.agentType {
	case agentTypeServer:
		cmdBytes, err = n.buildServerWriteCommand(cfg, value, hasValue, values, hasValues)
	case agentTypeClient:
		cmdBytes, err = n.buildClientWriteCommand(msg, cfg, value, hasValue, values, hasValues)
	default:
		return nil, ErrModbusAgentNotMODBUS
	}
	if err != nil {
		return nil, fmt.Errorf("modbus write: command build failed: %w", err)
	}

	// Agent Process() 호출 (R-MBRW-010, R-MBRW-012)
	_, err = n.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrModbusProcessFailed, err)
	}

	// 출력 메시지 구성 (R-MBRW-027, R-MBRW-028: 원본 payload 보존)
	outMsg := msg.Clone()
	outMsg.Payload().Set("success", true)
	outMsg.Payload().Set("register_area", cfg.RegisterArea)
	outMsg.Payload().Set("address", cfg.Address)
	outMsg.Payload().Set("count", cfg.Count)
	outMsg.Payload().Set("data_type", cfg.DataType)
	outMsg.Payload().Set("byte_order", cfg.ByteOrder)
	outMsg.Payload().Set("agent_type", n.agentType)
	outMsg.Metadata().Set("message_type", "response")

	return []message.Message{outMsg}, nil
}

// buildServerWriteCommand 는 Server Agent 쓰기 명령 JSON을 생성한다.
func (n *ModbusNode) buildServerWriteCommand(cfg ModbusConfig, value any, hasValue bool, values any, hasValues bool) ([]byte, error) {
	params := map[string]any{
		"address": cfg.Address,
	}

	var command string

	switch cfg.RegisterArea {
	case areaCoils:
		if cfg.Count > 1 && hasValues {
			// 다중 코일 쓰기 (R-MBRW-021)
			command = "set_coils"
			params["values"] = values
		} else {
			// 단일 코일 쓰기 (R-MBRW-021)
			command = "set_coil"
			params["value"] = value
		}

	case areaHoldingRegisters:
		// data_type/byte_order 전달 (R-MBRW-026)
		if !booleanAreas[cfg.RegisterArea] && cfg.DataType != "" {
			params["data_type"] = cfg.DataType
			params["byte_order"] = cfg.ByteOrder
		}

		if cfg.Count > 1 && hasValues {
			// 다중 레지스터 쓰기 (R-MBRW-021)
			command = "set_registers"
			params["values"] = values
		} else {
			// 단일 레지스터 쓰기 (R-MBRW-021)
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

// buildClientWriteCommand 는 Client Agent 쓰기 명령 JSON을 생성한다.
// device_id는 applyMessageOverrides에서 이미 오버라이드 처리됨.
func (n *ModbusNode) buildClientWriteCommand(msg message.Message, cfg ModbusConfig, value any, hasValue bool, values any, hasValues bool) ([]byte, error) {
	deviceID := fmt.Sprintf("%d", cfg.DeviceID)

	params := map[string]any{
		"address": cfg.Address,
	}

	var command string

	switch cfg.RegisterArea {
	case areaCoils:
		if cfg.Count > 1 && hasValues {
			// 다중 코일 쓰기 (R-MBRW-022)
			command = "write_coils"
			params["values"] = values
		} else {
			// 단일 코일 쓰기 (R-MBRW-022)
			command = "write_coil"
			params["value"] = value
		}

	case areaHoldingRegisters:
		// data_type/byte_order 전달
		if !booleanAreas[cfg.RegisterArea] && cfg.DataType != "" {
			params["data_type"] = cfg.DataType
			params["byte_order"] = cfg.ByteOrder
		}

		if cfg.Count > 1 && hasValues {
			// 다중 레지스터 쓰기 (R-MBRW-022)
			command = "write_registers"
			params["values"] = values
		} else {
			// 단일 레지스터 쓰기 (R-MBRW-022)
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
// Agent Process 호출
// ---------------------------------------------------------------------------

// callAgentProcess 는 Agent.Process()를 context timeout과 함께 호출한다.
func (n *ModbusNode) callAgentProcess(ctx context.Context, cmdBytes []byte) ([]byte, error) {
	if n.agent == nil {
		return nil, ErrModbusNoResolver
	}

	// context timeout 설정 (R-MBRW-012)
	timeoutCtx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()

	// 채널 기반 timeout 처리
	type processResult struct {
		data []byte
		err  error
	}
	ch := make(chan processResult, 1)

	go func() {
		data, err := n.agent.Process(cmdBytes)
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
// Shutdown
// ---------------------------------------------------------------------------

// Shutdown 은 ModbusNode를 종료한다.
func (n *ModbusNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// ---------------------------------------------------------------------------
// 유틸리티 함수
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
