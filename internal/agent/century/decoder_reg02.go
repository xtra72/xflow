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
//   - reg02_word_7 / live_13 / live_14 / live_15 : Inferred
//   - reg02_byte_0/3/4/5/6/9/10/16        : Unknown (4 캡처 모두 0x00)
//
// 2026-05-29 setpoint byte 위치 정정 (실측 검증):
//
//	사용자 6-point 실험 (18/20/22/24/26/28°C) 로 setpoint 가 data[11..12] LE u16 ÷ 10 로
//	완벽 linear 부호화됨이 확정되었다. 이전엔 data[7..8] 을 setpoint 로 가정 (CAP-3/4
//	fixture 가 우연히 두 위치 모두 250 = 25°C 이라 테스트가 통과해왔음).
//	data[7..8] 은 별개 의미 (≤25°C 일 때 250 고정, 26°C → 245, 28°C → 240 — "cooling
//	capacity ceiling" 또는 max compressor speed 추정) — reg02_word_7 으로 노출.
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

	// setpoint 은 data[11..12] LE u16 ÷ 10 (실측 검증, 2026-05-29).
	setpointRaw := binary.LittleEndian.Uint16(data[11:13])
	// data[7..8] 은 별개 운전 파라미터 (cooling capacity ceiling / max compressor speed 추정).
	word7Raw := binary.LittleEndian.Uint16(data[7:9])

	return &Reg02Decoded{
		Type:        EventTypeReg02Response,
		SubDevID:    HexU8(f.Payload[0]),
		Register:    reg,
		TimestampMs: tsMs,
		Direction:   direction,

		Mode:        NewModeField(data[1]),
		Fan:         FieldU8{Value: data[2], ConfirmationStatus: Confirmed},
		SetpointC:   FieldFloat32{Value: float32(setpointRaw) / 10.0, Raw: setpointRaw, ConfirmationStatus: Confirmed},
		Reg02Word7:  FieldFloat32{Value: float32(word7Raw) / 10.0, Raw: word7Raw, ConfirmationStatus: Inferred},
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
