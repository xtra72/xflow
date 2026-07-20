package lg

import (
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/device/adapter"
)

// Hvacr02DeviceProvider 는 LG HVACR-02 에이전트의 device.DeviceProvider 구현이다.
type Hvacr02DeviceProvider struct {
	agent *Hvacr02Agent
}

// Compile-time interface check.
var _ device.DeviceProvider = (*Hvacr02DeviceProvider)(nil)

// NewHvacr02DeviceProvider 는 LG HVACR-02 에이전트를 래핑하는 DeviceProvider 를 생성한다.
func NewHvacr02DeviceProvider(agent *Hvacr02Agent) *Hvacr02DeviceProvider {
	return &Hvacr02DeviceProvider{agent: agent}
}

// Devices 는 에이전트가 관리하는 모든 디바이스를 통합 Device 인터페이스로 반환한다.
// control_enabled 인 indoor 디바이스는 ControllableDevice 로 반환된다.
func (p *Hvacr02DeviceProvider) Devices() []device.Device {
	icp02Devices := p.agent.ListDevices()
	result := make([]device.Device, 0, len(icp02Devices))
	agentName := p.agent.Name()
	controlEnabled := p.agent.hvacr02Config.ControlEnabled

	for i := range icp02Devices {
		dev := &icp02Devices[i]
		info := icp02DeviceToInfo(dev)
		if controlEnabled && dev.Type == "HVACR.IDU" {
			executor := newHvacr02Executor(p.agent, dev.Address)
			result = append(result, adapter.NewControllableLGIcp02Device(agentName, info, executor))
		} else {
			result = append(result, adapter.NewLGIcp02Device(agentName, info))
		}
	}
	return result
}

// Device 는 글로벌 ID ("agentName:address") 로 특정 디바이스를 반환한다.
func (p *Hvacr02DeviceProvider) Device(id string) (device.Device, error) {
	agentName := p.agent.Name()
	prefix := agentName + ":"
	if len(id) <= len(prefix) || id[:len(prefix)] != prefix {
		return nil, device.ErrDeviceNotFound
	}
	addrStr := id[len(prefix):]

	icp02Devices := p.agent.ListDevices()
	controlEnabled := p.agent.hvacr02Config.ControlEnabled
	for i := range icp02Devices {
		dev := &icp02Devices[i]
		if dev.Address == addrStr {
			info := icp02DeviceToInfo(dev)
			if controlEnabled && dev.Type == "HVACR.IDU" {
				executor := newHvacr02Executor(p.agent, dev.Address)
				return adapter.NewControllableLGIcp02Device(agentName, info, executor), nil
			}
			return adapter.NewLGIcp02Device(agentName, info), nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

// icp02DeviceToInfo 는 Icp02Device 를 adapter.LGIcp02DeviceInfo 로 변환한다.
func icp02DeviceToInfo(dev *Icp02Device) adapter.LGIcp02DeviceInfo {
	info := adapter.LGIcp02DeviceInfo{
		Address:       dev.Address,
		Label:         dev.Label,
		DeviceType:    dev.Type,
		Online:        dev.Online,
		LastSeen:      dev.LastSeen,
		Source:        dev.Source,
		ReportEnabled: dev.ReportEnabled,
	}
	if dev.State != nil {
		info.Properties = dev.State.toProperties(dev.Type)
	}
	return info
}
