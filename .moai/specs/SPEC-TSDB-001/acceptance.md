---
id: SPEC-TSDB-001
type: acceptance
version: "1.0.0"
created: "2026-03-19"
updated: "2026-03-19"
---

# SPEC-TSDB-001: 인수 기준 (Acceptance Criteria)

## 1. Module 1: Core TSDB Engine

### Scenario 1.1: 기본 데이터 기록 및 조회

```gherkin
Given TSDB 인스턴스가 초기화되어 있다
When measurement="temperature", tags={"room": "101"}, fields={"value": 25.5}로 DataPoint를 기록한다
Then 시리즈 키 "temperature,room=101"이 자동 생성된다
And 해당 시리즈에서 기록된 DataPoint를 조회할 수 있다
And DataPoint의 fields["value"]가 25.5이다
```

### Scenario 1.2: 시리즈 키 정규화

```gherkin
Given TSDB 인스턴스가 초기화되어 있다
When tags={"floor": "2", "building": "A", "room": "101"}로 DataPoint를 기록한다
Then 시리즈 키가 "temperature,building=A,floor=2,room=101" (알파벳 순)로 생성된다
And 동일 태그를 다른 순서로 기록해도 같은 시리즈에 저장된다
```

### Scenario 1.3: 시간순 정렬 저장

```gherkin
Given 시리즈 "temperature,room=101"에 10개의 DataPoint가 저장되어 있다
When 모든 DataPoint를 조회한다
Then 타임스탬프 기준 오름차순으로 정렬되어 반환된다
```

### Scenario 1.4: Out-of-Order 삽입

```gherkin
Given 시리즈에 T1, T3 타임스탬프의 DataPoint가 저장되어 있다
When T2 타임스탬프(T1 < T2 < T3)의 DataPoint를 기록한다
Then DataPoint가 T1, T2, T3 순서로 정렬되어 저장된다
```

### Scenario 1.5: 동시 쓰기 안전성

```gherkin
Given TSDB 인스턴스가 초기화되어 있다
When 10개의 고루틴이 동시에 같은 시리즈에 각 100개의 DataPoint를 기록한다
Then 총 1,000개의 DataPoint가 데이터 레이스 없이 저장된다
And go test -race가 통과한다
```

### Scenario 1.6: 동시 읽기/쓰기

```gherkin
Given 시리즈에 데이터가 저장되어 있다
When 5개의 고루틴이 읽기를 수행하면서 동시에 2개의 고루틴이 쓰기를 수행한다
Then 읽기 결과는 일관성 있는 스냅샷이다
And 데이터 레이스가 발생하지 않는다
```

---

## 2. Module 2: Retention Policies

### Scenario 2.1: 시간 기반 보존

```gherkin
Given 시리즈에 MaxAge=1h 보존 정책이 설정되어 있다
And 2시간 전, 1시간 전, 30분 전, 현재 시각의 DataPoint가 저장되어 있다
When 백그라운드 퇴거가 실행된다
Then 2시간 전 DataPoint는 삭제된다
And 1시간 전, 30분 전, 현재 시각의 DataPoint는 유지된다
```

### Scenario 2.2: 개수 기반 보존

```gherkin
Given 시리즈에 MaxPoints=100 보존 정책이 설정되어 있다
And 현재 100개의 DataPoint가 저장되어 있다
When 새로운 DataPoint를 1개 기록한다
Then 가장 오래된 DataPoint 1개가 삭제된다
And 총 DataPoint 수는 100개이다
```

### Scenario 2.3: 용량 기반 보존

```gherkin
Given 글로벌 MaxBytes=1MB 보존 정책이 설정되어 있다
And 현재 추정 메모리 사용량이 900KB이다
When 200KB 분량의 DataPoint를 기록한다
Then 가장 오래된 시리즈의 가장 오래된 데이터부터 삭제하여 1MB 이하로 유지한다
```

### Scenario 2.4: 시리즈별 정책 우선순위

```gherkin
Given 글로벌 보존 정책이 MaxAge=24h이다
And 시리즈 "temperature,room=101"에 MaxAge=7d 정책이 설정되어 있다
When 48시간 전 DataPoint에 대해 퇴거를 실행한다
Then "temperature,room=101" 시리즈의 48시간 전 데이터는 유지된다 (7d 정책)
And 시리즈별 정책이 없는 다른 시리즈의 48시간 전 데이터는 삭제된다 (24h 정책)
```

### Scenario 2.5: Lazy Eviction

```gherkin
Given 시리즈에 만료된 DataPoint가 존재한다
When 해당 시리즈에 새로운 DataPoint를 기록한다
Then 기록 전에 만료된 DataPoint가 정리된다
And 새 DataPoint가 정상 저장된다
```

### Scenario 2.6: 백그라운드 퇴거 주기

```gherkin
Given 퇴거 주기가 30초로 설정되어 있다
And 만료된 DataPoint가 여러 시리즈에 존재한다
When 30초가 경과한다
Then 백그라운드 고루틴이 모든 시리즈를 순회하며 만료 데이터를 정리한다
And 메모리 사용량 추정치가 업데이트된다
```

---

## 3. Module 3: Query Engine

### Scenario 3.1: 시간 범위 쿼리

```gherkin
Given 시리즈 "temperature,room=101"에 24시간 동안 1분 간격으로 1,440개의 DataPoint가 저장되어 있다
When 최근 1시간 범위로 쿼리한다
Then 60개의 DataPoint가 반환된다
And 모든 DataPoint의 타임스탬프가 최근 1시간 내이다
```

### Scenario 3.2: 최신 N개 조회

```gherkin
Given 시리즈에 500개의 DataPoint가 저장되어 있다
When 최신 50개를 요청한다
Then 가장 최근 50개의 DataPoint가 시간순으로 반환된다
```

### Scenario 3.3: 집계 - 평균값

```gherkin
Given 시리즈에 fields={"value": 10}, {"value": 20}, {"value": 30}인 3개의 DataPoint가 있다
When "value" 필드에 대해 avg 집계를 실행한다
Then 결과는 20.0이다
```

### Scenario 3.4: 집계 - 최솟값/최댓값

```gherkin
Given 시리즈에 fields={"value": 5}, {"value": 15}, {"value": 10}인 3개의 DataPoint가 있다
When "value" 필드에 대해 min 집계를 실행한다
Then 결과는 5.0이다
When "value" 필드에 대해 max 집계를 실행한다
Then 결과는 15.0이다
```

### Scenario 3.5: 다운샘플링

```gherkin
Given 시리즈에 1분 간격으로 60개의 DataPoint가 저장되어 있다 (1시간분)
When 15분 버킷 간격으로 avg 다운샘플링 쿼리를 실행한다
Then 4개의 버킷 결과가 반환된다
And 각 버킷에는 15개 포인트의 평균값이 포함된다
```

### Scenario 3.6: 시리즈 목록 필터링

```gherkin
Given 다음 시리즈가 존재한다:
  | "temperature,room=101,floor=1" |
  | "temperature,room=201,floor=2" |
  | "humidity,room=101,floor=1"    |
When measurement="temperature"로 필터링한다
Then 2개의 시리즈가 반환된다
When tag "floor"="1"로 필터링한다
Then "temperature,room=101,floor=1"과 "humidity,room=101,floor=1"이 반환된다
```

### Scenario 3.7: 쿼리 결과 제한

```gherkin
Given 시리즈에 50,000개의 DataPoint가 저장되어 있다
And 최대 반환 제한이 10,000이다
When 전체 범위 쿼리를 실행한다
Then 최대 10,000개의 DataPoint만 반환된다
And 응답에 "truncated" 표시가 포함된다
```

---

## 4. Module 4: xflow Node Integration

### Scenario 4.1: tsdb-write 노드 기본 동작

```gherkin
Given tsdb-write 노드가 다음 설정으로 구성되어 있다:
  | measurement | "sensor_data" |
  | tag_keys    | ["device_id", "location"] |
  | field_keys  | ["temperature", "humidity"] |
When Payload에 {"device_id": "dev-001", "location": "room1", "temperature": 25.5, "humidity": 60.2}인 Message가 입력된다
Then TSDB에 measurement="sensor_data", tags={"device_id": "dev-001", "location": "room1"}, fields={"temperature": 25.5, "humidity": 60.2}로 기록된다
And 원본 Message가 출력 포트로 그대로 전달된다
```

### Scenario 4.2: tsdb-query 노드 기본 동작

```gherkin
Given tsdb-query 노드가 다음 설정으로 구성되어 있다:
  | measurement | "sensor_data" |
  | time_range  | "1h" |
  | aggregation | "avg" |
  | field       | "temperature" |
When 트리거 Message가 입력된다
Then TSDB에서 최근 1시간의 "sensor_data" 시리즈에 대해 avg 집계를 실행한다
And 결과가 출력 Message의 Payload에 {"result": [...]} 형태로 포함된다
```

### Scenario 4.3: tsdb-write 에러 처리

```gherkin
Given tsdb-write 노드가 구성되어 있다
When 필수 필드가 누락된 Message가 입력된다
Then 에러 포트로 에러 메시지가 전달된다
And 에러 메시지에 원본 Message와 에러 원인이 포함된다
```

### Scenario 4.4: 노드 레지스트리 등록

```gherkin
Given xflowd 서버가 시작된다
When 노드 타입 목록을 조회한다
Then "tsdb-write"와 "tsdb-query" 타입이 레지스트리에 등록되어 있다
And 각 타입의 설정 스키마가 제공된다
```

---

## 5. Module 5: API & WebSocket

### Scenario 5.1: 배치 쓰기 API

```gherkin
Given TSDB가 활성화되어 있다
When POST /api/v1/tsdb/write로 다음 요청을 전송한다:
  | measurement | tags | fields | timestamp |
  | "temperature" | {"room": "101"} | {"value": 25.5} | "2026-03-19T10:00:00Z" |
  | "temperature" | {"room": "101"} | {"value": 26.0} | "2026-03-19T10:01:00Z" |
Then 200 OK가 반환된다
And 2개의 DataPoint가 시리즈 "temperature,room=101"에 저장된다
```

### Scenario 5.2: 쿼리 API

```gherkin
Given 시리즈 "temperature,room=101"에 데이터가 저장되어 있다
When POST /api/v1/tsdb/query로 다음 요청을 전송한다:
  | series | "temperature,room=101" |
  | start  | "2026-03-19T09:00:00Z" |
  | end    | "2026-03-19T11:00:00Z" |
Then 200 OK와 해당 시간 범위의 DataPoint 배열이 반환된다
```

### Scenario 5.3: TSDB 통계 API

```gherkin
Given 여러 시리즈에 데이터가 저장되어 있다
When GET /api/v1/tsdb/stats를 요청한다
Then 다음 정보가 반환된다:
  | series_count | 활성 시리즈 수 |
  | total_points | 전체 DataPoint 수 |
  | memory_bytes | 추정 메모리 사용량 |
  | oldest_point | 가장 오래된 DataPoint 시각 |
```

### Scenario 5.4: 시리즈 삭제 API

```gherkin
Given 시리즈 "temperature,room=101"에 데이터가 저장되어 있다
When DELETE /api/v1/tsdb/series/temperature,room=101을 요청한다
Then 200 OK가 반환된다
And 해당 시리즈의 모든 데이터가 삭제된다
And 시리즈 목록에서 제거된다
```

### Scenario 5.5: WebSocket 실시간 구독

```gherkin
Given WebSocket으로 /api/v1/tsdb/subscribe에 연결되어 있다
And {"action": "subscribe", "series": ["temperature,room=101"]} 메시지를 전송했다
When POST /api/v1/tsdb/write로 "temperature,room=101" 시리즈에 새 DataPoint를 기록한다
Then WebSocket을 통해 실시간으로 새 DataPoint가 수신된다
And DataPoint에 시리즈 키, 타임스탬프, 필드가 포함된다
```

### Scenario 5.6: WebSocket 구독 해제

```gherkin
Given WebSocket으로 "temperature,room=101" 시리즈를 구독 중이다
When {"action": "unsubscribe", "series": ["temperature,room=101"]} 메시지를 전송한다
Then 이후 해당 시리즈에 데이터가 기록되어도 WebSocket으로 전달되지 않는다
```

### Scenario 5.7: 쿼리 타임아웃

```gherkin
Given 쿼리 타임아웃이 10초로 설정되어 있다
When 매우 큰 범위의 쿼리가 10초 이상 소요된다
Then 408 Request Timeout 응답이 반환된다
And TSDB 내부 상태는 일관성을 유지한다
```

---

## 6. Quality Gate

### 6.1 테스트 커버리지

| 패키지 | 최소 커버리지 |
|--------|-------------|
| internal/tsdb/ | 85% |
| internal/node/adapter/ (tsdb) | 85% |
| internal/api/handler/ (tsdb) | 80% |

### 6.2 레이스 디텍터

모든 테스트가 `go test -race` 통과 필수.

### 6.3 성능 기준

| 지표 | 목표 |
|------|------|
| 단일 시리즈 쓰기 처리량 | >= 100,000 points/sec |
| 범위 쿼리 (1,000 포인트) | < 1ms |
| 집계 쿼리 (10,000 포인트) | < 5ms |
| 메모리 오버헤드 (100만 포인트) | < 200MB |

### 6.4 Definition of Done

- [ ] Module 1~5의 모든 요구사항 구현
- [ ] 모든 인수 기준 시나리오 통과
- [ ] `go test -race ./internal/tsdb/...` 통과
- [ ] `go test -race ./internal/node/adapter/...` 통과 (tsdb 관련)
- [ ] `go test -race ./internal/api/handler/...` 통과 (tsdb 관련)
- [ ] 테스트 커버리지 85% 이상 (internal/tsdb/)
- [ ] golangci-lint 경고 0건
- [ ] go vet 에러 0건
- [ ] TSDB 설정이 xflow 설정 체계(Viper)에 통합됨
- [ ] 노드 타입이 레지스트리에 등록되어 CLI/Web에서 조회 가능

---

*문서 버전: 1.0.0*
*최종 수정: 2026-03-19*
