package adapter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/agent/hvac"
	"github.com/xtra/xflow/internal/device"
)

// Compile-time interface checks.
var (
	_ device.Device             = (*NASADeviceAdapter)(nil)
	_ device.ControllableDevice = (*NASADeviceAdapter)(nil)
)

// NASADeviceInfo holds pre-extracted data from a NASADevice.
// This breaks the import dependency on the samsung package.
type NASADeviceInfo struct {
	Address    string // Formatted as "XX.XX.XX" (e.g., "20.01.00")
	DeviceID   string // User-defined device identifier (may be empty)
	Name       string // User-defined device name (may be empty)
	DeviceType string // "HVACR.IDU", "HVACR.ODU", "controller" (v0.18.3)
	Online     bool
	Ready      bool
	LastSeen   time.Time
	ErrorCount int
	// State properties (from NASADeviceState, nil-safe)
	Power           *bool
	Mode            *string
	TargetTemp      *float32
	CurrentTemp     *float32
	FanSpeed        *string
	SwingVertical   *bool
	FilterAlarm     *bool
	ErrorCode       *uint16
	Protocol        string         // Override protocol name (empty defaults to "nasa")
	ExtraProperties map[string]any // Additional protocol-specific state properties
	DeviceSource    string         // "config" 또는 "auto"/"bridge"
}

// NASADeviceAdapter wraps NASA device data into the unified Device interface.
type NASADeviceAdapter struct {
	info      NASADeviceInfo
	agentName string
	metadata  device.DeviceMetadata
	executor  CommandExecutor      // nil for non-controllable devices
	commands  []device.CommandSpec // nil for non-controllable devices
}

// NewNASADevice creates a Device from NASA device info.
func NewNASADevice(agentName string, info NASADeviceInfo) *NASADeviceAdapter {
	return &NASADeviceAdapter{
		info:      info,
		agentName: agentName,
	}
}

// NewControllableNASADevice creates a ControllableDevice from NASA device info.
func NewControllableNASADevice(agentName string, info NASADeviceInfo, executor CommandExecutor) *NASADeviceAdapter {
	return &NASADeviceAdapter{
		info:      info,
		agentName: agentName,
		executor:  executor,
		commands:  nasaCommandSpecs(info.DeviceType),
	}
}

// ID returns the globally unique UUID v4 for this device.
//
// SPEC-DEVICE-IDENTITY-001 Phase D (xflowd v1.0 — D-T1, Breaking):
// ID() now returns the UUID (same value as UID()). The legacy composite
// key ("agentName:address") format has been fully removed.
func (a *NASADeviceAdapter) ID() string {
	return ResolveAdapterUID(a.agentName, a.info.Address)
}

// UID returns the globally unique UUID v4 for this device, resolved via
// agent.ResolveDeviceID(ctx, agentName, info.Address).
//
// The localID matches the unit identifier used by samsung and lg agents when
// they emit messages, so the UUID is guaranteed identical across the emit
// path and the REST/inventory paths.
//
// See SPEC-DEVICE-IDENTITY-001 § M1. Phase D (v1.0): ID() == UID().
func (a *NASADeviceAdapter) UID() string {
	return ResolveAdapterUID(a.agentName, a.info.Address)
}

// Name returns the device name with priority: metadata.Name > info.Name > DeviceID > generated default.
func (a *NASADeviceAdapter) Name() string {
	// 1순위: 메타데이터에 설정된 사용자 정의 이름
	if a.metadata.Name != "" {
		return a.metadata.Name
	}
	// 2순위: 디바이스 자체의 이름 (add_device 시 설정)
	if a.info.Name != "" {
		return a.info.Name
	}
	// 3순위: 디바이스 식별자
	if a.info.DeviceID != "" {
		return a.info.DeviceID
	}
	// 4순위: 프로토콜/타입/주소 기반 생성 이름
	proto := "NASA"
	if a.info.Protocol != "" {
		proto = strings.ToUpper(a.info.Protocol)
	}
	return fmt.Sprintf("%s %s %s", proto, a.info.DeviceType, a.info.Address)
}

// Type maps the string device type to a device.DeviceType constant.
func (a *NASADeviceAdapter) Type() device.DeviceType {
	switch a.info.DeviceType {
	case "HVACR.IDU":
		return device.DeviceTypeIndoor
	case "HVACR.ODU":
		return device.DeviceTypeOutdoor
	case "controller":
		return device.DeviceTypeController
	default:
		return device.DeviceTypeUnknown
	}
}

// Protocol returns the protocol name.
func (a *NASADeviceAdapter) Protocol() string {
	if a.info.Protocol != "" {
		return a.info.Protocol
	}
	return "nasa"
}

// AgentName returns the name of the owning agent.
func (a *NASADeviceAdapter) AgentName() string {
	return a.agentName
}

// Online returns whether the device is currently online.
func (a *NASADeviceAdapter) Online() bool {
	return a.info.Online
}

// LastSeen returns the last communication time.
func (a *NASADeviceAdapter) LastSeen() time.Time {
	return a.info.LastSeen
}

// State returns the current device state with NASA-specific properties.
func (a *NASADeviceAdapter) State() device.DeviceState {
	props := make(map[string]any)

	if a.info.Power != nil {
		props["power"] = *a.info.Power
	}
	// SPEC-CENTURY-001 v0.18.13 후속: mode/fan_speed 는 hvac 통일 ID (int) 로 emit.
	// 다른 HVAC 에이전트 (LGCP/LGCNP/Century) 와 schema 정합. NASA agent 의 raw
	// 문자열 ("cool"/"low") 은 web UI 의 hvac.ModeName(id) 매핑 layer 에서 변환.
	if a.info.Mode != nil {
		props["mode"] = hvac.ModeFromName(*a.info.Mode)
	}
	if a.info.TargetTemp != nil {
		props["target_temperature"] = *a.info.TargetTemp
	}
	if a.info.CurrentTemp != nil {
		props["current_temperature"] = *a.info.CurrentTemp
	}
	if a.info.FanSpeed != nil {
		props["fan_speed"] = hvac.FanSpeedFromName(*a.info.FanSpeed)
	}
	if a.info.SwingVertical != nil {
		props["swing_vertical"] = *a.info.SwingVertical
	}
	if a.info.FilterAlarm != nil {
		props["filter_alarm"] = *a.info.FilterAlarm
	}
	if a.info.ErrorCode != nil {
		props["error_code"] = *a.info.ErrorCode
	}

	for k, v := range a.info.ExtraProperties {
		props[k] = v
	}

	// Return nil Properties map when no state fields are set
	var propsResult map[string]any
	if len(props) > 0 {
		propsResult = props
	}

	return device.DeviceState{
		Online:     a.info.Online,
		Ready:      a.info.Ready,
		LastSeen:   a.info.LastSeen,
		ErrorCount: a.info.ErrorCount,
		Properties: propsResult,
	}
}

// Metadata returns user-defined metadata for this device.
func (a *NASADeviceAdapter) Metadata() device.DeviceMetadata {
	return a.metadata
}

func (a *NASADeviceAdapter) Source() string {
	if a.info.DeviceSource != "" {
		return a.info.DeviceSource
	}
	return "auto"
}

// Capabilities returns the list of supported capabilities based on device type.
func (a *NASADeviceAdapter) Capabilities() []string {
	if a.info.DeviceType == "HVACR.IDU" {
		return []string{"target_temperature", "set_mode", "set_power", "set_fan_speed"}
	}
	return nil
}

// Execute runs a named command with the given parameters and returns the result.
// Returns ErrNotControllable if the device has no executor.
func (a *NASADeviceAdapter) Execute(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
	if a.executor == nil {
		return nil, device.ErrNotControllable
	}
	return a.executor(ctx, command, params)
}

// Commands returns the list of available commands for this device.
// Returns nil for non-controllable devices.
func (a *NASADeviceAdapter) Commands() []device.CommandSpec {
	return a.commands
}

// nasaCommandSpecs generates the command specifications for a given NASA device type.
func nasaCommandSpecs(deviceType string) []device.CommandSpec {
	if deviceType != "HVACR.IDU" {
		return nil
	}

	minTemp := 16.0
	maxTemp := 30.0

	return []device.CommandSpec{
		{
			Name:        "target_temperature",
			Description: "Set the target temperature",
			Params: []device.ParamSpec{
				{
					Name:     "target_temperature",
					Type:     "float",
					Required: true,
					Min:      &minTemp,
					Max:      &maxTemp,
				},
			},
		},
		{
			Name:        "set_mode",
			Description: "Set the operating mode",
			Params: []device.ParamSpec{
				{
					Name:     "mode",
					Type:     "enum",
					Required: true,
					Enum:     []string{"auto", "cool", "dry", "fan", "heat"},
				},
			},
		},
		{
			Name:        "set_power",
			Description: "Set the power state",
			Params: []device.ParamSpec{
				{
					Name:     "power",
					Type:     "bool",
					Required: true,
				},
			},
		},
		{
			Name:        "set_fan_speed",
			Description: "Set the fan speed",
			Params: []device.ParamSpec{
				{
					Name:     "fan_speed",
					Type:     "enum",
					Required: true,
					Enum:     []string{"auto", "low", "medium", "high"},
				},
			},
		},
	}
}
