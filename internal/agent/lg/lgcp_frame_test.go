package lg

import (
	"bytes"
	"encoding/hex"
	"io"
	"testing"
)

func TestLGCPFrameParser_ProtocolExample(t *testing.T) {
	// 프로토콜 문서 예시 프레임을 파싱하여 모든 필드를 검증한다.
	reader := bytes.NewReader(protocolExampleFrame)
	parser := NewLGCPFrameParser(reader, true)

	frame, err := parser.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() 에러: %v", err)
	}
	if frame.ParseErr != nil {
		t.Fatalf("파싱 에러: %v", frame.ParseErr)
	}

	// LEN = 0x2D = 45
	if frame.Length != 45 {
		t.Errorf("Length = %d, want 45", frame.Length)
	}

	// DA = 44 55 00 66
	wantDA, _ := hex.DecodeString("44550066")
	if !bytes.Equal(frame.DA, wantDA) {
		t.Errorf("DA = %s, want %s", hex.EncodeToString(frame.DA), hex.EncodeToString(wantDA))
	}

	// SA = 44 55 00 00
	wantSA, _ := hex.DecodeString("44550000")
	if !bytes.Equal(frame.SA, wantSA) {
		t.Errorf("SA = %s, want %s", hex.EncodeToString(frame.SA), hex.EncodeToString(wantSA))
	}

	// CMD = 02 04
	if frame.CMD[0] != 0x02 || frame.CMD[1] != 0x04 {
		t.Errorf("CMD = %02X%02X, want 0204", frame.CMD[0], frame.CMD[1])
	}

	// SEQ0 = F8
	if frame.SEQ0 != 0xF8 {
		t.Errorf("SEQ0 = 0x%02X, want 0xF8", frame.SEQ0)
	}

	// PLEN = 0x1A = 26
	if len(frame.Payload) != 26 {
		t.Errorf("Payload 길이 = %d, want 26", len(frame.Payload))
	}

	// 페이로드 첫 3바이트 확인: 11 00 10
	if len(frame.Payload) >= 3 {
		if frame.Payload[0] != 0x11 || frame.Payload[1] != 0x00 || frame.Payload[2] != 0x10 {
			t.Errorf("Payload 시작 = %s, want 110010",
				hex.EncodeToString(frame.Payload[:3]))
		}
	}

	// SEQ1 = 98
	if frame.SEQ1 != 0x98 {
		t.Errorf("SEQ1 = 0x%02X, want 0x98", frame.SEQ1)
	}

	// CRC 검증 통과
	if !frame.CRCValid {
		t.Error("CRCValid = false, want true")
	}
}

func TestLGCPFrameParser_ConsecutiveFrames(t *testing.T) {
	// 프로토콜 예시 프레임 2개를 연속으로 이어붙인 스트림을 파싱한다.
	doubled := make([]byte, len(protocolExampleFrame)*2)
	copy(doubled, protocolExampleFrame)
	copy(doubled[len(protocolExampleFrame):], protocolExampleFrame)

	reader := bytes.NewReader(doubled)
	parser := NewLGCPFrameParser(reader, true)

	for i := 0; i < 2; i++ {
		frame, err := parser.ReadFrame()
		if err != nil {
			t.Fatalf("프레임 %d: ReadFrame() 에러: %v", i+1, err)
		}
		if frame.ParseErr != nil {
			t.Fatalf("프레임 %d: 파싱 에러: %v", i+1, frame.ParseErr)
		}
		if frame.Length != 45 {
			t.Errorf("프레임 %d: Length = %d, want 45", i+1, frame.Length)
		}
		if !frame.CRCValid {
			t.Errorf("프레임 %d: CRCValid = false, want true", i+1)
		}
	}

	// 세 번째 읽기는 EOF 여야 한다.
	_, err := parser.ReadFrame()
	if err != io.EOF {
		t.Errorf("세 번째 ReadFrame() 에러 = %v, want io.EOF", err)
	}
}

func TestLGCPFrameParser_GarbageBeforeSTX(t *testing.T) {
	// STX 앞에 가비지 바이트가 있어도 프레임을 올바르게 파싱한다.
	garbage := []byte{0x00, 0xFF, 0x12, 0x34, 0xAB}
	data := append(garbage, protocolExampleFrame...)

	reader := bytes.NewReader(data)
	parser := NewLGCPFrameParser(reader, true)

	frame, err := parser.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() 에러: %v", err)
	}
	if frame.ParseErr != nil {
		t.Fatalf("파싱 에러: %v", frame.ParseErr)
	}
	if frame.Length != 45 {
		t.Errorf("Length = %d, want 45", frame.Length)
	}
	if !frame.CRCValid {
		t.Error("CRCValid = false, want true")
	}
}

func TestLGCPFrameParser_PartialFrame(t *testing.T) {
	// 불완전한 프레임 (LEN 은 45 이지만 데이터가 부족)
	// STX + LEN + 5바이트만 제공
	partial := make([]byte, 7)
	partial[0] = lgcpSTX
	partial[1] = 45 // LEN = 45, 하지만 43바이트가 추가로 필요
	partial[2] = 0x04
	partial[3] = 0x11
	partial[4] = 0x22
	partial[5] = 0x33
	partial[6] = 0x44

	reader := bytes.NewReader(partial)
	parser := NewLGCPFrameParser(reader, true)

	_, err := parser.ReadFrame()
	if err == nil {
		t.Error("ReadFrame() 에러 = nil, 불완전 프레임에서 에러를 기대함")
	}
}

func TestLGCPFrameParser_InvalidLEN(t *testing.T) {
	tests := []struct {
		name     string
		lenValue byte
	}{
		{name: "LEN 너무 작음 (5)", lenValue: 5},
		{name: "LEN 최솟값 미만 (10)", lenValue: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 프레임 구성: STX + LEN + 충분한 패딩
			data := make([]byte, int(tt.lenValue))
			if len(data) < 2 {
				data = make([]byte, 2)
			}
			data[0] = lgcpSTX
			data[1] = tt.lenValue

			reader := bytes.NewReader(data)
			parser := NewLGCPFrameParser(reader, false)

			frame, err := parser.ReadFrame()
			if err != nil {
				// LEN 이 너무 작으면 remaining 이 음수이므로
				// 파서가 에러 프레임을 반환할 수 있다.
				return
			}
			if frame.ParseErr == nil {
				t.Errorf("LEN=%d 에서 ParseErr = nil, 에러를 기대함", tt.lenValue)
			}
		})
	}
}

func TestLGCPFrameParser_STXInMiddle(t *testing.T) {
	// 데이터 중간에 0x56 이 나타나는 경우:
	// 첫 번째 0x56 뒤에 잘못된 LEN 이 와서 실패하고,
	// 두 번째 실제 프레임이 정상 파싱되어야 한다.
	//
	// 가짜 STX(0x56) + 가짜 LEN(0x03 = 너무 짧음) + 실제 프레임
	fake := []byte{0x56, 0x03}
	// 가짜 프레임의 remaining = 3 - 2 = 1바이트
	// 이 1바이트가 실제 프레임의 STX 가 될 수 있으므로
	// 의도적으로 실제 프레임 바로 뒤에 배치한다.
	data := append(fake, byte(0x00)) // 가짜 프레임의 1바이트
	data = append(data, protocolExampleFrame...)

	reader := bytes.NewReader(data)
	parser := NewLGCPFrameParser(reader, true)

	// 첫 번째 ReadFrame: 가짜 프레임 (LEN=3, ParseErr 설정)
	frame1, err := parser.ReadFrame()
	if err != nil {
		t.Fatalf("첫 번째 ReadFrame() 에러: %v", err)
	}
	if frame1.ParseErr == nil {
		t.Log("첫 번째 프레임이 에러 없이 파싱됨 (LEN=3)")
	}

	// 두 번째 ReadFrame: 실제 프레임
	frame2, err := parser.ReadFrame()
	if err != nil {
		t.Fatalf("두 번째 ReadFrame() 에러: %v", err)
	}
	if frame2.ParseErr != nil {
		t.Fatalf("두 번째 프레임 파싱 에러: %v", frame2.ParseErr)
	}
	if frame2.Length != 45 {
		t.Errorf("두 번째 프레임 Length = %d, want 45", frame2.Length)
	}
	if !frame2.CRCValid {
		t.Error("두 번째 프레임 CRCValid = false, want true")
	}
}

func TestLGCPFrameParser_CRCFail(t *testing.T) {
	// 유효한 프레임 구조이지만 CRC 가 잘못된 경우
	tampered := make([]byte, len(protocolExampleFrame))
	copy(tampered, protocolExampleFrame)
	// CRC 마지막 바이트 변조
	tampered[len(tampered)-1] ^= 0xFF

	reader := bytes.NewReader(tampered)
	parser := NewLGCPFrameParser(reader, true)

	frame, err := parser.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() 에러: %v", err)
	}
	if frame.ParseErr != nil {
		t.Fatalf("파싱 에러: %v", frame.ParseErr)
	}
	if frame.CRCValid {
		t.Error("CRCValid = true, CRC 변조 프레임에서 false 를 기대함")
	}
}

func TestLGCPFrameParser_CRCVerifyDisabled(t *testing.T) {
	// CRC 검증 비활성화 시 변조 프레임도 CRCValid=true 가 된다.
	tampered := make([]byte, len(protocolExampleFrame))
	copy(tampered, protocolExampleFrame)
	tampered[len(tampered)-1] ^= 0xFF

	reader := bytes.NewReader(tampered)
	parser := NewLGCPFrameParser(reader, false) // CRC 검증 비활성화

	frame, err := parser.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() 에러: %v", err)
	}
	if !frame.CRCValid {
		t.Error("CRC 검증 비활성화 시 CRCValid = false, want true")
	}
}

func TestParseSingleFrame_ProtocolExample(t *testing.T) {
	frame := ParseSingleFrame(protocolExampleFrame, true)
	if frame.ParseErr != nil {
		t.Fatalf("ParseSingleFrame() 에러: %v", frame.ParseErr)
	}
	if !frame.CRCValid {
		t.Error("CRCValid = false, want true")
	}
	if frame.Length != 45 {
		t.Errorf("Length = %d, want 45", frame.Length)
	}
}

func TestParseSingleFrame_InvalidSTX(t *testing.T) {
	data := make([]byte, len(protocolExampleFrame))
	copy(data, protocolExampleFrame)
	data[0] = 0x00 // 잘못된 STX

	frame := ParseSingleFrame(data, true)
	if frame.ParseErr == nil {
		t.Error("잘못된 STX 에서 ParseErr = nil, 에러를 기대함")
	}
}

func TestParseSingleFrame_TooShort(t *testing.T) {
	frame := ParseSingleFrame([]byte{0x56}, true)
	if frame.ParseErr == nil {
		t.Error("1바이트 프레임에서 ParseErr = nil, 에러를 기대함")
	}
}

func TestLGCPFrame_String(t *testing.T) {
	// String() 메서드가 패닉 없이 동작하는지 확인한다.
	frame := ParseSingleFrame(protocolExampleFrame, true)
	s := frame.String()
	if s == "" {
		t.Error("String() 이 빈 문자열을 반환함")
	}
	t.Logf("String() = %s", s)
}

func TestLGCPFrameParser_EOF(t *testing.T) {
	// 빈 리더에서 EOF 를 반환해야 한다.
	reader := bytes.NewReader([]byte{})
	parser := NewLGCPFrameParser(reader, true)

	_, err := parser.ReadFrame()
	if err != io.EOF {
		t.Errorf("ReadFrame() 에러 = %v, want io.EOF", err)
	}
}
