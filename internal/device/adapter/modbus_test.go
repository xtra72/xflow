package adapter

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/storage"
)

// Compile-time interface checks
var _ device.Device = (*ModbusDeviceAdapter)(nil)
var _ device.ControllableDevice = (*ModbusDeviceAdapter)(nil)

// TestNewModbusDevice_IDFormat verifies SPEC-DEVICE-IDENTITY-001 Phase D
// (xflowd v1.0 — D-T1): Device.ID() returns the UUID v4 (same as UID()),
// not the legacy composite key. When DeviceIDRepository is configured, the
// same (agentName, localID) yields the same UUID across calls.
func TestNewModbusDevice_IDFormat(t *testing.T) {
	withRepository(t, storage.NewDeviceIDMemoryRepository())

	tests := []struct {
		name      string
		agentName string
		deviceID  string
	}{
		{
			name:      "standard ID format",
			agentName: "modbus-agent",
			deviceID:  "1",
		},
		{
			name:      "named device ID",
			agentName: "modbus-plc",
			deviceID:  "sensor-01",
		},
		{
			name:      "numeric agent name",
			agentName: "modbus-1",
			deviceID:  "42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ModbusDeviceInfo{
				DeviceID: tt.deviceID,
				Host:     "192.168.1.1",
				Port:     502,
				UnitID:   1,
				Online:   true,
			}
			dev := NewModbusDevice(tt.agentName, info)
			got := dev.ID()
			require.NotEmpty(t, got, "ID() must return a UUID, not empty")
			assert.True(t, isUUIDv4Shape(got), "ID() must be UUID v4 shape: %q", got)
			// D-AC1: ID() and UID() return identical values.
			assert.Equal(t, dev.UID(), got, "ID() must equal UID() (Phase D D-T1)")
		})
	}
}

func TestModbusDeviceAdapter_Name(t *testing.T) {
	tests := []struct {
		name     string
		deviceID string
		wantName string
	}{
		{
			name:     "numeric device ID gets formatted name",
			deviceID: "1",
			wantName: "Modbus Device 1",
		},
		{
			name:     "string device ID gets formatted name",
			deviceID: "sensor-01",
			wantName: "Modbus Device sensor-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ModbusDeviceInfo{
				DeviceID: tt.deviceID,
				Host:     "10.0.0.1",
				Port:     502,
				UnitID:   1,
			}
			dev := NewModbusDevice("modbus-agent", info)
			assert.Equal(t, tt.wantName, dev.Name())
		})
	}
}

func TestModbusDeviceAdapter_Type(t *testing.T) {
	info := ModbusDeviceInfo{
		DeviceID: "1",
		Host:     "192.168.1.1",
		Port:     502,
		UnitID:   1,
	}
	dev := NewModbusDevice("modbus-agent", info)
	assert.Equal(t, device.DeviceTypeSensor, dev.Type())
}

func TestModbusDeviceAdapter_Protocol(t *testing.T) {
	info := ModbusDeviceInfo{
		DeviceID: "1",
		Host:     "192.168.1.1",
		Port:     502,
		UnitID:   1,
	}
	dev := NewModbusDevice("modbus-agent", info)
	assert.Equal(t, "modbus", dev.Protocol())
}

func TestModbusDeviceAdapter_AgentName(t *testing.T) {
	info := ModbusDeviceInfo{
		DeviceID: "1",
		Host:     "10.0.0.1",
		Port:     502,
		UnitID:   1,
	}
	dev := NewModbusDevice("my-modbus-agent", info)
	assert.Equal(t, "my-modbus-agent", dev.AgentName())
}

func TestModbusDeviceAdapter_Online(t *testing.T) {
	tests := []struct {
		name   string
		online bool
	}{
		{"online device returns true", true},
		{"offline device returns false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ModbusDeviceInfo{
				DeviceID: "1",
				Host:     "10.0.0.1",
				Port:     502,
				UnitID:   1,
				Online:   tt.online,
			}
			dev := NewModbusDevice("modbus-agent", info)
			assert.Equal(t, tt.online, dev.Online())
		})
	}
}

func TestModbusDeviceAdapter_LastSeen(t *testing.T) {
	now := time.Now()
	info := ModbusDeviceInfo{
		DeviceID: "1",
		Host:     "10.0.0.1",
		Port:     502,
		UnitID:   1,
		LastSeen: now,
	}
	dev := NewModbusDevice("modbus-agent", info)
	assert.Equal(t, now, dev.LastSeen())
}

func TestModbusDeviceAdapter_State(t *testing.T) {
	now := time.Now()

	t.Run("state contains host port and unit_id", func(t *testing.T) {
		info := ModbusDeviceInfo{
			DeviceID: "1",
			Host:     "192.168.1.100",
			Port:     502,
			UnitID:   3,
			Online:   true,
			LastSeen: now,
		}
		dev := NewModbusDevice("modbus-agent", info)
		state := dev.State()

		assert.True(t, state.Online)
		assert.Equal(t, now, state.LastSeen)
		assert.Equal(t, "192.168.1.100", state.Properties["host"])
		assert.Equal(t, 502, state.Properties["port"])
		assert.Equal(t, byte(3), state.Properties["unit_id"])
	})

	t.Run("state includes cache data in properties", func(t *testing.T) {
		info := ModbusDeviceInfo{
			DeviceID: "1",
			Host:     "10.0.0.1",
			Port:     502,
			UnitID:   1,
			Online:   true,
			LastSeen: now,
			CacheData: map[string]any{
				"temperature": 25.5,
				"humidity":    60,
				"coil_0":      true,
			},
		}
		dev := NewModbusDevice("modbus-agent", info)
		state := dev.State()

		assert.Equal(t, 25.5, state.Properties["temperature"])
		assert.Equal(t, 60, state.Properties["humidity"])
		assert.Equal(t, true, state.Properties["coil_0"])
		// Also verify base properties are still present
		assert.Equal(t, "10.0.0.1", state.Properties["host"])
		assert.Equal(t, 502, state.Properties["port"])
	})

	t.Run("state for offline device", func(t *testing.T) {
		info := ModbusDeviceInfo{
			DeviceID: "1",
			Host:     "10.0.0.1",
			Port:     502,
			UnitID:   1,
			Online:   false,
			LastSeen: now,
		}
		dev := NewModbusDevice("modbus-agent", info)
		state := dev.State()

		assert.False(t, state.Online)
	})
}

func TestModbusDeviceAdapter_Metadata(t *testing.T) {
	meta := device.DeviceMetadata{
		Tags:     []string{"factory", "sensor"},
		Location: "Line A",
		Group:    "sensors",
		Labels:   map[string]string{"type": "temperature"},
	}
	info := ModbusDeviceInfo{
		DeviceID: "1",
		Host:     "10.0.0.1",
		Port:     502,
		UnitID:   1,
	}
	dev := NewModbusDevice("modbus-agent", info)
	dev.metadata = meta

	result := dev.Metadata()
	assert.Equal(t, meta, result)
}

func TestModbusDeviceAdapter_Capabilities_ReadOnly(t *testing.T) {
	info := ModbusDeviceInfo{
		DeviceID:       "1",
		Host:           "10.0.0.1",
		Port:           502,
		UnitID:         1,
		RegisterGroups: []string{"holding_registers", "input_registers"},
		Writable:       false,
	}
	dev := NewModbusDevice("modbus-agent", info)
	caps := dev.Capabilities()

	assert.Contains(t, caps, "read_registers")
	assert.Contains(t, caps, "holding_registers")
	assert.Contains(t, caps, "input_registers")
	assert.NotContains(t, caps, "write_register")
	assert.NotContains(t, caps, "write_coil")
}

func TestModbusDeviceAdapter_Capabilities_Writable(t *testing.T) {
	info := ModbusDeviceInfo{
		DeviceID:       "1",
		Host:           "10.0.0.1",
		Port:           502,
		UnitID:         1,
		RegisterGroups: []string{"holding_registers"},
		Writable:       true,
	}
	dev := NewControllableModbusDevice("modbus-agent", info, func(ctx context.Context, cmd string, params map[string]any) (map[string]any, error) {
		return nil, nil
	})
	caps := dev.Capabilities()

	assert.Contains(t, caps, "read_registers")
	assert.Contains(t, caps, "holding_registers")
	assert.Contains(t, caps, "write_register")
	assert.Contains(t, caps, "write_coil")
}

func TestNewControllableModbusDevice_ImplementsControllableDevice(t *testing.T) {
	info := ModbusDeviceInfo{
		DeviceID: "1",
		Host:     "10.0.0.1",
		Port:     502,
		UnitID:   1,
		Writable: true,
	}
	executor := func(ctx context.Context, cmd string, params map[string]any) (map[string]any, error) {
		return map[string]any{"status": "ok"}, nil
	}
	dev := NewControllableModbusDevice("modbus-agent", info, executor)

	// Verify it satisfies ControllableDevice interface
	var cd device.ControllableDevice = dev
	assert.NotNil(t, cd)
}

func TestModbusDeviceAdapter_Execute_Delegates(t *testing.T) {
	var calledCmd string
	var calledParams map[string]any

	info := ModbusDeviceInfo{
		DeviceID: "1",
		Host:     "10.0.0.1",
		Port:     502,
		UnitID:   1,
		Writable: true,
	}
	executor := func(ctx context.Context, cmd string, params map[string]any) (map[string]any, error) {
		calledCmd = cmd
		calledParams = params
		return map[string]any{"status": "ok"}, nil
	}
	dev := NewControllableModbusDevice("modbus-agent", info, executor)

	params := map[string]any{"address": 100, "value": 42}
	result, err := dev.Execute(context.Background(), "write_register", params)

	require.NoError(t, err)
	assert.Equal(t, "write_register", calledCmd)
	assert.Equal(t, params, calledParams)
	assert.Equal(t, "ok", result["status"])
}

func TestModbusDeviceAdapter_Execute_ReadOnly_ReturnsError(t *testing.T) {
	info := ModbusDeviceInfo{
		DeviceID: "1",
		Host:     "10.0.0.1",
		Port:     502,
		UnitID:   1,
		Writable: false,
	}
	dev := NewModbusDevice("modbus-agent", info)

	_, err := dev.Execute(context.Background(), "write_register", nil)
	assert.ErrorIs(t, err, device.ErrNotControllable)
}

func TestModbusDeviceAdapter_Execute_ErrorPropagation(t *testing.T) {
	info := ModbusDeviceInfo{
		DeviceID: "1",
		Host:     "10.0.0.1",
		Port:     502,
		UnitID:   1,
		Writable: true,
	}
	expectedErr := fmt.Errorf("write failed")
	executor := func(ctx context.Context, cmd string, params map[string]any) (map[string]any, error) {
		return nil, expectedErr
	}
	dev := NewControllableModbusDevice("modbus-agent", info, executor)

	_, err := dev.Execute(context.Background(), "write_register", nil)
	assert.ErrorIs(t, err, expectedErr)
}

func TestModbusDeviceAdapter_Commands_Writable(t *testing.T) {
	info := ModbusDeviceInfo{
		DeviceID: "1",
		Host:     "10.0.0.1",
		Port:     502,
		UnitID:   1,
		Writable: true,
	}
	executor := func(ctx context.Context, cmd string, params map[string]any) (map[string]any, error) {
		return nil, nil
	}
	dev := NewControllableModbusDevice("modbus-agent", info, executor)

	cmds := dev.Commands()

	// Should have write_register and write_coil commands
	require.Len(t, cmds, 2)

	// Find write_register command
	var writeRegCmd, writeCoilCmd device.CommandSpec
	for _, cmd := range cmds {
		switch cmd.Name {
		case "write_register":
			writeRegCmd = cmd
		case "write_coil":
			writeCoilCmd = cmd
		}
	}

	// Verify write_register command
	assert.Equal(t, "write_register", writeRegCmd.Name)
	assert.NotEmpty(t, writeRegCmd.Description)
	require.Len(t, writeRegCmd.Params, 2)

	addrParam := writeRegCmd.Params[0]
	assert.Equal(t, "address", addrParam.Name)
	assert.Equal(t, "int", addrParam.Type)
	assert.True(t, addrParam.Required)

	valueParam := writeRegCmd.Params[1]
	assert.Equal(t, "value", valueParam.Name)
	assert.Equal(t, "int", valueParam.Type)
	assert.True(t, valueParam.Required)

	// Verify write_coil command
	assert.Equal(t, "write_coil", writeCoilCmd.Name)
	assert.NotEmpty(t, writeCoilCmd.Description)
	require.Len(t, writeCoilCmd.Params, 2)

	coilAddrParam := writeCoilCmd.Params[0]
	assert.Equal(t, "address", coilAddrParam.Name)
	assert.Equal(t, "int", coilAddrParam.Type)
	assert.True(t, coilAddrParam.Required)

	coilValueParam := writeCoilCmd.Params[1]
	assert.Equal(t, "value", coilValueParam.Name)
	assert.Equal(t, "bool", coilValueParam.Type)
	assert.True(t, coilValueParam.Required)
}

func TestModbusDeviceAdapter_Commands_ReadOnly(t *testing.T) {
	info := ModbusDeviceInfo{
		DeviceID: "1",
		Host:     "10.0.0.1",
		Port:     502,
		UnitID:   1,
		Writable: false,
	}
	dev := NewModbusDevice("modbus-agent", info)

	cmds := dev.Commands()
	assert.Empty(t, cmds)
}
