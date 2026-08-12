package node

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// chirpStackDefaultBufferSize 는 노드 source 채널의 기본 버퍼 크기이다.
const chirpStackDefaultBufferSize = 256

// ChirpStackInNode 는 ChirpStack 에이전트가 fan-out 한 per-measurement 레코드를
// 소비하여 flow message 로 방출하는 수신 전용 SourceNode 이다
// (SPEC-CHIRPSTACK-001, REQ-M3-06, REQ-FROZEN-01/02).
//
// mqtt-subscriber 패턴을 미러링한다: agent_ref 로 에이전트를 resolve 하고
// MessageReceiver 로 레코드를 수신, receiveLoop 로 sourceCh 에 전달한다. 기존 Lua
// script+split 파이프라인을 대체하며 store/influx 소비자에 직접 연결된다.
type ChirpStackInNode struct {
	*BaseNode

	agentRef     string
	emitMetadata MetadataEmitOptions

	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent
	receiver  agent.MessageReceiver

	sourceCh chan message.Message
	stopCh   chan struct{}
	stopOnce sync.Once
	mu       sync.RWMutex
}

var (
	_ Node       = (*ChirpStackInNode)(nil)
	_ SourceNode = (*ChirpStackInNode)(nil)
)

// NewChirpStackInNode 는 ChirpStackInNode 팩토리 함수이다.
func NewChirpStackInNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &ChirpStackInNode{
		BaseNode:     base,
		emitMetadata: DefaultEmitOptions(),
		sourceCh:     make(chan message.Message, chirpStackDefaultBufferSize),
		stopCh:       make(chan struct{}),
	}
	n.resolver = extractResolverFromConfig(base)
	return n, nil
}

// Configure 는 노드 설정을 적용한다. agent_ref(필수)를 검증한다.
func (n *ChirpStackInNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	var ref string
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok {
			ref = s
		}
	}
	if ref == "" {
		return fmt.Errorf("chirpstack-in: agent_ref 는 필수입니다")
	}

	var emit MetadataEmitOptions
	parseEmitMetadata(config, &emit)

	n.mu.Lock()
	n.agentRef = ref
	n.emitMetadata = emit
	n.mu.Unlock()

	if r := extractResolverFromConfig(n.BaseNode); r != nil {
		n.resolver = r
	}
	return nil
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer /
// flow validate 의 agent_ref 필수 검증 대상).
func (n *ChirpStackInNode) AgentRef() flow.AgentRef {
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// initAgent 는 AgentResolver 로 에이전트를 resolve 하고 MessageReceiver 를 확인한다.
func (n *ChirpStackInNode) initAgent(ctx context.Context) error {
	if n.resolver == nil {
		return fmt.Errorf("chirpstack-in: AgentResolver 미주입")
	}
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()

	transport, err := n.resolver.ResolveAgent(ctx, flow.AgentRef{AgentID: ref, AgentName: ref})
	if err != nil {
		return fmt.Errorf("chirpstack-in: agent resolve 실패: %w", err)
	}
	n.transport = transport

	if accessor, ok := transport.(AgentAccessor); ok {
		n.agent = accessor.UnderlyingAgent()
	}
	recv, ok := n.agent.(agent.MessageReceiver)
	if !ok {
		return fmt.Errorf("chirpstack-in: 에이전트가 MessageReceiver 를 구현하지 않음")
	}
	n.receiver = recv
	return nil
}

// Init 은 노드를 초기화하고 수신 루프를 시작한다.
func (n *ChirpStackInNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.initAgent(ctx); err != nil {
		return err
	}
	go n.receiveLoop()
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// receiveLoop 는 에이전트로부터 per-measurement 레코드를 수신하여 message 로 빌드해
// sourceCh 에 전달한다.
func (n *ChirpStackInNode) receiveLoop() {
	n.mu.RLock()
	a := n.agent
	opts := n.emitMetadata
	n.mu.RUnlock()

	// HVAC 락 함정 회피: agentName 을 루프 밖에서 1회 캡처(락 보유 중 self-name
	// 재조회 금지).
	var agentName string
	if a != nil {
		agentName = a.Name()
	}

	for {
		select {
		case <-n.stopCh:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		data, err := n.receiver.ReceiveMessage(ctx)
		cancel()
		if err != nil || data == nil {
			continue
		}

		msg, ok := buildChirpStackMessage(data, n.ID(), a, agentName, opts)
		if !ok {
			continue
		}

		select {
		case n.sourceCh <- msg:
		case <-n.stopCh:
			return
		}
	}
}

// buildChirpStackMessage 는 에이전트가 emit 한 per-measurement 레코드(JSON)를 flow
// message 로 빌드한다 (REQ-FROZEN-01/02, REQ-M3-02/03/04).
//
// 다운스트림 계약(보존): $.type=event, $.timestamp(업링크 time),
// $.payload.value, $.metadata.measurement, $.metadata.tags.*, 그리고 노드가
// unit_id 를 승격한 $.metadata.device.{id,name}.
func buildChirpStackMessage(data []byte, nodeID string, a agent.Agent, agentName string, opts MetadataEmitOptions) (message.Message, bool) {
	var rec struct {
		Measurement string            `json:"measurement"`
		Value       any               `json:"value"`
		UnitID      string            `json:"unit_id"`
		TimeMs      int64             `json:"time_ms"`
		Tags        map[string]string `json:"tags"`
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, false
	}

	msg := message.New()
	msg.SetType("event")
	if rec.TimeMs > 0 {
		msg.SetTimestamp(time.UnixMilli(rec.TimeMs))
	}
	if rec.Measurement != "" {
		msg.Metadata().Set("measurement", rec.Measurement)
	}
	// tags verbatim pass-through (REQ-FROZEN-04) — 키 매핑 없이 그룹으로 전달.
	if len(rec.Tags) > 0 {
		msg.Metadata().SetGroup("tags", rec.Tags)
	}
	if opts.NodeID {
		msg.Metadata().Set("node_id", nodeID)
	}
	emitAgentGroup(msg, a, opts)

	// payload map 을 구성하고 device 그룹을 승격한다 (unit_id → device.{id,name,type}).
	// promoteDevIDWithUUID 는 payload 에서 unit_id 를 제거하고 device 그룹을 채운다.
	payload := make(map[string]any, 2)
	if rec.Value != nil {
		payload["value"] = rec.Value
	}
	if rec.UnitID != "" {
		payload["unit_id"] = rec.UnitID
	}
	promoteDevIDWithUUID(msg, payload, agentName, opts)

	for k, v := range payload {
		msg.Payload().Set(k, v)
	}
	return msg, true
}

// Process 는 SourceNode 이므로 입력 메시지를 그대로 통과시킨다.
func (n *ChirpStackInNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	return []message.Message{msg}, nil
}

// SourceCh 는 수신된 메시지를 전달하는 채널을 반환한다.
func (n *ChirpStackInNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// Shutdown 은 노드를 종료한다.
func (n *ChirpStackInNode) Shutdown(_ context.Context) error {
	n.stopOnce.Do(func() {
		close(n.stopCh)
	})
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}
