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
	"mid":    2, // 하위 호환 별칭
	"high":   3,
	"turbo":  4,
	"auto":   5,
}

// lgcpModeCodes 는 운전모드 문자열 → 프로토콜 코드 매핑이다.
var lgcpModeCodes = map[string]int{
	"cooling":    0,
	"cool":       0, // LGCP 디코더 출력 별칭
	"dehumidify": 1,
	"dry":        1, // LGCP 디코더 출력 별칭
	"fan":        2,
	"auto":       3,
	"heating":    4,
	"heat":       4, // LGCP 디코더 출력 별칭
}

const (
	lgcpDefaultFanCode  = 5 // auto
	lgcpDefaultModeCode = 0 // cooling
	lgcpMinTemp         = 15.0
	lgcpMaxTemp         = 30.0
)

// modeCodeToOpMode 는 운전모드 코드를 레지스터 0x13 운전 모드 바이트로 변환한다.
// heating(4) → 0x01, 나머지(cooling/dehumidify/fan/auto) → 0x00 (normal).
func modeCodeToOpMode(modeCode int) byte {
	if modeCode == 4 { // heating
		return 0x01
	}
	return 0x00 // normal (cooling, dehumidify, fan, auto)
}

// ---------------------------------------------------------------------------
// 페이로드 인코딩 함수
// ---------------------------------------------------------------------------

// encodePowerPayload 는 전원 ON/OFF 제어 페이로드를 인코딩한다.
// ON:  [0x10, 0xC1, 0x13, 0xC0|opMode, 0x18, 0x41, 0x18, 0x80|compCap, 0x29, 0xC0]
// OFF: [0x10, 0xC0, 0x13, 0xC0, 0x18, 0x40, 0x18, 0x80, 0x29, 0xC0]
//
// 레지스터 0x10 (실외기 활성화): 0xC1=활성, 0xC0=비활성.
// 레지스터 0x13 (운전 모드): 0xC0=normal, 0xC1=heating, 0xC3=defrost.
// 레지스터 0x18 (전원/압축기): 0x41=ON, 0x40=OFF.
// 실내기가 전원 ON을 수락하려면 운전 모드(0x13)가 함께 설정되어야 한다.
func encodePowerPayload(on bool, compCap int, opMode byte) []byte {
	if on {
		v := byte(compCap & 0x0F)
		return []byte{0x10, 0xC1, 0x13, 0xC0 | opMode, 0x18, 0x41, 0x18, 0x80 | v, 0x29, 0xC0}
	}
	return []byte{0x10, 0xC0, 0x13, 0xC0, 0x18, 0x40, 0x18, 0x80, 0x29, 0xC0}
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

// ---------------------------------------------------------------------------
// 서모스탯 사칭 모드 페이로드 (Unit→Controller 방향, 0x60+ 레지스터)
// ---------------------------------------------------------------------------

// encodeThermostatPowerPayload 는 서모스탯(실내기) 레지스터 형식으로
// 전원 ON/OFF 페이로드를 생성한다.
// 캡처 데이터에서 Unit 67→Controller 프레임의 레지스터:
//   - 62,41 = 활성 운전 (냉방/난방 등 압축기 필요 모드)
//   - 62,40 = 비활성 (송풍/자동)
//   - 64,50,XY = 풍량(X)+모드(Y)
//   - 64,8V = 설정 온도 (V = tempC - 15)
//
// Power ON:  [0x62, 0x41, 0x64, 0x50, fanMode, 0x64, 0x80|tempOffset]
// Power OFF: [0x62, 0x40]
func encodeThermostatPowerPayload(on bool, fanCode, modeCode int, tempC float64) []byte {
	if !on {
		return []byte{0x62, 0x40}
	}
	t := int(tempC)
	if t < int(lgcpMinTemp) {
		t = int(lgcpMinTemp)
	}
	if t > int(lgcpMaxTemp) {
		t = int(lgcpMaxTemp)
	}
	tempVal := byte(t - int(lgcpMinTemp))
	fanMode := byte((fanCode << 4) | (modeCode & 0x0F))
	return []byte{0x62, 0x41, 0x64, 0x50, fanMode, 0x64, 0x80 | tempVal}
}

// encodeThermostatTempPayload 는 서모스탯 형식의 설정 온도 페이로드를 생성한다.
// 캡처 데이터: Unit 67이 온도만 변경 시 [0x64, 0x80|offset] 만 전송.
func encodeThermostatTempPayload(tempC float64) ([]byte, error) {
	t := int(tempC)
	if tempC < lgcpMinTemp || tempC > lgcpMaxTemp {
		return nil, fmt.Errorf("%w: %v, must be %.0f-%.0f", ErrLGCPTemperatureOutOfRange, tempC, lgcpMinTemp, lgcpMaxTemp)
	}
	v := byte(t - int(lgcpMinTemp))
	return []byte{0x64, 0x80 | v}, nil
}

// encodeThermostatFanModePayload 는 서모스탯 형식의 풍량+모드 페이로드를 생성한다.
// 캡처 데이터: Unit 67이 모드 변경 시 [0x64, 0x50, (fan<<4)|mode] 전송.
func encodeThermostatFanModePayload(fanCode, modeCode int) []byte {
	xy := byte((fanCode << 4) | (modeCode & 0x0F))
	return []byte{0x64, 0x50, xy}
}

// buildThermostatPayload 는 서모스탯 사칭 모드용 페이로드를 결합한다.
// Unit→Controller 방향의 레지스터 형식을 사용한다.
func buildThermostatPayload(params map[string]interface{}, currentFanCode, currentModeCode int, currentTempC float64) ([]byte, error) {
	var payload []byte

	// 전원 제어 (서모스탯 레지스터: 62,41/62,40)
	if v, ok := params["power"]; ok {
		on, _ := v.(bool)
		payload = append(payload, encodeThermostatPowerPayload(on, currentFanCode, currentModeCode, currentTempC)...)
	}

	// 온도 제어 (서모스탯 레지스터: 64,8V)
	if v, ok := params["target_temperature"]; ok {
		var tempC float64
		switch t := v.(type) {
		case float64:
			tempC = t
		case int:
			tempC = float64(t)
		}
		tempPayload, err := encodeThermostatTempPayload(tempC)
		if err != nil {
			return nil, err
		}
		payload = append(payload, tempPayload...)
	}

	// 풍량/모드 제어 (서모스탯 레지스터: 64,50,XY)
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

	if _, hasFan := params["fan_speed"]; hasFan {
		payload = append(payload, encodeThermostatFanModePayload(fanCode, modeCode)...)
	} else if _, hasMode := params["mode"]; hasMode {
		payload = append(payload, encodeThermostatFanModePayload(fanCode, modeCode)...)
	}

	return payload, nil
}

// appendPayloadCRC 는 페이로드 데이터에 CRC-16/XMODEM 을 추가한다.
// LGCP 프레임의 페이로드는 마지막 2바이트에 자체 CRC 를 포함해야 한다.
// CRC 입력 범위: CMD(2B) + SEQ0(1B) + PLEN(1B) + register_data
// PLEN 은 register_data + CRC 2바이트를 포함한 최종 페이로드 길이이다.
func appendPayloadCRC(cmd [2]byte, seq0 byte, data []byte) []byte {
	plen := byte(len(data) + 2) // data + 2 CRC bytes
	prefix := []byte{cmd[0], cmd[1], seq0, plen}
	crcInput := append(prefix, data...)
	crc := CalcLGCPCRC16(crcInput)
	return append(data, byte(crc>>8), byte(crc&0xFF))
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
		opMode := modeCodeToOpMode(currentModeCode)
		payload = append(payload, encodePowerPayload(on, compCap, opMode)...)
	}

	// 온도 제어
	if v, ok := params["target_temperature"]; ok {
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
