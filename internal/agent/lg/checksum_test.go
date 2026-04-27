package lg

import (
	"testing"
)

func TestCalcLGAPChecksum(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want byte
	}{
		{
			name: "empty data returns 0x55 (0 XOR 0x55)",
			data: []byte{},
			want: 0x55,
		},
		{
			name: "single byte 0x00",
			data: []byte{0x00},
			want: 0x55,
		},
		{
			name: "single byte 0x55 returns 0x00",
			data: []byte{0x55},
			want: 0x00, // 0x55 XOR 0x55 = 0x00
		},
		{
			name: "single byte 0xFF",
			data: []byte{0xFF},
			want: 0xFF ^ 0x55,
		},
		{
			name: "real status query without checksum",
			// HeaderByte=0x10, CommandByte=0x00, CommandID=0xA0, zone=0x10, flags=0x00, combo=0x00, temp=0x00
			data: []byte{0x10, 0x00, 0xA0, 0x10, 0x00, 0x00, 0x00},
			want: func() byte {
				sum := byte(0x10 + 0x00 + 0xA0 + 0x10 + 0x00 + 0x00 + 0x00)
				return sum ^ 0x55
			}(),
		},
		{
			name: "two ascending bytes",
			data: []byte{0x01, 0x02},
			want: (0x01 + 0x02) ^ 0x55,
		},
		{
			name: "overflow wraps to byte",
			// sum overflows: 0x80 + 0x80 = 0x100, truncated to 0x00
			data: []byte{0x80, 0x80},
			want: 0x00 ^ 0x55,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalcLGAPChecksum(tt.data)
			if got != tt.want {
				t.Errorf("CalcLGAPChecksum(%#v) = 0x%02X, want 0x%02X", tt.data, got, tt.want)
			}
		})
	}
}

func TestVerifyChecksum(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "valid checksum for single byte",
			// CalcLGAPChecksum([]byte{0x01}) = 0x01 ^ 0x55 = 0x54
			data: []byte{0x01, 0x54},
			want: true,
		},
		{
			name: "invalid checksum",
			data: []byte{0x01, 0xFF},
			want: false,
		},
		{
			name: "data less than 2 bytes - empty",
			data: []byte{},
			want: false,
		},
		{
			name: "data less than 2 bytes - one byte",
			data: []byte{0x42},
			want: false,
		},
		{
			name: "valid checksum for empty payload",
			// CalcLGAPChecksum([]byte{}) = 0x55
			data: []byte{0x55},
			// len < 2, should be false
			want: false,
		},
		{
			name: "valid checksum for status query packet",
			data: func() []byte {
				pkt := []byte{0x10, 0x00, 0xA0, 0x10, 0x00, 0x00, 0x00}
				cs := CalcLGAPChecksum(pkt)
				return append(pkt, cs)
			}(),
			want: true,
		},
		{
			name: "corrupted status query packet",
			data: func() []byte {
				pkt := []byte{0x10, 0x00, 0xA0, 0x10, 0x00, 0x00, 0x00}
				cs := CalcLGAPChecksum(pkt)
				return append(pkt, cs^0xFF) // flip all bits
			}(),
			want: false,
		},
		{
			name: "two bytes - zero payload with correct checksum",
			// CalcLGAPChecksum([]byte{0x00}) = 0x00 ^ 0x55 = 0x55
			data: []byte{0x00, 0x55},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifyChecksum(tt.data)
			if got != tt.want {
				t.Errorf("VerifyChecksum(%#v) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

func TestChecksumRoundTrip(t *testing.T) {
	// Round-trip: append CalcLGAPChecksum result, then VerifyChecksum should return true.
	payloads := [][]byte{
		{},
		{0x00},
		{0x10, 0x00, 0xA0, 0x10, 0x00, 0x00, 0x00},
		{0xFF, 0xFF, 0xFF},
		{0x10, 0x00, 0xA0, 0x20, 0x03, 0x62, 0x0A},
	}

	for _, payload := range payloads {
		cs := CalcLGAPChecksum(payload)
		data := make([]byte, len(payload)+1)
		copy(data, payload)
		data[len(data)-1] = cs

		if len(data) < 2 {
			// Payloads that result in < 2 total bytes cannot be verified
			continue
		}
		if !VerifyChecksum(data) {
			t.Errorf("round-trip failed for payload %#v: checksum=0x%02X, data=%#v", payload, cs, data)
		}
	}
}
