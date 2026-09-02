---
id: SPEC-DASHBOARD-004
title: 대시보드 관리 및 사용자별 권한 부여 (Dashboard Management & Per-Dashboard Access Control)
version: 0.2.0
status: draft
created: 2026-08-19
updated: 2026-08-23
author: xtra
priority: high
domain: dashboard
related_specs:
  - SPEC-DASHBOARD-001
  - SPEC-AUTH-005
  - SPEC-AUTH-006
  - SPEC-ASSET-001
lifecycle_level: spec-first
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 0.2.0 | 2026-08-23 | xtra | §2.5 게이팅 대상 개정 — M7 재작업(`fc62a7bc`)으로 '대시보드 관리'가 별도 화면·메뉴·라우트에서 대시보드 편집(설정) 모드 셀렉터로 이전됨. `nav.dashboard` 키와 판정 취지는 유지, 게이팅 대상만 변경. acceptance.md AC-10 동반 개정. |
| 0.1.0 | 2026-08-19 | xtra | 최초 작성 — 대시보드를 스냅샷 내부의 배열 원소에서 서버 1급 엔티티로 승격하고, 대시보드 단위 소유권·공개범위·ACL 을 도입한다. SPEC-DASHBOARD-001 의 `(scope, owner)` 2행 모델과 `/dashboards/{shared,mine}` 계약을 대체한다. 커밋 `fa142058` 이 예고한 "대시보드 관리 별도 메뉴 분리"의 후속이다. |

---

## 1. 개요 (Overview)

### 1.1 목적

대시보드를 **사용자별로 만들고 공유하고 회수할 수 있는 관리 대상**으로 만든다. 네 축으로 구성된다.

1. **엔티티 승격**: `dashboards` 테이블을 재설계해 대시보드 1장 = 1행으로 만든다. 서버가 `id` · `name` · `owner` · `visibility` · `version` · `updated_at` 을 관리하고, `payload` 는 그 대시보드 한 장의 패널·레이아웃만 담는다.
2. **권한 부여**: 신규 `dashboard_acl` 테이블에 `user:<username>` 또는 `role:<rolename>` 을 대상(subject)으로 `view` / `edit` 2단계 권한을 등재한다.
3. **인가 판정 일원화**: (사용자, 대시보드) 쌍에 대한 view / edit / delete / grant 가능 여부를 단일 함수가 결정한다. 소유권 · 공개범위 · ACL · 전역 RBAC 권한의 합성 순서를 못박는다.
4. **관리 화면**: 사이드바에 '대시보드 관리' 메뉴를 신설하고, 목록 · 생성 · 이름변경 · 삭제 · 권한부여를 한 화면에서 처리한다. 메뉴 노출은 신규 `nav.dashboard` 키로 통제한다.

### 1.2 배경

#### 1.2.1 대시보드는 서버 엔티티가 아니다

[SPEC-DASHBOARD-001](../SPEC-DASHBOARD-001/spec.md) 은 대시보드 **묶음 전체**를 하나의 불투명 JSON 스냅샷으로 영속화했다. [`internal/storage/sqlite.go:88`](../../../internal/storage/sqlite.go) `migrateDashboardSchema` 가 만드는 스키마는 다음과 같다.

```sql
CREATE TABLE IF NOT EXISTS dashboards (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  scope      TEXT    NOT NULL CHECK (scope IN ('global', 'user')),
  owner      TEXT,
  version    INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL,
  payload    TEXT    NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS dashboards_scope_owner_uidx
  ON dashboards(scope, COALESCE(owner, ''));
```

유니크 인덱스가 `scope=global` 1행 + 사용자당 1행을 강제한다. 즉 **서버가 아는 대시보드 행은 전체 사용자 수 + 1개**이며, 개별 대시보드는 행이 아니라 `payload` 안의 배열 원소다.

[`web/src/types/dashboard.ts:19`](../../../web/src/types/dashboard.ts) 의 `DashboardPayload` 가 그 배열을 담는다.

```ts
export interface DashboardPayload {
  dashboardPages: DashboardPageConfig[];
  activeDashboardId: string;
  dashboardGridCols: number;
  dashboardShowGridLines: boolean;
  dashboardRefreshInterval: number;
  deviceGridLayout: Record<string, DashboardLayoutItem>;
}
```

[`web/src/stores/uiStore.ts:231`](../../../web/src/stores/uiStore.ts) 의 `DashboardPageConfig { id, name, isDefault, panels, layout }` 가 개별 대시보드에 대응하지만, 이는 클라이언트 타입일 뿐이다. 대시보드 생성은 [`web/src/pages/dashboard/CreateDashboardDialog.tsx:56`](../../../web/src/pages/dashboard/CreateDashboardDialog.tsx) 의 `addDashboardPage(trimmed)` — 순수한 클라이언트 스토어 변형이며 서버는 그 사실을 알지 못한다.

결과적으로 **대시보드 1장을 특정 사용자에게만 공유하는 것이 구조적으로 불가능하다.** 공유 단위가 "전부(global)" 또는 "나만(user)" 두 가지뿐이기 때문이다.

#### 1.2.2 이미 존재하는 자산 (재사용 대상)

| 자산 | 위치 | 상태 |
|------|------|------|
| `DashboardRepository{Get,Put,Delete}` + `ErrDashboardNotFound` / `ErrDashboardVersionMismatch` | `internal/storage/dashboard.go` | 존재. `(scope, owner)` 키잉 |
| 서버 부여 단조 증가 `version` + `If-Match` 낙관적 동시성 | `internal/storage/dashboard.go`, `internal/api/handler/dashboard.go` | 존재. 그대로 승계 |
| 256KB 페이로드 상한 → 413 | `internal/api/handler/dashboard.go:34` | 존재 |
| 라우트 6종 등록 + 권한 부착 | `internal/api/handler/dashboard.go:72` | 존재 |
| 권한 카탈로그 단일 정의 | `internal/rbac/catalog.go:65` `resourceActions` | 존재. `dashboard` 는 `{read, update}` 2종뿐 |
| 메뉴 노출 전용 축 `nav.*` | `internal/rbac/catalog.go` `ResourceNav` | 존재. 대시보드에는 nav 키 없음 |
| 1회성 이관 플래그 패턴 | `internal/storage/sqlite.go:318` `migrateRoleNavPermissions` + `roles.nav_migrated` | 존재. 본 SPEC 이 선례로 삼는다 |
| 500ms debounce · 단일 비행(single-flight) · 409 재PUT · 403 처리 | `web/src/hooks/useDashboardSync.ts` | 존재. 대시보드 단위로 재구성 |
| 권한 판정 훅 | `web/src/hooks/usePermission.ts:44` `hasPermissionOf` | 존재 |
| 사용자·역할 관리 API 선례 | `internal/api/handler/user.go` | 존재. 라우트·DTO·잠금방지 패턴을 따른다 |

즉 **영속화·동시성·권한 미들웨어는 완성되어 있고 대시보드의 입자성(granularity)만 잘못되어 있다.** 본 SPEC 은 입자성을 고치는 작업이며 동시성 정책과 인증 흐름은 건드리지 않는다.

#### 1.2.3 커밋 `fa142058` 과의 관계

커밋 `fa142058` 은 대시보드 화면에서 공유/내 대시보드 스코프 탭 바를 제거하면서 "대시보드 관리는 별도 메뉴로 분리 예정" 이라고 남겼다. 그 결과 [`web/src/stores/uiStore.ts:594`](../../../web/src/stores/uiStore.ts) 의 `activeDashboardScope` 와 [`web/src/pages/dashboard/DashboardPage.tsx:109`](../../../web/src/pages/dashboard/DashboardPage.tsx) 의 `sharedReadOnly` 가 UI 진입점 없이 남아 기본설정·이름변경 컨트롤만 게이팅하고 있다. 본 SPEC 이 그 예고된 후속이다.

### 1.3 비범위 (Out of Scope)

- 대시보드 패널 타입 추가·변경 — 기존 `PanelConfig` 를 그대로 승계한다
- 대시보드 변경 이력·되돌리기(버전 히스토리) — `version` 은 충돌 감지용이며 스냅샷을 보관하지 않는다
- 링크 공유(비인증 읽기 전용 URL) — 모든 접근은 인증을 요구한다
- 대시보드 폴더·태그·검색 — 목록은 단일 평면 목록이다
- 실시간 협업 편집(동시 커서·CRDT) — 충돌 정책은 기존 `If-Match` → 409 를 유지한다
- 원격 노드(remote) 대시보드의 쓰기·권한 관리 — 원격 경로는 읽기 전용 프록시로 유지한다
- PostgreSQL 저장소 대응 — `dashboards` 테이블은 SQLite 경로에만 존재한다
- 대시보드 권한 변경의 감사 로그 테이블 — `granted_by` / `granted_at` 컬럼으로만 남긴다

---

## 2. EARS 요구사항

### 2.1 [U1] (Ubiquitous) 대시보드 1급 엔티티 데이터 모델

시스템은 대시보드 1장을 `dashboards` 테이블의 1행으로 영속한다.

```sql
CREATE TABLE IF NOT EXISTS dashboards (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  uid        TEXT    NOT NULL UNIQUE,
  name       TEXT    NOT NULL,
  owner      TEXT    NOT NULL,
  visibility TEXT    NOT NULL CHECK (visibility IN ('private', 'shared', 'acl')),
  is_default INTEGER NOT NULL DEFAULT 0,
  sort_order INTEGER NOT NULL DEFAULT 0,
  version    INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  payload    TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS dashboards_owner_idx      ON dashboards(owner);
CREATE INDEX IF NOT EXISTS dashboards_visibility_idx ON dashboards(visibility);
```

- `uid` 는 클라이언트에 노출되는 식별자이며 기존 `DashboardPageConfig.id` 값을 그대로 승계한다. `shared` · `mine` · `state` 는 라우트 리터럴과 충돌하므로 예약어이며 `uid` 로 발급되지 않는다.
- `owner` 는 JWT `Claims.Username` 으로 서버가 결정한다. 요청 본문의 `owner` 는 무시된다.
- `visibility` 는 3모드다.

| 값 | 의미 |
|----|------|
| `private` | 소유자만 접근 |
| `shared` | 인증된 모든 사용자가 조회(view) |
| `acl` | `dashboard_acl` 에 등재된 대상만 접근 |

- `payload` 는 **그 대시보드 한 장**의 JSON 이다: `{ panels, layout, gridCols, showGridLines, refreshInterval }`. 기존 스냅샷의 `dashboardPages` 배열은 사라진다.
- 기존 `(scope, COALESCE(owner,''))` 유니크 인덱스는 제거된다. 한 사용자가 대시보드를 여러 장 소유할 수 있어야 하기 때문이다.

권한 대상(subject)은 `dashboard_acl` 에 영속한다.

```sql
CREATE TABLE IF NOT EXISTS dashboard_acl (
  dashboard_id INTEGER NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
  subject      TEXT    NOT NULL,
  level        TEXT    NOT NULL CHECK (level IN ('view', 'edit')),
  granted_by   TEXT    NOT NULL,
  granted_at   INTEGER NOT NULL,
  PRIMARY KEY (dashboard_id, subject)
);
CREATE INDEX IF NOT EXISTS dashboard_acl_subject_idx ON dashboard_acl(subject);
```

`subject` 형식은 `user:<username>` 또는 `role:<rolename>` 이다. 두 접두사 외의 값은 저장될 수 없다. 존재하지 않는 사용자·역할을 지정하면 400 으로 거부한다.

기존 스냅샷에 있었으나 개별 대시보드에 속하지 않는 두 값(`activeDashboardId`, `deviceGridLayout`)은 사용자 UI 상태로 분리한다.

```sql
CREATE TABLE IF NOT EXISTS dashboard_user_state (
  username             TEXT    PRIMARY KEY,
  active_dashboard_uid TEXT    NOT NULL DEFAULT '',
  device_grid_layout   TEXT    NOT NULL DEFAULT '{}',
  version              INTEGER NOT NULL DEFAULT 0,
  updated_at           INTEGER NOT NULL
);
```

### 2.2 [U2] (Ubiquitous) 인가 판정 규칙

시스템은 (요청자 `u`, 대시보드 `d`) 쌍에 대해 `view` / `edit` / `delete` / `grant` 가능 여부를 **단일 함수**로 판정한다. 판정은 다음 순서로 합성되며 먼저 확정된 단계에서 종료한다.

| 단계 | 조건 | 결과 |
|------|------|------|
| 0 | `basic_auth.enabled == false` | 4종 모두 허용, 종료 |
| 1 | `u` 가 카탈로그의 `dashboard.{read,create,update,delete}` 4종을 **모두** 보유 (= 대시보드 관리자) | 4종 모두 허용, 종료 |
| 2 | `d.owner == u.username` | 기본 4종 허용. 단 3단계의 전역 상한을 적용 |
| 3 | 그 외 | `base` 를 산출: `visibility=shared` → `view`; `visibility=acl` → `subject` 가 `user:<u>` 또는 `role:<u.role>` 인 ACL 행 중 **최대 레벨**; `visibility=private` → `none` |

3단계 이후 최종 판정:

| 판정 | 성립 조건 |
|------|-----------|
| `view` | `base >= view` **AND** `u.has("dashboard.read")` |
| `edit` | `base == edit` **AND** `u.has("dashboard.update")` |
| `delete` | (소유자 또는 1단계 관리자) **AND** `u.has("dashboard.delete")` |
| `grant` | (소유자 또는 1단계 관리자) **AND** `u.has("dashboard.update")` |

**전역 RBAC 권한은 상한(ceiling)이다.** 소유자라도 `dashboard.update` 가 없으면 자기 대시보드를 편집할 수 없다. ACL 은 상한을 올리지 못하며 상한 이내에서만 접근을 부여한다. 이 규칙이 §2.6 의 수용된 회귀를 만든다.

`user:` 와 `role:` 이 동시에 매치되면 **높은 레벨이 이긴다**(`edit` > `view`). ACL 은 거부(deny) 항목을 지원하지 않으므로 부여만으로 단조 증가한다.

### 2.3 [U3] (Ubiquitous) API 계약

시스템은 다음 라우트를 제공한다. `권한` 열은 미들웨어가 강제하는 전역 RBAC 키이며, `인가` 열은 §2.2 판정이다.

| 메서드 | 경로 | 권한 | 인가 | 성공 | 실패 |
|--------|------|------|------|------|------|
| GET | `/api/v1/dashboards` | `dashboard.read` | view 가능 항목만 반환 | 200 | — |
| POST | `/api/v1/dashboards` | `dashboard.create` | — | 201 | 400, 403, 413 |
| GET | `/api/v1/dashboards/{uid}` | `dashboard.read` | view | 200 | 403, 404 |
| PUT | `/api/v1/dashboards/{uid}` | `dashboard.update` | edit | 200 | 403, 404, 409, 413 |
| PATCH | `/api/v1/dashboards/{uid}` | `dashboard.update` | edit(이름) / grant(visibility·기본) | 200 | 400, 403, 404, 409 |
| DELETE | `/api/v1/dashboards/{uid}` | `dashboard.delete` | delete | 204 | 403, 404, 409 |
| GET | `/api/v1/dashboards/{uid}/acl` | `dashboard.read` | grant | 200 | 403, 404 |
| PUT | `/api/v1/dashboards/{uid}/acl` | `dashboard.update` | grant | 200 | 400, 403, 404 |
| GET | `/api/v1/dashboard-state` | 인증만 | 본인 | 200 | — |
| PUT | `/api/v1/dashboard-state` | 인증만 | 본인 | 200 | 400, 409 |

- `GET /dashboards` 목록 응답은 **`payload` 를 포함하지 않는다**. 대신 항목마다 `can_edit` · `can_delete` · `can_grant` 판정 결과를 실어 프론트가 §2.2 를 재구현하지 않게 한다.
- `PUT /dashboards/{uid}` 는 `If-Match: <version>` 을 지원하며 불일치 시 409 를 반환하고 서버 상태를 변경하지 않는다(기존 정책 승계).
- `PUT /dashboards/{uid}/acl` 은 ACL 전체를 치환한다. 부분 갱신은 제공하지 않는다 — 부분 갱신은 "지웠는지 안 지웠는지" 모호성을 만든다.
- `dashboard-state` 는 `/dashboards/{uid}` 경로 매칭과 충돌하지 않도록 별도 최상위 경로로 둔다.
- 페이로드 상한 256KB 와 413 매핑은 그대로 유지한다. 단위가 묶음에서 1장으로 줄었으므로 실효 상한은 완화된다.

**기존 `/dashboards/shared` · `/dashboards/mine` 의 처리 방침: GET 만 남기는 읽기 전용 호환 shim, PUT·DELETE 제거.**

근거는 원격 노드 프록시다. [`internal/api/handler/remote_query.go:133`](../../../internal/api/handler/remote_query.go) 이 `GET /remote/nodes/{instance_id}/dashboards/{shared,mine}` 을 등록하고, [`cmd/xflowd/remote_query.go:189`](../../../cmd/xflowd/remote_query.go) 이 `handler.DashboardSnapshotToDTO` 로 로컬 GET 과 **동일한 DTO 형상**을 직렬화하며, [`web/src/services/api/remoteService.ts:834`](../../../web/src/services/api/remoteService.ts) `getRemoteDashboard` 가 그것을 소비한다. 이 경로는 `remote_query.go` 주석이 명시하듯 READ-ONLY 다.

따라서 GET shim 을 유지하면 원격 경로 3개 파일을 건드리지 않고 본 SPEC 을 완료할 수 있다. shim 은 다음을 합성해 레거시 `DashboardSnapshot` 형상으로 반환한다.

| 경로 | 합성 내용 |
|------|-----------|
| `GET /dashboards/shared` | 요청자가 view 가능한 `visibility != 'private'` 대시보드를 `sort_order` 순으로 `dashboardPages` 배열에 담는다. `version` · `updatedAt` 은 포함된 행들의 최댓값 |
| `GET /dashboards/mine` | 요청자가 소유한 `visibility = 'private'` 대시보드를 동일 방식으로 담는다 |

쓰기(PUT/DELETE)를 제거하는 이유: 묶음 단위 쓰기는 새 모델에서 "어느 대시보드의 어느 version 에 대한 쓰기인가"를 결정할 수 없어 낙관적 동시성이 성립하지 않는다. 모호한 쓰기 경로를 남기는 것보다 제거가 안전하다. 제거된 라우트는 405 가 아니라 **404** 를 반환한다(라우트 미등록).

### 2.4 [U4] (Ubiquitous) 멱등 마이그레이션

시스템은 기존 데이터베이스를 파괴하지 않고 새 모델로 1회 이관한다. 절차는 재실행 안전(멱등)해야 한다.

이관 상태는 신규 마커 테이블에 기록한다.

```sql
CREATE TABLE IF NOT EXISTS schema_markers (
  key        TEXT PRIMARY KEY,
  value      TEXT    NOT NULL,
  applied_at INTEGER NOT NULL
);
```

`key = 'dashboard_entity_migrated'` 행의 **존재 여부**가 판정 기준이다. `roles.nav_migrated` 선례와 동일한 취지이되, 대시보드 이관은 역할별이 아니라 DB 전역 1회이므로 컬럼이 아닌 마커 행을 쓴다.

이 설계가 "이관 안 됨"과 "관리자가 의도적으로 비운 상태"를 구분한다.

| 상태 | 마커 행 | `dashboards` 행 수 | 이관 동작 |
|------|---------|--------------------|-----------|
| 이관 전 | 없음 | (구 스키마) | 이관 수행 |
| 이관 완료 | 있음 | N ≥ 0 | 건너뜀 |
| 이관 후 관리자가 전부 삭제 | 있음 | 0 | **건너뜀** (재생성하지 않음) |

행 수만으로 판정하면 마지막 표의 상태에서 삭제한 대시보드가 재부팅마다 되살아난다.

이관 절차:

1. `dashboards` 가 구 스키마(= `scope` 컬럼 보유)이면 `ALTER TABLE dashboards RENAME TO dashboard_snapshots_v1`.
2. 신규 `dashboards` · `dashboard_acl` · `dashboard_user_state` · `schema_markers` 를 `CREATE TABLE IF NOT EXISTS` 로 생성.
3. 마커가 없으면 단일 트랜잭션에서 이관:
   - `dashboard_snapshots_v1` 의 `scope='global'` 행의 `payload.dashboardPages` 각 원소 → `owner = <최초 admin 사용자>`, `visibility = 'shared'` 대시보드 1행.
   - `scope='user'` 행 각각의 `payload.dashboardPages` 각 원소 → `owner = <해당 username>`, `visibility = 'private'` 대시보드 1행.
   - 각 행의 `uid` 는 원본 `DashboardPageConfig.id` 를, `name` · `is_default` 는 원본 값을 승계한다. `sort_order` 는 배열 인덱스.
   - `payload` 의 `gridCols` · `showGridLines` · `refreshInterval` 은 출처 스냅샷의 값을 각 대시보드에 복사한다.
   - `activeDashboardId` · `deviceGridLayout` → `dashboard_user_state` 로 이관(`scope='global'` 출처는 버린다).
   - 마커 행 삽입.
4. `dashboard_snapshots_v1` 은 **삭제하지 않는다.** 이관이 잘못되었을 때의 유일한 복구 원본이다.

`uid` 중복(전역 스냅샷과 개인 스냅샷이 같은 `id` 를 쓰는 경우)은 개인 쪽에 `-<username>` 접미사를 붙여 해소하고, 해당 사용자의 `active_dashboard_uid` 도 함께 보정한다.

### 2.5 [U5] (Ubiquitous) 권한 카탈로그 확장

시스템은 [`internal/rbac/catalog.go:65`](../../../internal/rbac/catalog.go) `resourceActions` 표를 다음과 같이 확장한다.

| 리소스 | 기존 액션 | 신규 액션 |
|--------|-----------|-----------|
| `dashboard` | read, update | **create, delete** |
| `nav` | flow, agent, device, monitoring, schedule, node, remote, system, user, role | **dashboard** |

빌트인 역할 부여:

| 역할 | `dashboard.create` | `dashboard.delete` | `nav.dashboard` |
|------|--------------------|--------------------|-----------------|
| `admin` | O | O | O |
| `editor` | O | X | O |
| `viewer` | X | X | X |

- `dashboard.create` · `dashboard.delete` 는 `editorWritableResources` / `editorWritableActions` 파생 규칙을 그대로 따르면 editor 가 create 를 얻고 delete 는 얻지 않는다 — 추가 예외 없이 성립한다.
- `nav.dashboard` 는 파생 규칙(`nav.<menu>` 는 대응 데이터 read 보유 역할에 부여)을 그대로 적용하면 viewer 도 얻는다. viewer 는 생성·편집·삭제를 전혀 할 수 없으므로 빈 관리 화면만 보게 된다. 따라서 `nav.dashboard` 는 파생에서 제외하고 admin · editor 에만 명시 부여한다.

**`nav.dashboard` 는 대시보드 관리 어포던스에만 건다.** 대시보드를 *보는* 것 자체는 지금처럼 인증만으로 가능해야 한다. [`web/src/components/layout/Sidebar.tsx`](../../../web/src/components/layout/Sidebar.tsx) 의 대시보드 항목은 `permission` 을 지정하지 않는 현재 상태를 유지한다. 이 구분을 어기면 권한 0개 사용자가 빈 사이드바를 보게 되어 `catalog.go` 가 명시한 원칙을 위반한다.

> **v0.2.0 개정 — 게이팅 대상 변경.** 최초 작성 시점에는 `nav.dashboard` 가 사이드바의 신규 '대시보드 관리' **메뉴 항목**을 게이팅할 예정이었다. M7 재작업(커밋 `fc62a7bc`, 사용자 요청)으로 관리 화면을 걷어내고 관리 기능을 **대시보드 편집(설정) 모드의 셀렉터** 안으로 옮겼으므로, `nav.dashboard` 는 이제 [`web/src/pages/dashboard/DashboardSettingsSelector.tsx`](../../../web/src/pages/dashboard/DashboardSettingsSelector.tsx) 의 관리 컨트롤 렌더 여부를 게이팅한다. 사이드바에는 관리 항목이 존재하지 않고 `/dashboards/admin` 라우트도 등록하지 않는다.
>
> 카탈로그 키 `nav.dashboard` 자체는 **제거하지 않는다** — 제거하면 이미 부여된 `role_permissions` 행이 카탈로그 밖 키가 되어 역할 편집 저장이 깨진다.
>
> 판정은 두 축으로 분리한다. **메뉴 축**(`nav.dashboard`)은 관리 컨트롤을 *렌더할지*를, **컨트롤 축**(대시보드 단위 `can_edit` · `can_grant` · `can_delete`)은 렌더된 컨트롤을 *활성할지*를 결정한다. 컨트롤 축은 숨기지 않고 비활성하며, `nav.dashboard` 가 없어도 대시보드 선택 자체는 가능하다.

기존 역할은 `roles.nav_migrated = 1` 로 이미 표시되어 있어 [`internal/storage/sqlite.go:318`](../../../internal/storage/sqlite.go) `migrateRoleNavPermissions` 가 재실행되지 않는다. 따라서 `roles.dashboard_migrated` 컬럼을 [`internal/storage/sqlite.go:286`](../../../internal/storage/sqlite.go) `addColumnIfMissing` 으로 추가하고, `dashboard.read` 와 `dashboard.update` 를 **모두** 보유한 역할에만 `dashboard.create` 와 `nav.dashboard` 를 1회 부여한 뒤 `dashboard_migrated = 1` 로 표시한다. `dashboard.delete` 는 이관으로 부여하지 않는다 — 삭제 권한을 조용히 늘리는 것은 업그레이드가 사용자에게 보이지 않아야 한다는 선례의 취지를 벗어난다.

### 2.6 [U6] (Ubiquitous) 수용된 회귀 — viewer 의 개인 대시보드가 읽기 전용이 된다

**이것은 의도적으로 수용된 회귀다.**

현재 [`internal/api/handler/dashboard.go:72`](../../../internal/api/handler/dashboard.go) 는 `/dashboards/mine` 에 권한을 부착하지 않는다. 주석이 이유를 밝힌다 — `dashboard.update` 를 요구하면 viewer 가 자기 레이아웃조차 저장하지 못하기 때문이다. [`internal/api/handler/route_permission_coverage_test.go:29`](../../../internal/api/handler/route_permission_coverage_test.go) `permissionAllowlist` 에도 사유와 함께 등재되어 있다.

본 SPEC 이후:

| 시점 | viewer 의 개인 대시보드 |
|------|-------------------------|
| 현재 | 생성·편집·저장 가능(`/dashboards/mine` 무권한) |
| 이후 | **읽기 전용** — 마이그레이션된 대시보드는 보이지만 편집·생성 불가 |

원인은 두 가지다.

1. 모든 생성이 `dashboard.create` 를 요구하며 viewer 는 이를 보유하지 않는다.
2. §2.2 의 전역 상한 규칙에 따라 소유자라도 `dashboard.update` 없이는 편집할 수 없다.

**데이터는 삭제하지 않는다.** viewer 가 소유하던 개인 대시보드는 마이그레이션되어 그대로 남고 조회 가능하다.

**완화책**: 관리자가 역할 관리 화면([SPEC-AUTH-006](../SPEC-AUTH-006/spec.md) `/admin/roles`)에서 `viewer` 역할에 `dashboard.create` 와 `dashboard.update` 를 부여하면 즉시 해제된다. 권한은 요청 시점에 조회되므로([SPEC-AUTH-005](../SPEC-AUTH-005/spec.md) §4.3) 토큰 재발급도 재로그인도 필요하지 않다. UI 는 편집이 막힌 대시보드에서 이 완화책을 안내한다.

이 회귀를 수용하는 이유: 권한 모델에 예외 구멍을 남기면 "대시보드만 예외적으로 무권한"이라는 사실이 문서 밖으로 전파되지 않아, 이후 ACL 을 도입할 때 우회 경로가 된다. 예외 없이 한 축으로 통일하고 완화책을 문서화하는 편이 안전하다.

### 2.7 [E1] (Event-driven) 대시보드 생성

**When** 사용자가 관리 화면에서 대시보드 생성을 요청하면, **the system shall** `dashboard.create` 보유 여부를 검사하고, 보유하지 않으면 `403 Forbidden` 으로 거부하며, 보유하면 `owner = JWT username` · `visibility = 'private'` · `version = 1` 로 행을 생성하고 `201 Created` 와 함께 생성된 대시보드 메타를 반환한다.

- 요청 본문의 `owner` · `visibility` · `version` · `uid` 는 무시된다. `visibility` 변경은 생성 후 별도 PATCH 로만 가능하다.
- `name` 은 1~64자이며 공백만으로 구성될 수 없다. 동일 소유자 내 이름 중복은 허용한다(식별자는 `uid`).
- 생성 즉시 클라이언트는 목록을 재조회하지 않고 응답 본문으로 목록에 삽입한다.

### 2.8 [E2] (Event-driven) 대시보드 단위 저장

**When** 클라이언트가 활성 대시보드의 패널·레이아웃을 변경하면, **the system shall** 500ms debounce 후 `PUT /api/v1/dashboards/{uid}` 로 **그 대시보드 한 장만** 전송하고, 응답의 새 `version` 을 적용한다.

- 기존의 묶음 단위 PUT(`/dashboards/{shared,mine}`)은 사용하지 않는다. 대시보드 A 를 편집하는 동안 대시보드 B 의 내용이 함께 전송되어 타인의 변경을 덮어쓰던 문제가 사라진다.
- 단일 비행(single-flight) · 큐잉 · 409 재PUT · fingerprint 기반 spurious PUT 방지는 [`web/src/hooks/useDashboardSync.ts`](../../../web/src/hooks/useDashboardSync.ts) 의 기존 로직을 대시보드 단위로 재사용한다.
- 403(편집 권한 상실) 수신 시 재시도하지 않고 목록을 재조회하며, 해당 대시보드를 읽기 전용으로 전환하고 안내한다.

### 2.9 [E3] (Event-driven) 권한 부여

**When** 소유자 또는 대시보드 관리자가 ACL 저장을 요청하면, **the system shall** 각 `subject` 의 형식과 실재 여부를 검증하고, 하나라도 유효하지 않으면 `400` 으로 전체를 거부하며, 모두 유효하면 해당 대시보드의 ACL 을 요청 내용으로 전량 치환하고 `granted_by` 에 요청자 username 을, `granted_at` 에 서버 시각을 기록한다.

- `visibility` 가 `acl` 이 아닌 대시보드에도 ACL 을 저장할 수 있다. 저장은 되지만 §2.2 판정에서 무시되며, 이후 `visibility` 를 `acl` 로 바꾸면 즉시 유효해진다.
- ACL 변경은 캐시하지 않는다. 다음 요청부터 즉시 반영된다.

### 2.10 [S1] (State-Driven) 인증 비활성 상태

**While** `basic_auth.enabled == false` 인 동안, **the system shall** §2.2 의 모든 판정을 허용으로 처리하고 소유자를 빈 문자열 사용자로 취급한다.

[SPEC-AUTH-005](../SPEC-AUTH-005/spec.md) §2.5 와 동일한 이유다. 서버가 권한을 검사하지 않는 배포에서 대시보드만 잠그면 인증 도입 이전 배포가 사용 불가가 된다. 이 상태에서 생성되는 대시보드의 `owner` 는 `''` 이며, 이후 인증을 활성화하면 해당 대시보드는 `visibility` 에 따라서만 접근이 결정된다(소유자 특권 없음).

### 2.11 [S2] (State-Driven) 편집 권한 없는 대시보드 열람

**While** 사용자가 view 는 가능하나 edit 은 불가능한 대시보드를 보고 있는 동안, **the system shall** 패널 추가·삭제·레이아웃 편집·이름 변경·기본 설정 컨트롤을 비활성 상태로 표시하고 사유 툴팁을 제공한다.

[SPEC-AUTH-006](../SPEC-AUTH-006/spec.md) §4.2 의 원칙을 따른다 — 액션 컨트롤은 숨기지 않고 비활성한다. 이 판정이 [`web/src/pages/dashboard/DashboardPage.tsx:109`](../../../web/src/pages/dashboard/DashboardPage.tsx) 의 `sharedReadOnly` 를 대체한다.

### 2.12 [O1] (Optional) 선택 기능

- **가능하면** 목록 응답에 `can_edit` · `can_delete` · `can_grant` 를 포함해 프론트가 인가 규칙을 재구현하지 않게 한다.
- **가능하면** 소유권 이전(`PATCH` 의 `owner` 필드)을 제공한다. 대상 사용자는 `dashboard.create` 를 보유해야 한다.
- **가능하면** 관리 화면에서 대시보드 복제(payload 복사 + 새 `uid`)를 제공한다.
- **가능하면** ACL 편집 UI 에서 역할 대상(`role:`)을 선택할 때 그 역할에 속한 사용자 수를 함께 표시한다.

### 2.13 [UB1] (Unwanted-Behavior) 금지 동작

시스템은 다음을 허용하지 않는다.

| # | 금지 동작 | 대응 |
|---|-----------|------|
| 1 | 타 사용자의 `private` 대시보드 조회·수정 | 403 |
| 2 | `acl` 대시보드에 미등재된 사용자의 조회 | 403 |
| 3 | `view` 만 부여된 사용자의 저장(PUT) | 403, 서버 상태 불변 |
| 4 | 요청 본문의 `owner` · `visibility` · `version` · `uid` 로 접근 상승 시도 | 서버가 전부 무시. `visibility` 는 grant 권한자의 PATCH 로만 변경 |
| 5 | ACL 에 소유자 자신을 등재하거나 제거 | 400. 소유권이 항상 우선하므로 무의미하며, "나를 뺐다"는 오해를 만든다 |
| 6 | 시스템 전체에서 `dashboard.delete` 보유 사용자가 0명이 되는 역할 변경·사용자 삭제 | 409. 아무도 대시보드를 정리할 수 없는 상태를 만든다 |
| 7 | 소유자가 삭제되어 대시보드가 고아가 되는 상태 | 삭제 실행 관리자에게 소유권 승계. 승계 불가 시 409 |
| 8 | 마이그레이션 재실행으로 인한 대시보드 중복 생성 | 마커 행으로 차단(§2.4) |
| 9 | 256KB 를 초과하는 `payload` | 413, 저장하지 않음 |
| 10 | `If-Match` 불일치 상태의 저장(동시 편집 충돌) | 409, 서버 상태 불변 |
| 11 | 삭제되었거나 권한을 잃은 대시보드를 `active_dashboard_uid` 가 계속 참조 | 클라이언트가 목록의 `is_default` → 첫 항목 순으로 폴백하고 상태를 정정 저장. 서버는 존재하지 않는 `uid` 를 400 이 아니라 빈 문자열로 정규화 |

1번과 2번에 403 을 쓰는 이유: 404 를 쓰면 대시보드 존재 여부가 감춰지지만, 존재하는 대시보드에 대한 정상 404 와 구분되지 않아 클라이언트 폴백 로직(11번)이 두 경우를 혼동한다. 존재 노출을 감수하고 403 으로 통일한다.

### 2.14 [UB2] (Unwanted-Behavior) 프론트엔드 상태 불일치

1. `activeDashboardScope` 는 제거된다. 스코프 이중 동기화(`Promise.all([getShared, getMine])`)를 걷어내고 '접근 가능한 대시보드 목록' 단일 축으로 전환한다.
2. `sharedReadOnly` 를 쓰던 컨트롤은 §2.11 의 대시보드 단위 edit 판정으로 대체한다. 역할 이름(`isAdmin`) 비교는 남기지 않는다([SPEC-AUTH-006](../SPEC-AUTH-006/spec.md) §4.1).
3. 관리 화면에서 대시보드를 삭제하면 그 대시보드를 보고 있던 다른 탭은 다음 요청에서 404 를 받는다. 무한 재시도 없이 목록으로 복귀한다.
4. `permissions` 를 제공하지 않는 구버전 서버(하위 호환 폴백)에 접속하면 모든 판정이 허용되므로 관리 화면이 노출된다. 서버가 신규 라우트를 제공하지 않으면 404 를 받고 화면은 안내 문구를 표시한다.

---

## 3. 트레이서빌리티 표

| 요구사항 | 대상 파일 | 검증 |
|----------|-----------|------|
| U1 데이터 모델 | `internal/storage/sqlite.go:88`(`migrateDashboardSchema` 재작성), `internal/storage/dashboard.go`(인터페이스 교체), `internal/storage/dashboard_sqlite.go` | AC-01, AC-02 |
| U1 ACL 저장소 | `internal/storage/dashboard_acl_sqlite.go`(신규) | AC-05, AC-06 |
| U1 사용자 UI 상태 | `internal/storage/dashboard_state_sqlite.go`(신규) | AC-14 |
| U2 인가 판정 | `internal/dashboardacl/access.go`(신규, leaf 패키지) | AC-04, AC-05, AC-06, AC-07 |
| U3 API 계약 | `internal/api/handler/dashboard.go:72`(`RegisterRoutes` 재작성), `internal/api/dto/dashboard.go` | AC-03, AC-08, AC-11 |
| U3 호환 shim | `internal/api/handler/dashboard.go:411`(`DashboardSnapshotToDTO` 유지), `internal/api/handler/remote_query.go:133`, `cmd/xflowd/remote_query.go:189` | AC-12 |
| U4 마이그레이션 | `internal/storage/dashboard_migrate.go:125`(`migrateDashboardEntities` 신규), `internal/storage/sqlite.go:135`(`migrateDashboardSchemaV2` + `schema_markers`) | AC-01, AC-02 |
| U5 카탈로그 확장 | `internal/rbac/catalog.go:65`(`resourceActions`), `internal/storage/sqlite.go:286`(`addColumnIfMissing`), `internal/storage/sqlite.go:381`(`seedBuiltinRoles`) | AC-09, AC-10 |
| U5 라우트 권한 커버리지 | `internal/api/handler/route_permission_coverage_test.go:29`(`permissionAllowlist` 정리) | AC-13 |
| U6 수용된 회귀 | `internal/api/handler/dashboard.go`, `internal/rbac/catalog.go` | AC-03, AC-16 |
| E1 생성 | `internal/api/handler/dashboard.go`, `web/src/pages/dashboard/CreateDashboardDialog.tsx:56` | AC-03 |
| E2 대시보드 단위 저장 | `web/src/hooks/useDashboardSync.ts`, `web/src/services/api/dashboardService.ts` | AC-11, AC-15 |
| E3 권한 부여 | `internal/api/handler/dashboard_acl.go`(신규), `web/src/pages/dashboard/DashboardAclPanel.tsx`(신규), `web/src/pages/dashboard/DashboardSettingsSelector.tsx`(신규, 다이얼로그 호출부) | AC-05, AC-06, AC-17 |
| S1 인증 비활성 | `internal/dashboardacl/access.go`, `internal/api/handler/dashboard.go` | AC-18 |
| S2 편집 게이팅 | `web/src/pages/dashboard/DashboardPage.tsx:109`(`sharedReadOnly` 제거) | AC-15 |
| UB1 금지 동작 | `internal/api/handler/dashboard.go`, `internal/api/handler/user.go`, `internal/api/handler/role.go` | AC-04~AC-08, AC-19, AC-20 |
| UB2 상태 정합 | `web/src/stores/uiStore.ts:594`(`activeDashboardScope` 제거), `web/src/hooks/useDashboardSync.ts` | AC-15, AC-21 |
| nav.dashboard 게이팅 | `web/src/pages/dashboard/DashboardSettingsSelector.tsx:71`(`canSeeMenu('nav.dashboard')`), `web/src/components/layout/Sidebar.tsx`(관리 항목 미등록), `web/src/router.tsx`(`/dashboards/admin` 미등록), `web/src/hooks/usePermission.ts:44` | AC-10 |

---

## 4. 설계 결정

### 4.1 대시보드를 1급 엔티티로 승격한다 (payload 내 배열 유지 안 함)

`payload` 안의 배열 원소에 ACL 을 붙이는 방법도 있다. 그러나 그 경우:

- 인가 판정이 저장소 질의가 아니라 JSON 파싱을 요구한다. 목록 조회마다 전체 스냅샷을 역직렬화해야 한다.
- 낙관적 동시성이 묶음 단위로만 성립한다. A 를 편집하는 사용자와 B 를 편집하는 사용자가 서로 409 를 유발한다.
- ACL 을 참조 무결성 없이 JSON 안에 두게 되어, 삭제된 사용자의 권한 행이 영원히 남는다.

행 단위 승격은 세 문제를 동시에 해소한다. 대가는 마이그레이션이 롤백 불가라는 점이며, 원본 테이블 보존(§2.4 4단계)으로 완화한다.

### 4.2 "대시보드 관리자"를 역할 이름이 아니라 권한 조합으로 정의한다

§2.2 1단계는 `dashboard.{read,create,update,delete}` 4종을 모두 보유한 사용자를 관리자로 본다. 역할 이름 `admin` 을 비교하지 않는 이유는 [SPEC-AUTH-006](../SPEC-AUTH-006/spec.md) §4.1 이 명시한 대로 커스텀 역할이 생기는 순간 역할 이름 열거가 성립하지 않기 때문이다. [`internal/api/handler/dashboard.go:286`](../../../internal/api/handler/dashboard.go) 의 `requireAdmin`(= `ctx.UserRole() == "admin"`)은 본 SPEC 에서 제거된다.

대가: 관리자가 커스텀 역할에 `dashboard.*` 4종을 모두 부여하면 그 역할은 타인 대시보드를 삭제할 수 있게 된다. 이는 의도된 결과다 — 4종 전량 부여는 명시적 관리 위임이며, 별도 `dashboard.manage_all` 키를 신설하면 카탈로그에 대시보드만의 예외 축이 생긴다.

### 4.3 전역 RBAC 권한을 상한으로 둔다 (ACL 이 상한을 올리지 못한다)

ACL 이 전역 권한을 우회할 수 있게 하면 `dashboard.update` 가 없는 viewer 에게 `edit` 을 부여하는 것이 가능해지고, 그 순간 "역할이 권한의 원천"이라는 [SPEC-AUTH-005](../SPEC-AUTH-005/spec.md) 의 모델이 깨진다. 두 축이 서로를 우회하면 어느 쪽을 봐도 실제 권한을 알 수 없다.

대가가 §2.6 의 수용된 회귀다. 완화책이 역할 편집 한 번으로 끝나므로 감수한다.

### 4.4 `/dashboards/shared` · `/dashboards/mine` 을 읽기 전용 shim 으로 남긴다

제거·리다이렉트·shim 세 선택지를 검토했다.

| 선택지 | 결과 |
|--------|------|
| 완전 제거 | 원격 노드 프록시 3개 파일(`remote_query.go` 2개 + `remoteService.ts`)을 동시에 고쳐야 한다. 원격 노드는 버전이 섞여 배포되므로 신구 노드 혼재 시 원격 대시보드 조회가 깨진다 |
| 307 리다이렉트 | 새 경로는 배열을 반환하고 구 경로는 단일 스냅샷을 반환한다. 형상이 달라 리다이렉트로 호환되지 않는다 |
| **읽기 전용 shim** | 원격 경로가 무변경으로 동작한다. 쓰기 모호성은 라우트 미등록으로 제거된다 |

shim 은 신구 노드 혼재가 해소된 뒤 별도 SPEC 으로 제거한다.

### 4.5 ACL 전량 치환 (부분 갱신 없음)

`PUT /dashboards/{uid}/acl` 은 배열 전체를 받는다. `POST`/`DELETE` 로 항목을 개별 조작하면 클라이언트가 "화면에 보이는 목록"과 "서버 상태"의 차이를 계산해야 하고, 두 관리자가 동시에 편집할 때 중간 상태가 커밋된다. 전량 치환은 대시보드 본문과 동일하게 `If-Match` 를 적용할 수 있어 정책이 하나로 유지된다.

### 4.6 마커 행 vs `nav_migrated` 컬럼

`roles.nav_migrated` 는 역할별 1회 이관을 표현하므로 컬럼이 맞다. 대시보드 엔티티 이관은 DB 전역 1회이므로 `schema_markers` 행이 맞다. 두 패턴을 혼용하는 것이 아니라 이관 단위에 맞춰 선택한 것이다. `roles.dashboard_migrated`(§2.5)는 역할별이므로 컬럼 방식을 따른다.

---

## 5. 비기능 요구사항

| 항목 | 기준 |
|------|------|
| 목록 성능 | `GET /dashboards` 는 대시보드 수 N 에 대해 질의 2회(대시보드 + ACL) 이내. 인가 판정은 메모리에서 수행하며 행당 추가 질의 없음 |
| 저장 지연 | 대시보드 단위 PUT 은 기존 묶음 PUT 대비 전송량이 줄어든다. debounce 500ms 는 유지 |
| 하위 호환 | 인증 비활성 배포는 동작 변화 없음. 원격 노드 대시보드 조회는 무변경 동작 |
| 데이터 보존 | 마이그레이션은 기존 스냅샷 테이블을 보존한다. 이관 실패 시 트랜잭션 롤백으로 신규 테이블이 비어 있고 구 테이블이 온전하다 |
| 보안 | 요청 본문의 `owner` · `visibility` · `version` · `uid` 는 어떤 경로에서도 신뢰하지 않는다. 403 응답은 부족한 권한 키를 노출하지 않는다 |
| 테스트 | 신규 코드 커버리지 85% 이상(quality.yaml 기준). 인가 판정 함수는 진리표 전수 테스트 |
| 접근성 | 비활성 컨트롤은 `aria-disabled` 와 사유 툴팁을 제공 |
| 국제화 | 신규 문구는 ko/en 두 로케일 모두 추가 |

---

## 6. 가정 및 제약

1. 대상 저장소는 SQLite 이며 단일 xflowd 인스턴스가 `~/.xflow/xflow.db` 를 단독 점유한다([SPEC-DASHBOARD-001](../SPEC-DASHBOARD-001/spec.md) ASM-007 승계).
2. [SPEC-AUTH-005](../SPEC-AUTH-005/spec.md) 와 [SPEC-AUTH-006](../SPEC-AUTH-006/spec.md) 이 완료되어 `RequirePermission` 미들웨어와 `/auth/me` 의 `permissions` 배열이 동작한다.
3. 대시보드 총 개수는 수백 장 규모로 가정한다. 목록은 페이지네이션 없이 전량 반환한다.
4. ACL 항목 수는 대시보드당 수십 개 규모로 가정한다.
5. 사용자 삭제 시 소유권 승계 대상은 삭제를 실행한 관리자다. 다른 사용자로의 지정 승계는 범위 밖이다.
6. `uid` 는 기존 클라이언트 생성 문자열을 승계하므로 형식을 강제하지 않는다. 다만 예약어(`shared`, `mine`, `state`)와 슬래시를 포함할 수 없다.
7. 원격 노드는 본 SPEC 미적용 버전이 혼재할 수 있다. 원격 경로는 읽기 전용 shim 으로만 접근하며 권한 관리 대상이 아니다.
8. 마이그레이션은 서버 기동 시 자동 수행되며 사용자 확인 절차를 두지 않는다. 롤백 불가 지점이므로 plan.md 가 별도 마일스톤으로 분리해 검증 방법을 규정한다.
