---
spec_id: SPEC-AUTH-004
acceptance_version: 0.1.0
created: 2026-05-12
updated: 2026-05-12
author: xtra
---

## 1. 개요

본 문서는 SPEC-AUTH-004 의 6개 EARS 모듈(U1, U2, S1, UB1, UB2, O1) 및 회귀 방지 항목에 대한 7개의 Given/When/Then 시나리오와 품질 게이트, 수동 검증 절차를 정의한다. 각 시나리오는 plan 의 마일스톤·테스트 항목과 트레이서빌리티가 일치한다.

---

## 2. 시나리오

### AC-1 — 정상 로그인 → `{user, tokens}` 응답 + 스토어 정합 + localStorage 유효 JSON

- **EARS 매핑**: U1, U2
- **Given**:
  - 서버가 `basic_auth: true` 로 기동, 사용자 `admin/admin1234` (또는 환경 기본 자격증명) 가 인증 가능 상태.
  - 클라이언트 `useAuthStore` 가 `{user: null, tokens: null, isAuthenticated: false, authEnabled: true}` 상태.
  - 모킹된 `localStorage` 가 빈 상태로 초기화됨 (jsdom 기본).
- **When**:
  - `useAuth.login('admin', 'admin1234')` 호출.
  - HTTP 응답 200 수신 및 envelope 언래핑 완료.
- **Then**:
  - 서버 응답 body (envelope 언래핑 후 `data`) 의 최상위 키가 정확히 `user` 와 `tokens` 두 개.
  - `data.user.username === 'admin'`, `data.user.role` 가 비어있지 않은 문자열이며 알려진 `UserRole` 값(`admin` / `editor` / `viewer`) 중 하나.
  - `data.tokens.access_token` 비어있지 않은 문자열, `data.tokens.refresh_token` 비어있지 않은 문자열, `data.tokens.expires_at > 0`, `data.tokens.token_type === 'Bearer'`.
  - `useAuthStore.getState()` 가 다음 불변식 만족:
    - `user !== null`, `user.name === 'admin'`, `user.role` 유효.
    - `tokens !== null`, `tokens.access_token` 비어있지 않음, `tokens.refresh_token` 비어있지 않음, `tokens.expires_at > 0`.
    - `isAuthenticated === true`, `isLoading === false`.
  - `localStorage.getItem('xflow_auth_tokens')` 가 비어있지 않은 문자열.
  - 해당 문자열을 `JSON.parse()` 한 결과가 `tokens` 와 deep-equal (단, `token_type` 필드는 클라이언트가 보관하지 않으므로 비교 대상에서 제외).
- **And (음성 검증)**:
  - localStorage 값이 literal `"undefined"` 또는 `"null"` 이 **아님**.

### AC-2 — 페이지 새로고침 후 인증 상태 복원 + Authorization 헤더 자동 첨부

- **EARS 매핑**: S1, U2 (간접)
- **Given**:
  - AC-1 의 종료 상태 (정상 로그인 후 localStorage 에 유효 토큰 JSON 저장).
  - 인메모리 `useAuthStore` 상태를 reset (페이지 새로고침 시뮬레이션 — Vitest 환경에서는 store 재생성).
- **When**:
  - `useAuthStore.getState().initialize()` 호출.
  - 내부적으로 `getAuthStatus()` → `loadTokens()` → `getCurrentUser()` 순차 실행.
- **Then**:
  - `initialize()` resolve 후 `useAuthStore.getState()` 가 AC-1 의 종료 상태와 동일한 불변식 만족 (`user`, `tokens`, `isAuthenticated: true`).
  - localStorage 의 `xflow_auth_tokens` 값 미변경 (clean restoration).
- **And (Authorization 헤더 자동 첨부 검증)**:
  - 임의 인증 API 호출 (예: `apiClient.get('/agents?detail=summary')`) 시 request interceptor 가 `Authorization: Bearer <token>` 헤더를 첨부 (interceptor 의 `useAuthStore.getState().tokens?.access_token` 평가 결과가 truthy).

### AC-3 — 토큰 보유 상태에서 인증 API 호출 200 반환

- **EARS 매핑**: U1 (E2E), S1 (E2E)
- **Given**:
  - AC-1 또는 AC-2 의 종료 상태 (`isAuthenticated: true`, `tokens.access_token` 유효).
  - 서버가 정상 기동, `GET /api/v1/agents?detail=summary` endpoint 가 인증된 사용자에게 200 응답.
- **When**:
  - 클라이언트가 `apiClient.get('/agents?detail=summary')` 호출.
- **Then**:
  - HTTP 응답 status 200.
  - request 의 `Authorization` 헤더가 `Bearer <access_token>` 형태로 전송됨.
  - 응답 envelope (`{success: true, data: [...]}`) 정상 파싱.
- **And (회귀 방지)**:
  - 본 SPEC 적용 전(broken state)에는 동일 호출이 401 응답을 받음 (regression validation 의 부정 케이스 검증).

### AC-4 — SPEC-AUTH-003 수동 검증 §4 Step 2 통합 회귀 PASS

- **EARS 매핑**: 통합 회귀 (SPEC-AUTH-003 + SPEC-AUTH-004 합산 검증)
- **Given**:
  - 서버: `basic_auth: true` 활성화.
  - 클라이언트: SPEC-AUTH-003 + SPEC-AUTH-004 양쪽 변경 모두 적용됨.
  - 브라우저 localStorage 클린 상태로 시작.
- **When**:
  - 정상 자격증명으로 로그인 수행.
  - DevTools Network → WS 탭 관찰.
- **Then**:
  - 로그인 응답 body 가 `{user, tokens}` 형태 (Network → XHR → `/auth/login` response 탭에서 확인).
  - 로그인 직후 단일 `/ws?token=<jwt>` 요청 발생.
  - WS 요청 응답이 `101 Switching Protocols`.
  - 이후 WS frame inspector 에서 메시지가 정상 송수신됨.
- **And (불합격 신호 부재 검증)**:
  - `/ws` 요청에 `?token=` 누락된 사례 0건.
  - WS 401 응답 후 무한 재시도 0건.
  - 임의 인증 REST API 호출이 401 로 거절되는 사례 0건.

### AC-5 — saveTokens 의 falsy 차단 ([UB1] 직접 검증)

- **EARS 매핑**: UB1
- **Given**:
  - `useAuthStore` 가 임의의 정상 상태 (또는 초기 상태).
  - `localStorage` 의 `xflow_auth_tokens` 키가 비어있는 상태.
  - `console.warn` 을 `vi.spyOn` 으로 감시.
- **When (시나리오 5-A — `saveTokens` 직접 invoke)**:
  - `useAuthStore.getState()` 에 노출된 `saveTokens` (또는 store 내부 헬퍼 접근 가능한 경우) 를 `undefined`, `null`, `{}`, `{access_token: '', refresh_token: '', expires_at: 0}` 각각으로 호출.
- **Then (5-A)**:
  - 모든 호출에서 `localStorage.getItem('xflow_auth_tokens')` 결과 변경 없음 (`null` 유지).
  - 각 호출당 `console.warn` 이 정확히 1회 호출됨.
- **When (시나리오 5-B — `login` 함수의 가드 검증)**:
  - `useAuthStore.getState().login(undefined as any, undefined as any)` 호출.
- **Then (5-B)**:
  - 호출 전후 store 상태 동일 (`isAuthenticated` 가 이전 상태 유지).
  - `localStorage` 의 `xflow_auth_tokens` 키 변경 없음.
  - `console.error` 또는 `console.warn` 이 1회 발생 (가드 발동 메시지).
- **And (음성 검증 — 보안)**:
  - 모든 console 출력에 토큰 시크릿 (예: 실제 JWT 문자열) 이 포함되지 않음.

### AC-6 — loadTokens 의 broken state 자가 회복 ([UB2] 직접 검증)

- **EARS 매핑**: UB2
- **Given**:
  - `console.warn` 을 `vi.spyOn` 으로 감시.
- **When (시나리오 6-A — literal `"undefined"`)**:
  - `localStorage.setItem('xflow_auth_tokens', 'undefined')` 사전 설치.
  - `loadTokens()` 호출 (store 의 `initialize()` 내부 호출 또는 직접 invoke).
- **Then (6-A)**:
  - 반환값 `null`.
  - `localStorage.getItem('xflow_auth_tokens') === null` (자동 클린업 발동).
  - `console.warn` 1회 호출.
- **When (시나리오 6-B — literal `"null"`)**:
  - `localStorage.setItem('xflow_auth_tokens', 'null')` 사전 설치.
  - `loadTokens()` 호출.
- **Then (6-B)**:
  - 동일하게 `null` 반환 + 키 자동 삭제 + console.warn 1회.
- **When (시나리오 6-C — malformed JSON)**:
  - `localStorage.setItem('xflow_auth_tokens', '{not json')` 사전 설치.
  - `loadTokens()` 호출.
- **Then (6-C)**:
  - `null` 반환 + 키 자동 삭제 + console.warn 1회.
- **When (시나리오 6-D — 구조 불일치 JSON)**:
  - `localStorage.setItem('xflow_auth_tokens', JSON.stringify({foo: 'bar', baz: 42}))` 사전 설치.
  - `loadTokens()` 호출.
- **Then (6-D)**:
  - `null` 반환 + 키 자동 삭제 + console.warn 1회.
- **When (시나리오 6-E — 정상 JSON 정상 구조)**:
  - `localStorage.setItem('xflow_auth_tokens', JSON.stringify({access_token: 'A', refresh_token: 'R', expires_at: 9999999999}))` 사전 설치.
  - `loadTokens()` 호출.
- **Then (6-E)**:
  - 반환값이 사전 설치된 객체와 deep-equal.
  - `localStorage` 키 미변경.
  - `console.warn` 호출 0회.
- **And (음성 검증 — 보안)**:
  - 모든 console.warn 메시지에 raw 토큰 값이 포함되지 않음 (메시지에 raw 문자열이 포함된 경우 최대 16자 prefix 만 노출).

### AC-7 — RefreshResponse 비대칭 보존 ([O1] 검증)

- **EARS 매핑**: O1
- **Given**:
  - AC-1 또는 AC-2 의 종료 상태 (로그인 완료, `tokens.refresh_token` 유효).
  - 서버 `POST /api/v1/auth/refresh` endpoint 가 정상 동작.
- **When**:
  - 직접 호출: `apiClient.post('/auth/refresh', {refresh_token: <기존 refresh_token>})`.
  - HTTP 응답 200 수신 및 envelope 언래핑 완료.
- **Then**:
  - 응답 body (envelope 언래핑 후 `data`) 의 최상위 키가 정확히 4개: `access_token`, `refresh_token`, `expires_at`, `token_type` (flat 구조).
  - `data.user` 키가 **존재하지 않음** (login 응답과의 비대칭 유지 확인).
  - `data.tokens` 키가 **존재하지 않음** (login 응답과의 비대칭 유지 확인).
  - `data.access_token` 비어있지 않은 문자열, `data.refresh_token` 비어있지 않은 문자열, `data.expires_at > 0`, `data.token_type === 'Bearer'`.
- **And (Go server 단위 테스트)**:
  - `TestRefresh_ResponseShape` (M-3 신규 회귀 테스트) 가 GREEN — JSON 마샬링 결과의 최상위 키가 정확히 4개임을 검증.

---

## 3. 품질 게이트 기준

### 3.1 TRUST 5 컴플라이언스

| 축 | 기준 | 검증 방법 |
|----|------|-----------|
| **T**ested | 신규 코드 라인 커버리지 ≥ 85%, 모든 acceptance 시나리오 GREEN | `go test -cover ./internal/api/handler/...`, `pnpm test --coverage` (`web/`), CI 리포트 |
| **R**eadable | 신규 함수/변수명이 의도를 명확히 표현 (예: `TokenPair`, `saveTokensGuard`, `loadTokensSelfHeal`), 한국어 주석으로 비자명 로직 설명 | 코드 리뷰 + ESLint readability rules + golangci-lint |
| **U**nified | `gofmt -l internal/` 결과 빈 출력, `go vet ./...` 0 error, `pnpm lint` 신규 error 0건, `pnpm tsc --noEmit` 0 error | CI lint/typecheck job |
| **S**ecured | 토큰 값이 console 또는 에러 메시지에 노출되지 않음, JWT 시크릿 미노출, 서버 응답에 비밀번호 해시 미포함 | 코드 리뷰 (grep `access_token` in console/throw), 보안 수동 검증 |
| **T**rackable | commit 메시지에 `SPEC-AUTH-004` 참조, 본 SPEC 트레이서빌리티 표 일치 | git log 검사 |

### 3.2 회귀 방지 기준

- 기존 `internal/`, `web/` 테스트 스위트 (변경 전 GREEN 상태) → **변경 후에도 0건 실패**.
- LSP baseline (`.moai/specs/SPEC-AUTH-004/lsp-baseline.json`) 대비:
  - 신규 Go vet errors: **0**
  - 신규 gofmt 위반: **0**
  - 신규 TypeScript errors: **0**
  - 신규 type errors: **0**
  - 신규 ESLint errors: **0**
  - ESLint warnings 증가: **≤ 0** (증가 금지)
- SPEC-AUTH-003 의 자동/수동 검증 회귀: 0건.

### 3.3 Definition of Done (acceptance 수준)

- AC-1 ~ AC-7 모두 자동화 테스트로 GREEN (단, AC-3 의 일부 및 AC-4 는 통합/수동 검증 포함 가능).
- TRUST 5 모든 축 통과.
- 회귀 방지 기준 모두 통과.
- §4 수동 검증 절차 모든 단계 PASS.

---

## 4. 수동 검증 절차 (`basic_auth: true` 환경)

CI 자동화 외에 실제 브라우저에서 결함 회복을 확인하는 단계별 절차. 검증자는 DevTools 의 **Network** 탭 (필터: `XHR` + `WS`) 과 **Application** 탭(localStorage 인스펙터), **Console** 탭을 동시에 열어둔다.

### 4.1 사전 준비

- 서버 설정: `basic_auth: true` 활성화. 본 플래그가 `false` 면 결함 자체가 표면화되지 않으므로 반드시 `true` 인 환경 사용.
- 클라이언트: SPEC-AUTH-003 + SPEC-AUTH-004 양쪽 변경 모두 빌드/배포 완료.
- 브라우저: Chromium 계열 권장 (DevTools Application 탭의 localStorage 인스펙터 사용).
- 사전 정리: 브라우저 storage 초기화 (DevTools → Application → Clear storage) — 깨끗한 미인증 상태에서 시작.

### 4.2 단계별 검증

#### Step 1 — 본 SPEC 적용 전 broken state 시뮬레이션 → 자가 회복 검증

1. DevTools Console 에서 다음 실행:
   ```js
   localStorage.setItem('xflow_auth_tokens', 'undefined');
   ```
2. 페이지 새로고침.
3. DevTools Application → Local Storage 관찰:
   - **기대**: `xflow_auth_tokens` 키가 자동으로 삭제됨 ([UB2] 자가 회복 발동).
   - **불합격 신호**: literal `"undefined"` 가 잔존 → UB2 위반.
4. DevTools Console 관찰:
   - **기대**: `[authStore] loadTokens: literal non-JSON detected, cleaning up` 형식의 console.warn 메시지 1회 출력.
   - **기대**: 메시지에 토큰 시크릿이 노출되지 않음.

#### Step 2 — 정상 로그인 → 응답 스키마 + 스토어 정합 검증

1. 로그인 화면에서 정상 자격증명(예: `admin/admin1234`)으로 로그인 수행.
2. DevTools Network → XHR → `POST /api/v1/auth/login` 요청 선택 → Response 탭 관찰:
   - **기대**: response body 가 `{"success": true, "data": {"user": {"username": "admin", "role": "<role>"}, "tokens": {"access_token": "...", "refresh_token": "...", "expires_at": <num>, "token_type": "Bearer"}}}` 형태.
   - **불합격 신호**: response body 가 flat `{"access_token": ...}` 형태 → U1 위반 (서버 미배포 또는 코드 미적용).
3. DevTools Application → Local Storage → `xflow_auth_tokens` 값 관찰:
   - **기대**: 유효 JSON 문자열로 저장되어 있으며, `JSON.parse` 시 `{access_token, refresh_token, expires_at}` 구조.
   - **불합격 신호**: literal `"undefined"` 또는 `"null"` → U2 위반.
4. DevTools Console 에서 `JSON.stringify(window.useAuthStore?.getState?.() ?? 'no global', null, 2)` 실행 (또는 React DevTools 사용):
   - **기대**: `{user: {name: "admin", role: "..."}, tokens: {access_token, refresh_token, expires_at}, isAuthenticated: true, ...}` 구조.
   - **불합격 신호**: `user` 또는 `tokens` 가 `undefined`/`null` 이면서 `isAuthenticated: true` → U2 위반.

#### Step 3 — 인증 API 호출 + Authorization 헤더 자동 첨부 검증

1. 대시보드 진입 후 자동 발생하는 임의 인증 API 호출 관찰 (예: `GET /api/v1/agents?detail=summary`).
2. DevTools Network → XHR → 해당 요청 선택 → Headers 탭 관찰:
   - **기대**: `Authorization: Bearer <jwt>` 헤더 존재.
   - **기대**: 응답 status 200.
   - **불합격 신호**: Authorization 헤더 누락 → request interceptor 가 `tokens.access_token` 을 읽지 못한 것, 즉 U2 위반.
   - **불합격 신호**: 응답 401 → 토큰이 위조/만료가 아닌 한 U2 또는 U1 위반.

#### Step 4 — WebSocket 토큰 동반 연결 검증 (SPEC-AUTH-003 통합 회귀)

1. DevTools Network → WS 탭 관찰:
   - **기대**: `/ws?token=<jwt>` 요청 1회 발생 → `101 Switching Protocols` 응답.
   - **기대**: WS frame inspector 에서 메시지가 정상 송수신됨.
   - **불합격 신호**: `?token=` 누락된 요청 → SPEC-AUTH-003 의 U1 위반 또는 본 SPEC 의 U2 위반(토큰이 스토어에 없어서 WS 게이트가 disconnect 분기).
   - **불합격 신호**: 401 응답 후 즉시 재시도 무한 반복 → SPEC-AUTH-003 의 UB1 위반 (본 SPEC 으로 root cause 가 해소되었으면 발생 안 함).

#### Step 5 — 페이지 새로고침 후 인증 상태 복원

1. 현재 인증된 상태에서 페이지 새로고침 (F5 또는 Cmd+R).
2. DevTools Application → Local Storage 관찰:
   - **기대**: `xflow_auth_tokens` 키가 새로고침 전과 동일 값으로 유지됨.
3. DevTools Network → XHR 관찰:
   - **기대**: `GET /api/v1/auth/status` (또는 동등) → 200, `GET /api/v1/auth/me` → 200, 이후 정상 진입.
   - **기대**: 사용자에게 재로그인 화면이 표시되지 **않음**.
   - **불합격 신호**: 로그인 화면으로 강제 이동 → S1 위반.
4. DevTools Network → WS 관찰:
   - **기대**: `/ws?token=<jwt>` 신규 연결 발생, 정상 101 응답.

#### Step 6 — RefreshResponse 비대칭 보존 검증

1. DevTools Console 에서 다음 실행 (개발자 도구로 직접 refresh endpoint 호출):
   ```js
   const tokens = JSON.parse(localStorage.getItem('xflow_auth_tokens'));
   fetch('/api/v1/auth/refresh', {
     method: 'POST',
     headers: { 'Content-Type': 'application/json' },
     body: JSON.stringify({ refresh_token: tokens.refresh_token })
   }).then(r => r.json()).then(j => console.log(JSON.stringify(j.data, null, 2)));
   ```
2. Console 출력 관찰:
   - **기대**: `j.data` 가 flat `{access_token, refresh_token, expires_at, token_type}` 4개 키만 포함.
   - **기대**: `j.data.user` 가 `undefined`, `j.data.tokens` 가 `undefined`.
   - **불합격 신호**: `j.data` 에 `user` 또는 `tokens` 객체 키가 존재 → O1 위반 (RefreshResponse 가 의도와 다르게 변경됨).

### 4.3 합격 판정

- §4.2 의 Step 1 ~ Step 6 모두에서 "기대" 동작이 확인되고 "불합격 신호" 가 한 건도 관찰되지 않은 경우 수동 검증 PASS.
- Step 1 의 broken state 시뮬레이션은 SPEC 적용 전 broken state 가 존재했던 사용자 환경에서의 자가 회복을 검증한다. clean storage 에서 직접 시작한 경우 Step 1 의 1번을 수동 주입으로 수행하여 동등 검증.
- Step 4 의 WS 연결 검증은 SPEC-AUTH-003 의 통합 회귀 게이트로, 본 SPEC 의 root cause 해소가 SPEC-AUTH-003 의 의도된 동작과 정상 연결됨을 보증한다.
