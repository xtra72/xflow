package observe

import (
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"
)

// Tracer 는 트레이싱 인터페이스이다.
// StartSpan 으로 Span 을 생성하고, 활성화/샘플링 설정을 관리한다.
type Tracer interface {
	// StartSpan 은 새 Span 을 시작한다.
	// 비활성화 상태이거나 샘플링으로 제외되면 noopSpan 싱글턴을 반환한다 (제로 할당).
	StartSpan(traceID string, component string, operation string) Span

	// Enabled 는 트레이서 활성화 상태를 반환한다.
	Enabled() bool

	// SetEnabled 는 트레이서 활성화 상태를 설정한다. (원자적)
	SetEnabled(enabled bool)

	// SetSamplingRate 는 샘플링 비율을 설정한다. (0.0 ~ 1.0)
	SetSamplingRate(rate float64)

	// SamplingRate 는 현재 샘플링 비율을 반환한다.
	SamplingRate() float64
}

// Span 은 트레이스 Span 인터페이스이다.
type Span interface {
	// End 는 Span 을 종료하고 종료 시각을 기록한다.
	End()

	// TraceID 는 트레이스 ID 를 반환한다.
	TraceID() string

	// Component 는 컴포넌트 이름을 반환한다.
	Component() string

	// Operation 은 오퍼레이션 이름을 반환한다.
	Operation() string

	// Duration 은 Span 의 실행 시간을 반환한다.
	// End() 호출 전에는 0 을 반환한다.
	Duration() time.Duration

	// SetAttribute 는 키-값 속성을 설정한다. (스레드 안전)
	SetAttribute(key string, value any)

	// AddEvent 는 이벤트를 추가한다. (스레드 안전)
	AddEvent(name string, attrs ...any)
}

// globalNoopSpan 은 비활성화 시 반환되는 전역 noop span 싱글턴이다.
// 제로 할당을 보장한다.
var globalNoopSpan Span = &noopSpan{}

// noopSpan 은 아무 작업도 수행하지 않는 빈 Span 구현체이다.
// 필드가 없는 빈 구조체로, 전역 싱글턴으로 사용된다.
type noopSpan struct{}

func (n *noopSpan) End()                              {}
func (n *noopSpan) TraceID() string                   { return "" }
func (n *noopSpan) Component() string                 { return "" }
func (n *noopSpan) Operation() string                 { return "" }
func (n *noopSpan) Duration() time.Duration           { return 0 }
func (n *noopSpan) SetAttribute(_ string, _ any)      {}
func (n *noopSpan) AddEvent(_ string, _ ...any)       {}

// spanEvent 는 Span 에 기록된 이벤트를 나타낸다.
type spanEvent struct {
	name  string
	attrs []any
}

// span 은 실제 트레이스 Span 구현체이다.
type span struct {
	traceID   string
	component string
	operation string
	startTime time.Time
	endTime   time.Time
	ended     bool

	mu         sync.Mutex
	attributes map[string]any
	events     []spanEvent
}

// End 는 Span 을 종료한다. 중복 호출 시 첫 번째 종료 시각을 유지한다.
func (s *span) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return
	}
	s.endTime = time.Now()
	s.ended = true
}

// TraceID 는 트레이스 ID 를 반환한다.
func (s *span) TraceID() string { return s.traceID }

// Component 는 컴포넌트 이름을 반환한다.
func (s *span) Component() string { return s.component }

// Operation 은 오퍼레이션 이름을 반환한다.
func (s *span) Operation() string { return s.operation }

// Duration 은 Span 의 실행 시간을 반환한다.
// End() 호출 전에는 0 을 반환한다.
func (s *span) Duration() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ended {
		return 0
	}
	return s.endTime.Sub(s.startTime)
}

// SetAttribute 는 키-값 속성을 설정한다.
func (s *span) SetAttribute(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attributes == nil {
		s.attributes = make(map[string]any)
	}
	s.attributes[key] = value
}

// AddEvent 는 이벤트를 추가한다.
func (s *span) AddEvent(name string, attrs ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, spanEvent{name: name, attrs: attrs})
}

// tracer 는 Tracer 인터페이스의 구현체이다.
type tracer struct {
	enabled      atomic.Bool
	samplingRate atomic.Value // float64 저장
	maxSpans     int
}

// NewTracer 는 지정된 옵션으로 새 Tracer 를 생성한다.
// 기본값: 비활성화, 샘플링 비율 1.0, 최대 스팬 수 10000.
func NewTracer(opts ...TracerOption) Tracer {
	cfg := &tracerConfig{
		samplingRate: 1.0,
		maxSpans:     10000,
		enabled:      false,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	t := &tracer{
		maxSpans: cfg.maxSpans,
	}
	t.enabled.Store(cfg.enabled)
	t.samplingRate.Store(cfg.samplingRate)

	return t
}

// StartSpan 은 새 Span 을 시작한다.
// 비활성화 상태이거나 샘플링으로 제외되면 globalNoopSpan 싱글턴을 반환한다.
func (t *tracer) StartSpan(traceID string, component string, operation string) Span {
	if !t.enabled.Load() {
		return globalNoopSpan
	}

	// 샘플링 확인
	rate := t.samplingRate.Load().(float64)
	if rate < 1.0 {
		if rate <= 0.0 || rand.Float64() >= rate {
			return globalNoopSpan
		}
	}

	return &span{
		traceID:   traceID,
		component: component,
		operation: operation,
		startTime: time.Now(),
	}
}

// Enabled 는 트레이서 활성화 상태를 반환한다.
func (t *tracer) Enabled() bool {
	return t.enabled.Load()
}

// SetEnabled 는 트레이서 활성화 상태를 원자적으로 설정한다.
func (t *tracer) SetEnabled(enabled bool) {
	t.enabled.Store(enabled)
}

// SetSamplingRate 는 샘플링 비율을 원자적으로 설정한다. (0.0 ~ 1.0)
func (t *tracer) SetSamplingRate(rate float64) {
	t.samplingRate.Store(rate)
}

// SamplingRate 는 현재 샘플링 비율을 반환한다.
func (t *tracer) SamplingRate() float64 {
	return t.samplingRate.Load().(float64)
}
