# cli - xflow CLI 커맨드라인 도구

`internal/cli` 패키지는 xflow 워크플로우 오케스트레이션 플랫폼의 커맨드라인 인터페이스이다. xflowd 서버와 REST API 로 통신하여 플로우, 에이전트, 플러그인, 노드 등의 리소스를 관리하며, 다중 출력 형식(json/yaml/table/text)을 지원한다.

**SPEC**: SPEC-CLI-001, SPEC-CLI-002

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
         |          |         |        |       |      |       |
    +----+---+ +----+--+ +---+---+ +--+--+ +-+----+ ++------+++-----------+
    | flow   | | agent | | config| | node| |plugin| |status || interactive|
    | 12 sub | | 7 sub | | 6 sub | | 2   | | 4   | | 3 sub || REPL+Wizard|
    +--------+ +-------+ +-------+ +-----+ +-----+ +-------++-----------+
         |                                      |          |
    +----+----+                           +-----+-----+    |
    | Client  | <-- HTTP/JSON ----------> | xflowd    |    |
    | (REST)  |    GET/POST/PUT/DELETE    | API 서버  |    |
    +----+----+                           +-----------+    |
         |                                            +----+------+
    +----+----+                                       |autocomplete|
    | output  |  json | yaml | table | text           | + wizard   |
    +---------+                                       +-----------+
```

**핵심 설계 원칙**:

1. **클로저 기반 클라이언트 공유**: `var client *Client`를 `NewRootCmd` 내에서 선언하고, `**Client`로 서브커맨드에 전달하여 `PersistentPreRunE`에서 초기화된 클라이언트를 공유
2. **의존성 주입 테스트**: `confirmFn func(string, io.Reader) bool`을 주입하여 대화형 확인 프롬프트를 테스트에서 제어 가능
3. **우선순위 기반 설정 해석**: 서버 URL 및 토큰을 `--flag` > `환경변수` > `설정 파일` > `기본값` 순서로 결정
4. **httptest 기반 테스트**: 모든 API 호출을 `net/http/httptest` 모의 서버로 테스트하여 외부 의존성 제거
5. **다중 출력 형식**: 모든 list/get 커맨드가 `--format json|yaml|table|text`를 지원
6. **REPL 의존성 주입**: `readlineFn`, `pingFn`, `askFn`, `confirmFn` 등을 주입하여 대화형 컴포넌트를 테스트에서 완전 제어 가능
7. **TTL 캐시 기반 자동완성**: API 리소스 자동완성 결과를 30초 TTL 캐시로 관리하여 반복 호출 최소화

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

### InteractiveSession (`interactive.go`)

REPL(Read-Eval-Print Loop) 대화형 세션을 관리하는 핵심 타입이다.

```go
type InteractiveSession struct {
    rootCmd    *cobra.Command
    client     **Client
    writer     io.Writer
    histFile   string
    readlineFn readlineInterface
    pingFn     func() error // 서버 연결 확인 함수 (테스트 주입용)
    history    []string     // 인메모리 히스토리
}

func NewInteractiveSession(rootCmd *cobra.Command, client **Client, writer io.Writer) *InteractiveSession
func (s *InteractiveSession) Start() error
func (s *InteractiveSession) Stop()
```

- **readline 기반 REPL**: `github.com/chzyer/readline`을 사용한 줄 편집, 히스토리 탐색, Ctrl+R 역방향 검색
- **컨텍스트 인식 프롬프트**: 서버 연결 상태에 따라 `xflow [localhost:8080]> ` 또는 `xflow [disconnected]> ` 표시
- **Cobra 명령어 트리 실행**: 입력된 명령어를 Cobra 명령어 트리에 위임하여 셸 실행과 동일한 결과 보장
- **시그널 처리**: Ctrl+C(SIGINT)를 현재 입력 취소로 처리하여 세션이 종료되지 않도록 관리
- **유사 명령어 제안**: Levenshtein 거리 기반으로 잘못된 명령어 입력 시 가장 유사한 명령어를 제안
- **히스토리 영속화**: `~/.xflow/history` 파일에 명령어 히스토리를 저장하여 세션 간 유지
- **특수 명령어**: `help`/`?`(도움말), `exit`/`quit`(종료), `clear`(화면 지우기), `history`(히스토리 표시), `wizard`(Wizard 모드 진입)

### AutoCompleter (`autocomplete.go`)

탭 자동완성 엔진으로, 명령어/서브커맨드/플래그 및 API 리소스 이름의 동적 자동완성을 제공한다.

```go
type AutoCompleter struct {
    rootCmd  *cobra.Command
    client   **Client
    cache    map[string]*cacheEntry
    cacheTTL time.Duration
    mu       sync.RWMutex
}

func NewAutoCompleter(rootCmd *cobra.Command, client **Client) *AutoCompleter
```

- **명령어 자동완성**: Cobra 명령어 트리를 순회하여 현재 입력 컨텍스트에 맞는 명령어/서브커맨드 후보 제안
- **플래그 자동완성**: `--` 접두사 입력 시 해당 명령어의 사용 가능한 플래그 목록 제안
- **API 리소스 자동완성**: `flow get <Tab>` 입력 시 API를 쿼리하여 플로우 이름, Agent ID 등 리소스 이름 제안
- **TTL 캐시**: 자동완성 결과를 30초 TTL로 캐싱하여 반복적인 API 호출 최소화
- **readline.AutoCompleter 인터페이스**: `readline` 라이브러리의 `AutoCompleter` 인터페이스를 구현

### WizardRunner (`wizard.go`)

survey/v2 기반 단계별 가이드 프롬프트를 제공하여 초보 사용자가 복잡한 명령어를 쉽게 실행할 수 있도록 안내한다.

```go
type WizardRunner struct {
    rootCmd   *cobra.Command
    client    **Client
    writer    io.Writer
    askFn     func(questions []*WizardQuestion) (map[string]string, error)
    confirmFn func(message string) (bool, error)
}

func NewWizardRunner(rootCmd *cobra.Command, client **Client, writer io.Writer) *WizardRunner
func (w *WizardRunner) Run(commandPath string) error
```

- **단계별 파라미터 수집**: 명령어의 필수 인자와 선택 플래그에 대해 순차적으로 프롬프트 표시
- **기본값 제안**: 각 파라미터에 대해 기본값을 제안하고 입력 유효성 검사 수행
- **실행 전 요약 및 확인**: 수집된 모든 파라미터를 테이블 형태로 요약 표시 후 실행 확인 요청
- **Cobra 명령어 트리 연동**: 입력된 명령어 경로를 Cobra 트리에서 찾아 해당 명령어의 인자/플래그 정보를 추출
- **의존성 주입 테스트**: `askFn`과 `confirmFn`을 주입하여 대화형 프롬프트를 테스트에서 완전 제어 가능

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
├── status                           서버 상태 (3 서브커맨드, 읽기 전용)
│   ├── (기본)                       GET /api/v1/status
│   ├── logs <component>             GET /api/v1/logs?component=&lines=
│   └── metrics                      GET /api/v1/metrics
└── interactive (별칭: i, repl)      대화형 REPL 모드 (SPEC-CLI-002)
    ├── --wizard <command>           Wizard 모드로 직접 진입
    └── (REPL 내 특수 명령어)
        ├── help / ?                 사용 가능한 명령어 목록 표시
        ├── exit / quit              REPL 세션 종료
        ├── clear                    화면 지우기
        ├── history                  최근 명령어 히스토리 표시
        └── wizard <command>         지정된 명령어의 Wizard 모드 진입
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
| REPL Core | interactive.go | P0 | 구현 완료 |
| Tab Completion | autocomplete.go | P1 | 구현 완료 |
| Wizard Mode | wizard.go | P2 | 구현 완료 |
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
  node.go                # node type 서브커맨드 (목록/상세 조회, 읽기 전용)
  plugin.go              # plugin list/install/remove/update 서브커맨드 (4종)
  status.go              # status/logs/metrics 서브커맨드 (3종, 읽기 전용)

  interactive.go         # REPL 대화형 세션 (readline, 명령어 실행, 시그널 처리, 유사 명령어 제안)
  autocomplete.go        # 탭 자동완성 엔진 (명령어/서브커맨드/플래그/API 리소스, TTL 캐시)
  wizard.go              # Wizard 단계별 프롬프트 (survey/v2, 파라미터 수집, 요약/확인/실행)

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
  interactive_test.go    # REPL 세션 테스트 (25개 테스트 함수, 70+ 서브테스트)
  autocomplete_test.go   # 자동완성 테스트 (16개 테스트 함수)
  wizard_test.go         # Wizard 테스트 (18개 테스트 함수)

cmd/xflow/
  main.go                # 바이너리 진입점, CLIError 핸들링 및 os.Exit
```

## 의존성

- **외부 라이브러리**:
  - `github.com/spf13/cobra` v1.10.2 - CLI 프레임워크 (커맨드, 플래그, 도움말 자동 생성)
  - `github.com/spf13/viper` v1.21.0 - 설정 파일 관리 (YAML 로드, 키-값 접근)
  - `gopkg.in/yaml.v3` - YAML 직렬화/역직렬화 (Flow export/import)
  - `github.com/chzyer/readline` v1.5.1 - REPL 줄 편집, 히스토리, Ctrl+R 역방향 검색
  - `github.com/AlecAivazis/survey/v2` v2.3.7 - Wizard 단계별 프롬프트 (survey/v2)
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

### 대화형 모드 (Interactive Mode)

```bash
# REPL 대화형 모드 진입
xflow interactive

# 별칭으로 진입
xflow i
xflow repl

# REPL 세션 내 명령어 실행 (xflow 접두사 생략 가능)
xflow [localhost:8080]> flow list
xflow [localhost:8080]> agent get agent-1 --format json
xflow [localhost:8080]> status

# 탭 자동완성 사용
xflow [localhost:8080]> flow <Tab>        # 서브커맨드 목록 표시
xflow [localhost:8080]> flow get <Tab>    # API에서 플로우 이름 자동완성
xflow [localhost:8080]> flow list --<Tab> # 사용 가능한 플래그 표시

# 히스토리 탐색
xflow [localhost:8080]> <Up/Down>         # 이전/다음 명령어 탐색
xflow [localhost:8080]> <Ctrl+R>          # 역방향 히스토리 검색

# 특수 명령어
xflow [localhost:8080]> help              # 사용 가능한 명령어 목록
xflow [localhost:8080]> history           # 최근 명령어 히스토리
xflow [localhost:8080]> clear             # 화면 지우기
xflow [localhost:8080]> exit              # 세션 종료
```

### Wizard 모드

```bash
# 셸에서 직접 Wizard 모드 진입
xflow interactive --wizard flow deploy

# REPL 내에서 Wizard 모드 진입
xflow [localhost:8080]> wizard flow deploy

# Wizard 가 단계별로 파라미터를 물어봄:
#   [1/2] Flow ID: my-flow-1
#   [2/2] --format (기본값: table): json
#
# 실행 요약:
#   +----------+----------+
#   | 파라미터  | 값       |
#   +----------+----------+
#   | Flow ID  | my-flow-1|
#   | --format | json     |
#   +----------+----------+
#   실행하시겠습니까? (Y/n): Y
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

- 테스트 수: 230개 (SPEC-CLI-001: 173개, SPEC-CLI-002: 57개)
- 커버리지: 91.7%
- Race Detector: 이상 없음 (go test -race)

## 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-CLI-001 | 기반 SPEC | CLI 커맨드라인 도구 전체 명세 (root, client, output, errors, 6개 커맨드 그룹) |
| SPEC-CLI-002 | 확장 SPEC | Interactive Mode - REPL + Wizard 하이브리드 CLI (interactive, autocomplete, wizard) |
| SPEC-API-001 | 의존 | CLI 가 소비하는 REST API 엔드포인트 정의 (자동완성용 리소스 조회 포함) |
