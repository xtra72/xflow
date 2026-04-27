# xferr - XFlow 에러/폐기/상태 메시지 시스템

`pkg/xferr` 패키지는 XFlow 플랫폼의 에러(Error), 폐기(Dead Letter), 상태(Status) 메시지 데이터 타입과 라우팅 인프라를 제공한다. 에러 분류 체계, 폐기 정책, 수신자 기반 라우팅을 통해 플랫폼 전반의 에러 처리를 구조화한다.

**SPEC**: SPEC-ERR-001

## 아키텍처 개요

```
    에러/폐기/상태 메시지 시스템 계층 구조

    +----------------------------------------------------+
    |                ErrorRouter                          |
    |  에러/폐기/상태 메시지 라우팅 (scope 기반 매칭)       |
    |  Fan-out 전달, 수신자 미매칭 시 DiscardPolicy 적용   |
    +----------------------------------------------------+
            |                    |                |
    +-------v--------+  +-------v--------+  +----v-----------+
    | ErrorReceiver   |  | DeadLetterRecv |  | StatusReceiver |
    | Catch 노드 등   |  | DL 노드 등     |  | Status 노드 등  |
    +----------------+  +----------------+  +----------------+
            ^                    ^                ^
            |                    |                |
    +-------+---------+  +------+----------+  +--+---------------+
    |  ErrorMessage    |  | DeadLetterMsg   |  |  StatusEvent     |
    |  원본 + 에러정보  |  | 원본 + 폐기사유  |  |  상태 전이 이벤트 |
    +-----------------+  +-----------------+  +------------------+
            |                    |
    +-------+--------------------+--------+
    |           Classification             |
    |  ErrorSeverity / ErrorCategory       |
    |  DropReason / ComponentType          |
    +-------------------------------------+
```

**핵심 구성 요소**:

1. **ErrorMessage**: 원본 메시지 + 에러 컨텍스트 래핑 (심각도, 범주, 스택 컨텍스트)
2. **DeadLetterMessage**: 폐기된 메시지 + 폐기 사유 래핑 (TTL 만료, 백프레셔 등)
3. **StatusEvent**: 컴포넌트 상태 전이 이벤트 (Flow, Node, Agent, ScriptEngine, Plugin)
4. **Classification**: ErrorSeverity(3단계), ErrorCategory(6종), DropReason(6종), ComponentType(5종)
5. **DiscardPolicy**: 수신자 미연결 시 동작 정책 (LogAndDiscard, SilentDiscard, PanicOnCritical)
6. **ErrorRouter**: scope 기반 수신자 등록, 와일드카드 매칭, Fan-out 라우팅
7. **AlertThreshold**: 비율/개수 기반 알림 임계값 설정
8. **Errors**: 9개 sentinel 에러

## 핵심 인터페이스

### ErrorMessage 인터페이스 (7개 메서드)

```go
type ErrorMessage interface {
    OriginalMessage() message.Message  // 에러가 발생한 원본 메시지
    Error() error                      // 발생한 Go error
    SourceNodeID() string              // 에러 발생 노드 ID
    ErrorTimestamp() time.Time         // 에러 발생 시각
    Severity() ErrorSeverity           // 에러 심각도
    Category() ErrorCategory           // 에러 범주
    StackContext() string              // 스택 컨텍스트 (디버깅용)
}
```

### DeadLetterMessage 인터페이스 (6개 메서드)

```go
type DeadLetterMessage interface {
    OriginalMessage() message.Message  // 폐기된 원본 메시지
    Reason() DropReason                // 폐기 사유
    SourceNodeID() string              // 폐기 발생 노드 ID
    SourceWireID() string              // 폐기 발생 Wire ID
    DropTimestamp() time.Time          // 폐기 시각
    Context() map[string]string        // 추가 컨텍스트 (TTL 값, 재시도 횟수 등)
}
```

### StatusEvent 인터페이스 (6개 메서드)

```go
type StatusEvent interface {
    ComponentType() ComponentType      // 컴포넌트 타입
    ComponentID() string               // 컴포넌트 ID
    PreviousState() lifecycle.State    // 이전 상태
    NewState() lifecycle.State         // 새 상태
    Timestamp() time.Time              // 상태 전이 시각
    Info() map[string]string           // 추가 정보
}
```

### ErrorRouter 인터페이스

```go
type ErrorRouter interface {
    RouteError(ctx context.Context, errMsg ErrorMessage) error
    RouteDeadLetter(ctx context.Context, dlMsg DeadLetterMessage) error
    RouteStatus(ctx context.Context, evt StatusEvent) error
    RegisterErrorReceiver(scope string, receiver ErrorReceiver)
    RegisterDeadLetterReceiver(receiver DeadLetterReceiver)
    RegisterStatusReceiver(receiver StatusReceiver)
}
```

### DiscardPolicy 인터페이스

```go
type DiscardPolicy interface {
    HandleError(ctx context.Context, errMsg ErrorMessage)
    HandleDeadLetter(ctx context.Context, dlMsg DeadLetterMessage)
    HandleStatus(ctx context.Context, evt StatusEvent)
}
```

## 분류 체계

### ErrorSeverity (에러 심각도)

| 상수 | 값 | 설명 |
|------|-----|------|
| `SeverityCritical` | `"critical"` | 시스템 레벨 치명적 에러 (즉시 대응 필요) |
| `SeverityError` | `"error"` | 일반 처리 에러 (재시도 또는 핸들링 가능) |
| `SeverityWarning` | `"warning"` | 경고 (처리 계속, 주의 필요) |

### ErrorCategory (에러 범주)

| 상수 | 값 | 설명 |
|------|-----|------|
| `CategoryProcessing` | `"processing"` | 노드 데이터 처리 중 에러 |
| `CategoryValidation` | `"validation"` | 입력 데이터 검증 실패 |
| `CategoryTimeout` | `"timeout"` | 타임아웃 초과 |
| `CategoryConnection` | `"connection"` | 외부 연결 에러 |
| `CategoryConfiguration` | `"configuration"` | 설정 오류 |
| `CategorySystem` | `"system"` | 시스템 레벨 에러 |

### DropReason (폐기 사유)

| 상수 | 값 | 설명 |
|------|-----|------|
| `ReasonTTLExpired` | `"ttl_expired"` | 메시지 TTL 만료 |
| `ReasonBackpressureDrop` | `"backpressure_drop"` | 백프레셔로 인한 폐기 |
| `ReasonFilterRejected` | `"filter_rejected"` | 필터 노드 조건 불일치 |
| `ReasonMaxRetriesExceeded` | `"max_retries_exceeded"` | 최대 재시도 횟수 초과 |
| `ReasonNodeStopped` | `"node_stopped"` | 수신 노드 Stopped 상태 |
| `ReasonChannelFull` | `"channel_full"` | 채널 버퍼 가득 참 |

### ComponentType (컴포넌트 타입)

| 상수 | 값 | 설명 |
|------|-----|------|
| `ComponentFlow` | `"flow"` | 플로우 |
| `ComponentNode` | `"node"` | 노드 |
| `ComponentAgent` | `"agent"` | 에이전트 |
| `ComponentScriptEngine` | `"script_engine"` | 스크립트 엔진 |
| `ComponentPlugin` | `"plugin"` | 플러그인 |

## 사용 예시

### ErrorMessage 생성

```go
errMsg, err := xferr.NewErrorMessage(
    originalMsg,
    fmt.Errorf("processing failed"),
    "node-001",
    xferr.WithSeverity(xferr.SeverityCritical),
    xferr.WithCategory(xferr.CategoryProcessing),
    xferr.WithStackContext("MyNode.Process"),
)
```

### DeadLetterMessage 생성

```go
dlMsg, err := xferr.NewDeadLetterMessage(
    originalMsg,
    xferr.ReasonTTLExpired,
    xferr.WithSourceNodeID("node-001"),
    xferr.WithSourceWireID("wire-001"),
    xferr.WithContext(map[string]string{"ttl": "30s"}),
)
```

### StatusEvent 생성

```go
evt, err := xferr.NewStatusEvent(
    xferr.ComponentNode,
    "node-001",
    lifecycle.StateRunning,
    lifecycle.StateStopped,
    xferr.WithInfo(map[string]string{"reason": "shutdown"}),
)
```

### ErrorRouter 사용

```go
// 라우터 생성 (폐기 정책 설정)
router := xferr.NewDefaultErrorRouter(
    xferr.NewLogAndDiscardPolicy(logger, metrics),
)

// 수신자 등록
router.RegisterErrorReceiver("node-001", myCatchNode)   // 특정 노드
router.RegisterErrorReceiver("*", globalErrorHandler)     // 전체 와일드카드
router.RegisterDeadLetterReceiver(myDeadLetterNode)
router.RegisterStatusReceiver(myStatusNode)

// 에러 라우팅
router.RouteError(ctx, errMsg)
```

### DiscardPolicy 정책

```go
// 기본 정책: 로그 기록 후 폐기
policy := xferr.NewLogAndDiscardPolicy(logger, metrics)

// 조용한 폐기: 메트릭만 기록
policy := xferr.NewSilentDiscardPolicy(metrics)

// 개발 환경: Critical 에러 시 panic
policy := xferr.NewPanicOnCriticalPolicy(logger, metrics)
```

## 에러 타입 목록 (9개 센티넬 에러)

| 에러 변수 | 메시지 | 발생 조건 |
|-----------|--------|----------|
| `ErrNilMessage` | xferr: nil message | nil 메시지로 ErrorMessage/DeadLetterMessage 생성 시도 |
| `ErrNilError` | xferr: nil error | nil 에러로 ErrorMessage 생성 시도 |
| `ErrNoReceiver` | xferr: no receiver registered | 에러/폐기 메시지의 수신자가 없음 |
| `ErrAlertThresholdExceeded` | xferr: alert threshold exceeded | 알림 임계값 초과 |
| `ErrInvalidSeverity` | xferr: invalid severity | 유효하지 않은 심각도 |
| `ErrInvalidCategory` | xferr: invalid category | 유효하지 않은 범주 |
| `ErrInvalidDropReason` | xferr: invalid drop reason | 유효하지 않은 폐기 사유 |
| `ErrInvalidComponentType` | xferr: invalid component type | 유효하지 않은 컴포넌트 타입 |
| `ErrSameStateTransition` | xferr: same state transition | 동일 상태로의 전이 시도 |

모든 에러는 `errors.Is()` 함수로 비교 가능하다.

## 파일 구조

```
pkg/xferr/
  error_message.go          # ErrorMessage 인터페이스 + defaultErrorMessage + 생성자
  error_message_test.go     # ErrorMessage 단위 테스트
  dead_letter_message.go    # DeadLetterMessage 인터페이스 + defaultDeadLetterMessage + 생성자
  dead_letter_message_test.go
  status_event.go           # StatusEvent 인터페이스 + defaultStatusEvent + 생성자
  status_event_test.go
  classification.go         # ErrorSeverity, ErrorCategory, DropReason, ComponentType + 검증 함수
  classification_test.go
  discard_policy.go         # DiscardPolicy 인터페이스 + LogAndDiscard/SilentDiscard/PanicOnCritical
  discard_policy_test.go
  router.go                 # ErrorRouter 인터페이스 + DefaultErrorRouter (sync.RWMutex)
  router_test.go
  alert.go                  # AlertThreshold, AlertConfig, AlertCallback 타입
  alert_test.go
  errors.go                 # 9개 sentinel 에러 변수
  errors_test.go
  doc.go                    # 패키지 문서
```

## 의존성

- **표준 라이브러리**: `sync`, `time`, `context`, `errors`, `fmt`
- **내부 의존성**:
  - `pkg/message` (SPEC-MSG-001) - `Message` 인터페이스 (원본 메시지 참조)
  - `pkg/lifecycle` (SPEC-LIFE-001) - `State` 타입 (StatusEvent 상태 전이)

## 소비 패키지

- `internal/engine/` (SPEC-ENGINE-001) - Wire 에러 라우팅, TTL Dead Letter 생성, DiscardPolicy 설정
- `internal/node/` (SPEC-NODE-001) - BaseNode 에러 포트 전달, Catch/Status/Dead Letter 노드
- `internal/observe/` (SPEC-OBS-001) - 폐기 시 로그/메트릭 기록, Logger/Metrics 인터페이스 구현

## 테스트

```bash
# 전체 테스트 실행
go test ./pkg/xferr/...

# Race Detector 포함 테스트
go test -race ./pkg/xferr/...

# 커버리지 확인
go test -cover ./pkg/xferr/

# 상세 커버리지 리포트
go test -coverprofile=cover.out ./pkg/xferr/
go tool cover -html=cover.out
```

### 테스트 결과

- 커버리지: 98.0%
- Race Detector: 이상 없음 (go test -race)
- Go Vet: 이상 없음 (go vet)

## 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-MSG-001 | 의존 | `Message` 인터페이스 (ErrorMessage/DeadLetterMessage가 원본 메시지 참조) |
| SPEC-LIFE-001 | 의존 | `State` 타입 (StatusEvent의 이전/이후 상태) |
| SPEC-NODE-001 | 소비자 | BaseNode 에러 포트에서 `ErrorMessage` 전달, Catch/Status/Dead Letter 노드가 타입 소비 |
| SPEC-ENGINE-001 | 소비자 | Wire System이 `DeadLetterMessage` 생성, 에러 라우팅에 `ErrorRouter` 사용 |
| SPEC-OBS-001 | 소비자 | DiscardPolicy 실행 시 `Logger`/`Metrics`에 기록 |
