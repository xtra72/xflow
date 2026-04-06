package node

import (
	"context"
	"encoding/json"
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
	lgcpDefaultTimeout = 5 * time.Second

	// 기본 폴링 간격
	lgcpDefaultPollInterval = 30 * time.Second

	// 기본 LGCP 커맨드
	lgcpCmdGetStats   = "get_stats"
	lgcpCmdGetRecent  = "get_recent"
	lgcpCmdSetMultiple = "set_multiple"
)

// ---------------------------------------------------------------------------
// LGCPNodeConfig
// ---------------------------------------------------------------------------

// LGCPNodeConfig 는 LGCP 노드 공용 설정 구조체이다.
type LGCPNodeConfig struct {
	AgentRef       string `json:"agent_ref"`       // 필수: LGCP 에이전트 이름/ID
	DefaultAddress string `json:"default_address"` // 선택: 기본 실내기 주소 (hex)
	PollInterval   string `json:"poll_interval"`   // 선택: 폴링 간격 (기본 "30s")
	Timeout        string `json:"timeout"`         // 선택: Process 타임아웃 (기본 "5s")
	PollCommand    string `json:"poll_command"`    // 선택: 폴링 커맨드 (기본 "get_stats", "get_recent" 가능)
	RecentCount    int    `json:"recent_count"`    // 선택: get_recent 시 프레임 수 (기본 10)
}

// ---------------------------------------------------------------------------
// lgcpNodeBase
// ---------------------------------------------------------------------------

// lgcpNodeBase 는 LGCP 노드 공통 기반 구조체이다.
// LGCPStatusNode, LGCPControlNode, LGCPNode가 이를 임베딩한다.
type lgcpNodeBase struct {
	*BaseNode
	lgcpCfg   LGCPNodeConfig
	resolver  AgentResolver
	transport AgentTransport
	agent     agent.Agent   // 원본 LG LGCP Agent 객체
	timeout   time.Duration // Process 호출 타임아웃
	mu        sync.RWMutex  // 설정 보호 뮤텍스
}

// configure 는 공통 설정 파싱을 수행한다. agent_ref(필수)를 검증한다.
func (nb *lgcpNodeBase) configure(config map[string]any) error {
	if err := nb.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg LGCPNodeConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrLGCPMissingAgentRef
	}

	// default_address (선택)
	if v, ok := config["default_address"]; ok {
		if s, ok := v.(string); ok {
			cfg.DefaultAddress = s
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

	// poll_command (선택, 기본값 "get_stats")
	cfg.PollCommand = lgcpCmdGetStats
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

	// 타임아웃 파싱
	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		timeout = lgcpDefaultTimeout
	}

	nb.mu.Lock()
	nb.lgcpCfg = cfg
	nb.timeout = timeout
	nb.mu.Unlock()

	return nil
}

// initAgent 는 AgentResolver를 통해 에이전트를 resolve하고 LG LGCP 타입을 확인한다.
func (nb *lgcpNodeBase) initAgent(ctx context.Context) error {
	if nb.resolver == nil {
		return ErrLGCPNoResolver
	}

	nb.mu.RLock()
	agentRef := nb.lgcpCfg.AgentRef
	nb.mu.RUnlock()

	ref := flow.AgentRef{
		AgentID:   agentRef,
		AgentName: agentRef,
	}
	transport, err := nb.resolver.ResolveAgent(ctx, ref)
	if err != nil {
		return fmt.Errorf("lgcp init: agent resolve failed: %w", err)
	}
	nb.transport = transport

	// AgentAccessor를 통해 원본 Agent 객체 획득 및 타입 확인
	accessor, ok := transport.(AgentAccessor)
	if !ok {
		return ErrLGCPAgentNotLGCP
	}

	underlyingAgent := accessor.UnderlyingAgent()
	switch underlyingAgent.(type) {
	case *lg.LGCPAgent:
		nb.agent = underlyingAgent
	default:
		return ErrLGCPAgentNotLGCP
	}

	return nil
}

// callAgentProcess 는 Agent.Process()를 context timeout과 함께 호출한다.
func (nb *lgcpNodeBase) callAgentProcess(ctx context.Context, cmdBytes []byte) ([]byte, error) {
	if nb.agent == nil {
		return nil, ErrLGCPNoResolver
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
		return nil, fmt.Errorf("lgcp: %w", timeoutCtx.Err())
	case result := <-ch:
		return result.data, result.err
	}
}

// shutdown 은 공통 종료 로직을 수행한다.
func (nb *lgcpNodeBase) shutdown() error {
	return nb.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// ===========================================================================
// LGCPStatusNode
// ===========================================================================

// LGCPStatusNode 는 LG LGCP 에이전트의 상태를 조회하는 노드이다.
// SourceNode 인터페이스를 구현하여 폴링 기반 자체 메시지 생성을 지원한다.
type LGCPStatusNode struct {
	lgcpNodeBase
	pollInterval time.Duration
	sourceCh     chan message.Message
	stopCh       chan struct{}
	pollOnce     sync.Once // stopCh close 보호
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*LGCPStatusNode)(nil)
	_ SourceNode = (*LGCPStatusNode)(nil)
)

// NewLGCPStatusNode 는 새로운 LGCPStatusNode를 생성하는 팩토리 함수이다.
func NewLGCPStatusNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &LGCPStatusNode{
		lgcpNodeBase: lgcpNodeBase{
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

// Configure 는 LGCPStatusNode의 설정을 적용한다.
func (n *LGCPStatusNode) Configure(config map[string]any) error {
	if err := n.lgcpNodeBase.configure(config); err != nil {
		return err
	}

	// poll_interval 파싱
	n.mu.RLock()
	pollStr := n.lgcpCfg.PollInterval
	n.mu.RUnlock()

	pollInterval, err := time.ParseDuration(pollStr)
	if err != nil {
		pollInterval = lgcpDefaultPollInterval
	}
	n.pollInterval = pollInterval

	return nil
}

// Init 은 LGCPStatusNode를 초기화한다.
// 에이전트를 resolve하고, 폴링 고루틴을 시작한다.
func (n *LGCPStatusNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.lgcpNodeBase.initAgent(ctx); err != nil {
		return err
	}

	// 폴링 고루틴 시작 (SourceNode 지원)
	go n.pollLoop()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// pollLoop 는 설정된 간격으로 상태를 조회하여 sourceCh에 메시지를 전달한다.
func (n *LGCPStatusNode) pollLoop() {
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			n.mu.RLock()
			cfg := n.lgcpCfg
			n.mu.RUnlock()

			cmdBytes, err := buildLGCPStatusCommand(cfg)
			if err != nil {
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
			resp, err := n.lgcpNodeBase.callAgentProcess(ctx, cmdBytes)
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
			msg.Metadata().Set("lgcp_source", "poll")
			msg.Metadata().Set("lgcp_node_id", n.ID())

			select {
			case n.sourceCh <- msg:
			default:
				// 채널이 가득 차면 드롭
			}
		}
	}
}

// Process 는 입력 메시지를 받아 상태 조회를 수행하고 결과를 반환한다.
func (n *LGCPStatusNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.lgcpCfg
	n.mu.RUnlock()

	// 메시지 payload에서 오버라이드 적용
	cfg = applyLGCPOverrides(msg, cfg)

	cmdBytes, err := buildLGCPStatusCommand(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGCPProcessFailed, err)
	}

	resp, err := n.lgcpNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGCPProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrLGCPProcessFailed, err)
	}

	out := msg.Clone()
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	out.Metadata().Set("lgcp_source", "request")
	out.Metadata().Set("lgcp_node_id", n.ID())

	return []message.Message{out}, nil
}

// Shutdown 은 LGCPStatusNode를 종료한다. 폴링 고루틴을 정지한다.
func (n *LGCPStatusNode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})
	return n.lgcpNodeBase.shutdown()
}

// SourceCh 는 폴링으로 생성된 메시지를 수신하는 채널을 반환한다.
func (n *LGCPStatusNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// LGCPControlNode
// ===========================================================================

// LGCPControlNode 는 LG LGCP 에이전트에 제어 명령을 전송하는 노드이다.
type LGCPControlNode struct {
	lgcpNodeBase
}

// 인터페이스 컴파일 체크
var _ Node = (*LGCPControlNode)(nil)

// NewLGCPControlNode 는 새로운 LGCPControlNode를 생성하는 팩토리 함수이다.
func NewLGCPControlNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &LGCPControlNode{
		lgcpNodeBase: lgcpNodeBase{
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

// Configure 는 LGCPControlNode의 설정을 적용한다.
func (n *LGCPControlNode) Configure(config map[string]any) error {
	return n.lgcpNodeBase.configure(config)
}

// Init 은 LGCPControlNode를 초기화한다. 에이전트를 resolve한다.
func (n *LGCPControlNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.lgcpNodeBase.initAgent(ctx); err != nil {
		return err
	}

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 입력 메시지의 payload에서 제어 명령을 추출하여 Agent에 전달한다.
// payload에 "command" 키가 있으면 직접 명령으로 처리하고,
// 없으면 제어 키(power, mode, temperature, fan_speed)에서 set_multiple을 구성한다.
// 제어 명령에는 address가 필수이다 (payload "address" > cfg.DefaultAddress > 에러).
func (n *LGCPControlNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.lgcpCfg
	n.mu.RUnlock()

	cfg = applyLGCPOverrides(msg, cfg)

	cmdBytes, err := buildLGCPControlCommand(msg, cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGCPProcessFailed, err)
	}

	resp, err := n.lgcpNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGCPProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrLGCPProcessFailed, err)
	}

	out := msg.Clone()
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	out.Metadata().Set("lgcp_command", "control")
	out.Metadata().Set("lgcp_node_id", n.ID())

	return []message.Message{out}, nil
}

// Shutdown 은 LGCPControlNode를 종료한다.
func (n *LGCPControlNode) Shutdown(_ context.Context) error {
	return n.lgcpNodeBase.shutdown()
}

// ===========================================================================
// LGCPNode
// ===========================================================================

// LGCPNode 는 LG LGCP 에이전트의 상태 조회와 제어를 모두 수행하는 통합 노드이다.
// SourceNode 인터페이스를 구현하여 폴링 기반 자체 메시지 생성도 지원한다.
// payload에 제어 키(power, mode, temperature, fan_speed)가 있으면 제어,
// 없으면 상태 조회로 자동 감지한다.
type LGCPNode struct {
	lgcpNodeBase
	pollInterval time.Duration
	sourceCh     chan message.Message
	stopCh       chan struct{}
	pollOnce     sync.Once
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*LGCPNode)(nil)
	_ SourceNode = (*LGCPNode)(nil)
)

// NewLGCPNode 는 새로운 LGCPNode를 생성하는 팩토리 함수이다.
func NewLGCPNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &LGCPNode{
		lgcpNodeBase: lgcpNodeBase{
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

// Configure 는 LGCPNode의 설정을 적용한다.
func (n *LGCPNode) Configure(config map[string]any) error {
	if err := n.lgcpNodeBase.configure(config); err != nil {
		return err
	}

	// poll_interval 파싱
	n.mu.RLock()
	pollStr := n.lgcpCfg.PollInterval
	n.mu.RUnlock()

	pollInterval, err := time.ParseDuration(pollStr)
	if err != nil {
		pollInterval = lgcpDefaultPollInterval
	}
	n.pollInterval = pollInterval

	return nil
}

// Init 은 LGCPNode를 초기화한다.
// 에이전트를 resolve하고, 폴링 고루틴을 시작한다.
func (n *LGCPNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if err := n.lgcpNodeBase.initAgent(ctx); err != nil {
		return err
	}

	// 폴링 고루틴 시작 (SourceNode 지원)
	go n.pollLoop()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// pollLoop 는 설정된 간격으로 상태를 조회하여 sourceCh에 메시지를 전달한다.
func (n *LGCPNode) pollLoop() {
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			n.mu.RLock()
			cfg := n.lgcpCfg
			n.mu.RUnlock()

			cmdBytes, err := buildLGCPStatusCommand(cfg)
			if err != nil {
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
			resp, err := n.lgcpNodeBase.callAgentProcess(ctx, cmdBytes)
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
			msg.Metadata().Set("lgcp_source", "poll")
			msg.Metadata().Set("lgcp_node_id", n.ID())

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
func (n *LGCPNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cfg := n.lgcpCfg
	n.mu.RUnlock()

	cfg = applyLGCPOverrides(msg, cfg)

	var cmdBytes []byte
	var err error
	var cmdType string

	if hasLGCPControlKeys(msg) {
		cmdBytes, err = buildLGCPControlCommand(msg, cfg)
		cmdType = "control"
	} else {
		cmdBytes, err = buildLGCPStatusCommand(cfg)
		cmdType = "status"
	}

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGCPProcessFailed, err)
	}

	resp, err := n.lgcpNodeBase.callAgentProcess(ctx, cmdBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLGCPProcessFailed, err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("%w: invalid response JSON: %v", ErrLGCPProcessFailed, err)
	}

	out := msg.Clone()
	for k, v := range result {
		out.Payload().Set(k, v)
	}
	out.Metadata().Set("lgcp_command", cmdType)
	out.Metadata().Set("lgcp_node_id", n.ID())

	return []message.Message{out}, nil
}

// Shutdown 은 LGCPNode를 종료한다. 폴링 고루틴을 정지한다.
func (n *LGCPNode) Shutdown(_ context.Context) error {
	n.pollOnce.Do(func() {
		close(n.stopCh)
	})
	return n.lgcpNodeBase.shutdown()
}

// SourceCh 는 폴링으로 생성된 메시지를 수신하는 채널을 반환한다.
func (n *LGCPNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ===========================================================================
// 헬퍼 함수
// ===========================================================================

// lgcpControlKeys 는 LGCP 제어 명령으로 인식되는 payload 키 목록이다.
var lgcpControlKeys = []string{"power", "mode", "temperature", "fan_speed"}

// hasLGCPControlKeys 는 메시지 payload에 제어 키가 하나라도 있는지 확인한다.
func hasLGCPControlKeys(msg message.Message) bool {
	for _, key := range lgcpControlKeys {
		if _, ok := msg.Payload().Get(key); ok {
			return true
		}
	}
	return false
}

// buildLGCPStatusCommand 는 상태 조회용 JSON 커맨드를 생성한다.
// poll_command가 "get_recent"이면 count를 포함하고, 그 외에는 "get_stats"를 사용한다.
func buildLGCPStatusCommand(cfg LGCPNodeConfig) ([]byte, error) {
	cmd := map[string]any{}

	if cfg.PollCommand == lgcpCmdGetRecent {
		cmd["command"] = lgcpCmdGetRecent
		cmd["count"] = cfg.RecentCount
	} else {
		cmd["command"] = lgcpCmdGetStats
	}

	if cfg.DefaultAddress != "" {
		cmd["address"] = cfg.DefaultAddress
	}

	return json.Marshal(cmd)
}

// buildLGCPControlCommand 는 제어용 JSON 커맨드를 생성한다.
// payload에 "command" 키가 있으면 직접 커맨드로 전달하고,
// 없으면 제어 키(power, mode, temperature, fan_speed)를 수집하여 set_multiple을 구성한다.
// 제어 명령에는 address가 필수이다 (payload "address" > cfg.DefaultAddress > ErrLGCPMissingAddress).
func buildLGCPControlCommand(msg message.Message, cfg LGCPNodeConfig) ([]byte, error) {
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
				return nil, ErrLGCPMissingAddress
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
	for _, key := range lgcpControlKeys {
		if v, ok := msg.Payload().Get(key); ok {
			params[key] = v
		}
	}

	if len(params) == 0 {
		// 제어 키가 없으면 상태 조회로 폴백
		return buildLGCPStatusCommand(cfg)
	}

	if address == "" {
		return nil, ErrLGCPMissingAddress
	}

	cmd := map[string]any{
		"command": lgcpCmdSetMultiple,
		"address": address,
		"params":  params,
	}

	return json.Marshal(cmd)
}

// applyLGCPOverrides 는 입력 메시지 payload에서 address, timeout, poll_command, count를 오버라이드한다.
func applyLGCPOverrides(msg message.Message, cfg LGCPNodeConfig) LGCPNodeConfig {
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
