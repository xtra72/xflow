package century

import (
	"errors"
	"testing"
)

// TestDecodeReg04Read_CAP3 covers AC-A3 (cooling-start capture) for the reg
// 0x04 read response (REQ-CENTURY-008). Spec §6.3 / 부록 A reference values.
func TestDecodeReg04Read_CAP3(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap3_reg04_response.bin")
	f, err := ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}
	dec, err := DecodeReg04Read(f, 0, DirectionSlaveToMaster)
	if err != nil {
		t.Fatalf("DecodeReg04Read: %v", err)
	}

	if dec.StatusBits.Value != 0x3B || dec.StatusBits.ConfirmationStatus != Inferred {
		t.Errorf("StatusBits = %+v, want value=0x3B status=inferred", dec.StatusBits)
	}
	if dec.Reg04Const1.Value != 0xF6 {
		t.Errorf("Reg04Const1 = 0x%02X, want 0xF6", dec.Reg04Const1.Value)
	}
	if dec.Reg04Const2.Value != 0x09 {
		t.Errorf("Reg04Const2 = 0x%02X, want 0x09", dec.Reg04Const2.Value)
	}
	if dec.Reg04Const7.Value != 0x2C {
		t.Errorf("Reg04Const7 = 0x%02X, want 0x2C", dec.Reg04Const7.Value)
	}
	if dec.OpVal1.Value != 0 || dec.OpVal1.ConfirmationStatus != Inferred {
		t.Errorf("OpVal1 = %+v, want value=0 status=inferred", dec.OpVal1)
	}
	if dec.TempAC.Value != 25.2 || dec.TempAC.Raw != 252 {
		t.Errorf("TempAC = %+v, want value=25.2 raw=252", dec.TempAC)
	}
	if dec.OpVal2.Value != 252 || dec.OpVal2.ConfirmationStatus != Inferred {
		t.Errorf("OpVal2 = %+v, want value=252 status=inferred", dec.OpVal2)
	}

	// data[3..6] are all zero in every capture → Unknown.
	for _, p := range []FieldU8{dec.Reg04Byte3, dec.Reg04Byte4, dec.Reg04Byte5, dec.Reg04Byte6} {
		if p.Value != 0 || p.ConfirmationStatus != Unknown {
			t.Errorf("zero byte = %+v, want value=0 status=unknown", p)
		}
	}
}

// TestDecodeReg04Read_CAP4 covers AC-A3 (steady-state cooling) which is the
// distinguishing case for op_val_1=996 and op_val_2=1248 (spec §6.3).
func TestDecodeReg04Read_CAP4(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap4_reg04_response.bin")
	f, err := ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}
	dec, err := DecodeReg04Read(f, 0, DirectionSlaveToMaster)
	if err != nil {
		t.Fatalf("DecodeReg04Read: %v", err)
	}

	if dec.StatusBits.Value != 0x39 {
		t.Errorf("StatusBits = 0x%02X, want 0x39", dec.StatusBits.Value)
	}
	if dec.OpVal1.Value != 996 {
		t.Errorf("OpVal1 = %d, want 996", dec.OpVal1.Value)
	}
	if dec.TempAC.Value != 25.2 {
		t.Errorf("TempAC = %v, want 25.2", dec.TempAC.Value)
	}
	if dec.OpVal2.Value != 1248 {
		t.Errorf("OpVal2 = %d, want 1248", dec.OpVal2.Value)
	}
}

// TestDecodeReg04Read_CAP1 covers the off state where op fields are all zero.
func TestDecodeReg04Read_CAP1(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap1_reg04_response.bin")
	f, err := ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}
	dec, err := DecodeReg04Read(f, 0, DirectionSlaveToMaster)
	if err != nil {
		t.Fatalf("DecodeReg04Read: %v", err)
	}
	if dec.StatusBits.Value != 0x63 {
		t.Errorf("StatusBits = 0x%02X, want 0x63 (off-state)", dec.StatusBits.Value)
	}
	if dec.OpVal1.Value != 0 || dec.OpVal2.Value != 0 || dec.TempAC.Value != 0 {
		t.Errorf("op fields = %d/%v/%d, want all zero",
			dec.OpVal1.Value, dec.TempAC.Value, dec.OpVal2.Value)
	}
}

// TestDecodeReg04Read_WrongRegister rejects non-0x04 frames.
func TestDecodeReg04Read_WrongRegister(t *testing.T) {
	t.Parallel()
	raw := mustReadFixture(t, "cap3_reg02_response.bin")
	f, _ := ParseFrame(raw)
	if _, err := DecodeReg04Read(f, 0, DirectionSlaveToMaster); !errors.Is(err, ErrInvalidRegister) {
		t.Fatalf("error = %v, want ErrInvalidRegister", err)
	}
}

// TestDecodeReg04Read_WrongDataLength rejects frames whose data is not 14B.
func TestDecodeReg04Read_WrongDataLength(t *testing.T) {
	t.Parallel()
	// Reg 0x04 WRITE has 16B data → DecodeReg04Read should refuse it.
	raw := mustReadFixture(t, "cap3_write_reg04.bin")
	f, _ := ParseFrame(raw)
	if _, err := DecodeReg04Read(f, 0, DirectionMasterToSlave); !errors.Is(err, ErrInvalidPayloadLength) {
		t.Fatalf("error = %v, want ErrInvalidPayloadLength", err)
	}
}

// TestDecodeReg04Write_CAP3 covers AC-A4 (write mode_cmd = cooling), and most
// importantly verifies that mode_cmd is exposed as Confirmed while the live
// bytes are Inferred (REQ-CENTURY-009).
func TestDecodeReg04Write_CAP3(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap3_write_reg04.bin")
	f, err := ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}
	dec, err := DecodeReg04Write(f, 0, DirectionMasterToSlave)
	if err != nil {
		t.Fatalf("DecodeReg04Write: %v", err)
	}

	if dec.ObservationMode != "passive" {
		t.Errorf("ObservationMode = %q, want passive", dec.ObservationMode)
	}
	if dec.ModeCmd.Value != "cool" || dec.ModeCmd.Raw != 0x01 ||
		dec.ModeCmd.ConfirmationStatus != Confirmed {
		t.Errorf("ModeCmd = %+v, want value=cooling raw=1 status=confirmed", dec.ModeCmd)
	}
	if dec.WriteLive0.Value != 0x00 || dec.WriteLive0.ConfirmationStatus != Inferred {
		t.Errorf("WriteLive0 = %+v, want value=0 status=inferred", dec.WriteLive0)
	}
	if dec.WriteLive1.Value != 0x00 {
		t.Errorf("WriteLive1 = %+v, want 0", dec.WriteLive1)
	}
	if dec.WriteByte14.Value != 0xC7 {
		t.Errorf("WriteByte14 = 0x%02X, want 0xC7", dec.WriteByte14.Value)
	}
	if dec.WriteLive15.Value != 0x00 {
		t.Errorf("WriteLive15 = 0x%02X, want 0x00 (CAP-3)", dec.WriteLive15.Value)
	}
	if dec.Direction != DirectionMasterToSlave {
		t.Errorf("Direction = %q, want %q", dec.Direction, DirectionMasterToSlave)
	}
}

// TestDecodeReg04Write_CAP4 verifies the steady-state distinguishing values
// (write_live_0=0x02, write_live_1=0x04, write_byte_14=0xC0, write_live_15∈[0x0F..0x11]).
func TestDecodeReg04Write_CAP4(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap4_write_reg04.bin")
	f, _ := ParseFrame(raw)
	dec, err := DecodeReg04Write(f, 0, DirectionMasterToSlave)
	if err != nil {
		t.Fatalf("DecodeReg04Write: %v", err)
	}
	if dec.WriteLive0.Value != 0x02 {
		t.Errorf("WriteLive0 = 0x%02X, want 0x02", dec.WriteLive0.Value)
	}
	if dec.WriteLive1.Value != 0x04 {
		t.Errorf("WriteLive1 = 0x%02X, want 0x04", dec.WriteLive1.Value)
	}
	if dec.ModeCmd.Value != "cool" {
		t.Errorf("ModeCmd = %q, want cool", dec.ModeCmd.Value)
	}
	if dec.WriteByte14.Value != 0xC0 {
		t.Errorf("WriteByte14 = 0x%02X, want 0xC0", dec.WriteByte14.Value)
	}
	if dec.WriteLive15.Value < 0x0F || dec.WriteLive15.Value > 0x11 {
		t.Errorf("WriteLive15 = 0x%02X, want in [0x0F..0x11]", dec.WriteLive15.Value)
	}
}

// TestDecodeReg04Write_CAP1 covers off-state writes where mode_cmd=off and
// every live byte is zero except write_byte_14=0xC4.
func TestDecodeReg04Write_CAP1(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap1_write_reg04.bin")
	f, _ := ParseFrame(raw)
	dec, err := DecodeReg04Write(f, 0, DirectionMasterToSlave)
	if err != nil {
		t.Fatalf("DecodeReg04Write: %v", err)
	}
	if dec.ModeCmd.Value != "off" || dec.ModeCmd.Raw != 0x00 {
		t.Errorf("ModeCmd = %+v, want off/0", dec.ModeCmd)
	}
	if dec.WriteByte14.Value != 0xC4 {
		t.Errorf("WriteByte14 = 0x%02X, want 0xC4", dec.WriteByte14.Value)
	}
}

// TestDecodeReg04Write_WrongRegister rejects non-0x04 frames.
func TestDecodeReg04Write_WrongRegister(t *testing.T) {
	t.Parallel()
	raw := mustReadFixture(t, "cap3_reg03_response.bin")
	f, _ := ParseFrame(raw)
	if _, err := DecodeReg04Write(f, 0, DirectionMasterToSlave); !errors.Is(err, ErrInvalidRegister) {
		t.Fatalf("error = %v, want ErrInvalidRegister", err)
	}
}

// TestDecodeReg04Write_WrongDataLength rejects frames whose data is not 16B.
func TestDecodeReg04Write_WrongDataLength(t *testing.T) {
	t.Parallel()
	// Reg 0x04 READ has 14B data → DecodeReg04Write should refuse it.
	raw := mustReadFixture(t, "cap3_reg04_response.bin")
	f, _ := ParseFrame(raw)
	if _, err := DecodeReg04Write(f, 0, DirectionSlaveToMaster); !errors.Is(err, ErrInvalidPayloadLength) {
		t.Fatalf("error = %v, want ErrInvalidPayloadLength", err)
	}
}

// TestDecodeReg04Write_AdditiveModeCmd checks the additive enum behavior on
// mode_cmd (REQ-CENTURY-021, AC-E2 mirrored for WRITE).
func TestDecodeReg04Write_AdditiveModeCmd(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap3_write_reg04.bin")
	mutated := make([]byte, len(raw))
	copy(mutated, raw)
	// payload offset = HeaderLength(8) + prefix(3) + 4 = byte 15 holds data[4] (mode_cmd).
	mutated[HeaderLength+3+4] = 0x07
	recomputeCRC(mutated)

	f, err := ParseFrame(mutated)
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}
	dec, err := DecodeReg04Write(f, 0, DirectionMasterToSlave)
	if err != nil {
		t.Fatalf("DecodeReg04Write: %v", err)
	}
	if dec.ModeCmd.Value != "mode_unknown_07" {
		t.Errorf("ModeCmd.Value = %q, want mode_unknown_07", dec.ModeCmd.Value)
	}
	if dec.ModeCmd.ConfirmationStatus != Unknown {
		t.Errorf("ModeCmd.status = %v, want unknown", dec.ModeCmd.ConfirmationStatus)
	}
}

// TestDecodeReg04Read_NilFrame and TestDecodeReg04Write_NilFrame guard against nil input.
func TestDecodeReg04Read_NilFrame(t *testing.T) {
	t.Parallel()
	if _, err := DecodeReg04Read(nil, 0, DirectionSlaveToMaster); !errors.Is(err, ErrNilFrame) {
		t.Fatalf("error = %v, want ErrNilFrame", err)
	}
}

func TestDecodeReg04Write_NilFrame(t *testing.T) {
	t.Parallel()
	if _, err := DecodeReg04Write(nil, 0, DirectionMasterToSlave); !errors.Is(err, ErrNilFrame) {
		t.Fatalf("error = %v, want ErrNilFrame", err)
	}
}
