package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/device"
)

// ---------------------------------------------------------------------------
// LG PMBUSB00A (lg_hvacr03) 디바이스 어댑터
//
// SPEC-LG-HVACR-003 § M8.
//
// LGIcp02DeviceAdapter 를 재사용하지 않는 이유: 그쪽의 lgIcp02IndoorCommandSpecs()
// 는 제어 명령 5종으로 고정되어 있어, PMBUSB00A 가 노출하는 확장 명령 7종
// (스윙/필터 해제/잠금/온도 제한/ERV 3종)을 표현할 수 없다. 또한 명령 스펙이
// 기기 종류(에어컨/ERV/하이드로킷)마다 달라야 한다.
// ---------------------------------------------------------------------------

// Compile-time interface checks.
//
// 읽기 전용 어댑터는 device.Device 만 만족해야 한다. 소비 지점
// (internal/api/handler/device.go, internal/device/registry.go)이 타입 단언으로
// 제어 가능 여부를 판정하므로, 제어가 꺼진 디바이스가 ControllableDevice 를
// 만족하면 대시보드에 제어 UI 가 노출된다.
var (
	_ device.Device             = (*LGPmbusDeviceAdapter)(nil)
	_ device.Device             = (*LGPmbusControllableAdapter)(nil)
	_ device.ControllableDevice = (*LGPmbusControllableAdapter)(nil)
)

// 기기 종류 문자열. internal/agent/lg 의 pmbusDeviceType* 과 값이 일치해야 한다.
// 패키지 경계를 넘는 상수라 중복 정의하되, 값 불일치는 명령 스펙 선택 실패로 드러난다.
const (
	PmbusDeviceTypeIDU  = "HVACR.IDU"
	PmbusDeviceTypeERV  = "HVACR.ERV"
	PmbusDeviceTypeAWHP = "HVACR.AWHP"
)

// LGPmbusDeviceInfo 는 PMBUSB00A 디바이스의 스냅샷 데이터이다.
type LGPmbusDeviceInfo struct {
	Address       string // 실내기 주소 (10진 문자열, 예: "3")
	Label         string // 사람이 읽을 수 있는 라벨
	DeviceType    string // PmbusDeviceType* 중 하나
	Online        bool
	LastSeen      time.Time
	Properties    map[string]any // 상태 속성 (PmbusDeviceState.toProperties 결과)
	Source        string         // "config" 또는 "auto"
	ReportEnabled bool
}

// LGPmbusDeviceAdapter 는 PMBUSB00A 디바이스를 읽기 전용 Device 로 래핑한다.
type LGPmbusDeviceAdapter struct {
	info      LGPmbusDeviceInfo
	agentName string
}

// LGPmbusControllableAdapter 는 제어 가능한 PMBUSB00A 디바이스이다.
// 읽기 전용 어댑터를 임베드하고 제어 메서드만 덧붙인다.
type LGPmbusControllableAdapter struct {
	LGPmbusDeviceAdapter
	executor CommandExecutor
	commands []device.CommandSpec
}

// NewLGPmbusDevice 는 읽기 전용 어댑터를 생성한다.
func NewLGPmbusDevice(agentName string, info LGPmbusDeviceInfo) *LGPmbusDeviceAdapter {
	return &LGPmbusDeviceAdapter{info: info, agentName: agentName}
}

// NewControllableLGPmbusDevice 는 제어 가능한 어댑터를 생성한다.
// 명령 스펙은 기기 종류에 맞춰 선택된다.
func NewControllableLGPmbusDevice(agentName string, info LGPmbusDeviceInfo, executor CommandExecutor) *LGPmbusControllableAdapter {
	return &LGPmbusControllableAdapter{
		LGPmbusDeviceAdapter: LGPmbusDeviceAdapter{info: info, agentName: agentName},
		executor:             executor,
		commands:             lgPmbusCommandSpecs(info.DeviceType),
	}
}

// ID 는 이 디바이스의 글로벌 UUID v4 를 반환한다.
func (a *LGPmbusDeviceAdapter) ID() string {
	return ResolveAdapterUID(a.agentName, a.info.Address)
}

// UID 는 (agentName, address) 의 글로벌 UUID v4 를 반환한다.
func (a *LGPmbusDeviceAdapter) UID() string {
	return ResolveAdapterUID(a.agentName, a.info.Address)
}

// Name 은 표시 이름을 반환한다.
func (a *LGPmbusDeviceAdapter) Name() string {
	if a.info.Label != "" {
		return a.info.Label
	}
	return fmt.Sprintf("lg_pmbus %s %s", a.info.DeviceType, a.info.Address)
}

// Type 은 통합 디바이스 종류를 반환한다.
// ERV / 하이드로킷도 실내 설치 단말이므로 Indoor 로 분류한다.
func (a *LGPmbusDeviceAdapter) Type() device.DeviceType {
	switch a.info.DeviceType {
	case PmbusDeviceTypeIDU, PmbusDeviceTypeERV, PmbusDeviceTypeAWHP:
		return device.DeviceTypeIndoor
	default:
		return device.DeviceTypeUnknown
	}
}

// Protocol 은 프로토콜 이름을 반환한다.
func (a *LGPmbusDeviceAdapter) Protocol() string {
	return "lg_pmbus"
}

// AgentName 은 소속 에이전트 이름을 반환한다.
func (a *LGPmbusDeviceAdapter) AgentName() string {
	return a.agentName
}

// Online 은 연결 상태를 반환한다.
func (a *LGPmbusDeviceAdapter) Online() bool {
	return a.info.Online
}

// LastSeen 은 마지막 관측 시각을 반환한다.
func (a *LGPmbusDeviceAdapter) LastSeen() time.Time {
	return a.info.LastSeen
}

// State 는 현재 디바이스 상태를 반환한다.
func (a *LGPmbusDeviceAdapter) State() device.DeviceState {
	return device.DeviceState{
		Online:     a.info.Online,
		LastSeen:   a.info.LastSeen,
		Properties: a.info.Properties,
	}
}

// Metadata 는 사용자 정의 메타데이터를 반환한다.
func (a *LGPmbusDeviceAdapter) Metadata() device.DeviceMetadata {
	return device.DeviceMetadata{}
}

// Source 는 디바이스 등록 출처를 반환한다.
func (a *LGPmbusDeviceAdapter) Source() string {
	if a.info.Source != "" {
		return a.info.Source
	}
	return "auto"
}

// ReportEnabled 는 디바이스별 상태 전송 on/off 설정을 반환한다 (기본 true).
// device.Device 인터페이스가 아니라 optional-interface 로 REST DTO 가 읽는다.
func (a *LGPmbusDeviceAdapter) ReportEnabled() bool {
	return a.info.ReportEnabled
}

// Capabilities 는 지원 기능 목록을 반환한다.
// 읽기 전용 디바이스는 폴링 수집만 가능하다.
func (a *LGPmbusDeviceAdapter) Capabilities() []string {
	return []string{"modbus-gateway"}
}

// ---------------------------------------------------------------------------
// 제어 가능 어댑터 전용 메서드
// ---------------------------------------------------------------------------

// Capabilities 는 지원 기능 목록에 제어 명령을 더해 반환한다.
func (a *LGPmbusControllableAdapter) Capabilities() []string {
	caps := make([]string, 0, len(a.commands)+1)
	caps = append(caps, "modbus-gateway")
	for _, spec := range a.commands {
		caps = append(caps, spec.Name)
	}
	return caps
}

// Commands 는 지원 명령 스펙을 반환한다.
func (a *LGPmbusControllableAdapter) Commands() []device.CommandSpec {
	return a.commands
}

// Execute 는 명령을 에이전트로 전달한다.
func (a *LGPmbusControllableAdapter) Execute(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
	if a.executor == nil {
		return nil, device.ErrNotControllable
	}
	return a.executor(ctx, command, params)
}

// ---------------------------------------------------------------------------
// 명령 스펙
// ---------------------------------------------------------------------------

// lgPmbusCommandSpecs 는 기기 종류에 맞는 명령 스펙을 반환한다.
func lgPmbusCommandSpecs(deviceType string) []device.CommandSpec {
	switch deviceType {
	case PmbusDeviceTypeERV:
		return append(lgPmbusCommonSpecs(), lgPmbusERVSpecs()...)
	case PmbusDeviceTypeAWHP:
		return append(lgPmbusCommonSpecs(), lgPmbusHydroSpecs()...)
	default:
		return append(lgPmbusCommonSpecs(), lgPmbusIndoorSpecs()...)
	}
}

// lgPmbusCommonSpecs 는 모든 기기 종류에 공통인 명령이다.
func lgPmbusCommonSpecs() []device.CommandSpec {
	return []device.CommandSpec{
		{
			Name:        "set_power",
			Description: "Turn the unit on or off",
			Params: []device.ParamSpec{
				{Name: "power", Type: "bool", Required: true},
			},
		},
		{
			Name:        "clear_filter_alarm",
			Description: "Clear the filter alarm",
			Params:      []device.ParamSpec{},
		},
		{
			Name:        "set_lock",
			Description: "Lock or unlock a remote-control function",
			Params: []device.ParamSpec{
				{Name: "target", Type: "enum", Required: true,
					Enum: []string{"remote", "mode", "fan", "temp", "address"}},
				{Name: "locked", Type: "bool", Required: true},
			},
		},
	}
}

// lgPmbusIndoorSpecs 는 에어컨 실내기 전용 명령이다.
func lgPmbusIndoorSpecs() []device.CommandSpec {
	minTemp, maxTemp := 16.0, 30.0
	return []device.CommandSpec{
		{
			Name:        "set_mode",
			Description: "Change the operating mode",
			Params: []device.ParamSpec{
				{Name: "mode", Type: "enum", Required: true,
					Enum: []string{"cool", "dry", "fan", "auto", "heat"}},
			},
		},
		{
			Name:        "set_fan_speed",
			Description: "Change the fan speed",
			Params: []device.ParamSpec{
				{Name: "fan_speed", Type: "enum", Required: true,
					Enum: []string{"low", "medium", "high", "auto"}},
			},
		},
		{
			Name:        "target_temperature",
			Description: "Set the target temperature in degrees Celsius",
			Params: []device.ParamSpec{
				{Name: "temperature", Type: "float", Required: true, Min: &minTemp, Max: &maxTemp},
			},
		},
		{
			// 모드·풍량·온도를 한 트랜잭션으로 쓴다. 에어컨은 모드별로 마지막
			// 온도·풍량을 기억하므로, 개별 명령으로 나누면 중간 상태가 생긴다.
			Name:        "set_multiple",
			Description: "Set mode, fan speed, and temperature in one transaction",
			Params: []device.ParamSpec{
				{Name: "mode", Type: "enum", Required: false,
					Enum: []string{"cool", "dry", "fan", "auto", "heat"}},
				{Name: "fan_speed", Type: "enum", Required: false,
					Enum: []string{"low", "medium", "high", "auto"}},
				{Name: "temperature", Type: "float", Required: false, Min: &minTemp, Max: &maxTemp},
			},
		},
		{
			Name:        "set_swing",
			Description: "Enable or disable automatic airflow swing",
			Params: []device.ParamSpec{
				{Name: "swing", Type: "bool", Required: true},
			},
		},
		{
			Name:        "set_temp_limit",
			Description: "Set the upper and lower target-temperature limits",
			Params: []device.ParamSpec{
				{Name: "high", Type: "float", Required: true, Min: &minTemp, Max: &maxTemp},
				{Name: "low", Type: "float", Required: true, Min: &minTemp, Max: &maxTemp},
			},
		},
	}
}

// lgPmbusERVSpecs 는 환기(ERV) 전용 명령이다.
//
// 미실측: ERV 장비를 확보한 뒤 실동작 검증이 필요하다 (프로토콜 문서 §8 참조).
func lgPmbusERVSpecs() []device.CommandSpec {
	return []device.CommandSpec{
		{
			Name:        "set_erv_mode",
			Description: "Change the ventilation mode",
			Params: []device.ParamSpec{
				{Name: "erv_mode", Type: "enum", Required: true,
					Enum: []string{"heat_exchange", "auto", "normal"}},
			},
		},
		{
			Name:        "set_erv_rapid",
			Description: "Enable or disable rapid ventilation",
			Params: []device.ParamSpec{
				{Name: "enabled", Type: "bool", Required: true},
			},
		},
		{
			Name:        "set_erv_eco",
			Description: "Enable or disable energy-saving ventilation",
			Params: []device.ParamSpec{
				{Name: "enabled", Type: "bool", Required: true},
			},
		},
	}
}

// lgPmbusHydroSpecs 는 하이드로킷(THERMA V) 전용 명령이다.
//
// 미실측: 하이드로킷 장비를 확보한 뒤 실동작 검증이 필요하다.
func lgPmbusHydroSpecs() []device.CommandSpec {
	minTemp, maxTemp := 16.0, 30.0
	return []device.CommandSpec{
		{
			Name:        "set_mode",
			Description: "Change the operating mode",
			Params: []device.ParamSpec{
				{Name: "mode", Type: "enum", Required: true,
					Enum: []string{"cool", "auto", "heat"}},
			},
		},
		{
			Name:        "target_temperature",
			Description: "Set the target temperature in degrees Celsius",
			Params: []device.ParamSpec{
				{Name: "temperature", Type: "float", Required: true, Min: &minTemp, Max: &maxTemp},
			},
		},
		{
			Name:        "set_temp_limit",
			Description: "Set the upper and lower target-temperature limits",
			Params: []device.ParamSpec{
				{Name: "high", Type: "float", Required: true, Min: &minTemp, Max: &maxTemp},
				{Name: "low", Type: "float", Required: true, Min: &minTemp, Max: &maxTemp},
			},
		},
	}
}
