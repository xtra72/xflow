---
id: SPEC-TIMER-001
type: plan
version: "1.0.0"
spec_ref: SPEC-TIMER-001
status: draft
---

# SPEC-TIMER-001 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반): 신규 파일이므로 TDD(RED-GREEN-REFACTOR) 적용
- 모든 파일이 신규 생성이므로 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표
- `go test -race` 필수 실행 (동시성 안전 검증)

### 1.2 기술 스택

- **언어**: Go 1.23+
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성**: `sync.RWMutex` (타이머 맵 보호), `sync/atomic` (통계 카운터)
- **타이머**: `time.Ticker` (Interval), `time.AfterFunc` (Timeout)
- **크론**: `github.com/robfig/cron/v3`
- **생명주기**: `pkg/lifecycle/` (SPEC-LIFE-001)
- **선택적**: `pkg/message/` (Bridge Node 연동 시)

### 1.3 패키지 위치

- **경로**: `internal/agent/system/`
- **Tier**: internal (비공개 패키지)
- **소비자**: internal/engine/ (Flow Runtime), internal/node/ (Bridge Node), Script Node (Lua 바인딩)

---

## 2. 마일스톤

### Primary Goal: Error Types + Timer Interface + Interval + Cron + Timeout + Lifecycle (P0)

**범위**: Module 1, 2, 3, 4, 5, 7

**작업 항목**:

1. `timer_errors.go` + `timer_errors_test.go` 작성
   - 10개 sentinel error 변수 정의
   - `errors.Is()` 호환성 테스트

2. `timer.go` (인터페이스 부분) 작성
   - `Timer` 인터페이스 정의 (5개 메서드)
   - `TimerHandler`, `TimerTrigger`, `TimerID`, `TimerInfo`, `TimerType` 타입 정의
   - `TimerAgent` 구조체 기본 골격

3. `timer_options.go` 작성
   - `TimerOption` 함수 타입
   - `defaultConfig()` 팩토리 함수
   - `WithMinInterval()`, `WithMaxTimers()`, `WithCronParser()` 옵션

4. `timer_interval.go` + `timer_interval_test.go` 작성
   - `time.Ticker` 기반 주기적 실행 구현
   - 최소 간격(100ms) 검증 테스트
   - TickCount 추적 테스트
   - 독립 goroutine 실행 테스트
   - Cancel 시 Ticker 정지 테스트
   - 동시성 안전 테스트

5. `timer_cron.go` + `timer_cron_test.go` 작성
   - `robfig/cron/v3` 기반 크론 스케줄 구현
   - 5필드 표준 크론 파싱 테스트
   - 6필드 초 단위 크론 파싱 테스트 (옵션)
   - 유효하지 않은 크론 표현식 거부 테스트
   - Cancel 시 Cron Entry 제거 테스트
   - ScheduleID 추적 테스트

6. `timer_timeout.go` + `timer_timeout_test.go` 작성
   - `time.AfterFunc` 기반 지연 실행 구현
   - 단일 실행 확인 테스트
   - 실행 후 자동 제거 테스트
   - Cancel 시 실행 방지 테스트
   - 음수/0 지연 거부 테스트

7. `timer.go` (TimerAgent 부분) 완성 + `timer_test.go` 작성
   - `NewTimerAgent()` 생성자
   - `BaseLifecycle` 임베딩
   - `Init()`, `Start()`, `Pause()`, `Resume()`, `Stop()` 구현
   - `Configure()`, `GetConfig()` 구현
   - `HealthCheck()` 구현
   - 생명주기 전체 흐름 통합 테스트
   - Pause 시 등록 거부 / Cancel 허용 테스트
   - Graceful Shutdown 테스트 (핸들러 완료 대기)
   - 핸들러 패닉 recover 테스트
   - 최대 타이머 수 제한 테스트

**산출물**: Timer Agent의 핵심 기능 완성 (Interval, Cron, Timeout, 생명주기)

---

### Secondary Goal: Bridge Node Integration (P1)

**범위**: Module 6

**작업 항목**:

1. `timer_bridge.go` + `timer_bridge_test.go` 작성
   - Bridge Node 메시지 핸들러 구현
   - `timer.operation` 메타데이터 파싱
   - set_interval/set_cron/set_timeout 메시지 처리
   - cancel 메시지 처리
   - list 요청-응답 패턴 (JSON 배열 반환)
   - 잘못된 연산 유형 에러 처리 테스트

**산출물**: Bridge Node를 통한 메시지 기반 Timer 제어 완성

---

## 3. 기술적 접근

### 3.1 Interval 타이머 설계

`time.Ticker` 기반으로 각 Interval 타이머를 독립 goroutine에서 실행한다:

- 각 타이머마다 `time.NewTicker(interval)` + 전용 goroutine 생성
- goroutine 내에서 `select { case <-ticker.C: ... case <-done: return }` 패턴 사용
- `done` 채널 또는 `context.Cancel`로 goroutine 종료 제어
- 핸들러 호출 시 `recover()`로 패닉 보호
- `atomic.AddInt64(&tickCount, 1)`로 발화 횟수 원자적 증가

최소 간격 100ms 설계 이유:
- IoT 센서 폴링에서 100ms 미만은 거의 불필요
- 과도한 Ticker가 CPU/메모리를 낭비하는 것을 방지
- 설정 가능하도록 `WithMinInterval()` 옵션 제공

### 3.2 Cron 타이머 설계

`robfig/cron/v3`의 `cron.Cron` 인스턴스를 공유한다:

- TimerAgent 초기화 시 `cron.New(cron.WithParser(...))` 생성
- `SetCron()` 호출 시 `cron.AddFunc(expr, wrappedHandler)` 등록
- `wrappedHandler`에서 `TimerTrigger` 생성 후 사용자 핸들러 호출
- `cron.EntryID`를 `timerEntry.cronID`에 저장하여 Cancel 시 `cron.Remove(entryID)` 호출
- `cron.Start()`는 Init 시 호출, `cron.Stop()`은 Stop 시 호출
- `cron.Stop()`은 `context.Context`를 반환하므로 실행 중인 핸들러 완료를 대기

크론 파서 옵션:
- 기본: `cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)` (5필드)
- 옵션: `cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)` (6필드)
- `WithCronParser()` 옵션으로 사용자가 선택 가능

### 3.3 Timeout 타이머 설계

`time.AfterFunc`을 사용하여 단일 실행 타이머를 구현한다:

- `SetTimeout()` 호출 시 `time.AfterFunc(delay, wrappedHandler)` 생성
- `wrappedHandler`에서 핸들러 호출 후 타이머 맵에서 자동 제거
- `Cancel()` 호출 시 `timer.Stop()` → 반환값이 false이면 이미 실행됨 (에러 없이 무시)
- `cancel`된 Timeout은 타이머 맵에서 제거

### 3.4 타이머 맵 관리

```
map[TimerID]*timerEntry

timerEntry 구조:
  - info: TimerInfo (ID, Type, Expression, NextFire, LastFired, FireCount, Active)
  - handler: TimerHandler (사용자 콜백)
  - ticker: *time.Ticker (Interval 전용)
  - cronID: cron.EntryID (Cron 전용)
  - timer: *time.Timer (Timeout 전용)
  - cancel: context.CancelFunc (goroutine 종료용)
  - tickCount: int64 (atomic, 누적 발화 횟수)
```

동시성 제어:
- `sync.RWMutex`로 타이머 맵 보호
- 등록/취소: Write Lock
- 조회(List): Read Lock
- 핸들러 실행: Lock 외부 (Lock 내에서 핸들러를 호출하지 않음)

### 3.5 생명주기 통합

TimerAgent의 상태 전이:

```
Created -> Init() -> Initializing -> (cron 스케줄러 초기화, 타이머 맵 생성) -> Running
Running -> Pause() -> Paused (등록 거부, 트리거 일시정지)
Paused -> Resume() -> Running
Running/Paused -> Stop() -> Stopping -> (모든 타이머 취소, 핸들러 대기, cron 정지) -> Stopped
```

Pause 동작:
- 새 타이머 등록 거부 (`ErrTimerPaused`)
- Cancel은 허용 (기존 타이머 정리 가능)
- List는 허용 (읽기 전용)
- 활성 타이머의 트리거 일시정지 (핸들러 호출 건너뛰기)

Stop 동작:
1. `closed = true` 설정
2. 모든 Interval 타이머: `ticker.Stop()` + `cancel()` (goroutine 종료)
3. 모든 Timeout 타이머: `timer.Stop()`
4. Cron 스케줄러: `cron.Stop()` 호출 후 `<-ctx.Done()` 대기
5. `wg.Wait()` - 실행 중인 핸들러 완료 대기
6. 타이머 맵 비우기

### 3.6 핸들러 패닉 보호

```go
func (t *TimerAgent) safeCall(handler TimerHandler, trigger TimerTrigger) {
    defer func() {
        if r := recover(); r != nil {
            // 로그 기록 (패닉 발생, 하지만 Agent는 계속 동작)
        }
    }()
    t.wg.Add(1)
    defer t.wg.Done()
    handler(trigger)
}
```

---

## 4. 리스크 및 대응

### Risk 1: goroutine 누수

- **위험**: Interval 타이머의 goroutine이 Cancel이나 Stop 없이 누수될 수 있음
- **대응**: `context.WithCancel`로 모든 goroutine 종료 제어. Stop 시 모든 goroutine Cancel. `wg.Wait()`로 완료 확인

### Risk 2: 핸들러 장시간 블로킹

- **위험**: 핸들러가 무한 루프나 장시간 블로킹으로 Graceful Shutdown을 방해
- **대응**: `wg.Wait()`에 타임아웃 추가 검토. 현재 MVP에서는 핸들러가 합리적 시간 내 완료된다고 가정

### Risk 3: 대량 타이머 등록 시 성능

- **위험**: 1000개 이상의 Interval 타이머가 각각 goroutine을 점유하여 리소스 과부하
- **대응**: `maxTimers` 제한 (기본 1000)으로 방지. 초과 시 `ErrMaxTimersReached` 반환. Cron 타이머는 공유 스케줄러를 사용하므로 goroutine 오버헤드가 적음

### Risk 4: cron 표현식 검증 신뢰성

- **위험**: `robfig/cron/v3`가 예상과 다른 표현식을 수용하거나 거부할 수 있음
- **대응**: `cron.Parser.Parse()` 호출로 사전 검증 후 에러 시 `ErrInvalidCronExpression` 반환. 라이브러리 버전 고정

### Risk 5: Timeout 타이머 실행 후 Cancel 경합

- **위험**: Timeout 발화와 Cancel이 동시에 호출되면 경합 발생
- **대응**: `sync.RWMutex`로 타이머 맵 접근 제어. `timer.Stop()` 반환값으로 이미 실행 여부 확인. 자동 제거와 Cancel의 이중 제거를 안전하게 처리 (이미 없는 경우 무시)

### Risk 6: Pause 시 Interval Ticker 일시정지

- **위험**: `time.Ticker`는 Pause를 지원하지 않으므로 Tick이 계속 발생
- **대응**: Pause 상태에서는 Tick 수신 시 핸들러 호출을 건너뛰기 (paused 플래그 확인). Ticker 자체는 계속 동작하되 핸들러만 비활성화

---

## 5. 의존성 그래프

```
internal/agent/system/timer.go (본 SPEC)
  ├── 의존: pkg/lifecycle/          (SPEC-LIFE-001: BaseLifecycle, Configurable, HealthChecker)
  ├── 의존: github.com/robfig/cron/v3 (Cron 스케줄러)
  ├── 의존: 표준 라이브러리         (sync, time, context, errors, fmt, sync/atomic)
  ├── 선택: pkg/message/            (SPEC-MSG-001: Bridge Node 연동 시 Message 타입)
  ├── 소비자: internal/engine/      (Flow Runtime에서 TimerAgent 초기화)
  ├── 소비자: internal/node/bridge.go (Bridge Node에서 Timer 연산 메시지 처리)
  ├── 소비자: internal/script/      (Lua 바인딩에서 Timer 접근)
  └── 동료: internal/agent/system/event.go, logger.go, file.go, store.go (다른 System Agent)
```

---

## 6. 구현 순서 (파일별)

| 순서 | 파일 | 설명 | 의존성 |
|------|------|------|--------|
| 1 | timer_errors.go | Sentinel 에러 정의 | 없음 |
| 2 | timer.go (인터페이스) | Timer 인터페이스, 타입 정의 | timer_errors.go |
| 3 | timer_options.go | TimerOption 타입, 옵션 함수 | timer.go |
| 4 | timer_interval.go | Interval 타이머 구현 | timer.go, timer_errors.go |
| 5 | timer_cron.go | Cron 타이머 구현 | timer.go, timer_errors.go, cron/v3 |
| 6 | timer_timeout.go | Timeout 타이머 구현 | timer.go, timer_errors.go |
| 7 | timer.go (TimerAgent) | TimerAgent 생명주기 구현 | 전체 (1-6) + pkg/lifecycle/ |
| 8 | timer_bridge.go | Bridge Node 메시지 핸들러 | timer.go, pkg/message/ |

모든 파일에 대해 TDD 방식으로 테스트 파일(`*_test.go`)을 먼저 작성한다.
