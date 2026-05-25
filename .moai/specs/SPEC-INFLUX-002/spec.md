# SPEC-INFLUX-002: InfluxDB 전용 플로우 노드 구현

> **SPEC ID**: SPEC-INFLUX-002
> **제목**: InfluxDB 전용 플로우 노드 (influxdb-write, influxdb-read, influxdb-query)
> **생성일**: 2026-04-13
> **수정일**: 2026-05-21
> **상태**: Implemented (v1.5.0 — Timestamp 단위 버그 수정)
> **우선순위**: High
> **추적성**: SPEC-INFLUX-001 (InfluxDB 에이전트, completed)

---

## 변경 이력 (Change History)

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-04-13 | v1.0.0 | 초기 구현 (influxdb-write/read/query) |
| 2026-05-22 | v1.5.0 | **FIX — Timestamp 단위 버그 수정 (v0.16.5)**. 이전: `influxdb_v2.go` / `influxdb_v3.go` 가 `WriteData.Timestamp` 를 항상 `time.Unix(0, ts)` 로 처리 → 나노초로 해석. 그러나 v0.14.0 이후 influxdb-write 노드는 `msg.Timestamp().UnixMilli()` (밀리초) 를 전달 → 1000× 오차로 1970년 근처 시각이 InfluxDB 에 기록됨 ("influxdb write 안됨" 사용자 보고). **수정**: `internal/agent/system/influxdb_client.go` 에 `timestampToTime(ts, precision)` 헬퍼 추가 — Precision 설정 ("ns"/"us"/"ms"/"s") 에 따라 올바른 단위로 변환. v2/v3 클라이언트가 `precision` 을 보관하고 사용. 기본 Precision 을 "ns" → "ms" 로 변경 (노드 전달 단위와 일치). 회귀 테스트 `TestTimestampToTime` 추가 (4개 precision 모두 검증). **다운스트림 영향**: 기본 동작에서는 호환성 회복 (msg.Timestamp() 가 정상 기록됨). 명시적으로 precision: ns 를 설정하고 ns 단위 timestamp_key 를 사용하던 경우는 그대로 동작. |
| 2026-05-22 | v1.4.0 | **InfluxDB agent debug 옵션 추가 (v0.16.4)**. `debug: true` 설정 시 InfluxDB 로 전송되는 write (measurement / tags / fields / timestamp) 와 query (language / query string) 요청이 DEBUG 레벨 로그로 출력. 운영 환경에서는 false 권장 (로그 부하). `InfluxDBConfig.Debug bool` 필드 추가, parseInfluxDBConfig 가 `debug` 옵션 파싱. processWriteSingle / processWriteBatch / processQuery 에서 debug=true 시 logger.Debug 호출. web schema 에 debug boolean 필드 추가. |
| 2026-05-22 | v1.3.0 | **`tag_mappings` 를 map 형식으로 (v0.16.3)**. InfluxDB tag name → metadata key 매핑 (rename 지원). v0.14.2 의 list 형식 (rename 없음) 보다 유연. 예: `tag_mappings: { device: dev_id, kind: device_type }` → metadata.dev_id 가 tag "device" 로, metadata.device_type 이 tag "kind" 로 매핑됨. 비어있으면 모든 metadata 가 동일 이름으로 tag 로 매핑됨 (v0.14.0 기본 동작 유지). 호환: list 형식 (`tag_keys` / `tags`) 도 여전히 인식 — 이 경우 tag name = metadata key (v0.14.2 동작). 다운스트림: v0.14.2 list 형식 사용 중인 플로우는 변경 없이 동작. rename 이 필요하면 map 형식으로 작성. |
| 2026-05-21 | v1.2.0 | **BREAKING — `tag_mappings` (map) 를 `tag_keys` (list) 로 단순화 (v0.14.2)**. tag 이름은 metadata 키와 동일하므로 매핑 (tag_name → JSONPath) 이 불필요. 새 config: `tag_keys: [dev_id, device_type]` — 지정된 metadata 키만 tag 로 포함. 비어있거나 미지정 시 모든 metadata 를 tag 로 사용 (v0.14.0 기본 동작 유지). 호환: 기존 키 이름 `tag_mappings` 또는 `tags` 도 list 형식으로 입력하면 허용. payload/type/timestamp 를 tag 로 쓰던 경우는 metadata 로 미리 옮긴 후 tag_keys 에 지정 필요. field_mappings 는 JSONPath 매핑 유지 (payload 구조 변환 필요성). |
| 2026-05-21 | v1.1.0 | **influxdb-write 입력 매핑 정책 변경 (v0.14.0)**. 기본 동작: tag_mappings 미지정 시 모든 metadata 를 tags 로, field_mappings 미지정 시 전체 payload 를 fields 로, timestamp_key 미지정 시 msg.Timestamp() 를 timestamp 로 사용. tag_mappings/field_mappings/measurement_key/timestamp_key 값은 JSONPath 문법 (`$.metadata.X`, `$.payload.X`, `$.type`, `$.timestamp`, `$.id`) 지원 — `resolveTemplateExpr` 헬퍼 재사용. legacy 표기 (`$.` prefix 없음) 은 payload 직접 key 로 후방 호환. 다운스트림 영향: 기본 동작 변경 (이전엔 tags 가 빈 상태에서 시작) — 사용자가 tag_mappings 를 명시적으로 지정하지 않은 플로우는 v0.14.0 부터 모든 metadata 가 InfluxDB tags 로 자동 매핑됨. |

---

## 1. Environment (환경)

### 1.1 프로젝트 컨텍스트

- **프로젝트**: xflow (Go 모듈: `github.com/xtra/xflow`)
- **대상 시스템**: InfluxDB v2 / v3 시계열 데이터베이스
- **기존 구현**:
  - InfluxDB 에이전트: `internal/agent/system/influxdb_agent.go` (v2/v3, Write/Query/Health)
  - InfluxDB 클라이언트: `internal/agent/system/influxdb_client.go` (InfluxClient 인터페이스)
  - TSDB 노드 패턴: `internal/node/tsdb_write.go`, `tsdb_query.go` (인메모리 TSDB용)
  - LGCNP 노드 패턴: `internal/node/lgcnp.go` (SourceNode 폴링 패턴)
- **현재 상황**: InfluxDB 에이전트는 존재하나 전용 플로우 노드가 없어, 사용자가 bridge 노드를 통해 간접적으로 상호작용해야 하는 불편이 있음

### 1.2 기술 스택

- **언어**: Go 1.23+
- **라이프사이클**: `pkg/lifecycle.BaseLifecycle`
- **메시지 시스템**: `pkg/message.Message`
- **노드 프레임워크**: `internal/node.Node`, `internal/node.SourceNode` 인터페이스
- **에이전트 해석**: `internal/node.AgentResolver`, `internal/node.AgentAccessor`
- **Web UI**: React + TypeScript (`web/src/config/`)

### 1.3 InfluxDB 에이전트 Process 커맨드 체계

에이전트의 `Process(data []byte)` 메서드는 JSON 입력을 자동 판별하여 처리한다:

| 입력 형식 | 판별 기준 | 동작 |
|----------|----------|------|
| JSON 배열 | `[]json.RawMessage` 파싱 성공 | 배치 쓰기 (`processWriteBatch`) |
| JSON 객체 + `"measurement"` 키 | `obj["measurement"]` 존재 | 단일 쓰기 (`processWriteSingle`) |
| JSON 객체 + `"query"` 키 | `obj["query"]` 존재 | 쿼리 실행 (`processQuery`) |

쿼리 결과는 `Process()` 반환값이 아닌 `recvCh` 채널을 통해 `ReceiveMessage(ctx)`로 비동기 수신된다.

### 1.4 InfluxClient 인터페이스

```go
type InfluxClient interface {
    Write(ctx context.Context, data []WriteData) error
    Query(ctx context.Context, query string, lang string) ([]map[string]any, error)
    Health(ctx context.Context) error
    Close() error
}

type WriteData struct {
    Measurement string            `json:"measurement"`
    Tags        map[string]string `json:"tags,omitempty"`
    Fields      map[string]any    `json:"fields"`
    Timestamp   *int64            `json:"timestamp,omitempty"`
}
```

### 1.5 기존 TSDB 노드와의 차이점

| 항목 | TSDB 노드 | InfluxDB 노드 |
|------|----------|--------------|
| 백엔드 | 인메모리 TSDB (내부) | InfluxDB v2/v3 (외부) |
| 에이전트 접근 | `tsdbProvider` -> `TSDB()` 직접 호출 | `influxdbAgent` -> `Process(json)` 호출 |
| 쓰기 | `db.Write(measurement, tags, fields)` | JSON 직렬화 -> `agent.Process(writeJSON)` |
| 쿼리 | `db.Execute(Query{})` | JSON 직렬화 -> `agent.Process(queryJSON)` + `ReceiveMessage()` |
| 쿼리 언어 | 내부 Query 구조체 | Flux (v2), SQL (v3), InfluxQL (v2/v3) |
| 폴링 노드 | 없음 | influxdb-read (SourceNode) |

---

## 2. Assumptions (가정)

### 2.1 에이전트 가정

- [A-01] InfluxDB 에이전트의 `Process()` 메서드가 안정적으로 쓰기/쿼리를 처리한다.
- [A-02] 쿼리 결과는 `ReceiveMessage(ctx)` 를 통해 비동기로 수신되며, JSON `[]map[string]any` 형태이다.
- [A-03] 에이전트의 `Process()`는 쓰기 성공 시 `(nil, nil)` 반환, 쿼리 성공 시 `(nil, nil)` + recvCh 전송.
- [A-04] 에이전트가 paused 상태이면 모든 Process 호출이 거부된다.

### 2.2 구현 가정

- [A-05] 기존 TSDB 노드의 `AgentResolver` + `AgentAccessor` 패턴을 재사용한다.
- [A-06] InfluxDB 에이전트에 대한 타입 단언은 `influxdbAgent` 로컬 인터페이스를 통해 수행한다.
- [A-07] influxdb-read 노드는 기존 `lgcnp-status` 노드의 SourceNode 폴링 패턴을 따른다.
- [A-08] influxdb-query 노드는 입력 메시지 payload에서 쿼리를 구성하거나 고정 설정 쿼리를 사용한다.
- [A-09] 쿼리 언어는 InfluxDB 버전에 따라 다르다: v2=flux/influxql, v3=sql/influxql.

### 2.3 Web UI 가정

- [A-10] `nodeSchemas.ts`와 `nodeTypeMeta.ts`에 신규 노드 스키마를 추가해야 한다.
- [A-11] `agent_select` 옵션에 `influxdb` 에이전트 타입이 이미 등록되어 있다.

---

## 3. Requirements (요구사항) -- EARS 형식

### M1: influxdb-write 노드

**[REQ-M1-01]** 시스템은 **항상** `influxdb-write` 노드 타입을 제공해야 한다 (Node 인터페이스, 입력-출력 처리형).

**[REQ-M1-02]** `influxdb-write` 노드는 **항상** `agent_ref` 설정으로 InfluxDB 에이전트를 참조해야 한다.

**[REQ-M1-03]** **WHEN** `influxdb-write` 노드에 메시지가 입력되면 **THEN** payload에서 measurement, tags, fields를 추출하여 InfluxDB에 기록해야 한다.

**[REQ-M1-04]** **IF** `measurement` 설정이 고정값으로 지정되어 있으면 **THEN** 해당 값을 사용해야 한다. **IF** 비어있으면 **THEN** `measurement_key` 설정으로 지정된 payload 키에서 measurement 이름을 추출해야 한다.

**[REQ-M1-05]** **WHEN** `tag_mappings` 설정이 존재하면 **THEN** 매핑에 따라 payload에서 태그 값을 추출해야 한다 (tag_name -> payload_key).

**[REQ-M1-06]** **WHEN** `field_mappings` 설정이 존재하면 **THEN** 매핑에 따라 payload에서 필드 값을 추출해야 한다. **IF** `field_mappings`가 비어있으면 **THEN** payload 전체를 fields로 사용해야 한다.

**[REQ-M1-07]** **가능하면** `timestamp_key` 설정을 통해 payload에서 타임스탬프(Unix ms)를 추출하는 기능을 제공한다. 미지정 시 현재 시간을 사용한다.

**[REQ-M1-12]** **IF** `bool_to_int` 설정이 true이면 **THEN** payload의 boolean 값(true/false)을 정수(1/0)로 변환하여 기록해야 한다.

**[REQ-M1-08]** **WHEN** 쓰기가 성공하면 **THEN** 원본 메시지를 그대로 다음 노드로 전달해야 한다 (pass-through).

**[REQ-M1-09]** **IF** measurement가 빈 문자열이면 **THEN** 에러를 반환해야 한다.

**[REQ-M1-10]** **IF** fields가 비어있으면 **THEN** 쓰기를 건너뛰고 원본 메시지를 그대로 반환해야 한다.

**[REQ-M1-11]** **WHEN** 에이전트 Process 호출이 실패하면 **THEN** 에러를 반환해야 한다.

### M2: influxdb-read 노드 (SourceNode)

**[REQ-M2-01]** 시스템은 **항상** `influxdb-read` 노드 타입을 제공해야 한다 (SourceNode 인터페이스, 폴링 기반).

**[REQ-M2-02]** `influxdb-read` 노드는 **항상** `agent_ref` 설정으로 InfluxDB 에이전트를 참조해야 한다.

**[REQ-M2-03]** **WHEN** `influxdb-read` 노드가 폴링하면 **THEN** 설정된 쿼리를 에이전트의 `Process()`에 전달하고, `ReceiveMessage()`로 결과를 수신해야 한다.

**[REQ-M2-04]** **WHEN** 쿼리 결과가 수신되면 **THEN** `output_mode` 설정에 따라 출력해야 한다:
- `rows` (기본): 각 행을 개별 메시지로 변환하여 `SourceCh`에 출력
- `batch`: 전체 결과를 `{results: [...], count: N}` 단일 메시지로 출력 (InfluxDB 메타데이터 자동 제거)
- `grouped`: `_field` 키로 그룹핑하여 `{field_name: [{time, value}, ...]}` 형태의 단일 메시지로 출력

**[REQ-M2-05]** `influxdb-read` 노드는 **항상** 다음 설정을 지원해야 한다:
- `query`: 실행할 쿼리 문자열 (필수)
- `language`: 쿼리 언어 ("flux", "sql", "influxql", 기본: "flux")
- `poll_interval`: 폴링 간격 (기본: "30s")
- `timeout`: 쿼리 타임아웃 (기본: "10s")
- `output_mode`: 출력 모드 ("rows"=행별 개별 메시지, "batch"=전체 결과 단일 메시지, "grouped"=필드별 시계열 배열, 기본: "rows")

**[REQ-M2-06]** **IF** 쿼리 결과가 빈 배열이면 **THEN** 메시지를 생성하지 않아야 한다.

**[REQ-M2-07]** **WHEN** 쿼리가 실패하면 **THEN** 에러를 로그에 기록하고 다음 폴링 주기까지 대기해야 한다.

**[REQ-M2-08]** **WHEN** 노드가 Shutdown되면 **THEN** 폴링 루프를 정상 종료해야 한다.

### M3: influxdb-query 노드

**[REQ-M3-01]** 시스템은 **항상** `influxdb-query` 노드 타입을 제공해야 한다 (Node 인터페이스, 입력 트리거형).

**[REQ-M3-02]** `influxdb-query` 노드는 **항상** `agent_ref` 설정으로 InfluxDB 에이전트를 참조해야 한다.

**[REQ-M3-03]** **WHEN** 메시지가 입력되면 **THEN** 설정된 쿼리 또는 payload에서 추출한 쿼리를 실행하고, 결과를 새 메시지의 payload에 담아 반환해야 한다.

**[REQ-M3-04]** **IF** `query` 설정에 `$variable` 패턴이 포함되어 있으면 **THEN** 입력 메시지의 payload에서 해당 변수 값을 치환해야 한다.

**[REQ-M3-05]** **IF** `query` 설정이 비어있으면 **THEN** 입력 메시지의 payload에서 `query` 키의 값을 쿼리로 사용해야 한다.

**[REQ-M3-06]** `influxdb-query` 노드는 **항상** 다음 설정을 지원해야 한다:
- `query`: 쿼리 문자열 (선택, $variable 치환 지원)
- `language`: 쿼리 언어 ("flux", "sql", "influxql", 기본: "flux")
- `timeout`: 쿼리 타임아웃 (기본: "10s")
- `result_key`: 결과를 저장할 payload 키 (기본: "results")

**[REQ-M3-07]** **WHEN** 쿼리 결과가 수신되면 **THEN** 결과를 새 메시지의 `result_key`에 저장하고, 입력 메시지의 메타데이터를 보존해야 한다.

**[REQ-M3-08]** **IF** 쿼리가 비어있으면 (설정에도 없고 payload에도 없으면) **THEN** 에러를 반환해야 한다.

**[REQ-M3-09]** **WHEN** 쿼리가 실패하면 **THEN** 에러를 반환해야 한다.

### M4: 노드 레지스트리 및 Web UI

**[REQ-M4-01]** 시스템은 **항상** 노드 레지스트리에 `"influxdb-write"`, `"influxdb-read"`, `"influxdb-query"` 노드 타입을 등록해야 한다.

**[REQ-M4-02]** 노드 카테고리는 **항상** `"storage"`여야 한다 (기존 tsdb-write, tsdb-query와 동일).

**[REQ-M4-03]** 시스템은 **항상** `nodeSchemas.ts`에 3개 노드의 UI 스키마를 등록해야 한다.

**[REQ-M4-04]** 시스템은 **항상** `nodeTypeMeta.ts`에 3개 노드의 메타데이터(아이콘, 색상)를 등록해야 한다.

**[REQ-M4-05]** 노드 스키마의 `agent_ref` 필드는 **항상** `agent_select` 타입이며, 옵션에 `influxdb`를 포함해야 한다.

---

## 4. Specifications (명세)

### 4.1 influxdbAgent 로컬 인터페이스

```go
// influxdbAgent 는 InfluxDB 에이전트의 최소 인터페이스이다.
type influxdbAgent interface {
    Process(data []byte) ([]byte, error)
}

// influxdbMessageReceiver 는 쿼리 결과 비동기 수신 인터페이스이다.
type influxdbMessageReceiver interface {
    influxdbAgent
    ReceiveMessage(ctx context.Context) ([]byte, error)
}
```

### 4.2 에이전트 해석 패턴

TSDB 노드의 `resolveTSDB` 패턴을 참조하되, `tsdbProvider` 대신 `influxdbAgent` 인터페이스로 타입 단언:

1. `config["_influxdb_agent"]`로 직접 주입 확인 (테스트용)
2. `agentRef` 유효성 확인
3. `resolver.ResolveAgent(ctx, agentRef)` -> transport
4. `transport.(AgentAccessor).UnderlyingAgent().(influxdbAgent)` 타입 단언

### 4.3 파일 구조 (신규/수정)

```
internal/node/
  influxdb_write.go       -- InfluxDBWriteNode (Node)
  influxdb_read.go        -- InfluxDBReadNode (SourceNode, 폴링)
  influxdb_query.go       -- InfluxDBQueryNode (Node)
  influxdb_write_test.go
  influxdb_read_test.go
  influxdb_query_test.go

internal/node/registry.go   -- (수정) 3개 노드 등록 추가

web/src/config/
  nodeSchemas.ts           -- (수정) influxdb 노드 스키마 추가
  nodeTypeMeta.ts          -- (수정) influxdb 노드 메타 추가
```

### 4.4 influxdb-write 데이터 흐름

```
Input Message
  payload: { measurement: "temperature", location: "room1", value: 22.5 }
    |
    v
InfluxDBWriteNode
  measurement="temperature" (고정) 또는 measurement_key="measurement" (payload에서 추출)
  tag_mappings: { location: "location" }
  field_mappings: { value: "value" }
    |
    v
agent.Process(json.Marshal(WriteData{
    Measurement: "temperature",
    Tags: {"location": "room1"},
    Fields: {"value": 22.5},
}))
    |
    v
Output: 원본 메시지 pass-through
```

### 4.5 influxdb-read 데이터 흐름

```
[Polling Timer]
    |
    v
agent.Process(json.Marshal(QueryRequest{
    Query: "SELECT * FROM temperature WHERE time > now() - 1h",
    Language: "influxql",
}))
    |
    v
agent.ReceiveMessage(ctx) -> []byte (JSON []map[string]any)
    |
    v
JSON Unmarshal -> []map[string]any
    |
    v
각 row -> 개별 Message (payload에 row 데이터) -> SourceCh
```

### 4.6 influxdb-query 변수 치환 규칙

- 패턴: `$variable_name` (영문자, 숫자, 밑줄)
- 치환 소스: 입력 메시지의 payload
- 문자열 값은 자동으로 작은따옴표로 감쌈: `$name` -> `'value'`
- 숫자 값은 그대로 치환: `$limit` -> `100`
- 치환 실패(payload에 키 없음) 시 원본 `$variable` 유지

---

## 5. Traceability (추적성)

| 요구사항 ID | 구현 파일 | 에이전트 메서드 |
|------------|----------|--------------|
| REQ-M1-01~11 | influxdb_write.go | Process (write) |
| REQ-M2-01~08 | influxdb_read.go | Process (query) + ReceiveMessage |
| REQ-M3-01~09 | influxdb_query.go | Process (query) + ReceiveMessage |
| REQ-M4-01~02 | registry.go | -- |
| REQ-M4-03~05 | nodeSchemas.ts, nodeTypeMeta.ts | -- |
