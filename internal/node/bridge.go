package node

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/agent"
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

// AgentAccessor 는 AgentTransport에서 원본 Agent에 접근하기 위한 인터페이스이다.
type AgentAccessor interface {
	UnderlyingAgent() agent.Agent
}

// BridgeNode 는 에이전트와 플로우 간의 다리 역할을 하는 노드이다.
// 4가지 모드(in, out, inout, request_reply)를 지원한다.
// P0 모듈(BridgeConfig, BridgeTransformer, CorrelationTracker, bridgeStatsCollector)을 통합하여
// 설정 관리, 메시지 변환, 상관관계 추적, 통계 수집 기능을 제공한다.
type BridgeNode struct {
	*BaseNode
	agentRef     flow.AgentRef
	bridgeConfig BridgeConfig // 브릿지 통합 설정 (direction, timeout 등 포함)
	resolver     AgentResolver
	transport    AgentTransport
	transformer  BridgeTransformer     // 메시지 변환기
	adapter      BridgeAdapter         // 에이전트 타입별 전용 어댑터
	correlation  *CorrelationTracker   // 요청-응답 상관관계 추적기
	stats        *bridgeStatsCollector // 브릿지 통계 수집기
	recvCh       chan message.Message  // 수신 버퍼 (In/InOut 모드용)
	cancelFn     context.CancelFunc    // 수신 루프 및 클린업 루프 취소 함수
	connected    atomic.Bool           // 에이전트 연결 상태
	mu           sync.RWMutex
	bridgeTopics []string   // Bridge가 추가한 토픽 목록 (config + runtime)
	topicsMu     sync.Mutex // bridgeTopics 동시성 보호
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

	// 에이전트 타입별 어댑터 조회
	slog.Debug("bridge: 어댑터 조회 시작",
		"node", n.ID(),
		"name", n.Name(),
		"agentRef", n.agentRef.AgentName,
		"transportType", fmt.Sprintf("%T", transport),
		"isAgentAccessor", func() bool { _, ok := transport.(AgentAccessor); return ok }(),
	)
	if accessor, ok := transport.(AgentAccessor); ok {
		ag := accessor.UnderlyingAgent()
		agentType := ag.Type()
		slog.Debug("bridge: 어댑터 조회",
			"node", n.ID(),
			"name", n.Name(),
			"agentType", agentType,
		)
		if adapter, found := GetAdapter(agentType); found {
			slog.Debug("bridge: 어댑터 발견",
				"node", n.ID(),
				"agentType", agentType,
			)
			// AgentConfigurable 지원 시 에이전트 설정으로 per-agent 어댑터 생성
			if configurable, ok := adapter.(AgentConfigurable); ok {
				agentOpts := ag.Info().Config.Transport.Options
				slog.Debug("bridge: AgentConfigurable 확인",
					"node", n.ID(),
					"hasOptions", agentOpts != nil,
					"options", agentOpts,
				)
				if agentOpts != nil {
					configured, err := configurable.ConfigureFromAgent(agentOpts)
					if err != nil {
						return &NodeError{
							NodeID:   n.ID(),
							NodeType: n.Type(),
							Err:      fmt.Errorf("adapter configure: %w", err),
						}
					}
					adapter = configured
				}
			}
			// BridgeConfigurable 지원 시 Bridge 설정으로 per-bridge 어댑터 생성
			if bridgeCfg, ok := adapter.(BridgeConfigurable); ok {
				configured, err := bridgeCfg.ConfigureFromBridge(n.bridgeConfig)
				if err != nil {
					return &NodeError{
						NodeID:   n.ID(),
						NodeType: n.Type(),
						Err:      fmt.Errorf("adapter bridge configure: %w", err),
					}
				}
				adapter = configured
			}
			if err := adapter.Validate(n.bridgeConfig); err != nil {
				slog.Warn("bridge: 어댑터 검증 실패, 어댑터 없이 진행",
					"node", n.ID(),
					"error", err,
				)
			} else {
				n.adapter = adapter
				n.transformer = NewAdapterTransformerBridge(adapter)
				slog.Info("bridge: 어댑터 설정 완료",
					"node", n.ID(),
					"agentType", agentType,
				)
			}
		} else {
			slog.Debug("bridge: 어댑터 미등록",
				"node", n.ID(),
				"agentType", agentType,
			)
		}
	}

	// 연결 상태 설정
	n.connected.Store(true)

	// SubscriberAgent 토픽 구독 처리
	if len(n.bridgeConfig.Topics) > 0 {
		if accessor, ok := transport.(AgentAccessor); ok {
			if subscriber, ok := accessor.UnderlyingAgent().(agent.SubscriberAgent); ok {
				if err := subscriber.Subscribe(ctx, n.bridgeConfig.Topics); err != nil {
					// Subscribe 실패 시 경고 로그, Init은 계속 진행
					slog.Warn("bridge: 토픽 구독 실패 (Init 계속 진행)",
						"node", n.ID(),
						"topics", n.bridgeConfig.Topics,
						"error", err,
					)
				}
				n.topicsMu.Lock()
				n.bridgeTopics = append(n.bridgeTopics, n.bridgeConfig.Topics...)
				n.topicsMu.Unlock()
			} else {
				slog.Warn("bridge: 에이전트가 SubscriberAgent 인터페이스를 구현하지 않음",
					"node", n.ID(),
				)
			}
		}
	}

	// 내부 컨텍스트 생성 및 수신/폴링 루프 시작
	n.startLoops()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// startLoops 는 내부 컨텍스트를 생성하고 방향에 따라 수신/폴링/클린업 루프를 시작한다.
// Init과 Reinit에서 공통으로 사용된다.
func (n *BridgeNode) startLoops() {
	loopCtx, cancel := context.WithCancel(context.Background())
	n.cancelFn = cancel

	direction := n.bridgeConfig.Direction

	// BridgeIn/BridgeInOut 모드: 폴링 또는 수신 루프 시작
	if direction == flow.BridgeIn || direction == flow.BridgeInOut {
		slog.Debug("bridge: 어댑터 타입 체크",
			"node", n.ID(),
			"name", n.Name(),
			"adapterNil", n.adapter == nil,
			"adapterType", fmt.Sprintf("%T", n.adapter),
		)
		if n.adapter != nil {
			_, isPollable := n.adapter.(PollableAdapter)
			_, isMultiMsg := n.adapter.(MultiMessagePollAdapter)
			_, isCmdPoll := n.adapter.(CommandPollAdapter)
			slog.Debug("bridge: 어댑터 인터페이스 구현 확인",
				"node", n.ID(),
				"PollableAdapter", isPollable,
				"MultiMessagePollAdapter", isMultiMsg,
				"CommandPollAdapter", isCmdPoll,
			)
		}

		n.mu.RLock()
		transport := n.transport
		n.mu.RUnlock()

		// PollableAdapter가 있으면 브릿지 주도 폴링, 아니면 기존 수신 루프
		if pollable, ok := n.adapter.(PollableAdapter); ok && len(pollable.ReadSpecs()) > 0 {
			pollInterval := n.getPollingIntervalOverride()
			if pollInterval <= 0 {
				pollInterval = 1 * time.Second
			}
			n.startBridgePollLoop(loopCtx, pollable, pollInterval)

			// 에이전트가 MessageReceiver를 구현하면 비동기 변경 이벤트 수신을 위해
			// 수신 루프도 함께 시작한다.
			if accessor, ok := transport.(AgentAccessor); ok {
				if _, ok := accessor.UnderlyingAgent().(agent.MessageReceiver); ok {
					n.startReceiveLoop(loopCtx)
				}
			}
		} else if multiPoller, ok := n.adapter.(MultiMessagePollAdapter); ok {
			pollInterval := n.getPollingIntervalOverride()
			if pollInterval <= 0 {
				pollInterval = 5 * time.Second
			}
			n.startMultiMessagePollLoop(loopCtx, multiPoller, pollInterval)

			if accessor, ok := transport.(AgentAccessor); ok {
				if _, ok := accessor.UnderlyingAgent().(agent.MessageReceiver); ok {
					n.startReceiveLoop(loopCtx)
				}
			}
		} else if cmdPoller, ok := n.adapter.(CommandPollAdapter); ok {
			pollInterval := n.getPollingIntervalOverride()
			if pollInterval <= 0 {
				pollInterval = 5 * time.Second
			}
			n.startCommandPollLoop(loopCtx, cmdPoller, pollInterval)

			if accessor, ok := transport.(AgentAccessor); ok {
				if _, ok := accessor.UnderlyingAgent().(agent.MessageReceiver); ok {
					n.startReceiveLoop(loopCtx)
				}
			}
		} else {
			// PollableAdapter가 아닌 경우 기존 에이전트 폴링 간격 설정
			if accessor, ok := transport.(AgentAccessor); ok {
				if pollOverride := n.getPollingIntervalOverride(); pollOverride > 0 {
					ag := accessor.UnderlyingAgent()
					if pollCfg, ok := ag.(agent.PollingConfigurable); ok {
						if err := pollCfg.SetPollInterval(pollOverride); err != nil {
							slog.Warn("bridge: 폴링 간격 오버라이드 실패",
								"node", n.ID(),
								"error", err,
							)
						}
					}
				}
			}
			n.startReceiveLoop(loopCtx)
		}
	}

	// BridgeRequestReply 모드: 상관관계 클린업 루프 시작
	if direction == flow.BridgeRequestReply && n.correlation != nil {
		n.correlation.StartCleanupLoop(loopCtx)
	}
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
		// 제어 메시지 확인
		if isControlMessage(msg) {
			return n.handleControlMessage(ctx, msg)
		}

		slog.Debug("bridge: BridgeOut 메시지 수신",
			"node", n.ID(),
			"name", n.Name(),
			"agent", n.agentRef.AgentName,
			"msgID", msg.ID(),
			"payload", msg.Payload().ToMap(),
		)

		// 플로우 -> 에이전트: 어댑터가 있으면 변환 결과를 직접 전달, 없으면 기존 방식 사용
		start := time.Now()

		if n.adapter != nil && transport != nil {
			// 어댑터 변환: 플로우 메시지를 에이전트 커맨드 데이터로 변환
			data, meta, err := n.adapter.TransformToAgent(msg)
			if err != nil {
				n.stats.RecordTransformError()
				slog.Warn("bridge: adapter TransformToAgent 실패",
					"node", n.ID(),
					"error", err,
				)
				return nil, fmt.Errorf("%w: %v", ErrTransformFailed, err)
			}

			// publish_topic 이 설정되어 있고 어댑터가 토픽을 지정하지 않았으면 Bridge 설정값 사용
			if meta.Topic == "" && n.bridgeConfig.PublishTopic != "" {
				meta.Topic = n.bridgeConfig.PublishTopic
			}

			// 어댑터 변환 결과를 에이전트에 직접 전달
			if accessor, ok := transport.(AgentAccessor); ok {
				ag := accessor.UnderlyingAgent()
				n.recordInternalReceived(ag)

				// MessagePublisher 지원 시 토픽/QoS 메타데이터와 함께 발행
				if pub, ok := ag.(agent.MessagePublisher); ok {
					if pubErr := pub.PublishMessage(meta.Topic, byte(meta.QoS), meta.Retained, data); pubErr != nil {
						n.recordInternalErrored(ag)
						slog.Warn("bridge: 에이전트 메시지 발행 실패",
							"node", n.ID(),
							"agent", n.agentRef.AgentName,
							"topic", meta.Topic,
							"error", pubErr,
						)
						return nil, pubErr
					}
				} else if _, procErr := ag.Process(data); procErr != nil {
					n.recordInternalErrored(ag)
					slog.Warn("bridge: 에이전트 직접 전송 실패",
						"node", n.ID(),
						"agent", n.agentRef.AgentName,
						"error", procErr,
					)
					return nil, procErr
				}
			} else {
				// AgentAccessor 미구현 시 transport.Send 폴백
				if err := transport.Send(ctx, msg); err != nil {
					slog.Warn("bridge: 에이전트 전송 실패",
						"node", n.ID(),
						"agent", n.agentRef.AgentName,
						"error", err,
					)
					return nil, err
				}
			}

			slog.Debug("bridge: 어댑터 변환 후 에이전트 전송 완료",
				"node", n.ID(),
				"agent", n.agentRef.AgentName,
			)
		} else {
			// 어댑터 없음: 변환 검증 후 MessagePublisher 또는 transport.Send 방식
			data, err := n.transformer.FlowToAgent(msg)
			if err != nil {
				n.stats.RecordTransformError()
				slog.Warn("bridge: FlowToAgent 변환 실패",
					"node", n.ID(),
					"error", err,
				)
				return nil, fmt.Errorf("%w: %v", ErrTransformFailed, err)
			}

			if transport != nil {
				// MessagePublisher 지원 시 publish_topic 과 함께 발행
				if accessor, ok := transport.(AgentAccessor); ok {
					ag := accessor.UnderlyingAgent()
					n.recordInternalReceived(ag)
					if pub, ok := ag.(agent.MessagePublisher); ok && n.bridgeConfig.PublishTopic != "" {
						if pubErr := pub.PublishMessage(n.bridgeConfig.PublishTopic, 0, false, data); pubErr != nil {
							n.recordInternalErrored(ag)
							slog.Warn("bridge: 에이전트 메시지 발행 실패",
								"node", n.ID(),
								"agent", n.agentRef.AgentName,
								"topic", n.bridgeConfig.PublishTopic,
								"error", pubErr,
							)
							return nil, pubErr
						}
						slog.Debug("bridge: MessagePublisher 발행 완료",
							"node", n.ID(),
							"agent", n.agentRef.AgentName,
							"topic", n.bridgeConfig.PublishTopic,
						)
					} else if err := transport.Send(ctx, msg); err != nil {
						n.recordInternalErrored(ag)
						slog.Warn("bridge: 에이전트 전송 실패",
							"node", n.ID(),
							"agent", n.agentRef.AgentName,
							"error", err,
						)
						return nil, err
					}
				} else if err := transport.Send(ctx, msg); err != nil {
					slog.Warn("bridge: 에이전트 전송 실패",
						"node", n.ID(),
						"agent", n.agentRef.AgentName,
						"error", err,
					)
					return nil, err
				}
				slog.Debug("bridge: 에이전트 전송 완료",
					"node", n.ID(),
					"agent", n.agentRef.AgentName,
				)
			}
		}

		// 통계 기록
		n.stats.RecordToAgent()
		n.stats.RecordRelay(time.Since(start))

		// 에이전트로 전송된 메시지도 "out" 카운트에 반영한다.
		// 출력 와이어가 없으면 엔진이 메시지를 전달하지 않으므로 안전하다.
		return []message.Message{msg}, nil

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

		// 내부 통계: 노드→에이전트 송신
		if accessor, ok := transport.(AgentAccessor); ok {
			n.recordInternalReceived(accessor.UnderlyingAgent())
		}

		if err := transport.Send(ctx, msg); err != nil {
			if accessor, ok := transport.(AgentAccessor); ok {
				n.recordInternalErrored(accessor.UnderlyingAgent())
			}
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

		// 내부 통계: 에이전트→노드 응답
		if accessor, ok := transport.(AgentAccessor); ok {
			n.recordInternalSent(accessor.UnderlyingAgent())
		}

		n.stats.RecordFromAgent()
		n.stats.RecordRelay(time.Since(start))

		// agent 노드 통일 분류 표준: request-reply 응답은 response 분류.
		reply.SetType("response")

		return []message.Message{reply}, nil

	default:
		return []message.Message{msg}, nil
	}
}

// Shutdown 은 BridgeNode를 종료한다.
// Bridge가 추가한 토픽 구독을 해제하고, 수신 루프와 클린업 루프를 취소하고, CorrelationTracker를 닫는다.
func (n *BridgeNode) Shutdown(ctx context.Context) error {
	// Bridge가 추가한 토픽 구독 해제
	n.topicsMu.Lock()
	topics := make([]string, len(n.bridgeTopics))
	copy(topics, n.bridgeTopics)
	n.bridgeTopics = nil
	n.topicsMu.Unlock()

	if len(topics) > 0 {
		n.mu.RLock()
		transport := n.transport
		n.mu.RUnlock()

		if accessor, ok := transport.(AgentAccessor); ok {
			if subscriber, ok := accessor.UnderlyingAgent().(agent.SubscriberAgent); ok {
				if err := subscriber.Unsubscribe(ctx, topics); err != nil {
					slog.Warn("bridge: Shutdown 시 토픽 구독 해제 실패",
						"node", n.ID(),
						"topics", topics,
						"error", err,
					)
				}
			}
		}
	}

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

	// topics 설정 추출 (Bridge 초기화 시 에이전트에 자동 구독 요청)
	if topicsRaw, ok := config["topics"]; ok {
		n.bridgeConfig.Topics = bridgeToStringSlice(topicsRaw)
	}

	// publish_topic 설정 추출 (BridgeOut 방향에서 발행 토픽 템플릿)
	if pt, ok := config["publish_topic"]; ok {
		if s, ok := pt.(string); ok {
			n.bridgeConfig.PublishTopic = s
		}
	}

	return nil
}

// getPollingIntervalOverride 는 노드 설정에서 폴링 간격을 추출한다.
// polling_interval_ms (밀리초) 또는 polling_interval (밀리초) 키를 확인한다.
// 값이 없거나 유효하지 않으면 0을 반환한다. 최소 100ms 이상이어야 한다.
func (n *BridgeNode) getPollingIntervalOverride() time.Duration {
	config := n.GetConfig()
	if config == nil {
		return 0
	}
	// polling_interval_ms 우선, 없으면 polling_interval 사용
	v, ok := config["polling_interval_ms"]
	if !ok {
		v, ok = config["polling_interval"]
		if !ok {
			return 0
		}
	}
	switch val := v.(type) {
	case float64:
		if val >= 100 {
			return time.Duration(val) * time.Millisecond
		}
	case int:
		if val >= 100 {
			return time.Duration(val) * time.Millisecond
		}
	case int64:
		if val >= 100 {
			return time.Duration(val) * time.Millisecond
		}
	}
	return 0
}

// ConnectedAgent 는 이 브릿지 노드에 연결된 에이전트를 반환한다.
// Init 이전이거나 transport가 AgentAccessor를 구현하지 않으면 nil을 반환한다.
func (n *BridgeNode) ConnectedAgent() agent.Agent {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if accessor, ok := n.transport.(AgentAccessor); ok {
		return accessor.UnderlyingAgent()
	}
	return nil
}

// recordInternalReceived 는 에이전트의 내부 수신 통계를 기록한다.
// 노드가 에이전트에 데이터를 전송할 때 호출한다.
func (n *BridgeNode) recordInternalReceived(ag agent.Agent) {
	if recorder, ok := ag.(agent.InternalStatsRecorder); ok {
		recorder.RecordInternalReceived(n.ID(), "")
	}
}

// recordInternalSent 는 에이전트의 내부 송신 통계를 기록한다.
// 에이전트가 노드에 데이터를 반환/전송할 때 호출한다.
func (n *BridgeNode) recordInternalSent(ag agent.Agent) {
	if recorder, ok := ag.(agent.InternalStatsRecorder); ok {
		recorder.RecordInternalSent(n.ID(), "")
	}
}

// recordInternalErrored 는 에이전트의 내부 에러 통계를 기록한다.
func (n *BridgeNode) recordInternalErrored(ag agent.Agent) {
	if recorder, ok := ag.(agent.InternalStatsRecorder); ok {
		recorder.RecordInternalErrored(n.ID(), "")
	}
}

// AgentRef 는 이 브릿지 노드가 참조하는 에이전트 정보를 반환한다.
func (n *BridgeNode) AgentRef() flow.AgentRef {
	return n.agentRef
}

// Reinit 은 에이전트 재시작 후 transport를 재연결하고 토픽을 재구독한다.
// 기존 수신 루프를 중단하고 새 transport로 재시작한다.
func (n *BridgeNode) Reinit(ctx context.Context) error {
	// 1. 기존 수신 루프 중단
	if n.cancelFn != nil {
		n.cancelFn()
	}

	// 2. transport 재연결
	transport, err := n.resolver.ResolveAgent(ctx, n.agentRef)
	if err != nil {
		n.connected.Store(false)
		return fmt.Errorf("bridge reinit: resolve agent: %w", err)
	}
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
	n.connected.Store(true)

	// 3. 어댑터 재설정
	if accessor, ok := transport.(AgentAccessor); ok {
		ag := accessor.UnderlyingAgent()
		if adapter, found := GetAdapter(ag.Type()); found {
			if configurable, ok := adapter.(AgentConfigurable); ok {
				if opts := ag.Info().Config.Transport.Options; opts != nil {
					if configured, err := configurable.ConfigureFromAgent(opts); err == nil {
						adapter = configured
					}
				}
			}
			if bridgeCfg, ok := adapter.(BridgeConfigurable); ok {
				if configured, err := bridgeCfg.ConfigureFromBridge(n.bridgeConfig); err == nil {
					adapter = configured
				}
			}
			if err := adapter.Validate(n.bridgeConfig); err == nil {
				n.adapter = adapter
				n.transformer = NewAdapterTransformerBridge(adapter)
			}
		}
	}

	// 4. 토픽 재구독
	n.topicsMu.Lock()
	topics := make([]string, len(n.bridgeTopics))
	copy(topics, n.bridgeTopics)
	n.topicsMu.Unlock()

	if len(topics) > 0 {
		if accessor, ok := transport.(AgentAccessor); ok {
			if subscriber, ok := accessor.UnderlyingAgent().(agent.SubscriberAgent); ok {
				if err := subscriber.Subscribe(ctx, topics); err != nil {
					slog.Warn("bridge reinit: 토픽 재구독 실패",
						"node", n.ID(),
						"topics", topics,
						"error", err,
					)
				} else {
					slog.Info("bridge reinit: 토픽 재구독 완료",
						"node", n.ID(),
						"topics", topics,
					)
				}
			}
		}
	}

	// 5. 수신/폴링 루프 재시작
	n.startLoops()

	slog.Info("bridge reinit: 재초기화 완료",
		"node", n.ID(),
		"agent", n.agentRef.AgentName,
		"direction", n.bridgeConfig.Direction,
		"topics", topics,
	)
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
	slog.Info("bridge: 수신 루프 시작",
		"node", n.ID(),
		"name", n.Name(),
		"agent", n.agentRef.AgentName,
		"direction", n.bridgeConfig.Direction,
		"bufSize", cap(n.recvCh),
	)
	go func() {
		for {
			select {
			case <-ctx.Done():
				slog.Debug("bridge: 수신 루프 종료 (context 취소)",
					"node", n.ID(),
				)
				return
			default:
			}

			n.mu.RLock()
			transport := n.transport
			n.mu.RUnlock()

			if transport == nil {
				slog.Warn("bridge: 수신 루프 종료 (transport nil)",
					"node", n.ID(),
				)
				return
			}

			// transport로부터 메시지 수신
			received, err := transport.Receive(ctx)
			if err != nil {
				// 컨텍스트 취소인 경우 종료
				if ctx.Err() != nil {
					return
				}
				slog.Debug("bridge: transport.Receive 에러 (재시도)",
					"node", n.ID(),
					"error", err,
				)
				// 그 외 에러는 무시하고 재시도
				continue
			}

			slog.Debug("bridge: 에이전트에서 메시지 수신",
				"node", n.ID(),
				"msgID", received.ID(),
				"payload", received.Payload().ToMap(),
			)

			// 내부 통계: 에이전트→노드 송신
			if accessor, ok := transport.(AgentAccessor); ok {
				n.recordInternalSent(accessor.UnderlyingAgent())
			}

			// 수신한 메시지를 변환 (AgentToFlow 는 바이트 기반이므로, 현재는 직접 전달)
			// 향후 바이트 기반 transport에서 활용할 수 있도록 transformer를 유지한다.
			n.stats.RecordFromAgent()

			// agent 노드 통일 분류 표준: bridge 가 transport 에서 받은 메시지는 자발적
			// agent push 이므로 event 로 분류한다. 어댑터가 이미 설정했으면 보존한다.
			// SPEC-MESSAGE-TYPE-001 § T2: metadata.message_type 키 lookup → 1급 Type() 검사.
			if received.Type() == "" {
				received.SetType("event")
			}

			// 버퍼에 메시지 전달
			select {
			case n.recvCh <- received:
				slog.Debug("bridge: recvCh 에 메시지 전달 완료",
					"node", n.ID(),
					"chLen", len(n.recvCh),
				)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// startBridgePollLoop 은 브릿지가 직접 에이전트에 read_raw 명령을 보내 폴링하는 고루틴을 시작한다.
// PollableAdapter를 구현하는 어댑터가 있을 때 startReceiveLoop 대신 호출된다.
func (n *BridgeNode) startBridgePollLoop(ctx context.Context, pollable PollableAdapter, interval time.Duration) {
	specs := pollable.ReadSpecs()
	slog.Info("bridge: 브릿지 폴 루프 시작",
		"node", n.ID(),
		"name", n.Name(),
		"agent", n.agentRef.AgentName,
		"interval", interval,
		"readSpecs", len(specs),
	)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		pollCount := 0
		for {
			select {
			case <-ctx.Done():
				slog.Debug("bridge: 폴 루프 종료 (context 취소)",
					"node", n.ID(),
					"totalPolls", pollCount,
				)
				return
			case <-ticker.C:
				pollCount++

				n.mu.RLock()
				transport := n.transport
				n.mu.RUnlock()

				if transport == nil {
					continue
				}

				accessor, ok := transport.(AgentAccessor)
				if !ok {
					continue
				}
				ag := accessor.UnderlyingAgent()

				// 각 ReadSpec에 대해 read_raw 명령 전송
				n.recordInternalReceived(ag)
				results := make([]ReadResult, 0, len(specs))
				var lastErr error
				for _, spec := range specs {
					cmd := map[string]any{
						"command": "read_raw",
						"params": map[string]any{
							"function_code": spec.FunctionCode,
							"address":       spec.StartAddr,
							"quantity":      spec.Quantity,
							"unit_id":       spec.UnitID,
						},
					}
					cmdBytes, err := json.Marshal(cmd)
					if err != nil {
						lastErr = err
						continue
					}

					respBytes, err := ag.Process(cmdBytes)
					if err != nil {
						lastErr = err
						slog.Debug("bridge: read_raw 실패",
							"node", n.ID(),
							"spec", fmt.Sprintf("FC%d addr=%d qty=%d", spec.FunctionCode, spec.StartAddr, spec.Quantity),
							"error", err,
						)
						continue
					}

					// 응답에서 base64 data 추출
					var resp struct {
						Data string `json:"data"`
					}
					if err := json.Unmarshal(respBytes, &resp); err != nil {
						lastErr = err
						continue
					}

					rawData, err := base64Decode(resp.Data)
					if err != nil {
						lastErr = err
						continue
					}

					results = append(results, ReadResult{
						Spec: spec,
						Data: rawData,
					})
				}

				if len(results) == 0 {
					if lastErr != nil {
						slog.Debug("bridge: 폴 루프 - 모든 읽기 실패",
							"node", n.ID(),
							"error", lastErr,
						)
					}
					continue
				}

				// 어댑터로 메시지 조합
				msg, err := pollable.AssembleMessage(results)
				if err != nil {
					slog.Warn("bridge: AssembleMessage 실패",
						"node", n.ID(),
						"error", err,
					)
					continue
				}

				n.recordInternalSent(ag)
				n.stats.RecordFromAgent()

				// agent 노드 통일 분류 표준: 폴링 결과는 event 분류.
				// SPEC-MESSAGE-TYPE-001 § T2: metadata.message_type 키 lookup → 1급 Type() 검사.
				if msg.Type() == "" {
					msg.SetType("event")
				}

				// recvCh에 전달
				select {
				case n.recvCh <- msg:
					slog.Debug("bridge: 폴 메시지 recvCh 전달",
						"node", n.ID(),
						"pollCount", pollCount,
					)
				case <-ctx.Done():
					return
				}
			}
		}
	}()
}

// startMultiMessagePollLoop 은 MultiMessagePollAdapter를 사용하여 다중 메시지 분리 폴링 루프를 시작한다.
// 에이전트의 Process() 응답을 디바이스별 개별 메시지로 분리하여 recvCh에 전달한다.
func (n *BridgeNode) startMultiMessagePollLoop(ctx context.Context, poller MultiMessagePollAdapter, interval time.Duration) {
	slog.Info("bridge: 멀티 메시지 폴 루프 시작",
		"node", n.ID(),
		"name", n.Name(),
		"agent", n.agentRef.AgentName,
		"interval", interval,
	)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		pollCount := 0
		for {
			select {
			case <-ctx.Done():
				slog.Debug("bridge: 멀티 메시지 폴 루프 종료",
					"node", n.ID(),
					"totalPolls", pollCount,
				)
				return
			case <-ticker.C:
				pollCount++

				n.mu.RLock()
				transport := n.transport
				n.mu.RUnlock()

				if transport == nil {
					continue
				}

				accessor, ok := transport.(AgentAccessor)
				if !ok {
					continue
				}
				ag := accessor.UnderlyingAgent()

				n.recordInternalReceived(ag)
				respBytes, err := ag.Process(poller.PollCommand())
				if err != nil {
					n.recordInternalErrored(ag)
					slog.Debug("bridge: 멀티 메시지 폴 실패",
						"node", n.ID(),
						"error", err,
					)
					continue
				}

				msgs, err := poller.AssemblePollMessages(respBytes)
				if err != nil {
					slog.Warn("bridge: AssemblePollMessages 실패",
						"node", n.ID(),
						"error", err,
					)
					continue
				}
				for _, msg := range msgs {
					n.recordInternalSent(ag)
					n.stats.RecordFromAgent()
					// SPEC-MESSAGE-TYPE-001 § T2: metadata.message_type 키 lookup → 1급 Type() 검사.
					if msg.Type() == "" {
						msg.SetType("event")
					}
					select {
					case n.recvCh <- msg:
					case <-ctx.Done():
						return
					}
				}
				slog.Info("bridge: 멀티 메시지 폴 완료",
					"node", n.ID(),
					"pollCount", pollCount,
					"messageCount", len(msgs),
				)
			}
		}
	}()
}

// startCommandPollLoop 은 CommandPollAdapter를 사용하여 JSON 명령 기반 폴링 루프를 시작한다.
// PollableAdapter의 레지스터 기반 폴링과 달리, ag.Process()를 통해 고수준 명령으로 상태를 조회한다.
func (n *BridgeNode) startCommandPollLoop(ctx context.Context, poller CommandPollAdapter, interval time.Duration) {
	slog.Info("bridge: 커맨드 폴 루프 시작",
		"node", n.ID(),
		"name", n.Name(),
		"agent", n.agentRef.AgentName,
		"interval", interval,
	)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		pollCount := 0
		for {
			select {
			case <-ctx.Done():
				slog.Debug("bridge: 커맨드 폴 루프 종료 (context 취소)",
					"node", n.ID(),
					"totalPolls", pollCount,
				)
				return
			case <-ticker.C:
				pollCount++

				n.mu.RLock()
				transport := n.transport
				n.mu.RUnlock()

				if transport == nil {
					continue
				}

				accessor, ok := transport.(AgentAccessor)
				if !ok {
					continue
				}
				ag := accessor.UnderlyingAgent()

				n.recordInternalReceived(ag)
				respBytes, err := ag.Process(poller.PollCommand())
				if err != nil {
					n.recordInternalErrored(ag)
					slog.Debug("bridge: 커맨드 폴 실패",
						"node", n.ID(),
						"error", err,
					)
					continue
				}

				// MultiMessagePollAdapter 폴백: Init 라우팅과 무관하게 다중 분리 시도
				if multiPoller, ok := poller.(MultiMessagePollAdapter); ok {
					msgs, mErr := multiPoller.AssemblePollMessages(respBytes)
					if mErr != nil {
						slog.Warn("bridge: AssemblePollMessages 폴백 실패",
							"node", n.ID(),
							"error", mErr,
						)
						continue
					}
					for _, m := range msgs {
						n.recordInternalSent(ag)
						n.stats.RecordFromAgent()
						// SPEC-MESSAGE-TYPE-001 § T2: metadata.message_type 키 lookup → 1급 Type() 검사.
						if m.Type() == "" {
							m.SetType("event")
						}
						select {
						case n.recvCh <- m:
						case <-ctx.Done():
							return
						}
					}
					slog.Info("bridge: 커맨드 폴 (멀티 메시지 폴백) 완료",
						"node", n.ID(),
						"pollCount", pollCount,
						"messageCount", len(msgs),
					)
					continue
				}

				msg, err := poller.AssemblePollMessage(respBytes)
				if err != nil {
					slog.Warn("bridge: AssemblePollMessage 실패",
						"node", n.ID(),
						"error", err,
					)
					continue
				}
				n.recordInternalSent(ag)
				n.stats.RecordFromAgent()
				// SPEC-MESSAGE-TYPE-001 § T2: metadata.message_type 키 lookup → 1급 Type() 검사.
				if msg.Type() == "" {
					msg.SetType("event")
				}
				select {
				case n.recvCh <- msg:
					slog.Debug("bridge: 커맨드 폴 메시지 recvCh 전달",
						"node", n.ID(),
						"pollCount", pollCount,
					)
				case <-ctx.Done():
					return
				}
			}
		}
	}()
}

// base64Decode 는 base64 인코딩된 문자열을 디코딩한다.
func base64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// isControlMessage 는 메시지가 제어 메시지인지 확인한다.
// payload에 "action" 키가 존재하면 제어 메시지로 판단한다.
func isControlMessage(msg message.Message) bool {
	_, ok := msg.Payload().Get("action")
	return ok
}

// handleControlMessage 는 subscribe/unsubscribe 제어 메시지를 처리한다.
func (n *BridgeNode) handleControlMessage(ctx context.Context, msg message.Message) ([]message.Message, error) {
	actionRaw, _ := msg.Payload().Get("action")
	action, ok := actionRaw.(string)
	if !ok {
		return nil, fmt.Errorf("control message: invalid action type")
	}

	topicsRaw, _ := msg.Payload().Get("topics")
	topics := bridgeToStringSlice(topicsRaw)
	if len(topics) == 0 {
		return nil, fmt.Errorf("control message: topics field is required")
	}

	// Get SubscriberAgent from transport
	n.mu.RLock()
	transport := n.transport
	n.mu.RUnlock()

	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return nil, fmt.Errorf("control message: transport does not support AgentAccessor")
	}
	subscriber, ok := accessor.UnderlyingAgent().(agent.SubscriberAgent)
	if !ok {
		return nil, fmt.Errorf("control message: agent does not implement SubscriberAgent")
	}

	switch action {
	case "subscribe":
		if err := subscriber.Subscribe(ctx, topics); err != nil {
			return nil, fmt.Errorf("control message subscribe: %w", err)
		}
		n.topicsMu.Lock()
		n.bridgeTopics = append(n.bridgeTopics, topics...)
		n.topicsMu.Unlock()
		return []message.Message{}, nil

	case "unsubscribe":
		if err := subscriber.Unsubscribe(ctx, topics); err != nil {
			return nil, fmt.Errorf("control message unsubscribe: %w", err)
		}
		n.topicsMu.Lock()
		n.bridgeTopics = bridgeRemoveTopics(n.bridgeTopics, topics)
		n.topicsMu.Unlock()
		return []message.Message{}, nil

	default:
		return nil, fmt.Errorf("control message: unknown action %q", action)
	}
}

// bridgeToStringSlice 는 interface{} 값을 []string으로 변환한다.
func bridgeToStringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		result := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	default:
		return nil
	}
}

// bridgeRemoveTopics 는 목록에서 지정된 토픽들을 제거한다.
func bridgeRemoveTopics(list []string, toRemove []string) []string {
	removeSet := make(map[string]bool, len(toRemove))
	for _, t := range toRemove {
		removeSet[t] = true
	}
	result := make([]string, 0, len(list))
	for _, t := range list {
		if !removeSet[t] {
			result = append(result, t)
		}
	}
	return result
}
