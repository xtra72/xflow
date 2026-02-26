package modbus

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===========================================================================
// buildReadRequest 테스트
// ===========================================================================

// TestBuildReadRequest 는 FC03 보유 레지스터 읽기 요청의 MBAP 프레임 구조를 검증한다.
func TestBuildReadRequest(t *testing.T) {
	frame := buildReadRequest(0x0001, 1, FC03ReadHoldingRegisters, 0x0000, 10)

	require.Len(t, frame, 12, "프레임 길이는 12 바이트여야 한다")

	// MBAP 헤더 검증
	assert.Equal(t, uint16(0x0001), binary.BigEndian.Uint16(frame[0:2]), "Transaction ID")
	assert.Equal(t, MBAPProtocolID, binary.BigEndian.Uint16(frame[2:4]), "Protocol ID")
	assert.Equal(t, uint16(6), binary.BigEndian.Uint16(frame[4:6]), "Length")
	assert.Equal(t, byte(1), frame[6], "Unit ID")

	// PDU 검증
	assert.Equal(t, FC03ReadHoldingRegisters, frame[7], "Function Code")
	assert.Equal(t, uint16(0x0000), binary.BigEndian.Uint16(frame[8:10]), "Start Address")
	assert.Equal(t, uint16(10), binary.BigEndian.Uint16(frame[10:12]), "Quantity")
}

// TestBuildReadRequest_AllFunctionCodes 는 모든 읽기 기능 코드(FC01~FC04)에 대해
// 프레임이 올바르게 생성되는지 검증한다.
func TestBuildReadRequest_AllFunctionCodes(t *testing.T) {
	tests := []struct {
		name string
		fc   byte
	}{
		{name: "FC01_ReadCoils", fc: FC01ReadCoils},
		{name: "FC02_ReadDiscreteInputs", fc: FC02ReadDiscreteInputs},
		{name: "FC03_ReadHoldingRegisters", fc: FC03ReadHoldingRegisters},
		{name: "FC04_ReadInputRegisters", fc: FC04ReadInputRegisters},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := buildReadRequest(0x0042, 5, tt.fc, 100, 20)

			require.Len(t, frame, 12)
			assert.Equal(t, uint16(0x0042), binary.BigEndian.Uint16(frame[0:2]), "Transaction ID")
			assert.Equal(t, byte(5), frame[6], "Unit ID")
			assert.Equal(t, tt.fc, frame[7], "Function Code")
			assert.Equal(t, uint16(100), binary.BigEndian.Uint16(frame[8:10]), "Start Address")
			assert.Equal(t, uint16(20), binary.BigEndian.Uint16(frame[10:12]), "Quantity")
		})
	}
}

// ===========================================================================
// parseReadResponse 테스트
// ===========================================================================

// TestParseReadResponse 는 FC03 정상 응답(10 레지스터)을 올바르게 파싱하는지 검증한다.
func TestParseReadResponse(t *testing.T) {
	// FC03 응답: MBAP(7) + FC(1) + ByteCount(1) + Data(20) = 29 바이트
	data := make([]byte, 29)
	binary.BigEndian.PutUint16(data[0:2], 0x0001) // Transaction ID
	binary.BigEndian.PutUint16(data[2:4], 0x0000) // Protocol ID
	binary.BigEndian.PutUint16(data[4:6], 23)      // Length: 1+1+1+20
	data[6] = 1                                     // Unit ID
	data[7] = FC03ReadHoldingRegisters              // FC
	data[8] = 20                                    // Byte Count

	// 10 레지스터 값 채우기 (0x0001 ~ 0x000A)
	for i := 0; i < 10; i++ {
		binary.BigEndian.PutUint16(data[9+i*2:11+i*2], uint16(i+1))
	}

	unitID, fc, values, err := parseReadResponse(data)
	require.NoError(t, err)

	assert.Equal(t, byte(1), unitID, "Unit ID")
	assert.Equal(t, FC03ReadHoldingRegisters, fc, "Function Code")
	require.Len(t, values, 20, "응답 데이터 바이트 수")

	// 레지스터 디코딩 검증
	regs := decodeRegisters(values)
	require.Len(t, regs, 10)
	for i := 0; i < 10; i++ {
		assert.Equal(t, uint16(i+1), regs[i], "Register[%d]", i)
	}
}

// TestParseReadResponse_Exception 은 MODBUS 예외 응답을 올바르게 감지하는지 검증한다.
func TestParseReadResponse_Exception(t *testing.T) {
	// 예외 응답: MBAP(7) + FC|0x80(1) + ExceptionCode(1) = 9 바이트
	data := make([]byte, 9)
	binary.BigEndian.PutUint16(data[0:2], 0x0001) // Transaction ID
	binary.BigEndian.PutUint16(data[2:4], 0x0000) // Protocol ID
	binary.BigEndian.PutUint16(data[4:6], 3)       // Length
	data[6] = 1                                     // Unit ID
	data[7] = FC03ReadHoldingRegisters | 0x80       // 예외 표시
	data[8] = ExceptionIllegalDataAddress            // 예외 코드

	unitID, fc, values, err := parseReadResponse(data)

	require.Error(t, err)
	assert.Nil(t, values)
	assert.Equal(t, byte(1), unitID)
	assert.Equal(t, FC03ReadHoldingRegisters, fc, "예외 응답에서도 원본 FC 를 반환해야 한다")

	// ModbusException 타입 확인
	var mexc *ModbusException
	require.ErrorAs(t, err, &mexc)
	assert.Equal(t, ExceptionIllegalDataAddress, mexc.Code)
	assert.Equal(t, FC03ReadHoldingRegisters, mexc.FunctionCode)
}

// TestParseReadResponse_FrameTooShort 는 짧은 응답에 대해 에러를 반환하는지 검증한다.
func TestParseReadResponse_FrameTooShort(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: []byte{}},
		{name: "too_short_7bytes", data: make([]byte, 7)},
		{name: "too_short_8bytes", data: make([]byte, 8)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := parseReadResponse(tt.data)
			assert.ErrorIs(t, err, ErrFrameTooShort)
		})
	}
}

// ===========================================================================
// decodeCoils 테스트
// ===========================================================================

// TestDecodeCoils 는 바이트 배열을 코일 bool 슬라이스로 올바르게 디코딩하는지 검증한다.
func TestDecodeCoils(t *testing.T) {
	// 바이트 0xCD = 11001101 (비트 0부터: 1,0,1,1,0,0,1,1)
	// 바이트 0x6B = 01101011 (비트 0부터: 1,1,0,1,0,1,1,0)
	// 10개 코일만 사용
	data := []byte{0xCD, 0x6B}
	coils := decodeCoils(data, 10)

	require.Len(t, coils, 10)

	expected := []bool{
		true, false, true, true, false, false, true, true, // 0xCD
		true, true, // 0x6B 의 하위 2비트
	}
	assert.Equal(t, expected, coils)
}

// ===========================================================================
// decodeRegisters 테스트
// ===========================================================================

// TestDecodeRegisters 는 바이트 배열을 uint16 레지스터 슬라이스로 올바르게 디코딩하는지 검증한다.
func TestDecodeRegisters(t *testing.T) {
	data := []byte{
		0x00, 0x01, // 1
		0x00, 0xFF, // 255
		0x01, 0x00, // 256
		0xFF, 0xFF, // 65535
	}

	regs := decodeRegisters(data)
	require.Len(t, regs, 4)
	assert.Equal(t, uint16(1), regs[0])
	assert.Equal(t, uint16(255), regs[1])
	assert.Equal(t, uint16(256), regs[2])
	assert.Equal(t, uint16(65535), regs[3])
}

// ===========================================================================
// buildWriteSingleCoilRequest 테스트
// ===========================================================================

// TestBuildWriteSingleCoilRequest 는 FC05 단일 코일 쓰기 프레임을 검증한다.
func TestBuildWriteSingleCoilRequest(t *testing.T) {
	tests := []struct {
		name      string
		value     bool
		expectHi  byte
		expectLo  byte
	}{
		{name: "coil_on", value: true, expectHi: 0xFF, expectLo: 0x00},
		{name: "coil_off", value: false, expectHi: 0x00, expectLo: 0x00},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := buildWriteSingleCoilRequest(0x0003, 1, 100, tt.value)

			require.Len(t, frame, 12)
			assert.Equal(t, uint16(0x0003), binary.BigEndian.Uint16(frame[0:2]), "Transaction ID")
			assert.Equal(t, MBAPProtocolID, binary.BigEndian.Uint16(frame[2:4]), "Protocol ID")
			assert.Equal(t, uint16(6), binary.BigEndian.Uint16(frame[4:6]), "Length")
			assert.Equal(t, byte(1), frame[6], "Unit ID")
			assert.Equal(t, FC05WriteSingleCoil, frame[7], "Function Code")
			assert.Equal(t, uint16(100), binary.BigEndian.Uint16(frame[8:10]), "Address")
			assert.Equal(t, tt.expectHi, frame[10], "Value High Byte")
			assert.Equal(t, tt.expectLo, frame[11], "Value Low Byte")
		})
	}
}

// ===========================================================================
// buildWriteSingleRegisterRequest 테스트
// ===========================================================================

// TestBuildWriteSingleRegisterRequest 는 FC06 단일 레지스터 쓰기 프레임을 검증한다.
func TestBuildWriteSingleRegisterRequest(t *testing.T) {
	frame := buildWriteSingleRegisterRequest(0x0005, 2, 200, 0x1234)

	require.Len(t, frame, 12)
	assert.Equal(t, uint16(0x0005), binary.BigEndian.Uint16(frame[0:2]), "Transaction ID")
	assert.Equal(t, byte(2), frame[6], "Unit ID")
	assert.Equal(t, FC06WriteSingleRegister, frame[7], "Function Code")
	assert.Equal(t, uint16(200), binary.BigEndian.Uint16(frame[8:10]), "Address")
	assert.Equal(t, uint16(0x1234), binary.BigEndian.Uint16(frame[10:12]), "Value")
}

// ===========================================================================
// buildWriteMultipleCoilsRequest 테스트
// ===========================================================================

// TestBuildWriteMultipleCoilsRequest 는 FC15 다중 코일 쓰기 프레임을 검증한다.
func TestBuildWriteMultipleCoilsRequest(t *testing.T) {
	// 10개 코일: true, false, true, true, false, false, true, true, true, true
	values := []bool{true, false, true, true, false, false, true, true, true, true}
	frame := buildWriteMultipleCoilsRequest(0x0010, 1, 50, values)

	// MBAP(7) + FC(1) + Addr(2) + Qty(2) + ByteCount(1) + Data(2) = 15
	require.Len(t, frame, 15)
	assert.Equal(t, uint16(0x0010), binary.BigEndian.Uint16(frame[0:2]), "Transaction ID")
	assert.Equal(t, FC15WriteMultipleCoils, frame[7], "Function Code")
	assert.Equal(t, uint16(50), binary.BigEndian.Uint16(frame[8:10]), "Start Address")
	assert.Equal(t, uint16(10), binary.BigEndian.Uint16(frame[10:12]), "Quantity")
	assert.Equal(t, byte(2), frame[12], "Byte Count")

	// 코일 데이터 검증: 0xCD = 11001101, 0x03 = 00000011
	assert.Equal(t, byte(0xCD), frame[13], "Coil Data Byte 0")
	assert.Equal(t, byte(0x03), frame[14], "Coil Data Byte 1")
}

// ===========================================================================
// buildWriteMultipleRegistersRequest 테스트
// ===========================================================================

// TestBuildWriteMultipleRegistersRequest 는 FC16 다중 레지스터 쓰기 프레임을 검증한다.
func TestBuildWriteMultipleRegistersRequest(t *testing.T) {
	values := []uint16{0x000A, 0x0064, 0xFFFF}
	frame := buildWriteMultipleRegistersRequest(0x0020, 3, 300, values)

	// MBAP(7) + FC(1) + Addr(2) + Qty(2) + ByteCount(1) + Data(6) = 19
	require.Len(t, frame, 19)
	assert.Equal(t, uint16(0x0020), binary.BigEndian.Uint16(frame[0:2]), "Transaction ID")
	assert.Equal(t, byte(3), frame[6], "Unit ID")
	assert.Equal(t, FC16WriteMultipleRegisters, frame[7], "Function Code")
	assert.Equal(t, uint16(300), binary.BigEndian.Uint16(frame[8:10]), "Start Address")
	assert.Equal(t, uint16(3), binary.BigEndian.Uint16(frame[10:12]), "Quantity")
	assert.Equal(t, byte(6), frame[12], "Byte Count")

	// 레지스터 데이터 검증
	assert.Equal(t, uint16(0x000A), binary.BigEndian.Uint16(frame[13:15]), "Register 0")
	assert.Equal(t, uint16(0x0064), binary.BigEndian.Uint16(frame[15:17]), "Register 1")
	assert.Equal(t, uint16(0xFFFF), binary.BigEndian.Uint16(frame[17:19]), "Register 2")
}

// ===========================================================================
// parseWriteResponse 테스트
// ===========================================================================

// TestParseWriteResponse 는 정상 쓰기 응답을 올바르게 파싱하는지 검증한다.
func TestParseWriteResponse(t *testing.T) {
	tests := []struct {
		name         string
		fc           byte
		addr         uint16
		value        uint16
		wantQuantity uint16
	}{
		{
			name:         "FC05_SingleCoil",
			fc:           FC05WriteSingleCoil,
			addr:         100,
			value:        0xFF00,
			wantQuantity: 1,
		},
		{
			name:         "FC06_SingleRegister",
			fc:           FC06WriteSingleRegister,
			addr:         200,
			value:        0x1234,
			wantQuantity: 1,
		},
		{
			name:         "FC15_MultipleCoils",
			fc:           FC15WriteMultipleCoils,
			addr:         50,
			value:        10, // quantity
			wantQuantity: 10,
		},
		{
			name:         "FC16_MultipleRegisters",
			fc:           FC16WriteMultipleRegisters,
			addr:         300,
			value:        3, // quantity
			wantQuantity: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 쓰기 응답 프레임 생성: MBAP(7) + FC(1) + Addr(2) + Value/Qty(2) = 12 바이트
			data := make([]byte, 12)
			binary.BigEndian.PutUint16(data[0:2], 0x0001)
			binary.BigEndian.PutUint16(data[2:4], 0x0000)
			binary.BigEndian.PutUint16(data[4:6], 6)
			data[6] = 1
			data[7] = tt.fc
			binary.BigEndian.PutUint16(data[8:10], tt.addr)
			binary.BigEndian.PutUint16(data[10:12], tt.value)

			unitID, fc, addr, quantity, err := parseWriteResponse(data)
			require.NoError(t, err)

			assert.Equal(t, byte(1), unitID)
			assert.Equal(t, tt.fc, fc)
			assert.Equal(t, tt.addr, addr)
			assert.Equal(t, tt.wantQuantity, quantity)
		})
	}
}

// TestParseWriteResponse_Exception 은 쓰기 응답에서 예외를 올바르게 감지하는지 검증한다.
func TestParseWriteResponse_Exception(t *testing.T) {
	// 예외 응답: MBAP(7) + FC|0x80(1) + ExceptionCode(1) + 패딩(3) = 12 바이트
	data := make([]byte, 12)
	binary.BigEndian.PutUint16(data[0:2], 0x0001)
	binary.BigEndian.PutUint16(data[2:4], 0x0000)
	binary.BigEndian.PutUint16(data[4:6], 3)
	data[6] = 1
	data[7] = FC06WriteSingleRegister | 0x80
	data[8] = ExceptionSlaveDeviceFailure

	_, fc, _, _, err := parseWriteResponse(data)
	require.Error(t, err)
	assert.Equal(t, FC06WriteSingleRegister, fc)

	var mexc *ModbusException
	require.ErrorAs(t, err, &mexc)
	assert.Equal(t, ExceptionSlaveDeviceFailure, mexc.Code)
}

// ===========================================================================
// ModbusException 테스트
// ===========================================================================

// TestModbusException_Error 는 ModbusException.Error() 출력을 검증한다.
func TestModbusException_Error(t *testing.T) {
	exc := &ModbusException{
		FunctionCode: FC03ReadHoldingRegisters,
		Code:         ExceptionIllegalFunction,
	}

	errMsg := exc.Error()
	assert.Contains(t, errMsg, "modbus: exception")
	assert.Contains(t, errMsg, "Illegal Function")
}
