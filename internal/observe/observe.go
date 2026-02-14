package observe

import (
	"log/slog"
)

// Observer 는 관찰성 시스템의 통합 진입점이다.
// 로깅, 메트릭, 트레이싱, 스트림 라우팅을 하나의 구조체로 통합한다.
type Observer struct {
	// Loggers 는 컴포넌트별 로거를 생성하고 관리하는 팩토리이다.
	Loggers LoggerFactory

	// Levels 는 컴포넌트별 로그 레벨을 관리한다.
	Levels LevelManager

	// Metrics 는 메트릭 수집기이다.
	Metrics MetricsCollector

	// Tracer 는 트레이싱 시스템이다.
	Tracer Tracer

	// Streams 는 컴포넌트별 로그 스트림 라우터이다.
	Streams StreamRouter
}

// New 는 지정된 옵션으로 Observer 통합 인스턴스를 생성한다.
// 옵션을 전달하지 않으면 기본값이 적용된다:
//   - 기본 로그 레벨: slog.LevelInfo
//   - 기본 출력: os.Stdout (StreamRouter 기본값)
//   - 기본 포맷: "json"
//   - Tracer: 비활성화
//   - 메트릭 네임스페이스: "xflow"
func New(opts ...Option) *Observer {
	// 옵션을 observerConfig 로 파싱한다
	cfg := &observerConfig{}
	cfg.factoryConfig.defaultLevel = slog.LevelInfo
	for _, opt := range opts {
		opt(cfg)
	}

	// 1. LevelManager 를 생성한다
	levels := NewLevelManager(cfg.factoryConfig.defaultLevel)

	// 2. StreamRouter 를 생성한다 (LevelManager 연동)
	streamOpts := []StreamOption{
		WithStreamLevelManager(levels),
	}
	if cfg.streamConfig.defaultWriter != nil {
		streamOpts = append(streamOpts, WithDefaultWriter(cfg.streamConfig.defaultWriter))
	}
	if cfg.streamConfig.format != "" {
		streamOpts = append(streamOpts, WithFormat(cfg.streamConfig.format))
	}
	streams := NewStreamRouter(streamOpts...)

	// 3. LoggerFactory 를 생성한다 (LevelManager + StreamRouter 연동)
	loggers := NewLoggerFactory(
		WithLevelManager(levels),
		WithStreamRouter(streams),
	)

	// 4. MetricsCollector 를 생성한다
	var metricsOpts []MetricsOption
	if cfg.metricsConfig.registry != nil {
		metricsOpts = append(metricsOpts, WithRegistry(cfg.metricsConfig.registry))
	}
	if cfg.metricsConfig.namespace != "" {
		metricsOpts = append(metricsOpts, WithNamespace(cfg.metricsConfig.namespace))
	}
	if cfg.metricsConfig.subsystem != "" {
		metricsOpts = append(metricsOpts, WithSubsystem(cfg.metricsConfig.subsystem))
	}
	metrics := NewMetricsCollector(metricsOpts...)

	// 5. Tracer 를 생성한다
	var tracerOpts []TracerOption
	tracerOpts = append(tracerOpts, WithTracerEnabled(cfg.tracerConfig.enabled))
	if cfg.tracerConfig.samplingRate != 0 {
		tracerOpts = append(tracerOpts, WithSamplingRate(cfg.tracerConfig.samplingRate))
	}
	if cfg.tracerConfig.maxSpans != 0 {
		tracerOpts = append(tracerOpts, WithMaxSpans(cfg.tracerConfig.maxSpans))
	}
	tracer := NewTracer(tracerOpts...)

	return &Observer{
		Loggers: loggers,
		Levels:  levels,
		Metrics: metrics,
		Tracer:  tracer,
		Streams: streams,
	}
}
