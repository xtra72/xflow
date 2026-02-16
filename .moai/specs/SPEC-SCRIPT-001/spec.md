---
id: SPEC-SCRIPT-001
version: "1.1.0"
status: implementing
created: "2026-02-13"
updated: "2026-02-16"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-13 | 1.0.0 | 초기 SPEC 작성 |
| 2026-02-16 | 1.1.0 | P0 구현 완료 (eea59aa): Module 10 (Error Types), Module 8 (Data Bridge), Module 4 (Sandbox), Module 1+2+3 (Engine Core). 86 tests, 96.9% coverage. P1 모듈 구현 대기 중. |

---

# SPEC-SCRIPT-001: Lua Script Engine System - Lua 스크립트 엔진 시스템

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 Lua 스크립트 엔진 시스템을 정의한다. 스크립트 엔진은 GopherLua 기반의 순수 Go Lua VM을 사용하여, 플로우 실행 중 실시간으로 스크립트를 실행하고 핫 리로드할 수 있는 경량 스크립팅 환경을 제공한다.

본 SPEC은 다음을 다룬다:
- `ScriptEngine` 인터페이스 정의 (Init, Shutdown, Execute, Compile, Stats)
- Lua VM 풀 관리 (생성, 획득, 반환, 재활용)
- 스크립트 컴파일 및 캐싱 (FunctionProto 캐시)
- 샌드박스 실행 환경 (위험 함수 차단, 메모리/CPU 제한)
- 스크립트 소스 로더 (파일, 인라인, 스토어)
- 핫 리로드 (무중단 스크립트 교체, 버전 관리, 롤백)
- xflow 내장 표준 라이브러리 (JSON, math, string, time, log, store, event)
- Go-Lua 데이터 브릿지 (구조체/테이블 양방향 변환)
- 런타임 정보 및 통계 (VM 풀 사용률, 실행 통계)
- 에러 타입 정의

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/script/`
- **핵심 의존성**: `github.com/yuin/gopher-lua` v1.1+ (순수 Go Lua VM)
- **내부 의존성**: `pkg/message/` (SPEC-MSG-001), `internal/config/` (SPEC-CFG-001), `pkg/lifecycle/` (SPEC-LIFE-001), `internal/observe/` (SPEC-OBS-001), `internal/storage/` (SPEC-STORE-001), `pkg/xferr/` (SPEC-ERR-001)
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **Tier**: internal (비공개 패키지, 외부 임포트 불가)

### 1.3 설계 원칙

- **VM 풀링**: Lua VM 생성 비용을 줄이기 위해 VM 풀을 관리하고 재사용
- **샌드박스 격리**: 스크립트가 시스템 자원에 직접 접근할 수 없도록 위험 함수를 차단하고 리소스 제한 적용
- **핫 리로드**: 플로우 실행 중단 없이 스크립트를 교체할 수 있는 원자적 교체 메커니즘
- **컴파일 캐싱**: 동일 스크립트의 반복 컴파일을 방지하여 성능 최적화
- **양방향 브릿지**: Go 타입과 Lua 타입 간 자동 변환으로 데이터 통합 용이
- **생명주기 통합**: `pkg/lifecycle/Lifecycle`, `Configurable` 인터페이스 구현
- **CGo-free**: GopherLua는 순수 Go 구현이므로 크로스 컴파일과 배포가 용이

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- `ScriptEngine` 인터페이스 및 구현 (`engine.go`)
- Lua VM 풀 관리 (`engine.go`)
- 스크립트 컴파일 및 `*lua.FunctionProto` 캐싱 (`engine.go`)
- 샌드박스 환경 구성 (`sandbox.go`)
- 스크립트 소스 로딩 (`loader.go`)
- 핫 리로드 메커니즘 (`hotreload.go`)
- xflow 내장 표준 라이브러리 (`stdlib.go`)
- Go-Lua 데이터 브릿지 (`bridge.go`)
- 런타임 정보 및 통계 (`info.go`)
- 에러 타입 정의 (`errors.go`)

**OUT OF SCOPE (별도 SPEC)**:
- ScriptNode 구현 (SPEC-NODE-001 Module 9: `internal/node/script.go`)
- LuaTransformer 구현 (SPEC-BRIDGE-001: `internal/agent/bridge/`)
- Flow Engine 통합 (SPEC-ENGINE-001: `internal/engine/`)
- 관찰성 시스템 구현 자체 (SPEC-OBS-001: `internal/observe/`)
- 설정 시스템 구현 자체 (SPEC-CFG-001: `internal/config/`)
- REST API 핸들러 (별도 SPEC: `internal/api/`)

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-NODE-001 | 소비자 | ScriptNode(Module 9)가 ScriptEngine을 사용하여 Lua 스크립트 실행 |
| SPEC-BRIDGE-001 | 소비자 | LuaTransformer가 ScriptEngine을 사용하여 Agent 데이터 변환 |
| SPEC-MSG-001 | 의존 | Message 타입을 Go-Lua 브릿지에서 변환하여 사용 |
| SPEC-CFG-001 | 의존 | 스크립트 엔진 설정(timeout, vm_pool_size, sandbox) 관리 |
| SPEC-LIFE-001 | 의존 | ScriptEngine이 Lifecycle, Configurable 인터페이스 구현 |
| SPEC-OBS-001 | 동료 | 컴포넌트 키 "script", "script.vm-pool"로 로깅/메트릭 추적 |
| SPEC-ERR-001 | 의존 | ComponentScriptEngine = "script_engine" 컴포넌트 타입 사용 |
| SPEC-STORE-001 | 동료 | Lua 바인딩 xflow.store를 통해 Store Agent 접근 |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: GopherLua `*lua.LState`는 단일 goroutine에서만 사용해야 하며, VM 풀을 통해 goroutine별로 전용 VM을 할당한다
- A2: `*lua.FunctionProto`(컴파일된 스크립트)는 여러 `*lua.LState`에서 안전하게 공유할 수 있으며, 컴파일 캐시의 핵심이다
- A3: VM 풀 크기는 SPEC-CFG-001의 `script.vm_pool_size` 설정(기본값 10)으로 결정되며, 런타임에 변경 불가(IMMUTABLE)하다
- A4: 스크립트 실행 타임아웃은 SPEC-CFG-001의 `script.timeout` 설정(기본값 5s)으로 결정된다
- A5: 샌드박스는 `os`, `io`, `debug`, `loadfile`, `dofile` 모듈/함수를 차단하며, 화이트리스트 방식으로 허용 모듈을 관리한다
- A6: 핫 리로드는 현재 실행 중인 스크립트에는 영향을 주지 않고, 다음 실행부터 새 버전이 적용되는 원자적 교체 방식이다
- A7: Go struct -> Lua table 변환 시 exported 필드만 변환하며, 필드명은 소문자 snake_case로 변환한다
- A8: Lua table -> Go map 변환 시 Lua number는 float64로, string은 string으로, boolean은 bool로 변환한다
- A9: 컴파일 캐시는 스크립트 소스 내용의 해시를 키로 사용하며, 소스 변경 시 캐시가 무효화된다

### 2.2 도메인 가정

- A10: ScriptNode(SPEC-NODE-001)는 `ScriptEngine.Execute()`를 호출하여 메시지별로 스크립트를 실행한다
- A11: LuaTransformer(SPEC-BRIDGE-001)는 `ScriptEngine.Execute()`를 호출하여 Agent 데이터를 변환한다
- A12: 스크립트 실행 시 입력 데이터는 Lua 전역 변수 `msg`로 전달되며, 반환값은 Lua 함수의 리턴값이다
- A13: xflow 내장 라이브러리는 `xflow` 전역 테이블 하위에 등록된다 (예: `xflow.json.encode()`, `xflow.store.get()`)
- A14: 핫 리로드 시 이전 버전은 1개만 보관하며, 롤백은 직전 버전으로만 가능하다
- A15: VM 풀이 고갈되면 `ErrVMPoolExhausted` 에러를 즉시 반환하며, 대기하지 않는다

---

## 3. Requirements (요구사항)

### Module 1: Script Engine Core - 스크립트 엔진 코어 (P0)

파일: `engine.go`

#### REQ-SCRIPT-001-01-01 (Ubiquitous) ScriptEngine 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `ScriptEngine` 인터페이스를 제공해야 한다:

- `Init(ctx context.Context) error` - 엔진 초기화 (VM 풀 생성, 캐시 초기화)
- `Shutdown(ctx context.Context) error` - 엔진 종료 (VM 풀 해제, 리소스 정리)
- `Execute(ctx context.Context, scriptID string, input any) (any, error)` - 스크립트 실행
- `Compile(ctx context.Context, source ScriptSource) (string, error)` - 스크립트 컴파일 및 등록 (scriptID 반환)
- `Remove(scriptID string) error` - 등록된 스크립트 제거
- `Stats() ScriptEngineStats` - 엔진 통계 조회
- `Info() ScriptEngineInfo` - 엔진 런타임 정보 조회

#### REQ-SCRIPT-001-01-02 (Ubiquitous) 스레드 안전 엔진

시스템은 **항상** `ScriptEngine`의 모든 공개 메서드가 동시성 안전(thread-safe)하게 동작해야 한다. 다수의 goroutine에서 동시에 Execute, Compile 등을 호출해도 데이터 레이스가 발생하지 않아야 한다.

#### REQ-SCRIPT-001-01-03 (Event-Driven) Execute 호출 시 스크립트 실행

**WHEN** `ScriptEngine.Execute(ctx, scriptID, input)` 호출 시, **THEN** VM 풀에서 VM을 획득하고, 컴파일 캐시에서 해당 scriptID의 `FunctionProto`를 로드하여, 입력 데이터를 Lua 전역 변수로 설정한 뒤 스크립트를 실행하고, 실행 결과를 Go 타입으로 변환하여 반환해야 한다. 실행 완료 후 VM을 풀에 반환한다.

#### REQ-SCRIPT-001-01-04 (Event-Driven) Compile 호출 시 스크립트 등록

**WHEN** `ScriptEngine.Compile(ctx, source)` 호출 시, **THEN** 소스 코드를 Lua `FunctionProto`로 컴파일하고, 컴파일 결과를 캐시에 저장한 뒤, 고유 scriptID를 생성하여 반환해야 한다. 컴파일 실패 시 `ErrScriptCompileFailed` 에러를 반환한다.

#### REQ-SCRIPT-001-01-05 (Ubiquitous) 생명주기 통합

시스템은 **항상** `ScriptEngine`이 `pkg/lifecycle/Lifecycle` 및 `Configurable` 인터페이스를 구현해야 한다:

- `Lifecycle`: Init, Start, Pause, Resume, Stop, State
- `Configurable`: Configure, GetConfig (런타임 타임아웃 변경 등)

---

### Module 2: VM Pool - VM 풀 관리 (P0)

파일: `engine.go`

#### REQ-SCRIPT-001-02-01 (Ubiquitous) VM 풀 구조체

시스템은 **항상** 고정 크기의 Lua VM 풀을 관리하는 `VMPool` 구조체를 제공해야 한다. 풀 크기는 SPEC-CFG-001의 `script.vm_pool_size` 설정값으로 결정된다.

#### REQ-SCRIPT-001-02-02 (Event-Driven) VM 획득

**WHEN** VM 풀에서 VM 획득을 요청하면, **THEN** 사용 가능한 VM을 즉시 반환해야 한다. 모든 VM이 사용 중이면 `ErrVMPoolExhausted` 에러를 즉시 반환한다.

#### REQ-SCRIPT-001-02-03 (Event-Driven) VM 반환

**WHEN** 스크립트 실행이 완료되어 VM을 풀에 반환하면, **THEN** VM의 전역 상태를 정리(글로벌 변수 리셋)한 뒤 풀에 재배치해야 한다.

#### REQ-SCRIPT-001-02-04 (Ubiquitous) VM 재활용

시스템은 **항상** VM이 설정된 최대 사용 횟수(기본: 1000회)를 초과하면 해당 VM을 폐기하고 새 VM을 생성해야 한다. 이를 통해 메모리 누수를 방지한다.

#### REQ-SCRIPT-001-02-05 (Event-Driven) VM 생성 시 샌드박스 적용

**WHEN** 새 Lua VM을 생성할 때, **THEN** 샌드박스 설정을 적용하여 위험 모듈/함수를 제거하고, xflow 내장 라이브러리를 등록해야 한다.

#### REQ-SCRIPT-001-02-06 (Unwanted) VM 풀 크기 런타임 변경 금지

시스템은 VM 풀 크기를 런타임에 **변경하지 않아야 한다**. SPEC-CFG-001에서 `script.vm_pool_size`는 IMMUTABLE로 정의되어 있으며, 변경 시 재시작이 필요하다.

---

### Module 3: Script Compilation & Caching - 스크립트 컴파일 및 캐싱 (P0)

파일: `engine.go`

#### REQ-SCRIPT-001-03-01 (Ubiquitous) 컴파일 캐시

시스템은 **항상** 컴파일된 `*lua.FunctionProto`를 scriptID 키로 캐시에 저장하여 반복 컴파일을 방지해야 한다.

#### REQ-SCRIPT-001-03-02 (Event-Driven) 캐시 히트 시 재컴파일 스킵

**WHEN** `Execute()` 호출 시 해당 scriptID의 `FunctionProto`가 캐시에 존재하면, **THEN** 재컴파일 없이 캐시된 프로토를 사용해야 한다.

#### REQ-SCRIPT-001-03-03 (Event-Driven) 소스 변경 시 캐시 무효화

**WHEN** 핫 리로드 또는 `Compile()`을 통해 동일 scriptID의 소스가 변경되면, **THEN** 기존 캐시 엔트리를 무효화하고 새로 컴파일된 프로토로 교체해야 한다.

#### REQ-SCRIPT-001-03-04 (Event-Driven) 컴파일 에러 처리

**WHEN** 스크립트 소스에 문법 오류가 있어 컴파일이 실패하면, **THEN** `ErrScriptCompileFailed` 에러를 반환해야 하며, 에러 메시지에 줄 번호와 오류 내용을 포함해야 한다. 기존 캐시는 유지된다.

#### REQ-SCRIPT-001-03-05 (Event-Driven) Remove 호출 시 캐시 제거

**WHEN** `ScriptEngine.Remove(scriptID)` 호출 시, **THEN** 해당 scriptID의 컴파일 캐시를 삭제해야 한다. 존재하지 않는 scriptID에 대해서는 `ErrScriptNotFound`를 반환한다.

---

### Module 4: Sandbox Environment - 샌드박스 환경 (P0)

파일: `sandbox.go`

#### REQ-SCRIPT-001-04-01 (Ubiquitous) 위험 모듈 차단

시스템은 **항상** 다음 Lua 모듈 및 함수를 샌드박스 환경에서 제거해야 한다:

| 차단 대상 | 이유 |
|-----------|------|
| `os` 모듈 | 파일 시스템, 프로세스 조작 방지 |
| `io` 모듈 | 파일 입출력 방지 |
| `debug` 모듈 | 런타임 내부 조작 방지 |
| `loadfile()` | 파일 시스템에서 코드 로딩 방지 |
| `dofile()` | 파일 시스템에서 코드 실행 방지 |
| `load()` (선택적) | 동적 코드 로딩 제한 (설정에 따라 허용/차단) |

#### REQ-SCRIPT-001-04-02 (Ubiquitous) SandboxConfig 구조체

시스템은 **항상** 다음 필드를 포함하는 `SandboxConfig` 구조체를 제공해야 한다:

- `Enabled bool` - 샌드박스 활성화 여부 (기본: true)
- `DisabledModules []string` - 비활성화할 모듈 목록
- `DisabledFunctions []string` - 비활성화할 전역 함수 목록
- `MaxExecutionTime time.Duration` - 최대 실행 시간 (기본: 5s)
- `MaxMemoryMB int` - 최대 메모리 사용량 MB (기본: 128)
- `AllowDynamicLoad bool` - `load()` 함수 허용 여부 (기본: false)

#### REQ-SCRIPT-001-04-03 (State-Driven) 실행 시간 제한

**WHILE** 스크립트가 실행 중인 동안, 시스템은 `context.Context`의 타임아웃 또는 `MaxExecutionTime`을 초과하면 실행을 중단하고 `ErrScriptTimeout`을 반환해야 한다.

#### REQ-SCRIPT-001-04-04 (Unwanted) 차단된 함수 호출 금지

시스템은 스크립트에서 차단된 모듈/함수를 **호출하지 않아야 한다**. 차단된 함수 호출 시도 시 Lua 런타임 에러를 발생시키고, Go 측에서 `ErrSandboxViolation`을 반환해야 한다.

#### REQ-SCRIPT-001-04-05 (Event-Driven) 샌드박스 적용 메서드

**WHEN** `ApplySandbox(L *lua.LState, config SandboxConfig)` 호출 시, **THEN** 지정된 모듈과 함수를 Lua 상태에서 제거하고, xflow 표준 라이브러리를 등록해야 한다.

---

### Module 5: Script Loader - 스크립트 로더 (P1)

파일: `loader.go`

#### REQ-SCRIPT-001-05-01 (Ubiquitous) ScriptSource 구조체

시스템은 **항상** 다음 필드를 포함하는 `ScriptSource` 구조체를 제공해야 한다:

- `Type SourceType` - 소스 유형 (Inline, File, Store)
- `Content string` - 인라인 코드 (Type이 Inline일 때)
- `Path string` - 파일 경로 (Type이 File일 때)
- `StoreKey string` - 스토어 키 (Type이 Store일 때)
- `Name string` - 스크립트 이름 (식별용)

#### REQ-SCRIPT-001-05-02 (Ubiquitous) SourceType 열거형

시스템은 **항상** 다음 소스 유형을 지원해야 한다:

- `SourceInline` - 인라인 코드 문자열
- `SourceFile` - 파일 시스템 경로
- `SourceStore` - Store Agent를 통한 키-값 조회

#### REQ-SCRIPT-001-05-03 (Event-Driven) 인라인 코드 로딩

**WHEN** `LoadScript(ctx, source)` 호출 시 소스 유형이 `SourceInline`이면, **THEN** `Content` 필드의 문자열을 직접 스크립트 소스로 사용해야 한다.

#### REQ-SCRIPT-001-05-04 (Event-Driven) 파일 로딩

**WHEN** `LoadScript(ctx, source)` 호출 시 소스 유형이 `SourceFile`이면, **THEN** `Path` 필드의 경로에서 파일을 읽어 스크립트 소스를 반환해야 한다. 파일이 존재하지 않으면 `ErrScriptNotFound`를 반환한다.

#### REQ-SCRIPT-001-05-05 (Event-Driven) 스토어 로딩

**WHEN** `LoadScript(ctx, source)` 호출 시 소스 유형이 `SourceStore`이면, **THEN** Store Agent에서 `StoreKey`로 스크립트 소스를 조회하여 반환해야 한다. 키가 존재하지 않으면 `ErrScriptNotFound`를 반환한다.

#### REQ-SCRIPT-001-05-06 (Event-Driven) 소스 유효성 검증

**WHEN** `LoadScript(ctx, source)` 호출 시, **THEN** 소스 유형에 맞는 필수 필드가 설정되어 있는지 검증해야 한다. 필수 필드가 누락되면 `ErrInvalidScriptSource`를 반환한다.

---

### Module 6: Hot Reload - 핫 리로드 (P1)

파일: `hotreload.go`

#### REQ-SCRIPT-001-06-01 (Ubiquitous) 핫 리로드 매니저

시스템은 **항상** 스크립트의 무중단 교체를 관리하는 `HotReloadManager` 구조체를 제공해야 한다.

#### REQ-SCRIPT-001-06-02 (Event-Driven) 스크립트 교체

**WHEN** `HotReloadManager.Reload(ctx, scriptID, newSource)` 호출 시, **THEN** 새 소스를 컴파일하고, 성공하면 기존 `FunctionProto`를 원자적으로 교체해야 한다. 현재 실행 중인 스크립트에는 영향을 주지 않으며, 다음 실행부터 새 버전이 적용된다.

#### REQ-SCRIPT-001-06-03 (Ubiquitous) 버전 추적

시스템은 **항상** 각 스크립트의 현재 버전 번호를 관리해야 한다. 리로드 시 버전 번호가 자동 증가한다.

#### REQ-SCRIPT-001-06-04 (Event-Driven) 롤백 지원

**WHEN** `HotReloadManager.Rollback(ctx, scriptID)` 호출 시, **THEN** 직전 버전의 `FunctionProto`로 원자적 교체를 수행해야 한다. 이전 버전이 존재하지 않으면 `ErrHotReloadFailed`를 반환한다.

#### REQ-SCRIPT-001-06-05 (Event-Driven) 리로드 실패 시 기존 유지

**IF** 새 소스의 컴파일이 실패하면, **THEN** 기존 스크립트를 유지하고 `ErrScriptCompileFailed`를 반환해야 한다. 기존 스크립트의 실행에는 전혀 영향을 주지 않는다.

#### REQ-SCRIPT-001-06-06 (Optional) 리로드 콜백 지원

**가능하면** 핫 리로드 성공/실패 시 등록된 콜백을 호출하여 리로드 이벤트를 알림할 수 있어야 한다.

---

### Module 7: Built-in Standard Library - 내장 표준 라이브러리 (P1)

파일: `stdlib.go`

#### REQ-SCRIPT-001-07-01 (Ubiquitous) xflow 전역 테이블

시스템은 **항상** Lua 환경에 `xflow` 전역 테이블을 등록하고, 하위에 표준 라이브러리 모듈을 배치해야 한다.

#### REQ-SCRIPT-001-07-02 (Ubiquitous) xflow.json 모듈

시스템은 **항상** 다음 JSON 처리 함수를 제공해야 한다:

- `xflow.json.encode(value)` - Lua 값을 JSON 문자열로 인코딩
- `xflow.json.decode(jsonStr)` - JSON 문자열을 Lua 테이블로 디코딩

#### REQ-SCRIPT-001-07-03 (Ubiquitous) xflow.log 모듈

시스템은 **항상** 다음 로깅 함수를 제공해야 한다:

- `xflow.log.debug(msg, ...)` - DEBUG 레벨 로그
- `xflow.log.info(msg, ...)` - INFO 레벨 로그
- `xflow.log.warn(msg, ...)` - WARN 레벨 로그
- `xflow.log.error(msg, ...)` - ERROR 레벨 로그

로그 출력은 SPEC-OBS-001의 관찰성 시스템을 통해 전달된다.

#### REQ-SCRIPT-001-07-04 (Ubiquitous) xflow.time 모듈

시스템은 **항상** 다음 시간 함수를 제공해야 한다:

- `xflow.time.now()` - 현재 Unix 타임스탬프(초, float64) 반환
- `xflow.time.now_ms()` - 현재 Unix 타임스탬프(밀리초, integer) 반환
- `xflow.time.format(timestamp, layout)` - 타임스탬프를 포맷 문자열로 변환
- `xflow.time.parse(str, layout)` - 문자열을 타임스탬프로 파싱

#### REQ-SCRIPT-001-07-05 (Ubiquitous) xflow.string 모듈

시스템은 **항상** Lua 기본 string 라이브러리에 추가로 다음 문자열 유틸리티를 제공해야 한다:

- `xflow.string.trim(str)` - 앞뒤 공백 제거
- `xflow.string.split(str, sep)` - 구분자로 분리하여 테이블 반환
- `xflow.string.join(tbl, sep)` - 테이블의 요소를 구분자로 결합
- `xflow.string.starts_with(str, prefix)` - 접두사 확인
- `xflow.string.ends_with(str, suffix)` - 접미사 확인

#### REQ-SCRIPT-001-07-06 (Ubiquitous) xflow.math 모듈

시스템은 **항상** Lua 기본 math 라이브러리에 추가로 다음 수학 함수를 제공해야 한다:

- `xflow.math.round(num, decimals)` - 소수점 반올림
- `xflow.math.clamp(num, min, max)` - 범위 제한
- `xflow.math.lerp(a, b, t)` - 선형 보간

#### REQ-SCRIPT-001-07-07 (Event-Driven) xflow.store 모듈

**WHEN** Lua 스크립트에서 `xflow.store.get(key)` 호출 시, **THEN** SPEC-STORE-001의 Store Agent에서 해당 키의 값을 조회하여 Lua 값으로 반환해야 한다.

시스템은 다음 스토어 함수를 제공해야 한다:

- `xflow.store.get(key)` - 키로 값 조회 (없으면 nil 반환)
- `xflow.store.set(key, value)` - 키에 값 저장
- `xflow.store.delete(key)` - 키 삭제
- `xflow.store.has(key)` - 키 존재 여부 확인 (boolean 반환)

#### REQ-SCRIPT-001-07-08 (Optional) xflow.event 모듈

**가능하면** Lua 스크립트에서 시스템 이벤트를 발행할 수 있어야 한다:

- `xflow.event.emit(name, data)` - 이벤트 발행

#### REQ-SCRIPT-001-07-09 (Ubiquitous) 라이브러리 등록 메커니즘

시스템은 **항상** `RegisterStdlib(L *lua.LState, opts StdlibOptions)` 함수를 통해 표준 라이브러리를 일괄 등록해야 한다. `StdlibOptions`로 개별 모듈의 활성화/비활성화를 제어할 수 있다.

---

### Module 8: Go-Lua Data Bridge - Go-Lua 데이터 브릿지 (P0)

파일: `bridge.go`

#### REQ-SCRIPT-001-08-01 (Ubiquitous) Go -> Lua 변환

시스템은 **항상** 다음 Go 타입을 Lua 타입으로 변환하는 `ToLuaValue(L *lua.LState, value any) lua.LValue` 함수를 제공해야 한다:

| Go 타입 | Lua 타입 |
|---------|----------|
| `nil` | `LNil` |
| `bool` | `LBool` |
| `int`, `int8/16/32/64` | `LNumber` |
| `uint`, `uint8/16/32/64` | `LNumber` |
| `float32`, `float64` | `LNumber` |
| `string` | `LString` |
| `[]byte` | `LString` |
| `[]any` (슬라이스) | `LTable` (배열) |
| `map[string]any` | `LTable` (해시) |
| `struct` (exported fields) | `LTable` (해시, snake_case 키) |
| `*message.Message` | `LTable` (특수 변환) |

#### REQ-SCRIPT-001-08-02 (Ubiquitous) Lua -> Go 변환

시스템은 **항상** 다음 Lua 타입을 Go 타입으로 변환하는 `FromLuaValue(value lua.LValue) any` 함수를 제공해야 한다:

| Lua 타입 | Go 타입 |
|----------|---------|
| `LNil` | `nil` |
| `LBool` | `bool` |
| `LNumber` | `float64` |
| `LString` | `string` |
| `LTable` (배열) | `[]any` |
| `LTable` (해시) | `map[string]any` |

#### REQ-SCRIPT-001-08-03 (Ubiquitous) Message 양방향 변환

시스템은 **항상** `pkg/message/Message`와 Lua 테이블 간 양방향 변환을 지원해야 한다:

- `MessageToLuaTable(L *lua.LState, msg *message.Message) *lua.LTable` - Message를 Lua 테이블로 변환
- `LuaTableToMessage(L *lua.LState, tbl *lua.LTable) (*message.Message, error)` - Lua 테이블을 Message로 변환

Message 변환 시 Payload, Metadata 필드를 Lua 테이블의 `payload`, `metadata` 키에 매핑한다.

#### REQ-SCRIPT-001-08-04 (Ubiquitous) 중첩 구조 지원

시스템은 **항상** 중첩된 Go map/slice/struct와 Lua 중첩 테이블 간의 재귀적 변환을 지원해야 한다.

#### REQ-SCRIPT-001-08-05 (Event-Driven) []byte 양방향 변환

**WHEN** Go `[]byte` 값을 Lua로 변환할 때, **THEN** Lua `LString`으로 변환해야 한다. 역방향으로 `LString`을 `[]byte`로 변환하는 `LuaStringToBytes(value lua.LValue) ([]byte, error)` 함수도 제공한다.

#### REQ-SCRIPT-001-08-06 (Unwanted) 변환 불가 타입 전달 금지

시스템은 Go 함수, 채널, 복소수 등 변환할 수 없는 타입을 Lua로 **전달하지 않아야 한다**. 변환 불가 타입 감지 시 `LNil`을 반환하고 경고 로그를 기록한다.

---

### Module 9: Script Info & Stats - 스크립트 정보 및 통계 (P1)

파일: `info.go`

#### REQ-SCRIPT-001-09-01 (Ubiquitous) ScriptEngineInfo 구조체

시스템은 **항상** 다음 필드를 포함하는 `ScriptEngineInfo` 구조체를 제공해야 한다:

- `PoolSize int` - VM 풀 총 크기
- `ActiveVMs int` - 현재 사용 중인 VM 수
- `IdleVMs int` - 유휴 VM 수
- `CachedScripts int` - 캐시된 스크립트 수
- `SandboxEnabled bool` - 샌드박스 활성화 여부
- `MaxExecutionTime time.Duration` - 최대 실행 시간 설정
- `State string` - 엔진 상태 (lifecycle.State 문자열)

#### REQ-SCRIPT-001-09-02 (Ubiquitous) ScriptEngineStats 구조체

시스템은 **항상** 다음 필드를 포함하는 `ScriptEngineStats` 구조체를 제공해야 한다:

- `TotalExecutions int64` - 총 실행 횟수
- `TotalErrors int64` - 총 에러 횟수
- `TotalTimeouts int64` - 총 타임아웃 횟수
- `AvgExecutionTime time.Duration` - 평균 실행 시간
- `MaxExecutionTime time.Duration` - 최대 실행 시간 (기록)
- `VMPoolExhausted int64` - VM 풀 고갈 횟수
- `CacheHits int64` - 캐시 히트 횟수
- `CacheMisses int64` - 캐시 미스 횟수
- `HotReloads int64` - 핫 리로드 횟수
- `ScriptStats map[string]*ScriptStats` - 스크립트별 통계

#### REQ-SCRIPT-001-09-03 (Ubiquitous) ScriptStats 개별 통계

시스템은 **항상** 각 스크립트별 통계를 추적하는 `ScriptStats` 구조체를 제공해야 한다:

- `ScriptID string` - 스크립트 식별자
- `Executions int64` - 실행 횟수
- `Errors int64` - 에러 횟수
- `AvgExecutionTime time.Duration` - 평균 실행 시간
- `LastExecutedAt time.Time` - 마지막 실행 시각
- `Version int` - 현재 버전
- `CompiledAt time.Time` - 컴파일 시각

#### REQ-SCRIPT-001-09-04 (Ubiquitous) 관찰성 통합

시스템은 **항상** SPEC-OBS-001의 관찰성 시스템에 다음 메트릭을 노출해야 한다:

- 컴포넌트 키: `script`, `script.vm-pool`
- 메트릭: VM 풀 사용률, 실행 시간 히스토그램, 에러율, 핫 리로드 카운터

---

### Module 10: Error Types - 에러 타입 (P0)

파일: `errors.go`

#### REQ-SCRIPT-001-10-01 (Ubiquitous) 표준 에러 변수

시스템은 **항상** 다음 에러 변수를 제공해야 한다:

| 에러 변수 | 용도 |
|-----------|------|
| `ErrVMPoolExhausted` | 모든 VM이 사용 중이어서 획득 불가 |
| `ErrScriptCompileFailed` | 스크립트 소스 컴파일 실패 (문법 오류 등) |
| `ErrScriptExecutionFailed` | 스크립트 실행 중 런타임 에러 발생 |
| `ErrScriptTimeout` | 스크립트 실행이 타임아웃 초과 |
| `ErrSandboxViolation` | 샌드박스 제한 위반 (차단된 함수 호출 등) |
| `ErrScriptNotFound` | 등록되지 않은 scriptID 또는 존재하지 않는 스크립트 소스 |
| `ErrInvalidScriptSource` | 스크립트 소스 유효성 검증 실패 |
| `ErrHotReloadFailed` | 핫 리로드 실패 (롤백 버전 없음 등) |

#### REQ-SCRIPT-001-10-02 (Ubiquitous) 에러 래핑 지원

시스템은 **항상** 모든 에러 변수가 `errors.Is()` 및 `errors.As()`와 호환되도록 sentinel error 패턴을 사용해야 한다.

#### REQ-SCRIPT-001-10-03 (Ubiquitous) 상세 에러 구조체

시스템은 **항상** `ScriptError` 구조체를 제공하여 상세한 에러 정보를 포함해야 한다:

- `Err error` - 기본 에러 (sentinel error)
- `ScriptID string` - 관련 스크립트 ID
- `Line int` - 에러 발생 줄 번호 (해당 시)
- `Detail string` - 상세 에러 메시지

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
internal/script/
  engine.go              # ScriptEngine 인터페이스, 구현, VMPool, 컴파일 캐시
  sandbox.go             # SandboxConfig, ApplySandbox, 위험 함수 차단
  loader.go              # ScriptSource, SourceType, LoadScript
  hotreload.go           # HotReloadManager, Reload, Rollback, 버전 추적
  stdlib.go              # xflow 표준 라이브러리 (json, log, time, string, math, store, event)
  bridge.go              # ToLuaValue, FromLuaValue, MessageToLuaTable, LuaTableToMessage
  info.go                # ScriptEngineInfo, ScriptEngineStats, ScriptStats
  errors.go              # 에러 변수 및 ScriptError 구조체

  engine_test.go         # ScriptEngine 통합 테스트
  sandbox_test.go        # 샌드박스 환경 테스트
  loader_test.go         # 스크립트 로더 테스트
  hotreload_test.go      # 핫 리로드 테스트
  stdlib_test.go         # 표준 라이브러리 테스트
  bridge_test.go         # Go-Lua 브릿지 테스트
  info_test.go           # 정보/통계 테스트
  errors_test.go         # 에러 타입 테스트
```

### 4.2 타입 시그니처

```go
// ===== engine.go =====

// ScriptEngine - 스크립트 엔진 인터페이스
type ScriptEngine interface {
    lifecycle.Lifecycle
    lifecycle.Configurable

    Execute(ctx context.Context, scriptID string, input any) (any, error)
    Compile(ctx context.Context, source ScriptSource) (string, error)
    Remove(scriptID string) error
    Stats() ScriptEngineStats
    Info() ScriptEngineInfo
}

// DefaultScriptEngine - 기본 구현
type DefaultScriptEngine struct {
    *lifecycle.BaseLifecycle  // 임베딩
    // unexported fields
}

func NewScriptEngine(opts ...EngineOption) *DefaultScriptEngine
func (e *DefaultScriptEngine) Init(ctx context.Context) error
func (e *DefaultScriptEngine) Shutdown(ctx context.Context) error
func (e *DefaultScriptEngine) Execute(ctx context.Context, scriptID string, input any) (any, error)
func (e *DefaultScriptEngine) Compile(ctx context.Context, source ScriptSource) (string, error)
func (e *DefaultScriptEngine) Remove(scriptID string) error
func (e *DefaultScriptEngine) Stats() ScriptEngineStats
func (e *DefaultScriptEngine) Info() ScriptEngineInfo

// EngineOption - 엔진 옵션
type EngineOption func(*DefaultScriptEngine)

func WithPoolSize(size int) EngineOption
func WithMaxExecutionTime(d time.Duration) EngineOption
func WithSandboxConfig(cfg SandboxConfig) EngineOption
func WithMaxVMUses(n int) EngineOption
func WithStoreAgent(store StoreAccessor) EngineOption
func WithLogger(logger *slog.Logger) EngineOption

// StoreAccessor - Store Agent 접근 인터페이스 (순환 의존 방지)
type StoreAccessor interface {
    Get(ctx context.Context, key string) (any, error)
    Set(ctx context.Context, key string, value any) error
    Delete(ctx context.Context, key string) error
    Has(ctx context.Context, key string) (bool, error)
}

// VMPool - Lua VM 풀 (internal)
type vmPool struct {
    pool    chan *lua.LState
    size    int
    maxUses int
    sandbox SandboxConfig
    stdlib  StdlibOptions
    // unexported fields
}

// ===== sandbox.go =====

// SandboxConfig - 샌드박스 설정
type SandboxConfig struct {
    Enabled           bool
    DisabledModules   []string
    DisabledFunctions []string
    MaxExecutionTime  time.Duration
    MaxMemoryMB       int
    AllowDynamicLoad  bool
}

func DefaultSandboxConfig() SandboxConfig
func ApplySandbox(L *lua.LState, config SandboxConfig) error

// ===== loader.go =====

// SourceType - 스크립트 소스 유형
type SourceType int

const (
    SourceInline SourceType = iota
    SourceFile
    SourceStore
)

// ScriptSource - 스크립트 소스
type ScriptSource struct {
    Type     SourceType
    Content  string
    Path     string
    StoreKey string
    Name     string
}

// ScriptLoader - 스크립트 로더
type ScriptLoader struct {
    store StoreAccessor // Store Agent 접근 (선택적)
}

func NewScriptLoader(opts ...LoaderOption) *ScriptLoader
func (l *ScriptLoader) LoadScript(ctx context.Context, source ScriptSource) (string, error)

// ===== hotreload.go =====

// HotReloadManager - 핫 리로드 매니저
type HotReloadManager struct {
    // unexported fields
}

func NewHotReloadManager(engine *DefaultScriptEngine) *HotReloadManager
func (h *HotReloadManager) Reload(ctx context.Context, scriptID string, newSource ScriptSource) error
func (h *HotReloadManager) Rollback(ctx context.Context, scriptID string) error
func (h *HotReloadManager) Version(scriptID string) (int, error)
func (h *HotReloadManager) OnReload(callback func(scriptID string, version int, err error))

// ===== stdlib.go =====

// StdlibOptions - 표준 라이브러리 옵션
type StdlibOptions struct {
    EnableJSON   bool
    EnableLog    bool
    EnableTime   bool
    EnableString bool
    EnableMath   bool
    EnableStore  bool
    EnableEvent  bool
}

func DefaultStdlibOptions() StdlibOptions
func RegisterStdlib(L *lua.LState, opts StdlibOptions, deps StdlibDeps) error

// StdlibDeps - 표준 라이브러리 외부 의존성
type StdlibDeps struct {
    Store  StoreAccessor
    Logger *slog.Logger
    // EventEmitter EventEmitter (Optional)
}

// ===== bridge.go =====

func ToLuaValue(L *lua.LState, value any) lua.LValue
func FromLuaValue(value lua.LValue) any
func MessageToLuaTable(L *lua.LState, msg *message.Message) *lua.LTable
func LuaTableToMessage(L *lua.LState, tbl *lua.LTable) (*message.Message, error)
func LuaStringToBytes(value lua.LValue) ([]byte, error)

// ===== info.go =====

type ScriptEngineInfo struct {
    PoolSize         int
    ActiveVMs        int
    IdleVMs          int
    CachedScripts    int
    SandboxEnabled   bool
    MaxExecutionTime time.Duration
    State            string
}

type ScriptEngineStats struct {
    TotalExecutions  int64
    TotalErrors      int64
    TotalTimeouts    int64
    AvgExecutionTime time.Duration
    MaxExecutionTime time.Duration
    VMPoolExhausted  int64
    CacheHits        int64
    CacheMisses      int64
    HotReloads       int64
    ScriptStats      map[string]*ScriptStats
}

type ScriptStats struct {
    ScriptID        string
    Executions      int64
    Errors          int64
    AvgExecutionTime time.Duration
    LastExecutedAt  time.Time
    Version         int
    CompiledAt      time.Time
}

// ===== errors.go =====

var (
    ErrVMPoolExhausted       = errors.New("script: VM pool exhausted")
    ErrScriptCompileFailed   = errors.New("script: compilation failed")
    ErrScriptExecutionFailed = errors.New("script: execution failed")
    ErrScriptTimeout         = errors.New("script: execution timeout exceeded")
    ErrSandboxViolation      = errors.New("script: sandbox violation")
    ErrScriptNotFound        = errors.New("script: script not found")
    ErrInvalidScriptSource   = errors.New("script: invalid script source")
    ErrHotReloadFailed       = errors.New("script: hot reload failed")
)

// ScriptError - 상세 에러 구조체
type ScriptError struct {
    Err      error
    ScriptID string
    Line     int
    Detail   string
}

func (e *ScriptError) Error() string
func (e *ScriptError) Unwrap() error
```

### 4.3 SPEC-LIFE-001과의 관계

ScriptEngine은 `pkg/lifecycle/BaseLifecycle`을 임베딩하여 공통 상태 머신 로직을 재사용한다:

- `Init()` 호출 시 `TransitionTo(StateInitializing)` 후 VM 풀 생성, 캐시 초기화, 성공 시 `TransitionTo(StateRunning)`
- `Pause()` 호출 시 새로운 Execute 요청을 거부, 현재 실행 중인 스크립트는 완료까지 대기
- `Stop()` 호출 시 `TransitionTo(StateStopping)` 후 현재 실행 중인 스크립트 완료 대기, VM 풀 해제, `TransitionTo(StateStopped)`
- `Configure()` 호출 시 Running/Paused 상태에서 `script.timeout` 등 변경 가능

### 4.4 SPEC-NODE-001 Module 9 (ScriptNode) 연동

ScriptNode는 다음 패턴으로 ScriptEngine을 사용한다:

```
ScriptNode.Init():
  scriptID, err := engine.Compile(ctx, scriptSource)
  // scriptID를 내부에 저장

ScriptNode.Process(msg):
  result, err := engine.Execute(ctx, scriptID, msg)
  // result를 다음 노드로 전달

ScriptNode.Configure(newConfig):
  // 핫 리로드: engine.hotReloadManager.Reload(ctx, scriptID, newSource)
```

### 4.5 SPEC-BRIDGE-001 (LuaTransformer) 연동

LuaTransformer는 다음 패턴으로 ScriptEngine을 사용한다:

```
AgentToFlow(rawData []byte):
  result, err := engine.Execute(ctx, scriptID, rawData)
  // result를 Lua table -> message.Message로 변환

FlowToAgent(msg *message.Message):
  result, err := engine.Execute(ctx, scriptID, msg)
  // result를 Lua string -> []byte로 변환
```

### 4.6 구현 우선순위

| 우선순위 | 모듈 | 파일 | 설명 | 상태 |
|---------|------|------|------|------|
| P0 | Module 10: Error Types | errors.go | 에러 변수 및 ScriptError 구조체 | 완료 (eea59aa) |
| P0 | Module 8: Data Bridge | bridge.go | Go-Lua 타입 변환, Message 변환 | 완료 (eea59aa) |
| P0 | Module 4: Sandbox | sandbox.go | 샌드박스 설정, 위험 함수 차단 | 완료 (eea59aa) |
| P0 | Module 1+2+3: Engine Core | engine.go | VM 풀, 컴파일 캐시, ScriptEngine | 완료 (eea59aa) |
| P1 | Module 5: Loader | loader.go | 스크립트 소스 로딩 | 대기 |
| P1 | Module 7: Stdlib | stdlib.go | xflow 내장 라이브러리 | 대기 |
| P1 | Module 6: Hot Reload | hotreload.go | 무중단 스크립트 교체 | 대기 |
| P1 | Module 9: Info & Stats | info.go | 런타임 정보/통계 (기본 Stats/Info는 engine.go에 구현됨) | 대기 |

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 파일 | 우선순위 |
|------------|------|------|---------|
| REQ-SCRIPT-001-01-01 ~ 01-05 | Script Engine Core | engine.go | P0 |
| REQ-SCRIPT-001-02-01 ~ 02-06 | VM Pool | engine.go | P0 |
| REQ-SCRIPT-001-03-01 ~ 03-05 | Compilation & Cache | engine.go | P0 |
| REQ-SCRIPT-001-04-01 ~ 04-05 | Sandbox | sandbox.go | P0 |
| REQ-SCRIPT-001-05-01 ~ 05-06 | Script Loader | loader.go | P1 |
| REQ-SCRIPT-001-06-01 ~ 06-06 | Hot Reload | hotreload.go | P1 |
| REQ-SCRIPT-001-07-01 ~ 07-09 | Stdlib | stdlib.go | P1 |
| REQ-SCRIPT-001-08-01 ~ 08-06 | Data Bridge | bridge.go | P0 |
| REQ-SCRIPT-001-09-01 ~ 09-04 | Info & Stats | info.go | P1 |
| REQ-SCRIPT-001-10-01 ~ 10-03 | Error Types | errors.go | P0 |
