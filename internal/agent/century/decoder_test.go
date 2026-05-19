package century

import (
	"errors"
	"testing"
)

// TestFrameDirection covers the four cardinal direction mappings the dispatch
// layer relies on (master→slave for reads/writes, slave→master for responses
// and ACKs, unknown otherwise).
func TestFrameDirection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  uint16
		dst  uint16
		want string
	}{
		{"master to slave", AddrMaster, AddrSlave, DirectionMasterToSlave},
		{"slave to master", AddrSlave, AddrMaster, DirectionSlaveToMaster},
		{"unknown addresses", 0x00AB, 0x00CD, DirectionUnknown},
		{"master to master (illegal)", AddrMaster, AddrMaster, DirectionUnknown},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := &Frame{Src: tc.src, Dst: tc.dst}
			if got := FrameDirection(f); got != tc.want {
				t.Fatalf("FrameDirection = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDecode_ACKFixture verifies the dispatcher routes a 1-byte ACK frame to
// ACKDecoded with TimestampMs and Direction populated (REQ-CENTURY-010, AC-A5).
func TestDecode_ACKFixture(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap3_ack.bin")
	f, err := ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}
	const tsMs int64 = 1737216000789
	got, err := Decode(f, tsMs)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	ack, ok := got.(*ACKDecoded)
	if !ok {
		t.Fatalf("Decode returned %T, want *ACKDecoded", got)
	}
	if ack.TimestampMs != tsMs {
		t.Errorf("TimestampMs = %d, want %d", ack.TimestampMs, tsMs)
	}
	if ack.Direction != DirectionSlaveToMaster {
		t.Errorf("Direction = %q, want %q", ack.Direction, DirectionSlaveToMaster)
	}
}

// TestDecode_Reg02Response verifies the dispatcher routes a slave→master
// reg 0x02 response to *Reg02Decoded.
func TestDecode_Reg02Response(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap3_reg02_response.bin")
	f, _ := ParseFrame(raw)
	got, err := Decode(f, 100)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	dec, ok := got.(*Reg02Decoded)
	if !ok {
		t.Fatalf("Decode returned %T, want *Reg02Decoded", got)
	}
	if dec.Mode.Value != "cool" {
		t.Errorf("Mode = %q, want cool", dec.Mode.Value)
	}
	if dec.Direction != DirectionSlaveToMaster {
		t.Errorf("Direction = %q, want slave_to_master", dec.Direction)
	}
}

// TestDecode_Reg03Response verifies the dispatcher routes a slave→master
// reg 0x03 response to *Reg03Decoded.
func TestDecode_Reg03Response(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap4_reg03_response.bin")
	f, _ := ParseFrame(raw)
	got, err := Decode(f, 0)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	dec, ok := got.(*Reg03Decoded)
	if !ok {
		t.Fatalf("Decode returned %T, want *Reg03Decoded", got)
	}
	if dec.TempEvapAC.Value != 9.0 || dec.TempEvapBC.Value != 8.5 {
		t.Errorf("temps = %v/%v, want 9.0/8.5", dec.TempEvapAC.Value, dec.TempEvapBC.Value)
	}
}

// TestDecode_Reg04ReadResponse routes a slave→master reg 0x04 response to
// *Reg04ReadDecoded (14B data).
func TestDecode_Reg04ReadResponse(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap4_reg04_response.bin")
	f, _ := ParseFrame(raw)
	got, err := Decode(f, 0)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	dec, ok := got.(*Reg04ReadDecoded)
	if !ok {
		t.Fatalf("Decode returned %T, want *Reg04ReadDecoded", got)
	}
	if dec.OpVal1.Value != 996 {
		t.Errorf("OpVal1 = %d, want 996", dec.OpVal1.Value)
	}
}

// TestDecode_Reg04WriteRequest routes a master→slave reg 0x04 write to
// *Reg04WriteDecoded (16B data, passive observation).
func TestDecode_Reg04WriteRequest(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap4_write_reg04.bin")
	f, _ := ParseFrame(raw)
	got, err := Decode(f, 0)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	dec, ok := got.(*Reg04WriteDecoded)
	if !ok {
		t.Fatalf("Decode returned %T, want *Reg04WriteDecoded", got)
	}
	if dec.ObservationMode != "passive" {
		t.Errorf("ObservationMode = %q, want passive", dec.ObservationMode)
	}
	if dec.ModeCmd.Value != "cool" {
		t.Errorf("ModeCmd = %q, want cool", dec.ModeCmd.Value)
	}
	if dec.Direction != DirectionMasterToSlave {
		t.Errorf("Direction = %q, want master_to_slave", dec.Direction)
	}
}

// TestDecode_NilFrame guards against nil input.
func TestDecode_NilFrame(t *testing.T) {
	t.Parallel()
	if _, err := Decode(nil, 0); !errors.Is(err, ErrNilFrame) {
		t.Fatalf("error = %v, want ErrNilFrame", err)
	}
}

// TestDecode_ReadRequest covers master→slave read requests (FC 0x0B). These
// have no decoded payload of interest (the only payload bytes are the prefix
// itself); the dispatcher therefore returns ErrUnsupportedDirection so the
// caller can route the frame to raw-frame consumers only.
func TestDecode_ReadRequest(t *testing.T) {
	t.Parallel()

	raw := mustReadFixture(t, "cap3_read_req_reg02.bin")
	f, err := ParseFrame(raw)
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}
	got, err := Decode(f, 0)
	if !errors.Is(err, ErrUnsupportedDirection) {
		t.Fatalf("error = %v (result %v), want ErrUnsupportedDirection", err, got)
	}
}

// TestDecode_UnknownRegister verifies that a frame whose register byte is
// outside {0x02, 0x03, 0x04} surfaces an ErrInvalidPayloadPrefix from the
// upstream ParseFrame (defence in depth — ensure dispatch never sees one).
// In addition we synthesize a Frame directly to exercise the dispatcher's
// own fallback branch.
func TestDecode_UnknownRegister(t *testing.T) {
	t.Parallel()

	f := &Frame{
		Src:           AddrSlave,
		Dst:           AddrMaster,
		PayloadLength: 6,
		Reserved:      0x00,
		FunctionCode:  FCResponse,
		Payload:       []byte{0x3B, 0x00, 0x05, 0x01, 0x02, 0x03}, // register=0x05
	}
	_, err := Decode(f, 0)
	if !errors.Is(err, ErrUnknownRegister) {
		t.Fatalf("error = %v, want ErrUnknownRegister", err)
	}
}

// TestDecode_WriteWrongRegister covers a master→slave WRITE on a register
// that the v0.1.0 SPEC does not implement (only reg 0x04 WRITE is supported).
func TestDecode_WriteWrongRegister(t *testing.T) {
	t.Parallel()

	f := &Frame{
		Src:           AddrMaster,
		Dst:           AddrSlave,
		PayloadLength: 4,
		FunctionCode:  FCWrite,
		Payload:       []byte{0x3B, 0x00, 0x02, 0xAA}, // attempt WRITE on reg 0x02
	}
	_, err := Decode(f, 0)
	if !errors.Is(err, ErrUnsupportedDirection) {
		t.Fatalf("error = %v, want ErrUnsupportedDirection", err)
	}
}
