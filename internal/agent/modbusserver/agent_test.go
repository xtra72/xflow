package modbusserver

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"

	modbus "github.com/xtra/xflow/internal/agent/modbus"
)

// testAgentConfig returns a standard test configuration for the MODBUS server agent.
func testAgentConfig() agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "test-modbus-server",
		Name: "Test MODBUS Server",
		Type: "modbus-tcp-server",
		Transport: agent.TransportConfig{
			Type: "modbus-tcp-server",
			Options: map[string]any{
				"listen_address":   "127.0.0.1",
				"listen_port":      0,
				"unit_id":          1,
				"max_connections":  5,
				"idle_timeout":     "30s",
				"msg_channel_size": 64,
				"register_map": map[string]any{
					"holding_registers": map[string]any{
						"start_address": 0,
						"count":         100,
					},
					"coils": map[string]any{
						"start_address": 0,
						"count":         100,
					},
					"input_registers": map[string]any{
						"start_address": 0,
						"count":         100,
					},
					"discrete_inputs": map[string]any{
						"start_address": 0,
						"count":         100,
					},
				},
			},
		},
	}
}

// createAndStartAgent creates and starts a ModbusServerAgent, returning it and a cleanup function.
func createAndStartAgent(t *testing.T) (*ModbusServerAgent, func()) {
	t.Helper()
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)

	msa := a.(*ModbusServerAgent)

	ctx := context.Background()
	err = msa.Start(ctx)
	require.NoError(t, err)

	cleanup := func() {
		_ = msa.Stop(context.Background())
	}
	return msa, cleanup
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestNewModbusServerAgent(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	require.NotNil(t, a)

	msa := a.(*ModbusServerAgent)
	assert.Equal(t, lifecycle.StateRunning, msa.CurrentState())
	assert.Equal(t, "test-modbus-server", msa.ID())
	assert.Equal(t, "Test MODBUS Server", msa.Name())
	assert.Equal(t, "modbus-tcp-server", msa.Type())
}

func TestModbusServerAgent_StartStop(t *testing.T) {
	msa, cleanup := createAndStartAgent(t)
	defer cleanup()

	// Verify listener is active
	addr := msa.ListenAddr()
	require.NotNil(t, addr)
	assert.Contains(t, addr.String(), "127.0.0.1:")

	// Stop
	err := msa.Stop(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopped, msa.CurrentState())
}

func TestModbusServerAgent_Process_SetCoil(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_coil",
		"params": map[string]any{
			"address": 0,
			"value":   true,
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])

	// Verify the coil was actually set
	vals, err := msa.registerMap.ReadCoils(0, 1)
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, vals)
}

func TestModbusServerAgent_Process_SetCoils(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_coils",
		"params": map[string]any{
			"address": 0,
			"values":  []any{true, false, true},
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, float64(3), result["quantity"])

	vals, err := msa.registerMap.ReadCoils(0, 3)
	require.NoError(t, err)
	assert.Equal(t, []bool{true, false, true}, vals)
}

func TestModbusServerAgent_Process_SetRegister(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_register",
		"params": map[string]any{
			"address": 10,
			"value":   1234,
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])

	vals, err := msa.registerMap.ReadHoldingRegisters(10, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{1234}, vals)
}

func TestModbusServerAgent_Process_SetRegisters(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_registers",
		"params": map[string]any{
			"address": 0,
			"values":  []any{1234, 5678},
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])

	vals, err := msa.registerMap.ReadHoldingRegisters(0, 2)
	require.NoError(t, err)
	assert.Equal(t, []uint16{1234, 5678}, vals)
}

func TestModbusServerAgent_Process_SetInput_InputRegisters(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_input",
		"params": map[string]any{
			"area":    "input_registers",
			"address": 5,
			"value":   500,
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, "input_registers", result["area"])

	vals, err := msa.registerMap.ReadInputRegisters(5, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{500}, vals)
}

func TestModbusServerAgent_Process_SetInput_DiscreteInputs(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_input",
		"params": map[string]any{
			"area":    "discrete_inputs",
			"address": 3,
			"value":   true,
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, "discrete_inputs", result["area"])

	vals, err := msa.registerMap.ReadDiscreteInputs(3, 1)
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, vals)
}

func TestModbusServerAgent_Process_SetInputs_InputRegisters(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_inputs",
		"params": map[string]any{
			"area":    "input_registers",
			"address": 0,
			"values":  []any{100, 200},
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])

	vals, err := msa.registerMap.ReadInputRegisters(0, 2)
	require.NoError(t, err)
	assert.Equal(t, []uint16{100, 200}, vals)
}

func TestModbusServerAgent_Process_SetInputs_DiscreteInputs(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_inputs",
		"params": map[string]any{
			"area":    "discrete_inputs",
			"address": 0,
			"values":  []any{true, false},
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])

	vals, err := msa.registerMap.ReadDiscreteInputs(0, 2)
	require.NoError(t, err)
	assert.Equal(t, []bool{true, false}, vals)
}

func TestModbusServerAgent_Process_GetMap(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// Set a register first
	_, err = msa.registerMap.WriteHoldingRegisters(0, []uint16{42})
	require.NoError(t, err)

	data, _ := json.Marshal(map[string]any{"command": "get_map"})
	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Contains(t, result, "register_map")
}

func TestModbusServerAgent_Process_GetStatus(t *testing.T) {
	msa, cleanup := createAndStartAgent(t)
	defer cleanup()

	data, _ := json.Marshal(map[string]any{"command": "get_status"})
	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, "127.0.0.1", result["listen_address"])
	assert.Contains(t, result, "listen_port")
	assert.Contains(t, result, "active_connections")
	assert.Contains(t, result, "max_connections")
	assert.Contains(t, result, "uptime_seconds")
}

func TestModbusServerAgent_ReceiveMessage(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// Set a coil to generate a change notification via Process
	data, _ := json.Marshal(map[string]any{
		"command": "set_coil",
		"params": map[string]any{
			"address": 0,
			"value":   true,
		},
	})
	_, err = msa.Process(data)
	require.NoError(t, err)

	// Now receive the notification
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msg, err := msa.ReceiveMessage(ctx)
	require.NoError(t, err)
	require.NotNil(t, msg)

	var notification map[string]any
	require.NoError(t, json.Unmarshal(msg, &notification))
	assert.Equal(t, "register_updated", notification["type"])
	assert.Equal(t, "internal", notification["source"])
	assert.Equal(t, "set_coil", notification["command"])
	assert.Equal(t, "coils", notification["area"])
}

func TestModbusServerAgent_PauseResume(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	assert.Equal(t, lifecycle.StateRunning, msa.CurrentState())

	// Pause
	err = msa.Pause(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StatePaused, msa.CurrentState())

	// Resume
	err = msa.Resume(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, msa.CurrentState())
}

func TestModbusServerAgent_Health(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// Running -> Healthy
	h := msa.Health()
	assert.Equal(t, agent.HealthHealthy, h.Status)
	assert.Contains(t, h.Message, "running")

	// Paused -> Degraded
	err = msa.Pause(context.Background())
	require.NoError(t, err)
	h = msa.Health()
	assert.Equal(t, agent.HealthDegraded, h.Status)
	assert.Contains(t, h.Message, "paused")
}

func TestModbusServerAgent_State(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	state := msa.State()
	assert.Equal(t, "127.0.0.1", state["listen_address"])
	assert.Contains(t, state, "listen_port")
	assert.Contains(t, state, "unit_id")
	assert.Contains(t, state, "active_connections")
	assert.Contains(t, state, "max_connections")
	assert.Contains(t, state, "register_map")
}

func TestModbusServerAgent_Info(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	info := msa.Info()
	assert.Equal(t, "test-modbus-server", info.ID)
	assert.Equal(t, "Test MODBUS Server", info.Name)
	assert.Equal(t, "modbus-tcp-server", info.Type)
	assert.Equal(t, lifecycle.StateRunning, info.State)
}

func TestModbusServerAgent_Type(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	assert.Equal(t, "modbus-tcp-server", a.Type())
}

func TestModbusServerAgent_InvalidCommand(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)

	data, _ := json.Marshal(map[string]any{"command": "invalid_command"})
	_, err = a.Process(data)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidCommand))
}

func TestModbusServerAgent_Configure(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)

	newCfg := cfg
	newCfg.Name = "Updated Server"
	err = a.Configure(newCfg)
	require.NoError(t, err)
	assert.Equal(t, "Updated Server", a.Name())
}

func TestModbusServerAgent_Stats(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)

	stats := a.Stats()
	assert.Equal(t, int64(0), stats.MessagesReceived)
	assert.Equal(t, int64(0), stats.MessagesSent)
}

// ---------------------------------------------------------------------------
// TCP integration test
// ---------------------------------------------------------------------------

func TestModbusServerAgent_TCPIntegration(t *testing.T) {
	msa, cleanup := createAndStartAgent(t)
	defer cleanup()

	addr := msa.ListenAddr()
	require.NotNil(t, addr)

	// Set a holding register value first
	_, err := msa.registerMap.WriteHoldingRegisters(0, []uint16{12345})
	require.NoError(t, err)

	// Connect a TCP client
	conn, err := net.DialTimeout("tcp", addr.String(), 2*time.Second)
	require.NoError(t, err)
	defer conn.Close()

	// Build a MODBUS/TCP read holding registers request (FC03)
	// MBAP Header: TxID(2) + ProtocolID(2) + Length(2) + UnitID(1) = 7 bytes
	// PDU: FC(1) + StartAddr(2) + Quantity(2) = 5 bytes
	txID := uint16(1)
	pdu := make([]byte, 5)
	pdu[0] = modbus.FC03ReadHoldingRegisters
	binary.BigEndian.PutUint16(pdu[1:3], 0)  // start address
	binary.BigEndian.PutUint16(pdu[3:5], 1)  // quantity

	frame := make([]byte, modbus.MBAPHeaderSize+len(pdu))
	binary.BigEndian.PutUint16(frame[0:2], txID)
	binary.BigEndian.PutUint16(frame[2:4], modbus.MBAPProtocolID) // protocol ID
	binary.BigEndian.PutUint16(frame[4:6], uint16(1+len(pdu)))    // length
	frame[6] = 1                                                   // unit ID
	copy(frame[7:], pdu)

	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Write(frame)
	require.NoError(t, err)

	// Read response
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	respBuf := make([]byte, 256)
	n, err := conn.Read(respBuf)
	require.NoError(t, err)
	require.Greater(t, n, modbus.MBAPHeaderSize)

	// Parse response MBAP header
	respTxID := binary.BigEndian.Uint16(respBuf[0:2])
	assert.Equal(t, txID, respTxID)

	// Parse response PDU
	respPDU := respBuf[modbus.MBAPHeaderSize:n]
	require.Greater(t, len(respPDU), 2)
	assert.Equal(t, byte(modbus.FC03ReadHoldingRegisters), respPDU[0]) // FC
	byteCount := int(respPDU[1])
	assert.Equal(t, 2, byteCount) // 1 register = 2 bytes

	// Parse register value
	regValue := binary.BigEndian.Uint16(respPDU[2:4])
	assert.Equal(t, uint16(12345), regValue)
}

func TestModbusServerAgent_TCPWriteCoilIntegration(t *testing.T) {
	msa, cleanup := createAndStartAgent(t)
	defer cleanup()

	addr := msa.ListenAddr()
	require.NotNil(t, addr)

	// Connect a TCP client
	conn, err := net.DialTimeout("tcp", addr.String(), 2*time.Second)
	require.NoError(t, err)
	defer conn.Close()

	// Build FC05 Write Single Coil request
	txID := uint16(2)
	pdu := make([]byte, 5)
	pdu[0] = modbus.FC05WriteSingleCoil
	binary.BigEndian.PutUint16(pdu[1:3], 5)      // address 5
	binary.BigEndian.PutUint16(pdu[3:5], 0xFF00)  // ON

	frame := make([]byte, modbus.MBAPHeaderSize+len(pdu))
	binary.BigEndian.PutUint16(frame[0:2], txID)
	binary.BigEndian.PutUint16(frame[2:4], modbus.MBAPProtocolID)
	binary.BigEndian.PutUint16(frame[4:6], uint16(1+len(pdu)))
	frame[6] = 1
	copy(frame[7:], pdu)

	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Write(frame)
	require.NoError(t, err)

	// Read response
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	respBuf := make([]byte, 256)
	n, err := conn.Read(respBuf)
	require.NoError(t, err)
	require.Greater(t, n, modbus.MBAPHeaderSize)

	// Verify echo-back
	respTxID := binary.BigEndian.Uint16(respBuf[0:2])
	assert.Equal(t, txID, respTxID)

	// Verify coil was actually set
	vals, err := msa.registerMap.ReadCoils(5, 1)
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, vals)

	// Verify a change notification was sent to msgCh
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msg, err := msa.ReceiveMessage(ctx)
	require.NoError(t, err)

	var notification map[string]any
	require.NoError(t, json.Unmarshal(msg, &notification))
	assert.Equal(t, "register_change", notification["type"])
}

func TestModbusServerAgent_Process_InvalidJSON(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)

	_, err = a.Process([]byte("not-json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestModbusServerAgent_Process_SetInput_UnsupportedArea(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)

	data, _ := json.Marshal(map[string]any{
		"command": "set_input",
		"params": map[string]any{
			"area":    "unknown_area",
			"address": 0,
			"value":   true,
		},
	})

	_, err = a.Process(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported area")
}

func TestModbusServerAgent_Process_SetInputs_UnsupportedArea(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)

	data, _ := json.Marshal(map[string]any{
		"command": "set_inputs",
		"params": map[string]any{
			"area":    "unknown_area",
			"address": 0,
			"values":  []any{true},
		},
	})

	_, err = a.Process(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported area")
}

func TestModbusServerAgent_ListenAddr_BeforeStart(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// Before Start, Addr should be nil
	assert.Nil(t, msa.ListenAddr())
}

func TestModbusServerAgent_ReceiveMessage_ContextCancelled(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = msa.ReceiveMessage(ctx)
	require.Error(t, err)
}

func TestModbusServerAgent_Process_SetCoil_MissingParams(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)

	// Missing value
	data, _ := json.Marshal(map[string]any{
		"command": "set_coil",
		"params": map[string]any{
			"address": 0,
		},
	})
	_, err = a.Process(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "value")

	// Missing address
	data, _ = json.Marshal(map[string]any{
		"command": "set_coil",
		"params": map[string]any{
			"value": true,
		},
	})
	_, err = a.Process(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "address")
}

func TestModbusServerAgent_Process_SetRegister_MissingParams(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)

	// Missing value
	data, _ := json.Marshal(map[string]any{
		"command": "set_register",
		"params": map[string]any{
			"address": 0,
		},
	})
	_, err = a.Process(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "value")
}

func TestRegisterModbusServerTypes(t *testing.T) {
	mgr := agent.NewManager()
	err := RegisterModbusServerTypes(mgr)
	require.NoError(t, err)
}

func TestModbusServerAgent_NoChangeSetOnSameValue(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// Set coil to false (which is the initial value - no change)
	data, _ := json.Marshal(map[string]any{
		"command": "set_coil",
		"params": map[string]any{
			"address": 0,
			"value":   false,
		},
	})
	_, err = msa.Process(data)
	require.NoError(t, err)

	// msgCh should be empty (no change event)
	select {
	case <-msa.msgCh:
		t.Fatal("expected no message in msgCh for no-change write")
	default:
		// OK - no message expected
	}
}
