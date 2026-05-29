package lg

import (
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// LG ICP-02 프레임 상수
// ---------------------------------------------------------------------------

const (
	// icp02STX 는 LG ICP-02 프레임의 시작 바이트이다.
	icp02STX byte = 0x56

	// icp02MinFrameLen 은 페이로드가 0인 최소 프레임 길이이다.
	// STX(1) + LEN(1) + DLEN(1) + DA(min 1) + SLEN(1) + SA(min 1) +
	// CMD(2) + SEQ0(1) + PLEN(1) + SEQ1(1) + CRC(2) = 13
	// 실제로는 DA/SA 가 4바이트 이상이므로 더 크지만, 파서에서는 느슨하게 검증한다.
	icp02MinFrameLen = 13

	// icp02MaxFrameLen 은 LEN 필드 최댓값(255)에 해당하는 최대 프레임 길이이다.
	icp02MaxFrameLen = 255
)

// ---------------------------------------------------------------------------
// LG ICP-02 프레임 구조체
// ---------------------------------------------------------------------------

// Icp02Frame 은 파싱된 LG ICP-02 프로토콜 프레임을 나타낸다.
type Icp02Frame struct {
	Raw       []byte    // STX 부터 CRC 까지 전체 원시 바이트
	Timestamp time.Time // 수신 시각
	Length    int       // LEN 필드 값 (프레임 전체 길이)
	DA        []byte    // 목적지 주소
	SA        []byte    // 소스 주소
	CMD       [2]byte   // 명령 코드
	SEQ0      byte      // 명령 시퀀스 번호
	Payload   []byte    // 페이로드 데이터
	SEQ1      byte      // 프레임 시퀀스 번호
	CRCValid  bool      // CRC 검증 결과
	ParseErr  error     // 파싱 에러 (정상이면 nil)
}

// String 은 프레임의 요약 문자열을 반환한다.
func (f *Icp02Frame) String() string {
	if f.ParseErr != nil {
		return fmt.Sprintf("Icp02Frame{err=%v, raw=%s}", f.ParseErr, hex.EncodeToString(f.Raw))
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Icp02Frame{len=%d", f.Length)
	fmt.Fprintf(&sb, ", DA=%s", hex.EncodeToString(f.DA))
	fmt.Fprintf(&sb, ", SA=%s", hex.EncodeToString(f.SA))
	fmt.Fprintf(&sb, ", CMD=%02X%02X", f.CMD[0], f.CMD[1])
	fmt.Fprintf(&sb, ", SEQ0=%02X", f.SEQ0)
	fmt.Fprintf(&sb, ", PLEN=%d", len(f.Payload))
	fmt.Fprintf(&sb, ", SEQ1=%02X", f.SEQ1)
	fmt.Fprintf(&sb, ", CRC=%v}", f.CRCValid)
	return sb.String()
}

// ---------------------------------------------------------------------------
// LG ICP-02 프레임 파서
// ---------------------------------------------------------------------------

// Icp02FrameParser 는 io.Reader 에서 바이트를 읽어 LG ICP-02 프레임을 추출한다.
type Icp02FrameParser struct {
	reader    io.Reader
	verifyCRC bool
}

// NewIcp02FrameParser 는 새 프레임 파서를 생성한다.
// verifyCRC 가 true 이면 프레임의 CRC 를 검증한다.
func NewIcp02FrameParser(reader io.Reader, verifyCRC bool) *Icp02FrameParser {
	return &Icp02FrameParser{reader: reader, verifyCRC: verifyCRC}
}

// ReadFrame 은 스트림에서 하나의 완전한 LG ICP-02 프레임을 읽어 반환한다.
// STX(0x56) 바이트를 찾을 때까지 스캔한 뒤, LEN 바이트를 읽고
// 나머지 (LEN - 2) 바이트를 읽어 프레임을 구성한다.
// EOF 가 발생하면 io.EOF 를 반환한다.
func (p *Icp02FrameParser) ReadFrame() (*Icp02Frame, error) {
	// 1단계: STX 바이트 스캔
	buf := make([]byte, 1)
	for {
		_, err := io.ReadFull(p.reader, buf)
		if err != nil {
			return nil, err
		}
		if buf[0] == icp02STX {
			break
		}
	}

	// 2단계: LEN 바이트 읽기
	_, err := io.ReadFull(p.reader, buf)
	if err != nil {
		return nil, fmt.Errorf("lg_icp02: LEN 읽기 실패: %w", err)
	}
	frameLen := int(buf[0])

	// 3단계: LEN 유효성 검증
	// LEN 은 STX + LEN 자체를 포함한 전체 프레임 길이이다.
	if frameLen < icp02MinFrameLen {
		return &Icp02Frame{
			Raw:       []byte{icp02STX, buf[0]},
			Timestamp: time.Now(),
			Length:    frameLen,
			ParseErr:  fmt.Errorf("lg_icp02: LEN 값이 너무 작음: %d (최소 %d)", frameLen, icp02MinFrameLen),
		}, nil
	}

	// 4단계: 나머지 바이트 읽기 (LEN - 2: STX + LEN 이미 읽음)
	remaining := frameLen - 2
	rest := make([]byte, remaining)
	_, err = io.ReadFull(p.reader, rest)
	if err != nil {
		return nil, fmt.Errorf("lg_icp02: 프레임 데이터 읽기 실패 (%d 바이트 필요): %w", remaining, err)
	}

	// 전체 프레임 구성
	frame := make([]byte, frameLen)
	frame[0] = icp02STX
	frame[1] = byte(frameLen)
	copy(frame[2:], rest)

	// 5단계: 프레임 파싱
	return p.parseFrame(frame), nil
}

// parseFrame 은 전체 프레임 바이트를 파싱하여 Icp02Frame 을 반환한다.
func (p *Icp02FrameParser) parseFrame(frame []byte) *Icp02Frame {
	f := &Icp02Frame{
		Raw:       frame,
		Timestamp: time.Now(),
		Length:    int(frame[1]),
	}

	// CRC 검증
	if p.verifyCRC {
		f.CRCValid = VerifyIcp02CRC(frame)
	} else {
		f.CRCValid = true // CRC 검증 비활성화 시 항상 true
	}

	// 헤더 필드 파싱
	// frame[0] = STX, frame[1] = LEN
	// frame[2] = DLEN
	pos := 2
	if pos >= len(frame)-2 { // CRC 2바이트 확보
		f.ParseErr = fmt.Errorf("lg_icp02: 프레임이 너무 짧아 DLEN 을 읽을 수 없음")
		return f
	}

	dlen := int(frame[pos])
	pos++

	// DA (목적지 주소)
	if pos+dlen > len(frame)-2 {
		f.ParseErr = fmt.Errorf("lg_icp02: DA 읽기 실패: DLEN=%d, 남은 바이트 부족", dlen)
		return f
	}
	f.DA = make([]byte, dlen)
	copy(f.DA, frame[pos:pos+dlen])
	pos += dlen

	// SLEN
	if pos >= len(frame)-2 {
		f.ParseErr = fmt.Errorf("lg_icp02: 프레임이 너무 짧아 SLEN 을 읽을 수 없음")
		return f
	}
	slen := int(frame[pos])
	pos++

	// SA (소스 주소)
	if pos+slen > len(frame)-2 {
		f.ParseErr = fmt.Errorf("lg_icp02: SA 읽기 실패: SLEN=%d, 남은 바이트 부족", slen)
		return f
	}
	f.SA = make([]byte, slen)
	copy(f.SA, frame[pos:pos+slen])
	pos += slen

	// CMD (2바이트)
	if pos+2 > len(frame)-2 {
		f.ParseErr = fmt.Errorf("lg_icp02: CMD 읽기 실패: 남은 바이트 부족")
		return f
	}
	f.CMD[0] = frame[pos]
	f.CMD[1] = frame[pos+1]
	pos += 2

	// SEQ0 (1바이트)
	if pos >= len(frame)-2 {
		f.ParseErr = fmt.Errorf("lg_icp02: SEQ0 읽기 실패: 남은 바이트 부족")
		return f
	}
	f.SEQ0 = frame[pos]
	pos++

	// PLEN (1바이트)
	if pos >= len(frame)-2 {
		f.ParseErr = fmt.Errorf("lg_icp02: PLEN 읽기 실패: 남은 바이트 부족")
		return f
	}
	plen := int(frame[pos])
	pos++

	// PAYLOAD (PLEN 바이트)
	if pos+plen > len(frame)-3 { // SEQ1(1) + CRC(2) 필요
		f.ParseErr = fmt.Errorf("lg_icp02: 페이로드 읽기 실패: PLEN=%d, 남은 바이트 부족", plen)
		return f
	}
	if plen > 0 {
		f.Payload = make([]byte, plen)
		copy(f.Payload, frame[pos:pos+plen])
	}
	pos += plen

	// SEQ1 (1바이트)
	if pos >= len(frame)-2 {
		f.ParseErr = fmt.Errorf("lg_icp02: SEQ1 읽기 실패: 남은 바이트 부족")
		return f
	}
	f.SEQ1 = frame[pos]

	return f
}

// ParseSingleFrame 은 완전한 프레임 바이트 슬라이스를 파싱하여 Icp02Frame 을 반환한다.
// 스트림이 아닌 단일 프레임을 파싱할 때 사용한다.
func ParseSingleFrame(frame []byte, verifyCRC bool) *Icp02Frame {
	if len(frame) < 2 {
		return &Icp02Frame{
			Raw:       frame,
			Timestamp: time.Now(),
			ParseErr:  fmt.Errorf("lg_icp02: 프레임이 너무 짧음: %d 바이트", len(frame)),
		}
	}
	if frame[0] != icp02STX {
		return &Icp02Frame{
			Raw:       frame,
			Timestamp: time.Now(),
			ParseErr:  fmt.Errorf("lg_icp02: 잘못된 STX: 0x%02X (예상 0x%02X)", frame[0], icp02STX),
		}
	}

	parser := &Icp02FrameParser{verifyCRC: verifyCRC}
	return parser.parseFrame(frame)
}
