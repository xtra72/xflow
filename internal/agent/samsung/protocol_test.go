package samsung

import (
	"bytes"
	"errors"
	"testing"
)

// ---------------------------------------------------------------------------
// 헬퍼: 참조 프레임 바이트 (프로토콜 스펙 기준)
// Frame: 32 0015 620000 200000 C013 A8 02 400001 42010118 CD4D 34
// ---------------------------------------------------------------------------

var refFrame = []byte{
	0x32,                   // STX
	0x00, 0x15,             // LEN = 21
	0x62, 0x00, 0x00,       // SA
	0x20, 0x00, 0x00,       // DA
	0xC0, 0x13,             // CMD = CmdNormalControl
	0xA8,                   // SEQ#
	0x02,                   // CNT = 2
	0x40, 0x00, 0x01,       // MSG1: index=0x4000, value=[0x01]
	0x42, 0x01, 0x01, 0x18, // MSG2: index=0x4201, value=[0x01, 0x18]
	0xCD, 0x4D,             // CRC16
	0x34,                   // ETX
}

// refMessage 는 참조 프레임에 대응하는 NasaMessage 이다.
var refMessage = NasaMessage{
	SourceAddr:  NasaAddress{0x62, 0x00, 0x00},
	DestAddr:    NasaAddress{0x20, 0x00, 0x00},
	CommandCode: CmdNormalControl,
	SequenceNum: 0xA8,
	MessageSets: []NasaMessageSet{
		{Index: 0x4000, Value: []byte{0x01}},
		{Index: 0x4201, Value: []byte{0x01, 0x18}},
	},
	Checksum: 0xCD4D,
}

// ===========================================================================
// NewNasaProtocol
// ===========================================================================

func TestNewNasaProtocol(t *testing.T) {
	p := NewNasaProtocol()
	if p == nil {
		t.Fatal("NewNasaProtocol() returned nil")
	}
}

// ===========================================================================
// EncodeMessageSets / ParseMessageSets
// ===========================================================================

func TestEncodeMessageSets(t *testing.T) {
	p := NewNasaProtocol()

	tests := []struct {
		name string
		sets []NasaMessageSet
		want []byte
	}{
		{
			name: "두 개의 메시지 세트 인코딩",
			sets: []NasaMessageSet{
				{Index: 0x4000, Value: []byte{0x01}},
				{Index: 0x4201, Value: []byte{0x01, 0x18}},
			},
			want: []byte{0x40, 0x00, 0x01, 0x42, 0x01, 0x01, 0x18},
		},
		{
			name: "빈 메시지 세트 인코딩",
			sets: []NasaMessageSet{},
			want: []byte{},
		},
		{
			name: "단일 1바이트 메시지 세트",
			sets: []NasaMessageSet{
				{Index: 0x4001, Value: []byte{0x03}},
			},
			want: []byte{0x40, 0x01, 0x03},
		},
		{
			name: "4바이트 값 메시지 세트",
			sets: []NasaMessageSet{
				{Index: 0x0408, Value: []byte{0xFF, 0xFF, 0xFF, 0xFF}},
			},
			want: []byte{0x04, 0x08, 0xFF, 0xFF, 0xFF, 0xFF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.EncodeMessageSets(tt.sets)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("EncodeMessageSets() = %X, want %X", got, tt.want)
			}
		})
	}
}

func TestParseMessageSets(t *testing.T) {
	p := NewNasaProtocol()

	t.Run("참조 프레임의 2개 메시지 세트 파싱", func(t *testing.T) {
		data := []byte{0x40, 0x00, 0x01, 0x42, 0x01, 0x01, 0x18}
		sets, err := p.ParseMessageSets(data, 2)
		if err != nil {
			t.Fatalf("ParseMessageSets() error = %v", err)
		}
		if len(sets) != 2 {
			t.Fatalf("ParseMessageSets() len = %d, want 2", len(sets))
		}

		// MSG1
		if sets[0].Index != 0x4000 {
			t.Errorf("sets[0].Index = 0x%04X, want 0x4000", sets[0].Index)
		}
		if !bytes.Equal(sets[0].Value, []byte{0x01}) {
			t.Errorf("sets[0].Value = %X, want [01]", sets[0].Value)
		}

		// MSG2
		if sets[1].Index != 0x4201 {
			t.Errorf("sets[1].Index = 0x%04X, want 0x4201", sets[1].Index)
		}
		if !bytes.Equal(sets[1].Value, []byte{0x01, 0x18}) {
			t.Errorf("sets[1].Value = %X, want [01 18]", sets[1].Value)
		}
	})

	t.Run("빈 데이터에서 0개 파싱", func(t *testing.T) {
		sets, err := p.ParseMessageSets([]byte{}, 0)
		if err != nil {
			t.Fatalf("ParseMessageSets() error = %v", err)
		}
		if len(sets) != 0 {
			t.Errorf("ParseMessageSets() len = %d, want 0", len(sets))
		}
	})

	t.Run("잘린 데이터는 에러 반환", func(t *testing.T) {
		// 인덱스 2바이트만 있고 값이 없는 경우
		data := []byte{0x40, 0x00}
		_, err := p.ParseMessageSets(data, 1)
		if err == nil {
			t.Error("ParseMessageSets() expected error for truncated data")
		}
	})

	t.Run("인덱스조차 읽기 전에 데이터 부족", func(t *testing.T) {
		data := []byte{0x40}
		_, err := p.ParseMessageSets(data, 1)
		if err == nil {
			t.Error("ParseMessageSets() expected error for insufficient data")
		}
	})

	t.Run("가변 길이 인덱스는 에러 반환", func(t *testing.T) {
		// 니블 6 → -1 (가변 길이)
		data := []byte{0x06, 0x00, 0x01}
		_, err := p.ParseMessageSets(data, 1)
		if !errors.Is(err, ErrInvalidMessageSetIndex) {
			t.Errorf("ParseMessageSets() error = %v, want %v", err, ErrInvalidMessageSetIndex)
		}
	})
}

// ===========================================================================
// CalculateChecksum
// ===========================================================================

func TestCalculateChecksum(t *testing.T) {
	p := NewNasaProtocol()

	// 참조 프레임의 SA~MSGs 영역
	body := []byte{
		0x62, 0x00, 0x00, // SA
		0x20, 0x00, 0x00, // DA
		0xC0, 0x13, // CMD
		0xA8,       // SEQ#
		0x02,       // CNT
		0x40, 0x00, 0x01, // MSG1
		0x42, 0x01, 0x01, 0x18, // MSG2
	}

	got := p.CalculateChecksum(body)
	if got != 0xCD4D {
		t.Errorf("CalculateChecksum() = 0x%04X, want 0xCD4D", got)
	}
}

// ===========================================================================
// Encode
// ===========================================================================

func TestEncode(t *testing.T) {
	p := NewNasaProtocol()

	t.Run("참조 메시지 인코딩", func(t *testing.T) {
		msg := &NasaMessage{
			SourceAddr:  NasaAddress{0x62, 0x00, 0x00},
			DestAddr:    NasaAddress{0x20, 0x00, 0x00},
			CommandCode: CmdNormalControl,
			SequenceNum: 0xA8,
			MessageSets: []NasaMessageSet{
				{Index: 0x4000, Value: []byte{0x01}},
				{Index: 0x4201, Value: []byte{0x01, 0x18}},
			},
		}

		data, err := p.Encode(msg)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		if !bytes.Equal(data, refFrame) {
			t.Errorf("Encode() =\n  %X\nwant:\n  %X", data, refFrame)
		}

		// Encode 후 msg.Checksum 과 msg.Raw 가 설정되어야 한다
		if msg.Checksum != 0xCD4D {
			t.Errorf("msg.Checksum = 0x%04X, want 0xCD4D", msg.Checksum)
		}
		if !bytes.Equal(msg.Raw, refFrame) {
			t.Errorf("msg.Raw =\n  %X\nwant:\n  %X", msg.Raw, refFrame)
		}
	})

	t.Run("메시지 세트 없는 메시지 인코딩", func(t *testing.T) {
		msg := &NasaMessage{
			SourceAddr:  AddrController,
			DestAddr:    NasaAddress{0x20, 0x00, 0x01},
			CommandCode: CmdNormalRequest,
			SequenceNum: 0x01,
			MessageSets: []NasaMessageSet{},
		}

		data, err := p.Encode(msg)
		if err != nil {
			t.Fatalf("Encode() error = %v", err)
		}

		// MinFrameSize = 16 바이트
		if len(data) != MinFrameSize {
			t.Errorf("Encode() frame length = %d, want %d", len(data), MinFrameSize)
		}

		// STX 와 ETX 확인
		if data[0] != FrameSTX {
			t.Errorf("data[0] = 0x%02X, want STX (0x%02X)", data[0], FrameSTX)
		}
		if data[len(data)-1] != FrameETX {
			t.Errorf("data[last] = 0x%02X, want ETX (0x%02X)", data[len(data)-1], FrameETX)
		}
	})
}

// ===========================================================================
// Decode
// ===========================================================================

func TestDecode(t *testing.T) {
	p := NewNasaProtocol()

	t.Run("참조 프레임 디코딩", func(t *testing.T) {
		msg, err := p.Decode(refFrame)
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}

		if msg.SourceAddr != (NasaAddress{0x62, 0x00, 0x00}) {
			t.Errorf("SourceAddr = %v, want 62 00 00", msg.SourceAddr)
		}
		if msg.DestAddr != (NasaAddress{0x20, 0x00, 0x00}) {
			t.Errorf("DestAddr = %v, want 20 00 00", msg.DestAddr)
		}
		if msg.CommandCode != CmdNormalControl {
			t.Errorf("CommandCode = 0x%04X, want 0x%04X", msg.CommandCode, CmdNormalControl)
		}
		if msg.SequenceNum != 0xA8 {
			t.Errorf("SequenceNum = 0x%02X, want 0xA8", msg.SequenceNum)
		}
		if len(msg.MessageSets) != 2 {
			t.Fatalf("MessageSets len = %d, want 2", len(msg.MessageSets))
		}

		// MSG1
		if msg.MessageSets[0].Index != 0x4000 {
			t.Errorf("MessageSets[0].Index = 0x%04X, want 0x4000", msg.MessageSets[0].Index)
		}
		if !bytes.Equal(msg.MessageSets[0].Value, []byte{0x01}) {
			t.Errorf("MessageSets[0].Value = %X, want [01]", msg.MessageSets[0].Value)
		}

		// MSG2
		if msg.MessageSets[1].Index != 0x4201 {
			t.Errorf("MessageSets[1].Index = 0x%04X, want 0x4201", msg.MessageSets[1].Index)
		}
		if !bytes.Equal(msg.MessageSets[1].Value, []byte{0x01, 0x18}) {
			t.Errorf("MessageSets[1].Value = %X, want [01 18]", msg.MessageSets[1].Value)
		}

		if msg.Checksum != 0xCD4D {
			t.Errorf("Checksum = 0x%04X, want 0xCD4D", msg.Checksum)
		}
		if !bytes.Equal(msg.Raw, refFrame) {
			t.Errorf("Raw 가 원본 프레임과 다름")
		}
	})
}

func TestDecodeErrors(t *testing.T) {
	p := NewNasaProtocol()

	tests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{
			name:    "빈 데이터",
			data:    []byte{},
			wantErr: ErrInvalidFrameLength,
		},
		{
			name:    "프레임 최소 크기 미달",
			data:    make([]byte, MinFrameSize-1),
			wantErr: ErrInvalidFrameLength,
		},
		{
			name: "잘못된 STX",
			data: func() []byte {
				d := make([]byte, len(refFrame))
				copy(d, refFrame)
				d[0] = 0xFF
				return d
			}(),
			wantErr: ErrInvalidFrameSTX,
		},
		{
			name: "잘못된 ETX",
			data: func() []byte {
				d := make([]byte, len(refFrame))
				copy(d, refFrame)
				d[len(d)-1] = 0xFF
				return d
			}(),
			wantErr: ErrInvalidFrameETX,
		},
		{
			name: "LEN 불일치 (너무 큰 값)",
			data: func() []byte {
				d := make([]byte, len(refFrame))
				copy(d, refFrame)
				// LEN 을 0x00FF 로 변경 (실제 데이터보다 큼)
				d[1] = 0x00
				d[2] = 0xFF
				return d
			}(),
			wantErr: ErrInvalidFrameLength,
		},
		{
			name: "CRC 불일치",
			data: func() []byte {
				d := make([]byte, len(refFrame))
				copy(d, refFrame)
				// CRC 위치를 변조
				d[len(d)-3] = 0xFF
				d[len(d)-2] = 0xFF
				return d
			}(),
			wantErr: ErrChecksumMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.Decode(tt.data)
			if err == nil {
				t.Fatalf("Decode() expected error %v, got nil", tt.wantErr)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Decode() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// ===========================================================================
// Encode/Decode 라운드 트립
// ===========================================================================

func TestEncodeDecodeRoundTrip(t *testing.T) {
	p := NewNasaProtocol()

	tests := []struct {
		name string
		msg  NasaMessage
	}{
		{
			name: "참조 메시지 라운드 트립",
			msg: NasaMessage{
				SourceAddr:  NasaAddress{0x62, 0x00, 0x00},
				DestAddr:    NasaAddress{0x20, 0x00, 0x00},
				CommandCode: CmdNormalControl,
				SequenceNum: 0xA8,
				MessageSets: []NasaMessageSet{
					{Index: 0x4000, Value: []byte{0x01}},
					{Index: 0x4201, Value: []byte{0x01, 0x18}},
				},
			},
		},
		{
			name: "메시지 세트 없는 라운드 트립",
			msg: NasaMessage{
				SourceAddr:  AddrController,
				DestAddr:    NasaAddress{0x20, 0x00, 0x01},
				CommandCode: CmdNormalRequest,
				SequenceNum: 0x01,
				MessageSets: []NasaMessageSet{},
			},
		},
		{
			name: "4바이트 메시지 세트 라운드 트립",
			msg: NasaMessage{
				SourceAddr:  AddrController,
				DestAddr:    NasaAddress{0x20, 0x00, 0x00},
				CommandCode: CmdNormalRequest,
				SequenceNum: 0x55,
				MessageSets: []NasaMessageSet{
					{Index: MsgAddrInfo, Value: []byte{0xFF, 0xFF, 0xFF, 0xFF}},
				},
			},
		},
		{
			name: "여러 메시지 세트 혼합 라운드 트립",
			msg: NasaMessage{
				SourceAddr:  AddrController,
				DestAddr:    NasaAddress{0x20, 0x00, 0x02},
				CommandCode: CmdNormalControl,
				SequenceNum: 0x10,
				MessageSets: []NasaMessageSet{
					{Index: MsgPower, Value: []byte{0x01}},
					{Index: MsgMode, Value: []byte{0x03}},
					{Index: MsgTargetTemp, Value: []byte{0x00, 0xE6}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 인코딩
			encoded, err := p.Encode(&tt.msg)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			// 디코딩
			decoded, err := p.Decode(encoded)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			// 필드 비교
			if decoded.SourceAddr != tt.msg.SourceAddr {
				t.Errorf("SourceAddr = %v, want %v", decoded.SourceAddr, tt.msg.SourceAddr)
			}
			if decoded.DestAddr != tt.msg.DestAddr {
				t.Errorf("DestAddr = %v, want %v", decoded.DestAddr, tt.msg.DestAddr)
			}
			if decoded.CommandCode != tt.msg.CommandCode {
				t.Errorf("CommandCode = 0x%04X, want 0x%04X", decoded.CommandCode, tt.msg.CommandCode)
			}
			if decoded.SequenceNum != tt.msg.SequenceNum {
				t.Errorf("SequenceNum = 0x%02X, want 0x%02X", decoded.SequenceNum, tt.msg.SequenceNum)
			}
			if len(decoded.MessageSets) != len(tt.msg.MessageSets) {
				t.Fatalf("MessageSets len = %d, want %d", len(decoded.MessageSets), len(tt.msg.MessageSets))
			}
			for i, ms := range decoded.MessageSets {
				if ms.Index != tt.msg.MessageSets[i].Index {
					t.Errorf("MessageSets[%d].Index = 0x%04X, want 0x%04X", i, ms.Index, tt.msg.MessageSets[i].Index)
				}
				if !bytes.Equal(ms.Value, tt.msg.MessageSets[i].Value) {
					t.Errorf("MessageSets[%d].Value = %X, want %X", i, ms.Value, tt.msg.MessageSets[i].Value)
				}
			}
		})
	}
}

// ===========================================================================
// BuildStatusQuery
// ===========================================================================

func TestBuildStatusQuery(t *testing.T) {
	p := NewNasaProtocol()

	tests := []struct {
		name   string
		addr   NasaAddress
		seqNum byte
	}{
		{
			name:   "실내기 주소로 상태 쿼리 생성",
			addr:   NasaAddress{0x20, 0x00, 0x01},
			seqNum: 0x01,
		},
		{
			name:   "실외기 주소로 상태 쿼리 생성",
			addr:   NasaAddress{0x10, 0x00, 0x00},
			seqNum: 0xFF,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame, err := p.BuildStatusQuery(tt.addr, tt.seqNum)
			if err != nil {
				t.Fatalf("BuildStatusQuery() error = %v", err)
			}

			// STX/ETX 확인
			if frame[0] != FrameSTX {
				t.Errorf("frame[0] = 0x%02X, want STX (0x%02X)", frame[0], FrameSTX)
			}
			if frame[len(frame)-1] != FrameETX {
				t.Errorf("frame[last] = 0x%02X, want ETX (0x%02X)", frame[len(frame)-1], FrameETX)
			}

			// 디코딩하여 필드 검증
			msg, err := p.Decode(frame)
			if err != nil {
				t.Fatalf("Decode(BuildStatusQuery result) error = %v", err)
			}

			// SA = AddrController
			if msg.SourceAddr != AddrController {
				t.Errorf("SourceAddr = %v, want %v", msg.SourceAddr, AddrController)
			}
			// DA = 대상 주소
			if msg.DestAddr != tt.addr {
				t.Errorf("DestAddr = %v, want %v", msg.DestAddr, tt.addr)
			}
			// CMD = CmdNormalRequest (0xC011)
			if msg.CommandCode != CmdNormalRequest {
				t.Errorf("CommandCode = 0x%04X, want 0x%04X", msg.CommandCode, CmdNormalRequest)
			}
			// SEQ#
			if msg.SequenceNum != tt.seqNum {
				t.Errorf("SequenceNum = 0x%02X, want 0x%02X", msg.SequenceNum, tt.seqNum)
			}
			// 메시지 세트 1개: MsgAddrInfo = 0x0408, Value = [FF FF FF FF]
			if len(msg.MessageSets) != 1 {
				t.Fatalf("MessageSets len = %d, want 1", len(msg.MessageSets))
			}
			if msg.MessageSets[0].Index != MsgAddrInfo {
				t.Errorf("MessageSets[0].Index = 0x%04X, want 0x%04X", msg.MessageSets[0].Index, MsgAddrInfo)
			}
			if !bytes.Equal(msg.MessageSets[0].Value, []byte{0xFF, 0xFF, 0xFF, 0xFF}) {
				t.Errorf("MessageSets[0].Value = %X, want [FF FF FF FF]", msg.MessageSets[0].Value)
			}
		})
	}
}

// ===========================================================================
// BuildControlCommand
// ===========================================================================

func TestBuildControlCommand(t *testing.T) {
	p := NewNasaProtocol()

	tests := []struct {
		name   string
		addr   NasaAddress
		seqNum byte
		sets   []NasaMessageSet
	}{
		{
			name:   "단일 1바이트 메시지 세트로 제어 명령",
			addr:   NasaAddress{0x20, 0x00, 0x01},
			seqNum: 0x05,
			sets:   []NasaMessageSet{{Index: MsgPower, Value: []byte{0x01}}},
		},
		{
			name:   "여러 메시지 세트로 제어 명령",
			addr:   NasaAddress{0x20, 0x00, 0x02},
			seqNum: 0x0A,
			sets: []NasaMessageSet{
				{Index: MsgPower, Value: []byte{0x01}},
				{Index: MsgTargetTemp, Value: []byte{0x00, 0xE6}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame, err := p.BuildControlCommand(tt.addr, tt.seqNum, tt.sets)
			if err != nil {
				t.Fatalf("BuildControlCommand() error = %v", err)
			}

			// STX/ETX 확인
			if frame[0] != FrameSTX {
				t.Errorf("frame[0] = 0x%02X, want STX (0x%02X)", frame[0], FrameSTX)
			}
			if frame[len(frame)-1] != FrameETX {
				t.Errorf("frame[last] = 0x%02X, want ETX (0x%02X)", frame[len(frame)-1], FrameETX)
			}

			// 디코딩하여 필드 검증
			msg, err := p.Decode(frame)
			if err != nil {
				t.Fatalf("Decode(BuildControlCommand result) error = %v", err)
			}

			// SA = AddrController
			if msg.SourceAddr != AddrController {
				t.Errorf("SourceAddr = %v, want %v", msg.SourceAddr, AddrController)
			}
			// DA = 대상 주소
			if msg.DestAddr != tt.addr {
				t.Errorf("DestAddr = %v, want %v", msg.DestAddr, tt.addr)
			}
			// CMD = CmdNormalControl (0xC013)
			if msg.CommandCode != CmdNormalControl {
				t.Errorf("CommandCode = 0x%04X, want 0x%04X", msg.CommandCode, CmdNormalControl)
			}
			// SEQ#
			if msg.SequenceNum != tt.seqNum {
				t.Errorf("SequenceNum = 0x%02X, want 0x%02X", msg.SequenceNum, tt.seqNum)
			}
			// 메시지 세트 검증
			if len(msg.MessageSets) != len(tt.sets) {
				t.Fatalf("MessageSets len = %d, want %d", len(msg.MessageSets), len(tt.sets))
			}
			for i, ms := range msg.MessageSets {
				if ms.Index != tt.sets[i].Index {
					t.Errorf("MessageSets[%d].Index = 0x%04X, want 0x%04X", i, ms.Index, tt.sets[i].Index)
				}
				if !bytes.Equal(ms.Value, tt.sets[i].Value) {
					t.Errorf("MessageSets[%d].Value = %X, want %X", i, ms.Value, tt.sets[i].Value)
				}
			}
		})
	}
}
