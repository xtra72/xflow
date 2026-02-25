---
id: SPEC-AGENT-002
version: "1.1.0"
status: draft
created: "2026-02-19"
updated: "2026-02-25"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-19 | 1.0.0 | 초기 SPEC 작성 |
| 2026-02-25 | 1.1.0 | 에이전트 상세 조회 모듈 추가 (Module 6) |

---

# SPEC-AGENT-002: Agent Import/Export - 설정파일 가져오기/내보내기

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼에서 Agent 설정을 JSON/YAML 파일로 내보내기(Export)하거나 파일에서 가져오기(Import)하는 기능을 제공한다. 기존 Flow Import/Export 패턴(`internal/cli/flow.go`)을 참조하여 일관된 CLI/API 인터페이스를 구현한다.

본 SPEC은 다음 기능을 정의한다:

- **Agent Export (CLI)**: 단일 에이전트 설정을 파일로 내보내기
- **Agent Import (CLI)**: 파일에서 에이전트를 생성(가져오기)
- **Batch Operations**: 디렉토리 단위 일괄 가져오기/내보내기
- **API Export Endpoint**: REST API 레벨 Export 엔드포인트
- **Agent Config Examples**: 예제 설정 파일 제공

### 1.2 기술 환경

- **언어**: Go 1.23+
- **CLI 프레임워크**: `github.com/spf13/cobra` + `github.com/spf13/viper`
- **패키지 경로**:
  - CLI: `internal/cli/agent.go` (기존 파일 확장)
  - Handler: `internal/api/handler/agent.go` (기존 파일 확장)
  - Examples: `examples/agents/` (신규 디렉토리)
- **의존 패키지**:
  - `encoding/json` (JSON 직렬화/역직렬화)
  - `gopkg.in/yaml.v3` (YAML 직렬화/역직렬화)
  - `os` (파일 I/O)
  - `path/filepath` (경로 처리)
- **참조 구현**: `internal/cli/flow.go` - `newFlowExportCmd()`, `newFlowImportCmd()`
- **관련 SPEC**: SPEC-AGENT-001 (Agent System 프레임워크)

### 1.3 설계 원칙

- **Flow 패턴 일관성**: `flow export/import` 와 동일한 UX 패턴 유지
- **기존 헬퍼 재사용**: `readFile()`, `detectFileFormat()` 등 `root.go` 공유 함수 활용
- **파일 형식 자동 감지**: 확장자 기반 JSON/YAML 자동 판별
- **멱등성**: Import 시 동일 설정 재실행 가능
- **역호환성**: 기존 `agent create -f` 동작에 영향 없음

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- Agent Export CLI 커맨드 (`agent export <id> -o <file>`)
- Agent Import CLI 커맨드 (`agent import -f <file>`)
- Batch Import/Export (디렉토리 단위)
- API Export 엔드포인트 (`GET /api/v1/agents/{id}/export`, `GET /api/v1/agents/export`)
- Agent 설정 예제 파일 (`examples/agents/`)

**OUT OF SCOPE (본 SPEC 범위 외)**:
- Agent Manager 내부 로직 변경 (SPEC-AGENT-001 관할)
- Agent 런타임 상태(Status, Stats) Export (설정만 Export)
- Agent 바이너리/플러그인 Export
- 원격 서버 간 Agent 마이그레이션
- Agent 버전 관리 및 히스토리 추적

---

## 2. Assumptions (가정)

### 2.1 선행 조건

- SPEC-AGENT-001의 Agent 시스템 프레임워크가 구현되어 있다
- `internal/cli/agent.go`에 기존 7개 서브커맨드(list, get, create, start, stop, restart, delete)가 존재한다
- `internal/cli/root.go`에 `readFile()`, `detectFileFormat()` 헬퍼가 존재한다
- `internal/api/handler/agent.go`의 `AgentHandler`와 `AgentManager` 인터페이스가 사용 가능하다
- `internal/api/dto/request.go`의 `AgentCreateRequest` DTO가 사용 가능하다

### 2.2 기술 가정

- Export 대상은 Agent 설정 정보(Name, Type, Config)이며, 런타임 상태(Status, Stats)는 제외한다
- Import 시 새로운 Agent가 생성되며, 기존 Agent를 덮어쓰지 않는다
- 파일 형식(JSON/YAML)은 파일 확장자로 자동 감지한다
- Batch Export 시 파일명은 `{agent-name}.{format}` 또는 `{agent-id}.{format}` 형태이다
- API Export는 JSON 형식으로 응답하며, `Accept` 헤더로 YAML 지원은 선택 사항이다

---

## 3. Requirements (요구사항)

### Module 1: Agent Export (CLI) - P0

#### REQ-AGENT-002-01: 단일 에이전트 Export

**WHEN** 사용자가 `xflow agent export <id> -o <file>` 명령을 실행하면
**THEN** 시스템은 지정된 에이전트의 설정 정보를 조회하여 해당 파일에 저장해야 한다

#### REQ-AGENT-002-02: Export 파일 형식 자동 감지

**WHEN** 사용자가 Export 출력 파일 경로를 `.json` 확장자로 지정하면
**THEN** 시스템은 JSON 형식으로 직렬화해야 한다

**WHEN** 사용자가 Export 출력 파일 경로를 `.yaml` 또는 `.yml` 확장자로 지정하면
**THEN** 시스템은 YAML 형식으로 직렬화해야 한다

#### REQ-AGENT-002-03: Export 성공 메시지

**WHEN** Export가 성공적으로 완료되면
**THEN** 시스템은 `에이전트 '{id}' 를 {path} 로 내보냈습니다.` 형식의 메시지를 출력해야 한다

#### REQ-AGENT-002-04: Export 시 런타임 필드 제외

시스템은 **항상** Export 데이터에서 런타임 전용 필드(id, status, connected, uptime, messages_in, messages_out, error_count)를 제외해야 한다

#### REQ-AGENT-002-05: Export 출력 플래그 필수

**IF** 사용자가 `-o` (output) 플래그 없이 `agent export` 명령을 실행하면
**THEN** 시스템은 `출력 파일 경로(-o)를 지정해야 합니다` 에러를 반환해야 한다

### Module 2: Agent Import (CLI) - P0

#### REQ-AGENT-002-06: 파일에서 에이전트 Import

**WHEN** 사용자가 `xflow agent import -f <file>` 명령을 실행하면
**THEN** 시스템은 파일을 읽고 파싱하여 `POST /api/v1/agents` 로 에이전트를 생성해야 한다

#### REQ-AGENT-002-07: Import 파일 형식 자동 감지

**WHEN** Import 파일 확장자가 `.json`이면
**THEN** 시스템은 JSON으로 파싱해야 한다

**WHEN** Import 파일 확장자가 `.yaml` 또는 `.yml`이면
**THEN** 시스템은 YAML로 파싱해야 한다

**WHEN** Import 파일 확장자를 인식할 수 없으면
**THEN** 시스템은 JSON으로 파싱을 시도해야 한다

#### REQ-AGENT-002-08: Import 시 AgentCreateRequest 형식 래핑

**WHEN** Import 파일에 `name`과 `type` 필드가 존재하면
**THEN** 시스템은 `AgentCreateRequest` 형식(`name`, `type`, `config`)으로 래핑하여 API에 전송해야 한다

#### REQ-AGENT-002-09: Import 파일 경로 필수

**IF** 사용자가 `-f` (file) 플래그 없이 `agent import` 명령을 실행하면
**THEN** 시스템은 `가져올 에이전트 파일 경로(-f)를 지정해야 합니다` 에러를 반환해야 한다

#### REQ-AGENT-002-10: Import 결과 출력

**WHEN** Import가 성공적으로 완료되면
**THEN** 시스템은 생성된 에이전트 정보를 현재 `--format` 설정에 따라 출력해야 한다

### Module 3: Batch Operations - P1

#### REQ-AGENT-002-11: 디렉토리 일괄 Import

**WHEN** 사용자가 `xflow agent import -f <directory>` 명령을 실행하면
**THEN** 시스템은 해당 디렉토리 내의 모든 `.json`, `.yaml`, `.yml` 파일을 순회하며 각각 에이전트를 생성해야 한다

#### REQ-AGENT-002-12: Batch Import 결과 요약

**WHEN** 디렉토리 일괄 Import가 완료되면
**THEN** 시스템은 성공/실패 건수를 요약하여 출력해야 한다
(예: `3개 에이전트 가져오기 완료 (성공: 2, 실패: 1)`)

#### REQ-AGENT-002-13: Batch Import 부분 실패 처리

**IF** 일괄 Import 중 일부 파일에서 에러가 발생하면
**THEN** 시스템은 해당 파일의 에러를 출력하고 나머지 파일의 처리를 계속해야 한다

#### REQ-AGENT-002-14: 전체 에이전트 Export

**WHEN** 사용자가 `xflow agent export --all -o <directory>` 명령을 실행하면
**THEN** 시스템은 모든 에이전트를 조회하여 지정된 디렉토리에 개별 파일로 저장해야 한다

#### REQ-AGENT-002-15: Batch Export 파일 명명 규칙

**WHEN** 전체 에이전트를 디렉토리로 Export 하면
**THEN** 각 파일의 이름은 `{agent-name}.{format}` 형식이어야 한다
(agent-name에 허용되지 않는 문자가 있으면 `{agent-id}.{format}` 사용)

#### REQ-AGENT-002-16: Batch Export 형식 지정

**WHEN** `--all` 플래그와 함께 `-o <directory>` 를 지정하면
**THEN** 시스템은 `--export-format` 플래그(기본값: `yaml`)에 따라 출력 형식을 결정해야 한다

### Module 4: API Export Endpoint - P1

#### REQ-AGENT-002-17: 단일 에이전트 API Export

**WHEN** `GET /api/v1/agents/{id}/export` 요청이 수신되면
**THEN** 시스템은 해당 에이전트의 설정 정보를 런타임 필드 제외한 JSON으로 응답해야 한다

#### REQ-AGENT-002-18: 전체 에이전트 API Export

**WHEN** `GET /api/v1/agents/export` 요청이 수신되면
**THEN** 시스템은 모든 에이전트의 설정 정보를 런타임 필드 제외한 JSON 배열로 응답해야 한다

#### REQ-AGENT-002-19: Export API 응답 형식

시스템은 **항상** Export API 응답을 `dto.NewSuccessResponse()` 래핑 없이 원본 설정 데이터로 반환해야 한다
(파일 저장 및 Import 호환성을 위해 원본 데이터 직접 반환)

#### REQ-AGENT-002-20: 존재하지 않는 에이전트 Export 요청

**IF** 존재하지 않는 에이전트 ID로 Export 요청이 수신되면
**THEN** 시스템은 `404 Not Found` 에러를 반환해야 한다

### Module 5: Agent Config Examples - P0

#### REQ-AGENT-002-21: 예제 파일 제공

시스템은 **항상** `examples/agents/` 디렉토리에 다음 예제 파일을 포함해야 한다:
- `serial-modbus.yaml`: Serial Transport + Modbus 프로토콜 에이전트 예제
- `tcp-custom.json`: TCP Transport + Custom 프로토콜 에이전트 예제
- `mqtt-sensor.yaml`: MQTT 기반 센서 에이전트 예제

#### REQ-AGENT-002-22: 예제 파일 유효성

시스템은 **항상** 예제 파일이 `AgentCreateRequest` DTO의 필수 필드(`name`, `type`)를 포함하도록 해야 한다

#### REQ-AGENT-002-23: 예제 파일 Import 호환성

**WHEN** 사용자가 예제 파일을 `xflow agent import -f examples/agents/<file>` 명령으로 Import하면
**THEN** 시스템은 에러 없이 에이전트를 생성해야 한다 (서버 연결 시)

### Module 6: Agent Detail View Enhancement - P0

#### REQ-AGENT-002-24: 에이전트 상세 응답 보강

**WHEN** `GET /api/v1/agents/{id}` 요청이 수신되면
**THEN** 시스템은 기존 5개 필드(id, name, type, status, config)에 더하여 health, stats, uptime, started_at, created_at, connected, shared_info 필드를 포함하여 응답해야 한다

#### REQ-AGENT-002-25: 상세 수준 쿼리 파라미터

**WHEN** `GET /api/v1/agents/{id}?detail=summary` 요청이 수신되면 (기본값)
**THEN** 시스템은 보강된 기본 정보(health, stats, uptime, started_at, created_at, connected)를 포함하여 응답해야 한다

**WHEN** `GET /api/v1/agents/{id}?detail=full` 요청이 수신되면
**THEN** 시스템은 summary 수준의 모든 정보에 추가로 shared_info와 타입별 커스텀 상태(state)를 포함하여 응답해야 한다

#### REQ-AGENT-002-26: StatefulAgent 인터페이스

**IF** 에이전트가 `StatefulAgent` 인터페이스를 구현하면
**THEN** 시스템은 해당 에이전트의 커스텀 런타임 상태를 `state` 필드로 노출할 수 있어야 한다
(예: Samsung NASA 에이전트의 디바이스 목록 및 디바이스 상태)

**IF** 에이전트가 `StatefulAgent` 인터페이스를 구현하지 않으면
**THEN** 시스템은 응답에 `state` 필드를 포함하지 않아야 한다

#### REQ-AGENT-002-27: CLI 기본 상세 조회

**WHEN** 사용자가 `xflow agent get <id>` 명령을 실행하면
**THEN** 시스템은 보강된 요약 정보(status, health, uptime, connected, 메시지 수신/발신 건수, 에러 건수)를 구조화된 형식으로 출력해야 한다

#### REQ-AGENT-002-28: CLI --detail 플래그

**WHEN** 사용자가 `xflow agent get <id> --detail` 명령을 실행하면
**THEN** 시스템은 요약 정보에 추가로 shared_info와 타입별 커스텀 상태 데이터를 포함하여 출력해야 한다

#### REQ-AGENT-002-29: CLI 구조화된 출력 형식

시스템은 **항상** `agent get` 명령의 상세 조회 결과를 키-값 쌍 형식의 구조화된 텍스트로 출력해야 한다
(원시 JSON 덤프가 아닌, 사람이 읽기 쉬운 레이블-값 형식)

#### REQ-AGENT-002-30: 타입별 상태 데이터 조건부 포함

**IF** 에이전트가 `StatefulAgent` 인터페이스를 구현하고 `detail=full`이 요청되면
**THEN** 시스템은 응답의 `state` 필드에 해당 에이전트의 타입별 런타임 상태 데이터를 포함해야 한다

**IF** `detail=summary`이거나 에이전트가 `StatefulAgent`를 구현하지 않으면
**THEN** 시스템은 응답에 `state` 필드를 포함하지 않아야 한다

---

## 4. Specifications (명세)

### 4.1 Module 1: Agent Export (CLI)

#### 4.1.1 커맨드 시그니처

```
xflow agent export <id> -o <file>
xflow agent export --all -o <directory> [--export-format yaml|json]
```

#### 4.1.2 구현 위치

- **파일**: `internal/cli/agent.go`
- **함수**: `newAgentExportCmd(client **Client) *cobra.Command`
- **등록**: `newAgentCmd()` 내 `cmd.AddCommand(newAgentExportCmd(client))` 추가

#### 4.1.3 Export 데이터 구조

Export 파일에 포함되는 필드:

| 필드 | 타입 | 설명 |
|------|------|------|
| name | string | 에이전트 이름 |
| type | string | 에이전트 타입 |
| config | map[string]any | 에이전트 설정 (선택) |

Export 파일에서 제외되는 런타임 필드:

| 필드 | 제외 사유 |
|------|----------|
| id | 서버가 자동 생성 |
| status | 런타임 상태 |
| connected | 런타임 상태 |
| uptime | 런타임 통계 |
| messages_in | 런타임 통계 |
| messages_out | 런타임 통계 |
| error_count | 런타임 통계 |

#### 4.1.4 참조 패턴

`newFlowExportCmd()` (internal/cli/flow.go:272-323) 패턴을 따른다:
1. `(*client).Get("/api/v1/agents/"+id, &agent)` 로 에이전트 조회
2. `detectFileFormat(outputPath)` 로 출력 형식 감지
3. 런타임 필드 제거 (`stripRuntimeFields()` 헬퍼)
4. `json.MarshalIndent()` 또는 `yaml.Marshal()` 로 직렬화
5. `os.WriteFile()` 로 파일 저장

### 4.2 Module 2: Agent Import (CLI)

#### 4.2.1 커맨드 시그니처

```
xflow agent import -f <file>
xflow agent import -f <directory>
```

#### 4.2.2 구현 위치

- **파일**: `internal/cli/agent.go`
- **함수**: `newAgentImportCmd(client **Client) *cobra.Command`
- **등록**: `newAgentCmd()` 내 `cmd.AddCommand(newAgentImportCmd(client))` 추가

#### 4.2.3 Import 로직

`newFlowImportCmd()` (internal/cli/flow.go:327-363) 패턴을 따른다:
1. `readFile(filePath)` 로 파일 읽기
2. `detectFileFormat(filePath)` 로 형식 감지
3. JSON 또는 YAML 파싱
4. `AgentCreateRequest` 형식으로 래핑: `name`, `type`, `config`
5. `(*client).Post("/api/v1/agents", request, &result)` 로 API 호출

#### 4.2.4 Import 파일 형식

```yaml
# YAML 예제
name: "my-serial-agent"
type: "serial"
config:
  transport:
    type: "serial"
    options:
      port: "/dev/ttyUSB0"
      baud_rate: 9600
  protocol_file: "protocols/modbus.yaml"
```

```json
{
  "name": "my-tcp-agent",
  "type": "tcp",
  "config": {
    "transport": {
      "type": "tcp",
      "options": {
        "address": "192.168.1.100:502"
      }
    }
  }
}
```

### 4.3 Module 3: Batch Operations

#### 4.3.1 Batch Import 로직

1. `os.Stat(filePath)` 로 디렉토리 여부 판별
2. 디렉토리이면 `filepath.WalkDir()` 로 `.json`, `.yaml`, `.yml` 파일 탐색
3. 각 파일에 대해 단일 Import 로직 수행
4. 성공/실패 카운트 집계 및 결과 출력

#### 4.3.2 Batch Export 로직

1. `--all` 플래그 확인
2. `(*client).Get("/api/v1/agents", &agents)` 로 전체 에이전트 조회
3. `-o` 경로에 디렉토리 생성 (`os.MkdirAll`)
4. 각 에이전트에 대해 `stripRuntimeFields()` 적용
5. `--export-format` (기본: yaml) 형식으로 개별 파일 저장

### 4.4 Module 4: API Export Endpoint

#### 4.4.1 라우트 등록

| 메서드 | 경로 | 핸들러 |
|--------|------|--------|
| GET | /agents/{id}/export | AgentHandler.Export |
| GET | /agents/export | AgentHandler.ExportAll |

#### 4.4.2 구현 위치

- **파일**: `internal/api/handler/agent.go`
- **RegisterRoutes** 에 2개 라우트 추가
- 기존 `AgentManager` 인터페이스 메서드(`GetAgent`, `ListAgents`) 재사용

#### 4.4.3 응답 형식

```json
// GET /api/v1/agents/{id}/export
{
  "name": "my-serial-agent",
  "type": "serial",
  "config": {
    "transport": { "type": "serial", "options": {...} }
  }
}
```

```json
// GET /api/v1/agents/export
[
  { "name": "agent-1", "type": "serial", "config": {...} },
  { "name": "agent-2", "type": "tcp", "config": {...} }
]
```

### 4.5 Module 5: Agent Config Examples

#### 4.5.1 디렉토리 구조

```
examples/
  agents/
    serial-modbus.yaml      # Serial + Modbus 예제
    tcp-custom.json          # TCP + Custom Protocol 예제
    mqtt-sensor.yaml         # MQTT 센서 예제
```

#### 4.5.2 예제 파일 요구사항

- 각 파일은 `AgentCreateRequest` 필수 필드 포함
- `agent import -f` 명령으로 직접 Import 가능한 형식
- 주석(YAML) 또는 설명을 통해 각 필드의 용도 설명

### 4.6 Module 6: Agent Detail View Enhancement

#### 4.6.1 handler.AgentInfo 구조체 보강

기존 5개 필드에 다음 필드를 추가한다:

| 필드 | 타입 | JSON 태그 | 설명 |
|------|------|-----------|------|
| health | object | `json:"health,omitempty"` | 헬스 상태 (status, last_check) |
| stats | object | `json:"stats,omitempty"` | 메시지 통계 (messages_in, messages_out, errors) |
| uptime | string | `json:"uptime,omitempty"` | 가동 시간 (Duration 문자열) |
| started_at | time | `json:"started_at,omitempty"` | 시작 시각 (RFC3339) |
| created_at | time | `json:"created_at,omitempty"` | 생성 시각 (RFC3339) |
| connected | bool | `json:"connected"` | 연결 상태 |
| shared_info | object | `json:"shared_info,omitempty"` | 공유 정보 (ref_count, flows) - detail=full 시에만 포함 |
| state | any | `json:"state,omitempty"` | 타입별 커스텀 상태 - StatefulAgent + detail=full 시에만 포함 |

#### 4.6.2 StatefulAgent 인터페이스

```go
// StatefulAgent 는 타입별 커스텀 런타임 상태를 노출하는 에이전트가 구현하는 선택적 인터페이스이다.
type StatefulAgent interface {
    State() map[string]any  // 타입별 런타임 상태를 반환한다
}
```

- **위치**: `internal/agent/agent.go`
- **구현 예시**: Samsung NASA 에이전트 (`internal/agent/samsung/agent.go`)가 디바이스 목록 및 디바이스 상태를 반환

#### 4.6.3 API 상세 수준 쿼리 파라미터

| 파라미터 | 값 | 동작 |
|----------|------|------|
| `detail` | `summary` (기본값) | 보강된 기본 정보 반환 (health, stats, uptime, started_at, created_at, connected) |
| `detail` | `full` | summary + shared_info + state (StatefulAgent 구현 시) |

#### 4.6.4 API 응답 예시: detail=summary

```json
{
  "id": "abc-123",
  "name": "nasa-hvac",
  "type": "samsung-nasa",
  "status": "running",
  "config": {},
  "health": {"status": "healthy", "last_check": "2026-02-25T12:30:00Z"},
  "stats": {"messages_in": 1234, "messages_out": 1200, "errors": 5},
  "uptime": "2h30m15s",
  "started_at": "2026-02-25T10:00:00Z",
  "created_at": "2026-02-25T09:55:00Z",
  "connected": true
}
```

#### 4.6.5 API 응답 예시: detail=full (StatefulAgent 구현 시)

```json
{
  "id": "abc-123",
  "name": "nasa-hvac",
  "type": "samsung-nasa",
  "status": "running",
  "config": {},
  "health": {"status": "healthy", "last_check": "2026-02-25T12:30:00Z"},
  "stats": {"messages_in": 1234, "messages_out": 1200, "errors": 5},
  "uptime": "2h30m15s",
  "started_at": "2026-02-25T10:00:00Z",
  "created_at": "2026-02-25T09:55:00Z",
  "connected": true,
  "shared_info": {"ref_count": 2, "flows": ["flow-1", "flow-2"]},
  "state": {
    "devices": [
      {
        "address": "20.00",
        "device_id": "indoor-1",
        "type": "indoor",
        "online": true,
        "state": {
          "power": true,
          "mode": "cool",
          "target_temp": 24.0,
          "current_temp": 25.5
        }
      }
    ]
  }
}
```

#### 4.6.6 agent_adapter.go 변환 로직 변경

- **파일**: `internal/api/service/agent_adapter.go`
- **함수**: `agentToHandlerInfo(agent.AgentInfo, detail string) handler.AgentInfo`
- `detail` 파라미터를 추가하여 변환 수준을 제어한다
- `detail=summary`: 도메인 AgentInfo의 Health, Stats, Uptime, StartedAt, CreatedAt 필드를 handler DTO에 매핑
- `detail=full`: summary 매핑에 추가로 SharedInfo를 매핑하고, 에이전트가 StatefulAgent 인터페이스를 구현하면 `State()` 호출 결과를 `state` 필드에 포함

#### 4.6.7 CLI 출력 형식: 기본 (summary)

```
ID:        abc-123
Name:      nasa-hvac
Type:      samsung-nasa
Status:    running
Health:    healthy
Uptime:    2h30m15s
Connected: true

Stats:
  Messages In:  1,234
  Messages Out: 1,200
  Errors:       5
```

#### 4.6.8 CLI 출력 형식: --detail

기본 출력에 추가로 다음을 포함한다:

```
SharedInfo:
  Ref Count: 2
  Flows:     flow-1, flow-2

State:
  Devices: 3
  - indoor-1 (20.00): online, cool 24.0°C (current: 25.5°C)
  - indoor-2 (20.01): online, heat 22.0°C (current: 20.1°C)
  - outdoor-1 (10.00): online
```

#### 4.6.9 CLI --detail 플래그

- **파일**: `internal/cli/agent.go`
- `newAgentGetCmd()` 함수에 `--detail` bool 플래그 추가
- `--detail` 플래그가 설정되면 API 호출 시 `?detail=full` 쿼리 파라미터 전달
- 기본값은 `detail=summary`

#### 4.6.10 영향받는 파일

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| `internal/agent/agent.go` | 인터페이스 추가 | `StatefulAgent` 인터페이스 정의 |
| `internal/api/handler/agent.go` | 구조체 확장 | `AgentInfo` DTO에 새 필드 추가 |
| `internal/api/service/agent_adapter.go` | 함수 수정 | `agentToHandlerInfo`에 detail 파라미터 추가, 매핑 로직 보강 |
| `internal/cli/agent.go` | CLI 확장 | `agent get`에 `--detail` 플래그 추가, 구조화된 출력 구현 |
| `internal/agent/samsung/agent.go` | 인터페이스 구현 | `StatefulAgent` 인터페이스 구현 (디바이스 상태 노출) |

---

## 5. Traceability (추적성)

### 5.1 SPEC 간 참조

| 참조 SPEC | 관계 |
|-----------|------|
| SPEC-AGENT-001 | Agent System 프레임워크 (선행 의존) |
| SPEC-CLI-001 | CLI 기본 모듈 (node, plugin, status) |
| SPEC-CLI-002 | CLI 인터랙티브 모드 |

### 5.2 파일-모듈 매핑

| 파일 | 모듈 | 변경 유형 |
|------|------|----------|
| `internal/cli/agent.go` | Module 1, 2, 3, 6 | 기존 파일 확장 |
| `internal/api/handler/agent.go` | Module 4, 6 | 기존 파일 확장 |
| `internal/api/service/agent_adapter.go` | Module 6 | 기존 파일 수정 |
| `internal/agent/agent.go` | Module 6 | 인터페이스 추가 |
| `internal/agent/samsung/agent.go` | Module 6 | 인터페이스 구현 |
| `examples/agents/serial-modbus.yaml` | Module 5 | 신규 생성 |
| `examples/agents/tcp-custom.json` | Module 5 | 신규 생성 |
| `examples/agents/mqtt-sensor.yaml` | Module 5 | 신규 생성 |

### 5.3 요구사항-모듈 매핑

| 요구사항 ID | 모듈 | 우선순위 |
|------------|------|---------|
| REQ-AGENT-002-01 ~ 05 | Module 1: Agent Export (CLI) | P0 |
| REQ-AGENT-002-06 ~ 10 | Module 2: Agent Import (CLI) | P0 |
| REQ-AGENT-002-11 ~ 16 | Module 3: Batch Operations | P1 |
| REQ-AGENT-002-17 ~ 20 | Module 4: API Export Endpoint | P1 |
| REQ-AGENT-002-21 ~ 23 | Module 5: Agent Config Examples | P0 |
| REQ-AGENT-002-24 ~ 30 | Module 6: Agent Detail View Enhancement | P0 |
