---
id: SPEC-WEB-003
version: "1.1.0"
status: completed
created: "2026-03-08"
updated: "2026-03-08"
author: xtra
priority: high
dependencies:
  - SPEC-WEB-002
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-03-08 | 1.0.0 | 초기 SPEC 작성 - Agent Observer 통합을 통한 WebSocket 로그 스트리밍 |
| 2026-03-08 | 1.1.0 | 구현 완료 - M1/M2/M3 전모듈 구현, 14개 파일 변경, 모든 테스트 통과 |

---

# SPEC-WEB-003: Agent Observer 통합을 통한 WebSocket 로그 스트리밍

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 모든 에이전트가 `slog.Default()`를 직접 사용하고 있어 Observer의 StreamRouter 시스템을 우회한다. 이로 인해 에이전트 로그가 Web UI의 실시간 로그 스트리밍(WebSocket)에 표시되지 않는다.

노드는 이미 Observer 기반 로깅을 올바르게 사용하고 있다 (`engine.go`에서 `e.observer.Loggers.NewLogger(component)` 패턴). SPEC-WEB-002에서 StreamRouter의 `SetDefaultWriter(io.MultiWriter(os.Stdout, wsLogWriter))`를 통해 StreamRouter를 경유하는 모든 로그를 WebSocket으로 전달하도록 수정했으나, 에이전트는 StreamRouter를 아예 우회하므로 여전히 WebSocket 로그에 포함되지 않는다.

본 SPEC은 에이전트 시스템에 Observer를 통합하여 에이전트 로그도 노드와 동일하게 컴포넌트별 로거를 사용하도록 개선한다. 이를 통해:

1. 에이전트 로그가 Web UI의 실시간 로그 스트리밍에 표시된다
2. 에이전트별 로그 레벨을 런타임에 개별 변경할 수 있다
3. 컴포넌트 명명 규칙(`agent.{type}.{name}`)으로 로그 필터링이 가능하다

### 1.2 기술 환경

- **백엔드**: Go, xflowd 데몬
- **Agent 시스템**: `internal/agent/` (Agent, BaseAgent, DefaultManager, TypeRegistry)
- **Observer 시스템**: `internal/observe/` (Observer, LoggerFactory, LevelManager, StreamRouter)
- **엔진 참조 패턴**: `internal/engine/engine.go` (노드 로거 주입 패턴)
- **WebSocket**: `internal/api/ws/` (MonitoringBroadcaster, wsLogWriter)
- **의존 SPEC**:
  - SPEC-WEB-002: WebSocket 모니터링 브로드캐스팅 서비스 (StreamRouter -> WebSocket 연결 완료)

### 1.3 현재 문제 분석

**에이전트별 `slog.Default()` 사용 현황**:

| 에이전트 타입 | 파일 | 로거 생성 방식 |
|---|---|---|
| modbus-tcp | `internal/agent/modbus/agent.go` | `slog.Default()` |
| modbus-tcp-server | `internal/agent/modbusserver/agent.go` | `slog.Default()` |
| samsung-nasa | `internal/agent/samsung/agent.go` | `slog.Default()` |
| mqtt-subscriber | `internal/agent/system/mqtt_subscriber.go` | `slog.Default()` |
| console-logger | `internal/agent/system/console_logger.go` | `slog.Default()` |
| influxdb-write | `internal/agent/system/influxdb_agent.go` | `slog.Default()` |

**노드의 Observer 기반 로거 주입 패턴** (참조):
```go
// internal/engine/engine.go:91-108
component := fmt.Sprintf("node.%s", nd.Name)
nodeLogger := e.observer.Loggers.NewLogger(component)
e.observer.Levels.SetLevel(component, lvl)
nodeOpts = append(nodeOpts, node.WithLogger(nodeLogger))
```

### 1.4 설계 원칙

- **하위 호환성**: `config.Logger`가 nil이면 `slog.Default()`로 폴백하여 기존 테스트와 독립 실행에 영향 없음
- **최소 변경**: Observer 패키지 자체는 수정하지 않고 에이전트 레이어만 변경
- **노드 패턴 재사용**: 노드에서 이미 검증된 Observer 기반 로거 주입 패턴을 에이전트에도 동일하게 적용
- **Option 패턴 일관성**: Manager에 Observer를 주입할 때 기존 코드와 일관된 `WithObserver()` 옵션 패턴 사용

### 1.5 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- Module 1: Manager에 Observer 참조 추가 및 AgentConfig에 Logger 필드 추가
- Module 2: 모든 에이전트 팩토리 함수에서 주입된 로거를 사용하도록 마이그레이션
- Module 3: 에이전트별 로그 레벨 관리 (Observer.Levels 연동)

**OUT OF SCOPE (별도 SPEC 또는 미래 구현)**:
- Observer 패키지 자체 수정
- 프론트엔드 로그 뷰어 필터링 UI (에이전트별 필터)
- 에이전트 메트릭 수집 (Observer.Metrics 연동)
- 에이전트 트레이싱 (Observer.Tracer 연동)

---

## 2. Terminology (용어 정의)

| 용어 | 정의 |
|------|------|
| Observer | `observe.Observer` - 로깅, 메트릭, 트레이싱, 스트림 라우팅을 통합 관리하는 관찰성 시스템 |
| LoggerFactory | `observe.LoggerFactory` - 컴포넌트별 `ComponentLogger`를 생성하는 팩토리 |
| ComponentLogger | `observe.ComponentLogger` - 컴포넌트 이름이 자동 포함되는 구조화된 로거 인터페이스 |
| LevelManager | `observe.LevelManager` - 컴포넌트별 로그 레벨을 런타임에 개별 관리하는 매니저 |
| StreamRouter | `observe.StreamRouter` - 컴포넌트별 로그 출력을 라우팅하는 스트림 라우터 |
| AgentConfig.Logger | 본 SPEC에서 새로 추가하는 필드. Manager가 Observer에서 생성한 `*slog.Logger`를 에이전트에 전달 |
| 컴포넌트 명명 규칙 | `agent.{type}.{name}` 형식 (예: `agent.modbus-tcp.modbus-reader`) |

---

## 3. Assumptions (가정 사항)

### 3.1 기술 가정

- A-001: `observe.LoggerFactory.NewLogger(component)`가 동일 컴포넌트 이름에 대해 멱등적으로 같은 인스턴스를 반환한다
- A-002: `observe.LevelManager.SetLevel(component, level)`로 컴포넌트별 로그 레벨을 설정할 수 있다
- A-003: `observe.ComponentLogger.Logger()`가 내부 `*slog.Logger`를 반환하여 기존 에이전트 코드와 호환된다
- A-004: SPEC-WEB-002에서 구현한 `SetDefaultWriter(io.MultiWriter(os.Stdout, wsLogWriter))`가 StreamRouter를 경유하는 모든 로그를 WebSocket으로 전달한다
- A-005: 각 에이전트 팩토리 함수가 `AgentConfig`를 매개변수로 받아 에이전트를 생성한다
- A-006: `DefaultManager.Create()`에서 팩토리 호출 전에 config를 수정할 수 있다
- A-007: 기존 에이전트 테스트는 `AgentConfig.Logger`가 nil인 상태에서 `slog.Default()`를 사용하므로, 폴백 로직이 있으면 테스트 수정이 불필요하다

### 3.2 운영 가정

- A-008: 에이전트 수는 최대 50개 이내이므로 컴포넌트 로거 등록에 의한 메모리 오버헤드는 무시할 수 있다
- A-009: 에이전트 로그 발생량은 노드 로그와 합산하여 SPEC-WEB-002의 rate limiting (초당 100건)으로 충분히 제어 가능하다

---

## 4. Requirements (요구사항)

### 4.1 Module 1: Manager Observer 통합 (P0 - 핵심 기능)

#### REQ-WEB-003-01-01 (Event-Driven)
**WHEN** `DefaultManager`가 `WithObserver(obs)` 옵션으로 생성되면, **THEN** Observer 참조를 내부에 저장하여 에이전트 생성 시 로거 주입에 사용해야 한다.

#### REQ-WEB-003-01-02 (Event-Driven)
**WHEN** `DefaultManager.Create(config)`가 호출되고 Observer가 설정되어 있으면, **THEN** `observer.Loggers.NewLogger("agent.{type}.{name}")` 형식으로 ComponentLogger를 생성하고, `ComponentLogger.Logger()`로 얻은 `*slog.Logger`를 `config.Logger`에 설정한 후 팩토리 함수를 호출해야 한다.

#### REQ-WEB-003-01-03 (Ubiquitous)
`AgentConfig` 구조체에는 **항상** `Logger *slog.Logger` 필드가 존재해야 한다. 이 필드는 선택적이며 nil 허용된다.

#### REQ-WEB-003-01-04 (State-Driven)
**IF** `AgentConfig.Logger`가 nil인 상태 **THEN** 에이전트는 `slog.Default()`를 사용하여 하위 호환성을 유지해야 한다.

#### REQ-WEB-003-01-05 (Event-Driven)
**WHEN** `cmd/xflowd/main.go`에서 `agent.NewManager()`를 호출할 때, **THEN** `agent.WithObserver(obs)` 옵션을 전달하여 Observer를 주입해야 한다.

### 4.2 Module 2: Agent 로거 마이그레이션 (P0 - 핵심 기능)

#### REQ-WEB-003-02-01 (Event-Driven)
**WHEN** 에이전트 팩토리 함수가 호출되면, **THEN** `config.Logger`가 nil이 아닌 경우 해당 로거를 사용하고, nil인 경우 `slog.Default()`를 사용해야 한다.

#### REQ-WEB-003-02-02 (Ubiquitous)
다음 에이전트 타입의 팩토리 함수는 **항상** `config.Logger` 우선 사용 패턴을 적용해야 한다:
- `modbus-tcp` (`internal/agent/modbus/agent.go`)
- `modbus-tcp-server` (`internal/agent/modbusserver/agent.go`)
- `samsung-nasa` (`internal/agent/samsung/agent.go`)
- `mqtt-subscriber` (`internal/agent/system/mqtt_subscriber.go`)
- `console-logger` (`internal/agent/system/console_logger.go`)
- `influxdb-write` (`internal/agent/system/influxdb_agent.go`)

#### REQ-WEB-003-02-03 (Unwanted)
에이전트는 `slog.Default()`를 직접 호출**하지 않아야 한다** (config.Logger 폴백 헬퍼 함수를 통해서만 접근).

#### REQ-WEB-003-02-04 (Ubiquitous)
로거 폴백 헬퍼 함수는 **항상** `internal/agent/` 패키지에 공용으로 제공되어야 한다. 각 에이전트가 동일한 폴백 로직을 중복 구현하지 않도록 한다.

### 4.3 Module 3: Agent 로그 레벨 관리 (P1 - 중요 기능)

#### REQ-WEB-003-03-01 (Event-Driven)
**WHEN** Manager가 Observer를 가진 상태에서 에이전트를 생성하면, **THEN** `observer.Levels.SetLevel("agent.{type}.{name}", level)`을 호출하여 AgentConfig.LogLevel에 지정된 레벨을 Observer에 등록해야 한다.

#### REQ-WEB-003-03-02 (State-Driven)
**IF** `AgentConfig.LogLevel`이 빈 문자열인 상태 **THEN** Observer의 기본 로그 레벨(INFO)을 적용해야 한다.

#### REQ-WEB-003-03-03 (Event-Driven)
**WHEN** REST API를 통해 에이전트의 로그 레벨이 변경되면 (`PUT /api/v1/observe/level?component=agent.modbus-tcp.reader&level=debug`), **THEN** 해당 에이전트의 로그 출력 레벨이 즉시 변경되어야 한다.

#### REQ-WEB-003-03-04 (Ubiquitous)
에이전트 컴포넌트 이름은 **항상** `agent.{type}.{name}` 형식을 따라야 한다.
- 예: `agent.modbus-tcp.modbus-reader`
- 예: `agent.samsung-nasa.hvac-controller`
- 예: `agent.mqtt-subscriber.sensor-data`

---

## 5. Specifications (기술 사양)

### 5.1 Module 1: Manager Observer 통합

**변경 파일 1: `internal/agent/config.go`**
- `AgentConfig` 구조체에 `Logger *slog.Logger` 필드 추가
- `Logger` 필드는 JSON 직렬화에서 제외 (`json:"-"`)
- `Validate()` 메서드에서 Logger 필드는 검증하지 않음 (nil 허용)

**변경 파일 2: `internal/agent/manager.go`**
- `DefaultManager` 구조체에 `observer *observe.Observer` 필드 추가
- `ManagerOption` 함수 옵션 타입 정의
- `WithObserver(obs *observe.Observer) ManagerOption` 옵션 함수 추가
- `NewManager(opts ...ManagerOption) *DefaultManager` 시그니처 변경
- `Create()` 메서드에서 Observer가 있으면 로거 생성 및 config.Logger 설정 로직 추가

**변경 파일 3: `cmd/xflowd/main.go`**
- `agent.NewManager()` 호출을 `agent.NewManager(agent.WithObserver(obs))` 로 변경

**컴포넌트 명명 규칙**:
```
"agent.{config.Type}.{config.Name}"
```
예시: `agent.modbus-tcp.modbus-reader`, `agent.samsung-nasa.hvac-1`

**Create() 메서드 로거 주입 로직**:
```
1. Observer가 nil이면 기존 동작 유지 (config.Logger 미설정)
2. Observer가 있으면:
   a. component := fmt.Sprintf("agent.%s.%s", config.Type, config.Name)
   b. componentLogger := m.observer.Loggers.NewLogger(component)
   c. config.Logger = componentLogger.Logger()
   d. LogLevel이 있으면 m.observer.Levels.SetLevel(component, parsedLevel)
3. 팩토리 함수 호출
```

### 5.2 Module 2: Agent 로거 마이그레이션

**신규 파일: `internal/agent/logger.go`**
- `ResolveLogger(config AgentConfig) *slog.Logger` 헬퍼 함수
  - `config.Logger`가 nil이 아니면 반환
  - nil이면 `slog.Default()` 반환
- 모든 에이전트 팩토리에서 이 함수를 사용

**변경 파일 (6개 에이전트)**:
각 에이전트 파일에서 `slog.Default()` 호출을 `agent.ResolveLogger(config)` 또는 `resolveLogger(config)` 호출로 교체.

| 파일 | 변경 내용 |
|------|----------|
| `internal/agent/modbus/agent.go` | `slog.Default()` -> `agent.ResolveLogger(config)` |
| `internal/agent/modbusserver/agent.go` | `slog.Default()` -> `agent.ResolveLogger(config)` |
| `internal/agent/samsung/agent.go` | `slog.Default()` -> `agent.ResolveLogger(config)` |
| `internal/agent/system/mqtt_subscriber.go` | `slog.Default()` -> `agent.ResolveLogger(config)` |
| `internal/agent/system/console_logger.go` | `slog.Default()` -> `agent.ResolveLogger(config)` |
| `internal/agent/system/influxdb_agent.go` | `slog.Default()` -> `agent.ResolveLogger(config)` |

**마이그레이션 패턴**:
```
변경 전: logger: slog.Default()
변경 후: logger: agent.ResolveLogger(config)
```

### 5.3 Module 3: Agent 로그 레벨 관리

**변경 파일: `internal/agent/manager.go` (Create 메서드 내)**
- `AgentConfig.LogLevel` 문자열을 `slog.Level`로 파싱
- `observer.Levels.SetLevel(component, level)` 호출

**LogLevel 파싱 규칙**:

| 문자열 값 | slog.Level | 비고 |
|-----------|------------|------|
| `""` (빈 문자열) | 미설정 (Observer 기본값 적용) | 기본 동작 |
| `"debug"` | `slog.LevelDebug` | |
| `"info"` | `slog.LevelInfo` | |
| `"warn"` | `slog.LevelWarn` | |
| `"error"` | `slog.LevelError` | |

**기존 API 활용**: 이미 구현된 `PUT /api/v1/observe/level` 엔드포인트를 통해 에이전트 컴포넌트의 로그 레벨을 런타임에 변경할 수 있다. 별도 API 추가 불필요.

### 5.4 Cross-SPEC 의존성

| 본 SPEC 모듈 | 의존 SPEC/시스템 | 의존 내용 |
|-------------|----------------|----------|
| Module 1 | observe.Observer (기존) | `Loggers.NewLogger()`, `Levels.SetLevel()` |
| Module 1 | agent.DefaultManager (기존) | `Create()` 메서드 수정 |
| Module 2 | agent 팩토리 함수 (기존) | `slog.Default()` -> `ResolveLogger()` 교체 |
| Module 3 | observe.LevelManager (기존) | 에이전트별 로그 레벨 등록 |
| 전체 | SPEC-WEB-002 (완료) | StreamRouter -> WebSocket 연결이 이미 구현됨. Observer 로거를 사용하면 자동으로 WebSocket에 로그가 전달됨 |

### 5.5 우선순위 매트릭스

| 우선순위 | 모듈 | 근거 |
|----------|------|------|
| P0 (즉시) | Module 1: Manager Observer 통합 | 로거 주입 인프라. Module 2와 3의 전제 조건 |
| P0 (즉시) | Module 2: Agent 로거 마이그레이션 | 핵심 문제 해결. 에이전트 로그가 WebSocket에 표시되려면 필수 |
| P1 (중요) | Module 3: Agent 로그 레벨 관리 | 운영 편의성. 에이전트별 디버깅 레벨 제어 |

---

*SPEC ID: SPEC-WEB-003*
*버전: 1.1.0*
*상태: completed*
*최종 수정: 2026-03-08*
