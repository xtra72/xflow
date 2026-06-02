package lg

import (
	"encoding/hex"
	"fmt"
)

// Icp02FrameBuilder 는 LG ICP-02 제어 프레임을 구성한다.
//
// 프레임 구조:
//
//	[STX=0x56][LEN][DLEN=0x04][DA(4B)][SLEN=0x04][SA(4B)][CMD(2B)][SEQ0][PLEN][PAYLOAD][SEQ1][CRC16]
//
// LEN = 전체 프레임 길이 - 1 (STX 제외)
// CRC = CRC-16/XMODEM (Big-Endian), 계산 범위: frame[0:len-2]
type Icp02FrameBuilder struct{}

// NewIcp02FrameBuilder 는 프레임 빌더를 생성한다.
func NewIcp02FrameBuilder() *Icp02FrameBuilder {
	return &Icp02FrameBuilder{}
}

// Build 는 완전한 LG ICP-02 프레임을 구성하여 반환한다.
// da, sa 는 각각 4바이트 주소, cmd 는 2바이트 명령,
// seq0 는 명령별 시퀀스, payload 는 가변 페이로드, seq1 은 전역 시퀀스이다.
func (b *Icp02FrameBuilder) Build(da, sa []byte, cmd [2]byte, seq0 byte, payload []byte, seq1 byte) []byte {
	plen := byte(len(payload))
	// STX(1) + LEN(1) + DLEN(1) + DA(4) + SLEN(1) + SA(4) + CMD(2) + SEQ0(1) + PLEN(1) + payload + SEQ1(1) + CRC(2)
	frameLen := 19 + len(payload)

	frame := make([]byte, 0, frameLen)
	frame = append(frame, icp02STX)
	frame = append(frame, byte(frameLen)) // LEN = 전체 프레임 길이 (STX+LEN 포함)
	frame = append(frame, 0x04)             // DLEN
	frame = append(frame, da...)            // DA (4 bytes)
	frame = append(frame, 0x04)             // SLEN
	frame = append(frame, sa...)            // SA (4 bytes)
	frame = append(frame, cmd[0], cmd[1])   // CMD
	frame = append(frame, seq0)             // SEQ0
	frame = append(frame, plen)             // PLEN
	frame = append(frame, payload...)       // PAYLOAD
	frame = append(frame, seq1)             // SEQ1

	// CRC 계산: frame[0:len] (CRC 제외)
	crc := CalcIcp02CRC16(frame)
	frame = append(frame, byte(crc>>8), byte(crc&0xFF))

	return frame
}

// BuildControl 은 제어 프레임(CMD=0x0201) 구성 편의 메서드이다.
func (b *Icp02FrameBuilder) BuildControl(da, sa []byte, seq0 byte, payload []byte, seq1 byte) []byte {
	return b.Build(da, sa, [2]byte{0x02, 0x01}, seq0, payload, seq1)
}

// ParseHexAddress 는 8자리 HEX 문자열을 4바이트 슬라이스로 변환한다.
// 예: "44550065" → []byte{0x44, 0x55, 0x00, 0x65}
func ParseHexAddress(s string) ([]byte, error) {
	if len(s) != 8 {
		return nil, fmt.Errorf("lg_icp02: address must be 8 hex chars, got %d", len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("lg_icp02: invalid hex address %q: %w", s, err)
	}
	return b, nil
}
