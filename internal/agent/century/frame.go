package century

import "encoding/binary"

// 프레임 포맷 상수 (SPEC-CENTURY-001 §2 / REQ-CENTURY-005).
//
// Wire layout (모든 멀티바이트 정수는 little-endian):
//
//	+-------+-------+-------+---+---+-----------------------+-----------+
//	| src   | dst   | p_len |rsv| fc|     payload (N B)     |  CRC-16   |
//	| LE u16| LE u16| LE u16|=00|   |                       |  LE u16   |
//	+-------+-------+-------+---+---+-----------------------+-----------+
//	0       2       4       6   7   8                       N+8     N+10
const (
	// AddrMaster 는 상위 컨트롤러(마스터)의 LE u16 주소이다 (REQ-CENTURY-002).
	AddrMaster uint16 = 0x0030

	// AddrSlave 는 에어컨 본체(슬레이브)의 LE u16 주소이다 (REQ-CENTURY-002).
	AddrSlave uint16 = 0x0001

	// FCResponse 는 슬레이브의 Response / ACK function code 이다 (SPEC §4).
	FCResponse byte = 0x06

	// FCRead 는 마스터의 Read Request function code 이다.
	FCRead byte = 0x0B

	// FCWrite 는 마스터의 Write Request function code 이다.
	FCWrite byte = 0x0C

	// HeaderLength 는 고정 헤더 길이이다 (src 2 + dst 2 + p_len 2 + reserved 1 + fc 1).
	HeaderLength = 8

	// CRCLength 는 트레일 CRC 길이이다.
	CRCLength = 2

	// MaxPayloadLength 는 frame scanner 가 허용하는 최대 payload 크기이다 (REQ-CENTURY-003).
	// 실제 관측된 최대치는 reg 0x02 응답의 20B 이지만, 펌웨어 변종과 향후 register 추가를
	// 고려해 256 으로 보수적으로 설정한다. 자세한 근거는 SPEC §5.5 참조.
	MaxPayloadLength = 256

	// MinFrameLength 는 완전한 프레임의 최소 길이이다.
	// ACK 가 payload_length=1 이므로 최소 = HeaderLength(8) + 1 + CRCLength(2) = 11.
	MinFrameLength = HeaderLength + 1 + CRCLength
)

// Frame 은 디코딩된 Century 바이너리 프레임이다 (SPEC §2, §4).
//
// CRC 필드는 트레일에 저장된 원시 LE u16 값이며, ValidateCRC 가 raw bytes 전체를
// 받아 재계산값과 비교하여 무결성을 검증한다 (REQ-CENTURY-004).
type Frame struct {
	// Src 는 송신자 주소이다 (LE u16, payload offset 0–1).
	Src uint16

	// Dst 는 수신자 주소이다 (LE u16, 2–3).
	Dst uint16

	// PayloadLength 는 헤더의 p_len 필드이다 (LE u16, 4–5). len(Payload) 와 일치해야 한다.
	PayloadLength uint16

	// Reserved 는 SPEC 상 항상 0x00 인 6번째 바이트이다.
	Reserved byte

	// FunctionCode 는 7번째 바이트이다 (0x06 / 0x0B / 0x0C).
	FunctionCode byte

	// Payload 는 헤더 이후 N 바이트의 페이로드 사본이다.
	// 호출자가 안전하게 보유할 수 있도록 frame parser 가 새 슬라이스로 복사한다.
	Payload []byte

	// CRC 는 트레일에 저장된 LE u16 CRC-16/ARC 값이다.
	CRC uint16
}

// ValidateCRC 는 raw bytes (헤더 + payload + CRC 트레일 전체) 에 대해
// CRC-16/ARC (init 0x0000) 를 재계산하고 마지막 2바이트 (LE) 와 비교한다.
//
// raw 가 너무 짧으면 (CRC 트레일러 미포함) false 를 반환한다.
// (REQ-CENTURY-004, REQ-CENTURY-011 단계 2)
func (f *Frame) ValidateCRC(raw []byte) bool {
	if len(raw) < CRCLength {
		return false
	}
	body := raw[:len(raw)-CRCLength]
	stored := binary.LittleEndian.Uint16(raw[len(raw)-CRCLength:])
	return CRC16ARC(body) == stored
}

// Register 는 페이로드 prefix 의 register byte (payload[2]) 를 반환한다.
// ACK (payload_length=1) 또는 payload 가 3바이트 미만인 경우 (0, false) 를 반환한다.
// (REQ-CENTURY-005)
func (f *Frame) Register() (byte, bool) {
	if len(f.Payload) < 3 {
		return 0, false
	}
	return f.Payload[2], true
}

// Data 는 register-specific data slice (payload[3:]) 를 반환한다.
// ACK 또는 prefix 가 없는 짧은 frame 의 경우 nil 을 반환한다.
// 호출자는 반환된 slice 가 Frame.Payload 의 부분 슬라이스임을 인지해야 한다 — 수정하지 말 것.
// (REQ-CENTURY-005)
func (f *Frame) Data() []byte {
	if len(f.Payload) < 3 {
		return nil
	}
	return f.Payload[3:]
}

// IsACK 는 frame 이 1바이트 ACK payload(0x00) 인지 여부를 반환한다 (REQ-CENTURY-010).
func (f *Frame) IsACK() bool {
	return f.FunctionCode == FCResponse && len(f.Payload) == 1
}
