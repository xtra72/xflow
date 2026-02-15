# system - Timer 시스템 에이전트

`internal/agent/system` 패키지의 Timer Agent는 xflow 엔진의 타이머/스케줄러 시스템 에이전트를 제공한다. time.Ticker 기반 주기적 실행, robfig/cron/v3 기반 크론 스케줄, time.AfterFunc 기반 지연 실행, 네이티브 Timer 인터페이스, Bridge Node 메시지 프로토콜을 통합 지원한다.

**SPEC**: SPEC-TIMER-001

## 아키텍처 개요

```
    3종 타이머 통합 아키텍처 (Timer Interface)

    +---------------------------------------------------------+
    |                    TimerAgent                            |
    |  BaseLifecycle 임베딩 (7상태 생명주기)                     |
    |  Lifecycle + Configurable + HealthChecker                |
    +---------------------------------------------------------+
            |                             |
    +-------v--------+           +--------v---------+
    |  Timer Interface |           |  BridgeHandler   |
    |  5개 메서드      |           |  메시지 디스패처   |
    +----------------+           +------------------+
            |
    +-------+-------+--------+
    |       |       |        |
    v       v       v        v
Interval  Cron  Timeout  Cancel/List
타이머    타이머  타이머

+------------------+  +------------------+  +------------------+
| time.Ticker 기반 |  | robfig/cron/v3  |  | time.AfterFunc  |
| 주기적 실행       |  | 크론 스케줄      |  | 단일 지연 실행   |
| TickCount 추적   |  | 5/6필드 크론     |  | 실행 후 자동제거 |
| 독립 goroutine   |  | ScheduleID      |  | Stop으로 취소   |
+------------------+  +------------------+  +------------------+
```

**핵심 구성 요소**:

1. **Timer 인터페이스**: 5개 메서드 (SetInterval/SetCron/SetTimeout/Cancel/List)
2. **TimerAgent**: BaseLifecycle 임베딩, Lifecycle/Configurable/HealthChecker 구현
3. **Interval 타이머**: time.Ticker 기반 주기적 실행 (최소 100ms)
4. **Cron 타이머**: robfig/cron/v3 기반 크론 스케줄 (5필드 표준 + 6필드 초 단위)
5. **Timeout 타이머**: time.AfterFunc 기반 지연 실행 (단일 실행, 자동 제거)
6. **TimerHandler**: 타이머 발화 시 호출되는 콜백 함수
7. **BridgeHandler**: Bridge Node 메시지 기반 Timer 접근 디스패처

## 빠른 시작

### TimerAgent 생성 및 초기화

`NewTimerAgent()` 함수는 Options 패턴으로 설정을 받아 TimerAgent를 생성한다. `Init()`으로 초기화하면 cron 스케줄러와 타이머 맵이 시작된다.

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/xtra/xflow/internal/agent/system"
)

func main() {
    ctx := context.Background()

    // TimerAgent 생성 (Options 패턴)
    agent := system.NewTimerAgent(
        system.WithMinInterval(100 * time.Millisecond), // 최소 간격
        system.WithMaxTimers(1000),                      // 최대 타이머 수
    )

    // 초기화 (Created → Initializing → Running)
    if err := agent.Init(ctx); err != nil {
        panic(err)
    }
    defer agent.Stop(ctx)
}
```

### Interval 타이머 - 주기적 실행

`SetInterval()` 메서드로 주기적 실행 타이머를 등록한다. time.Ticker 기반으로 지정된 간격마다 핸들러를 호출한다.

```go
// 5초마다 실행되는 타이머
timerID, err := agent.SetInterval("health-check", 5*time.Second,
    func(trigger system.TimerTrigger) {
        fmt.Printf("헬스 체크 실행 #%d at %s\n",
            trigger.TickCount,
            trigger.TriggerAt.Format(time.RFC3339))
    })

if err != nil {
    panic(err)
}

// 타이머 취소
err = agent.Cancel(timerID)
```

**주요 특징**:
- time.Ticker 기반 주기적 실행
- 최소 간격 100ms (WithMinInterval로 변경 가능)
- TickCount 누적 추적 (TimerTrigger.TickCount)
- 독립 goroutine 실행 (타이머 간 간섭 없음)
- Cancel 시 Ticker.Stop() 및 goroutine 종료

### Cron 타이머 - 크론 스케줄

`SetCron()` 메서드로 크론 표현식 기반 타이머를 등록한다. robfig/cron/v3 기반으로 스케줄에 따라 핸들러를 호출한다.

```go
// 평일 오전 9시마다 실행 (5필드 표준 크론)
cronID, err := agent.SetCron("daily-report", "0 9 * * 1-5",
    func(trigger system.TimerTrigger) {
        fmt.Printf("일일 보고서 생성 at %s (ScheduleID: %s)\n",
            trigger.TriggerAt.Format(time.RFC3339),
            trigger.ScheduleID)
    })

// 30초마다 실행 (6필드 초 단위 크론, WithCronParser로 활성화)
agent30s, _ := system.NewTimerAgent(
    system.WithCronParser(cron.NewParser(
        cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)),
)
agent30s.Init(ctx)

cronID2, err := agent30s.SetCron("frequent-sync", "*/30 * * * * *",
    func(trigger system.TimerTrigger) {
        fmt.Println("30초마다 동기화")
    })
```

**주요 특징**:
- robfig/cron/v3 기반 크론 스케줄러
- 5필드 표준 크론 (분, 시, 일, 월, 요일)
- 6필드 초 단위 크론 (WithCronParser로 활성화)
- ScheduleID 추적 (TimerTrigger.ScheduleID)
- Cancel 시 cron Entry 제거

**크론 표현식 예시**:
- `"*/5 * * * *"` - 5분마다
- `"0 9 * * 1-5"` - 평일 오전 9시
- `"0 0 1 * *"` - 매월 1일 자정
- `"*/30 * * * * *"` - 30초마다 (6필드)

### Timeout 타이머 - 지연 실행

`SetTimeout()` 메서드로 지연 실행 타이머를 등록한다. time.AfterFunc 기반으로 지정된 시간 후 핸들러를 한 번만 호출한다.

```go
// 10초 후 1회 실행
timeoutID, err := agent.SetTimeout("delayed-task", 10*time.Second,
    func(trigger system.TimerTrigger) {
        fmt.Printf("지연 작업 실행 at %s (TickCount: %d)\n",
            trigger.TriggerAt.Format(time.RFC3339),
            trigger.TickCount) // 항상 1
    })

// 취소 (실행 전에만 가능)
err = agent.Cancel(timeoutID)
```

**주요 특징**:
- time.AfterFunc 기반 단일 실행
- TickCount 항상 1
- 실행 후 타이머 목록에서 자동 제거
- Cancel로 실행 전 취소 가능
- 음수 또는 0 지연 시 ErrInvalidDelay 반환

### 타이머 목록 조회

`List()` 메서드로 모든 활성 타이머 정보를 조회한다.

```go
timers := agent.List()
for _, info := range timers {
    fmt.Printf("ID: %s, Type: %s, Expression: %s\n",
        info.ID, info.Type, info.Expression)
    fmt.Printf("  NextFire: %s, LastFired: %s\n",
        info.NextFire.Format(time.RFC3339),
        info.LastFired.Format(time.RFC3339))
    fmt.Printf("  FireCount: %d, Active: %t\n",
        info.FireCount, info.Active)
}
```

### Bridge Node 메시지 프로토콜

BridgeHandler를 사용하면 Bridge Node를 통해 메시지 기반으로 Timer에 접근할 수 있다.

```go
handler := system.NewTimerBridgeHandler(agent)

// SetInterval 연산 메시지 구성
msg := message.New()
msg.Metadata().Set("timer.operation", "set_interval")
msg.Metadata().Set("timer.id", "periodic-sync")
msg.Metadata().Set("timer.interval", "5s")

resp, err := handler.HandleMessage(ctx, msg)
status, _ := resp.Metadata().Get("timer.status") // "ok" 또는 "error"

// List 연산 메시지
listMsg := message.New()
listMsg.Metadata().Set("timer.operation", "list")

listResp, _ := handler.HandleMessage(ctx, listMsg)
timers, _ := listResp.Payload().Get("timers") // []TimerInfo JSON 배열
```

## API 레퍼런스

### Timer 인터페이스

```go
type Timer interface {
    SetInterval(id string, interval time.Duration, handler TimerHandler) (TimerID, error)
    SetCron(id string, cronExpr string, handler TimerHandler) (TimerID, error)
    SetTimeout(id string, delay time.Duration, handler TimerHandler) (TimerID, error)
    Cancel(id TimerID) error
    List() []TimerInfo
}
```

### TimerHandler 콜백

타이머 발화 시 호출되는 콜백 함수 타입이다.

```go
type TimerHandler func(trigger TimerTrigger)
```

### TimerTrigger 구조체

타이머 발화 시 핸들러에 전달되는 정보 구조체이다.

| 필드 | 타입 | 설명 |
|------|------|------|
| `TimerID` | `TimerID` | 발화된 타이머의 ID |
| `TriggerAt` | `time.Time` | 실제 발화 시각 |
| `TickCount` | `int64` | 누적 발화 횟수 (Interval 타이머에서 유효, Timeout은 항상 1) |
| `ScheduleID` | `string` | 스케줄 식별자 (Cron 타이머에서 cron entry ID) |

### TimerInfo 구조체

타이머의 현재 상태 정보를 나타낸다.

| 필드 | 타입 | 설명 |
|------|------|------|
| `ID` | `TimerID` | 타이머 고유 식별자 |
| `Type` | `TimerType` | 타이머 유형 (interval, cron, timeout) |
| `Expression` | `string` | 스케줄 표현식 (Interval: duration 문자열, Cron: cron 표현식, Timeout: delay 문자열) |
| `NextFire` | `time.Time` | 다음 발화 예정 시각 |
| `LastFired` | `time.Time` | 마지막 발화 시각 (미발화 시 zero value) |
| `FireCount` | `int64` | 누적 발화 횟수 |
| `Active` | `bool` | 활성 상태 여부 |

### TimerType 상수

```go
const (
    TimerTypeInterval TimerType = "interval"
    TimerTypeCron     TimerType = "cron"
    TimerTypeTimeout  TimerType = "timeout"
)
```

### TimerOption 옵션

| 옵션 함수 | 기본값 | 설명 |
|-----------|--------|------|
| `WithMinInterval(duration)` | `100ms` | 최소 인터벌 간격 |
| `WithMaxTimers(n)` | `1000` | 최대 동시 타이머 수 |
| `WithCronParser(parser)` | 5필드 표준 | cron 파서 옵션 (6필드 초 단위 활성화) |

### TimerAgent 메서드

| 메서드 | 설명 |
|--------|------|
| `NewTimerAgent(opts ...TimerOption)` | Options 패턴으로 TimerAgent 생성 |
| `Init(ctx)` | Created -> Initializing -> Running 전이 |
| `Start(ctx)` | Running 상태에서 no-op |
| `Pause(ctx)` | Running -> Paused (새 타이머 등록 거부, 트리거 일시정지) |
| `Resume(ctx)` | Paused -> Running |
| `Stop(ctx)` | Running/Paused -> Stopping -> Stopped (Graceful Shutdown) |
| `State()` | 현재 생명주기 상태 반환 |
| `Configure(ctx, cfg)` | Running/Paused에서 런타임 설정 변경 |
| `GetConfig()` | 현재 설정을 map으로 반환 |
| `HealthCheck(ctx)` | 건강 상태 반환 (활성 타이머 수, 통계) |

### Configure 지원 키

| 키 | 타입 | 설명 |
|----|------|------|
| `min_interval` | `string` (duration) | 최소 인터벌 간격 변경 |
| `max_timers` | `int` | 최대 동시 타이머 수 변경 |

### BridgeHandler 메시지 프로토콜

**요청 메타데이터**:

| 메타데이터 키 | 필수 | 설명 |
|--------------|------|------|
| `timer.operation` | 필수 | 연산 종류 ("set_interval"/"set_cron"/"set_timeout"/"cancel"/"list") |
| `timer.id` | 연산별 | 타이머 ID |
| `timer.interval` | set_interval | 간격 값 (duration 문자열, 예: "5s") |
| `timer.cron` | set_cron | 크론 표현식 |
| `timer.delay` | set_timeout | 지연 값 (duration 문자열) |

**요청 Payload**: 없음 (모든 정보는 메타데이터로 전달)

**응답 메타데이터**:

| 메타데이터 키 | 설명 |
|--------------|------|
| `timer.status` | "ok" 또는 "error" |
| `timer.error` | 에러 메시지 (status가 "error"일 때) |
| `timer.id` | 생성된 타이머 ID (set_* 연산 성공 시) |

**응답 Payload**:

| 연산 | Payload 필드 | 설명 |
|------|-------------|------|
| `list` | `timers` | 모든 활성 타이머 정보 ([]TimerInfo JSON 배열) |

## 타이머 유형별 특징

### Interval 타이머 (time.Ticker)

주기적 실행 타이머로 time.Ticker 기반으로 구현된다.

- **자료구조**: `time.Ticker` (Go 내장 주기적 트리거)
- **실행 모델**: 독립 goroutine (타이머마다 1개)
- **TickCount**: 누적 발화 횟수 추적 (atomic.Int64)
- **최소 간격**: 기본 100ms (WithMinInterval로 변경)
- **취소**: Cancel 시 Ticker.Stop() 및 goroutine 종료
- **핸들러 보호**: recover()로 패닉 포착

### Cron 타이머 (robfig/cron/v3)

크론 표현식 기반 스케줄 타이머로 robfig/cron/v3 기반으로 구현된다.

- **자료구조**: `cron.Cron` 스케줄러 (thread-safe)
- **크론 형식**: 5필드 표준 (분, 시, 일, 월, 요일)
- **6필드 지원**: WithCronParser로 초 단위 크론 활성화
- **ScheduleID**: cron.EntryID 추적
- **파싱**: cron.Parser로 표현식 검증
- **취소**: Cancel 시 cron.Remove(entryID)
- **Graceful Shutdown**: Stop 시 cron.Stop().Done() 대기

### Timeout 타이머 (time.AfterFunc)

지연 실행 타이머로 time.AfterFunc 기반으로 구현된다.

- **자료구조**: `time.Timer` (time.AfterFunc 반환값)
- **실행 횟수**: 정확히 1회만 실행 (TickCount 항상 1)
- **자동 제거**: 실행 후 타이머 맵에서 자동 삭제
- **취소**: Cancel 시 timer.Stop() 호출
- **핸들러 보호**: recover()로 패닉 포착
- **유효성 검증**: 음수 또는 0 지연 시 ErrInvalidDelay

## 생명주기 관리

### Graceful Shutdown 순서

Stop 호출 시 다음 순서로 정지한다:

1. **Interval 타이머**: 모든 Ticker.Stop() 및 goroutine 종료
2. **Timeout 타이머**: 모든 timer.Stop() 호출
3. **Cron 스케줄러**: cron.Stop() 호출 및 Done() 대기 (실행 중인 핸들러 완료)
4. **타이머 맵**: 모든 엔트리 삭제 및 리소스 해제
5. **상태 전이**: StateStopped로 전이

### Pause/Resume 동작

- **Pause**: 새로운 타이머 등록 거부 (ErrTimerPaused), 활성 타이머 트리거 일시정지
- **Resume**: 타이머 등록 재허용, 일시정지된 트리거 재개

### 핸들러 패닉 보호

모든 타이머 핸들러 실행 시 recover()로 패닉을 포착하여, 하나의 핸들러 패닉이 Timer Agent 전체를 중단시키지 않는다.

```go
func (t *TimerAgent) executeHandler(handler TimerHandler, trigger TimerTrigger) {
    defer func() {
        if r := recover(); r != nil {
            // 패닉 로깅 및 복구
            fmt.Printf("Timer handler panicked: %v\n", r)
        }
    }()

    t.wg.Add(1)
    defer t.wg.Done()

    handler(trigger)
}
```

## 센티널 에러

| 에러 변수 | 메시지 | 발생 조건 |
|-----------|--------|----------|
| `ErrIntervalTooShort` | timer: interval below minimum (100ms) | 인터벌 간격이 최소 간격 미만 |
| `ErrInvalidCronExpression` | timer: invalid cron expression | 유효하지 않은 크론 표현식 |
| `ErrTimerNotFound` | timer: timer not found | 존재하지 않는 타이머 Cancel 시도 |
| `ErrTimerIDEmpty` | timer: timer ID cannot be empty | 빈 문자열 ID로 등록 시도 |
| `ErrTimerClosed` | timer: timer agent is closed | 중지된 Timer Agent 연산 시도 |
| `ErrTimerPaused` | timer: timer agent is paused (registration disabled) | 일시정지 상태에서 등록 시도 |
| `ErrNilHandler` | timer: nil handler not allowed | nil 핸들러로 등록 시도 |
| `ErrDuplicateTimerID` | timer: duplicate timer ID | 이미 사용 중인 ID로 등록 시도 |
| `ErrMaxTimersReached` | timer: maximum number of timers reached | 최대 동시 타이머 수 초과 |
| `ErrInvalidDelay` | timer: invalid delay (must be > 0) | 음수 또는 0 지연 값 |

## 설계 특징

- **인터페이스 우선**: 모든 공개 API는 `Timer` 인터페이스로 정의되며, TimerAgent가 구현
- **3종 타이머 통합**: Interval, Cron, Timeout을 하나의 인터페이스로 관리
- **동시성 안전**: sync.RWMutex(타이머 맵 보호), atomic.Int64(통계), sync.WaitGroup(핸들러 대기)
- **BaseLifecycle 임베딩**: 7상태 생명주기 관리를 상속하여 Init/Start/Pause/Resume/Stop 구현
- **상태 기반 접근 제어**: Paused 시 등록 거부, Stopped 시 모든 연산 차단
- **Graceful Shutdown**: Stop 시 모든 타이머 취소 및 핸들러 완료 대기
- **핸들러 패닉 보호**: recover()로 패닉 포착하여 Agent 안정성 보장
- **Options 패턴**: NewTimerAgent()에 함수 옵션 패턴 적용
- **Configure 확장**: Running/Paused 상태에서 최소 간격, 최대 타이머 수 런타임 변경

## 파일 구조

```
internal/agent/system/
  timer_errors.go          # 센티널 에러 정의 (10개)
  timer.go                 # TimerAgent 구조체, Timer 인터페이스, 타입 정의
  timer_options.go         # TimerOption 타입, timerConfig, 3개 옵션 함수
  timer_interval.go        # Interval 타이머 구현 (time.Ticker 기반)
  timer_cron.go            # Cron 타이머 구현 (robfig/cron/v3 기반)
  timer_timeout.go         # Timeout 타이머 구현 (time.AfterFunc 기반)
  timer_bridge.go          # TimerBridgeHandler (메시지 기반 Timer 접근 디스패처)
  timer_errors_test.go     # 센티널 에러 테스트 (10개)
  timer_test.go            # TimerAgent 통합 테스트 (30개)
  timer_interval_test.go   # Interval 타이머 단위 테스트 (28개)
  timer_cron_test.go       # Cron 타이머 단위 테스트 (32개)
  timer_timeout_test.go    # Timeout 타이머 단위 테스트 (24개)
  timer_bridge_test.go     # BridgeHandler 테스트 (72개)
```

## 의존성

- **표준 라이브러리**: `sync`, `sync/atomic`, `time`, `context`, `fmt`, `errors`
- **외부 의존성**:
  - `github.com/robfig/cron/v3` - 크론 스케줄러
- **내부 의존성**:
  - `pkg/lifecycle` (SPEC-LIFE-001) - 7상태 생명주기 관리 (BaseLifecycle 임베딩)
  - `pkg/message` (SPEC-MSG-001) - Bridge Node 메시지 처리 (BridgeHandler)

## 테스트

```bash
# 전체 테스트 실행
go test ./internal/agent/system/...

# Race Detector 포함 테스트
go test -race ./internal/agent/system/...

# 커버리지 확인
go test -cover ./internal/agent/system/...

# 상세 커버리지 리포트
go test -coverprofile=cover.out ./internal/agent/system/...
go tool cover -html=cover.out
```

### 테스트 결과

- 테스트: 186개 전체 통과
- 커버리지: 86.5%
- Race Detector: 이상 없음 (go test -race)
- Go Vet: 이상 없음 (go vet)

## HealthCheck 정보

HealthCheck 호출 시 다음 통계를 반환한다:

```go
type HealthStatus struct {
    State          string         // Running, Paused, Stopped
    ActiveTimers   int            // 현재 활성 타이머 수
    TotalTriggers  int64          // 총 트리거 횟수
    IntervalTimers int            // 활성 Interval 타이머 수
    CronTimers     int            // 활성 Cron 타이머 수
    TimeoutTimers  int            // 활성 Timeout 타이머 수
}
```

## 사용 예시

### 복합 타이머 시나리오

```go
ctx := context.Background()
agent := system.NewTimerAgent()
agent.Init(ctx)
defer agent.Stop(ctx)

// 1. 헬스 체크 (5초마다)
healthID, _ := agent.SetInterval("health-check", 5*time.Second,
    func(trigger system.TimerTrigger) {
        fmt.Printf("헬스 체크 #%d\n", trigger.TickCount)
    })

// 2. 일일 보고서 (평일 오전 9시)
reportID, _ := agent.SetCron("daily-report", "0 9 * * 1-5",
    func(trigger system.TimerTrigger) {
        fmt.Println("일일 보고서 생성")
    })

// 3. 초기화 지연 (10초 후 1회)
initID, _ := agent.SetTimeout("delayed-init", 10*time.Second,
    func(trigger system.TimerTrigger) {
        fmt.Println("지연 초기화 완료")
    })

// 타이머 목록 확인
timers := agent.List()
fmt.Printf("활성 타이머: %d개\n", len(timers))

// 헬스 체크
health := agent.HealthCheck(ctx)
fmt.Printf("상태: %s, 총 트리거: %d\n", health.State, health.TotalTriggers)
```

## 모범 사례

1. **최소 간격 설정**: CPU 과부하 방지를 위해 최소 100ms 간격 권장
2. **최대 타이머 수 제한**: 메모리 소진 방지를 위해 최대 1000개 제한 권장
3. **Graceful Shutdown**: 애플리케이션 종료 시 반드시 Stop() 호출
4. **핸들러 최적화**: 타이머 핸들러는 가볍게 작성하고, 무거운 작업은 별도 goroutine에서 처리
5. **에러 핸들링**: 핸들러 내에서 패닉 발생 시 복구되지만, 에러 로깅 권장
6. **Timeout 자동 제거**: Timeout 타이머는 실행 후 자동 제거되므로 Cancel 불필요
7. **크론 검증**: 크론 표현식 등록 전 파서로 사전 검증 권장

## 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-LIFE-001 | 의존 | Timer Agent가 Lifecycle, Configurable, HealthChecker 인터페이스를 구현 |
| SPEC-MSG-001 | 동료 | Bridge Node 통한 메시지 기반 접근 시 Message 타입 사용 |
| SPEC-STORE-001 | 동료 | 동일한 System Agent 패턴(BaseLifecycle 임베딩, Options 패턴) 공유 |
| SPEC-FLOW-001 | 소비자 | 플로우가 Bridge Node 또는 직접 API로 Timer Agent 참조 |
| SPEC-OBS-001 | 소비자 | Timer 트리거 이벤트가 관찰성 시스템을 통해 로깅/메트릭 추적 |
| SPEC-CFG-001 | 소비자 | Timer 설정(최소 간격, 최대 타이머 수 등)을 설정 시스템으로 관리 |
