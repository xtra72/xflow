package lg

import (
	"time"

	"github.com/xtra/xflow/internal/agent/hvac"
)

// LGAPDevice 는 LG LGAP HVAC 디바이스를 나타낸다.
type LGAPDevice struct {
	Zone       byte   // 존 주소 바이트 (상위 니블=그룹, 하위 니블=유닛)
	UnitID     string // v0.18.7: 사용자 지정 디바이스 식별자 / 프로토콜 unit id (이전 DeviceID, 비어 있을 수 있음)
	Name       string // 사용자 정의 디바이스 이름 (비어 있을 수 있음)
	Online     bool
	LastSeen   time.Time
	State      *LGAPDeviceState // 현재 상태
	ErrorCount int
	Source     string // "config", "bridge"

	// connInitialEmitted 는 device_connection.initial 이 이 device 에 대해 이미
	// 방출되었는지를 나타낸다 (SPEC-HVACR-CONNSTATE-001 §4.6.5/§4.6.6).
	// 프로세스 수명당 device 별 1회 initial 을 보장(N9)하고, per-device 순서 보장
	// (initial 이 첫 change/report 보다 먼저, E9/S5)의 게이트로 사용된다.
	// a.mu 하에서만 접근한다.
	connInitialEmitted bool
}

// LGAPDeviceState 는 실내기의 현재 운전 상태를 나타낸다.
type LGAPDeviceState struct {
	Power       bool    `json:"power"`
	Mode        string  `json:"mode"`                 // "cool", "heat", "dry", "fan", "auto"
	FanSpeed    string  `json:"fan_speed"`            // "low", "medium", "high", "auto", "slow", "turbo"
	TargetTemp  int     `json:"target_temperature"`   // 설정 온도 (섭씨)
	RoomTemp    float32 `json:"current_temperature"`  // 실내 온도 (섭씨) — v0.x: NASA/Century 통일 (이전 room_temp)
	PipeInTemp  float32 `json:"pipe_in_temperature"`  // 파이프 입구 온도 (섭씨)
	PipeOutTemp float32 `json:"pipe_out_temperature"` // 파이프 출구 온도 (섭씨)
	ZoneLoad    byte    `json:"zone_load"`            // 존 부하 (204=유휴)
	ZonePower   byte    `json:"zone_power"`           // 존 전원 플래그 (0=운전중, 1=정지)
	DesignLoad  byte    `json:"design_load"`          // 설계 부하
	ODULoad     byte    `json:"odu_load"`             // 실외기 총 부하
	ErrorCode   byte    `json:"error_code"`           // 에러 코드 (0=에러 없음)
	Locked      bool    `json:"locked"`               // 잠금 상태
	Plasma      bool    `json:"plasma"`               // 플라즈마 상태
	SwingAuto   bool    `json:"swing_auto"`           // 스윙 자동 여부
}

// UpdateFromResponse 는 LGAP 응답으로부터 디바이스 상태를 업데이트한다.
func (s *LGAPDeviceState) UpdateFromResponse(resp *LGAPResponse) {
	// 플래그에서 전원/잠금/플라즈마 추출
	s.Power = resp.FlagsEcho&FlagPower != 0
	s.Locked = resp.FlagsEcho&FlagLock != 0
	s.Plasma = resp.FlagsEcho&FlagPlasma != 0

	// 모드/팬/스윙 디코딩
	mode, fan, swingAuto := DecodeModeCombo(resp.ModeCombo)
	if name, ok := ModeToString[mode]; ok {
		s.Mode = name
	}
	if name, ok := FanSpeedToString[fan]; ok {
		s.FanSpeed = name
	}
	s.SwingAuto = swingAuto

	// 온도 디코딩
	s.TargetTemp = DecodeTargetTemp(resp.TargetTemp)
	s.RoomTemp = DecodeMeasuredTemp(resp.RoomTemp)
	s.PipeInTemp = DecodeMeasuredTemp(resp.PipeInTemp)
	s.PipeOutTemp = DecodeMeasuredTemp(resp.PipeOutTemp)

	// 부하 및 전원 정보
	s.ZoneLoad = resp.ZoneLoad
	s.ZonePower = resp.ZonePower
	s.DesignLoad = resp.DesignLoad
	s.ODULoad = resp.ODULoad
	s.ErrorCode = resp.Error
}

// lgapStateOutput 은 노드로 송신되는 JSON 직렬화용 상태 구조체이다 (v0.7.5).
// Mode / FanSpeed 는 hvac 통일 ID (int) 로 변환되어 출력된다.
type lgapStateOutput struct {
	Power       bool    `json:"power"`
	Mode        int     `json:"mode"`      // v0.7.5: hvac 통일 ID
	FanSpeed    int     `json:"fan_speed"` // v0.7.5: hvac 통일 ID
	TargetTemp  int     `json:"target_temperature"`
	RoomTemp    float32 `json:"current_temperature"`
	PipeInTemp  float32 `json:"pipe_in_temperature"`
	PipeOutTemp float32 `json:"pipe_out_temperature"`
	ZoneLoad    byte    `json:"zone_load"`
	ZonePower   byte    `json:"zone_power"`
	DesignLoad  byte    `json:"design_load"`
	ODULoad     byte    `json:"odu_load"`
	ErrorCode   byte    `json:"error_code"`
	Locked      bool    `json:"locked"`
	Plasma      bool    `json:"plasma"`
	SwingAuto   bool    `json:"swing_auto"`
}

// StateForJSON 은 JSON 직렬화용 상태를 반환한다.
// v0.7.5: mode / fan_speed 를 hvac 통일 ID 로 변환. Power=false 면 0 으로 고정.
func (s *LGAPDeviceState) StateForJSON() any {
	out := &lgapStateOutput{
		Power:       s.Power,
		Mode:        hvac.ModeFromName(s.Mode),
		FanSpeed:    hvac.FanSpeedFromName(s.FanSpeed),
		TargetTemp:  s.TargetTemp,
		RoomTemp:    s.RoomTemp,
		PipeInTemp:  s.PipeInTemp,
		PipeOutTemp: s.PipeOutTemp,
		ZoneLoad:    s.ZoneLoad,
		ZonePower:   s.ZonePower,
		DesignLoad:  s.DesignLoad,
		ODULoad:     s.ODULoad,
		ErrorCode:   s.ErrorCode,
		Locked:      s.Locked,
		Plasma:      s.Plasma,
		SwingAuto:   s.SwingAuto,
	}
	if !s.Power {
		out.Mode = hvac.ModeOffOrAuto
		out.FanSpeed = hvac.FanOff
	}
	return out
}
