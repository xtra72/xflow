// device_metrics.go (SPEC-DEVICE-IDENTITY-001 Phase A — M2 metric)
//
// Phase A 에서는 Device.UID() 가 빈 문자열을 반환하는 경우 (graceful
// degradation — DeviceIDRepository 미설정 / 매핑 부재 / 에러) 를 추적하기 위해
// xflowd_device_uid_missing_total counter 를 노출한다.
//
// 이 카운터는 운영자가 Phase D (UUID 필수) 전환 가능 여부를 판단하는 데 사용된다.
// Phase D 에서 UID() 가 빈 문자열이면 부팅 실패로 전환되므로, 이 메트릭이 0 으로
// 정착할 때까지 Phase D 진입을 보류해야 한다.
//
// 와이어링: 호출자는 IncDeviceUIDMissing(agentName) 으로 카운터를 증가시킨다.
// 카운터 자체는 패키지-레벨 lazy init 으로 단 한 번만 생성된다. 기본적으로 어떤
// Registerer 에도 등록되지 않으며, /metrics 엔드포인트에 노출하려면 cmd/xflowd
// 의 startup 코드가 RegisterDeviceUIDMetrics(reg) 를 호출해야 한다.
// (코드베이스 기존 패턴 — observe.MetricsCollector 와 일관성 — 과 정렬.)

package observe

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

var (
	deviceUIDMissingOnce  sync.Once
	deviceUIDMissingTotal *prometheus.CounterVec
)

// deviceUIDMissingCounter 는 lazy 초기화로 CounterVec 를 반환한다.
// 첫 호출 시 생성되며, 그 이후로는 같은 인스턴스를 재사용한다.
func deviceUIDMissingCounter() *prometheus.CounterVec {
	deviceUIDMissingOnce.Do(func() {
		deviceUIDMissingTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "xflowd_device_uid_missing_total",
				Help: "Total count of Device.UID() returning empty string " +
					"(graceful degradation in Phase A; will fail boot in Phase D). " +
					"See SPEC-DEVICE-IDENTITY-001 § M1/M2.",
			},
			[]string{"agent_name"},
		)
	})
	return deviceUIDMissingTotal
}

// IncDeviceUIDMissing 은 지정한 agent 의 UID-missing 카운터를 1 증가시킨다.
//
// agentName 이 빈 문자열이면 라벨 "unknown" 으로 기록된다 (Prometheus 가 빈
// 라벨 값을 허용하지 않을 수 있는 환경 보호).
//
// SPEC-DEVICE-IDENTITY-001 § M1/M2 의 graceful degradation 추적용.
func IncDeviceUIDMissing(agentName string) {
	if agentName == "" {
		agentName = "unknown"
	}
	deviceUIDMissingCounter().WithLabelValues(agentName).Inc()
}

// RegisterDeviceUIDMetrics 는 주어진 Registerer 에 device-UID 관련 메트릭을
// 등록한다. cmd/xflowd 의 startup 코드가 한 번 호출해야 /metrics 엔드포인트에
// 노출된다.
//
// 동일 메트릭의 중복 등록 (prometheus.AlreadyRegisteredError) 은 무시한다.
// reg 가 nil 이면 no-op.
func RegisterDeviceUIDMetrics(reg prometheus.Registerer) {
	if reg == nil {
		return
	}
	if err := reg.Register(deviceUIDMissingCounter()); err != nil {
		// AlreadyRegisteredError 는 다중 호출 시 정상 — 무시한다.
		if _, ok := err.(prometheus.AlreadyRegisteredError); ok {
			return
		}
		// 다른 에러는 무시 (메트릭 등록 실패가 비즈니스 로직을 깨면 안 됨).
		_ = err
	}
}

// CollectDeviceUIDMissing 은 테스트/조회 용으로 (agent_name → 누적 count) 를
// 스냅샷으로 반환한다. lazy init 이전이면 빈 맵을 반환한다.
func CollectDeviceUIDMissing() map[string]float64 {
	if deviceUIDMissingTotal == nil {
		return map[string]float64{}
	}
	ch := make(chan prometheus.Metric, 64)
	deviceUIDMissingTotal.Collect(ch)
	close(ch)
	out := make(map[string]float64)
	for m := range ch {
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			continue
		}
		agentName := ""
		for _, lp := range pb.GetLabel() {
			if lp.GetName() == "agent_name" {
				agentName = lp.GetValue()
				break
			}
		}
		if c := pb.GetCounter(); c != nil {
			out[agentName] = c.GetValue()
		}
	}
	return out
}
