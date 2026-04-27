package samsung

import (
	"testing"
)

func TestCalcCRC16(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want uint16
	}{
		{
			name: "empty data returns 0x0000",
			data: []byte{},
			want: 0x0000,
		},
		{
			name: "single byte 0x00",
			data: []byte{0x00},
			want: 0x0000,
		},
		{
			name: "single byte 0x01",
			data: []byte{0x01},
			want: 0x1021,
		},
		{
			name: "protocol spec reference frame",
			// Frame: 32 0015 620000 200000 C013 A8 02 400001 42010118 CD4D 34
			// CRC is over SA+DA+CMD+SEQ#+CNT+MSGs (excludes LEN)
			data: []byte{
				0x62, 0x00, 0x00, // SA
				0x20, 0x00, 0x00, // DA
				0xC0, 0x13, // CMD
				0xA8,       // SEQ#
				0x02,       // CNT
				0x40, 0x00, 0x01, // MSG1
				0x42, 0x01, 0x01, 0x18, // MSG2
			},
			want: 0xCD4D,
		},
		{
			name: "single byte 0xFF",
			data: []byte{0xFF},
			want: 0x1EF0,
		},
		{
			name: "two bytes ascending",
			data: []byte{0x01, 0x02},
			want: 0x1373,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalcCRC16(tt.data)
			if got != tt.want {
				t.Errorf("CalcCRC16(%#v) = 0x%04X, want 0x%04X", tt.data, got, tt.want)
			}
		})
	}
}

func TestVerifyCRC16(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "valid CRC from protocol spec frame",
			// SA+DA+CMD+SEQ#+CNT+MSGs with CRC appended in Big-Endian
			data: []byte{
				0x62, 0x00, 0x00, // SA
				0x20, 0x00, 0x00, // DA
				0xC0, 0x13, // CMD
				0xA8,       // SEQ#
				0x02,       // CNT
				0x40, 0x00, 0x01, // MSG1
				0x42, 0x01, 0x01, 0x18, // MSG2
				0xCD, 0x4D, // CRC16 in Big-Endian
			},
			want: true,
		},
		{
			name: "invalid CRC returns false",
			data: []byte{
				0x62, 0x00, 0x00,
				0x20, 0x00, 0x00,
				0xC0, 0x13,
				0xA8,
				0x02,
				0x40, 0x00, 0x01,
				0x42, 0x01, 0x01, 0x18,
				0xFF, 0xFF, // wrong CRC
			},
			want: false,
		},
		{
			name: "data too short - empty",
			data: []byte{},
			want: false,
		},
		{
			name: "data too short - one byte",
			data: []byte{0x01},
			want: false,
		},
		{
			name: "two bytes only - CRC of empty data is 0x0000",
			data: []byte{0x00, 0x00},
			want: true,
		},
		{
			name: "single byte with valid CRC appended",
			// CalcCRC16([]byte{0x01}) = 0x1021
			data: []byte{0x01, 0x10, 0x21},
			want: true,
		},
		{
			name: "single byte with invalid CRC appended",
			data: []byte{0x01, 0x00, 0x00},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifyCRC16(tt.data)
			if got != tt.want {
				t.Errorf("VerifyCRC16(%#v) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}
