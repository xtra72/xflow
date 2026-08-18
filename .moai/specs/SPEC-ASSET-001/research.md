# SPEC-ASSET-001 — 코드베이스 조사 (research.md)

> 아래 앵커는 모두 실제 파일을 읽어 확인한 실측 시그니처/라인이다(작성 전 Read 로 검증).

## 1. 저장소 계층 (internal/storage)

### 1.1 SQLite 기반 + 멱등 마이그레이션 — `sqlite.go`
- `NewSQLiteRepository(ctx, dbPath) (*SQLiteRepository, error)` (line 26): `os.MkdirAll` 부모 디렉토리 →
  `sql.Open("sqlite", dbPath)` → `PRAGMA journal_mode=WAL` → 마이그레이션.
- `OpenSQLiteDB(ctx, dbPath) (*sql.DB, error)` (line 126): 공유 `*sql.DB` 를 열고 dashboards/users 스키마를
  멱등 마이그레이션. **호출자가 수명 책임**(Close). main.go:699 가 이를 써서 authDashboardDB 공유.
- `migrateDashboardSchema` (line 78): `CREATE TABLE IF NOT EXISTS dashboards(... CHECK(scope IN ...))` +
  `CREATE UNIQUE INDEX IF NOT EXISTS dashboards_scope_owner_uidx ON dashboards(scope, COALESCE(owner, ''))`
  (line 92-94) — **COALESCE 표현식 유니크 인덱스가 modernc.org/sqlite 에서 정상 동작**(주석 line 73-75).
- `migrateUsersSchema` (line 104): `created_at INTEGER NOT NULL` / `updated_at INTEGER NOT NULL` — epoch 정수 규약.
- 멱등성: 여러 저장소가 별도 `*sql.DB` 핸들을 열어도 WAL 하에서 안전 공존(주석 line 55-59, ASM-007).
→ 본 SPEC: `migrateAssetSchema` 를 동일 기법으로 작성, `COALESCE(parent_id,0)` 루트 유니크에 재사용.

### 1.2 인터페이스 + factory 패턴 — `settings_repository.go`, `settings_sqlite.go`, `factory.go`
- `settings_repository.go`: `SettingsRepository` 인터페이스(GetSetting/SetSetting/Close) + `ErrSettingNotFound`
  sentinel + `NewSettingsRepository(ctx, storageType, sqlitePath)` factory(switch, 기본 sqlite).
- `settings_sqlite.go`:
  - `NewSettingsSQLiteRepository(ctx, dbPath)` (line 35): `sqliteDSN(dbPath)` DSN 사용.
  - `NewSettingsSQLiteRepositoryWithDB(ctx, db *sql.DB)` (line 59): **공유 핸들 주입 진입점**. main.go:870 사용.
  - `migrateSettingsSchema` (line 71): `CREATE TABLE IF NOT EXISTS settings(...)`.
  - Upsert idiom (line 97-108): `INSERT ... VALUES(...) ON CONFLICT(key) DO UPDATE SET value=excluded.value,
    updated_at=excluded.updated_at`, `time.Now().UnixMilli()` (line 104).
  - 컴파일 타임 검증: `var _ SettingsRepository = (*SettingsSQLiteRepository)(nil)` (line 26).
- `factory.go`: `NewRepository(ctx, cfg config.StorageConfig)` (line 15) switch(file/sqlite/postgres);
  `NewAgentRepository` (line 33) 유사. → 본 SPEC 은 별도 `NewAssetRepository` factory 추가(기존 스위치 불변).

### 1.3 register-if-missing 선례 — `device_id_repository.go`
- `DeviceIDRepository.GetOrCreate(ctx, agentName, unitID string) (string, error)` (line 30/82):
  double-check 잠금(RLock 조회 → Lock → 재확인 → 신규 생성 → 저장; 저장 실패 시 캐시 롤백 line 103-108).
- 복합 키 `deviceIDKey(a,b) = a + ":" + b` (line 78). in-memory 변형(line 227)도 동일 계약.
→ 본 SPEC `GetOrCreate` 계약 모델. 단 파일-JSON 대신 **SQLite 트랜잭션 + 유니크 제약**으로 원자화.

### 1.4 DSN 헬퍼 — `remote_audit_sqlite.go:30`
- `sqliteDSN(dbPath) = "file:" + dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"` —
  풀의 모든 연결에 pragma 적용(경합 시 대기·재시도). → 본 SPEC WithDB 아닌 경로에서 재사용.

### 1.5 기존 xsfm station 레지스트리(범위 밖) — `station_registry_repository.go:15`
- `StationRegistryEntry{Station, StationNumber string, Line, DisplayName string, Order int, Places []PlaceEntry}`
  (파일-JSON, atomic write + 인메모리 캐시). `PlaceEntry{Place, DisplayName, Order}`.
- `internal/agent/xsfm/station_registry.go`: `UpsertStation`/`ResolveLine`/`ResolveStationNumber`.
→ 개념 중복(line/station/place ≈ Asset 트리)이나 **리팩터 안 함**(design.md §9 통합 노트로만).

### 1.6 config — `internal/config/types.go:60`
- `StorageConfig{ Type string; ...; SQLitePath string }` (types.go:60-63). `cfg.Storage().SQLitePath`
  = `storage.sqlite.path`(config.go:349). main.go:578 `storageCfg := cfg.Storage()`.

## 2. 노드 프레임워크 (internal/node)

### 2.1 Node 계약 — `base.go`
- `Node` 인터페이스(line 15): `ID/Name/Type/Init/Process(ctx,msg)([]message.Message,error)/Shutdown/
  Configure(map[string]any)/Ports`.
- `NodeOption func(*BaseNode)` (line 63). `NewBaseNode(def flow.NodeDef, opts ...NodeOption) *BaseNode` (line 103).
- `BaseNode.config map[string]any` (line 89); `Configure` 는 config 복사(line 172), nil → `ErrInvalidConfig`.
- lifecycle: `TransitionTo(lifecycle.StateInitializing/StateRunning/StateStopping)`.

### 2.2 빌트인 등록 — `registry.go`
- `NodeFactory func(def flow.NodeDef, opts ...NodeOption) (Node, error)` (line 10).
- `registerBuiltins()` (line 73): builtins 슬라이스 `{typeName, factory, category, description}` 리터럴 —
  여기에 `asset-encode`/`asset-decode` 2행 추가.
- `RegisterWithMeta`(line 209), `Types()`(line 263), `AllTypeMeta()`(line 232), `Has`(line 276).

### 2.3 함수형 resolver 주입(Pattern B) — `inventory.go`
- `WithDeviceRegistryFunc(fn func() device.DeviceRegistry) NodeOption` (line 120): `b.config[key]=fn`.
- config 키 상수 `_inventory_device_registry_fn`(line 61). 팩토리에서
  `base.config[key].(func() device.DeviceRegistry)` 로 추출(line 247-258).
- sentinel `ErrInventoryDeviceRegistryNotAvailable = fmt.Errorf("inventory: %w: ...", ErrNodeNotInitialized)`
  (line 81). `Init` 에서 source 별 의존성 검증 → 미주입 시 sentinel 반환(line 374-397, **fail-fast**).
→ 본 SPEC `WithAssetRegistryFunc` 를 이 패턴으로 정확히 미러.

### 2.4 AgentResolver 주입(Pattern A, 비채택) — `bridge.go`, `store_write.go`
- `WithAgentResolver(resolver AgentResolver) NodeOption` (bridge.go:60): `b.config["_agent_resolver"]=resolver`.
- store_write.go:180 팩토리에서 추출, `resolveStore`(line 203)가 `AgentRef` 로 에이전트 해석(line 226) —
  **특정 에이전트 지목** 필요. asset 레지스트리는 전역 싱글턴이라 부적합 → Pattern B 채택(design.md §4).

### 2.5 config 파싱 & `$.`-path — `store_write.go`
- `resolveTemplateExpr(expr string, msg message.Message) (any, error)` (line 899): `$.payload.x[.y]`,
  `$.metadata.x[.y]`, `$.id/$.type/$.timestamp` 지원. non-`$.` 은 payload 직접 필드.
- `resolveKeyTemplate`(line 867) `{expr}` 보간. Configure 에서 `$.` prefix 강제 검증 idiom(line 286).

### 2.6 reverse-lookup + default 폴백 — `mapping.go`
- `MappingNode.Process`(line 101): `messageToMap(msg)` → `message.NewPayload(msgMap).GetPath(field)`(line 113) →
  키 조회 실패 시 `hasDefault` 면 default, 아니면 `ErrMappingKeyNotFound`(line 124-129).
- `target != ""` 이면 `msg.Payload().Set(target, result)`, 아니면 `lastPathKey(field)` 에 Set(line 132-138).
- `lastPathKey(path)`(line 150): `$.payload.status_code` → `status_code`.
→ 본 SPEC asset-decode 의 raw passthrough(미발견 시 무변경)를 이 default-부재 모델로 구현.

## 3. 메시지 API (pkg/message)
- `message.New(opts ...Option) Message`(message.go:122); `SetType`/`Payload()`/`Metadata()`/`Clone()`.
- `Payload` 인터페이스(payload.go:9): `Set(key,value)`(line 57, upsert), `Get(key)(any,bool)`,
  `GetPath(jsonpath)(any,error)`, `ToMap`, `Clone`. `NewPayload(data...)`(line 37).

## 4. API 계층 (internal/api)
- `Context` 인터페이스(router.go:24): `Param(name)`, `Query(name)`, `QueryValues(name)`,
  `Bind(v any) error`, `JSON(code,v) error`, `Context() context.Context`.
- `RouteGroup` 메서드(router.go:174-194): `GET/POST/PUT/PATCH/DELETE(path, handler, mw...)`.
- 핸들러 패턴 — `store_query.go`: `NewStoreQueryHandler(agents, logger)`(line 156),
  `RegisterRoutes(g *api.RouteGroup)`(line 164, Go1.22 mux `"/store/{agent_name}/keys"`),
  `dto.NewSuccessResponse(...)`, `api.ErrBadRequest/ErrNotFound/ErrConflict.WithMessage(...)`.
- 최소 CRUD 핸들러 — `settings.go`: `NewSettingsHandler(repo, logger)`(line 55),
  `RegisterRoutes`(line 66) `GET/PUT /settings/{key}`, `ctx.Param("key")`, `ctx.Bind`, `json.Valid`,
  `dto.NewSuccessResponse(SettingsResponse{...})`.
- 중앙 배선 — `cmd/xflowd/main.go:876` `server.RegisterRoutes(func(g *api.RouteGroup){ ...
  settingsHandler.RegisterRoutes(g) })`(line 911). → 여기에 `assetHandler.RegisterRoutes(g)` 추가.

## 5. main.go 배선 지점 (cmd/xflowd/main.go)
- line 202 `deviceRegistry := device.NewRegistry()`; line 417 `agentResolver := ...`.
- line 515-544 inventory 옵션 4종 생성 → line 524 `eng = engine.NewEngine(engine.WithNodeOptions(...))`
  (line 529 목록). → 여기에 `assetRegistryOpt` 추가(M2).
- line 578-580 `storageCfg := cfg.Storage()`; `storage.NewRepository(...)`.
- line 699 `authDashboardDB, err := storage.OpenSQLiteDB(context.Background(), storageCfg.SQLitePath)` —
  공유 `*sql.DB`. → asset 저장소도 `NewAssetSQLiteRepositoryWithDB(ctx, authDashboardDB)` 로 공유(M2).
- line 810 `storeQueryHandler := handler.NewStoreQueryHandler(agentMgr, ...)`;
  line 870-874 `settingsRepo := storage.NewSettingsSQLiteRepositoryWithDB(...)` + `NewSettingsHandler`.

## 6. CSV 시드 데이터 (examples/subway) — 실측
- `station.csv`: 헤더 `# station,line,name,order`; 예 `st01,ui-line,신설동,1` / `st02,ui-line,보문,2`.
- `spot.csv`: 헤더 `# station,code,name,floor`; 예 `st01,01,상행계단#1,1` / `st01,02,상행계단#2,1`.
- `device.csv`: 헤더 `# station,code,index`; 예 `st01,01,1`.
- **기존 importer 없음(net-new)**. 계층: line(ui-line) → station(st01) → spot(01) → device.

## 7. 미해결/결정 처리 요약
조사 결과 모든 설계 포인트가 기존 코드로 해소됨 → **blocker 없음**. 유일한 명세 내부 긴장
(confirmed decision #2 "unique within (parent,kind)" vs Data model "(parent_id, local_code)")은
권위 있는 "Data model to specify" 섹션 채택 + ASM-02(한 부모=한 kind)로 해소(design.md §3 D-3a).
