package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 상수 정의
// ---------------------------------------------------------------------------

const (
	// thingplusDefaultBufferSize 는 다운링크 수신 채널의 기본 버퍼 크기이다.
	thingplusDefaultBufferSize = 64

	// thingplusRawMsgType 은 다운링크 수신 바이트가 message.Message JSON 이 아닐 때
	// (FromJSON 실패 시) 부여하는 fallback 메시지 타입이다.
	thingplusRawMsgType = "thingplus.raw"
)

// ---------------------------------------------------------------------------
// 좁은 인터페이스: agentProcessor
// ---------------------------------------------------------------------------

// agentProcessor 는 업링크 노드가 필요로 하는 최소 인터페이스이다.
// agent.Agent 가 Process([]byte)([]byte,error) 를 노출하므로 이를 만족한다.
// 좁은 인터페이스를 사용하여 업링크 노드가 에이전트의 전체 표면에 결합하지 않도록 한다.
type agentProcessor interface {
	Process(data []byte) ([]byte, error)
}

// ---------------------------------------------------------------------------
// ThingplusNodeConfig
// ---------------------------------------------------------------------------

// ThingplusNodeConfig 는 Thingplus 게이트웨이 노드 공용 설정 구조체이다.
// 업링크는 AgentRef + EmitMetadata 만 사용하고, 다운링크는 추가로 Topics / BufferSize 를 사용한다.
type ThingplusNodeConfig struct {
	// AgentRef 는 대상 thingplus-gateway 에이전트 이름/ID 이다 (필수).
	AgentRef string `json:"agent_ref"`

	// Topics 는 다운링크 노드가 추가로 구독할 토픽 목록이다 (선택).
	// 에이전트는 onConnect 에서 v1/gateway/rpc + v1/gateway/attributes 를 자동 구독하므로
	// 노드 레벨 Subscribe 는 선택 사항이다. Topics 가 비어 있으면 Subscribe 를 호출하지 않는다.
	Topics []string `json:"topics"`

	// BufferSize 는 다운링크 수신 채널 버퍼 크기이다 (기본 64).
	BufferSize int `json:"buffer_size"`

	// EmitMetadata 는 metadata 그룹 emit 정책을 제어한다 (P3).
	// Thingplus 는 디바이스 노드가 아니므로 Agent(에이전트 그룹)만 사용한다.
	// parseEmitMetadata 가 Agent 기본 ON — emit_agent:false 로 비활성화.
	EmitMetadata MetadataEmitOptions `json:"emit_metadata"`
}

// ---------------------------------------------------------------------------
// thingplusNodeBase
// ---------------------------------------------------------------------------

// thingplusNodeBase 는 Thingplus 게이트웨이 노드 공통 기반 구조체이다.
// ThingplusUplinkNode, ThingplusDownlinkNode 가 이를 임베딩한다.
// mqttNodeBase 를 참조 모델로 하여 configure()/initAgent()/shutdown()/AgentRef() 를 제공한다.
type thingplusNodeBase struct {
	*BaseNode
	thingplusCfg ThingplusNodeConfig
	resolver     AgentResolver
	transport    AgentTransport
	agent        agent.Agent  // 원본 Agent 객체
	mu           sync.RWMutex // 설정 보호 뮤텍스
}

// configure 는 공통 설정 파싱을 수행한다. agent_ref(필수)를 검증한다.
func (tb *thingplusNodeBase) configure(config map[string]any) error {
	if err := tb.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg ThingplusNodeConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrThingplusMissingAgentRef
	}

	// topics (선택 — 다운링크 전용, 비어 있으면 Subscribe 미호출)
	if v, ok := config["topics"]; ok {
		switch val := v.(type) {
		case []string:
			cfg.Topics = val
		case []any:
			for _, item := range val {
				if s, ok := item.(string); ok {
					cfg.Topics = append(cfg.Topics, s)
				}
			}
		}
	}

	// buffer_size (선택, 기본값 thingplusDefaultBufferSize)
	cfg.BufferSize = thingplusDefaultBufferSize
	if v, ok := config["buffer_size"]; ok {
		if n := toInt(v); n > 0 {
			cfg.BufferSize = n
		}
	}

	// P3: emit_metadata — agent 그룹 emit 정책 (기본 ON).
	parseEmitMetadata(config, &cfg.EmitMetadata)

	tb.mu.Lock()
	tb.thingplusCfg = cfg
	tb.mu.Unlock()

	return nil
}

// initAgent 는 AgentResolver를 통해 에이전트를 resolve한다.
func (tb *thingplusNodeBase) initAgent(ctx context.Context) error {
	if tb.resolver == nil {
		return ErrThingplusNoResolver
	}

	tb.mu.RLock()
	agentRef := tb.thingplusCfg.AgentRef
	tb.mu.RUnlock()

	ref := flow.AgentRef{
		AgentID:   agentRef,
		AgentName: agentRef,
	}
	transport, err := tb.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("thingplus init: agent resolve failed: %w", err)
	}
	tb.transport = transport

	// AgentAccessor를 통해 원본 Agent 객체 획득
	if accessor, ok := transport.(AgentAccessor); ok {
		tb.agent = accessor.UnderlyingAgent()
	}

	return nil
}

// shutdown 은 공통 종료 로직을 수행한다.
func (tb *thingplusNodeBase) shutdown() error {
	return tb.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (tb *thingplusNodeBase) AgentRef() flow.AgentRef {
	tb.mu.RLock()
	ref := tb.thingplusCfg.AgentRef
	tb.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// ===========================================================================
// ThingplusUplinkNode (업링크: device → platform)
// ===========================================================================

// ThingplusUplinkNode 는 인입 플로우 메시지를 thingplus-gateway 에이전트의 업링크 경로로
// 전달하는 Process 기반 노드이다 (mqtt-publisher 유사).
//
// 에이전트의 Process([]byte) 진입점을 호출하여 텔레메트리/속성을 v1/gateway/telemetry 로
// 발행하고 RPC 응답을 처리하도록 위임한다. 발행 토픽은 에이전트가 내부적으로 결정한다.
//
// 메시지 Type 보존이 핵심이다: 인입 message.Message 를 FULL JSON(MarshalJSON)으로
// 직렬화하여 전달하므로, Type "thingplus.rpc.response" 인 메시지가 에이전트의 decodeInbound
// 에서 RPC 응답으로 올바르게 인식된다 (Type 이 소실되면 텔레메트리로 오분류됨).
type ThingplusUplinkNode struct {
	thingplusNodeBase
	processor agentProcessor // 업링크 Process 진입점 (agent.Agent 가 만족)
}

// 인터페이스 컴파일 체크
var _ Node = (*ThingplusUplinkNode)(nil)

// NewThingplusUplinkNode 는 새로운 ThingplusUplinkNode를 생성하는 팩토리 함수이다.
func NewThingplusUplinkNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &ThingplusUplinkNode{
		thingplusNodeBase: thingplusNodeBase{
			BaseNode: base,
		},
	}

	// 옵션에서 AgentResolver 추출
	if base.config != nil {
		if r, ok := base.config["_agent_resolver"]; ok {
			if resolver, ok := r.(AgentResolver); ok {
				n.resolver = resolver
			}
		}
	}

	return n, nil
}

// Configure 는 ThingplusUplinkNode의 설정을 적용한다.
func (n *ThingplusUplinkNode) Configure(config map[string]any) error {
	return n.thingplusNodeBase.configure(config)
}

// Init 은 ThingplusUplinkNode를 초기화한다.
// 에이전트를 resolve하고, 업링크 Process 진입점(agentProcessor)을 확인한다.
func (n *ThingplusUplinkNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.thingplusNodeBase.initAgent(ctx); err != nil {
		return err
	}

	// 업링크 Process 진입점 확인 (agent.Agent 가 Process 를 노출하므로 만족).
	proc, ok := n.agent.(agentProcessor)
	if !ok {
		return ErrThingplusAgentUnsupported
	}
	n.processor = proc

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 인입 메시지를 에이전트의 업링크 경로로 전달한다.
//
// 인입 message.Message 를 FULL JSON(MarshalJSON)으로 직렬화하여 agent.Process 에 전달한다.
// 이는 Type 을 보존하여 RPC 응답("thingplus.rpc.response")이 에이전트에서 올바르게
// 분기되도록 하기 위함이다. 발행 후 원본 메시지를 복제하여 "out" 으로 통과시킨다.
func (n *ThingplusUplinkNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	proc := n.processor
	emit := n.thingplusCfg.EmitMetadata
	ag := n.agent
	n.mu.RUnlock()

	if proc == nil {
		return nil, ErrThingplusAgentUnsupported
	}

	// 인입 메시지를 FULL message.Message JSON 으로 직렬화 (Type 보존).
	data, err := msg.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("%w: message to JSON: %v", ErrThingplusPublishFailed, err)
	}

	// 에이전트 업링크 진입점 호출.
	if _, err := proc.Process(data); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrThingplusPublishFailed, err)
	}

	// 출력 메시지에 통과 메타데이터 설정.
	out := msg.Clone()
	out.Metadata().Set("node_id", n.ID())
	// P3: agent:{type,id} 그룹 (기본 ON). node_id 는 flat 유지.
	emitAgentGroup(out, ag, emit)
	out.SetType("response")

	return []message.Message{out}, nil
}

// Shutdown 은 ThingplusUplinkNode를 종료한다.
func (n *ThingplusUplinkNode) Shutdown(_ context.Context) error {
	return n.thingplusNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport / processor 참조를 재해석한다.
// process-only 노드이므로 별도 고루틴 재시작이 필요 없다.
func (n *ThingplusUplinkNode) Reinit(ctx context.Context) error {
	if err := n.thingplusNodeBase.initAgent(ctx); err != nil {
		return err
	}

	proc, ok := n.agent.(agentProcessor)
	if !ok {
		return ErrThingplusAgentUnsupported
	}
	n.mu.Lock()
	n.processor = proc
	n.mu.Unlock()
	return nil
}

// ===========================================================================
// ThingplusDownlinkNode (다운링크: platform → flow)
// ===========================================================================

// ThingplusDownlinkNode 는 thingplus-gateway 에이전트의 다운링크(RPC/공유 속성)를 드레인하여
// 플로우 메시지를 방출하는 SourceNode이다 (mqtt-subscriber 유사).
//
// 핵심 차이: 에이전트가 recvCh 에 FULLY MARSHALED message.Message JSON(Type 포함)을 넣으므로,
// 이 노드는 message.FromJSON 으로 복원하여 Type("thingplus.rpc.request" /
// "thingplus.attr.update")을 보존한다. mqtt-subscriber 처럼 raw 를 Type="event" 로 감싸지 않는다.
type ThingplusDownlinkNode struct {
	thingplusNodeBase
	receiver   agent.MessageReceiver // 메시지 수신 인터페이스
	subscriber agent.SubscriberAgent // 구독 관리 인터페이스 (Topics 지정 시)
	subscribed bool                  // 노드가 명시적으로 Subscribe 했는지 여부
	sourceCh   chan message.Message  // SourceNode 메시지 채널
	stopCh     chan struct{}         // 수신 루프 종료 시그널
	stopOnce   sync.Once             // stopCh close 보호
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*ThingplusDownlinkNode)(nil)
	_ SourceNode = (*ThingplusDownlinkNode)(nil)
)

// NewThingplusDownlinkNode 는 새로운 ThingplusDownlinkNode를 생성하는 팩토리 함수이다.
func NewThingplusDownlinkNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &ThingplusDownlinkNode{
		thingplusNodeBase: thingplusNodeBase{
			BaseNode: base,
		},
		sourceCh: make(chan message.Message, thingplusDefaultBufferSize),
		stopCh:   make(chan struct{}),
	}

	// 옵션에서 AgentResolver 추출
	if base.config != nil {
		if r, ok := base.config["_agent_resolver"]; ok {
			if resolver, ok := r.(AgentResolver); ok {
				n.resolver = resolver
			}
		}
	}

	return n, nil
}

// Configure 는 ThingplusDownlinkNode의 설정을 적용한다.
// buffer_size 설정에 따라 sourceCh 용량을 재조정한다.
func (n *ThingplusDownlinkNode) Configure(config map[string]any) error {
	if err := n.thingplusNodeBase.configure(config); err != nil {
		return err
	}

	// buffer_size 에 맞춰 sourceCh 재생성 (아직 수신 루프 시작 전이므로 안전).
	n.mu.RLock()
	bufSize := n.thingplusCfg.BufferSize
	n.mu.RUnlock()
	if bufSize > 0 && bufSize != cap(n.sourceCh) {
		n.sourceCh = make(chan message.Message, bufSize)
	}

	return nil
}

// Init 은 ThingplusDownlinkNode를 초기화한다.
// 에이전트를 resolve하고, MessageReceiver(필수) / SubscriberAgent(Topics 지정 시) 인터페이스를
// 확인한 후, 선택적으로 토픽을 구독하고 수신 루프를 시작한다.
func (n *ThingplusDownlinkNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.thingplusNodeBase.initAgent(ctx); err != nil {
		return err
	}

	// MessageReceiver 인터페이스 확인 (필수)
	recv, ok := n.agent.(agent.MessageReceiver)
	if !ok {
		return ErrThingplusAgentNotReceiver
	}
	n.receiver = recv

	// 토픽이 지정된 경우에만 SubscriberAgent 를 확인하고 구독한다.
	// 에이전트가 onConnect 에서 rpc/attributes 를 자동 구독하므로 Subscribe 는 선택 사항이다.
	n.mu.RLock()
	topics := n.thingplusCfg.Topics
	n.mu.RUnlock()

	if len(topics) > 0 {
		sub, ok := n.agent.(agent.SubscriberAgent)
		if !ok {
			return ErrThingplusAgentNotSubscriber
		}
		n.subscriber = sub
		if err := sub.Subscribe(ctx, topics); err != nil {
			return fmt.Errorf("thingplus downlink: subscribe failed: %w", err)
		}
		n.subscribed = true
	}

	// 수신 루프 시작
	go n.receiveLoop()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// receiveLoop 는 에이전트로부터 다운링크 메시지를 수신하여 sourceCh에 전달하는 고루틴이다.
//
// 에이전트는 FULLY MARSHALED message.Message JSON 을 방출하므로 message.FromJSON 으로
// 복원하여 Type 을 보존한다. FromJSON 실패 시 raw 바이트를 thingplusRawMsgType 으로 감싼다.
func (n *ThingplusDownlinkNode) receiveLoop() {
	for {
		select {
		case <-n.stopCh:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		data, err := n.receiver.ReceiveMessage(ctx)
		cancel()

		if err != nil {
			// 타임아웃이나 컨텍스트 취소는 정상 -- 다시 시도
			continue
		}

		if data == nil {
			continue
		}

		msg := n.buildDownlinkMessage(data)

		select {
		case n.sourceCh <- msg:
		case <-n.stopCh:
			return
		}
	}
}

// buildDownlinkMessage 는 수신 바이트로부터 방출 메시지를 조립한다.
//
// message.FromJSON 으로 복원하여 Type("thingplus.rpc.request"/"thingplus.attr.update")을
// 보존한다. 복원 실패 시 raw 페이로드를 thingplusRawMsgType 으로 감싸는 fallback 을 적용한다.
// 두 경로 모두 node_id 메타데이터와 agent 그룹(P3)을 부여한다.
func (n *ThingplusDownlinkNode) buildDownlinkMessage(data []byte) message.Message {
	n.mu.RLock()
	emit := n.thingplusCfg.EmitMetadata
	ag := n.agent
	n.mu.RUnlock()

	msg, err := message.FromJSON(data)
	if err != nil {
		// FromJSON 실패 → raw fallback. Type 을 sensible 값으로 부여한다.
		msg = message.New()
		msg.Payload().Set("_raw", data)
		msg.SetType(thingplusRawMsgType)
	}

	// 메타데이터에 노드 정보 설정 (FromJSON 이 복원한 기존 metadata 는 유지).
	msg.Metadata().Set("node_id", n.ID())
	// P3: agent:{type,id} 그룹 (기본 ON).
	emitAgentGroup(msg, ag, emit)

	return msg
}

// Process 는 ThingplusDownlinkNode에서는 사용되지 않는다 (SourceNode이므로).
// 입력 메시지를 그대로 통과시킨다.
func (n *ThingplusDownlinkNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	return []message.Message{msg}, nil
}

// Shutdown 은 ThingplusDownlinkNode를 종료한다.
// 명시적으로 구독한 토픽이 있으면 해제하고 수신 루프를 종료한다.
func (n *ThingplusDownlinkNode) Shutdown(ctx context.Context) error {
	n.stopOnce.Do(func() {
		close(n.stopCh)
	})

	// 노드가 명시적으로 Subscribe 한 경우에만 Unsubscribe 한다.
	if n.subscribed && n.subscriber != nil {
		n.mu.RLock()
		topics := n.thingplusCfg.Topics
		n.mu.RUnlock()
		if len(topics) > 0 {
			_ = n.subscriber.Unsubscribe(ctx, topics)
		}
	}

	return n.thingplusNodeBase.shutdown()
}

// SourceCh 는 수신된 다운링크 메시지를 전달하는 채널을 반환한다.
func (n *ThingplusDownlinkNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// Reinit 은 에이전트 재시작 후 agent / transport / receiver / subscriber 참조를 재해석하고,
// 기존 receiveLoop 를 종료한 뒤 필요 시 토픽을 재구독하고 새 receiveLoop 를 시작한다.
func (n *ThingplusDownlinkNode) Reinit(ctx context.Context) error {
	// 1. 기존 수신 루프 종료
	n.stopOnce.Do(func() {
		close(n.stopCh)
	})

	// 2. 기존 구독 해제 (구 transport 기준 - best-effort)
	if n.subscribed && n.subscriber != nil {
		n.mu.RLock()
		topics := n.thingplusCfg.Topics
		n.mu.RUnlock()
		if len(topics) > 0 {
			_ = n.subscriber.Unsubscribe(ctx, topics)
		}
	}

	// 3. agent / transport 재해석
	if err := n.thingplusNodeBase.initAgent(ctx); err != nil {
		return err
	}

	// 4. MessageReceiver 인터페이스 재확인
	recv, ok := n.agent.(agent.MessageReceiver)
	if !ok {
		return ErrThingplusAgentNotReceiver
	}

	n.mu.Lock()
	n.receiver = recv
	n.stopCh = make(chan struct{})
	n.stopOnce = sync.Once{}
	n.subscribed = false
	n.subscriber = nil
	topics := n.thingplusCfg.Topics
	n.mu.Unlock()

	// 5. 새 transport 로 토픽 재구독 (지정된 경우).
	if len(topics) > 0 {
		sub, ok := n.agent.(agent.SubscriberAgent)
		if !ok {
			return ErrThingplusAgentNotSubscriber
		}
		if err := sub.Subscribe(ctx, topics); err != nil {
			return fmt.Errorf("thingplus reinit: subscribe failed: %w", err)
		}
		n.mu.Lock()
		n.subscriber = sub
		n.subscribed = true
		n.mu.Unlock()
	}

	// 6. 새 receiveLoop 시작
	go n.receiveLoop()
	return nil
}
