package lg

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// LGCNP-01 프레임 상수
// ---------------------------------------------------------------------------

const (
	// lgcnpODUSTX 는 TYPE-A (ODU) 프레임의 시작 바이트이다.
	lgcnpODUSTX byte = 0x58

	// lgcnpODUFrameLen 은 TYPE-A (ODU) 프레임의 고정 길이이다.
	lgcnpODUFrameLen = 20

	// lgcnpIDUFrameLen 은 TYPE-B (IDU) 프레임의 고정 길이이다.
	lgcnpIDUFrameLen = 40

	// lgcnpIDUAddrMin 은 TYPE-B (IDU) 프레임 STX의 최소값이다 (0x81 = IDU #1).
	lgcnpIDUAddrMin byte = 0x81

	// lgcnpIDUAddrMax 는 TYPE-B (IDU) 프레임 STX의 최대값이다 (0x85 = IDU #5).
	lgcnpIDUAddrMax byte = 0x85
)

// ---------------------------------------------------------------------------
// LGCNP-01 TYPE-A (ODU) 프레임 구조체
// ---------------------------------------------------------------------------

// LGCNPODUFrame 은 파싱된 LGCNP-01 TYPE-A (ODU) 프레임이다 (20바이트 고정).
type LGCNPODUFrame struct {
	Raw           [lgcnpODUFrameLen]byte // 원시 바이트
	Timestamp     time.Time              // 수신 시각
	SEQ           byte                   // byte[1], 01~05
	ChecksumValid bool                   // 체크섬 검증 결과
	ParseErr      error                  // 파싱 에러 (정상이면 nil)
}

// String 은 ODU 프레임의 요약 문자열을 반환한다.
func (f *LGCNPODUFrame) String() string {
	if f.ParseErr != nil {
		return fmt.Sprintf("LGCNPODUFrame{err=%v, raw=%s}", f.ParseErr, hex.EncodeToString(f.Raw[:]))
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "LGCNPODUFrame{SEQ=%d", f.SEQ)
	fmt.Fprintf(&sb, ", checksum=%v", f.ChecksumValid)
	fmt.Fprintf(&sb, ", raw=%s}", hex.EncodeToString(f.Raw[:]))
	return sb.String()
}

// ---------------------------------------------------------------------------
// LGCNP-01 TYPE-B (IDU) 프레임 구조체
// ---------------------------------------------------------------------------

// LGCNPIDUFrame 은 파싱된 LGCNP-01 TYPE-B (IDU) 프레임이다 (40바이트 고정).
//
// v0.18.1: 일부 LGCNP 디바이스 / Serial-to-TCP 브릿지 환경에서 IDU 프레임이
// b[0..19] 의 20바이트 short 형식으로만 수신되는 경우가 있다 (b[20..39]
// redundancy 절반이 다음 IDU/ODU 프레임으로 잘림). 이때 IsShort=true 로
// 표시되며 다음 필드만 유효: Power, Mode, SetTemp, SlotNum, OpMode, DevType,
// DeviceID. RoomTemp/InletTemp/OutletTemp/FanByte 는 부재.
type LGCNPIDUFrame struct {
	Raw             [lgcnpIDUFrameLen]byte // 원시 바이트
	IsShort         bool                   // v0.18.1: true 면 20바이트 short variant — b[20..39] 무효 (zero-pad)
	Timestamp       time.Time              // 수신 시각
	IDUAddr         byte                   // 0x81~0x85
	IDUNum          int                    // 1~5 (IDUAddr - 0x80)
	CMD             byte                   // byte[1] 전체 CMD 바이트
	SubCMD          byte                   // byte[2] 서브커맨드 (0x00 또는 0x01)
	CycleBit        bool                   // CMD bit6: true=B사이클, false=A사이클
	ActiveBit       bool                   // CMD bit0: 활성 운전 상태
	GroupBBit       bool                   // CMD bit3: 그룹 B (냉방 그룹 등)
	UnchangedBit    bool                   // CMD bit2: 설정 미변경 IDU 마커
	ActiveFlag      bool                   // b[18] bit7: 활성 운전 플래그
	SetTempReliable bool                   // CMD가 설정온도 신뢰 가능한 프레임인지
	RedundancyValid bool                   // 이중 기록 검증 결과 (Short 면 trivially true)
	StructureValid  bool                   // 구조 검증 결과 (Short 면 trivially true)
	RangeOk         bool                   // 물리 범위 검증 결과 (Short 면 SetTemp 만 검증)
	SlotNum         byte                   // byte[9] IDU 슬롯번호 (0x51~0x55)
	SetTemp         float64                // 설정 온도 (°C) = b[11] + 15
	RoomTemp        float64                // 실내 온도 (°C) — Short 면 0 (무효)
	InletTemp       float64                // 입구 온도 (°C) — Short 면 0 (무효)
	OutletTemp      float64                // 출구 온도 (°C) — Short 면 0 (무효)
	FanByte         byte                   // byte[30] 풍량 바이트 — Short 면 0 (무효)
	OpMode          byte                   // byte[10] 운전 모드
	SetTempRaw      byte                   // byte[11] 설정온도 원시값
	DevType         byte                   // byte[3] 디바이스 타입
	DeviceID        byte                   // byte[4] 디바이스 ID
	ParseErr        error                  // 파싱 에러 (정상이면 nil)
}

// String 은 IDU 프레임의 요약 문자열을 반환한다.
func (f *LGCNPIDUFrame) String() string {
	if f.ParseErr != nil {
		return fmt.Sprintf("LGCNPIDUFrame{err=%v, raw=%s}", f.ParseErr, hex.EncodeToString(f.Raw[:]))
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "LGCNPIDUFrame{IDU=%d", f.IDUNum)
	fmt.Fprintf(&sb, ", CMD=%02X", f.CMD)
	cycle := "A"
	if f.CycleBit {
		cycle = "B"
	}
	fmt.Fprintf(&sb, "(%s)", cycle)
	if f.ActiveBit {
		sb.WriteString(",active")
	}
	if f.ActiveFlag {
		sb.WriteString(",flag")
	}
	fmt.Fprintf(&sb, ", slot=%02X, set=%.0f°C", f.SlotNum, f.SetTemp)
	if !f.SetTempReliable {
		sb.WriteString("(?)")
	}
	fmt.Fprintf(&sb, ", room=%.1f°C", f.RoomTemp)
	fmt.Fprintf(&sb, ", inlet=%.1f°C, outlet=%.1f°C", f.InletTemp, f.OutletTemp)
	fmt.Fprintf(&sb, ", redundancy=%v, structure=%v, range=%v}", f.RedundancyValid, f.StructureValid, f.RangeOk)
	return sb.String()
}

// ---------------------------------------------------------------------------
// LGCNP-01 프레임 파서
// ---------------------------------------------------------------------------

// LGCNPFrameParser 는 io.Reader 에서 바이트를 읽어 LGCNP-01 프레임을 추출한다.
//
// v0.18.1: bufio.Reader 로 wrap 하여 Peek 기반 IDU 길이 자동 감지 지원.
// 일부 디바이스 / Serial-to-TCP 브릿지 환경에서 IDU 프레임이 20바이트 short
// 형식으로 도착하는 경우, b[20] 위치의 다음 STX 를 미리 보고 20바이트만
// 소비하여 다음 프레임의 동기를 유지한다.
type LGCNPFrameParser struct {
	reader *bufio.Reader
}

// NewLGCNPFrameParser 는 새 LGCNP-01 프레임 파서를 생성한다.
func NewLGCNPFrameParser(reader io.Reader) *LGCNPFrameParser {
	if br, ok := reader.(*bufio.Reader); ok {
		return &LGCNPFrameParser{reader: br}
	}
	return &LGCNPFrameParser{reader: bufio.NewReaderSize(reader, 256)}
}

// ReadFrame 은 스트림에서 하나의 완전한 LGCNP-01 프레임을 읽어 반환한다.
// STX 바이트에 따라 TYPE-A(0x58) 또는 TYPE-B(0x81~0x85)를 식별한다.
// frameType: 'A' = ODU, 'B' = IDU
// oduFrame, iduFrame 중 하나만 non-nil 이다.
func (p *LGCNPFrameParser) ReadFrame() (frameType byte, oduFrame *LGCNPODUFrame, iduFrame *LGCNPIDUFrame, err error) {
	// 1단계: STX 바이트 스캔
	buf := make([]byte, 1)
	for {
		_, err = io.ReadFull(p.reader, buf)
		if err != nil {
			return 0, nil, nil, err
		}

		stx := buf[0]
		if stx == lgcnpODUSTX {
			// TYPE-A (ODU): 나머지 19바이트 읽기
			oduFrame, err = p.readODUFrame(stx)
			if err != nil {
				return 0, nil, nil, err
			}
			return 'A', oduFrame, nil, nil
		}
		if stx >= lgcnpIDUAddrMin && stx <= lgcnpIDUAddrMax {
			// TYPE-B (IDU): 나머지 39바이트 읽기
			iduFrame, err = p.readIDUFrame(stx)
			if err != nil {
				return 0, nil, nil, err
			}
			return 'B', nil, iduFrame, nil
		}
		// 알 수 없는 바이트 → 건너뛰기 (동기화 복구)
	}
}

// isLGCNPSTX 는 byte 가 LGCNP-01 의 유효한 STX (ODU 0x58 또는 IDU 0x81~0x85)
// 인지 반환한다. v0.18.1 IDU 길이 자동 감지에 사용.
func isLGCNPSTX(b byte) bool {
	return b == lgcnpODUSTX || (b >= lgcnpIDUAddrMin && b <= lgcnpIDUAddrMax)
}

// readODUFrame 은 STX 이후 나머지 19바이트를 읽어 TYPE-A 프레임을 파싱한다.
func (p *LGCNPFrameParser) readODUFrame(stx byte) (*LGCNPODUFrame, error) {
	var raw [lgcnpODUFrameLen]byte
	raw[0] = stx

	_, err := io.ReadFull(p.reader, raw[1:])
	if err != nil {
		return nil, fmt.Errorf("lgcnp: ODU 프레임 읽기 실패: %w", err)
	}

	f := &LGCNPODUFrame{
		Raw:       raw,
		Timestamp: time.Now(),
		SEQ:       raw[1],
	}

	// 체크섬 검증
	f.ChecksumValid = lgcnpVerifyODUChecksum(raw, f.SEQ)

	return f, nil
}

// readIDUFrame 은 STX 이후 나머지 39바이트를 읽어 TYPE-B 프레임을 파싱한다.
//
// v0.18.1: 일부 디바이스 / Serial-to-TCP 브릿지 환경에서 IDU 프레임이
// 20바이트 short 형식으로 도착하는 경우를 자동 감지. STX 이후 19바이트를
// 먼저 읽은 후 다음 byte 를 Peek 하여:
//   - 다음 byte 가 다른 LGCNP STX (0x58 또는 0x81..0x85) 면 → 20바이트 short 변형으로 처리 (다음 프레임 동기 유지)
//   - 그 외 → 추가 20바이트를 읽어 표준 40바이트 long 형식으로 처리
//
// Short 변형은 RedundancyValid/StructureValid 가 trivially true 이고
// b[20..39] 가 zero-pad 되어 RoomTemp/InletTemp/OutletTemp/FanByte 가 무효.
func (p *LGCNPFrameParser) readIDUFrame(stx byte) (*LGCNPIDUFrame, error) {
	var raw [lgcnpIDUFrameLen]byte
	raw[0] = stx

	// 먼저 19바이트 (b[1..19]) 만 읽는다.
	if _, err := io.ReadFull(p.reader, raw[1:20]); err != nil {
		return nil, fmt.Errorf("lgcnp: IDU 프레임 첫 절반 읽기 실패: %w", err)
	}

	// 다음 byte 가 다른 LGCNP STX 인지 Peek 으로 확인 (소비하지 않음).
	//
	// 판단 규칙:
	//   - Peek 성공 + LGCNP STX → short variant (다음 프레임 시작)
	//   - Peek 성공 + 비-STX → long variant (redundancy 절반 읽기)
	//   - Peek 실패 (EOF) → short variant (스트림 종료, 이 20B 가 마지막 완전한 프레임)
	isShort := false
	peeked, perr := p.reader.Peek(1)
	if perr != nil {
		// 스트림 종료 또는 일시적 read 실패 — 이미 읽은 20B 를 완전한 short 프레임으로 처리.
		isShort = true
	} else if len(peeked) == 1 && isLGCNPSTX(peeked[0]) {
		// 다음 byte 가 새 프레임 STX → 이번 IDU 는 20바이트 short 변형.
		isShort = true
	}
	if isShort {
		// raw[20..39] 는 zero 그대로 유지 (RoomTemp 등 무효 표시).
	} else {
		// 표준 40바이트 long 형식: 나머지 20바이트 읽기.
		if _, err := io.ReadFull(p.reader, raw[20:]); err != nil {
			return nil, fmt.Errorf("lgcnp: IDU 프레임 두번째 절반 읽기 실패: %w", err)
		}
	}

	cmd := raw[1]
	f := &LGCNPIDUFrame{
		Raw:          raw,
		IsShort:      isShort,
		Timestamp:    time.Now(),
		IDUAddr:      stx,
		IDUNum:       int(stx) - 0x80,
		CMD:          cmd,
		SubCMD:       raw[2],
		CycleBit:     cmd&0x40 != 0,
		ActiveBit:    cmd&0x01 != 0,
		GroupBBit:    cmd&0x08 != 0,
		UnchangedBit: cmd&0x04 != 0,
		ActiveFlag:   raw[18]&0x80 != 0,
		DevType:      raw[3],
		DeviceID:     raw[4],
		FanByte:      raw[30], // Short 면 0
		OpMode:       raw[10],
		SetTempRaw:   raw[11],
	}

	// (02,00) 프레임은 설정온도 비신뢰. 그 외 CMD에서만 신뢰 가능.
	f.SetTempReliable = !(cmd == 0x02 && raw[2] == 0x00)

	// b[09]는 IDU 슬롯번호 (0x51~0x55)
	f.SlotNum = raw[9]

	// 온도 변환: b[11] = 설정온도 원시값, 설정온도 = b[11] + 15
	f.SetTemp = float64(int(raw[11]) + 15)
	if !isShort {
		f.RoomTemp = lgcnpDecodeSensorTemp(raw[23])
		f.InletTemp = lgcnpDecodeSensorTemp(raw[24])
		f.OutletTemp = lgcnpDecodeSensorTemp(raw[25])
	}
	// Short 면 RoomTemp/InletTemp/OutletTemp 0 유지 (무효).

	if isShort {
		// Short variant: redundancy/structure 검증 불가능 → trivially 통과.
		// 핸들러 (handleIDUFrame) 에서 IsShort 를 확인하여 무효 필드 emit 회피.
		f.RedundancyValid = true
		f.StructureValid = true
	} else {
		f.RedundancyValid = lgcnpVerifyIDURedundancy(raw)
		f.StructureValid = lgcnpVerifyIDUStructure(raw)
	}

	// 물리 범위 검증 (Short 는 SetTemp 만 의미 있음)
	f.RangeOk = lgcnpVerifyIDURange(f)

	return f, nil
}

// ---------------------------------------------------------------------------
// 체크섬 / 검증 함수
// ---------------------------------------------------------------------------

// lgcnpVerifyODUChecksum 은 TYPE-A (ODU) 프레임의 체크섬을 검증한다.
//
//	SEQ=01, SEQ=05: XOR(bytes[0:19]) == bytes[19]
//	SEQ=04: SUM(bytes[0:19]) & 0xFF == bytes[19]
//	         (v0.18.10) SUM 실패 시 bytes[19]==0x55 이면 fixed marker variant
//	         로 인식하여 유효 처리. 일부 디바이스가 표준 SUM 대신 0x55 marker
//	         를 사용함 — verify_odu_checksum 옵션 없이 자동 감지.
//	SEQ=02, SEQ=03: 체크섬 없음 (bytes[18:20]은 센서 데이터), 항상 유효
func lgcnpVerifyODUChecksum(raw [lgcnpODUFrameLen]byte, seq byte) bool {
	switch seq {
	case 0x01, 0x05:
		// XOR 체크섬: bytes[0:19] XOR = bytes[19]
		var xor byte
		for i := 0; i < 19; i++ {
			xor ^= raw[i]
		}
		return xor == raw[19]

	case 0x04:
		// SUM 체크섬: SUM(bytes[0:19]) & 0xFF = bytes[19]
		var sum byte
		for i := 0; i < 19; i++ {
			sum += raw[i]
		}
		if sum == raw[19] {
			return true
		}
		// v0.18.10: fixed 0x55 marker variant 자동 감지.
		return raw[19] == 0x55

	case 0x02, 0x03:
		// 체크섬 없음 — 항상 유효
		return true

	default:
		// 알 수 없는 SEQ — 체크섬 검증 불가, 유효로 처리
		return true
	}
}

// lgcnpVerifyIDURedundancy 는 TYPE-B (IDU) 프레임의 이중 기록을 검증한다.
//
//	b[9]  == b[29] (슬롯번호 중복)
//	b[11] == b[31] (설정온도 중복)
//	b[23] == b[36] (실내 온도 중복)
func lgcnpVerifyIDURedundancy(raw [lgcnpIDUFrameLen]byte) bool {
	return raw[9] == raw[29] && raw[11] == raw[31] && raw[23] == raw[36]
}

// lgcnpVerifyIDUStructure 는 TYPE-B (IDU) 프레임의 구조를 검증한다.
//
//	b[1] CMD: 허용 비트 마스크 검증 (§6.8)
//	b[20]은 (b[0] - 0x81 + 1) 이어야 함
func lgcnpVerifyIDUStructure(raw [lgcnpIDUFrameLen]byte) bool {
	if !lgcnpIsValidCMD(raw[1]) {
		return false
	}
	expectedB20 := raw[0] - 0x81 + 1
	return raw[20] == expectedB20
}

// lgcnpIsValidCMD 는 CMD 바이트가 유효한지 비트 마스크로 검증한다.
// 허용 비트: bit6(0x40), bit3(0x08), bit2(0x04), bit1(0x02), bit0(0x01)
// 비허용 비트: bit7, bit5, bit4 — 이 비트가 세팅되면 무효.
func lgcnpIsValidCMD(cmd byte) bool {
	const allowedMask byte = 0x4F // 0b0100_1111 = bit6|bit3|bit2|bit1|bit0
	return cmd & ^allowedMask == 0
}

// lgcnpVerifyIDURange 는 TYPE-B (IDU) 프레임의 온도 물리 범위를 검증한다.
//
//	설정 온도: 18~30°C
//	실내 온도: 0~50°C
//	입구/출구 온도: 0~70°C
//
// v0.18.1: IsShort 인 경우 RoomTemp/InletTemp/OutletTemp 는 부재 (0 값) 이므로
// SetTemp 만 검증한다.
func lgcnpVerifyIDURange(f *LGCNPIDUFrame) bool {
	if f.SetTemp < 18 || f.SetTemp > 30 {
		return false
	}
	if f.IsShort {
		return true
	}
	if f.RoomTemp < 0 || f.RoomTemp > 50 {
		return false
	}
	if f.InletTemp < 0 || f.InletTemp > 70 {
		return false
	}
	if f.OutletTemp < 0 || f.OutletTemp > 70 {
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// 온도 변환 함수
// ---------------------------------------------------------------------------

// lgcnpDecodeSensorTemp 는 센서 온도 바이트를 섭씨로 변환한다.
//
//	sensor_temp = (b - 0x40) / 2.0 (0.5°C 단위)
func lgcnpDecodeSensorTemp(b byte) float64 {
	return float64(int(b)-0x40) / 2.0
}

// lgcnpDecodeODUOutdoorTemp 는 ODU SEQ=02 프레임의 실외 온도를 변환한다.
//
//	outdoor_temp = (b - 0x40) / 2.0
func lgcnpDecodeODUOutdoorTemp(b byte) float64 {
	return float64(int(b)-0x40) / 2.0
}
