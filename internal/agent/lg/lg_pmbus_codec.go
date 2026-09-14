package lg

import (
	"fmt"

	"github.com/xtra/xflow/internal/agent/hvac"
)

// ---------------------------------------------------------------------------
// PMBUSB00A 값 인코딩 / 디코딩
//
// SPEC-LG-HVACR-003 § M1. 트랜스포트에 의존하지 않는 순수 함수만 둔다.
//
// 미검증 가정의 주입: temp_scale / fan_auto_code 는 상수가 아니라 인자로 받는다.
// 현장 실측이 문서와 다를 때 재빌드 없이 설정으로 교정하기 위함이다.
// ---------------------------------------------------------------------------

// pmbusDecodeTemp 는 레지스터 워드를 °C 로 변환한다.
//
// 온도는 음수를 표현하므로 반드시 signed int16 으로 해석해야 한다.
// 예: 0xFFC4 = −60 = −6.0 °C (scale=10). uint16 으로 읽으면 6553.2 가 되어버린다.
func pmbusDecodeTemp(raw uint16, scale int) float64 {
	if scale <= 0 {
		scale = 1
	}
	return float64(int16(raw)) / float64(scale)
}

// pmbusEncodeTemp 는 °C 를 레지스터 워드로 변환한다.
// 음수 온도도 2의 보수로 올바르게 인코딩된다.
func pmbusEncodeTemp(tempC float64, scale int) uint16 {
	if scale <= 0 {
		scale = 1
	}
	// 부동소수 오차로 240 이 239 로 내려앉지 않도록 반올림한다.
	v := tempC * float64(scale)
	if v >= 0 {
		return uint16(int16(v + 0.5))
	}
	return uint16(int16(v - 0.5))
}

// pmbusModeToUnifiedID 는 Holding ① 운전 모드 코드를 hvac 통일 ID 로 변환한다.
// 알 수 없는 코드는 통일 ID 0 (off/auto) 으로 폴백한다.
func pmbusModeToUnifiedID(code uint16) int {
	switch code {
	case pmbusModeCool:
		return hvac.ModeCool
	case pmbusModeDry:
		return hvac.ModeDry
	case pmbusModeFan:
		return hvac.ModeFan
	case pmbusModeAuto:
		return hvac.ModeOffOrAuto
	case pmbusModeHeat:
		return hvac.ModeHeat
	default:
		return hvac.ModeOffOrAuto
	}
}

// pmbusModeFromName 은 사용자 입력 모드명을 Holding ① 프로토콜 코드로 변환한다.
// lg_hvacr02 가 받아들이는 별칭(cool/cooling, dry/dehumidify, heat/heating)을 동일하게 지원한다.
func pmbusModeFromName(name string) (uint16, error) {
	switch name {
	case "cool", "cooling":
		return pmbusModeCool, nil
	case "dry", "dehumidify":
		return pmbusModeDry, nil
	case "fan":
		return pmbusModeFan, nil
	case "auto":
		return pmbusModeAuto, nil
	case "heat", "heating":
		return pmbusModeHeat, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrHvacr03InvalidMode, name)
	}
}

// pmbusFanToUnifiedID 는 Holding ② 풍량 코드를 hvac 통일 ID 로 변환한다.
//
// autoCode 는 "자동"에 해당하는 프로토콜 코드이다. 문서는 4 를 자동으로 정의하지만
// LGCP 는 4 를 초강(turbo), 5 를 자동으로 쓴다. 실측이 문서와 다르면 설정으로
// autoCode 를 5 로 바꾸며, 그 경우 4 는 turbo 로 해석된다.
func pmbusFanToUnifiedID(code uint16, autoCode int) int {
	if autoCode > 0 && int(code) == autoCode {
		return hvac.FanAuto
	}
	switch code {
	case pmbusFanLow:
		return hvac.FanLow
	case pmbusFanMedium:
		return hvac.FanMedium
	case pmbusFanHigh:
		return hvac.FanHigh
	case 4:
		// autoCode 가 4 가 아닌 경우에만 여기 도달한다 (autoCode=5 로 교정된 상황).
		return hvac.FanTurbo
	default:
		return hvac.FanOff
	}
}

// pmbusFanFromName 은 사용자 입력 풍량명을 Holding ② 프로토콜 코드로 변환한다.
// autoCode 는 pmbusFanToUnifiedID 와 동일한 의미이다.
func pmbusFanFromName(name string, autoCode int) (uint16, error) {
	switch name {
	case "low", "slow":
		return pmbusFanLow, nil
	case "medium", "mid":
		return pmbusFanMedium, nil
	case "high":
		return pmbusFanHigh, nil
	case "auto":
		if autoCode <= 0 {
			return 0, fmt.Errorf("%w: auto code not configured", ErrHvacr03InvalidFanSpeed)
		}
		return uint16(autoCode), nil
	case "turbo":
		// 문서가 정의하는 4값 체계(1~3 + 자동)에는 turbo 가 없다. autoCode 가 5 로
		// 교정된 경우에만 4 가 turbo 로 성립한다.
		if autoCode == 4 {
			return 0, fmt.Errorf("%w: turbo is not available when fan_auto_code is 4", ErrHvacr03InvalidFanSpeed)
		}
		return 4, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrHvacr03InvalidFanSpeed, name)
	}
}

// pmbusERVModeFromName 은 환기 운전 모드명을 Holding ⑥ 코드로 변환한다.
func pmbusERVModeFromName(name string) (uint16, error) {
	switch name {
	case "heat_exchange", "heat-exchange":
		return pmbusERVModeHeatExchange, nil
	case "auto":
		return pmbusERVModeAuto, nil
	case "normal":
		return pmbusERVModeNormal, nil
	default:
		return 0, fmt.Errorf("%w: erv_mode %q", ErrHvacr03InvalidMode, name)
	}
}

// pmbusLockCoilItem 은 잠금 대상 이름을 Coil 항목 번호로 변환한다.
func pmbusLockCoilItem(target string) (uint16, error) {
	switch target {
	case "remote":
		return pmbusCoilLockRemote, nil
	case "mode":
		return pmbusCoilLockMode, nil
	case "fan":
		return pmbusCoilLockFan, nil
	case "temp", "temperature":
		return pmbusCoilLockTemp, nil
	case "address":
		return pmbusCoilLockAddress, nil
	default:
		return 0, fmt.Errorf("%w: lock target %q", ErrHvacr03MissingParam, target)
	}
}

// pmbusParseUnitAddr 는 사용자 지정 실내기 주소 문자열을 내부 N (0-base) 으로 변환한다.
//
// base 는 설정의 address_base 이다. 사용자가 1-base 로 주소를 다루는 현장에서
// address_base: 1 로 두면 "1" 이 내부 N=0 으로 매핑된다.
//
// LGCP 의 8자리 hex 물리 주소(예: "44550065")는 여기서 받아들이지 않는다 —
// Modbus 의 N 은 실외기에 설정된 중앙 주소로 LGCP 물리 주소와 별개의 값이다.
func pmbusParseUnitAddr(s string, base int) (uint16, error) {
	if s == "" {
		return 0, fmt.Errorf("%w: address", ErrHvacr03MissingParam)
	}
	// 10진 정수만 허용한다. strconv 대신 직접 파싱하여 "0x10" / "+3" / 공백 등
	// 관대한 해석을 원천 차단한다.
	v := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("%w: %q", ErrHvacr03InvalidAddress, s)
		}
		v = v*10 + int(c-'0')
		if v > 9999 { // 조기 이탈 — 어차피 범위 밖
			return 0, fmt.Errorf("%w: %q", ErrHvacr03InvalidAddress, s)
		}
	}
	n := v - base
	if n < pmbusMinUnitAddr || n > pmbusMaxUnitAddr {
		return 0, fmt.Errorf("%w: %q (base=%d)", ErrHvacr03InvalidAddress, s, base)
	}
	return uint16(n), nil
}

// pmbusFormatUnitAddr 는 내부 N 을 사용자 표기 주소 문자열로 변환한다.
// pmbusParseUnitAddr 의 역함수이다.
func pmbusFormatUnitAddr(n uint16, base int) string {
	return fmt.Sprintf("%d", int(n)+base)
}
