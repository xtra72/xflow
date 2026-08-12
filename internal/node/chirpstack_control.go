package node

import (
	"context"
	"fmt"
	"sync"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/chirpstack"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// chirpStackDownlinkQoS 는 다운링크 명령 발행 QoS 이다.
//
// 새 설정 노브를 추가하지 않기 위해(REQ-M4-04) 상수로 고정한다. v1 은 fire-and-publish
// 이며 큐/ack 처리를 하지 않으므로(A4) 최소 오버헤드의 QoS 0 을 사용한다.
const chirpStackDownlinkQoS byte = 0

// chirpStackDownlinker 는 control 노드가 에이전트에 요구하는 능력이다.
//
//   - MessagePublisher: 다운링크 토픽으로의 발행(SPEC-CHIRPSTACK-002 REQ-M1-01).
//   - DownlinkTarget: 업링크로 캐시된 applicationId + deviceProfileName 조회(REQ-M2-05).
type chirpStackDownlinker interface {
	agent.MessagePublisher
	DownlinkTarget(devEui string) (applicationID string, deviceProfileName string, ok bool)
}

// ChirpStackControlNode 는 typed command 입력을 deviceProfile 코덱으로 인코딩하여
// ChirpStack LoRaWAN 다운링크로 발행하는 제어 노드이다 (SPEC-CHIRPSTACK-002 REQ-M2-03).
//
// 노드 1개 인스턴스가 N 개 디바이스를 담당한다 — 대상 디바이스는 설정이 아니라 입력
// 메시지 payload 의 unit_id(폴백 device_id)로 지정된다.
//
// 순서 제약(R5): 해당 devEui 의 최초 업링크가 applicationId 를 캐시하기 전에는 다운링크
// 토픽을 구성할 수 없어 에러를 반환하고 발행하지 않는다.
type ChirpStackControlNode struct {
	*BaseNode

	agentRef     string
	emitMetadata MetadataEmitOptions

	resolver   AgentResolver
	transport  AgentTransport
	agent      agent.Agent
	downlinker chirpStackDownlinker

	mu sync.RWMutex
}

var _ Node = (*ChirpStackControlNode)(nil)

// NewChirpStackControlNode 는 ChirpStackControlNode 팩토리 함수이다.
func NewChirpStackControlNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &ChirpStackControlNode{
		BaseNode:     base,
		emitMetadata: DefaultEmitOptions(),
	}
	n.resolver = extractResolverFromConfig(base)
	return n, nil
}

// Configure 는 노드 설정을 적용한다. agent_ref(필수)를 검증한다 (chirpstack-in 미러).
func (n *ChirpStackControlNode) Configure(config map[string]any) error {
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
		return fmt.Errorf("chirpstack-control: agent_ref 는 필수입니다")
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
func (n *ChirpStackControlNode) AgentRef() flow.AgentRef {
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// initAgent 는 AgentResolver 로 에이전트를 resolve 하고 다운링크 능력을 확인한다.
func (n *ChirpStackControlNode) initAgent(ctx context.Context) error {
	if n.resolver == nil {
		return fmt.Errorf("chirpstack-control: AgentResolver 미주입")
	}
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()

	transport, err := n.resolver.ResolveAgent(ctx, flow.AgentRef{AgentID: ref, AgentName: ref})
	if err != nil {
		return fmt.Errorf("chirpstack-control: agent resolve 실패: %w", err)
	}

	var resolved agent.Agent
	if accessor, ok := transport.(AgentAccessor); ok {
		resolved = accessor.UnderlyingAgent()
	}
	dl, ok := resolved.(chirpStackDownlinker)
	if !ok {
		return fmt.Errorf("chirpstack-control: 에이전트가 다운링크 발행(MessagePublisher + DownlinkTarget)을 지원하지 않음")
	}

	n.mu.Lock()
	n.transport = transport
	n.agent = resolved
	n.downlinker = dl
	n.mu.Unlock()
	return nil
}

// Init 은 노드를 초기화한다. 고루틴이 없는 process-only 노드이다.
func (n *ChirpStackControlNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.initAgent(ctx); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 typed command 입력을 다운링크로 발행한다 (REQ-M2-03).
//
// 흐름: devEui 추출 → 캐시된 applicationId/deviceProfileName 조회 → 코덱 인코딩 →
// 토픽/페이로드 구성 → PublishMessage.
//
// 미등록 코덱/미지 command(REQ-M2-04) 또는 applicationId 미캐시(REQ-M2-05)는 에러를
// 반환하고 발행하지 않는다.
func (n *ChirpStackControlNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	dl := n.downlinker
	a := n.agent
	opts := n.emitMetadata
	n.mu.RUnlock()

	if dl == nil {
		return nil, fmt.Errorf("chirpstack-control: 에이전트 미초기화")
	}

	devEui := chirpStackExtractDevEui(msg)
	if devEui == "" {
		return nil, fmt.Errorf("chirpstack-control: payload.unit_id(또는 device_id)가 필요합니다")
	}

	cmdName := chirpStackExtractCommand(msg)
	if cmdName == "" {
		return nil, fmt.Errorf("chirpstack-control: payload.command 가 필요합니다")
	}

	applicationID, deviceProfileName, ok := dl.DownlinkTarget(devEui)
	if !ok {
		return nil, fmt.Errorf(
			"chirpstack-control: devEui=%s 의 applicationId 미캐시 — 최초 업링크 수신 전에는 다운링크 토픽을 구성할 수 없습니다",
			devEui)
	}

	fPort, data, confirmed, err := chirpstack.EncodeDownlink(deviceProfileName, chirpstack.DownlinkCommand{
		DevEui:    devEui,
		Name:      cmdName,
		Params:    chirpStackExtractParams(msg),
		Confirmed: chirpStackExtractConfirmed(msg),
	})
	if err != nil {
		return nil, fmt.Errorf("chirpstack-control: %w", err)
	}

	topic := chirpstack.BuildDownlinkTopic(applicationID, devEui)
	payload, err := chirpstack.BuildDownlinkPayload(devEui, confirmed, fPort, data)
	if err != nil {
		return nil, fmt.Errorf("chirpstack-control: %w", err)
	}

	if err := dl.PublishMessage(topic, chirpStackDownlinkQoS, false, payload); err != nil {
		return nil, fmt.Errorf("chirpstack-control: 다운링크 발행 실패: %w", err)
	}

	out := msg.Clone()
	if opts.NodeID {
		out.Metadata().Set("node_id", n.ID())
	}
	out.Metadata().Set("chirpstack_command", cmdName)
	out.Metadata().Set("chirpstack_downlink_topic", topic)
	emitAgentGroup(out, a, opts)
	out.SetType("response")

	return []message.Message{out}, nil
}

// Shutdown 은 노드를 종료한다.
func (n *ChirpStackControlNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Reinit 은 에이전트 재시작 후 agent / transport / downlinker 참조를 재해석한다.
// 고루틴이 없는 process-only 노드이므로 initAgent 만 재호출한다.
func (n *ChirpStackControlNode) Reinit(ctx context.Context) error {
	return n.initAgent(ctx)
}

// ---------------------------------------------------------------------------
// 입력 추출 헬퍼 (payload typed command)
// ---------------------------------------------------------------------------

// chirpStackExtractDevEui 는 대상 devEui 를 추출한다.
//
// unit_id 를 우선하고 device_id 로 폴백한다(수신 경로가 devEui 를 unit_id 로 방출하며,
// 다른 제어 노드는 device_id 를 쓰므로 양쪽을 모두 받아들인다). payload 우선,
// metadata 폴백은 xsfmExtractDeviceID 관용구를 따른다.
func chirpStackExtractDevEui(msg message.Message) string {
	for _, key := range []string{"unit_id", "device_id"} {
		if v, ok := msg.Payload().Get(key); ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	for _, key := range []string{"unit_id", "device_id"} {
		if s, ok := msg.Metadata().Get(key); ok && s != "" {
			return s
		}
	}
	return ""
}

// chirpStackExtractCommand 는 payload 의 command 이름을 추출한다
// (buildLGAPControlCommand 관용구).
func chirpStackExtractCommand(msg message.Message) string {
	if v, ok := msg.Payload().Get("command"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// chirpStackExtractParams 는 payload 의 params 맵을 추출한다(없으면 nil).
func chirpStackExtractParams(msg message.Message) map[string]any {
	v, ok := msg.Payload().Get("params")
	if !ok {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// chirpStackExtractConfirmed 는 payload 의 confirmed 플래그를 추출한다(기본 false).
func chirpStackExtractConfirmed(msg message.Message) bool {
	if v, ok := msg.Payload().Get("confirmed"); ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
