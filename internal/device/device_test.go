package device

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockDevice implements Device interface for testing purposes.
type mockDevice struct {
	id           string
	name         string
	deviceType   DeviceType
	protocol     string
	agentName    string
	online       bool
	lastSeen     time.Time
	state        DeviceState
	metadata     DeviceMetadata
	capabilities []string
}

func (m *mockDevice) ID() string              { return m.id }
func (m *mockDevice) Name() string            { return m.name }
func (m *mockDevice) Type() DeviceType        { return m.deviceType }
func (m *mockDevice) Protocol() string        { return m.protocol }
func (m *mockDevice) AgentName() string       { return m.agentName }
func (m *mockDevice) Online() bool            { return m.online }
func (m *mockDevice) LastSeen() time.Time     { return m.lastSeen }
func (m *mockDevice) State() DeviceState      { return m.state }
func (m *mockDevice) Metadata() DeviceMetadata { return m.metadata }
func (m *mockDevice) Capabilities() []string  { return m.capabilities }

func TestDeviceTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		dt       DeviceType
		expected string
	}{
		{"indoor type", DeviceTypeIndoor, "indoor"},
		{"outdoor type", DeviceTypeOutdoor, "outdoor"},
		{"controller type", DeviceTypeController, "controller"},
		{"sensor type", DeviceTypeSensor, "sensor"},
		{"actuator type", DeviceTypeActuator, "actuator"},
		{"unknown type", DeviceTypeUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, DeviceType(tt.expected), tt.dt)
		})
	}
}

func TestDeviceInterface(t *testing.T) {
	now := time.Now()
	dev := &mockDevice{
		id:         "nasa-agent:20.01.00",
		name:       "Lobby Indoor Unit",
		deviceType: DeviceTypeIndoor,
		protocol:   "nasa",
		agentName:  "nasa-agent",
		online:     true,
		lastSeen:   now,
		state: DeviceState{
			Online:     true,
			Ready:      true,
			LastSeen:   now,
			ErrorCount: 0,
			Properties: map[string]any{
				"power":        true,
				"target_temp":  24.0,
				"current_temp": 25.2,
			},
		},
		metadata: DeviceMetadata{
			Tags:     []string{"hvac", "lobby"},
			Location: "1F Lobby",
			Group:    "1F",
			Labels:   map[string]string{"zone": "public"},
		},
		capabilities: []string{"set_temperature", "set_mode", "set_power"},
	}

	// Verify the mock satisfies the Device interface.
	var _ Device = dev

	t.Run("ID returns global unique ID", func(t *testing.T) {
		assert.Equal(t, "nasa-agent:20.01.00", dev.ID())
	})

	t.Run("Name returns user-defined name", func(t *testing.T) {
		assert.Equal(t, "Lobby Indoor Unit", dev.Name())
	})

	t.Run("Type returns device type", func(t *testing.T) {
		assert.Equal(t, DeviceTypeIndoor, dev.Type())
	})

	t.Run("Protocol returns protocol name", func(t *testing.T) {
		assert.Equal(t, "nasa", dev.Protocol())
	})

	t.Run("AgentName returns owning agent name", func(t *testing.T) {
		assert.Equal(t, "nasa-agent", dev.AgentName())
	})

	t.Run("Online returns true when device is online", func(t *testing.T) {
		assert.True(t, dev.Online())
	})

	t.Run("LastSeen returns last communication time", func(t *testing.T) {
		assert.Equal(t, now, dev.LastSeen())
	})

	t.Run("State returns current device state", func(t *testing.T) {
		state := dev.State()
		assert.True(t, state.Online)
		assert.True(t, state.Ready)
		assert.Equal(t, 0, state.ErrorCount)
		assert.Equal(t, true, state.Properties["power"])
		assert.Equal(t, 24.0, state.Properties["target_temp"])
	})

	t.Run("Metadata returns device metadata", func(t *testing.T) {
		meta := dev.Metadata()
		assert.Equal(t, []string{"hvac", "lobby"}, meta.Tags)
		assert.Equal(t, "1F Lobby", meta.Location)
		assert.Equal(t, "1F", meta.Group)
		assert.Equal(t, "public", meta.Labels["zone"])
	})

	t.Run("Capabilities returns supported capabilities list", func(t *testing.T) {
		caps := dev.Capabilities()
		assert.Len(t, caps, 3)
		assert.Contains(t, caps, "set_temperature")
		assert.Contains(t, caps, "set_mode")
		assert.Contains(t, caps, "set_power")
	})
}

func TestDeviceStateZeroValue(t *testing.T) {
	var state DeviceState

	t.Run("zero value has sensible defaults", func(t *testing.T) {
		assert.False(t, state.Online)
		assert.False(t, state.Ready)
		assert.True(t, state.LastSeen.IsZero())
		assert.Equal(t, 0, state.ErrorCount)
		assert.Nil(t, state.Properties)
	})
}

func TestDeviceMetadataZeroValue(t *testing.T) {
	var meta DeviceMetadata

	t.Run("zero value has nil slices and empty strings", func(t *testing.T) {
		assert.Nil(t, meta.Tags)
		assert.Empty(t, meta.Location)
		assert.Empty(t, meta.Group)
		assert.Nil(t, meta.Labels)
	})
}

func TestDeviceOffline(t *testing.T) {
	dev := &mockDevice{
		id:       "modbus-agent:sensor-01",
		online:   false,
		lastSeen: time.Now().Add(-10 * time.Minute),
		state: DeviceState{
			Online:     false,
			Ready:      false,
			ErrorCount: 3,
		},
	}

	t.Run("offline device returns false for Online", func(t *testing.T) {
		assert.False(t, dev.Online())
	})

	t.Run("offline device state reflects error count", func(t *testing.T) {
		assert.Equal(t, 3, dev.State().ErrorCount)
	})
}

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrDeviceNotFound", ErrDeviceNotFound, "device not found"},
		{"ErrNotControllable", ErrNotControllable, "device is not controllable"},
		{"ErrAgentStopped", ErrAgentStopped, "agent is stopped"},
		{"ErrDuplicateDevice", ErrDuplicateDevice, "device already registered"},
		{"ErrProviderNotFound", ErrProviderNotFound, "device provider not found"},
		{"ErrCommandNotFound", ErrCommandNotFound, "command not found"},
		{"ErrInvalidParams", ErrInvalidParams, "invalid command parameters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualError(t, tt.err, tt.msg)
		})
	}
}

func TestCommandSpec(t *testing.T) {
	minVal := 16.0
	maxVal := 30.0
	spec := CommandSpec{
		Name:        "set_temperature",
		Description: "Set target temperature",
		Params: []ParamSpec{
			{
				Name:     "value",
				Type:     "float",
				Required: true,
				Min:      &minVal,
				Max:      &maxVal,
			},
			{
				Name:     "unit",
				Type:     "enum",
				Required: false,
				Enum:     []string{"celsius", "fahrenheit"},
			},
		},
	}

	t.Run("command spec has correct name and description", func(t *testing.T) {
		assert.Equal(t, "set_temperature", spec.Name)
		assert.Equal(t, "Set target temperature", spec.Description)
	})

	t.Run("command spec params have correct types", func(t *testing.T) {
		assert.Len(t, spec.Params, 2)

		valueParam := spec.Params[0]
		assert.Equal(t, "value", valueParam.Name)
		assert.Equal(t, "float", valueParam.Type)
		assert.True(t, valueParam.Required)
		assert.NotNil(t, valueParam.Min)
		assert.Equal(t, 16.0, *valueParam.Min)
		assert.NotNil(t, valueParam.Max)
		assert.Equal(t, 30.0, *valueParam.Max)
		assert.Nil(t, valueParam.Enum)

		unitParam := spec.Params[1]
		assert.Equal(t, "unit", unitParam.Name)
		assert.Equal(t, "enum", unitParam.Type)
		assert.False(t, unitParam.Required)
		assert.Nil(t, unitParam.Min)
		assert.Nil(t, unitParam.Max)
		assert.Equal(t, []string{"celsius", "fahrenheit"}, unitParam.Enum)
	})
}

func TestParamSpecZeroValue(t *testing.T) {
	var p ParamSpec

	t.Run("zero value has empty strings and nil pointers", func(t *testing.T) {
		assert.Empty(t, p.Name)
		assert.Empty(t, p.Type)
		assert.False(t, p.Required)
		assert.Nil(t, p.Min)
		assert.Nil(t, p.Max)
		assert.Nil(t, p.Enum)
	})
}
