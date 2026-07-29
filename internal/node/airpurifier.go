package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/airpurifier"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 상수 / 센티널 에러 (airpurifier 노드 전용)
// ---------------------------------------------------------------------------

const (
	// airpurifierDefaultTimeout 은 Process 호출 기본 타임아웃이다.
	airpurifierDefaultTimeout = 5 * time.Second

	// airpurifierSourceBuffer 는 status 텔레메트리 / control 출력 포트 채널 버퍼 크기이다.
	airpurifierSourceBuffer = 64

	// airpurifierRecvTimeout 은 텔레메트리 수신 루프의 ReceiveMessage 타임아웃이다.
	airpurifierRecvTimeout = 5 * time.Second
)

var (
	// ErrAirPurifierAgentNotAirPurifier 는 resolve된 Agent가 AirPurifier 타입이 아닐 때 반환된다.
	ErrAirPurifierAgentNotAirPurifier = fmt.Errorf("airpurifier: %w: agent is not an AirPurifier type", ErrInvalidConfig)

	// ErrAirPurifierMissingAgentRef 는 agent_ref 설정이 없을 때 반환된다.
	ErrAirPurifierMissingAgentRef = fmt.Errorf("airpurifier: %w: agent_ref is required", ErrInvalidConfig)

	// ErrAirPurifierNoResolver 는 AgentResolver가 설정되지 않았을 때 반환된다.
	ErrAirPurifierNoResolver = fmt.Errorf("airpurifier: %w: agent resolver not configured", ErrNodeNotInitialized)

	// ErrAirPurifierProcessFailed 는 Agent Process() 호출이 실패했을 때 반환된다.
	ErrAirPurifierProcessFailed = fmt.Errorf("airpurifier: agent process failed")
)

// ---------------------------------------------------------------------------
// AirPurifierNodeConfig
// ---------------------------------------------------------------------------

// AirPurifierNodeConfig 는 공기청정기 노드 공용 설정 구조체이다.
type AirPurifierNodeConfig struct {
	AgentRef string `json:"agent_ref"` // 대상 AirPurifier Agent 이름/ID (필수)
	Timeout  string `json:"timeout"`   // Process 호출 타임아웃 (선택, 기본값 "5s")

	// EmitMetadata 는 metadata 옵션 필드의 emit 정책을 제어한다 (samsung 패턴).
	EmitMetadata MetadataEmitOptions `json:"emit_metadata"`
}

// ---------------------------------------------------------------------------
// airpurifierNodeBase
// ---------------------------------------------------------------------------

// airpurifierNodeBase 는 공기청정기 status/control 노드 공통 기반 구조체이다
// (samsungHvacr01NodeBase 패턴).
type airpurifierNodeBase struct {
	*BaseNode
	apCfg     AirPurifierNodeConfig
	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent
	apAgent   *airpurifier.AirPurifierAgent // 편의용 구체 타입 참조 (initAgent 후 설정)
	timeout   time.Duration
	mu        sync.RWMutex
}

// configure 는 공통 설정 파싱을 수행한다. agent_ref (필수) 를 검증한다.
func (nb *airpurifierNodeBase) configure(config map[string]any) error {
	if err := nb.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg AirPurifierNodeConfig

	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrAirPurifierMissingAgentRef
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
		timeout = airpurifierDefaultTimeout
	}

	nb.mu.Lock()
	nb.apCfg = cfg
	nb.timeout = timeout
	nb.mu.Unlock()

	return nil
}

// initAgent 는 AgentResolver를 통해 에이전트를 resolve하고 AirPurifier 타입을 확인한다.
func (nb *airpurifierNodeBase) initAgent(ctx context.Context) error {
	if nb.resolver == nil {
		return ErrAirPurifierNoResolver
	}

	nb.mu.RLock()
	agentRef := nb.apCfg.AgentRef
	nb.mu.RUnlock()

	ref := flow.AgentRef{AgentID: agentRef, AgentName: agentRef}
	transport, err := nb.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("airpurifier init: agent resolve failed: %w", err)
	}
	nb.transport = transport

	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return ErrAirPurifierAgentNotAirPurifier
	}

	underlyingAgent := accessor.UnderlyingAgent()
	apAgent, ok := underlyingAgent.(*airpurifier.AirPurifierAgent)
	if !ok {
		return ErrAirPurifierAgentNotAirPurifier
	}
	nb.agent = underlyingAgent
	nb.apAgent = apAgent

	return nil
}

// callAgentProcess 는 Agent.Process()를 context timeout과 함께 호출한다.
func (nb *airpurifierNodeBase) callAgentProcess(ctx context.Context, cmdBytes []byte) ([]byte, error) {
	if nb.agent == nil {
		return nil, ErrAirPurifierNoResolver
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
		return nil, fmt.Errorf("airpurifier: %w", timeoutCtx.Err())
	case result := <-ch:
		return result.data, result.err
	}
}

// shutdown 은 공통 종료 로직을 수행한다.
func (nb *airpurifierNodeBase) shutdown() error {
	return nb.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (nb *airpurifierNodeBase) AgentRef() flow.AgentRef {
	nb.mu.RLock()
	ref := nb.apCfg.AgentRef
	nb.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// ===========================================================================
// AirpurifierStatusNode — 상태 텔레메트리(SourceNode) + 상태 입력 포트(Process)
// ===========================================================================

// AirpurifierStatusNode 는 공기청정기 에이전트의 상태를 다루는 노드이다.
//
// 두 개의 서로 다른 관심사를 가진다 (REQ-AIRPUR-001-06-01, -05-01):
//   - 텔레메트리 출력(SourceNode): 에이전트가 방출하는 device_state_changed 등을
//     ReceiveMessage 로 drain 하여 SourceCh 로 하류(influxdb-write 등)에 전달한다.
//   - 상태 입력 포트(Process): port 모드에서 상류 mqtt-in 의 device-STATE 메시지를 받아
//     에이전트의 FeedState 로 주입한다. direct 모드에서는 에이전트가 자체 구독으로
//     상태를 받으므로 입력 포트는 사용되지 않는다.
//
// 이 노드의 상태 입력 포트는 제어 출력 포트(control 노드)와 항상 분리되어 있다
// (REQ-AIRPUR-001-01-12).
type AirpurifierStatusNode struct {
	airpurifierNodeBase
	sourceCh chan message.Message
	stopCh   chan struct{}
	stopOnce sync.Once
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*AirpurifierStatusNode)(nil)
	_ SourceNode = (*AirpurifierStatusNode)(nil)
)

// NewAirpurifierStatusNode 는 새로운 AirpurifierStatusNode를 생성하는 팩토리 함수이다.
func NewAirpurifierStatusNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &AirpurifierStatusNode{
		airpurifierNodeBase: airpurifierNodeBase{BaseNode: base},
		sourceCh:            make(chan message.Message, airpurifierSourceBuffer),
		stopCh:              make(chan struct{}),
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

// Configure 는 AirpurifierStatusNode의 설정을 적용한다.
func (n *AirpurifierStatusNode) Configure(config map[string]any) error {
	return n.airpurifierNodeBase.configure(config)
}

// Init 은 AirpurifierStatusNode를 초기화하고 텔레메트리 수신 루프를 시작한다.
func (n *AirpurifierStatusNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.airpurifierNodeBase.initAgent(ctx); err != nil {
		if errors.Is(err, ErrAirPurifierNoResolver) || errors.Is(err, ErrAirPurifierAgentNotAirPurifier) {
			return err
		}
		if logger := n.Logger(); logger != nil {
			logger.Warn("airpurifier status init: agent not available, deferring connection",
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
func (n *AirpurifierStatusNode) receiveLoop(stopCh chan struct{}, ag agent.Agent) {
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

		ctx, cancel := context.WithTimeout(context.Background(), airpurifierRecvTimeout)
		data, err := receiver.ReceiveMessage(ctx)
		cancel()
		if err != nil || data == nil {
			continue
		}

		msg := message.New()
		msgType := "device_state_changed"
		if jsonErr := airpurifierTrySetJSONPayload(msg, data); jsonErr != nil {
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
// 주입한다 (REQ-AIRPUR-001-05-01 port 경로). device_id 가 없으면 주입하지 않는다.
//
// 상태 텔레메트리는 SourceCh 로 별도 방출되므로 이 포트는 하류로 메시지를 반환하지 않는다.
func (n *AirpurifierStatusNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	deviceID := airpurifierExtractDeviceID(msg)
	if deviceID == "" {
		return nil, nil
	}
	if n.apAgent == nil {
		return nil, nil
	}

	payload, err := msg.Payload().ToJSON()
	if err != nil {
		return nil, fmt.Errorf("%w: marshal state payload: %v", ErrAirPurifierProcessFailed, err)
	}
	n.apAgent.FeedState(deviceID, payload)

	return nil, nil
}

// Shutdown 은 AirpurifierStatusNode를 종료한다.
func (n *AirpurifierStatusNode) Shutdown(_ context.Context) error {
	n.stopOnce.Do(func() { close(n.stopCh) })
	return n.airpurifierNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 수신 루프를 재구성한다.
func (n *AirpurifierStatusNode) Reinit(ctx context.Context) error {
	n.stopOnce.Do(func() { close(n.stopCh) })

	if err := n.airpurifierNodeBase.initAgent(ctx); err != nil {
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
func (n *AirpurifierStatusNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// AirpurifierControlNode — 제어(Process) + 제어 출력 포트(SourceNode)
// ===========================================================================

// AirpurifierControlNode 는 공기청정기 에이전트에 제어 명령을 전송하는 노드이다.
//
// 두 개의 서로 다른 관심사를 가진다 (REQ-AIRPUR-001-01-11/12):
//   - 제어 입력(Process): 입력 메시지의 제어 명령을 에이전트의 Process 로 전달한다
//     (양 모드 공통). direct 모드에서는 에이전트가 브로커로 직접 발행한다.
//   - 제어 출력 포트(SourceNode): port 모드에서 에이전트의 ControlPort 를 drain 하여
//     각 명령을 하류(mqtt-out)로 방출한다.
//
// 이 노드의 제어 출력 포트는 상태 입력 포트(status 노드)와 항상 분리되어 있다
// (REQ-AIRPUR-001-01-12).
type AirpurifierControlNode struct {
	airpurifierNodeBase
	sourceCh chan message.Message
	stopCh   chan struct{}
	stopOnce sync.Once
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*AirpurifierControlNode)(nil)
	_ SourceNode = (*AirpurifierControlNode)(nil)
)

// NewAirpurifierControlNode 는 새로운 AirpurifierControlNode를 생성하는 팩토리 함수이다.
func NewAirpurifierControlNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &AirpurifierControlNode{
		airpurifierNodeBase: airpurifierNodeBase{BaseNode: base},
		sourceCh:            make(chan message.Message, airpurifierSourceBuffer),
		stopCh:              make(chan struct{}),
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

// Configure 는 AirpurifierControlNode의 설정을 적용한다.
func (n *AirpurifierControlNode) Configure(config map[string]any) error {
	return n.airpurifierNodeBase.configure(config)
}

// Init 은 AirpurifierControlNode를 초기화하고 port 모드에서 제어 출력 drain 루프를 시작한다.
func (n *AirpurifierControlNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.airpurifierNodeBase.initAgent(ctx); err != nil {
		if errors.Is(err, ErrAirPurifierNoResolver) || errors.Is(err, ErrAirPurifierAgentNotAirPurifier) {
			return err
		}
		if logger := n.Logger(); logger != nil {
			logger.Warn("airpurifier control init: agent not available, deferring connection",
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
// ControlMessage 를 하류 message.Message 로 방출한다 (REQ-AIRPUR-001-01-12).
//
// stopCh / apAgent / ag 는 실행 시점 값을 인자로 캡처하여 Reinit 재할당과의
// 데이터 레이스를 회피한다.
func (n *AirpurifierControlNode) drainControlPort(stopCh chan struct{}, apAgent *airpurifier.AirPurifierAgent, ag agent.Agent) {
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
			msg := airpurifierControlMessageToFlow(cm)
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
func (n *AirpurifierControlNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	deviceID := airpurifierExtractDeviceID(msg)

	cmdBytes, err := buildAirpurifierControlCommand(msg, deviceID, n.ID())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAirPurifierProcessFailed, err)
	}

	resp, err := n.airpurifierNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAirPurifierProcessFailed, err)
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

// Shutdown 은 AirpurifierControlNode를 종료한다.
func (n *AirpurifierControlNode) Shutdown(_ context.Context) error {
	n.stopOnce.Do(func() { close(n.stopCh) })
	return n.airpurifierNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 제어 출력 drain 루프를 재구성한다.
func (n *AirpurifierControlNode) Reinit(ctx context.Context) error {
	n.stopOnce.Do(func() { close(n.stopCh) })

	if err := n.airpurifierNodeBase.initAgent(ctx); err != nil {
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
func (n *AirpurifierControlNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// 헬퍼 함수
// ===========================================================================

// airpurifierControlKeys 는 제어 명령으로 인식되는 payload 키 목록이다.
var airpurifierControlKeys = []string{"power", "fan_speed"}

// airpurifierTrySetJSONPayload 는 JSON 바이트를 파싱하여 message payload 에 병합한다.
func airpurifierTrySetJSONPayload(msg message.Message, data []byte) error {
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	for k, v := range parsed {
		msg.Payload().Set(k, v)
	}
	return nil
}

// airpurifierControlMessageToFlow 는 에이전트 ControlMessage 를 하류 message.Message 로
// 변환한다. device_id 는 메타/페이로드에, 인코딩된 명령 페이로드는 payload 로 나른다.
//
// M14 다중 필드: port 모드에서 하류 mqtt-out 이 명령 토픽을 재구성할 수 있도록 주소 필드
// (station_code/place_code/device_index 등)와 attribute 토큰을 메타/페이로드에 실어 나른다.
func airpurifierControlMessageToFlow(cm airpurifier.ControlMessage) message.Message {
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

// airpurifierExtractDeviceID 는 메시지 payload/metadata 에서 device_id 를 추출한다.
func airpurifierExtractDeviceID(msg message.Message) string {
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

// buildAirpurifierControlCommand 는 입력 메시지에서 에이전트 제어 명령 JSON 을 구성한다.
//
// payload 에 "command" 키가 있으면 직접 사용하고, 없으면 제어 키(power/fan_speed)에서
// 명령을 추론한다: 둘 다 있으면 set_multiple, power 만 있으면 set_power, fan_speed 만
// 있으면 set_fan_speed. params 는 payload 의 "params" 를 우선 사용하고, 없으면 제어
// 키에서 수집한다.
func buildAirpurifierControlCommand(msg message.Message, deviceID, nodeID string) ([]byte, error) {
	cmd := map[string]any{}
	if deviceID != "" {
		cmd["device_id"] = deviceID
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
	for _, key := range airpurifierControlKeys {
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
