package samsung

import (
	"encoding/binary"
	"fmt"
)

// NasaProtocol 은 Samsung NASA HVAC 프로토콜의 인코딩/디코딩 인터페이스이다.
type NasaProtocol interface {
	// Encode 는 NasaMessage 를 바이트 프레임으로 인코딩한다.
	Encode(msg *NasaMessage) ([]byte, error)

	// Decode 는 바이트 프레임을 NasaMessage 로 디코딩한다.
	Decode(data []byte) (*NasaMessage, error)

	// BuildStatusQuery 는 지정된 주소로 C011 상태 쿼리 프레임을 생성한다.
	BuildStatusQuery(addr NasaAddress, seqNum byte) ([]byte, error)

	// BuildControlCommand 는 지정된 주소로 C013 제어 명령 프레임을 생성한다.
	BuildControlCommand(addr NasaAddress, seqNum byte, sets []NasaMessageSet) ([]byte, error)

	// CalculateChecksum 은 데이터의 CRC16 체크섬을 계산한다.
	CalculateChecksum(data []byte) uint16

	// ParseMessageSets 는 바이트 데이터에서 count 개의 메시지 세트를 파싱한다.
	ParseMessageSets(data []byte, count int) ([]NasaMessageSet, error)

	// EncodeMessageSets 는 메시지 세트 목록을 바이트로 인코딩한다.
	EncodeMessageSets(sets []NasaMessageSet) []byte
}

// nasaProtocol 은 NasaProtocol 인터페이스의 상태 없는 구현체이다.
type nasaProtocol struct{}

// NewNasaProtocol 은 NasaProtocol 인터페이스의 새 인스턴스를 반환한다.
func NewNasaProtocol() NasaProtocol {
	return &nasaProtocol{}
}

// EncodeMessageSets 는 메시지 세트 목록을 바이트로 인코딩한다.
// 각 세트는 Index(2바이트 BE) + Value 바이트로 구성된다.
func (p *nasaProtocol) EncodeMessageSets(sets []NasaMessageSet) []byte {
	// 필요한 총 바이트 수를 미리 계산하여 할당을 최소화한다.
	totalSize := 0
	for _, ms := range sets {
		totalSize += 2 + len(ms.Value) // Index(2) + Value
	}
	buf := make([]byte, 0, totalSize)
	for _, ms := range sets {
		buf = append(buf, byte(ms.Index>>8), byte(ms.Index&0xFF))
		buf = append(buf, ms.Value...)
	}
	return buf
}

// ParseMessageSets 는 바이트 데이터에서 count 개의 메시지 세트를 파싱한다.
func (p *nasaProtocol) ParseMessageSets(data []byte, count int) ([]NasaMessageSet, error) {
	sets := make([]NasaMessageSet, 0, count)
	offset := 0

	for i := 0; i < count; i++ {
		// 인덱스 2바이트 읽기
		if offset+2 > len(data) {
			return nil, fmt.Errorf("%w: 인덱스 읽기 위한 데이터 부족 (offset=%d, len=%d)", ErrProtocolParseFailed, offset, len(data))
		}
		index := binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2

		// 값 크기 결정
		size := MessageSetValueSize(index)
		if size < 0 {
			return nil, fmt.Errorf("%w: index=0x%04X", ErrInvalidMessageSetIndex, index)
		}

		// 값 바이트 읽기
		if offset+size > len(data) {
			return nil, fmt.Errorf("%w: 값 읽기 위한 데이터 부족 (index=0x%04X, need=%d, remain=%d)",
				ErrProtocolParseFailed, index, size, len(data)-offset)
		}

		value := make([]byte, size)
		copy(value, data[offset:offset+size])
		offset += size

		sets = append(sets, NasaMessageSet{Index: index, Value: value})
	}

	return sets, nil
}

// CalculateChecksum 은 데이터의 CRC16-CCITT 체크섬을 계산한다.
func (p *nasaProtocol) CalculateChecksum(data []byte) uint16 {
	return CalcCRC16(data)
}

// Encode 는 NasaMessage 를 NASA 프로토콜 프레임으로 인코딩한다.
//
// 프레임 구조:
//
//	[STX=0x32][LEN(2 BE)][SA(3)][DA(3)][CMD(2)][SEQ#(1)][CNT(1)][MSGs...][CRC(2 BE)][ETX=0x34]
func (p *nasaProtocol) Encode(msg *NasaMessage) ([]byte, error) {
	// 1. 메시지 세트 인코딩
	msgBytes := p.EncodeMessageSets(msg.MessageSets)

	// 2. 프레임 바디 구성: SA(3) + DA(3) + CMD(2) + SEQ#(1) + CNT(1) + 메시지 세트
	bodyLen := HeaderSize + len(msgBytes)
	body := make([]byte, bodyLen)

	// SA (3바이트)
	copy(body[0:3], msg.SourceAddr[:])
	// DA (3바이트)
	copy(body[3:6], msg.DestAddr[:])
	// CMD (2바이트 BE)
	binary.BigEndian.PutUint16(body[6:8], msg.CommandCode)
	// SEQ# (1바이트)
	body[8] = msg.SequenceNum
	// CNT (1바이트)
	body[9] = byte(len(msg.MessageSets))
	// 메시지 세트 바이트
	copy(body[HeaderSize:], msgBytes)

	// 3. CRC 계산 (SA ~ MSGs 영역)
	crc := CalcCRC16(body)

	// 4. LEN 계산: 2(LEN 자체) + len(body) + 2(CRC)
	frameLen := uint16(2 + bodyLen + 2)

	// 5. 전체 프레임 조립: STX + LEN(2 BE) + body + CRC(2 BE) + ETX
	totalSize := 1 + 2 + bodyLen + 2 + 1
	frame := make([]byte, totalSize)

	frame[0] = FrameSTX
	binary.BigEndian.PutUint16(frame[1:3], frameLen)
	copy(frame[3:3+bodyLen], body)
	binary.BigEndian.PutUint16(frame[3+bodyLen:3+bodyLen+2], crc)
	frame[totalSize-1] = FrameETX

	// 6. msg 에 체크섬과 원본 프레임 설정
	msg.Checksum = crc
	msg.Raw = frame

	return frame, nil
}

// Decode 는 NASA 프로토콜 프레임을 NasaMessage 로 디코딩한다.
func (p *nasaProtocol) Decode(data []byte) (*NasaMessage, error) {
	// 프레임 내 고정 오프셋 상수
	const (
		offSTX    = 0  // STX 위치
		offLEN    = 1  // LEN 시작 위치 (2바이트)
		offSA     = 3  // SA 시작 위치 (3바이트)
		offDA     = 6  // DA 시작 위치 (3바이트)
		offCMD    = 9  // CMD 시작 위치 (2바이트)
		offSEQ    = 11 // SEQ# 위치 (1바이트)
		offCNT    = 12 // CNT 위치 (1바이트)
		offMsgSet = 13 // 메시지 세트 시작 위치

		crcSize = 2 // CRC 바이트 수
		etxSize = 1 // ETX 바이트 수
	)

	// 1. 최소 프레임 크기 확인
	if len(data) < MinFrameSize {
		return nil, ErrInvalidFrameLength
	}

	// 2. STX 확인
	if data[offSTX] != FrameSTX {
		return nil, ErrInvalidFrameSTX
	}

	// 3. LEN 읽기 및 총 프레임 길이 검증
	frameLen := binary.BigEndian.Uint16(data[offLEN : offLEN+2])
	// 총 프레임 = STX(1) + LEN 값 + ETX(1)
	expectedTotal := 1 + int(frameLen) + 1
	if len(data) != expectedTotal {
		return nil, ErrInvalidFrameLength
	}

	// 4. ETX 확인
	if data[len(data)-1] != FrameETX {
		return nil, ErrInvalidFrameETX
	}

	// 5. 헤더 필드 추출
	var sa, da NasaAddress
	copy(sa[:], data[offSA:offSA+3])
	copy(da[:], data[offDA:offDA+3])
	cmd := binary.BigEndian.Uint16(data[offCMD : offCMD+2])
	seqNum := data[offSEQ]
	cnt := int(data[offCNT])

	// 6. 메시지 세트 영역 추출 (CRC + ETX 앞까지)
	msgEnd := len(data) - crcSize - etxSize
	msgData := data[offMsgSet:msgEnd]

	// 7. 메시지 세트 파싱
	sets, err := p.ParseMessageSets(msgData, cnt)
	if err != nil {
		return nil, err
	}

	// 8. CRC 검증: SA ~ MSGs 영역 (data[offSA:msgEnd])
	crcBody := data[offSA:msgEnd]
	expectedCRC := binary.BigEndian.Uint16(data[msgEnd : msgEnd+crcSize])
	calculatedCRC := CalcCRC16(crcBody)
	if expectedCRC != calculatedCRC {
		return nil, ErrChecksumMismatch
	}

	// 9. Raw 복사
	raw := make([]byte, len(data))
	copy(raw, data)

	return &NasaMessage{
		SourceAddr:  sa,
		DestAddr:    da,
		CommandCode: cmd,
		SequenceNum: seqNum,
		MessageSets: sets,
		Checksum:    expectedCRC,
		Raw:         raw,
	}, nil
}

// BuildStatusQuery 는 C011 상태 쿼리 프레임을 생성한다.
// SA = AddrController, DA = addr, CMD = CmdNormalRequest.
// 메시지 세트: MsgAddrInfo(0x0408) = [0xFF, 0xFF, 0xFF, 0xFF].
func (p *nasaProtocol) BuildStatusQuery(addr NasaAddress, seqNum byte) ([]byte, error) {
	msg := &NasaMessage{
		SourceAddr:  AddrController,
		DestAddr:    addr,
		CommandCode: CmdNormalRequest,
		SequenceNum: seqNum,
		MessageSets: []NasaMessageSet{
			{Index: MsgAddrInfo, Value: []byte{0xFF, 0xFF, 0xFF, 0xFF}},
		},
	}
	return p.Encode(msg)
}

// BuildControlCommand 는 C013 제어 명령 프레임을 생성한다.
// SA = AddrController, DA = addr, CMD = CmdNormalControl.
func (p *nasaProtocol) BuildControlCommand(addr NasaAddress, seqNum byte, sets []NasaMessageSet) ([]byte, error) {
	msg := &NasaMessage{
		SourceAddr:  AddrController,
		DestAddr:    addr,
		CommandCode: CmdNormalControl,
		SequenceNum: seqNum,
		MessageSets: sets,
	}
	return p.Encode(msg)
}
