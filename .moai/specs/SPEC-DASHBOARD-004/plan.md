# SPEC-DASHBOARD-004 구현 계획

## 1. 작업 분해

마일스톤은 백엔드 → 프론트엔드 순서이며, 각 마일스톤은 **독립적으로 `go build ./... && go test ./...` (또는 `npm run build && npm test`) 를 통과**해야 한다. M3(마이그레이션)은 롤백 불가 지점이므로 별도로 분리한다.

### M1 — 권한 카탈로그 확장 (Priority High)

| # | 작업 | 산출물 |
|---|------|--------|
| 1.1 | `resourceActions` 의 `dashboard` 행에 `create` · `delete` 추가 | `internal/rbac/catalog.go:65` |
| 1.2 | `resourceActions` 의 `nav` 행에 `dashboard` 추가 | 동일 |
| 1.3 | `nav.dashboard` 파생 제외 규칙 — viewer 에 부여하지 않음 | `internal/rbac/catalog.go` `init()` |
| 1.4 | 빌트인 역할 재시드 검증(admin=전량, editor=create+nav, viewer=미부여) | `internal/storage/sqlite.go:381` `seedBuiltinRoles` |
| 1.5 | `roles.dashboard_migrated` 컬럼 + 1회성 부여 마이그레이션 | `internal/storage/sqlite.go:286` `addColumnIfMissing`, `migrateRoleDashboardPermissions`(신규) |

M1 을 먼저 두는 이유: 이후 모든 마일스톤이 `dashboard.create` / `dashboard.delete` / `nav.dashboard` 키의 존재를 전제한다. 카탈로그에 없는 키를 라우트에 부착하면 [SPEC-AUTH-005](../SPEC-AUTH-005/spec.md) 의 카탈로그 검증이 거부한다.

기존 역할 이관 규칙:

| 기존 보유 | 부여 |
|-----------|------|
| `dashboard.read` + `dashboard.update` 둘 다 | `dashboard.create`, `nav.dashboard` |
| 그 외 | 없음 |

`dashboard.delete` 는 이관으로 부여하지 않는다. 삭제 권한을 조용히 늘리면 업그레이드가 사용자에게 보이지 않아야 한다는 `migrateRoleNavPermissions` 의 취지를 벗어난다.

**검증**: `go test ./internal/rbac/... ./internal/storage/...`. 카탈로그 스냅샷 테스트가 신규 키 3종을 반영하는지 확인.

### M2 — 신규 스키마와 저장소 계층 (Priority High)

| # | 작업 | 산출물 |
|---|------|--------|
| 2.1 | `schema_markers` 테이블 생성 | `internal/storage/sqlite.go` |
| 2.2 | 신규 `dashboards` / `dashboard_acl` / `dashboard_user_state` DDL | `internal/storage/sqlite.go` `migrateDashboardSchemaV2`(신규) |
| 2.3 | `DashboardRepository` 인터페이스 교체 (`List/Get/Create/Update/Delete` by `uid`) | `internal/storage/dashboard.go` |
| 2.4 | SQLite 구현 | `internal/storage/dashboard_sqlite.go` |
| 2.5 | ACL 저장소 (`ListByDashboard`, `ListBySubjects`, `Replace`, `DeleteByDashboard`) | `internal/storage/dashboard_acl_sqlite.go`(신규) |
| 2.6 | 사용자 UI 상태 저장소 (`Get`, `Put`) | `internal/storage/dashboard_state_sqlite.go`(신규) |
| 2.7 | 인가 판정 leaf 패키지 | `internal/dashboardacl/access.go`(신규) |

`internal/dashboardacl` 을 leaf 패키지로 두는 이유는 `internal/rbac` 와 동일하다 — `storage` 와 `api` 가 모두 참조하므로 순환 참조를 피해야 한다.

인가 판정 함수 시그니처(구현 형태는 자유, 판정 순서는 spec.md §2.2 고정):

```go
type Subject struct {
    Username    string
    Role        string
    Permissions map[string]struct{}
    AuthEnabled bool
}

type Access struct{ View, Edit, Delete, Grant bool }

func Evaluate(s Subject, d Dashboard, acl []ACLEntry) Access
```

기존 `migrateDashboardSchema`(`sqlite.go:88`)는 이 단계에서 **제거하지 않는다.** M3 가 구 스키마의 존재를 판정 근거로 쓰기 때문이다.

**검증**: `go test ./internal/storage/... ./internal/dashboardacl/...`. 판정 함수는 (visibility 3) × (소유자 여부 2) × (ACL 레벨 3) × (전역 권한 조합) 진리표 전수 테스트.

### M3 — 데이터 이관 (Priority High, 롤백 불가 지점)

**이 마일스톤만 단독으로 커밋하고 검증한다.** 이전·이후 마일스톤과 섞지 않는다.

절차(spec.md §2.4):

1. `sqlite_master` 에서 `dashboards` DDL 문자열을 조회해 `scope` 컬럼 보유 여부로 구 스키마를 판정한다. 구 스키마이면 `ALTER TABLE dashboards RENAME TO dashboard_snapshots_v1`.
2. 신규 테이블 4종을 `CREATE TABLE IF NOT EXISTS` 로 생성.
3. `schema_markers` 에 `dashboard_entity_migrated` 행이 있으면 즉시 반환(멱등).
4. 단일 트랜잭션에서 이관:
   - `scope='global'` 행 → `visibility='shared'`, `owner=<최초 admin>` 대시보드 N장
   - `scope='user'` 행 → `visibility='private'`, `owner=<해당 username>` 대시보드 M장
   - `uid` 충돌 시 개인 쪽에 `-<username>` 접미사, `active_dashboard_uid` 동시 보정
   - `activeDashboardId` · `deviceGridLayout` → `dashboard_user_state`
5. 마커 행 삽입 후 커밋.

**롤백 계획**: 트랜잭션 실패 시 신규 `dashboards` 는 비어 있고 `dashboard_snapshots_v1` 은 온전하다. 마커도 기록되지 않으므로 다음 기동에서 재시도된다. 구 테이블은 **어떤 경로에서도 DROP 하지 않는다.**

**검증 방법** (모두 자동화 테스트로 고정):

| 항목 | 방법 |
|------|------|
| 원본 보존 | 이관 전후 `SELECT COUNT(*) FROM dashboard_snapshots_v1` 동일 |
| 장수 일치 | 신규 `dashboards` 행 수 == 모든 스냅샷의 `dashboardPages` 길이 합 |
| 소유자 정합 | `scope='user'` 출처 행의 `owner` 가 원본 `owner` 와 일치 |
| 멱등성 | 이관 함수 3회 연속 호출 후 행 수 불변, 마커 1행 |
| 빈 상태 유지 | 이관 후 전 대시보드 삭제 → 재기동 → 재생성되지 않음 |
| 이관 전 백업 | 실행 전 `~/.xflow/xflow.db` 파일 복사본을 `xflow.db.pre-dashboard004` 로 남긴다 |

이관 전 파일 백업을 마이그레이션 코드가 직접 수행한다. WAL 모드에서 단순 파일 복사는 불완전할 수 있으므로 `VACUUM INTO` 를 사용한다.

### M4 — API 계층 (Priority High)

| # | 작업 | 산출물 |
|---|------|--------|
| 4.1 | 대시보드 CRUD 핸들러 + DTO | `internal/api/handler/dashboard.go`, `internal/api/dto/dashboard.go` |
| 4.2 | ACL 핸들러 | `internal/api/handler/dashboard_acl.go`(신규) |
| 4.3 | 사용자 UI 상태 핸들러 (`/dashboard-state`) | `internal/api/handler/dashboard.go` |
| 4.4 | 읽기 전용 호환 shim (`GET /dashboards/{shared,mine}`) | 동일 |
| 4.5 | `requireAdmin`(`dashboard.go:286`) 제거 → `dashboardacl.Evaluate` 로 대체 | 동일 |
| 4.6 | 라우트 등록 + 권한 부착, `permissionAllowlist` 에서 `/dashboards/mine` 쓰기 항목 제거 | `internal/api/handler/route_permission_coverage_test.go:29` |
| 4.7 | 잠금 방지 불변식 2종(`dashboard.delete` 보유자 0명 방지, 소유권 승계) | `internal/api/handler/user.go`, `internal/api/handler/role.go` |
| 4.8 | `cmd/xflowd/main.go` 라우터 등록 갱신 | `cmd/xflowd/main.go` |

라우트 등록 시 `shared` · `mine` · `state` 리터럴 세그먼트가 `/dashboards/{uid}` 보다 우선 매칭되어야 한다. [`internal/api/handler/user.go`](../../../internal/api/handler/user.go) 의 `/users/{username}/password` 선례와 동일한 등록 순서를 따른다.

`DashboardSnapshotToDTO`(`dashboard.go:411`)의 시그니처는 **변경하지 않는다.** shim 이 신규 모델에서 레거시 `storage.DashboardSnapshot` 을 합성해 이 함수에 넘기므로 원격 프록시 경로 3개 파일이 무변경으로 남는다.

**검증**: `go test ./internal/api/...`. 라우트 권한 커버리지 테스트(`route_permission_coverage_test.go`)가 신규 라우트 전부에 대해 통과해야 한다.

### M5 — 프론트엔드 서비스·동기화 재설계 (Priority High)

| # | 작업 | 산출물 |
|---|------|--------|
| 5.1 | 신규 타입 (`Dashboard`, `DashboardAclEntry`, `DashboardVisibility`, `DashboardUserState`) | `web/src/types/dashboard.ts` |
| 5.2 | API 클라이언트 교체 (`listDashboards`, `getDashboard`, `createDashboard`, `updateDashboard`, `patchDashboard`, `deleteDashboard`, `getAcl`, `putAcl`, `getState`, `putState`) | `web/src/services/api/dashboardService.ts` |
| 5.3 | `useDashboardSync` 재설계 — 부팅 시 목록 1회 조회, 활성 대시보드 단위 PUT | `web/src/hooks/useDashboardSync.ts` |
| 5.4 | `activeDashboardScope` · `sharedSnapshot` · `mineSnapshot` 제거, `dashboards: Dashboard[]` 단일 축 도입 | `web/src/stores/uiStore.ts:594` 외 |
| 5.5 | 403/404 폴백 — 목록 재조회 후 `is_default` → 첫 항목 순 전환 | `web/src/hooks/useDashboardSync.ts` |

기존 로직 중 **재사용**: 500ms debounce, fingerprint 기반 spurious PUT 방지, 단일 비행 + 큐잉, 409 재PUT(1회) 후 서버 상태 강제 적용. 이들은 대시보드 단위로 키를 바꾸기만 한다(`Record<Scope, T>` → `Record<uid, T>`).

기존 로직 중 **제거**: `Promise.all([getSharedDashboard, getMyDashboard])` 병렬 부팅, `readActiveScopeFromSession`, `setActiveDashboardScope`, 스코프 전환 시 legacy 필드 투영.

**검증**: `npm test -- useDashboardSync uiStore`. 기존 `useDashboardSync.test.ts` 의 스코프 관련 4개 케이스는 대시보드 단위 케이스로 대체한다.

### M6 — 대시보드 화면 게이팅 (Priority Medium)

| # | 작업 | 산출물 |
|---|------|--------|
| 6.1 | `sharedReadOnly` 제거 → `canEdit(activeDashboard)` 판정 | `web/src/pages/dashboard/DashboardPage.tsx:109` |
| 6.2 | 이름 변경·기본 설정·삭제 컨트롤을 대시보드 단위 판정으로 게이팅 | 동일 |
| 6.3 | 편집 불가 사유 툴팁 + 완화책 안내 문구 | 동일, i18n ko/en |
| 6.4 | 생성 다이얼로그를 서버 POST 로 전환 | `web/src/pages/dashboard/CreateDashboardDialog.tsx:56` |
| 6.5 | 헤더 대시보드 선택 드롭다운을 목록 API 기반으로 전환 | `web/src/components/layout/Header.tsx` |

**검증**: `npm test -- DashboardPage CreateDashboardDialog`.

### M7 — 관리 어포던스와 게이팅 (Priority Medium)

> **v0.2.0 개정.** 아래 표는 실제 착지 형태다. 최초 계획은 별도 관리 화면
> (`DashboardAdminPage.tsx`) + 사이드바 항목 + `/dashboards/admin` 라우트였으나,
> M7 재작업(커밋 `fc62a7bc`, 사용자 요청)으로 셋 다 걷어내고 관리 기능을
> **대시보드 편집(설정) 모드의 셀렉터** 안으로 옮겼다. 사유: 설정 모드에서
> 대시보드를 바꿀 수 없어(정적 타이틀 + 배지뿐) 관리와 편집이 두 화면으로
> 갈라져 있었고, 게이팅 판정 사본이 둘로 늘어났다.

| # | 작업 | 산출물 |
|---|------|--------|
| 7.1 | 설정 모드 대시보드 셀렉터 — 목록·생성·이름변경·기본지정·공개범위·삭제 인라인 컨트롤 | `web/src/pages/dashboard/DashboardSettingsSelector.tsx`(신규) |
| 7.2 | 권한 부여 패널(subject 선택 + view/edit 레벨), 셀렉터에서 다이얼로그로 재사용 | `web/src/pages/dashboard/DashboardAclPanel.tsx`(신규) |
| 7.3 | 관리 컨트롤을 `nav.dashboard` 로 게이팅(메뉴 축 — 렌더 여부) | `DashboardSettingsSelector.tsx` |
| 7.4 | `/dashboards/admin` 라우트 미등록 + 되살아남 방지 회귀 가드 | `web/src/router.tsx`, `web/src/router.dashboardAdmin.test.ts`(신규) |
| 7.5 | i18n ko/en 문구 추가 | `web/src/lib/i18n/` |

사이드바에는 **관리 항목을 두지 않는다.** 기존 대시보드(보기) 항목은 `permission`
미지정 상태를 유지한다(spec.md §2.5). 대시보드 셀렉터는 볼 수 있는 대시보드를
전부 싣는다 — 활성이 아닌 것도, 편집할 수 없는 것도 빼지 않는다.

두 축을 섞지 않는다: `nav.dashboard`(메뉴 축)가 없으면 관리 컨트롤을 아예
렌더하지 않고, 대시보드 단위 `can_edit`/`can_grant`/`can_delete`(컨트롤 축)는
숨기지 않고 비활성한다. `nav.dashboard` 가 없어도 대시보드 선택은 가능하다.

`nav.dashboard` 카탈로그 키는 제거하지 않는다 — 제거하면 이미 부여된
`role_permissions` 행이 카탈로그 밖 키가 되어 역할 편집 저장이 깨진다.

**검증**: `npm test -- Sidebar DashboardSettingsSelector router.dashboardAdmin`.

### M8 — 정리와 회귀 검증 (Priority Low)

| # | 작업 | 산출물 |
|---|------|--------|
| 8.1 | 사용하지 않게 된 uiStore legacy 필드·셀렉터 제거 | `web/src/stores/uiStore.ts` |
| 8.2 | `permissionAllowlist` 잔여 항목 정리 | `internal/api/handler/route_permission_coverage_test.go` |
| 8.3 | 원격 대시보드 조회 회귀 테스트 | `internal/api/handler/remote_query_test.go` |
| 8.4 | CHANGELOG 에 수용된 회귀(spec.md §2.6)와 완화책 기재 | `CHANGELOG.md` |

---

## 2. 기술 스택

- Go 1.x, 기존 `internal/api` 자체 라우터(외부 프레임워크 추가 없음)
- SQLite (`modernc.org/sqlite`) — 신규 의존성 없음
- React 19 + TypeScript + Zustand — 신규 의존성 없음
- 인가 판정은 `internal/rbac` 와 동일한 leaf 패키지 패턴을 따른다

**새 외부 의존성 없음.**

---

## 3. 위험 분석

| 위험 | 영향 | 완화 |
|------|------|------|
| M3 이관 중 실패로 대시보드 손실 | 전 사용자 대시보드 소실 | 단일 트랜잭션 + 구 테이블 보존 + `VACUUM INTO` 사전 백업 + 멱등 마커 |
| `uid` 충돌로 대시보드가 서로 덮어씀 | 데이터 혼선 | 개인 쪽 접미사 규칙 + `uid` UNIQUE 제약 + 이관 단위 테스트 |
| 원격 노드 프록시 형상 변경 | 원격 대시보드 조회 500/파싱 실패 | `DashboardSnapshotToDTO` 시그니처 고정 + shim + 회귀 테스트(AC-12) |
| viewer 회귀가 예고 없이 배포됨 | 사용자 불만, 원인 파악 지연 | spec.md §2.6 명시 + CHANGELOG + UI 안내 문구 + AC-16 |
| 라우트 매칭 충돌(`shared`/`mine`/`state` vs `{uid}`) | 관리 API 가 shim 으로 라우팅 | 리터럴 우선 등록 + 예약어 검증 + AC-12 |
| 인가 판정 누락으로 타인 대시보드 노출 | 정보 유출 | 판정 진리표 전수 테스트 + 핸들러 전 경로에서 동일 함수 호출 강제 |
| `activeDashboardScope` 제거 시 잔여 참조 | 빌드 실패 또는 런타임 undefined | 타입 제거 후 `tsc` 로 전수 검출 |
| ACL 대상(사용자·역할) 삭제 후 잔여 행 | 유령 권한 | `user:`/`role:` 검증은 저장 시점에만 수행하고, 판정 시 존재하지 않는 대상은 매치되지 않으므로 무해. 정리는 사용자·역할 삭제 경로에서 수행 |

---

## 4. 구현 순서

M1 → M2 → **M3** → M4 → M5 → M6 → M7 → M8

- **M3 를 M2 직후, M4 이전에 두는 이유**: 신규 저장소 계층이 없으면 이관 코드를 작성할 수 없고, API 계층을 먼저 만들면 통합 테스트가 비어 있는 신규 테이블에 막혀 원인 파악이 어려워진다.
- **M4(백엔드 완결)를 M5 이전에 두는 이유**: 프론트가 소비할 계약이 확정되어야 한다. M4 완료 시점에 API 만으로 대시보드 관리가 완결되어야 하며, 프론트가 없어도 운영 가능해야 한다([SPEC-AUTH-005](../SPEC-AUTH-005/spec.md) 가정 5 와 동일 원칙).
- **M6 과 M7 의 순서**: M6(기존 화면 게이팅)이 먼저다. M7 의 셀렉터는 신규 컴포넌트라 회귀 위험이 낮지만, M6 은 기존 화면을 건드리므로 먼저 안정화한다.

---

## 5. 검증 방법

각 마일스톤 완료 시:

```bash
go build ./... && go test ./internal/rbac/... ./internal/storage/... ./internal/dashboardacl/... ./internal/api/...
go vet ./...
```

프론트엔드 마일스톤(M5~M8):

```bash
cd web && npm run build && npm test
```

M3 완료 시 추가:

```bash
# 이관 멱등성 — 3회 연속 기동 후 행 수 불변
sqlite3 ~/.xflow/xflow.db "SELECT COUNT(*) FROM dashboards;"
sqlite3 ~/.xflow/xflow.db "SELECT COUNT(*) FROM dashboard_snapshots_v1;"
sqlite3 ~/.xflow/xflow.db "SELECT key, applied_at FROM schema_markers;"
```

전체 완료 시 acceptance.md 의 AC-01~AC-21 을 순서대로 검증한다.
