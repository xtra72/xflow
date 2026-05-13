# Sync Report — SPEC-AUTH-004

| 항목 | 값 |
|------|------|
| **동기화 모드** | auto |
| **대상 SPEC** | SPEC-AUTH-004 |
| **SPEC 제목** | REST 로그인 응답 스키마 정합 및 토큰 보존 자가 회복 |
| **Lifecycle Level** | spec-first |
| **실행 일자** | 2026-05-12 |
| **소스 commit** | `8bf49e0` |
| **브랜치** | `feature/SPEC-DASHBOARD-001` |
| **Phase** | Phase 2 (manager-docs) — Phase 3 (manager-git) 대기 중 |

---

## 1. 처리된 파일 목록

### 1.1 변경된 파일 (Modified)

| 경로 | 변경 내용 | 라인 변화 (대략) |
|------|-----------|-------------------|
| `.moai/specs/SPEC-AUTH-004/spec.md` | frontmatter `status: draft → completed`, `completed: 2026-05-12` 추가, §9 Implementation Notes 신규 섹션 추가 (약 90 라인) | +93 / -1 |
| `CHANGELOG.md` | `[Unreleased]` 의 최상단에 `### 수정 (Fixed)` 신규 서브섹션 추가 (SPEC-AUTH-004 단일 엔트리) | +5 / 0 |

### 1.2 신규 생성된 파일 (Added)

| 경로 | 용도 |
|------|------|
| `.moai/reports/sync-report-2026-05-12-spec-auth-004.md` | 본 sync 리포트 |

### 1.3 스킵된 파일 (Skipped)

| 경로 / 디렉터리 | 사유 |
|------------------|------|
| `README.md` | `## 인증 (Authentication)` 섹션이 존재하나 login response 스키마는 문서화되어 있지 않음 (basic_auth 설정 / 자격증명 저장소 / 역할만 다룸). 새로 발명하지 않는다는 strict scope rule 에 따라 SKIP. |
| `docs/api/` | 현재 `dashboards.md` 단일 파일만 존재. auth login endpoint 를 문서화하는 파일 부재 → 신규 doc 파일 생성은 strict scope rule 에 의해 금지되어 SKIP. |
| `.moai/project/architecture.md` | divergence 분석상 `new_architectural_patterns=0` (Zustand store API 무변경, Echo handler 인터페이스 무변경). 본 SPEC 은 defect fix 이므로 SKIP. |
| `.moai/project/product.md` | 사용자 가시 제품 기능 변경 없음 — 결함 수정만 수행. SKIP. |
| `.moai/project/structure.md` | `new_directories=0`, `new_dependencies=0`. 구조 변경 없음 → SKIP. |
| `.moai/project/tech.md` | `go.mod` / `web/package.json` 무변경 (신규 dependency 0건). 기술 스택 변경 없음 → SKIP. |

---

## 2. SPEC 상태 전이

| 필드 | Before | After |
|------|--------|-------|
| `status` | `draft` | `completed` |
| `completed` | (부재) | `2026-05-12` |
| `updated` | `2026-05-12` | `2026-05-12` (유지) |
| `version` | `0.1.0` | `0.1.0` (유지) |

전이 근거: 모든 자동화 가능 acceptance 시나리오(AC-1/2/5/6/7) GREEN, TRUST 5 PASS, LSP 회귀 0건. 잔여 수동 게이트(AC-3, AC-4)는 main 머지 이전 사용자 검증 대상으로 명시화되어 별도 트래킹.

---

## 3. CHANGELOG 항목 배치

- **위치**: `## [Unreleased]` 의 최상단 (기존 `### 변경 (BREAKING)` 섹션 바로 위)
- **새로 추가된 서브섹션**: `### 수정 (Fixed)`
- **엔트리 수**: 1건 (SPEC-AUTH-004)
- **배치 근거**: Keep a Changelog 규약상 "Fixed" 카테고리가 동일 release 내 "Changed (BREAKING)" 와 별개 분류이며, defect fix 의 시각적 강조를 위해 [Unreleased] 의 최상단에 배치.

---

## 4. Divergence 요약 (Phase 1.5 입력)

| 지표 | 계획 (plan.md §7) | 실측 | 일치 여부 |
|------|--------------------|------|------------|
| 영향 파일 수 | 6 | 6 | PASS |
| 추가 features | 0 | 0 | PASS |
| 신규 dependencies | 0 | 0 | PASS |
| 신규 directories | 0 | 0 | PASS |
| 신규 architectural patterns | 0 | 0 | PASS |

### 4.1 Scope Changes (2건)

| # | 변경 내용 | 분류 | 근거 |
|---|-----------|------|------|
| 1 | `internal/api/handler/auth_test.go` 의 sibling 테스트 4건 (`TestAuthHandler_Me`, `TestAuthHandler_Logout`, `TestAuthHandler_Refresh`, `TestAuthHandler_ChangePassword`) 에서 `loginResp.Data.AccessToken` → `loginResp.Data.Tokens.AccessToken` 으로 mechanical migration. | behavior preserving / compile recovery | LoginResponse DTO 가 nested `{user, tokens}` 형태로 변경됨에 따른 불가피한 영향. 테스트 의도/검증 대상 보존. |
| 2 | `web/src/stores/authStore.ts` 에 `export const __test__ = { saveTokens, loadTokens, TOKENS_STORAGE_KEY }` 테스트 시임 추가. | guideline-compliant | `@internal` JSDoc 어노테이션으로 production 사용 금지 표기. plan.md M-6 의 "내부 헬퍼 단위 테스트가 필요하면 `@internal` + `__test__` 네임스페이스" 가이드 준수. |

두 scope change 는 모두 spec.md §9.2 Implementation Notes 에 명시화 완료.

---

## 5. 품질 검증 요약 (Phase 2.5 입력)

| 게이트 | 결과 | 비고 |
|--------|------|------|
| TRUST 5 — Tested | PASS | 906 tests GREEN (server + client) |
| TRUST 5 — Readable | PASS | 명확한 네이밍, English 주석 |
| TRUST 5 — Unified | PASS | gofmt / eslint / biome 0 issues |
| TRUST 5 — Secured | PASS | R-6 보안 게이트: console 내 raw 토큰 누출 0건 |
| TRUST 5 — Trackable | PASS | conventional commit `8bf49e0`, SPEC reference 포함 |
| LSP 회귀 | 0건 | `go vet` / `gofmt` / `tsc --noEmit` / `eslint` 신규 진단 0 |
| Test coverage | 충족 | 27 신규/갱신 테스트 (16 server + 11 client) |
| AC-1 (자동) | GREEN | 로그인 응답 `{user, tokens}` 구조 검증 |
| AC-2 (자동) | GREEN | 클라이언트 상태 전이 및 localStorage 직렬화 검증 |
| AC-5 (자동) | GREEN | UB1 saveTokens falsy 차단 검증 |
| AC-6 (자동) | GREEN | UB2 loadTokens broken state 자가 회복 검증 |
| AC-7 (자동) | GREEN | RefreshResponse 비대칭 보존 회귀 방지 |

---

## 6. 수동 검증 게이트 (main 머지 이전 필수)

Phase 3 (manager-git) 의 PR 머지 이전에 다음 두 시나리오를 **사용자가 직접 브라우저 환경에서** 검증해야 한다. 두 시나리오 모두 PASS 하기 전까지 main 머지를 보류한다.

### 6.1 AC-3 — 페이지 새로고침 후 인증 상태 복원

1. 브라우저 개발자 도구 열기 (`Cmd+Option+I` / `F12`).
2. Application 탭 → Local Storage → 현재 origin 선택 → 모든 키 삭제 (clean state 시작).
3. xflow 웹 UI 접속 → admin 계정으로 로그인.
4. 로그인 직후 콘솔에 토큰 누출 로그(`access_token`, `refresh_token` 의 raw 값) 가 없는지 확인.
5. `localStorage.xflow_auth_tokens` 가 유효 JSON 으로 저장되었는지 확인 (literal `"undefined"` 가 아님).
6. **페이지 새로고침** (`Cmd+R` / `F5`).
7. 재로그인 요구 없이 동일한 인증 상태가 복원되는지 확인 (`isAuthenticated === true`, 사용자 메뉴 표시).

**PASS 조건**: 새로고침 후 자동 인증 복원 + console 에러 0건.

### 6.2 AC-4 — SPEC-AUTH-003 통합 WS 회귀

1. 6.1 의 1~5 단계 수행 후 진행.
2. WebSocket 으로 데이터를 수신하는 페이지(예: 대시보드의 실시간 위젯) 로 이동.
3. 브라우저 개발자 도구 Network 탭 → WS 필터.
4. WS 연결이 성립하고 `Authorization: Bearer <jwt>` 헤더(혹은 query param `token=<jwt>`) 가 첨부되었는지 확인.
5. WS 가 1006 무한 재시도 없이 정상 유지되는지 확인 (SPEC-AUTH-003 의 회귀 방지).

**PASS 조건**: WS 정상 연결 + 토큰 동반 + 1006 재시도 0회.

---

## 7. 다음 단계 안내

### 7.1 Phase 3 (manager-git) 에 전달할 commit guidance

본 sync 단계의 산출물(spec.md, CHANGELOG.md, sync report) 은 별도 commit 으로 생성하되, commit 메시지는 다음 형식을 권장한다:

```
docs: SPEC-AUTH-004 — sync (spec status: completed, CHANGELOG fix entry)
```

본 commit 은 source code 변경 없이 문서만 갱신하므로 LSP 회귀 게이트는 자동 PASS. 별도 SPEC 변경이 없어 PR 머지 대상 commit 의 위치는 `8bf49e0` 직후.

### 7.2 사용자 액션 아이템

1. **수동 검증 게이트 통과**: 위 §6.1 (AC-3) 및 §6.2 (AC-4) 를 직접 수행하고 결과를 기록.
2. **수동 검증 PASS 후 main 머지**: 두 게이트 모두 PASS 한 경우에만 `feature/SPEC-DASHBOARD-001` → `main` PR 머지 진행.
3. **재로그인 안내**: 본 패치 머지 후 모든 활성 사용자는 invalidation 되며 재로그인이 필요함을 운영 채널에 사전 공지.

---

## 8. 메타데이터

- **Manager**: manager-docs (Phase 2)
- **Successor**: manager-git (Phase 3) — commit 작업 대기
- **Predecessor SPEC**: SPEC-AUTH-003 (`9ec8597`, `dad6010`)
- **Related SPECs**: SPEC-DASHBOARD-001 (`6b37c98`, `2efa614`), SPEC-AUTH-002 (`8635e1f`)
- **Token budget 소비량 (Phase 2)**: 본 동기화는 4개 파일 변경 + 1개 신규 + 5개 SKIP 으로 구성, 40K 토큰 budget 내 처리.
