package node

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/century"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// Century 노드 에러 변수
// ---------------------------------------------------------------------------

var (
	// ErrCenturyMissingAgentRef 는 agent_ref 설정이 없을 때 반환된다.
	ErrCenturyMissingAgentRef = errors.New("century node: agent_ref is required")

	// ErrCenturyNoResolver 는 AgentResolver 가 설정되지 않았을 때 반환된다.
	ErrCenturyNoResolver = errors.New("century node: agent resolver not set")

	// ErrCenturyAgentNotCentury 는 resolve 된 Agent 가 Century 타입이 아닐 때 반환된다.
	ErrCenturyAgentNotCentury = errors.New("century node: agent is not a Century HVAC agent")

	// ErrCenturyProcessFailed 는 Agent Process() 호출이 실패했을 때 반환된다.
	ErrCenturyProcessFailed = errors.New("century node: process command failed")
)

// ---------------------------------------------------------------------------
// 상수 정의
// ---------------------------------------------------------------------------

const (
	centuryDefaultTimeout      = 5 * time.Second
	centuryDefaultPollInterval = 100 * time.Millisecond
	centuryMinPollInterval     = 1 * time.Millisecond

	centuryCmdGetStats  = "get_stats"
	centuryCmdGetRecent = "get_recent"
	centuryCmdGetAll    = "get_all"
	centuryCmdGetState  = "get_state"
	centuryCmdDrain     = "drain"
)

// 제어 키 (CenturyNode 통합 노드의 not_supported 분기 판정용; REQ-CENTURY-018).
// LGCNP 와 정렬: power / mode / temperature / fan_speed. Century 는 setpoint 도 인식.
var centuryControlKeys = []string{"power", "mode", "temperature", "setpoint", "fan_speed"}

// ---------------------------------------------------------------------------
// CenturyNodeConfig
// ---------------------------------------------------------------------------

// CenturyNodeConfig 는 Century 노드 공용 설정 구조체이다 (REQ-CENTURY-016 ~ 019).
type CenturyNodeConfig struct {
	AgentRef     string `json:"agent_ref"`     // 필수: Century 에이전트 이름/ID
	PollInterval string `json:"poll_interval"` // 선택: 폴링 간격 (기본 "100ms")
	Timeout      string `json:"timeout"`       // 선택: Process 타임아웃 (기본 "5s")
	PollCommand  string `json:"poll_command"`  // 선택: 폴링 커맨드 (기본 "drain")
	RecentCount  int    `json:"recent_count"`  // 선택: get_recent 시 프레임 수 (기본 10)
	BatchSize    int    `json:"batch_size"`    // 선택: 폴링 시 벌크 수신 수량 (기본 32)
}

// ---------------------------------------------------------------------------
// centuryNodeBase
// ---------------------------------------------------------------------------

// centuryNodeBase 는 Century 노드 공통 기반 구조체이다 (LGCNP 패턴 정렬).
type centuryNodeBase struct {
	*BaseNode
	centuryCfg CenturyNodeConfig
	resolver   AgentResolver
	transport  AgentTransport
	agent      agent.Agent
	timeout    time.Duration
	mu         sync.RWMutex
}

// configure 는 공통 설정 파싱을 수행한다.
func (nb *centuryNodeBase) configure(config map[string]any) error {
	if err := nb.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg CenturyNodeConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrCenturyMissingAgentRef
	}

	cfg.PollInterval = "100ms"
	if v, ok := config["poll_interval"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.PollInterval = s
		}
	}

	cfg.Timeout = "5s"
	if v, ok := config["timeout"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.Timeout = s
		}
	}

	cfg.PollCommand = centuryCmdDrain
	if v, ok := config["poll_command"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.PollCommand = s
		}
	}

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
		timeout = centuryDefaultTimeout
	}

	nb.mu.Lock()
	nb.centuryCfg = cfg
	nb.timeout = timeout
	nb.mu.Unlock()

	return nil
}

// initAgent 는 AgentResolver 를 통해 에이전트를 resolve 하고 Century 타입을 확인한다.
//
// AC-C7 (Init-tolerance): resolver 가 nil 이면 hard-fail (구성 오류). 에이전트가 아직
// 등록되지 않은 경우는 hard-fail 하지 않고 nil 반환하여 deferred connection 으로 진행한다
// (LGCNP v1.3 패턴 정렬).
func (nb *centuryNodeBase) initAgent(ctx context.Context) error {
	if nb.resolver == nil {
		return ErrCenturyNoResolver
	}

	nb.mu.RLock()
	agentRef := nb.centuryCfg.AgentRef
	nb.mu.RUnlock()

	ref := flow.AgentRef{
		AgentID:   agentRef,
		AgentName: agentRef,
	}
	transport, err := nb.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		// deferred connection — engine 의 ReinitNodesForAgent 가 후에 재호출한다.
		return nil
	}
	nb.transport = transport

	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return ErrCenturyAgentNotCentury
	}

	underlying := accessor.UnderlyingAgent()
	switch underlying.(type) {
	case *century.CenturyAgent:
		nb.agent = underlying
	default:
		return ErrCenturyAgentNotCentury
	}
	return nil
}

// drainDeviceStateEvents 는 v0.3.11 의 polling 경로 device_state drain helper 이다.
//
// agent 의 processDrainDeviceState 를 호출해 keepalive/change device_state JSON 들을
// 가져와 message 로 변환 후 sourceCh 에 전달한다. sourceCh 가 가득 차면 즉시 반환한다.
//
// 사용자 보고 ("keepalive 전송 안됨") 의 root cause 였던 "polling 노드가 msgCh 를
// 보지 못함" 문제를 해결한다. msgCh 의 Bridge 컨슈머와는 독립적인 별도 buffer 를
// 사용하므로 두 경로가 경쟁하지 않는다.
func (nb *centuryNodeBase) drainDeviceStateEvents(nodeID string, sourceCh chan<- message.Message) {
	if nb.agent == nil {
		return
	}
	cmdBytes, err := json.Marshal(map[string]any{"command": "drain_device_state"})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), nb.timeout)
	resp, err := nb.callAgentProcess(ctx, cmdBytes)
	cancel()
	if err != nil {
		return
	}
	var result struct {
		Count  int               `json:"count"`
		Events []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return
	}
	for _, evb := range result.Events {
		var fields map[string]any
		if err := json.Unmarshal(evb, &fields); err != nil {
			continue
		}
		msg := message.New()
		// v0.7.14: payload 내부의 metadata 그룹을 message metadata 로 promote.
		promotePayloadMetadata(msg, fields)
		for k, v := range fields {
			msg.Payload().Set(k, v)
		}
		msg.Metadata().Set("century_source", "device_state")
		msg.Metadata().Set("century_node_id", nodeID)
		msg.Metadata().Set("message_type", "event")
		select {
		case sourceCh <- msg:
		default:
			// sourceCh full — drop remainder (drop-oldest 는 agent buffer 단계에서 처리됨).
			return
		}
	}
}

// callAgentProcess 는 Agent.Process() 를 context timeout 과 함께 호출한다.
func (nb *centuryNodeBase) callAgentProcess(ctx context.Context, cmdBytes []byte) ([]byte, error) {
	if nb.agent == nil {
		return nil, ErrCenturyNoResolver
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
		return nil, fmt.Errorf("century: %w", timeoutCtx.Err())
	case r := <-ch:
		return r.data, r.err
	}
}

// shutdown 은 공통 종료 로직을 수행한다.
func (nb *centuryNodeBase) shutdown() error {
	return nb.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (nb *centuryNodeBase) AgentRef() flow.AgentRef {
	nb.mu.RLock()
	ref := nb.centuryCfg.AgentRef
	nb.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// extractResolverFromConfig 는 BaseNode.config 에 endpoint 가 주입한 AgentResolver 를
// 꺼내는 공통 헬퍼이다 (LGCNP 와 동일 패턴).
func extractResolverFromConfig(base *BaseNode) AgentResolver {
	if base.config == nil {
		return nil
	}
	r, ok := base.config["_agent_resolver"]
	if !ok {
		return nil
	}
	if resolver, ok := r.(AgentResolver); ok {
		return resolver
	}
	return nil
}

// ===========================================================================
// CenturyStatusNode — 상태 조회 전용 (SourceNode)
// ===========================================================================

// CenturyStatusNode 는 Century 에이전트의 디코딩된 상태 이벤트를 폴링하여 송출하는 노드이다 (REQ-CENTURY-016).
type CenturyStatusNode struct {
	centuryNodeBase
	pollInterval time.Duration
	sourceCh     chan message.Message
	stopCh       chan struct{}
	pollOnce     sync.Once
	lastSeq      uint64
	// v0.7.7: pollSingle 의 byte-equal dedup 용 (get_all / get_state 의 동일 응답 반복 송출 방지).
	// get_stats 는 counter 가 매번 변하므로 사실상 dedup 효과 없음.
	lastSingleResp []byte
}

var (
	_ Node       = (*CenturyStatusNode)(nil)
	_ SourceNode = (*CenturyStatusNode)(nil)
)

// NewCenturyStatusNode 는 새로운 CenturyStatusNode 를 생성한다.
func NewCenturyStatusNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &CenturyStatusNode{
		centuryNodeBase: centuryNodeBase{BaseNode: base},
		sourceCh:        make(chan message.Message, 64),
		stopCh:          make(chan struct{}),
	}
	n.resolver = extractResolverFromConfig(base)
	return n, nil
}

// Configure 는 CenturyStatusNode 설정을 적용한다.
func (n *CenturyStatusNode) Configure(config map[string]any) error {
	if err := n.centuryNodeBase.configure(config); err != nil {
		return err
	}
	n.mu.RLock()
	pollStr := n.centuryCfg.PollInterval
	n.mu.RUnlock()
	pollInterval, err := time.ParseDuration(pollStr)
	if err != nil {
		pollInterval = centuryDefaultPollInterval
	}
	if pollInterval < centuryMinPollInterval {
		pollInterval = centuryMinPollInterval
	}
	n.pollInterval = pollInterval
	return nil
}

// Init 은 CenturyStatusNode 를 초기화한다.
func (n *CenturyStatusNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.centuryNodeBase.initAgent(ctx); err != nil {
		return err
	}
	go n.pollLoop()
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// pollLoop 는 에이전트에서 프레임을 조회하여 sourceCh 에 전달한다.
//
// FrameNotifier 가 신호를 발행하면 타이머를 기다리지 않고 즉시 폴링한다 (AC-C8).
func (n *CenturyStatusNode) pollLoop() {
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	var notifyCh <-chan struct{}
	if fn, ok := n.agent.(agent.FrameNotifier); ok {
		notifyCh = fn.FrameNotifyCh()
	}

	poll := func() {
		n.mu.RLock()
		cfg := n.centuryCfg
		n.mu.RUnlock()

		switch cfg.PollCommand {
		case centuryCmdGetRecent, centuryCmdDrain:
			n.pollBulk(cfg /* rawMode */, false)
		default:
			n.pollSingle(cfg)
		}
		// v0.3.11: device_state (change/keepalive) 이벤트도 polling 경로로 drain.
		// 사용자 보고 "keepalive 전송 안됨" root cause fix.
		n.centuryNodeBase.drainDeviceStateEvents(n.ID(), n.sourceCh)
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

// pollSingle 은 get_stats / get_all / get_state 요청을 한 번 실행하여 단일 메시지를 송출한다.
//
// v0.7.7: byte-equal dedup — 직전 응답과 완전히 동일하면 emit skip
// (get_all / get_state 의 동일 snapshot 반복 송출 방지). get_stats 는 counter 가
// 매 polling 마다 변하므로 사실상 dedup 효과 없음 → poll_interval 로 조절.
func (n *CenturyStatusNode) pollSingle(cfg CenturyNodeConfig) {
	cmdBytes, err := buildCenturyStatusCommand(cfg)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	resp, err := n.centuryNodeBase.callAgentProcess(ctx, cmdBytes)
	cancel()
	if err != nil {
		return
	}
	// v0.7.8: 휘발성 필드 (last_seen_ms) 제외하고 dedup 비교.
	normalized := normalizeForDedup(resp)
	if bytes.Equal(normalized, n.lastSingleResp) {
		return
	}
	n.lastSingleResp = append(n.lastSingleResp[:0], normalized...)

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return
	}
	msg := message.New()
	// v0.7.14: payload 내부의 metadata 그룹은 message metadata 로 promote.
	promotePayloadMetadata(msg, result)
	for k, v := range result {
		msg.Payload().Set(k, v)
	}
	msg.Metadata().Set("century_source", "poll")
	msg.Metadata().Set("century_node_id", n.ID())
	msg.Metadata().Set("message_type", "event")
	select {
	case n.sourceCh <- msg:
	default:
	}
}

// pollBulk 는 get_recent / drain 요청으로 다수 프레임을 조회하고,
// 각 프레임을 개별 메시지로 송출한다.
//
// rawMode=false (status 노드): decoded 가 있는 frame 만 송출하고 decoded 페이로드를 펼친다.
// rawMode=true  (raw  노드)  : 모든 frame 을 raw + 메타데이터로 송출한다.
func (n *CenturyStatusNode) pollBulk(cfg CenturyNodeConfig, rawMode bool) {
	frames, lastSeq, ok := centuryRequestBulk(n.agent, &n.centuryNodeBase, cfg, n.lastSeq)
	if !ok {
		return
	}
	for _, fr := range frames {
		msg, ok := buildCenturyMessage(fr, n.ID(), rawMode)
		if !ok {
			continue
		}
		select {
		case n.sourceCh <- msg:
		default:
			// downstream 채널이 full — 추가 송출은 다음 cycle 에서 시도 (drop oldest 가 아닌 skip).
			return
		}
	}
	if lastSeq > n.lastSeq {
		n.lastSeq = lastSeq
	}
}

// Process 는 입력 메시지를 받아 단일 상태 조회를 수행한다 (AC-B6 / AC-B7 / AC-B8).
func (n *CenturyStatusNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.centuryCfg
	n.mu.RUnlock()

	cmdBytes, err := buildCenturyStatusCommand(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCenturyProcessFailed, err)
	}
	resp, err := n.centuryNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCenturyProcessFailed, err)
	}
	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrCenturyProcessFailed, err)
	}
	out := msg.Clone()
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	out.Metadata().Set("century_source", "request")
	out.Metadata().Set("century_node_id", n.ID())
	out.Metadata().Set("message_type", "response")
	return []message.Message{out}, nil
}

// Shutdown 은 CenturyStatusNode 를 종료한다.
func (n *CenturyStatusNode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() { close(n.stopCh) })
	return n.centuryNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 FrameNotifier 구독을 재구성한다.
func (n *CenturyStatusNode) Reinit(ctx context.Context) error {
	n.pollOnce.Do(func() { close(n.stopCh) })
	if err := n.centuryNodeBase.initAgent(ctx); err != nil {
		return err
	}
	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.pollOnce = sync.Once{}
	n.mu.Unlock()
	go n.pollLoop()
	return nil
}

// SourceCh 는 폴링으로 생성된 메시지 채널을 반환한다.
func (n *CenturyStatusNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// CenturyControlNode — 제어 미지원 placeholder (REQ-CENTURY-017)
// ===========================================================================

// CenturyControlNode 는 Century 제어 노드이다. 본 에이전트가 패시브 캡처 전용이므로
// 노드는 항상 not_supported 응답을 반환하고 agent.Process() 를 호출하지 않는다.
//
// (AC-C3, AC-B9)
type CenturyControlNode struct {
	centuryNodeBase
}

var _ Node = (*CenturyControlNode)(nil)

// NewCenturyControlNode 는 새로운 CenturyControlNode 를 생성한다.
func NewCenturyControlNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &CenturyControlNode{
		centuryNodeBase: centuryNodeBase{BaseNode: base},
	}
	n.resolver = extractResolverFromConfig(base)
	return n, nil
}

// Configure 는 CenturyControlNode 설정을 적용한다.
func (n *CenturyControlNode) Configure(config map[string]any) error {
	return n.centuryNodeBase.configure(config)
}

// Init 은 CenturyControlNode 를 초기화한다.
//
// 본 노드는 agent.Process() 를 호출하지 않으므로 agent resolve 가 실패해도 진행한다.
func (n *CenturyControlNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.centuryNodeBase.initAgent(ctx); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 어떤 메시지를 받더라도 not_supported 응답을 반환한다 (AC-C3).
//
// CRITICAL: agent.Process() 와 transport.Write() 는 절대 호출하지 않는다.
func (n *CenturyControlNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	out := msg.Clone()
	out.Payload().Set("status", "not_supported")
	out.Payload().Set("reason", "century_passive_only")
	out.Payload().Set("message", "Century HVAC agent operates in passive sniff mode; control commands are never transmitted")
	out.Metadata().Set("century_command", "control")
	out.Metadata().Set("century_node_id", n.ID())
	out.Metadata().Set("message_type", "response")
	return []message.Message{out}, nil
}

// Shutdown 은 CenturyControlNode 를 종료한다.
func (n *CenturyControlNode) Shutdown(_ context.Context) error {
	return n.centuryNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조를 갱신한다.
func (n *CenturyControlNode) Reinit(ctx context.Context) error {
	return n.centuryNodeBase.initAgent(ctx)
}

// ===========================================================================
// CenturyNode — 상태 조회 + 제어 통합 (REQ-CENTURY-018)
// ===========================================================================

// CenturyNode 는 상태 조회 (SourceNode) 와 제어 (ProcessNode) 를 통합한 노드이다.
// 제어 키가 포함된 메시지는 not_supported 를 반환하고, 그 외에는 상태 조회를 수행한다.
type CenturyNode struct {
	centuryNodeBase
	pollInterval time.Duration
	sourceCh     chan message.Message
	stopCh       chan struct{}
	pollOnce     sync.Once
	lastSeq      uint64
}

var (
	_ Node       = (*CenturyNode)(nil)
	_ SourceNode = (*CenturyNode)(nil)
)

// NewCenturyNode 는 새로운 CenturyNode 를 생성한다.
func NewCenturyNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &CenturyNode{
		centuryNodeBase: centuryNodeBase{BaseNode: base},
		sourceCh:        make(chan message.Message, 64),
		stopCh:          make(chan struct{}),
	}
	n.resolver = extractResolverFromConfig(base)
	return n, nil
}

// Configure 는 CenturyNode 설정을 적용한다.
func (n *CenturyNode) Configure(config map[string]any) error {
	if err := n.centuryNodeBase.configure(config); err != nil {
		return err
	}
	n.mu.RLock()
	pollStr := n.centuryCfg.PollInterval
	n.mu.RUnlock()
	pollInterval, err := time.ParseDuration(pollStr)
	if err != nil {
		pollInterval = centuryDefaultPollInterval
	}
	if pollInterval < centuryMinPollInterval {
		pollInterval = centuryMinPollInterval
	}
	n.pollInterval = pollInterval
	return nil
}

// Init 은 CenturyNode 를 초기화한다.
func (n *CenturyNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.centuryNodeBase.initAgent(ctx); err != nil {
		return err
	}
	go n.pollLoop()
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// pollLoop 는 status 노드와 동일한 폴링 루프를 수행한다.
func (n *CenturyNode) pollLoop() {
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	var notifyCh <-chan struct{}
	if fn, ok := n.agent.(agent.FrameNotifier); ok {
		notifyCh = fn.FrameNotifyCh()
	}

	poll := func() {
		n.mu.RLock()
		cfg := n.centuryCfg
		n.mu.RUnlock()
		switch cfg.PollCommand {
		case centuryCmdGetRecent, centuryCmdDrain:
			frames, lastSeq, ok := centuryRequestBulk(n.agent, &n.centuryNodeBase, cfg, n.lastSeq)
			if !ok {
				return
			}
			for _, fr := range frames {
				msg, mok := buildCenturyMessage(fr, n.ID(), false)
				if !mok {
					continue
				}
				select {
				case n.sourceCh <- msg:
				default:
					return
				}
			}
			if lastSeq > n.lastSeq {
				n.lastSeq = lastSeq
			}
		default:
			// pollSingle 경로는 status node 와 동일하다 — 여기서는 stats 요청만.
			cmdBytes, err := buildCenturyStatusCommand(cfg)
			if err != nil {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
			resp, err := n.centuryNodeBase.callAgentProcess(ctx, cmdBytes)
			cancel()
			if err != nil {
				return
			}
			var result map[string]any
			if err := json.Unmarshal(resp, &result); err != nil {
				return
			}
			msg := message.New()
			// v0.7.14: payload 내부의 metadata 그룹을 message metadata 로 promote.
			promotePayloadMetadata(msg, result)
			for k, v := range result {
				msg.Payload().Set(k, v)
			}
			msg.Metadata().Set("century_source", "poll")
			msg.Metadata().Set("century_node_id", n.ID())
			msg.Metadata().Set("message_type", "event")
			select {
			case n.sourceCh <- msg:
			default:
			}
		}
		// v0.3.11: device_state (change/keepalive) 이벤트도 polling 경로로 drain.
		n.centuryNodeBase.drainDeviceStateEvents(n.ID(), n.sourceCh)
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

// Process 는 입력 메시지를 받는다. 제어 키가 포함되면 not_supported 응답을 반환한다 (AC-C4).
func (n *CenturyNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	if hasCenturyControlKey(msg) {
		out := msg.Clone()
		out.Payload().Set("status", "not_supported")
		out.Payload().Set("reason", "century_passive_only")
		out.Payload().Set("message", "Century HVAC agent operates in passive sniff mode; control commands are never transmitted")
		out.Metadata().Set("century_command", "control")
		out.Metadata().Set("century_node_id", n.ID())
		out.Metadata().Set("message_type", "response")
		return []message.Message{out}, nil
	}

	n.mu.RLock()
	cfg := n.centuryCfg
	n.mu.RUnlock()
	cmdBytes, err := buildCenturyStatusCommand(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCenturyProcessFailed, err)
	}
	resp, err := n.centuryNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCenturyProcessFailed, err)
	}
	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrCenturyProcessFailed, err)
	}
	out := msg.Clone()
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	out.Metadata().Set("century_command", "status")
	out.Metadata().Set("century_node_id", n.ID())
	out.Metadata().Set("message_type", "response")
	return []message.Message{out}, nil
}

// Shutdown 은 CenturyNode 를 종료한다.
func (n *CenturyNode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() { close(n.stopCh) })
	return n.centuryNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 FrameNotifier 구독을 재구성한다.
func (n *CenturyNode) Reinit(ctx context.Context) error {
	n.pollOnce.Do(func() { close(n.stopCh) })
	if err := n.centuryNodeBase.initAgent(ctx); err != nil {
		return err
	}
	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.pollOnce = sync.Once{}
	n.mu.Unlock()
	go n.pollLoop()
	return nil
}

// SourceCh 는 폴링으로 생성된 메시지 채널을 반환한다.
func (n *CenturyNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// CenturyRawFrameNode — raw 프레임 캡처 (REQ-CENTURY-019)
// ===========================================================================

// CenturyRawFrameNode 는 에이전트의 ring buffer 에 저장된 모든 프레임 (decoded 성공 여부와
// 무관하게) 을 raw 바이트 + 메타데이터로 송출한다.
//
// AC-F1: dedupe_writes=true 이어도 raw frame 노드는 모든 WRITE frame 을 emit 한다.
type CenturyRawFrameNode struct {
	centuryNodeBase
	pollInterval time.Duration
	sourceCh     chan message.Message
	stopCh       chan struct{}
	pollOnce     sync.Once
	lastSeq      uint64
}

var (
	_ Node       = (*CenturyRawFrameNode)(nil)
	_ SourceNode = (*CenturyRawFrameNode)(nil)
)

// NewCenturyRawFrameNode 는 새로운 CenturyRawFrameNode 를 생성한다.
func NewCenturyRawFrameNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &CenturyRawFrameNode{
		centuryNodeBase: centuryNodeBase{BaseNode: base},
		sourceCh:        make(chan message.Message, 64),
		stopCh:          make(chan struct{}),
	}
	n.resolver = extractResolverFromConfig(base)
	return n, nil
}

// Configure 는 CenturyRawFrameNode 설정을 적용한다.
//
// raw 노드는 poll_command 가 무의미하므로 무조건 drain 으로 강제한다 (모든 frame 캡처).
func (n *CenturyRawFrameNode) Configure(config map[string]any) error {
	if err := n.centuryNodeBase.configure(config); err != nil {
		return err
	}
	// raw 노드는 항상 drain 으로 동작 (모든 frame 송출).
	n.mu.Lock()
	n.centuryCfg.PollCommand = centuryCmdDrain
	pollStr := n.centuryCfg.PollInterval
	n.mu.Unlock()

	pollInterval, err := time.ParseDuration(pollStr)
	if err != nil {
		pollInterval = centuryDefaultPollInterval
	}
	if pollInterval < centuryMinPollInterval {
		pollInterval = centuryMinPollInterval
	}
	n.pollInterval = pollInterval
	return nil
}

// Init 은 CenturyRawFrameNode 를 초기화한다.
func (n *CenturyRawFrameNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.centuryNodeBase.initAgent(ctx); err != nil {
		return err
	}
	go n.pollLoop()
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// pollLoop 는 ring buffer 로부터 모든 frame 을 폴링하여 raw 메시지로 송출한다.
func (n *CenturyRawFrameNode) pollLoop() {
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	var notifyCh <-chan struct{}
	if fn, ok := n.agent.(agent.FrameNotifier); ok {
		notifyCh = fn.FrameNotifyCh()
	}

	poll := func() {
		n.mu.RLock()
		cfg := n.centuryCfg
		n.mu.RUnlock()
		frames, lastSeq, ok := centuryRequestBulk(n.agent, &n.centuryNodeBase, cfg, n.lastSeq)
		if !ok {
			return
		}
		for _, fr := range frames {
			msg, mok := buildCenturyMessage(fr, n.ID(), true)
			if !mok {
				continue
			}
			select {
			case n.sourceCh <- msg:
			default:
				return
			}
		}
		if lastSeq > n.lastSeq {
			n.lastSeq = lastSeq
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

// Process 는 입력 메시지를 그대로 무시한다 (raw 노드는 source-only).
func (n *CenturyRawFrameNode) Process(_ context.Context, _ message.Message) ([]message.Message, error) {
	return nil, nil
}

// Shutdown 은 CenturyRawFrameNode 를 종료한다.
func (n *CenturyRawFrameNode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() { close(n.stopCh) })
	return n.centuryNodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 FrameNotifier 구독을 재구성한다.
func (n *CenturyRawFrameNode) Reinit(ctx context.Context) error {
	n.pollOnce.Do(func() { close(n.stopCh) })
	if err := n.centuryNodeBase.initAgent(ctx); err != nil {
		return err
	}
	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.pollOnce = sync.Once{}
	n.mu.Unlock()
	go n.pollLoop()
	return nil
}

// SourceCh 는 raw frame 메시지 채널을 반환한다.
func (n *CenturyRawFrameNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// 헬퍼 함수
// ===========================================================================

// hasCenturyControlKey 는 메시지에 알려진 제어 키 (centuryControlKeys) 중 하나가 있는지 검사한다.
func hasCenturyControlKey(msg message.Message) bool {
	for _, k := range centuryControlKeys {
		if _, ok := msg.Payload().Get(k); ok {
			return true
		}
	}
	return false
}

// buildCenturyStatusCommand 는 상태 조회용 JSON 커맨드를 생성한다.
//
// poll_command 에 따라:
//   - get_recent: count = recent_count (count==0 이면 drain 동작)
//   - get_all:    모든 device 즉시 snapshot
//   - get_state:  단일 device 즉시 snapshot (현재 노드 폴링에선 미사용)
//   - drain:      v0.7.1 deprecated alias (get_recent + count=0)
//   - 그 외:      get_stats
func buildCenturyStatusCommand(cfg CenturyNodeConfig) ([]byte, error) {
	cmd := map[string]any{}
	switch cfg.PollCommand {
	case centuryCmdGetRecent:
		cmd["command"] = centuryCmdGetRecent
		cmd["count"] = cfg.RecentCount
	case centuryCmdGetAll:
		cmd["command"] = centuryCmdGetAll
	case centuryCmdGetState:
		cmd["command"] = centuryCmdGetState
	case centuryCmdDrain:
		cmd["command"] = centuryCmdDrain
	default:
		cmd["command"] = centuryCmdGetStats
	}
	return json.Marshal(cmd)
}

// rawFrameEntry 는 ring buffer 로부터 받은 한 frame 의 raw form 이다.
type rawFrameEntry struct {
	Seq          uint64          `json:"seq"`
	TimestampMs  int64           `json:"timestamp_ms"`
	RawHex       string          `json:"raw_hex"`
	FunctionCode byte            `json:"function_code"`
	Register     *byte           `json:"register,omitempty"`
	Decoded      json.RawMessage `json:"decoded,omitempty"`
	DecodeError  string          `json:"decode_error,omitempty"`
}

// centuryRequestBulk 는 agent 에 get_recent 또는 drain 요청을 보내고 frames 슬라이스를 반환한다.
//
// 반환된 lastSeq 는 응답 frame 중 가장 큰 seq. 호출자는 다음 폴링 cycle 에 이 값을 전달하여
// 재전송을 방지한다. ok=false 인 경우 호출자는 무시한다.
func centuryRequestBulk(_ agent.Agent, nb *centuryNodeBase, cfg CenturyNodeConfig, lastSeq uint64) ([]rawFrameEntry, uint64, bool) {
	if nb.agent == nil {
		return nil, lastSeq, false
	}
	cmd := map[string]any{}
	switch cfg.PollCommand {
	case centuryCmdGetRecent:
		cmd["command"] = centuryCmdGetRecent
		cmd["count"] = cfg.RecentCount
	default:
		cmd["command"] = centuryCmdDrain
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return nil, lastSeq, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), nb.timeout)
	resp, err := nb.callAgentProcess(ctx, cmdBytes)
	cancel()
	if err != nil {
		return nil, lastSeq, false
	}
	var result struct {
		Count  int             `json:"count"`
		Frames []rawFrameEntry `json:"frames"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, lastSeq, false
	}

	// last_seq 보다 큰 항목만 유지. drain 은 비파괴적이지 않으나 보수적으로 필터.
	out := make([]rawFrameEntry, 0, len(result.Frames))
	maxSeq := lastSeq
	for _, fr := range result.Frames {
		if fr.Seq <= lastSeq {
			continue
		}
		out = append(out, fr)
		if fr.Seq > maxSeq {
			maxSeq = fr.Seq
		}
	}
	return out, maxSeq, true
}

// buildCenturyMessage 는 한 frame entry 를 출력 message 로 변환한다.
//
// rawMode=true: 모든 frame (decoded 성공 / 실패 모두) 을 raw 페이로드로 송출.
// rawMode=false: decoded 가 있는 frame 만 송출하고 decoded 페이로드를 펼친다.
func buildCenturyMessage(fr rawFrameEntry, nodeID string, rawMode bool) (message.Message, bool) {
	if rawMode {
		msg := message.New()
		msg.Payload().Set("type", "century_raw_frame")
		msg.Payload().Set("seq", fr.Seq)
		msg.Payload().Set("timestamp_ms", fr.TimestampMs)
		msg.Payload().Set("raw_hex", fr.RawHex)
		msg.Payload().Set("function_code", int(fr.FunctionCode))
		if fr.Register != nil {
			msg.Payload().Set("register", int(*fr.Register))
		}
		msg.Payload().Set("crc_ok", true) // ring buffer 에 들어온 frame 은 CRC 통과
		if fr.DecodeError != "" {
			// decoded 실패 — 어느 단계에서 실패했는지 표시.
			msg.Payload().Set("validation_stage", centuryValidationStage(fr.DecodeError))
			msg.Payload().Set("decode_error", fr.DecodeError)
		} else {
			msg.Payload().Set("validation_stage", "ok")
		}
		msg.Payload().Set("confirmation_status", "raw")
		msg.Metadata().Set("century_source", "raw_frame")
		msg.Metadata().Set("century_node_id", nodeID)
		msg.Metadata().Set("message_type", "event")
		return msg, true
	}

	// status 모드: decoded 가 있는 frame 만 emit.
	if len(fr.Decoded) == 0 {
		return nil, false
	}
	var decoded map[string]any
	if err := json.Unmarshal(fr.Decoded, &decoded); err != nil {
		return nil, false
	}
	msg := message.New()
	// v0.7.14: payload 내부의 metadata 그룹을 message metadata 로 promote.
	promotePayloadMetadata(msg, decoded)
	for k, v := range decoded {
		msg.Payload().Set(k, v)
	}
	msg.Payload().Set("seq", fr.Seq)
	// v0.3.6: raw_hex 가 빈 string 이면 페이로드에 set 하지 않음
	// (include_raw_hex=false 시 capturedFrameEvent.RawHex 가 빈 string 으로 채워짐).
	if fr.RawHex != "" {
		msg.Payload().Set("raw_hex", fr.RawHex)
	}
	msg.Metadata().Set("century_source", "poll_bulk")
	msg.Metadata().Set("century_node_id", nodeID)
	msg.Metadata().Set("message_type", "event")
	return msg, true
}

// centuryValidationStage 는 decoder 에러 메시지로부터 validation stage 문자열을 추정한다.
func centuryValidationStage(decodeErr string) string {
	switch {
	case decodeErr == "":
		return "ok"
	case containsSubstr(decodeErr, "register length"):
		return "register_length"
	case containsSubstr(decodeErr, "payload prefix"):
		return "payload_prefix"
	case containsSubstr(decodeErr, "function_code"):
		return "header"
	case containsSubstr(decodeErr, "unknown register"):
		return "payload_prefix"
	default:
		return "decode_error"
	}
}

// containsSubstr 은 strings.Contains 의 가벼운 래퍼이다 (의존성 줄이기용).
func containsSubstr(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
