package samsung

import (
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/device/adapter"
)

// NASADeviceProvider implements device.DeviceProvider for the NASA agent.
// It converts NASADevice instances to the unified Device interface using adapters.
type NASADeviceProvider struct {
	agent *NASAAgent
}

// Compile-time interface check.
var _ device.DeviceProvider = (*NASADeviceProvider)(nil)

// NewNASADeviceProvider creates a DeviceProvider wrapping a NASAAgent.
func NewNASADeviceProvider(agent *NASAAgent) *NASADeviceProvider {
	return &NASADeviceProvider{agent: agent}
}

// Devices returns all devices managed by this agent as unified Device instances.
func (p *NASADeviceProvider) Devices() []device.Device {
	nasaDevices := p.agent.ListDevices()
	result := make([]device.Device, 0, len(nasaDevices))
	agentName := p.agent.Name()

	for i := range nasaDevices {
		dev := &nasaDevices[i]
		info := nasaDeviceToInfo(dev)

		if dev.Type == "HVACR.IDU" {
			executor := p.createExecutor(dev.Address)
			result = append(result, adapter.NewControllableNASADevice(agentName, info, executor))
		} else {
			result = append(result, adapter.NewNASADevice(agentName, info))
		}
	}
	return result
}

// Device returns a specific device by its global ID ("agentName:address").
func (p *NASADeviceProvider) Device(id string) (device.Device, error) {
	agentName := p.agent.Name()
	prefix := agentName + ":"
	if len(id) <= len(prefix) || id[:len(prefix)] != prefix {
		return nil, device.ErrDeviceNotFound
	}
	addrStr := id[len(prefix):]

	nasaDevices := p.agent.ListDevices()
	for i := range nasaDevices {
		dev := &nasaDevices[i]
		if dev.Address.String() == addrStr {
			info := nasaDeviceToInfo(dev)
			if dev.Type == "HVACR.IDU" {
				executor := p.createExecutor(dev.Address)
				return adapter.NewControllableNASADevice(agentName, info, executor), nil
			}
			return adapter.NewNASADevice(agentName, info), nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

// createExecutor creates a CommandExecutor that delegates to NASAAgent.Process.
func (p *NASADeviceProvider) createExecutor(addr NASAAddress) adapter.CommandExecutor {
	return newNASAExecutor(p.agent, addr)
}

// nasaDeviceToInfo converts a NASADevice to adapter.NASADeviceInfo.
func nasaDeviceToInfo(dev *NASADevice) adapter.NASADeviceInfo {
	info := adapter.NASADeviceInfo{
		Address:      dev.Address.String(),
		DeviceID:     dev.DeviceID,
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
