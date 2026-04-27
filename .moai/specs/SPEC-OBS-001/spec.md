---
id: SPEC-OBS-001
version: "1.0.0"
status: implemented
created: "2026-02-12"
updated: "2026-02-14"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-12 | 1.0.0 | 초기 SPEC 작성 |
| 2026-02-14 | 1.0.0 | 구현 완료 (status: implemented) |

---

# SPEC-OBS-001: Observe System - 컴포넌트별 구조화된 관찰성 시스템 (로깅, 메트릭, 트레이싱)

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 엔진의 횡단 관심사(cross-cutting concern)인 Observability 시스템을 정의한다. 모든 시스템 구성 요소(Flow Engine, Agent, Node, Script Engine, Plugin, API Server)가 개별적으로 디버깅 가능하도록 컴포넌트별 로그 분리, 런타임 로그 레벨 관리, Prometheus 메트릭 수집, 메시지 트레이싱 기능을 제공한다. 모든 외부 접근은 인터페이스를 통해 이루어지며, 구현체는 unexported로 캡슐화한다.

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/observe/`
- **의존성**: `log/slog` (표준 라이브러리), `github.com/prometheus/client_golang` v1.18+
- **테스트 프레임워크**: Go 표준 `testing` 패키지
- **외부 의존성**: Prometheus client_golang (1개)
- **Tier**: Tier 2 - 횡단 관심사 (모든 `internal/` 패키지에서 임포트)

### 1.3 설계 원칙

- **인터페이스 우선**: 모든 공개 API는 인터페이스로 정의
- **캡슐화**: 구현체(struct)는 unexported, 팩토리 함수만 exported
- **제로 오버헤드**: Trace/Metrics 비활성화 시 비용 없음
- **계층적 컴포넌트 명명**: dot-notation으로 컴포넌트 계층 표현 (예: `agent.mqtt.client1`)
- **slog.Handler 합성**: 커스텀 Handler를 통한 컴포넌트별 스트림 라우팅
- **Registry 패턴**: 중앙 레지스트리에서 컴포넌트 로거와 레벨을 통합 관리

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: `log/slog`는 Go 1.21+에서 표준으로 제공되며, 구조화된 로깅에 충분한 기능을 제공한다
- A2: `slog.LevelVar`는 atomic 연산으로 동시성 안전한 런타임 레벨 변경을 보장한다
- A3: Prometheus client_golang은 thread-safe한 메트릭 수집을 보장한다
- A4: 컴포넌트 이름은 dot-notation 형식의 문자열이며, 빈 문자열은 허용하지 않는다
- A5: `io.Writer` 인터페이스는 로그 출력 대상 추상화에 충분하다

### 2.2 도메인 가정

- A6: 모든 `internal/` 패키지는 이 observe 패키지를 임포트하여 로깅/메트릭/트레이싱을 수행한다
- A7: 컴포넌트 이름은 계층적 dot-notation을 따른다 (예: `engine.scheduler`, `agent.mqtt.client1`, `node.filter.node-3`)
- A8: REST API 핸들러(internal/api)는 이 SPEC의 범위 밖이며, observe 패키지의 인터페이스만 제공한다
- A9: Web Dashboard 로그 뷰어(web/)는 이 SPEC의 범위 밖이다
- A10: 각 컴포넌트의 observe 통합 코드는 해당 컴포넌트의 SPEC에서 다룬다
- A11: Grafana/Prometheus 서버 설정은 인프라 영역이며 이 SPEC의 범위 밖이다
- A12: 메트릭 레이블의 카디널리티는 사용자가 등록한 컴포넌트 수에 비례하며, 무제한 동적 레이블은 금지한다

---

## 3. Requirements (요구사항)

### Module 1: Logger Factory (Core) - P0

#### REQ-OBS-001-01-01 (Ubiquitous) ComponentLogger 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `ComponentLogger` 인터페이스를 제공해야 한다:

- `Debug(msg string, args ...any)` - DEBUG 레벨 로그 출력
- `Info(msg string, args ...any)` - INFO 레벨 로그 출력
- `Warn(msg string, args ...any)` - WARN 레벨 로그 출력
- `Error(msg string, args ...any)` - ERROR 레벨 로그 출력
- `With(args ...any) ComponentLogger` - 추가 구조화 필드를 포함한 새 로거 반환
- `WithGroup(name string) ComponentLogger` - 로그 그룹을 설정한 새 로거 반환
- `Component() string` - 컴포넌트 이름 반환
- `Logger() *slog.Logger` - 내부 slog.Logger 접근 (호환성 보장)

#### REQ-OBS-001-01-02 (Ubiquitous) ComponentLogger 기본 구현체

시스템은 **항상** `componentLogger`(unexported struct)를 `ComponentLogger` 인터페이스의 기본 구현체로 사용해야 한다. 내부적으로 `*slog.Logger`를 보유하며, 모든 로그 출력에 `"component"` 키로 컴포넌트 이름을 자동 포함한다.

#### REQ-OBS-001-01-03 (Ubiquitous) LoggerFactory 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `LoggerFactory` 인터페이스를 제공해야 한다:

- `NewLogger(component string) ComponentLogger` - 지정 컴포넌트용 로거 생성
- `GetLogger(component string) (ComponentLogger, bool)` - 등록된 로거 조회
- `Components() []string` - 등록된 모든 컴포넌트 이름 목록 반환

#### REQ-OBS-001-01-04 (Ubiquitous) LoggerFactory 팩토리 함수

시스템은 **항상** `NewLoggerFactory(opts ...FactoryOption) LoggerFactory` 팩토리 함수를 제공해야 한다.

- 반환 타입은 `LoggerFactory` 인터페이스이다
- Options Pattern으로 기본 핸들러, 기본 로그 레벨 등을 설정한다

#### REQ-OBS-001-01-05 (Ubiquitous) 계층적 컴포넌트 명명

시스템은 **항상** 컴포넌트 이름을 dot-notation 계층 구조로 관리해야 한다.

- 예시: `engine`, `engine.scheduler`, `agent.mqtt`, `agent.mqtt.client1`, `node.filter.node-3`
- 빈 문자열 컴포넌트 이름은 거부해야 한다

#### REQ-OBS-001-01-06 (Event-Driven) NewLogger 중복 컴포넌트 처리

**WHEN** `NewLogger(component)`가 이미 등록된 컴포넌트 이름으로 호출되면, **THEN** 기존에 생성된 로거를 반환해야 한다 (중복 생성 방지).

---

### Module 2: Level Management - P0

#### REQ-OBS-001-02-01 (Ubiquitous) LevelManager 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `LevelManager` 인터페이스를 제공해야 한다:

- `GetLevel(component string) slog.Level` - 특정 컴포넌트의 현재 로그 레벨 반환
- `SetLevel(component string, level slog.Level)` - 특정 컴포넌트의 로그 레벨 설정
- `SetLevelByPattern(pattern string, level slog.Level) int` - 패턴 매칭으로 복수 컴포넌트 레벨 일괄 설정, 변경된 컴포넌트 수 반환
- `DefaultLevel() slog.Level` - 기본 로그 레벨 반환
- `SetDefaultLevel(level slog.Level)` - 기본 로그 레벨 설정
- `Levels() map[string]slog.Level` - 모든 컴포넌트의 현재 레벨 맵 반환

#### REQ-OBS-001-02-02 (Ubiquitous) slog.LevelVar 기반 런타임 레벨 변경

시스템은 **항상** 각 컴포넌트의 로그 레벨을 `slog.LevelVar`로 관리하여, 서버 재시작 없이 atomic하게 레벨을 변경할 수 있어야 한다.

#### REQ-OBS-001-02-03 (Event-Driven) SetLevel 런타임 동작

**WHEN** `SetLevel(component, level)` 호출 시, **THEN** 해당 컴포넌트의 `slog.LevelVar`가 즉시 업데이트되어 이후 로그 출력에 반영되어야 한다.

#### REQ-OBS-001-02-04 (Event-Driven) 와일드카드 패턴 매칭

**WHEN** `SetLevelByPattern("agent.*", slog.LevelDebug)` 호출 시, **THEN** `agent.mqtt`, `agent.http`, `agent.mqtt.client1` 등 패턴에 매칭되는 모든 컴포넌트의 레벨이 DEBUG로 변경되어야 한다.

#### REQ-OBS-001-02-05 (State-Driven) 미등록 컴포넌트 기본 레벨 적용

**IF** `GetLevel(component)` 호출 시 해당 컴포넌트가 등록되지 않은 상태이면, **THEN** 기본 로그 레벨(`DefaultLevel()`)을 반환해야 한다.

#### REQ-OBS-001-02-06 (Unwanted) 동시성 안전 위반 금지

시스템은 `LevelManager`의 모든 메서드에서 데이터 레이스가 **발생하지 않아야 한다**. 다중 goroutine에서 동시에 Get/Set을 호출해도 안전해야 한다.

---

### Module 3: Metrics Collection - P1

#### REQ-OBS-001-03-01 (Ubiquitous) MetricsCollector 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `MetricsCollector` 인터페이스를 제공해야 한다:

- `Counter(name string, component string) CounterMetric` - Counter 메트릭 생성/조회
- `Histogram(name string, component string) HistogramMetric` - Histogram 메트릭 생성/조회
- `Gauge(name string, component string) GaugeMetric` - Gauge 메트릭 생성/조회
- `Registry() *prometheus.Registry` - Prometheus Registry 접근

#### REQ-OBS-001-03-02 (Ubiquitous) 표준 메트릭 인터페이스

시스템은 **항상** 다음 메트릭 인터페이스를 제공해야 한다:

- `CounterMetric`: `Inc()`, `Add(float64)` 메서드
- `HistogramMetric`: `Observe(float64)` 메서드
- `GaugeMetric`: `Set(float64)`, `Inc()`, `Dec()`, `Add(float64)` 메서드

#### REQ-OBS-001-03-03 (Ubiquitous) 사전 정의 메트릭 셋

시스템은 **항상** 다음 사전 정의 메트릭을 컴포넌트 레이블과 함께 제공해야 한다:

- `xflow_component_messages_total{component="..."}` - 처리된 메시지 총 수 (Counter)
- `xflow_component_errors_total{component="..."}` - 발생한 에러 총 수 (Counter)
- `xflow_component_processing_duration_seconds{component="..."}` - 처리 소요 시간 (Histogram)

#### REQ-OBS-001-03-04 (Ubiquitous) MetricsCollector 팩토리 함수

시스템은 **항상** `NewMetricsCollector(opts ...MetricsOption) MetricsCollector` 팩토리 함수를 제공해야 한다.

- 반환 타입은 `MetricsCollector` 인터페이스이다
- Options Pattern으로 커스텀 Prometheus Registry, namespace, subsystem 등을 설정한다

#### REQ-OBS-001-03-05 (Event-Driven) 메트릭 등록 중복 방지

**WHEN** 동일한 `(name, component)` 조합으로 메트릭을 여러 번 요청하면, **THEN** 기존에 등록된 메트릭을 반환해야 한다 (중복 등록 방지).

#### REQ-OBS-001-03-06 (Unwanted) 카디널리티 폭발 방지

시스템은 동적으로 무제한 레이블 값을 생성하는 패턴을 **허용하지 않아야 한다**. 컴포넌트 레이블은 사전 등록된 컴포넌트만 허용한다.

---

### Module 4: Message Tracing - P1

#### REQ-OBS-001-04-01 (Ubiquitous) Tracer 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Tracer` 인터페이스를 제공해야 한다:

- `StartSpan(traceID string, component string, operation string) Span` - 새 Span 시작
- `Enabled() bool` - 트레이싱 활성화 여부 반환
- `SetEnabled(enabled bool)` - 트레이싱 활성화/비활성화
- `SetSamplingRate(rate float64)` - 샘플링 비율 설정 (0.0 ~ 1.0)
- `SamplingRate() float64` - 현재 샘플링 비율 반환

#### REQ-OBS-001-04-02 (Ubiquitous) Span 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Span` 인터페이스를 제공해야 한다:

- `End()` - Span 종료 (처리 시간 기록)
- `TraceID() string` - Trace ID 반환
- `Component() string` - 컴포넌트 이름 반환
- `Operation() string` - 연산 이름 반환
- `Duration() time.Duration` - 처리 소요 시간 반환 (End 호출 후)
- `SetAttribute(key string, value any)` - Span 속성 추가
- `AddEvent(name string, attrs ...any)` - Span 이벤트 추가

#### REQ-OBS-001-04-03 (Ubiquitous) Tracer 팩토리 함수

시스템은 **항상** `NewTracer(opts ...TracerOption) Tracer` 팩토리 함수를 제공해야 한다.

- 반환 타입은 `Tracer` 인터페이스이다
- 기본 비활성화 상태로 생성한다
- Options Pattern으로 샘플링 비율, 최대 Span 보관 수 등을 설정한다

#### REQ-OBS-001-04-04 (State-Driven) 트레이싱 비활성 상태 제로 오버헤드

**IF** 트레이싱이 비활성화 상태이면, **THEN** `StartSpan()`은 noop Span을 반환하여 메모리 할당과 처리 비용이 발생하지 않아야 한다.

#### REQ-OBS-001-04-05 (Event-Driven) 샘플링 기반 트레이싱

**WHEN** 트레이싱이 활성화되고 샘플링 비율이 1.0 미만이면, **THEN** 설정된 비율에 따라 확률적으로 Span을 생성/무시해야 한다.

#### REQ-OBS-001-04-06 (Unwanted) 트레이싱 비활성 시 비용 발생 금지

시스템은 트레이싱이 비활성화된 상태에서 Span 생성, 속성 기록, 이벤트 추가 등의 연산에 대해 유의미한 비용이 **발생하지 않아야 한다**.

---

### Module 5: Stream Routing - P2

#### REQ-OBS-001-05-01 (Ubiquitous) StreamRouter 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `StreamRouter` 인터페이스를 제공해야 한다:

- `AddRoute(component string, writer io.Writer)` - 컴포넌트에 출력 대상 추가
- `RemoveRoute(component string, writer io.Writer)` - 컴포넌트에서 출력 대상 제거
- `Routes(component string) []io.Writer` - 컴포넌트의 현재 출력 대상 목록 반환
- `SetDefaultWriter(writer io.Writer)` - 기본 출력 대상 설정
- `Handler() slog.Handler` - slog.Handler 반환 (slog 통합용)

#### REQ-OBS-001-05-02 (Ubiquitous) StreamRouter 팩토리 함수

시스템은 **항상** `NewStreamRouter(opts ...StreamOption) StreamRouter` 팩토리 함수를 제공해야 한다.

- 반환 타입은 `StreamRouter` 인터페이스이다
- 기본 출력 대상은 `os.Stdout`이다
- Options Pattern으로 기본 Writer, 포맷(JSON/Text) 등을 설정한다

#### REQ-OBS-001-05-03 (Event-Driven) 로그 라우팅 동작

**WHEN** 특정 컴포넌트의 로그가 발생하면, **THEN** StreamRouter의 slog.Handler가 해당 컴포넌트에 매핑된 모든 `io.Writer`로 로그를 출력해야 한다.

#### REQ-OBS-001-05-04 (State-Driven) 컴포넌트별 라우팅 미설정 시 기본 출력

**IF** 특정 컴포넌트에 대한 라우팅이 설정되지 않은 상태이면, **THEN** 기본 Writer(`SetDefaultWriter`로 설정된 대상)로 출력해야 한다.

#### REQ-OBS-001-05-05 (Event-Driven) 다중 출력 지원

**WHEN** 하나의 컴포넌트에 여러 `io.Writer`가 등록되면, **THEN** 해당 컴포넌트의 로그는 등록된 모든 Writer에 동시에 출력되어야 한다 (fan-out).

#### REQ-OBS-001-05-06 (Unwanted) Writer 에러 전파 금지

시스템은 특정 `io.Writer`에서 에러가 발생해도 다른 Writer로의 출력이 **중단되지 않아야 한다**. 에러가 발생한 Writer의 실패는 내부적으로 기록하되, 나머지 출력은 정상 수행한다.

---

## 4. Specifications (사양)

### 4.1 패키지 구조

```
internal/observe/
  logger.go         # ComponentLogger 인터페이스, componentLogger, LoggerFactory, NewLoggerFactory()
  level.go          # LevelManager 인터페이스, levelManager, slog.LevelVar 관리
  metrics.go        # MetricsCollector 인터페이스, metricsCollector, Prometheus 통합
  trace.go          # Tracer 인터페이스, Span 인터페이스, tracer, noopSpan
  stream.go         # StreamRouter 인터페이스, streamRouter, routingHandler (slog.Handler)
  options.go        # FactoryOption, MetricsOption, TracerOption, StreamOption 정의
  observe.go        # Observer 통합 구조체 (LoggerFactory + LevelManager + MetricsCollector + Tracer + StreamRouter)
  observe_test.go   # 통합 테스트
  logger_test.go    # Logger Factory 테스트
  level_test.go     # Level Manager 테스트
  metrics_test.go   # Metrics Collector 테스트
  trace_test.go     # Tracer 테스트
  stream_test.go    # Stream Router 테스트
```

### 4.2 인터페이스 시그니처 요약

```go
// ComponentLogger - 컴포넌트별 로거 인터페이스
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

// LoggerFactory - 로거 팩토리 인터페이스
type LoggerFactory interface {
    NewLogger(component string) ComponentLogger
    GetLogger(component string) (ComponentLogger, bool)
    Components() []string
}

// LevelManager - 로그 레벨 관리 인터페이스
type LevelManager interface {
    GetLevel(component string) slog.Level
    SetLevel(component string, level slog.Level)
    SetLevelByPattern(pattern string, level slog.Level) int
    DefaultLevel() slog.Level
    SetDefaultLevel(level slog.Level)
    Levels() map[string]slog.Level
}

// MetricsCollector - 메트릭 수집 인터페이스
type MetricsCollector interface {
    Counter(name string, component string) CounterMetric
    Histogram(name string, component string) HistogramMetric
    Gauge(name string, component string) GaugeMetric
    Registry() *prometheus.Registry
}

// CounterMetric - 카운터 메트릭 인터페이스
type CounterMetric interface {
    Inc()
    Add(float64)
}

// HistogramMetric - 히스토그램 메트릭 인터페이스
type HistogramMetric interface {
    Observe(float64)
}

// GaugeMetric - 게이지 메트릭 인터페이스
type GaugeMetric interface {
    Set(float64)
    Inc()
    Dec()
    Add(float64)
}

// Tracer - 트레이싱 인터페이스
type Tracer interface {
    StartSpan(traceID string, component string, operation string) Span
    Enabled() bool
    SetEnabled(enabled bool)
    SetSamplingRate(rate float64)
    SamplingRate() float64
}

// Span - 트레이스 Span 인터페이스
type Span interface {
    End()
    TraceID() string
    Component() string
    Operation() string
    Duration() time.Duration
    SetAttribute(key string, value any)
    AddEvent(name string, attrs ...any)
}

// StreamRouter - 로그 스트림 라우팅 인터페이스
type StreamRouter interface {
    AddRoute(component string, writer io.Writer)
    RemoveRoute(component string, writer io.Writer)
    Routes(component string) []io.Writer
    SetDefaultWriter(writer io.Writer)
    Handler() slog.Handler
}
```

### 4.3 Observer 통합 구조체

```go
// Observer - 관찰성 시스템 통합 진입점
type Observer struct {
    Loggers  LoggerFactory
    Levels   LevelManager
    Metrics  MetricsCollector
    Tracer   Tracer
    Streams  StreamRouter
}

// New(opts ...Option) *Observer - Observer 통합 생성자
```

- `Observer`는 모든 관찰성 하위 시스템을 통합하는 편의 구조체이다
- `New()` 함수로 한 번에 모든 하위 시스템을 초기화한다
- 각 하위 시스템은 개별적으로도 생성/사용 가능하다

### 4.4 사전 정의 메트릭

| 메트릭 이름 | 타입 | 레이블 | 설명 |
|------------|------|--------|------|
| `xflow_component_messages_total` | Counter | `component` | 처리된 메시지 총 수 |
| `xflow_component_errors_total` | Counter | `component` | 발생한 에러 총 수 |
| `xflow_component_processing_duration_seconds` | Histogram | `component` | 처리 소요 시간 |

### 4.5 컴포넌트 명명 규칙

| 구성 요소 | 컴포넌트 이름 예시 | 설명 |
|-----------|-------------------|------|
| Flow Engine | `engine`, `engine.scheduler` | 엔진 코어 및 스케줄러 |
| Agent | `agent.mqtt`, `agent.mqtt.client1` | 에이전트 타입 및 인스턴스 |
| Node | `node.filter.node-3`, `node.transform.node-7` | 노드 타입 및 인스턴스 |
| Script Engine | `script`, `script.vm-pool` | 스크립트 엔진 및 하위 구성 |
| Plugin | `plugin.custom-transform` | 플러그인 이름 |
| API Server | `api`, `api.auth` | API 서버 및 하위 핸들러 |

### 4.6 와일드카드 패턴 매칭 규칙

| 패턴 | 매칭 대상 | 비매칭 대상 |
|------|----------|------------|
| `agent.*` | `agent.mqtt`, `agent.http`, `agent.mqtt.client1` | `engine`, `node.filter` |
| `agent.mqtt.*` | `agent.mqtt.client1`, `agent.mqtt.client2` | `agent.http`, `agent.mqtt` |
| `*` | 모든 등록된 컴포넌트 | - |
| `node.*` | `node.filter.node-3`, `node.transform.node-7` | `agent.mqtt`, `engine` |

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 카테고리 | 검증 방법 |
|-------------|------|----------|-----------|
| REQ-OBS-001-01-01 | Logger Factory | Ubiquitous | 단위 테스트 (인터페이스 구현 검증) |
| REQ-OBS-001-01-02 | Logger Factory | Ubiquitous | 단위 테스트 (구조화 로그 출력 검증) |
| REQ-OBS-001-01-03 | Logger Factory | Ubiquitous | 단위 테스트 (팩토리 메서드 검증) |
| REQ-OBS-001-01-04 | Logger Factory | Ubiquitous | 단위 테스트 (팩토리 함수 + Options) |
| REQ-OBS-001-01-05 | Logger Factory | Ubiquitous | 단위 테스트 (dot-notation 검증, 빈 문자열 거부) |
| REQ-OBS-001-01-06 | Logger Factory | Event-Driven | 단위 테스트 (중복 컴포넌트 동일 로거 반환) |
| REQ-OBS-001-02-01 | Level Management | Ubiquitous | 단위 테스트 (인터페이스 구현 검증) |
| REQ-OBS-001-02-02 | Level Management | Ubiquitous | 단위 테스트 (LevelVar atomic 변경 검증) |
| REQ-OBS-001-02-03 | Level Management | Event-Driven | 단위 테스트 (SetLevel 후 로그 출력 변경) |
| REQ-OBS-001-02-04 | Level Management | Event-Driven | 단위 테스트 (와일드카드 패턴 매칭) |
| REQ-OBS-001-02-05 | Level Management | State-Driven | 단위 테스트 (미등록 컴포넌트 기본 레벨) |
| REQ-OBS-001-02-06 | Level Management | Unwanted | `go test -race` (동시성 안전 검증) |
| REQ-OBS-001-03-01 | Metrics Collection | Ubiquitous | 단위 테스트 (인터페이스 구현 검증) |
| REQ-OBS-001-03-02 | Metrics Collection | Ubiquitous | 단위 테스트 (메트릭 인터페이스 동작 검증) |
| REQ-OBS-001-03-03 | Metrics Collection | Ubiquitous | 단위 테스트 (사전 정의 메트릭 등록 검증) |
| REQ-OBS-001-03-04 | Metrics Collection | Ubiquitous | 단위 테스트 (팩토리 함수 + Options) |
| REQ-OBS-001-03-05 | Metrics Collection | Event-Driven | 단위 테스트 (중복 등록 시 동일 메트릭 반환) |
| REQ-OBS-001-03-06 | Metrics Collection | Unwanted | 설계 검증 (사전 등록 컴포넌트만 허용) |
| REQ-OBS-001-04-01 | Message Tracing | Ubiquitous | 단위 테스트 (Tracer 인터페이스 검증) |
| REQ-OBS-001-04-02 | Message Tracing | Ubiquitous | 단위 테스트 (Span 인터페이스 검증) |
| REQ-OBS-001-04-03 | Message Tracing | Ubiquitous | 단위 테스트 (팩토리 함수 + Options) |
| REQ-OBS-001-04-04 | Message Tracing | State-Driven | 벤치마크 테스트 (비활성 시 제로 할당) |
| REQ-OBS-001-04-05 | Message Tracing | Event-Driven | 통계적 검증 (샘플링 비율 준수) |
| REQ-OBS-001-04-06 | Message Tracing | Unwanted | 벤치마크 테스트 (비활성 시 비용 측정) |
| REQ-OBS-001-05-01 | Stream Routing | Ubiquitous | 단위 테스트 (인터페이스 구현 검증) |
| REQ-OBS-001-05-02 | Stream Routing | Ubiquitous | 단위 테스트 (팩토리 함수 + Options) |
| REQ-OBS-001-05-03 | Stream Routing | Event-Driven | 단위 테스트 (컴포넌트별 라우팅 검증) |
| REQ-OBS-001-05-04 | Stream Routing | State-Driven | 단위 테스트 (미설정 시 기본 Writer) |
| REQ-OBS-001-05-05 | Stream Routing | Event-Driven | 단위 테스트 (다중 Writer fan-out) |
| REQ-OBS-001-05-06 | Stream Routing | Unwanted | 단위 테스트 (Writer 에러 시 다른 Writer 정상 동작) |
