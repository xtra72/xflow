// remote_bridge_node.go 는 P3 매니저 측 엔진 remote-bridge 통합을 구현한다
// (@SPEC:SPEC-SUBFLOW-001 v1.3 그룹 RB, REQ-SUBFLOW-RB01/RB05/RB06/RB07/RB09/RB12).
//
// 배경 — 살아남은 remote:// flow-node 를 라이브 노드로 실행:
//
//	ExpandSubflows 는 remote:// flow-node 를 확장하지 않고 LIVE NODE 로 남긴다(RB01). P3 은
//	이 살아남은 flow-node 를 매니저 측 엔진에서 라이브 브리지 엔드포인트로 실행한다. flow-node
//	의 INPUT 포트로 들어온 메시지는 bridge.SendInput(port,data)로 원격 노드에 주입되고(WRITE),
//	bridge.Outputs()로 수신한 {port,data}는 flow-node 의 OUTPUT 포트로 emit 되어 하류 와이어로
//	전달된다(READ). flow-node 인스턴스마다 독립 브리지(RB12)이다.
//
// 엔진 제약(단일 노드는 source 또는 processor):
//
//	엔진은 입력 와이어가 있는 노드의 SourceCh 를 드레인하지 않는다(engine.go — source 분기는
//	len(inputWires)==0 한정). 따라서 입력 수신(Process)과 비동기 출력 emit(SourceCh)을 한
//	노드가 동시에 할 수 없다. bridge_runner.go(노드 측)와 동일하게, 살아남은 remote flow-node
//	를 두 실제 노드로 재배선한다:
//	  - 입력 forwarder(ProcessNode): flow-node INPUT 포트 와이어를 받아 Process 에서
//	    controller.forwardInput(port,data) → bridge.SendInput. 즉 `upstream→rfn:inP` 를
//	    `upstream→fwd:inP` 로 재배선.
//	  - 출력 emitter(MultiSourceNode): controller.onOutput(bridge.Outputs 펌프)이 포트별
//	    채널로 흘린 메시지를 엔진이 OUTPUT 와이어로 라우팅. 즉 `rfn:outP→downstream` 를
//	    `emit:outP→downstream` 로 재배선.
//	두 노드는 NodeDef.Config 의 bridgeNodeID 로 process-global managerBridgeTable 에서 per-
//	flow-node 컨트롤러를 찾아 결선한다(tap 노드와 동일 side-channel 패턴 — registry.Create 가
//	NodeDef 만 받으므로).
//
// 라이프사이클: 컨트롤러는 forwarder 노드 Init 에서 start(OpenBridge), shutdown 에서
// stop(bridge.Close)한다(노드 라이프사이클이 브리지 라이프사이클을 구동 — RB09). 출력
// emitter 는 펌프 채널만 보유하므로 controller 가 채널을 close 해 emitter 고루틴을 종료한다.
//
// FlowBridgeOpener 주입: managerBridgeTable 에 컨트롤러를 등록할 때 opener 가 바인딩된다
// (rewireRemoteBridges 가 opener 를 받아 컨트롤러를 만든다). cmd/xflowd(server 모드)가
// FlowServiceAdapter 에 opener 를 설정하고, DeployFlow 가 재배선 시 사용한다. 비-server
// 모드(opener nil)는 remote:// flow-node 배포를 명확한 오류로 거부한다(RB 미지원 모드).
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/xtra/xflow/pkg/flow"
)

const (
	// bridgeFwdType / bridgeEmitType 은 매니저 측 입력 forwarder / 출력 emitter 노드 타입이다.
	// server-mode 엔진 레지스트리에만 등록되며(RegisterRemoteBridgeNodes), 일반 플로우에는
	// 나타나지 않는다(remote:// flow-node 재배선으로만 생성).
	bridgeFwdType  = "__remote_bridge_fwd__"
	bridgeEmitType = "__remote_bridge_emit__"

	// bridgeNodeIDConfigKey 는 fwd/emit 노드가 자신의 per-flow-node 컨트롤러를 찾는 키이다.
	bridgeNodeIDConfigKey = "__remote_bridge_id__"
)

// ErrRemoteBridgeUnavailable 은 server 모드가 아니어서(FlowBridgeOpener 미주입) remote://
// flow-node 를 실행할 수 없을 때 반환된다(비-server 모드 명확한 거부 — RB 미지원).
var ErrRemoteBridgeUnavailable = fmt.Errorf("remote bridge unavailable: remote:// flow-node requires server mode (no FlowBridgeOpener injected)")

// ---------------------------------------------------------------------------
// 매니저 측 브리지 컨트롤러 + process-global 테이블
// ---------------------------------------------------------------------------

// managerBridgeController 는 한 살아남은 remote flow-node 인스턴스의 라이브 브리지 상태를
// 보유한다(RB12 — flow-node 별 독립). forwarder 가 start/stop 을 구동하고, emitter 가
// 출력 채널을 소비한다.
type managerBridgeController struct {
	bridgeNodeID string
	instanceID   string
	remoteFlowID string
	inputPorts   []string
	outputPorts  []string
	opener       FlowBridgeOpener
	logger       *slog.Logger

	mu     sync.Mutex
	bridge FlowBridge
	status BridgeStatus
	closed bool

	// onOutput 은 출력 emitter 노드가 설정하는 콜백이다(포트별 채널로 흘림). 펌프 고루틴이
	// bridge.Outputs() 를 읽어 호출한다. nil 이면 출력은 폐기된다(emitter 미배선 — 방어).
	onOutput func(port string, data json.RawMessage)

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// newManagerBridgeController 는 컨트롤러를 생성한다(start 전 — opener 바인딩).
func newManagerBridgeController(instanceID, remoteFlowID string, inputPorts, outputPorts []string, opener FlowBridgeOpener, logger *slog.Logger) *managerBridgeController {
	if logger == nil {
		logger = slog.Default()
	}
	return &managerBridgeController{
		instanceID:   instanceID,
		remoteFlowID: remoteFlowID,
		inputPorts:   inputPorts,
		outputPorts:  outputPorts,
		opener:       opener,
		logger:       logger,
	}
}

// start 는 OpenBridge 로 라이브 브리지를 열고 출력/상태 펌프 고루틴을 시작한다(RB05).
// opener 미주입 시 ErrRemoteBridgeUnavailable.
func (c *managerBridgeController) start(ctx context.Context) error {
	if c.opener == nil {
		return ErrRemoteBridgeUnavailable
	}
	bridge, err := c.opener.OpenBridge(ctx, c.instanceID, c.remoteFlowID, c.inputPorts, c.outputPorts)
	if err != nil {
		return fmt.Errorf("remote bridge open(instance=%q, flow=%q): %w", c.instanceID, c.remoteFlowID, err)
	}
	pumpCtx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.bridge = bridge
	c.cancel = cancel
	c.mu.Unlock()

	c.wg.Add(2)
	go c.pumpOutputs(pumpCtx, bridge.Outputs())
	go c.pumpStatus(pumpCtx, bridge.Status())
	c.logger.Info("매니저 라이브 브리지 시작",
		"instance_id", c.instanceID, "remote_flow_id", c.remoteFlowID,
		"input_ports", c.inputPorts, "output_ports", c.outputPorts)
	return nil
}

// pumpOutputs 는 bridge.Outputs() 를 읽어 onOutput 콜백으로 전달한다(READ — RB07). 출력
// emitter 가 이를 OUTPUT 포트 채널로 흘려 엔진이 하류 와이어로 라우팅한다.
func (c *managerBridgeController) pumpOutputs(ctx context.Context, src <-chan BridgeOutput) {
	defer c.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case out, ok := <-src:
			if !ok {
				return
			}
			c.mu.Lock()
			cb := c.onOutput
			c.mu.Unlock()
			if cb != nil {
				cb(out.Port, out.Data)
			}
		}
	}
}

// pumpStatus 는 bridge.Status() 를 읽어 lastStatus 를 갱신한다(RB09 — P4 web 노출/오프라인
// 무출력 토대). 여기서는 노출/로그만 한다.
func (c *managerBridgeController) pumpStatus(ctx context.Context, src <-chan BridgeStatus) {
	defer c.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case st, ok := <-src:
			if !ok {
				return
			}
			c.mu.Lock()
			c.status = st
			c.mu.Unlock()
			c.logger.Debug("매니저 라이브 브리지 상태",
				"instance_id", c.instanceID, "remote_flow_id", c.remoteFlowID,
				"state", st.State, "detail", st.Detail)
		}
	}
}

// forwardInput 은 forwarder 노드가 호출한다: 입력 메시지를 bridge.SendInput 으로 주입한다
// (WRITE — RB07, 이름 기반 포트 매핑 — RB06).
func (c *managerBridgeController) forwardInput(port string, data json.RawMessage) error {
	c.mu.Lock()
	b := c.bridge
	closed := c.closed
	c.mu.Unlock()
	if closed || b == nil {
		return nil // 미시작/이미 종료 — 조용히 무시(노드 정지 경합).
	}
	return b.SendInput(port, data)
}

// lastStatus 는 마지막으로 관측된 브리지 상태를 반환한다(RB09 노출).
func (c *managerBridgeController) lastStatus() BridgeStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// setOnOutput 은 출력 emitter 노드가 자신의 포트별 채널 흘림 콜백을 등록한다.
func (c *managerBridgeController) setOnOutput(fn func(port string, data json.RawMessage)) {
	c.mu.Lock()
	c.onOutput = fn
	c.mu.Unlock()
}

// stop 은 브리지를 닫고 펌프를 종료한다(멱등 — RB09).
func (c *managerBridgeController) stop(_ context.Context) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	b := c.bridge
	cancel := c.cancel
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if b != nil {
		_ = b.Close() // bridge_close + 채널 close → 펌프 종료.
	}
	c.wg.Wait()
	c.logger.Info("매니저 라이브 브리지 종료",
		"instance_id", c.instanceID, "remote_flow_id", c.remoteFlowID)
}

// managerBridgeTable 은 bridgeNodeID → 컨트롤러 매핑의 process-global 레지스트리이다.
// fwd/emit 노드 Init 이 NodeDef.Config 의 bridgeNodeID 로 컨트롤러를 찾아 결선한다.
type managerBridgeTable struct {
	mu sync.RWMutex
	m  map[string]*managerBridgeController
}

func newManagerBridgeTable() *managerBridgeTable {
	return &managerBridgeTable{m: make(map[string]*managerBridgeController)}
}

func (t *managerBridgeTable) register(id string, c *managerBridgeController) {
	t.mu.Lock()
	t.m[id] = c
	t.mu.Unlock()
}

func (t *managerBridgeTable) unregister(id string) {
	t.mu.Lock()
	delete(t.m, id)
	t.mu.Unlock()
}

func (t *managerBridgeTable) lookup(id string) (*managerBridgeController, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	c, ok := t.m[id]
	return c, ok
}

// globalManagerBridgeTable 은 fwd/emit 노드와 deploy 경로가 공유하는 process-global 테이블이다.
var globalManagerBridgeTable = newManagerBridgeTable()

var managerBridgeIDCounter atomic.Uint64

// newManagerBridgeID 는 프로세스 내 유일한 bridgeNodeID 를 생성한다(RB12 — flow-node 별).
func newManagerBridgeID() string {
	return fmt.Sprintf("mbridge-%d", managerBridgeIDCounter.Add(1))
}

// ---------------------------------------------------------------------------
// 재배선: 살아남은 remote flow-node → fwd + emit 쌍
// ---------------------------------------------------------------------------

// bridgeFwdNodeID / bridgeEmitNodeID 는 remote flow-node id 에서 파생한 fwd/emit 노드 id 이다.
func bridgeFwdNodeID(fnID string) string  { return "__rbridge_fwd__" + fnID }
func bridgeEmitNodeID(fnID string) string { return "__rbridge_emit__" + fnID }

// rewireRemoteBridges 는 f 의 살아남은 remote:// flow-node 를 입력 forwarder + 출력 emitter
// 쌍으로 재배선한 새 플로우와, 등록된 컨트롤러 목록을 반환한다(RB01/RB07/RB12).
//
//	각 remote flow-node 마다 고유 bridgeNodeID 컨트롤러를 만들고 tbl 에 등록한다. 부모 와이어
//	중 flow-node 를 INPUT 으로 갖는 와이어(upstream→fn:inP)는 fwd 노드로, flow-node 를 OUTPUT
//	으로 갖는 와이어(fn:outP→downstream)는 emit 노드로 재배선한다. flow-node 자체는 결과에서
//	제거된다(fwd+emit 로 치환).
//
// opener 가 nil 이고 remote flow-node 가 존재하면 ErrRemoteBridgeUnavailable 을 반환한다
// (비-server 모드 명확한 거부 — RB 미지원).
func rewireRemoteBridges(f flow.Flow, opener FlowBridgeOpener, tbl *managerBridgeTable, logger *slog.Logger) (flow.Flow, []*managerBridgeController, error) {
	srcNodes := f.Nodes()
	srcWires := f.Wires()

	// 살아남은 remote flow-node 를 식별한다.
	type remoteFN struct {
		id           string
		instanceID   string
		remoteFlowID string
		inputPorts   []string
		outputPorts  []string
	}
	remotes := make(map[string]*remoteFN)
	for _, n := range srcNodes {
		if n.Type != flowNodeType {
			continue
		}
		flowID, _ := n.Config[flowNodeFlowIDKey].(string)
		instanceID, remoteFlowID, isRemote, perr := parseRemoteFlowRef(flowID)
		if perr != nil {
			return nil, nil, fmt.Errorf("flow-node %q: %w", n.ID, perr)
		}
		if !isRemote {
			continue // LOCAL flow-node 는 ExpandSubflows 에서 이미 소비됨(여기 도달 시 방어).
		}
		rf := &remoteFN{id: n.ID, instanceID: instanceID, remoteFlowID: remoteFlowID}
		for _, p := range n.Inputs {
			rf.inputPorts = append(rf.inputPorts, p.Name)
		}
		for _, p := range n.Outputs {
			rf.outputPorts = append(rf.outputPorts, p.Name)
		}
		remotes[n.ID] = rf
	}

	if len(remotes) == 0 {
		return f, nil, nil // 원격 참조 없음 — 원본 그대로(회귀 0).
	}
	if opener == nil {
		return nil, nil, ErrRemoteBridgeUnavailable
	}

	// 비-remote 노드는 보존, remote flow-node 는 제거 + fwd/emit 추가.
	resultNodes := make([]flow.NodeDef, 0, len(srcNodes)+len(remotes)*2)
	for _, n := range srcNodes {
		if _, isRemote := remotes[n.ID]; isRemote {
			continue // remote flow-node 제거(fwd+emit 로 치환).
		}
		resultNodes = append(resultNodes, n)
	}

	controllers := make([]*managerBridgeController, 0, len(remotes))
	for _, rf := range remotes {
		bridgeNodeID := newManagerBridgeID()
		ctrl := newManagerBridgeController(rf.instanceID, rf.remoteFlowID,
			rf.inputPorts, rf.outputPorts, opener, logger)
		ctrl.bridgeNodeID = bridgeNodeID
		tbl.register(bridgeNodeID, ctrl)
		controllers = append(controllers, ctrl)

		// fwd 노드: flow-node 입력 포트를 입력 포트로 선언(엔진이 입력 와이어 매핑).
		if len(rf.inputPorts) > 0 {
			resultNodes = append(resultNodes,
				bridgeNodeDef(bridgeFwdNodeID(rf.id), bridgeFwdType, bridgeNodeID, rf.inputPorts, nil))
		}
		// emit 노드: flow-node 출력 포트를 출력 포트로 선언(엔진이 출력 와이어 라우팅).
		if len(rf.outputPorts) > 0 {
			resultNodes = append(resultNodes,
				bridgeNodeDef(bridgeEmitNodeID(rf.id), bridgeEmitType, bridgeNodeID, nil, rf.outputPorts))
		}
	}

	// 와이어 재배선: flow-node 를 INPUT/OUTPUT 으로 갖는 와이어를 fwd/emit 로 향하게 한다.
	resultWires := make([]flow.Wire, 0, len(srcWires))
	for _, w := range srcWires {
		nw := w
		if _, ok := remotes[w.TargetNodeID]; ok {
			// upstream → fn:inPort  ⇒  upstream → fwd:inPort.
			nw.TargetNodeID = bridgeFwdNodeID(w.TargetNodeID)
		}
		if _, ok := remotes[w.SourceNodeID]; ok {
			// fn:outPort → downstream  ⇒  emit:outPort → downstream.
			nw.SourceNodeID = bridgeEmitNodeID(w.SourceNodeID)
		}
		resultWires = append(resultWires, nw)
	}

	return flow.RebuildFlow(f, resultNodes, resultWires), controllers, nil
}

// bridgeNodeDef 는 fwd/emit 노드 정의를 만든다. Config 에 bridgeNodeID 를 실어 노드 Init 이
// 컨트롤러를 찾게 한다.
func bridgeNodeDef(id, typ, bridgeNodeID string, inputs, outputs []string) flow.NodeDef {
	nd := flow.NodeDef{
		ID:     id,
		Name:   id,
		Type:   typ,
		Config: map[string]any{bridgeNodeIDConfigKey: bridgeNodeID},
	}
	for _, p := range inputs {
		nd.Inputs = append(nd.Inputs, flow.Port{ID: p, Name: p, Direction: flow.PortInput})
	}
	for _, p := range outputs {
		nd.Outputs = append(nd.Outputs, flow.Port{ID: p, Name: p, Direction: flow.PortOutput})
	}
	return nd
}
