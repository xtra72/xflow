// tsdb_dual_tag_metrics.go (SPEC-DEVICE-IDENTITY-001 Phase C — § C3)
//
// 본 파일은 시계열 DB 쓰기 경로에서 dual-tag (composite + uid) 부착 상태를
// 추적하는 Prometheus counter 를 제공한다. 호환 기간 동안 운영자가 마이그레이션
// 진척도를 모니터링하여 Phase D (composite 완전 제거 + xflowd v1.0) 진입 시점을
// 결정하는 데 사용된다.
//
// 메트릭 이름: xflowd_tsdb_dual_tag_total
// 라벨:        state — dual-tag 부착 결과 분류
//
// state 라벨 값 (총 4 종):
//   - "both"           — composite source tag 가 있고 ResolveDevice 가 성공하여
//                        uid 가 새로 추가됨 (마이그레이션 정상 진행).
//   - "composite_only" — composite source tag 가 있지만 ResolveDevice 가 실패
//                        (unmapped). composite 만 보유 — C1/C2 backfill 또는
//                        DeviceIDRepository 매핑 보완 필요.
//   - "uid_only"       — uid tag 가 이미 보유되어 변경 없음 (idempotent).
//                        composite tag 유무와 무관 — 이미 마이그레이션된 시계열.
//   - "unmapped"       — composite source tag 자체가 없음 — device 가 식별되지
//                        않는 일반 measurement (system metrics 등).
//
// 운영 가이드:
//   - "both" 카운터는 호환 기간 동안 점진적으로 증가하며 정상 상태.
//   - "composite_only" 가 0 또는 무시 가능한 수준으로 떨어지면 Phase D 진입 검토.
//   - "uid_only" 는 이미 마이그레이션된 시계열이 다시 기록될 때 증가 (정상).
//   - "unmapped" 는 device 식별이 불가능한 일반 measurement (정보용).
//
// 와이어링:
//   - IncTSDBDualTag(state) 로 카운터 증가.
//   - cmd/xflowd 의 startup 코드가 RegisterTSDBDualTagMetrics(reg) 호출.
//   - device_composite_metrics.go (B-T9) 와 동일 패턴 — lazy init + 다중 등록 안전.

package observe

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// 표준 state 라벨 상수 — 타이포 방지를 위한 공유 값.
const (
	// TSDBDualTagStateBoth: composite source 발견 + ResolveDevice 성공 + uid 새로 추가.
	TSDBDualTagStateBoth = "both"

	// TSDBDualTagStateCompositeOnly: composite source 발견 + ResolveDevice 실패 (unmapped).
	TSDBDualTagStateCompositeOnly = "composite_only"

	// TSDBDualTagStateUIDOnly: 이미 uid tag 보유 — 변경 없음 (idempotent).
	TSDBDualTagStateUIDOnly = "uid_only"

	// TSDBDualTagStateUnmapped: composite source 자체가 없음 — device 미식별 measurement.
	TSDBDualTagStateUnmapped = "unmapped"
)

var (
	tsdbDualTagOnce  sync.Once
	tsdbDualTagTotal *prometheus.CounterVec
)

// tsdbDualTagCounter 는 lazy 초기화로 CounterVec 를 반환한다.
// 첫 호출 시 생성되며, 그 이후로는 같은 인스턴스를 재사용한다.
func tsdbDualTagCounter() *prometheus.CounterVec {
	tsdbDualTagOnce.Do(func() {
		tsdbDualTagTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "xflowd_tsdb_dual_tag_total",
				Help: "Total count of TSDB write data classified by dual-tag emission state. " +
					"Used to track SPEC-DEVICE-IDENTITY-001 Phase C3 dual-tag migration progress. " +
					"Label 'state' takes one of: both, composite_only, uid_only, unmapped.",
			},
			[]string{"state"},
		)
	})
	return tsdbDualTagTotal
}

// IncTSDBDualTag 는 지정한 state 의 dual-tag 카운터를 1 증가시킨다.
//
// state 가 빈 문자열이거나 정의된 4 종 외 값이면 라벨 "unknown" 으로 기록된다
// (호출자 버그 탐지 + Prometheus 카디널리티 폭주 방지).
//
// 표준 state 값은 위 상수 (TSDBDualTagState*) 사용을 권장한다.
//
// SPEC-DEVICE-IDENTITY-001 § C3.
func IncTSDBDualTag(state string) {
	switch state {
	case TSDBDualTagStateBoth,
		TSDBDualTagStateCompositeOnly,
		TSDBDualTagStateUIDOnly,
		TSDBDualTagStateUnmapped:
		// 정상 라벨 — 그대로 사용.
	default:
		state = "unknown"
	}
	tsdbDualTagCounter().WithLabelValues(state).Inc()
}

// RegisterTSDBDualTagMetrics 는 주어진 Registerer 에 dual-tag 메트릭을 등록한다.
// cmd/xflowd 의 startup 코드가 한 번 호출해야 /metrics 엔드포인트에 노출된다.
//
// 동일 메트릭의 중복 등록 (prometheus.AlreadyRegisteredError) 은 무시한다.
// reg 가 nil 이면 no-op.
func RegisterTSDBDualTagMetrics(reg prometheus.Registerer) {
	if reg == nil {
		return
	}
	if err := reg.Register(tsdbDualTagCounter()); err != nil {
		// AlreadyRegisteredError 는 다중 호출 시 정상 — 무시한다.
		if _, ok := err.(prometheus.AlreadyRegisteredError); ok {
			return
		}
		// 다른 에러는 무시 (메트릭 등록 실패가 비즈니스 로직을 깨면 안 됨).
		_ = err
	}
}

// CollectTSDBDualTag 는 테스트/조회 용으로 (state → 누적 count) 를 스냅샷으로
// 반환한다. lazy init 이전이면 빈 맵을 반환한다.
func CollectTSDBDualTag() map[string]float64 {
	if tsdbDualTagTotal == nil {
		return map[string]float64{}
	}
	ch := make(chan prometheus.Metric, 64)
	tsdbDualTagTotal.Collect(ch)
	close(ch)
	out := make(map[string]float64)
	for m := range ch {
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			continue
		}
		state := ""
		for _, lp := range pb.GetLabel() {
			if lp.GetName() == "state" {
				state = lp.GetValue()
				break
			}
		}
		if c := pb.GetCounter(); c != nil {
			out[state] = c.GetValue()
		}
	}
	return out
}

// ResetTSDBDualTagForTest 는 테스트 격리를 위해 카운터를 초기화한다.
//
// production 코드에서는 호출 금지. 단일 메트릭 인스턴스가 sync.Once 로 생성되므로
// 진정한 초기화는 불가능하나, 동일 키의 누적 값을 0으로 재설정하여 테스트 간 격리를
// 제공한다 (DeleteLabelValues 호출).
//
// 테스트 외 호출 시 다른 테스트의 누적 카운터를 깨뜨릴 수 있다.
func ResetTSDBDualTagForTest() {
	if tsdbDualTagTotal == nil {
		return
	}
	tsdbDualTagTotal.Reset()
}
