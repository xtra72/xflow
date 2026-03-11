package node

import (
	"context"
	"encoding/json"
	"fmt"
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
	// ErrModbusRWAgentNotMODBUS 는 resolve된 Agent가 MODBUS 타입이 아닐 때 반환된다.
	ErrModbusRWAgentNotMODBUS = fmt.Errorf("modbus_rw: %w: agent is not a MODBUS type", ErrInvalidConfig)

	// ErrModbusRWMissingAgentRef 는 agent_ref 설정이 없을 때 반환된다.
	ErrModbusRWMissingAgentRef = fmt.Errorf("modbus_rw: %w: agent_ref is required", ErrInvalidConfig)

	// ErrModbusRWInvalidOperation 은 operation이 read/write가 아닐 때 반환된다.
	ErrModbusRWInvalidOperation = fmt.Errorf("modbus_rw: %w: operation must be \"read\" or \"write\"", ErrInvalidConfig)

	// ErrModbusRWInvalidRegisterArea 는 지원하지 않는 register_area 값일 때 반환된다.
	ErrModbusRWInvalidRegisterArea = fmt.Errorf("modbus_rw: %w: unsupported register_area", ErrInvalidConfig)

	// ErrModbusRWReadOnlyArea 는 읽기 전용 영역에 쓰기를 시도할 때 반환된다.
	ErrModbusRWReadOnlyArea = fmt.Errorf("modbus_rw: %w: cannot write to read-only register area", ErrInvalidConfig)

	// ErrModbusRWInvalidCount 는 count가 0 이하일 때 반환된다.
	ErrModbusRWInvalidCount = fmt.Errorf("modbus_rw: %w: count must be positive", ErrInvalidConfig)

	// ErrModbusRWNoResolver 는 AgentResolver가 설정되지 않았을 때 반환된다.
	ErrModbusRWNoResolver = fmt.Errorf("modbus_rw: %w: agent resolver not configured", ErrNodeNotInitialized)

	// ErrModbusRWWriteValueMissing 는 쓰기 연산 시 value/values가 없을 때 반환된다.
	ErrModbusRWWriteValueMissing = fmt.Errorf("modbus_rw: write value missing: payload must contain \"value\" or \"values\" key")

	// ErrModbusRWProcessFailed 는 Agent Process() 호출이 실패했을 때 반환된다.
	ErrModbusRWProcessFailed = fmt.Errorf("modbus_rw: agent process failed")
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
// ModbusRWConfig 구조체
// ---------------------------------------------------------------------------

// ModbusRWConfig 는 ModbusRWNode의 설정 구조체이다.
type ModbusRWConfig struct {
	AgentRef     string `json:"agent_ref"`     // 대상 Agent 이름/ID
	Operation    string `json:"operation"`      // "read" | "write"
	RegisterArea string `json:"register_area"`  // "coils" | "discrete_inputs" | "holding_registers" | "input_registers"
	Address      uint16 `json:"address"`        // 시작 레지스터 주소
	Count        uint16 `json:"count"`          // 레지스터 수 (기본값 1)
	DataType     string `json:"data_type"`      // "uint16" | "int16" | "float32" | "uint32" | "int32"
	ByteOrder    string `json:"byte_order"`     // "big_endian" | "little_endian"
	DeviceID     uint8  `json:"device_id"`      // Client Agent 전용 (기본값 1)
	Timeout      string `json:"timeout"`        // Process 호출 타임아웃 (기본값 "5s")
}

// ---------------------------------------------------------------------------
// ModbusRWNode 구조체
// ---------------------------------------------------------------------------

// ModbusRWNode 는 MODBUS 에이전트의 레지스터를 읽기/쓰기하는 처리 노드이다.
// Server Agent와 Client Agent를 자동 감지하여 적절한 Process() 명령을 전송한다.
type ModbusRWNode struct {
	*BaseNode
	rwConfig  ModbusRWConfig
	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent   // 원본 Agent 객체
	agentType string        // "server" | "client"
	timeout   time.Duration // Process 호출 타임아웃
	mu        sync.RWMutex  // 설정 보호 뮤텍스
}

// 인터페이스 컴파일 체크
var _ Node = (*ModbusRWNode)(nil)

// ---------------------------------------------------------------------------
// 팩토리 함수
// ---------------------------------------------------------------------------

// NewModbusRWNode 는 새로운 ModbusRWNode를 생성하는 팩토리 함수이다.
func NewModbusRWNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &ModbusRWNode{
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

// Configure 는 ModbusRWNode의 설정을 적용한다.
// agent_ref(필수), operation(필수), register_area(필수), address(필수)를 검증한다.
func (n *ModbusRWNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg ModbusRWConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrModbusRWMissingAgentRef
	}

	// operation (필수)
	if v, ok := config["operation"]; ok {
		if s, ok := v.(string); ok {
			cfg.Operation = s
		}
	}
	if cfg.Operation != "read" && cfg.Operation != "write" {
		return ErrModbusRWInvalidOperation
	}

	// register_area (필수)
	if v, ok := config["register_area"]; ok {
		if s, ok := v.(string); ok {
			cfg.RegisterArea = s
		}
	}
	if !validRegisterAreas[cfg.RegisterArea] {
		return ErrModbusRWInvalidRegisterArea
	}

	// 읽기 전용 영역에 쓰기 시도 검증 (R-MBRW-004)
	if cfg.Operation == "write" && readOnlyAreas[cfg.RegisterArea] {
		return ErrModbusRWReadOnlyArea
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
		return ErrModbusRWInvalidCount
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
	n.rwConfig = cfg
	n.mu.Unlock()

	return nil
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

// Init 은 ModbusRWNode를 초기화한다.
// AgentResolver를 통해 에이전트를 resolve하고, 타입을 자동 감지한다.
func (n *ModbusRWNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	// AgentResolver 확인 (NewModbusRWNode에서 옵션으로 설정됨)
	if n.resolver == nil {
		return ErrModbusRWNoResolver
	}

	// 에이전트 resolve (R-MBRW-007)
	n.mu.RLock()
	agentRef := n.rwConfig.AgentRef
	n.mu.RUnlock()

	ref := flow.AgentRef{
		AgentName: agentRef,
	}
	transport, err := n.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("modbus_rw init: agent resolve failed: %w", err)
	}
	n.transport = transport

	// AgentAccessor를 통해 원본 Agent 객체 획득 및 타입 감지 (R-MBRW-008)
	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return ErrModbusRWAgentNotMODBUS
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
		return ErrModbusRWAgentNotMODBUS
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// ---------------------------------------------------------------------------
// Process
// ---------------------------------------------------------------------------

// Process 는 MODBUS 에이전트에 레지스터 읽기/쓰기 명령을 전달하고 결과를 반환한다.
func (n *ModbusRWNode) Process(ctx context.Context, msg message.Message) (result []message.Message, retErr error) {
	// 패닉 방지 (R-MBRW-032)
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("modbus_rw: panic recovered: %v", r)
		}
	}()

	n.mu.RLock()
	cfg := n.rwConfig
	n.mu.RUnlock()

	switch cfg.Operation {
	case "read":
		return n.processRead(ctx, msg, cfg)
	case "write":
		return n.processWrite(ctx, msg, cfg)
	default:
		return nil, ErrModbusRWInvalidOperation
	}
}

// ---------------------------------------------------------------------------
// 읽기 연산
// ---------------------------------------------------------------------------

// processRead 는 읽기 연산을 수행한다.
func (n *ModbusRWNode) processRead(ctx context.Context, msg message.Message, cfg ModbusRWConfig) ([]message.Message, error) {
	var cmdBytes []byte
	var err error

	switch n.agentType {
	case agentTypeServer:
		cmdBytes, err = n.buildServerReadCommand(cfg)
	case agentTypeClient:
		cmdBytes, err = n.buildClientReadCommand(msg, cfg)
	default:
		return nil, ErrModbusRWAgentNotMODBUS
	}
	if err != nil {
		return nil, fmt.Errorf("modbus_rw read: command build failed: %w", err)
	}

	// Agent Process() 호출 (R-MBRW-010, R-MBRW-012)
	respBytes, err := n.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrModbusRWProcessFailed, err)
	}

	// 응답 파싱
	var resp map[string]any
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("modbus_rw read: response parse failed: %w", err)
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

	return []message.Message{outMsg}, nil
}

// buildServerReadCommand 는 Server Agent 읽기 명령 JSON을 생성한다.
func (n *ModbusRWNode) buildServerReadCommand(cfg ModbusRWConfig) ([]byte, error) {
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
			params["data_type"] = cfg.DataType
			params["byte_order"] = cfg.ByteOrder
		} else {
			command = "get_holding_registers"
			params["quantity"] = cfg.Count
		}
	case areaInputRegisters:
		if cfg.DataType != defaultDataType && !booleanAreas[cfg.RegisterArea] {
			command = "get_register_typed"
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
func (n *ModbusRWNode) buildClientReadCommand(msg message.Message, cfg ModbusRWConfig) ([]byte, error) {
	// device_id 결정: 메시지 payload 오버라이드 (R-MBRW-020) > 노드 설정
	deviceID := fmt.Sprintf("%d", cfg.DeviceID)
	if v, ok := msg.Payload().Get("device_id"); ok {
		if s, ok := v.(string); ok && s != "" {
			deviceID = s
		} else {
			// 숫자 타입도 지원
			deviceID = fmt.Sprintf("%v", v)
		}
	}

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
func extractReadResult(resp map[string]any, agentType string, cfg ModbusRWConfig) any {
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
func (n *ModbusRWNode) processWrite(ctx context.Context, msg message.Message, cfg ModbusRWConfig) ([]message.Message, error) {
	// 읽기 전용 영역 쓰기 차단 (R-MBRW-034)
	if readOnlyAreas[cfg.RegisterArea] {
		return nil, ErrModbusRWReadOnlyArea
	}

	// 쓰기 값 추출 (R-MBRW-023, R-MBRW-024)
	value, hasValue := msg.Payload().Get("value")
	values, hasValues := msg.Payload().Get("values")

	if !hasValue && !hasValues {
		return nil, ErrModbusRWWriteValueMissing
	}

	var cmdBytes []byte
	var err error

	switch n.agentType {
	case agentTypeServer:
		cmdBytes, err = n.buildServerWriteCommand(cfg, value, hasValue, values, hasValues)
	case agentTypeClient:
		cmdBytes, err = n.buildClientWriteCommand(msg, cfg, value, hasValue, values, hasValues)
	default:
		return nil, ErrModbusRWAgentNotMODBUS
	}
	if err != nil {
		return nil, fmt.Errorf("modbus_rw write: command build failed: %w", err)
	}

	// Agent Process() 호출 (R-MBRW-010, R-MBRW-012)
	_, err = n.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrModbusRWProcessFailed, err)
	}

	// 출력 메시지 구성 (R-MBRW-027, R-MBRW-028: 원본 payload 보존)
	outMsg := msg.Clone()
	outMsg.Payload().Set("success", true)
	outMsg.Payload().Set("register_area", cfg.RegisterArea)
	outMsg.Payload().Set("address", cfg.Address)
	outMsg.Payload().Set("count", cfg.Count)
	outMsg.Payload().Set("agent_type", n.agentType)

	return []message.Message{outMsg}, nil
}

// buildServerWriteCommand 는 Server Agent 쓰기 명령 JSON을 생성한다.
func (n *ModbusRWNode) buildServerWriteCommand(cfg ModbusRWConfig, value any, hasValue bool, values any, hasValues bool) ([]byte, error) {
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
func (n *ModbusRWNode) buildClientWriteCommand(msg message.Message, cfg ModbusRWConfig, value any, hasValue bool, values any, hasValues bool) ([]byte, error) {
	// device_id 결정: 메시지 payload 오버라이드 > 노드 설정
	deviceID := fmt.Sprintf("%d", cfg.DeviceID)
	if v, ok := msg.Payload().Get("device_id"); ok {
		if s, ok := v.(string); ok && s != "" {
			deviceID = s
		} else {
			deviceID = fmt.Sprintf("%v", v)
		}
	}

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
func (n *ModbusRWNode) callAgentProcess(ctx context.Context, cmdBytes []byte) ([]byte, error) {
	if n.agent == nil {
		return nil, ErrModbusRWNoResolver
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
		return nil, fmt.Errorf("modbus_rw: %w", timeoutCtx.Err())
	case result := <-ch:
		return result.data, result.err
	}
}

// ---------------------------------------------------------------------------
// Shutdown
// ---------------------------------------------------------------------------

// Shutdown 은 ModbusRWNode를 종료한다.
func (n *ModbusRWNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// ---------------------------------------------------------------------------
// 유틸리티 함수
// ---------------------------------------------------------------------------

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
