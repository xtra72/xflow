---

## id: SPEC-STORE-001
version: "2.0.0"
status: draft
created: "2026-02-13"
updated: "2026-04-15"
author: xtra
priority: high

## HISTORY


| 날짜         | 버전    | 변경 내용                              |
| ---------- | ----- | ---------------------------------- |
| 2026-02-13 | 1.0.0 | 초기 SPEC 작성                         |
| 2026-04-15 | 2.0.0 | Module 9: Per-Key Retention Policy 추가 |


---

# SPEC-STORE-001: Store System Agent - 키-값 저장소 시스템 에이전트

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 5개 System Agent(Event, Logger, File, Timer, Store) 중 하나인 Store Agent를 정의한다. Store Agent는 플로우 간 공유 데이터를 저장/조회할 수 있는 **키-값 저장소** 서비스이며, 영속적(DB) 또는 휘발성(메모리) 저장 백엔드를 선택할 수 있다.

본 SPEC은 다음을 다룬다:

- `Store` 인터페이스 정의 (Get, Set, Delete, Has, Keys, Clear)
- `StoreEntry` 타입 (값, TTL, 메타데이터)
- 네임스페이스 기반 키 격리 및 보안
- 휘발성 저장소 구현 (`sync.Map` 기반)
- 영속적 저장소 구현 (`internal/storage/` 리포지토리 패턴)
- TTL 관리 (만료, 백그라운드 스캔, 갱신)
- Store Agent 생명주기 (Lifecycle, Configurable, HealthChecker 인터페이스 구현)
- Bridge Node 연동 (선택적 메시지 기반 접근)
- 에러 타입 정의

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/agent/system/store.go` (구현), `internal/agent/system/system_test.go` (테스트)
- **의존성**: 표준 라이브러리 (`sync`, `time`, `errors`, `context`, `fmt`) + `pkg/lifecycle/` + `pkg/message/` (Bridge Node 연동 시)
- **선택적 의존성**: `internal/storage/` (영속적 백엔드 사용 시)
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **Tier**: internal (비공개 패키지, 외부 임포트 불가)

### 1.3 설계 원칙

- **인터페이스 우선**: `Store` 인터페이스를 정의하고 휘발성/영속적 백엔드가 동일 인터페이스를 구현
- **동시성 안전**: 모든 저장소 연산은 thread-safe 하게 동작 (sync.Map 또는 sync.Mutex 기반)
- **네임스페이스 격리**: 플로우별 접근 범위를 네임스페이스로 격리하여 보안 보장
- **TTL 기반 자동 만료**: 키별 TTL 설정으로 오래된 데이터를 자동 제거
- **생명주기 통합**: `pkg/lifecycle/Lifecycle`, `Configurable`, `HealthChecker` 인터페이스 구현
- **System Agent 패턴**: Transport/Protocol 설정 불필요, 시스템 시작 시 자동 활성화
- **최소 외부 의존성**: 핵심 기능은 표준 라이브러리만 사용, DB 백엔드는 선택적

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:

- `Store` 인터페이스 정의 (Get, Set, Delete, Has, Keys, Clear, SetWithTTL)
- `StoreEntry` 구조체 (Value, TTL, CreatedAt, UpdatedAt, Namespace)
- `VolatileStore` 구현 (sync.Map 기반 인메모리 저장소)
- `PersistentStore` 구현 (internal/storage/ 리포지토리 패턴 기반)
- `NamespacedStore` 래퍼 (네임스페이스 기반 접근 제어)
- TTL 관리 (백그라운드 만료 스캔 고루틴, TTL 갱신)
- `StoreAgent` 구조체 (System Agent로서의 생명주기 관리)
- 에러 타입 정의 (ErrKeyNotFound, ErrNamespaceNotAllowed 등)
- Bridge Node 통한 메시지 기반 접근 패턴 정의

**OUT OF SCOPE (별도 SPEC)**:

- Flow Engine 통합 (internal/engine/ - 별도 SPEC)
- Bridge Node 구현 자체 (internal/node/bridge.go - SPEC-FLOW-001 범위)
- 관찰성 시스템 통합 구현 (SPEC-OBS-001: internal/observe/)
- 설정 파일 로딩 및 Viper 통합 (SPEC-CFG-001: internal/config/)
- REST API 핸들러 (별도 SPEC: internal/api/)
- 다른 System Agent 구현 (Event, Logger, File, Timer)

### 1.5 관련 SPEC


| SPEC ID       | 관계  | 설명                                                            |
| ------------- | --- | ------------------------------------------------------------- |
| SPEC-LIFE-001 | 의존  | Store Agent가 Lifecycle, Configurable, HealthChecker 인터페이스를 구현 |
| SPEC-MSG-001  | 동료  | Bridge Node 통한 메시지 기반 접근 시 Message 타입 사용                      |
| SPEC-FLOW-001 | 소비자 | 플로우가 Bridge Node 또는 직접 API로 Store Agent 참조                    |
| SPEC-OBS-001  | 소비자 | Store 연산이 관찰성 시스템을 통해 로깅/메트릭 추적                               |
| SPEC-CFG-001  | 소비자 | Store 설정(백엔드 유형, TTL 기본값 등)을 설정 시스템으로 관리                      |


---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: `sync.Map`은 읽기 빈도가 높고 쓰기 빈도가 낮은 Store 접근 패턴에 적합하다
- A2: TTL 만료 스캔은 별도 고루틴에서 주기적(기본 1초)으로 수행하며, 만료 키를 배치로 제거한다
- A3: `StoreEntry.Value`는 `any` 타입으로, JSON 직렬화 가능한 모든 Go 값을 저장할 수 있다
- A4: 영속적 백엔드 사용 시 `internal/storage/` 리포지토리 인터페이스를 통해 접근하며, 직접 DB 드라이버를 import하지 않는다
- A5: Store Agent는 `pkg/lifecycle/BaseLifecycle`을 임베딩하여 공통 생명주기 로직을 재사용한다
- A6: Bridge Node 연동 시 요청/응답 패턴은 `pkg/message/Message`의 메타데이터에 연산 유형(get/set/delete)을 포함하여 전달한다
- A7: 네임스페이스 키 형식은 `{namespace}:{key}` 패턴을 사용한다 (예: `flow-123:temperature`)
- A8: 하나의 StoreAgent 인스턴스가 전역으로 공유되며, 네임스페이스로 플로우별 격리를 수행한다

### 2.2 도메인 가정

- A9: Store Agent는 시스템 시작 시 자동 활성화되며, 외부 연결 설정이 불필요하다
- A10: 기본 저장소 백엔드는 휘발성(메모리)이며, 설정에 따라 영속적(DB)으로 전환할 수 있다
- A11: 글로벌 네임스페이스(`global`)는 모든 플로우에서 접근 가능하며, 명시적 opt-in이 필요하다
- A12: TTL이 0 또는 미설정인 키는 만료되지 않으며, Store Agent가 중지될 때까지 유지된다
- A13: Store Agent의 Graceful Shutdown 시, 영속적 백엔드를 사용 중이면 미저장 데이터를 플러시한다
- A14: Store Agent의 HealthCheck는 백엔드 연결 상태(영속적) 또는 메모리 사용량(휘발성)을 확인한다

---

## 3. Requirements (요구사항)

### Module 1: Store Interface - 스토어 인터페이스 (P0)

#### REQ-STORE-001-01-01 (Ubiquitous) Store 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Store` 인터페이스를 제공해야 한다:

- `Get(ctx context.Context, key string) (StoreEntry, error)` - 키로 값 조회
- `Set(ctx context.Context, key string, value any) error` - 키에 값 저장 (TTL 없음)
- `SetWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error` - 키에 값 저장 (TTL 포함)
- `Delete(ctx context.Context, key string) error` - 키 삭제
- `Has(ctx context.Context, key string) (bool, error)` - 키 존재 여부 확인
- `Keys(ctx context.Context, pattern string) ([]string, error)` - 패턴에 매칭되는 키 목록 반환
- `Clear(ctx context.Context) error` - 모든 키 삭제

#### REQ-STORE-001-01-02 (Ubiquitous) StoreEntry 구조체 정의

시스템은 **항상** 다음 필드를 포함하는 `StoreEntry` 구조체를 제공해야 한다:

- `Value any` - 저장된 값
- `TTL time.Duration` - 키의 남은 유효 시간 (0이면 만료 없음)
- `CreatedAt time.Time` - 최초 생성 시각
- `UpdatedAt time.Time` - 마지막 갱신 시각
- `Namespace string` - 소속 네임스페이스
- `ExpiresAt time.Time` - 만료 예정 시각 (TTL 없으면 zero value)

#### REQ-STORE-001-01-03 (Event-Driven) Get 호출 시 만료 확인

**WHEN** `Store.Get(ctx, key)` 호출 시 해당 키가 존재하고 TTL이 설정되어 있으면, **THEN** 만료 여부를 확인하여 만료된 키는 `ErrKeyNotFound`를 반환하고 자동 삭제해야 한다.

#### REQ-STORE-001-01-04 (Event-Driven) Set 호출 시 기존 키 덮어쓰기

**WHEN** `Store.Set(ctx, key, value)` 호출 시 이미 존재하는 키이면, **THEN** 값을 덮어쓰고 `UpdatedAt`을 갱신해야 한다. 기존 TTL은 유지된다.

#### REQ-STORE-001-01-05 (Event-Driven) SetWithTTL 호출 시 TTL 설정

**WHEN** `Store.SetWithTTL(ctx, key, value, ttl)` 호출 시, **THEN** 값을 저장하고 `ExpiresAt`을 현재 시각 + ttl로 설정해야 한다. 기존 키가 있으면 TTL도 함께 갱신한다.

#### REQ-STORE-001-01-06 (Ubiquitous) Keys 패턴 매칭

시스템은 **항상** `Keys(ctx, pattern)` 호출 시 glob 패턴 매칭(`*`은 임의 문자열)을 지원해야 한다. 빈 패턴 `""`은 `"*"`과 동일하게 모든 키를 반환한다.

#### REQ-STORE-001-01-07 (Unwanted) nil 값 저장 금지

시스템은 `Set()` 또는 `SetWithTTL()`에 nil 값을 **저장하지 않아야 한다**. nil 값 저장 시도 시 `ErrNilValue` 에러를 반환해야 한다.

---

### Module 2: Volatile Store - 휘발성 저장소 (P0)

#### REQ-STORE-001-02-01 (Ubiquitous) VolatileStore 구조체

시스템은 **항상** `sync.Map` 기반의 `VolatileStore` 구조체를 제공해야 한다. `VolatileStore`는 `Store` 인터페이스를 구현한다.

#### REQ-STORE-001-02-02 (Ubiquitous) 동시성 안전

시스템은 **항상** `VolatileStore`의 모든 연산이 동시성 안전(thread-safe)하게 동작해야 한다. 다수의 goroutine에서 동시에 Get/Set/Delete를 호출해도 데이터 레이스가 발생하지 않아야 한다.

#### REQ-STORE-001-02-03 (Ubiquitous) 인메모리 저장

시스템은 **항상** `VolatileStore`의 데이터를 프로세스 메모리에만 저장해야 한다. 프로세스 종료 시 모든 데이터가 소멸된다.

#### REQ-STORE-001-02-04 (Event-Driven) Clear 호출 시 전체 삭제

**WHEN** `VolatileStore.Clear(ctx)` 호출 시, **THEN** 현재 네임스페이스 또는 전체 저장소의 모든 키를 삭제해야 한다.

---

### Module 3: Persistent Store - 영속적 저장소 (P1)

#### REQ-STORE-001-03-01 (Ubiquitous) PersistentStore 구조체

시스템은 **항상** `internal/storage/` 리포지토리 패턴을 사용하는 `PersistentStore` 구조체를 제공해야 한다. `PersistentStore`는 `Store` 인터페이스를 구현한다.

#### REQ-STORE-001-03-02 (Ubiquitous) 캐시 레이어

시스템은 **항상** `PersistentStore` 내부에 `sync.Map` 기반 인메모리 캐시를 유지하여, 읽기 성능을 최적화해야 한다. 캐시 미스 시 DB에서 로딩하여 캐시에 저장한다.

#### REQ-STORE-001-03-03 (Event-Driven) Set 호출 시 동기 DB 저장

**WHEN** `PersistentStore.Set(ctx, key, value)` 호출 시, **THEN** 캐시와 DB 모두에 값을 저장해야 한다. DB 저장 실패 시 에러를 반환하고 캐시도 롤백한다.

#### REQ-STORE-001-03-04 (Ubiquitous) 값 직렬화

시스템은 **항상** DB에 저장할 값을 JSON 형식으로 직렬화하고, 조회 시 역직렬화해야 한다. 직렬화할 수 없는 값(함수, 채널 등) 저장 시 `ErrNotSerializable` 에러를 반환한다.

#### REQ-STORE-001-03-05 (Event-Driven) 시작 시 DB에서 캐시 로딩

**WHEN** `PersistentStore`가 초기화될 때, **THEN** DB에 저장된 미만료 데이터를 캐시로 로딩해야 한다.

---

### Module 4: Namespace & Security - 네임스페이스 및 보안 (P0)

#### REQ-STORE-001-04-01 (Ubiquitous) NamespacedStore 래퍼

시스템은 **항상** 네임스페이스 기반 접근 제어를 제공하는 `NamespacedStore` 래퍼를 제공해야 한다. `NamespacedStore`는 `Store` 인터페이스를 구현하며, 내부적으로 키에 네임스페이스 접두사를 추가한다.

#### REQ-STORE-001-04-02 (Ubiquitous) 네임스페이스 키 형식

시스템은 **항상** `{namespace}:{key}` 형식으로 내부 키를 관리해야 한다. 외부에서는 네임스페이스 접두사 없이 순수 키만 사용한다.

#### REQ-STORE-001-04-03 (State-Driven) 플로우별 네임스페이스 격리

**IF** 특정 플로우가 자신의 네임스페이스로 Store에 접근할 때, **THEN** 해당 플로우의 네임스페이스에 속한 키만 조회/수정/삭제할 수 있어야 한다.

#### REQ-STORE-001-04-04 (State-Driven) 글로벌 네임스페이스 접근

**IF** 플로우가 글로벌 네임스페이스(`global`)로 Store에 접근할 때, **THEN** 모든 플로우에서 공유되는 글로벌 키에 접근할 수 있어야 한다.

#### REQ-STORE-001-04-05 (Unwanted) 타 네임스페이스 접근 차단

시스템은 한 플로우의 NamespacedStore가 다른 플로우의 네임스페이스에 속한 키를 직접 접근**하지 않아야 한다**. 접근 시도 시 `ErrNamespaceNotAllowed` 에러를 반환해야 한다.

#### REQ-STORE-001-04-06 (Ubiquitous) ForNamespace() 팩토리 메서드

시스템은 **항상** `StoreAgent.ForNamespace(namespace string) Store` 메서드를 제공하여, 특정 네임스페이스에 바인딩된 `NamespacedStore` 인스턴스를 반환해야 한다.

---

### Module 5: TTL Management - TTL 관리 (P0)

#### REQ-STORE-001-05-01 (Ubiquitous) 키별 TTL 설정

시스템은 **항상** 개별 키에 대해 TTL(Time-To-Live)을 설정할 수 있어야 한다. TTL이 0이면 만료 없음을 의미한다.

#### REQ-STORE-001-05-02 (Ubiquitous) 백그라운드 만료 스캔

시스템은 **항상** 별도 고루틴에서 주기적으로(기본 1초 간격) 만료된 키를 스캔하고 삭제해야 한다. 스캔 주기는 설정 가능하다.

#### REQ-STORE-001-05-03 (Event-Driven) TTL 갱신

**WHEN** 기존 키에 대해 `SetWithTTL(ctx, key, value, newTTL)`을 호출하면, **THEN** TTL이 새로운 값으로 갱신되고 `ExpiresAt`이 재계산되어야 한다.

#### REQ-STORE-001-05-04 (Event-Driven) 만료 시 자동 삭제

**WHEN** 키의 `ExpiresAt` 시각이 현재 시각을 초과하면, **THEN** 해당 키를 자동으로 삭제해야 한다. 삭제된 키에 대한 Get 호출은 `ErrKeyNotFound`를 반환한다.

#### REQ-STORE-001-05-05 (Optional) 만료 콜백 지원

**가능하면** 키 만료 시 등록된 콜백을 호출하여 만료 이벤트를 알림할 수 있어야 한다.

#### REQ-STORE-001-05-06 (Unwanted) 음수 TTL 허용 금지

시스템은 음수 TTL 값을 **설정하지 않아야 한다**. 음수 TTL 설정 시도 시 `ErrInvalidTTL` 에러를 반환해야 한다.

---

### Module 6: Store Agent Lifecycle - 스토어 에이전트 생명주기 (P0)

#### REQ-STORE-001-06-01 (Ubiquitous) StoreAgent 구조체

시스템은 **항상** `StoreAgent` 구조체를 제공해야 한다. `StoreAgent`는 다음 인터페이스를 구현한다:

- `pkg/lifecycle/Lifecycle` - 생명주기 관리 (Init, Start, Pause, Resume, Stop, State)
- `pkg/lifecycle/Configurable` - 런타임 설정 변경 (Configure, GetConfig)
- `pkg/lifecycle/HealthChecker` - 헬스 체크 (HealthCheck)

#### REQ-STORE-001-06-02 (Ubiquitous) BaseLifecycle 임베딩

시스템은 **항상** `StoreAgent`가 `pkg/lifecycle/BaseLifecycle`을 임베딩하여 공통 상태 전이 로직을 재사용해야 한다.

#### REQ-STORE-001-06-03 (Event-Driven) Init 시 저장소 초기화

**WHEN** `StoreAgent.Init(ctx)` 호출 시, **THEN** 설정에 따라 `VolatileStore` 또는 `PersistentStore`를 초기화하고, TTL 만료 스캔 고루틴을 시작해야 한다.

#### REQ-STORE-001-06-04 (Event-Driven) Stop 시 Graceful Shutdown

**WHEN** `StoreAgent.Stop(ctx)` 호출 시, **THEN** TTL 만료 스캔 고루틴을 중지하고, PersistentStore 사용 시 미저장 데이터를 플러시한 뒤, 리소스를 해제해야 한다.

#### REQ-STORE-001-06-05 (Event-Driven) Pause 시 쓰기 중단

**WHEN** `StoreAgent.Pause(ctx)` 호출 시, **THEN** 새로운 Set/Delete 연산을 거부하되 Get 연산은 허용해야 한다. TTL 만료 스캔은 일시정지한다.

#### REQ-STORE-001-06-06 (Event-Driven) Resume 시 쓰기 재개

**WHEN** `StoreAgent.Resume(ctx)` 호출 시, **THEN** Set/Delete 연산을 다시 허용하고 TTL 만료 스캔을 재개해야 한다.

#### REQ-STORE-001-06-07 (Ubiquitous) 자동 활성화

시스템은 **항상** 시스템 시작 시 StoreAgent를 자동으로 생성하고 Init/Start하여 활성화해야 한다. 별도의 사용자 등록이 불필요하다.

#### REQ-STORE-001-06-08 (State-Driven) Configure를 통한 런타임 설정 변경

**IF** StoreAgent가 Running 또는 Paused 상태일 때, **THEN** `Configure(ctx, cfg)`를 통해 다음 설정을 런타임에 변경할 수 있어야 한다:

- `ttl_scan_interval` - TTL 스캔 주기
- `default_ttl` - 기본 TTL 값

#### REQ-STORE-001-06-09 (Ubiquitous) HealthCheck 구현

시스템은 **항상** `StoreAgent.HealthCheck(ctx)` 호출 시 다음을 확인하여 `HealthStatus`를 반환해야 한다:

- 휘발성 백엔드: 저장된 키 수, 메모리 사용 추정
- 영속적 백엔드: DB 연결 상태, 저장된 키 수

---

### Module 7: Bridge Node Integration - 브릿지 노드 연동 (P1)

#### REQ-STORE-001-07-01 (Optional) Bridge Node 메시지 기반 접근

**가능하면** Store Agent는 Bridge Node를 통해 메시지 기반으로 접근할 수 있어야 한다. 메시지의 메타데이터에 연산 유형을 포함한다:

- `store.operation`: `get`, `set`, `delete`, `has`, `keys`, `clear`
- `store.key`: 대상 키
- `store.value`: 저장할 값 (set 연산 시)
- `store.ttl`: TTL 값 (set 연산 시, 선택)
- `store.namespace`: 네임스페이스 (선택, 기본값은 플로우 ID)

#### REQ-STORE-001-07-02 (Event-Driven) Get 요청-응답 패턴

**WHEN** Bridge Node를 통해 `get` 연산 메시지를 수신하면, **THEN** 해당 키의 값을 조회하여 응답 메시지의 Payload에 포함하여 반환해야 한다. Correlation ID를 통해 요청-응답을 매칭한다.

#### REQ-STORE-001-07-03 (Event-Driven) Set 메시지 처리

**WHEN** Bridge Node를 통해 `set` 연산 메시지를 수신하면, **THEN** 메시지 Payload에서 키, 값, TTL을 추출하여 Store에 저장해야 한다.

---

### Module 8: Error Types - 에러 타입 (P0)

#### REQ-STORE-001-08-01 (Ubiquitous) 표준 에러 변수

시스템은 **항상** 다음 에러 변수를 제공해야 한다:


| 에러 변수                    | 용도                                   |
| ------------------------ | ------------------------------------ |
| `ErrKeyNotFound`         | 조회한 키가 존재하지 않거나 만료된 경우               |
| `ErrNamespaceNotAllowed` | 허용되지 않은 네임스페이스 접근 시도 시               |
| `ErrStoreClosed`         | 중지된 Store에 대한 연산 시도 시                |
| `ErrStorePaused`         | 일시정지된 Store에 대한 쓰기 연산 시도 시           |
| `ErrNilValue`            | nil 값 저장 시도 시                        |
| `ErrInvalidTTL`          | 음수 TTL 설정 시도 시                       |
| `ErrNotSerializable`     | 직렬화 불가능한 값을 PersistentStore에 저장 시도 시 |
| `ErrKeyTooLong`          | 키 길이가 최대 허용 길이(512바이트)를 초과한 경우       |


#### REQ-STORE-001-08-02 (Ubiquitous) 에러 래핑 지원

시스템은 **항상** 모든 에러 변수가 `errors.Is()` 및 `errors.As()`와 호환되도록 sentinel error 패턴을 사용해야 한다.

#### REQ-STORE-001-08-03 (Ubiquitous) 키 길이 제한

시스템은 **항상** 키 문자열 길이를 512바이트 이하로 제한해야 한다. 초과 시 `ErrKeyTooLong` 에러를 반환한다.

---

### Module 9: Per-Key Retention Policy - 키별 보존 정책 (P0)

#### REQ-STORE-001-09-01 (Ubiquitous) RetentionPolicy 타입 정의

시스템은 **항상** 다음 필드를 포함하는 `RetentionPolicy` 구조체를 제공해야 한다:

- `MaxCount int` - 해당 키의 히스토리 최대 보관 수 (0이면 글로벌 설정 사용)
- `Interval time.Duration` - 해당 키의 히스토리 항목 최대 보관 시간 (0이면 글로벌 설정 사용)

`RetentionPolicy`는 키별로 히스토리 보존 전략을 개별 지정하기 위한 타입이다. 글로벌 `maxHistorySize`와 `historyTTL`의 키별 오버라이드 역할을 한다.

#### REQ-STORE-001-09-02 (Event-Driven) SetRetention 메서드

**WHEN** `Store.SetRetention(ctx, key, policy)` 호출 시, **THEN** 해당 키에 대한 보존 정책을 설정해야 한다. 키가 존재하지 않아도 보존 정책은 미리 설정할 수 있다 (키 생성 시 적용됨).

#### REQ-STORE-001-09-03 (Event-Driven) GetRetention 메서드

**WHEN** `Store.GetRetention(ctx, key)` 호출 시, **THEN** 해당 키에 설정된 보존 정책을 반환해야 한다. 보존 정책이 설정되지 않은 키는 zero value `RetentionPolicy{}`를 반환한다.

#### REQ-STORE-001-09-04 (State-Driven) 키별 보존 정책 적용

**IF** 특정 키에 보존 정책이 설정된 상태에서 값이 갱신될 때, **THEN** 글로벌 `maxHistorySize`/`historyTTL` 대신 키별 보존 정책(`RetentionPolicy.MaxCount`, `RetentionPolicy.Interval`)을 적용하여 히스토리를 관리해야 한다.

- `RetentionPolicy.MaxCount`가 0이 아니면: 해당 키의 히스토리를 `MaxCount`개까지만 보관
- `RetentionPolicy.Interval`이 0이 아니면: 해당 키의 히스토리 중 `Interval`보다 오래된 항목을 제거
- 두 값 모두 설정된 경우: 두 조건을 모두 적용 (AND 조건)

#### REQ-STORE-001-09-05 (Unwanted) 글로벌 상한 초과 금지

시스템은 키별 `RetentionPolicy.MaxCount`가 글로벌 `maxHistorySize`를 **초과하지 않아야 한다**. `MaxCount`가 글로벌 `maxHistorySize`보다 큰 값으로 설정되면 자동으로 글로벌 `maxHistorySize` 값으로 클램핑(clamping)한다. 단, 글로벌 `maxHistorySize`가 0(비활성)인 경우에는 클램핑을 적용하지 않는다.

#### REQ-STORE-001-09-06 (Event-Driven) DeleteRetention 메서드

**WHEN** `Store.DeleteRetention(ctx, key)` 호출 시, **THEN** 해당 키에 설정된 보존 정책을 제거하고 글로벌 설정으로 복귀해야 한다. 보존 정책이 설정되지 않은 키에 대해 호출해도 에러 없이 성공해야 한다.

#### REQ-STORE-001-09-07 (Ubiquitous) NamespacedStore 보존 정책 투명 지원

시스템은 **항상** `NamespacedStore`가 `SetRetention`, `GetRetention`, `DeleteRetention` 메서드를 내부 Store에 네임스페이스 접두사를 추가하여 투명하게 위임해야 한다. 사용자는 네임스페이스를 의식하지 않고 순수 키만으로 보존 정책을 관리할 수 있다.

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
internal/agent/system/
  store.go              # StoreAgent 구조체, Store 인터페이스, StoreEntry, RetentionPolicy
  store_volatile.go     # VolatileStore 구현 (sync.Map 기반, 키별 보존 정책 포함)
  store_persistent.go   # PersistentStore 구현 (internal/storage/ 기반)
  store_namespace.go    # NamespacedStore 래퍼 (보존 정책 위임 포함)
  store_ttl.go          # TTL 관리 (만료 스캔 고루틴)
  store_bridge.go       # Bridge Node 메시지 핸들러 (선택적)
  store_errors.go       # 에러 변수 정의
  store_options.go      # StoreOption 함수 타입 및 옵션 함수

  store_test.go         # Store 인터페이스 통합 테스트
  store_volatile_test.go    # VolatileStore 단위 테스트
  store_persistent_test.go  # PersistentStore 단위 테스트
  store_namespace_test.go   # NamespacedStore 테스트
  store_ttl_test.go         # TTL 관리 테스트
  store_bridge_test.go      # Bridge Node 연동 테스트
```

### 4.2 타입 시그니처

```go
// Store 인터페이스
type Store interface {
    Get(ctx context.Context, key string) (StoreEntry, error)
    Set(ctx context.Context, key string, value any) error
    SetWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error
    Delete(ctx context.Context, key string) error
    Has(ctx context.Context, key string) (bool, error)
    Keys(ctx context.Context, pattern string) ([]string, error)
    Clear(ctx context.Context) error
    GetHistory(ctx context.Context, key string) ([]HistoryEntry, error)

    // v2.0.0: Per-Key Retention Policy (Module 9)
    SetRetention(ctx context.Context, key string, policy RetentionPolicy) error
    GetRetention(ctx context.Context, key string) (RetentionPolicy, error)
    DeleteRetention(ctx context.Context, key string) error
}

// RetentionPolicy - 키별 히스토리 보존 정책 (v2.0.0)
type RetentionPolicy struct {
    MaxCount int           // 키별 히스토리 최대 보관 수 (0이면 글로벌 설정 사용)
    Interval time.Duration // 키별 히스토리 최대 보관 시간 (0이면 글로벌 설정 사용)
}

// StoreEntry - 저장 엔트리
type StoreEntry struct {
    Value     any
    TTL       time.Duration
    CreatedAt time.Time
    UpdatedAt time.Time
    Namespace string
    ExpiresAt time.Time  // zero value면 만료 없음
}

// StoreAgent - System Agent
type StoreAgent struct {
    *lifecycle.BaseLifecycle  // 임베딩
    // unexported fields
}

func NewStoreAgent(opts ...StoreOption) *StoreAgent
func (s *StoreAgent) Init(ctx context.Context) error
func (s *StoreAgent) Start(ctx context.Context) error
func (s *StoreAgent) Pause(ctx context.Context) error
func (s *StoreAgent) Resume(ctx context.Context) error
func (s *StoreAgent) Stop(ctx context.Context) error
func (s *StoreAgent) State() lifecycle.State
func (s *StoreAgent) Configure(ctx context.Context, cfg map[string]any) error
func (s *StoreAgent) GetConfig() map[string]any
func (s *StoreAgent) HealthCheck(ctx context.Context) lifecycle.HealthStatus
func (s *StoreAgent) ForNamespace(namespace string) Store

// VolatileStore - 인메모리 저장소
type VolatileStore struct { /* unexported fields */ }

func NewVolatileStore() *VolatileStore

// PersistentStore - DB 기반 저장소
type PersistentStore struct { /* unexported fields */ }

func NewPersistentStore(repo StoreRepository) *PersistentStore

// StoreRepository - 영속 저장소 인터페이스 (internal/storage/ 패턴 준수)
type StoreRepository interface {
    GetEntry(ctx context.Context, key string) (*StoreEntry, error)
    SetEntry(ctx context.Context, key string, entry *StoreEntry) error
    DeleteEntry(ctx context.Context, key string) error
    ListKeys(ctx context.Context, pattern string) ([]string, error)
    DeleteExpired(ctx context.Context, before time.Time) (int, error)
    ClearNamespace(ctx context.Context, namespace string) error
}

// NamespacedStore - 네임스페이스 래퍼
type NamespacedStore struct { /* unexported fields */ }

func NewNamespacedStore(store Store, namespace string) *NamespacedStore

// Options Pattern
type StoreOption func(*StoreAgent)

func WithBackend(backend string) StoreOption          // "volatile" 또는 "persistent"
func WithDefaultTTL(ttl time.Duration) StoreOption
func WithScanInterval(interval time.Duration) StoreOption
func WithRepository(repo StoreRepository) StoreOption
func WithMaxKeyLength(length int) StoreOption

// 에러 변수
var (
    ErrKeyNotFound         = errors.New("store: key not found")
    ErrNamespaceNotAllowed = errors.New("store: namespace not allowed")
    ErrStoreClosed         = errors.New("store: store is closed")
    ErrStorePaused         = errors.New("store: store is paused (write operations disabled)")
    ErrNilValue            = errors.New("store: nil value not allowed")
    ErrInvalidTTL          = errors.New("store: invalid TTL (must be >= 0)")
    ErrNotSerializable     = errors.New("store: value is not serializable")
    ErrKeyTooLong          = errors.New("store: key exceeds maximum length")
)
```

### 4.3 SPEC-LIFE-001과의 관계

Store Agent는 `pkg/lifecycle/BaseLifecycle`을 임베딩하여 공통 상태 머신 로직을 재사용한다:

- `Init()` 호출 시 `BaseLifecycle.TransitionTo(StateInitializing)` 후 저장소 초기화, 성공 시 `TransitionTo(StateRunning)`
- `Stop()` 호출 시 `TransitionTo(StateStopping)` 후 리소스 해제, 완료 시 `TransitionTo(StateStopped)`
- `Configure()` 호출 시 `CurrentState()` 확인 후 Running/Paused 상태에서만 허용
- `HealthCheck()` 호출은 상태와 무관하게 항상 가능

### 4.4 SPEC-MSG-001과의 관계

Bridge Node 연동 시 `pkg/message/Message`의 Metadata를 활용하여 Store 연산을 인코딩한다:

- 요청 메시지: `Metadata["store.operation"]`, `Metadata["store.key"]`, `Payload["value"]`
- 응답 메시지: `Payload["value"]` (Get 결과), `Metadata["store.status"]` ("ok" 또는 "error")
- Correlation ID를 통한 요청-응답 매칭은 Bridge Node의 책임 (SPEC-FLOW-001)

### 4.5 내부 키 저장 구조

```
실제 저장 키: "{namespace}:{user_key}"
예시:
  "flow-abc:temperature"  -> 플로우 abc의 temperature 키
  "flow-xyz:counter"      -> 플로우 xyz의 counter 키
  "global:shared_config"  -> 글로벌 공유 설정 키
```

NamespacedStore는 사용자에게는 순수 키(`temperature`)만 노출하고, 내부적으로 네임스페이스 접두사(`flow-abc:temperature`)를 추가/제거하여 관리한다.

---

## 5. Traceability (추적성)


| 요구사항 ID                      | 모듈                        | 파일                                          | 우선순위 |
| ---------------------------- | ------------------------- | ------------------------------------------- | ---- |
| REQ-STORE-001-01-01 ~ 01-07  | Store Interface           | store.go                                    | P0   |
| REQ-STORE-001-02-01 ~ 02-04  | Volatile Store            | store_volatile.go                           | P0   |
| REQ-STORE-001-03-01 ~ 03-05  | Persistent Store          | store_persistent.go                         | P1   |
| REQ-STORE-001-04-01 ~ 04-06  | Namespace & Security      | store_namespace.go                          | P0   |
| REQ-STORE-001-05-01 ~ 05-06  | TTL Management            | store_ttl.go                                | P0   |
| REQ-STORE-001-06-01 ~ 06-09  | Store Agent Lifecycle     | store.go                                    | P0   |
| REQ-STORE-001-07-01 ~ 07-03  | Bridge Node Integration   | store_bridge.go                             | P1   |
| REQ-STORE-001-08-01 ~ 08-03  | Error Types               | store_errors.go                             | P0   |
| REQ-STORE-001-09-01 ~ 09-07  | Per-Key Retention Policy  | store.go, store_volatile.go, store_namespace.go | P0   |


