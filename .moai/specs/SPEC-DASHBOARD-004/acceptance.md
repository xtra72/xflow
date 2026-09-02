# SPEC-DASHBOARD-004 인수 조건

모든 시나리오는 `basic_auth.enabled = true` 를 전제로 한다(AC-18 제외).

공통 픽스처:

| 사용자 | 역할 | 보유 dashboard 권한 |
|--------|------|---------------------|
| `root` | `admin` | read, create, update, delete |
| `edi` | `editor` | read, create, update |
| `vie` | `viewer` | read |
| `ops` | `operator`(커스텀) | read, update |

`operator` 역할은 `POST /api/v1/roles` 로 `{"name":"operator","permissions":["dashboard.read","dashboard.update"]}` 를 생성해 준비한다.

---

## AC-01 — 마이그레이션 멱등성

**Given** 구 스키마(`scope` 컬럼 보유)의 `dashboards` 테이블에 `scope='global'` 1행(`dashboardPages` 3장)과 `scope='user'` 2행(각 2장)이 저장된 데이터베이스가 있고
**When** 서버를 기동해 스키마 마이그레이션을 수행하면
**Then** 신규 `dashboards` 테이블에 7행(3 + 2 + 2)이 생성되고, `schema_markers` 에 `dashboard_entity_migrated` 1행이 기록되며, `dashboard_snapshots_v1` 에 원본 3행이 그대로 남는다.
**And** 서버를 2회 더 재기동해도 `dashboards` 행 수는 7로 불변이고 `schema_markers` 행 수는 1이다.

```bash
sqlite3 "$DB" "SELECT COUNT(*) FROM dashboards;"              # 7
sqlite3 "$DB" "SELECT COUNT(*) FROM dashboard_snapshots_v1;"  # 3
sqlite3 "$DB" "SELECT COUNT(*) FROM schema_markers WHERE key='dashboard_entity_migrated';"  # 1
```

Go 테스트: `TestMigrateDashboardEntities_Idempotent` (`internal/storage/`)

## AC-02 — 이관 후 관리자가 비운 상태 유지

**Given** AC-01 의 마이그레이션이 완료된 데이터베이스에서
**When** 관리자가 7장의 대시보드를 모두 삭제한 뒤 서버를 재기동하면
**Then** `dashboards` 행 수는 0으로 유지되고 대시보드가 재생성되지 않는다.

```bash
sqlite3 "$DB" "DELETE FROM dashboards;"
# 재기동 후
sqlite3 "$DB" "SELECT COUNT(*) FROM dashboards;"  # 0
```

**And** `dashboard_snapshots_v1` 은 여전히 3행이다(원본 보존).

Go 테스트: `TestMigrateDashboardEntities_EmptyAfterMigrationStaysEmpty`

## AC-03 — viewer 생성 차단

**Given** `vie` 가 로그인해 있고
**When** `POST /api/v1/dashboards` 로 `{"name":"내 대시보드"}` 를 보내면
**Then** `403 Forbidden` 이 반환되고 `dashboards` 행 수가 증가하지 않는다.

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST "$API/dashboards" \
  -H "Authorization: Bearer $VIE_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"내 대시보드"}'   # 403
```

**And** 동일 요청을 `edi` 토큰으로 보내면 `201` 이 반환되고 응답의 `owner` 는 `"edi"`, `visibility` 는 `"private"` 이다.
**And** 403 응답 본문에 부족한 권한 키(`dashboard.create`)가 노출되지 않는다.

## AC-04 — private 대시보드의 타인 접근 차단

**Given** `edi` 가 소유한 `visibility='private'` 대시보드 `uid=D1` 이 있고
**When** `ops` 가 `GET /api/v1/dashboards/D1` 을 호출하면
**Then** `403` 이 반환된다.
**And** `ops` 의 `GET /api/v1/dashboards` 목록 응답에 `D1` 이 포함되지 않는다.
**And** `ops` 가 `PUT /api/v1/dashboards/D1` 을 시도하면 `403` 이고 `D1` 의 `version` 이 변하지 않는다.

```bash
curl -s -o /dev/null -w '%{http_code}\n' "$API/dashboards/D1" -H "Authorization: Bearer $OPS_TOKEN"  # 403
curl -s "$API/dashboards" -H "Authorization: Bearer $OPS_TOKEN" | jq '[.[] | select(.uid=="D1")] | length'  # 0
```

## AC-05 — ACL 경유 조회 성공

**Given** `edi` 소유 대시보드 `D2` 의 `visibility` 가 `acl` 이고
**When** `edi` 가 `PUT /api/v1/dashboards/D2/acl` 로 `[{"subject":"user:ops","level":"view"}]` 를 저장한 뒤
**Then** `ops` 의 `GET /api/v1/dashboards/D2` 는 `200` 이고 응답에 `payload` 가 포함된다.
**And** 목록 응답의 해당 항목은 `can_edit=false`, `can_delete=false`, `can_grant=false` 이다.
**And** `ops` 의 `PUT /api/v1/dashboards/D2` 는 `403` 이다(view 만 부여).
**And** ACL 레벨을 `edit` 으로 바꾸면 `ops` 의 `PUT` 은 `200` 이 된다(`ops` 는 `dashboard.update` 보유).

```bash
sqlite3 "$DB" "SELECT subject, level, granted_by FROM dashboard_acl WHERE dashboard_id=(SELECT id FROM dashboards WHERE uid='D2');"
# user:ops|view|edi
```

## AC-06 — ACL 미등재 차단

**Given** AC-05 의 `D2` 에 `user:ops` 만 등재되어 있고
**When** `vie` 가 `GET /api/v1/dashboards/D2` 를 호출하면
**Then** `403` 이 반환된다.
**And** `edi` 가 ACL 을 `[]` 로 전량 치환하면 `ops` 의 `GET /api/v1/dashboards/D2` 도 `403` 이 된다.

## AC-07 — `role:` subject 경유 접근

**Given** `edi` 소유 대시보드 `D3` 의 `visibility` 가 `acl` 이고 ACL 이 `[{"subject":"role:operator","level":"edit"}]` 일 때
**When** `operator` 역할을 가진 `ops` 가 `GET /api/v1/dashboards/D3` 를 호출하면
**Then** `200` 이 반환되고 `can_edit=true` 이다.
**And** `operator` 역할이 아닌 `vie` 는 `403` 이다.
**And** 관리자가 `ops` 의 역할을 `viewer` 로 변경하면, **기존 토큰 그대로** `GET /api/v1/dashboards/D3` 가 `403` 이 된다(권한은 요청 시점 조회 — [SPEC-AUTH-005](../SPEC-AUTH-005/spec.md) §4.3).
**And** `user:ops` 와 `role:operator` 가 각각 `view` / `edit` 로 동시 등재되면 높은 레벨(`edit`)이 적용된다.

## AC-08 — 소유자 삭제 성공, 비소유자 삭제 차단

**Given** `edi` 소유 대시보드 `D4` 가 있고
**When** `edi` 가 `DELETE /api/v1/dashboards/D4` 를 호출하면
**Then** `403` 이 반환된다 — `editor` 는 `dashboard.delete` 를 보유하지 않으며 전역 권한이 상한이다(spec.md §4.3).

**Given** `root` 소유 대시보드 `D5` 가 있고
**When** `root` 가 `DELETE /api/v1/dashboards/D5` 를 호출하면
**Then** `204` 가 반환되고 `dashboards` 에서 행이 사라지며 `dashboard_acl` 의 관련 행도 CASCADE 로 함께 삭제된다.

**Given** `root` 소유 대시보드 `D6` 이 `visibility='shared'` 일 때
**When** `edi` 또는 `ops` 가 `DELETE /api/v1/dashboards/D6` 을 호출하면
**Then** `403` 이고 `D6` 은 남아 있다.

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X DELETE "$API/dashboards/D5" -H "Authorization: Bearer $ROOT_TOKEN"  # 204
sqlite3 "$DB" "SELECT COUNT(*) FROM dashboard_acl WHERE dashboard_id NOT IN (SELECT id FROM dashboards);"  # 0
```

## AC-09 — 카탈로그 확장과 빌트인 역할

**Given** 서버가 기동해 카탈로그를 노출할 때
**When** `GET /api/v1/permissions` 를 호출하면
**Then** 응답에 `dashboard.create`, `dashboard.delete`, `nav.dashboard` 가 포함된다.
**And** `GET /api/v1/roles` 응답에서 역할별 보유 권한이 아래와 같다.

| 역할 | `dashboard.create` | `dashboard.delete` | `nav.dashboard` |
|------|--------------------|--------------------|-----------------|
| `admin` | 포함 | 포함 | 포함 |
| `editor` | 포함 | 미포함 | 포함 |
| `viewer` | 미포함 | 미포함 | **미포함** |

```bash
sqlite3 "$DB" "SELECT r.name, p.permission FROM roles r JOIN role_permissions p ON p.role_id=r.id
  WHERE p.permission IN ('dashboard.create','dashboard.delete','nav.dashboard') ORDER BY r.name, p.permission;"
```

Go 테스트: `TestBuiltinRoles_DashboardPermissions` (`internal/rbac/`)

## AC-10 — nav.dashboard 없는 사용자에게 관리 어포던스 미노출

> **v0.2.0 개정.** M7 재작업(커밋 `fc62a7bc`, 사용자 요청)으로 '대시보드 관리'는
> 별도 화면·사이드바 항목·라우트가 아니라 **대시보드 편집(설정) 모드 안의 셀렉터**가
> 되었다. 게이트 키(`nav.dashboard`)와 "관리 능력이 있는 역할에만 노출한다"는 판정
> 취지는 그대로이고, 게이팅 **대상**만 사이드바 항목에서 설정 모드 어포던스로 옮겼다.
> 개정 전 문구는 삭제된 `DashboardAdminPage` 를 전제해 더는 검증할 수 없었다.

**Given** `vie` 가 로그인해 있고(`nav.dashboard` 미보유)
**When** 사이드바를 렌더하면
**Then** '대시보드 관리' 항목은 **애초에 존재하지 않는다** — 사이드바에 관리 항목 자체가 없다.
**And** **대시보드(보기) 항목은 노출된다** — 대시보드 보기는 인증만 요구하며, `nav.dashboard` 를 거부해도 영향을 받지 않는다.
**And** 권한이 0개인 사용자에게도 대시보드(보기) 항목은 남는다(빈 사이드바 금지 — spec.md §2.5).
**And** `/dashboards/admin` 라우트는 등록되어 있지 않고 `/dashboards` 하위 라우트가 0개이므로, 주소창 직접 진입 경로 자체가 존재하지 않는다(리다이렉트 가드 불필요).

**When** 설정(편집) 모드의 대시보드 셀렉터를 렌더하면
**Then** `nav.dashboard` 미보유 시 관리 컨트롤(이름변경·기본지정·공개범위·권한부여·삭제)을 **아예 렌더하지 않는다**(메뉴 축).
**And** `nav.dashboard` 미보유여도 대시보드 **선택** 자체는 계속 가능하다 — 보기는 다른 축이다.
**And** `edi`(`nav.dashboard` 보유)에게는 관리 컨트롤이 전부 렌더되며, 대시보드 단위 `can_edit` · `can_grant` · `can_delete` 가 없는 항목은 **숨기지 않고 비활성**한다(컨트롤 축).

두 축을 섞지 않는다: 메뉴 축(`nav.dashboard`)은 렌더 여부를, 컨트롤 축(대시보드 단위 인가)은 활성 여부를 결정한다.

Vitest:
- `Sidebar.test.tsx` — `it('대시보드 관리 항목은 사이드바에 존재하지 않는다')`, `it('권한이 0개인 사용자에게도 대시보드(보기) 항목은 남는다')`, `it('nav.dashboard 가 없어도 대시보드(보기) 항목은 남는다')`
- `router.dashboardAdmin.test.ts` — `it('/dashboards/admin 라우트가 존재하지 않는다')`, `it('/dashboards 하위 라우트를 아예 두지 않는다')`
- `DashboardSettingsSelector.test.tsx` — `it('nav.dashboard 가 없으면 관리 컨트롤을 아예 렌더하지 않는다')`, `it('nav.dashboard 가 없어도 대시보드 선택은 계속 가능하다 (보기는 다른 축)')`, `it('nav.dashboard 가 있으면 관리 컨트롤이 전부 렌더된다')`, `it('can_grant=false 는 기본 지정·공개범위·권한 부여만 잠근다')`

## AC-11 — 409 낙관적 동시성 유지

**Given** `root` 소유 대시보드 `D7` 의 현재 `version` 이 5 이고
**When** `PUT /api/v1/dashboards/D7` 을 `If-Match: 4` 로 호출하면
**Then** `409 Conflict` 가 반환되고 서버의 `version` 은 5로 불변이다.
**And** `If-Match: 5` 로 호출하면 `200` 이 반환되고 `version` 이 6이 된다.
**And** `If-Match` 헤더가 없으면 무조건 저장되고 `version` 이 증가한다(기존 정책 승계).

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X PUT "$API/dashboards/D7" \
  -H "Authorization: Bearer $ROOT_TOKEN" -H 'If-Match: 4' \
  -H 'Content-Type: application/json' -d '{"payload":{"panels":[],"layout":[]}}'   # 409
```

## AC-12 — 호환 shim 과 원격 프록시 회귀 없음

**Given** 마이그레이션이 완료된 서버에서
**When** `GET /api/v1/dashboards/shared` 를 호출하면
**Then** `200` 이 반환되고 응답 형상이 기존 `DashboardSnapshot`(`scope`, `owner`, `version`, `updatedAt`, `payload.dashboardPages[]`)과 동일하다.
**And** `GET /api/v1/dashboards/mine` 은 요청자가 소유한 `private` 대시보드만 담는다.
**And** `PUT /api/v1/dashboards/shared` 와 `DELETE /api/v1/dashboards/mine` 은 `404` 를 반환한다(라우트 미등록).
**And** `GET /api/v1/remote/nodes/{id}/dashboards/shared` 응답이 본 SPEC 이전과 동일한 필드 집합을 가진다.
**And** `uid` 가 `shared` / `mine` / `state` 인 대시보드를 생성하려 하면 `400` 으로 거부되고, `/dashboards/shared` 는 shim 으로 라우팅된다.

```bash
curl -s "$API/dashboards/shared" -H "Authorization: Bearer $ROOT_TOKEN" \
  | jq 'has("scope") and has("owner") and has("version") and has("updatedAt") and (.payload | has("dashboardPages"))'  # true
curl -s -o /dev/null -w '%{http_code}\n' -X PUT "$API/dashboards/shared" -H "Authorization: Bearer $ROOT_TOKEN" -d '{}'  # 404
```

Go 테스트: `TestDashboardSharedShim_LegacyShape`, `TestRemoteQueryDashboard_ShapeUnchanged`

## AC-13 — 라우트 권한 커버리지

**Given** 대시보드 핸들러가 등록하는 모든 라우트 목록이 있고
**When** 라우트 등록 결과를 검사하면
**Then** 각 라우트에 권한 미들웨어가 부착되어 있거나 `permissionAllowlist` 에 사유와 함께 등재되어 있다.
**And** `PUT /api/v1/dashboards/mine` 과 `DELETE /api/v1/dashboards/mine` 항목은 라우트가 사라졌으므로 allowlist 에서 제거되어 있다(`TestPermissionAllowlistHasNoStaleEntries` 통과).
**And** `GET /api/v1/dashboard-state` 와 `PUT /api/v1/dashboard-state` 는 사유("본인 소유 UI 상태 — 핸들러가 username 을 세션 사용자로 고정")와 함께 allowlist 에 등재되어 있다.

Go 테스트: `internal/api/handler/route_permission_coverage_test.go`

## AC-14 — 사용자 UI 상태 분리 보존

**Given** 마이그레이션 전 `scope='user'` 스냅샷에 `activeDashboardId="D1"` 과 `deviceGridLayout` 3개 항목이 있었고
**When** 마이그레이션이 완료되면
**Then** `dashboard_user_state` 에 해당 사용자 행이 생성되고 두 값이 보존된다.

```bash
sqlite3 "$DB" "SELECT username, active_dashboard_uid, json_array_length(json_keys(device_grid_layout)) FROM dashboard_user_state;"
```

**And** `GET /api/v1/dashboard-state` 가 그 값을 반환한다.
**And** `scope='global'` 스냅샷의 `activeDashboardId` / `deviceGridLayout` 은 이관되지 않는다.

## AC-15 — 프론트엔드 단일 축 전환

**Given** 웹 앱이 부팅될 때
**When** 네트워크 요청을 관찰하면
**Then** `GET /api/v1/dashboards` 가 1회 호출되고, `GET /api/v1/dashboards/shared` 와 `GET /api/v1/dashboards/mine` 은 호출되지 않는다.
**And** 활성 대시보드의 패널을 변경하면 500ms 후 `PUT /api/v1/dashboards/{활성 uid}` 만 1회 발생하며, 다른 대시보드의 `uid` 로는 PUT 이 발생하지 않는다.
**And** `useUIStore.getState()` 에 `activeDashboardScope` · `sharedSnapshot` · `mineSnapshot` 키가 존재하지 않는다.
**And** `grep -rn "activeDashboardScope\|sharedReadOnly" web/src` 가 무출력이다.

```bash
grep -rn "activeDashboardScope\|sharedReadOnly" web/src ; echo "exit=$?"   # exit=1 (무매치)
cd web && npx tsc --noEmit                                                  # 오류 0
```

Vitest: `useDashboardSync.test.ts` — `it('부팅 시 목록 API 를 1회만 호출한다')`, `it('활성 대시보드만 PUT 한다')`

## AC-16 — 수용된 회귀와 완화책

**Given** 마이그레이션 후 `vie`(viewer) 가 소유한 개인 대시보드 `D8` 이 있고
**When** `vie` 가 `GET /api/v1/dashboards/D8` 을 호출하면
**Then** `200` 이 반환되고 데이터가 보존되어 있다(삭제되지 않았다).
**And** `PUT /api/v1/dashboards/D8` 은 `403` 이다(`dashboard.update` 미보유).
**And** UI 는 편집 컨트롤을 비활성하고 "관리자에게 대시보드 편집 권한을 요청하세요" 취지의 사유를 표시한다.

**When** 관리자가 `PUT /api/v1/roles/viewer` 로 권한에 `dashboard.create` 와 `dashboard.update` 를 추가하면
**Then** `vie` 가 **기존 토큰 그대로** `PUT /api/v1/dashboards/D8` 을 호출해 `200` 을 받는다(재로그인 불필요).

## AC-17 — ACL 유효성 검증

**Given** `root` 소유 대시보드 `D9` 가 있고
**When** 아래를 각각 `PUT /api/v1/dashboards/D9/acl` 로 보내면
**Then** 표와 같이 응답하고 실패 시 ACL 이 전혀 변경되지 않는다.

| 본문 | 기대 |
|------|------|
| `[{"subject":"user:edi","level":"view"}]` | 200 |
| `[{"subject":"role:operator","level":"edit"}]` | 200 |
| `[{"subject":"edi","level":"view"}]` | 400 (접두사 없음) |
| `[{"subject":"group:dev","level":"view"}]` | 400 (미지원 접두사) |
| `[{"subject":"user:nobody","level":"view"}]` | 400 (존재하지 않는 사용자) |
| `[{"subject":"role:nosuchrole","level":"view"}]` | 400 (존재하지 않는 역할) |
| `[{"subject":"user:edi","level":"manage"}]` | 400 (미지원 레벨) |
| `[{"subject":"user:root","level":"view"}]` | 400 (소유자 자신 — spec.md §2.13 #5) |
| `[{"subject":"user:edi","level":"view"},{"subject":"user:edi","level":"edit"}]` | 400 (중복 subject) |

**And** `edi`(비소유자, `grant` 불가)가 동일 요청을 보내면 `403` 이다.

## AC-18 — 인증 비활성 회귀 없음

**Given** `basic_auth.enabled = false` 로 서버를 기동하고
**When** 토큰 없이 `GET /api/v1/dashboards`, `POST /api/v1/dashboards`, `DELETE /api/v1/dashboards/{uid}` 를 호출하면
**Then** 권한 검사로 인한 403 이 발생하지 않고 모두 정상 처리된다.
**And** 이때 생성된 대시보드의 `owner` 는 빈 문자열이다.
**And** 목록 응답의 모든 항목에서 `can_edit` · `can_delete` · `can_grant` 가 `true` 이다.

## AC-19 — 잠금 방지: dashboard.delete 보유자 0명 차단

**Given** `dashboard.delete` 를 보유한 사용자가 `root` 1명뿐일 때
**When** 아래를 시도하면
**Then** 모두 `409` 로 거부되고 상태가 변하지 않는다.

| 시도 | 기대 |
|------|------|
| `PUT /api/v1/users/root` 로 역할을 `editor` 로 강등 | 409 |
| `DELETE /api/v1/users/root` | 409 |
| `PUT /api/v1/roles/admin` 으로 `dashboard.delete` 제거 | 409 |

**And** `ops` 에게 `dashboard.delete` 를 부여한 뒤에는 동일 시도가 성공한다(더 이상 마지막 1명이 아니므로).

## AC-20 — 소유자 삭제 시 소유권 승계

**Given** `edi` 가 `visibility='shared'` 대시보드 `D10` 을 소유하고 있고
**When** `root` 가 `DELETE /api/v1/users/edi` 를 호출하면
**Then** `204` 가 반환되고 `D10` 의 `owner` 가 `root` 로 승계된다.

```bash
sqlite3 "$DB" "SELECT owner FROM dashboards WHERE uid='D10';"   # root
sqlite3 "$DB" "SELECT COUNT(*) FROM dashboards WHERE owner NOT IN (SELECT username FROM users);"  # 0
```

**And** `edi` 를 대상으로 하던 `dashboard_acl` 의 `user:edi` 행이 제거된다.

## AC-21 — 삭제된 대시보드를 참조하는 활성 상태

**Given** `root` 의 `active_dashboard_uid` 가 `D11` 이고
**When** `D11` 이 삭제된 뒤 `root` 가 앱을 새로고침하면
**Then** `GET /api/v1/dashboard-state` 응답의 `active_dashboard_uid` 는 빈 문자열로 정규화되어 반환된다(400 아님).
**And** 클라이언트는 목록에서 `is_default=true` 인 대시보드로, 없으면 첫 항목으로 전환하고 `PUT /api/v1/dashboard-state` 로 정정 저장한다.
**And** 접근 가능한 대시보드가 0장이면 빈 상태 안내를 표시하고 무한 재시도하지 않는다.
**And** 편집 중 대시보드가 타 세션에서 삭제되어 PUT 이 `404` 를 받으면, 목록을 1회 재조회하고 폴백하며 재시도하지 않는다.

Vitest: `useDashboardSync.test.ts` — `it('삭제된 활성 대시보드를 기본 대시보드로 폴백한다')`, `it('404 수신 후 재시도하지 않는다')`

---

## 품질 게이트

| 항목 | 기준 |
|------|------|
| 빌드 | `go build ./...` 성공, `cd web && npm run build` 성공 |
| 테스트 | `go test ./...` 전체 통과, `cd web && npm test` 전체 통과 |
| 정적 분석 | `go vet ./...` 무출력, `cd web && npx tsc --noEmit` 오류 0 |
| 커버리지 | 신규 패키지(`internal/dashboardacl`, `internal/storage/dashboard_*_sqlite.go`, `internal/api/handler/dashboard*.go`) 85% 이상 |
| 인가 진리표 | `dashboardacl.Evaluate` 는 (visibility 3) × (소유자 2) × (ACL 레벨 3) × (전역 권한 조합 4) 전수 테스트 |
| 회귀 | 기존 원격 쿼리 테스트(`internal/api/handler/remote_query_test.go`), 인증 테스트(`internal/api/handler/auth_test.go`), RBAC 테스트(`internal/rbac/`) 전부 통과 |
| 잔여 참조 | `grep -rn "activeDashboardScope\|sharedReadOnly" web/src` 무매치 |

## 엣지 케이스

| 상황 | 기대 동작 |
|------|-----------|
| `name` 이 공백만으로 구성 | 400, 생성되지 않음 |
| `name` 이 65자 이상 | 400 |
| 동일 소유자가 같은 이름의 대시보드를 2장 생성 | 허용(식별자는 `uid`) |
| `payload` 가 256KB 초과 | 413, 저장하지 않음, `version` 불변 |
| `payload` 가 유효하지 않은 JSON | 400 |
| 존재하지 않는 `uid` 조회 | 404 |
| `visibility` 를 `acl` 로 바꿨으나 ACL 이 비어 있음 | 소유자·관리자만 접근. 타 사용자 403 |
| `visibility` 가 `private` 인 대시보드에 ACL 저장 | 200(저장은 되나 판정에 미반영). `acl` 로 전환 시 즉시 유효 |
| ACL 대상 역할이 나중에 삭제됨 | 판정에서 매치되지 않아 접근 거부. 500 아님 |
| 이관 시 전역·개인 스냅샷의 `DashboardPageConfig.id` 충돌 | 개인 쪽에 `-<username>` 접미사, `active_dashboard_uid` 동시 보정 |
| 이관 시 `dashboardPages` 가 빈 배열 | 대시보드 0장 생성, 마커는 정상 기록 |
| 이관 시 admin 사용자가 존재하지 않음 | 전역 스냅샷의 소유자를 빈 문자열로 두고 `visibility='shared'` 유지 |
| `is_default` 가 2장 이상에 설정됨 | 마지막 것만 유지하고 나머지는 0으로 정규화 |
| 마지막 대시보드 삭제 | 허용. 빈 상태 안내 표시 |
| `If-Match` 에 숫자가 아닌 값 | 무조건 저장(-1, unconditional) — 기존 `parseIfMatch` 동작 승계 |
