---
id: SPEC-PLUGIN-001
type: acceptance
version: "1.0.0"
spec_ref: SPEC-PLUGIN-001
---

# SPEC-PLUGIN-001 수락 기준

## Module 1: Plugin Interface - 플러그인 공통 인터페이스

### AC-PLUGIN-001-01: Plugin 인터페이스 컴파일 검증

```gherkin
Given Plugin 인터페이스가 정의되어 있을 때
Then ID() string 메서드가 포함되어야 한다
And Name() string 메서드가 포함되어야 한다
And Version() string 메서드가 포함되어야 한다
And Type() PluginType 메서드가 포함되어야 한다
And Init(ctx context.Context) error 메서드가 포함되어야 한다
And Start(ctx context.Context) error 메서드가 포함되어야 한다
And Stop(ctx context.Context) error 메서드가 포함되어야 한다
And NodeTypes() []string 메서드가 포함되어야 한다
And CreateNode(typeName string, config map[string]any) (node.Node, error) 메서드가 포함되어야 한다
```

### AC-PLUGIN-001-02: PluginType 상수 검증

```gherkin
Given PluginType 상수가 정의되어 있을 때
Then PluginTypeGoNative 값은 "go_native"이어야 한다
And PluginTypeWASM 값은 "wasm"이어야 한다
```

### AC-PLUGIN-001-03: PluginLoader 인터페이스 컴파일 검증

```gherkin
Given PluginLoader 인터페이스가 정의되어 있을 때
Then Load(ctx context.Context, path string) (Plugin, error) 메서드가 포함되어야 한다
And CanLoad(path string) bool 메서드가 포함되어야 한다
And Type() PluginType 메서드가 포함되어야 한다
```

### AC-PLUGIN-001-04: PluginMetadata 구조체 검증

```gherkin
Given PluginMetadata 구조체가 정의되어 있을 때
Then ID, Name, Version, Author, Description, License 문자열 필드를 포함해야 한다
And NodeTypes []string 필드를 포함해야 한다
And Dependencies []string 필드를 포함해야 한다
And JSON 태그가 올바르게 설정되어야 한다
```

### AC-PLUGIN-001-05: PluginOption 함수형 옵션 검증

```gherkin
Given WithPluginLogger(mockLogger) 옵션이 주어졌을 때
When PluginOption으로 적용하면
Then 내부 logger가 mockLogger와 동일해야 한다

Given WithPluginMetrics(mockMetrics) 옵션이 주어졌을 때
When PluginOption으로 적용하면
Then 내부 metrics가 mockMetrics와 동일해야 한다

Given WithPluginConfig(configMap) 옵션이 주어졌을 때
When PluginOption으로 적용하면
Then 내부 config가 configMap과 동일해야 한다
```

---

## Module 2: Plugin Manager - 플러그인 관리자

### AC-PLUGIN-001-06: Manager 생성자 검증

```gherkin
Given Registry와 PluginLoader 목록이 주어졌을 때
When NewManager(registry, loaders, WithManagerLogger(logger))를 호출하면
Then BaseLifecycle이 StateCreated 상태인 Manager를 반환해야 한다
And 내부 registry가 주어진 Registry와 동일해야 한다
And 내부 loaders가 주어진 목록과 동일해야 한다
```

### AC-PLUGIN-001-07: 플러그인 로드 성공

```gherkin
Given GoPluginLoader가 "test.so" 파일을 로드할 수 있도록 Mock 설정되었을 때
And Registry가 비어 있을 때
When Manager.LoadPlugin(ctx, "test.so")를 호출하면
Then 에러 없이 Plugin 인스턴스를 반환해야 한다
And Plugin.Init(ctx)가 호출되어야 한다
And Registry에 플러그인이 등록되어야 한다
And plugin.loaded 메트릭이 1 증가해야 한다
```

### AC-PLUGIN-001-08: 플러그인 로드 실패 - 로더 없음

```gherkin
Given 어떤 로더도 "test.unknown" 파일을 처리할 수 없을 때
When Manager.LoadPlugin(ctx, "test.unknown")를 호출하면
Then ErrNoLoaderFound 에러를 반환해야 한다
```

### AC-PLUGIN-001-09: 중복 플러그인 로드 방지

```gherkin
Given 이미 "plugin-001" ID를 가진 플러그인이 로드되어 있을 때
When 동일한 "plugin-001" ID를 가진 플러그인 파일을 LoadPlugin으로 로드하면
Then ErrPluginAlreadyLoaded 에러를 반환해야 한다
And Registry에 기존 플러그인이 유지되어야 한다
```

### AC-PLUGIN-001-10: 플러그인 언로드 성공

```gherkin
Given "plugin-001" ID의 플러그인이 Running 상태로 등록되어 있을 때
When Manager.UnloadPlugin(ctx, "plugin-001")를 호출하면
Then Plugin.Stop(ctx)가 호출되어야 한다
And Registry에서 플러그인이 제거되어야 한다
And plugin.unloaded 메트릭이 1 증가해야 한다
```

### AC-PLUGIN-001-11: 존재하지 않는 플러그인 언로드

```gherkin
Given Registry에 "non-existent" ID의 플러그인이 없을 때
When Manager.UnloadPlugin(ctx, "non-existent")를 호출하면
Then ErrPluginNotFound 에러를 반환해야 한다
```

### AC-PLUGIN-001-12: 디렉토리 스캔 로드

```gherkin
Given 플러그인 디렉토리에 "a.so", "b.wasm", "c.txt" 파일이 있을 때
And GoPluginLoader와 WASMPluginLoader가 등록되어 있을 때
When Manager.LoadFromDirectory(ctx, dirPath)를 호출하면
Then "a.so"와 "b.wasm"에 대해 LoadPlugin이 호출되어야 한다
And "c.txt"는 무시되어야 한다
And []LoadResult에 2개의 결과가 포함되어야 한다
```

### AC-PLUGIN-001-13: 디렉토리 스캔 - 부분 실패

```gherkin
Given 플러그인 디렉토리에 "good.so", "bad.wasm" 파일이 있을 때
And "bad.wasm"이 유효하지 않은 WASM 모듈일 때
When Manager.LoadFromDirectory(ctx, dirPath)를 호출하면
Then "good.so"는 성공적으로 로드되어야 한다
And "bad.wasm"의 LoadResult.Err는 nil이 아니어야 한다
And 전체 스캔이 중단되지 않아야 한다
```

### AC-PLUGIN-001-14: Manager 시작/정지 라이프사이클

```gherkin
Given Manager가 Initialized 상태이고 AutoLoad가 true일 때
When Manager.Start(ctx)를 호출하면
Then 플러그인 디렉토리에서 자동 로드가 수행되어야 한다
And 로드된 모든 플러그인의 Start(ctx)가 호출되어야 한다
And Manager 상태가 Running이어야 한다

Given Manager가 Running 상태이고 2개 플러그인이 로드되어 있을 때
When Manager.Stop(ctx)를 호출하면
Then 모든 플러그인의 Stop(ctx)가 역순으로 호출되어야 한다
And Registry가 비어 있어야 한다
And Manager 상태가 Stopped이어야 한다
```

### AC-PLUGIN-001-15: Manager Pause/Resume

```gherkin
Given Manager가 Running 상태이고 2개 플러그인이 Running 상태일 때
When Manager.Pause(ctx)를 호출하면
Then 모든 플러그인의 Pause(ctx)가 호출되어야 한다
And Manager 상태가 Paused이어야 한다

Given Manager가 Paused 상태일 때
When Manager.Resume(ctx)를 호출하면
Then 모든 플러그인의 Resume(ctx)가 호출되어야 한다
And Manager 상태가 Running이어야 한다
```

---

## Module 3: Go Plugin Loader - Go 네이티브 플러그인 로더

### AC-PLUGIN-001-16: GoPluginLoader CanLoad 검증

```gherkin
Given GoPluginLoader가 생성되어 있고 Go.Enabled가 true일 때
When CanLoad("test.so")를 호출하면
Then true를 반환해야 한다

Given GoPluginLoader가 생성되어 있고 Go.Enabled가 true일 때
When CanLoad("test.wasm")를 호출하면
Then false를 반환해야 한다

Given GoPluginLoader가 생성되어 있고 Go.Enabled가 false일 때
When CanLoad("test.so")를 호출하면
Then false를 반환해야 한다
```

### AC-PLUGIN-001-17: GoPluginLoader Type 검증

```gherkin
Given GoPluginLoader가 생성되어 있을 때
When Type()를 호출하면
Then PluginTypeGoNative를 반환해야 한다
```

### AC-PLUGIN-001-18: Go Plugin 심볼 누락 에러

```gherkin
Given .so 파일에 "NewPlugin" 심볼이 없을 때
When GoPluginLoader.Load(ctx, path)를 호출하면
Then ErrPluginSymbolNotFound 에러를 반환해야 한다
And 에러 메시지에 "NewPlugin" 심볼 이름이 포함되어야 한다
```

### AC-PLUGIN-001-19: Go Plugin 타입 불일치 에러

```gherkin
Given .so 파일의 "NewPlugin" 심볼이 PluginFactory 타입과 다를 때
When GoPluginLoader.Load(ctx, path)를 호출하면
Then ErrPluginTypeMismatch 에러를 반환해야 한다
```

---

## Module 4: WASM Plugin Loader - WebAssembly 플러그인 로더

### AC-PLUGIN-001-20: WASMPluginLoader CanLoad 검증

```gherkin
Given WASMPluginLoader가 생성되어 있고 WASM.Enabled가 true일 때
When CanLoad("test.wasm")를 호출하면
Then true를 반환해야 한다

Given WASMPluginLoader가 생성되어 있고 WASM.Enabled가 true일 때
When CanLoad("test.so")를 호출하면
Then false를 반환해야 한다

Given WASMPluginLoader가 생성되어 있고 WASM.Enabled가 false일 때
When CanLoad("test.wasm")를 호출하면
Then false를 반환해야 한다
```

### AC-PLUGIN-001-21: WASMPluginLoader Type 검증

```gherkin
Given WASMPluginLoader가 생성되어 있을 때
When Type()를 호출하면
Then PluginTypeWASM를 반환해야 한다
```

### AC-PLUGIN-001-22: WASM Plugin 로드 성공

```gherkin
Given 유효한 .wasm 파일이 존재하고 필수 익스포트 함수가 정의되어 있을 때
When WASMPluginLoader.Load(ctx, path)를 호출하면
Then WASMPlugin 인스턴스를 반환해야 한다
And WASMPlugin이 Plugin 인터페이스를 구현해야 한다
And WASMPlugin의 BaseLifecycle이 StateCreated 상태여야 한다
```

### AC-PLUGIN-001-23: WASM 익스포트 함수 누락 에러

```gherkin
Given .wasm 모듈에 "plugin_metadata" 함수가 없을 때
When WASMPluginLoader.Load(ctx, path)를 호출하면
Then ErrWASMExportNotFound 에러를 반환해야 한다
And 에러 메시지에 "plugin_metadata" 함수 이름이 포함되어야 한다
```

### AC-PLUGIN-001-24: WASM 컴파일 실패

```gherkin
Given 유효하지 않은 바이트를 포함한 .wasm 파일이 주어졌을 때
When WASMPluginLoader.Load(ctx, path)를 호출하면
Then ErrWASMCompileFailed 에러를 반환해야 한다
```

### AC-PLUGIN-001-25: WASM Host Function 바인딩 검증

```gherkin
Given WASMPluginLoader가 모듈을 로드하고 인스턴스화했을 때
Then host_log 호스트 함수가 바인딩되어야 한다
And host_metric_inc 호스트 함수가 바인딩되어야 한다
And host_get_config 호스트 함수가 바인딩되어야 한다
```

### AC-PLUGIN-001-26: WASM Plugin 리소스 정리

```gherkin
Given WASMPlugin이 Running 상태일 때
When WASMPlugin.Stop(ctx)를 호출하면
Then WASM 모듈 인스턴스가 Close되어야 한다
And 메모리 리소스가 정리되어야 한다
And 컴파일 캐시는 유지되어야 한다
```

---

## Module 5: Plugin Registry - 플러그인 레지스트리

### AC-PLUGIN-001-27: Registry 플러그인 등록 및 조회

```gherkin
Given 빈 Registry가 주어졌을 때
And "plugin-001" ID를 가진 Mock Plugin이 주어졌을 때
When Registry.Register(mockPlugin)를 호출하면
Then 에러 없이 성공해야 한다

When Registry.Get("plugin-001")를 호출하면
Then mockPlugin과 동일한 Plugin을 반환해야 한다
And 에러가 nil이어야 한다
```

### AC-PLUGIN-001-28: Registry 중복 등록 방지

```gherkin
Given "plugin-001" ID의 플러그인이 이미 등록되어 있을 때
When 동일한 "plugin-001" ID를 가진 다른 플러그인을 Register하면
Then ErrPluginAlreadyRegistered 에러를 반환해야 한다
```

### AC-PLUGIN-001-29: Registry 존재하지 않는 플러그인 조회

```gherkin
Given Registry에 "non-existent" ID의 플러그인이 없을 때
When Registry.Get("non-existent")를 호출하면
Then ErrPluginNotFound 에러를 반환해야 한다
```

### AC-PLUGIN-001-30: Registry 플러그인 해제

```gherkin
Given "plugin-001" ID의 플러그인이 등록되어 있을 때
When Registry.Unregister("plugin-001")를 호출하면
Then 에러 없이 성공해야 한다
And Registry.Get("plugin-001")이 ErrPluginNotFound를 반환해야 한다
```

### AC-PLUGIN-001-31: Registry 전체 목록 조회

```gherkin
Given 3개의 플러그인이 등록되어 있을 때
When Registry.List()를 호출하면
Then 3개의 Plugin 슬라이스를 반환해야 한다
And 반환된 슬라이스가 원본 맵의 스냅샷이어야 한다
```

### AC-PLUGIN-001-32: Registry 타입별 필터 조회

```gherkin
Given GoNative 타입 2개, WASM 타입 1개가 등록되어 있을 때
When Registry.ListByType(PluginTypeGoNative)를 호출하면
Then 2개의 Plugin 슬라이스를 반환해야 한다

When Registry.ListByType(PluginTypeWASM)를 호출하면
Then 1개의 Plugin 슬라이스를 반환해야 한다
```

### AC-PLUGIN-001-33: Registry 동시성 안전

```gherkin
Given 빈 Registry가 주어졌을 때
When 10개의 goroutine이 동시에 Register, Get, List, Unregister를 호출하면
Then data race가 발생하지 않아야 한다 (go test -race 통과)
And 모든 작업이 정상 완료되어야 한다
```

---

## Module 6: Error Types - 에러 타입

### AC-PLUGIN-001-34: Sentinel 에러 변수 존재 확인

```gherkin
Given errors.go 파일이 컴파일될 때
Then ErrPluginNotFound 변수가 존재해야 한다
And ErrPluginAlreadyLoaded 변수가 존재해야 한다
And ErrPluginAlreadyRegistered 변수가 존재해야 한다
And ErrPluginLoadFailed 변수가 존재해야 한다
And ErrPluginInitFailed 변수가 존재해야 한다
And ErrPluginStartFailed 변수가 존재해야 한다
And ErrPluginStopFailed 변수가 존재해야 한다
And ErrPluginSymbolNotFound 변수가 존재해야 한다
And ErrPluginTypeMismatch 변수가 존재해야 한다
And ErrWASMExportNotFound 변수가 존재해야 한다
And ErrWASMCompileFailed 변수가 존재해야 한다
And ErrWASMInstantiateFailed 변수가 존재해야 한다
And ErrNoLoaderFound 변수가 존재해야 한다
And ErrPluginDisabled 변수가 존재해야 한다
And ErrSandboxViolation 변수가 존재해야 한다
```

### AC-PLUGIN-001-35: 에러 래핑 패턴 검증

```gherkin
Given 플러그인 로드 중 원본 에러가 발생했을 때
When NewXFlowError(ComponentPlugin, pluginID, cause)로 래핑하면
Then 래핑된 에러의 ComponentType이 ComponentPlugin이어야 한다
And errors.Is()로 원본 에러를 검출할 수 있어야 한다
And errors.Unwrap()으로 원본 에러를 추출할 수 있어야 한다
```

---

## Module 7: Plugin Configuration - 플러그인 설정

### AC-PLUGIN-001-36: 설정 읽기 검증

```gherkin
Given Config.Plugin()이 PluginConfig를 반환하도록 설정되었을 때
When Manager가 초기화될 때
Then Directory 경로를 읽어야 한다
And Go.Enabled 값을 읽어야 한다
And WASM.Enabled 값을 읽어야 한다
And WASM.MaxMemoryMB 값을 읽어야 한다
And WASM.MaxExecutionTimeMs 값을 읽어야 한다
```

### AC-PLUGIN-001-37: 설정 검증 - 기본값 적용

```gherkin
Given WASM.MaxMemoryMB가 0으로 설정되었을 때
When 설정 검증을 수행하면
Then 기본값 64(MB)가 적용되어야 한다

Given WASM.MaxExecutionTimeMs가 0으로 설정되었을 때
When 설정 검증을 수행하면
Then 기본값 5000(ms)가 적용되어야 한다
```

### AC-PLUGIN-001-38: 설정 검증 - 양쪽 비활성화 경고

```gherkin
Given Go.Enabled가 false이고 WASM.Enabled도 false일 때
When 설정 검증을 수행하면
Then 경고 로그가 출력되어야 한다
And 에러를 반환하지 않아야 한다
```

### AC-PLUGIN-001-39: HotReload 설정 변경 연동

```gherkin
Given Go.Enabled가 true이고 Go 플러그인 2개가 로드되어 있을 때
When HotReload로 Go.Enabled가 false로 변경되면
Then 모든 Go 플러그인이 언로드되어야 한다
And 설정 변경 이벤트가 로깅되어야 한다

Given WASM.Enabled가 false일 때
When HotReload로 WASM.Enabled가 true로 변경되면
Then 플러그인 디렉토리에서 .wasm 파일을 재스캔해야 한다
```

---

## Module 8: Sandbox Environment - 샌드박스 환경

### AC-PLUGIN-001-40: SandboxConfig 구조체 검증

```gherkin
Given SandboxConfig가 정의되어 있을 때
Then MaxMemoryBytes uint64 필드를 포함해야 한다
And MaxExecutionTime time.Duration 필드를 포함해야 한다
And MaxFuelLimit uint64 필드를 포함해야 한다
And AllowedHostFunctions []string 필드를 포함해야 한다
```

### AC-PLUGIN-001-41: 메모리 제한 적용

```gherkin
Given MaxMemoryBytes가 64MB로 설정된 SandboxConfig가 주어졌을 때
When WASM 모듈 인스턴스를 생성하면
Then Wazero ModuleConfig에 메모리 제한이 설정되어야 한다
```

### AC-PLUGIN-001-42: 실행 시간 제한 적용

```gherkin
Given MaxExecutionTime이 5초로 설정된 SandboxConfig가 주어졌을 때
When WASM 플러그인 함수를 실행하면
Then 5초 후 context가 취소되어야 한다
And 실행이 중단되어야 한다
```

### AC-PLUGIN-001-43: 리소스 초과 시 ErrSandboxViolation

```gherkin
Given WASM 플러그인이 메모리 제한을 초과하려 할 때
When 메모리 할당이 실패하면
Then ErrSandboxViolation 에러가 반환되어야 한다
And plugin.sandbox.violation 메트릭이 1 증가해야 한다
```

---

## Module 9: Plugin Info & Stats - 플러그인 정보 및 통계

### AC-PLUGIN-001-44: PluginInfo 구조체 검증

```gherkin
Given PluginInfo 구조체가 정의되어 있을 때
Then Metadata PluginMetadata 필드를 포함해야 한다
And State lifecycle.State 필드를 포함해야 한다
And LoadedAt time.Time 필드를 포함해야 한다
And Stats PluginStats 필드를 포함해야 한다
```

### AC-PLUGIN-001-45: PluginStats 구조체 검증

```gherkin
Given PluginStats 구조체가 정의되어 있을 때
Then ProcessedMessages uint64 필드를 포함해야 한다
And FailedMessages uint64 필드를 포함해야 한다
And TotalExecutionTime time.Duration 필드를 포함해야 한다
And AverageExecutionTime time.Duration 필드를 포함해야 한다
And LastExecutionTime time.Time 필드를 포함해야 한다
And MemoryUsageBytes uint64 필드를 포함해야 한다
```

### AC-PLUGIN-001-46: 플러그인 정보 조회

```gherkin
Given "plugin-001" ID의 플러그인이 로드되어 Running 상태일 때
When Manager.GetPluginInfo("plugin-001")를 호출하면
Then PluginInfo의 State가 Running이어야 한다
And PluginInfo의 Metadata.ID가 "plugin-001"이어야 한다
And PluginInfo의 LoadedAt이 zero time이 아니어야 한다
```

### AC-PLUGIN-001-47: 전체 플러그인 상태 리포트

```gherkin
Given 3개의 플러그인이 로드되어 있을 때
When Manager.GetAllPluginInfo()를 호출하면
Then 3개의 PluginInfo 슬라이스를 반환해야 한다
And 각 PluginInfo에 유효한 Metadata와 State가 포함되어야 한다
```

### AC-PLUGIN-001-48: 통계 업데이트 동시성

```gherkin
Given 플러그인 노드가 메시지를 처리하고 있을 때
When 10개의 goroutine이 동시에 통계를 업데이트하면
Then data race가 발생하지 않아야 한다 (go test -race 통과)
And ProcessedMessages가 정확하게 증가해야 한다
```

---

## Module 10: Plugin Marketplace - 플러그인 마켓플레이스

### AC-PLUGIN-001-49: Marketplace 인터페이스 컴파일 검증

```gherkin
Given Marketplace 인터페이스가 정의되어 있을 때
Then Search(ctx, query) ([]PluginMetadata, error) 메서드가 포함되어야 한다
And Install(ctx, pluginID, version) error 메서드가 포함되어야 한다
And Update(ctx, pluginID) error 메서드가 포함되어야 한다
And Uninstall(ctx, pluginID) error 메서드가 포함되어야 한다
```

### AC-PLUGIN-001-50: LocalMarketplace 검색 기능

```gherkin
Given 플러그인 디렉토리에 "sensor.so", "transform.wasm" 파일이 있을 때
When LocalMarketplace.Search(ctx, "sensor")를 호출하면
Then "sensor" 관련 PluginMetadata 슬라이스를 반환해야 한다
```

---

## Quality Gates

### Definition of Done

- [ ] 모든 테스트 통과: `go test -race ./internal/plugin/...`
- [ ] 테스트 커버리지 85% 이상
- [ ] 15개 sentinel error 모두 정의 및 테스트
- [ ] Plugin, PluginLoader 인터페이스 컴파일 검증
- [ ] GoPluginLoader: CanLoad, Type 검증, 에러 처리 테스트
- [ ] WASMPluginLoader: CanLoad, Type 검증, 로드 성공/실패 테스트
- [ ] Registry: CRUD + 동시성 안전 테스트
- [ ] Manager: 로드/언로드/시작/정지/일시정지/재개 라이프사이클 테스트
- [ ] Config: 설정 검증 + HotReload 연동 테스트
- [ ] Sandbox: 리소스 제한 적용 + 위반 감지 테스트
- [ ] Stats: 통계 업데이트 + 동시성 안전 테스트
- [ ] `go vet ./internal/plugin/...` 경고 없음
- [ ] SPEC-LIFE-001 BaseLifecycle 임베딩 패턴 준수
- [ ] SPEC-ERR-001 ComponentPlugin 에러 래핑 패턴 준수
- [ ] SPEC-OBS-001 `plugin.*` 접두사 로깅/메트릭 패턴 준수
