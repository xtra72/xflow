package century

import (
	"encoding/binary"
	"testing"
)

// TestConstants pins the canonical addressing, function-code, and length
// constants from SPEC-CENTURY-001 §2 (frame format) and §4 (specifications).
//
// (REQ-CENTURY-003, REQ-CENTURY-004, REQ-CENTURY-005)
func TestConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  uint
		want uint
	}{
		{"AddrMaster", uint(AddrMaster), 0x0030},
		{"AddrSlave", uint(AddrSlave), 0x0001},
		{"FCResponse (ACK / Read Response)", uint(FCResponse), 0x06},
		{"FCRead", uint(FCRead), 0x0B},
		{"FCWrite", uint(FCWrite), 0x0C},
		{"MaxPayloadLength", uint(MaxPayloadLength), 256},
		// Header(8) + min payload(1, for ACK) + CRC(2) = 11.
		{"MinFrameLength", uint(MinFrameLength), 11},
		// Header is 8 bytes (src u16 + dst u16 + p_len u16 + reserved u8 + fc u8).
		{"HeaderLength", uint(HeaderLength), 8},
		{"CRCLength", uint(CRCLength), 2},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Fatalf("%s = %d (0x%X), want %d (0x%X)", tc.name, tc.got, tc.got, tc.want, tc.want)
			}
		})
	}
}

// TestFrameValidateCRC verifies that Frame.ValidateCRC recomputes CRC-16/ARC
// over raw[:len(raw)-2] and compares against the LE trailer.
// Positive cases use real CAP-3 fixtures; the negative case flips one bit
// of the CRC trailer.
//
// (REQ-CENTURY-004)
func TestFrameValidateCRC(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap3_reg02_response.bin")
	body := raw[:len(raw)-2]
	trailer := binary.LittleEndian.Uint16(raw[len(raw)-2:])

	f := &Frame{CRC: trailer}
	if !f.ValidateCRC(raw) {
		t.Fatalf("ValidateCRC on a known-good CAP-3 frame returned false (stored CRC=0x%04X, computed=0x%04X)",
			trailer, CRC16ARC(body))
	}

	corrupted := make([]byte, len(raw))
	copy(corrupted, raw)
	corrupted[len(corrupted)-1] ^= 0x01 // flip 1 bit of the high CRC byte
	corruptedTrailer := binary.LittleEndian.Uint16(corrupted[len(corrupted)-2:])
	fc := &Frame{CRC: corruptedTrailer}
	if fc.ValidateCRC(corrupted) {
		t.Fatalf("ValidateCRC must reject 1-bit-corrupted CRC trailer")
	}
}

// TestFrameRegister covers both branches of Frame.Register:
//   - ACK frames (payload length 1) → returns (0, false)
//   - Read/Write frames (payload prefix present) → returns payload[2]
//
// (REQ-CENTURY-005, REQ-CENTURY-010)
func TestFrameRegister(t *testing.T) {
	t.Parallel()

	t.Run("read response reg 0x02 returns 0x02", func(t *testing.T) {
		t.Parallel()
		f := &Frame{
			FunctionCode: FCResponse,
			Payload:      []byte{0x3B, 0x00, 0x02, 0xAA, 0xBB, 0xCC},
		}
		reg, ok := f.Register()
		if !ok || reg != 0x02 {
			t.Fatalf("Register() = (0x%X, %v), want (0x02, true)", reg, ok)
		}
	})

	t.Run("write request reg 0x04 returns 0x04", func(t *testing.T) {
		t.Parallel()
		f := &Frame{
			FunctionCode: FCWrite,
			Payload:      []byte{0x3B, 0x00, 0x04, 0xDE, 0xAD},
		}
		reg, ok := f.Register()
		if !ok || reg != 0x04 {
			t.Fatalf("Register() = (0x%X, %v), want (0x04, true)", reg, ok)
		}
	})

	t.Run("ACK frame (payload length 1) returns (0, false)", func(t *testing.T) {
		t.Parallel()
		f := &Frame{
			FunctionCode: FCResponse,
			Payload:      []byte{0x00},
		}
		reg, ok := f.Register()
		if ok || reg != 0 {
			t.Fatalf("Register() on ACK = (0x%X, %v), want (0x00, false)", reg, ok)
		}
	})

	t.Run("frame with too-short payload (<3) returns (0, false)", func(t *testing.T) {
		t.Parallel()
		f := &Frame{
			FunctionCode: FCResponse,
			Payload:      []byte{0x3B, 0x00},
		}
		reg, ok := f.Register()
		if ok || reg != 0 {
			t.Fatalf("Register() = (0x%X, %v), want (0x00, false) for 2-byte payload", reg, ok)
		}
	})
}

// TestFrameData verifies the Data() accessor returns payload[3:] for prefixed
// frames and nil for ACK / too-short payloads.
//
// (REQ-CENTURY-005)
func TestFrameData(t *testing.T) {
	t.Parallel()

	t.Run("returns payload[3:] when prefix present", func(t *testing.T) {
		t.Parallel()
		f := &Frame{
			FunctionCode: FCResponse,
			Payload:      []byte{0x3B, 0x00, 0x02, 0xDE, 0xAD, 0xBE, 0xEF},
		}
		got := f.Data()
		want := []byte{0xDE, 0xAD, 0xBE, 0xEF}
		if !equalBytes(got, want) {
			t.Fatalf("Data() = %x, want %x", got, want)
		}
	})

	t.Run("returns nil for ACK frame", func(t *testing.T) {
		t.Parallel()
		f := &Frame{
			FunctionCode: FCResponse,
			Payload:      []byte{0x00},
		}
		if got := f.Data(); got != nil {
			t.Fatalf("Data() on ACK = %x, want nil", got)
		}
	})

	t.Run("returns empty slice when payload prefix has no data", func(t *testing.T) {
		t.Parallel()
		f := &Frame{
			FunctionCode: FCResponse,
			Payload:      []byte{0x3B, 0x00, 0x02},
		}
		got := f.Data()
		if len(got) != 0 {
			t.Fatalf("Data() with no payload data = %x, want empty", got)
		}
	})
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
