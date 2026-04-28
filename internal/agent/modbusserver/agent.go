package modbusserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/internal/agent"
	modbus "github.com/xtra/xflow/internal/modbus"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// Compile-time interface checks
var _ agent.Agent = (*ModbusServerAgent)(nil)
var _ agent.MessageReceiver = (*ModbusServerAgent)(nil)
var _ agent.StatefulAgent = (*ModbusServerAgent)(nil)
var _ agent.BufferInfoProvider = (*ModbusServerAgent)(nil)

// processRequest is the JSON request structure for the Process method.
type processRequest struct {
	Command string         `json:"command"`
	Params  map[string]any `json:"params,omitempty"`
}

// ModbusServerAgent is a MODBUS/TCP server agent.
// It implements agent.Agent, agent.MessageReceiver, and agent.StatefulAgent.
type ModbusServerAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig   agent.AgentConfig
	config        ModbusServerConfig
	deviceManager *DeviceManager
	registerMap   *RegisterMap // 하위 호환: 첫 번째 디바이스의 RegisterMap (Process 메서드용)
	listener      *Listener
	handler       *ModbusHandler
	cancelFn      context.CancelFunc
	msgCh         chan map[string]any
	hasReceiver   *atomic.Bool // ReceiveMessage 호출 시 true → sendChangeEvent 활성화
	receiverOn    chan struct{} // ReceiveMessage 활성화 신호 (drainMsgCh 즉시 종료용, 1회 close)
	receiverOnce  sync.Once     // receiverOn 채널의 1회 close 보장
	stopCh        chan struct{}
	stats         *agent.AgentStats
	logger        *slog.Logger
	mu            sync.RWMutex
	startedAt     time.Time
	createdAt     time.Time
	paused        bool
}

// NewModbusServerAgent creates a new MODBUS/TCP server agent.
func NewModbusServerAgent(agentConfig agent.AgentConfig) (agent.Agent, error) {
	cfg, err := parseModbusServerConfig(agentConfig.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("modbus-server agent: %w", err)
	}

	msgCh := make(chan map[string]any, cfg.MsgChannelSize)
	logger := agent.ResolveLogger(agentConfig)

	// DeviceManager 생성 (멀티-디바이스 지원)
	dm, err := NewDeviceManager(cfg.Devices, logger)
	if err != nil {
		return nil, fmt.Errorf("modbus-server agent: %w", err)
	}

	handler := NewModbusHandler(dm, msgCh, logger)

	listenAddr := fmt.Sprintf("%s:%d", cfg.ListenAddress, cfg.ListenPort)
	listener := NewListener(listenAddr, cfg.MaxConnections, cfg.IdleTimeout, handler, logger)

	a := &ModbusServerAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("modbus-tcp-server")),
		config:        cfg,
		deviceManager: dm,
		registerMap:   dm.FirstDevice().RegisterMap, // 하위 호환: 첫 번째 디바이스
		listener:      listener,
		handler:       handler,
		msgCh:         msgCh,
		hasReceiver:   &atomic.Bool{},
		receiverOn:    make(chan struct{}),
		stopCh:        make(chan struct{}),
		stats:         agent.NewAgentStats(),
		logger:        logger,
		createdAt:     time.Now(),
	}

	if err := a.Init(agentConfig); err != nil {
		return nil, err
	}

	return a, nil
}

// Init initializes the agent.
func (a *ModbusServerAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("modbus-server init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("modbus-server init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("modbus-server init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	a.logger.Info("modbus-server: agent initialized",
		"listen_address", a.config.ListenAddress,
		"listen_port", a.config.ListenPort,
		"unit_id", a.config.UnitID,
	)

	return nil
}

// Start begins the TCP listener.
func (a *ModbusServerAgent) Start(ctx context.Context) error {
	switch a.CurrentState() {
	case lifecycle.StateRunning:
		// 정상 진행
	case lifecycle.StateStopped:
		if err := a.TransitionTo(lifecycle.StateCreated); err != nil {
			return fmt.Errorf("modbus-server start: reset to created: %w", err)
		}
		a.mu.RLock()
		cfg := a.agentConfig
		a.mu.RUnlock()
		return a.Init(cfg)
	default:
		return fmt.Errorf("modbus-server start: agent is not in running state (current: %s)", a.CurrentState())
	}

	childCtx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.cancelFn = cancel
	a.mu.Unlock()

	if err := a.listener.Start(childCtx); err != nil {
		cancel()
		return fmt.Errorf("modbus-server start: %w", err)
	}

	// msgCh 자체 배수 고루틴: 외부 소비자(ReceiveMessage)가 연결되기 전까지
	// 에이전트가 직접 msgCh를 drain하여 채널 오버플로를 방지한다.
	go a.drainMsgCh(childCtx)

	a.logger.Info("modbus-server: listener started")
	return nil
}

// Stop stops the agent and listener.
func (a *ModbusServerAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("modbus-server stop: %w", err)
	}

	a.mu.RLock()
	cancelFn := a.cancelFn
	a.mu.RUnlock()

	if cancelFn != nil {
		cancelFn()
	}

	if err := a.listener.Stop(); err != nil {
		a.logger.Warn("modbus-server: listener stop failed", "error", err)
	}

	// Drain msgCh
	for {
		select {
		case <-a.msgCh:
		default:
			goto drained
		}
	}
drained:

	close(a.stopCh)

	if err := a.TransitionTo(lifecycle.StateStopped); err != nil {
		return fmt.Errorf("modbus-server stop: %w", err)
	}

	return nil
}

// Pause transitions Running -> Paused.
func (a *ModbusServerAgent) Pause(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("modbus-server pause: %w", err)
	}
	a.mu.Lock()
	a.paused = true
	a.mu.Unlock()
	return nil
}

// Resume transitions Paused -> Running.
func (a *ModbusServerAgent) Resume(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("modbus-server resume: %w", err)
	}
	a.mu.Lock()
	a.paused = false
	a.mu.Unlock()
	return nil
}

// Health returns the agent's health status.
func (a *ModbusServerAgent) Health() agent.HealthStatus {
	now := time.Now()
	state := a.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		return agent.HealthStatus{
			Status:    agent.HealthHealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("modbus-server agent is running (active connections: %d)", a.listener.ActiveConnections()),
		}
	case lifecycle.StatePaused:
		return agent.HealthStatus{
			Status:    agent.HealthDegraded,
			LastCheck: now,
			Message:   "modbus-server agent is paused",
		}
	default:
		return agent.HealthStatus{
			Status:    agent.HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("modbus-server agent is in %s state", state),
		}
	}
}

// Process handles JSON commands to interact with the server's register map.
func (a *ModbusServerAgent) Process(data []byte) ([]byte, error) {
	var req processRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("modbus-server process: invalid JSON: %w", err)
	}

	switch req.Command {
	case "set_coil":
		return a.processSetCoil(&req)
	case "set_coils":
		return a.processSetCoils(&req)
	case "set_register":
		return a.processSetRegister(&req)
	case "set_registers":
		return a.processSetRegisters(&req)
	case "set_input":
		return a.processSetInput(&req)
	case "set_inputs":
		return a.processSetInputs(&req)
	case "bulk_write":
		return a.processBulkWrite(&req)
	case "get_coils":
		return a.processGetCoils(&req)
	case "get_discrete_inputs":
		return a.processGetDiscreteInputs(&req)
	case "get_holding_registers":
		return a.processGetHoldingRegisters(&req)
	case "get_input_registers":
		return a.processGetInputRegisters(&req)
	case "get_register_typed":
		return a.processGetRegisterTyped(&req)
	case "get_map":
		return a.processGetMap(&req)
	case "get_register_defs":
		return a.processGetRegisterDefs(&req)
	case "get_status":
		return a.processGetStatus()
	case "read_raw":
		return a.processReadRaw(&req)
	case "list_devices":
		return a.processListDevices()
	case "add_device":
		return a.processAddDevice(&req)
	case "remove_device":
		return a.processRemoveDevice(&req)
	case "get_device_status":
		return a.processGetDeviceStatus(&req)
	default:
		return nil, ErrInvalidCommand
	}
}

// processSetCoil sets a single coil value.
func (a *ModbusServerAgent) processSetCoil(req *processRequest) ([]byte, error) {
	rm := a.resolveRegisterMap(req.Params)
	addr, ok := getParamInt(req.Params, "address")
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_coil requires 'address' param")
	}
	val, ok := getParamBool(req.Params, "value")
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_coil requires 'value' param (bool)")
	}

	cs, err := rm.WriteCoils(uint16(addr), []bool{val})
	if err != nil {
		return nil, fmt.Errorf("modbus-server: set_coil: %w", err)
	}

	a.sendChangeEvent(cs, req.Command, "")
	return json.Marshal(map[string]any{"ok": true, "address": addr, "value": val})
}

// processSetCoils sets multiple coil values.
func (a *ModbusServerAgent) processSetCoils(req *processRequest) ([]byte, error) {
	rm := a.resolveRegisterMap(req.Params)
	addr, ok := getParamInt(req.Params, "address")
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_coils requires 'address' param")
	}
	rawValues, ok := req.Params["values"]
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_coils requires 'values' param")
	}
	arr, ok := rawValues.([]any)
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_coils 'values' must be an array")
	}

	values := make([]bool, len(arr))
	for i, v := range arr {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("modbus-server: set_coils values[%d] must be bool", i)
		}
		values[i] = b
	}

	cs, err := rm.WriteCoils(uint16(addr), values)
	if err != nil {
		return nil, fmt.Errorf("modbus-server: set_coils: %w", err)
	}

	a.sendChangeEvent(cs, req.Command, "")
	return json.Marshal(map[string]any{"ok": true, "address": addr, "quantity": len(values)})
}

// processSetRegister sets a single holding register value.
// Supports optional data_type and byte_order params for typed writes.
func (a *ModbusServerAgent) processSetRegister(req *processRequest) ([]byte, error) {
	rm := a.resolveRegisterMap(req.Params)
	addr, ok := getParamInt(req.Params, "address")
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_register requires 'address' param")
	}

	dataType, byteOrder := a.resolveDataType(req.Params, "holding_registers", uint16(addr))

	// 타입이 지정된 경우: WriteTyped 사용
	if dataType != modbus.DataTypeUint16 {
		value, ok := getParamFloat64(req.Params, "value")
		if !ok {
			return nil, fmt.Errorf("modbus-server: set_register requires 'value' param")
		}
		cs, err := rm.WriteTyped("holding_registers", uint16(addr), value, dataType, byteOrder)
		if err != nil {
			return nil, fmt.Errorf("modbus-server: set_register: %w", err)
		}
		a.sendChangeEvent(cs, req.Command, dataType)
		return json.Marshal(map[string]any{"ok": true, "address": addr, "value": value, "data_type": dataType})
	}

	// 기본 uint16: 기존 동작 유지
	val, ok := getParamInt(req.Params, "value")
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_register requires 'value' param (int)")
	}

	cs, err := rm.WriteHoldingRegisters(uint16(addr), []uint16{uint16(val)})
	if err != nil {
		return nil, fmt.Errorf("modbus-server: set_register: %w", err)
	}

	a.sendChangeEvent(cs, req.Command, "")
	return json.Marshal(map[string]any{"ok": true, "address": addr, "value": val})
}

// processSetRegisters sets multiple holding register values.
// Supports optional data_type and byte_order params for typed writes.
func (a *ModbusServerAgent) processSetRegisters(req *processRequest) ([]byte, error) {
	rm := a.resolveRegisterMap(req.Params)
	addr, ok := getParamInt(req.Params, "address")
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_registers requires 'address' param")
	}
	rawValues, ok := req.Params["values"]
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_registers requires 'values' param")
	}
	arr, ok := rawValues.([]any)
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_registers 'values' must be an array")
	}

	dataType, _ := getParamString(req.Params, "data_type")
	byteOrder, _ := getParamString(req.Params, "byte_order")
	if byteOrder == "" {
		byteOrder = modbus.ByteOrderBigEndian
	}
	if byteOrder != modbus.ByteOrderBigEndian && byteOrder != modbus.ByteOrderLittleEndian {
		return nil, fmt.Errorf("modbus-server: set_registers: byte_order must be %q or %q (got %q)",
			modbus.ByteOrderBigEndian, modbus.ByteOrderLittleEndian, byteOrder)
	}

	// TypeOverlay 기본값 적용: 명시적 data_type이 없으면 resolveDataType으로 해석
	if dataType == "" {
		dataType, byteOrder = a.resolveDataType(req.Params, "holding_registers", uint16(addr))
	}

	// 타입이 지정된 경우: 각 값을 TypedValueToRegisters로 변환
	if dataType != modbus.DataTypeUint16 {
		var allRegs []uint16
		for i, v := range arr {
			regs, err := modbus.TypedValueToRegisters(v, dataType, byteOrder)
			if err != nil {
				return nil, fmt.Errorf("modbus-server: set_registers values[%d]: %w", i, err)
			}
			allRegs = append(allRegs, regs...)
		}

		cs, err := rm.WriteHoldingRegisters(uint16(addr), allRegs)
		if err != nil {
			return nil, fmt.Errorf("modbus-server: set_registers: %w", err)
		}

		a.sendChangeEvent(cs, req.Command, dataType)
		return json.Marshal(map[string]any{"ok": true, "address": addr, "quantity": len(arr), "data_type": dataType})
	}

	// 기본 uint16: 기존 동작 유지
	values := make([]uint16, len(arr))
	for i, v := range arr {
		n, ok := toParamUint16(v)
		if !ok {
			return nil, fmt.Errorf("modbus-server: set_registers values[%d] must be a number", i)
		}
		values[i] = n
	}

	cs, err := rm.WriteHoldingRegisters(uint16(addr), values)
	if err != nil {
		return nil, fmt.Errorf("modbus-server: set_registers: %w", err)
	}

	a.sendChangeEvent(cs, req.Command, "")
	return json.Marshal(map[string]any{"ok": true, "address": addr, "quantity": len(values)})
}

// processSetInput sets a single input register or discrete input value.
// Supports optional data_type and byte_order params for typed writes on input_registers.
func (a *ModbusServerAgent) processSetInput(req *processRequest) ([]byte, error) {
	rm := a.resolveRegisterMap(req.Params)
	area, ok := req.Params["area"].(string)
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_input requires 'area' param (string)")
	}
	addr, ok := getParamInt(req.Params, "address")
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_input requires 'address' param")
	}

	switch area {
	case "input_registers":
		dataType, byteOrder := a.resolveDataType(req.Params, "input_registers", uint16(addr))

		// 타입이 지정된 경우: WriteTyped 사용
		if dataType != modbus.DataTypeUint16 {
			value, ok := getParamFloat64(req.Params, "value")
			if !ok {
				return nil, fmt.Errorf("modbus-server: set_input requires 'value' param")
			}
			cs, err := rm.WriteTyped("input_registers", uint16(addr), value, dataType, byteOrder)
			if err != nil {
				return nil, fmt.Errorf("modbus-server: set_input: %w", err)
			}
			a.sendChangeEvent(cs, req.Command, dataType)
			return json.Marshal(map[string]any{"ok": true, "area": area, "address": addr, "value": value, "data_type": dataType})
		}

		// 기본 uint16: 기존 동작 유지
		val, ok := getParamInt(req.Params, "value")
		if !ok {
			return nil, fmt.Errorf("modbus-server: set_input requires 'value' param (int) for input_registers")
		}
		cs, err := rm.WriteInputRegisters(uint16(addr), []uint16{uint16(val)})
		if err != nil {
			return nil, fmt.Errorf("modbus-server: set_input: %w", err)
		}
		a.sendChangeEvent(cs, req.Command, "")
		return json.Marshal(map[string]any{"ok": true, "area": area, "address": addr, "value": val})

	case "discrete_inputs":
		val, ok := getParamBool(req.Params, "value")
		if !ok {
			return nil, fmt.Errorf("modbus-server: set_input requires 'value' param (bool) for discrete_inputs")
		}
		cs, err := rm.WriteDiscreteInputs(uint16(addr), []bool{val})
		if err != nil {
			return nil, fmt.Errorf("modbus-server: set_input: %w", err)
		}
		a.sendChangeEvent(cs, req.Command, "")
		return json.Marshal(map[string]any{"ok": true, "area": area, "address": addr, "value": val})

	default:
		return nil, fmt.Errorf("modbus-server: set_input: unsupported area %q", area)
	}
}

// processSetInputs sets multiple input registers or discrete inputs.
// Supports optional data_type and byte_order params for typed writes on input_registers.
func (a *ModbusServerAgent) processSetInputs(req *processRequest) ([]byte, error) {
	area, ok := req.Params["area"].(string)
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_inputs requires 'area' param (string)")
	}
	addr, ok := getParamInt(req.Params, "address")
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_inputs requires 'address' param")
	}
	rawValues, ok := req.Params["values"]
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_inputs requires 'values' param")
	}
	arr, ok := rawValues.([]any)
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_inputs 'values' must be an array")
	}

	switch area {
	case "input_registers":
		rm := a.resolveRegisterMap(req.Params)
		dataType, _ := getParamString(req.Params, "data_type")
		byteOrder, _ := getParamString(req.Params, "byte_order")
		if byteOrder == "" {
			byteOrder = modbus.ByteOrderBigEndian
		}
		if byteOrder != modbus.ByteOrderBigEndian && byteOrder != modbus.ByteOrderLittleEndian {
			return nil, fmt.Errorf("modbus-server: set_inputs: byte_order must be %q or %q (got %q)",
				modbus.ByteOrderBigEndian, modbus.ByteOrderLittleEndian, byteOrder)
		}

		// TypeOverlay 기본값 적용: 명시적 data_type이 없으면 resolveDataType으로 해석
		if dataType == "" {
			dataType, byteOrder = a.resolveDataType(req.Params, "input_registers", uint16(addr))
		}

		// 타입이 지정된 경우: 각 값을 TypedValueToRegisters로 변환
		if dataType != modbus.DataTypeUint16 {
			var allRegs []uint16
			for i, v := range arr {
				regs, err := modbus.TypedValueToRegisters(v, dataType, byteOrder)
				if err != nil {
					return nil, fmt.Errorf("modbus-server: set_inputs values[%d]: %w", i, err)
				}
				allRegs = append(allRegs, regs...)
			}

			cs, err := rm.WriteInputRegisters(uint16(addr), allRegs)
			if err != nil {
				return nil, fmt.Errorf("modbus-server: set_inputs: %w", err)
			}

			a.sendChangeEvent(cs, req.Command, dataType)
			return json.Marshal(map[string]any{"ok": true, "area": area, "address": addr, "quantity": len(arr), "data_type": dataType})
		}

		// 기본 uint16: 기존 동작 유지
		values := make([]uint16, len(arr))
		for i, v := range arr {
			n, ok := toParamUint16(v)
			if !ok {
				return nil, fmt.Errorf("modbus-server: set_inputs values[%d] must be a number", i)
			}
			values[i] = n
		}
		cs, err := rm.WriteInputRegisters(uint16(addr), values)
		if err != nil {
			return nil, fmt.Errorf("modbus-server: set_inputs: %w", err)
		}
		a.sendChangeEvent(cs, req.Command, "")
		return json.Marshal(map[string]any{"ok": true, "area": area, "address": addr, "quantity": len(values)})

	case "discrete_inputs":
		rm := a.resolveRegisterMap(req.Params)
		values := make([]bool, len(arr))
		for i, v := range arr {
			b, ok := v.(bool)
			if !ok {
				return nil, fmt.Errorf("modbus-server: set_inputs values[%d] must be bool", i)
			}
			values[i] = b
		}
		cs, err := rm.WriteDiscreteInputs(uint16(addr), values)
		if err != nil {
			return nil, fmt.Errorf("modbus-server: set_inputs: %w", err)
		}
		a.sendChangeEvent(cs, req.Command, "")
		return json.Marshal(map[string]any{"ok": true, "area": area, "address": addr, "quantity": len(values)})

	default:
		return nil, fmt.Errorf("modbus-server: set_inputs: unsupported area %q", area)
	}
}

// processBulkWrite 는 여러 영역의 레지스터/코일을 한 번에 쓴다.
// params.writes 배열의 각 항목은 area, address, value, (선택) data_type, byte_order를 포함한다.
// 브릿지 어댑터가 플로우 메시지를 ModbusServerAgent에 전달할 때 사용한다.
func (a *ModbusServerAgent) processBulkWrite(req *processRequest) ([]byte, error) {
	rm := a.resolveRegisterMap(req.Params)
	rawWrites, ok := req.Params["writes"]
	if !ok {
		return nil, fmt.Errorf("modbus-server: bulk_write requires 'writes' param")
	}
	writes, ok := rawWrites.([]any)
	if !ok {
		return nil, fmt.Errorf("modbus-server: bulk_write 'writes' must be an array")
	}
	if len(writes) == 0 {
		return json.Marshal(map[string]any{"ok": true, "count": 0})
	}

	count := 0
	for i, w := range writes {
		entry, ok := w.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("modbus-server: bulk_write writes[%d] must be an object", i)
		}

		area, _ := entry["area"].(string)
		addr, ok := getParamInt(entry, "address")
		if !ok {
			return nil, fmt.Errorf("modbus-server: bulk_write writes[%d] requires 'address'", i)
		}

		switch area {
		case "holding_registers":
			dataType, _ := entry["data_type"].(string)
			byteOrder, _ := entry["byte_order"].(string)
			if byteOrder == "" {
				byteOrder = modbus.ByteOrderBigEndian
			}
			if dataType == "" {
				dataType, byteOrder = resolveDataTypeFromRM(rm, entry, "holding_registers", uint16(addr))
			}
			value, ok := getParamFloat64(entry, "value")
			if !ok {
				return nil, fmt.Errorf("modbus-server: bulk_write writes[%d] requires 'value'", i)
			}
			if dataType != modbus.DataTypeUint16 {
				cs, err := rm.WriteTyped("holding_registers", uint16(addr), value, dataType, byteOrder)
				if err != nil {
					return nil, fmt.Errorf("modbus-server: bulk_write writes[%d]: %w", i, err)
				}
				a.sendChangeEvent(cs, "bulk_write", dataType)
			} else {
				cs, err := rm.WriteHoldingRegisters(uint16(addr), []uint16{uint16(value)})
				if err != nil {
					return nil, fmt.Errorf("modbus-server: bulk_write writes[%d]: %w", i, err)
				}
				a.sendChangeEvent(cs, "bulk_write", "")
			}

		case "input_registers":
			dataType, _ := entry["data_type"].(string)
			byteOrder, _ := entry["byte_order"].(string)
			if byteOrder == "" {
				byteOrder = modbus.ByteOrderBigEndian
			}
			if dataType == "" {
				dataType, byteOrder = resolveDataTypeFromRM(rm, entry, "input_registers", uint16(addr))
			}
			value, ok := getParamFloat64(entry, "value")
			if !ok {
				return nil, fmt.Errorf("modbus-server: bulk_write writes[%d] requires 'value'", i)
			}
			if dataType != modbus.DataTypeUint16 {
				cs, err := rm.WriteTyped("input_registers", uint16(addr), value, dataType, byteOrder)
				if err != nil {
					return nil, fmt.Errorf("modbus-server: bulk_write writes[%d]: %w", i, err)
				}
				a.sendChangeEvent(cs, "bulk_write", dataType)
			} else {
				cs, err := rm.WriteInputRegisters(uint16(addr), []uint16{uint16(value)})
				if err != nil {
					return nil, fmt.Errorf("modbus-server: bulk_write writes[%d]: %w", i, err)
				}
				a.sendChangeEvent(cs, "bulk_write", "")
			}

		case "coils":
			value, ok := getParamBool(entry, "value")
			if !ok {
				return nil, fmt.Errorf("modbus-server: bulk_write writes[%d] requires 'value' (bool) for coils", i)
			}
			cs, err := rm.WriteCoils(uint16(addr), []bool{value})
			if err != nil {
				return nil, fmt.Errorf("modbus-server: bulk_write writes[%d]: %w", i, err)
			}
			a.sendChangeEvent(cs, "bulk_write", "")

		case "discrete_inputs":
			value, ok := getParamBool(entry, "value")
			if !ok {
				return nil, fmt.Errorf("modbus-server: bulk_write writes[%d] requires 'value' (bool) for discrete_inputs", i)
			}
			cs, err := rm.WriteDiscreteInputs(uint16(addr), []bool{value})
			if err != nil {
				return nil, fmt.Errorf("modbus-server: bulk_write writes[%d]: %w", i, err)
			}
			a.sendChangeEvent(cs, "bulk_write", "")

		default:
			return nil, fmt.Errorf("modbus-server: bulk_write writes[%d]: unsupported area %q", i, area)
		}
		count++
	}

	return json.Marshal(map[string]any{"ok": true, "count": count})
}

// processGetCoils reads coil values by address and quantity.
func (a *ModbusServerAgent) processGetCoils(req *processRequest) ([]byte, error) {
	rm := a.resolveRegisterMap(req.Params)
	addr, ok := getParamInt(req.Params, "address")
	if !ok {
		return nil, fmt.Errorf("modbus-server: get_coils requires 'address' param")
	}
	qty, ok := getParamInt(req.Params, "quantity")
	if !ok {
		return nil, fmt.Errorf("modbus-server: get_coils requires 'quantity' param")
	}

	values, err := rm.ReadCoils(uint16(addr), uint16(qty))
	if err != nil {
		return nil, fmt.Errorf("modbus-server: get_coils: %w", err)
	}

	return json.Marshal(map[string]any{
		"ok":       true,
		"address":  addr,
		"quantity": qty,
		"values":   values,
	})
}

// processGetDiscreteInputs reads discrete input values by address and quantity.
func (a *ModbusServerAgent) processGetDiscreteInputs(req *processRequest) ([]byte, error) {
	rm := a.resolveRegisterMap(req.Params)
	addr, ok := getParamInt(req.Params, "address")
	if !ok {
		return nil, fmt.Errorf("modbus-server: get_discrete_inputs requires 'address' param")
	}
	qty, ok := getParamInt(req.Params, "quantity")
	if !ok {
		return nil, fmt.Errorf("modbus-server: get_discrete_inputs requires 'quantity' param")
	}

	values, err := rm.ReadDiscreteInputs(uint16(addr), uint16(qty))
	if err != nil {
		return nil, fmt.Errorf("modbus-server: get_discrete_inputs: %w", err)
	}

	return json.Marshal(map[string]any{
		"ok":       true,
		"address":  addr,
		"quantity": qty,
		"values":   values,
	})
}

// processGetHoldingRegisters reads holding register values by address and quantity.
func (a *ModbusServerAgent) processGetHoldingRegisters(req *processRequest) ([]byte, error) {
	rm := a.resolveRegisterMap(req.Params)
	addr, ok := getParamInt(req.Params, "address")
	if !ok {
		return nil, fmt.Errorf("modbus-server: get_holding_registers requires 'address' param")
	}
	qty, ok := getParamInt(req.Params, "quantity")
	if !ok {
		return nil, fmt.Errorf("modbus-server: get_holding_registers requires 'quantity' param")
	}

	values, err := rm.ReadHoldingRegisters(uint16(addr), uint16(qty))
	if err != nil {
		return nil, fmt.Errorf("modbus-server: get_holding_registers: %w", err)
	}

	resp := map[string]any{
		"ok":       true,
		"address":  addr,
		"quantity": qty,
	}

	// data_type 파라미터가 지정되면 해당 타입으로 변환하여 출력
	if dt, _ := getParamString(req.Params, "data_type"); dt != "" {
		byteOrder, _ := getParamString(req.Params, "byte_order")
		if byteOrder == "" {
			byteOrder = modbus.ByteOrderBigEndian
		}
		resp["values"] = a.convertValues(values, dt, byteOrder)
	} else {
		resp["values"] = values
		if tv := a.buildTypedValues(rm, "holding_registers", uint16(addr), values); tv != nil {
			resp["typed_values"] = tv
		}
	}

	return json.Marshal(resp)
}

// processGetInputRegisters reads input register values by address and quantity.
func (a *ModbusServerAgent) processGetInputRegisters(req *processRequest) ([]byte, error) {
	rm := a.resolveRegisterMap(req.Params)
	addr, ok := getParamInt(req.Params, "address")
	if !ok {
		return nil, fmt.Errorf("modbus-server: get_input_registers requires 'address' param")
	}
	qty, ok := getParamInt(req.Params, "quantity")
	if !ok {
		return nil, fmt.Errorf("modbus-server: get_input_registers requires 'quantity' param")
	}

	values, err := rm.ReadInputRegisters(uint16(addr), uint16(qty))
	if err != nil {
		return nil, fmt.Errorf("modbus-server: get_input_registers: %w", err)
	}

	resp := map[string]any{
		"ok":       true,
		"address":  addr,
		"quantity": qty,
	}

	if dt, _ := getParamString(req.Params, "data_type"); dt != "" {
		byteOrder, _ := getParamString(req.Params, "byte_order")
		if byteOrder == "" {
			byteOrder = modbus.ByteOrderBigEndian
		}
		resp["values"] = a.convertValues(values, dt, byteOrder)
	} else {
		resp["values"] = values
		if tv := a.buildTypedValues(rm, "input_registers", uint16(addr), values); tv != nil {
			resp["typed_values"] = tv
		}
	}

	return json.Marshal(resp)
}

// convertValues 는 raw 레지스터 배열을 지정된 데이터 타입으로 변환하여 슬라이스로 반환한다.
// 변환 실패 시 해당 위치는 raw uint16 값을 유지한다.
func (a *ModbusServerAgent) convertValues(rawValues []uint16, dataType string, byteOrder string) []any {
	regCount, err := modbus.RegisterCountForType(dataType)
	if err != nil {
		// 알 수 없는 타입이면 원본 반환
		result := make([]any, len(rawValues))
		for i, v := range rawValues {
			result[i] = v
		}
		return result
	}

	var result []any
	qty := uint16(len(rawValues))
	for offset := uint16(0); offset+regCount <= qty; offset += regCount {
		regs := rawValues[offset : offset+regCount]
		val, err := modbus.RegistersToTypedValue(regs, dataType, byteOrder)
		if err != nil {
			for _, r := range regs {
				result = append(result, r)
			}
			continue
		}
		result = append(result, val)
	}
	// 나머지 레지스터 (타입에 필요한 수보다 부족한 경우)
	remainder := qty % regCount
	if remainder != 0 {
		for i := qty - remainder; i < qty; i++ {
			result = append(result, rawValues[i])
		}
	}
	return result
}

// buildTypedValues 는 TypeOverlay를 사용하여 raw 레지스터 값을 타입 변환된 값 목록으로 변환한다.
// TypeOverlay에 해당 영역의 엔트리가 없으면 nil을 반환한다.
func (a *ModbusServerAgent) buildTypedValues(rm *RegisterMap, area string, startAddr uint16, rawValues []uint16) []map[string]any {
	overlay := rm.GetTypeOverlay()
	if overlay == nil {
		return nil
	}

	var result []map[string]any
	qty := uint16(len(rawValues))

	for offset := uint16(0); offset < qty; {
		addr := startAddr + offset
		key := area + ":" + fmt.Sprintf("%d", addr)

		entry, hasType := overlay[key]
		if !hasType {
			// TypeOverlay에 없으면 uint16 기본 처리
			entry = modbus.TypeOverlayEntry{
				DataType:      modbus.DataTypeUint16,
				RegisterCount: 1,
				ByteOrder:     modbus.ByteOrderBigEndian,
			}
		}

		regCount := entry.RegisterCount
		if offset+regCount > qty {
			// 남은 레지스터가 타입에 필요한 수보다 부족하면 uint16로 개별 출력
			for i := offset; i < qty; i++ {
				result = append(result, map[string]any{
					"address":    startAddr + i,
					"data_type":  modbus.DataTypeUint16,
					"byte_order": modbus.ByteOrderBigEndian,
					"value":      rawValues[i],
				})
			}
			break
		}

		regs := rawValues[offset : offset+regCount]
		value, err := modbus.RegistersToTypedValue(regs, entry.DataType, entry.ByteOrder)
		if err != nil {
			// 변환 실패 시 raw uint16으로 폴백
			for _, r := range regs {
				result = append(result, map[string]any{
					"address":    addr,
					"data_type":  modbus.DataTypeUint16,
					"byte_order": modbus.ByteOrderBigEndian,
					"value":      r,
				})
				addr++
			}
			offset += regCount
			continue
		}

		result = append(result, map[string]any{
			"address":    addr,
			"data_type":  entry.DataType,
			"byte_order": entry.ByteOrder,
			"value":      value,
		})

		offset += regCount
	}

	return result
}

// processGetRegisterTyped reads a single typed register value.
func (a *ModbusServerAgent) processGetRegisterTyped(req *processRequest) ([]byte, error) {
	rm := a.resolveRegisterMap(req.Params)
	addr, ok := getParamInt(req.Params, "address")
	if !ok {
		return nil, fmt.Errorf("modbus-server: get_register_typed requires 'address' param")
	}
	area, ok := getParamString(req.Params, "area")
	if !ok {
		return nil, fmt.Errorf("modbus-server: get_register_typed requires 'area' param")
	}

	dataType, byteOrder := a.resolveDataType(req.Params, area, uint16(addr))

	value, err := rm.ReadTyped(area, uint16(addr), dataType, byteOrder)
	if err != nil {
		return nil, fmt.Errorf("modbus-server: get_register_typed: %w", err)
	}

	return json.Marshal(map[string]any{
		"ok":        true,
		"address":   addr,
		"value":     value,
		"data_type": dataType,
		"area":      area,
	})
}

// processGetMap returns the register map snapshot.
// If a TypeOverlay exists, it is included in the response.
func (a *ModbusServerAgent) processGetMap(req *processRequest) ([]byte, error) {
	rm := a.resolveRegisterMap(req.Params)
	snap := rm.GetSnapshot()
	resp := map[string]any{"register_map": snap}

	if overlay := rm.GetTypeOverlay(); overlay != nil {
		resp["type_overlay"] = overlay
	}

	return json.Marshal(resp)
}

// processReadRaw 는 로컬 레지스터 스토어에서 원시 바이트를 읽어 base64 인코딩하여 반환한다.
// 브릿지 주도 폴링에서 사용된다. params: function_code, address, quantity.
func (a *ModbusServerAgent) processReadRaw(req *processRequest) ([]byte, error) {
	if req.Params == nil {
		return nil, fmt.Errorf("modbus-server read_raw: params required")
	}

	rm := a.resolveRegisterMap(req.Params)
	fc, ok := getParamInt(req.Params, "function_code")
	if !ok {
		return nil, fmt.Errorf("modbus-server read_raw: function_code required")
	}
	addr, ok := getParamInt(req.Params, "address")
	if !ok {
		return nil, fmt.Errorf("modbus-server read_raw: address required")
	}
	qty, ok := getParamInt(req.Params, "quantity")
	if !ok {
		return nil, fmt.Errorf("modbus-server read_raw: quantity required")
	}

	var values []uint16
	var err error

	const (
		fc03 = 3 // ReadHoldingRegisters
		fc04 = 4 // ReadInputRegisters
	)
	switch byte(fc) {
	case fc03:
		values, err = rm.ReadHoldingRegisters(uint16(addr), uint16(qty))
	case fc04:
		values, err = rm.ReadInputRegisters(uint16(addr), uint16(qty))
	default:
		return nil, fmt.Errorf("modbus-server read_raw: unsupported function_code %d (only FC03, FC04)", fc)
	}
	if err != nil {
		return nil, fmt.Errorf("modbus-server read_raw: %w", err)
	}

	rawBytes := encodeRegisters(values)
	resp := map[string]any{
		"data": base64.StdEncoding.EncodeToString(rawBytes),
	}
	return json.Marshal(resp)
}

// processGetStatus returns the server status.
func (a *ModbusServerAgent) processGetStatus() ([]byte, error) {
	a.mu.RLock()
	startedAt := a.startedAt
	a.mu.RUnlock()

	resp := map[string]any{
		"listen_address":     a.config.ListenAddress,
		"listen_port":        a.config.ListenPort,
		"unit_id":            a.config.UnitID,
		"active_connections": a.listener.ActiveConnections(),
		"max_connections":    a.config.MaxConnections,
		"uptime_seconds":     time.Since(startedAt).Seconds(),
	}
	return json.Marshal(resp)
}

// ---------------------------------------------------------------------------
// Exec commands: device management (M3)
// ---------------------------------------------------------------------------

// processListDevices 는 등록된 모든 디바이스 목록을 반환한다.
func (a *ModbusServerAgent) processListDevices() ([]byte, error) {
	devices := a.deviceManager.GetAllDevices()

	devList := make([]map[string]any, 0, len(devices))
	for _, dev := range devices {
		devList = append(devList, map[string]any{
			"unit_id":         dev.UnitID,
			"name":            dev.Name,
			"register_counts": dev.RegisterMap.RegisterCounts(),
			"status":          "active",
			"stats": map[string]any{
				"read_count":  dev.Stats.ReadCount.Load(),
				"write_count": dev.Stats.WriteCount.Load(),
				"error_count": dev.Stats.ErrorCount.Load(),
			},
		})
	}

	resp := map[string]any{
		"devices":      devList,
		"device_count": len(devList),
	}
	return json.Marshal(resp)
}

// processAddDevice 는 런타임에 새 디바이스를 추가한다.
// params: unit_id (1-247, 필수), name (선택), register_map (필수)
func (a *ModbusServerAgent) processAddDevice(req *processRequest) ([]byte, error) {
	// unit_id 파라미터 추출 및 검증
	uid, ok := getParamInt(req.Params, "unit_id")
	if !ok {
		return nil, fmt.Errorf("modbus-server: add_device requires 'unit_id' param: %w", ErrInvalidDeviceConfig)
	}
	if uid < 1 || uid > 247 {
		return nil, fmt.Errorf("modbus-server: unit_id must be between 1 and 247, got %d: %w", uid, ErrInvalidDeviceConfig)
	}

	// register_map 파라미터 추출 및 파싱
	rmRaw, ok := req.Params["register_map"]
	if !ok {
		return nil, fmt.Errorf("modbus-server: add_device requires 'register_map' param: %w", ErrInvalidDeviceConfig)
	}
	rmMap, ok := rmRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("modbus-server: register_map must be an object: %w", ErrInvalidDeviceConfig)
	}
	rmCfg, err := parseRegisterMapConfig(rmMap)
	if err != nil {
		return nil, fmt.Errorf("modbus-server: add_device register_map parse error: %w", err)
	}

	// name 파라미터 (선택)
	name, _ := getParamString(req.Params, "name")
	if name == "" {
		name = fmt.Sprintf("device-%d", uid)
	}

	// register_defs 파라미터 (선택: 디바이스별 레지스터 정의)
	var registerDefs []any
	if rawDefs, ok := req.Params["register_defs"]; ok {
		if defs, ok := rawDefs.([]any); ok {
			registerDefs = defs
		}
	}

	// RegisterMap + RequestHandler 생성
	rm := NewRegisterMap(rmCfg)
	reqHandler := NewRequestHandler(rm, a.logger)

	dev := &Device{
		UnitID:       byte(uid),
		Name:         name,
		RegisterMap:  rm,
		ReqHandler:   reqHandler,
		RegisterDefs: registerDefs,
	}

	// DeviceManager 에 추가 (중복 UnitID 검사 포함)
	if err := a.deviceManager.AddDevice(dev); err != nil {
		return nil, err
	}

	resp := map[string]any{
		"status":          "added",
		"unit_id":         dev.UnitID,
		"name":            dev.Name,
		"register_counts": rm.RegisterCounts(),
	}
	return json.Marshal(resp)
}

// processRemoveDevice 는 런타임에 디바이스를 제거한다.
// params: unit_id (필수)
func (a *ModbusServerAgent) processRemoveDevice(req *processRequest) ([]byte, error) {
	uid, ok := getParamInt(req.Params, "unit_id")
	if !ok {
		return nil, fmt.Errorf("modbus-server: remove_device requires 'unit_id' param: %w", ErrInvalidDeviceConfig)
	}

	// 마지막 디바이스 제거 방지 (AC-015)
	if a.deviceManager.DeviceCount() <= 1 {
		return nil, fmt.Errorf("modbus-server: cannot remove last device: %w", ErrInvalidDeviceConfig)
	}

	if err := a.deviceManager.RemoveDevice(byte(uid)); err != nil {
		return nil, err
	}

	resp := map[string]any{
		"status":  "removed",
		"unit_id": uid,
	}
	return json.Marshal(resp)
}

// processGetDeviceStatus 는 특정 디바이스의 상세 상태를 반환한다.
// params: unit_id (필수)
func (a *ModbusServerAgent) processGetDeviceStatus(req *processRequest) ([]byte, error) {
	uid, ok := getParamInt(req.Params, "unit_id")
	if !ok {
		return nil, fmt.Errorf("modbus-server: get_device_status requires 'unit_id' param: %w", ErrInvalidDeviceConfig)
	}

	dev := a.deviceManager.GetDevice(byte(uid))
	if dev == nil {
		return nil, fmt.Errorf("modbus-server: device with unit_id %d not found: %w", uid, ErrDeviceNotFound)
	}

	lastAccess := dev.Stats.GetLastAccess()
	var lastAccessStr string
	if !lastAccess.IsZero() {
		lastAccessStr = lastAccess.Format(time.RFC3339)
	}

	resp := map[string]any{
		"unit_id":         dev.UnitID,
		"name":            dev.Name,
		"register_counts": dev.RegisterMap.RegisterCounts(),
		"register_map":    dev.RegisterMap.GetSnapshot(),
		"stats": map[string]any{
			"read_count":  dev.Stats.ReadCount.Load(),
			"write_count": dev.Stats.WriteCount.Load(),
			"error_count": dev.Stats.ErrorCount.Load(),
			"last_access": lastAccessStr,
		},
	}
	return json.Marshal(resp)
}

// sendChangeEvent sends a register_updated event to msgCh (non-blocking).
// dataType is optional; when non-empty, it is included in the notification.
func (a *ModbusServerAgent) sendChangeEvent(cs *ChangeSet, command string, dataType string) {
	if cs == nil {
		return
	}

	notification := map[string]any{
		"type":       "register_updated",
		"source":     "internal",
		"command":    command,
		"area":       cs.Area,
		"address":    cs.Address,
		"quantity":   cs.Quantity,
		"old_values": cs.OldValues,
		"new_values": cs.NewValues,
	}

	if dataType != "" {
		notification["data_type"] = dataType
	}

	select {
	case a.msgCh <- notification:
	default:
		a.logger.Warn("modbus-server: msgCh full, dropping event", "command", command)
	}
}

// ReceiveMessage receives a message from msgCh.
// Implements agent.MessageReceiver.
// 최초 호출 시 hasReceiver=true + receiverOn close 로 drainMsgCh 를 즉시 종료시킨다.
// (atomic 만으로는 drainMsgCh 가 select 블록 중일 때 종료를 보장 못함 → channel close 로
//  select 의 첫 번째 case 를 깨운다.)
func (a *ModbusServerAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	a.activateReceiver()
	select {
	case msg := <-a.msgCh:
		data, err := json.Marshal(msg)
		if err != nil {
			return nil, fmt.Errorf("modbus-server: marshal message: %w", err)
		}
		return data, nil
	case <-a.stopCh:
		return nil, fmt.Errorf("modbus-server: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// activateReceiver 는 hasReceiver 를 true 로 설정하고 receiverOn 채널을 닫는다.
// 한 번만 닫힐 수 있도록 sync.Once 로 보장하며, drainMsgCh 가 receiverOn 종료를
// select case 로 감지해 즉시 빠져나오도록 한다.
func (a *ModbusServerAgent) activateReceiver() {
	a.hasReceiver.Store(true)
	a.receiverOnce.Do(func() {
		close(a.receiverOn)
	})
}

// drainMsgCh 는 외부 소비자(ReceiveMessage)가 연결되기 전까지 msgCh를 자체 배수한다.
// receiverOn 채널이 닫히면 (= 첫 ReceiveMessage 호출) 즉시 종료한다.
// hasReceiver atomic 만 보던 이전 구현은 select 블록 중일 때 종료가 지연되어
// drainMsgCh 가 들어오는 알림을 가로채는 race 가 있었기에 channel signal 로 교체.
func (a *ModbusServerAgent) drainMsgCh(ctx context.Context) {
	for {
		select {
		case <-a.receiverOn:
			return
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-a.msgCh:
			// 이벤트 폐기
		}
	}
}

// Configure validates and stores the agent configuration.
func (a *ModbusServerAgent) Configure(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("modbus-server configure: %w", err)
	}
	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()
	return nil
}

// ID returns the agent ID.
func (a *ModbusServerAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name returns the agent name.
func (a *ModbusServerAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type returns the agent type.
func (a *ModbusServerAgent) Type() string {
	return "modbus-tcp-server"
}

// Info returns the agent info snapshot.
func (a *ModbusServerAgent) Info() agent.AgentInfo {
	a.mu.RLock()
	cfg := a.agentConfig
	startedAt := a.startedAt
	createdAt := a.createdAt
	a.mu.RUnlock()

	state := a.CurrentState()
	var uptime time.Duration
	if state == lifecycle.StateRunning && !startedAt.IsZero() {
		uptime = time.Since(startedAt)
	}

	return agent.AgentInfo{
		ID:        cfg.ID,
		Name:      cfg.Name,
		Type:      "modbus-tcp-server",
		State:     state,
		Health:    a.Health(),
		Config:    cfg,
		Stats:     a.stats.Snapshot(),
		StartedAt: startedAt,
		Uptime:    uptime,
		CreatedAt: createdAt,
	}
}

// BufferInfo returns the pending and capacity of the message buffer.
func (a *ModbusServerAgent) BufferInfo() (int, int) {
	return len(a.msgCh), cap(a.msgCh)
}

// Stats returns a statistics snapshot.
func (a *ModbusServerAgent) Stats() agent.StatsSnapshot {
	s := a.stats.Snapshot()
	s.MsgBufferPending, s.MsgBufferCapacity = a.BufferInfo()
	s.Extra = map[string]any{
		"device_count": a.deviceManager.DeviceCount(),
	}
	return s
}

// State returns the agent's runtime state.
// Implements agent.StatefulAgent.
func (a *ModbusServerAgent) State() map[string]any {
	result := map[string]any{
		"listen_address":     a.config.ListenAddress,
		"listen_port":        a.config.ListenPort,
		"active_connections": a.listener.ActiveConnections(),
		"max_connections":    a.config.MaxConnections,
	}

	// 멀티-디바이스 정보
	devices := a.deviceManager.GetAllDevices()
	deviceInfos := make([]map[string]any, 0, len(devices))
	for _, dev := range devices {
		info := map[string]any{
			"unit_id":      dev.UnitID,
			"name":         dev.Name,
			"register_map": dev.RegisterMap.GetSnapshot(),
			"stats": map[string]any{
				"read_count":  dev.Stats.ReadCount.Load(),
				"write_count": dev.Stats.WriteCount.Load(),
				"error_count": dev.Stats.ErrorCount.Load(),
			},
		}
		deviceInfos = append(deviceInfos, info)
	}
	result["devices"] = deviceInfos
	result["device_count"] = len(devices)

	// 하위 호환: 첫 번째 디바이스의 unit_id 와 register_map
	if first := a.deviceManager.FirstDevice(); first != nil {
		result["unit_id"] = first.UnitID
		result["register_map"] = first.RegisterMap.GetSnapshot()
	}

	// register_defs 에 현재 값을 포함하여 반환
	if defs := a.buildRegisterDefsWithValues(nil); defs != nil {
		result["register_defs"] = defs
	}

	return result
}

// buildRegisterDefsWithValues 는 register_defs 를 읽어
// 각 항목에 current_value 필드를 추가하여 반환한다.
// 해석 순서:
//  1. params 에 unit_id 가 있으면 해당 디바이스의 RegisterDefs
//  2. 첫 번째 디바이스의 RegisterDefs
//  3. 최상위 Transport.Options["register_defs"] (하위 호환)
func (a *ModbusServerAgent) buildRegisterDefsWithValues(params map[string]any) []map[string]any {
	var rawDefs []any
	var targetRM *RegisterMap

	// 1. params 의 unit_id 로 디바이스별 register_defs 확인
	if params != nil {
		if uid, ok := getParamInt(params, "unit_id"); ok {
			if dev := a.deviceManager.GetDevice(byte(uid)); dev != nil && len(dev.RegisterDefs) > 0 {
				rawDefs = dev.RegisterDefs
				targetRM = dev.RegisterMap
			}
		}
	}

	// 2. 첫 번째 디바이스의 RegisterDefs
	if rawDefs == nil {
		if first := a.deviceManager.FirstDevice(); first != nil && len(first.RegisterDefs) > 0 {
			rawDefs = first.RegisterDefs
			targetRM = first.RegisterMap
		}
	}

	// 3. 최상위 register_defs (하위 호환)
	if rawDefs == nil {
		a.mu.RLock()
		top, ok := a.agentConfig.Transport.Options["register_defs"]
		a.mu.RUnlock()
		if ok {
			if items, ok := top.([]any); ok {
				rawDefs = items
			}
		}
		targetRM = a.registerMap
	}

	if rawDefs == nil {
		return nil
	}

	// targetRM 이 여전히 nil 이면 하위 호환 기본값
	if targetRM == nil {
		targetRM = a.registerMap
	}

	result := make([]map[string]any, 0, len(rawDefs))
	for _, item := range rawDefs {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		// 원본 필드를 복사
		enriched := make(map[string]any, len(m)+1)
		for k, v := range m {
			enriched[k] = v
		}

		// 레지스터 메타데이터 추출
		address := toUint16(m["address"])
		dataType, _ := m["data_type"].(string)
		byteOrder, _ := m["byte_order"].(string)
		area, _ := m["area"].(string)

		// byte_order 정규화: "big" → "big_endian", "little" → "little_endian"
		byteOrder = normalizeByteOrder(byteOrder)

		// area 기본값: 서버 에이전트의 주요 용도인 input_registers
		if area == "" {
			area = "input_registers"
		}

		// 현재 레지스터 값 읽기 (대상 디바이스의 RegisterMap 사용)
		if dataType != "" {
			value, err := targetRM.ReadTyped(area, address, dataType, byteOrder)
			if err == nil {
				enriched["current_value"] = value
			} else {
				enriched["current_value"] = nil
			}
		}

		result = append(result, enriched)
	}

	return result
}

// normalizeByteOrder 는 축약 byte_order ("big", "little") 를
// Modbus 패키지 형식 ("big_endian", "little_endian") 으로 변환한다.
func normalizeByteOrder(order string) string {
	if order == "little" {
		return "little_endian"
	}
	return "big_endian"
}

// processGetRegisterDefs 는 register_defs 에 정의된 모든 레지스터의 현재 값을 반환한다.
// params 에 unit_id 가 있으면 해당 디바이스의 register_defs 를 사용한다.
func (a *ModbusServerAgent) processGetRegisterDefs(req *processRequest) ([]byte, error) {
	defs := a.buildRegisterDefsWithValues(req.Params)
	if defs == nil {
		return json.Marshal(map[string]any{
			"ok":            false,
			"error":         "register_defs not configured",
			"register_defs": []any{},
		})
	}
	return json.Marshal(map[string]any{
		"ok":            true,
		"register_defs": defs,
	})
}

// ListenAddr returns the actual listening address (useful for tests with port 0).
func (a *ModbusServerAgent) ListenAddr() net.Addr {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.listener != nil {
		return a.listener.Addr()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Device resolution helper
// ---------------------------------------------------------------------------

// resolveRegisterMap resolves the target RegisterMap based on unit_id parameter.
// If unit_id is not specified, uses the first device's RegisterMap (backward compatibility).
func (a *ModbusServerAgent) resolveRegisterMap(params map[string]any) *RegisterMap {
	if params != nil {
		if uid, ok := getParamInt(params, "unit_id"); ok {
			dev := a.deviceManager.GetDevice(byte(uid))
			if dev != nil {
				return dev.RegisterMap
			}
		}
	}
	return a.registerMap // 첫 번째 디바이스 (하위 호환)
}

// ---------------------------------------------------------------------------
// Type resolution helper
// ---------------------------------------------------------------------------

// resolveDataType resolves the data_type for a given area and address.
// Priority: explicit param > TypeOverlay > "uint16" default
func (a *ModbusServerAgent) resolveDataType(params map[string]any, area string, address uint16) (string, string) {
	rm := a.resolveRegisterMap(params)
	return resolveDataTypeFromRM(rm, params, area, address)
}

// resolveDataTypeFromRM resolves data type using the given RegisterMap.
func resolveDataTypeFromRM(rm *RegisterMap, params map[string]any, area string, address uint16) (string, string) {
	dataType, _ := getParamString(params, "data_type")
	byteOrder, _ := getParamString(params, "byte_order")
	if byteOrder == "" {
		byteOrder = modbus.ByteOrderBigEndian
	}

	if dataType == "" {
		// TypeOverlay 확인
		overlay := rm.GetTypeOverlay()
		key := area + ":" + fmt.Sprintf("%d", address)
		if entry, ok := overlay[key]; ok {
			dataType = entry.DataType
			if entry.ByteOrder != "" {
				byteOrder = entry.ByteOrder
			}
		}
	}

	if dataType == "" {
		dataType = modbus.DataTypeUint16
	}

	return dataType, byteOrder
}

// ---------------------------------------------------------------------------
// Parameter extraction helpers
// ---------------------------------------------------------------------------

// getParamString extracts a string value from params map.
func getParamString(params map[string]any, key string) (string, bool) {
	v, ok := params[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// getParamFloat64 extracts a float64 value from params map.
// Handles both float64 and int types from JSON unmarshalling.
func getParamFloat64(params map[string]any, key string) (float64, bool) {
	v, ok := params[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

// getParamInt extracts an int value from params map.
func getParamInt(params map[string]any, key string) (int, bool) {
	v, ok := params[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

// getParamBool extracts a bool value from params map.
func getParamBool(params map[string]any, key string) (bool, bool) {
	v, ok := params[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// toParamUint16 converts an any value to uint16.
func toParamUint16(v any) (uint16, bool) {
	switch n := v.(type) {
	case int:
		return uint16(n), true
	case float64:
		return uint16(n), true
	default:
		return 0, false
	}
}
