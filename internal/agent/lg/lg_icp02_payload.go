package lg

import "math"

// ---------------------------------------------------------------------------
// LG ICP-02 페이로드 레지스터-속성 쌍 디코더
// ---------------------------------------------------------------------------

// Icp02RegPair 는 디코딩된 레지스터-속성 쌍이다.
type Icp02RegPair struct {
	Reg  byte // 레지스터 ID
	Attr byte // 속성 바이트
	Ext  byte // 확장 값 (3바이트 인코딩 시)
	Len  int  // 2 또는 3
}

// Icp02DecodedPayload 는 디코딩된 페이로드 결과이다.
type Icp02DecodedPayload struct {
	// 레지스터-속성 쌍 원본 (프로토콜 분석용, 알람에서는 불필요)
	Pairs []Icp02RegPairJSON `json:"pairs,omitempty"`

	// 해석된 필드 (알려진 레지스터만)
	Power         *string  `json:"power,omitempty"`          // "ON" / "OFF" (제어 명령: 0x18 0x4_)
	PowerState    *string  `json:"power_state,omitempty"`    // "ON" / "OFF" (응답 상태: 0x60 0xC_)
	SetTempC      *float64 `json:"target_temperature,omitempty"`    // 설정 온도 (°C) — v0.x: NASA/Century 와 통일 (이전 set_temp_c)
	FanSpeed      *string  `json:"fan_speed,omitempty"`      // "low" / "medium" / "high" / "turbo" / "auto"
	Mode          *string  `json:"mode,omitempty"`           // "cool" / "dry" / "fan" / "auto" / "heat"
	CompressorCap *int     `json:"compressor_cap,omitempty"` // 압축기 용량 (4비트 값)
	CompressorHz  *int     `json:"compressor_hz,omitempty"`  // 압축기 주파수 (확장 Hz)
	ValveOpen     *bool    `json:"valve_open,omitempty"`     // 냉매 밸브 개폐 (0x62 0x4_)
	FanMotorHz    *int     `json:"fan_motor_hz,omitempty"`   // 실내기 팬모터 주파수 (0x62 0xD_+ext)
	PipeTemp1C    *float64 `json:"pipe_temperature1_c,omitempty"`   // 배관 온도 1 (°C)
	PipeTemp2C    *float64 `json:"pipe_temperature2_c,omitempty"`   // 배관 온도 2 (°C) — 0x62 0xD0+V 두 번째
	IndoorTempC   *float64 `json:"current_temperature,omitempty"`   // 실내 온도 (°C) — v0.x: NASA/Century 와 통일 (이전 indoor_temp_c)
	FanSpeedResp  *int     `json:"fan_speed_resp,omitempty"` // 풍속 응답 (0x71)
	OutdoorActive *bool    `json:"outdoor_active,omitempty"` // 실외기 활성 (0x10 0xC_)
	HeatDemand    *bool    `json:"heat_demand,omitempty"`    // 난방 요구 신호 (0x11 0x0_)
	CompressorRun *bool    `json:"compressor_run,omitempty"` // 압축기 운전 플래그 (0x12 0x4_)
	RefrigerantOn *bool    `json:"refrigerant_on,omitempty"` // 냉매 회로 운전 (0x1A 0xC_)
	OpMode        *string  `json:"op_mode,omitempty"`        // 운전 모드 (0x13 0xC_)
}

// Icp02RegPairJSON 은 레지스터-속성 쌍의 JSON 표현이다.
type Icp02RegPairJSON struct {
	Reg  string `json:"r"`           // 레지스터 (hex)
	Attr string `json:"a"`           // 속성 (hex)
	Ext  string `json:"x,omitempty"` // 확장 값 (hex, 3바이트일 때만)
}

// isExtendedAttr 은 속성 바이트의 상위 니블이 3바이트 확장 인코딩인지 판별한다.
// 확장 타입: 0x1_, 0x5_, 0x9_, 0xD_ → 3바이트
func isExtendedAttr(attr byte) bool {
	hi := attr >> 4
	return hi == 0x1 || hi == 0x5 || hi == 0x9 || hi == 0xD
}

// parseRegPairs 는 페이로드 바이트를 레지스터-속성 쌍 목록으로 분리한다.
func parseRegPairs(payload []byte) []Icp02RegPair {
	var pairs []Icp02RegPair
	i := 0
	for i < len(payload) {
		if i+1 >= len(payload) {
			break // 잔여 바이트 무시
		}
		reg := payload[i]
		attr := payload[i+1]

		if isExtendedAttr(attr) {
			if i+2 >= len(payload) {
				// 확장 바이트 부족 — 2바이트로 처리
				pairs = append(pairs, Icp02RegPair{Reg: reg, Attr: attr, Len: 2})
				i += 2
			} else {
				ext := payload[i+2]
				pairs = append(pairs, Icp02RegPair{Reg: reg, Attr: attr, Ext: ext, Len: 3})
				i += 3
			}
		} else {
			pairs = append(pairs, Icp02RegPair{Reg: reg, Attr: attr, Len: 2})
			i += 2
		}
	}
	return pairs
}

// DecodePayload 는 페이로드 바이트를 파싱하여 레지스터 쌍과 해석 결과를 반환한다.
func DecodePayload(payload []byte) *Icp02DecodedPayload {
	if len(payload) == 0 {
		return nil
	}

	pairs := parseRegPairs(payload)
	d := &Icp02DecodedPayload{}

	// JSON 표현 생성
	d.Pairs = make([]Icp02RegPairJSON, len(pairs))
	for i, p := range pairs {
		d.Pairs[i] = Icp02RegPairJSON{
			Reg:  hexByte(p.Reg),
			Attr: hexByte(p.Attr),
		}
		if p.Len == 3 {
			ext := hexByte(p.Ext)
			d.Pairs[i].Ext = ext
		}
	}

	// 해석: 이미 파싱된 pair를 순회하며 알려진 레지스터를 해석
	reg62Count := 0 // 0x62 확장값 출현 횟수 (첫째=배관온도2, 둘째=압축기주파수)
	for _, p := range pairs {
		interpretPair(d, p, &reg62Count)
	}

	return d
}

// interpretPair 는 단일 레지스터-속성 쌍을 해석하여 d에 채운다.
func interpretPair(d *Icp02DecodedPayload, p Icp02RegPair, reg62Count *int) {
	hi := p.Attr >> 4
	lo := p.Attr & 0x0F

	switch p.Reg {
	case 0x60:
		// 실내기 전원 상태 (응답): attr=0xC0(OFF)/0xC1(ON)
		if hi == 0xC {
			if lo&0x01 != 0 {
				s := "ON"
				d.PowerState = &s
			} else {
				s := "OFF"
				d.PowerState = &s
			}
		}

	case 0x18:
		// 전원 ON/OFF: attr=0x40(OFF)/0x41(ON)
		if hi == 0x4 {
			if lo&0x01 != 0 {
				s := "ON"
				d.Power = &s
			} else {
				s := "OFF"
				d.Power = &s
			}
		}
		// 압축기 용량 4비트: attr=0x8V
		if hi == 0x8 {
			v := int(lo)
			d.CompressorCap = &v
		}
		// 압축기 확장 Hz: attr=0x9_, ext=Hz
		if hi == 0x9 && p.Len == 3 {
			v := int(p.Ext)
			d.CompressorHz = &v
		}

	case 0x64:
		// 설정 온도: attr=0x8V → V+15 = °C
		if hi == 0x8 {
			temp := float64(lo) + 15
			d.SetTempC = &temp
		}
		// 풍속+모드 복합: attr=0x50, ext=XY
		if hi == 0x5 && p.Len == 3 {
			fanCode := p.Ext >> 4
			modeCode := p.Ext & 0x0F
			fan := decodeFanSpeed(fanCode)
			mode := decodeMode(modeCode)
			d.FanSpeed = &fan
			d.Mode = &mode
		}

	case 0x71:
		// 풍속 응답: attr=0x0V
		if hi == 0x0 {
			v := int(lo)
			d.FanSpeedResp = &v
		}

	case 0x61:
		// 실내 온도: attr=0x9_, ext=V → (157V - V² - 796) / 162, 0.5°C 단위 반올림
		// NTC 열저항 비선형성 보정 2차 다항식 (제조사 앱 검증 완료, 23~30°C 구간)
		if hi == 0x9 && p.Len == 3 {
			v := float64(p.Ext)
			raw := (157*v - v*v - 796) / 162
			temp := math.Round(raw*2) / 2
			d.IndoorTempC = &temp
		}
		// 배관 온도 1: attr=0xD0+V, ext=value
		if hi == 0xD && p.Len == 3 {
			temp := float64(p.Ext) / 2
			d.PipeTemp1C = &temp
		}

	case 0x62:
		// 실내기 팬모터 주파수 (확장): attr=0xD0+V, ext=value
		if hi == 0xD && p.Len == 3 {
			*reg62Count++
			if *reg62Count == 1 {
				// plen=41 응답에서 0x62 0xD0+V = 실내기 팬모터 주파수 (Hz)
				hz := int(p.Ext)
				d.FanMotorHz = &hz
			}
		}
		// 냉매 밸브 개폐: attr=0x4_
		if hi == 0x4 {
			open := (lo & 0x01) != 0
			d.ValveOpen = &open
		}
		// 배관 온도 2: attr=0x1_, ext=value
		if hi == 0x1 && p.Len == 3 {
			temp := float64(p.Ext) / 2
			d.PipeTemp2C = &temp
		}

	case 0x74:
		// 고정값 (0x19=25) — 실내 온도가 아님, 무시

	case 0x10:
		// 실외기 활성: attr=0xC0(비활성)/0xC1(활성)
		if hi == 0xC {
			active := (lo & 0x01) != 0
			d.OutdoorActive = &active
		}

	case 0x11:
		// 난방 요구 신호: attr=0x00(OFF)/0x01(ON)
		if hi == 0x0 {
			demand := (lo & 0x01) != 0
			d.HeatDemand = &demand
		}

	case 0x12:
		// 압축기 운전 플래그: attr=0x40(OFF)/0x41(ON)
		if hi == 0x4 {
			run := (lo & 0x01) != 0
			d.CompressorRun = &run
		}

	case 0x13:
		// 운전 모드: attr=0xC_
		if hi == 0xC {
			mode := decodeOpMode(lo)
			d.OpMode = &mode
		}

	case 0x1A:
		// 냉매 회로 운전: attr=0xC0(OFF)/0xC1(ON)
		if hi == 0xC {
			on := (lo & 0x01) != 0
			d.RefrigerantOn = &on
		}
	}
}

// hexByte 는 바이트를 2자리 16진수 문자열로 변환한다.
func hexByte(b byte) string {
	const hex = "0123456789abcdef"
	return string([]byte{hex[b>>4], hex[b&0x0F]})
}

// decodeFanSpeed 는 풍속 코드를 문자열로 변환한다.
func decodeFanSpeed(code byte) string {
	switch code {
	case 1:
		return "low"
	case 2:
		return "medium"
	case 3:
		return "high"
	case 4:
		return "turbo"
	case 5:
		return "auto"
	default:
		return "unknown"
	}
}

// decodeMode 는 운전 모드 코드를 문자열로 변환한다.
func decodeMode(code byte) string {
	switch code {
	case 0:
		return "cool"
	case 1:
		return "dry"
	case 2:
		return "fan"
	case 3:
		return "auto"
	case 4:
		return "heat"
	default:
		return "unknown"
	}
}

// decodeOpMode 는 0x13 운전 모드 하위 니블을 문자열로 변환한다.
func decodeOpMode(lo byte) string {
	switch lo {
	case 0x0:
		return "normal"
	case 0x1:
		return "heating"
	case 0x3:
		return "defrost"
	case 0x6:
		return "defrost-transition"
	default:
		return "unknown"
	}
}
