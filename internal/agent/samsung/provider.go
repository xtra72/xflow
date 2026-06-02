package samsung

import (
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/device/adapter"
)

// Hvacr01DeviceProvider implements device.DeviceProvider for the NASA agent.
// It converts NasaDevice instances to the unified Device interface using adapters.
type Hvacr01DeviceProvider struct {
	agent *Hvacr01Agent
}

// Compile-time interface check.
var _ device.DeviceProvider = (*Hvacr01DeviceProvider)(nil)

// NewHvacr01DeviceProvider creates a DeviceProvider wrapping a Hvacr01Agent.
func NewHvacr01DeviceProvider(agent *Hvacr01Agent) *Hvacr01DeviceProvider {
	return &Hvacr01DeviceProvider{agent: agent}
}

// Devices returns all devices managed by this agent as unified Device instances.
func (p *Hvacr01DeviceProvider) Devices() []device.Device {
	hvacr01Devices := p.agent.ListDevices()
	result := make([]device.Device, 0, len(hvacr01Devices))
	agentName := p.agent.Name()

	for i := range hvacr01Devices {
		dev := &hvacr01Devices[i]
		info := hvacr01DeviceToInfo(dev)

		if dev.Type == "HVACR.IDU" {
			executor := p.createExecutor(dev.Address)
			result = append(result, adapter.NewControllableSamsungNasaDevice(agentName, info, executor))
		} else {
			result = append(result, adapter.NewSamsungNasaDevice(agentName, info))
		}
	}
	return result
}

// Device returns a specific device by its global ID ("agentName:address").
func (p *Hvacr01DeviceProvider) Device(id string) (device.Device, error) {
	agentName := p.agent.Name()
	prefix := agentName + ":"
	if len(id) <= len(prefix) || id[:len(prefix)] != prefix {
		return nil, device.ErrDeviceNotFound
	}
	addrStr := id[len(prefix):]

	hvacr01Devices := p.agent.ListDevices()
	for i := range hvacr01Devices {
		dev := &hvacr01Devices[i]
		if dev.Address.String() == addrStr {
			info := hvacr01DeviceToInfo(dev)
			if dev.Type == "HVACR.IDU" {
				executor := p.createExecutor(dev.Address)
				return adapter.NewControllableSamsungNasaDevice(agentName, info, executor), nil
			}
			return adapter.NewSamsungNasaDevice(agentName, info), nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

// createExecutor creates a CommandExecutor that delegates to Hvacr01Agent.Process.
func (p *Hvacr01DeviceProvider) createExecutor(addr NasaAddress) adapter.CommandExecutor {
	return newHvacr01Executor(p.agent, addr)
}

// hvacr01DeviceToInfo converts a NasaDevice to adapter.SamsungNasaDeviceInfo.
func hvacr01DeviceToInfo(dev *NasaDevice) adapter.SamsungNasaDeviceInfo {
	info := adapter.SamsungNasaDeviceInfo{
		Address:      dev.Address.String(),
		DeviceID:     dev.UnitID, // v0.18.7: NasaDevice.UnitID 가 adapter SamsungNasaDeviceInfo.DeviceID (사용자 식별자) 로 매핑
		Name:         dev.Name,
		DeviceType:   dev.Type,
		Online:       dev.Online,
		Ready:        dev.Ready,
		LastSeen:     dev.LastSeen,
		ErrorCount:   dev.ErrorCount,
		DeviceSource: dev.Source,
	}

	if dev.State != nil {
		info.Power = &dev.State.Power
		info.Mode = &dev.State.Mode
		info.TargetTemp = &dev.State.TargetTemp
		info.CurrentTemp = &dev.State.CurrentTemp
		info.FanSpeed = &dev.State.FanSpeed
		info.SwingVertical = &dev.State.SwingVertical
		info.FilterAlarm = &dev.State.FilterAlarm
		info.ErrorCode = &dev.State.ErrorCode
	}

	return info
}
