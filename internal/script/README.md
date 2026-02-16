# script - Lua 스크립트 엔진

`internal/script` 패키지는 XFlow 플랫폼의 GopherLua 기반 Lua 스크립트 엔진을 제공한다. chan 기반 VM 풀, sync.Map 컴파일 캐시, 샌드박스 실행 환경, Go-Lua 양방향 데이터 브릿지, 스크립트 로더, xflow 내장 표준 라이브러리, 핫 리로드, 스크립트별 통계 추적을 통합 지원한다.

**SPEC**: SPEC-SCRIPT-001

## 아키텍처 개요

```
    스크립트 엔진 구조

    +-----------------------------------------------------------+
    |                  DefaultScriptEngine                       |
    |  ScriptEngine 인터페이스 구현                                |
    |  Compile / Execute / Remove / Stats / Info                 |
    +-----------------------------------------------------------+
            |                    |                    |
    +-------v--------+  +-------v--------+  +--------v--------+
    |    vmPool       |  |  Compile Cache |  |  ScriptEngine   |
    |  chan 기반       |  |  sync.Map      |  |  Stats          |
    |  비차단 획득     |  |  FunctionProto |  |  atomic.Int64   |
    |  maxUses 재활용  |  |  캐싱          |  |  카운터          |
    +-------+--------+  +----------------+  +-----------------+
            |
    +-------v--------+
    |   ApplySandbox  |  VM 생성 시 자동 적용
    |  os/io/debug    |  위험 모듈 차단
    |  loadfile/dofile|  위험 함수 차단
    +-------+--------+
            |
    +-------v-------------------+
    |   RegisterStdlib          |  xflow 내장 라이브러리
    |  json/log/time/string     |  자동 등록
    |  math/store               |
    +---------------------------+

    +-----------------------------------------------------------+
    |                    Data Bridge                             |
    |  ToLuaValue / FromLuaValue                                |
    |  MessageToLuaTable / LuaTableToMessage                    |
    |  struct -> snake_case table 자동 변환                      |
    +-----------------------------------------------------------+

    +-----------------------------------------------------------+
    |               ScriptLoader                                |
    |  SourceInline / SourceFile / SourceStore                  |
    |  StoreAccessor 인터페이스를 통한 Store 연동                 |
    +-----------------------------------------------------------+

    +-----------------------------------------------------------+
    |              HotReloadManager                             |
    |  Reload / Rollback / Version / OnReload                  |
    |  원자적 FunctionProto 교체, 버전 추적                      |
    +-----------------------------------------------------------+

    +-----------------------------------------------------------+
    |           ScriptStatsTracker                              |
    |  스크립트별 실행/에러/컴파일 통계 추적                       |
    |  ScriptStats / ScriptStatsSnapshot                        |
    +-----------------------------------------------------------+
```

**핵심 구성 요소**:

1. **ScriptEngine 인터페이스**: 5개 메서드 (Execute/Compile/Remove/Stats/Info)
2. **DefaultScriptEngine**: ScriptEngine 구현체, Init/Shutdown 생명주기 지원
3. **vmPool**: chan 기반 비차단 VM 풀 (고정 크기, maxUses 재활용)
4. **Compile Cache**: sync.Map 기반 `*lua.FunctionProto` 캐시
5. **SandboxConfig**: 위험 모듈/함수 차단, 실행 시간/메모리 제한 설정
6. **Data Bridge**: Go-Lua 양방향 타입 변환, Message 특수 변환 지원
7. **ScriptError**: sentinel error 래핑, 줄 번호/상세 정보 포함
8. **ScriptLoader**: Inline/File/Store 3종 소스 로딩, StoreAccessor 연동
9. **RegisterStdlib**: xflow 내장 표준 라이브러리 (json, log, time, string, math, store)
10. **HotReloadManager**: 무중단 스크립트 교체, 버전 추적, 롤백
11. **ScriptStatsTracker**: 스크립트별 실행/에러/컴파일 통계 추적

## 빠른 시작

### ScriptEngine 생성 및 초기화

`NewScriptEngine()` 함수는 Options 패턴으로 설정을 받아 엔진을 생성한다. `Init()`으로 초기화하면 VM 풀이 생성되고 샌드박스가 적용된다.

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/xtra/xflow/internal/script"
)

func main() {
    ctx := context.Background()

    // ScriptEngine 생성 (Options 패턴)
    engine := script.NewScriptEngine(
        script.WithPoolSize(8),                          // VM 풀 크기 (기본: 8)
        script.WithMaxExecutionTime(5 * time.Second),    // 최대 실행 시간 (기본: 5s)
        script.WithMaxVMUses(1000),                      // VM 재활용 횟수 (기본: 0=무제한)
        script.WithSandboxConfig(script.DefaultSandboxConfig()), // 샌드박스 설정
    )

    // 초기화 (VM 풀 생성, 샌드박스 적용)
    if err := engine.Init(ctx); err != nil {
        panic(err)
    }
    defer engine.Shutdown(ctx)
}
```

### 스크립트 컴파일 및 실행

`Compile()`로 스크립트를 등록하고, `Execute()`로 실행한다. 입력 데이터는 Lua 전역 변수 `msg`로 전달된다.

```go
// 스크립트 컴파일
source := script.ScriptSource{
    Type:    script.SourceInline,
    Content: `return msg * 2`,
    Name:    "double",
}
scriptID, err := engine.Compile(ctx, source)
if err != nil {
    panic(err)
}

// 스크립트 실행
result, err := engine.Execute(ctx, scriptID, 21)
fmt.Println(result) // 42

// 스크립트 제거
err = engine.Remove(scriptID)
```

### 엔진 통계 및 정보 조회

```go
// 실행 통계 조회
stats := engine.Stats()
fmt.Println("총 실행:", stats.TotalExecutions.Load())
fmt.Println("총 에러:", stats.TotalErrors.Load())
fmt.Println("캐시 히트:", stats.CacheHits.Load())

// 엔진 정보 조회
info := engine.Info()
fmt.Println("풀 크기:", info.PoolSize)
fmt.Println("활성 VM:", info.ActiveVMs)
fmt.Println("유휴 VM:", info.IdleVMs)
fmt.Println("캐시된 스크립트:", info.CachedScripts)
```

## API 레퍼런스

### ScriptEngine 인터페이스

```go
type ScriptEngine interface {
    Execute(ctx context.Context, scriptID string, input any) (any, error)
    Compile(ctx context.Context, source ScriptSource) (string, error)
    Remove(scriptID string) error
    Stats() *ScriptEngineStats
    Info() ScriptEngineInfo
}
```

### ScriptSource 구조체

| 필드 | 타입 | 설명 |
|------|------|------|
| `Type` | `SourceType` | 소스 유형 (`SourceInline`, `SourceFile`, `SourceStore`) |
| `Content` | `string` | 스크립트 본문 (Inline 시 사용) |
| `Path` | `string` | 파일 경로 (File 시 사용) |
| `StoreKey` | `string` | 스토어 키 (Store 시 사용) |
| `Name` | `string` | 스크립트 이름 (scriptID 생성에 사용) |

### SourceType 열거형

| 상수 | 값 | 설명 |
|------|------|------|
| `SourceInline` | `0` | 인라인 코드 문자열 |
| `SourceFile` | `1` | 파일 시스템 경로에서 로딩 |
| `SourceStore` | `2` | Store Agent를 통한 키-값 조회 |

### ScriptEngineInfo 구조체

| 필드 | 타입 | 설명 |
|------|------|------|
| `PoolSize` | `int` | VM 풀 총 크기 |
| `ActiveVMs` | `int` | 현재 사용 중인 VM 수 |
| `IdleVMs` | `int` | 유휴 VM 수 |
| `CachedScripts` | `int` | 캐시된 스크립트 수 |
| `SandboxEnabled` | `bool` | 샌드박스 활성화 여부 |
| `MaxExecutionTime` | `time.Duration` | 최대 실행 시간 설정 |

### ScriptEngineStats 구조체

| 필드 | 타입 | 설명 |
|------|------|------|
| `TotalExecutions` | `atomic.Int64` | 총 실행 횟수 |
| `TotalErrors` | `atomic.Int64` | 총 에러 횟수 |
| `TotalTimeouts` | `atomic.Int64` | 총 타임아웃 횟수 |
| `VMPoolExhausted` | `atomic.Int64` | VM 풀 고갈 횟수 |
| `CacheHits` | `atomic.Int64` | 캐시 히트 횟수 |
| `CacheMisses` | `atomic.Int64` | 캐시 미스 횟수 |

### EngineOption 옵션

| 옵션 함수 | 기본값 | 설명 |
|-----------|--------|------|
| `WithPoolSize(size)` | `8` | VM 풀 크기 |
| `WithMaxExecutionTime(d)` | `5s` | 스크립트 최대 실행 시간 |
| `WithSandboxConfig(cfg)` | `DefaultSandboxConfig()` | 샌드박스 설정 |
| `WithMaxVMUses(n)` | `0` (무제한) | VM 재사용 횟수 제한 |
| `WithStoreAgent(store)` | `nil` | Store Agent 접근용 StoreAccessor |
| `WithLogger(logger)` | `nil` | slog.Logger 인스턴스 |

### DefaultScriptEngine 메서드

| 메서드 | 설명 |
|--------|------|
| `NewScriptEngine(opts ...EngineOption)` | Options 패턴으로 엔진 생성 |
| `Init(ctx)` | VM 풀 생성 및 캐시 초기화 |
| `Shutdown(ctx)` | 모든 VM 닫기 및 캐시 비우기 |
| `Compile(ctx, source)` | 스크립트 컴파일 및 캐시 저장 (scriptID 반환) |
| `Execute(ctx, scriptID, input)` | 컴파일된 스크립트 실행 (결과 반환) |
| `Remove(scriptID)` | 캐시에서 스크립트 제거 |
| `Stats()` | 엔진 실행 통계 반환 |
| `Info()` | 엔진 현재 상태 정보 반환 |

## 스크립트 로더

### ScriptLoader

`ScriptLoader`는 3종의 소스 유형(Inline, File, Store)에서 스크립트 소스를 로딩하는 통합 로더이다.

```go
// 로더 생성 (Store 연동 옵션)
loader := script.NewScriptLoader(
    script.WithStoreAccessor(myStore),
)

// 인라인 소스 로딩
code, err := loader.LoadScript(ctx, script.ScriptSource{
    Type:    script.SourceInline,
    Content: `return msg * 2`,
})

// 파일 소스 로딩
code, err := loader.LoadScript(ctx, script.ScriptSource{
    Type: script.SourceFile,
    Path: "/path/to/script.lua",
})

// Store 소스 로딩
code, err := loader.LoadScript(ctx, script.ScriptSource{
    Type:     script.SourceStore,
    StoreKey: "scripts/transform",
})
```

### StoreAccessor 인터페이스

Store Agent와의 순환 의존을 방지하기 위해 추상화된 인터페이스이다.

```go
type StoreAccessor interface {
    Get(ctx context.Context, key string) (any, error)
    Set(ctx context.Context, key string, value any) error
    Delete(ctx context.Context, key string) error
    Has(ctx context.Context, key string) (bool, error)
}
```

### LoaderOption

| 옵션 함수 | 설명 |
|-----------|------|
| `WithStoreAccessor(store)` | Store Agent 접근용 StoreAccessor 설정 |

## xflow 내장 표준 라이브러리

### 개요

`RegisterStdlib()`를 통해 Lua 환경에 `xflow` 전역 테이블과 하위 모듈이 등록된다. 각 모듈은 `StdlibOptions`로 개별 활성화/비활성화할 수 있다.

### StdlibOptions 구조체

| 필드 | 타입 | 기본값 | 설명 |
|------|------|--------|------|
| `EnableJSON` | `bool` | `true` | xflow.json 모듈 활성화 |
| `EnableLog` | `bool` | `true` | xflow.log 모듈 활성화 |
| `EnableTime` | `bool` | `true` | xflow.time 모듈 활성화 |
| `EnableString` | `bool` | `true` | xflow.string 모듈 활성화 |
| `EnableMath` | `bool` | `true` | xflow.math 모듈 활성화 |
| `EnableStore` | `bool` | `true` | xflow.store 모듈 활성화 |
| `EnableEvent` | `bool` | `false` | xflow.event 모듈 활성화 (예약) |

### StdlibDeps 구조체

| 필드 | 타입 | 설명 |
|------|------|------|
| `Store` | `StoreAccessor` | xflow.store 모듈의 외부 Store 접근 |
| `Logger` | `LogFunc` | xflow.log 모듈의 로그 출력 함수 |

### xflow.json 모듈

| 함수 | 설명 |
|------|------|
| `xflow.json.encode(value)` | Lua 값을 JSON 문자열로 인코딩 |
| `xflow.json.decode(jsonStr)` | JSON 문자열을 Lua 테이블로 디코딩 |

### xflow.log 모듈

| 함수 | 설명 |
|------|------|
| `xflow.log.debug(msg, ...)` | DEBUG 레벨 로그 출력 |
| `xflow.log.info(msg, ...)` | INFO 레벨 로그 출력 |
| `xflow.log.warn(msg, ...)` | WARN 레벨 로그 출력 |
| `xflow.log.error(msg, ...)` | ERROR 레벨 로그 출력 |

### xflow.time 모듈

| 함수 | 설명 |
|------|------|
| `xflow.time.now()` | 현재 Unix 타임스탬프 (초, float64) 반환 |
| `xflow.time.now_ms()` | 현재 Unix 타임스탬프 (밀리초, integer) 반환 |
| `xflow.time.format(timestamp, layout)` | 타임스탬프를 Go 레이아웃 문자열로 포맷 |
| `xflow.time.parse(str, layout)` | 문자열을 타임스탬프로 파싱 |

### xflow.string 모듈

| 함수 | 설명 |
|------|------|
| `xflow.string.trim(str)` | 앞뒤 공백 제거 |
| `xflow.string.split(str, sep)` | 구분자로 분리하여 테이블 반환 |
| `xflow.string.join(tbl, sep)` | 테이블의 요소를 구분자로 결합 |
| `xflow.string.starts_with(str, prefix)` | 접두사 확인 (boolean) |
| `xflow.string.ends_with(str, suffix)` | 접미사 확인 (boolean) |

### xflow.math 모듈

| 함수 | 설명 |
|------|------|
| `xflow.math.round(num, decimals)` | 소수점 반올림 |
| `xflow.math.clamp(num, min, max)` | 범위 제한 (min 이상 max 이하) |
| `xflow.math.lerp(a, b, t)` | 선형 보간 (a + (b-a)*t) |

### xflow.store 모듈

| 함수 | 설명 |
|------|------|
| `xflow.store.get(key)` | 키로 값 조회 (없으면 nil) |
| `xflow.store.set(key, value)` | 키에 값 저장 |
| `xflow.store.delete(key)` | 키 삭제 |
| `xflow.store.has(key)` | 키 존재 여부 확인 (boolean) |

## 핫 리로드

### HotReloadManager

`HotReloadManager`는 실행 중인 엔진의 스크립트를 무중단으로 교체하고, 버전 추적 및 롤백을 지원한다. `*lua.FunctionProto`를 원자적으로 교체하여 현재 실행 중인 스크립트에는 영향을 주지 않는다.

```go
// HotReloadManager 생성
hrm := script.NewHotReloadManager(engine)

// 리로드 콜백 등록
hrm.OnReload(func(scriptID string, version int, err error) {
    if err != nil {
        fmt.Printf("리로드 실패: %s v%d: %v\n", scriptID, version, err)
    } else {
        fmt.Printf("리로드 성공: %s v%d\n", scriptID, version)
    }
})

// 스크립트 교체 (다음 실행부터 새 버전 적용)
err := hrm.Reload(ctx, scriptID, script.ScriptSource{
    Type:    script.SourceInline,
    Content: `return msg * 3`,
    Name:    "triple",
})

// 현재 버전 조회
version, err := hrm.Version(scriptID)

// 직전 버전으로 롤백
err = hrm.Rollback(ctx, scriptID)
```

### HotReloadManager 메서드

| 메서드 | 설명 |
|--------|------|
| `NewHotReloadManager(engine)` | 엔진에 연결된 HotReloadManager 생성 |
| `Reload(ctx, scriptID, newSource)` | 새 소스로 스크립트 교체 (컴파일 실패 시 기존 유지) |
| `Rollback(ctx, scriptID)` | 직전 버전으로 롤백 (이전 버전 없으면 ErrHotReloadFailed) |
| `Version(scriptID)` | 현재 버전 번호 조회 |
| `OnReload(callback)` | 리로드 성공/실패 콜백 등록 |

### 핫 리로드 특징

- **원자적 교체**: `FunctionProto` 포인터를 atomic으로 교체하여 현재 실행 중인 스크립트에 영향 없음
- **버전 추적**: 리로드마다 버전 번호 자동 증가
- **롤백 지원**: 직전 버전 1개를 보관하여 즉시 롤백 가능
- **컴파일 실패 안전**: 새 소스 컴파일 실패 시 기존 스크립트 유지
- **콜백 알림**: 리로드 성공/실패 이벤트 콜백 지원

## 스크립트 통계 추적

### ScriptStatsTracker

`ScriptStatsTracker`는 스크립트별 실행 횟수, 에러 횟수, 컴파일 정보를 추적한다.

### ScriptStats 구조체

| 필드 | 타입 | 설명 |
|------|------|------|
| `ScriptID` | `string` | 스크립트 식별자 |
| `Executions` | `atomic.Int64` | 실행 횟수 |
| `Errors` | `atomic.Int64` | 에러 횟수 |
| `TotalDuration` | `atomic.Int64` | 총 실행 시간 (나노초) |
| `LastExecutedAt` | `atomic.Int64` | 마지막 실행 시각 (UnixNano) |
| `Version` | `atomic.Int32` | 현재 버전 |
| `CompiledAt` | `atomic.Int64` | 컴파일 시각 (UnixNano) |

### ScriptStatsSnapshot 구조체

`ScriptStats.Snapshot()`으로 특정 시점의 스냅샷을 생성한다.

| 필드 | 타입 | 설명 |
|------|------|------|
| `ScriptID` | `string` | 스크립트 식별자 |
| `Executions` | `int64` | 실행 횟수 |
| `Errors` | `int64` | 에러 횟수 |
| `AvgExecutionTime` | `time.Duration` | 평균 실행 시간 |
| `LastExecutedAt` | `time.Time` | 마지막 실행 시각 |
| `Version` | `int` | 현재 버전 |
| `CompiledAt` | `time.Time` | 컴파일 시각 |

### ScriptStatsTracker 메서드

| 메서드 | 설명 |
|--------|------|
| `NewScriptStatsTracker()` | 새 ScriptStatsTracker 생성 |
| `RecordExecution(scriptID, duration, err)` | 실행 결과 기록 (시간, 에러 여부) |
| `RecordCompile(scriptID, version)` | 컴파일 결과 기록 |
| `GetScriptStats(scriptID)` | 특정 스크립트의 통계 스냅샷 조회 |
| `AllScriptStats()` | 전체 스크립트 통계 스냅샷 맵 조회 |
| `RemoveScript(scriptID)` | 특정 스크립트의 통계 제거 |

## 샌드박스 환경

### SandboxConfig 구조체

| 필드 | 타입 | 기본값 | 설명 |
|------|------|--------|------|
| `Enabled` | `bool` | `true` | 샌드박스 활성화 여부 |
| `DisabledModules` | `[]string` | `["os", "io", "debug"]` | 비활성화할 Lua 모듈 |
| `DisabledFunctions` | `[]string` | `["loadfile", "dofile"]` | 비활성화할 전역 함수 |
| `MaxExecutionTime` | `time.Duration` | `5s` | 최대 실행 시간 |
| `MaxMemoryMB` | `int` | `128` | 최대 메모리 사용량 (MB) |
| `AllowDynamicLoad` | `bool` | `false` | `load()` 함수 허용 여부 |

### 차단 대상

| 차단 대상 | 이유 |
|-----------|------|
| `os` 모듈 | 파일 시스템, 프로세스 조작 방지 |
| `io` 모듈 | 파일 입출력 방지 |
| `debug` 모듈 | 런타임 내부 조작 방지 |
| `loadfile()` | 파일 시스템에서 코드 로딩 방지 |
| `dofile()` | 파일 시스템에서 코드 실행 방지 |
| `load()` | 동적 코드 로딩 제한 (AllowDynamicLoad=false 시) |

## Go-Lua 데이터 브릿지

### 타입 변환 매핑

**Go -> Lua (ToLuaValue)**:

| Go 타입 | Lua 타입 |
|---------|----------|
| `nil` | `LNil` |
| `bool` | `LBool` |
| `int`, `int8`~`int64` | `LNumber` |
| `uint`, `uint8`~`uint64` | `LNumber` |
| `float32`, `float64` | `LNumber` |
| `string` | `LString` |
| `[]byte` | `LString` |
| `[]any` (슬라이스) | `LTable` (배열) |
| `map[string]any` | `LTable` (해시) |
| `struct` (exported fields) | `LTable` (해시, snake_case 키) |
| `message.Message` | `LTable` (특수 변환) |
| 미지원 타입 (func, chan 등) | `LNil` |

**Lua -> Go (FromLuaValue)**:

| Lua 타입 | Go 타입 |
|----------|---------|
| `LNil` | `nil` |
| `LBool` | `bool` |
| `LNumber` | `float64` |
| `LString` | `string` |
| `LTable` (배열) | `[]any` |
| `LTable` (해시) | `map[string]any` |

### 브릿지 함수

| 함수 | 설명 |
|------|------|
| `ToLuaValue(L, value)` | Go 값을 Lua 값으로 변환 |
| `FromLuaValue(value)` | Lua 값을 Go 값으로 변환 |
| `MessageToLuaTable(L, msg)` | Message를 Lua 테이블로 변환 (payload, metadata 매핑) |
| `LuaTableToMessage(L, tbl)` | Lua 테이블을 Message로 변환 |
| `LuaStringToBytes(value)` | LString을 []byte로 변환 |

### struct 변환 규칙

Go 구조체를 Lua 테이블로 변환할 때:
- exported 필드만 변환 (소문자 시작 필드 제외)
- 필드명은 PascalCase/CamelCase에서 snake_case로 자동 변환
- 예시: `UserName` -> `user_name`, `HTTPStatus` -> `http_status`

## 센티널 에러

| 에러 변수 | 메시지 | 발생 조건 |
|-----------|--------|----------|
| `ErrVMPoolExhausted` | script: VM pool exhausted | 모든 VM이 사용 중이어서 획득 불가 |
| `ErrScriptCompileFailed` | script: compile failed | 스크립트 소스 컴파일 실패 (문법 오류 등) |
| `ErrScriptExecutionFailed` | script: execution failed | 스크립트 실행 중 런타임 에러 |
| `ErrScriptTimeout` | script: execution timeout | 실행이 제한 시간 초과 |
| `ErrSandboxViolation` | script: sandbox violation | 샌드박스 정책 위반 |
| `ErrScriptNotFound` | script: script not found | 미등록 scriptID 접근 |
| `ErrInvalidScriptSource` | script: invalid script source | 빈 Content 등 유효성 검증 실패 |
| `ErrHotReloadFailed` | script: hot reload failed | 핫 리로드 실패 (롤백 버전 없음 등) |

### ScriptError 구조체

상세 에러 정보를 포함하는 래핑 구조체이다. `errors.Is()` 및 `errors.As()`와 호환된다.

| 필드 | 타입 | 설명 |
|------|------|------|
| `Err` | `error` | 원본 sentinel error |
| `ScriptID` | `string` | 에러가 발생한 스크립트 ID |
| `Line` | `int` | 에러 발생 줄 번호 (0이면 줄 정보 없음) |
| `Detail` | `string` | 상세 에러 설명 |

에러 메시지 형식:
- 줄 번호 있을 때: `[script:<ScriptID>:<Line>] <Detail>`
- 줄 번호 없을 때: `[script:<ScriptID>] <Detail>`

## 설계 특징

- **chan 기반 VM 풀**: 버퍼드 채널로 비차단 VM 획득/반환, 풀 고갈 시 즉시 에러 반환 (대기 없음)
- **maxUses 재활용**: VM 사용 횟수 제한으로 메모리 누수 방지, 초과 시 자동 폐기/재생성
- **sync.Map 컴파일 캐시**: 동시성 안전한 `*lua.FunctionProto` 캐시, 반복 컴파일 방지
- **context 타임아웃**: `context.WithTimeout`으로 스크립트 실행 시간 제한, 초과 시 자동 중단
- **전역 변수 정리**: Execute 완료 후 `msg` 전역 변수를 nil로 리셋하여 VM 상태 오염 방지
- **재귀적 타입 변환**: 중첩된 map/slice/struct와 Lua 중첩 테이블 간 재귀 변환 지원
- **Options 패턴**: `NewScriptEngine()`에 함수 옵션 패턴 적용
- **CGo-free**: GopherLua는 순수 Go 구현이므로 크로스 컴파일 용이
- **원자적 핫 리로드**: FunctionProto 포인터 atomic 교체로 무중단 스크립트 갱신
- **스크립트별 통계**: atomic 카운터 기반 스크립트별 실행/에러/시간 추적

## 파일 구조

```
internal/script/
  engine.go              # ScriptEngine 인터페이스, DefaultScriptEngine, vmPool, 컴파일 캐시
  sandbox.go             # SandboxConfig, DefaultSandboxConfig, ApplySandbox
  bridge.go              # ToLuaValue, FromLuaValue, MessageToLuaTable, LuaTableToMessage
  errors.go              # 센티널 에러 (8개), ScriptError 구조체
  loader.go              # ScriptLoader, SourceType, LoadScript, StoreAccessor
  stdlib.go              # RegisterStdlib, xflow 내장 라이브러리 (json/log/time/string/math/store)
  hotreload.go           # HotReloadManager, Reload, Rollback, Version, OnReload
  info.go                # ScriptStats, ScriptStatsSnapshot, ScriptStatsTracker
  engine_test.go         # ScriptEngine 통합 테스트
  sandbox_test.go        # 샌드박스 환경 테스트
  bridge_test.go         # Go-Lua 브릿지 테스트
  errors_test.go         # 에러 타입 테스트
  loader_test.go         # 스크립트 로더 테스트
  stdlib_test.go         # 표준 라이브러리 테스트
  hotreload_test.go      # 핫 리로드 테스트
  info_test.go           # 정보/통계 테스트
```

## 의존성

- **외부 의존성**: `github.com/yuin/gopher-lua` v1.1.1 (순수 Go Lua VM)
- **표준 라이브러리**: `sync`, `sync/atomic`, `context`, `crypto/sha256`, `reflect`, `strings`, `unicode`, `time`, `fmt`, `errors`, `os`, `encoding/json`, `math`
- **내부 의존성**:
  - `pkg/message` (SPEC-MSG-001) - Message 타입 양방향 변환 (Data Bridge)

## 테스트

```bash
# 전체 테스트 실행
go test ./internal/script/...

# Race Detector 포함 테스트
go test -race ./internal/script/...

# 커버리지 확인
go test -cover ./internal/script/...

# 상세 커버리지 리포트
go test -coverprofile=cover.out ./internal/script/...
go tool cover -html=cover.out
```

### 테스트 결과

- 테스트: 164개 전체 통과
- 커버리지: 94.7%
- Race Detector: 이상 없음
- Go Vet: 이상 없음
