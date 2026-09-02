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
// ModbusControlNode - command-set 기반 MODBUS 제어 노드
// ---------------------------------------------------------------------------
// config `command_set`(op 배열)을 기본값으로 파싱하고, 입력 payload에 command_set이
// 있으면 이번 호출에 한해 오버라이드한다. 각 op를 순서대로 적용하고 결과 1건을 emit한다.
//
// op 스키마: {action, params?}
//   - start/stop/pause/resume: resolve된 원본 agent의 lifecycle 메서드를 직접 호출(양쪽 공용).
//   - reconnect: Client에 reconnect Process 명령이 없으므로 Stop→Start 로 구현(양쪽 공용).
//   - add_device/remove_device: Server 전용 Process 명령(params 전달).
//   - set_config: Client 전용 Process 명령(params 전달).
//   - command: 범용 탈출구. params.command(string) + params.params(object) 를
//              {command, params} 로 그대로 agent.Process 에 전달.

// ModbusControlNode 는 MODBUS 에이전트를 command-set 기반으로 제어하는 노드이다.
type ModbusControlNode struct {
	modbusNodeBase
	agentRef   string           // 대상 에이전트 이름/ID
	commandSet []map[string]any // config 기본 command_set(op 배열)
}

// 인터페이스 컴파일 체크
var (
	_ Node               = (*ModbusControlNode)(nil)
	_ AgentReinitializer = (*ModbusControlNode)(nil)
)

// ---------------------------------------------------------------------------
// 팩토리
// ---------------------------------------------------------------------------

// NewModbusControlNode 는 새로운 ModbusControlNode를 생성한다.
func NewModbusControlNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &ModbusControlNode{modbusNodeBase: modbusNodeBase{BaseNode: base}}
	n.initModbusResolver()
	return n, nil
}

// ---------------------------------------------------------------------------
// Configure
// ---------------------------------------------------------------------------

// Configure 는 ModbusControlNode의 설정을 적용한다.
// agent_ref(필수), command_set(선택, 배열), timeout(선택)을 파싱한다.
func (n *ModbusControlNode) Configure(config map[string]any) error {
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
func (n *ModbusControlNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()

	if err := n.resolveModbusAgent(ctx, ref); err != nil {
		if logger := n.Logger(); logger != nil {
			logger.Warn("modbus-control init: agent resolve deferred",
				"nodeID", n.ID(), "agentRef", ref, "error", err)
		}
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다(AgentReinitializer).
func (n *ModbusControlNode) AgentRef() flow.AgentRef {
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// Reinit 은 에이전트 재시작 후 agent/transport/agentType을 재해석한다.
func (n *ModbusControlNode) Reinit(ctx context.Context) error {
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()
	return n.resolveModbusAgent(ctx, ref)
}

// Shutdown 은 노드를 종료한다.
func (n *ModbusControlNode) Shutdown(_ context.Context) error {
	return n.modbusShutdown()
}

// ---------------------------------------------------------------------------
// Process
// ---------------------------------------------------------------------------

// Process 는 command_set의 각 제어 op를 순서대로 적용하고 결과 1건을 emit한다.
func (n *ModbusControlNode) Process(ctx context.Context, msg message.Message) (result []message.Message, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("modbus-control: panic recovered: %v", r)
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

	results := make([]map[string]any, 0, len(ops))
	overall := true
	for i, op := range ops {
		action, _ := op["action"].(string)
		ok, reason, hardErr := n.applyControlOp(ctx, op)
		if hardErr != nil {
			return nil, hardErr
		}
		entry := map[string]any{"index": i, "action": action, "ok": ok}
		if !ok {
			entry["reason"] = reason
			overall = false
		}
		results = append(results, entry)
	}

	out := msg.Clone()
	out.Payload().Set("success", overall)
	out.Payload().Set("results", results)
	out.SetType("response")
	return []message.Message{out}, nil
}

// applyControlOp 은 단일 제어 op를 적용한다.
// 반환: (ok, reason, hardErr). hardErr != nil 이면 즉시 중단해야 한다.
func (n *ModbusControlNode) applyControlOp(ctx context.Context, op map[string]any) (bool, string, error) {
	action, _ := op["action"].(string)
	params, _ := op["params"].(map[string]any)

	switch action {
	case "start", "stop", "pause", "resume":
		return n.lifecycleAction(ctx, action)

	case "reconnect":
		// Client/Server 모두 reconnect Process 명령이 없으므로 Stop→Start 로 구현한다.
		a := n.underlyingModbusAgent()
		if a == nil {
			return false, "agent not resolved", nil
		}
		if err := a.Stop(ctx); err != nil {
			return false, "reconnect stop: " + err.Error(), nil
		}
		if err := a.Start(ctx); err != nil {
			return false, "reconnect start: " + err.Error(), nil
		}
		return true, "", nil

	case "add_device", "remove_device":
		if n.resolvedAgentType() != agentTypeServer {
			return false, action + " requires a server agent", nil
		}
		return n.runAgentCommand(ctx, mergeCommand(action, params))

	case "set_config":
		if n.resolvedAgentType() != agentTypeClient {
			return false, "set_config requires a client agent", nil
		}
		return n.runAgentCommand(ctx, mergeCommand("set_config", params))

	case "command":
		// 범용 탈출구: {command, params} 를 그대로 전달한다.
		cmdName, _ := params["command"].(string)
		if cmdName == "" {
			return false, "command action requires params.command", nil
		}
		inner, _ := params["params"].(map[string]any)
		cmd := map[string]any{"command": cmdName}
		if inner != nil {
			cmd["params"] = inner
		}
		return n.runAgentCommand(ctx, cmd)

	default:
		return false, "unsupported action: " + action, nil
	}
}

// lifecycleAction 은 resolve된 원본 agent의 lifecycle 메서드를 직접 호출한다.
func (n *ModbusControlNode) lifecycleAction(ctx context.Context, action string) (bool, string, error) {
	a := n.underlyingModbusAgent()
	if a == nil {
		return false, "agent not resolved", nil
	}
	var err error
	switch action {
	case "start":
		err = a.Start(ctx)
	case "stop":
		err = a.Stop(ctx)
	case "pause":
		err = a.Pause(ctx)
	case "resume":
		err = a.Resume(ctx)
	}
	if err != nil {
		return false, err.Error(), nil
	}
	return true, "", nil
}

// runAgentCommand 는 완성된 {command, ...} 맵을 agent.Process 에 전달하고
// 응답을 판정한다. 하드 트랜스포트 오류는 즉시 반환한다.
func (n *ModbusControlNode) runAgentCommand(ctx context.Context, cmd map[string]any) (bool, string, error) {
	if n.underlyingModbusAgent() == nil {
		return false, "agent not resolved", nil
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return false, err.Error(), nil
	}
	respBytes, err := n.callModbusAgentProcess(ctx, cmdBytes)
	if err != nil {
		if isHardModbusError(err) {
			return false, "", err
		}
		return false, err.Error(), nil
	}
	ok, reason := evalWriteResponse(n.resolvedAgentType(), respBytes)
	return ok, reason, nil
}

// mergeCommand 는 {command: name} 에 params의 최상위 키들을 병합한다.
// (예: set_config 는 device_id/params 를 최상위로 받는다.)
func mergeCommand(name string, params map[string]any) map[string]any {
	cmd := map[string]any{"command": name}
	for k, v := range params {
		cmd[k] = v
	}
	return cmd
}
