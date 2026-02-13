---
id: SPEC-ERR-001
version: "1.0.0"
status: draft
created: "2026-02-13"
updated: "2026-02-13"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-13 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-ERR-001: Error & Dead Letter Message System - 에러/폐기/상태 메시지 데이터 타입, 분류 체계, 라우팅 인프라, 폐기 정책

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 횡단 관심사(cross-cutting concern)인 에러 및 폐기 메시지 처리를 위한 데이터 타입과 인프라를 정의한다. 본 SPEC은 에러 메시지, 폐기(Dead Letter) 메시지, 상태 이벤트의 구조적 타입을 정의하고, 이러한 메시지를 적절한 수신자에게 라우팅하는 인프라와, 수신자가 없을 때의 폐기 정책을 다룬다.

본 패키지는 **다른 SPEC들이 소비하는 공통 데이터 타입과 인프라**를 제공하는 역할이다:

- SPEC-ENGINE-001 (Wire System): TTL 만료 시 `DeadLetterMessage` 생성, 수신 노드 Stopped 시 에러 라우팅
- SPEC-NODE-001 (BaseNode): 에러 출력 포트를 통해 `ErrorMessage` 전달, Catch/Status/Dead Letter 노드가 이 타입을 소비

본 SPEC은 다음을 포함한다:

- **ErrorMessage Type** (`error_message.go`): 원본 메시지 + 에러 컨텍스트를 래핑하는 구조체
- **DeadLetterMessage Type** (`dead_letter_message.go`): 폐기된 메시지 + 폐기 사유를 래핑하는 구조체
- **StatusEvent Type** (`status_event.go`): 컴포넌트 상태 전이 이벤트 구조체
- **Error Classification** (`classification.go`): 에러 심각도, 에러 범주, 폐기 사유 열거 타입
- **Discard Policy** (`discard_policy.go`): 수신자 미연결 시 폐기 동작 정책
- **Error Router** (`router.go`): 에러/폐기/상태 메시지를 적절한 수신자에게 라우팅하는 인터페이스
- **Alert Threshold** (`alert.go`): 에러/폐기 비율 초과 시 알림 트리거 설정
- **Error Types** (`errors.go`): 본 패키지 전용 sentinel 에러 정의

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `pkg/xferr/`
- **Tier**: Tier 1 - 공개 기반 계층 (pkg/message/, pkg/lifecycle/와 동일 계층)
- **의존 패키지**:
  - `pkg/message/` (SPEC-MSG-001): `Message` 인터페이스 (원본 메시지 참조)
  - `pkg/lifecycle/` (SPEC-LIFE-001): `State` 타입 (StatusEvent의 상태 전이)
- **소비 패키지**:
  - `internal/engine/` (SPEC-ENGINE-001): Wire 에러 라우팅, TTL Dead Letter 생성
  - `internal/node/` (SPEC-NODE-001): BaseNode 에러 포트 전달, Catch/Status/Dead Letter 노드
  - `internal/observe/` (SPEC-OBS-001): 폐기 시 로그/메트릭 기록
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **외부 의존성**: 없음 (표준 라이브러리 + pkg/ 계층 패키지만 의존)

### 1.3 설계 원칙

- **인터페이스 우선**: ErrorMessage, DeadLetterMessage, StatusEvent는 인터페이스로 정의하고, 구현체는 unexported struct로 캡슐화
- **제로 할당 원칙**: Catch 노드 미연결 시 에러 채널을 생성하지 않음 (product.md: "에러 포트에 Catch 노드가 연결되지 않으면 채널을 생성하지 않음")
- **관찰성 내장**: 수신자 유무와 관계없이 모든 에러/폐기 이벤트는 관찰성 시스템에 메트릭/로그 기록 (product.md: "폐기 시에도 관찰성 시스템에 카운터/로그 기록")
- **정책 기반 설계**: 폐기 동작을 정책 타입으로 정의하여 런타임 변경 지원
- **최소 의존성**: 표준 라이브러리 + pkg/ 계층 패키지만 의존하여 순환 의존 방지
- **타입 안전 분류**: 에러 심각도, 범주, 폐기 사유를 타입으로 정의하여 컴파일 타임 안전성 보장

### 1.4 패키지 명명

Go 표준 `errors` 패키지와의 이름 충돌을 피하기 위해 `pkg/xferr/`로 명명한다. import 시 `xferr` 패키지명으로 사용한다:

```
import "github.com/xtra/xflow/pkg/xferr"
```

### 1.5 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- ErrorMessage 데이터 타입 (원본 메시지 + 에러 컨텍스트 래핑)
- DeadLetterMessage 데이터 타입 (폐기 메시지 + 폐기 사유 래핑)
- StatusEvent 데이터 타입 (컴포넌트 상태 전이 이벤트)
- 에러 심각도/범주/폐기 사유 열거 타입 및 분류 함수
- DiscardPolicy 설정 (수신자 미연결 시 동작 정책)
- ErrorRouter 인터페이스 (에러/폐기/상태 메시지 라우팅 계약)
- AlertThreshold 설정 (에러/폐기 비율 알림 트리거)
- 본 패키지 전용 sentinel 에러

**OUT OF SCOPE (별도 SPEC)**:
- 에러 출력 포트의 실제 Go 채널 구현 (SPEC-NODE-001: BaseNode 포트 시스템)
- Catch 노드 구현 (SPEC-NODE-001: Module 11)
- Status 노드 구현 (SPEC-NODE-001: Module 12)
- Dead Letter 노드 구현 (SPEC-NODE-001: Module 13)
- Wire 레벨 에러 라우팅 구현 (SPEC-ENGINE-001: Wire System)
- TTL 만료 Dead Letter 라우팅 구현 (SPEC-ENGINE-001: TTL Management)
- 관찰성 시스템 통합 구현 (SPEC-OBS-001: internal/observe/)

### 1.6 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-MSG-001 | 의존 | `Message` 인터페이스 (ErrorMessage/DeadLetterMessage가 원본 메시지 참조) |
| SPEC-LIFE-001 | 의존 | `State` 타입 (StatusEvent의 이전/이후 상태) |
| SPEC-NODE-001 | 소비자 | BaseNode 에러 포트에서 `ErrorMessage` 전달, Catch/Status/Dead Letter 노드가 타입 소비 |
| SPEC-ENGINE-001 | 소비자 | Wire System이 `DeadLetterMessage` 생성, 에러 라우팅에 `ErrorRouter` 사용 |
| SPEC-OBS-001 | 소비자 | DiscardPolicy 실행 시 `Logger`/`Metrics`에 기록 |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: `pkg/message/Message` 인터페이스가 이미 정의되어 있으며, `ID()`, `Timestamp()`, `Payload()`, `Metadata()`, `Clone()` 메서드를 제공한다 (SPEC-MSG-001)
- A2: `pkg/lifecycle/State` 타입이 이미 정의되어 있으며, 7개 공통 상태 상수를 제공한다 (SPEC-LIFE-001)
- A3: ErrorMessage, DeadLetterMessage, StatusEvent는 단일 goroutine에서 생성되므로 구조체 자체의 동시성 안전은 불필요하다
- A4: `pkg/xferr/` 패키지는 `internal/observe/`에 직접 의존하지 않으며, 관찰성 통합은 소비자(Engine, Node)가 담당한다
- A5: ErrorRouter 인터페이스의 구현체는 `internal/engine/` 또는 `internal/node/`에서 제공한다
- A6: Go의 `errors` 표준 패키지와 이름 충돌을 피하기 위해 `pkg/xferr/` 패키지명을 사용한다

### 2.2 도메인 가정

- A7: 하나의 에러 메시지는 정확히 하나의 원본 메시지에 대응한다
- A8: 에러 심각도는 3단계(Critical, Error, Warning)로 충분하며, 런타임에 새 심각도를 추가할 필요는 없다
- A9: 폐기 사유는 6종(TTLExpired, BackpressureDrop, FilterRejected, MaxRetriesExceeded, NodeStopped, ChannelFull)으로 초기 버전에 충분하다
- A10: StatusEvent는 모든 컴포넌트 타입(Flow, Node, Agent, ScriptEngine, Plugin)의 상태 변경을 통합 표현한다
- A11: 수신자가 없는 에러/폐기 메시지는 관찰성 시스템에 기록 후 폐기하는 것이 기본 정책이다 (product.md: "에러 포트에 연결된 노드가 없으면 자동 폐기 (로그에는 기록)")
- A12: Alert threshold는 슬라이딩 윈도우 기반 비율 계산이며, 단순 카운터 기반은 불충분하다

---

## 3. Requirements (요구사항)

### Module 1: ErrorMessage Type (P0)

#### REQ-ERR-001-01-01 (Ubiquitous) ErrorMessage 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `ErrorMessage` 인터페이스를 제공해야 한다:

- `OriginalMessage() message.Message` - 에러가 발생한 원본 메시지 반환
- `Error() error` - 발생한 Go error 값 반환
- `SourceNodeID() string` - 에러가 발생한 노드 ID 반환
- `ErrorTimestamp() time.Time` - 에러 발생 시각 반환
- `Severity() ErrorSeverity` - 에러 심각도 반환
- `Category() ErrorCategory` - 에러 범주 반환
- `StackContext() string` - 스택 컨텍스트 문자열 반환 (디버깅용, 빈 문자열 허용)

#### REQ-ERR-001-01-02 (Ubiquitous) ErrorMessage 기본 구현체

시스템은 **항상** `defaultErrorMessage` (unexported struct)를 `ErrorMessage` 인터페이스의 기본 구현체로 사용해야 한다.

#### REQ-ERR-001-01-03 (Ubiquitous) NewErrorMessage 생성자

시스템은 **항상** `NewErrorMessage(msg message.Message, err error, sourceNodeID string, opts ...ErrorMessageOption) ErrorMessage` 팩토리 함수를 제공해야 한다:

- `msg`, `err`, `sourceNodeID`는 필수 파라미터
- `ErrorTimestamp`는 `time.Now()`로 자동 설정
- 기본 `Severity`는 `SeverityError`
- 기본 `Category`는 `CategoryProcessing`
- 기본 `StackContext`는 빈 문자열
- Options Pattern으로 `Severity`, `Category`, `StackContext` 오버라이드 가능

#### REQ-ERR-001-01-04 (Unwanted) nil 메시지로 ErrorMessage 생성 거부

시스템은 `msg` 파라미터가 nil인 경우 `NewErrorMessage`에서 `ErrorMessage`를 생성**하지 않아야 한다**. nil 대신 적절한 zero-value ErrorMessage를 반환하거나 panic해야 한다.

#### REQ-ERR-001-01-05 (Unwanted) nil error로 ErrorMessage 생성 거부

시스템은 `err` 파라미터가 nil인 경우 `NewErrorMessage`에서 `ErrorMessage`를 생성**하지 않아야 한다**. `ErrNilError`를 반환해야 한다.

---

### Module 2: DeadLetterMessage Type (P0)

#### REQ-ERR-001-02-01 (Ubiquitous) DeadLetterMessage 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `DeadLetterMessage` 인터페이스를 제공해야 한다:

- `OriginalMessage() message.Message` - 폐기된 원본 메시지 반환
- `Reason() DropReason` - 폐기 사유 열거값 반환
- `SourceNodeID() string` - 폐기가 발생한 노드 ID 반환 (Wire에서 발생한 경우 송신 노드 ID)
- `SourceWireID() string` - 폐기가 발생한 Wire ID 반환 (노드에서 발생한 경우 빈 문자열)
- `DropTimestamp() time.Time` - 폐기 시각 반환
- `Context() map[string]string` - 추가 컨텍스트 반환 (TTL 값, 재시도 횟수 등)

#### REQ-ERR-001-02-02 (Ubiquitous) DeadLetterMessage 기본 구현체

시스템은 **항상** `defaultDeadLetterMessage` (unexported struct)를 `DeadLetterMessage` 인터페이스의 기본 구현체로 사용해야 한다.

#### REQ-ERR-001-02-03 (Ubiquitous) NewDeadLetterMessage 생성자

시스템은 **항상** `NewDeadLetterMessage(msg message.Message, reason DropReason, opts ...DeadLetterOption) DeadLetterMessage` 팩토리 함수를 제공해야 한다:

- `msg`, `reason`은 필수 파라미터
- `DropTimestamp`는 `time.Now()`로 자동 설정
- 기본 `SourceNodeID`는 빈 문자열
- 기본 `SourceWireID`는 빈 문자열
- 기본 `Context`는 빈 map
- Options Pattern으로 `SourceNodeID`, `SourceWireID`, `Context` 오버라이드 가능

#### REQ-ERR-001-02-04 (Unwanted) 유효하지 않은 DropReason 거부

시스템은 정의되지 않은 `DropReason` 값으로 `NewDeadLetterMessage`를 호출한 경우 `DeadLetterMessage`를 생성**하지 않아야 한다**. `ErrInvalidDropReason` 에러를 반환해야 한다.

---

### Module 3: StatusEvent Type (P0)

#### REQ-ERR-001-03-01 (Ubiquitous) StatusEvent 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `StatusEvent` 인터페이스를 제공해야 한다:

- `ComponentType() ComponentType` - 컴포넌트 타입 반환 (Flow, Node, Agent, ScriptEngine, Plugin)
- `ComponentID() string` - 컴포넌트 고유 ID 반환
- `PreviousState() lifecycle.State` - 이전 상태 반환
- `NewState() lifecycle.State` - 새 상태 반환
- `Timestamp() time.Time` - 상태 전이 시각 반환
- `Info() map[string]string` - 추가 정보 반환 (Error 상태 전이 시 에러 상세 등)

#### REQ-ERR-001-03-02 (Ubiquitous) StatusEvent 기본 구현체

시스템은 **항상** `defaultStatusEvent` (unexported struct)를 `StatusEvent` 인터페이스의 기본 구현체로 사용해야 한다.

#### REQ-ERR-001-03-03 (Ubiquitous) NewStatusEvent 생성자

시스템은 **항상** `NewStatusEvent(compType ComponentType, compID string, prevState, newState lifecycle.State, opts ...StatusEventOption) StatusEvent` 팩토리 함수를 제공해야 한다:

- `compType`, `compID`, `prevState`, `newState`는 필수 파라미터
- `Timestamp`는 `time.Now()`로 자동 설정
- 기본 `Info`는 빈 map
- Options Pattern으로 `Info` 오버라이드 가능

#### REQ-ERR-001-03-04 (Ubiquitous) ComponentType 열거 정의

시스템은 **항상** 다음 `ComponentType` 상수를 제공해야 한다:

- `ComponentFlow` - 플로우
- `ComponentNode` - 노드
- `ComponentAgent` - 에이전트
- `ComponentScriptEngine` - 스크립트 엔진
- `ComponentPlugin` - 플러그인

#### REQ-ERR-001-03-05 (Unwanted) 동일 상태 전이 거부

시스템은 `prevState`와 `newState`가 동일한 경우 `NewStatusEvent`에서 `StatusEvent`를 생성**하지 않아야 한다**. `ErrSameStateTransition` 에러를 반환해야 한다.

---

### Module 4: Error Severity & Classification (P0)

#### REQ-ERR-001-04-01 (Ubiquitous) ErrorSeverity 타입 정의

시스템은 **항상** `ErrorSeverity` 타입 (string 기반)과 다음 상수를 제공해야 한다:

- `SeverityCritical` - 시스템 레벨 치명적 에러 (즉시 대응 필요)
- `SeverityError` - 일반 처리 에러 (재시도 또는 에러 핸들링 가능)
- `SeverityWarning` - 경고 (처리는 계속되지만 주의 필요)

#### REQ-ERR-001-04-02 (Ubiquitous) ErrorCategory 타입 정의

시스템은 **항상** `ErrorCategory` 타입 (string 기반)과 다음 상수를 제공해야 한다:

- `CategoryProcessing` - 노드 데이터 처리 중 에러
- `CategoryValidation` - 입력 데이터 검증 실패
- `CategoryTimeout` - 타임아웃 초과
- `CategoryConnection` - 외부 연결 (Agent, DB 등) 에러
- `CategoryConfiguration` - 설정 오류
- `CategorySystem` - 시스템 레벨 에러 (메모리 부족, goroutine 누수 등)

#### REQ-ERR-001-04-03 (Ubiquitous) DropReason 타입 정의

시스템은 **항상** `DropReason` 타입 (string 기반)과 다음 상수를 제공해야 한다:

- `ReasonTTLExpired` - 메시지 TTL 만료
- `ReasonBackpressureDrop` - 백프레셔로 인한 폐기
- `ReasonFilterRejected` - 필터 노드에서 조건 불일치로 제외
- `ReasonMaxRetriesExceeded` - 최대 재시도 횟수 초과
- `ReasonNodeStopped` - 수신 노드가 Stopped 상태
- `ReasonChannelFull` - 채널 버퍼 가득 참 (드롭 정책 적용 시)

#### REQ-ERR-001-04-04 (Ubiquitous) 심각도/범주 유효성 검증 함수

시스템은 **항상** 다음 유효성 검증 함수를 제공해야 한다:

- `IsValidSeverity(s ErrorSeverity) bool` - 유효한 심각도 여부 반환
- `IsValidCategory(c ErrorCategory) bool` - 유효한 범주 여부 반환
- `IsValidDropReason(r DropReason) bool` - 유효한 폐기 사유 여부 반환

#### REQ-ERR-001-04-05 (Ubiquitous) 에러 분류 헬퍼 함수

시스템은 **항상** 다음 헬퍼 함수를 제공해야 한다:

- `ClassifySeverity(err error) ErrorSeverity` - Go error를 심각도로 분류 (context.DeadlineExceeded -> SeverityError, panic recovery -> SeverityCritical 등)
- `ClassifyCategory(err error) ErrorCategory` - Go error를 범주로 분류 (net.Error -> CategoryConnection, context.DeadlineExceeded -> CategoryTimeout 등)

---

### Module 5: Discard Policy (P1)

#### REQ-ERR-001-05-01 (Ubiquitous) DiscardPolicy 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `DiscardPolicy` 인터페이스를 제공해야 한다:

- `HandleError(ctx context.Context, errMsg ErrorMessage)` - 수신자 없는 에러 메시지 처리
- `HandleDeadLetter(ctx context.Context, dlMsg DeadLetterMessage)` - 수신자 없는 폐기 메시지 처리
- `HandleStatus(ctx context.Context, evt StatusEvent)` - 수신자 없는 상태 이벤트 처리

#### REQ-ERR-001-05-02 (Ubiquitous) LogAndDiscard 정책 구현

시스템은 **항상** `LogAndDiscardPolicy` 구현체를 제공해야 한다 (기본 정책):

- 에러 메시지: 관찰성 시스템에 에러 로그 기록 + 에러 카운터 증가 후 폐기
- 폐기 메시지: 관찰성 시스템에 드롭 카운터 증가 후 폐기
- 상태 이벤트: 관찰성 시스템에 상태 전이 로그 기록 후 폐기

#### REQ-ERR-001-05-03 (Optional) SilentDiscard 정책 구현

**가능하면** `SilentDiscardPolicy` 구현체를 제공해야 한다:

- 메트릭 카운터만 증가, 로그 기록 없음
- 고처리량 환경에서 로그 오버헤드 최소화 용도

#### REQ-ERR-001-05-04 (Optional) PanicOnCritical 정책 구현

**가능하면** `PanicOnCriticalPolicy` 구현체를 제공해야 한다:

- `SeverityCritical` 에러 메시지 수신 시 panic 발생 (개발/테스트 환경 전용)
- 그 외 심각도는 `LogAndDiscardPolicy`와 동일하게 동작

#### REQ-ERR-001-05-05 (Ubiquitous) DiscardPolicy 콜백 타입

시스템은 **항상** 다음 콜백 타입을 제공해야 한다:

- `type DiscardCallback func(ctx context.Context, logger Logger, metrics Metrics)` - 폐기 정책 내부에서 관찰성 시스템에 접근하기 위한 콜백
- `Logger` 인터페이스: `Error(msg string, args ...any)`, `Warn(msg string, args ...any)`, `Info(msg string, args ...any)` 메서드
- `Metrics` 인터페이스: `IncrCounter(name string, labels map[string]string)` 메서드

---

### Module 6: Error Router (P1)

#### REQ-ERR-001-06-01 (Ubiquitous) ErrorRouter 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `ErrorRouter` 인터페이스를 제공해야 한다:

- `RouteError(ctx context.Context, errMsg ErrorMessage) error` - 에러 메시지를 적절한 수신자에게 라우팅
- `RouteDeadLetter(ctx context.Context, dlMsg DeadLetterMessage) error` - 폐기 메시지를 적절한 수신자에게 라우팅
- `RouteStatus(ctx context.Context, evt StatusEvent) error` - 상태 이벤트를 적절한 수신자에게 라우팅

#### REQ-ERR-001-06-02 (Ubiquitous) ErrorReceiver 인터페이스 정의

시스템은 **항상** 다음 수신자 인터페이스를 제공해야 한다:

- `ErrorReceiver` 인터페이스: `ReceiveError(ctx context.Context, errMsg ErrorMessage) error` 메서드
- `DeadLetterReceiver` 인터페이스: `ReceiveDeadLetter(ctx context.Context, dlMsg DeadLetterMessage) error` 메서드
- `StatusReceiver` 인터페이스: `ReceiveStatus(ctx context.Context, evt StatusEvent) error` 메서드

#### REQ-ERR-001-06-03 (Ubiquitous) ErrorRouter 수신자 등록

시스템은 **항상** `ErrorRouter`에 다음 수신자 등록 메서드를 제공해야 한다:

- `RegisterErrorReceiver(scope string, receiver ErrorReceiver)` - 범위 기반 에러 수신자 등록
- `RegisterDeadLetterReceiver(receiver DeadLetterReceiver)` - 폐기 메시지 수신자 등록
- `RegisterStatusReceiver(receiver StatusReceiver)` - 상태 이벤트 수신자 등록
- `scope` 파라미터: 특정 노드 ID(단일 노드), 와일드카드 `"*"`(플로우 전체), 또는 노드 그룹 패턴

#### REQ-ERR-001-06-04 (Event-Driven) 에러 메시지 라우팅

**WHEN** `ErrorRouter.RouteError(ctx, errMsg)` 호출 시 매칭되는 `ErrorReceiver`가 등록되어 있으면, **THEN** 다음을 수행해야 한다:

1. `errMsg.SourceNodeID()`와 등록된 scope를 매칭
2. 매칭되는 모든 수신자에게 Fan-out 전달 (동일 에러를 여러 Catch 노드가 수신 가능)
3. 모든 수신자 전달 완료 후 nil 반환

#### REQ-ERR-001-06-05 (Event-Driven) 수신자 미매칭 시 폐기 정책 실행

**WHEN** `ErrorRouter.RouteError(ctx, errMsg)` 호출 시 매칭되는 `ErrorReceiver`가 없으면, **THEN** 설정된 `DiscardPolicy.HandleError(ctx, errMsg)`를 실행해야 한다.

#### REQ-ERR-001-06-06 (Ubiquitous) DefaultErrorRouter 기본 구현체

시스템은 **항상** `DefaultErrorRouter` 구조체를 `ErrorRouter` 인터페이스의 기본 구현체로 제공해야 한다:

- `NewDefaultErrorRouter(policy DiscardPolicy) *DefaultErrorRouter` 생성자
- 내부에 수신자 레지스트리 (scope -> receiver 매핑) 관리
- scope 매칭 로직: 정확한 노드 ID 매칭 우선, 와일드카드 `"*"` 매칭은 후순위
- `sync.RWMutex`로 수신자 레지스트리 동시성 보호

---

### Module 7: Alert Thresholds (P2)

#### REQ-ERR-001-07-01 (Ubiquitous) AlertThreshold 구조체 정의

시스템은 **항상** `AlertThreshold` 구조체를 제공해야 한다:

- `Name string` - 알림 이름
- `ThresholdType AlertThresholdType` - 비율 기반(`Rate`) 또는 개수 기반(`Count`)
- `Value float64` - 임계값 (Rate: 초당 에러 수, Count: 누적 에러 수)
- `WindowDuration time.Duration` - 슬라이딩 윈도우 크기 (Rate 타입에서 사용)
- `Scope string` - 적용 범위 (특정 플로우 ID, 노드 ID, 또는 `"*"` 전체)

#### REQ-ERR-001-07-02 (Ubiquitous) AlertThresholdType 열거 정의

시스템은 **항상** 다음 `AlertThresholdType` 상수를 제공해야 한다:

- `AlertThresholdRate` - 슬라이딩 윈도우 기반 비율 임계값
- `AlertThresholdCount` - 누적 개수 기반 임계값

#### REQ-ERR-001-07-03 (Ubiquitous) AlertCallback 타입 정의

시스템은 **항상** `AlertCallback` 타입을 제공해야 한다:

- `type AlertCallback func(threshold AlertThreshold, currentValue float64)` - 임계값 초과 시 호출되는 콜백
- 콜백 호출 시 현재 측정값과 초과된 임계값 정보를 전달

#### REQ-ERR-001-07-04 (Ubiquitous) AlertConfig 구조체 정의

시스템은 **항상** `AlertConfig` 구조체를 제공해야 한다:

- `Thresholds []AlertThreshold` - 설정된 알림 임계값 목록
- `Callback AlertCallback` - 알림 콜백 함수
- `Enabled bool` - 알림 활성화 여부

---

### Module 8: Error Types (P0)

#### REQ-ERR-001-08-01 (Ubiquitous) Sentinel 에러 정의

시스템은 **항상** 다음 sentinel 에러 변수를 제공해야 한다:

- `ErrNilMessage` - ErrorMessage/DeadLetterMessage 생성 시 nil 메시지 전달
- `ErrNilError` - ErrorMessage 생성 시 nil error 전달
- `ErrNoReceiver` - 에러/폐기 메시지의 수신자가 없음
- `ErrAlertThresholdExceeded` - 알림 임계값 초과
- `ErrInvalidSeverity` - 유효하지 않은 심각도
- `ErrInvalidCategory` - 유효하지 않은 범주
- `ErrInvalidDropReason` - 유효하지 않은 폐기 사유
- `ErrInvalidComponentType` - 유효하지 않은 컴포넌트 타입
- `ErrSameStateTransition` - 이전 상태와 새 상태가 동일

#### REQ-ERR-001-08-02 (Ubiquitous) errors.Is() 호환성

시스템은 **항상** 모든 sentinel 에러가 `errors.Is()` 함수로 비교 가능해야 한다.

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
pkg/xferr/
  error_message.go        # ErrorMessage 인터페이스 + 구현체 + 생성자
  error_message_test.go   # ErrorMessage 단위 테스트
  dead_letter_message.go  # DeadLetterMessage 인터페이스 + 구현체 + 생성자
  dead_letter_message_test.go
  status_event.go         # StatusEvent 인터페이스 + 구현체 + 생성자
  status_event_test.go
  classification.go       # ErrorSeverity, ErrorCategory, DropReason, ComponentType 타입 + 상수 + 검증 함수
  classification_test.go
  discard_policy.go       # DiscardPolicy 인터페이스 + LogAndDiscard/SilentDiscard/PanicOnCritical 구현
  discard_policy_test.go
  router.go               # ErrorRouter 인터페이스 + DefaultErrorRouter 구현
  router_test.go
  alert.go                # AlertThreshold, AlertConfig, AlertCallback 타입
  alert_test.go
  errors.go               # sentinel 에러 변수
  errors_test.go
  doc.go                  # 패키지 문서
```

### 4.2 주요 타입 시그니처

```go
package xferr

import (
    "context"
    "time"

    "github.com/xtra/xflow/pkg/lifecycle"
    "github.com/xtra/xflow/pkg/message"
)

// === Module 4: Classification Types ===

type ErrorSeverity string

const (
    SeverityCritical ErrorSeverity = "critical"
    SeverityError    ErrorSeverity = "error"
    SeverityWarning  ErrorSeverity = "warning"
)

type ErrorCategory string

const (
    CategoryProcessing    ErrorCategory = "processing"
    CategoryValidation    ErrorCategory = "validation"
    CategoryTimeout       ErrorCategory = "timeout"
    CategoryConnection    ErrorCategory = "connection"
    CategoryConfiguration ErrorCategory = "configuration"
    CategorySystem        ErrorCategory = "system"
)

type DropReason string

const (
    ReasonTTLExpired         DropReason = "ttl_expired"
    ReasonBackpressureDrop   DropReason = "backpressure_drop"
    ReasonFilterRejected     DropReason = "filter_rejected"
    ReasonMaxRetriesExceeded DropReason = "max_retries_exceeded"
    ReasonNodeStopped        DropReason = "node_stopped"
    ReasonChannelFull        DropReason = "channel_full"
)

type ComponentType string

const (
    ComponentFlow         ComponentType = "flow"
    ComponentNode         ComponentType = "node"
    ComponentAgent        ComponentType = "agent"
    ComponentScriptEngine ComponentType = "script_engine"
    ComponentPlugin       ComponentType = "plugin"
)

// === Module 1: ErrorMessage ===

type ErrorMessage interface {
    OriginalMessage() message.Message
    Error() error
    SourceNodeID() string
    ErrorTimestamp() time.Time
    Severity() ErrorSeverity
    Category() ErrorCategory
    StackContext() string
}

type ErrorMessageOption func(*defaultErrorMessage)

func NewErrorMessage(msg message.Message, err error, sourceNodeID string, opts ...ErrorMessageOption) (ErrorMessage, error)
func WithSeverity(s ErrorSeverity) ErrorMessageOption
func WithCategory(c ErrorCategory) ErrorMessageOption
func WithStackContext(ctx string) ErrorMessageOption

// === Module 2: DeadLetterMessage ===

type DeadLetterMessage interface {
    OriginalMessage() message.Message
    Reason() DropReason
    SourceNodeID() string
    SourceWireID() string
    DropTimestamp() time.Time
    Context() map[string]string
}

type DeadLetterOption func(*defaultDeadLetterMessage)

func NewDeadLetterMessage(msg message.Message, reason DropReason, opts ...DeadLetterOption) (DeadLetterMessage, error)
func WithSourceNodeID(id string) DeadLetterOption
func WithSourceWireID(id string) DeadLetterOption
func WithContext(ctx map[string]string) DeadLetterOption

// === Module 3: StatusEvent ===

type StatusEvent interface {
    ComponentType() ComponentType
    ComponentID() string
    PreviousState() lifecycle.State
    NewState() lifecycle.State
    Timestamp() time.Time
    Info() map[string]string
}

type StatusEventOption func(*defaultStatusEvent)

func NewStatusEvent(compType ComponentType, compID string, prevState, newState lifecycle.State, opts ...StatusEventOption) (StatusEvent, error)
func WithInfo(info map[string]string) StatusEventOption

// === Module 5: Discard Policy ===

type Logger interface {
    Error(msg string, args ...any)
    Warn(msg string, args ...any)
    Info(msg string, args ...any)
}

type Metrics interface {
    IncrCounter(name string, labels map[string]string)
}

type DiscardPolicy interface {
    HandleError(ctx context.Context, errMsg ErrorMessage)
    HandleDeadLetter(ctx context.Context, dlMsg DeadLetterMessage)
    HandleStatus(ctx context.Context, evt StatusEvent)
}

func NewLogAndDiscardPolicy(logger Logger, metrics Metrics) DiscardPolicy
func NewSilentDiscardPolicy(metrics Metrics) DiscardPolicy
func NewPanicOnCriticalPolicy(logger Logger, metrics Metrics) DiscardPolicy

// === Module 6: Error Router ===

type ErrorReceiver interface {
    ReceiveError(ctx context.Context, errMsg ErrorMessage) error
}

type DeadLetterReceiver interface {
    ReceiveDeadLetter(ctx context.Context, dlMsg DeadLetterMessage) error
}

type StatusReceiver interface {
    ReceiveStatus(ctx context.Context, evt StatusEvent) error
}

type ErrorRouter interface {
    RouteError(ctx context.Context, errMsg ErrorMessage) error
    RouteDeadLetter(ctx context.Context, dlMsg DeadLetterMessage) error
    RouteStatus(ctx context.Context, evt StatusEvent) error
    RegisterErrorReceiver(scope string, receiver ErrorReceiver)
    RegisterDeadLetterReceiver(receiver DeadLetterReceiver)
    RegisterStatusReceiver(receiver StatusReceiver)
}

func NewDefaultErrorRouter(policy DiscardPolicy) *DefaultErrorRouter

// === Module 7: Alert ===

type AlertThresholdType string

const (
    AlertThresholdRate  AlertThresholdType = "rate"
    AlertThresholdCount AlertThresholdType = "count"
)

type AlertThreshold struct {
    Name           string
    ThresholdType  AlertThresholdType
    Value          float64
    WindowDuration time.Duration
    Scope          string
}

type AlertCallback func(threshold AlertThreshold, currentValue float64)

type AlertConfig struct {
    Thresholds []AlertThreshold
    Callback   AlertCallback
    Enabled    bool
}
```

### 4.3 의존성 다이어그램

```
pkg/xferr/
    ├── 의존 ──► pkg/message/ (SPEC-MSG-001)
    ├── 의존 ──► pkg/lifecycle/ (SPEC-LIFE-001)
    │
    ├── 소비자 ◄── internal/engine/ (SPEC-ENGINE-001)
    │                ├── Wire: DeadLetterMessage 생성 (TTL 만료)
    │                ├── Wire: ErrorRouter.RouteError() 호출 (수신 노드 Stopped)
    │                └── Engine: DiscardPolicy 설정
    │
    ├── 소비자 ◄── internal/node/ (SPEC-NODE-001)
    │                ├── BaseNode: ErrorMessage 생성 (Process 실패 시)
    │                ├── Catch Node: ErrorReceiver 구현
    │                ├── Status Node: StatusReceiver 구현
    │                └── Dead Letter Node: DeadLetterReceiver 구현
    │
    └── 소비자 ◄── internal/observe/ (SPEC-OBS-001)
                     ├── Logger 인터페이스 구현 제공
                     └── Metrics 인터페이스 구현 제공
```

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 우선순위 | product.md 근거 |
|------------|------|---------|-----------------|
| REQ-ERR-001-01-01 ~ 05 | ErrorMessage Type | P0 | "에러 메시지에는 원본 메시지, 에러 원인, 발생 노드 정보가 포함" |
| REQ-ERR-001-02-01 ~ 04 | DeadLetterMessage Type | P0 | "폐기 원인(reason)과 폐기 시점 메타데이터 포함" |
| REQ-ERR-001-03-01 ~ 05 | StatusEvent Type | P0 | "Agent/Node의 상태 전이 이벤트를 수신" |
| REQ-ERR-001-04-01 ~ 05 | Error Classification | P0 | 에러 구조적 분류를 위한 타입 시스템 |
| REQ-ERR-001-05-01 ~ 05 | Discard Policy | P1 | "에러 포트에 연결된 노드가 없으면 자동 폐기 (로그에는 기록)" |
| REQ-ERR-001-06-01 ~ 06 | Error Router | P1 | "에러 포트에 Catch 노드가 연결되지 않으면 채널을 생성하지 않음" |
| REQ-ERR-001-07-01 ~ 04 | Alert Thresholds | P2 | "과도한 에러/폐기 발생 시 알림 트리거 설정 가능" |
| REQ-ERR-001-08-01 ~ 02 | Error Types | P0 | 패키지 전용 sentinel 에러 |
