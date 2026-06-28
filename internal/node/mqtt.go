package node

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
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
	// mqttDefaultBufferSize 는 수신 메시지 버퍼의 기본 크기이다.
	mqttDefaultBufferSize = 256
)

// ---------------------------------------------------------------------------
// MQTTNodeConfig
// ---------------------------------------------------------------------------

// MQTTNodeConfig 는 MQTT 노드 공용 설정 구조체이다.
type MQTTNodeConfig struct {
	AgentRef     string   `json:"agent_ref"`     // 대상 MQTT Agent 이름/ID (필수)
	Topics       []string `json:"topics"`        // 구독 토픽 목록 (Subscriber 전용)
	QoS          int      `json:"qos"`           // 기본 QoS 레벨 (0, 1, 2)
	Retained     bool     `json:"retained"`      // 기본 Retained 플래그
	PublishTopic string   `json:"publish_topic"` // 발행 토픽 템플릿 (Publisher 전용)

	// EmitMetadata 는 metadata 그룹 emit 정책을 제어한다 (P3).
	// MQTT 는 디바이스 노드가 아니므로 Agent(에이전트 그룹)만 사용한다.
	// parseEmitMetadata 가 Agent 기본 ON — emit_agent:false 로 비활성화.
	EmitMetadata MetadataEmitOptions `json:"emit_metadata"`
}

// ---------------------------------------------------------------------------
// mqttNodeBase
// ---------------------------------------------------------------------------

// mqttNodeBase 는 MQTT 노드 공통 기반 구조체이다.
// MQTTSubNode, MQTTPublisherNode가 이를 임베딩한다.
type mqttNodeBase struct {
	*BaseNode
	mqttCfg   MQTTNodeConfig
	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent  // 원본 Agent 객체
	mu        sync.RWMutex // 설정 보호 뮤텍스
}

// configure 는 공통 설정 파싱을 수행한다. agent_ref(필수)를 검증한다.
func (mb *mqttNodeBase) configure(config map[string]any) error {
	if err := mb.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg MQTTNodeConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrMQTTMissingAgentRef
	}

	// topics (선택)
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

	// qos (선택, 기본값 0)
	if v, ok := config["qos"]; ok {
		cfg.QoS = toInt(v)
	}

	// retained (선택, 기본값 false)
	if v, ok := config["retained"]; ok {
		if b, ok := v.(bool); ok {
			cfg.Retained = b
		}
	}

	// publish_topic / default_topic (선택, 둘 다 지원)
	if v, ok := config["publish_topic"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.PublishTopic = s
		}
	} else if v, ok := config["default_topic"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.PublishTopic = s
		}
	}

	// default_qos (publish_topic 전용 별칭)
	if v, ok := config["default_qos"]; ok {
		cfg.QoS = toInt(v)
	}

	// default_retained (publish_topic 전용 별칭)
	if v, ok := config["default_retained"]; ok {
		if b, ok := v.(bool); ok {
			cfg.Retained = b
		}
	}

	// P3: emit_metadata — agent 그룹 emit 정책 (기본 ON).
	parseEmitMetadata(config, &cfg.EmitMetadata)

	mb.mu.Lock()
	mb.mqttCfg = cfg
	mb.mu.Unlock()

	return nil
}

// initAgent 는 AgentResolver를 통해 에이전트를 resolve한다.
func (mb *mqttNodeBase) initAgent(ctx context.Context) error {
	if mb.resolver == nil {
		return ErrMQTTNoResolver
	}

	mb.mu.RLock()
	agentRef := mb.mqttCfg.AgentRef
	mb.mu.RUnlock()

	ref := flow.AgentRef{
		AgentID:   agentRef,
		AgentName: agentRef,
	}
	transport, err := mb.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("mqtt init: agent resolve failed: %w", err)
	}
	mb.transport = transport

	// AgentAccessor를 통해 원본 Agent 객체 획득
	if accessor, ok := transport.(AgentAccessor); ok {
		mb.agent = accessor.UnderlyingAgent()
	}

	return nil
}

// shutdown 은 공통 종료 로직을 수행한다.
func (mb *mqttNodeBase) shutdown() error {
	return mb.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (mb *mqttNodeBase) AgentRef() flow.AgentRef {
	mb.mu.RLock()
	ref := mb.mqttCfg.AgentRef
	mb.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// ===========================================================================
// MQTTSubNode (M1)
// ===========================================================================

// MQTTSubNode 는 MQTT 브로커에서 메시지를 구독하는 SourceNode이다.
// Agent의 SubscriberAgent + MessageReceiver 인터페이스를 사용하여
// 토픽 구독 및 메시지 수신을 수행한다.
type MQTTSubNode struct {
	mqttNodeBase
	subscriber agent.SubscriberAgent // 구독 관리 인터페이스
	receiver   agent.MessageReceiver // 메시지 수신 인터페이스
	sourceCh   chan message.Message  // SourceNode 메시지 채널
	stopCh     chan struct{}         // 수신 루프 종료 시그널
	stopOnce   sync.Once             // stopCh close 보호
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*MQTTSubNode)(nil)
	_ SourceNode = (*MQTTSubNode)(nil)
)

// NewMQTTSubNode 는 새로운 MQTTSubNode를 생성하는 팩토리 함수이다.
func NewMQTTSubNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &MQTTSubNode{
		mqttNodeBase: mqttNodeBase{
			BaseNode: base,
		},
		sourceCh: make(chan message.Message, mqttDefaultBufferSize),
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

// Configure 는 MQTTSubNode의 설정을 적용한다.
func (n *MQTTSubNode) Configure(config map[string]any) error {
	return n.mqttNodeBase.configure(config)
}

// Init 은 MQTTSubNode를 초기화한다.
// 에이전트를 resolve하고, SubscriberAgent/MessageReceiver 인터페이스를 확인한 후,
// 토픽을 구독하고 수신 루프를 시작한다.
func (n *MQTTSubNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.mqttNodeBase.initAgent(ctx); err != nil {
		return err
	}

	// SubscriberAgent 인터페이스 확인
	sub, ok := n.agent.(agent.SubscriberAgent)
	if !ok {
		return ErrMQTTAgentNotSubscriber
	}
	n.subscriber = sub

	// MessageReceiver 인터페이스 확인
	recv, ok := n.agent.(agent.MessageReceiver)
	if !ok {
		return ErrMQTTAgentNotReceiver
	}
	n.receiver = recv

	// 토픽 구독
	n.mu.RLock()
	topics := n.mqttCfg.Topics
	n.mu.RUnlock()

	if len(topics) > 0 {
		if err := n.subscriber.Subscribe(ctx, topics); err != nil {
			return fmt.Errorf("mqtt subscriber: subscribe failed: %w", err)
		}
	}

	// 수신 루프 시작
	go n.receiveLoop()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// receiveLoop 는 Agent로부터 메시지를 수신하여 sourceCh에 전달하는 고루틴이다.
func (n *MQTTSubNode) receiveLoop() {
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

		// JSON 파싱하여 플로우 메시지로 변환
		msg := message.New()
		if jsonErr := mqttTrySetJSONPayload(msg, data); jsonErr != nil {
			msg.Payload().Set("_raw", data)
		}

		// 메타데이터에 노드 정보 설정
		msg.Metadata().Set("node_id", n.ID())
		if n.mqttCfg.QoS != 0 {
			msg.Metadata().Set("mqtt.qos", strconv.Itoa(n.mqttCfg.QoS))
		}
		// P3: agent:{type,id} 그룹 (기본 ON). node_id/mqtt.qos 는 flat 유지.
		emitAgentGroup(msg, n.agent, n.mqttCfg.EmitMetadata)
		msg.SetType("event")

		select {
		case n.sourceCh <- msg:
		case <-n.stopCh:
			return
		}
	}
}

// Process 는 MQTTSubNode에서는 사용되지 않는다 (SourceNode이므로).
// 입력 메시지를 그대로 통과시킨다.
func (n *MQTTSubNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	return []message.Message{msg}, nil
}

// Shutdown 은 MQTTSubNode를 종료한다.
// 토픽 구독을 해제하고 수신 루프를 종료한다.
func (n *MQTTSubNode) Shutdown(ctx context.Context) error {
	n.stopOnce.Do(func() {
		close(n.stopCh)
	})

	// 토픽 구독 해제
	if n.subscriber != nil {
		n.mu.RLock()
		topics := n.mqttCfg.Topics
		n.mu.RUnlock()

		if len(topics) > 0 {
			_ = n.subscriber.Unsubscribe(ctx, topics)
		}
	}

	return n.mqttNodeBase.shutdown()
}

// SourceCh 는 수신된 MQTT 메시지를 전달하는 채널을 반환한다.
func (n *MQTTSubNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// Reinit 은 에이전트 재시작 후 agent / transport / subscriber / receiver 참조를
// 재해석하고, 기존 receiveLoop 를 종료한 뒤 토픽을 재구독하고 새 receiveLoop 를
// 시작한다.
func (n *MQTTSubNode) Reinit(ctx context.Context) error {
	// 1. 기존 수신 루프 종료
	n.stopOnce.Do(func() {
		close(n.stopCh)
	})

	// 2. 기존 구독 해제 (구 transport 기준 - best-effort)
	if n.subscriber != nil {
		n.mu.RLock()
		topics := n.mqttCfg.Topics
		n.mu.RUnlock()
		if len(topics) > 0 {
			_ = n.subscriber.Unsubscribe(ctx, topics)
		}
	}

	// 3. agent / transport 재해석
	if err := n.mqttNodeBase.initAgent(ctx); err != nil {
		return err
	}

	// 4. SubscriberAgent / MessageReceiver 인터페이스 재확인
	sub, ok := n.agent.(agent.SubscriberAgent)
	if !ok {
		return ErrMQTTAgentNotSubscriber
	}
	recv, ok := n.agent.(agent.MessageReceiver)
	if !ok {
		return ErrMQTTAgentNotReceiver
	}

	n.mu.Lock()
	n.subscriber = sub
	n.receiver = recv
	n.stopCh = make(chan struct{})
	n.stopOnce = sync.Once{}
	topics := n.mqttCfg.Topics
	n.mu.Unlock()

	// 5. 새 transport 로 토픽 재구독
	if len(topics) > 0 {
		if err := sub.Subscribe(ctx, topics); err != nil {
			return fmt.Errorf("mqtt reinit: subscribe failed: %w", err)
		}
	}

	// 6. 새 receiveLoop 시작
	go n.receiveLoop()
	return nil
}

// ===========================================================================
// MQTTPublisherNode (M2)
// ===========================================================================

// MQTTPublisherNode 는 MQTT 브로커로 메시지를 발행하는 Process 기반 노드이다.
// Agent의 MessagePublisher 인터페이스를 사용하여 토픽/QoS/Retained를 지정하여 발행한다.
type MQTTPublisherNode struct {
	mqttNodeBase
	publisher agent.MessagePublisher // 메시지 발행 인터페이스
}

// 인터페이스 컴파일 체크
var _ Node = (*MQTTPublisherNode)(nil)

// NewMQTTPublisherNode 는 새로운 MQTTPublisherNode를 생성하는 팩토리 함수이다.
func NewMQTTPublisherNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &MQTTPublisherNode{
		mqttNodeBase: mqttNodeBase{
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

// Configure 는 MQTTPublisherNode의 설정을 적용한다.
func (n *MQTTPublisherNode) Configure(config map[string]any) error {
	return n.mqttNodeBase.configure(config)
}

// Init 은 MQTTPublisherNode를 초기화한다.
// 에이전트를 resolve하고, MessagePublisher 인터페이스를 확인한다.
func (n *MQTTPublisherNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.mqttNodeBase.initAgent(ctx); err != nil {
		return err
	}

	// MessagePublisher 인터페이스 확인
	pub, ok := n.agent.(agent.MessagePublisher)
	if !ok {
		return ErrMQTTAgentNotPublisher
	}
	n.publisher = pub

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 입력 메시지를 MQTT 브로커로 발행한다.
// 메시지 메타데이터 또는 설정에서 토픽/QoS/Retained를 추출하고,
// 페이로드를 JSON으로 직렬화하여 MessagePublisher를 통해 발행한다.
// 발행 후 원본 메시지를 복제하여 출력한다.
func (n *MQTTPublisherNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.mqttCfg
	n.mu.RUnlock()

	// 토픽 결정: 메타데이터 > 설정 publish_topic 템플릿
	topic := ""
	if t, ok := msg.Metadata().Get("mqtt.topic"); ok {
		topic = t
	} else if cfg.PublishTopic != "" {
		topic = mqttInterpolateTemplate(cfg.PublishTopic, msg)
	}

	// QoS 결정: 메타데이터 > 설정 기본값
	qos := cfg.QoS
	if qosStr, ok := msg.Metadata().Get("mqtt.qos"); ok {
		if parsed, parseErr := strconv.Atoi(qosStr); parseErr == nil {
			qos = parsed
		}
	}

	// Retained 결정: 메타데이터 > 설정 기본값
	retained := cfg.Retained
	if retStr, ok := msg.Metadata().Get("mqtt.retained"); ok {
		retained = (retStr == "true")
	}

	// 페이로드 직렬화
	data, err := msg.Payload().ToJSON()
	if err != nil {
		return nil, fmt.Errorf("%w: payload to JSON: %v", ErrMQTTPublishFailed, err)
	}

	// MessagePublisher를 통해 발행
	if err := n.publisher.PublishMessage(topic, byte(qos), retained, data); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMQTTPublishFailed, err)
	}

	// 출력 메시지에 발행 메타데이터 설정
	out := msg.Clone()
	out.Metadata().Set("node_id", n.ID())
	out.Metadata().Set("mqtt_published_topic", topic)
	// P3: agent:{type,id} 그룹 (기본 ON). node_id/mqtt_published_topic 는 flat 유지.
	emitAgentGroup(out, n.agent, n.mqttCfg.EmitMetadata)
	out.SetType("response")

	return []message.Message{out}, nil
}

// Shutdown 은 MQTTPublisherNode를 종료한다.
func (n *MQTTPublisherNode) Shutdown(_ context.Context) error {
	return n.mqttNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport / publisher 참조를 재해석한다.
// process-only 노드이므로 별도 고루틴 재시작이 필요 없다.
func (n *MQTTPublisherNode) Reinit(ctx context.Context) error {
	if err := n.mqttNodeBase.initAgent(ctx); err != nil {
		return err
	}

	pub, ok := n.agent.(agent.MessagePublisher)
	if !ok {
		return ErrMQTTAgentNotPublisher
	}
	n.mu.Lock()
	n.publisher = pub
	n.mu.Unlock()
	return nil
}

// ===========================================================================
// 헬퍼 함수
// ===========================================================================

// toInt 는 다양한 숫자 타입을 int로 변환한다.
// JSON 언마샬 시 float64로 들어오는 경우를 처리한다.
func toInt(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case int64:
		return int(val)
	default:
		return 0
	}
}

// mqttTemplatePattern 은 {expr} 형식의 플레이스홀더를 매칭하는 정규식이다.
// v0.18.2: `}` 를 제외한 모든 문자를 expr 로 허용하여 JSONPath
// (e.g., `$.metadata.device_type`) 을 지원.
var mqttTemplatePattern = regexp.MustCompile(`\{([^{}]+)\}`)

// mqttInterpolateTemplate 은 템플릿 문자열의 {expr} 플레이스홀더를 메시지 값으로 치환한다.
//
// expr 형식:
//   - `field` — 페이로드 직접 키 (legacy)
//   - `$.payload.field` / `$.payload.x.y` — 페이로드 JSONPath
//   - `$.metadata.key` — 메타데이터 단일 키
//   - `$.id` / `$.type` / `$.timestamp` — 메시지 top-level 필드
//
// 해당 키가 없거나 평가 실패 시 원본 플레이스홀더를 유지한다 (v0.18.2).
// 예: `xflow/{$.metadata.device_type}/status` 가 `xflow/indoor/status` 로 치환됨.
func mqttInterpolateTemplate(template string, msg message.Message) string {
	return mqttTemplatePattern.ReplaceAllStringFunc(template, func(match string) string {
		expr := match[1 : len(match)-1]
		val, err := resolveTemplateExpr(expr, msg)
		if err != nil {
			return match
		}
		return fmt.Sprintf("%v", val)
	})
}

// mqttTrySetJSONPayload 는 바이트 데이터를 JSON으로 파싱하여 메시지 페이로드에 설정한다.
// JSON 파싱에 실패하면 에러를 반환한다.
func mqttTrySetJSONPayload(msg message.Message, data []byte) error {
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	for k, v := range parsed {
		msg.Payload().Set(k, v)
	}
	return nil
}
