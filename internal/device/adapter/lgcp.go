package adapter

import (
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/device"
)

// Compile-time interface check.
var _ device.Device = (*LGCPDeviceAdapter)(nil)

// LGCPDeviceInfo 는 LGCP 디바이스의 스냅샷 데이터이다.
type LGCPDeviceInfo struct {
	Address    string         // 주소 hex (예: "44550067")
	Label      string         // 사람이 읽을 수 있는 라벨 (예: "indoor-3")
	DeviceType string         // "indoor", "controller", "unknown"
	Online     bool
	LastSeen   time.Time
	Properties map[string]any // 상태 속성 (LGCPDeviceState.toProperties() 결과)
	Source     string         // "config" 또는 "auto"
}

// LGCPDeviceAdapter 는 LGCP 디바이스를 통합 Device 인터페이스로 래핑한다.
type LGCPDeviceAdapter struct {
	info      LGCPDeviceInfo
	agentName string
}

// NewLGCPDevice 는 LGCP 디바이스 어댑터를 생성한다.
func NewLGCPDevice(agentName string, info LGCPDeviceInfo) *LGCPDeviceAdapter {
	return &LGCPDeviceAdapter{
		info:      info,
		agentName: agentName,
	}
}

func (a *LGCPDeviceAdapter) ID() string {
	return fmt.Sprintf("%s:%s", a.agentName, a.info.Address)
}

func (a *LGCPDeviceAdapter) Name() string {
	if a.info.Label != "" {
		return a.info.Label
	}
	return fmt.Sprintf("LGCP %s %s", a.info.DeviceType, a.info.Address)
}

func (a *LGCPDeviceAdapter) Type() device.DeviceType {
	switch a.info.DeviceType {
	case "indoor":
		return device.DeviceTypeIndoor
	case "controller":
		return device.DeviceTypeController
	default:
		return device.DeviceTypeUnknown
	}
}

func (a *LGCPDeviceAdapter) Protocol() string {
	return "lgcp"
}

func (a *LGCPDeviceAdapter) AgentName() string {
	return a.agentName
}

func (a *LGCPDeviceAdapter) Online() bool {
	return a.info.Online
}

func (a *LGCPDeviceAdapter) LastSeen() time.Time {
	return a.info.LastSeen
}

func (a *LGCPDeviceAdapter) State() device.DeviceState {
	return device.DeviceState{
		Online:     a.info.Online,
		LastSeen:   a.info.LastSeen,
		Properties: a.info.Properties,
	}
}

func (a *LGCPDeviceAdapter) Metadata() device.DeviceMetadata {
	return device.DeviceMetadata{}
}

func (a *LGCPDeviceAdapter) Source() string {
	if a.info.Source != "" {
		return a.info.Source
	}
	return "auto"
}

func (a *LGCPDeviceAdapter) Capabilities() []string {
	return []string{"passive-monitor"}
}
