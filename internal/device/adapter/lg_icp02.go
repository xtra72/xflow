package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/device"
)

// Compile-time interface checks.
var (
	_ device.Device             = (*LGIcp02DeviceAdapter)(nil)
	_ device.ControllableDevice = (*LGIcp02DeviceAdapter)(nil)
)

// LGIcp02DeviceInfo 는 LGCP 디바이스의 스냅샷 데이터이다.
type LGIcp02DeviceInfo struct {
	Address    string // 주소 hex (예: "44550067")
	Label      string // 사람이 읽을 수 있는 라벨 (예: "indoor-3")
	DeviceType string // "HVACR.IDU", "controller", "unknown" (v0.18.3)
	Online     bool
	LastSeen   time.Time
	Properties map[string]any // 상태 속성 (LGCPDeviceState.toProperties() 결과)
	Source     string         // "config" 또는 "auto"
}

// LGIcp02DeviceAdapter 는 LGCP 디바이스를 통합 Device 인터페이스로 래핑한다.
type LGIcp02DeviceAdapter struct {
	info      LGIcp02DeviceInfo
	agentName string
	executor  CommandExecutor      // nil for non-controllable devices
	commands  []device.CommandSpec // nil for non-controllable devices
}

// NewLGIcp02Device 는 읽기 전용 LGCP 디바이스 어댑터를 생성한다.
func NewLGIcp02Device(agentName string, info LGIcp02DeviceInfo) *LGIcp02DeviceAdapter {
	return &LGIcp02DeviceAdapter{
		info:      info,
		agentName: agentName,
	}
}

// NewControllableLGIcp02Device 는 제어 가능한 LGCP 디바이스 어댑터를 생성한다.
func NewControllableLGIcp02Device(agentName string, info LGIcp02DeviceInfo, executor CommandExecutor) *LGIcp02DeviceAdapter {
	return &LGIcp02DeviceAdapter{
		info:      info,
		agentName: agentName,
		executor:  executor,
		commands:  lgIcp02IndoorCommandSpecs(),
	}
}

// ID returns the globally unique UUID v4 for this device.
//
// SPEC-DEVICE-IDENTITY-001 Phase D (xflowd v1.0 — D-T1, Breaking):
// ID() now returns the UUID (same value as UID()). The legacy composite
// key ("agent_name:local_id") format has been fully removed.
func (a *LGIcp02DeviceAdapter) ID() string {
	return ResolveAdapterUID(a.agentName, a.info.Address)
}

// UID 는 (agentName, info.Address) 의 글로벌 UUID v4 를 반환한다.
//
// localID 는 LGCPAgent 가 ResolveDeviceID 호출 시 사용하는 unitID (디바이스
// 주소 hex) 와 동일하다.
//
// SPEC-DEVICE-IDENTITY-001 § M1. Phase D (v1.0): ID() == UID().
func (a *LGIcp02DeviceAdapter) UID() string {
	return ResolveAdapterUID(a.agentName, a.info.Address)
}

func (a *LGIcp02DeviceAdapter) Name() string {
	if a.info.Label != "" {
		return a.info.Label
	}
	return fmt.Sprintf("lg_icp02 %s %s", a.info.DeviceType, a.info.Address)
}

func (a *LGIcp02DeviceAdapter) Type() device.DeviceType {
	switch a.info.DeviceType {
	case "HVACR.IDU":
		return device.DeviceTypeIndoor
	case "controller":
		return device.DeviceTypeController
	default:
		return device.DeviceTypeUnknown
	}
}

func (a *LGIcp02DeviceAdapter) Protocol() string {
	return "lg_icp02"
}

func (a *LGIcp02DeviceAdapter) AgentName() string {
	return a.agentName
}

func (a *LGIcp02DeviceAdapter) Online() bool {
	return a.info.Online
}

func (a *LGIcp02DeviceAdapter) LastSeen() time.Time {
	return a.info.LastSeen
}

func (a *LGIcp02DeviceAdapter) State() device.DeviceState {
	return device.DeviceState{
		Online:     a.info.Online,
		LastSeen:   a.info.LastSeen,
		Properties: a.info.Properties,
	}
}

func (a *LGIcp02DeviceAdapter) Metadata() device.DeviceMetadata {
	return device.DeviceMetadata{}
}

func (a *LGIcp02DeviceAdapter) Source() string {
	if a.info.Source != "" {
		return a.info.Source
	}
	return "auto"
}

func (a *LGIcp02DeviceAdapter) Capabilities() []string {
	if a.executor != nil && a.info.DeviceType == "HVACR.IDU" {
		return []string{"passive-monitor", "set_power", "target_temperature", "set_fan_speed", "set_mode", "set_multiple"}
	}
	return []string{"passive-monitor"}
}

// Execute 는 제어 명령을 실행한다. executor 가 없으면 ErrNotControllable 반환.
func (a *LGIcp02DeviceAdapter) Execute(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
	if a.executor == nil {
		return nil, device.ErrNotControllable
	}
	return a.executor(ctx, command, params)
}

// Commands 는 이 디바이스에 사용 가능한 명령 목록을 반환한다.
func (a *LGIcp02DeviceAdapter) Commands() []device.CommandSpec {
	return a.commands
}

// lgIcp02IndoorCommandSpecs 는 LGCP 실내기 제어 명령 스펙을 생성한다.
func lgIcp02IndoorCommandSpecs() []device.CommandSpec {
	minTemp := 15.0
	maxTemp := 30.0

	return []device.CommandSpec{
		{
			Name:        "set_power",
			Description: "실내기 전원 ON/OFF",
			Params: []device.ParamSpec{
				{Name: "power", Type: "bool", Required: true},
			},
		},
		{
			Name:        "target_temperature",
			Description: "설정 온도 변경 (15~30도)",
			Params: []device.ParamSpec{
				// target_temp: NASA/LGAP/LGCP 공통 컨벤션. 이전엔 'temperature' 였으나
				// LGCPAgent.buildThermostatPayloadForCommand 가 params["target_temperature"] 를
				// 요구하여 이름 불일치로 ErrLGCPMissingParam 발생. (참조: lgcp_agent.go:703)
				{Name: "target_temperature", Type: "float", Required: true, Min: &minTemp, Max: &maxTemp},
			},
		},
		{
			Name:        "set_fan_speed",
			Description: "풍량 변경",
			Params: []device.ParamSpec{
				{Name: "fan_speed", Type: "enum", Required: true, Enum: []string{"low", "medium", "high", "turbo", "auto"}},
			},
		},
		{
			Name:        "set_mode",
			Description: "운전모드 변경",
			Params: []device.ParamSpec{
				{Name: "mode", Type: "enum", Required: true, Enum: []string{"cooling", "dehumidify", "fan", "auto", "heating"}},
			},
		},
		{
			Name:        "set_multiple",
			Description: "여러 설정을 동시 변경",
			Params: []device.ParamSpec{
				{Name: "power", Type: "bool"},
				// target_temp: lgcp_control.go buildControlPayload 가 params["target_temperature"] 를 사용
				{Name: "target_temperature", Type: "float", Min: &minTemp, Max: &maxTemp},
				{Name: "fan_speed", Type: "enum", Enum: []string{"low", "medium", "high", "turbo", "auto"}},
				{Name: "mode", Type: "enum", Enum: []string{"cooling", "dehumidify", "fan", "auto", "heating"}},
			},
		},
	}
}
