package century

import (
	"errors"
	"testing"
)

// TestDecodeReg03_AllCaptures exercises CAP-1 / CAP-3 / CAP-4 reg 0x03
// responses to confirm REQ-CENTURY-007 (evaporator refrigerant pipe
// temperatures, two confirmed sensors) and AC-A2.
//
// Reference: spec 부록 B-1 4-capture comparison table.
//
//	CAP-1 (off)   : 21.5 / 22.0  °C
//	CAP-3 (start) : 19.5 / 19.5  °C
//	CAP-4 (steady): 9.0  / 8.5   °C
func TestDecodeReg03_AllCaptures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fixture  string
		wantA    float32
		wantB    float32
		wantARaw uint16
		wantBRaw uint16
	}{
		{"CAP-1 off", "cap1_reg03_response.bin", 21.5, 22.0, 215, 220},
		{"CAP-3 cooling start", "cap3_reg03_response.bin", 19.5, 19.5, 195, 195},
		{"CAP-4 cooling steady", "cap4_reg03_response.bin", 9.0, 8.5, 90, 85},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := mustReadFixture(t, tc.fixture)
			f, err := ParseFrame(raw)
			if err != nil {
				t.Fatalf("ParseFrame: %v", err)
			}
			dec, err := DecodeReg03(f, 0, DirectionSlaveToMaster)
			if err != nil {
				t.Fatalf("DecodeReg03: %v", err)
			}

			if dec.Register != 0x03 {
				t.Errorf("Register = 0x%02X, want 0x03", dec.Register)
			}
			if dec.SubDevID != 0x3B {
				t.Errorf("SubDevID = 0x%02X, want 0x3B", dec.SubDevID)
			}
			if dec.EvaporatorTemperatureA.Value != tc.wantA || dec.EvaporatorTemperatureA.Raw != tc.wantARaw {
				t.Errorf("EvaporatorTemperatureA = %+v, want value=%v raw=%d", dec.EvaporatorTemperatureA, tc.wantA, tc.wantARaw)
			}
			if dec.EvaporatorTemperatureB.Value != tc.wantB || dec.EvaporatorTemperatureB.Raw != tc.wantBRaw {
				t.Errorf("EvaporatorTemperatureB = %+v, want value=%v raw=%d", dec.EvaporatorTemperatureB, tc.wantB, tc.wantBRaw)
			}
			if dec.EvaporatorTemperatureA.ConfirmationStatus != Confirmed ||
				dec.EvaporatorTemperatureB.ConfirmationStatus != Confirmed {
				t.Errorf("evap status = %v/%v, want both confirmed",
					dec.EvaporatorTemperatureA.ConfirmationStatus, dec.EvaporatorTemperatureB.ConfirmationStatus)
			}

			// data[4..15] is zero-padding in every capture, exposed as Unknown.
			pads := []FieldU8{
				dec.Reg03Pad4, dec.Reg03Pad5, dec.Reg03Pad6, dec.Reg03Pad7,
				dec.Reg03Pad8, dec.Reg03Pad9, dec.Reg03Pad10, dec.Reg03Pad11,
				dec.Reg03Pad12, dec.Reg03Pad13, dec.Reg03Pad14, dec.Reg03Pad15,
			}
			for i, p := range pads {
				if p.Value != 0 || p.ConfirmationStatus != Unknown {
					t.Errorf("pad[%d] = %+v, want value=0 status=unknown", i, p)
				}
			}
		})
	}
}

// TestDecodeReg03_WrongRegister ensures the decoder rejects a non-0x03 frame.
func TestDecodeReg03_WrongRegister(t *testing.T) {
	t.Parallel()
	raw := mustReadFixture(t, "cap3_reg02_response.bin")
	f, _ := ParseFrame(raw)
	if _, err := DecodeReg03(f, 0, DirectionSlaveToMaster); !errors.Is(err, ErrInvalidRegister) {
		t.Fatalf("error = %v, want ErrInvalidRegister", err)
	}
}

// TestDecodeReg03_WrongDataLength checks the 16B length guard (REQ-CENTURY-007).
func TestDecodeReg03_WrongDataLength(t *testing.T) {
	t.Parallel()
	f := &Frame{
		Src:           AddrSlave,
		Dst:           AddrMaster,
		PayloadLength: 13, // prefix(3) + 10 (too short)
		FunctionCode:  FCResponse,
		Payload:       append([]byte{0x3B, 0x00, 0x03}, make([]byte, 10)...),
	}
	if _, err := DecodeReg03(f, 0, DirectionSlaveToMaster); !errors.Is(err, ErrInvalidPayloadLength) {
		t.Fatalf("error = %v, want ErrInvalidPayloadLength", err)
	}
}

// TestDecodeReg03_NilFrame guards against nil input.
func TestDecodeReg03_NilFrame(t *testing.T) {
	t.Parallel()
	if _, err := DecodeReg03(nil, 0, DirectionSlaveToMaster); !errors.Is(err, ErrNilFrame) {
		t.Fatalf("error = %v, want ErrNilFrame", err)
	}
}
