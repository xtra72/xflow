# SPEC-ASSET-001 — 수용 기준 (acceptance.md)

> Given-When-Then 형식. 각 AC 는 검증 가능하며 REQ/마일스톤에 추적된다.
> 테스트 위치는 `plan.md` 마일스톤 참조. development_mode=hybrid → 각 AC 는 TDD 테스트로 선구현.

## M1 — 저장소 (`AssetRepository`/`AssetSQLiteRepository`)

### AC-01 — 스키마 멱등 생성 (REQ-01, REQ-02)
- **Given** 빈 SQLite DB
- **When** `NewAssetSQLiteRepository`(또는 `...WithDB`) 를 두 번 호출
- **Then** `assets` 테이블과 3개 유니크 인덱스(`assets_parent_localcode_uidx`,
  `assets_parent_kind_value_uidx`, `assets_fullcode_uidx`)가 생성되고, 재호출해도 에러 없이 멱등하다.

### AC-02 — 계층 유니크 제약 (REQ-02)
- **Given** 부모 A(full_code `a`)와 부모 B(full_code `b`)
- **When** A 밑에 `(kind=spot, value="계단")`, B 밑에 `(kind=spot, value="계단")` 을 각각 등록
- **Then** 서로 다른 자산·서로 다른 full_code(`a.01`, `b.01`)가 만들어진다(같은 value 라도 부모가
  다르면 별개). 또한 서로 다른 부모 밑에서 같은 local_code(`01`)가 공존할 수 있다.

### AC-03 — 값→코드 자동 등록 (신규) (REQ-03)
- **Given** 부모 `st01`(full_code `st01`)이 등록되어 있고, 그 밑에 자식이 없음
- **When** `GetOrCreate(ctx, "st01", "spot", "상행계단#1")` 호출
- **Then** 반환 `(fullCode="st01.01", created=true)`, `source="auto"`, local_code `01`(zero-pad 폭 2),
  `full_code = "st01" + "." + "01"` 로 저장된다.

### AC-04 — 값→코드 재조회(중복 없음) (REQ-04)
- **Given** AC-03 이후 `st01` 밑에 `("spot","상행계단#1")` 가 이미 존재
- **When** 동일하게 `GetOrCreate(ctx, "st01", "spot", "상행계단#1")` 재호출
- **Then** 반환 `(fullCode="st01.01", created=false)`, 새 행이 생성되지 않는다(row count 불변).

### AC-05 — 순차 채번 (REQ-03)
- **Given** `st01` 밑에 spot 3개를 순서대로 등록
- **When** `GetOrCreate` 를 3회(서로 다른 value) 호출
- **Then** local_code 가 `01`, `02`, `03` 으로 순차 발급되고 full_code 는 `st01.01`, `st01.02`,
  `st01.03` 이다(부모 스코프 순차).

### AC-06 — 루트 자산 코드 (REQ-03)
- **Given** 부모 없이(root) 등록
- **When** `Create`(manual) 또는 encode 경로로 root kind=line 자산 2개 등록
- **Then** root 의 full_code = local_code(`01`, `02` — 구분자 없음), `parent_id` 는 NULL,
  `COALESCE(parent_id,0)` 유니크로 루트 중복이 방지된다.

### AC-07 — tree-aware CRUD (REQ-07)
- **Given** 빈 레지스트리
- **When** `Create`(manual root) → `Create`(child) → `Get(id)` → `Update`(display_name) →
  `List(filter{parent})` → `Delete(child)`
- **Then** 각 연산이 기대대로 동작하고, `Create` 는 `source="manual"`, `List` 는 부모 필터로
  자식만 반환, `Delete` 는 자식 없는 자산을 제거한다.

### AC-08 — 부모 미존재 거부 (REQ-08) & 리프-온리 삭제 (REQ-07)
- **Given** 존재하지 않는 부모 참조 / 자식 있는 부모
- **When (a)** 존재하지 않는 `parent` 로 자식 `Create`/`GetOrCreate`
- **Then (a)** `ErrAssetNotFound` 반환, 자식 미생성.
- **When (b)** 자식이 있는 자산을 `Delete(id, cascade=false)`
- **Then (b)** `ErrAssetHasChildren` 반환(거부). `Delete(id, cascade=true)` 시에만 하위 포함 삭제.

### AC-09 — ResolveByCode (REQ-05)
- **Given** `st01.01 → "상행계단#1"` 이 등록됨
- **When (a)** `ResolveByCode(ctx, "st01.01")`
- **Then (a)** `("상행계단#1", found=true, err=nil)`.
- **When (b)** `ResolveByCode(ctx, "zz99.99")`(미존재)
- **Then (b)** `("", found=false, err=nil)` — 에러 아님.

### AC-11 — 동시 GetOrCreate 단일 코드 (REQ-06)
- **Given** 부모 `st01`, 동일 `(kind=spot, value="계단")`
- **When** N개(예: 20) 고루틴이 동시에 `GetOrCreate(ctx,"st01","spot","계단")` 호출
- **Then** 모두 동일한 full_code 를 반환하고, `assets` 에는 정확히 1개의 새 행만 생성된다
  (중복 채번 없음). `go test -race` 로 검증.

## M2 — 노드 의존성 주입

### AC-10 — 함수형 resolver 주입 & fail-fast (REQ-09, REQ-10)
- **Given** `WithAssetRegistryFunc(fn)` 로 생성된 `asset-encode`/`asset-decode` 노드
- **When (a)** 옵션 주입 후 팩토리 생성 → `Init`
- **Then (a)** 노드가 config 키 `_asset_registry_fn` 에서 resolver 를 추출하여 `Init` 통과.
- **When (b)** resolver 미주입 상태로 `Init`
- **Then (b)** `ErrAssetRegistryNotAvailable`(sentinel) 를 `Init` 에서 반환(fail-fast, Process 아님).

## M3 — `asset-encode` 노드

### AC-12 — encode 필드 치환 (REQ-11, REQ-12)
- **Given** `st01`(parent line 하위 station) 이 존재하고 노드 config:
  `mappings: [{field:"$.payload.spot", kind:"spot", parent:"$.payload.station_code"}]`,
  메시지 `payload: {station_code:"st01", spot:"상행계단#1"}`
- **When** `Process(msg)`
- **Then** `GetOrCreate("st01","spot","상행계단#1")` 가 호출되어 자동 등록되고, 메시지의
  `$.payload.spot` 이 전체 코드 `st01.01` 로 치환된 채 passthrough 된다.

### AC-14 — encode 부모 미존재 에러 (REQ-14)
- **Given** parent full_code(`zz99`)가 레지스트리에 없음
- **When** `asset-encode` 가 그 parent 로 매핑을 처리
- **Then** 해당 메시지 처리에서 에러를 표면화한다(고아 자동 생성 없음 — encode 는 부모를 만들지 않는다).

### AC-13a — encode 필드 부재 skip (REQ-13)
- **Given** 매핑의 `field`(`$.payload.spot`)가 메시지에 없음
- **When** `Process(msg)`
- **Then** 해당 매핑은 자동등록/치환하지 않고 건너뛴다(값 보존, 에러 아님). 다른 매핑은 정상 처리.

## M4 — `asset-decode` 노드

### AC-13 — decode + raw passthrough (REQ-15, REQ-16, REQ-17)
- **Given** `st01.01 → "상행계단#1"` 등록, config `{field:"$.payload.code", target:"name"}`
- **When (a)** `payload:{code:"st01.01"}` 로 `Process`
- **Then (a)** `ResolveByCode` 결과 `"상행계단#1"` 이 `$.payload.name` 에 기록된다.
- **When (b)** `payload:{code:"unknown.99"}`(미등록 코드)
- **Then (b)** 값이 그대로 통과되어 `code` 는 `"unknown.99"` 로 유지되고(원값 보존), 노드는 에러 없이
  메시지를 passthrough 한다(mapping.go default-부재 폴백 모델).
- **When (c)** target 비움
- **Then (c)** found 시 소스 `field` 자체를 덮어쓴다.

## M5 — CRUD API

### AC-15 — CRUD 라운드트립 (REQ-18)
- **Given** asset 핸들러가 중앙 RegisterRoutes 에 배선됨
- **When** `POST /assets`(manual root) → `POST /assets`(child) → `GET /assets/{id}` →
  `GET /assets?parent=<root>` → `PUT /assets/{id}` → `DELETE /assets/{id}`
- **Then**
  - 생성/조회/수정/삭제가 `dto.NewSuccessResponse` 형태로 200/201 응답,
  - 존재하지 않는 id `GET`/`PUT`/`DELETE` → 404(`api.ErrNotFound`),
  - 자식 있는 자산 `DELETE`(cascade 없이) → 409 또는 명시적 거부 응답,
  - 잘못된 body → 400(`api.ErrBadRequest`).

## M6 — (선택) CSV 시더

### AC-16 — subway CSV 시드 (REQ-S1, 선택)
- **Given** `examples/subway/{station,spot,device}.csv`(`#` 주석 헤더)
- **When** `SeedFromSubwayCSV(ctx, repo, dir)` 실행
- **Then** line(root) → station → spot → device 계층이 `source="manual"` 로 등록되고,
  예: station `st01`(신설동, line `ui-line`) 밑 spot `01`(상행계단#1) 이 full_code `…st01.01`(설계상
  구분자 조인) 형태로 조회 가능하다. **본 AC 는 M6 착수 시에만 검증**(미착수 시 스킵 허용).

## 품질 게이트 (Quality Gates)

- 신규 코드 커버리지 ≥ 85%(hybrid: 신규=TDD).
- `go test ./internal/storage/... ./internal/node/... ./internal/api/...` 통과, `-race` 그린(AC-11).
- `golangci-lint run` 클린, `gofmt -l` 무출력.
- `asset-encode`/`asset-decode` 가 `Registry.Types()` 및 `AllTypeMeta()` 에 노출.
- 기존 SPEC/코드 미회귀: 전체 `go test ./...` 통과(기존 테스트 불변).

## 검증 도구

- 단위/통합: `go test`(table-driven, testify 선택), race: `go test -race`.
- 정적: `golangci-lint`, `gofmt`, `goimports`.
- 수동 스모크(선택): xflowd 기동 후 `curl` 로 `POST/GET /assets`.
