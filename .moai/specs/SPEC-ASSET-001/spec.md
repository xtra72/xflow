---
id: SPEC-ASSET-001
title: "Asset Manager — 계층형 자산 코드 레지스트리 + encode/decode 파이프라인 노드"
version: "0.1.0"
status: draft
created: 2026-08-11
updated: 2026-08-11
author: xtra
priority: P2
phase: "v0.19.0"
module: "internal/storage, internal/node"
lifecycle: spec-anchored
tags: "asset, registry, hierarchy, sqlite, node, encode, decode, greenfield"
---

# SPEC-ASSET-001 — Asset Manager (계층형 자산 코드 레지스트리)

## HISTORY

| 일자 | 버전 | 변경 | 작성자 |
|------|------|------|--------|
| 2026-08-11 | 0.1.0 | 최초 작성. 단일 `assets` 테이블(`kind` + `parent_id` 제네릭 트리) 기반 SQLite 레지스트리, 부모별 순차 로컬 코드 + `.` 조인 전체 코드, `asset-encode`(값→코드 자동등록)·`asset-decode`(코드→값 raw 폴백) 파이프라인 노드, `AssetRepository`/`AssetSQLiteRepository`(GetOrCreate/ResolveByCode/CRUD), CRUD API 핸들러, (스트레치) subway CSV 시더. Tier L, EARS 5모듈. | xtra |

## 개요 (Overview)

xflow 백엔드에 **자산(asset)을 계층형 코드로 관리하는 레지스트리**를 추가한다. 자산은
위치(location)·구역(spot)·역사(station)·디바이스(device) 등 임의 종류(`kind`)를 가지며,
자기참조 `parent_id` 로 임의 깊이의 트리를 구성한다. 각 자산은 부모 안에서 **순차적으로
할당된 로컬 코드**(subway 스타일 `01`, `02`, …)를 갖고, **부모의 전체 코드**에 구분자
(기본 `.`)로 로컬 코드를 이어 붙인 **전체 코드**(예: `st01.01`)를 갖는다. 이 레지스트리는
SQLite(`assets` 테이블)에 영속되며, 재시작 후에도 값↔코드 매핑이 안정적으로 유지된다.

레지스트리는 두 개의 파이프라인 노드로 소비된다:

- **`asset-encode`(정방향)**: 메시지의 지정 필드에서 자산 값을 읽어 `(부모, kind, 값)` 으로
  레지스트리를 조회하고, 없으면 자동 등록한 뒤 그 필드 값을 **전체 코드**로 치환한다.
- **`asset-decode`(역방향)**: 지정 필드의 코드를 값으로 복원하며, 코드가 레지스트리에 없으면
  원래 값을 그대로 통과시킨다(passthrough).

부모 지정은 노드 config 에서 필드별로 (a) 고정 부모 전체 코드(경로) 또는 (b) 메시지에서
부모를 파생하는 `$.`-JSONPath 로 지정한다.

기존 xsfm `station_registry`(파일-JSON, xsfm 전용)는 **본 SPEC 의 범위가 아니며 리팩터하지
않는다**. 신규 Asset Manager 는 독립적이고 DB 기반이다(§ 비목표 및 design.md §9 통합 노트).

### 비목표 (Non-Goals)

- 기존 xsfm `internal/storage/station_registry_repository.go` / `internal/agent/xsfm/station_registry.go`
  의 리팩터·통합·마이그레이션(향후 통합은 design.md §9 설계 노트로만 기록, 범위 밖).
- 트리를 넘어서는 일반 그래프(다중 부모·순환·간선 속성) 기능. 본 SPEC 은 **단일 부모 트리**만 지원.
- 코드 재계산/부모 이동에 따른 하위 전체 코드 대량 재작성(리네이밍/re-parent 캐스케이드)의
  자동화. 본 SPEC 은 최초 생성 시점의 코드 채번만 다룬다(design.md §5 참고).
- TSDB/시계열 연동, 대시보드 UI. 본 SPEC 은 백엔드 레지스트리 + 노드 + CRUD API 만 다룬다.

## 용어 (Terminology)

- **asset(자산)**: 레지스트리의 단일 노드. 컬럼 `kind`, `parent_id`, `local_code`,
  `full_code`, `value` 등을 가진다.
- **kind(종류)**: 자산의 레벨/분류 라벨(예: `location`/`spot`/`station`/`device`). 하드코딩하지
  않으며 자유 문자열로 확장 가능하다.
- **value(값)**: 자산을 값→코드 조회할 때 사용하는 원본 자산 값/이름(예: `신설동`, `상행계단#1`).
- **local_code(로컬 코드)**: 부모 안에서 순차 채번된 코드(zero-padded, 기본 폭 2 — `01`, `02`, …).
- **full_code(전체 코드)**: 루트→해당 자산까지 로컬 코드를 구분자로 이은 경로 코드(예: `st01.01.03`).
  루트(부모 없음)의 전체 코드는 로컬 코드 그 자체이다.
- **separator(구분자)**: 전체 코드 조인 문자. 기본 `.`.
- **source**: 자산 등록 출처. `auto`(노드가 자동 등록) 또는 `manual`(API/시더로 등록).

## 환경 (Environment)

- 언어/런타임: Go 1.23+, `modernc.org/sqlite`(Pure Go 드라이버, 기존 xflow.db 공유).
- 저장소 패턴: 인터페이스 + sqlite 구현 + factory 스위치(기존 `FlowRepository`/`SettingsRepository`
  패턴 준용). 마이그레이션은 `CREATE TABLE IF NOT EXISTS` 멱등.
- 노드 프레임워크: `internal/node`(Node 인터페이스, `NewBaseNode`, `NodeOption`, `Registry.registerBuiltins`).
- 메시지: `pkg/message`(`Payload().Set/Get`, `GetPath`, `New`).
- API: `internal/api`(RouteGroup, Go 1.22 mux 패턴 `"GET /path/{param}"`, `api.Context`).
- development_mode: `hybrid` → 신규 코드는 **TDD**(RED-GREEN-REFACTOR), 85% 커버리지 목표.

## 가정 (Assumptions)

- ASM-01: 신규 `assets` 테이블은 기존 공유 `xflow.db` 에 `IF NOT EXISTS` 로 안전 추가되며
  WAL 모드에서 다중 핸들과 공존한다(기존 dashboards/users/settings 와 동일 — sqlite.go 주석 ASM-007).
- ASM-02: 실무상 한 부모(parent)는 한 종류(kind)의 자식만 갖는다(예: station 의 자식은 spot).
  따라서 "부모 안 순차 로컬 코드"와 "(parent, kind) 유니크"는 실질적으로 일치한다. 채번은
  **부모 스코프**로 하고 유니크 제약은 `(parent_id, local_code)` 로 강제한다(design.md §3 결정).
- ASM-03: 로컬 코드 zero-padding 폭은 기본 2(`01`…`99`), 100 이상은 자연 폭 확장(`100`). 폭은
  상수로 정의하며 폭 확장은 정렬을 깨지 않는 범위에서 허용한다(design.md §3).
- ASM-04: 값(value)은 부모+종류 스코프에서 조회 키다. 같은 값이라도 부모가 다르면 서로 다른
  자산·코드가 된다(ASM 트리 성질).
- ASM-05: full_code 는 전역 유니크하며 별도 컬럼에 저장(denormalized)하여 역방향(코드→값) 조회를
  단일 인덱스 룩업으로 처리한다.
- ASM-06: 노드 의존성 주입은 inventory 노드와 동일한 **함수형 resolver(Pattern B)** 로 하며
  (`WithAssetRegistryFunc(func() AssetRegistry)`), 초기화 순서 문제를 회피한다(design.md §4 결정).
- ASM-07: 동시 `GetOrCreate` 는 트랜잭션 + 유니크 제약으로 직렬화되어, 동일 `(부모, kind, 값)`
  에 대해 항상 하나의 코드만 발급된다(경합 시 재조회로 수렴).

## 요구사항 (Requirements — EARS)

EARS 5모듈, 총 18 REQ. 키워드/식별자는 English, 서술은 Korean.

### 모듈 M1 — Asset 데이터 모델 & 저장소 (`assets` table + `AssetRepository`)

- **REQ-01 (Ubiquitous)**: The system **shall** persist assets in a single `assets` table
  containing `id`, `parent_id`(nullable self-FK), `kind`, `local_code`, `full_code`, `value`,
  `display_name`, `sort_order`, `source`, `created_at`, `updated_at`.
  (시스템은 **항상** 위 컬럼을 가진 단일 `assets` 테이블에 자산을 영속한다.)
- **REQ-02 (Ubiquitous)**: The system **shall** enforce uniqueness of `(parent_id, local_code)`,
  `(parent_id, kind, value)`, and global `full_code`, and **shall** index `full_code`.
  (시스템은 **항상** 세 유니크 제약과 full_code 인덱스를 강제한다. NULL parent 는 `COALESCE(parent_id, 0)`
  로 정규화하여 루트 유니크를 보장한다 — dashboards 인덱스 선례.)
- **REQ-03 (Event-Driven)**: **When** `GetOrCreate(ctx, parent, kind, value)` is called and no
  asset matches `(parent, kind, value)`, the system **shall** mint the next sequential local code
  within the parent, compute `full_code = parent.full_code + separator + local_code` (root →
  `full_code = local_code`), persist with `source="auto"`, and return `(full_code, created=true)`.
  (미존재 시 부모 스코프 순차 로컬 코드 채번 → 전체 코드 계산 → auto 저장 → `(전체코드, created=true)` 반환.)
- **REQ-04 (Event-Driven)**: **When** `GetOrCreate` is called and an asset already matches
  `(parent, kind, value)`, the system **shall** return the existing `full_code` with
  `created=false` and **shall not** create a duplicate.
  (기존 매칭 시 기존 전체 코드 + `created=false` 반환, 중복 생성 금지.)
- **REQ-05 (Event-Driven)**: **When** `ResolveByCode(ctx, fullCode)` is called, the system
  **shall** return `(value, found=true)` if the code exists, else `("", found=false)` with no error.
  (코드 존재 시 값+true, 미존재 시 빈 값+false, 에러 아님.)
- **REQ-06 (State-Driven)**: **While** two concurrent `GetOrCreate` calls request the same
  `(parent, kind, value)`, the system **shall** issue exactly one code (one creator, the other
  reads the same code) via transaction + unique-constraint retry.
  (동시 요청 시 정확히 하나의 코드만 발급.)
- **REQ-07 (Ubiquitous)**: The system **shall** expose tree-aware CRUD —
  `List`, `Get`, `Create`, `Update`, `Delete` — where `Create` accepts an explicit
  `source="manual"` and `Delete` rejects deleting an asset that has children (leaf-only delete)
  unless cascade is explicitly requested.
  (트리 인지 CRUD 제공. Create 는 manual 출처, Delete 는 자식 있는 자산 거부(리프-온리) — cascade 명시 시 예외.)
- **REQ-08 (Unwanted)**: **If** `parent` refers to a non-existent asset (by id or full_code),
  **then** the system **shall not** create the child and **shall** return a not-found error.
  (부모 미존재 시 자식 생성 금지 + not-found 에러.)

### 모듈 M2 — 노드 의존성 주입 (`WithAssetRegistryFunc`)

- **REQ-09 (Ubiquitous)**: The system **shall** provide a `WithAssetRegistryFunc(fn func() AssetRegistry)`
  NodeOption that injects the registry via a functional resolver (mirroring inventory's
  `WithDeviceRegistryFunc`), stored under a private config key and consumed at node construction.
  (inventory 선례와 동일한 함수형 resolver NodeOption 제공.)
- **REQ-10 (State-Driven)**: **While** an `asset-encode`/`asset-decode` node is initialized and no
  registry resolver was injected, the system **shall** return a sentinel error at `Init` (fail-fast),
  not at `Process`.
  (레지스트리 미주입 시 Init 에서 sentinel 에러 — inventory `ErrInventory*NotAvailable` 선례.)

### 모듈 M3 — `asset-encode` 노드 (값 → 코드, 자동등록)

- **REQ-11 (Ubiquitous)**: The system **shall** register a builtin node type `asset-encode` in
  `registerBuiltins()` whose config declares one or more field mappings, each with: `field`
  (`$.`-path to the value), `kind`, and `parent` (a fixed parent full_code/path OR a `$.`-path to
  derive the parent from the message).
  (빌트인 `asset-encode` 등록. config 는 필드 매핑 목록(field/kind/parent) 선언.)
- **REQ-12 (Event-Driven)**: **When** an `asset-encode` node processes a message, for each mapping
  it **shall** read the value at `field`, resolve the parent (literal or `$.`-derived), call
  `GetOrCreate(parent, kind, value)`, and **shall** replace the field value with the returned
  full code, then pass the message through.
  (각 매핑에 대해 값 읽기 → 부모 해석 → GetOrCreate → 필드를 전체 코드로 치환 → passthrough.)
- **REQ-13 (Unwanted)**: **If** the `field` path is absent in the message, **then** the node
  **shall not** auto-register and **shall** leave that mapping's field unchanged (skip, not error),
  emitting the message unchanged for that mapping.
  (필드 부재 시 해당 매핑은 자동등록/치환하지 않고 건너뛴다 — 값 보존.)
- **REQ-14 (Unwanted)**: **If** the resolved `parent` full_code does not exist in the registry,
  **then** the node **shall** surface a processing error for that message (no silent orphan
  creation) — parents are not auto-created by encode.
  (부모 전체 코드 미존재 시 처리 에러 — encode 는 부모를 자동 생성하지 않는다.)

### 모듈 M4 — `asset-decode` 노드 (코드 → 값, raw 폴백)

- **REQ-15 (Ubiquitous)**: The system **shall** register a builtin node type `asset-decode` whose
  config declares the source `field` (`$.`-path to the code) and an optional `target` (output
  field; empty overwrites the source field), modeled on the `mapping` node's Configure idiom.
  (빌트인 `asset-decode` 등록. config: field(코드 경로) + 선택 target, mapping 노드 idiom 준용.)
- **REQ-16 (Event-Driven)**: **When** an `asset-decode` node processes a message, it **shall** read
  the code at `field`, call `ResolveByCode`, and **shall** write the resolved value to the target
  (or overwrite the source field) when `found=true`.
  (코드 읽기 → ResolveByCode → found 시 값 기록.)
- **REQ-17 (Unwanted)**: **If** the code is not found in the registry, **then** the node **shall**
  leave the value as-is (raw passthrough), modeled on the `mapping` node default-when-absent
  behavior (`internal/node/mapping.go`).
  (코드 미발견 시 값 그대로 통과 — mapping 노드 default 부재 폴백 모델.)

### 모듈 M5 — CRUD API 핸들러

- **REQ-18 (Ubiquitous)**: The system **shall** expose asset CRUD over HTTP via a new
  `internal/api/handler/asset.go` handler registered through the central `RegisterRoutes` wiring,
  following the `store_query.go`/`settings.go` handler pattern (`api.Context`, `dto.NewSuccessResponse`,
  Go 1.22 mux path params), covering list (tree/flat), get, create(manual), update, delete.
  (신규 asset 핸들러로 CRUD HTTP 노출 — 기존 핸들러 패턴 준용, 중앙 RegisterRoutes 에 배선.)

### 스트레치 (Optional — M6, 명시적 선택 범위)

- **REQ-S1 (Optional)**: **Where** a CSV importer is provided, the system **shall** seed the
  registry from `examples/subway/{station,spot,device}.csv` (net-new; no importer exists today),
  mapping line→station→spot→device with `source="manual"`. 본 요구는 **선택(deferrable)** 이며
  M1–M5 완료 후 별도 승인 시에만 착수한다.
  (선택: subway CSV 시더 — line→station→spot→device 계층으로 manual 등록. M6 는 연기 가능.)

## 명세 (Specifications)

### AssetRegistry / AssetRepository 계약

```go
// AssetRegistry 는 노드가 소비하는 최소 계약(값↔코드).
// 순환 의존을 피하기 위해 node 패키지에 최소 인터페이스로 재선언한다(store_write.StoreWriter 선례).
type AssetRegistry interface {
    GetOrCreate(ctx context.Context, parent, kind, value string) (fullCode string, created bool, err error)
    ResolveByCode(ctx context.Context, fullCode string) (value string, found bool, err error)
}

// AssetRepository 는 storage 계층의 완전한 계약(레지스트리 + tree-aware CRUD).
type AssetRepository interface {
    AssetRegistry
    List(ctx context.Context, filter AssetFilter) ([]Asset, error)
    Get(ctx context.Context, id int64) (Asset, error)
    Create(ctx context.Context, in AssetCreate) (Asset, error) // source="manual"
    Update(ctx context.Context, id int64, in AssetUpdate) (Asset, error)
    Delete(ctx context.Context, id int64, cascade bool) error
    Close() error
}
```

`parent` 인자는 **id 문자열 또는 full_code** 를 허용한다(design.md §3 에서 판별 규칙 명시:
숫자만이면 id, 그 외는 full_code 로 간주하되, encode 노드는 항상 full_code 를 전달).

### `assets` 테이블 스키마 (design.md §2 상세)

```sql
CREATE TABLE IF NOT EXISTS assets (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    parent_id    INTEGER,                    -- nullable self-FK (root = NULL)
    kind         TEXT    NOT NULL,
    local_code   TEXT    NOT NULL,           -- zero-padded, parent-scoped sequential
    full_code    TEXT    NOT NULL,           -- denormalized path code (globally unique)
    value        TEXT    NOT NULL,           -- lookup key (value→code)
    display_name TEXT,
    sort_order   INTEGER NOT NULL DEFAULT 0,
    source       TEXT    NOT NULL DEFAULT 'manual' CHECK (source IN ('auto','manual')),
    created_at   INTEGER NOT NULL,           -- epoch ms (project convention)
    updated_at   INTEGER NOT NULL,
    FOREIGN KEY (parent_id) REFERENCES assets(id)
);
CREATE UNIQUE INDEX IF NOT EXISTS assets_parent_localcode_uidx
    ON assets(COALESCE(parent_id, 0), local_code);
CREATE UNIQUE INDEX IF NOT EXISTS assets_parent_kind_value_uidx
    ON assets(COALESCE(parent_id, 0), kind, value);
CREATE UNIQUE INDEX IF NOT EXISTS assets_fullcode_uidx ON assets(full_code);
```

### 노드 config 예 (asset-encode)

```yaml
# asset-encode: 메시지 필드 값을 전체 코드로 치환(자동 등록)
type: asset-encode
config:
  mappings:
    - field: "$.payload.station"        # 값 위치
      kind: "station"
      parent: "$.payload.line_code"     # 메시지에서 부모(라인) 전체 코드 파생
    - field: "$.payload.spot"
      kind: "spot"
      parent: "$.payload.station"       # 위 매핑이 먼저 치환한 station 전체 코드
```

### 노드 config 예 (asset-decode)

```yaml
type: asset-decode
config:
  field: "$.payload.code"   # 코드 위치
  target: "asset_name"      # 선택: 결과 기록 필드(비우면 소스 덮어쓰기)
```

### 타임스탬프 규약

`created_at`/`updated_at` 는 **epoch milliseconds(int64, `time.Now().UnixMilli()`)** — 프로젝트
규약(settings/dashboards 선례). RFC3339 나 `time.Time` 저장 금지.

## Traceability

| REQ | 모듈 | 구현 산출물(anchor) | AC |
|-----|------|---------------------|----|
| REQ-01..02 | M1 | `internal/storage/asset_sqlite.go`(`migrateAssetSchema`, sqlite.go 선례) | AC-01, AC-02 |
| REQ-03..06 | M1 | `asset_sqlite.go` `GetOrCreate`(device_id `GetOrCreate` 선례 + tx) | AC-03..06, AC-11 |
| REQ-05 | M1 | `asset_sqlite.go` `ResolveByCode` | AC-05, AC-09 |
| REQ-07..08 | M1 | `asset_repository.go`(interface) + `asset_sqlite.go`(CRUD), `factory.go` 스위치 | AC-07, AC-08 |
| REQ-09..10 | M2 | `internal/node/asset.go`(`WithAssetRegistryFunc`, inventory 선례) + `cmd/xflowd/main.go:529` WithNodeOptions | AC-10 |
| REQ-11..14 | M3 | `internal/node/asset_encode.go` + `registry.go:73` 등록 | AC-03, AC-07-encode, AC-12, AC-14 |
| REQ-15..17 | M4 | `internal/node/asset_decode.go`(mapping.go 선례) + `registry.go:73` 등록 | AC-09, AC-13 |
| REQ-18 | M5 | `internal/api/handler/asset.go`(store_query/settings 선례) + `main.go:876` RegisterRoutes | AC-15 |
| REQ-S1 | M6(opt) | `internal/storage/asset_seed.go`(net-new CSV 시더) | AC-16 |

Acceptance criteria 상세: `acceptance.md`. 구현 계획/마일스톤: `plan.md`. 기술 설계: `design.md`.
코드베이스 조사·앵커: `research.md`.
