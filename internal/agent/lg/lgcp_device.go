package lg

import (
	"time"

	"github.com/xtra/xflow/internal/agent/hvac"
)

// ---------------------------------------------------------------------------
// LGCP 디바이스 모델 — 패시브 캡처에서 자동 발견된 디바이스 상태 관리
// ---------------------------------------------------------------------------

// LGCPDevice 는 LGCP 버스에서 관측된 디바이스이다.
type LGCPDevice struct {
	Address  string // 주소 hex (예: "44550067")
	Label    string // 사람이 읽을 수 있는 라벨 (예: "indoor-3")
	Type     string // "HVACR.IDU", "controller", "unknown" (v0.18.3)
	Online   bool
	LastSeen time.Time
	Source   string           // "auto" (자동 발견) 또는 "config" (설정 등록)
	State    *LGCPDeviceState // 현재 상태 (누적)
}

// LGCPDeviceState 는 디바이스의 누적 상태이다.
// 각 프레임의 디코딩 결과를 병합하여 최신 상태를 유지한다.
type LGCPDeviceState struct {
	// 응답 필드 (실내기 → 실외기)
	PowerState   *string  `json:"power_state,omitempty"`
	IndoorTempC  *float64 `json:"current_temperature,omitempty"` // v0.x: NASA/Century 통일
	SetTempC     *float64 `json:"target_temperature,omitempty"`  // v0.x: NASA/Century 통일
	FanSpeed     *string  `json:"fan_speed,omitempty"`
	Mode         *string  `json:"mode,omitempty"`
	ValveOpen    *bool    `json:"valve_open,omitempty"`
	FanMotorHz   *int     `json:"fan_motor_hz,omitempty"`
	PipeTemp1C   *float64 `json:"pipe_temperature1_c,omitempty"`
	PipeTemp2C   *float64 `json:"pipe_temperature2_c,omitempty"`
	FanSpeedResp *int     `json:"fan_speed_resp,omitempty"`

	// 제어 필드 (실외기 → 실내기)
	Power         *string `json:"power,omitempty"`
	CompressorCap *int    `json:"compressor_cap,omitempty"`
	CompressorHz  *int    `json:"compressor_hz,omitempty"`
	OutdoorActive *bool   `json:"outdoor_active,omitempty"`
	HeatDemand    *bool   `json:"heat_demand,omitempty"`
	CompressorRun *bool   `json:"compressor_run,omitempty"`
	RefrigerantOn *bool   `json:"refrigerant_on,omitempty"`
	OpMode        *string `json:"op_mode,omitempty"`
}

// mergeControlFields 는 제어 명령(0201) 페이로드에서 제어 필드만 병합한다.
// 센서값(실내 온도, 배관 온도 등)은 제어 명령에서 무시한다.
func (s *LGCPDeviceState) mergeControlFields(d *LGCPDecodedPayload) {
	if d == nil {
		return
	}
	if d.PowerState != nil {
		s.PowerState = d.PowerState
	}
	if d.Power != nil {
		s.Power = d.Power
	}
	if d.SetTempC != nil {
		s.SetTempC = d.SetTempC
	}
	if d.FanSpeed != nil {
		s.FanSpeed = d.FanSpeed
	}
	if d.Mode != nil {
		s.Mode = d.Mode
	}
}

// mergeDecoded 는 디코딩된 페이로드를 현재 상태에 병합한다.
// nil이 아닌 필드만 덮어쓴다.
func (s *LGCPDeviceState) mergeDecoded(d *LGCPDecodedPayload) {
	if d == nil {
		return
	}
	if d.PowerState != nil {
		s.PowerState = d.PowerState
	}
	if d.IndoorTempC != nil {
		s.IndoorTempC = d.IndoorTempC
	}
	if d.SetTempC != nil {
		s.SetTempC = d.SetTempC
	}
	if d.FanSpeed != nil {
		s.FanSpeed = d.FanSpeed
	}
	if d.Mode != nil {
		s.Mode = d.Mode
	}
	if d.ValveOpen != nil {
		s.ValveOpen = d.ValveOpen
	}
	if d.FanMotorHz != nil {
		s.FanMotorHz = d.FanMotorHz
	}
	if d.PipeTemp1C != nil {
		s.PipeTemp1C = d.PipeTemp1C
	}
	if d.PipeTemp2C != nil {
		s.PipeTemp2C = d.PipeTemp2C
	}
	if d.FanSpeedResp != nil {
		s.FanSpeedResp = d.FanSpeedResp
	}
	if d.Power != nil {
		s.Power = d.Power
	}
	if d.CompressorCap != nil {
		s.CompressorCap = d.CompressorCap
	}
	if d.CompressorHz != nil {
		s.CompressorHz = d.CompressorHz
	}
	if d.OutdoorActive != nil {
		s.OutdoorActive = d.OutdoorActive
	}
	if d.HeatDemand != nil {
		s.HeatDemand = d.HeatDemand
	}
	if d.CompressorRun != nil {
		s.CompressorRun = d.CompressorRun
	}
	if d.RefrigerantOn != nil {
		s.RefrigerantOn = d.RefrigerantOn
	}
	if d.OpMode != nil {
		s.OpMode = d.OpMode
	}
}

// snapshot 은 현재 상태의 복사본을 반환한다.
func (s *LGCPDeviceState) snapshot() LGCPDeviceState {
	return *s
}

// nonTempFieldsChangedLGCP 는 비온도 필드 중 하나라도 변경되었는지 검사한다 (v0.6.7).
// SetTempC 는 사용자 설정값이라 비온도(제어) 카테고리. PipeTemp1C/2C 는 센서 온도라 제외.
func nonTempFieldsChangedLGCP(prev, curr LGCPDeviceState) bool {
	if !ptrStrEq(prev.PowerState, curr.PowerState) {
		return true
	}
	if !ptrStrEq(prev.Power, curr.Power) {
		return true
	}
	if !ptrF64Eq(prev.SetTempC, curr.SetTempC) {
		return true
	}
	if !ptrStrEq(prev.FanSpeed, curr.FanSpeed) {
		return true
	}
	if !ptrStrEq(prev.Mode, curr.Mode) {
		return true
	}
	if !ptrBoolEq(prev.ValveOpen, curr.ValveOpen) {
		return true
	}
	if !ptrIntEq(prev.FanMotorHz, curr.FanMotorHz) {
		return true
	}
	if !ptrIntEq(prev.CompressorCap, curr.CompressorCap) {
		return true
	}
	if !ptrIntEq(prev.CompressorHz, curr.CompressorHz) {
		return true
	}
	if !ptrBoolEq(prev.OutdoorActive, curr.OutdoorActive) {
		return true
	}
	if !ptrStrEq(prev.OpMode, curr.OpMode) {
		return true
	}
	if !ptrBoolEq(prev.HeatDemand, curr.HeatDemand) {
		return true
	}
	if !ptrBoolEq(prev.CompressorRun, curr.CompressorRun) {
		return true
	}
	if !ptrBoolEq(prev.RefrigerantOn, curr.RefrigerantOn) {
		return true
	}
	if !ptrIntEq(prev.FanSpeedResp, curr.FanSpeedResp) {
		return true
	}
	return false
}

// maxTempDeltaLGCP 는 모든 온도 센서값(IndoorTempC + PipeTemp1C + PipeTemp2C)의
// 최대 |Δ| 를 반환한다 (v0.6.7). 한쪽만 nil 이면 큰 값 반환 (게이트 우회).
func maxTempDeltaLGCP(prev, curr LGCPDeviceState) float64 {
	d := ptrFloat64AbsDelta(prev.IndoorTempC, curr.IndoorTempC)
	if x := ptrFloat64AbsDelta(prev.PipeTemp1C, curr.PipeTemp1C); x > d {
		d = x
	}
	if x := ptrFloat64AbsDelta(prev.PipeTemp2C, curr.PipeTemp2C); x > d {
		d = x
	}
	return d
}

// stateChanged 는 두 상태를 비교하여 주요 필드가 변경되었는지 판별한다.
func stateChanged(prev, curr LGCPDeviceState) bool {
	if !ptrStrEq(prev.PowerState, curr.PowerState) {
		return true
	}
	if !ptrStrEq(prev.Power, curr.Power) {
		return true
	}
	if !ptrF64Eq(prev.IndoorTempC, curr.IndoorTempC) {
		return true
	}
	if !ptrF64Eq(prev.SetTempC, curr.SetTempC) {
		return true
	}
	if !ptrStrEq(prev.FanSpeed, curr.FanSpeed) {
		return true
	}
	if !ptrStrEq(prev.Mode, curr.Mode) {
		return true
	}
	if !ptrBoolEq(prev.ValveOpen, curr.ValveOpen) {
		return true
	}
	if !ptrIntEq(prev.FanMotorHz, curr.FanMotorHz) {
		return true
	}
	if !ptrIntEq(prev.CompressorCap, curr.CompressorCap) {
		return true
	}
	if !ptrIntEq(prev.CompressorHz, curr.CompressorHz) {
		return true
	}
	if !ptrBoolEq(prev.OutdoorActive, curr.OutdoorActive) {
		return true
	}
	if !ptrStrEq(prev.OpMode, curr.OpMode) {
		return true
	}
	if !ptrBoolEq(prev.HeatDemand, curr.HeatDemand) {
		return true
	}
	if !ptrBoolEq(prev.CompressorRun, curr.CompressorRun) {
		return true
	}
	if !ptrBoolEq(prev.RefrigerantOn, curr.RefrigerantOn) {
		return true
	}
	return false
}

// modeToCanonical 은 프로토콜별 모드 값을 통일된 이름으로 변환한다.
// v0.7.4: NASA/LG ICP-01/Century 와 통일 — "cool"/"heat"/"dry"/"fan"/"auto"
// (이전: cooling/heating/dehumidify 같은 외장형 명칭)
var modeToCanonical = map[string]string{
	"cool": "cool", "cooling": "cool",
	"heat": "heat", "heating": "heat",
	"auto": "auto",
	"dry":  "dry", "dehumidify": "dry",
	"fan": "fan",
}

// isPowerOn 은 전원 상태를 판별한다.
func (s *LGCPDeviceState) isPowerOn() bool {
	if s.PowerState != nil {
		return *s.PowerState == "ON"
	}
	if s.Power != nil {
		return *s.Power == "ON"
	}
	return false
}

// toProperties 는 디바이스 상태를 map[string]any 로 변환한다 (Device 인터페이스 용).
// 속성명은 NASA 에이전트와 통일: power(bool), current_temp, target_temp, mode("cooling"/"heating"/…).
// devType 에 따라 해당 디바이스 유형의 속성만 노출한다.
// 실내기 전원 OFF 시 운전 관련 속성은 "-" 로 표시한다.
func (s *LGCPDeviceState) toProperties(devType string) map[string]any {
	switch devType {
	case "controller":
		return s.controllerProperties()
	default: // indoor
		return s.indoorProperties()
	}
}

// indoorProperties 는 실내기 속성만 반환한다.
func (s *LGCPDeviceState) indoorProperties() map[string]any {
	props := make(map[string]any)
	powerOn := s.isPowerOn()

	// power 는 항상 출력
	if s.PowerState != nil || s.Power != nil {
		props["power"] = powerOn
	}
	// 전원 OFF 시 운전 관련 속성은 표시하지 않음
	if powerOn {
		if s.IndoorTempC != nil {
			props["current_temperature"] = *s.IndoorTempC
		}
		if s.SetTempC != nil {
			props["target_temperature"] = *s.SetTempC
		}
		// v0.7.5: fan_speed / mode 를 hvac 통일 ID (int) 로 변환.
		if s.FanSpeed != nil {
			props["fan_speed"] = hvac.FanSpeedFromName(*s.FanSpeed)
		} else {
			props["fan_speed"] = hvac.FanOff
		}
		if s.Mode != nil {
			canonical := *s.Mode
			if v, ok := modeToCanonical[*s.Mode]; ok {
				canonical = v
			}
			props["mode"] = hvac.ModeFromName(canonical)
		} else {
			props["mode"] = hvac.ModeOffOrAuto
		}
		if s.ValveOpen != nil {
			props["valve_open"] = *s.ValveOpen
		}
		if s.FanMotorHz != nil {
			props["fan_motor_hz"] = *s.FanMotorHz
		}
		if s.PipeTemp1C != nil {
			props["pipe_temperature1_c"] = *s.PipeTemp1C
		}
		if s.PipeTemp2C != nil {
			props["pipe_temperature2_c"] = *s.PipeTemp2C
		}
	} else {
		// v0.7.5: 전원 OFF — mode/fan_speed 는 통일 ID 0 으로 노출 (운영 호환).
		props["target_temperature"] = "-"
		props["fan_speed"] = hvac.FanOff
		props["mode"] = hvac.ModeOffOrAuto
		props["valve_open"] = "-"
		props["fan_motor_hz"] = "-"
		props["pipe_temperature1_c"] = "-"
		props["pipe_temperature2_c"] = "-"
	}
	return props
}

// controllerProperties 는 컨트롤러/실외기 속성만 반환한다.
func (s *LGCPDeviceState) controllerProperties() map[string]any {
	props := make(map[string]any)
	if s.CompressorCap != nil {
		props["compressor_cap"] = *s.CompressorCap
	}
	if s.CompressorHz != nil {
		props["compressor_hz"] = *s.CompressorHz
	}
	if s.CompressorRun != nil {
		props["compressor_run"] = *s.CompressorRun
	}
	if s.OutdoorActive != nil {
		props["outdoor_active"] = *s.OutdoorActive
	}
	if s.HeatDemand != nil {
		props["heat_demand"] = *s.HeatDemand
	}
	if s.RefrigerantOn != nil {
		props["refrigerant_on"] = *s.RefrigerantOn
	}
	if s.OpMode != nil {
		props["op_mode"] = *s.OpMode
	}
	return props
}

// ---------------------------------------------------------------------------
// 포인터 비교 헬퍼
// ---------------------------------------------------------------------------

func ptrStrEq(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func ptrF64Eq(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func ptrBoolEq(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func ptrIntEq(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// detectLGCPDeviceType 은 LGCP 주소에서 디바이스 타입을 추정한다.
func detectLGCPDeviceType(addrHex string) string {
	switch {
	case addrHex == "44550000":
		return "controller"
	case addrHex == "ffffffff":
		return "broadcast"
	default:
		return "HVACR.IDU" // v0.18.3
	}
}
