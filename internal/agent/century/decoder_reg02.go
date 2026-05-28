package century

import (
	"encoding/binary"
	"fmt"
)

// reg02DataLength 는 reg 0x02 응답의 data 영역 길이이다 (REQ-CENTURY-006).
const reg02DataLength = 17

// DecodeReg02 는 reg 0x02 응답 (현재 설정 readback, 17B data) 을 typed event 로 디코딩한다.
//
// 입력 frame 은 ParseFrame 을 거친 *Frame 이어야 하며, register byte 가 0x02 이고
// data 길이가 정확히 17B 이어야 한다. SPEC-CENTURY-HVACR-001 §6.1 의 모든 17 바이트를
// 빠짐없이 typed field 로 노출한다.
//
// confirmation_status 분류 (REQ-CENTURY-021):
//   - mode / fan / setpoint_c             : Confirmed
//   - reg02_word_11 / live_13 / live_14 / live_15 : Inferred
//   - reg02_byte_0/3/4/5/6/9/10/16        : Unknown (4 캡처 모두 0x00)
//
// (REQ-CENTURY-006, REQ-CENTURY-011, REQ-CENTURY-020, REQ-CENTURY-021, REQ-CENTURY-026)
func DecodeReg02(f *Frame, tsMs int64, direction string) (*Reg02Decoded, error) {
	if f == nil {
		return nil, ErrNilFrame
	}
	reg, ok := f.Register()
	if !ok {
		return nil, fmt.Errorf("%w: payload prefix missing", ErrInvalidPayloadPrefix)
	}
	if reg != 0x02 {
		return nil, fmt.Errorf("%w: got 0x%02X, want 0x02", ErrInvalidRegister, reg)
	}
	data := f.Data()
	if len(data) != reg02DataLength {
		return nil, fmt.Errorf("%w: reg 0x02 data length = %d, want %d",
			ErrInvalidPayloadLength, len(data), reg02DataLength)
	}

	setpointRaw := binary.LittleEndian.Uint16(data[7:9])
	word11Raw := binary.LittleEndian.Uint16(data[11:13])

	return &Reg02Decoded{
		Type:        EventTypeReg02Response,
		SubDevID:    HexU8(f.Payload[0]),
		Register:    reg,
		TimestampMs: tsMs,
		Direction:   direction,

		Mode:        NewModeField(data[1]),
		Fan:         FieldU8{Value: data[2], ConfirmationStatus: Confirmed},
		SetpointC:   FieldFloat32{Value: float32(setpointRaw) / 10.0, Raw: setpointRaw, ConfirmationStatus: Confirmed},
		Reg02Word11: FieldFloat32{Value: float32(word11Raw) / 10.0, Raw: word11Raw, ConfirmationStatus: Inferred},
		Reg02Live13: FieldU8{Value: data[13], ConfirmationStatus: Inferred},
		Reg02Live14: FieldU8{Value: data[14], ConfirmationStatus: Inferred},
		Reg02Live15: FieldU8{Value: data[15], ConfirmationStatus: Inferred},

		Reg02Byte0:  FieldU8{Value: data[0], ConfirmationStatus: Unknown},
		Reg02Byte3:  FieldU8{Value: data[3], ConfirmationStatus: Unknown},
		Reg02Byte4:  FieldU8{Value: data[4], ConfirmationStatus: Unknown},
		Reg02Byte5:  FieldU8{Value: data[5], ConfirmationStatus: Unknown},
		Reg02Byte6:  FieldU8{Value: data[6], ConfirmationStatus: Unknown},
		Reg02Byte9:  FieldU8{Value: data[9], ConfirmationStatus: Unknown},
		Reg02Byte10: FieldU8{Value: data[10], ConfirmationStatus: Unknown},
		Reg02Byte16: FieldU8{Value: data[16], ConfirmationStatus: Unknown},
	}, nil
}
