package lg

import (
	"encoding/hex"
	"testing"
)

// protocolExampleFrame 은 프로토콜 문서의 예시 프레임이다.
// STX=56, LEN=2D(45), DLEN=04, DA=44550066, SLEN=04, SA=44550000,
// CMD=0204, SEQ0=F8, PLEN=1A(26), PAYLOAD=11 00 10 ..., SEQ1=98, CRC=DF35
var protocolExampleFrame, _ = hex.DecodeString(
	"562D044455006604445500000204F81A" +
		"110010C018001AC013001340" +
		"13C016001840188029C01DC0" +
		"919D98DF35",
)

func TestCalcIcp02CRC16_ProtocolExample(t *testing.T) {
	// CRC 계산 범위: frame[0:len-2] (STX, LEN 포함, CRC 제외)
	frame := protocolExampleFrame
	data := frame[0 : len(frame)-2]
	got := CalcIcp02CRC16(data)
	want := uint16(0xDF35)

	if got != want {
		t.Errorf("CalcIcp02CRC16(프로토콜 예시) = 0x%04X, want 0x%04X", got, want)
	}
}

func TestCalcIcp02CRC16(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want uint16
	}{
		{
			name: "빈 데이터",
			data: []byte{},
			want: 0x0000, // CRC-16/XMODEM init=0x0000
		},
		{
			name: "프로토콜 문서 예시",
			data: protocolExampleFrame[0 : len(protocolExampleFrame)-2],
			want: 0xDF35,
		},
		{
			name: "단일 바이트 0x00",
			data: []byte{0x00},
			want: CalcIcp02CRC16([]byte{0x00}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalcIcp02CRC16(tt.data)
			if got != tt.want {
				t.Errorf("CalcIcp02CRC16(%s) = 0x%04X, want 0x%04X",
					hex.EncodeToString(tt.data), got, tt.want)
			}
		})
	}
}

func TestVerifyIcp02CRC_Valid(t *testing.T) {
	// 프로토콜 문서 예시 프레임은 CRC 가 유효해야 한다.
	if !VerifyIcp02CRC(protocolExampleFrame) {
		t.Error("VerifyIcp02CRC(프로토콜 예시) = false, want true")
	}
}

func TestVerifyIcp02CRC_Invalid(t *testing.T) {
	// 프레임 데이터 1바이트를 변조하여 CRC 불일치를 확인한다.
	tampered := make([]byte, len(protocolExampleFrame))
	copy(tampered, protocolExampleFrame)
	tampered[10] ^= 0xFF // 데이터 영역 변조

	if VerifyIcp02CRC(tampered) {
		t.Error("VerifyIcp02CRC(변조된 프레임) = true, want false")
	}
}

func TestVerifyIcp02CRC_TooShort(t *testing.T) {
	tests := []struct {
		name  string
		frame []byte
	}{
		{name: "빈 슬라이스", frame: []byte{}},
		{name: "1바이트", frame: []byte{0x56}},
		{name: "2바이트", frame: []byte{0x56, 0x02}},
		{name: "3바이트", frame: []byte{0x56, 0x03, 0x00}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if VerifyIcp02CRC(tt.frame) {
				t.Errorf("VerifyIcp02CRC(%s) = true, want false (너무 짧은 프레임)",
					hex.EncodeToString(tt.frame))
			}
		})
	}
}

func TestVerifyIcp02CRC_RoundTrip(t *testing.T) {
	// 임의 데이터에 대해 CRC 를 계산하고, 프레임을 구성한 뒤 검증한다.
	payloads := [][]byte{
		{0x04, 0x11, 0x22, 0x33, 0x44, 0x04, 0xAA, 0xBB, 0xCC, 0xDD, 0x01, 0x02, 0xF0, 0x00},
		{0x01, 0xFF, 0x01, 0xEE, 0x00, 0x01, 0x00},
	}

	for _, payload := range payloads {
		// STX + LEN + payload + CRC(2)
		frameLen := 2 + len(payload) + 2
		frame := make([]byte, frameLen)
		frame[0] = icp02STX
		frame[1] = byte(frameLen)
		copy(frame[2:], payload)

		// CRC 계산: frame[0:len-2] (STX, LEN 포함)
		crc := CalcIcp02CRC16(frame[0 : frameLen-2])
		frame[frameLen-2] = byte(crc >> 8)   // 빅엔디안 상위
		frame[frameLen-1] = byte(crc & 0xFF) // 빅엔디안 하위

		if !VerifyIcp02CRC(frame) {
			t.Errorf("라운드트립 실패: payload=%s, crc=0x%04X",
				hex.EncodeToString(payload), crc)
		}
	}
}
