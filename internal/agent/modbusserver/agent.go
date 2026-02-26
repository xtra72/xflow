package modbusserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// Compile-time interface checks
var _ agent.Agent = (*ModbusServerAgent)(nil)
var _ agent.MessageReceiver = (*ModbusServerAgent)(nil)
var _ agent.StatefulAgent = (*ModbusServerAgent)(nil)

// processRequest is the JSON request structure for the Process method.
type processRequest struct {
	Command string         `json:"command"`
	Params  map[string]any `json:"params,omitempty"`
}

// ModbusServerAgent is a MODBUS/TCP server agent.
// It implements agent.Agent, agent.MessageReceiver, and agent.StatefulAgent.
type ModbusServerAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	config      ModbusServerConfig
	registerMap *RegisterMap
	listener    *Listener
	handler     *ModbusHandler
	reqHandler  *RequestHandler
	cancelFn    context.CancelFunc
	msgCh       chan map[string]any
	stopCh      chan struct{}
	stats       *agent.AgentStats
	logger      *slog.Logger
	mu          sync.RWMutex
	startedAt   time.Time
	createdAt   time.Time
	paused      bool
}

// NewModbusServerAgent creates a new MODBUS/TCP server agent.
func NewModbusServerAgent(agentConfig agent.AgentConfig) (agent.Agent, error) {
	cfg, err := parseModbusServerConfig(agentConfig.Transport.Options)
	if err != nil {
		return nil, fmt.Errorf("modbus-server agent: %w", err)
	}

	rm := NewRegisterMap(cfg.RegisterMap)
	msgCh := make(chan map[string]any, cfg.MsgChannelSize)
	logger := slog.Default()
	reqHandler := NewRequestHandler(rm, logger)
	handler := NewModbusHandler(cfg.UnitID, reqHandler, msgCh, logger)

	listenAddr := fmt.Sprintf("%s:%d", cfg.ListenAddress, cfg.ListenPort)
	listener := NewListener(listenAddr, cfg.MaxConnections, cfg.IdleTimeout, handler, logger)

	a := &ModbusServerAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("modbus-tcp-server")),
		config:        cfg,
		registerMap:   rm,
		listener:      listener,
		handler:       handler,
		reqHandler:    reqHandler,
		msgCh:         msgCh,
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
	if a.CurrentState() != lifecycle.StateRunning {
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
	case "get_map":
		return a.processGetMap()
	case "get_status":
		return a.processGetStatus()
	default:
		return nil, ErrInvalidCommand
	}
}

// processSetCoil sets a single coil value.
func (a *ModbusServerAgent) processSetCoil(req *processRequest) ([]byte, error) {
	addr, ok := getParamInt(req.Params, "address")
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_coil requires 'address' param")
	}
	val, ok := getParamBool(req.Params, "value")
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_coil requires 'value' param (bool)")
	}

	cs, err := a.registerMap.WriteCoils(uint16(addr), []bool{val})
	if err != nil {
		return nil, fmt.Errorf("modbus-server: set_coil: %w", err)
	}

	a.sendChangeEvent(cs, req.Command)
	return json.Marshal(map[string]any{"ok": true, "address": addr, "value": val})
}

// processSetCoils sets multiple coil values.
func (a *ModbusServerAgent) processSetCoils(req *processRequest) ([]byte, error) {
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

	cs, err := a.registerMap.WriteCoils(uint16(addr), values)
	if err != nil {
		return nil, fmt.Errorf("modbus-server: set_coils: %w", err)
	}

	a.sendChangeEvent(cs, req.Command)
	return json.Marshal(map[string]any{"ok": true, "address": addr, "quantity": len(values)})
}

// processSetRegister sets a single holding register value.
func (a *ModbusServerAgent) processSetRegister(req *processRequest) ([]byte, error) {
	addr, ok := getParamInt(req.Params, "address")
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_register requires 'address' param")
	}
	val, ok := getParamInt(req.Params, "value")
	if !ok {
		return nil, fmt.Errorf("modbus-server: set_register requires 'value' param (int)")
	}

	cs, err := a.registerMap.WriteHoldingRegisters(uint16(addr), []uint16{uint16(val)})
	if err != nil {
		return nil, fmt.Errorf("modbus-server: set_register: %w", err)
	}

	a.sendChangeEvent(cs, req.Command)
	return json.Marshal(map[string]any{"ok": true, "address": addr, "value": val})
}

// processSetRegisters sets multiple holding register values.
func (a *ModbusServerAgent) processSetRegisters(req *processRequest) ([]byte, error) {
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

	values := make([]uint16, len(arr))
	for i, v := range arr {
		n, ok := toParamUint16(v)
		if !ok {
			return nil, fmt.Errorf("modbus-server: set_registers values[%d] must be a number", i)
		}
		values[i] = n
	}

	cs, err := a.registerMap.WriteHoldingRegisters(uint16(addr), values)
	if err != nil {
		return nil, fmt.Errorf("modbus-server: set_registers: %w", err)
	}

	a.sendChangeEvent(cs, req.Command)
	return json.Marshal(map[string]any{"ok": true, "address": addr, "quantity": len(values)})
}

// processSetInput sets a single input register or discrete input value.
func (a *ModbusServerAgent) processSetInput(req *processRequest) ([]byte, error) {
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
		val, ok := getParamInt(req.Params, "value")
		if !ok {
			return nil, fmt.Errorf("modbus-server: set_input requires 'value' param (int) for input_registers")
		}
		cs, err := a.registerMap.WriteInputRegisters(uint16(addr), []uint16{uint16(val)})
		if err != nil {
			return nil, fmt.Errorf("modbus-server: set_input: %w", err)
		}
		a.sendChangeEvent(cs, req.Command)
		return json.Marshal(map[string]any{"ok": true, "area": area, "address": addr, "value": val})

	case "discrete_inputs":
		val, ok := getParamBool(req.Params, "value")
		if !ok {
			return nil, fmt.Errorf("modbus-server: set_input requires 'value' param (bool) for discrete_inputs")
		}
		cs, err := a.registerMap.WriteDiscreteInputs(uint16(addr), []bool{val})
		if err != nil {
			return nil, fmt.Errorf("modbus-server: set_input: %w", err)
		}
		a.sendChangeEvent(cs, req.Command)
		return json.Marshal(map[string]any{"ok": true, "area": area, "address": addr, "value": val})

	default:
		return nil, fmt.Errorf("modbus-server: set_input: unsupported area %q", area)
	}
}

// processSetInputs sets multiple input registers or discrete inputs.
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
		values := make([]uint16, len(arr))
		for i, v := range arr {
			n, ok := toParamUint16(v)
			if !ok {
				return nil, fmt.Errorf("modbus-server: set_inputs values[%d] must be a number", i)
			}
			values[i] = n
		}
		cs, err := a.registerMap.WriteInputRegisters(uint16(addr), values)
		if err != nil {
			return nil, fmt.Errorf("modbus-server: set_inputs: %w", err)
		}
		a.sendChangeEvent(cs, req.Command)
		return json.Marshal(map[string]any{"ok": true, "area": area, "address": addr, "quantity": len(values)})

	case "discrete_inputs":
		values := make([]bool, len(arr))
		for i, v := range arr {
			b, ok := v.(bool)
			if !ok {
				return nil, fmt.Errorf("modbus-server: set_inputs values[%d] must be bool", i)
			}
			values[i] = b
		}
		cs, err := a.registerMap.WriteDiscreteInputs(uint16(addr), values)
		if err != nil {
			return nil, fmt.Errorf("modbus-server: set_inputs: %w", err)
		}
		a.sendChangeEvent(cs, req.Command)
		return json.Marshal(map[string]any{"ok": true, "area": area, "address": addr, "quantity": len(values)})

	default:
		return nil, fmt.Errorf("modbus-server: set_inputs: unsupported area %q", area)
	}
}

// processGetMap returns the register map snapshot.
func (a *ModbusServerAgent) processGetMap() ([]byte, error) {
	snap := a.registerMap.GetSnapshot()
	return json.Marshal(map[string]any{"register_map": snap})
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

// sendChangeEvent sends a register_updated event to msgCh (non-blocking).
func (a *ModbusServerAgent) sendChangeEvent(cs *ChangeSet, command string) {
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

	select {
	case a.msgCh <- notification:
	default:
		a.logger.Warn("modbus-server: msgCh full, dropping event", "command", command)
	}
}

// ReceiveMessage receives a message from msgCh.
// Implements agent.MessageReceiver.
func (a *ModbusServerAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
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

// Stats returns a statistics snapshot.
func (a *ModbusServerAgent) Stats() agent.StatsSnapshot {
	return a.stats.Snapshot()
}

// State returns the agent's runtime state.
// Implements agent.StatefulAgent.
func (a *ModbusServerAgent) State() map[string]any {
	return map[string]any{
		"listen_address":     a.config.ListenAddress,
		"listen_port":        a.config.ListenPort,
		"unit_id":            a.config.UnitID,
		"active_connections": a.listener.ActiveConnections(),
		"max_connections":    a.config.MaxConnections,
		"register_map":       a.registerMap.GetSnapshot(),
	}
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
// Parameter extraction helpers
// ---------------------------------------------------------------------------

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
