package century

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/device"
)

// makeProviderAgent 는 provider 테스트용 CenturyAgent 를 생성한다.
//
// Init/Start 는 호출하지 않는다 — provider 는 agent.Name() 과 ListDevices() 만
// 사용하므로 lifecycle 진입 없이도 동작한다 (lifecycle race 회피).
func makeProviderAgent(t *testing.T, name string) *CenturyAgent {
	t.Helper()
	rt := newRecordingTransport(nil)
	t.Cleanup(func() { _ = rt.Close() })
	opts := map[string]any{"serial_port": "/dev/ttyTEST"}
	centuryCfg, err := parseCenturyConfig(opts)
	if err != nil {
		t.Fatalf("parseCenturyConfig: %v", err)
	}
	cfg := agent.AgentConfig{
		ID:        name,
		Name:      name,
		Type:      "century-hvac",
		Transport: agent.TransportConfig{Type: "serial", Options: opts},
	}
	return newCenturyAgentForTest(cfg, centuryCfg, rt)
}

// addProviderDevice 는 agent.devices 맵에 직접 device 를 등록한다 (테스트 헬퍼).
func addProviderDevice(a *CenturyAgent, subDevID byte, source string, online bool, st *CenturyDeviceState) {
	now := time.Now()
	dev := NewCenturyDevice(subDevID, source, now)
	dev.Online = online
	if st != nil {
		dev.State = st
	}
	a.devicesMu.Lock()
	a.devices[subDevID] = dev
	a.devicesMu.Unlock()
}

func TestCenturyDeviceProvider_NewPanicsOnNil(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() { _ = NewCenturyDeviceProvider(nil) })
}

func TestCenturyDeviceProvider_DevicesEmpty(t *testing.T) {
	t.Parallel()
	a := makeProviderAgent(t, "ct-empty")
	p := NewCenturyDeviceProvider(a)
	assert.Empty(t, p.Devices())
}

func TestCenturyDeviceProvider_DevicesReturnsAllRegistered(t *testing.T) {
	t.Parallel()
	a := makeProviderAgent(t, "ct-multi")
	addProviderDevice(a, 0x3B, "auto", true, &CenturyDeviceState{})
	addProviderDevice(a, 0x3C, "config", false, &CenturyDeviceState{})

	p := NewCenturyDeviceProvider(a)
	devs := p.Devices()
	require.Len(t, devs, 2)

	ids := make(map[string]device.Device, len(devs))
	for _, d := range devs {
		ids[d.ID()] = d
	}
	d3b, ok := ids["ct-multi:3b"]
	require.True(t, ok, "device 0x3B must be present")
	assert.True(t, d3b.Online())
	assert.Equal(t, "auto", d3b.Source())
	assert.Equal(t, "century-hvac", d3b.Protocol())
	assert.Equal(t, "ct-multi", d3b.AgentName())
	assert.Equal(t, device.DeviceTypeIndoor, d3b.Type())

	d3c, ok := ids["ct-multi:3c"]
	require.True(t, ok, "device 0x3C must be present")
	assert.False(t, d3c.Online())
	assert.Equal(t, "config", d3c.Source())
}

func TestCenturyDeviceProvider_DeviceByID(t *testing.T) {
	t.Parallel()
	a := makeProviderAgent(t, "ct-byid")
	addProviderDevice(a, 0x3B, "auto", true, &CenturyDeviceState{})
	p := NewCenturyDeviceProvider(a)

	t.Run("hit hex lower", func(t *testing.T) {
		d, err := p.Device("ct-byid:3b")
		require.NoError(t, err)
		assert.Equal(t, "ct-byid:3b", d.ID())
	})
	t.Run("hit hex upper", func(t *testing.T) {
		d, err := p.Device("ct-byid:3B")
		require.NoError(t, err)
		assert.Equal(t, "ct-byid:3b", d.ID())
	})
	t.Run("hit hex 0x prefix", func(t *testing.T) {
		d, err := p.Device("ct-byid:0x3b")
		require.NoError(t, err)
		assert.Equal(t, "ct-byid:3b", d.ID())
	})
	t.Run("miss unknown agent", func(t *testing.T) {
		_, err := p.Device("other:3b")
		assert.ErrorIs(t, err, device.ErrDeviceNotFound)
	})
	t.Run("miss empty suffix", func(t *testing.T) {
		_, err := p.Device("ct-byid:")
		assert.ErrorIs(t, err, device.ErrDeviceNotFound)
	})
	t.Run("miss bad hex", func(t *testing.T) {
		_, err := p.Device("ct-byid:zz")
		assert.ErrorIs(t, err, device.ErrDeviceNotFound)
	})
	t.Run("miss unknown sub_dev_id", func(t *testing.T) {
		_, err := p.Device("ct-byid:99")
		assert.ErrorIs(t, err, device.ErrDeviceNotFound)
	})
}

func TestCenturyDeviceAdapter_StateProperties_Reg02(t *testing.T) {
	t.Parallel()
	st := &CenturyDeviceState{
		Reg02: &Reg02Decoded{
			Mode:      ModeField{Value: "cooling"},
			Fan:       FieldU8{Value: 17},
			SetpointC: FieldFloat32{Value: 25.0},
		},
	}
	a := makeProviderAgent(t, "ct-st")
	addProviderDevice(a, 0x3B, "auto", true, st)

	p := NewCenturyDeviceProvider(a)
	d, err := p.Device("ct-st:3b")
	require.NoError(t, err)

	state := d.State()
	assert.True(t, state.Online)
	props := state.Properties
	assert.Equal(t, "cooling", props["mode"])
	assert.Equal(t, true, props["power"])
	assert.Equal(t, 17, props["fan_speed"])
	assert.InDelta(t, 25.0, props["target_temp"].(float64), 0.001)
}

func TestCenturyDeviceAdapter_PowerOff_WhenModeOff(t *testing.T) {
	t.Parallel()
	st := &CenturyDeviceState{
		Reg02: &Reg02Decoded{
			Mode: ModeField{Value: "off"},
		},
	}
	a := makeProviderAgent(t, "ct-off")
	addProviderDevice(a, 0x3B, "auto", true, st)

	p := NewCenturyDeviceProvider(a)
	d, err := p.Device("ct-off:3b")
	require.NoError(t, err)
	assert.Equal(t, false, d.State().Properties["power"])
}

func TestCenturyDeviceAdapter_AllRegisters_PopulatesAllProps(t *testing.T) {
	t.Parallel()
	st := &CenturyDeviceState{
		Reg02: &Reg02Decoded{
			Mode:      ModeField{Value: "cooling"},
			Fan:       FieldU8{Value: 17},
			SetpointC: FieldFloat32{Value: 25.0},
		},
		Reg03: &Reg03Decoded{
			TempEvapAC: FieldFloat32{Value: 9.0},
			TempEvapBC: FieldFloat32{Value: 8.5},
		},
		Reg04Read: &Reg04ReadDecoded{
			StatusBits: FieldU8{Value: 0x39},
			OpVal1:     FieldU16{Value: 996},
			TempAC:     FieldFloat32{Value: 25.2},
			OpVal2:     FieldU16{Value: 1248},
		},
	}
	a := makeProviderAgent(t, "ct-full")
	addProviderDevice(a, 0x3B, "auto", true, st)

	p := NewCenturyDeviceProvider(a)
	d, err := p.Device("ct-full:3b")
	require.NoError(t, err)
	props := d.State().Properties

	// 통합 속성
	assert.Equal(t, "cooling", props["mode"])
	assert.Equal(t, true, props["power"])
	assert.Equal(t, 17, props["fan_speed"])
	assert.InDelta(t, 25.0, props["target_temp"].(float64), 0.001)
	assert.InDelta(t, 25.2, props["current_temperature"].(float64), 0.001)
	// Century 전용 속성
	assert.InDelta(t, 9.0, props["temp_evap_a"].(float64), 0.001)
	assert.InDelta(t, 8.5, props["temp_evap_b"].(float64), 0.001)
	assert.Equal(t, uint16(996), props["op_val_1"])
	assert.Equal(t, uint16(1248), props["op_val_2"])
	assert.Equal(t, uint8(0x39), props["status_bits"])
}

func TestCenturyDeviceAdapter_Execute_NotControllable(t *testing.T) {
	t.Parallel()
	a := makeProviderAgent(t, "ct-exec")
	addProviderDevice(a, 0x3B, "auto", true, &CenturyDeviceState{})
	p := NewCenturyDeviceProvider(a)
	d, err := p.Device("ct-exec:3b")
	require.NoError(t, err)

	// Capabilities 는 Device 인터페이스에 포함된다 — 패시브 전용임을 노출한다.
	assert.Equal(t, []string{"passive-monitor"}, d.Capabilities())

	// Execute 는 ControllableDevice 인터페이스에 있다. 어댑터가 해당 인터페이스를
	// 구현하더라도 (REQ-CENTURY-017) 항상 ErrNotControllable 을 반환해야 한다.
	adapter, ok := d.(*centuryDeviceAdapter)
	require.True(t, ok, "expected *centuryDeviceAdapter")
	_, execErr := adapter.Execute(context.Background(), "set_power", map[string]any{"power": true})
	assert.ErrorIs(t, execErr, device.ErrNotControllable)
}

func TestCenturyDeviceAdapter_Name_FallbackWhenLabelEmpty(t *testing.T) {
	t.Parallel()
	a := makeProviderAgent(t, "ct-name")
	now := time.Now()
	dev := &CenturyDevice{SubDevID: 0x3B, Source: "auto", Online: true, LastSeen: now, State: &CenturyDeviceState{}}
	a.devicesMu.Lock()
	a.devices[0x3B] = dev
	a.devicesMu.Unlock()

	p := NewCenturyDeviceProvider(a)
	d, err := p.Device("ct-name:3b")
	require.NoError(t, err)
	assert.Equal(t, "Century indoor 0x3B", d.Name())
}
