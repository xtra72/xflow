package modbus

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===========================================================================
// buildMBAPFrame — ADU 프레이밍 특성화(byte-identical) 테스트
// ===========================================================================
//
// M1 리팩터링(ADU-중립화) 전, buildReadRequest/buildWrite*Request 는 MBAP+PDU
// 전체 프레임을 직접 생성했다. 리팩터링 후에는 buildReadPDU 등이 순수 PDU 를,
// buildMBAPFrame 이 MBAP 부착을 담당한다. 아래 골든 바이트는 리팩터링 이전
// buildReadRequest(0x0001, 1, FC03, 0x0000, 10) 가 산출하던 12바이트 프레임과
// 바이트 단위로 동일함을 고정하여 `modbus-tcp` 와이어 동작 보존을 검증한다.

func TestBuildMBAPFrame_ReadPDU_ByteIdentical(t *testing.T) {
	pdu := buildReadPDU(FC03ReadHoldingRegisters, 0x0000, 10)
	frame := buildMBAPFrame(0x0001, 1, pdu)

	// 리팩터링 이전 buildReadRequest 가 생성하던 정확한 12바이트
	want := []byte{
		0x00, 0x01, // Transaction ID
		0x00, 0x00, // Protocol ID
		0x00, 0x06, // Length: UnitID(1)+FC(1)+StartAddr(2)+Quantity(2)
		0x01,       // Unit ID
		0x03,       // Function Code (FC03)
		0x00, 0x00, // Start Address
		0x00, 0x0A, // Quantity (10)
	}
	assert.Equal(t, want, frame, "MBAP+PDU 프레임이 리팩터링 이전과 바이트 단위로 동일해야 한다")
}

// TestBuildMBAPFrame_LengthField 는 Length 필드가 UnitID(1)+PDU 길이임을 검증한다.
func TestBuildMBAPFrame_LengthField(t *testing.T) {
	tests := []struct {
		name string
		pdu  []byte
	}{
		{name: "read_pdu_5bytes", pdu: buildReadPDU(FC03ReadHoldingRegisters, 0, 10)},
		{name: "write_single_5bytes", pdu: buildWriteSingleRegisterPDU(200, 0x1234)},
		{name: "write_multi_regs", pdu: buildWriteMultipleRegistersPDU(300, []uint16{1, 2, 3})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := buildMBAPFrame(0x0007, 9, tt.pdu)
			require.Len(t, frame, MBAPHeaderSize+len(tt.pdu))
			assert.Equal(t, uint16(0x0007), binary.BigEndian.Uint16(frame[0:2]), "Transaction ID")
			assert.Equal(t, MBAPProtocolID, binary.BigEndian.Uint16(frame[2:4]), "Protocol ID")
			assert.Equal(t, uint16(1+len(tt.pdu)), binary.BigEndian.Uint16(frame[4:6]), "Length")
			assert.Equal(t, byte(9), frame[6], "Unit ID")
			assert.Equal(t, tt.pdu, frame[MBAPHeaderSize:], "PDU 본문")
		})
	}
}

// ===========================================================================
// buildReadPDU 테스트 (순수 PDU)
// ===========================================================================

// TestBuildReadPDU 는 FC03 보유 레지스터 읽기 PDU 구조를 검증한다.
func TestBuildReadPDU(t *testing.T) {
	pdu := buildReadPDU(FC03ReadHoldingRegisters, 0x0000, 10)

	require.Len(t, pdu, 5, "PDU 길이는 5 바이트여야 한다")
	assert.Equal(t, FC03ReadHoldingRegisters, pdu[0], "Function Code")
	assert.Equal(t, uint16(0x0000), binary.BigEndian.Uint16(pdu[1:3]), "Start Address")
	assert.Equal(t, uint16(10), binary.BigEndian.Uint16(pdu[3:5]), "Quantity")
}

// TestBuildReadPDU_AllFunctionCodes 는 모든 읽기 기능 코드(FC01~FC04)에 대해
// PDU 가 올바르게 생성되는지 검증한다.
func TestBuildReadPDU_AllFunctionCodes(t *testing.T) {
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
			pdu := buildReadPDU(tt.fc, 100, 20)

			require.Len(t, pdu, 5)
			assert.Equal(t, tt.fc, pdu[0], "Function Code")
			assert.Equal(t, uint16(100), binary.BigEndian.Uint16(pdu[1:3]), "Start Address")
			assert.Equal(t, uint16(20), binary.BigEndian.Uint16(pdu[3:5]), "Quantity")
		})
	}
}

// ===========================================================================
// parseReadResponse 테스트 (순수 PDU 입력)
// ===========================================================================

// TestParseReadResponse 는 FC03 정상 응답 PDU(10 레지스터)를 올바르게 파싱하는지 검증한다.
func TestParseReadResponse(t *testing.T) {
	// FC03 응답 PDU: FC(1) + ByteCount(1) + Data(20) = 22 바이트
	pdu := make([]byte, 22)
	pdu[0] = FC03ReadHoldingRegisters // FC
	pdu[1] = 20                       // Byte Count

	// 10 레지스터 값 채우기 (0x0001 ~ 0x000A)
	for i := 0; i < 10; i++ {
		binary.BigEndian.PutUint16(pdu[2+i*2:4+i*2], uint16(i+1))
	}

	fc, values, err := parseReadResponse(pdu)
	require.NoError(t, err)

	assert.Equal(t, FC03ReadHoldingRegisters, fc, "Function Code")
	require.Len(t, values, 20, "응답 데이터 바이트 수")

	// 레지스터 디코딩 검증
	regs := decodeRegisters(values)
	require.Len(t, regs, 10)
	for i := 0; i < 10; i++ {
		assert.Equal(t, uint16(i+1), regs[i], "Register[%d]", i)
	}
}

// TestParseReadResponse_Exception 은 MODBUS 예외 응답 PDU 를 올바르게 감지하는지 검증한다.
func TestParseReadResponse_Exception(t *testing.T) {
	// 예외 응답 PDU: FC|0x80(1) + ExceptionCode(1) = 2 바이트
	pdu := []byte{
		FC03ReadHoldingRegisters | 0x80, // 예외 표시
		ExceptionIllegalDataAddress,     // 예외 코드
	}

	fc, values, err := parseReadResponse(pdu)

	require.Error(t, err)
	assert.Nil(t, values)
	assert.Equal(t, FC03ReadHoldingRegisters, fc, "예외 응답에서도 원본 FC 를 반환해야 한다")

	// ModbusException 타입 확인
	var mexc *ModbusException
	require.ErrorAs(t, err, &mexc)
	assert.Equal(t, ExceptionIllegalDataAddress, mexc.Code)
	assert.Equal(t, FC03ReadHoldingRegisters, mexc.FunctionCode)
}

// TestParseReadResponse_FrameTooShort 는 짧은 PDU 에 대해 에러를 반환하는지 검증한다.
func TestParseReadResponse_FrameTooShort(t *testing.T) {
	tests := []struct {
		name string
		pdu  []byte
	}{
		{name: "empty", pdu: []byte{}},
		{name: "fc_only_1byte", pdu: make([]byte, 1)},
		{name: "bytecount_exceeds_data", pdu: []byte{FC03ReadHoldingRegisters, 20, 0x00}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseReadResponse(tt.pdu)
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
// buildWriteSingleCoilPDU 테스트
// ===========================================================================

// TestBuildWriteSingleCoilPDU 는 FC05 단일 코일 쓰기 PDU 를 검증한다.
func TestBuildWriteSingleCoilPDU(t *testing.T) {
	tests := []struct {
		name     string
		value    bool
		expectHi byte
		expectLo byte
	}{
		{name: "coil_on", value: true, expectHi: 0xFF, expectLo: 0x00},
		{name: "coil_off", value: false, expectHi: 0x00, expectLo: 0x00},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdu := buildWriteSingleCoilPDU(100, tt.value)

			require.Len(t, pdu, 5)
			assert.Equal(t, FC05WriteSingleCoil, pdu[0], "Function Code")
			assert.Equal(t, uint16(100), binary.BigEndian.Uint16(pdu[1:3]), "Address")
			assert.Equal(t, tt.expectHi, pdu[3], "Value High Byte")
			assert.Equal(t, tt.expectLo, pdu[4], "Value Low Byte")
		})
	}
}

// ===========================================================================
// buildWriteSingleRegisterPDU 테스트
// ===========================================================================

// TestBuildWriteSingleRegisterPDU 는 FC06 단일 레지스터 쓰기 PDU 를 검증한다.
func TestBuildWriteSingleRegisterPDU(t *testing.T) {
	pdu := buildWriteSingleRegisterPDU(200, 0x1234)

	require.Len(t, pdu, 5)
	assert.Equal(t, FC06WriteSingleRegister, pdu[0], "Function Code")
	assert.Equal(t, uint16(200), binary.BigEndian.Uint16(pdu[1:3]), "Address")
	assert.Equal(t, uint16(0x1234), binary.BigEndian.Uint16(pdu[3:5]), "Value")
}

// ===========================================================================
// buildWriteMultipleCoilsPDU 테스트
// ===========================================================================

// TestBuildWriteMultipleCoilsPDU 는 FC15 다중 코일 쓰기 PDU 를 검증한다.
func TestBuildWriteMultipleCoilsPDU(t *testing.T) {
	// 10개 코일: true, false, true, true, false, false, true, true, true, true
	values := []bool{true, false, true, true, false, false, true, true, true, true}
	pdu := buildWriteMultipleCoilsPDU(50, values)

	// FC(1) + Addr(2) + Qty(2) + ByteCount(1) + Data(2) = 8
	require.Len(t, pdu, 8)
	assert.Equal(t, FC15WriteMultipleCoils, pdu[0], "Function Code")
	assert.Equal(t, uint16(50), binary.BigEndian.Uint16(pdu[1:3]), "Start Address")
	assert.Equal(t, uint16(10), binary.BigEndian.Uint16(pdu[3:5]), "Quantity")
	assert.Equal(t, byte(2), pdu[5], "Byte Count")

	// 코일 데이터 검증: 0xCD = 11001101, 0x03 = 00000011
	assert.Equal(t, byte(0xCD), pdu[6], "Coil Data Byte 0")
	assert.Equal(t, byte(0x03), pdu[7], "Coil Data Byte 1")
}

// ===========================================================================
// buildWriteMultipleRegistersPDU 테스트
// ===========================================================================

// TestBuildWriteMultipleRegistersPDU 는 FC16 다중 레지스터 쓰기 PDU 를 검증한다.
func TestBuildWriteMultipleRegistersPDU(t *testing.T) {
	values := []uint16{0x000A, 0x0064, 0xFFFF}
	pdu := buildWriteMultipleRegistersPDU(300, values)

	// FC(1) + Addr(2) + Qty(2) + ByteCount(1) + Data(6) = 12
	require.Len(t, pdu, 12)
	assert.Equal(t, FC16WriteMultipleRegisters, pdu[0], "Function Code")
	assert.Equal(t, uint16(300), binary.BigEndian.Uint16(pdu[1:3]), "Start Address")
	assert.Equal(t, uint16(3), binary.BigEndian.Uint16(pdu[3:5]), "Quantity")
	assert.Equal(t, byte(6), pdu[5], "Byte Count")

	// 레지스터 데이터 검증
	assert.Equal(t, uint16(0x000A), binary.BigEndian.Uint16(pdu[6:8]), "Register 0")
	assert.Equal(t, uint16(0x0064), binary.BigEndian.Uint16(pdu[8:10]), "Register 1")
	assert.Equal(t, uint16(0xFFFF), binary.BigEndian.Uint16(pdu[10:12]), "Register 2")
}

// ===========================================================================
// parseWriteResponse 테스트 (순수 PDU 입력)
// ===========================================================================

// TestParseWriteResponse 는 정상 쓰기 응답 PDU 를 올바르게 파싱하는지 검증한다.
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
			// 쓰기 응답 PDU: FC(1) + Addr(2) + Value/Qty(2) = 5 바이트
			pdu := make([]byte, 5)
			pdu[0] = tt.fc
			binary.BigEndian.PutUint16(pdu[1:3], tt.addr)
			binary.BigEndian.PutUint16(pdu[3:5], tt.value)

			fc, addr, quantity, err := parseWriteResponse(pdu)
			require.NoError(t, err)

			assert.Equal(t, tt.fc, fc)
			assert.Equal(t, tt.addr, addr)
			assert.Equal(t, tt.wantQuantity, quantity)
		})
	}
}

// TestParseWriteResponse_Exception 은 쓰기 응답 PDU 에서 예외를 올바르게 감지하는지 검증한다.
func TestParseWriteResponse_Exception(t *testing.T) {
	// 예외 응답 PDU: FC|0x80(1) + ExceptionCode(1) = 2 바이트
	pdu := []byte{
		FC06WriteSingleRegister | 0x80,
		ExceptionSlaveDeviceFailure,
	}

	fc, _, _, err := parseWriteResponse(pdu)
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
