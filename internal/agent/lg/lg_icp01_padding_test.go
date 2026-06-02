package lg

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIcp01_IDUFrame_ShortWithPadding 는 short frame 사이에 0x00 padding 이
// 1~3 byte 삽입된 환경 (사용자 실측, 2026-05-24) 에서 파서가 short variant 로
// 올바르게 인식하고 다음 frame 의 STX 동기를 유지하는지 검증한다 (v0.18.15).
//
// 사용자 stream:
//
//	8143017600164200205230050000000008006f5d  (IDU#1 short, 20B)
//	00                                          (1B padding)
//	82430176351642002055100e000000000800        (IDU#2 short, 20B 시작)
//
// 이전 v0.18.1 의 Peek(1) 은 padding=0x00 을 보고 long variant 로 판단해
// 40B 를 한 프레임으로 묶어 redundancy 검증 실패. v0.18.15 는 padding-tolerant
// Peek(3) 로 STX 발견 시 padding 소비 후 short 처리.
func TestIcp01_IDUFrame_ShortWithPadding(t *testing.T) {
	t.Parallel()

	// 사용자 실측 raw: IDU#1 short + 0x00 padding + IDU#2 short.
	raw, err := hex.DecodeString(
		"8143017600164200205230050000000008006f5d" + // IDU#1 (20B)
			"00" + // 1B padding
			"82430176351642002055100e0000000008000900", // IDU#2 (20B) + 0x00 dummy tail
	)
	require.NoError(t, err)

	reader := bytes.NewReader(raw)
	parser := NewIcp01FrameParser(reader)

	// 첫 ReadFrame → IDU#1 short.
	frameType, _, iduFrame, err := parser.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, byte('B'), frameType)
	require.NotNil(t, iduFrame)
	assert.Equal(t, byte(0x81), iduFrame.IDUAddr, "IDU#1 STX")
	assert.Equal(t, 1, iduFrame.IDUNum, "IDU#1 번호")
	assert.True(t, iduFrame.IsShort, "short variant 인식")

	// 두번째 ReadFrame → padding 소비 후 IDU#2 short.
	frameType2, _, iduFrame2, err := parser.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, byte('B'), frameType2)
	require.NotNil(t, iduFrame2)
	assert.Equal(t, byte(0x82), iduFrame2.IDUAddr, "IDU#2 STX (padding 소비 후)")
	assert.Equal(t, 2, iduFrame2.IDUNum, "IDU#2 번호")
	assert.True(t, iduFrame2.IsShort, "short variant 인식")
}

// TestIcp01_IDUFrame_LongFrame_NotFooled 는 표준 long frame (b[20]=IDU_INDEX
// 0x01~0x05) 이 padding-tolerant 로직 도입 후에도 정확히 long 으로 처리되는지
// regression 검증.
func TestIcp01_IDUFrame_LongFrame_NotFooled(t *testing.T) {
	t.Parallel()

	// 표준 long frame: b[20]=0x01 (IDU_INDEX for IDU#1) — short padding 으로
	// 오인되어선 안 됨.
	rawSlice := buildTestIDUFrame() // 40B long frame
	require.Equal(t, byte(0x01), rawSlice[20], "테스트 전제: b[20]=IDU_INDEX")

	reader := bytes.NewReader(rawSlice)
	parser := NewIcp01FrameParser(reader)

	frameType, _, iduFrame, err := parser.ReadFrame()
	require.NoError(t, err)
	assert.Equal(t, byte('B'), frameType)
	require.NotNil(t, iduFrame)
	assert.False(t, iduFrame.IsShort, "표준 long frame 이 short 로 오인되어선 안 됨")
}
