# system - Logger 시스템 에이전트

`internal/agent/system` 패키지의 Logger Agent는 xflow 엔진의 로깅 시스템 에이전트를 제공한다. 기존 `internal/observe` 패키지(Observer, LoggerFactory, LevelManager, StreamRouter)를 래핑하여 System Agent 인터페이스를 제공한다.

**SPEC**: SPEC-LOG-001

## 아키텍처 개요

```
    Observer 래핑 아키텍처 (Wrapper Pattern)

    +---------------------------------------------------------+
    |                    LoggerAgent                            |
    |  BaseLifecycle 임베딩 (7상태 생명주기)                     |
    |  Lifecycle + HealthChecker + Logger                      |
    +---------------------------------------------------------+
            |                             |
    +-------v--------+           +--------v---------+
    |  Logger Interface|          |  BridgeHandler   |
    |  7개 메서드      |           |  메시지 디스패처   |
    +----------------+           +------------------+
            |
    +-------v---------+
    |   observe.Observer |  래핑 대상
    +-----------------+
       |        |        |
       v        v        v
    Loggers  Levels   Streams
    (Factory) (Manager) (Router)
```

**핵심 구성 요소**:

1. **Logger 인터페이스**: 7개 메서드 (WriteLog/SetLevel/SetLevelByPattern/GetLevel/Subscribe/Unsubscribe/Components)
2. **LoggerAgent**: BaseLifecycle 임베딩, Lifecycle/HealthChecker/Logger 구현
3. **observe.Observer 래핑**: LoggerFactory(컴포넌트별 로거), LevelManager(레벨 관리), StreamRouter(스트림 라우팅) 위임
4. **subscription 관리**: sync.RWMutex 보호 구독 맵, 원자 카운터 기반 ID 생성
5. **Pause 안전 정책**: Debug/Info 억제, Warn/Error 항상 허용
6. **BridgeHandler**: Bridge Node 메시지 기반 Logger 접근 디스패처 (7개 연산)

## 빠른 시작

### LoggerAgent 생성 및 초기화

`NewLoggerAgent()` 함수는 Options 패턴으로 설정을 받아 LoggerAgent를 생성한다. `Init()`으로 초기화하면 Observer와 구독 맵이 생성된다.

```go
package main

import (
    "context"
    "log/slog"
    "os"

    "github.com/xtra/xflow/internal/agent/system"
)

func main() {
    ctx := context.Background()

    // LoggerAgent 생성 (Options 패턴)
    agent := system.NewLoggerAgent(
        system.WithLogDefaultLevel(slog.LevelDebug), // 기본 레벨
        system.WithLogFormat("json"),                  // 출력 포맷
        system.WithLogWriter(os.Stdout),               // 출력 Writer
    )

    // 초기화 (Created -> Initializing -> Running)
    if err := agent.Init(ctx); err != nil {
        panic(err)
    }
    defer agent.Stop(ctx)
}
```

### 로그 기록

`WriteLog()` 메서드로 컴포넌트별 로그를 기록한다. Observer의 LoggerFactory에서 ComponentLogger를 획득하여 위임한다.

```go
// 컴포넌트별 로그 기록
err := agent.WriteLog(ctx, "auth-service", slog.LevelInfo, "사용자 인증 성공",
    "user_id", "user-123",
    "method", "oauth2")

err = agent.WriteLog(ctx, "db-pool", slog.LevelWarn, "커넥션 풀 부족",
    "active", 95,
    "max", 100)
```

**주요 특징**:
- Observer의 LoggerFactory.NewLogger(component)로 ComponentLogger 획득
- Debug/Info/Warn/Error 레벨별 메서드 자동 분기
- Pause 상태에서 Debug/Info는 억제 (nil 반환, 에러 아님)
- Pause 상태에서 Warn/Error는 정상 기록 (안전 우선)

### 레벨 관리

`SetLevel()`, `SetLevelByPattern()`, `GetLevel()` 메서드로 컴포넌트별 로그 레벨을 관리한다.

```go
// 컴포넌트별 레벨 설정
err := agent.SetLevel(ctx, "auth-service", slog.LevelDebug)

// 패턴 매칭 레벨 일괄 설정 ("db-*" 패턴)
count, err := agent.SetLevelByPattern(ctx, "db-*", slog.LevelWarn)
fmt.Printf("%d개 컴포넌트 레벨 변경\n", count)

// 레벨 조회
level, err := agent.GetLevel(ctx, "auth-service")
fmt.Printf("auth-service 레벨: %s\n", level.String())
```

### 로그 스트림 구독

`Subscribe()`, `Unsubscribe()` 메서드로 컴포넌트 로그를 추가 Writer로 라우팅한다.

```go
// 파일로 로그 스트림 구독
file, _ := os.Create("auth.log")
subID, err := agent.Subscribe(ctx, "auth-service", file)
fmt.Printf("구독 ID: %s\n", subID) // "sub-auth-service-1"

// 구독 해제
err = agent.Unsubscribe(ctx, subID)
```

### 컴포넌트 목록 조회

`Components()` 메서드로 등록된 모든 컴포넌트 이름을 조회한다.

```go
components, err := agent.Components(ctx)
for _, name := range components {
    fmt.Printf("컴포넌트: %s\n", name)
}
```

### Bridge Node 메시지 프로토콜

LoggerBridgeHandler를 사용하면 Bridge Node를 통해 메시지 기반으로 Logger에 접근할 수 있다.

```go
handler := system.NewLoggerBridgeHandler(agent)

// Write 연산 메시지 구성
msg := message.New()
msg.Metadata().Set("logger.operation", "write")
msg.Metadata().Set("logger.component", "auth-service")
msg.Metadata().Set("logger.level", "info")
msg.Payload().Set("message", "사용자 로그인")

resp, err := handler.HandleMessage(ctx, msg)
status, _ := resp.Metadata().Get("logger.status") // "ok" 또는 "error"

// GetLevel 연산 메시지
levelMsg := message.New()
levelMsg.Metadata().Set("logger.operation", "get_level")
levelMsg.Metadata().Set("logger.component", "auth-service")

levelResp, _ := handler.HandleMessage(ctx, levelMsg)
level, _ := levelResp.Payload().Get("level") // "INFO"

// Components 연산 메시지
compMsg := message.New()
compMsg.Metadata().Set("logger.operation", "components")

compResp, _ := handler.HandleMessage(ctx, compMsg)
comps, _ := compResp.Payload().Get("components") // []string
```

## API 레퍼런스

### Logger 인터페이스

```go
type Logger interface {
    WriteLog(ctx context.Context, component string, level slog.Level, msg string, args ...any) error
    SetLevel(ctx context.Context, component string, level slog.Level) error
    SetLevelByPattern(ctx context.Context, pattern string, level slog.Level) (int, error)
    GetLevel(ctx context.Context, component string) (slog.Level, error)
    Subscribe(ctx context.Context, component string, writer io.Writer) (string, error)
    Unsubscribe(ctx context.Context, subscriptionID string) error
    Components(ctx context.Context) ([]string, error)
}
```

### LoggerOption 옵션

| 옵션 함수 | 기본값 | 설명 |
|-----------|--------|------|
| `WithLogDefaultLevel(level)` | `slog.LevelInfo` | 기본 로그 레벨 |
| `WithLogFormat(format)` | `"json"` | 출력 포맷 ("json" 또는 "text") |
| `WithLogWriter(writer)` | `os.Stdout` (via Observer) | 기본 출력 Writer |
| `WithLogObserver(observer)` | `nil` (Init에서 생성) | 외부 Observer 주입 |

### LoggerAgent 메서드

| 메서드 | 설명 |
|--------|------|
| `NewLoggerAgent(opts ...LoggerOption)` | Options 패턴으로 LoggerAgent 생성 |
| `Init(ctx)` | Created -> Initializing -> Running 전이 |
| `Start(ctx)` | Running 상태에서 no-op |
| `Pause(ctx)` | Running -> Paused (Debug/Info 억제, Warn/Error 허용) |
| `Resume(ctx)` | Paused -> Running (전체 레벨 복원) |
| `Stop(ctx)` | Running/Paused -> Stopping -> Stopped (Graceful Shutdown) |
| `State()` | 현재 생명주기 상태 반환 |
| `HealthCheck(ctx)` | 건강 상태 반환 (구독 수, 컴포넌트 수) |

### BridgeHandler 메시지 프로토콜

**요청 메타데이터**:

| 메타데이터 키 | 필수 | 설명 |
|--------------|------|------|
| `logger.operation` | 필수 | 연산 종류 ("write"/"set_level"/"set_level_pattern"/"get_level"/"subscribe"/"unsubscribe"/"components") |
| `logger.component` | 연산별 | 대상 컴포넌트 이름 |
| `logger.level` | 연산별 | 로그 레벨 문자열 ("debug", "info", "warn", "error") |
| `logger.pattern` | set_level_pattern | 패턴 매칭 문자열 |
| `logger.subscription_id` | unsubscribe | 구독 ID |

**요청 Payload**:

| 연산 | Payload 필드 | 설명 |
|------|-------------|------|
| `write` | `message` | 로그 메시지 텍스트 |

**응답 메타데이터**:

| 메타데이터 키 | 설명 |
|--------------|------|
| `logger.status` | "ok" 또는 "error" |
| `logger.error` | 에러 메시지 (status가 "error"일 때) |

**응답 Payload**:

| 연산 | Payload 필드 | 설명 |
|------|-------------|------|
| `get_level` | `level` | 컴포넌트 로그 레벨 문자열 |
| `set_level_pattern` | `count` | 변경된 컴포넌트 수 |
| `subscribe` | `subscription_id` | 생성된 구독 ID |
| `components` | `components` | 등록된 컴포넌트 목록 ([]string) |

## Pause 동작

### 레벨별 억제 정책

Pause 상태에서 WriteLog는 레벨에 따라 동작이 다르다:

- **Debug**: 억제 (return nil, 에러 아님)
- **Info**: 억제 (return nil, 에러 아님)
- **Warn**: 정상 기록 (안전 우선 로그)
- **Error**: 정상 기록 (안전 우선 로그)

### 관리 연산

Pause 상태에서도 관리 연산은 정상 동작한다:

- `SetLevel` / `SetLevelByPattern` / `GetLevel`: 정상 동작
- `Subscribe` / `Unsubscribe`: 정상 동작
- `Components`: 정상 동작

## 생명주기 관리

### 상태 전이 다이어그램

```
Created -> Initializing -> Running <-> Paused
                              |           |
                              v           v
                           Stopping -> Stopped
```

### Graceful Shutdown 순서

Stop 호출 시 다음 순서로 정지한다:

1. **상태 전이**: Running/Paused -> Stopping
2. **closed 플래그 설정**: 새 연산 차단
3. **구독 정리**: 모든 활성 구독의 StreamRouter.RemoveRoute 호출
4. **구독 맵 비우기**: 모든 subscription 엔트리 삭제
5. **상태 전이**: Stopping -> Stopped

## HealthCheck 정보

HealthCheck 호출 시 다음 정보를 반환한다:

| 상태 | Healthy | Message |
|------|---------|---------|
| Running | `true` | "logger-agent is healthy" |
| Paused | `true` | "logger-agent is degraded (paused)" |
| 기타 | `false` | "logger-agent is not healthy (state: {state})" |

**Details 필드** (Running/Paused 상태에서):

| 키 | 타입 | 설명 |
|----|------|------|
| `state` | `string` | 현재 생명주기 상태 |
| `active_subscriptions` | `int` | 활성 구독 수 |
| `registered_components` | `int` | 등록된 컴포넌트 수 |

## 센티넬 에러

| 에러 변수 | 메시지 | 발생 조건 |
|-----------|--------|----------|
| `ErrLoggerClosed` | logger: logger agent is closed | 중지된 Logger Agent 연산 시도 |
| `ErrLoggerPaused` | logger: logger agent is paused (debug/info suppressed) | 일시정지 상태에서 Debug/Info WriteLog 시도 |
| `ErrInvalidLevel` | logger: invalid log level | 유효하지 않은 로그 레벨 문자열 |
| `ErrInvalidComponent` | logger: invalid component name | 빈 문자열 컴포넌트 이름 |
| `ErrSubscriptionNotFound` | logger: subscription not found | 존재하지 않는 구독 ID로 Unsubscribe 시도 |

## 설계 특징

- **래퍼 패턴**: `internal/observe`의 Observer를 래핑하여 System Agent 인터페이스 제공. observe 패키지의 기능을 재구현하지 않고 위임
- **인터페이스 우선**: 모든 공개 API는 `Logger` 인터페이스로 정의되며, LoggerAgent가 구현
- **동시성 안전**: sync.RWMutex(구독 맵 및 상태 보호), atomic.Int64(구독 ID 카운터)
- **BaseLifecycle 임베딩**: 7상태 생명주기 관리를 상속하여 Init/Start/Pause/Resume/Stop 구현
- **안전 우선 Pause**: Pause 시 Debug/Info는 무시(nil 반환), Warn/Error는 항상 허용하여 안전 관련 로그 보장
- **관리 연산 비차단**: Pause 상태에서 SetLevel, Subscribe 등 관리 연산은 정상 동작
- **Graceful Shutdown**: Stop 시 모든 구독 해제 및 StreamRouter 라우트 제거
- **Options 패턴**: NewLoggerAgent()에 함수 옵션 패턴 적용
- **외부 Observer 주입**: WithLogObserver로 테스트 또는 공유 Observer 주입 가능

## 파일 구조

```
internal/agent/system/
  logger_errors.go          # 센티넬 에러 정의 (5개)
  logger.go                 # Logger 인터페이스, LoggerAgent 구조체, 생명주기/로깅 메서드 (388줄)
  logger_options.go         # LoggerOption 타입, loggerConfig, 4개 옵션 함수
  logger_bridge.go          # LoggerBridgeHandler (메시지 기반 Logger 접근 디스패처, 7개 연산)
  logger_test.go            # LoggerAgent 통합 테스트 (34개)
  logger_options_test.go    # Options 단위 테스트 (7개)
  logger_bridge_test.go     # BridgeHandler 테스트 (20개)
```

## 의존성

- **표준 라이브러리**: `sync`, `sync/atomic`, `time`, `context`, `fmt`, `io`, `log/slog`, `strings`, `errors`
- **내부 의존성**:
  - `internal/observe` (SPEC-OBS-001) - Observer 래핑 대상 (LoggerFactory, LevelManager, StreamRouter)
  - `pkg/lifecycle` (SPEC-LIFE-001) - 7상태 생명주기 관리 (BaseLifecycle 임베딩)
  - `pkg/message` (SPEC-MSG-001) - Bridge Node 메시지 처리 (LoggerBridgeHandler)

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

- 테스트: 61개 전체 통과 (34 + 7 + 20)
- 커버리지: 88.0%
- Race Detector: 이상 없음 (go test -race)
- Go Vet: 이상 없음 (go vet)

## 사용 예시

### 복합 로깅 시나리오

```go
ctx := context.Background()
agent := system.NewLoggerAgent(
    system.WithLogDefaultLevel(slog.LevelDebug),
    system.WithLogFormat("json"),
)
agent.Init(ctx)
defer agent.Stop(ctx)

// 1. 컴포넌트별 로그 기록
agent.WriteLog(ctx, "auth", slog.LevelInfo, "서비스 시작")
agent.WriteLog(ctx, "db", slog.LevelDebug, "쿼리 실행", "sql", "SELECT * FROM users")

// 2. 레벨 관리
agent.SetLevel(ctx, "db", slog.LevelWarn)              // db 컴포넌트 Warn 이상만
agent.SetLevelByPattern(ctx, "api-*", slog.LevelInfo)    // api-* 패턴 일괄 설정

// 3. 로그 스트림 구독
var buf bytes.Buffer
subID, _ := agent.Subscribe(ctx, "auth", &buf)

// 4. 컴포넌트 목록 조회
components, _ := agent.Components(ctx)
fmt.Printf("등록된 컴포넌트: %v\n", components)

// 5. 헬스 체크
health := agent.HealthCheck(ctx)
fmt.Printf("상태: %s, 구독: %d\n", health.Message, health.Details["active_subscriptions"])

// 6. Pause/Resume (Debug/Info 억제)
agent.Pause(ctx)
agent.WriteLog(ctx, "auth", slog.LevelDebug, "이 로그는 억제됨")   // nil 반환, 기록 안 됨
agent.WriteLog(ctx, "auth", slog.LevelError, "이 로그는 기록됨")   // 정상 기록
agent.Resume(ctx)

// 7. 구독 해제
agent.Unsubscribe(ctx, subID)
```

## 모범 사례

1. **컴포넌트 이름 규칙**: 명확한 식별을 위해 하이픈 구분 소문자 사용 권장 (예: "auth-service", "db-pool")
2. **Graceful Shutdown**: 애플리케이션 종료 시 반드시 Stop() 호출하여 구독 정리
3. **Pause 활용**: 부하 상황에서 Debug/Info를 억제하여 로그 노이즈 감소 (Warn/Error는 항상 보존)
4. **Observer 공유**: 여러 시스템이 동일 Observer를 사용하려면 WithLogObserver로 주입
5. **구독 관리**: 사용 완료된 구독은 즉시 Unsubscribe하여 리소스 누수 방지
6. **레벨 패턴**: SetLevelByPattern으로 관련 컴포넌트 그룹의 레벨을 효율적으로 관리
7. **에러 처리**: ErrLoggerClosed 확인으로 이미 종료된 Agent 접근 감지

## 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-LIFE-001 | 의존 | Logger Agent가 Lifecycle, HealthChecker 인터페이스를 구현 |
| SPEC-MSG-001 | 동료 | Bridge Node 통한 메시지 기반 접근 시 Message 타입 사용 |
| SPEC-STORE-001 | 동료 | 동일한 System Agent 패턴(BaseLifecycle 임베딩, Options 패턴) 공유 |
| SPEC-TIMER-001 | 동료 | 동일한 System Agent 패턴 공유, Bridge 핸들러 패턴 참조 |
| SPEC-OBS-001 | 래핑 대상 | Logger Agent가 internal/observe의 Observer를 래핑 |
| SPEC-FLOW-001 | 소비자 | 플로우가 Bridge Node 또는 직접 API로 Logger Agent 참조 |
| SPEC-CFG-001 | 소비자 | Logger 설정(기본 레벨, 포맷 등)을 설정 시스템으로 관리 |
