---
id: SPEC-DASHBOARD-001
title: 대시보드 구성 서버 영속화 (Dashboard Configuration Server Persistence)
version: 0.2.0
status: implemented
created: 2026-05-12
updated: 2026-05-12
author: xtra
priority: high
domain: dashboard
related_specs:
  - SPEC-WEB-001
  - SPEC-WEB-005
  - SPEC-CHART-001
lifecycle: spec-anchored
---

# SPEC-DASHBOARD-001: 대시보드 구성 서버 영속화

## HISTORY

- **0.2.0** (2026-05-12): 0.1.0 결정 일부 무효화 및 다음 4개 핵심 변경 반영.
  1. **저장소 백엔드를 JSON 파일 → SQLite 로 확정** (OI-003 CLOSED). 기존
     `modernc.org/sqlite` (cgo-free) 인프라(`internal/storage/sqlite.go`) 와
     동일한 `~/.xflow/xflow.db` 파일을 공유하며 `dashboards` 테이블을
     `CREATE TABLE IF NOT EXISTS` 패턴으로 자동 생성한다. golang-migrate 는 사용하지
     않는다. JSON 파일 1차 구현(`internal/storage/dashboard_file.go`) 결정은 폐기.
  2. **사용자 분리/인증을 본 SPEC 에서 도입** (OI-001 CLOSED). 공유 + 개인
     **병행 모델** 채택. URL 분리(`/shared`, `/mine`) 와 역할(admin/editor/viewer) 기반
     인가. JWT `Claims.Username` (=`sub`) 으로 owner 자동 결정. **basic_auth 필수화**
     (ASM-003 변경).
  3. **자격증명 저장소를 yaml → SQLite `users` 테이블로 이관**. 부팅 시
     1회성 자동 마이그레이션(`users.yaml` → `users.yaml.migrated` rename) 후
     SQLite 가 source-of-truth. `internal/auth/credentials.go` 의
     `Load`/`Save`/`Authenticate`/`ChangePassword`/`EnsureDefaultAdmin` API 인터페이스는
     유지하고 내부 구현만 교체(백워드 호환). 사용자 등록/삭제/목록 관리 API 는 본 SPEC
     범위 외 — OI-004 로 분리.
  4. **localStorage 마이그레이션 폐기 (블랭크 슬레이트)**. 0.1.0 의 "1회성
     마이그레이션 (localStorage → 서버)" 로직 삭제. 부팅 시 GET 200/404 만 처리하고,
     기존 `localStorage.xflow-ui` 의 대시보드 관련 키들은 클라이언트가 명시적으로 제거
     한 뒤 빌트인 기본 대시보드로 초기화한다. 사용자에게는 토스트/CHANGELOG 로 안내.

- **0.1.0** (2026-05-12): 최초 초안. 대시보드 페이지/패널 구성을 브라우저 `localStorage` 에서
  서버측 영속 저장소로 이전하여 모든 브라우저/기기에서 동일한 대시보드 구성을 공유하도록 정의.
  단일/다중 사용자 분리는 미정 (Open Issue OI-001 참조) — 현 단계는 "단일 운영자 전역 공유"
  를 기본 가정으로 진행하되, 향후 사용자별 스코프 도입 시 마이그레이션 가능한 데이터 모델
  (`scope`, `owner`, `version`, `updatedAt`) 을 사전 도입한다. 최소 충돌 정책은
  `last-write-wins + If-Match (version)` 으로 정한다. **(v0.2.0 에서 일부 결정 무효화)**

| Version | Date       | Author | Change                                                                                    |
| ------- | ---------- | ------ | ----------------------------------------------------------------------------------------- |
| 0.1.0   | 2026-05-12 | xtra   | 최초 작성 — 서버 영속화 + 로컬 마이그레이션 + 충돌 정책 + 미래 확장                       |
| 0.2.0   | 2026-05-12 | xtra   | SQLite 채택 + 공유/개인 분리 + 사용자 SQLite 이관 + localStorage 마이그레이션 폐기        |

---

## 개요 (Overview)

### 목적

xflow 대시보드 페이지(`dashboardPages`)와 활성 페이지(`activeDashboardId`), 그리고 그리드
표시 옵션(`dashboardGridCols`, `dashboardShowGridLines`, `dashboardRefreshInterval`),
`deviceGridLayout` 을 **SQLite 영속 저장소**에 저장한다. 다음 두 가지 사용 컨텍스트를 병행 지원
한다.

- **공유 대시보드(`scope=global`, `owner=NULL`)**: 모든 인증된 사용자가 동일하게 보는 운영용
  대시보드. admin 만 편집 가능, editor/viewer 는 읽기 전용.
- **개인 대시보드(`scope=user`, `owner=<username>`)**: 각 사용자가 자유롭게 구성하는 개인용
  대시보드. 본인만 GET/PUT/DELETE 가능.

운영자가 어떤 브라우저/기기/시크릿창에서 접속하더라도 (공유) + (본인 개인) 두 snapshot 이
일관되게 보이도록 한다.

### 배경 (확인된 문제)

v0.1.0 에서 식별한 문제와 동일하다 — 대시보드 페이지 구성이 `web/src/stores/uiStore.ts:570`
의 Zustand `persist` 미들웨어로 브라우저 `localStorage` (key: `xflow-ui`) 에만 저장되어,
다른 브라우저에서 접속 시 대시보드가 비어 보이거나 다르게 보였다.

v0.2.0 에서는 다음 추가 결정으로 0.1.0 의 한계도 동시에 해소한다.

- **저장소 일원화**: 기존 flows/agents 와 동일한 SQLite (`~/.xflow/xflow.db`) 에 모든 영속
  데이터를 통합. JSON 파일 1차 채택 결정 폐기 — 트랜잭션/동시쓰기 보호/향후 history 확장이
  용이하다.
- **사용자 컨텍스트 반영**: basic_auth 가 다중 사용자(admin/editor/viewer 역할)를 이미
  지원하고 있으므로 단일 운영자 가정(ASM-002)을 폐기하고, 공유 + 개인 병행 모델을 1급
  지원한다.
- **자격증명도 SQLite 로**: `~/.xflow/users.yaml` 의 사용자 정보를 SQLite `users` 테이블로
  이관하여 저장소 분산을 제거한다. 백워드 호환을 위해 기존 `CredentialsManager` API 는
  그대로 유지한다.

### 영향 범위

**신규 파일**

- `internal/storage/dashboard_sqlite.go` — `dashboards` 테이블 기반 `DashboardRepository`
  1차 구현 (v0.2.0 의 유일한 구현).
- `internal/storage/users_sqlite.go` — `users` 테이블 CRUD 헬퍼(또는
  `credentials.go` 내부 직접 교체). yaml → SQLite 자동 마이그레이션 포함.
- `internal/api/handler/dashboard.go` — `/api/dashboards/shared` 와
  `/api/dashboards/mine` 2벌 핸들러 + 권한 미들웨어.
- `internal/api/dto/dashboard.go` — `DashboardSnapshot` DTO.
- `web/src/types/dashboard.ts` — `DashboardSnapshot`, `DashboardScope` 타입.
- `web/src/services/api/dashboardService.ts` — REST 클라이언트 (`getShared`, `putShared`,
  `deleteShared`, `getMine`, `putMine`, `deleteMine`).
- `web/src/hooks/useDashboardSync.ts` — 부팅 시 GET shared + GET mine 병렬 + 변경 폴백 PUT.

**수정 파일**

- `internal/storage/sqlite.go` — `dashboards`, `users` 테이블 자동 생성 추가
  (`CREATE TABLE IF NOT EXISTS`).
- `internal/auth/credentials.go` — 백엔드를 SQLite 기반으로 교체. 외부 API 시그니처
  (`Load`/`Save`/`Authenticate`/`ChangePassword`/`EnsureDefaultAdmin`/`GetUser`) 는 유지.
  yaml 파일 존재 시 1회성 자동 마이그레이션 후 `users.yaml.migrated` 로 rename.
- `cmd/xflowd/main.go` — `dashboards` 라우트 등록(shared/mine), 권한 미들웨어 연결,
  SQLite 기반 `CredentialsManager` 초기화 순서 조정.
- `web/src/stores/uiStore.ts` — `partialize` 축소(대시보드 관련 키 모두 제거), 부팅 시
  기존 localStorage 키 명시적 제거 로직 추가.
- `web/src/pages/dashboard/DashboardPage.tsx` — "공유 / 내 대시보드" 탭 UI 추가, 현재
  스코프에 따라 `dashboardPages` 를 양쪽 store/snapshot 으로부터 선택.

**제거 파일**

- 없음. (v0.1.0 의 `internal/storage/dashboard_file.go` 는 실제 작성된 적이 없으므로
  v0.2.0 에서는 작성하지 않는다. v0.1.0 SPEC 의 해당 결정은 폐기되었음을 본 HISTORY 와
  Open Issues 에 명시한다.)

**범위 외**

- 사용자 등록/삭제/목록 관리 REST API 는 별도 SPEC(`SPEC-USER-MGMT-001`, 추후) 로 분리.
  본 SPEC 은 기존 `EnsureDefaultAdmin` 의 admin/admin 자동 생성과 `ChangePassword` 동작만
  SQLite 위에서 유지한다(OI-004).
- 개인 대시보드의 읽기 전용 link share / 공유 기능은 OI-005 로 추후 검토.
- WebSocket 실시간 브로드캐스트는 OI-002 유지(deferred).

---

## 가정 (Assumptions)

- **ASM-001** (유지): xflowd 데몬은 단일 인스턴스로 운영되며, 데이터 디렉토리(`~/.xflow/`)
  는 호스트 단일 파일 시스템에 위치한다.
- **ASM-002** (**폐기**, v0.2.0): 0.1.0 의 "단일 운영자가 전역적으로 동일한 대시보드 구성을
  공유" 가정은 무효. 본 SPEC 은 다중 사용자(admin/editor/viewer) 를 1급 지원하고
  공유 + 개인 병행 모델을 채택한다.
- **ASM-003** (**변경**, v0.2.0): basic_auth 는 **필수** 이다.
  `serverCfg.BasicAuth.Enabled=true` 가 강제되며, 비활성화 상태로는 부팅을 거부하거나 부팅
  과정에서 자동으로 활성화한다(구현은 plan.md 의 결정에 따른다). 익명 접근은 모든
  `/api/dashboards/*` 엔드포인트에서 불허된다.
- **ASM-004** (유지): 대시보드 페이지 수는 사용자당 평균 10개 이하, 페이지당 패널 수는
  평균 20개 이하, 단일 snapshot JSON 크기는 256 KB 이하로 가정한다. 이는 단일 row 로 다루
  기에 충분하다.
- **ASM-005** (유지): 클라이언트는 변경 시 즉시(debounce 500ms) 서버에 전체 snapshot 을
  PUT 한다. 변경 단위(페이지/패널/레이아웃) 별 부분 업데이트는 도입하지 않는다.
- **ASM-006** (**폐기**, v0.2.0): "localStorage → 서버 1회성 마이그레이션" 가정은 폐기.
  본 SPEC 은 블랭크 슬레이트 정책을 채택하고, 기존 `localStorage` 의 대시보드 키들은
  부팅 시 클라이언트가 명시적으로 1회 제거한다.
- **ASM-007** (신규): SQLite WAL 모드 (`PRAGMA journal_mode=WAL`, 기존 `sqlite.go:38`)
  하에서 단일 호스트 단일 데몬이 `~/.xflow/xflow.db` 를 단독 점유한다. 멀티 인스턴스
  운영은 본 SPEC 범위 외이다.
- **ASM-008** (신규): 부팅 시 `~/.xflow/users.yaml` 이 존재하면 1회성으로 SQLite `users`
  테이블에 `INSERT OR IGNORE` 후 yaml 파일을 `users.yaml.migrated` 로 rename 하여 더 이상
  사용하지 않는다. SQLite `users` 테이블이 비어 있고 yaml 도 없으면 admin/admin 기본 계정을
  자동 생성한다(기존 `EnsureDefaultAdmin` 동작 유지).
- **ASM-009** (신규): JWT `Claims.Username` (=`sub`) 은 사용자별 대시보드의 owner 결정자
  로 사용되며, 클라이언트는 PUT 페이로드에 `owner` 를 보내지 않는다(보내더라도 서버가
  무시한다). owner spoofing 은 차단된다.

---

## 요구사항 (Requirements - EARS Format)

### Ubiquitous Requirements (항상 active)

- **UR-001**: 시스템은 **항상** 대시보드 구성(`DashboardSnapshot`) 을 SQLite
  `dashboards` 테이블에 영속 저장해야 한다.
- **UR-002**: 시스템은 **항상** `DashboardSnapshot` 에 `scope` (`'global'` 또는
  `'user'`), `owner` (scope=global 시 NULL, scope=user 시 username), `version` (단조
  증가 정수), `updatedAt` (epoch ms, int64) 필드를 포함해야 한다.
- **UR-003**: 시스템은 **항상** PUT 페이로드의 JSON schema 유효성과 페이로드 크기 한도
  (256 KB) 를 서버측에서 검증해야 한다 — 위반 시 `400 Bad Request` 또는 `413 Payload
  Too Large` 로 거부한다.
- **UR-004**: 시스템은 **항상** `basic_auth` 가 활성화된 상태로 운영되어야 하며,
  `/api/dashboards/*` 의 모든 엔드포인트는 유효한 JWT 를 요구한다 (익명 접근 불허).
- **UR-005**: 시스템은 **항상** `version` 과 `updatedAt` 을 서버가 부여한다 — 클라이언트
  페이로드의 해당 값은 무시된다.
- **UR-006**: 시스템은 **항상** 자격증명을 SQLite `users` 테이블에서 조회·저장한다.
  부팅 시 yaml 파일이 존재하면 1회성 자동 마이그레이션 후 yaml 을 `users.yaml.migrated`
  로 rename 하여 더 이상 사용하지 않는다.

### Event-Driven Requirements (이벤트 기반)

- **WHEN** 사용자가 공유 대시보드 페이지를 추가/수정/삭제하거나 패널 레이아웃을 변경하면,
  **THEN** 클라이언트는 500ms debounce 후 `PUT /api/dashboards/shared` 로 전체 snapshot
  을 전송해야 한다 (서버는 요청자가 admin 인지 검증).
- **WHEN** 사용자가 개인 대시보드를 변경하면, **THEN** 클라이언트는 500ms debounce 후
  `PUT /api/dashboards/mine` 로 전체 snapshot 을 전송해야 한다 (서버가 JWT 의 username
  으로 owner 자동 결정).
- **WHEN** 클라이언트가 페이지 로드 또는 새로고침(F5) 후 첫 렌더 직전, **THEN**
  클라이언트는 `GET /api/dashboards/shared` 와 `GET /api/dashboards/mine` 을 **병렬**
  로 호출하여 두 snapshot 으로 Zustand store 의 공유/개인 영역을 각각 초기화해야 한다.
- **WHEN** 서버에 해당 scope 의 snapshot 이 존재하지 않고 (`404 Not Found`),
  **THEN** 클라이언트는 빌트인 기본 대시보드(`DEFAULT_DASHBOARD_PAGE`) 로 그 scope 의
  메모리 상태를 초기화하고, 사용자의 **첫 변경 시점**에 자동으로 PUT 하여 snapshot 을
  최초 생성해야 한다.
- **WHEN** 클라이언트가 PUT 요청 시 `If-Match: <version>` 헤더를 포함하고 서버의 현재
  version 이 헤더 값과 다르면, **THEN** 서버는 `409 Conflict` 와 함께 서버측 최신
  snapshot 을 반환해야 한다 (last-write-wins 충돌 정책 유지).
- **WHEN** v0.2.0 으로 첫 부팅한 클라이언트가 페이지를 처음 로드하면, **THEN**
  클라이언트는 기존 `localStorage.xflow-ui` 의 `dashboardPages`, `activeDashboardId`,
  `dashboardGridCols`, `dashboardShowGridLines`, `dashboardRefreshInterval`,
  `deviceGridLayout` 키들을 **1회** 명시적으로 제거하고, 사용자에게 토스트로 "이번
  업데이트로 대시보드 구성이 서버 저장으로 전환되었습니다. 기존 로컬 구성은 초기화됩니다."
  를 안내해야 한다.

### State-Driven Requirements (상태 기반)

- **IF** 요청에 유효한 JWT 가 없거나 만료된 상태이면, **THEN** 시스템은
  `401 Unauthorized` 를 반환해야 한다 (모든 `/api/dashboards/*` 엔드포인트).
- **IF** 요청자가 admin 역할이 아닌 상태에서 `PUT /api/dashboards/shared` 또는
  `DELETE /api/dashboards/shared` 를 호출하면, **THEN** 시스템은 `403 Forbidden`
  을 반환해야 한다 (editor/viewer 는 공유 대시보드를 GET 만 가능).
- **IF** `PUT` 요청의 `If-Match: <version>` 헤더가 서버의 현재 version 과 일치하지 않는
  상태이면, **THEN** 서버는 `409 Conflict` 와 함께 서버측 최신 snapshot 을 반환해야
  한다.
- **IF** in-flight PUT 이 존재하는 상태이면, **THEN** 클라이언트는 추가 변경을 큐에 쌓아
  직전 PUT 응답 수신 후(성공 또는 conflict 해결 후) 다음 PUT 을 발행해야 한다 — 동시
  PUT 금지.
- **IF** 응답이 `409 Conflict` 인 상태이면, **THEN** 클라이언트는 응답 body 의 서버
  snapshot 을 메모리에 적용하고 자신의 변경을 `If-Match: <new-version>` 으로 1회만 재
  PUT 한다 (last-write-wins, 무한루프 방지).
- **IF** 부팅 시 `serverCfg.BasicAuth.Enabled=false` 인 상태이면, **THEN** 시스템은
  부팅을 거부하거나(권장) 강제 활성화 후 경고 로그를 출력해야 한다.

### Optional Requirements (선택사항, deferred)

- **OR-001** (deferred): 가능하면 시스템은
  `GET /api/dashboards/shared/history`,
  `GET /api/dashboards/mine/history` 엔드포인트로 최근 N 개 변경 이력을 제공할 수 있다.
  v0.2.0 미구현 — 별도 SPEC.
- **OR-002** (deferred): 가능하면 시스템은 WebSocket `dashboards_shared_changed`,
  `dashboards_mine_changed` 이벤트로 동일 scope 의 다른 클라이언트에 broadcast 할 수
  있다. v0.2.0 미구현 — OI-002.
- **OR-003** (deferred): 가능하면 시스템은 페이지 단위/패널 단위 부분 업데이트
  (`PATCH /api/dashboards/{scope}/pages/{id}`) 를 제공할 수 있다. v0.2.0 은 전체
  snapshot PUT 만 지원.

### Unwanted Behavior Requirements (금지 동작)

- **UB-001**: 시스템은 인증된 사용자의 변경을 **소리 없이 누락** (silent drop) 하지
  **않아야** 한다 — 모든 PUT 응답은 명시적 상태 코드 (200/400/401/403/409/413/500) 와 함께
  반환된다.
- **UB-002**: 클라이언트는 `localStorage` 의 `dashboardPages`/`activeDashboardId`/그리드
  설정/`deviceGridLayout` 을 source-of-truth 로 사용해서는 **안 된다** — v0.2.0 부터는
  partialize 대상에서 완전 제외되며, 기존 키는 부팅 시 1회 명시적으로 제거된다.
- **UB-003**: 서버는 클라이언트가 제공한 `version`/`updatedAt`/`owner` 를 **그대로** 저장
  해서는 **안 된다** — 서버가 PUT 성공 시점에 `version = old.version + 1`,
  `updatedAt = now()` 을 부여하고, `owner` 는 JWT `Claims.Username` 에서 결정한다.
  클라이언트가 보낸 `owner` 는 무시된다 (owner spoofing 차단).
- **UB-004**: 시스템은 비 admin 사용자가 `/api/dashboards/shared` 를 PUT/DELETE 하는
  것을 **허용해서는 안 된다** — 명시적으로 `403 Forbidden` 응답.
- **UB-005**: 시스템은 사용자 A 가 사용자 B 의 개인 대시보드를 GET/PUT/DELETE 하는
  것을 **허용해서는 안 된다** — `/api/dashboards/mine` 은 항상 JWT 의 username 으로만
  owner 가 결정되며, 별도의 username 파라미터를 받지 않는다.
- **UB-006**: 시스템은 PUT 페이로드의 `scope` 와 URL 의 scope (shared/mine) 가 불일치
  하는 경우 silent normalize 하지 **않아야** 한다 — `400 Bad Request` 로 명시적으로
  거부한다 (URL 이 source-of-truth).
- **UB-007**: 시스템은 자격증명을 yaml 과 SQLite 양쪽에 **동시 유지하지 않아야** 한다 —
  yaml → SQLite 마이그레이션 완료 후 yaml 파일은 즉시 `users.yaml.migrated` 로 rename
  되며 이후 어떤 인증 흐름에서도 참조되지 않는다.

---

## 명세 (Specifications)

### API 엔드포인트

| Method | Path                           | Description                            | 권한                  |
| ------ | ------------------------------ | -------------------------------------- | --------------------- |
| GET    | `/api/dashboards/shared`       | 공유 snapshot 조회                     | 인증된 사용자 (전부)  |
| PUT    | `/api/dashboards/shared`       | 공유 snapshot 저장 (`If-Match` 권장)   | **admin only**        |
| DELETE | `/api/dashboards/shared`       | 공유 snapshot 삭제                     | **admin only**        |
| GET    | `/api/dashboards/mine`         | 본인 개인 snapshot 조회                | 인증된 사용자         |
| PUT    | `/api/dashboards/mine`         | 본인 개인 snapshot 저장 (`If-Match`)   | 인증된 사용자         |
| DELETE | `/api/dashboards/mine`         | 본인 개인 snapshot 삭제                | 인증된 사용자         |

응답 코드:

- `200 OK` — 성공
- `204 No Content` — DELETE 성공
- `400 Bad Request` — payload schema 오류, scope/URL 불일치
- `401 Unauthorized` — JWT 없음/만료
- `403 Forbidden` — 권한 부족 (예: editor 가 shared PUT)
- `404 Not Found` — GET 시 snapshot 미존재 (초기 상태)
- `409 Conflict` — PUT 시 `If-Match` version 불일치 (body 에 서버측 최신 snapshot)
- `413 Payload Too Large` — payload 크기 256 KB 초과
- `500 Internal Server Error` — 저장소 I/O 실패

### 데이터 모델

```typescript
// web/src/types/dashboard.ts (신규)
export type DashboardScope = 'global' | 'user';

export interface DashboardSnapshot {
  // 메타데이터 (서버가 부여, 클라이언트 PUT 페이로드에서는 무시됨)
  scope: DashboardScope;        // URL 로부터 결정 (shared=global, mine=user)
  owner: string | null;          // scope=global 시 null, scope=user 시 username
  version: number;               // 단조 증가 정수, 서버 부여
  updatedAt: number;             // epoch ms (int64), 서버 부여

  // 페이로드 (클라이언트 source-of-truth)
  payload: {
    dashboardPages: DashboardPageConfig[];
    activeDashboardId: string;
    dashboardGridCols: number;
    dashboardShowGridLines: boolean;
    dashboardRefreshInterval: number;
    deviceGridLayout: Record<string, DashboardLayoutItem[]>;
  };
}
```

```go
// internal/api/dto/dashboard.go (신규)
type DashboardSnapshot struct {
    Scope     string                 `json:"scope"`      // "global" | "user"
    Owner     *string                `json:"owner"`      // global 시 nil, user 시 *username
    Version   int64                  `json:"version"`
    UpdatedAt int64                  `json:"updatedAt"`  // epoch ms
    Payload   map[string]interface{} `json:"payload"`
}
```

### SQLite 스키마 (DDL)

기존 `sqlite.go` 의 `flows` 테이블 자동 생성과 동일한 패턴(`CREATE TABLE IF NOT EXISTS`)
으로 `NewSQLiteRepository` 또는 별도 `Migrate(ctx, db)` 함수에서 실행한다. golang-migrate
는 도입하지 않는다.

```sql
-- 대시보드 snapshot 저장소
CREATE TABLE IF NOT EXISTS dashboards (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    scope      TEXT    NOT NULL CHECK (scope IN ('global', 'user')),
    owner      TEXT,            -- scope=global 시 NULL, scope=user 시 username
    version    INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL,   -- epoch ms
    payload    TEXT    NOT NULL    -- JSON
);

-- (scope, owner) 의 유일성을 NULL 친화적으로 보장
-- SQLite 는 UNIQUE 제약에서 NULL 을 distinct 로 취급하므로 COALESCE 기반 partial index 사용
CREATE UNIQUE INDEX IF NOT EXISTS dashboards_scope_owner_uidx
    ON dashboards(scope, COALESCE(owner, ''));

-- 사용자 자격증명 (yaml 대체)
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    role          TEXT    NOT NULL DEFAULT 'viewer'
                  CHECK (role IN ('admin', 'editor', 'viewer')),
    created_at    INTEGER NOT NULL,   -- epoch ms
    updated_at    INTEGER NOT NULL    -- epoch ms
);
```

### 저장소 인터페이스

```go
// internal/storage/dashboard.go (신규)
type DashboardRepository interface {
    // Get 은 (scope, owner) 의 단일 snapshot 을 조회한다. 없으면 ErrDashboardNotFound.
    // scope=global 시 owner 는 "" (빈 문자열) 로 호출한다.
    Get(ctx context.Context, scope, owner string) (*DashboardSnapshot, error)
    // Put 은 snapshot 을 저장하고 새 version 을 부여한다.
    // expectedVersion 이 -1 이면 unconditional, >= 0 이면 If-Match 검증.
    Put(ctx context.Context, scope, owner string, payload []byte, expectedVersion int64) (*DashboardSnapshot, error)
    // Delete 는 snapshot 을 삭제한다.
    Delete(ctx context.Context, scope, owner string) error
}

var ErrDashboardNotFound = errors.New("dashboard snapshot not found")
var ErrDashboardVersionMismatch = errors.New("dashboard version mismatch")
```

`scope, owner` 의미:

- scope=`"global"`, owner=`""` → 공유 대시보드 (단일 row, owner 컬럼은 NULL 로 저장)
- scope=`"user"`, owner=`<username>` → 특정 사용자의 개인 대시보드

핸들러 레이어가 URL (`/shared` vs `/mine`) 과 JWT 의 `Claims.Username` 으로부터 적절한
(scope, owner) 쌍을 결정하여 저장소를 호출한다. 클라이언트는 (scope, owner) 를 직접
지정할 수 없다.

### 저장소 구현 (v0.2.0)

**채택**: `internal/storage/dashboard_sqlite.go` — SQLite `dashboards` 테이블 기반의
유일한 구현.

- 기존 `~/.xflow/xflow.db` (`flows`, `agents` 와 공유) 를 그대로 사용. WAL 모드 활성
  (`sqlite.go:38`) 으로 단일 데몬의 동시 읽기 성능 확보.
- 트랜잭션 내에서 SELECT → UPDATE 순서로 If-Match (`expectedVersion`) 검증 및 version
  증가 수행.
- `Put` 성공 시 새 `version = COALESCE(old.version, 0) + 1`, `updated_at = epoch_ms(now())`
  를 서버가 부여한다.
- `internal/storage/dashboard_file.go` 는 작성하지 않는다 (v0.1.0 결정 폐기).

### 자격증명 저장소 (`internal/auth/credentials.go`)

외부 API 인터페이스(`Load`, `Save`, `Authenticate`, `ChangePassword`, `EnsureDefaultAdmin`,
`GetUser`) 는 그대로 유지하고, 내부 구현만 SQLite 로 교체한다. 호출자(예
`cmd/xflowd/main.go`) 의 코드 변경을 최소화한다.

부팅 시 동작 (`EnsureDefaultAdmin` 진입점):

1. `users` 테이블이 비어 있고 `~/.xflow/users.yaml` 이 존재하면 → yaml 파싱 후
   `INSERT OR IGNORE INTO users(...)` 로 1회성 이관. `users.yaml` 을
   `users.yaml.migrated` 로 rename. 로그: `auth: migrated N users from yaml to sqlite`.
2. `users` 테이블이 여전히 비어 있으면 → admin/admin (role=admin) 자동 생성 + 경고 로그
   (기존 동작 유지).
3. 그 외의 경우 → 정상 진행, 아무 작업도 하지 않음.

`Authenticate`, `ChangePassword` 는 SQLite `users` 테이블을 직접 조회·갱신한다.
타이밍 공격 방지(존재하지 않는 사용자에 대해서도 bcrypt 더미 비교) 는 그대로 유지한다.

**범위 외**: 사용자 등록/삭제/목록 관리 REST API 는 OI-004 로 분리(별도 SPEC).
본 SPEC 에서는 admin 의 비밀번호 변경 (`PUT /api/auth/change-password`, 기존 핸들러)
이 SQLite 위에서 정상 동작함만 보장한다.

### 프론트엔드 동기화 전략

**부팅 시 (App mount)**: `useDashboardSync()` 훅이 다음을 수행한다.

1. 기존 `localStorage.xflow-ui` 가 v0.1.0 이전 형식이면 (즉 `dashboardPages` 등 키가
   존재하면) **1회만** 해당 키들을 명시적으로 제거하고 토스트로 안내한다 (블랭크
   슬레이트). 이 시점에 서버로 PUT 하지 않는다.
2. `GET /api/dashboards/shared` 와 `GET /api/dashboards/mine` 을 병렬로 호출한다.
   - 200 → 각 scope 의 메모리 상태를 서버 snapshot 으로 초기화.
   - 404 → 빌트인 `DEFAULT_DASHBOARD_PAGE` 로 해당 scope 의 메모리 상태 초기화.
     (첫 변경 시 자동 PUT 으로 snapshot 최초 생성)
   - 401 → 로그인 화면으로 리다이렉트 (기존 인증 흐름).
   - 403 (개인 대시보드에서는 발생하지 않음. 공유 GET 은 모두 허용)
3. 사용자의 현재 활성 탭("공유" / "내 대시보드") 에 따라 컴포넌트에 노출할 페이지 목록을
   선택한다.

**변경 시 (Zustand store mutation)**:

- 500ms debounce 후 활성 스코프에 따라 `PUT /api/dashboards/shared` 또는
  `PUT /api/dashboards/mine` 호출.
- 페이로드: `{ payload: { ... } }` 만 전송 (scope/owner/version/updatedAt 은 서버 결정).
- 헤더: `If-Match: <last-known-version>` (없으면 unconditional).
- 200 응답: 새 `version` 을 메모리에 기록.
- 403 응답(공유 PUT 시 admin 아님): 사용자에게 토스트로 "공유 대시보드 편집 권한이
  없습니다." 안내. 변경은 메모리에 남기되 다음 변경 시도에서 동일 결과. UI 측면에서
  admin 이 아닐 때는 공유 탭의 편집 컨트롤을 비활성화하여 사전 방지한다.
- 409 응답: 응답 body 의 server snapshot 을 store 에 적용한 뒤, 로컬 변경을 `If-Match:
  <server-version>` 으로 1회 재PUT (last-write-wins). 두 번째 409 발생 시 사용자에게
  토스트로 알리고 강제 동기화.

**Zustand `partialize` 변경** (`web/src/stores/uiStore.ts`):

- 제거 대상: `dashboardPages`, `activeDashboardId`, `dashboardGridCols`,
  `dashboardShowGridLines`, `dashboardRefreshInterval`, `deviceGridLayout`.
- 유지: `sidebarCollapsed`, `theme`, `customThemeTokens` (기기별 환경설정).
- 추가: 부팅 시 마이그레이션 키 제거 표식 (`xflow-ui:dashboard-migrated-v0.2`) 을
  localStorage 에 저장하여 토스트/제거 로직이 1회만 동작하도록 보장.

### localStorage 마이그레이션 폐기 (블랭크 슬레이트)

v0.1.0 의 "1회성 마이그레이션 (localStorage → 서버 PUT)" 흐름은 **완전히 삭제**된다.
근거:

- v0.2.0 부터는 사용자별 스코프와 권한 모델이 도입되므로, 클라이언트 측의 단일 페이로드를
  "공유" 와 "개인" 중 어디로 보낼지 결정할 수 없다 (사용자 동의 없이 자의적 결정 불가).
- 대신 모든 사용자는 빌트인 기본 대시보드에서 새로 시작하며, admin 이 공유 대시보드를
  새로 구성한다.

대신 다음을 보장한다:

- 부팅 시 클라이언트가 `localStorage.xflow-ui` 의 v0.1 시절 키들을 1회 명시적으로 제거.
- 사용자에게 토스트 알림: "이번 업데이트로 대시보드 구성이 서버 저장으로 전환되었습니다.
  기존 로컬 구성은 초기화됩니다."
- CHANGELOG 항목에 동일 내용을 명시.

### 백워드 호환

- 기존 SPEC-CHART-001, SPEC-WEB-005 등에서 정의된 패널/페이지 schema 는 그대로 유지된다.
- 페이로드 schema 변경 시 서버는 `payload.schemaVersion` (선택, 기본 1) 필드를 검사하고
  알 수 없는 값이면 400 반환.
- `internal/auth/credentials.go` 의 외부 API 시그니처를 유지하므로 인증 핸들러/미들웨어
  (예 login, change-password) 의 호출부는 변경 없이 동작한다.

### Traceability

| Requirement                              | Implementation Files                                                                          | Tests                                                                |
| ---------------------------------------- | --------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| UR-001~003 (SQLite 영속화, schema, 크기) | `internal/storage/dashboard_sqlite.go`, `internal/storage/sqlite.go`                          | `dashboard_sqlite_test.go`                                           |
| UR-004 (basic_auth 필수)                 | `cmd/xflowd/main.go` (boot guard), `internal/api/handler/dashboard.go`                        | `main_boot_test.go` (또는 통합), `dashboard_test.go` (auth case)     |
| UR-005 (server-side version/updatedAt)   | `internal/storage/dashboard_sqlite.go`                                                        | `dashboard_sqlite_test.go` (version monotonic)                       |
| UR-006 (yaml → SQLite 마이그레이션)      | `internal/auth/credentials.go`, `internal/storage/users_sqlite.go`                            | `credentials_migration_test.go`                                      |
| WHEN shared/mine PUT debounce            | `web/src/hooks/useDashboardSync.ts`, `web/src/services/api/dashboardService.ts`               | `useDashboardSync.test.ts`                                           |
| WHEN boot → GET shared+mine (parallel)   | `web/src/hooks/useDashboardSync.ts`                                                           | `useDashboardSync.test.ts` (boot scenario)                           |
| WHEN 404 → built-in default              | `web/src/hooks/useDashboardSync.ts`                                                           | `useDashboardSync.test.ts` (404 fallback)                            |
| WHEN v0.2 첫 부팅 → localStorage 제거    | `web/src/stores/uiStore.ts`, `web/src/hooks/useDashboardSync.ts`                              | `useDashboardSync.test.ts` (blank slate)                             |
| IF auth → 401                            | `internal/api/handler/dashboard.go`, JWT 미들웨어                                             | `dashboard_test.go` (auth case)                                      |
| IF non-admin shared PUT → 403            | `internal/api/handler/dashboard.go` (role middleware)                                         | `dashboard_test.go` (rbac case)                                      |
| IF version mismatch → 409                | `internal/storage/dashboard_sqlite.go`, `internal/api/handler/dashboard.go`                   | `dashboard_sqlite_test.go`, `dashboard_test.go` (conflict)           |
| UB-003 (owner spoofing 차단)             | `internal/api/handler/dashboard.go` (페이로드 owner 무시)                                     | `dashboard_test.go` (spoofing case)                                  |
| UB-005 (다른 사용자 mine 접근 차단)      | `internal/api/handler/dashboard.go` (JWT username 기반 owner 결정)                            | `dashboard_test.go` (cross-user case)                                |
| UB-006 (URL scope vs body scope)         | `internal/api/handler/dashboard.go`                                                           | `dashboard_test.go` (scope mismatch)                                 |
| UB-007 (yaml 잔류 금지)                  | `internal/auth/credentials.go` (rename 검증)                                                  | `credentials_migration_test.go` (rename verified)                    |

---

## Open Issues

- **OI-001 (사용자 분리/인증 도입 시점) — CLOSED in v0.2.0**

  공유 + 개인 병행 모델을 본 SPEC 에서 채택. `/api/dashboards/shared` 와
  `/api/dashboards/mine` URL 분리, 역할(admin/editor/viewer) 기반 인가, JWT
  `Claims.Username` 으로 owner 자동 결정.

- **OI-002 (WebSocket 실시간 동기화) — 유지 (deferred)**

  여러 브라우저가 동시에 같은 (공유 또는 동일 사용자의 개인) 대시보드를 편집할 때 실시간
  broadcast 가 필요하면 별도 SPEC 으로 정의 (OR-002 참조). v0.2.0 은 polling 또는 다음
  PUT 시점에 409 처리로 한정.

- **OI-003 (저장소 백엔드 선택) — CLOSED in v0.2.0**

  SQLite (`modernc.org/sqlite`, cgo-free) 채택. 기존 `flows`/`agents` 와 동일한
  `~/.xflow/xflow.db` 공유. golang-migrate 미사용, `CREATE TABLE IF NOT EXISTS` 패턴
  유지.

- **OI-004 (신규) — 사용자 등록/삭제/목록 관리 API — `SPEC-USER-MGMT-001` (추후)**

  본 SPEC 은 자격증명 저장소를 yaml 에서 SQLite 로 이관하는 데까지만 다룬다. 관리 REST
  API (`POST /api/auth/users`, `DELETE /api/auth/users/{username}`,
  `GET /api/auth/users` 등) 는 별도 SPEC 으로 분리. 본 SPEC 범위에서는 기존
  `EnsureDefaultAdmin` (admin/admin 자동 생성) 과 `ChangePassword` (본인 비밀번호 변경)
  만 SQLite 위에서 보장.

- **OI-005 (신규) — 개인 대시보드 공유 (read-only link share) — 추후 검토**

  특정 사용자가 자신의 개인 대시보드를 다른 사용자에게 읽기 전용 링크로 공유하는 기능.
  요구가 명확해지면 별도 SPEC 으로 분리. v0.2.0 범위 외.

---

## 비고 (Notes)

- 본 SPEC 은 SPEC-WEB-005 (테마/사용자환경), SPEC-CHART-001 (차트 패널), SPEC-WEB-006
  (업데이트 UI) 과 독립적이며, 그들의 패널 schema 를 변경하지 않는다.
- `theme`, `customThemeTokens`, `sidebarCollapsed` 는 **기기별 개인 환경설정**이므로 서버
  영속화 대상에서 **제외**한다 (사용자가 다른 기기/브라우저에서 다른 테마를 쓸 수 있어야
  함).
- `dashboardRefreshInterval`, `dashboardGridCols`, `dashboardShowGridLines`,
  `deviceGridLayout` 은 대시보드 자체 구성으로 보아 (공유 또는 개인) snapshot 에 포함하여
  서버 영속화 한다.
- localStorage 의 v0.1 시절 키들은 v0.2.0 첫 부팅 시 클라이언트가 명시적으로 제거하며,
  사용자에게는 토스트 한 번으로 안내한다. 이후 partialize 에서 더 이상 직렬화되지 않는다.
- **사용자 안내 (UI 토스트 / CHANGELOG 공통 문구)**:
  "이번 업데이트(v0.2.0)로 대시보드 구성이 서버 저장으로 전환되었습니다. 기존 로컬 구성은
  초기화됩니다. 공유 대시보드는 관리자가 다시 구성해 주세요."
