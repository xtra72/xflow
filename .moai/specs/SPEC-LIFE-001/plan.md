---
id: SPEC-LIFE-001
type: plan
version: "1.0.0"
spec_ref: SPEC-LIFE-001
---

# SPEC-LIFE-001 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반): 신규 패키지이므로 TDD(RED-GREEN-REFACTOR) 적용
- 모든 파일이 신규 생성이므로 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표

### 1.2 기술 스택

- **언어**: Go 1.23+
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성**: `sync.Mutex` (상태 전이 보호)
- **외부 의존성**: 없음 (표준 라이브러리만 사용)

### 1.3 패키지 위치

- **경로**: `pkg/lifecycle/`
- **Tier**: Tier 1 - 공개 기반 계층
- **소비자**: internal/engine/, internal/node/, internal/agent/, internal/script/, internal/plugin/

---

## 2. 마일스톤

### Primary Goal: Core State + Lifecycle Interface + Error Types (P0)

**범위**: Module 1, 2, 3, 4, 7

**작업 항목**:

1. `errors.go` + `errors_test.go` 작성
   - 6개 sentinel error 변수 정의
   - `errors.Is()` 호환성 테스트

2. `state.go` + `state_test.go` 작성
   - `State` 타입 및 7개 상수 정의
   - `String()`, `IsValid()`, `ParseState()` 메서드 구현
   - `ValidTransitions` 맵 및 `IsValidTransition()` 함수 구현
   - 테이블 기반 테스트: 모든 유효/무효 전이 조합 검증

3. `lifecycle.go` 작성
   - `Lifecycle` 인터페이스 정의 (6개 메서드)
   - 인터페이스만 정의, 구현은 `base.go`에서

4. `configurable.go` + `configurable_test.go` 작성
   - `Configurable` 인터페이스 정의 (2개 메서드)
   - Mock 기반 인터페이스 계약 테스트

5. `event.go` 작성
   - `StateChangeEvent` 구조체
   - `StateChangeCallback`, `UnsubscribeFunc` 타입 정의

6. `options.go` 작성
   - `BaseOption` 함수 타입 및 `WithName()`, `WithOnStateChange()` 옵션

7. `base.go` + `base_test.go` 작성
   - `BaseLifecycle` 구조체 (unexported fields + exported methods)
   - `NewBaseLifecycle()`, `TransitionTo()`, `CurrentState()`, `OnStateChange()`, `ComponentName()`
   - 동시성 안전 테스트 (goroutine 경쟁 상태 검증)
   - 콜백 등록/해제/호출 순서 테스트
   - 콜백 panic recovery 테스트

**산출물**: `pkg/lifecycle/` 패키지의 핵심 기능 완성 및 테스트 통과

---

### Secondary Goal: Health Check & Recovery Policy (P1)

**범위**: Module 6

**작업 항목**:

1. `health.go` + `health_test.go` 작성
   - `HealthChecker` 인터페이스
   - `HealthStatus` 구조체
   - `RecoveryPolicy` 구조체 및 `RecoveryAction` 상수
   - `DefaultRecoveryPolicy()` 함수
   - 정책 기본값 검증 테스트

**산출물**: 헬스 체크 인터페이스 및 복구 정책 타입 완성

---

### Final Goal: State Change Event 고급 기능 (P1)

**범위**: Module 5 고급 기능

**작업 항목**:

1. 콜백 panic recovery 강화
   - `recover()` 기반 패닉 격리 구현
   - 패닉 발생 시 로그 기록 (fmt.Fprintf(os.Stderr) 사용, slog 의존 회피)
   - 패닉 후 나머지 콜백 연속 실행 보장 테스트

2. 동시성 안전 강화 테스트
   - `-race` 플래그 기반 경쟁 상태 검출 테스트
   - 다수 goroutine에서 동시 상태 전이 시도 테스트
   - 콜백 등록/해제가 상태 전이와 동시에 발생하는 시나리오 테스트

**산출물**: 프로덕션 수준의 동시성 안전성 검증 완료

---

## 3. 기술적 접근

### 3.1 State 타입 설계

`State`는 `string` 기반 타입으로 정의한다. string 기반을 선택한 이유:
- JSON/YAML 직렬화 시 사람이 읽을 수 있는 형태 유지
- SPEC-FLOW-001의 `FlowState`와 동일한 패턴 사용 (일관성)
- `fmt.Stringer` 구현이 자연스러움

### 3.2 BaseLifecycle 임베딩 패턴

```
컴포넌트 구조체가 BaseLifecycle을 임베딩하여 공통 상태 관리 로직을 재사용:

type MyAgent struct {
    *lifecycle.BaseLifecycle  // 임베딩
    // agent-specific fields
}

func NewMyAgent() *MyAgent {
    a := &MyAgent{
        BaseLifecycle: lifecycle.NewBaseLifecycle(
            lifecycle.WithName("agent.mqtt.client1"),
        ),
    }
    return a
}
```

### 3.3 콜백 관리 전략

- 콜백은 slice로 관리, 등록 순서대로 호출
- `UnsubscribeFunc` 호출 시 해당 콜백을 slice에서 제거 (nil 마킹 + 정리)
- 콜백 호출 중 새 콜백 등록/해제가 발생하면, 현재 호출 사이클에는 영향 없음 (snapshot 방식)
- 각 콜백은 `recover()`로 감싸서 panic 전파 방지

### 3.4 동시성 모델

- `sync.Mutex`를 사용하여 상태 읽기/쓰기 보호
- `TransitionTo()`: Lock -> 유효성 검증 -> 상태 변경 -> Unlock -> 콜백 호출
- 콜백 호출은 Lock 외부에서 수행 (데드락 방지)
- `CurrentState()`: Lock -> 상태 읽기 -> Unlock

### 3.5 SPEC-FLOW-001과의 분리 전략

- `pkg/lifecycle/` 와 `pkg/flow/`는 서로를 import하지 않음
- `FlowState` ↔ `State` 매핑이 필요한 경우 `internal/engine/`에서 변환 함수 구현
- 공통 상태명(Initializing, Running, Paused, Stopping, Stopped, Error)은 의미적으로 동일하나 타입이 다름

---

## 4. 리스크 및 대응

### Risk 1: FlowState와의 혼동

- **위험**: 개발자가 `State`와 `FlowState`를 혼용할 가능성
- **대응**: GoDoc 문서에 명확한 사용 가이드 작성, `internal/engine/`에서의 매핑 함수 제공 시 타입 안전한 변환 보장

### Risk 2: 콜백 내 장시간 블로킹

- **위험**: 상태 변경 콜백에서 I/O 등 느린 작업 수행 시 전체 전이 지연
- **대응**: GoDoc에 콜백 가이드라인 문서화 (비동기 처리 권장), 필요 시 콜백 타임아웃 옵션 추가 고려 (현재 스코프 밖)

### Risk 3: 순환 의존성

- **위험**: `pkg/lifecycle/`이 `internal/observe/` 등을 import하면 순환 의존 발생
- **대응**: 표준 라이브러리만 의존하도록 엄격히 제한, 패닉 로깅은 `fmt.Fprintf(os.Stderr)` 사용

### Risk 4: 동시성 버그

- **위험**: Mutex 사용 패턴 오류로 데드락 또는 경쟁 상태 발생
- **대응**: 콜백 호출을 Lock 외부에서 수행, `go test -race` 필수 실행, 동시성 시나리오 전용 테스트 작성

---

## 5. 의존성 그래프

```
pkg/lifecycle/ (본 SPEC)
  ├── 의존: 표준 라이브러리 (sync, fmt, time, errors, context)
  ├── 소비자: internal/engine/ (Flow 런타임)
  ├── 소비자: internal/node/ (Node 런타임)
  ├── 소비자: internal/agent/ (Agent 런타임)
  ├── 소비자: internal/script/ (Script 런타임)
  ├── 소비자: internal/plugin/ (Plugin 런타임)
  └── 동일 Tier: pkg/flow/ (상호 의존 없음), pkg/message/ (상호 의존 없음)
```

---

## 6. 구현 순서 (파일별)

| 순서 | 파일 | 설명 | 의존성 |
|------|------|------|--------|
| 1 | errors.go | Sentinel 에러 정의 | 없음 |
| 2 | state.go | State 타입, 상수, 전이 맵 | errors.go |
| 3 | event.go | StateChangeEvent, 콜백 타입 | state.go |
| 4 | lifecycle.go | Lifecycle 인터페이스 | state.go |
| 5 | configurable.go | Configurable 인터페이스 | 없음 |
| 6 | options.go | BaseOption 타입, 옵션 함수 | event.go |
| 7 | base.go | BaseLifecycle 구현 | state.go, event.go, errors.go, options.go |
| 8 | health.go | HealthChecker, RecoveryPolicy | 없음 |

모든 파일에 대해 TDD 방식으로 테스트 파일을 먼저 작성한다.
