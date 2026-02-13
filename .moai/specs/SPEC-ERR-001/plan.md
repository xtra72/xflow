---
id: SPEC-ERR-001
type: plan
version: "1.0.0"
spec_ref: SPEC-ERR-001
---

# SPEC-ERR-001 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반): 모든 파일이 신규 생성이므로 TDD(RED-GREEN-REFACTOR) 적용
- 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표
- `go test -race` 필수 실행 (인터페이스 타입 안전성 검증)

### 1.2 기술 스택

- **언어**: Go 1.23+
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **의존 패키지**:
  - `pkg/message/` (SPEC-MSG-001): Message 인터페이스
  - `pkg/lifecycle/` (SPEC-LIFE-001): State 타입
- **동시성**: `sync.RWMutex` (ErrorRouter 수신자 레지스트리 보호)

### 1.3 패키지 위치

- **경로**: `pkg/xferr/`
- **Tier**: Tier 1 - 공개 기반 계층 (외부 임포트 가능)
- **소비자**: `internal/engine/`, `internal/node/`, `internal/observe/`

### 1.4 패키지 명명 결정

Go 표준 `errors` 패키지와의 이름 충돌을 방지하기 위해 `pkg/xferr/`로 명명한다. `xf`는 xflow의 약어이며, 소비자 코드에서 다음과 같이 사용한다:

```go
import "github.com/xtra/xflow/pkg/xferr"

errMsg, err := xferr.NewErrorMessage(msg, processingErr, nodeID)
```

---

## 2. 마일스톤

### Primary Goal: Error Types + Classification + Core Data Types (P0)

**범위**: Module 4, 8, 1, 2, 3

**작업 항목**:

1. `errors.go` + `errors_test.go` 작성
   - 9개 sentinel error 변수 정의 (`ErrNilMessage`, `ErrNilError`, `ErrNoReceiver`, `ErrAlertThresholdExceeded`, `ErrInvalidSeverity`, `ErrInvalidCategory`, `ErrInvalidDropReason`, `ErrInvalidComponentType`, `ErrSameStateTransition`)
   - `errors.Is()` 호환성 테스트

2. `classification.go` + `classification_test.go` 작성
   - `ErrorSeverity` 타입 + 3개 상수 (Critical, Error, Warning)
   - `ErrorCategory` 타입 + 6개 상수
   - `DropReason` 타입 + 6개 상수
   - `ComponentType` 타입 + 5개 상수
   - `IsValidSeverity()`, `IsValidCategory()`, `IsValidDropReason()` 검증 함수
   - `ClassifySeverity()`, `ClassifyCategory()` 분류 헬퍼 함수
   - 유효/무효 값에 대한 테이블 드리븐 테스트

3. `error_message.go` + `error_message_test.go` 작성
   - `ErrorMessage` 인터페이스 정의
   - `defaultErrorMessage` unexported 구현체
   - `NewErrorMessage()` 생성자 + Options Pattern
   - `WithSeverity()`, `WithCategory()`, `WithStackContext()` 옵션 함수
   - nil message/error 거부 테스트
   - 기본값 적용 테스트 (Severity=Error, Category=Processing)
   - 옵션 오버라이드 테스트

4. `dead_letter_message.go` + `dead_letter_message_test.go` 작성
   - `DeadLetterMessage` 인터페이스 정의
   - `defaultDeadLetterMessage` unexported 구현체
   - `NewDeadLetterMessage()` 생성자 + Options Pattern
   - `WithSourceNodeID()`, `WithSourceWireID()`, `WithContext()` 옵션 함수
   - 유효하지 않은 DropReason 거부 테스트
   - 기본값 적용 테스트
   - Context 맵 독립성 테스트 (외부 맵 변경이 내부에 영향 없음)

5. `status_event.go` + `status_event_test.go` 작성
   - `StatusEvent` 인터페이스 정의
   - `defaultStatusEvent` unexported 구현체
   - `NewStatusEvent()` 생성자 + Options Pattern
   - `WithInfo()` 옵션 함수
   - 동일 상태 전이 거부 테스트
   - 모든 ComponentType에 대한 생성 테스트
   - Info 맵 독립성 테스트

**산출물**: 에러 시스템의 핵심 데이터 타입 완성

---

### Secondary Goal: Discard Policy + Error Router (P1)

**범위**: Module 5, 6

**작업 항목**:

1. `discard_policy.go` + `discard_policy_test.go` 작성
   - `Logger` 인터페이스 정의
   - `Metrics` 인터페이스 정의
   - `DiscardPolicy` 인터페이스 정의
   - `LogAndDiscardPolicy` 구현
     - ErrorMessage 수신 시 로그 기록 + 카운터 증가 테스트
     - DeadLetterMessage 수신 시 카운터 증가 테스트
     - StatusEvent 수신 시 로그 기록 테스트
   - `SilentDiscardPolicy` 구현
     - 로그 기록 없이 카운터만 증가 테스트
   - `PanicOnCriticalPolicy` 구현
     - SeverityCritical 시 panic 발생 테스트
     - SeverityError/Warning 시 일반 폐기 테스트
   - Mock Logger/Metrics를 사용한 검증

2. `router.go` + `router_test.go` 작성
   - `ErrorReceiver` 인터페이스 정의
   - `DeadLetterReceiver` 인터페이스 정의
   - `StatusReceiver` 인터페이스 정의
   - `ErrorRouter` 인터페이스 정의
   - `DefaultErrorRouter` 구현
     - 수신자 등록 테스트 (단일 노드 scope, 와일드카드 scope)
     - 에러 라우팅 매칭 테스트 (정확한 노드 ID 매칭)
     - 와일드카드 매칭 테스트 (`"*"` scope)
     - Fan-out 전달 테스트 (동일 에러를 여러 수신자에게 전달)
     - 수신자 미매칭 시 DiscardPolicy 호출 테스트
     - DeadLetter 라우팅 테스트
     - Status 라우팅 테스트
     - 동시성 안전 테스트 (`sync.RWMutex` 검증)

**산출물**: 에러 라우팅 인프라 완성

---

### Tertiary Goal: Alert Thresholds (P2)

**범위**: Module 7

**작업 항목**:

1. `alert.go` + `alert_test.go` 작성
   - `AlertThresholdType` 타입 + 2개 상수 (Rate, Count)
   - `AlertThreshold` 구조체 정의
   - `AlertCallback` 함수 타입 정의
   - `AlertConfig` 구조체 정의
   - AlertThreshold 유효성 검증 테스트
   - AlertConfig 기본값 테스트

**산출물**: 알림 임계값 설정 타입 완성

---

### Optional Goal: 패키지 문서화

**범위**: doc.go

**작업 항목**:

1. `doc.go` 작성
   - 패키지 개요 문서
   - 주요 타입 사용 예시
   - 다른 패키지와의 관계 설명

**산출물**: GoDoc 문서 완성

---

## 3. 기술적 접근 방식

### 3.1 인터페이스 설계 패턴

모든 핵심 타입(ErrorMessage, DeadLetterMessage, StatusEvent)은 인터페이스로 정의하고, 구현체는 unexported struct로 캡슐화한다. 이는 SPEC-MSG-001의 Message 타입과 동일한 패턴이다:

- 공개 인터페이스: 읽기 전용 접근자 메서드만 노출
- unexported 구현체: 내부 필드 직접 접근 불가
- 팩토리 함수: `NewXxx()` 형태의 생성자만 exported

### 3.2 Options Pattern

모든 생성자에 Options Pattern을 적용하여 선택적 파라미터를 유연하게 전달한다:

```go
errMsg, err := xferr.NewErrorMessage(
    originalMsg,
    processingErr,
    "node-filter-01",
    xferr.WithSeverity(xferr.SeverityCritical),
    xferr.WithCategory(xferr.CategoryConnection),
    xferr.WithStackContext("connection to broker lost at line 142"),
)
```

### 3.3 맵 필드 방어적 복사

`Context()`, `Info()` 메서드가 반환하는 `map[string]string`은 내부 맵의 복사본을 반환하여 외부 변경이 내부 상태에 영향을 미치지 않도록 한다. 생성자에서 맵을 받을 때도 복사하여 저장한다.

### 3.4 ErrorRouter 동시성 모델

`DefaultErrorRouter`의 수신자 레지스트리는 `sync.RWMutex`로 보호한다:

- `RegisterXxxReceiver()`: Write Lock
- `RouteXxx()`: Read Lock
- 등록은 초기화 시점에 주로 수행되고, 라우팅은 고빈도로 호출되므로 RWMutex가 적합

### 3.5 에러 분류 함수 확장성

`ClassifySeverity()`와 `ClassifyCategory()` 함수는 알려진 Go 에러 타입(`context.DeadlineExceeded`, `net.Error` 등)에 대해 매핑을 제공하되, 알 수 없는 에러는 기본값(`SeverityError`, `CategoryProcessing`)을 반환한다. 소비자가 더 세밀한 분류가 필요하면 직접 `WithSeverity()`/`WithCategory()` 옵션을 사용한다.

---

## 4. 리스크 및 대응

### R1: pkg/message/ 및 pkg/lifecycle/ 미완성

- **리스크**: 의존 패키지가 아직 구현되지 않아 컴파일 불가
- **대응**: 테스트에서 mock 인터페이스 사용, 의존 패키지 완성 후 통합 테스트 실행
- **심각도**: 중간 (인터페이스만 의존하므로 mock으로 대체 가능)

### R2: ErrorRouter 성능

- **리스크**: 고빈도 에러 발생 시 라우팅 오버헤드
- **대응**: scope 매칭에 map 기반 O(1) 조회 사용, 벤치마크 테스트 포함
- **심각도**: 낮음 (에러 경로는 정상 처리 경로보다 빈도가 낮음)

### R3: 패키지 명명 혼동

- **리스크**: `xferr` 패키지명이 직관적이지 않을 수 있음
- **대응**: doc.go에 패키지 목적과 명명 이유를 명확히 문서화, import alias 사용 예시 제공
- **심각도**: 낮음 (명명 규칙이 일관적이면 적응 가능)

---

## 5. 의존성 순서

```
구현 순서 (의존성 방향):

1. errors.go           (의존성 없음)
2. classification.go   (의존성 없음)
3. error_message.go    (pkg/message 의존)
4. dead_letter_message.go (pkg/message 의존)
5. status_event.go     (pkg/lifecycle 의존)
6. discard_policy.go   (error_message, dead_letter_message, status_event 의존)
7. router.go           (discard_policy, error_message, dead_letter_message, status_event 의존)
8. alert.go            (의존성 없음, 독립 타입)
9. doc.go              (문서만)
```

---

## 6. 테스트 전략

### 6.1 단위 테스트 구조

- 모든 테스트는 테이블 드리븐 방식으로 작성
- mock Logger/Metrics를 테스트 파일 내에 정의
- `testify/assert` 및 `testify/require` 사용

### 6.2 테스트 카테고리

| 카테고리 | 대상 | 검증 항목 |
|---------|------|---------|
| 생성자 테스트 | NewErrorMessage, NewDeadLetterMessage, NewStatusEvent | 필수 파라미터, 기본값, 옵션 오버라이드, nil 거부 |
| 분류 테스트 | ClassifySeverity, ClassifyCategory | 알려진 에러 타입 매핑, 알 수 없는 에러 기본값 |
| 검증 테스트 | IsValidSeverity, IsValidCategory, IsValidDropReason | 유효/무효 값 판별 |
| 정책 테스트 | LogAndDiscard, SilentDiscard, PanicOnCritical | Logger/Metrics 호출 검증, panic 발생 검증 |
| 라우팅 테스트 | DefaultErrorRouter | scope 매칭, Fan-out, 폐기 정책 폴백, 동시성 |
| sentinel 에러 테스트 | errors.Is() | 모든 sentinel 에러의 비교 호환성 |

### 6.3 벤치마크 테스트

- `BenchmarkNewErrorMessage` - ErrorMessage 생성 성능
- `BenchmarkRouteError` - 에러 라우팅 성능 (수신자 10개)
- `BenchmarkClassifySeverity` - 에러 분류 성능
