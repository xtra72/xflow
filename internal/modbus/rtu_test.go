package modbus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===========================================================================
// 공유 Modbus RTU 프레이밍 테스트
// ===========================================================================
//
// 규격: 반사형 다항식 0xA001, 초기값 0xFFFF. CRC 는 리틀엔디언(lo, hi)으로 부착.
// 클라이언트 패키지(internal/agent/modbus)의 private 구현과 동작이 동일하다.

// TestCRC16_KnownVectors 는 널리 알려진 Modbus RTU CRC 벡터로 검증한다.
func TestCRC16_KnownVectors(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantCRC uint16
		wantLo  byte
		wantHi  byte
	}{
		{
			name:    "read_holding_10regs",
			data:    []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x0A},
			wantCRC: 0xCDC5,
			wantLo:  0xC5,
			wantHi:  0xCD,
		},
		{
			name:    "spec_read_3regs_slave0x11",
			data:    []byte{0x11, 0x03, 0x00, 0x6B, 0x00, 0x03},
			wantCRC: 0x8776,
			wantLo:  0x76,
			wantHi:  0x87,
		},
		{
			name:    "read_1reg",
			data:    []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x01},
			wantCRC: 0x0A84,
			wantLo:  0x84,
			wantHi:  0x0A,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CRC16(tt.data)
			assert.Equalf(t, tt.wantCRC, got, "CRC16(%X) = 0x%04X, want 0x%04X", tt.data, got, tt.wantCRC)
			assert.Equal(t, tt.wantLo, byte(got&0xFF), "CRC-lo (와이어 첫 바이트)")
			assert.Equal(t, tt.wantHi, byte(got>>8), "CRC-hi (와이어 둘째 바이트)")
		})
	}
}

// TestCRC16_Empty 는 빈 입력에 대해 초기값(0xFFFF)을 반환하는지 검증한다.
func TestCRC16_Empty(t *testing.T) {
	assert.Equal(t, uint16(0xFFFF), CRC16(nil))
	assert.Equal(t, uint16(0xFFFF), CRC16([]byte{}))
}

// TestBuildRTUADU 는 unitID+PDU 로부터 CRC 리틀엔디언 부착 프레임을 생성하는지 검증한다.
func TestBuildRTUADU(t *testing.T) {
	// unitID=0x01, PDU = 03 00 00 00 0A → 전체 프레임 = 01 03 00 00 00 0A + CRC(lo,hi)
	pdu := []byte{0x03, 0x00, 0x00, 0x00, 0x0A}
	adu := BuildRTUADU(0x01, pdu)

	require.Len(t, adu, 1+len(pdu)+2)
	assert.Equal(t, byte(0x01), adu[0], "unitID")
	assert.Equal(t, pdu, adu[1:1+len(pdu)], "PDU 본문")

	// CRC 는 [unitID][PDU] 전체에 대한 값이며 리틀엔디언으로 부착된다.
	wantCRC := CRC16(adu[:len(adu)-2])
	assert.Equal(t, byte(wantCRC&0xFF), adu[len(adu)-2], "CRC-lo")
	assert.Equal(t, byte(wantCRC>>8), adu[len(adu)-1], "CRC-hi")
}

// TestParseRTUADU_RoundTrip 는 BuildRTUADU 로 만든 응답 프레임을 ParseRTUADU 로
// 되돌려 순수 PDU 를 복원하는지 검증한다(왕복 특성).
func TestParseRTUADU_RoundTrip(t *testing.T) {
	// FC03 읽기 응답 PDU: [FC=03][byteCount=04][data 4바이트]
	respPDU := []byte{0x03, 0x04, 0x00, 0x0A, 0x00, 0x0B}
	adu := BuildRTUADU(0x01, respPDU)

	got, err := ParseRTUADU(adu, 0x01, 0x03)
	require.NoError(t, err)
	assert.Equal(t, respPDU, got)
}

// TestParseRTUADU_Errors 는 손상 프레임/불일치 케이스를 검증한다.
func TestParseRTUADU_Errors(t *testing.T) {
	respPDU := []byte{0x03, 0x02, 0x00, 0x0A}
	adu := BuildRTUADU(0x01, respPDU)

	t.Run("too_short", func(t *testing.T) {
		_, err := ParseRTUADU([]byte{0x01, 0x03}, 0x01, 0x03)
		assert.ErrorIs(t, err, ErrRTUFrameTooShort)
	})

	t.Run("crc_mismatch", func(t *testing.T) {
		bad := append([]byte{}, adu...)
		bad[len(bad)-1] ^= 0xFF // CRC 손상
		_, err := ParseRTUADU(bad, 0x01, 0x03)
		assert.ErrorIs(t, err, ErrCRCMismatch)
	})

	t.Run("unit_id_mismatch", func(t *testing.T) {
		_, err := ParseRTUADU(adu, 0x02, 0x03)
		assert.ErrorIs(t, err, ErrUnitIDMismatch)
	})

	t.Run("function_code_mismatch", func(t *testing.T) {
		_, err := ParseRTUADU(adu, 0x01, 0x04)
		assert.ErrorIs(t, err, ErrFunctionCodeMismatch)
	})

	t.Run("exception_passthrough", func(t *testing.T) {
		// 예외 응답(FC|0x80)은 유효 프레임으로 통과해야 한다.
		excPDU := []byte{0x83, 0x02} // FC03 예외 + IllegalDataAddress
		excADU := BuildRTUADU(0x01, excPDU)
		got, err := ParseRTUADU(excADU, 0x01, 0x03)
		require.NoError(t, err)
		assert.Equal(t, excPDU, got)
	})
}
