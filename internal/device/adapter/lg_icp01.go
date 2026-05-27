package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/device"
)

// 컴파일 타임 인터페이스 체크
var _ device.Device = (*Icp01DeviceAdapter)(nil)

// Icp01DeviceInfo 는 LGCNP 디바이스의 스냅샷 데이터이다.
type Icp01DeviceInfo struct {
	Address    string // 주소 (예: "odu", "81"~"85")
	Label      string // 사람이 읽을 수 있는 라벨 (예: "indoor-1", "outdoor")
	DeviceType string // "HVACR.IDU" 또는 "HVACR.ODU" (v0.18.3)
	Online     bool
	LastSeen   time.Time
	Properties map[string]any // 상태 속성 (toProperties() 결과)
	Source     string         // "config" 또는 "auto"
}

// Icp01DeviceAdapter 는 LGCNP 디바이스를 통합 Device 인터페이스로 래핑한다.
type Icp01DeviceAdapter struct {
	info      Icp01DeviceInfo
	agentName string
}

// NewIcp01Device 는 읽기 전용 LGCNP 디바이스 어댑터를 생성한다.
func NewIcp01Device(agentName string, info Icp01DeviceInfo) *Icp01DeviceAdapter {
	return &Icp01DeviceAdapter{
		info:      info,
		agentName: agentName,
	}
}

// ID returns the globally unique UUID v4 for this device.
//
// SPEC-DEVICE-IDENTITY-001 Phase D (xflowd v1.0 — D-T1, Breaking):
// ID() now returns the UUID (same value as UID()). The legacy composite
// key ("agent_name:local_id") format has been fully removed.
func (a *Icp01DeviceAdapter) ID() string {
	return ResolveAdapterUID(a.agentName, a.info.Address)
}

// UID 는 (agentName, info.Address) 의 글로벌 UUID v4 를 반환한다.
//
// localID 는 Hvacr01Agent 의 emit 경로에서 ResolveDeviceID 호출 시 사용하는
// unitID 와 동일하므로, REST/inventory 와 emit 메시지의 uid 는 같은 UUID 로
// 일치한다.
//
// SPEC-DEVICE-IDENTITY-001 § M1. Phase D (v1.0): ID() == UID().
func (a *Icp01DeviceAdapter) UID() string {
	return ResolveAdapterUID(a.agentName, a.info.Address)
}

func (a *Icp01DeviceAdapter) Name() string {
	if a.info.Label != "" {
		return a.info.Label
	}
	return fmt.Sprintf("lg_icp01 %s %s", a.info.DeviceType, a.info.Address)
}

func (a *Icp01DeviceAdapter) Type() device.DeviceType {
	switch a.info.DeviceType {
	case "HVACR.IDU":
		return device.DeviceTypeIndoor
	case "HVACR.ODU":
		return device.DeviceTypeOutdoor
	default:
		return device.DeviceTypeUnknown
	}
}

// Protocol 은 "lg_icp01" 를 반환한다.
func (a *Icp01DeviceAdapter) Protocol() string {
	return "lg_icp01"
}

func (a *Icp01DeviceAdapter) AgentName() string {
	return a.agentName
}

func (a *Icp01DeviceAdapter) Online() bool {
	return a.info.Online
}

func (a *Icp01DeviceAdapter) LastSeen() time.Time {
	return a.info.LastSeen
}

func (a *Icp01DeviceAdapter) State() device.DeviceState {
	return device.DeviceState{
		Online:     a.info.Online,
		LastSeen:   a.info.LastSeen,
		Properties: a.info.Properties,
	}
}

func (a *Icp01DeviceAdapter) Metadata() device.DeviceMetadata {
	return device.DeviceMetadata{}
}

func (a *Icp01DeviceAdapter) Source() string {
	if a.info.Source != "" {
		return a.info.Source
	}
	return "auto"
}

func (a *Icp01DeviceAdapter) Capabilities() []string {
	return []string{"passive-monitor"}
}

// Execute 는 LGCNP 는 제어를 지원하지 않으므로 항상 ErrNotControllable 을 반환한다.
func (a *Icp01DeviceAdapter) Execute(_ context.Context, _ string, _ map[string]any) (map[string]any, error) {
	return nil, device.ErrNotControllable
}
