package century

import (
	"encoding/binary"
	"fmt"
)

// reg04 의 두 가지 data 길이 (REQ-CENTURY-008, REQ-CENTURY-009).
const (
	reg04ReadDataLength  = 14
	reg04WriteDataLength = 16
)

// validateReg04 은 reg 0x04 의 두 디코더 공통 검증을 수행한다:
// frame nil 체크, register byte 0x04 확인, data 길이 일치.
//
// (REQ-CENTURY-008, REQ-CENTURY-009, REQ-CENTURY-011 stage 5)
func validateReg04(f *Frame, wantDataLength int) ([]byte, error) {
	if f == nil {
		return nil, ErrNilFrame
	}
	reg, ok := f.Register()
	if !ok {
		return nil, fmt.Errorf("%w: payload prefix missing", ErrInvalidPayloadPrefix)
	}
	if reg != 0x04 {
		return nil, fmt.Errorf("%w: got 0x%02X, want 0x04", ErrInvalidRegister, reg)
	}
	data := f.Data()
	if len(data) != wantDataLength {
		return nil, fmt.Errorf("%w: reg 0x04 data length = %d, want %d",
			ErrInvalidPayloadLength, len(data), wantDataLength)
	}
	return data, nil
}

// DecodeReg04Read 는 reg 0x04 응답 (운전 상태 + 운전 데이터, 14B data) 을 typed event 로 디코딩한다.
//
// SPEC §6.3 / 부록 B-1 분류:
//   - data[0]   : status bitmap                       (Inferred)
//   - data[1]   : 4 캡처 모두 0xF6                    (Inferred — 상수)
//   - data[2]   : 4 캡처 모두 0x09                    (Inferred — 상수)
//   - data[7]   : 4 캡처 모두 0x2C                    (Inferred — 상수)
//   - data[8..9]: op_val_1 LE u16                    (Inferred, CAP-4 996)
//   - data[10..11]: reg04_word_10 LE u16 ÷10        (Inferred — 이전엔 TempAC=current_temp 으로
//     가정. 2026-05-29 실측 검증으로 ambient 가 아님이 확인됨. 실제 의미 TBD)
//   - data[12..13]: op_val_2 LE u16                  (Inferred, CAP-4 1248)
//   - data[3..6]: zero padding                        (Unknown)
//
// (REQ-CENTURY-008, REQ-CENTURY-011)
func DecodeReg04Read(f *Frame, tsMs int64, direction string) (*Reg04ReadDecoded, error) {
	data, err := validateReg04(f, reg04ReadDataLength)
	if err != nil {
		return nil, err
	}

	opVal1 := binary.LittleEndian.Uint16(data[8:10])
	// data[10..11] LE u16 ÷ 10 (Inferred — 이전엔 TempAC=current_temp 으로 가정.
	// 2026-05-29 실측 검증으로 ambient 가 아님이 확인됨. 실제 의미 TBD).
	word10Raw := binary.LittleEndian.Uint16(data[10:12])
	opVal2 := binary.LittleEndian.Uint16(data[12:14])

	return &Reg04ReadDecoded{
		Type:        EventTypeReg04Response,
		SubDevID:    HexU8(f.Payload[0]),
		Register:    0x04,
		TimestampMs: tsMs,
		Direction:   direction,

		StatusBits:  FieldU8{Value: data[0], ConfirmationStatus: Inferred},
		Reg04Const1: FieldU8{Value: data[1], ConfirmationStatus: Inferred},
		Reg04Const2: FieldU8{Value: data[2], ConfirmationStatus: Inferred},
		Reg04Const7: FieldU8{Value: data[7], ConfirmationStatus: Inferred},
		OpVal1:      FieldU16{Value: opVal1, ConfirmationStatus: Inferred},
		Reg04Word10: FieldFloat32{Value: float32(word10Raw) / 10.0, Raw: word10Raw, ConfirmationStatus: Inferred},
		OpVal2:      FieldU16{Value: opVal2, ConfirmationStatus: Inferred},

		Reg04Byte3: FieldU8{Value: data[3], ConfirmationStatus: Unknown},
		Reg04Byte4: FieldU8{Value: data[4], ConfirmationStatus: Unknown},
		Reg04Byte5: FieldU8{Value: data[5], ConfirmationStatus: Unknown},
		Reg04Byte6: FieldU8{Value: data[6], ConfirmationStatus: Unknown},
	}, nil
}

// DecodeReg04Write 는 reg 0x04 WRITE 요청 (마스터의 운전 제어 명령, 16B data) 을 typed event 로 디코딩한다.
//
// 본 에이전트는 패시브 캡처 전용이므로 결과의 ObservationMode 는 항상 "passive" 이다.
//
// SPEC §6.4 / 부록 B-1 분류:
//   - data[0]    : 운전 중 set, CAP-4 0x02              (Inferred)
//   - data[1]    : 운전 중 set, CAP-4 0x04              (Inferred)
//   - data[4]    : mode_cmd, 0x00=off / 0x01=cooling    (Confirmed)
//   - data[14]   : 캡처별 0xC4/0xC7/0xC0                (Inferred)
//   - data[15]   : CAP-4 0x0F~0x11 변동                 (Inferred)
//   - data[2,3,5..13]: zero padding                     (Unknown)
//
// (REQ-CENTURY-009, REQ-CENTURY-011, REQ-CENTURY-026)
func DecodeReg04Write(f *Frame, tsMs int64, direction string) (*Reg04WriteDecoded, error) {
	data, err := validateReg04(f, reg04WriteDataLength)
	if err != nil {
		return nil, err
	}

	return &Reg04WriteDecoded{
		Type:            EventTypeReg04WriteRequest,
		SubDevID:        HexU8(f.Payload[0]),
		Register:        0x04,
		TimestampMs:     tsMs,
		Direction:       direction,
		ObservationMode: "passive",

		WriteLive0:  FieldU8{Value: data[0], ConfirmationStatus: Inferred},
		WriteLive1:  FieldU8{Value: data[1], ConfirmationStatus: Inferred},
		ModeCmd:     NewModeField(data[4]),
		WriteByte14: FieldU8{Value: data[14], ConfirmationStatus: Inferred},
		WriteLive15: FieldU8{Value: data[15], ConfirmationStatus: Inferred},

		WriteByte2:  FieldU8{Value: data[2], ConfirmationStatus: Unknown},
		WriteByte3:  FieldU8{Value: data[3], ConfirmationStatus: Unknown},
		WriteByte5:  FieldU8{Value: data[5], ConfirmationStatus: Unknown},
		WriteByte6:  FieldU8{Value: data[6], ConfirmationStatus: Unknown},
		WriteByte7:  FieldU8{Value: data[7], ConfirmationStatus: Unknown},
		WriteByte8:  FieldU8{Value: data[8], ConfirmationStatus: Unknown},
		WriteByte9:  FieldU8{Value: data[9], ConfirmationStatus: Unknown},
		WriteByte10: FieldU8{Value: data[10], ConfirmationStatus: Unknown},
		WriteByte11: FieldU8{Value: data[11], ConfirmationStatus: Unknown},
		WriteByte12: FieldU8{Value: data[12], ConfirmationStatus: Unknown},
		WriteByte13: FieldU8{Value: data[13], ConfirmationStatus: Unknown},
	}, nil
}
