---
id: SPEC-STORE-002
type: acceptance
version: "1.0.0"
created: "2026-03-30"
updated: "2026-03-30"
---

# SPEC-STORE-002 인수 조건: Store Value History

## 테스트 시나리오

### Scenario 1: 히스토리 비활성 (하위 호환성)

```gherkin
Given max_history_size=0인 StoreAgent가 초기화됨
When "key1"에 "v1" Set 후 "v2"로 Set
Then Get("key1").Value == "v2"
And Get("key1").HistoryCount == 0
And Get("key1").MaxHistorySize == 0
And GetHistory("key1") == []HistoryEntry{} (빈 슬라이스)
```

### Scenario 2: 히스토리 축적 (최신순)

```gherkin
Given max_history_size=10인 StoreAgent가 초기화됨
When "temp"에 20.0, 21.0, 22.0을 순서대로 Set
Then Get("temp").Value == 22.0
And GetHistory("temp") 길이 == 2
And GetHistory("temp")[0].Value == 21.0 (가장 최근)
And GetHistory("temp")[1].Value == 20.0 (가장 오래됨)
```

### Scenario 3: 갯수 초과 시 오래된 항목 제거

```gherkin
Given max_history_size=3인 StoreAgent가 초기화됨
When "key1"에 v1, v2, v3, v4, v5를 순서대로 Set (5회)
Then Get("key1").Value == v5
And GetHistory("key1") 길이 == 3
And GetHistory("key1")[0].Value == v4
And GetHistory("key1")[1].Value == v3
And GetHistory("key1")[2].Value == v2
And v1은 히스토리에 존재하지 않음
```

### Scenario 4: 시간 초과 시 오래된 항목 제거

```gherkin
Given max_history_size=100, history_ttl=5s인 StoreAgent가 초기화됨
And "key1"에 1초 간격으로 10회 Set 수행
When 6초 경과 후 GetHistory("key1") 호출
Then Timestamp가 5초 이전인 항목은 제거됨
And 최근 5초 이내 항목만 남음
```

### Scenario 5: Delete 시 히스토리 삭제

```gherkin
Given "key1"에 히스토리 5개 축적
When Delete("key1") 호출
Then GetHistory("key1") == ErrKeyNotFound
And Has("key1") == false
```

### Scenario 6: Clear 시 전체 히스토리 삭제

```gherkin
Given "key1", "key2", "key3"에 각각 히스토리 축적
When Clear() 호출
Then GetHistory("key1") == ErrKeyNotFound
And GetHistory("key2") == ErrKeyNotFound
And GetHistory("key3") == ErrKeyNotFound
```

### Scenario 7: NamespacedStore 히스토리 격리

```gherkin
Given ns1 = NamespacedStore("ns1"), ns2 = NamespacedStore("ns2")
And max_history_size=10
When ns1.Set("key", "a") 후 ns1.Set("key", "b") 호출
Then ns1.GetHistory("key") 길이 == 1
And ns1.GetHistory("key")[0].Value == "a"
And ns2.GetHistory("key") == ErrKeyNotFound
```

### Scenario 8: YAML 설정 파싱

```gherkin
Given Agent YAML 설정: max_history_size: 50, history_ttl: "30m"
When UserStoreAgent가 설정 파싱
Then storeConfig.maxHistorySize == 50
And storeConfig.historyTTL == 30분
```

### Scenario 9: Configure() 런타임 변경

```gherkin
Given max_history_size=100, 각 키에 80개 히스토리 축적
When Configure(ctx, {"max_history_size": 50}) 호출
Then 모든 키의 히스토리가 50개 이하로 트리밍됨
And 이후 Set 시 50개 제한 적용
```

### Scenario 10: TTL 매니저 히스토리 정리

```gherkin
Given max_history_size=100, history_ttl=5s
And "key1"에 히스토리 10개 축적 후 Set 호출 없이 대기
When 6초 후 ttlManager.scan() 실행
Then "key1"의 5초 이전 히스토리 항목이 제거됨
```

### Scenario 11: State() 히스토리 메타데이터

```gherkin
Given "k1"에 10개, "k2"에 20개, "k3"에 30개 히스토리 축적
When UserStoreAgent.State() 호출
Then 각 엔트리에 history_count 필드 포함 (10, 20, 30)
And 응답에 total_history_entries == 60
```

### Scenario 12: 동시성 안전

```gherkin
Given max_history_size=100인 StoreAgent
When 10개 goroutine에서 동일 키에 Set과 GetHistory를 1000회 반복
Then go test -race 통과
And 히스토리 길이는 항상 100 이하
And 데이터 손실 없음
```

### Scenario 13: StoreReadNode include_history

```gherkin
Given include_history=true로 설정된 StoreReadNode
And "sensor1" 키에 히스토리 [25.0, 24.0, 23.0] 축적
When StoreReadNode 실행
Then 출력 payload에 "history" 배열 포함
And history[0]["value"] == 25.0
```

### Scenario 14: BridgeHandler history 오퍼레이션

```gherkin
Given "key1"에 히스토리 5개 축적
When store.operation="history", store.key="key1" 메시지 수신
Then 응답 Payload에 history 배열 (길이 5)
And count == 5
And max_size == 설정된 maxHistorySize
```

### Scenario 15: SetWithTTL 히스토리 축적

```gherkin
Given max_history_size=10인 StoreAgent
When SetWithTTL("key1", "v1", 10s) 후 SetWithTTL("key1", "v2", 10s) 호출
Then GetHistory("key1")[0].Value == "v1"
And Get("key1").Value == "v2"
```

## Quality Gate

- [ ] 기존 Store 관련 테스트 전체 통과 (하위 호환성)
- [ ] 새로운 히스토리 테스트 전체 통과
- [ ] `go test -race ./internal/agent/system/...` 통과
- [ ] 히스토리 비활성(max_history_size=0) 시 성능 오버헤드 없음
- [ ] 웹 UI StoreTab에서 히스토리 정상 표시

## Definition of Done

- [ ] Module 1~4 (P0) 구현 및 테스트 완료
- [ ] Module 5~7 (P1) 구현 및 테스트 완료
- [ ] Module 8 (P2) 구현 및 테스트 완료
- [ ] 기존 테스트 100% 통과
- [ ] 신규 테스트 85%+ 커버리지
- [ ] 코드 리뷰 완료
