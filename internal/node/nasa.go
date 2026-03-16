package node

import (
	"context"
	"encoding/json"
	"fmt"
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
	nasaCmdGetState    = "get_state"
	nasaCmdGetAllState = "get_all_states"
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
	pollOnce     sync.Once // stopCh close 보호
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

	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			n.mu.RLock()
			cfg := n.nasaCfg
			n.mu.RUnlock()

			cmdBytes, err := buildStatusCommand(cfg)
			if err != nil {
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
			resp, err := n.nasaNodeBase.callAgentProcess(ctx, cmdBytes)
			cancel()

			if err != nil {
				continue
			}

			var result map[string]any
			if err := json.Unmarshal(resp, &result); err != nil {
				continue
			}

			msg := message.New()
			for k, v := range result {
				msg.Payload().Set(k, v)
			}
			msg.Metadata().Set("nasa_source", "poll")
			msg.Metadata().Set("nasa_node_id", n.ID())

			select {
			case n.sourceCh <- msg:
			default:
				// 채널이 가득 차면 드롭
			}
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

	cmdBytes, err := buildStatusCommand(cfg)
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

	cmdBytes, err := buildControlCommand(msg, cfg)
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

	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			n.mu.RLock()
			cfg := n.nasaCfg
			n.mu.RUnlock()

			cmdBytes, err := buildStatusCommand(cfg)
			if err != nil {
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
			resp, err := n.nasaNodeBase.callAgentProcess(ctx, cmdBytes)
			cancel()

			if err != nil {
				continue
			}

			var result map[string]any
			if err := json.Unmarshal(resp, &result); err != nil {
				continue
			}

			msg := message.New()
			for k, v := range result {
				msg.Payload().Set(k, v)
			}
			msg.Metadata().Set("nasa_source", "poll")
			msg.Metadata().Set("nasa_node_id", n.ID())

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
func (n *NASANode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.nasaCfg
	n.mu.RUnlock()

	cfg = applyNASAOverrides(msg, cfg)

	var cmdBytes []byte
	var err error
	var cmdType string

	if hasNASAControlKeys(msg) {
		cmdBytes, err = buildControlCommand(msg, cfg)
		cmdType = "control"
	} else {
		cmdBytes, err = buildStatusCommand(cfg)
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
func buildStatusCommand(cfg NASANodeConfig) ([]byte, error) {
	cmd := map[string]any{}

	if cfg.DeviceID != "" {
		cmd["command"] = nasaCmdGetState
		cmd["device_id"] = cfg.DeviceID
	} else {
		cmd["command"] = nasaCmdGetAllState
	}

	return json.Marshal(cmd)
}

// buildControlCommand 는 제어용 JSON 커맨드를 생성한다.
// payload에 "command" 키가 있으면 직접 커맨드로 전달하고,
// 없으면 제어 키(power, mode, temperature, fan_speed)를 수집하여 set_multiple을 구성한다.
func buildControlCommand(msg message.Message, cfg NASANodeConfig) ([]byte, error) {
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
	for _, key := range nasaControlKeys {
		if v, ok := msg.Payload().Get(key); ok {
			settings[key] = v
		}
	}

	if len(settings) == 0 {
		// 제어 키가 없으면 상태 조회로 폴백
		return buildStatusCommand(cfg)
	}

	cmd := map[string]any{
		"command":  nasaCmdSetMultiple,
		"settings": settings,
	}
	if cfg.DeviceID != "" {
		cmd["device_id"] = cfg.DeviceID
	}

	return json.Marshal(cmd)
}
