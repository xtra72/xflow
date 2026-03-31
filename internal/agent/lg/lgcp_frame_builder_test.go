package lg

import (
	"testing"
)

func TestLGCPFrameBuilder_Build_BasicStructure(t *testing.T) {
	b := NewLGCPFrameBuilder()
	da := []byte{0x44, 0x55, 0x00, 0x65}
	sa := []byte{0x44, 0x55, 0x00, 0x00}
	cmd := [2]byte{0x02, 0x01}
	payload := []byte{0x18, 0x41, 0x18, 0x80, 0x29, 0xC0}

	frame := b.Build(da, sa, cmd, 0x00, payload, 0x00)

	// STX
	if frame[0] != 0x56 {
		t.Errorf("STX: want 0x56, got 0x%02X", frame[0])
	}
	// LEN = frameLen - 1 (STX 제외) = 25 - 1 = 24
	if frame[1] != 24 {
		t.Errorf("LEN: want 24, got %d", frame[1])
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
	if !VerifyLGCPCRC(frame) {
		t.Errorf("CRC verification failed")
	}
}

func TestLGCPFrameBuilder_Build_CRCCrossValidation(t *testing.T) {
	b := NewLGCPFrameBuilder()
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
		if !VerifyLGCPCRC(frame) {
			t.Errorf("payload %d: CRC verification failed", i)
		}
	}
}

func TestLGCPFrameBuilder_BuildControl(t *testing.T) {
	b := NewLGCPFrameBuilder()
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
	if !VerifyLGCPCRC(frame) {
		t.Errorf("CRC verification failed")
	}
}

func TestLGCPFrameBuilder_Build_EmptyPayload(t *testing.T) {
	b := NewLGCPFrameBuilder()
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
	if !VerifyLGCPCRC(frame) {
		t.Errorf("CRC verification failed")
	}
}

func TestLGCPFrameBuilder_Build_LargePayload(t *testing.T) {
	b := NewLGCPFrameBuilder()
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
	if !VerifyLGCPCRC(frame) {
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
