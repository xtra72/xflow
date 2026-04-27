package lg

import (
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/device/adapter"
)

// LGCPDeviceProvider 는 LGCP 에이전트의 device.DeviceProvider 구현이다.
type LGCPDeviceProvider struct {
	agent *LGCPAgent
}

// Compile-time interface check.
var _ device.DeviceProvider = (*LGCPDeviceProvider)(nil)

// NewLGCPDeviceProvider 는 LGCP 에이전트를 래핑하는 DeviceProvider 를 생성한다.
func NewLGCPDeviceProvider(agent *LGCPAgent) *LGCPDeviceProvider {
	return &LGCPDeviceProvider{agent: agent}
}

// Devices 는 에이전트가 관리하는 모든 디바이스를 통합 Device 인터페이스로 반환한다.
// control_enabled 인 indoor 디바이스는 ControllableDevice 로 반환된다.
func (p *LGCPDeviceProvider) Devices() []device.Device {
	lgcpDevices := p.agent.ListDevices()
	result := make([]device.Device, 0, len(lgcpDevices))
	agentName := p.agent.Name()
	controlEnabled := p.agent.lgcpConfig.ControlEnabled

	for i := range lgcpDevices {
		dev := &lgcpDevices[i]
		info := lgcpDeviceToInfo(dev)
		if controlEnabled && dev.Type == "indoor" {
			executor := newLGCPExecutor(p.agent, dev.Address)
			result = append(result, adapter.NewControllableLGCPDevice(agentName, info, executor))
		} else {
			result = append(result, adapter.NewLGCPDevice(agentName, info))
		}
	}
	return result
}

// Device 는 글로벌 ID ("agentName:address") 로 특정 디바이스를 반환한다.
func (p *LGCPDeviceProvider) Device(id string) (device.Device, error) {
	agentName := p.agent.Name()
	prefix := agentName + ":"
	if len(id) <= len(prefix) || id[:len(prefix)] != prefix {
		return nil, device.ErrDeviceNotFound
	}
	addrStr := id[len(prefix):]

	lgcpDevices := p.agent.ListDevices()
	controlEnabled := p.agent.lgcpConfig.ControlEnabled
	for i := range lgcpDevices {
		dev := &lgcpDevices[i]
		if dev.Address == addrStr {
			info := lgcpDeviceToInfo(dev)
			if controlEnabled && dev.Type == "indoor" {
				executor := newLGCPExecutor(p.agent, dev.Address)
				return adapter.NewControllableLGCPDevice(agentName, info, executor), nil
			}
			return adapter.NewLGCPDevice(agentName, info), nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

// lgcpDeviceToInfo 는 LGCPDevice 를 adapter.LGCPDeviceInfo 로 변환한다.
func lgcpDeviceToInfo(dev *LGCPDevice) adapter.LGCPDeviceInfo {
	info := adapter.LGCPDeviceInfo{
		Address:    dev.Address,
		Label:      dev.Label,
		DeviceType: dev.Type,
		Online:     dev.Online,
		LastSeen:   dev.LastSeen,
		Source:     dev.Source,
	}
	if dev.State != nil {
		info.Properties = dev.State.toProperties(dev.Type)
	}
	return info
}
