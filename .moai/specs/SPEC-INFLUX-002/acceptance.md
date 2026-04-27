# SPEC-INFLUX-002: 인수 기준

> **SPEC ID**: SPEC-INFLUX-002
> **형식**: Given-When-Then (Gherkin)

---

## M1: influxdb-write 노드

### AC-M1-01: 고정 measurement로 쓰기

```gherkin
Given influxdb-write 노드가 measurement="temperature", field_mappings={value: "value"} 으로 설정되었을 때
And   agent_ref가 유효한 InfluxDB 에이전트를 참조할 때
When  payload={value: 22.5, location: "room1"} 메시지가 입력되면
Then  에이전트의 Process에 WriteData JSON이 전달되어야 한다
And   WriteData.Measurement이 "temperature"여야 한다
And   WriteData.Fields가 {value: 22.5}여야 한다
And   원본 메시지가 그대로 반환되어야 한다 (pass-through)
```

### AC-M1-02: measurement_key로 동적 measurement

```gherkin
Given influxdb-write 노드가 measurement_key="metric_name" 으로 설정되었을 때
When  payload={metric_name: "cpu_usage", value: 85.2} 메시지가 입력되면
Then  WriteData.Measurement이 "cpu_usage"여야 한다
```

### AC-M1-03: tag_mappings 적용

```gherkin
Given influxdb-write 노드가 tag_mappings={host: "hostname", region: "region"} 으로 설정되었을 때
When  payload={hostname: "server-1", region: "ap-northeast-2", value: 42} 메시지가 입력되면
Then  WriteData.Tags가 {host: "server-1", region: "ap-northeast-2"}여야 한다
```

### AC-M1-04: field_mappings 미지정 시 전체 payload

```gherkin
Given influxdb-write 노드가 measurement="sensor", field_mappings가 비어있을 때
When  payload={temperature: 22.5, humidity: 65.0} 메시지가 입력되면
Then  WriteData.Fields가 {temperature: 22.5, humidity: 65.0}여야 한다
```

### AC-M1-05: 빈 measurement 에러

```gherkin
Given influxdb-write 노드가 measurement과 measurement_key가 모두 비어있을 때
When  payload={value: 10} 메시지가 입력되면
Then  에러가 반환되어야 한다
And   에러 메시지에 "measurement" 관련 내용이 포함되어야 한다
```

### AC-M1-06: 빈 fields 건너뛰기

```gherkin
Given influxdb-write 노드가 measurement="test", field_mappings={value: "value"} 으로 설정되었을 때
When  payload에 "value" 키가 없는 메시지가 입력되면
Then  쓰기를 건너뛰어야 한다
And   원본 메시지가 그대로 반환되어야 한다
```

### AC-M1-07: agent.Process 실패 시 에러 전파

```gherkin
Given influxdb-write 노드가 정상 설정되었을 때
And   에이전트의 Process가 에러를 반환하도록 설정되었을 때
When  유효한 메시지가 입력되면
Then  에러가 반환되어야 한다
And   에러에 원본 에러 메시지가 포함되어야 한다
```

### AC-M1-08: timestamp_key 적용

```gherkin
Given influxdb-write 노드가 timestamp_key="ts" 으로 설정되었을 때
When  payload={ts: 1700000000000, value: 10} 메시지가 입력되면
Then  WriteData.Timestamp이 1700000000000이어야 한다
```

### AC-M1-09: agent_ref 미지정 에러

```gherkin
Given influxdb-write 노드의 agent_ref가 nil일 때
When  Init을 호출하면
Then  agent_ref 관련 에러가 반환되어야 한다
```

---

## M2: influxdb-read 노드 (SourceNode)

### AC-M2-01: 폴링 쿼리 실행

```gherkin
Given influxdb-read 노드가 query="SELECT * FROM cpu LIMIT 10", language="influxql", poll_interval="5s" 으로 설정되었을 때
When  폴링 주기가 도달하면
Then  에이전트의 Process에 QueryRequest JSON이 전달되어야 한다
And   QueryRequest.Query가 "SELECT * FROM cpu LIMIT 10"이어야 한다
And   QueryRequest.Language가 "influxql"이어야 한다
```

### AC-M2-02: 쿼리 결과를 개별 메시지로 변환

```gherkin
Given 에이전트의 ReceiveMessage가 3개 행의 JSON 배열을 반환할 때
When  influxdb-read 노드가 결과를 처리하면
Then  SourceCh에 3개의 개별 메시지가 출력되어야 한다
And   각 메시지의 payload에 해당 행의 데이터가 포함되어야 한다
```

### AC-M2-03: 빈 결과 무시

```gherkin
Given 에이전트의 ReceiveMessage가 빈 배열 []을 반환할 때
When  influxdb-read 노드가 결과를 처리하면
Then  SourceCh에 메시지가 출력되지 않아야 한다
```

### AC-M2-04: 쿼리 실패 시 에러 복구

```gherkin
Given 에이전트의 Process가 에러를 반환할 때
When  influxdb-read 노드의 폴링이 실행되면
Then  에러가 로그에 기록되어야 한다
And   노드가 종료되지 않고 다음 폴링 주기까지 대기해야 한다
```

### AC-M2-05: Shutdown 시 정상 종료

```gherkin
Given influxdb-read 노드가 폴링 중일 때
When  Shutdown을 호출하면
Then  폴링 루프가 정상 종료되어야 한다
And   SourceCh가 닫혀야 한다
```

### AC-M2-06: SourceNode 인터페이스 구현

```gherkin
Given influxdb-read 노드가 초기화되었을 때
When  SourceCh() 메서드를 호출하면
Then  읽기 전용 메시지 채널이 반환되어야 한다
```

### AC-M2-07: 기본 설정값 확인

```gherkin
Given influxdb-read 노드에서 language, poll_interval, timeout이 생략되었을 때
When  Configure를 호출하면
Then  language가 "flux"여야 한다
And   pollInterval이 30초여야 한다
And   timeout이 10초여야 한다
```

---

## M3: influxdb-query 노드

### AC-M3-01: 고정 쿼리 실행

```gherkin
Given influxdb-query 노드가 query="SELECT mean(value) FROM temperature GROUP BY time(1h)", language="influxql" 으로 설정되었을 때
When  임의의 메시지가 입력되면
Then  에이전트의 Process에 해당 쿼리가 전달되어야 한다
And   결과가 새 메시지의 payload "results" 키에 저장되어야 한다
```

### AC-M3-02: payload에서 쿼리 추출

```gherkin
Given influxdb-query 노드의 query 설정이 비어있을 때
When  payload={query: "SELECT * FROM cpu LIMIT 5"} 메시지가 입력되면
Then  payload의 query 값으로 쿼리가 실행되어야 한다
```

### AC-M3-03: $variable 치환 -- 문자열

```gherkin
Given influxdb-query 노드가 query="SELECT * FROM $measurement WHERE host=$host" 으로 설정되었을 때
When  payload={measurement: "cpu", host: "server-1"} 메시지가 입력되면
Then  치환된 쿼리가 "SELECT * FROM cpu WHERE host='server-1'"이어야 한다
```

### AC-M3-04: $variable 치환 -- 숫자

```gherkin
Given influxdb-query 노드가 query="SELECT * FROM cpu LIMIT $limit" 으로 설정되었을 때
When  payload={limit: 100} 메시지가 입력되면
Then  치환된 쿼리가 "SELECT * FROM cpu LIMIT 100"이어야 한다
```

### AC-M3-05: $variable 미발견 시 원본 유지

```gherkin
Given influxdb-query 노드가 query="SELECT * FROM $table WHERE id=$id" 으로 설정되었을 때
When  payload={table: "cpu"} 메시지가 입력되면 (id 없음)
Then  치환된 쿼리가 "SELECT * FROM cpu WHERE id=$id"여야 한다
```

### AC-M3-06: 빈 쿼리 에러

```gherkin
Given influxdb-query 노드의 query 설정이 비어있을 때
When  payload에 query 키가 없는 메시지가 입력되면
Then  에러가 반환되어야 한다
And   에러 메시지에 "query" 관련 내용이 포함되어야 한다
```

### AC-M3-07: result_key 설정

```gherkin
Given influxdb-query 노드가 result_key="data" 으로 설정되었을 때
When  쿼리가 성공하면
Then  결과가 payload의 "data" 키에 저장되어야 한다
And   "results" 키에는 저장되지 않아야 한다
```

### AC-M3-08: 쿼리 실패 시 에러 전파

```gherkin
Given 에이전트의 Process가 쿼리 에러를 반환할 때
When  influxdb-query 노드에 메시지가 입력되면
Then  에러가 반환되어야 한다
```

### AC-M3-09: 입력 메타데이터 보존

```gherkin
Given 입력 메시지에 메타데이터 {source: "sensor", flow_id: "f1"}이 있을 때
When  influxdb-query 노드가 쿼리를 실행하고 결과 메시지를 생성하면
Then  결과 메시지에 influxdb 관련 메타데이터가 추가되어야 한다
And   influxdb_source, influxdb_language 메타데이터가 포함되어야 한다
```

---

## M4: 노드 레지스트리 및 Web UI

### AC-M4-01: 노드 레지스트리 등록

```gherkin
Given 노드 레지스트리가 초기화될 때
When  등록 테이블이 로드되면
Then  "influxdb-write", "influxdb-read", "influxdb-query" 노드가 등록되어야 한다
And   각 노드의 카테고리가 "storage"여야 한다
```

### AC-M4-02: 레지스트리 노드 수 갱신

```gherkin
Given 노드 레지스트리의 빌트인 노드 수가 주석에 기록되어 있을 때
When  3개 InfluxDB 노드가 추가되면
Then  빌트인 노드 수가 36에서 39로 갱신되어야 한다
```

### AC-M4-03: Web UI 노드 스키마 등록

```gherkin
Given nodeSchemas.ts의 노드 타입 목록
When  UI에서 노드 생성 폼을 열면
Then  influxdb-write, influxdb-read, influxdb-query 노드 타입이 표시되어야 한다
And   agent_select 필드의 옵션에 'influxdb'가 포함되어야 한다
```

### AC-M4-04: influxdb-read 스키마 필수 필드

```gherkin
Given influxdb-read 노드 스키마
When  설정 폼이 렌더링되면
Then  query 필드가 필수로 표시되어야 한다
And   language 선택 필드에 flux, sql, influxql 옵션이 있어야 한다
And   poll_interval 기본값이 "30s"여야 한다
```

### AC-M4-05: influxdb-query 스키마 변수 치환 안내

```gherkin
Given influxdb-query 노드 스키마
When  query 필드가 렌더링되면
Then  $variable 치환 문법에 대한 도움말이 표시되어야 한다
```

---

## 품질 게이트 (Definition of Done)

### 필수 통과 조건

- [ ] 모든 인수 기준(AC-*) 테스트 통과
- [ ] `go test -race ./internal/node/...` 통과
- [ ] 테스트 커버리지 85% 이상 (신규 파일 기준)
- [ ] `go vet ./...` 경고 없음
- [ ] 노드 레지스트리 테스트에서 빌트인 노드 수 검증 (39개)
- [ ] Web UI에서 3개 노드 타입이 정상 표시

### 선택 통과 조건

- [ ] influxdb-write 배치 쓰기 지원 (다수 WriteData 한번에 전송)
- [ ] influxdb-query 쿼리 캐싱 (동일 쿼리 반복 시 캐시 적용)
- [ ] 실제 InfluxDB 인스턴스 연동 통합 테스트
