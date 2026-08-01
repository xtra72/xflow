package modbus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===========================================================================
// RTU ADU 빌드/파싱 테스트 (TDD)
// ===========================================================================
//
// RTU ADU 형식: [unitID][PDU...][CRC-lo][CRC-hi]
// - buildRTUADU: PDU 앞에 unitID, 뒤에 리틀엔디언 CRC 를 부착한다.
// - parseRTUADU: CRC 재계산·검증, unitID 일치, function code 일관성 확인 후
//   순수 응답 PDU([FC][payload...])를 반환한다. 손상 프레임은 오류로 처리한다(AC-07).

// TestBuildRTUADU 는 FC03 읽기 요청의 RTU ADU 프레이밍을 검증한다.
func TestBuildRTUADU(t *testing.T) {
	// PDU: FC03 read, start=0x0000, qty=0x000A
	pdu := buildReadPDU(FC03ReadHoldingRegisters, 0x0000, 10)
	adu := buildRTUADU(0x01, pdu)

	// 기대 ADU: 01 03 00 00 00 0A C5 CD (SPEC 명시 CRC 벡터)
	want := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x0A, 0xC5, 0xCD}
	assert.Equal(t, want, adu, "RTU ADU 는 [unitID][PDU][CRC-lo][CRC-hi] 형식이어야 한다")

	// 구조 검증
	require.Len(t, adu, 1+len(pdu)+2)
	assert.Equal(t, byte(0x01), adu[0], "unitID")
	assert.Equal(t, pdu, adu[1:1+len(pdu)], "PDU 본문")
}

// TestBuildRTUADU_CRCLittleEndian 는 CRC 가 리틀엔디언(lo 먼저)으로 부착되는지 검증한다.
func TestBuildRTUADU_CRCLittleEndian(t *testing.T) {
	pdu := buildReadPDU(FC03ReadHoldingRegisters, 0x0000, 10)
	adu := buildRTUADU(0x01, pdu)

	crc := modbusCRC16(adu[:len(adu)-2])
	assert.Equal(t, byte(crc&0xFF), adu[len(adu)-2], "CRC-lo 는 마지막에서 둘째 바이트")
	assert.Equal(t, byte(crc>>8), adu[len(adu)-1], "CRC-hi 는 마지막 바이트")
}

// TestParseRTUADU_ValidResponse 는 유효한 FC03 응답 ADU 를 파싱해 PDU 를 반환하는지 검증한다.
func TestParseRTUADU_ValidResponse(t *testing.T) {
	// FC03 응답 PDU: FC(0x03) + ByteCount(0x04) + Data(2 레지스터: 0x1234, 0x5678)
	respPDU := []byte{0x03, 0x04, 0x12, 0x34, 0x56, 0x78}
	adu := buildRTUADU(0x01, respPDU)

	got, err := parseRTUADU(adu, 0x01, FC03ReadHoldingRegisters)
	require.NoError(t, err)
	assert.Equal(t, respPDU, got, "파싱 결과는 unitID/CRC 를 제거한 순수 PDU 여야 한다")

	// 상위 파서로 디코딩 가능한지 확인
	fc, values, perr := parseReadResponse(got)
	require.NoError(t, perr)
	assert.Equal(t, FC03ReadHoldingRegisters, fc)
	assert.Equal(t, []uint16{0x1234, 0x5678}, decodeRegisters(values))
}

// TestParseRTUADU_RoundTrip 는 build → parse 왕복 후 원본 PDU 가 보존되는지 검증한다.
func TestParseRTUADU_RoundTrip(t *testing.T) {
	pdu := buildWriteSingleRegisterPDU(200, 0x1234)
	adu := buildRTUADU(0x05, pdu)

	got, err := parseRTUADU(adu, 0x05, FC06WriteSingleRegister)
	require.NoError(t, err)
	assert.Equal(t, pdu, got)
}

// TestParseRTUADU_CRCMismatch 는 CRC 가 손상된 프레임을 오류로 거부하는지 검증한다(AC-07).
func TestParseRTUADU_CRCMismatch(t *testing.T) {
	respPDU := []byte{0x03, 0x02, 0xAB, 0xCD}
	adu := buildRTUADU(0x01, respPDU)

	// 마지막 CRC 바이트를 손상시킨다.
	corrupted := append([]byte{}, adu...)
	corrupted[len(corrupted)-1] ^= 0xFF

	got, err := parseRTUADU(corrupted, 0x01, FC03ReadHoldingRegisters)
	require.ErrorIs(t, err, ErrCRCMismatch, "CRC 불일치는 ErrCRCMismatch 로 거부해야 한다")
	assert.Nil(t, got, "손상 프레임은 유효한 PDU 로 반환하지 않아야 한다")
}

// TestParseRTUADU_PayloadCorruptionDetected 는 페이로드 손상도 CRC 로 탐지되는지 검증한다.
func TestParseRTUADU_PayloadCorruptionDetected(t *testing.T) {
	respPDU := []byte{0x03, 0x02, 0xAB, 0xCD}
	adu := buildRTUADU(0x01, respPDU)

	corrupted := append([]byte{}, adu...)
	corrupted[3] ^= 0x01 // 페이로드 비트 반전 (CRC 는 그대로)

	_, err := parseRTUADU(corrupted, 0x01, FC03ReadHoldingRegisters)
	require.ErrorIs(t, err, ErrCRCMismatch)
}

// TestParseRTUADU_UnitIDMismatch 는 unitID 불일치를 오류로 거부하는지 검증한다.
func TestParseRTUADU_UnitIDMismatch(t *testing.T) {
	respPDU := []byte{0x03, 0x02, 0xAB, 0xCD}
	adu := buildRTUADU(0x02, respPDU) // unitID=0x02 로 프레이밍

	_, err := parseRTUADU(adu, 0x01, FC03ReadHoldingRegisters) // 0x01 을 기대
	require.ErrorIs(t, err, ErrUnitIDMismatch)
}

// TestParseRTUADU_FunctionCodeMismatch 는 function code 불일치를 오류로 거부하는지 검증한다.
func TestParseRTUADU_FunctionCodeMismatch(t *testing.T) {
	// 요청은 FC03 이지만 응답 PDU 는 FC04 로 시작 (일관성 위반)
	respPDU := []byte{FC04ReadInputRegisters, 0x02, 0xAB, 0xCD}
	adu := buildRTUADU(0x01, respPDU)

	_, err := parseRTUADU(adu, 0x01, FC03ReadHoldingRegisters)
	require.ErrorIs(t, err, ErrFunctionCodeMismatch)
}

// TestParseRTUADU_ExceptionResponseAllowed 는 예외 응답(fc|0x80)이 유효한 프레임으로
// 통과되고 상위 파서가 예외를 감지하는지 검증한다.
func TestParseRTUADU_ExceptionResponseAllowed(t *testing.T) {
	// FC03 예외 응답 PDU: [FC|0x80][ExceptionCode]
	respPDU := []byte{FC03ReadHoldingRegisters | 0x80, ExceptionIllegalDataAddress}
	adu := buildRTUADU(0x01, respPDU)

	got, err := parseRTUADU(adu, 0x01, FC03ReadHoldingRegisters)
	require.NoError(t, err, "예외 응답은 유효한 RTU 프레임으로 통과해야 한다")
	assert.Equal(t, respPDU, got)

	// 상위 파서가 ModbusException 을 감지하는지 확인
	_, _, perr := parseReadResponse(got)
	var mexc *ModbusException
	require.ErrorAs(t, perr, &mexc)
	assert.Equal(t, ExceptionIllegalDataAddress, mexc.Code)
}

// TestParseRTUADU_FrameTooShort 는 최소 길이 미만 프레임을 오류로 거부하는지 검증한다.
func TestParseRTUADU_FrameTooShort(t *testing.T) {
	tests := []struct {
		name string
		adu  []byte
	}{
		{name: "empty", adu: []byte{}},
		{name: "one_byte", adu: []byte{0x01}},
		{name: "three_bytes", adu: []byte{0x01, 0x03, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRTUADU(tt.adu, 0x01, FC03ReadHoldingRegisters)
			assert.ErrorIs(t, err, ErrRTUFrameTooShort)
		})
	}
}
