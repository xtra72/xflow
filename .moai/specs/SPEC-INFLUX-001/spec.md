---
id: SPEC-INFLUX-001
version: "1.1.0"
status: completed
created: "2026-02-22"
updated: "2026-02-22"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-22 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-INFLUX-001: InfluxDB 연동 에이전트 구현

## 1. Environment (환경)

### 1.1 시스템 개요

xflow는 플로우 기반 데이터 처리 엔진으로, 에이전트(Agent)를 통해 외부 시스템과 연동한다. 현재 MQTT 에이전트가 구현되어 있으며, 동일한 패턴을 활용하여 InfluxDB 에이전트를 추가한다.

InfluxDB 에이전트는 브릿지 노드(Bridge Node)를 통해 플로우와 연동되며, 두 가지 핵심 기능을 제공한다:

1. **쓰기(Write)**: `BridgeOut` 방향으로 플로우에서 전달된 JSON 데이터를 InfluxDB에 저장
2. **쿼리(Query)**: `BridgeRequestReply` 방향으로 플로우에서 전달된 쿼리를 실행하고 결과를 반환

InfluxDB 2.x와 3.x를 모두 지원하되, 추상화 레이어를 통해 버전 차이를 내부적으로 처리한다.

### 1.2 기술 환경

- **언어**: Go 1.25+
- **패키지 경로**:
  - `internal/agent/system/influxdb_agent.go`: 메인 에이전트 구현
  - `internal/agent/system/influxdb_config.go`: 설정 파싱
  - `internal/agent/system/influxdb_client.go`: 클라이언트 추상화 인터페이스 + 팩토리
  - `internal/agent/system/influxdb_v2.go`: v2 어댑터
  - `internal/agent/system/influxdb_v3.go`: v3 어댑터
  - `internal/agent/system/influxdb_register.go`: 타입 등록
- **신규 의존 패키지**:
  - `github.com/influxdata/influxdb-client-go/v2`: InfluxDB 2.x 클라이언트
  - `github.com/InfluxCommunity/influxdb3-go/v2`: InfluxDB 3.x 클라이언트
- **기존 인터페이스**:
  - `Agent` 인터페이스 (`internal/agent/agent.go`): Init, Start, Stop, Pause, Resume, Health, Process, Configure, ID, Name, Type, Info, Stats
  - `MessageReceiver` 인터페이스: `ReceiveMessage(ctx context.Context) ([]byte, error)` - 비동기 메시지 수신
  - `AgentConfig`: ID, Name, Type, Transport (TransportConfig: Type string + Options map[string]any)
- **브릿지 노드** (`internal/node/bridge.go`):
  - `BridgeOut`: 플로우 -> 에이전트 (쓰기 용도)
  - `BridgeRequestReply`: 플로우 -> 에이전트 -> 플로우 (쿼리 용도)
- **에이전트 트랜스포트** (`internal/engine/resolver.go`):
  - `Send()`: `agent.Process(data)` 호출 (반환값 무시)
  - `Receive()`: `MessageReceiver` 인터페이스 확인 후 `ReceiveMessage()` 호출
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`

### 1.3 설계 원칙

- **추상화 레이어**: InfluxDB 2.x/3.x 차이를 내부 인터페이스로 캡슐화
- **기존 패턴 준수**: MQTT 에이전트(`mqtt_subscriber.go`)의 설정 파싱, 채널 기반 메시지 전달 패턴 활용
- **관심사 분리**: 에이전트 로직, 설정 파싱, 클라이언트 추상화를 별도 파일로 분리
- **테스트 가능성**: 인터페이스 기반 설계로 목(mock) 주입을 통한 단위 테스트 가능
- **관찰 가능성**: 연결 상태, 쓰기/쿼리 결과를 로그로 기록

### 1.4 범위 경계

- **범위 내(In-Scope)**:
  - InfluxDB 에이전트 구현 (Agent, MessageReceiver 인터페이스 준수)
  - 클라이언트 추상화 인터페이스 및 v2/v3 어댑터
  - 설정 파싱 (Transport.Options 기반)
  - 타입 등록 (`RegisterInfluxDBTypes`)
  - JSON 기반 쓰기/쿼리 프로토콜
  - 단위 테스트
  - 예제 설정 파일 (에이전트 YAML, 플로우 YAML)
- **범위 외(Out-of-Scope)**:
  - Bridge 노드 자체의 변경
  - InfluxDB 서버 설치/관리
  - 인증 관리 시스템 (토큰은 설정으로 직접 제공)
  - 통합 테스트 (실제 InfluxDB 서버 연동)
  - TLS/mTLS 설정

---

## 2. Assumptions (가정)

| ID | 가정 | 근거 | 위험도 |
|----|------|------|--------|
| A-1 | InfluxDB 2.x 클라이언트 `influxdb-client-go/v2`가 안정적이다 | InfluxData 공식 유지보수 라이브러리, GitHub Stars 500+, 활발한 릴리스 | 낮음 |
| A-2 | InfluxDB 3.x 클라이언트 `influxdb3-go/v2`가 프로덕션 사용 가능하다 | InfluxCommunity 관리, v2 메이저 릴리스, Apache Arrow Flight 기반 | 중간 |
| A-3 | 쓰기 데이터는 JSON 형식으로 플로우에서 전달된다 | 기존 브릿지 노드의 데이터 전달 방식이 `[]byte` 기반 | 낮음 |
| A-4 | 쿼리 결과를 JSON 배열로 직렬화할 수 있다 | InfluxDB 쿼리 결과는 테이블/레코드 형식이며 JSON 변환 가능 | 낮음 |
| A-5 | InfluxDB 2.x의 Flux 쿼리와 v1 호환 InfluxQL API를 모두 활용 가능하다 | influxdb-client-go v1 호환 API 제공 확인 | 낮음 |
| A-6 | InfluxDB 3.x는 SQL과 InfluxQL을 네이티브로 지원한다 | InfluxDB 3.x Core/Enterprise 문서에서 SQL 및 InfluxQL 지원 명시 | 낮음 |
| A-7 | `BridgeRequestReply` 방향이 쿼리 요청-응답 패턴에 적합하다 | Bridge 노드의 Send() -> ReceiveMessage() 호출 흐름 확인 | 낮음 |
| A-8 | 에이전트 재시작 시 InfluxDB 재연결이 필요할 수 있다 | 네트워크 장애 또는 서버 재시작 시 연결 끊김 가능 | 중간 |

---

## 3. Requirements (요구사항)

### REQ-1: InfluxDB 에이전트 타입 등록 [HARD]

시스템은 **항상** `"influxdb"` 타입의 에이전트를 에이전트 매니저에 등록하여 플로우 설정에서 사용 가능하도록 해야 한다.

- `RegisterInfluxDBTypes(mgr)` 함수를 통해 팩토리 등록
- `mgr.RegisterType("influxdb", factory)` 호출
- 기존 `RegisterMQTTTypes` 패턴 준수

### REQ-2: 설정 파싱 [HARD]

**WHEN** 에이전트 설정이 제공되면 **THEN** 시스템은 `Transport.Options` 맵에서 InfluxDB 연결에 필요한 모든 설정값을 파싱해야 한다.

- 필수 설정: `url`, `token`, `bucket`, `version` ("2" 또는 "3")
- 선택 설정: `org` (v2 필수, v3 선택), `query_language` (기본값: v2는 "flux", v3는 "sql"), `timeout_sec` (기본값: 10), `buffer_size` (기본값: 256), `batch_size` (기본값: 1000), `flush_interval_ms` (기본값: 1000), `precision` (기본값: "ns")
- 필수 설정 누락 시 명확한 에러 메시지 반환

### REQ-3: 클라이언트 추상화 인터페이스 [HARD]

시스템은 **항상** InfluxDB 버전 차이를 추상화하는 내부 인터페이스를 제공해야 한다.

- `InfluxClient` 인터페이스 정의: `Write(ctx, data WriteData) error`, `Query(ctx, query string, lang string) ([]map[string]any, error)`, `Health(ctx) error`, `Close() error`
- 팩토리 함수가 `version` 설정에 따라 v2 또는 v3 어댑터 생성

### REQ-4: InfluxDB 2.x 어댑터 [HARD]

**WHEN** 설정의 `version`이 "2"인 경우 **THEN** 시스템은 `influxdb-client-go/v2` 패키지를 사용하여 InfluxDB 2.x에 연결해야 한다.

- Write: `WriteAPIBlocking` 또는 `WriteAPI`(비동기)를 사용하여 포인트 쓰기
- Query: `query_language`에 따라 분기
  - `"flux"`: Flux API로 쿼리 실행
  - `"influxql"`: v1 호환 쿼리 API 활용
  - `"sql"`: v2에서는 미지원, 에러 반환
- Health: `Health()` API로 서버 상태 확인

### REQ-5: InfluxDB 3.x 어댑터 [HARD]

**WHEN** 설정의 `version`이 "3"인 경우 **THEN** 시스템은 `influxdb3-go/v2` 패키지를 사용하여 InfluxDB 3.x에 연결해야 한다.

- Write: Line Protocol 기반 포인트 쓰기
- Query: `query_language`에 따라 분기
  - `"sql"`: SQL 쿼리 실행 (Apache Arrow Flight)
  - `"influxql"`: InfluxQL 쿼리 실행
  - `"flux"`: v3에서는 미지원, 에러 반환
- Health: 연결 상태 확인

### REQ-6: 쓰기 연산 (BridgeOut) [HARD]

**WHEN** 에이전트의 `Process(data []byte)` 메서드가 호출되고 데이터가 쓰기 JSON 형식인 경우 **THEN** 시스템은 JSON을 파싱하여 InfluxDB에 포인트로 저장해야 한다.

- 입력 JSON 형식 (단건):
  ```json
  {
    "measurement": "temperature",
    "tags": {"host": "server01", "region": "us-west"},
    "fields": {"value": 25.5, "status": "ok"},
    "timestamp": 1234567890000000000
  }
  ```
- 입력 JSON 형식 (배치):
  ```json
  [
    {"measurement": "temperature", "tags": {...}, "fields": {...}, "timestamp": ...},
    {"measurement": "temperature", "tags": {...}, "fields": {...}, "timestamp": ...}
  ]
  ```
- `timestamp`가 없으면 현재 시간 사용
- `measurement`, `fields`는 필수, `tags`는 선택
- 쓰기 실패 시 에러를 로그에 기록

### REQ-7: 쿼리 연산 (BridgeRequestReply) [HARD]

**WHEN** 에이전트의 `Process(data []byte)` 메서드가 호출되고 데이터가 쿼리 JSON 형식인 경우 **THEN** 시스템은 쿼리를 실행하고 결과를 `recvCh` 채널에 전달해야 한다.

- 입력 JSON 형식:
  ```json
  {
    "query": "SELECT mean(value) FROM temperature WHERE time > now() - 1h GROUP BY host",
    "language": "influxql"
  }
  ```
- `language` 필드로 쿼리 언어 지정 (`"influxql"`, `"sql"`, `"flux"`)
- `language` 미지정 시 설정의 `query_language` 기본값 사용
- 결과는 JSON 배열로 직렬화하여 `recvCh`에 전달:
  ```json
  [
    {"time": "2026-02-22T10:00:00Z", "host": "server01", "mean_value": 25.3},
    {"time": "2026-02-22T10:00:00Z", "host": "server02", "mean_value": 23.1}
  ]
  ```
- `ReceiveMessage()` 호출 시 `recvCh`에서 결과를 읽어 반환

### REQ-8: 요청 타입 자동 판별 [HARD]

**WHEN** `Process(data []byte)`가 호출되면 **THEN** 시스템은 입력 JSON의 구조를 분석하여 쓰기 요청과 쿼리 요청을 자동으로 구분해야 한다.

- `"query"` 필드가 존재하면 쿼리 요청으로 처리
- `"measurement"` 필드가 존재하면 쓰기 요청으로 처리
- JSON 배열이면 배치 쓰기 요청으로 처리
- 판별 불가 시 에러 반환

### REQ-9: Agent 인터페이스 준수 [HARD]

시스템은 **항상** `Agent` 및 `MessageReceiver` 인터페이스를 완전히 구현해야 한다.

- `Init()`: InfluxDB 클라이언트 생성 및 연결 확인
- `Start()`: 쓰기 배치 플러셔 시작 (해당 시)
- `Stop()`: 클라이언트 연결 종료, 미처리 데이터 플러시
- `Pause()`/`Resume()`: 쓰기/쿼리 일시 중지 및 재개
- `Health()`: InfluxDB 서버 연결 상태 확인
- `Process(data)`: 쓰기 또는 쿼리 처리 (REQ-6, REQ-7, REQ-8)
- `Configure(config)`: 런타임 설정 변경
- `ReceiveMessage(ctx)`: 쿼리 결과 채널에서 읽기

### REQ-10: 지원되지 않는 쿼리 언어 에러 처리 [HARD]

**IF** 사용자가 현재 InfluxDB 버전에서 지원되지 않는 쿼리 언어를 요청하면 **THEN** 시스템은 명확한 에러 메시지를 반환하고, 지원되는 언어 목록을 안내해야 한다.

- v2에서 `"sql"` 요청 시: "InfluxDB 2.x에서는 SQL을 지원하지 않습니다. 'flux' 또는 'influxql'을 사용하세요."
- v3에서 `"flux"` 요청 시: "InfluxDB 3.x에서는 Flux를 지원하지 않습니다. 'sql' 또는 'influxql'을 사용하세요."

### REQ-11: 에이전트 통계 정보 제공 [SOFT]

**WHEN** `Stats()` 메서드가 호출되면 **THEN** 시스템은 에이전트의 운영 통계를 반환해야 한다.

- 총 쓰기 요청 수
- 총 쿼리 요청 수
- 쓰기 실패 수
- 쿼리 실패 수
- 마지막 쓰기/쿼리 시간
- 현재 연결 상태

### REQ-12: 쿼리 언어 변환 [SOFT]

**가능하면** 에이전트는 사용자가 제공한 쿼리를 InfluxDB API에 맞도록 적절히 변환하여 요청하는 기능을 제공한다.

- v2 InfluxQL 쿼리 시 v1 호환 API 경로 활용
- v3 SQL 쿼리 시 Apache Arrow Flight SQL 프로토콜 활용
- 쿼리 파라미터 이스케이핑 및 안전한 전달

---

## 4. Specifications (명세)

### 4.1 에이전트 구조

```
InfluxDBAgent
├── config: InfluxDBConfig          // 파싱된 설정
├── client: InfluxClient            // 추상화된 클라이언트 인터페이스
├── recvCh: chan []byte             // 쿼리 결과 전달 채널
├── stats: InfluxDBStats            // 운영 통계
├── mu: sync.RWMutex                // 동시성 보호
└── paused: bool                    // 일시 중지 상태
```

### 4.2 클라이언트 추상화

```
InfluxClient (인터페이스)
├── Write(ctx context.Context, data []WriteData) error
├── Query(ctx context.Context, query string, lang string) ([]map[string]any, error)
├── Health(ctx context.Context) error
└── Close() error

InfluxV2Client (구현체)
├── client: influxdb2.Client
├── writeAPI: api.WriteAPIBlocking
├── queryAPI: api.QueryAPI
├── org: string
└── bucket: string

InfluxV3Client (구현체)
├── client: *influxdb3.Client
├── database: string
└── precision: lineprotocol.Precision
```

### 4.3 데이터 모델

```go
// 쓰기 요청 데이터
type WriteData struct {
    Measurement string            `json:"measurement"`
    Tags        map[string]string `json:"tags,omitempty"`
    Fields      map[string]any    `json:"fields"`
    Timestamp   *int64            `json:"timestamp,omitempty"`
}

// 쿼리 요청 데이터
type QueryRequest struct {
    Query    string `json:"query"`
    Language string `json:"language,omitempty"`
}
```

### 4.4 설정 구조

```go
type InfluxDBConfig struct {
    URL            string  // InfluxDB 서버 URL
    Token          string  // 인증 토큰
    Org            string  // 조직 (v2 필수)
    Bucket         string  // 버킷/데이터베이스 이름
    Version        string  // "2" 또는 "3"
    QueryLanguage  string  // "influxql", "sql", "flux"
    TimeoutSec     int     // 연결 타임아웃 (초)
    BufferSize     int     // 결과 버퍼 크기
    BatchSize      int     // 쓰기 배치 크기
    FlushIntervalMs int    // 쓰기 플러시 간격 (밀리초)
    Precision      string  // 쓰기 정밀도 ("ns", "us", "ms", "s")
}
```

### 4.5 Process() 처리 흐름

1. `Process(data []byte)` 호출
2. JSON 파싱 시도
3. 요청 타입 판별 (REQ-8):
   - `"query"` 키 존재 -> 쿼리 처리 (REQ-7)
   - `"measurement"` 키 존재 -> 단건 쓰기 처리 (REQ-6)
   - JSON 배열 -> 배치 쓰기 처리 (REQ-6)
   - 기타 -> 에러 반환
4. 쓰기 경로: `client.Write(ctx, data)` 호출
5. 쿼리 경로: `client.Query(ctx, query, lang)` 호출 -> 결과 JSON 직렬화 -> `recvCh`에 전달
6. 통계 업데이트 (REQ-11)

### 4.6 Import 추가

```go
// influxdb_client.go
import (
    // InfluxClient 인터페이스 정의만 - 별도 import 불필요
)

// influxdb_v2.go
import (
    influxdb2 "github.com/influxdata/influxdb-client-go/v2"
    "github.com/influxdata/influxdb-client-go/v2/api"
)

// influxdb_v3.go
import (
    "github.com/InfluxCommunity/influxdb3-go/v2/influxdb3"
)
```

---

## 5. Traceability (추적성)

| 요구사항 | 구현 위치 | 테스트 |
|----------|----------|--------|
| REQ-1 | `influxdb_register.go` - `RegisterInfluxDBTypes()` | `TestRegisterInfluxDBTypes` |
| REQ-2 | `influxdb_config.go` - `parseInfluxDBConfig()` | `TestParseInfluxDBConfig_*` |
| REQ-3 | `influxdb_client.go` - `InfluxClient` 인터페이스 + `NewInfluxClient()` 팩토리 | `TestNewInfluxClient_*` |
| REQ-4 | `influxdb_v2.go` - `InfluxV2Client` 구현 | `TestInfluxV2Client_*` |
| REQ-5 | `influxdb_v3.go` - `InfluxV3Client` 구현 | `TestInfluxV3Client_*` |
| REQ-6 | `influxdb_agent.go` - `Process()` 쓰기 경로 | `TestInfluxDBAgent_ProcessWrite_*` |
| REQ-7 | `influxdb_agent.go` - `Process()` 쿼리 경로 + `ReceiveMessage()` | `TestInfluxDBAgent_ProcessQuery_*` |
| REQ-8 | `influxdb_agent.go` - `Process()` 요청 판별 | `TestInfluxDBAgent_RequestTypeDetection_*` |
| REQ-9 | `influxdb_agent.go` - 전체 Agent/MessageReceiver 구현 | `TestInfluxDBAgent_Lifecycle_*` |
| REQ-10 | `influxdb_v2.go`, `influxdb_v3.go` - 언어 검증 | `TestUnsupportedQueryLanguage_*` |
| REQ-11 | `influxdb_agent.go` - `Stats()` | `TestInfluxDBAgent_Stats` |
| REQ-12 | `influxdb_v2.go`, `influxdb_v3.go` - 쿼리 변환 | `TestQueryLanguageTransform_*` |

---

## 6. 관련 SPEC

| SPEC ID | 제목 | 관계 |
|---------|------|------|
| SPEC-AGENT-001 | Agent 인터페이스 설계 | 상위 컨텍스트 (인터페이스 기반) |
| SPEC-BRIDGE-001 | Bridge 노드 구현 | 상위 컨텍스트 (브릿지 연동) |
| SPEC-SYSAGENT-001 | 시스템 에이전트 매니저 | 상위 컨텍스트 (타입 등록) |
| SPEC-MQTT-001 | MQTT Bridge Topic Subscription | 참고 (유사 구현 패턴) |

---

## 7. Implementation Notes (구현 노트)

### 구현 완료일: 2026-02-22

### 구현 파일 목록

| 파일 | 역할 | 라인 수 |
|------|------|---------|
| `internal/agent/system/influxdb_client.go` | InfluxClient 인터페이스, WriteData/QueryRequest 구조체, NewInfluxClient 팩토리 | ~60 |
| `internal/agent/system/influxdb_config.go` | InfluxDBConfig 구조체, parseInfluxDBConfig 함수 | ~120 |
| `internal/agent/system/influxdb_v2.go` | influxdb-client-go/v2 기반 v2 어댑터 | ~180 |
| `internal/agent/system/influxdb_v3.go` | influxdb3-go/v2 기반 v3 어댑터 | ~170 |
| `internal/agent/system/influxdb_agent.go` | 메인 에이전트 (Agent + MessageReceiver 구현) | ~420 |
| `internal/agent/system/influxdb_register.go` | RegisterInfluxDBTypes 팩토리 등록 | ~15 |
| `internal/agent/system/influxdb_config_test.go` | 설정 파싱 테스트 (12 케이스) | ~250 |
| `internal/agent/system/influxdb_agent_test.go` | 에이전트 테스트 (23 케이스, 목 클라이언트 포함) | ~450 |
| `examples/agents/influxdb-writer.yaml` | 에이전트 설정 예제 | ~54 |
| `examples/flows/influxdb-metrics.yaml` | MQTT→InfluxDB 플로우 예제 | ~135 |
| `cmd/xflowd/main.go` | RegisterInfluxDBTypes 호출 추가 (1줄) | +1 |

### 의존성 추가

- `github.com/influxdata/influxdb-client-go/v2` v2.14.0 (InfluxDB 2.x)
- `github.com/InfluxCommunity/influxdb3-go/v2` v2.13.0 (InfluxDB 3.x)

### 구현 결정 사항

1. **테스트 파일 통합**: plan.md에서 `influxdb_client_test.go`를 별도로 계획했으나, 목 클라이언트를 `influxdb_agent_test.go`에 통합하여 테스트 복잡도를 줄임
2. **v2 쓰기**: `WriteAPIBlocking` 사용 (동기 쓰기, 안정성 우선)
3. **v2 InfluxQL**: Flux API를 통한 InfluxQL 실행 (`queryFlux` 공유)
4. **v3 쿼리 옵션**: `influxdb3.WithQueryType(influxdb3.InfluxQL)` 사용
5. **v3 Health**: v3 클라이언트에 health endpoint가 없어 nil 체크만 수행
6. **에러 메시지**: 모든 에러 메시지와 주석을 한국어(code_comments: "ko")로 작성

### 테스트 결과

- 전체 테스트: 35개 PASS (race detector 포함)
- `go vet`: 경고 없음
- `go build`: 성공
