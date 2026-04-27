---
id: SPEC-LIFE-001
version: "1.0.0"
status: implemented
created: "2026-02-12"
updated: "2026-02-12"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-12 | 1.0.0 | 초기 SPEC 작성 |
| 2026-02-14 | 1.0.1 | 구현 완료 (TDD, 100% 커버리지) |

---

# SPEC-LIFE-001: Lifecycle System - 공통 생명주기 인터페이스, 상태 머신, 기반 구현체

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 모든 구성 요소(Flow Engine, Node, Agent, Script Engine, Plugin)가 공유하는 공통 생명주기 관리 시스템을 정의한다. 본 SPEC은 제네릭한 `Lifecycle` 인터페이스, 공통 `State` 타입 및 유효 상태 전이 맵, 컴포넌트가 임베딩하여 사용할 수 있는 기반 구현체(`BaseLifecycle`), 헬스 체크/자동 복구 인터페이스, 런타임 설정 변경 패턴, 상태 변경 이벤트 알림 메커니즘을 다룬다.

본 패키지는 `pkg/lifecycle/`에 위치하며, 플러그인 개발자를 포함한 외부 사용자에게 공개되는 Tier 1 패키지이다.

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `pkg/lifecycle/`
- **의존성**: 표준 라이브러리만 사용 (`sync`, `fmt`, `time`, `errors`)
- **테스트 프레임워크**: Go 표준 `testing` 패키지
- **Tier**: Tier 1 - 공개 기반 계층 (SPEC-MSG-001, SPEC-FLOW-001과 동일 계층)

### 1.3 설계 원칙

- **인터페이스 우선**: 모든 공개 API는 인터페이스로 정의하며, 기반 구현체(`BaseLifecycle`)는 exported struct로 제공하여 임베딩 가능
- **제네릭 상태 모델**: `State` 타입은 Flow 전용이 아닌, 모든 컴포넌트가 공유하는 범용 상태 열거형
- **임베딩 패턴**: `BaseLifecycle` 구조체를 컴포넌트가 임베딩(embedding)하여 공통 상태 전이 로직을 재사용
- **동시성 안전**: 모든 상태 전이는 `sync.Mutex` 기반으로 thread-safe 보장
- **콜백 기반 확장**: 상태 변경 이벤트를 콜백으로 알려 관찰성 시스템과의 통합 지원
- **정책 기반 복구**: 자동 복구 전략을 정책 타입으로 정의하여 컴포넌트별 커스터마이징 허용
- **최소 의존성**: 표준 라이브러리만 의존하여 순환 의존 방지 및 컴파일 속도 보장

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- `Lifecycle` 인터페이스 정의 (Init, Start, Pause, Resume, Stop, State)
- `Configurable` 인터페이스 정의 (Configure, GetConfig) - 생명주기 관점의 런타임 설정 변경 계약
- 공통 `State` 타입 및 상수 (Created, Initializing, Running, Paused, Stopping, Stopped, Error)
- 유효 상태 전이 맵 (`ValidTransitions`)
- `BaseLifecycle` 기반 구현체 (임베딩용, 상태 머신 + Mutex 내장)
- `StateChangeEvent` 및 `StateChangeCallback` 타입 정의
- `HealthChecker` 인터페이스 및 `RecoveryPolicy` 타입 정의
- 상태 전이 유효성 검증 유틸리티 함수

**OUT OF SCOPE (별도 SPEC)**:
- `FlowState` (SPEC-FLOW-001: `pkg/flow/state.go` - Flow 전용 확장 상태)
- 실제 컴포넌트 구현 (internal/engine/, internal/node/, internal/agent/, internal/script/, internal/plugin/)
- 관찰성 시스템 통합 구현 (SPEC-OBS-001: `internal/observe/`)
- 설정 파일 로딩 및 Viper 통합 (SPEC-CFG-001: `internal/config/`)
- REST API 핸들러 (별도 SPEC: `internal/api/`)
- Agent Manager의 참조 카운팅 및 공유 관리 (별도 SPEC)

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-MSG-001 | 동일 Tier | 메시지 시스템 (Tier 1, `pkg/message/`) |
| SPEC-FLOW-001 | 소비자 | `FlowState`는 `State`를 참조하거나 매핑하여 사용 가능 |
| SPEC-OBS-001 | 소비자 | `StateChangeCallback`을 통해 상태 변경 이벤트를 관찰성 시스템에 전달 |
| SPEC-CFG-001 | 소비자 | `Configurable` 인터페이스 구현체에서 Config 변경 콜백과 연동 |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: `State` 타입은 `string` 기반이며, 각 컴포넌트는 본 SPEC에서 정의한 7개 공통 상태만 사용한다
- A2: `sync.Mutex`는 상태 전이의 원자성을 보장하기에 충분하다 (고빈도 전이 없음)
- A3: `BaseLifecycle`은 exported struct로 제공하여 컴포넌트가 Go 임베딩으로 재사용한다
- A4: 상태 전이 콜백은 동기적으로 호출되며, 콜백 내에서 장시간 블로킹 작업은 수행하지 않는다
- A5: `Configurable` 인터페이스의 `Configure()` 호출은 Running 또는 Paused 상태에서만 허용된다
- A6: `HealthChecker` 인터페이스는 선택적이며, 헬스 체크가 필요한 컴포넌트만 구현한다
- A7: 자동 복구(Recovery)의 실제 실행은 각 컴포넌트의 런타임 관리자가 담당하며, 본 SPEC은 정책 타입만 정의한다
- A8: `pkg/lifecycle/`은 표준 라이브러리 외의 외부 의존성을 갖지 않는다

### 2.2 도메인 가정

- A9: 모든 XFlow 구성 요소는 동일한 7개 상태(Created, Initializing, Running, Paused, Stopping, Stopped, Error)를 공유한다
- A10: SPEC-FLOW-001의 `FlowState`는 Flow 전용 확장 상태(Stored, Loaded 등)를 추가 정의하지만, 공통 상태와의 매핑은 internal/engine/의 책임이다
- A11: 상태 전이 이벤트는 관찰성 시스템과의 통합에 필수적이지만, `pkg/lifecycle/`은 콜백 타입만 정의하고 실제 연동은 구현체에 위임한다
- A12: Hot Configuration 변경은 `Configurable.Configure()` 호출로 시작되며, 변경 가능/불변 키 구분은 각 컴포넌트 구현체의 책임이다

---

## 3. Requirements (요구사항)

### Module 1: Core State - 상태 타입 및 전이 규칙 (P0)

#### REQ-LIFE-001-01-01 (Ubiquitous) State 타입 정의

시스템은 **항상** `State` 타입(`string` 기반)과 다음 7개 상수를 제공해야 한다:

- `StateCreated` - 생성됨 (초기 상태, 리소스 미할당)
- `StateInitializing` - 초기화 중 (리소스 할당 및 설정 적용 진행)
- `StateRunning` - 실행 중 (정상 동작, 데이터 처리 가능)
- `StatePaused` - 일시정지 (데이터 처리 중단, 상태/연결 유지)
- `StateStopping` - 중지 중 (정상 종료 진행, 리소스 해제 중)
- `StateStopped` - 중지됨 (리소스 해제 완료)
- `StateError` - 에러 (오류 발생, 복구 대기 또는 중지 대기)

#### REQ-LIFE-001-01-02 (Ubiquitous) State.String() 메서드

시스템은 **항상** `State.String() string` 메서드를 제공하여 사람이 읽을 수 있는 상태명을 반환해야 한다.

#### REQ-LIFE-001-01-03 (Ubiquitous) IsValid() 메서드

시스템은 **항상** `State.IsValid() bool` 메서드를 제공하여 해당 State 값이 7개 유효 상태 중 하나인지 검증할 수 있어야 한다.

#### REQ-LIFE-001-01-04 (Ubiquitous) 유효 상태 전이 맵

시스템은 **항상** `ValidTransitions` 맵을 제공하여 각 State에서 전이 가능한 상태 목록을 정의해야 한다:

| 현재 상태 | 전이 가능 상태 |
|-----------|--------------|
| Created | Initializing |
| Initializing | Running, Error |
| Running | Paused, Stopping, Error |
| Paused | Running, Stopping, Error |
| Stopping | Stopped, Error |
| Stopped | Created (재초기화) |
| Error | Stopping, Stopped (강제), Created (재초기화) |

#### REQ-LIFE-001-01-05 (Ubiquitous) IsValidTransition() 함수

시스템은 **항상** `IsValidTransition(from, to State) bool` 함수를 제공하여 상태 전이 가능 여부를 확인할 수 있어야 한다.

#### REQ-LIFE-001-01-06 (Ubiquitous) ParseState() 함수

시스템은 **항상** `ParseState(s string) (State, error)` 함수를 제공하여 문자열에서 State를 파싱할 수 있어야 한다. 유효하지 않은 문자열은 `ErrInvalidState` 에러를 반환한다.

#### REQ-LIFE-001-01-07 (Unwanted) 유효하지 않은 상태 전이 거부

시스템은 `ValidTransitions` 맵에 정의되지 않은 상태 전이를 **시도하지 않아야 한다**. 유효하지 않은 전이 시도 시 `ErrInvalidStateTransition` 에러를 반환해야 한다.

---

### Module 2: Lifecycle Interface - 생명주기 인터페이스 (P0)

#### REQ-LIFE-001-02-01 (Ubiquitous) Lifecycle 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Lifecycle` 인터페이스를 제공해야 한다:

- `Init(ctx context.Context) error` - 초기화 수행 (Created -> Initializing -> Running)
- `Start(ctx context.Context) error` - 실행 시작 (Initializing -> Running, 또는 Stopped -> Created -> ... -> Running)
- `Pause(ctx context.Context) error` - 일시정지 (Running -> Paused)
- `Resume(ctx context.Context) error` - 재개 (Paused -> Running)
- `Stop(ctx context.Context) error` - 정상 중지 (Running/Paused -> Stopping -> Stopped)
- `State() State` - 현재 상태 반환

#### REQ-LIFE-001-02-02 (Event-Driven) Init 호출 시 상태 전이

**WHEN** `Lifecycle.Init(ctx)` 호출 시 현재 상태가 `Created`이면, **THEN** 상태를 `Initializing`으로 전이하고 초기화 로직을 수행한 뒤, 성공 시 `Running`으로, 실패 시 `Error`로 전이해야 한다.

#### REQ-LIFE-001-02-03 (Event-Driven) Stop 호출 시 Graceful Shutdown

**WHEN** `Lifecycle.Stop(ctx)` 호출 시 현재 상태가 `Running` 또는 `Paused`이면, **THEN** 상태를 `Stopping`으로 전이하고, 리소스 해제를 수행한 뒤, 완료 시 `Stopped`로 전이해야 한다.

#### REQ-LIFE-001-02-04 (Event-Driven) Pause 호출 시 데이터 처리 중단

**WHEN** `Lifecycle.Pause(ctx)` 호출 시 현재 상태가 `Running`이면, **THEN** 상태를 `Paused`로 전이하고, 데이터 처리를 중단하되 상태와 연결은 유지해야 한다.

#### REQ-LIFE-001-02-05 (Event-Driven) Resume 호출 시 데이터 처리 재개

**WHEN** `Lifecycle.Resume(ctx)` 호출 시 현재 상태가 `Paused`이면, **THEN** 상태를 `Running`으로 전이하고, 중단된 데이터 처리를 재개해야 한다.

#### REQ-LIFE-001-02-06 (Unwanted) 잘못된 상태에서의 전이 호출 거부

시스템은 현재 상태에서 허용되지 않는 Lifecycle 메서드 호출을 **수행하지 않아야 한다**. 예를 들어, `Paused` 상태에서 `Pause()` 재호출 시 `ErrInvalidStateTransition`을 반환해야 한다.

---

### Module 3: Configurable Interface - 런타임 설정 변경 계약 (P0)

#### REQ-LIFE-001-03-01 (Ubiquitous) Configurable 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Configurable` 인터페이스를 제공해야 한다:

- `Configure(ctx context.Context, cfg map[string]any) error` - 런타임 설정 변경 적용
- `GetConfig() map[string]any` - 현재 설정값 반환 (방어적 복사)

#### REQ-LIFE-001-03-02 (State-Driven) Running 또는 Paused 상태에서만 Configure 허용

**IF** 컴포넌트의 현재 상태가 `Running` 또는 `Paused`일 때, **THEN** `Configure()` 호출을 허용해야 한다. 다른 상태에서는 `ErrInvalidStateForConfigure` 에러를 반환해야 한다.

#### REQ-LIFE-001-03-03 (Event-Driven) Configure 실패 시 원래 설정 유지

**WHEN** `Configure(ctx, cfg)` 호출이 실패하면, **THEN** 컴포넌트의 설정은 호출 이전 상태를 유지해야 한다 (롤백 보장).

#### REQ-LIFE-001-03-04 (Ubiquitous) GetConfig 방어적 복사

시스템은 **항상** `GetConfig()` 호출 시 내부 설정 맵의 복사본을 반환해야 한다. 반환된 맵의 수정이 내부 상태에 영향을 주지 않아야 한다.

---

### Module 4: BaseLifecycle - 기반 구현체 (P0)

#### REQ-LIFE-001-04-01 (Ubiquitous) BaseLifecycle 구조체 정의

시스템은 **항상** 다음 기능을 제공하는 `BaseLifecycle` exported 구조체를 제공해야 한다:

- 내부 `State` 필드 (초기값: `StateCreated`)
- `sync.Mutex` 기반 동시성 안전한 상태 관리
- 상태 전이 유효성 검증 내장
- 상태 변경 콜백 등록/호출 메커니즘

#### REQ-LIFE-001-04-02 (Ubiquitous) NewBaseLifecycle() 생성자

시스템은 **항상** `NewBaseLifecycle(opts ...BaseOption) *BaseLifecycle` 생성자를 제공하여, Options Pattern으로 초기 설정(콜백, 이름 등)을 구성할 수 있어야 한다.

#### REQ-LIFE-001-04-03 (Ubiquitous) TransitionTo() 메서드

시스템은 **항상** `BaseLifecycle.TransitionTo(newState State) error` 메서드를 제공하여:

- `sync.Mutex` Lock 획득
- 현재 상태에서 `newState`로의 전이 유효성 검증
- 유효하면 상태 변경 및 등록된 콜백 호출
- 유효하지 않으면 `ErrInvalidStateTransition` 반환

#### REQ-LIFE-001-04-04 (Ubiquitous) CurrentState() 메서드

시스템은 **항상** `BaseLifecycle.CurrentState() State` 메서드를 제공하여, `sync.Mutex` 보호 하에 현재 상태를 안전하게 반환해야 한다.

#### REQ-LIFE-001-04-05 (Ubiquitous) OnStateChange() 콜백 등록

시스템은 **항상** `BaseLifecycle.OnStateChange(cb StateChangeCallback) UnsubscribeFunc` 메서드를 제공하여:

- 상태 변경 콜백을 등록하고
- 등록 해제를 위한 `UnsubscribeFunc`을 반환해야 한다

#### REQ-LIFE-001-04-06 (Event-Driven) 상태 전이 시 콜백 호출

**WHEN** `TransitionTo()`가 성공적으로 상태를 변경하면, **THEN** 등록된 모든 `StateChangeCallback`을 `StateChangeEvent`와 함께 순서대로 호출해야 한다.

#### REQ-LIFE-001-04-07 (Ubiquitous) ComponentName() 메서드

시스템은 **항상** `BaseLifecycle.ComponentName() string` 메서드를 제공하여, `NewBaseLifecycle` 생성 시 지정한 컴포넌트 이름을 반환해야 한다. 이 이름은 `StateChangeEvent`에 포함된다.

---

### Module 5: State Change Event - 상태 변경 이벤트 (P1)

#### REQ-LIFE-001-05-01 (Ubiquitous) StateChangeEvent 구조체

시스템은 **항상** 다음 필드를 포함하는 `StateChangeEvent` 구조체를 제공해야 한다:

- `Component string` - 상태 변경이 발생한 컴포넌트 이름 (dot-notation, 예: `agent.mqtt.client1`)
- `From State` - 이전 상태
- `To State` - 새 상태
- `Timestamp time.Time` - 상태 변경 시각
- `Error error` - Error 상태 전이 시 원인 에러 (nil 가능)

#### REQ-LIFE-001-05-02 (Ubiquitous) StateChangeCallback 타입

시스템은 **항상** `StateChangeCallback func(event StateChangeEvent)` 타입을 제공해야 한다.

#### REQ-LIFE-001-05-03 (Ubiquitous) UnsubscribeFunc 타입

시스템은 **항상** `UnsubscribeFunc func()` 타입을 제공하여, 콜백 등록 해제에 사용할 수 있어야 한다.

#### REQ-LIFE-001-05-04 (Unwanted) 콜백 패닉 전파 방지

시스템은 `StateChangeCallback` 내부에서 발생하는 panic이 상태 전이 로직으로 전파**되지 않아야 한다**. 콜백 panic은 recover하여 로그 기록 후 다음 콜백 실행을 계속해야 한다.

---

### Module 6: Health Check & Recovery - 헬스 체크 및 복구 정책 (P1)

#### REQ-LIFE-001-06-01 (Ubiquitous) HealthChecker 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `HealthChecker` 인터페이스를 제공해야 한다:

- `HealthCheck(ctx context.Context) HealthStatus` - 현재 건강 상태 확인

#### REQ-LIFE-001-06-02 (Ubiquitous) HealthStatus 구조체

시스템은 **항상** 다음 필드를 포함하는 `HealthStatus` 구조체를 제공해야 한다:

- `Healthy bool` - 건강 여부
- `Message string` - 상태 메시지 (건강하지 않은 경우 원인 설명)
- `LastChecked time.Time` - 마지막 체크 시각
- `Details map[string]any` - 추가 상세 정보 (선택적)

#### REQ-LIFE-001-06-03 (Ubiquitous) RecoveryPolicy 구조체

시스템은 **항상** 다음 필드를 포함하는 `RecoveryPolicy` 구조체를 제공해야 한다:

- `MaxRetries int` - 최대 재시도 횟수 (0이면 자동 복구 비활성화)
- `InitialBackoff time.Duration` - 초기 백오프 간격
- `MaxBackoff time.Duration` - 최대 백오프 간격
- `BackoffMultiplier float64` - 백오프 증가 배수 (지수 백오프)
- `OnMaxRetriesExceeded RecoveryAction` - 최대 재시도 초과 시 동작

#### REQ-LIFE-001-06-04 (Ubiquitous) RecoveryAction 타입

시스템은 **항상** `RecoveryAction` 타입(string 기반)과 다음 상수를 제공해야 한다:

- `RecoveryStop` - 최대 재시도 초과 시 컴포넌트 중지
- `RecoveryKeepError` - 최대 재시도 초과 시 Error 상태 유지 (수동 개입 대기)

#### REQ-LIFE-001-06-05 (Ubiquitous) DefaultRecoveryPolicy() 함수

시스템은 **항상** `DefaultRecoveryPolicy() RecoveryPolicy` 함수를 제공하여, 합리적인 기본값(MaxRetries: 3, InitialBackoff: 1s, MaxBackoff: 30s, Multiplier: 2.0, Action: RecoveryStop)을 반환해야 한다.

#### REQ-LIFE-001-06-06 (Optional) HealthChecker 구현 선택적

**가능하면** 외부 연결이 필요한 컴포넌트(Agent 등)는 `HealthChecker` 인터페이스를 구현하여 헬스 체크를 제공해야 한다. 헬스 체크가 불필요한 컴포넌트(순수 데이터 변환 Node 등)는 구현하지 않을 수 있다.

---

### Module 7: Error Types - 에러 타입 (P0)

#### REQ-LIFE-001-07-01 (Ubiquitous) 표준 에러 변수

시스템은 **항상** 다음 에러 변수를 제공해야 한다:

| 에러 변수 | 용도 |
|-----------|------|
| `ErrInvalidState` | 알 수 없는 State 문자열 파싱 시 |
| `ErrInvalidStateTransition` | 유효하지 않은 상태 전이 시도 시 |
| `ErrInvalidStateForConfigure` | Configure 허용 상태가 아닌 경우 |
| `ErrAlreadyInitialized` | 이미 초기화된 컴포넌트에 재초기화 시도 시 |
| `ErrNotRunning` | Running 상태가 아닌 컴포넌트에 Pause 시도 시 |
| `ErrNotPaused` | Paused 상태가 아닌 컴포넌트에 Resume 시도 시 |

#### REQ-LIFE-001-07-02 (Ubiquitous) 에러 래핑 지원

시스템은 **항상** 모든 에러 변수가 `errors.Is()` 및 `errors.As()`와 호환되도록 sentinel error 패턴을 사용해야 한다.

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
pkg/lifecycle/
  state.go             # State 타입, 상수, ValidTransitions, IsValidTransition(), ParseState()
  lifecycle.go         # Lifecycle 인터페이스 정의
  configurable.go      # Configurable 인터페이스 정의
  base.go              # BaseLifecycle 구조체, NewBaseLifecycle(), TransitionTo(), OnStateChange()
  event.go             # StateChangeEvent, StateChangeCallback, UnsubscribeFunc
  health.go            # HealthChecker 인터페이스, HealthStatus, RecoveryPolicy, RecoveryAction
  errors.go            # 에러 변수 정의
  options.go           # BaseOption, WithName(), WithOnStateChange() 등

  state_test.go        # State 파싱, 유효성 검증, 전이 맵 테스트
  lifecycle_test.go    # Lifecycle 인터페이스 계약 테스트 (mock 기반)
  base_test.go         # BaseLifecycle 통합 테스트 (전이, 콜백, 동시성)
  health_test.go       # HealthChecker, RecoveryPolicy 테스트
  configurable_test.go # Configurable 인터페이스 계약 테스트
```

### 4.2 타입 시그니처

```go
// State 타입
type State string

const (
    StateCreated      State = "created"
    StateInitializing State = "initializing"
    StateRunning      State = "running"
    StatePaused       State = "paused"
    StateStopping     State = "stopping"
    StateStopped      State = "stopped"
    StateError        State = "error"
)

// 인터페이스
type Lifecycle interface {
    Init(ctx context.Context) error
    Start(ctx context.Context) error
    Pause(ctx context.Context) error
    Resume(ctx context.Context) error
    Stop(ctx context.Context) error
    State() State
}

type Configurable interface {
    Configure(ctx context.Context, cfg map[string]any) error
    GetConfig() map[string]any
}

type HealthChecker interface {
    HealthCheck(ctx context.Context) HealthStatus
}

// 이벤트
type StateChangeEvent struct {
    Component string
    From      State
    To        State
    Timestamp time.Time
    Error     error
}

type StateChangeCallback func(event StateChangeEvent)
type UnsubscribeFunc func()

// 기반 구현체
type BaseLifecycle struct { /* unexported fields */ }

func NewBaseLifecycle(opts ...BaseOption) *BaseLifecycle
func (b *BaseLifecycle) TransitionTo(newState State) error
func (b *BaseLifecycle) CurrentState() State
func (b *BaseLifecycle) OnStateChange(cb StateChangeCallback) UnsubscribeFunc
func (b *BaseLifecycle) ComponentName() string

// 헬스 체크
type HealthStatus struct {
    Healthy     bool
    Message     string
    LastChecked time.Time
    Details     map[string]any
}

type RecoveryPolicy struct {
    MaxRetries           int
    InitialBackoff       time.Duration
    MaxBackoff           time.Duration
    BackoffMultiplier    float64
    OnMaxRetriesExceeded RecoveryAction
}

type RecoveryAction string

const (
    RecoveryStop      RecoveryAction = "stop"
    RecoveryKeepError RecoveryAction = "keep_error"
)

func DefaultRecoveryPolicy() RecoveryPolicy

// Options Pattern
type BaseOption func(*BaseLifecycle)

func WithName(name string) BaseOption
func WithOnStateChange(cb StateChangeCallback) BaseOption

// 유틸리티
func IsValidTransition(from, to State) bool
func ParseState(s string) (State, error)

// 에러
var (
    ErrInvalidState             = errors.New("lifecycle: invalid state")
    ErrInvalidStateTransition   = errors.New("lifecycle: invalid state transition")
    ErrInvalidStateForConfigure = errors.New("lifecycle: invalid state for configure")
    ErrAlreadyInitialized       = errors.New("lifecycle: already initialized")
    ErrNotRunning               = errors.New("lifecycle: not running")
    ErrNotPaused                = errors.New("lifecycle: not paused")
)
```

### 4.3 SPEC-FLOW-001과의 관계

SPEC-FLOW-001은 Flow 전용의 `FlowState`(Stored, Loaded, Initializing, Running, Paused, Stopping, Stopped, Error)를 정의한다. 본 SPEC의 `State`는 범용 공통 상태이며, `FlowState`는 Flow 도메인 전용 확장이다.

관계 설계:
- `pkg/lifecycle/State`는 범용 상태 7개를 정의
- `pkg/flow/FlowState`는 Flow 전용 상태 8개를 독립적으로 정의 (Stored, Loaded 포함)
- `internal/engine/`에서 `FlowState` ↔ `State` 매핑이 필요한 경우, 변환 함수를 해당 엔진 패키지에서 구현
- `pkg/lifecycle/`과 `pkg/flow/`는 서로 직접 의존하지 않음 (Tier 1 동일 계층, 상호 의존 없음)

### 4.4 SPEC-CFG-001과의 관계

SPEC-CFG-001은 Viper 기반의 설정 로딩/감시 시스템을 `internal/config/`에서 다룬다. 본 SPEC의 `Configurable` 인터페이스는 생명주기 관점에서 컴포넌트가 런타임 설정 변경을 수용하는 계약을 정의한다.

관계 설계:
- `pkg/lifecycle/Configurable`은 컴포넌트가 구현하는 인터페이스 (Configure, GetConfig)
- `internal/config/`는 설정 소스 관리 및 변경 감지를 수행
- `internal/config/`의 `OnChange` 콜백이 발화되면, 해당 컴포넌트의 `Configurable.Configure()`를 호출하는 것은 런타임 관리자(internal/engine/ 등)의 책임

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 파일 | 우선순위 |
|------------|------|------|---------|
| REQ-LIFE-001-01-01 ~ 01-07 | Core State | state.go | P0 |
| REQ-LIFE-001-02-01 ~ 02-06 | Lifecycle Interface | lifecycle.go | P0 |
| REQ-LIFE-001-03-01 ~ 03-04 | Configurable Interface | configurable.go | P0 |
| REQ-LIFE-001-04-01 ~ 04-07 | BaseLifecycle | base.go, options.go | P0 |
| REQ-LIFE-001-05-01 ~ 05-04 | State Change Event | event.go | P1 |
| REQ-LIFE-001-06-01 ~ 06-06 | Health Check & Recovery | health.go | P1 |
| REQ-LIFE-001-07-01 ~ 07-02 | Error Types | errors.go | P0 |
