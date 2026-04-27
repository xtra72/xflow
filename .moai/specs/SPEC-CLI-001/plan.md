---
id: SPEC-CLI-001
type: plan
version: "1.1.0"
status: draft
created: "2026-02-13"
updated: "2026-03-10"
author: xtra
---

# SPEC-CLI-001: CLI Tool (xflow) - 구현 계획

## 1. 개요 및 접근 방식

### 1.1 개발 방법론

Hybrid 모드 적용 (quality.yaml 설정 준수):
- **신규 코드 (TDD)**: `internal/cli/` 및 `cmd/xflow/` 전체가 신규 작성이므로 RED-GREEN-REFACTOR 사이클 적용
- 테스트 커버리지 목표: 85% 이상
- 모든 명령어 핸들러, API 클라이언트, 출력 포맷터에 대해 테스트를 먼저 작성한 후 구현

### 1.2 핵심 설계 결정

1. **인터페이스 기반 API 클라이언트**: `APIClient` 인터페이스를 정의하여 테스트 시 모킹 가능
2. **OutputFormatter 인터페이스**: 포맷별 구현체를 교체 가능하도록 설계
3. **Cobra 표준 패턴**: Cobra 라이브러리의 RunE 패턴을 사용하여 에러를 상위로 전파
4. **설정 통합**: Viper를 사용하여 플래그/환경변수/파일을 자동 병합
5. **에러 래핑**: API 에러를 CLI 에러 타입으로 변환하는 중앙 매핑 레이어

### 1.3 기술 스택

| 구성 요소 | 선택 | 비고 |
|-----------|------|------|
| CLI Framework | github.com/spf13/cobra v1.8+ | 서브커맨드 구조 |
| Config | github.com/spf13/viper v1.18+ | 다중 소스 설정 관리 |
| HTTP Client | net/http (표준 라이브러리) | 외부 의존성 최소화 |
| JSON | encoding/json (표준 라이브러리) | 직렬화/역직렬화 |
| YAML | gopkg.in/yaml.v3 | YAML 출력/파싱 |
| Table | text/tabwriter (표준 라이브러리) | 테이블 포맷팅 |
| Color | github.com/fatih/color | 터미널 색상 |
| Testing | stretchr/testify v1.9+ | 테스트 어설션 |

---

## 2. 파일별 구현 상세

### 2.1 P0 핵심 파일 (1차 마일스톤)

#### `internal/cli/errors.go` (~100 라인)
- `CLIError` 구조체 정의 (Message, Hint, Cause, ExitCode)
- 7개 사전 정의 에러 인스턴스:
  - `ErrServerUnreachable`, `ErrAuthenticationFailed`, `ErrPermissionDenied`
  - `ErrResourceNotFound`, `ErrInvalidInput`, `ErrFileNotFound`, `ErrConfigNotInitialized`
- `MapAPIError(statusCode int, body []byte) *CLIError`: HTTP 에러 -> CLI 에러 변환
- `FormatError(err *CLIError, verbose bool) string`: 에러 메시지 포맷팅
- 의존성: 없음 (순수 에러 타입)

#### `internal/cli/output.go` (~200 라인)
- `OutputFormatter` 인터페이스: `Format(data any) (string, error)`
- `TableFormatter` 구현: `text/tabwriter` 기반, 헤더 포함, 자동 정렬
- `JSONFormatter` 구현: `json.MarshalIndent` 기반, 2-space 들여쓰기
- `YAMLFormatter` 구현: `yaml.Marshal` 기반
- `TextFormatter` 구현: 키=값 또는 줄당 하나의 값 출력
- `NewFormatter(format string) OutputFormatter`: 팩토리 함수
- `PrintResult(formatter OutputFormatter, data any, w io.Writer) error`: 출력 헬퍼
- 색상 지원: `IsTerminal()` 감지, `--no-color` 처리
- 프로그레스 스피너: `StartSpinner(msg)`, `StopSpinner()`
- 의존성: `gopkg.in/yaml.v3`, `github.com/fatih/color`

#### `internal/cli/client.go` (~250 라인)
- `HTTPClient` 구조체:
  - `baseURL string`, `token string`, `httpClient *http.Client`, `verbose bool`
- `NewHTTPClient(baseURL, token string, timeout time.Duration) *HTTPClient`
- `Get(path string) ([]byte, error)`: GET 요청
- `Post(path string, body any) ([]byte, error)`: POST 요청 (JSON 직렬화)
- `Put(path string, body any) ([]byte, error)`: PUT 요청
- `Delete(path string) ([]byte, error)`: DELETE 요청
- `doRequest(method, path string, body io.Reader) ([]byte, error)`: 내부 공통 요청 로직
  - 인증 헤더 자동 주입
  - Content-Type 헤더 설정
  - 응답 상태 코드 검사 및 에러 매핑
  - verbose 모드 시 요청/응답 로깅
- `Ping() error`: `/health` 엔드포인트 호출
- `StreamSSE(path string, handler func(event string)) error`: SSE 스트리밍 수신
- 타임아웃: 기본 30초, 설정 가능
- 재시도: 최대 3회, 지수 백오프 (1s, 2s, 4s)
- 의존성: `internal/cli/errors.go`

#### `internal/cli/root.go` (~180 라인)
- `NewRootCmd() *cobra.Command`: 루트 명령어 생성
- 글로벌 플래그 등록:
  - `--config`, `--server`, `--format`, `--token`, `--verbose`, `--quiet`, `--no-color`
- `PersistentPreRunE`: Viper 설정 바인딩, 설정 파일 로딩, HTTPClient 초기화
- 서버 URL 해석: 플래그 > `XFLOW_SERVER` > config > default
- 토큰 해석: 플래그 > `XFLOW_TOKEN` > config
- 서브커맨드 등록: flow, agent, node, plugin, config, status, logs, metrics, version
- `versionCmd`: 빌드 정보 출력 (version, commit, date)
- 의존성: `cobra`, `viper`, `internal/cli/client.go`, `internal/cli/output.go`

#### `internal/cli/config.go` (~200 라인)
- `newConfigCmd(client *HTTPClient) *cobra.Command`: config 서브커맨드 그룹
- `configInitCmd()`: `~/.xflow/config.yaml` 기본 설정 생성
  - 기본 템플릿: server.url, auth.token, output.format, output.color, logging.verbose
  - 기존 파일 존재 시 덮어쓰기 확인
- `configGetCmd()`: 키 기반 설정 조회
- `configSetCmd()`: 키-값 설정 변경, 파일 저장
- `configServerCmd()`: `server.url` 단축 설정
- `configTokenCmd()`: `auth.token` 단축 설정
- `configListCmd()`: 전체 설정 출력, 민감 정보 마스킹
- 의존성: `viper`, `os` (파일 I/O)

#### `internal/cli/flow.go` (~350 라인)
- `newFlowCmd(client *HTTPClient, formatter OutputFormatter) *cobra.Command`: flow 서브커맨드 그룹
- `flowListCmd()`: GET /api/v1/flows -> 테이블 출력
- `flowGetCmd()`: GET /api/v1/flows/{id} -> 상세 출력
- `flowCreateCmd()`: 파일 읽기 -> POST /api/v1/flows
- `flowUpdateCmd()`: 파일 읽기 -> PUT /api/v1/flows/{id}
- `flowDeleteCmd()`: 확인 프롬프트 -> DELETE /api/v1/flows/{id}
  - `--yes` 플래그로 확인 생략
- `flowDeployCmd()`: POST /api/v1/flows/{id}/deploy
- `flowStartCmd()`: POST /api/v1/flows/{id}/start
- `flowStopCmd()`: POST /api/v1/flows/{id}/stop
- `flowRestartCmd()`: POST /api/v1/flows/{id}/restart
- `flowExportCmd()`: GET /api/v1/flows/{id}/export -> 파일 저장
  - 확장자 기반 형식 결정 (.json, .yaml, .yml)
- `flowImportCmd()`: 파일 읽기 -> POST /api/v1/flows/import
- `flowStatusCmd()`: GET /api/v1/flows/{id}/status -> 상태 출력
- 공통 헬퍼: `readFlowFile(path string) ([]byte, error)`, `confirmAction(msg string) bool`
- 의존성: `client.go`, `output.go`, `errors.go`

#### `internal/cli/agent.go` (~250 라인)
- `newAgentCmd(client *HTTPClient, formatter OutputFormatter) *cobra.Command`
- `agentListCmd()`: GET /api/v1/agents -> 테이블 출력
- `agentGetCmd()`: GET /api/v1/agents/{id} -> 상세 출력
- `agentCreateCmd()`: 파일 읽기 -> POST /api/v1/agents
- `agentStartCmd()`: POST /api/v1/agents/{id}/start
- `agentStopCmd()`: POST /api/v1/agents/{id}/stop
- `agentRestartCmd()`: POST /api/v1/agents/{id}/restart
- `agentDeleteCmd()`: 확인 프롬프트 -> DELETE /api/v1/agents/{id}
  - 참조 플로우 존재 시 삭제 거부 메시지
- 의존성: `client.go`, `output.go`, `errors.go`

#### `cmd/xflow/main.go` (~30 라인)
- `main()` 함수: `cli.NewRootCmd()` 호출, `cmd.Execute()` 실행
- 에러 발생 시 `os.Exit(1)`
- 빌드 변수: `version`, `commit`, `date` (링크 타임 주입)
- 의존성: `internal/cli/`

### 2.2 P1 확장 파일 (2차 마일스톤)

#### `internal/cli/node.go` (~100 라인)
- `newNodeCmd(client *HTTPClient, formatter OutputFormatter) *cobra.Command`
- `nodeListCmd()`: GET /api/v1/nodes -> 카테고리별 테이블 출력
- `nodeInfoCmd()`: GET /api/v1/nodes/{type} -> 상세 정보 (포트, 설정 스키마)
- 의존성: `client.go`, `output.go`

#### `internal/cli/plugin.go` (~180 라인)
- `newPluginCmd(client *HTTPClient, formatter OutputFormatter) *cobra.Command`
- `pluginListCmd()`: GET /api/v1/plugins -> 테이블 출력
- `pluginInstallCmd()`: POST /api/v1/plugins -> 프로그레스 바 표시
- `pluginRemoveCmd()`: 확인 프롬프트 -> DELETE /api/v1/plugins/{name}
- `pluginUpdateCmd()`: PUT /api/v1/plugins/{name} -> 프로그레스 바 표시
- 의존성: `client.go`, `output.go`, `errors.go`

#### `internal/cli/status.go` (~200 라인)
- `newStatusCmd(client *HTTPClient, formatter OutputFormatter) *cobra.Command`
- `statusCmd()`: GET /api/v1/system/status -> 서버 상태 출력
- `newLogsCmd(client *HTTPClient) *cobra.Command`
  - SSE 기반 실시간 로그 스트리밍
  - `Ctrl+C` 인터럽트 핸들링 (`os/signal`)
  - 컴포넌트별 로그 필터링
- `newMetricsCmd(client *HTTPClient, formatter OutputFormatter) *cobra.Command`
  - GET /api/v1/system/metrics -> 메트릭 요약 출력
- 의존성: `client.go`, `output.go`

---

## 3. 마일스톤

### 마일스톤 1: 핵심 기반 (Primary Goal)

**목표**: CLI 프레임워크, API 클라이언트, 핵심 명령어 구현

| 순서 | 파일 | 예상 라인 | 설명 |
|------|------|----------|------|
| 1 | errors.go | ~100 | 에러 타입 정의 (모든 모듈의 기반) |
| 2 | output.go | ~200 | 출력 포맷터 (테이블/JSON/YAML/텍스트) |
| 3 | client.go | ~250 | HTTP 클라이언트 (API 통신 핵심) |
| 4 | root.go | ~180 | 루트 명령어, 설정 로딩 |
| 5 | config.go | ~200 | 설정 명령어 (초기 설정 필수) |
| 6 | flow.go | ~350 | 플로우 명령어 (핵심 기능) |
| 7 | agent.go | ~250 | Agent 명령어 (핵심 기능) |
| 8 | main.go | ~30 | 바이너리 엔트리포인트 |

**예상 총 라인**: ~1,560 라인 (테스트 코드 별도)

**완료 기준**:
- `xflow version` 정상 동작
- `xflow config init/get/set/server/token/list` 정상 동작
- `xflow flow list/get/create/update/delete/deploy/start/stop/restart/export/import/status` 정상 동작
- `xflow agent list/get/create/start/stop/restart/delete` 정상 동작
- 모든 출력 형식(table/json/yaml/text) 정상 동작
- 에러 메시지에 해결 방안 포함
- 테스트 커버리지 85% 이상

### 마일스톤 2: 확장 기능 (Secondary Goal)

**목표**: 보조 명령어 및 모니터링 기능 추가

| 순서 | 파일 | 예상 라인 | 설명 |
|------|------|----------|------|
| 1 | node.go | ~100 | 노드 타입 조회 |
| 2 | plugin.go | ~180 | 플러그인 관리 |
| 3 | status.go | ~200 | 서버 상태, 로그, 메트릭 |

**예상 총 라인**: ~480 라인 (테스트 코드 별도)

**완료 기준**:
- `xflow node list/info` 정상 동작
- `xflow plugin list/install/remove/update` 정상 동작
- `xflow status` 서버 상태 표시
- `xflow logs <component>` 실시간 로그 스트리밍
- `xflow metrics` 메트릭 요약 표시
- 테스트 커버리지 85% 이상

### 마일스톤 2.5: CLI 출력 가독성 개선 (Priority High - v1.1.0)

**목표**: `flow get`, `status`, `TextFormatter`의 중첩 데이터 출력 가독성 개선

**배경**: `xflow flow get <name>` 실행 시 config 내부의 nodes/edges 데이터가 Go raw `map[...]` 형식으로 출력되어 사용자가 읽을 수 없는 문제 발견. `agent get`, `flow status`는 이미 `DetailFormatter`를 사용하여 가독성 높은 출력을 제공하고 있으므로, 동일한 패턴을 미적용 명령어에 확장 적용.

#### 2.5.1 Module 11: Flow Detail Formatter (P0)

| 순서 | 작업 | 대상 파일 | 설명 |
|------|------|----------|------|
| 1 | 플로우 상세 필드 정의 | flow.go | `flowDetailFieldOrder`, `flowDetailLabelMap`, `flowDetailSectionKeys` 변수 정의 |
| 2 | 노드 요약 추출 헬퍼 | flow.go | `extractNodeSummary(config map[string]any) []map[string]any` - nodes에서 label, type, direction, agent_name 추출 |
| 3 | 노드 ID-라벨 매핑 빌더 | flow.go | `buildNodeIDMap(nodes []any) map[string]string` - id -> data.label 매핑 테이블 |
| 4 | 엣지 가독성 변환 헬퍼 | flow.go | `formatEdges(edges []any, nodeIDMap map[string]string) []string` - source/target ID를 라벨로 치환하여 `label1 -> label2 (out -> in)` 형식 반환 |
| 5 | flowGetCmd 리팩터링 | flow.go | `TextFormatter` 폴백을 `DetailFormatter` + 커스텀 섹션 렌더링으로 교체 |
| 6 | 플로우 데이터 전처리 | flow.go | API 응답 map에서 `node_count`, 노드 요약, 엣지 요약을 계산하여 출력용 데이터 구조 생성 |
| 7 | --detail 플래그 추가 | flow.go | `--detail summary|full` 플래그로 출력 상세 수준 제어 (선택적) |

**구현 상세**:

`flow get`의 현재 코드:
```go
// 현재 (문제)
if format == "table" {
    format = "text"  // TextFormatter로 폴백 -> raw map[...] 출력
}
return PrintResult(w, format, flow, nil, nil)
```

개선 후:
```go
// 개선 (DetailFormatter 사용)
if format == "table" || format == "text" {
    displayData := prepareFlowDetail(flow)  // nodes/edges 전처리
    df := NewDetailFormatter(flowDetailFieldOrder, flowDetailLabelMap, flowDetailSectionKeys)
    return df.Format(displayData, w)
}
return PrintResult(w, format, flow, nil, nil)
```

`prepareFlowDetail` 함수는:
1. API 응답에서 `config.nodes` 배열의 길이를 `node_count`로 계산
2. 각 노드에서 `data.label`, `data.nodeType`, `data.direction`, `data.agent_name`을 추출하여 미니 테이블 데이터 생성
3. `id` -> `data.label` 매핑 테이블을 구축
4. 각 엣지의 `source`/`target` 노드 ID를 라벨로 치환하여 가독성 높은 문자열 생성
5. 전처리된 map을 반환

**기대 출력**:
```
ID:         883190bb-b594-4da5-9b32-2bf2f9168638
Name:       mqtt-to-modbus-v2
Status:     running
Nodes:      8
Created At: 2026-03-10T10:47:09+09:00

Nodes:
  LABEL               TYPE        DIRECTION  AGENT
  mqtt-receiver       bridge      in         mqtt-sensor-agent
  address-resolver    transform   -          -
  sensor-mapper       transform   -          -
  modbus-writer       bridge      out        modbus-gateway-server
  modbus-reader       bridge      in         modbus-gateway-server
  monitor-formatter   transform   -          -
  monitor-logger      bridge      out        console-logger
  error-logger        bridge      out        error-logger

Edges:
  mqtt-receiver -> address-resolver (out -> in)
  address-resolver -> sensor-mapper (out -> in)
  sensor-mapper -> modbus-writer (out -> in)
  modbus-reader -> monitor-formatter (out -> in)
  monitor-formatter -> monitor-logger (out -> in)
```

#### 2.5.2 Module 12: Status Detail Formatter (P1)

| 순서 | 작업 | 대상 파일 | 설명 |
|------|------|----------|------|
| 1 | 상태 필드 정의 | status.go | `statusFieldOrder`, `statusLabelMap`, `statusSectionKeys` 변수 정의 |
| 2 | statusCmd 리팩터링 | status.go | `format = "text"` 폴백을 `DetailFormatter`로 교체 |
| 3 | metricsCmd 리팩터링 | status.go | `format = "text"` 폴백을 `DetailFormatter`로 교체, 메트릭 카테고리를 섹션으로 분리 |

**구현 상세**:

status 명령어의 응답 구조에 맞춰 fieldOrder를 정의. API 응답에 포함되는 주요 필드(version, uptime, running_flows, active_agents 등)를 기반으로 순서와 라벨을 매핑.

메트릭 명령어의 경우, CPU/메모리/디스크 등의 카테고리를 `sectionKeys`로 등록하여 DetailFormatter의 섹션 렌더링 기능 활용.

#### 2.5.3 Module 13: TextFormatter Enhancement (P1)

| 순서 | 작업 | 대상 파일 | 설명 |
|------|------|----------|------|
| 1 | formatMap 타입 분기 추가 | output.go | 값이 map/slice인 경우 재귀 렌더링으로 분기 |
| 2 | 중첩 map 렌더링 구현 | output.go | `formatMapIndented(v reflect.Value, writer io.Writer, indent string)` 추가 |
| 3 | 중첩 slice 렌더링 구현 | output.go | `formatSliceIndented(v reflect.Value, writer io.Writer, indent string)` 추가 |
| 4 | 기존 테스트 업데이트 | output_test.go | 중첩 데이터 렌더링 테스트 케이스 추가 |

**구현 상세**:

현재 `formatMap`의 문제 코드:
```go
// 현재 (문제)
fmt.Fprintf(writer, "%s: %v\n", key, val.Interface())
// -> config: map[edges:[map[id:... source:... target:...] ...] nodes:[...]]
```

개선 후:
```go
// 개선 (타입 분기 + 재귀 렌더링)
switch val.Kind() {
case reflect.Map:
    fmt.Fprintf(writer, "%s:\n", key)
    formatMapIndented(val, writer, "  ")
case reflect.Slice:
    fmt.Fprintf(writer, "%s:\n", key)
    formatSliceIndented(val, writer, "  ")
default:
    fmt.Fprintf(writer, "%s: %v\n", key, val.Interface())
}
```

**완료 기준**:
- `xflow flow get <name>` 출력이 구조화된 가독성 높은 형식
- `xflow status` 출력이 필드 정렬된 DetailFormatter 형식
- `xflow status metrics` 출력이 섹션별로 구분된 형식
- TextFormatter가 중첩 map/slice를 들여쓰기로 렌더링
- 기존 DetailFormatter 사용 명령어(agent get, flow status)에 영향 없음
- `--format json`/`--format yaml` 출력에 영향 없음
- 테스트 커버리지 85% 이상 유지

---

### 마일스톤 3: 품질 완성 (Final Goal)

**목표**: 품질 기준 완전 충족 및 크로스 플랫폼 검증

**완료 기준**:
- 전체 테스트 커버리지 85% 이상
- `go vet` 및 `golangci-lint` 경고 0건
- Linux, macOS, Windows 크로스 컴파일 검증
- 셸 자동 완성(bash, zsh, fish, PowerShell) 생성
- GoReleaser 빌드 설정 완료

---

## 4. 의존성 그래프

```
cmd/xflow/main.go
    └── internal/cli/root.go
            ├── internal/cli/errors.go     (에러 타입)
            ├── internal/cli/output.go     (출력 포맷터)
            ├── internal/cli/client.go     (API 클라이언트)
            │     └── internal/cli/errors.go
            ├── internal/cli/config.go     (설정 명령어)
            ├── internal/cli/flow.go       (플로우 명령어)
            │     ├── internal/cli/client.go
            │     ├── internal/cli/output.go
            │     └── internal/cli/errors.go
            ├── internal/cli/agent.go      (Agent 명령어)
            │     ├── internal/cli/client.go
            │     ├── internal/cli/output.go
            │     └── internal/cli/errors.go
            ├── internal/cli/node.go       (노드 명령어)
            ├── internal/cli/plugin.go     (플러그인 명령어)
            └── internal/cli/status.go     (상태/로그/메트릭)

외부 의존 SPEC:
    SPEC-API-001 ────── REST API 엔드포인트 (CLI가 소비)
    SPEC-AUTH-001 ───── JWT/API Key 인증
    SPEC-CFG-001 ────── 서버측 설정 시스템
```

구현 순서 제약:
- `errors.go` -> 모든 파일 이전에 구현 (에러 타입 기반)
- `output.go` -> 명령어 파일들 이전에 구현 (출력 포맷 기반)
- `client.go` -> 명령어 파일들 이전에 구현 (API 통신 기반)
- `root.go` -> 명령어 파일들 이전에 구현 (프레임워크 기반)
- `config.go` -> 다른 명령어들보다 먼저 (초기 설정 필수)
- `flow.go`, `agent.go` -> 독립적, 병렬 구현 가능
- `node.go`, `plugin.go`, `status.go` -> 마일스톤 1 이후

---

## 5. 리스크 분석

### R1: SPEC-API-001 구현 미완료 (확률: 중 / 영향: 고)
- **설명**: CLI가 소비하는 REST API 엔드포인트가 아직 구현되지 않았을 수 있다
- **대응**: API 클라이언트를 인터페이스로 추상화하여 목(mock) 기반 테스트 가능. API 스텁 서버를 통한 통합 테스트

### R2: API 응답 형식 불일치 (확률: 중 / 영향: 중)
- **설명**: 서버의 실제 API 응답 형식이 SPEC-API-001 명세와 다를 수 있다
- **대응**: 응답 파싱 로직에 방어적 프로그래밍 적용. 선택적(optional) 필드 처리. 통합 테스트로 조기 발견

### R3: 크로스 플랫폼 호환성 (확률: 저 / 영향: 중)
- **설명**: 설정 파일 경로(`~/.xflow/`)가 Windows에서 다르게 동작할 수 있다
- **대응**: `os.UserHomeDir()` 사용. 플랫폼별 경로 처리 테스트 추가

### R4: SSE 스트리밍 안정성 (확률: 중 / 영향: 저)
- **설명**: 로그 스트리밍(SSE)이 네트워크 불안정 환경에서 연결 끊김이 발생할 수 있다
- **대응**: 자동 재연결 메커니즘 구현. 연결 끊김 시 사용자에게 알림. P1 우선순위로 후순위 구현

### R5: Cobra/Viper 설정 충돌 (확률: 저 / 영향: 중)
- **설명**: Cobra 플래그와 Viper 설정 파일 간 바인딩에서 우선순위 문제가 발생할 수 있다
- **대응**: `viper.BindPFlag()` 사용으로 플래그 우선순위 보장. 단위 테스트로 우선순위 검증

### R6: 파일 형식 자동 감지 실패 (확률: 저 / 영향: 저)
- **설명**: 사용자가 확장자 없는 파일이나 잘못된 확장자를 가진 파일을 전달할 수 있다
- **대응**: 파일 내용의 첫 문자 검사로 JSON/YAML 자동 감지 시도. 감지 실패 시 명확한 에러 메시지

### R7: 인증 토큰 보안 (확률: 중 / 영향: 중)
- **설명**: 설정 파일에 저장된 토큰이 평문으로 노출될 수 있다
- **대응**: 설정 파일 권한을 600으로 설정. `config list` 명령어에서 토큰 마스킹. 환경 변수 사용 권장 문서화

---

## 6. 파일 의존성 매트릭스

| 파일 | errors.go | output.go | client.go | root.go | cobra | viper | yaml.v3 | color |
|------|:---------:|:---------:|:---------:|:-------:|:-----:|:-----:|:-------:|:-----:|
| errors.go | - | | | | | | | |
| output.go | | - | | | | | O | O |
| client.go | O | | - | | | | | |
| root.go | O | O | O | - | O | O | | |
| config.go | O | | | | O | O | | |
| flow.go | O | O | O | | O | | O | |
| agent.go | O | O | O | | O | | | |
| node.go | | O | O | | O | | | |
| plugin.go | O | O | O | | O | | | |
| status.go | | O | O | | O | | | |
| main.go | | | | O | O | | | |

O = 의존

---

## 7. v1.1.0 변경 영향 범위

### 수정 대상 파일

| 파일 | 변경 유형 | 예상 라인 변경 | 설명 |
|------|----------|--------------|------|
| `internal/cli/flow.go` | 수정 | +80~120 | flowGetCmd에 DetailFormatter 적용, 노드/엣지 전처리 헬퍼 추가 |
| `internal/cli/status.go` | 수정 | +30~50 | statusCmd, metricsCmd에 DetailFormatter 적용 |
| `internal/cli/output.go` | 수정 | +40~60 | TextFormatter.formatMap/formatSlice 재귀 렌더링 추가 |
| `internal/cli/output_test.go` | 수정 | +50~80 | 중첩 데이터 렌더링 테스트 추가 |
| `internal/cli/flow_test.go` | 수정 | +30~50 | flow get 출력 형식 테스트 추가 |

### 영향 없는 파일

- `internal/cli/agent.go` - 이미 DetailFormatter 사용 중
- `internal/cli/client.go` - 출력 계층과 무관
- `internal/cli/root.go` - 프레임워크 계층, 변경 불필요
- `internal/cli/config.go` - 설정 명령어, 변경 불필요
- `internal/cli/errors.go` - 에러 타입, 변경 불필요

### 하위 호환성

- `--format json`, `--format yaml` 출력은 변경 없음 (API 응답 원본 그대로)
- `--format text`의 중첩 데이터 출력만 개선 (기존 단순 키-값은 동일)
- `--format table`(기본값)의 단일 객체 출력이 개선 (기존 테이블 목록 출력은 동일)

---

*문서 버전: 1.1.0*
*최종 수정: 2026-03-10*
*작성: MoAI manager-spec*
