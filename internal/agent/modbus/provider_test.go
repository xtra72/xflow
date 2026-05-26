package modbus

import (
	"log/slog"
	"os"
	"testing"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
)

// newTestModbusAgentForProvider creates a minimal ModbusAgent for provider testing.
func newTestModbusAgentForProvider(name string, devices []*ModbusDevice, caches map[string]*RegisterCache) *ModbusAgent {
	return &ModbusAgent{
		agentConfig: agent.AgentConfig{Name: name},
		devices:     devices,
		caches:      caches,
		config: ModbusConfig{
			EnableWriteEvents: true,
		},
		logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}
}

func TestModbusDeviceProvider_Devices(t *testing.T) {
	dev1 := &ModbusDevice{
		config: DeviceConfig{
			ID:     "plc-1",
			Host:   "192.168.1.100",
			Port:   502,
			UnitID: 1,
			RegisterGroups: []RegisterGroupConfig{
				{Name: "temperature"},
				{Name: "humidity"},
			},
		},
		online: true,
	}

	dev2 := &ModbusDevice{
		config: DeviceConfig{
			ID:     "plc-2",
			Host:   "192.168.1.101",
			Port:   502,
			UnitID: 2,
		},
		online: false,
	}

	cache := NewRegisterCache()
	cache.UpdateHoldingRegisters(0, []uint16{100, 200})

	caches := map[string]*RegisterCache{
		"plc-1": cache,
	}

	a := newTestModbusAgentForProvider("modbus-test", []*ModbusDevice{dev1, dev2}, caches)
	provider := NewModbusDeviceProvider(a)

	devs := provider.Devices()
	if len(devs) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devs))
	}

	// Find plc-1 by Name (SPEC-DEVICE-IDENTITY-001 Phase D D-T1: Device.ID()
	// returns UUID, not the legacy "<agent>:<deviceID>" composite).
	var plc1 device.Device
	for _, d := range devs {
		if d.Name() == "Modbus Device plc-1" {
			plc1 = d
			break
		}
	}

	if plc1 == nil {
		t.Fatal("expected to find plc-1")
	}

	if plc1.Protocol() != "modbus" {
		t.Errorf("Protocol = %q, want modbus", plc1.Protocol())
	}
	if plc1.AgentName() != "modbus-test" {
		t.Errorf("AgentName = %q, want modbus-test", plc1.AgentName())
	}
	if !plc1.Online() {
		t.Error("plc-1 should be online")
	}

	// Verify capabilities include register groups
	caps := plc1.Capabilities()
	hasTemp := false
	hasHumidity := false
	for _, c := range caps {
		if c == "temperature" {
			hasTemp = true
		}
		if c == "humidity" {
			hasHumidity = true
		}
	}
	if !hasTemp {
		t.Error("capabilities should include 'temperature'")
	}
	if !hasHumidity {
		t.Error("capabilities should include 'humidity'")
	}

	// Controllable
	controllable, ok := plc1.(device.ControllableDevice)
	if !ok {
		t.Fatal("modbus device should implement ControllableDevice")
	}
	cmds := controllable.Commands()
	if len(cmds) != 2 {
		t.Errorf("expected 2 commands, got %d", len(cmds))
	}

	// Verify state has cache data
	state := plc1.State()
	if state.Properties == nil {
		t.Fatal("state.Properties should not be nil")
	}
}

func TestModbusDeviceProvider_Device(t *testing.T) {
	dev := &ModbusDevice{
		config: DeviceConfig{
			ID:     "sensor-1",
			Host:   "10.0.0.1",
			Port:   502,
			UnitID: 1,
		},
		online: true,
	}

	a := newTestModbusAgentForProvider("mb-agent", []*ModbusDevice{dev}, map[string]*RegisterCache{})
	provider := NewModbusDeviceProvider(a)

	// Provider.Device 는 여전히 composite lookup contract 를 보존한다
	// (DeviceRegistry.Get 의 fallback 경로 — D-T2 후에도 provider 내부 매칭은
	// 호환을 위해 유지). SPEC-DEVICE-IDENTITY-001 Phase D D-T1 에 따라
	// 반환되는 Device.ID() 는 UUID 이지만 lookup 입력 자체는 composite 가능.
	d, err := provider.Device("mb-agent:sensor-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Name() != "Modbus Device sensor-1" {
		t.Errorf("Name = %q, want %q", d.Name(), "Modbus Device sensor-1")
	}

	// Wrong agent prefix
	_, err = provider.Device("other:sensor-1")
	if err != device.ErrDeviceNotFound {
		t.Errorf("expected ErrDeviceNotFound, got %v", err)
	}

	// Non-existent device
	_, err = provider.Device("mb-agent:no-such")
	if err != device.ErrDeviceNotFound {
		t.Errorf("expected ErrDeviceNotFound, got %v", err)
	}

	// Empty ID
	_, err = provider.Device("")
	if err != device.ErrDeviceNotFound {
		t.Errorf("expected ErrDeviceNotFound, got %v", err)
	}
}

func TestModbusDeviceProvider_ReadOnlyDevices(t *testing.T) {
	dev := &ModbusDevice{
		config: DeviceConfig{
			ID:   "sensor-1",
			Host: "10.0.0.1",
			Port: 502,
		},
		online: true,
	}

	// EnableWriteEvents = false -> read-only devices
	a := &ModbusAgent{
		agentConfig: agent.AgentConfig{Name: "read-only-agent"},
		devices:     []*ModbusDevice{dev},
		caches:      map[string]*RegisterCache{},
		config: ModbusConfig{
			EnableWriteEvents: false,
		},
		logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	provider := NewModbusDeviceProvider(a)
	devs := provider.Devices()

	if len(devs) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devs))
	}

	// Should NOT implement ControllableDevice (or have nil executor)
	_, isControllable := devs[0].(device.ControllableDevice)
	// Read-only devices are created with NewModbusDevice (no executor)
	// They still have Execute method but it returns ErrNotControllable
	if isControllable {
		t.Log("read-only device still satisfies ControllableDevice interface (adapter always implements it)")
	}

	// Verify it returns ErrNotControllable on Execute
	if controllable, ok := devs[0].(device.ControllableDevice); ok {
		_, err := controllable.Execute(nil, "write_register", nil)
		if err != device.ErrNotControllable {
			t.Errorf("expected ErrNotControllable, got %v", err)
		}
	}
}

func TestModbusDeviceToInfo(t *testing.T) {
	dev := &ModbusDevice{
		config: DeviceConfig{
			ID:     "meter-1",
			Host:   "192.168.0.10",
			Port:   503,
			UnitID: 5,
			RegisterGroups: []RegisterGroupConfig{
				{Name: "power"},
				{Name: "energy"},
			},
		},
		online: true,
	}

	cache := NewRegisterCache()
	cache.UpdateHoldingRegisters(100, []uint16{42})

	caches := map[string]*RegisterCache{
		"meter-1": cache,
	}

	info := modbusDeviceToInfo(dev, caches)

	if info.DeviceID != "meter-1" {
		t.Errorf("DeviceID = %q, want meter-1", info.DeviceID)
	}
	if info.Host != "192.168.0.10" {
		t.Errorf("Host = %q, want 192.168.0.10", info.Host)
	}
	if info.Port != 503 {
		t.Errorf("Port = %d, want 503", info.Port)
	}
	if info.UnitID != 5 {
		t.Errorf("UnitID = %d, want 5", info.UnitID)
	}
	if !info.Online {
		t.Error("Online should be true")
	}
	if len(info.RegisterGroups) != 2 {
		t.Errorf("RegisterGroups count = %d, want 2", len(info.RegisterGroups))
	}
	if info.CacheData == nil {
		t.Error("CacheData should not be nil when cache exists")
	}
}

func TestModbusDeviceProvider_InterfaceCompliance(t *testing.T) {
	var _ device.DeviceProvider = (*ModbusDeviceProvider)(nil)
}
