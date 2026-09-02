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
// ModbusWriteNode - command-set 기반 MODBUS 쓰기 노드
// ---------------------------------------------------------------------------
// config `command_set`(op 배열)을 기본값으로 파싱하고, 입력 payload에 command_set이
// 있으면 이번 호출에 한해 오버라이드한다. 각 op를 순서대로 적용하고 결과 1건을 emit한다.
//
// op 스키마: {area, address, value?|values?, data_type?, byte_order?, unit_id?}
//   - Client: coils / holding_registers 만 쓰기 가능(그 외 read-only 영역은 op 실패).
//   - Server: coils / holding_registers 는 set_coil(s)/set_register(s),
//             discrete_inputs / input_registers 는 set_input(s) 로 처리.
//   - unit_id: 0 은 공유 컨테이너를 대상으로 한다(Server).

// ModbusWriteNode 는 MODBUS 에이전트에 command_set 기반 쓰기를 수행하는 노드이다.
type ModbusWriteNode struct {
	modbusNodeBase
	agentRef   string           // 대상 에이전트 이름/ID
	commandSet []map[string]any // config 기본 command_set(op 배열)
}

// 인터페이스 컴파일 체크
var (
	_ Node               = (*ModbusWriteNode)(nil)
	_ AgentReinitializer = (*ModbusWriteNode)(nil)
)

// ---------------------------------------------------------------------------
// 팩토리
// ---------------------------------------------------------------------------

// NewModbusWriteNode 는 새로운 ModbusWriteNode를 생성한다.
func NewModbusWriteNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &ModbusWriteNode{modbusNodeBase: modbusNodeBase{BaseNode: base}}
	n.initModbusResolver()
	return n, nil
}

// ---------------------------------------------------------------------------
// Configure
// ---------------------------------------------------------------------------

// Configure 는 ModbusWriteNode의 설정을 적용한다.
// agent_ref(필수), command_set(선택, 배열), timeout(선택)을 파싱한다.
func (n *ModbusWriteNode) Configure(config map[string]any) error {
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

// Init 은 에이전트를 best-effort로 resolve한다.
// resolve 실패(미해결/비-MODBUS 등)는 경고만 남기고 Running으로 진입한다.
// 이후 에이전트가 준비되면 엔진이 Reinit을 호출해 재해석한다.
func (n *ModbusWriteNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()

	if err := n.resolveModbusAgent(ctx, ref); err != nil {
		if logger := n.Logger(); logger != nil {
			logger.Warn("modbus-write init: agent resolve deferred",
				"nodeID", n.ID(), "agentRef", ref, "error", err)
		}
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다(AgentReinitializer).
func (n *ModbusWriteNode) AgentRef() flow.AgentRef {
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// Reinit 은 에이전트 재시작 후 agent/transport/agentType을 재해석한다.
func (n *ModbusWriteNode) Reinit(ctx context.Context) error {
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()
	return n.resolveModbusAgent(ctx, ref)
}

// Shutdown 은 노드를 종료한다.
func (n *ModbusWriteNode) Shutdown(_ context.Context) error {
	return n.modbusShutdown()
}

// ---------------------------------------------------------------------------
// Process
// ---------------------------------------------------------------------------

// Process 는 command_set의 각 쓰기 op를 순서대로 적용하고 결과 1건을 emit한다.
// payload에 command_set이 있으면 config 기본값을 오버라이드한다.
// 하드 트랜스포트 오류(타임아웃/취소/미연결)는 즉시 error 포트로 라우팅한다.
func (n *ModbusWriteNode) Process(ctx context.Context, msg message.Message) (result []message.Message, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("modbus-write: panic recovered: %v", r)
		}
	}()

	// command_set: payload 오버라이드가 config 기본값을 이긴다.
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

	// 에이전트 미연결이면 하드 오류로 처리한다.
	if n.underlyingModbusAgent() == nil {
		return nil, ErrModbusNoResolver
	}
	agentType := n.resolvedAgentType()

	results := make([]map[string]any, 0, len(ops))
	overall := true
	for i, op := range ops {
		ok, reason, hardErr := n.applyWriteOp(ctx, agentType, op)
		if hardErr != nil {
			return nil, hardErr
		}
		entry := map[string]any{"index": i, "ok": ok}
		if !ok {
			entry["reason"] = reason
			overall = false
		}
		results = append(results, entry)
	}

	out := msg.Clone()
	out.Payload().Set("success", overall)
	out.Payload().Set("results", results)
	out.Payload().Set("agent_type", agentType)
	out.SetType("response")
	return []message.Message{out}, nil
}

// applyWriteOp 은 단일 쓰기 op를 적용한다.
// 반환: (ok, reason, hardErr). hardErr != nil 이면 즉시 중단해야 한다.
func (n *ModbusWriteNode) applyWriteOp(ctx context.Context, agentType string, op map[string]any) (bool, string, error) {
	w := parseWriteOp(op)

	if !validRegisterAreas[w.area] {
		return false, "invalid register area: " + w.area, nil
	}
	if !w.hasValue && !w.hasValues {
		return false, "missing value/values", nil
	}

	cmdBytes, buildErr := n.buildWriteCommand(agentType, w)
	if buildErr != nil {
		// 의미론적 빌드 오류(read-only 영역 등)는 op별 reason으로 수집한다.
		return false, buildErr.Error(), nil
	}

	respBytes, err := n.callModbusAgentProcess(ctx, cmdBytes)
	if err != nil {
		if isHardModbusError(err) {
			return false, "", err
		}
		return false, err.Error(), nil
	}

	ok, reason := evalWriteResponse(agentType, respBytes)
	return ok, reason, nil
}

// buildWriteCommand 는 agent 타입과 op에 맞는 Process 명령 JSON을 생성한다.
func (n *ModbusWriteNode) buildWriteCommand(agentType string, w modbusWriteOp) ([]byte, error) {
	cfg := writeOpConfig(w)

	switch agentType {
	case agentTypeServer:
		switch w.area {
		case areaCoils, areaHoldingRegisters:
			b, err := buildModbusServerWriteCommand(cfg, w.value, w.hasValue, w.values, w.hasValues)
			if err != nil {
				return nil, err
			}
			// 살베지 빌더는 unit_id를 넣지 않으므로 명시된 경우 후처리로 주입한다.
			if w.hasUnitID {
				return injectServerUnitID(b, w.unitID)
			}
			return b, nil
		case areaDiscreteInputs, areaInputRegisters:
			// Server 전용: discrete_inputs / input_registers 는 set_input(s)로 쓴다
			// (살베지 빌더가 다루지 않으므로 인라인으로 생성).
			return buildServerSetInputCommand(w)
		}
	case agentTypeClient:
		switch w.area {
		case areaCoils, areaHoldingRegisters:
			return buildModbusClientWriteCommand(cfg, w.value, w.hasValue, w.values, w.hasValues)
		case areaDiscreteInputs, areaInputRegisters:
			return nil, fmt.Errorf("client cannot write to read-only area %q", w.area)
		}
	}
	return nil, fmt.Errorf("modbus-write: unresolved agent type or area %q", w.area)
}

// ---------------------------------------------------------------------------
// op 파싱 / 인라인 명령 빌더
// ---------------------------------------------------------------------------

// modbusWriteOp 는 파싱된 단일 쓰기 op이다.
type modbusWriteOp struct {
	area      string
	address   uint16
	value     any
	hasValue  bool
	values    any
	hasValues bool
	dataType  string
	byteOrder string
	unitID    uint8
	hasUnitID bool
}

// parseWriteOp 은 op 맵을 modbusWriteOp로 파싱한다.
func parseWriteOp(op map[string]any) modbusWriteOp {
	w := modbusWriteOp{}
	if s, ok := op["area"].(string); ok {
		w.area = s
	}
	w.address = toUint16FromAny(op["address"])
	if v, ok := op["value"]; ok {
		w.value = v
		w.hasValue = true
	}
	if v, ok := op["values"]; ok {
		w.values = v
		w.hasValues = true
	}
	if s, ok := op["data_type"].(string); ok {
		w.dataType = s
	}
	if s, ok := op["byte_order"].(string); ok {
		w.byteOrder = s
	}
	if v, ok := op["unit_id"]; ok {
		w.unitID = toByte(v)
		w.hasUnitID = true
	}
	return w
}

// writeOpConfig 는 op에서 살베지 빌더용 ModbusConfig를 구성한다.
func writeOpConfig(w modbusWriteOp) ModbusConfig {
	cfg := ModbusConfig{
		RegisterArea: w.area,
		Address:      w.address,
		DataType:     w.dataType,
		ByteOrder:    w.byteOrder,
	}
	if cfg.ByteOrder == "" {
		cfg.ByteOrder = defaultByteOrder
	}
	if w.hasUnitID {
		cfg.DeviceID = w.unitID
	} else {
		cfg.DeviceID = 1 // Client 기본 device_id
	}
	cnt := uint16(1)
	if w.hasValues {
		if l := valuesLen(w.values); l > 0 {
			cnt = uint16(l)
		}
	}
	cfg.Count = cnt
	return cfg
}

// buildServerSetInputCommand 는 Server의 discrete_inputs / input_registers
// 쓰기(set_input / set_inputs) 명령 JSON을 생성한다.
func buildServerSetInputCommand(w modbusWriteOp) ([]byte, error) {
	params := map[string]any{
		"area":    w.area,
		"address": w.address,
	}
	if w.dataType != "" {
		params["data_type"] = w.dataType
		byteOrder := w.byteOrder
		if byteOrder == "" {
			byteOrder = defaultByteOrder
		}
		params["byte_order"] = byteOrder
	}
	if w.hasUnitID {
		params["unit_id"] = w.unitID
	}

	var command string
	if w.hasValues {
		command = "set_inputs"
		params["values"] = w.values
	} else {
		command = "set_input"
		params["value"] = w.value
	}

	req := map[string]any{
		"command": command,
		"params":  params,
	}
	return json.Marshal(req)
}
