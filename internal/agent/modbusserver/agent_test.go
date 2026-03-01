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
	modbus "github.com/xtra/xflow/internal/agent/modbus"
	"github.com/xtra/xflow/pkg/lifecycle"
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

// ---------------------------------------------------------------------------
// Typed operation tests (Milestone 4)
// ---------------------------------------------------------------------------

// testAgentConfigWithTypeMap returns a test configuration with TypeMap entries.
func testAgentConfigWithTypeMap() agent.AgentConfig {
	cfg := testAgentConfig()
	opts := cfg.Transport.Options
	rm := opts["register_map"].(map[string]any)
	rm["holding_registers"] = map[string]any{
		"start_address": 0,
		"count":         100,
		"type_map": []any{
			map[string]any{"address": 0, "data_type": "float32"},
			map[string]any{"address": 2, "data_type": "int32"},
			map[string]any{"address": 4, "data_type": "uint32"},
			map[string]any{"address": 6, "data_type": "int16"},
		},
	}
	rm["input_registers"] = map[string]any{
		"start_address": 0,
		"count":         100,
		"type_map": []any{
			map[string]any{"address": 0, "data_type": "float32"},
		},
	}
	return cfg
}

func TestModbusServerAgent_Process_SetRegister_WithDataType_Float32(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_register",
		"params": map[string]any{
			"address":   10,
			"value":     3.14,
			"data_type": "float32",
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, "float32", result["data_type"])

	// float32(3.14)를 다시 읽어서 검증
	typedVal, err := msa.registerMap.ReadTyped("holding_registers", 10, "float32", "big_endian")
	require.NoError(t, err)
	f, ok := typedVal.(float32)
	require.True(t, ok)
	assert.InDelta(t, float64(3.14), float64(f), 0.001)
}

func TestModbusServerAgent_Process_SetRegister_WithDataType_Int32(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_register",
		"params": map[string]any{
			"address":   10,
			"value":     -100000,
			"data_type": "int32",
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, "int32", result["data_type"])

	typedVal, err := msa.registerMap.ReadTyped("holding_registers", 10, "int32", "big_endian")
	require.NoError(t, err)
	assert.Equal(t, int32(-100000), typedVal)
}

func TestModbusServerAgent_Process_SetRegister_WithDataType_Uint32(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_register",
		"params": map[string]any{
			"address":   10,
			"value":     100000,
			"data_type": "uint32",
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, "uint32", result["data_type"])

	typedVal, err := msa.registerMap.ReadTyped("holding_registers", 10, "uint32", "big_endian")
	require.NoError(t, err)
	assert.Equal(t, uint32(100000), typedVal)
}

func TestModbusServerAgent_Process_SetRegister_WithDataType_Int16(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_register",
		"params": map[string]any{
			"address":   10,
			"value":     -1,
			"data_type": "int16",
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, "int16", result["data_type"])

	typedVal, err := msa.registerMap.ReadTyped("holding_registers", 10, "int16", "big_endian")
	require.NoError(t, err)
	assert.Equal(t, int16(-1), typedVal)
}

func TestModbusServerAgent_Process_SetRegister_BackwardCompatible(t *testing.T) {
	// 기존 uint16 동작이 data_type 없이도 유지되는지 확인
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
	// data_type 필드가 없어야 함
	_, hasDataType := result["data_type"]
	assert.False(t, hasDataType, "backward compatible response should not have data_type field")

	vals, err := msa.registerMap.ReadHoldingRegisters(10, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{1234}, vals)
}

func TestModbusServerAgent_Process_SetRegister_TypeOverlayDefault(t *testing.T) {
	// TypeMap이 있을 때 data_type 파라미터 없이도 TypeOverlay에서 타입을 가져오는지 확인
	cfg := testAgentConfigWithTypeMap()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// address 0 은 TypeMap에서 float32로 지정됨
	data, _ := json.Marshal(map[string]any{
		"command": "set_register",
		"params": map[string]any{
			"address": 0,
			"value":   2.5,
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, "float32", result["data_type"])

	// 읽기 검증
	typedVal, err := msa.registerMap.ReadTyped("holding_registers", 0, "float32", "big_endian")
	require.NoError(t, err)
	f, ok := typedVal.(float32)
	require.True(t, ok)
	assert.InDelta(t, 2.5, float64(f), 0.001)
}

func TestModbusServerAgent_Process_SetRegisters_WithDataType(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_registers",
		"params": map[string]any{
			"address":   10,
			"values":    []any{3.14, -2.71},
			"data_type": "float32",
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, "float32", result["data_type"])
	assert.Equal(t, float64(2), result["quantity"])

	// float32는 각 2 레지스터 → 총 4 레지스터 기록됨
	// 첫 번째 값 검증
	typedVal, err := msa.registerMap.ReadTyped("holding_registers", 10, "float32", "big_endian")
	require.NoError(t, err)
	f, ok := typedVal.(float32)
	require.True(t, ok)
	assert.InDelta(t, 3.14, float64(f), 0.001)

	// 두 번째 값 검증 (address 12)
	typedVal2, err := msa.registerMap.ReadTyped("holding_registers", 12, "float32", "big_endian")
	require.NoError(t, err)
	f2, ok := typedVal2.(float32)
	require.True(t, ok)
	assert.InDelta(t, -2.71, float64(f2), 0.01)
}

func TestModbusServerAgent_Process_SetInput_WithDataType(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_input",
		"params": map[string]any{
			"area":      "input_registers",
			"address":   10,
			"value":     1.5,
			"data_type": "float32",
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, "input_registers", result["area"])
	assert.Equal(t, "float32", result["data_type"])

	typedVal, err := msa.registerMap.ReadTyped("input_registers", 10, "float32", "big_endian")
	require.NoError(t, err)
	f, ok := typedVal.(float32)
	require.True(t, ok)
	assert.InDelta(t, 1.5, float64(f), 0.001)
}

func TestModbusServerAgent_Process_SetInputs_WithDataType(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_inputs",
		"params": map[string]any{
			"area":      "input_registers",
			"address":   10,
			"values":    []any{3.14, -2.71},
			"data_type": "float32",
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, "input_registers", result["area"])
	assert.Equal(t, "float32", result["data_type"])
	assert.Equal(t, float64(2), result["quantity"])

	// 첫 번째 float32 검증
	typedVal, err := msa.registerMap.ReadTyped("input_registers", 10, "float32", "big_endian")
	require.NoError(t, err)
	f, ok := typedVal.(float32)
	require.True(t, ok)
	assert.InDelta(t, 3.14, float64(f), 0.001)

	// 두 번째 float32 검증 (address 12)
	typedVal2, err := msa.registerMap.ReadTyped("input_registers", 12, "float32", "big_endian")
	require.NoError(t, err)
	f2, ok := typedVal2.(float32)
	require.True(t, ok)
	assert.InDelta(t, -2.71, float64(f2), 0.01)
}

func TestModbusServerAgent_Process_GetRegisterTyped(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// float32 값을 먼저 기록
	_, err = msa.registerMap.WriteTyped("holding_registers", 10, float64(3.14), "float32", "big_endian")
	require.NoError(t, err)

	data, _ := json.Marshal(map[string]any{
		"command": "get_register_typed",
		"params": map[string]any{
			"address":   10,
			"area":      "holding_registers",
			"data_type": "float32",
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, float64(10), result["address"])
	assert.Equal(t, "float32", result["data_type"])
	assert.Equal(t, "holding_registers", result["area"])

	// value가 float32(3.14)에 가까운지 확인 (JSON은 float64로 디코딩)
	val, ok := result["value"].(float64)
	require.True(t, ok)
	assert.InDelta(t, 3.14, val, 0.001)
}

func TestModbusServerAgent_Process_GetRegisterTyped_DefaultOverlay(t *testing.T) {
	// TypeMap이 있을 때 data_type 파라미터 없이도 TypeOverlay에서 타입을 가져오는지 확인
	cfg := testAgentConfigWithTypeMap()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// address 0은 TypeMap에서 float32로 지정됨 → 먼저 값을 기록
	_, err = msa.registerMap.WriteTyped("holding_registers", 0, float64(42.5), "float32", "big_endian")
	require.NoError(t, err)

	data, _ := json.Marshal(map[string]any{
		"command": "get_register_typed",
		"params": map[string]any{
			"address": 0,
			"area":    "holding_registers",
			// data_type 없음 → TypeOverlay에서 float32 참조
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, "float32", result["data_type"])

	val, ok := result["value"].(float64)
	require.True(t, ok)
	assert.InDelta(t, 42.5, val, 0.001)
}

func TestModbusServerAgent_Process_GetMap_WithTypeOverlay(t *testing.T) {
	cfg := testAgentConfigWithTypeMap()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{"command": "get_map"})
	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Contains(t, result, "register_map")
	assert.Contains(t, result, "type_overlay")

	overlay, ok := result["type_overlay"].(map[string]any)
	require.True(t, ok)
	assert.NotEmpty(t, overlay)
}

func TestModbusServerAgent_Process_GetMap_WithoutTypeOverlay(t *testing.T) {
	// TypeMap이 없는 기본 설정에서는 type_overlay가 응답에 없어야 함
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{"command": "get_map"})
	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Contains(t, result, "register_map")
	_, hasOverlay := result["type_overlay"]
	assert.False(t, hasOverlay, "response should not contain type_overlay when no TypeMap is configured")
}

func TestModbusServerAgent_Process_ChangeEvent_WithDataType(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// float32 타입으로 레지스터 설정
	data, _ := json.Marshal(map[string]any{
		"command": "set_register",
		"params": map[string]any{
			"address":   10,
			"value":     3.14,
			"data_type": "float32",
		},
	})
	_, err = msa.Process(data)
	require.NoError(t, err)

	// 변경 이벤트 수신
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msg, err := msa.ReceiveMessage(ctx)
	require.NoError(t, err)

	var notification map[string]any
	require.NoError(t, json.Unmarshal(msg, &notification))
	assert.Equal(t, "register_updated", notification["type"])
	assert.Equal(t, "set_register", notification["command"])
	assert.Equal(t, "float32", notification["data_type"])
}

func TestModbusServerAgent_Process_ChangeEvent_WithoutDataType(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// 기본 uint16으로 레지스터 설정 (data_type 없음)
	data, _ := json.Marshal(map[string]any{
		"command": "set_register",
		"params": map[string]any{
			"address": 10,
			"value":   1234,
		},
	})
	_, err = msa.Process(data)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msg, err := msa.ReceiveMessage(ctx)
	require.NoError(t, err)

	var notification map[string]any
	require.NoError(t, json.Unmarshal(msg, &notification))
	assert.Equal(t, "register_updated", notification["type"])
	_, hasDataType := notification["data_type"]
	assert.False(t, hasDataType, "change event should not have data_type field for uint16 writes")
}

func TestModbusServerAgent_Process_SetInput_TypeOverlayDefault(t *testing.T) {
	// input_registers의 TypeMap에서 address 0이 float32로 지정됨
	cfg := testAgentConfigWithTypeMap()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_input",
		"params": map[string]any{
			"area":    "input_registers",
			"address": 0,
			"value":   7.5,
			// data_type 없음 → TypeOverlay에서 float32 참조
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, "float32", result["data_type"])

	typedVal, err := msa.registerMap.ReadTyped("input_registers", 0, "float32", "big_endian")
	require.NoError(t, err)
	f, ok := typedVal.(float32)
	require.True(t, ok)
	assert.InDelta(t, 7.5, float64(f), 0.001)
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestGetParamString(t *testing.T) {
	params := map[string]any{
		"key1": "value1",
		"key2": 123,
	}

	s, ok := getParamString(params, "key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", s)

	_, ok = getParamString(params, "key2")
	assert.False(t, ok)

	_, ok = getParamString(params, "missing")
	assert.False(t, ok)
}

func TestGetParamFloat64(t *testing.T) {
	params := map[string]any{
		"float":   3.14,
		"int":     42,
		"string":  "not a number",
	}

	f, ok := getParamFloat64(params, "float")
	assert.True(t, ok)
	assert.InDelta(t, 3.14, f, 0.001)

	f, ok = getParamFloat64(params, "int")
	assert.True(t, ok)
	assert.Equal(t, float64(42), f)

	_, ok = getParamFloat64(params, "string")
	assert.False(t, ok)

	_, ok = getParamFloat64(params, "missing")
	assert.False(t, ok)
}

func TestResolveDataType(t *testing.T) {
	// TypeMap이 있는 설정
	cfg := testAgentConfigWithTypeMap()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// 명시적 data_type 파라미터 우선
	params := map[string]any{"data_type": "int32", "byte_order": "little_endian"}
	dt, bo := msa.resolveDataType(params, "holding_registers", 0)
	assert.Equal(t, "int32", dt)
	assert.Equal(t, "little_endian", bo)

	// data_type 파라미터 없음 → TypeOverlay에서 가져옴 (address 0 = float32)
	params2 := map[string]any{}
	dt2, bo2 := msa.resolveDataType(params2, "holding_registers", 0)
	assert.Equal(t, "float32", dt2)
	assert.Equal(t, "big_endian", bo2)

	// data_type 파라미터 없음, TypeOverlay 없음 → uint16 기본값
	params3 := map[string]any{}
	dt3, bo3 := msa.resolveDataType(params3, "holding_registers", 50)
	assert.Equal(t, "uint16", dt3)
	assert.Equal(t, "big_endian", bo3)
}

func TestModbusServerAgent_Process_GetRegisterTyped_InputRegisters(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// input_registers에 int32 값 기록
	_, err = msa.registerMap.WriteTyped("input_registers", 5, float64(-50000), "int32", "big_endian")
	require.NoError(t, err)

	data, _ := json.Marshal(map[string]any{
		"command": "get_register_typed",
		"params": map[string]any{
			"address":   5,
			"area":      "input_registers",
			"data_type": "int32",
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, "int32", result["data_type"])
	assert.Equal(t, "input_registers", result["area"])

	// JSON은 int32를 float64로 디코딩
	val, ok := result["value"].(float64)
	require.True(t, ok)
	assert.Equal(t, float64(-50000), val)
}

func TestModbusServerAgent_Process_SetRegister_Float32_IntegerValue(t *testing.T) {
	// REQ-M4-07: JSON에서 정수로 전달된 float32 값이 올바르게 변환되는지 확인
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	data, _ := json.Marshal(map[string]any{
		"command": "set_register",
		"params": map[string]any{
			"address":   10,
			"value":     42,
			"data_type": "float32",
		},
	})

	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])

	typedVal, err := msa.registerMap.ReadTyped("holding_registers", 10, "float32", "big_endian")
	require.NoError(t, err)
	f, ok := typedVal.(float32)
	require.True(t, ok)
	assert.InDelta(t, 42.0, float64(f), 0.001)
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

func TestModbusServerAgent_Process_GetCoils(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// 코일 설정: address 0=true, 1=false, 2=true
	_, err = msa.registerMap.WriteCoils(0, []bool{true, false, true})
	require.NoError(t, err)

	// get_coils 커맨드 실행
	data, _ := json.Marshal(map[string]any{
		"command": "get_coils",
		"params": map[string]any{
			"address":  0,
			"quantity": 3,
		},
	})
	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, float64(0), result["address"])
	assert.Equal(t, float64(3), result["quantity"])

	values, ok := result["values"].([]any)
	require.True(t, ok, "values should be an array")
	require.Len(t, values, 3)
	assert.Equal(t, true, values[0])
	assert.Equal(t, false, values[1])
	assert.Equal(t, true, values[2])
}

func TestModbusServerAgent_Process_GetCoils_MissingParams(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// address 파라미터 누락
	data, _ := json.Marshal(map[string]any{
		"command": "get_coils",
		"params":  map[string]any{"quantity": 1},
	})
	_, err = msa.Process(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "address")

	// quantity 파라미터 누락
	data, _ = json.Marshal(map[string]any{
		"command": "get_coils",
		"params":  map[string]any{"address": 0},
	})
	_, err = msa.Process(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "quantity")
}

func TestModbusServerAgent_Process_GetDiscreteInputs(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// 디스크리트 입력 설정
	_, err = msa.registerMap.WriteDiscreteInputs(0, []bool{false, true, true, false})
	require.NoError(t, err)

	data, _ := json.Marshal(map[string]any{
		"command": "get_discrete_inputs",
		"params": map[string]any{
			"address":  0,
			"quantity": 4,
		},
	})
	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, float64(0), result["address"])
	assert.Equal(t, float64(4), result["quantity"])

	values, ok := result["values"].([]any)
	require.True(t, ok)
	require.Len(t, values, 4)
	assert.Equal(t, false, values[0])
	assert.Equal(t, true, values[1])
	assert.Equal(t, true, values[2])
	assert.Equal(t, false, values[3])
}

func TestModbusServerAgent_Process_GetHoldingRegisters(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// 홀딩 레지스터 설정
	_, err = msa.registerMap.WriteHoldingRegisters(0, []uint16{100, 200, 300, 400, 500})
	require.NoError(t, err)

	// 일부 범위만 읽기 (address=1, quantity=3 → [200, 300, 400])
	data, _ := json.Marshal(map[string]any{
		"command": "get_holding_registers",
		"params": map[string]any{
			"address":  1,
			"quantity": 3,
		},
	})
	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, float64(1), result["address"])
	assert.Equal(t, float64(3), result["quantity"])

	values, ok := result["values"].([]any)
	require.True(t, ok)
	require.Len(t, values, 3)
	assert.Equal(t, float64(200), values[0])
	assert.Equal(t, float64(300), values[1])
	assert.Equal(t, float64(400), values[2])
}

func TestModbusServerAgent_Process_GetInputRegisters(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// 입력 레지스터 설정
	_, err = msa.registerMap.WriteInputRegisters(0, []uint16{1000, 2000, 3000})
	require.NoError(t, err)

	data, _ := json.Marshal(map[string]any{
		"command": "get_input_registers",
		"params": map[string]any{
			"address":  0,
			"quantity": 3,
		},
	})
	resp, err := msa.Process(data)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(resp, &result))
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, float64(0), result["address"])
	assert.Equal(t, float64(3), result["quantity"])

	values, ok := result["values"].([]any)
	require.True(t, ok)
	require.Len(t, values, 3)
	assert.Equal(t, float64(1000), values[0])
	assert.Equal(t, float64(2000), values[1])
	assert.Equal(t, float64(3000), values[2])
}

func TestModbusServerAgent_Process_GetHoldingRegisters_OutOfRange(t *testing.T) {
	cfg := testAgentConfig()
	a, err := NewModbusServerAgent(cfg)
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// 범위를 벗어나는 요청
	data, _ := json.Marshal(map[string]any{
		"command": "get_holding_registers",
		"params": map[string]any{
			"address":  9999,
			"quantity": 10,
		},
	})
	_, err = msa.Process(data)
	assert.Error(t, err)
}
