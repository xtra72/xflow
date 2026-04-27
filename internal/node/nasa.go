package node

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/samsung"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 상수 정의
// ---------------------------------------------------------------------------

const (
	// 기본 타임아웃
	nasaDefaultTimeout = 5 * time.Second

	// 기본 폴링 간격
	nasaDefaultPollInterval = 30 * time.Second

	// 기본 NASA 커맨드
	nasaCmdGetState        = "get_state"
	nasaCmdGetAllState     = "get_all_states"
	nasaCmdGetRecentStates = "get_recent_states"
	nasaCmdSetPower    = "set_power"
	nasaCmdSetMode     = "set_mode"
	nasaCmdSetTemp     = "set_temperature"
	nasaCmdSetFanSpeed = "set_fan_speed"
	nasaCmdSetMultiple = "set_multiple"
)

// ---------------------------------------------------------------------------
// NASANodeConfig (R2)
// ---------------------------------------------------------------------------

// NASANodeConfig 는 NASA 노드 공용 설정 구조체이다.
type NASANodeConfig struct {
	AgentRef     string `json:"agent_ref"`      // 대상 Samsung NASA Agent 이름/ID (필수)
	DeviceID     string `json:"device_id"`      // 대상 디바이스 ID (선택, 빈 문자열이면 get_all_states)
	PollInterval string `json:"poll_interval"`  // 폴링 간격 (선택, SourceNode 전용, 기본값 "30s")
	Timeout      string `json:"timeout"`        // Process 호출 타임아웃 (선택, 기본값 "5s")
	PollCommand  string `json:"poll_command"`   // 폴링 커맨드 (선택, "get_all_states" 또는 "get_recent_states", 기본값 "get_recent_states")
	BatchSize    int    `json:"batch_size"`     // 벌크 수신 수량 (선택, get_recent_states 전용, 기본값 32)
}

// ---------------------------------------------------------------------------
// nasaNodeBase (R3)
// ---------------------------------------------------------------------------

// nasaNodeBase 는 NASA 노드 공통 기반 구조체이다.
// NASAStatusNode, NASAControlNode, NASANode가 이를 임베딩한다.
type nasaNodeBase struct {
	*BaseNode
	nasaCfg   NASANodeConfig
	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent   // 원본 Samsung NASA Agent 객체
	timeout   time.Duration // Process 호출 타임아웃
	mu        sync.RWMutex  // 설정 보호 뮤텍스
}

// configure 는 공통 설정 파싱을 수행한다. agent_ref(필수)를 검증한다.
func (nb *nasaNodeBase) configure(config map[string]any) error {
	if err := nb.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg NASANodeConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrNASAMissingAgentRef
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

	// poll_command (선택, 기본값 "get_recent_states")
	cfg.PollCommand = nasaCmdGetRecentStates
	if v, ok := config["poll_command"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.PollCommand = s
		}
	}

	// batch_size (선택, 기본값 32)
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

	// 타임아웃 파싱
	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		timeout = nasaDefaultTimeout
	}

	nb.mu.Lock()
	nb.nasaCfg = cfg
	nb.timeout = timeout
	nb.mu.Unlock()

	return nil
}

// initAgent 는 AgentResolver를 통해 에이전트를 resolve하고 Samsung NASA 타입을 확인한다.
func (nb *nasaNodeBase) initAgent(ctx context.Context) error {
	if nb.resolver == nil {
		return ErrNASANoResolver
	}

	nb.mu.RLock()
	agentRef := nb.nasaCfg.AgentRef
	nb.mu.RUnlock()

	ref := flow.AgentRef{
		AgentID:   agentRef,
		AgentName: agentRef,
	}
	transport, err := nb.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("nasa init: agent resolve failed: %w", err)
	}
	nb.transport = transport

	// AgentAccessor를 통해 원본 Agent 객체 획득 및 타입 확인
	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return ErrNASAAgentNotNASA
	}

	underlyingAgent := accessor.UnderlyingAgent()
	switch underlyingAgent.(type) {
	case *samsung.NASAAgent:
		nb.agent = underlyingAgent
	default:
		return ErrNASAAgentNotNASA
	}

	return nil
}

// callAgentProcess 는 Agent.Process()를 context timeout과 함께 호출한다.
func (nb *nasaNodeBase) callAgentProcess(ctx context.Context, cmdBytes []byte) ([]byte, error) {
	if nb.agent == nil {
		return nil, ErrNASANoResolver
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
		return nil, fmt.Errorf("nasa: %w", timeoutCtx.Err())
	case result := <-ch:
		return result.data, result.err
	}
}

// shutdown 은 공통 종료 로직을 수행한다.
func (nb *nasaNodeBase) shutdown() error {
	return nb.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// applyNASAOverrides 는 입력 메시지 payload에서 device_id, timeout을 오버라이드한다.
func applyNASAOverrides(msg message.Message, cfg NASANodeConfig) NASANodeConfig {
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
// NASAStatusNode (R4, R5, R6)
// ===========================================================================

// NASAStatusNode 는 Samsung NASA 에이전트의 상태를 조회하는 노드이다.
// SourceNode 인터페이스를 구현하여 폴링 기반 자체 메시지 생성을 지원한다.
type NASAStatusNode struct {
	nasaNodeBase
	pollInterval time.Duration
	sourceCh     chan message.Message
	stopCh       chan struct{}
	pollOnce     sync.Once    // stopCh close 보호
	lastHash     [sha256.Size]byte // 이전 응답 해시 (변경 감지용)
	lastSeq      int64             // get_recent_states 마지막 수신 seq (중복 방지)
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*NASAStatusNode)(nil)
	_ SourceNode = (*NASAStatusNode)(nil)
)

// NewNASAStatusNode 는 새로운 NASAStatusNode를 생성하는 팩토리 함수이다.
func NewNASAStatusNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &NASAStatusNode{
		nasaNodeBase: nasaNodeBase{
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

// Configure 는 NASAStatusNode의 설정을 적용한다.
func (n *NASAStatusNode) Configure(config map[string]any) error {
	if err := n.nasaNodeBase.configure(config); err != nil {
		return err
	}

	// poll_interval 파싱
	n.mu.RLock()
	pollStr := n.nasaCfg.PollInterval
	n.mu.RUnlock()

	pollInterval, err := time.ParseDuration(pollStr)
	if err != nil {
		pollInterval = nasaDefaultPollInterval
	}
	n.pollInterval = pollInterval

	return nil
}

// Init 은 NASAStatusNode를 초기화한다.
// 에이전트를 resolve하고, 폴링 고루틴을 시작한다.
func (n *NASAStatusNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.nasaNodeBase.initAgent(ctx); err != nil {
		return err
	}

	// 폴링 고루틴 시작 (SourceNode 지원)
	go n.pollLoop()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// pollLoop 는 설정된 간격으로 상태를 조회하여 sourceCh에 메시지를 전달한다.
func (n *NASAStatusNode) pollLoop() {
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	// 에이전트가 FrameNotifier를 구현하면 이벤트 기반 폴링 활성화
	var notifyCh <-chan struct{}
	if fn, ok := n.agent.(agent.FrameNotifier); ok {
		notifyCh = fn.FrameNotifyCh()
	}

	poll := func() {
		n.mu.RLock()
		cfg := n.nasaCfg
		n.mu.RUnlock()

		switch cfg.PollCommand {
		case nasaCmdGetRecentStates:
			n.pollRecentBulk(cfg)
		default:
			n.pollSnapshot(cfg)
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

// pollSnapshot 는 get_all_states 스냅샷 모드로 폴링한다 (hash dedup 적용).
func (n *NASAStatusNode) pollSnapshot(cfg NASANodeConfig) {
	cmdBytes, err := buildStatusCommand(cfg, n.ID())
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	resp, err := n.nasaNodeBase.callAgentProcess(ctx, cmdBytes)
	cancel()

	if err != nil {
		return
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return
	}

	h := nasaStateHash(result)
	if h == n.lastHash {
		return
	}
	n.lastHash = h

	msgs := splitNASAPollResult(result, n.ID())
	for _, msg := range msgs {
		select {
		case n.sourceCh <- msg:
		default:
		}
	}
}

// pollRecentBulk 는 get_recent_states 커맨드로 벌크 수신하여 새 스냅샷만 개별 메시지로 전송한다.
// lastSeq를 기준으로 이미 전송한 스냅샷을 필터링하여 중복을 방지한다.
func (n *NASAStatusNode) pollRecentBulk(cfg NASANodeConfig) {
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 32
	}

	cmdBytes, err := json.Marshal(map[string]any{
		"command": nasaCmdGetRecentStates,
		"node_id": n.ID(),
		"params": map[string]any{
			"last_seq": n.lastSeq,
			"count":    batchSize,
		},
	})
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	resp, err := n.nasaNodeBase.callAgentProcess(ctx, cmdBytes)
	cancel()

	if err != nil {
		return
	}

	var result struct {
		Count     int               `json:"count"`
		Snapshots []json.RawMessage `json:"snapshots"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return
	}

	// 각 스냅샷은 단일 디바이스 → 1:1 메시지 매핑
	for _, raw := range result.Snapshots {
		var snap struct {
			Seq    int64           `json:"seq"`
			Device json.RawMessage `json:"device"`
		}
		if err := json.Unmarshal(raw, &snap); err != nil {
			continue
		}
		if snap.Seq <= n.lastSeq {
			continue
		}

		var dev map[string]any
		if err := json.Unmarshal(snap.Device, &dev); err != nil {
			continue
		}

		msg := message.New()
		for k, v := range dev {
			msg.Payload().Set(k, v)
		}
		msg.Metadata().Set("nasa_source", "poll_bulk")
		msg.Metadata().Set("nasa_node_id", n.ID())
		msg.Metadata().Set("nasa_seq", fmt.Sprintf("%d", snap.Seq))

		select {
		case n.sourceCh <- msg:
			n.lastSeq = snap.Seq
		default:
			return
		}
	}
}

// Process 는 입력 메시지를 받아 상태 조회를 수행하고 결과를 반환한다.
func (n *NASAStatusNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.nasaCfg
	n.mu.RUnlock()

	// 메시지 payload에서 오버라이드 적용
	cfg = applyNASAOverrides(msg, cfg)

	cmdBytes, err := buildStatusCommand(cfg, n.ID())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNASAProcessFailed, err)
	}

	resp, err := n.nasaNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNASAProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrNASAProcessFailed, err)
	}

	out := msg.Clone()
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	out.Metadata().Set("nasa_source", "request")
	out.Metadata().Set("nasa_node_id", n.ID())

	return []message.Message{out}, nil
}

// Shutdown 은 NASAStatusNode를 종료한다. 폴링 고루틴을 정지한다.
func (n *NASAStatusNode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})
	return n.nasaNodeBase.shutdown()
}

// SourceCh 는 폴링으로 생성된 메시지를 수신하는 채널을 반환한다.
func (n *NASAStatusNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// NASAControlNode (R7, R8, R9)
// ===========================================================================

// NASAControlNode 는 Samsung NASA 에이전트에 제어 명령을 전송하는 노드이다.
type NASAControlNode struct {
	nasaNodeBase
}

// 인터페이스 컴파일 체크
var _ Node = (*NASAControlNode)(nil)

// NewNASAControlNode 는 새로운 NASAControlNode를 생성하는 팩토리 함수이다.
func NewNASAControlNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &NASAControlNode{
		nasaNodeBase: nasaNodeBase{
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

// Configure 는 NASAControlNode의 설정을 적용한다.
func (n *NASAControlNode) Configure(config map[string]any) error {
	return n.nasaNodeBase.configure(config)
}

// Init 은 NASAControlNode를 초기화한다. 에이전트를 resolve한다.
func (n *NASAControlNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.nasaNodeBase.initAgent(ctx); err != nil {
		return err
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 입력 메시지의 payload에서 제어 명령을 추출하여 Agent에 전달한다.
// payload에 "command" 키가 있으면 직접 명령으로 처리하고,
// 없으면 제어 키(power, mode, temperature, fan_speed)에서 set_multiple을 구성한다.
func (n *NASAControlNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.nasaCfg
	n.mu.RUnlock()

	cfg = applyNASAOverrides(msg, cfg)

	cmdBytes, err := buildControlCommand(msg, cfg, n.ID())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNASAProcessFailed, err)
	}

	resp, err := n.nasaNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNASAProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrNASAProcessFailed, err)
	}

	out := msg.Clone()
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	out.Metadata().Set("nasa_command", "control")
	out.Metadata().Set("nasa_node_id", n.ID())

	return []message.Message{out}, nil
}

// Shutdown 은 NASAControlNode를 종료한다.
func (n *NASAControlNode) Shutdown(_ context.Context) error {
	return n.nasaNodeBase.shutdown()
}

// ===========================================================================
// NASANode (R10, R11)
// ===========================================================================

// NASANode 는 Samsung NASA 에이전트의 상태 조회와 제어를 모두 수행하는 통합 노드이다.
// SourceNode 인터페이스를 구현하여 폴링 기반 자체 메시지 생성도 지원한다.
// payload에 제어 키(power, mode, temperature, fan_speed)가 있으면 제어,
// 없으면 상태 조회로 자동 감지한다.
type NASANode struct {
	nasaNodeBase
	pollInterval time.Duration
	sourceCh     chan message.Message
	stopCh       chan struct{}
	pollOnce     sync.Once
	lastHash     [sha256.Size]byte // 이전 응답 해시 (변경 감지용)
	lastSeq      int64             // get_recent_states 마지막 수신 seq (중복 방지)
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*NASANode)(nil)
	_ SourceNode = (*NASANode)(nil)
)

// NewNASANode 는 새로운 NASANode를 생성하는 팩토리 함수이다.
func NewNASANode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &NASANode{
		nasaNodeBase: nasaNodeBase{
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

// Configure 는 NASANode의 설정을 적용한다.
func (n *NASANode) Configure(config map[string]any) error {
	if err := n.nasaNodeBase.configure(config); err != nil {
		return err
	}

	// poll_interval 파싱
	n.mu.RLock()
	pollStr := n.nasaCfg.PollInterval
	n.mu.RUnlock()

	pollInterval, err := time.ParseDuration(pollStr)
	if err != nil {
		pollInterval = nasaDefaultPollInterval
	}
	n.pollInterval = pollInterval

	return nil
}

// Init 은 NASANode를 초기화한다.
// 에이전트를 resolve하고, 폴링 고루틴을 시작한다.
func (n *NASANode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.nasaNodeBase.initAgent(ctx); err != nil {
		return err
	}

	// 폴링 고루틴 시작 (SourceNode 지원)
	go n.pollLoop()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// pollLoop 는 설정된 간격으로 상태를 조회하여 sourceCh에 메시지를 전달한다.
func (n *NASANode) pollLoop() {
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	// 에이전트가 FrameNotifier를 구현하면 이벤트 기반 폴링 활성화
	var notifyCh <-chan struct{}
	if fn, ok := n.agent.(agent.FrameNotifier); ok {
		notifyCh = fn.FrameNotifyCh()
	}

	poll := func() {
		n.mu.RLock()
		cfg := n.nasaCfg
		n.mu.RUnlock()

		switch cfg.PollCommand {
		case nasaCmdGetRecentStates:
			n.pollRecentBulk(cfg)
		default:
			n.pollSnapshot(cfg)
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

// pollSnapshot 는 get_all_states 스냅샷 모드로 폴링한다 (hash dedup 적용).
func (n *NASANode) pollSnapshot(cfg NASANodeConfig) {
	cmdBytes, err := buildStatusCommand(cfg, n.ID())
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	resp, err := n.nasaNodeBase.callAgentProcess(ctx, cmdBytes)
	cancel()

	if err != nil {
		return
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return
	}

	h := nasaStateHash(result)
	if h == n.lastHash {
		return
	}
	n.lastHash = h

	msgs := splitNASAPollResult(result, n.ID())
	for _, msg := range msgs {
		select {
		case n.sourceCh <- msg:
		default:
		}
	}
}

// pollRecentBulk 는 get_recent_states 커맨드로 벌크 수신하여 새 스냅샷만 개별 메시지로 전송한다.
func (n *NASANode) pollRecentBulk(cfg NASANodeConfig) {
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 32
	}

	cmdBytes, err := json.Marshal(map[string]any{
		"command": nasaCmdGetRecentStates,
		"node_id": n.ID(),
		"params": map[string]any{
			"last_seq": n.lastSeq,
			"count":    batchSize,
		},
	})
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	resp, err := n.nasaNodeBase.callAgentProcess(ctx, cmdBytes)
	cancel()

	if err != nil {
		return
	}

	var result struct {
		Count     int               `json:"count"`
		Snapshots []json.RawMessage `json:"snapshots"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return
	}

	for _, raw := range result.Snapshots {
		var snap struct {
			Seq    int64           `json:"seq"`
			Device json.RawMessage `json:"device"`
		}
		if err := json.Unmarshal(raw, &snap); err != nil {
			continue
		}
		if snap.Seq <= n.lastSeq {
			continue
		}

		var dev map[string]any
		if err := json.Unmarshal(snap.Device, &dev); err != nil {
			continue
		}

		msg := message.New()
		for k, v := range dev {
			msg.Payload().Set(k, v)
		}
		msg.Metadata().Set("nasa_source", "poll_bulk")
		msg.Metadata().Set("nasa_node_id", n.ID())
		msg.Metadata().Set("nasa_seq", fmt.Sprintf("%d", snap.Seq))

		select {
		case n.sourceCh <- msg:
			n.lastSeq = snap.Seq
		default:
			return
		}
	}
}

// Process 는 입력 메시지를 받아 자동으로 상태 조회 또는 제어를 수행한다.
// payload에 제어 키(power, mode, temperature, fan_speed)가 있으면 제어 명령,
// 없으면 상태 조회 명령을 전송한다.
func (n *NASANode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.nasaCfg
	n.mu.RUnlock()

	cfg = applyNASAOverrides(msg, cfg)

	var cmdBytes []byte
	var err error
	var cmdType string

	if hasNASAControlKeys(msg) {
		cmdBytes, err = buildControlCommand(msg, cfg, n.ID())
		cmdType = "control"
	} else {
		cmdBytes, err = buildStatusCommand(cfg, n.ID())
		cmdType = "status"
	}

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNASAProcessFailed, err)
	}

	resp, err := n.nasaNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNASAProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrNASAProcessFailed, err)
	}

	out := msg.Clone()
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	out.Metadata().Set("nasa_command", cmdType)
	out.Metadata().Set("nasa_node_id", n.ID())

	return []message.Message{out}, nil
}

// Shutdown 은 NASANode를 종료한다. 폴링 고루틴을 정지한다.
func (n *NASANode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})
	return n.nasaNodeBase.shutdown()
}

// SourceCh 는 폴링으로 생성된 메시지를 수신하는 채널을 반환한다.
func (n *NASANode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// 헬퍼 함수 (R12, R13, R14)
// ===========================================================================

// splitNASAPollResult 는 get_all_states 응답에 devices 배열이 있으면
// 디바이스별 개별 메시지로 분리한다. devices 배열이 없으면 전체 응답을 단일 메시지로 반환한다.
// 각 메시지의 페이로드 구조: { device_id, state, address, ... }
// state-formatter 표현식($.payload.device_id, $.payload.state.Power 등)과 호환된다.
// nasaStateHash 는 NASA 폴링 응답에서 volatile 필드(last_seen)를 제외하고 해싱한다.
// last_seen 은 에이전트가 디바이스를 폴링할 때마다 갱신되므로, 상태 변경과
// 무관하게 매번 달라진다. 이를 제외해야 실제 상태 변경만 감지할 수 있다.
// 또한 get_all_states 는 Go map 순회로 디바이스 순서가 비결정적이므로,
// address 기준 정렬 후 해싱한다.
func nasaStateHash(result map[string]any) [sha256.Size]byte {
	// get_all_states: devices 배열 응답
	if devicesRaw, ok := result["devices"]; ok {
		if devSlice, ok := devicesRaw.([]any); ok {
			cleaned := make([]map[string]any, 0, len(devSlice))
			for _, d := range devSlice {
				if dm, ok := d.(map[string]any); ok {
					c := make(map[string]any, len(dm))
					for k, v := range dm {
						if k != "last_seen" {
							c[k] = v
						}
					}
					cleaned = append(cleaned, c)
				}
			}
			// Go map 순회 순서가 비결정적이므로 address 기준 정렬
			sort.Slice(cleaned, func(i, j int) bool {
				ai, _ := cleaned[i]["address"].(string)
				aj, _ := cleaned[j]["address"].(string)
				return ai < aj
			})
			b, _ := json.Marshal(cleaned)
			return sha256.Sum256(b)
		}
	}
	// get_state: 단일 디바이스 응답
	c := make(map[string]any, len(result))
	for k, v := range result {
		if k != "last_seen" {
			c[k] = v
		}
	}
	b, _ := json.Marshal(c)
	return sha256.Sum256(b)
}

func splitNASAPollResult(result map[string]any, nodeID string) []message.Message {
	// devices 배열 추출 시도
	devicesRaw, ok := result["devices"]
	if ok {
		if devSlice, ok := devicesRaw.([]any); ok && len(devSlice) > 0 {
			msgs := make([]message.Message, 0, len(devSlice))
			for _, d := range devSlice {
				devMap, ok := d.(map[string]any)
				if !ok {
					continue
				}
				msg := message.New()
				for k, v := range devMap {
					msg.Payload().Set(k, v)
				}
				msg.Metadata().Set("nasa_source", "poll")
				msg.Metadata().Set("nasa_node_id", nodeID)
				msgs = append(msgs, msg)
			}
			if len(msgs) > 0 {
				return msgs
			}
		}
	}

	// devices 배열이 없거나 비어있으면 전체 응답을 단일 메시지로
	msg := message.New()
	for k, v := range result {
		msg.Payload().Set(k, v)
	}
	msg.Metadata().Set("nasa_source", "poll")
	msg.Metadata().Set("nasa_node_id", nodeID)
	return []message.Message{msg}
}

// nasaControlKeys 는 NASA 제어 명령으로 인식되는 payload 키 목록이다.
var nasaControlKeys = []string{"power", "mode", "temperature", "fan_speed"}

// hasNASAControlKeys 는 메시지 payload에 제어 키가 하나라도 있는지 확인한다.
func hasNASAControlKeys(msg message.Message) bool {
	for _, key := range nasaControlKeys {
		if _, ok := msg.Payload().Get(key); ok {
			return true
		}
	}
	return false
}

// buildStatusCommand 는 상태 조회용 JSON 커맨드를 생성한다.
// device_id가 설정되어 있으면 get_state, 없으면 get_all_states를 사용한다.
func buildStatusCommand(cfg NASANodeConfig, nodeID string) ([]byte, error) {
	cmd := map[string]any{}

	if cfg.DeviceID != "" {
		cmd["command"] = nasaCmdGetState
		cmd["device_id"] = cfg.DeviceID
	} else {
		cmd["command"] = nasaCmdGetAllState
	}
	if nodeID != "" {
		cmd["node_id"] = nodeID
	}

	return json.Marshal(cmd)
}

// buildControlCommand 는 제어용 JSON 커맨드를 생성한다.
// payload에 "command" 키가 있으면 직접 커맨드로 전달하고,
// 없으면 제어 키(power, mode, temperature, fan_speed)를 수집하여 set_multiple을 구성한다.
func buildControlCommand(msg message.Message, cfg NASANodeConfig, nodeID string) ([]byte, error) {
	// payload에 "command" 키가 있으면 직접 전달
	if v, ok := msg.Payload().Get("command"); ok {
		if cmdStr, ok := v.(string); ok && cmdStr != "" {
			cmd := map[string]any{
				"command": cmdStr,
			}
			if cfg.DeviceID != "" {
				cmd["device_id"] = cfg.DeviceID
			}
			if nodeID != "" {
				cmd["node_id"] = nodeID
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
	for _, key := range nasaControlKeys {
		if v, ok := msg.Payload().Get(key); ok {
			settings[key] = v
		}
	}

	if len(settings) == 0 {
		// 제어 키가 없으면 상태 조회로 폴백
		return buildStatusCommand(cfg, nodeID)
	}

	cmd := map[string]any{
		"command":  nasaCmdSetMultiple,
		"settings": settings,
	}
	if cfg.DeviceID != "" {
		cmd["device_id"] = cfg.DeviceID
	}
	if nodeID != "" {
		cmd["node_id"] = nodeID
	}

	return json.Marshal(cmd)
}
