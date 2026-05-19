package century

import (
	"encoding/json"
	"fmt"
)

// ConfirmationStatus 는 디코딩된 필드의 의미 확신도를 표시하는 마커이다 (REQ-CENTURY-020).
//
// 분류 기준은 REQ-CENTURY-021 에 따른다:
//   - Confirmed : 프로토콜 문서 §8.1 "확정" 섹션 또는 CAP-3/CAP-4 ground truth 로 검증됨
//   - Inferred  : 4 캡처에서 일관된 패턴 또는 합리적 추론, 의미는 미확정이나 거동 확인
//   - Unknown   : 4 캡처 모두 0x00 인 reserved / zero-padding 또는 의미 추정 불가
//
// 향후 캡처로 의미가 확정되면 SPEC 후속 버전에서 동일 필드명을 유지한 채
// Unknown → Inferred → Confirmed 로 진화시킬 수 있다 (downstream 호환).
type ConfirmationStatus uint8

const (
	// Unknown 은 의미 추정 불가 또는 4 캡처 모두 0x00 인 reserved 필드를 표시한다.
	Unknown ConfirmationStatus = iota
	// Inferred 는 합리적 추론은 가능하나 ground truth 검증이 없는 필드를 표시한다.
	Inferred
	// Confirmed 는 프로토콜 문서 또는 CAP-3/CAP-4 로 의미가 확정된 필드를 표시한다.
	Confirmed
)

// String 은 ConfirmationStatus 의 wire 형식 (소문자) 을 반환한다.
// JSON 직렬화도 동일한 문자열을 사용한다 (REQ-CENTURY-020).
func (s ConfirmationStatus) String() string {
	switch s {
	case Confirmed:
		return "confirmed"
	case Inferred:
		return "inferred"
	case Unknown:
		return "unknown"
	default:
		return "unknown"
	}
}

// MarshalJSON 은 ConfirmationStatus 를 lowercase JSON 문자열로 직렬화한다.
func (s ConfirmationStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON 은 lowercase JSON 문자열을 ConfirmationStatus 로 역직렬화한다.
// 알 수 없는 값은 Unknown 으로 처리한다 (forward-compatible).
func (s *ConfirmationStatus) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch raw {
	case "confirmed":
		*s = Confirmed
	case "inferred":
		*s = Inferred
	case "unknown":
		*s = Unknown
	default:
		*s = Unknown
	}
	return nil
}

// ModeCode 는 reg 0x02 응답 data[1] 또는 reg 0x04 WRITE data[4] 의 운전 모드 바이트이다 (REQ-CENTURY-006, REQ-CENTURY-009).
//
// 확정된 값:
//   - 0x00 : off / standby (CAP-1, CAP-2)
//   - 0x01 : cooling       (CAP-3, CAP-4 ground truth)
//
// 그 외 값은 additive enum 정책에 따라 mode_unknown_<hex> 로 노출되며 ConfirmationStatus
// 는 Unknown 으로 분류된다 (REQ-CENTURY-021, REQ-CENTURY-026).
type ModeCode byte

const (
	// ModeOff 는 꺼짐 / 대기 모드 (0x00) 이다.
	ModeOff ModeCode = 0x00
	// ModeCooling 은 냉방 모드 (0x01) 이다.
	ModeCooling ModeCode = 0x01
)

// String 은 ModeCode 의 사람이 읽을 수 있는 표현을 반환한다.
// v0.3.1: NASA / LGCNP 와 어휘 통일 — "cool" / "heat" / "dry" / "fan" / "auto" 이며
// 미확정 코드(0x02 이상) 는 "mode_unknown_<hex>" 형식으로 노출되어 downstream
// 컨슈머가 raw 값으로도 분기할 수 있게 한다 (REQ-CENTURY-021).
func (m ModeCode) String() string {
	switch m {
	case ModeOff:
		return "off"
	case ModeCooling:
		return "cool"
	default:
		return fmt.Sprintf("mode_unknown_%02x", byte(m))
	}
}

// IsConfirmed 는 ModeCode 가 SPEC §8.1 의 확정 코드 집합에 속하는지 검사한다.
// 후속 캡처로 새 코드가 확정될 때마다 이 함수가 갱신된다 (additive only).
func (m ModeCode) IsConfirmed() bool {
	return m == ModeOff || m == ModeCooling
}

// Status 는 ModeCode 의 ConfirmationStatus 를 반환한다.
// 확정된 코드는 Confirmed, 그 외는 Unknown.
func (m ModeCode) Status() ConfirmationStatus {
	if m.IsConfirmed() {
		return Confirmed
	}
	return Unknown
}

// Direction 상수는 디코딩된 이벤트가 회선상 어떤 방향이었는지 표시한다.
// 프로토콜 문서 §4 의 master/slave 주소 매핑에 따라 결정된다.
const (
	// DirectionMasterToSlave : src=0x0030, dst=0x0001 인 마스터 발신 프레임.
	DirectionMasterToSlave = "master_to_slave"
	// DirectionSlaveToMaster : src=0x0001, dst=0x0030 인 슬레이브 응답 프레임.
	DirectionSlaveToMaster = "slave_to_master"
	// DirectionUnknown : 주소 매핑이 알려진 enum 외인 경우.
	DirectionUnknown = "unknown"
)

// FieldU8 는 u8 raw value 와 ConfirmationStatus 를 함께 노출하는 typed field 이다.
// (REQ-CENTURY-020 의 value+status 형태. raw 는 value 와 동일하므로 별도 노출하지 않는다.)
type FieldU8 struct {
	Value              uint8              `json:"value"`
	ConfirmationStatus ConfirmationStatus `json:"status"`
}

// FieldU16 는 u16 raw value 와 ConfirmationStatus 를 노출하는 typed field 이다.
type FieldU16 struct {
	Value              uint16             `json:"value"`
	ConfirmationStatus ConfirmationStatus `json:"status"`
}

// FieldFloat32 는 ÷10 스케일된 float32 와 raw u16 + ConfirmationStatus 를 노출하는 typed field 이다.
// raw 는 원래의 u16 값 (예: 250) 을, value 는 ÷10 된 섭씨 (예: 25.0) 를 보유한다.
type FieldFloat32 struct {
	Value              float32            `json:"value"`
	Raw                uint16             `json:"raw"`
	ConfirmationStatus ConfirmationStatus `json:"status"`
}

// ModeField 는 ModeCode 를 typed field 형태로 노출한다.
// value 는 "cooling" / "off" / "mode_unknown_XX" 문자열, raw 는 원시 바이트, status 는 ModeCode.Status() 결과.
type ModeField struct {
	Value              string             `json:"value"`
	Raw                uint8              `json:"raw"`
	ConfirmationStatus ConfirmationStatus `json:"status"`
}

// NewModeField 는 raw byte 로부터 ModeField 를 구성한다 (REQ-CENTURY-006, REQ-CENTURY-021).
func NewModeField(raw byte) ModeField {
	m := ModeCode(raw)
	return ModeField{
		Value:              m.String(),
		Raw:                raw,
		ConfirmationStatus: m.Status(),
	}
}

// Reg02Decoded 는 reg 0x02 응답 (현재 설정 readback, 17B data) 의 디코딩 결과이다 (REQ-CENTURY-006).
//
// 4.4 예시 1 의 페이로드 스키마와 1:1 매핑된다.
//
// v0.4.0: type 필드 추가 — downstream 분기/필터 용. transformDecodedPayload 가 nested
// 필드만 state 그룹으로 이동시키므로 본 top-level 문자열 필드는 그대로 유지된다.
type Reg02Decoded struct {
	Type        string `json:"type"` // "century_reg02_response" (v0.4.0)
	SubDevID    uint8  `json:"dev_id"`
	Register    uint8  `json:"register"`
	TimestampMs int64  `json:"timestamp_ms"`
	Direction   string `json:"direction"`

	// data[1] mode (Confirmed: 0x00=off, 0x01=cooling; 그 외는 additive Unknown)
	Mode ModeField `json:"mode"`
	// data[2] fan 세기 (Confirmed, CAP-3 17 관측)
	Fan FieldU8 `json:"fan"`
	// data[7..8] 설정 온도 LE u16 ÷10 (Confirmed)
	SetpointC FieldFloat32 `json:"setpoint_c"`
	// data[11..12] 두 번째 25.0℃ 슬롯 (Inferred — setpoint 복제 또는 모드별 슬롯)
	Reg02Word11 FieldFloat32 `json:"reg02_word_11"`
	// data[13] 운전 중 채워지는 live byte (Inferred)
	Reg02Live13 FieldU8 `json:"reg02_live_13"`
	// data[14] 0x39↔0x38 미세 변동 (Inferred)
	Reg02Live14 FieldU8 `json:"reg02_live_14"`
	// data[15] 운전 중 채워지는 live byte (Inferred)
	Reg02Live15 FieldU8 `json:"reg02_live_15"`
	// data[0,3,4,5,6,9,10,16] 4 캡처 모두 0x00 (Unknown / zero-padding)
	Reg02Byte0  FieldU8 `json:"reg02_byte_0"`
	Reg02Byte3  FieldU8 `json:"reg02_byte_3"`
	Reg02Byte4  FieldU8 `json:"reg02_byte_4"`
	Reg02Byte5  FieldU8 `json:"reg02_byte_5"`
	Reg02Byte6  FieldU8 `json:"reg02_byte_6"`
	Reg02Byte9  FieldU8 `json:"reg02_byte_9"`
	Reg02Byte10 FieldU8 `json:"reg02_byte_10"`
	Reg02Byte16 FieldU8 `json:"reg02_byte_16"`
}

// Reg03Decoded 는 reg 0x03 응답 (증발기 냉매 배관 온도, 16B data) 의 디코딩 결과이다 (REQ-CENTURY-007).
type Reg03Decoded struct {
	Type        string `json:"type"` // "century_reg03_response" (v0.4.0)
	SubDevID    uint8  `json:"dev_id"`
	Register    uint8  `json:"register"`
	TimestampMs int64  `json:"timestamp_ms"`
	Direction   string `json:"direction"`

	// data[0..1] 증발기 word0 LE u16 ÷10 (Confirmed)
	TempEvapAC FieldFloat32 `json:"temp_evap_a_c"`
	// data[2..3] 증발기 word1 LE u16 ÷10 (Confirmed)
	TempEvapBC FieldFloat32 `json:"temp_evap_b_c"`
	// data[4..15] 12 바이트 zero-padding (Unknown / Confirmed-as-zero)
	Reg03Pad4  FieldU8 `json:"reg03_pad_4"`
	Reg03Pad5  FieldU8 `json:"reg03_pad_5"`
	Reg03Pad6  FieldU8 `json:"reg03_pad_6"`
	Reg03Pad7  FieldU8 `json:"reg03_pad_7"`
	Reg03Pad8  FieldU8 `json:"reg03_pad_8"`
	Reg03Pad9  FieldU8 `json:"reg03_pad_9"`
	Reg03Pad10 FieldU8 `json:"reg03_pad_10"`
	Reg03Pad11 FieldU8 `json:"reg03_pad_11"`
	Reg03Pad12 FieldU8 `json:"reg03_pad_12"`
	Reg03Pad13 FieldU8 `json:"reg03_pad_13"`
	Reg03Pad14 FieldU8 `json:"reg03_pad_14"`
	Reg03Pad15 FieldU8 `json:"reg03_pad_15"`
}

// Reg04ReadDecoded 는 reg 0x04 응답 (운전 상태 + 운전 데이터, 14B data) 의 디코딩 결과이다 (REQ-CENTURY-008).
type Reg04ReadDecoded struct {
	Type        string `json:"type"` // "century_reg04_response" (v0.4.0)
	SubDevID    uint8  `json:"dev_id"`
	Register    uint8  `json:"register"`
	TimestampMs int64  `json:"timestamp_ms"`
	Direction   string `json:"direction"`

	// data[0] status bitmap (Inferred — 캡처마다 변동)
	StatusBits FieldU8 `json:"status_bits"`
	// data[1] 4 캡처 모두 0xF6 (Inferred — 상수성 확인됨)
	Reg04Const1 FieldU8 `json:"reg04_const_1"`
	// data[2] 4 캡처 모두 0x09 (Inferred)
	Reg04Const2 FieldU8 `json:"reg04_const_2"`
	// data[7] 4 캡처 모두 0x2C (Inferred)
	Reg04Const7 FieldU8 `json:"reg04_const_7"`
	// data[8..9] op_val_1 LE u16 (Inferred, CAP-4 996)
	OpVal1 FieldU16 `json:"op_val_1"`
	// data[10..11] temp_A LE u16 ÷10 (Inferred, CAP-3/4 25.2℃)
	TempAC FieldFloat32 `json:"temp_A_c"`
	// data[12..13] op_val_2 LE u16 (Inferred, CAP-4 1248)
	OpVal2 FieldU16 `json:"op_val_2"`
	// data[3..6] 4 캡처 모두 0x00 (Unknown)
	Reg04Byte3 FieldU8 `json:"reg04_byte_3"`
	Reg04Byte4 FieldU8 `json:"reg04_byte_4"`
	Reg04Byte5 FieldU8 `json:"reg04_byte_5"`
	Reg04Byte6 FieldU8 `json:"reg04_byte_6"`
}

// Reg04WriteDecoded 는 reg 0x04 WRITE 요청 (마스터 → 슬레이브, 16B data) 의 디코딩 결과이다 (REQ-CENTURY-009).
//
// 본 에이전트는 패시브 캡처 전용이므로 이 구조체는 회선상 관측된 마스터의 명령을 의미하며,
// 본 에이전트가 송신한 프레임이 아니다.
type Reg04WriteDecoded struct {
	Type        string `json:"type"` // "century_reg04_write_request" (v0.4.0)
	SubDevID    uint8  `json:"dev_id"`
	Register    uint8  `json:"register"`
	TimestampMs int64  `json:"timestamp_ms"`
	Direction   string `json:"direction"`
	// ObservationMode 는 본 디코딩이 passive observation 임을 명시한다 (REQ-CENTURY-009).
	ObservationMode string `json:"observation_mode"`

	// data[0] 운전 중 set, CAP-4: 0x02 (Inferred)
	WriteLive0 FieldU8 `json:"write_live_0"`
	// data[1] 운전 중 set, CAP-4: 0x04 (Inferred)
	WriteLive1 FieldU8 `json:"write_live_1"`
	// data[4] 마스터의 모드 명령 (Confirmed: 0x00=off, 0x01=cooling)
	ModeCmd ModeField `json:"mode_cmd"`
	// data[14] 캡처별 0xC4/0xC7/0xC0 (Inferred — 마스터 측 설정/펌웨어 추정)
	WriteByte14 FieldU8 `json:"write_byte_14"`
	// data[15] CAP-4 0x0F~0x11 변동 (Inferred — 마스터 측 라이브 센서값 추정)
	WriteLive15 FieldU8 `json:"write_live_15"`
	// data[2,3,5..13] reserved (Unknown)
	WriteByte2  FieldU8 `json:"write_byte_2"`
	WriteByte3  FieldU8 `json:"write_byte_3"`
	WriteByte5  FieldU8 `json:"write_byte_5"`
	WriteByte6  FieldU8 `json:"write_byte_6"`
	WriteByte7  FieldU8 `json:"write_byte_7"`
	WriteByte8  FieldU8 `json:"write_byte_8"`
	WriteByte9  FieldU8 `json:"write_byte_9"`
	WriteByte10 FieldU8 `json:"write_byte_10"`
	WriteByte11 FieldU8 `json:"write_byte_11"`
	WriteByte12 FieldU8 `json:"write_byte_12"`
	WriteByte13 FieldU8 `json:"write_byte_13"`
}

// ACKDecoded 는 ACK 프레임 (function_code=0x06, payload=[0x00], 1B) 의 디코딩 결과이다 (REQ-CENTURY-010).
//
// ACK 는 페이로드 prefix (sub_dev_id / register) 가 없으므로 SubDevID 와 Register 는 0 으로 둔다.
type ACKDecoded struct {
	Type        string `json:"type"` // "century_ack" (v0.4.0)
	TimestampMs int64  `json:"timestamp_ms"`
	Direction   string `json:"direction"`
}

// EventTypeDeviceState 는 CenturyDeviceStateEvent 의 type 필드 값이다 (REQ-CENTURY-033).
//
// downstream flow node 가 register-decoded 메시지와 device_state 메시지를 분기하기 위한 식별자.
const EventTypeDeviceState = "device_state"

// v0.4.0 register-decoded type 식별자. SPEC §4.4 예시와 일치.
//
// downstream 의 filter / routing 분기에 사용한다. transformDecodedPayload 의 state-grouped
// 출력에서도 top-level "type" 필드로 유지된다.
const (
	EventTypeReg02Response     = "century_reg02_response"
	EventTypeReg03Response     = "century_reg03_response"
	EventTypeReg04Response     = "century_reg04_response"
	EventTypeReg04WriteRequest = "century_reg04_write_request"
	EventTypeACK               = "century_ack"
)

// DeviceStateTrigger 는 device_state emit 의 트리거 종류이다 (REQ-CENTURY-035).
const (
	// TriggerChange 는 5 핵심 필드 또는 online 상태 변경으로 인한 emit 이다.
	TriggerChange = "change"
	// TriggerReport 는 변경 없이 report_interval 경과 후 발생하는 주기적 상태보고이다 (v0.6.0).
	// 이전 명칭 TriggerKeepalive 는 폐기 — "keepalive" 는 향후 세션 연결 관리에 예약.
	TriggerReport = "report"
)

// CenturyDeviceStateInner 는 device_state 이벤트의 nested state 그룹 페이로드이다 (v0.4.0).
//
// 5 핵심 필드 + online 을 묶어 LGCNP / register-decoded 와 동일한 state-grouped 패턴을
// 따른다. 외부 wrapper 인 CenturyDeviceStateEvent 가 top-level metadata (dev_id, label,
// timestamp_ms, last_seen_ms, trigger, type) 를 노출한다.
//
// v0.5.1: Reg03 증발기 온도(temp_evap_a_c / temp_evap_b_c) 를 state 에 통합 (이전엔
// register-decoded 메시지에서만 노출). Reg03 미수신 시 omitempty 로 자동 제외하도록
// pointer 사용.
type CenturyDeviceStateInner struct {
	Online      bool    `json:"online"`
	Power       bool    `json:"power"`
	Mode        string  `json:"mode"`         // "off" / "cool" / "mode_unknown_<hex>" — NASA/LGCNP 통일
	FanSpeed    uint8   `json:"fan_speed"`    // NASA/LGCNP 통일 (이전 "fan")
	TargetTemp  float32 `json:"target_temp"`  // °C — NASA/LGCNP 통일 (이전 "set_temp_c")
	CurrentTemp float32 `json:"current_temp"` // °C — NASA/LGCNP 통일 (이전 "current_temp_c")

	// v0.5.1: Reg03 증발기 온도. 미수신 시 nil → omitempty 로 출력 제외.
	TempEvapAC *float32 `json:"temp_evap_a_c,omitempty"`
	TempEvapBC *float32 `json:"temp_evap_b_c,omitempty"`
}

// CenturyDeviceStateMetadata 는 device_state 이벤트의 metadata 그룹이다 (v0.5.0).
//
// label 등 식별/표시용 메타데이터를 묶는다. omitempty 로 미설정 필드는 자동 제외.
// 사용자 요구 "metadata => slot_num, label" — Century 는 slot_num 미지원이므로
// label 만 노출. 추후 슬롯 개념이 도입되면 SlotNum 필드 추가.
type CenturyDeviceStateMetadata struct {
	Label      string `json:"label,omitempty"`
	DeviceType string `json:"device_type,omitempty"` // v0.6.4: 디바이스 타입 (Century 는 항상 "indoor")
}

// CenturyDeviceStateEvent 는 v0.3.0 기본 emit 인 device-centric 통합 상태 이벤트이다 (REQ-CENTURY-033).
//
// 단일 메시지에 register 0x02 (mode/fan/setpoint) + register 0x04 read (현재 온도)
// 의 종합을 노출한다. v0.4.2 부터는 Reg02 + Reg04 모두 수신 후 emit 한다 (A14 갱신).
//
// 변경 감지 (5 핵심 필드 + online 전이) 시 trigger="change" 로 emit 되며,
// keepalive_interval 경과 시 trigger="keepalive" 로 fallback emit 된다 (REQ-CENTURY-035).
//
// v0.4.0: state nested group, 5 핵심 + online 을 state 안으로.
// v0.5.0 Breaking: 통합 출력 schema 적용 (사용자 요구) —
//   - timestamp_ms 제거 (last_seen_ms 단일화)
//   - label 을 top-level 에서 metadata.label 로 이동
//   - raw_hex 옵션 추가 (include_raw_hex=true 시에만 노출)
//
// JSON snake_case + epoch ms timestamp 컨벤션을 따른다 (A9).
type CenturyDeviceStateEvent struct {
	Type       string                     `json:"type"`
	SubDevID   string                     `json:"dev_id"`
	Trigger    string                     `json:"trigger"`
	LastSeenMs int64                      `json:"last_seen_ms"`
	RawHex     string                     `json:"raw_hex,omitempty"` // v0.5.0: include_raw_hex=true 시에만 노출
	State      CenturyDeviceStateInner    `json:"state"`
	Metadata   CenturyDeviceStateMetadata `json:"metadata,omitempty"`
}

// CenturyDeviceStateSnapshot 은 변경 감지용 5 핵심 + online + ModeRaw 스냅샷이다 (REQ-CENTURY-035).
//
// Equals 는 5 핵심 필드 + 증발기 온도 (v0.5.1) 를 비교한다.
// Online 전이는 별도 필드로 관리되어 captureLoop 와 offlineWatchLoop 에서 직접 비교한다.
//
// v0.5.1: 증발기 온도(Reg03 의 temp_evap_a_c / temp_evap_b_c) 를 다시 snapshot 에
// 포함 — state 그룹의 일부로 출력되므로 change detection 도 함께 수행. omitempty 를
// 위해 pointer 사용 (nil = Reg03 미수신).
type CenturyDeviceStateSnapshot struct {
	// Power 는 mode != ModeOff 여부이다.
	Power bool
	// Mode 는 ModeCode.String() 결과 ("off" / "cool" / "mode_unknown_<hex>") 이다. NASA/LGCNP 통일.
	Mode string
	// ModeRaw 는 raw 바이트 (Reg02 미수신 시 0) — change 비교 시 보조 정확도 확보용.
	ModeRaw byte
	// FanSpeed 는 reg 0x02 data[2] 의 raw uint8. NASA/LGCNP 통일 (이전 "Fan").
	FanSpeed uint8
	// TargetTemp 는 reg 0x02 setpoint (LE u16 ÷ 10.0, 미수신 시 0.0). NASA/LGCNP 통일 (이전 "SetTempC").
	TargetTemp float32
	// CurrentTemp 는 reg 0x04 read response 의 temp_A_c (미수신 시 0.0). NASA/LGCNP 통일 (이전 "CurrentTempC").
	CurrentTemp float32
	// Online 은 디바이스의 현재 online 상태.
	Online bool
	// TempEvapAC / TempEvapBC 는 Reg03 의 증발기 온도 (v0.5.1). nil = Reg03 미수신.
	TempEvapAC *float32
	TempEvapBC *float32
}

// Equals 는 두 snapshot 의 비교 대상 필드를 검사한다.
//
// 5 핵심 + 증발기 온도 중 하나라도 다르면 false. online 은 비교하지 않는다 (별도 비교).
// pointer 값 비교는 둘 다 nil 이거나 둘 다 non-nil 이면서 같은 값일 때만 같음.
func (s CenturyDeviceStateSnapshot) Equals(other CenturyDeviceStateSnapshot) bool {
	if s.Power != other.Power ||
		s.ModeRaw != other.ModeRaw ||
		s.FanSpeed != other.FanSpeed ||
		s.TargetTemp != other.TargetTemp ||
		s.CurrentTemp != other.CurrentTemp {
		return false
	}
	if !floatPtrEqual(s.TempEvapAC, other.TempEvapAC) {
		return false
	}
	if !floatPtrEqual(s.TempEvapBC, other.TempEvapBC) {
		return false
	}
	return true
}

// EqualsExceptCurrentTemp 는 CurrentTemp 를 제외한 모든 비교 대상 필드가 같은지 검사한다 (v0.6.6).
// event_temp_threshold gate 에서 "실내온도만 변경" 케이스를 판별할 때 사용한다.
func (s CenturyDeviceStateSnapshot) EqualsExceptCurrentTemp(other CenturyDeviceStateSnapshot) bool {
	if s.Power != other.Power ||
		s.ModeRaw != other.ModeRaw ||
		s.FanSpeed != other.FanSpeed ||
		s.TargetTemp != other.TargetTemp {
		return false
	}
	if !floatPtrEqual(s.TempEvapAC, other.TempEvapAC) {
		return false
	}
	if !floatPtrEqual(s.TempEvapBC, other.TempEvapBC) {
		return false
	}
	return true
}

// floatPtrEqual 는 두 *float32 의 같음 여부를 검사한다 (nil-aware).
func floatPtrEqual(a, b *float32) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// BuildDeviceStateSnapshot 은 CenturyDeviceState 로부터 변경 감지용 snapshot 을 빌드한다 (REQ-CENTURY-033).
//
// 미수신 register 는 0.0 / 0 / false 로 채워진다 (A14). Mode/ModeRaw 는 Reg02 미수신 시 "off" / 0x00.
// v0.5.1: Reg03 미수신 시 TempEvapAC / TempEvapBC 는 nil → state 출력에서 omitempty 자동 제외.
// 호출자는 state 가 dereference 가능한지 (nil 아님) 확인해야 한다.
func BuildDeviceStateSnapshot(state *CenturyDeviceState, online bool) CenturyDeviceStateSnapshot {
	s := CenturyDeviceStateSnapshot{Online: online}
	if state == nil {
		// Reg02 nil → mode=off, power=false
		s.Mode = ModeOff.String()
		return s
	}
	if state.Reg02 != nil {
		s.ModeRaw = state.Reg02.Mode.Raw
		s.Mode = ModeCode(s.ModeRaw).String()
		s.Power = s.ModeRaw != byte(ModeOff)
		s.FanSpeed = state.Reg02.Fan.Value
		s.TargetTemp = state.Reg02.SetpointC.Value
	} else {
		s.Mode = ModeOff.String()
	}
	if state.Reg04Read != nil {
		s.CurrentTemp = state.Reg04Read.TempAC.Value
	}
	if state.Reg03 != nil {
		evapA := state.Reg03.TempEvapAC.Value
		evapB := state.Reg03.TempEvapBC.Value
		s.TempEvapAC = &evapA
		s.TempEvapBC = &evapB
	}
	return s
}

// NewDeviceStateEvent 는 snapshot + 디바이스 메타 + 시각 + 트리거로부터 emit 용 이벤트를 빌드한다.
//
// sub_dev_id 는 "0x3B" 형식의 uppercase 2-digit hex 문자열로 직렬화된다 (REQ-CENTURY-033).
//
// v0.5.0 통합 schema:
//   - nowMs 인자는 last_seen_ms 로 직접 매핑 (이전: 별도 timestamp_ms 필드 존재).
//   - label 은 metadata.label 로 이동.
//   - rawHex 가 비어있지 않으면 raw_hex 필드로 노출 (include_raw_hex 옵션).
func NewDeviceStateEvent(
	snap CenturyDeviceStateSnapshot,
	subDevID byte,
	label string,
	lastSeenMs int64,
	trigger string,
	rawHex string,
) *CenturyDeviceStateEvent {
	return &CenturyDeviceStateEvent{
		Type:       EventTypeDeviceState,
		SubDevID:   fmt.Sprintf("0x%02X", subDevID),
		Trigger:    trigger,
		LastSeenMs: lastSeenMs,
		RawHex:     rawHex,
		State: CenturyDeviceStateInner{
			Online:      snap.Online,
			Power:       snap.Power,
			Mode:        snap.Mode,
			FanSpeed:    snap.FanSpeed,
			TargetTemp:  snap.TargetTemp,
			CurrentTemp: snap.CurrentTemp,
			TempEvapAC:  snap.TempEvapAC,
			TempEvapBC:  snap.TempEvapBC,
		},
		Metadata: CenturyDeviceStateMetadata{
			Label:      label,
			DeviceType: "indoor", // v0.6.4: Century 는 IDU 만 처리 (Reg02/03/04 모두 indoor unit).
		},
	}
}
