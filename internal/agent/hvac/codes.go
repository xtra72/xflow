// Package hvac 는 모든 HVAC 에이전트 (Century / NASA / LGCNP / LGCP / LGAP)
// 가 노드로 출력하는 mode / fan_speed 의 통일 ID 코드를 정의한다 (v0.7.5).
//
// 각 에어컨 프로토콜의 raw 값과는 별개로, 에이전트 → 노드 출력 시점에
// 본 패키지의 통일 ID 로 변환된다.
package hvac

// Mode 통일 ID (v0.7.5).
//
//	0: off / auto (전원 꺼짐 또는 자동 모드)
//	1: cool
//	2: heat
//	3: dry
//	4: fan
const (
	ModeOffOrAuto = 0
	ModeCool      = 1
	ModeHeat      = 2
	ModeDry       = 3
	ModeFan       = 4
)

// FanSpeed 통일 ID (v0.7.5).
//
//	0: off
//	1: auto
//	2: quiet
//	3: low
//	4: medium
//	5: high
//	6: turbo
const (
	FanOff    = 0
	FanAuto   = 1
	FanQuiet  = 2
	FanLow    = 3
	FanMedium = 4
	FanHigh   = 5
	FanTurbo  = 6
)

// ModeFromName 은 표준 영문 모드명 ("cool"/"heat"/"dry"/"fan"/"auto"/"off")
// 을 통일 ID 로 변환한다. 알 수 없는 값은 0 (off/auto) 반환.
func ModeFromName(name string) int {
	switch name {
	case "cool", "cooling":
		return ModeCool
	case "heat", "heating":
		return ModeHeat
	case "dry", "dehumidify":
		return ModeDry
	case "fan":
		return ModeFan
	case "off", "auto", "":
		return ModeOffOrAuto
	default:
		return ModeOffOrAuto
	}
}

// FanSpeedFromName 은 표준 영문 풍량명 ("auto"/"quiet"/"low"/"medium"/"high"/"turbo"/"off")
// 을 통일 ID 로 변환한다. 알 수 없는 값은 0 (off) 반환.
func FanSpeedFromName(name string) int {
	switch name {
	case "auto":
		return FanAuto
	case "quiet":
		return FanQuiet
	case "low", "slow":
		return FanLow
	case "medium":
		return FanMedium
	case "high":
		return FanHigh
	case "turbo":
		return FanTurbo
	case "off", "":
		return FanOff
	default:
		return FanOff
	}
}
