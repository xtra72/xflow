package modbus

import "errors"

// ---------------------------------------------------------------------------
// Modbus RTU 프레이밍 (공유 exported)
// ---------------------------------------------------------------------------
//
// RTU ADU 형식: [unitID][PDU...][CRC-lo][CRC-hi]
//
// 이 파일은 서버(슬레이브)와 클라이언트(마스터) 양쪽에서 공유하는 순수
// CRC/프레이밍 로직을 exported 형태로 제공한다. 규격은 클라이언트 패키지
// (internal/agent/modbus)의 private 구현(rtu_crc.go/rtu_adu.go)과 동작이
// 동일하다 — 반사형 다항식 0xA001, 초기값 0xFFFF, CRC 는 리틀엔디언으로 부착.
//
// 클라이언트 패키지는 자체 private 사본을 계속 유지한다(스코프 유지). 서버
// 에이전트의 RTU 슬레이브 리스너는 이 공유 함수를 사용한다.

var (
	// ErrRTUFrameTooShort 는 RTU ADU 가 최소 길이(unitID+FC+CRC = 4바이트)보다 짧을 때 반환된다.
	ErrRTUFrameTooShort = errors.New("modbus: RTU frame too short")

	// ErrCRCMismatch 는 RTU 프레임의 CRC-16 재계산 검증이 실패했을 때 반환된다.
	ErrCRCMismatch = errors.New("modbus: RTU CRC mismatch")

	// ErrUnitIDMismatch 는 RTU 응답의 unitID 가 기대 unitID 와 일치하지 않을 때 반환된다.
	ErrUnitIDMismatch = errors.New("modbus: RTU unit ID mismatch")

	// ErrFunctionCodeMismatch 는 RTU 응답의 function code 가 요청과 일치하지 않을 때 반환된다.
	ErrFunctionCodeMismatch = errors.New("modbus: RTU function code mismatch")
)

// rtuCRCPoly 는 Modbus RTU CRC-16 의 반사형 다항식이다.
const rtuCRCPoly uint16 = 0xA001

// rtuCRCInit 는 Modbus RTU CRC-16 의 초기값이다.
const rtuCRCInit uint16 = 0xFFFF

// rtuADUMinLen 은 유효한 RTU ADU 의 최소 길이이다: unitID(1) + FC(1) + CRC(2).
const rtuADUMinLen = 4

// CRC16 은 data 에 대한 Modbus RTU CRC-16 값을 계산한다.
// 반환값은 uint16 이며, 와이어에는 리틀엔디언(byte(crc), byte(crc>>8))으로 부착한다.
func CRC16(data []byte) uint16 {
	crc := rtuCRCInit
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&0x0001 != 0 {
				crc >>= 1
				crc ^= rtuCRCPoly
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

// BuildRTUADU 는 unitID 와 순수 PDU 로부터 RTU ADU 프레임을 생성한다.
// 결과: [unitID][PDU...][CRC-lo][CRC-hi] (CRC 는 리틀엔디언).
func BuildRTUADU(unitID byte, pdu []byte) []byte {
	adu := make([]byte, 0, 1+len(pdu)+2)
	adu = append(adu, unitID)
	adu = append(adu, pdu...)

	crc := CRC16(adu)
	adu = append(adu, byte(crc&0xFF), byte(crc>>8)) // 리틀엔디언: lo 먼저

	return adu
}

// ParseRTUADU 는 수신한 RTU 응답 ADU 를 검증하고 순수 응답 PDU([FC][payload...])를
// 반환한다. CRC 재계산·검증, unitID 일치, function code 일관성을 확인한다.
// CRC 불일치·손상 프레임은 오류로 반환한다.
//
// expectUnitID 는 요청에 사용한 unitID, requestFC 는 요청 function code 이다.
// 예외 응답(requestFC|0x80)은 유효한 프레임으로 통과시킨다.
func ParseRTUADU(adu []byte, expectUnitID byte, requestFC byte) ([]byte, error) {
	if len(adu) < rtuADUMinLen {
		return nil, ErrRTUFrameTooShort
	}

	// CRC 검증: 마지막 2바이트가 리틀엔디언 CRC, 나머지에 대해 재계산.
	dataLen := len(adu) - 2
	wantCRC := uint16(adu[dataLen]) | uint16(adu[dataLen+1])<<8
	if gotCRC := CRC16(adu[:dataLen]); wantCRC != gotCRC {
		return nil, ErrCRCMismatch
	}

	// unitID 일치 확인.
	if adu[0] != expectUnitID {
		return nil, ErrUnitIDMismatch
	}

	// function code 일관성: 정상(requestFC) 또는 예외(requestFC|0x80)만 허용.
	respFC := adu[1]
	if respFC != requestFC && respFC != (requestFC|0x80) {
		return nil, ErrFunctionCodeMismatch
	}

	// 순수 응답 PDU = unitID 와 CRC 를 제외한 [FC][payload...].
	pdu := make([]byte, dataLen-1)
	copy(pdu, adu[1:dataLen])
	return pdu, nil
}
