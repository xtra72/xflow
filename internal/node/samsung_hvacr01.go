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

	// v0.18.26: inactivity-fallback 기본 시간 (LG ICP-01 통일).
	nasaDefaultInactivityTimeout = 90 * time.Second
	nasaMinInactivityTimeout     = 5 * time.Second

	// 기본 NASA 커맨드 (v0.7.1: 5 HVAC 노드 명령 통일 — get_recent / get_all).
	// 이전 get_all_states / get_recent_states 는 에이전트 측 deprecation alias.
	nasaCmdGetState        = "get_state"
	nasaCmdGetAllState     = "get_all"
	nasaCmdGetRecentStates = "get_recent"
	nasaCmdSetPower        = "set_power"
	nasaCmdSetMode         = "set_mode"
	nasaCmdSetTemp         = "target_temperature"
	nasaCmdSetFanSpeed     = "set_fan_speed"
	nasaCmdSetMultiple     = "set_multiple"

	// SPEC-HVACR-SYNC-001 M10: mirror-message 노드 I/O marker.
	//
	// 엔진은 입력 와이어를 하나의 fan-in 스트림으로 병합한 뒤 Process 를 호출하므로,
	// mirror-in 포트는 포트 이름으로 제어 in 포트와 구별할 수 없다. 따라서 Process 는
	// 메시지 타입 marker(mirrorUplinkMsgType)로 판별해 mirror 업링크면 FeedMirrorWire 로
	// 급전하고, 아니면 기존 제어/상태 처리로 폴백한다.
	mirrorUplinkMsgType = "mirror.uplink"
	// mirrorWirePayloadKey 는 미러 업링크 와이어 JSON 을 담는 payload 키이다.
	// mirror-out 은 이 키로 방출하고, mirror-in 은 이 키에서 읽어 FeedMirrorWire 에 넘긴다.
	mirrorWirePayloadKey = "mirror_wire"
	// mirrorOutSourceBufferSize 는 mirror-out 소스 포트 채널의 버퍼 크기이다.
	mirrorOutSourceBufferSize = 64
)

// ---------------------------------------------------------------------------
// SamsungHvacr01NodeConfig
// ---------------------------------------------------------------------------

// SamsungHvacr01NodeConfig 는 Samsung HVACR-01 노드 공용 설정 구조체이다.
//
// v0.18.26 (2026-05-28): polling 모델 → LG inactivity 모델 통일.
//   - 제거: device_id, poll_interval, poll_command
//   - 추가: inactivity_timeout, group_id, unit_id
//
// group_id / unit_id 는 프로토콜 어드레싱 (advanced):
//
//	group_id : NASA addr byte 1 (외기 인덱스, "00"-"0F"). 빈 값이면 모든 group.
//	unit_id  : NASA addr byte 2 ("00"-"3F" indoor 인덱스; outdoor 는 group_id 와 동일).
//	           빈 값이면 모든 unit.
//
// 두 필드 모두 비어있으면 모든 디바이스 frame 처리 + broadcast request_state.
type SamsungHvacr01NodeConfig struct {
	AgentRef          string `json:"agent_ref"`           // 대상 Samsung NASA Agent 이름/ID (필수)
	InactivityTimeout string `json:"inactivity_timeout"`  // v0.18.26: inactivity-fallback 시간 (기본 "90s")
	Timeout           string `json:"timeout"`             // Process 호출 타임아웃 (선택, 기본값 "5s")
	BatchSize         int    `json:"batch_size"`          // 벌크 수신 수량 (선택, 기본 32)
	OmitStateWhenOff  bool   `json:"omit_state_when_off"` // v0.18.0: power=false 시 current_temperature/mode/fan_speed 제거

	// v0.18.26: 어드레싱 (advanced).
	GroupID string `json:"group_id,omitempty"`
	UnitID  string `json:"unit_id,omitempty"`

	// EmitMetadata 는 metadata 옵션 필드의 emit 정책을 제어한다 (v0.18.8, v0.18.26).
	// device_id 만 default emit, 나머지는 default OFF.
	EmitMetadata MetadataEmitOptions `json:"emit_metadata"`
}

// ---------------------------------------------------------------------------
// samsungHvacr01NodeBase
// ---------------------------------------------------------------------------

// samsungHvacr01NodeBase 는 Samsung HVACR-01 노드 공통 기반 구조체이다.
type samsungHvacr01NodeBase struct {
	*BaseNode
	hvacr01Cfg SamsungHvacr01NodeConfig
	resolver   AgentResolver
	transport  AgentTransport
	agent      agent.Agent
	timeout    time.Duration
	mu         sync.RWMutex
}

// configure 는 공통 설정 파싱을 수행한다. agent_ref (필수) 를 검증한다.
func (nb *samsungHvacr01NodeBase) configure(config map[string]any) error {
	if err := nb.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg SamsungHvacr01NodeConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrNASAMissingAgentRef
	}

	// inactivity_timeout (v0.18.26, 기본 "90s") — receiveLoop 의 무수신 fallback 임계.
	cfg.InactivityTimeout = "90s"
	if v, ok := config["inactivity_timeout"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.InactivityTimeout = s
		}
	}

	// timeout (선택, 기본값 "5s")
	cfg.Timeout = "5s"
	if v, ok := config["timeout"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.Timeout = s
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

	// v0.18.0: omit_state_when_off — power=false 시 불확실 상태 필드 제거.
	if v, ok := config["omit_state_when_off"].(bool); ok {
		cfg.OmitStateWhenOff = v
	}

	// v0.18.26: 어드레싱 (advanced).
	if v, ok := config["group_id"].(string); ok {
		cfg.GroupID = v
	}
	if v, ok := config["unit_id"].(string); ok {
		cfg.UnitID = v
	}

	// v0.18.8: emit_metadata — metadata 옵션 필드 emit 정책.
	parseEmitMetadata(config, &cfg.EmitMetadata)

	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		timeout = nasaDefaultTimeout
	}

	nb.mu.Lock()
	nb.hvacr01Cfg = cfg
	nb.timeout = timeout
	nb.mu.Unlock()

	return nil
}

// initAgent 는 AgentResolver를 통해 에이전트를 resolve하고 Samsung NASA 타입을 확인한다.
func (nb *samsungHvacr01NodeBase) initAgent(ctx context.Context) error {
	if nb.resolver == nil {
		return ErrNASANoResolver
	}

	nb.mu.RLock()
	agentRef := nb.hvacr01Cfg.AgentRef
	nb.mu.RUnlock()

	ref := flow.AgentRef{
		AgentID:   agentRef,
		AgentName: agentRef,
	}
	transport, err := nb.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("samsung_hvacr01 init: agent resolve failed: %w", err)
	}
	nb.transport = transport

	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return ErrNASAAgentNotNASA
	}

	underlyingAgent := accessor.UnderlyingAgent()
	switch underlyingAgent.(type) {
	case *samsung.Hvacr01Agent:
		nb.agent = underlyingAgent
	default:
		return ErrNASAAgentNotNASA
	}

	return nil
}

// callAgentProcess 는 Agent.Process()를 context timeout과 함께 호출한다.
func (nb *samsungHvacr01NodeBase) callAgentProcess(ctx context.Context, cmdBytes []byte) ([]byte, error) {
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
		return nil, fmt.Errorf("samsung_nasa: %w", timeoutCtx.Err())
	case result := <-ch:
		return result.data, result.err
	}
}

// shutdown 은 공통 종료 로직을 수행한다.
func (nb *samsungHvacr01NodeBase) shutdown() error {
	return nb.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (nb *samsungHvacr01NodeBase) AgentRef() flow.AgentRef {
	nb.mu.RLock()
	ref := nb.hvacr01Cfg.AgentRef
	nb.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// ---------------------------------------------------------------------------
// 어드레싱 매칭 (v0.18.26)
// ---------------------------------------------------------------------------

// samsungHvacr01MatchAddressing 은 디바이스 페이로드의 NASA 주소 (address /
// unit_id) 가 cfg 의 group_id / unit_id 어드레싱과 매칭되는지 확인한다.
//
// 매칭 전략:
//   - cfg.GroupID 와 cfg.UnitID 가 모두 비어있으면 항상 true (필터 없음).
//   - Samsung 디바이스 payload 의 "address" 필드는 NASA 6-bit address 형식
//     ("AA.BB.CC" 또는 "1.2.0" 등). byte 1 = group, byte 2 = unit.
//   - "unit_id" 필드는 v0.18.7 부터 effectiveDeviceID 가 들어가는데 (사용자
//     label 우선, 없으면 "AABBCC" hex), 어드레싱 비교용으로는 address 우선.
//   - address 가 파싱 불가하면 매칭 실패 (false).
func samsungHvacr01MatchAddressing(payload map[string]any, cfg SamsungHvacr01NodeConfig) bool {
	if cfg.GroupID == "" && cfg.UnitID == "" {
		return true
	}
	addrRaw, ok := payload["address"]
	if !ok {
		return false
	}
	addr, ok := addrRaw.(string)
	if !ok {
		return false
	}
	groupByte, unitByte, ok := samsungParseAddressBytes(addr)
	if !ok {
		return false
	}
	if cfg.GroupID != "" {
		want, ok := parseHexByte(cfg.GroupID)
		if !ok {
			return false
		}
		if groupByte != want {
			return false
		}
	}
	if cfg.UnitID != "" {
		want, ok := parseHexByte(cfg.UnitID)
		if !ok {
			return false
		}
		if unitByte != want {
			return false
		}
	}
	return true
}

// samsungParseAddressBytes 는 NASA address 문자열에서 byte 1 (group) / byte 2 (unit) 를 추출한다.
//
// 지원 형식 (실제 emit 경로 확인):
//   - "AA.BB.CC" 점 구분 hex (예: "10.0F.00")
//   - "AABBCC" 연속 6 hex chars (예: "100F00")
//
// 그 외 형식은 (0, 0, false).
func samsungParseAddressBytes(addr string) (group byte, unit byte, ok bool) {
	addr = strings.TrimSpace(addr)
	if strings.Contains(addr, ".") {
		parts := strings.Split(addr, ".")
		if len(parts) < 2 {
			return 0, 0, false
		}
		g, err1 := strconv.ParseUint(parts[0], 16, 8)
		u, err2 := strconv.ParseUint(parts[1], 16, 8)
		if err1 != nil || err2 != nil {
			return 0, 0, false
		}
		return byte(g), byte(u), true
	}
	if len(addr) == 6 {
		g, err1 := strconv.ParseUint(addr[0:2], 16, 8)
		u, err2 := strconv.ParseUint(addr[2:4], 16, 8)
		if err1 != nil || err2 != nil {
			return 0, 0, false
		}
		return byte(g), byte(u), true
	}
	return 0, 0, false
}

// applySamsungHvacr01Overrides 는 입력 메시지 payload에서 device_id, unit_id, timeout 을 노드 cfg 에
// 잠시 덮어쓰기 한다. Control / Process 경로에서 사용 — Web UI 에서 control 명령에 동봉되는
// device_id (또는 unit_id) 를 인식한다.
//
// v0.18.26: cfg.DeviceID 가 제거되었으므로 별도 string 으로 반환. control command
// 빌드 함수가 이 값을 받아 사용한다.
func applySamsungHvacr01Overrides(msg message.Message, cfg SamsungHvacr01NodeConfig) (SamsungHvacr01NodeConfig, string) {
	deviceID := ""
	if v, ok := msg.Payload().Get("device_id"); ok {
		if s, ok := v.(string); ok && s != "" {
			deviceID = s
		}
	}
	// unit_id 가 명시되었으면 control target 으로 사용 (device_id 보다 우선).
	if v, ok := msg.Payload().Get("unit_id"); ok {
		if s, ok := v.(string); ok && s != "" {
			deviceID = s
		}
	}
	if v, ok := msg.Payload().Get("timeout"); ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.Timeout = s
		}
	}
	return cfg, deviceID
}

// ===========================================================================
// SamsungHvacr01StatusNode — 상태 조회 전용 (SourceNode)
// ===========================================================================

// SamsungHvacr01StatusNode 는 Samsung NASA 에이전트의 상태를 조회하는 노드이다.
//
// v0.18.26 (2026-05-28) 동작 모델 변경 (LG ICP-01 통일):
//   - 이전: ticker 기반 폴링 (poll_interval 마다 get_recent_states / get_all_states).
//   - 현재: receiveLoop — FrameNotifyCh 신호 수신 시 ring buffer 의 새 snapshot drain.
//     inactivityTimeout 동안 무수신 시에만 agent 에 "request_state" 명령 →
//     agent 가 각 디바이스의 마지막 상태를 push 경로로 emit → notify 수신 →
//     drain 으로 흐름 복귀.
type SamsungHvacr01StatusNode struct {
	samsungHvacr01NodeBase
	inactivityTimeout time.Duration
	sourceCh          chan message.Message
	stopCh            chan struct{}
	pollOnce          sync.Once
	lastSeq           int64
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*SamsungHvacr01StatusNode)(nil)
	_ SourceNode = (*SamsungHvacr01StatusNode)(nil)
)

// NewSamsungHvacr01StatusNode 는 새로운 SamsungHvacr01StatusNode를 생성하는 팩토리 함수이다.
func NewSamsungHvacr01StatusNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &SamsungHvacr01StatusNode{
		samsungHvacr01NodeBase: samsungHvacr01NodeBase{
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

// Configure 는 SamsungHvacr01StatusNode의 설정을 적용한다.
func (n *SamsungHvacr01StatusNode) Configure(config map[string]any) error {
	if err := n.samsungHvacr01NodeBase.configure(config); err != nil {
		return err
	}

	n.mu.RLock()
	timeoutStr := n.hvacr01Cfg.InactivityTimeout
	n.mu.RUnlock()

	inactivity, err := time.ParseDuration(timeoutStr)
	if err != nil {
		inactivity = nasaDefaultInactivityTimeout
	}
	if inactivity < nasaMinInactivityTimeout {
		inactivity = nasaMinInactivityTimeout
	}
	n.inactivityTimeout = inactivity

	return nil
}

// Init 은 SamsungHvacr01StatusNode를 초기화한다.
func (n *SamsungHvacr01StatusNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.samsungHvacr01NodeBase.initAgent(ctx); err != nil {
		if errors.Is(err, ErrNASANoResolver) || errors.Is(err, ErrNASAAgentNotNASA) {
			return err
		}
		if logger := n.Logger(); logger != nil {
			logger.Warn("samsung_hvacr01 init: agent not available, deferring connection",
				"nodeID", n.ID(),
				"agentRef", n.hvacr01Cfg.AgentRef,
				"error", err,
			)
		}
		return n.BaseNode.TransitionTo(lifecycle.StateRunning)
	}

	// v0.18.26: receiveLoop 시작 (LG inactivity 모델).
	go n.receiveLoop()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// receiveLoop 는 에이전트의 push 프레임을 수신한다 (v0.18.26, LG ICP-01 통일).
//
//   - FrameNotifyCh 신호 → drainNewFrames 로 ring buffer 의 새 snapshot 흡수.
//   - inactivityTimeout 동안 무신호 → requestStateRefresh 발송.
//     agent 가 각 디바이스 마지막 상태를 push 경로로 emit → notify → drain.
//   - 첫 진입 시 즉시 1회 drain (이전 누적 snapshot 흡수).
func (n *SamsungHvacr01StatusNode) receiveLoop() {
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
	cfg := n.hvacr01Cfg
	n.mu.RUnlock()
	n.drainNewFrames(cfg)

	for {
		select {
		case <-n.stopCh:
			return
		case <-notifyCh:
			n.mu.RLock()
			cfg := n.hvacr01Cfg
			n.mu.RUnlock()
			n.drainNewFrames(cfg)
			resetTimer()
		case <-timer.C:
			n.mu.RLock()
			cfg := n.hvacr01Cfg
			n.mu.RUnlock()
			n.requestStateRefresh(cfg)
			resetTimer()
		}
	}
}

// requestStateRefresh 는 에이전트에 "request_state" 를 보내 각 디바이스의 마지막
// 상태를 push 경로로 emit 하게 한다. agent emit → ring buffer + FrameNotifyCh →
// drain 흐름으로 자동 흡수.
func (n *SamsungHvacr01StatusNode) requestStateRefresh(cfg SamsungHvacr01NodeConfig) {
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
	_, _ = n.samsungHvacr01NodeBase.callAgentProcess(ctx, cmdBytes)
}

// drainNewFrames 는 ring buffer 에 누적된 새 snapshot (lastSeq 이후) 를 sourceCh
// 로 전달한다 (v0.18.26).
//
// 어드레싱 필터: cfg.GroupID / cfg.UnitID 가 설정되어 있으면 매칭되는 디바이스만
// emit. 빈 값이면 모든 디바이스 통과.
func (n *SamsungHvacr01StatusNode) drainNewFrames(cfg SamsungHvacr01NodeConfig) {
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
	resp, err := n.samsungHvacr01NodeBase.callAgentProcess(ctx, cmdBytes)
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

		// v0.18.26: 어드레싱 필터.
		if !samsungHvacr01MatchAddressing(dev, cfg) {
			n.lastSeq = snap.Seq // 매칭 안 되어도 watermark 진행 (skip 표시).
			continue
		}

		// 연결 정보가 device_state 단일 스트림으로 일원화되어, 링버퍼의 모든 스냅샷은
		// device_state 로 처리한다(별도 device_connection 분기 제거).
		msg := message.New()
		promotePayloadMetadata(msg, dev, cfg.EmitMetadata)
		applyDeviceStateMessageType(msg, dev, "poll")
		promoteDevIDWithUUID(msg, dev, cfg.AgentRef, cfg.EmitMetadata)
		promoteLastSeenToTimestamp(msg, dev)
		flattenStateToPayload(dev)
		applyPowerOffFilter(dev, cfg.OmitStateWhenOff)
		for k, v := range dev {
			msg.Payload().Set(k, v)
		}
		if cfg.EmitMetadata.NodeSource {
			msg.Metadata().Set("node_source", "push")
		}
		if cfg.EmitMetadata.NodeID {
			msg.Metadata().Set("node_id", n.ID())
		}
		msg.Metadata().Set("seq", fmt.Sprintf("%d", snap.Seq))
		emitAgentGroup(msg, n.agent, cfg.EmitMetadata)

		select {
		case n.sourceCh <- msg:
			n.lastSeq = snap.Seq
		case <-n.stopCh:
			return
		}
	}
}

// Process 는 입력 메시지를 받아 상태 조회를 수행하고 결과를 반환한다.
//
// v0.18.26: 단일 디바이스 조회는 unit_id 또는 device_id payload override 로 가능.
// 비어있으면 get_all 로 모든 디바이스 즉시 snapshot.
func (n *SamsungHvacr01StatusNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.hvacr01Cfg
	n.mu.RUnlock()

	cfg, deviceID := applySamsungHvacr01Overrides(msg, cfg)

	cmdBytes, err := buildStatusCommand(cfg, deviceID, n.ID())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNASAProcessFailed, err)
	}

	resp, err := n.samsungHvacr01NodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNASAProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrNASAProcessFailed, err)
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
	emitAgentGroup(out, n.agent, cfg.EmitMetadata)
	out.SetType("device_state.response")

	return []message.Message{out}, nil
}

// Shutdown 은 SamsungHvacr01StatusNode를 종료한다.
func (n *SamsungHvacr01StatusNode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})
	return n.samsungHvacr01NodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 FrameNotifier 채널 구독을
// 재구성한다 (LG ICP-01 v0.18.24 패턴).
func (n *SamsungHvacr01StatusNode) Reinit(ctx context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})

	if err := n.samsungHvacr01NodeBase.initAgent(ctx); err != nil {
		return err
	}

	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.pollOnce = sync.Once{}
	n.mu.Unlock()

	go n.receiveLoop()
	return nil
}

// SourceCh 는 receiveLoop 가 생성한 메시지를 수신하는 채널을 반환한다.
func (n *SamsungHvacr01StatusNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// SamsungHvacr01ControlNode
// ===========================================================================

// SamsungHvacr01ControlNode 는 Samsung NASA 에이전트에 제어 명령을 전송하는 노드이다.
type SamsungHvacr01ControlNode struct {
	samsungHvacr01NodeBase
}

// 인터페이스 컴파일 체크
var _ Node = (*SamsungHvacr01ControlNode)(nil)

// NewSamsungHvacr01ControlNode 는 새로운 SamsungHvacr01ControlNode를 생성하는 팩토리 함수이다.
func NewSamsungHvacr01ControlNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &SamsungHvacr01ControlNode{
		samsungHvacr01NodeBase: samsungHvacr01NodeBase{
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

// Configure 는 SamsungHvacr01ControlNode의 설정을 적용한다.
func (n *SamsungHvacr01ControlNode) Configure(config map[string]any) error {
	return n.samsungHvacr01NodeBase.configure(config)
}

// Init 은 SamsungHvacr01ControlNode를 초기화한다. 에이전트를 resolve한다.
func (n *SamsungHvacr01ControlNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.samsungHvacr01NodeBase.initAgent(ctx); err != nil {
		if errors.Is(err, ErrNASANoResolver) || errors.Is(err, ErrNASAAgentNotNASA) {
			return err
		}
		if logger := n.Logger(); logger != nil {
			logger.Warn("samsung_hvacr01 control init: agent not available, deferring connection",
				"nodeID", n.ID(),
				"agentRef", n.hvacr01Cfg.AgentRef,
				"error", err,
			)
		}
		return n.BaseNode.TransitionTo(lifecycle.StateRunning)
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 입력 메시지의 payload에서 제어 명령을 추출하여 Agent에 전달한다.
//
// v0.18.26: cfg.DeviceID 제거. 대신 payload 의 device_id / unit_id 또는 cfg.UnitID
// (어드레싱) 를 target 으로 사용. unit_id 우선.
func (n *SamsungHvacr01ControlNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.hvacr01Cfg
	n.mu.RUnlock()

	cfg, deviceID := applySamsungHvacr01Overrides(msg, cfg)
	if deviceID == "" {
		deviceID = cfg.UnitID
	}

	cmdBytes, err := buildControlCommand(msg, cfg, deviceID, n.ID())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNASAProcessFailed, err)
	}

	resp, err := n.samsungHvacr01NodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNASAProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrNASAProcessFailed, err)
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
	out.Metadata().Set("nasa_command", "control")
	if cfg.EmitMetadata.NodeID {
		out.Metadata().Set("node_id", n.ID())
	}
	emitAgentGroup(out, n.agent, cfg.EmitMetadata)
	out.SetType("device_state.response")

	return []message.Message{out}, nil
}

// Shutdown 은 SamsungHvacr01ControlNode를 종료한다.
func (n *SamsungHvacr01ControlNode) Shutdown(_ context.Context) error {
	return n.samsungHvacr01NodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조를 갱신한다.
func (n *SamsungHvacr01ControlNode) Reinit(ctx context.Context) error {
	return n.samsungHvacr01NodeBase.initAgent(ctx)
}

// ===========================================================================
// SamsungHvacr01Node — 상태 조회 + 제어 통합
// ===========================================================================

// SamsungHvacr01Node 는 Samsung NASA 에이전트의 상태 조회 (SourceNode 통한 inactivity
// 모델) + 제어 (ProcessNode) 를 통합한 노드이다.
//
// v0.18.26: LG ICP-01 모델 통일. polling 제거.
type SamsungHvacr01Node struct {
	samsungHvacr01NodeBase
	inactivityTimeout time.Duration
	sourceCh          chan message.Message
	stopCh            chan struct{}
	pollOnce          sync.Once
	lastSeq           int64

	// SPEC-HVACR-SYNC-001 M10: mirror-out 소스 포트 채널. mirror-message 모드
	// 에이전트(MirrorOutCh() != nil)일 때만 non-nil 이며, mirrorOutLoop 이 에이전트의
	// MirrorOutCh 를 드레인해 이 채널로 message.Message 를 방출한다. ExtraSourceChannels
	// 가 "mirror-out" 포트로 노출한다(SerialInNode raw_out 선례).
	mirrorOutCh chan message.Message
}

// 인터페이스 컴파일 체크
var (
	_ Node            = (*SamsungHvacr01Node)(nil)
	_ SourceNode      = (*SamsungHvacr01Node)(nil)
	_ MultiSourceNode = (*SamsungHvacr01Node)(nil)
)

// NewSamsungHvacr01Node 는 새로운 SamsungHvacr01Node를 생성하는 팩토리 함수이다.
func NewSamsungHvacr01Node(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &SamsungHvacr01Node{
		samsungHvacr01NodeBase: samsungHvacr01NodeBase{
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

// Configure 는 SamsungHvacr01Node의 설정을 적용한다.
func (n *SamsungHvacr01Node) Configure(config map[string]any) error {
	if err := n.samsungHvacr01NodeBase.configure(config); err != nil {
		return err
	}

	n.mu.RLock()
	timeoutStr := n.hvacr01Cfg.InactivityTimeout
	n.mu.RUnlock()

	inactivity, err := time.ParseDuration(timeoutStr)
	if err != nil {
		inactivity = nasaDefaultInactivityTimeout
	}
	if inactivity < nasaMinInactivityTimeout {
		inactivity = nasaMinInactivityTimeout
	}
	n.inactivityTimeout = inactivity

	return nil
}

// Init 은 SamsungHvacr01Node를 초기화한다.
func (n *SamsungHvacr01Node) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.samsungHvacr01NodeBase.initAgent(ctx); err != nil {
		if errors.Is(err, ErrNASANoResolver) || errors.Is(err, ErrNASAAgentNotNASA) {
			return err
		}
		if logger := n.Logger(); logger != nil {
			logger.Warn("samsung_hvacr01 source init: agent not available, deferring connection",
				"nodeID", n.ID(),
				"agentRef", n.hvacr01Cfg.AgentRef,
				"error", err,
			)
		}
		return n.BaseNode.TransitionTo(lifecycle.StateRunning)
	}

	go n.receiveLoop()
	// M10: mirror-message 모드면 mirror-out 드레인 고루틴 기동(비-message 모드는 no-op).
	n.startMirrorOut()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// receiveLoop 는 LG ICP-01 모델 그대로 — FrameNotifyCh + inactivity timer.
func (n *SamsungHvacr01Node) receiveLoop() {
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
	cfg := n.hvacr01Cfg
	n.mu.RUnlock()
	n.drainNewFrames(cfg)

	for {
		select {
		case <-n.stopCh:
			return
		case <-notifyCh:
			n.mu.RLock()
			cfg := n.hvacr01Cfg
			n.mu.RUnlock()
			n.drainNewFrames(cfg)
			resetTimer()
		case <-timer.C:
			n.mu.RLock()
			cfg := n.hvacr01Cfg
			n.mu.RUnlock()
			n.requestStateRefresh(cfg)
			resetTimer()
		}
	}
}

// requestStateRefresh 는 SamsungHvacr01StatusNode 와 동일.
func (n *SamsungHvacr01Node) requestStateRefresh(cfg SamsungHvacr01NodeConfig) {
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
	_, _ = n.samsungHvacr01NodeBase.callAgentProcess(ctx, cmdBytes)
}

// drainNewFrames 는 SamsungHvacr01StatusNode 와 동일 (어드레싱 필터 포함).
func (n *SamsungHvacr01Node) drainNewFrames(cfg SamsungHvacr01NodeConfig) {
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
	resp, err := n.samsungHvacr01NodeBase.callAgentProcess(ctx, cmdBytes)
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

		if !samsungHvacr01MatchAddressing(dev, cfg) {
			n.lastSeq = snap.Seq
			continue
		}

		msg := message.New()
		promotePayloadMetadata(msg, dev, cfg.EmitMetadata)
		applyDeviceStateMessageType(msg, dev, "poll")
		promoteDevIDWithUUID(msg, dev, cfg.AgentRef, cfg.EmitMetadata)
		promoteLastSeenToTimestamp(msg, dev)
		flattenStateToPayload(dev)
		applyPowerOffFilter(dev, cfg.OmitStateWhenOff)
		for k, v := range dev {
			msg.Payload().Set(k, v)
		}
		if cfg.EmitMetadata.NodeSource {
			msg.Metadata().Set("node_source", "push")
		}
		if cfg.EmitMetadata.NodeID {
			msg.Metadata().Set("node_id", n.ID())
		}
		msg.Metadata().Set("seq", fmt.Sprintf("%d", snap.Seq))
		emitAgentGroup(msg, n.agent, cfg.EmitMetadata)

		select {
		case n.sourceCh <- msg:
			n.lastSeq = snap.Seq
		case <-n.stopCh:
			return
		}
	}
}

// Process 는 입력 메시지를 받아 자동으로 상태 조회 또는 제어를 수행한다.
// payload에 제어 키(power, mode, temperature, fan_speed)가 있으면 제어 명령,
// 없으면 상태 조회 명령을 전송한다.
func (n *SamsungHvacr01Node) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	// SPEC-HVACR-SYNC-001 M10: mirror-in 판별 (엔진 fan-in 병합으로 포트 이름 구별 불가).
	// 메시지 타입 marker 가 mirror 업링크면 에이전트 ingress 로 급전하고 하류 emit 없이
	// 반환한다. 아니면 아래 기존 제어/상태 처리로 폴백한다(행위 보존).
	if msg.Type() == mirrorUplinkMsgType {
		return n.handleMirrorIn(msg)
	}

	n.mu.RLock()
	cfg := n.hvacr01Cfg
	n.mu.RUnlock()

	cfg, deviceID := applySamsungHvacr01Overrides(msg, cfg)
	if deviceID == "" {
		deviceID = cfg.UnitID
	}

	var cmdBytes []byte
	var err error
	var cmdType string

	if hasSamsungHvacr01ControlKeys(msg) {
		cmdBytes, err = buildControlCommand(msg, cfg, deviceID, n.ID())
		cmdType = "control"
	} else {
		cmdBytes, err = buildStatusCommand(cfg, deviceID, n.ID())
		cmdType = "status"
	}

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNASAProcessFailed, err)
	}

	resp, err := n.samsungHvacr01NodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNASAProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrNASAProcessFailed, err)
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
	out.Metadata().Set("nasa_command", cmdType)
	if cfg.EmitMetadata.NodeID {
		out.Metadata().Set("node_id", n.ID())
	}
	emitAgentGroup(out, n.agent, cfg.EmitMetadata)
	out.SetType("device_state.response")

	return []message.Message{out}, nil
}

// Shutdown 은 SamsungHvacr01Node를 종료한다.
func (n *SamsungHvacr01Node) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})
	return n.samsungHvacr01NodeBase.shutdown()
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조와 receiveLoop 를 재구성한다.
func (n *SamsungHvacr01Node) Reinit(ctx context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})

	if err := n.samsungHvacr01NodeBase.initAgent(ctx); err != nil {
		return err
	}

	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.pollOnce = sync.Once{}
	n.mu.Unlock()

	go n.receiveLoop()
	// M10: 에이전트 재시작 후 mirror-out 드레인 고루틴도 재기동(비-message 모드는 no-op).
	n.startMirrorOut()
	return nil
}

// SourceCh 는 receiveLoop 가 생성한 메시지를 수신하는 채널을 반환한다.
func (n *SamsungHvacr01Node) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ExtraSourceChannels 는 추가 출력 포트 채널을 반환한다 (SPEC-HVACR-SYNC-001 M10).
// mirror-message 모드 에이전트일 때만 "mirror-out" 포트 채널을 포함한다. 그 외에는
// nil 을 반환해 기존 결합 노드 동작을 보존한다(SerialInNode.ExtraSourceChannels 선례).
func (n *SamsungHvacr01Node) ExtraSourceChannels() map[string]<-chan message.Message {
	n.mu.RLock()
	ch := n.mirrorOutCh
	n.mu.RUnlock()
	if ch == nil {
		return nil
	}
	return map[string]<-chan message.Message{
		"mirror-out": ch,
	}
}

// startMirrorOut 은 에이전트가 mirror-message 모드이면 mirror-out 소스 채널을 준비하고
// 드레인 고루틴을 기동한다. Init/Reinit 에서 에이전트 resolve 이후 호출한다. 비-message
// 에이전트(MirrorOutCh()==nil)에서는 no-op 이라 기존 동작을 보존한다.
func (n *SamsungHvacr01Node) startMirrorOut() {
	hv, ok := n.agent.(*samsung.Hvacr01Agent)
	if !ok {
		return
	}
	outCh := hv.MirrorOutCh()
	if outCh == nil {
		return // mirror-message 모드가 아님
	}
	n.mu.Lock()
	if n.mirrorOutCh == nil {
		n.mirrorOutCh = make(chan message.Message, mirrorOutSourceBufferSize)
	}
	n.mu.Unlock()
	go n.mirrorOutLoop(outCh)
}

// mirrorOutLoop 은 에이전트의 MirrorOutCh(디코드 메시지 tap, 와이어 JSON)를 드레인해
// message.Message 로 감싸 mirror-out 소스 채널로 전달하는 고루틴이다. stopCh 로 종료한다
// (Shutdown 시 close). 재사용 버퍼 우려는 없으나(MirrorOutCh 는 매번 새 JSON) 방어적으로
// 문자열로 복사해 담는다.
func (n *SamsungHvacr01Node) mirrorOutLoop(outCh <-chan []byte) {
	for {
		select {
		case <-n.stopCh:
			return
		case wire, ok := <-outCh:
			if !ok {
				return
			}
			msg := message.New()
			msg.SetType(mirrorUplinkMsgType)
			msg.Payload().Set(mirrorWirePayloadKey, string(wire))
			select {
			case n.mirrorOutCh <- msg:
			case <-n.stopCh:
				return
			}
		}
	}
}

// handleMirrorIn 은 mirror-in marker 메시지를 에이전트 ingress 로 급전한다 (M10).
// 하류 emit 없이 반환한다. 에이전트가 mirror-message 모드가 아니거나 와이어를 추출할 수
// 없으면 명시적 에러를 반환한다(silent 무시 금지).
func (n *SamsungHvacr01Node) handleMirrorIn(msg message.Message) ([]message.Message, error) {
	hv, ok := n.agent.(*samsung.Hvacr01Agent)
	if !ok {
		return nil, fmt.Errorf("%w: mirror-in: 에이전트가 samsung Hvacr01Agent 가 아님", ErrNASAProcessFailed)
	}
	payload, err := mirrorWireBytes(msg)
	if err != nil {
		return nil, fmt.Errorf("%w: mirror-in: %v", ErrNASAProcessFailed, err)
	}
	if err := hv.FeedMirrorWire(payload); err != nil {
		return nil, fmt.Errorf("%w: mirror-in feed: %v", ErrNASAProcessFailed, err)
	}
	return nil, nil // mirror-in 은 하류로 emit 하지 않음
}

// mirrorWireBytes 는 mirror-in 메시지 payload 에서 와이어 JSON 바이트를 추출한다.
// mirror-out 이 string 으로 저장하므로 string 을 우선 처리하되, []byte/json.RawMessage 도
// 방어적으로 수용한다(노드 간 직렬화 변형 대비).
func mirrorWireBytes(msg message.Message) ([]byte, error) {
	v, ok := msg.Payload().Get(mirrorWirePayloadKey)
	if !ok {
		return nil, fmt.Errorf("payload 키 %q 누락", mirrorWirePayloadKey)
	}
	switch b := v.(type) {
	case string:
		return []byte(b), nil
	case []byte:
		return b, nil
	case json.RawMessage:
		return []byte(b), nil
	default:
		return nil, fmt.Errorf("payload 키 %q 타입 미지원: %T", mirrorWirePayloadKey, v)
	}
}

// ===========================================================================
// 헬퍼 함수
// ===========================================================================

// samsungHvacr01ControlKeys 는 NASA 제어 명령으로 인식되는 payload 키 목록이다.
var samsungHvacr01ControlKeys = []string{"power", "mode", "temperature", "fan_speed"}

// hasSamsungHvacr01ControlKeys 는 메시지 payload에 제어 키가 하나라도 있는지 확인한다.
func hasSamsungHvacr01ControlKeys(msg message.Message) bool {
	for _, key := range samsungHvacr01ControlKeys {
		if _, ok := msg.Payload().Get(key); ok {
			return true
		}
	}
	return false
}

// buildStatusCommand 는 상태 조회용 JSON 커맨드를 생성한다 (v0.18.26).
//
// deviceID 가 비어있지 않으면 get_state 로 단일 디바이스 조회,
// 비어있으면 get_all 로 전체 즉시 snapshot.
func buildStatusCommand(cfg SamsungHvacr01NodeConfig, deviceID, nodeID string) ([]byte, error) {
	_ = cfg
	cmd := map[string]any{}

	if deviceID != "" {
		cmd["command"] = nasaCmdGetState
		cmd["device_id"] = deviceID
	} else {
		cmd["command"] = nasaCmdGetAllState
	}
	if nodeID != "" {
		cmd["node_id"] = nodeID
	}

	return json.Marshal(cmd)
}

// buildControlCommand 는 제어용 JSON 커맨드를 생성한다 (v0.18.26).
//
// payload에 "command" 키가 있으면 직접 커맨드로 전달하고,
// 없으면 제어 키(power, mode, temperature, fan_speed)를 수집하여 set_multiple을 구성한다.
//
// deviceID 는 target 식별자 (unit_id 또는 device_id payload override 우선).
func buildControlCommand(msg message.Message, cfg SamsungHvacr01NodeConfig, deviceID, nodeID string) ([]byte, error) {
	_ = cfg
	// payload에 "command" 키가 있으면 직접 전달
	if v, ok := msg.Payload().Get("command"); ok {
		if cmdStr, ok := v.(string); ok && cmdStr != "" {
			cmd := map[string]any{
				"command": cmdStr,
			}
			if deviceID != "" {
				cmd["device_id"] = deviceID
			}
			if nodeID != "" {
				cmd["node_id"] = nodeID
			}
			if params, ok := msg.Payload().Get("params"); ok {
				cmd["params"] = params
			}
			return json.Marshal(cmd)
		}
	}

	// 제어 키에서 set_multiple 구성
	settings := map[string]any{}
	for _, key := range samsungHvacr01ControlKeys {
		if v, ok := msg.Payload().Get(key); ok {
			settings[key] = v
		}
	}

	if len(settings) == 0 {
		// 제어 키가 없으면 상태 조회로 폴백
		return buildStatusCommand(cfg, deviceID, nodeID)
	}

	cmd := map[string]any{
		"command":  nasaCmdSetMultiple,
		"settings": settings,
	}
	if deviceID != "" {
		cmd["device_id"] = deviceID
	}
	if nodeID != "" {
		cmd["node_id"] = nodeID
	}

	return json.Marshal(cmd)
}
