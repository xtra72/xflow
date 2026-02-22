---
id: SPEC-INFLUX-001
type: acceptance
version: "1.0.0"
created: "2026-02-22"
updated: "2026-02-22"
author: xtra
---

# SPEC-INFLUX-001 인수 기준: InfluxDB 연동 에이전트 구현

## 1. 인수 시나리오

### Scenario 1: 에이전트 타입 등록

```gherkin
Given xflowd 엔진이 초기화되는 경우
When 에이전트 매니저의 타입 등록이 수행되면
Then "influxdb" 타입이 에이전트 팩토리에 등록되어야 한다
And "influxdb" 타입으로 에이전트 생성이 가능해야 한다
```

### Scenario 2: 필수 설정 파싱

```gherkin
Given 에이전트 설정의 Transport.Options에 url, token, bucket, version이 모두 제공된 경우
When parseInfluxDBConfig()가 호출되면
Then 모든 필수 값이 올바르게 파싱되어야 한다
And InfluxDBConfig 구조체에 정확한 값이 설정되어야 한다
```

### Scenario 3: 필수 설정 누락 에러

```gherkin
Given 에이전트 설정에서 url이 누락된 경우
When parseInfluxDBConfig()가 호출되면
Then "url is required" 에러가 반환되어야 한다

Given 에이전트 설정에서 token이 누락된 경우
When parseInfluxDBConfig()가 호출되면
Then "token is required" 에러가 반환되어야 한다

Given 에이전트 설정에서 bucket이 누락된 경우
When parseInfluxDBConfig()가 호출되면
Then "bucket is required" 에러가 반환되어야 한다

Given 에이전트 설정에서 version이 누락된 경우
When parseInfluxDBConfig()가 호출되면
Then "version is required" 에러가 반환되어야 한다
```

### Scenario 4: 기본값 적용

```gherkin
Given 에이전트 설정에서 선택적 옵션이 제공되지 않은 경우
When parseInfluxDBConfig()가 호출되면
Then query_language 기본값은 version "2"에서 "flux"이어야 한다
And query_language 기본값은 version "3"에서 "sql"이어야 한다
And timeout_sec 기본값은 10이어야 한다
And buffer_size 기본값은 256이어야 한다
And batch_size 기본값은 1000이어야 한다
And flush_interval_ms 기본값은 1000이어야 한다
And precision 기본값은 "ns"이어야 한다
```

### Scenario 5: 잘못된 version 값

```gherkin
Given 에이전트 설정에서 version이 "1"로 제공된 경우
When parseInfluxDBConfig()가 호출되면
Then "version must be '2' or '3'" 에러가 반환되어야 한다
```

### Scenario 6: v2 클라이언트 생성

```gherkin
Given 파싱된 설정의 version이 "2"인 경우
When NewInfluxClient()가 호출되면
Then InfluxV2Client 인스턴스가 생성되어야 한다
And influxdb-client-go/v2 패키지 기반 클라이언트가 초기화되어야 한다
```

### Scenario 7: v3 클라이언트 생성

```gherkin
Given 파싱된 설정의 version이 "3"인 경우
When NewInfluxClient()가 호출되면
Then InfluxV3Client 인스턴스가 생성되어야 한다
And influxdb3-go/v2 패키지 기반 클라이언트가 초기화되어야 한다
```

### Scenario 8: 단건 쓰기 요청 처리

```gherkin
Given InfluxDB 에이전트가 초기화된 상태에서
When Process()에 다음 JSON이 전달되면:
  {"measurement": "temperature", "tags": {"host": "srv1"}, "fields": {"value": 25.5}, "timestamp": 1234567890000000000}
Then 에이전트는 InfluxDB에 measurement="temperature", tag host="srv1", field value=25.5로 쓰기 요청을 보내야 한다
And timestamp가 1234567890000000000 나노초로 설정되어야 한다
And 쓰기 성공 시 에러 없이 반환되어야 한다
```

### Scenario 9: 배치 쓰기 요청 처리

```gherkin
Given InfluxDB 에이전트가 초기화된 상태에서
When Process()에 JSON 배열이 전달되면:
  [{"measurement": "temp", "fields": {"value": 25.5}}, {"measurement": "temp", "fields": {"value": 26.1}}]
Then 에이전트는 2개의 포인트를 배치로 InfluxDB에 쓰기 요청을 보내야 한다
And 두 포인트 모두 성공적으로 저장되어야 한다
```

### Scenario 10: timestamp 미제공 시 현재 시간 사용

```gherkin
Given 쓰기 요청 JSON에서 timestamp 필드가 생략된 경우
When 쓰기 처리가 수행되면
Then 현재 시간이 timestamp로 자동 설정되어야 한다
```

### Scenario 11: 쿼리 요청 처리 (InfluxQL)

```gherkin
Given InfluxDB 에이전트가 v2로 초기화된 상태에서
When Process()에 다음 JSON이 전달되면:
  {"query": "SELECT mean(value) FROM temperature WHERE time > now() - 1h", "language": "influxql"}
Then 에이전트는 InfluxQL 쿼리를 실행해야 한다
And 쿼리 결과가 JSON 배열로 직렬화되어야 한다
And ReceiveMessage()로 결과를 수신할 수 있어야 한다
```

### Scenario 12: 쿼리 요청 처리 (SQL on v3)

```gherkin
Given InfluxDB 에이전트가 v3로 초기화된 상태에서
When Process()에 다음 JSON이 전달되면:
  {"query": "SELECT avg(value) FROM temperature WHERE time > now() - interval '1 hour'", "language": "sql"}
Then 에이전트는 SQL 쿼리를 Apache Arrow Flight 프로토콜로 실행해야 한다
And 쿼리 결과가 JSON 배열로 직렬화되어 recvCh에 전달되어야 한다
```

### Scenario 13: 쿼리 요청 처리 (Flux on v2)

```gherkin
Given InfluxDB 에이전트가 v2로 초기화된 상태에서
When Process()에 다음 JSON이 전달되면:
  {"query": "from(bucket:\"metrics\") |> range(start: -1h) |> mean()", "language": "flux"}
Then 에이전트는 Flux 쿼리를 실행해야 한다
And 쿼리 결과가 JSON 배열로 직렬화되어 recvCh에 전달되어야 한다
```

### Scenario 14: 쿼리 언어 미지정 시 기본값 사용

```gherkin
Given InfluxDB 에이전트가 v3로 초기화되고 기본 query_language가 "sql"인 상태에서
When Process()에 다음 JSON이 전달되면:
  {"query": "SELECT * FROM temperature LIMIT 10"}
Then language가 설정의 기본값 "sql"로 적용되어야 한다
And SQL 쿼리로 실행되어야 한다
```

### Scenario 15: 요청 타입 자동 판별 - 쿼리

```gherkin
Given JSON 데이터에 "query" 키가 존재하는 경우
When Process()가 호출되면
Then 쿼리 요청으로 판별되어야 한다
And 쿼리 처리 경로가 실행되어야 한다
```

### Scenario 16: 요청 타입 자동 판별 - 쓰기

```gherkin
Given JSON 데이터에 "measurement" 키가 존재하고 "query" 키가 없는 경우
When Process()가 호출되면
Then 쓰기 요청으로 판별되어야 한다
And 쓰기 처리 경로가 실행되어야 한다
```

### Scenario 17: 요청 타입 판별 불가

```gherkin
Given JSON 데이터에 "query"도 "measurement"도 없고 배열도 아닌 경우
When Process()가 호출되면
Then "unsupported request format" 에러가 반환되어야 한다
```

### Scenario 18: v2에서 SQL 쿼리 요청 시 에러

```gherkin
Given InfluxDB 에이전트가 v2로 초기화된 상태에서
When Process()에 language "sql" 쿼리가 전달되면
Then "InfluxDB 2.x에서는 SQL을 지원하지 않습니다. 'flux' 또는 'influxql'을 사용하세요." 에러가 반환되어야 한다
```

### Scenario 19: v3에서 Flux 쿼리 요청 시 에러

```gherkin
Given InfluxDB 에이전트가 v3로 초기화된 상태에서
When Process()에 language "flux" 쿼리가 전달되면
Then "InfluxDB 3.x에서는 Flux를 지원하지 않습니다. 'sql' 또는 'influxql'을 사용하세요." 에러가 반환되어야 한다
```

### Scenario 20: 에이전트 생명주기

```gherkin
Given InfluxDB 에이전트 설정이 유효한 경우
When Init()이 호출되면
Then InfluxDB 클라이언트가 생성되어야 한다
And Health 체크가 수행되어야 한다

When Start()가 호출되면
Then 에이전트가 활성 상태가 되어야 한다

When Stop()이 호출되면
Then InfluxDB 클라이언트 연결이 종료되어야 한다
And 미처리 데이터가 플러시되어야 한다
```

### Scenario 21: ReceiveMessage 타임아웃

```gherkin
Given 쿼리 결과가 recvCh에 없는 상태에서
When ReceiveMessage()가 취소된 컨텍스트와 함께 호출되면
Then context.Canceled 또는 context.DeadlineExceeded 에러가 반환되어야 한다
```

### Scenario 22: 에이전트 일시 중지 및 재개

```gherkin
Given 에이전트가 실행 중인 상태에서
When Pause()가 호출되면
Then Process() 호출 시 "agent is paused" 에러가 반환되어야 한다

When Resume()가 호출되면
Then Process() 호출이 정상적으로 처리되어야 한다
```

---

## 2. 엣지 케이스

### Edge Case 1: 쓰기 데이터에 필수 필드 누락

```gherkin
Given 쓰기 JSON에서 measurement가 누락된 경우
When Process()가 호출되면
Then "measurement is required" 에러가 반환되어야 한다

Given 쓰기 JSON에서 fields가 비어있는 경우
When Process()가 호출되면
Then "at least one field is required" 에러가 반환되어야 한다
```

### Edge Case 2: 빈 쿼리 문자열

```gherkin
Given 쿼리 JSON에서 query가 빈 문자열인 경우
When Process()가 호출되면
Then "query is required" 에러가 반환되어야 한다
```

### Edge Case 3: 잘못된 JSON 형식

```gherkin
Given Process()에 유효하지 않은 JSON 바이트가 전달된 경우
When JSON 파싱이 시도되면
Then "invalid JSON format" 에러가 반환되어야 한다
```

### Edge Case 4: recvCh 버퍼 풀

```gherkin
Given recvCh 버퍼가 가득 찬 상태에서
When 새로운 쿼리 결과가 전달되면
Then 쓰기 시도가 블로킹되지 않아야 한다
And 경고 로그가 기록되어야 한다
```

### Edge Case 5: nil 또는 빈 바이트 입력

```gherkin
Given Process()에 nil 또는 빈 바이트 슬라이스가 전달된 경우
When 처리가 시도되면
Then "empty data" 에러가 반환되어야 한다
```

### Edge Case 6: 쓰기 데이터 필드 타입 검증

```gherkin
Given 쓰기 JSON의 fields에 지원되지 않는 타입(예: 중첩 객체)이 포함된 경우
When 쓰기 처리가 수행되면
Then 적절한 에러 메시지가 반환되어야 한다
And 지원되는 타입(string, float64, int64, bool)이 안내되어야 한다
```

### Edge Case 7: v2에서 org 미설정

```gherkin
Given version "2" 설정에서 org가 누락된 경우
When parseInfluxDBConfig()가 호출되면
Then "org is required for InfluxDB 2.x" 에러가 반환되어야 한다
```

---

## 3. Quality Gate 기준

### 3.1 테스트 커버리지

- `influxdb_config.go`: 설정 파싱의 모든 분기 커버리지 100%
- `influxdb_agent.go`: Process() 쓰기/쿼리/판별 경로 커버리지 85% 이상
- `influxdb_client.go`: 팩토리 함수 분기 커버리지 100%
- 전체 InfluxDB 관련 파일 커버리지 85% 이상

### 3.2 검증 방법

| 검증 항목 | 방법 | 기준 |
|----------|------|------|
| Agent 인터페이스 준수 | 컴파일 타임 인터페이스 검증 (`var _ agent.Agent = (*InfluxDBAgent)(nil)`) | 컴파일 에러 없음 |
| MessageReceiver 인터페이스 준수 | 컴파일 타임 인터페이스 검증 | 컴파일 에러 없음 |
| 설정 파싱 정확성 | 필수/선택 설정의 테이블 드리븐 테스트 | 모든 케이스 통과 |
| 쓰기 요청 처리 | 목 클라이언트를 통한 Write 호출 검증 | 올바른 데이터 전달 |
| 쿼리 요청 처리 | 목 클라이언트를 통한 Query 호출 + recvCh 수신 검증 | 결과 JSON 일치 |
| 요청 타입 판별 | 다양한 JSON 입력에 대한 분기 테스트 | 100% 정확 판별 |
| 에러 처리 | 에러 메시지 포함 검증 | 명확한 에러 메시지 |
| 동시성 안전 | `go test -race` 플래그 | 레이스 컨디션 없음 |

### 3.3 Definition of Done

- [ ] `influxdb_client.go`: InfluxClient 인터페이스 및 팩토리 함수 구현 완료
- [ ] `influxdb_config.go`: 설정 파싱 구현 완료 (필수/선택 설정, 기본값, 유효성 검증)
- [ ] `influxdb_v2.go`: InfluxDB 2.x 어댑터 구현 완료 (Write, Query, Health, Close)
- [ ] `influxdb_v3.go`: InfluxDB 3.x 어댑터 구현 완료 (Write, Query, Health, Close)
- [ ] `influxdb_agent.go`: Agent + MessageReceiver 인터페이스 완전 구현
- [ ] `influxdb_register.go`: 타입 등록 함수 구현 완료
- [ ] `cmd/xflowd/main.go`: RegisterInfluxDBTypes 호출 추가
- [ ] `influxdb_config_test.go`: 설정 파싱 테스트 통과
- [ ] `influxdb_client_test.go`: 클라이언트 추상화 테스트 통과
- [ ] `influxdb_agent_test.go`: 에이전트 Process/ReceiveMessage 테스트 통과
- [ ] `go test -race ./internal/agent/system/...` 전체 통과
- [ ] v2에서 SQL 요청 시 명확한 에러 메시지 반환
- [ ] v3에서 Flux 요청 시 명확한 에러 메시지 반환
- [ ] `go.mod`에 InfluxDB 클라이언트 의존성 추가됨
- [ ] 예제 YAML 파일 작성 완료

---

## 4. 추적성

| 시나리오 | 요구사항 | 구현 위치 |
|----------|----------|----------|
| Scenario 1 | REQ-1 | `influxdb_register.go` - RegisterInfluxDBTypes() |
| Scenario 2, 3 | REQ-2 | `influxdb_config.go` - parseInfluxDBConfig() |
| Scenario 4 | REQ-2 | `influxdb_config.go` - 기본값 로직 |
| Scenario 5 | REQ-2 | `influxdb_config.go` - version 유효성 검증 |
| Scenario 6 | REQ-4 | `influxdb_v2.go` - InfluxV2Client |
| Scenario 7 | REQ-5 | `influxdb_v3.go` - InfluxV3Client |
| Scenario 8, 9, 10 | REQ-6 | `influxdb_agent.go` - Process() 쓰기 경로 |
| Scenario 11, 12, 13, 14 | REQ-7 | `influxdb_agent.go` - Process() 쿼리 경로 |
| Scenario 15, 16, 17 | REQ-8 | `influxdb_agent.go` - 요청 타입 판별 |
| Scenario 18, 19 | REQ-10 | `influxdb_v2.go`, `influxdb_v3.go` - 언어 검증 |
| Scenario 20 | REQ-9 | `influxdb_agent.go` - Init/Start/Stop |
| Scenario 21 | REQ-7, REQ-9 | `influxdb_agent.go` - ReceiveMessage() |
| Scenario 22 | REQ-9 | `influxdb_agent.go` - Pause/Resume |
| Edge Case 1-6 | REQ-6, REQ-7 | `influxdb_agent.go` - 입력 유효성 검증 |
| Edge Case 7 | REQ-2 | `influxdb_config.go` - v2 org 필수 검증 |
