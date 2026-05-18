// Package century 의 센티널 에러 정의.
//
// 각 에러는 SPEC-CENTURY-001 의 다층 검증(REQ-CENTURY-011) 단계에 대응한다.
// 단계별로 별도 카운터를 증가시키기 위해 errors.Is 비교가 가능하도록 sentinel 값으로 노출한다.
package century

import "errors"

// 프레임 스캐너 / 파서 단계별 에러 (REQ-CENTURY-011 의 5단계 검증).
var (
	// ErrInvalidLength 는 프레임 전체 길이가 8 + payload_length + 2 와 일치하지 않을 때 반환된다.
	// 단계 1: 길이 검증.
	ErrInvalidLength = errors.New("century: invalid frame length")

	// ErrInvalidCRC 는 CRC-16/ARC (init 0x0000) 계산값이 트레일 CRC 와 일치하지 않을 때 반환된다.
	// 단계 2: CRC 검증.
	ErrInvalidCRC = errors.New("century: invalid CRC")

	// ErrInvalidHeader 는 reserved 바이트가 0x00 이 아니거나 function_code 가 알려진 값(0x06/0x0B/0x0C)
	// 외일 때 반환된다.
	// 단계 3: 헤더 검증.
	ErrInvalidHeader = errors.New("century: invalid frame header")

	// ErrInvalidPayloadPrefix 는 페이로드 prefix 의 reserved2(payload[1]) 가 0x00 이 아니거나
	// register byte 가 {0x02, 0x03, 0x04} 외일 때 반환된다.
	// 단계 4: 페이로드 prefix 검증. ACK(payload_length=1) 는 이 단계를 건너뛴다.
	ErrInvalidPayloadPrefix = errors.New("century: invalid payload prefix")

	// ErrPayloadTooLarge 는 헤더의 payload_length 가 MaxPayloadLength 를 초과할 때 반환된다.
	// 단계 1 의 일부(합리 범위 검증).
	ErrPayloadTooLarge = errors.New("century: payload length exceeds maximum")

	// ErrUnknownFunctionCode 는 function_code 가 알려진 enum 외일 때 반환된다.
	// ErrInvalidHeader 의 specialization 으로도 사용 가능하나, 호출자가 명확히 분리하고 싶을 때 사용한다.
	ErrUnknownFunctionCode = errors.New("century: unknown function code")

	// ErrUnknownRegister 는 register byte 가 알려진 enum 외일 때 반환된다.
	// ErrInvalidPayloadPrefix 의 specialization.
	ErrUnknownRegister = errors.New("century: unknown register")
)
