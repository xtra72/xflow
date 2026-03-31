package lg

import (
	"fmt"
)

// ---------------------------------------------------------------------------
// 풍량/모드 코드 매핑
// ---------------------------------------------------------------------------

// lgcpFanSpeedCodes 는 풍량 문자열 → 프로토콜 코드 매핑이다.
var lgcpFanSpeedCodes = map[string]int{
	"low":    1,
	"medium": 2,
	"high":   3,
	"turbo":  4,
	"auto":   5,
}

// lgcpModeCodes 는 운전모드 문자열 → 프로토콜 코드 매핑이다.
var lgcpModeCodes = map[string]int{
	"cooling":    0,
	"dehumidify": 1,
	"fan":        2,
	"auto":       3,
	"heating":    4,
}

const (
	lgcpDefaultFanCode  = 5 // auto
	lgcpDefaultModeCode = 0 // cooling
	lgcpMinTemp         = 15.0
	lgcpMaxTemp         = 30.0
)

// ---------------------------------------------------------------------------
// 페이로드 인코딩 함수
// ---------------------------------------------------------------------------

// encodePowerPayload 는 전원 ON/OFF 제어 페이로드를 인코딩한다.
// ON:  [0x18, 0x41, 0x18, 0x80|compCap, 0x29, 0xC0]
// OFF: [0x18, 0x40, 0x18, 0x80, 0x29, 0xC0]
func encodePowerPayload(on bool, compCap int) []byte {
	if on {
		v := byte(compCap & 0x0F)
		return []byte{0x18, 0x41, 0x18, 0x80 | v, 0x29, 0xC0}
	}
	return []byte{0x18, 0x40, 0x18, 0x80, 0x29, 0xC0}
}

// encodeTemperaturePayload 는 설정 온도 변경 페이로드를 인코딩한다.
// 페이로드: [0x64, 0x80|(int(tempC)-15)]
// 범위: 15.0-30.0, 소수점 버림. 범위 밖이면 에러 반환.
func encodeTemperaturePayload(tempC float64) ([]byte, error) {
	t := int(tempC)
	if tempC < lgcpMinTemp || tempC > lgcpMaxTemp {
		return nil, fmt.Errorf("%w: %v, must be %.0f-%.0f", ErrLGCPTemperatureOutOfRange, tempC, lgcpMinTemp, lgcpMaxTemp)
	}
	v := byte(t - int(lgcpMinTemp))
	return []byte{0x64, 0x80 | v}, nil
}

// encodeFanModePayload 는 풍량+모드 페이로드를 인코딩한다.
// 페이로드: [0x64, 0x50, (fanCode<<4)|(modeCode&0x0F)]
func encodeFanModePayload(fanCode, modeCode int) []byte {
	xy := byte((fanCode << 4) | (modeCode & 0x0F))
	return []byte{0x64, 0x50, xy}
}

// lookupFanSpeedCode 는 풍량 문자열을 프로토콜 코드로 변환한다.
func lookupFanSpeedCode(fanSpeed string) (int, error) {
	code, ok := lgcpFanSpeedCodes[fanSpeed]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrLGCPInvalidFanSpeed, fanSpeed)
	}
	return code, nil
}

// lookupModeCode 는 운전모드 문자열을 프로토콜 코드로 변환한다.
func lookupModeCode(mode string) (int, error) {
	code, ok := lgcpModeCodes[mode]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrLGCPInvalidMode, mode)
	}
	return code, nil
}

// buildControlPayload 는 여러 제어 파라미터를 하나의 페이로드로 결합한다.
// set_multiple 명령에서 사용된다.
func buildControlPayload(params map[string]interface{}, currentFanCode, currentModeCode int) ([]byte, error) {
	var payload []byte

	// 전원 제어
	if v, ok := params["power"]; ok {
		on, _ := v.(bool)
		compCap := 0
		if cc, ok := params["compressor_capacity"]; ok {
			switch c := cc.(type) {
			case float64:
				compCap = int(c)
			case int:
				compCap = c
			}
		}
		payload = append(payload, encodePowerPayload(on, compCap)...)
	}

	// 온도 제어
	if v, ok := params["temperature"]; ok {
		var tempC float64
		switch t := v.(type) {
		case float64:
			tempC = t
		case int:
			tempC = float64(t)
		}
		tempPayload, err := encodeTemperaturePayload(tempC)
		if err != nil {
			return nil, err
		}
		payload = append(payload, tempPayload...)
	}

	// 풍량/모드 제어 (둘 중 하나라도 있으면 fan+mode 페이로드 생성)
	fanCode := currentFanCode
	modeCode := currentModeCode

	if v, ok := params["fan_speed"]; ok {
		s, _ := v.(string)
		code, err := lookupFanSpeedCode(s)
		if err != nil {
			return nil, err
		}
		fanCode = code
	}
	if v, ok := params["mode"]; ok {
		s, _ := v.(string)
		code, err := lookupModeCode(s)
		if err != nil {
			return nil, err
		}
		modeCode = code
	}

	// fan_speed 또는 mode 가 지정되었으면 fan+mode 페이로드 추가
	if _, hasFan := params["fan_speed"]; hasFan {
		payload = append(payload, encodeFanModePayload(fanCode, modeCode)...)
	} else if _, hasMode := params["mode"]; hasMode {
		payload = append(payload, encodeFanModePayload(fanCode, modeCode)...)
	}

	return payload, nil
}
