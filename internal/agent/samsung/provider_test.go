package samsung

import (
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/device/adapter"
)

// newTestNASAAgentForProvider creates a minimal NASAAgent for provider testing.
// It bypasses full agent construction and only sets fields needed by the provider.
func newTestNASAAgentForProvider(name string, devices map[NASAAddress]*NASADevice) *NASAAgent {
	return &NASAAgent{
		agentConfig: agent.AgentConfig{Name: name},
		devices:     devices,
		deviceIDs:   make(map[string]NASAAddress),
	}
}

func TestNASADeviceProvider_Devices(t *testing.T) {
	now := time.Now()
	devices := map[NASAAddress]*NASADevice{
		{0x20, 0x00, 0x01}: {
			Address:  NASAAddress{0x20, 0x00, 0x01},
			DeviceID: "living-room",
			Type:     "indoor",
			Online:   true,
			Ready:    true,
			LastSeen: now,
			State: &NASADeviceState{
				Power:      true,
				Mode:       "cool",
				TargetTemp: 24.0,
				CurrentTemp: 25.5,
				FanSpeed:   "auto",
			},
		},
		{0x10, 0x00, 0x00}: {
			Address:  NASAAddress{0x10, 0x00, 0x00},
			Type:     "outdoor",
			Online:   true,
			LastSeen: now,
		},
	}

	a := newTestNASAAgentForProvider("nasa-test", devices)
	provider := NewNASADeviceProvider(a)

	devs := provider.Devices()
	if len(devs) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devs))
	}

	// Verify we have both indoor and outdoor devices
	var indoor, outdoor device.Device
	for _, d := range devs {
		switch d.Type() {
		case device.DeviceTypeIndoor:
			indoor = d
		case device.DeviceTypeOutdoor:
			outdoor = d
		}
	}

	if indoor == nil {
		t.Fatal("expected indoor device")
	}
	if outdoor == nil {
		t.Fatal("expected outdoor device")
	}

	// Verify indoor device
	if indoor.ID() != "nasa-test:20.00.01" {
		t.Errorf("indoor ID = %q, want %q", indoor.ID(), "nasa-test:20.00.01")
	}
	if indoor.Name() != "living-room" {
		t.Errorf("indoor Name = %q, want %q", indoor.Name(), "living-room")
	}
	if indoor.Protocol() != "nasa" {
		t.Errorf("indoor Protocol = %q, want %q", indoor.Protocol(), "nasa")
	}
	if !indoor.Online() {
		t.Error("indoor should be online")
	}

	// Indoor should be ControllableDevice
	controllable, ok := indoor.(device.ControllableDevice)
	if !ok {
		t.Fatal("indoor device should implement ControllableDevice")
	}
	cmds := controllable.Commands()
	if len(cmds) == 0 {
		t.Error("indoor device should have commands")
	}

	// Verify outdoor device is not controllable (no executor)
	if outdoor.ID() != "nasa-test:10.00.00" {
		t.Errorf("outdoor ID = %q, want %q", outdoor.ID(), "nasa-test:10.00.00")
	}

	// Verify state properties for indoor device
	state := indoor.State()
	if !state.Online {
		t.Error("state.Online should be true")
	}
	if state.Properties == nil {
		t.Fatal("state.Properties should not be nil")
	}
	if power, ok := state.Properties["power"].(bool); !ok || !power {
		t.Errorf("state.Properties[power] = %v, want true", state.Properties["power"])
	}
	if mode, ok := state.Properties["mode"].(string); !ok || mode != "cool" {
		t.Errorf("state.Properties[mode] = %v, want cool", state.Properties["mode"])
	}
}

func TestNASADeviceProvider_Device(t *testing.T) {
	devices := map[NASAAddress]*NASADevice{
		{0x20, 0x00, 0x01}: {
			Address:  NASAAddress{0x20, 0x00, 0x01},
			DeviceID: "ac-1",
			Type:     "indoor",
			Online:   true,
		},
	}

	a := newTestNASAAgentForProvider("my-nasa", devices)
	provider := NewNASADeviceProvider(a)

	// Valid ID
	dev, err := provider.Device("my-nasa:20.00.01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev.ID() != "my-nasa:20.00.01" {
		t.Errorf("ID = %q, want %q", dev.ID(), "my-nasa:20.00.01")
	}

	// Invalid prefix
	_, err = provider.Device("other-agent:20.00.01")
	if err != device.ErrDeviceNotFound {
		t.Errorf("expected ErrDeviceNotFound, got %v", err)
	}

	// Non-existent address
	_, err = provider.Device("my-nasa:FF.FF.FF")
	if err != device.ErrDeviceNotFound {
		t.Errorf("expected ErrDeviceNotFound, got %v", err)
	}

	// Empty ID
	_, err = provider.Device("")
	if err != device.ErrDeviceNotFound {
		t.Errorf("expected ErrDeviceNotFound, got %v", err)
	}
}

func TestNASADeviceProvider_DeviceWithoutState(t *testing.T) {
	// Outdoor device has nil State
	devices := map[NASAAddress]*NASADevice{
		{0x10, 0x00, 0x00}: {
			Address: NASAAddress{0x10, 0x00, 0x00},
			Type:    "outdoor",
			Online:  true,
		},
	}

	a := newTestNASAAgentForProvider("nasa-agent", devices)
	provider := NewNASADeviceProvider(a)

	devs := provider.Devices()
	if len(devs) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devs))
	}

	dev := devs[0]
	state := dev.State()
	// Outdoor device with nil State should have nil properties
	if state.Properties != nil {
		t.Errorf("expected nil Properties for outdoor device, got %v", state.Properties)
	}
}

func TestNASAAddressDotFormat(t *testing.T) {
	tests := []struct {
		addr NASAAddress
		want string
	}{
		{NASAAddress{0x20, 0x00, 0x01}, "20.00.01"},
		{NASAAddress{0x10, 0x00, 0x00}, "10.00.00"},
		{NASAAddress{0x6A, 0xEE, 0xFF}, "6A.EE.FF"},
		{NASAAddress{0x00, 0x00, 0x00}, "00.00.00"},
		{NASAAddress{0xFF, 0xFF, 0xFF}, "FF.FF.FF"},
	}

	for _, tt := range tests {
		got := tt.addr.String()
		if got != tt.want {
			t.Errorf("NASAAddress(%v).String() = %q, want %q", tt.addr, got, tt.want)
		}
	}
}

func TestNasaDeviceToInfo(t *testing.T) {
	now := time.Now()
	dev := &NASADevice{
		Address:    NASAAddress{0x20, 0x01, 0x02},
		DeviceID:   "bedroom",
		Type:       "indoor",
		Online:     true,
		Ready:      true,
		LastSeen:   now,
		ErrorCount: 3,
		State: &NASADeviceState{
			Power:         true,
			Mode:          "heat",
			TargetTemp:    22.0,
			CurrentTemp:   20.5,
			FanSpeed:      "high",
			SwingVertical: true,
			FilterAlarm:   false,
			ErrorCode:     0,
		},
	}

	info := nasaDeviceToInfo(dev)

	if info.Address != "20.01.02" {
		t.Errorf("Address = %q, want %q", info.Address, "20.01.02")
	}
	if info.DeviceID != "bedroom" {
		t.Errorf("DeviceID = %q, want %q", info.DeviceID, "bedroom")
	}
	if info.DeviceType != "indoor" {
		t.Errorf("DeviceType = %q, want %q", info.DeviceType, "indoor")
	}
	if !info.Online {
		t.Error("Online should be true")
	}
	if !info.Ready {
		t.Error("Ready should be true")
	}
	if info.ErrorCount != 3 {
		t.Errorf("ErrorCount = %d, want 3", info.ErrorCount)
	}
	if info.Power == nil || !*info.Power {
		t.Error("Power should be true")
	}
	if info.Mode == nil || *info.Mode != "heat" {
		t.Errorf("Mode = %v, want heat", info.Mode)
	}
	if info.TargetTemp == nil || *info.TargetTemp != 22.0 {
		t.Errorf("TargetTemp = %v, want 22.0", info.TargetTemp)
	}
}

func TestNasaDeviceToInfo_NilState(t *testing.T) {
	dev := &NASADevice{
		Address: NASAAddress{0x10, 0x00, 0x00},
		Type:    "outdoor",
		Online:  true,
	}

	info := nasaDeviceToInfo(dev)

	if info.Power != nil {
		t.Error("Power should be nil for outdoor device")
	}
	if info.Mode != nil {
		t.Error("Mode should be nil for outdoor device")
	}
	if info.TargetTemp != nil {
		t.Error("TargetTemp should be nil for outdoor device")
	}
}

func TestNASADeviceProvider_InterfaceCompliance(t *testing.T) {
	// Verify compile-time interface compliance
	var _ device.DeviceProvider = (*NASADeviceProvider)(nil)

	devices := map[NASAAddress]*NASADevice{
		{0x20, 0x00, 0x01}: {
			Address: NASAAddress{0x20, 0x00, 0x01},
			Type:    "indoor",
			Online:  true,
			State:   &NASADeviceState{},
		},
	}

	a := newTestNASAAgentForProvider("test-agent", devices)
	provider := NewNASADeviceProvider(a)

	devs := provider.Devices()
	if len(devs) == 0 {
		t.Fatal("expected at least 1 device")
	}

	dev := devs[0]
	// Indoor device should implement ControllableDevice
	_, ok := dev.(device.ControllableDevice)
	if !ok {
		t.Error("indoor device should implement ControllableDevice")
	}
}

func TestNASADeviceProvider_Capabilities(t *testing.T) {
	devices := map[NASAAddress]*NASADevice{
		{0x20, 0x00, 0x01}: {
			Address: NASAAddress{0x20, 0x00, 0x01},
			Type:    "indoor",
			Online:  true,
			State:   &NASADeviceState{},
		},
	}

	a := newTestNASAAgentForProvider("test-agent", devices)
	provider := NewNASADeviceProvider(a)

	devs := provider.Devices()
	caps := devs[0].Capabilities()

	expected := []string{"set_temperature", "set_mode", "set_power", "set_fan_speed"}
	if len(caps) != len(expected) {
		t.Fatalf("capabilities count = %d, want %d", len(caps), len(expected))
	}
	for i, c := range caps {
		if c != expected[i] {
			t.Errorf("capabilities[%d] = %q, want %q", i, c, expected[i])
		}
	}
}

// Test that adapter types properly satisfy device interfaces
func TestNASAAdapterInterfaces(t *testing.T) {
	info := adapter.NASADeviceInfo{
		Address:    "20.00.01",
		DeviceType: "indoor",
		Online:     true,
	}

	// Read-only
	readOnly := adapter.NewNASADevice("test", info)
	var _ device.Device = readOnly

	// Controllable
	controllable := adapter.NewControllableNASADevice("test", info, nil)
	var _ device.Device = controllable
	var _ device.ControllableDevice = controllable
}
