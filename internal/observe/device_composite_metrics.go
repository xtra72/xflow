// device_composite_metrics.go (SPEC-DEVICE-IDENTITY-001 Phase B — B-T9)
//
// 본 파일은 composite key ("agent:local_id" — Deprecated alias) 사용 빈도를
// 추적하는 Prometheus counter 를 제공한다. 운영자가 외부 클라이언트의
// 마이그레이션 진척도를 모니터링하여 Phase D (composite 완전 제거 + xflowd
// v1.0 메이저 버전) 진입 시점을 결정하는 데 사용된다.
//
// 메트릭 이름: xflowd_device_composite_use_total
// 라벨:       source — composite alias 가 사용된 진입 경로
//
// 표준 라벨 값 (확장 가능):
//   - "yaml"     — yaml 설정 파일의 디바이스 참조 (B-T7 yaml resolver)
//   - "log"      — 로그 라인에서 composite fallback 표시 (B-T6 device_format)
//   - "rest_url" — REST URL alias dispatch (B-T4, Phase B2 세션에서 추가)
//   - "ws_event" — WebSocket event payload (B-T3, Phase B2 세션에서 추가)
//   - "callback" — agent callback 의 v1 시그니처 호출 (B-T2 호환 wrapper)
//
// 운영 가이드:
//   - 호환 기간 동안 본 메트릭이 점진적으로 감소해야 한다.
//   - 모든 source 라벨이 0 또는 무시 가능한 수준으로 떨어지면 Phase D 진입 검토.
//   - 라벨별 차등을 통해 어떤 진입 경로가 가장 느리게 마이그레이션되는지 식별
//     가능 (예: rest_url 이 높으면 외부 REST 클라이언트 마이그레이션 필요).
//
// 와이어링:
//   - IncDeviceCompositeUse(source) 로 카운터 증가.
//   - cmd/xflowd 의 startup 코드가 RegisterDeviceCompositeMetrics(reg) 호출.
//   - device_metrics.go (B-T9 외) 와 동일 패턴 — lazy init + 다중 등록 안전.

package observe

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// 표준 source 라벨 상수 — 타이포 방지를 위한 공유 값.
const (
	CompositeUseSourceYAML     = "yaml"
	CompositeUseSourceLog      = "log"
	CompositeUseSourceRESTURL  = "rest_url"
	CompositeUseSourceWSEvent  = "ws_event"
	CompositeUseSourceCallback = "callback"
)

var (
	deviceCompositeUseOnce  sync.Once
	deviceCompositeUseTotal *prometheus.CounterVec
)

// deviceCompositeUseCounter 는 lazy 초기화로 CounterVec 를 반환한다.
// 첫 호출 시 생성되며, 그 이후로는 같은 인스턴스를 재사용한다.
func deviceCompositeUseCounter() *prometheus.CounterVec {
	deviceCompositeUseOnce.Do(func() {
		deviceCompositeUseTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "xflowd_device_composite_use_total",
				Help: "Total count of legacy composite device reference (agent:local_id) usage. " +
					"Deprecated alias usage tracker for SPEC-DEVICE-IDENTITY-001 Phase B/D migration. " +
					"Label 'source' identifies the entry path (yaml, log, rest_url, ws_event, callback).",
			},
			[]string{"source"},
		)
	})
	return deviceCompositeUseTotal
}

// IncDeviceCompositeUse 는 지정한 source 의 composite-use 카운터를 1 증가시킨다.
//
// source 가 빈 문자열이면 라벨 "unknown" 으로 기록된다 (Prometheus 가 빈 라벨
// 값을 허용하지 않을 수 있는 환경 보호 + 호출자 버그 탐지).
//
// 표준 source 값은 위 상수 (CompositeUseSource*) 사용을 권장한다.
//
// SPEC-DEVICE-IDENTITY-001 § B-T9.
func IncDeviceCompositeUse(source string) {
	if source == "" {
		source = "unknown"
	}
	deviceCompositeUseCounter().WithLabelValues(source).Inc()
}

// RegisterDeviceCompositeMetrics 는 주어진 Registerer 에 composite-use 메트릭을
// 등록한다. cmd/xflowd 의 startup 코드가 한 번 호출해야 /metrics 엔드포인트에
// 노출된다.
//
// 동일 메트릭의 중복 등록 (prometheus.AlreadyRegisteredError) 은 무시한다.
// reg 가 nil 이면 no-op.
func RegisterDeviceCompositeMetrics(reg prometheus.Registerer) {
	if reg == nil {
		return
	}
	if err := reg.Register(deviceCompositeUseCounter()); err != nil {
		// AlreadyRegisteredError 는 다중 호출 시 정상 — 무시한다.
		if _, ok := err.(prometheus.AlreadyRegisteredError); ok {
			return
		}
		// 다른 에러는 무시 (메트릭 등록 실패가 비즈니스 로직을 깨면 안 됨).
		_ = err
	}
}

// CollectDeviceCompositeUse 는 테스트/조회 용으로 (source → 누적 count) 를
// 스냅샷으로 반환한다. lazy init 이전이면 빈 맵을 반환한다.
func CollectDeviceCompositeUse() map[string]float64 {
	if deviceCompositeUseTotal == nil {
		return map[string]float64{}
	}
	ch := make(chan prometheus.Metric, 64)
	deviceCompositeUseTotal.Collect(ch)
	close(ch)
	out := make(map[string]float64)
	for m := range ch {
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			continue
		}
		source := ""
		for _, lp := range pb.GetLabel() {
			if lp.GetName() == "source" {
				source = lp.GetValue()
				break
			}
		}
		if c := pb.GetCounter(); c != nil {
			out[source] = c.GetValue()
		}
	}
	return out
}
