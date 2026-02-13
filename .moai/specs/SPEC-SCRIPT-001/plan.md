---
id: SPEC-SCRIPT-001
type: plan
version: "1.0.0"
spec_ref: SPEC-SCRIPT-001
---

# SPEC-SCRIPT-001 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반): 신규 파일이므로 TDD(RED-GREEN-REFACTOR) 적용
- 모든 파일이 신규 생성이므로 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표
- `go test -race` 필수 실행 (동시성 안전 검증)

### 1.2 기술 스택

- **언어**: Go 1.23+
- **Lua VM**: `github.com/yuin/gopher-lua` v1.1+ (순수 Go, CGo-free)
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성**: `chan *lua.LState` (VM Pool), `sync.RWMutex` (캐시/통계 관리)
- **내부 의존성**: `pkg/lifecycle/` (SPEC-LIFE-001), `pkg/message/` (SPEC-MSG-001), `internal/config/` (SPEC-CFG-001), `internal/observe/` (SPEC-OBS-001)
- **선택적 의존성**: `internal/agent/system/` (SPEC-STORE-001: StoreAccessor 인터페이스)

### 1.3 패키지 위치

- **경로**: `internal/script/`
- **Tier**: internal (비공개 패키지)
- **소비자**: internal/node/ (ScriptNode, SPEC-NODE-001), internal/agent/bridge/ (LuaTransformer, SPEC-BRIDGE-001), internal/engine/ (Flow Runtime)

---

## 2. 마일스톤

### Primary Goal: Error Types + Data Bridge + Sandbox + Engine Core (P0)

**범위**: Module 10, 8, 4, 1+2+3

**작업 항목**:

1. `errors.go` + `errors_test.go` 작성
   - 8개 sentinel error 변수 정의 (`ErrVMPoolExhausted`, `ErrScriptCompileFailed`, `ErrScriptExecutionFailed`, `ErrScriptTimeout`, `ErrSandboxViolation`, `ErrScriptNotFound`, `ErrInvalidScriptSource`, `ErrHotReloadFailed`)
   - `ScriptError` 상세 에러 구조체 (Err, ScriptID, Line, Detail)
   - `errors.Is()` / `Unwrap()` 호환성 테스트

2. `bridge.go` + `bridge_test.go` 작성
   - `ToLuaValue(L, value)` - Go -> Lua 타입 변환 (nil, bool, 정수, 실수, string, []byte, slice, map, struct)
   - `FromLuaValue(value)` - Lua -> Go 타입 변환 (LNil, LBool, LNumber, LString, LTable)
   - `MessageToLuaTable(L, msg)` - Message -> Lua table (payload, metadata 매핑)
   - `LuaTableToMessage(L, tbl)` - Lua table -> Message
   - `LuaStringToBytes(value)` - LString -> []byte
   - 중첩 구조 재귀 변환 테스트
   - struct exported 필드 snake_case 변환 테스트
   - 변환 불가 타입(func, chan, complex) LNil 반환 테스트

3. `sandbox.go` + `sandbox_test.go` 작성
   - `SandboxConfig` 구조체 (Enabled, DisabledModules, DisabledFunctions, MaxExecutionTime, MaxMemoryMB, AllowDynamicLoad)
   - `DefaultSandboxConfig()` 기본값 생성
   - `ApplySandbox(L, config)` - os, io, debug 모듈 제거, loadfile/dofile 제거
   - 차단된 모듈 접근 시 Lua 런타임 에러 발생 테스트
   - `load()` 함수 AllowDynamicLoad 설정에 따른 허용/차단 테스트

4. `engine.go` + `engine_test.go` 작성 (VM Pool + Compile Cache + ScriptEngine)
   - `vmPool` 구조체 (`chan *lua.LState` 기반 고정 크기 풀)
   - VM 획득(Acquire) / 반환(Release) / 재활용(MaxUses 초과 시 폐기)
   - `compiledScript` 내부 구조체 (`*lua.FunctionProto` + 메타데이터)
   - 컴파일 캐시 (`sync.RWMutex` + `map[string]*compiledScript`)
   - `DefaultScriptEngine` 구현 (ScriptEngine 인터페이스)
   - `NewScriptEngine(opts...)` 생성자
   - `EngineOption` 함수들 (WithPoolSize, WithMaxExecutionTime, WithSandboxConfig, WithMaxVMUses, WithStoreAgent, WithLogger)
   - `Init()` - VM 풀 생성, 샌드박스 적용, stdlib 등록
   - `Shutdown()` - VM 풀 해제, 리소스 정리
   - `Execute(ctx, scriptID, input)` - VM 획득 -> FunctionProto 로드 -> 입력 설정 -> 실행 -> 결과 변환 -> VM 반환
   - `Compile(ctx, source)` - 소스 로딩 -> 컴파일 -> 캐시 저장 -> scriptID 반환
   - `Remove(scriptID)` - 캐시 삭제
   - `Stats()`, `Info()` - 통계/정보 반환
   - `BaseLifecycle` 임베딩, `Lifecycle` + `Configurable` 구현
   - 동시성 테스트 (다수 goroutine 동시 Execute)
   - VM 풀 고갈 시 ErrVMPoolExhausted 즉시 반환 테스트
   - VM 재활용 (maxUses 초과) 테스트
   - 컴파일 캐시 히트/미스 테스트
   - context 타임아웃 시 ErrScriptTimeout 테스트
   - 생명주기 전체 흐름 통합 테스트

**산출물**: Script Engine 핵심 기능 완성 (에러 타입, 데이터 브릿지, 샌드박스, VM 풀, 컴파일 캐시, 생명주기)

---

### Secondary Goal: Script Loader + Standard Library (P1)

**범위**: Module 5, 7

**작업 항목**:

1. `loader.go` + `loader_test.go` 작성
   - `SourceType` 열거형 (SourceInline, SourceFile, SourceStore)
   - `ScriptSource` 구조체 (Type, Content, Path, StoreKey, Name)
   - `ScriptLoader` 구조체 (StoreAccessor 선택적 의존성)
   - `NewScriptLoader(opts...)` 생성자
   - `LoadScript(ctx, source)` - 유형별 소스 로딩 (Inline/File/Store)
   - 소스 유효성 검증 (유형별 필수 필드 확인)
   - 인라인 코드 직접 반환 테스트
   - 파일 읽기 테스트 (존재/미존재 경로)
   - Store 키 조회 테스트 (mock StoreAccessor)
   - 유효성 검증 실패 시 ErrInvalidScriptSource 테스트

2. `stdlib.go` + `stdlib_test.go` 작성
   - `StdlibOptions` 구조체 (EnableJSON, EnableLog, EnableTime, EnableString, EnableMath, EnableStore, EnableEvent)
   - `DefaultStdlibOptions()` 기본값 (Event 제외 전부 활성화)
   - `StdlibDeps` 구조체 (Store StoreAccessor, Logger *slog.Logger)
   - `RegisterStdlib(L, opts, deps)` 일괄 등록
   - `xflow` 전역 테이블 생성
   - `xflow.json.encode()` / `xflow.json.decode()` 구현 + 테스트
   - `xflow.log.debug/info/warn/error()` 구현 + 테스트 (slog 연동)
   - `xflow.time.now()` / `now_ms()` / `format()` / `parse()` 구현 + 테스트
   - `xflow.string.trim/split/join/starts_with/ends_with()` 구현 + 테스트
   - `xflow.math.round/clamp/lerp()` 구현 + 테스트
   - `xflow.store.get/set/delete/has()` 구현 + 테스트 (mock StoreAccessor)
   - 개별 모듈 비활성화 테스트

**산출물**: 스크립트 소스 로딩 및 xflow 내장 표준 라이브러리 완성

---

### Final Goal: Hot Reload + Info & Stats (P1)

**범위**: Module 6, 9

**작업 항목**:

1. `hotreload.go` + `hotreload_test.go` 작성
   - `HotReloadManager` 구조체 (engine 참조, 이전 버전 보관)
   - `NewHotReloadManager(engine)` 생성자
   - `Reload(ctx, scriptID, newSource)` - 새 소스 컴파일 -> 원자적 교체 -> 버전 증가
   - `Rollback(ctx, scriptID)` - 직전 버전 복원
   - `Version(scriptID)` - 현재 버전 번호 조회
   - `OnReload(callback)` - 리로드 콜백 등록
   - 리로드 성공 시 다음 Execute부터 새 버전 적용 테스트
   - 리로드 실패(컴파일 에러) 시 기존 버전 유지 테스트
   - 롤백 성공 테스트
   - 이전 버전 없을 때 ErrHotReloadFailed 테스트
   - 버전 번호 자동 증가 테스트

2. `info.go` + `info_test.go` 작성
   - `ScriptEngineInfo` 구조체 (PoolSize, ActiveVMs, IdleVMs, CachedScripts, SandboxEnabled, MaxExecutionTime, State)
   - `ScriptEngineStats` 구조체 (TotalExecutions, TotalErrors, TotalTimeouts, AvgExecutionTime, MaxExecutionTime, VMPoolExhausted, CacheHits, CacheMisses, HotReloads, ScriptStats)
   - `ScriptStats` 구조체 (ScriptID, Executions, Errors, AvgExecutionTime, LastExecutedAt, Version, CompiledAt)
   - 관찰성 통합 (SPEC-OBS-001 컴포넌트 키 "script", "script.vm-pool")
   - 통계 카운터 증가 정확성 테스트
   - 동시 실행 시 통계 데이터 레이스 없음 테스트
   - Info 실시간 반영 테스트 (ActiveVMs, IdleVMs)

**산출물**: 핫 리로드 메커니즘 및 런타임 정보/통계 시스템 완성

---

### Optional Goal: xflow.event 모듈 (P2)

**범위**: REQ-SCRIPT-001-07-08

**작업 항목**:

1. `stdlib.go`에 xflow.event 모듈 추가
   - `xflow.event.emit(name, data)` - 시스템 이벤트 발행
   - `EventEmitter` 인터페이스 정의 (StdlibDeps에 추가)
   - 이벤트 발행 테스트

**산출물**: Lua 스크립트에서 시스템 이벤트 발행 기능

---

## 3. 기술적 접근

### 3.1 VM Pool 설계

`chan *lua.LState`를 사용하여 고정 크기 VM 풀을 구현한다:

- 풀 크기: `script.vm_pool_size` 설정값 (기본 10, IMMUTABLE)
- 버퍼 채널: `make(chan *lua.LState, poolSize)`
- `Acquire()`: 비차단 select로 VM 획득, 실패 시 `ErrVMPoolExhausted` 즉시 반환
- `Release()`: 글로벌 변수 리셋 후 채널에 반환
- VM 사용 횟수 추적: 최대 사용 횟수(기본 1000) 초과 시 폐기 후 새 VM 생성

`chan` 선택 이유:
- 고정 크기 리소스 풀 관리에 Go 채널이 가장 직관적이고 안전
- 비차단 select로 풀 고갈 시 즉시 에러 반환 가능
- 채널 자체가 goroutine-safe하므로 별도 잠금 불필요

### 3.2 컴파일 캐시 전략

`sync.RWMutex` + `map[string]*compiledScript`로 컴파일 캐시를 구현한다:

- 키: scriptID (Compile 시 생성되는 고유 식별자)
- 값: `*compiledScript` (FunctionProto + 소스 해시 + 버전 + 컴파일 시각)
- 읽기(Execute 시 캐시 조회): `RLock()` 사용으로 다수 goroutine 동시 읽기 가능
- 쓰기(Compile/Remove/HotReload): `Lock()` 사용으로 배타적 접근
- FunctionProto 공유: `*lua.FunctionProto`는 불변(immutable)이므로 여러 VM에서 안전하게 공유 가능 (A2 가정)

### 3.3 스크립트 실행 흐름

```
Execute(ctx, scriptID, input)
  1. compiledScripts에서 scriptID로 FunctionProto 조회 (RLock)
  2. vmPool.Acquire()로 VM 획득
  3. VM에 입력 데이터를 전역 변수 "msg"로 설정 (bridge.ToLuaValue 사용)
  4. L.Push(L.NewFunctionFromProto(proto))로 함수 로드
  5. context 타임아웃 연동: L.SetContext(ctx)
  6. L.PCall()로 스크립트 실행
  7. 반환값을 bridge.FromLuaValue로 Go 타입 변환
  8. VM 글로벌 상태 정리
  9. vmPool.Release()로 VM 반환
  10. 통계 업데이트 (실행 횟수, 시간, 에러 등)
```

### 3.4 샌드박스 적용 전략

`ApplySandbox(L, config)` 호출 시:

- `L.SetGlobal("os", lua.LNil)` - os 모듈 제거
- `L.SetGlobal("io", lua.LNil)` - io 모듈 제거
- `L.SetGlobal("debug", lua.LNil)` - debug 모듈 제거
- `L.SetGlobal("loadfile", lua.LNil)` - loadfile 제거
- `L.SetGlobal("dofile", lua.LNil)` - dofile 제거
- `AllowDynamicLoad == false`이면 `L.SetGlobal("load", lua.LNil)` - load 제거
- 추가 DisabledModules/DisabledFunctions에 대해 동일 처리
- GopherLua의 `lua.Options{SkipOpenLibs: true}`로 VM 생성 후 필요한 라이브러리만 선택적 로드 가능

### 3.5 핫 리로드 메커니즘

원자적 교체 패턴:

- `HotReloadManager`는 각 scriptID별 직전 `*compiledScript` 포인터를 보관
- `Reload()` 호출 시: 새 소스 컴파일 -> 기존 포인터를 prev에 저장 -> 캐시에 새 포인터 원자적 교체
- 현재 실행 중인 VM은 이미 FunctionProto를 로드했으므로 영향 없음
- 다음 Execute 호출부터 새 FunctionProto 사용
- `Rollback()` 호출 시: prev 포인터를 현재로 복원 (prev가 없으면 ErrHotReloadFailed)

### 3.6 생명주기 통합

DefaultScriptEngine의 상태 전이:

```
Created -> Init() -> Initializing -> (VM 풀 생성, 캐시 초기화) -> Running
Running -> Pause() -> Paused (새 Execute 거부, 현재 실행 완료 대기)
Paused -> Resume() -> Running
Running/Paused -> Stop() -> Stopping -> (현재 실행 완료 대기, VM 풀 해제) -> Stopped
Error -> Stop() -> Stopping -> Stopped
```

### 3.7 Go struct -> Lua table 변환 전략

reflect 패키지를 사용하여 exported 필드를 snake_case로 변환:

- `reflect.VisibleFields()`로 exported 필드 목록 획득
- 필드명을 CamelCase -> snake_case 변환 (예: `UserName` -> `user_name`)
- `json` struct tag가 있으면 해당 이름 사용
- 재귀적으로 중첩 struct/slice/map 처리

---

## 4. 리스크 및 대응

### Risk 1: GopherLua 실행 시간 제한의 정확성

- **위험**: GopherLua의 `L.SetContext(ctx)`가 모든 Lua 명령어에서 context 취소를 확인하지 않을 수 있어, 무한 루프 등에서 타임아웃이 정확하게 동작하지 않을 가능성
- **대응**: GopherLua는 `SetContext`를 지원하며, 각 Lua 명령어 실행 전 context를 확인함. 추가로 `LState.SetMaxVMCalls()`를 통해 CPU 명령어 수 제한도 병행 설정. 테스트에서 무한 루프 스크립트로 타임아웃 동작 검증

### Risk 2: VM Pool 크기 부족

- **위험**: 동시 스크립트 실행 요청이 VM Pool 크기를 초과하면 `ErrVMPoolExhausted`가 빈번하게 발생
- **대응**: VM Pool 크기는 SPEC-CFG-001에서 IMMUTABLE로 정의. 기본값 10은 대부분의 IoT 시나리오에 충분. Pool 고갈 통계를 `ScriptEngineStats.VMPoolExhausted`에 기록하여 모니터링 가능. 향후 필요 시 동적 풀 크기 조정 기능을 별도 SPEC으로 검토

### Risk 3: 메모리 누수 (Lua VM)

- **위험**: Lua VM 내부 상태가 스크립트 실행 후 완전히 정리되지 않아 메모리 누수 발생 가능
- **대응**: VM 반환 시 글로벌 변수 리셋 수행. VM 최대 사용 횟수(기본 1000)를 설정하여 주기적으로 VM을 폐기하고 새로 생성. GopherLua의 `L.Close()`를 통한 완전 해제. Stopped 상태 전이 시 모든 VM Close 보장

### Risk 4: Go-Lua 타입 변환 성능

- **위험**: 대규모 중첩 데이터 구조(깊은 중첩 map/slice)의 Go-Lua 변환이 느릴 수 있음
- **대응**: 일반적인 IoT 메시지는 2-3레벨 중첩으로 성능 문제 없음. reflect 사용을 최소화하고, 자주 사용되는 `*message.Message` 변환은 별도 최적화 경로 제공. 변환 깊이 제한(기본 10레벨) 옵션 검토

### Risk 5: 핫 리로드 시 경쟁 조건

- **위험**: 핫 리로드 중 동시에 Execute가 호출되면 새/구 버전이 혼재될 수 있음
- **대응**: `sync.RWMutex`로 캐시 접근을 보호. Execute는 `RLock`으로 FunctionProto를 읽은 뒤 VM에 로드하므로, 리로드 시점에 이미 읽은 Execute는 구 버전을 완료. 리로드 후 새 Execute부터 새 버전 적용. 데이터 레이스 없음

### Risk 6: StoreAccessor 순환 의존

- **위험**: `internal/script/`가 `internal/agent/system/` (SPEC-STORE-001)을 직접 import하면 순환 의존 발생 가능
- **대응**: `StoreAccessor` 인터페이스를 `internal/script/` 패키지에 정의하여 의존성 역전(DIP). Store Agent가 이 인터페이스를 구현. `WithStoreAgent(store StoreAccessor)` 옵션으로 주입

### Risk 7: Lua 표준 라이브러리와의 충돌

- **위험**: xflow 내장 라이브러리와 Lua 기본 라이브러리(string, math 등) 간 이름 충돌
- **대응**: xflow 라이브러리는 `xflow.` 접두사 하위에 등록되므로 Lua 기본 라이브러리와 네임스페이스 분리. Lua의 `string`, `math` 라이브러리는 그대로 유지하면서 `xflow.string`, `xflow.math`로 확장 함수만 추가

---

## 5. 의존성 그래프

```
internal/script/ (본 SPEC)
  ├── 의존: github.com/yuin/gopher-lua  (외부: 순수 Go Lua VM)
  ├── 의존: pkg/lifecycle/              (SPEC-LIFE-001: BaseLifecycle, Configurable)
  ├── 의존: pkg/message/                (SPEC-MSG-001: Message 타입, bridge 변환용)
  ├── 의존: internal/config/            (SPEC-CFG-001: 스크립트 엔진 설정 읽기)
  ├── 의존: internal/observe/           (SPEC-OBS-001: 로깅, 메트릭 연동)
  ├── 의존: pkg/xferr/                  (SPEC-ERR-001: ComponentScriptEngine 타입)
  ├── 의존: 표준 라이브러리             (context, errors, sync, time, reflect, encoding/json, fmt, log/slog, path, strings, math, io, crypto/sha256)
  ├── 소비자: internal/node/            (SPEC-NODE-001: ScriptNode가 ScriptEngine.Execute() 호출)
  ├── 소비자: internal/agent/bridge/    (SPEC-BRIDGE-001: LuaTransformer가 ScriptEngine.Execute() 호출)
  ├── 소비자: internal/engine/          (SPEC-ENGINE-001: Flow Runtime에서 ScriptEngine 초기화)
  └── 동료: internal/agent/system/      (SPEC-STORE-001: StoreAccessor 인터페이스를 통한 Store 접근)
```

---

## 6. 구현 순서 (파일별)

| 순서 | 파일 | 설명 | 의존성 |
|------|------|------|--------|
| 1 | errors.go | Sentinel 에러 변수, ScriptError 구조체 | 없음 |
| 2 | bridge.go | Go-Lua 타입 변환, Message 변환 | errors.go, pkg/message/ |
| 3 | sandbox.go | SandboxConfig, ApplySandbox | errors.go, gopher-lua |
| 4 | engine.go | VMPool, 컴파일 캐시, ScriptEngine | errors.go, bridge.go, sandbox.go, pkg/lifecycle/ |
| 5 | loader.go | ScriptSource, ScriptLoader | errors.go |
| 6 | stdlib.go | xflow 표준 라이브러리 | bridge.go, errors.go, gopher-lua |
| 7 | hotreload.go | HotReloadManager | engine.go, errors.go |
| 8 | info.go | ScriptEngineInfo, ScriptEngineStats | engine.go |

모든 파일에 대해 TDD 방식으로 테스트 파일(`*_test.go`)을 먼저 작성한다.
