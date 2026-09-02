package node

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
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
//
// ── 동작 모드 ────────────────────────────────────────────────────────────────
//   - On-demand(기본): 입력 메시지마다 Process(ctx, msg)로 1회 읽는다. SourceNode 아님.
//   - Periodic(source): config `poll_interval`(Go duration 문자열, 예 "5s") > 0 AND
//     config `command_set` 이 비어있지 않을 때만 활성. 이때 노드는 SourceNode 가 되어
//     poll_interval 주기의 ticker 가 config command_set 을 실행하고 결과 1건을
//     sourceCh 로 emit 한다. (source 노드는 입력 메시지가 없으므로 config command_set 이
//     필수 — payload override 로 op 를 실어 나를 수 없기 때문. 사용자 확정: "명령셋이
//     추가되어 있는 경우에만 적용".)
//   - poll_interval 이 0/미설정 이거나 config command_set 이 비어있으면 On-demand 모드로
//     동작하며 SourceCh() 는 nil 을 반환한다(엔진의 SourceNode 소비는 select 기반이라
//     nil 채널에 안전). Process 는 두 모드 모두에서 정상 동작한다.

// modbusReadSourceBufSize 는 periodic(source) 모드 sourceCh 버퍼 크기이다.
const modbusReadSourceBufSize = 64

// ModbusReadNode 는 MODBUS 에이전트에서 command-set 기반 읽기를 수행하는 노드이다.
type ModbusReadNode struct {
	modbusNodeBase
	agentRef   string           // 대상 에이전트 이름/ID
	commandSet []map[string]any // config 기본 command_set(op 배열)

	// periodic(source) 모드 (Init 시점의 config 로 확정)
	pollInterval time.Duration        // config poll_interval (0 이면 on-demand)
	sourceMode   bool                 // periodic 모드 활성 여부
	sourceCh     chan message.Message // source 모드에서만 할당(그 외 nil)
	stopCh       chan struct{}        // poll 루프 종료 신호
	pollDone     chan struct{}        // poll 루프 종료 완료 신호(close 순서 보장)
	stopOnce     sync.Once            // stopCh/sourceCh 1회 close 보장
}

// 인터페이스 컴파일 체크
var (
	_ Node               = (*ModbusReadNode)(nil)
	_ AgentReinitializer = (*ModbusReadNode)(nil)
	_ SourceNode         = (*ModbusReadNode)(nil)
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

	// poll_interval (선택, Go duration 문자열). 0/미설정/파싱실패 → on-demand.
	// 실제 periodic 모드 시작 여부는 Init 시점에 poll_interval>0 AND command_set 비어있지
	// 않음으로 확정한다.
	pollInterval := time.Duration(0)
	if s, ok := config["poll_interval"].(string); ok && s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			pollInterval = d
		}
	}

	n.mu.Lock()
	n.agentRef = agentRef
	n.commandSet = ops
	n.timeout = timeout
	n.pollInterval = pollInterval
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

	// periodic(source) 모드: poll_interval>0 AND config command_set 비어있지 않을 때만.
	n.mu.Lock()
	startSource := n.pollInterval > 0 && len(n.commandSet) > 0
	interval := n.pollInterval
	if startSource {
		n.sourceMode = true
		n.sourceCh = make(chan message.Message, modbusReadSourceBufSize)
		n.stopCh = make(chan struct{})
		n.pollDone = make(chan struct{})
	}
	n.mu.Unlock()
	if startSource {
		go n.pollLoop(interval)
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// SourceCh 는 periodic 모드에서 주기 읽기 결과 채널을 반환한다.
// on-demand 모드에서는 nil 을 반환한다(엔진 소비는 select 기반이라 nil 안전).
func (n *ModbusReadNode) SourceCh() <-chan message.Message {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.sourceCh
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

// Shutdown 은 노드를 종료한다. periodic 모드였으면 poll 루프를 멈추고 sourceCh 를 닫는다.
func (n *ModbusReadNode) Shutdown(_ context.Context) error {
	n.mu.RLock()
	sm := n.sourceMode
	n.mu.RUnlock()
	if sm {
		n.stopOnce.Do(func() {
			close(n.stopCh)   // poll 루프 종료 신호
			<-n.pollDone      // poll 루프 완전 종료 대기(send-on-closed 방지)
			close(n.sourceCh) // 엔진 소비 루프에 EOF 통지
		})
	}
	return n.modbusShutdown()
}

// ---------------------------------------------------------------------------
// periodic(source) 모드 — poll 루프
// ---------------------------------------------------------------------------

// pollLoop 는 interval 주기로 config command_set 을 실행하여 결과를 sourceCh 로 emit 한다.
// stopCh 신호를 받으면 종료하고 pollDone 을 닫는다.
func (n *ModbusReadNode) pollLoop(interval time.Duration) {
	defer close(n.pollDone)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			msg, ok := n.pollOnce(context.Background())
			if !ok || msg == nil {
				continue
			}
			// non-blocking send + drop-on-full 경고(trigger.go 패턴).
			select {
			case n.sourceCh <- msg:
			default:
				if logger := n.Logger(); logger != nil {
					logger.Warn("modbus-read: sourceCh full, dropping message", "nodeID", n.ID())
				}
			}
		}
	}
}

// pollOnce 는 config command_set 을 1회 실행하여 결과 메시지를 만든다.
// command_set 이 비었거나 하드 에러(미해결 에이전트/타임아웃)면 (nil,false) — 이번 틱 스킵.
func (n *ModbusReadNode) pollOnce(ctx context.Context) (message.Message, bool) {
	n.mu.RLock()
	ops := n.commandSet
	n.mu.RUnlock()
	if len(ops) == 0 {
		return nil, false
	}

	values, agentType, overall, hardErr := n.executeCommandSet(ctx, ops)
	if hardErr != nil {
		if logger := n.Logger(); logger != nil {
			logger.Warn("modbus-read: periodic read skipped", "nodeID", n.ID(), "error", hardErr)
		}
		return nil, false
	}

	msg := message.New()
	msg.Payload().Set("success", overall)
	msg.Payload().Set("values", values)
	msg.Payload().Set("agent_type", agentType)
	msg.Payload().Set("timestamp", time.Now().UnixMilli())
	msg.Metadata().Set("node_source", "poll")
	msg.Metadata().Set("node_id", n.ID())
	msg.SetType("response")
	return msg, true
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

	values, agentType, overall, hardErr := n.executeCommandSet(ctx, ops)
	if hardErr != nil {
		return nil, hardErr
	}

	out := msg.Clone()
	out.Payload().Set("success", overall)
	out.Payload().Set("values", values)
	out.Payload().Set("agent_type", agentType)
	out.SetType("response")
	return []message.Message{out}, nil
}

// executeCommandSet 은 ops 를 순서대로 읽어 결과를 조립한다.
// Process(on-demand)와 pollOnce(periodic)가 공유하는 핵심 읽기 경로이다.
// 반환: (values, agentType, overall, hardErr). hardErr != nil 이면 즉시 중단해야 한다
// (미해결 에이전트/타임아웃 등). op별 소프트 에러는 entry["error"] 로 수집되며
// overall=false 로 반영된다.
func (n *ModbusReadNode) executeCommandSet(ctx context.Context, ops []map[string]any) (values []map[string]any, agentType string, overall bool, hardErr error) {
	if n.underlyingModbusAgent() == nil {
		return nil, "", false, ErrModbusNoResolver
	}
	agentType = n.resolvedAgentType()

	values = make([]map[string]any, 0, len(ops))
	overall = true
	for i, op := range ops {
		entry, hErr := n.applyReadOp(ctx, agentType, i, op)
		if hErr != nil {
			return nil, agentType, false, hErr
		}
		if _, failed := entry["error"]; failed {
			overall = false
		}
		values = append(values, entry)
	}
	return values, agentType, overall, nil
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
