package samsung

import (
	"testing"
)

// makeValidFrame 는 테스트용 유효한 NASA 프로토콜 프레임을 생성한다.
func makeValidFrame(t *testing.T) []byte {
	t.Helper()
	proto := NewNasaProtocol()
	msg := &NasaMessage{
		SourceAddr:  NasaAddress{0x20, 0x00, 0x00},
		DestAddr:    NasaAddress{0x10, 0x00, 0x00},
		CommandCode: 0xC011,
		SequenceNum: 1,
		MessageSets: nil,
	}
	frame, err := proto.Encode(msg)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	return frame
}

func TestFrameScanner_SingleCompleteFrame(t *testing.T) {
	frame := makeValidFrame(t)
	s := newFrameScanner()
	s.Write(frame)

	got, ok := s.Next()
	if !ok {
		t.Fatal("Next() returned false, expected a frame")
	}
	if len(got) != len(frame) {
		t.Errorf("frame length = %d, want %d", len(got), len(frame))
	}

	// 버퍼가 비었어야 한다
	if s.Buffered() != 0 {
		t.Errorf("Buffered() = %d, want 0", s.Buffered())
	}

	// 추가 프레임 없음
	_, ok = s.Next()
	if ok {
		t.Error("Next() returned true, expected false (no more frames)")
	}
}

func TestFrameScanner_TwoFramesAtOnce(t *testing.T) {
	frame := makeValidFrame(t)
	s := newFrameScanner()

	// 두 프레임 연속 입력
	combined := make([]byte, 0, len(frame)*2)
	combined = append(combined, frame...)
	combined = append(combined, frame...)
	s.Write(combined)

	// 첫 번째 프레임
	got1, ok := s.Next()
	if !ok {
		t.Fatal("Next() #1 returned false")
	}
	if len(got1) != len(frame) {
		t.Errorf("frame #1 length = %d, want %d", len(got1), len(frame))
	}

	// 두 번째 프레임
	got2, ok := s.Next()
	if !ok {
		t.Fatal("Next() #2 returned false")
	}
	if len(got2) != len(frame) {
		t.Errorf("frame #2 length = %d, want %d", len(got2), len(frame))
	}

	// 추가 프레임 없음
	_, ok = s.Next()
	if ok {
		t.Error("Next() returned true, expected no more frames")
	}
}

func TestFrameScanner_FragmentedDelivery(t *testing.T) {
	frame := makeValidFrame(t)
	s := newFrameScanner()

	// 프레임을 3조각으로 나누어 입력
	mid := len(frame) / 2
	chunk1 := frame[:mid]
	chunk2 := frame[mid:]

	// 첫 번째 조각
	s.Write(chunk1)
	_, ok := s.Next()
	if ok {
		t.Fatal("Next() returned frame from incomplete data")
	}

	// 두 번째 조각 → 이제 완전한 프레임
	s.Write(chunk2)
	got, ok := s.Next()
	if !ok {
		t.Fatal("Next() returned false after completing frame")
	}
	if len(got) != len(frame) {
		t.Errorf("frame length = %d, want %d", len(got), len(frame))
	}
}

func TestFrameScanner_GarbageBeforeFrame(t *testing.T) {
	frame := makeValidFrame(t)
	s := newFrameScanner()

	// 쓰레기 바이트 + 유효 프레임
	garbage := []byte{0xFF, 0x00, 0xAA, 0xBB, 0xCC}
	data := make([]byte, 0, len(garbage)+len(frame))
	data = append(data, garbage...)
	data = append(data, frame...)
	s.Write(data)

	got, ok := s.Next()
	if !ok {
		t.Fatal("Next() returned false, expected a frame after garbage")
	}
	if len(got) != len(frame) {
		t.Errorf("frame length = %d, want %d", len(got), len(frame))
	}
}

func TestFrameScanner_GarbageBetweenFrames(t *testing.T) {
	frame := makeValidFrame(t)
	s := newFrameScanner()

	garbage := []byte{0xFF, 0xFE, 0xFD}
	data := make([]byte, 0, len(frame)+len(garbage)+len(frame))
	data = append(data, frame...)
	data = append(data, garbage...)
	data = append(data, frame...)
	s.Write(data)

	// 첫 번째 프레임
	got1, ok := s.Next()
	if !ok {
		t.Fatal("Next() #1 returned false")
	}
	if len(got1) != len(frame) {
		t.Errorf("frame #1 length = %d, want %d", len(got1), len(frame))
	}

	// 두 번째 프레임 (쓰레기 이후)
	got2, ok := s.Next()
	if !ok {
		t.Fatal("Next() #2 returned false")
	}
	if len(got2) != len(frame) {
		t.Errorf("frame #2 length = %d, want %d", len(got2), len(frame))
	}
}

func TestFrameScanner_ByteByByte(t *testing.T) {
	frame := makeValidFrame(t)
	s := newFrameScanner()

	// 바이트 하나씩 입력 — 마지막 바이트에서 프레임 완성
	for i := 0; i < len(frame)-1; i++ {
		s.Write(frame[i : i+1])
		_, ok := s.Next()
		if ok {
			t.Fatalf("Next() returned frame at byte %d/%d", i+1, len(frame))
		}
	}

	// 마지막 바이트
	s.Write(frame[len(frame)-1:])
	got, ok := s.Next()
	if !ok {
		t.Fatal("Next() returned false after final byte")
	}
	if len(got) != len(frame) {
		t.Errorf("frame length = %d, want %d", len(got), len(frame))
	}
}

func TestFrameScanner_InvalidLEN(t *testing.T) {
	s := newFrameScanner()

	// STX + 비정상 LEN(0x00 0x01 = 프레임 크기 3, MinFrameSize 미만) + 쓰레기 + 유효 프레임
	frame := makeValidFrame(t)
	data := []byte{FrameSTX, 0x00, 0x01, 0xFF}
	data = append(data, frame...)
	s.Write(data)

	got, ok := s.Next()
	if !ok {
		t.Fatal("Next() returned false, expected valid frame after invalid LEN")
	}
	if len(got) != len(frame) {
		t.Errorf("frame length = %d, want %d", len(got), len(frame))
	}
}

func TestFrameScanner_NoSTX(t *testing.T) {
	s := newFrameScanner()
	s.Write([]byte{0xFF, 0xFE, 0xFD, 0xFC})

	_, ok := s.Next()
	if ok {
		t.Fatal("Next() returned frame from data with no STX")
	}
	if s.Buffered() != 0 {
		t.Errorf("Buffered() = %d, want 0 (garbage should be discarded)", s.Buffered())
	}
}

func TestFrameScanner_Reset(t *testing.T) {
	s := newFrameScanner()
	s.Write([]byte{FrameSTX, 0x00, 0x10})
	if s.Buffered() != 3 {
		t.Errorf("Buffered() = %d, want 3", s.Buffered())
	}
	s.Reset()
	if s.Buffered() != 0 {
		t.Errorf("Buffered() after Reset = %d, want 0", s.Buffered())
	}
}

func TestFrameScanner_DecodeIntegration(t *testing.T) {
	// frameScanner로 추출한 프레임이 Decode에 성공하는지 확인
	proto := NewNasaProtocol()
	frame := makeValidFrame(t)

	s := newFrameScanner()
	// 쓰레기 + 프레임 + 쓰레기 + 프레임
	data := []byte{0xFF, 0xFE}
	data = append(data, frame...)
	data = append(data, 0xAA)
	data = append(data, frame...)
	s.Write(data)

	count := 0
	for {
		extracted, ok := s.Next()
		if !ok {
			break
		}
		msg, err := proto.Decode(extracted)
		if err != nil {
			t.Errorf("Decode() error on frame %d: %v", count+1, err)
			continue
		}
		if msg.CommandCode != 0xC011 {
			t.Errorf("frame %d: CommandCode = 0x%04X, want 0xC011", count+1, msg.CommandCode)
		}
		count++
	}
	if count != 2 {
		t.Errorf("extracted %d frames, want 2", count)
	}
}
