# config - XFlow 설정 관리 시스템

`internal/config` 패키지는 XFlow 플랫폼의 다중 소스 설정 관리 시스템을 제공한다. Viper 기반 5단계 오버라이드 체인, 7개 카테고리별 타입 안전 접근자, Mutable/Immutable 키 레지스트리, fsnotify 기반 핫 리로드, OnChange 콜백, FIFO 변경 이력을 통합적으로 지원한다.

**SPEC**: SPEC-CFG-001

## 아키텍처 개요

```
    5단계 오버라이드 체인 (우선순위 낮음 → 높음)

    +-------------------+
    | 코드 기본값        |  SetDefaults()
    +-------------------+
            |
    +-------------------+
    | 기본 경로 파일     |  $HOME/.xflow/xflow.yaml, /etc/xflow/xflow.yaml
    +-------------------+
            |
    +-------------------+
    | 사용자 지정 파일   |  WithConfigFile("custom.yaml")
    +-------------------+
            |
    +-------------------+
    | 환경 변수          |  XFLOW_SERVER_PORT=9090
    +-------------------+
            |
    +-------------------+
    | CLI 플래그         |  --server.port=9090
    +-------------------+

    +-------------------------------------------+
    |              Config 인터페이스               |
    |  Server() Engine() Storage() Auth()       |
    |  Observe() Script() Plugin()              |
    |  Get() Set() OnChange() WatchConfig()     |
    +-------------------------------------------+
            |
    +-------------------------------------------+
    |            viperConfig (비공개)              |
    |  sync.RWMutex  |  *viper.Viper            |
    |  callbacks     |  history (FIFO, max 100) |
    |  atomic nextID |  fsnotify watcher        |
    +-------------------------------------------+
            |
    +-------------------+-------------------+
    |                   |                   |
    +--------+    +----------+    +---------+
    | Mutable|    | Validate |    | Defaults|
    | Registry|   | (8 rules)|    |         |
    +--------+    +----------+    +---------+
```

**핵심 구성 요소**:

1. **Config 인터페이스**: 7개 카테고리 접근자 + 범용 Get/Set + OnChange 콜백 + WatchConfig
2. **viperConfig**: Config 인터페이스의 비공개 구현체 (sync.RWMutex 기반 동시성 안전)
3. **Mutable/Immutable 레지스트리**: 런타임 변경 가능/불가 키 분류 (와일드카드 패턴 지원)
4. **SetDefaults**: 7개 카테고리 전체 기본값 정의
5. **Validate**: 8개 검증기 기반 유효성 검증 (에러 집계)
6. **ChangeEvent + History**: FIFO 방식 변경 이력 추적 (최대 100건)

## 빠른 시작

### 기본 설정 로드

`Load()` 함수는 5단계 오버라이드 체인을 따라 설정을 로드하고, 유효성 검증을 수행한 뒤 `Config` 인터페이스를 반환한다.

```go
package main

import (
    "fmt"
    "log"

    "github.com/xtra/xflow/internal/config"
)

func main() {
    // 기본 설정 로드 (기본값 + 파일 탐색 + 환경 변수)
    cfg, err := config.Load()
    if err != nil {
        log.Fatal(err)
    }

    // 카테고리별 타입 안전 접근
    fmt.Println("서버 포트:", cfg.Server().Port)        // 8080
    fmt.Println("스토리지 타입:", cfg.Storage().Type)    // "sqlite"
    fmt.Println("엔진 정책:", cfg.Engine().ExecutionPolicy) // "parallel"

    // 범용 접근
    fmt.Println("로그 레벨:", cfg.Get("observe.default_level")) // "info"
}
```

### 옵션 지정 로드

```go
cfg, err := config.Load(
    config.WithConfigFile("/path/to/custom.yaml"),  // 사용자 설정 파일
    config.WithEnvPrefix("MYAPP"),                  // 환경 변수 접두사
    config.WithConfigPaths("/etc/myapp", "."),       // 기본 경로 탐색 위치
    config.WithLogger(slog.Default()),               // 로거 주입
)
```

### 런타임 설정 변경 (Set)

Mutable 키만 런타임에 변경할 수 있다. Immutable 키 변경 시 `ErrImmutableKey` 에러가 반환된다.

```go
// Mutable 키 변경 (성공)
err := cfg.Set("engine.backpressure_threshold", 2000)

// Immutable 키 변경 시도 (에러)
err = cfg.Set("server.port", 9090)
// err: config: key is immutable at runtime: server.port
```

### OnChange 콜백 등록

```go
// 특정 키의 변경을 감시
unsubscribe := cfg.OnChange("observe.default_level", func(event config.ChangeEvent) {
    fmt.Printf("로그 레벨 변경: %v -> %v (source: %s)\n",
        event.OldValue, event.NewValue, event.Source)
})
defer unsubscribe() // 구독 해제

// 변경 적용 -> 콜백 호출됨
cfg.Set("observe.default_level", "debug")
```

### 파일 감시 (WatchConfig)

fsnotify 기반 파일 변경 감시를 활성화하면 설정 파일 수정 시 자동으로 반영된다.

```go
// 파일 감시 시작 (100ms 디바운스)
if err := cfg.WatchConfig(); err != nil {
    log.Fatal(err)
}
defer cfg.StopWatch()

// 파일 변경 시 등록된 OnChange 콜백이 source="file"로 호출됨
```

## API 레퍼런스

### Config 인터페이스

```go
type Config interface {
    // 카테고리별 설정 접근 (7개)
    Server() ServerConfig
    Engine() EngineConfig
    Storage() StorageConfig
    Auth() AuthConfig
    Observe() ObserveConfig
    Script() ScriptConfig
    Plugin() PluginConfig

    // 범용 접근
    Get(key string) any
    IsSet(key string) bool

    // 런타임 변경
    Set(key string, value any) error
    OnChange(key string, fn ChangeCallback) UnsubscribeFunc
    ChangeHistory() []ChangeEvent

    // 파일 감시
    WatchConfig() error
    StopWatch()
}
```

### 카테고리 구조체 (7개)

| 구조체 | 키 접두사 | 주요 필드 |
|--------|----------|----------|
| `ServerConfig` | `server.*` | Port, Host, Mode, TLS, CORS, RateLimit |
| `EngineConfig` | `engine.*` | BackpressureThreshold, MaxConcurrentFlows, ExecutionPolicy, WireDefaultBuffer |
| `StorageConfig` | `storage.*` | Type, SQLitePath, PostgresDSN, PoolSize |
| `AuthConfig` | `auth.*` | JWT (Secret, AccessTTL, RefreshTTL), APIKey, OAuth2 |
| `ObserveConfig` | `observe.*` | DefaultLevel, MetricsEnabled, TraceEnabled |
| `ScriptConfig` | `script.*` | Timeout, VMPoolSize, Sandbox (Enabled, MaxMemoryMB, MaxExecutionMS) |
| `PluginConfig` | `plugin.*` | Directory, WASMEnabled, GoEnabled |

### 하위 구조체 (6개)

| 구조체 | 소속 | 필드 |
|--------|------|------|
| `TLSConfig` | ServerConfig | Enabled, CertFile, KeyFile |
| `CORSConfig` | ServerConfig | Enabled, AllowedOrigins |
| `RateLimitConfig` | ServerConfig | Enabled, RequestsPerSecond |
| `JWTConfig` | AuthConfig | Secret, AccessTTL, RefreshTTL |
| `APIKeyConfig` | AuthConfig | Enabled |
| `OAuth2Config` | AuthConfig | Providers |
| `SandboxConfig` | ScriptConfig | Enabled, MaxMemoryMB, MaxExecutionMS |

### 팩토리 함수

| 함수 | 반환 타입 | 설명 |
|------|----------|------|
| `Load(opts ...LoadOption)` | `(Config, error)` | 설정 로드 및 유효성 검증 |
| `SetDefaults(v *viper.Viper)` | - | Viper 인스턴스에 기본값 설정 |
| `Validate(v *viper.Viper)` | `error` | 8개 규칙 기반 유효성 검증 |
| `IsMutable(key string)` | `bool` | 키의 런타임 변경 가능 여부 |
| `IsImmutable(key string)` | `bool` | 키의 런타임 변경 불가 여부 |

### LoadOption 옵션

| 옵션 함수 | 기본값 | 설명 |
|-----------|--------|------|
| `WithConfigFile(path)` | `""` | 사용자 지정 설정 파일 경로 |
| `WithConfigName(name)` | `"xflow"` | 확장자 없는 설정 파일명 |
| `WithConfigPaths(paths...)` | `[".", "$HOME/.xflow", "/etc/xflow"]` | 기본 경로 탐색 위치 |
| `WithEnvPrefix(prefix)` | `"XFLOW"` | 환경 변수 접두사 |
| `WithFlags(flags)` | `nil` | pflag.FlagSet 바인딩 |
| `WithDefaults(fn)` | `nil` | 커스텀 기본값 설정 함수 |
| `WithLogger(logger)` | `nil` | slog.Logger 주입 |

### 타입 정의

| 타입 | 설명 |
|------|------|
| `ChangeCallback` | `func(event ChangeEvent)` - 변경 콜백 함수 |
| `UnsubscribeFunc` | `func()` - 콜백 구독 해제 함수 |
| `ChangeEvent` | Key, OldValue, NewValue, Source, Timestamp 필드 |
| `Configurable` | `Configure(cfg map[string]any) error` + `GetConfig() map[string]any` 인터페이스 |

## Mutable 키 목록

런타임에 `Set()`으로 변경할 수 있는 키이다.

| 키 | 타입 | 설명 |
|----|------|------|
| `engine.backpressure_threshold` | int (양수) | 백프레셔 임계값 |
| `engine.max_concurrent_flows` | int (양수) | 최대 동시 플로우 수 |
| `engine.execution_policy` | string | 실행 정책 ("parallel" / "sequential") |
| `observe.default_level` | string | 기본 로그 레벨 ("debug" / "info" / "warn" / "error") |
| `observe.metrics.enabled` | bool | 메트릭 활성화 |
| `observe.trace.enabled` | bool | 트레이스 활성화 |
| `server.cors.allowed_origins` | []string | CORS 허용 오리진 |
| `server.rate_limit.requests_per_second` | int (양수) | 초당 요청 제한 |
| `auth.jwt.access_ttl` | string (duration) | JWT 액세스 토큰 TTL |
| `auth.jwt.refresh_ttl` | string (duration) | JWT 리프레시 토큰 TTL |

### Immutable 와일드카드 패턴

| 패턴 | 설명 |
|------|------|
| `server.tls.*` | TLS 관련 설정 (재시작 필요) |

위 목록에 없는 모든 키는 기본적으로 Immutable이다.

## 유효성 검증 규칙

`Validate()` 함수는 8개 검증기를 실행하며, 모든 에러를 `ValidationErrors`로 집계하여 반환한다.

| 검증기 | 검증 대상 | 규칙 |
|--------|----------|------|
| `validatePort` | `server.port` | 1 ~ 65535 범위 |
| `validateStorageType` | `storage.type` | "sqlite" 또는 "postgres" |
| `validateLogLevel` | `observe.default_level` | "debug", "info", "warn", "error" |
| `validatePositiveValues` | 4개 키 | 양수 정수 (backpressure, flows, vm_pool, pool_size) |
| `validateDurations` | 3개 키 | `time.ParseDuration` 호환 문자열 |
| `validateTLS` | `server.tls.*` | TLS 활성화 시 cert_file, key_file 파일 존재 |
| `validateProductionJWT` | `auth.jwt.secret` | 프로덕션 모드에서 필수 |
| `validatePostgresDSN` | `storage.postgres.dsn` | postgres 타입에서 필수 |

## 센티널 에러

| 에러 변수 | 메시지 | 발생 조건 |
|-----------|--------|----------|
| `ErrInvalidPort` | port must be between 1 and 65535 | 유효하지 않은 포트 번호 |
| `ErrFileNotFound` | required file not found | TLS 인증서/키 파일 미존재 |
| `ErrRequiredField` | required field is missing | 필수 필드 미설정 |
| `ErrInvalidStorageType` | unsupported storage type | 잘못된 스토리지 타입 |
| `ErrInvalidLogLevel` | invalid log level | 잘못된 로그 레벨 |
| `ErrInvalidPositiveValue` | value must be positive | 양수가 아닌 값 |
| `ErrInvalidDuration` | invalid duration string | 잘못된 기간 문자열 |
| `ErrImmutableKey` | key is immutable at runtime | Immutable 키 런타임 변경 시도 |
| `ErrConfigNotLoaded` | configuration not loaded | 설정 미로드 상태 |

## 설계 특징

- **인터페이스 우선**: 모든 공개 API는 `Config` 인터페이스로 정의되며, 구현체(`viperConfig`)는 비공개
- **5단계 오버라이드 체인**: 코드 기본값 < 기본 경로 파일 < 사용자 파일 < 환경 변수 < CLI 플래그
- **동시성 안전**: `sync.RWMutex`로 설정 읽기/쓰기, `atomic.Int64`로 콜백 ID 생성
- **패닉 복구**: OnChange 콜백 내부 panic을 recover하여 다른 콜백에 영향 차단
- **디바운스**: fsnotify 파일 변경 이벤트 100ms 디바운스로 중복 호출 방지
- **에러 집계**: `ValidationErrors`가 모든 검증 에러를 수집하여 한 번에 보고
- **Options 패턴**: `Load()` 함수에 함수 옵션 패턴 적용
- **FIFO 이력**: 최대 100건 변경 이력 유지 (오래된 항목 자동 제거)

## 파일 구조

```
internal/config/
  errors.go            # 센티널 에러 정의 (9개) + ValidationErrors 타입
  types.go             # 설정 카테고리 구조체 (7개 카테고리, 13개 타입)
  mutable.go           # Mutable/Immutable 키 레지스트리 (IsMutable, IsImmutable)
  defaults.go          # SetDefaults 함수 (7개 카테고리 기본값)
  validate.go          # Validate 함수 (8개 검증기)
  config.go            # Config 인터페이스, viperConfig, Load(), 카테고리 접근자, Set/OnChange/WatchConfig
  config_test.go       # 통합 테스트 (87개 테스트, 4개 벤치마크)
  errors_test.go       # 센티널 에러 테스트
  types_test.go        # 카테고리 구조체 테스트
  mutable_test.go      # Mutable/Immutable 레지스트리 테스트
  defaults_test.go     # 기본값 테스트
  validate_test.go     # 유효성 검증 테스트
```

## 의존성

- **표준 라이브러리**: `sync`, `time`, `context`, `fmt`, `os`, `strings`, `log/slog`, `sync/atomic`
- **외부 의존성**:
  - `github.com/spf13/viper` v1.21.0 - 다중 소스 설정 관리
  - `github.com/spf13/cobra` v1.10.2 - CLI 프레임워크 (pflag 포함)
  - `github.com/fsnotify/fsnotify` v1.9.0 - 파일 변경 감시

## 테스트

```bash
# 전체 테스트 실행
go test ./internal/config/...

# Race Detector 포함 테스트
go test -race ./internal/config/...

# 커버리지 확인
go test -cover ./internal/config/...

# 벤치마크 실행
go test -bench=. -benchmem ./internal/config/...

# 상세 커버리지 리포트
go test -coverprofile=cover.out ./internal/config/...
go tool cover -html=cover.out
```

### 테스트 결과

- 테스트: 87개 전체 통과
- 커버리지: 96.1%
- Race Detector: 이상 없음
- 벤치마크: Load ~19K ops/s, Set ~3.3M ops/s
