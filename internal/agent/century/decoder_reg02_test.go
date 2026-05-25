package century

import (
	"errors"
	"testing"
)

// TestDecodeReg02_CAP3Cooling verifies that the CAP-3 reg 0x02 response
// (refrigeration / 25.0°C / fan 17, ground-truth) decodes to the typed
// fields specified in REQ-CENTURY-006 / AC-A1 / spec §4.4 example 1.
func TestDecodeReg02_CAP3Cooling(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap3_reg02_response.bin")
	f, err := ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}

	const tsMs int64 = 1737216000123
	dec, err := DecodeReg02(f, tsMs, DirectionSlaveToMaster)
	if err != nil {
		t.Fatalf("DecodeReg02: %v", err)
	}

	// Wrapper fields
	if dec.SubDevID != 0x3B {
		t.Errorf("SubDevID = 0x%02X, want 0x3B", dec.SubDevID)
	}
	if dec.Register != 0x02 {
		t.Errorf("Register = 0x%02X, want 0x02", dec.Register)
	}
	if dec.TimestampMs != tsMs {
		t.Errorf("TimestampMs = %d, want %d", dec.TimestampMs, tsMs)
	}
	if dec.Direction != DirectionSlaveToMaster {
		t.Errorf("Direction = %q, want %q", dec.Direction, DirectionSlaveToMaster)
	}

	// Confirmed fields
	if dec.Mode.Value != "cool" || dec.Mode.Raw != 0x01 {
		t.Errorf("Mode = %+v, want value=cooling raw=1", dec.Mode)
	}
	if dec.Mode.ConfirmationStatus != Confirmed {
		t.Errorf("Mode.status = %v, want confirmed", dec.Mode.ConfirmationStatus)
	}
	if dec.Fan.Value != 17 || dec.Fan.ConfirmationStatus != Confirmed {
		t.Errorf("Fan = %+v, want value=17 status=confirmed", dec.Fan)
	}
	if dec.SetpointC.Value != 25.0 || dec.SetpointC.Raw != 250 ||
		dec.SetpointC.ConfirmationStatus != Confirmed {
		t.Errorf("SetpointC = %+v, want value=25.0 raw=250 status=confirmed", dec.SetpointC)
	}

	// Inferred fields
	if dec.Reg02Word11.Value != 25.0 || dec.Reg02Word11.Raw != 250 ||
		dec.Reg02Word11.ConfirmationStatus != Inferred {
		t.Errorf("Reg02Word11 = %+v, want value=25.0 raw=250 status=inferred", dec.Reg02Word11)
	}
	if dec.Reg02Live13.Value != 27 || dec.Reg02Live13.ConfirmationStatus != Inferred {
		t.Errorf("Reg02Live13 = %+v, want value=27 status=inferred", dec.Reg02Live13)
	}
	if dec.Reg02Live14.Value != 0x39 || dec.Reg02Live14.ConfirmationStatus != Inferred {
		t.Errorf("Reg02Live14 = %+v, want value=0x39 status=inferred", dec.Reg02Live14)
	}
	if dec.Reg02Live15.Value != 0x39 || dec.Reg02Live15.ConfirmationStatus != Inferred {
		t.Errorf("Reg02Live15 = %+v, want value=0x39 status=inferred", dec.Reg02Live15)
	}

	// Unknown (zero) bytes
	zeroes := []struct {
		name string
		f    FieldU8
	}{
		{"Reg02Byte0", dec.Reg02Byte0},
		{"Reg02Byte3", dec.Reg02Byte3},
		{"Reg02Byte4", dec.Reg02Byte4},
		{"Reg02Byte5", dec.Reg02Byte5},
		{"Reg02Byte6", dec.Reg02Byte6},
		{"Reg02Byte9", dec.Reg02Byte9},
		{"Reg02Byte10", dec.Reg02Byte10},
		{"Reg02Byte16", dec.Reg02Byte16},
	}
	for _, z := range zeroes {
		if z.f.Value != 0 || z.f.ConfirmationStatus != Unknown {
			t.Errorf("%s = %+v, want value=0 status=unknown", z.name, z.f)
		}
	}
}

// TestDecodeReg02_CAP1Off verifies that the off-state fixture decodes the
// confirmed fields with mode=off, fan=0, setpoint maintained at 25.0°C, and
// every live byte at 0 (REQ-CENTURY-006, AC-B1a / spec 부록 B).
func TestDecodeReg02_CAP1Off(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap1_reg02_response.bin")
	f, err := ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}

	dec, err := DecodeReg02(f, 0, DirectionSlaveToMaster)
	if err != nil {
		t.Fatalf("DecodeReg02: %v", err)
	}

	if dec.Mode.Value != "off" || dec.Mode.Raw != 0x00 ||
		dec.Mode.ConfirmationStatus != Confirmed {
		t.Errorf("Mode = %+v, want value=off raw=0 status=confirmed", dec.Mode)
	}
	if dec.Fan.Value != 0 || dec.Fan.ConfirmationStatus != Confirmed {
		t.Errorf("Fan = %+v, want value=0 status=confirmed", dec.Fan)
	}
	// Setpoint is preserved even when the unit is off.
	if dec.SetpointC.Value != 25.0 || dec.SetpointC.Raw != 250 {
		t.Errorf("SetpointC = %+v, want 25.0/250", dec.SetpointC)
	}
	// Live bytes are 0 in off state.
	if dec.Reg02Live13.Value != 0 ||
		dec.Reg02Live14.Value != 0 ||
		dec.Reg02Live15.Value != 0 {
		t.Errorf("live bytes = %d/%d/%d, want all zero",
			dec.Reg02Live13.Value, dec.Reg02Live14.Value, dec.Reg02Live15.Value)
	}
}

// TestDecodeReg02_CAP4SteadyCooling exercises the steady-state cooling
// capture; the most notable difference vs CAP-3 is reg02_live_14 alternating
// between 0x38 and 0x39 (spec 부록 B-1 비교 표).
func TestDecodeReg02_CAP4SteadyCooling(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap4_reg02_response.bin")
	f, err := ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}
	dec, err := DecodeReg02(f, 0, DirectionSlaveToMaster)
	if err != nil {
		t.Fatalf("DecodeReg02: %v", err)
	}

	if dec.Mode.Value != "cool" {
		t.Errorf("Mode = %q, want cool", dec.Mode.Value)
	}
	if dec.Fan.Value != 17 {
		t.Errorf("Fan = %d, want 17", dec.Fan.Value)
	}
	if dec.SetpointC.Value != 25.0 {
		t.Errorf("SetpointC = %v, want 25.0", dec.SetpointC.Value)
	}
	// CAP-4 live14 alternates between 0x38 and 0x39 — this fixture captures 0x38.
	if dec.Reg02Live14.Value != 0x38 && dec.Reg02Live14.Value != 0x39 {
		t.Errorf("Reg02Live14 = 0x%02X, want 0x38 or 0x39", dec.Reg02Live14.Value)
	}
}

// TestDecodeReg02_WrongRegister verifies that the decoder refuses a frame
// whose register byte is not 0x02 (REQ-CENTURY-006).
func TestDecodeReg02_WrongRegister(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap3_reg03_response.bin")
	f, err := ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}
	_, err = DecodeReg02(f, 0, DirectionSlaveToMaster)
	if !errors.Is(err, ErrInvalidRegister) {
		t.Fatalf("DecodeReg02 on reg 0x03 frame error = %v, want ErrInvalidRegister", err)
	}
}

// TestDecodeReg02_WrongDataLength asserts that a reg 0x02 frame whose data
// portion is not 17 bytes is rejected (REQ-CENTURY-011 stage 5 / AC-A10).
func TestDecodeReg02_WrongDataLength(t *testing.T) {
	t.Parallel()

	// Hand-craft a Frame with register 0x02 but only 16B data (short by 1).
	f := &Frame{
		Src:           AddrSlave,
		Dst:           AddrMaster,
		PayloadLength: 19, // prefix(3) + data(16)
		Reserved:      0x00,
		FunctionCode:  FCResponse,
		Payload:       append([]byte{0x3B, 0x00, 0x02}, make([]byte, 16)...),
	}
	_, err := DecodeReg02(f, 0, DirectionSlaveToMaster)
	if !errors.Is(err, ErrInvalidPayloadLength) {
		t.Fatalf("error = %v, want ErrInvalidPayloadLength", err)
	}
}

// TestDecodeReg02_AdditiveModeEnum covers AC-E2: a synthetic frame with
// mode=0x02 must decode without error, emit mode_unknown_02, and tag the
// field as Unknown (additive enum policy per REQ-CENTURY-021).
func TestDecodeReg02_AdditiveModeEnum(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap3_reg02_response.bin")
	// payload offset = HeaderLength(8) + prefix(3) + 1 = byte 12 holds data[1] (mode).
	mutated := make([]byte, len(raw))
	copy(mutated, raw)
	mutated[12] = 0x02
	recomputeCRC(mutated)

	f, err := ParseFrame(mutated)
	if err != nil {
		t.Fatalf("ParseFrame on additive-mode frame: %v", err)
	}
	dec, err := DecodeReg02(f, 0, DirectionSlaveToMaster)
	if err != nil {
		t.Fatalf("DecodeReg02 returned error on additive mode: %v", err)
	}
	if dec.Mode.Value != "mode_unknown_02" {
		t.Errorf("Mode.Value = %q, want mode_unknown_02", dec.Mode.Value)
	}
	if dec.Mode.Raw != 0x02 {
		t.Errorf("Mode.Raw = 0x%02X, want 0x02", dec.Mode.Raw)
	}
	if dec.Mode.ConfirmationStatus != Unknown {
		t.Errorf("Mode.status = %v, want unknown", dec.Mode.ConfirmationStatus)
	}
}
