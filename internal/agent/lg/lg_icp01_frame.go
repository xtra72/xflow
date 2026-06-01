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
// LG ICP-01 프레임 상수
// ---------------------------------------------------------------------------

const (
	// icp01ODUSTX 는 TYPE-A (ODU) 프레임의 시작 바이트이다.
	icp01ODUSTX byte = 0x58

	// icp01ODUFrameLen 은 TYPE-A (ODU) 프레임의 고정 길이이다.
	icp01ODUFrameLen = 20

	// icp01IDUFrameLen 은 TYPE-B (IDU) 프레임의 고정 길이이다.
	icp01IDUFrameLen = 40

	// icp01IDUAddrMin 은 TYPE-B (IDU) 프레임 STX의 최소값이다 (0x81 = IDU #1).
	icp01IDUAddrMin byte = 0x81

	// icp01IDUAddrMax 는 TYPE-B (IDU) 프레임 STX의 최대값이다 (0x85 = IDU #5).
	icp01IDUAddrMax byte = 0x85
)

// ---------------------------------------------------------------------------
// LG ICP-01 TYPE-A (ODU) 프레임 구조체
// ---------------------------------------------------------------------------

// Icp01ODUFrame 은 파싱된 LG ICP-01 TYPE-A (ODU) 프레임이다 (20바이트 고정).
type Icp01ODUFrame struct {
	Raw           [icp01ODUFrameLen]byte // 원시 바이트
	Timestamp     time.Time              // 수신 시각
	SEQ           byte                   // byte[1], 01~05
	ChecksumValid bool                   // 체크섬 검증 결과
	ParseErr      error                  // 파싱 에러 (정상이면 nil)
}

// String 은 ODU 프레임의 요약 문자열을 반환한다.
func (f *Icp01ODUFrame) String() string {
	if f.ParseErr != nil {
		return fmt.Sprintf("Icp01ODUFrame{err=%v, raw=%s}", f.ParseErr, hex.EncodeToString(f.Raw[:]))
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Icp01ODUFrame{SEQ=%d", f.SEQ)
	fmt.Fprintf(&sb, ", checksum=%v", f.ChecksumValid)
	fmt.Fprintf(&sb, ", raw=%s}", hex.EncodeToString(f.Raw[:]))
	return sb.String()
}

// ---------------------------------------------------------------------------
// LG ICP-01 TYPE-B (IDU) 프레임 구조체
// ---------------------------------------------------------------------------

// Icp01IDUFrame 은 파싱된 LG ICP-01 TYPE-B (IDU) 프레임이다 (40바이트 고정).
//
// v0.18.1: 일부 LG ICP-01 디바이스 / Serial-to-TCP 브릿지 환경에서 IDU 프레임이
// b[0..19] 의 20바이트 short 형식으로만 수신되는 경우가 있다 (b[20..39]
// redundancy 절반이 다음 IDU/ODU 프레임으로 잘림). 이때 IsShort=true 로
// 표시되며 다음 필드만 유효: Power, Mode, SetTemp, SlotNum, OpMode, DevType,
// DeviceID. RoomTemp/InletTemp/OutletTemp/FanByte 는 부재.
type Icp01IDUFrame struct {
	Raw             [icp01IDUFrameLen]byte // 원시 바이트
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
func (f *Icp01IDUFrame) String() string {
	if f.ParseErr != nil {
		return fmt.Sprintf("Icp01IDUFrame{err=%v, raw=%s}", f.ParseErr, hex.EncodeToString(f.Raw[:]))
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Icp01IDUFrame{IDU=%d", f.IDUNum)
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
// LG ICP-01 프레임 파서
// ---------------------------------------------------------------------------

// Icp01FrameParser 는 io.Reader 에서 바이트를 읽어 LG ICP-01 프레임을 추출한다.
//
// v0.18.1: bufio.Reader 로 wrap 하여 Peek 기반 IDU 길이 자동 감지 지원.
// 일부 디바이스 / Serial-to-TCP 브릿지 환경에서 IDU 프레임이 20바이트 short
// 형식으로 도착하는 경우, b[20] 위치의 다음 STX 를 미리 보고 20바이트만
// 소비하여 다음 프레임의 동기를 유지한다.
//
// v0.18.14: 동기화 복구 (알 수 없는 byte skip) 시 호출자가 추적 가능하도록
// SkippedBytes 누적 + LastSkipped 버퍼 노출. 호출자가 매 ReadFrame 후 확인해
// DEBUG 로그 emit.
type Icp01FrameParser struct {
	reader *bufio.Reader
	// v0.18.14: 마지막 ReadFrame 호출에서 STX 동기화를 위해 skip 한 byte 수.
	// 호출 시점에 0 으로 reset 후 누적, 호출자는 반환 후 LastSkippedCount 로 조회.
	lastSkippedCount int
	// v0.18.14: 마지막 skip 한 byte 의 hex 샘플 (최대 16 byte). 디버그 로그용.
	lastSkippedSample []byte
}

// NewIcp01FrameParser 는 새 LG ICP-01 프레임 파서를 생성한다.
func NewIcp01FrameParser(reader io.Reader) *Icp01FrameParser {
	if br, ok := reader.(*bufio.Reader); ok {
		return &Icp01FrameParser{reader: br}
	}
	return &Icp01FrameParser{reader: bufio.NewReaderSize(reader, 256)}
}

// LastSkippedCount 는 직전 ReadFrame 호출에서 STX 복구로 폐기된 byte 수를 반환 (v0.18.14).
func (p *Icp01FrameParser) LastSkippedCount() int {
	return p.lastSkippedCount
}

// LastSkippedSample 는 직전 ReadFrame 호출에서 폐기된 byte 의 hex 샘플을 반환 (v0.18.14).
// 최대 16 byte 까지. 빈 슬라이스면 skip 없었음.
func (p *Icp01FrameParser) LastSkippedSample() []byte {
	return p.lastSkippedSample
}

// ReadFrame 은 스트림에서 하나의 완전한 LG ICP-01 프레임을 읽어 반환한다.
// STX 바이트에 따라 TYPE-A(0x58) 또는 TYPE-B(0x81~0x85)를 식별한다.
// frameType: 'A' = ODU, 'B' = IDU
// oduFrame, iduFrame 중 하나만 non-nil 이다.
func (p *Icp01FrameParser) ReadFrame() (frameType byte, oduFrame *Icp01ODUFrame, iduFrame *Icp01IDUFrame, err error) {
	// v0.18.14: skip 카운터 / 샘플 리셋. 호출자가 ReadFrame 후 확인.
	p.lastSkippedCount = 0
	p.lastSkippedSample = nil

	// 1단계: STX 바이트 스캔
	buf := make([]byte, 1)
	for {
		_, err = io.ReadFull(p.reader, buf)
		if err != nil {
			return 0, nil, nil, err
		}

		stx := buf[0]
		if stx == icp01ODUSTX {
			// TYPE-A (ODU): 나머지 19바이트 읽기
			oduFrame, err = p.readODUFrame(stx)
			if err != nil {
				return 0, nil, nil, err
			}
			return 'A', oduFrame, nil, nil
		}
		if stx >= icp01IDUAddrMin && stx <= icp01IDUAddrMax {
			// TYPE-B (IDU): 나머지 39바이트 읽기
			iduFrame, err = p.readIDUFrame(stx)
			if err != nil {
				return 0, nil, nil, err
			}
			return 'B', nil, iduFrame, nil
		}
		// 알 수 없는 바이트 → 건너뛰기 (동기화 복구).
		// v0.18.14: 누적 카운트 + 샘플 보관 (호출자가 DEBUG 로그 emit).
		p.lastSkippedCount++
		if len(p.lastSkippedSample) < 16 {
			p.lastSkippedSample = append(p.lastSkippedSample, stx)
		}
	}
}

// isIcp01STX 는 byte 가 LG ICP-01 의 유효한 STX (ODU 0x58 또는 IDU 0x81~0x85)
// 인지 반환한다. v0.18.1 IDU 길이 자동 감지에 사용.
func isIcp01STX(b byte) bool {
	return b == icp01ODUSTX || (b >= icp01IDUAddrMin && b <= icp01IDUAddrMax)
}

// readODUFrame 은 STX 이후 나머지 19바이트를 읽어 TYPE-A 프레임을 파싱한다.
func (p *Icp01FrameParser) readODUFrame(stx byte) (*Icp01ODUFrame, error) {
	var raw [icp01ODUFrameLen]byte
	raw[0] = stx

	_, err := io.ReadFull(p.reader, raw[1:])
	if err != nil {
		return nil, fmt.Errorf("lg_icp01: ODU 프레임 읽기 실패: %w", err)
	}

	f := &Icp01ODUFrame{
		Raw:       raw,
		Timestamp: time.Now(),
		SEQ:       raw[1],
	}

	// 체크섬 검증
	f.ChecksumValid = icp01VerifyODUChecksum(raw, f.SEQ)

	return f, nil
}

// readIDUFrame 은 STX 이후 나머지 39바이트를 읽어 TYPE-B 프레임을 파싱한다.
//
// v0.18.1: 일부 디바이스 / Serial-to-TCP 브릿지 환경에서 IDU 프레임이
// 20바이트 short 형식으로 도착하는 경우를 자동 감지. STX 이후 19바이트를
// 먼저 읽은 후 다음 byte 를 Peek 하여:
//   - 다음 byte 가 다른 LG ICP-01 STX (0x58 또는 0x81..0x85) 면 → 20바이트 short 변형으로 처리 (다음 프레임 동기 유지)
//   - 그 외 → 추가 20바이트를 읽어 표준 40바이트 long 형식으로 처리
//
// Short 변형은 RedundancyValid/StructureValid 가 trivially true 이고
// b[20..39] 가 zero-pad 되어 RoomTemp/InletTemp/OutletTemp/FanByte 가 무효.
func (p *Icp01FrameParser) readIDUFrame(stx byte) (*Icp01IDUFrame, error) {
	var raw [icp01IDUFrameLen]byte
	raw[0] = stx

	// 먼저 19바이트 (b[1..19]) 만 읽는다.
	if _, err := io.ReadFull(p.reader, raw[1:20]); err != nil {
		return nil, fmt.Errorf("lg_icp01: IDU 프레임 첫 절반 읽기 실패: %w", err)
	}

	// v0.18.15: padding-tolerant short detection.
	// v0.18.17: 타이밍 독립성 fix — Peek() timeout 처리 개선.
	//
	// 핵심 문제 (RPI 에서 재현됨):
	//   - transport 의 500ms read deadline → Peek(3) 도중 timeout 발생
	//   - Peek() 가 (partial bytes, timeout_error) 반환
	//   - 기존 로직: timeout error 무시, partial bytes 로 SHORT/LONG 판정
	//   - default case 에서 padding scan 실패 → isShort=false 유지 → LONG 읽음
	//   - io.ReadFull(raw[20:]) 이 다음 프레임 20B 소비 → desync
	//
	// 수정:
	//   - mid-frame read 중 timeout 발생 시: EOF 가 아니면 재시도 (프레임 진행 중)
	//   - between-frame idle timeout: normal continue (idle_timeouts 누적)
	//
	// 판단 규칙 (우선순위 순):
	//   1. Peek 실패 (EOF, perr != nil && len==0) → short variant (스트림 종료)
	//   2. b[20] = LG ICP-01 STX → short variant, no padding
	//   3. b[20] = IDU_INDEX (0x01~0x05) → standard long variant
	//   4. b[20]/b[21]/b[22] 내에서 LG ICP-01 STX 발견 → short variant + padding 소비
	//   5. 그 외 → long variant 로 시도 (redundancy 검증이 폐기 여부 결정)
	const maxPadding = 3
	isShort := false

	// v0.18.17: mid-frame read 중 timeout 발생 시 재시도.
	// between-frame idle timeout 과 구분하기 위해, b[1..20] 읽기 성공 후이므로
	// 프레임 진행 중으로 판단 → timeout 은 일시적 네트워크 지연이지 프레임 경계가 아님.
	var peeked []byte
	var perr error
	for attempt := 0; attempt < 3; attempt++ {
		peeked, perr = p.reader.Peek(maxPadding)
		// perr 가 nil 이거나, EOF 면 루프 빠져나감
		// timeout error 면 재시도 (attempt < 3)
		if perr == nil {
			break
		}
		if perr == io.EOF {
			break
		}
		// timeout error 인지 확인: net.Error.Timeout() == true
		if te, ok := perr.(interface{ Timeout() bool }); ok && te.Timeout() {
			// mid-frame timeout → 재시도 (sleep 없음, 즉시)
			continue
		}
		// 다른 error: 루프 빠져나감
		break
	}

	switch {
	case perr != nil && len(peeked) == 0:
		// 스트림 종료 또는 재시도 실패 — 이 20B 를 완전한 short 프레임으로.
		isShort = true
	case len(peeked) >= 1 && isIcp01STX(peeked[0]):
		// 다음 byte 가 STX → short, no padding.
		isShort = true
	case len(peeked) >= 1 && peeked[0] >= 0x01 && peeked[0] <= 0x05:
		// b[20] 가 IDU_INDEX 범위 (0x01~0x05) → 표준 long frame.
		// 명시적으로 short 가 아니므로 long path 로.
	default:
		// b[20] 가 anomalous (0x00 등) — padding 가능성 검사. 1~maxPadding-1 byte 내에서 STX 면 short.
		for i := 1; i < len(peeked); i++ {
			if isIcp01STX(peeked[i]) {
				// i byte 만큼 padding 소비 (다음 ReadFrame 이 STX 부터 시작하도록).
				_, _ = p.reader.Discard(i)
				isShort = true
				break
			}
		}
	}
	if isShort {
		// raw[20..39] 는 zero 그대로 유지 (RoomTemp 등 무효 표시).
	} else {
		// 표준 40바이트 long 형식: 나머지 20바이트 읽기.
		if _, err := io.ReadFull(p.reader, raw[20:]); err != nil {
			return nil, fmt.Errorf("lg_icp01: IDU 프레임 두번째 절반 읽기 실패: %w", err)
		}
	}

	cmd := raw[1]
	f := &Icp01IDUFrame{
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
		// v0.18.13: b[3] 의 lower nibble 은 frame 마다 변화 (counter/status 추정).
		// upper nibble 만 stable device class 식별자로 사용. 예: 0x72 / 0x73 / 0x75
		// 모두 동일 IDU 에서 관측 → 모두 0x70 으로 정규화.
		DevType:    raw[3] & 0xF0,
		DeviceID:   raw[4],
		FanByte:    raw[30], // Short 면 0
		OpMode:     raw[10],
		SetTempRaw: raw[11],
	}

	// (02,00) 프레임은 설정온도 비신뢰. 그 외 CMD에서만 신뢰 가능.
	f.SetTempReliable = !(cmd == 0x02 && raw[2] == 0x00)

	// b[09]는 IDU 슬롯번호 (0x51~0x55)
	f.SlotNum = raw[9]

	// 온도 변환: b[11] = 설정온도 원시값, 설정온도 = b[11] + 15
	f.SetTemp = float64(int(raw[11]) + 15)
	if !isShort {
		f.RoomTemp = icp01DecodeSensorTemp(raw[23])
		f.InletTemp = icp01DecodeSensorTemp(raw[24])
		f.OutletTemp = icp01DecodeSensorTemp(raw[25])
	}
	// Short 면 RoomTemp/InletTemp/OutletTemp 0 유지 (무효).

	if isShort {
		// Short variant: redundancy/structure 검증 불가능 → trivially 통과.
		// 핸들러 (handleIDUFrame) 에서 IsShort 를 확인하여 무효 필드 emit 회피.
		f.RedundancyValid = true
		f.StructureValid = true
	} else {
		f.RedundancyValid = icp01VerifyIDURedundancy(raw)
		f.StructureValid = icp01VerifyIDUStructure(raw)
	}

	// 물리 범위 검증 (Short 는 SetTemp 만 의미 있음)
	f.RangeOk = icp01VerifyIDURange(f)

	return f, nil
}

// ---------------------------------------------------------------------------
// 체크섬 / 검증 함수
// ---------------------------------------------------------------------------

// icp01VerifyODUChecksum 은 TYPE-A (ODU) 프레임의 체크섬을 검증한다.
//
//	SEQ=01, SEQ=05: XOR(bytes[0:19]) == bytes[19]
//	SEQ=04: SUM(bytes[0:19]) & 0xFF == bytes[19]
//	         (v0.18.10) SUM 실패 시 bytes[19]==0x55 이면 fixed marker variant
//	         로 인식하여 유효 처리. 일부 디바이스가 표준 SUM 대신 0x55 marker
//	         를 사용함 — verify_odu_checksum 옵션 없이 자동 감지.
//	SEQ=02, SEQ=03: 체크섬 없음 (bytes[18:20]은 센서 데이터), 항상 유효
func icp01VerifyODUChecksum(raw [icp01ODUFrameLen]byte, seq byte) bool {
	switch seq {
	case 0x01, 0x05:
		// XOR 체크섬: bytes[0:19] XOR = bytes[19]
		var xor byte
		for i := 0; i < 19; i++ {
			xor ^= raw[i]
		}
		if xor == raw[19] {
			return true
		}
		// v0.18.22: SEQ=01 의 일부 디바이스 변형은 표준 XOR 미사용,
		// 대신 b[19] = b[13] ^ 0x1D marker 패턴 (사용자 실측 4 프레임 확인).
		// 표준 XOR 디바이스 동작 무영향 (먼저 XOR 일치 확인 후 fallback).
		if seq == 0x01 && raw[13]^0x1D == raw[19] {
			return true
		}
		return false

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

// icp01VerifyIDURedundancy 는 TYPE-B (IDU) 프레임의 이중 기록을 검증한다.
//
//	b[9]  == b[29] (슬롯번호 중복)
//	b[11] == b[31] (설정온도 중복)
//	b[23] == b[36] (실내 온도 중복)
func icp01VerifyIDURedundancy(raw [icp01IDUFrameLen]byte) bool {
	return raw[9] == raw[29] && raw[11] == raw[31] && raw[23] == raw[36]
}

// icp01VerifyIDUStructure 는 TYPE-B (IDU) 프레임의 구조를 검증한다.
//
//	b[1] CMD: 허용 비트 마스크 검증 (§6.8)
//	b[20]은 (b[0] - 0x81 + 1) 이어야 함
func icp01VerifyIDUStructure(raw [icp01IDUFrameLen]byte) bool {
	if !icp01IsValidCMD(raw[1]) {
		return false
	}
	expectedB20 := raw[0] - 0x81 + 1
	return raw[20] == expectedB20
}

// icp01IsValidCMD 는 CMD 바이트가 유효한지 비트 마스크로 검증한다.
// 허용 비트: bit6(0x40), bit3(0x08), bit2(0x04), bit1(0x02), bit0(0x01)
// 비허용 비트: bit7, bit5, bit4 — 이 비트가 세팅되면 무효.
func icp01IsValidCMD(cmd byte) bool {
	const allowedMask byte = 0x4F // 0b0100_1111 = bit6|bit3|bit2|bit1|bit0
	return cmd & ^allowedMask == 0
}

// icp01VerifyIDURange 는 TYPE-B (IDU) 프레임의 온도 물리 범위를 검증한다.
//
//	설정 온도: 18~30°C
//	실내 온도: 0~50°C
//	입구/출구 온도: 0~70°C
//
// v0.18.1: IsShort 인 경우 RoomTemp/InletTemp/OutletTemp 는 부재 (0 값) 이므로
// SetTemp 만 검증한다.
func icp01VerifyIDURange(f *Icp01IDUFrame) bool {
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

// icp01DecodeSensorTemp 는 센서 온도 바이트를 섭씨로 변환한다.
//
//	sensor_temp = (b - 0x40) / 2.0 (0.5°C 단위)
func icp01DecodeSensorTemp(b byte) float64 {
	return float64(int(b)-0x40) / 2.0
}

// icp01DecodeODUOutdoorTemp 는 ODU SEQ=02 프레임의 실외 온도를 변환한다.
//
//	outdoor_temp = (b - 0x40) / 2.0
func icp01DecodeODUOutdoorTemp(b byte) float64 {
	return float64(int(b)-0x40) / 2.0
}
