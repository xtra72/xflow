---
spec_id: SPEC-AUTH-004
plan_version: 0.1.0
created: 2026-05-12
updated: 2026-05-12
author: xtra
---

## 1. 개요

본 plan 은 SPEC-AUTH-004 (`spec.md`) 의 6개 EARS 모듈을 server-side 스키마 정합 + client-side 방어 가드/자가 회복으로 구현하기 위한 마일스톤·기술 스택·의존성·위험 분석을 정의한다. 영향 파일은 정확히 5개로 한정되며, 신규 의존성(npm, Go module) 도입은 0건이다.

---

## 2. 기술 스택

| 항목 | 버전 / 도구 | 비고 |
|------|------------|------|
| Server Language | Go 1.23+ | `go.mod` 기존 설정 유지 |
| Server Framework | xflow 내부 `internal/api` (Echo-like Context 래퍼) | 기존 라우팅 유지 |
| Server Testing | `testing` + `testify` (기존 의존성) | `internal/api/handler/auth_test.go` 갱신 |
| Server Linting | `gofmt`, `go vet`, `golangci-lint` | 기존 명령 그대로 |
| Client Language | TypeScript 5.9 | `web/tsconfig.json` strict 유지 |
| Client UI | React 19 | 영향 없음 (변경 대상은 store/service) |
| Client State | Zustand 5 (`useAuthStore`) | 기존 인스턴스 사용 |
| Client Testing | Vitest (현행) + `@testing-library/react` | jsdom 환경 |
| Client Linting | ESLint 9, `tsc --noEmit` | 기존 명령 |
| Package Manager (web) | pnpm | 기존 `web/pnpm-lock.yaml` |

**Production-stable 만 사용** — beta/alpha 패키지 도입 금지. 신규 dependency 도입 없음.

---

## 3. 마일스톤 (M-1 ~ M-6)

각 마일스톤은 정확히 한 파일 그룹에 매핑되며, 이전 마일스톤 완료 후 다음 마일스톤이 시작된다 (의존성 sequential, 단 M-1/M-2 와 M-3/M-4/M-5 는 server/client 분리로 독립 실행 가능).

### M-1 — `internal/api/dto/auth.go` 스키마 재구조화

- 파일: `internal/api/dto/auth.go`
- EARS 매핑: U1 (server-side schema), O1 (RefreshResponse 보존)
- 변경 요약:
  - `LoginResponse` 구조를 다음으로 재정의:
    ```go
    type LoginResponse struct {
        User   UserInfoResponse `json:"user"`
        Tokens TokenPair        `json:"tokens"`
    }
    type TokenPair struct {
        AccessToken  string `json:"access_token"`
        RefreshToken string `json:"refresh_token"`
        ExpiresAt    int64  `json:"expires_at"`
        TokenType    string `json:"token_type"`
    }
    ```
  - 기존 flat 필드(`AccessToken`, `RefreshToken`, `ExpiresAt`, `TokenType`) 를 `TokenPair` 로 이전.
  - `UserInfoResponse` 는 기존 정의 재사용 (`Username`, `Role`).
  - `RefreshResponse` 는 **변경 없음** (`O1` 에 따라 flat 유지). 단, `TokenPair` 와의 필드 중복이 발생하므로 `RefreshResponse` 정의 위에 `// O1 (SPEC-AUTH-004): LoginResponse 와 의도적으로 다른 flat 구조 — 리프레시는 user 정보를 반복 전송하지 않음.` 주석 추가.
  - Go 패키지 export 시그니처 변경: 외부 패키지가 `dto.LoginResponse{AccessToken: ...}` 직접 리터럴 생성하는 경우가 있다면 컴파일 에러 발생 가능. grep 으로 확인 후 영향 파일은 `handler/auth.go` 만임을 검증 (M-2 에서 처리).
- 완료 기준: `go vet ./...` 0 error, `go build ./...` 성공.

### M-2 — `internal/api/handler/auth.go` `login()` 핸들러 응답 조립 변경

- 파일: `internal/api/handler/auth.go`
- EARS 매핑: U1
- 변경 요약:
  - `login()` 함수의 응답 조립부 (line 88-93) 를 다음으로 변경:
    ```go
    return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(dto.LoginResponse{
        User: dto.UserInfoResponse{
            Username: user.Username,
            Role:     user.Role,
        },
        Tokens: dto.TokenPair{
            AccessToken:  accessToken,
            RefreshToken: refreshToken,
            ExpiresAt:    expiresAt,
            TokenType:    "Bearer",
        },
    }))
    ```
  - `refresh()` 함수는 변경 없음 (`O1`).
- 완료 기준: `go test ./internal/api/handler/... -run TestLogin` 가 갱신된 단위 테스트(M-3)와 함께 GREEN.

### M-3 — `internal/api/handler/auth_test.go` 단위 테스트 갱신

- 파일: `internal/api/handler/auth_test.go`
- EARS 매핑: U1, O1
- 변경 요약:
  - 기존 `TestLogin_Success` (또는 동등 테스트) 가 응답 body 의 flat 필드를 assert 하는 경우, 본 SPEC 의 `{user, tokens}` 중첩 구조로 갱신.
  - 추가 assertion:
    - `resp.User.Username` 이 요청한 username 과 일치.
    - `resp.User.Role` 이 비어있지 않은 문자열이며 알려진 role 값.
    - `resp.Tokens.AccessToken`, `resp.Tokens.RefreshToken` 비어있지 않음.
    - `resp.Tokens.ExpiresAt > 0`.
    - `resp.Tokens.TokenType == "Bearer"`.
  - 신규 회귀 테스트 `TestLogin_ResponseShape` 추가:
    - JSON 마샬링 후 최상위 키가 정확히 `{user, tokens}` 두 개임을 확인.
    - `tokens` 가 객체이며 `access_token`, `refresh_token`, `expires_at`, `token_type` 네 개 키를 모두 포함.
  - 신규 회귀 테스트 `TestRefresh_ResponseShape` 추가 ([O1] 검증):
    - JSON 마샬링 후 최상위 키가 정확히 flat `{access_token, refresh_token, expires_at, token_type}` 네 개임을 확인 (변경되지 않았음을 명시).
- 완료 기준: 갱신된 + 신규 테스트 모두 GREEN, 기존 다른 테스트 회귀 0건.

### M-4 — `web/src/stores/authStore.ts` 방어 가드 + 자가 회복

- 파일: `web/src/stores/authStore.ts`
- EARS 매핑: UB1 (saveTokens 가드), UB2 (loadTokens 자가 회복)
- 변경 요약:
  - `saveTokens(tokens: AuthTokens)` 진입부에 다음 가드 추가:
    ```ts
    if (
      !tokens ||
      typeof tokens !== 'object' ||
      !tokens.access_token ||
      !tokens.refresh_token ||
      typeof tokens.expires_at !== 'number' ||
      tokens.expires_at <= 0
    ) {
      console.warn('[authStore] saveTokens: invalid tokens, skipping persist');
      return;
    }
    ```
  - `loadTokens()` 를 다음 흐름으로 재작성:
    ```ts
    function loadTokens(): AuthTokens | null {
      let raw: string | null;
      try {
        raw = localStorage.getItem(TOKENS_STORAGE_KEY);
      } catch {
        return null;
      }
      if (!raw) return null;

      // UB2: literal "undefined" / "null" 자가 회복
      if (raw === 'undefined' || raw === 'null') {
        console.warn('[authStore] loadTokens: literal non-JSON detected, cleaning up');
        try { localStorage.removeItem(TOKENS_STORAGE_KEY); } catch { /* ignore */ }
        return null;
      }

      let parsed: unknown;
      try {
        parsed = JSON.parse(raw);
      } catch {
        console.warn('[authStore] loadTokens: malformed JSON, cleaning up');
        try { localStorage.removeItem(TOKENS_STORAGE_KEY); } catch { /* ignore */ }
        return null;
      }

      // 구조적 유효성 검증
      if (
        !parsed ||
        typeof parsed !== 'object' ||
        typeof (parsed as AuthTokens).access_token !== 'string' ||
        !(parsed as AuthTokens).access_token ||
        typeof (parsed as AuthTokens).refresh_token !== 'string' ||
        !(parsed as AuthTokens).refresh_token ||
        typeof (parsed as AuthTokens).expires_at !== 'number' ||
        (parsed as AuthTokens).expires_at <= 0
      ) {
        console.warn('[authStore] loadTokens: invalid shape, cleaning up');
        try { localStorage.removeItem(TOKENS_STORAGE_KEY); } catch { /* ignore */ }
        return null;
      }

      return parsed as AuthTokens;
    }
    ```
  - 기존 `login`, `logout`, `setTokens`, `initialize` 의 흐름은 변경하지 않음. 단 `saveTokens` 호출 직전 명시적 가드는 도입 (이중 안전망 — `login` 진입 시 user/tokens 검증).
  - `login(user, tokens)` 진입부에 다음 가드 추가:
    ```ts
    if (!user || !tokens) {
      console.error('[authStore] login: rejected (user or tokens missing)');
      return;
    }
    ```
- 완료 기준: M-6 의 단위 테스트가 GREEN.

### M-5 — `web/src/services/api/authService.ts` 응답 매핑 정합

- 파일: `web/src/services/api/authService.ts` (또는 동등 파일, 정확한 경로는 M-5 착수 시 확인)
- EARS 매핑: U2 (클라이언트 매핑)
- 변경 요약:
  - `login(credentials)` 함수의 반환 형태를 신 스키마에 맞춰 갱신:
    - 서버 응답 envelope 언래핑 후 형태: `{user: {username, role}, tokens: {access_token, refresh_token, expires_at, token_type}}`.
    - 클라이언트 도메인 매핑:
      - `User { name: response.user.username, role: response.user.role as UserRole }`.
      - `AuthTokens { access_token, refresh_token, expires_at }` (token_type 무시).
    - 반환: `{ user, tokens }` (`web/src/types/auth.ts` 의 `LoginResponse` 와 일치).
  - 본 마일스톤 착수 시 `authService.login()` 의 현재 시그니처와 envelope 언래핑 단계를 확인하고, 매핑 변환을 어느 layer 에 둘지 결정 (가능한 한 `authService` 내부에서 1회 변환하여 호출자(`useAuth.login`)는 도메인 타입만 받도록 함).
  - `useAuth.login()` (`web/src/hooks/useAuth.ts:24-25`) 는 무변경 — `response.user`, `response.tokens` 가 이미 도메인 타입으로 매핑되어 도착.
- 완료 기준: 통합 테스트(M-6 의 통합 케이스 또는 수동 검증 §4)에서 정상 로그인 시 `useAuthStore` 가 [U2] 의 불변식을 만족.

### M-6 — `web/src/stores/authStore.test.ts` (신규 또는 갱신) 단위 테스트

- 파일: `web/src/stores/authStore.test.ts` (현재 존재 여부 확인 후 신규 또는 갱신)
- EARS 매핑: UB1, UB2, U2
- 테스트 항목:
  - U2-T1: `login(user, tokens)` 호출 시 `useAuthStore` 상태가 `{user: defined, tokens: defined, isAuthenticated: true, isLoading: false}` 로 전이.
  - U2-T2: 로그인 직후 `localStorage.getItem('xflow_auth_tokens')` 가 유효한 JSON 이며 파싱 시 `tokens` 와 일치.
  - UB1-T1: `saveTokens(undefined)` 호출 (직접 invoke) 시 localStorage 변경 없음, console.warn 발생.
  - UB1-T2: `saveTokens({access_token: '', refresh_token: 'x', expires_at: 100})` (빈 access_token) 호출 시 localStorage 변경 없음.
  - UB1-T3: `login(undefined as any, undefined as any)` 호출 시 store 상태 변경 없음, `isAuthenticated` 가 이전 상태 유지.
  - UB2-T1: localStorage 에 literal `"undefined"` 가 사전 저장된 상태에서 `loadTokens()` 호출 → `null` 반환 + localStorage 키 자동 삭제.
  - UB2-T2: localStorage 에 `"null"` 저장 상태 → 동일하게 자가 회복.
  - UB2-T3: localStorage 에 malformed JSON (`"{not json"`) 저장 상태 → 자가 회복.
  - UB2-T4: localStorage 에 파싱 가능하나 구조 불일치 JSON (`'{"foo": "bar"}'`) 저장 상태 → 자가 회복.
  - UB2-T5: localStorage 에 유효 JSON 이며 유효 구조 (`{access_token, refresh_token, expires_at}`) 저장 상태 → 정상 반환, localStorage 미변경.
  - U2-T3 (통합): broken state (`localStorage.setItem(KEY, "undefined")`) 후 `useAuthStore.getState().initialize()` 호출 → localStorage 클린업 + 정상 미인증 상태 (`isAuthenticated: false`).
- 테스트 도구: Vitest, `@testing-library/react` (필요 시), `vi.spyOn(console, 'warn')`, jsdom `localStorage`.
- 완료 기준: 11 개 케이스 모두 GREEN, `authStore.ts` 의 신규/수정 라인 커버리지 ≥ 85%.

---

## 4. 의존성

### 4.1 신규 패키지

- **신규 추가 금지** (가정 §6 참조).

### 4.2 기존 모듈 의존

- `internal/api/dto` — `LoginResponse`, `UserInfoResponse`, `RefreshResponse`, `TokenPair` (신규 export).
- `internal/api/handler` — `AuthHandler.login()`, `auth.CredentialsManager`, `auth.JWTService`.
- `web/src/types/auth.ts` — `User`, `AuthTokens`, `LoginResponse`, `UserRole`.
- `web/src/stores/authStore.ts` — `useAuthStore` (zustand).
- `web/src/services/api/authService.ts` — `login()`, `getAuthStatus()`, `getCurrentUser()`.
- React 19 표준 hooks (변경 없음).

### 4.3 SPEC 의존

- `SPEC-DASHBOARD-001 v0.2.0`: `basic_auth: true` 기본값 도입 — 본 SPEC 이 해결하는 결함의 surface 트리거.
- `SPEC-AUTH-002`: 결함의 origin (커밋 `8635e1f`, 2026-03-31) — 본 SPEC 이 해결 대상.
- `SPEC-AUTH-003`: WebSocket 인증 경로 수정 — 본 SPEC 적용 후 SPEC-AUTH-003 의 수동 검증 §4 Step 2 가 PASS 해야 함 (통합 회귀 게이트).

---

## 5. 위험 분석 및 완화책

| # | 위험 요소 | 영향도 | 완화책 |
|---|----------|--------|--------|
| R-1 | DTO 시그니처 변경으로 다른 server-side 호출자 컴파일 에러 | Med | M-1 착수 전 `grep -r "dto.LoginResponse{" internal/` 로 모든 호출자 확인. 현재 코드베이스에서 `handler/auth.go` 만 영향받음을 검증. |
| R-2 | 클라이언트 응답 매핑 누락으로 로그인 후 상태 여전히 broken | High | M-5 의 매핑 변환을 `authService.login()` 내부에 1회만 수행하고 호출자는 도메인 타입만 받도록 단일화. M-6 의 U2-T1, U2-T2 로 회귀 검증. |
| R-3 | UB2 의 자가 회복이 정상 broken state(예: storage quota 초과로 인한 잘림) 도 무차별 정리 | Low | UB2 발동 조건을 명확한 화이트리스트(literal `"undefined"`, `"null"`, JSON parse fail, 구조 불일치) 로만 한정. 기타 케이스는 silent null 반환만 (기존 동작 유지). |
| R-4 | 본 SPEC 적용 직후 활성 사용자 세션이 모두 invalidation 됨 | Med | 본 SPEC 의 의도된 동작이며 [UB2] 가 자동 클린업 후 재로그인 유도. README/CHANGELOG 에 1줄 안내 추가 (M-7 의 sync phase 입력). |
| R-5 | RefreshResponse 비대칭이 미래 개발자에게 혼란 유발 | Low | M-1 에서 `RefreshResponse` 정의 위 한국어 주석으로 의도적 비대칭임을 명시. [O1] EARS 모듈 및 본 plan §3 M-1 에 명시. |
| R-6 | console.warn 출력에 토큰 시크릿 노출 | High | M-4 의 모든 console.warn 메시지에 raw 토큰 값을 포함하지 않음을 코드 리뷰로 검증. raw 문자열은 출력하지 않으며, 필요시 최대 16자 prefix 만 출력. |
| R-7 | jsdom 환경에서 `localStorage` 가 정상 동작하지 않을 가능성 | Low | Vitest jsdom 기본 환경에서 `localStorage` 는 정상 동작 확인됨 (SPEC-AUTH-003 의 테스트도 동일 환경 사용). 추가 모킹 불필요. |
| R-8 | 서버 단위 테스트가 응답 body 구조 변경으로 다수 회귀 | Med | M-3 에서 모든 영향받는 테스트를 한 번에 갱신. `grep -r "AccessToken" internal/api/handler/auth_test.go` 로 영향 라인 식별. |

---

## 6. LSP Baseline 캡처 (orchestrator 수행)

본 plan 작성자는 LSP baseline 을 직접 캡처하며, 결과는 `.moai/specs/SPEC-AUTH-004/lsp-baseline.json` 에 저장된다.

캡처 명령:

- Go vet: `cd /Users/xtra/Projects/xflow && go vet ./... 2>&1`
- Go fmt: `gofmt -l internal/api/`
- TypeScript: `cd web && pnpm tsc --noEmit 2>&1`
- ESLint: `cd web && pnpm lint 2>&1`

baseline 캡처 결과는 run phase 의 quality gate (zero new errors, zero new type errors, zero new lint errors) 기준점으로 사용된다.

---

## 7. 영향 범위 (정확히 5개 파일)

| 파일 | 상태 | 변경 종류 |
|------|------|-----------|
| `internal/api/dto/auth.go` | 수정 | M-1 (LoginResponse 재구조화 + TokenPair 신설 + RefreshResponse 보존 주석) |
| `internal/api/handler/auth.go` | 수정 | M-2 (login() 응답 조립 변경) |
| `internal/api/handler/auth_test.go` | 수정 | M-3 (단위 테스트 갱신 + 신규 회귀 테스트 2건) |
| `web/src/stores/authStore.ts` | 수정 | M-4 (saveTokens 가드 + loadTokens 자가 회복 + login 가드) |
| `web/src/services/api/authService.ts` | 수정 | M-5 (응답 매핑 정합화) |

신규 파일:

| 파일 | 상태 | 변경 종류 |
|------|------|-----------|
| `web/src/stores/authStore.test.ts` | 신규 또는 갱신 | M-6 (11 케이스 단위 테스트) |

서버 외 다른 frontend 모듈(`useAuth.ts`, `useWebSocket.ts`, `interceptors.ts`), 빌드/CI 설정, 의존성 manifest 모두 변경하지 않는다. 단 M-6 시점에 `authStore.test.ts` 가 기존에 존재하지 않는다면 신규 추가.

---

## 8. 우선순위 및 마일스톤 순서

- **Primary Goal**: M-1 → M-2 → M-3 (server-side 스키마 정합 + 단위 테스트). 결함의 root cause 해소.
- **Secondary Goal**: M-5 (클라이언트 응답 매핑). [U2] 의 정합 마무리.
- **Tertiary Goal**: M-4 → M-6 (방어 가드 + 자가 회복 + 단위 테스트). 미래 회귀 방지 및 기존 broken state 회복.
- **Final Goal**: SPEC-AUTH-003 수동 검증 §4 Step 2 통합 회귀 PASS — TRUST 5 의 Tested 통과 + 결함 영구 종결.

순차 실행 권장. 단 server(M-1/M-2/M-3) 와 client(M-4/M-5/M-6) 는 독립 영향 영역이므로 병렬 실행 가능. 통합 회귀 검증은 양측 머지 후 진행.

---

## 9. 완료 기준 (Definition of Done — plan 수준)

- 모든 acceptance 시나리오(AC-1 ~ AC-7) GREEN.
- LSP baseline 대비 신규 Go vet 에러 0건, gofmt 위반 0건, TypeScript type errors 0건, 신규 ESLint errors 0건.
- `internal/api/handler/auth.go`, `internal/api/dto/auth.go`, `web/src/stores/authStore.ts`, `web/src/services/api/authService.ts` 의 변경 라인 커버리지 ≥ 85%.
- 기존 `internal/`, `web/` 테스트 스위트 회귀 0건.
- 본 plan 영향 범위 §7 외 파일 변경 0건.
- SPEC-AUTH-003 의 수동 검증 §4 Step 2 (`로그인 → WS 연결`) 가 본 SPEC 적용 후 PASS.
