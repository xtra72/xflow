package lg

import "time"

// LGAPDevice 는 LG LGAP HVAC 디바이스를 나타낸다.
type LGAPDevice struct {
	Zone       byte             // 존 주소 바이트 (상위 니블=그룹, 하위 니블=유닛)
	DeviceID   string           // 사용자 지정 디바이스 식별자 (비어 있을 수 있음)
	Name       string           // 사용자 정의 디바이스 이름 (비어 있을 수 있음)
	Online     bool
	LastSeen   time.Time
	State      *LGAPDeviceState // 현재 상태
	ErrorCount int
	Source     string           // "config", "bridge"
}

// LGAPDeviceState 는 실내기의 현재 운전 상태를 나타낸다.
type LGAPDeviceState struct {
	Power       bool
	Mode        string  // "cool", "heat", "dry", "fan", "auto"
	FanSpeed    string  // "low", "medium", "high", "auto", "slow", "turbo"
	TargetTemp  int     // 설정 온도 (섭씨)
	RoomTemp    float32 // 실내 온도 (섭씨)
	PipeInTemp  float32 // 파이프 입구 온도 (섭씨)
	PipeOutTemp float32 // 파이프 출구 온도 (섭씨)
	ZoneLoad    byte    // 존 부하 (204=유휴)
	ZonePower   byte    // 존 전원 플래그 (0=운전중, 1=정지)
	DesignLoad  byte    // 설계 부하
	ODULoad     byte    // 실외기 총 부하
	ErrorCode   byte    // 에러 코드 (0=에러 없음)
	Locked      bool    // 잠금 상태
	Plasma      bool    // 플라즈마 상태
	SwingAuto   bool    // 스윙 자동 여부
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

// StateForJSON 은 JSON 직렬화용 상태를 반환한다.
func (s *LGAPDeviceState) StateForJSON() any {
	return s
}
