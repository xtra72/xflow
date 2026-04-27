# system - Event Agent, File Agent, SystemAgentManager

`internal/agent/system` 패키지의 SPEC-SYSAGENT-001 신규 구현 모듈이다. Event Agent (Go 채널 기반 Pub/Sub), File Agent (샌드박스 파일 I/O), SystemAgentManager (일괄 생명주기 관리)를 제공한다.

**SPEC**: SPEC-SYSAGENT-001

## 아키텍처 개요

```
    SPEC-SYSAGENT-001 신규 모듈 구조

    +----------------------------------------------------+
    |              SystemAgentManager                     |
    |  Initialize / Start / Stop 생명주기                  |
    |  초기화 순서: Event -> File                          |
    |  종료 순서: File -> Event (역순)                     |
    +----------------------------------------------------+
            |                         |
    +-------v--------+       +-------v--------+
    |  EventAgentImpl |       |  FileAgentImpl |
    |  *BaseAgent 임베딩|       |  *BaseAgent 임베딩|
    |  Pub/Sub 시스템  |       |  샌드박스 파일 I/O |
    +----------------+       +----------------+
            |                         |
    +-------v--------+       +-------v--------+
    |  EventEmitter   |       |  FileOperator  |
    |  6개 메서드      |       |  8개 메서드      |
    +----------------+       +----------------+
```

**핵심 구성 요소**:

1. **EventAgentImpl**: Go 채널 기반 Pub/Sub, 토픽/패턴 구독, 비동기 전달
2. **FileAgentImpl**: 샌드박스 경로 제한, 파일 CRUD, fsnotify 디렉토리 감시
3. **SystemAgentManager**: Event/File 에이전트 일괄 생성/시작/중지 조정

---

## Event Agent

### EventEmitter 인터페이스

```go
type EventEmitter interface {
    Emit(topic string, data interface{}) error
    Subscribe(topic string, handler EventHandler) (SubscriptionID, error)
    SubscribePattern(pattern string, handler EventHandler) (SubscriptionID, error)
    Unsubscribe(id SubscriptionID) error
    Topics() []string
    SubscriberCount(topic string) int
}
```

### Event 구조체

```go
type Event struct {
    Topic     string      // dot-separated 토픽 이름
    Data      interface{} // 이벤트 페이로드
    Timestamp time.Time   // 발행 시각
    Source    string      // 발행자 Agent ID
}
```

### Pub/Sub 패턴

**토픽 구독**:

`Subscribe(topic, handler)` 메서드로 특정 토픽에 핸들러를 등록한다. 동일 토픽에 다중 구독자를 등록할 수 있으며, 고유한 `SubscriptionID`를 반환한다.

```go
agent := system.NewEventAgent(256)
agent.Init(agent.AgentConfig{ID: "system-event", Name: "event", Type: "event"})

subID, err := agent.Subscribe("flow.started", func(event system.Event) {
    fmt.Printf("플로우 시작: %v\n", event.Data)
})
```

**패턴 구독**:

`SubscribePattern(pattern, handler)` 메서드로 와일드카드 패턴에 매칭되는 모든 토픽의 이벤트를 수신한다.

```go
// 단일 세그먼트 패턴
subID, _ := agent.SubscribePattern("flow.*", handler)
// -> flow.started, flow.stopped 매칭

// 다중 세그먼트 패턴
subID, _ := agent.SubscribePattern("agent.**", handler)
// -> agent.connected, agent.mqtt.error 모두 매칭
```

### 패턴 매칭 규칙

| 패턴 | 의미 | 매칭 예시 | 비매칭 예시 |
|------|------|----------|------------|
| `*` | 단일 세그먼트 | `flow.*` -> `flow.started` | `flow.sub.detail` |
| `**` | 다중 세그먼트 | `agent.**` -> `agent.mqtt.error` | `flow.started` |

### 비동기 전달 메커니즘

- 각 구독자에 버퍼 채널 할당 (기본 256)
- `Emit()` 호출 시 non-blocking으로 채널에 이벤트 전송
- 별도 goroutine에서 채널 수신 후 핸들러 호출

**버퍼 드롭 전략**:

- 채널 버퍼가 가득 차면 해당 구독자에 대한 이벤트를 드롭
- 드롭된 이벤트 수를 `eventsDropped` atomic 카운터로 추적
- 구독자가 없는 토픽에 발행 시 에러 없이 무시

### 이벤트 통계

`eventStats` 구조체의 atomic 카운터로 통계를 관리한다.

| 통계 필드 | 설명 |
|-----------|------|
| `EventsEmitted` | 발행된 총 이벤트 수 |
| `EventsDelivered` | 전달 성공한 총 이벤트 수 |
| `EventsDropped` | 버퍼 초과로 드롭된 이벤트 수 |
| `ActiveSubscriptions` | 현재 활성 구독 수 |
| `ActiveTopics` | 현재 구독자가 있는 토픽 수 |

---

## File Agent

### FileOperator 인터페이스

```go
type FileOperator interface {
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, data []byte, perm os.FileMode) error
    AppendFile(path string, data []byte) error
    FileExists(path string) bool
    RemoveFile(path string) error
    ListDir(dir string) ([]FileInfo, error)
    WatchDir(dir string, handler FileEventHandler) (FileWatchID, error)
    UnwatchDir(id FileWatchID) error
}
```

### 샌드박스 보안 모델

모든 파일 접근은 샌드박스 디렉토리 내로 제한된다.

**경로 검증 순서**:

1. `filepath.Clean()` 으로 경로 정규화
2. `filepath.Abs()` 로 절대 경로 변환
3. `filepath.EvalSymlinks()` 로 심볼릭 링크 해석
4. 해석된 실제 경로가 샌드박스 루트 하위인지 검증

**보안 규칙**:

- 샌드박스 외부 접근 시 `ErrPathOutsideSandbox` 에러
- 심볼릭 링크가 샌드박스 외부를 가리키는 경우에도 차단
- 샌드박스 루트는 `SystemAgentManager.Initialize()` 시 `SystemConfig.FileSandboxRoot`로 설정

### 파일 CRUD 연산

| 연산 | 메서드 | 동작 |
|------|--------|------|
| 읽기 | `ReadFile(path)` | 파일 전체 내용을 `[]byte`로 반환 |
| 쓰기 | `WriteFile(path, data, perm)` | 파일 생성 또는 덮어쓰기 (중간 디렉토리 자동 생성) |
| 추가 | `AppendFile(path, data)` | 기존 파일 끝에 데이터 추가 |
| 존재 확인 | `FileExists(path)` | 파일 존재 여부 반환 |
| 삭제 | `RemoveFile(path)` | 파일 삭제 |
| 목록 | `ListDir(dir)` | 디렉토리 내 파일/폴더 목록 반환 |

### fsnotify 디렉토리 감시

`WatchDir(dir, handler)` 메서드로 디렉토리 내 파일 변경을 실시간 감시한다.

```go
watchID, err := fileAgent.WatchDir("/data/input", func(event system.FileEvent) {
    fmt.Printf("파일 변경: %s (%s)\n", event.Path, event.Op)
})
defer fileAgent.UnwatchDir(watchID)
```

**감시 이벤트 종류**:

| FileOp | 설명 |
|--------|------|
| `FileOpCreate` | 파일 생성 |
| `FileOpWrite` | 파일 쓰기 |
| `FileOpRemove` | 파일 삭제 |
| `FileOpRename` | 파일 이름 변경 |
| `FileOpChmod` | 파일 권한 변경 |

**제한 사항**:

- 재귀적 하위 디렉토리 감시는 지원하지 않음 (지정 디렉토리만)
- 감시 goroutine은 context 취소 시 정상 종료

### 파일 통계

| 통계 필드 | 설명 |
|-----------|------|
| `FilesRead` | 읽기 실행 횟수 |
| `FilesWritten` | 쓰기 실행 횟수 |
| `BytesRead` | 읽은 총 바이트 수 |
| `BytesWritten` | 쓴 총 바이트 수 |
| `ActiveWatches` | 활성 디렉토리 감시 수 |
| `FileEventsReceived` | 수신된 파일 이벤트 수 |

---

## SystemAgentManager

SystemAgentManager는 Event Agent와 File Agent의 일괄 생성/시작/중지를 관리한다.

### 인터페이스

```go
type SystemAgentManager struct {
    // Event() *EventAgentImpl  - Event Agent 접근자
    // File()  *FileAgentImpl   - File Agent 접근자
}

type SystemConfig struct {
    EventBufferSize int    // Event 채널 버퍼 크기 (기본: 256)
    FileSandboxRoot string // File Agent 샌드박스 루트 디렉토리
    LogDefaultLevel string // 기본 로그 레벨 (향후 사용)
}
```

### Initialize / Start / Stop 생명주기

**Initialize(config SystemConfig)**:

초기화 시 Event Agent와 File Agent를 순서대로 생성한다.

- 초기화 순서: Event -> File
- 이미 초기화된 상태에서 재호출 시 `ErrAlreadyInitialized` 에러
- File Agent 초기화 실패 시 Event Agent를 롤백 (Stop 호출)

**Start(ctx context.Context)**:

모든 관리 대상 Agent를 시작한다.

- 시작 순서: Event -> File
- 하나의 Agent 시작 실패 시 이미 시작된 Agent를 역순으로 중지하고 에러 반환

**Stop(ctx context.Context)**:

모든 관리 대상 Agent를 역순으로 중지한다.

- 종료 순서: File -> Event (역순)
- 각 Agent의 Graceful Shutdown 보장

### 에러 롤백 메커니즘

초기화 및 시작 과정에서 에러가 발생하면 이미 성공한 작업을 역순으로 롤백한다.

```
Initialize 롤백 예시:
  1. Event Agent 초기화 성공
  2. File Agent 초기화 실패
  3. -> Event Agent Stop() 호출 (롤백)
  4. -> 에러 반환

Start 롤백 예시:
  1. Event Agent 시작 성공
  2. File Agent 시작 실패
  3. -> Event Agent Stop() 호출 (롤백)
  4. -> 에러 반환
```

### 사용 예시

```go
mgr := system.NewSystemAgentManager()

// 초기화
config := system.SystemConfig{
    EventBufferSize: 512,
    FileSandboxRoot: "/data/sandbox",
}
if err := mgr.Initialize(config); err != nil {
    log.Fatal(err)
}

// 시작
ctx := context.Background()
if err := mgr.Start(ctx); err != nil {
    log.Fatal(err)
}
defer mgr.Stop(ctx)

// Event Agent 사용
subID, _ := mgr.Event().Subscribe("flow.started", func(e system.Event) {
    fmt.Println("플로우 시작:", e.Data)
})
mgr.Event().Emit("flow.started", "flow-001")

// File Agent 사용
data, _ := mgr.File().ReadFile("/data/sandbox/config.json")
fmt.Println(string(data))
```

---

## 에러 타입 목록

### Event Agent 에러 (6개)

| 에러 변수 | 메시지 | 발생 조건 |
|-----------|--------|----------|
| `ErrTopicEmpty` | system/event: topic cannot be empty | 빈 토픽으로 발행/구독 시도 |
| `ErrHandlerNil` | system/event: handler cannot be nil | nil 핸들러로 구독 시도 |
| `ErrInvalidPattern` | system/event: invalid subscription pattern | 유효하지 않은 패턴 구독 |
| `ErrEventBufferFull` | system/event: event buffer is full | 이벤트 버퍼 초과 |
| `ErrEventAgentClosed` | system/event: event agent is closed | 닫힌 Event Agent 연산 |
| `ErrEventSubNotFound` | system/event: subscription not found | 존재하지 않는 구독 해제 |

### File Agent 에러 (6개)

| 에러 변수 | 메시지 | 발생 조건 |
|-----------|--------|----------|
| `ErrPathOutsideSandbox` | system/file: path outside sandbox directory | 샌드박스 외부 경로 접근 |
| `ErrSandboxNotConfigured` | system/file: sandbox directory not configured | 샌드박스 미설정 |
| `ErrFileNotFound` | system/file: file not found | 파일 미존재 |
| `ErrWatchPathInvalid` | system/file: watch path is invalid | 유효하지 않은 감시 경로 |
| `ErrFileAgentClosed` | system/file: file agent is closed | 닫힌 File Agent 연산 |
| `ErrDirNotFound` | system/file: directory not found | 디렉토리 미존재 |

### Manager 에러 (3개)

| 에러 변수 | 메시지 | 발생 조건 |
|-----------|--------|----------|
| `ErrAlreadyInitialized` | system: system agents already initialized | 중복 초기화 시도 |
| `ErrNotInitialized` | system: system agents not initialized | 초기화 전 조작 시도 |
| `ErrAgentTypeUnknown` | system: unknown system agent type | 미등록 System Agent 타입 |

모든 에러는 `errors.Is()` 함수로 비교 가능하다.

---

## 파일 구조

```
internal/agent/system/
  # Event Agent (SPEC-SYSAGENT-001 신규)
  event.go                 # EventAgentImpl, EventEmitter 인터페이스
  event_errors.go          # Event 센티넬 에러 (6개)
  event_test.go            # Event Agent 테스트
  event_errors_test.go     # Event 에러 테스트

  # File Agent (SPEC-SYSAGENT-001 신규)
  file.go                  # FileAgentImpl, FileOperator 인터페이스
  file_errors.go           # File 센티넬 에러 (6개)
  file_test.go             # File Agent 테스트
  file_errors_test.go      # File 에러 테스트

  # SystemAgentManager (SPEC-SYSAGENT-001 신규)
  system_manager.go        # SystemAgentManager 생명주기 관리
  system_manager_test.go   # Manager 테스트
  manager_errors.go        # Manager 센티넬 에러 (3개)
  manager_errors_test.go   # Manager 에러 테스트

  # 기존 모듈 (변경 없음)
  store.go                 # StoreAgent (SPEC-STORE-001)
  timer.go                 # TimerAgent (SPEC-TIMER-001)
  logger.go                # LoggerAgent
  ...
```

## 의존성

- **표준 라이브러리**: `sync`, `sync/atomic`, `time`, `context`, `os`, `path/filepath`, `strings`, `fmt`, `errors`
- **외부 의존성**:
  - `github.com/fsnotify/fsnotify` - File Agent 디렉토리 감시
- **내부 의존성**:
  - `internal/agent` (SPEC-AGENT-001) - BaseAgent, Agent/SystemAgent 인터페이스, AgentConfig

## 테스트

```bash
# 전체 테스트 실행
go test ./internal/agent/system/...

# Race Detector 포함 테스트
go test -race ./internal/agent/system/...

# 커버리지 확인
go test -cover ./internal/agent/system/...

# 상세 커버리지 리포트
go test -coverprofile=cover.out ./internal/agent/system/...
go tool cover -html=cover.out
```

### 테스트 결과

- 테스트: 55개 전체 통과
- 커버리지: 87.4%
- Race Detector: 이상 없음 (go test -race)
- Go Vet: 이상 없음 (go vet)

## 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-AGENT-001 | 의존 | BaseAgent, Agent/SystemAgent 인터페이스, AgentConfig |
| SPEC-LIFE-001 | 의존 | lifecycle.BaseLifecycle 임베딩 (BaseAgent 경유) |
| SPEC-STORE-001 | 동료 | 동일 패키지 StoreAgent (기존, 변경 없음) |
| SPEC-TIMER-001 | 동료 | 동일 패키지 TimerAgent (기존, 변경 없음) |
| SPEC-NODE-001 | 소비자 | Bridge Node가 System Agent를 참조 |
| SPEC-ENGINE-001 | 소비자 | Flow Engine이 SystemAgentManager를 자동 초기화 |
