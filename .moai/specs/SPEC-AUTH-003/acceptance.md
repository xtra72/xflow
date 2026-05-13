---
spec_id: SPEC-AUTH-003
acceptance_version: 0.1.0
created: 2026-05-12
updated: 2026-05-12
author: xtra
---

## 1. 개요

본 문서는 SPEC-AUTH-003 의 5개 EARS 모듈(U1, S1, E1, UB1, O1) 및 회귀 방지 항목에 대한 8 개의 Given/When/Then 시나리오와 품질 게이트, 수동 검증 절차를 정의한다. 각 시나리오는 plan 의 마일스톤·테스트 항목과 트레이서빌리티가 일치한다.

---

## 2. 시나리오

### AC-1 — auth-disabled 환경에서 토큰 없이 정상 연결

- **EARS 매핑**: U1, S1 (`authEnabled === false` 행), Provider 마운트
- **Given**:
  - `useAuthStore` 가 `{ authEnabled: false, isAuthenticated: false, tokens: null }` 상태로 초기화 완료.
  - `WebSocketProvider` 가 아직 마운트되지 않음.
  - 모킹된 `WebSocket` 생성자가 `globalThis.WebSocket` 으로 설치됨.
- **When**:
  - `<WebSocketProvider>` 가 마운트됨.
- **Then**:
  - 모킹된 `WebSocket` 생성자가 정확히 1회 호출됨.
  - 호출 인자 URL 은 `ws://<host>/ws` (또는 https 컨텍스트면 `wss://<host>/ws`) 로, **`?token=` 쿼리 파라미터를 포함하지 않는다**.
  - `useWebSocket().state` 는 모킹된 `onopen` 발생 후 `connected` 로 전이.
- **And (회귀 방지)**:
  - `useAuthStore.getState().authEnabled` 값이 `false` 로 유지되는 한 추가 connect 호출 없음.

### AC-2 — auth-enabled, 초기화 미완료 (`authEnabled === null`)

- **EARS 매핑**: S1 (`authEnabled === null` 행)
- **Given**:
  - `useAuthStore` 초기 상태가 `{ authEnabled: null, isAuthenticated: false, tokens: null }` (즉, `authStore.initialize()` 가 아직 완료되지 않음).
  - 모킹된 `WebSocket` 생성자가 설치됨.
- **When**:
  - `<WebSocketProvider>` 가 마운트됨.
- **Then**:
  - 모킹된 `WebSocket` 생성자 호출 횟수 = **0**.
  - `useWebSocket().state` 는 `disconnected` 유지.
- **And (회귀 방지)**:
  - 마운트 후 50ms 이상 경과해도 connect 시도가 발생하지 않음 (race-free wait).

### AC-3 — auth-enabled + 인증 완료 → 토큰 동반 connect

- **EARS 매핑**: U1, S1 (`authEnabled === true && isAuthenticated && token` 행)
- **Given**:
  - `useAuthStore` 가 `{ authEnabled: true, isAuthenticated: true, tokens: { access_token: 'JWT_AAA' } }` 상태.
  - 모킹된 `WebSocket` 생성자 설치됨, `onopen` 을 정상 발생시키도록 설정.
- **When**:
  - `<WebSocketProvider>` 가 마운트됨.
- **Then**:
  - 모킹된 `WebSocket` 생성자가 정확히 1회 호출됨.
  - 호출 인자 URL 은 `ws://<host>/ws?token=JWT_AAA` (또는 https 컨텍스트면 `wss://...`).
  - `onopen` 발생 후 `useWebSocket().state` 는 `connected`.
- **And**:
  - `WSClient` 내부 `lastConnectAttemptHadToken === true`.
  - `WSClient` 내부 `connectionEverEstablished === true` (onopen 후).

### AC-4 — 로그아웃 시 disconnect, reconnect 시도 없음

- **EARS 매핑**: E1 (connect → disconnect 전이), S1 (`authEnabled === true && isAuthenticated === false` 행)
- **Given**:
  - AC-3 의 종료 상태 (connected, token JWT_AAA 사용 중).
- **When**:
  - 사용자가 로그아웃 → `useAuthStore.setState({ isAuthenticated: false, tokens: null })`.
- **Then**:
  - 활성 `WebSocket` 인스턴스의 `close()` 가 호출됨.
  - `useWebSocket().state` 가 `disconnected` 로 전이.
  - 이후 100ms 이내 신규 `WebSocket` 생성자 호출 횟수 = **0** (reconnect 시도 없음).
- **And (회귀 방지)**:
  - `WSClient` singleton 인스턴스 참조는 동일 (재생성되지 않음).

### AC-5 — 토큰 회전 시 무중단 reconnect (Optional 모듈, scope 정식 포함)

- **EARS 매핑**: O1, E1 (token A → token B 전이)
- **Given**:
  - AC-3 의 종료 상태 (connected, token JWT_AAA 사용 중).
  - `WSClient` 인스턴스 참조를 보존: `const oldRef = useWebSocket().client`.
- **When**:
  - refresh 흐름이 새 토큰을 발급 → `useAuthStore.setState({ tokens: { access_token: 'JWT_BBB' } })`.
- **Then**:
  - 직전 `WebSocket` 인스턴스의 `close()` 가 호출됨.
  - 신규 `WebSocket` 생성자가 정확히 1회 추가 호출됨, URL = `ws://<host>/ws?token=JWT_BBB`.
  - 신규 `onopen` 발생 후 `useWebSocket().state === connected`.
  - `useWebSocket().client === oldRef` (singleton 인스턴스 정체성 유지).
  - `WSClient` 내부 `lastFailureWasAuth === false` (자동 reset).
- **And**:
  - 사용자에게 추가 액션(페이지 새로고침, 재로그인)이 요구되지 않음.

### AC-6 — 서버 401 (1006 close) 시 무한 재연결 차단

- **EARS 매핑**: UB1
- **Given**:
  - `useAuthStore` 가 `{ authEnabled: true, isAuthenticated: true, tokens: { access_token: 'INVALID_JWT' } }` 상태.
  - 모킹된 `WebSocket` 생성자가 다음과 같이 설정됨:
    - `onopen` 을 발생시키지 않음.
    - 생성 직후 `onerror` 이벤트 발생.
    - 직후 `onclose` 이벤트 with `{ wasClean: false, code: 1006, reason: '' }`.
- **When**:
  - `<WebSocketProvider>` 가 마운트됨.
- **Then**:
  - 모킹된 `WebSocket` 생성자가 정확히 **1회** 호출됨 (재시도 없음).
  - 호출된 URL 은 `?token=INVALID_JWT` 포함.
  - `WSClient` 내부 `lastFailureWasAuth === true`.
  - `useWebSocket().state` 는 `disconnected`.
- **And (음성 검증 — 무한 재시도가 발생하지 않음을 보장)**:
  - 마운트 후 1000ms 경과 시점까지 `WebSocket` 생성자 호출 횟수가 1회를 초과하지 않음.
- **And (회복 경로 — UB1 ↔ E1 연계)**:
  - 이후 토큰이 `'VALID_JWT'` 로 변경되면 (`useAuthStore.setState(...)`) **자동으로** 신규 connect 시도가 발생함 (`?token=VALID_JWT`). 단 본 케이스의 검증은 토큰 변경 직후 connect 발생 여부까지로 한정.

### AC-7 — 브라우저 탭 visibility hidden 중 기존 visibility-pause 동작 보존 (회귀 방지)

- **EARS 매핑**: 회귀 방지 (`wsClient.ts` 의 `visibilityHandler` 보존)
- **Given**:
  - AC-3 의 종료 상태 (connected).
  - `document.visibilityState` 모킹 가능 환경.
- **When**:
  - `document.visibilityState` 가 `'hidden'` 으로 변경되고 `visibilitychange` 이벤트가 dispatch 됨.
- **Then**:
  - 기존 visibility-pause 로직이 그대로 실행됨 (현행 동작 보존; 본 SPEC 은 이를 변경하지 않음).
- **And**:
  - visibility 가 `'visible'` 로 복귀하더라도, 본 SPEC 의 새 auth-gated reconnect 정책이 visibility 복귀로 인한 reconnect 와 충돌을 일으키지 않는다 (auth 상태가 유효하면 정상 reconnect, 무효하면 차단).

### AC-8 — React 19 StrictMode 이중 마운트 시 단일 활성 connect

- **EARS 매핑**: 회귀 방지 (StrictMode 안전성)
- **Given**:
  - AC-3 의 초기 상태 (`authEnabled: true, isAuthenticated: true, tokens: { access_token: 'JWT_AAA' }`).
  - `<React.StrictMode>` 로 `<WebSocketProvider>` 를 감싼 테스트 트리.
- **When**:
  - 트리가 마운트됨 (StrictMode 이중 마운트 발생: mount → unmount → mount).
- **Then**:
  - 마운트 완료 시점에서 활성 `WebSocket` 인스턴스 수 = **정확히 1**.
  - cleanup 단계에서 첫 인스턴스의 `close()` 가 호출되었음이 보장됨.
  - 두 번째 마운트의 `WSClient` 가 정상 connect 상태 유지.
- **And**:
  - 동일 시퀀스를 100회 반복해도 활성 `WebSocket` 수가 1을 초과하지 않음 (안정성 회귀).

---

## 3. 품질 게이트 기준

### 3.1 TRUST 5 컴플라이언스

| 축 | 기준 | 검증 방법 |
|----|------|-----------|
| **T**ested | 신규 코드 라인 커버리지 ≥ 85%, 모든 acceptance 시나리오 GREEN | `pnpm test --coverage` (`web/` 디렉토리), CI 리포트 확인 |
| **R**eadable | 신규 함수/변수명이 의도를 명확히 표현 (예: `evaluateGate`, `lastFailureWasAuth`, `tokenGetter`), 한국어 주석으로 비자명 로직 설명 | 코드 리뷰 + ESLint readability rules |
| **U**nified | `pnpm lint` 0 warning, `pnpm tsc --noEmit` 0 error, 기존 코드 스타일 (Prettier/Biome) 일관성 유지 | CI lint/typecheck job |
| **S**ecured | 토큰이 console.log/에러 메시지에 노출되지 않음, query token 전달은 기존 서버 설계와 일치 | 코드 리뷰 (grep `access_token` in console/throw) |
| **T**rackable | commit 메시지에 `SPEC-AUTH-003` 참조, 본 SPEC 트레이서빌리티 표 일치 | git log 검사 |

### 3.2 회귀 방지 기준

- 기존 `web/` 테스트 스위트 (변경 전 GREEN 상태) → **변경 후에도 0건 실패**.
- LSP baseline (Phase 2.5 캡처) 대비:
  - 신규 TypeScript errors: **0**
  - 신규 type errors: **0**
  - 신규 ESLint errors: **0**
  - ESLint warnings 증가: **≤ 0** (증가 금지)

### 3.3 Definition of Done (acceptance 수준)

- AC-1 ~ AC-8 모두 자동화 테스트로 GREEN.
- TRUST 5 모든 축 통과.
- 회귀 방지 기준 모두 통과.
- §4 수동 검증 절차 모든 단계 PASS.

---

## 4. 수동 검증 절차 (`basic_auth: true` 환경)

CI 자동화 외에 실제 브라우저에서 결함 회복을 확인하는 단계별 절차. 검증자는 DevTools 의 **Network** 탭 (필터: `WS`) 과 **Console** 탭을 동시에 열어둔다.

### 4.1 사전 준비

- 서버 설정: `basic_auth: true` 활성화. 본 플래그가 `false` 면 결함 자체가 표면화되지 않으므로 반드시 `true` 인 환경 사용.
- 브라우저: Chromium 계열 권장 (DevTools Network 탭의 WS frame inspector 지원).
- 사전 정리: 브라우저 storage 초기화 (`localStorage`, `sessionStorage`, cookies) — 깨끗한 미인증 상태에서 시작.

### 4.2 단계별 검증

#### Step 1 — 비로그인 페이지 진입 (`authEnabled === null` → `true` 초기화)

1. 브라우저에서 dashboard URL 접속.
2. **DevTools Network → WS** 탭 관찰:
   - **기대**: `authStore.initialize()` 완료 전까지 `/ws` 요청이 발생하지 않음.
   - **불합격 신호**: 로그인 화면 진입 즉시 `/ws` 요청이 401 로 거절되는 모습 → AC-2 위반.

#### Step 2 — 로그인 → WS 연결

1. 정상 자격증명으로 로그인.
2. DevTools Network → WS 탭 관찰:
   - **기대**: `/ws?token=<jwt>` 요청 1회 발생 → `101 Switching Protocols` 응답.
   - WS frame inspector 에서 메시지가 정상 송수신됨.
   - **불합격 신호**: `?token=` 누락된 요청 발생 → U1 위반. 401 응답 후 즉시 재시도 → UB1 위반.

#### Step 3 — 로그아웃 → 자동 disconnect

1. 로그아웃 버튼 클릭.
2. DevTools Network → WS 탭 관찰:
   - **기대**: 활성 WS 연결이 close 됨 (`Close frame` 또는 connection 종료 표시).
   - 이후 추가 `/ws` 요청 발생 횟수 = **0**.
   - **불합격 신호**: 로그아웃 후에도 `/ws` 재시도가 반복됨 → AC-4 위반.

#### Step 4 — 재로그인

1. 동일하게 다시 로그인.
2. DevTools Network → WS 탭 관찰:
   - **기대**: 새 토큰을 포함한 `/ws?token=<jwt-new>` 요청 1회, 정상 101.
   - **불합격 신호**: 직전 (로그아웃 전) 토큰이 그대로 사용됨 → 토큰 캐싱 결함, U1 위반.

#### Step 5 — 토큰 만료 후 refresh

1. 액세스 토큰의 짧은 만료 시간을 기다리거나, DevTools Console 에서 `useAuthStore.setState({ tokens: { access_token: 'STALE_TOKEN_FOR_TEST', ... } })` 로 인위적으로 무효 토큰 주입.
2. DevTools Network → WS 탭 관찰:
   - **A) 정상 refresh 흐름**: `authService` 가 새 토큰을 발급하면 `tokens.access_token` 변경 → WS 가 자동으로 disconnect → 신규 토큰으로 reconnect (`/ws?token=<jwt-refreshed>` 101).
   - **기대**: 사용자에게 페이지 새로고침이나 재로그인을 요구하지 않음 (AC-5).
   - **불합격 신호**: 토큰 회전 후에도 옛 토큰으로 401 재시도 무한 반복 → O1 또는 R-1 위반.
3. **B) refresh 가 실패하여 토큰이 사라지는 경우** (`useAuthStore.setState({ isAuthenticated: false, tokens: null })`):
   - **기대**: WS 가 disconnect 되고, 추가 reconnect 시도가 발생하지 않음 (AC-4 와 동일 동작).

#### Step 6 — 인위적 인증 실패 시나리오 (UB1 직접 검증)

1. DevTools Console 에서 `useAuthStore.setState({ authEnabled: true, isAuthenticated: true, tokens: { access_token: 'OBVIOUSLY_INVALID' } })` 실행 (정상 토큰을 의도적으로 무효 토큰으로 교체).
2. DevTools Network → WS 탭 관찰:
   - **기대**: `/ws?token=OBVIOUSLY_INVALID` 요청 1회 → 401 응답 → 추가 재시도 발생하지 않음.
   - 이후 1분 이상 관찰해도 추가 `/ws` 요청 0건.
   - **불합격 신호**: 동일 무효 토큰으로 초당 수회 재시도 발생 → UB1 위반.
3. 회복 검증: Console 에서 `useAuthStore.setState({ tokens: { access_token: '<original-valid-jwt>' } })` 실행 → 자동으로 신규 connect 발생 확인 (E1 회복 경로).

### 4.3 합격 판정

- §4.2 의 Step 1 ~ Step 6 모두에서 "기대" 동작이 확인되고 "불합격 신호" 가 한 건도 관찰되지 않은 경우 수동 검증 PASS.
- Step 5 의 정상 refresh 흐름 (A) 을 환경상 재현하기 어려운 경우 (B) 또는 Step 6 의 인위적 회전으로 대체 검증 가능.
