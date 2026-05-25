package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/device"
)

// 컴파일 타임 인터페이스 체크
var _ device.Device = (*LGCNPDeviceAdapter)(nil)

// LGCNPDeviceInfo 는 LGCNP 디바이스의 스냅샷 데이터이다.
type LGCNPDeviceInfo struct {
	Address    string // 주소 (예: "odu", "81"~"85")
	Label      string // 사람이 읽을 수 있는 라벨 (예: "indoor-1", "outdoor")
	DeviceType string // "HVACR.IDU" 또는 "HVACR.ODU" (v0.18.3)
	Online     bool
	LastSeen   time.Time
	Properties map[string]any // 상태 속성 (toProperties() 결과)
	Source     string         // "config" 또는 "auto"
}

// LGCNPDeviceAdapter 는 LGCNP 디바이스를 통합 Device 인터페이스로 래핑한다.
type LGCNPDeviceAdapter struct {
	info      LGCNPDeviceInfo
	agentName string
}

// NewLGCNPDevice 는 읽기 전용 LGCNP 디바이스 어댑터를 생성한다.
func NewLGCNPDevice(agentName string, info LGCNPDeviceInfo) *LGCNPDeviceAdapter {
	return &LGCNPDeviceAdapter{
		info:      info,
		agentName: agentName,
	}
}

func (a *LGCNPDeviceAdapter) ID() string {
	return fmt.Sprintf("%s:%s", a.agentName, a.info.Address)
}

// UID 는 (agentName, info.Address) 의 글로벌 UUID v4 를 반환한다.
//
// localID 는 LGCNPAgent 의 emit 경로에서 ResolveDeviceID 호출 시 사용하는
// unitID 와 동일하므로, REST/inventory 와 emit 메시지의 uid 는 같은 UUID 로
// 일치한다. DeviceIDRepository 미설정/에러 시 빈 문자열 (Phase A graceful
// degradation; xflowd_device_uid_missing_total 메트릭으로 추적).
//
// SPEC-DEVICE-IDENTITY-001 § M1.
func (a *LGCNPDeviceAdapter) UID() string {
	return ResolveAdapterUID(a.agentName, a.info.Address)
}

func (a *LGCNPDeviceAdapter) Name() string {
	if a.info.Label != "" {
		return a.info.Label
	}
	return fmt.Sprintf("LGCNP %s %s", a.info.DeviceType, a.info.Address)
}

func (a *LGCNPDeviceAdapter) Type() device.DeviceType {
	switch a.info.DeviceType {
	case "HVACR.IDU":
		return device.DeviceTypeIndoor
	case "HVACR.ODU":
		return device.DeviceTypeOutdoor
	default:
		return device.DeviceTypeUnknown
	}
}

// Protocol 은 "lgcnp" 를 반환한다.
func (a *LGCNPDeviceAdapter) Protocol() string {
	return "lgcnp"
}

func (a *LGCNPDeviceAdapter) AgentName() string {
	return a.agentName
}

func (a *LGCNPDeviceAdapter) Online() bool {
	return a.info.Online
}

func (a *LGCNPDeviceAdapter) LastSeen() time.Time {
	return a.info.LastSeen
}

func (a *LGCNPDeviceAdapter) State() device.DeviceState {
	return device.DeviceState{
		Online:     a.info.Online,
		LastSeen:   a.info.LastSeen,
		Properties: a.info.Properties,
	}
}

func (a *LGCNPDeviceAdapter) Metadata() device.DeviceMetadata {
	return device.DeviceMetadata{}
}

func (a *LGCNPDeviceAdapter) Source() string {
	if a.info.Source != "" {
		return a.info.Source
	}
	return "auto"
}

func (a *LGCNPDeviceAdapter) Capabilities() []string {
	return []string{"passive-monitor"}
}

// Execute 는 LGCNP 는 제어를 지원하지 않으므로 항상 ErrNotControllable 을 반환한다.
func (a *LGCNPDeviceAdapter) Execute(_ context.Context, _ string, _ map[string]any) (map[string]any, error) {
	return nil, device.ErrNotControllable
}
