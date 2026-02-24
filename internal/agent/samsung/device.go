package samsung

import (
	"encoding/binary"
	"time"
)

// NASADevice 는 Samsung NASA HVAC 디바이스를 나타낸다.
type NASADevice struct {
	Address    NASAAddress
	DeviceID   string           // 사용자 지정 디바이스 식별자 (비어 있을 수 있음)
	Type       string           // "indoor", "outdoor", "controller"
	Online     bool
	Ready      bool             // 통신 준비 완료 (실외기: C015 0xAx)
	LastSeen   time.Time
	State      *NASADeviceState // 현재 상태 (실내기 전용)
	ErrorCount int
	Source     string           // "config", "bridge", "auto", "discovery"
}

// NASADeviceState 는 실내기의 현재 운전 상태를 나타낸다.
type NASADeviceState struct {
	Power          bool
	Mode           string            // "cool", "heat", "dry", "fan", "auto"
	TargetTemp     float32
	CurrentTemp    float32
	FanSpeed       string            // "auto", "low", "medium", "high"
	SwingVertical  bool
	FilterAlarm    bool
	ErrorCode      uint16
	RawMessageSets map[uint16][]byte // 수신된 모든 메시지 세트
}

// ---------------------------------------------------------------------------
// 디바이스 타입 감지
// ---------------------------------------------------------------------------

// DetectDeviceType 는 NASA 주소로부터 디바이스 타입을 판별한다.
// 첫 번째 바이트 0x10 -> "outdoor", 0x20 -> "indoor",
// AddrController 와 동일하면 "controller", 그 외 "unknown".
func DetectDeviceType(addr NASAAddress) string {
	if addr == AddrController {
		return "controller"
	}
	switch addr[0] {
	case 0x10:
		return "outdoor"
	case 0x20:
		return "indoor"
	default:
		return "unknown"
	}
}

// ---------------------------------------------------------------------------
// 모드 매핑
// ---------------------------------------------------------------------------

// ModeToString 은 NASA 모드 바이트를 문자열로 변환하는 맵이다.
var ModeToString = map[byte]string{
	0x00: "auto",
	0x01: "cool",
	0x02: "dry",
	0x03: "fan",
	0x04: "heat",
}

// StringToMode 는 모드 문자열을 NASA 바이트로 변환하는 맵이다.
var StringToMode = map[string]byte{
	"auto": 0x00,
	"cool": 0x01,
	"dry":  0x02,
	"fan":  0x03,
	"heat": 0x04,
}

// ---------------------------------------------------------------------------
// 팬 속도 매핑
// ---------------------------------------------------------------------------

// FanSpeedToString 은 NASA 팬 속도 바이트를 문자열로 변환하는 맵이다.
var FanSpeedToString = map[byte]string{
	0x00: "auto",
	0x01: "low",
	0x02: "medium",
	0x03: "high",
}

// StringToFanSpeed 는 팬 속도 문자열을 NASA 바이트로 변환하는 맵이다.
var StringToFanSpeed = map[string]byte{
	"auto":   0x00,
	"low":    0x01,
	"medium": 0x02,
	"high":   0x03,
}

// ---------------------------------------------------------------------------
// 온도 인코딩/디코딩
// ---------------------------------------------------------------------------

// EncodeTemperature 는 섭씨 온도를 NASA uint16 인코딩으로 변환한다 (temp * 10, BE).
// 음수 온도는 2의 보수를 사용한다.
func EncodeTemperature(temp float32) uint16 {
	raw := int16(temp * 10)
	return uint16(raw)
}

// DecodeTemperature 는 NASA uint16 인코딩을 섭씨 온도로 변환한다.
// 0x0000~0x7FFF = 양수, 0x8000~0xFFFF = 음수 (2의 보수).
func DecodeTemperature(raw uint16) float32 {
	signed := int16(raw)
	return float32(signed) / 10.0
}

// ---------------------------------------------------------------------------
// 메시지 세트로부터 상태 업데이트
// ---------------------------------------------------------------------------

// UpdateFromMessageSets 는 수신된 메시지 세트로 디바이스 상태를 업데이트한다.
func (s *NASADeviceState) UpdateFromMessageSets(sets []NASAMessageSet) {
	for _, ms := range sets {
		// 모든 수신된 메시지 세트를 RawMessageSets 에 저장한다.
		raw := make([]byte, len(ms.Value))
		copy(raw, ms.Value)
		s.RawMessageSets[ms.Index] = raw

		switch ms.Index {
		case MsgPower:
			s.Power = ms.Value[0] != 0
		case MsgMode:
			if name, ok := ModeToString[ms.Value[0]]; ok {
				s.Mode = name
			}
		case MsgFanSpeed:
			if name, ok := FanSpeedToString[ms.Value[0]]; ok {
				s.FanSpeed = name
			}
		case MsgTargetTemp:
			if len(ms.Value) >= 2 {
				s.TargetTemp = DecodeTemperature(binary.BigEndian.Uint16(ms.Value[:2]))
			}
		case MsgCurrentTemp:
			if len(ms.Value) >= 2 {
				s.CurrentTemp = DecodeTemperature(binary.BigEndian.Uint16(ms.Value[:2]))
			}
		case MsgSwingVertical:
			s.SwingVertical = ms.Value[0] != 0
		case MsgFilterCleanAlarm:
			s.FilterAlarm = ms.Value[0] != 0
		case MsgErrorCode:
			if len(ms.Value) >= 2 {
				s.ErrorCode = binary.BigEndian.Uint16(ms.Value[:2])
			}
		}
	}
}
