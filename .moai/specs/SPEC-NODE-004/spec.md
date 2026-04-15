---
id: SPEC-NODE-004
version: "1.1.0"
status: implemented
created: "2026-04-15"
updated: "2026-04-16"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-04-15 | 1.0.0 | 초기 SPEC 작성 (draft) |
| 2026-04-16 | 1.1.0 | 구현 완료: (1) Timer Agent 주입 경로 확정 — `AgentResolver`는 user agent manager 소속 에이전트만 해석하므로, 시스템 Timer Agent는 `node.WithTimer(system.Timer)` NodeOption 으로 엔진 구성 시점에 직접 주입한다. `resolveTimer()` 스텁 로직을 `n.timer` 기반 통과 + config fallback 패턴으로 재작성. (2) 웹 UI — `trigger` 노드 스키마 신규 등록, `TriggerScheduleEditor` 전용 컴포넌트 추가 (interval/cron/once/times 4종 전용 위젯 + 프리셋 + 인라인 검증), `payload_mode` UI 전용 가상 필드로 payload/payload_template 토글, `source_ch_size` 고급 설정 섹션 분리 |

# SPEC-NODE-004: Trigger Node - 스케줄 기반 데이터 생성 소스 노드

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼에 스케줄 기반 데이터 생성 기능을 제공하는 Trigger Node를 정의한다. Trigger Node는 지정된 시간 간격, cron 표현식, 특정 시각, 또는 매일 반복 시각에 따라 설정된 페이로드(숫자, 문자열, 오브젝트 등)를 자동 생성하여 플로우에 전송하는 SourceNode이다.

본 SPEC은 다음을 포함한다:

- **TriggerNode** (`trigger.go`): BaseNode + SourceNode 인터페이스 구현, 스케줄 기반 메시지 생성
- **Schedule Configuration**: 다중 스케줄 동시 지원 (interval, cron, once, times)
- **Payload Generation**: 정적 값 또는 트리거 컨텍스트 기반 템플릿 페이로드 생성
- **Timer Agent Integration**: Timer System Agent를 AgentResolver 패턴으로 참조

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/node/`
- **Tier**: internal (비공개 패키지)
- **의존 패키지**:
  - `pkg/lifecycle/` (SPEC-LIFE-001): `BaseLifecycle`, `State` 임베딩
  - `pkg/flow/` (SPEC-FLOW-001): `NodeDef`, `Port` 데이터 구조
  - `pkg/message/` (SPEC-MSG-001): `Message`, `Payload`, `Metadata` 인터페이스
  - `internal/node/` (SPEC-NODE-001): `BaseNode`, `SourceNode`, `Node` 인터페이스, `AgentResolver`
  - `internal/agent/system/` (SPEC-TIMER-001): `Timer` 인터페이스 (AgentResolver를 통해 간접 참조)
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성 모델**: sourceCh 채널 기반 메시지 생성, 내부 상태 `sync.Mutex` 보호

### 1.3 설계 원칙

- **SourceNode 패턴 준수**: Engine이 `SourceCh()` 채널에서 메시지를 읽어 출력 와이어로 전달
- **AgentResolver 패턴**: Timer Agent에 직접 의존하지 않고, AgentResolver 인터페이스로 간접 참조
- **다중 스케줄 동시 실행**: 하나의 Trigger Node에서 여러 스케줄을 병렬 실행
- **유연한 페이로드**: 정적 값(숫자, 문자열, 맵, 배열, 불리언)과 템플릿 기반 동적 값 모두 지원
- **Graceful Shutdown**: 모든 타이머 취소 후 sourceCh 정상 닫기

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- TriggerNode 구조체 (BaseNode 임베딩 + SourceNode 인터페이스)
- 4종 스케줄 타입: interval, cron, once, times
- 다중 스케줄 동시 설정 및 실행
- 정적 및 템플릿 기반 페이로드 생성
- 메시지 메타데이터 자동 첨부
- Timer Agent 통합 (AgentResolver 패턴)
- sourceCh 채널 기반 메시지 출력
- 생명주기 통합 (Init, Pause, Resume, Shutdown)
- Node Registry 등록 ("trigger" 타입)
- Sentinel 에러 정의

**OUT OF SCOPE (별도 SPEC)**:
- Timer Agent 자체 구현 (SPEC-TIMER-001: `internal/agent/system/`)
- Engine의 SourceNode 처리 루프 (SPEC-ENGINE-001)
- Flow YAML 파싱 (SPEC-FLOW-001)
- Message 데이터 구조 (SPEC-MSG-001)

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-NODE-001 | 의존 | `BaseNode`, `SourceNode`, `Node` 인터페이스, `AgentResolver`, Node Registry |
| SPEC-LIFE-001 | 의존 | `BaseLifecycle`, `State` 임베딩 |
| SPEC-FLOW-001 | 의존 | `NodeDef`, `Port` 데이터 구조 |
| SPEC-MSG-001 | 의존 | `Message`, `Payload`, `Metadata` 인터페이스 |
| SPEC-ENGINE-001 | 소비자 | Engine이 `SourceCh()`에서 메시지를 읽어 출력 와이어로 전달 |
| SPEC-TIMER-001 | 협력 | Timer Agent를 AgentResolver로 참조하여 스케줄 등록 |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: Timer System Agent가 시스템 시작 시 자동 활성화되어 있으며, AgentResolver를 통해 조회 가능하다
- A2: Timer Agent의 `SetInterval()`, `SetCron()`, `SetTimeout()` 메서드는 goroutine-safe하다
- A3: Timer Agent의 핸들러 함수(`TimerHandler`)는 독립 goroutine에서 실행된다 (타이머 간 간섭 없음)
- A4: Engine이 SourceNode 인터페이스를 확인하고 `SourceCh()`에서 메시지를 읽는 goroutine을 관리한다
- A5: sourceCh 채널이 닫히면 Engine의 SourceNode 읽기 goroutine이 정상 종료된다
- A6: `_agent_resolver` 키로 config에 주입되는 AgentResolver가 Timer 인터페이스를 구현하는 Agent를 반환한다

### 2.2 도메인 가정

- A7: 하나의 Trigger Node에 1개 이상의 스케줄이 반드시 설정되어야 한다
- A8: 스케줄 미설정 시 Init에서 에러를 반환한다
- A9: "times" 스케줄 타입의 시각은 "HH:MM" 형식이며, 매일 해당 시각에 반복 실행된다
- A10: 페이로드 미설정 시 기본 페이로드 `{"trigger_time": <현재시각>}`을 생성한다
- A11: sourceCh 버퍼가 가득 찬 경우(backpressure), 메시지를 드롭하고 경고 로그를 기록한다
- A12: 템플릿 페이로드의 변수는 `$.trigger_time`, `$.tick_count`, `$.schedule_id`, `$.trigger_id` 4종을 지원한다

---

## 3. Requirements (요구사항)

### Module 1: TriggerNode Core - 트리거 노드 핵심 (P0)

#### REQ-NODE-004-01-01 (Ubiquitous) TriggerNode 구조체

시스템은 **항상** `BaseNode`를 임베딩하고 `SourceNode` 인터페이스를 구현하는 `TriggerNode` 구조체를 제공해야 한다:

- `sourceCh chan message.Message`: 메시지 출력 채널 (Engine이 읽기)
- `SourceCh() <-chan message.Message`: SourceNode 인터페이스 메서드
- 노드 타입: `"trigger"`
- 카테고리: `"input"`

#### REQ-NODE-004-01-02 (Ubiquitous) NewTriggerNode 팩토리 함수

시스템은 **항상** `NewTriggerNode(def flow.NodeDef, opts ...NodeOption) (Node, error)` 팩토리 함수를 제공해야 한다:

- `flow.NodeDef`에서 스케줄 설정과 페이로드 설정을 추출
- `config["_agent_resolver"]`에서 `AgentResolver`를 추출
- sourceCh 채널 생성 (버퍼 크기 설정 가능, 기본 64)

#### REQ-NODE-004-01-03 (Ubiquitous) Node Registry 등록

시스템은 **항상** Node Registry의 `RegisterDefaults()`에서 `"trigger"` 타입으로 `NewTriggerNode` 팩토리 함수를 등록해야 한다.

#### REQ-NODE-004-01-04 (Event-Driven) Process 메서드

**WHEN** `TriggerNode.Process(ctx, msg)` 호출 시, **THEN** SourceNode는 외부 입력을 받지 않으므로 입력 메시지를 무시하고 빈 결과를 반환해야 한다.

---

### Module 2: Schedule Configuration - 스케줄 설정 (P0)

#### REQ-NODE-004-02-01 (Ubiquitous) 다중 스케줄 설정

시스템은 **항상** 하나의 TriggerNode에 1개 이상의 스케줄을 설정할 수 있어야 한다. 설정 키는 `"schedules"`이며, 각 스케줄은 `type`과 `value`를 포함한다.

#### REQ-NODE-004-02-02 (State-Driven) interval 스케줄 타입

**IF** 스케줄 타입이 `"interval"`이면, **THEN** `value` 필드를 `time.Duration` 문자열로 파싱하고 Timer Agent의 `SetInterval()`로 주기적 타이머를 등록해야 한다.

예시: `{"type": "interval", "value": "5s"}`

#### REQ-NODE-004-02-03 (State-Driven) cron 스케줄 타입

**IF** 스케줄 타입이 `"cron"`이면, **THEN** `value` 필드를 cron 표현식으로 Timer Agent의 `SetCron()`으로 크론 타이머를 등록해야 한다.

예시: `{"type": "cron", "value": "0 */5 * * * *"}`

#### REQ-NODE-004-02-04 (State-Driven) once 스케줄 타입

**IF** 스케줄 타입이 `"once"`이면, **THEN** `value` 필드를 ISO 8601 타임스탬프로 파싱하여, 현재 시각과의 차이를 계산하고 Timer Agent의 `SetTimeout()`으로 1회성 타이머를 등록해야 한다.

예시: `{"type": "once", "value": "2026-04-15T10:00:00Z"}`

#### REQ-NODE-004-02-05 (State-Driven) times 스케줄 타입

**IF** 스케줄 타입이 `"times"`이면, **THEN** `value` 필드를 `"HH:MM"` 형식의 시각 배열로 파싱하여, 각 시각에 대해 cron 표현식(`MM HH * * *`)으로 변환하고 Timer Agent의 `SetCron()`으로 매일 반복 타이머를 등록해야 한다.

예시: `{"type": "times", "value": ["09:00", "12:00", "18:00"]}`

#### REQ-NODE-004-02-06 (Unwanted) 스케줄 미설정 거부

시스템은 `schedules` 설정이 비어있거나 누락된 경우 Init 시 `ErrTriggerNoSchedules` 에러를 반환**해야 한다**.

#### REQ-NODE-004-02-07 (Unwanted) 잘못된 스케줄 타입 거부

시스템은 지원하지 않는 스케줄 타입이 설정된 경우 Init 시 `ErrTriggerInvalidScheduleType` 에러를 반환**해야 한다**.

#### REQ-NODE-004-02-08 (Unwanted) 잘못된 스케줄 값 거부

시스템은 스케줄 값이 유효하지 않은 경우(파싱 실패, 과거 시각 등) Init 시 `ErrTriggerInvalidScheduleValue` 에러를 반환**해야 한다**.

---

### Module 3: Payload Generation - 페이로드 생성 (P0)

#### REQ-NODE-004-03-01 (State-Driven) 정적 페이로드

**IF** `"payload"` 설정 키에 값이 지정되어 있으면, **THEN** 트리거 시마다 해당 값을 메시지의 Payload로 사용해야 한다. 지원 타입:

- 숫자 (int, float64)
- 문자열 (string)
- 불리언 (bool)
- 오브젝트 (map[string]any)
- 배열 ([]any)

#### REQ-NODE-004-03-02 (State-Driven) 기본 페이로드

**IF** `"payload"` 설정이 없거나 nil이면, **THEN** 기본 페이로드 `{"trigger_time": <현재 ISO 8601 시각>}`을 생성해야 한다.

#### REQ-NODE-004-03-03 (Optional) 템플릿 페이로드

**가능하면** `"payload_template"` 설정 키가 존재하는 경우, 트리거 컨텍스트 변수를 치환하여 동적 페이로드를 생성해야 한다. 지원 변수:

- `$.trigger_time`: 트리거 발생 ISO 8601 시각
- `$.tick_count`: 해당 스케줄의 누적 트리거 횟수
- `$.schedule_id`: 스케줄 식별자
- `$.trigger_id`: 트리거 노드 이름

---

### Module 4: Message Metadata - 메시지 메타데이터 (P0)

#### REQ-NODE-004-04-01 (Ubiquitous) 트리거 메타데이터 자동 첨부

시스템은 **항상** 생성된 메시지의 Metadata에 다음 정보를 첨부해야 한다:

- `trigger.schedule_type`: 스케줄 타입 (`"interval"` | `"cron"` | `"once"` | `"times"`)
- `trigger.schedule_id`: 스케줄 고유 식별자 (TimerID)
- `trigger.tick_count`: 해당 스케줄의 누적 트리거 횟수
- `trigger.trigger_time`: 트리거 발생 시각 (ISO 8601)
- `trigger.node_name`: Trigger 노드의 이름 (`Node.Name()`)

---

### Module 5: Lifecycle Integration - 생명주기 통합 (P0)

#### REQ-NODE-004-05-01 (Event-Driven) Init 초기화

**WHEN** `TriggerNode.Init(ctx)` 호출 시, **THEN** 다음을 수행해야 한다:

1. 상태를 `StateInitializing`으로 전이
2. `config["_agent_resolver"]`에서 AgentResolver 추출 및 Timer Agent resolve
3. Timer Agent가 `Timer` 인터페이스를 구현하는지 확인
4. 모든 스케줄을 파싱하고 Timer Agent에 등록
5. 각 스케줄의 핸들러에서 페이로드를 생성하여 sourceCh로 전송
6. 초기화 성공 시 상태를 `StateRunning`으로 전이
7. 초기화 실패 시 등록된 타이머를 모두 취소하고 상태를 `StateError`로 전이

#### REQ-NODE-004-05-02 (Event-Driven) Shutdown 종료

**WHEN** `TriggerNode.Shutdown(ctx)` 호출 시, **THEN** 다음을 수행해야 한다:

1. 상태를 `StateStopping`으로 전이
2. 등록된 모든 타이머를 Timer Agent에서 취소 (`Cancel()`)
3. sourceCh 채널 닫기
4. 상태를 `StateStopped`으로 전이

#### REQ-NODE-004-05-03 (Event-Driven) Pause 일시정지

**WHEN** Trigger Node가 Pause 상태로 전이하면, **THEN** 새로운 메시지 생성을 중단해야 한다. 기존 sourceCh에 버퍼링된 메시지는 Engine이 계속 읽을 수 있다.

#### REQ-NODE-004-05-04 (Event-Driven) Resume 재개

**WHEN** Trigger Node가 Resume 상태로 전이하면, **THEN** 메시지 생성을 재개해야 한다.

---

### Module 6: Error Handling - 에러 처리 (P0)

#### REQ-NODE-004-06-01 (Ubiquitous) Sentinel 에러 정의

시스템은 **항상** 다음 sentinel 에러를 정의해야 한다:

- `ErrTriggerNoSchedules`: 스케줄 미설정
- `ErrTriggerInvalidScheduleType`: 지원하지 않는 스케줄 타입
- `ErrTriggerInvalidScheduleValue`: 유효하지 않은 스케줄 값
- `ErrTriggerTimerNotAvailable`: Timer Agent를 찾을 수 없음
- `ErrTriggerPayloadTemplateFailed`: 템플릿 페이로드 생성 실패

#### REQ-NODE-004-06-02 (Event-Driven) Timer Agent 미사용 가능

**WHEN** AgentResolver를 통해 Timer Agent를 찾을 수 없으면, **THEN** Init 시 `ErrTriggerTimerNotAvailable` 에러를 반환해야 한다.

#### REQ-NODE-004-06-03 (Event-Driven) 채널 만료 시 메시지 드롭

**WHEN** sourceCh 채널 버퍼가 가득 차서 메시지를 전송할 수 없으면, **THEN** 해당 메시지를 드롭하고 경고 로그를 기록해야 한다.

#### REQ-NODE-004-06-04 (Event-Driven) 템플릿 평가 실패

**WHEN** 템플릿 페이로드 생성 중 에러가 발생하면, **THEN** 경고 로그를 기록하고 에러 메타데이터(`trigger.error`)를 포함한 빈 페이로드 메시지를 전송해야 한다.

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
internal/node/
  trigger.go              # TriggerNode 구현
  trigger_test.go         # 단위 테스트
```

### 4.2 Flow YAML 설정 예시

```yaml
nodes:
  - name: "my-trigger"
    type: "trigger"
    config:
      schedules:
        - type: "interval"
          value: "5s"
        - type: "cron"
          value: "0 */5 * * * *"
        - type: "once"
          value: "2026-04-15T10:00:00Z"
        - type: "times"
          value: ["09:00", "12:00", "18:00"]
      payload:
        temperature: 25.5
        status: "active"
      channel_buffer: 64
    outputs:
      - "out"
```

### 4.3 타입 시그니처

```go
package node

import (
    "context"
    "sync"
    "time"

    "xflow/pkg/flow"
    "xflow/pkg/lifecycle"
    "xflow/pkg/message"
    "xflow/internal/agent/system"
)

// ── Schedule Types ──

type TriggerScheduleType string

const (
    TriggerScheduleInterval TriggerScheduleType = "interval"
    TriggerScheduleCron     TriggerScheduleType = "cron"
    TriggerScheduleOnce     TriggerScheduleType = "once"
    TriggerScheduleTimes    TriggerScheduleType = "times"
)

// TriggerSchedule 은 단일 스케줄 설정을 나타낸다.
type TriggerSchedule struct {
    Type  TriggerScheduleType // 스케줄 타입
    Value any                 // interval: "5s", cron: "0 */5 * * * *", once: "2026-...", times: ["09:00",...]
}

// triggerTimerEntry 는 등록된 타이머의 런타임 정보를 추적한다.
type triggerTimerEntry struct {
    timerID      system.TimerID
    scheduleType TriggerScheduleType
    scheduleID   string      // 사용자 식별용 (예: "interval-0", "cron-1")
    tickCount    int64       // 누적 트리거 횟수 (atomic)
}

// ── TriggerNode ──

type TriggerNode struct {
    *BaseNode
    sourceCh      chan message.Message
    schedules     []TriggerSchedule
    payload       any                    // 정적 페이로드 (nil이면 기본 페이로드)
    payloadTmpl   map[string]any         // 템플릿 페이로드 (nil이면 미사용)
    channelBuffer int                    // sourceCh 버퍼 크기 (기본 64)
    timer         system.Timer           // Timer Agent 인터페이스
    resolver      AgentResolver          // Agent 조회용
    entries       []triggerTimerEntry    // 등록된 타이머 목록
    paused        bool                   // Pause 상태 플래그
    mu            sync.Mutex             // 내부 상태 보호
}

// 컴파일 타임 인터페이스 체크
var _ Node = (*TriggerNode)(nil)
var _ SourceNode = (*TriggerNode)(nil)

func NewTriggerNode(def flow.NodeDef, opts ...NodeOption) (Node, error)

func (n *TriggerNode) Init(ctx context.Context) error
func (n *TriggerNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error)
func (n *TriggerNode) Shutdown(ctx context.Context) error
func (n *TriggerNode) SourceCh() <-chan message.Message

// ── Errors ──

var (
    ErrTriggerNoSchedules          = fmt.Errorf("trigger: %w: no schedules configured", ErrInvalidConfig)
    ErrTriggerInvalidScheduleType  = fmt.Errorf("trigger: %w: unsupported schedule type", ErrInvalidConfig)
    ErrTriggerInvalidScheduleValue = fmt.Errorf("trigger: %w: invalid schedule value", ErrInvalidConfig)
    ErrTriggerTimerNotAvailable    = fmt.Errorf("trigger: %w: timer agent not available", ErrNodeNotInitialized)
    ErrTriggerPayloadTemplateFailed = fmt.Errorf("trigger: %w: payload template evaluation failed", ErrInvalidConfig)
)

// WithTimer 는 TriggerNode 에 Timer 인터페이스를 직접 주입하는 옵션을 반환한다.
// 시스템 Timer Agent 는 AgentResolver(user agent manager)를 통해 접근할 수 없으므로
// 엔진 구성 시점에 이 옵션으로 직접 주입한다.
func WithTimer(timer system.Timer) NodeOption
```

#### 4.3.1 Timer Agent 주입 경로 (v1.1.0)

Trigger 노드는 시스템 Timer Agent 를 필요로 하지만, `AgentResolver` 인터페이스는
`agent.Manager` 소속 user agent 만 해석할 수 있다. 시스템 에이전트(Timer, Logger, Store, Event, File)는
`SystemAgentManager` 가 별도로 관리하므로 resolver 경로로는 접근 불가능하다.

이를 해결하기 위해 **`NodeOption` 기반 직접 주입** 경로를 채택한다:

```
cmd/xflowd/main.go
    sysMgr := system.NewSystemAgentManager()
    sysMgr.Initialize(cfg)
    sysMgr.Start(ctx)
    │
    ▼ sysMgr.Timer() returns *TimerAgent implementing system.Timer
    │
    eng := engine.NewEngine(
        engine.WithNodeOptions(
            node.WithAgentResolver(agentResolver),  // user agents
            node.WithTimer(sysMgr.Timer()),          // system timer (NEW)
        ),
    )
    │
    ▼ Engine.DeployFlow 시 각 노드 생성 시점에 옵션 전달
    │
    NewTriggerNode(def, opts...)
        base := NewBaseNode(def, opts...)
            for _, opt := range opts { opt(b) }    // b.config["_timer_agent"] = timer
        if a, ok := base.config["_timer_agent"]; ok {
            n.timer = a.(system.Timer)              // 노드 필드에 복사 (Configure 보호)
        }
    │
    ▼ Configure(def.Config) 호출 시 b.config 덮어쓰기 되지만 n.timer 는 보존
    │
    Init(ctx)
        resolveTimer()
            if n.timer != nil { return nil }        // 즉시 통과 (production path)
            if a, ok := n.config["_timer_agent"]; ok { ... }  // test path (기존 호환)
            return ErrTriggerTimerNotAvailable
```

**주입 우선순위**:
1. `NodeOption.WithTimer()` → factory 시점에 `n.timer` 설정 (production)
2. `config["_timer_agent"]` → Configure 시점에 `n.timer` 설정 (테스트, backward compat)
3. 둘 다 없음 → `ErrTriggerTimerNotAvailable`

`AgentResolver` 기반 해석 경로는 설계 단계에서 고려되었으나 시스템 에이전트와 user 에이전트의
관리 주체가 다르므로 폐기되었다.

### 4.4 TriggerNode 동작 다이어그램

```
                 Timer Agent
                 (System Agent)
                     │
    ┌────────────────┼─────────────────────┐
    │  TriggerNode   │                     │
    │                │                     │
    │  Schedule 1 ───┤ SetInterval("5s")   │
    │  Schedule 2 ───┤ SetCron("0 */5 *")  │
    │  Schedule 3 ───┤ SetTimeout(delta)   │
    │  Schedule 4 ───┤ SetCron("00 09 *")  │  ← times -> cron 변환
    │                │                     │
    │  TimerHandler ←┘                     │
    │       │                              │
    │       ▼                              │
    │  [Payload 생성]                       │
    │  [Metadata 첨부]                      │
    │       │                              │
    │       ▼                              │
    │  sourceCh ──────────────────────────►│──► Engine ──► 출력 와이어
    │                                      │
    └──────────────────────────────────────┘
```

### 4.5 메시지 생성 흐름

```
Timer Agent 트리거
    │
    ├── 1. TimerHandler 호출 (독립 goroutine)
    │       ├── TimerTrigger 수신 {TimerID, TriggerAt, TickCount, ScheduleID}
    │       └── Paused 상태 확인 → paused=true이면 리턴 (메시지 미생성)
    │
    ├── 2. 페이로드 결정
    │       ├── payload 설정 있음 → 정적 페이로드 복사
    │       ├── payload_template 설정 있음 → 변수 치환
    │       └── 둘 다 없음 → 기본 페이로드 {"trigger_time": now}
    │
    ├── 3. Message 생성
    │       ├── Payload: 결정된 페이로드
    │       └── Metadata:
    │             ├── trigger.schedule_type
    │             ├── trigger.schedule_id
    │             ├── trigger.tick_count
    │             ├── trigger.trigger_time
    │             └── trigger.node_name
    │
    └── 4. sourceCh 전송
            ├── 성공 → 다음 트리거 대기
            └── 버퍼 풀 → 메시지 드롭 + 경고 로그
```

### 4.6 Web UI - Trigger 노드 설정 에디터 (v1.1.0)

트리거 노드의 복잡한 다중 스케줄 + 페이로드 설정을 사용자가 직관적으로 편집할 수 있도록
전용 프론트엔드 컴포넌트를 제공한다.

#### 파일 구조

```
web/src/
  config/
    nodeSchemas.ts                        # trigger 노드 스키마 등록
  components/property/
    TriggerScheduleEditor.tsx             # 스케줄 전용 에디터 (신규)
    FormField.tsx                         # trigger_schedules 타입 분기
    DynamicForm.tsx                       # advanced 필드 섹션 분리
    PropertyPanel.tsx                     # payload_mode 가상 필드 유도/정리
  types/
    node.ts                               # ConfigField.type 확장
```

#### 스키마 정의

`trigger` 노드 스키마 (nodeSchemas.ts):

| 필드 | 타입 | 설명 |
|------|------|------|
| `schedules` | `trigger_schedules` (신규) | 필수. 다중 스케줄 편집 전용 위젯 |
| `payload_mode` | `select` | `none`/`static`/`template` (UI 전용 가상 필드) |
| `payload` | `object` | `payload_mode=static` 일 때만 표시 |
| `payload_template` | `key_value_map` | `payload_mode=template` 일 때만 표시 |
| `source_ch_size` | `number` | **고급 설정** (기본 64, 접힘 섹션) |

#### TriggerScheduleEditor 컴포넌트

스케줄 타입별 전용 위젯:

| 타입 | 위젯 | 프리셋 |
|------|------|--------|
| `interval` | duration 문자열 + 칩 | 1s/5s/30s/1m/5m/15m/1h |
| `cron` | cron 표현식 + 칩 | 매분/5분마다/매시/매일 자정/매일 9시/평일 9시 |
| `once` | `<input type="datetime-local">` | 로컬 시간 선택 → RFC3339 변환 |
| `times` | `<input type="time">` + 칩 목록 | HH:MM 정렬 추가/삭제 |

실시간 인라인 검증:
- `interval`: duration 정규식 `^\d+(ns|us|µs|ms|s|m|h)$`
- `cron`: 5/6 필드 개수 체크
- `once`: RFC3339 파싱 + 미래 시각 확인
- `times`: `HH:MM` 정규식 + 중복 제거

저장 형식은 백엔드와 호환되는 배열:
```json
[
  {"type": "interval", "value": "5s"},
  {"type": "cron",     "value": "0 */5 * * * *"},
  {"type": "once",     "value": "2026-04-15T10:00:00Z"},
  {"type": "times",    "value": ["09:00", "12:00"]}
]
```

#### payload_mode 가상 필드 처리

`payload_mode` 는 UI 전용 토글로, 백엔드 YAML 에 저장되지 않는다.

**로드 시** (PropertyPanel useEffect):
- `payload_template` 이 비어있지 않음 → `mode = 'template'`
- `payload` 가 있음 → `mode = 'static'`
- 둘 다 없음 → `mode = 'none'`

**저장 시** (handleApply):
- `mode = 'static'` → `delete payload_template`
- `mode = 'template'` → `delete payload`
- `mode = 'none'` → `delete payload; delete payload_template`
- 모든 경우에 `delete payload_mode` (스토어에 저장 안 함)

**변경 감지** (hasChanges):
- `payload_mode` 필드는 비교에서 제외되어 로드 직후에는 "변경 없음" 상태 유지

#### advanced 섹션 인프라

`ConfigField` 에 `advanced?: boolean` 필드를 추가하고, `DynamicForm` 이 이를 분리하여
기본 접힘 상태의 "고급 설정" collapsible 섹션으로 렌더링한다. trigger 뿐 아니라
다른 노드(향후 deduplicate, filter 등)에서도 재사용 가능한 공통 인프라이다.

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 파일 | 우선순위 |
|------------|------|------|---------|
| REQ-NODE-004-01-01 ~ 01-04 | TriggerNode Core | internal/node/trigger.go | P0 |
| REQ-NODE-004-02-01 ~ 02-08 | Schedule Configuration | internal/node/trigger.go | P0 |
| REQ-NODE-004-03-01 ~ 03-03 | Payload Generation | internal/node/trigger.go | P0 |
| REQ-NODE-004-04-01 | Message Metadata | internal/node/trigger.go | P0 |
| REQ-NODE-004-05-01 ~ 05-04 | Lifecycle Integration | internal/node/trigger.go | P0 |
| REQ-NODE-004-06-01 ~ 06-04 | Error Handling | internal/node/trigger.go | P0 |
| REQ-NODE-004-07-01 (v1.1.0) | Timer Agent 주입 (WithTimer NodeOption) | internal/node/trigger.go, cmd/xflowd/main.go | P0 |
| REQ-NODE-004-08-01 (v1.1.0) | Web UI - 전용 스케줄 에디터 | web/src/components/property/TriggerScheduleEditor.tsx | P1 |
| REQ-NODE-004-08-02 (v1.1.0) | Web UI - 스키마 등록 | web/src/config/nodeSchemas.ts | P1 |
| REQ-NODE-004-08-03 (v1.1.0) | Web UI - advanced 섹션 인프라 | web/src/components/property/DynamicForm.tsx | P1 |
| REQ-NODE-004-08-04 (v1.1.0) | Web UI - payload_mode 가상 필드 | web/src/components/property/PropertyPanel.tsx | P1 |
