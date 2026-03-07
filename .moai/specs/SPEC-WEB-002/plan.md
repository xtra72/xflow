---
id: SPEC-WEB-002
type: plan
version: "1.1.0"
status: completed
created: "2026-03-08"
updated: "2026-03-08"
author: xtra
---

# SPEC-WEB-002: 구현 계획 - WebSocket 모니터링 브로드캐스팅 서비스

## 1. 마일스톤 개요

| 마일스톤 | 모듈 | 우선순위 | 의존성 | 상태 |
|----------|------|----------|--------|------|
| M1: Hub 주입 및 메트릭 브로드캐스팅 | Module 1 | P0 (즉시) | 없음 | 완료 |
| M2: 로그 스트리밍 서비스 | Module 2 | P1 (중요) | M1 완료 필수 | 완료 |
| M3: 시스템 이벤트 퍼블리셔 | Module 3 | P1 (중요) | M1 완료 필수 | 완료 |

---

## 2. M1: Hub 주입 및 메트릭 브로드캐스팅 (P0)

### 2.1 근본 원인 분석

- **증상**: 프론트엔드 모니터링 페이지(MetricsChart, LogViewer, EventTimeline)가 모두 빈 상태로 표시됨
- **원인**: `cmd/xflowd/main.go:271`에서 `wsHub := ws.NewHub(apiLogger.Logger())`로 허브를 생성하고 `go wsHub.Run()`으로 실행하지만, 어떤 서비스에도 주입하지 않음
- **결과**: `hub.BroadcastMessage()`를 호출하는 코드가 존재하지 않아 WebSocket 클라이언트가 연결되어도 데이터를 수신할 수 없음

### 2.2 수정 대상 파일

| 파일 | 변경 내용 | 실제 크기 |
|------|----------|----------|
| `internal/api/ws/broadcaster.go` (신규) | MonitoringBroadcaster 구조체, MetricsSource 인터페이스, 메트릭 수집/브로드캐스트 고루틴 | 212줄 |
| `internal/api/ws/broadcaster_test.go` (신규) | MonitoringBroadcaster 단위 테스트 (12개 테스트) | 구현 완료 |
| `cmd/xflowd/main.go` | wsHub 생성 위치 이동 + MonitoringBroadcaster 초기화 + Start/Stop | 수정 완료 |

### 2.3 기술 접근

#### 2.3.1 MonitoringBroadcaster 구조체

```
MonitoringBroadcaster
├── hub          *ws.Hub           // WebSocket 허브
├── engine       MetricsSource     // 엔진 인터페이스 (메트릭 소스)
├── observer     *observe.Observer // 관찰성 시스템 (로그/메트릭)
├── interval     time.Duration     // 메트릭 수집 주기 (기본 1초)
├── cancel       context.CancelFunc
├── wg           sync.WaitGroup
├── logger       *slog.Logger
├── prevMsgCount int64             // 이전 틱의 총 메시지 수 (처리량 계산용)
├── prevErrCount int64             // 이전 틱의 총 에러 수 (에러율 계산용)
└── prevTickTime time.Time         // 이전 틱 시각
```

#### 2.3.2 MetricsSource 인터페이스

Engine의 직접 의존성 대신 인터페이스를 사용하여 테스트 용이성을 확보:

```go
type MetricsSource interface {
    ListFlows() []engine.FlowStatus
}
```

`engine.Engine`이 이 인터페이스를 이미 만족하므로 별도 어댑터 불필요.

#### 2.3.3 메트릭 수집 로직

1. `hub.ClientCount()` 확인 → 0이면 건너뜀
2. `runtime.ReadMemStats(&mem)` → 메모리 사용률 계산
3. CPU 사용률: `runtime.NumGoroutine()` 기반 근사치, 또는 프로세스 CPU 시간 증분 계산
4. `engine.ListFlows()` → 전체 FlowStatus 집계
   - `throughput`: (현재 총 MessageCount - 이전 총 MessageCount) / 경과 시간
   - `error_rate`: (현재 총 ErrorCount - 이전 총 ErrorCount) / (현재 총 MessageCount - 이전 총 MessageCount) * 100
5. `hub.BroadcastMessage(TypeFlowMetrics, metricsPayload)` 호출

#### 2.3.4 CPU 사용률 계산 방안

**채택: 방안 A - Go runtime 기반 근사치** (외부 의존성 없음)
- `runtime.NumGoroutine()` 기반 고루틴 수를 CPU 부하 지표로 사용
- 정밀도보다 외부 의존성 없는 단순한 구현을 우선
- 향후 정밀한 CPU 측정이 필요하면 gopsutil 등으로 교체 가능

### 2.4 검증 방법

- `MonitoringBroadcaster` 단위 테스트: mock Hub/Engine으로 메트릭 브로드캐스트 검증
- 통합 테스트: WebSocket 클라이언트 연결 후 `flow.metrics` 메시지 수신 확인
- `wscat -c ws://localhost:8080/ws`로 수동 검증

---

## 3. M2: 로그 스트리밍 서비스 (P1)

### 3.1 수정 대상 파일

| 파일 | 변경 내용 | 실제 크기 |
|------|----------|----------|
| `internal/api/ws/log_writer.go` (신규) | wsLogWriter, rateLimiter (커스텀 토큰 버킷), io.Writer 구현 | 구현 완료 |
| `internal/api/ws/log_writer_test.go` (신규) | wsLogWriter 단위 테스트 (13개 테스트) | 구현 완료 |
| `internal/api/ws/broadcaster.go` | WithStreamRouter 옵션, Start/Stop에 로그 Writer 등록/해제 | 수정 완료 |

### 3.2 기술 접근

#### 3.2.1 wsLogWriter 구조체 (구현 결과)

```
rateLimiter (커스텀 토큰 버킷 - 외부 의존성 없음)
├── maxPerSecond int64          // 초당 최대 토큰 (100)
├── count        atomic.Int64   // 현재 카운트
└── lastReset    atomic.Int64   // 마지막 리셋 시각 (Unix초)

wsLogWriter
├── hub       *ws.Hub           // WebSocket 허브
├── limiter   *rateLimiter      // 커스텀 atomic 기반 rate limiter
├── minLevel  slog.Level        // 최소 로그 레벨 필터 (기본: DEBUG)
├── dropped   atomic.Int64      // 드롭된 로그 수
└── logger    *slog.Logger      // (순환 방지: 모든 내부 에러 silent drop)
```

#### 3.2.2 io.Writer 구현

`Write(p []byte)` 메서드:
1. `hub.ClientCount()` 확인 → 0이면 즉시 반환 (len(p), nil)
2. JSON 파싱: `p`가 JSON 형식 (`{"time":"...","level":"INFO","msg":"..."}`)
3. 레벨 필터: 파싱된 레벨이 `minLevel` 미만이면 건너뜀
4. Rate limit 확인: `limiter.Allow()` → false이면 `dropped` 카운터 증가 후 건너뜀
5. 페이로드 구성: `{ level: "INFO", message: "...", timestamp: "..." }`
6. `hub.BroadcastMessage(TypeLogEntry, payload)` 호출

#### 3.2.3 StreamRouter 연결

`observe.StreamRouter`는 기본적으로 `defaultWriter`(os.Stdout)로 모든 로그를 출력한다.
wsLogWriter를 추가하는 방법:

- `StreamRouter.AddRoute("", wsLogWriter)` - 빈 컴포넌트 이름으로 범용 라우트 등록
- 이 방식이 기존 `defaultWriter`에 영향을 주지 않고 로그를 복제 수신할 수 있는지 코드 확인 필요
- 대안: `StreamRouter`의 `defaultWriter`를 `io.MultiWriter(original, wsLogWriter)`로 교체

#### 3.2.4 순환 로그 방지

- `wsLogWriter` 내부에서 발생하는 로그(에러 등)는 `slog.Default()`를 직접 사용
- `wsLogWriter`가 자신의 로그를 다시 수신하지 않도록 별도 logger 사용
- 또는 `wsLogWriter.Write()` 내부에서 재진입 방지 플래그 사용

### 3.3 검증 방법

- 단위 테스트: mock Hub로 `Write()` 호출 시 `BroadcastMessage` 검증
- Rate limiting 테스트: 초당 200건 Write 시 100건만 브로드캐스트되는지 확인
- 레벨 필터 테스트: DEBUG 로그가 기본 필터(INFO)에서 제외되는지 확인
- 순환 방지 테스트: wsLogWriter의 에러 로그가 무한 루프를 유발하지 않는지 확인

---

## 4. M3: 시스템 이벤트 퍼블리셔 (P1)

### 4.1 수정 대상 파일

| 파일 | 변경 내용 | 실제 크기 |
|------|----------|----------|
| `internal/api/ws/event_publisher.go` (신규) | EventPublisher, 이벤트 타입 상수, eventMessages 맵 | 구현 완료 |
| `internal/api/ws/event_publisher_test.go` (신규) | EventPublisher 단위 테스트 (10개 테스트) | 구현 완료 |
| `internal/api/handler/flow.go` | FlowHandlerOption 패턴, WithEventPublisher, Deploy/Start/Stop 이벤트 발행 | 수정 완료 |
| `cmd/xflowd/main.go` | wsHub 생성 위치 이동, EventPublisher 초기화, FlowHandler에 주입 | 수정 완료 |

### 4.2 기술 접근

#### 4.2.1 EventPublisher 구조체

```
EventPublisher
├── hub    *ws.Hub
└── logger *slog.Logger
```

#### 4.2.2 이벤트 발행 메서드

```go
func (ep *EventPublisher) PublishFlowEvent(eventType, flowName, flowID string)
func (ep *EventPublisher) PublishAgentEvent(eventType, agentName, agentID string)
```

- `hub.ClientCount()` 확인 → 0이면 즉시 반환
- `hub.BroadcastMessage(TypeSystemEvent, payload)` 호출
- 에러 로깅만 수행 (이벤트 전송 실패가 핵심 로직을 블로킹하면 안 됨)

#### 4.2.3 플로우 핸들러 통합

`internal/api/handler/flow.go`의 `FlowHandler`에 `EventPublisher`를 주입:

- `DeployFlow` 핸들러: 성공 후 `ep.PublishFlowEvent("flow_deployed", name, id)`
- `StartFlow` 핸들러: 성공 후 `ep.PublishFlowEvent("flow_started", name, id)`
- `StopFlow` 핸들러: 성공 후 `ep.PublishFlowEvent("flow_stopped", name, id)`

에이전트 이벤트는 `agent_handler.go`에서 유사하게 통합.

#### 4.2.4 에러 이벤트

플로우 에러 이벤트의 경우:
- 현재 Engine 내부에서 에러를 카운터로만 추적하므로, 즉각적인 에러 이벤트 발행이 어려움
- 방안 1: FlowHandler의 에러 응답 시 이벤트 발행 (API 호출 실패만 캡처)
- 방안 2: Engine에 에러 콜백 인터페이스 추가 (향후 개선)
- 본 SPEC에서는 방안 1을 적용하고, 방안 2는 미래 SPEC으로 연기

### 4.3 검증 방법

- 단위 테스트: mock Hub로 `PublishFlowEvent` 호출 시 올바른 메시지 형식 검증
- 통합 테스트: 플로우 배포/시작/중지 후 WebSocket에서 `system.event` 수신 확인
- 클라이언트 0명 테스트: 클라이언트 없을 때 이벤트 발행이 에러 없이 건너뛰어지는지 확인

---

## 5. 의존성 그래프

```
M1 (메트릭 브로드캐스팅) ──── 독립 (즉시 실행 가능)
  │
  ├── M2 (로그 스트리밍) ──── M1의 MonitoringBroadcaster 구조체 확장
  │
  └── M3 (시스템 이벤트) ──── M1 완료 후 실행 (main.go 변경 공유)
```

### 실행 순서 (실제 수행)

1. **M1 단독 진행** (완료): `broadcaster.go` 신규 작성, `main.go` 수정, 빌드/테스트 통과
2. **M2 + M3 병렬 진행** (완료): M1 완료 후 두 에이전트가 병렬 실행
   - M2: `log_writer.go` 신규 + `broadcaster.go` 확장 (WithStreamRouter)
   - M3: `event_publisher.go` 신규 + `flow.go` 수정 (FlowHandlerOption) + `main.go` 구조 변경
3. **테스트 병렬 작성** (완료): M1+M2 테스트와 M3 테스트를 병렬로 작성, 35개 전체 통과 (`-race` 플래그 포함)

---

## 6. 리스크 분석

| 리스크 | 심각도 | 발생 확률 | 완화 방안 |
|--------|--------|----------|----------|
| CPU 사용률 정확도 낮음 (runtime 기반 근사치) | 낮 | 높 | Go runtime 기반 근사치를 기본으로 사용. 필요 시 gopsutil 라이브러리 추가 |
| 대량 로그 발생 시 WebSocket 과부하 | 높 | 중 | Rate limiter(초당 100건) 적용. 클라이언트 0명이면 완전 스킵 |
| StreamRouter에 wsLogWriter 등록 시 기존 로그 출력 영향 | 중 | 낮 | AddRoute("") 방식 사전 테스트. 문제 시 io.MultiWriter 대안 사용 |
| wsLogWriter 순환 로그 (자신의 에러 → 자신이 수신 → 재에러) | 높 | 중 | 별도 logger 사용 및 재진입 방지 플래그 적용 |
| Engine 내부 에러의 실시간 이벤트 캡처 불가 | 중 | 높 | 본 SPEC에서는 API 핸들러 레벨 캡처만 구현. Engine 콜백은 미래 SPEC |
| main.go 변경 충돌 (다른 SPEC과 동시 작업 시) | 낮 | 낮 | M1에서 한번에 main.go 변경 완료. M2/M3는 main.go 추가 변경 최소화 |

---

## 7. 변경 파일 목록 (전체) - 구현 완료

| 파일 | 모듈 | 변경 유형 | 상태 |
|------|------|----------|------|
| `internal/api/ws/broadcaster.go` | M1, M2 | 신규 | 완료 (212줄) |
| `internal/api/ws/broadcaster_test.go` | M1, M2 | 신규 | 완료 (12개 테스트) |
| `internal/api/ws/log_writer.go` | M2 | 신규 | 완료 |
| `internal/api/ws/log_writer_test.go` | M2 | 신규 | 완료 (13개 테스트) |
| `internal/api/ws/event_publisher.go` | M3 | 신규 | 완료 |
| `internal/api/ws/event_publisher_test.go` | M3 | 신규 | 완료 (10개 테스트) |
| `cmd/xflowd/main.go` | M1, M2, M3 | 수정 | 완료 |
| `internal/api/handler/flow.go` | M3 | 수정 | 완료 |

---

## 8. 전문가 상담 권장

| 영역 | 에이전트 | 이유 |
|------|---------|------|
| 백엔드 | expert-backend | MonitoringBroadcaster 설계, Go 동시성 패턴(고루틴, context), rate limiting 구현, Engine 인터페이스 활용 |

---

*SPEC ID: SPEC-WEB-002*
*버전: 1.1.0*
*상태: completed*
*최종 수정: 2026-03-08*
