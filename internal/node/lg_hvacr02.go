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
	hvacr02DefaultTimeout = 5 * time.Second

	// 기본 폴링 간격 (control / combined node 에서 사용 — status 는 inactivity 모델 전환됨)
	hvacr02DefaultPollInterval = 100 * time.Millisecond

	// 최소 폴링 간격
	lgHvacr02MinPollInterval = 1 * time.Millisecond

	// 2026-05-30: status node 의 inactivity-fallback 임계값 (LG HVACR-01 통일).
	lgHvacr02DefaultInactivityTimeout = 90 * time.Second
	lgHvacr02MinInactivityTimeout     = 5 * time.Second

	// 기본 LGCP 커맨드
	lgHvacr02CmdGetStats    = "get_stats"
	lgHvacr02CmdGetRecent   = "get_recent"
	lgHvacr02CmdGetAll      = "get_all"
	lgHvacr02CmdGetState    = "get_state"
	lgHvacr02CmdDrain       = "drain"
	lgHvacr02CmdSetMultiple = "set_multiple"
)

// ---------------------------------------------------------------------------
// LGHvacr02NodeConfig
// ---------------------------------------------------------------------------

// LGHvacr02NodeConfig 는 LGCP 노드 공용 설정 구조체이다.
type LGHvacr02NodeConfig struct {
	AgentRef         string `json:"agent_ref"`           // 필수: LGCP 에이전트 이름/ID
	DefaultAddress   string `json:"default_address"`     // 선택: 기본 실내기 주소 (control / combined 노드용. status 는 미사용)
	PollInterval     string `json:"poll_interval"`       // 선택: 폴링 간격 (control / combined 전용, 기본 "100ms"). status 는 inactivity 모델로 미사용.
	Timeout          string `json:"timeout"`             // 선택: Process 타임아웃 (기본 "5s")
	PollCommand      string `json:"poll_command"`        // 선택: 폴링 커맨드 (control / combined 전용). status 는 미사용.
	RecentCount      int    `json:"recent_count"`        // 선택: get_recent 시 프레임 수 (control / combined 전용). status 는 미사용.
	BatchSize        int    `json:"batch_size"`          // 선택: drain 배치 크기 (기본 32). status 의 drainNewFrames 도 사용.
	OmitStateWhenOff bool   `json:"omit_state_when_off"` // v0.18.0: power=false 시 current_temperature/mode/fan_speed 제거

	// 2026-05-30: LG HVACR-01 통일 — status 노드의 inactivity-fallback 옵션.
	InactivityTimeout string `json:"inactivity_timeout"` // 선택: 에이전트 무수신 시 request_state 호출 임계값 (기본 "90s"). status 전용.
	UnitID            string `json:"unit_id"`            // 선택: status 노드의 어드레싱 필터 (hex byte). 빈 값=broadcast.

	// EmitMetadata 는 metadata 옵션 필드의 emit 정책을 제어한다 (v0.18.8).
	// device_id / unit_id 는 항상 emit (필수), 나머지는 default OFF.
	EmitMetadata MetadataEmitOptions `json:"emit_metadata"`
}

// ---------------------------------------------------------------------------
// lgHvacr02NodeBase
// ---------------------------------------------------------------------------

// lgHvacr02NodeBase 는 LGCP 노드 공통 기반 구조체이다.
// LGHvacr02StatusNode, LGHvacr02ControlNode, LGHvacr02Node가 이를 임베딩한다.
type lgHvacr02NodeBase struct {
	*BaseNode
	lgHvacr02Cfg LGHvacr02NodeConfig
	resolver     AgentResolver
	transport    AgentTransport
	agent        agent.Agent   // 원본 LG HVACR-02 Agent 객체
	timeout      time.Duration // Process 호출 타임아웃
	mu           sync.RWMutex  // 설정 보호 뮤텍스
}

// configure 는 공통 설정 파싱을 수행한다. agent_ref(필수)를 검증한다.
func (nb *lgHvacr02NodeBase) configure(config map[string]any) error {
	if err := nb.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg LGHvacr02NodeConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrLGHvacr02MissingAgentRef
	}

	// default_address (선택)
	if v, ok := config["default_address"]; ok {
		if s, ok := v.(string); ok {
			cfg.DefaultAddress = s
		}
	}

	// poll_interval (선택, 기본값 "30s")
	cfg.PollInterval = "100ms"
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

	// poll_command (선택, 기본값 "get_recent" — 다중 노드 안전)
	cfg.PollCommand = lgHvacr02CmdGetRecent
	if v, ok := config["poll_command"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.PollCommand = s
		}
	}

	// recent_count (선택, 기본값 10)
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
	// v0.18.0: omit_state_when_off — power=false 시 불확실 상태 필드 제거.
	if v, ok := config["omit_state_when_off"].(bool); ok {
		cfg.OmitStateWhenOff = v
	}

	// 2026-05-30: LG HVACR-01 통일 — status 노드의 inactivity 모델 옵션.
	cfg.InactivityTimeout = "90s"
	if v, ok := config["inactivity_timeout"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.InactivityTimeout = s
		}
	}
	if v, ok := config["unit_id"]; ok {
		if s, ok := v.(string); ok {
			cfg.UnitID = s
		}
	}

	// v0.18.8: emit_metadata — metadata 옵션 필드 emit 정책.
	parseEmitMetadata(config, &cfg.EmitMetadata)

	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		timeout = hvacr02DefaultTimeout
	}

	nb.mu.Lock()
	nb.lgHvacr02Cfg = cfg
	nb.timeout = timeout
	nb.mu.Unlock()

	return nil
}

// initAgent 는 AgentResolver를 통해 에이전트를 resolve하고 LG LGCP 타입을 확인한다.
func (nb *lgHvacr02NodeBase) initAgent(ctx context.Context) error {
	if nb.resolver == nil {
		return ErrLGHvacr02NoResolver
	}

	nb.mu.RLock()
	agentRef := nb.lgHvacr02Cfg.AgentRef
	nb.mu.RUnlock()

	ref := flow.AgentRef{
		AgentID:   agentRef,
		AgentName: agentRef,
	}
	transport, err := nb.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("lg_hvacr02 init: agent resolve failed: %w", err)
	}
	nb.transport = transport

	// AgentAccessor를 통해 원본 Agent 객체 획득 및 타입 확인
	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return ErrLGHvacr02AgentNotLGHvacr02
	}

	underlyingAgent := accessor.UnderlyingAgent()
	switch underlyingAgent.(type) {
	case *lg.Hvacr02Agent:
		nb.agent = underlyingAgent
	default:
		return ErrLGHvacr02AgentNotLGHvacr02
	}

	return nil
}

// callAgentProcess 는 Agent.Process()를 context timeout과 함께 호출한다.
func (nb *lgHvacr02NodeBase) callAgentProcess(ctx context.Context, cmdBytes []byte) ([]byte, error) {
	if nb.agent == nil {
		return nil, ErrLGHvacr02NoResolver
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
		return nil, fmt.Errorf("lg_hvacr02: %w", timeoutCtx.Err())
	case result := <-ch:
		return result.data, result.err
	}
}

// shutdown 은 공통 종료 로직을 수행한다.
func (nb *lgHvacr02NodeBase) shutdown() error {
	return nb.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (nb *lgHvacr02NodeBase) AgentRef() flow.AgentRef {
	nb.mu.RLock()
	ref := nb.lgHvacr02Cfg.AgentRef
	nb.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// ===========================================================================
// LGHvacr02StatusNode
// ===========================================================================

// LGHvacr02StatusNode 는 LG HVACR-02 (LG ICP-02 프로토콜) 에이전트의 상태를 조회하는 노드이다.
//
// 2026-05-30 동작 모델 변경 (LG HVACR-01 v0.18.24 패턴 통일):
//   - 이전: ticker 기반 폴링 (poll_interval 마다 get_recent / drain 요청).
//   - 현재: receiveLoop — FrameNotifyCh 신호 수신 시 ring buffer drain (delta).
//     inactivityTimeout 동안 무수신 시에만 agent 에 "request_state" 명령 →
//     agent 가 각 디바이스의 마지막 상태를 push 경로로 emit → notify 수신 →
//     drain 으로 흐름 복귀.
type LGHvacr02StatusNode struct {
	lgHvacr02NodeBase
	inactivityTimeout time.Duration
	sourceCh          chan message.Message
	stopCh            chan struct{}
	pollOnce          sync.Once // stopCh close 보호
	lastSeq           int64     // 마지막으로 전송한 프레임 seq (drain delta 식별)
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*LGHvacr02StatusNode)(nil)
	_ SourceNode = (*LGHvacr02StatusNode)(nil)
)

// NewLGHvacr02StatusNode 는 새로운 LGHvacr02StatusNode를 생성하는 팩토리 함수이다.
func NewLGHvacr02StatusNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &LGHvacr02StatusNode{
		lgHvacr02NodeBase: lgHvacr02NodeBase{
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

// Configure 는 LGHvacr02StatusNode의 설정을 적용한다.
// 2026-05-30: inactivity_timeout 으로 동작 모델 변경.
func (n *LGHvacr02StatusNode) Configure(config map[string]any) error {
	if err := n.lgHvacr02NodeBase.configure(config); err != nil {
		return err
	}

	n.mu.RLock()
	timeoutStr := n.lgHvacr02Cfg.InactivityTimeout
	n.mu.RUnlock()

	inactivity, err := time.ParseDuration(timeoutStr)
	if err != nil {
		inactivity = lgHvacr02DefaultInactivityTimeout
	}
	if inactivity < lgHvacr02MinInactivityTimeout {
		inactivity = lgHvacr02MinInactivityTimeout
	}
	n.inactivityTimeout = inactivity

	return nil
}

// Init 은 LGHvacr02StatusNode를 초기화한다.
// 에이전트를 resolve하고, 폴링 고루틴을 시작한다.
func (n *LGHvacr02StatusNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.lgHvacr02NodeBase.initAgent(ctx); err != nil {
		// 에러 분류:
		// - ErrLGHvacr02NoResolver: 구성 오류, 플로우 시작 실패
		// - ErrLGHvacr02AgentNotLGHvacr02: 구성 오류 (agent 타입 불일치), 플로우 시작 실패
		// - 기타 (agent not found): 런타임 가용성 문제, 플로우는 진행하되 노드 대기
		if errors.Is(err, ErrLGHvacr02NoResolver) || errors.Is(err, ErrLGHvacr02AgentNotLGHvacr02) {
			return err // 구성 오류 전파
		}

		// 에이전트를 찾을 수 없어도 플로우는 시작되도록 함 (나중에 Reinit으로 연결)
		// 로거를 통해 경고만 출력
		if logger := n.Logger(); logger != nil {
			logger.Warn("lg_hvacr02 init: agent not available, deferring connection",
				"nodeID", n.ID(),
				"agentRef", n.lgHvacr02Cfg.AgentRef,
				"error", err,
			)
		}
		// 노드가 Running 상태로 진행하지만, 아직 폴링 루프를 시작하지 않음
		// (agent, transport는 nil 상태)
		return n.BaseNode.TransitionTo(lifecycle.StateRunning)
	}

	// 폴링 고루틴 시작 (SourceNode 지원)
	go n.receiveLoop()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// receiveLoop 는 에이전트의 push 프레임을 수신하여 sourceCh 로 전달한다.
//
// 2026-05-30 새 모델 (LG HVACR-01 v0.18.24 통일):
//   - FrameNotifyCh 신호 → drainNewFrames 로 ring buffer 의 새 frame (lastSeq
//     이후) 을 sourceCh 로 전달.
//   - inactivityTimeout 동안 무신호 → "request_state" 를 agent 에 발송.
//     agent 가 각 device 마지막 상태를 push 경로로 emit → notify 신호 →
//     drain 으로 메시지 수신.
//   - 첫 진입 시점에도 즉시 1회 drain (기존 ring buffer 의 frame 흡수).
func (n *LGHvacr02StatusNode) receiveLoop() {
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
	cfg := n.lgHvacr02Cfg
	n.mu.RUnlock()
	n.drainNewFrames(cfg)

	for {
		select {
		case <-n.stopCh:
			return
		case <-notifyCh:
			n.mu.RLock()
			cfg := n.lgHvacr02Cfg
			n.mu.RUnlock()
			n.drainNewFrames(cfg)
			resetTimer()
		case <-timer.C:
			n.mu.RLock()
			cfg := n.lgHvacr02Cfg
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
//
// cfg.UnitID 가 설정되어 있으면 target unit_id 를 전달 (agent 가 지원할 때).
// 빈 값이면 broadcast.
func (n *LGHvacr02StatusNode) requestStateRefresh(cfg LGHvacr02NodeConfig) {
	cmd := map[string]any{
		"command": "request_state",
		"node_id": n.ID(),
	}
	if cfg.UnitID != "" {
		cmd["unit_id"] = cfg.UnitID
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()
	_, _ = n.lgHvacr02NodeBase.callAgentProcess(ctx, cmdBytes)
}

// drainNewFrames 는 ring buffer 의 lastSeq 이후 새 frame 을 sourceCh 로 전달한다.
// agent push 경로 (handleCapturedFrame / request_state) 의 모든 emit 을 동일한
// delta 로 처리하므로 중복 emit 없이 흐름 보장.
//
// cfg.UnitID 가 비어있지 않으면 payload.unit_id 가 매칭되는 프레임만 emit.
func (n *LGHvacr02StatusNode) drainNewFrames(cfg LGHvacr02NodeConfig) {
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
	resp, err := n.lgHvacr02NodeBase.callAgentProcess(ctx, cmdBytes)
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

	// get_recent 는 최신→오래된 순서로 반환 — 시간순 전송을 위해 역순 순회.
	for i := len(result.Frames) - 1; i >= 0; i-- {
		var payload map[string]any
		if err := json.Unmarshal(result.Frames[i], &payload); err != nil {
			continue
		}
		// unit_id 어드레싱 필터 — payload.unit_id 를 cfg.UnitID 와 비교.
		if !lgHvacr02MatchAddressing(payload, cfg) {
			continue
		}

		var frameSeq struct {
			Seq int64 `json:"seq"`
		}
		_ = json.Unmarshal(result.Frames[i], &frameSeq)
		if frameSeq.Seq > 0 && frameSeq.Seq <= n.lastSeq {
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
			if frameSeq.Seq > n.lastSeq {
				n.lastSeq = frameSeq.Seq
			}
		case <-n.stopCh:
			return
		}
	}

	if result.LastSeq > n.lastSeq {
		n.lastSeq = result.LastSeq
	}
}

// lgHvacr02BulkFrame 는 combined 노드의 pollRecentBulk 에서 사용하는 내부 구조체이다.
type lgHvacr02BulkFrame struct {
	seq  int64
	data json.RawMessage
}

// lgHvacr02MatchAddressing 는 payload 의 unit_id 가 cfg 의 어드레싱과 매칭되는지
// 확인한다. cfg.UnitID 가 비어있으면 true (필터 없음).
func lgHvacr02MatchAddressing(payload map[string]any, cfg LGHvacr02NodeConfig) bool {
	if cfg.UnitID == "" {
		return true
	}
	raw, ok := payload["unit_id"]
	if !ok {
		return false
	}
	got, ok := raw.(string)
	if !ok {
		return false
	}
	return hexByteEqual(got, cfg.UnitID)
}

// Process 는 입력 메시지를 받아 상태 조회를 수행하고 결과를 반환한다.
func (n *LGHvacr02StatusNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.lgHvacr02Cfg
	n.mu.RUnlock()

	// 메시지 payload에서 오버라이드 적용
	cfg = applyLGHvacr02Overrides(msg, cfg)

	cmdBytes, err := buildLGHvacr02StatusCommand(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGHvacr02ProcessFailed, err)
	}

	resp, err := n.lgHvacr02NodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGHvacr02ProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrLGHvacr02ProcessFailed, err)
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
	// v0.10.0: lg_hvacr02_source="request" 제거 (message_type="device_state.response" 와 중복).
	out.SetType("device_state.response")

	return []message.Message{out}, nil
}

// Shutdown 은 LGHvacr02StatusNode를 종료한다. 폴링 고루틴을 정지한다.
func (n *LGHvacr02StatusNode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})
	return n.lgHvacr02NodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 FrameNotifier 채널 구독을
// 재구성한다. pollLoop 가 nb.agent 와 FrameNotifyCh() 를 고루틴 시작 시 한 번
// 캡처하므로 폴링 고루틴을 종료한 뒤 새 stopCh / pollOnce 로 재시작한다.
func (n *LGHvacr02StatusNode) Reinit(ctx context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})

	if err := n.lgHvacr02NodeBase.initAgent(ctx); err != nil {
		return err
	}

	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.pollOnce = sync.Once{}
	n.mu.Unlock()

	go n.receiveLoop()
	return nil
}

// SourceCh 는 폴링으로 생성된 메시지를 수신하는 채널을 반환한다.
func (n *LGHvacr02StatusNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// LGHvacr02ControlNode
// ===========================================================================

// LGHvacr02ControlNode 는 LG LGCP 에이전트에 제어 명령을 전송하는 노드이다.
type LGHvacr02ControlNode struct {
	lgHvacr02NodeBase
}

// 인터페이스 컴파일 체크
var _ Node = (*LGHvacr02ControlNode)(nil)

// NewLGHvacr02ControlNode 는 새로운 LGHvacr02ControlNode를 생성하는 팩토리 함수이다.
func NewLGHvacr02ControlNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &LGHvacr02ControlNode{
		lgHvacr02NodeBase: lgHvacr02NodeBase{
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

// Configure 는 LGHvacr02ControlNode의 설정을 적용한다.
func (n *LGHvacr02ControlNode) Configure(config map[string]any) error {
	return n.lgHvacr02NodeBase.configure(config)
}

// Init 은 LGHvacr02ControlNode를 초기화한다. 에이전트를 resolve한다.
func (n *LGHvacr02ControlNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.lgHvacr02NodeBase.initAgent(ctx); err != nil {
		// 에러 분류:
		// - ErrLGHvacr02NoResolver: 구성 오류, 플로우 시작 실패
		// - ErrLGHvacr02AgentNotLGHvacr02: 구성 오류 (agent 타입 불일치), 플로우 시작 실패
		// - 기타 (agent not found): 런타임 가용성 문제, 플로우는 진행하되 노드 대기
		if errors.Is(err, ErrLGHvacr02NoResolver) || errors.Is(err, ErrLGHvacr02AgentNotLGHvacr02) {
			return err // 구성 오류 전파
		}

		// 에이전트를 찾을 수 없어도 플로우는 시작되도록 함 (나중에 Reinit으로 연결)
		// 로거를 통해 경고만 출력
		if logger := n.Logger(); logger != nil {
			logger.Warn("lg_hvacr02 control init: agent not available, deferring connection",
				"nodeID", n.ID(),
				"agentRef", n.lgHvacr02Cfg.AgentRef,
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
// 제어 명령에는 address가 필수이다 (payload "address" > cfg.DefaultAddress > 에러).
func (n *LGHvacr02ControlNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.lgHvacr02Cfg
	n.mu.RUnlock()

	cfg = applyLGHvacr02Overrides(msg, cfg)

	cmdBytes, err := buildLGHvacr02ControlCommand(msg, cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGHvacr02ProcessFailed, err)
	}

	resp, err := n.lgHvacr02NodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGHvacr02ProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrLGHvacr02ProcessFailed, err)
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
	out.Metadata().Set("lg_hvacr02_command", "control")
	if cfg.EmitMetadata.NodeID {
		out.Metadata().Set("node_id", n.ID())
	}
	out.SetType("device_state.response")

	return []message.Message{out}, nil
}

// Shutdown 은 LGHvacr02ControlNode를 종료한다.
func (n *LGHvacr02ControlNode) Shutdown(_ context.Context) error {
	return n.lgHvacr02NodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조를 갱신한다.
// 고루틴이 없는 process-only 노드이므로 initAgent 만 재호출한다.
func (n *LGHvacr02ControlNode) Reinit(ctx context.Context) error {
	return n.lgHvacr02NodeBase.initAgent(ctx)
}

// ===========================================================================
// LGHvacr02Node
// ===========================================================================

// LGHvacr02Node 는 LG LGCP 에이전트의 상태 조회와 제어를 모두 수행하는 통합 노드이다.
// SourceNode 인터페이스를 구현하여 폴링 기반 자체 메시지 생성도 지원한다.
// payload에 제어 키(power, mode, temperature, fan_speed)가 있으면 제어,
// 없으면 상태 조회로 자동 감지한다.
type LGHvacr02Node struct {
	lgHvacr02NodeBase
	pollInterval time.Duration
	sourceCh     chan message.Message
	stopCh       chan struct{}
	pollOnce     sync.Once
	lastSeq      int64 // 마지막으로 전송한 프레임 seq (벌크 중복 제거용)
	// v0.7.7: pollSingle byte-equal dedup.
	lastSingleResp []byte
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*LGHvacr02Node)(nil)
	_ SourceNode = (*LGHvacr02Node)(nil)
)

// NewLGHvacr02Node 는 새로운 LGHvacr02Node를 생성하는 팩토리 함수이다.
func NewLGHvacr02Node(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &LGHvacr02Node{
		lgHvacr02NodeBase: lgHvacr02NodeBase{
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

// Configure 는 LGHvacr02Node의 설정을 적용한다.
func (n *LGHvacr02Node) Configure(config map[string]any) error {
	if err := n.lgHvacr02NodeBase.configure(config); err != nil {
		return err
	}

	// poll_interval 파싱
	n.mu.RLock()
	pollStr := n.lgHvacr02Cfg.PollInterval
	n.mu.RUnlock()

	pollInterval, err := time.ParseDuration(pollStr)
	if err != nil {
		pollInterval = hvacr02DefaultPollInterval
	}
	if pollInterval < lgHvacr02MinPollInterval {
		pollInterval = lgHvacr02MinPollInterval
	}
	n.pollInterval = pollInterval

	return nil
}

// Init 은 LGHvacr02Node를 초기화한다.
// 에이전트를 resolve하고, 폴링 고루틴을 시작한다.
func (n *LGHvacr02Node) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.lgHvacr02NodeBase.initAgent(ctx); err != nil {
		// 에러 분류:
		// - ErrLGHvacr02NoResolver: 구성 오류, 플로우 시작 실패
		// - ErrLGHvacr02AgentNotLGHvacr02: 구성 오류 (agent 타입 불일치), 플로우 시작 실패
		// - 기타 (agent not found): 런타임 가용성 문제, 플로우는 진행하되 노드 대기
		if errors.Is(err, ErrLGHvacr02NoResolver) || errors.Is(err, ErrLGHvacr02AgentNotLGHvacr02) {
			return err // 구성 오류 전파
		}

		// 에이전트를 찾을 수 없어도 플로우는 시작되도록 함 (나중에 Reinit으로 연결)
		// 로거를 통해 경고만 출력
		if logger := n.Logger(); logger != nil {
			logger.Warn("lg_hvacr02 source init: agent not available, deferring connection",
				"nodeID", n.ID(),
				"agentRef", n.lgHvacr02Cfg.AgentRef,
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

// pollLoop 는 에이전트에서 상태를 조회하여 sourceCh에 메시지를 전달한다.
// 에이전트가 FrameNotifier를 구현하면 새 프레임 도착 즉시 폴링하고,
// 그렇지 않으면 설정된 간격(poll_interval)으로 폴백한다.
func (n *LGHvacr02Node) pollLoop() {
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	// 에이전트가 FrameNotifier를 구현하면 즉시 알림 수신
	var notifyCh <-chan struct{}
	if fn, ok := n.agent.(agent.FrameNotifier); ok {
		notifyCh = fn.FrameNotifyCh()
	}

	poll := func() {
		n.mu.RLock()
		cfg := n.lgHvacr02Cfg
		n.mu.RUnlock()

		switch cfg.PollCommand {
		case lgHvacr02CmdGetRecent, lgHvacr02CmdDrain:
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

// pollSingle 는 get_stats 등 단일 응답 커맨드를 처리한다.
//
// v0.7.7: byte-equal dedup.
func (n *LGHvacr02Node) pollSingle(cfg LGHvacr02NodeConfig) {
	cmdBytes, err := buildLGHvacr02StatusCommand(cfg)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	resp, err := n.lgHvacr02NodeBase.callAgentProcess(ctx, cmdBytes)
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
	// v0.7.14: payload 내부의 metadata 그룹을 message metadata 로 promote.
	promotePayloadMetadata(msg, result, cfg.EmitMetadata)
	// v0.8.0: payload.trigger → metadata.message_type. trigger 없으면 "poll" fallback.
	applyDeviceStateMessageType(msg, result, "poll")
	// v0.12.0: payload.dev_id → metadata.dev_id, payload.last_seen_ms → msg.Timestamp.
	promoteDevIDWithUUID(msg, result, cfg.AgentRef, cfg.EmitMetadata)
	promoteLastSeenToTimestamp(msg, result)
	flattenStateToPayload(result)
	// v0.18.0: power=false 시 신뢰할 수 없는 상태 필드 제거.
	applyPowerOffFilter(result, cfg.OmitStateWhenOff)
	for k, v := range result {
		msg.Payload().Set(k, v)
	}
	if cfg.EmitMetadata.NodeSource {
		msg.Metadata().Set("node_source", "poll")
	}
	if cfg.EmitMetadata.NodeID {
		if cfg.EmitMetadata.NodeID {
			msg.Metadata().Set("node_id", n.ID())
		}
	}

	select {
	case n.sourceCh <- msg:
	default:
	}
}

// pollRecentBulk 는 get_recent 또는 drain 커맨드로 벌크 수신하여 새 프레임만 개별 메시지로 전송한다.
func (n *LGHvacr02Node) pollRecentBulk(cfg LGHvacr02NodeConfig) {
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
	resp, err := n.lgHvacr02NodeBase.callAgentProcess(ctx, cmdBytes)
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

	// get_recent는 최신→오래된 순서로 반환하므로 역순으로 순회하여 시간순 전송
	newFrames := make([]lgHvacr02BulkFrame, 0, len(result.Frames))
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
		newFrames = append(newFrames, lgHvacr02BulkFrame{
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
		// v0.7.14: payload 내부의 metadata 그룹을 message metadata 로 promote.
		promotePayloadMetadata(msg, payload, cfg.EmitMetadata)
		// v0.8.0: payload.trigger → metadata.message_type. trigger 없으면 "poll" fallback.
		applyDeviceStateMessageType(msg, payload, "poll")
		// v0.12.0: payload.dev_id → metadata.dev_id, payload.last_seen_ms → msg.Timestamp.
		promoteDevIDWithUUID(msg, payload, cfg.AgentRef, cfg.EmitMetadata)
		promoteLastSeenToTimestamp(msg, payload)
		flattenStateToPayload(payload)
		// v0.18.0: power=false 시 신뢰할 수 없는 상태 필드 제거.
		applyPowerOffFilter(payload, cfg.OmitStateWhenOff)
		for k, v := range payload {
			msg.Payload().Set(k, v)
		}
		if cfg.EmitMetadata.NodeSource {
			msg.Metadata().Set("node_source", "poll_bulk")
		}
		if cfg.EmitMetadata.NodeID {
			if cfg.EmitMetadata.NodeID {
				msg.Metadata().Set("node_id", n.ID())
			}
		}

		select {
		case n.sourceCh <- msg:
			n.lastSeq = f.seq
		default:
			return
		}
	}
}

// Process 는 입력 메시지를 받아 자동으로 상태 조회 또는 제어를 수행한다.
// payload에 제어 키(power, mode, temperature, fan_speed)가 있으면 제어 명령,
// 없으면 상태 조회 명령을 전송한다.
func (n *LGHvacr02Node) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.lgHvacr02Cfg
	n.mu.RUnlock()

	cfg = applyLGHvacr02Overrides(msg, cfg)

	var cmdBytes []byte
	var err error
	var cmdType string

	if hasLGHvacr02ControlKeys(msg) {
		cmdBytes, err = buildLGHvacr02ControlCommand(msg, cfg)
		cmdType = "control"
	} else {
		cmdBytes, err = buildLGHvacr02StatusCommand(cfg)
		cmdType = "status"
	}

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGHvacr02ProcessFailed, err)
	}

	resp, err := n.lgHvacr02NodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGHvacr02ProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrLGHvacr02ProcessFailed, err)
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
	out.Metadata().Set("lg_hvacr02_command", cmdType)
	if cfg.EmitMetadata.NodeID {
		out.Metadata().Set("node_id", n.ID())
	}
	out.SetType("device_state.response")

	return []message.Message{out}, nil
}

// Shutdown 은 LGHvacr02Node를 종료한다. 폴링 고루틴을 정지한다.
func (n *LGHvacr02Node) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})
	return n.lgHvacr02NodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 FrameNotifier 채널 구독을
// 재구성한다 (LGHvacr02StatusNode.Reinit 과 동일한 패턴).
func (n *LGHvacr02Node) Reinit(ctx context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})

	if err := n.lgHvacr02NodeBase.initAgent(ctx); err != nil {
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
func (n *LGHvacr02Node) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// 헬퍼 함수
// ===========================================================================

// lgHvacr02ControlKeys 는 LGCP 제어 명령으로 인식되는 payload 키 목록이다.
var lgHvacr02ControlKeys = []string{"power", "mode", "temperature", "fan_speed"}

// hasLGHvacr02ControlKeys 는 메시지 payload에 제어 키가 하나라도 있는지 확인한다.
func hasLGHvacr02ControlKeys(msg message.Message) bool {
	for _, key := range lgHvacr02ControlKeys {
		if _, ok := msg.Payload().Get(key); ok {
			return true
		}
	}
	return false
}

// buildLGHvacr02StatusCommand 는 상태 조회용 JSON 커맨드를 생성한다.
//
// poll_command 별 동작:
//   - get_recent: count = recent_count (count==0 이면 drain)
//   - get_all:    모든 device 즉시 snapshot
//   - get_state:  단일 device 즉시 snapshot (address 필수)
//   - drain:      v0.7.1 deprecated (get_recent + count=0)
//   - 그 외:      get_stats
func buildLGHvacr02StatusCommand(cfg LGHvacr02NodeConfig) ([]byte, error) {
	cmd := map[string]any{}

	switch cfg.PollCommand {
	case lgHvacr02CmdGetRecent:
		cmd["command"] = lgHvacr02CmdGetRecent
		cmd["count"] = cfg.RecentCount
	case lgHvacr02CmdGetAll:
		cmd["command"] = lgHvacr02CmdGetAll
	case lgHvacr02CmdGetState:
		cmd["command"] = lgHvacr02CmdGetState
	case lgHvacr02CmdDrain:
		cmd["command"] = lgHvacr02CmdDrain
	default:
		cmd["command"] = lgHvacr02CmdGetStats
	}

	if cfg.DefaultAddress != "" {
		cmd["address"] = cfg.DefaultAddress
	}

	return json.Marshal(cmd)
}

// buildLGHvacr02ControlCommand 는 제어용 JSON 커맨드를 생성한다.
// payload에 "command" 키가 있으면 직접 커맨드로 전달하고,
// 없으면 제어 키(power, mode, temperature, fan_speed)를 수집하여 set_multiple을 구성한다.
// 제어 명령에는 address가 필수이다 (payload "address" > cfg.DefaultAddress > ErrLGHvacr02MissingAddress).
func buildLGHvacr02ControlCommand(msg message.Message, cfg LGHvacr02NodeConfig) ([]byte, error) {
	// address 결정: payload "address" > cfg.DefaultAddress
	address := cfg.DefaultAddress
	if v, ok := msg.Payload().Get("address"); ok {
		if s, ok := v.(string); ok && s != "" {
			address = s
		}
	}

	// payload에 "command" 키가 있으면 직접 전달
	if v, ok := msg.Payload().Get("command"); ok {
		if cmdStr, ok := v.(string); ok && cmdStr != "" {
			if address == "" {
				return nil, ErrLGHvacr02MissingAddress
			}
			cmd := map[string]any{
				"command": cmdStr,
				"address": address,
			}
			// payload에서 params 추출
			if params, ok := msg.Payload().Get("params"); ok {
				cmd["params"] = params
			}
			return json.Marshal(cmd)
		}
	}

	// 제어 키에서 set_multiple 구성
	params := map[string]any{}
	for _, key := range lgHvacr02ControlKeys {
		if v, ok := msg.Payload().Get(key); ok {
			params[key] = v
		}
	}

	if len(params) == 0 {
		// 제어 키가 없으면 상태 조회로 폴백
		return buildLGHvacr02StatusCommand(cfg)
	}

	if address == "" {
		return nil, ErrLGHvacr02MissingAddress
	}

	cmd := map[string]any{
		"command": lgHvacr02CmdSetMultiple,
		"address": address,
		"params":  params,
	}

	return json.Marshal(cmd)
}

// applyLGHvacr02Overrides 는 입력 메시지 payload에서 address, timeout, poll_command, count를 오버라이드한다.
func applyLGHvacr02Overrides(msg message.Message, cfg LGHvacr02NodeConfig) LGHvacr02NodeConfig {
	if v, ok := msg.Payload().Get("address"); ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.DefaultAddress = s
		}
	}
	if v, ok := msg.Payload().Get("timeout"); ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.Timeout = s
		}
	}
	if v, ok := msg.Payload().Get("poll_command"); ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.PollCommand = s
		}
	}
	if v, ok := msg.Payload().Get("count"); ok {
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
	return cfg
}
