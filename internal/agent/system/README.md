# system - Store 시스템 에이전트

`internal/agent/system` 패키지는 xflow 엔진의 키-값 저장소 시스템 에이전트를 제공한다. sync.Map 기반 인메모리 저장소, Write-Through 캐시 영속 저장소, 네임스페이스 격리, 이중 TTL 만료(lazy expiration + 백그라운드 스캔), Bridge Node 메시지 프로토콜을 통합 지원한다.

**SPEC**: SPEC-STORE-001

## 아키텍처 개요

```
    3계층 합성 아키텍처 (Composition)

    +---------------------------------------------------------+
    |                    StoreAgent                            |
    |  BaseLifecycle 임베딩 (7상태 생명주기)                     |
    |  Lifecycle + Configurable + HealthChecker                |
    +---------------------------------------------------------+
            |                             |
    +-------v--------+           +--------v---------+
    |  ForNamespace() |           |  BridgeHandler   |
    |  팩토리 메서드    |           |  메시지 디스패처   |
    +----------------+           +------------------+
            |
    +-------v---------+
    | NamespacedStore  |  3계층: "{namespace}:{key}" 접두사 데코레이터
    +-----------------+
            |
    +-------v---------+
    |   agentStore     |  2계층: paused/closed 상태 확인 래퍼
    +-----------------+
            |
    +-------v---------+        +--------------------+
    | VolatileStore    |  또는  |  PersistentStore   |
    | (sync.Map 기반)  |        | (Write-Through     |
    |                  |        |  캐시 + Repository) |
    +------------------+        +--------------------+
            |
    +-------v---------+
    |   ttlManager     |  백그라운드 만료 스캔 (atomic.Bool pause)
    +-----------------+
```

**핵심 구성 요소**:

1. **Store 인터페이스**: 7개 메서드 (Get/Set/SetWithTTL/Delete/Has/Keys/Clear)
2. **StoreAgent**: BaseLifecycle 임베딩, Lifecycle/Configurable/HealthChecker 구현
3. **VolatileStore**: sync.Map 기반 인메모리 저장소 (읽기 최적화)
4. **PersistentStore**: Write-Through 캐시 + StoreRepository (JSON 직렬화)
5. **NamespacedStore**: 데코레이터 패턴 네임스페이스 격리 ("{namespace}:{key}")
6. **ttlManager**: 백그라운드 만료 스캔 (atomic.Bool, time.Ticker)
7. **BridgeHandler**: Bridge Node 메시지 기반 Store 접근 디스패처

## 빠른 시작

### StoreAgent 생성 및 초기화

`NewStoreAgent()` 함수는 Options 패턴으로 설정을 받아 StoreAgent를 생성한다. `Init()`으로 초기화하면 VolatileStore와 ttlManager가 시작된다.

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/xtra/xflow/internal/agent/system"
)

func main() {
    ctx := context.Background()

    // StoreAgent 생성 (Options 패턴)
    agent := system.NewStoreAgent(
        system.WithDefaultTTL(5 * time.Minute),      // 기본 TTL
        system.WithScanInterval(500 * time.Millisecond), // TTL 스캔 주기
        system.WithMaxKeyLength(1024),                  // 최대 키 길이
    )

    // 초기화 (Created → Initializing → Running)
    if err := agent.Init(ctx); err != nil {
        panic(err)
    }
    defer agent.Stop(ctx)
}
```

### 네임스페이스별 Store 사용

`ForNamespace()` 팩토리 메서드로 네임스페이스별 Store를 획득한다. 각 네임스페이스는 독립적인 키 공간을 가진다.

```go
// 네임스페이스별 Store 획득
flowStore := agent.ForNamespace("flow-abc")
sensorStore := agent.ForNamespace("sensor-data")

// 값 저장
err := flowStore.Set(ctx, "temperature", 25.5)

// TTL 포함 저장
err = sensorStore.SetWithTTL(ctx, "humidity", 60.0, 30*time.Second)

// 값 조회
entry, err := flowStore.Get(ctx, "temperature")
fmt.Println("온도:", entry.Value)         // 25.5
fmt.Println("생성:", entry.CreatedAt)      // 최초 생성 시각
fmt.Println("네임스페이스:", entry.Namespace) // "flow-abc"

// 키 존재 확인
exists, err := flowStore.Has(ctx, "temperature")

// 패턴 기반 키 검색 (path.Match 형식)
keys, err := sensorStore.Keys(ctx, "hum*")

// 네임스페이스 전체 삭제 (다른 네임스페이스에 영향 없음)
err = sensorStore.Clear(ctx)
```

### Bridge Node 메시지 프로토콜

BridgeHandler를 사용하면 Bridge Node를 통해 메시지 기반으로 Store에 접근할 수 있다.

```go
handler := system.NewBridgeHandler(agent)

// Set 연산 메시지 구성
msg := message.New()
msg.Metadata().Set("store.operation", "set")
msg.Metadata().Set("store.key", "temperature")
msg.Metadata().Set("store.namespace", "sensor-data")
msg.Metadata().Set("store.ttl", "30s") // 선택적 TTL
msg.Payload().Set("value", 25.5)

resp, err := handler.HandleMessage(ctx, msg)
status, _ := resp.Metadata().Get("store.status") // "ok" 또는 "error"
```

## API 레퍼런스

### Store 인터페이스

```go
type Store interface {
    Get(ctx context.Context, key string) (StoreEntry, error)
    Set(ctx context.Context, key string, value any) error
    SetWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error
    Delete(ctx context.Context, key string) error
    Has(ctx context.Context, key string) (bool, error)
    Keys(ctx context.Context, pattern string) ([]string, error)
    Clear(ctx context.Context) error
}
```

### StoreEntry 구조체

| 필드 | 타입 | 설명 |
|------|------|------|
| `Value` | `any` | 저장된 값 |
| `TTL` | `time.Duration` | 남은 유효 시간 (0이면 만료 없음) |
| `CreatedAt` | `time.Time` | 최초 생성 시각 |
| `UpdatedAt` | `time.Time` | 마지막 갱신 시각 |
| `Namespace` | `string` | 소속 네임스페이스 |
| `ExpiresAt` | `time.Time` | 만료 예정 시각 (zero value면 만료 없음) |

### StoreRepository 인터페이스

영속 저장소 백엔드를 추상화하는 인터페이스이다.

```go
type StoreRepository interface {
    GetEntry(ctx context.Context, key string) (*StoreEntry, error)
    SetEntry(ctx context.Context, key string, entry *StoreEntry) error
    DeleteEntry(ctx context.Context, key string) error
    ListKeys(ctx context.Context, pattern string) ([]string, error)
    DeleteExpired(ctx context.Context, before time.Time) (int, error)
    ClearNamespace(ctx context.Context, namespace string) error
}
```

### StoreOption 옵션

| 옵션 함수 | 기본값 | 설명 |
|-----------|--------|------|
| `WithBackend(backend)` | `"volatile"` | 저장소 백엔드 ("volatile" / "persistent") |
| `WithDefaultTTL(ttl)` | `0` (만료 없음) | 기본 TTL |
| `WithScanInterval(interval)` | `1s` | TTL 만료 스캔 주기 |
| `WithRepository(repo)` | `nil` | 영속 저장소 리포지토리 |
| `WithMaxKeyLength(length)` | `512` | 최대 키 길이 (바이트) |

### StoreAgent 메서드

| 메서드 | 설명 |
|--------|------|
| `NewStoreAgent(opts ...StoreOption)` | Options 패턴으로 StoreAgent 생성 |
| `Init(ctx)` | Created -> Initializing -> Running 전이 |
| `Start(ctx)` | Running 상태에서 no-op |
| `Pause(ctx)` | Running -> Paused (쓰기 비활성화, 읽기 허용) |
| `Resume(ctx)` | Paused -> Running |
| `Stop(ctx)` | Running/Paused -> Stopping -> Stopped |
| `State()` | 현재 생명주기 상태 반환 |
| `Configure(ctx, cfg)` | Running/Paused에서 런타임 설정 변경 |
| `GetConfig()` | 현재 설정을 map으로 반환 |
| `HealthCheck(ctx)` | 건강 상태 반환 (키 수 포함) |
| `ForNamespace(namespace)` | 네임스페이스별 Store 팩토리 |

### Configure 지원 키

| 키 | 타입 | 설명 |
|----|------|------|
| `ttl_scan_interval` | `string` (duration) | TTL 만료 스캔 주기 변경 |
| `default_ttl` | `string` (duration) | 기본 TTL 변경 |

### BridgeHandler 메시지 프로토콜

**요청 메타데이터**:

| 메타데이터 키 | 필수 | 설명 |
|--------------|------|------|
| `store.operation` | 필수 | 연산 종류 ("get"/"set"/"delete"/"has"/"keys"/"clear") |
| `store.key` | 연산별 | 대상 키 |
| `store.ttl` | 선택 | TTL 기간 문자열 (set 연산용, 예: "30s") |
| `store.namespace` | 선택 | 네임스페이스 (기본값: "default") |
| `store.pattern` | 선택 | keys 연산용 패턴 |

**요청 Payload**: set 연산 시 `value` 필드에 저장할 값 포함

**응답 메타데이터**:

| 메타데이터 키 | 설명 |
|--------------|------|
| `store.status` | "ok" 또는 "error" |
| `store.error` | 에러 메시지 (status가 "error"일 때) |

**응답 Payload**:

| 연산 | Payload 필드 | 설명 |
|------|-------------|------|
| `get` | `value` | 조회된 값 |
| `has` | `exists` | 존재 여부 (bool) |
| `keys` | `keys` | 매칭된 키 목록 ([]string) |

## 저장소 백엔드

### VolatileStore (인메모리)

sync.Map 기반 인메모리 저장소이다. 읽기 빈도가 높은 워크로드에 최적화되어 있다.

- **자료구조**: `sync.Map` (Go 내장 동시성 안전 맵)
- **만료 전략**: lazy expiration (Get 시 만료 확인 및 삭제)
- **키 검색**: `path.Match` glob 패턴 지원 ("sensor-*", "flow-?")
- **키 길이 제한**: 설정 가능 (기본 512바이트)
- **nil 값 방지**: ErrNilValue 반환
- **CreatedAt 보존**: Set 시 기존 키의 CreatedAt/ExpiresAt 유지

### PersistentStore (Write-Through 캐시)

sync.Map 캐시와 StoreRepository 백엔드를 조합한 Write-Through 캐시 패턴이다.

- **캐시**: sync.Map (읽기 최적화)
- **백엔드**: StoreRepository 인터페이스 (SQLite/PostgreSQL 등)
- **직렬화**: JSON (encoding/json)
- **Write-Through**: 캐시와 리포지토리에 동시 기록
- **롤백**: 리포지토리 실패 시 캐시 자동 롤백
- **캐시 적재**: `LoadFromRepo()`로 리포지토리에서 캐시로 일괄 로드

### NamespacedStore (네임스페이스 격리)

데코레이터 패턴으로 내부 Store에 네임스페이스 접두사를 자동 적용한다.

- **키 변환**: `"{namespace}:{key}"` 형식으로 내부 키 생성
- **격리**: 각 네임스페이스는 독립적인 키 공간 보유
- **Clear 범위**: 해당 네임스페이스 키만 삭제 (다른 네임스페이스 무영향)
- **Keys 필터링**: 네임스페이스 접두사 제거 후 패턴 매칭

## TTL 관리

이중 만료 전략으로 TTL을 관리한다.

### Lazy Expiration (접근 시 만료)

Get, Has 호출 시 만료 여부를 확인하고, 만료된 키는 즉시 삭제한다. 접근하지 않는 키는 메모리에 남아 있을 수 있다.

### 백그라운드 스캔 (ttlManager)

설정된 주기(기본 1초)마다 모든 키를 스캔하여 만료된 항목을 일괄 삭제한다.

- **atomic.Bool**: Pause/Resume 시 데이터 레이스 없이 스캔 일시정지/재개
- **time.Ticker**: 주기적 스캔 트리거
- **SetInterval**: 런타임에 스캔 주기 변경 가능 (다음 틱부터 적용)
- **Stop**: 채널 닫기로 고루틴 안전 종료

## 센티널 에러

| 에러 변수 | 메시지 | 발생 조건 |
|-----------|--------|----------|
| `ErrKeyNotFound` | store: key not found | 키 미존재 또는 만료 |
| `ErrNamespaceNotAllowed` | store: namespace not allowed | 허용되지 않은 네임스페이스 접근 |
| `ErrStoreClosed` | store: store is closed | 닫힌 저장소에 연산 시도 |
| `ErrStorePaused` | store: store is paused | 일시정지 상태에서 쓰기 연산 시도 |
| `ErrNilValue` | store: nil value not allowed | nil 값 저장 시도 |
| `ErrInvalidTTL` | store: invalid TTL (must be >= 0) | 음수 TTL 지정 |
| `ErrNotSerializable` | store: value is not serializable | JSON 직렬화 불가 값 (PersistentStore) |
| `ErrKeyTooLong` | store: key exceeds maximum length | 키 길이 초과 (기본 512바이트) |

## 설계 특징

- **인터페이스 우선**: 모든 공개 API는 `Store` 인터페이스로 정의되며, 구현체(VolatileStore, PersistentStore)는 교체 가능
- **3계층 합성**: NamespacedStore -> agentStore -> VolatileStore/PersistentStore 체인
- **동시성 안전**: sync.Map(읽기 최적화), sync.RWMutex(상태 보호), atomic.Bool(TTL pause)
- **BaseLifecycle 임베딩**: 7상태 생명주기 관리를 상속하여 Init/Start/Pause/Resume/Stop 구현
- **상태 기반 접근 제어**: Paused 시 읽기만 허용, Stopped 시 모든 연산 차단
- **이중 만료**: lazy expiration(접근 시) + 백그라운드 스캔(주기적)으로 확실한 만료 보장
- **Write-Through 롤백**: PersistentStore에서 리포지토리 실패 시 캐시 자동 복원
- **Options 패턴**: NewStoreAgent()에 함수 옵션 패턴 적용
- **Configure 확장**: Running/Paused 상태에서 TTL 스캔 주기와 기본 TTL 런타임 변경

## 파일 구조

```
internal/agent/system/
  store_errors.go          # 센티널 에러 정의 (8개)
  store.go                 # Store 인터페이스, StoreEntry, StoreRepository, StoreAgent, agentStore, ForNamespace
  store_options.go         # StoreOption 타입, storeConfig, 5개 옵션 함수
  store_volatile.go        # VolatileStore (sync.Map 기반, lazy expiration, path.Match glob)
  store_ttl.go             # ttlManager (백그라운드 스캔, atomic.Bool pause, time.Ticker)
  store_namespace.go       # NamespacedStore 데코레이터 ("{namespace}:{key}" 접두사)
  store_persistent.go      # PersistentStore (Write-Through 캐시 + StoreRepository, JSON 직렬화)
  store_bridge.go          # BridgeHandler (메시지 기반 Store 접근 디스패처)
  store_errors_test.go     # 센티널 에러 테스트 (4개)
  store_volatile_test.go   # VolatileStore 테스트 (18개)
  store_ttl_test.go        # ttlManager 테스트 (8개)
  store_namespace_test.go  # NamespacedStore 테스트 (10개)
  store_test.go            # StoreAgent 통합 테스트 (24개)
  store_persistent_test.go # PersistentStore 테스트 (24개)
  store_bridge_test.go     # BridgeHandler 테스트 (11개)
```

## 의존성

- **표준 라이브러리**: `sync`, `sync/atomic`, `time`, `context`, `fmt`, `path`, `strings`, `encoding/json`, `errors`
- **내부 의존성**:
  - `pkg/lifecycle` (SPEC-LIFE-001) - 7상태 생명주기 관리 (BaseLifecycle 임베딩)
  - `pkg/message` (SPEC-MSG-001) - Bridge Node 메시지 처리 (BridgeHandler)

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

- 테스트: 106개 전체 통과 (99 tests + 7 subtests 포함)
- 커버리지: 90.9%
- Race Detector: 이상 없음
- Go Vet: 이상 없음
