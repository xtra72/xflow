package observe

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsCollector 는 메트릭 수집 인터페이스이다.
// Counter, Histogram, Gauge 메트릭을 컴포넌트별로 관리한다.
type MetricsCollector interface {
	// Counter 는 지정된 이름과 컴포넌트에 대한 카운터 메트릭을 반환한다.
	// 동일한 (name, component) 조합은 같은 메트릭을 반환한다 (멱등성).
	Counter(name string, component string) CounterMetric

	// Histogram 은 지정된 이름과 컴포넌트에 대한 히스토그램 메트릭을 반환한다.
	// 동일한 (name, component) 조합은 같은 메트릭을 반환한다 (멱등성).
	Histogram(name string, component string) HistogramMetric

	// Gauge 는 지정된 이름과 컴포넌트에 대한 게이지 메트릭을 반환한다.
	// 동일한 (name, component) 조합은 같은 메트릭을 반환한다 (멱등성).
	Gauge(name string, component string) GaugeMetric

	// Registry 는 내부 Prometheus Registry 를 반환한다.
	Registry() *prometheus.Registry
}

// CounterMetric 은 카운터 메트릭 인터페이스이다.
type CounterMetric interface {
	// Inc 는 카운터를 1 증가시킨다.
	Inc()

	// Add 는 카운터에 지정된 값을 추가한다.
	Add(float64)
}

// HistogramMetric 은 히스토그램 메트릭 인터페이스이다.
type HistogramMetric interface {
	// Observe 는 히스토그램에 관측값을 기록한다.
	Observe(float64)
}

// GaugeMetric 은 게이지 메트릭 인터페이스이다.
type GaugeMetric interface {
	// Set 은 게이지를 지정된 값으로 설정한다.
	Set(float64)

	// Inc 는 게이지를 1 증가시킨다.
	Inc()

	// Dec 는 게이지를 1 감소시킨다.
	Dec()

	// Add 는 게이지에 지정된 값을 추가한다.
	Add(float64)
}

// metricsCollector 는 MetricsCollector 의 구현체이다.
// Prometheus 레지스트리를 사용하여 메트릭을 관리한다.
type metricsCollector struct {
	registry  *prometheus.Registry
	namespace string
	subsystem string

	mu         sync.RWMutex
	counters   map[string]*prometheus.CounterVec
	histograms map[string]*prometheus.HistogramVec
	gauges     map[string]*prometheus.GaugeVec
}

// NewMetricsCollector 는 지정된 옵션으로 새 MetricsCollector 를 생성한다.
// 기본값: namespace="xflow", subsystem="component", registry=prometheus.NewRegistry()
// 생성 시 사전 정의된 메트릭 3 종을 자동으로 등록한다.
func NewMetricsCollector(opts ...MetricsOption) MetricsCollector {
	cfg := &metricsConfig{
		namespace: "xflow",
		subsystem: "component",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.registry == nil {
		cfg.registry = prometheus.NewRegistry()
	}

	mc := &metricsCollector{
		registry:   cfg.registry,
		namespace:  cfg.namespace,
		subsystem:  cfg.subsystem,
		counters:   make(map[string]*prometheus.CounterVec),
		histograms: make(map[string]*prometheus.HistogramVec),
		gauges:     make(map[string]*prometheus.GaugeVec),
	}

	// 사전 정의된 메트릭을 등록한다 (REQ-OBS-001-03-03)
	mc.registerPreDefinedMetrics()

	return mc
}

// registerPreDefinedMetrics 는 사전 정의된 메트릭 3 종을 등록한다.
// 1. messages_total (Counter) - 처리된 메시지 총 수
// 2. errors_total (Counter) - 발생한 에러 총 수
// 3. processing_duration_seconds (Histogram) - 처리 소요 시간
func (mc *metricsCollector) registerPreDefinedMetrics() {
	mc.getOrCreateCounterVec("messages_total")
	mc.getOrCreateCounterVec("errors_total")
	mc.getOrCreateHistogramVec("processing_duration_seconds")
}

// Counter 는 지정된 이름과 컴포넌트에 대한 카운터 메트릭을 반환한다.
// 내부적으로 CounterVec 를 사용하며, component 라벨로 구분한다.
func (mc *metricsCollector) Counter(name string, component string) CounterMetric {
	vec := mc.getOrCreateCounterVec(name)
	return vec.WithLabelValues(component)
}

// Histogram 은 지정된 이름과 컴포넌트에 대한 히스토그램 메트릭을 반환한다.
// 내부적으로 HistogramVec 를 사용하며, component 라벨로 구분한다.
func (mc *metricsCollector) Histogram(name string, component string) HistogramMetric {
	vec := mc.getOrCreateHistogramVec(name)
	return vec.WithLabelValues(component)
}

// Gauge 는 지정된 이름과 컴포넌트에 대한 게이지 메트릭을 반환한다.
// 내부적으로 GaugeVec 를 사용하며, component 라벨로 구분한다.
func (mc *metricsCollector) Gauge(name string, component string) GaugeMetric {
	vec := mc.getOrCreateGaugeVec(name)
	return vec.WithLabelValues(component)
}

// Registry 는 내부 Prometheus Registry 를 반환한다.
func (mc *metricsCollector) Registry() *prometheus.Registry {
	return mc.registry
}

// getOrCreateCounterVec 는 이름으로 CounterVec 를 조회하거나 새로 생성한다.
// 중복 등록을 방지한다 (REQ-OBS-001-03-05).
func (mc *metricsCollector) getOrCreateCounterVec(name string) *prometheus.CounterVec {
	mc.mu.RLock()
	if vec, ok := mc.counters[name]; ok {
		mc.mu.RUnlock()
		return vec
	}
	mc.mu.RUnlock()

	mc.mu.Lock()
	defer mc.mu.Unlock()

	// 이중 확인 (double-check locking)
	if vec, ok := mc.counters[name]; ok {
		return vec
	}

	vec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: mc.namespace,
		Subsystem: mc.subsystem,
		Name:      name,
	}, []string{"component"})

	mc.registry.MustRegister(vec)
	mc.counters[name] = vec
	return vec
}

// getOrCreateHistogramVec 는 이름으로 HistogramVec 를 조회하거나 새로 생성한다.
// 중복 등록을 방지한다 (REQ-OBS-001-03-05).
func (mc *metricsCollector) getOrCreateHistogramVec(name string) *prometheus.HistogramVec {
	mc.mu.RLock()
	if vec, ok := mc.histograms[name]; ok {
		mc.mu.RUnlock()
		return vec
	}
	mc.mu.RUnlock()

	mc.mu.Lock()
	defer mc.mu.Unlock()

	// 이중 확인 (double-check locking)
	if vec, ok := mc.histograms[name]; ok {
		return vec
	}

	vec := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: mc.namespace,
		Subsystem: mc.subsystem,
		Name:      name,
		Buckets:   prometheus.DefBuckets,
	}, []string{"component"})

	mc.registry.MustRegister(vec)
	mc.histograms[name] = vec
	return vec
}

// getOrCreateGaugeVec 는 이름으로 GaugeVec 를 조회하거나 새로 생성한다.
// 중복 등록을 방지한다 (REQ-OBS-001-03-05).
func (mc *metricsCollector) getOrCreateGaugeVec(name string) *prometheus.GaugeVec {
	mc.mu.RLock()
	if vec, ok := mc.gauges[name]; ok {
		mc.mu.RUnlock()
		return vec
	}
	mc.mu.RUnlock()

	mc.mu.Lock()
	defer mc.mu.Unlock()

	// 이중 확인 (double-check locking)
	if vec, ok := mc.gauges[name]; ok {
		return vec
	}

	vec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: mc.namespace,
		Subsystem: mc.subsystem,
		Name:      name,
	}, []string{"component"})

	mc.registry.MustRegister(vec)
	mc.gauges[name] = vec
	return vec
}
