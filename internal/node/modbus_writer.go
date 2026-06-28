package node

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 센티널 에러 정의
// ---------------------------------------------------------------------------

var (
	ErrModbusWriterMissingAgentRef     = fmt.Errorf("modbus-writer: %w: agent_ref is required", ErrInvalidConfig)
	ErrModbusWriterInvalidRegisterArea = fmt.Errorf("modbus-writer: %w: unsupported or read-only register_area", ErrInvalidConfig)
)

// ---------------------------------------------------------------------------
// ModbusWriterConfig
// ---------------------------------------------------------------------------

// ModbusWriterConfig 는 ModbusWriterNode 전용 설정 구조체이다.
type ModbusWriterConfig struct {
	AgentRef     string `json:"agent_ref"`
	RegisterArea string `json:"register_area"`
	Address      uint16 `json:"address"`
	Count        uint16 `json:"count"`
	DataType     string `json:"data_type"`
	ByteOrder    string `json:"byte_order"`
	DeviceID     uint8  `json:"device_id"`
	Timeout      string `json:"timeout"`

	// EmitMetadata 는 metadata 그룹 emit 정책을 제어한다 (P3).
	// modbus-writer 는 디바이스 노드가 아니므로 Agent(에이전트 그룹)만 사용한다.
	// parseEmitMetadata 가 Agent 기본 ON — emit_agent:false 로 비활성화.
	EmitMetadata MetadataEmitOptions `json:"emit_metadata"`
}

// ---------------------------------------------------------------------------
// ModbusWriterNode
// ---------------------------------------------------------------------------

// ModbusWriterNode 는 MODBUS 에이전트 레지스터에 값을 쓰는 전용 ProcessNode이다.
// 쓰기 가능 영역(coils, holding_registers)만 허용한다.
// 입력 메시지의 payload에서 value/values를 추출하여 레지스터에 쓴다.
type ModbusWriterNode struct {
	modbusNodeBase
	writerConfig ModbusWriterConfig
}

// 인터페이스 컴파일 체크
var _ Node = (*ModbusWriterNode)(nil)

// NewModbusWriterNode 는 새로운 ModbusWriterNode를 생성하는 팩토리 함수이다.
func NewModbusWriterNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &ModbusWriterNode{
		modbusNodeBase: modbusNodeBase{
			BaseNode: base,
		},
	}
	n.initModbusResolver()
	return n, nil
}

// ---------------------------------------------------------------------------
// Configure
// ---------------------------------------------------------------------------

// Configure 는 ModbusWriterNode의 설정을 적용한다.
// register_area는 쓰기 가능 영역(coils, holding_registers)만 허용한다.
func (n *ModbusWriterNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg ModbusWriterConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrModbusWriterMissingAgentRef
	}

	// register_area (선택, 기본값 holding_registers, 쓰기 가능만)
	cfg.RegisterArea = areaHoldingRegisters
	if v, ok := config["register_area"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.RegisterArea = s
		}
	}
	// 유효성 + 쓰기 가능 검증
	if !validRegisterAreas[cfg.RegisterArea] || readOnlyAreas[cfg.RegisterArea] {
		return ErrModbusWriterInvalidRegisterArea
	}

	// address (선택, 기본값 0)
	cfg.Address = toUint16FromAny(config["address"])

	// count (선택, 기본값 1)
	cfg.Count = 1
	if v, ok := config["count"]; ok {
		if c := toUint16FromAny(v); c > 0 {
			cfg.Count = c
		}
	}

	// data_type (선택, 기본값 uint16)
	cfg.DataType = defaultDataType
	if v, ok := config["data_type"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.DataType = s
		}
	}

	// byte_order (선택, 기본값 big_endian)
	cfg.ByteOrder = defaultByteOrder
	if v, ok := config["byte_order"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.ByteOrder = s
		}
	}

	// device_id (선택, 기본값 1)
	cfg.DeviceID = 1
	if v, ok := config["device_id"]; ok {
		if d := toByte(v); d > 0 {
			cfg.DeviceID = d
		}
	}

	// timeout (선택, 기본값 5s)
	n.timeout = defaultTimeout
	if v, ok := config["timeout"]; ok {
		if s, ok := v.(string); ok && s != "" {
			if d, err := time.ParseDuration(s); err == nil {
				n.timeout = d
			}
		}
	}
	cfg.Timeout = n.timeout.String()

	// P3: emit_metadata — agent 그룹 emit 정책 (기본 ON).
	parseEmitMetadata(config, &cfg.EmitMetadata)

	n.mu.Lock()
	n.writerConfig = cfg
	n.mu.Unlock()

	return nil
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

// Init 은 ModbusWriterNode를 초기화한다.
func (n *ModbusWriterNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	n.mu.RLock()
	agentRef := n.writerConfig.AgentRef
	n.mu.RUnlock()

	if err := n.resolveModbusAgent(ctx, agentRef); err != nil {
		return err
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// ---------------------------------------------------------------------------
// Process
// ---------------------------------------------------------------------------

// Process 는 입력 메시지에서 value/values를 추출하여 레지스터에 쓴다.
// 메시지 payload로 address, count, data_type 등을 오버라이드할 수 있다.
func (n *ModbusWriterNode) Process(ctx context.Context, msg message.Message) (result []message.Message, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("modbus-writer: panic recovered: %v", r)
		}
	}()

	n.mu.RLock()
	wcfg := n.writerConfig
	n.mu.RUnlock()

	// ModbusConfig로 변환하여 applyMessageOverrides 재사용
	cfg := ModbusConfig{
		AgentRef:     wcfg.AgentRef,
		Operation:    "write",
		RegisterArea: wcfg.RegisterArea,
		Address:      wcfg.Address,
		Count:        wcfg.Count,
		DataType:     wcfg.DataType,
		ByteOrder:    wcfg.ByteOrder,
		DeviceID:     wcfg.DeviceID,
		EmitMetadata: wcfg.EmitMetadata,
	}
	cfg = applyMessageOverrides(msg, cfg)

	// 읽기 전용 영역 쓰기 차단 (오버라이드 후 재검증)
	if readOnlyAreas[cfg.RegisterArea] {
		return nil, ErrModbusReadOnlyArea
	}

	// 쓰기 값 추출 (value/values 없으면 스킵 — strip_nulls 호환)
	value, hasValue := msg.Payload().Get("value")
	values, hasValues := msg.Payload().Get("values")
	if !hasValue && !hasValues {
		return nil, nil // 값 없으면 조용히 스킵
	}

	// 쓰기 명령 생성
	var cmdBytes []byte
	var err error

	switch n.agentType {
	case agentTypeServer:
		cmdBytes, err = buildModbusServerWriteCommand(cfg, value, hasValue, values, hasValues)
	case agentTypeClient:
		cmdBytes, err = buildModbusClientWriteCommand(cfg, value, hasValue, values, hasValues)
	default:
		return nil, ErrModbusAgentNotMODBUS
	}
	if err != nil {
		return nil, fmt.Errorf("modbus-writer: command build failed: %w", err)
	}

	// Agent Process() 호출
	_, err = n.callModbusAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrModbusProcessFailed, err)
	}

	// 출력 메시지 (passthrough + 쓰기 결과)
	outMsg := msg.Clone()
	outMsg.Payload().Set("success", true)
	outMsg.Payload().Set("register_area", cfg.RegisterArea)
	outMsg.Payload().Set("address", cfg.Address)
	outMsg.Payload().Set("data_type", cfg.DataType)
	if cfg.ByteOrder != defaultByteOrder {
		outMsg.Payload().Set("byte_order", cfg.ByteOrder)
	}
	outMsg.Payload().Set("agent_type", n.agentType)
	// P3: agent:{type,id} 그룹 (기본 ON). payload.agent_type 는 별개 데이터 필드로 유지.
	emitAgentGroup(outMsg, n.agent, cfg.EmitMetadata)
	outMsg.SetType("response")

	return []message.Message{outMsg}, nil
}

// ---------------------------------------------------------------------------
// AgentRef / Reinit (AgentReinitializer 인터페이스 구현)
// ---------------------------------------------------------------------------

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (n *ModbusWriterNode) AgentRef() flow.AgentRef {
	n.mu.RLock()
	ref := n.writerConfig.AgentRef
	n.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조를 재해석한다.
// process-only 노드이므로 별도 고루틴 재시작이 필요 없다.
func (n *ModbusWriterNode) Reinit(ctx context.Context) error {
	n.mu.RLock()
	agentRef := n.writerConfig.AgentRef
	n.mu.RUnlock()
	return n.resolveModbusAgent(ctx, agentRef)
}

// ---------------------------------------------------------------------------
// Shutdown
// ---------------------------------------------------------------------------

// Shutdown 은 ModbusWriterNode를 종료한다.
func (n *ModbusWriterNode) Shutdown(_ context.Context) error {
	return n.modbusShutdown()
}

// ---------------------------------------------------------------------------
// 쓰기 명령 빌더 (package-level 재사용 가능)
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
