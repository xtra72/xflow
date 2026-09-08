package handler

import (
	"testing"

	"github.com/xtra/xflow/internal/device"
)

// 갱신 시간 제한만 설정된 디바이스도 메타데이터를 응답에 실어야 한다.
// 종전처럼 조건식이 세 벌이었다면 한 화면에서만 값이 사라졌을 것이다.
func TestHasMetadata_StaleAfterSecAlone(t *testing.T) {
	sec := 300
	if !hasMetadata(device.DeviceMetadata{StaleAfterSec: &sec}) {
		t.Fatal("갱신 시간 제한만 있어도 메타데이터가 있는 것이다")
	}
}

func TestHasMetadata_Empty(t *testing.T) {
	if hasMetadata(device.DeviceMetadata{}) {
		t.Fatal("빈 메타데이터를 실어 보낼 이유가 없다")
	}
}

func TestHasMetadata_EachFieldCounts(t *testing.T) {
	pinned := false
	cases := map[string]device.DeviceMetadata{
		"name":     {Name: "온도계"},
		"location": {Location: "1층"},
		"tags":     {Tags: []string{"a"}},
		"group":    {Group: "g"},
		"labels":   {Labels: map[string]string{"k": "v"}},
		"pinned":   {Pinned: &pinned}, // false 도 "설정됨"이다
	}
	for name, meta := range cases {
		if !hasMetadata(meta) {
			t.Errorf("%s 만 있어도 메타데이터가 있는 것이다", name)
		}
	}
}

// 소유 정보는 내부 복원용이라 그것만으로는 사용자에게 보일 메타데이터가 아니다.
func TestHasMetadata_OwnershipAloneIsNotUserMetadata(t *testing.T) {
	if hasMetadata(device.DeviceMetadata{AgentName: "cs", LocalID: "aabb"}) {
		t.Fatal("소유 정보는 복원용 내부 필드다")
	}
}
