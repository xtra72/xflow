# pkg/lifecycle - 공통 생명주기 관리 시스템

## 개요

`pkg/lifecycle`은 xflow 컴포넌트(Flow, Node, Agent, Plugin 등)의 공통 생명주기를 관리하는 패키지이다. 7개 상태와 유효 전이 규칙을 정의하고, 임베딩 가능한 `BaseLifecycle` 기본 구현체를 제공한다. 표준 라이브러리만 사용하며 외부 의존성이 없다.

## 주요 타입 및 인터페이스

### State

`string` 기반 상태 타입으로 7개 상수를 정의한다.

| 상수 | 값 | 설명 |
|------|------|------|
| `StateCreated` | `"created"` | 생성 직후 초기 상태 |
| `StateInitializing` | `"initializing"` | 초기화 진행 중 |
| `StateRunning` | `"running"` | 정상 실행 중 |
| `StatePaused` | `"paused"` | 일시정지 상태 |
| `StateStopping` | `"stopping"` | 정지 진행 중 |
| `StateStopped` | `"stopped"` | 완전 정지 |
| `StateError` | `"error"` | 오류 발생 |

`String()`, `IsValid()`, `ParseState()` 메서드를 제공한다.

### Lifecycle 인터페이스

```go
type Lifecycle interface {
    Init(ctx context.Context) error
    Start(ctx context.Context) error
    Pause(ctx context.Context) error
    Resume(ctx context.Context) error
    Stop(ctx context.Context) error
    State() State
}
```

### Configurable 인터페이스

```go
type Configurable interface {
    Configure(ctx context.Context, cfg map[string]any) error
    GetConfig() map[string]any
}
```

`Created` 또는 `Stopped` 상태에서만 설정 변경이 가능하다. `GetConfig()`는 방어적 복사본을 반환한다.

### BaseLifecycle 구조체

`Lifecycle` 인터페이스의 공통 상태 관리를 제공하는 임베딩용 기본 구현체이다.

- `sync.Mutex` 기반 동시성 안전 상태 전이
- 상태 변경 콜백 메커니즘 (Observer 패턴)
- 콜백 패닉 복구 (다른 콜백에 영향 없음)
- 콜백은 락 해제 후 호출하여 데드락 방지

### StateChangeEvent / StateChangeCallback

```go
type StateChangeEvent struct {
    Component string
    From      State
    To        State
    Timestamp time.Time
    Error     error
}

type StateChangeCallback func(event StateChangeEvent)
type UnsubscribeFunc func()
```

### HealthChecker 인터페이스

```go
type HealthChecker interface {
    HealthCheck(ctx context.Context) HealthStatus
}
```

`HealthStatus` 구조체와 `RecoveryPolicy`(지수 백오프 기반 자동 복구 전략)를 제공한다.

### Options 패턴

- `WithName(name string)`: 컴포넌트 이름 설정
- `WithOnStateChange(cb StateChangeCallback)`: 상태 변경 콜백 등록

## 상태 전이 다이어그램

```
Created ──> Initializing ──> Running ⇄ Paused
                │                │         │
                │                ▼         ▼
                │            Stopping ──> Stopped ──> Created
                │                │
                └──> Error ◄─────┘
                       │
                       ├──> Stopping
                       ├──> Stopped
                       └──> Created
```

## 유효 상태 전이 표

| 출발 상태 | 전이 가능 대상 |
|-----------|---------------|
| `Created` | `Initializing` |
| `Initializing` | `Running`, `Error` |
| `Running` | `Paused`, `Stopping`, `Error` |
| `Paused` | `Running`, `Stopping`, `Error` |
| `Stopping` | `Stopped`, `Error` |
| `Stopped` | `Created` |
| `Error` | `Stopping`, `Stopped`, `Created` |

## 사용 예시

```go
package main

import (
    "context"
    "fmt"

    "github.com/xtra/xflow/pkg/lifecycle"
)

// MyComponent 는 BaseLifecycle을 임베딩하여 Lifecycle을 구현한다.
type MyComponent struct {
    *lifecycle.BaseLifecycle
}

func NewMyComponent(name string) *MyComponent {
    return &MyComponent{
        BaseLifecycle: lifecycle.NewBaseLifecycle(
            lifecycle.WithName(name),
            lifecycle.WithOnStateChange(func(e lifecycle.StateChangeEvent) {
                fmt.Printf("[%s] %s -> %s\n", e.Component, e.From, e.To)
            }),
        ),
    }
}

func (c *MyComponent) Init(ctx context.Context) error {
    if err := c.TransitionTo(lifecycle.StateInitializing); err != nil {
        return err
    }
    // 초기화 로직 수행
    return c.TransitionTo(lifecycle.StateRunning)
}

func (c *MyComponent) Start(ctx context.Context) error {
    // Running 상태에서 시작 로직 수행
    return nil
}

func (c *MyComponent) Pause(ctx context.Context) error {
    return c.TransitionTo(lifecycle.StatePaused)
}

func (c *MyComponent) Resume(ctx context.Context) error {
    return c.TransitionTo(lifecycle.StateRunning)
}

func (c *MyComponent) Stop(ctx context.Context) error {
    if err := c.TransitionTo(lifecycle.StateStopping); err != nil {
        return err
    }
    // 정리 로직 수행
    return c.TransitionTo(lifecycle.StateStopped)
}

func (c *MyComponent) State() lifecycle.State {
    return c.CurrentState()
}
```

## 에러 타입

| 에러 변수 | 설명 |
|-----------|------|
| `ErrInvalidState` | 유효하지 않은 상태 값 |
| `ErrInvalidStateTransition` | 허용되지 않은 상태 전이 |
| `ErrInvalidStateForConfigure` | 설정 변경 불가 상태에서 Configure 호출 |
| `ErrAlreadyInitialized` | 이미 초기화된 컴포넌트에 Init 재호출 |
| `ErrNotRunning` | 실행 중이 아닌 상태에서 실행 전제 연산 시도 |
| `ErrNotPaused` | 일시정지 상태가 아닌데 Resume 호출 |

모든 에러는 `errors.Is()`와 호환된다.

## 파일 구성

| 파일 | 설명 |
|------|------|
| `errors.go` | 6개 sentinel 에러 정의 |
| `errors_test.go` | errors.Is() 호환성 테스트 |
| `state.go` | State 타입, 7개 상수, 전이 맵, ParseState |
| `state_test.go` | 상태 연산 테이블 기반 테스트 |
| `event.go` | StateChangeEvent, StateChangeCallback, UnsubscribeFunc |
| `lifecycle.go` | Lifecycle 인터페이스 정의 |
| `configurable.go` | Configurable 인터페이스 정의 |
| `options.go` | BaseOption, WithName, WithOnStateChange |
| `base.go` | BaseLifecycle 구현체 (상태 머신, 콜백, 패닉 복구) |
| `base_test.go` | 동시성, 콜백, 인터페이스 구현 테스트 |
| `health.go` | HealthChecker 인터페이스, HealthStatus, RecoveryPolicy |
| `health_test.go` | 헬스 모듈 테스트 |

## 테스트

```bash
go test -v -race -cover ./pkg/lifecycle/...
```

- 테스트: 132개 전체 통과 (47개 top-level + 85개 sub-test)
- 커버리지: 100.0%
- Race Detector: 이상 없음
- Go Vet: 이상 없음

## 의존성

표준 라이브러리만 사용한다 (외부 의존성 없음).

- `sync`: 뮤텍스 기반 동시성 제어
- `context`: 컨텍스트 전파
- `time`: 타임스탬프, Duration
- `errors`: 에러 생성
- `fmt`: 포맷 출력
- `os`: 표준 에러 출력

## SPEC 문서

- SPEC ID: SPEC-LIFE-001
- 상태: 구현 완료
