package century

import (
	"encoding/binary"
	"fmt"
)

// ParseFrame 은 완전한 raw 바이트 (헤더 + payload + CRC 트레일) 를 디코딩하여 *Frame 을 반환한다.
//
// 검증은 REQ-CENTURY-011 의 다층 검증 순서를 따른다:
//  1. 길이 검증     : len(raw) ≥ MinFrameLength, 0 < payload_length ≤ MaxPayloadLength,
//     len(raw) == HeaderLength + payload_length + CRCLength
//  2. CRC 검증       : CRC-16/ARC (init 0x0000) over raw[:len(raw)-2] == LE trailer
//  3. 헤더 검증      : reserved == 0x00, function_code ∈ {0x06, 0x0B, 0x0C}
//  4. payload prefix : (ACK 제외) payload[1] == 0x00, register ∈ {0x02, 0x03, 0x04}
//
// 각 단계에서 실패하면 후속 단계를 건너뛰고 해당 단계의 sentinel error 를 반환한다.
// (REQ-CENTURY-003, REQ-CENTURY-004, REQ-CENTURY-005, REQ-CENTURY-011)
func ParseFrame(raw []byte) (*Frame, error) {
	// --- 단계 1a: 헤더 + CRC 트레일러 만큼이라도 들어왔는지 확인 ---
	if len(raw) < MinFrameLength {
		return nil, fmt.Errorf("%w: got %d bytes, need >= %d",
			ErrInvalidLength, len(raw), MinFrameLength)
	}

	src := binary.LittleEndian.Uint16(raw[0:2])
	dst := binary.LittleEndian.Uint16(raw[2:4])
	payloadLen := binary.LittleEndian.Uint16(raw[4:6])
	reserved := raw[6]
	fc := raw[7]

	// --- 단계 1b: payload_length 합리 범위 ---
	if payloadLen == 0 {
		return nil, fmt.Errorf("%w: payload_length=0 not allowed",
			ErrInvalidLength)
	}
	if int(payloadLen) > MaxPayloadLength {
		return nil, fmt.Errorf("%w: payload_length=%d > MaxPayloadLength=%d",
			ErrPayloadTooLarge, payloadLen, MaxPayloadLength)
	}

	// --- 단계 1c: 전체 길이 일치 ---
	expected := HeaderLength + int(payloadLen) + CRCLength
	if len(raw) != expected {
		return nil, fmt.Errorf("%w: len=%d, want %d (8 + payload_length(%d) + 2)",
			ErrInvalidLength, len(raw), expected, payloadLen)
	}

	// --- 단계 2: CRC 검증 ---
	body := raw[:len(raw)-CRCLength]
	stored := binary.LittleEndian.Uint16(raw[len(raw)-CRCLength:])
	if computed := CRC16ARC(body); computed != stored {
		return nil, fmt.Errorf("%w: computed=0x%04X, stored=0x%04X",
			ErrInvalidCRC, computed, stored)
	}

	// --- 단계 3: 헤더 검증 ---
	if reserved != 0x00 {
		return nil, fmt.Errorf("%w: reserved byte = 0x%02X, want 0x00",
			ErrInvalidHeader, reserved)
	}
	if !isValidFunctionCode(fc) {
		return nil, fmt.Errorf("%w: function_code = 0x%02X, want one of {0x06, 0x0B, 0x0C}",
			ErrInvalidHeader, fc)
	}

	// payload 의 방어적 복사본을 만든다 — 호출자가 raw 버퍼를 재사용/수정해도 안전하도록.
	payload := make([]byte, payloadLen)
	copy(payload, raw[HeaderLength:HeaderLength+int(payloadLen)])

	f := &Frame{
		Src:           src,
		Dst:           dst,
		PayloadLength: payloadLen,
		Reserved:      reserved,
		FunctionCode:  fc,
		Payload:       payload,
		CRC:           stored,
	}

	// --- 단계 4: payload prefix 검증 (ACK 는 건너뜀) ---
	// ACK = FCResponse + payload_length == 1; payload prefix 자체가 없다.
	if !f.IsACK() {
		if payloadLen < 3 {
			return nil, fmt.Errorf("%w: non-ACK frame payload_length=%d < 3 (prefix missing)",
				ErrInvalidPayloadPrefix, payloadLen)
		}
		if payload[1] != 0x00 {
			return nil, fmt.Errorf("%w: payload reserved2 = 0x%02X, want 0x00",
				ErrInvalidPayloadPrefix, payload[1])
		}
		reg := payload[2]
		if !isKnownRegister(reg) {
			return nil, fmt.Errorf("%w: register = 0x%02X, want one of {0x02, 0x03, 0x04}",
				ErrInvalidPayloadPrefix, reg)
		}
	}

	return f, nil
}

// isValidFunctionCode 는 function_code 가 SPEC §4 의 enum 에 속하는지 검사한다.
func isValidFunctionCode(fc byte) bool {
	return fc == FCResponse || fc == FCRead || fc == FCWrite
}

// isKnownRegister 는 register byte 가 SPEC §6 의 알려진 register 인지 검사한다.
// reg 0x02 (current settings readback), 0x03 (evaporator pipe temperature),
// 0x04 (operation state / control command) 만 v0.1.0 범위.
func isKnownRegister(reg byte) bool {
	return reg == 0x02 || reg == 0x03 || reg == 0x04
}
