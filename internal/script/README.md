# script - Lua 스크립트 엔진

`internal/script` 패키지는 XFlow 플랫폼의 GopherLua 기반 Lua 스크립트 엔진을 제공한다. chan 기반 VM 풀, sync.Map 컴파일 캐시, 샌드박스 실행 환경, Go-Lua 양방향 데이터 브릿지를 통합 지원한다.

**SPEC**: SPEC-SCRIPT-001

## 아키텍처 개요

```
    스크립트 엔진 구조 (P0 구현)

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
    +----------------+

    +-----------------------------------------------------------+
    |                    Data Bridge                             |
    |  ToLuaValue / FromLuaValue                                |
    |  MessageToLuaTable / LuaTableToMessage                    |
    |  struct -> snake_case table 자동 변환                      |
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
| `Type` | `SourceType` | 소스 유형 (P0에서는 `SourceInline`만 지원) |
| `Content` | `string` | 스크립트 본문 |
| `Name` | `string` | 스크립트 이름 (scriptID 생성에 사용) |

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
| `ErrHotReloadFailed` | script: hot reload failed | 핫 리로드 실패 (P1 예약) |

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

## 파일 구조

```
internal/script/
  engine.go              # ScriptEngine 인터페이스, DefaultScriptEngine, vmPool, 컴파일 캐시
  sandbox.go             # SandboxConfig, DefaultSandboxConfig, ApplySandbox
  bridge.go              # ToLuaValue, FromLuaValue, MessageToLuaTable, LuaTableToMessage
  errors.go              # 센티널 에러 (8개), ScriptError 구조체
  engine_test.go         # ScriptEngine 통합 테스트
  sandbox_test.go        # 샌드박스 환경 테스트
  bridge_test.go         # Go-Lua 브릿지 테스트
  errors_test.go         # 에러 타입 테스트
```

## 의존성

- **외부 의존성**: `github.com/yuin/gopher-lua` v1.1.1 (순수 Go Lua VM)
- **표준 라이브러리**: `sync`, `sync/atomic`, `context`, `crypto/sha256`, `reflect`, `strings`, `unicode`, `time`, `fmt`, `errors`
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

- 테스트: 86개 전체 통과
- 커버리지: 96.9%
- Race Detector: 이상 없음
- Go Vet: 이상 없음

## P1 로드맵

현재 P0 핵심 모듈이 구현 완료되었으며, 다음 P1 모듈이 계획되어 있다:

| 모듈 | 파일 | 설명 |
|------|------|------|
| Module 5: Loader | `loader.go` | 스크립트 소스 로딩 (Inline/File/Store) |
| Module 7: Stdlib | `stdlib.go` | xflow 내장 표준 라이브러리 (json, log, time, string, math, store, event) |
| Module 6: Hot Reload | `hotreload.go` | 무중단 스크립트 교체, 버전 추적, 롤백 |
| Module 9: Info & Stats | `info.go` | 확장 런타임 정보/통계 (스크립트별 통계, 관찰성 통합) |
