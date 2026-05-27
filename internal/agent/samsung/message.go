package samsung

// ---------------------------------------------------------------------------
// 프레임 상수
// ---------------------------------------------------------------------------

const (
	// FrameSTX 는 NASA 프로토콜 프레임의 시작 바이트이다.
	FrameSTX byte = 0x32

	// FrameETX 는 NASA 프로토콜 프레임의 종료 바이트이다.
	FrameETX byte = 0x34

	// MinFrameSize 는 최소 프레임 크기이다.
	// STX(1) + LEN(2) + SA(3) + DA(3) + CMD(2) + SEQ(1) + CNT(1) + CRC(2) + ETX(1) = 16
	MinFrameSize = 16

	// HeaderSize 는 헤더 크기이다.
	// SA(3) + DA(3) + CMD(2) + SEQ(1) + CNT(1) = 10
	HeaderSize = 10
)

// ---------------------------------------------------------------------------
// 명령 코드 상수 (REQ-03-02-02)
// ---------------------------------------------------------------------------

const (
	CmdStandbyRequest  uint16 = 0xC001
	CmdStandbyResponse uint16 = 0xC005
	CmdNormalRequest   uint16 = 0xC011
	CmdNormalSetting   uint16 = 0xC012
	CmdNormalControl   uint16 = 0xC013
	CmdNotification    uint16 = 0xC014
	CmdAddressResponse uint16 = 0xC015
	CmdControlResponse uint16 = 0xC016
)

// ---------------------------------------------------------------------------
// 메시지 인덱스 상수 (REQ-03-02-03)
// ---------------------------------------------------------------------------

const (
	MsgPower            uint16 = 0x4000
	MsgMode             uint16 = 0x4001
	MsgFanSpeed         uint16 = 0x4006
	MsgLongWind         uint16 = 0x4007
	MsgSwingVertical    uint16 = 0x4011
	MsgFilterCleanReset uint16 = 0x4025
	MsgFilterCleanAlarm uint16 = 0x4027
	MsgAirPurifier      uint16 = 0x4043
	MsgBuzzer           uint16 = 0x4050
	MsgWindless         uint16 = 0x4060
	MsgSwingHorizontal  uint16 = 0x407E
	MsgAutoDry          uint16 = 0x4111
	MsgTargetTemp       uint16 = 0x4201
	MsgCurrentTemp      uint16 = 0x4203
	MsgErrorCode        uint16 = 0x0202
	MsgRemoteLimit      uint16 = 0x0409
	MsgAddrInfo         uint16 = 0x0408
	MsgAddrRegister     uint16 = 0x2004
	MsgReadyState       uint16 = 0x2010
)

// ---------------------------------------------------------------------------
// NasaMessageSet 는 NASA 프로토콜의 개별 메시지 세트를 나타낸다 (REQ-03-02-01).
// ---------------------------------------------------------------------------

// NasaMessageSet 는 메시지 인덱스와 값의 쌍이다.
type NasaMessageSet struct {
	Index uint16 // 메시지 인덱스 (2바이트)
	Value []byte // 메시지 값 (크기는 Index의 2번째 니블로 결정)
}

// ---------------------------------------------------------------------------
// NasaMessage 는 NASA 프로토콜의 전체 메시지를 나타낸다 (REQ-03-02).
// ---------------------------------------------------------------------------

// NasaMessage 는 파싱된 NASA 프로토콜 메시지이다.
type NasaMessage struct {
	SourceAddr  NasaAddress      // 발신 주소 (3바이트)
	DestAddr    NasaAddress      // 수신 주소 (3바이트)
	CommandCode uint16           // 명령 코드 (2바이트)
	SequenceNum byte             // 시퀀스 번호 (1바이트)
	MessageSets []NasaMessageSet // 메시지 세트 목록
	Checksum    uint16           // CRC16 체크섬 (2바이트)
	Raw         []byte           // 원본 바이트 (STX~ETX 포함)
}

// ---------------------------------------------------------------------------
// MessageSetValueSize 는 주어진 Index에 대한 예상 Value 크기를 반환한다.
// Index의 2번째 니블(상위 바이트의 하위 4비트)에 따라 크기가 결정된다.
// 가변 길이(니블 6)이거나 알 수 없는 니블 값인 경우 -1을 반환한다.
// ---------------------------------------------------------------------------
func MessageSetValueSize(index uint16) int {
	nibble := (index >> 8) & 0x0F
	switch nibble {
	case 0, 1:
		return 1
	case 2:
		return 2
	case 4:
		return 4
	default:
		return -1
	}
}
