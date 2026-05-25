// device_callback_test.go (SPEC-DEVICE-IDENTITY-001 Phase B — B-T2)
//
// 본 테스트는 AdaptLegacyCallback 의 동작을 검증한다:
//   - v1 시그니처 → V2 시그니처 어댑터의 인자 forwarding 정확성
//   - nil 인자 처리

package agent

import (
	"testing"
)

func TestAdaptLegacyCallback_ForwardsCompositeIDOnly(t *testing.T) {
	t.Parallel()

	var (
		gotAgent    string
		gotDeviceID string
		callCount   int
	)

	v1 := func(agentName, deviceID string) {
		gotAgent = agentName
		gotDeviceID = deviceID
		callCount++
	}

	v2 := AdaptLegacyCallback(v1)
	if v2 == nil {
		t.Fatal("expected non-nil adapter")
	}

	// V2 호출 시 deviceUID 는 drop, deviceCompositeID 가 v1 의 deviceID 로 전달.
	v2("lgcnp", "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", "lgcnp:81")

	if callCount != 1 {
		t.Errorf("expected v1 to be called once, got %d", callCount)
	}
	if gotAgent != "lgcnp" {
		t.Errorf("got agent %q, want %q", gotAgent, "lgcnp")
	}
	if gotDeviceID != "lgcnp:81" {
		t.Errorf("got deviceID %q (should be composite, not UUID), want %q",
			gotDeviceID, "lgcnp:81")
	}
}

func TestAdaptLegacyCallback_NilReturnsNil(t *testing.T) {
	t.Parallel()

	if v2 := AdaptLegacyCallback(nil); v2 != nil {
		t.Errorf("expected nil adapter for nil input, got non-nil")
	}
}
