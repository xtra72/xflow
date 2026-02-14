---
id: SPEC-STORE-001
type: plan
version: "1.0.0"
spec_ref: SPEC-STORE-001
status: implemented
---

# SPEC-STORE-001 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반): 신규 파일이므로 TDD(RED-GREEN-REFACTOR) 적용
- 모든 파일이 신규 생성이므로 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표
- `go test -race` 필수 실행 (동시성 안전 검증)

### 1.2 기술 스택

- **언어**: Go 1.23+
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성**: `sync.Map` (VolatileStore), `sync.Mutex` (상태 관리)
- **DB 접근**: `internal/storage/` 리포지토리 인터페이스 (PersistentStore)
- **외부 의존성**: `pkg/lifecycle/` (SPEC-LIFE-001), `pkg/message/` (Bridge Node 연동 시)

### 1.3 패키지 위치

- **경로**: `internal/agent/system/`
- **Tier**: internal (비공개 패키지)
- **소비자**: internal/engine/ (Flow Runtime), internal/node/ (Bridge Node), Script Node (Lua 바인딩)

---

## 2. 마일스톤

### Primary Goal: Error Types + Store Interface + Volatile Store + Namespace + TTL + Lifecycle (P0)

**범위**: Module 1, 2, 4, 5, 6, 8

**작업 항목**:

1. `store_errors.go` + `store_errors_test.go` 작성
   - 8개 sentinel error 변수 정의
   - `errors.Is()` 호환성 테스트

2. `store.go` (인터페이스 부분) 작성
   - `Store` 인터페이스 정의 (7개 메서드)
   - `StoreEntry` 구조체 정의 (6개 필드)
   - `StoreAgent` 구조체 기본 골격

3. `store_options.go` 작성
   - `StoreOption` 함수 타입
   - `WithBackend()`, `WithDefaultTTL()`, `WithScanInterval()`, `WithRepository()`, `WithMaxKeyLength()` 옵션

4. `store_volatile.go` + `store_volatile_test.go` 작성
   - `VolatileStore` 구조체 (`sync.Map` 기반)
   - `Store` 인터페이스 구현 (Get, Set, SetWithTTL, Delete, Has, Keys, Clear)
   - 동시성 안전 테스트 (다수 goroutine 동시 접근)
   - nil 값 저장 거부 테스트
   - 키 길이 제한 테스트

5. `store_namespace.go` + `store_namespace_test.go` 작성
   - `NamespacedStore` 래퍼 구현
   - 네임스페이스 키 접두사 자동 추가/제거
   - 글로벌 네임스페이스 접근 테스트
   - 타 네임스페이스 접근 차단 테스트

6. `store_ttl.go` + `store_ttl_test.go` 작성
   - TTL 만료 스캔 고루틴 구현
   - 만료 키 자동 삭제 테스트
   - TTL 갱신 테스트
   - 음수 TTL 거부 테스트
   - 스캔 주기 설정 가능 테스트

7. `store.go` (StoreAgent 부분) 완성 + `store_test.go` 작성
   - `NewStoreAgent()` 생성자
   - `BaseLifecycle` 임베딩
   - `Init()`, `Start()`, `Pause()`, `Resume()`, `Stop()` 구현
   - `Configure()`, `GetConfig()` 구현
   - `HealthCheck()` 구현
   - `ForNamespace()` 팩토리 메서드 구현
   - 생명주기 전체 흐름 통합 테스트
   - Pause 시 쓰기 거부 / Get 허용 테스트

**산출물**: Store Agent의 핵심 기능 완성 (휘발성 백엔드, 네임스페이스, TTL, 생명주기)

---

### Secondary Goal: Persistent Store (P1)

**범위**: Module 3

**작업 항목**:

1. `store_persistent.go` + `store_persistent_test.go` 작성
   - `StoreRepository` 인터페이스 정의
   - `PersistentStore` 구조체 (캐시 레이어 + DB 백엔드)
   - `Store` 인터페이스 구현
   - 값 JSON 직렬화/역직렬화
   - 캐시 미스 시 DB 로딩 테스트
   - Set 실패 시 캐시 롤백 테스트
   - 시작 시 DB -> 캐시 로딩 테스트
   - 직렬화 불가능한 값 거부 테스트

**산출물**: DB 기반 영속적 저장소 구현 완성

---

### Final Goal: Bridge Node Integration (P1)

**범위**: Module 7

**작업 항목**:

1. `store_bridge.go` + `store_bridge_test.go` 작성
   - Bridge Node 메시지 핸들러 구현
   - `store.operation` 메타데이터 파싱
   - Get 요청-응답 패턴 (Correlation ID 매칭)
   - Set 메시지 처리
   - Delete 메시지 처리
   - 잘못된 연산 유형 에러 처리 테스트

**산출물**: Bridge Node를 통한 메시지 기반 Store 접근 완성

---

### Optional Goal: 만료 콜백 지원 (P2)

**범위**: REQ-STORE-001-05-05

**작업 항목**:

1. TTL 만료 콜백 등록/호출 메커니즘 추가
   - `OnExpire(callback func(key string, entry StoreEntry))` 메서드
   - 만료 스캔 시 콜백 호출
   - 콜백 panic recovery

**산출물**: 키 만료 이벤트 알림 기능

---

## 3. 기술적 접근

### 3.1 VolatileStore 설계

`sync.Map`을 사용하여 동시성 안전한 인메모리 저장소를 구현한다:

- 키: `string` (네임스페이스 접두사 포함된 전체 키)
- 값: `*storeItem` (내부 구조체, StoreEntry + 만료 시각)
- `sync.Map.Load()` / `Store()` / `Delete()` 네이티브 연산 활용
- `Range()`로 Keys 패턴 매칭 및 만료 스캔 구현

`sync.Map` 선택 이유:
- Store 특성상 읽기 빈도가 쓰기 빈도보다 높음 (캐시 패턴)
- `sync.Map`은 읽기 중심 워크로드에서 `sync.RWMutex` + `map`보다 우수한 성능
- Go 런타임 내장이므로 추가 의존성 없음

### 3.2 TTL 관리 전략

별도 고루틴에서 주기적으로 만료 키를 스캔한다:

- `time.Ticker` 기반 주기적 스캔 (기본 1초)
- 스캔 시 `sync.Map.Range()`로 전체 키 순회
- 만료된 키를 배치로 수집 후 일괄 삭제
- `context.Context` 기반 고루틴 종료 제어 (Graceful Shutdown)
- Pause 상태에서는 Ticker 일시정지

Lazy Expiration도 병행:
- `Get()` 호출 시 해당 키의 만료 여부 즉시 확인
- 만료된 키는 즉시 삭제 후 `ErrKeyNotFound` 반환
- 백그라운드 스캔 + Lazy Expiration 이중 보장

### 3.3 네임스페이스 구현 전략

`NamespacedStore`는 데코레이터 패턴으로 기존 `Store`를 래핑한다:

- 키 변환: `Set(ctx, "temp", 25)` -> 내부적으로 `Set(ctx, "flow-abc:temp", 25)`
- 키 역변환: `Get()` 반환 시 `StoreEntry.Namespace` 필드에 네임스페이스명 포함
- `Keys()` 호출 시 해당 네임스페이스의 키만 필터링하여 접두사 제거 후 반환
- `Clear()` 호출 시 해당 네임스페이스의 키만 삭제

### 3.4 PersistentStore 캐시 전략

Write-Through 캐시 패턴:
- `Set()` 호출 시: 캐시에 저장 -> DB에 저장 -> 성공 반환
- DB 저장 실패 시: 캐시에서 롤백 (이전 값 복원 또는 삭제)
- `Get()` 호출 시: 캐시에서 조회 -> 미스 시 DB 조회 -> 캐시에 저장 -> 반환
- 시작 시: DB의 모든 미만료 엔트리를 캐시로 프리로드

### 3.5 생명주기 통합

StoreAgent의 상태 전이:

```
Created -> Init() -> Initializing -> (저장소 초기화) -> Running
Running -> Pause() -> Paused (Get 허용, Set/Delete 거부)
Paused -> Resume() -> Running
Running/Paused -> Stop() -> Stopping -> (데이터 플러시) -> Stopped
Error -> Stop() -> Stopping -> Stopped
```

### 3.6 직접 참조 vs Bridge Node 접근

| 접근 방식 | 사용 위치 | 특징 |
|-----------|----------|------|
| 직접 참조 | Node 코드 (Go) | `storeAgent.ForNamespace(flowID).Get(ctx, key)` |
| Lua 바인딩 | Script Node | `xflow.store.get("key")` |
| Bridge Node | 플로우 에디터 | 메시지 기반, 시각적 연결 |

---

## 4. 리스크 및 대응

### Risk 1: sync.Map의 Range() 성능

- **위험**: 키가 수만 개 이상일 때 `Range()` 기반 TTL 스캔 및 Keys 패턴 매칭이 느려질 수 있음
- **대응**: 별도 만료 큐(heap) 도입 검토, 키 수가 증가하면 샤딩 또는 인덱스 구조 추가. 현재 MVP에서는 `Range()` 사용으로 충분

### Risk 2: PersistentStore DB 장애

- **위험**: DB 연결 실패 시 PersistentStore 전체가 동작 불능
- **대응**: 캐시 레이어가 있으므로 읽기는 캐시에서 서비스, 쓰기 실패는 에러 반환, HealthCheck에서 DB 상태 보고

### Risk 3: 네임스페이스 우회

- **위험**: NamespacedStore를 우회하여 원본 Store에 직접 접근하면 네임스페이스 격리가 깨짐
- **대응**: `StoreAgent.ForNamespace()` 메서드만 공개하고, 내부 Store 인스턴스는 unexported로 보호. 직접 참조 시에도 반드시 `ForNamespace()`를 거치도록 설계

### Risk 4: TTL 스캔 주기와 정확성

- **위험**: 스캔 주기(1초)보다 짧은 TTL을 설정하면 만료가 지연될 수 있음
- **대응**: Lazy Expiration과 병행하여 `Get()` 시 즉시 만료 확인. 백그라운드 스캔은 보조적 정리 역할

### Risk 5: Graceful Shutdown 시 데이터 유실

- **위험**: PersistentStore의 캐시에만 있고 DB에 미반영된 데이터가 Shutdown 시 유실
- **대응**: Write-Through 패턴이므로 Set() 시점에 항상 DB에 기록. 추가로 Stop() 시 캐시 플러시 확인

### Risk 6: 직렬화 호환성

- **위험**: PersistentStore에 JSON 직렬화 불가능한 타입(함수, 채널 등)이 전달될 수 있음
- **대응**: `Set()` 호출 시 `json.Marshal()` 사전 검증, 실패 시 `ErrNotSerializable` 반환

---

## 5. 의존성 그래프

```
internal/agent/system/store.go (본 SPEC)
  ├── 의존: pkg/lifecycle/          (SPEC-LIFE-001: BaseLifecycle, Configurable, HealthChecker)
  ├── 의존: pkg/message/            (SPEC-MSG-001: Bridge Node 연동 시 Message 타입)
  ├── 의존: internal/storage/       (PersistentStore: StoreRepository 인터페이스)
  ├── 의존: 표준 라이브러리         (sync, time, errors, context, fmt, encoding/json, path)
  ├── 소비자: internal/engine/      (Flow Runtime에서 StoreAgent 초기화)
  ├── 소비자: internal/node/bridge.go (Bridge Node에서 Store 연산 메시지 처리)
  ├── 소비자: internal/script/      (Lua 바인딩에서 Store 접근)
  └── 동료: internal/agent/system/event.go, logger.go, file.go, timer.go (다른 System Agent)
```

---

## 6. 구현 순서 (파일별)

| 순서 | 파일 | 설명 | 의존성 |
|------|------|------|--------|
| 1 | store_errors.go | Sentinel 에러 정의 | 없음 |
| 2 | store.go (인터페이스) | Store 인터페이스, StoreEntry 구조체 | store_errors.go |
| 3 | store_options.go | StoreOption 타입, 옵션 함수 | store.go |
| 4 | store_volatile.go | VolatileStore 구현 | store.go, store_errors.go |
| 5 | store_ttl.go | TTL 만료 스캔 고루틴 | store_volatile.go |
| 6 | store_namespace.go | NamespacedStore 래퍼 | store.go, store_errors.go |
| 7 | store.go (StoreAgent) | StoreAgent 생명주기 구현 | 전체 (1-6) + pkg/lifecycle/ |
| 8 | store_persistent.go | PersistentStore 구현 | store.go, internal/storage/ |
| 9 | store_bridge.go | Bridge Node 메시지 핸들러 | store.go, pkg/message/ |

모든 파일에 대해 TDD 방식으로 테스트 파일(`*_test.go`)을 먼저 작성한다.
