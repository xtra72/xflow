package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
	// ErrCenturyHvacr01MissingAgentRef 는 agent_ref 설정이 없을 때 반환된다.
	ErrCenturyHvacr01MissingAgentRef = errors.New("century node: agent_ref is required")

	// ErrCenturyHvacr01NoResolver 는 AgentResolver 가 설정되지 않았을 때 반환된다.
	ErrCenturyHvacr01NoResolver = errors.New("century node: agent resolver not set")

	// ErrCenturyHvacr01AgentNotCentury 는 resolve 된 Agent 가 Century 타입이 아닐 때 반환된다.
	ErrCenturyHvacr01AgentNotCentury = errors.New("century node: agent is not a Century HVAC agent")

	// ErrCenturyHvacr01ProcessFailed 는 Agent Process() 호출이 실패했을 때 반환된다.
	ErrCenturyHvacr01ProcessFailed = errors.New("century node: process command failed")
)

// ---------------------------------------------------------------------------
// 상수 정의
// ---------------------------------------------------------------------------

const (
	centuryHvacr01DefaultTimeout           = 5 * time.Second
	centuryHvacr01DefaultInactivityTimeout = 90 * time.Second // v0.18.26: LG ICP-01 통일
	centuryHvacr01MinInactivityTimeout     = 5 * time.Second

	centuryHvacr01CmdGetStats  = "get_stats"
	centuryHvacr01CmdGetRecent = "get_recent"
	centuryHvacr01CmdGetAll    = "get_all"
	centuryHvacr01CmdGetState  = "get_state"
	centuryHvacr01CmdDrain     = "drain"
)

// 제어 키 (CenturyHvacr01Node 통합 노드의 not_supported 분기 판정용; REQ-CENTURY-018).
// LG ICP-01 과 정렬: power / mode / temperature / fan_speed. Century 는 setpoint 도 인식.
var centuryHvacr01ControlKeys = []string{"power", "mode", "temperature", "setpoint", "fan_speed"}

// ---------------------------------------------------------------------------
// CenturyHvacr01NodeConfig
// ---------------------------------------------------------------------------

// CenturyHvacr01NodeConfig 는 Century 노드 공용 설정 구조체이다.
//
// v0.18.26 (2026-05-28): polling 모델 → LG inactivity 모델 통일.
//   - 제거: poll_interval, poll_command, recent_count
//   - 추가: inactivity_timeout, group_id, unit_id
//   - 유지: emit_raw_frames (직전 리팩토링)
//
// group_id / unit_id 어드레싱 (advanced):
//
//	group_id : Century 에서는 사용 안 함 (스키마 parity 유지용 필드).
//	unit_id  : Century sub_dev_id hex ("3B" 등). 빈 값이면 모든 디바이스 처리.
type CenturyHvacr01NodeConfig struct {
	AgentRef          string `json:"agent_ref"`           // 필수: Century HVACR-01 에이전트 이름/ID
	InactivityTimeout string `json:"inactivity_timeout"`  // v0.18.26: inactivity-fallback 시간 (기본 "90s")
	Timeout           string `json:"timeout"`             // 선택: Process 타임아웃 (기본 "5s")
	BatchSize         int    `json:"batch_size"`          // 선택: 폴링 시 벌크 수신 수량 (기본 32)
	OmitStateWhenOff  bool   `json:"omit_state_when_off"` // v0.18.0: power=false 시 current_temperature/mode/fan_speed 제거

	// v0.18.26: 어드레싱 (advanced). Century 는 group_id 미사용 (parity 유지용).
	GroupID string `json:"group_id,omitempty"`
	UnitID  string `json:"unit_id,omitempty"`

	// EmitRawFrames 는 status 노드의 emit 모드를 raw 프레임 모드로 전환한다 (REQ-CENTURY-019, raw-frame 통합).
	//
	// false (default): status 노드 기본 동작 — decoded device_state 메시지를 emit (inactivity 모델).
	// true: 이전 CenturyRawFrameNode 의 동작을 흡수 — ring buffer 를 drain 하여
	//       모든 frame (decoded 성공/실패 무관) 을 raw + 메타데이터로 emit. dedupe 적용
	//       안 됨 (모든 WRITE frame 통과). inactivity / 어드레싱 필터 무시.
	EmitRawFrames bool `json:"emit_raw_frames,omitempty"`

	// EmitMetadata 는 metadata 옵션 필드의 emit 정책을 제어한다 (v0.18.8, v0.18.26).
	// device_id 만 default emit, 나머지는 default OFF.
	EmitMetadata MetadataEmitOptions `json:"emit_metadata"`
}

// ---------------------------------------------------------------------------
// centuryHvacr01NodeBase
// ---------------------------------------------------------------------------

// centuryHvacr01NodeBase 는 Century 노드 공통 기반 구조체이다 (LG ICP-01 패턴 정렬).
type centuryHvacr01NodeBase struct {
	*BaseNode
	centuryCfg CenturyHvacr01NodeConfig
	resolver   AgentResolver
	transport  AgentTransport
	agent      agent.Agent
	timeout    time.Duration
	mu         sync.RWMutex
}

// configure 는 공통 설정 파싱을 수행한다.
func (nb *centuryHvacr01NodeBase) configure(config map[string]any) error {
	if err := nb.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg CenturyHvacr01NodeConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrCenturyHvacr01MissingAgentRef
	}

	// inactivity_timeout (v0.18.26, 기본 "90s")
	cfg.InactivityTimeout = "90s"
	if v, ok := config["inactivity_timeout"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.InactivityTimeout = s
		}
	}

	cfg.Timeout = "5s"
	if v, ok := config["timeout"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.Timeout = s
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

	// v0.18.0: omit_state_when_off — power=false 시 불확실 상태 필드 제거.
	if v, ok := config["omit_state_when_off"].(bool); ok {
		cfg.OmitStateWhenOff = v
	}

	// v0.18.26: 어드레싱 (advanced). group_id 는 Century 에서 사용 안 함.
	if v, ok := config["group_id"].(string); ok {
		cfg.GroupID = v
	}
	if v, ok := config["unit_id"].(string); ok {
		cfg.UnitID = v
	}

	// emit_raw_frames (REQ-CENTURY-019, raw-frame 통합): true 시 status 노드가 raw 프레임 모드.
	if v, ok := config["emit_raw_frames"].(bool); ok {
		cfg.EmitRawFrames = v
	}

	// v0.18.8: emit_metadata — metadata 옵션 필드 emit 정책.
	parseEmitMetadata(config, &cfg.EmitMetadata)

	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		timeout = centuryHvacr01DefaultTimeout
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
// (LG ICP-01 v1.3 패턴 정렬).
func (nb *centuryHvacr01NodeBase) initAgent(ctx context.Context) error {
	if nb.resolver == nil {
		return ErrCenturyHvacr01NoResolver
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
		return ErrCenturyHvacr01AgentNotCentury
	}

	underlying := accessor.UnderlyingAgent()
	switch underlying.(type) {
	case *century.Hvacr01Agent:
		nb.agent = underlying
	default:
		return ErrCenturyHvacr01AgentNotCentury
	}
	return nil
}

// drainDeviceStateEvents 는 polling 경로 device_state drain helper 이다.
//
// agent 의 processDrainDeviceState 를 호출해 keepalive/change/response device_state JSON 들을
// 가져와 message 로 변환 후 sourceCh 에 전달한다. sourceCh 가 가득 차면 즉시 반환한다.
//
// v0.18.26: 어드레싱 필터 (cfg.UnitID) 적용. 매칭 안 되면 skip.
func (nb *centuryHvacr01NodeBase) drainDeviceStateEvents(nodeID string, sourceCh chan<- message.Message) {
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

	nb.mu.RLock()
	cfg := nb.centuryCfg
	nb.mu.RUnlock()

	for _, evb := range result.Events {
		var fields map[string]any
		if err := json.Unmarshal(evb, &fields); err != nil {
			continue
		}
		// v0.18.26: 어드레싱 필터.
		if !centuryHvacr01MatchAddressing(fields, cfg) {
			continue
		}
		msg := message.New()
		promotePayloadMetadata(msg, fields, cfg.EmitMetadata)
		applyDeviceStateMessageType(msg, fields, "event")
		promoteDevIDWithUUID(msg, fields, cfg.AgentRef, cfg.EmitMetadata)
		promoteLastSeenToTimestamp(msg, fields)
		flattenStateToPayload(fields)
		applyPowerOffFilter(fields, cfg.OmitStateWhenOff)
		for k, v := range fields {
			msg.Payload().Set(k, v)
		}
		if cfg.EmitMetadata.NodeSource {
			msg.Metadata().Set("node_source", "device_state")
		}
		if cfg.EmitMetadata.NodeID {
			msg.Metadata().Set("node_id", nodeID)
		}
		select {
		case sourceCh <- msg:
		default:
			return
		}
	}
}

// callAgentProcess 는 Agent.Process() 를 context timeout 과 함께 호출한다.
func (nb *centuryHvacr01NodeBase) callAgentProcess(ctx context.Context, cmdBytes []byte) ([]byte, error) {
	if nb.agent == nil {
		return nil, ErrCenturyHvacr01NoResolver
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
		return nil, fmt.Errorf("century_hvacr01: %w", timeoutCtx.Err())
	case r := <-ch:
		return r.data, r.err
	}
}

// shutdown 은 공통 종료 로직을 수행한다.
func (nb *centuryHvacr01NodeBase) shutdown() error {
	return nb.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (nb *centuryHvacr01NodeBase) AgentRef() flow.AgentRef {
	nb.mu.RLock()
	ref := nb.centuryCfg.AgentRef
	nb.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// extractResolverFromConfig 는 BaseNode.config 에 endpoint 가 주입한 AgentResolver 를
// 꺼내는 공통 헬퍼이다 (LG ICP-01 과 동일 패턴).
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

// ---------------------------------------------------------------------------
// 어드레싱 매칭 (v0.18.26)
// ---------------------------------------------------------------------------

// centuryHvacr01MatchAddressing 는 디바이스 페이로드의 unit_id 가 cfg.UnitID 와
// 매칭되는지 확인한다.
//
// cfg.UnitID 가 비어있으면 항상 true. payload 의 unit_id 는 "0x3B" 또는 "3B" 등
// hex string 형식 (Century agent 의 emit 컨벤션).
//
// group_id 는 Century 에서 사용 안 함.
func centuryHvacr01MatchAddressing(payload map[string]any, cfg CenturyHvacr01NodeConfig) bool {
	if cfg.UnitID == "" {
		return true
	}
	raw, ok := payload["unit_id"]
	if !ok {
		return false
	}
	got, ok := raw.(string)
	if !ok {
		// 정수일 수도 있음 (decoded 경로). 변환 시도.
		switch n := raw.(type) {
		case int:
			got = strconv.FormatInt(int64(n), 16)
		case int64:
			got = strconv.FormatInt(n, 16)
		case float64:
			got = strconv.FormatInt(int64(n), 16)
		default:
			return false
		}
	}
	return hexByteEqual(got, cfg.UnitID)
}

// ===========================================================================
// CenturyHvacr01StatusNode — 상태 조회 전용 (SourceNode)
// ===========================================================================

// CenturyHvacr01StatusNode 는 Century 에이전트의 디코딩된 상태 이벤트를 inactivity
// 모델로 수신하는 노드이다 (v0.18.26, LG ICP-01 통일).
//
// EmitRawFrames=true 시 (REQ-CENTURY-019): ring buffer 를 drain 하여 모든 frame
// 을 raw + 메타데이터로 emit. inactivity / 어드레싱 필터 우회.
type CenturyHvacr01StatusNode struct {
	centuryHvacr01NodeBase
	inactivityTimeout time.Duration
	sourceCh          chan message.Message
	stopCh            chan struct{}
	pollOnce          sync.Once
	lastSeq           uint64
}

var (
	_ Node       = (*CenturyHvacr01StatusNode)(nil)
	_ SourceNode = (*CenturyHvacr01StatusNode)(nil)
)

// NewCenturyHvacr01StatusNode 는 새로운 CenturyHvacr01StatusNode 를 생성한다.
func NewCenturyHvacr01StatusNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &CenturyHvacr01StatusNode{
		centuryHvacr01NodeBase: centuryHvacr01NodeBase{BaseNode: base},
		sourceCh:               make(chan message.Message, 64),
		stopCh:                 make(chan struct{}),
	}
	n.resolver = extractResolverFromConfig(base)
	return n, nil
}

// Configure 는 CenturyHvacr01StatusNode 설정을 적용한다.
func (n *CenturyHvacr01StatusNode) Configure(config map[string]any) error {
	if err := n.centuryHvacr01NodeBase.configure(config); err != nil {
		return err
	}
	n.mu.RLock()
	timeoutStr := n.centuryCfg.InactivityTimeout
	n.mu.RUnlock()
	inactivity, err := time.ParseDuration(timeoutStr)
	if err != nil {
		inactivity = centuryHvacr01DefaultInactivityTimeout
	}
	if inactivity < centuryHvacr01MinInactivityTimeout {
		inactivity = centuryHvacr01MinInactivityTimeout
	}
	n.inactivityTimeout = inactivity
	return nil
}

// Init 은 CenturyHvacr01StatusNode 를 초기화한다.
func (n *CenturyHvacr01StatusNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.centuryHvacr01NodeBase.initAgent(ctx); err != nil {
		return err
	}
	go n.receiveLoop()
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// receiveLoop 는 LG ICP-01 모델 (FrameNotifyCh + inactivity timer).
//
// EmitRawFrames=true 시에는 raw 모드 — inactivity / 어드레싱 우회하고
// drain (get_recent count=0) 으로 ring buffer 전체 raw frame 을 emit.
func (n *CenturyHvacr01StatusNode) receiveLoop() {
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
	cfg := n.centuryCfg
	n.mu.RUnlock()
	n.drainNewFrames(cfg)

	for {
		select {
		case <-n.stopCh:
			return
		case <-notifyCh:
			n.mu.RLock()
			cfg := n.centuryCfg
			n.mu.RUnlock()
			n.drainNewFrames(cfg)
			resetTimer()
		case <-timer.C:
			n.mu.RLock()
			cfg := n.centuryCfg
			n.mu.RUnlock()
			if !cfg.EmitRawFrames {
				n.requestStateRefresh(cfg)
			}
			resetTimer()
		}
	}
}

// requestStateRefresh 는 에이전트에 request_state 를 보내 마지막 상태를 push.
// EmitRawFrames=true 시에는 호출되지 않는다 (raw 모드는 inactivity 무관).
func (n *CenturyHvacr01StatusNode) requestStateRefresh(cfg CenturyHvacr01NodeConfig) {
	cmd := map[string]any{
		"command": "request_state",
		"node_id": n.ID(),
	}
	if cfg.UnitID != "" {
		cmd["unit_id"] = cfg.UnitID
	}
	if cfg.GroupID != "" {
		cmd["group_id"] = cfg.GroupID
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()
	_, _ = n.centuryHvacr01NodeBase.callAgentProcess(ctx, cmdBytes)
}

// drainNewFrames 는 ring buffer 의 lastSeq 이후 새 frame 을 sourceCh 로 전달한다.
//
// EmitRawFrames=true: 모든 frame 을 raw 페이로드로 emit (dedupe 적용 안 됨).
// EmitRawFrames=false: device_state drain + 어드레싱 필터.
func (n *CenturyHvacr01StatusNode) drainNewFrames(cfg CenturyHvacr01NodeConfig) {
	if cfg.EmitRawFrames {
		n.drainRawFrames(cfg)
		return
	}
	// device_state drain (change / keepalive / response).
	n.centuryHvacr01NodeBase.drainDeviceStateEvents(n.ID(), n.sourceCh)
}

// drainRawFrames 는 raw frame 모드로 ring buffer 의 모든 frame 을 emit 한다.
func (n *CenturyHvacr01StatusNode) drainRawFrames(cfg CenturyHvacr01NodeConfig) {
	frames, lastSeq, ok := centuryHvacr01RequestBulk(n.agent, &n.centuryHvacr01NodeBase, cfg.BatchSize, n.lastSeq)
	if !ok {
		return
	}
	for _, fr := range frames {
		msg, ok := buildCenturyHvacr01Message(fr, n.ID(), cfg.AgentRef, true /* rawMode */, cfg.OmitStateWhenOff, cfg.EmitMetadata)
		if !ok {
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

// Process 는 입력 메시지를 받아 단일 상태 조회를 수행한다 (AC-B6 / AC-B7 / AC-B8).
func (n *CenturyHvacr01StatusNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.centuryCfg
	n.mu.RUnlock()

	cmdBytes, err := buildCenturyHvacr01StatusCommand(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCenturyHvacr01ProcessFailed, err)
	}
	resp, err := n.centuryHvacr01NodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCenturyHvacr01ProcessFailed, err)
	}
	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrCenturyHvacr01ProcessFailed, err)
	}
	out := msg.Clone()
	promotePayloadMetadata(out, result, cfg.EmitMetadata)
	promoteDevIDWithUUID(out, result, cfg.AgentRef, cfg.EmitMetadata)
	promoteLastSeenToTimestamp(out, result)
	flattenStateToPayload(result)
	applyPowerOffFilter(result, cfg.OmitStateWhenOff)
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	if cfg.EmitMetadata.NodeID {
		out.Metadata().Set("node_id", n.ID())
	}
	out.SetType("device_state.response")
	return []message.Message{out}, nil
}

// Shutdown 은 CenturyHvacr01StatusNode 를 종료한다.
func (n *CenturyHvacr01StatusNode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() { close(n.stopCh) })
	return n.centuryHvacr01NodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 receiveLoop 를 재구성한다.
func (n *CenturyHvacr01StatusNode) Reinit(ctx context.Context) error {
	n.pollOnce.Do(func() { close(n.stopCh) })
	if err := n.centuryHvacr01NodeBase.initAgent(ctx); err != nil {
		return err
	}
	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.pollOnce = sync.Once{}
	n.mu.Unlock()
	go n.receiveLoop()
	return nil
}

// SourceCh 는 receiveLoop 가 생성한 메시지 채널을 반환한다.
func (n *CenturyHvacr01StatusNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// CenturyHvacr01ControlNode — 제어 미지원 placeholder (REQ-CENTURY-017)
// ===========================================================================

// CenturyHvacr01ControlNode 는 Century 제어 노드이다. 본 에이전트가 패시브 캡처 전용이므로
// 노드는 항상 not_supported 응답을 반환하고 agent.Process() 를 호출하지 않는다.
type CenturyHvacr01ControlNode struct {
	centuryHvacr01NodeBase
}

var _ Node = (*CenturyHvacr01ControlNode)(nil)

// NewCenturyHvacr01ControlNode 는 새로운 CenturyHvacr01ControlNode 를 생성한다.
func NewCenturyHvacr01ControlNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &CenturyHvacr01ControlNode{
		centuryHvacr01NodeBase: centuryHvacr01NodeBase{BaseNode: base},
	}
	n.resolver = extractResolverFromConfig(base)
	return n, nil
}

// Configure 는 CenturyHvacr01ControlNode 설정을 적용한다.
func (n *CenturyHvacr01ControlNode) Configure(config map[string]any) error {
	return n.centuryHvacr01NodeBase.configure(config)
}

// Init 은 CenturyHvacr01ControlNode 를 초기화한다.
func (n *CenturyHvacr01ControlNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.centuryHvacr01NodeBase.initAgent(ctx); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 어떤 메시지를 받더라도 not_supported 응답을 반환한다 (AC-C3).
func (n *CenturyHvacr01ControlNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	out := msg.Clone()
	out.Payload().Set("status", "not_supported")
	out.Payload().Set("reason", "century_passive_only")
	out.Payload().Set("message", "Century HVAC agent operates in passive sniff mode; control commands are never transmitted")
	out.Metadata().Set("century_command", "control")
	if n.centuryCfg.EmitMetadata.NodeID {
		out.Metadata().Set("node_id", n.ID())
	}
	out.SetType("device_state.response")
	return []message.Message{out}, nil
}

// Shutdown 은 CenturyHvacr01ControlNode 를 종료한다.
func (n *CenturyHvacr01ControlNode) Shutdown(_ context.Context) error {
	return n.centuryHvacr01NodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조를 갱신한다.
func (n *CenturyHvacr01ControlNode) Reinit(ctx context.Context) error {
	return n.centuryHvacr01NodeBase.initAgent(ctx)
}

// ===========================================================================
// CenturyHvacr01Node — 상태 조회 + 제어 통합 (REQ-CENTURY-018)
// ===========================================================================

// CenturyHvacr01Node 는 상태 조회 (SourceNode) 와 제어 (ProcessNode) 를 통합한 노드이다.
// 제어 키가 포함된 메시지는 not_supported 를 반환하고, 그 외에는 상태 조회를 수행한다.
//
// v0.18.26: LG ICP-01 inactivity 모델 통일.
type CenturyHvacr01Node struct {
	centuryHvacr01NodeBase
	inactivityTimeout time.Duration
	sourceCh          chan message.Message
	stopCh            chan struct{}
	pollOnce          sync.Once
	lastSeq           uint64
}

var (
	_ Node       = (*CenturyHvacr01Node)(nil)
	_ SourceNode = (*CenturyHvacr01Node)(nil)
)

// NewCenturyHvacr01Node 는 새로운 CenturyHvacr01Node 를 생성한다.
func NewCenturyHvacr01Node(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &CenturyHvacr01Node{
		centuryHvacr01NodeBase: centuryHvacr01NodeBase{BaseNode: base},
		sourceCh:               make(chan message.Message, 64),
		stopCh:                 make(chan struct{}),
	}
	n.resolver = extractResolverFromConfig(base)
	return n, nil
}

// Configure 는 CenturyHvacr01Node 설정을 적용한다.
func (n *CenturyHvacr01Node) Configure(config map[string]any) error {
	if err := n.centuryHvacr01NodeBase.configure(config); err != nil {
		return err
	}
	n.mu.RLock()
	timeoutStr := n.centuryCfg.InactivityTimeout
	n.mu.RUnlock()
	inactivity, err := time.ParseDuration(timeoutStr)
	if err != nil {
		inactivity = centuryHvacr01DefaultInactivityTimeout
	}
	if inactivity < centuryHvacr01MinInactivityTimeout {
		inactivity = centuryHvacr01MinInactivityTimeout
	}
	n.inactivityTimeout = inactivity
	return nil
}

// Init 은 CenturyHvacr01Node 를 초기화한다.
func (n *CenturyHvacr01Node) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if err := n.centuryHvacr01NodeBase.initAgent(ctx); err != nil {
		return err
	}
	go n.receiveLoop()
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// receiveLoop 는 CenturyHvacr01StatusNode 와 동일.
func (n *CenturyHvacr01Node) receiveLoop() {
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
	cfg := n.centuryCfg
	n.mu.RUnlock()
	n.drainNewFrames(cfg)

	for {
		select {
		case <-n.stopCh:
			return
		case <-notifyCh:
			n.mu.RLock()
			cfg := n.centuryCfg
			n.mu.RUnlock()
			n.drainNewFrames(cfg)
			resetTimer()
		case <-timer.C:
			n.mu.RLock()
			cfg := n.centuryCfg
			n.mu.RUnlock()
			if !cfg.EmitRawFrames {
				n.requestStateRefresh(cfg)
			}
			resetTimer()
		}
	}
}

func (n *CenturyHvacr01Node) requestStateRefresh(cfg CenturyHvacr01NodeConfig) {
	cmd := map[string]any{
		"command": "request_state",
		"node_id": n.ID(),
	}
	if cfg.UnitID != "" {
		cmd["unit_id"] = cfg.UnitID
	}
	if cfg.GroupID != "" {
		cmd["group_id"] = cfg.GroupID
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()
	_, _ = n.centuryHvacr01NodeBase.callAgentProcess(ctx, cmdBytes)
}

func (n *CenturyHvacr01Node) drainNewFrames(cfg CenturyHvacr01NodeConfig) {
	if cfg.EmitRawFrames {
		frames, lastSeq, ok := centuryHvacr01RequestBulk(n.agent, &n.centuryHvacr01NodeBase, cfg.BatchSize, n.lastSeq)
		if !ok {
			return
		}
		for _, fr := range frames {
			msg, mok := buildCenturyHvacr01Message(fr, n.ID(), cfg.AgentRef, true /* rawMode */, cfg.OmitStateWhenOff, cfg.EmitMetadata)
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
		return
	}
	n.centuryHvacr01NodeBase.drainDeviceStateEvents(n.ID(), n.sourceCh)
}

// Process 는 입력 메시지를 받는다. 제어 키가 포함되면 not_supported 응답을 반환한다 (AC-C4).
func (n *CenturyHvacr01Node) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	if hasCenturyHvacr01ControlKey(msg) {
		out := msg.Clone()
		out.Payload().Set("status", "not_supported")
		out.Payload().Set("reason", "century_passive_only")
		out.Payload().Set("message", "Century HVAC agent operates in passive sniff mode; control commands are never transmitted")
		out.Metadata().Set("century_command", "control")
		if n.centuryCfg.EmitMetadata.NodeID {
			out.Metadata().Set("node_id", n.ID())
		}
		out.SetType("device_state.response")
		return []message.Message{out}, nil
	}

	n.mu.RLock()
	cfg := n.centuryCfg
	n.mu.RUnlock()
	cmdBytes, err := buildCenturyHvacr01StatusCommand(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCenturyHvacr01ProcessFailed, err)
	}
	resp, err := n.centuryHvacr01NodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCenturyHvacr01ProcessFailed, err)
	}
	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrCenturyHvacr01ProcessFailed, err)
	}
	out := msg.Clone()
	promotePayloadMetadata(out, result, cfg.EmitMetadata)
	promoteDevIDWithUUID(out, result, cfg.AgentRef, cfg.EmitMetadata)
	promoteLastSeenToTimestamp(out, result)
	flattenStateToPayload(result)
	applyPowerOffFilter(result, cfg.OmitStateWhenOff)
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	out.Metadata().Set("century_command", "status")
	if cfg.EmitMetadata.NodeID {
		out.Metadata().Set("node_id", n.ID())
	}
	out.SetType("device_state.response")
	return []message.Message{out}, nil
}

// Shutdown 은 CenturyHvacr01Node 를 종료한다.
func (n *CenturyHvacr01Node) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() { close(n.stopCh) })
	return n.centuryHvacr01NodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 receiveLoop 를 재구성한다.
func (n *CenturyHvacr01Node) Reinit(ctx context.Context) error {
	n.pollOnce.Do(func() { close(n.stopCh) })
	if err := n.centuryHvacr01NodeBase.initAgent(ctx); err != nil {
		return err
	}
	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.pollOnce = sync.Once{}
	n.mu.Unlock()
	go n.receiveLoop()
	return nil
}

// SourceCh 는 receiveLoop 가 생성한 메시지 채널을 반환한다.
func (n *CenturyHvacr01Node) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// 헬퍼 함수
// ===========================================================================

// hasCenturyHvacr01ControlKey 는 메시지에 알려진 제어 키 중 하나가 있는지 검사한다.
func hasCenturyHvacr01ControlKey(msg message.Message) bool {
	for _, k := range centuryHvacr01ControlKeys {
		if _, ok := msg.Payload().Get(k); ok {
			return true
		}
	}
	return false
}

// buildCenturyHvacr01StatusCommand 는 Process() 단발 호출용 status 커맨드를 생성한다 (v0.18.26).
//
// inactivity 모델에서는 Process 단발 호출 시 stats 만 반환하도록 단순화.
// device_state 흐름은 receiveLoop 의 drainDeviceStateEvents 가 담당.
func buildCenturyHvacr01StatusCommand(cfg CenturyHvacr01NodeConfig) ([]byte, error) {
	_ = cfg
	cmd := map[string]any{"command": centuryHvacr01CmdGetStats}
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

// centuryHvacr01RequestBulk 는 agent 에 drain 요청을 보내고 frames 슬라이스를 반환한다.
//
// raw-frame 모드에서만 호출되며, lastSeq 보다 큰 frame 만 반환한다.
func centuryHvacr01RequestBulk(_ agent.Agent, nb *centuryHvacr01NodeBase, batchSize int, lastSeq uint64) ([]rawFrameEntry, uint64, bool) {
	if nb.agent == nil {
		return nil, lastSeq, false
	}
	if batchSize <= 0 {
		batchSize = 32
	}
	cmd := map[string]any{
		"command": centuryHvacr01CmdGetRecent,
		"count":   batchSize,
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

// buildCenturyHvacr01Message 는 한 frame entry 를 출력 message 로 변환한다.
//
// rawMode=true: 모든 frame (decoded 성공 / 실패 모두) 을 raw 페이로드로 송출.
// rawMode=false: decoded 가 있는 frame 만 송출하고 decoded 페이로드를 펼친다.
func buildCenturyHvacr01Message(fr rawFrameEntry, nodeID, agentName string, rawMode, omitStateWhenOff bool, opts MetadataEmitOptions) (message.Message, bool) {
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
		msg.Payload().Set("crc_ok", true)
		if fr.DecodeError != "" {
			msg.Payload().Set("validation_stage", centuryHvacr01ValidationStage(fr.DecodeError))
			msg.Payload().Set("decode_error", fr.DecodeError)
		} else {
			msg.Payload().Set("validation_stage", "ok")
		}
		msg.Payload().Set("confirmation_status", "raw")
		if opts.NodeID {
			msg.Metadata().Set("node_id", nodeID)
		}
		msg.SetType("raw_frame.event")
		return msg, true
	}

	if len(fr.Decoded) == 0 {
		return nil, false
	}
	var decoded map[string]any
	if err := json.Unmarshal(fr.Decoded, &decoded); err != nil {
		return nil, false
	}
	msg := message.New()
	promotePayloadMetadata(msg, decoded, opts)
	payloadType, _ := decoded["type"].(string)
	if strings.HasSuffix(payloadType, "_write_request") {
		msg.SetType("control.request")
	} else {
		applyDeviceStateMessageType(msg, decoded, "poll")
	}
	promoteDevIDWithUUID(msg, decoded, agentName, opts)
	promoteLastSeenToTimestamp(msg, decoded)
	flattenStateToPayload(decoded)
	applyPowerOffFilter(decoded, omitStateWhenOff)
	for k, v := range decoded {
		msg.Payload().Set(k, v)
	}
	msg.Payload().Set("seq", fr.Seq)
	if fr.RawHex != "" {
		msg.Payload().Set("raw_hex", fr.RawHex)
	}
	if opts.NodeSource {
		msg.Metadata().Set("node_source", "poll_bulk")
	}
	if opts.NodeID {
		msg.Metadata().Set("node_id", nodeID)
	}
	return msg, true
}

// centuryHvacr01ValidationStage 는 decoder 에러 메시지로부터 validation stage 문자열을 추정한다.
func centuryHvacr01ValidationStage(decodeErr string) string {
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
