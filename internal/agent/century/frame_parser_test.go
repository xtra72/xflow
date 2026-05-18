package century

import (
	"bytes"
	"errors"
	"testing"
)

// TestParseFrame_CAP3RegResponses exercises Group A scenarios A1 (reg 0x02
// read response decoding) and the corresponding reg 0x03 / reg 0x04 frames.
// Every CAP-3 byte from spec 부록 A must round-trip through ParseFrame and
// expose the expected addressing, function-code, payload length, and register.
//
// (REQ-CENTURY-005, REQ-CENTURY-011, AC-A1)
func TestParseFrame_CAP3RegResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		fixture          string
		wantSrc          uint16
		wantDst          uint16
		wantFC           byte
		wantPayloadLen   uint16
		wantRegister     byte
		wantRegisterOK   bool
		wantPayloadFirst byte // first payload byte (sub_dev_id, expected 0x3B in CAP-3)
	}{
		{
			name:             "reg 0x02 read response",
			fixture:          "cap3_reg02_response.bin",
			wantSrc:          AddrSlave,
			wantDst:          AddrMaster,
			wantFC:           FCResponse,
			wantPayloadLen:   20,
			wantRegister:     0x02,
			wantRegisterOK:   true,
			wantPayloadFirst: 0x3B,
		},
		{
			name:             "reg 0x03 read response",
			fixture:          "cap3_reg03_response.bin",
			wantSrc:          AddrSlave,
			wantDst:          AddrMaster,
			wantFC:           FCResponse,
			wantPayloadLen:   19,
			wantRegister:     0x03,
			wantRegisterOK:   true,
			wantPayloadFirst: 0x3B,
		},
		{
			name:             "reg 0x04 read response",
			fixture:          "cap3_reg04_response.bin",
			wantSrc:          AddrSlave,
			wantDst:          AddrMaster,
			wantFC:           FCResponse,
			wantPayloadLen:   17,
			wantRegister:     0x04,
			wantRegisterOK:   true,
			wantPayloadFirst: 0x3B,
		},
		{
			name:             "reg 0x04 write request",
			fixture:          "cap3_write_reg04.bin",
			wantSrc:          AddrMaster,
			wantDst:          AddrSlave,
			wantFC:           FCWrite,
			wantPayloadLen:   19,
			wantRegister:     0x04,
			wantRegisterOK:   true,
			wantPayloadFirst: 0x3B,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := mustReadFixture(t, tc.fixture)
			f, err := ParseFrame(raw)
			if err != nil {
				t.Fatalf("ParseFrame(%s) returned %v", tc.fixture, err)
			}
			if f.Src != tc.wantSrc {
				t.Errorf("Src = 0x%04X, want 0x%04X", f.Src, tc.wantSrc)
			}
			if f.Dst != tc.wantDst {
				t.Errorf("Dst = 0x%04X, want 0x%04X", f.Dst, tc.wantDst)
			}
			if f.FunctionCode != tc.wantFC {
				t.Errorf("FunctionCode = 0x%02X, want 0x%02X", f.FunctionCode, tc.wantFC)
			}
			if f.PayloadLength != tc.wantPayloadLen {
				t.Errorf("PayloadLength = %d, want %d", f.PayloadLength, tc.wantPayloadLen)
			}
			if int(f.PayloadLength) != len(f.Payload) {
				t.Errorf("len(Payload) = %d, want %d (matches PayloadLength)", len(f.Payload), f.PayloadLength)
			}
			if f.Reserved != 0x00 {
				t.Errorf("Reserved = 0x%02X, want 0x00", f.Reserved)
			}
			if f.Payload[0] != tc.wantPayloadFirst {
				t.Errorf("Payload[0] (sub_dev_id) = 0x%02X, want 0x%02X", f.Payload[0], tc.wantPayloadFirst)
			}
			reg, ok := f.Register()
			if ok != tc.wantRegisterOK || reg != tc.wantRegister {
				t.Errorf("Register() = (0x%02X, %v), want (0x%02X, %v)",
					reg, ok, tc.wantRegister, tc.wantRegisterOK)
			}
			if !f.ValidateCRC(raw) {
				t.Errorf("ValidateCRC returned false for a known-good CAP-3 fixture")
			}
		})
	}
}

// TestParseFrame_ACK covers Group A scenario A5: the slave's 1-byte ACK
// frame parses without entering the payload-prefix validation branch.
//
// (REQ-CENTURY-010, AC-A5)
func TestParseFrame_ACK(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap3_ack.bin")
	f, err := ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame(ack) returned %v", err)
	}
	if f.FunctionCode != FCResponse {
		t.Errorf("FunctionCode = 0x%02X, want 0x%02X", f.FunctionCode, FCResponse)
	}
	if f.PayloadLength != 1 {
		t.Errorf("PayloadLength = %d, want 1", f.PayloadLength)
	}
	if len(f.Payload) != 1 || f.Payload[0] != 0x00 {
		t.Errorf("Payload = %x, want [0x00]", f.Payload)
	}
	if !f.IsACK() {
		t.Errorf("IsACK() = false, want true")
	}
	// ACK has no payload prefix → Register() must report (0, false).
	if reg, ok := f.Register(); ok || reg != 0 {
		t.Errorf("Register() on ACK = (0x%02X, %v), want (0x00, false)", reg, ok)
	}
}

// TestParseFrame_CRCMismatch implements Group A scenario A6: a 1-bit
// corruption of the CRC trailer must produce ErrInvalidCRC.
//
// (REQ-CENTURY-004, AC-A6)
func TestParseFrame_CRCMismatch(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap3_reg02_response.bin")
	corrupted := bytes.Clone(raw)
	corrupted[len(corrupted)-1] ^= 0x01

	_, err := ParseFrame(corrupted)
	if !errors.Is(err, ErrInvalidCRC) {
		t.Fatalf("ParseFrame(corrupted) error = %v, want ErrInvalidCRC", err)
	}
}

// TestParseFrame_NegativeCases covers the boundary conditions called out by
// the M1 plan: empty bytes, header-only, payload_length larger than the
// buffer, reserved byte != 0x00, function code unknown, and payload
// exceeding MaxPayloadLength.
//
// (REQ-CENTURY-003, REQ-CENTURY-005, REQ-CENTURY-011, AC-A8, AC-A9, AC-A10)
func TestParseFrame_NegativeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty bytes returns ErrInvalidLength", func(t *testing.T) {
		t.Parallel()
		if _, err := ParseFrame(nil); !errors.Is(err, ErrInvalidLength) {
			t.Fatalf("ParseFrame(nil) error = %v, want ErrInvalidLength", err)
		}
	})

	t.Run("shorter than MinFrameLength returns ErrInvalidLength", func(t *testing.T) {
		t.Parallel()
		short := []byte{0x01, 0x00, 0x30, 0x00, 0x01, 0x00, 0x00, 0x06}
		if _, err := ParseFrame(short); !errors.Is(err, ErrInvalidLength) {
			t.Fatalf("ParseFrame(short) error = %v, want ErrInvalidLength", err)
		}
	})

	t.Run("payload_length larger than buffer returns ErrInvalidLength", func(t *testing.T) {
		t.Parallel()
		// Claim payload_length = 100 but only provide a few bytes of payload.
		buf := []byte{
			0x01, 0x00, // src
			0x30, 0x00, // dst
			0x64, 0x00, // p_len = 100
			0x00,       // reserved
			0x06,       // fc
			0x3B, 0x00, // partial payload
			0x00, 0x00, // CRC placeholder
		}
		if _, err := ParseFrame(buf); !errors.Is(err, ErrInvalidLength) {
			t.Fatalf("error = %v, want ErrInvalidLength", err)
		}
	})

	t.Run("reserved byte != 0x00 returns ErrInvalidHeader", func(t *testing.T) {
		t.Parallel()
		// REQ-CENTURY-011 validates in order: length → CRC → header → prefix.
		// Corrupting reserved alone breaks CRC first, so we must recompute the
		// CRC trailer to exercise the header-validation branch in isolation.
		raw := bytes.Clone(mustReadFixture(t, "cap3_reg02_response.bin"))
		raw[6] = 0xFF // corrupt reserved
		recomputeCRC(raw)
		_, err := ParseFrame(raw)
		if !errors.Is(err, ErrInvalidHeader) {
			t.Fatalf("error = %v, want ErrInvalidHeader", err)
		}
	})

	t.Run("unknown function_code returns ErrInvalidHeader", func(t *testing.T) {
		t.Parallel()
		raw := bytes.Clone(mustReadFixture(t, "cap3_reg02_response.bin"))
		raw[7] = 0x77 // arbitrary unknown fc
		recomputeCRC(raw)
		_, err := ParseFrame(raw)
		if !errors.Is(err, ErrInvalidHeader) {
			t.Fatalf("error = %v, want ErrInvalidHeader", err)
		}
	})

	t.Run("payload_length exceeds MaxPayloadLength returns ErrPayloadTooLarge", func(t *testing.T) {
		t.Parallel()
		// Construct a header with payload_length = 0xFFFF.
		buf := make([]byte, HeaderLength+1+CRCLength)
		buf[0] = 0x01
		buf[1] = 0x00
		buf[2] = 0x30
		buf[3] = 0x00
		buf[4] = 0xFF // p_len low
		buf[5] = 0xFF // p_len high
		buf[6] = 0x00
		buf[7] = FCResponse
		_, err := ParseFrame(buf)
		if !errors.Is(err, ErrPayloadTooLarge) {
			t.Fatalf("error = %v, want ErrPayloadTooLarge", err)
		}
	})

	t.Run("payload_length zero returns ErrInvalidLength", func(t *testing.T) {
		t.Parallel()
		// SPEC REQ-CENTURY-003 requires 0 < N. A frame claiming payload_length=0
		// has no payload prefix and no register byte, so reject it as invalid.
		buf := make([]byte, HeaderLength+CRCLength)
		buf[0] = 0x01
		buf[1] = 0x00
		buf[2] = 0x30
		buf[3] = 0x00
		buf[4] = 0x00 // p_len = 0
		buf[5] = 0x00
		buf[6] = 0x00
		buf[7] = FCResponse
		_, err := ParseFrame(buf)
		if !errors.Is(err, ErrInvalidLength) {
			t.Fatalf("error = %v, want ErrInvalidLength", err)
		}
	})

	t.Run("non-ACK frame with payload_prefix reserved2 != 0x00 returns ErrInvalidPayloadPrefix", func(t *testing.T) {
		t.Parallel()
		raw := bytes.Clone(mustReadFixture(t, "cap3_reg02_response.bin"))
		// payload offset 1 (after header) is reserved2; corrupt it.
		raw[HeaderLength+1] = 0xAA
		recomputeCRC(raw) // ensure we exercise prefix branch, not CRC
		_, err := ParseFrame(raw)
		if !errors.Is(err, ErrInvalidPayloadPrefix) {
			t.Fatalf("error = %v, want ErrInvalidPayloadPrefix", err)
		}
	})

	t.Run("non-ACK frame with unknown register returns ErrInvalidPayloadPrefix", func(t *testing.T) {
		t.Parallel()
		raw := bytes.Clone(mustReadFixture(t, "cap3_reg02_response.bin"))
		raw[HeaderLength+2] = 0x05 // register = 0x05 (not in {0x02, 0x03, 0x04})
		recomputeCRC(raw)
		_, err := ParseFrame(raw)
		if !errors.Is(err, ErrInvalidPayloadPrefix) {
			t.Fatalf("error = %v, want ErrInvalidPayloadPrefix", err)
		}
	})
}

// recomputeCRC overwrites the LE CRC trailer (last 2 bytes) so that the body
// is internally consistent. Used by negative tests that mutate header /
// payload bytes and want to exercise downstream validation stages without
// being blocked by the CRC check earlier in the pipeline.
func recomputeCRC(raw []byte) {
	if len(raw) < CRCLength {
		return
	}
	body := raw[:len(raw)-CRCLength]
	crc := CRC16ARC(body)
	raw[len(raw)-2] = byte(crc & 0xFF)
	raw[len(raw)-1] = byte(crc >> 8)
}

// TestParseFrame_PayloadIsCopy ensures ParseFrame returns a defensive copy of
// the payload bytes so the caller can safely mutate / reuse the input buffer
// (the frame scanner relies on this property).
func TestParseFrame_PayloadIsCopy(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap3_reg02_response.bin")
	src := bytes.Clone(raw)
	f, err := ParseFrame(src)
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}
	// Mutate the source buffer; the Frame should not change.
	for i := range src {
		src[i] = 0xFF
	}
	if f.Payload[0] != 0x3B {
		t.Fatalf("Frame.Payload was aliased to input buffer (got Payload[0]=0x%02X, want 0x3B)", f.Payload[0])
	}
}
