package century

import (
	"encoding/binary"
	"fmt"
)

// reg03DataLength 는 reg 0x03 응답의 data 영역 길이이다 (REQ-CENTURY-007).
const reg03DataLength = 16

// DecodeReg03 는 reg 0x03 응답 (증발기 냉매 배관 온도, 16B data) 을 typed event 로 디코딩한다.
//
// SPEC §6.2 / 부록 B-1 로 확정:
//   - data[0..1] : temp_evap_a LE u16 ÷10 (Confirmed)
//   - data[2..3] : temp_evap_b LE u16 ÷10 (Confirmed)
//   - data[4..15]: zero padding (Unknown — 4 캡처 모두 0x00)
//
// (REQ-CENTURY-007, REQ-CENTURY-011, REQ-CENTURY-020, REQ-CENTURY-021)
func DecodeReg03(f *Frame, tsMs int64, direction string) (*Reg03Decoded, error) {
	if f == nil {
		return nil, ErrNilFrame
	}
	reg, ok := f.Register()
	if !ok {
		return nil, fmt.Errorf("%w: payload prefix missing", ErrInvalidPayloadPrefix)
	}
	if reg != 0x03 {
		return nil, fmt.Errorf("%w: got 0x%02X, want 0x03", ErrInvalidRegister, reg)
	}
	data := f.Data()
	if len(data) != reg03DataLength {
		return nil, fmt.Errorf("%w: reg 0x03 data length = %d, want %d",
			ErrInvalidPayloadLength, len(data), reg03DataLength)
	}

	rawA := binary.LittleEndian.Uint16(data[0:2])
	rawB := binary.LittleEndian.Uint16(data[2:4])

	return &Reg03Decoded{
		SubDevID:    f.Payload[0],
		Register:    reg,
		TimestampMs: tsMs,
		Direction:   direction,

		TempEvapAC: FieldFloat32{Value: float32(rawA) / 10.0, Raw: rawA, ConfirmationStatus: Confirmed},
		TempEvapBC: FieldFloat32{Value: float32(rawB) / 10.0, Raw: rawB, ConfirmationStatus: Confirmed},

		Reg03Pad4:  FieldU8{Value: data[4], ConfirmationStatus: Unknown},
		Reg03Pad5:  FieldU8{Value: data[5], ConfirmationStatus: Unknown},
		Reg03Pad6:  FieldU8{Value: data[6], ConfirmationStatus: Unknown},
		Reg03Pad7:  FieldU8{Value: data[7], ConfirmationStatus: Unknown},
		Reg03Pad8:  FieldU8{Value: data[8], ConfirmationStatus: Unknown},
		Reg03Pad9:  FieldU8{Value: data[9], ConfirmationStatus: Unknown},
		Reg03Pad10: FieldU8{Value: data[10], ConfirmationStatus: Unknown},
		Reg03Pad11: FieldU8{Value: data[11], ConfirmationStatus: Unknown},
		Reg03Pad12: FieldU8{Value: data[12], ConfirmationStatus: Unknown},
		Reg03Pad13: FieldU8{Value: data[13], ConfirmationStatus: Unknown},
		Reg03Pad14: FieldU8{Value: data[14], ConfirmationStatus: Unknown},
		Reg03Pad15: FieldU8{Value: data[15], ConfirmationStatus: Unknown},
	}, nil
}
