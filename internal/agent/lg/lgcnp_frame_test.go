package lg

import (
	"bytes"
	"encoding/hex"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 테스트 벡터 (프로토콜 분석 문서 기반)
// ---------------------------------------------------------------------------

// TYPE-A SEQ=01 XOR 체크섬 테스트 벡터
// 58 01 00 00 00 00 00 00 00 57 66 00 00 00 2e 00 19 01 43 1d
var lgcnpTestODU_SEQ01, _ = hex.DecodeString("58010000000000000057660000002e00190143" + "1d")

// TYPE-A SEQ=02 체크섬 없음, 실외 온도 포함
// 58 02 e0 06 02 00 81 4d 9f 00 31 cd e5 00 6b 5c 0f 00 f8 ee
var lgcnpTestODU_SEQ02, _ = hex.DecodeString("580200060200814d9f0031cde5006b5c0f00f8ee")

// TYPE-A SEQ=04 SUM 체크섬 테스트 벡터 (수정됨)
// 58 04 00 00 00 00 64 fe fe 00 84 00 00 00 00 00 00 00 15 55
var lgcnpTestODU_SEQ04, _ = hex.DecodeString("5804000000006464fe008400000000000000" + "1555")

// TYPE-B IDU_ADDR=0x81 테스트 벡터
// b[0]=0x81, b[1]=0x02, b[9]=0x52(set=22°C), b[20]=0x01
// b[23]=0x6d(room=22.5°C), b[24]=0x75(inlet=26.5°C), b[25]=0x77(outlet=27.5°C)
// b[29]=0x52, b[36]=0x6d (redundancy)
func buildTestIDUFrame() []byte {
	raw := make([]byte, 40)
	raw[0] = 0x81  // IDU addr
	raw[1] = 0x02  // CMD
	raw[3] = 0x10  // DevType
	raw[4] = 0x01  // DeviceID
	raw[9] = 0x52  // SlotNum (IDU 슬롯번호)
	raw[10] = 0x14 // OpMode
	raw[11] = 0x07 // SetTempRaw: 7 + 15 = 22°C
	raw[20] = 0x01 // b[20] = IDUAddr - 0x81 + 1 = 1
	raw[23] = 0x6D // RoomTemp: (0x6D - 0x40) / 2 = 22.5°C
	raw[24] = 0x75 // InletTemp: (0x75 - 0x40) / 2 = 26.5°C
	raw[25] = 0x77 // OutletTemp: (0x77 - 0x40) / 2 = 27.5°C
	raw[29] = 0x52 // Redundancy: SlotNum
	raw[31] = 0x07 // Redundancy: SetTempRaw
	raw[36] = 0x6D // Redundancy: RoomTemp
	return raw
}

// ---------------------------------------------------------------------------
// ODU 체크섬 테스트
// ---------------------------------------------------------------------------

func TestLGCNP_ODUChecksum_SEQ01_XOR(t *testing.T) {
	t.Parallel()

	var raw [20]byte
	copy(raw[:], lgcnpTestODU_SEQ01)

	valid := lgcnpVerifyODUChecksum(raw, 0x01)
	assert.True(t, valid, "SEQ=01 XOR 체크섬이 유효해야 함")
}

func TestLGCNP_ODUChecksum_SEQ02_NoChecksum(t *testing.T) {
	t.Parallel()

	var raw [20]byte
	copy(raw[:], lgcnpTestODU_SEQ02)

	valid := lgcnpVerifyODUChecksum(raw, 0x02)
	assert.True(t, valid, "SEQ=02는 체크섬 없음, 항상 유효")
}

func TestLGCNP_ODUChecksum_SEQ03_NoChecksum(t *testing.T) {
	t.Parallel()

	var raw [20]byte
	// 임의의 데이터
	raw[0] = 0x58
	raw[1] = 0x03

	valid := lgcnpVerifyODUChecksum(raw, 0x03)
	assert.True(t, valid, "SEQ=03은 체크섬 없음, 항상 유효")
}

func TestLGCNP_ODUChecksum_SEQ04_SUM(t *testing.T) {
	t.Parallel()

	// SUM 체크섬 테스트: 직접 계산하여 검증
	var raw [20]byte
	raw[0] = 0x58
	raw[1] = 0x04
	raw[6] = 0x64
	raw[7] = 0xFE
	raw[8] = 0xFE
	raw[10] = 0x84

	// SUM(bytes[0:19]) & 0xFF 를 bytes[19]에 설정
	var sum byte
	for i := 0; i < 19; i++ {
		sum += raw[i]
	}
	raw[19] = sum

	valid := lgcnpVerifyODUChecksum(raw, 0x04)
	assert.True(t, valid, "SEQ=04 SUM 체크섬이 유효해야 함")
}

// v0.18.10: 일부 디바이스는 SEQ=04 의 b[19] 를 표준 SUM 대신 고정 0x55 marker
// 로 사용한다. SUM 검증 실패 시 b[19]==0x55 이면 fixed marker variant 로
// 인식해 유효 처리해야 한다 (verify_odu_checksum=false 옵션 없이 자동 감지).
func TestLGCNP_ODUChecksum_SEQ04_Fixed0x55Marker(t *testing.T) {
	t.Parallel()

	// 사용자 환경 실측 프레임: SUM 검증은 실패하지만 b[19]=0x55 marker.
	raw := [20]byte{
		0x58, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x64,
		0xff, 0xff, 0x00, 0x9a, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x55,
	}

	// 우선 SUM 검증이 실제로 실패하는지 확인 (테스트 가설 검증).
	var sum byte
	for i := 0; i < 19; i++ {
		sum += raw[i]
	}
	assert.NotEqual(t, sum, raw[19], "테스트 전제: SUM 은 0x55 와 불일치해야 함")

	// auto-detect: b[19]=0x55 marker 면 유효.
	valid := lgcnpVerifyODUChecksum(raw, 0x04)
	assert.True(t, valid, "SEQ=04 b[19]=0x55 fixed marker 는 유효 처리되어야 함")
}

func TestLGCNP_ODUChecksum_SEQ01_Invalid(t *testing.T) {
	t.Parallel()

	var raw [20]byte
	copy(raw[:], lgcnpTestODU_SEQ01)
	// b[19] 만 변조하면 v0.18.22 의 b[13]^0x1D fallback 이 우연히
	// 일치할 수 있으므로 b[19] 를 marker 패턴과도 다른 값으로 설정.
	raw[19] = (raw[13] ^ 0x1D) ^ 0x42 // marker 와도 불일치, XOR 과도 불일치 보장

	valid := lgcnpVerifyODUChecksum(raw, 0x01)
	assert.False(t, valid, "변조된 SEQ=01 체크섬 (XOR + marker 모두 불일치) 은 실패해야 함")
}

// v0.18.22: 일부 디바이스 변형은 SEQ=01 에서 표준 XOR 체크섬 미사용,
// 대신 b[19] = b[13] ^ 0x1D marker 패턴 사용. 사용자 실측 4 프레임으로 검증.
func TestLGCNP_ODUChecksum_SEQ01_B13MarkerVariant(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  [20]byte
	}{
		{
			name: "frame 1 b[13]=0x96 → b[19]=0x8B",
			raw: [20]byte{
				0x58, 0x01, 0x01, 0x19, 0x19, 0x00, 0x00, 0x00,
				0x00, 0x60, 0x57, 0x00, 0x00, 0x96, 0x2e, 0x00,
				0x19, 0x01, 0x16, 0x8b,
			},
		},
		{
			name: "frame 2 b[13]=0x96 → b[19]=0x8B (다른 b[3..8])",
			raw: [20]byte{
				0x58, 0x01, 0x01, 0x14, 0x14, 0x08, 0x08, 0x08,
				0x08, 0x64, 0x57, 0x00, 0x00, 0x96, 0x2e, 0x00,
				0x19, 0x01, 0x08, 0x8b,
			},
		},
		{
			name: "frame 3 b[13]=0x00 → b[19]=0x1D",
			raw: [20]byte{
				0x58, 0x01, 0x00, 0x00, 0x14, 0x08, 0x08, 0x08,
				0x08, 0x57, 0x66, 0x00, 0x00, 0x00, 0x2e, 0x00,
				0x19, 0x01, 0x1f, 0x1d,
			},
		},
		{
			name: "frame 4 b[13]=0x00 → b[19]=0x1D (다른 b[18])",
			raw: [20]byte{
				0x58, 0x01, 0x00, 0x00, 0x00, 0x00, 0x08, 0x00,
				0x08, 0x57, 0x66, 0x00, 0x00, 0x00, 0x2e, 0x00,
				0x19, 0x01, 0x73, 0x1d,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// 전제 검증: 표준 XOR 체크섬은 실패해야 함.
			var xor byte
			for i := 0; i < 19; i++ {
				xor ^= tc.raw[i]
			}
			assert.NotEqual(t, tc.raw[19], xor, "테스트 전제: 표준 XOR 은 b[19] 와 불일치")

			// 가설 검증: b[13] ^ 0x1D == b[19].
			assert.Equal(t, tc.raw[19], tc.raw[13]^0x1D, "marker 패턴: b[13] ^ 0x1D == b[19]")

			// fallback auto-detect 가 유효 처리.
			valid := lgcnpVerifyODUChecksum(tc.raw, 0x01)
			assert.True(t, valid, "SEQ=01 b[13]^0x1D marker variant 는 유효 처리")
		})
	}
}

func TestLGCNP_ODUChecksum_SEQ05_XOR(t *testing.T) {
	t.Parallel()

	// SEQ=05도 XOR 체크섬 사용
	var raw [20]byte
	raw[0] = 0x58
	raw[1] = 0x05
	raw[5] = 0xAA

	var xor byte
	for i := 0; i < 19; i++ {
		xor ^= raw[i]
	}
	raw[19] = xor

	valid := lgcnpVerifyODUChecksum(raw, 0x05)
	assert.True(t, valid, "SEQ=05 XOR 체크섬이 유효해야 함")
}

// ---------------------------------------------------------------------------
// IDU 검증 테스트
// ---------------------------------------------------------------------------

func TestLGCNP_IDURedundancy_Valid(t *testing.T) {
	t.Parallel()

	rawSlice := buildTestIDUFrame()
	var raw [40]byte
	copy(raw[:], rawSlice)

	valid := lgcnpVerifyIDURedundancy(raw)
	assert.True(t, valid, "이중 기록이 일치하면 유효해야 함")
}

func TestLGCNP_IDURedundancy_Invalid_SlotNum(t *testing.T) {
	t.Parallel()

	rawSlice := buildTestIDUFrame()
	rawSlice[29] = 0x00 // 슬롯번호 불일치
	var raw [40]byte
	copy(raw[:], rawSlice)

	valid := lgcnpVerifyIDURedundancy(raw)
	assert.False(t, valid, "슬롯번호 불일치 시 실패해야 함")
}

func TestLGCNP_IDURedundancy_Invalid_RoomTemp(t *testing.T) {
	t.Parallel()

	rawSlice := buildTestIDUFrame()
	rawSlice[36] = 0x00 // 실내 온도 불일치
	var raw [40]byte
	copy(raw[:], rawSlice)

	valid := lgcnpVerifyIDURedundancy(raw)
	assert.False(t, valid, "실내 온도 불일치 시 실패해야 함")
}

func TestLGCNP_IDUStructure_Valid(t *testing.T) {
	t.Parallel()

	rawSlice := buildTestIDUFrame()
	var raw [40]byte
	copy(raw[:], rawSlice)

	valid := lgcnpVerifyIDUStructure(raw)
	assert.True(t, valid, "구조 검증이 통과해야 함")
}

func TestLGCNP_IDUStructure_InvalidCMD(t *testing.T) {
	t.Parallel()

	rawSlice := buildTestIDUFrame()
	rawSlice[1] = 0xFF // 잘못된 CMD
	var raw [40]byte
	copy(raw[:], rawSlice)

	valid := lgcnpVerifyIDUStructure(raw)
	assert.False(t, valid, "잘못된 CMD 시 구조 검증 실패해야 함")
}

func TestLGCNP_IDUStructure_InvalidB20(t *testing.T) {
	t.Parallel()

	rawSlice := buildTestIDUFrame()
	rawSlice[20] = 0xFF // 잘못된 IDU 번호
	var raw [40]byte
	copy(raw[:], rawSlice)

	valid := lgcnpVerifyIDUStructure(raw)
	assert.False(t, valid, "b[20] 불일치 시 구조 검증 실패해야 함")
}

// ---------------------------------------------------------------------------
// 온도 변환 테스트
// ---------------------------------------------------------------------------

func TestLGCNP_DecodeSensorTemp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		b        byte
		expected float64
	}{
		{"22.5°C", 0x6D, 22.5},
		{"26.5°C", 0x75, 26.5},
		{"27.5°C", 0x77, 27.5},
		{"0°C", 0x40, 0.0},
		{"25.0°C", 0x72, 25.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := lgcnpDecodeSensorTemp(tc.b)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestLGCNP_DecodeODUOutdoorTemp(t *testing.T) {
	t.Parallel()

	// SEQ=02 바이트[14]=0x6B → (0x6B-0x40)/2 = 21.5°C
	result := lgcnpDecodeODUOutdoorTemp(0x6B)
	assert.Equal(t, 21.5, result)

	// SEQ=02 바이트[15]=0x5C → (0x5C-0x40)/2 = 14.0°C
	result2 := lgcnpDecodeODUOutdoorTemp(0x5C)
	assert.Equal(t, 14.0, result2)
}

// ---------------------------------------------------------------------------
// 범위 검증 테스트
// ---------------------------------------------------------------------------

func TestLGCNP_IDURange_Valid(t *testing.T) {
	t.Parallel()

	f := &LGCNPIDUFrame{
		SetTemp:    22.0,
		RoomTemp:   22.5,
		InletTemp:  26.5,
		OutletTemp: 27.5,
	}
	assert.True(t, lgcnpVerifyIDURange(f))
}

func TestLGCNP_IDURange_RoomTempTooHigh(t *testing.T) {
	t.Parallel()

	f := &LGCNPIDUFrame{
		SetTemp:    22.0,
		RoomTemp:   51.0,
		InletTemp:  26.5,
		OutletTemp: 27.5,
	}
	assert.False(t, lgcnpVerifyIDURange(f))
}

func TestLGCNP_IDURange_InletTempTooHigh(t *testing.T) {
	t.Parallel()

	f := &LGCNPIDUFrame{
		SetTemp:    22.0,
		RoomTemp:   22.5,
		InletTemp:  71.0,
		OutletTemp: 27.5,
	}
	assert.False(t, lgcnpVerifyIDURange(f))
}

// ---------------------------------------------------------------------------
// 프레임 파서 스트림 테스트
// ---------------------------------------------------------------------------

func TestLGCNP_FrameParser_ReadODUFrame(t *testing.T) {
	t.Parallel()

	reader := bytes.NewReader(lgcnpTestODU_SEQ01)
	parser := NewLGCNPFrameParser(reader)

	frameType, oduFrame, iduFrame, err := parser.ReadFrame()
	require.NoError(t, err)

	assert.Equal(t, byte('A'), frameType)
	assert.NotNil(t, oduFrame)
	assert.Nil(t, iduFrame)
	assert.Equal(t, byte(0x01), oduFrame.SEQ)
	assert.True(t, oduFrame.ChecksumValid)
}

func TestLGCNP_FrameParser_ReadIDUFrame(t *testing.T) {
	t.Parallel()

	rawSlice := buildTestIDUFrame()
	reader := bytes.NewReader(rawSlice)
	parser := NewLGCNPFrameParser(reader)

	frameType, oduFrame, iduFrame, err := parser.ReadFrame()
	require.NoError(t, err)

	assert.Equal(t, byte('B'), frameType)
	assert.Nil(t, oduFrame)
	assert.NotNil(t, iduFrame)
	assert.Equal(t, byte(0x81), iduFrame.IDUAddr)
	assert.Equal(t, 1, iduFrame.IDUNum)
	assert.Equal(t, byte(0x02), iduFrame.CMD)
	assert.Equal(t, byte(0x52), iduFrame.SlotNum)
	assert.Equal(t, 22.0, iduFrame.SetTemp) // b[11]=0x07, 7+15=22
	assert.Equal(t, 22.5, iduFrame.RoomTemp)
	assert.Equal(t, 26.5, iduFrame.InletTemp)
	assert.Equal(t, 27.5, iduFrame.OutletTemp)
	assert.True(t, iduFrame.RedundancyValid)
	assert.True(t, iduFrame.StructureValid)
	assert.True(t, iduFrame.RangeOk)
}

func TestLGCNP_FrameParser_MixedStream(t *testing.T) {
	t.Parallel()

	// ODU 프레임 + IDU 프레임을 연결한 스트림
	iduRaw := buildTestIDUFrame()
	stream := append(lgcnpTestODU_SEQ01, iduRaw...)
	reader := bytes.NewReader(stream)
	parser := NewLGCNPFrameParser(reader)

	// 첫 번째: ODU
	ft1, odu1, _, err1 := parser.ReadFrame()
	require.NoError(t, err1)
	assert.Equal(t, byte('A'), ft1)
	assert.NotNil(t, odu1)

	// 두 번째: IDU
	ft2, _, idu2, err2 := parser.ReadFrame()
	require.NoError(t, err2)
	assert.Equal(t, byte('B'), ft2)
	assert.NotNil(t, idu2)

	// 세 번째: EOF
	_, _, _, err3 := parser.ReadFrame()
	assert.ErrorIs(t, err3, io.EOF)
}

func TestLGCNP_FrameParser_SkipGarbage(t *testing.T) {
	t.Parallel()

	// 가비지 바이트 + ODU 프레임
	garbage := []byte{0x00, 0xFF, 0x12, 0x34}
	stream := append(garbage, lgcnpTestODU_SEQ01...)
	reader := bytes.NewReader(stream)
	parser := NewLGCNPFrameParser(reader)

	frameType, oduFrame, _, err := parser.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, byte('A'), frameType)
	assert.NotNil(t, oduFrame)
	assert.Equal(t, byte(0x01), oduFrame.SEQ)
}

func TestLGCNP_FrameParser_EOF(t *testing.T) {
	t.Parallel()

	reader := bytes.NewReader([]byte{})
	parser := NewLGCNPFrameParser(reader)

	_, _, _, err := parser.ReadFrame()
	assert.ErrorIs(t, err, io.EOF)
}

func TestLGCNP_FrameParser_TruncatedODU(t *testing.T) {
	t.Parallel()

	// STX + 부족한 바이트 (19바이트 필요, 5바이트만 제공)
	truncated := []byte{0x58, 0x01, 0x00, 0x00, 0x00, 0x00}
	reader := bytes.NewReader(truncated)
	parser := NewLGCNPFrameParser(reader)

	_, _, _, err := parser.ReadFrame()
	assert.Error(t, err, "잘린 ODU 프레임은 에러를 반환해야 함")
}

func TestLGCNP_FrameParser_CMD_0x43(t *testing.T) {
	t.Parallel()

	// CMD=0x43도 유효한 구조임을 확인
	rawSlice := buildTestIDUFrame()
	rawSlice[1] = 0x43
	var raw [40]byte
	copy(raw[:], rawSlice)

	valid := lgcnpVerifyIDUStructure(raw)
	assert.True(t, valid, "CMD=0x43도 유효한 구조여야 함")
}

func TestLGCNP_FrameParser_IDU_AllAddresses(t *testing.T) {
	t.Parallel()

	// 0x81~0x85 모든 IDU 주소에 대해 파싱 확인
	for addr := byte(0x81); addr <= 0x85; addr++ {
		rawSlice := buildTestIDUFrame()
		rawSlice[0] = addr
		expectedNum := int(addr) - 0x80
		rawSlice[20] = byte(expectedNum)

		reader := bytes.NewReader(rawSlice)
		parser := NewLGCNPFrameParser(reader)

		ft, _, idu, err := parser.ReadFrame()
		require.NoError(t, err, "IDU addr 0x%02X 파싱 실패", addr)
		assert.Equal(t, byte('B'), ft)
		assert.Equal(t, int(addr-0x80), idu.IDUNum)
		assert.Equal(t, addr, idu.IDUAddr)
	}
}

// ---------------------------------------------------------------------------
// String() 메서드 테스트
// ---------------------------------------------------------------------------

func TestLGCNP_ODUFrame_String(t *testing.T) {
	t.Parallel()

	var raw [20]byte
	copy(raw[:], lgcnpTestODU_SEQ01)

	f := &LGCNPODUFrame{Raw: raw, SEQ: 0x01, ChecksumValid: true}
	s := f.String()
	assert.Contains(t, s, "SEQ=1")
	assert.Contains(t, s, "checksum=true")
}

func TestLGCNP_ODUFrame_String_Error(t *testing.T) {
	t.Parallel()

	f := &LGCNPODUFrame{ParseErr: io.ErrUnexpectedEOF}
	s := f.String()
	assert.Contains(t, s, "err=")
}

func TestLGCNP_IDUFrame_String(t *testing.T) {
	t.Parallel()

	f := &LGCNPIDUFrame{
		IDUNum:          1,
		CMD:             0x02,
		SlotNum:         0x52,
		SetTemp:         22.0,
		RoomTemp:        22.5,
		InletTemp:       26.5,
		OutletTemp:      27.5,
		RedundancyValid: true,
		StructureValid:  true,
		RangeOk:         true,
	}
	s := f.String()
	assert.Contains(t, s, "IDU=1")
	assert.Contains(t, s, "slot=52")
	assert.Contains(t, s, "set=22")
	assert.Contains(t, s, "22.5")
}

// ---------------------------------------------------------------------------
// CMD 비트 구조 테스트 (§6.8 확장)
// ---------------------------------------------------------------------------

func TestLGCNP_IsValidCMD(t *testing.T) {
	t.Parallel()

	valid := []byte{0x00, 0x01, 0x02, 0x03, 0x06, 0x08, 0x09, 0x41, 0x43, 0x47, 0x49}
	for _, cmd := range valid {
		assert.True(t, lgcnpIsValidCMD(cmd), "CMD 0x%02X는 유효해야 함", cmd)
	}

	invalid := []byte{0xFF, 0x80, 0x10, 0x20, 0x30, 0x50, 0x60, 0x90, 0xA0}
	for _, cmd := range invalid {
		assert.False(t, lgcnpIsValidCMD(cmd), "CMD 0x%02X는 무효해야 함", cmd)
	}
}

func TestLGCNP_IDUStructure_ExtendedCMD(t *testing.T) {
	t.Parallel()

	validCMDs := []byte{0x00, 0x01, 0x02, 0x03, 0x06, 0x08, 0x09, 0x41, 0x43, 0x47, 0x49}
	for _, cmd := range validCMDs {
		rawSlice := buildTestIDUFrame()
		rawSlice[1] = cmd
		var raw [40]byte
		copy(raw[:], rawSlice)

		valid := lgcnpVerifyIDUStructure(raw)
		assert.True(t, valid, "CMD=0x%02X도 유효한 구조여야 함", cmd)
	}
}

func TestLGCNP_IDUFrame_CMDBits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		cmd          byte
		wantCycle    bool
		wantActive   bool
		wantGroupB   bool
		wantUnchange bool
	}{
		{"0x02 base-A", 0x02, false, false, false, false},
		{"0x43 base-B", 0x43, true, true, false, false},
		{"0x41 groupA-active-B", 0x41, true, true, false, false},
		{"0x49 groupB-active-B", 0x49, true, true, true, false},
		{"0x01 groupA-active-A", 0x01, false, true, false, false},
		{"0x09 groupB-active-A", 0x09, false, true, true, false},
		{"0x06 base-A+unchanged", 0x06, false, false, false, true},
		{"0x47 base-B+unchanged", 0x47, true, true, false, true},
		{"0x00 transition-start", 0x00, false, false, false, false},
		{"0x08 transition-prog", 0x08, false, false, true, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rawSlice := buildTestIDUFrame()
			rawSlice[1] = tc.cmd

			reader := bytes.NewReader(rawSlice)
			parser := NewLGCNPFrameParser(reader)

			_, _, f, err := parser.ReadFrame()
			require.NoError(t, err)

			assert.Equal(t, tc.wantCycle, f.CycleBit, "CycleBit")
			assert.Equal(t, tc.wantActive, f.ActiveBit, "ActiveBit")
			assert.Equal(t, tc.wantGroupB, f.GroupBBit, "GroupBBit")
			assert.Equal(t, tc.wantUnchange, f.UnchangedBit, "UnchangedBit")
		})
	}
}

func TestLGCNP_IDUFrame_SetTempReliable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cmd      byte
		subCmd   byte
		reliable bool
	}{
		{"(02,00) 비신뢰", 0x02, 0x00, false},
		{"(02,01) 신뢰", 0x02, 0x01, true},
		{"(43,01) 신뢰", 0x43, 0x01, true},
		{"(41,01) 신뢰", 0x41, 0x01, true},
		{"(49,01) 신뢰", 0x49, 0x01, true},
		{"(01,01) 신뢰", 0x01, 0x01, true},
		{"(09,01) 신뢰", 0x09, 0x01, true},
		{"(00,01) 신뢰", 0x00, 0x01, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rawSlice := buildTestIDUFrame()
			rawSlice[1] = tc.cmd
			rawSlice[2] = tc.subCmd

			reader := bytes.NewReader(rawSlice)
			parser := NewLGCNPFrameParser(reader)

			_, _, f, err := parser.ReadFrame()
			require.NoError(t, err)
			assert.Equal(t, tc.reliable, f.SetTempReliable, "SetTempReliable")
		})
	}
}

func TestLGCNP_IDUFrame_ActiveFlag(t *testing.T) {
	t.Parallel()

	rawSlice := buildTestIDUFrame()
	rawSlice[1] = 0x41
	rawSlice[18] = 0x80 // bit7 set

	reader := bytes.NewReader(rawSlice)
	parser := NewLGCNPFrameParser(reader)

	_, _, f, err := parser.ReadFrame()
	require.NoError(t, err)
	assert.True(t, f.ActiveFlag, "b[18] bit7=1이면 ActiveFlag=true")
}

func TestLGCNP_IDUFrame_SetTempFormula(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		raw11 byte
		wantC float64
	}{
		{"18도", 0x03, 18.0},
		{"23도", 0x08, 23.0},
		{"24도", 0x09, 24.0},
		{"25도", 0x0A, 25.0},
		{"30도", 0x0F, 30.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rawSlice := buildTestIDUFrame()
			rawSlice[11] = tc.raw11
			rawSlice[31] = tc.raw11 // 이중 기록

			reader := bytes.NewReader(rawSlice)
			parser := NewLGCNPFrameParser(reader)

			_, _, f, err := parser.ReadFrame()
			require.NoError(t, err)
			assert.InDelta(t, tc.wantC, f.SetTemp, 0.01, "설정온도 = b[11]+15")
		})
	}
}

// ---------------------------------------------------------------------------
// v0.18.1: 20바이트 short IDU 변형 (재동기화) 테스트
// ---------------------------------------------------------------------------

// TestLGCNP_FrameParser_ShortIDU_FollowedByIDU 는 IDU#1 의 20바이트 short
// 프레임 직후 IDU#2 의 20바이트 short 프레임이 도착하는 시나리오를 검증한다.
// b[20] 위치에서 0x82 (IDU#2 STX) 를 Peek 으로 감지하여 IDU#1 의 두 번째
// 절반 (redundancy) 을 소비하지 않고 IDU#2 동기를 유지해야 한다.
func TestLGCNP_FrameParser_ShortIDU_FollowedByIDU(t *testing.T) {
	t.Parallel()

	// 실 캡처 데이터 (사용자 보고): IDU#1+IDU#2 20바이트씩 stacked.
	stream, err := hex.DecodeString("8102007c3515020020523005000000000800a75d8202007c351502002055100e0000000008008a5d")
	require.NoError(t, err)
	parser := NewLGCNPFrameParser(bytes.NewReader(stream))

	// 첫 번째 ReadFrame: IDU#1 short.
	ft1, _, idu1, err1 := parser.ReadFrame()
	require.NoError(t, err1)
	assert.Equal(t, byte('B'), ft1)
	require.NotNil(t, idu1)
	assert.True(t, idu1.IsShort, "20B 다음에 IDU STX 등장 → short variant")
	assert.Equal(t, 1, idu1.IDUNum)
	assert.Equal(t, byte(0x52), idu1.SlotNum)
	assert.Equal(t, byte(0x30), idu1.OpMode, "OpMode (power OFF + cool)")
	assert.Equal(t, 20.0, idu1.SetTemp, "SetTemp = 0x05 + 15 = 20°C")
	assert.True(t, idu1.RedundancyValid, "short variant 는 trivially 통과")
	assert.True(t, idu1.StructureValid, "short variant 는 trivially 통과")
	assert.True(t, idu1.RangeOk, "short variant 는 SetTemp 만 검증")
	assert.Equal(t, 0.0, idu1.RoomTemp, "short 는 RoomTemp 무효 (0)")

	// 두 번째 ReadFrame: IDU#2 short.
	ft2, _, idu2, err2 := parser.ReadFrame()
	require.NoError(t, err2)
	assert.Equal(t, byte('B'), ft2)
	require.NotNil(t, idu2)
	assert.Equal(t, 2, idu2.IDUNum)
	assert.Equal(t, byte(0x55), idu2.SlotNum)
	assert.Equal(t, byte(0x10), idu2.OpMode, "OpMode (power ON + cool)")
	assert.Equal(t, 29.0, idu2.SetTemp, "SetTemp = 0x0e + 15 = 29°C")

	// EOF.
	_, _, _, err3 := parser.ReadFrame()
	assert.ErrorIs(t, err3, io.EOF)
}

// TestLGCNP_FrameParser_ShortIDU_FollowedByODU 는 IDU#5 의 20바이트 short
// 프레임 직후 ODU SEQ=01 (STX=0x58) 이 도착하는 시나리오를 검증한다.
func TestLGCNP_FrameParser_ShortIDU_FollowedByODU(t *testing.T) {
	t.Parallel()

	// 실 캡처 데이터: IDU#5 short + ODU SEQ=01.
	stream, err := hex.DecodeString("8502007c351402002053100b000000000800895d58010400000000000057663500002e0019011a1d")
	require.NoError(t, err)
	parser := NewLGCNPFrameParser(bytes.NewReader(stream))

	// 첫 번째: IDU#5 short.
	ft1, _, idu1, err1 := parser.ReadFrame()
	require.NoError(t, err1)
	assert.Equal(t, byte('B'), ft1)
	require.NotNil(t, idu1)
	assert.True(t, idu1.IsShort)
	assert.Equal(t, 5, idu1.IDUNum)
	assert.Equal(t, byte(0x53), idu1.SlotNum)
	assert.Equal(t, byte(0x10), idu1.OpMode)
	assert.Equal(t, 26.0, idu1.SetTemp, "SetTemp = 0x0b + 15 = 26°C")

	// 두 번째: ODU SEQ=01.
	ft2, odu2, _, err2 := parser.ReadFrame()
	require.NoError(t, err2)
	assert.Equal(t, byte('A'), ft2)
	require.NotNil(t, odu2)
	assert.Equal(t, byte(0x01), odu2.SEQ)
}

// TestLGCNP_FrameParser_LongIDU_NotMisidentifiedAsShort 는 표준 40바이트
// IDU 프레임이 short 로 잘못 인식되지 않는지 검증한다. b[20] 의 값이 IDU_INDEX
// (0x01~0x05) 이므로 LGCNP STX 범위 밖이라 Peek 으로 short 라 판단되지 않아야
// 한다. 단 IDU_INDEX 가 0x05 인 IDU#5 의 경우 b[20]=0x05 는 STX 범위 (0x81~0x85)
// 밖이므로 무관.
func TestLGCNP_FrameParser_LongIDU_NotMisidentifiedAsShort(t *testing.T) {
	t.Parallel()

	rawSlice := buildTestIDUFrame() // IDU#1, b[20]=0x01 (표준 IDU_INDEX)
	reader := bytes.NewReader(rawSlice)
	parser := NewLGCNPFrameParser(reader)

	_, _, idu, err := parser.ReadFrame()
	require.NoError(t, err)
	require.NotNil(t, idu)
	assert.False(t, idu.IsShort, "b[20]=0x01 은 LGCNP STX 가 아니므로 long 으로 인식")
	assert.True(t, idu.RedundancyValid)
	assert.True(t, idu.StructureValid)
	assert.Equal(t, 22.5, idu.RoomTemp, "long 은 RoomTemp 정상 디코드")
}
