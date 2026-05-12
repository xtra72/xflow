---
spec_id: SPEC-AUTH-003
plan_version: 0.1.0
created: 2026-05-12
updated: 2026-05-12
author: xtra
---

## 1. 개요

본 plan 은 SPEC-AUTH-003 (`spec.md`) 의 5개 EARS 모듈을 frontend-only 변경으로 구현하기 위한 마일스톤·기술 스택·의존성·위험 분석을 정의한다. 영향 파일은 정확히 4개로 한정되며, 신규 npm dependency 는 도입하지 않는다.

---

## 2. 기술 스택

| 항목 | 버전 / 도구 | 비고 |
|------|------------|------|
| Language | TypeScript 5.9 | `web/tsconfig.json` 기존 설정 유지 (strict mode) |
| UI Framework | React 19 | 기존 stack, Server Components 사용하지 않음 (client-only) |
| State Store | Zustand 5 | `useAuthStore` (`web/src/stores/authStore.ts`) — 기존 인스턴스 사용 |
| Test Runner | Vitest (현행 버전) | jsdom 환경 |
| WebSocket Mocking | (검토) `mock-socket` 또는 직접 `globalThis.WebSocket` 모킹 | M-5 착수 시 결정 |
| Lint / Type Check | ESLint 9 (또는 Biome), `tsc --noEmit` | 기존 명령 그대로 (`pnpm lint`, `pnpm tsc --noEmit`) |
| Package Manager | pnpm | 기존 (`web/pnpm-lock.yaml`) |

**Production-stable 만 사용** — beta/alpha 패키지 도입 금지. 검토 항목인 `mock-socket` 도 도입 시점에 latest stable 버전(현재 9.x 계열)만 채택한다.

---

## 3. 마일스톤 (M-1 ~ M-7)

각 마일스톤은 정확히 한 파일 그룹에 매핑되며, 이전 마일스톤 완료 후 다음 마일스톤이 시작된다 (의존성 sequential).

### M-1 — `wsClient.ts` URL 평가 시점 지연 + 토큰 getter 콜백 지원

- 파일: `web/src/services/ws/wsClient.ts`
- EARS 매핑: U1
- 변경 요약:
  - `createWSClient()` 의 시그니처를 `createWSClient(options?: { url?: string; tokenGetter?: () => string | undefined })` 형태로 확장.
  - 모듈 평가 시점의 URL 1회 계산을 제거하고, `WSClient.connect()` 내부에서 매번 `tokenGetter?.()` 호출 → query 조립 → URL 결정.
  - base URL (`window.location` 기반) 계산은 `connect()` 시점으로 이동.
  - `tokenGetter` 가 truthy 값을 반환하면 `?token=<jwt>` 를 query 에 추가, 부재 시 query 없이 연결.
  - 기존 호출자(`createWSClient()` 인자 없음 호출)와의 backward compatibility 유지 — `tokenGetter` 미제공 시 토큰 없이 연결 (기존 동작과 동일).
- 완료 기준: U1 단위 테스트(M-5 의 일부)가 GREEN.

### M-2 — `wsClient.ts` 인증 실패 close code 인식 + 재연결 억제

- 파일: `web/src/services/ws/wsClient.ts`
- EARS 매핑: UB1
- 변경 요약:
  - `WSClient` 내부 상태에 `lastConnectAttemptHadToken: boolean`, `connectionEverEstablished: boolean`, `lastFailureWasAuth: boolean` 필드 추가.
  - `connect()` 진입 시 `lastConnectAttemptHadToken` 갱신, `connectionEverEstablished` 를 `false` 로 reset.
  - `onopen` 핸들러에서 `connectionEverEstablished = true`, `lastFailureWasAuth = false`.
  - `onclose` 핸들러에서 `spec.md §4.3` 의 휴리스틱으로 인증 실패 판정.
  - 인증 실패 판정 시 `scheduleReconnect()` 호출 금지, `lastFailureWasAuth = true` 기록, 상태를 `disconnected` 로 emit.
  - 외부에서 인증 실패 상태를 알 수 있도록 `onAuthFailure(cb)` 또는 기존 `onStateChange` 에 부수 정보 전달 형태 중 하나 채택 (M-2 착수 시 결정, 기본은 후자로 단순화).
- 완료 기준: UB1 단위 테스트(M-5 의 일부)가 GREEN.

### M-3 — `useWebSocket.ts` `WebSocketProvider` authStore 구독 + 게이트 매트릭스 구현

- 파일: `web/src/hooks/useWebSocket.ts`
- EARS 매핑: S1, E1 (트리거의 connect/disconnect 결정 부분), U1 (tokenGetter 주입)
- 변경 요약:
  - `WebSocketProvider` 의 `useEffect` 에서 `createWSClient({ tokenGetter: () => useAuthStore.getState().tokens?.access_token })` 로 변경.
  - 마운트 시 즉시 `ws.connect()` 호출 제거 — 대신 게이트 매트릭스(`spec.md §2.2`) 평가 함수 `evaluateGate()` 도입.
  - `evaluateGate()` 가 `'wait' | 'connect' | 'disconnect'` 중 하나를 반환.
  - 마운트 직후 한 번 `evaluateGate()` 실행, 추가로 `useAuthStore.subscribe()` 등록하여 store 변경 시마다 재평가.
  - 평가 결과에 따라 `ws.connect()` 또는 `ws.disconnect()` 호출 (no-op 케이스는 호출 생략).
  - cleanup 함수에서 `useAuthStore.subscribe` 해제 + `ws.disconnect()`.
- 완료 기준: S1, E1 의 connect/disconnect 분기 통합 테스트(M-6 의 일부)가 GREEN.

### M-4 — `useWebSocket.ts` 토큰 회전 시 무중단 reconnect 구현

- 파일: `web/src/hooks/useWebSocket.ts`
- EARS 매핑: O1, E1 (토큰 회전 분기)
- 변경 요약:
  - `useAuthStore.subscribe()` 콜백에서 직전 `tokens?.access_token` 값을 클로저로 보존.
  - 신규 값이 직전 값과 다르면서 둘 다 truthy 인 경우 → `ws.disconnect()` 즉시 호출 후 `ws.connect()` (URL 은 자동으로 신규 토큰 반영).
  - `WSClient` singleton 인스턴스는 재생성하지 않음 (`useState` 또는 `useRef` 로 보존).
  - 토큰 회전으로 인한 reconnect 는 `WSClient` 내부 `lastFailureWasAuth` 플래그를 자동 reset (M-2 의 인터페이스 사용).
- 완료 기준: O1 통합 테스트(M-6 의 일부)가 GREEN.

### M-5 — `wsClient.test.ts` (신규) — 단위 테스트

- 파일: `web/src/services/ws/wsClient.test.ts` (신규)
- EARS 매핑: U1, UB1
- 테스트 항목:
  - U1-T1: `tokenGetter` 미제공 시 base URL 만으로 연결 (auth-disabled).
  - U1-T2: `tokenGetter` 제공 시 `?token=<jwt>` 가 URL 에 포함됨.
  - U1-T3: 매 connect 시도마다 `tokenGetter` 가 재호출되어 최신 토큰을 반영.
  - UB1-T1: `onclose` with `code: 1006`, `connectionEverEstablished: false`, `lastConnectAttemptHadToken: true` → reconnect 호출되지 않음, `lastFailureWasAuth: true`.
  - UB1-T2: `onclose` with `code: 1006`, `connectionEverEstablished: true` (한 번이라도 onopen 발생) → 기존 reconnect 정책 적용.
  - UB1-T3: `onclose` with `code: 1006`, `lastConnectAttemptHadToken: false` (토큰 없이 시도) → 인증 실패 판정, reconnect 호출되지 않음.
- WebSocket mocking 전략: `mock-socket` 도입 여부를 본 마일스톤 착수 시 평가. 1차 시도는 `globalThis.WebSocket` 을 직접 vi.fn 기반으로 모킹하여 dependency 추가 회피. 모킹 복잡도가 높으면 `mock-socket` 도입.
- 완료 기준: 6 개 케이스 모두 GREEN, `wsClient.ts` 신규/수정 코드 라인 커버리지 ≥ 85%.

### M-6 — `useWebSocket.test.tsx` (신규) — Provider/hook 통합 테스트

- 파일: `web/src/hooks/useWebSocket.test.tsx` (신규)
- EARS 매핑: S1, E1, O1, 회귀(StrictMode)
- 테스트 항목:
  - S1-T1: `authEnabled === null` 마운트 → connect 시도 0회.
  - S1-T2: `authEnabled === false` 마운트 → connect 1회 (토큰 없음).
  - S1-T3: `authEnabled === true && isAuthenticated && token` 마운트 → connect 1회 (`?token=` 포함).
  - E1-T1: 마운트 시 `null` → 이후 store 가 `true + authenticated + token` 으로 전이 → connect 발생.
  - E1-T2: connect 상태에서 logout (`isAuthenticated: false`) → disconnect 발생, reconnect 시도 없음.
  - O1-T1: connect 상태에서 토큰 회전 (token A → token B) → disconnect → connect (token B), `WSClient` 인스턴스 동일 (참조 비교).
  - StrictMode-T1: `<StrictMode>` 래핑 후 마운트 → `WSClient` 활성 connect 정확히 1개.
- 테스트 도구: `@testing-library/react`, `act`, Zustand store 직접 조작 (`useAuthStore.setState(...)`).
- 완료 기준: 7 개 케이스 모두 GREEN, `useWebSocket.ts` 신규/수정 코드 라인 커버리지 ≥ 85%.

### M-7 — 문서화 메모

- 파일: 없음 (run 단계의 sync phase 입력용 메모)
- 변경 요약:
  - CHANGELOG hotfix 항목 작성용 한 줄 요약 메모 (예: `fix(web): WebSocket handshake now sends JWT and gates connect by auth state`).
  - README 변경 불필요 (사용자 경험상 동작은 자가 회복되며, 별도 사용자 액션 불필요).
- 완료 기준: 본 plan 의 [§7] 영향 파일 표가 정확히 반영됨.

---

## 4. 의존성

### 4.1 신규 패키지

- **신규 추가 금지** (가정 §6.4 / spec §6 참조).
- 단, jsdom 환경에서의 WebSocket 모킹 복잡도가 vi.fn 기반 직접 모킹으로 감당이 어려운 경우 `mock-socket` 도입을 M-5 착수 시 검토. 도입 결정 시 본 plan 을 0.2.0 으로 갱신.

### 4.2 기존 모듈 의존

- `web/src/stores/authStore.ts` — `useAuthStore` (구독, `getState()` 호출).
- `web/src/services/ws/wsClient.ts` — `WSClient`, `ConnectionState` 타입.
- `web/src/hooks/useWebSocket.ts` — `WebSocketProvider`, `useWebSocket()`.
- React 19 표준 hooks (`useEffect`, `useRef`, `useState`, `useCallback`, `useContext`, `createContext`).

### 4.3 SPEC 의존

- `SPEC-DASHBOARD-001` (v0.2.0): `basic_auth: true` 기본값 도입 — 본 SPEC 이 해결하는 결함의 surface 트리거.
- `SPEC-AUTH-001`, `SPEC-AUTH-002`: 인증 시스템 기반 (`authStore`, JWT, refresh 흐름).

---

## 5. 위험 분석 및 완화책

| # | 위험 요소 | 영향도 | 완화책 |
|---|----------|--------|--------|
| R-1 | 토큰 회전 레이스 컨디션 — refresh 중 reconnect 가 옛 토큰으로 시도 | High | M-1 의 `tokenGetter` 콜백이 connect 시점에 매번 `useAuthStore.getState()` 호출 → 항상 최신 스냅샷 사용. M-4 의 회전 트리거가 disconnect 후 즉시 connect 하므로 race window 최소화. |
| R-2 | React 19 StrictMode 이중 마운트로 다중 connect | High | M-3 의 cleanup 에서 정확히 disconnect, `useState` 또는 `useRef` 로 `WSClient` singleton 보존. M-6 의 StrictMode-T1 으로 회귀 검증. |
| R-3 | `authStore.initialize()` vs `WebSocketProvider` 마운트 경합 | Med | S1 매트릭스에 `authEnabled === null` → wait 분기를 명시. M-3 마운트 직후 첫 평가에서 `null` 이면 connect 호출 안 함. M-6 의 S1-T1, E1-T1 으로 검증. |
| R-4 | 브라우저 탭 visibility 인터랙션과 새 auth-gated reconnect 충돌 | Med | 기존 visibility-pause 로직(`wsClient.ts` 의 `visibilityHandler`)은 보존. M-3 의 disconnect 가 visibility-pause 와 무관하게 동작하도록 명령형 접근. acceptance AC-7 로 회귀 검증. |
| R-5 | 인증 실패 close code 식별 모호성 (서버가 표준 4xxx 코드를 안 씀) | High | spec §4 에서 휴리스틱 명문화. `lastConnectAttemptHadToken` + `connectionEverEstablished` + `code === 1006` 조합으로 식별. UB1-T1/T2/T3 으로 false positive/negative 검증. |
| R-6 | singleton vs 매 connect 새 인스턴스 — URL 만 변경 vs 인스턴스 교체 | Med | 본 plan 은 인스턴스 재사용 + connect 시점 URL 재평가 채택 (외부 참조 안정성 보장, O1 요구사항 충족). M-1 에서 인스턴스 재생성 없음 명시. |
| R-7 | jsdom 에 native `WebSocket` 부재 — Vitest mocking 복잡 | Med | M-5 착수 시 우선 vi.fn 기반 직접 모킹 시도, 실패 시 `mock-socket` 도입. 본 plan 0.2.0 갱신으로 결정 기록. |

---

## 6. LSP Baseline 캡처 (orchestrator 수행)

본 plan 작성자는 LSP baseline 을 직접 캡처하지 **않는다**. Phase 2.5 에서 orchestrator 가 다음 명령으로 캡처 후 `.moai/specs/SPEC-AUTH-003/lsp-baseline.json` 에 저장한다:

- TypeScript: `cd web && pnpm tsc --noEmit`
- ESLint: `cd web && pnpm lint`

baseline 캡처 결과는 run phase 의 quality gate (zero new errors, zero new type errors, zero new lint errors) 기준점으로 사용된다.

---

## 7. 영향 범위 (정확히 4개 파일)

| 파일 | 상태 | 변경 종류 |
|------|------|-----------|
| `web/src/services/ws/wsClient.ts` | 수정 | M-1 (URL 지연 + tokenGetter), M-2 (인증 실패 식별 + reconnect 억제) |
| `web/src/hooks/useWebSocket.ts` | 수정 | M-3 (게이트 매트릭스 + authStore 구독), M-4 (토큰 회전 reconnect) |
| `web/src/services/ws/wsClient.test.ts` | 신규 | M-5 (6 케이스 단위 테스트) |
| `web/src/hooks/useWebSocket.test.tsx` | 신규 | M-6 (7 케이스 통합 테스트) |

서버 측, 다른 frontend 모듈, 빌드/CI 설정, 의존성 manifest 모두 변경하지 않는다. (단, M-5 결과로 `mock-socket` 도입 결정 시 `web/package.json` devDependencies 1줄 추가 가능 — 본 plan 0.2.0 갱신 필요)

---

## 8. 우선순위 및 마일스톤 순서

- **Primary Goal**: M-1 → M-2 → M-3 (게이트 매트릭스 + 토큰 전달 + 무한 재시도 차단). 결함의 핵심 표면 증상 해소.
- **Secondary Goal**: M-4 (토큰 회전 무중단 reconnect). 사용성 향상.
- **Final Goal**: M-5 → M-6 (테스트 커버리지 ≥ 85%, 회귀 방지). TRUST 5 의 Tested 통과.
- **Optional Goal**: M-7 (CHANGELOG 메모) — sync phase 의 입력으로 사용.

순차 실행 권장. 단 M-5 와 M-6 는 각자의 대상 모듈 구현 완료(M-1/2 → M-5, M-3/4 → M-6) 후 병렬 실행 가능.

---

## 9. 완료 기준 (Definition of Done — plan 수준)

- 모든 acceptance 시나리오(AC-1 ~ AC-8) GREEN.
- LSP baseline 대비 신규 errors / type errors / lint errors 0 건.
- `web/src/services/ws/wsClient.ts` 및 `web/src/hooks/useWebSocket.ts` 의 변경 라인 커버리지 ≥ 85%.
- 기존 `web/` 테스트 스위트 회귀 0건.
- 본 plan 영향 범위 §7 외 파일 변경 0건.
