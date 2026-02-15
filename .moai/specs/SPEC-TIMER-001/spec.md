---
id: SPEC-TIMER-001
version: "1.0.0"
status: draft
created: "2026-02-14"
updated: "2026-02-14"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-14 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-TIMER-001: Timer System Agent - 시스템 타이머/스케줄러 에이전트

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 5개 System Agent(Event, Logger, File, Timer, Store) 중 하나인 Timer Agent를 정의한다. Timer Agent는 **주기적 실행, 크론 스케줄, 지연 실행**을 지원하는 시스템 타이머/스케줄러 서비스이며, `time.Ticker`, `time.AfterFunc`, `robfig/cron/v3`를 조합하여 다양한 타이머 패턴을 구현한다.

본 SPEC은 다음을 다룬다:
- `Timer` 인터페이스 정의 (SetInterval, SetCron, SetTimeout, Cancel, List)
- `TimerHandler`, `TimerTrigger`, `TimerInfo`, `TimerType` 타입 정의
- Interval 타이머 구현 (`time.Ticker` 기반, 최소 간격 100ms)
- Cron 타이머 구현 (`robfig/cron/v3` 기반, 5필드 표준 + 6필드 초 단위)
- Timeout 타이머 구현 (`time.AfterFunc` 기반, 단일 실행 후 자동 제거)
- Timer Agent 생명주기 (Lifecycle, Configurable, HealthChecker 인터페이스 구현)
- Bridge Node 연동 (선택적 메시지 기반 타이머 제어)
- 에러 타입 정의

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/agent/system/timer.go` (구현), `internal/agent/system/timer_*_test.go` (테스트)
- **의존성**: 표준 라이브러리 (`sync`, `time`, `context`, `errors`, `fmt`) + `pkg/lifecycle/` + `github.com/robfig/cron/v3`
- **선택적 의존성**: `pkg/message/` (Bridge Node 연동 시)
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **Tier**: internal (비공개 패키지, 외부 임포트 불가)

### 1.3 설계 원칙

- **인터페이스 우선**: `Timer` 인터페이스를 정의하고 Interval/Cron/Timeout 타이머가 동일 인터페이스로 관리
- **동시성 안전**: 모든 타이머 연산은 thread-safe 하게 동작 (`sync.RWMutex` 기반)
- **최소 간격 보장**: Interval 타이머의 최소 간격을 100ms로 제한하여 과도한 트리거 방지
- **Graceful Shutdown**: Stop 시 모든 타이머를 취소하고 실행 중인 핸들러의 완료를 대기
- **생명주기 통합**: `pkg/lifecycle/Lifecycle`, `Configurable`, `HealthChecker` 인터페이스 구현
- **System Agent 패턴**: Transport/Protocol 설정 불필요, 시스템 시작 시 자동 활성화
- **최소 외부 의존성**: 핵심 기능은 표준 라이브러리 + robfig/cron만 사용

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- `Timer` 인터페이스 정의 (SetInterval, SetCron, SetTimeout, Cancel, List)
- `TimerHandler`, `TimerTrigger`, `TimerID`, `TimerInfo`, `TimerType` 타입 정의
- Interval 타이머 구현 (`time.Ticker` 기반, 최소 간격 100ms, TickCount 추적)
- Cron 타이머 구현 (`robfig/cron/v3`, 5필드 표준 + 선택적 6필드 초 단위)
- Timeout 타이머 구현 (`time.AfterFunc`, 단일 실행, 실행 후 자동 제거)
- `TimerAgent` 구조체 (System Agent로서의 생명주기 관리)
- 에러 타입 정의 (ErrIntervalTooShort, ErrInvalidCronExpression, ErrTimerNotFound 등)
- Bridge Node 통한 메시지 기반 타이머 제어 패턴 정의
- 타이머 통계 (ActiveTimers, TotalTriggers, IntervalTimers, CronTimers, TimeoutTimers)

**OUT OF SCOPE (별도 SPEC)**:
- Flow Engine 통합 (internal/engine/ - 별도 SPEC)
- Bridge Node 구현 자체 (internal/node/bridge.go - SPEC-FLOW-001 범위)
- 관찰성 시스템 통합 구현 (SPEC-OBS-001: internal/observe/)
- 설정 파일 로딩 및 Viper 통합 (SPEC-CFG-001: internal/config/)
- REST API 핸들러 (별도 SPEC: internal/api/)
- 다른 System Agent 구현 (Event, Logger, File, Store)
- 분산 타이머/클러스터 스케줄링

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-LIFE-001 | 의존 | Timer Agent가 Lifecycle, Configurable, HealthChecker 인터페이스를 구현 |
| SPEC-MSG-001 | 동료 | Bridge Node 통한 메시지 기반 접근 시 Message 타입 사용 |
| SPEC-STORE-001 | 동료 | 동일한 System Agent 패턴(BaseLifecycle 임베딩, Options 패턴) 공유 |
| SPEC-FLOW-001 | 소비자 | 플로우가 Bridge Node 또는 직접 API로 Timer Agent 참조 |
| SPEC-OBS-001 | 소비자 | Timer 트리거 이벤트가 관찰성 시스템을 통해 로깅/메트릭 추적 |
| SPEC-CFG-001 | 소비자 | Timer 설정(최소 간격, 최대 타이머 수 등)을 설정 시스템으로 관리 |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: `time.Ticker`는 일정 간격 트리거에 적합하며, goroutine 당 하나의 Ticker를 할당한다
- A2: `robfig/cron/v3`는 thread-safe한 cron 스케줄러이며, 내부적으로 goroutine 관리를 수행한다
- A3: `time.AfterFunc`는 단일 실행 타이머에 적합하며, 실행 후 자동으로 GC 대상이 된다
- A4: `TimerHandler`는 사용자 정의 콜백이며, 핸들러 내에서의 패닉은 Timer Agent를 중단시키지 않도록 recover로 보호한다
- A5: Timer Agent는 `pkg/lifecycle/BaseLifecycle`을 임베딩하여 공통 생명주기 로직을 재사용한다
- A6: Bridge Node 연동 시 요청/응답 패턴은 `pkg/message/Message`의 메타데이터에 연산 유형(set_interval/set_cron/set_timeout/cancel/list)을 포함하여 전달한다
- A7: Cron 표현식 파싱은 `cron.Parser`를 통해 수행하며, 기본적으로 5필드 표준 크론을 지원하고 옵션으로 초 단위(6필드)를 지원한다
- A8: 하나의 TimerAgent 인스턴스가 전역으로 공유되며, TimerID로 개별 타이머를 식별한다

### 2.2 도메인 가정

- A9: Timer Agent는 시스템 시작 시 자동 활성화되며, 외부 연결 설정이 불필요하다
- A10: 최소 실행 간격(기본 100ms)은 시스템 자원 과부하를 방지하기 위한 보안 제한이다
- A11: 최대 동시 타이머 수(기본 1000)를 설정하여 리소스 소진을 방지한다
- A12: Timeout 타이머는 실행 후 자동으로 타이머 목록에서 제거되며, Cancel을 별도로 호출할 필요가 없다
- A13: Timer Agent의 Graceful Shutdown 시, 모든 활성 타이머를 취소하고 실행 중인 핸들러의 완료를 대기한 뒤, cron 스케줄러를 정지한다
- A14: Timer Agent의 HealthCheck는 활성 타이머 수, 총 트리거 횟수, 타이머 유형별 통계를 포함한다

---

## 3. Requirements (요구사항)

### Module 1: Timer Interface - 타이머 인터페이스 (P0)

#### REQ-TIMER-001-01-01 (Ubiquitous) Timer 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Timer` 인터페이스를 제공해야 한다:

- `SetInterval(id string, interval time.Duration, handler TimerHandler) (TimerID, error)` - 주기적 실행 타이머 등록
- `SetCron(id string, cronExpr string, handler TimerHandler) (TimerID, error)` - 크론 스케줄 타이머 등록
- `SetTimeout(id string, delay time.Duration, handler TimerHandler) (TimerID, error)` - 지연 실행 타이머 등록
- `Cancel(id TimerID) error` - 타이머 취소
- `List() []TimerInfo` - 모든 활성 타이머 정보 조회

#### REQ-TIMER-001-01-02 (Ubiquitous) TimerHandler 콜백 타입

시스템은 **항상** `TimerHandler func(trigger TimerTrigger)` 타입을 제공해야 한다. 핸들러는 타이머가 발화될 때 `TimerTrigger` 정보와 함께 호출된다.

#### REQ-TIMER-001-01-03 (Ubiquitous) TimerTrigger 구조체

시스템은 **항상** 다음 필드를 포함하는 `TimerTrigger` 구조체를 제공해야 한다:

- `TimerID TimerID` - 발화된 타이머의 ID
- `TriggerAt time.Time` - 실제 발화 시각
- `TickCount int64` - 누적 발화 횟수 (Interval 타이머에서 유효, Timeout은 항상 1)
- `ScheduleID string` - 스케줄 식별자 (Cron 타이머에서 cron entry ID)

#### REQ-TIMER-001-01-04 (Ubiquitous) TimerInfo 구조체

시스템은 **항상** 다음 필드를 포함하는 `TimerInfo` 구조체를 제공해야 한다:

- `ID TimerID` - 타이머 고유 식별자
- `Type TimerType` - 타이머 유형 (interval, cron, timeout)
- `Expression string` - 스케줄 표현식 (Interval: duration 문자열, Cron: cron 표현식, Timeout: delay 문자열)
- `NextFire time.Time` - 다음 발화 예정 시각
- `LastFired time.Time` - 마지막 발화 시각 (미발화 시 zero value)
- `FireCount int64` - 누적 발화 횟수
- `Active bool` - 활성 상태 여부

#### REQ-TIMER-001-01-05 (Ubiquitous) TimerType 상수

시스템은 **항상** 다음 타이머 유형 상수를 제공해야 한다:

- `TimerTypeInterval TimerType = "interval"` - 주기적 실행 타이머
- `TimerTypeCron TimerType = "cron"` - 크론 스케줄 타이머
- `TimerTypeTimeout TimerType = "timeout"` - 지연 실행 타이머

#### REQ-TIMER-001-01-06 (Unwanted) 빈 ID 등록 금지

시스템은 빈 문자열 ID로 타이머를 등록**하지 않아야 한다**. 빈 ID 등록 시도 시 `ErrTimerIDEmpty` 에러를 반환해야 한다.

#### REQ-TIMER-001-01-07 (Unwanted) nil 핸들러 등록 금지

시스템은 nil 핸들러로 타이머를 등록**하지 않아야 한다**. nil 핸들러 등록 시도 시 `ErrNilHandler` 에러를 반환해야 한다.

---

### Module 2: Interval Timer - 인터벌 타이머 (P0)

#### REQ-TIMER-001-02-01 (Event-Driven) SetInterval 등록

**WHEN** `Timer.SetInterval(id, interval, handler)` 호출 시, **THEN** `time.Ticker` 기반의 주기적 실행 타이머를 생성하고, 지정된 간격마다 핸들러를 호출해야 한다.

#### REQ-TIMER-001-02-02 (Unwanted) 최소 간격 미만 등록 금지

시스템은 최소 간격(기본 100ms) 미만의 인터벌 타이머를 등록**하지 않아야 한다**. 최소 간격 미만 등록 시도 시 `ErrIntervalTooShort` 에러를 반환해야 한다.

#### REQ-TIMER-001-02-03 (Ubiquitous) TickCount 추적

시스템은 **항상** Interval 타이머의 누적 발화 횟수(TickCount)를 추적하고, `TimerTrigger.TickCount`에 전달해야 한다.

#### REQ-TIMER-001-02-04 (Ubiquitous) 독립 goroutine 실행

시스템은 **항상** 각 Interval 타이머를 독립 goroutine에서 실행해야 한다. 하나의 타이머 핸들러 지연이 다른 타이머에 영향을 주지 않아야 한다.

#### REQ-TIMER-001-02-05 (Event-Driven) Cancel 시 Ticker 정지

**WHEN** Interval 타이머에 대해 `Cancel(id)` 호출 시, **THEN** 해당 `time.Ticker`를 Stop하고 goroutine을 종료해야 한다.

---

### Module 3: Cron Timer - 크론 타이머 (P0)

#### REQ-TIMER-001-03-01 (Event-Driven) SetCron 등록

**WHEN** `Timer.SetCron(id, cronExpr, handler)` 호출 시, **THEN** `robfig/cron/v3` 기반의 크론 스케줄 타이머를 등록하고, 크론 표현식에 따라 핸들러를 호출해야 한다.

#### REQ-TIMER-001-03-02 (Ubiquitous) 5필드 표준 크론 지원

시스템은 **항상** 5필드 표준 크론 표현식(분, 시, 일, 월, 요일)을 지원해야 한다:

- `"*/5 * * * *"` - 5분마다
- `"0 9 * * 1-5"` - 평일 오전 9시
- `"0 0 1 * *"` - 매월 1일 자정

#### REQ-TIMER-001-03-03 (Optional) 6필드 초 단위 크론 지원

**가능하면** 6필드 크론 표현식(초, 분, 시, 일, 월, 요일)을 지원하여 초 단위 정밀 스케줄링이 가능해야 한다:

- `"*/30 * * * * *"` - 30초마다
- `"0 */5 * * * *"` - 5분마다 (정각)

#### REQ-TIMER-001-03-04 (Unwanted) 유효하지 않은 크론 표현식 등록 금지

시스템은 유효하지 않은 크론 표현식으로 타이머를 등록**하지 않아야 한다**. 유효하지 않은 표현식 등록 시도 시 `ErrInvalidCronExpression` 에러를 반환해야 한다.

#### REQ-TIMER-001-03-05 (Event-Driven) Cancel 시 Cron Entry 제거

**WHEN** Cron 타이머에 대해 `Cancel(id)` 호출 시, **THEN** cron 스케줄러에서 해당 Entry를 제거해야 한다.

#### REQ-TIMER-001-03-06 (Ubiquitous) ScheduleID 추적

시스템은 **항상** Cron 타이머의 스케줄러 내부 Entry ID를 `TimerTrigger.ScheduleID`에 전달해야 한다.

---

### Module 4: Timeout Timer - 타임아웃 타이머 (P0)

#### REQ-TIMER-001-04-01 (Event-Driven) SetTimeout 등록

**WHEN** `Timer.SetTimeout(id, delay, handler)` 호출 시, **THEN** `time.AfterFunc` 기반의 지연 실행 타이머를 생성하고, 지정된 지연 후 핸들러를 한 번 호출해야 한다.

#### REQ-TIMER-001-04-02 (Ubiquitous) 단일 실행

시스템은 **항상** Timeout 타이머를 정확히 1회만 실행해야 한다. `TimerTrigger.TickCount`는 항상 1이다.

#### REQ-TIMER-001-04-03 (Event-Driven) 실행 후 자동 제거

**WHEN** Timeout 타이머가 발화되면, **THEN** 핸들러 실행 완료 후 타이머 목록에서 자동으로 제거해야 한다.

#### REQ-TIMER-001-04-04 (Event-Driven) Cancel 시 실행 방지

**WHEN** 아직 발화되지 않은 Timeout 타이머에 대해 `Cancel(id)` 호출 시, **THEN** 타이머를 취소하여 핸들러가 실행되지 않도록 하고, 타이머 목록에서 제거해야 한다.

#### REQ-TIMER-001-04-05 (Unwanted) 음수 또는 0 지연 금지

시스템은 음수 또는 0인 지연 값으로 Timeout 타이머를 등록**하지 않아야 한다**. 유효하지 않은 지연 값 등록 시도 시 `ErrInvalidDelay` 에러를 반환해야 한다.

---

### Module 5: Timer Agent Lifecycle - 타이머 에이전트 생명주기 (P0)

#### REQ-TIMER-001-05-01 (Ubiquitous) TimerAgent 구조체

시스템은 **항상** `TimerAgent` 구조체를 제공해야 한다. `TimerAgent`는 다음 인터페이스를 구현한다:

- `pkg/lifecycle/Lifecycle` - 생명주기 관리 (Init, Start, Pause, Resume, Stop, State)
- `pkg/lifecycle/Configurable` - 런타임 설정 변경 (Configure, GetConfig)
- `pkg/lifecycle/HealthChecker` - 헬스 체크 (HealthCheck)

#### REQ-TIMER-001-05-02 (Ubiquitous) BaseLifecycle 임베딩

시스템은 **항상** `TimerAgent`가 `pkg/lifecycle/BaseLifecycle`을 임베딩하여 공통 상태 전이 로직을 재사용해야 한다.

#### REQ-TIMER-001-05-03 (Event-Driven) Init 시 스케줄러 초기화

**WHEN** `TimerAgent.Init(ctx)` 호출 시, **THEN** cron 스케줄러를 초기화하고, 타이머 맵을 생성하고, Running 상태로 전이해야 한다.

#### REQ-TIMER-001-05-04 (Event-Driven) Stop 시 Graceful Shutdown

**WHEN** `TimerAgent.Stop(ctx)` 호출 시, **THEN** 다음 순서로 정지해야 한다:
1. 모든 Interval 타이머의 Ticker를 Stop하고 goroutine을 종료
2. 모든 Timeout 타이머의 AfterFunc를 Stop
3. Cron 스케줄러를 Stop하고 실행 중인 핸들러 완료를 대기 (cron.Stop().Done())
4. 타이머 맵을 비우고 리소스를 해제

#### REQ-TIMER-001-05-05 (Event-Driven) Pause 시 새 트리거 중단

**WHEN** `TimerAgent.Pause(ctx)` 호출 시, **THEN** 새로운 타이머 등록을 거부하고, 활성 타이머의 트리거를 일시정지해야 한다. 이미 실행 중인 핸들러는 완료를 허용한다.

#### REQ-TIMER-001-05-06 (Event-Driven) Resume 시 트리거 재개

**WHEN** `TimerAgent.Resume(ctx)` 호출 시, **THEN** 타이머 등록을 다시 허용하고, 일시정지된 타이머의 트리거를 재개해야 한다.

#### REQ-TIMER-001-05-07 (Ubiquitous) 자동 활성화

시스템은 **항상** 시스템 시작 시 TimerAgent를 자동으로 생성하고 Init/Start하여 활성화해야 한다. 별도의 사용자 등록이 불필요하다.

#### REQ-TIMER-001-05-08 (State-Driven) Configure를 통한 런타임 설정 변경

**IF** TimerAgent가 Running 또는 Paused 상태일 때, **THEN** `Configure(ctx, cfg)`를 통해 다음 설정을 런타임에 변경할 수 있어야 한다:

- `min_interval` - 최소 인터벌 간격
- `max_timers` - 최대 동시 타이머 수

#### REQ-TIMER-001-05-09 (Ubiquitous) HealthCheck 구현

시스템은 **항상** `TimerAgent.HealthCheck(ctx)` 호출 시 다음을 확인하여 `HealthStatus`를 반환해야 한다:

- `ActiveTimers` - 현재 활성 타이머 수
- `TotalTriggers` - 총 트리거 횟수
- `IntervalTimers` - 활성 Interval 타이머 수
- `CronTimers` - 활성 Cron 타이머 수
- `TimeoutTimers` - 활성 Timeout 타이머 수

#### REQ-TIMER-001-05-10 (Ubiquitous) 핸들러 패닉 보호

시스템은 **항상** 타이머 핸들러 실행 시 `recover()`로 패닉을 포착하여, 하나의 핸들러 패닉이 Timer Agent 전체를 중단시키지 않아야 한다.

#### REQ-TIMER-001-05-11 (State-Driven) 최대 타이머 수 제한

**IF** 활성 타이머 수가 최대 제한(기본 1000)에 도달한 상태일 때, **THEN** 새로운 타이머 등록 시 `ErrMaxTimersReached` 에러를 반환해야 한다.

---

### Module 6: Bridge Node Integration - 브릿지 노드 연동 (P1)

#### REQ-TIMER-001-06-01 (Optional) Bridge Node 메시지 기반 타이머 제어

**가능하면** Timer Agent는 Bridge Node를 통해 메시지 기반으로 접근할 수 있어야 한다. 메시지의 메타데이터에 연산 유형을 포함한다:

- `timer.operation`: `set_interval`, `set_cron`, `set_timeout`, `cancel`, `list`
- `timer.id`: 타이머 ID
- `timer.interval`: 간격 값 (set_interval 시, duration 문자열)
- `timer.cron`: 크론 표현식 (set_cron 시)
- `timer.delay`: 지연 값 (set_timeout 시, duration 문자열)

#### REQ-TIMER-001-06-02 (Event-Driven) List 요청-응답 패턴

**WHEN** Bridge Node를 통해 `list` 연산 메시지를 수신하면, **THEN** 모든 활성 타이머 정보를 조회하여 응답 메시지의 Payload에 JSON 배열로 포함하여 반환해야 한다.

#### REQ-TIMER-001-06-03 (Event-Driven) Cancel 메시지 처리

**WHEN** Bridge Node를 통해 `cancel` 연산 메시지를 수신하면, **THEN** 메시지에서 타이머 ID를 추출하여 해당 타이머를 취소해야 한다.

---

### Module 7: Error Types - 에러 타입 (P0)

#### REQ-TIMER-001-07-01 (Ubiquitous) 표준 에러 변수

시스템은 **항상** 다음 에러 변수를 제공해야 한다:

| 에러 변수 | 용도 |
|-----------|------|
| `ErrIntervalTooShort` | 인터벌 간격이 최소 간격(100ms) 미만인 경우 |
| `ErrInvalidCronExpression` | 유효하지 않은 크론 표현식 등록 시도 시 |
| `ErrTimerNotFound` | 존재하지 않는 타이머에 대한 Cancel 시도 시 |
| `ErrTimerIDEmpty` | 빈 문자열 ID로 타이머 등록 시도 시 |
| `ErrTimerClosed` | 중지된 Timer Agent에 대한 연산 시도 시 |
| `ErrTimerPaused` | 일시정지된 Timer Agent에 대한 등록 연산 시도 시 |
| `ErrNilHandler` | nil 핸들러로 타이머 등록 시도 시 |
| `ErrDuplicateTimerID` | 이미 사용 중인 ID로 타이머 등록 시도 시 |
| `ErrMaxTimersReached` | 최대 동시 타이머 수 초과 시 |
| `ErrInvalidDelay` | 음수 또는 0인 지연 값으로 Timeout 등록 시도 시 |

#### REQ-TIMER-001-07-02 (Ubiquitous) 에러 래핑 지원

시스템은 **항상** 모든 에러 변수가 `errors.Is()` 및 `errors.As()`와 호환되도록 sentinel error 패턴을 사용해야 한다.

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
internal/agent/system/
  timer.go              # TimerAgent 구조체, Timer 인터페이스, 타입 정의
  timer_interval.go     # Interval 타이머 구현 (time.Ticker 기반)
  timer_cron.go         # Cron 타이머 구현 (robfig/cron/v3 기반)
  timer_timeout.go      # Timeout 타이머 구현 (time.AfterFunc 기반)
  timer_bridge.go       # Bridge Node 메시지 핸들러 (선택적)
  timer_errors.go       # 에러 변수 정의
  timer_options.go      # TimerOption 함수 타입 및 옵션 함수

  timer_test.go             # Timer 인터페이스 통합 테스트
  timer_interval_test.go    # Interval 타이머 단위 테스트
  timer_cron_test.go        # Cron 타이머 단위 테스트
  timer_timeout_test.go     # Timeout 타이머 단위 테스트
  timer_bridge_test.go      # Bridge Node 연동 테스트
```

### 4.2 타입 시그니처

```go
// TimerID 는 타이머 고유 식별자이다.
type TimerID string

// TimerType 은 타이머 유형을 나타낸다.
type TimerType string

const (
    TimerTypeInterval TimerType = "interval"
    TimerTypeCron     TimerType = "cron"
    TimerTypeTimeout  TimerType = "timeout"
)

// TimerHandler 는 타이머 발화 시 호출되는 콜백 함수 타입이다.
type TimerHandler func(trigger TimerTrigger)

// TimerTrigger 는 타이머 발화 시 전달되는 정보 구조체이다.
type TimerTrigger struct {
    TimerID    TimerID
    TriggerAt  time.Time
    TickCount  int64
    ScheduleID string
}

// TimerInfo 는 타이머의 현재 상태 정보를 나타낸다.
type TimerInfo struct {
    ID         TimerID
    Type       TimerType
    Expression string
    NextFire   time.Time
    LastFired  time.Time
    FireCount  int64
    Active     bool
}

// Timer 는 타이머/스케줄러 인터페이스이다.
type Timer interface {
    SetInterval(id string, interval time.Duration, handler TimerHandler) (TimerID, error)
    SetCron(id string, cronExpr string, handler TimerHandler) (TimerID, error)
    SetTimeout(id string, delay time.Duration, handler TimerHandler) (TimerID, error)
    Cancel(id TimerID) error
    List() []TimerInfo
}

// TimerAgent - System Agent
type TimerAgent struct {
    *lifecycle.BaseLifecycle          // 임베딩
    config    timerConfig             // 설정
    mu        sync.RWMutex            // 타이머 맵 보호
    timers    map[TimerID]*timerEntry // 활성 타이머 맵
    cronSched *cron.Cron              // cron 스케줄러
    wg        sync.WaitGroup          // 실행 중인 핸들러 대기
    paused    bool                    // Pause 상태 플래그
    closed    bool                    // Stop 상태 플래그
    stats     timerStats              // 통계
}

func NewTimerAgent(opts ...TimerOption) *TimerAgent
func (t *TimerAgent) Init(ctx context.Context) error
func (t *TimerAgent) Start(ctx context.Context) error
func (t *TimerAgent) Pause(ctx context.Context) error
func (t *TimerAgent) Resume(ctx context.Context) error
func (t *TimerAgent) Stop(ctx context.Context) error
func (t *TimerAgent) State() lifecycle.State
func (t *TimerAgent) Configure(ctx context.Context, cfg map[string]any) error
func (t *TimerAgent) GetConfig() map[string]any
func (t *TimerAgent) HealthCheck(ctx context.Context) lifecycle.HealthStatus

// Timer 인터페이스 구현
func (t *TimerAgent) SetInterval(id string, interval time.Duration, handler TimerHandler) (TimerID, error)
func (t *TimerAgent) SetCron(id string, cronExpr string, handler TimerHandler) (TimerID, error)
func (t *TimerAgent) SetTimeout(id string, delay time.Duration, handler TimerHandler) (TimerID, error)
func (t *TimerAgent) Cancel(id TimerID) error
func (t *TimerAgent) List() []TimerInfo

// timerEntry - 내부 타이머 엔트리 (unexported)
type timerEntry struct {
    info      TimerInfo
    handler   TimerHandler
    ticker    *time.Ticker    // Interval 전용
    cronID    cron.EntryID    // Cron 전용
    timer     *time.Timer     // Timeout 전용
    cancel    context.CancelFunc // goroutine 종료용
    tickCount int64           // 누적 발화 횟수 (atomic)
}

// timerStats - 통계 (unexported)
type timerStats struct {
    totalTriggers int64  // 총 트리거 횟수 (atomic)
}

// Options Pattern
type TimerOption func(*timerConfig)

type timerConfig struct {
    minInterval  time.Duration  // 최소 간격 (기본: 100ms)
    maxTimers    int            // 최대 동시 타이머 수 (기본: 1000)
    cronParser   cron.Parser    // cron 파서 옵션
}

func WithMinInterval(d time.Duration) TimerOption
func WithMaxTimers(n int) TimerOption
func WithCronParser(parser cron.Parser) TimerOption

// 에러 변수
var (
    ErrIntervalTooShort      = errors.New("timer: interval below minimum (100ms)")
    ErrInvalidCronExpression = errors.New("timer: invalid cron expression")
    ErrTimerNotFound         = errors.New("timer: timer not found")
    ErrTimerIDEmpty          = errors.New("timer: timer ID cannot be empty")
    ErrTimerClosed           = errors.New("timer: timer agent is closed")
    ErrTimerPaused           = errors.New("timer: timer agent is paused (registration disabled)")
    ErrNilHandler            = errors.New("timer: nil handler not allowed")
    ErrDuplicateTimerID      = errors.New("timer: duplicate timer ID")
    ErrMaxTimersReached      = errors.New("timer: maximum number of timers reached")
    ErrInvalidDelay          = errors.New("timer: invalid delay (must be > 0)")
)
```

### 4.3 SPEC-LIFE-001과의 관계

Timer Agent는 `pkg/lifecycle/BaseLifecycle`을 임베딩하여 공통 상태 머신 로직을 재사용한다:

- `Init()` 호출 시 `BaseLifecycle.TransitionTo(StateInitializing)` 후 cron 스케줄러 및 타이머 맵 초기화, 성공 시 `TransitionTo(StateRunning)`
- `Stop()` 호출 시 `TransitionTo(StateStopping)` 후 모든 타이머 취소, cron 스케줄러 정지, 핸들러 완료 대기, 완료 시 `TransitionTo(StateStopped)`
- `Configure()` 호출 시 `CurrentState()` 확인 후 Running/Paused 상태에서만 허용
- `HealthCheck()` 호출은 상태와 무관하게 항상 가능

### 4.4 SPEC-MSG-001과의 관계

Bridge Node 연동 시 `pkg/message/Message`의 Metadata를 활용하여 Timer 연산을 인코딩한다:

- 요청 메시지: `Metadata["timer.operation"]`, `Metadata["timer.id"]`, `Metadata["timer.interval"]`, `Metadata["timer.cron"]`, `Metadata["timer.delay"]`
- 응답 메시지: `Payload["timers"]` (List 결과 JSON 배열), `Metadata["timer.status"]` ("ok" 또는 "error")
- Correlation ID를 통한 요청-응답 매칭은 Bridge Node의 책임 (SPEC-FLOW-001)

### 4.5 내부 타이머 관리 구조

```
타이머 맵: map[TimerID]*timerEntry

timerEntry 내부:
  - Interval: time.Ticker 인스턴스 + 전용 goroutine (done 채널로 종료)
  - Cron:     cron.EntryID로 스케줄러에 등록 (스케줄러가 goroutine 관리)
  - Timeout:  time.Timer 인스턴스 (AfterFunc 반환값, Stop으로 취소)

핸들러 실행 흐름:
  1. 타이머 발화 (Tick/Cron/AfterFunc)
  2. recover()로 패닉 보호
  3. wg.Add(1) → 핸들러 실행 → wg.Done()
  4. stats.totalTriggers 증가 (atomic)
  5. TimerInfo.FireCount 증가, LastFired 갱신
  6. Timeout의 경우 실행 후 타이머 맵에서 자동 제거
```

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 파일 | 우선순위 |
|------------|------|------|---------|
| REQ-TIMER-001-01-01 ~ 01-07 | Timer Interface | timer.go | P0 |
| REQ-TIMER-001-02-01 ~ 02-05 | Interval Timer | timer_interval.go | P0 |
| REQ-TIMER-001-03-01 ~ 03-06 | Cron Timer | timer_cron.go | P0 |
| REQ-TIMER-001-04-01 ~ 04-05 | Timeout Timer | timer_timeout.go | P0 |
| REQ-TIMER-001-05-01 ~ 05-11 | Timer Agent Lifecycle | timer.go | P0 |
| REQ-TIMER-001-06-01 ~ 06-03 | Bridge Node Integration | timer_bridge.go | P1 |
| REQ-TIMER-001-07-01 ~ 07-02 | Error Types | timer_errors.go | P0 |
