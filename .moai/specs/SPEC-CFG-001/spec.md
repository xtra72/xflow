---
id: SPEC-CFG-001
version: "1.0.0"
status: implemented
created: "2026-02-12"
updated: "2026-02-12"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-12 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-CFG-001: Config System - Viper 기반 다중 소스 설정 관리 및 런타임 핫 리로드

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 애플리케이션 설정 관리 시스템을 정의한다. Config 패키지는 파일(YAML), 환경 변수, CLI 플래그의 3가지 소스에서 설정을 통합 로딩하며, Viper 라이브러리를 기반으로 설정 읽기/쓰기/감시를 수행한다. 런타임 중 설정 변경(핫 리로드)을 지원하고, 변경 사항을 구성 요소별 콜백으로 전파한다. 모든 내부 패키지(`internal/engine`, `internal/node`, `internal/agent`, `internal/script`, `internal/plugin`, `internal/api`, `internal/auth`, `internal/storage`, `internal/observe`)와 CLI/서버 엔트리포인트(`cmd/xflowd`, `cmd/xflow`, `cmd/xflow-agent`)가 이 패키지에 의존한다.

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/config/`
- **핵심 의존성**:
  - `github.com/spf13/viper` v1.18+ (다중 소스 설정 관리)
  - `github.com/spf13/cobra` v1.8+ (CLI 플래그 통합)
- **내부 의존성**: `internal/observe` (로깅, SPEC-OBS-001에 의존)
- **Tier**: Tier 2 - 횡단 관심사 인프라
- **테스트 프레임워크**: Go 표준 `testing` + `github.com/stretchr/testify`

### 1.3 설계 원칙

- **인터페이스 우선**: 모든 공개 API는 인터페이스로 정의하며, 구현체는 unexported
- **캡슐화**: Viper 인스턴스를 내부에 감추고, 타입 안전한 접근자(accessor) 제공
- **Options Pattern**: `Load()` 함수에 유연한 설정 옵션 제공
- **동시성 안전**: `sync.RWMutex` 기반으로 설정 읽기는 동시 접근 허용, 쓰기는 배타적 잠금
- **Mutable/Immutable 구분**: 런타임 변경 가능한 설정과 재시작이 필요한 설정을 명확히 구분
- **콜백 기반 전파**: 설정 변경 시 등록된 콜백 함수를 통해 관련 구성 요소에 알림

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: Viper v1.18+의 `WatchConfig()`는 파일 시스템 변경 이벤트(fsnotify)를 안정적으로 감지한다
- A2: 설정 파일은 YAML 포맷만 지원한다 (Viper는 JSON, TOML 등도 지원하지만, 표준 포맷을 YAML로 통일)
- A3: 환경 변수는 `XFLOW_` 접두사를 사용하며, 중첩 키는 `_`로 구분한다 (예: `XFLOW_SERVER_PORT`)
- A4: CLI 플래그는 Cobra의 PersistentFlags와 바인딩되며, Viper의 `BindPFlags()`를 통해 통합된다
- A5: 설정 로딩 우선순위는 CLI 플래그 > 환경 변수 > 사용자 지정 설정 파일 > 기본 경로 설정 파일 > 기본값 순이다
- A6: 각 바이너리(xflowd, xflow, xflow-agent)는 고유한 기본 설정 파일 경로를 가진다
  - **xflowd**: `./xflowd.yaml`, `$HOME/.xflow/xflowd.yaml`, `/etc/xflow/xflowd.yaml`
  - **xflow**: `./xflow.yaml`, `$HOME/.xflow/xflow.yaml`, `/etc/xflow/xflow.yaml`
  - **xflow-agent**: `./xflow-agent.yaml`, `$HOME/.xflow/xflow-agent.yaml`, `/etc/xflow/xflow-agent.yaml`
- A6-1: 기본 경로의 설정 파일이 존재하면 기본값 위에 오버라이딩되고, 사용자가 `--config` 플래그로 설정 파일을 지정하면 기본 경로 설정 파일 위에 다시 오버라이딩된다
- A7: `internal/observe` 패키지가 초기화되기 전에도 설정 로딩이 가능해야 한다 (로깅은 선택적)

### 2.2 비즈니스 가정

- A8: 서버 설정(포트, TLS)은 시작 시에만 적용되며, 런타임 변경은 재시작을 요구한다
- A9: 플로우/노드/에이전트별 설정 변경은 REST API를 통해 런타임에 수행되며, 이 SPEC은 설정 변경 전파 메커니즘만 담당한다 (API 핸들러는 `internal/api` 패키지의 책임)
- A10: 설정 변경 이력은 메모리에 유한한 크기(기본 100건)로 유지하며, 영속적 감사 로그는 `internal/observe`에 위임한다
- A11: 모든 설정값은 시작 시 유효성 검증을 통과해야 하며, 유효하지 않은 설정은 서버 시작을 차단한다

---

## 3. Requirements (요구사항)

### Module 1: Core - 설정 구조체 및 인터페이스

#### REQ-CFG-001-01-01 (Ubiquitous) Config 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Config` 인터페이스를 제공해야 한다:

- `Server() ServerConfig` - 서버 설정 반환 (포트, TLS, CORS, 레이트 리밋)
- `Engine() EngineConfig` - Flow Engine 설정 반환 (백프레셔 임계값, 실행 정책)
- `Storage() StorageConfig` - 저장소 설정 반환 (DB 타입, 연결 문자열)
- `Auth() AuthConfig` - 인증 설정 반환 (JWT, API Key, OAuth2)
- `Observe() ObserveConfig` - 관찰성 설정 반환 (로그 레벨, 메트릭, 트레이싱)
- `Script() ScriptConfig` - 스크립트 엔진 설정 반환 (타임아웃, VM 풀 크기)
- `Plugin() PluginConfig` - 플러그인 설정 반환 (디렉토리, 활성화 목록)

#### REQ-CFG-001-01-02 (Ubiquitous) Config 기본 구현체

시스템은 **항상** `viperConfig`(unexported struct)를 `Config` 인터페이스의 기본 구현체로 사용해야 한다. 내부에 `*viper.Viper` 인스턴스와 `sync.RWMutex`를 보유한다.

#### REQ-CFG-001-01-03 (Ubiquitous) Load 생성자

시스템은 **항상** `Load(opts ...LoadOption) (Config, error)` 팩토리 함수를 제공해야 한다.

- 반환 타입은 `Config` 인터페이스이다
- 설정 파일, 환경 변수, CLI 플래그를 통합 로딩한다
- 기본값을 먼저 설정한 후 외부 소스를 오버라이드한다
- 로딩 후 자동으로 유효성 검증을 수행한다
- 검증 실패 시 상세한 에러를 반환한다

#### REQ-CFG-001-01-04 (Ubiquitous) LoadOption 타입

시스템은 **항상** 다음 LoadOption 함수들을 제공해야 한다:

- `type LoadOption func(*loadConfig)` - 설정 함수 타입
- `WithConfigFile(path string) LoadOption` - 사용자 지정 설정 파일 경로 (기본 경로 설정 파일 위에 오버라이딩)
- `WithConfigName(name string) LoadOption` - 기본 설정 파일명 (바이너리별: `xflowd`, `xflow`, `xflow-agent`)
- `WithConfigPaths(paths ...string) LoadOption` - 기본 설정 파일 탐색 경로 (기본: `./`, `$HOME/.xflow/`, `/etc/xflow/`)
- `WithEnvPrefix(prefix string) LoadOption` - 환경 변수 접두사 (기본: `XFLOW`)
- `WithFlags(flags *pflag.FlagSet) LoadOption` - CLI 플래그셋 바인딩
- `WithDefaults(fn func(*viper.Viper)) LoadOption` - 커스텀 기본값 설정 함수
- `WithLogger(logger *slog.Logger) LoadOption` - 로거 주입 (선택적)

#### REQ-CFG-001-01-05 (Ubiquitous) 설정 카테고리 구조체

시스템은 **항상** 다음 설정 카테고리 구조체(exported, 읽기 전용 값 타입)를 제공해야 한다:

- `ServerConfig` - HTTP 포트, TLS 인증서 경로, CORS 설정, 레이트 리밋
- `EngineConfig` - 백프레셔 임계값, 실행 정책, 최대 동시 플로우
- `StorageConfig` - DB 타입(sqlite/postgres), 연결 문자열, 풀 크기
- `AuthConfig` - JWT 시크릿, 토큰 만료 시간, API 키 활성화, OAuth2 프로바이더
- `ObserveConfig` - 기본 로그 레벨, 메트릭 활성화, 트레이스 활성화
- `ScriptConfig` - 스크립트 타임아웃, VM 풀 크기, 샌드박스 설정
- `PluginConfig` - 플러그인 디렉토리, WASM 활성화, Go 플러그인 활성화

---

### Module 2: Defaults - 기본값 정의

#### REQ-CFG-001-02-01 (Ubiquitous) 서버 기본값

시스템은 **항상** 다음의 서버 기본값을 제공해야 한다:

| 키 | 기본값 | 설명 |
|----|--------|------|
| `server.port` | `8080` | HTTP 포트 |
| `server.host` | `0.0.0.0` | 바인딩 호스트 |
| `server.tls.enabled` | `false` | TLS 비활성화 |
| `server.cors.enabled` | `true` | CORS 활성화 |
| `server.cors.allowed_origins` | `["*"]` | 허용 오리진 |
| `server.rate_limit.enabled` | `true` | 레이트 리밋 활성화 |
| `server.rate_limit.requests_per_second` | `100` | 초당 요청 제한 |

#### REQ-CFG-001-02-02 (Ubiquitous) 엔진 기본값

시스템은 **항상** 다음의 엔진 기본값을 제공해야 한다:

| 키 | 기본값 | 설명 |
|----|--------|------|
| `engine.backpressure_threshold` | `1000` | 백프레셔 임계값 (메시지 수) |
| `engine.max_concurrent_flows` | `100` | 최대 동시 플로우 |
| `engine.execution_policy` | `parallel` | 실행 정책 (parallel/sequential) |
| `engine.wire_default_buffer` | `0` | Wire 기본 버퍼 크기 (0=바이패스) |

#### REQ-CFG-001-02-03 (Ubiquitous) 저장소, 인증, 관찰성, 스크립트, 플러그인 기본값

시스템은 **항상** 다음의 기본값을 제공해야 한다:

| 키 | 기본값 | 설명 |
|----|--------|------|
| `storage.type` | `sqlite` | DB 타입 |
| `storage.sqlite.path` | `./data/xflow.db` | SQLite 파일 경로 |
| `storage.pool_size` | `10` | 연결 풀 크기 |
| `auth.jwt.secret` | `""` (필수 설정) | JWT 시크릿 |
| `auth.jwt.access_ttl` | `15m` | 액세스 토큰 만료 |
| `auth.jwt.refresh_ttl` | `168h` | 리프레시 토큰 만료 (7일) |
| `auth.api_key.enabled` | `true` | API 키 활성화 |
| `observe.default_level` | `info` | 기본 로그 레벨 |
| `observe.metrics.enabled` | `true` | 메트릭 수집 활성화 |
| `observe.trace.enabled` | `false` | 트레이싱 비활성화 |
| `script.timeout` | `5s` | 스크립트 실행 타임아웃 |
| `script.vm_pool_size` | `10` | Lua VM 풀 크기 |
| `script.sandbox.enabled` | `true` | 샌드박스 활성화 |
| `plugin.directory` | `./plugins` | 플러그인 디렉토리 |
| `plugin.wasm.enabled` | `true` | WASM 플러그인 활성화 |
| `plugin.go.enabled` | `true` | Go 플러그인 활성화 |

---

### Module 3: Validate - 설정 유효성 검증

#### REQ-CFG-001-03-01 (Ubiquitous) Validate 함수

시스템은 **항상** `Validate(cfg Config) error` 함수를 제공하여, 로딩된 설정의 전체 유효성을 검증해야 한다. 복수의 검증 에러가 발생하면 모든 에러를 집계하여 반환한다.

#### REQ-CFG-001-03-02 (Event-Driven) 포트 범위 검증

**WHEN** 설정 로딩 시 `server.port` 값이 1~65535 범위를 벗어나면, **THEN** `ErrInvalidPort` 에러를 반환해야 한다.

#### REQ-CFG-001-03-03 (Event-Driven) 경로 존재 검증

**WHEN** TLS가 활성화(`server.tls.enabled=true`)되고, 인증서 파일 경로(`server.tls.cert_file`, `server.tls.key_file`)가 존재하지 않으면, **THEN** `ErrFileNotFound` 에러를 반환해야 한다.

#### REQ-CFG-001-03-04 (Event-Driven) 필수값 검증

**WHEN** 프로덕션 모드(`server.mode=production`)에서 `auth.jwt.secret`이 비어 있으면, **THEN** `ErrRequiredField` 에러를 반환해야 한다.

#### REQ-CFG-001-03-05 (Event-Driven) 저장소 타입 검증

**WHEN** `storage.type`이 `sqlite` 또는 `postgres` 이외의 값이면, **THEN** `ErrInvalidStorageType` 에러를 반환해야 한다.

#### REQ-CFG-001-03-06 (Event-Driven) PostgreSQL 연결 문자열 검증

**WHEN** `storage.type=postgres`이고 `storage.postgres.dsn`이 비어 있으면, **THEN** `ErrRequiredField` 에러를 반환해야 한다.

#### REQ-CFG-001-03-07 (Event-Driven) 로그 레벨 검증

**WHEN** `observe.default_level`이 `debug`, `info`, `warn`, `error` 이외의 값이면, **THEN** `ErrInvalidLogLevel` 에러를 반환해야 한다.

#### REQ-CFG-001-03-08 (Event-Driven) 양수값 검증

**WHEN** `engine.backpressure_threshold`, `engine.max_concurrent_flows`, `script.vm_pool_size`, `storage.pool_size` 중 하나가 0 이하이면, **THEN** `ErrInvalidPositiveValue` 에러를 반환해야 한다.

#### REQ-CFG-001-03-09 (Event-Driven) Duration 파싱 검증

**WHEN** `auth.jwt.access_ttl`, `auth.jwt.refresh_ttl`, `script.timeout` 값이 유효한 Go `time.Duration` 문자열이 아니면, **THEN** `ErrInvalidDuration` 에러를 반환해야 한다.

#### REQ-CFG-001-03-10 (Ubiquitous) 검증 에러 집계

시스템은 **항상** 검증 중 발생한 모든 에러를 `ValidationErrors` 타입으로 집계하여, 첫 번째 에러에서 중단하지 않고 전체 검증을 완료해야 한다.

---

### Module 4: HotReload - 런타임 설정 변경 및 알림

#### REQ-CFG-001-04-01 (Ubiquitous) Configurable 인터페이스

시스템은 **항상** 런타임 설정 변경을 수용하는 구성 요소를 위한 `Configurable` 인터페이스를 제공해야 한다:

- `Configure(cfg map[string]any) error` - 설정 변경 적용
- `GetConfig() map[string]any` - 현재 설정 조회

#### REQ-CFG-001-04-02 (Ubiquitous) 변경 콜백 등록

시스템은 **항상** `OnChange(key string, fn ChangeCallback) UnsubscribeFunc` 메서드를 Config 인터페이스에 제공하여, 특정 설정 키의 변경 시 콜백을 호출해야 한다.

- `type ChangeCallback func(event ChangeEvent)` - 콜백 함수 타입
- `type UnsubscribeFunc func()` - 구독 해제 함수
- `ChangeEvent`는 `Key`, `OldValue`, `NewValue`, `Timestamp` 필드를 포함한다

#### REQ-CFG-001-04-03 (Event-Driven) 파일 변경 감지

**WHEN** 설정 파일이 외부에서 수정되면, **THEN** Viper `WatchConfig()`를 통해 변경을 감지하고, 등록된 콜백을 호출해야 한다.

#### REQ-CFG-001-04-04 (Event-Driven) API 기반 설정 변경

**WHEN** `Set(key string, value any) error` 메서드를 통해 설정 값이 변경되면, **THEN** 변경된 키에 등록된 모든 콜백을 비동기로 호출하고, 변경 이력에 기록해야 한다.

#### REQ-CFG-001-04-05 (State-Driven) Mutable/Immutable 설정 구분

**IF** 설정 키가 Immutable로 분류된 경우(포트, TLS 인증서, Transport 유형, VM 풀 크기, 플러그인 바이너리), **THEN** `Set()` 호출 시 `ErrImmutableKey` 에러를 반환해야 한다.

Mutable 키 목록:
- `engine.backpressure_threshold` - 백프레셔 임계값
- `observe.default_level` - 기본 로그 레벨
- `observe.metrics.enabled` - 메트릭 활성화
- `server.cors.allowed_origins` - CORS 허용 오리진
- `server.rate_limit.requests_per_second` - 레이트 리밋

Immutable 키 목록 (예시):
- `server.port` - HTTP 포트
- `server.host` - 바인딩 호스트
- `server.tls.*` - TLS 관련 모든 설정
- `storage.type` - 저장소 타입
- `script.vm_pool_size` - VM 풀 크기

#### REQ-CFG-001-04-06 (Event-Driven) 런타임 변경 검증

**WHEN** `Set()` 메서드로 설정이 변경되면, **THEN** 변경될 키에 대해 유효성 검증을 수행하고, 검증 실패 시 변경을 취소하고 에러를 반환해야 한다.

#### REQ-CFG-001-04-07 (Ubiquitous) 변경 이력 추적

시스템은 **항상** `ChangeHistory() []ChangeEvent` 메서드를 제공하여, 최근 설정 변경 이력을 반환해야 한다. 이력은 메모리에 최대 `maxChangeHistory`(기본 100건)까지 유지하며, FIFO 방식으로 관리한다.

#### REQ-CFG-001-04-08 (Event-Driven) 파일 감시 시작/중지

**WHEN** `WatchConfig() error` 메서드가 호출되면, **THEN** Viper의 파일 감시를 시작하고, 이미 감시 중인 경우 에러 없이 무시해야 한다.

**WHEN** `StopWatch()` 메서드가 호출되면, **THEN** 파일 감시를 중지하고, 리소스를 정리해야 한다.

#### REQ-CFG-001-04-09 (Unwanted) 콜백 패닉 전파 금지

시스템은 콜백 함수 내에서 발생한 panic을 **전파하지 않아야 한다**. panic은 recover하여 로그에 기록하고, 다른 콜백의 실행을 중단하지 않아야 한다.

---

### Module 5: Concurrency - 동시성 안전 및 에러 처리

#### REQ-CFG-001-05-01 (Ubiquitous) 읽기 동시성 안전

시스템은 **항상** `Config` 인터페이스의 읽기 메서드(`Server()`, `Engine()`, `Storage()` 등)가 다수의 고루틴에서 동시에 호출되더라도 안전하게 동작해야 한다. `sync.RWMutex`의 `RLock()`을 사용한다.

#### REQ-CFG-001-05-02 (Ubiquitous) 쓰기 배타적 잠금

시스템은 **항상** `Set()` 메서드 호출 시 `sync.RWMutex`의 `Lock()`을 사용하여 배타적 잠금을 획득해야 한다. 읽기 중인 고루틴은 쓰기 완료 후 최신 값을 읽을 수 있다.

#### REQ-CFG-001-05-03 (Ubiquitous) 에러 타입 정의

시스템은 **항상** 다음 에러 변수를 제공해야 한다:

| 에러 변수 | 설명 |
|-----------|------|
| `ErrInvalidPort` | 포트 번호가 유효 범위(1-65535)를 벗어남 |
| `ErrFileNotFound` | 필수 파일 경로가 존재하지 않음 |
| `ErrRequiredField` | 필수 설정 값이 누락됨 |
| `ErrInvalidStorageType` | 지원하지 않는 저장소 타입 |
| `ErrInvalidLogLevel` | 유효하지 않은 로그 레벨 |
| `ErrInvalidPositiveValue` | 양수가 필요한 필드에 0 이하의 값 |
| `ErrInvalidDuration` | 유효하지 않은 Duration 문자열 |
| `ErrImmutableKey` | 런타임에 변경 불가능한 설정 키 |
| `ErrConfigNotLoaded` | 설정이 아직 로딩되지 않음 |

#### REQ-CFG-001-05-04 (Ubiquitous) ValidationErrors 타입

시스템은 **항상** `ValidationErrors` 타입을 제공하여, 복수의 검증 에러를 하나의 에러로 집계해야 한다:

- `Error() string` - 모든 에러를 줄바꿈으로 연결한 문자열 반환
- `Errors() []error` - 개별 에러 슬라이스 반환
- `HasErrors() bool` - 에러 존재 여부 확인

---

## 4. Specifications (명세)

### 4.1 패키지 구조

```
internal/config/
  config.go        # Config 인터페이스, viperConfig 구현, Load(), LoadOption
  types.go         # ServerConfig, EngineConfig 등 카테고리 구조체 정의
  defaults.go      # SetDefaults() 기본값 설정 함수
  validate.go      # Validate(), ValidationErrors, 개별 검증 함수
  hotreload.go     # OnChange, Set, WatchConfig, ChangeEvent, ChangeHistory
  mutable.go       # Mutable/Immutable 키 분류 레지스트리
  errors.go        # 에러 변수 정의
  config_test.go   # 전체 단위 테스트
```

### 4.2 인터페이스 시그니처 요약

```go
// Config - 설정 접근 인터페이스
type Config interface {
    // 카테고리별 설정 접근
    Server() ServerConfig
    Engine() EngineConfig
    Storage() StorageConfig
    Auth() AuthConfig
    Observe() ObserveConfig
    Script() ScriptConfig
    Plugin() PluginConfig

    // 런타임 변경
    Set(key string, value any) error
    Get(key string) any
    IsSet(key string) bool

    // 변경 알림
    OnChange(key string, fn ChangeCallback) UnsubscribeFunc
    ChangeHistory() []ChangeEvent

    // 파일 감시
    WatchConfig() error
    StopWatch()
}

// Configurable - 런타임 설정 변경 수용 인터페이스
type Configurable interface {
    Configure(cfg map[string]any) error
    GetConfig() map[string]any
}

// ChangeCallback - 변경 콜백 함수 타입
type ChangeCallback func(event ChangeEvent)

// UnsubscribeFunc - 구독 해제 함수
type UnsubscribeFunc func()
```

### 4.3 데이터 구조체

```go
// ChangeEvent - 설정 변경 이벤트
type ChangeEvent struct {
    Key       string    // 변경된 설정 키
    OldValue  any       // 이전 값
    NewValue  any       // 새 값
    Source    string    // 변경 소스 ("file", "api", "env")
    Timestamp time.Time // 변경 시각
}

// ValidationErrors - 검증 에러 집계
type ValidationErrors struct {
    errors []error
}
```

### 4.4 설정 파일 스키마 (YAML)

```yaml
server:
  port: 8080
  host: "0.0.0.0"
  mode: "development"   # development | production
  tls:
    enabled: false
    cert_file: ""
    key_file: ""
  cors:
    enabled: true
    allowed_origins: ["*"]
  rate_limit:
    enabled: true
    requests_per_second: 100

engine:
  backpressure_threshold: 1000
  max_concurrent_flows: 100
  execution_policy: "parallel"
  wire_default_buffer: 0

storage:
  type: "sqlite"
  sqlite:
    path: "./data/xflow.db"
  postgres:
    dsn: ""
  pool_size: 10

auth:
  jwt:
    secret: ""
    access_ttl: "15m"
    refresh_ttl: "168h"
  api_key:
    enabled: true
  oauth2:
    providers: []

observe:
  default_level: "info"
  metrics:
    enabled: true
  trace:
    enabled: false

script:
  timeout: "5s"
  vm_pool_size: 10
  sandbox:
    enabled: true
    max_memory_mb: 64
    max_execution_ms: 5000

plugin:
  directory: "./plugins"
  wasm:
    enabled: true
  go:
    enabled: true
```

### 4.5 Mutable/Immutable 키 레지스트리

| 분류 | 키 패턴 | 런타임 변경 |
|------|---------|------------|
| Immutable | `server.port`, `server.host`, `server.tls.*` | 재시작 필요 |
| Immutable | `storage.type`, `storage.sqlite.path`, `storage.postgres.dsn` | 재시작 필요 |
| Immutable | `script.vm_pool_size` | 재시작 필요 |
| Immutable | `plugin.directory` | 재시작 필요 |
| Mutable | `engine.backpressure_threshold` | 즉시 반영 |
| Mutable | `engine.execution_policy` | 즉시 반영 |
| Mutable | `observe.default_level` | 즉시 반영 |
| Mutable | `observe.metrics.enabled`, `observe.trace.enabled` | 즉시 반영 |
| Mutable | `server.cors.allowed_origins` | 즉시 반영 |
| Mutable | `server.rate_limit.requests_per_second` | 즉시 반영 |
| Mutable | `auth.jwt.access_ttl`, `auth.jwt.refresh_ttl` | 즉시 반영 |

### 4.6 설정 우선순위

각 바이너리(xflowd, xflow, xflow-agent)는 동일한 5단계 설정 오버라이딩 체계를 따른다:

```
CLI 플래그 (최우선)
    ↓
환경 변수 (XFLOW_ 접두사)
    ↓
사용자 지정 설정 파일 (--config 플래그로 지정)
    ↓
기본 경로 설정 파일 (바이너리별 기본 경로에서 탐색)
    ↓
기본값 (코드 내 하드코딩된 기본값, 최하위)
```

#### 바이너리별 기본 설정 경로

| 바이너리 | 기본 설정 파일명 | 탐색 경로 (우선순위 순) |
|---------|----------------|----------------------|
| xflowd | `xflowd.yaml` | `./` → `$HOME/.xflow/` → `/etc/xflow/` |
| xflow | `xflow.yaml` | `./` → `$HOME/.xflow/` → `/etc/xflow/` |
| xflow-agent | `xflow-agent.yaml` | `./` → `$HOME/.xflow/` → `/etc/xflow/` |

#### 오버라이딩 예시

1. `xflowd`가 `Port: 8080` 기본값을 갖고 있다
2. `$HOME/.xflow/xflowd.yaml`에 `server.port: 9090`이 있으면 → 포트가 9090으로 오버라이딩
3. `xflowd --config /opt/xflow/custom.yaml`로 실행하고, 해당 파일에 `server.port: 3000`이 있으면 → 포트가 3000으로 오버라이딩
4. `XFLOW_SERVER_PORT=4000` 환경 변수가 설정되어 있으면 → 포트가 4000으로 오버라이딩
5. `xflowd --port 5000` CLI 플래그가 있으면 → 포트가 5000으로 최종 결정

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 카테고리 | 검증 방법 |
|-------------|------|----------|-----------|
| REQ-CFG-001-01-01 ~ 01-05 | Core | Ubiquitous | 단위 테스트 (인터페이스 구현 검증) |
| REQ-CFG-001-02-01 ~ 02-03 | Defaults | Ubiquitous | 단위 테스트 (기본값 확인) |
| REQ-CFG-001-03-01 ~ 03-10 | Validate | Event-Driven/Ubiquitous | 단위 테스트 (유효/무효 입력) |
| REQ-CFG-001-04-01 ~ 04-09 | HotReload | Event-Driven/State-Driven/Unwanted | 통합 테스트 (콜백 호출, 변경 이력) |
| REQ-CFG-001-05-01 ~ 05-04 | Concurrency | Ubiquitous | 단위 테스트 (`-race` 플래그), 벤치마크 |
