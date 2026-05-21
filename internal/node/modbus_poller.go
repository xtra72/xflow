package node

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 센티널 에러 정의
// ---------------------------------------------------------------------------

var (
	ErrModbusPollerMissingAgentRef     = fmt.Errorf("modbus-poller: %w: agent_ref is required", ErrInvalidConfig)
	ErrModbusPollerInvalidPollInterval = fmt.Errorf("modbus-poller: %w: poll_interval must be >= 100ms", ErrInvalidConfig)
	ErrModbusPollerMissingRegisterMap  = fmt.Errorf("modbus-poller: %w: register_map is required", ErrInvalidConfig)
)

// ---------------------------------------------------------------------------
// 상수
// ---------------------------------------------------------------------------

const (
	pollerDefaultPollInterval = 5 * time.Second
	pollerMinPollInterval     = 100 * time.Millisecond
	pollerDefaultBufferSize   = 64
)

// ---------------------------------------------------------------------------
// RegisterMapEntry
// ---------------------------------------------------------------------------

// RegisterMapEntry 는 register_map의 개별 레지스터 정의이다.
type RegisterMapEntry struct {
	Name         string `json:"name"`
	RegisterArea string `json:"register_area"`
	Address      uint16 `json:"address"`
	Count        uint16 `json:"count"`
	DataType     string `json:"data_type"`
	ByteOrder    string `json:"byte_order"`
	DeviceID     uint8  `json:"device_id"`
}

// ---------------------------------------------------------------------------
// ModbusPollerConfig
// ---------------------------------------------------------------------------

// ModbusPollerConfig 는 ModbusPollerNode의 설정 구조체이다.
// 레지스터 영역, 주소, 데이터 타입 등은 register_map에서 정의한다.
type ModbusPollerConfig struct {
	AgentRef     string             `json:"agent_ref"`
	DeviceID     uint8              `json:"device_id"`
	PollInterval string             `json:"poll_interval"`
	Timeout      string             `json:"timeout"`
	RegisterMap  []RegisterMapEntry `json:"register_map"`
}

// ---------------------------------------------------------------------------
// ModbusPollerNode
// ---------------------------------------------------------------------------

// ModbusPollerNode 는 MODBUS 레지스터를 주기적으로 폴링하는 SourceNode이다.
// poll_interval 간격으로 register_map에 정의된 레지스터를 읽어 sourceCh에 메시지를 전달한다.
// in 포트로 메시지를 수신하면 설정을 동적으로 변경할 수 있다.
type ModbusPollerNode struct {
	modbusNodeBase
	pollerConfig ModbusPollerConfig
	pollInterval time.Duration
	sourceCh     chan message.Message
	stopCh       chan struct{}
	stopOnce     sync.Once
	reconfigCh   chan struct{} // poll_interval 변경 시 ticker 리셋 신호
}

// 인터페이스 컴파일 체크
var (
	_ Node       = (*ModbusPollerNode)(nil)
	_ SourceNode = (*ModbusPollerNode)(nil)
)

// NewModbusPollerNode 는 새로운 ModbusPollerNode를 생성하는 팩토리 함수이다.
func NewModbusPollerNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &ModbusPollerNode{
		modbusNodeBase: modbusNodeBase{
			BaseNode: base,
		},
		sourceCh:   make(chan message.Message, pollerDefaultBufferSize),
		stopCh:     make(chan struct{}),
		reconfigCh: make(chan struct{}, 1),
	}
	n.initModbusResolver()
	return n, nil
}

// ---------------------------------------------------------------------------
// Configure
// ---------------------------------------------------------------------------

// Configure 는 ModbusPollerNode의 설정을 적용한다.
// register_map은 필수이며, 레지스터 영역/주소/데이터타입은 각 항목에서 정의한다.
func (n *ModbusPollerNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	var cfg ModbusPollerConfig

	// agent_ref (필수)
	if v, ok := config["agent_ref"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.AgentRef = s
		}
	}
	if cfg.AgentRef == "" {
		return ErrModbusPollerMissingAgentRef
	}

	// device_id (선택, 기본값 1 — register_map 항목에서 개별 지정 가능)
	cfg.DeviceID = 1
	if v, ok := config["device_id"]; ok {
		if d := toByte(v); d > 0 {
			cfg.DeviceID = d
		}
	}

	// poll_interval (선택, 기본값 5s, 최소 100ms)
	n.pollInterval = pollerDefaultPollInterval
	if v, ok := config["poll_interval"]; ok {
		if s, ok := v.(string); ok && s != "" {
			if d, err := time.ParseDuration(s); err == nil {
				if d >= pollerMinPollInterval {
					n.pollInterval = d
				} else {
					return ErrModbusPollerInvalidPollInterval
				}
			}
		}
	}
	cfg.PollInterval = n.pollInterval.String()

	// timeout (선택, 기본값 5s)
	n.timeout = defaultTimeout
	if v, ok := config["timeout"]; ok {
		if s, ok := v.(string); ok && s != "" {
			if d, err := time.ParseDuration(s); err == nil {
				n.timeout = d
			}
		}
	}
	cfg.Timeout = n.timeout.String()

	// register_map (필수)
	if v, ok := config["register_map"]; ok {
		cfg.RegisterMap = parseRegisterMap(v, cfg.DeviceID)
	}
	if len(cfg.RegisterMap) == 0 {
		return ErrModbusPollerMissingRegisterMap
	}

	n.mu.Lock()
	n.pollerConfig = cfg
	n.mu.Unlock()

	return nil
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

// Init 은 ModbusPollerNode를 초기화한다.
func (n *ModbusPollerNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	n.mu.RLock()
	agentRef := n.pollerConfig.AgentRef
	n.mu.RUnlock()

	if err := n.resolveModbusAgent(ctx, agentRef); err != nil {
		return err
	}

	go n.pollLoop()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// ---------------------------------------------------------------------------
// AgentRef / Reinit (AgentReinitializer 인터페이스 구현)
// ---------------------------------------------------------------------------

// AgentRef 는 이 노드가 의존하는 에이전트 식별자를 반환한다 (AgentReinitializer).
func (n *ModbusPollerNode) AgentRef() flow.AgentRef {
	n.mu.RLock()
	ref := n.pollerConfig.AgentRef
	n.mu.RUnlock()
	return flow.AgentRef{AgentID: ref, AgentName: ref}
}

// Reinit 은 에이전트 재시작 후 agent / transport 참조를 재해석하고 폴링 루프를
// 재시작한다.
func (n *ModbusPollerNode) Reinit(ctx context.Context) error {
	n.stopOnce.Do(func() {
		close(n.stopCh)
	})

	n.mu.RLock()
	agentRef := n.pollerConfig.AgentRef
	n.mu.RUnlock()

	if err := n.resolveModbusAgent(ctx, agentRef); err != nil {
		return err
	}

	n.mu.Lock()
	n.stopCh = make(chan struct{})
	n.stopOnce = sync.Once{}
	n.mu.Unlock()

	go n.pollLoop()
	return nil
}

// ---------------------------------------------------------------------------
// pollLoop
// ---------------------------------------------------------------------------

// pollLoop 는 설정된 간격으로 레지스터를 읽어 sourceCh에 메시지를 전달한다.
// reconfigCh 신호를 수신하면 현재 poll_interval로 ticker를 리셋한다.
func (n *ModbusPollerNode) pollLoop() {
	n.mu.RLock()
	interval := n.pollInterval
	n.mu.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-n.stopCh:
			return
		case <-n.reconfigCh:
			n.mu.RLock()
			newInterval := n.pollInterval
			n.mu.RUnlock()
			ticker.Reset(newInterval)
		case <-ticker.C:
			n.mu.RLock()
			cfg := n.pollerConfig
			n.mu.RUnlock()

			msg, err := n.pollRegisterMap(cfg)
			if err != nil || msg == nil {
				continue
			}

			msg.Metadata().Set("node_source", "poll")
			msg.Metadata().Set("node_id", n.ID())
			msg.Metadata().Set("message_type", "event")

			select {
			case n.sourceCh <- msg:
			default:
				// 채널이 가득 차면 드롭
			}
		}
	}
}

// ---------------------------------------------------------------------------
// pollRegisterMap
// ---------------------------------------------------------------------------

// pollRegisterMap 은 register_map의 각 항목을 순차적으로 읽어 이름 기반 payload를 구성한다.
func (n *ModbusPollerNode) pollRegisterMap(cfg ModbusPollerConfig) (message.Message, error) {
	msg := message.New()

	for _, entry := range cfg.RegisterMap {
		mcfg := ModbusConfig{
			AgentRef:     cfg.AgentRef,
			RegisterArea: entry.RegisterArea,
			Address:      entry.Address,
			Count:        entry.Count,
			DataType:     entry.DataType,
			ByteOrder:    entry.ByteOrder,
			DeviceID:     entry.DeviceID,
		}

		cmdBytes, err := n.buildPollerReadCommand(mcfg)
		if err != nil {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
		resp, err := n.callModbusAgentProcess(ctx, cmdBytes)
		cancel()

		if err != nil {
			continue
		}

		var respMap map[string]any
		if err := json.Unmarshal(resp, &respMap); err != nil {
			continue
		}

		result := extractReadResult(respMap, n.agentType, mcfg)
		msg.Payload().Set(entry.Name, result)
	}

	return msg, nil
}

// ---------------------------------------------------------------------------
// 읽기 명령 빌더
// ---------------------------------------------------------------------------

// buildPollerReadCommand 는 Server/Client Agent별 읽기 명령을 생성한다.
func (n *ModbusPollerNode) buildPollerReadCommand(cfg ModbusConfig) ([]byte, error) {
	switch n.agentType {
	case agentTypeServer:
		return buildModbusServerReadCommand(cfg)
	case agentTypeClient:
		return buildModbusClientReadCommand(cfg)
	default:
		return nil, ErrModbusAgentNotMODBUS
	}
}

// buildModbusServerReadCommand 는 Server Agent 읽기 명령 JSON을 생성한다.
func buildModbusServerReadCommand(cfg ModbusConfig) ([]byte, error) {
	params := map[string]any{
		"address": cfg.Address,
	}

	var command string

	switch cfg.RegisterArea {
	case areaCoils:
		command = "get_coils"
		params["quantity"] = cfg.Count
	case areaDiscreteInputs:
		command = "get_discrete_inputs"
		params["quantity"] = cfg.Count
	case areaHoldingRegisters:
		if cfg.DataType != defaultDataType && !booleanAreas[cfg.RegisterArea] {
			command = "get_register_typed"
			params["area"] = cfg.RegisterArea
			params["data_type"] = cfg.DataType
			params["byte_order"] = cfg.ByteOrder
		} else {
			command = "get_holding_registers"
			params["quantity"] = cfg.Count
		}
	case areaInputRegisters:
		if cfg.DataType != defaultDataType && !booleanAreas[cfg.RegisterArea] {
			command = "get_register_typed"
			params["area"] = cfg.RegisterArea
			params["data_type"] = cfg.DataType
			params["byte_order"] = cfg.ByteOrder
		} else {
			command = "get_input_registers"
			params["quantity"] = cfg.Count
		}
	}

	req := map[string]any{
		"command": command,
		"params":  params,
	}
	return json.Marshal(req)
}

// buildModbusClientReadCommand 는 Client Agent 읽기 명령 JSON을 생성한다.
func buildModbusClientReadCommand(cfg ModbusConfig) ([]byte, error) {
	deviceID := fmt.Sprintf("%d", cfg.DeviceID)
	fc := areaToFunctionCode[cfg.RegisterArea]

	params := map[string]any{
		"function_code": fc,
		"address":       cfg.Address,
		"quantity":      cfg.Count,
	}

	req := map[string]any{
		"command":   "read_raw",
		"device_id": deviceID,
		"params":    params,
	}
	return json.Marshal(req)
}

// ---------------------------------------------------------------------------
// Process / Shutdown / SourceCh
// ---------------------------------------------------------------------------

// Process 는 입력 메시지로 폴링 설정을 동적으로 변경한다.
// device_id, poll_interval, register_map 필드를 오버라이드할 수 있다.
// 변경된 설정은 다음 폴링 주기부터 적용된다.
func (n *ModbusPollerNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	if msg.Payload() == nil {
		return []message.Message{msg}, nil
	}

	n.mu.Lock()
	cfg := n.pollerConfig
	changed := false
	intervalChanged := false

	// device_id 오버라이드 (전체 기본값)
	if v, ok := msg.Payload().Get("device_id"); ok {
		if d := toByte(v); d > 0 {
			cfg.DeviceID = d
			changed = true
		}
	}

	// poll_interval 오버라이드
	if v, ok := msg.Payload().Get("poll_interval"); ok {
		if s, ok := v.(string); ok && s != "" {
			if d, err := time.ParseDuration(s); err == nil && d >= pollerMinPollInterval {
				n.pollInterval = d
				cfg.PollInterval = d.String()
				intervalChanged = true
				changed = true
			}
		}
	}

	// register_map 오버라이드
	if v, ok := msg.Payload().Get("register_map"); ok {
		if entries := parseRegisterMap(v, cfg.DeviceID); len(entries) > 0 {
			cfg.RegisterMap = entries
			changed = true
		}
	}

	if changed {
		n.pollerConfig = cfg
	}
	n.mu.Unlock()

	// poll_interval 변경 시 ticker 리셋 신호
	if intervalChanged {
		select {
		case n.reconfigCh <- struct{}{}:
		default:
		}
	}

	// 설정 변경 확인 응답
	outMsg := msg.Clone()
	outMsg.Payload().Set("config_updated", changed)
	outMsg.Metadata().Set("message_type", "response")

	return []message.Message{outMsg}, nil
}

// Shutdown 은 ModbusPollerNode를 종료한다.
func (n *ModbusPollerNode) Shutdown(_ context.Context) error {
	n.stopOnce.Do(func() {
		close(n.stopCh)
	})
	return n.modbusShutdown()
}

// SourceCh 는 폴링된 레지스터 읽기 결과 메시지를 전달하는 채널을 반환한다.
func (n *ModbusPollerNode) SourceCh() <-chan message.Message {
	return n.sourceCh
}

// ---------------------------------------------------------------------------
// register_map 파싱 유틸리티
// ---------------------------------------------------------------------------

// parseRegisterMap 은 config의 register_map 값을 []RegisterMapEntry로 변환한다.
// defaultDeviceID 는 항목에 device_id가 없을 때 사용하는 기본값이다.
func parseRegisterMap(v any, defaultDeviceID uint8) []RegisterMapEntry {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}

	var entries []RegisterMapEntry
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		entry := RegisterMapEntry{}
		if name, ok := m["name"].(string); ok {
			entry.Name = name
		}

		// register_area (기본: holding_registers)
		if area, ok := m["register_area"].(string); ok && area != "" {
			entry.RegisterArea = area
		} else {
			entry.RegisterArea = areaHoldingRegisters
		}

		entry.Address = toUint16FromAny(m["address"])
		if c := toUint16FromAny(m["count"]); c > 0 {
			entry.Count = c
		} else {
			entry.Count = 1
		}
		if dt, ok := m["data_type"].(string); ok && dt != "" {
			entry.DataType = dt
		} else {
			entry.DataType = defaultDataType
		}
		if bo, ok := m["byte_order"].(string); ok && bo != "" {
			entry.ByteOrder = bo
		} else {
			entry.ByteOrder = defaultByteOrder
		}

		// device_id (기본: config의 device_id)
		if d := toByte(m["device_id"]); d > 0 {
			entry.DeviceID = d
		} else {
			entry.DeviceID = defaultDeviceID
		}

		if entry.Name != "" {
			entries = append(entries, entry)
		}
	}
	return entries
}
