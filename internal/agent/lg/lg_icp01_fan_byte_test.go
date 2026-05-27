package lg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIcp01FanByteToID_DevType91 는 DEV_TYPE=0x91 (Multi V) 의 fan_byte 매핑을
// 검증한다. 0x54=quiet, 0x14=low.
func TestIcp01FanByteToID_DevType91(t *testing.T) {
	t.Parallel()

	assert.Equal(t, FanSpeedQuiet, icp01FanByteToID(0x54, 0x91))
	assert.Equal(t, FanSpeedLow, icp01FanByteToID(0x14, 0x91))
}

// TestIcp01FanByteToID_DevType7C 는 DEV_TYPE=0x7C 의 fan_byte 매핑을 검증한다.
// 0x50=low (134dbd2 실측 확인).
func TestIcp01FanByteToID_DevType7C(t *testing.T) {
	t.Parallel()

	assert.Equal(t, FanSpeedLow, icp01FanByteToID(0x50, 0x7C))
}

// TestIcp01FanByteToID_Universal0x30 는 fan_byte=0x30 이 DEV_TYPE 무관
// 범용 미풍 (quiet) 으로 매핑되는지 검증한다 (v0.18.10 + v0.18.11).
//
//	0x72, 0x73: 사용자 실측 확인 — 0x30=미풍
//	기타 DEV_TYPE: 동일 family 가정 — 범용 0x30=quiet 적용
func TestIcp01FanByteToID_Universal0x30(t *testing.T) {
	t.Parallel()

	cases := []byte{0x72, 0x73, 0x91, 0x7C, 0x00, 0xFF}
	for _, devType := range cases {
		devType := devType
		t.Run(string(rune(devType)), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, FanSpeedQuiet, icp01FanByteToID(0x30, devType))
		})
	}
}

// TestIcp01FanByteToID_UnknownFallsBackToAuto 는 인식되지 않는 fan_byte 가
// FanSpeedAuto 로 폴백되는지 검증한다.
func TestIcp01FanByteToID_UnknownFallsBackToAuto(t *testing.T) {
	t.Parallel()

	assert.Equal(t, FanSpeedAuto, icp01FanByteToID(0xFF, 0x00))
	assert.Equal(t, FanSpeedAuto, icp01FanByteToID(0xAB, 0x91))
}

// TestIcp01IsKnownFanByte 는 알려진 (devType, fanByte) 조합이 known 으로
// 판단되어 디버그 로그가 suppress 되는지 검증한다.
func TestIcp01IsKnownFanByte(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		devType byte
		raw     byte
		want    bool
	}{
		{"0x91 quiet (0x54)", 0x91, 0x54, true},
		{"0x91 low (0x14)", 0x91, 0x14, true},
		{"0x7C low (0x50)", 0x7C, 0x50, true},
		{"0x72 quiet (0x30)", 0x72, 0x30, true},
		{"0x73 quiet (0x30)", 0x73, 0x30, true}, // v0.18.11: 0x73 family 확장
		{"0x91 quiet (0x30)", 0x91, 0x30, true}, // v0.18.11: 범용 0x30
		{"0x72 unknown (0xFF)", 0x72, 0xFF, false},
		{"unknown devType + 0x14", 0xFF, 0x14, true}, // 범용 0x14 인식
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, icp01IsKnownFanByte(tc.devType, tc.raw))
		})
	}
}
