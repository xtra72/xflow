---
id: SPEC-SYSAGENT-001
type: plan
version: "1.0.0"
spec_ref: SPEC-SYSAGENT-001
---

# SPEC-SYSAGENT-001 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반): 모든 파일이 신규 생성이므로 TDD(RED-GREEN-REFACTOR) 적용
- 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표
- `go test -race` 필수 실행 (동시성 안전 검증)

### 1.2 기술 스택

- **언어**: Go 1.23+
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **의존 패키지**:
  - `internal/agent/` (SPEC-AGENT-001): BaseAgent, SystemAgent, TypeRegistry
  - `pkg/lifecycle/` (SPEC-LIFE-001): State 타입
  - `pkg/message/` (SPEC-MSG-001): Message 인터페이스
  - `pkg/xferr/` (SPEC-ERR-001): ErrorMessage, StatusEvent
  - `internal/observe/` (SPEC-OBS-001): 컴포넌트별 로거 팩토리
- **외부 라이브러리**:
  - `github.com/fsnotify/fsnotify` (File Agent)
  - `github.com/robfig/cron/v3` (Timer Agent)
- **동시성**: `sync.Map` (Store), `sync.RWMutex` (Event 구독자), `atomic` (통계 카운터), Go 채널 (이벤트 전달)

### 1.3 패키지 위치

- **경로**: `internal/agent/system/`
- **Tier**: Tier 2 - 내부 실행 계층 (external import 불가)
- **소비자**: `internal/node/`, `internal/script/`, `internal/engine/`, `internal/plugin/`

---

## 2. 마일스톤

### Primary Goal: Event Agent + Store Agent + Error Types (P0)

**범위**: Module 1, 2, 4

**작업 항목**:

1. `errors.go` + `errors_test.go` 작성
   - 23개 sentinel error 변수 정의 (Event 5, Store 4, Timer 4, File 4, Logger 3, Manager 3)
   - `errors.Is()` 호환성 테스트
   - `fmt.Errorf("%w")` 래핑 호환성 테스트

2. `event.go` + `event_test.go` 작성
   - `EventAgent` 인터페이스 정의
   - `eventAgent` unexported 구현체 (BaseAgent 임베딩)
   - `IsSystem() bool` = true, `RequiresTransport() bool` = false
   - `init()` 함수에서 TypeRegistry 자동 등록
   - `Emit()` 구현: 토픽별 구독자에게 비동기 전달 (Go 채널)
   - `Subscribe()` / `SubscribePattern()` 구현: SubscriptionID 생성, 구독자 등록
   - `Unsubscribe()` 구현: 구독 해제
   - 와일드카드 패턴 매칭 (`*`, `**`) 구현
   - 이벤트 버퍼링 (기본 256, 초과 시 드롭 + 메트릭)
   - 내장 이벤트 토픽 상수 정의
   - 이벤트 통계 (EventsEmitted, EventsDelivered, EventsDropped 등)
   - 동시성 테스트 (`sync.RWMutex` 기반 구독자 관리)
   - Emit + Subscribe + Unsubscribe 동시 호출 테스트

3. `store.go` + `store_test.go` 작성
   - `StoreAgent` 인터페이스 정의
   - `storeAgent` unexported 구현체 (BaseAgent 임베딩)
   - `IsSystem() bool` = true, `RequiresTransport() bool` = false
   - `init()` 함수에서 TypeRegistry 자동 등록
   - `Get()` / `Set()` / `Delete()` / `Keys()` / `Clear()` 구현
   - `SetWithTTL()` 구현: 만료 goroutine + lazy expiration
   - 네임스페이스 격리 구현 (`sync.Map` 중첩)
   - 네임스페이스명 유효성 검증 (영문, 숫자, 하이픈, 밑줄)
   - `Watch()` / `Unwatch()` 구현: 키 변경 감시
   - Watch 와일드카드 `"*"` 구현
   - 스토어 통계 (TotalKeys, GetHits, GetMisses 등)
   - TTL 만료 정리 goroutine 테스트
   - `sync.Map` 동시성 테스트

**산출물**: 핵심 System Agent(Event, Store)와 에러 타입 완성

---

### Secondary Goal: Timer Agent (P0)

**범위**: Module 3

**작업 항목**:

1. `timer.go` + `timer_test.go` 작성
   - `TimerAgent` 인터페이스 정의
   - `timerAgent` unexported 구현체 (BaseAgent 임베딩)
   - `IsSystem() bool` = true, `RequiresTransport() bool` = false
   - `init()` 함수에서 TypeRegistry 자동 등록
   - `SetInterval()` 구현: `time.Ticker` 기반 주기 실행
   - 최소 간격 검증 (100ms, `ErrIntervalTooShort`)
   - `SetCron()` 구현: `robfig/cron/v3` 연동
   - cron 표현식 유효성 검증 (`ErrInvalidCronExpression`)
   - `SetTimeout()` 구현: `time.AfterFunc` 기반 1회 실행
   - `Cancel()` 구현: 타이머 중지 및 제거
   - `List()` 구현: 활성 타이머 목록 조회
   - Graceful Shutdown: context 취소 시 모든 타이머 정리
   - 타이머 통계 (ActiveTimers, TotalTriggers 등)
   - 동시성 테스트 (동시 Set/Cancel)
   - cron 트리거 정확성 테스트

**산출물**: Timer Agent 완성

---

### Tertiary Goal: File Agent + Logger Agent (P1)

**범위**: Module 5, 6

**작업 항목**:

1. `file.go` + `file_test.go` 작성
   - `FileAgent` 인터페이스 정의
   - `fileAgent` unexported 구현체 (BaseAgent 임베딩)
   - `IsSystem() bool` = true, `RequiresTransport() bool` = false
   - `init()` 함수에서 TypeRegistry 자동 등록
   - 샌드박스 경로 검증: `filepath.Clean()` + `filepath.Abs()` + `filepath.EvalSymlinks()`
   - `Read()` / `Write()` / `Append()` 구현
   - `Exists()` / `Remove()` / `List()` 구현
   - `WatchDir()` 구현: `fsnotify.Watcher` 연동
   - `UnwatchDir()` 구현: 감시 해제
   - 파일 통계 (FilesRead, BytesRead 등)
   - 샌드박스 외부 접근 거부 테스트
   - 심볼릭 링크 추적 후 샌드박스 검증 테스트
   - fsnotify 이벤트 수신 테스트 (임시 디렉토리 활용)

2. `logger.go` + `logger_test.go` 작성
   - `LoggerAgent` 인터페이스 정의
   - `loggerAgent` unexported 구현체 (BaseAgent 임베딩)
   - `IsSystem() bool` = true, `RequiresTransport() bool` = false
   - `init()` 함수에서 TypeRegistry 자동 등록
   - `Log()` / `Debug()` / `Info()` / `Warn()` / `Error()` 구현
   - slog 연동: `internal/observe/` 로거 팩토리 사용
   - `SetLevel()` / `GetLevel()` 구현: 런타임 로그 레벨 변경
   - `Subscribe()` / `Unsubscribe()` 구현: 로그 스트림 실시간 구독
   - 로거 통계 (TotalLogs, LogsByLevel 등)
   - 로그 레벨 동적 변경 테스트
   - 로그 스트림 구독/해제 테스트
   - 느린 소비자 드롭 테스트

**산출물**: File Agent + Logger Agent 완성

---

### Final Goal: System Agent Manager + Info/Stats (P1)

**범위**: Module 7, 8

**작업 항목**:

1. `manager.go` + `manager_test.go` 작성
   - `SystemAgentManager` 인터페이스 정의
   - `systemAgentManager` unexported 구현체
   - `Initialize()` 구현: 5종 Agent 일괄 생성 (의존성 순서)
   - `Start()` 구현: 순서대로 시작 (Event -> Store -> Timer -> Logger -> File)
   - `Stop()` 구현: 역순으로 중지 (File -> Logger -> Timer -> Store -> Event)
   - 접근자 메서드: `Event()`, `Logger()`, `File()`, `Timer()`, `Store()`
   - `IsInitialized()` 구현
   - 초기화 순서 테스트
   - 시작 실패 시 롤백 테스트 (이미 시작된 Agent 중지)
   - 중복 초기화 방지 테스트
   - 초기화 전 접근 시 에러 테스트

2. 각 System Agent의 `Info()` 메서드에 `SystemAgentInfo` 확장 구현
   - `SystemType`, `AutoStart`, `Config` 필드 추가
   - Agent별 특화 통계 필드 통합 검증

**산출물**: System Agent Manager + 정보/통계 시스템 완성

---

### Optional Goal: 패키지 문서화

**범위**: doc.go

**작업 항목**:

1. `doc.go` 작성
   - 패키지 개요 문서
   - 각 System Agent 사용 예시
   - 접근 방식 설명 (직접 참조 vs Bridge Node 경유)
   - TypeRegistry 등록 패턴 설명

**산출물**: GoDoc 문서 완성

---

## 3. 기술적 접근 방식

### 3.1 BaseAgent 임베딩 패턴

모든 System Agent는 SPEC-AGENT-001의 `BaseAgent`를 임베딩하여 공통 로직을 재사용한다. 이는 SPEC-SAGENT-001의 표준 Agent와 동일한 패턴이다:

- BaseAgent가 Init, Start, Stop, Pause, Resume, Health, ID, Name, Type의 기본 구현을 제공
- System Agent는 도메인 특화 메서드만 추가 구현
- `IsSystem()` = true, `RequiresTransport()` = false를 오버라이드

### 3.2 동시성 모델

각 System Agent의 동시성 전략:

| Agent | 동시성 도구 | 보호 대상 |
|-------|-----------|----------|
| Event | `sync.RWMutex` + 버퍼 채널 | 구독자 맵, 이벤트 전달 |
| Store | `sync.Map` | 네임스페이스별 키-값 맵 |
| Timer | `sync.RWMutex` | 타이머 맵, cron 스케줄러 |
| File | `sync.Map` | 파일 감시 맵 |
| Logger | `sync.Map` | 로그 레벨 맵, 스트림 맵 |

### 3.3 TypeRegistry 등록

SPEC-SAGENT-001과 동일 패턴으로, 각 파일의 `init()` 함수에서 TypeRegistry에 팩토리를 등록한다. 이를 통해 `import _ "github.com/xtra/xflow/internal/agent/system"` 시 자동으로 모든 System Agent 타입이 등록된다.

### 3.4 이벤트 버퍼 오버플로우 처리

Event Agent의 채널 버퍼가 가득 찼을 때의 전략:

1. `select` + `default` 패턴으로 non-blocking send
2. 오버플로우 발생 시 이벤트를 드롭
3. 드롭된 이벤트 수를 `EventsDropped` 메트릭으로 기록
4. 관찰성 시스템에 경고 로그 출력

### 3.5 Store TTL 구현

TTL은 두 가지 전략을 결합하여 구현한다:

- **Lazy expiration**: `Get()` 호출 시 만료 여부 확인, 만료되었으면 삭제 후 `(nil, false)` 반환
- **Periodic cleanup**: 백그라운드 goroutine이 주기적으로(기본 1분) 만료된 키를 정리
- TTL 정보는 키별 메타데이터 구조체에 저장 (`expiresAt time.Time`)

### 3.6 파일 샌드박스 검증

File Agent의 경로 검증 절차:

1. `filepath.Clean(path)` - 경로 정규화 (`..` 등 제거)
2. `filepath.Abs(cleaned)` - 절대 경로 변환
3. `filepath.EvalSymlinks(abs)` - 심볼릭 링크 해결
4. `strings.HasPrefix(resolved, sandboxRoot)` - 샌드박스 범위 확인
5. 위반 시 `ErrPathOutsideSandbox` 반환

### 3.7 와일드카드 패턴 매칭

Event Agent의 토픽 패턴 매칭 알고리즘:

- 토픽은 `.`으로 분리된 세그먼트 배열
- `*`: 정확히 하나의 세그먼트에 매칭 (예: `flow.*` -> `flow.started`, `flow.stopped`)
- `**`: 하나 이상의 세그먼트에 매칭 (예: `agent.**` -> `agent.connected`, `agent.mqtt.error`)
- 패턴 구독 시 정규식으로 컴파일하여 캐시, Emit 시 정규식 매칭

---

## 4. 리스크 및 대응

### R1: 프레임워크 패키지 미완성

- **리스크**: SPEC-AGENT-001의 BaseAgent, SystemAgent, TypeRegistry가 아직 구현되지 않아 컴파일 불가
- **대응**: 인터페이스 mock을 사용하여 테스트 작성, 프레임워크 완성 후 통합 테스트 실행
- **심각도**: 중간 (인터페이스 계약은 확정되어 있으므로 mock으로 진행 가능)

### R2: Event Agent 메모리 누수

- **리스크**: 구독 해제 없이 구독자가 축적되면 메모리 누수 발생
- **대응**: 구독자 최대 수 제한 설정, Agent Stop 시 모든 구독 자동 해제, WeakRef 패턴 고려
- **심각도**: 중간

### R3: Store Agent sync.Map 성능

- **리스크**: 키 수가 급증하면 `sync.Map`의 성능 특성 변화
- **대응**: 네임스페이스별 분리로 단일 맵 크기 제한, 벤치마크 테스트로 성능 임계점 확인
- **심각도**: 낮음 (대부분의 IoT 시나리오에서는 키 수가 제한적)

### R4: Timer Agent goroutine 누수

- **리스크**: 타이머 취소 없이 Agent Stop 시 goroutine 누수
- **대응**: Graceful Shutdown에서 모든 타이머 강제 취소, context 기반 goroutine 종료 보장
- **심각도**: 중간

### R5: File Agent 보안

- **리스크**: 심볼릭 링크 TOCTOU (Time-of-Check, Time-of-Use) 취약점
- **대응**: `filepath.EvalSymlinks()` 후 즉시 파일 작업 수행, 가능하면 `O_NOFOLLOW` 플래그 사용
- **심각도**: 중간 (샌드박스 환경이므로 제한적 위험)

### R6: 외부 라이브러리 호환성

- **리스크**: `fsnotify`, `robfig/cron` 라이브러리 버전 변경
- **대응**: `go.mod`에 정확한 버전 고정, 인터페이스 래퍼로 격리
- **심각도**: 낮음

---

## 5. 의존성 순서

```
구현 순서 (의존성 방향):

1. errors.go          (의존성 없음)
2. event.go           (errors.go 의존)
3. store.go           (errors.go 의존)
4. timer.go           (errors.go, robfig/cron 의존)
5. file.go            (errors.go, fsnotify 의존)
6. logger.go          (errors.go, internal/observe 의존)
7. manager.go         (event, store, timer, file, logger 의존)
8. doc.go             (문서만)
```

---

## 6. 테스트 전략

### 6.1 단위 테스트 구조

- 모든 테스트는 테이블 드리븐 방식으로 작성
- mock BaseAgent/SystemAgent를 테스트 파일 내에 정의
- `testify/assert` 및 `testify/require` 사용
- 임시 디렉토리 활용 (`t.TempDir()`) - File Agent 테스트

### 6.2 테스트 카테고리

| 카테고리 | 대상 | 검증 항목 |
|---------|------|---------|
| 인터페이스 테스트 | EventAgent, StoreAgent 등 | SystemAgent 인터페이스 준수 (IsSystem, RequiresTransport) |
| CRUD 테스트 | StoreAgent | Get/Set/Delete/Keys/Clear 기본 동작 |
| Pub/Sub 테스트 | EventAgent | 발행/구독/해제, 와일드카드 매칭 |
| 스케줄링 테스트 | TimerAgent | Interval/Cron/Timeout 동작, 취소 |
| 파일 IO 테스트 | FileAgent | 읽기/쓰기/감시, 샌드박스 검증 |
| 로깅 테스트 | LoggerAgent | 레벨 변경, 스트림 구독, slog 연동 |
| 생명주기 테스트 | SystemAgentManager | 초기화/시작/중지 순서, 롤백 |
| 동시성 테스트 | 전체 | `go test -race` 통과, 다중 goroutine 접근 |
| 통계 테스트 | 전체 | Agent별 특화 통계 정확성 |
| sentinel 에러 테스트 | errors.go | `errors.Is()` 호환성, 래핑 호환성 |

### 6.3 벤치마크 테스트

- `BenchmarkEventEmit` - 이벤트 발행 성능 (구독자 10/100/1000)
- `BenchmarkStoreGetSet` - Store Get/Set 성능 (키 100/1000/10000)
- `BenchmarkTimerSetInterval` - 타이머 생성 성능
- `BenchmarkPatternMatch` - 와일드카드 패턴 매칭 성능
- `BenchmarkFileRead` - 파일 읽기 성능 (1KB/1MB/10MB)

### 6.4 통합 테스트 시나리오

프레임워크 패키지 완성 후 실행:

1. **SystemAgentManager 전체 생명주기**: Initialize -> Start -> 각 Agent 사용 -> Stop
2. **Event + Timer 연동**: Timer가 주기적으로 Event를 발행, 구독자가 수신
3. **Store + File 연동**: File 변경 감지 -> Store에 메타데이터 저장
4. **Logger + Event 연동**: Event 발행 시 Logger로 로그 기록
