---
id: SPEC-PLUGIN-001
type: plan
version: "1.0.0"
spec_ref: SPEC-PLUGIN-001
---

# SPEC-PLUGIN-001 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반): 모든 파일이 신규 생성이므로 TDD(RED-GREEN-REFACTOR) 적용
- 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표
- `go test -race` 필수 실행 (동시성 안전 검증)
- Registry, Stats 등 동시성 관련 코드는 `-race` 플래그로 data race 검증

### 1.2 기술 스택

- **언어**: Go 1.23+
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성**: `sync.RWMutex` (Registry), `sync/atomic` (Stats 카운터)
- **컨텍스트**: `context.Context` (취소, 타임아웃 전파)
- **WASM 런타임**: `github.com/tetratelabs/wazero` v1.6+ (순수 Go, CGo 없음)
- **의존 패키지**:
  - `pkg/lifecycle/` (SPEC-LIFE-001): BaseLifecycle, Configurable, State
  - `pkg/xferr/` (SPEC-ERR-001): ComponentPlugin, NewXFlowError
  - `pkg/message/` (SPEC-MSG-001): Message, Payload, Metadata
  - `internal/observe/` (SPEC-OBS-001): Logger, Metrics 인터페이스
  - `internal/config/` (SPEC-CFG-001): Config, PluginConfig, HotReload

### 1.3 패키지 위치

- **경로**: `internal/plugin/`
- **Tier**: internal (비공개 패키지)
- **소비자**: `internal/node/` (Node Registry 연동), `internal/engine/` (플로우 실행), `internal/api/` (REST API)

---

## 2. 마일스톤

### Primary Goal: Error Types + Plugin Interface + Plugin Registry (P0 기반)

**범위**: Module 6, 1, 5

**작업 항목**:

1. `errors.go` + `errors_test.go` 작성
   - 15개 sentinel error 변수 정의
   - `pkg/xferr/` 래핑 패턴 테스트
   - 에러 메시지 포맷 검증

2. `plugin.go` + `plugin_test.go` 작성
   - `PluginType` 상수 정의 (`PluginTypeGoNative`, `PluginTypeWASM`)
   - `Plugin` 인터페이스 정의 (lifecycle.Lifecycle + lifecycle.Configurable 임베딩)
   - `PluginMetadata` 구조체 정의
   - `PluginFactory` 타입 정의
   - `PluginOption` 패턴 정의 (WithPluginLogger, WithPluginMetrics, WithPluginConfig)
   - `PluginLoader` 인터페이스 정의
   - Mock 기반 인터페이스 컴파일 검증 테스트

3. `registry.go` + `registry_test.go` 작성
   - `Registry` 구조체 (sync.RWMutex 기반)
   - Register/Get/Unregister/List/ListByType 메서드
   - 중복 등록 에러 테스트
   - 동시성 안전 테스트 (`-race` 플래그)
   - 존재하지 않는 ID 조회 에러 테스트

**검증 기준**: `go test -race ./internal/plugin/...` 전체 통과, 커버리지 85%+

### Secondary Goal: Go Plugin Loader + WASM Plugin Loader (P0 로더)

**범위**: Module 3, 4

**작업 항목**:

1. `go_loader.go` + `go_loader_test.go` 작성
   - `GoPluginLoader` 구조체 정의
   - `PluginLoader` 인터페이스 구현
   - `Load(ctx, path)`: plugin.Open -> Lookup("NewPlugin") -> Lookup("Metadata") -> factory 호출
   - `CanLoad(path)`: 확장자 `.so` + Go.Enabled 확인
   - 심볼 누락/타입 불일치 에러 처리 테스트
   - 참고: Go Plugin 테스트는 `.so` 파일 빌드가 필요하므로 mock/stub 기반 테스트 + integration test 태그 분리

2. `wasm_loader.go` + `wasm_loader_test.go` 작성
   - `WASMPluginLoader` 구조체 정의 (wazero.Runtime 보유)
   - `PluginLoader` 인터페이스 구현
   - `Load(ctx, path)`: ReadFile -> CompileModule -> BindHostFunctions -> InstantiateModule -> WASMPlugin 생성
   - `CanLoad(path)`: 확장자 `.wasm` + WASM.Enabled 확인
   - `WASMPlugin` 어댑터 구현 (BaseLifecycle 임베딩)
   - Host Function 바인딩 (host_log, host_metric_inc, host_get_config)
   - 익스포트 함수 누락 에러 테스트
   - 참고: WASM 테스트용 최소 `.wasm` 파일은 testdata/ 디렉토리에 포함

**검증 기준**: 로더별 테스트 통과, 인터페이스 호환성 검증

### Tertiary Goal: Plugin Manager (P0 코어)

**범위**: Module 2

**작업 항목**:

1. `manager.go` + `manager_test.go` 작성
   - `Manager` 구조체 (BaseLifecycle 임베딩)
   - `NewManager(registry, loaders, opts...)` 생성자
   - `LoadPlugin(ctx, path)`: 로더 선택 -> 로드 -> Init -> Registry 등록 -> NodeTypes 등록
   - `UnloadPlugin(ctx, pluginID)`: Stop -> NodeTypes 해제 -> Registry 해제
   - `LoadFromDirectory(ctx, dirPath)`: 디렉토리 스캔 -> 개별 로드 -> []LoadResult
   - `Start(ctx)` / `Stop(ctx)` / `Pause(ctx)` / `Resume(ctx)` 라이프사이클
   - 중복 로드 방지 테스트 (`ErrPluginAlreadyLoaded`)
   - Mock Loader 기반 통합 테스트
   - 라이프사이클 상태 전이 테스트

**검증 기준**: Manager 전체 라이프사이클 테스트 통과, 동시성 안전 검증

### Final Goal: Plugin Configuration + Sandbox + Stats (P1)

**범위**: Module 7, 8, 9

**작업 항목**:

1. `config.go` + `config_test.go` 작성
   - `Config.Plugin() PluginConfig` 설정 소비
   - 설정 검증 (디렉토리 존재 확인, 기본값 적용)
   - HotReload 콜백 등록 (Go/WASM 활성화 변경 시 자동 언로드/재로드)
   - 설정 변경 시 Manager 연동 테스트

2. `sandbox.go` + `sandbox_test.go` 작성
   - `SandboxConfig` 구조체 정의
   - Wazero ModuleConfig에 메모리/Fuel 제한 적용
   - context.WithTimeout 기반 시간 제한
   - 리소스 초과 시 `ErrSandboxViolation` 에러 테스트
   - 허용 호스트 함수 필터링 테스트

3. `stats.go` + `stats_test.go` 작성
   - `PluginInfo`, `PluginStats` 구조체 정의
   - `Manager.GetPluginInfo(pluginID)` / `GetAllPluginInfo()` 메서드
   - atomic 기반 통계 업데이트
   - 동시성 안전 통계 수집 테스트

**검증 기준**: 설정 검증 + HotReload 연동 + 샌드박스 제한 + 통계 수집 테스트 통과

### Optional Goal: Plugin Marketplace (P2)

**범위**: Module 10

**작업 항목**:

1. `marketplace.go` + `marketplace_test.go` 작성
   - `Marketplace` 인터페이스 정의
   - `LocalMarketplace` 구현 (파일시스템 기반)
   - Search/Install/Update/Uninstall 기본 구현
   - 인터페이스 컴파일 검증 테스트

**검증 기준**: 로컬 Marketplace 기본 기능 테스트 통과

---

## 3. 기술적 접근

### 3.1 Loader 추상화 전략

`PluginLoader` 인터페이스로 Go Plugin과 WASM Plugin을 동일한 추상화 레벨에서 처리한다. Manager는 등록된 로더 목록을 순회하며 `CanLoad()`로 적합한 로더를 선택한다. 이 설계는 향후 새로운 플러그인 타입(예: Lua Script Plugin)을 추가할 때 새 Loader만 구현하면 된다.

### 3.2 WASM Host Function 설계

Wazero의 `wazero.NewHostModuleBuilder()`를 사용하여 호스트 함수를 "env" 네임스페이스에 등록한다. 호스트 함수는 WASM 선형 메모리에서 포인터/길이 쌍으로 문자열을 교환한다. 이는 WASI Preview 1 호환 방식이다.

### 3.3 동시성 모델

- **Registry**: `sync.RWMutex` - 읽기 다수/쓰기 소수 패턴
- **Stats**: `sync/atomic` - 고빈도 카운터 업데이트
- **Manager**: `sync.Mutex` - 로드/언로드 직렬화 (빈도 낮음)
- **WASM Instance**: 인스턴스별 격리 (동시성 문제 없음)

### 3.4 Go Plugin 테스트 전략

Go `plugin` 패키지는 런타임에 `.so` 파일을 로드하므로 단위 테스트가 어렵다. 다음 전략을 적용한다:

1. **인터페이스 기반 Mock**: `PluginLoader` 인터페이스에 대한 mock으로 Manager 테스트
2. **Integration Test 태그**: `//go:build integration` 태그로 실제 `.so` 로딩 테스트 분리
3. **testdata/ 디렉토리**: 테스트용 최소 플러그인 `.so` / `.wasm` 파일 포함

### 3.5 WASM 테스트 전략

1. **최소 WASM 모듈**: WAT(WebAssembly Text) 형식으로 테스트용 최소 모듈 작성
2. **Wazero 인메모리**: 파일시스템 없이 바이트 배열로 모듈 컴파일 테스트
3. **Host Function 검증**: 호스트 함수 호출 시 올바른 데이터가 전달되는지 검증

---

## 4. 리스크

### 4.1 Go Plugin 패키지 제약

**리스크**: Go `plugin` 패키지는 Linux/macOS에서만 동작하며, 빌드 Go 버전이 호스트와 동일해야 한다.

**대응**: WASM Plugin을 기본 전략으로 권장하고, Go Plugin은 고성능이 필요한 경우의 옵션으로 위치시킨다. CI에서 cross-platform 테스트 시 Go Plugin 테스트는 Linux만 실행한다.

### 4.2 Wazero API 안정성

**리스크**: Wazero v1.6+의 API가 향후 변경될 수 있다.

**대응**: `WASMPluginLoader` 내부에 Wazero 의존성을 캡슐화하여 API 변경 영향을 최소화한다. Wazero의 `wazero.Runtime` 인터페이스를 직접 노출하지 않고 추상화 레이어를 유지한다.

### 4.3 WASM 성능 오버헤드

**리스크**: WASM 함수 호출 시 직렬화/역직렬화 오버헤드가 있다.

**대응**: 메시지 배치 처리 최적화, 컴파일 캐시 활용, 벤치마크 테스트로 성능 기준선을 확보한다.

### 4.4 플러그인 간 의존성

**리스크**: 플러그인 간 의존성(`Dependencies` 필드)이 복잡해질 수 있다.

**대응**: Primary Goal에서는 독립 플러그인만 지원하고, 의존성 해결은 Optional Goal로 분류한다.

---

## 5. 의존성 그래프

```
errors.go (Module 6) ── 의존성 없음
    │
    ▼
plugin.go (Module 1) ── pkg/lifecycle, pkg/xferr, internal/observe
    │
    ├─────────────────┐
    ▼                 ▼
registry.go (5)    go_loader.go (3) ── plugin 패키지
    │                 │
    │                 ▼
    │              wasm_loader.go (4) ── wazero
    │                 │
    ▼                 ▼
manager.go (Module 2) ── registry + loaders 통합
    │
    ├──────────────┬──────────────┐
    ▼              ▼              ▼
config.go (7)  sandbox.go (8)  stats.go (9)
    │
    ▼
marketplace.go (Module 10)
```

---

## 6. 구현 순서 (파일별)

| 순서 | 파일 | 테스트 파일 | 모듈 | 마일스톤 | 우선순위 |
|------|------|-----------|------|---------|---------|
| 1 | errors.go | errors_test.go | Error Types (6) | Primary | P0 |
| 2 | plugin.go | plugin_test.go | Plugin Interface (1) | Primary | P0 |
| 3 | registry.go | registry_test.go | Plugin Registry (5) | Primary | P0 |
| 4 | go_loader.go | go_loader_test.go | Go Plugin Loader (3) | Secondary | P0 |
| 5 | wasm_loader.go | wasm_loader_test.go | WASM Plugin Loader (4) | Secondary | P0 |
| 6 | manager.go | manager_test.go | Plugin Manager (2) | Tertiary | P0 |
| 7 | config.go | config_test.go | Plugin Configuration (7) | Final | P1 |
| 8 | sandbox.go | sandbox_test.go | Sandbox Environment (8) | Final | P1 |
| 9 | stats.go | stats_test.go | Plugin Info & Stats (9) | Final | P1 |
| 10 | marketplace.go | marketplace_test.go | Plugin Marketplace (10) | Optional | P2 |

---

## 7. 참고 사항

### 7.1 testdata 디렉토리

```
internal/plugin/testdata/
  sample.wasm         # 테스트용 최소 WASM 모듈 (WAT에서 컴파일)
  sample.wat          # 최소 WASM 텍스트 소스
  invalid.wasm        # 잘못된 WASM 파일 (에러 테스트용)
```

Go Plugin `.so` 파일은 CI 파이프라인에서 빌드하여 사용한다 (플랫폼 의존적).

### 7.2 cross-SPEC 통합 포인트

- **SPEC-NODE-001**: `Manager.LoadPlugin()` 성공 시 `NodeRegistry.Register(typeName, factory)` 호출
- **SPEC-CFG-001**: `Config.Plugin()` 메서드로 설정 소비, HotReload 콜백 등록
- **SPEC-OBS-001**: `plugin.*` 접두사로 Logger/Metrics 사용
- **SPEC-ERR-001**: `ComponentPlugin` + `NewXFlowError` 래핑 패턴 사용
