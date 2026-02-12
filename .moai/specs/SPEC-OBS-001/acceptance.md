# SPEC-OBS-001: Observe System - 인수 테스트 기준

> TAG: SPEC-OBS-001
> Status: Planned
> Created: 2026-02-12

---

## AC-001: LoggerFactory 생성 및 ComponentLogger 반환

**Given** `internal/observe` 패키지가 임포트된 상태에서

**When** `observe.NewLoggerFactory()` 함수를 호출하고, `factory.NewLogger("agent.mqtt")` 로 로거를 생성하면

**Then** ComponentLogger 인터페이스를 만족하는 객체가 반환되어야 한다

- `Component()`는 `"agent.mqtt"`를 반환한다
- `Logger()`는 유효한 `*slog.Logger`를 반환한다
- `Debug()`, `Info()`, `Warn()`, `Error()` 메서드가 정상 호출된다
- 모든 로그 출력에 `"component"="agent.mqtt"` 구조화 필드가 포함된다

**검증 방법:**
- 커스텀 `slog.Handler`를 주입하여 로그 레코드의 속성을 캡처
- `"component"` 키의 존재 및 값이 `"agent.mqtt"`인지 확인
- `With("key", "value")` 호출 후 추가 필드가 로그에 포함되는지 확인

---

## AC-002: 중복 컴포넌트 로거 동일성 보장

**Given** `factory.NewLogger("engine.scheduler")`로 로거가 이미 생성된 상태에서

**When** 다시 `factory.NewLogger("engine.scheduler")`를 호출하면

**Then** 이전에 생성된 동일한 로거 인스턴스를 반환해야 한다

**검증 방법:**
- 두 호출의 반환값 포인터가 동일한지 확인
- `factory.Components()`에 `"engine.scheduler"`가 중복 없이 1개만 존재하는지 확인
- `factory.GetLogger("engine.scheduler")`가 `(logger, true)`를 반환하는지 확인
- `factory.GetLogger("nonexistent")`가 `(nil, false)`를 반환하는지 확인

---

## AC-003: 런타임 로그 레벨 변경

**Given** `agent.mqtt` 컴포넌트의 기본 로그 레벨이 INFO인 상태에서

**When** `levelManager.SetLevel("agent.mqtt", slog.LevelDebug)`를 호출하면

**Then** 해당 컴포넌트의 DEBUG 로그가 출력되기 시작해야 한다

| 단계 | 동작 | 기대 결과 |
|------|------|----------|
| 1 | `logger.Debug("test")` 호출 (변경 전) | 출력되지 않음 (INFO > DEBUG) |
| 2 | `SetLevel("agent.mqtt", slog.LevelDebug)` | 레벨 변경 |
| 3 | `logger.Debug("test")` 호출 (변경 후) | 출력됨 |
| 4 | `GetLevel("agent.mqtt")` 조회 | `slog.LevelDebug` 반환 |

**추가 검증:**
- 다른 컴포넌트의 레벨은 변경되지 않는다
- 서버 재시작 없이 즉시 반영된다
- `Levels()` 맵에 변경 내용이 반영된다

---

## AC-004: 와일드카드 패턴으로 레벨 일괄 변경

**Given** `agent.mqtt`, `agent.http`, `agent.mqtt.client1`, `engine.scheduler` 컴포넌트가 등록된 상태에서

**When** `levelManager.SetLevelByPattern("agent.*", slog.LevelDebug)`를 호출하면

**Then** `agent.mqtt`, `agent.http`, `agent.mqtt.client1`의 레벨만 DEBUG로 변경되어야 한다

- 반환값은 `3` (변경된 컴포넌트 수)
- `GetLevel("agent.mqtt")`는 `slog.LevelDebug`
- `GetLevel("agent.http")`는 `slog.LevelDebug`
- `GetLevel("agent.mqtt.client1")`는 `slog.LevelDebug`
- `GetLevel("engine.scheduler")`는 기존 레벨 유지 (변경되지 않음)

**추가 검증:**
- `SetLevelByPattern("*", slog.LevelWarn)` 호출 시 모든 컴포넌트 변경
- `SetLevelByPattern("nonexistent.*", slog.LevelDebug)` 호출 시 반환값 `0`
- `SetLevelByPattern("agent.mqtt.*", slog.LevelDebug)` 호출 시 `agent.mqtt.client1`만 변경

---

## AC-005: 미등록 컴포넌트 기본 레벨 적용

**Given** 기본 레벨이 `slog.LevelInfo`로 설정된 상태에서

**When** 등록되지 않은 컴포넌트 `"plugin.custom"`의 레벨을 조회하면

**Then** 기본 레벨 `slog.LevelInfo`를 반환해야 한다

**추가 검증:**
- `SetDefaultLevel(slog.LevelWarn)` 후 미등록 컴포넌트 조회 시 `slog.LevelWarn` 반환
- 이미 등록된 컴포넌트는 `SetDefaultLevel` 영향을 받지 않는다

---

## AC-006: Prometheus 메트릭 수집

**Given** `observe.NewMetricsCollector()`로 MetricsCollector가 생성된 상태에서

**When** 사전 정의 메트릭을 사용하여 카운터를 증가시키면

**Then** Prometheus Registry에서 해당 메트릭을 조회할 수 있어야 한다

| 연산 | 기대 결과 |
|------|----------|
| `collector.Counter("messages_total", "agent.mqtt").Inc()` | `xflow_component_messages_total{component="agent.mqtt"}` 값 1 |
| `collector.Counter("messages_total", "agent.mqtt").Add(5)` | 값 6으로 증가 |
| `collector.Counter("errors_total", "agent.mqtt").Inc()` | `xflow_component_errors_total{component="agent.mqtt"}` 값 1 |
| `collector.Histogram("processing_duration_seconds", "node.filter").Observe(0.05)` | 히스토그램에 0.05 기록 |
| `collector.Gauge("active_connections", "agent.mqtt").Set(10)` | 게이지 값 10 설정 |

**추가 검증:**
- `collector.Registry()`로 Prometheus Registry 접근 가능
- 동일 `(name, component)` 조합으로 여러 번 호출 시 동일 메트릭 반환
- 서로 다른 컴포넌트의 동일 이름 메트릭은 독립적

---

## AC-007: 트레이싱 활성화 및 Span 생성

**Given** `observe.NewTracer()`로 Tracer가 생성되고 `tracer.SetEnabled(true)`로 활성화된 상태에서

**When** `tracer.StartSpan("trace-123", "node.filter", "process")`를 호출하면

**Then** 유효한 Span이 반환되어야 한다

- `span.TraceID()`는 `"trace-123"`을 반환한다
- `span.Component()`는 `"node.filter"`를 반환한다
- `span.Operation()`는 `"process"`를 반환한다
- `span.End()` 호출 후 `span.Duration()`은 양수 값을 반환한다
- `span.SetAttribute("key", "value")` 호출이 정상 동작한다
- `span.AddEvent("checkpoint")` 호출이 정상 동작한다

**검증 방법:**
- `End()` 호출 전후의 `Duration()` 차이 확인
- `time.Sleep(10 * time.Millisecond)` 후 `End()` 호출 시 Duration >= 10ms 확인

---

## AC-008: 트레이싱 비활성 시 제로 오버헤드

**Given** Tracer가 기본 상태(비활성화)로 생성된 상태에서

**When** `tracer.StartSpan("trace-123", "node.filter", "process")`를 호출하면

**Then**

- `tracer.Enabled()`는 `false`를 반환한다
- 반환된 Span은 noop 구현체이다
- `span.End()`, `span.SetAttribute()`, `span.AddEvent()` 호출 시 아무 동작도 하지 않는다
- `span.Duration()`은 `0`을 반환한다

**검증 방법:**
- 벤치마크로 비활성 Tracer의 `StartSpan()`이 0 allocation인지 확인
- `testing.AllocsPerRun`으로 메모리 할당 횟수가 0인지 검증
- 활성화된 Tracer 대비 100배 이상 빠른 처리 속도 확인

---

## AC-009: 샘플링 기반 트레이싱

**Given** Tracer가 활성화되고 `tracer.SetSamplingRate(0.5)`로 샘플링 비율이 50%로 설정된 상태에서

**When** 1000번의 `StartSpan()` 호출을 수행하면

**Then** 약 500개(허용 오차 +/- 10%)의 실제 Span이 생성되고, 나머지는 noop Span이 반환되어야 한다

**검증 방법:**
- `SamplingRate()`가 `0.5`를 반환하는지 확인
- 1000번 호출 후 실제 Span 수가 400~600 범위인지 확인
- `SetSamplingRate(1.0)` 설정 시 모든 호출에서 실제 Span 생성 확인
- `SetSamplingRate(0.0)` 설정 시 모든 호출에서 noop Span 반환 확인

---

## AC-010: StreamRouter 컴포넌트별 로그 라우팅

**Given** StreamRouter가 생성되고 `agent.mqtt` 컴포넌트에 커스텀 `io.Writer`가 등록된 상태에서

**When** `agent.mqtt` 컴포넌트의 로거가 로그를 출력하면

**Then** 등록된 커스텀 Writer에 해당 로그가 기록되어야 한다

| 단계 | 동작 | 기대 결과 |
|------|------|----------|
| 1 | `router.AddRoute("agent.mqtt", &buf1)` | 라우팅 등록 |
| 2 | `agent.mqtt` 로거로 `Info("connected")` | `buf1`에 로그 기록됨 |
| 3 | `engine.scheduler` 로거로 `Info("started")` | `buf1`에 기록되지 않음 (기본 Writer로 출력) |
| 4 | `router.AddRoute("agent.mqtt", &buf2)` | 두 번째 Writer 추가 |
| 5 | `agent.mqtt` 로거로 `Warn("timeout")` | `buf1`과 `buf2` 모두에 기록됨 (fan-out) |

**추가 검증:**
- `Routes("agent.mqtt")`가 등록된 Writer 목록을 반환한다
- `RemoveRoute("agent.mqtt", &buf1)` 후 `buf1`에는 더 이상 기록되지 않는다
- `Handler()`가 유효한 `slog.Handler`를 반환하여 slog 시스템과 통합된다

---

## AC-011: StreamRouter 기본 Writer 폴백

**Given** StreamRouter에 기본 Writer가 설정된 상태에서

**When** 라우팅이 설정되지 않은 `plugin.custom` 컴포넌트의 로거가 로그를 출력하면

**Then** 기본 Writer로 로그가 출력되어야 한다

**검증 방법:**
- 기본 Writer를 `bytes.Buffer`로 설정
- 라우팅 미설정 컴포넌트의 로그 출력 후 버퍼 내용 확인
- `SetDefaultWriter(newBuf)` 호출 후 새 버퍼로 출력되는지 확인

---

## AC-012: StreamRouter Writer 에러 내성

**Given** `agent.mqtt`에 정상 Writer(`buf1`)와 에러를 반환하는 Writer(`errWriter`)가 등록된 상태에서

**When** `agent.mqtt` 로거가 로그를 출력하면

**Then** `errWriter`의 에러와 무관하게 `buf1`에는 정상적으로 로그가 기록되어야 한다

**검증 방법:**
- `errWriter`는 `Write()` 호출 시 항상 에러 반환하는 mock
- `buf1`에 로그가 정상 기록되는지 확인
- 에러 발생 후에도 후속 로그 출력이 정상 동작하는지 확인

---

## AC-013: Observer 통합 생성

**Given** `observe` 패키지가 임포트된 상태에서

**When** `observe.New()` 함수를 호출하면

**Then** 모든 하위 시스템이 초기화된 `Observer` 구조체가 반환되어야 한다

- `observer.Loggers`는 유효한 `LoggerFactory`
- `observer.Levels`는 유효한 `LevelManager`
- `observer.Metrics`는 유효한 `MetricsCollector`
- `observer.Tracer`는 유효한 `Tracer` (기본 비활성화)
- `observer.Streams`는 유효한 `StreamRouter`

**검증 방법:**
- 각 필드의 nil 체크
- `observer.Loggers.NewLogger("test")` 호출 시 정상 로거 반환
- `observer.Levels.SetLevel("test", slog.LevelDebug)` 호출 시 정상 동작
- `observer.Metrics.Counter("test", "comp").Inc()` 호출 시 정상 동작
- `observer.Tracer.Enabled()`가 `false` 반환 (기본 비활성화)

---

## AC-014: LevelManager 동시성 안전

**Given** LevelManager가 생성되고 여러 컴포넌트가 등록된 상태에서

**When** 10개의 goroutine이 동시에 `SetLevel`, `GetLevel`, `SetLevelByPattern`을 호출하면

**Then** 데이터 레이스 없이 모든 연산이 정상 완료되어야 한다

**검증 방법:**
- `go test -race` 통과
- 10개 goroutine이 1000번씩 Get/Set 반복
- 최종 결과의 일관성 확인 (마지막 Set 값이 반영)

---

## AC-015: 계층적 컴포넌트 이름 검증

**Given** LoggerFactory가 생성된 상태에서

**When** 다양한 형식의 컴포넌트 이름으로 `NewLogger()`를 호출하면

**Then** 유효한 dot-notation 이름은 허용하고, 빈 문자열은 거부해야 한다

| 입력 | 기대 결과 |
|------|----------|
| `"engine"` | 성공 - 단일 계층 |
| `"engine.scheduler"` | 성공 - 2단계 계층 |
| `"agent.mqtt.client1"` | 성공 - 3단계 계층 |
| `"node.filter.node-3"` | 성공 - 하이픈 포함 |
| `""` | 실패 - 빈 문자열 거부 (panic 또는 에러) |

---

## 품질 게이트

| 항목 | 기준 | 검증 명령 |
|------|------|----------|
| 테스트 커버리지 | 85% 이상 | `go test -coverprofile=cover.out ./internal/observe/...` |
| 경쟁 조건 | go test -race 통과 | `go test -race ./internal/observe/...` |
| 벤치마크 | `NewLogger()`, `Info()`, `SetLevel()`, `Counter.Inc()`, `StartSpan()` 벤치마크 기록 | `go test -bench=. -benchmem ./internal/observe/...` |
| 린트 | go vet + golangci-lint 통과 | `go vet ./internal/observe/... && golangci-lint run ./internal/observe/...` |
| 제로 오버헤드 | 비활성 Tracer의 StartSpan() 0 allocation | 벤치마크 AllocsPerRun 검증 |
| 동시성 안전 | LevelManager, MetricsCollector race-free | `go test -race` 통과 |

---

## Definition of Done

- [ ] AC-001 ~ AC-015 모든 인수 테스트 통과
- [ ] 테스트 커버리지 85% 이상 달성
- [ ] `go test -race` 경쟁 조건 없음
- [ ] `go vet` 및 `golangci-lint` 경고 없음
- [ ] 벤치마크 결과 기록 완료
- [ ] 비활성 Tracer StartSpan() 0 allocation 확인
- [ ] slog.Handler 합성 패턴 정상 동작 확인
- [ ] Prometheus 메트릭 등록 및 조회 정상 확인
- [ ] 와일드카드 패턴 매칭 정상 동작 확인
- [ ] StreamRouter fan-out 및 Writer 에러 내성 확인
- [ ] Observer 통합 구조체 전체 초기화 확인
