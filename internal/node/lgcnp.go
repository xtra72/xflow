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
	lgcnpDefaultTimeout      = 5 * time.Second
	lgcnpDefaultPollInterval = 100 * time.Millisecond
	lgcnpMinPollInterval     = 1 * time.Millisecond

	lgcnpCmdGetStats  = "get_stats"
	lgcnpCmdGetRecent = "get_recent"
	lgcnpCmdDrain     = "drain"
)

// ---------------------------------------------------------------------------
// LGCNPNodeConfig
// ---------------------------------------------------------------------------

// LGCNPNodeConfig 는 LGCNP 노드 공용 설정 구조체이다.
type LGCNPNodeConfig struct {
	AgentRef     string `json:"agent_ref"`     // 필수: LGCNP 에이전트 이름/ID
	PollInterval string `json:"poll_interval"` // 선택: 폴링 간격 (기본 "100ms")
	Timeout      string `json:"timeout"`       // 선택: Process 타임아웃 (기본 "5s")
	PollCommand  string `json:"poll_command"`  // 선택: 폴링 커맨드 (기본 "drain")
	RecentCount  int    `json:"recent_count"`  // 선택: get_recent 시 프레임 수 (기본 10)
	BatchSize    int    `json:"batch_size"`    // 선택: 폴링 시 벌크 수신 수량 (기본 32)
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

	// poll_interval (기본 "100ms")
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

// ===========================================================================
// LGCNPStatusNode — 상태 조회 전용 (SourceNode)
// ===========================================================================

// LGCNPStatusNode 는 LG LGCNP-01 에이전트의 상태를 조회하는 노드이다.
type LGCNPStatusNode struct {
	lgcnpNodeBase
	pollInterval time.Duration
	sourceCh     chan message.Message
	stopCh       chan struct{}
	pollOnce     sync.Once
	lastSeq      int64
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
	pollStr := n.lgcnpCfg.PollInterval
	n.mu.RUnlock()

	pollInterval, err := time.ParseDuration(pollStr)
	if err != nil {
		pollInterval = lgcnpDefaultPollInterval
	}
	if pollInterval < lgcnpMinPollInterval {
		pollInterval = lgcnpMinPollInterval
	}
	n.pollInterval = pollInterval

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
	go n.pollLoop()
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// pollLoop 는 에이전트에서 프레임을 조회하여 sourceCh에 전달한다.
func (n *LGCNPStatusNode) pollLoop() {
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	var notifyCh <-chan struct{}
	if fn, ok := n.agent.(agent.FrameNotifier); ok {
		notifyCh = fn.FrameNotifyCh()
	}

	poll := func() {
		n.mu.RLock()
		cfg := n.lgcnpCfg
		n.mu.RUnlock()

		switch cfg.PollCommand {
		case lgcnpCmdGetRecent, lgcnpCmdDrain:
			n.pollRecentBulk(cfg)
		default:
			n.pollSingle(cfg)
		}
	}

	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			poll()
		case <-notifyCh:
			poll()
		}
	}
}

func (n *LGCNPStatusNode) pollSingle(cfg LGCNPNodeConfig) {
	cmdBytes, err := buildLGCNPStatusCommand(cfg)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	resp, err := n.lgcnpNodeBase.callAgentProcess(ctx, cmdBytes)
	cancel()
	if err != nil {
		return
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return
	}

	msg := message.New()
	for k, v := range result {
		msg.Payload().Set(k, v)
	}
	msg.Metadata().Set("lgcnp_source", "poll")
	msg.Metadata().Set("lgcnp_node_id", n.ID())
	msg.Metadata().Set("message_type", "event")

	select {
	case n.sourceCh <- msg:
	default:
	}
}

func (n *LGCNPStatusNode) pollRecentBulk(cfg LGCNPNodeConfig) {
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 32
	}

	cmdBytes, err := json.Marshal(map[string]any{
		"command":  cfg.PollCommand,
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
		Count  int               `json:"count"`
		Frames []json.RawMessage `json:"frames"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return
	}

	newFrames := make([]lgcnpBulkFrame, 0, len(result.Frames))
	for i := len(result.Frames) - 1; i >= 0; i-- {
		var frame struct {
			Seq int64 `json:"seq"`
		}
		if err := json.Unmarshal(result.Frames[i], &frame); err != nil {
			continue
		}
		if frame.Seq <= n.lastSeq {
			continue
		}
		newFrames = append(newFrames, lgcnpBulkFrame{
			seq:  frame.Seq,
			data: result.Frames[i],
		})
	}

	for _, f := range newFrames {
		var payload map[string]any
		if err := json.Unmarshal(f.data, &payload); err != nil {
			continue
		}
		msg := message.New()
		for k, v := range payload {
			msg.Payload().Set(k, v)
		}
		msg.Metadata().Set("lgcnp_source", "poll_bulk")
		msg.Metadata().Set("lgcnp_node_id", n.ID())
		msg.Metadata().Set("message_type", "event")

		select {
		case n.sourceCh <- msg:
			n.lastSeq = f.seq
		default:
			return
		}
	}
}

// lgcnpBulkFrame 는 벌크 수신 시 프레임 데이터를 보관하는 내부 구조체이다.
type lgcnpBulkFrame struct {
	seq  int64
	data json.RawMessage
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
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	out.Metadata().Set("lgcnp_source", "request")
	out.Metadata().Set("lgcnp_node_id", n.ID())
	out.Metadata().Set("message_type", "response")

	return []message.Message{out}, nil
}

// Shutdown 은 LGCNPStatusNode를 종료한다.
func (n *LGCNPStatusNode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})
	return n.lgcnpNodeBase.shutdown()
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
	out.Metadata().Set("lgcnp_node_id", n.ID())
	out.Metadata().Set("message_type", "response")
	return []message.Message{out}, nil
}

// Shutdown 은 LGCNPControlNode를 종료한다.
func (n *LGCNPControlNode) Shutdown(_ context.Context) error {
	return n.lgcnpNodeBase.shutdown()
}

// ===========================================================================
// LGCNPNode — 상태 조회 + 제어 통합
// ===========================================================================

// LGCNPNode 는 LGCNP-01 상태 조회와 제어를 모두 수행하는 통합 노드이다.
// 제어 요청 시에는 not_supported를 반환한다.
type LGCNPNode struct {
	lgcnpNodeBase
	pollInterval time.Duration
	sourceCh     chan message.Message
	stopCh       chan struct{}
	pollOnce     sync.Once
	lastSeq      int64
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
	pollStr := n.lgcnpCfg.PollInterval
	n.mu.RUnlock()

	pollInterval, err := time.ParseDuration(pollStr)
	if err != nil {
		pollInterval = lgcnpDefaultPollInterval
	}
	if pollInterval < lgcnpMinPollInterval {
		pollInterval = lgcnpMinPollInterval
	}
	n.pollInterval = pollInterval

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
	go n.pollLoop()
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// pollLoop 는 에이전트에서 프레임을 조회한다.
func (n *LGCNPNode) pollLoop() {
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	var notifyCh <-chan struct{}
	if fn, ok := n.agent.(agent.FrameNotifier); ok {
		notifyCh = fn.FrameNotifyCh()
	}

	poll := func() {
		n.mu.RLock()
		cfg := n.lgcnpCfg
		n.mu.RUnlock()

		switch cfg.PollCommand {
		case lgcnpCmdGetRecent, lgcnpCmdDrain:
			n.pollRecentBulk(cfg)
		default:
			n.pollSingle(cfg)
		}
	}

	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			poll()
		case <-notifyCh:
			poll()
		}
	}
}

func (n *LGCNPNode) pollSingle(cfg LGCNPNodeConfig) {
	cmdBytes, err := buildLGCNPStatusCommand(cfg)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	resp, err := n.lgcnpNodeBase.callAgentProcess(ctx, cmdBytes)
	cancel()
	if err != nil {
		return
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return
	}

	msg := message.New()
	for k, v := range result {
		msg.Payload().Set(k, v)
	}
	msg.Metadata().Set("lgcnp_source", "poll")
	msg.Metadata().Set("lgcnp_node_id", n.ID())
	msg.Metadata().Set("message_type", "event")

	select {
	case n.sourceCh <- msg:
	default:
	}
}

func (n *LGCNPNode) pollRecentBulk(cfg LGCNPNodeConfig) {
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 32
	}

	cmdBytes, err := json.Marshal(map[string]any{
		"command":  cfg.PollCommand,
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
		Count  int               `json:"count"`
		Frames []json.RawMessage `json:"frames"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return
	}

	newFrames := make([]lgcnpBulkFrame, 0, len(result.Frames))
	for i := len(result.Frames) - 1; i >= 0; i-- {
		var frame struct {
			Seq int64 `json:"seq"`
		}
		if err := json.Unmarshal(result.Frames[i], &frame); err != nil {
			continue
		}
		if frame.Seq <= n.lastSeq {
			continue
		}
		newFrames = append(newFrames, lgcnpBulkFrame{
			seq:  frame.Seq,
			data: result.Frames[i],
		})
	}

	for _, f := range newFrames {
		var payload map[string]any
		if err := json.Unmarshal(f.data, &payload); err != nil {
			continue
		}
		msg := message.New()
		for k, v := range payload {
			msg.Payload().Set(k, v)
		}
		msg.Metadata().Set("lgcnp_source", "poll_bulk")
		msg.Metadata().Set("lgcnp_node_id", n.ID())
		msg.Metadata().Set("message_type", "event")

		select {
		case n.sourceCh <- msg:
			n.lastSeq = f.seq
		default:
			return
		}
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
			out.Metadata().Set("lgcnp_node_id", n.ID())
			out.Metadata().Set("message_type", "response")
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
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	out.Metadata().Set("lgcnp_command", "status")
	out.Metadata().Set("lgcnp_node_id", n.ID())
	out.Metadata().Set("message_type", "response")

	return []message.Message{out}, nil
}

// Shutdown 은 LGCNPNode를 종료한다.
func (n *LGCNPNode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})
	return n.lgcnpNodeBase.shutdown()
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
	case lgcnpCmdDrain:
		cmd["command"] = lgcnpCmdDrain
		cmd["count"] = cfg.BatchSize
	default:
		cmd["command"] = lgcnpCmdGetStats
	}

	return json.Marshal(cmd)
}
