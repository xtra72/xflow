# cli - xflow CLI 커맨드라인 도구

`internal/cli` 패키지는 xflow 워크플로우 오케스트레이션 플랫폼의 커맨드라인 인터페이스이다. xflowd 서버와 REST API 로 통신하여 플로우, 에이전트, 플러그인, 노드 등의 리소스를 관리하며, 다중 출력 형식(json/yaml/table/text)을 지원한다.

**SPEC**: SPEC-CLI-001

## 아키텍처 개요

```
    xflow CLI 아키텍처

    +---------------------------------------------------+
    |                    cmd/xflow/main.go               |
    |  CLIError 핸들링, os.Exit(exitCode)                |
    +---------------------------------------------------+
                           |
    +---------------------------------------------------+
    |               NewRootCmd (root.go)                 |
    |  7 글로벌 플래그, PersistentPreRunE                |
    |  설정 로딩 + Client 초기화                         |
    |  우선순위: flag > env > config > default           |
    +---------------------------------------------------+
         |          |         |        |       |      |
    +----+---+ +----+--+ +---+---+ +--+--+ +-+----+ ++------+
    | flow   | | agent | | config| | node| |plugin| |status |
    | 12 sub | | 7 sub | | 6 sub | | 2   | | 4   | | 3 sub |
    +--------+ +-------+ +-------+ +-----+ +-----+ +-------+
         |                                      |
    +----+----+                           +-----+-----+
    | Client  | <-- HTTP/JSON ----------> | xflowd    |
    | (REST)  |    GET/POST/PUT/DELETE    | API 서버  |
    +----+----+                           +-----------+
         |
    +----+----+
    | output  |  json | yaml | table | text
    +---------+
```

**핵심 설계 원칙**:

1. **클로저 기반 클라이언트 공유**: `var client *Client`를 `NewRootCmd` 내에서 선언하고, `**Client`로 서브커맨드에 전달하여 `PersistentPreRunE`에서 초기화된 클라이언트를 공유
2. **의존성 주입 테스트**: `confirmFn func(string, io.Reader) bool`을 주입하여 대화형 확인 프롬프트를 테스트에서 제어 가능
3. **우선순위 기반 설정 해석**: 서버 URL 및 토큰을 `--flag` > `환경변수` > `설정 파일` > `기본값` 순서로 결정
4. **httptest 기반 테스트**: 모든 API 호출을 `net/http/httptest` 모의 서버로 테스트하여 외부 의존성 제거
5. **다중 출력 형식**: 모든 list/get 커맨드가 `--format json|yaml|table|text`를 지원

## 핵심 타입 및 역할

### Client (`client.go`)

xflowd API 서버와 HTTP 통신을 수행하는 클라이언트이다.

```go
type Client struct {
    baseURL    string
    token      string
    httpClient *http.Client
    verbose    bool
}

func NewClient(baseURL, token string, timeout time.Duration, verbose bool) *Client
func (c *Client) Get(path string, result any) error
func (c *Client) Post(path string, body any, result any) error
func (c *Client) Put(path string, body any, result any) error
func (c *Client) Delete(path string, result any) error
func (c *Client) GetRaw(path string) ([]byte, error)
func (c *Client) PostRaw(path string, body any) ([]byte, error)
func (c *Client) Ping() error
```

- **인증 토큰 주입**: `Authorization: Bearer <token>` 헤더 자동 설정
- **Verbose 로깅**: `--verbose` 시 `[HTTP] GET /api/v1/flows` 형태로 stderr 출력
- **API 엔벨로프 파싱**: `{"success": bool, "data": ..., "error": ...}` 구조를 자동 해석
- **에러 매핑**: HTTP 상태 코드(401, 403, 404 등)를 `CLIError`로 자동 변환

### CLIError (`errors.go`)

사용자 친화적 에러 타입으로, 힌트 메시지와 종료 코드를 포함한다.

```go
type CLIError struct {
    Message  string
    Hint     string
    Cause    error
    ExitCode int
}

func (e *CLIError) Error() string
func (e *CLIError) Unwrap() error

func ErrServerUnreachable(url string) *CLIError
func ErrAuthenticationFailed() *CLIError
func ErrPermissionDenied() *CLIError
func ErrResourceNotFound(resourceType, id string) *CLIError
func ErrInvalidInput(detail string) *CLIError
func ErrFileNotFound(path string) *CLIError
func ErrConfigNotInitialized() *CLIError

func MapAPIError(statusCode int, body []byte) *CLIError
func FormatError(err *CLIError, verbose bool) string
```

| 에러 팩토리 | 용도 |
|------------|------|
| `ErrServerUnreachable` | 서버 연결 실패 시 |
| `ErrAuthenticationFailed` | 401 인증 실패 시 |
| `ErrPermissionDenied` | 403 권한 거부 시 |
| `ErrResourceNotFound` | 리소스를 찾을 수 없을 때 |
| `ErrInvalidInput` | 잘못된 사용자 입력 시 |
| `ErrFileNotFound` | 파일을 찾을 수 없을 때 |
| `ErrConfigNotInitialized` | 설정 미초기화 시 |

### 출력 포매터 (`output.go`)

다중 형식 출력을 위한 `Formatter` 인터페이스와 구현체를 제공한다.

```go
type Formatter interface {
    Format(data any, writer io.Writer) error
}

type JSONFormatter struct{}
type YAMLFormatter struct{}
type TableFormatter struct {
    headers []string
    rowFunc func(any) []string
}
type TextFormatter struct{}

func NewFormatter(format string) (Formatter, error)
func NewTableFormatter(headers []string, rowFunc func(any) []string) *TableFormatter
func PrintResult(w io.Writer, format string, data any, tableHeaders []string, rowFunc func(any) []string) error
func IsColorEnabled(noColor bool, writer io.Writer) bool
func StartSpinner(w io.Writer, msg string) func()
```

- **JSONFormatter**: 2칸 들여쓰기 JSON 출력
- **YAMLFormatter**: YAML 형식 출력
- **TableFormatter**: `tabwriter` 기반 정렬된 테이블 출력 (외부 라이브러리 없이 구현)
- **TextFormatter**: Map은 `key: value` 형태, Slice는 항목별 한 줄로 출력
- **StartSpinner**: 비동기 로딩 애니메이션 (braille 문자 기반)

### NewRootCmd (`root.go`)

루트 커맨드로, 7개의 글로벌 플래그와 모든 서브커맨드를 등록한다.

```go
func NewRootCmd() *cobra.Command

// 빌드 시 ldflags 로 주입
var Version   = "dev"
var Commit    = "unknown"
var BuildDate = "unknown"
```

**글로벌 플래그 (7개)**:

| 플래그 | 타입 | 기본값 | 설명 |
|--------|------|--------|------|
| `--config` | string | `~/.xflow/config.yaml` | 설정 파일 경로 |
| `--server` | string | `http://localhost:8080` | xflowd 서버 URL |
| `--format` | string | `table` | 출력 형식 (json/yaml/table/text) |
| `--token` | string | `""` | 인증 토큰 |
| `--verbose` | bool | `false` | 상세 출력 모드 |
| `--quiet` | bool | `false` | 조용한 출력 모드 |
| `--no-color` | bool | `false` | 색상 출력 비활성화 |

## 커맨드 계층 구조

```
xflow
├── version                          버전 정보 출력
├── flow                             플로우 관리 (12 서브커맨드)
│   ├── list                         GET /api/v1/flows
│   ├── get <id>                     GET /api/v1/flows/:id
│   ├── create -f <file>             POST /api/v1/flows
│   ├── update <id> -f <file>        PUT /api/v1/flows/:id
│   ├── delete <id> [--yes]          DELETE /api/v1/flows/:id
│   ├── deploy <id>                  POST /api/v1/flows/:id/deploy
│   ├── start <id>                   POST /api/v1/flows/:id/start
│   ├── stop <id>                    POST /api/v1/flows/:id/stop
│   ├── restart <id>                 POST /api/v1/flows/:id/restart
│   ├── export <id> -o <file>        GET /api/v1/flows/:id → 파일
│   ├── import -f <file>             POST /api/v1/flows
│   └── status <id>                  GET /api/v1/flows/:id/status
├── agent                            에이전트 관리 (7 서브커맨드)
│   ├── list                         GET /api/v1/agents
│   ├── get <id>                     GET /api/v1/agents/:id
│   ├── create -f <file>             POST /api/v1/agents
│   ├── start <id>                   POST /api/v1/agents/:id/start
│   ├── stop <id>                    POST /api/v1/agents/:id/stop
│   ├── restart <id>                 POST /api/v1/agents/:id/restart
│   └── delete <id> [--yes]          DELETE /api/v1/agents/:id
├── config                           설정 관리 (6 서브커맨드)
│   ├── init                         ~/.xflow/config.yaml 생성
│   ├── get <key>                    설정 값 조회
│   ├── set <key> <value>            설정 값 변경
│   ├── server <url>                 서버 URL 설정 (단축)
│   ├── token <token>                인증 토큰 설정 (단축)
│   └── list                         모든 설정 출력 (토큰 마스킹)
├── node                             노드 타입 조회 (2 서브커맨드, 읽기 전용)
│   ├── list                         GET /api/v1/nodes
│   └── info <type>                  GET /api/v1/nodes/:type
├── plugin                           플러그인 관리 (4 서브커맨드)
│   ├── list                         GET /api/v1/plugins
│   ├── install <name|path>          POST /api/v1/plugins
│   ├── remove <name> [--yes]        DELETE /api/v1/plugins/:name
│   └── update <name>                PUT /api/v1/plugins/:name
└── status                           서버 상태 (3 서브커맨드, 읽기 전용)
    ├── (기본)                       GET /api/v1/status
    ├── logs <component>             GET /api/v1/logs?component=&lines=
    └── metrics                      GET /api/v1/metrics
```

## 모듈 구현 상태

| 모듈 | 파일 | 우선순위 | 상태 |
|------|------|---------|------|
| Root Command | root.go | P0 | 구현 완료 |
| HTTP Client | client.go | P0 | 구현 완료 |
| Error Types | errors.go | P0 | 구현 완료 |
| Output Formatter | output.go | P0 | 구현 완료 |
| Config Management | config.go | P0 | 구현 완료 |
| Flow Commands | flow.go | P0 | 구현 완료 |
| Agent Commands | agent.go | P0 | 구현 완료 |
| Node Commands | node.go | P1 | 구현 완료 |
| Plugin Commands | plugin.go | P1 | 구현 완료 |
| Status Commands | status.go | P1 | 구현 완료 |
| Binary Entrypoint | cmd/xflow/main.go | P0 | 구현 완료 |

## 파일 구조

```
internal/cli/
  root.go                # NewRootCmd(), 7 글로벌 플래그, PersistentPreRunE, version 서브커맨드
  client.go              # Client 구조체, Get/Post/Put/Delete/GetRaw/PostRaw/Ping, API 엔벨로프 파싱
  errors.go              # CLIError 타입, 7개 에러 팩토리, MapAPIError, FormatError
  output.go              # Formatter 인터페이스, JSON/YAML/Table/Text 포매터, PrintResult, StartSpinner

  config.go              # config init/get/set/server/token/list 서브커맨드 (6종)
  flow.go                # flow list/get/create/update/delete/deploy/start/stop/restart/export/import/status (12종)
  agent.go               # agent list/get/create/start/stop/restart/delete 서브커맨드 (7종)
  node.go                # node list/info 서브커맨드 (2종, 읽기 전용)
  plugin.go              # plugin list/install/remove/update 서브커맨드 (4종)
  status.go              # status/logs/metrics 서브커맨드 (3종, 읽기 전용)

  root_test.go           # 루트 커맨드 테스트 (플래그, 설정 우선순위)
  client_test.go         # HTTP 클라이언트 테스트 (httptest 모의 서버)
  errors_test.go         # CLIError, MapAPIError, FormatError 테스트
  output_test.go         # 포매터 테스트 (JSON/YAML/Table/Text)
  config_test.go         # config 서브커맨드 테스트
  flow_test.go           # flow 서브커맨드 테스트
  agent_test.go          # agent 서브커맨드 테스트
  node_test.go           # node 서브커맨드 테스트
  plugin_test.go         # plugin 서브커맨드 테스트
  status_test.go         # status 서브커맨드 테스트

cmd/xflow/
  main.go                # 바이너리 진입점, CLIError 핸들링 및 os.Exit
```

## 의존성

- **외부 라이브러리**:
  - `github.com/spf13/cobra` v1.10.2 - CLI 프레임워크 (커맨드, 플래그, 도움말 자동 생성)
  - `github.com/spf13/viper` v1.21.0 - 설정 파일 관리 (YAML 로드, 키-값 접근)
  - `gopkg.in/yaml.v3` - YAML 직렬화/역직렬화 (Flow export/import)
- **표준 라이브러리**: `net/http`, `encoding/json`, `io`, `os`, `fmt`, `text/tabwriter`, `reflect`, `sync`, `time`, `bufio`, `strings`, `path/filepath`, `runtime`, `sort`, `bytes`, `errors`
- **내부 의존성**: 없음 (독립 패키지)

## 사용 예시

### 초기 설정

```bash
# 설정 파일 초기화
xflow config init

# 서버 URL 설정
xflow config server http://my-xflowd:8080

# 인증 토큰 설정
xflow config token my-secret-token

# 설정 확인
xflow config list
```

### 플로우 관리

```bash
# 플로우 목록 조회
xflow flow list

# 플로우 상세 조회 (JSON 형식)
xflow flow get flow-1 --format json

# YAML 파일에서 플로우 생성
xflow flow create -f my-flow.yaml

# 플로우 배포 및 시작
xflow flow deploy flow-1
xflow flow start flow-1

# 플로우 내보내기/가져오기
xflow flow export flow-1 -o backup.yaml
xflow flow import -f backup.yaml

# 플로우 런타임 상태 조회
xflow flow status flow-1

# 플로우 삭제 (확인 건너뛰기)
xflow flow delete flow-1 --yes
```

### 에이전트 관리

```bash
# 에이전트 목록 조회
xflow agent list

# 에이전트 생성 및 시작
xflow agent create -f agent-config.json
xflow agent start agent-1

# 에이전트 재시작
xflow agent restart agent-1
```

### 서버 모니터링

```bash
# 서버 상태 확인
xflow status

# 엔진 로그 조회 (최근 100줄)
xflow status logs engine --lines 100

# 메트릭스 조회 (YAML 형식)
xflow status metrics --format yaml
```

### 플래그 우선순위

```bash
# 환경변수로 서버 URL 지정
XFLOW_SERVER=http://staging:8080 xflow flow list

# 커맨드라인 플래그 (최우선)
xflow flow list --server http://custom:8080 --token my-token

# 상세 출력 모드 (HTTP 요청/응답 로깅)
xflow flow list --verbose
```

## 테스트

```bash
# 전체 테스트 실행
go test ./internal/cli/...

# Race Detector 포함 테스트
go test -race ./internal/cli/...

# 커버리지 확인
go test -cover ./internal/cli/

# 상세 커버리지 리포트
go test -coverprofile=cover.out ./internal/cli/
go tool cover -html=cover.out
```

### 테스트 결과

- 테스트 수: 173개
- 커버리지: 90.3%
- Race Detector: 이상 없음 (go test -race)

## 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-CLI-001 | 본 SPEC | CLI 커맨드라인 도구 전체 명세 |
| SPEC-API-001 | 의존 | CLI 가 소비하는 REST API 엔드포인트 정의 |
