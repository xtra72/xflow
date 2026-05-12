---
spec_id: SPEC-DASHBOARD-001
title: 대시보드 구성 서버 영속화 — Implementation Plan
version: 0.2.0
status: implemented
created: 2026-05-12
updated: 2026-05-12
---

# SPEC-DASHBOARD-001: Implementation Plan (v0.2.0)

> v0.2.0 핵심 변경: SQLite 채택 + 공유/개인 분리 + 사용자 SQLite 이관 + localStorage
> 마이그레이션 폐기. 자세한 결정 근거는 `spec.md` HISTORY 0.2.0 참조.

## 마일스톤 (우선순위 기반, 시간 추정 금지)

### Phase A — Foundation: SQLite 스키마 & 자격증명 이관 (Primary Goal)

- **M-1 (Priority High)**: `internal/storage/sqlite.go` 에 `dashboards`, `users` 테이블
  자동 생성 (`CREATE TABLE IF NOT EXISTS`) 추가. UNIQUE 제약은 `dashboards_scope_owner_uidx`
  partial index 로 정의 (`COALESCE(owner, '')`).
- **M-2 (Priority High)**: `internal/storage/users_sqlite.go` 신규
  - `users` 테이블 CRUD 헬퍼 (`InsertUser`, `GetUserByUsername`, `UpdatePasswordHash`,
    `CountUsers`).
  - 또는 `credentials.go` 내부에 직접 구현. (선택은 구현자 재량)
- **M-3 (Priority High)**: `internal/auth/credentials.go` 백엔드 SQLite 교체
  - 외부 API 시그니처 유지: `Load`, `Save`, `Authenticate`, `ChangePassword`,
    `EnsureDefaultAdmin`, `GetUser`.
  - `EnsureDefaultAdmin` 동작 재정의:
    1. `users` 테이블이 비어 있고 yaml 존재 → yaml 파싱 후 `INSERT OR IGNORE`,
       이후 yaml 을 `users.yaml.migrated` 로 rename.
    2. 여전히 비어 있으면 admin/admin 생성 (기존 동작).
  - 모든 인증 흐름이 SQLite 만 참조하도록 보장 (UB-007).

### Phase B — Dashboard Storage Core (Primary Goal)

- **M-4 (Priority High)**: `internal/storage/dashboard.go` 인터페이스 정의 +
  에러 타입 (`ErrDashboardNotFound`, `ErrDashboardVersionMismatch`).
  - 시그니처: `Get(ctx, scope, owner)`, `Put(ctx, scope, owner, payload, expectedVersion)`,
    `Delete(ctx, scope, owner)`.
- **M-5 (Priority High)**: `internal/storage/dashboard_sqlite.go` SQLite 1차 구현
  - 트랜잭션 내 SELECT → UPDATE 또는 INSERT 패턴.
  - 서버측 version 부여: `new_version = COALESCE(old.version, 0) + 1`.
  - `updated_at` 은 `time.Now().UnixMilli()`.
  - If-Match 검증: `expectedVersion >= 0 && old.version != expectedVersion` 시
    `ErrDashboardVersionMismatch` 반환 (트랜잭션 롤백).

### Phase C — API Handler & Authorization (Primary Goal)

- **M-6 (Priority High)**: `internal/api/dto/dashboard.go` DTO 정의 (`DashboardSnapshot`).
- **M-7 (Priority High)**: `internal/api/handler/dashboard.go` 핸들러
  - `RegisterRoutes(g *api.RouteGroup)` 으로 2벌 라우트 등록:
    - `GET /api/dashboards/shared`, `PUT /api/dashboards/shared`,
      `DELETE /api/dashboards/shared`
    - `GET /api/dashboards/mine`, `PUT /api/dashboards/mine`,
      `DELETE /api/dashboards/mine`
  - JWT 미들웨어로 401 처리 (기존 패턴 차용).
  - **권한 미들웨어**: `requireAdmin` (PUT/DELETE shared 에만 적용) — JWT
    `Claims.Role != "admin"` 이면 403.
  - **owner 결정**:
    - shared 경로 → `(scope="global", owner="")`
    - mine 경로 → `(scope="user", owner=Claims.Username)`
  - **owner spoofing 차단** (UB-003): 클라이언트가 PUT body 에 보낸 `scope`/`owner`/
    `version`/`updatedAt` 은 모두 무시되고, 서버가 URL/JWT/저장소 상태로 결정.
  - **URL vs body scope 검증** (UB-006): body 에 `scope` 가 있고 URL 의 scope 와
    불일치하면 400 Bad Request (silent normalize 금지).
  - **payload 크기 한도** (UR-003): 256 KB 초과 시 413 Payload Too Large.
  - 응답 코드: 200/204/400/401/403/404/409/413/500 모두 명시적 처리.

### Phase D — Boot Wiring & Auth Enforcement (Secondary Goal)

- **M-8 (Priority High)**: `cmd/xflowd/main.go` 부팅 흐름 수정
  - basic_auth 강제 활성화 (UR-004): `serverCfg.BasicAuth.Enabled=false` 면 부팅 거부
    또는 강제 `true` 설정 후 경고 로그. (구현자 재량 — 권장: 거부 후 명확한 에러 메시지)
  - `dashboardRepo := storage.NewDashboardSQLiteRepository(db)` 생성 (`db` 는 기존
    `flows`/`agents` 와 공유).
  - `dashboardHandler := handler.NewDashboardHandler(dashboardRepo, jwtSvc)` 생성.
  - `server.RegisterRoutes` 블록에 `dashboardHandler.RegisterRoutes(g)` 추가.
  - `CredentialsManager` 초기화 순서를 SQLite 가 열린 후로 이동 (이관 의존성).

### Phase E — Frontend (Secondary Goal)

- **M-9 (Priority High)**: `web/src/types/dashboard.ts` 타입 정의
  - `DashboardSnapshot`, `DashboardScope`, `DashboardPayload`.
- **M-10 (Priority High)**: `web/src/services/api/dashboardService.ts` REST 클라이언트
  - `getSharedDashboard()`, `putSharedDashboard(payload, ifMatch?)`,
    `deleteSharedDashboard(ifMatch?)`,
    `getMyDashboard()`, `putMyDashboard(payload, ifMatch?)`,
    `deleteMyDashboard(ifMatch?)`.
  - `If-Match` 헤더 지원 + 401/403/409 응답 분기.
- **M-11 (Priority High)**: `web/src/hooks/useDashboardSync.ts` 동기화 훅
  - 부팅 시: `Promise.all([getShared, getMine])` 병렬 호출.
  - 200 → 해당 scope 의 메모리 상태 초기화. 404 → 빌트인 `DEFAULT_DASHBOARD_PAGE` 로
    초기화 (첫 변경 시 자동 PUT 으로 snapshot 최초 생성).
  - Zustand subscribe + 500ms debounce → 활성 scope 에 맞춰 `PUT shared` 또는
    `PUT mine`.
  - 409 시 last-write-wins 재PUT (1회 한정). 두 번째 409 발생 시 토스트 + 강제 동기화.
  - 403 시 (shared PUT 에서) 토스트로 "공유 대시보드 편집 권한이 없습니다." 안내.
- **M-12 (Priority High)**: `web/src/stores/uiStore.ts` 변경
  - `partialize` 에서 대시보드 관련 키 6개 모두 제거 (`dashboardPages`,
    `activeDashboardId`, `dashboardGridCols`, `dashboardShowGridLines`,
    `dashboardRefreshInterval`, `deviceGridLayout`).
  - 부팅 시 1회 명시적 제거 로직: `xflow-ui:dashboard-migrated-v0.2` 플래그가 없으면
    위 키들을 `localStorage` 에서 제거하고 토스트 안내 후 플래그를 set.
  - 메모리 상태에 `sharedSnapshot`, `mineSnapshot` 두 슬롯과 활성 스코프
    (`activeDashboardScope: 'shared' | 'mine'`) 추가.
- **M-13 (Priority High)**: `web/src/pages/dashboard/DashboardPage.tsx` 탭 UI
  - 상단/사이드에 "공유" / "내 대시보드" 탭 토글. 사용자 역할이 admin 이 아니면 공유 탭
    의 편집 컨트롤(추가/삭제/이름 변경/패널 배치) 을 비활성화.
  - 탭 전환 시 활성 스코프 변경 → 컴포넌트가 해당 스냅샷의 `dashboardPages` 를 표시.

### Phase F — Verification & Documentation (Final Goal)

- **M-14 (Priority Medium)**: 테스트
  - `internal/storage/dashboard_sqlite_test.go` — Get/Put/Delete, version monotonic,
    If-Match conflict, partial unique index 동작 (global+NULL vs user+username).
  - `internal/api/handler/dashboard_test.go` — happy path 6 endpoints, 400 (scope
    mismatch), 401 (no JWT), 403 (editor → shared PUT), 404 (missing), 409 (version),
    413 (oversized), owner spoofing 차단, cross-user 차단.
  - `internal/auth/credentials_migration_test.go` — yaml 존재 시 1회 이관 + rename
    검증, yaml 없을 때 admin/admin 생성, 이관 후 yaml 미참조.
  - `web/src/hooks/useDashboardSync.test.ts` (Vitest) — boot 병렬 GET, 404 fallback,
    409 재PUT, 403 토스트, blank slate (localStorage 제거 + 토스트 1회).
- **M-15 (Priority Medium)**: 문서화 + CHANGELOG
  - API 문서에 `/api/dashboards/shared`, `/api/dashboards/mine` 6 endpoint 추가.
  - CHANGELOG v0.2.0 항목 작성 (사용자 안내 문구 포함).
  - README 의 인증 섹션에 "basic_auth 필수" 명시.

### Optional Goal (이번 SPEC 범위 외 — 별도 SPEC)

- **M-16** (Priority Low, 별도 SPEC `SPEC-USER-MGMT-001`): 사용자 등록/삭제/목록
  관리 API (OI-004).
- **M-17** (Priority Low, 별도 SPEC): WebSocket 실시간 broadcast (OR-002 / OI-002).
- **M-18** (Priority Low, 별도 SPEC): 페이지 단위 PATCH (OR-003).
- **M-19** (Priority Low, 별도 SPEC): 변경 이력 history (OR-001).
- **M-20** (Priority Low, 별도 SPEC): 개인 대시보드 read-only link share (OI-005).

---

## 기술 접근 (Technical Approach)

### 백엔드 (Go)

기존 패턴 준수:

- `internal/api/handler/flow.go` 의 핸들러 패턴 (`RegisterRoutes(g *api.RouteGroup)`).
- `internal/storage/repository.go` (`FlowRepository`) 의 인터페이스 추상화.
- `internal/storage/sqlite.go` 의 `CREATE TABLE IF NOT EXISTS` 자동 마이그레이션.
- `context.Context` 는 모든 저장소 메서드의 첫 파라미터.
- 에러는 `errors.Is(err, ErrDashboardNotFound)` 패턴.

**SQLite 트랜잭션 (`Put`)**:

```text
BEGIN
SELECT version FROM dashboards WHERE scope=? AND COALESCE(owner,'')=COALESCE(?, '')
  -- old.version 확정
  -- if expectedVersion >= 0 && old.version != expectedVersion: ROLLBACK, 반환 ErrDashboardVersionMismatch
INSERT INTO dashboards(scope, owner, version, updated_at, payload)
  VALUES(?, ?, old.version+1, now_ms, ?)
  ON CONFLICT(scope, COALESCE(owner,''))  -- partial unique index
  DO UPDATE SET
    version    = excluded.version,
    updated_at = excluded.updated_at,
    payload    = excluded.payload
COMMIT
```

(주: SQLite `ON CONFLICT` 가 partial unique index 와 직접 결합되지 않는 환경이면
`SELECT id` 후 UPDATE/INSERT 를 명시적으로 분기. 구현자 재량.)

### 프론트엔드 (TypeScript)

기존 패턴 준수:

- `web/src/services/api/` 의 다른 service 와 동일한 fetch wrapper / 인증 헤더.
- `web/src/hooks/` 의 다른 hook 과 동일한 Zustand 의존성.
- TanStack Query 의 `useQuery` / `useMutation` 활용.
- Zustand `subscribe` API + lodash-free 직접 debounce.

활성 스코프 결정 UX:

- 페이지 진입 직후 기본값은 사용자 역할에 따라:
  - admin → "공유" 탭 기본 선택.
  - editor/viewer → 본인 개인 대시보드가 비어 있으면 "공유" 기본 선택, 아니면 "내
    대시보드" 기본 선택.
- 사용자가 선택한 활성 스코프는 `sessionStorage` 에 저장하여 새로고침 시 유지 (서버
  영속 대상 아님 — UI 상태).

### 충돌 처리 (last-write-wins, 유지)

- 서버: `If-Match` 헤더 검사 + 409 응답 (body 에 server snapshot 포함).
- 클라이언트: 409 수신 시 server snapshot 적용 + 로컬 변경 재PUT 1회 한정. 두 번째
  409 발생 시 사용자에게 토스트 + 강제 동기화.

### 아키텍처 설계 방향

- **Backend layering**: Handler (HTTP + 권한) → Repository (영속화) → SQLite.
- **Frontend layering**: Component (탭 UI) → Hook (useDashboardSync) → Service
  (fetch) → REST API.
- **State source-of-truth**: 서버 > 메모리(Zustand) > localStorage(테마/사이드바만).
- **확장성**: `DashboardRepository` 가 `(scope, owner)` 를 받으므로 향후 팀/역할별
  스코프 추가 시 핸들러 레이어만 수정.

---

## 위험과 대응 (Risks & Mitigations)

| 위험                                                                | 영향                          | 대응                                                                                       |
| ------------------------------------------------------------------- | ----------------------------- | ------------------------------------------------------------------------------------------ |
| yaml → SQLite 이관 중 부분 실패                                     | 사용자 잠금 (로그인 불가)     | INSERT OR IGNORE + rename 은 1트랜잭션에 가깝게 처리. 실패 시 yaml 미rename → 다음 부팅 재시도 |
| `dashboards` partial unique index 호환성 문제                       | DDL 실패                      | `CREATE UNIQUE INDEX ... ON (scope, COALESCE(owner, ''))` 는 modernc/sqlite 에서 동작 확인 필요. 미동작 시 sentinel 빈 문자열로 NULL 대체 |
| 다중 사용자 동시 편집 시 변경 손실 (공유 또는 동일 사용자)          | 일부 변경 누락                | last-write-wins + If-Match. 추후 WebSocket broadcast (OI-002).                             |
| editor 가 공유 대시보드를 편집하려다 토스트 만으로는 UX 혼선        | 사용자 혼란                   | UI 측 사전 비활성화 + 토스트 메시지 + 도움말.                                              |
| basic_auth 강제로 인한 기존 비인증 운영 환경 깨짐                   | 기존 데모/개발 환경 부팅 실패 | 부팅 거부 대신 강제 활성화 + 경고 로그를 선택할 수 있음. 운영 문서에 명시.                 |
| 대용량 페이로드 (>256KB)                                            | 네트워크 지연/저장소 압박     | 413 명시적 반환 (현실적 한도 ASM-004).                                                     |
| owner spoofing 시도                                                 | 다른 사용자 데이터 변조       | 핸들러가 항상 JWT username 으로 owner 결정. body 의 owner 무시. 테스트로 보장.             |
| localStorage 제거 누락으로 기존 키가 잔류                           | 사용자 혼란 (옛 데이터 잔재)  | `xflow-ui:dashboard-migrated-v0.2` 플래그로 1회 보장. 테스트로 검증.                       |
| 사용자 안내(토스트) 미노출 시 갑작스러운 초기화로 인한 불만         | 사용자 경험 저하              | 토스트 + CHANGELOG + README 안내 3중 노출.                                                 |

---

## Acceptance 미리보기

상세 시나리오는 `acceptance.md` 참조. 핵심 게이트:

1. admin 이 공유 대시보드를 편집 → 다른 인증된 사용자가 GET 시 같은 결과 확인.
2. editor 가 `PUT /api/dashboards/shared` 시도 → 403 Forbidden.
3. 사용자 A 의 `PUT /api/dashboards/mine` 이 사용자 B 의 `GET /api/dashboards/mine`
   에 영향 없음 (cross-user 격리).
4. 기존 `users.yaml` 사용자가 SQLite 로 자동 이관된 후 정상 로그인. yaml 은
   `.migrated` 로 rename 되어 이후 부팅에서 참조되지 않음.
5. `basic_auth.enabled=false` 로 부팅 시 거부(또는 강제 활성화 + 경고).
6. v0.2 첫 부팅 시 기존 `localStorage.xflow-ui` 의 대시보드 키들이 1회 제거되고 토스트가
   1회 노출됨.
7. version 불일치 PUT → 409 + 서버 최신 snapshot. 클라이언트 1회 재PUT 으로 사용자
   변경이 살아남음.
8. TRUST 5 게이트: 백엔드 85%+ 커버리지, 프론트엔드 hook 단위 테스트, golangci-lint /
   biome 통과.
