package lg

import (
	"bytes"
	"io"
	"testing"
)

// FragmentedReader 는 io.Reader 에서 바이트를 지정된 청크 크기로 분해하여
// 반환한다. TCP 네트워크에서 데이터가 작은 세그먼트로 도착하는 상황을 시뮬레이션한다.
type FragmentedReader struct {
	data      []byte
	chunkSize int
	offset    int
}

// NewFragmentedReader 는 data 를 chunkSize 바이트씩 분해하는 reader 를 생성한다.
func NewFragmentedReader(data []byte, chunkSize int) *FragmentedReader {
	return &FragmentedReader{
		data:      data,
		chunkSize: chunkSize,
		offset:    0,
	}
}

// Read 는 최대 chunkSize 바이트를 반환한다 (실제 TCP 동작 시뮬레이션).
func (r *FragmentedReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := r.chunkSize
	if n > len(p) {
		n = len(p)
	}
	if r.offset+n > len(r.data) {
		n = len(r.data) - r.offset
	}
	copy(p, r.data[r.offset:r.offset+n])
	r.offset += n
	return n, nil
}

// TestFragmentedIDUShortVsLongDetection 는 TCP 청킹 환경에서
// IDU SHORT(20바이트) vs LONG(40바이트) 감지가 올바르게 작동하는지 검증한다.
//
// 시나리오:
// - IDU SHORT 프레임 (20바이트) 다음 ODU 프레임이 도착
// - FragmentedReader 는 3바이트씩 분해하여 전달 (RPI 환경 시뮬레이션)
// - parser.ReadFrame() 이 SHORT 를 올바르게 감지해야 함
// - 두 번째 ReadFrame() 이 ODU 프레임을 올바르게 파싱해야 함
// - 바이트 정렬(alignment) 이 유지되어야 함
func TestFragmentedIDUShortVsLongDetection(t *testing.T) {
	// 생성할 프레임:
	// 1. IDU SHORT (0x81 STX + 19바이트): 총 20바이트
	//    raw: 81 ... (19바이트)
	// 2. ODU (0x58 STX + 19바이트): 총 20바이트
	//    raw: 58 ... (19바이트)

	// IDU SHORT 프레임 (20바이트): STX=0x81 + 임의 19바이트
	iduShortFrame := []byte{
		0x81,                         // STX (IDU #1)
		0x02,                         // CMD
		0x00,                         // SubCMD
		0x70,                         // DevType
		0x01,                         // DeviceID
		0x00, 0x00, 0x00, 0x00, 0x51, // bytes[5..9]: OpMode + SlotNum
		0x00, 0x1B, // bytes[10..11]: OpMode + SetTempRaw
		0x00, 0x00, 0x00, 0x00, 0x00, // bytes[12..16]
		0x00, 0x80, 0x00, // bytes[17..19]
	}
	if len(iduShortFrame) != 20 {
		t.Fatalf("IDU SHORT frame should be 20 bytes, got %d", len(iduShortFrame))
	}

	// ODU 프레임 (20바이트): STX=0x58 + 임의 19바이트
	oduFrame := []byte{
		0x58, // STX (ODU)
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09,
		0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10, 0x11, 0x12, 0x13,
	}
	if len(oduFrame) != 20 {
		t.Fatalf("ODU frame should be 20 bytes, got %d", len(oduFrame))
	}

	// 연속 바이트 스트림: IDU SHORT + ODU
	combinedStream := append(iduShortFrame, oduFrame...)

	// 3바이트씩 분해하여 전달 (RPI 상황 시뮬레이션)
	fragReader := NewFragmentedReader(combinedStream, 3)

	// Parser 생성
	parser := NewIcp01FrameParser(fragReader)

	// 첫 번째 ReadFrame: IDU SHORT 파싱
	frameType, _, iduFrame, err := parser.ReadFrame()
	if err != nil {
		t.Fatalf("First ReadFrame failed: %v", err)
	}
	if frameType != 'B' {
		t.Fatalf("First frame should be IDU (type 'B'), got %c", frameType)
	}
	if iduFrame == nil {
		t.Fatalf("First frame should be IDU, got nil")
	}

	// IDU SHORT 인지 확인
	if !iduFrame.IsShort {
		t.Errorf("IDU frame should be IsShort=true (파서가 20바이트로 감지), got IsShort=%v", iduFrame.IsShort)
	}

	// IDU 프레임이 올바르게 파싱되었는지 확인
	if iduFrame.IDUAddr != 0x81 {
		t.Errorf("IDU addr should be 0x81, got 0x%02x", iduFrame.IDUAddr)
	}
	if iduFrame.IDUNum != 1 {
		t.Errorf("IDU num should be 1, got %d", iduFrame.IDUNum)
	}

	// IDU SHORT 의 Raw 첫 20바이트가 입력과 일치하는지 확인
	if !bytes.Equal(iduFrame.Raw[:20], iduShortFrame) {
		t.Errorf("IDU SHORT raw mismatch.\nExpected: %v\nGot:      %v",
			iduShortFrame, iduFrame.Raw[:20])
	}

	// 두 번째 ReadFrame: ODU 파싱
	frameType, oduFrameParsed, _, err := parser.ReadFrame()
	if err != nil {
		t.Fatalf("Second ReadFrame failed: %v", err)
	}
	if frameType != 'A' {
		t.Fatalf("Second frame should be ODU (type 'A'), got %c", frameType)
	}
	if oduFrameParsed == nil {
		t.Fatalf("Second frame should be ODU, got nil")
	}

	// ODU 프레임이 올바르게 파싱되었는지 확인
	if oduFrameParsed.Raw[0] != 0x58 {
		t.Errorf("ODU STX should be 0x58, got 0x%02x", oduFrameParsed.Raw[0])
	}
	if !bytes.Equal(oduFrameParsed.Raw[:], oduFrame) {
		t.Errorf("ODU raw mismatch.\nExpected: %v\nGot:      %v",
			oduFrame, oduFrameParsed.Raw[:])
	}

	t.Logf("SUCCESS: Fragmented IDU SHORT and ODU framing correct")
}

// TestFragmentedIDULongFrameParsing 는 청킹 환경에서
// IDU LONG(40바이트) 프레임이 올바르게 파싱되는지 검증한다.
func TestFragmentedIDULongFrameParsing(t *testing.T) {
	// IDU LONG 프레임 (40바이트): STX=0x81 + 39바이트
	iduLongFrame := make([]byte, 40)
	iduLongFrame[0] = 0x81 // STX
	iduLongFrame[1] = 0x02 // CMD
	iduLongFrame[2] = 0x00 // SubCMD
	for i := 3; i < 40; i++ {
		iduLongFrame[i] = byte(i % 256)
	}

	// ODU 프레임 (20바이트)
	oduFrame := []byte{
		0x58, // STX
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09,
		0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10, 0x11, 0x12, 0x13,
	}

	// 연속 스트림: IDU LONG + ODU
	combinedStream := append(iduLongFrame, oduFrame...)

	// 3바이트씩 분해
	fragReader := NewFragmentedReader(combinedStream, 3)
	parser := NewIcp01FrameParser(fragReader)

	// 첫 번째 ReadFrame: IDU LONG
	frameType, _, iduFrame, err := parser.ReadFrame()
	if err != nil {
		t.Fatalf("First ReadFrame failed: %v", err)
	}
	if frameType != 'B' {
		t.Fatalf("First frame should be IDU, got %c", frameType)
	}

	// LONG 프레임인지 확인 (b[20] = 0x14, IDU_INDEX 범위가 아니므로 long 으로 파싱)
	if iduFrame.IsShort {
		t.Errorf("IDU frame should be IsShort=false (40바이트 파싱), got IsShort=%v", iduFrame.IsShort)
	}

	// 전체 40바이트가 올바르게 파싱되었는지 확인
	if !bytes.Equal(iduFrame.Raw[:], iduLongFrame) {
		t.Errorf("IDU LONG raw mismatch.\nExpected: %v\nGot:      %v",
			iduLongFrame, iduFrame.Raw[:])
	}

	// 두 번째 ReadFrame: ODU
	frameType, oduFrameParsed, _, err := parser.ReadFrame()
	if err != nil {
		t.Fatalf("Second ReadFrame failed: %v", err)
	}
	if frameType != 'A' {
		t.Fatalf("Second frame should be ODU, got %c", frameType)
	}
	if !bytes.Equal(oduFrameParsed.Raw[:], oduFrame) {
		t.Errorf("ODU raw mismatch after LONG frame")
	}

	t.Logf("SUCCESS: Fragmented IDU LONG framing correct")
}

// TestPartialPeekDuringShortDetection 는 bufio.Peek 이 부분 데이터만
// 반환하는 상황에서 SHORT 감지가 실패하는 버그를 재현한다.
//
// 증상: Peek(3) 이 1-2바이트만 반환 (다음 프레임 바이트가 아직 도착 안함)
// 하면, 파서가 이를 "padding 가능성" 으로 취급 → for 루프에서 STX 를
// 찾지 못함 → short flag 설정 안함 → io.ReadFull(raw[20:]) 로 20바이트
// 추가 소비 → 다음 프레임의 첫 20바이트 삼킴 → 스트림 미스얼라인.
func TestPartialPeekDuringShortDetection(t *testing.T) {
	// 이 테스트는 프래그먼트 리더의 제약으로 bufio 내부 상태를 정확히
	// 제어할 수 없어서, 실제 미스얼라인 증상을 보기 어렵다.
	// 대신 매우 작은 청크 (1바이트)로 분해하여 Peek 가 부분 데이터만
	// 반환할 가능성을 높인다.

	iduShortFrame := []byte{
		0x81, 0x02, 0x00, 0x70, 0x01, 0x00, 0x00, 0x00, 0x00, 0x51,
		0x00, 0x1B, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00,
	}

	oduFrame := []byte{
		0x58, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09,
		0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10, 0x11, 0x12, 0x13,
	}

	combinedStream := append(iduShortFrame, oduFrame...)

	// 1바이트씩 분해 — Peek 이 부분 데이터만 반환할 가능성 높음
	fragReader := NewFragmentedReader(combinedStream, 1)
	parser := NewIcp01FrameParser(fragReader)

	// 첫 번째 ReadFrame: IDU SHORT
	frameType1, _, iduFrame1, err1 := parser.ReadFrame()
	if err1 != nil {
		t.Fatalf("First ReadFrame failed: %v", err1)
	}
	if frameType1 != 'B' || iduFrame1 == nil {
		t.Fatalf("First frame parsing failed: type=%c, idu=%v", frameType1, iduFrame1)
	}

	// 첫 프레임이 SHORT 로 감지되었는지 확인
	if !iduFrame1.IsShort {
		t.Logf("WARNING: First IDU frame marked as LONG (IsShort=false), should be SHORT")
		t.Logf("  Raw[0..20]: %v", iduFrame1.Raw[0:20])
		t.Logf("  Raw[20..40]: %v", iduFrame1.Raw[20:40])
		// 버그 증상: raw[20..40] 이 ODU 프레임의 시작 바이트들을 포함
		if iduFrame1.Raw[20] == 0x58 {
			t.Errorf("BUG DETECTED: IDU LONG misdetection consumed ODU frame start (0x58)")
			t.Errorf("  Expected IsShort=true but got false, swallowed ODU bytes into IDU raw[20..40]")
			return // 버그 재현 성공
		}
	}

	// 두 번째 ReadFrame: 0x58 ODU 를 기대
	frameType2, oduFrame2, _, err2 := parser.ReadFrame()
	if err2 != nil {
		t.Logf("Second ReadFrame failed (possible misalignment): %v", err2)
		// 이 시점에서 EOF 또는 프레임 파싱 에러가 발생했다면 미스얼라인 확실
		if frameType2 == 0 {
			t.Errorf("Second frame not parsed — stream misaligned (BUG)")
		}
		return
	}

	if frameType2 != 'A' || oduFrame2 == nil {
		t.Errorf("Second frame should be ODU, got type=%c", frameType2)
		return
	}

	// ODU 바이트 검증
	if oduFrame2.Raw[0] != 0x58 {
		t.Errorf("ODU STX mismatch: expected 0x58, got 0x%02x — stream misaligned (BUG)", oduFrame2.Raw[0])
	}

	if !bytes.Equal(oduFrame2.Raw[:], oduFrame) {
		t.Errorf("ODU payload mismatch — stream misaligned")
		t.Logf("Expected: %v", oduFrame)
		t.Logf("Got:      %v", oduFrame2.Raw[:])
	}
}

// TestRealisticStreamWithMultipleFrames 는 현실적인 프레임 시퀀스를 검증:
// IDU SHORT + ODU + IDU LONG + ODU + IDU SHORT ...
// 각 청크는 2-5 바이트로 분해 (네트워크 변동성 시뮬레이션)
func TestRealisticStreamWithMultipleFrames(t *testing.T) {
	// 프레임 시퀀스 구성
	iduShort1 := make([]byte, 20)
	iduShort1[0] = 0x81
	iduShort1[1] = 0x02

	odu1 := make([]byte, 20)
	odu1[0] = 0x58
	odu1[1] = 0x01

	iduLong := make([]byte, 40)
	iduLong[0] = 0x82
	iduLong[1] = 0x03

	odu2 := make([]byte, 20)
	odu2[0] = 0x58
	odu2[1] = 0x02

	iduShort2 := make([]byte, 20)
	iduShort2[0] = 0x83
	iduShort2[1] = 0x04

	stream := [][]byte{iduShort1, odu1, iduLong, odu2, iduShort2}
	var combined []byte
	for _, frame := range stream {
		combined = append(combined, frame...)
	}

	fragReader := NewFragmentedReader(combined, 4) // 4바이트씩 분해
	parser := NewIcp01FrameParser(fragReader)

	expectedTypes := []byte{'B', 'A', 'B', 'A', 'B'}
	expectedIsShort := []bool{true, false, false, false, true}
	expectedSTX := []byte{0x81, 0x58, 0x82, 0x58, 0x83}

	for i := 0; i < 5; i++ {
		frameType, oduFrame, iduFrame, err := parser.ReadFrame()
		if err != nil {
			t.Fatalf("Frame %d ReadFrame failed: %v", i, err)
		}
		if frameType != expectedTypes[i] {
			t.Errorf("Frame %d: expected type %c, got %c", i, expectedTypes[i], frameType)
		}

		if frameType == 'A' {
			if oduFrame.Raw[0] != expectedSTX[i] {
				t.Errorf("Frame %d: ODU STX expected 0x%02x, got 0x%02x", i, expectedSTX[i], oduFrame.Raw[0])
			}
		} else {
			if iduFrame.Raw[0] != expectedSTX[i] {
				t.Errorf("Frame %d: IDU STX expected 0x%02x, got 0x%02x", i, expectedSTX[i], iduFrame.Raw[0])
			}
			if iduFrame.IsShort != expectedIsShort[i] {
				t.Errorf("Frame %d: expected IsShort=%v, got %v", i, expectedIsShort[i], iduFrame.IsShort)
			}
		}
	}

	t.Logf("SUCCESS: Realistic multi-frame stream parsed correctly")
}

// 실패 사례를 명시적으로 재현하는 벤치마크 스타일 테스트
func BenchmarkFragmentationStress(b *testing.B) {
	iduShort := make([]byte, 20)
	iduShort[0] = 0x81

	odu := make([]byte, 20)
	odu[0] = 0x58

	// 많은 프레임을 연속으로 파싱
	stream := []byte{}
	for i := 0; i < 100; i++ {
		stream = append(stream, iduShort...)
		stream = append(stream, odu...)
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		fragReader := NewFragmentedReader(stream, 3)
		parser := NewIcp01FrameParser(fragReader)

		for {
			frameType, _, _, err := parser.ReadFrame()
			if err != nil {
				if err == io.EOF {
					break
				}
				b.Fatalf("Parsing error: %v", err)
			}
			if frameType != 'A' && frameType != 'B' {
				b.Fatalf("Invalid frame type: %c", frameType)
			}
		}
	}
}
