package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/lg"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 상수 정의
// ---------------------------------------------------------------------------

const (
	// 기본 타임아웃
	lgapDefaultTimeout = 5 * time.Second

	// 기본 폴링 간격
	lgapDefaultPollInterval = 30 * time.Second

	// 기본 LGAP 커맨드 (v0.7.1: get_all_states → get_all, 5 HVAC 노드 명령 통일).
	// 이전 get_all_states 는 에이전트 측 deprecation alias.
	lgapCmdGetState    = "get_state"
	lgapCmdGetAllState = "get_all"
	lgapCmdSetMultiple = "set_multiple"
)

// ---------------------------------------------------------------------------
// LGAPNodeConfig
// ---------------------------------------------------------------------------

// LGAPNodeConfig 는 LGAP 노드 공용 설정 구조체이다.
type LGAPNodeConfig struct {
	AgentRef     string `json:"agent_ref"`     // 대상 LG LGAP Agent 이름/ID (필수)
	DeviceID     string `json:"device_id"`     // 대상 디바이스 ID (선택, 빈 문자열이면 get_all_states)
	PollInterval string `json:"poll_interval"` // 폴링 간격 (선택, SourceNode 전용, 기본값 "30s")
	Timeout      string `json:"timeout"`       // Process 호출 타임아웃 (선택, 기본값 "5s")
}

// ---------------------------------------------------------------------------
// lgapNodeBase
// ---------------------------------------------------------------------------

// lgapNodeBase 는 LGAP 노드 공통 기반 구조체이다.
// LGAPStatusNode, LGAPControlNode, LGAPNode가 이를 임베딩한다.
type lgapNodeBase struct {
	*BaseNode
	lgapCfg   LGAPNodeConfig
	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent   // 원본 LG LGAP Agent 객체
	timeout   time.Duration // Process 호출 타임아웃
	mu        sync.RWMutex  // 설정 보호 뮤텍스
}

// configure 는 공통 설정 파싱을 수행한다. agent_ref(필수)를 검증한다.
func (nb *lgapNodeBase) configure(config map[string]any) error {
	if err := nb.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg LGAPNodeConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrLGAPMissingAgentRef
	}

	// device_id (선택)
	if v, ok := config["device_id"]; ok {
		if s, ok := v.(string); ok {
			cfg.DeviceID = s
		}
	}

	// poll_interval (선택, 기본값 "30s")
	cfg.PollInterval = "30s"
	if v, ok := config["poll_interval"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.PollInterval = s
		}
	}

	// timeout (선택, 기본값 "5s")
	cfg.Timeout = "5s"
	if v, ok := config["timeout"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.Timeout = s
		}
	}

	// 타임아웃 파싱
	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		timeout = lgapDefaultTimeout
	}

	nb.mu.Lock()
	nb.lgapCfg = cfg
	nb.timeout = timeout
	nb.mu.Unlock()

	return nil
}

// initAgent 는 AgentResolver를 통해 에이전트를 resolve하고 LG LGAP 타입을 확인한다.
func (nb *lgapNodeBase) initAgent(ctx context.Context) error {
	if nb.resolver == nil {
		return ErrLGAPNoResolver
	}

	nb.mu.RLock()
	agentRef := nb.lgapCfg.AgentRef
	nb.mu.RUnlock()

	ref := flow.AgentRef{
		AgentID:   agentRef,
		AgentName: agentRef,
	}
	transport, err := nb.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("lgap init: agent resolve failed: %w", err)
	}
	nb.transport = transport

	// AgentAccessor를 통해 원본 Agent 객체 획득 및 타입 확인
	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return ErrLGAPAgentNotLGAP
	}

	underlyingAgent := accessor.UnderlyingAgent()
	switch underlyingAgent.(type) {
	case *lg.LGAPAgent:
		nb.agent = underlyingAgent
	default:
		return ErrLGAPAgentNotLGAP
	}

	return nil
}

// callAgentProcess 는 Agent.Process()를 context timeout과 함께 호출한다.
func (nb *lgapNodeBase) callAgentProcess(ctx context.Context, cmdBytes []byte) ([]byte, error) {
	if nb.agent == nil {
		return nil, ErrLGAPNoResolver
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
		return nil, fmt.Errorf("lgap: %w", timeoutCtx.Err())
	case result := <-ch:
		return result.data, result.err
	}
}

// shutdown 은 공통 종료 로직을 수행한다.
func (nb *lgapNodeBase) shutdown() error {
	return nb.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (nb *lgapNodeBase) AgentRef() flow.AgentRef {
	nb.mu.RLock()
	ref := nb.lgapCfg.AgentRef
	nb.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// applyLGAPOverrides 는 입력 메시지 payload에서 device_id, timeout을 오버라이드한다.
func applyLGAPOverrides(msg message.Message, cfg LGAPNodeConfig) LGAPNodeConfig {
	if v, ok := msg.Payload().Get("device_id"); ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.DeviceID = s
		}
	}
	if v, ok := msg.Payload().Get("timeout"); ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.Timeout = s
		}
	}
	return cfg
}

// ===========================================================================
// LGAPStatusNode
// ===========================================================================

// LGAPStatusNode 는 LG LGAP 에이전트의 상태를 조회하는 노드이다.
// SourceNode 인터페이스를 구현하여 폴링 기반 자체 메시지 생성을 지원한다.
type LGAPStatusNode struct {
	lgapNodeBase
	pollInterval time.Duration
	sourceCh     chan message.Message
	stopCh       chan struct{}
	pollOnce     sync.Once // stopCh close 보호
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*LGAPStatusNode)(nil)
	_ SourceNode = (*LGAPStatusNode)(nil)
)

// NewLGAPStatusNode 는 새로운 LGAPStatusNode를 생성하는 팩토리 함수이다.
func NewLGAPStatusNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &LGAPStatusNode{
		lgapNodeBase: lgapNodeBase{
			BaseNode: base,
		},
		sourceCh: make(chan message.Message, 64),
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

// Configure 는 LGAPStatusNode의 설정을 적용한다.
func (n *LGAPStatusNode) Configure(config map[string]any) error {
	if err := n.lgapNodeBase.configure(config); err != nil {
		return err
	}

	// poll_interval 파싱
	n.mu.RLock()
	pollStr := n.lgapCfg.PollInterval
	n.mu.RUnlock()

	pollInterval, err := time.ParseDuration(pollStr)
	if err != nil {
		pollInterval = lgapDefaultPollInterval
	}
	n.pollInterval = pollInterval

	return nil
}

// Init 은 LGAPStatusNode를 초기화한다.
// 에이전트를 resolve하고, 폴링 고루틴을 시작한다.
func (n *LGAPStatusNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.lgapNodeBase.initAgent(ctx); err != nil {
		// 에러 분류:
		// - ErrLGAPNoResolver: 구성 오류, 플로우 시작 실패
		// - ErrLGAPAgentNotLGAP: 구성 오류 (agent 타입 불일치), 플로우 시작 실패
		// - 기타 (agent not found): 런타임 가용성 문제, 플로우는 진행하되 노드 대기
		if errors.Is(err, ErrLGAPNoResolver) || errors.Is(err, ErrLGAPAgentNotLGAP) {
			return err // 구성 오류 전파
		}

		// 에이전트를 찾을 수 없어도 플로우는 시작되도록 함 (나중에 Reinit으로 연결)
		// 로거를 통해 경고만 출력
		if logger := n.Logger(); logger != nil {
			logger.Warn("lgap init: agent not available, deferring connection",
				"nodeID", n.ID(),
				"agentRef", n.lgapCfg.AgentRef,
				"error", err,
			)
		}
		// 노드가 Running 상태로 진행하지만, 아직 폴링 루프를 시작하지 않음
		// (agent, transport는 nil 상태)
		return n.BaseNode.TransitionTo(lifecycle.StateRunning)
	}

	// 폴링 고루틴 시작 (SourceNode 지원)
	go n.pollLoop()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// pollLoop 는 설정된 간격으로 상태를 조회하여 sourceCh에 메시지를 전달한다.
func (n *LGAPStatusNode) pollLoop() {
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			n.mu.RLock()
			cfg := n.lgapCfg
			n.mu.RUnlock()

			cmdBytes, err := buildLGAPStatusCommand(cfg)
			if err != nil {
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
			resp, err := n.lgapNodeBase.callAgentProcess(ctx, cmdBytes)
			cancel()

			if err != nil {
				continue
			}

			var result map[string]any
			if err := json.Unmarshal(resp, &result); err != nil {
				continue
			}

			msg := message.New()
			// v0.7.14: payload 내부의 metadata 그룹을 message metadata 로 promote.
			promotePayloadMetadata(msg, result)
			// v0.8.0: payload.trigger → metadata.message_type. trigger 없으면 "poll" fallback.
			applyDeviceStateMessageType(msg, result, "poll")
			for k, v := range result {
				msg.Payload().Set(k, v)
			}
			msg.Metadata().Set("lgap_source", "poll")
			msg.Metadata().Set("lgap_node_id", n.ID())

			select {
			case n.sourceCh <- msg:
			default:
				// 채널이 가득 차면 드롭
			}
		}
	}
}

// Process 는 입력 메시지를 받아 상태 조회를 수행하고 결과를 반환한다.
func (n *LGAPStatusNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.lgapCfg
	n.mu.RUnlock()

	// 메시지 payload에서 오버라이드 적용
	cfg = applyLGAPOverrides(msg, cfg)

	cmdBytes, err := buildLGAPStatusCommand(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGAPProcessFailed, err)
	}

	resp, err := n.lgapNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGAPProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrLGAPProcessFailed, err)
	}

	out := msg.Clone()
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	out.Metadata().Set("lgap_source", "request")
	out.Metadata().Set("lgap_node_id", n.ID())
	out.Metadata().Set("message_type", "device_state.response")

	return []message.Message{out}, nil
}

// Shutdown 은 LGAPStatusNode를 종료한다. 폴링 고루틴을 정지한다.
func (n *LGAPStatusNode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})
	return n.lgapNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조를 갱신하고 폴링 루프를
// 재시작한다. LGAP 는 FrameNotifier 미지원이지만, 일관된 lifecycle 관리를 위해
// 동일한 stop-restart 패턴을 따른다.
func (n *LGAPStatusNode) Reinit(ctx context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})

	if err := n.lgapNodeBase.initAgent(ctx); err != nil {
		return err
	}

	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.pollOnce = sync.Once{}
	n.mu.Unlock()

	go n.pollLoop()
	return nil
}

// SourceCh 는 폴링으로 생성된 메시지를 수신하는 채널을 반환한다.
func (n *LGAPStatusNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// LGAPControlNode
// ===========================================================================

// LGAPControlNode 는 LG LGAP 에이전트에 제어 명령을 전송하는 노드이다.
type LGAPControlNode struct {
	lgapNodeBase
}

// 인터페이스 컴파일 체크
var _ Node = (*LGAPControlNode)(nil)

// NewLGAPControlNode 는 새로운 LGAPControlNode를 생성하는 팩토리 함수이다.
func NewLGAPControlNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &LGAPControlNode{
		lgapNodeBase: lgapNodeBase{
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

// Configure 는 LGAPControlNode의 설정을 적용한다.
func (n *LGAPControlNode) Configure(config map[string]any) error {
	return n.lgapNodeBase.configure(config)
}

// Init 은 LGAPControlNode를 초기화한다. 에이전트를 resolve한다.
func (n *LGAPControlNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.lgapNodeBase.initAgent(ctx); err != nil {
		// 에러 분류:
		// - ErrLGAPNoResolver: 구성 오류, 플로우 시작 실패
		// - ErrLGAPAgentNotLGAP: 구성 오류 (agent 타입 불일치), 플로우 시작 실패
		// - 기타 (agent not found): 런타임 가용성 문제, 플로우는 진행하되 노드 대기
		if errors.Is(err, ErrLGAPNoResolver) || errors.Is(err, ErrLGAPAgentNotLGAP) {
			return err // 구성 오류 전파
		}

		// 에이전트를 찾을 수 없어도 플로우는 시작되도록 함 (나중에 Reinit으로 연결)
		// 로거를 통해 경고만 출력
		if logger := n.Logger(); logger != nil {
			logger.Warn("lgap control init: agent not available, deferring connection",
				"nodeID", n.ID(),
				"agentRef", n.lgapCfg.AgentRef,
				"error", err,
			)
		}
		// 노드가 Running 상태로 진행 (agent, transport는 nil 상태)
		return n.BaseNode.TransitionTo(lifecycle.StateRunning)
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 입력 메시지의 payload에서 제어 명령을 추출하여 Agent에 전달한다.
// payload에 "command" 키가 있으면 직접 명령으로 처리하고,
// 없으면 제어 키(power, mode, temperature, fan_speed)에서 set_multiple을 구성한다.
func (n *LGAPControlNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.lgapCfg
	n.mu.RUnlock()

	cfg = applyLGAPOverrides(msg, cfg)

	cmdBytes, err := buildLGAPControlCommand(msg, cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGAPProcessFailed, err)
	}

	resp, err := n.lgapNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGAPProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrLGAPProcessFailed, err)
	}

	out := msg.Clone()
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	out.Metadata().Set("lgap_command", "control")
	out.Metadata().Set("lgap_node_id", n.ID())
	out.Metadata().Set("message_type", "device_state.response")

	return []message.Message{out}, nil
}

// Shutdown 은 LGAPControlNode를 종료한다.
func (n *LGAPControlNode) Shutdown(_ context.Context) error {
	return n.lgapNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조를 갱신한다.
// 고루틴이 없는 process-only 노드이므로 initAgent 만 재호출한다.
func (n *LGAPControlNode) Reinit(ctx context.Context) error {
	return n.lgapNodeBase.initAgent(ctx)
}

// ===========================================================================
// LGAPNode
// ===========================================================================

// LGAPNode 는 LG LGAP 에이전트의 상태 조회와 제어를 모두 수행하는 통합 노드이다.
// SourceNode 인터페이스를 구현하여 폴링 기반 자체 메시지 생성도 지원한다.
// payload에 제어 키(power, mode, temperature, fan_speed)가 있으면 제어,
// 없으면 상태 조회로 자동 감지한다.
type LGAPNode struct {
	lgapNodeBase
	pollInterval time.Duration
	sourceCh     chan message.Message
	stopCh       chan struct{}
	pollOnce     sync.Once
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*LGAPNode)(nil)
	_ SourceNode = (*LGAPNode)(nil)
)

// NewLGAPNode 는 새로운 LGAPNode를 생성하는 팩토리 함수이다.
func NewLGAPNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &LGAPNode{
		lgapNodeBase: lgapNodeBase{
			BaseNode: base,
		},
		sourceCh: make(chan message.Message, 64),
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

// Configure 는 LGAPNode의 설정을 적용한다.
func (n *LGAPNode) Configure(config map[string]any) error {
	if err := n.lgapNodeBase.configure(config); err != nil {
		return err
	}

	// poll_interval 파싱
	n.mu.RLock()
	pollStr := n.lgapCfg.PollInterval
	n.mu.RUnlock()

	pollInterval, err := time.ParseDuration(pollStr)
	if err != nil {
		pollInterval = lgapDefaultPollInterval
	}
	n.pollInterval = pollInterval

	return nil
}

// Init 은 LGAPNode를 초기화한다.
// 에이전트를 resolve하고, 폴링 고루틴을 시작한다.
func (n *LGAPNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.lgapNodeBase.initAgent(ctx); err != nil {
		// 에러 분류:
		// - ErrLGAPNoResolver: 구성 오류, 플로우 시작 실패
		// - ErrLGAPAgentNotLGAP: 구성 오류 (agent 타입 불일치), 플로우 시작 실패
		// - 기타 (agent not found): 런타임 가용성 문제, 플로우는 진행하되 노드 대기
		if errors.Is(err, ErrLGAPNoResolver) || errors.Is(err, ErrLGAPAgentNotLGAP) {
			return err // 구성 오류 전파
		}

		// 에이전트를 찾을 수 없어도 플로우는 시작되도록 함 (나중에 Reinit으로 연결)
		// 로거를 통해 경고만 출력
		if logger := n.Logger(); logger != nil {
			logger.Warn("lgap source init: agent not available, deferring connection",
				"nodeID", n.ID(),
				"agentRef", n.lgapCfg.AgentRef,
				"error", err,
			)
		}
		// 노드가 Running 상태로 진행하지만, 아직 폴링 루프를 시작하지 않음
		// (agent, transport는 nil 상태)
		return n.BaseNode.TransitionTo(lifecycle.StateRunning)
	}

	// 폴링 고루틴 시작 (SourceNode 지원)
	go n.pollLoop()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// pollLoop 는 설정된 간격으로 상태를 조회하여 sourceCh에 메시지를 전달한다.
func (n *LGAPNode) pollLoop() {
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			n.mu.RLock()
			cfg := n.lgapCfg
			n.mu.RUnlock()

			cmdBytes, err := buildLGAPStatusCommand(cfg)
			if err != nil {
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
			resp, err := n.lgapNodeBase.callAgentProcess(ctx, cmdBytes)
			cancel()

			if err != nil {
				continue
			}

			var result map[string]any
			if err := json.Unmarshal(resp, &result); err != nil {
				continue
			}

			msg := message.New()
			// v0.7.14: payload 내부의 metadata 그룹을 message metadata 로 promote.
			promotePayloadMetadata(msg, result)
			// v0.8.0: payload.trigger → metadata.message_type. trigger 없으면 "poll" fallback.
			applyDeviceStateMessageType(msg, result, "poll")
			for k, v := range result {
				msg.Payload().Set(k, v)
			}
			msg.Metadata().Set("lgap_source", "poll")
			msg.Metadata().Set("lgap_node_id", n.ID())

			select {
			case n.sourceCh <- msg:
			default:
			}
		}
	}
}

// Process 는 입력 메시지를 받아 자동으로 상태 조회 또는 제어를 수행한다.
// payload에 제어 키(power, mode, temperature, fan_speed)가 있으면 제어 명령,
// 없으면 상태 조회 명령을 전송한다.
func (n *LGAPNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.lgapCfg
	n.mu.RUnlock()

	cfg = applyLGAPOverrides(msg, cfg)

	var cmdBytes []byte
	var err error
	var cmdType string

	if hasLGAPControlKeys(msg) {
		cmdBytes, err = buildLGAPControlCommand(msg, cfg)
		cmdType = "control"
	} else {
		cmdBytes, err = buildLGAPStatusCommand(cfg)
		cmdType = "status"
	}

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGAPProcessFailed, err)
	}

	resp, err := n.lgapNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGAPProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrLGAPProcessFailed, err)
	}

	out := msg.Clone()
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	out.Metadata().Set("lgap_command", cmdType)
	out.Metadata().Set("lgap_node_id", n.ID())
	out.Metadata().Set("message_type", "device_state.response")

	return []message.Message{out}, nil
}

// Shutdown 은 LGAPNode를 종료한다. 폴링 고루틴을 정지한다.
func (n *LGAPNode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})
	return n.lgapNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조를 갱신하고 폴링 루프를
// 재시작한다 (LGAPStatusNode.Reinit 과 동일한 패턴).
func (n *LGAPNode) Reinit(ctx context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})

	if err := n.lgapNodeBase.initAgent(ctx); err != nil {
		return err
	}

	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.pollOnce = sync.Once{}
	n.mu.Unlock()

	go n.pollLoop()
	return nil
}

// SourceCh 는 폴링으로 생성된 메시지를 수신하는 채널을 반환한다.
func (n *LGAPNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// 헬퍼 함수
// ===========================================================================

// lgapControlKeys 는 LGAP 제어 명령으로 인식되는 payload 키 목록이다.
var lgapControlKeys = []string{"power", "mode", "temperature", "fan_speed"}

// hasLGAPControlKeys 는 메시지 payload에 제어 키가 하나라도 있는지 확인한다.
func hasLGAPControlKeys(msg message.Message) bool {
	for _, key := range lgapControlKeys {
		if _, ok := msg.Payload().Get(key); ok {
			return true
		}
	}
	return false
}

// buildLGAPStatusCommand 는 상태 조회용 JSON 커맨드를 생성한다.
// device_id가 설정되어 있으면 get_state, 없으면 get_all_states를 사용한다.
func buildLGAPStatusCommand(cfg LGAPNodeConfig) ([]byte, error) {
	cmd := map[string]any{}

	if cfg.DeviceID != "" {
		cmd["command"] = lgapCmdGetState
		cmd["device_id"] = cfg.DeviceID
	} else {
		cmd["command"] = lgapCmdGetAllState
	}

	return json.Marshal(cmd)
}

// buildLGAPControlCommand 는 제어용 JSON 커맨드를 생성한다.
// payload에 "command" 키가 있으면 직접 커맨드로 전달하고,
// 없으면 제어 키(power, mode, temperature, fan_speed)를 수집하여 set_multiple을 구성한다.
func buildLGAPControlCommand(msg message.Message, cfg LGAPNodeConfig) ([]byte, error) {
	// payload에 "command" 키가 있으면 직접 전달
	if v, ok := msg.Payload().Get("command"); ok {
		if cmdStr, ok := v.(string); ok && cmdStr != "" {
			cmd := map[string]any{
				"command": cmdStr,
			}
			if cfg.DeviceID != "" {
				cmd["device_id"] = cfg.DeviceID
			}
			// payload에서 params 추출
			if params, ok := msg.Payload().Get("params"); ok {
				cmd["params"] = params
			}
			return json.Marshal(cmd)
		}
	}

	// 제어 키에서 set_multiple 구성
	settings := map[string]any{}
	for _, key := range lgapControlKeys {
		if v, ok := msg.Payload().Get(key); ok {
			settings[key] = v
		}
	}

	if len(settings) == 0 {
		// 제어 키가 없으면 상태 조회로 폴백
		return buildLGAPStatusCommand(cfg)
	}

	cmd := map[string]any{
		"command":  lgapCmdSetMultiple,
		"settings": settings,
	}
	if cfg.DeviceID != "" {
		cmd["device_id"] = cfg.DeviceID
	}

	return json.Marshal(cmd)
}
