package observe

import (
	"io"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
)

// --- FactoryOption: LoggerFactory 설정 ---

// FactoryOption 은 LoggerFactory 를 설정하는 함수 옵션이다.
type FactoryOption func(*factoryConfig)

// factoryConfig 는 LoggerFactory 의 내부 설정을 담는 구조체이다.
type factoryConfig struct {
	defaultLevel slog.Level
	handler      slog.Handler
	levelManager LevelManager
	streamRouter StreamRouter
}

// WithDefaultLevel 은 기본 로그 레벨을 설정한다.
func WithDefaultLevel(level slog.Level) FactoryOption {
	return func(c *factoryConfig) {
		c.defaultLevel = level
	}
}

// WithHandler 는 slog.Handler 를 설정한다.
func WithHandler(h slog.Handler) FactoryOption {
	return func(c *factoryConfig) {
		c.handler = h
	}
}

// WithLevelManager 는 LevelManager 를 설정한다.
func WithLevelManager(lm LevelManager) FactoryOption {
	return func(c *factoryConfig) {
		c.levelManager = lm
	}
}

// WithStreamRouter 는 StreamRouter 를 설정한다.
func WithStreamRouter(sr StreamRouter) FactoryOption {
	return func(c *factoryConfig) {
		c.streamRouter = sr
	}
}

// --- MetricsOption: MetricsCollector 설정 ---

// MetricsOption 은 MetricsCollector 를 설정하는 함수 옵션이다.
type MetricsOption func(*metricsConfig)

// metricsConfig 는 MetricsCollector 의 내부 설정을 담는 구조체이다.
type metricsConfig struct {
	registry  *prometheus.Registry
	namespace string
	subsystem string
}

// WithRegistry 는 Prometheus Registry 를 설정한다.
func WithRegistry(r *prometheus.Registry) MetricsOption {
	return func(c *metricsConfig) {
		c.registry = r
	}
}

// WithNamespace 는 메트릭 네임스페이스를 설정한다.
func WithNamespace(ns string) MetricsOption {
	return func(c *metricsConfig) {
		c.namespace = ns
	}
}

// WithSubsystem 은 메트릭 서브시스템을 설정한다.
func WithSubsystem(ss string) MetricsOption {
	return func(c *metricsConfig) {
		c.subsystem = ss
	}
}

// --- TracerOption: Tracer 설정 ---

// TracerOption 은 Tracer 를 설정하는 함수 옵션이다.
type TracerOption func(*tracerConfig)

// tracerConfig 는 Tracer 의 내부 설정을 담는 구조체이다.
type tracerConfig struct {
	samplingRate float64
	maxSpans     int
	enabled      bool
}

// WithSamplingRate 는 트레이싱 샘플링 비율을 설정한다.
func WithSamplingRate(rate float64) TracerOption {
	return func(c *tracerConfig) {
		c.samplingRate = rate
	}
}

// WithMaxSpans 는 최대 스팬 수를 설정한다.
func WithMaxSpans(n int) TracerOption {
	return func(c *tracerConfig) {
		c.maxSpans = n
	}
}

// WithTracerEnabled 는 트레이서 활성화 여부를 설정한다.
func WithTracerEnabled(enabled bool) TracerOption {
	return func(c *tracerConfig) {
		c.enabled = enabled
	}
}

// --- StreamOption: StreamRouter 설정 ---

// StreamOption 은 StreamRouter 를 설정하는 함수 옵션이다.
type StreamOption func(*streamConfig)

// streamConfig 는 StreamRouter 의 내부 설정을 담는 구조체이다.
type streamConfig struct {
	defaultWriter io.Writer
	format        string // "json" 또는 "text"
	levelManager  LevelManager
}

// WithDefaultWriter 는 기본 출력 Writer 를 설정한다.
func WithDefaultWriter(w io.Writer) StreamOption {
	return func(c *streamConfig) {
		c.defaultWriter = w
	}
}

// WithFormat 은 출력 포맷을 설정한다. ("json" 또는 "text")
func WithFormat(format string) StreamOption {
	return func(c *streamConfig) {
		c.format = format
	}
}

// WithStreamLevelManager 는 StreamRouter 에 레벨 필터링용 LevelManager 를 설정한다.
func WithStreamLevelManager(lm LevelManager) StreamOption {
	return func(c *streamConfig) {
		c.levelManager = lm
	}
}

// --- Option: Observer 통합 생성자 설정 ---

// Option 은 Observer 통합 생성자를 설정하는 함수 옵션이다.
type Option func(*observerConfig)

// observerConfig 는 Observer 의 통합 설정으로, 모든 하위 설정을 포함한다.
type observerConfig struct {
	factoryConfig
	metricsConfig
	tracerConfig
	streamConfig
}

// WithObserverDefaultLevel 은 Observer 의 기본 로그 레벨을 설정한다.
func WithObserverDefaultLevel(level slog.Level) Option {
	return func(c *observerConfig) {
		c.factoryConfig.defaultLevel = level
	}
}

// WithObserverWriter 는 Observer 의 StreamRouter 기본 Writer 를 설정한다.
func WithObserverWriter(w io.Writer) Option {
	return func(c *observerConfig) {
		c.streamConfig.defaultWriter = w
	}
}

// WithObserverFormat 은 Observer 의 로그 출력 포맷을 설정한다. ("json" 또는 "text")
func WithObserverFormat(format string) Option {
	return func(c *observerConfig) {
		c.streamConfig.format = format
	}
}

// WithObserverRegistry 는 Observer 의 Prometheus Registry 를 설정한다.
func WithObserverRegistry(r *prometheus.Registry) Option {
	return func(c *observerConfig) {
		c.metricsConfig.registry = r
	}
}

// WithObserverNamespace 는 Observer 의 메트릭 네임스페이스를 설정한다.
func WithObserverNamespace(ns string) Option {
	return func(c *observerConfig) {
		c.metricsConfig.namespace = ns
	}
}

// WithObserverTracerEnabled 는 Observer 의 Tracer 활성화 여부를 설정한다.
func WithObserverTracerEnabled(enabled bool) Option {
	return func(c *observerConfig) {
		c.tracerConfig.enabled = enabled
	}
}

// WithObserverSamplingRate 는 Observer 의 Tracer 샘플링 비율을 설정한다. (0.0 ~ 1.0)
func WithObserverSamplingRate(rate float64) Option {
	return func(c *observerConfig) {
		c.tracerConfig.samplingRate = rate
	}
}
