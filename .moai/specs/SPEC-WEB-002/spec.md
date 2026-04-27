---
id: SPEC-WEB-002
version: "1.1.0"
status: implemented
created: "2026-03-08"
updated: "2026-03-08"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-03-08 | 1.0.0 | 초기 SPEC 작성 - WebSocket 모니터링 브로드캐스팅 서비스 (메트릭/로그/이벤트) |
| 2026-03-08 | 1.1.0 | 구현 완료 - 3개 모듈 (M1 메트릭, M2 로그, M3 이벤트) 구현 및 35개 테스트 통과 |

---

# SPEC-WEB-002: WebSocket 모니터링 브로드캐스팅 서비스

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 실시간 모니터링 시스템을 활성화한다. 현재 프론트엔드 모니터링 페이지(`MonitoringPage.tsx`)와 WebSocket 인프라(`ws.Hub`, `ws.Client`, `ws.Message`)가 완전히 구현되어 있지만, 백엔드에서 WebSocket Hub를 통해 실제 데이터를 브로드캐스트하는 서비스가 존재하지 않는다.

본 SPEC은 3개 모듈로 구성된 백엔드 브로드캐스팅 서비스를 구현한다:

1. **Hub 주입 및 메트릭 브로드캐스팅**: `wsHub`를 서비스에 주입하고, 주기적으로 시스템 메트릭을 수집하여 `flow.metrics` 타입으로 브로드캐스트
2. **로그 스트리밍**: `observe.StreamRouter`에 WebSocket Writer를 연결하여 실시간 로그를 `log.entry` 타입으로 브로드캐스트
3. **시스템 이벤트 퍼블리셔**: 플로우 라이프사이클 이벤트(시작, 중지, 배포, 에러)를 `system.event` 타입으로 브로드캐스트

### 1.2 기술 환경

- **백엔드**: Go, xflowd 데몬
- **WebSocket 인프라**: `internal/api/ws/` (Hub, Client, Message)
- **모니터링 핸들러**: `internal/api/handler/monitor.go` (MetricsResponse, MonitorManager)
- **엔진**: `internal/engine/` (Engine, FlowStatus, NodeInstanceInfo)
- **관찰성**: `internal/observe/` (Observer, StreamRouter, LevelManager, MetricsCollector)
- **프론트엔드**: `web/src/pages/monitoring/` (MonitoringPage, MetricsChart, LogViewer, EventTimeline)
- **메시지 타입**: `TypeFlowMetrics="flow.metrics"`, `TypeLogEntry="log.entry"`, `TypeSystemEvent="system.event"`
- **의존 SPEC**:
  - SPEC-WEB-001: 웹 UI 에이전트 통계 및 디버깅 레벨 설정 (완료)

### 1.3 설계 원칙

- **기존 인프라 재활용**: `ws.Hub.BroadcastMessage()`, `observe.StreamRouter`, `engine.Engine` 등 기존 인터페이스를 최대한 활용한다
- **최소 변경 원칙**: 새로운 서비스 파일을 추가하되, 기존 코드(`hub.go`, `client.go`, `message.go`, `monitor.go`)는 수정하지 않는다
- **리소스 효율성**: 연결된 클라이언트가 없으면 브로드캐스트를 건너뛰어 불필요한 직렬화 비용을 방지한다
- **유량 제한**: 로그 스트리밍에 rate limiting을 적용하여 대량 로그 발생 시 WebSocket 과부하를 방지한다
- **Graceful Shutdown**: 모든 백그라운드 고루틴이 컨텍스트 취소 시 정상 종료된다

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- Module 1: `wsHub`를 MonitoringBroadcaster 서비스에 주입하고, 1초 주기 시스템 메트릭 브로드캐스트
- Module 2: `observe.StreamRouter`에 WebSocket Writer를 연결하여 실시간 로그 브로드캐스트
- Module 3: 플로우 라이프사이클 이벤트를 WebSocket으로 브로드캐스트
- `cmd/xflowd/main.go`에서 서비스 초기화 및 `wsHub` 주입

**OUT OF SCOPE (별도 SPEC 또는 미래 구현)**:
- 프론트엔드 모니터링 페이지 수정 (이미 완료된 상태)
- WebSocket Hub/Client/Message 구조 변경
- WebSocket 인증/권한 제어
- 메트릭 히스토리 영속화 (DB 저장)
- 클라이언트별 구독 필터링 (메시지 타입별 선택적 수신)

---

## 2. Terminology (용어 정의)

| 용어 | 정의 |
|------|------|
| Hub | `ws.Hub` - WebSocket 클라이언트를 관리하고 메시지를 브로드캐스트하는 중앙 허브 |
| BroadcastMessage | `Hub.BroadcastMessage(msgType, payload)` - 타입과 페이로드로 메시지를 생성하여 모든 클라이언트에 전송 |
| MonitoringBroadcaster | 본 SPEC에서 새로 구현하는 서비스. 메트릭/로그/이벤트를 수집하여 Hub로 브로드캐스트 |
| flow.metrics | WebSocket 메시지 타입. CPU, 메모리, 처리량, 에러율 데이터 포함 |
| log.entry | WebSocket 메시지 타입. 로그 레벨, 메시지, 타임스탬프 포함 |
| system.event | WebSocket 메시지 타입. 이벤트 유형, 메시지, 타임스탬프, 상세 정보 포함 |
| StreamRouter | `observe.StreamRouter` - 컴포넌트별 로그 라우팅을 관리하는 인터페이스. `AddRoute`로 Writer를 추가할 수 있음 |
| FlowStatus | `engine.FlowStatus` - 배포된 플로우의 런타임 상태 (노드 수, 메시지 수, 에러 수 등) |
| EventBus | 시스템 이벤트를 발행/구독하기 위한 내부 이벤트 채널 |

---

## 3. Assumptions (가정 사항)

### 3.1 기술 가정

- A-001: `ws.Hub.BroadcastMessage(msgType, payload)` 메서드가 페이로드를 JSON으로 직렬화하여 모든 연결된 클라이언트에 전송한다
- A-002: `ws.Hub.ClientCount()` 메서드가 현재 연결된 클라이언트 수를 원자적으로 반환한다
- A-003: `engine.Engine`의 `ListFlows()` 메서드가 배포된 모든 플로우의 `FlowStatus`를 반환한다
- A-004: `observe.StreamRouter.AddRoute(component, writer)`로 WebSocket Writer를 등록하면 해당 컴포넌트의 로그가 Writer로 전달된다
- A-005: `runtime.MemStats`로 Go 프로세스의 메모리 사용량을 조회할 수 있다
- A-006: `cmd/xflowd/main.go`에서 `wsHub`가 생성(271줄)되지만 어떤 서비스에도 주입되지 않는 상태이다
- A-007: 프론트엔드 `MonitoringPage.tsx`가 `flow.metrics`, `log.entry`, `system.event` 타입의 WebSocket 메시지를 이미 처리할 준비가 되어 있다

### 3.2 운영 가정

- A-008: 동시 접속 WebSocket 클라이언트 수는 최대 50개 이내이다
- A-009: 시스템 로그 발생량은 초당 최대 1,000건을 초과하지 않는다
- A-010: 메트릭 브로드캐스트 주기 1초는 모니터링 대시보드의 실시간 차트 갱신에 충분하다

---

## 4. Requirements (요구사항)

### 4.1 Module 1: Hub 주입 및 메트릭 브로드캐스팅 서비스 (P0 - 핵심 기능)

#### REQ-WEB-002-01-01 (Event-Driven)
**WHEN** `MonitoringBroadcaster` 서비스가 시작되면, **THEN** 1초 간격의 ticker 고루틴을 생성하여 시스템 메트릭을 수집하고 `flow.metrics` 타입으로 브로드캐스트해야 한다.

#### REQ-WEB-002-01-02 (State-Driven)
**IF** 연결된 WebSocket 클라이언트가 0명인 상태 **THEN** 메트릭 수집 및 브로드캐스트를 건너뛰어 불필요한 CPU/메모리 사용을 방지해야 한다.

#### REQ-WEB-002-01-03 (Ubiquitous)
시스템은 **항상** 메트릭 페이로드에 다음 필드를 포함해야 한다: `cpu` (CPU 사용률 %), `memory` (메모리 사용률 %), `throughput` (전체 플로우 초당 처리량 msg/s), `error_rate` (에러율 %).

#### REQ-WEB-002-01-04 (Event-Driven)
**WHEN** 컨텍스트가 취소되면, **THEN** 메트릭 브로드캐스팅 고루틴이 정상적으로 종료되어야 한다.

#### REQ-WEB-002-01-05 (Event-Driven)
**WHEN** `cmd/xflowd/main.go`가 서버를 초기화할 때, **THEN** `wsHub`와 `engine.Engine`을 `MonitoringBroadcaster`에 주입하고 백그라운드 고루틴을 시작해야 한다.

### 4.2 Module 2: 로그 스트리밍 서비스 (P1 - 중요 기능)

#### REQ-WEB-002-02-01 (Event-Driven)
**WHEN** `MonitoringBroadcaster`가 시작되면, **THEN** `observe.StreamRouter`에 WebSocket 전용 `io.Writer`를 등록하여 애플리케이션 로그를 캡처해야 한다.

#### REQ-WEB-002-02-02 (Event-Driven)
**WHEN** 애플리케이션 로그가 발생하면, **THEN** 해당 로그를 `log.entry` 메시지 형식(`{ level, message, timestamp }`)으로 변환하여 WebSocket으로 브로드캐스트해야 한다.

#### REQ-WEB-002-02-03 (State-Driven)
**IF** 연결된 WebSocket 클라이언트가 0명인 상태 **THEN** 로그를 WebSocket Writer에 기록하지 않고 버려야 한다.

#### REQ-WEB-002-02-04 (Unwanted)
로그 스트리밍은 초당 100건을 초과하여 WebSocket으로 전송**하지 않아야 한다**. 초과분은 버려야 한다.

#### REQ-WEB-002-02-05 (Event-Driven)
**WHEN** `MonitoringBroadcaster`가 종료되면, **THEN** `StreamRouter`에서 WebSocket Writer를 제거(`RemoveRoute`)하여 리소스 누수를 방지해야 한다.

#### REQ-WEB-002-02-06 (State-Driven)
**IF** 로그 레벨 필터가 설정된 상태 **THEN** 설정된 최소 레벨 이상의 로그만 WebSocket으로 스트리밍해야 한다 (기본값: INFO).

### 4.3 Module 3: 시스템 이벤트 퍼블리셔 (P1 - 중요 기능)

#### REQ-WEB-002-03-01 (Event-Driven)
**WHEN** 플로우가 시작(`StartFlow`)되면, **THEN** `system.event` 메시지를 `{ type: "flow_started", message: "플로우 '{name}' 시작됨", timestamp, details: flowId }` 형식으로 브로드캐스트해야 한다.

#### REQ-WEB-002-03-02 (Event-Driven)
**WHEN** 플로우가 중지(`StopFlow`)되면, **THEN** `system.event` 메시지를 `{ type: "flow_stopped", message: "플로우 '{name}' 중지됨", timestamp, details: flowId }` 형식으로 브로드캐스트해야 한다.

#### REQ-WEB-002-03-03 (Event-Driven)
**WHEN** 플로우가 배포(`DeployFlow`)되면, **THEN** `system.event` 메시지를 `{ type: "flow_deployed", message: "플로우 '{name}' 배포됨", timestamp, details: flowId }` 형식으로 브로드캐스트해야 한다.

#### REQ-WEB-002-03-04 (Event-Driven)
**WHEN** 플로우에서 에러가 발생하면, **THEN** `system.event` 메시지를 `{ type: "flow_error", message: "플로우 '{name}' 에러 발생", timestamp, details: errorMessage }` 형식으로 브로드캐스트해야 한다.

#### REQ-WEB-002-03-05 (Event-Driven)
**WHEN** 에이전트가 연결되면, **THEN** `system.event` 메시지를 `{ type: "agent_connected", message: "에이전트 '{name}' 연결됨", timestamp, details: agentId }` 형식으로 브로드캐스트해야 한다.

#### REQ-WEB-002-03-06 (Event-Driven)
**WHEN** 에이전트가 연결 해제되면, **THEN** `system.event` 메시지를 `{ type: "agent_disconnected", message: "에이전트 '{name}' 연결 해제됨", timestamp, details: agentId }` 형식으로 브로드캐스트해야 한다.

#### REQ-WEB-002-03-07 (State-Driven)
**IF** 연결된 WebSocket 클라이언트가 0명인 상태 **THEN** 이벤트를 브로드캐스트하지 않고 건너뛰어야 한다.

---

## 5. Specifications (기술 사양)

### 5.1 Module 1: Hub 주입 및 메트릭 브로드캐스팅

**근본 원인**: `cmd/xflowd/main.go`에서 `wsHub`가 생성되고 `go wsHub.Run()` 으로 실행되지만, 어떤 서비스에도 주입되지 않아 `hub.BroadcastMessage()`를 호출하는 코드가 존재하지 않음.

**구현 파일**: `internal/api/ws/broadcaster.go` (212줄)
- `MetricsSource` 인터페이스: `ListFlows() []engine.FlowStatus` - 테스트 모킹 가능
- `MonitoringBroadcaster` 구조체: `Hub`, `MetricsSource`, `logger` 의존성 + 델타 계산용 prev 상태
- `BroadcasterOption` 함수 옵션 패턴: `WithBroadcastInterval`, `WithStreamRouter`
- `NewMonitoringBroadcaster(hub, engine, logger, opts...)` 생성자
- `Start(ctx)` 메서드: child context 생성, 초기 카운터 설정, 메트릭 ticker 고루틴 시작
- `Stop()` 메서드: 로그 Writer 해제 + context 취소 + WaitGroup 대기

**메트릭 수집 소스 (구현 결정)**:
- `cpu`: `runtime.NumGoroutine()` 기반 고루틴 수 (방안 A 채택 - 외부 의존성 없음)
- `memory`: `runtime.MemStats.Alloc / runtime.MemStats.Sys * 100` (%)
- `throughput`: 전체 플로우 `FlowStatus.MessageCount` 델타 / 경과 시간 (msg/s)
- `error_rate`: 에러 델타 / 메시지 델타 * 100 (%, deltaMsg=0이면 0)

**메트릭 페이로드 형식**:
```json
{
  "cpu": 12.5,
  "memory": 45.2,
  "throughput": 1250.0,
  "error_rate": 0.3
}
```

**수정 파일**: `cmd/xflowd/main.go`
- `wsHub` 생성 위치를 핸들러 생성 전으로 이동 (M3 EventPublisher 주입을 위해)
- `MonitoringBroadcaster` 초기화: `ws.NewMonitoringBroadcaster(wsHub, eng, logger, ws.WithStreamRouter(obs.Streams))`
- `broadcaster.Start(ctx)` 호출, `defer broadcaster.Stop()` 추가

### 5.2 Module 2: 로그 스트리밍

**구현 파일**: `internal/api/ws/log_writer.go`
- `rateLimiter` 구조체: `atomic.Int64` 기반 커스텀 토큰 버킷 (외부 의존성 없음)
  - `maxPerSecond int64` (기본 100), `count atomic.Int64`, `lastReset atomic.Int64`
  - `Allow() bool`: 초당 리셋, 카운트 체크
- `wsLogWriter` 구조체: `io.Writer` 인터페이스 구현
  - `hub *Hub`, `limiter *rateLimiter`, `minLevel slog.Level`, `dropped atomic.Int64`
  - `Write(p []byte)`: 항상 `len(p), nil` 반환 (slog 파이프라인 보호)
  - `Dropped() int64`: 드롭 카운터 조회
- JSON 로그 파싱: `encoding/json` 으로 `level`, `msg`, `time` 필드 추출
- 순환 로그 방지: 모든 내부 에러를 무시 (silent drop)

**StreamRouter 연결 방식 (구현 결정)**:
- `streams.SetDefaultWriter(io.MultiWriter(os.Stdout, wsLogWriter))` 방식 채택 - defaultWriter를 MultiWriter로 교체
- `broadcaster.Start()`에서 설정, `Stop()`에서 `SetDefaultWriter(os.Stdout)` 복원
- 초기 `AddRoute("")` 방식은 빈 컴포넌트 키가 실제 로그의 컴포넌트 이름과 매칭되지 않아 버그 발생하여 수정
- 종료 시 드롭 카운터가 0 초과이면 로그 출력

**로그 메시지 페이로드 형식**:
```json
{
  "level": "INFO",
  "message": "플로우 modbus-flow 시작됨",
  "timestamp": "2026-03-08T10:30:00Z"
}
```

**Rate Limiting 구현 (구현 결정)**:
- 커스텀 `rateLimiter`: `atomic.Int64` 기반 토큰 버킷 (초당 100 토큰)
- `golang.org/x/time/rate` 대신 자체 구현 선택 (외부 의존성 최소화)
- 토큰이 소진되면 로그를 버림 (dropped 카운터 원자적 증가)
- 드롭 카운터는 `Stop()` 시 로깅

### 5.3 Module 3: 시스템 이벤트 퍼블리셔

**구현 파일**: `internal/api/ws/event_publisher.go`
- `EventPublisher` 구조체: `hub *Hub`, `logger *slog.Logger` 의존성
- `NewEventPublisher(hub, logger)` 생성자
- `PublishFlowEvent(eventType, flowName, flowID)` 메서드
- `PublishAgentEvent(eventType, agentName, agentID)` 메서드
- `eventMessages` 맵: 이벤트 타입별 한국어 메시지 포맷 (e.g., "플로우 '%s' 시작됨")
- 에러는 로깅만 수행 (반환하지 않음 - non-blocking)

**이벤트 주입 방식 (구현 결정: 옵션 B - 핸들러 래퍼 방식)**:
- `FlowHandler`에 `FlowHandlerOption` 함수 옵션 패턴 추가
- `WithEventPublisher(ep *EventPublisher)` 옵션으로 주입
- `Deploy`, `Start`, `Stop` 핸들러에서 성공 후 `h.events.PublishFlowEvent()` 호출
- 플로우 이름 조회: `h.flows.GetFlow(ctx, id)`로 이름을 가져와 이벤트 메시지에 사용
- Engine 코드 수정 없이 최소 변경으로 구현
- `main.go`에서 `wsHub` 생성을 핸들러 생성 전으로 이동하여 EventPublisher 주입 가능하게 구조 변경

**이벤트 페이로드 형식**:
```json
{
  "type": "flow_started",
  "message": "플로우 'modbus-flow' 시작됨",
  "timestamp": "2026-03-08T10:30:00Z",
  "details": "flow-123-abc"
}
```

**이벤트 타입 상수**:
| 이벤트 타입 | 발생 시점 |
|------------|----------|
| `flow_started` | FlowHandler.Start() 성공 후 |
| `flow_stopped` | FlowHandler.Stop() 성공 후 |
| `flow_deployed` | FlowHandler.Deploy() 성공 후 |
| `flow_error` | 미구현 (Engine 콜백 필요 - 향후 SPEC) |
| `agent_connected` | 에이전트 연결 시 (호출 인터페이스 구현 완료) |
| `agent_disconnected` | 에이전트 연결 해제 시 (호출 인터페이스 구현 완료) |

### 5.4 Cross-SPEC 의존성

| 본 SPEC 모듈 | 의존 SPEC/시스템 | 의존 내용 |
|-------------|----------------|----------|
| Module 1 | ws.Hub (기존) | `BroadcastMessage()`, `ClientCount()` |
| Module 1 | engine.Engine (기존) | `ListFlows()` - FlowStatus 집계 |
| Module 1 | handler/monitor.go (기존) | MetricsResponse 패턴 참조 (runtime.MemStats) |
| Module 2 | observe.StreamRouter (기존) | `AddRoute()`, `RemoveRoute()` |
| Module 2 | observe.LevelManager (기존) | 로그 레벨 필터링 |
| Module 3 | engine.Engine (기존) | 플로우 라이프사이클 이벤트 소스 |
| 전체 | SPEC-WEB-001 (완료) | 프론트엔드 모니터링 페이지가 이미 메시지를 수신할 준비 완료 |

### 5.5 우선순위 매트릭스

| 우선순위 | 모듈 | 근거 |
|----------|------|------|
| P0 (즉시) | Module 1: 메트릭 브로드캐스팅 | 모니터링 대시보드의 핵심 기능. 메트릭 차트가 데이터 없이 비어 있는 상태 해소 |
| P1 (중요) | Module 2: 로그 스트리밍 | LogViewer 컴포넌트가 빈 상태. 실시간 로그 확인은 디버깅의 핵심 |
| P1 (중요) | Module 3: 시스템 이벤트 | EventTimeline 컴포넌트가 빈 상태. 플로우 운영 상태 파악에 중요 |

---

*SPEC ID: SPEC-WEB-002*
*버전: 1.1.0*
*상태: implemented*
*최종 수정: 2026-03-08*
