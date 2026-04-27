package modbus

import (
	"encoding/binary"
	"fmt"
)

// ---------------------------------------------------------------------------
// MBAP 헤더 상수
// ---------------------------------------------------------------------------

const (
	// MBAPHeaderSize 는 MODBUS/TCP MBAP 헤더의 바이트 크기이다.
	MBAPHeaderSize = 7

	// MBAPProtocolID 는 MODBUS 프로토콜 식별자이다 (항상 0).
	MBAPProtocolID uint16 = 0x0000
)

// ---------------------------------------------------------------------------
// 기능 코드 상수
// ---------------------------------------------------------------------------

const (
	// FC01ReadCoils 는 코일 상태 읽기 기능 코드이다.
	FC01ReadCoils byte = 0x01

	// FC02ReadDiscreteInputs 는 이산 입력 읽기 기능 코드이다.
	FC02ReadDiscreteInputs byte = 0x02

	// FC03ReadHoldingRegisters 는 보유 레지스터 읽기 기능 코드이다.
	FC03ReadHoldingRegisters byte = 0x03

	// FC04ReadInputRegisters 는 입력 레지스터 읽기 기능 코드이다.
	FC04ReadInputRegisters byte = 0x04

	// FC05WriteSingleCoil 는 단일 코일 쓰기 기능 코드이다.
	FC05WriteSingleCoil byte = 0x05

	// FC06WriteSingleRegister 는 단일 레지스터 쓰기 기능 코드이다.
	FC06WriteSingleRegister byte = 0x06

	// FC15WriteMultipleCoils 는 다중 코일 쓰기 기능 코드이다.
	FC15WriteMultipleCoils byte = 0x0F

	// FC16WriteMultipleRegisters 는 다중 레지스터 쓰기 기능 코드이다.
	FC16WriteMultipleRegisters byte = 0x10
)

// ---------------------------------------------------------------------------
// 예외 코드 상수
// ---------------------------------------------------------------------------

const (
	ExceptionIllegalFunction     byte = 0x01
	ExceptionIllegalDataAddress  byte = 0x02
	ExceptionIllegalDataValue    byte = 0x03
	ExceptionSlaveDeviceFailure  byte = 0x04
)

// ---------------------------------------------------------------------------
// 최대 수량 상수
// ---------------------------------------------------------------------------

const (
	MaxCoilsRead      = 2000
	MaxRegistersRead  = 125
	MaxCoilsWrite     = 1968
	MaxRegistersWrite = 123
)

// ---------------------------------------------------------------------------
// MBAP 헤더 구조체
// ---------------------------------------------------------------------------

// MBAPHeader 는 MODBUS/TCP MBAP(MODBUS Application Protocol) 헤더이다.
type MBAPHeader struct {
	TransactionID uint16
	ProtocolID    uint16
	Length        uint16
	UnitID        byte
}

// ---------------------------------------------------------------------------
// ModbusException 은 MODBUS 예외 응답을 나타낸다.
// ---------------------------------------------------------------------------

// ModbusException 은 디바이스가 반환한 MODBUS 예외 정보를 포함한다.
type ModbusException struct {
	FunctionCode byte
	Code         byte
}

// Error 는 error 인터페이스를 구현한다.
func (e *ModbusException) Error() string {
	return fmt.Sprintf("modbus: exception fc=0x%02X code=0x%02X (%s)",
		e.FunctionCode, e.Code, exceptionCodeString(e.Code))
}

// exceptionCodeString 은 예외 코드에 대한 사람이 읽을 수 있는 문자열을 반환한다.
func exceptionCodeString(code byte) string {
	switch code {
	case ExceptionIllegalFunction:
		return "Illegal Function"
	case ExceptionIllegalDataAddress:
		return "Illegal Data Address"
	case ExceptionIllegalDataValue:
		return "Illegal Data Value"
	case ExceptionSlaveDeviceFailure:
		return "Slave Device Failure"
	default:
		return "Unknown"
	}
}

// ---------------------------------------------------------------------------
// 프레임 빌드 함수
// ---------------------------------------------------------------------------

// buildReadRequest 는 MODBUS/TCP 읽기 요청 프레임(MBAP + PDU)을 생성한다.
// fc 는 FC01, FC02, FC03, FC04 중 하나여야 한다.
func buildReadRequest(transactionID uint16, unitID byte, fc byte, startAddr uint16, quantity uint16) []byte {
	// MBAP(7) + FC(1) + StartAddr(2) + Quantity(2) = 12 바이트
	frame := make([]byte, 12)

	// MBAP 헤더
	binary.BigEndian.PutUint16(frame[0:2], transactionID)
	binary.BigEndian.PutUint16(frame[2:4], MBAPProtocolID)
	binary.BigEndian.PutUint16(frame[4:6], 6) // Length: UnitID(1) + FC(1) + StartAddr(2) + Quantity(2)
	frame[6] = unitID

	// PDU
	frame[7] = fc
	binary.BigEndian.PutUint16(frame[8:10], startAddr)
	binary.BigEndian.PutUint16(frame[10:12], quantity)

	return frame
}

// parseReadResponse 는 MODBUS/TCP 읽기 응답을 파싱한다.
// 예외 응답인 경우 ModbusException 에러를 반환한다.
func parseReadResponse(data []byte) (unitID byte, fc byte, values []byte, err error) {
	// 최소 MBAP(7) + FC(1) + ByteCount(1) = 9 바이트
	if len(data) < 9 {
		return 0, 0, nil, ErrFrameTooShort
	}

	unitID = data[6]
	fc = data[7]

	// 예외 응답 감지: 기능 코드의 최상위 비트가 설정됨
	if fc&0x80 != 0 {
		if len(data) < 9 {
			return 0, 0, nil, ErrFrameTooShort
		}
		return unitID, fc & 0x7F, nil, &ModbusException{
			FunctionCode: fc & 0x7F,
			Code:         data[8],
		}
	}

	byteCount := int(data[8])
	if len(data) < 9+byteCount {
		return 0, 0, nil, ErrFrameTooShort
	}

	values = make([]byte, byteCount)
	copy(values, data[9:9+byteCount])

	return unitID, fc, values, nil
}

// ---------------------------------------------------------------------------
// 쓰기 요청 빌드 함수
// ---------------------------------------------------------------------------

// buildWriteSingleCoilRequest 는 FC05 단일 코일 쓰기 요청 프레임을 생성한다.
// value 가 true 이면 0xFF00, false 이면 0x0000 을 전송한다.
func buildWriteSingleCoilRequest(txID uint16, unitID byte, addr uint16, value bool) []byte {
	frame := make([]byte, 12)

	// MBAP 헤더
	binary.BigEndian.PutUint16(frame[0:2], txID)
	binary.BigEndian.PutUint16(frame[2:4], MBAPProtocolID)
	binary.BigEndian.PutUint16(frame[4:6], 6) // Length: UnitID(1) + FC(1) + Addr(2) + Value(2)
	frame[6] = unitID

	// PDU
	frame[7] = FC05WriteSingleCoil
	binary.BigEndian.PutUint16(frame[8:10], addr)
	if value {
		frame[10] = 0xFF
		frame[11] = 0x00
	} else {
		frame[10] = 0x00
		frame[11] = 0x00
	}

	return frame
}

// buildWriteSingleRegisterRequest 는 FC06 단일 레지스터 쓰기 요청 프레임을 생성한다.
func buildWriteSingleRegisterRequest(txID uint16, unitID byte, addr uint16, value uint16) []byte {
	frame := make([]byte, 12)

	// MBAP 헤더
	binary.BigEndian.PutUint16(frame[0:2], txID)
	binary.BigEndian.PutUint16(frame[2:4], MBAPProtocolID)
	binary.BigEndian.PutUint16(frame[4:6], 6) // Length: UnitID(1) + FC(1) + Addr(2) + Value(2)
	frame[6] = unitID

	// PDU
	frame[7] = FC06WriteSingleRegister
	binary.BigEndian.PutUint16(frame[8:10], addr)
	binary.BigEndian.PutUint16(frame[10:12], value)

	return frame
}

// buildWriteMultipleCoilsRequest 는 FC15 다중 코일 쓰기 요청 프레임을 생성한다.
func buildWriteMultipleCoilsRequest(txID uint16, unitID byte, addr uint16, values []bool) []byte {
	quantity := uint16(len(values))
	byteCount := (len(values) + 7) / 8 // 코일 8개당 1바이트

	// MBAP(7) + FC(1) + Addr(2) + Quantity(2) + ByteCount(1) + Data(byteCount)
	frameLen := 7 + 1 + 2 + 2 + 1 + byteCount
	frame := make([]byte, frameLen)

	// MBAP 헤더
	binary.BigEndian.PutUint16(frame[0:2], txID)
	binary.BigEndian.PutUint16(frame[2:4], MBAPProtocolID)
	binary.BigEndian.PutUint16(frame[4:6], uint16(1+1+2+2+1+byteCount)) // Length
	frame[6] = unitID

	// PDU
	frame[7] = FC15WriteMultipleCoils
	binary.BigEndian.PutUint16(frame[8:10], addr)
	binary.BigEndian.PutUint16(frame[10:12], quantity)
	frame[12] = byte(byteCount)

	// 코일 데이터 인코딩: 각 바이트에 최대 8개 코일, LSB 먼저
	for i, v := range values {
		if v {
			byteIdx := i / 8
			bitIdx := uint(i % 8)
			frame[13+byteIdx] |= 1 << bitIdx
		}
	}

	return frame
}

// buildWriteMultipleRegistersRequest 는 FC16 다중 레지스터 쓰기 요청 프레임을 생성한다.
func buildWriteMultipleRegistersRequest(txID uint16, unitID byte, addr uint16, values []uint16) []byte {
	quantity := uint16(len(values))
	byteCount := len(values) * 2

	// MBAP(7) + FC(1) + Addr(2) + Quantity(2) + ByteCount(1) + Data(byteCount)
	frameLen := 7 + 1 + 2 + 2 + 1 + byteCount
	frame := make([]byte, frameLen)

	// MBAP 헤더
	binary.BigEndian.PutUint16(frame[0:2], txID)
	binary.BigEndian.PutUint16(frame[2:4], MBAPProtocolID)
	binary.BigEndian.PutUint16(frame[4:6], uint16(1+1+2+2+1+byteCount)) // Length
	frame[6] = unitID

	// PDU
	frame[7] = FC16WriteMultipleRegisters
	binary.BigEndian.PutUint16(frame[8:10], addr)
	binary.BigEndian.PutUint16(frame[10:12], quantity)
	frame[12] = byte(byteCount)

	// 레지스터 데이터 인코딩: 각 레지스터 2바이트 Big-Endian
	for i, v := range values {
		binary.BigEndian.PutUint16(frame[13+i*2:15+i*2], v)
	}

	return frame
}

// ---------------------------------------------------------------------------
// 쓰기 응답 파싱
// ---------------------------------------------------------------------------

// parseWriteResponse 는 MODBUS/TCP 쓰기 응답(FC05, FC06, FC15, FC16)을 파싱한다.
// 단일 쓰기(FC05, FC06)의 경우 quantity 는 1 로 반환된다.
func parseWriteResponse(data []byte) (unitID byte, fc byte, addr uint16, quantity uint16, err error) {
	// 최소 MBAP(7) + FC(1) = 8 바이트 (예외 응답은 9 바이트)
	if len(data) < 8 {
		return 0, 0, 0, 0, ErrFrameTooShort
	}

	unitID = data[6]
	fc = data[7]

	// 예외 응답 감지
	if fc&0x80 != 0 {
		if len(data) < 9 {
			return 0, 0, 0, 0, ErrFrameTooShort
		}
		return unitID, fc & 0x7F, 0, 0, &ModbusException{
			FunctionCode: fc & 0x7F,
			Code:         data[8],
		}
	}

	// 정상 응답: MBAP(7) + FC(1) + Addr(2) + Value/Quantity(2) = 12 바이트
	if len(data) < 12 {
		return 0, 0, 0, 0, ErrFrameTooShort
	}

	addr = binary.BigEndian.Uint16(data[8:10])
	quantity = binary.BigEndian.Uint16(data[10:12])

	// FC05/FC06 단일 쓰기의 경우 echo-back 값이 반환되므로 quantity 를 1 로 설정
	if fc == FC05WriteSingleCoil || fc == FC06WriteSingleRegister {
		quantity = 1
	}

	return unitID, fc, addr, quantity, nil
}

// ---------------------------------------------------------------------------
// 디코딩 유틸리티
// ---------------------------------------------------------------------------

// decodeCoils 는 바이트 배열에서 코일 상태(bool 슬라이스)를 디코딩한다.
// count 는 실제 코일 수를 지정한다 (패딩 비트 제외).
func decodeCoils(data []byte, count int) []bool {
	coils := make([]bool, count)
	for i := 0; i < count; i++ {
		byteIdx := i / 8
		bitIdx := uint(i % 8)
		if byteIdx < len(data) {
			coils[i] = data[byteIdx]&(1<<bitIdx) != 0
		}
	}
	return coils
}

// decodeRegisters 는 바이트 배열에서 레지스터 값(uint16 슬라이스)을 디코딩한다.
// 바이트 순서는 Big-Endian 이다.
func decodeRegisters(data []byte) []uint16 {
	count := len(data) / 2
	regs := make([]uint16, count)
	for i := 0; i < count; i++ {
		regs[i] = binary.BigEndian.Uint16(data[i*2 : i*2+2])
	}
	return regs
}
