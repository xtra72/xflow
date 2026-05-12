---
id: SPEC-AUTH-004
title: REST 로그인 응답 스키마 정합 및 토큰 보존 자가 회복
version: 0.1.0
status: completed
created: 2026-05-12
updated: 2026-05-12
completed: 2026-05-12
author: xtra
priority: high
domain: auth
related_specs:
  - SPEC-DASHBOARD-001
  - SPEC-AUTH-002
  - SPEC-AUTH-003
lifecycle_level: spec-first
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 0.1.0 | 2026-05-12 | xtra | 최초 작성 — SPEC-AUTH-003 수동 검증 중 발견된 REST `POST /api/v1/auth/login` 응답 스키마 불일치 결함을 SPEC으로 정식화. 서버 DTO와 클라이언트 TypeScript 타입 간 형태 불일치로 토큰이 영구 손실되고 localStorage에 literal `"undefined"` 가 기록되어 모든 인증 경로가 붕괴되는 결함을 서버 측 스키마 정합 + 클라이언트 측 방어 가드 + 자가 회복 로직으로 해결한다. |

---

## 1. 개요 (Overview)

### 1.1 목적

`SPEC-DASHBOARD-001 v0.2.0` 의 `basic_auth: true` 기본값 전환 이후 표면화된 **REST 로그인 응답 스키마 비정합 결함**을 제거한다. 본 SPEC 은 다음 세 축을 동시에 정의한다.

1. 서버 `POST /api/v1/auth/login` 응답이 클라이언트 `LoginResponse` 타입과 형태가 일치하도록 `dto.LoginResponse` 스키마를 정합화한다 (`{user, tokens}` 중첩 구조).
2. 클라이언트 `authStore.saveTokens()` 가 falsy 값을 localStorage 에 기록하지 못하도록 런타임 가드를 도입한다.
3. 클라이언트 `authStore` 의 토큰 로드 경로가 기존 broken state (literal `"undefined"` 가 저장된 localStorage) 를 만나도 자동 클린업 후 정상 인증 흐름을 회복하도록 self-healing 로직을 추가한다.

본 SPEC 의 산출물은 server 측 DTO + handler + 단위 테스트(필수) 와 client 측 방어 가드 + cleanup 로직(선택적 보강)으로 구성된다.

### 1.2 배경

#### 1.2.1 결함 표면화 경로

- SPEC-AUTH-003 (commits `dad6010`, `9ec8597`) 가 WebSocket 토큰 전달 결함을 해소한 직후 동일한 수동 검증 절차의 Step 2 (`로그인 → WS 연결`) 가 여전히 실패함이 발견됨. 분석 결과 WS 게이트는 정상 동작하나 그 전 단계인 REST 로그인 응답이 토큰을 클라이언트 스토어로 전달하지 못함이 root cause.
- `9dcc69c` 핫픽스(`fix(web): SPEC-DASHBOARD-001 v0.2.0 — 401/500 무한 PUT 루프 차단 (hotfix)`)는 REST PUT 경로의 무한 401 루프를 차단했으나, 로그인 응답 자체의 토큰 손실은 별개의 결함으로 잔존.
- 결함 자체는 `8635e1f` (SPEC-AUTH-002, 2026-03-31) 시점부터 존재했으나 `basic_auth: false` 환경에서 인증 미들웨어가 pass-through 였기에 잠재적 결함으로 머무름. v0.2.0 의 `basic_auth: true` 기본값 전환과 함께 즉시 표면화.

#### 1.2.2 코드 레벨 근거 (스키마 비정합)

**서버 응답** (`internal/api/dto/auth.go:10-15` + `internal/api/handler/auth.go:88-93`, envelope 언래핑 후):

```json
{
  "access_token": "<jwt>",
  "refresh_token": "<jwt>",
  "expires_at": 1234567890,
  "token_type": "Bearer"
}
```

플랫 구조. `user` 객체 없음. `web/src/services/api/client.ts:43` 의 envelope interceptor 가 `response.data.data` 를 풀어낸 시점의 형태.

**클라이언트 기대** (`web/src/types/auth.ts:37-40`):

```ts
interface LoginResponse {
  user: User;
  tokens: AuthTokens;
}
```

중첩 `{user, tokens}` 구조. 두 필드 모두 소비 시점(`web/src/hooks/useAuth.ts:24-25`)에서 `undefined`:

```ts
const response = await authService.login(...);
storeLogin(response.user, response.tokens);  // 둘 다 undefined
```

#### 1.2.3 결함의 관찰 가능한 캐스케이드

1. `storeLogin(undefined, undefined)` 가 `authStore.ts:69-77` 에 진입.
2. `saveTokens(undefined)` 호출 → `localStorage.setItem('xflow_auth_tokens', JSON.stringify(undefined))` → **literal 문자열 `"undefined"`** 가 저장됨 (`JSON.stringify(undefined) === undefined` 인 JavaScript 의 미묘한 동작; `setItem` 이 두 번째 인자를 강제로 String 변환).
3. 인메모리 상태: `{user: undefined, tokens: undefined, isAuthenticated: true}` — `isAuthenticated` 만 `true` 인 가짜 인증 상태.
4. REST request interceptor (`web/src/services/api/interceptors.ts:28-38`) 가 `useAuthStore.getState().tokens?.access_token` 평가 → `undefined` → `Authorization` 헤더 미첨부.
5. 서버가 모든 인증 endpoint 에 대해 401 응답 (예: `GET /api/v1/agents?detail=summary`).
6. SPEC-AUTH-003 의 WS S1 게이트 평가: `authEnabled === true ∧ isAuthenticated === true ∧ !access_token` → `disconnect` 분기 (WS 연결 불가; SPEC-AUTH-003 가 의도한 정확한 방어 동작이나 root cause 는 본 SPEC 의 영역).
7. 페이지 새로고침: `loadTokens()` 가 `JSON.parse("undefined")` 실행 → SyntaxError → 기존 try/catch (`authStore.ts:37-38`) 가 silent null 반환 → 인증 미복원 상태 유지. 단, **literal `"undefined"` 가 localStorage 에 잔존**하여 매 새로고침마다 동일 경로 반복.

#### 1.2.4 RefreshResponse 와의 비대칭 관찰

`dto.RefreshResponse` (`internal/api/dto/auth.go:23-28`) 는 동일한 flat 구조이며 클라이언트 `refreshToken()` (`web/src/services/api/interceptors.ts:84-89`) 가 이를 `AuthTokens` 로 직접 소비하여 정상 동작한다. 즉 결함은 **`login` 한 경로에만** 존재하며 `refresh` 는 영향받지 않는다. 본 SPEC 은 이 비대칭(login = `{user, tokens}`, refresh = flat `AuthTokens`) 을 의도된 설계로 명시화한다.

### 1.3 비범위 (Out of Scope)

- WebSocket 인증 경로 — SPEC-AUTH-003 가 이미 해결.
- REST PUT 무한 401/500 루프 — hotfix `9dcc69c` 가 이미 해결 (다른 결함).
- 리프레시 토큰 응답 스키마 변경 — `RefreshResponse` 는 그대로 유지 (의도된 비대칭).
- JWT 클레임/만료 시간 정책 변경.
- 신규 인증 방식(OAuth, mTLS 등) 도입.
- 사용자 권한(`role`) 체계 확장.
- localStorage 외 저장 매체(sessionStorage, cookie) 도입.

---

## 2. EARS 요구사항

본 SPEC 은 6개의 EARS 모듈로 구성된다. 각 모듈은 독립적으로 테스트 가능하며, 식별자(`U1`, `U2`, `S1`, `UB1`, `UB2`, `O1`)는 트레이서빌리티 표 및 acceptance 시나리오에서 직접 참조된다.

### 2.1 [U1] (Ubiquitous) 로그인 응답 스키마 정합

시스템은 항상 `POST /api/v1/auth/login` 의 성공 응답을 envelope 언래핑 후 다음 형태로 반환해야 한다:

```json
{
  "user": { "username": "<non-empty>", "role": "<non-empty>" },
  "tokens": {
    "access_token": "<non-empty JWT>",
    "refresh_token": "<non-empty JWT>",
    "expires_at": <positive int64>,
    "token_type": "Bearer"
  }
}
```

**상세 규약**

- 최상위 키는 정확히 `user` 와 `tokens` 두 개로 한정한다 (확장성을 위한 추가 키는 본 SPEC 범위 외).
- `user.username` 은 비어있지 않은 문자열이며 인증된 사용자의 username 과 일치한다.
- `user.role` 은 비어있지 않은 문자열이며 `admin`, `editor`, `viewer` 중 하나이다 (`web/src/types/auth.ts:6` 의 `UserRole` 과 일치).
- `tokens.access_token`, `tokens.refresh_token` 은 비어있지 않은 JWT 문자열이다.
- `tokens.expires_at` 은 양의 int64 정수 (Unix epoch seconds).
- `tokens.token_type` 은 항상 `"Bearer"` 리터럴이다.
- 응답은 envelope (`{success: true, data: <위 객체>, ...}`) 로 감싸지며, 본 EARS 의 형태 명세는 envelope 언래핑 **후** 의 `data` 페이로드를 지칭한다.

### 2.2 [U2] (Ubiquitous) 클라이언트 로그인 성공 상태 전이

시스템은 항상 `useAuth.login(username, password)` 가 성공 resolve 한 직후 `useAuthStore` 의 상태가 다음 불변식을 만족하도록 전이해야 한다:

- `user !== null` 이며 `user.name` 비어있지 않은 문자열, `user.role` 은 `UserRole` 유효 값.
- `tokens !== null` 이며 `tokens.access_token` 비어있지 않은 문자열, `tokens.refresh_token` 비어있지 않은 문자열, `tokens.expires_at` 양의 정수.
- `isAuthenticated === true`.
- `isLoading === false`.
- localStorage 의 `xflow_auth_tokens` 키에 위 `tokens` 객체의 유효한 JSON 직렬화 결과가 저장됨.

**상세 규약**

- 본 EARS 는 `useAuth.login()` 의 호출 시점 클라이언트가 서버 응답을 정확히 클라이언트 도메인 타입(`User`, `AuthTokens`) 에 매핑함을 요구한다.
- 서버 응답의 `user.username` 은 클라이언트 `User.name` 으로 매핑된다 (`web/src/types/auth.ts:12-15` 의 기존 정의 유지).
- 매핑 변환은 `authService.login()` 내부에서 1회 수행되며, 호출자(`useAuth.login`) 는 변환된 도메인 타입만 받는다.

### 2.3 [S1] (State-Driven) 페이지 새로고침 시 인증 상태 복원

`WHILE` 직전 로그인이 [U2] 의 불변식을 만족한 채로 종료되어 localStorage 에 유효한 토큰 JSON 이 저장되어 있는 동안, 페이지 새로고침이 발생하면 `authStore.initialize()` 가 다음 순서로 동작하여 동일한 인증 상태를 복원해야 한다:

1. `authService.getAuthStatus()` 호출 → `authEnabled === true` 확인.
2. `loadTokens()` 호출 → 저장된 토큰 객체 복원 (`AuthTokens` 형태).
3. `tokens` 상태를 우선 설정 (request interceptor 가 `Authorization` 헤더를 첨부할 수 있도록 보장).
4. `authService.getCurrentUser()` 호출 (`GET /api/v1/auth/me`) → 200 응답 수신 후 `user` 상태 설정.
5. 최종 상태: `{user: defined, tokens: defined, isAuthenticated: true, isLoading: false, authEnabled: true}`.

**상세 규약**

- 1 단계에서 `authEnabled === false` 인 경우 본 EARS 의 적용 대상이 아니며 기존 동작(인증 비활성화 경로) 유지.
- 4 단계에서 `getCurrentUser()` 가 실패한 경우(401, 네트워크 오류 등) 기존 동작(`clearTokens()` + 모든 상태 reset) 유지.
- 새로고침 전후 사용자에게 추가 액션(재로그인) 이 요구되지 않아야 한다.

### 2.4 [UB1] (Unwanted-Behavior) saveTokens 의 falsy 값 차단

`authStore.saveTokens()` 는 `null`, `undefined`, 또는 truthy 가 아닌 값(빈 객체 포함)을 localStorage 에 저장해서는 **안 된다**.

**상세 규약**

- 런타임 가드: `saveTokens()` 진입 즉시 인자에 대한 null/undefined 체크 수행. falsy 인 경우 즉시 return 하며 localStorage 변경을 발생시키지 않음.
- 정적 안전: TypeScript 시그니처는 기존 `saveTokens(tokens: AuthTokens): void` 를 유지 (호출자가 null/undefined 를 전달하지 못하도록 차단). [U2] 의 정합화로 정상 경로에서는 falsy 가 전달될 일이 없으나, 본 가드는 미래의 정합 결함을 막는 안전망.
- 추가 가드: `tokens.access_token` 또는 `tokens.refresh_token` 이 비어있는 문자열이거나 falsy 인 경우에도 저장하지 않음 (구조적 유효성 1차 검증).
- 가드 발동 시 console.warn 으로 진단 메시지를 남기되, 토큰 값 자체는 로그에 노출하지 않는다.

### 2.5 [UB2] (Unwanted-Behavior) loadTokens 의 broken state 자가 회복

`authStore.loadTokens()` 는 localStorage 의 `xflow_auth_tokens` 키 값이 비-JSON 문자열(literal `"undefined"`, `"null"`, 또는 JSON 파싱 실패하는 임의 값) 이거나 파싱은 성공해도 `AuthTokens` 구조를 만족하지 않는 경우, silent null 반환에 더하여 **해당 localStorage 키를 자동 삭제**해야 한다.

**상세 규약**

- 본 가드의 목적은 본 SPEC 이전 버전에서 생성된 broken state (literal `"undefined"`) 를 회복하기 위함.
- 회복 조건:
  - `raw === "undefined"` (literal 문자열).
  - `raw === "null"` (literal 문자열).
  - `JSON.parse(raw)` 가 throw.
  - 파싱 성공했으나 `access_token` 또는 `refresh_token` 가 비어있는 문자열이거나 falsy.
  - 파싱 성공했으나 `expires_at` 이 양의 정수가 아님.
- 회복 동작: `localStorage.removeItem(TOKENS_STORAGE_KEY)` 호출 후 `null` 반환.
- 정상 케이스(유효 JSON, 유효 구조) 는 기존 동작 유지.
- 회복 발동 시 console.warn 으로 진단 메시지를 남기되, 원본 raw 값은 잘라낸 일부만 노출 (토큰 시크릿 노출 방지).

### 2.6 [O1] (Optional) RefreshResponse 비대칭 명시화

가능하면, `POST /api/v1/auth/refresh` 의 응답 스키마는 본 SPEC 의 변경에 영향받지 않으며 기존 flat `{access_token, refresh_token, expires_at, token_type}` 형태(클라이언트 `AuthTokens` 와 동일) 를 그대로 유지한다.

**상세 규약 — 본 SPEC 의 acceptance scope 에 정식 포함**

- "Optional" EARS 분류는 유지되나(스키마 변경의 부재 자체를 명문화하는 의미), acceptance 시나리오에는 정식으로 포함되어 회귀 방지 게이트로 기능한다.
- 로그인 응답은 `{user, tokens}` 중첩, 리프레시 응답은 flat `AuthTokens` 라는 비대칭은 의도된 설계이며 다음 근거에 기반한다:
  - 로그인은 사용자 식별 정보(`user`) 와 토큰을 함께 전달해야 하나, 리프레시는 토큰 회전만 수행하며 사용자 정보는 이미 보유 상태.
  - 클라이언트 `refreshToken()` 흐름은 응답을 `AuthTokens` 로 직접 사용 (`interceptors.ts:84-89`).
  - 본 비대칭을 통일하려면 `RefreshResponse` 도 `{user, tokens}` 로 변경해야 하나, 사용자 정보를 매 리프레시마다 재전송하는 것은 불필요한 페이로드 증가이며 기존 동작 회귀를 유발.
- 본 모듈의 acceptance 시나리오(AC-7)는 `RefreshResponse` 구조가 변경되지 않았음을 회귀 테스트로 검증한다.

---

## 3. 트레이서빌리티 표

각 EARS 모듈을 영향 파일 및 acceptance 시나리오에 매핑한다.

| 모듈 ID | 요약 | 영향 파일 | 신규/수정 | 관련 acceptance |
|---------|------|-----------|-----------|-----------------|
| U1 | 로그인 응답 `{user, tokens}` 스키마 | `internal/api/dto/auth.go`, `internal/api/handler/auth.go` | 수정 | AC-1, AC-3 |
| U2 | 클라이언트 로그인 성공 상태 전이 | `web/src/services/api/authService.ts` (매핑), `web/src/hooks/useAuth.ts` (소비), `web/src/stores/authStore.ts` | 수정 | AC-1 |
| S1 | 새로고침 시 인증 상태 복원 | `web/src/stores/authStore.ts` (`initialize` 흐름 확인) | 보존/검증 | AC-2, AC-3 |
| UB1 | saveTokens falsy 차단 | `web/src/stores/authStore.ts` | 수정 | AC-5 |
| UB2 | loadTokens 자가 회복 | `web/src/stores/authStore.ts` | 수정 | AC-5, AC-6 |
| O1 | RefreshResponse 비대칭 보존 | `internal/api/dto/auth.go` (RefreshResponse 부분) | 보존 | AC-7 |
| 회귀 방지 | 서버 핸들러 단위 테스트 갱신 | `internal/api/handler/auth_test.go` | 수정 | AC-1, AC-3 |
| 회귀 방지 | SPEC-AUTH-003 수동 검증 Step 2 통합 회귀 | (수동 검증 절차 §4) | 검증 | AC-4 |

---

## 4. 스키마 결정 근거 (B-2 선택)

본 절은 server-side fix 옵션 중 본 SPEC 이 채택한 변형(B-2)의 선택 이유를 명문화한다.

### 4.1 검토된 대안

| 옵션 | 응답 형태 | 서버 diff | 클라이언트 diff | 채택 여부 |
|------|-----------|-----------|-----------------|-----------|
| A | flat `{access_token, refresh_token, expires_at, token_type}` (현 상태) | 변경 없음 | 클라이언트 `LoginResponse` 를 flat `AuthTokens` 로 변경 + `useAuth.login()` 사용자 정보 별도 fetch | 기각 — 로그인 후 추가 round-trip(`/auth/me`) 필요, UX 저하 |
| B-1 | flat with user `{user, access_token, refresh_token, expires_at, token_type}` | DTO에 `user` 필드만 추가 | 클라이언트가 flat 응답에서 `AuthTokens` 객체를 재조립 | 기각 — 클라이언트 매핑 코드 추가 필요, 타입 불일치 잔존 |
| **B-2** | **nested `{user, tokens: {...}}`** | **DTO 재구조화 + handler 응답 조립 변경** | **클라이언트 `LoginResponse` 와 정확히 일치, 추가 코드 없음** | **채택** |
| C | server는 그대로, 클라이언트가 flat 응답을 `{user, tokens}` 로 어댑터에서 재조립 + 사용자 정보 별도 fetch | 변경 없음 | 어댑터 신설 + `/auth/me` 추가 호출 | 기각 — round-trip 증가, 본질적 결함은 server 가 user 정보를 한 번에 안 줘서 발생한 것이므로 어댑터로 우회하는 것은 root cause 회피 |

### 4.2 B-2 선택 이유

- **클라이언트 타입과 단일 진실 원천**: `web/src/types/auth.ts:37-40` 의 `LoginResponse = {user: User, tokens: AuthTokens}` 가 이미 정확한 의도를 표현하고 있으며, server 가 그 형태로 응답하면 클라이언트 매핑 변환이 최소화된다.
- **`useAuth.login()` 의 무변경**: `web/src/hooks/useAuth.ts:24-25` 의 `storeLogin(response.user, response.tokens)` 가 그대로 동작 (단, `response.user.username` → `User.name` 매핑은 `authService.login()` 내부에서 처리, [U2] 참조).
- **단일 round-trip 보존**: 로그인 직후 사용자 정보를 별도로 fetch 할 필요 없음 (`/auth/me` 추가 호출 회피).
- **`RefreshResponse` 비대칭 정당화**: refresh 는 사용자 정보 재전송이 불필요하므로 flat 유지가 합리적이며, 본 SPEC 의 [O1] 이 이를 명시화.

### 4.3 클라이언트-서버 필드 매핑

| 서버 응답 필드 | 클라이언트 도메인 타입 필드 | 매핑 위치 |
|----------------|------------------------------|-----------|
| `user.username` (string) | `User.name` (string) | `authService.login()` 내부 매핑 |
| `user.role` (string) | `User.role` (`UserRole`) | `authService.login()` 내부 매핑 (타입 단언) |
| `tokens.access_token` | `AuthTokens.access_token` | 동일 키, 직접 전달 |
| `tokens.refresh_token` | `AuthTokens.refresh_token` | 동일 키, 직접 전달 |
| `tokens.expires_at` | `AuthTokens.expires_at` | 동일 키, 직접 전달 |
| `tokens.token_type` | (클라이언트 도메인 타입에 부재) | 무시 (서버에서 전송하나 클라이언트는 보관하지 않음) |

---

## 5. 비기능 요구사항

- **성능**: 응답 스키마 변경은 단일 추가 객체 키(`user`) 만 도입하며 페이로드 증가 < 100 bytes. 클라이언트 매핑 변환은 O(1) 동기 작업.
- **보안**: 서버는 응답에 비밀번호 해시, JWT 시크릿, 또는 기타 민감 정보를 포함하지 않는다. 클라이언트 console.warn 로그는 토큰 값을 노출하지 않으며 raw 문자열은 최대 16자까지 잘라서만 표시한다.
- **신뢰성**: [UB2] 의 자가 회복으로 본 SPEC 이전 버전에서 생성된 broken localStorage state 가 자동 정리되며, 사용자에게 수동 정리(개발자 도구로 localStorage 삭제) 가 요구되지 않는다.
- **테스트 가능성**: 서버 DTO 변경은 Go 단위 테스트(`internal/api/handler/auth_test.go`) 갱신으로 검증. 클라이언트 가드는 Vitest 환경에서 `localStorage` 모킹으로 검증.
- **호환성**: 본 SPEC 적용 후 구버전 클라이언트가 신버전 서버에 로그인하면 동작 불능. 단, 본 프로젝트는 web 단일 클라이언트만 존재하므로 서버-클라이언트 동시 배포로 백워드 호환 불필요.
- **회귀 방지**: SPEC-AUTH-003 의 수동 검증 §4 Step 2 (로그인 → WS 토큰 동반 연결) 가 본 SPEC 적용 후 PASS 함을 통합 회귀 게이트로 확인.

---

## 6. 가정 및 제약

- 서버 측 `auth.CredentialsManager.Authenticate()` 가 반환하는 user 객체에 `Username` 과 `Role` 필드가 이미 존재함 (`internal/api/handler/auth.go:66` 의 흐름에서 확인됨).
- 클라이언트 측 `User.name` 필드는 username 을 의미 (`web/src/types/auth.ts:13` 의 기존 정의 유지). 향후 표시명(display name) 도입은 별도 SPEC.
- `RefreshResponse` 는 본 SPEC 의 변경 대상이 아니며 비대칭 명시화는 [O1] 에서만 다룬다.
- 본 SPEC 은 `feature/SPEC-DASHBOARD-001` 브랜치 위에서 SPEC-AUTH-003 의 후속 작업으로 진행되며, 별도 브랜치 분기는 하지 않는다.
- 신규 npm dependency / Go module 도입은 금지 — 기존 표준 라이브러리 및 testify 만 사용.
- 본 SPEC 적용 시 모든 활성 사용자 세션은 invalidation 되며 재로그인 필요 (구 응답 형태로 저장된 broken state 는 [UB2] 가 자동 정리).

---

## 9. Implementation Notes (2026-05-12)

본 절은 SPEC-AUTH-004 의 구현 완료 시점(`8bf49e0`) 기준 실측 사항을 기록한다.

### 9.1 영향 범위 정합성

- 변경된 파일은 정확히 **6개** 이며, `plan.md` §7 의 계획된 영향 범위와 1:1 일치한다.
  - `internal/api/dto/auth.go` (LoginResponse DTO 재구조화)
  - `internal/api/handler/auth.go` (Login 핸들러 응답 조립 변경)
  - `internal/api/handler/auth_test.go` (Login 테스트 갱신 + 회귀 방지)
  - `web/src/stores/authStore.ts` (UB1 saveTokens 가드 + UB2 loadTokens 자가 회복 + test seam)
  - `web/src/services/api/authService.ts` (서버 응답 → 클라이언트 도메인 타입 매핑)
  - `web/src/stores/authStore.test.ts` (가드 단위 테스트)
- 9개 atomic task (TASK-001 ~ TASK-009) 가 `manager-ddd` 단일 위임으로 순차 실행되었다.

### 9.2 Scope Changes (계획 대비 추가 변경)

본 SPEC 의 구현 과정에서 plan.md §7 에 명시되지 않은 두 가지 추가 변경이 발생했다. 두 변경 모두 **behavior preserving** 이며 DTO 재구조화의 필연적 영향이거나 가이드 준수 차원이다.

#### 9.2.1 Sibling 테스트의 mechanical migration

`internal/api/handler/auth_test.go` 내 동급 테스트 4건이 `loginResp.Data.AccessToken` → `loginResp.Data.Tokens.AccessToken` 으로 mechanical 마이그레이션되었다. 이는 LoginResponse DTO 가 nested `{user, tokens}` 형태로 변경됨에 따른 **compile recovery** 차원의 불가피한 변경이며, 테스트 의도/검증 대상은 그대로 보존된다.

대상 테스트:

- `TestAuthHandler_Me`
- `TestAuthHandler_Logout`
- `TestAuthHandler_Refresh`
- `TestAuthHandler_ChangePassword`

#### 9.2.2 authStore 의 test seam 추가

`web/src/stores/authStore.ts` 에 다음 export 가 추가되었다:

```ts
/** @internal exported for unit tests only — do NOT use in production */
export const __test__ = { saveTokens, loadTokens, TOKENS_STORAGE_KEY };
```

이는 plan.md M-6 의 "내부 헬퍼는 모듈 내 closure 로 유지하되 단위 테스트가 필요하면 `@internal` JSDoc 어노테이션 + `__test__` 네임스페이스 export 패턴을 사용한다" 가이드를 준수한 것이다. Production 코드에서의 사용은 명시적으로 금지된다.

### 9.3 Acceptance 결과

자동화 가능한 acceptance 시나리오는 본 commit 시점에 모두 GREEN 이다.

| AC ID | 상태 | 검증 수단 | 비고 |
|-------|------|-----------|------|
| AC-1 | GREEN (자동) | 16 server tests + 11 client tests | 27 신규/갱신 테스트 |
| AC-2 | GREEN (자동) | client unit tests | localStorage 직렬화 검증 |
| AC-3 | **DEFERRED** | 사용자 수동 검증 게이트 | 페이지 새로고침 후 인증 복원 |
| AC-4 | **DEFERRED** | 사용자 수동 검증 게이트 | SPEC-AUTH-003 통합 WS 회귀 |
| AC-5 | GREEN (자동) | client unit tests | UB1 saveTokens falsy 차단 |
| AC-6 | GREEN (자동) | client unit tests | UB2 loadTokens 자가 회복 |
| AC-7 | GREEN (자동) | server unit tests | RefreshResponse 비대칭 보존 |

수동 검증 게이트(AC-3, AC-4)는 **main 머지 이전 필수**이며 사용자가 직접 브라우저 환경에서 실행한다.

### 9.4 품질 게이트

- **LSP baseline 회귀**: 0건 (`max_new=0` 정책 충족, `go vet` / `gofmt` / `tsc --noEmit` / `eslint` 모두 신규 진단 0).
- **TRUST 5**: 5/5 PASS (Tested / Readable / Unified / Secured / Trackable).
- **자동화 테스트**: 906 tests GREEN (server + client 전체).
- **R-6 보안 검증**: console 출력 내 raw 토큰 누출 0건 (UB1/UB2 가드의 truncation 검증 통과).

### 9.5 의존성 / 디렉터리 / 아키텍처 변경 없음

- **신규 의존성**: 0건 (`go.mod` 및 `web/package.json` 무변경 — `git diff HEAD~1 -- go.mod web/package.json` 로 확인).
- **신규 디렉터리**: 0개.
- **신규 아키텍처 패턴**: 0건 (Zustand store API 형태 무변경, Echo handler 인터페이스 무변경).

### 9.6 호환성 및 사용자 영향

- 본 패치 적용 시 **모든 활성 사용자 세션이 invalidation** 된다 (구 응답 형태로 저장된 broken localStorage state 는 UB2 가 자동 클린업하여 사용자 개입 없이 회복됨).
- 사용자는 재로그인이 필요하며, 재로그인 후에는 정상 인증 흐름이 복원된다.

### 9.7 Commit 및 브랜치

- **Commit**: `8bf49e0` — `fix(auth): SPEC-AUTH-004 — login 응답 {user, tokens} 정합 + authStore 자가 회복`
- **Branch**: `feature/SPEC-DASHBOARD-001` (유지, 별도 분기 없음)
- **Predecessor**: SPEC-AUTH-003 (`9ec8597`, `dad6010`)

