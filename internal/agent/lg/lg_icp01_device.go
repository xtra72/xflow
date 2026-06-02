package lg

import (
	"time"

	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/device/adapter"
)

// ---------------------------------------------------------------------------
// LG ICP-01 디바이스 모델 — 패시브 캡처에서 자동 발견된 디바이스 상태 관리
// ---------------------------------------------------------------------------

// Icp01Device 는 LG ICP-01 버스에서 관측된 디바이스이다.
type Icp01Device struct {
	Address  string // "odu" 또는 "81"~"85"
	Label    string // "outdoor", "indoor-1"~"indoor-5"
	Type     string // "HVACR.ODU" 또는 "HVACR.IDU" (v0.18.3)
	Online   bool
	LastSeen time.Time
	Source   string            // "auto" 또는 "config"
	State    *Icp01DeviceState // IDU 상태 (indoor)
	ODUState *Icp01ODUState    // ODU 상태 (outdoor)
	// v0.7.0: IDU 메타 (frame 의 slot_num 저장 — 정기 보고 시 metadata 재현용).
	IDUNum  int  // 1..5 (IDU 인덱스)
	SlotNum byte // frame.SlotNum (0x51~0x55)
}

// Icp01DeviceState 는 IDU 디바이스의 누적 상태이다.
type Icp01DeviceState struct {
	Power      *bool    `json:"power,omitempty"`
	SetTemp    *float64 `json:"target_temperature,omitempty"`
	RoomTemp   *float64 `json:"current_temperature,omitempty"`
	InletTemp  *float64 `json:"inlet_temperature,omitempty"`
	OutletTemp *float64 `json:"outlet_temperature,omitempty"`
	FanSpeed   *int     `json:"fan_speed,omitempty"`
	OpMode     *int     `json:"op_mode,omitempty"`
	CMDCycle   *string  `json:"cmd_cycle,omitempty"`
	DevType    *int     `json:"device_type,omitempty"`
	DeviceID   *int     `json:"device_id,omitempty"`
}

// Icp01ODUState 는 ODU(실외기)의 누적 상태이다.
type Icp01ODUState struct {
	// SEQ=02 확정 필드
	OutdoorTemp       *float64 `json:"outdoor_temperature,omitempty"`              // b[06] 외기온도
	CompSuctionTemp   *float64 `json:"compressor_suction_temperature,omitempty"`   // b[08] 압축기 흡입온도
	CompDischargeTemp *float64 `json:"compressor_discharge_temperature,omitempty"` // b[11] 압축기 토출온도
	CondenserTempA    *float64 `json:"condenser_temperature_a,omitempty"`          // b[14] 응축측 온도A
	CondenserTempB    *float64 `json:"condenser_temperature_b,omitempty"`          // b[15] 응축측 온도B
	// SEQ=04 확정 필드
	AvgTemp *float64 `json:"avg_temperature,omitempty"` // b[10] 운전 평균 온도
}

// snapshot 은 현재 상태의 복사본을 반환한다.
func (s *Icp01DeviceState) snapshot() Icp01DeviceState {
	return *s
}

// snapshot 은 현재 상태의 복사본을 반환한다.
func (s *Icp01ODUState) snapshot() Icp01ODUState {
	return *s
}

// toProperties 는 디바이스 상태를 통합 속성 맵으로 변환한다.
// 속성명은 NASA/LGCP 에이전트와 통일: power, current_temp, target_temp, mode, fan_speed.
func (s *Icp01DeviceState) toProperties() map[string]any {
	props := make(map[string]any)

	// power: 에이전트에서 프레임의 원시 OP_MODE bit5 기반으로 설정됨.
	powerOn := true
	if s.Power != nil {
		powerOn = *s.Power
	}
	props["power"] = powerOn

	// OFF 시 전원 외 모든 정보 표시하지 않음
	if !powerOn {
		return props
	}

	if s.OpMode != nil {
		props["mode"] = icp01OpModeToHVACID(*s.OpMode)
	} else {
		props["mode"] = 0 // hvac.ModeOffOrAuto
	}
	if s.FanSpeed != nil {
		props["fan_speed"] = icp01FanSpeedToHVACID(*s.FanSpeed)
	} else {
		props["fan_speed"] = 0 // hvac.FanOff
	}
	if s.SetTemp != nil {
		props["target_temperature"] = *s.SetTemp
	}
	if s.RoomTemp != nil {
		props["current_temperature"] = *s.RoomTemp
	}
	if s.InletTemp != nil {
		props["inlet_temperature"] = *s.InletTemp
	}
	if s.OutletTemp != nil {
		props["outlet_temperature"] = *s.OutletTemp
	}
	return props
}

// ---------------------------------------------------------------------------
// 통일 운전 모드 ID (전 프로토콜 공통)
// ---------------------------------------------------------------------------
//
// | ID | 모드 | 문자열 |
// |----|------|--------|
// | 0  | 냉방 | cool   |
// | 1  | 제습 | dry    |
// | 2  | 송풍 | fan    |
// | 3  | 자동 | auto   |
// | 4  | 난방 | heat   |

const (
	OpModeCool = 0
	OpModeDry  = 1
	OpModeFan  = 2
	OpModeAuto = 3
	OpModeHeat = 4
)

// OpModeIDToString 은 통일 운전 모드 ID를 문자열로 변환한다.
var OpModeIDToString = map[int]string{
	OpModeCool: "cool",
	OpModeDry:  "dry",
	OpModeFan:  "fan",
	OpModeAuto: "auto",
	OpModeHeat: "heat",
}

// icp01OpModeToID 는 LG ICP-01 b[10] 원시 바이트를 통일 운전 모드 ID로 변환한다.
// 하위 니블이 LGAP 모드 코드와 일치. 실측: 0x14 → 니블 4 → heat.
func icp01OpModeToID(raw byte) int {
	nibble := int(raw & 0x0F)
	if nibble <= OpModeHeat {
		return nibble
	}
	return OpModeCool // 알 수 없는 값은 기본값
}

// icp01DecodeOpMode 는 통일 운전 모드 ID(int)를 문자열로 변환한다.
func icp01DecodeOpMode(raw int) string {
	if s, ok := OpModeIDToString[raw]; ok {
		return s
	}
	return "cool"
}

// icp01OpModeToHVACID 는 LG ICP-01 내부 OpMode ID 를 hvac 통일 ID 로 변환한다 (v0.7.5).
//
//	LG ICP-01 내부: 0=cool, 1=dry, 2=fan, 3=auto, 4=heat
//	hvac:       0=off/auto, 1=cool, 2=heat, 3=dry, 4=fan
func icp01OpModeToHVACID(icp01ID int) int {
	switch icp01ID {
	case OpModeCool:
		return 1 // hvac.ModeCool
	case OpModeHeat:
		return 2 // hvac.ModeHeat
	case OpModeDry:
		return 3 // hvac.ModeDry
	case OpModeFan:
		return 4 // hvac.ModeFan
	case OpModeAuto:
		return 0 // hvac.ModeOffOrAuto
	default:
		return 0
	}
}

// ---------------------------------------------------------------------------
// 통일 풍량 ID (전 프로토콜 공통)
// ---------------------------------------------------------------------------
//
// | ID | 풍량 | 문자열 |
// |----|------|--------|
// | 0  | 자동 | auto   |
// | 1  | 미풍 | quiet  |
// | 2  | 약   | low    |
// | 3  | 중   | medium |
// | 4  | 강   | high   |
// | 5  | 터보 | turbo  |

const (
	FanSpeedAuto   = 0
	FanSpeedQuiet  = 1
	FanSpeedLow    = 2
	FanSpeedMedium = 3
	FanSpeedHigh   = 4
	FanSpeedTurbo  = 5
)

// FanSpeedIDToString 은 통일 풍량 ID를 문자열로 변환한다.
var FanSpeedIDToString = map[int]string{
	FanSpeedAuto:   "auto",
	FanSpeedQuiet:  "quiet",
	FanSpeedLow:    "low",
	FanSpeedMedium: "medium",
	FanSpeedHigh:   "high",
	FanSpeedTurbo:  "turbo",
}

// icp01FanByteToID 는 LG ICP-01 b[30] 원시 바이트를 통일 풍량 ID 로 변환한다.
//
// 범용 인코딩 (LG family 전반에서 일관 관측):
//
//	0x30 (v0.18.10), 0x54 → quiet (미풍)
//	0x14, 0x50          → low   (약풍)
//
// DEV_TYPE 파라미터는 향후 장치-특이 override 를 위해 유지하나 현재는
// 사용하지 않음 — 사용자 실측에서 0x30 이 DEV_TYPE=0x72/0x73 모두에서
// 동일하게 미풍이라 범용으로 분류.
//
// 미인식 바이트는 FanSpeedAuto 로 폴백.
func icp01FanByteToID(raw byte, _ byte) int {
	switch raw {
	case 0x30, 0x54:
		return FanSpeedQuiet
	case 0x14, 0x50:
		return FanSpeedLow
	default:
		return FanSpeedAuto
	}
}

// icp01IsKnownFanByte 는 (devType, fanByte) 조합이 인식된 매핑에 해당하는지
// 반환한다. 디버그 로그를 알려진 조합에 대해 suppress 하는 용도.
//
// devType 파라미터는 미사용 (icp01FanByteToID 와 동일 범위) — 시그니처는
// 향후 장치-특이 매핑이 도입될 경우에 대비.
func icp01IsKnownFanByte(_ byte, raw byte) bool {
	return raw == 0x14 || raw == 0x30 || raw == 0x50 || raw == 0x54
}

// icp01FanSpeedToHVACID 는 LG ICP-01 내부 FanSpeed ID 를 hvac 통일 ID 로 변환한다 (v0.7.5).
//
//	LG ICP-01 내부: 0=auto, 1=quiet, 2=low, 3=medium, 4=high, 5=turbo
//	hvac:       0=off, 1=auto, 2=quiet, 3=low, 4=medium, 5=high, 6=turbo
func icp01FanSpeedToHVACID(icp01ID int) int {
	switch icp01ID {
	case FanSpeedAuto:
		return 1 // hvac.FanAuto
	case FanSpeedQuiet:
		return 2 // hvac.FanQuiet
	case FanSpeedLow:
		return 3 // hvac.FanLow
	case FanSpeedMedium:
		return 4 // hvac.FanMedium
	case FanSpeedHigh:
		return 5 // hvac.FanHigh
	case FanSpeedTurbo:
		return 6 // hvac.FanTurbo
	default:
		return 0 // hvac.FanOff
	}
}

// icp01DecodeFanSpeed 는 통일 풍량 ID(int)를 문자열로 변환한다.
func icp01DecodeFanSpeed(raw int) string {
	if s, ok := FanSpeedIDToString[raw]; ok {
		return s
	}
	return "auto"
}

// toProperties 는 ODU 상태를 통합 속성 맵으로 변환한다.
func (s *Icp01ODUState) toProperties() map[string]any {
	props := make(map[string]any)
	if s.OutdoorTemp != nil {
		props["outdoor_temperature"] = *s.OutdoorTemp
	}
	if s.CompSuctionTemp != nil {
		props["compressor_suction_temperature"] = *s.CompSuctionTemp
	}
	if s.CompDischargeTemp != nil {
		props["compressor_discharge_temperature"] = *s.CompDischargeTemp
	}
	if s.CondenserTempA != nil {
		props["condenser_temperature_a"] = *s.CondenserTempA
	}
	if s.CondenserTempB != nil {
		props["condenser_temperature_b"] = *s.CondenserTempB
	}
	if s.AvgTemp != nil {
		props["avg_temperature"] = *s.AvgTemp
	}
	return props
}

// icp01DeviceStateChanged 는 두 IDU 상태를 비교하여 주요 필드가 변경되었는지 판별한다.
func icp01DeviceStateChanged(prev, curr Icp01DeviceState) bool {
	if !ptrF64Eq(prev.SetTemp, curr.SetTemp) {
		return true
	}
	if !ptrF64Eq(prev.RoomTemp, curr.RoomTemp) {
		return true
	}
	if !ptrF64Eq(prev.InletTemp, curr.InletTemp) {
		return true
	}
	if !ptrF64Eq(prev.OutletTemp, curr.OutletTemp) {
		return true
	}
	if !ptrIntEq(prev.OpMode, curr.OpMode) {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Icp01DeviceProvider — device.DeviceProvider 구현
// ---------------------------------------------------------------------------

// Icp01DeviceProvider 는 LG ICP-01 에이전트의 device.DeviceProvider 구현이다.
type Icp01DeviceProvider struct {
	agent *Hvacr01Agent
}

// 컴파일 타임 인터페이스 체크
var _ device.DeviceProvider = (*Icp01DeviceProvider)(nil)

// NewIcp01DeviceProvider 는 LG ICP-01 에이전트를 래핑하는 DeviceProvider 를 생성한다.
func NewIcp01DeviceProvider(a *Hvacr01Agent) *Icp01DeviceProvider {
	return &Icp01DeviceProvider{agent: a}
}

// Devices 는 에이전트가 관리하는 모든 디바이스를 통합 Device 인터페이스로 반환한다.
func (p *Icp01DeviceProvider) Devices() []device.Device {
	icp01Devices := p.agent.ListDevices()
	result := make([]device.Device, 0, len(icp01Devices))
	agentName := p.agent.Name()

	for i := range icp01Devices {
		dev := &icp01Devices[i]
		info := icp01DeviceToInfo(dev)
		result = append(result, adapter.NewIcp01Device(agentName, info))
	}
	return result
}

// Device 는 글로벌 ID ("agentName:address") 로 특정 디바이스를 반환한다.
func (p *Icp01DeviceProvider) Device(id string) (device.Device, error) {
	agentName := p.agent.Name()
	prefix := agentName + ":"
	if len(id) <= len(prefix) || id[:len(prefix)] != prefix {
		return nil, device.ErrDeviceNotFound
	}
	addrStr := id[len(prefix):]

	icp01Devices := p.agent.ListDevices()
	for i := range icp01Devices {
		dev := &icp01Devices[i]
		if dev.Address == addrStr {
			info := icp01DeviceToInfo(dev)
			return adapter.NewIcp01Device(agentName, info), nil
		}
	}
	return nil, device.ErrDeviceNotFound
}

// icp01DeviceToInfo 는 Icp01Device 를 adapter.Icp01DeviceInfo 로 변환한다.
func icp01DeviceToInfo(dev *Icp01Device) adapter.Icp01DeviceInfo {
	info := adapter.Icp01DeviceInfo{
		Address:    dev.Address,
		Label:      dev.Label,
		DeviceType: dev.Type,
		Online:     dev.Online,
		LastSeen:   dev.LastSeen,
		Source:     dev.Source,
	}
	if dev.State != nil {
		info.Properties = dev.State.toProperties()
	} else if dev.ODUState != nil {
		info.Properties = dev.ODUState.toProperties()
	}
	return info
}
