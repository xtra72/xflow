package node

import (
	"context"
	"fmt"
	"sync"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// chirpStackCommReader 는 status 노드가 에이전트에 요구하는 능력이다.
//
//   - CommStateRecordJSON: commMu 하에서 캐시된 comm-state 를 값 복사로 읽어
//     device_state 레코드 JSON 으로 직렬화(SPEC-CHIRPSTACK-002 REQ-M3-01/03).
//   - CommStateEnabled: emit_comm_state 노브 상태(전제조건 경고용).
//
// 발행 능력은 의도적으로 요구하지 않는다 — status 노드는 캐시 읽기 전용이며 어떤
// 발행 경로도 갖지 않는다(REQ-M3-02). 이 파일 전체에 발행 인터페이스 참조가 없다는
// 사실이 구조적 증거이다.
type chirpStackCommReader interface {
	CommStateRecordJSON(devEui, trigger string) ([]byte, error)
	CommStateEnabled() bool
}

// ChirpStackStatusNode 는 ChirpStack 에이전트의 comm 맵에 캐시된 마지막 통신 상태를
// 입력/트리거 1건당 메시지 1건으로 방출하는 상태 조회 노드이다
// (SPEC-CHIRPSTACK-002 REQ-M3-01).
//
// 전제조건: 대상 에이전트의 `emit_comm_state` 가 true 여야 한다. comm 맵은 이 노브가
// 켜져 있을 때만 채워지므로(agent.go handleUplink), 노브가 꺼진 에이전트에 붙으면 이
// 노드는 항상 offline/unknown 을 방출한다. Init 에서 1회 경고를 남긴다.
//
// 캐시 읽기 전용이다: MQTT 발행 0건, 온디맨드 폴 0건, 타이머/티커 없음(REQ-M3-02).
// 대상 디바이스는 설정이 아니라 입력 메시지 payload 의 unit_id(폴백 device_id)로
// 지정되므로 노드 1개 인스턴스가 N 개 디바이스를 담당한다(control 노드와 동일 관용구).
type ChirpStackStatusNode struct {
	*BaseNode

	agentRef     string
	emitMetadata MetadataEmitOptions

	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent
	// agentName 은 Init(initAgent) 에서 1회 pre-capture 한다 — 방출 경로에서 매번
	// a.Name() 을 재조회하지 않기 위함이다(REQ-FROZEN-B, R4).
	agentName string
	reader    chirpStackCommReader

	mu sync.RWMutex
}

var _ Node = (*ChirpStackStatusNode)(nil)

// NewChirpStackStatusNode 는 ChirpStackStatusNode 팩토리 함수이다.
func NewChirpStackStatusNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &ChirpStackStatusNode{
		BaseNode:     base,
		emitMetadata: DefaultEmitOptions(),
	}
	n.resolver = extractResolverFromConfig(base)
	return n, nil
}

// Configure 는 노드 설정을 적용한다. agent_ref(필수)를 검증한다 (chirpstack-in 미러).
func (n *ChirpStackStatusNode) Configure(config map[string]any) error {
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
		return fmt.Errorf("chirpstack-status: agent_ref 는 필수입니다")
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
func (n *ChirpStackStatusNode) AgentRef() flow.AgentRef {
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// initAgent 는 AgentResolver 로 에이전트를 resolve 하고 comm-state 조회 능력을
// 확인한다. agentName 은 여기서 1회 pre-capture 한다 (REQ-FROZEN-B, R4).
func (n *ChirpStackStatusNode) initAgent(ctx context.Context) error {
	if n.resolver == nil {
		return fmt.Errorf("chirpstack-status: AgentResolver 미주입")
	}
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()

	transport, err := n.resolver.ResolveAgent(ctx, flow.AgentRef{AgentID: ref, AgentName: ref})
	if err != nil {
		return fmt.Errorf("chirpstack-status: agent resolve 실패: %w", err)
	}

	var resolved agent.Agent
	if accessor, ok := transport.(AgentAccessor); ok {
		resolved = accessor.UnderlyingAgent()
	}
	reader, ok := resolved.(chirpStackCommReader)
	if !ok {
		return fmt.Errorf("chirpstack-status: 에이전트가 comm-state 조회를 지원하지 않음")
	}

	// 락 획득 전 1회 캡처 — 방출 경로에서 a.Name() 재조회 금지(HVAC 락 함정 회피).
	agentName := resolved.Name()

	n.mu.Lock()
	n.transport = transport
	n.agent = resolved
	n.agentName = agentName
	n.reader = reader
	n.mu.Unlock()
	return nil
}

// Init 은 노드를 초기화한다. 고루틴/타이머가 없는 process-only 노드이다(REQ-M3-02).
//
// 대상 에이전트의 emit_comm_state 가 꺼져 있으면 1회 경고를 남긴다 — 이 경우 comm
// 맵이 비어 있어 항상 offline/unknown 이 방출된다(하드 실패시키지 않는다).
func (n *ChirpStackStatusNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.initAgent(ctx); err != nil {
		return err
	}

	n.mu.RLock()
	reader := n.reader
	ref := n.agentRef
	n.mu.RUnlock()

	if reader != nil && !reader.CommStateEnabled() {
		if logger := n.Logger(); logger != nil {
			logger.Warn("chirpstack-status: 대상 에이전트의 emit_comm_state 가 비활성 상태입니다 — comm 맵이 채워지지 않아 항상 offline/unknown 이 방출됩니다",
				"node_id", n.ID(),
				"agent_ref", ref,
			)
		}
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 입력/트리거 1건당 캐시된 comm-state 메시지 1건을 방출한다 (REQ-M3-01).
//
// 흐름: devEui 추출 → 에이전트 comm 맵 스냅샷(device_state 레코드 JSON) →
// buildChirpStackDeviceStateMessage(수신 경로와 동일 빌더)로 메시지 빌드.
//
// 발행/폴 없음(REQ-M3-02): 이 경로에는 발행 인터페이스 참조도, 네트워크 왕복도 없다.
// comm 엔트리 부재 시 online=false / last_seen_ms=0 (offline/unknown) 이 방출되며
// online=true 를 조기 보고하지 않는다 (REQ-M3-04).
func (n *ChirpStackStatusNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	reader := n.reader
	a := n.agent
	agentName := n.agentName
	opts := n.emitMetadata
	n.mu.RUnlock()

	if reader == nil {
		return nil, fmt.Errorf("chirpstack-status: 에이전트 미초기화")
	}

	devEui := chirpStackExtractDevEui(msg)
	if devEui == "" {
		return nil, fmt.Errorf("chirpstack-status: payload.unit_id(또는 device_id)가 필요합니다")
	}

	data, err := reader.CommStateRecordJSON(devEui, commTriggerReport)
	if err != nil {
		return nil, fmt.Errorf("chirpstack-status: %w", err)
	}

	out, ok := buildChirpStackDeviceStateMessage(data, n.ID(), a, agentName, opts)
	if !ok {
		return nil, fmt.Errorf("chirpstack-status: devEui=%s 의 device_state 메시지 빌드 실패", devEui)
	}
	return []message.Message{out}, nil
}

// Shutdown 은 노드를 종료한다. 정리할 고루틴/타이머가 없다.
func (n *ChirpStackStatusNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Reinit 은 에이전트 재시작 후 agent / transport / reader 참조와 pre-captured
// agentName 을 재해석한다. process-only 노드이므로 initAgent 만 재호출한다.
func (n *ChirpStackStatusNode) Reinit(ctx context.Context) error {
	return n.initAgent(ctx)
}
