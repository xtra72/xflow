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

// sysMetricsDefaultBufferSize 는 노드 source 채널의 기본 버퍼 크기이다.
//
// 표본 하나가 메시지 하나이므로 크지 않아도 된다.
const sysMetricsDefaultBufferSize = 256

// sysMetricsReceiveTimeout 은 1회 수신 대기 상한이다. 초과 시 반환되는
// context.DeadlineExceeded 는 "유휴"이며 실패가 아니다.
const sysMetricsReceiveTimeout = 5 * time.Second

// SysMetricsInNode 는 시스템 모니터링 에이전트가 표본마다 하나씩 내보내는 일괄
// 레코드를 소비하여 flow message 로 방출하는 수신 전용 SourceNode 이다.
//
// chirpstack-in 패턴을 미러링한다: agent_ref 로 에이전트를 resolve 하고
// MessageReceiver 로 레코드를 수신, receiveLoop 로 sourceCh 에 전달한다. 방출된
// 메시지는 storage-write / tsdb-write 로 그대로 이어져 Store·TSDB 에 쌓이고,
// 대시보드 패널이 기존 Store/TSDB 데이터 소스로 조회한다.
type SysMetricsInNode struct {
	*BaseNode

	agentRef     string
	emitMetadata MetadataEmitOptions

	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent
	receiver  agent.MessageReceiver

	// agentName 은 initAgent 에서 1회 pre-capture 한다 — 방출 경로에서 매번
	// Name() 을 호출하면 에이전트 락 함정에 노출되기 때문이다(chirpstack-in 과 동일).
	agentName string

	sourceCh chan message.Message
	stopCh   chan struct{}
	stopOnce sync.Once
	mu       sync.RWMutex
}

var (
	_ Node       = (*SysMetricsInNode)(nil)
	_ SourceNode = (*SysMetricsInNode)(nil)
	// AgentReinitializer 미구현은 엔진의 노드 재초기화 루프에서 조용히 건너뛰어져
	// 에이전트 재시작 후 노드가 영구 무음이 된다. 컴파일 타임에 고정한다.
	_ AgentReinitializer = (*SysMetricsInNode)(nil)
)

// NewSysMetricsInNode 는 SysMetricsInNode 팩토리 함수이다.
func NewSysMetricsInNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &SysMetricsInNode{
		BaseNode:     base,
		emitMetadata: DefaultEmitOptions(),
		sourceCh:     make(chan message.Message, sysMetricsDefaultBufferSize),
		stopCh:       make(chan struct{}),
	}
	n.resolver = extractResolverFromConfig(base)
	return n, nil
}

// Configure 는 노드 설정을 적용한다. agent_ref(필수)를 검증한다.
func (n *SysMetricsInNode) Configure(config map[string]any) error {
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
		return fmt.Errorf("sysmetrics-in: agent_ref 는 필수입니다")
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

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다.
func (n *SysMetricsInNode) AgentRef() flow.AgentRef {
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// initAgent 는 AgentResolver 로 에이전트를 resolve 하고 MessageReceiver 를 확인한다.
func (n *SysMetricsInNode) initAgent(ctx context.Context) error {
	if n.resolver == nil {
		return fmt.Errorf("sysmetrics-in: AgentResolver 미주입")
	}
	n.mu.RLock()
	ref := n.agentRef
	n.mu.RUnlock()

	transport, err := n.resolver.ResolveAgent(ctx, flow.AgentRef{AgentID: ref, AgentName: ref})
	if err != nil {
		return fmt.Errorf("sysmetrics-in: agent resolve 실패: %w", err)
	}

	var resolved agent.Agent
	if accessor, ok := transport.(AgentAccessor); ok {
		resolved = accessor.UnderlyingAgent()
	}
	recv, ok := resolved.(agent.MessageReceiver)
	if !ok {
		return fmt.Errorf("sysmetrics-in: 에이전트가 MessageReceiver 를 구현하지 않음")
	}

	// 락 함정 회피: Name() 은 n.mu 를 잡기 전에 1회 호출한다.
	agentName := resolved.Name()

	n.mu.Lock()
	n.transport = transport
	n.agent = resolved
	n.receiver = recv
	n.agentName = agentName
	n.mu.Unlock()
	return nil
}

// Init 은 노드를 초기화하고 수신 루프를 시작한다.
func (n *SysMetricsInNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.initAgent(ctx); err != nil {
		return err
	}
	go n.receiveLoop()
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// receiveLoop 는 에이전트로부터 일괄 레코드를 수신하여 message 로 빌드해
// sourceCh 에 전달한다.
//
// stopCh 은 반드시 "이 루프 세대"의 채널이어야 한다 — Reinit 이 교체하므로 매 반복마다
// n.stopCh 을 다시 읽으면 구 루프가 새(열린) 채널을 보고 종료하지 못해 누수 +
// 이중 소비가 발생한다(chirpstack-in 과 같은 함정).
func (n *SysMetricsInNode) receiveLoop() {
	n.mu.RLock()
	a := n.agent
	recv := n.receiver
	opts := n.emitMetadata
	agentName := n.agentName
	stopCh := n.stopCh
	n.mu.RUnlock()

	var throttle receiveFailureThrottle

	for {
		select {
		case <-stopCh:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), sysMetricsReceiveTimeout)
		data, err := recv.ReceiveMessage(ctx)
		cancel()
		if err != nil || data == nil {
			// 5초 타임아웃은 정상 유휴이므로 실패로 계상하지 않는다(throttle 이 판별).
			if count, emit := throttle.note(err, time.Now()); emit {
				if lg := n.Logger(); lg != nil {
					lg.Warn("sysmetrics-in: 수신 실패 지속",
						"node_id", n.ID(),
						"consecutive_failures", count,
						"error", err,
					)
				}
			}
			continue
		}
		throttle.reset()

		msg, ok := buildSysMetricsMessage(data, n.ID(), a, agentName, opts)
		if !ok {
			continue
		}

		select {
		case n.sourceCh <- msg:
		case <-stopCh:
			return
		}
	}
}

// buildSysMetricsMessage 는 일괄 레코드 JSON 을 flow 메시지로 옮긴다.
//
// 레코드 형상은 에이전트의 `SysMetricsBatch` 와 1:1 이다
// (measurement / time_ms / fields). measurement 는 항상 `sysmetrics` 하나이고,
// 값들은 payload 에 그룹별로 중첩되어 들어간다:
//
//	$.payload.cpu.usage_percent
//	$.payload.memory.used_bytes
//	$.payload.storage["/data"].used_bytes
//	$.payload.network.en0.bytes_recv
func buildSysMetricsMessage(
	data []byte,
	nodeID string,
	a agent.Agent,
	agentName string,
	opts MetadataEmitOptions,
) (message.Message, bool) {
	var rec struct {
		Measurement string         `json:"measurement"`
		TimeMs      int64          `json:"time_ms"`
		Fields      map[string]any `json:"fields"`
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, false
	}
	// 측정 이름이 없으면 저장 키를 만들 수 없다 — 조용히 버린다.
	if rec.Measurement == "" {
		return nil, false
	}
	// 값이 하나도 없는 표본은 저장할 것이 없다. 빈 메시지를 흘려보내면 하류가
	// 빈 레코드를 쌓는다.
	if len(rec.Fields) == 0 {
		return nil, false
	}

	msg := message.New()
	msg.SetType("event")
	if rec.TimeMs > 0 {
		msg.SetTimestamp(time.UnixMilli(rec.TimeMs))
	}
	msg.Metadata().Set("measurement", rec.Measurement)
	if opts.NodeID {
		msg.Metadata().Set("node_id", nodeID)
	}
	emitAgentGroup(msg, a, opts)
	_ = agentName // 에이전트 이름은 emitAgentGroup 이 opts 에 따라 싣는다.

	// 그룹을 통째로 옮긴다 — 중첩 구조를 여기서 펴면 payload 형상이 에이전트가
	// 정한 것과 달라져, 하류 참조 경로가 두 곳에서 갈린다.
	for group, value := range rec.Fields {
		msg.Payload().Set(group, value)
	}
	return msg, true
}

// Process 는 수신 전용 노드이므로 입력 메시지를 그대로 통과시킨다.
func (n *SysMetricsInNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	return []message.Message{msg}, nil
}

// SourceCh 는 수신된 메시지를 전달하는 채널을 반환한다.
func (n *SysMetricsInNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// Shutdown 은 노드를 종료한다.
func (n *SysMetricsInNode) Shutdown(_ context.Context) error {
	n.stopCurrentLoop()
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// stopCurrentLoop 은 현재 세대의 receiveLoop 에 종료를 신호한다.
func (n *SysMetricsInNode) stopCurrentLoop() {
	n.mu.Lock()
	n.stopOnce.Do(func() {
		close(n.stopCh)
	})
	n.mu.Unlock()
}

// Reinit 은 에이전트 재시작(새 인스턴스로 교체) 후 노드를 재초기화한다.
//
// 순서가 중요하다: 기존 receiveLoop 종료 → agent/receiver 재해석 →
// stopCh / stopOnce 재생성 → 새 receiveLoop 기동. stopCh 을 재생성하지 않으면
// 새 루프가 첫 select 에서 즉시 종료된다(구 stopCh 은 이미 close 된 상태).
func (n *SysMetricsInNode) Reinit(ctx context.Context) error {
	n.stopCurrentLoop()

	if err := n.initAgent(ctx); err != nil {
		return err
	}

	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.stopOnce = sync.Once{}
	n.mu.Unlock()

	go n.receiveLoop()
	return nil
}
