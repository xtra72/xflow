---
id: SPEC-NODE-004
type: acceptance
version: "1.3.0"
spec_ref: SPEC-NODE-004
---

# SPEC-NODE-004 수락 기준

> **v1.2.0 개정 노트 (2026-07-28)**: AC-NODE-004-01 ~ 34는 v1.0.0/v1.1.0 원본 수락 기준이다. v1.2.0 개정으로 추가된 weekly / monthly / per-schedule payload 및 하위 호환 시나리오는 **Module 7 (v1.2.0 신규 기능)** 에 AC-NODE-004-35 이후로 추가된다.
>
> **v1.3.0 개정 노트 (2026-07-28)**: 페이로드 모델을 단일 통합 템플릿 엔진으로 통합함에 따라, **Module 3(AC-NODE-004-13 ~ 20)의 정적/템플릿 시나리오는 통합 모델로 개정**되었다. config 하위호환 라우팅에 따라 노드 레벨 비-map 스칼라/배열 페이로드는 `{"value": <v>}`로 래핑된다. 통합 엔진 신규 시나리오(`$$` 이스케이프, 리터럴+변수 혼합, 통째 타입 치환, 미지 변수 에러+null, 구 static 마이그레이션, 단일 페이로드 에디터)는 **Module 8 (v1.3.0 통합 페이로드 엔진)** 에 AC-NODE-004-46 이후로 추가된다. 프론트엔드 AC-NODE-004-45도 단일 페이로드 에디터로 개정되었다.

## Module 1: TriggerNode Core - 트리거 노드 핵심

### AC-NODE-004-01: NewTriggerNode 생성자

```gherkin
Given interval 스케줄 1개와 payload가 설정된 flow.NodeDef가 주어졌을 때
When NewTriggerNode(def, opts...)를 호출하면
Then Node 인터페이스를 구현한 TriggerNode 인스턴스를 반환해야 한다
And Type()이 "trigger"여야 한다
And SourceCh()가 nil이 아닌 읽기 전용 채널을 반환해야 한다
```

### AC-NODE-004-02: SourceNode 인터페이스 구현

```gherkin
Given NewTriggerNode()로 생성된 TriggerNode가 있을 때
Then SourceNode 인터페이스로 타입 단언이 성공해야 한다
And SourceCh()가 <-chan message.Message 타입을 반환해야 한다
```

### AC-NODE-004-03: Process 메서드 (SourceNode 무시)

```gherkin
Given 초기화된 TriggerNode가 있을 때
When Process(ctx, msg)를 임의의 메시지로 호출하면
Then 빈 메시지 슬라이스(길이 0)를 반환해야 한다
And error는 nil이어야 한다
```

### AC-NODE-004-04: Registry 등록 확인

```gherkin
Given NewRegistry()로 생성된 Registry가 있을 때
Then Has("trigger")가 true여야 한다
And Types()에 "trigger"가 포함되어야 한다

When Create(triggerNodeDef)를 호출하면
Then Node 인터페이스를 구현한 인스턴스를 반환해야 한다
And 반환된 노드의 Type()이 "trigger"여야 한다
```

---

## Module 2: Schedule Configuration - 스케줄 설정

### AC-NODE-004-05: interval 스케줄 메시지 생성

```gherkin
Given TriggerNode에 schedules: [{"type": "interval", "value": "100ms"}]가 설정되어 있을 때
When Init(ctx)를 호출하고 300ms 대기하면
Then SourceCh()에서 최소 2개의 메시지를 수신할 수 있어야 한다
And 각 메시지의 Metadata에 trigger.schedule_type이 "interval"이어야 한다
```

### AC-NODE-004-06: cron 스케줄 등록

```gherkin
Given TriggerNode에 schedules: [{"type": "cron", "value": "0 */5 * * * *"}]가 설정되어 있을 때
When Init(ctx)를 호출하면
Then Mock Timer의 SetCron()이 cron 표현식 "0 */5 * * * *"로 호출되어야 한다
And 에러가 nil이어야 한다

When Mock Timer가 핸들러를 호출하면
Then SourceCh()에서 메시지 1개를 수신할 수 있어야 한다
And Metadata의 trigger.schedule_type이 "cron"이어야 한다
```

### AC-NODE-004-07: once 스케줄 1회 실행

```gherkin
Given TriggerNode에 schedules: [{"type": "once", "value": "<미래시각>"}]가 설정되어 있을 때
When Init(ctx)를 호출하면
Then Mock Timer의 SetTimeout()이 호출되어야 한다
And delay가 현재 시각과 설정 시각의 차이여야 한다

When Mock Timer가 핸들러를 호출하면
Then SourceCh()에서 메시지 1개를 수신할 수 있어야 한다
And Metadata의 trigger.schedule_type이 "once"여야 한다
```

### AC-NODE-004-08: times 스케줄 cron 변환

```gherkin
Given TriggerNode에 schedules: [{"type": "times", "value": ["09:00", "12:30", "18:00"]}]가 설정되어 있을 때
When Init(ctx)를 호출하면
Then Mock Timer의 SetCron()이 3번 호출되어야 한다
And 첫 번째 호출의 cron 표현식이 "0 9 * * *"여야 한다
And 두 번째 호출의 cron 표현식이 "30 12 * * *"여야 한다
And 세 번째 호출의 cron 표현식이 "0 18 * * *"여야 한다
```

### AC-NODE-004-09: 다중 스케줄 동시 실행

```gherkin
Given TriggerNode에 interval("100ms")과 cron("* * * * *") 2개 스케줄이 설정되어 있을 때
When Init(ctx)를 호출하면
Then Mock Timer의 SetInterval()과 SetCron()이 각각 1번씩 호출되어야 한다
And 두 스케줄 모두 독립적으로 메시지를 생성해야 한다
```

### AC-NODE-004-10: 스케줄 미설정 거부

```gherkin
Given TriggerNode에 schedules 설정이 빈 배열이거나 누락되어 있을 때
When Init(ctx)를 호출하면
Then ErrTriggerNoSchedules 에러를 반환해야 한다
And 상태가 StateError여야 한다
```

### AC-NODE-004-11: 잘못된 스케줄 타입 거부

```gherkin
Given TriggerNode에 schedules: [{"type": "unknown", "value": "5s"}]가 설정되어 있을 때
When Init(ctx)를 호출하면
Then ErrTriggerInvalidScheduleType 에러를 반환해야 한다
```

### AC-NODE-004-12: 잘못된 스케줄 값 거부

```gherkin
Given TriggerNode에 schedules: [{"type": "interval", "value": "not-a-duration"}]가 설정되어 있을 때
When Init(ctx)를 호출하면
Then ErrTriggerInvalidScheduleValue 에러를 반환해야 한다

Given TriggerNode에 schedules: [{"type": "once", "value": "2020-01-01T00:00:00Z"}]가 설정되어 있을 때 (과거 시각)
When Init(ctx)를 호출하면
Then ErrTriggerInvalidScheduleValue 에러를 반환해야 한다

Given TriggerNode에 schedules: [{"type": "times", "value": ["25:00"]}]가 설정되어 있을 때 (유효하지 않은 시각)
When Init(ctx)를 호출하면
Then ErrTriggerInvalidScheduleValue 에러를 반환해야 한다
```

---

## Module 3: Payload Generation - 페이로드 생성 (v1.3.0 통합 모델로 개정)

### AC-NODE-004-13: 페이로드 - 숫자 스칼라 (config 하위호환 래핑)

```gherkin
Given TriggerNode에 payload: 42가 설정되어 있을 때 (비-map 스칼라)
When 트리거가 발생하면
Then 통합 엔진이 스칼라를 {"value": 42}로 래핑하여 평가해야 한다
And 생성된 메시지의 Payload가 {"value": 42}여야 한다 (value가 숫자 42)
```

### AC-NODE-004-14: 페이로드 - 문자열 스칼라 (리터럴, 래핑)

```gherkin
Given TriggerNode에 payload: "hello"가 설정되어 있을 때 (비-map 스칼라)
When 트리거가 발생하면
Then 통합 엔진이 {"value": "hello"}로 래핑하고 "hello"에 $. / $$가 없으므로 리터럴로 통과해야 한다
And 생성된 메시지의 Payload가 {"value": "hello"}여야 한다
```

### AC-NODE-004-15: 페이로드 - 오브젝트 (map 직접 평가)

```gherkin
Given TriggerNode에 payload: {"temperature": 25.5, "status": "active"}가 설정되어 있을 때 (map)
When 트리거가 발생하면
Then 통합 엔진이 map을 직접 평가해야 한다 (래핑 없음)
And Payload에 temperature: 25.5가 포함되어야 한다 (비문자열 패스스루)
And Payload에 status: "active"가 포함되어야 한다 ($. / $$ 없으므로 리터럴)
```

### AC-NODE-004-16: 페이로드 - 배열 스칼라 (래핑)

```gherkin
Given TriggerNode에 payload: [1, 2, 3]가 설정되어 있을 때 (비-map 배열)
When 트리거가 발생하면
Then 통합 엔진이 {"value": [1, 2, 3]}로 래핑하여 평가해야 한다
And 생성된 메시지의 Payload가 {"value": [1, 2, 3]}여야 한다 (배열은 비문자열 패스스루)
```

### AC-NODE-004-17: 기본 페이로드

```gherkin
Given TriggerNode에 payload 설정이 없을 때
When 트리거가 발생하면
Then 생성된 메시지의 Payload에 "trigger_time" 키가 포함되어야 한다
And trigger_time 값이 ISO 8601 형식의 현재 시각이어야 한다
```

### AC-NODE-004-18: 페이로드 깊은 복사 (중첩 맵 미재귀)

```gherkin
Given TriggerNode에 payload: {"data": {"value": 1}}가 설정되어 있을 때 (중첩 map)
When 트리거가 2번 발생하면
Then 중첩 맵 {"value": 1}은 재귀 평가되지 않고 리터럴 패스스루되어야 한다 (알려진 한계)
And 첫 번째 메시지의 Payload와 두 번째 메시지의 Payload가 깊은 복사로 독립적이어야 한다
And 첫 번째 메시지의 Payload를 변경해도 두 번째 메시지에 영향을 주지 않아야 한다
```

### AC-NODE-004-19: 통째 변수 치환 - 문자열/숫자 (통합 엔진)

```gherkin
Given TriggerNode에 payload: {"time": "$.trigger_time", "count": "$.tick_count"}가 설정되어 있을 때
When 트리거가 발생하면 (tick_count=3)
Then "time" 값 전체가 "$.trigger_time"과 일치하므로 문자열 ISO 8601 시각으로 치환되어야 한다
And "count" 값 전체가 "$.tick_count"와 일치하므로 네이티브 타입 숫자 3으로 치환되어야 한다 (문자열 "3"이 아님)
```

### AC-NODE-004-20: 미지 변수 - 에러 기록 + 키 null (통합 엔진)

```gherkin
Given TriggerNode에 payload: {"bad": "$.unknown_var"}가 설정되어 있을 때
When 트리거가 발생하면
Then 통합 엔진이 미지 변수 $.unknown_var를 감지하여 에러 메시지를 기록해야 한다
And "bad" 키의 값이 null이 되어야 한다
And 메시지의 Metadata에 trigger.error가 포함되어야 한다
And 메시지가 sourceCh로 전송되어야 한다
```

---

## Module 4: Message Metadata - 메시지 메타데이터

### AC-NODE-004-21: 트리거 메타데이터 자동 첨부

```gherkin
Given TriggerNode에 interval("100ms") 스케줄과 이름 "my-trigger"가 설정되어 있을 때
When 트리거가 발생하여 메시지가 생성되면
Then Metadata에 trigger.schedule_type이 "interval"이어야 한다
And Metadata에 trigger.schedule_id가 빈 문자열이 아니어야 한다
And Metadata에 trigger.tick_count가 1 이상이어야 한다
And Metadata에 trigger.trigger_time이 ISO 8601 형식이어야 한다
And Metadata에 trigger.node_name이 "my-trigger"여야 한다
```

### AC-NODE-004-22: tick_count 누적

```gherkin
Given TriggerNode에 interval("50ms") 스케줄이 설정되어 있을 때
When 150ms 동안 트리거가 반복되면
Then 수신된 메시지들의 trigger.tick_count가 순차적으로 증가해야 한다
And 첫 번째 메시지의 tick_count가 1이어야 한다
```

---

## Module 5: Lifecycle Integration - 생명주기 통합

### AC-NODE-004-23: Init 성공 상태 전이

```gherkin
Given NewTriggerNode()로 생성된 TriggerNode가 StateCreated 상태일 때
When Init(ctx)를 호출하면 (유효한 스케줄 + Timer Agent 사용 가능)
Then 상태가 StateRunning이어야 한다
And 에러가 nil이어야 한다
```

### AC-NODE-004-24: Init 실패 상태 전이

```gherkin
Given 스케줄이 비어있는 TriggerNode가 StateCreated 상태일 때
When Init(ctx)를 호출하면
Then 상태가 StateError여야 한다
And ErrTriggerNoSchedules 에러를 반환해야 한다
```

### AC-NODE-004-25: Shutdown 타이머 취소

```gherkin
Given 2개 스케줄이 등록된 TriggerNode가 StateRunning 상태일 때
When Shutdown(ctx)를 호출하면
Then Mock Timer의 Cancel()이 2번 호출되어야 한다
And 상태가 StateStopped여야 한다
```

### AC-NODE-004-26: Shutdown sourceCh 닫기

```gherkin
Given TriggerNode가 StateRunning 상태일 때
When Shutdown(ctx)를 호출하면
Then SourceCh()에서 읽기가 채널 닫힘(close)으로 종료되어야 한다
```

### AC-NODE-004-27: Pause 메시지 생성 중단

```gherkin
Given TriggerNode에 interval("50ms") 스케줄이 설정되고 Running 상태일 때
When Pause()를 호출하면
Then 200ms 대기 후에도 새로운 메시지가 sourceCh에 추가되지 않아야 한다
```

### AC-NODE-004-28: Resume 메시지 생성 재개

```gherkin
Given TriggerNode가 Paused 상태일 때
When Resume()를 호출하면
Then 다음 스케줄 트리거 시점에 메시지가 sourceCh로 전송되어야 한다
```

### AC-NODE-004-29: Init 실패 시 부분 등록 롤백

```gherkin
Given TriggerNode에 3개 스케줄이 설정되어 있고, 3번째 스케줄이 유효하지 않을 때
When Init(ctx)를 호출하면
Then 에러를 반환해야 한다
And 이미 등록된 1, 2번째 타이머가 Cancel()로 취소되어야 한다
```

---

## Module 6: Error Handling - 에러 처리

### AC-NODE-004-30: Sentinel 에러 정의

```gherkin
Given trigger 관련 에러가 정의되어 있을 때
Then ErrTriggerNoSchedules가 정의되어 있어야 한다
And ErrTriggerInvalidScheduleType이 정의되어 있어야 한다
And ErrTriggerInvalidScheduleValue가 정의되어 있어야 한다
And ErrTriggerTimerNotAvailable이 정의되어 있어야 한다
And ErrTriggerPayloadTemplateFailed가 정의되어 있어야 한다
```

### AC-NODE-004-31: errors.Is() 호환성

```gherkin
Given ErrTriggerNoSchedules 에러가 주어졌을 때
Then errors.Is(ErrTriggerNoSchedules, ErrInvalidConfig)가 true를 반환해야 한다

Given ErrTriggerTimerNotAvailable 에러가 주어졌을 때
Then errors.Is(ErrTriggerTimerNotAvailable, ErrNodeNotInitialized)가 true를 반환해야 한다
```

### AC-NODE-004-32: Timer Agent 미사용 가능

```gherkin
Given AgentResolver가 Timer Agent를 반환하지 않는 TriggerNode가 있을 때
When Init(ctx)를 호출하면
Then ErrTriggerTimerNotAvailable 에러를 반환해야 한다
```

### AC-NODE-004-33: sourceCh 버퍼 풀 시 메시지 드롭

```gherkin
Given TriggerNode의 sourceCh 버퍼 크기가 2이고 interval("10ms") 스케줄이 설정되어 있을 때
When sourceCh에서 메시지를 읽지 않고 100ms 대기하면
Then 경고 로그에 "channel full" 또는 "message dropped" 메시지가 기록되어야 한다
And 노드가 정상 동작을 유지해야 한다 (panic 없음)
```

### AC-NODE-004-34: 동시성 안전

```gherkin
Given TriggerNode에 3개 interval 스케줄이 설정되어 있을 때
When Init(ctx) 후 100ms 동안 동시 트리거가 발생하면
Then race condition이 발생하지 않아야 한다 (go test -race 통과)
And 모든 메시지가 올바른 메타데이터를 포함해야 한다
```

---

## Module 7: v1.2.0 신규 기능 - weekly / monthly / per-schedule payload

### AC-NODE-004-35: weekly 다중 요일×시각 조합 cron 등록

```gherkin
Given TriggerNode에 schedules: [{"type": "weekly", "days": ["mon", "wed", "fri"], "times": ["09:00", "18:00"]}]가 설정되어 있을 때
When Init(ctx)를 호출하면
Then Mock Timer의 SetCron()이 6번(3요일 × 2시각) 호출되어야 한다
And 등록된 cron 표현식 집합이 {"0 9 * * 1", "0 18 * * 1", "0 9 * * 3", "0 18 * * 3", "0 9 * * 5", "0 18 * * 5"}와 일치해야 한다

When Mock Timer가 임의의 핸들러를 호출하면
Then SourceCh()에서 메시지를 수신할 수 있어야 한다
And Metadata의 trigger.schedule_type이 "weekly"여야 한다
```

### AC-NODE-004-36: weekly 정수 요일 토큰 및 잘못된 토큰 거부

```gherkin
Given TriggerNode에 schedules: [{"type": "weekly", "days": [0, 6], "times": ["10:00"]}]가 설정되어 있을 때 (0=일요일, 6=토요일)
When Init(ctx)를 호출하면
Then SetCron()이 "0 10 * * 0"와 "0 10 * * 6"로 각각 호출되어야 한다

Given TriggerNode에 schedules: [{"type": "weekly", "days": ["funday"], "times": ["10:00"]}]가 설정되어 있을 때 (잘못된 요일 토큰)
When Init(ctx)를 호출하면
Then ErrTriggerInvalidScheduleValue 에러를 반환해야 한다

Given TriggerNode에 schedules: [{"type": "weekly", "days": ["mon"], "times": ["25:61"]}]가 설정되어 있을 때 (잘못된 시각)
When Init(ctx)를 호출하면
Then ErrTriggerInvalidScheduleValue 에러를 반환해야 한다
```

### AC-NODE-004-37: monthly 특정 일자 cron 등록

```gherkin
Given TriggerNode에 schedules: [{"type": "monthly", "day": 15, "times": ["08:30"]}]가 설정되어 있을 때
When Init(ctx)를 호출하면
Then Mock Timer의 SetCron()이 "30 8 15 * *"로 호출되어야 한다

When Mock Timer가 핸들러를 호출하면
Then SourceCh()에서 메시지 1개를 수신할 수 있어야 한다
And Metadata의 trigger.schedule_type이 "monthly"여야 한다
```

### AC-NODE-004-38: monthly "first" 일자 cron 등록

```gherkin
Given TriggerNode에 schedules: [{"type": "monthly", "day": "first", "times": ["00:00"]}]가 설정되어 있을 때
When Init(ctx)를 호출하면
Then Mock Timer의 SetCron()이 "0 0 1 * *"로 호출되어야 한다 (first = 1일)
```

### AC-NODE-004-39: monthly "last" 월말 emit 게이트

```gherkin
Given TriggerNode에 schedules: [{"type": "monthly", "day": "last", "times": ["23:59"]}]가 설정되어 있을 때
When Init(ctx)를 호출하면
Then Mock Timer의 SetCron()이 매일 cron "59 23 * * *"로 호출되어야 한다 (마지막 날 직접 표현 불가)

Given 주입된 clock의 현재 날짜가 2026-02-28 (2월 마지막 날)일 때
When Mock Timer가 핸들러를 호출하면
Then SourceCh()에서 메시지 1개를 수신할 수 있어야 한다

Given 주입된 clock의 현재 날짜가 2026-02-27 (월말 아님)일 때
When Mock Timer가 핸들러를 호출하면
Then 메시지가 생성되지 않아야 한다 (게이트로 차단)

Given 주입된 clock의 현재 날짜가 2028-02-29 (윤년 2월 마지막 날)일 때
When Mock Timer가 핸들러를 호출하면
Then SourceCh()에서 메시지 1개를 수신할 수 있어야 한다

Given 주입된 clock의 현재 날짜가 2026-04-30 (30일 달의 마지막 날)일 때
When Mock Timer가 핸들러를 호출하면
Then SourceCh()에서 메시지 1개를 수신할 수 있어야 한다

Given 주입된 clock의 현재 날짜가 2026-01-31 (31일 달의 마지막 날)일 때
When Mock Timer가 핸들러를 호출하면
Then SourceCh()에서 메시지 1개를 수신할 수 있어야 한다
```

### AC-NODE-004-40: monthly 존재하지 않는 정수 일자 (표준 cron 미발화)

```gherkin
Given TriggerNode에 schedules: [{"type": "monthly", "day": 31, "times": ["10:00"]}]가 설정되어 있을 때
When Init(ctx)를 호출하면
Then Mock Timer의 SetCron()이 "0 10 31 * *"로 호출되어야 한다
And 이는 31일이 없는 달(2월, 4월 등)에는 발화하지 않는 표준 cron 동작이어야 한다 (의도됨: 항상 월말은 "last" 사용)
```

### AC-NODE-004-41: per-schedule payload 우선 (스케줄 페이로드가 노드 레벨보다 우선)

```gherkin
Given TriggerNode에 노드 레벨 payload: {"src": "node"}가 있고
And schedules: [{"type": "interval", "value": "50ms", "payload": {"src": "schedule"}}]가 설정되어 있을 때
When 트리거가 발생하면
Then 생성된 메시지의 Payload가 {"src": "schedule"}여야 한다 (스케줄 항목 페이로드 우선)
And 노드 레벨 payload는 사용되지 않아야 한다
```

### AC-NODE-004-42: per-schedule payload 폴백 순서 (노드 레벨 → 기본)

```gherkin
Given TriggerNode에 노드 레벨 payload: {"src": "node"}가 있고
And schedules: [{"type": "interval", "value": "50ms"}]가 설정되어 있을 때 (스케줄 항목 페이로드 없음)
When 트리거가 발생하면
Then 생성된 메시지의 Payload가 {"src": "node"}여야 한다 (노드 레벨로 폴백)

Given TriggerNode에 노드 레벨 payload도 없고
And schedules: [{"type": "interval", "value": "50ms"}]가 설정되어 있을 때
When 트리거가 발생하면
Then 생성된 메시지의 Payload에 "trigger_time" 키가 포함되어야 한다 (기본 페이로드로 폴백)
```

### AC-NODE-004-43: per-schedule payload_template 우선 치환

```gherkin
Given schedules: [{"type": "interval", "value": "50ms", "payload_template": {"c": "$.tick_count"}}]가 설정되어 있을 때
When 트리거가 발생하면 (tick_count=2)
Then 생성된 메시지의 Payload에 c가 2여야 한다 (스케줄 항목 템플릿 우선 치환)
```

### AC-NODE-004-44: 하위 호환 - 기존 4종 타입 + 노드 레벨 payload 동작 불변

```gherkin
Given TriggerNode에 schedules: [{"type": "times", "value": ["09:00", "18:00"]}]와 노드 레벨 payload: {"t": 1}만 설정되어 있을 때 (per-schedule payload 미사용, v1.1.0 형식)
When Init(ctx)를 호출하면
Then SetCron()이 "0 9 * * *"와 "0 18 * * *"로 호출되어야 한다 (기존 times 동작 그대로)

When 트리거가 발생하면
Then 생성된 메시지의 Payload가 {"t": 1}여야 한다 (노드 레벨 payload, v1.1.0과 동일)
And interval/cron/once/times 타입의 기존 동작이 변경되지 않아야 한다
```

### AC-NODE-004-45: Web UI - weekly/monthly 위젯 및 스케줄별 페이로드 편집

```gherkin
Given TriggerScheduleEditor에서 스케줄 타입으로 "weekly"를 선택했을 때
Then 요일 토글 버튼(일~토)과 시각 칩 입력 위젯이 표시되어야 한다
And 저장 시 {"type": "weekly", "days": [...], "times": [...]} 형식으로 직렬화되어야 한다

Given 스케줄 타입으로 "monthly"를 선택했을 때
Then 일자 선택(1~31 | first | last)과 시각 칩 위젯이 표시되어야 한다
And day가 29~31 정수일 때 "해당 일이 없는 달에는 발화하지 않음" 안내 힌트가 노출되어야 한다
And 저장 시 {"type": "monthly", "day": ..., "times": [...]} 형식으로 직렬화되어야 한다

Given 임의 스케줄 항목에서 "이 스케줄 전용 페이로드"를 단일 JSON 페이로드 에디터에 입력했을 때 (v1.3.0: payload_mode static/template 토글 없음)
Then 저장 시 해당 스케줄 항목에 payload(오브젝트)가 포함되어야 한다
And 미입력 시 노드 레벨 페이로드로 폴백함이 안내되어야 한다
And 에디터에 $.<var> 변수 목록과 $$ 이스케이프 인라인 힌트가 노출되어야 한다
```

---

## Module 8: v1.3.0 통합 페이로드 엔진 - `$$` 이스케이프 / 혼합 / 통째 타입 / 마이그레이션 / 단일 에디터

### AC-NODE-004-46: `$$` → 리터럴 `$` 이스케이프

```gherkin
Given TriggerNode에 payload: {"price": "$$100", "label": "cost is $$"}가 설정되어 있을 때
When 트리거가 발생하면
Then "price" 값의 $$가 리터럴 $로 치환되어 "$100"이 되어야 한다
And "label" 값의 $$가 리터럴 $로 치환되어 "cost is $"가 되어야 한다
```

### AC-NODE-004-47: 한 문자열 내 리터럴 + 변수 혼합 (interpolation)

```gherkin
Given TriggerNode에 payload: {"cmd": "open", "at": "$.trigger_time"}가 설정되어 있을 때
When 트리거가 발생하면
Then "cmd" 값은 $. / $$가 없으므로 리터럴 "open" 그대로여야 한다
And "at" 값 전체가 "$.trigger_time"과 정확히 일치하므로 문자열 ISO 8601 시각으로 치환되어야 한다

Given TriggerNode에 payload: {"msg": "run at $.trigger_time now"}가 설정되어 있을 때 (리터럴 + 변수 혼합)
When 트리거가 발생하면
Then "msg" 값이 문자 단위 interpolation되어 "run at <ISO시각> now" 형태의 문자열이어야 한다 (변수는 문자열 형태로 삽입)
```

### AC-NODE-004-48: 통째 값 네이티브 타입 치환 (문자열 vs 숫자 구분)

```gherkin
Given TriggerNode에 payload: {"n": "$.tick_count"}가 설정되어 있을 때
When 트리거가 발생하면 (tick_count=7)
Then "n" 값 전체가 "$.tick_count"와 일치하므로 네이티브 타입 숫자 7로 치환되어야 한다 (문자열 "7"이 아님)

Given TriggerNode에 payload: {"n": "count=$.tick_count"}가 설정되어 있을 때 (통째 일치 아님)
When 트리거가 발생하면 (tick_count=7)
Then "n" 값이 interpolation되어 문자열 "count=7"이어야 한다 (숫자 삽입이 아니라 문자열 형태)
```

### AC-NODE-004-49: 하위호환 마이그레이션 - 구 static 맵 동일 출력

```gherkin
Given TriggerNode에 payload: {"cmd": "open", "level": 3}가 설정되어 있을 때 (구 static 맵, $. / $$ 없음)
When 트리거가 발생하면
Then 통합 엔진이 map을 직접 평가하여 구 static 동작과 동일하게 {"cmd": "open", "level": 3}를 생성해야 한다
And "open"은 리터럴 통과, level 3은 비문자열 패스스루여야 한다
```

### AC-NODE-004-50: 하위호환 마이그레이션 - 구 static 스칼라 → {"value": ...} 래핑

```gherkin
Given TriggerNode에 payload: 5가 설정되어 있을 때 (구 static 스칼라)
When 트리거가 발생하면
Then 통합 엔진이 비-map 스칼라를 {"value": 5}로 래핑하여 평가해야 한다
And 생성된 메시지의 Payload가 {"value": 5}여야 한다 (value가 숫자 5)
```

### AC-NODE-004-51: Web UI - 단일 페이로드 에디터 + 힌트 (payload_mode 제거)

```gherkin
Given TriggerNode 속성 패널을 열었을 때 (v1.3.0)
Then payload_mode(static/template) 토글이 표시되지 않아야 한다
And 단일 JSON 오브젝트 페이로드 에디터 하나만 표시되어야 한다
And 에디터에 $.trigger_time, $.tick_count, $.schedule_id, $.trigger_id 변수 목록과 $$ 리터럴-달러 이스케이프 인라인 힌트가 노출되어야 한다

Given 기존 payload_template(map) 형식이 저장된 노드를 로드했을 때 (하위호환)
Then 단일 페이로드 에디터에 해당 값이 로드되어 편집 가능해야 한다

Given 페이로드 에디터를 비운 채 저장했을 때
Then payload와 payload_template가 모두 제거되어 기본 페이로드로 폴백해야 한다
```

---

## 품질 게이트

### Definition of Done

- [ ] 모든 수락 기준(AC-NODE-004-01 ~ 51) 테스트 통과
- [ ] v1.2.0 신규 기능(AC-NODE-004-35 ~ 45): weekly/monthly/per-schedule payload 테스트 통과
- [ ] v1.3.0 통합 페이로드 엔진(AC-NODE-004-13 ~ 20 개정 + AC-NODE-004-46 ~ 51): 통째 타입 치환/interpolation/`$$` 이스케이프/미지 변수 에러+null/config 하위호환 래핑/단일 에디터 테스트 통과
- [ ] v1.3.0 하위호환 회귀: 구 static 맵 동일 출력(AC-49), 구 static 스칼라 `{"value":...}` 래핑(AC-50) 확인
- [ ] monthly "last" 월말 게이트: 2월(28/29), 30일 달, 31일 달 경계 테스트 통과 (주입 clock)
- [ ] 하위 호환 회귀(AC-NODE-004-44): 기존 4종 타입 + 노드 레벨 payload 동작 불변 확인
- [ ] `go test ./internal/node/...` 전체 통과
- [ ] `go test -race ./internal/node/...` 경쟁 상태 없음
- [ ] `go vet ./internal/node/...` 경고 없음
- [ ] 테스트 커버리지 85% 이상 (`go test -cover`)
- [ ] GoDoc 주석 작성 완료 (모든 exported 타입/함수/메서드)
- [ ] 성능 목표 검증:
  - [ ] 메시지 생성 오버헤드: < 100us (페이로드 복사 포함)
  - [ ] sourceCh 전송: < 10us (non-blocking)
  - [ ] Init() 전체: < 10ms (Timer Agent resolve + 스케줄 등록)
- [ ] goroutine 누수 없음 (Init/Shutdown 사이클 후 goroutine 수 원복 확인)
- [ ] sourceCh 정상 닫힘 확인 (Shutdown 후 읽기 종료)
- [ ] 페이로드 깊은 복사 확인 (메시지 간 데이터 격리; 통합 엔진 평가 결과 및 비문자열 패스스루 포함)

### 검증 도구

| 도구 | 용도 |
|------|------|
| `go test` | 단위/통합 테스트 실행 |
| `go test -race` | 경쟁 상태 검출 |
| `go test -cover` | 커버리지 측정 |
| `go test -bench` | 벤치마크 테스트 |
| `go vet` | 정적 분석 |
| `golangci-lint` | 코드 품질 린팅 |
| `runtime.NumGoroutine()` | goroutine 누수 검증 |
