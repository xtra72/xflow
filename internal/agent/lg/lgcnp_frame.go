package lg

import (
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
type LGCNPIDUFrame struct {
	Raw             [lgcnpIDUFrameLen]byte // 원시 바이트
	Timestamp       time.Time              // 수신 시각
	IDUAddr         byte                   // 0x81~0x85
	IDUNum          int                    // 1~5 (IDUAddr - 0x80)
	CMD             byte                   // byte[1], 0x02 또는 0x43
	RedundancyValid bool                   // 이중 기록 검증 결과
	StructureValid  bool                   // 구조 검증 결과
	RangeOk         bool                   // 물리 범위 검증 결과
	SlotNum         byte                   // byte[9] IDU 슬롯번호 (0x51~0x55)
	RoomTemp        float64                // 실내 온도 (°C)
	InletTemp       float64                // 입구 온도 (°C)
	OutletTemp      float64                // 출구 온도 (°C)
	FanParam        byte                   // byte[8] 팬/풍량 파라미터
	OpMode          byte                   // byte[10] 운전 모드
	StatusFlags     byte                   // byte[11] 상태 플래그
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
	fmt.Fprintf(&sb, ", slot=%02X, room=%.1f°C", f.SlotNum, f.RoomTemp)
	fmt.Fprintf(&sb, ", inlet=%.1f°C, outlet=%.1f°C", f.InletTemp, f.OutletTemp)
	fmt.Fprintf(&sb, ", redundancy=%v, structure=%v, range=%v}", f.RedundancyValid, f.StructureValid, f.RangeOk)
	return sb.String()
}

// ---------------------------------------------------------------------------
// LGCNP-01 프레임 파서
// ---------------------------------------------------------------------------

// LGCNPFrameParser 는 io.Reader 에서 바이트를 읽어 LGCNP-01 프레임을 추출한다.
type LGCNPFrameParser struct {
	reader io.Reader
}

// NewLGCNPFrameParser 는 새 LGCNP-01 프레임 파서를 생성한다.
func NewLGCNPFrameParser(reader io.Reader) *LGCNPFrameParser {
	return &LGCNPFrameParser{reader: reader}
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
func (p *LGCNPFrameParser) readIDUFrame(stx byte) (*LGCNPIDUFrame, error) {
	var raw [lgcnpIDUFrameLen]byte
	raw[0] = stx

	_, err := io.ReadFull(p.reader, raw[1:])
	if err != nil {
		return nil, fmt.Errorf("lgcnp: IDU 프레임 읽기 실패: %w", err)
	}

	f := &LGCNPIDUFrame{
		Raw:       raw,
		Timestamp: time.Now(),
		IDUAddr:   stx,
		IDUNum:    int(stx) - 0x80,
		CMD:       raw[1],
		DevType:     raw[3],
		DeviceID:    raw[4],
		FanParam:    raw[8],
		OpMode:      raw[10],
		StatusFlags: raw[11],
	}

	// b[09]는 IDU 슬롯번호 (0x51~0x55), 설정온도가 아님
	f.SlotNum = raw[9]

	// 온도 변환
	f.RoomTemp = lgcnpDecodeSensorTemp(raw[23])
	f.InletTemp = lgcnpDecodeSensorTemp(raw[24])
	f.OutletTemp = lgcnpDecodeSensorTemp(raw[25])

	// 이중 기록 검증 (redundancy)
	f.RedundancyValid = lgcnpVerifyIDURedundancy(raw)

	// 구조 검증
	f.StructureValid = lgcnpVerifyIDUStructure(raw)

	// 물리 범위 검증
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
		return sum == raw[19]

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
//	b[9] == b[29] (슬롯번호 중복)
//	b[23] == b[36] (실내 온도 중복)
func lgcnpVerifyIDURedundancy(raw [lgcnpIDUFrameLen]byte) bool {
	return raw[9] == raw[29] && raw[23] == raw[36]
}

// lgcnpVerifyIDUStructure 는 TYPE-B (IDU) 프레임의 구조를 검증한다.
//
//	b[1]은 0x02 또는 0x43 이어야 함
//	b[20]은 (b[0] - 0x81 + 1) 이어야 함
func lgcnpVerifyIDUStructure(raw [lgcnpIDUFrameLen]byte) bool {
	if raw[1] != 0x02 && raw[1] != 0x43 {
		return false
	}
	expectedB20 := raw[0] - 0x81 + 1
	return raw[20] == expectedB20
}

// lgcnpVerifyIDURange 는 TYPE-B (IDU) 프레임의 온도 물리 범위를 검증한다.
//
//	실내 온도: 0~50°C
//	입구/출구 온도: 0~70°C
func lgcnpVerifyIDURange(f *LGCNPIDUFrame) bool {
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
