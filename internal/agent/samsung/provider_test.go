package samsung

import (
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/device/adapter"
)

// newTestHvacr01AgentForProvider creates a minimal Hvacr01Agent for provider testing.
// It bypasses full agent construction and only sets fields needed by the provider.
func newTestHvacr01AgentForProvider(name string, devices map[NasaAddress]*NasaDevice) *Hvacr01Agent {
	return &Hvacr01Agent{
		agentConfig: agent.AgentConfig{Name: name},
		devices:     devices,
		deviceIDs:   make(map[string]NasaAddress),
	}
}

func TestHvacr01DeviceProvider_Devices(t *testing.T) {
	now := time.Now()
	devices := map[NasaAddress]*NasaDevice{
		{0x20, 0x00, 0x01}: {
			Address:  NasaAddress{0x20, 0x00, 0x01},
			UnitID:   "living-room",
			Type:     "HVACR.IDU",
			Online:   true,
			Ready:    true,
			LastSeen: now,
			State: &NasaDeviceState{
				Power:       true,
				Mode:        "cool",
				TargetTemp:  24.0,
				CurrentTemp: 25.5,
				FanSpeed:    "auto",
			},
		},
		{0x10, 0x00, 0x00}: {
			Address:  NasaAddress{0x10, 0x00, 0x00},
			Type:     "HVACR.ODU",
			Online:   true,
			LastSeen: now,
		},
	}

	a := newTestHvacr01AgentForProvider("samsung-nasa-test", devices)
	provider := NewHvacr01DeviceProvider(a)

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

	// SPEC-DEVICE-IDENTITY-001 Phase D D-T1: Device.ID() returns UUID, no
	// longer the composite "agent:address" format. We verify the device by
	// its agent/name pair (1급 식별자) and other domain attributes.
	if indoor.AgentName() != "samsung-nasa-test" {
		t.Errorf("indoor AgentName = %q, want %q", indoor.AgentName(), "samsung-nasa-test")
	}
	if indoor.Name() != "living-room" {
		t.Errorf("indoor Name = %q, want %q", indoor.Name(), "living-room")
	}
	if indoor.Protocol() != "samsung_nasa" {
		t.Errorf("indoor Protocol = %q, want %q", indoor.Protocol(), "samsung_nasa")
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

	// Verify outdoor device is not controllable (no executor).
	// Phase D D-T1: outdoor.ID() returns UUID; identify via agent + Type instead.
	if outdoor.AgentName() != "samsung-nasa-test" {
		t.Errorf("outdoor AgentName = %q, want %q", outdoor.AgentName(), "samsung-nasa-test")
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
	// SPEC-CENTURY-001 v0.18.13 후속: mode 는 hvac 통일 ID (int). "cool" → 1.
	if mode, ok := state.Properties["mode"].(int); !ok || mode != 1 {
		t.Errorf("state.Properties[mode] = %v, want 1 (cool)", state.Properties["mode"])
	}
}

func TestHvacr01DeviceProvider_Device(t *testing.T) {
	devices := map[NasaAddress]*NasaDevice{
		{0x20, 0x00, 0x01}: {
			Address: NasaAddress{0x20, 0x00, 0x01},
			UnitID:  "ac-1",
			Type:    "HVACR.IDU",
			Online:  true,
		},
	}

	a := newTestHvacr01AgentForProvider("my-samsung-nasa", devices)
	provider := NewHvacr01DeviceProvider(a)

	// Provider.Device 는 여전히 composite lookup contract 를 보존한다 — D-T2 후에도
	// DeviceRegistry.Get fallback 경로에서 사용됨. SPEC-DEVICE-IDENTITY-001 Phase D
	// D-T1 에 따라 반환된 Device.ID() 는 UUID 이지만 lookup 입력은 composite 가능.
	dev, err := provider.Device("my-samsung-nasa:20.00.01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev.AgentName() != "my-samsung-nasa" {
		t.Errorf("AgentName = %q, want %q", dev.AgentName(), "my-samsung-nasa")
	}
	if dev.Name() != "ac-1" {
		t.Errorf("Name = %q, want %q", dev.Name(), "ac-1")
	}

	// Invalid prefix
	_, err = provider.Device("other-agent:20.00.01")
	if err != device.ErrDeviceNotFound {
		t.Errorf("expected ErrDeviceNotFound, got %v", err)
	}

	// Non-existent address
	_, err = provider.Device("my-samsung-nasa:FF.FF.FF")
	if err != device.ErrDeviceNotFound {
		t.Errorf("expected ErrDeviceNotFound, got %v", err)
	}

	// Empty ID
	_, err = provider.Device("")
	if err != device.ErrDeviceNotFound {
		t.Errorf("expected ErrDeviceNotFound, got %v", err)
	}
}

func TestHvacr01DeviceProvider_DeviceWithoutState(t *testing.T) {
	// Outdoor device has nil State
	devices := map[NasaAddress]*NasaDevice{
		{0x10, 0x00, 0x00}: {
			Address: NasaAddress{0x10, 0x00, 0x00},
			Type:    "HVACR.ODU",
			Online:  true,
		},
	}

	a := newTestHvacr01AgentForProvider("samsung-hvacr01-agent", devices)
	provider := NewHvacr01DeviceProvider(a)

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

func TestNasaAddressDotFormat(t *testing.T) {
	tests := []struct {
		addr NasaAddress
		want string
	}{
		{NasaAddress{0x20, 0x00, 0x01}, "20.00.01"},
		{NasaAddress{0x10, 0x00, 0x00}, "10.00.00"},
		{NasaAddress{0x6A, 0xEE, 0xFF}, "6A.EE.FF"},
		{NasaAddress{0x00, 0x00, 0x00}, "00.00.00"},
		{NasaAddress{0xFF, 0xFF, 0xFF}, "FF.FF.FF"},
	}

	for _, tt := range tests {
		got := tt.addr.String()
		if got != tt.want {
			t.Errorf("NasaAddress(%v).String() = %q, want %q", tt.addr, got, tt.want)
		}
	}
}

func TestHvacr01DeviceToInfo(t *testing.T) {
	now := time.Now()
	dev := &NasaDevice{
		Address:    NasaAddress{0x20, 0x01, 0x02},
		UnitID:     "bedroom",
		Type:       "HVACR.IDU",
		Online:     true,
		Ready:      true,
		LastSeen:   now,
		ErrorCount: 3,
		State: &NasaDeviceState{
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

	info := hvacr01DeviceToInfo(dev)

	if info.Address != "20.01.02" {
		t.Errorf("Address = %q, want %q", info.Address, "20.01.02")
	}
	if info.DeviceID != "bedroom" {
		t.Errorf("DeviceID = %q, want %q", info.DeviceID, "bedroom")
	}
	if info.DeviceType != "HVACR.IDU" {
		t.Errorf("DeviceType = %q, want %q", info.DeviceType, "HVACR.IDU")
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
	dev := &NasaDevice{
		Address: NasaAddress{0x10, 0x00, 0x00},
		Type:    "HVACR.ODU",
		Online:  true,
	}

	info := hvacr01DeviceToInfo(dev)

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

func TestHvacr01DeviceProvider_InterfaceCompliance(t *testing.T) {
	// Verify compile-time interface compliance
	var _ device.DeviceProvider = (*Hvacr01DeviceProvider)(nil)

	devices := map[NasaAddress]*NasaDevice{
		{0x20, 0x00, 0x01}: {
			Address: NasaAddress{0x20, 0x00, 0x01},
			Type:    "HVACR.IDU",
			Online:  true,
			State:   &NasaDeviceState{},
		},
	}

	a := newTestHvacr01AgentForProvider("test-agent", devices)
	provider := NewHvacr01DeviceProvider(a)

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

func TestHvacr01DeviceProvider_Capabilities(t *testing.T) {
	devices := map[NasaAddress]*NasaDevice{
		{0x20, 0x00, 0x01}: {
			Address: NasaAddress{0x20, 0x00, 0x01},
			Type:    "HVACR.IDU",
			Online:  true,
			State:   &NasaDeviceState{},
		},
	}

	a := newTestHvacr01AgentForProvider("test-agent", devices)
	provider := NewHvacr01DeviceProvider(a)

	devs := provider.Devices()
	caps := devs[0].Capabilities()

	expected := []string{"target_temperature", "set_mode", "set_power", "set_fan_speed"}
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
func TestSamsungHvacr01AdapterInterfaces(t *testing.T) {
	info := adapter.SamsungNasaDeviceInfo{
		Address:    "20.00.01",
		DeviceType: "HVACR.IDU",
		Online:     true,
	}

	// Read-only
	readOnly := adapter.NewSamsungNasaDevice("test", info)
	var _ device.Device = readOnly

	// Controllable
	controllable := adapter.NewControllableSamsungNasaDevice("test", info, nil)
	var _ device.Device = controllable
	var _ device.ControllableDevice = controllable
}
