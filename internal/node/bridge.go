package node

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// AgentResolver 는 에이전트 참조를 실제 AgentTransport로 해석하는 인터페이스이다.
// 실제 구현은 engine/agent 패키지에서 제공한다.
type AgentResolver interface {
	ResolveAgent(ctx context.Context, ref flow.AgentRef) (AgentTransport, error)
}

// AgentTransport 는 에이전트와의 통신을 담당하는 인터페이스이다.
type AgentTransport interface {
	Send(ctx context.Context, msg message.Message) error
	Receive(ctx context.Context) (message.Message, error)
}

// BridgeNode 는 에이전트와 플로우 간의 다리 역할을 하는 노드이다.
// 4가지 모드(in, out, inout, request_reply)를 지원한다.
// P0 모듈(BridgeConfig, BridgeTransformer, CorrelationTracker, bridgeStatsCollector)을 통합하여
// 설정 관리, 메시지 변환, 상관관계 추적, 통계 수집 기능을 제공한다.
type BridgeNode struct {
	*BaseNode
	agentRef     flow.AgentRef
	bridgeConfig BridgeConfig          // 브릿지 통합 설정 (direction, timeout 등 포함)
	resolver     AgentResolver
	transport    AgentTransport
	transformer  BridgeTransformer     // 메시지 변환기
	correlation  *CorrelationTracker   // 요청-응답 상관관계 추적기
	stats        *bridgeStatsCollector // 브릿지 통계 수집기
	recvCh       chan message.Message  // 수신 버퍼 (In/InOut 모드용)
	cancelFn     context.CancelFunc   // 수신 루프 및 클린업 루프 취소 함수
	connected    atomic.Bool          // 에이전트 연결 상태
	mu           sync.RWMutex
}

// WithAgentResolver 는 BridgeNode에 AgentResolver를 설정하는 옵션을 반환한다.
func WithAgentResolver(resolver AgentResolver) NodeOption {
	return func(b *BaseNode) {
		// resolver는 BridgeNode 생성 후 적용되므로, config에 저장
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config["_agent_resolver"] = resolver
	}
}

// WithReplyTimeout 은 BridgeNode의 요청-응답 타임아웃을 설정하는 옵션을 반환한다.
// 기존 호환성을 유지하며, 내부적으로 BridgeConfig.RequestTimeout에 반영된다.
func WithReplyTimeout(d time.Duration) NodeOption {
	return func(b *BaseNode) {
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config["_reply_timeout"] = d
	}
}

// WithBridgeConfig 는 BridgeNode에 BridgeConfig를 설정하는 옵션을 반환한다.
// WithReplyTimeout보다 우선 적용되며, 전체 설정을 한 번에 지정할 수 있다.
func WithBridgeConfig(cfg BridgeConfig) NodeOption {
	return func(b *BaseNode) {
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config["_bridge_config"] = cfg
	}
}

// NewBridgeNode 는 새로운 BridgeNode를 생성하는 팩토리 함수이다.
// NodeDef에 AgentRef가 없으면 에러를 반환한다.
func NewBridgeNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	if def.AgentRef == nil {
		return nil, &NodeError{
			NodeID:   def.ID,
			NodeType: def.Type,
			Err:      ErrAgentNotFound,
		}
	}

	base := NewBaseNode(def, opts...)

	// 기본 BridgeConfig 생성
	cfg := DefaultBridgeConfig(*def.AgentRef)

	n := &BridgeNode{
		BaseNode:     base,
		agentRef:     *def.AgentRef,
		bridgeConfig: cfg,
		transformer:  NewDefaultTransformer(),
		stats:        newBridgeStatsCollector(),
	}

	// 옵션에서 설정값 추출
	if base.config != nil {
		// BridgeConfig 옵션이 있으면 먼저 적용 (가장 높은 우선순위)
		if c, ok := base.config["_bridge_config"]; ok {
			if bridgeCfg, ok := c.(BridgeConfig); ok {
				n.bridgeConfig = bridgeCfg
			}
		}

		// AgentResolver 추출
		if r, ok := base.config["_agent_resolver"]; ok {
			if resolver, ok := r.(AgentResolver); ok {
				n.resolver = resolver
			}
		}

		// ReplyTimeout 추출 (WithBridgeConfig가 없을 때만 적용)
		if t, ok := base.config["_reply_timeout"]; ok {
			if timeout, ok := t.(time.Duration); ok {
				// _bridge_config가 명시적으로 설정되지 않았을 때만 timeout을 덮어쓴다
				if _, hasBridgeCfg := base.config["_bridge_config"]; !hasBridgeCfg {
					n.bridgeConfig.RequestTimeout = timeout
				}
			}
		}
	}

	// RequestReply 모드일 때 CorrelationTracker 초기화
	if n.bridgeConfig.Direction == flow.BridgeRequestReply {
		n.correlation = NewCorrelationTracker(n.bridgeConfig.RequestTimeout)
	}

	// 수신 버퍼 초기화
	bufSize := n.bridgeConfig.BufferSize
	if bufSize < 1 {
		bufSize = 256 // 안전한 기본값
	}
	n.recvCh = make(chan message.Message, bufSize)

	return n, nil
}

// Init 은 BridgeNode를 초기화한다.
// AgentResolver가 설정되어 있으면 에이전트를 해석하여 transport를 설정한다.
// BridgeIn/BridgeInOut 모드에서는 수신 루프 고루틴을 시작하고,
// BridgeRequestReply 모드에서는 상관관계 클린업 루프를 시작한다.
func (n *BridgeNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if n.resolver == nil {
		return &NodeError{
			NodeID:   n.ID(),
			NodeType: n.Type(),
			Err:      ErrAgentNotFound,
		}
	}

	transport, err := n.resolver.ResolveAgent(ctx, n.agentRef)
	if err != nil {
		return &NodeError{
			NodeID:   n.ID(),
			NodeType: n.Type(),
			Err:      err,
		}
	}
	// Transport에 PayloadFormat 설정 (지원하는 경우)
	if setter, ok := transport.(PayloadFormatSetter); ok {
		format := n.bridgeConfig.Transform.PayloadFormat
		if format == "" {
			format = PayloadFormatAuto
		}
		setter.SetPayloadFormat(format)
	}

	n.mu.Lock()
	n.transport = transport
	n.mu.Unlock()

	// 연결 상태 설정
	n.connected.Store(true)

	// 내부 컨텍스트 생성 (수신 루프 및 클린업 루프용)
	loopCtx, cancel := context.WithCancel(context.Background())
	n.cancelFn = cancel

	// BridgeIn/BridgeInOut 모드: 수신 루프 고루틴 시작
	direction := n.bridgeConfig.Direction
	if direction == flow.BridgeIn || direction == flow.BridgeInOut {
		n.startReceiveLoop(loopCtx)
	}

	// BridgeRequestReply 모드: 상관관계 클린업 루프 시작
	if direction == flow.BridgeRequestReply && n.correlation != nil {
		n.correlation.StartCleanupLoop(loopCtx)
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 방향에 따라 메시지를 처리한다.
// BridgeOut: 플로우 메시지를 변환 검증 후 에이전트에 전송한다.
// BridgeIn/BridgeInOut: 메시지를 통과시킨다 (수신 루프에서 비동기 수신 처리).
// BridgeRequestReply: CorrelationTracker를 사용하여 요청-응답 패턴을 처리한다.
func (n *BridgeNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	transport := n.transport
	direction := n.bridgeConfig.Direction
	n.mu.RUnlock()

	switch direction {
	case flow.BridgeOut:
		// 플로우 -> 에이전트: 변환 검증 후 메시지를 에이전트에 전송
		start := time.Now()

		// 변환 검증 (바이트 데이터로 변환 가능한지 확인)
		if _, err := n.transformer.FlowToAgent(msg); err != nil {
			n.stats.RecordTransformError()
			return nil, fmt.Errorf("%w: %v", ErrTransformFailed, err)
		}

		if transport != nil {
			if err := transport.Send(ctx, msg); err != nil {
				return nil, err
			}
		}

		// 통계 기록
		n.stats.RecordToAgent()
		n.stats.RecordRelay(time.Since(start))

		return []message.Message{}, nil

	case flow.BridgeIn, flow.BridgeInOut:
		// 에이전트 -> 플로우: 외부에서 수신 처리, Process는 통과
		// 수신 루프 고루틴이 transport.Receive()를 폴링하여 recvCh에 전달한다.
		n.stats.RecordFromAgent()
		n.stats.RecordRelay(0)
		return []message.Message{msg}, nil

	case flow.BridgeRequestReply:
		// 요청-응답 패턴: CorrelationTracker를 사용하여 메시지 전송 후 응답 대기
		if transport == nil {
			return nil, ErrAgentNotFound
		}

		start := time.Now()

		correlationID := uuid.New().String()
		msg.Metadata().Set("_correlationID", correlationID)

		replyCh := make(chan message.Message, 1)
		n.correlation.Track(correlationID, replyCh)

		// 변환 검증
		if _, err := n.transformer.FlowToAgent(msg); err != nil {
			n.stats.RecordTransformError()
			return nil, fmt.Errorf("%w: %v", ErrTransformFailed, err)
		}

		if err := transport.Send(ctx, msg); err != nil {
			return nil, err
		}
		n.stats.RecordToAgent()

		// 타임아웃 컨텍스트 생성
		timeoutCtx, cancel := context.WithTimeout(ctx, n.bridgeConfig.RequestTimeout)
		defer cancel()

		// 응답 수신 시도
		reply, err := transport.Receive(timeoutCtx)
		if err != nil {
			n.stats.RecordCorrelationTimeout()
			return nil, ErrRequestTimeout
		}

		// 상관관계 해소 (응답 채널에서 제거)
		n.correlation.Resolve(correlationID, reply)

		n.stats.RecordFromAgent()
		n.stats.RecordRelay(time.Since(start))

		return []message.Message{reply}, nil

	default:
		return []message.Message{msg}, nil
	}
}

// Shutdown 은 BridgeNode를 종료한다.
// 수신 루프와 클린업 루프를 취소하고, CorrelationTracker를 닫는다.
func (n *BridgeNode) Shutdown(_ context.Context) error {
	// 수신 루프 및 클린업 루프 취소
	if n.cancelFn != nil {
		n.cancelFn()
	}

	// CorrelationTracker 정리
	if n.correlation != nil {
		n.correlation.Close()
	}

	// 연결 상태 해제
	n.connected.Store(false)

	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 BridgeNode의 설정을 적용한다.
// config에 "payload_format" 키가 있으면 수신 데이터의 변환 방식을 설정한다.
func (n *BridgeNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}
	if config == nil {
		return nil
	}

	// payload_format 설정 추출
	if format, ok := config["payload_format"]; ok {
		if f, ok := format.(string); ok {
			n.bridgeConfig.Transform.PayloadFormat = f
		}
	}

	return nil
}

// Info 는 BridgeNode의 현재 런타임 상태 정보 스냅샷을 반환한다.
func (n *BridgeNode) Info() BridgeInfo {
	pendingCorrelations := 0
	if n.correlation != nil {
		pendingCorrelations = n.correlation.PendingCount()
	}

	return BridgeInfo{
		AgentID:   n.agentRef.AgentID,
		AgentName: n.agentRef.AgentName,
		Direction: n.bridgeConfig.Direction,
		Connected: n.connected.Load(),
		Stats:     n.stats.Snapshot(pendingCorrelations),
	}
}

// Stats 는 BridgeNode의 현재 통계 스냅샷을 반환한다.
func (n *BridgeNode) Stats() BridgeStatsSnapshot {
	pendingCorrelations := 0
	if n.correlation != nil {
		pendingCorrelations = n.correlation.PendingCount()
	}
	return n.stats.Snapshot(pendingCorrelations)
}

// SourceCh 는 수신 버퍼 채널을 반환한다 (SourceNode 인터페이스 구현).
// BridgeIn/BridgeInOut 모드에서 startReceiveLoop가 이 채널에 메시지를 전달하며,
// 엔진은 이 채널에서 메시지를 읽어 출력 와이어로 전달한다.
func (n *BridgeNode) SourceCh() <-chan message.Message {
	return n.recvCh
}

// startReceiveLoop 은 transport로부터 메시지를 폴링하여 recvCh에 전달하는 고루틴을 시작한다.
// BridgeIn 또는 BridgeInOut 모드에서 Init 시점에 호출된다.
// 컨텍스트가 취소되면 루프가 종료된다.
func (n *BridgeNode) startReceiveLoop(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n.mu.RLock()
			transport := n.transport
			n.mu.RUnlock()

			if transport == nil {
				return
			}

			// transport로부터 메시지 수신
			received, err := transport.Receive(ctx)
			if err != nil {
				// 컨텍스트 취소인 경우 종료
				if ctx.Err() != nil {
					return
				}
				// 그 외 에러는 무시하고 재시도
				continue
			}

			// 수신한 메시지를 변환 (AgentToFlow 는 바이트 기반이므로, 현재는 직접 전달)
			// 향후 바이트 기반 transport에서 활용할 수 있도록 transformer를 유지한다.
			n.stats.RecordFromAgent()

			// 버퍼에 메시지 전달
			select {
			case n.recvCh <- received:
				// 성공적으로 버퍼에 전달
			case <-ctx.Done():
				return
			}
		}
	}()
}
