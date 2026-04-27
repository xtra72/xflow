package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/device"
)

// Compile-time interface checks.
var (
	_ device.Device             = (*LGCPDeviceAdapter)(nil)
	_ device.ControllableDevice = (*LGCPDeviceAdapter)(nil)
)

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
	executor  CommandExecutor      // nil for non-controllable devices
	commands  []device.CommandSpec // nil for non-controllable devices
}

// NewLGCPDevice 는 읽기 전용 LGCP 디바이스 어댑터를 생성한다.
func NewLGCPDevice(agentName string, info LGCPDeviceInfo) *LGCPDeviceAdapter {
	return &LGCPDeviceAdapter{
		info:      info,
		agentName: agentName,
	}
}

// NewControllableLGCPDevice 는 제어 가능한 LGCP 디바이스 어댑터를 생성한다.
func NewControllableLGCPDevice(agentName string, info LGCPDeviceInfo, executor CommandExecutor) *LGCPDeviceAdapter {
	return &LGCPDeviceAdapter{
		info:      info,
		agentName: agentName,
		executor:  executor,
		commands:  lgcpIndoorCommandSpecs(),
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
	if a.executor != nil && a.info.DeviceType == "indoor" {
		return []string{"passive-monitor", "set_power", "set_temperature", "set_fan_speed", "set_mode", "set_multiple"}
	}
	return []string{"passive-monitor"}
}

// Execute 는 제어 명령을 실행한다. executor 가 없으면 ErrNotControllable 반환.
func (a *LGCPDeviceAdapter) Execute(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
	if a.executor == nil {
		return nil, device.ErrNotControllable
	}
	return a.executor(ctx, command, params)
}

// Commands 는 이 디바이스에 사용 가능한 명령 목록을 반환한다.
func (a *LGCPDeviceAdapter) Commands() []device.CommandSpec {
	return a.commands
}

// lgcpIndoorCommandSpecs 는 LGCP 실내기 제어 명령 스펙을 생성한다.
func lgcpIndoorCommandSpecs() []device.CommandSpec {
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
			Name:        "set_temperature",
			Description: "설정 온도 변경 (15~30도)",
			Params: []device.ParamSpec{
				// target_temp: NASA/LGAP/LGCP 공통 컨벤션. 이전엔 'temperature' 였으나
				// LGCPAgent.buildThermostatPayloadForCommand 가 params["target_temp"] 를
				// 요구하여 이름 불일치로 ErrLGCPMissingParam 발생. (참조: lgcp_agent.go:703)
				{Name: "target_temp", Type: "float", Required: true, Min: &minTemp, Max: &maxTemp},
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
				// target_temp: lgcp_control.go buildControlPayload 가 params["target_temp"] 를 사용
				{Name: "target_temp", Type: "float", Min: &minTemp, Max: &maxTemp},
				{Name: "fan_speed", Type: "enum", Enum: []string{"low", "medium", "high", "turbo", "auto"}},
				{Name: "mode", Type: "enum", Enum: []string{"cooling", "dehumidify", "fan", "auto", "heating"}},
			},
		},
	}
}
