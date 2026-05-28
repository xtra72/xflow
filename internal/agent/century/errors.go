// Package century 의 센티널 에러 정의.
//
// 각 에러는 SPEC-CENTURY-HVACR-001 의 다층 검증(REQ-CENTURY-011) 단계에 대응한다.
// 단계별로 별도 카운터를 증가시키기 위해 errors.Is 비교가 가능하도록 sentinel 값으로 노출한다.
package century

import "errors"

// 프레임 스캐너 / 파서 단계별 에러 (REQ-CENTURY-011 의 5단계 검증).
var (
	// ErrInvalidLength 는 프레임 전체 길이가 8 + payload_length + 2 와 일치하지 않을 때 반환된다.
	// 단계 1: 길이 검증.
	ErrInvalidLength = errors.New("century_icp01: invalid frame length")

	// ErrInvalidCRC 는 CRC-16/ARC (init 0x0000) 계산값이 트레일 CRC 와 일치하지 않을 때 반환된다.
	// 단계 2: CRC 검증.
	ErrInvalidCRC = errors.New("century_icp01: invalid CRC")

	// ErrInvalidHeader 는 reserved 바이트가 0x00 이 아니거나 function_code 가 알려진 값(0x06/0x0B/0x0C)
	// 외일 때 반환된다.
	// 단계 3: 헤더 검증.
	ErrInvalidHeader = errors.New("century_icp01: invalid frame header")

	// ErrInvalidPayloadPrefix 는 페이로드 prefix 의 reserved2(payload[1]) 가 0x00 이 아니거나
	// register byte 가 {0x02, 0x03, 0x04} 외일 때 반환된다.
	// 단계 4: 페이로드 prefix 검증. ACK(payload_length=1) 는 이 단계를 건너뛴다.
	ErrInvalidPayloadPrefix = errors.New("century_icp01: invalid payload prefix")

	// ErrPayloadTooLarge 는 헤더의 payload_length 가 MaxPayloadLength 를 초과할 때 반환된다.
	// 단계 1 의 일부(합리 범위 검증).
	ErrPayloadTooLarge = errors.New("century_icp01: payload length exceeds maximum")

	// ErrUnknownFunctionCode 는 function_code 가 알려진 enum 외일 때 반환된다.
	// ErrInvalidHeader 의 specialization 으로도 사용 가능하나, 호출자가 명확히 분리하고 싶을 때 사용한다.
	ErrUnknownFunctionCode = errors.New("century_icp01: unknown function code")

	// ErrUnknownRegister 는 register byte 가 알려진 enum 외일 때 반환된다.
	// ErrInvalidPayloadPrefix 의 specialization.
	ErrUnknownRegister = errors.New("century_icp01: unknown register")

	// ErrInvalidRegister 는 디코더가 자신이 처리할 수 있는 register 외의 frame 을 받았을 때 반환된다.
	// 예: DecodeReg02 에 register=0x03 frame 이 전달된 경우.
	// (M2 디코더 단계)
	ErrInvalidRegister = errors.New("century_icp01: register does not match decoder")

	// ErrInvalidPayloadLength 는 디코더가 자신이 기대하는 data 길이와 다른 frame 을 받았을 때 반환된다.
	// REQ-CENTURY-011 단계 5 (레지스터별 길이 검증) 에 해당.
	// 예: reg 0x02 응답이 17B 가 아닐 때.
	ErrInvalidPayloadLength = errors.New("century_icp01: payload data length mismatch for decoder")

	// ErrUnsupportedDirection 은 디코더 디스패치가 (src, dst, function_code) 조합을 인식하지 못할 때 반환된다.
	ErrUnsupportedDirection = errors.New("century_icp01: unsupported frame direction for register")

	// ErrNilFrame 은 nil *Frame 이 디코더에 전달되었을 때 반환된다.
	ErrNilFrame = errors.New("century_icp01: nil frame")
)

// M3: 에이전트 설정 / 런타임 에러.
//
// 설정 파싱 단계에서는 ParseHvacr01Config 가 이 sentinel 들을 errors.Is 와 함께
// 사용하여 호출자가 분기를 가능하게 한다 (AC-D2).
var (
	// ErrSerialPortRequired 는 transport_type=serial 인데 serial_port 가 비어있을 때 반환된다.
	ErrSerialPortRequired = errors.New("century_hvacr01: serial_port is required")

	// ErrUnknownTransportType 는 지원하지 않는 transport_type 이 지정되었을 때 반환된다.
	// v0.2.0 부터 "serial", "tcp-client", "tcp-server" 를 지원한다 (REQ-CENTURY-028).
	ErrUnknownTransportType = errors.New("century_hvacr01: unknown transport_type (supports serial / tcp-client / tcp-server)")

	// ErrHvacr01TCPPortRequired 는 transport_type=tcp-client 또는 tcp-server 에서 tcp_port 가
	// 미설정 또는 1~65535 범위 외일 때 반환된다 (REQ-CENTURY-028).
	ErrHvacr01TCPPortRequired = errors.New("century_hvacr01: tcp_port is required for tcp-client and tcp-server transport (1..65535)")

	// ErrHvacr01TCPHostRequired 는 transport_type=tcp-client 에서 tcp_host 가 미설정일 때 반환된다 (REQ-CENTURY-028).
	ErrHvacr01TCPHostRequired = errors.New("century_hvacr01: tcp_host is required for tcp-client transport")

	// ErrHvacr01TCPDialFailed 는 tcp-client 의 dial 이 실패했을 때 underlying error 를 wrap 한다 (REQ-CENTURY-029).
	ErrHvacr01TCPDialFailed = errors.New("century_hvacr01: tcp-client dial failed")

	// ErrHvacr01TCPListenFailed 는 tcp-server 의 net.Listen 이 실패했을 때 underlying error 를 wrap 한다 (REQ-CENTURY-030).
	ErrHvacr01TCPListenFailed = errors.New("century_hvacr01: tcp-server listen failed")

	// ErrTransportPassiveOnly 는 TCP transport wrapper 의 Write 가 호출되었을 때 반환된다.
	// 본 에이전트는 회선 RX-only 패시브 캡처이므로 어떠한 transport.Write 도 허용하지 않는다.
	// (AC-B9 invariant, AC-G8 의 defensive type-level enforcement)
	ErrTransportPassiveOnly = errors.New("century_hvacr01: transport is passive (RX-only); Write is not allowed")

	// ErrInvalidBaudRate 는 baud_rate 가 양수가 아닐 때 반환된다.
	ErrInvalidBaudRate = errors.New("century_hvacr01: invalid baud_rate (must be >= 300)")

	// ErrInvalidRingBufferSize 는 ring_buffer_size 가 16 미만일 때 반환된다.
	ErrInvalidRingBufferSize = errors.New("century_hvacr01: invalid ring_buffer_size (must be >= 16)")

	// ErrInvalidOfflineTimeout 는 offline_timeout 이 양수가 아닐 때 반환된다.
	ErrInvalidOfflineTimeout = errors.New("century_hvacr01: invalid offline_timeout (must be > 0)")

	// ErrInvalidCycleIdleTimeout 는 cycle_idle_timeout 이 양수가 아닐 때 반환된다.
	ErrInvalidCycleIdleTimeout = errors.New("century_hvacr01: invalid cycle_idle_timeout (must be > 0)")

	// ErrInvalidAddress 는 master_address / slave_address / sub_dev_id 등이 파싱 불가일 때 반환된다.
	ErrInvalidAddress = errors.New("century_hvacr01: invalid address value")

	// ErrControlNotSupported 는 Process() 에 알려지지 않은 / 제어성 커맨드가 전달되었을 때 반환된다.
	// 본 에이전트는 패시브 캡처 전용이므로 어떠한 제어도 수행하지 않는다 (REQ-CENTURY-017).
	ErrControlNotSupported = errors.New("century_hvacr01: control commands are not supported (passive sniff mode)")

	// ErrTransportNotOpen 은 트랜스포트가 열려있지 않은 상태에서 작업을 시도했을 때 반환된다.
	ErrTransportNotOpen = errors.New("century_hvacr01: transport not open")

	// ErrAgentStopped 는 에이전트가 정지된 후 작업을 시도했을 때 반환된다.
	ErrAgentStopped = errors.New("century_hvacr01: agent is stopped")

	// ErrHvacr01NoOutputEnabled 는 emit_device_state 와 emit_register_decoded 가 모두 false 로
	// 설정되었을 때 parseHvacr01Config 가 반환한다 (REQ-CENTURY-034, AC-H9).
	//
	// v0.3.0 의 fail-fast 정책: 최소 하나의 output stream 이 활성화되어야 한다.
	// silent 동작 정지 (downstream message rate 0) 를 방지하기 위해 Init 단계에서 차단한다.
	ErrHvacr01NoOutputEnabled = errors.New("century_hvacr01: at least one of emit_device_state or emit_register_decoded must be true")
)
