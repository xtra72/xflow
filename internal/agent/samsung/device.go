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

// NasaDevice 는 Samsung NASA HVAC 디바이스를 나타낸다.
type NasaDevice struct {
	Address    NasaAddress
	UnitID     string // v0.18.7: 사용자 지정 디바이스 식별자 / 프로토콜 unit id (이전 DeviceID, 비어 있을 수 있음)
	Name       string // 사용자 정의 디바이스 이름 (비어 있을 수 있음)
	Type       string // "HVACR.IDU", "HVACR.ODU", "controller"
	Online     bool
	Ready      bool // 통신 준비 완료 (실외기: C015 0xAx)
	LastSeen   time.Time
	State      *NasaDeviceState // 현재 상태 (실내기 IDU 전용)
	Outdoor    *OutdoorState    // 현재 상태 (실외기 ODU 전용) — IDU 는 nil, ODU 는 State 가 nil
	ErrorCount int
	Source     string // "config", "bridge", "auto", "discovery"
	// ReportEnabled 는 디바이스별 상태 전송 on/off 이다(기본 true=on). false 면 이 디바이스에
	// 대한 device_state 리포트/변경(연결 정보 포함)과 모든 디바이스 이벤트를 노드로 방출하지
	// 않는다. 트랜스포트 단위 이벤트(transport_*)는 게이트하지 않는다.
	// a.mu 하에서만 접근한다(단순 필드 읽기이므로 기존 락 하에서 안전).
	ReportEnabled bool
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

// NasaDeviceState 는 실내기의 현재 운전 상태를 나타낸다.
// JSON 직렬화 시 모든 필드는 snake_case 키로 출력된다.
type NasaDeviceState struct {
	Power           bool          `json:"power"`
	Mode            string        `json:"mode"` // "cool", "heat", "dry", "fan", "auto"
	TargetTemp      float32       `json:"target_temperature"`
	CurrentTemp     float32       `json:"current_temperature"`
	CurrentHumidity uint8         `json:"current_humidity"` // 현재 습도(%) — NASA V1.1 지표 세트 #10 (0x4038), raw uint8 (0~100)
	FanSpeed        string        `json:"fan_speed"`        // "auto", "low", "medium", "high"
	SwingVertical   bool          `json:"swing_vertical"`
	FilterAlarm     bool          `json:"filter_alarm"`
	ErrorCode       uint16        `json:"error_code"`
	RawMessageSets  HexKeyByteMap `json:"raw_message_sets"` // 수신된 모든 메시지 세트

	// observedCore 는 각 상태 필드의 관측 여부를 나타내는 bitmask 이다 (json 미직렬화).
	//
	// 관측 기반 emit: 디바이스 메시지 셋으로 한 번이라도 관측된 필드만 노드로
	// 전송한다("확인된 값만 전송"). 관측되지 않은 필드는 zero-value("" / 0)로 채워
	// 보내지 않고 payload 에서 생략한다. UpdateFromMessageSets 가 각 메시지 셋 처리
	// 시 해당 bit 를 set 하고, StateForJSON 이 set 된 필드만 출력한다.
	//
	// v1.1: 8비트(observedPower..observedError)가 모두 사용되어 uint16 으로 확장했다.
	// observedHumidity(1<<8)가 9번째 bit 로 추가되었다.
	observedCore uint16 `json:"-"`
}

// 상태 필드별 observedCore bitmask. AllCoreObserved 는 5 핵심 필드가 모두 set 된 값이다.
const (
	observedPower       uint16 = 1 << 0 // 0x01
	observedMode        uint16 = 1 << 1 // 0x02
	observedTargetTemp  uint16 = 1 << 2 // 0x04
	observedCurrentTemp uint16 = 1 << 3 // 0x08
	observedFanSpeed    uint16 = 1 << 4 // 0x10
	observedSwing       uint16 = 1 << 5 // 0x20
	observedFilter      uint16 = 1 << 6 // 0x40
	observedError       uint16 = 1 << 7 // 0x80
	observedHumidity    uint16 = 1 << 8 // 0x100 — v1.1 현재 습도(%) (핵심 5필드 gate 에는 미포함)
	observedAllCore     uint16 = observedPower | observedMode | observedTargetTemp |
		observedCurrentTemp | observedFanSpeed
)

// AllCoreObserved 는 5 핵심 필드 (power/mode/target_temp/current_temp/fan_speed)
// 가 모두 적어도 한 번 관측되었는지 반환한다.
func (s *NasaDeviceState) AllCoreObserved() bool {
	return s.observedCore&observedAllCore == observedAllCore
}

// observed 는 지정한 필드 bit 가 관측되었는지 반환한다.
func (s *NasaDeviceState) observed(bit uint16) bool {
	return s.observedCore&bit != 0
}

// StateForJSON 은 관측된 상태 필드만 담은 JSON 직렬화용 map 을 반환한다.
//
// 관측 기반 emit("확인된 값만 전송"): 디바이스 메시지 셋으로 한 번이라도 관측된
// 필드만 포함한다. 관측되지 않은 필드는 zero-value("" / 0)로 채워 보내지 않고
// payload 에서 아예 생략한다(예: power off 로 mode/온도/풍량을 보고하지 않는
// 디바이스는 power 만 emit). Mode / FanSpeed 는 hvac 통일 ID(int)로 변환하며,
// Power=false 면 mode=off/auto(0), fan_speed=off(0) 로 정규화한다.
func (s *NasaDeviceState) StateForJSON(includeRaw bool) any {
	out := make(map[string]any, 8)
	// power 도 다른 필드와 동일하게 관측 기반으로만 emit 한다("확인된 값만 전송").
	// power 프레임(0x4000)을 아직 받지 못한 상태(재시작 직후 등)에서 기본값 false 를
	// 방출하면, 대시보드가 실제 on 상태를 default off 로 덮어써 "꺼졌다 켜진" 것처럼
	// 보이는 회귀가 발생한다. 미관측 시 payload 에서 생략해 마지막 확인값을 보존한다.
	if s.observed(observedPower) {
		out["power"] = s.Power
	}
	if s.observed(observedMode) {
		mode := hvac.ModeFromName(s.Mode)
		if !s.Power {
			mode = hvac.ModeOffOrAuto
		}
		out["mode"] = mode
	}
	if s.observed(observedTargetTemp) {
		out["target_temperature"] = s.TargetTemp
	}
	if s.observed(observedCurrentTemp) {
		out["current_temperature"] = s.CurrentTemp
	}
	if s.observed(observedHumidity) {
		out["current_humidity"] = s.CurrentHumidity
	}
	if s.observed(observedFanSpeed) {
		fan := hvac.FanSpeedFromName(s.FanSpeed)
		if !s.Power {
			fan = hvac.FanOff
		}
		out["fan_speed"] = fan
	}
	if s.observed(observedSwing) {
		out["swing_vertical"] = s.SwingVertical
	}
	if s.observed(observedFilter) {
		out["filter_alarm"] = s.FilterAlarm
	}
	if s.observed(observedError) {
		out["error_code"] = s.ErrorCode
	}
	if includeRaw && len(s.RawMessageSets) > 0 {
		out["raw_message_sets"] = s.RawMessageSets
	}
	return out
}

// ---------------------------------------------------------------------------
// 디바이스 타입 감지
// ---------------------------------------------------------------------------

// DetectDeviceType 는 NASA 주소로부터 디바이스 타입을 판별한다.
// 첫 번째 바이트 0x10 -> "HVACR.ODU", 0x20 -> "HVACR.IDU" (v0.18.3),
// AddrController 와 동일하면 "controller", 그 외 "unknown".
func DetectDeviceType(addr NasaAddress) string {
	if addr == AddrController {
		return "controller"
	}
	switch addr[0] {
	case 0x10:
		return "HVACR.ODU"
	case 0x20:
		return "HVACR.IDU"
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
func (s *NasaDeviceState) UpdateFromMessageSets(sets []NasaMessageSet) {
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
		case MsgCurrentHumidity:
			// NASA V1.1 지표 세트 #10 (0x4038): 1바이트 습도 percent (raw uint8, 0~100).
			// 2바이트 온도와 달리 BigEndian 디코드나 음수 처리 없이 그대로 저장한다.
			if len(ms.Value) >= 1 {
				s.CurrentHumidity = ms.Value[0]
				s.observedCore |= observedHumidity
			}
		case MsgSwingVertical:
			s.SwingVertical = ms.Value[0] != 0
			s.observedCore |= observedSwing
		case MsgFilterCleanAlarm:
			s.FilterAlarm = ms.Value[0] != 0
			s.observedCore |= observedFilter
		case MsgErrorCode:
			if len(ms.Value) >= 2 {
				s.ErrorCode = binary.BigEndian.Uint16(ms.Value[:2])
				s.observedCore |= observedError
			}
		}
	}
}
