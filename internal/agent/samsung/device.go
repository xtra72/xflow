package samsung

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/agent/hvac"
)

// NASADevice 는 Samsung NASA HVAC 디바이스를 나타낸다.
type NASADevice struct {
	Address    NASAAddress
	DeviceID   string // 사용자 지정 디바이스 식별자 (비어 있을 수 있음)
	Name       string // 사용자 정의 디바이스 이름 (비어 있을 수 있음)
	Type       string // "indoor", "outdoor", "controller"
	Online     bool
	Ready      bool // 통신 준비 완료 (실외기: C015 0xAx)
	LastSeen   time.Time
	State      *NASADeviceState // 현재 상태 (실내기 전용)
	ErrorCount int
	Source     string // "config", "bridge", "auto", "discovery"
}

// HexKeyByteMap 는 uint16 키를 16진수 문자열("0x0402")로 직렬화하는 바이트맵이다.
// JSON 출력 시 키가 10진수("1026") 대신 16진수로 표현된다.
type HexKeyByteMap map[uint16][]byte

// MarshalJSON 은 uint16 키를 "0x0402" 형식의 16진수 문자열로 변환한다.
func (m HexKeyByteMap) MarshalJSON() ([]byte, error) {
	out := make(map[string][]byte, len(m))
	for k, v := range m {
		out[fmt.Sprintf("0x%04X", k)] = v
	}
	return json.Marshal(out)
}

// UnmarshalJSON 은 "0x0402"(16진수) 및 "1026"(10진수) 형식 모두를 지원한다.
func (m *HexKeyByteMap) UnmarshalJSON(data []byte) error {
	var raw map[string][]byte
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = make(HexKeyByteMap, len(raw))
	for k, v := range raw {
		var idx uint64
		var err error
		if strings.HasPrefix(k, "0x") || strings.HasPrefix(k, "0X") {
			idx, err = strconv.ParseUint(k[2:], 16, 16)
		} else {
			idx, err = strconv.ParseUint(k, 10, 16)
		}
		if err != nil {
			return fmt.Errorf("invalid message set key %q: %w", k, err)
		}
		(*m)[uint16(idx)] = v
	}
	return nil
}

// NASADeviceState 는 실내기의 현재 운전 상태를 나타낸다.
// JSON 직렬화 시 모든 필드는 snake_case 키로 출력된다.
type NASADeviceState struct {
	Power          bool          `json:"power"`
	Mode           string        `json:"mode"` // "cool", "heat", "dry", "fan", "auto"
	TargetTemp     float32       `json:"target_temperature"`
	CurrentTemp    float32       `json:"current_temperature"`
	FanSpeed       string        `json:"fan_speed"` // "auto", "low", "medium", "high"
	SwingVertical  bool          `json:"swing_vertical"`
	FilterAlarm    bool          `json:"filter_alarm"`
	ErrorCode      uint16        `json:"error_code"`
	RawMessageSets HexKeyByteMap `json:"raw_message_sets"` // 수신된 모든 메시지 세트

	// observedCore 는 5 핵심 필드의 관측 여부를 나타내는 bitmask 이다 (json 미직렬화).
	//
	// 사용자 보고 "초기값 0, 빈 string 이 emit 되는 결함" 의 fix —
	// 5 핵심 필드 모두 observed (== observedAllCore) 되기 전에는 emit 보류한다.
	// UpdateFromMessageSets 가 각 메시지 셋 처리 시 해당 bit 를 set 한다.
	observedCore uint8 `json:"-"`
}

// 5 핵심 필드의 observedCore bitmask. AllCoreObserved 는 모두 set 된 값이다.
const (
	observedPower       uint8 = 1 << 0 // 0x01
	observedMode        uint8 = 1 << 1 // 0x02
	observedTargetTemp  uint8 = 1 << 2 // 0x04
	observedCurrentTemp uint8 = 1 << 3 // 0x08
	observedFanSpeed    uint8 = 1 << 4 // 0x10
	observedAllCore     uint8 = observedPower | observedMode | observedTargetTemp |
		observedCurrentTemp | observedFanSpeed
)

// AllCoreObserved 는 5 핵심 필드 (power/mode/target_temp/current_temp/fan_speed)
// 가 모두 적어도 한 번 관측되었는지 반환한다. emit gate 에 사용 (v0.x — NASA dedup fix).
func (s *NASADeviceState) AllCoreObserved() bool {
	return s.observedCore == observedAllCore
}

// stateOutput 은 노드로 송신되는 JSON 직렬화용 상태 구조체이다 (v0.7.5).
// Mode / FanSpeed 는 hvac 패키지의 통일 ID (int) 로 변환되어 출력된다.
// 키는 NASADeviceState 와 동일하게 snake_case.
type stateOutput struct {
	Power          bool          `json:"power"`
	Mode           int           `json:"mode"` // v0.7.5: 통일 ID (off/auto=0, cool=1, heat=2, dry=3, fan=4)
	TargetTemp     float32       `json:"target_temperature"`
	CurrentTemp    float32       `json:"current_temperature"`
	FanSpeed       int           `json:"fan_speed"` // v0.7.5: 통일 ID (off=0, auto=1, quiet=2, low=3, medium=4, high=5, turbo=6)
	SwingVertical  bool          `json:"swing_vertical"`
	FilterAlarm    bool          `json:"filter_alarm"`
	ErrorCode      uint16        `json:"error_code"`
	RawMessageSets HexKeyByteMap `json:"raw_message_sets,omitempty"`
}

// StateForJSON 은 includeRaw 여부에 따라 JSON 직렬화용 상태를 반환한다 (v0.7.5).
// Mode / FanSpeed 는 hvac 통일 ID 로 변환. Power=false 면 mode=0, fan_speed=0.
func (s *NASADeviceState) StateForJSON(includeRaw bool) any {
	out := &stateOutput{
		Power:         s.Power,
		Mode:          hvac.ModeFromName(s.Mode),
		TargetTemp:    s.TargetTemp,
		CurrentTemp:   s.CurrentTemp,
		FanSpeed:      hvac.FanSpeedFromName(s.FanSpeed),
		SwingVertical: s.SwingVertical,
		FilterAlarm:   s.FilterAlarm,
		ErrorCode:     s.ErrorCode,
	}
	if !s.Power {
		out.Mode = hvac.ModeOffOrAuto
		out.FanSpeed = hvac.FanOff
	}
	if includeRaw {
		out.RawMessageSets = s.RawMessageSets
	}
	return out
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
//
// 5 핵심 필드 (power/mode/target_temp/current_temp/fan_speed) 가 한 번이라도
// 처리되면 observedCore bitmask 의 해당 bit 가 set 된다. emit 보류 gate 에 사용.
func (s *NASADeviceState) UpdateFromMessageSets(sets []NASAMessageSet) {
	for _, ms := range sets {
		// 모든 수신된 메시지 세트를 RawMessageSets 에 저장한다.
		raw := make([]byte, len(ms.Value))
		copy(raw, ms.Value)
		s.RawMessageSets[ms.Index] = raw

		switch ms.Index {
		case MsgPower:
			s.Power = ms.Value[0] != 0
			s.observedCore |= observedPower
		case MsgMode:
			if name, ok := ModeToString[ms.Value[0]]; ok {
				s.Mode = name
				s.observedCore |= observedMode
			}
		case MsgFanSpeed:
			if name, ok := FanSpeedToString[ms.Value[0]]; ok {
				s.FanSpeed = name
				s.observedCore |= observedFanSpeed
			}
		case MsgTargetTemp:
			if len(ms.Value) >= 2 {
				s.TargetTemp = DecodeTemperature(binary.BigEndian.Uint16(ms.Value[:2]))
				s.observedCore |= observedTargetTemp
			}
		case MsgCurrentTemp:
			if len(ms.Value) >= 2 {
				s.CurrentTemp = DecodeTemperature(binary.BigEndian.Uint16(ms.Value[:2]))
				s.observedCore |= observedCurrentTemp
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
