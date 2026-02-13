---
id: SPEC-SCRIPT-001
type: acceptance
version: "1.0.0"
spec_ref: SPEC-SCRIPT-001
---

# SPEC-SCRIPT-001 수락 기준

## Module 10: Error Types - 에러 타입

### AC-SCRIPT-001-01: Sentinel 에러 변수 정의

```gherkin
Given errors.go가 로드되어 있을 때
Then ErrVMPoolExhausted가 정의되어 있어야 한다
And ErrScriptCompileFailed가 정의되어 있어야 한다
And ErrScriptExecutionFailed가 정의되어 있어야 한다
And ErrScriptTimeout가 정의되어 있어야 한다
And ErrSandboxViolation이 정의되어 있어야 한다
And ErrScriptNotFound가 정의되어 있어야 한다
And ErrInvalidScriptSource가 정의되어 있어야 한다
And ErrHotReloadFailed가 정의되어 있어야 한다
```

### AC-SCRIPT-001-02: errors.Is() 호환성

```gherkin
Given ErrScriptNotFound 에러가 주어졌을 때
When fmt.Errorf("execute failed: %w", ErrScriptNotFound)로 래핑한 후
Then errors.Is(wrappedErr, ErrScriptNotFound)가 true를 반환해야 한다
```

### AC-SCRIPT-001-03: ScriptError 상세 에러 구조체

```gherkin
Given ScriptError{Err: ErrScriptCompileFailed, ScriptID: "test-01", Line: 42, Detail: "unexpected symbol"}가 주어졌을 때
Then Error()가 "script: compilation failed: test-01 (line 42): unexpected symbol" 형태의 문자열을 반환해야 한다
And Unwrap()가 ErrScriptCompileFailed를 반환해야 한다
And errors.Is(scriptErr, ErrScriptCompileFailed)가 true를 반환해야 한다
```

---

## Module 8: Go-Lua Data Bridge - Go-Lua 데이터 브릿지

### AC-SCRIPT-001-04: Go -> Lua 기본 타입 변환

```gherkin
Given Lua LState가 주어졌을 때
When ToLuaValue(L, nil)를 호출하면
Then LNil을 반환해야 한다

When ToLuaValue(L, true)를 호출하면
Then LTrue를 반환해야 한다

When ToLuaValue(L, 42)를 호출하면
Then LNumber(42)를 반환해야 한다

When ToLuaValue(L, 3.14)를 호출하면
Then LNumber(3.14)를 반환해야 한다

When ToLuaValue(L, "hello")를 호출하면
Then LString("hello")를 반환해야 한다

When ToLuaValue(L, []byte{0x48, 0x69})를 호출하면
Then LString("Hi")를 반환해야 한다
```

### AC-SCRIPT-001-05: Go -> Lua 복합 타입 변환

```gherkin
Given Lua LState가 주어졌을 때
When ToLuaValue(L, []any{1, "two", true})를 호출하면
Then LTable을 반환해야 한다
And 테이블[1]이 LNumber(1)이어야 한다
And 테이블[2]이 LString("two")이어야 한다
And 테이블[3]이 LTrue이어야 한다

When ToLuaValue(L, map[string]any{"name": "sensor", "value": 42.5})를 호출하면
Then LTable을 반환해야 한다
And 테이블["name"]이 LString("sensor")이어야 한다
And 테이블["value"]이 LNumber(42.5)이어야 한다
```

### AC-SCRIPT-001-06: Go struct -> Lua table 변환

```gherkin
Given exported 필드를 가진 Go struct가 주어졌을 때
  type Sensor struct {
    DeviceID  string
    TempValue float64
  }
When ToLuaValue(L, Sensor{DeviceID: "s01", TempValue: 25.5})를 호출하면
Then LTable을 반환해야 한다
And 테이블["device_id"]이 LString("s01")이어야 한다
And 테이블["temp_value"]이 LNumber(25.5)이어야 한다
```

### AC-SCRIPT-001-07: Lua -> Go 타입 변환

```gherkin
Given Lua 값이 주어졌을 때
When FromLuaValue(LNil)를 호출하면
Then nil을 반환해야 한다

When FromLuaValue(LTrue)를 호출하면
Then true를 반환해야 한다

When FromLuaValue(LNumber(99))를 호출하면
Then float64(99)를 반환해야 한다

When FromLuaValue(LString("test"))를 호출하면
Then "test"를 반환해야 한다
```

### AC-SCRIPT-001-08: Lua table -> Go 타입 변환

```gherkin
Given 배열 형태의 Lua 테이블 {1, 2, 3}이 주어졌을 때
When FromLuaValue(table)를 호출하면
Then []any{float64(1), float64(2), float64(3)}을 반환해야 한다

Given 해시 형태의 Lua 테이블 {name="test", count=5}가 주어졌을 때
When FromLuaValue(table)를 호출하면
Then map[string]any{"name": "test", "count": float64(5)}를 반환해야 한다
```

### AC-SCRIPT-001-09: Message 양방향 변환

```gherkin
Given message.Message{Payload: map[string]any{"temp": 25.5}, Metadata: map[string]string{"source": "sensor-1"}}가 주어졌을 때
When MessageToLuaTable(L, msg)를 호출하면
Then LTable을 반환해야 한다
And 테이블["payload"]["temp"]가 LNumber(25.5)이어야 한다
And 테이블["metadata"]["source"]가 LString("sensor-1")이어야 한다

Given 위 변환 결과 Lua 테이블이 주어졌을 때
When LuaTableToMessage(L, tbl)를 호출하면
Then 원본 Message와 동일한 Payload와 Metadata를 가진 Message를 반환해야 한다
```

### AC-SCRIPT-001-10: 중첩 구조 재귀 변환

```gherkin
Given map[string]any{"level1": map[string]any{"level2": []any{1, 2, 3}}}가 주어졌을 때
When ToLuaValue(L, value)를 호출한 뒤 FromLuaValue(result)를 호출하면
Then 원본과 동일한 중첩 구조를 반환해야 한다
```

### AC-SCRIPT-001-11: 변환 불가 타입 처리

```gherkin
Given Go 함수 func() {}가 주어졌을 때
When ToLuaValue(L, funcValue)를 호출하면
Then LNil을 반환해야 한다
And 경고 로그가 기록되어야 한다

Given Go 채널 make(chan int)가 주어졌을 때
When ToLuaValue(L, chanValue)를 호출하면
Then LNil을 반환해야 한다
```

### AC-SCRIPT-001-12: []byte 양방향 변환

```gherkin
Given []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F}가 주어졌을 때
When ToLuaValue(L, bytes)를 호출하면
Then LString("Hello")를 반환해야 한다

Given LString("World") Lua 값이 주어졌을 때
When LuaStringToBytes(value)를 호출하면
Then []byte{0x57, 0x6F, 0x72, 0x6C, 0x64}를 반환해야 한다
```

---

## Module 4: Sandbox Environment - 샌드박스 환경

### AC-SCRIPT-001-13: 위험 모듈 차단

```gherkin
Given 샌드박스가 적용된 Lua VM이 주어졌을 때
When Lua 스크립트에서 os.execute("ls")를 호출하면
Then Lua 런타임 에러가 발생해야 한다

When Lua 스크립트에서 io.open("/etc/passwd", "r")를 호출하면
Then Lua 런타임 에러가 발생해야 한다

When Lua 스크립트에서 debug.getinfo(1)를 호출하면
Then Lua 런타임 에러가 발생해야 한다

When Lua 스크립트에서 loadfile("script.lua")를 호출하면
Then Lua 런타임 에러가 발생해야 한다

When Lua 스크립트에서 dofile("script.lua")를 호출하면
Then Lua 런타임 에러가 발생해야 한다
```

### AC-SCRIPT-001-14: SandboxConfig 기본값

```gherkin
Given DefaultSandboxConfig()를 호출할 때
Then Enabled가 true여야 한다
And DisabledModules가 ["os", "io", "debug"]를 포함해야 한다
And DisabledFunctions가 ["loadfile", "dofile"]를 포함해야 한다
And MaxExecutionTime이 5초여야 한다
And MaxMemoryMB가 128이어야 한다
And AllowDynamicLoad가 false여야 한다
```

### AC-SCRIPT-001-15: load() 함수 제어

```gherkin
Given AllowDynamicLoad가 false인 샌드박스가 적용된 VM이 주어졌을 때
When Lua 스크립트에서 load("return 1+1")()를 호출하면
Then Lua 런타임 에러가 발생해야 한다

Given AllowDynamicLoad가 true인 샌드박스가 적용된 VM이 주어졌을 때
When Lua 스크립트에서 load("return 1+1")()를 호출하면
Then 2를 정상 반환해야 한다
```

### AC-SCRIPT-001-16: 실행 시간 제한

```gherkin
Given MaxExecutionTime이 100ms인 샌드박스가 적용된 VM이 주어졌을 때
When 무한 루프 스크립트 "while true do end"를 실행하면
Then 100ms 이내에 ErrScriptTimeout 에러를 반환해야 한다
And VM이 정상적으로 풀에 반환되어야 한다
```

### AC-SCRIPT-001-17: 허용된 기능은 정상 동작

```gherkin
Given 샌드박스가 적용된 Lua VM이 주어졌을 때
When Lua 스크립트에서 string.format("%d", 42)를 호출하면
Then "42"를 정상 반환해야 한다

When Lua 스크립트에서 math.floor(3.7)를 호출하면
Then 3을 정상 반환해야 한다

When Lua 스크립트에서 table.insert(t, 1)를 호출하면
Then 정상 동작해야 한다
```

---

## Module 1+2+3: Script Engine Core - 스크립트 엔진 코어

### AC-SCRIPT-001-18: ScriptEngine 인터페이스 메서드 시그니처

```gherkin
Given ScriptEngine 인터페이스가 정의되어 있을 때
Then Execute(ctx context.Context, scriptID string, input any) (any, error) 메서드가 존재해야 한다
And Compile(ctx context.Context, source ScriptSource) (string, error) 메서드가 존재해야 한다
And Remove(scriptID string) error 메서드가 존재해야 한다
And Stats() ScriptEngineStats 메서드가 존재해야 한다
And Info() ScriptEngineInfo 메서드가 존재해야 한다
And lifecycle.Lifecycle 인터페이스를 만족해야 한다
And lifecycle.Configurable 인터페이스를 만족해야 한다
```

### AC-SCRIPT-001-19: NewScriptEngine 생성자

```gherkin
Given NewScriptEngine()를 호출할 때
Then DefaultScriptEngine 인스턴스를 반환해야 한다
And State()가 lifecycle.StateCreated를 반환해야 한다

Given NewScriptEngine(WithPoolSize(5), WithMaxExecutionTime(3*time.Second))를 호출할 때
Then 생성된 엔진의 VM 풀 크기가 5여야 한다
And 최대 실행 시간이 3초여야 한다
```

### AC-SCRIPT-001-20: Compile 및 Execute 기본 흐름

```gherkin
Given 초기화된 ScriptEngine이 주어졌을 때
When Compile(ctx, ScriptSource{Type: SourceInline, Content: "return msg + 1", Name: "add-one"})를 호출하면
Then scriptID 문자열을 반환해야 한다
And 에러는 nil이어야 한다

When Execute(ctx, scriptID, 41)를 호출하면
Then float64(42)를 반환해야 한다
And 에러는 nil이어야 한다
```

### AC-SCRIPT-001-21: 컴파일 에러 처리

```gherkin
Given 초기화된 ScriptEngine이 주어졌을 때
When Compile(ctx, ScriptSource{Type: SourceInline, Content: "function ??? end", Name: "bad"})를 호출하면
Then ErrScriptCompileFailed 에러를 반환해야 한다
And 에러 메시지에 줄 번호 정보가 포함되어야 한다
```

### AC-SCRIPT-001-22: Remove 후 Execute 실패

```gherkin
Given scriptID "test-01"이 등록된 ScriptEngine이 주어졌을 때
When Remove("test-01")를 호출한 후
Then nil error를 반환해야 한다

When Execute(ctx, "test-01", nil)를 호출하면
Then ErrScriptNotFound 에러를 반환해야 한다
```

### AC-SCRIPT-001-23: 존재하지 않는 scriptID Remove

```gherkin
Given 초기화된 ScriptEngine이 주어졌을 때
When Remove("nonexistent")를 호출하면
Then ErrScriptNotFound 에러를 반환해야 한다
```

### AC-SCRIPT-001-24: VM Pool 고갈

```gherkin
Given PoolSize가 2인 ScriptEngine이 주어졌을 때
When 3개의 goroutine이 동시에 긴 스크립트를 Execute하면
Then 2개는 정상 실행되어야 한다
And 1개는 ErrVMPoolExhausted 에러를 즉시 반환해야 한다 (대기 없음)
```

### AC-SCRIPT-001-25: VM Pool 크기 런타임 변경 금지

```gherkin
Given 초기화된 ScriptEngine이 주어졌을 때
When Configure(ctx, map[string]any{"vm_pool_size": 20})를 시도하면
Then vm_pool_size 변경은 무시되어야 한다 (IMMUTABLE 설정)
And 기존 VM Pool 크기가 유지되어야 한다
```

### AC-SCRIPT-001-26: VM 재활용 (maxUses 초과)

```gherkin
Given MaxVMUses가 3인 ScriptEngine이 주어졌을 때
When 동일 스크립트를 4번 실행하면
Then 4번째 실행 시 내부적으로 기존 VM이 폐기되고 새 VM이 생성되어야 한다
And 4번 모두 정상 결과를 반환해야 한다
```

### AC-SCRIPT-001-27: 컴파일 캐시 히트

```gherkin
Given scriptID "cached-01"이 이미 컴파일된 ScriptEngine이 주어졌을 때
When Execute(ctx, "cached-01", input)를 2번 호출하면
Then Stats().CacheHits가 2 증가해야 한다
And Stats().CacheMisses는 증가하지 않아야 한다
```

### AC-SCRIPT-001-28: 동시성 안전

```gherkin
Given 초기화된 ScriptEngine이 주어졌을 때
When 100개의 goroutine이 동시에 Compile과 Execute를 호출하면
Then race condition이 발생하지 않아야 한다 (go test -race 통과)
And 모든 연산이 정상 완료되거나 적절한 에러를 반환해야 한다
```

### AC-SCRIPT-001-29: context 타임아웃

```gherkin
Given 초기화된 ScriptEngine이 주어졌을 때
When 100ms 타임아웃 context로 "while true do end" 스크립트를 Execute하면
Then ErrScriptTimeout 에러를 반환해야 한다
And 소요 시간이 200ms를 초과하지 않아야 한다
```

### AC-SCRIPT-001-30: 생명주기 상태 전이

```gherkin
Given StateCreated 상태의 ScriptEngine이 주어졌을 때
When Init(ctx)를 호출하면
Then State()가 lifecycle.StateRunning을 반환해야 한다
And VM Pool이 생성되어야 한다

When Pause(ctx)를 호출한 후 Execute를 시도하면
Then 실행이 거부되어야 한다

When Resume(ctx)를 호출한 후 Execute를 시도하면
Then 정상 실행되어야 한다

When Shutdown(ctx)를 호출하면
Then State()가 lifecycle.StateStopped를 반환해야 한다
And VM Pool의 모든 VM이 Close되어야 한다
```

### AC-SCRIPT-001-31: 스크립트 입력 전달

```gherkin
Given 초기화된 ScriptEngine에 "return msg.name" 스크립트가 등록되어 있을 때
When Execute(ctx, scriptID, map[string]any{"name": "xflow"})를 호출하면
Then "xflow"를 반환해야 한다

When Execute(ctx, scriptID, nil)를 호출하면
Then nil을 반환해야 한다 (msg가 nil)
```

---

## Module 5: Script Loader - 스크립트 로더

### AC-SCRIPT-001-32: 인라인 코드 로딩

```gherkin
Given ScriptLoader 인스턴스가 주어졌을 때
When LoadScript(ctx, ScriptSource{Type: SourceInline, Content: "return 42"})를 호출하면
Then "return 42"를 반환해야 한다
And 에러는 nil이어야 한다
```

### AC-SCRIPT-001-33: 파일 로딩 성공

```gherkin
Given "return msg * 2" 내용의 임시 파일이 존재할 때
When LoadScript(ctx, ScriptSource{Type: SourceFile, Path: tempFilePath})를 호출하면
Then "return msg * 2"를 반환해야 한다
And 에러는 nil이어야 한다
```

### AC-SCRIPT-001-34: 파일 로딩 실패

```gherkin
Given 존재하지 않는 경로가 주어졌을 때
When LoadScript(ctx, ScriptSource{Type: SourceFile, Path: "/nonexistent/script.lua"})를 호출하면
Then ErrScriptNotFound 에러를 반환해야 한다
```

### AC-SCRIPT-001-35: Store 로딩

```gherkin
Given Mock StoreAccessor에 키 "scripts/transform"의 값 "return msg + 1"이 설정되어 있을 때
When LoadScript(ctx, ScriptSource{Type: SourceStore, StoreKey: "scripts/transform"})를 호출하면
Then "return msg + 1"를 반환해야 한다

Given Mock StoreAccessor에 키 "scripts/missing"이 존재하지 않을 때
When LoadScript(ctx, ScriptSource{Type: SourceStore, StoreKey: "scripts/missing"})를 호출하면
Then ErrScriptNotFound 에러를 반환해야 한다
```

### AC-SCRIPT-001-36: 소스 유효성 검증

```gherkin
Given SourceInline 유형인데 Content가 빈 문자열인 ScriptSource가 주어졌을 때
When LoadScript(ctx, source)를 호출하면
Then ErrInvalidScriptSource 에러를 반환해야 한다

Given SourceFile 유형인데 Path가 빈 문자열인 ScriptSource가 주어졌을 때
When LoadScript(ctx, source)를 호출하면
Then ErrInvalidScriptSource 에러를 반환해야 한다

Given SourceStore 유형인데 StoreKey가 빈 문자열인 ScriptSource가 주어졌을 때
When LoadScript(ctx, source)를 호출하면
Then ErrInvalidScriptSource 에러를 반환해야 한다
```

---

## Module 7: Built-in Standard Library - 내장 표준 라이브러리

### AC-SCRIPT-001-37: xflow 전역 테이블 등록

```gherkin
Given RegisterStdlib(L, DefaultStdlibOptions(), deps)가 호출된 VM이 주어졌을 때
When Lua 스크립트에서 type(xflow)를 평가하면
Then "table"을 반환해야 한다
And type(xflow.json)이 "table"이어야 한다
And type(xflow.log)이 "table"이어야 한다
And type(xflow.time)이 "table"이어야 한다
And type(xflow.string)이 "table"이어야 한다
And type(xflow.math)이 "table"이어야 한다
And type(xflow.store)이 "table"이어야 한다
```

### AC-SCRIPT-001-38: xflow.json 모듈

```gherkin
Given xflow 표준 라이브러리가 등록된 VM이 주어졌을 때
When xflow.json.encode({name="test", value=42})를 실행하면
Then '{"name":"test","value":42}' 형태의 JSON 문자열을 반환해야 한다

When xflow.json.decode('{"temp":25.5}')를 실행하면
Then Lua 테이블을 반환해야 한다
And result.temp가 25.5여야 한다
```

### AC-SCRIPT-001-39: xflow.log 모듈

```gherkin
Given xflow 표준 라이브러리가 등록된 VM이 주어졌을 때
When xflow.log.info("sensor reading: %d", 42)를 실행하면
Then slog Logger에 INFO 레벨 로그가 기록되어야 한다
And 로그 메시지에 "sensor reading: 42"가 포함되어야 한다

When xflow.log.error("failed")를 실행하면
Then slog Logger에 ERROR 레벨 로그가 기록되어야 한다
```

### AC-SCRIPT-001-40: xflow.time 모듈

```gherkin
Given xflow 표준 라이브러리가 등록된 VM이 주어졌을 때
When xflow.time.now()를 실행하면
Then 현재 Unix 타임스탬프(초, float64)를 반환해야 한다
And 반환값이 현재 시각의 +/- 1초 이내여야 한다

When xflow.time.now_ms()를 실행하면
Then 현재 Unix 타임스탬프(밀리초, integer)를 반환해야 한다

When xflow.time.format(1700000000, "2006-01-02")를 실행하면
Then "2023-11-14" 형태의 문자열을 반환해야 한다

When xflow.time.parse("2023-11-14", "2006-01-02")를 실행하면
Then 1699920000 근처의 Unix 타임스탬프를 반환해야 한다
```

### AC-SCRIPT-001-41: xflow.string 모듈

```gherkin
Given xflow 표준 라이브러리가 등록된 VM이 주어졌을 때
When xflow.string.trim("  hello  ")를 실행하면
Then "hello"를 반환해야 한다

When xflow.string.split("a,b,c", ",")를 실행하면
Then {"a", "b", "c"} 테이블을 반환해야 한다

When xflow.string.join({"x", "y", "z"}, "-")를 실행하면
Then "x-y-z"를 반환해야 한다

When xflow.string.starts_with("hello world", "hello")를 실행하면
Then true를 반환해야 한다

When xflow.string.ends_with("hello world", "world")를 실행하면
Then true를 반환해야 한다
```

### AC-SCRIPT-001-42: xflow.math 모듈

```gherkin
Given xflow 표준 라이브러리가 등록된 VM이 주어졌을 때
When xflow.math.round(3.456, 2)를 실행하면
Then 3.46을 반환해야 한다

When xflow.math.clamp(15, 0, 10)를 실행하면
Then 10을 반환해야 한다

When xflow.math.clamp(-5, 0, 10)를 실행하면
Then 0을 반환해야 한다

When xflow.math.lerp(0, 100, 0.5)를 실행하면
Then 50을 반환해야 한다
```

### AC-SCRIPT-001-43: xflow.store 모듈

```gherkin
Given Mock StoreAccessor가 주입된 xflow 표준 라이브러리가 등록된 VM이 주어졌을 때
When xflow.store.set("key1", "value1")를 실행하면
Then StoreAccessor.Set(ctx, "key1", "value1")가 호출되어야 한다

When StoreAccessor에 "key1" = "value1"이 설정된 후 xflow.store.get("key1")를 실행하면
Then "value1"를 반환해야 한다

When xflow.store.has("key1")를 실행하면
Then true를 반환해야 한다

When xflow.store.delete("key1")를 실행한 후 xflow.store.has("key1")를 실행하면
Then false를 반환해야 한다
```

### AC-SCRIPT-001-44: xflow.store.get 존재하지 않는 키

```gherkin
Given Mock StoreAccessor에 "missing" 키가 존재하지 않을 때
When xflow.store.get("missing")를 실행하면
Then nil을 반환해야 한다 (에러 발생 없음)
```

### AC-SCRIPT-001-45: 개별 모듈 비활성화

```gherkin
Given StdlibOptions{EnableStore: false}로 등록한 VM이 주어졌을 때
When type(xflow.store)를 평가하면
Then "nil"을 반환해야 한다
And type(xflow.json)은 여전히 "table"이어야 한다
```

---

## Module 6: Hot Reload - 핫 리로드

### AC-SCRIPT-001-46: 핫 리로드 성공

```gherkin
Given scriptID "calc"에 "return msg * 2" 스크립트가 등록되어 있을 때
When HotReloadManager.Reload(ctx, "calc", ScriptSource{Type: SourceInline, Content: "return msg * 3"})를 호출하면
Then nil error를 반환해야 한다

When Execute(ctx, "calc", 10)를 호출하면
Then float64(30)를 반환해야 한다 (새 버전 적용)
```

### AC-SCRIPT-001-47: 리로드 실패 시 기존 유지

```gherkin
Given scriptID "safe"에 "return 1" 스크립트가 등록되어 있을 때
When HotReloadManager.Reload(ctx, "safe", ScriptSource{Type: SourceInline, Content: "function ??? end"})를 호출하면
Then ErrScriptCompileFailed 에러를 반환해야 한다

When Execute(ctx, "safe", nil)를 호출하면
Then float64(1)를 반환해야 한다 (기존 버전 유지)
```

### AC-SCRIPT-001-48: 롤백 성공

```gherkin
Given scriptID "rollback-test"가 버전 1("return 1")에서 버전 2("return 2")로 리로드된 상태일 때
When HotReloadManager.Rollback(ctx, "rollback-test")를 호출하면
Then nil error를 반환해야 한다

When Execute(ctx, "rollback-test", nil)를 호출하면
Then float64(1)를 반환해야 한다 (버전 1로 복원)
```

### AC-SCRIPT-001-49: 이전 버전 없을 때 롤백 실패

```gherkin
Given scriptID "new-script"가 처음 컴파일된 직후 (리로드 이력 없음)
When HotReloadManager.Rollback(ctx, "new-script")를 호출하면
Then ErrHotReloadFailed 에러를 반환해야 한다
```

### AC-SCRIPT-001-50: 버전 추적

```gherkin
Given scriptID "versioned"가 등록된 직후
When HotReloadManager.Version("versioned")를 호출하면
Then 1을 반환해야 한다

When Reload를 2번 수행한 후 Version("versioned")를 호출하면
Then 3을 반환해야 한다
```

### AC-SCRIPT-001-51: 리로드 콜백 호출

```gherkin
Given OnReload(callback)로 콜백이 등록된 HotReloadManager가 주어졌을 때
When Reload(ctx, "cb-test", newSource)를 성공적으로 호출하면
Then callback이 scriptID="cb-test", version=2, err=nil로 호출되어야 한다

When 컴파일 실패하는 소스로 Reload를 호출하면
Then callback이 err != nil로 호출되어야 한다
```

---

## Module 9: Script Info & Stats - 스크립트 정보 및 통계

### AC-SCRIPT-001-52: ScriptEngineInfo 실시간 반영

```gherkin
Given PoolSize 5로 초기화된 ScriptEngine이 주어졌을 때
When Info()를 호출하면
Then PoolSize가 5여야 한다
And IdleVMs가 5여야 한다
And ActiveVMs가 0이어야 한다
And State가 "running"이어야 한다

When 2개의 스크립트가 실행 중일 때 Info()를 호출하면
Then ActiveVMs가 2여야 한다
And IdleVMs가 3이어야 한다
```

### AC-SCRIPT-001-53: ScriptEngineStats 통계 누적

```gherkin
Given 초기화된 ScriptEngine이 주어졌을 때
When 스크립트를 3번 성공적으로 Execute하면
Then Stats().TotalExecutions가 3이어야 한다
And Stats().TotalErrors가 0이어야 한다

When 런타임 에러를 발생시키는 스크립트를 1번 Execute하면
Then Stats().TotalErrors가 1이어야 한다
And Stats().TotalExecutions가 4여야 한다
```

### AC-SCRIPT-001-54: ScriptStats 개별 스크립트 통계

```gherkin
Given scriptID "stats-test"가 등록된 ScriptEngine이 주어졌을 때
When "stats-test"를 5번 Execute하면
Then Stats().ScriptStats["stats-test"].Executions가 5여야 한다
And Stats().ScriptStats["stats-test"].LastExecutedAt이 zero value가 아니어야 한다
And Stats().ScriptStats["stats-test"].AvgExecutionTime이 0보다 커야 한다
```

### AC-SCRIPT-001-55: VM Pool 고갈 통계

```gherkin
Given PoolSize 1인 ScriptEngine에서 VM 풀이 고갈된 상태일 때
When ErrVMPoolExhausted가 3번 발생하면
Then Stats().VMPoolExhausted가 3이어야 한다
```

### AC-SCRIPT-001-56: 캐시 히트/미스 통계

```gherkin
Given scriptID "cached"가 등록된 ScriptEngine이 주어졌을 때
When "cached"를 2번 Execute하면
Then Stats().CacheHits가 2여야 한다

When 존재하지 않는 scriptID로 Execute를 시도하면
Then Stats().CacheMisses는 증가하지 않아야 한다 (ErrScriptNotFound로 조기 반환)
```

### AC-SCRIPT-001-57: 동시 실행 시 통계 데이터 레이스 없음

```gherkin
Given 초기화된 ScriptEngine이 주어졌을 때
When 50개의 goroutine이 동시에 Execute와 Stats()를 호출하면
Then race condition이 발생하지 않아야 한다 (go test -race 통과)
And 최종 Stats().TotalExecutions가 실제 성공 실행 횟수와 일치해야 한다
```

---

## 품질 게이트

### Definition of Done

- [ ] 모든 수락 기준(AC-SCRIPT-001-01 ~ 57) 테스트 통과
- [ ] `go test ./internal/script/...` 전체 통과
- [ ] `go test -race ./internal/script/...` 경쟁 상태 없음
- [ ] `go vet ./internal/script/...` 경고 없음
- [ ] 테스트 커버리지 85% 이상 (`go test -cover`)
- [ ] GoDoc 주석 작성 완료 (모든 exported 타입/함수/메서드)
- [ ] `pkg/lifecycle/` 인터페이스 구현 확인 (Lifecycle, Configurable)
- [ ] sentinel error가 `errors.Is()` / `Unwrap()` 호환 확인
- [ ] 샌드박스 위험 모듈 차단 검증 (os, io, debug, loadfile, dofile)
- [ ] VM Pool 고갈 시 즉시 에러 반환 검증 (비차단)
- [ ] 핫 리로드 시 기존 실행 영향 없음 검증
- [ ] Go-Lua 양방향 타입 변환 정확성 검증 (중첩 포함)
- [ ] Message <-> Lua table 양방향 변환 검증
- [ ] xflow 표준 라이브러리 전체 기능 검증
- [ ] 무한 루프 스크립트 타임아웃 검증
- [ ] 동시성 안전 검증 (100+ goroutine Execute)

### 검증 도구

| 도구 | 용도 |
|------|------|
| `go test` | 단위/통합 테스트 실행 |
| `go test -race` | 경쟁 상태 검출 |
| `go test -cover` | 커버리지 측정 |
| `go vet` | 정적 분석 |
| `golangci-lint` | 코드 품질 린팅 |
| `testify/assert` | 테스트 어설션 |
| `testify/require` | 테스트 필수 어설션 (실패 시 즉시 중단) |
