# SPEC-ASSET-001 — 구현 계획 (plan.md)

> 연관: `spec.md`(EARS 요구), `acceptance.md`(AC), `design.md`(설계), `research.md`(앵커).
> development_mode: `hybrid` → 신규 코드 전부 **TDD**(RED-GREEN-REFACTOR), 85% 커버리지 목표.
> 시간 예측 없음 — 우선순위/의존 기반 마일스톤.

## 기술 접근 (Technical Approach)

기존 검증된 3계층 패턴을 그대로 재사용한다(가정·추측 없이 아래 실측 앵커 인용):

1. **저장소**: 인터페이스(`asset_repository.go`) + sqlite 구현(`asset_sqlite.go`) + `factory.go`
   스위치 확장. 스키마는 `sqlite.go`(`migrateDashboardSchema`/`migrateUsersSchema` 선례)와
   동일한 `CREATE TABLE/INDEX IF NOT EXISTS` 멱등 마이그레이션. DSN 은
   `remote_audit_sqlite.go:30` `sqliteDSN`(WAL + busy_timeout) 재사용. 공유 핸들 주입은
   `settings_sqlite.go:59` `NewSettingsSQLiteRepositoryWithDB(ctx, *sql.DB)` 선례.
2. **register-if-missing**: `device_id_repository.go:82` `GetOrCreate` 의 double-check + 저장
   실패 롤백 계약을 복제하되, 순차 채번은 SQLite **트랜잭션 + 유니크 제약 재시도**로 원자화한다.
3. **노드 주입**: `inventory.go` 의 함수형 resolver(Pattern B)를 정확히 미러 —
   `WithAssetRegistryFunc(func() AssetRegistry) NodeOption`(private config 키 + Init 검증).
   `cmd/xflowd/main.go:529` `engine.WithNodeOptions(...)` 에 옵션 추가.
4. **노드 config 파싱**: `store_write.go` 의 `$.`-path(`resolveTemplateExpr`),
   `mapping.go` 의 `Configure`/default-부재 폴백 idiom.
5. **API**: `store_query.go`/`settings.go` 핸들러 패턴 + `main.go:876` 중앙 `RegisterRoutes` 배선.

### Pattern B 선택 근거 (design.md §4 상세)

- Pattern A(`WithAgentResolver` + `_agent_resolver` config 키, store_write.go:180)는 노드가
  `AgentRef` 로 특정 Store 에이전트를 지목·해석해야 한다 → asset 레지스트리는 특정 에이전트가
  아니라 **프로세스 전역 싱글턴**(deviceRegistry 처럼)이므로 부적합.
- Pattern B(`WithDeviceRegistryFunc` 류 함수형 resolver, inventory.go:120)는 전역 싱글턴을
  지연 획득하며 초기화 순서 문제를 회피 → asset 레지스트리에 정확히 부합. **Pattern B 채택**.

## 마일스톤 (우선순위 기반, 의존 순서)

### M1 — `assets` 테이블 + `AssetRepository`/`AssetSQLiteRepository` [Priority High · 선행]

목표: 스키마·마이그레이션·factory 배선·`GetOrCreate`(순차 채번)·`ResolveByCode`·tree-aware CRUD.

- `internal/storage/asset_repository.go`: `Asset`/`AssetCreate`/`AssetUpdate`/`AssetFilter` 타입,
  `AssetRepository` 인터페이스(`spec.md` 명세), `ErrAssetNotFound`/`ErrAssetHasChildren` sentinel,
  `NewAssetRepository(ctx, cfg)` factory(현재 sqlite 만; `NewSettingsRepository` 선례).
- `internal/storage/asset_sqlite.go`: `migrateAssetSchema(ctx, db)`(3 유니크 인덱스 + FK),
  `NewAssetSQLiteRepository(ctx, dbPath)` + `NewAssetSQLiteRepositoryWithDB(ctx, *sql.DB)`,
  `GetOrCreate`(tx: 조회→미존재 시 부모 스코프 `MAX(local_code)+1` 채번→full_code 계산→insert;
  유니크 충돌 시 재조회), `ResolveByCode`(full_code 인덱스 단일 룩업), `List/Get/Create/Update/Delete`.
- `internal/storage/factory.go`: 필요 시 `NewAssetRepository` 추가(sqlite 만). (기존 `NewRepository`
  스위치는 건드리지 않고 별도 factory 함수 추가 — settings 선례.)
- TDD: `asset_sqlite_test.go` — AC-01..09, AC-11 커버(순차 채번, 값→코드 자동등록/중복없음,
  계층 유니크, ResolveByCode, 동시 GetOrCreate).

의존: 없음(선행). 완료 기준: AC-01..09, AC-11 그린 + `go test ./internal/storage/...` 통과.

### M2 — `WithAssetRegistryFunc` NodeOption + main.go 주입 [Priority High · M1 후]

목표: 노드가 소비할 `AssetRegistry` 최소 인터페이스(node 패키지) + 함수형 resolver 옵션 + 엔진 주입.

- `internal/node/asset.go`: node 패키지 로컬 `AssetRegistry` 인터페이스(store_write `StoreWriter`
  선례 — 순환 의존 회피), `WithAssetRegistryFunc(fn func() AssetRegistry) NodeOption`,
  private config 키 `_asset_registry_fn`, 공용 sentinel `ErrAssetRegistryNotAvailable`.
- `cmd/xflowd/main.go`: M1 의 `AssetSQLiteRepository` 인스턴스 생성(공유 `authDashboardDB` 재사용,
  `OpenSQLiteDB`/`NewAssetSQLiteRepositoryWithDB` — line 699/870 선례) → `assetRegistryOpt :=
  node.WithAssetRegistryFunc(func() node.AssetRegistry { return assetRepo })` → `engine.WithNodeOptions(...)`
  (line 529) 에 추가.
- TDD: `asset_test.go`(node) — 옵션이 config 키를 설정하고 팩토리가 추출하는지, 미주입 시 Init sentinel.

의존: M1. 완료 기준: AC-10 그린.

### M3 — `asset-encode` 노드 (값→코드, 자동등록) [Priority High · M2 후]

목표: 필드 매핑(field/kind/parent) 기반 값→전체코드 치환 + passthrough.

- `internal/node/asset_encode.go`: `AssetEncodeNode`(`*BaseNode`), `NewAssetEncodeNode` 팩토리
  (base.config 에서 `_asset_registry_fn` 추출 — inventory 선례), `Configure`(mappings 파싱:
  field/kind/parent, `$.`-path 검증), `Init`(레지스트리 검증 fail-fast), `Process`(각 매핑:
  값 읽기 → parent 해석(리터럴 or `$.` via `resolveTemplateExpr`) → `GetOrCreate` → `Payload().Set`
  로 전체 코드 치환; 필드 부재는 skip, 부모 미존재는 error).
- `internal/node/registry.go:73` `registerBuiltins()` 테이블에 `{"asset-encode", NewAssetEncodeNode,
  "processing", "자산 값을 계층 코드로 인코딩(미존재 시 자동 등록)"}` 추가.
- TDD: `asset_encode_test.go` — AC-03(encode), AC-07-encode, AC-12, AC-14, 필드부재 skip(AC-13a).

의존: M1, M2. 완료 기준: AC-12, AC-14 그린 + 노드 등록/생성 검증.

### M4 — `asset-decode` 노드 (코드→값, raw 폴백) [Priority Medium · M2 후]

목표: 코드→값 복원, 미발견 시 원값 passthrough.

- `internal/node/asset_decode.go`: `AssetDecodeNode`(`*BaseNode`), `NewAssetDecodeNode`,
  `Configure`(field 필수 + target 선택 — mapping.go idiom), `Init`(레지스트리 검증),
  `Process`(field 코드 읽기 → `ResolveByCode` → found 시 target/소스에 기록, 미발견 시 무변경 —
  `mapping.go:104-141` default-부재 폴백 모델, `lastPathKey` 재사용 가능).
- `registry.go:73` 에 `{"asset-decode", NewAssetDecodeNode, "processing", "계층 코드를 자산 값으로
  디코딩(미발견 시 원값 통과)"}` 추가.
- TDD: `asset_decode_test.go` — AC-09, AC-13(raw passthrough).

의존: M1, M2. 완료 기준: AC-13 그린. (M3 와 병렬 가능하나 파일 분리로 충돌 없음 — 순차 권장.)

### M5 — CRUD API 핸들러 + 라우트 배선 [Priority Medium · M1 후]

목표: HTTP CRUD 노출.

- `internal/api/handler/asset.go`: `AssetHandler`(repo 주입), `NewAssetHandler(repo, logger)`,
  `RegisterRoutes(g *api.RouteGroup)`:
  - `GET /assets`(list; `?parent=`/`?kind=` 필터, tree/flat),
  - `GET /assets/{id}`,
  - `POST /assets`(manual create),
  - `PUT /assets/{id}`(update),
  - `DELETE /assets/{id}`(`?cascade=true` 선택).
  `store_query.go`/`settings.go` 패턴(`ctx.Param`/`ctx.Bind`/`ctx.JSON`, `dto.NewSuccessResponse`,
  `api.ErrBadRequest`/`ErrNotFound`/`ErrConflict`) 준용.
- `cmd/xflowd/main.go:876` 중앙 `RegisterRoutes` 클로저에 `assetHandler.RegisterRoutes(g)` 추가
  (settingsHandler 선례, line 911).
- TDD: `asset_test.go`(handler) — AC-15(CRUD 라운드트립, not-found 404, has-children 409/거부).

의존: M1. 완료 기준: AC-15 그린.

### M6 — (스트레치/선택) subway CSV 시더 [Priority Low · 연기 가능]

목표: `examples/subway/{station,spot,device}.csv` → 레지스트리 manual 시드(net-new; 기존 importer 없음).

- `internal/storage/asset_seed.go`: `SeedFromSubwayCSV(ctx, repo, dir)` — station.csv(`station,line,name,order`)
  → line(root, kind=line) + station(parent=line); spot.csv(`station,code,name,floor`) → spot(parent=station);
  device.csv(`station,code,index`) → device(parent=spot 또는 station, design.md §7 매핑 결정). `#` 주석 헤더 스킵.
- **본 마일스톤은 명시적으로 선택(deferrable)** — M1–M5 완료 및 사용자 승인 후에만 착수. 미착수 시 SPEC
  는 M1–M5 로 완결로 간주한다(REQ-S1 Optional).
- TDD: `asset_seed_test.go` — AC-16(계층·코드 형태 검증).

의존: M1. 완료 기준: AC-16 그린(선택).

## 아키텍처 설계 방향 (요약 — design.md 상세)

- 단일 `assets` 테이블 제네릭 트리(kind + parent_id). 그래프 확장 금지(Enforce Simplicity).
- full_code denormalized 저장 → 역방향 단일 인덱스 룩업. 부모 이동/리네임 캐스케이드는 범위 밖.
- 노드↔저장소는 최소 인터페이스(`AssetRegistry`)로 분리(순환 의존 회피, store_write/inventory 선례).
- Git 작업(브랜치/커밋/PR)은 본 계획 범위 밖(manager-git). 커밋 금지(사용자 지시).

## 리스크 및 대응

| 리스크 | 영향 | 대응 |
|--------|------|------|
| 순차 채번 경합(동시 GetOrCreate) | 중복 코드 | 트랜잭션 + `(parent,local_code)` 유니크 + 충돌 재조회(REQ-06, device_id double-check 선례) |
| NULL parent 유니크 미보장(SQLite NULL distinct) | 루트 중복 | `COALESCE(parent_id,0)` 표현식 인덱스(dashboards 선례 검증됨) |
| 부모 이동 시 하위 full_code 불일치 | 정합성 | 범위 밖 명시(Non-Goals). 최초 채번만 다룸; 리네임은 후속 SPEC |
| kind 스코프 vs 부모 스코프 채번 혼선(ASM-02) | 코드 규칙 모호 | design.md §3 결정: 부모 스코프 채번 + `(parent,local_code)` 유니크로 확정 |
| 기존 xsfm station_registry 와 개념 중복 | 유지보수 부담 | 통합 안 함(greenfield). design.md §9 통합 노트로만 기록 |

## Definition of Done (전체)

- M1–M5 의 모든 AC(AC-01..15) 그린, `go test ./internal/storage/... ./internal/node/... ./internal/api/...`
  통과, 85%+ 신규 코드 커버리지.
- `golangci-lint` 클린, `gofmt`/`goimports` 정렬.
- `asset-encode`/`asset-decode` 가 `Registry.Types()` 에 노출.
- 기존 xsfm station_registry 및 여타 SPEC 미변경(additive only).
- M6 는 선택 — 미착수 시에도 DoD 충족.
