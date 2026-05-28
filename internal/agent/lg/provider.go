package lg

import (
	"fmt"

	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/device/adapter"
)

// LGAPDeviceProvider implements device.DeviceProvider for the LGAP agent.
// It converts LGAPDevice instances to the unified Device interface using adapters.
type LGAPDeviceProvider struct {
	agent *LGAPAgent
}

// Compile-time interface check.
var _ device.DeviceProvider = (*LGAPDeviceProvider)(nil)

// NewLGAPDeviceProvider creates a DeviceProvider wrapping a LGAPAgent.
func NewLGAPDeviceProvider(agent *LGAPAgent) *LGAPDeviceProvider {
	return &LGAPDeviceProvider{agent: agent}
}

// Devices returns all devices managed by this agent as unified Device instances.
func (p *LGAPDeviceProvider) Devices() []device.Device {
	lgapDevices := p.agent.ListDevices()
	result := make([]device.Device, 0, len(lgapDevices))
	agentName := p.agent.Name()

	for i := range lgapDevices {
		dev := &lgapDevices[i]
		info := lgapDeviceToInfo(dev)

		// LGAP 는 실내기만 지원하므로 모두 ControllableDevice 로 생성
		executor := p.createExecutor(dev.Zone)
		result = append(result, adapter.NewControllableSamsungNasaDevice(agentName, info, executor))
	}
	return result
}

// Device returns a specific device by its global ID ("agentName:zone").
func (p *LGAPDeviceProvider) Device(id string) (device.Device, error) {
	agentName := p.agent.Name()
	prefix := agentName + ":"
	if len(id) <= len(prefix) || id[:len(prefix)] != prefix {
		return nil, device.ErrDeviceNotFound
	}
	zoneStr := id[len(prefix):]

	lgapDevices := p.agent.ListDevices()
	for i := range lgapDevices {
		dev := &lgapDevices[i]
		if formatZone(dev.Zone) == zoneStr {
			info := lgapDeviceToInfo(dev)
			executor := p.createExecutor(dev.Zone)
			return adapter.NewControllableSamsungNasaDevice(agentName, info, executor), nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

// createExecutor creates a CommandExecutor that delegates to LGAPAgent.Process.
func (p *LGAPDeviceProvider) createExecutor(zone byte) adapter.CommandExecutor {
	return newLGAPExecutor(p.agent, zone)
}

// formatZone formats a zone byte as "XX" (2-digit hex).
// This is the canonical format used in device IDs.
func formatZone(zone byte) string {
	return fmt.Sprintf("%02X", zone)
}

// lgapDeviceToInfo converts a LGAPDevice to adapter.SamsungNasaDeviceInfo.
// LGAP 디바이스는 SamsungNasaDeviceInfo 어댑터를 재사용한다 (필드 호환).
func lgapDeviceToInfo(dev *LGAPDevice) adapter.SamsungNasaDeviceInfo {
	info := adapter.SamsungNasaDeviceInfo{
		Address:      formatZone(dev.Zone),
		DeviceID:     dev.UnitID,
		Name:         dev.Name,
		DeviceType:   "HVACR.IDU", // LGAP 는 실내기만 지원 (v0.18.3)
		Online:       dev.Online,
		Ready:        dev.Online, // LGAP 는 온라인이면 Ready
		LastSeen:     dev.LastSeen,
		ErrorCount:   dev.ErrorCount,
		Protocol:     "lgap",
		DeviceSource: dev.Source,
	}

	if dev.State != nil {
		info.Power = &dev.State.Power
		info.Mode = &dev.State.Mode
		targetTemp := float32(dev.State.TargetTemp)
		info.TargetTemp = &targetTemp
		info.CurrentTemp = &dev.State.RoomTemp
		info.FanSpeed = &dev.State.FanSpeed
		errorCode := uint16(dev.State.ErrorCode)
		info.ErrorCode = &errorCode
		info.ExtraProperties = map[string]any{
			"swing_auto":           dev.State.SwingAuto,
			"locked":               dev.State.Locked,
			"plasma":               dev.State.Plasma,
			"pipe_in_temperature":  dev.State.PipeInTemp,
			"pipe_out_temperature": dev.State.PipeOutTemp,
			"zone_load":            dev.State.ZoneLoad,
			"zone_power":           dev.State.ZonePower,
		}
	}

	return info
}
