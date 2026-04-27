# observe - XFlow 관찰성 시스템

`internal/observe` 패키지는 XFlow 엔진의 횡단 관심사(cross-cutting concern)인 관찰성(Observability) 시스템을 제공한다. 컴포넌트별 구조화된 로깅, 런타임 로그 레벨 관리, Prometheus 메트릭 수집, 메시지 트레이싱, 로그 스트림 라우팅을 통합적으로 지원한다.

**SPEC**: SPEC-OBS-001

## 아키텍처 개요

```
                    +-------------------+
                    |     Observer      |
                    |  (통합 진입점)      |
                    +-------------------+
                    |                   |
        +-----------+-----------+-------+--------+
        |           |           |       |        |
   +--------+  +--------+  +-------+ +------+ +--------+
   | Logger |  | Level   |  |Metrics| |Tracer| | Stream |
   | Factory|  | Manager |  |Collect| |      | | Router |
   +--------+  +--------+  +-------+ +------+ +--------+
        |           |           |       |        |
   ComponentLogger  LevelVar   Prom    Span   slog.Handler
   (slog 래핑)     (atomic)   Registry (noop) (라우팅)
```

**5개 모듈 구성**:

1. **LoggerFactory + ComponentLogger**: slog 기반 컴포넌트별 구조화된 로깅
2. **LevelManager**: slog.LevelVar를 사용한 런타임 로그 레벨 관리 (와일드카드 패턴 매칭)
3. **MetricsCollector**: Prometheus client_golang 통합 메트릭 수집 (Counter, Histogram, Gauge)
4. **Tracer + Span**: noopSpan 제로 오버헤드 패턴 기반 메시지 트레이싱 및 샘플링
5. **StreamRouter**: slog.Handler 합성을 통한 컴포넌트별 로그 스트림 라우팅 (fan-out)

## 빠른 시작

### Observer 통합 생성

`Observer`는 모든 하위 시스템을 한 번에 초기화하는 통합 진입점이다.

```go
package main

import (
    "log/slog"
    "os"

    "github.com/xtra/xflow/internal/observe"
)

func main() {
    // Observer 생성 (기본값: INFO 레벨, JSON 포맷, stdout 출력, Tracer 비활성화)
    obs := observe.New()

    // 옵션 지정 생성
    obs = observe.New(
        observe.WithObserverDefaultLevel(slog.LevelDebug),
        observe.WithObserverFormat("text"),
        observe.WithObserverWriter(os.Stderr),
        observe.WithObserverTracerEnabled(true),
        observe.WithObserverSamplingRate(0.5),
        observe.WithObserverNamespace("myapp"),
    )

    // 컴포넌트 로거 생성
    logger := obs.Loggers.NewLogger("engine.scheduler")
    logger.Info("스케줄러 시작", "workers", 4)
    logger.Debug("디버그 정보", "state", "idle")

    // 구조화된 필드 추가
    reqLogger := logger.With("request_id", "req-123")
    reqLogger.Info("요청 처리 중")

    // 런타임 로그 레벨 변경
    obs.Levels.SetLevel("engine.scheduler", slog.LevelWarn)

    // 와일드카드 패턴으로 일괄 변경
    obs.Levels.SetLevelByPattern("agent.*", slog.LevelDebug)

    // 메트릭 기록
    obs.Metrics.Counter("messages_total", "engine.scheduler").Inc()
    obs.Metrics.Histogram("processing_duration_seconds", "engine.scheduler").Observe(0.035)

    // 트레이싱 (활성화된 경우)
    span := obs.Tracer.StartSpan("trace-001", "engine.scheduler", "process")
    span.SetAttribute("msg_type", "sensor_data")
    defer span.End()

    // 로그 스트림 라우팅
    logFile, _ := os.Create("agent.log")
    obs.Streams.AddRoute("agent.mqtt", logFile)
}
```

### 개별 모듈 사용

각 하위 시스템은 독립적으로 생성하여 사용할 수 있다.

```go
// LevelManager 단독 사용
levels := observe.NewLevelManager(slog.LevelInfo)
levels.SetLevel("agent.mqtt", slog.LevelDebug)

// StreamRouter 단독 사용
streams := observe.NewStreamRouter(
    observe.WithFormat("text"),
    observe.WithStreamLevelManager(levels),
)

// LoggerFactory 단독 사용 (LevelManager + StreamRouter 연동)
loggers := observe.NewLoggerFactory(
    observe.WithLevelManager(levels),
    observe.WithStreamRouter(streams),
)

// MetricsCollector 단독 사용
metrics := observe.NewMetricsCollector(
    observe.WithNamespace("xflow"),
)

// Tracer 단독 사용
tracer := observe.NewTracer(
    observe.WithTracerEnabled(true),
    observe.WithSamplingRate(0.1),
)
```

## API 레퍼런스

### 인터페이스

```go
// ComponentLogger - 컴포넌트별 구조화된 로거
type ComponentLogger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
    With(args ...any) ComponentLogger
    WithGroup(name string) ComponentLogger
    Component() string
    Logger() *slog.Logger
}

// LoggerFactory - 컴포넌트 로거 팩토리
type LoggerFactory interface {
    NewLogger(component string) ComponentLogger
    GetLogger(component string) (ComponentLogger, bool)
    Components() []string
}

// LevelManager - 런타임 로그 레벨 관리
type LevelManager interface {
    GetLevel(component string) slog.Level
    SetLevel(component string, level slog.Level)
    SetLevelByPattern(pattern string, level slog.Level) int
    DefaultLevel() slog.Level
    SetDefaultLevel(level slog.Level)
    Levels() map[string]slog.Level
}

// MetricsCollector - Prometheus 메트릭 수집
type MetricsCollector interface {
    Counter(name string, component string) CounterMetric
    Histogram(name string, component string) HistogramMetric
    Gauge(name string, component string) GaugeMetric
    Registry() *prometheus.Registry
}

// CounterMetric - 카운터 메트릭
type CounterMetric interface {
    Inc()
    Add(float64)
}

// HistogramMetric - 히스토그램 메트릭
type HistogramMetric interface {
    Observe(float64)
}

// GaugeMetric - 게이지 메트릭
type GaugeMetric interface {
    Set(float64)
    Inc()
    Dec()
    Add(float64)
}

// Tracer - 메시지 트레이싱
type Tracer interface {
    StartSpan(traceID string, component string, operation string) Span
    Enabled() bool
    SetEnabled(enabled bool)
    SetSamplingRate(rate float64)
    SamplingRate() float64
}

// Span - 트레이스 Span
type Span interface {
    End()
    TraceID() string
    Component() string
    Operation() string
    Duration() time.Duration
    SetAttribute(key string, value any)
    AddEvent(name string, attrs ...any)
}

// StreamRouter - 로그 스트림 라우팅
type StreamRouter interface {
    AddRoute(component string, writer io.Writer)
    RemoveRoute(component string, writer io.Writer)
    Routes(component string) []io.Writer
    SetDefaultWriter(writer io.Writer)
    Handler() slog.Handler
}
```

### 팩토리 함수

| 함수 | 반환 타입 | 설명 |
|------|----------|------|
| `New(opts ...Option)` | `*Observer` | 통합 Observer 인스턴스 생성 |
| `NewLoggerFactory(opts ...FactoryOption)` | `LoggerFactory` | 로거 팩토리 생성 |
| `NewLevelManager(defaultLevel slog.Level)` | `LevelManager` | 레벨 관리자 생성 |
| `NewMetricsCollector(opts ...MetricsOption)` | `MetricsCollector` | 메트릭 수집기 생성 |
| `NewTracer(opts ...TracerOption)` | `Tracer` | 트레이서 생성 |
| `NewStreamRouter(opts ...StreamOption)` | `StreamRouter` | 스트림 라우터 생성 |

## 설정 옵션

### Observer 옵션 (Option)

| 옵션 함수 | 기본값 | 설명 |
|-----------|--------|------|
| `WithObserverDefaultLevel(level)` | `slog.LevelInfo` | 기본 로그 레벨 |
| `WithObserverWriter(w)` | `os.Stdout` | 기본 출력 대상 |
| `WithObserverFormat(format)` | `"json"` | 출력 포맷 (`"json"` 또는 `"text"`) |
| `WithObserverRegistry(r)` | 새 Registry | Prometheus Registry |
| `WithObserverNamespace(ns)` | `"xflow"` | 메트릭 네임스페이스 |
| `WithObserverTracerEnabled(enabled)` | `false` | Tracer 활성화 여부 |
| `WithObserverSamplingRate(rate)` | `1.0` | 트레이싱 샘플링 비율 (0.0~1.0) |

### LoggerFactory 옵션 (FactoryOption)

| 옵션 함수 | 설명 |
|-----------|------|
| `WithDefaultLevel(level)` | 기본 로그 레벨 |
| `WithHandler(h)` | 커스텀 slog.Handler |
| `WithLevelManager(lm)` | LevelManager 연동 |
| `WithStreamRouter(sr)` | StreamRouter 연동 |

### MetricsCollector 옵션 (MetricsOption)

| 옵션 함수 | 기본값 | 설명 |
|-----------|--------|------|
| `WithRegistry(r)` | 새 Registry | Prometheus Registry |
| `WithNamespace(ns)` | `"xflow"` | 메트릭 네임스페이스 |
| `WithSubsystem(ss)` | `"component"` | 메트릭 서브시스템 |

### Tracer 옵션 (TracerOption)

| 옵션 함수 | 기본값 | 설명 |
|-----------|--------|------|
| `WithTracerEnabled(enabled)` | `false` | 트레이서 활성화 여부 |
| `WithSamplingRate(rate)` | `1.0` | 샘플링 비율 (0.0~1.0) |
| `WithMaxSpans(n)` | `10000` | 최대 Span 보관 수 |

### StreamRouter 옵션 (StreamOption)

| 옵션 함수 | 기본값 | 설명 |
|-----------|--------|------|
| `WithDefaultWriter(w)` | `os.Stdout` | 기본 출력 Writer |
| `WithFormat(format)` | `"json"` | 출력 포맷 (`"json"` 또는 `"text"`) |
| `WithStreamLevelManager(lm)` | `nil` | 레벨 필터링용 LevelManager |

## 사전 정의 메트릭

`NewMetricsCollector()` 호출 시 자동으로 등록되는 메트릭이다.

| 메트릭 이름 | 타입 | 레이블 | 설명 |
|------------|------|--------|------|
| `xflow_component_messages_total` | Counter | `component` | 처리된 메시지 총 수 |
| `xflow_component_errors_total` | Counter | `component` | 발생한 에러 총 수 |
| `xflow_component_processing_duration_seconds` | Histogram | `component` | 처리 소요 시간 |

## 컴포넌트 명명 규칙

dot-notation 계층 구조로 컴포넌트를 식별한다. 빈 문자열은 허용되지 않는다.

| 구성 요소 | 컴포넌트 이름 예시 | 설명 |
|-----------|-------------------|------|
| Flow Engine | `engine`, `engine.scheduler` | 엔진 코어 및 스케줄러 |
| Agent | `agent.mqtt`, `agent.mqtt.client1` | 에이전트 타입 및 인스턴스 |
| Node | `node.filter.node-3`, `node.transform.node-7` | 노드 타입 및 인스턴스 |
| Script Engine | `script`, `script.vm-pool` | 스크립트 엔진 및 하위 구성 |
| Plugin | `plugin.custom-transform` | 플러그인 이름 |
| API Server | `api`, `api.auth` | API 서버 및 하위 핸들러 |

### 와일드카드 패턴 매칭

`SetLevelByPattern()`에서 사용하는 패턴 규칙이다.

| 패턴 | 매칭 대상 | 비매칭 대상 |
|------|----------|------------|
| `agent.*` | `agent.mqtt`, `agent.http`, `agent.mqtt.client1` | `engine`, `node.filter` |
| `agent.mqtt.*` | `agent.mqtt.client1`, `agent.mqtt.client2` | `agent.http`, `agent.mqtt` |
| `*` | 모든 등록된 컴포넌트 | - |
| `node.*` | `node.filter.node-3`, `node.transform.node-7` | `agent.mqtt`, `engine` |

## 설계 특징

- **인터페이스 우선**: 모든 공개 API는 인터페이스로 정의되며, 구현체(struct)는 unexported
- **제로 오버헤드**: Tracer 비활성화 시 `noopSpan` 싱글턴 반환 (0 allocs/op, ~2ns/op)
- **동시성 안전**: `sync.Map`, `slog.LevelVar`, `atomic.Bool/Value` 사용
- **Registry 패턴**: 컴포넌트별 로거와 레벨을 중앙에서 통합 관리
- **slog.Handler 합성**: `routingHandler`가 컴포넌트 속성 기반으로 로그를 라우팅
- **Options 패턴**: 모든 팩토리 함수에 함수 옵션 패턴 적용
- **에러 격리**: StreamRouter의 Writer 에러가 다른 Writer로 전파되지 않음

## 파일 구조

```
internal/observe/
  options.go         # FactoryOption, MetricsOption, TracerOption, StreamOption, Option 정의
  level.go           # LevelManager 인터페이스 및 구현체 (slog.LevelVar 관리)
  stream.go          # StreamRouter 인터페이스 및 구현체 (routingHandler - slog.Handler)
  logger.go          # ComponentLogger, LoggerFactory 인터페이스 및 구현체
  metrics.go         # MetricsCollector 인터페이스 및 구현체 (Prometheus 통합)
  trace.go           # Tracer, Span 인터페이스 및 구현체 (noopSpan)
  observe.go         # Observer 통합 구조체 및 New() 팩토리
  level_test.go      # LevelManager 단위 테스트
  stream_test.go     # StreamRouter 단위 테스트
  logger_test.go     # LoggerFactory 단위 테스트
  metrics_test.go    # MetricsCollector 단위 테스트
  trace_test.go      # Tracer 단위 테스트 + 벤치마크
  observe_test.go    # Observer 통합 테스트
```

## 의존성

- **표준 라이브러리**: `log/slog`, `sync`, `time`, `io`, `strings`, `math/rand/v2`
- **외부 의존성**: `github.com/prometheus/client_golang` v1.18+ (1개)

## 테스트

```bash
# 전체 테스트 실행
go test ./internal/observe/...

# Race Detector 포함 테스트
go test -race ./internal/observe/...

# 커버리지 확인
go test -cover ./internal/observe/...

# 벤치마크 실행
go test -bench=. -benchmem ./internal/observe/...

# 상세 커버리지 리포트
go test -coverprofile=cover.out ./internal/observe/...
go tool cover -html=cover.out
```

### 테스트 결과

- 테스트: 86개 전체 통과
- 커버리지: 95.7%
- Race Detector: 이상 없음
- 벤치마크: 비활성 Tracer `StartSpan()` 0 allocs/op (2.1ns/op)
