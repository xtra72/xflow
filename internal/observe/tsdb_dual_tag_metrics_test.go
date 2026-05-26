// tsdb_dual_tag_metrics_test.go (SPEC-DEVICE-IDENTITY-001 Phase C § C3)
//
// dual-tag 메트릭의 4 state 분류 동작과 register / collect / reset 라이프사이클을
// 검증한다. device_composite_metrics_test.go (B-T9) 와 동일 패턴.

package observe

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestIncTSDBDualTag_StandardStates(t *testing.T) {
	ResetTSDBDualTagForTest()

	IncTSDBDualTag(TSDBDualTagStateBoth)
	IncTSDBDualTag(TSDBDualTagStateBoth)
	IncTSDBDualTag(TSDBDualTagStateCompositeOnly)
	IncTSDBDualTag(TSDBDualTagStateUIDOnly)
	IncTSDBDualTag(TSDBDualTagStateUnmapped)
	IncTSDBDualTag(TSDBDualTagStateUnmapped)
	IncTSDBDualTag(TSDBDualTagStateUnmapped)

	got := CollectTSDBDualTag()

	wantPairs := map[string]float64{
		TSDBDualTagStateBoth:          2,
		TSDBDualTagStateCompositeOnly: 1,
		TSDBDualTagStateUIDOnly:       1,
		TSDBDualTagStateUnmapped:      3,
	}
	for state, want := range wantPairs {
		if got[state] != want {
			t.Errorf("state %q: got %v, want %v (full snapshot: %v)", state, got[state], want, got)
		}
	}
}

func TestIncTSDBDualTag_UnknownState(t *testing.T) {
	ResetTSDBDualTagForTest()

	// 빈 문자열과 비표준 값 — 둘 다 "unknown" 으로 정규화되어야 한다.
	IncTSDBDualTag("")
	IncTSDBDualTag("bogus_value")
	IncTSDBDualTag("typo_state")

	got := CollectTSDBDualTag()
	if got["unknown"] != 3 {
		t.Errorf("unknown state: got %v, want 3 (full snapshot: %v)", got["unknown"], got)
	}
	// 표준 라벨에는 누적되지 않아야 한다.
	if got[TSDBDualTagStateBoth] != 0 {
		t.Errorf("standard label 'both' must be 0 but got %v", got[TSDBDualTagStateBoth])
	}
}

func TestRegisterTSDBDualTagMetrics_Idempotent(t *testing.T) {
	// 새 Registerer 에 두 번 등록해도 panic 없이 idempotent.
	reg := prometheus.NewRegistry()
	RegisterTSDBDualTagMetrics(reg)
	RegisterTSDBDualTagMetrics(reg)

	// nil Registerer 는 no-op.
	RegisterTSDBDualTagMetrics(nil)
}

func TestRegisterTSDBDualTagMetrics_ExposedInGather(t *testing.T) {
	ResetTSDBDualTagForTest()
	reg := prometheus.NewRegistry()
	RegisterTSDBDualTagMetrics(reg)

	IncTSDBDualTag(TSDBDualTagStateBoth)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	var found bool
	for _, f := range families {
		if f.GetName() == "xflowd_tsdb_dual_tag_total" {
			found = true
			if len(f.GetMetric()) == 0 {
				t.Error("metric family has no samples")
			}
			break
		}
	}
	if !found {
		t.Error("xflowd_tsdb_dual_tag_total not found in registered families")
	}
}

func TestCollectTSDBDualTag_BeforeInit_EmptyMap(t *testing.T) {
	// 본 테스트는 lazy init 직전 상태를 가정하지만, 다른 테스트가 먼저 실행되어
	// counter 가 생성되었을 수 있으므로 nil 가드만 검증한다. 실제 nil 상태는 단위
	// 테스트로 직접 만들기 어렵다 (sync.Once 비가역). 빈 맵 반환 자체는 정상.
	got := CollectTSDBDualTag()
	if got == nil {
		t.Error("CollectTSDBDualTag returned nil map (expected empty map)")
	}
}
