---
id: SPEC-STORE-001
type: acceptance
version: "1.0.0"
spec_ref: SPEC-STORE-001
---

# SPEC-STORE-001 수락 기준

## Module 1: Store Interface - 스토어 인터페이스

### AC-STORE-001-01: Store 인터페이스 메서드 시그니처

```gherkin
Given Store 인터페이스가 정의되어 있을 때
Then Get(ctx context.Context, key string) (StoreEntry, error) 메서드가 존재해야 한다
And Set(ctx context.Context, key string, value any) error 메서드가 존재해야 한다
And SetWithTTL(ctx context.Context, key string, value any, ttl time.Duration) error 메서드가 존재해야 한다
And Delete(ctx context.Context, key string) error 메서드가 존재해야 한다
And Has(ctx context.Context, key string) (bool, error) 메서드가 존재해야 한다
And Keys(ctx context.Context, pattern string) ([]string, error) 메서드가 존재해야 한다
And Clear(ctx context.Context) error 메서드가 존재해야 한다
```

### AC-STORE-001-02: StoreEntry 구조체

```gherkin
Given StoreEntry가 정의되어 있을 때
Then Value, TTL, CreatedAt, UpdatedAt, Namespace, ExpiresAt 필드가 존재해야 한다
And Value는 any 타입이어야 한다
And TTL은 time.Duration 타입이어야 한다
And CreatedAt, UpdatedAt, ExpiresAt는 time.Time 타입이어야 한다
And Namespace는 string 타입이어야 한다
```

### AC-STORE-001-03: Get 호출 시 만료 확인

```gherkin
Given Store에 키 "temp"가 TTL 100ms로 저장되어 있을 때
When 150ms 후 Get(ctx, "temp")를 호출하면
Then ErrKeyNotFound 에러를 반환해야 한다
And 해당 키는 Store에서 삭제되어 있어야 한다
```

### AC-STORE-001-04: Set 호출 시 기존 키 덮어쓰기

```gherkin
Given Store에 키 "count"가 값 10으로 저장되어 있을 때
When Set(ctx, "count", 20)를 호출하면
Then Get(ctx, "count")가 값 20을 포함하는 StoreEntry를 반환해야 한다
And StoreEntry.UpdatedAt이 갱신되어야 한다
And StoreEntry.CreatedAt은 최초 생성 시각을 유지해야 한다
```

### AC-STORE-001-05: SetWithTTL 호출 시 TTL 설정

```gherkin
Given 빈 Store가 주어졌을 때
When SetWithTTL(ctx, "session", "abc123", 5*time.Second)를 호출하면
Then Get(ctx, "session")가 성공해야 한다
And StoreEntry.ExpiresAt이 현재 시각 + 5초 근처여야 한다
And StoreEntry.TTL이 5초여야 한다
```

### AC-STORE-001-06: Keys 패턴 매칭

```gherkin
Given Store에 "user:1", "user:2", "order:1" 키가 저장되어 있을 때
When Keys(ctx, "user:*")를 호출하면
Then ["user:1", "user:2"]를 반환해야 한다 (순서 무관)

Given Store에 여러 키가 저장되어 있을 때
When Keys(ctx, "")를 호출하면
Then 모든 키를 반환해야 한다
```

### AC-STORE-001-07: nil 값 저장 거부

```gherkin
Given 빈 Store가 주어졌을 때
When Set(ctx, "key", nil)를 호출하면
Then ErrNilValue 에러를 반환해야 한다
And Has(ctx, "key")가 false를 반환해야 한다
```

---

## Module 2: Volatile Store - 휘발성 저장소

### AC-STORE-001-08: VolatileStore 인터페이스 구현

```gherkin
Given NewVolatileStore()로 생성한 인스턴스가 주어졌을 때
Then Store 인터페이스를 구현해야 한다 (컴파일 타임 확인)
```

### AC-STORE-001-09: 동시성 안전

```gherkin
Given VolatileStore 인스턴스가 하나 주어졌을 때
When 100개의 goroutine이 동시에 Set/Get/Delete를 호출하면
Then race condition이 발생하지 않아야 한다 (go test -race 통과)
And 모든 연산이 정상 완료되어야 한다
```

### AC-STORE-001-10: 인메모리 저장 기본 CRUD

```gherkin
Given NewVolatileStore()로 생성한 인스턴스가 주어졌을 때
When Set(ctx, "key1", "value1")를 호출하면
Then Get(ctx, "key1")가 "value1"을 포함하는 StoreEntry를 반환해야 한다

When Delete(ctx, "key1")를 호출하면
Then Get(ctx, "key1")가 ErrKeyNotFound를 반환해야 한다

When Has(ctx, "key1")를 호출하면
Then false를 반환해야 한다
```

### AC-STORE-001-11: Clear 전체 삭제

```gherkin
Given Store에 "a", "b", "c" 키가 저장되어 있을 때
When Clear(ctx)를 호출하면
Then Keys(ctx, "*")가 빈 슬라이스를 반환해야 한다
```

---

## Module 3: Persistent Store - 영속적 저장소

### AC-STORE-001-12: PersistentStore 인터페이스 구현

```gherkin
Given Mock StoreRepository와 NewPersistentStore(repo)로 생성한 인스턴스가 주어졌을 때
Then Store 인터페이스를 구현해야 한다 (컴파일 타임 확인)
```

### AC-STORE-001-13: 캐시 미스 시 DB 로딩

```gherkin
Given PersistentStore 캐시에 키 "remote"가 없고 DB에는 존재할 때
When Get(ctx, "remote")를 호출하면
Then DB에서 값을 로딩하여 반환해야 한다
And 이후 동일 키 Get 호출 시 캐시에서 즉시 반환해야 한다
```

### AC-STORE-001-14: Set 실패 시 캐시 롤백

```gherkin
Given PersistentStore가 주어지고 DB에 키 "data"가 "old" 값으로 존재할 때
When Set(ctx, "data", "new")를 호출하고 DB 저장이 실패하면
Then 에러를 반환해야 한다
And Get(ctx, "data")가 "old" 값을 반환해야 한다 (캐시 롤백)
```

### AC-STORE-001-15: 값 직렬화/역직렬화

```gherkin
Given PersistentStore가 주어졌을 때
When Set(ctx, "config", map[string]any{"timeout": 30})를 호출하면
Then DB에 JSON 직렬화된 값이 저장되어야 한다
And Get(ctx, "config")가 원본 맵과 동일한 값을 반환해야 한다

Given 직렬화 불가능한 값(func() {})이 주어졌을 때
When Set(ctx, "func", value)를 호출하면
Then ErrNotSerializable 에러를 반환해야 한다
```

### AC-STORE-001-16: 시작 시 DB 캐시 프리로드

```gherkin
Given DB에 미만료 엔트리 3개가 존재할 때
When PersistentStore를 초기화하면
Then 3개의 엔트리가 캐시에 로딩되어야 한다
And Get 호출 시 DB 조회 없이 캐시에서 반환해야 한다
```

---

## Module 4: Namespace & Security - 네임스페이스 및 보안

### AC-STORE-001-17: NamespacedStore 키 격리

```gherkin
Given StoreAgent에서 ForNamespace("flow-abc")로 생성한 NamespacedStore가 주어졌을 때
When Set(ctx, "temp", 25.5)를 호출하면
Then 내부 저장소에 "flow-abc:temp" 키로 저장되어야 한다
And Get(ctx, "temp")가 25.5를 반환해야 한다
```

### AC-STORE-001-18: 네임스페이스 간 격리

```gherkin
Given "flow-abc" 네임스페이스에 "count" = 10 이 저장되어 있고
And "flow-xyz" 네임스페이스에 "count" = 20 이 저장되어 있을 때
When "flow-abc" NamespacedStore에서 Get(ctx, "count")를 호출하면
Then 10을 반환해야 한다
When "flow-xyz" NamespacedStore에서 Get(ctx, "count")를 호출하면
Then 20을 반환해야 한다
```

### AC-STORE-001-19: 타 네임스페이스 접근 차단

```gherkin
Given "flow-abc" NamespacedStore가 주어졌을 때
When "flow-xyz:secret" 키 형식으로 직접 접근을 시도하면
Then 실제 저장 키는 "flow-abc:flow-xyz:secret"이 되어
And "flow-xyz" 네임스페이스의 데이터에는 접근할 수 없어야 한다
```

### AC-STORE-001-20: 글로벌 네임스페이스

```gherkin
Given "global" NamespacedStore에 "shared_key" = "shared_value"가 저장되어 있을 때
When 다른 네임스페이스의 NamespacedStore에서 ForNamespace("global")를 통해 접근하면
Then Get(ctx, "shared_key")가 "shared_value"를 반환해야 한다
```

### AC-STORE-001-21: ForNamespace 팩토리 메서드

```gherkin
Given 초기화된 StoreAgent가 주어졌을 때
When ForNamespace("test-ns")를 호출하면
Then Store 인터페이스를 구현하는 NamespacedStore 인스턴스를 반환해야 한다
And 반환된 Store의 모든 연산이 "test-ns" 네임스페이스 범위로 제한되어야 한다
```

### AC-STORE-001-22: Keys 패턴 매칭이 네임스페이스 범위

```gherkin
Given "flow-abc" 네임스페이스에 "user:1", "user:2" 키가 있고
And "flow-xyz" 네임스페이스에 "user:3" 키가 있을 때
When "flow-abc" NamespacedStore에서 Keys(ctx, "user:*")를 호출하면
Then ["user:1", "user:2"]를 반환해야 한다 (네임스페이스 접두사 제거됨)
And "user:3"은 포함되지 않아야 한다
```

### AC-STORE-001-23: Clear가 네임스페이스 범위

```gherkin
Given "flow-abc" 네임스페이스에 2개 키, "flow-xyz" 네임스페이스에 1개 키가 있을 때
When "flow-abc" NamespacedStore에서 Clear(ctx)를 호출하면
Then "flow-abc" 네임스페이스의 2개 키만 삭제되어야 한다
And "flow-xyz" 네임스페이스의 1개 키는 유지되어야 한다
```

---

## Module 5: TTL Management - TTL 관리

### AC-STORE-001-24: 키별 TTL 설정

```gherkin
Given 빈 Store가 주어졌을 때
When SetWithTTL(ctx, "session", "token123", 2*time.Second)를 호출하면
Then Get(ctx, "session")가 성공해야 한다
And StoreEntry.ExpiresAt이 zero value가 아니어야 한다

When 3초 후 Get(ctx, "session")를 호출하면
Then ErrKeyNotFound를 반환해야 한다
```

### AC-STORE-001-25: 백그라운드 만료 스캔

```gherkin
Given TTL 200ms로 "expire_me" 키가 저장되어 있고 스캔 주기가 100ms일 때
When 500ms 후 Has(ctx, "expire_me")를 호출하면
Then false를 반환해야 한다 (백그라운드 스캔에 의해 삭제됨)
```

### AC-STORE-001-26: TTL 갱신

```gherkin
Given "key1"이 TTL 1초로 저장되어 있을 때
When 500ms 후 SetWithTTL(ctx, "key1", "new_value", 2*time.Second)를 호출하면
Then 2.5초 후에도 Get(ctx, "key1")가 성공해야 한다 (TTL이 갱신됨)

Given "key1"이 TTL 없이 저장되어 있을 때
When SetWithTTL(ctx, "key1", "value", 1*time.Second)를 호출하면
Then 1.5초 후 Get(ctx, "key1")가 ErrKeyNotFound를 반환해야 한다 (TTL 추가됨)
```

### AC-STORE-001-27: TTL 0은 만료 없음

```gherkin
Given SetWithTTL(ctx, "permanent", "value", 0)로 저장했을 때
When 임의 시간 후 Get(ctx, "permanent")를 호출하면
Then 항상 성공해야 한다
And StoreEntry.ExpiresAt이 zero value여야 한다
```

### AC-STORE-001-28: Set()은 기존 TTL 유지

```gherkin
Given "key1"이 TTL 5초로 저장되어 있을 때
When Set(ctx, "key1", "updated_value")를 호출하면
Then 값은 "updated_value"로 갱신되어야 한다
And 기존 TTL(5초)은 유지되어야 한다
```

### AC-STORE-001-29: 음수 TTL 거부

```gherkin
Given 빈 Store가 주어졌을 때
When SetWithTTL(ctx, "key", "value", -1*time.Second)를 호출하면
Then ErrInvalidTTL 에러를 반환해야 한다
And Has(ctx, "key")가 false를 반환해야 한다
```

---

## Module 6: Store Agent Lifecycle - 스토어 에이전트 생명주기

### AC-STORE-001-30: NewStoreAgent 생성자

```gherkin
Given NewStoreAgent()를 호출할 때
Then State()가 lifecycle.StateCreated를 반환해야 한다

Given NewStoreAgent(WithBackend("volatile"), WithDefaultTTL(30*time.Second))를 호출할 때
Then 생성된 StoreAgent의 설정이 반영되어야 한다
```

### AC-STORE-001-31: Init -> Start 생명주기

```gherkin
Given StateCreated 상태의 StoreAgent가 주어졌을 때
When Init(ctx)를 호출하면
Then State()가 lifecycle.StateRunning을 반환해야 한다
And 내부 저장소가 초기화되어야 한다
And TTL 만료 스캔 고루틴이 시작되어야 한다
```

### AC-STORE-001-32: Pause 시 쓰기 거부

```gherkin
Given StateRunning 상태의 StoreAgent가 주어졌을 때
When Pause(ctx)를 호출한 후 Set(ctx, "key", "value")를 시도하면
Then ErrStorePaused 에러를 반환해야 한다

When Pause(ctx) 후 Get(ctx, "existing_key")를 호출하면
Then 정상적으로 값을 반환해야 한다 (읽기는 허용)
```

### AC-STORE-001-33: Resume 시 쓰기 재개

```gherkin
Given StatePaused 상태의 StoreAgent가 주어졌을 때
When Resume(ctx)를 호출한 후 Set(ctx, "key", "value")를 시도하면
Then nil error를 반환해야 한다 (쓰기 허용)
```

### AC-STORE-001-34: Stop 시 Graceful Shutdown

```gherkin
Given StateRunning 상태의 StoreAgent가 주어졌을 때
When Stop(ctx)를 호출하면
Then State()가 lifecycle.StateStopped를 반환해야 한다
And TTL 만료 스캔 고루틴이 종료되어야 한다
And 이후 Get/Set 호출 시 ErrStoreClosed를 반환해야 한다
```

### AC-STORE-001-35: Configure 런타임 설정 변경

```gherkin
Given StateRunning 상태의 StoreAgent가 주어졌을 때
When Configure(ctx, map[string]any{"ttl_scan_interval": "500ms"})를 호출하면
Then TTL 스캔 주기가 500ms로 변경되어야 한다
And GetConfig()가 변경된 설정을 반환해야 한다

Given StateCreated 상태의 StoreAgent가 주어졌을 때
When Configure(ctx, cfg)를 호출하면
Then ErrInvalidStateForConfigure 에러를 반환해야 한다
```

### AC-STORE-001-36: HealthCheck

```gherkin
Given StateRunning 상태의 StoreAgent가 주어졌을 때 (휘발성 백엔드)
When HealthCheck(ctx)를 호출하면
Then Healthy=true인 HealthStatus를 반환해야 한다
And Details에 저장된 키 수 정보가 포함되어야 한다

Given StateStopped 상태의 StoreAgent가 주어졌을 때
When HealthCheck(ctx)를 호출하면
Then Healthy=false인 HealthStatus를 반환해야 한다
```

### AC-STORE-001-37: 자동 활성화

```gherkin
Given 시스템 시작 프로세스에서 StoreAgent가 생성될 때
Then 별도의 사용자 등록 없이 자동으로 Init/Start가 호출되어야 한다
And State()가 lifecycle.StateRunning을 반환해야 한다
```

---

## Module 7: Bridge Node Integration - 브릿지 노드 연동

### AC-STORE-001-38: Get 요청-응답 메시지

```gherkin
Given StoreAgent에 키 "sensor_data" = 42.5 가 저장되어 있을 때
When Bridge Node를 통해 store.operation="get", store.key="sensor_data" 메시지를 수신하면
Then 응답 메시지의 Payload에 42.5 값이 포함되어야 한다
And 응답 메시지의 Metadata["store.status"]가 "ok"여야 한다
```

### AC-STORE-001-39: Set 메시지 처리

```gherkin
Given 빈 Store가 주어졌을 때
When Bridge Node를 통해 store.operation="set", store.key="alert", Payload["value"]=true 메시지를 수신하면
Then 직접 API로 Get(ctx, "alert")를 호출했을 때 true를 반환해야 한다
```

### AC-STORE-001-40: 잘못된 연산 메시지

```gherkin
Given StoreAgent가 주어졌을 때
When Bridge Node를 통해 store.operation="invalid_op" 메시지를 수신하면
Then 응답 메시지의 Metadata["store.status"]가 "error"여야 한다
And 에러 메시지가 포함되어야 한다
```

### AC-STORE-001-41: 존재하지 않는 키 Get 응답

```gherkin
Given Store에 "missing_key"가 존재하지 않을 때
When Bridge Node를 통해 store.operation="get", store.key="missing_key" 메시지를 수신하면
Then 응답 메시지의 Metadata["store.status"]가 "error"여야 한다
And 에러 메시지에 "key not found" 정보가 포함되어야 한다
```

---

## Module 8: Error Types - 에러 타입

### AC-STORE-001-42: Sentinel 에러 정의

```gherkin
Given store_errors.go가 로드되어 있을 때
Then ErrKeyNotFound가 정의되어 있어야 한다
And ErrNamespaceNotAllowed가 정의되어 있어야 한다
And ErrStoreClosed가 정의되어 있어야 한다
And ErrStorePaused가 정의되어 있어야 한다
And ErrNilValue가 정의되어 있어야 한다
And ErrInvalidTTL이 정의되어 있어야 한다
And ErrNotSerializable이 정의되어 있어야 한다
And ErrKeyTooLong이 정의되어 있어야 한다
```

### AC-STORE-001-43: errors.Is() 호환성

```gherkin
Given ErrKeyNotFound 에러가 주어졌을 때
When fmt.Errorf("get failed: %w", ErrKeyNotFound)로 래핑한 후
Then errors.Is(wrappedErr, ErrKeyNotFound)가 true를 반환해야 한다
```

### AC-STORE-001-44: 키 길이 제한

```gherkin
Given 513바이트 길이의 키 문자열이 주어졌을 때
When Set(ctx, longKey, "value")를 호출하면
Then ErrKeyTooLong 에러를 반환해야 한다

Given 512바이트 길이의 키 문자열이 주어졌을 때
When Set(ctx, maxKey, "value")를 호출하면
Then nil error를 반환해야 한다 (허용 범위 이내)
```

---

## 품질 게이트

### Definition of Done

- [ ] 모든 수락 기준(AC-STORE-001-01 ~ 44) 테스트 통과
- [ ] `go test ./internal/agent/system/...` 전체 통과
- [ ] `go test -race ./internal/agent/system/...` 경쟁 상태 없음
- [ ] `go vet ./internal/agent/system/...` 경고 없음
- [ ] 테스트 커버리지 85% 이상 (`go test -cover`)
- [ ] GoDoc 주석 작성 완료 (모든 exported 타입/함수/메서드)
- [ ] `pkg/lifecycle/` 인터페이스 구현 확인 (Lifecycle, Configurable, HealthChecker)
- [ ] sentinel error가 `errors.Is()` 호환 확인
- [ ] 네임스페이스 격리 검증 완료 (교차 접근 차단)
- [ ] TTL 만료 동작 검증 완료 (백그라운드 스캔 + Lazy Expiration)
- [ ] Pause 상태에서 읽기 허용 / 쓰기 거부 검증
- [ ] Graceful Shutdown 검증 (고루틴 종료, PersistentStore 플러시)

### 검증 도구

| 도구 | 용도 |
|------|------|
| `go test` | 단위/통합 테스트 실행 |
| `go test -race` | 경쟁 상태 검출 |
| `go test -cover` | 커버리지 측정 |
| `go vet` | 정적 분석 |
| `golangci-lint` | 코드 품질 린팅 |
| `testify/assert` | 테스트 어설션 |
| `testify/mock` | StoreRepository 목 객체 |
