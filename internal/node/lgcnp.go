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
// LGCNP 노드 에러 변수
// ---------------------------------------------------------------------------

var (
	// ErrLGCNPMissingAgentRef 는 agent_ref 설정이 없을 때 반환된다.
	ErrLGCNPMissingAgentRef = errors.New("lgcnp node: agent_ref is required")

	// ErrLGCNPNoResolver 는 AgentResolver가 설정되지 않았을 때 반환된다.
	ErrLGCNPNoResolver = errors.New("lgcnp node: agent resolver not set")

	// ErrLGCNPAgentNotLGCNP 는 resolve된 Agent가 LGCNP 타입이 아닐 때 반환된다.
	ErrLGCNPAgentNotLGCNP = errors.New("lgcnp node: agent is not a LGCNP agent")

	// ErrLGCNPProcessFailed 는 Agent Process() 호출이 실패했을 때 반환된다.
	ErrLGCNPProcessFailed = errors.New("lgcnp node: process command failed")
)

// ---------------------------------------------------------------------------
// 상수 정의
// ---------------------------------------------------------------------------

const (
	lgcnpDefaultTimeout           = 5 * time.Second
	lgcnpDefaultPollInterval      = 100 * time.Millisecond
	lgcnpMinPollInterval          = 1 * time.Millisecond
	lgcnpDefaultInactivityTimeout = 90 * time.Second // v0.18.24: 기본 inactivity-fallback 시간
	lgcnpMinInactivityTimeout     = 5 * time.Second

	lgcnpCmdGetStats  = "get_stats"
	lgcnpCmdGetRecent = "get_recent"
	lgcnpCmdGetAll    = "get_all"
	lgcnpCmdGetState  = "get_state"
	lgcnpCmdDrain     = "drain"
)

// ---------------------------------------------------------------------------
// LGCNPNodeConfig
// ---------------------------------------------------------------------------

// LGCNPNodeConfig 는 LGCNP 노드 공용 설정 구조체이다.
type LGCNPNodeConfig struct {
	AgentRef          string `json:"agent_ref"`           // 필수: LGCNP 에이전트 이름/ID
	InactivityTimeout string `json:"inactivity_timeout"`  // v0.18.24: 에이전트 무수신 시 request_state 호출 임계값 (기본 "90s")
	PollInterval      string `json:"poll_interval"`       // (deprecated, v0.18.24 이전 호환) 폴링 간격
	Timeout           string `json:"timeout"`             // 선택: Process 타임아웃 (기본 "5s")
	PollCommand       string `json:"poll_command"`        // 선택: drain 시 사용. (deprecated 의미 — receiveLoop 가 기본)
	RecentCount       int    `json:"recent_count"`        // 선택: get_recent 시 프레임 수 (기본 10)
	BatchSize         int    `json:"batch_size"`          // 선택: drain 시 벌크 수신 수량 (기본 32)
	OmitStateWhenOff  bool   `json:"omit_state_when_off"` // v0.18.0: power=false 시 current_temperature/mode/fan_speed 제거

	// EmitMetadata 는 metadata 옵션 필드의 emit 정책을 제어한다 (v0.18.8).
	// device_id / unit_id 는 항상 emit (필수), 나머지는 default OFF.
	EmitMetadata MetadataEmitOptions `json:"emit_metadata"`
}

// ---------------------------------------------------------------------------
// lgcnpNodeBase
// ---------------------------------------------------------------------------

// lgcnpNodeBase 는 LGCNP 노드 공통 기반 구조체이다.
type lgcnpNodeBase struct {
	*BaseNode
	lgcnpCfg  LGCNPNodeConfig
	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent
	timeout   time.Duration
	mu        sync.RWMutex
}

// configure 는 공통 설정 파싱을 수행한다.
func (nb *lgcnpNodeBase) configure(config map[string]any) error {
	if err := nb.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg LGCNPNodeConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrLGCNPMissingAgentRef
	}

	// inactivity_timeout (v0.18.24, 기본 "90s") — receiveLoop 의 무수신 fallback 임계.
	cfg.InactivityTimeout = "90s"
	if v, ok := config["inactivity_timeout"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.InactivityTimeout = s
		}
	}

	// poll_interval (deprecated, 기본 "100ms") — 호환 유지만, 본 fix 이후 사용 안 함.
	cfg.PollInterval = "100ms"
	if v, ok := config["poll_interval"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.PollInterval = s
		}
	}

	// timeout (기본 "5s")
	cfg.Timeout = "5s"
	if v, ok := config["timeout"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.Timeout = s
		}
	}

	// poll_command (기본 "drain")
	cfg.PollCommand = lgcnpCmdDrain
	if v, ok := config["poll_command"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.PollCommand = s
		}
	}

	// recent_count (기본 10)
	cfg.RecentCount = 10
	if v, ok := config["recent_count"]; ok {
		switch n := v.(type) {
		case int:
			if n > 0 {
				cfg.RecentCount = n
			}
		case float64:
			if int(n) > 0 {
				cfg.RecentCount = int(n)
			}
		}
	}

	// batch_size (기본 32)
	cfg.BatchSize = 32
	if v, ok := config["batch_size"]; ok {
		switch n := v.(type) {
		case int:
			if n > 0 {
				cfg.BatchSize = n
			}
		case float64:
			if int(n) > 0 {
				cfg.BatchSize = int(n)
			}
		}
	}

	// v0.18.0: omit_state_when_off — power=false 시 불확실 상태 필드 제거.
	if v, ok := config["omit_state_when_off"].(bool); ok {
		cfg.OmitStateWhenOff = v
	}

	// v0.18.8: emit_metadata — metadata 옵션 필드 emit 정책.
	parseEmitMetadata(config, &cfg.EmitMetadata)

	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		timeout = lgcnpDefaultTimeout
	}

	nb.mu.Lock()
	nb.lgcnpCfg = cfg
	nb.timeout = timeout
	nb.mu.Unlock()

	return nil
}

// initAgent 는 AgentResolver를 통해 에이전트를 resolve하고 LGCNP 타입을 확인한다.
func (nb *lgcnpNodeBase) initAgent(ctx context.Context) error {
	if nb.resolver == nil {
		return ErrLGCNPNoResolver
	}

	nb.mu.RLock()
	agentRef := nb.lgcnpCfg.AgentRef
	nb.mu.RUnlock()

	ref := flow.AgentRef{
		AgentID:   agentRef,
		AgentName: agentRef,
	}
	transport, err := nb.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("lgcnp init: agent resolve failed: %w", err)
	}
	nb.transport = transport

	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return ErrLGCNPAgentNotLGCNP
	}

	underlyingAgent := accessor.UnderlyingAgent()
	switch underlyingAgent.(type) {
	case *lg.LGCNPAgent:
		nb.agent = underlyingAgent
	default:
		return ErrLGCNPAgentNotLGCNP
	}

	return nil
}

// callAgentProcess 는 Agent.Process()를 context timeout과 함께 호출한다.
func (nb *lgcnpNodeBase) callAgentProcess(ctx context.Context, cmdBytes []byte) ([]byte, error) {
	if nb.agent == nil {
		return nil, ErrLGCNPNoResolver
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
		return nil, fmt.Errorf("lgcnp: %w", timeoutCtx.Err())
	case result := <-ch:
		return result.data, result.err
	}
}

// shutdown 은 공통 종료 로직을 수행한다.
func (nb *lgcnpNodeBase) shutdown() error {
	return nb.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (nb *lgcnpNodeBase) AgentRef() flow.AgentRef {
	nb.mu.RLock()
	ref := nb.lgcnpCfg.AgentRef
	nb.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// ===========================================================================
// LGCNPStatusNode — 상태 조회 전용 (SourceNode)
// ===========================================================================

// LGCNPStatusNode 는 LG LGCNP-01 에이전트의 상태를 조회하는 노드이다.
//
// v0.18.24 (2026-05-27) 동작 모델 변경:
//   - 이전: ticker 기반 폴링 (pollInterval 마다 agent 에 get_recent/drain 요청).
//   - 현재: receiveLoop — FrameNotifyCh 신호 수신 시 ring buffer drain (delta).
//     inactivityTimeout 동안 무수신 시에만 agent 에 "request_state" 명령 →
//     agent 가 각 디바이스의 마지막 상태를 push 경로로 emit → notify 수신 →
//     drain 으로 흐름 복귀.
type LGCNPStatusNode struct {
	lgcnpNodeBase
	inactivityTimeout time.Duration
	sourceCh          chan message.Message
	stopCh            chan struct{}
	pollOnce          sync.Once
	lastSeq           int64
}

var (
	_ Node       = (*LGCNPStatusNode)(nil)
	_ SourceNode = (*LGCNPStatusNode)(nil)
)

// NewLGCNPStatusNode 는 새로운 LGCNPStatusNode를 생성한다.
func NewLGCNPStatusNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &LGCNPStatusNode{
		lgcnpNodeBase: lgcnpNodeBase{
			BaseNode: base,
		},
		sourceCh: make(chan message.Message, 64),
		stopCh:   make(chan struct{}),
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

// Configure 는 LGCNPStatusNode의 설정을 적용한다.
func (n *LGCNPStatusNode) Configure(config map[string]any) error {
	if err := n.lgcnpNodeBase.configure(config); err != nil {
		return err
	}

	n.mu.RLock()
	timeoutStr := n.lgcnpCfg.InactivityTimeout
	n.mu.RUnlock()

	inactivity, err := time.ParseDuration(timeoutStr)
	if err != nil {
		inactivity = lgcnpDefaultInactivityTimeout
	}
	if inactivity < lgcnpMinInactivityTimeout {
		inactivity = lgcnpMinInactivityTimeout
	}
	n.inactivityTimeout = inactivity

	return nil
}

// Init 은 LGCNPStatusNode를 초기화한다.
func (n *LGCNPStatusNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.lgcnpNodeBase.initAgent(ctx); err != nil {
		return err
	}
	go n.receiveLoop()
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// receiveLoop 는 에이전트의 push 프레임을 수신하여 sourceCh 로 전달한다.
//
// v0.18.24 (2026-05-27) 새 모델:
//   - FrameNotifyCh 신호 → drain 으로 ring buffer 의 새 frame (lastSeq 이후) 을
//     sourceCh 로 전달.
//   - inactivityTimeout 동안 무신호 → "request_state" 를 agent 에 발송.
//     agent 가 각 디바이스 마지막 상태를 push 경로로 emit → notify 신호 →
//     drain 으로 메시지 수신.
//   - 첫 진입 시점에도 즉시 1회 drain (기존 ring buffer 의 frame 흡수).
func (n *LGCNPStatusNode) receiveLoop() {
	var notifyCh <-chan struct{}
	if fn, ok := n.agent.(agent.FrameNotifier); ok {
		notifyCh = fn.FrameNotifyCh()
	}

	timer := time.NewTimer(n.inactivityTimeout)
	defer timer.Stop()

	resetTimer := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(n.inactivityTimeout)
	}

	// 첫 진입: 이전에 누적된 frame 이 있을 수 있으므로 drain.
	n.mu.RLock()
	cfg := n.lgcnpCfg
	n.mu.RUnlock()
	n.drainNewFrames(cfg)

	for {
		select {
		case <-n.stopCh:
			return
		case <-notifyCh:
			n.mu.RLock()
			cfg := n.lgcnpCfg
			n.mu.RUnlock()
			n.drainNewFrames(cfg)
			resetTimer()
		case <-timer.C:
			n.mu.RLock()
			cfg := n.lgcnpCfg
			n.mu.RUnlock()
			n.requestStateRefresh(cfg)
			resetTimer()
		}
	}
}

// requestStateRefresh 는 에이전트에 "request_state" 를 보내 각 디바이스의
// 마지막 상태를 push 경로로 emit 하게 한다. agent 가 emit 한 frame 은 ring
// buffer + msgCh 에 들어가고 FrameNotifyCh 신호가 발생하므로, 후속 select 가
// notify case 로 들어가 자동으로 drain 된다.
func (n *LGCNPStatusNode) requestStateRefresh(cfg LGCNPNodeConfig) {
	cmdBytes, err := json.Marshal(map[string]any{
		"command": "request_state",
		"node_id": n.ID(),
	})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()
	_, _ = n.lgcnpNodeBase.callAgentProcess(ctx, cmdBytes)
}

// drainNewFrames 는 ring buffer 의 lastSeq 이후 새 frame 을 sourceCh 로 전달한다.
// agent push 경로 (notifyLoop / handleFrame / request_state) 의 모든 emit 을
// 동일한 delta 로 처리하므로 중복 emit 없이 흐름 보장.
func (n *LGCNPStatusNode) drainNewFrames(cfg LGCNPNodeConfig) {
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 32
	}

	cmdBytes, err := json.Marshal(map[string]any{
		"command":  "get_recent",
		"count":    batchSize,
		"node_id":  n.ID(),
		"last_seq": n.lastSeq,
	})
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	resp, err := n.lgcnpNodeBase.callAgentProcess(ctx, cmdBytes)
	cancel()
	if err != nil {
		return
	}

	var result struct {
		Count   int               `json:"count"`
		Frames  []json.RawMessage `json:"frames"`
		LastSeq int64             `json:"last_seq"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return
	}

	// 노드는 오래된 것부터 sourceCh 로 전달하기 위해 역순 순회한다.
	for i := len(result.Frames) - 1; i >= 0; i-- {
		var payload map[string]any
		if err := json.Unmarshal(result.Frames[i], &payload); err != nil {
			continue
		}
		msg := message.New()
		promotePayloadMetadata(msg, payload, cfg.EmitMetadata)
		applyDeviceStateMessageType(msg, payload, "poll")
		promoteDevIDWithUUID(msg, payload, cfg.AgentRef, cfg.EmitMetadata)
		promoteLastSeenToTimestamp(msg, payload)
		flattenStateToPayload(payload)
		applyPowerOffFilter(payload, cfg.OmitStateWhenOff)
		for k, v := range payload {
			msg.Payload().Set(k, v)
		}
		if cfg.EmitMetadata.NodeSource {
			msg.Metadata().Set("node_source", "push")
		}
		if cfg.EmitMetadata.NodeID {
			msg.Metadata().Set("node_id", n.ID())
		}

		select {
		case n.sourceCh <- msg:
		case <-n.stopCh:
			return
		}
	}

	if result.LastSeq > n.lastSeq {
		n.lastSeq = result.LastSeq
	}
}

// Process 는 입력 메시지를 받아 상태 조회를 수행한다.
func (n *LGCNPStatusNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.lgcnpCfg
	n.mu.RUnlock()

	cmdBytes, err := buildLGCNPStatusCommand(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGCNPProcessFailed, err)
	}

	resp, err := n.lgcnpNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGCNPProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrLGCNPProcessFailed, err)
	}

	out := msg.Clone()

	// v0.12.0: payload schema promotion (dev_id → metadata, last_seen_ms → timestamp, nested metadata).

	promotePayloadMetadata(out, result, cfg.EmitMetadata)

	promoteDevIDWithUUID(out, result, cfg.AgentRef, cfg.EmitMetadata)

	promoteLastSeenToTimestamp(out, result)

	flattenStateToPayload(result)
	// v0.18.0: power=false 시 신뢰할 수 없는 상태 필드 제거.
	applyPowerOffFilter(result, cfg.OmitStateWhenOff)
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	if cfg.EmitMetadata.NodeID {
		out.Metadata().Set("node_id", n.ID())
	}
	// v0.10.0: lgcnp_source="request" 제거 (message_type="device_state.response" 와 중복).
	out.SetType("device_state.response")

	return []message.Message{out}, nil
}

// Shutdown 은 LGCNPStatusNode를 종료한다.
func (n *LGCNPStatusNode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})
	return n.lgcnpNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 FrameNotifier 채널 구독을
// 재구성한다. receiveLoop 가 nb.agent 와 FrameNotifyCh() 를 고루틴 시작 시 한
// 번 캡처하므로 수신 고루틴을 종료한 뒤 새 stopCh / pollOnce 로 재시작한다.
func (n *LGCNPStatusNode) Reinit(ctx context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})

	if err := n.lgcnpNodeBase.initAgent(ctx); err != nil {
		return err
	}

	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.pollOnce = sync.Once{}
	n.mu.Unlock()

	go n.receiveLoop()
	return nil
}

// SourceCh 는 폴링으로 생성된 메시지 채널을 반환한다.
func (n *LGCNPStatusNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// LGCNPControlNode — 제어 전용 (미지원 플레이스홀더)
// ===========================================================================

// LGCNPControlNode 는 LGCNP-01 제어 노드이다 (미지원, 항상 not_supported 반환).
type LGCNPControlNode struct {
	lgcnpNodeBase
}

var _ Node = (*LGCNPControlNode)(nil)

// NewLGCNPControlNode 는 새로운 LGCNPControlNode를 생성한다.
func NewLGCNPControlNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &LGCNPControlNode{
		lgcnpNodeBase: lgcnpNodeBase{
			BaseNode: base,
		},
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

// Configure 는 LGCNPControlNode의 설정을 적용한다.
func (n *LGCNPControlNode) Configure(config map[string]any) error {
	return n.lgcnpNodeBase.configure(config)
}

// Init 은 LGCNPControlNode를 초기화한다.
func (n *LGCNPControlNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.lgcnpNodeBase.initAgent(ctx); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 제어 명령을 처리한다 — LGCNP-01은 제어 미지원이므로 항상 not_supported.
func (n *LGCNPControlNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	out := msg.Clone()
	out.Payload().Set("status", "not_supported")
	out.Payload().Set("message", "LGCNP-01 protocol does not support control commands")
	out.Metadata().Set("lgcnp_command", "control")
	if n.lgcnpCfg.EmitMetadata.NodeID {
		out.Metadata().Set("node_id", n.ID())
	}
	out.SetType("device_state.response")
	return []message.Message{out}, nil
}

// Shutdown 은 LGCNPControlNode를 종료한다.
func (n *LGCNPControlNode) Shutdown(_ context.Context) error {
	return n.lgcnpNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조를 갱신한다.
// 고루틴이 없는 process-only 노드이므로 initAgent 만 재호출한다.
func (n *LGCNPControlNode) Reinit(ctx context.Context) error {
	return n.lgcnpNodeBase.initAgent(ctx)
}

// ===========================================================================
// LGCNPNode — 상태 조회 + 제어 통합
// ===========================================================================

// LGCNPNode 는 LGCNP-01 상태 조회와 제어를 모두 수행하는 통합 노드이다.
// 제어 요청 시에는 not_supported를 반환한다.
//
// v0.18.24 (2026-05-27) 동작 모델: LGCNPStatusNode 와 동일. receiveLoop +
// inactivity timer + request_state fallback.
type LGCNPNode struct {
	lgcnpNodeBase
	inactivityTimeout time.Duration
	sourceCh          chan message.Message
	stopCh            chan struct{}
	pollOnce          sync.Once
	lastSeq           int64
}

var (
	_ Node       = (*LGCNPNode)(nil)
	_ SourceNode = (*LGCNPNode)(nil)
)

// NewLGCNPNode 는 새로운 LGCNPNode를 생성한다.
func NewLGCNPNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &LGCNPNode{
		lgcnpNodeBase: lgcnpNodeBase{
			BaseNode: base,
		},
		sourceCh: make(chan message.Message, 64),
		stopCh:   make(chan struct{}),
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

// Configure 는 LGCNPNode의 설정을 적용한다.
func (n *LGCNPNode) Configure(config map[string]any) error {
	if err := n.lgcnpNodeBase.configure(config); err != nil {
		return err
	}

	n.mu.RLock()
	timeoutStr := n.lgcnpCfg.InactivityTimeout
	n.mu.RUnlock()

	inactivity, err := time.ParseDuration(timeoutStr)
	if err != nil {
		inactivity = lgcnpDefaultInactivityTimeout
	}
	if inactivity < lgcnpMinInactivityTimeout {
		inactivity = lgcnpMinInactivityTimeout
	}
	n.inactivityTimeout = inactivity

	return nil
}

// Init 은 LGCNPNode를 초기화한다.
func (n *LGCNPNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.lgcnpNodeBase.initAgent(ctx); err != nil {
		return err
	}
	go n.receiveLoop()
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// receiveLoop 는 에이전트의 push 프레임을 수신한다 (LGCNPStatusNode 와 동일 모델).
func (n *LGCNPNode) receiveLoop() {
	var notifyCh <-chan struct{}
	if fn, ok := n.agent.(agent.FrameNotifier); ok {
		notifyCh = fn.FrameNotifyCh()
	}

	timer := time.NewTimer(n.inactivityTimeout)
	defer timer.Stop()

	resetTimer := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(n.inactivityTimeout)
	}

	n.mu.RLock()
	cfg := n.lgcnpCfg
	n.mu.RUnlock()
	n.drainNewFrames(cfg)

	for {
		select {
		case <-n.stopCh:
			return
		case <-notifyCh:
			n.mu.RLock()
			cfg := n.lgcnpCfg
			n.mu.RUnlock()
			n.drainNewFrames(cfg)
			resetTimer()
		case <-timer.C:
			n.mu.RLock()
			cfg := n.lgcnpCfg
			n.mu.RUnlock()
			n.requestStateRefresh(cfg)
			resetTimer()
		}
	}
}

// requestStateRefresh 는 에이전트에 request_state 요청을 보낸다.
func (n *LGCNPNode) requestStateRefresh(cfg LGCNPNodeConfig) {
	cmdBytes, err := json.Marshal(map[string]any{
		"command": "request_state",
		"node_id": n.ID(),
	})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()
	_, _ = n.lgcnpNodeBase.callAgentProcess(ctx, cmdBytes)
}

// drainNewFrames 는 ring buffer 의 lastSeq 이후 새 frame 을 sourceCh 로 전달한다.
func (n *LGCNPNode) drainNewFrames(cfg LGCNPNodeConfig) {
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 32
	}

	cmdBytes, err := json.Marshal(map[string]any{
		"command":  "get_recent",
		"count":    batchSize,
		"node_id":  n.ID(),
		"last_seq": n.lastSeq,
	})
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	resp, err := n.lgcnpNodeBase.callAgentProcess(ctx, cmdBytes)
	cancel()
	if err != nil {
		return
	}

	var result struct {
		Count   int               `json:"count"`
		Frames  []json.RawMessage `json:"frames"`
		LastSeq int64             `json:"last_seq"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return
	}

	for i := len(result.Frames) - 1; i >= 0; i-- {
		var payload map[string]any
		if err := json.Unmarshal(result.Frames[i], &payload); err != nil {
			continue
		}
		msg := message.New()
		promotePayloadMetadata(msg, payload, cfg.EmitMetadata)
		applyDeviceStateMessageType(msg, payload, "poll")
		promoteDevIDWithUUID(msg, payload, cfg.AgentRef, cfg.EmitMetadata)
		promoteLastSeenToTimestamp(msg, payload)
		flattenStateToPayload(payload)
		applyPowerOffFilter(payload, cfg.OmitStateWhenOff)
		for k, v := range payload {
			msg.Payload().Set(k, v)
		}
		if cfg.EmitMetadata.NodeSource {
			msg.Metadata().Set("node_source", "push")
		}
		if cfg.EmitMetadata.NodeID {
			msg.Metadata().Set("node_id", n.ID())
		}

		select {
		case n.sourceCh <- msg:
		case <-n.stopCh:
			return
		}
	}

	if result.LastSeq > n.lastSeq {
		n.lastSeq = result.LastSeq
	}
}

// Process 는 입력 메시지를 받아 상태 조회를 수행한다.
// 제어 키가 있으면 not_supported를 반환한다.
func (n *LGCNPNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	// 제어 키 감지 → 미지원 응답
	for _, key := range []string{"power", "mode", "temperature", "fan_speed"} {
		if _, ok := msg.Payload().Get(key); ok {
			out := msg.Clone()
			out.Payload().Set("status", "not_supported")
			out.Payload().Set("message", "LGCNP-01 protocol does not support control commands")
			out.Metadata().Set("lgcnp_command", "control")
			if n.lgcnpCfg.EmitMetadata.NodeID {
				out.Metadata().Set("node_id", n.ID())
			}
			out.SetType("device_state.response")
			return []message.Message{out}, nil
		}
	}

	n.mu.RLock()
	cfg := n.lgcnpCfg
	n.mu.RUnlock()

	cmdBytes, err := buildLGCNPStatusCommand(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGCNPProcessFailed, err)
	}

	resp, err := n.lgcnpNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGCNPProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrLGCNPProcessFailed, err)
	}

	out := msg.Clone()

	// v0.12.0: payload schema promotion (dev_id → metadata, last_seen_ms → timestamp, nested metadata).

	promotePayloadMetadata(out, result, cfg.EmitMetadata)

	promoteDevIDWithUUID(out, result, cfg.AgentRef, cfg.EmitMetadata)

	promoteLastSeenToTimestamp(out, result)

	flattenStateToPayload(result)
	// v0.18.0: power=false 시 신뢰할 수 없는 상태 필드 제거.
	applyPowerOffFilter(result, cfg.OmitStateWhenOff)
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	out.Metadata().Set("lgcnp_command", "status")
	if cfg.EmitMetadata.NodeID {
		out.Metadata().Set("node_id", n.ID())
	}
	out.SetType("device_state.response")

	return []message.Message{out}, nil
}

// Shutdown 은 LGCNPNode를 종료한다.
func (n *LGCNPNode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})
	return n.lgcnpNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 FrameNotifier 채널 구독을
// 재구성한다 (LGCNPStatusNode.Reinit 과 동일한 패턴).
func (n *LGCNPNode) Reinit(ctx context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})

	if err := n.lgcnpNodeBase.initAgent(ctx); err != nil {
		return err
	}

	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.pollOnce = sync.Once{}
	n.mu.Unlock()

	go n.receiveLoop()
	return nil
}

// SourceCh 는 폴링으로 생성된 메시지 채널을 반환한다.
func (n *LGCNPNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// 헬퍼 함수
// ===========================================================================

// buildLGCNPStatusCommand 는 상태 조회용 JSON 커맨드를 생성한다.
func buildLGCNPStatusCommand(cfg LGCNPNodeConfig) ([]byte, error) {
	cmd := map[string]any{}

	switch cfg.PollCommand {
	case lgcnpCmdGetRecent:
		cmd["command"] = lgcnpCmdGetRecent
		cmd["count"] = cfg.RecentCount
	case lgcnpCmdGetAll:
		cmd["command"] = lgcnpCmdGetAll
	case lgcnpCmdGetState:
		cmd["command"] = lgcnpCmdGetState
	case lgcnpCmdDrain:
		cmd["command"] = lgcnpCmdDrain
		cmd["count"] = cfg.BatchSize
	default:
		cmd["command"] = lgcnpCmdGetStats
	}

	return json.Marshal(cmd)
}
