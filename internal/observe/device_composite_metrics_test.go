// device_composite_metrics_test.go (SPEC-DEVICE-IDENTITY-001 Phase B — B-T9)

package observe

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestIncDeviceCompositeUse_KnownSources(t *testing.T) {
	// 본 테스트는 패키지-레벨 카운터를 변형하므로 t.Parallel 하지 않는다.

	// baseline 캡처 (다른 테스트의 잔여 카운트 보정).
	baseline := CollectDeviceCompositeUse()

	IncDeviceCompositeUse(CompositeUseSourceYAML)
	IncDeviceCompositeUse(CompositeUseSourceYAML)
	IncDeviceCompositeUse(CompositeUseSourceLog)

	got := CollectDeviceCompositeUse()
	if delta := got[CompositeUseSourceYAML] - baseline[CompositeUseSourceYAML]; delta != 2 {
		t.Errorf("yaml delta = %v, want 2", delta)
	}
	if delta := got[CompositeUseSourceLog] - baseline[CompositeUseSourceLog]; delta != 1 {
		t.Errorf("log delta = %v, want 1", delta)
	}
}

func TestIncDeviceCompositeUse_EmptySourceBecomesUnknown(t *testing.T) {
	baseline := CollectDeviceCompositeUse()

	IncDeviceCompositeUse("")

	got := CollectDeviceCompositeUse()
	if delta := got["unknown"] - baseline["unknown"]; delta != 1 {
		t.Errorf("unknown delta = %v, want 1", delta)
	}
}

func TestRegisterDeviceCompositeMetrics_NilRegisterer(t *testing.T) {
	t.Parallel()
	// nil registerer 는 no-op (panic 없음).
	RegisterDeviceCompositeMetrics(nil)
}

func TestRegisterDeviceCompositeMetrics_DuplicateIgnored(t *testing.T) {
	t.Parallel()
	reg := prometheus.NewRegistry()
	RegisterDeviceCompositeMetrics(reg)
	// 두 번째 등록은 AlreadyRegisteredError 를 받아 무시되어야 한다.
	RegisterDeviceCompositeMetrics(reg)
}

func TestCollectDeviceCompositeUse_BeforeFirstIncReturnsEmpty(t *testing.T) {
	t.Parallel()
	// 본 함수의 핵심 보장 — 카운터 미초기화 상태에서 호출해도 panic 없이
	// 빈/현재 스냅샷 반환. 이미 다른 테스트에서 초기화 되었더라도 map 만
	// 반환하면 충분.
	snap := CollectDeviceCompositeUse()
	if snap == nil {
		t.Error("expected non-nil map (possibly empty)")
	}
}
