package observe_test

import (
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/xtra/xflow/internal/observe"
)

// TestMetricsCollector_Counter 는 카운터 메트릭의 생성, Inc, Add 동작을 검증한다.
func TestMetricsCollector_Counter(t *testing.T) {
	mc := observe.NewMetricsCollector(
		observe.WithRegistry(prometheus.NewRegistry()),
	)

	counter := mc.Counter("test_counter", "agent.mqtt")
	counter.Inc()
	counter.Add(4)

	// prometheus.Counter 는 prometheus.Collector 를 구현한다
	c, ok := counter.(prometheus.Collector)
	if !ok {
		t.Fatal("Counter 가 prometheus.Collector 를 구현하지 않는다")
	}

	val := testutil.ToFloat64(c)
	if val != 5 {
		t.Errorf("카운터 값 = %v, 기대값 = 5", val)
	}
}

// TestMetricsCollector_Histogram 은 히스토그램 메트릭의 Observe 동작을 검증한다.
func TestMetricsCollector_Histogram(t *testing.T) {
	reg := prometheus.NewRegistry()
	mc := observe.NewMetricsCollector(
		observe.WithRegistry(reg),
	)

	histogram := mc.Histogram("test_duration", "agent.mqtt")
	histogram.Observe(0.5)
	histogram.Observe(1.5)
	histogram.Observe(2.5)

	// Registry 에서 메트릭을 수집하여 히스토그램이 존재하는지 확인한다
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("메트릭 수집 실패: %v", err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "xflow_component_test_duration" {
			found = true
			metrics := mf.GetMetric()
			if len(metrics) == 0 {
				t.Fatal("히스토그램 메트릭이 비어있다")
			}
			h := metrics[0].GetHistogram()
			if h.GetSampleCount() != 3 {
				t.Errorf("히스토그램 샘플 수 = %v, 기대값 = 3", h.GetSampleCount())
			}
			if h.GetSampleSum() != 4.5 {
				t.Errorf("히스토그램 샘플 합 = %v, 기대값 = 4.5", h.GetSampleSum())
			}
		}
	}
	if !found {
		t.Error("xflow_component_test_duration 히스토그램을 찾을 수 없다")
	}
}

// TestMetricsCollector_Gauge 는 게이지 메트릭의 Set, Inc, Dec, Add 동작을 검증한다.
func TestMetricsCollector_Gauge(t *testing.T) {
	mc := observe.NewMetricsCollector(
		observe.WithRegistry(prometheus.NewRegistry()),
	)

	gauge := mc.Gauge("active_connections", "agent.mqtt")

	// Set
	gauge.Set(10)
	g, ok := gauge.(prometheus.Collector)
	if !ok {
		t.Fatal("Gauge 가 prometheus.Collector 를 구현하지 않는다")
	}
	if val := testutil.ToFloat64(g); val != 10 {
		t.Errorf("Set 후 게이지 값 = %v, 기대값 = 10", val)
	}

	// Inc
	gauge.Inc()
	if val := testutil.ToFloat64(g); val != 11 {
		t.Errorf("Inc 후 게이지 값 = %v, 기대값 = 11", val)
	}

	// Dec
	gauge.Dec()
	if val := testutil.ToFloat64(g); val != 10 {
		t.Errorf("Dec 후 게이지 값 = %v, 기대값 = 10", val)
	}

	// Add
	gauge.Add(5)
	if val := testutil.ToFloat64(g); val != 15 {
		t.Errorf("Add 후 게이지 값 = %v, 기대값 = 15", val)
	}
}

// TestMetricsCollector_Registry 는 Registry() 가 nil 이 아닌 *prometheus.Registry 를 반환하는지 검증한다.
func TestMetricsCollector_Registry(t *testing.T) {
	mc := observe.NewMetricsCollector()

	reg := mc.Registry()
	if reg == nil {
		t.Fatal("Registry() 가 nil 을 반환했다")
	}

	// 메트릭을 수집할 수 있는지 확인한다
	_, err := reg.Gather()
	if err != nil {
		t.Errorf("Registry 에서 메트릭 수집 실패: %v", err)
	}
}

// TestMetricsCollector_DuplicateCounter 는 동일한 (name, component) 로 Counter 를 호출하면
// 같은 메트릭을 반환하는지 검증한다 (멱등성).
func TestMetricsCollector_DuplicateCounter(t *testing.T) {
	mc := observe.NewMetricsCollector(
		observe.WithRegistry(prometheus.NewRegistry()),
	)

	c1 := mc.Counter("dup_counter", "agent.mqtt")
	c1.Inc()

	c2 := mc.Counter("dup_counter", "agent.mqtt")
	c2.Inc()

	// 같은 메트릭이므로 값이 누적되어야 한다
	col, ok := c2.(prometheus.Collector)
	if !ok {
		t.Fatal("Counter 가 prometheus.Collector 를 구현하지 않는다")
	}
	val := testutil.ToFloat64(col)
	if val != 2 {
		t.Errorf("중복 카운터 값 = %v, 기대값 = 2", val)
	}
}

// TestMetricsCollector_DuplicateHistogram 은 동일한 (name, component) 로 Histogram 을 호출하면
// 같은 메트릭을 반환하는지 검증한다 (멱등성).
func TestMetricsCollector_DuplicateHistogram(t *testing.T) {
	reg := prometheus.NewRegistry()
	mc := observe.NewMetricsCollector(
		observe.WithRegistry(reg),
	)

	h1 := mc.Histogram("dup_histogram", "agent.mqtt")
	h1.Observe(1.0)

	h2 := mc.Histogram("dup_histogram", "agent.mqtt")
	h2.Observe(2.0)

	// Registry 에서 수집하여 검증
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("메트릭 수집 실패: %v", err)
	}

	for _, mf := range mfs {
		if mf.GetName() == "xflow_component_dup_histogram" {
			metrics := mf.GetMetric()
			if len(metrics) == 0 {
				t.Fatal("히스토그램 메트릭이 비어있다")
			}
			h := metrics[0].GetHistogram()
			if h.GetSampleCount() != 2 {
				t.Errorf("중복 히스토그램 샘플 수 = %v, 기대값 = 2", h.GetSampleCount())
			}
			if h.GetSampleSum() != 3.0 {
				t.Errorf("중복 히스토그램 샘플 합 = %v, 기대값 = 3.0", h.GetSampleSum())
			}
			return
		}
	}
	t.Error("xflow_component_dup_histogram 히스토그램을 찾을 수 없다")
}

// TestMetricsCollector_DuplicateGauge 는 동일한 (name, component) 로 Gauge 를 호출하면
// 같은 메트릭을 반환하는지 검증한다 (멱등성).
func TestMetricsCollector_DuplicateGauge(t *testing.T) {
	mc := observe.NewMetricsCollector(
		observe.WithRegistry(prometheus.NewRegistry()),
	)

	g1 := mc.Gauge("dup_gauge", "agent.mqtt")
	g1.Set(100)

	g2 := mc.Gauge("dup_gauge", "agent.mqtt")

	// 같은 메트릭이므로 g2 로 읽어도 100 이어야 한다
	col, ok := g2.(prometheus.Collector)
	if !ok {
		t.Fatal("Gauge 가 prometheus.Collector 를 구현하지 않는다")
	}
	val := testutil.ToFloat64(col)
	if val != 100 {
		t.Errorf("중복 게이지 값 = %v, 기대값 = 100", val)
	}
}

// TestMetricsCollector_PreDefinedMetrics 는 생성 시 사전 정의된 메트릭 3 종이 등록되는지 검증한다.
// 1. xflow_component_messages_total (Counter)
// 2. xflow_component_errors_total (Counter)
// 3. xflow_component_processing_duration_seconds (Histogram)
func TestMetricsCollector_PreDefinedMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	mc := observe.NewMetricsCollector(
		observe.WithRegistry(reg),
	)

	// 사전 정의된 메트릭을 사용하여 값을 기록한다
	mc.Counter("messages_total", "test").Inc()
	mc.Counter("errors_total", "test").Inc()
	mc.Histogram("processing_duration_seconds", "test").Observe(0.1)

	// Registry 에서 수집하여 검증
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("메트릭 수집 실패: %v", err)
	}

	expectedNames := map[string]bool{
		"xflow_component_messages_total":               false,
		"xflow_component_errors_total":                 false,
		"xflow_component_processing_duration_seconds":  false,
	}

	for _, mf := range mfs {
		if _, ok := expectedNames[mf.GetName()]; ok {
			expectedNames[mf.GetName()] = true
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("사전 정의 메트릭 '%s' 를 찾을 수 없다", name)
		}
	}
}

// TestMetricsCollector_DifferentComponents 는 동일한 메트릭 이름에 다른 컴포넌트를 사용하면
// 독립된 값을 가지는지 검증한다.
func TestMetricsCollector_DifferentComponents(t *testing.T) {
	mc := observe.NewMetricsCollector(
		observe.WithRegistry(prometheus.NewRegistry()),
	)

	c1 := mc.Counter("requests", "agent.mqtt")
	c2 := mc.Counter("requests", "agent.http")

	c1.Inc()
	c1.Inc()
	c1.Inc()

	c2.Inc()

	col1, ok1 := c1.(prometheus.Collector)
	col2, ok2 := c2.(prometheus.Collector)
	if !ok1 || !ok2 {
		t.Fatal("Counter 가 prometheus.Collector 를 구현하지 않는다")
	}

	val1 := testutil.ToFloat64(col1)
	val2 := testutil.ToFloat64(col2)

	if val1 != 3 {
		t.Errorf("agent.mqtt 카운터 값 = %v, 기대값 = 3", val1)
	}
	if val2 != 1 {
		t.Errorf("agent.http 카운터 값 = %v, 기대값 = 1", val2)
	}
}

// TestMetricsCollector_CustomRegistry 는 WithRegistry 옵션으로 커스텀 레지스트리를 설정할 수 있는지 검증한다.
func TestMetricsCollector_CustomRegistry(t *testing.T) {
	customReg := prometheus.NewRegistry()
	mc := observe.NewMetricsCollector(
		observe.WithRegistry(customReg),
	)

	// 반환된 Registry 가 커스텀 레지스트리인지 확인
	if mc.Registry() != customReg {
		t.Error("Registry() 가 커스텀 레지스트리를 반환하지 않는다")
	}

	// 메트릭을 기록한 후 커스텀 레지스트리에서 수집 가능한지 확인
	mc.Counter("custom_test", "test").Inc()
	mfs, err := customReg.Gather()
	if err != nil {
		t.Fatalf("커스텀 레지스트리에서 메트릭 수집 실패: %v", err)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "xflow_component_custom_test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("커스텀 레지스트리에서 xflow_component_custom_test 를 찾을 수 없다")
	}
}

// TestMetricsCollector_CustomNamespace 는 WithNamespace 옵션으로 메트릭 접두사를 변경할 수 있는지 검증한다.
func TestMetricsCollector_CustomNamespace(t *testing.T) {
	reg := prometheus.NewRegistry()
	mc := observe.NewMetricsCollector(
		observe.WithRegistry(reg),
		observe.WithNamespace("myapp"),
	)

	mc.Counter("requests_total", "test").Inc()

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("메트릭 수집 실패: %v", err)
	}

	found := false
	for _, mf := range mfs {
		// myapp_component_requests_total 이어야 한다
		if mf.GetName() == "myapp_component_requests_total" {
			found = true
			break
		}
	}
	if !found {
		t.Error("커스텀 네임스페이스 메트릭 myapp_component_requests_total 을 찾을 수 없다")
	}
}

// TestMetricsCollector_Concurrent 는 여러 고루틴에서 동시에 메트릭을 생성하고 기록해도
// 데이터 레이스 없이 올바르게 동작하는지 검증한다.
func TestMetricsCollector_Concurrent(t *testing.T) {
	mc := observe.NewMetricsCollector(
		observe.WithRegistry(prometheus.NewRegistry()),
	)

	const goroutines = 100
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				mc.Counter("concurrent_counter", "agent.mqtt").Inc()
				mc.Histogram("concurrent_histogram", "agent.mqtt").Observe(0.1)
				mc.Gauge("concurrent_gauge", "agent.mqtt").Add(1)
			}
		}()
	}

	wg.Wait()

	// 카운터 값 검증: goroutines * iterations = 5000
	cc := mc.Counter("concurrent_counter", "agent.mqtt")
	col, ok := cc.(prometheus.Collector)
	if !ok {
		t.Fatal("Counter 가 prometheus.Collector 를 구현하지 않는다")
	}
	val := testutil.ToFloat64(col)
	expected := float64(goroutines * iterations)
	if val != expected {
		t.Errorf("동시성 카운터 값 = %v, 기대값 = %v", val, expected)
	}
}
