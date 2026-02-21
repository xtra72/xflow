# SPEC-SYSAGENT-002: 구현 계획

| 항목 | 값 |
|------|-----|
| SPEC ID | SPEC-SYSAGENT-002 |
| 선행 SPEC | SPEC-SYSAGENT-001 |
| 개발 방법론 | Hybrid (기존 코드: DDD, 신규 코드: TDD) |

---

## 1. 마일스톤

### Primary Goal: SystemConfig 확장 및 에러 정의

**대상 요구사항**: REQ-SYSAGENT-002-001, REQ-SYSAGENT-002-002, REQ-SYSAGENT-002-040, REQ-SYSAGENT-002-041

**작업 내역**:
- `SystemConfig` 구조체에 `StoreDefaultTTL`, `TimerMinInterval`, `LogDefaultLevel` 필드 추가
- `LogDefaultLevel` 타입을 `string`에서 `LogLevel`로 변경
- `manager_errors.go`에 `ErrAgentInitFailed`, `ErrAgentStartFailed`, `ErrAgentStopFailed` 센티넬 에러 추가
- 기존 테스트가 깨지지 않는지 확인

**수정 파일**:
- `internal/agent/system/system_manager.go` (SystemConfig 수정)
- `internal/agent/system/manager_errors.go` (에러 추가)
- `internal/agent/system/system_manager_test.go` (SystemConfig 테스트 업데이트)

---

### Secondary Goal: SystemAgentManager 5종 에이전트 통합

**대상 요구사항**: REQ-SYSAGENT-002-010 ~ REQ-SYSAGENT-002-017

**작업 내역**:
- `SystemAgentManager` 구조체에 `logger`, `timer`, `store` 필드 추가
- `Initialize()` 메서드를 5종 에이전트 순서대로 생성/초기화하도록 확장
  - Event (BaseAgent): `NewEventAgent()` -> `Init(agent.AgentConfig{})`
  - Store (BaseLifecycle): `NewStoreAgent(opts...)` -> `Init(ctx)`
  - Timer (BaseLifecycle): `NewTimerAgent(opts...)` -> `Init(ctx)`
  - Logger (BaseLifecycle): `NewLoggerAgent(opts...)` -> `Init(ctx)`
  - File (BaseAgent): `NewFileAgent()` -> `Init(agent.AgentConfig{})`
- `Start()` 메서드를 5종 에이전트 순서대로 시작하도록 확장
- `Stop()` 메서드를 역순(File -> Logger -> Timer -> Store -> Event)으로 종료하도록 확장
- 각 단계의 롤백 로직 구현
- `Logger()`, `Timer()`, `Store()` 접근자 메서드 추가

**수정 파일**:
- `internal/agent/system/system_manager.go`
- `internal/agent/system/system_manager_test.go`

**핵심 기술 과제**:
- BaseAgent vs BaseLifecycle의 다른 Init 시그니처 처리
- Initialize 롤백 시 이미 초기화된 에이전트를 역순으로 Stop하는 로직
- BaseLifecycle 에이전트는 `Init()` 후 이미 Running이므로 Stop 가능 확인

---

### Tertiary Goal: 브릿지 핸들러 접근

**대상 요구사항**: REQ-SYSAGENT-002-020, REQ-SYSAGENT-002-021

**작업 내역**:
- `LoggerBridge()`, `TimerBridge()`, `StoreBridge()` 메서드 추가
- 각 메서드는 해당 에이전트가 nil이면 nil 반환
- 초기화된 상태에서는 해당 브릿지 핸들러 인스턴스를 생성하여 반환

**수정 파일**:
- `internal/agent/system/system_manager.go`
- `internal/agent/system/system_manager_test.go`

---

### Final Goal: main.go 통합

**대상 요구사항**: REQ-SYSAGENT-002-030, REQ-SYSAGENT-002-031

**작업 내역**:
- `cmd/xflowd/main.go`의 `runServer()` 함수에 SystemAgentManager 생성/초기화/시작/종료 통합
- 설정 파일에서 SystemConfig 값을 읽어오거나 기본값 사용
- Observer 인스턴스를 Logger 에이전트에 전달
- 서버 종료 시 SystemAgentManager.Stop() 호출 추가

**수정 파일**:
- `cmd/xflowd/main.go`

---

## 2. 기술 접근 방식

### 2.1 Hybrid 개발 방법론 적용

| 변경 유형 | 방법론 | 적용 대상 |
|-----------|--------|-----------|
| 기존 코드 수정 | DDD (ANALYZE-PRESERVE-IMPROVE) | `SystemAgentManager.Initialize()`, `Start()`, `Stop()` 확장 |
| 신규 코드 추가 | TDD (RED-GREEN-REFACTOR) | `Logger()`, `Timer()`, `Store()` 접근자, 브릿지 메서드, 새 에러 |

### 2.2 Init 시그니처 차이 처리

두 가지 에이전트 계열의 Init 시그니처가 다르므로, `Initialize()` 내부에서 각각 직접 호출한다:

```
// BaseAgent 기반 (Event, File)
eventAgent.Init(agent.AgentConfig{ID: "system-event", Name: "system-event-agent", Type: "system.event"})

// BaseLifecycle 기반 (Store, Timer, Logger)
storeAgent.Init(context.Background())
```

별도 인터페이스나 어댑터 패턴 없이, Initialize() 메서드가 각 에이전트 타입의 차이를 직접 처리한다. 이 접근은:
- 추상화 레이어 추가를 방지한다
- 각 에이전트의 구체적 생성 로직을 명확하게 보여준다
- 기존 에이전트 구현을 변경하지 않는다

### 2.3 롤백 전략

Initialize와 Start 모두 동일한 롤백 패턴을 사용한다:

```
에이전트 목록: [Event, Store, Timer, Logger, File]
N번째 에이전트 실패 시:
  for i := N-1; i >= 0; i-- {
    agents[i].Stop(ctx)  // 에러 무시
  }
  return fmt.Errorf("agent %s failed: %w", name, ErrAgentInitFailed)
```

롤백 시 Stop 에러는 무시한다 (best-effort 정리).

### 2.4 브릿지 핸들러 생성 전략

브릿지 메서드는 호출 시마다 새 인스턴스를 생성하여 반환한다:

```
func (m *SystemAgentManager) LoggerBridge() *LoggerBridgeHandler {
    m.mu.Lock()
    defer m.mu.Unlock()
    if m.logger == nil { return nil }
    return NewLoggerBridgeHandler(m.logger)
}
```

캐싱하지 않는 이유: 브릿지 핸들러는 가벼운 래퍼이며, 매니저 레벨에서 캐싱 복잡도를 추가할 필요가 없다.

---

## 3. 아키텍처 설계 방향

### 3.1 변경 전후 비교

**변경 전** (SPEC-SYSAGENT-001 결과):
```
SystemAgentManager
  ├── event  *EventAgentImpl     (BaseAgent)
  └── file   *FileAgentImpl      (BaseAgent)
```

**변경 후** (SPEC-SYSAGENT-002 목표):
```
SystemAgentManager
  ├── event  *EventAgentImpl     (BaseAgent)
  ├── store  *StoreAgent          (BaseLifecycle)
  ├── timer  *TimerAgent          (BaseLifecycle)
  ├── logger *LoggerAgent         (BaseLifecycle)
  └── file   *FileAgentImpl      (BaseAgent)
```

### 3.2 main.go 통합 위치

```
runServer() 함수 내:
  1. 설정 로딩       (기존)
  2. 관찰성 초기화    (기존)
  3. 노드 레지스트리  (기존)
  4. SystemAgentManager 생성/초기화/시작  (신규 - Agent Manager 이전)
  5. Agent Manager   (기존 - SystemAgentManager와 연결)
  6. Flow 엔진       (기존)
  7. API 서버        (기존)
  8. 시그널 처리      (기존)
  9. 정리: SystemAgentManager.Stop()  (신규 - agentMgr.Shutdown() 이전)
```

---

## 4. 리스크 및 대응

| 리스크 | 영향 | 대응 |
|--------|------|------|
| BaseLifecycle Init()이 Running 전이에 실패할 경우 | Start() 호출 시 예상치 못한 상태 | Init 반환값 검사 후 즉시 롤백 |
| Logger 에이전트에 Observer가 없을 경우 | 로깅 기능 미작동 | SystemConfig에 Observer 필드 추가 또는 LoggerOption으로 전달 |
| 기존 Event+File 테스트와의 호환성 | 기존 테스트 실패 | SystemConfig에 추가된 필드가 zero value일 때 기본값 적용 |
| 동시성 경합 (race condition) | 데이터 손상 | 기존 sync.Mutex 패턴 유지, `go test -race` 필수 실행 |
| main.go 변경 시 서버 시작 순서 영향 | 서버 시작 실패 | SystemAgentManager를 Agent Manager 이전에 초기화 |

---

## 5. Traceability

| 요구사항 | 마일스톤 | 작업 |
|----------|----------|------|
| REQ-SYSAGENT-002-001, 002 | Primary | SystemConfig 확장 |
| REQ-SYSAGENT-002-040, 041 | Primary | 에러 정의 |
| REQ-SYSAGENT-002-010~017 | Secondary | Manager 확장 |
| REQ-SYSAGENT-002-020, 021 | Tertiary | 브릿지 접근 |
| REQ-SYSAGENT-002-030, 031 | Final | main.go 통합 |
