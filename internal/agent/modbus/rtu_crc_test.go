package modbus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===========================================================================
// Modbus RTU CRC-16 테스트 (TDD)
// ===========================================================================
//
// 규격: 반사형 다항식 0xA001, 초기값 0xFFFF.
// 결과는 리틀엔디언 2바이트로 부착한다 (CRC-lo, CRC-hi).
// century(CRC-16/ARC, init 0x0000), samsung(CCITT)와는 다른 알고리즘이다.

// TestModbusCRC16_KnownVectors 는 널리 알려진 Modbus RTU CRC 벡터로 검증한다.
func TestModbusCRC16_KnownVectors(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantCRC uint16 // hi<<8 | lo
		wantLo  byte
		wantHi  byte
	}{
		{
			// SPEC 명시 벡터: 01 03 00 00 00 0A → 와이어 CRC 바이트 C5 CD (lo=0xC5, hi=0xCD)
			name:    "read_holding_10regs",
			data:    []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x0A},
			wantCRC: 0xCDC5,
			wantLo:  0xC5,
			wantHi:  0xCD,
		},
		{
			// Modbus 규격 예제 프레임: 11 03 00 6B 00 03 → CRC 76 87 (lo=0x76, hi=0x87)
			name:    "spec_read_3regs_slave0x11",
			data:    []byte{0x11, 0x03, 0x00, 0x6B, 0x00, 0x03},
			wantCRC: 0x8776,
			wantLo:  0x76,
			wantHi:  0x87,
		},
		{
			// 단일 레지스터 읽기: 01 03 00 00 00 01 → CRC 84 0A (lo=0x84, hi=0x0A)
			name:    "read_1reg",
			data:    []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x01},
			wantCRC: 0x0A84,
			wantLo:  0x84,
			wantHi:  0x0A,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := modbusCRC16(tt.data)
			assert.Equalf(t, tt.wantCRC, got,
				"CRC16(%X) = 0x%04X, want 0x%04X", tt.data, got, tt.wantCRC)
			assert.Equal(t, tt.wantLo, byte(got&0xFF), "CRC-lo (와이어 첫 바이트)")
			assert.Equal(t, tt.wantHi, byte(got>>8), "CRC-hi (와이어 둘째 바이트)")
		})
	}
}

// TestModbusCRC16_Empty 는 빈 입력에 대해 초기값(0xFFFF)을 반환하는지 검증한다.
func TestModbusCRC16_Empty(t *testing.T) {
	assert.Equal(t, uint16(0xFFFF), modbusCRC16(nil))
	assert.Equal(t, uint16(0xFFFF), modbusCRC16([]byte{}))
}

// TestModbusCRC16_AppendAndVerify 는 CRC 를 리틀엔디언으로 부착한 뒤,
// 전체 프레임(데이터+CRC)에 대한 CRC 재계산이 0 이 되는 순환 특성을 검증한다.
func TestModbusCRC16_AppendAndVerify(t *testing.T) {
	data := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x0A}
	crc := modbusCRC16(data)

	full := append(append([]byte{}, data...), byte(crc&0xFF), byte(crc>>8))
	// 데이터 + 리틀엔디언 CRC 전체에 대한 CRC 는 0 이어야 한다 (CRC 자기검증 특성).
	require.Equal(t, uint16(0x0000), modbusCRC16(full),
		"데이터+CRC(리틀엔디언) 전체의 CRC 는 0 이어야 한다")
}
