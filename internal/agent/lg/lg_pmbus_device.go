package lg

import (
	"reflect"
	"time"

	"github.com/xtra/xflow/internal/agent/hvac"
)

// ---------------------------------------------------------------------------
// PMBUSB00A 디바이스 모델
//
// SPEC-LG-HVACR-003 § M5. lg_hvacr02 의 Icp02Device 구조를 복제하되, 상태 필드는
// PMBUSB00A 레지스터 맵에 맞춘다.
// ---------------------------------------------------------------------------

// 기기 종류. PMBUSB00A 는 MULTI V(에어컨) / ERV(환기) / THERMA V(하이드로킷) 를 지원하며,
// 지원 레지스터가 종류마다 다르다.
const (
	pmbusDeviceTypeIDU  = "HVACR.IDU"  // 에어컨 실내기 (기본)
	pmbusDeviceTypeERV  = "HVACR.ERV"  // 환기 (Energy Recovery Ventilator)
	pmbusDeviceTypeAWHP = "HVACR.AWHP" // 하이드로킷 / THERMA V
)

// isValidPmbusDeviceType 은 설정에서 지정 가능한 기기 종류인지 확인한다.
func isValidPmbusDeviceType(s string) bool {
	switch s {
	case pmbusDeviceTypeIDU, pmbusDeviceTypeERV, pmbusDeviceTypeAWHP:
		return true
	default:
		return false
	}
}

// PmbusDevice 는 PMBUSB00A 게이트웨이를 통해 관측되는 실내기이다.
type PmbusDevice struct {
	Address  string // 사용자 표기 주소 (10진, address_base 반영. 예: "3")
	UnitN    uint16 // 내부 N (0-base). 레지스터 주소 계산에 쓴다.
	Label    string // 사람이 읽을 수 있는 라벨
	Type     string // pmbusDeviceType* 중 하나
	Online   bool
	LastSeen time.Time
	Source   string // "auto" (스캔 발견) 또는 "config" (설정 등록)
	State    *PmbusDeviceState
	// ReportEnabled 는 디바이스별 상태 전송 on/off 이다 (기본 true).
	// false 면 이 디바이스의 device_state 방출을 억제한다. In-memory 만 유지한다.
	ReportEnabled bool
	// TypePinned 는 기기 종류가 설정으로 고정되었는지 여부이다. true 면 프로토콜
	// 신호(Discrete ④)로 종류를 자동 승격하지 않는다 — 사용자 지정이 우선한다.
	TypePinned bool
}

// PmbusDeviceState 는 디바이스의 누적 상태이다.
//
// 모든 필드가 포인터인 이유: PMBUSB00A 는 지원하지 않는 항목도 0 을 돌려준다.
// "관측되지 않음"과 "값이 0"을 구분해야 미지원 레지스터의 0 이 유효값으로
// 새어 나가지 않는다.
type PmbusDeviceState struct {
	// --- Discrete Input (FC02) ---
	Connected   *bool `json:"connected,omitempty"`
	Alarm       *bool `json:"alarm,omitempty"`
	FilterAlarm *bool `json:"filter_alarm,omitempty"`
	// TempBasisWater 는 목표 온도 기준이다 (false=공기, true=물). 하이드로킷 판별에 쓴다.
	TempBasisWater *bool `json:"-"`
	// ErrorKindBC 는 에러 구분이다 (false=CH 타입, true=BC 타입). 하이드로킷 전용.
	ErrorKindBC *bool `json:"-"`

	// --- Coil (FC01) ---
	Power       *bool `json:"power,omitempty"`
	Swing       *bool `json:"swing,omitempty"`
	LockRemote  *bool `json:"lock_remote,omitempty"`
	LockMode    *bool `json:"lock_mode,omitempty"`
	LockFan     *bool `json:"lock_fan,omitempty"`
	LockTemp    *bool `json:"lock_temp,omitempty"`
	LockAddress *bool `json:"lock_address,omitempty"`
	ERVRapid    *bool `json:"erv_rapid,omitempty"`
	ERVEco      *bool `json:"erv_eco,omitempty"`

	// --- Holding Register (FC03) — 프로토콜 raw 코드를 보존한다 ---
	ModeCode      *uint16  `json:"-"`
	FanCode       *uint16  `json:"-"`
	SetTempC      *float64 `json:"target_temperature,omitempty"`
	TempLimitHigh *float64 `json:"temp_limit_high_c,omitempty"`
	TempLimitLow  *float64 `json:"temp_limit_low_c,omitempty"`
	ERVModeCode   *uint16  `json:"erv_mode,omitempty"`

	// --- Input Register (FC04) ---
	ErrorCode  *uint16  `json:"error_code,omitempty"`
	RoomTempC  *float64 `json:"current_temperature,omitempty"`
	PipeInC    *float64 `json:"pipe_in_temperature_c,omitempty"`
	PipeOutC   *float64 `json:"pipe_out_temperature_c,omitempty"`
	WaterTankC *float64 `json:"water_tank_temperature_c,omitempty"`
	SolarC     *float64 `json:"solar_temperature_c,omitempty"`
}

// snapshot 은 상태의 값 복사본을 반환한다. 포인터 필드는 새 포인터로 복제하여
// 호출자가 원본을 통해 값을 바꿀 수 없게 한다.
func (s *PmbusDeviceState) snapshot() PmbusDeviceState {
	cp := PmbusDeviceState{}
	cp.Connected = copyBoolPtr(s.Connected)
	cp.Alarm = copyBoolPtr(s.Alarm)
	cp.FilterAlarm = copyBoolPtr(s.FilterAlarm)
	cp.TempBasisWater = copyBoolPtr(s.TempBasisWater)
	cp.ErrorKindBC = copyBoolPtr(s.ErrorKindBC)
	cp.Power = copyBoolPtr(s.Power)
	cp.Swing = copyBoolPtr(s.Swing)
	cp.LockRemote = copyBoolPtr(s.LockRemote)
	cp.LockMode = copyBoolPtr(s.LockMode)
	cp.LockFan = copyBoolPtr(s.LockFan)
	cp.LockTemp = copyBoolPtr(s.LockTemp)
	cp.LockAddress = copyBoolPtr(s.LockAddress)
	cp.ERVRapid = copyBoolPtr(s.ERVRapid)
	cp.ERVEco = copyBoolPtr(s.ERVEco)
	cp.ModeCode = copyUint16Ptr(s.ModeCode)
	cp.FanCode = copyUint16Ptr(s.FanCode)
	cp.ERVModeCode = copyUint16Ptr(s.ERVModeCode)
	cp.ErrorCode = copyUint16Ptr(s.ErrorCode)
	cp.SetTempC = copyFloatPtr(s.SetTempC)
	cp.TempLimitHigh = copyFloatPtr(s.TempLimitHigh)
	cp.TempLimitLow = copyFloatPtr(s.TempLimitLow)
	cp.RoomTempC = copyFloatPtr(s.RoomTempC)
	cp.PipeInC = copyFloatPtr(s.PipeInC)
	cp.PipeOutC = copyFloatPtr(s.PipeOutC)
	cp.WaterTankC = copyFloatPtr(s.WaterTankC)
	cp.SolarC = copyFloatPtr(s.SolarC)
	return cp
}

func copyBoolPtr(p *bool) *bool {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func copyUint16Ptr(p *uint16) *uint16 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func copyFloatPtr(p *float64) *float64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// isPowerOn 은 전원 상태를 반환한다. 미관측이면 false 로 간주한다.
func (s *PmbusDeviceState) isPowerOn() bool {
	return s.Power != nil && *s.Power
}

// ---------------------------------------------------------------------------
// 상태 투영
//
// lg_hvacr02 의 toProperties 와 동일한 키 이름·전원 OFF 규약을 따르되,
// PMBUSB00A 가 추가로 노출하는 항목을 기기 종류별 게이트와 함께 덧붙인다.
// ---------------------------------------------------------------------------

// pmbusProjectionOpts 는 투영에 필요한 설정 의존 값이다.
type pmbusProjectionOpts struct {
	fanAutoCode int
}

// toProperties 는 기기 종류에 맞는 속성 맵을 생성한다.
//
// 기기 종류 게이트가 필요한 이유: 문서 §5.1 의 응답 예에서 보듯 미지원 레지스터는
// 0 으로 돌아온다. 에어컨에 급탕 탱크 온도를 투영하면 대시보드에 0 °C 가 표시된다.
func (s *PmbusDeviceState) toProperties(devType string, opts pmbusProjectionOpts) map[string]any {
	props := make(map[string]any)

	// 공통 — 알람/에러/잠금은 모든 기기 종류에 유효하다.
	if s.Power != nil {
		props["power"] = *s.Power
	}
	if s.ErrorCode != nil {
		props["error_code"] = int(*s.ErrorCode)
	}
	if s.Alarm != nil {
		props["alarm"] = *s.Alarm
	}
	if s.FilterAlarm != nil {
		props["filter_alarm"] = *s.FilterAlarm
	}
	putBool(props, "lock_remote", s.LockRemote)
	putBool(props, "lock_mode", s.LockMode)
	putBool(props, "lock_fan", s.LockFan)
	putBool(props, "lock_temp", s.LockTemp)
	putBool(props, "lock_address", s.LockAddress)

	switch devType {
	case pmbusDeviceTypeERV:
		s.projectERV(props)
	case pmbusDeviceTypeAWHP:
		s.projectAWHP(props, opts)
	default:
		s.projectIndoor(props, opts)
	}
	return props
}

// projectIndoor 는 에어컨 실내기 속성을 투영한다.
//
// 전원 OFF 규약은 lg_hvacr02 의 indoorProperties 와 동일하다: 운전 관련 속성을
// 생략하고 fan_speed / mode 만 통일 ID 0 으로 남긴다.
func (s *PmbusDeviceState) projectIndoor(props map[string]any, opts pmbusProjectionOpts) {
	if !s.isPowerOn() {
		props["fan_speed"] = hvac.FanOff
		props["mode"] = hvac.ModeOffOrAuto
		return
	}

	if s.RoomTempC != nil {
		props["current_temperature"] = *s.RoomTempC
	}
	if s.SetTempC != nil {
		props["target_temperature"] = *s.SetTempC
	}
	if s.FanCode != nil {
		props["fan_speed"] = pmbusFanToUnifiedID(*s.FanCode, opts.fanAutoCode)
	} else {
		props["fan_speed"] = hvac.FanOff
	}
	if s.ModeCode != nil {
		props["mode"] = pmbusModeToUnifiedID(*s.ModeCode)
	} else {
		props["mode"] = hvac.ModeOffOrAuto
	}
	putBool(props, "swing", s.Swing)
	putFloat(props, "pipe_in_temperature_c", s.PipeInC)
	putFloat(props, "pipe_out_temperature_c", s.PipeOutC)
	putFloat(props, "temp_limit_high_c", s.TempLimitHigh)
	putFloat(props, "temp_limit_low_c", s.TempLimitLow)
}

// projectERV 는 환기 장치 속성을 투영한다.
// 환기에는 설정 온도·풍량·배관 온도 개념이 없다.
func (s *PmbusDeviceState) projectERV(props map[string]any) {
	if !s.isPowerOn() {
		return
	}
	if s.ERVModeCode != nil {
		props["erv_mode"] = int(*s.ERVModeCode)
	}
	putBool(props, "erv_rapid", s.ERVRapid)
	putBool(props, "erv_eco", s.ERVEco)
}

// projectAWHP 는 하이드로킷(THERMA V) 속성을 투영한다.
// 배관 온도는 입수/출수 온도를, 급탕 탱크·태양열 온도는 이 종류에서만 유효하다.
func (s *PmbusDeviceState) projectAWHP(props map[string]any, opts pmbusProjectionOpts) {
	if !s.isPowerOn() {
		props["mode"] = hvac.ModeOffOrAuto
		return
	}
	if s.RoomTempC != nil {
		props["current_temperature"] = *s.RoomTempC
	}
	if s.SetTempC != nil {
		props["target_temperature"] = *s.SetTempC
	}
	if s.ModeCode != nil {
		props["mode"] = pmbusModeToUnifiedID(*s.ModeCode)
	} else {
		props["mode"] = hvac.ModeOffOrAuto
	}
	putFloat(props, "water_in_temperature_c", s.PipeInC)
	putFloat(props, "water_out_temperature_c", s.PipeOutC)
	putFloat(props, "water_tank_temperature_c", s.WaterTankC)
	putFloat(props, "solar_temperature_c", s.SolarC)
	putFloat(props, "temp_limit_high_c", s.TempLimitHigh)
	putFloat(props, "temp_limit_low_c", s.TempLimitLow)
	_ = opts // 하이드로킷은 풍량 개념이 없어 fanAutoCode 를 쓰지 않는다.
}

func putBool(props map[string]any, key string, v *bool) {
	if v != nil {
		props[key] = *v
	}
}

func putFloat(props map[string]any, key string, v *float64) {
	if v != nil {
		props[key] = *v
	}
}

// propertiesEqualPmbus 는 두 투영 맵의 동일성을 비교한다 (dedup 판정용).
func propertiesEqualPmbus(a, b map[string]any) bool {
	return reflect.DeepEqual(a, b)
}
