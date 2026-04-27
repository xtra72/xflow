---
id: SPEC-NODE-004
type: plan
version: "1.0.0"
spec_ref: SPEC-NODE-004
---

# SPEC-NODE-004 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반): 신규 파일이므로 TDD(RED-GREEN-REFACTOR) 적용
- 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표
- `go test -race` 필수 실행 (동시성 안전 검증)

### 1.2 기술 스택

- **언어**: Go 1.23+
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성**: `sync.Mutex` (내부 상태 보호), `atomic.Int64` (tickCount), `chan message.Message` (sourceCh)
- **시간 처리**: `time.Duration`, `time.Parse(time.RFC3339, ...)`, cron 표현식 변환
- **의존 패키지**:
  - `internal/node/` (SPEC-NODE-001): BaseNode, SourceNode, AgentResolver, NodeOption
  - `internal/agent/system/` (SPEC-TIMER-001): Timer 인터페이스, TimerHandler, TimerTrigger, TimerID
  - `pkg/lifecycle/` (SPEC-LIFE-001): BaseLifecycle, State
  - `pkg/flow/` (SPEC-FLOW-001): NodeDef
  - `pkg/message/` (SPEC-MSG-001): Message, NewMessage

### 1.3 패키지 위치

- **경로**: `internal/node/trigger.go`, `internal/node/trigger_test.go`
- **Registry 등록**: `internal/node/registry.go`의 `RegisterDefaults()` 수정

---

## 2. 마일스톤

### Primary Goal: TriggerNode Core + Schedule + Payload + Metadata + Errors (P0)

**범위**: Module 1, 2, 3, 4, 6

**작업 항목**:

1. `trigger.go` 에러 정의
   - 5개 sentinel error 변수 정의
   - `errors.Is()` 호환성 보장 (기존 `ErrInvalidConfig`, `ErrNodeNotInitialized` 래핑)

2. `trigger_test.go` 테스트 작성 (TDD RED)
   - NewTriggerNode 생성자 테스트
   - SourceCh() 채널 반환 테스트
   - Process() 빈 결과 반환 테스트
   - 스케줄 미설정 시 Init 에러 테스트
   - 잘못된 스케줄 타입 Init 에러 테스트
   - 잘못된 스케줄 값 Init 에러 테스트
   - interval 스케줄 메시지 생성 테스트
   - cron 스케줄 메시지 생성 테스트
   - once 스케줄 1회 메시지 생성 테스트
   - times 스케줄 cron 변환 테스트
   - 다중 스케줄 동시 실행 테스트
   - 정적 페이로드 (숫자, 문자열, 맵, 배열) 테스트
   - 기본 페이로드 생성 테스트
   - 메타데이터 자동 첨부 테스트
   - Timer Agent 미사용 시 에러 테스트
   - sourceCh 버퍼 풀 시 드롭 테스트

3. `trigger.go` 구현 (TDD GREEN)
   - TriggerScheduleType 상수 및 TriggerSchedule 구조체
   - TriggerNode 구조체 (BaseNode 임베딩)
   - NewTriggerNode() 팩토리 함수
   - SourceCh() 메서드
   - Init() 구현: 스케줄 파싱, Timer Agent resolve, 타이머 등록
   - Process() 구현: 빈 결과 반환
   - 페이로드 생성 로직 (정적/기본)
   - 메타데이터 첨부 로직
   - sourceCh 전송 로직 (non-blocking select)

4. `registry.go` 수정
   - `RegisterDefaults()`에 `"trigger"` 타입 등록

**산출물**: 기본 트리거 노드 핵심 기능 완성 (스케줄, 페이로드, 메타데이터, 에러)

---

### Secondary Goal: Lifecycle Integration (P0)

**범위**: Module 5

**작업 항목**:

1. 테스트 추가 (TDD RED)
   - Init 상태 전이 테스트 (Created -> Initializing -> Running)
   - Init 실패 시 상태 전이 테스트 (Created -> Initializing -> Error)
   - Shutdown 상태 전이 테스트 (Running -> Stopping -> Stopped)
   - Shutdown 시 타이머 전체 취소 테스트
   - Shutdown 시 sourceCh 닫기 테스트
   - Pause 시 메시지 생성 중단 테스트
   - Resume 시 메시지 생성 재개 테스트
   - Pause/Resume 반복 테스트

2. 구현 (TDD GREEN)
   - Init()에 상태 전이 로직 추가
   - Shutdown()에 타이머 취소 + 채널 닫기 로직
   - Pause/Resume 핸들링 (paused 플래그 기반)
   - Init 실패 시 부분 등록 타이머 롤백 로직

**산출물**: 전체 생명주기 통합 완성

---

### Tertiary Goal: 템플릿 페이로드 (P1)

**범위**: Module 3 (REQ-NODE-004-03-03)

**작업 항목**:

1. 테스트 추가
   - 템플릿 변수 치환 테스트 (`$.trigger_time`, `$.tick_count`, `$.schedule_id`, `$.trigger_id`)
   - 템플릿 평가 실패 시 에러 메타데이터 포함 메시지 전송 테스트
   - 중첩 오브젝트 내 템플릿 변수 치환 테스트

2. 구현
   - payloadTemplate 파서 및 변수 치환 로직
   - 트리거 컨텍스트에서 변수 추출
   - 평가 실패 시 폴백 로직 (빈 페이로드 + 에러 메타데이터)

**산출물**: 동적 페이로드 생성 기능

---

### Optional Goal: 통합 테스트 및 벤치마크 (P2)

**범위**: 전체 통합, 성능 검증

**작업 항목**:

1. 통합 테스트
   - Registry에서 "trigger" 타입 생성 확인
   - Mock Timer Agent와 완전 연동 테스트
   - 다중 스케줄(interval + cron + times) 혼합 테스트
   - Init -> Pause -> Resume -> Shutdown 전체 사이클 테스트

2. 벤치마크 테스트
   - 메시지 생성 throughput 벤치마크 (interval 10ms, 1초간)
   - 페이로드 복사 오버헤드 벤치마크
   - sourceCh 전송 지연 벤치마크

3. 동시성 검증
   - `go test -race` 통과 확인
   - goroutine 누수 테스트 (Init/Shutdown 사이클 후 goroutine 수 원복)

**산출물**: 전체 통합 검증 및 성능 목표 달성

---

## 3. 기술적 접근

### 3.1 TriggerNode 아키텍처

```
TriggerNode
├── *BaseNode                   // 임베딩 (Node 인터페이스 공통 구현)
├── sourceCh chan message.Message  // SourceNode 출력 채널
├── schedules []TriggerSchedule    // 설정된 스케줄 목록
├── payload any                    // 정적 페이로드
├── payloadTmpl map[string]any     // 템플릿 페이로드
├── timer system.Timer             // Timer Agent 참조
├── resolver AgentResolver         // Agent 조회용
├── entries []triggerTimerEntry    // 등록된 타이머 추적
├── paused bool                    // Pause 상태 플래그
└── mu sync.Mutex                  // 내부 상태 보호
```

### 3.2 Timer Agent 통합 전략

```
Init() 시:
  1. config["_agent_resolver"] 에서 AgentResolver 추출
  2. resolver.ResolveAgent("timer") 로 Timer Agent resolve
  3. Timer 인터페이스 type assertion
  4. 각 스케줄에 대해:
     - interval → timer.SetInterval(id, duration, handler)
     - cron     → timer.SetCron(id, expr, handler)
     - once     → timer.SetTimeout(id, delay, handler)
     - times    → "HH:MM" → "MM HH * * *" cron 변환 후 timer.SetCron()
  5. 핸들러(TimerHandler):
     func(trigger TimerTrigger) {
         n.mu.Lock()
         if n.paused { n.mu.Unlock(); return }
         n.mu.Unlock()
         msg := n.buildMessage(trigger, entry)
         select {
         case n.sourceCh <- msg:  // 전송 성공
         default:                  // 버퍼 풀 - 드롭
             n.logger.Warn("trigger message dropped: channel full")
         }
     }
```

### 3.3 페이로드 복사 전략

정적 페이로드는 트리거마다 깊은 복사(deep copy)하여 메시지 간 데이터 격리를 보장한다:

- 기본 타입(숫자, 문자열, 불리언): 값 복사 (자동)
- map[string]any: 재귀적 깊은 복사
- []any: 재귀적 깊은 복사

### 3.4 times -> cron 변환 전략

```
"09:00" → "0 9 * * *"     (매일 09:00)
"12:30" → "30 12 * * *"   (매일 12:30)
"18:00" → "0 18 * * *"    (매일 18:00)
```

각 시각은 `fmt.Sprintf("%d %d * * *", minute, hour)` 형태의 5필드 표준 cron으로 변환된다.

### 3.5 sourceCh Non-blocking 전송

```go
select {
case n.sourceCh <- msg:
    // 전송 성공
default:
    // 버퍼 풀 - 드롭 + 경고 로그
    n.logger.Warn("trigger message dropped",
        "node", n.Name(),
        "schedule_id", entry.scheduleID,
        "reason", "channel_full")
}
```

### 3.6 Mock Timer 전략

테스트에서는 Timer 인터페이스를 구현하는 Mock Timer를 사용한다:

- `SetInterval()`: 핸들러를 즉시 또는 지정 횟수만큼 동기 호출
- `SetCron()`: 핸들러를 즉시 호출 (cron 표현식 파싱 검증만)
- `SetTimeout()`: 핸들러를 즉시 호출
- `Cancel()`: timerID 기록만
- `List()`: 등록된 타이머 정보 반환

---

## 4. 리스크 및 대응

### Risk 1: Timer Agent 가용성

- **위험**: Timer System Agent가 초기화되지 않은 상태에서 TriggerNode Init 호출
- **대응**: AgentResolver가 nil 반환 시 `ErrTriggerTimerNotAvailable` 에러 반환. Timer Agent 상태 확인 로직 추가

### Risk 2: sourceCh 버퍼 고갈

- **위험**: 고빈도 interval (예: 10ms)에서 다운스트림 처리가 느릴 때 버퍼 고갈
- **대응**: Non-blocking select로 드롭 처리. 채널 버퍼 크기 설정 가능 (기본 64). 드롭 메트릭 기록

### Risk 3: 타이머 핸들러 panic

- **위험**: 페이로드 생성 또는 메시지 빌드 중 panic 발생 시 Timer Agent goroutine 중단
- **대응**: Timer Agent가 이미 핸들러에 recover() 래퍼를 적용 (SPEC-TIMER-001 보장). TriggerNode 핸들러 내부에도 추가 recover() 적용

### Risk 4: once 스케줄 과거 시각

- **위험**: 이미 지나간 시각이 once 스케줄에 설정된 경우
- **대응**: Init 시 과거 시각 검출하여 `ErrTriggerInvalidScheduleValue` 에러 반환

### Risk 5: times 스케줄 cron 변환 정확성

- **위험**: "HH:MM" 파싱 실패 또는 잘못된 cron 변환
- **대응**: 정규표현식 기반 "HH:MM" 검증. 변환된 cron 표현식을 Timer Agent에 등록 시 에러 체크

### Risk 6: Pause/Resume 레이스 컨디션

- **위험**: Pause/Resume 전환 시점에 진행 중인 타이머 핸들러와의 경쟁
- **대응**: `sync.Mutex`로 paused 플래그 보호. 핸들러 진입 시 paused 확인 후 즉시 리턴

---

## 5. 의존성 그래프

```
internal/node/trigger.go (본 SPEC)
  ├── 의존: internal/node/base.go          (SPEC-NODE-001: BaseNode, SourceNode, AgentResolver, NodeOption)
  ├── 의존: internal/node/errors.go        (SPEC-NODE-001: ErrInvalidConfig, ErrNodeNotInitialized)
  ├── 의존: internal/node/registry.go      (SPEC-NODE-001: Registry.RegisterDefaults 수정)
  ├── 의존: internal/agent/system/timer.go (SPEC-TIMER-001: Timer 인터페이스, TimerHandler, TimerTrigger, TimerID)
  ├── 의존: pkg/lifecycle/                 (SPEC-LIFE-001: BaseLifecycle, State)
  ├── 의존: pkg/flow/                      (SPEC-FLOW-001: NodeDef)
  ├── 의존: pkg/message/                   (SPEC-MSG-001: Message, NewMessage)
  ├── 의존: 표준 라이브러리                (context, sync, time, fmt, strings, strconv, errors)
  └── 소비자: internal/engine/             (SPEC-ENGINE-001: Engine이 SourceCh()에서 메시지 읽기)
```

---

## 6. 구현 순서 (파일별)

| 순서 | 파일 | 설명 | 의존성 |
|------|------|------|--------|
| 1 | trigger_test.go | Mock Timer + 전체 테스트 (TDD RED) | base.go, errors.go, system/timer.go |
| 2 | trigger.go | TriggerNode 구현 (TDD GREEN) | base.go, errors.go, system/timer.go |
| 3 | registry.go | RegisterDefaults()에 "trigger" 추가 | trigger.go |

모든 파일에 대해 TDD 방식으로 테스트 파일을 먼저 작성한다.
