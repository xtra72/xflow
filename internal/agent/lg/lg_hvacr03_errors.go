package lg

import "errors"

// ---------------------------------------------------------------------------
// LG HVACR-03 (PMBUSB00A Modbus 게이트웨이) 센티널 에러
//
// SPEC-LG-HVACR-003 § M1.
// ---------------------------------------------------------------------------

var (
	// ErrHvacr03UnknownTransportType 은 알 수 없는 트랜스포트 타입이 지정되었을 때 반환된다.
	// lg_hvacr02 와 달리 tcp-server 는 지원하지 않는다 — 본 에이전트는 능동 마스터이므로
	// 서버 모드가 성립하지 않는다.
	ErrHvacr03UnknownTransportType = errors.New("lg_hvacr03: unknown transport_type (must be rtu or tcp-client)")

	// ErrHvacr03SerialPortRequired 는 rtu 모드에서 serial_port 가 누락되었을 때 반환된다.
	ErrHvacr03SerialPortRequired = errors.New("lg_hvacr03: serial_port is required for rtu transport")

	// ErrHvacr03TCPHostRequired 는 tcp-client 모드에서 tcp_host 가 누락되었을 때 반환된다.
	ErrHvacr03TCPHostRequired = errors.New("lg_hvacr03: tcp_host is required for tcp-client transport")

	// ErrHvacr03InvalidSlaveID 는 게이트웨이 슬레이브 주소가 1~16 범위를 벗어났을 때 반환된다.
	// 범위는 보드의 4비트 DIP 스위치(SW_02M)가 표현할 수 있는 값에서 온다.
	ErrHvacr03InvalidSlaveID = errors.New("lg_hvacr03: slave_id must be 1-16")

	// ErrHvacr03PollIntervalTooShort 는 폴링 주기가 하한(5s)보다 짧을 때 반환된다.
	// 그보다 빠르게 돌리면 게이트웨이가 응답을 거르기 시작한다.
	ErrHvacr03PollIntervalTooShort = errors.New("lg_hvacr03: poll_interval must be at least 5s")

	// ErrHvacr03InvalidAddress 는 실내기 주소 N 이 유효하지 않을 때 반환된다.
	// N 은 0~15 정수이며, LGCP 의 8자리 hex 물리 주소와는 별개의 값이다.
	ErrHvacr03InvalidAddress = errors.New("lg_hvacr03: invalid indoor unit address (must be 0-15)")

	// ErrHvacr03ControlNotEnabled 는 제어 기능이 비활성화된 상태에서 제어를 시도할 때 반환된다.
	ErrHvacr03ControlNotEnabled = errors.New("lg_hvacr03: control not enabled for this agent")

	// ErrHvacr03DeviceNotConnected 는 미연결 실내기에 쓰기를 시도할 때 반환된다.
	// 미설치 N 에 대한 쓰기는 예외 응답 없이 조용히 무시될 수 있으므로 사전 차단한다.
	ErrHvacr03DeviceNotConnected = errors.New("lg_hvacr03: indoor unit is not connected")

	// ErrHvacr03TemperatureOutOfRange 는 설정 온도가 허용 범위를 벗어났을 때 반환된다.
	ErrHvacr03TemperatureOutOfRange = errors.New("lg_hvacr03: temperature out of range")

	// ErrHvacr03InvalidMode 는 유효하지 않은 운전 모드가 지정되었을 때 반환된다.
	ErrHvacr03InvalidMode = errors.New("lg_hvacr03: invalid mode")

	// ErrHvacr03InvalidFanSpeed 는 유효하지 않은 풍량이 지정되었을 때 반환된다.
	ErrHvacr03InvalidFanSpeed = errors.New("lg_hvacr03: invalid fan_speed")

	// ErrHvacr03MissingParam 은 필수 파라미터가 누락되었을 때 반환된다.
	ErrHvacr03MissingParam = errors.New("lg_hvacr03: missing required parameter")

	// ErrHvacr03UnsupportedForDeviceType 은 기기 종류가 지원하지 않는 명령을 시도할 때 반환된다.
	// 예: 에어컨 실내기에 ERV 전용 명령.
	ErrHvacr03UnsupportedForDeviceType = errors.New("lg_hvacr03: command not supported for this device type")

	// ErrHvacr03NotConnected 는 트랜스포트가 연결되지 않은 상태에서 트랜잭션을 시도할 때 반환된다.
	ErrHvacr03NotConnected = errors.New("lg_hvacr03: transport not connected")

	// ErrHvacr03ShortResponse 는 응답 PDU 가 기대 길이보다 짧을 때 반환된다.
	ErrHvacr03ShortResponse = errors.New("lg_hvacr03: response PDU too short")

	// ErrHvacr03UnexpectedFunctionCode 는 응답의 function code 가 요청과 다를 때 반환된다.
	ErrHvacr03UnexpectedFunctionCode = errors.New("lg_hvacr03: unexpected function code in response")

	// ErrHvacr03InvalidQuantity 는 읽기/쓰기 개수가 유효 범위를 벗어났을 때 반환된다.
	ErrHvacr03InvalidQuantity = errors.New("lg_hvacr03: invalid quantity")
)

// ModbusException 은 게이트웨이가 반환한 Modbus 예외 응답이다.
// 응답 PDU 의 function code 최상위 비트가 설정된 경우 생성된다.
type ModbusException struct {
	FunctionCode byte
	Code         byte
}

// Error 는 error 인터페이스를 구현한다.
// hexByte 는 패키지 공용 헬퍼(lg_icp02_payload.go)를 재사용한다.
func (e *ModbusException) Error() string {
	return "lg_hvacr03: modbus exception " + modbusExceptionName(e.Code) +
		" (fc=0x" + hexByte(e.FunctionCode) + ", code=0x" + hexByte(e.Code) + ")"
}

// modbusExceptionName 은 표준 Modbus 예외 코드의 이름을 반환한다.
func modbusExceptionName(code byte) string {
	switch code {
	case 0x01:
		return "ILLEGAL_FUNCTION"
	case 0x02:
		return "ILLEGAL_DATA_ADDRESS"
	case 0x03:
		return "ILLEGAL_DATA_VALUE"
	case 0x04:
		return "SLAVE_DEVICE_FAILURE"
	case 0x05:
		return "ACKNOWLEDGE"
	case 0x06:
		return "SLAVE_DEVICE_BUSY"
	case 0x08:
		return "MEMORY_PARITY_ERROR"
	case 0x0A:
		return "GATEWAY_PATH_UNAVAILABLE"
	case 0x0B:
		return "GATEWAY_TARGET_NO_RESPONSE"
	default:
		return "UNKNOWN"
	}
}
