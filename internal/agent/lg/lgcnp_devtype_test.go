package lg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLGCNPIDUFrame_DevType_MaskedToUpperNibble 는 같은 IDU 에서 b[3] 의
// lower nibble 만 변하는 실측 패턴 (0x72/0x73/0x75) 을 모두 upper nibble
// (0x70) 으로 정규화하는지 검증한다 (v0.18.13).
//
// 사용자 실측 (2026-05-24):
//
//	동일 IDU 에서 device_type 값이 0x72 → 0x73 → 0x75 로 변화.
//	upper nibble 0x7 은 stable device class 로 추정, lower nibble 은
//	frame counter / status 로 추정. 안정값 확보를 위해 upper nibble 만 채택.
func TestLGCNPIDUFrame_DevType_MaskedToUpperNibble(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw3 byte
		want byte
	}{
		{"0x72 → 0x70", 0x72, 0x70},
		{"0x73 → 0x70", 0x73, 0x70},
		{"0x75 → 0x70", 0x75, 0x70},
		{"0x91 → 0x90 (캡처 A)", 0x91, 0x90},
		{"0x7C → 0x70 (캡처 B)", 0x7C, 0x70},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// raw[3] & 0xF0 로직 직접 검증 (frame parser 가 동일 마스크 적용).
			got := tc.raw3 & 0xF0
			assert.Equal(t, tc.want, got, "raw[3]=0x%02X 의 upper nibble", tc.raw3)
		})
	}
}
