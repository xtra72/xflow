package adapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/storage"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func ptrBool(v bool) *bool      { return &v }
func ptrStr(v string) *string   { return &v }
func ptrF32(v float32) *float32 { return &v }
func ptrU16(v uint16) *uint16   { return &v }

// fullIndoorInfo returns a NASADeviceInfo representing a typical indoor device
// with all state fields populated.
func fullIndoorInfo() NASADeviceInfo {
	return NASADeviceInfo{
		Address:       "20.01.00",
		DeviceID:      "living-room-ac",
		DeviceType:    "HVACR.IDU",
		Online:        true,
		Ready:         true,
		LastSeen:      time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC),
		ErrorCount:    2,
		Power:         ptrBool(true),
		Mode:          ptrStr("cool"),
		TargetTemp:    ptrF32(24.0),
		CurrentTemp:   ptrF32(26.5),
		FanSpeed:      ptrStr("auto"),
		SwingVertical: ptrBool(false),
		FilterAlarm:   ptrBool(false),
		ErrorCode:     ptrU16(0),
	}
}

// outdoorInfo returns a NASADeviceInfo representing an outdoor device.
func outdoorInfo() NASADeviceInfo {
	return NASADeviceInfo{
		Address:    "10.00.00",
		DeviceID:   "",
		DeviceType: "HVACR.ODU",
		Online:     true,
		Ready:      false,
		LastSeen:   time.Date(2026, 3, 12, 9, 0, 0, 0, time.UTC),
		ErrorCount: 0,
	}
}

// controllerInfo returns a NASADeviceInfo representing a controller device.
func controllerInfo() NASADeviceInfo {
	return NASADeviceInfo{
		Address:    "6A.EE.FF",
		DeviceID:   "main-controller",
		DeviceType: "controller",
		Online:     true,
		Ready:      true,
		LastSeen:   time.Date(2026, 3, 12, 8, 0, 0, 0, time.UTC),
		ErrorCount: 0,
	}
}

// ---------------------------------------------------------------------------
// Test: ID format
// ---------------------------------------------------------------------------

// TestNASADeviceAdapter_ID verifies SPEC-DEVICE-IDENTITY-001 Phase D
// (xflowd v1.0 — D-T1): Device.ID() returns the UUID v4 (same as UID()),
// not the legacy composite key ("agentName:address"). When
// DeviceIDRepository is configured, the same (agentName, localID) yields
// the same UUID across calls (idempotent).
func TestNASADeviceAdapter_ID(t *testing.T) {
	withRepository(t, storage.NewDeviceIDMemoryRepository())

	tests := []struct {
		name      string
		agentName string
		info      NASADeviceInfo
	}{
		{
			name:      "indoor device returns UUID",
			agentName: "nasa-agent",
			info:      fullIndoorInfo(),
		},
		{
			name:      "outdoor device returns UUID",
			agentName: "hvac-agent",
			info:      outdoorInfo(),
		},
		{
			name:      "controller device returns UUID",
			agentName: "nasa-agent",
			info:      controllerInfo(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewNASADevice(tt.agentName, tt.info)
			got := d.ID()
			require.NotEmpty(t, got, "ID() must return a UUID, not empty")
			assert.True(t, isUUIDv4Shape(got), "ID() must be UUID v4 shape: %q", got)
			// D-AC1: ID() and UID() return identical values.
			assert.Equal(t, d.UID(), got, "ID() must equal UID() (Phase D D-T1)")
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Name
// ---------------------------------------------------------------------------

func TestNASADeviceAdapter_Name(t *testing.T) {
	tests := []struct {
		name     string
		info     NASADeviceInfo
		wantName string
	}{
		{
			name:     "returns DeviceID when set",
			info:     fullIndoorInfo(),
			wantName: "living-room-ac",
		},
		{
			name:     "returns formatted name when DeviceID is empty",
			info:     outdoorInfo(),
			wantName: "NASA HVACR.ODU 10.00.00",
		},
		{
			name: "returns formatted name for unknown type with empty DeviceID",
			info: NASADeviceInfo{
				Address:    "FF.00.01",
				DeviceID:   "",
				DeviceType: "unknown",
			},
			wantName: "NASA unknown FF.00.01",
		},
		{
			name: "returns formatted name with overridden protocol",
			info: NASADeviceInfo{
				Address:    "11",
				DeviceID:   "",
				DeviceType: "HVACR.IDU",
				Protocol:   "lgap",
			},
			wantName: "LGAP HVACR.IDU 11",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewNASADevice("agent", tt.info)
			assert.Equal(t, tt.wantName, d.Name())
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Type mapping
// ---------------------------------------------------------------------------

func TestNASADeviceAdapter_Type(t *testing.T) {
	tests := []struct {
		name       string
		deviceType string
		wantType   device.DeviceType
	}{
		{"HVACR.IDU maps to DeviceTypeIndoor", "HVACR.IDU", device.DeviceTypeIndoor},
		{"HVACR.ODU maps to DeviceTypeOutdoor", "HVACR.ODU", device.DeviceTypeOutdoor},
		{"controller maps to DeviceTypeController", "controller", device.DeviceTypeController},
		{"unknown maps to DeviceTypeUnknown", "unknown", device.DeviceTypeUnknown},
		{"empty string maps to DeviceTypeUnknown", "", device.DeviceTypeUnknown},
		{"arbitrary string maps to DeviceTypeUnknown", "foobar", device.DeviceTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := NASADeviceInfo{DeviceType: tt.deviceType}
			d := NewNASADevice("agent", info)
			assert.Equal(t, tt.wantType, d.Type())
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Protocol
// ---------------------------------------------------------------------------

func TestNASADeviceAdapter_Protocol(t *testing.T) {
	t.Run("default protocol is nasa", func(t *testing.T) {
		d := NewNASADevice("agent", fullIndoorInfo())
		assert.Equal(t, "nasa", d.Protocol())
	})
	t.Run("protocol override", func(t *testing.T) {
		info := fullIndoorInfo()
		info.Protocol = "lgap"
		d := NewNASADevice("agent", info)
		assert.Equal(t, "lgap", d.Protocol())
	})
}

// ---------------------------------------------------------------------------
// Test: AgentName
// ---------------------------------------------------------------------------

func TestNASADeviceAdapter_AgentName(t *testing.T) {
	d := NewNASADevice("my-nasa-agent", fullIndoorInfo())
	assert.Equal(t, "my-nasa-agent", d.AgentName())
}

// ---------------------------------------------------------------------------
// Test: Online and LastSeen
// ---------------------------------------------------------------------------

func TestNASADeviceAdapter_OnlineAndLastSeen(t *testing.T) {
	info := fullIndoorInfo()
	d := NewNASADevice("agent", info)

	assert.True(t, d.Online())
	assert.Equal(t, info.LastSeen, d.LastSeen())

	// Offline device
	info.Online = false
	d2 := NewNASADevice("agent", info)
	assert.False(t, d2.Online())
}

// ---------------------------------------------------------------------------
// Test: State with populated fields
// ---------------------------------------------------------------------------

func TestNASADeviceAdapter_State_FullyPopulated(t *testing.T) {
	d := NewNASADevice("agent", fullIndoorInfo())
	state := d.State()

	assert.True(t, state.Online)
	assert.True(t, state.Ready)
	assert.Equal(t, 2, state.ErrorCount)

	// Verify all state properties from NASADeviceState fields
	props := state.Properties
	require.NotNil(t, props)

	assert.Equal(t, true, props["power"])
	assert.Equal(t, "cool", props["mode"])
	assert.InDelta(t, float32(24.0), props["target_temperature"], 0.01)
	assert.InDelta(t, float32(26.5), props["current_temperature"], 0.01)
	assert.Equal(t, "auto", props["fan_speed"])
	assert.Equal(t, false, props["swing_vertical"])
	assert.Equal(t, false, props["filter_alarm"])
	assert.Equal(t, uint16(0), props["error_code"])
}

// ---------------------------------------------------------------------------
// Test: State with nil (absent) state fields
// ---------------------------------------------------------------------------

func TestNASADeviceAdapter_State_NilFields(t *testing.T) {
	info := NASADeviceInfo{
		Address:    "10.00.00",
		DeviceType: "HVACR.ODU",
		Online:     true,
		Ready:      false,
		LastSeen:   time.Now(),
		ErrorCount: 0,
		// All state pointers remain nil
	}

	d := NewNASADevice("agent", info)
	state := d.State()

	assert.True(t, state.Online)
	assert.False(t, state.Ready)

	// Properties should be empty (or nil map) when no state fields are set
	assert.Empty(t, state.Properties)
}

// ---------------------------------------------------------------------------
// Test: State with ExtraProperties
// ---------------------------------------------------------------------------

func TestNASADeviceAdapter_State_ExtraProperties(t *testing.T) {
	info := fullIndoorInfo()
	info.ExtraProperties = map[string]any{
		"locked":    true,
		"plasma":    false,
		"zone_load": 50,
	}
	d := NewNASADevice("agent", info)
	state := d.State()
	props := state.Properties

	assert.Equal(t, true, props["locked"])
	assert.Equal(t, false, props["plasma"])
	assert.Equal(t, 50, props["zone_load"])
	// Original NASA properties should still be present
	assert.Equal(t, true, props["power"])
}

// ---------------------------------------------------------------------------
// Test: Metadata (default)
// ---------------------------------------------------------------------------

func TestNASADeviceAdapter_Metadata(t *testing.T) {
	d := NewNASADevice("agent", fullIndoorInfo())
	meta := d.Metadata()

	// Default metadata should be zero-value
	assert.Empty(t, meta.Tags)
	assert.Empty(t, meta.Location)
	assert.Empty(t, meta.Group)
	assert.Empty(t, meta.Labels)
}

// ---------------------------------------------------------------------------
// Test: Capabilities by device type
// ---------------------------------------------------------------------------

func TestNASADeviceAdapter_Capabilities(t *testing.T) {
	tests := []struct {
		name       string
		deviceType string
		wantCaps   []string
	}{
		{
			name:       "indoor device has control capabilities",
			deviceType: "HVACR.IDU",
			wantCaps:   []string{"target_temperature", "set_mode", "set_power", "set_fan_speed"},
		},
		{
			name:       "outdoor device has no capabilities",
			deviceType: "HVACR.ODU",
			wantCaps:   nil,
		},
		{
			name:       "controller device has no capabilities",
			deviceType: "controller",
			wantCaps:   nil,
		},
		{
			name:       "unknown device has no capabilities",
			deviceType: "unknown",
			wantCaps:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := NASADeviceInfo{DeviceType: tt.deviceType}
			d := NewNASADevice("agent", info)
			caps := d.Capabilities()

			if tt.wantCaps == nil {
				assert.Empty(t, caps)
			} else {
				assert.Equal(t, tt.wantCaps, caps)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Test: Compile-time interface checks
// ---------------------------------------------------------------------------

func TestNASADeviceAdapter_ImplementsDeviceInterface(t *testing.T) {
	// Compile-time check: NASADeviceAdapter must implement device.Device
	var _ device.Device = (*NASADeviceAdapter)(nil)

	// Compile-time check: NASADeviceAdapter must implement device.ControllableDevice
	var _ device.ControllableDevice = (*NASADeviceAdapter)(nil)
}

// ---------------------------------------------------------------------------
// Test: ControllableDevice - Execute
// ---------------------------------------------------------------------------

func TestNASADeviceAdapter_Execute_DelegatesToExecutor(t *testing.T) {
	called := false
	expectedResult := map[string]any{"status": "ok"}

	executor := func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
		called = true
		assert.Equal(t, "target_temperature", command)
		assert.Equal(t, map[string]any{"value": float64(24)}, params)
		return expectedResult, nil
	}

	d := NewControllableNASADevice("agent", fullIndoorInfo(), executor)
	result, err := d.Execute(context.Background(), "target_temperature", map[string]any{"value": float64(24)})

	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, expectedResult, result)
}

func TestNASADeviceAdapter_Execute_ReturnsErrorFromExecutor(t *testing.T) {
	executorErr := errors.New("device timeout")

	executor := func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
		return nil, executorErr
	}

	d := NewControllableNASADevice("agent", fullIndoorInfo(), executor)
	_, err := d.Execute(context.Background(), "set_power", nil)

	require.Error(t, err)
	assert.Equal(t, executorErr, err)
}

func TestNASADeviceAdapter_Execute_ReturnsErrNotControllable_WhenNoExecutor(t *testing.T) {
	// NewNASADevice creates a non-controllable device (no executor)
	d := NewNASADevice("agent", fullIndoorInfo())
	_, err := d.Execute(context.Background(), "set_power", nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, device.ErrNotControllable)
}

// ---------------------------------------------------------------------------
// Test: ControllableDevice - Commands
// ---------------------------------------------------------------------------

func TestNASADeviceAdapter_Commands_IndoorDevice(t *testing.T) {
	executor := func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
		return nil, nil
	}

	d := NewControllableNASADevice("agent", fullIndoorInfo(), executor)
	cmds := d.Commands()

	require.Len(t, cmds, 4)

	// Verify command names
	cmdNames := make([]string, len(cmds))
	for i, c := range cmds {
		cmdNames[i] = c.Name
	}
	assert.Contains(t, cmdNames, "target_temperature")
	assert.Contains(t, cmdNames, "set_mode")
	assert.Contains(t, cmdNames, "set_power")
	assert.Contains(t, cmdNames, "set_fan_speed")
}

func TestNASADeviceAdapter_Commands_OutdoorDevice(t *testing.T) {
	info := outdoorInfo()
	executor := func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
		return nil, nil
	}

	d := NewControllableNASADevice("agent", info, executor)
	cmds := d.Commands()

	assert.Empty(t, cmds)
}

// ---------------------------------------------------------------------------
// Test: CommandSpec params validation
// ---------------------------------------------------------------------------

func TestNASADeviceAdapter_CommandSpec_SetTemperature(t *testing.T) {
	executor := func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
		return nil, nil
	}

	d := NewControllableNASADevice("agent", fullIndoorInfo(), executor)
	cmds := d.Commands()

	// Find target_temperature command
	var tempCmd *device.CommandSpec
	for i := range cmds {
		if cmds[i].Name == "target_temperature" {
			tempCmd = &cmds[i]
			break
		}
	}

	require.NotNil(t, tempCmd, "target_temperature command must exist")
	require.Len(t, tempCmd.Params, 1)

	param := tempCmd.Params[0]
	assert.Equal(t, "target_temperature", param.Name)
	assert.Equal(t, "float", param.Type)
	assert.True(t, param.Required)
	require.NotNil(t, param.Min)
	require.NotNil(t, param.Max)
	assert.Equal(t, 16.0, *param.Min)
	assert.Equal(t, 30.0, *param.Max)
}

func TestNASADeviceAdapter_CommandSpec_SetMode(t *testing.T) {
	executor := func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
		return nil, nil
	}

	d := NewControllableNASADevice("agent", fullIndoorInfo(), executor)
	cmds := d.Commands()

	// Find set_mode command
	var modeCmd *device.CommandSpec
	for i := range cmds {
		if cmds[i].Name == "set_mode" {
			modeCmd = &cmds[i]
			break
		}
	}

	require.NotNil(t, modeCmd, "set_mode command must exist")
	require.Len(t, modeCmd.Params, 1)

	param := modeCmd.Params[0]
	assert.Equal(t, "mode", param.Name)
	assert.Equal(t, "enum", param.Type)
	assert.True(t, param.Required)
	assert.ElementsMatch(t, []string{"auto", "cool", "dry", "fan", "heat"}, param.Enum)
}

func TestNASADeviceAdapter_CommandSpec_SetPower(t *testing.T) {
	executor := func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
		return nil, nil
	}

	d := NewControllableNASADevice("agent", fullIndoorInfo(), executor)
	cmds := d.Commands()

	// Find set_power command
	var powerCmd *device.CommandSpec
	for i := range cmds {
		if cmds[i].Name == "set_power" {
			powerCmd = &cmds[i]
			break
		}
	}

	require.NotNil(t, powerCmd, "set_power command must exist")
	require.Len(t, powerCmd.Params, 1)

	param := powerCmd.Params[0]
	assert.Equal(t, "power", param.Name)
	assert.Equal(t, "bool", param.Type)
	assert.True(t, param.Required)
}

func TestNASADeviceAdapter_CommandSpec_SetFanSpeed(t *testing.T) {
	executor := func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
		return nil, nil
	}

	d := NewControllableNASADevice("agent", fullIndoorInfo(), executor)
	cmds := d.Commands()

	// Find set_fan_speed command
	var fanCmd *device.CommandSpec
	for i := range cmds {
		if cmds[i].Name == "set_fan_speed" {
			fanCmd = &cmds[i]
			break
		}
	}

	require.NotNil(t, fanCmd, "set_fan_speed command must exist")
	require.Len(t, fanCmd.Params, 1)

	param := fanCmd.Params[0]
	assert.Equal(t, "fan_speed", param.Name)
	assert.Equal(t, "enum", param.Type)
	assert.True(t, param.Required)
	assert.ElementsMatch(t, []string{"auto", "low", "medium", "high"}, param.Enum)
}

// ---------------------------------------------------------------------------
// Test: NewNASADevice vs NewControllableNASADevice
// ---------------------------------------------------------------------------

func TestNASADeviceAdapter_NonControllable_HasNilCommands(t *testing.T) {
	d := NewNASADevice("agent", fullIndoorInfo())
	cmds := d.Commands()
	assert.Empty(t, cmds)
}

func TestNASADeviceAdapter_Controllable_HasExecutor(t *testing.T) {
	executor := func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
		return map[string]any{"done": true}, nil
	}

	d := NewControllableNASADevice("agent", fullIndoorInfo(), executor)
	result, err := d.Execute(context.Background(), "set_power", map[string]any{"power": true})

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"done": true}, result)
}
