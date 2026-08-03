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
// ModbusReadNode - command-set 기반 MODBUS 읽기 노드
// ---------------------------------------------------------------------------
// config `command_set`(op 배열)을 기본값으로 파싱하고, 입력 payload에 command_set이
// 있으면 이번 호출에 한해 오버라이드한다. 각 op를 순서대로 읽고 결과 1건을 emit한다.
//
// op 스키마: {area, address, count, data_type?, byte_order?, unit_id?}
//   - Server: get_coils/get_discrete_inputs/get_holding_registers/get_input_registers,
//             data_type이 uint16이 아니면 get_register_typed(typed 읽기).
//   - Client: read_raw(areaToFunctionCode 기반, 원시 바이트).
//   - unit_id: 0 은 공유 컨테이너를 대상으로 한다(Server).

// ModbusReadNode 는 MODBUS 에이전트에서 command-set 기반 읽기를 수행하는 노드이다.
type ModbusReadNode struct {
	modbusNodeBase
	agentRef   string           // 대상 에이전트 이름/ID
	commandSet []map[string]any // config 기본 command_set(op 배열)
}

// 인터페이스 컴파일 체크
var (
	_ Node               = (*ModbusReadNode)(nil)
	_ AgentReinitializer = (*ModbusReadNode)(nil)
)

// ---------------------------------------------------------------------------
// 팩토리
// ---------------------------------------------------------------------------

// NewModbusReadNode 는 새로운 ModbusReadNode를 생성한다.
func NewModbusReadNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &ModbusReadNode{modbusNodeBase: modbusNodeBase{BaseNode: base}}
	n.initModbusResolver()
	return n, nil
}

// ---------------------------------------------------------------------------
// Configure
// ---------------------------------------------------------------------------

// Configure 는 ModbusReadNode의 설정을 적용한다.
// agent_ref(필수), command_set(선택, 배열), timeout(선택)을 파싱한다.
func (n *ModbusReadNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	var agentRef string
	if s, ok := config["agent_ref"].(string); ok {
		agentRef = s
	}
	if agentRef == "" {
		return ErrModbusMissingAgentRef
	}

	ops := toOpList(config["command_set"])

	timeout := defaultTimeout
	if s, ok := config["timeout"].(string); ok && s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			timeout = d
		}
	}

	n.mu.Lock()
	n.agentRef = agentRef
	n.commandSet = ops
	n.timeout = timeout
	n.mu.Unlock()

	return nil
}

// ---------------------------------------------------------------------------
// Init / Reinit / Shutdown
// ---------------------------------------------------------------------------

// Init 은 에이전트를 best-effort로 resolve한다(실패 시 경고 후 Running).
func (n *ModbusReadNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()

	if err := n.resolveModbusAgent(ctx, ref); err != nil {
		if logger := n.Logger(); logger != nil {
			logger.Warn("modbus-read init: agent resolve deferred",
				"nodeID", n.ID(), "agentRef", ref, "error", err)
		}
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다(AgentReinitializer).
func (n *ModbusReadNode) AgentRef() flow.AgentRef {
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// Reinit 은 에이전트 재시작 후 agent/transport/agentType을 재해석한다.
func (n *ModbusReadNode) Reinit(ctx context.Context) error {
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()
	return n.resolveModbusAgent(ctx, ref)
}

// Shutdown 은 노드를 종료한다.
func (n *ModbusReadNode) Shutdown(_ context.Context) error {
	return n.modbusShutdown()
}

// ---------------------------------------------------------------------------
// Process
// ---------------------------------------------------------------------------

// Process 는 command_set의 각 읽기 op를 순서대로 실행하고 결과 1건을 emit한다.
func (n *ModbusReadNode) Process(ctx context.Context, msg message.Message) (result []message.Message, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("modbus-read: panic recovered: %v", r)
		}
	}()

	n.mu.RLock()
	ops := n.commandSet
	n.mu.RUnlock()
	if msg.Payload() != nil {
		if v, ok := msg.Payload().Get("command_set"); ok {
			if override := toOpList(v); len(override) > 0 {
				ops = override
			}
		}
	}
	if len(ops) == 0 {
		return nil, ErrModbusEmptyCommandSet
	}

	if n.underlyingModbusAgent() == nil {
		return nil, ErrModbusNoResolver
	}
	agentType := n.resolvedAgentType()

	values := make([]map[string]any, 0, len(ops))
	overall := true
	for i, op := range ops {
		entry, hardErr := n.applyReadOp(ctx, agentType, i, op)
		if hardErr != nil {
			return nil, hardErr
		}
		if _, failed := entry["error"]; failed {
			overall = false
		}
		values = append(values, entry)
	}

	out := msg.Clone()
	out.Payload().Set("success", overall)
	out.Payload().Set("values", values)
	out.Payload().Set("agent_type", agentType)
	out.SetType("response")
	return []message.Message{out}, nil
}

// applyReadOp 은 단일 읽기 op를 실행하고 결과 엔트리를 반환한다.
// 반환: (entry, hardErr). hardErr != nil 이면 즉시 중단해야 한다.
func (n *ModbusReadNode) applyReadOp(ctx context.Context, agentType string, index int, op map[string]any) (map[string]any, error) {
	r := parseReadOp(op)

	entry := map[string]any{
		"index":   index,
		"area":    r.area,
		"address": r.address,
		"count":   r.count,
	}

	if !validRegisterAreas[r.area] {
		entry["error"] = "invalid register area: " + r.area
		return entry, nil
	}

	cmdBytes, buildErr := n.buildReadCommand(agentType, r)
	if buildErr != nil {
		entry["error"] = buildErr.Error()
		return entry, nil
	}

	respBytes, err := n.callModbusAgentProcess(ctx, cmdBytes)
	if err != nil {
		if isHardModbusError(err) {
			return nil, err
		}
		entry["error"] = err.Error()
		return entry, nil
	}

	var resp map[string]any
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		entry["error"] = "invalid agent response: " + err.Error()
		return entry, nil
	}
	// 에이전트가 명시적 error를 반환하면 op 실패로 기록한다.
	if e, has := resp["error"]; has {
		entry["error"] = toReasonString(e)
		return entry, nil
	}

	extracted := extractReadResult(resp, agentType, r.cfg())

	// typed 읽기(Server, data_type != uint16/raw) 는 "values"+data_type,
	// 그 외(raw / Client read_raw) 는 "raw" 로 담는다.
	typed := agentType == agentTypeServer &&
		r.dataType != "" && r.dataType != "raw" && r.dataType != defaultDataType
	if typed {
		entry["values"] = extracted
		entry["data_type"] = r.dataType
	} else {
		entry["raw"] = extracted
	}
	return entry, nil
}

// buildReadCommand 는 agent 타입과 op에 맞는 읽기 명령 JSON을 생성한다.
func (n *ModbusReadNode) buildReadCommand(agentType string, r modbusReadOp) ([]byte, error) {
	cfg := r.cfg()
	switch agentType {
	case agentTypeServer:
		b, err := buildModbusServerReadCommand(cfg)
		if err != nil {
			return nil, err
		}
		if r.hasUnitID {
			return injectServerUnitID(b, r.unitID)
		}
		return b, nil
	case agentTypeClient:
		return buildModbusClientReadCommand(cfg)
	}
	return nil, fmt.Errorf("modbus-read: unresolved agent type")
}

// ---------------------------------------------------------------------------
// op 파싱
// ---------------------------------------------------------------------------

// modbusReadOp 는 파싱된 단일 읽기 op이다.
type modbusReadOp struct {
	area      string
	address   uint16
	count     uint16
	dataType  string
	byteOrder string
	unitID    uint8
	hasUnitID bool
}

// parseReadOp 은 op 맵을 modbusReadOp로 파싱한다.
func parseReadOp(op map[string]any) modbusReadOp {
	r := modbusReadOp{
		dataType:  defaultDataType,
		byteOrder: defaultByteOrder,
	}
	if s, ok := op["area"].(string); ok {
		r.area = s
	}
	r.address = toUint16FromAny(op["address"])
	r.count = toUint16FromAny(op["count"])
	if r.count == 0 {
		r.count = 1
	}
	if s, ok := op["data_type"].(string); ok && s != "" {
		r.dataType = s
	}
	if s, ok := op["byte_order"].(string); ok && s != "" {
		r.byteOrder = s
	}
	if v, ok := op["unit_id"]; ok {
		r.unitID = toByte(v)
		r.hasUnitID = true
	}
	return r
}

// cfg 는 읽기 op에서 살베지 빌더용 ModbusConfig를 구성한다.
func (r modbusReadOp) cfg() ModbusConfig {
	cfg := ModbusConfig{
		RegisterArea: r.area,
		Address:      r.address,
		Count:        r.count,
		DataType:     r.dataType,
		ByteOrder:    r.byteOrder,
	}
	if r.hasUnitID {
		cfg.DeviceID = r.unitID
	} else {
		cfg.DeviceID = 1 // Client 기본 device_id
	}
	return cfg
}
