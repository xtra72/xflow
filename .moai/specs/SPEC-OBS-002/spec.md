---
id: SPEC-OBS-002
version: "1.0.0"
status: planned
created: "2026-02-21"
updated: "2026-02-21"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-21 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-OBS-002: File-Based Log Output Configuration - 파일 기반 로그 출력 설정

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 `internal/config` 패키지에 `observe.format`과 `observe.output` 설정 필드를 추가하여, 로그 출력 대상(stdout, 파일, 또는 둘 다)과 로그 포맷(json, text)을 YAML 설정 파일로 제어할 수 있게 한다.

현재 Observer는 기본적으로 `os.Stdout`에 JSON 포맷으로 출력하며, `WithObserverWriter(io.Writer)`와 `WithObserverFormat(format string)` 옵션을 이미 지원한다. 그러나 이 옵션들은 설정 파일에서 읽어오는 경로가 없어 코드 레벨에서만 변경 가능하다. 본 SPEC은 설정 파일 기반 제어 경로를 추가한다.

**핵심 설계 원칙: 기존 observe 패키지의 옵션 시스템을 활용하여 설정 파일과 연결한다.**

본 SPEC은 다음을 다룬다:
- `ObserveConfig` 구조체에 `Format`, `Output` 필드 추가
- `defaults.go`에 기본값 등록 (`observe.format`, `observe.output`)
- `config.go`의 `Observe()` 접근자에 새 필드 반영
- `validate.go`에 `validateLogFormat`, `validateLogOutput` 검증 함수 추가
- `cmd/xflowd/main.go`에서 설정값 기반 Observer 초기화 로직 추가
- CLI `--log-output` 플래그 추가 (기존 `--log-level` 패턴과 동일)
- 파일 핸들 생명주기 관리 (열기, 닫기)

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/config/` (types.go, defaults.go, config.go, validate.go), `cmd/xflowd/main.go`
- **의존성**: 표준 라이브러리 (`os`, `io`, `fmt`, `path/filepath`, `strings`) + `github.com/spf13/viper` + `internal/observe`
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **Tier**: internal (비공개 패키지, 외부 임포트 불가)

### 1.3 설계 원칙

- **설정 우선**: YAML 설정 파일 값이 기본값을 오버라이드하고, CLI 플래그가 설정 파일 값을 오버라이드
- **최소 변경**: 기존 Observer 옵션 시스템(`WithObserverWriter`, `WithObserverFormat`)을 그대로 활용하며, config 레이어에서 파싱만 추가
- **안전한 기본값**: format 기본값 "json", output 기본값 "stdout" (기존 동작 유지)
- **파일 핸들 정리**: 파일 출력 설정 시 서버 종료 시점에 파일 핸들을 반드시 닫아야 함
- **불변 설정**: `observe.output`과 `observe.format`은 런타임 변경 불가 (파일 핸들 안전성)

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- `ObserveConfig` 구조체 확장 (Format, Output 필드)
- Viper 기본값 등록 (`observe.format`, `observe.output`)
- `Observe()` 접근자 확장 (Format, Output 읽기)
- 검증 함수 추가 (`validateLogFormat`, `validateLogOutput`)
- `cmd/xflowd/main.go` Observer 초기화 로직 확장
- CLI `--log-output` 플래그 추가
- 출력 대상 파싱 로직 ("stdout", 파일 경로, "stdout+파일경로")
- 파일 핸들 생명주기 관리

**OUT OF SCOPE (별도 SPEC)**:
- `internal/observe` 패키지 자체 변경 (이미 WithObserverWriter, WithObserverFormat 지원)
- StreamRouter의 컴포넌트별 라우팅 설정
- 로그 로테이션 (lumberjack 등 외부 라이브러리 통합)
- 원격 로그 전송 (syslog, fluentd 등)
- `observe.output`, `observe.format`의 런타임 변경 지원

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-OBS-001 | 기반 | Observer 패키지 - WithObserverWriter, WithObserverFormat 옵션 제공 |
| SPEC-LOG-001 | 소비자 | LoggerAgent가 Observer를 래핑 - 본 SPEC의 설정으로 초기화된 Observer를 사용 |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: `internal/observe` 패키지의 `WithObserverWriter(io.Writer)`와 `WithObserverFormat(format string)` 옵션이 이미 구현되어 있으며, 정상 동작한다
- A2: `observe.StreamRouter`의 `defaultWriter`는 `os.Stdout`이며, `SetDefaultWriter(io.Writer)`로 변경 가능하다
- A3: Viper의 `SetDefault`, `GetString` API로 새 필드를 추가할 수 있으며, 기존 설정 파일과 하위 호환성을 유지한다
- A4: 설정 파일이 없는 경우 기본값(`format: "json"`, `output: "stdout"`)이 적용되어 기존 동작이 변경되지 않는다
- A5: 파일 경로에 상대 경로가 지정되면 프로세스의 현재 작업 디렉토리 기준으로 해석한다
- A6: `io.MultiWriter`를 사용하여 stdout과 파일에 동시에 출력할 수 있다

### 2.2 도메인 가정

- A7: 로그 출력 대상은 서버 시작 시 결정되며, 런타임에 변경할 수 없다 (파일 핸들 안전성)
- A8: 파일 출력 시 디렉토리가 존재하지 않으면 자동 생성한다 (`os.MkdirAll`)
- A9: 파일은 추가(append) 모드로 열어 기존 로그를 보존한다
- A10: "stdout" 문자열은 표준 출력을 의미하는 예약어이며, 파일 경로로 사용할 수 없다

---

## 3. Requirements (요구사항)

### Module 1: Config Types Extension - 설정 타입 확장 (P0)

#### REQ-OBS-002-01-01 (Ubiquitous) ObserveConfig 구조체 확장

시스템은 **항상** `ObserveConfig` 구조체에 다음 필드를 포함해야 한다:

- `Format string` - 로그 출력 포맷 ("json" 또는 "text")
- `Output string` - 로그 출력 대상 ("stdout", 파일 경로, 또는 "stdout+파일경로")

#### REQ-OBS-002-01-02 (Ubiquitous) Viper 기본값 등록

시스템은 **항상** `SetDefaults()` 함수에서 다음 기본값을 등록해야 한다:

- `observe.format` = `"json"` (기본 포맷)
- `observe.output` = `"stdout"` (기본 출력 대상)

#### REQ-OBS-002-01-03 (Ubiquitous) Observe() 접근자 확장

시스템은 **항상** `Observe()` 접근자가 `ObserveConfig`를 반환할 때 `Format`과 `Output` 필드를 Viper에서 읽어 포함해야 한다.

#### REQ-OBS-002-01-04 (Ubiquitous) Immutable 설정

시스템은 **항상** `observe.format`과 `observe.output` 키를 런타임 변경 불가(immutable)로 취급해야 한다. `mutableKeys` 맵에 추가하지 않아야 한다.

---

### Module 2: Config Validation - 설정 검증 (P0)

#### REQ-OBS-002-02-01 (Event-Driven) Format 검증

**WHEN** 설정 검증(`Validate`) 실행 시, **THEN** `observe.format` 값이 "json" 또는 "text"가 아니면 `ErrInvalidLogFormat` 에러를 추가해야 한다.

#### REQ-OBS-002-02-02 (Event-Driven) Output 검증

**WHEN** 설정 검증(`Validate`) 실행 시, **THEN** `observe.output` 값이 다음 패턴 중 하나가 아니면 `ErrInvalidLogOutput` 에러를 추가해야 한다:

- `"stdout"` - 표준 출력
- 절대 또는 상대 파일 경로 (빈 문자열 아님)
- `"stdout+파일경로"` - 표준 출력과 파일 동시 출력 (`+` 구분자 사용)

#### REQ-OBS-002-02-03 (Unwanted Behavior) 빈 Output 값 거부

시스템은 `observe.output` 값이 빈 문자열이거나 `+`만 포함하는 경우 `ErrInvalidLogOutput` 에러를 반환**해야 한다**.

---

### Module 3: Observer Initialization - Observer 초기화 (P0)

#### REQ-OBS-002-03-01 (Event-Driven) Format 설정 적용

**WHEN** 서버 시작 시 Observer를 초기화할 때, **THEN** `cfg.Observe().Format` 값을 읽어 `observe.WithObserverFormat(format)` 옵션으로 전달해야 한다. CLI `--log-level` 패턴과 동일하게 설정 파일 값이 기본값을 오버라이드한다.

#### REQ-OBS-002-03-02 (Event-Driven) stdout 출력 설정

**WHEN** `observe.output`이 `"stdout"`이면, **THEN** 기본 동작을 유지하고 별도의 Writer 설정을 하지 않아야 한다 (Observer 기본값이 `os.Stdout`).

#### REQ-OBS-002-03-03 (Event-Driven) 파일 출력 설정

**WHEN** `observe.output`이 파일 경로이면, **THEN** 해당 경로의 파일을 `os.OpenFile`로 열고(append 모드, 0644 퍼미션), 디렉토리가 없으면 `os.MkdirAll`로 생성한 뒤, `observe.WithObserverWriter(file)` 옵션으로 전달해야 한다.

#### REQ-OBS-002-03-04 (Event-Driven) stdout + 파일 동시 출력 설정

**WHEN** `observe.output`이 `"stdout+파일경로"` 형식이면, **THEN** `os.Stdout`과 해당 파일의 `io.MultiWriter`를 생성하여 `observe.WithObserverWriter(multiWriter)` 옵션으로 전달해야 한다.

#### REQ-OBS-002-03-05 (Event-Driven) 파일 열기 실패 처리

**WHEN** 파일 경로로 지정된 파일을 열 수 없으면, **THEN** 서버 시작을 중단하고 에러 메시지를 반환해야 한다 (`fmt.Errorf("로그 출력 파일 열기 실패: %w", err)`).

---

### Module 4: CLI Flag Integration - CLI 플래그 통합 (P1)

#### REQ-OBS-002-04-01 (Optional) --log-output CLI 플래그

**가능하면** `xflowd` 커맨드에 `--log-output` 플래그를 제공하여 CLI에서 직접 로그 출력 대상을 지정할 수 있어야 한다. CLI 값이 설정 파일 값보다 우선한다.

#### REQ-OBS-002-04-02 (State-Driven) CLI 값 우선 적용

**IF** `--log-output` CLI 플래그가 비어있지 않으면 **THEN** 설정 파일의 `observe.output` 값 대신 CLI 플래그 값을 사용해야 한다.

---

### Module 5: File Handle Lifecycle - 파일 핸들 생명주기 (P0)

#### REQ-OBS-002-05-01 (Event-Driven) 서버 종료 시 파일 핸들 닫기

**WHEN** 서버가 종료(`ctx` 취소 또는 시그널 수신)되면, **THEN** 열려 있는 로그 파일 핸들을 `Close()`하여 리소스를 정리해야 한다. Agent 매니저 종료 이후, 최종 로그 메시지 기록 완료 후 닫아야 한다.

#### REQ-OBS-002-05-02 (Unwanted Behavior) 파일 핸들 누수 방지

시스템은 어떤 종료 경로에서도 파일 핸들이 닫히지 않은 채 프로세스가 종료되는 상황이 발생**하지 않아야 한다**. `defer` 패턴을 사용하여 보장한다.

---

## 4. Specifications (명세)

### 4.1 설정 YAML 구조

```yaml
observe:
  default_level: "info"       # 기존
  metrics:
    enabled: true             # 기존
  trace:
    enabled: false            # 기존
  format: "json"              # 신규 - "json" 또는 "text"
  output: "stdout"            # 신규 - "stdout", 파일 경로, "stdout+파일경로"
```

### 4.2 수정 대상 파일

```
internal/config/
  types.go          # ObserveConfig 구조체 확장
  defaults.go       # observe.format, observe.output 기본값 추가
  config.go         # Observe() 접근자 확장
  validate.go       # validateLogFormat, validateLogOutput 추가
  errors.go         # ErrInvalidLogFormat, ErrInvalidLogOutput 추가 (기존 errors.go에)

cmd/xflowd/
  main.go           # Observer 초기화 로직 확장, --log-output 플래그, 파일 핸들 관리
```

### 4.3 타입 시그니처

```go
// ObserveConfig - 관측 설정 (확장)
type ObserveConfig struct {
    DefaultLevel   string // "debug", "info", "warn", "error"
    MetricsEnabled bool
    TraceEnabled   bool
    Format         string // "json" 또는 "text" (신규)
    Output         string // "stdout", 파일경로, "stdout+파일경로" (신규)
}
```

### 4.4 검증 함수 시그니처

```go
// validateLogFormat - observe.format이 "json" 또는 "text"인지 검증
func validateLogFormat(v *viper.Viper, ve *ValidationErrors)

// validateLogOutput - observe.output이 유효한 출력 대상인지 검증
func validateLogOutput(v *viper.Viper, ve *ValidationErrors)
```

### 4.5 에러 변수

```go
var (
    ErrInvalidLogFormat = errors.New("invalid log format (must be 'json' or 'text')")
    ErrInvalidLogOutput = errors.New("invalid log output target")
)
```

### 4.6 출력 대상 파싱 로직

```go
// parseLogOutput 은 observe.output 문자열을 파싱하여 io.Writer를 반환한다.
// 반환값: writer io.Writer, closer io.Closer (파일 핸들, nil if stdout only), err error
func parseLogOutput(output string) (io.Writer, io.Closer, error)
```

파싱 규칙:
- `"stdout"` → `os.Stdout`, `nil`, `nil`
- `"/path/to/file"` → `*os.File`, `*os.File`, `nil`
- `"stdout+/path/to/file"` → `io.MultiWriter(os.Stdout, file)`, `file`, `nil`

### 4.7 CLI 플래그 확장

```go
// newRootCmd 에 추가
cmd.PersistentFlags().StringVar(&logOutput, "log-output", "", "로그 출력 대상 (설정 파일 값 우선)")
```

### 4.8 Observer 초기화 확장 (main.go)

```go
// Format 설정 적용
obsCfg := cfg.Observe()
if obsCfg.Format != "" {
    obsOpts = append(obsOpts, observe.WithObserverFormat(obsCfg.Format))
}

// Output 설정 적용 (CLI 우선)
outputTarget := obsCfg.Output
if logOutput != "" {
    outputTarget = logOutput
}
if outputTarget != "" && outputTarget != "stdout" {
    writer, closer, err := parseLogOutput(outputTarget)
    if err != nil {
        return fmt.Errorf("로그 출력 설정 실패: %w", err)
    }
    if closer != nil {
        defer closer.Close()
    }
    obsOpts = append(obsOpts, observe.WithObserverWriter(writer))
}
```

### 4.9 mutableKeys 관련

`observe.format`과 `observe.output`은 `mutableKeys` 맵에 추가하지 **않는다**. 기본적으로 immutable 키로 취급되어 `IsImmutable("observe.format") == true`, `IsImmutable("observe.output") == true`가 된다. 파일 핸들의 런타임 변경은 안전하지 않으므로 시작 시 설정만 지원한다.

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 대상 파일 | 우선순위 |
|------------|------|----------|---------|
| REQ-OBS-002-01-01 ~ 01-04 | Config Types Extension | types.go, defaults.go, config.go, mutable.go | P0 |
| REQ-OBS-002-02-01 ~ 02-03 | Config Validation | validate.go, errors.go | P0 |
| REQ-OBS-002-03-01 ~ 03-05 | Observer Initialization | cmd/xflowd/main.go | P0 |
| REQ-OBS-002-04-01 ~ 04-02 | CLI Flag Integration | cmd/xflowd/main.go | P1 |
| REQ-OBS-002-05-01 ~ 05-02 | File Handle Lifecycle | cmd/xflowd/main.go | P0 |
