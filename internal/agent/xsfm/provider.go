package xsfm

import (
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/device/adapter"
)

// XSFMDeviceProvider implements device.DeviceProvider for the facility agent.
// It converts roster Device instances into unified device.Device adapters, mirroring
// samsung.Hvacr01DeviceProvider / lg.LGAPDeviceProvider (REQ-XSFM-001-08).
type XSFMDeviceProvider struct {
	agent *XSFMAgent
}

// Compile-time interface check.
var _ device.DeviceProvider = (*XSFMDeviceProvider)(nil)

// NewXSFMDeviceProvider creates a DeviceProvider wrapping an XSFMAgent.
func NewXSFMDeviceProvider(agent *XSFMAgent) *XSFMDeviceProvider {
	return &XSFMDeviceProvider{agent: agent}
}

// Devices returns all roster devices as controllable unified device.Device instances.
// Every facility is controllable (2-axis: set_power + set_fan_speed), so each device
// is wrapped with an executor bound to its device_id.
func (p *XSFMDeviceProvider) Devices() []device.Device {
	apDevices := p.agent.ListDevices()
	result := make([]device.Device, 0, len(apDevices))
	agentName := p.agent.Name()

	for i := range apDevices {
		dev := &apDevices[i]
		info := xsfmDeviceToInfo(dev)
		executor := adapter.NewXSFMExecutor(p.agent, dev.DeviceID)
		result = append(result, adapter.NewControllableXSFMDevice(agentName, info, executor))
	}
	return result
}

// Device returns a specific device by its global UUID (device.Device.ID()).
// After SPEC-DEVICE-IDENTITY-001 Phase D the device ID is a UUID, so lookup matches
// on the resolved adapter ID rather than a composite prefix.
func (p *XSFMDeviceProvider) Device(id string) (device.Device, error) {
	if id == "" {
		return nil, device.ErrDeviceNotFound
	}
	for _, d := range p.Devices() {
		if d.ID() == id {
			return d, nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

// xsfmDeviceToInfo converts a roster Device into adapter.XSFMDeviceInfo.
// Observation-gating parity with the agent: Power / FanSpeed are populated only when the
// corresponding axis has actually been observed (samsung observed-bitmask pattern). The
// adapter applies the additional "fan_speed valid only when power is on" gate in State().
func xsfmDeviceToInfo(dev *Device) adapter.XSFMDeviceInfo {
	info := adapter.XSFMDeviceInfo{
		DeviceID: dev.DeviceID,
		Name:     dev.Name,
		GroupID:  dev.GroupID,
		Online:   dev.Online,
		LastSeen: dev.LastSeen,
		Source:   dev.Source,
	}
	if dev.isObserved(observedPower) {
		power := dev.Power
		info.Power = &power
	}
	if dev.isObserved(observedFanSpeed) {
		fanSpeed := dev.FanSpeed
		info.FanSpeed = &fanSpeed
	}
	return info
}
