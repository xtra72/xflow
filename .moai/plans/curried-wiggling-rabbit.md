# Plan: console-logger 출력 설정 + output 노드 생성

## Context

현재 `console-logger` 에이전트는 항상 stdout으로만 출력한다. 파일 출력과 롤링 파일 기능을 추가하여 운영 환경에서 로그를 파일로 관리할 수 있게 한다.

또한 플로우에서 메시지를 포맷팅하여 출력하는 `output` 노드를 새로 만든다. prefix와 Go text/template 기반 메시지 구성을 지원하며, 지정하지 않으면 전체 페이로드를 메시지로 출력한다.

---

## Milestone 1: RollingWriter 구현

**목표**: 크기/기간 기반 롤링 파일 writer

**파일**: [rollingwriter.go](internal/io/rollingwriter.go) (NEW)

### 구조체 설계

```
RollingWriterConfig:
  FilePath   string  // 필수
  MaxSize    int64   // 바이트, 기본: 10MB
  MaxAge     int     // 일, 기본: 0 (무제한)
  MaxBackups int     // 기본: 0 (무제한)
  Compress   bool    // 기본: false

RollingWriter:
  config     RollingWriterConfig
  file       *os.File
  size       int64
  mu         sync.Mutex
```

### 핵심 로직

- `Write(p []byte)`: 현재 파일에 쓰기. `size + len(p) > MaxSize`이면 로테이션
- `rotate()`: 현재 파일 닫기 → `{name}-{timestamp}.log`로 이름 변경 → 새 파일 열기
- `cleanup()`: MaxBackups 초과 시 오래된 백업 삭제, MaxAge 초과 시 오래된 백업 삭제
- `compress()`: Compress=true일 때 백업 파일을 gzip 압축 (비동기 goroutine)
- `Close()`: 파일 닫기, io.WriteCloser 구현

### 설계 결정

- **외부 의존성 없음**: lumberjack 등 쓰지 않고 직접 구현 (의존성 최소화)
- **io.WriteCloser 구현**: slog.Handler의 writer로 직접 사용 가능
- **스레드 안전**: sync.Mutex로 동시 쓰기 보호
- **백업 파일 네이밍**: `{basename}-{20060102T150405}.log` (시간 정렬 가능)

---

## Milestone 2: console-logger 에이전트 확장

**목표**: 출력 대상(stdout/stderr/file), 포맷(text/json), 롤링 설정 추가

**파일**: [console_logger.go](internal/agent/system/console_logger.go) (EDIT)

### Config 확장

```
ConsoleLoggerConfig:
  Prefix     string     // 기존 유지
  Level      slog.Level // 기존 유지
  Output     string     // "stdout" (기본), "stderr", 파일 경로
  Format     string     // "text" (기본), "json"
  MaxSize    int64      // 롤링: 최대 크기 (bytes), 기본 10MB
  MaxAge     int        // 롤링: 보관 기간 (일), 기본 0
  MaxBackups int        // 롤링: 최대 백업 수, 기본 0
  Compress   bool       // 롤링: gzip 압축, 기본 false
```

### 구현 변경

- `parseConsoleLoggerConfig()`: output, format, max_size, max_age, max_backups, compress 파싱
- `NewConsoleLoggerAgent()`: `resolveWriter()` 호출로 writer 결정, `createLogger()` 로 slog.Logger 생성
- `resolveWriter(config) (io.Writer, io.Closer)`:
  - "stdout" 또는 "" → `os.Stdout, nil`
  - "stderr" → `os.Stderr, nil`
  - 파일 경로 → `NewRollingWriter(config)` (MaxSize>0일 때) 또는 일반 openLogFile
- `createLogger(writer, config) *slog.Logger`:
  - format=="json" → `slog.NewJSONHandler(writer, opts)`
  - format=="text" (기본) → `slog.NewTextHandler(writer, opts)`
- `Stop()`: writer가 io.Closer이면 Close() 호출
- `Configure()`: 설정 변경 시 기존 closer 닫고 writer/logger 재생성

### 필드 추가 (구조체)

```go
type ConsoleLoggerAgent struct {
  ...      // 기존 필드
  writer io.Writer   // 출력 대상
  closer io.Closer   // 파일인 경우 Close용
}
```

### 기존 코드 재사용

- `agent.ResolveLogger()`: 외부 주입 logger가 있으면 우선 사용 (기존 패턴 유지)
- `config/output.go`의 `openLogFile()`: 롤링 미사용 시 기본 파일 열기에 참조

---

## Milestone 3: output 노드 구현

**목표**: 메시지를 포맷팅하여 slog로 출력하는 ProcessNode (pass-through)

**파일**: [output.go](internal/node/output.go) (NEW)

### 구조체 설계

```
OutputNode:
  *BaseNode
  prefix   string             // 출력 접두어
  tmpl     *template.Template // nil이면 전체 페이로드 JSON 출력
  mu       sync.RWMutex
```

### Config 옵션

| 키 | 타입 | 기본값 | 설명 |
|---|---|---|---|
| prefix | string | "[output]" | 로그 접두어 |
| template | string | "" | Go text/template 문자열. 비어있으면 전체 페이로드를 JSON으로 출력 |

### 핵심 로직

- `Configure(config)`: prefix 설정, template이 있으면 `template.New().Parse()` 로 컴파일
- `Process(ctx, msg)`:
  1. template이 nil이면 `payload.ToJSON()`으로 전체 페이로드를 문자열화
  2. template이 있으면 `tmpl.Execute(buf, payload.ToMap())`으로 렌더링
  3. `logger.Info(formatted, "prefix", prefix)` 로 출력
  4. 원본 메시지를 그대로 반환 (pass-through)
- `Init()`: Initializing → Running
- `Shutdown()`: Running → Stopping

### 설계 결정

- **slog만 사용**: 노드는 BaseNode.Logger()를 사용. 출력 대상 설정 없음 (에이전트가 담당)
- **error 포트 없음**: 템플릿 실행 실패 시 로그 경고 후 원본 메시지 그대로 통과
- **Go text/template**: 표준 라이브러리, 외부 의존성 없음
- **pass-through**: debug 노드와 동일하게 원본 메시지를 변경 없이 통과

### 참조 패턴

- [debug.go](internal/node/debug.go): 같은 카테고리의 ProcessNode, BaseNode 임베딩, pass-through
- [registry.go](internal/node/registry.go) L79: debug 노드 등록 패턴

### 템플릿 예시

```
# 지정하지 않으면 전체 페이로드 JSON
{"temperature":25.5,"humidity":60}

# template: "온도={{.temperature}} 습도={{.humidity}}"
온도=25.5 습도=60

# template: "디바이스 {{.device_id}}: {{.status}}"
디바이스 A001: normal
```

---

## Milestone 4: 노드 레지스트리 등록

**파일**: [registry.go](internal/node/registry.go) (EDIT)

`registerBuiltins()`에 추가:
```
{"output", NewOutputNode, "debug", "메시지를 포맷팅하여 출력"}
```

---

## Milestone 5: 프론트엔드 스키마

### 5.1 agentSchemas.ts

**파일**: [agentSchemas.ts](web/src/config/agentSchemas.ts) (EDIT)

CONSOLE_LOGGER_FIELDS에 추가:
- output: select (stdout, stderr, file), default: stdout
- format: select (text, json), default: text
- file_path: string, description: "파일 경로 (output이 file일 때)"
- max_size: number, label: "최대 크기 (MB)", default: 10
- max_age: number, label: "보관 기간 (일)", default: 0
- max_backups: number, label: "최대 백업 수", default: 0
- compress: boolean, label: "gzip 압축", default: false

### 5.2 nodeSchemas.ts

**파일**: [nodeSchemas.ts](web/src/config/nodeSchemas.ts) (EDIT)

output 노드 스키마:
- prefix: string, default: "[output]"
- template: string (textarea), description: "Go text/template 형식"
- defaultPorts: [in, out]

### 5.3 nodeTypeMeta.ts

**파일**: [nodeTypeMeta.ts](web/src/pages/nodes/nodeTypeMeta.ts) (EDIT)

output 노드 메타데이터 추가.

---

## Milestone 6: 테스트

### 6.1 RollingWriter 테스트

**파일**: [rollingwriter_test.go](internal/io/rollingwriter_test.go) (NEW)

- TestRollingWriter_Basic: 기본 쓰기
- TestRollingWriter_Rotation: MaxSize 초과 시 로테이션
- TestRollingWriter_MaxBackups: 백업 수 제한
- TestRollingWriter_MaxAge: 오래된 백업 삭제
- TestRollingWriter_Compress: gzip 압축
- TestRollingWriter_Close: 정상 종료

### 6.2 console-logger 테스트

**파일**: [console_logger_test.go](internal/agent/system/console_logger_test.go) (EDIT)

- TestConsoleLoggerAgent_OutputStderr: stderr 출력
- TestConsoleLoggerAgent_OutputFile: 파일 출력
- TestConsoleLoggerAgent_FormatJSON: JSON 포맷
- TestConsoleLoggerAgent_RollingFile: 롤링 파일 설정
- TestConsoleLoggerAgent_StopClosesFile: Stop 시 파일 닫기
- TestConsoleLoggerAgent_ConfigureNewOutput: 설정 변경 시 writer 재생성

### 6.3 output 노드 테스트

**파일**: [output_test.go](internal/node/output_test.go) (NEW)

- TestNewOutputNode_Basic
- TestOutputNode_Configure_Defaults
- TestOutputNode_Configure_WithTemplate
- TestOutputNode_Process_NoTemplate: 전체 페이로드 JSON
- TestOutputNode_Process_WithTemplate: 템플릿 렌더링
- TestOutputNode_Process_InvalidTemplate: 잘못된 템플릿 시 경고 후 통과
- TestOutputNode_Process_PassThrough: 원본 메시지 불변 확인
- TestOutputNode_Shutdown

---

## Files

| File | Action | Description |
|------|--------|-------------|
| [rollingwriter.go](internal/io/rollingwriter.go) | Create | 롤링 파일 writer |
| [rollingwriter_test.go](internal/io/rollingwriter_test.go) | Create | 롤링 writer 테스트 |
| [console_logger.go](internal/agent/system/console_logger.go) | Edit | 출력 대상, 포맷, 롤링 설정 |
| [console_logger_test.go](internal/agent/system/console_logger_test.go) | Edit | 신규 설정 테스트 추가 |
| [output.go](internal/node/output.go) | Create | output 노드 |
| [output_test.go](internal/node/output_test.go) | Create | output 노드 테스트 |
| [registry.go](internal/node/registry.go) | Edit | output 노드 등록 |
| [registry_test.go](internal/node/registry_test.go) | Edit | 기대값 업데이트 |
| [agentSchemas.ts](web/src/config/agentSchemas.ts) | Edit | console-logger 스키마 확장 |
| [nodeSchemas.ts](web/src/config/nodeSchemas.ts) | Edit | output 노드 스키마 |
| [nodeTypeMeta.ts](web/src/pages/nodes/nodeTypeMeta.ts) | Edit | output 노드 메타 |

## Verification

- [ ] `go build ./cmd/xflowd` — 백엔드 빌드
- [ ] `go test ./internal/io/...` — RollingWriter 테스트
- [ ] `go test ./internal/agent/system/...` — console-logger 테스트
- [ ] `go test ./internal/node/...` — output 노드 + 기존 테스트
- [ ] `go test -race ./internal/...` — 동시성 안전
- [ ] `npx tsc --noEmit` (web/) — 프론트엔드 타입 체크
