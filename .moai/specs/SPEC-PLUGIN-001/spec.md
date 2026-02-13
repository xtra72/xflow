---
id: SPEC-PLUGIN-001
version: "1.0.0"
status: draft
created: "2026-02-13"
updated: "2026-02-13"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-13 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-PLUGIN-001: Plugin System - Go 네이티브 플러그인, WASM 확장, 플러그인 레지스트리, 샌드박스 환경

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 확장성을 담당하는 Plugin System을 정의한다. Plugin은 외부에서 제공하는 커스텀 노드 타입, 데이터 변환 로직, 연동 어댑터 등을 런타임에 동적으로 로드하여 플랫폼 기능을 확장하는 메커니즘이다. 본 시스템은 두 가지 플러그인 전략을 동시에 지원한다:

- **Go Native Plugin** (`.so` 파일): Go의 `plugin` 패키지를 활용한 네이티브 공유 라이브러리. Go 개발자에게 친숙하며 네이티브 성능을 제공한다.
- **WASM Plugin** (`.wasm` 파일): Wazero 런타임 기반 WebAssembly 모듈. 언어 무관 확장성과 샌드박스 보안을 제공하며, CGo 의존성이 없는 순수 Go 구현이다.

본 SPEC은 다음을 포함한다:

- **Plugin Interface** (`plugin.go`): 모든 플러그인이 구현하는 공통 인터페이스 및 팩토리 타입 정의
- **Plugin Manager** (`manager.go`): 플러그인 로드/언로드/라이프사이클 총괄 관리자
- **Go Plugin Loader** (`go_loader.go`): `.so` 파일 로딩 및 심볼 검증
- **WASM Plugin Loader** (`wasm_loader.go`): Wazero 기반 `.wasm` 모듈 로딩 및 호스트 함수 바인딩
- **Plugin Registry** (`registry.go`): 로드된 플러그인 목록 관리, 이름/타입 기반 조회
- **Error Types** (`errors.go`): 플러그인 패키지 전용 sentinel 에러 정의
- **Plugin Configuration** (`config.go`): 플러그인 설정 로딩 및 검증, HotReload 연동
- **Sandbox Environment** (`sandbox.go`): WASM 플러그인 리소스 제한 및 격리
- **Plugin Info & Stats** (`stats.go`): 플러그인 메타데이터, 실행 통계, 상태 리포팅
- **Plugin Marketplace** (`marketplace.go`): 플러그인 검색/설치/업데이트 (향후 확장)

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/plugin/`
- **Tier**: internal (비공개 패키지)
- **의존 패키지**:
  - `pkg/lifecycle/` (SPEC-LIFE-001): `Lifecycle`, `BaseLifecycle`, `Configurable`, `State` 임베딩
  - `pkg/xferr/` (SPEC-ERR-001): `ComponentPlugin`, `ErrorMessage`, `StatusEvent`, `NewXFlowError`
  - `pkg/message/` (SPEC-MSG-001): `Message`, `Payload`, `Metadata` 인터페이스
  - `internal/observe/` (SPEC-OBS-001): `Logger`, `Metrics` 인터페이스 (`plugin.*` 접두사)
  - `internal/config/` (SPEC-CFG-001): `Config`, `PluginConfig`, `HotReload` 인터페이스
  - `internal/store/` (SPEC-STORE-001): 플러그인 상태 영속화 (optional)
- **외부 의존성**:
  - `github.com/tetratelabs/wazero` v1.6+: 순수 Go WASM 런타임 (CGo 없음)
- **소비자 패키지**:
  - `internal/node/` (SPEC-NODE-001): 플러그인 기반 커스텀 노드 타입 등록
  - `internal/engine/` (SPEC-ENGINE-001): 플로우 실행 시 플러그인 노드 생성
  - `internal/api/` (SPEC-API-001): 플러그인 관리 REST API
  - `cmd/xflow/` (SPEC-CLI-001): 플러그인 CLI 명령어
  - `internal/auth/` (SPEC-AUTH-001): 플러그인 서명 검증 (향후)

### 1.3 설계 원칙

- **Loader 추상화**: Go Plugin과 WASM Plugin은 동일한 `PluginLoader` 인터페이스를 구현하여 Manager가 로딩 전략을 투명하게 전환할 수 있다
- **Lifecycle 임베딩**: 모든 플러그인은 `BaseLifecycle`을 임베딩하여 `Created -> Initialized -> Running <-> Paused -> Stopped -> Destroyed` 상태 전이를 공유한다
- **안전한 격리**: WASM 플러그인은 Wazero의 샌드박스 환경에서 실행되며, 메모리/CPU/시간 제한을 강제한다
- **Registry 분리**: Plugin Registry는 Node Registry와 독립적으로 동작하되, 플러그인이 제공하는 노드 타입은 Node Registry에 자동 등록된다
- **관찰 가능성**: 모든 플러그인 작업(로드/언로드/실행/에러)은 `plugin.*` 접두사로 로깅 및 메트릭 수집된다
- **설정 기반 활성화**: Go Plugin과 WASM Plugin은 각각 `Config.Plugin().Go.Enabled`, `Config.Plugin().WASM.Enabled` 설정으로 독립 활성화/비활성화된다

### 1.4 범위

**포함**:

- Plugin 공통 인터페이스 및 타입 정의
- Go Native Plugin 로더 (`.so` 파일)
- WASM Plugin 로더 (Wazero, `.wasm` 파일)
- Plugin Manager (로드/언로드/라이프사이클)
- Plugin Registry (등록/조회/해제)
- Plugin Error Types
- Plugin Configuration 및 HotReload 연동
- WASM Sandbox (메모리/CPU/시간 제한)
- Plugin 메타데이터 및 실행 통계

**제외**:

- Node 인터페이스 자체 정의 (SPEC-NODE-001 관할)
- 플로우 실행 엔진 로직 (SPEC-ENGINE-001 관할)
- REST API 엔드포인트 구현 (SPEC-API-001 관할)
- CLI 명령어 구현 (SPEC-CLI-001 관할)
- 플러그인 서명/인증 (SPEC-AUTH-001 관할)
- 실제 Marketplace 서버 구현 (향후 별도 SPEC)

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-LIFE-001 | 의존 | BaseLifecycle 임베딩, State 전이 |
| SPEC-CFG-001 | 의존 | PluginConfig 설정 로딩, HotReload |
| SPEC-OBS-001 | 의존 | Logger, Metrics (`plugin.*` 접두사) |
| SPEC-ERR-001 | 의존 | ComponentPlugin, 에러 타입 |
| SPEC-STORE-001 | 의존 | 플러그인 상태 영속화 (optional) |
| SPEC-MSG-001 | 의존 | Message 인터페이스 (플러그인 노드 처리) |
| SPEC-NODE-001 | 소비자 | 플러그인 기반 커스텀 노드 타입 등록 |
| SPEC-ENGINE-001 | 소비자 | 플로우 실행 시 플러그인 노드 생성 |
| SPEC-API-001 | 소비자 | 플러그인 관리 REST API |
| SPEC-CLI-001 | 소비자 | 플러그인 CLI 명령어 |
| SPEC-AUTH-001 | 소비자 | 플러그인 서명 검증 (향후) |

---

## 2. Assumptions (가정)

### 2.1 플랫폼 가정

- Go 1.23 이상 환경에서 `plugin` 패키지가 Linux/macOS에서 정상 동작한다 (Windows는 Go Plugin 미지원, WASM만 가능)
- Wazero v1.6+ 런타임이 WASI Preview 1을 지원하며, 순수 Go로 빌드된다
- 플러그인 디렉토리(`Config.Plugin().Directory`)가 파일시스템에 존재하며 읽기 권한이 있다

### 2.2 인터페이스 가정

- SPEC-LIFE-001의 `Lifecycle`, `BaseLifecycle`, `Configurable` 인터페이스가 구현 완료되어 사용 가능하다
- SPEC-ERR-001의 `ComponentPlugin` 상수와 에러 래핑 패턴이 정의되어 있다
- SPEC-OBS-001의 `Logger`, `Metrics` 인터페이스가 사용 가능하다
- SPEC-CFG-001의 `Config.Plugin() PluginConfig` 메서드가 구현되어 있다
- SPEC-MSG-001의 `Message` 인터페이스가 정의되어 있다

### 2.3 보안 가정

- Go Native Plugin은 빌드 환경이 동일한 Go 버전으로 컴파일되었음을 전제한다 (Go Plugin 패키지 제약)
- WASM Plugin은 Wazero 샌드박스 내에서만 실행되며, 호스트 자원에 직접 접근할 수 없다
- 플러그인 서명 검증은 SPEC-AUTH-001에서 처리하며, 본 SPEC에서는 로더 레벨 검증만 담당한다

### 2.4 성능 가정

- Go Native Plugin 로딩은 `plugin.Open()` 호출 기준 수십 ms 내 완료된다
- WASM Plugin 컴파일은 초기 로딩 시 수백 ms가 소요될 수 있으나, 컴파일 캐시(`wazero.CompilationCache`)로 재시작 시 최적화된다
- 플러그인 인스턴스 수는 단일 XFlow 인스턴스 기준 수십 개 이내로 예상한다

---

## 3. Requirements (요구사항)

### Module 1: Plugin Interface - 플러그인 공통 인터페이스 (P0)

#### REQ-PLUGIN-001-01-01 (Ubiquitous) Plugin 인터페이스 정의

시스템은 **항상** 모든 플러그인이 구현해야 하는 `Plugin` 인터페이스를 제공해야 한다:

- `ID() string` - 플러그인 고유 식별자
- `Name() string` - 플러그인 표시 이름
- `Version() string` - 플러그인 시맨틱 버전
- `Type() PluginType` - 플러그인 타입 (GoNative / WASM)
- `Init(ctx context.Context) error` - 초기화
- `Start(ctx context.Context) error` - 실행 시작
- `Stop(ctx context.Context) error` - 실행 정지
- `NodeTypes() []string` - 제공하는 노드 타입 이름 목록
- `CreateNode(typeName string, config map[string]any) (node.Node, error)` - 노드 인스턴스 생성

#### REQ-PLUGIN-001-01-02 (Ubiquitous) PluginType 열거 정의

시스템은 **항상** 다음 `PluginType` 상수를 제공해야 한다:

- `PluginTypeGoNative` - Go 네이티브 플러그인 (`.so`)
- `PluginTypeWASM` - WebAssembly 플러그인 (`.wasm`)

#### REQ-PLUGIN-001-01-03 (Ubiquitous) PluginLoader 인터페이스 정의

시스템은 **항상** 플러그인 로딩 전략을 추상화하는 `PluginLoader` 인터페이스를 제공해야 한다:

- `Load(ctx context.Context, path string) (Plugin, error)` - 파일 경로에서 플러그인 로드
- `CanLoad(path string) bool` - 해당 파일을 로드할 수 있는지 판단
- `Type() PluginType` - 이 로더가 처리하는 플러그인 타입

#### REQ-PLUGIN-001-01-04 (Ubiquitous) PluginMetadata 구조체 정의

시스템은 **항상** 플러그인 메타데이터를 담는 `PluginMetadata` 구조체를 제공해야 한다:

- `ID string` - 플러그인 고유 식별자
- `Name string` - 표시 이름
- `Version string` - 시맨틱 버전
- `Author string` - 작성자
- `Description string` - 설명
- `License string` - 라이선스
- `NodeTypes []string` - 제공 노드 타입 목록
- `Dependencies []string` - 의존 플러그인 ID 목록

#### REQ-PLUGIN-001-01-05 (Ubiquitous) PluginFactory 타입 정의

시스템은 **항상** 플러그인 팩토리 함수 타입을 정의해야 한다:

- `type PluginFactory func(metadata PluginMetadata, opts ...PluginOption) (Plugin, error)`
- Go Native Plugin은 `.so` 파일에서 `NewPlugin` 심볼을 `PluginFactory`로 캐스팅하여 사용한다

#### REQ-PLUGIN-001-01-06 (Ubiquitous) PluginOption 함수형 옵션

시스템은 **항상** 플러그인 생성 시 선택적 의존성을 주입하는 `PluginOption` 패턴을 제공해야 한다:

- `WithPluginLogger(logger observe.Logger) PluginOption`
- `WithPluginMetrics(metrics observe.Metrics) PluginOption`
- `WithPluginConfig(config map[string]any) PluginOption`

### Module 2: Plugin Manager - 플러그인 관리자 (P0)

#### REQ-PLUGIN-001-02-01 (Ubiquitous) Manager 구조체 정의

시스템은 **항상** 플러그인 라이프사이클을 총괄하는 `Manager` 구조체를 제공해야 한다:

- `BaseLifecycle` 임베딩
- 내부에 `Registry`, `[]PluginLoader`, `Config`, `Logger`, `Metrics` 보유
- `NewManager(registry *Registry, loaders []PluginLoader, opts ...ManagerOption) *Manager` 생성자

#### REQ-PLUGIN-001-02-02 (Event-Driven) 플러그인 로드

**WHEN** `Manager.LoadPlugin(ctx, path)` 호출 **THEN**:

1. 등록된 `PluginLoader` 중 `CanLoad(path) == true`인 로더를 선택한다
2. 선택된 로더의 `Load(ctx, path)`로 `Plugin` 인스턴스를 생성한다
3. `Plugin.Init(ctx)`를 호출하여 초기화한다
4. `Registry`에 플러그인을 등록한다
5. 플러그인이 제공하는 `NodeTypes()`를 Node Registry에 팩토리로 등록한다
6. `plugin.loaded` 메트릭을 증가시키고 로드 이벤트를 로깅한다
7. 로드된 `Plugin`과 nil 에러를 반환한다

#### REQ-PLUGIN-001-02-03 (Event-Driven) 플러그인 언로드

**WHEN** `Manager.UnloadPlugin(ctx, pluginID)` 호출 **THEN**:

1. `Registry`에서 해당 플러그인을 조회한다
2. 플러그인이 `Running` 상태이면 `Plugin.Stop(ctx)`를 호출한다
3. 플러그인이 등록한 노드 타입을 Node Registry에서 해제한다
4. `Registry`에서 플러그인을 제거한다
5. `plugin.unloaded` 메트릭을 증가시키고 언로드 이벤트를 로깅한다

#### REQ-PLUGIN-001-02-04 (Event-Driven) 디렉토리 스캔 로드

**WHEN** `Manager.LoadFromDirectory(ctx, dirPath)` 호출 **THEN**:

1. 지정 디렉토리에서 `.so` 및 `.wasm` 파일을 스캔한다
2. 각 파일에 대해 `LoadPlugin(ctx, filePath)`를 호출한다
3. 로드 성공/실패 결과를 `[]LoadResult`로 반환한다
4. 개별 플러그인 로드 실패가 전체 스캔을 중단하지 않는다

#### REQ-PLUGIN-001-02-05 (Event-Driven) Manager 시작

**WHEN** `Manager.Start(ctx)` 호출 **THEN**:

1. 설정에서 플러그인 디렉토리 경로를 읽는다 (`Config.Plugin().Directory`)
2. 디렉토리에서 활성화된 타입의 플러그인을 자동 로드한다
3. 로드된 모든 플러그인의 `Plugin.Start(ctx)`를 호출한다
4. Manager 상태를 `Running`으로 전이한다

#### REQ-PLUGIN-001-02-06 (Event-Driven) Manager 정지

**WHEN** `Manager.Stop(ctx)` 호출 **THEN**:

1. 로드된 모든 플러그인의 `Plugin.Stop(ctx)`를 역순으로 호출한다
2. 모든 플러그인을 Registry에서 제거한다
3. WASM 런타임 리소스를 정리한다
4. Manager 상태를 `Stopped`로 전이한다

#### REQ-PLUGIN-001-02-07 (Unwanted) 중복 플러그인 로드 방지

시스템은 동일한 `Plugin.ID()`를 가진 플러그인을 중복 로드**하지 않아야 한다**:

- 이미 등록된 ID로 `LoadPlugin`을 호출하면 `ErrPluginAlreadyLoaded` 에러를 반환한다

#### REQ-PLUGIN-001-02-08 (State-Driven) Manager Pause/Resume

**IF** Manager가 `Running` 상태 **AND WHEN** `Manager.Pause(ctx)` 호출 **THEN**:

- 모든 로드된 플러그인의 `Pause(ctx)`를 호출하여 실행을 일시 정지한다
- Manager 상태를 `Paused`로 전이한다

**IF** Manager가 `Paused` 상태 **AND WHEN** `Manager.Resume(ctx)` 호출 **THEN**:

- 모든 일시 정지된 플러그인의 `Resume(ctx)`를 호출한다
- Manager 상태를 `Running`으로 전이한다

### Module 3: Go Plugin Loader - Go 네이티브 플러그인 로더 (P0)

#### REQ-PLUGIN-001-03-01 (Ubiquitous) GoPluginLoader 구조체 정의

시스템은 **항상** Go 네이티브 플러그인을 로딩하는 `GoPluginLoader` 구조체를 제공해야 한다:

- `PluginLoader` 인터페이스 구현
- `NewGoPluginLoader(opts ...GoLoaderOption) *GoPluginLoader` 생성자

#### REQ-PLUGIN-001-03-02 (Event-Driven) .so 파일 로딩

**WHEN** `GoPluginLoader.Load(ctx, path)` 호출 **THEN**:

1. `plugin.Open(path)`로 `.so` 파일을 로드한다
2. `Lookup("NewPlugin")` 으로 `PluginFactory` 심볼을 조회한다
3. `Lookup("Metadata")` 으로 `*PluginMetadata` 심볼을 조회한다
4. `PluginFactory(metadata, opts...)`로 `Plugin` 인스턴스를 생성한다
5. 생성된 `Plugin`이 `Plugin` 인터페이스를 만족하는지 타입 단언으로 검증한다

#### REQ-PLUGIN-001-03-03 (Event-Driven) CanLoad 확인

**WHEN** `GoPluginLoader.CanLoad(path)` 호출 **THEN**:

1. 파일 확장자가 `.so`인지 확인한다
2. `Config.Plugin().Go.Enabled`가 `true`인지 확인한다
3. 두 조건 모두 만족하면 `true`를 반환한다

#### REQ-PLUGIN-001-03-04 (Unwanted) 심볼 누락 시 에러 처리

시스템은 `.so` 파일에 `NewPlugin` 또는 `Metadata` 심볼이 없으면 로드를 **수행하지 않아야 한다**:

- `ErrPluginSymbolNotFound` 에러를 반환한다
- 에러 메시지에 누락된 심볼 이름과 파일 경로를 포함한다

#### REQ-PLUGIN-001-03-05 (Unwanted) 타입 불일치 에러 처리

시스템은 `NewPlugin` 심볼이 `PluginFactory` 타입과 일치하지 않으면 로드를 **수행하지 않아야 한다**:

- `ErrPluginTypeMismatch` 에러를 반환한다
- 에러 메시지에 기대 타입과 실제 타입 정보를 포함한다

### Module 4: WASM Plugin Loader - WebAssembly 플러그인 로더 (P0)

#### REQ-PLUGIN-001-04-01 (Ubiquitous) WASMPluginLoader 구조체 정의

시스템은 **항상** WASM 플러그인을 로딩하는 `WASMPluginLoader` 구조체를 제공해야 한다:

- `PluginLoader` 인터페이스 구현
- Wazero `wazero.Runtime` 인스턴스 보유
- `wazero.CompilationCache` 활용한 컴파일 캐시
- `NewWASMPluginLoader(runtime wazero.Runtime, opts ...WASMLoaderOption) *WASMPluginLoader` 생성자

#### REQ-PLUGIN-001-04-02 (Event-Driven) .wasm 파일 로딩

**WHEN** `WASMPluginLoader.Load(ctx, path)` 호출 **THEN**:

1. `.wasm` 파일을 바이트 배열로 읽는다
2. `runtime.CompileModule(ctx, wasmBytes)`로 모듈을 컴파일한다
3. 호스트 함수(Host Functions)를 바인딩한다 (로깅, 메트릭 등)
4. `runtime.InstantiateModule(ctx, compiled, config)`로 인스턴스를 생성한다
5. 익스포트된 함수(`plugin_metadata`, `plugin_node_types`, `plugin_create_node`, `plugin_process`)를 검증한다
6. `WASMPlugin` 어댑터를 생성하여 `Plugin` 인터페이스를 구현한다

#### REQ-PLUGIN-001-04-03 (Event-Driven) CanLoad 확인

**WHEN** `WASMPluginLoader.CanLoad(path)` 호출 **THEN**:

1. 파일 확장자가 `.wasm`인지 확인한다
2. `Config.Plugin().WASM.Enabled`가 `true`인지 확인한다
3. 두 조건 모두 만족하면 `true`를 반환한다

#### REQ-PLUGIN-001-04-04 (Ubiquitous) WASMPlugin 어댑터

시스템은 **항상** WASM 모듈 인스턴스를 `Plugin` 인터페이스로 래핑하는 `WASMPlugin` 구조체를 제공해야 한다:

- `BaseLifecycle` 임베딩
- 내부에 `wazero.CompiledModule`, `api.Module` 인스턴스 보유
- WASM 익스포트 함수 호출을 Go 메서드로 매핑한다

#### REQ-PLUGIN-001-04-05 (Ubiquitous) Host Function 바인딩

시스템은 **항상** WASM 모듈에 다음 호스트 함수를 제공해야 한다:

- `host_log(level int32, msg_ptr int32, msg_len int32)` - 로깅
- `host_metric_inc(name_ptr int32, name_len int32, value float64)` - 메트릭 증가
- `host_get_config(key_ptr int32, key_len int32) (val_ptr int32, val_len int32)` - 설정 조회

#### REQ-PLUGIN-001-04-06 (Unwanted) WASM 익스포트 함수 누락 에러

시스템은 `.wasm` 모듈에 필수 익스포트 함수(`plugin_metadata`, `plugin_node_types`, `plugin_create_node`, `plugin_process`)가 없으면 로드를 **수행하지 않아야 한다**:

- `ErrWASMExportNotFound` 에러를 반환한다
- 에러 메시지에 누락된 함수 이름을 포함한다

#### REQ-PLUGIN-001-04-07 (Event-Driven) WASM 리소스 정리

**WHEN** `WASMPlugin.Stop(ctx)` 호출 **THEN**:

1. WASM 모듈 인스턴스를 `Close(ctx)`로 해제한다
2. 컴파일 캐시는 유지한다 (재시작 시 재사용)
3. 메모리 리소스를 정리한다

### Module 5: Plugin Registry - 플러그인 레지스트리 (P0)

#### REQ-PLUGIN-001-05-01 (Ubiquitous) Registry 구조체 정의

시스템은 **항상** 로드된 플러그인을 관리하는 `Registry` 구조체를 제공해야 한다:

- `sync.RWMutex` 기반 동시성 안전 맵
- `NewRegistry() *Registry` 생성자

#### REQ-PLUGIN-001-05-02 (Event-Driven) 플러그인 등록

**WHEN** `Registry.Register(plugin Plugin)` 호출 **THEN**:

1. `plugin.ID()`를 키로 내부 맵에 저장한다
2. 이미 동일 ID가 등록되어 있으면 `ErrPluginAlreadyRegistered` 에러를 반환한다

#### REQ-PLUGIN-001-05-03 (Event-Driven) 플러그인 조회

**WHEN** `Registry.Get(pluginID string)` 호출 **THEN**:

1. 해당 ID의 플러그인을 반환한다
2. 존재하지 않으면 `ErrPluginNotFound` 에러를 반환한다

#### REQ-PLUGIN-001-05-04 (Event-Driven) 플러그인 해제

**WHEN** `Registry.Unregister(pluginID string)` 호출 **THEN**:

1. 해당 ID의 플러그인을 내부 맵에서 제거한다
2. 존재하지 않으면 `ErrPluginNotFound` 에러를 반환한다

#### REQ-PLUGIN-001-05-05 (Event-Driven) 전체 목록 조회

**WHEN** `Registry.List()` 호출 **THEN**:

1. 등록된 모든 플러그인의 스냅샷 슬라이스를 반환한다 (동시성 안전)

#### REQ-PLUGIN-001-05-06 (Event-Driven) 타입별 필터 조회

**WHEN** `Registry.ListByType(pluginType PluginType)` 호출 **THEN**:

1. 지정된 타입의 플러그인만 필터링하여 반환한다

### Module 6: Error Types - 에러 타입 (P0)

#### REQ-PLUGIN-001-06-01 (Ubiquitous) Sentinel 에러 정의

시스템은 **항상** 다음 sentinel 에러 변수를 제공해야 한다:

- `ErrPluginNotFound` - 플러그인 ID 조회 실패
- `ErrPluginAlreadyLoaded` - 동일 ID 플러그인 중복 로드
- `ErrPluginAlreadyRegistered` - 동일 ID 플러그인 중복 등록
- `ErrPluginLoadFailed` - 플러그인 로드 실패 (일반)
- `ErrPluginInitFailed` - 플러그인 초기화 실패
- `ErrPluginStartFailed` - 플러그인 시작 실패
- `ErrPluginStopFailed` - 플러그인 정지 실패
- `ErrPluginSymbolNotFound` - Go Plugin 심볼 미발견
- `ErrPluginTypeMismatch` - Go Plugin 심볼 타입 불일치
- `ErrWASMExportNotFound` - WASM 필수 익스포트 함수 미발견
- `ErrWASMCompileFailed` - WASM 모듈 컴파일 실패
- `ErrWASMInstantiateFailed` - WASM 모듈 인스턴스화 실패
- `ErrNoLoaderFound` - 파일에 매칭되는 로더 없음
- `ErrPluginDisabled` - 해당 플러그인 타입이 설정에서 비활성화됨
- `ErrSandboxViolation` - 샌드박스 리소스 제한 초과

#### REQ-PLUGIN-001-06-02 (Ubiquitous) 에러 래핑 패턴

시스템은 **항상** `pkg/xferr/` 패키지의 `NewXFlowError`를 사용하여 에러를 래핑해야 한다:

- `ComponentType`은 `ComponentPlugin`을 사용한다
- `ComponentID`는 플러그인 ID를 사용한다
- 원본 에러를 `Cause`로 포함한다

### Module 7: Plugin Configuration - 플러그인 설정 (P1)

#### REQ-PLUGIN-001-07-01 (Ubiquitous) PluginConfig 인터페이스 소비

시스템은 **항상** `Config.Plugin() PluginConfig` 메서드의 반환값에서 다음 설정을 읽어야 한다:

- `Directory string` - 플러그인 파일 디렉토리 경로
- `Go.Enabled bool` - Go Native Plugin 활성화 여부
- `WASM.Enabled bool` - WASM Plugin 활성화 여부
- `WASM.MaxMemoryMB int` - WASM 최대 메모리 제한 (MB)
- `WASM.MaxExecutionTimeMs int` - WASM 최대 실행 시간 (ms)
- `AutoLoad bool` - 시작 시 자동 로드 여부

#### REQ-PLUGIN-001-07-02 (Event-Driven) 설정 검증

**WHEN** Manager가 초기화될 때 **THEN**:

1. `Directory` 경로가 존재하고 읽기 가능한지 검증한다
2. `Go.Enabled`와 `WASM.Enabled`가 모두 `false`이면 경고를 로깅한다
3. `WASM.MaxMemoryMB`가 0 이하이면 기본값(64MB)을 적용한다
4. `WASM.MaxExecutionTimeMs`가 0 이하이면 기본값(5000ms)을 적용한다

#### REQ-PLUGIN-001-07-03 (Event-Driven) HotReload 연동

**WHEN** 설정이 HotReload로 변경될 때 **THEN**:

1. `Go.Enabled` 변경 시: 비활성화되면 모든 Go Plugin을 언로드, 활성화되면 디렉토리 재스캔
2. `WASM.Enabled` 변경 시: 비활성화되면 모든 WASM Plugin을 언로드, 활성화되면 디렉토리 재스캔
3. `Directory` 변경 시: 기존 플러그인을 모두 언로드하고 새 디렉토리에서 재로드
4. 설정 변경 이벤트를 로깅한다

### Module 8: Sandbox Environment - 샌드박스 환경 (P1)

#### REQ-PLUGIN-001-08-01 (Ubiquitous) SandboxConfig 구조체 정의

시스템은 **항상** WASM 플러그인 리소스 제한을 정의하는 `SandboxConfig` 구조체를 제공해야 한다:

- `MaxMemoryBytes uint64` - 최대 메모리 사용량 (bytes)
- `MaxExecutionTime time.Duration` - 최대 실행 시간
- `MaxFuelLimit uint64` - Wazero fuel-based CPU 제한
- `AllowedHostFunctions []string` - 허용된 호스트 함수 목록

#### REQ-PLUGIN-001-08-02 (Event-Driven) 샌드박스 적용

**WHEN** WASM 모듈 인스턴스 생성 시 **THEN**:

1. `wazero.ModuleConfig`에 메모리 제한을 설정한다
2. Fuel-based execution limit을 설정한다 (Wazero `WithFuel`)
3. `context.WithTimeout`으로 실행 시간 제한을 적용한다
4. 허용된 호스트 함수만 바인딩한다

#### REQ-PLUGIN-001-08-03 (Unwanted) 리소스 초과 시 강제 종료

시스템은 WASM 플러그인이 리소스 제한을 초과하면 실행을 즉시 **중단해야 한다**:

- 메모리 초과: `ErrSandboxViolation` 에러와 함께 모듈 종료
- 시간 초과: context 취소를 통한 실행 중단
- Fuel 소진: Wazero 런타임의 자동 중단 활용
- 모든 위반은 `plugin.sandbox.violation` 메트릭과 경고 로깅

### Module 9: Plugin Info & Stats - 플러그인 정보 및 통계 (P1)

#### REQ-PLUGIN-001-09-01 (Ubiquitous) PluginInfo 구조체 정의

시스템은 **항상** 플러그인 런타임 정보를 담는 `PluginInfo` 구조체를 제공해야 한다:

- `Metadata PluginMetadata` - 정적 메타데이터
- `State lifecycle.State` - 현재 상태
- `LoadedAt time.Time` - 로드 시각
- `Stats PluginStats` - 실행 통계

#### REQ-PLUGIN-001-09-02 (Ubiquitous) PluginStats 구조체 정의

시스템은 **항상** 플러그인 실행 통계를 담는 `PluginStats` 구조체를 제공해야 한다:

- `ProcessedMessages uint64` - 처리한 메시지 수
- `FailedMessages uint64` - 실패한 메시지 수
- `TotalExecutionTime time.Duration` - 총 실행 시간
- `AverageExecutionTime time.Duration` - 평균 실행 시간
- `LastExecutionTime time.Time` - 마지막 실행 시각
- `MemoryUsageBytes uint64` - 메모리 사용량 (WASM 전용)

#### REQ-PLUGIN-001-09-03 (Event-Driven) 플러그인 정보 조회

**WHEN** `Manager.GetPluginInfo(pluginID string)` 호출 **THEN**:

1. Registry에서 플러그인을 조회한다
2. 현재 상태, 메타데이터, 실행 통계를 수집하여 `PluginInfo`를 반환한다

#### REQ-PLUGIN-001-09-04 (Event-Driven) 전체 플러그인 상태 리포트

**WHEN** `Manager.GetAllPluginInfo()` 호출 **THEN**:

1. 모든 등록된 플러그인의 `PluginInfo` 슬라이스를 반환한다
2. 스냅샷 방식으로 동시성 안전하게 수집한다

#### REQ-PLUGIN-001-09-05 (Event-Driven) 통계 업데이트

**WHEN** 플러그인 노드가 메시지를 처리할 때 **THEN**:

1. `ProcessedMessages` 또는 `FailedMessages`를 `atomic` 연산으로 증가시킨다
2. 실행 시간을 `TotalExecutionTime`에 누적한다
3. `AverageExecutionTime`을 재계산한다
4. `LastExecutionTime`을 갱신한다

### Module 10: Plugin Marketplace - 플러그인 마켓플레이스 (P2)

#### REQ-PLUGIN-001-10-01 (Optional) Marketplace 인터페이스 정의

**가능하면** 플러그인 검색 및 설치를 위한 `Marketplace` 인터페이스를 제공한다:

- `Search(ctx context.Context, query string) ([]PluginMetadata, error)` - 플러그인 검색
- `Install(ctx context.Context, pluginID string, version string) error` - 플러그인 설치
- `Update(ctx context.Context, pluginID string) error` - 플러그인 업데이트
- `Uninstall(ctx context.Context, pluginID string) error` - 플러그인 제거

#### REQ-PLUGIN-001-10-02 (Optional) 로컬 Marketplace 구현

**가능하면** 파일시스템 기반 로컬 Marketplace를 제공한다:

- 플러그인 디렉토리 내 파일 스캔으로 검색 기능 구현
- 파일 복사/삭제로 설치/제거 기능 구현
- 원격 Marketplace는 향후 SPEC에서 별도 정의

---

## 4. Specifications (사양)

### 4.1 파일 구조

```
internal/plugin/
  plugin.go           # Plugin, PluginLoader, PluginType, PluginMetadata, PluginFactory, PluginOption
  manager.go          # Manager, ManagerOption, LoadResult
  go_loader.go        # GoPluginLoader, GoLoaderOption
  wasm_loader.go      # WASMPluginLoader, WASMPlugin, WASMLoaderOption
  registry.go         # Registry
  errors.go           # sentinel 에러 변수
  config.go           # PluginConfig 소비, 설정 검증, HotReload 핸들러
  sandbox.go          # SandboxConfig, 리소스 제한 적용
  stats.go            # PluginInfo, PluginStats
  marketplace.go      # Marketplace 인터페이스, LocalMarketplace
  plugin_test.go      # 공통 인터페이스 테스트
  manager_test.go     # Manager 테스트
  go_loader_test.go   # Go Plugin Loader 테스트
  wasm_loader_test.go # WASM Plugin Loader 테스트
  registry_test.go    # Registry 테스트
  config_test.go      # Configuration 테스트
  sandbox_test.go     # Sandbox 테스트
  stats_test.go       # Stats 테스트
  marketplace_test.go # Marketplace 테스트
```

### 4.2 타입 시그니처

```go
// === plugin.go ===

// PluginType 열거
type PluginType string

const (
    PluginTypeGoNative PluginType = "go_native"
    PluginTypeWASM     PluginType = "wasm"
)

// Plugin 인터페이스 - 모든 플러그인이 구현
type Plugin interface {
    lifecycle.Lifecycle
    lifecycle.Configurable

    ID() string
    Name() string
    Version() string
    Type() PluginType
    NodeTypes() []string
    CreateNode(typeName string, config map[string]any) (node.Node, error)
}

// PluginMetadata 구조체
type PluginMetadata struct {
    ID           string   `json:"id"`
    Name         string   `json:"name"`
    Version      string   `json:"version"`
    Author       string   `json:"author"`
    Description  string   `json:"description"`
    License      string   `json:"license"`
    NodeTypes    []string `json:"node_types"`
    Dependencies []string `json:"dependencies"`
}

// PluginFactory 팩토리 함수 타입
type PluginFactory func(metadata PluginMetadata, opts ...PluginOption) (Plugin, error)

// PluginOption 함수형 옵션
type PluginOption func(*pluginOptions)

func WithPluginLogger(logger observe.Logger) PluginOption
func WithPluginMetrics(metrics observe.Metrics) PluginOption
func WithPluginConfig(config map[string]any) PluginOption

// PluginLoader 인터페이스 - 로딩 전략 추상화
type PluginLoader interface {
    Load(ctx context.Context, path string) (Plugin, error)
    CanLoad(path string) bool
    Type() PluginType
}

// === manager.go ===

// LoadResult 로드 결과
type LoadResult struct {
    Path   string
    Plugin Plugin
    Err    error
}

// ManagerOption 함수형 옵션
type ManagerOption func(*managerOptions)

func WithManagerLogger(logger observe.Logger) ManagerOption
func WithManagerMetrics(metrics observe.Metrics) ManagerOption
func WithManagerConfig(config config.Config) ManagerOption

// Manager 플러그인 관리자
type Manager struct {
    lifecycle.BaseLifecycle
    // unexported fields: registry, loaders, config, logger, metrics
}

func NewManager(registry *Registry, loaders []PluginLoader, opts ...ManagerOption) *Manager

func (m *Manager) LoadPlugin(ctx context.Context, path string) (Plugin, error)
func (m *Manager) UnloadPlugin(ctx context.Context, pluginID string) error
func (m *Manager) LoadFromDirectory(ctx context.Context, dirPath string) []LoadResult
func (m *Manager) Start(ctx context.Context) error
func (m *Manager) Stop(ctx context.Context) error
func (m *Manager) Pause(ctx context.Context) error
func (m *Manager) Resume(ctx context.Context) error
func (m *Manager) GetPluginInfo(pluginID string) (PluginInfo, error)
func (m *Manager) GetAllPluginInfo() []PluginInfo

// === go_loader.go ===

type GoLoaderOption func(*goLoaderOptions)

type GoPluginLoader struct {
    // unexported fields: logger, config
}

func NewGoPluginLoader(opts ...GoLoaderOption) *GoPluginLoader

func (l *GoPluginLoader) Load(ctx context.Context, path string) (Plugin, error)
func (l *GoPluginLoader) CanLoad(path string) bool
func (l *GoPluginLoader) Type() PluginType

// === wasm_loader.go ===

type WASMLoaderOption func(*wasmLoaderOptions)

type WASMPluginLoader struct {
    // unexported fields: runtime, compilationCache, sandbox, logger, config
}

func NewWASMPluginLoader(runtime wazero.Runtime, opts ...WASMLoaderOption) *WASMPluginLoader

func (l *WASMPluginLoader) Load(ctx context.Context, path string) (Plugin, error)
func (l *WASMPluginLoader) CanLoad(path string) bool
func (l *WASMPluginLoader) Type() PluginType

// WASMPlugin - WASM 모듈을 Plugin 인터페이스로 래핑
type WASMPlugin struct {
    lifecycle.BaseLifecycle
    // unexported fields: module, compiled, metadata, sandbox
}

// === registry.go ===

type Registry struct {
    // unexported fields: mu sync.RWMutex, plugins map[string]Plugin
}

func NewRegistry() *Registry

func (r *Registry) Register(plugin Plugin) error
func (r *Registry) Get(pluginID string) (Plugin, error)
func (r *Registry) Unregister(pluginID string) error
func (r *Registry) List() []Plugin
func (r *Registry) ListByType(pluginType PluginType) []Plugin

// === sandbox.go ===

type SandboxConfig struct {
    MaxMemoryBytes       uint64        `json:"max_memory_bytes"`
    MaxExecutionTime     time.Duration `json:"max_execution_time"`
    MaxFuelLimit         uint64        `json:"max_fuel_limit"`
    AllowedHostFunctions []string      `json:"allowed_host_functions"`
}

// === stats.go ===

type PluginInfo struct {
    Metadata PluginMetadata  `json:"metadata"`
    State    lifecycle.State `json:"state"`
    LoadedAt time.Time       `json:"loaded_at"`
    Stats    PluginStats     `json:"stats"`
}

type PluginStats struct {
    ProcessedMessages    uint64        `json:"processed_messages"`
    FailedMessages       uint64        `json:"failed_messages"`
    TotalExecutionTime   time.Duration `json:"total_execution_time"`
    AverageExecutionTime time.Duration `json:"average_execution_time"`
    LastExecutionTime    time.Time     `json:"last_execution_time"`
    MemoryUsageBytes     uint64        `json:"memory_usage_bytes"`
}

// === marketplace.go ===

type Marketplace interface {
    Search(ctx context.Context, query string) ([]PluginMetadata, error)
    Install(ctx context.Context, pluginID string, version string) error
    Update(ctx context.Context, pluginID string) error
    Uninstall(ctx context.Context, pluginID string) error
}
```

### 4.3 상태 전이 다이어그램

```
Plugin Lifecycle (BaseLifecycle 기반):

  Created ──Init()──> Initialized ──Start()──> Running
                                                  │  ▲
                                            Pause()│  │Resume()
                                                  ▼  │
                                                Paused
  Running ──Stop()──> Stopped ──Destroy()──> Destroyed

Manager Lifecycle:

  Created ──Init()──> Initialized ──Start()──> Running (auto-load plugins)
                                                  │  ▲
                                            Pause()│  │Resume()
                                                  ▼  │
                                                Paused (all plugins paused)
  Running ──Stop()──> Stopped (all plugins stopped + unloaded)
```

### 4.4 플러그인 로드 시퀀스

```
Manager.LoadPlugin(ctx, path)
  │
  ├── 1. for loader in loaders:
  │       if loader.CanLoad(path) → selected
  │
  ├── 2. loader.Load(ctx, path)
  │       ├── [Go] plugin.Open(path) → Lookup("NewPlugin") → Lookup("Metadata") → factory()
  │       └── [WASM] ReadFile → CompileModule → BindHostFunctions → InstantiateModule → WASMPlugin{}
  │
  ├── 3. plugin.Init(ctx)
  │
  ├── 4. registry.Register(plugin)
  │
  ├── 5. for _, nodeType := range plugin.NodeTypes():
  │       nodeRegistry.Register(nodeType, plugin.CreateNode)
  │
  └── 6. metrics.Inc("plugin.loaded"), logger.Info("plugin loaded")
```

### 4.5 메트릭 정의

| 메트릭 이름 | 타입 | 설명 |
|------------|------|------|
| `plugin.loaded` | Counter | 플러그인 로드 횟수 |
| `plugin.unloaded` | Counter | 플러그인 언로드 횟수 |
| `plugin.load.error` | Counter | 플러그인 로드 실패 횟수 |
| `plugin.process.duration` | Histogram | 플러그인 노드 메시지 처리 시간 |
| `plugin.process.error` | Counter | 플러그인 노드 처리 실패 횟수 |
| `plugin.active` | Gauge | 현재 활성 플러그인 수 |
| `plugin.sandbox.violation` | Counter | 샌드박스 위반 횟수 |
| `plugin.wasm.memory` | Gauge | WASM 플러그인 메모리 사용량 |

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 파일 | 우선순위 |
|------------|------|------|---------|
| REQ-PLUGIN-001-01-01 ~ 01-06 | Plugin Interface | plugin.go | P0 |
| REQ-PLUGIN-001-02-01 ~ 02-08 | Plugin Manager | manager.go | P0 |
| REQ-PLUGIN-001-03-01 ~ 03-05 | Go Plugin Loader | go_loader.go | P0 |
| REQ-PLUGIN-001-04-01 ~ 04-07 | WASM Plugin Loader | wasm_loader.go | P0 |
| REQ-PLUGIN-001-05-01 ~ 05-06 | Plugin Registry | registry.go | P0 |
| REQ-PLUGIN-001-06-01 ~ 06-02 | Error Types | errors.go | P0 |
| REQ-PLUGIN-001-07-01 ~ 07-03 | Plugin Configuration | config.go | P1 |
| REQ-PLUGIN-001-08-01 ~ 08-03 | Sandbox Environment | sandbox.go | P1 |
| REQ-PLUGIN-001-09-01 ~ 09-05 | Plugin Info & Stats | stats.go | P1 |
| REQ-PLUGIN-001-10-01 ~ 10-02 | Plugin Marketplace | marketplace.go | P2 |
