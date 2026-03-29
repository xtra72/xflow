package lg

// ---------------------------------------------------------------------------
// 패킷 상수
// ---------------------------------------------------------------------------

const (
	// RequestSize 는 LGAP 요청 패킷의 고정 크기이다 (8바이트).
	RequestSize = 8

	// ResponseSize 는 LGAP 응답 패킷의 고정 크기이다 (16바이트).
	ResponseSize = 16

	// HeaderByte 는 LGAP 프로토콜의 헤더 바이트이다 (TX0/RX0).
	HeaderByte byte = 0x10

	// CommandByte 는 LGAP 프로토콜의 명령 바이트이다 (TX1).
	CommandByte byte = 0x00

	// CommandID 는 LGAP 프로토콜의 명령 ID 바이트이다 (TX2).
	CommandID byte = 0xA0
)

// ---------------------------------------------------------------------------
// 운전 모드 상수 (TX5/RX6 bits[7:5])
// ---------------------------------------------------------------------------

const (
	ModeCool byte = 0 // 냉방
	ModeDry  byte = 1 // 제습
	ModeFan  byte = 2 // 송풍
	ModeAuto byte = 3 // 자동
	ModeHeat byte = 4 // 난방
)

// ---------------------------------------------------------------------------
// 팬 속도 상수 (TX5/RX6 bits[4:2])
// ---------------------------------------------------------------------------

const (
	FanNoChange byte = 0 // 변경 없음
	FanLow      byte = 1 // 약풍
	FanMedium   byte = 2 // 중풍
	FanHigh     byte = 3 // 강풍
	FanAuto     byte = 4 // 자동
	FanSlow     byte = 5 // 미풍
	FanTurbo    byte = 6 // 터보
)

// ---------------------------------------------------------------------------
// 제어 플래그 비트 (TX4)
// ---------------------------------------------------------------------------

const (
	FlagPower   byte = 0x01 // bit 0: 전원 ON(1)/OFF(0)
	FlagExecute byte = 0x02 // bit 1: 쓰기 모드 EXE(1)/읽기(0)
	FlagLock    byte = 0x04 // bit 2: 잠금(1)/해제(0)
	FlagPlasma  byte = 0x10 // bit 4: 플라즈마 ON(1)/OFF(0)
)

// ---------------------------------------------------------------------------
// 모드 매핑
// ---------------------------------------------------------------------------

// StringToMode 는 모드 문자열을 LGAP 바이트로 변환하는 맵이다.
var StringToMode = map[string]byte{
	"cool": ModeCool,
	"dry":  ModeDry,
	"fan":  ModeFan,
	"auto": ModeAuto,
	"heat": ModeHeat,
}

// ModeToString 은 LGAP 모드 바이트를 문자열로 변환하는 맵이다.
var ModeToString = map[byte]string{
	ModeCool: "cool",
	ModeDry:  "dry",
	ModeFan:  "fan",
	ModeAuto: "auto",
	ModeHeat: "heat",
}

// ---------------------------------------------------------------------------
// 팬 속도 매핑
// ---------------------------------------------------------------------------

// StringToFanSpeed 는 팬 속도 문자열을 LGAP 바이트로 변환하는 맵이다.
var StringToFanSpeed = map[string]byte{
	"low":    FanLow,
	"medium": FanMedium,
	"high":   FanHigh,
	"auto":   FanAuto,
	"slow":   FanSlow,
	"turbo":  FanTurbo,
}

// FanSpeedToString 은 LGAP 팬 속도 바이트를 문자열로 변환하는 맵이다.
var FanSpeedToString = map[byte]string{
	FanNoChange: "no_change",
	FanLow:      "low",
	FanMedium:   "medium",
	FanHigh:     "high",
	FanAuto:     "auto",
	FanSlow:     "slow",
	FanTurbo:    "turbo",
}

// ---------------------------------------------------------------------------
// LGAPRequest 는 8바이트 LGAP 요청 패킷을 나타낸다.
// ---------------------------------------------------------------------------

// LGAPRequest 는 LGAP 요청 패킷의 구조체이다.
type LGAPRequest struct {
	Zone        byte // TX3: 존 주소 (상위 니블=그룹, 하위 니블=실내기)
	Flags       byte // TX4: 제어 플래그
	ModeCombo   byte // TX5: 모드/팬/스윙 조합 바이트
	Temperature byte // TX6: 설정 온도 (섭씨 - 15)
}

// ---------------------------------------------------------------------------
// LGAPResponse 는 16바이트 LGAP 응답 패킷을 나타낸다.
// ---------------------------------------------------------------------------

// LGAPResponse 는 LGAP 응답 패킷의 구조체이다.
type LGAPResponse struct {
	Status      byte     // RX1: 상태 바이트 (0=정상)
	FlagsEcho   byte     // RX2: 요청 플래그 에코
	Zone        byte     // RX4: 존 에코
	Error       byte     // RX5: 에러 코드 (0=에러 없음)
	ModeCombo   byte     // RX6: 모드/팬/스윙 에코
	TargetTemp  byte     // RX7: 설정 온도 (하위 니블 + 15 = 섭씨)
	RoomTemp    byte     // RX8: 실내 온도 원시값
	PipeInTemp  byte     // RX9: 파이프 입구 온도 원시값
	PipeOutTemp byte     // RX10: 파이프 출구 온도 원시값
	ZoneLoad    byte     // RX11: 존 부하 (204=유휴)
	ZonePower   byte     // RX12: 존 전원 플래그 (0=운전중, 1=정지)
	DesignLoad  byte     // RX13: 설계 부하
	ODULoad     byte     // RX14: 실외기 총 부하
	Raw         [16]byte // 전체 원시 패킷
}

// ---------------------------------------------------------------------------
// 온도 변환 헬퍼
// ---------------------------------------------------------------------------

// EncodeTargetTemp 는 섭씨 온도를 LGAP 설정 온도 바이트로 변환한다.
// 범위: 16~30도 -> 1~15.
func EncodeTargetTemp(celsius int) byte {
	return byte(celsius - 15)
}

// DecodeTargetTemp 는 LGAP 설정 온도 바이트를 섭씨 온도로 변환한다.
// RX7 의 하위 니블 + 15 = 섭씨.
func DecodeTargetTemp(raw byte) int {
	return int(raw&0x0F) + 15
}

// DecodeMeasuredTemp 는 LGAP 측정 온도 바이트를 섭씨 온도(float32)로 변환한다.
// 계산식: (192 - raw) / 3.0
func DecodeMeasuredTemp(raw byte) float32 {
	return float32(192-int(raw)) / 3.0
}

// ---------------------------------------------------------------------------
// 모드/팬/스윙 조합 바이트 인코딩/디코딩 (TX5/RX6)
// ---------------------------------------------------------------------------

// EncodeModeCombo 는 모드, 팬 속도, 스윙 자동 여부를 조합 바이트로 인코딩한다.
//
//	bits[7:5] = 모드 (0~4)
//	bits[4:2] = 팬 속도 (0~6)
//	bit1      = 스윙 자동(1)/수동(0)
//	bit0      = 예약
func EncodeModeCombo(mode, fan byte, swingAuto bool) byte {
	var combo byte
	combo |= (mode & 0x07) << 5  // bits[7:5]
	combo |= (fan & 0x07) << 2   // bits[4:2]
	if swingAuto {
		combo |= 0x02            // bit1
	}
	return combo
}

// DecodeModeCombo 는 조합 바이트에서 모드, 팬 속도, 스윙 자동 여부를 추출한다.
func DecodeModeCombo(combo byte) (mode byte, fan byte, swingAuto bool) {
	mode = (combo >> 5) & 0x07
	fan = (combo >> 2) & 0x07
	swingAuto = combo&0x02 != 0
	return
}

// ---------------------------------------------------------------------------
// 존 주소 헬퍼
// ---------------------------------------------------------------------------

// ZoneToGroupUnit 은 존 바이트를 그룹 번호와 유닛 번호로 변환한다.
// 존 바이트: 상위 니블 = 그룹, 하위 니블 = 유닛.
func ZoneToGroupUnit(zone byte) (group, unit int) {
	return int(zone >> 4), int(zone & 0x0F)
}

// GroupUnitToZone 은 그룹 번호와 유닛 번호를 존 바이트로 변환한다.
func GroupUnitToZone(group, unit int) byte {
	return byte((group&0x0F)<<4 | (unit & 0x0F))
}
