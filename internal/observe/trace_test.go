package observe_test

import (
	"sync"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/observe"
)

// TestTracer_DefaultDisabled 는 새 Tracer 가 기본적으로 비활성화 상태인지 검증한다.
func TestTracer_DefaultDisabled(t *testing.T) {
	tracer := observe.NewTracer()

	if tracer.Enabled() {
		t.Error("새 Tracer 가 기본적으로 활성화되어 있다, 기대값: 비활성화")
	}
}

// TestTracer_EnableDisable 는 SetEnabled 로 활성/비활성 상태를 전환할 수 있는지 검증한다.
func TestTracer_EnableDisable(t *testing.T) {
	tracer := observe.NewTracer()

	// 활성화
	tracer.SetEnabled(true)
	if !tracer.Enabled() {
		t.Error("SetEnabled(true) 후 Enabled() = false, 기대값: true")
	}

	// 비활성화
	tracer.SetEnabled(false)
	if tracer.Enabled() {
		t.Error("SetEnabled(false) 후 Enabled() = true, 기대값: false")
	}
}

// TestTracer_StartSpan_Enabled 는 활성화된 Tracer 가 올바른 필드를 가진 실제 Span 을 반환하는지 검증한다.
func TestTracer_StartSpan_Enabled(t *testing.T) {
	tracer := observe.NewTracer(observe.WithTracerEnabled(true))

	span := tracer.StartSpan("trace-123", "engine", "process")

	if span.TraceID() != "trace-123" {
		t.Errorf("TraceID() = %q, 기대값 \"trace-123\"", span.TraceID())
	}
	if span.Component() != "engine" {
		t.Errorf("Component() = %q, 기대값 \"engine\"", span.Component())
	}
	if span.Operation() != "process" {
		t.Errorf("Operation() = %q, 기대값 \"process\"", span.Operation())
	}
}

// TestTracer_StartSpan_Disabled 는 비활성화된 Tracer 가 noop span 을 반환하는지 검증한다.
func TestTracer_StartSpan_Disabled(t *testing.T) {
	tracer := observe.NewTracer() // 기본 비활성화

	span := tracer.StartSpan("trace-456", "agent", "send")

	// noop span 은 빈 문자열을 반환한다
	if span.TraceID() != "" {
		t.Errorf("비활성화 시 TraceID() = %q, 기대값 \"\"", span.TraceID())
	}
	if span.Component() != "" {
		t.Errorf("비활성화 시 Component() = %q, 기대값 \"\"", span.Component())
	}
	if span.Operation() != "" {
		t.Errorf("비활성화 시 Operation() = %q, 기대값 \"\"", span.Operation())
	}

	// noop span 메서드들은 패닉 없이 실행되어야 한다
	span.SetAttribute("key", "value")
	span.AddEvent("event", "attr", 42)
	span.End()
}

// TestTracer_NoopSpan_ZeroDuration 는 noop span 의 Duration 이 0 인지 검증한다.
func TestTracer_NoopSpan_ZeroDuration(t *testing.T) {
	tracer := observe.NewTracer() // 비활성화

	span := tracer.StartSpan("t", "c", "op")
	span.End()

	if d := span.Duration(); d != 0 {
		t.Errorf("noop span Duration() = %v, 기대값 0", d)
	}
}

// TestTracer_NoopSpan_EmptyFields 는 noop span 의 문자열 필드가 빈 값인지 검증한다.
func TestTracer_NoopSpan_EmptyFields(t *testing.T) {
	tracer := observe.NewTracer() // 비활성화

	span := tracer.StartSpan("trace-1", "comp", "op")

	if span.TraceID() != "" {
		t.Errorf("noop TraceID() = %q, 기대값 \"\"", span.TraceID())
	}
	if span.Component() != "" {
		t.Errorf("noop Component() = %q, 기대값 \"\"", span.Component())
	}
	if span.Operation() != "" {
		t.Errorf("noop Operation() = %q, 기대값 \"\"", span.Operation())
	}
}

// TestTracer_NoopSpan_ZeroAlloc 는 비활성화된 Tracer 에서 StartSpan 이 힙 할당을 하지 않는지 검증한다.
func TestTracer_NoopSpan_ZeroAlloc(t *testing.T) {
	tracer := observe.NewTracer() // 비활성화

	allocs := testing.AllocsPerRun(100, func() {
		span := tracer.StartSpan("trace-1", "comp", "op")
		span.End()
	})
	if allocs != 0 {
		t.Errorf("비활성화 Tracer 에서 할당 수 = %f, 기대값 0", allocs)
	}
}

// TestTracer_Span_End 는 End() 호출 후 Duration 이 양수인지 검증한다.
func TestTracer_Span_End(t *testing.T) {
	tracer := observe.NewTracer(observe.WithTracerEnabled(true))

	span := tracer.StartSpan("trace-1", "comp", "op")

	// End 전 Duration 은 0 이어야 한다
	if d := span.Duration(); d != 0 {
		t.Errorf("End 전 Duration() = %v, 기대값 0", d)
	}

	time.Sleep(1 * time.Millisecond) // 측정 가능한 시간 경과
	span.End()

	d := span.Duration()
	if d <= 0 {
		t.Errorf("End 후 Duration() = %v, 기대값: 양수", d)
	}
}

// TestTracer_Span_DoubleEnd 는 End() 를 두 번 호출해도 Duration 이 변하지 않는지 검증한다.
func TestTracer_Span_DoubleEnd(t *testing.T) {
	tracer := observe.NewTracer(observe.WithTracerEnabled(true))

	span := tracer.StartSpan("trace-1", "comp", "op")
	time.Sleep(1 * time.Millisecond)
	span.End()

	firstDuration := span.Duration()

	time.Sleep(2 * time.Millisecond)
	span.End() // 두 번째 End

	secondDuration := span.Duration()

	if firstDuration != secondDuration {
		t.Errorf("두 번째 End 후 Duration 이 변경되었다: %v -> %v", firstDuration, secondDuration)
	}
}

// TestTracer_Span_SetAttribute 는 Span 에 속성을 설정할 수 있는지 검증한다.
func TestTracer_Span_SetAttribute(t *testing.T) {
	tracer := observe.NewTracer(observe.WithTracerEnabled(true))

	span := tracer.StartSpan("trace-1", "comp", "op")
	// SetAttribute 가 패닉 없이 실행되어야 한다
	span.SetAttribute("key1", "value1")
	span.SetAttribute("key2", 42)
	span.SetAttribute("key3", true)
	span.End()
}

// TestTracer_Span_AddEvent 는 Span 에 이벤트를 추가할 수 있는지 검증한다.
func TestTracer_Span_AddEvent(t *testing.T) {
	tracer := observe.NewTracer(observe.WithTracerEnabled(true))

	span := tracer.StartSpan("trace-1", "comp", "op")
	// AddEvent 가 패닉 없이 실행되어야 한다
	span.AddEvent("event1")
	span.AddEvent("event2", "attr1", "val1", "attr2", 42)
	span.End()
}

// TestTracer_SamplingRate 는 SetSamplingRate/SamplingRate 가 올바르게 동작하는지 검증한다.
func TestTracer_SamplingRate(t *testing.T) {
	tracer := observe.NewTracer()

	// 기본 샘플링 비율은 1.0 이어야 한다
	if rate := tracer.SamplingRate(); rate != 1.0 {
		t.Errorf("기본 SamplingRate() = %f, 기대값 1.0", rate)
	}

	// 변경 후 조회
	tracer.SetSamplingRate(0.5)
	if rate := tracer.SamplingRate(); rate != 0.5 {
		t.Errorf("SetSamplingRate(0.5) 후 SamplingRate() = %f, 기대값 0.5", rate)
	}

	tracer.SetSamplingRate(0.0)
	if rate := tracer.SamplingRate(); rate != 0.0 {
		t.Errorf("SetSamplingRate(0.0) 후 SamplingRate() = %f, 기대값 0.0", rate)
	}
}

// TestTracer_Sampling_All 은 샘플링 비율 1.0 에서 모든 span 이 실제 span 인지 검증한다.
func TestTracer_Sampling_All(t *testing.T) {
	tracer := observe.NewTracer(observe.WithTracerEnabled(true))
	tracer.SetSamplingRate(1.0)

	for i := 0; i < 100; i++ {
		span := tracer.StartSpan("t", "c", "op")
		if span.TraceID() == "" {
			t.Fatalf("샘플링 비율 1.0 에서 noop span 이 반환되었다 (반복 %d)", i)
		}
	}
}

// TestTracer_Sampling_None 은 샘플링 비율 0.0 에서 모든 span 이 noop span 인지 검증한다.
func TestTracer_Sampling_None(t *testing.T) {
	tracer := observe.NewTracer(observe.WithTracerEnabled(true))
	tracer.SetSamplingRate(0.0)

	for i := 0; i < 100; i++ {
		span := tracer.StartSpan("t", "c", "op")
		if span.TraceID() != "" {
			t.Fatalf("샘플링 비율 0.0 에서 실제 span 이 반환되었다 (반복 %d)", i)
		}
	}
}

// TestTracer_Sampling_Half 는 샘플링 비율 0.5 에서 약 50% 의 실제 span 이 생성되는지 검증한다.
func TestTracer_Sampling_Half(t *testing.T) {
	tracer := observe.NewTracer(observe.WithTracerEnabled(true))
	tracer.SetSamplingRate(0.5)

	realCount := 0
	const iterations = 1000
	for i := 0; i < iterations; i++ {
		span := tracer.StartSpan("t", "c", "op")
		if span.TraceID() != "" {
			realCount++
		}
	}

	// 50% +/- 10% 허용 범위
	if realCount < 400 || realCount > 600 {
		t.Errorf("샘플링 비율 0.5 에서 %d/%d 실제 span (기대범위: 400-600)", realCount, iterations)
	}
}

// TestTracer_WithOptions 는 WithSamplingRate, WithMaxSpans, WithTracerEnabled 옵션이 동작하는지 검증한다.
func TestTracer_WithOptions(t *testing.T) {
	tracer := observe.NewTracer(
		observe.WithTracerEnabled(true),
		observe.WithSamplingRate(0.75),
		observe.WithMaxSpans(5000),
	)

	if !tracer.Enabled() {
		t.Error("WithTracerEnabled(true) 설정 후 Enabled() = false")
	}

	if rate := tracer.SamplingRate(); rate != 0.75 {
		t.Errorf("WithSamplingRate(0.75) 설정 후 SamplingRate() = %f", rate)
	}
}

// TestTracer_Concurrent 는 Tracer 의 동시성 안전성을 검증한다.
func TestTracer_Concurrent(t *testing.T) {
	tracer := observe.NewTracer(observe.WithTracerEnabled(true))

	var wg sync.WaitGroup
	const goroutines = 10
	const iterations = 100

	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			for j := range iterations {
				switch j % 5 {
				case 0:
					span := tracer.StartSpan("trace", "comp", "op")
					span.SetAttribute("key", id)
					span.AddEvent("event")
					span.End()
				case 1:
					tracer.SetEnabled(true)
				case 2:
					tracer.SetEnabled(false)
				case 3:
					tracer.SetSamplingRate(0.5)
				case 4:
					_ = tracer.Enabled()
					_ = tracer.SamplingRate()
				}
			}
		}(i)
	}

	wg.Wait()
	// 패닉 없이 완료되면 성공
}

// TestTracer_Interface 는 Tracer 인터페이스를 구현하는지 컴파일 타임에 검증한다.
func TestTracer_Interface(t *testing.T) {
	var _ observe.Tracer = observe.NewTracer()
}

// TestSpan_Interface 는 Span 인터페이스를 구현하는지 컴파일 타임에 검증한다.
func TestSpan_Interface(t *testing.T) {
	tracer := observe.NewTracer(observe.WithTracerEnabled(true))
	var _ observe.Span = tracer.StartSpan("t", "c", "op")
}

// BenchmarkStartSpanDisabled 는 비활성화 시 StartSpan 의 성능을 측정한다.
// 0 allocs/op 이어야 한다.
func BenchmarkStartSpanDisabled(b *testing.B) {
	tracer := observe.NewTracer() // 기본 비활성화
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		span := tracer.StartSpan("trace-1", "comp", "op")
		span.End()
	}
}

// BenchmarkStartSpanEnabled 는 활성화 시 StartSpan 의 기본 성능을 측정한다.
func BenchmarkStartSpanEnabled(b *testing.B) {
	tracer := observe.NewTracer(observe.WithTracerEnabled(true))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		span := tracer.StartSpan("trace-1", "comp", "op")
		span.End()
	}
}
