# SPEC-OBS-003: 노드별 로그 출력 라우팅 설정

## 메타데이터

| 항목 | 값 |
|------|-----|
| SPEC ID | SPEC-OBS-003 |
| 제목 | 노드별 로그 출력 라우팅 (Per-Node Log Output Configuration) |
| 상태 | Planned |
| 우선순위 | High |
| 생성일 | 2026-02-21 |
| 의존성 | SPEC-OBS-002 (서버 레벨 파일 기반 로그 출력) |
| 관련 파일 | `internal/engine/engine.go`, `internal/observe/stream.go`, `pkg/flow/flow.go`, `internal/engine/types.go` |

---

## 환경 (Environment)

### 현재 아키텍처

xflow 엔진은 이미 노드별 계층적 로그 레벨을 지원한다.

- `resolveNodeLogLevel` 함수가 노드 `config["log_level"]` -> 플로우 `config.log_level` -> 데몬 기본값 순서로 로그 레벨을 결정한다
- `Observer.Streams` (`StreamRouter`)가 컴포넌트별 `io.Writer` 라우팅을 지원한다 (`AddRoute`, `RemoveRoute`, `Routes`, `SetDefaultWriter`)
- `flowRuntime` 구조체가 배포된 Flow의 런타임 상태를 관리한다
- `NodeDef.Config`는 `map[string]any` 타입으로, 임의의 설정 키를 수용 가능하다
- `FlowConfig` 구조체에 `LogLevel string` 필드가 이미 존재한다

### 기술 스택

- Go 1.23+, `log/slog` 표준 라이브러리
- `internal/observe` 패키지: `StreamRouter`, `LevelManager`, `LoggerFactory`
- `pkg/flow` 패키지: `Flow` 인터페이스, `FlowConfig`, `NodeDef`
- `internal/engine` 패키지: `Engine`, `flowRuntime`, `resolveNodeLogLevel`

---

## 가정 (Assumptions)

1. SPEC-OBS-002 (서버 레벨 `observe.output` 설정)가 이미 완료되었거나, 이 SPEC과 독립적으로 구현 가능하다
2. 로그 파일 경로는 절대 경로만 허용하거나, 서버의 작업 디렉토리 기준 상대 경로를 지원한다
3. `log_output` 값은 `"stdout"`, `"/path/to/file"`, `"stdout+/path/to/file"` 세 가지 형식을 지원한다
4. 파일 핸들은 `DeployFlow` 시 열리고, `StopFlow`/`UndeployFlow` 시 닫힌다
5. 로그 파일은 append 모드(`os.O_APPEND|os.O_CREATE|os.O_WRONLY`)로 열린다
6. 파일 열기 실패 시 `DeployFlow`가 실패해야 하며, 무시하지 않는다
7. 기존 `resolveNodeLogLevel` 패턴을 `resolveNodeLogOutput`에도 동일하게 적용한다

---

## 요구사항 (Requirements)

### REQ-001: Flow 레벨 log_output 필드

시스템은 **항상** `FlowConfig` 구조체에 `LogOutput string` 필드를 포함해야 한다.

- `FlowConfig.LogOutput`은 플로우 내 모든 노드의 기본 로그 출력 대상으로 사용된다
- 빈 문자열(`""`)은 "상위 레벨(서버 기본값) 상속"을 의미한다
- JSON/YAML 직렬화 시 `log_output` 키로 매핑된다 (`json:"log_output,omitempty"`)

### REQ-002: 노드별 log_output 설정

**WHEN** 노드의 `config["log_output"]`에 유효한 값이 설정되어 있을 때, **THEN** 해당 노드의 로그는 지정된 출력 대상으로 라우팅되어야 한다.

```yaml
nodes:
  - name: "field-extractor"
    type: "transform"
    config:
      log_level: "debug"
      log_output: "/var/log/xflow/field-extractor.log"
```

### REQ-003: 계층적 로그 출력 우선순위

시스템은 **항상** 다음 우선순위로 로그 출력 대상을 결정해야 한다:

1. 노드 `config["log_output"]` (최우선)
2. 플로우 `config.log_output` (플로우 레벨 기본값)
3. 서버 `observe.output` 설정 (서버 레벨 기본값)
4. `stdout` (최종 기본값)

### REQ-004: 출력 형식 파싱

시스템은 **항상** 다음 세 가지 `log_output` 형식을 지원해야 한다:

| 형식 | 예시 | 동작 |
|------|------|------|
| `"stdout"` | `log_output: "stdout"` | 표준 출력만 사용 |
| `"/path/to/file"` | `log_output: "/var/log/xflow/node.log"` | 파일에만 출력 |
| `"stdout+/path/to/file"` | `log_output: "stdout+/var/log/xflow/node.log"` | 표준 출력과 파일 모두에 출력 |

### REQ-005: resolveNodeLogOutput 함수

**WHEN** `DeployFlow`가 호출될 때, **THEN** 시스템은 `resolveNodeLogOutput` 함수를 통해 각 노드의 로그 출력 대상을 결정해야 한다.

- 함수 시그니처: `resolveNodeLogOutput(nd flow.NodeDef, flowCfg flow.FlowConfig, serverDefault string) (LogOutputTarget, bool)`
- `resolveNodeLogLevel`과 동일한 계층적 결정 패턴을 따른다
- `LogOutputTarget`은 파싱된 출력 대상(stdout 여부, 파일 경로)을 캡슐화한다

### REQ-006: Engine 통합 - StreamRouter 연동

**WHEN** `DeployFlow`에서 노드별 로그 출력이 결정되었을 때, **THEN** 시스템은 다음을 수행해야 한다:

1. 파일 출력이 포함된 경우, 파일을 열어 `io.Writer`를 생성한다
2. `observer.Streams.AddRoute(component, fileWriter)`를 호출하여 라우팅을 등록한다
3. `stdout+file` 형식인 경우, 파일 라우트를 추가하되 기본 stdout 라우트도 유지한다
4. 파일 전용 출력인 경우, `observer.Streams.AddRoute(component, fileWriter)`만 등록한다

### REQ-007: 파일 핸들 생명주기 관리

시스템은 **항상** 로그 파일 핸들의 생명주기를 관리해야 한다:

- `flowRuntime` 구조체에 `closers []io.Closer` 필드를 추가하여 열린 파일 핸들을 추적한다
- `StopFlow` 또는 `UndeployFlow` 시 모든 `closers`를 순회하며 `Close()`를 호출한다
- 파일은 `os.O_APPEND|os.O_CREATE|os.O_WRONLY`, 권한 `0644`로 연다

### REQ-008: 디렉토리 자동 생성

**WHEN** 로그 파일 경로의 디렉토리가 존재하지 않을 때, **THEN** 시스템은 `os.MkdirAll(dir, 0755)`로 디렉토리를 자동 생성해야 한다.

### REQ-009: 파일 열기 실패 처리

**IF** 로그 파일 열기에 실패하면, **THEN** `DeployFlow`는 에러를 반환하고 플로우 배포를 중단해야 한다.

- 이미 생성된 파일 핸들은 모두 정리(Close)해야 한다
- 에러 메시지에 실패한 파일 경로와 원인을 포함해야 한다

### REQ-010: log_output 값 유효성 검증

**WHEN** `log_output` 값이 설정되었을 때, **THEN** 시스템은 다음을 검증해야 한다:

- 빈 문자열은 "상위 레벨 상속"으로 유효하다
- `"stdout"`은 유효하다
- 파일 경로는 빈 문자열이 아닌 유효한 경로여야 한다
- `"stdout+"` 뒤에 빈 경로가 오면 유효하지 않다
- 지원하지 않는 형식은 에러를 반환한다

### REQ-011: Flow YAML 직렬화 호환성

시스템은 **항상** `FlowConfig.LogOutput` 필드를 JSON/YAML 직렬화에 올바르게 포함해야 한다.

- YAML 키: `log_output`
- `omitempty` 태그 적용: 빈 문자열이면 직렬화에서 제외한다
- 기존 Flow YAML 파일과의 역호환성을 유지한다 (필드 부재 시 빈 문자열)

### REQ-012: BaseNode 구조체 미변경

시스템은 `BaseNode` 구조체를 변경**하지 않아야 한다**.

- 로그 라우팅은 `Engine` 레벨에서 `StreamRouter`를 통해 처리한다
- 노드는 자신의 로그 출력 대상을 알 필요가 없다
- 기존 `node.WithLogger(nodeLogger)` 패턴은 그대로 유지한다

---

## 명세 (Specifications)

### SPEC-001: LogOutputTarget 타입 정의

```go
// internal/engine/log_output.go

// LogOutputTarget 은 파싱된 로그 출력 대상을 나타낸다.
type LogOutputTarget struct {
    UseStdout bool   // stdout으로 출력 여부
    FilePath  string // 파일 경로 (빈 문자열이면 파일 미사용)
}
```

### SPEC-002: ParseLogOutput 함수

```go
// ParseLogOutput 은 log_output 문자열을 LogOutputTarget 으로 파싱한다.
// 형식: "stdout", "/path/to/file", "stdout+/path/to/file"
func ParseLogOutput(raw string) (LogOutputTarget, error)
```

### SPEC-003: resolveNodeLogOutput 함수

```go
// resolveNodeLogOutput 은 노드의 로그 출력 대상을 계층적으로 결정한다.
// 우선순위: 노드 config["log_output"] -> 플로우 config.log_output -> 서버 기본값
// 명시적 설정이 있으면 (target, true)를, 기본값 사용이면 (_, false)를 반환한다.
func resolveNodeLogOutput(nd flow.NodeDef, flowCfg flow.FlowConfig, serverDefault string) (LogOutputTarget, bool)
```

### SPEC-004: FlowConfig 확장

```go
// pkg/flow/flow.go
type FlowConfig struct {
    TrackHistory   bool        `json:"track_history" yaml:"track_history"`
    MaxHistorySize int         `json:"max_history_size" yaml:"max_history_size"`
    ErrorHandling  ErrorPolicy `json:"error_handling" yaml:"error_handling"`
    LogLevel       string      `json:"log_level,omitempty" yaml:"log_level,omitempty"`
    LogOutput      string      `json:"log_output,omitempty" yaml:"log_output,omitempty"` // NEW
}
```

### SPEC-005: flowRuntime 확장

```go
// internal/engine/types.go
type flowRuntime struct {
    flow         flow.Flow
    nodes        map[string]node.Node
    wires        []*RuntimeWire
    cancel       func()
    wg           sync.WaitGroup
    paused       atomic.Bool
    messageCount atomic.Int64
    errorCount   atomic.Int64
    droppedCount atomic.Int64
    startedAt    time.Time
    closers      []io.Closer  // NEW: 로그 파일 핸들 추적
}
```

### SPEC-006: DeployFlow 통합

`DeployFlow` 함수의 노드 생성 루프(engine.go:83-97)에 로그 출력 라우팅 로직을 추가한다:

```go
if e.observer != nil {
    component := fmt.Sprintf("node.%s", nd.Name)
    nodeLogger := e.observer.Loggers.NewLogger(component)

    // 기존: 계층적 로그 레벨 결정
    if lvl, ok := resolveNodeLogLevel(nd, f.Config(), e.observer.Levels.DefaultLevel()); ok {
        e.observer.Levels.SetLevel(component, lvl)
    }

    // NEW: 계층적 로그 출력 대상 결정
    if target, ok := resolveNodeLogOutput(nd, f.Config(), serverLogOutput); ok {
        if target.FilePath != "" {
            writer, err := openLogFile(target.FilePath)
            if err != nil {
                // 이미 열린 파일 정리 후 에러 반환
                closeAll(closers)
                return fmt.Errorf("engine: failed to open log file for node %q: %w", nd.Name, err)
            }
            closers = append(closers, writer)
            e.observer.Streams.AddRoute(component, writer)
        }
        if !target.UseStdout && target.FilePath != "" {
            // 파일 전용: 기본 stdout 라우트 사용 안 함
            // StreamRouter의 Routes()가 등록된 writer만 반환하므로
            // 기본 defaultWriter는 사용되지 않는다
        }
    }

    nodeOpts = append(nodeOpts, node.WithLogger(nodeLogger))
}
```

### SPEC-007: StopFlow/UndeployFlow 정리

`StopFlow` 및 `UndeployFlow`에서 `flowRuntime.closers`를 정리한다:

```go
// StopFlow 종료 시
for _, c := range rt.closers {
    _ = c.Close()
}
rt.closers = nil
```

### SPEC-008: YAML 예제

```yaml
# Flow 레벨 log_output
config:
  track_history: true
  log_level: "info"
  log_output: "/var/log/xflow/mqtt-metrics.log"

nodes:
  - name: "field-extractor"
    type: "transform"
    config:
      log_level: "debug"
      log_output: "/var/log/xflow/field-extractor.log"
      expression: |
        { device_id: $.payload.deviceInfo.devEui }

  - name: "metrics-aggregator"
    type: "aggregate"
    config:
      log_level: "debug"
      log_output: "stdout+/var/log/xflow/aggregator.log"
      window_type: "count"
      window_size: 10

  - name: "data-filter"
    type: "filter"
    config:
      log_level: "info"
      # log_output 미설정 -> 플로우 레벨 log_output 상속
      condition: "$.payload.temperature > 30"
```

---

## 추적성 (Traceability)

| 요구사항 | 구현 대상 | 테스트 대상 |
|----------|----------|------------|
| REQ-001 | `pkg/flow/flow.go` FlowConfig | `pkg/flow/flow_test.go` |
| REQ-002 | `internal/engine/engine.go` DeployFlow | `internal/engine/engine_test.go` |
| REQ-003 | `internal/engine/log_output.go` resolveNodeLogOutput | `internal/engine/log_output_test.go` |
| REQ-004 | `internal/engine/log_output.go` ParseLogOutput | `internal/engine/log_output_test.go` |
| REQ-005 | `internal/engine/log_output.go` resolveNodeLogOutput | `internal/engine/log_output_test.go` |
| REQ-006 | `internal/engine/engine.go` DeployFlow | `internal/engine/engine_test.go` |
| REQ-007 | `internal/engine/types.go` flowRuntime | `internal/engine/engine_test.go` |
| REQ-008 | `internal/engine/log_output.go` openLogFile | `internal/engine/log_output_test.go` |
| REQ-009 | `internal/engine/engine.go` DeployFlow | `internal/engine/engine_test.go` |
| REQ-010 | `internal/engine/log_output.go` ParseLogOutput | `internal/engine/log_output_test.go` |
| REQ-011 | `pkg/flow/serialize.go`, `pkg/flow/flow.go` | `pkg/flow/serialize_test.go` |
| REQ-012 | (미변경 확인) | 기존 테스트 유지 |
