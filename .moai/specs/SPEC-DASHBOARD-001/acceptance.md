---
spec_id: SPEC-DASHBOARD-001
title: 대시보드 구성 서버 영속화 — Acceptance Criteria
version: 0.2.0
status: implemented
created: 2026-05-12
updated: 2026-05-12
---

# SPEC-DASHBOARD-001: Acceptance Criteria (v0.2.0)

> v0.2.0 핵심 변경: SQLite + 공유/개인 병행 + 사용자 SQLite 이관 + localStorage
> 블랭크 슬레이트. 0.1.0 의 시나리오는 본 문서에서 새 API/권한 모델 기준으로 재작성
> 되었다. 자세한 결정 근거는 `spec.md` HISTORY 0.2.0 참조.

## 시나리오 (Given-When-Then)

### AC-1: 첫 부팅 — SQLite 및 yaml 모두 비어 있는 경우 (블랭크 슬레이트)

- **Given** xflowd 가 처음 실행되어 `~/.xflow/xflow.db` 가 갓 생성되었고 `dashboards`
  와 `users` 테이블이 자동 생성되었다
- **And** `~/.xflow/users.yaml` 파일이 존재하지 않는다
- **When** xflowd 부팅이 진행된다
- **Then** `EnsureDefaultAdmin` 이 admin/admin (role=admin) 계정을 `users` 테이블에
  자동 생성한다
- **And** 경고 로그 "기본 admin 계정이 생성되었습니다. 운영 환경에서는 반드시 비밀번호를
  변경하세요." 가 출력된다
- **When** 운영자가 처음 접속한 브라우저에서 admin 으로 로그인한다
- **Then** 클라이언트는 `GET /api/dashboards/shared` 와 `GET /api/dashboards/mine`
  을 병렬로 호출한다
- **And** 두 호출 모두 `404 Not Found` 를 반환받고, 각각 빌트인 `DEFAULT_DASHBOARD_PAGE`
  로 메모리 상태가 초기화된다
- **And** 사용자에게는 활성 탭("공유" 또는 "내 대시보드") 에 빈 기본 대시보드가 보인다

### AC-2: 기존 yaml 사용자 → SQLite 자동 이관

- **Given** v0.1.x 에서 운영되던 `~/.xflow/users.yaml` 에 admin, alice, bob 세 사용자가
  저장되어 있다
- **And** SQLite `users` 테이블은 비어 있다
- **When** v0.2.0 xflowd 가 부팅한다
- **Then** `EnsureDefaultAdmin` 이 yaml 을 파싱하고 `INSERT OR IGNORE INTO users` 로
  3명을 이관한다
- **And** 로그 "auth: migrated 3 users from yaml to sqlite" 가 출력된다
- **And** `~/.xflow/users.yaml` 이 `~/.xflow/users.yaml.migrated` 로 rename 된다
- **When** alice 가 자신의 기존 비밀번호로 로그인한다
- **Then** 인증이 성공하고 JWT 가 발급된다
- **And** 이후 부팅에서는 yaml.migrated 가 무시되고 SQLite 만 source-of-truth 로 사용
  된다 (yaml 잔류 없음 — UB-007)

### AC-3: admin 이 공유 대시보드 편집 → 다른 사용자에게 동일하게 보임

- **Given** admin 으로 로그인하여 활성 탭이 "공유" 이다
- **And** 서버에 공유 snapshot 이 없다 (404 상태)
- **When** admin 이 새 페이지를 추가하고 패널을 배치한다
- **Then** 클라이언트는 500ms 이내에 `PUT /api/dashboards/shared` 를 호출한다
  (If-Match 없음, 최초 생성)
- **And** 서버는 `200 OK` 와 함께 `{ scope: 'global', owner: null, version: 1,
  updatedAt: <now-ms>, payload: { ... } }` 를 반환한다
- **And** SQLite `dashboards` 테이블에 `scope='global', owner=NULL, version=1` row 가
  생성된다
- **When** editor 또는 viewer 가 다른 브라우저로 로그인하여 "공유" 탭을 본다
- **Then** 클라이언트는 `GET /api/dashboards/shared` 로 동일한 snapshot 을 받는다
- **And** admin 이 추가한 페이지/패널이 그대로 보인다

### AC-4: editor 가 공유 PUT 시도 → 403 Forbidden

- **Given** editor 역할로 로그인된 사용자가 "공유" 탭을 본다
- **And** UI 측에서는 편집 컨트롤(추가/삭제/이름 변경/패널 드래그)이 비활성화되어 있다
- **When** 사용자가 (개발자 도구 등으로) 직접 `PUT /api/dashboards/shared` 를 호출
  한다
- **Then** 서버는 `403 Forbidden` 을 반환한다
- **And** SQLite 의 공유 snapshot 은 변경되지 않는다
- **And** 클라이언트에서는 토스트 "공유 대시보드 편집 권한이 없습니다." 가 노출된다

### AC-5: 사용자 A 의 개인 PUT → 사용자 B 의 mine 에 영향 없음 (cross-user 격리)

- **Given** alice 와 bob 두 사용자가 SQLite `users` 에 존재한다
- **And** SQLite `dashboards` 에는 아직 어떤 사용자 개인 snapshot 도 없다
- **When** alice 가 로그인하여 "내 대시보드" 탭에서 페이지를 추가한다
- **Then** 클라이언트는 `PUT /api/dashboards/mine` 을 호출하고 `200 OK` 와 함께
  `{ scope: 'user', owner: 'alice', version: 1 }` 를 반환받는다
- **And** SQLite 에 `scope='user', owner='alice', version=1` row 가 생성된다
- **When** bob 이 다른 브라우저에서 로그인하여 "내 대시보드" 탭을 본다
- **Then** 클라이언트는 `GET /api/dashboards/mine` 으로 `404 Not Found` 를 받는다
  (bob 의 snapshot 은 아직 없음)
- **And** bob 의 메모리 상태는 빌트인 `DEFAULT_DASHBOARD_PAGE` 로 초기화된다
- **When** bob 이 자신의 페이지를 추가한다
- **Then** `PUT /api/dashboards/mine` 으로 `{ owner: 'bob', version: 1 }` 이 생성되며
  alice 의 snapshot 은 변경되지 않는다

### AC-6: owner spoofing 시도 차단 (UB-003)

- **Given** alice 로 로그인된 사용자가 `PUT /api/dashboards/mine` body 에 명시적으로
  `{ scope: 'user', owner: 'bob', payload: {...} }` 를 포함하여 호출한다
- **When** 서버가 요청을 처리한다
- **Then** 서버는 body 의 `owner` 를 무시하고 JWT `Claims.Username='alice'` 로 owner 를
  결정한다
- **And** 결과적으로 `owner='alice'` 의 snapshot 이 갱신된다
- **And** bob 의 snapshot 은 변경되지 않는다

### AC-7: URL vs body scope 불일치 → 400 Bad Request (UB-006)

- **Given** alice 로 로그인된 사용자가 `PUT /api/dashboards/mine` 을 호출하면서 body
  에 `{ scope: 'global', payload: {...} }` 를 포함한다
- **When** 서버가 요청을 처리한다
- **Then** 서버는 `400 Bad Request` 를 반환한다 (silent normalize 금지)
- **And** SQLite 의 어떤 row 도 변경되지 않는다

### AC-8: 비인증 요청 거부 (UR-004, IF auth → 401)

- **Given** xflowd 가 `basic_auth.enabled=true` (강제) 로 실행 중이다
- **When** JWT 헤더 없이 `GET /api/dashboards/shared` 를 호출한다
- **Then** 서버는 `401 Unauthorized` 를 반환한다
- **When** 만료된 JWT 로 `PUT /api/dashboards/mine` 을 호출한다
- **Then** 서버는 `401 Unauthorized` 를 반환하고 SQLite 는 변경되지 않는다

### AC-9: basic_auth 비활성화 시 부팅 거부 (또는 강제 활성화) — IF basic_auth=false

- **Given** `serverCfg.BasicAuth.Enabled=false` 로 설정된 환경에서 xflowd 가 부팅된다
- **When** 부팅 절차가 ASM-003 (basic_auth 필수) 을 평가한다
- **Then** 구현 선택에 따라 다음 중 하나가 발생한다:
  - (권장) 부팅이 거부되고 에러 로그 "auth: basic_auth 는 필수입니다 (SPEC-DASHBOARD-001
    v0.2.0)" 가 출력되며 프로세스가 비정상 종료된다
  - 또는 basic_auth 가 강제로 `true` 로 활성화되고 경고 로그 "auth: basic_auth 가 자동
    활성화되었습니다" 가 출력된다
- **And** 어떤 경우에도 비인증 상태로 `/api/dashboards/*` 가 노출되지 않는다

### AC-10: 동시 편집 충돌 — last-write-wins (공유 또는 동일 사용자)

- **Given** 공유 snapshot 이 `version=5` 로 저장되어 있다
- **And** admin 두 명(또는 같은 admin 의 두 브라우저) 이 모두 `version=5` 를 받아
  들고 있다
- **When** 브라우저 A 가 페이지를 변경하여 `PUT /api/dashboards/shared` (If-Match: 5)
  를 호출한다
- **Then** 서버는 `200 OK` 와 `version=6` 을 반환한다
- **When** 브라우저 B 가 약간 후 다른 페이지를 변경하여 `PUT (If-Match: 5)` 를 호출
  한다
- **Then** 서버는 `409 Conflict` 와 함께 서버측 최신 snapshot(`version=6`) 을 반환
  한다
- **And** 브라우저 B 의 클라이언트는 응답의 server snapshot 을 store 에 적용한다
- **And** 브라우저 B 는 자신의 변경을 `PUT (If-Match: 6)` 으로 1회 재시도한다
- **Then** 서버는 `200 OK` 와 `version=7` 을 반환한다 (브라우저 B 의 변경이 살아남음)
- **And** 두 번째 409 발생 시에는 토스트로 알리고 server snapshot 으로 강제 동기화한다

### AC-11: 잘못된 페이로드 거부 (UR-003)

- **Given** 클라이언트가 잘못된 JSON (예: `dashboardPages` 가 배열이 아닌 객체) 을
  PUT 한다
- **When** `PUT /api/dashboards/mine` 을 호출한다
- **Then** 서버는 `400 Bad Request` 를 반환하고 SQLite 는 변경되지 않는다
- **And** 응답 body 에 에러 메시지가 포함된다

### AC-12: 페이로드 크기 초과 거부 (UR-003)

- **Given** 클라이언트가 256 KB 를 초과하는 페이로드를 PUT 한다
- **When** `PUT /api/dashboards/mine` 또는 `PUT /api/dashboards/shared` 를 호출한다
- **Then** 서버는 `413 Payload Too Large` 를 반환한다

### AC-13: 그리드 설정도 함께 영속화 (scope 별로 분리)

- **Given** admin 이 공유 탭에서 `dashboardGridCols=12`,
  `dashboardShowGridLines=false` 로 변경한다
- **When** 다른 admin 이 다른 브라우저에서 공유 탭을 본다
- **Then** 동일한 그리드 설정이 적용된다
- **And** editor 가 본 인 개인 탭에서 다른 그리드 설정(`dashboardGridCols=6`)을 사용
  하고 있다면 그 설정은 영향받지 않는다 (scope 별 독립)

### AC-14: 환경설정은 기기별 (서버 영속 제외) — 유지

- **Given** alice 가 브라우저 A 에서 `theme=dark`, `sidebarCollapsed=true` 를 설정
  한다
- **When** alice 가 브라우저 B 에서 접속한다
- **Then** 브라우저 B 의 테마/사이드바 상태는 브라우저 B 의 `localStorage` 값을 따른다
  (서버 미영속)
- **And** `/api/dashboards/*` 응답의 어떤 페이로드에도 `theme`, `sidebarCollapsed`,
  `customThemeTokens` 가 포함되지 않는다

### AC-15: localStorage 블랭크 슬레이트 — v0.2 첫 부팅 시 1회 제거 + 토스트 1회

- **Given** v0.1.x 클라이언트에서 사용되던 `localStorage.xflow-ui` 가 존재하며 그 안에
  `dashboardPages`, `activeDashboardId`, `dashboardGridCols`,
  `dashboardShowGridLines`, `dashboardRefreshInterval`, `deviceGridLayout` 키가 채워져
  있다
- **And** `localStorage.xflow-ui:dashboard-migrated-v0.2` 플래그가 존재하지 않는다
- **When** 사용자가 v0.2.0 클라이언트로 페이지를 처음 연다
- **Then** 클라이언트는 위 6개 키를 `localStorage` 에서 명시적으로 제거한다
  (서버로 PUT 하지 않음)
- **And** 사용자에게 토스트 "이번 업데이트(v0.2.0)로 대시보드 구성이 서버 저장으로
  전환되었습니다. 기존 로컬 구성은 초기화됩니다. 공유 대시보드는 관리자가 다시 구성해
  주세요." 가 1회 노출된다
- **And** `localStorage.xflow-ui:dashboard-migrated-v0.2` 플래그가 set 된다
- **When** 사용자가 페이지를 새로고침 한다
- **Then** 토스트는 다시 노출되지 않는다 (1회 한정)
- **And** `localStorage.xflow-ui` 의 `theme`, `sidebarCollapsed`, `customThemeTokens`
  는 그대로 유지된다

### AC-16: GET 404 → 빌트인 기본값 → 첫 변경 시 자동 PUT 으로 snapshot 최초 생성

- **Given** alice 가 처음 로그인했고 `dashboards` 테이블에 alice 의 row 가 없다
- **When** 클라이언트가 `GET /api/dashboards/mine` 을 호출한다
- **Then** `404 Not Found` 를 받고 빌트인 `DEFAULT_DASHBOARD_PAGE` 로 메모리 상태가
  초기화된다
- **When** alice 가 패널 하나를 이동한다
- **Then** 500ms 이내에 클라이언트가 `PUT /api/dashboards/mine` (If-Match 없음, 최초
  생성) 을 호출한다
- **And** 서버는 `200 OK` 와 `{ owner: 'alice', version: 1 }` 을 반환하고 row 가 최초
  생성된다

### AC-17: DELETE 동작 (관리/리셋용)

- **Given** alice 가 자신의 개인 대시보드를 가지고 있다 (`version=3`)
- **When** alice 가 `DELETE /api/dashboards/mine` 을 호출한다
- **Then** 서버는 `204 No Content` 를 반환하고 SQLite 에서 alice 의 row 가 삭제된다
- **When** 이후 `GET /api/dashboards/mine` 을 호출한다
- **Then** `404 Not Found` 가 반환된다
- **And** bob 의 row, 공유 row 는 영향받지 않는다
- **And** editor 가 `DELETE /api/dashboards/shared` 를 호출하면 `403 Forbidden` 이
  반환된다 (admin only)

---

## 품질 게이트 기준 (TRUST 5)

### Tested

- 백엔드:
  - `internal/storage/dashboard_sqlite_test.go` — Get/Put/Delete, version monotonic,
    If-Match 충돌, partial unique index 동작 (global+NULL vs user+username 두 row 가
    공존 가능), cross-user 격리.
  - `internal/api/handler/dashboard_test.go` — 6 endpoint × happy path, 400 (scope
    mismatch / invalid JSON), 401, 403 (editor → shared PUT), 404, 409, 413 (oversize),
    owner spoofing 차단, cross-user 차단.
  - `internal/auth/credentials_migration_test.go` — yaml 존재 시 1회 이관 + rename
    검증, 이관 후 인증 정상 동작, yaml 없을 때 admin/admin 생성, 이미 이관된 환경에서
    no-op 검증.
  - 커버리지 85% 이상.
- 프론트엔드:
  - `web/src/hooks/useDashboardSync.test.ts` (Vitest) — boot 병렬 GET, 404 fallback,
    409 재PUT (1회), 403 토스트, blank slate (localStorage 제거 + 토스트 1회).
  - `web/src/stores/uiStore.test.ts` — partialize 축소 검증, 마이그레이션 플래그 동작.
- E2E: AC-3, AC-4, AC-5, AC-10, AC-15 중 핵심 3개는 Playwright 또는 수동 검증.

### Readable

- 모든 Go exported 함수에 doc comment.
- TS public 함수에 JSDoc.
- 한글/영문 혼용 주석 허용 (기존 코드베이스 컨벤션).

### Unified

- gofmt + goimports + golangci-lint 통과.
- prettier + biome (또는 eslint) 통과.

### Secured

- basic_auth 강제 활성화 (AC-8, AC-9) 검증.
- editor → shared PUT 403 (AC-4) 검증.
- owner spoofing 차단 (AC-6) 검증.
- cross-user 격리 (AC-5) 검증.
- 페이로드 크기 한도 256KB → 413 (AC-12) 검증.
- yaml.migrated 잔류 시에도 더 이상 인증에 참조되지 않음 검증 (UB-007).
- bcrypt 비밀번호 해시는 SQLite `password_hash` 컬럼에 저장 (기존 동작 유지).
- JSON unmarshal 시 unknown 필드 무시 (DoS 방어).

### Trackable

- 모든 변경 commit message: `feat(dashboard,auth,api): SPEC-DASHBOARD-001 v0.2.0 …`
  패턴.
- 핵심 로그 (Info 레벨): PUT 성공 시 `dashboard snapshot saved scope=global|user
  owner=<username> version=N`.
- 마이그레이션 로그: `auth: migrated N users from yaml to sqlite`.
- 에러 로그 (Warn/Error): 401/403/409/413/500 시 사유 포함.

---

## 검증 방법 및 도구

| 단계         | 도구                        | 산출물                                                                |
| ------------ | --------------------------- | --------------------------------------------------------------------- |
| Unit (Go)    | `go test -race ./...`       | 85%+ 커버리지, race condition 없음, SQLite 트랜잭션 정상 동작         |
| Unit (TS)    | `vitest run`                | hook 시나리오 통과 (boot/404/409/403/blank-slate)                     |
| Integration  | `go test -tags=integration` | 실제 SQLite (`xflow.db`) + HTTP 핸들러 e2e, yaml 이관 시뮬레이션      |
| Manual       | 두 브라우저 + 다른 사용자   | AC-3, AC-4, AC-5, AC-10, AC-15 수동 검증                              |
| Lint         | `golangci-lint`, `biome`    | 0 warning                                                             |
| API spec     | curl + jq                   | `GET/PUT/DELETE /api/dashboards/{shared,mine}` 응답 schema 확인       |
| DB schema    | `sqlite3 ~/.xflow/xflow.db .schema` | `dashboards`, `users` 테이블 및 partial unique index 존재 확인 |

---

## Definition of Done

- [ ] M-1 ~ M-15 마일스톤 모두 완료
- [ ] AC-1 ~ AC-17 시나리오 모두 통과
- [ ] TRUST 5 게이트 모두 PASS
- [ ] CHANGELOG v0.2.0 진입 (사용자 안내 문구 포함)
- [ ] README 의 인증 섹션에 "basic_auth 필수" 명시
- [ ] 기존 SPEC (`SPEC-WEB-005`, `SPEC-CHART-001`) 의 패널 schema 영향 없음 확인
- [ ] Open Issues 상태 확정:
  - [ ] OI-001 CLOSED (공유+개인 병행 채택)
  - [ ] OI-002 유지 (deferred — WebSocket)
  - [ ] OI-003 CLOSED (SQLite 채택)
  - [ ] OI-004 신규 (SPEC-USER-MGMT-001 분리)
  - [ ] OI-005 신규 (개인 대시보드 link share 검토)
- [ ] yaml → SQLite 이관 동작 후 `users.yaml.migrated` 존재 및 yaml 미참조 확인
- [ ] v0.2 첫 부팅 시 `localStorage.xflow-ui` 의 대시보드 키 6개 1회 제거 + 토스트 1회
  노출 확인
- [ ] `internal/storage/dashboard_file.go` 가 작성되지 않았음을 PR 에서 확인 (0.1.0
  결정 폐기)
