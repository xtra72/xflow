package century

import "fmt"

// FrameDirection 은 frame 의 src/dst 주소 쌍으로 회선상 방향을 판정한다.
//
// 매핑 (REQ-CENTURY-005, SPEC §4):
//   - (Src=AddrMaster=0x0030, Dst=AddrSlave=0x0001)  → "master_to_slave"
//   - (Src=AddrSlave=0x0001,  Dst=AddrMaster=0x0030) → "slave_to_master"
//   - 그 외                                          → "unknown"
//
// frame 이 nil 이거나 알려진 주소 쌍이 아니면 DirectionUnknown 을 반환한다.
func FrameDirection(f *Frame) string {
	if f == nil {
		return DirectionUnknown
	}
	switch {
	case f.Src == AddrMaster && f.Dst == AddrSlave:
		return DirectionMasterToSlave
	case f.Src == AddrSlave && f.Dst == AddrMaster:
		return DirectionSlaveToMaster
	default:
		return DirectionUnknown
	}
}

// Decode 는 *Frame 을 적절한 typed event 로 라우팅한다.
//
// 라우팅 규칙 (SPEC §4 + §6):
//   - frame.IsACK()                                    → *ACKDecoded
//   - FC=0x06 (slave→master Response) + reg=0x02       → *Reg02Decoded
//   - FC=0x06 (slave→master Response) + reg=0x03       → *Reg03Decoded
//   - FC=0x06 (slave→master Response) + reg=0x04       → *Reg04ReadDecoded   (14B data)
//   - FC=0x0C (master→slave WRITE)    + reg=0x04       → *Reg04WriteDecoded  (16B data, passive)
//   - FC=0x0B (master→slave READ request)              → ErrUnsupportedDirection
//   - 알려지지 않은 register                            → ErrUnknownRegister
//
// 반환된 interface{} 는 위 5 가지 포인터 타입 중 하나이거나 (nil, err).
// 호출자는 type switch 로 분기한다.
//
// (REQ-CENTURY-005, REQ-CENTURY-010, REQ-CENTURY-011, REQ-CENTURY-020)
func Decode(f *Frame, tsMs int64) (any, error) {
	if f == nil {
		return nil, ErrNilFrame
	}

	direction := FrameDirection(f)

	// ACK 분기 (페이로드 prefix 없음).
	if f.IsACK() {
		return &ACKDecoded{
			TimestampMs: tsMs,
			Direction:   direction,
		}, nil
	}

	reg, ok := f.Register()
	if !ok {
		return nil, fmt.Errorf("%w: payload prefix missing for non-ACK frame",
			ErrInvalidPayloadPrefix)
	}

	switch f.FunctionCode {
	case FCResponse:
		// 슬레이브 → 마스터 응답.
		switch reg {
		case 0x02:
			return DecodeReg02(f, tsMs, direction)
		case 0x03:
			return DecodeReg03(f, tsMs, direction)
		case 0x04:
			return DecodeReg04Read(f, tsMs, direction)
		default:
			return nil, fmt.Errorf("%w: response register 0x%02X", ErrUnknownRegister, reg)
		}

	case FCWrite:
		// 마스터 → 슬레이브 WRITE — v0.1.0 은 reg 0x04 만 지원.
		if reg == 0x04 {
			return DecodeReg04Write(f, tsMs, direction)
		}
		return nil, fmt.Errorf("%w: WRITE on register 0x%02X not supported (v0.1.0 supports only 0x04)",
			ErrUnsupportedDirection, reg)

	case FCRead:
		// 마스터 → 슬레이브 READ request 는 payload 의 prefix 만 있고 decoded
		// content 가 없다. raw frame consumer 가 필요로 하지만 typed event 는 생성하지 않는다.
		return nil, fmt.Errorf("%w: read request frames have no decoded payload (use raw frame node instead)",
			ErrUnsupportedDirection)

	default:
		return nil, fmt.Errorf("%w: function_code 0x%02X", ErrUnknownFunctionCode, f.FunctionCode)
	}
}
