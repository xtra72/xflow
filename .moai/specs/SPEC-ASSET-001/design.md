# SPEC-ASSET-001 — 기술 설계 (design.md)

> 모든 인용 API 는 실제 파일을 읽어 확인한 실측 시그니처이다(추측 금지). 앵커 목록: `research.md`.

## §1 아키텍처 개요

3계층 additive 설계:

```
   ┌─────────────────────────────────────────────────────────────┐
   │  internal/api/handler/asset.go   (M5, CRUD HTTP)              │
   │    └─ AssetRepository                                          │
   ├─────────────────────────────────────────────────────────────┤
   │  internal/node/asset_encode.go, asset_decode.go  (M3/M4)      │
   │    └─ node.AssetRegistry (최소 인터페이스; 순환의존 회피)       │
   │       주입: WithAssetRegistryFunc (M2, Pattern B)             │
   ├─────────────────────────────────────────────────────────────┤
   │  internal/storage/asset_repository.go + asset_sqlite.go  (M1) │
   │    └─ AssetRepository (레지스트리 + tree-aware CRUD)           │
   │       assets 테이블 (공유 xflow.db, WAL)                       │
   └─────────────────────────────────────────────────────────────┘
```

`node.AssetRegistry`(최소 계약)와 `storage.AssetRepository`(전체 계약)를 분리한다. node 패키지는
storage 를 import 하지 않고 자체 최소 인터페이스를 선언한다 — `internal/node/store_write.go:25`
`StoreWriter` 가 동일 패턴("순환 의존 방지를 위해 node 패키지 내에 최소 인터페이스로 정의")을
쓰는 것을 확인했다.

## §2 데이터 모델 — `assets` 테이블

`internal/storage/sqlite.go` 의 `migrateDashboardSchema`(line 78)·`migrateUsersSchema`(line 104)
가 `CREATE TABLE IF NOT EXISTS` + `CREATE UNIQUE INDEX IF NOT EXISTS`(COALESCE 표현식 인덱스,
line 92-94)를 쓰는 것을 확인했다. `dashboards` 의 `CREATE UNIQUE INDEX ... ON dashboards(scope,
COALESCE(owner, ''))` 는 modernc.org/sqlite 에서 정상 동작함이 SPEC-DASHBOARD-001 에서 검증됨
(sqlite.go:73-75 주석). 동일 기법을 NULL parent 루트 유니크에 적용한다.

```sql
CREATE TABLE IF NOT EXISTS assets (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    parent_id    INTEGER,
    kind         TEXT    NOT NULL,
    local_code   TEXT    NOT NULL,
    full_code    TEXT    NOT NULL,
    value        TEXT    NOT NULL,
    display_name TEXT,
    sort_order   INTEGER NOT NULL DEFAULT 0,
    source       TEXT    NOT NULL DEFAULT 'manual' CHECK (source IN ('auto','manual')),
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL,
    FOREIGN KEY (parent_id) REFERENCES assets(id)
);
CREATE UNIQUE INDEX IF NOT EXISTS assets_parent_localcode_uidx
    ON assets(COALESCE(parent_id, 0), local_code);
CREATE UNIQUE INDEX IF NOT EXISTS assets_parent_kind_value_uidx
    ON assets(COALESCE(parent_id, 0), kind, value);
CREATE UNIQUE INDEX IF NOT EXISTS assets_fullcode_uidx ON assets(full_code);
```

- `created_at`/`updated_at`: epoch ms(int64). `settings_sqlite.go:104` `time.Now().UnixMilli()`,
  sqlite `migrateUsersSchema` 의 `created_at INTEGER NOT NULL` 규약 확인.
- FK: SQLite 는 기본적으로 FK 강제가 꺼져 있으나(PRAGMA foreign_keys), 본 설계는 애플리케이션 레벨
  (`Get(parent)` 존재 확인 + `Delete` 리프-온리 체크)로 무결성을 강제하므로 FK 는 문서적/보조적이다.

## §3 코드 채번 규칙 (핵심 결정)

**결정 D-3a — 부모 스코프 순차 채번 + `(parent_id, local_code)` 유니크.**
`spec.md` "Data model to specify" 섹션이 `(parent_id, local_code)` 를 코드 유니크로 명시하고,
confirmed decision #2 가 "sequential within its parent" 를 명시한다. confirmed decision #2 의
부가 문장 "unique within (parent, kind)" 와의 미세한 긴장은 ASM-02(한 부모=한 kind)로 해소하며,
**권위 있는 Data model 섹션을 채택**하여 `(parent_id, local_code)` 를 채번/유니크 키로 확정한다.
(추가로 `(parent_id, kind, value)` 유니크는 값→코드 조회 dedup 용으로 별도 강제 — REQ-02.)

**결정 D-3b — zero-padding 폭 2, 100 이상 자연 확장.** 상수 `assetLocalCodeWidth = 2`.
`fmt.Sprintf("%0*d", width, n)`. n≥100 이면 폭이 자연 확장(`100`)되어 subway 스타일(`01`,`02`)과
호환. 정렬은 채번 순서(sort_order/created_at)로 보조.

**채번 알고리즘(트랜잭션):**
```
BEGIN
  SELECT id, full_code FROM assets
    WHERE COALESCE(parent_id,0)=? AND kind=? AND value=?      -- dedup 조회
  IF found → COMMIT; return (full_code, created=false)
  n := SELECT COALESCE(MAX(CAST(local_code AS INTEGER)),0)+1
         FROM assets WHERE COALESCE(parent_id,0)=?            -- 부모 스코프 카운터
  local := zeroPad(n, 2)
  full  := parentFullCode=="" ? local : parentFullCode + separator + local
  INSERT ... source='auto', created_at=updated_at=now
  COMMIT; return (full, created=true)
ON UNIQUE CONFLICT (동시 삽입) → ROLLBACK; 재조회로 수렴(REQ-06)
```
device_id `GetOrCreate`(device_id_repository.go:82)의 double-check 계약 + 저장 실패 롤백을 참조하되,
파일-JSON 대신 **SQLite 트랜잭션 + 유니크 제약**으로 원자화한다. 경합 시 유니크 위반 → 재조회.

**결정 D-3c — `parent` 인자 판별.** `GetOrCreate(ctx, parent, kind, value)` 의 `parent` 문자열:
- `""` → 루트(부모 없음).
- 전부 숫자(`^[0-9]+$`) → id 로 해석(내부/API 편의).
- 그 외 → full_code 로 해석(encode 노드는 항상 full_code 전달).
판별 후 부모 행을 조회해 `parent_id`·`parent.full_code` 를 얻는다. 부모 미존재 → `ErrAssetNotFound`(REQ-08).

**결정 D-3d — separator.** 상수 `assetCodeSeparator = "."`. 저장소 생성 옵션으로 재정의 가능하되
기본 `.`. full_code 는 저장(denormalized)되어 역방향은 `assets_fullcode_uidx` 단일 룩업이다.

## §4 노드 의존성 주입 — Pattern B 채택

**결정 D-4 — Pattern B(함수형 resolver).** 후보 비교:

| | Pattern A (`WithAgentResolver`) | Pattern B (`WithXxxFunc`) |
|--|--|--|
| 선례 | `bridge.go:60`, `store_write.go:180-253` | `inventory.go:120` `WithDeviceRegistryFunc` |
| 대상 | 특정 에이전트(`AgentRef` 로 해석) | 프로세스 전역 싱글턴(지연 획득) |
| asset 적합성 | ✗ asset 레지스트리는 특정 에이전트가 아님 | ✓ deviceRegistry 처럼 전역 |
| 초기화 순서 | resolver 준비 필요 | 함수형 지연 → 순서 문제 회피 |

→ **Pattern B**. `inventory.go:120-127` 를 정확히 미러:
```go
// internal/node/asset.go
type AssetRegistry interface {
    GetOrCreate(ctx context.Context, parent, kind, value string) (string, bool, error)
    ResolveByCode(ctx context.Context, fullCode string) (string, bool, error)
}
const assetRegistryFnKey = "_asset_registry_fn"
var ErrAssetRegistryNotAvailable = fmt.Errorf(
    "asset: %w: asset registry not configured (use WithAssetRegistryFunc)", ErrNodeNotInitialized)

func WithAssetRegistryFunc(fn func() AssetRegistry) NodeOption {
    return func(b *BaseNode) {
        if b.config == nil { b.config = make(map[string]any) }
        b.config[assetRegistryFnKey] = fn
    }
}
```
`ErrNodeNotInitialized` 는 inventory sentinel 이 래핑하는 기존 에러(inventory.go:81 확인). 팩토리는
`base.config[assetRegistryFnKey].(func() AssetRegistry)` 로 추출(inventory.go:247-258 패턴).

**main.go 배선(M2):** line 515-544 의 inventory 옵션 블록과 동일 위치에 추가.
```go
// M1 저장소 인스턴스(공유 핸들 재사용 — main.go:699/870 선례)
assetRepo, err := storage.NewAssetSQLiteRepositoryWithDB(context.Background(), authDashboardDB)
...
assetRegistryOpt := node.WithAssetRegistryFunc(func() node.AssetRegistry { return assetRepo })
// engine.WithNodeOptions(...) (line 529) 목록에 assetRegistryOpt 추가
```
`storage.AssetSQLiteRepository` 가 `node.AssetRegistry`(2메서드)를 구조적으로 만족한다(덕 타이핑).

## §5 노드 Process 흐름

### asset-encode (M3)
`store_write.go` 의 `resolveTemplateExpr(expr, msg) (any, error)`(line 899, `$.payload.x`/`$.metadata.x`
지원)와 `Payload().Set`(payload.go:57)을 사용한다.
```
for each mapping{field,kind,parent}:
    val, err := resolveTemplateExpr(field, msg)       // 필드 부재 → skip(REQ-13)
    if err != nil { continue }
    parentCode := parent 이 "$." prefix면 resolveTemplateExpr(parent,msg) 문자열화, else 리터럴
    full, created, err := reg.GetOrCreate(ctx, parentCode, kind, fmt.Sprint(val))
    if err != nil { return nil, err }                 // 부모 미존재 등 → 에러(REQ-14)
    msg.Payload().Set(lastPathKey(field), full)       // 전체 코드로 치환
return []message.Message{msg}, nil                    // passthrough
```
`lastPathKey`(mapping.go:150)로 `$.payload.spot` → `spot` 키에 Set. (payload 최상위 키 기록;
중첩 경로 기록이 필요하면 후속 확장 — 본 SPEC 은 최상위 필드 치환으로 단순화, Enforce Simplicity.)

### asset-decode (M4)
`mapping.go:101-141` `Process`(default-부재 폴백)를 모델로 한다.
```
raw, err := message.NewPayload(messageToMap(msg)).GetPath(field)  // mapping.go:113 idiom
if err != nil || raw == nil { return []message.Message{msg}, nil } // 코드 없음 → 무변경 passthrough
value, found, err := reg.ResolveByCode(ctx, fmt.Sprint(raw))
if err != nil { return nil, err }
if !found { return []message.Message{msg}, nil }                   // raw passthrough(REQ-17)
if target != "" { msg.Payload().Set(target, value) } else { msg.Payload().Set(lastPathKey(field), value) }
return []message.Message{msg}, nil
```
mapping.go 의 `messageToMap`/`NewPayload`/`GetPath`/`lastPathKey` 재사용.

## §6 노드 등록

`registry.go:73` `registerBuiltins()` builtins 슬라이스에 2행 추가(테이블 리터럴):
```go
{"asset-encode", NewAssetEncodeNode, "processing", "자산 값을 계층 코드로 인코딩(미존재 시 자동 등록)"},
{"asset-decode", NewAssetDecodeNode, "processing", "계층 코드를 자산 값으로 디코딩(미발견 시 원값 통과)"},
```
`NodeFactory` 시그니처 `func(def flow.NodeDef, opts ...NodeOption) (Node, error)`(registry.go:10) 준수.

## §7 CRUD API (M5)

`store_query.go`/`settings.go` 패턴. `api.Context`(router.go:24: `Param`/`Query`/`QueryValues`/
`Bind`/`JSON`/`Context()`), `dto.NewSuccessResponse`, `api.ErrBadRequest`/`ErrNotFound`/`ErrConflict`.
Go 1.22 mux path param(`store_query.go:166` `g.GET("/store/{agent_name}/keys", ...)` 선례).
```
GET    /assets                 list (?parent=<id|code>&kind=<k>)  → tree/flat
GET    /assets/{id}            get
POST   /assets                 create (manual)   body: {parent, kind, value, display_name?, sort_order?}
PUT    /assets/{id}            update            body: {display_name?, sort_order?, value?}
DELETE /assets/{id}            delete (?cascade=true)
```
`main.go:876` 중앙 `RegisterRoutes` 클로저에 `assetHandler.RegisterRoutes(g)` 추가(settings 선례
line 911). 핸들러 생성은 `NewAssetHandler(assetRepo, logger)`(settings.go:55 `NewSettingsHandler` 선례).

## §8 CSV 시더 (M6, 선택)

`examples/subway/*.csv` 실측 형태:
- `station.csv`: `# station,line,name,order` → 예 `st01,ui-line,신설동,1`
- `spot.csv`: `# station,code,name,floor` → 예 `st01,01,상행계단#1,1`
- `device.csv`: `# station,code,index` → 예 `st01,01,1`

매핑 결정 D-8:
- line(root, kind=`line`, value=`ui-line`) → station(parent=line, kind=`station`, value=`st01`/name).
- spot(parent=station, kind=`spot`, value=name). CSV 의 `code`(01)는 참고용; 시더는 순차 채번을
  신뢰(또는 manual local_code 지정 옵션 — design 여지). **주의:** CSV `code` 와 채번 결과가 일치하도록
  등록 순서를 `order`/`code` 로 정렬한다.
- device(parent=spot, kind=`device`) — device.csv 는 `station,code` 만 있어 spot 을 `code` 로 역참조.
`#` 로 시작하는 헤더 라인 스킵. `source="manual"`. **본 절은 M6 착수 시에만 구현**(REQ-S1 Optional).

## §9 기존 xsfm station_registry 통합 노트 (범위 밖 — 기록만)

`internal/storage/station_registry_repository.go:15` `StationRegistryEntry{Station, StationNumber,
Line, DisplayName, Order, Places[]}`(파일-JSON, xsfm 전용)와 `internal/agent/xsfm/station_registry.go`
(`UpsertStation`/`ResolveLine`/`ResolveStationNumber`)는 line↔station↔place 개념이 본 Asset Manager
트리(line→station→spot)와 **개념적으로 중복**된다. 그러나:
- 본 SPEC 은 **greenfield·DB 기반·제네릭**이고, xsfm 레지스트리는 **파일-JSON·xsfm 도메인 특화**이다.
- 향후 xsfm 레지스트리를 Asset Manager 위로 이관(place→asset kind='place', line/station 매핑)하는
  통합이 가능하나, 행위 보존 리팩터 리스크가 크므로 **본 SPEC 범위 밖**으로 명시한다(Non-Goals).
- 통합 시 고려: xsfm 의 `StationNumber`(대외 표시 번호)는 asset 의 `display_name`/추가 컬럼으로,
  `Places[]` 는 자식 asset(kind='place')로 매핑. 이는 **후속 SPEC**의 설계 입력으로만 남긴다.

## §10 단순성 원칙 준수 (Enforce Simplicity)

- 트리만 지원(다중 부모/그래프/간선 속성 금지).
- 레지스트리 표면은 `GetOrCreate` + `ResolveByCode` + CRUD 로 최소화.
- full_code denormalized 저장으로 역방향을 단일 인덱스 룩업으로 단순화(재귀 CTE 회피).
- 부모 이동/리네임 캐스케이드 미구현(최초 채번만). 필요 시 후속 SPEC.
- 노드↔저장소 최소 인터페이스 분리(store_write/inventory 선례) — 과설계 회피.
