package lg

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ICP-01 and ICP-02 Mixed Stream Tests (v0.18.x)
// ---------------------------------------------------------------------------

// TestIcp01Parser_MixedIcp01Icp02Stream 는 ICP-01 프레임 사이에 ICP-02 프레임이
// interleaved 되는 실제 시나리오를 테스트한다.
//
// RED test: 혼합 스트림 [valid ICP-01 ODU 20B] + [valid ICP-02 frame 0x56 with
// DLEN=0x04, payload containing 0x81 and 0x84 bytes] + [valid ICP-01 IDU 40B].
// 파서는 ODU, 그 다음 IDU를 반환해야 하며 (ICP-02 payload의 0x81 때문에
// 잘못된 IDU가 아닌), ICP-02 프레임은 하나의 통합된 단위로 skip되어야 한다.
//
// Before fix: 파서는 ODU를 반환하고, ICP-02 payload의 0x81 byte를 false IDU STX로
// 인식하여 잘못된 40바이트 read를 시도하고, 실제 IDU frame을 corrupt한다.
//
// After fix: 파서는 ODU를 반환하고, ICP-02를 cleanly skip하고, 실제 IDU를 반환한다.
// LastIcp02SkippedCount()는 1을 반환해야 하고, LastSkippedCount()는 0이어야 한다.
func TestIcp01Parser_MixedIcp01Icp02Stream(t *testing.T) {
	t.Parallel()

	// 1. 유효한 ICP-01 ODU 프레임 구성 (SEQ=01, XOR 체크섬)
	// STX(0x58) + 19 bytes
	oduFrame := []byte{
		0x58,             // STX
		0x01,             // SEQ (01 = ODU)
		0x00,             // byte[2]
		0x00, 0x00, 0x00, // byte[3:6]
		0x00, 0x00, 0x00, // byte[6:9]
		0x00, 0x00, 0x00, // byte[9:12]
		0x00, 0x00, 0x00, // byte[12:15]
		0x00, 0x00, // byte[15:17]
		0x00, 0x00, // byte[17:19]
		0x00, // byte[19] checksum (XOR of bytes[0:19])
	}
	// XOR checksum 계산
	var xor byte
	for i := 0; i < 19; i++ {
		xor ^= oduFrame[i]
	}
	oduFrame[19] = xor

	// 2. 유효한 ICP-02 프레임 구성
	// 프레임: STX(0x56) + LEN + DLEN(0x04) + DA(4B) + SLEN(0x04) + SA(4B) +
	//         CMD(2B) + SEQ0(1B) + PLEN(1B) + PAYLOAD + SEQ1(1B) + CRC(2B)
	// Payload에 0x81과 0x84를 포함하여 false IDU STX로 인식될 수 있도록 함.
	icp02Payload := []byte{0x81, 0x84} // false IDU markers in payload
	// LEN = STX(1) + LEN(1) + DLEN(1) + DA(4) + SLEN(1) + SA(4) + CMD(2) + SEQ0(1) + PLEN(1) + PAYLOAD + SEQ1(1) + CRC(2)
	icp02FrameLen := byte(1 + 1 + 1 + 4 + 1 + 4 + 2 + 1 + 1 + len(icp02Payload) + 1 + 2)
	icp02Frame := []byte{
		0x56,                   // STX
		icp02FrameLen,          // LEN (전체 프레임 길이, STX와 LEN 포함)
		0x04,                   // DLEN
		0x44, 0x55, 0x00, 0x00, // DA (4 bytes)
		0x04,                   // SLEN
		0x44, 0x55, 0x00, 0x67, // SA (4 bytes)
		0x02, 0x04, // CMD
		0x00,                    // SEQ0
		byte(len(icp02Payload)), // PLEN
	}
	icp02Frame = append(icp02Frame, icp02Payload...)
	icp02Frame = append(icp02Frame, []byte{
		0x01,       // SEQ1
		0x00, 0x00, // CRC (간단히 0x0000, 검증하지 않음)
	}...)

	// 3. 유효한 ICP-01 IDU 프레임 구성 (STX=0x81, long format 40B)
	iduFrame := make([]byte, 40)
	iduFrame[0] = 0x81  // STX (IDU #1)
	iduFrame[1] = 0x02  // CMD
	iduFrame[2] = 0x00  // SubCMD
	iduFrame[3] = 0x70  // DevType
	iduFrame[4] = 0x00  // DeviceID
	iduFrame[9] = 0x51  // SlotNum
	iduFrame[10] = 0x00 // OpMode
	iduFrame[11] = 20   // SetTempRaw (= 20 + 15 = 35°C)
	iduFrame[20] = 0x01 // b[20] should be IDU index (1)
	iduFrame[23] = 0x48 // RoomTemp sensor (decode to ~4°C)
	iduFrame[24] = 0x48 // InletTemp
	iduFrame[25] = 0x50 // OutletTemp
	iduFrame[29] = 0x51 // SlotNum redundancy
	iduFrame[31] = 20   // SetTempRaw redundancy
	iduFrame[36] = 0x48 // RoomTemp redundancy
	// 체크섬은 테스트용으로 무시 (RedundancyValid/StructureValid 검증)

	// 4. 혼합 스트림: ODU + ICP-02 + IDU
	mixedStream := bytes.NewReader(append(append(oduFrame, icp02Frame...), iduFrame...))

	// 5. 파서 생성 및 프레임 읽기
	parser := NewIcp01FrameParser(mixedStream)

	// 첫 번째 프레임: ODU
	frameType1, oduResult, iduResult, err1 := parser.ReadFrame()
	require.NoError(t, err1)
	require.Equal(t, byte('A'), frameType1)
	require.NotNil(t, oduResult)
	require.Nil(t, iduResult)
	assert.Equal(t, byte(0x01), oduResult.SEQ)

	// ICP-02를 skip한 후 두 번째 프레임: 실제 IDU (ICP-02 payload의 0x81이 아님)
	frameType2, oduResult2, iduResult2, err2 := parser.ReadFrame()
	require.NoError(t, err2)
	require.Equal(t, byte('B'), frameType2)
	require.Nil(t, oduResult2)
	require.NotNil(t, iduResult2)
	assert.Equal(t, byte(0x81), iduResult2.IDUAddr)
	assert.Equal(t, 1, iduResult2.IDUNum)

	// 핵심 검증: ICP-02 프레임은 하나의 통합된 단위로 skip되었는가?
	assert.Equal(t, 1, parser.LastIcp02SkippedCount(),
		"ICP-02 프레임이 하나의 완전한 단위로 skip되어야 함")
	assert.Equal(t, 0, parser.LastSkippedCount(),
		"알 수 없는 byte로 인한 skip은 없어야 함 (ICP-02는 recognized frame)")
}

// TestIcp01Parser_Icp02ConservativeFallback 는 ICP-02 형식처럼 보이지만 검증에 실패하는
// byte 시퀀스가 단일 unknown byte로 처리되는지 확인한다.
func TestIcp01Parser_Icp02ConservativeFallback(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		stream        []byte
		expectSkipped int // 단일 unknown byte로 처리되어야 함
	}{
		{
			name: "0x56 with invalid LEN (too small)",
			// 0x56 + 0x05 (LEN < 13) = invalid ICP-02
			// After 0x56 fails validation, 0x05 is not a valid ICP-01 STX either,
			// so it gets skipped as unknown byte. Total 2 bytes skipped.
			stream:        []byte{0x56, 0x05, 0x58}, // 0x58 = ODU STX
			expectSkipped: 2,                        // 0x56 and 0x05
		},
		{
			name: "0x56 with DLEN != 0x04",
			// 0x56 + 0x58 (LEN=88, valid) + 0x03 (DLEN != 0x04) = invalid
			// 0x56 fails validation, 0x58 is valid ODU STX → stops skipping
			stream:        []byte{0x56, 0x58}, // 0x56 invalid, 0x58 valid ODU STX
			expectSkipped: 1,                  // just 0x56
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// 유효한 ODU 프레임을 뒤에 붙여 complete parse 테스트
			oduFrame := []byte{
				0x58, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, // checksum
			}
			// XOR 계산
			var xor byte
			for i := 0; i < 19; i++ {
				xor ^= oduFrame[i]
			}
			oduFrame[19] = xor

			stream := bytes.NewReader(append(tc.stream, oduFrame...))
			parser := NewIcp01FrameParser(stream)

			frameType, oduResult, _, err := parser.ReadFrame()
			require.NoError(t, err)
			assert.Equal(t, byte('A'), frameType)
			assert.NotNil(t, oduResult)

			// 0x56은 single unknown byte로 처리되어야 함
			assert.Equal(t, tc.expectSkipped, parser.LastSkippedCount(),
				"0x56은 invalid ICP-02 형식이므로 단일 byte skip으로 폴백해야 함")
			assert.Equal(t, 0, parser.LastIcp02SkippedCount(),
				"invalid 형식이므로 ICP-02 skip count는 0이어야 함")
		})
	}
}

// TestIcp01Parser_Icp02PayloadWithFalseStx 는 ICP-02 payload가 0x58과 0x81..0x85를
// 포함하는 경우, 이들이 false IDU로 인식되지 않는지 확인한다.
func TestIcp01Parser_Icp02PayloadWithFalseStx(t *testing.T) {
	t.Parallel()

	// ICP-02 프레임 with false ODU (0x58) and false IDU (0x81, 0x84) in payload
	icp02Frame := []byte{
		0x56,                   // STX
		24,                     // LEN (STX + LEN + rest) = 1 + 1 + 22
		0x04,                   // DLEN
		0x44, 0x55, 0x00, 0x00, // DA (4 bytes)
		0x04,                   // SLEN
		0x44, 0x55, 0x00, 0x67, // SA (4 bytes)
		0x02, 0x04, // CMD
		0x00,                         // SEQ0
		0x05,                         // PLEN (payload length)
		0x58, 0x81, 0x84, 0xAA, 0xBB, // PAYLOAD with false STX bytes
		0x01,       // SEQ1
		0x00, 0x00, // CRC
	}

	// 유효한 ICP-01 ODU와 IDU
	oduFrame := []byte{
		0x58, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, // checksum
	}
	var xor byte
	for i := 0; i < 19; i++ {
		xor ^= oduFrame[i]
	}
	oduFrame[19] = xor

	iduFrame := make([]byte, 40)
	iduFrame[0] = 0x81
	iduFrame[1] = 0x02
	iduFrame[2] = 0x00
	iduFrame[3] = 0x70
	iduFrame[4] = 0x00
	iduFrame[9] = 0x51
	iduFrame[10] = 0x00
	iduFrame[11] = 20
	iduFrame[20] = 0x01
	iduFrame[23] = 0x48
	iduFrame[24] = 0x48
	iduFrame[25] = 0x50
	iduFrame[29] = 0x51
	iduFrame[31] = 20
	iduFrame[36] = 0x48

	// 혼합 스트림: ODU + ICP-02 (with false STX in payload) + IDU
	mixedStream := bytes.NewReader(append(append(oduFrame, icp02Frame...), iduFrame...))
	parser := NewIcp01FrameParser(mixedStream)

	// ODU
	frameType1, oduResult, _, err1 := parser.ReadFrame()
	require.NoError(t, err1)
	assert.Equal(t, byte('A'), frameType1)
	assert.NotNil(t, oduResult)

	// IDU (not false IDU from ICP-02 payload)
	frameType2, _, iduResult, err2 := parser.ReadFrame()
	require.NoError(t, err2)
	assert.Equal(t, byte('B'), frameType2)
	assert.NotNil(t, iduResult)
	assert.Equal(t, byte(0x81), iduResult.IDUAddr)

	// ICP-02 frame was skipped as a complete unit
	assert.Equal(t, 1, parser.LastIcp02SkippedCount())
	assert.Equal(t, 0, parser.LastSkippedCount())
}

// TestIcp01Parser_Icp01OnlyStream_NoRegression 는 ICP-02가 없는 순수 ICP-01 스트림이
// 변경 전과 동일하게 동작하는지 확인한다 (회귀 테스트).
func TestIcp01Parser_Icp01OnlyStream_NoRegression(t *testing.T) {
	t.Parallel()

	// 순수 ICP-01 스트림: ODU + IDU + ODU
	oduFrame1 := []byte{
		0x58, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00,
	}
	var xor byte
	for i := 0; i < 19; i++ {
		xor ^= oduFrame1[i]
	}
	oduFrame1[19] = xor

	iduFrame := make([]byte, 40)
	iduFrame[0] = 0x81
	iduFrame[1] = 0x02
	iduFrame[2] = 0x00
	iduFrame[3] = 0x70
	iduFrame[4] = 0x00
	iduFrame[9] = 0x51
	iduFrame[10] = 0x00
	iduFrame[11] = 20
	iduFrame[20] = 0x01
	iduFrame[23] = 0x48
	iduFrame[24] = 0x48
	iduFrame[25] = 0x50
	iduFrame[29] = 0x51
	iduFrame[31] = 20
	iduFrame[36] = 0x48

	stream := bytes.NewReader(append(append(oduFrame1, iduFrame...), oduFrame1...))
	parser := NewIcp01FrameParser(stream)

	// ODU
	frameType1, oduResult1, _, err1 := parser.ReadFrame()
	require.NoError(t, err1)
	assert.Equal(t, byte('A'), frameType1)
	assert.NotNil(t, oduResult1)
	assert.Equal(t, 0, parser.LastSkippedCount())
	assert.Equal(t, 0, parser.LastIcp02SkippedCount())

	// IDU
	frameType2, _, iduResult, err2 := parser.ReadFrame()
	require.NoError(t, err2)
	assert.Equal(t, byte('B'), frameType2)
	assert.NotNil(t, iduResult)
	assert.Equal(t, byte(0x81), iduResult.IDUAddr)
	assert.Equal(t, 0, parser.LastSkippedCount())
	assert.Equal(t, 0, parser.LastIcp02SkippedCount())

	// ODU again
	frameType3, oduResult3, _, err3 := parser.ReadFrame()
	require.NoError(t, err3)
	assert.Equal(t, byte('A'), frameType3)
	assert.NotNil(t, oduResult3)
	assert.Equal(t, 0, parser.LastSkippedCount())
	assert.Equal(t, 0, parser.LastIcp02SkippedCount())
}

// TestIcp01Parser_Icp02AtEOF 는 ICP-02 프레임이 스트림 끝에서 불완전하게 끝나는
// 경우를 처리한다.
func TestIcp01Parser_Icp02AtEOF(t *testing.T) {
	t.Parallel()

	// Incomplete ICP-02 frame at stream end
	icp02FrameIncomplete := []byte{
		0x56,       // STX
		0x20,       // LEN
		0x04,       // DLEN (valid so far)
		0x44, 0x55, // partial DA
		// stream ends here (EOF before full frame)
	}

	stream := bytes.NewReader(icp02FrameIncomplete)
	parser := NewIcp01FrameParser(stream)

	// This should hit EOF while trying to read the frame
	_, _, _, err := parser.ReadFrame()
	assert.ErrorIs(t, err, io.EOF)
}
