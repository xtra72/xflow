package lg

import (
	"testing"
)

func TestIcp02FrameBuilder_Build_BasicStructure(t *testing.T) {
	b := NewIcp02FrameBuilder()
	da := []byte{0x44, 0x55, 0x00, 0x65}
	sa := []byte{0x44, 0x55, 0x00, 0x00}
	cmd := [2]byte{0x02, 0x01}
	payload := []byte{0x18, 0x41, 0x18, 0x80, 0x29, 0xC0}

	frame := b.Build(da, sa, cmd, 0x00, payload, 0x00)

	// STX
	if frame[0] != 0x56 {
		t.Errorf("STX: want 0x56, got 0x%02X", frame[0])
	}
	// LEN = 전체 프레임 길이 = 25
	if frame[1] != 25 {
		t.Errorf("LEN: want 25, got %d", frame[1])
	}
	// DLEN
	if frame[2] != 0x04 {
		t.Errorf("DLEN: want 0x04, got 0x%02X", frame[2])
	}
	// DA
	for i, want := range da {
		if frame[3+i] != want {
			t.Errorf("DA[%d]: want 0x%02X, got 0x%02X", i, want, frame[3+i])
		}
	}
	// SLEN
	if frame[7] != 0x04 {
		t.Errorf("SLEN: want 0x04, got 0x%02X", frame[7])
	}
	// SA
	for i, want := range sa {
		if frame[8+i] != want {
			t.Errorf("SA[%d]: want 0x%02X, got 0x%02X", i, want, frame[8+i])
		}
	}
	// CMD
	if frame[12] != 0x02 || frame[13] != 0x01 {
		t.Errorf("CMD: want 0x0201, got 0x%02X%02X", frame[12], frame[13])
	}
	// PLEN
	if frame[15] != 0x06 {
		t.Errorf("PLEN: want 0x06, got 0x%02X", frame[15])
	}
	// CRC 검증
	if !VerifyIcp02CRC(frame) {
		t.Errorf("CRC verification failed")
	}
}

func TestIcp02FrameBuilder_Build_CRCCrossValidation(t *testing.T) {
	b := NewIcp02FrameBuilder()
	da := []byte{0x44, 0x55, 0x00, 0x65}
	sa := []byte{0x44, 0x55, 0x00, 0x00}

	payloads := [][]byte{
		{0x18, 0x41, 0x18, 0x80, 0x29, 0xC0},
		{0x18, 0x40, 0x18, 0x80, 0x29, 0xC0},
		{0x64, 0x8A},
		{0x64, 0x50, 0x30},
		{},
	}

	for i, payload := range payloads {
		frame := b.Build(da, sa, [2]byte{0x02, 0x01}, 0x00, payload, 0x00)
		if !VerifyIcp02CRC(frame) {
			t.Errorf("payload %d: CRC verification failed", i)
		}
	}
}

func TestIcp02FrameBuilder_BuildControl(t *testing.T) {
	b := NewIcp02FrameBuilder()
	da := []byte{0x44, 0x55, 0x00, 0x65}
	sa := []byte{0x44, 0x55, 0x00, 0x00}
	payload := []byte{0x18, 0x41}

	frame := b.BuildControl(da, sa, 0x01, payload, 0x02)

	// CMD = 0x0201
	if frame[12] != 0x02 || frame[13] != 0x01 {
		t.Errorf("CMD: want 0x0201, got 0x%02X%02X", frame[12], frame[13])
	}
	// SEQ0
	if frame[14] != 0x01 {
		t.Errorf("SEQ0: want 0x01, got 0x%02X", frame[14])
	}
	if !VerifyIcp02CRC(frame) {
		t.Errorf("CRC verification failed")
	}
}

func TestIcp02FrameBuilder_Build_EmptyPayload(t *testing.T) {
	b := NewIcp02FrameBuilder()
	da := []byte{0x44, 0x55, 0x00, 0x65}
	sa := []byte{0x44, 0x55, 0x00, 0x00}

	frame := b.Build(da, sa, [2]byte{0x02, 0x01}, 0x00, nil, 0x00)

	// PLEN = 0
	if frame[15] != 0x00 {
		t.Errorf("PLEN: want 0x00, got 0x%02X", frame[15])
	}
	// 프레임 길이 = 19 + 0 = 19
	if len(frame) != 19 {
		t.Errorf("frame length: want 19, got %d", len(frame))
	}
	if !VerifyIcp02CRC(frame) {
		t.Errorf("CRC verification failed")
	}
}

func TestIcp02FrameBuilder_Build_LargePayload(t *testing.T) {
	b := NewIcp02FrameBuilder()
	da := []byte{0x44, 0x55, 0x00, 0x65}
	sa := []byte{0x44, 0x55, 0x00, 0x00}
	payload := make([]byte, 25)
	for i := range payload {
		payload[i] = byte(i)
	}

	frame := b.Build(da, sa, [2]byte{0x02, 0x01}, 0x00, payload, 0x00)

	if frame[15] != 25 {
		t.Errorf("PLEN: want 25, got %d", frame[15])
	}
	if len(frame) != 19+25 {
		t.Errorf("frame length: want %d, got %d", 19+25, len(frame))
	}
	if !VerifyIcp02CRC(frame) {
		t.Errorf("CRC verification failed")
	}
}

// TestIcp02FrameBuilder_Build_ProtocolExample 은 프로토콜 문서의 실제 프레임 예제와 대조한다.
// 예제: DA=44550066, SA=44550000, CMD=0204, SEQ0=F8, PLEN=1A(26), SEQ1=98, CRC=DF35
func TestIcp02FrameBuilder_Build_ProtocolExample(t *testing.T) {
	b := NewIcp02FrameBuilder()
	da := []byte{0x44, 0x55, 0x00, 0x66}
	sa := []byte{0x44, 0x55, 0x00, 0x00}
	cmd := [2]byte{0x02, 0x04}
	payload := []byte{
		0x11, 0x00, 0x10, 0xC0, 0x18, 0x00, 0x1A, 0xC0,
		0x13, 0x00, 0x13, 0x40, 0x13, 0xC0, 0x16, 0x00,
		0x18, 0x40, 0x18, 0x80, 0x29, 0xC0, 0x1D, 0xC0,
		0x91, 0x9D,
	}

	frame := b.Build(da, sa, cmd, 0xF8, payload, 0x98)

	// LEN = 0x2D (45) — 프로토콜 문서 기준
	if frame[1] != 0x2D {
		t.Errorf("LEN: want 0x2D (45), got 0x%02X (%d)", frame[1], frame[1])
	}
	// 총 프레임 길이 = 45
	if len(frame) != 45 {
		t.Errorf("frame length: want 45, got %d", len(frame))
	}
	// CRC = 0xDF35 (프로토콜 문서 기준)
	crcHi := frame[len(frame)-2]
	crcLo := frame[len(frame)-1]
	if crcHi != 0xDF || crcLo != 0x35 {
		t.Errorf("CRC: want DF35, got %02X%02X", crcHi, crcLo)
	}
	// VerifyIcp02CRC 교차 검증
	if !VerifyIcp02CRC(frame) {
		t.Errorf("CRC verification failed")
	}
}

func TestParseHexAddress(t *testing.T) {
	tests := []struct {
		input string
		want  []byte
		err   bool
	}{
		{"44550065", []byte{0x44, 0x55, 0x00, 0x65}, false},
		{"44550000", []byte{0x44, 0x55, 0x00, 0x00}, false},
		{"ffffffff", []byte{0xFF, 0xFF, 0xFF, 0xFF}, false},
		{"ZZZZ0000", nil, true},
		{"4455", nil, true},
		{"", nil, true},
	}

	for _, tt := range tests {
		got, err := ParseHexAddress(tt.input)
		if tt.err {
			if err == nil {
				t.Errorf("ParseHexAddress(%q): want error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseHexAddress(%q): unexpected error: %v", tt.input, err)
			continue
		}
		for i, b := range got {
			if b != tt.want[i] {
				t.Errorf("ParseHexAddress(%q)[%d]: want 0x%02X, got 0x%02X", tt.input, i, tt.want[i], b)
			}
		}
	}
}
