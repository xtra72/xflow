---
id: SPEC-NODE-004
type: plan
version: "1.3.0"
spec_ref: SPEC-NODE-004
---

# SPEC-NODE-004 구현 계획

> **v1.2.0 개정 노트 (2026-07-28)**: 아래 §1~§6은 v1.0.0/v1.1.0 원본 계획이다. v1.2.0 인플레이스 개정으로 추가된 3종 기능(weekly / monthly / per-schedule payload)의 구현 계획은 **§7 (v1.2.0 개정 마일스톤)** 및 **§8 (v1.2.0 기술적 접근)**에 별도로 기술한다. 코드는 후속 run-phase에서 구현한다.
>
> **v1.3.0 개정 노트 (2026-07-28)**: 페이로드 모델을 단일 통합 템플릿 엔진으로 통합(static/template 이원성 제거 + `$$` 이스케이프)하는 개정의 구현 계획은 **§10 (v1.3.0 개정 마일스톤)**, **§11 (v1.3.0 기술적 접근)**, **§12 (v1.3.0 리스크)**에 별도로 기술한다. Tier M(3파일: spec/plan/acceptance) 유지, design.md/research.md 없음. 코드는 후속 run-phase에서 구현한다.

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

---

## 7. v1.2.0 개정 마일스톤 (weekly / monthly / per-schedule payload)

기존 구조(하위 호환)를 보존하면서 3종 기능을 추가한다. 방법론은 신규 로직에 대해 TDD(RED-GREEN-REFACTOR)를 적용한다.

### Primary Goal (v1.2.0): weekly 스케줄 타입 (P0)

**범위**: REQ-NODE-004-02-09

**작업 항목**:

1. 테스트 추가 (TDD RED)
   - weekday 토큰 파싱 테스트 (`sun`~`sat`, 정수 `0`~`6`)
   - `days × times` 조합 → cron 등록 수 검증 (예: 3요일 × 2시각 = 6개)
   - 생성 cron 표현식 정확성 (`0 9 * * 1` 등) 검증
   - 잘못된 요일 토큰/시각 → `ErrTriggerInvalidScheduleValue`
2. 구현 (TDD GREEN)
   - `TriggerScheduleWeekly` 상수, `TriggerSchedule.Days`/`Times` 필드 파싱
   - weekday 토큰 → cron Dow 매핑 테이블
   - `(day × time)` 조합마다 `SetCron("MM HH * * DOW")` 등록

**산출물**: weekly 스케줄 타입 완성

### Secondary Goal (v1.2.0): monthly 스케줄 타입 + last-day 게이트 (P0)

**범위**: REQ-NODE-004-02-10, REQ-NODE-004-02-11

**작업 항목**:

1. 테스트 추가 (TDD RED)
   - `day` 정수(1~31) / `"first"` → `MM HH N * *` cron 등록 검증
   - `day="last"` → 매일 cron(`MM HH * * *`) 등록 검증
   - 월말 게이트: 핸들러가 `time.Now().Day() == lastDayOfMonth` 일 때만 emit (2월 28/29, 30일 달, 31일 달 각각 검증 — 주입 가능한 clock 사용)
   - 존재하지 않는 정수 일자(31)가 짧은 달에 미발화 (표준 cron 동작) 문서화 테스트
   - 잘못된 `day` 값 → `ErrTriggerInvalidScheduleValue`
2. 구현 (TDD GREEN)
   - `TriggerScheduleMonthly` 상수, `TriggerSchedule.Day`/`Times` 파싱
   - `day` 정수/`first`/`last` 분기
   - last-day 게이트: entry에 last-day 플래그 저장, 핸들러 진입 시 월 마지막 날 계산 후 게이트

**산출물**: monthly 스케줄 타입(date/first/last) 완성

### Tertiary Goal (v1.2.0): 스케줄별 페이로드 (per-schedule payload) (P0)

**범위**: REQ-NODE-004-03-04 (전 스케줄 타입 적용)

**작업 항목**:

1. 테스트 추가 (TDD RED)
   - 스케줄 항목 payload 존재 → 항목 페이로드 우선 사용
   - 스케줄 항목 payload 없음 → 노드 레벨 페이로드 폴백
   - 노드 레벨도 없음 → 기본 페이로드 폴백
   - 항목 `payload_template` 우선 치환 테스트
   - 하위 호환: 기존 노드-레벨 전용 설정이 오늘과 동일 동작 (회귀 테스트)
   - interval/cron/once/times/weekly/monthly 전 타입에서 per-schedule payload 동작 검증
2. 구현 (TDD GREEN)
   - `TriggerSchedule.Payload`/`PayloadTmpl` 파싱
   - `buildMessage()` 페이로드 결정부를 3단계 폴백 순서로 재작성 (entry 페이로드 → 노드 레벨 → 기본)
   - 깊은 복사 정책 유지

**산출물**: 스케줄별 페이로드 오버라이드 완성 (하위 호환 보존)

### Optional Goal (v1.2.0): Web UI weekly/monthly 위젯 + 스케줄별 페이로드 편집 (P1)

**범위**: REQ-NODE-004-08-05

**작업 항목**:

1. `TriggerScheduleEditor.tsx`: weekly(요일 토글 + 시각 칩), monthly(일자 선택 + 시각 칩) 전용 위젯 추가
2. `nodeSchemas.ts`: `trigger_schedules` 위젯이 신규 필드(`days`/`day`/`times`) 직렬화 지원
3. 각 스케줄 항목에 "이 스케줄 전용 페이로드(선택)" 서브 섹션 (항목별 payload_mode 토글)
4. 인라인 검증: 요일 최소 1개, 일자 유효성, `HH:MM` 정규식, 29~31일 미발화 안내 힌트

**산출물**: 웹 UI 편집 지원 (백엔드 저장 형식과 호환)

---

## 8. v1.2.0 기술적 접근

### 8.1 weekly → cron 변환 전략

```
days=["mon","wed","fri"], times=["09:00","18:00"]
  → 조합 (day × time) 6개:
     mon 09:00 → "0 9 * * 1"
     mon 18:00 → "0 18 * * 1"
     wed 09:00 → "0 9 * * 3"
     wed 18:00 → "0 18 * * 3"
     fri 09:00 → "0 9 * * 5"
     fri 18:00 → "0 18 * * 5"
```

weekday 토큰 매핑: `sun/0→0, mon/1→1, tue/2→2, wed/3→3, thu/4→4, fri/5→5, sat/6→6`.
`times` 타입과 동일한 `SetCron()` 경로를 사용하므로 별도 타임존 처리는 없다 (A13).

### 8.2 monthly → cron 변환 전략

```
day=15, times=["08:30"]        → "30 8 15 * *"   (매월 15일)
day="first", times=["00:00"]   → "0 0 1 * *"     (매월 1일)
day="last", times=["23:59"]    → "59 23 * * *"   (매일 등록) + 핸들러 월말 게이트
```

- 정수/`first`: 표준 5필드 cron `MM HH N * *`. 존재하지 않는 일자(31 등)는 짧은 달에 미발화 (표준 cron, 의도됨).
- `last`: robfig/cron이 마지막 날을 표현하지 못하므로 매일 cron으로 등록 후 게이트.

### 8.3 monthly last-day 게이트 구현

```go
// entry.isLastDay == true 인 스케줄 핸들러 진입부:
now := n.clock.Now()
lastDay := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()
if now.Day() != lastDay {
    return // 월말 아님 → emit 안 함
}
// 월말 → 정상 메시지 생성
```

`time.Date(year, month+1, 0, ...)`는 "다음 달 0일" = "이번 달 마지막 날"을 반환하므로
2월(28/29), 30일 달, 31일 달을 모두 자동 처리한다. 테스트 용이성을 위해 주입 가능한 clock 인터페이스 사용을 권장한다.

### 8.4 per-schedule payload 결정 로직

```go
func (n *TriggerNode) resolvePayload(entry triggerTimerEntry, tr TimerTrigger) any {
    // (1) 스케줄 항목 페이로드 우선
    if entry.payload != nil { return deepCopy(entry.payload) }
    if entry.payloadTmpl != nil { return renderTemplate(entry.payloadTmpl, tr, entry) }
    // (2) 노드 레벨 폴백
    if n.payload != nil { return deepCopy(n.payload) }
    if n.payloadTmpl != nil { return renderTemplate(n.payloadTmpl, tr, entry) }
    // (3) 기본 페이로드
    return map[string]any{"trigger_time": tr.TriggerAt.Format(time.RFC3339)}
}
```

기존 `buildMessage()`의 노드-레벨 전용 로직을 위 3단계 폴백으로 승격한다. entry 페이로드가 없는 기존 설정은 (2)/(3) 경로로 오늘과 동일하게 동작한다 (하위 호환).

---

## 9. v1.2.0 리스크 및 대응

### Risk 7: monthly last-day 게이트의 시간대/경계 오류

- **위험**: 자정 근처 실행 시 날짜 경계, DST 전환, 잘못된 마지막 날 계산
- **대응**: `time.Date(y, m+1, 0, ...)` 표준 관용구 사용(윤년/월별 일수 자동 처리). 주입 가능한 clock으로 2월 28/29, 30/31일 달 경계 테스트

### Risk 8: weekly/monthly 다중 조합 타이머 누수

- **위험**: `days × times` 조합으로 다수 타이머 등록 시 Init 실패 롤백 또는 Shutdown 누락
- **대응**: 모든 등록 타이머를 `entries`에 추적. Init 부분 실패 시 기존 롤백 로직(REQ-NODE-004-05-01) 재사용, Shutdown 시 전체 Cancel

### Risk 9: per-schedule payload 하위 호환 회귀

- **위험**: 페이로드 결정부 재작성으로 기존 노드-레벨 페이로드 동작 변경
- **대응**: 기존 설정(노드 레벨 전용) 회귀 테스트를 명시적 AC로 추가(AC-NODE-004-42). 폴백 순서 (2)/(3)이 기존 경로와 동일함을 보장

---

## 10. v1.3.0 개정 마일스톤 (통합 페이로드 엔진)

기존 페이로드 결정부(v1.2.0의 3단계 폴백)를 보존하면서, **평가 단계**를 단일 통합 템플릿 엔진으로 교체한다. static/template 이원 분기를 제거하되 config 키 수용은 하위호환으로 유지한다. 신규 로직에 TDD(RED-GREEN-REFACTOR)를 적용한다.

### Primary Goal (v1.3.0): 통합 템플릿 스캐너 (P0)

**범위**: REQ-NODE-004-03-01, REQ-NODE-004-03-03, REQ-NODE-004-06-04

**작업 항목**:

1. 테스트 추가 (TDD RED)
   - 통째 변수 치환(문자열 전체 `$.<var>` → 네이티브 타입): 숫자/문자열 각각 (AC-19, AC-48)
   - 문자 단위 스캔: `$$`→리터럴 `$` (AC-46), 리터럴+변수 혼합 interpolation (AC-47)
   - 미지 변수 `$.x` → 에러 기록 + 키 null (AC-20)
   - 비문자열 값(숫자/불리언/배열/중첩 맵) 리터럴 패스스루 + 중첩 맵 미재귀 (AC-18)
2. 구현 (TDD GREEN)
   - `evalPayload(src map[string]any, ctx) (map[string]any, []error)`: 최상위 문자열 값만 스캔
   - 문자열 평가기 `evalString(s, ctx) (any, bool, error)`: (a) 통째 일치 → 네이티브 값(any) 반환, (b) 그 외 char 스캔 → 문자열 빌드
   - char 스캐너: `$$`/`$.<name>`/lone `$`/일반문자 상태 처리, 미지 변수 시 에러 + null 반환

**산출물**: 단일 통합 템플릿 스캐너 완성 (whole-value 타입 보존 + interpolation + `$$` 이스케이프)

### Secondary Goal (v1.3.0): config 하위호환 라우팅 + 결정부 통합 (P0)

**범위**: REQ-NODE-004-03-03, REQ-NODE-004-03-04 (전 스케줄 타입)

**작업 항목**:

1. 테스트 추가 (TDD RED)
   - 노드/스케줄 `payload`(map) → 직접 평가; 비-map 스칼라/배열 → `{"value": v}` 래핑 후 평가 (AC-13/14/16/50)
   - 구 static 맵 동일 출력 회귀 (AC-49)
   - per-schedule 해결 순서(항목→노드→기본)가 통합 엔진 적용 후에도 불변 (AC-41~44)
2. 구현 (TDD GREEN)
   - `normalizeSource(v any) map[string]any`: map이면 그대로, 아니면 `{"value": v}` 래핑
   - `resolvePayload()`(v1.2.0 3단계 폴백)의 반환을 `normalizeSource` → `evalPayload` 파이프라인으로 연결
   - v1.2.0 static-복사/template-치환 이원 분기 제거, 단일 경로로 교체

**산출물**: config 하위호환 라우팅 + 통합 결정-평가 파이프라인 (하위호환 보존)

### Optional Goal (v1.3.0): Web UI 단일 페이로드 에디터 (P1)

**범위**: REQ-NODE-004-08-04(폐기), REQ-NODE-004-08-05

**작업 항목**:

1. `PropertyPanel.tsx`: `payload_mode` 가상 필드 유도/정리 로직 제거, 로드 시 `payload`/`payload_template` 병합 로드
2. `nodeSchemas.ts`: `payload_mode`(select)·별도 `payload_template` 필드 제거, 단일 `payload` object 에디터로 대체
3. `TriggerScheduleEditor.tsx`: 항목별 payload_mode 서브 토글 제거, 단일 JSON 페이로드 에디터로 통일
4. 인라인 힌트: `$.<var>` 4종 목록 + `$$` 이스케이프 설명 노출 (AC-51)

**산출물**: 단일 페이로드 에디터 UI (백엔드 통합 엔진과 호환)

---

## 11. v1.3.0 기술적 접근

### 11.1 통합 문자열 평가 규칙

```
evalString(s):
  if s == "$." + name  and name ∈ knownVars:   // 통째 일치
      return nativeValue(name)                  // 숫자/문자열 등 네이티브 타입 (any)
  else:                                          // 문자 단위 스캔 (interpolation)
      buf := ""
      i := 0
      while i < len(s):
          if s[i:i+2] == "$$":      buf += "$"; i += 2
          elif s[i:i+2] == "$.":    name := scanName(s, i+2)
                                    if name ∈ knownVars: buf += stringForm(name)
                                    else:                 err = unknownVar(name); return null, err
                                    i += 2 + len(name)
          else:                     buf += s[i]; i += 1
      return buf
```

- **통째 일치만 네이티브 타입 보존**: 부분 일치(interpolation)에서는 항상 문자열 형태로 삽입한다 (예: `"count=$.tick_count"` → `"count=7"`).
- **알려진 변수 4종 불변**: `$.trigger_time`, `$.tick_count`, `$.schedule_id`, `$.trigger_id` (v1.2.0 컨텍스트 재사용).

### 11.2 config 하위호환 라우팅

```
normalizeSource(v):
  if v is map[string]any: return v            // 직접 평가
  else:                   return {"value": v} // 구 static 스칼라/배열 래핑

evalPayload(src):
  m := normalizeSource(src)
  out := {}
  errs := []
  for k, val := range m:
      if val is string:  out[k], e := evalString(val); if e: errs.append(e)
      else:              out[k] = deepCopy(val)   // 비문자열 리터럴 패스스루 (중첩 맵 미재귀)
  return out, errs
```

- **중첩 맵 미재귀**: `evalPayload`는 최상위 맵의 문자열 값만 평가한다. 중첩 map/배열은 `deepCopy`로 리터럴 패스스루된다 (알려진 한계, A17).
- **에러 처리**: `errs`가 비어있지 않으면 `trigger.error` 메타데이터를 첨부하고, 문제 키는 이미 `null`로 설정되어 있다 (REQ-06-04).

### 11.3 결정-평가 파이프라인 통합

v1.2.0 `resolvePayload()`(항목→노드→기본 3단계 폴백)는 그대로 유지하되, 반환값을 `normalizeSource → evalPayload` 로 흘려보낸다. static-깊은복사/template-치환의 이원 분기는 제거되고, 모든 경로가 동일 파이프라인을 통과한다. 기본 페이로드 `{"trigger_time": now}`는 이미 map이므로 동일하게 평가된다(리터럴 통과).

### 11.4 마이그레이션 영향 (문서화)

- 구 static 맵 `{"cmd":"open"}` → 동일 출력 (리터럴 통과).
- 두 경계 케이스만 의미 변화: (1) 리터럴 `$.trigger_time` 포함 static 문자열 → interpolation, (2) 리터럴 `$` → `$$` 필요. run-phase 구현 시 CHANGELOG/마이그레이션 노트에 명시한다.

---

## 12. v1.3.0 리스크 및 대응

### Risk 10: 통째 일치 vs interpolation 경계 판정 오류

- **위험**: `"$.tick_count"`(통째, 숫자)와 `"$.tick_count "`(뒤 공백, 문자열)의 구분 실패로 타입 손상
- **대응**: 통째 일치는 `s == "$." + name` **정확 일치**로만 판정. 공백/추가 문자 포함 시 즉시 interpolation 경로. 경계 테스트(AC-48) 추가

### Risk 11: `$$`/`$.`/lone `$` 스캐너 상태 오류

- **위험**: 문자열 끝 단독 `$`, `$$$` 연속, `$.`뒤 빈 이름 등 엣지 케이스에서 스캐너 오작동
- **대응**: char 스캐너 상태별 단위 테스트(문자열 끝 `$`, 연속 `$$`, `$.` 뒤 비영숫자) 추가. 이름 스캔은 알려진 변수 이름 문자 집합으로 한정

### Risk 12: config 하위호환 회귀 (스칼라 래핑 의미 변화)

- **위험**: 구 static 스칼라(`payload: 42`)가 `{"value":42}`로 래핑되어 기존 소비자가 최상위 스칼라를 기대할 경우 파손
- **대응**: 스칼라 래핑은 **의도된 하위호환 규약**임을 마이그레이션 노트에 명시(AC-50). 맵 페이로드는 래핑 없이 동일 동작(AC-49)하여 대부분의 실사용 케이스는 영향 없음
