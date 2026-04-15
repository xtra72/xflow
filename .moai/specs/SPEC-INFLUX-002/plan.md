# SPEC-INFLUX-002: 구현 계획

> **SPEC ID**: SPEC-INFLUX-002
> **개발 방법론**: Hybrid (TDD for new code, DDD for modifications)
> **상태**: Planned

---

## 1. 마일스톤 개요

| 마일스톤 | 내용 | 우선순위 | 의존성 |
|---------|------|---------|-------|
| M1 | influxdb-write 노드 | Primary Goal | 없음 |
| M2 | influxdb-read 노드 (SourceNode) | Primary Goal | M1 (공통 코드 공유) |
| M3 | influxdb-query 노드 | Primary Goal | M1 (공통 코드 공유) |
| M4 | 노드 레지스트리 및 Web UI | Secondary Goal | M1, M2, M3 |

---

## 2. M1: influxdb-write 노드 (Primary Goal)

### 2.1 파일: `internal/node/influxdb_write.go`

**목표**: 메시지 payload에서 데이터를 추출하여 InfluxDB에 기록하는 처리 노드.

**기술 접근**:

1. **에이전트 접근 인터페이스** (공통, 3개 노드 파일 간 공유)
   - `influxdbAgent` 인터페이스: `Process(data []byte) ([]byte, error)`
   - `influxdbMessageReceiver` 인터페이스: `influxdbAgent` + `ReceiveMessage(ctx) ([]byte, error)`
   - 이 인터페이스들은 influxdb_write.go에 정의하고, 다른 파일에서 참조

2. **`InfluxDBWriteNode` 구조체**
   - `*BaseNode` 임베딩
   - `agent influxdbAgent` (해석된 에이전트 참조)
   - `resolver AgentResolver`
   - `agentRef *flow.AgentRef`
   - 설정 필드: `measurement`, `measurementKey`, `tagMappings`, `fieldMappings`, `timestampKey`

3. **에이전트 해석** (`resolveInfluxDB` 메서드)
   - TSDB 노드의 `resolveTSDB` 패턴 참조
   - `config["_influxdb_agent"]` 직접 주입 (테스트용)
   - `AgentResolver` -> `AgentAccessor` -> `influxdbAgent` 타입 단언

4. **Process 메서드**
   - measurement 결정 (고정값 또는 payload에서 추출)
   - tag_mappings에 따라 태그 추출
   - field_mappings에 따라 필드 추출 (비어있으면 전체 payload)
   - `WriteData` 구조체 생성 -> JSON 직렬화 -> `agent.Process(json)` 호출
   - 성공 시 원본 메시지 pass-through 반환

5. **Configure 메서드**
   - `measurement`: string
   - `measurement_key`: string
   - `tag_mappings`: map[string]any -> map[string]string
   - `field_mappings`: map[string]any -> map[string]string
   - `timestamp_key`: string (선택)

**TSDB Write와의 차이점 (주의)**:
- TSDB는 `db.Write(measurement, tags, fields)` 직접 호출
- InfluxDB는 WriteData JSON -> `agent.Process(json)` 간접 호출
- WriteData에 Timestamp 필드 포함 (TSDB에는 없음)

### 2.2 파일: `internal/node/influxdb_write_test.go`

- Table-driven 테스트
- 고정 measurement vs measurement_key 테스트
- tag_mappings/field_mappings 테스트
- 빈 measurement 에러 테스트
- 빈 fields 건너뛰기 테스트
- agent.Process 실패 시 에러 전파 테스트
- mock influxdbAgent 사용

---

## 3. M2: influxdb-read 노드 (Primary Goal)

### 3.1 파일: `internal/node/influxdb_read.go`

**목표**: 주기적으로 InfluxDB를 쿼리하여 결과를 메시지로 출력하는 SourceNode.

**기술 접근**:

1. **`InfluxDBReadNode` 구조체**
   - `*BaseNode` 임베딩
   - `agent influxdbMessageReceiver` (쿼리 + 결과 수신)
   - `resolver AgentResolver`
   - `agentRef *flow.AgentRef`
   - `sourceCh chan message.Message`
   - 설정 필드: `query`, `language`, `pollInterval`, `timeout`

2. **SourceNode 인터페이스 구현**
   - `SourceCh() <-chan message.Message` 반환
   - `Init(ctx)` 에서 에이전트 해석 + 폴링 고루틴 시작
   - `Shutdown(ctx)` 에서 폴링 고루틴 정지

3. **폴링 루프** (`pollLoop` 고루틴)
   - lgcnp-status 노드의 폴링 패턴 참조
   - `time.Ticker` 기반 주기적 쿼리 실행
   - 쿼리 요청: `QueryRequest{Query, Language}` -> JSON -> `agent.Process(json)`
   - 결과 수신: `agent.ReceiveMessage(ctx)` -> JSON -> `[]map[string]any`
   - 각 row를 개별 Message로 변환하여 `sourceCh`에 전송
   - 빈 결과는 무시
   - 에러 시 로그 기록 후 다음 주기 대기

4. **Configure 메서드**
   - `query`: string (필수)
   - `language`: string (기본: "flux")
   - `poll_interval`: string -> time.Duration (기본: "30s")
   - `timeout`: string -> time.Duration (기본: "10s")

**lgcnp-status와의 차이점**:
- lgcnp-status는 `get_recent`/`drain` 커맨드 사용
- influxdb-read는 `QueryRequest` JSON -> Process + ReceiveMessage 사용
- 결과 형식: lgcnp는 프레임 이벤트, influxdb는 `[]map[string]any` 행

### 3.2 파일: `internal/node/influxdb_read_test.go`

- 폴링 주기 테스트
- 쿼리 실행 및 결과 수신 테스트
- 빈 결과 무시 테스트
- 에러 복구 테스트
- Shutdown 시 정상 종료 테스트
- mock influxdbMessageReceiver 사용

---

## 4. M3: influxdb-query 노드 (Primary Goal)

### 4.1 파일: `internal/node/influxdb_query.go`

**목표**: 입력 메시지를 트리거로 InfluxDB 쿼리를 실행하고 결과를 반환하는 처리 노드.

**기술 접근**:

1. **`InfluxDBQueryNode` 구조체**
   - `*BaseNode` 임베딩
   - `agent influxdbMessageReceiver` (쿼리 + 결과 수신)
   - `resolver AgentResolver`
   - `agentRef *flow.AgentRef`
   - 설정 필드: `query`, `language`, `timeout`, `resultKey`

2. **Process 메서드**
   - 쿼리 결정:
     1. `query` 설정이 있으면 해당 문자열 사용 ($variable 치환 적용)
     2. `query` 설정이 없으면 payload의 `query` 키에서 추출
     3. 둘 다 없으면 에러
   - 변수 치환: `$variable` 패턴을 payload 값으로 대체
   - 쿼리 실행: `QueryRequest{Query, Language}` -> JSON -> `agent.Process(json)`
   - 결과 수신: `agent.ReceiveMessage(ctx)` -> JSON -> `[]map[string]any`
   - 결과 메시지 생성: payload에 `result_key` 키로 결과 저장

3. **변수 치환 함수** (`substituteVariables`)
   - 정규식: `\$([a-zA-Z_][a-zA-Z0-9_]*)` 패턴
   - 문자열 값: 작은따옴표 래핑 (`'value'`)
   - 숫자 값: 그대로 (`100`, `22.5`)
   - bool 값: 문자열 변환 (`true`, `false`)
   - 미발견 변수: 원본 유지 (`$unknown`)

4. **Configure 메서드**
   - `query`: string (선택)
   - `language`: string (기본: "flux")
   - `timeout`: string -> time.Duration (기본: "10s")
   - `result_key`: string (기본: "results")

**tsdb-query와의 차이점**:
- tsdb-query는 내부 Query 구조체 사용
- influxdb-query는 문자열 쿼리 + $variable 치환
- tsdb-query는 동기식 `db.Execute()`, influxdb-query는 비동기 Process + ReceiveMessage

### 4.2 파일: `internal/node/influxdb_query_test.go`

- 고정 쿼리 실행 테스트
- payload에서 쿼리 추출 테스트
- $variable 치환 테스트 (문자열, 숫자, bool)
- 미발견 변수 원본 유지 테스트
- 빈 쿼리 에러 테스트
- result_key 설정 테스트
- 에러 전파 테스트
- mock influxdbMessageReceiver 사용

---

## 5. M4: 노드 레지스트리 및 Web UI (Secondary Goal)

### 5.1 파일: `internal/node/registry.go` (수정)

노드 등록 테이블에 3개 항목 추가:

```go
{"influxdb-write", NewInfluxDBWriteNode, "storage", "메시지를 InfluxDB에 기록"},
{"influxdb-read", NewInfluxDBReadNode, "storage", "InfluxDB에서 주기적으로 데이터 조회"},
{"influxdb-query", NewInfluxDBQueryNode, "storage", "입력 메시지 기반 InfluxDB 쿼리 실행"},
```

레지스트리 주석과 빌트인 노드 수 업데이트 (36 -> 39).

### 5.2 파일: `web/src/config/nodeSchemas.ts` (수정)

**influxdb-write 스키마**:
- `agent_ref`: agent_select, options: ['influxdb'], 필수
- `measurement`: text, 선택 (고정 measurement 이름)
- `measurement_key`: text, 선택 (payload에서 추출할 키)
- `tag_mappings`: keyvalue, 선택 (태그 이름 -> payload 키)
- `field_mappings`: keyvalue, 선택 (필드 이름 -> payload 키)
- `timestamp_key`: text, 선택

**influxdb-read 스키마**:
- `agent_ref`: agent_select, options: ['influxdb'], 필수
- `query`: textarea, 필수 (Flux/SQL/InfluxQL 쿼리)
- `language`: select, options: ['flux', 'sql', 'influxql'], 기본: 'flux'
- `poll_interval`: text, 기본: '30s'
- `timeout`: text, 기본: '10s'

**influxdb-query 스키마**:
- `agent_ref`: agent_select, options: ['influxdb'], 필수
- `query`: textarea, 선택 ($variable 지원)
- `language`: select, options: ['flux', 'sql', 'influxql'], 기본: 'flux'
- `timeout`: text, 기본: '10s'
- `result_key`: text, 기본: 'results'

### 5.3 파일: `web/src/config/nodeTypeMeta.ts` (수정)

3개 노드의 메타데이터 추가:
- `influxdb-write`: storage 카테고리, 적절한 아이콘/색상
- `influxdb-read`: storage 카테고리
- `influxdb-query`: storage 카테고리

---

## 6. 리스크 및 대응

| 리스크 | 영향 | 대응 |
|-------|------|------|
| 쿼리 결과 비동기 수신 타이밍 | ReceiveMessage 타임아웃 | timeout 설정으로 대기 시간 제한 |
| recvCh 버퍼 오버플로 | 쿼리 결과 드롭 | 에이전트 BufferSize 설정 안내 |
| $variable 치환 시 SQL 인젝션 | 보안 위험 | 문자열 값 이스케이프 처리 (작은따옴표 치환) |
| v2/v3 쿼리 언어 호환성 | 잘못된 language 설정 | UI에서 에이전트 버전 기반 language 기본값 안내 |
| InfluxDB 서버 다운 | 쿼리/쓰기 실패 | 에러 로그 기록, 폴링 노드는 다음 주기에 재시도 |
| 대량 쿼리 결과 | 메모리 사용량 증가 | LIMIT 절 사용 권장, 문서에 명시 |

---

## 7. 의존성

### 내부 패키지 의존성

- `internal/node`: Node, SourceNode, BaseNode, NodeOption, AgentResolver, AgentAccessor
- `internal/agent/system`: InfluxDBAgent (타입 단언 대상, 직접 import는 하지 않음)
- `pkg/lifecycle`: BaseLifecycle, 상태 전이
- `pkg/flow`: NodeDef, AgentRef
- `pkg/message`: Message, Payload 인터페이스

### 외부 의존성

- 추가 외부 의존성 없음 (encoding/json, regexp, time 등 표준 라이브러리만 사용)

---

## 8. 구현 순서 요약

```
M1 (influxdb-write) -- 공통 인터페이스 정의 포함
 |
 +--> M2 (influxdb-read, SourceNode)
 |
 +--> M3 (influxdb-query, 변수 치환)
 |
 v
M4 (레지스트리 등록 + Web UI 스키마)
```

M1 완료 후 M2와 M3는 독립적으로 병렬 진행 가능.
각 마일스톤 완료 후 `go test -race ./internal/node/...` 실행으로 회귀 확인.
