package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/xsfm"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 상수 / 센티널 에러 (xsfm 노드 전용)
// ---------------------------------------------------------------------------

const (
	// xsfmDefaultTimeout 은 Process 호출 기본 타임아웃이다.
	xsfmDefaultTimeout = 5 * time.Second

	// xsfmSourceBuffer 는 status 텔레메트리 / control 출력 포트 채널 버퍼 크기이다.
	xsfmSourceBuffer = 64

	// xsfmRecvTimeout 은 텔레메트리 수신 루프의 ReceiveMessage 타임아웃이다.
	xsfmRecvTimeout = 5 * time.Second
)

var (
	// ErrXSFMAgentNotXSFM 는 resolve된 Agent가 XSFM 타입이 아닐 때 반환된다.
	ErrXSFMAgentNotXSFM = fmt.Errorf("xsfm: %w: agent is not an XSFM type", ErrInvalidConfig)

	// ErrXSFMMissingAgentRef 는 agent_ref 설정이 없을 때 반환된다.
	ErrXSFMMissingAgentRef = fmt.Errorf("xsfm: %w: agent_ref is required", ErrInvalidConfig)

	// ErrXSFMNoResolver 는 AgentResolver가 설정되지 않았을 때 반환된다.
	ErrXSFMNoResolver = fmt.Errorf("xsfm: %w: agent resolver not configured", ErrNodeNotInitialized)

	// ErrXSFMProcessFailed 는 Agent Process() 호출이 실패했을 때 반환된다.
	ErrXSFMProcessFailed = fmt.Errorf("xsfm: agent process failed")
)

// ---------------------------------------------------------------------------
// XSFMNodeConfig
// ---------------------------------------------------------------------------

// XSFMNodeConfig 는 설비 노드 공용 설정 구조체이다.
type XSFMNodeConfig struct {
	AgentRef string `json:"agent_ref"` // 대상 XSFM Agent 이름/ID (필수)
	Timeout  string `json:"timeout"`   // Process 호출 타임아웃 (선택, 기본값 "5s")

	// EmitMetadata 는 metadata 옵션 필드의 emit 정책을 제어한다 (samsung 패턴).
	EmitMetadata MetadataEmitOptions `json:"emit_metadata"`
}

// ---------------------------------------------------------------------------
// xsfmNodeBase
// ---------------------------------------------------------------------------

// xsfmNodeBase 는 설비 status/control 노드 공통 기반 구조체이다
// (samsungHvacr01NodeBase 패턴).
type xsfmNodeBase struct {
	*BaseNode
	apCfg     XSFMNodeConfig
	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent
	apAgent   *xsfm.XSFMAgent // 편의용 구체 타입 참조 (initAgent 후 설정)
	timeout   time.Duration
	mu        sync.RWMutex
}

// configure 는 공통 설정 파싱을 수행한다. agent_ref (필수) 를 검증한다.
func (nb *xsfmNodeBase) configure(config map[string]any) error {
	if err := nb.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg XSFMNodeConfig

	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrXSFMMissingAgentRef
	}

	cfg.Timeout = "5s"
	if v, ok := config["timeout"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.Timeout = s
		}
	}

	parseEmitMetadata(config, &cfg.EmitMetadata)

	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		timeout = xsfmDefaultTimeout
	}

	nb.mu.Lock()
	nb.apCfg = cfg
	nb.timeout = timeout
	nb.mu.Unlock()

	return nil
}

// initAgent 는 AgentResolver를 통해 에이전트를 resolve하고 XSFM 타입을 확인한다.
func (nb *xsfmNodeBase) initAgent(ctx context.Context) error {
	if nb.resolver == nil {
		return ErrXSFMNoResolver
	}

	nb.mu.RLock()
	agentRef := nb.apCfg.AgentRef
	nb.mu.RUnlock()

	ref := flow.AgentRef{AgentID: agentRef, AgentName: agentRef}
	transport, err := nb.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("xsfm init: agent resolve failed: %w", err)
	}
	nb.transport = transport

	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return ErrXSFMAgentNotXSFM
	}

	underlyingAgent := accessor.UnderlyingAgent()
	apAgent, ok := underlyingAgent.(*xsfm.XSFMAgent)
	if !ok {
		return ErrXSFMAgentNotXSFM
	}
	nb.agent = underlyingAgent
	nb.apAgent = apAgent

	return nil
}

// callAgentProcess 는 Agent.Process()를 context timeout과 함께 호출한다.
func (nb *xsfmNodeBase) callAgentProcess(ctx context.Context, cmdBytes []byte) ([]byte, error) {
	if nb.agent == nil {
		return nil, ErrXSFMNoResolver
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, nb.timeout)
	defer cancel()

	type processResult struct {
		data []byte
		err  error
	}
	ch := make(chan processResult, 1)

	go func() {
		data, err := nb.agent.Process(cmdBytes)
		ch <- processResult{data: data, err: err}
	}()

	select {
	case <-timeoutCtx.Done():
		return nil, fmt.Errorf("xsfm: %w", timeoutCtx.Err())
	case result := <-ch:
		return result.data, result.err
	}
}

// shutdown 은 공통 종료 로직을 수행한다.
func (nb *xsfmNodeBase) shutdown() error {
	return nb.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (nb *xsfmNodeBase) AgentRef() flow.AgentRef {
	nb.mu.RLock()
	ref := nb.apCfg.AgentRef
	nb.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// ===========================================================================
// XsfmStatusNode — 상태 텔레메트리(SourceNode) + 상태 입력 포트(Process)
// ===========================================================================

// XsfmStatusNode 는 설비 에이전트의 상태를 다루는 노드이다.
//
// 두 개의 서로 다른 관심사를 가진다 (REQ-XSFM-001-06-01, -05-01):
//   - 텔레메트리 출력(SourceNode): 에이전트가 방출하는 device_state_changed 등을
//     ReceiveMessage 로 drain 하여 SourceCh 로 하류(influxdb-write 등)에 전달한다.
//   - 상태 입력 포트(Process): port 모드에서 상류 mqtt-in 의 device-STATE 메시지를 받아
//     에이전트의 FeedState 로 주입한다. direct 모드에서는 에이전트가 자체 구독으로
//     상태를 받으므로 입력 포트는 사용되지 않는다.
//
// 이 노드의 상태 입력 포트는 제어 출력 포트(control 노드)와 항상 분리되어 있다
// (REQ-XSFM-001-01-12).
type XsfmStatusNode struct {
	xsfmNodeBase
	sourceCh chan message.Message
	stopCh   chan struct{}
	stopOnce sync.Once
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*XsfmStatusNode)(nil)
	_ SourceNode = (*XsfmStatusNode)(nil)
)

// NewXsfmStatusNode 는 새로운 XsfmStatusNode를 생성하는 팩토리 함수이다.
func NewXsfmStatusNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &XsfmStatusNode{
		xsfmNodeBase: xsfmNodeBase{BaseNode: base},
		sourceCh:     make(chan message.Message, xsfmSourceBuffer),
		stopCh:       make(chan struct{}),
	}
	if base.config != nil {
		if r, ok := base.config["_agent_resolver"]; ok {
			if resolver, ok := r.(AgentResolver); ok {
				n.resolver = resolver
			}
		}
	}
	return n, nil
}

// Configure 는 XsfmStatusNode의 설정을 적용한다.
func (n *XsfmStatusNode) Configure(config map[string]any) error {
	return n.xsfmNodeBase.configure(config)
}

// Init 은 XsfmStatusNode를 초기화하고 텔레메트리 수신 루프를 시작한다.
func (n *XsfmStatusNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.xsfmNodeBase.initAgent(ctx); err != nil {
		if errors.Is(err, ErrXSFMNoResolver) || errors.Is(err, ErrXSFMAgentNotXSFM) {
			return err
		}
		if logger := n.Logger(); logger != nil {
			logger.Warn("xsfm status init: agent not available, deferring connection",
				"nodeID", n.ID(), "agentRef", n.apCfg.AgentRef, "error", err)
		}
		return n.BaseNode.TransitionTo(lifecycle.StateRunning)
	}

	go n.receiveLoop(n.stopCh, n.agent)

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// receiveLoop 는 에이전트가 방출하는 텔레메트리(device_state_changed 등)를
// ReceiveMessage 로 drain 하여 SourceCh 로 전달한다 (mqtt-subscriber 수신 루프 패턴).
//
// stopCh / ag 는 실행 시점의 값을 인자로 캡처한다 — Reinit 이 필드를 재할당해도
// 이전 루프가 자신의 캡처값만 참조하도록 하여 데이터 레이스를 회피한다.
func (n *XsfmStatusNode) receiveLoop(stopCh chan struct{}, ag agent.Agent) {
	receiver, ok := ag.(agent.MessageReceiver)
	if !ok {
		return
	}
	for {
		select {
		case <-stopCh:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), xsfmRecvTimeout)
		data, err := receiver.ReceiveMessage(ctx)
		cancel()
		if err != nil || data == nil {
			continue
		}

		msg := message.New()
		msgType := "device_state_changed"
		if jsonErr := xsfmTrySetJSONPayload(msg, data); jsonErr != nil {
			msg.Payload().Set("_raw", data)
		} else if v, ok := msg.Payload().Get("type"); ok {
			// 에이전트 이벤트 타입(device_state_changed / device_online / ...)을 보존한다.
			if s, ok := v.(string); ok && s != "" {
				msgType = s
			}
		}
		if n.apCfg.EmitMetadata.NodeID {
			msg.Metadata().Set("node_id", n.ID())
		}
		emitAgentGroup(msg, ag, n.apCfg.EmitMetadata)
		msg.SetType(msgType)

		select {
		case n.sourceCh <- msg:
		case <-stopCh:
			return
		}
	}
}

// Process 는 상태 입력 포트이다: 상류 device-STATE 메시지를 에이전트의 FeedState 로
// 주입한다 (REQ-XSFM-001-05-01 port 경로). device_id 가 없으면 주입하지 않는다.
//
// 상태 텔레메트리는 SourceCh 로 별도 방출되므로 이 포트는 하류로 메시지를 반환하지 않는다.
func (n *XsfmStatusNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	deviceID := xsfmExtractDeviceID(msg)
	if deviceID == "" {
		return nil, nil
	}
	if n.apAgent == nil {
		return nil, nil
	}

	payload, err := msg.Payload().ToJSON()
	if err != nil {
		return nil, fmt.Errorf("%w: marshal state payload: %v", ErrXSFMProcessFailed, err)
	}
	n.apAgent.FeedState(deviceID, payload)

	return nil, nil
}

// Shutdown 은 XsfmStatusNode를 종료한다.
func (n *XsfmStatusNode) Shutdown(_ context.Context) error {
	n.stopOnce.Do(func() { close(n.stopCh) })
	return n.xsfmNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 수신 루프를 재구성한다.
func (n *XsfmStatusNode) Reinit(ctx context.Context) error {
	n.stopOnce.Do(func() { close(n.stopCh) })

	if err := n.xsfmNodeBase.initAgent(ctx); err != nil {
		return err
	}

	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.stopOnce = sync.Once{}
	stopCh := n.stopCh
	ag := n.agent
	n.mu.Unlock()

	go n.receiveLoop(stopCh, ag)
	return nil
}

// SourceCh 는 텔레메트리 메시지를 전달하는 채널을 반환한다 (out 포트).
func (n *XsfmStatusNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// XsfmControlNode — 제어(Process) + 제어 출력 포트(SourceNode)
// ===========================================================================

// XsfmControlNode 는 설비 에이전트에 제어 명령을 전송하는 노드이다.
//
// 두 개의 서로 다른 관심사를 가진다 (REQ-XSFM-001-01-11/12):
//   - 제어 입력(Process): 입력 메시지의 제어 명령을 에이전트의 Process 로 전달한다
//     (양 모드 공통). direct 모드에서는 에이전트가 브로커로 직접 발행한다.
//   - 제어 출력 포트(SourceNode): port 모드에서 에이전트의 ControlPort 를 drain 하여
//     각 명령을 하류(mqtt-out)로 방출한다.
//
// 이 노드의 제어 출력 포트는 상태 입력 포트(status 노드)와 항상 분리되어 있다
// (REQ-XSFM-001-01-12).
type XsfmControlNode struct {
	xsfmNodeBase
	sourceCh chan message.Message
	stopCh   chan struct{}
	stopOnce sync.Once
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*XsfmControlNode)(nil)
	_ SourceNode = (*XsfmControlNode)(nil)
)

// NewXsfmControlNode 는 새로운 XsfmControlNode를 생성하는 팩토리 함수이다.
func NewXsfmControlNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &XsfmControlNode{
		xsfmNodeBase: xsfmNodeBase{BaseNode: base},
		sourceCh:     make(chan message.Message, xsfmSourceBuffer),
		stopCh:       make(chan struct{}),
	}
	if base.config != nil {
		if r, ok := base.config["_agent_resolver"]; ok {
			if resolver, ok := r.(AgentResolver); ok {
				n.resolver = resolver
			}
		}
	}
	return n, nil
}

// Configure 는 XsfmControlNode의 설정을 적용한다.
func (n *XsfmControlNode) Configure(config map[string]any) error {
	return n.xsfmNodeBase.configure(config)
}

// Init 은 XsfmControlNode를 초기화하고 port 모드에서 제어 출력 drain 루프를 시작한다.
func (n *XsfmControlNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.xsfmNodeBase.initAgent(ctx); err != nil {
		if errors.Is(err, ErrXSFMNoResolver) || errors.Is(err, ErrXSFMAgentNotXSFM) {
			return err
		}
		if logger := n.Logger(); logger != nil {
			logger.Warn("xsfm control init: agent not available, deferring connection",
				"nodeID", n.ID(), "agentRef", n.apCfg.AgentRef, "error", err)
		}
		return n.BaseNode.TransitionTo(lifecycle.StateRunning)
	}

	// port 모드에서만 제어 출력 포트를 drain 한다 (direct 모드는 ControlPort() == nil).
	if n.apAgent != nil && n.apAgent.ControlPort() != nil {
		go n.drainControlPort(n.stopCh, n.apAgent, n.agent)
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// drainControlPort 는 에이전트의 제어 출력 포트(ControlPort)를 drain 하여 각
// ControlMessage 를 하류 message.Message 로 방출한다 (REQ-XSFM-001-01-12).
//
// stopCh / apAgent / ag 는 실행 시점 값을 인자로 캡처하여 Reinit 재할당과의
// 데이터 레이스를 회피한다.
func (n *XsfmControlNode) drainControlPort(stopCh chan struct{}, apAgent *xsfm.XSFMAgent, ag agent.Agent) {
	ctrlCh := apAgent.ControlPort()
	if ctrlCh == nil {
		return
	}
	for {
		select {
		case <-stopCh:
			return
		case cm, ok := <-ctrlCh:
			if !ok {
				return
			}
			msg := xsfmControlMessageToFlow(cm)
			if n.apCfg.EmitMetadata.NodeID {
				msg.Metadata().Set("node_id", n.ID())
			}
			emitAgentGroup(msg, ag, n.apCfg.EmitMetadata)
			select {
			case n.sourceCh <- msg:
			case <-stopCh:
				return
			}
		}
	}
}

// Process 는 입력 메시지의 제어 명령을 에이전트에 전달한다 (양 모드 공통).
//
// direct 모드에서는 에이전트가 브로커로 직접 발행하고, port 모드에서는 에이전트가
// ControlPort 로 명령을 방출하며 drainControlPort 가 이를 하류로 전달한다.
func (n *XsfmControlNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	deviceID := xsfmExtractDeviceID(msg)

	cmdBytes, err := buildXsfmControlCommand(msg, deviceID, n.ID())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrXSFMProcessFailed, err)
	}

	resp, err := n.xsfmNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrXSFMProcessFailed, err)
	}

	out := msg.Clone()
	if len(resp) > 0 {
		var result map[string]any
		if jsonErr := json.Unmarshal(resp, &result); jsonErr == nil {
			for k, v := range result {
				out.Payload().Set(k, v)
			}
		}
	}
	if n.apCfg.EmitMetadata.NodeID {
		out.Metadata().Set("node_id", n.ID())
	}
	emitAgentGroup(out, n.agent, n.apCfg.EmitMetadata)
	out.SetType("device_state.response")

	return []message.Message{out}, nil
}

// Shutdown 은 XsfmControlNode를 종료한다.
func (n *XsfmControlNode) Shutdown(_ context.Context) error {
	n.stopOnce.Do(func() { close(n.stopCh) })
	return n.xsfmNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 제어 출력 drain 루프를 재구성한다.
func (n *XsfmControlNode) Reinit(ctx context.Context) error {
	n.stopOnce.Do(func() { close(n.stopCh) })

	if err := n.xsfmNodeBase.initAgent(ctx); err != nil {
		return err
	}

	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.stopOnce = sync.Once{}
	stopCh := n.stopCh
	apAgent := n.apAgent
	ag := n.agent
	n.mu.Unlock()

	if apAgent != nil && apAgent.ControlPort() != nil {
		go n.drainControlPort(stopCh, apAgent, ag)
	}
	return nil
}

// SourceCh 는 제어 출력 포트 메시지를 전달하는 채널을 반환한다.
func (n *XsfmControlNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// XsfmNode — 상태 수신 + 제어 송신 통합 노드
// ===========================================================================

// XsfmNode 는 설비 에이전트의 상태 수신과 제어 송신을 하나로 묶은 통합 노드이다
// (LGAPNode 통합 패턴). XsfmStatusNode 와 XsfmControlNode 의 관심사를 단일 노드로
// 결합하여, 브로커 직결 없이 플로우 상에서 상태를 받고 제어를 보내게 한다.
//
// SourceNode(출력): 두 개의 출력 소스를 단일 SourceCh 로 병합한다.
//   - 상태 텔레메트리: 에이전트가 방출하는 device_state_changed 등을 ReceiveMessage
//     로 drain 하여 하류로 전달한다 (XsfmStatusNode.receiveLoop 패턴).
//   - 제어 출력(port 모드): 에이전트의 ControlPort 를 drain 하여 각 명령을 하류로
//     방출한다 (XsfmControlNode.drainControlPort 패턴). direct 모드에서는
//     ControlPort()==nil 이므로 생략된다.
//
// Process(입력): 입력 메시지를 라우팅한다 (LGAPNode 의 키-존재 기반 라우팅과 일관).
//   - 제어 명령(명시적 command/params 존재): 에이전트 제어 경로로 전달하고 응답을
//     하류로 반환한다 (XsfmControlNode.Process 패턴).
//   - 상태 메시지(그 외, device_id 존재): 에이전트의 FeedState 로 주입한다
//     (XsfmStatusNode.Process 패턴). 하류로는 반환하지 않는다.
type XsfmNode struct {
	xsfmNodeBase
	sourceCh chan message.Message
	stopCh   chan struct{}
	stopOnce sync.Once
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*XsfmNode)(nil)
	_ SourceNode = (*XsfmNode)(nil)
)

// NewXsfmNode 는 새로운 통합 XsfmNode 를 생성하는 팩토리 함수이다.
func NewXsfmNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &XsfmNode{
		xsfmNodeBase: xsfmNodeBase{BaseNode: base},
		sourceCh:     make(chan message.Message, xsfmSourceBuffer),
		stopCh:       make(chan struct{}),
	}
	if base.config != nil {
		if r, ok := base.config["_agent_resolver"]; ok {
			if resolver, ok := r.(AgentResolver); ok {
				n.resolver = resolver
			}
		}
	}
	return n, nil
}

// Configure 는 XsfmNode 의 설정을 적용한다.
func (n *XsfmNode) Configure(config map[string]any) error {
	return n.xsfmNodeBase.configure(config)
}

// Init 은 XsfmNode 를 초기화하고 상태 수신 + 제어 출력 drain 루프를 시작한다.
func (n *XsfmNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.xsfmNodeBase.initAgent(ctx); err != nil {
		if errors.Is(err, ErrXSFMNoResolver) || errors.Is(err, ErrXSFMAgentNotXSFM) {
			return err
		}
		if logger := n.Logger(); logger != nil {
			logger.Warn("xsfm init: agent not available, deferring connection",
				"nodeID", n.ID(), "agentRef", n.apCfg.AgentRef, "error", err)
		}
		return n.BaseNode.TransitionTo(lifecycle.StateRunning)
	}

	n.startSourceLoops(n.stopCh, n.apAgent, n.agent)

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// startSourceLoops 는 상태 텔레메트리 수신 루프와 (port 모드 한정) 제어 출력 drain
// 루프를 시작한다. 두 루프는 동일한 sourceCh 로 방출하여 단일 출력 포트로 병합된다.
// direct 모드에서는 ControlPort()==nil 이므로 제어 출력 drain 은 생략한다.
func (n *XsfmNode) startSourceLoops(stopCh chan struct{}, apAgent *xsfm.XSFMAgent, ag agent.Agent) {
	go n.receiveLoop(stopCh, ag)
	if apAgent != nil && apAgent.ControlPort() != nil {
		go n.drainControlPort(stopCh, apAgent, ag)
	}
}

// receiveLoop 는 에이전트 텔레메트리(device_state_changed 등)를 drain 하여 SourceCh 로
// 전달한다 (XsfmStatusNode.receiveLoop 와 동일 패턴). stopCh / ag 는 실행 시점 값을
// 인자로 캡처하여 Reinit 재할당과의 데이터 레이스를 회피한다.
func (n *XsfmNode) receiveLoop(stopCh chan struct{}, ag agent.Agent) {
	receiver, ok := ag.(agent.MessageReceiver)
	if !ok {
		return
	}
	for {
		select {
		case <-stopCh:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), xsfmRecvTimeout)
		data, err := receiver.ReceiveMessage(ctx)
		cancel()
		if err != nil || data == nil {
			continue
		}

		msg := message.New()
		msgType := "device_state_changed"
		if jsonErr := xsfmTrySetJSONPayload(msg, data); jsonErr != nil {
			msg.Payload().Set("_raw", data)
		} else if v, ok := msg.Payload().Get("type"); ok {
			if s, ok := v.(string); ok && s != "" {
				msgType = s
			}
		}
		if n.apCfg.EmitMetadata.NodeID {
			msg.Metadata().Set("node_id", n.ID())
		}
		emitAgentGroup(msg, ag, n.apCfg.EmitMetadata)
		msg.SetType(msgType)

		select {
		case n.sourceCh <- msg:
		case <-stopCh:
			return
		}
	}
}

// drainControlPort 는 에이전트 제어 출력 포트(ControlPort)를 drain 하여 각 명령을
// 하류 message.Message 로 방출한다 (XsfmControlNode.drainControlPort 와 동일 패턴).
func (n *XsfmNode) drainControlPort(stopCh chan struct{}, apAgent *xsfm.XSFMAgent, ag agent.Agent) {
	ctrlCh := apAgent.ControlPort()
	if ctrlCh == nil {
		return
	}
	for {
		select {
		case <-stopCh:
			return
		case cm, ok := <-ctrlCh:
			if !ok {
				return
			}
			msg := xsfmControlMessageToFlow(cm)
			if n.apCfg.EmitMetadata.NodeID {
				msg.Metadata().Set("node_id", n.ID())
			}
			emitAgentGroup(msg, ag, n.apCfg.EmitMetadata)
			select {
			case n.sourceCh <- msg:
			case <-stopCh:
				return
			}
		}
	}
}

// Process 는 입력 메시지를 제어 경로와 상태 주입 경로로 라우팅한다.
//
// 명시적 command/params 가 있으면 제어 명령으로 간주하여 에이전트 제어 경로로 전달하고
// 응답을 하류로 반환한다. 그 외(raw 상태 payload)는 상태 주입으로 라우팅하여 FeedState 로
// 로스터를 갱신한다 (상태 텔레메트리는 SourceCh 로 별도 방출되므로 하류 반환 없음).
func (n *XsfmNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	// 상태 주입 경로: command/params 가 없으면 상류 device-STATE 로 간주한다.
	if !hasXsfmControlCommand(msg) {
		deviceID := xsfmExtractDeviceID(msg)
		if deviceID == "" || n.apAgent == nil {
			return nil, nil
		}
		payload, err := msg.Payload().ToJSON()
		if err != nil {
			return nil, fmt.Errorf("%w: marshal state payload: %v", ErrXSFMProcessFailed, err)
		}
		n.apAgent.FeedState(deviceID, payload)
		return nil, nil
	}

	// 제어 경로: buildXsfmControlCommand + callAgentProcess (XsfmControlNode.Process 패턴).
	deviceID := xsfmExtractDeviceID(msg)
	cmdBytes, err := buildXsfmControlCommand(msg, deviceID, n.ID())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrXSFMProcessFailed, err)
	}

	resp, err := n.xsfmNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrXSFMProcessFailed, err)
	}

	out := msg.Clone()
	if len(resp) > 0 {
		var result map[string]any
		if jsonErr := json.Unmarshal(resp, &result); jsonErr == nil {
			for k, v := range result {
				out.Payload().Set(k, v)
			}
		}
	}
	if n.apCfg.EmitMetadata.NodeID {
		out.Metadata().Set("node_id", n.ID())
	}
	emitAgentGroup(out, n.agent, n.apCfg.EmitMetadata)
	out.SetType("device_state.response")

	return []message.Message{out}, nil
}

// Shutdown 은 XsfmNode 를 종료한다.
func (n *XsfmNode) Shutdown(_ context.Context) error {
	n.stopOnce.Do(func() { close(n.stopCh) })
	return n.xsfmNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 두 소스 루프를 재구성한다.
func (n *XsfmNode) Reinit(ctx context.Context) error {
	n.stopOnce.Do(func() { close(n.stopCh) })

	if err := n.xsfmNodeBase.initAgent(ctx); err != nil {
		return err
	}

	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.stopOnce = sync.Once{}
	stopCh := n.stopCh
	apAgent := n.apAgent
	ag := n.agent
	n.mu.Unlock()

	n.startSourceLoops(stopCh, apAgent, ag)
	return nil
}

// SourceCh 는 상태 텔레메트리 + 제어 출력이 병합되어 흐르는 채널을 반환한다.
func (n *XsfmNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// 헬퍼 함수
// ===========================================================================

// xsfmControlKeys 는 제어 명령으로 인식되는 payload 키 목록이다.
var xsfmControlKeys = []string{"power", "fan_speed"}

// hasXsfmControlCommand 는 통합 노드(XsfmNode)의 입력 라우팅 판정 함수이다
// (LGAPNode 의 hasLGAPControlKeys 와 일관된 키-존재 기반 판정).
//
// 명시적 command 문자열 또는 params 맵이 있으면 제어 명령으로 간주한다. 추가로 그룹
// 셀렉터(group_id)가 실려 있으면 제어 명령으로 간주한다: group_id 는 device_id 로 키잉되는
// 상태 스냅샷에는 절대 실리지 않고(상태 유입은 device_id 기준) 오직 그룹 일괄 제어에서만
// 유입되므로, {group_id, power} 처럼 command/params 가 없는 그룹 제어도 상태 주입이 아닌
// 제어 경로로 라우팅된다. 그 외의 raw 상태 payload(제어 키만 실린 device-STATE 스냅샷 포함)는
// 상태 주입으로 라우팅되어 제어 명령과 상태 주입의 모호성을 제거한다.
//
// @MX:NOTE: 셀렉터(group_id) 존재는 제어 라우팅 신호이다. device_id+제어키 상태 스냅샷의
// 상태 주입 라우팅을 보존하기 위해 제어키(power/fan_speed) 단독은 여기서 제어로 승격하지 않는다.
func hasXsfmControlCommand(msg message.Message) bool {
	if v, ok := msg.Payload().Get("command"); ok {
		if s, ok := v.(string); ok && s != "" {
			return true
		}
	}
	if _, ok := msg.Payload().Get("params"); ok {
		return true
	}
	if xsfmExtractGroupID(msg) != "" {
		return true
	}
	return false
}

// xsfmTrySetJSONPayload 는 JSON 바이트를 파싱하여 message payload 에 병합한다.
func xsfmTrySetJSONPayload(msg message.Message, data []byte) error {
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	for k, v := range parsed {
		msg.Payload().Set(k, v)
	}
	return nil
}

// xsfmControlMessageToFlow 는 에이전트 ControlMessage 를 하류 message.Message 로
// 변환한다. device_id 는 메타/페이로드에, 인코딩된 명령 페이로드는 payload 로 나른다.
//
// M14 다중 필드: port 모드에서 하류 mqtt-out 이 명령 토픽을 재구성할 수 있도록 주소 필드
// (station_code/place_code/device_index 등)와 attribute 토큰을 메타/페이로드에 실어 나른다.
func xsfmControlMessageToFlow(cm xsfm.ControlMessage) message.Message {
	msg := message.New()
	msg.Payload().Set("device_id", cm.DeviceID)
	// 인코딩된 명령 페이로드가 JSON 이면 파싱하여 payload 로 노출하고, 아니면 원시 바이트로.
	// attribute-per-topic 모드의 스칼라 페이로드는 JSON 이 아닐 수 있으므로 원시 바이트로 보존된다.
	var parsed map[string]any
	if err := json.Unmarshal(cm.Payload, &parsed); err == nil {
		for k, v := range parsed {
			msg.Payload().Set(k, v)
		}
	} else {
		msg.Payload().Set("payload", cm.Payload)
	}
	// 주소 필드/attribute 를 메타·페이로드로 전파 (하류 토픽 재구성용, M14).
	for k, v := range cm.Fields {
		msg.Payload().Set(k, v)
		msg.Metadata().Set(k, v)
	}
	if cm.Attribute != "" {
		msg.Payload().Set("attribute", cm.Attribute)
		msg.Metadata().Set("attribute", cm.Attribute)
	}
	msg.Metadata().Set("device_id", cm.DeviceID)
	msg.SetType("device_command")
	return msg
}

// xsfmExtractDeviceID 는 메시지 payload/metadata 에서 device_id 를 추출한다.
func xsfmExtractDeviceID(msg message.Message) string {
	if v, ok := msg.Payload().Get("device_id"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	if s, ok := msg.Metadata().Get("device_id"); ok && s != "" {
		return s
	}
	return ""
}

// xsfmExtractGroupID 는 메시지 payload/metadata 에서 group_id 셀렉터를 추출한다
// (xsfmExtractDeviceID 미러). payload "group_id" 우선, 없으면 metadata "group_id" 폴백.
// 그룹 일괄 제어(FacilityBulkControl 의 {group_id} 계약)의 셀렉터 유입 경로이다.
func xsfmExtractGroupID(msg message.Message) string {
	if v, ok := msg.Payload().Get("group_id"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	if s, ok := msg.Metadata().Get("group_id"); ok && s != "" {
		return s
	}
	return ""
}

// buildXsfmControlCommand 는 입력 메시지에서 에이전트 제어 명령 JSON 을 구성한다.
//
// 대상 선정(개별/그룹): device_id(개별) 와 group_id(그룹 일괄) 셀렉터를 최상위 필드로
// 실어 보낸다 — 에이전트 processRequest 가 두 셀렉터를 모두 최상위 json 태그로 읽으며
// (device_id/group_id), device_id 가 지정되면 개별 경로, 없으면 셀렉터 fan-out 경로로
// 라우팅한다. 둘 다 있으면 에이전트가 우선순위(device_id > station > line > group_id)를
// 적용하므로, 이 빌더는 존재하는 셀렉터를 조용히 드롭하지 않고 그대로 실어 우선순위 판정을
// 에이전트에 위임한다(현행 device_id 동작 무회귀 + 그룹 제어 신설).
//
// 명령 추론(개별/그룹 공통): payload 에 "command" 키가 있으면 직접 사용하고, 없으면 제어
// 키(power/fan_speed)에서 추론한다 — 둘 다 있으면 set_multiple, power 만 있으면 set_power,
// fan_speed 만 있으면 set_fan_speed. params 는 payload 의 "params" 를 우선 사용하고, 없으면
// 제어 키에서 수집한다. 예) {group_id, power} → set_power 로 해당 그룹을 일괄 제어한다.
func buildXsfmControlCommand(msg message.Message, deviceID, nodeID string) ([]byte, error) {
	cmd := map[string]any{}
	if deviceID != "" {
		cmd["device_id"] = deviceID
	}
	// group_id 셀렉터(그룹 일괄 제어). device_id 와 병존 가능하며 우선순위는 에이전트가
	// 적용한다(위 주석 참조). custom:/station:/line:/레거시 접두사는 에이전트 GroupMembers 가 해석.
	if groupID := xsfmExtractGroupID(msg); groupID != "" {
		cmd["group_id"] = groupID
	}
	if nodeID != "" {
		cmd["node_id"] = nodeID
	}

	// params: 명시적 payload "params" 우선, 없으면 제어 키에서 수집.
	params := map[string]any{}
	if v, ok := msg.Payload().Get("params"); ok {
		if m, ok := v.(map[string]any); ok {
			for k, val := range m {
				params[k] = val
			}
		}
	}
	for _, key := range xsfmControlKeys {
		if _, present := params[key]; present {
			continue
		}
		if v, ok := msg.Payload().Get(key); ok {
			params[key] = v
		}
	}
	cmd["params"] = params

	// command: 명시적 payload "command" 우선, 없으면 제어 키에서 추론.
	if v, ok := msg.Payload().Get("command"); ok {
		if s, ok := v.(string); ok && s != "" {
			cmd["command"] = s
			return json.Marshal(cmd)
		}
	}

	_, hasPower := params["power"]
	_, hasFan := params["fan_speed"]
	switch {
	case hasPower && hasFan:
		cmd["command"] = "set_multiple"
	case hasPower:
		cmd["command"] = "set_power"
	case hasFan:
		cmd["command"] = "set_fan_speed"
	default:
		return nil, fmt.Errorf("no control command or control keys (power/fan_speed) in message")
	}

	return json.Marshal(cmd)
}
