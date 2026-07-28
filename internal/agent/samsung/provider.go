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
		Address:       dev.Address.String(),
		DeviceID:      dev.UnitID, // v0.18.7: NasaDevice.UnitID 가 adapter SamsungNasaDeviceInfo.DeviceID (사용자 식별자) 로 매핑
		Name:          dev.Name,
		DeviceType:    dev.Type,
		Online:        dev.Online,
		Ready:         dev.Ready,
		LastSeen:      dev.LastSeen,
		ErrorCount:    dev.ErrorCount,
		DeviceSource:  dev.Source,
		ReportEnabled: dev.ReportEnabled,
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

	// 실외기(ODU): State=nil, Outdoor 에 디코드된 텔레메트리(out_* / outdoor_temperature 등)를
	// 보유한다. 관측된 필드만 ExtraProperties 로 노출해 device.State().Properties 에 평탄화한다
	// — 어댑터가 이를 병합하므로 UI 디바이스 상세에 실외기 상태 속성이 표시된다.
	if dev.Outdoor != nil && len(dev.Outdoor.Fields) > 0 {
		extra := make(map[string]any, len(dev.Outdoor.Fields))
		for k, v := range dev.Outdoor.Fields {
			extra[k] = v
		}
		info.ExtraProperties = extra
	}

	return info
}
