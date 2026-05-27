package device

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockControllableDevice implements ControllableDevice for testing.
type mockControllableDevice struct {
	mockDevice
	executeFunc func(ctx context.Context, command string, params map[string]any) (map[string]any, error)
	commands    []CommandSpec
}

func (m *mockControllableDevice) Execute(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, command, params)
	}
	return nil, nil
}

func (m *mockControllableDevice) Commands() []CommandSpec {
	return m.commands
}

// mockDeviceProvider implements DeviceProvider for testing.
type mockDeviceProvider struct {
	devices []Device
}

func (m *mockDeviceProvider) Devices() []Device {
	return m.devices
}

func (m *mockDeviceProvider) Device(id string) (Device, error) {
	for _, d := range m.devices {
		if d.ID() == id {
			return d, nil
		}
	}
	return nil, ErrDeviceNotFound
}

// Helper to create a simple mock provider with given devices.
func newMockProvider(devices ...Device) *mockDeviceProvider {
	return &mockDeviceProvider{devices: devices}
}

// Helper to create a simple mock device with minimal fields.
func newRegistryTestDevice(id, protocol, agentName string, online bool, tags []string) *mockDevice {
	return &mockDevice{
		id:         id,
		name:       "Device " + id,
		deviceType: DeviceTypeSensor,
		protocol:   protocol,
		agentName:  agentName,
		online:     online,
		lastSeen:   time.Now(),
		state:      DeviceState{Online: online},
		metadata: DeviceMetadata{
			Tags: tags,
		},
	}
}

func TestNewRegistryReturnsEmptyRegistry(t *testing.T) {
	reg := NewRegistry()

	assert.NotNil(t, reg)
	assert.Equal(t, 0, reg.Count())
}

func TestRegisterProviderAddsDevices(t *testing.T) {
	reg := NewRegistry()

	dev1 := newRegistryTestDevice("agent1:dev1", "nasa", "agent1", true, nil)
	dev2 := newRegistryTestDevice("agent1:dev2", "nasa", "agent1", true, nil)
	provider := newMockProvider(dev1, dev2)

	reg.RegisterProvider("agent1", provider)

	assert.Equal(t, 2, reg.Count())
}

func TestListWithEmptyFilterReturnsAllDevices(t *testing.T) {
	reg := NewRegistry()

	dev1 := newRegistryTestDevice("agent1:dev1", "nasa", "agent1", true, nil)
	dev2 := newRegistryTestDevice("agent2:dev1", "modbus", "agent2", true, nil)

	reg.RegisterProvider("agent1", newMockProvider(dev1))
	reg.RegisterProvider("agent2", newMockProvider(dev2))

	devices := reg.List(DeviceFilter{})

	assert.Len(t, devices, 2)
}

func TestListWithProtocolFilter(t *testing.T) {
	reg := NewRegistry()

	nasaDev := newRegistryTestDevice("nasa-agent:dev1", "nasa", "nasa-agent", true, nil)
	modbusDev := newRegistryTestDevice("modbus-agent:dev1", "modbus", "modbus-agent", true, nil)

	reg.RegisterProvider("nasa-agent", newMockProvider(nasaDev))
	reg.RegisterProvider("modbus-agent", newMockProvider(modbusDev))

	devices := reg.List(DeviceFilter{Protocol: "nasa"})

	assert.Len(t, devices, 1)
	assert.Equal(t, "nasa-agent:dev1", devices[0].ID())
}

func TestListWithAgentNameFilter(t *testing.T) {
	reg := NewRegistry()

	dev1 := newRegistryTestDevice("agent1:dev1", "nasa", "agent1", true, nil)
	dev2 := newRegistryTestDevice("agent1:dev2", "nasa", "agent1", true, nil)
	dev3 := newRegistryTestDevice("agent2:dev1", "modbus", "agent2", true, nil)

	reg.RegisterProvider("agent1", newMockProvider(dev1, dev2))
	reg.RegisterProvider("agent2", newMockProvider(dev3))

	devices := reg.List(DeviceFilter{AgentName: "agent1"})

	assert.Len(t, devices, 2)
	for _, d := range devices {
		assert.Equal(t, "agent1", d.AgentName())
	}
}

func TestListWithOnlineFilter(t *testing.T) {
	reg := NewRegistry()

	onlineDev := newRegistryTestDevice("agent1:online", "nasa", "agent1", true, nil)
	offlineDev := newRegistryTestDevice("agent1:offline", "nasa", "agent1", false, nil)

	reg.RegisterProvider("agent1", newMockProvider(onlineDev, offlineDev))

	boolTrue := true
	boolFalse := false

	t.Run("filter online=true returns only online devices", func(t *testing.T) {
		devices := reg.List(DeviceFilter{Online: &boolTrue})
		assert.Len(t, devices, 1)
		assert.True(t, devices[0].Online())
	})

	t.Run("filter online=false returns only offline devices", func(t *testing.T) {
		devices := reg.List(DeviceFilter{Online: &boolFalse})
		assert.Len(t, devices, 1)
		assert.False(t, devices[0].Online())
	})
}

func TestListWithTagsFilter(t *testing.T) {
	reg := NewRegistry()

	dev1 := newRegistryTestDevice("agent1:dev1", "nasa", "agent1", true, []string{"hvac", "lobby"})
	dev2 := newRegistryTestDevice("agent1:dev2", "nasa", "agent1", true, []string{"hvac", "office"})
	dev3 := newRegistryTestDevice("agent1:dev3", "nasa", "agent1", true, []string{"sensor"})

	reg.RegisterProvider("agent1", newMockProvider(dev1, dev2, dev3))

	t.Run("single tag filter", func(t *testing.T) {
		devices := reg.List(DeviceFilter{Tags: []string{"hvac"}})
		assert.Len(t, devices, 2)
	})

	t.Run("multiple tags filter (AND)", func(t *testing.T) {
		devices := reg.List(DeviceFilter{Tags: []string{"hvac", "lobby"}})
		assert.Len(t, devices, 1)
		assert.Equal(t, "agent1:dev1", devices[0].ID())
	})
}

func TestListWithMultipleFilterCriteria(t *testing.T) {
	reg := NewRegistry()

	boolTrue := true

	dev1 := newRegistryTestDevice("nasa-agent:dev1", "nasa", "nasa-agent", true, []string{"hvac"})
	dev2 := newRegistryTestDevice("nasa-agent:dev2", "nasa", "nasa-agent", false, []string{"hvac"})
	dev3 := newRegistryTestDevice("modbus-agent:dev1", "modbus", "modbus-agent", true, []string{"hvac"})

	reg.RegisterProvider("nasa-agent", newMockProvider(dev1, dev2))
	reg.RegisterProvider("modbus-agent", newMockProvider(dev3))

	// Protocol=nasa AND Online=true AND Tags=hvac -> only dev1
	devices := reg.List(DeviceFilter{
		Protocol: "nasa",
		Online:   &boolTrue,
		Tags:     []string{"hvac"},
	})

	assert.Len(t, devices, 1)
	assert.Equal(t, "nasa-agent:dev1", devices[0].ID())
}

func TestGetExistingDevice(t *testing.T) {
	reg := NewRegistry()

	dev := newRegistryTestDevice("agent1:dev1", "nasa", "agent1", true, nil)
	reg.RegisterProvider("agent1", newMockProvider(dev))

	result, err := reg.Get("agent1:dev1")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "agent1:dev1", result.ID())
}

func TestGetNonExistentDeviceReturnsError(t *testing.T) {
	reg := NewRegistry()

	dev := newRegistryTestDevice("agent1:dev1", "nasa", "agent1", true, nil)
	reg.RegisterProvider("agent1", newMockProvider(dev))

	result, err := reg.Get("agent1:nonexistent")

	assert.Nil(t, result)
	assert.True(t, errors.Is(err, ErrDeviceNotFound))
}

func TestUnregisterProviderMarksDevicesOffline(t *testing.T) {
	reg := NewRegistry()

	dev := newRegistryTestDevice("agent1:dev1", "nasa", "agent1", true, nil)
	reg.RegisterProvider("agent1", newMockProvider(dev))

	// Before unregister, device is online.
	result, err := reg.Get("agent1:dev1")
	assert.NoError(t, err)
	assert.True(t, result.Online())

	// Unregister the provider.
	reg.UnregisterProvider("agent1")

	// After unregister, device should appear offline.
	result, err = reg.Get("agent1:dev1")
	assert.NoError(t, err)
	assert.False(t, result.Online())
}

func TestUnregisterProviderDevicesStillAppearInList(t *testing.T) {
	reg := NewRegistry()

	dev := newRegistryTestDevice("agent1:dev1", "nasa", "agent1", true, nil)
	reg.RegisterProvider("agent1", newMockProvider(dev))

	reg.UnregisterProvider("agent1")

	// Devices from offline agent should still appear in List.
	devices := reg.List(DeviceFilter{})
	assert.Len(t, devices, 1)
	assert.Equal(t, "agent1:dev1", devices[0].ID())
	assert.False(t, devices[0].Online()) // but shown as offline
}

func TestReRegisterProviderRestoresOnlineStatus(t *testing.T) {
	reg := NewRegistry()

	dev := newRegistryTestDevice("agent1:dev1", "nasa", "agent1", true, nil)
	provider := newMockProvider(dev)

	reg.RegisterProvider("agent1", provider)
	reg.UnregisterProvider("agent1")

	// After unregister, device is offline.
	result, _ := reg.Get("agent1:dev1")
	assert.False(t, result.Online())

	// Re-register the same provider.
	reg.RegisterProvider("agent1", provider)

	// After re-register, device should be online again.
	result, err := reg.Get("agent1:dev1")
	assert.NoError(t, err)
	assert.True(t, result.Online())
}

func TestSetMetadataSucceedsForExistingDevice(t *testing.T) {
	reg := NewRegistry()

	dev := newRegistryTestDevice("agent1:dev1", "nasa", "agent1", true, nil)
	reg.RegisterProvider("agent1", newMockProvider(dev))

	meta := DeviceMetadata{
		Tags:     []string{"hvac", "lobby"},
		Location: "1F Lobby",
		Group:    "1F",
		Labels:   map[string]string{"zone": "public"},
	}

	err := reg.SetMetadata("agent1:dev1", meta)
	assert.NoError(t, err)

	// Verify metadata was stored.
	storedMeta, err := reg.GetMetadata("agent1:dev1")
	assert.NoError(t, err)
	assert.Equal(t, meta.Tags, storedMeta.Tags)
	assert.Equal(t, meta.Location, storedMeta.Location)
	assert.Equal(t, meta.Group, storedMeta.Group)
	assert.Equal(t, "public", storedMeta.Labels["zone"])
}

// TestSetMetadataAcceptsNonExistentDevice 는 SetMetadata 가 디바이스 미존재 시
// 에도 메타데이터를 저장하는지 검증한다 (2026-05-27 변경 이후 동작).
//
// 배경: auto-discovered 디바이스는 서버 부팅 시점엔 아직 발견되지 않을 수 있다.
// 영속 저장소에서 메타데이터를 pre-load 할 때, 디바이스 존재 검증이 있으면
// ErrDeviceNotFound 로 실패하여 사용자의 이름 변경이 재시작 후 사라지는
// race condition 발생. 검증을 제거하여 추후 디바이스가 발견되었을 때
// GetMetadata 가 정상 동작.
func TestSetMetadataAcceptsNonExistentDevice(t *testing.T) {
	reg := NewRegistry()

	meta := DeviceMetadata{Name: "거실", Tags: []string{"test"}}
	err := reg.SetMetadata("yet-to-be-discovered-uuid", meta)
	assert.NoError(t, err)

	stored, err := reg.GetMetadata("yet-to-be-discovered-uuid")
	assert.NoError(t, err)
	assert.Equal(t, "거실", stored.Name)
}

// TestSetMetadataRejectsEmptyID 는 빈 ID 만 거부하는지 검증.
func TestSetMetadataRejectsEmptyID(t *testing.T) {
	reg := NewRegistry()
	err := reg.SetMetadata("", DeviceMetadata{Name: "x"})
	assert.True(t, errors.Is(err, ErrDeviceNotFound))
}

// compositeOnlyProvider 는 실제 NASA/LGCP/LG ICP-01/Century provider 의 패턴을
// 재현한다: Device(id) 가 "agentName:address" 형식만 받아들이고 UUID 는
// 거부한다. registry 가 provider 의 composite 한계에 의존하지 않고 UUID
// lookup 을 지원하는지 검증하기 위한 mock.
//
// SPEC-DEVICE-IDENTITY-001 Phase D § D-T1 정합성 회귀 방지용.
type compositeOnlyProvider struct {
	agentName string
	devices   []Device
}

func (p *compositeOnlyProvider) Devices() []Device {
	return p.devices
}

func (p *compositeOnlyProvider) Device(id string) (Device, error) {
	prefix := p.agentName + ":"
	if len(id) <= len(prefix) || id[:len(prefix)] != prefix {
		return nil, ErrDeviceNotFound
	}
	addr := id[len(prefix):]
	for _, d := range p.devices {
		// composite 매칭만 지원 — UUID 는 의도적으로 매칭 실패.
		if "agent1:"+addr == d.ID() || d.Name() == addr {
			return d, nil
		}
	}
	return nil, ErrDeviceNotFound
}

// TestSetMetadataWorksWithUUIDIDOnCompositeOnlyProvider 는 SPEC-DEVICE-IDENTITY-001
// Phase D § D-T1 이후 Device.ID() 가 UUID 를 반환하는 환경에서, provider 의
// Device(id) 가 여전히 composite format 만 지원하더라도 SetMetadata 가 UUID
// 로 정상 동작하는지 검증한다.
//
// 회귀 방지: 2026-05-26 발견된 디바이스 명 변경 안됨 버그. 이전 코드는
// inMemoryRegistry.deviceExists 가 provider.Device(uuid) 를 호출 → composite
// 형식 미일치로 ErrDeviceNotFound → SetMetadata 가 항상 404 반환.
func TestSetMetadataWorksWithUUIDIDOnCompositeOnlyProvider(t *testing.T) {
	reg := NewRegistry()

	// UUID 를 ID 로 반환하는 디바이스 (Phase D § D-T1 정상 동작).
	uuid := "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"
	dev := newRegistryTestDevice(uuid, "nasa", "agent1", true, nil)
	// provider 는 composite 형식만 지원 (real-world NASA/LGCP/Century 와 동일).
	provider := &compositeOnlyProvider{agentName: "agent1", devices: []Device{dev}}
	reg.RegisterProvider("agent1", provider)

	meta := DeviceMetadata{Name: "거실 에어컨", Location: "1F"}
	err := reg.SetMetadata(uuid, meta)
	assert.NoError(t, err, "SetMetadata 는 UUID 로 동작해야 한다 (provider Device(uuid) 가 실패해도)")

	stored, err := reg.GetMetadata(uuid)
	assert.NoError(t, err)
	assert.Equal(t, "거실 에어컨", stored.Name)
	assert.Equal(t, "1F", stored.Location)
}

// TestGetWorksWithUUIDIDOnCompositeOnlyProvider 는 Phase D § D-T1 환경에서
// registry.Get(uuid) 가 동작하는지 검증한다.
func TestGetWorksWithUUIDIDOnCompositeOnlyProvider(t *testing.T) {
	reg := NewRegistry()

	uuid := "b69cb778-6852-5c4d-ae3f-8f4c9b2c3d4e"
	dev := newRegistryTestDevice(uuid, "nasa", "agent1", true, nil)
	provider := &compositeOnlyProvider{agentName: "agent1", devices: []Device{dev}}
	reg.RegisterProvider("agent1", provider)

	result, err := reg.Get(uuid)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, uuid, result.ID())
}

func TestGetMetadataReturnsEmptyForDeviceWithoutMetadata(t *testing.T) {
	reg := NewRegistry()

	dev := newRegistryTestDevice("agent1:dev1", "nasa", "agent1", true, nil)
	reg.RegisterProvider("agent1", newMockProvider(dev))

	meta, err := reg.GetMetadata("agent1:dev1")

	assert.NoError(t, err)
	assert.Empty(t, meta.Tags)
	assert.Empty(t, meta.Location)
	assert.Empty(t, meta.Group)
	assert.Nil(t, meta.Labels)
}

func TestExecuteOnControllableDeviceSucceeds(t *testing.T) {
	reg := NewRegistry()

	controllable := &mockControllableDevice{
		mockDevice: mockDevice{
			id:        "agent1:ctrl1",
			name:      "Controllable Device",
			protocol:  "nasa",
			agentName: "agent1",
			online:    true,
			lastSeen:  time.Now(),
			state:     DeviceState{Online: true},
		},
		executeFunc: func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
			return map[string]any{"status": "ok", "command": command}, nil
		},
		commands: []CommandSpec{
			{Name: "target_temperature", Description: "Set temperature"},
		},
	}

	reg.RegisterProvider("agent1", newMockProvider(controllable))

	result, err := reg.Execute(context.Background(), "agent1:ctrl1", "target_temperature", map[string]any{"value": 24})

	assert.NoError(t, err)
	assert.Equal(t, "ok", result["status"])
	assert.Equal(t, "target_temperature", result["command"])
}

func TestExecuteOnNonControllableDeviceReturnsError(t *testing.T) {
	reg := NewRegistry()

	dev := newRegistryTestDevice("agent1:sensor1", "modbus", "agent1", true, nil)
	reg.RegisterProvider("agent1", newMockProvider(dev))

	result, err := reg.Execute(context.Background(), "agent1:sensor1", "read", nil)

	assert.Nil(t, result)
	assert.True(t, errors.Is(err, ErrNotControllable))
}

func TestExecuteOnOfflineAgentReturnsError(t *testing.T) {
	reg := NewRegistry()

	controllable := &mockControllableDevice{
		mockDevice: mockDevice{
			id:        "agent1:ctrl1",
			name:      "Controllable Device",
			protocol:  "nasa",
			agentName: "agent1",
			online:    true,
			lastSeen:  time.Now(),
			state:     DeviceState{Online: true},
		},
		executeFunc: func(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
			return map[string]any{"status": "ok"}, nil
		},
	}

	reg.RegisterProvider("agent1", newMockProvider(controllable))
	reg.UnregisterProvider("agent1")

	result, err := reg.Execute(context.Background(), "agent1:ctrl1", "target_temperature", nil)

	assert.Nil(t, result)
	assert.True(t, errors.Is(err, ErrAgentStopped))
}

func TestConcurrentAccessSafety(t *testing.T) {
	reg := NewRegistry()

	dev := newRegistryTestDevice("agent1:dev1", "nasa", "agent1", true, nil)
	provider := newMockProvider(dev)

	var wg sync.WaitGroup
	const goroutines = 50

	// Concurrent RegisterProvider / UnregisterProvider / List / Get / Count operations.
	wg.Add(goroutines * 5)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			reg.RegisterProvider("agent1", provider)
		}()

		go func() {
			defer wg.Done()
			reg.UnregisterProvider("agent1")
		}()

		go func() {
			defer wg.Done()
			reg.List(DeviceFilter{})
		}()

		go func() {
			defer wg.Done()
			_, _ = reg.Get("agent1:dev1")
		}()

		go func() {
			defer wg.Done()
			reg.Count()
		}()
	}

	wg.Wait()

	// If we reach here without data race panic, the test passes.
	assert.True(t, true, "concurrent access completed without race conditions")
}

func TestRegisterProviderReplacesExistingProvider(t *testing.T) {
	reg := NewRegistry()

	dev1 := newRegistryTestDevice("agent1:dev1", "nasa", "agent1", true, nil)
	dev2 := newRegistryTestDevice("agent1:dev2", "nasa", "agent1", true, nil)

	provider1 := newMockProvider(dev1)
	provider2 := newMockProvider(dev2)

	reg.RegisterProvider("agent1", provider1)
	assert.Equal(t, 1, reg.Count())

	// Replace with new provider that has a different device.
	reg.RegisterProvider("agent1", provider2)
	assert.Equal(t, 1, reg.Count())

	// Old device should not be found, new device should.
	_, err := reg.Get("agent1:dev1")
	assert.True(t, errors.Is(err, ErrDeviceNotFound))

	result, err := reg.Get("agent1:dev2")
	assert.NoError(t, err)
	assert.Equal(t, "agent1:dev2", result.ID())
}

func TestExecuteOnNonExistentDeviceReturnsError(t *testing.T) {
	reg := NewRegistry()

	result, err := reg.Execute(context.Background(), "nonexistent:dev", "cmd", nil)

	assert.Nil(t, result)
	assert.True(t, errors.Is(err, ErrDeviceNotFound))
}

func TestCountAcrossMultipleProviders(t *testing.T) {
	reg := NewRegistry()

	dev1 := newRegistryTestDevice("agent1:dev1", "nasa", "agent1", true, nil)
	dev2 := newRegistryTestDevice("agent1:dev2", "nasa", "agent1", true, nil)
	dev3 := newRegistryTestDevice("agent2:dev1", "modbus", "agent2", true, nil)

	reg.RegisterProvider("agent1", newMockProvider(dev1, dev2))
	reg.RegisterProvider("agent2", newMockProvider(dev3))

	assert.Equal(t, 3, reg.Count())
}

func TestGetMetadataForNonExistentDeviceReturnsEmpty(t *testing.T) {
	reg := NewRegistry()

	// GetMetadata for a device that doesn't exist in any provider
	// returns empty metadata without error per the contract.
	meta, err := reg.GetMetadata("nonexistent:dev")

	assert.NoError(t, err)
	assert.Equal(t, DeviceMetadata{}, meta)
}

func TestUnregisterProviderForNonExistentAgent(t *testing.T) {
	reg := NewRegistry()

	// Unregistering a non-existent agent should not panic.
	assert.NotPanics(t, func() {
		reg.UnregisterProvider("nonexistent-agent")
	})
}

func TestOnlineFilterWorksWithOfflineAgentDevices(t *testing.T) {
	reg := NewRegistry()

	boolTrue := true

	dev := newRegistryTestDevice("agent1:dev1", "nasa", "agent1", true, nil)
	reg.RegisterProvider("agent1", newMockProvider(dev))
	reg.UnregisterProvider("agent1")

	// Online filter should see device as offline since agent is offline.
	devices := reg.List(DeviceFilter{Online: &boolTrue})
	assert.Len(t, devices, 0)
}
