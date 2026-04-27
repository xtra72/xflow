# SPEC-OBS-003: 구현 계획

## 개요

노드별 로그 출력 라우팅 기능을 구현한다. 기존 `resolveNodeLogLevel` 패턴을 활용하여 `resolveNodeLogOutput` 함수를 추가하고, `StreamRouter`를 통해 노드별 로그를 파일 또는 stdout으로 라우팅한다.

---

## 마일스톤

### Primary Goal: 핵심 파싱 및 라우팅 로직

**목적**: `log_output` 값을 파싱하고, 계층적으로 결정하는 핵심 로직 구현

**변경 대상 파일**:

1. `internal/engine/log_output.go` (신규)
   - `LogOutputTarget` 타입 정의
   - `ParseLogOutput(raw string) (LogOutputTarget, error)` 함수
   - `resolveNodeLogOutput(nd, flowCfg, serverDefault) (LogOutputTarget, bool)` 함수
   - `openLogFile(path string) (*os.File, error)` 헬퍼 함수

2. `internal/engine/log_output_test.go` (신규)
   - `ParseLogOutput` 테이블 기반 테스트 (정상 케이스 + 에러 케이스)
   - `resolveNodeLogOutput` 계층적 결정 테스트
   - `openLogFile` 디렉토리 자동 생성 및 에러 처리 테스트

**기술적 접근**:

- `ParseLogOutput`은 문자열을 분석하여 `LogOutputTarget`을 반환
  - `"stdout"` -> `{UseStdout: true, FilePath: ""}`
  - `"/var/log/app.log"` -> `{UseStdout: false, FilePath: "/var/log/app.log"}`
  - `"stdout+/var/log/app.log"` -> `{UseStdout: true, FilePath: "/var/log/app.log"}`
- `resolveNodeLogOutput`은 `resolveNodeLogLevel`과 동일한 3단계 우선순위 패턴

**위험 요소**:

- 파일 경로 유효성 검증의 범위 결정 (존재 여부 vs 형식만)
- 상대 경로와 절대 경로의 처리 방식 통일 필요

---

### Secondary Goal: FlowConfig 확장 및 직렬화

**목적**: `FlowConfig`에 `LogOutput` 필드를 추가하고, JSON/YAML 직렬화 호환성을 확보

**변경 대상 파일**:

1. `pkg/flow/flow.go`
   - `FlowConfig` 구조체에 `LogOutput string` 필드 추가
   - JSON 태그: `json:"log_output,omitempty"`
   - YAML 태그: `yaml:"log_output,omitempty"`

2. `pkg/flow/serialize.go`
   - `flowJSON` 구조체의 `Config` 필드가 `FlowConfig`를 직접 사용하므로, `FlowConfig` 변경이 자동 반영됨
   - YAML 직렬화 관련 `flowYAML` 구조체 및 `configYAML` 구조체 확인 및 필요 시 업데이트

3. `pkg/flow/serialize_test.go`
   - `LogOutput` 포함된 JSON/YAML 직렬화/역직렬화 테스트 추가
   - 기존 테스트가 `LogOutput` 부재 시에도 정상 동작하는지 확인

**기술적 접근**:

- `FlowConfig`에 `omitempty` 태그를 적용하여 빈 문자열 시 직렬화 제외
- 기존 Flow YAML 파일과의 역호환성 자동 보장 (Go의 `omitempty` 특성)

**위험 요소**:

- YAML 직렬화에서 `configYAML` 중간 구조체 사용 시 필드 누락 가능성
- 기존 테스트의 `FlowConfig` 리터럴이 새 필드 추가로 영향 받을 수 있음

---

### Tertiary Goal: Engine 통합 및 파일 핸들 관리

**목적**: `DeployFlow`에 로그 출력 라우팅을 통합하고, 파일 핸들 생명주기를 관리

**변경 대상 파일**:

1. `internal/engine/types.go`
   - `flowRuntime` 구조체에 `closers []io.Closer` 필드 추가

2. `internal/engine/engine.go`
   - `DeployFlow` 함수: 노드 생성 루프에 `resolveNodeLogOutput` 호출 및 `StreamRouter.AddRoute` 로직 추가
   - `StopFlow` 함수: `flowRuntime.closers` 정리 로직 추가
   - `UndeployFlow` 함수: `flowRuntime.closers` 정리 로직 추가
   - 에러 시 rollback 로직 (이미 열린 파일 핸들 정리)

3. `internal/engine/engine_test.go`
   - `DeployFlow`에서 `log_output` 설정된 노드의 스트림 라우팅 검증
   - 파일 열기 실패 시 `DeployFlow` 에러 반환 검증
   - `StopFlow` 후 파일 핸들 정리 검증
   - `stdout+file` 모드의 이중 출력 검증

**기술적 접근**:

- `DeployFlow`의 노드 생성 루프에서 `resolveNodeLogOutput` 호출
- 파일 출력 대상이 결정되면 `openLogFile`로 파일을 열고 `closers`에 추가
- `StreamRouter.AddRoute(component, fileWriter)`로 라우팅 등록
- `stdout+file` 모드에서는 파일 라우트만 추가하고, `StreamRouter`의 기본 `defaultWriter`(stdout)가 라우트 미등록 시 자동 사용되는 메커니즘을 활용
- 파일 전용 모드에서는 파일 라우트만 등록하여, `StreamRouter.Routes()`가 해당 writer만 반환하도록 함

**위험 요소**:

- `StreamRouter`의 기본 동작 이해 필요: `Routes()`가 빈 경우에만 `defaultWriter` 사용
- 파일 전용 모드에서 stdout 출력을 확실히 차단하려면, `StreamRouter`의 동작 방식을 정확히 파악해야 함
- 동시성: `DeployFlow` 중 에러 발생 시 부분적으로 등록된 라우트와 파일 핸들의 정리

---

### Optional Goal: 유효성 검증 확장

**목적**: `flow.Validate()` 또는 `DeployFlow` 시 `log_output` 값의 사전 검증

**변경 대상 파일**:

1. `pkg/flow/validate.go` (선택)
   - `log_output` 형식 검증 규칙 추가
   - 노드 `config["log_output"]` 값 형식 검증

2. `pkg/flow/validate_test.go` (선택)
   - 잘못된 `log_output` 형식에 대한 경고/에러 검증

**기술적 접근**:

- `flow.Validate()`에 새로운 검증 규칙 추가는 선택 사항
- `DeployFlow` 내에서 `ParseLogOutput`을 호출할 때 검증이 이미 수행됨
- `Validate()`에 추가하면 사전 검증이 가능하지만, `engine` 패키지의 `ParseLogOutput`을 `flow` 패키지에서 호출하는 의존성 문제 발생
- 대안: `ParseLogOutput`을 `pkg/flow` 또는 `internal/observe` 패키지로 이동

**위험 요소**:

- 패키지 의존성 방향 위반 (`pkg/flow` -> `internal/engine` 불가)
- 검증 로직 중복 가능성

---

## 아키텍처 설계 방향

### 데이터 흐름

```
Flow YAML
  |
  v
FlowConfig.LogOutput ──> resolveNodeLogOutput() ──> LogOutputTarget
                                |                        |
NodeDef.Config["log_output"] ───┘                        |
                                                         v
                                                   openLogFile()
                                                         |
                                                         v
                                              StreamRouter.AddRoute()
                                                         |
                                                         v
                                              routingHandler.Handle()
                                                   |         |
                                                   v         v
                                                stdout    log file
```

### 파일 구조

```
internal/engine/
  log_output.go       # LogOutputTarget, ParseLogOutput, resolveNodeLogOutput, openLogFile
  log_output_test.go  # 위 함수들의 단위 테스트
  types.go            # flowRuntime.closers 필드 추가
  engine.go           # DeployFlow/StopFlow/UndeployFlow 수정

pkg/flow/
  flow.go             # FlowConfig.LogOutput 필드 추가
  serialize.go        # YAML 직렬화 중간 구조체 확인
  serialize_test.go   # 직렬화 테스트 추가
```

### StreamRouter 동작 분석

현재 `routingHandler.Handle()`의 동작:

1. `record`에서 `component` 추출
2. `router.Routes(component)` 호출
3. 라우트가 **있으면**: 해당 라우트의 writer들에만 출력
4. 라우트가 **없으면**: `defaultWriter`(stdout)에 출력

이 동작 방식에 따른 설계:

- **파일 전용 모드** (`"/var/log/node.log"`): 파일 writer만 `AddRoute` -> `Routes()`가 파일만 반환 -> stdout 출력 안 됨
- **stdout+file 모드** (`"stdout+/var/log/node.log"`): 파일 writer `AddRoute` + stdout writer `AddRoute` -> 두 곳 모두 출력
- **stdout 전용 모드** (`"stdout"`): 라우트 미등록 -> `Routes()`가 nil 반환 -> `defaultWriter`(stdout) 사용
- **미설정** (상위 상속): 결정된 대상에 따라 위 3가지 중 하나 적용

> 주의: `stdout+file` 모드에서는 `os.Stdout`도 명시적으로 `AddRoute`해야 한다. `Routes()`가 비어있지 않으면 `defaultWriter`가 사용되지 않기 때문이다.

---

## 제약사항

- `BaseNode` 구조체를 변경하지 않는다
- 파일 관리는 `Engine`/`flowRuntime` 레벨에서만 수행한다
- 모든 코드는 `go test -race`를 통과해야 한다
- 테스트 커버리지 목표: 85% 이상
- 기존 테스트가 깨지지 않아야 한다 (역호환성)
- `internal/engine` -> `internal/observe` 방향의 의존성만 허용 (`pkg/flow` -> `internal/engine` 불가)
