package samsung

import (
	"testing"
)

// ---------------------------------------------------------------------------
// TestMessageSetValueSize 는 Index의 2번째 니블(upper byte의 하위 4비트)에 따라
// 올바른 Value 크기를 반환하는지 검증한다.
//
// Index 2번째 니블 = (index >> 8) & 0x0F
// 니블 0 -> 1바이트, 니블 1 -> 1바이트, 니블 2 -> 2바이트,
// 니블 4 -> 4바이트, 니블 6 -> -1 (가변), 그 외 -> -1 (알 수 없음)
// ---------------------------------------------------------------------------
func TestMessageSetValueSize(t *testing.T) {
	tests := []struct {
		name  string
		index uint16
		want  int
	}{
		// 니블 0 -> 1바이트
		{name: "MsgPower (0x4000) nibble 0 -> 1", index: 0x4000, want: 1},
		{name: "MsgMode (0x4001) nibble 0 -> 1", index: 0x4001, want: 1},
		{name: "MsgFanSpeed (0x4006) nibble 0 -> 1", index: 0x4006, want: 1},
		{name: "MsgLongWind (0x4007) nibble 0 -> 1", index: 0x4007, want: 1},
		{name: "MsgBuzzer (0x4050) nibble 0 -> 1", index: 0x4050, want: 1},
		{name: "MsgWindless (0x4060) nibble 0 -> 1", index: 0x4060, want: 1},
		{name: "MsgSwingHorizontal (0x407E) nibble 0 -> 1", index: 0x407E, want: 1},
		{name: "MsgAddrRegister (0x2004) nibble 0 -> 1", index: 0x2004, want: 1},
		{name: "MsgReadyState (0x2010) nibble 0 -> 1", index: 0x2010, want: 1},

		// 니블 1 -> 1바이트
		{name: "MsgSwingVertical (0x4011) nibble 1 -> 1", index: 0x4011, want: 1},
		{name: "MsgFilterCleanReset (0x4025) nibble 0 -> 1", index: 0x4025, want: 1},
		{name: "MsgFilterCleanAlarm (0x4027) nibble 0 -> 1", index: 0x4027, want: 1},
		{name: "MsgXSFM (0x4043) nibble 0 -> 1", index: 0x4043, want: 1},
		{name: "MsgAutoDry (0x4111) nibble 1 -> 1", index: 0x4111, want: 1},

		// 니블 2 -> 2바이트
		{name: "MsgTargetTemp (0x4201) nibble 2 -> 2", index: 0x4201, want: 2},
		{name: "MsgCurrentTemp (0x4203) nibble 2 -> 2", index: 0x4203, want: 2},
		{name: "MsgErrorCode (0x0202) nibble 2 -> 2", index: 0x0202, want: 2},

		// 니블 4 -> 4바이트
		{name: "MsgRemoteLimit (0x0409) nibble 4 -> 4", index: 0x0409, want: 4},
		{name: "MsgAddrInfo (0x0408) nibble 4 -> 4", index: 0x0408, want: 4},

		// 니블 6 -> 가변 (-1)
		{name: "nibble 6 variable (0x4600) -> -1", index: 0x4600, want: -1},

		// 알 수 없는 니블 -> -1
		{name: "nibble 3 unknown (0x4301) -> -1", index: 0x4301, want: -1},
		{name: "nibble 5 unknown (0x4501) -> -1", index: 0x4501, want: -1},
		{name: "nibble 7 unknown (0x4701) -> -1", index: 0x4701, want: -1},
		{name: "nibble 8 unknown (0x4801) -> -1", index: 0x4801, want: -1},
		{name: "nibble 9 unknown (0x4901) -> -1", index: 0x4901, want: -1},
		{name: "nibble A unknown (0x4A01) -> -1", index: 0x4A01, want: -1},
		{name: "nibble F unknown (0x4F01) -> -1", index: 0x4F01, want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MessageSetValueSize(tt.index)
			if got != tt.want {
				t.Errorf("MessageSetValueSize(0x%04X) = %d, want %d", tt.index, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestCommandCodeConstants 는 명령 코드 상수가 올바른 값을 갖는지 검증한다.
// ---------------------------------------------------------------------------
func TestCommandCodeConstants(t *testing.T) {
	tests := []struct {
		name string
		got  uint16
		want uint16
	}{
		{name: "CmdStandbyRequest", got: CmdStandbyRequest, want: 0xC001},
		{name: "CmdStandbyResponse", got: CmdStandbyResponse, want: 0xC005},
		{name: "CmdNormalRequest", got: CmdNormalRequest, want: 0xC011},
		{name: "CmdNormalSetting", got: CmdNormalSetting, want: 0xC012},
		{name: "CmdNormalControl", got: CmdNormalControl, want: 0xC013},
		{name: "CmdNotification", got: CmdNotification, want: 0xC014},
		{name: "CmdAddressResponse", got: CmdAddressResponse, want: 0xC015},
		{name: "CmdControlResponse", got: CmdControlResponse, want: 0xC016},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = 0x%04X, want 0x%04X", tt.name, tt.got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestFrameConstants 는 프레임 상수가 올바른 값을 갖는지 검증한다.
// ---------------------------------------------------------------------------
func TestFrameConstants(t *testing.T) {
	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{name: "FrameSTX", got: FrameSTX, want: byte(0x32)},
		{name: "FrameETX", got: FrameETX, want: byte(0x34)},
		{name: "MinFrameSize", got: MinFrameSize, want: 16},
		{name: "HeaderSize", got: HeaderSize, want: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestMessageIndexConstants 는 메시지 인덱스 상수가 올바른 값을 갖는지 검증한다.
// ---------------------------------------------------------------------------
func TestMessageIndexConstants(t *testing.T) {
	tests := []struct {
		name string
		got  uint16
		want uint16
	}{
		{name: "MsgPower", got: MsgPower, want: 0x4000},
		{name: "MsgMode", got: MsgMode, want: 0x4001},
		{name: "MsgFanSpeed", got: MsgFanSpeed, want: 0x4006},
		{name: "MsgLongWind", got: MsgLongWind, want: 0x4007},
		{name: "MsgSwingVertical", got: MsgSwingVertical, want: 0x4011},
		{name: "MsgFilterCleanReset", got: MsgFilterCleanReset, want: 0x4025},
		{name: "MsgFilterCleanAlarm", got: MsgFilterCleanAlarm, want: 0x4027},
		{name: "MsgXSFM", got: MsgXSFM, want: 0x4043},
		{name: "MsgBuzzer", got: MsgBuzzer, want: 0x4050},
		{name: "MsgWindless", got: MsgWindless, want: 0x4060},
		{name: "MsgSwingHorizontal", got: MsgSwingHorizontal, want: 0x407E},
		{name: "MsgAutoDry", got: MsgAutoDry, want: 0x4111},
		{name: "MsgTargetTemp", got: MsgTargetTemp, want: 0x4201},
		{name: "MsgCurrentTemp", got: MsgCurrentTemp, want: 0x4203},
		{name: "MsgErrorCode", got: MsgErrorCode, want: 0x0202},
		{name: "MsgRemoteLimit", got: MsgRemoteLimit, want: 0x0409},
		{name: "MsgAddrInfo", got: MsgAddrInfo, want: 0x0408},
		{name: "MsgAddrRegister", got: MsgAddrRegister, want: 0x2004},
		{name: "MsgReadyState", got: MsgReadyState, want: 0x2010},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = 0x%04X, want 0x%04X", tt.name, tt.got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestNasaMessageStruct 는 NasaMessage 구조체의 필드가 올바르게 설정되는지 검증한다.
// ---------------------------------------------------------------------------
func TestNasaMessageStruct(t *testing.T) {
	msg := NasaMessage{
		SourceAddr:  NasaAddress{0x20, 0x00, 0x00},
		DestAddr:    NasaAddress{0x6A, 0xEE, 0xFF},
		CommandCode: CmdNormalControl,
		SequenceNum: 0xA8,
		MessageSets: []NasaMessageSet{
			{Index: MsgPower, Value: []byte{0x01}},
			{Index: MsgTargetTemp, Value: []byte{0x01, 0x18}},
		},
		Checksum: 0xCD4D,
		Raw:      []byte{0x32, 0x34},
	}

	if msg.SourceAddr != (NasaAddress{0x20, 0x00, 0x00}) {
		t.Errorf("SourceAddr = %v, want %v", msg.SourceAddr, NasaAddress{0x20, 0x00, 0x00})
	}
	if msg.DestAddr != (NasaAddress{0x6A, 0xEE, 0xFF}) {
		t.Errorf("DestAddr = %v, want %v", msg.DestAddr, NasaAddress{0x6A, 0xEE, 0xFF})
	}
	if msg.CommandCode != CmdNormalControl {
		t.Errorf("CommandCode = 0x%04X, want 0x%04X", msg.CommandCode, CmdNormalControl)
	}
	if msg.SequenceNum != 0xA8 {
		t.Errorf("SequenceNum = 0x%02X, want 0x%02X", msg.SequenceNum, 0xA8)
	}
	if len(msg.MessageSets) != 2 {
		t.Fatalf("len(MessageSets) = %d, want 2", len(msg.MessageSets))
	}
	if msg.MessageSets[0].Index != MsgPower {
		t.Errorf("MessageSets[0].Index = 0x%04X, want 0x%04X", msg.MessageSets[0].Index, MsgPower)
	}
	if len(msg.MessageSets[0].Value) != 1 || msg.MessageSets[0].Value[0] != 0x01 {
		t.Errorf("MessageSets[0].Value = %v, want [0x01]", msg.MessageSets[0].Value)
	}
	if msg.MessageSets[1].Index != MsgTargetTemp {
		t.Errorf("MessageSets[1].Index = 0x%04X, want 0x%04X", msg.MessageSets[1].Index, MsgTargetTemp)
	}
	if len(msg.MessageSets[1].Value) != 2 {
		t.Errorf("len(MessageSets[1].Value) = %d, want 2", len(msg.MessageSets[1].Value))
	}
	if msg.Checksum != 0xCD4D {
		t.Errorf("Checksum = 0x%04X, want 0x%04X", msg.Checksum, 0xCD4D)
	}
	if len(msg.Raw) != 2 {
		t.Errorf("len(Raw) = %d, want 2", len(msg.Raw))
	}
}

// ---------------------------------------------------------------------------
// TestNasaMessageSetStruct 는 NasaMessageSet 구조체의 필드가 올바르게 설정되는지 검증한다.
// ---------------------------------------------------------------------------
func TestNasaMessageSetStruct(t *testing.T) {
	tests := []struct {
		name      string
		set       NasaMessageSet
		wantIndex uint16
		wantValue []byte
	}{
		{
			name:      "power on (1바이트 값)",
			set:       NasaMessageSet{Index: MsgPower, Value: []byte{0x01}},
			wantIndex: 0x4000,
			wantValue: []byte{0x01},
		},
		{
			name:      "target temp 24.0C (2바이트 값)",
			set:       NasaMessageSet{Index: MsgTargetTemp, Value: []byte{0x01, 0x18}},
			wantIndex: 0x4201,
			wantValue: []byte{0x01, 0x18},
		},
		{
			name:      "remote limit (4바이트 값)",
			set:       NasaMessageSet{Index: MsgRemoteLimit, Value: []byte{0x00, 0x00, 0x00, 0x01}},
			wantIndex: 0x0409,
			wantValue: []byte{0x00, 0x00, 0x00, 0x01},
		},
		{
			name:      "nil value",
			set:       NasaMessageSet{Index: MsgPower, Value: nil},
			wantIndex: 0x4000,
			wantValue: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set.Index != tt.wantIndex {
				t.Errorf("Index = 0x%04X, want 0x%04X", tt.set.Index, tt.wantIndex)
			}
			if tt.wantValue == nil {
				if tt.set.Value != nil {
					t.Errorf("Value = %v, want nil", tt.set.Value)
				}
			} else {
				if len(tt.set.Value) != len(tt.wantValue) {
					t.Fatalf("len(Value) = %d, want %d", len(tt.set.Value), len(tt.wantValue))
				}
				for i := range tt.wantValue {
					if tt.set.Value[i] != tt.wantValue[i] {
						t.Errorf("Value[%d] = 0x%02X, want 0x%02X", i, tt.set.Value[i], tt.wantValue[i])
					}
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestMessageSetValueSizeConsistency 는 정의된 메시지 인덱스 상수에 대해
// MessageSetValueSize 가 프로토콜 스펙에 맞는 크기를 반환하는지 검증한다.
// ---------------------------------------------------------------------------
func TestMessageSetValueSizeConsistency(t *testing.T) {
	tests := []struct {
		name     string
		constant uint16
		wantSize int
	}{
		// 1바이트 메시지들 (니블 0)
		{name: "MsgPower", constant: MsgPower, wantSize: 1},
		{name: "MsgMode", constant: MsgMode, wantSize: 1},
		{name: "MsgFanSpeed", constant: MsgFanSpeed, wantSize: 1},
		{name: "MsgLongWind", constant: MsgLongWind, wantSize: 1},
		{name: "MsgBuzzer", constant: MsgBuzzer, wantSize: 1},
		{name: "MsgWindless", constant: MsgWindless, wantSize: 1},
		{name: "MsgSwingHorizontal", constant: MsgSwingHorizontal, wantSize: 1},
		{name: "MsgAddrRegister", constant: MsgAddrRegister, wantSize: 1},
		{name: "MsgReadyState", constant: MsgReadyState, wantSize: 1},

		// 1바이트 메시지들 (니블 0 - 0x40XX 범위)
		{name: "MsgFilterCleanReset", constant: MsgFilterCleanReset, wantSize: 1},
		{name: "MsgFilterCleanAlarm", constant: MsgFilterCleanAlarm, wantSize: 1},
		{name: "MsgXSFM", constant: MsgXSFM, wantSize: 1},

		// 1바이트 메시지들 (니블 1)
		{name: "MsgSwingVertical", constant: MsgSwingVertical, wantSize: 1},
		{name: "MsgAutoDry", constant: MsgAutoDry, wantSize: 1},

		// 2바이트 메시지들 (니블 2)
		{name: "MsgTargetTemp", constant: MsgTargetTemp, wantSize: 2},
		{name: "MsgCurrentTemp", constant: MsgCurrentTemp, wantSize: 2},
		{name: "MsgErrorCode", constant: MsgErrorCode, wantSize: 2},

		// 4바이트 메시지들 (니블 4)
		{name: "MsgRemoteLimit", constant: MsgRemoteLimit, wantSize: 4},
		{name: "MsgAddrInfo", constant: MsgAddrInfo, wantSize: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MessageSetValueSize(tt.constant)
			if got != tt.wantSize {
				t.Errorf("MessageSetValueSize(%s=0x%04X) = %d, want %d",
					tt.name, tt.constant, got, tt.wantSize)
			}
		})
	}
}
