package lg

import (
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/device/adapter"
)

// ---------------------------------------------------------------------------
// LG HVACR-03 DeviceProvider
//
// SPEC-LG-HVACR-003 § M8. control_enabled 가 활성일 때만 ControllableDevice 로
// 노출하여, 제어가 꺼진 에이전트의 디바이스가 대시보드에서 조작 가능해 보이는 일을
// 막는다.
// ---------------------------------------------------------------------------

// Hvacr03DeviceProvider 는 Hvacr03Agent 의 device.DeviceProvider 구현이다.
type Hvacr03DeviceProvider struct {
	agent *Hvacr03Agent
}

// Compile-time interface check.
var _ device.DeviceProvider = (*Hvacr03DeviceProvider)(nil)

// NewHvacr03DeviceProvider 는 에이전트를 래핑하는 DeviceProvider 를 생성한다.
func NewHvacr03DeviceProvider(a *Hvacr03Agent) *Hvacr03DeviceProvider {
	return &Hvacr03DeviceProvider{agent: a}
}

// Devices 는 관리 중인 모든 디바이스를 통합 Device 인터페이스로 반환한다.
func (p *Hvacr03DeviceProvider) Devices() []device.Device {
	devices := p.agent.ListDevices()
	agentName := p.agent.Name()

	p.agent.mu.RLock()
	controlEnabled := p.agent.hvacr03Config.ControlEnabled
	opts := p.agent.projectionOptsLocked()
	p.agent.mu.RUnlock()

	result := make([]device.Device, 0, len(devices))
	for i := range devices {
		result = append(result, p.wrap(agentName, &devices[i], controlEnabled, opts))
	}
	return result
}

// Device 는 글로벌 ID ("agentName:address") 로 특정 디바이스를 반환한다.
func (p *Hvacr03DeviceProvider) Device(id string) (device.Device, error) {
	agentName := p.agent.Name()
	prefix := agentName + ":"
	if len(id) <= len(prefix) || id[:len(prefix)] != prefix {
		return nil, device.ErrDeviceNotFound
	}
	addr := id[len(prefix):]

	devices := p.agent.ListDevices()

	p.agent.mu.RLock()
	controlEnabled := p.agent.hvacr03Config.ControlEnabled
	opts := p.agent.projectionOptsLocked()
	p.agent.mu.RUnlock()

	for i := range devices {
		if devices[i].Address == addr {
			return p.wrap(agentName, &devices[i], controlEnabled, opts), nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

// wrap 은 내부 디바이스를 어댑터로 감싼다.
func (p *Hvacr03DeviceProvider) wrap(agentName string, dev *PmbusDevice, controlEnabled bool, opts pmbusProjectionOpts) device.Device {
	info := pmbusDeviceToInfo(dev, opts)
	if controlEnabled {
		executor := newHvacr03Executor(p.agent, dev.Address)
		return adapter.NewControllableLGPmbusDevice(agentName, info, executor)
	}
	return adapter.NewLGPmbusDevice(agentName, info)
}

// pmbusDeviceToInfo 는 PmbusDevice 를 어댑터 DTO 로 변환한다.
func pmbusDeviceToInfo(dev *PmbusDevice, opts pmbusProjectionOpts) adapter.LGPmbusDeviceInfo {
	info := adapter.LGPmbusDeviceInfo{
		Address:       dev.Address,
		Label:         dev.Label,
		DeviceType:    dev.Type,
		Online:        dev.Online,
		LastSeen:      dev.LastSeen,
		Source:        dev.Source,
		ReportEnabled: dev.ReportEnabled,
	}
	if dev.State != nil {
		info.Properties = dev.State.toProperties(dev.Type, opts)
	}
	return info
}
