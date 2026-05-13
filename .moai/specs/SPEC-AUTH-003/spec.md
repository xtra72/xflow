---
id: SPEC-AUTH-003
title: WebSocket 핸드셰이크 JWT 전달 및 재연결 게이팅
version: 0.1.0
status: draft
created: 2026-05-12
updated: 2026-05-12
author: xtra
priority: high
domain: auth
related_specs:
  - SPEC-DASHBOARD-001
  - SPEC-AUTH-001
  - SPEC-AUTH-002
lifecycle_level: spec-first
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 0.1.0 | 2026-05-12 | xtra | 최초 작성 — `SPEC-DASHBOARD-001 v0.2.0` (`basic_auth: true`) 도입으로 표면화된 WebSocket 클라이언트 토큰 전달 누락 결함을 SPEC으로 정식화. 서버 측 핸드셰이크는 이미 정상 동작하므로 frontend(`web/`) 한정 수정 범위. |

---

## 1. 개요 (Overview)

### 1.1 목적

`SPEC-DASHBOARD-001 v0.2.0`이 `basic_auth: true`를 기본 정책으로 도입한 이후, 프런트엔드 `WSClient`가 WebSocket 핸드셰이크에 JWT를 전혀 전달하지 않아 발생하는 **무한 401 재연결 폭주 결함**을 제거한다. 본 SPEC은 다음 세 축을 동시에 정의한다.

1. WebSocket 연결 시점에 항상 `authStore` 최신 상태를 평가하여 토큰을 핸드셰이크에 실어 보낸다.
2. `authEnabled` × `isAuthenticated` × `tokens.access_token` 조합에 따라 connect/대기/disconnect 중 단일 행동만 결정한다.
3. 인증 실패(`onerror` → `onclose` with `code: 1006`)에 대해서는 동일한 토큰으로 자동 재연결을 시도하지 않고, 토큰 변경 또는 명시적 사용자 액션을 기다린다.

본 SPEC의 산출물은 frontend 단독 수정으로 server 측 변경은 발생하지 않는다.

### 1.2 배경

#### 1.2.1 결함 표면화 경로

- `9dcc69c` 핫픽스(`fix(web): SPEC-DASHBOARD-001 v0.2.0 — 401/500 무한 PUT 루프 차단 (hotfix)`)는 REST PUT 경로의 한 갈래만 차단했고, WebSocket 인증 경로의 무한 재시도는 그대로 남아 있다.
- `basic_auth: false` 환경에서는 서버가 토큰 검증을 건너뛰므로 결함이 외형적으로 드러나지 않았으나, v0.2.0 이후 기본값이 `true`로 전환되며 즉시 발생.

#### 1.2.2 코드 레벨 근거

| 위치 | 현 동작 | 결함 |
|------|---------|------|
| `web/src/services/ws/wsClient.ts:264-268` | `createWSClient()`가 모듈 평가 시점에 `window.location` 만으로 URL을 1회 계산해 인스턴스에 고정한다. | URL이 미리 굳어 있어 이후 토큰을 query/header 어느 쪽으로도 주입할 통로가 없다. 브라우저 표준상 `new WebSocket()`은 커스텀 헤더를 지원하지 않으므로 query 전달이 유일한 수단이지만 그 경로가 부재. |
| `web/src/hooks/useWebSocket.ts:36-50` | `WebSocketProvider`의 `useEffect`가 마운트 즉시 `ws.connect()`를 호출하며 `useAuthStore`를 구독하지 않는다. | auth 상태와 무관하게 항상 connect 시도 → `basic_auth: true`에서는 무조건 401. |
| `web/src/stores/authStore.ts` | `authEnabled` (`boolean | null`, `null`은 "아직 확인 전"), `isAuthenticated`, `tokens?.access_token` 보유. | WS 계층이 이 상태를 전혀 참조하지 않아 정보 격리. |
| `internal/api/handler/websocket.go:55-87` | `Authorization: Bearer <jwt>` 헤더 또는 `?token=<jwt>` 쿼리를 검증하고 실패 시 `http.Error(w, "Unauthorized", http.StatusUnauthorized)`. | **서버는 정상이며 본 SPEC의 수정 대상이 아님.** |

#### 1.2.3 결함의 관찰 가능한 증상

- `basic_auth: true` 상태에서 사용자가 페이지를 열면 서버 로그에 `WebSocket 인증 실패: 토큰 없음` 경고가 초당 수회 누적된다.
- 브라우저 DevTools Network 탭에서 동일 `/ws` 요청이 401로 거절되는 직후 `WSClient.scheduleReconnect()`가 발동되어 동일한 tokenless URL로 재시도가 무한 반복된다.
- 정상 로그인 직후에도 `WSClient` 인스턴스 URL이 그대로이므로 토큰을 전달할 방법이 없어 결함이 자가 회복되지 않는다.

### 1.3 비범위 (Out of Scope)

- 서버 측 WebSocket 인증 로직 변경 — 이미 정상.
- Token refresh 로직 자체의 신규 구현 — 기존 `authService` refresh 흐름 결과(`access_token` 값 변경)에만 반응한다.
- WebSocket 메시지 프로토콜, heartbeat, backpressure 등 비-인증 영역.
- 새로운 인증 모드(OAuth, mTLS 등) 또는 신규 endpoint 추가.
- Hotfix 커밋(`9dcc69c`)이 이미 처리한 REST PUT 401/500 루프 — 본 SPEC은 WS 경로에만 한정.

---

## 2. EARS 요구사항

본 SPEC은 5개의 EARS 모듈로 구성된다. 각 모듈은 독립적으로 테스트 가능하며, 식별자(`U1`, `S1`, `E1`, `UB1`, `O1`)는 트레이서빌리티 표 및 acceptance 시나리오에서 직접 참조된다.

### 2.1 [U1] (Ubiquitous) WebSocket 핸드셰이크 인증 정책

시스템은 항상 WebSocket 연결을 시도하는 시점에 `authStore`의 현재 상태를 다시 평가하여, 유효한 `access_token`이 존재할 경우 `?token=<access_token>` 쿼리 파라미터로 JWT를 핸드셰이크 요청 URL에 포함해야 한다.

**상세 규약**

- 토큰 전달 채널은 query parameter `?token=<jwt>` 단일 채널로 한정한다 (브라우저 `WebSocket` API가 custom request header를 지원하지 않기 때문).
- URL 평가는 모듈 평가 시점이 아니라 **각 connect 시도 시점**에 수행되어야 한다.
- 토큰 값은 `useAuthStore.getState().tokens?.access_token`을 통해 매 시도마다 최신 값을 조회한다.
- `?token=` 외의 기존 query parameter가 있을 경우 보존되어야 한다 (현재 코드에는 없으나 미래 확장성을 위함).
- 토큰이 부재(`undefined`/`""`)하면 query를 추가하지 않으며, 이는 `authEnabled === false` 분기에서만 도달 가능하다.

### 2.2 [S1] (State-Driven) 연결 게이트 매트릭스

`WHILE` `authEnabled` × `isAuthenticated` × `tokens?.access_token` 조합이 다음 매트릭스에 정의된 상태에 있는 동안, `WebSocketProvider`는 매트릭스가 지시하는 정확히 하나의 상태(connect / wait / disconnect)만을 유지해야 한다.

| `authEnabled` | `isAuthenticated` | `tokens?.access_token` | 행동 |
|---------------|-------------------|------------------------|------|
| `null` | (any) | (any) | **wait** — connect 시도 금지, 기존 연결이 있으면 유지하지 않고 disconnect 보류(아직 결정할 수 없음) |
| `false` | (any) | (any) | **connect (no token)** — 토큰 없이 즉시 connect |
| `true` | `false` | (any) | **disconnect** — 인증되지 않음, 연결 유지 금지 |
| `true` | `true` | falsy | **disconnect** — 토큰 부재, 연결 시도 금지 |
| `true` | `true` | truthy | **connect (with token)** — `?token=<jwt>` 포함하여 connect |

**상세 규약**

- `authEnabled === null` 상태는 `authStore.initialize()` 완료 전이며, 이 구간에서는 어떠한 connect 시도도 발생해서는 안 된다.
- 매트릭스 행동은 connect/disconnect 명령 형태로 표현되며, `WSClient` 인스턴스 자체는 컴포넌트 트리 생애주기 동안 단일 인스턴스로 유지되어야 한다.

### 2.3 [E1] (Event-Driven) authStore 변경 → WS 라이프사이클 트리거

`WHEN` `useAuthStore` 의 `authEnabled`, `isAuthenticated`, 또는 `tokens.access_token` 중 하나라도 값이 변경되면, `THEN` `WebSocketProvider`는 변경 후 상태를 [S1]의 매트릭스에 다시 적용하여 다음 중 하나의 동작을 수행해야 한다:

1. 현재 행동과 동일 → no-op.
2. wait/disconnect → connect 전이 → 신규 토큰으로 connect.
3. connect → disconnect 전이 → 즉시 disconnect, 자동 재연결 큐 비움.
4. connect (token A) → connect (token B) 전이 (토큰 회전) → 기존 연결 disconnect 후 즉시 신규 토큰으로 reconnect ([O1] 참조).

**상세 규약**

- 트리거 평가는 React effect 의 의존성 추적이 아닌 `useAuthStore.subscribe()` 콜백을 통해 이루어져야 하며(중간 re-render 누락 방지), 콜백 내부에서는 `WSClient` singleton 인스턴스를 통해 직접 명령한다.
- 트리거는 idempotent — 동일 상태로의 변경은 no-op이며 추가 connect/disconnect 비용을 유발하지 않는다.
- `WSClient` 인스턴스는 모든 트리거에 걸쳐 유지되며, 트리거가 인스턴스를 새로 생성하지 않는다.

### 2.4 [UB1] (Unwanted-Behavior) 인증 실패 시 무한 재연결 차단

서버가 인증 실패로 핸드셰이크를 거절한 경우(`onerror` 직후 `onclose` 이벤트, `wasClean: false`, `code: 1006`, 직전 시도가 인증 토큰을 동반했으나 거절된 정황), 클라이언트는 동일한 `access_token` 값으로의 자동 재연결을 시도하지 **않아야 한다**. 클라이언트는 `tokens.access_token` 값이 변경되거나 사용자가 명시적으로 reconnect를 요청할 때까지 대기 상태(`disconnected` 또는 신규 `auth_failed` 상태)를 유지해야 한다.

**인증 실패 close code 매핑 — 중요한 제약**

서버 인증 실패는 표준 WebSocket close frame이 아니라 **HTTP 401 응답**으로 종료된다. 이는 `internal/api/handler/websocket.go:55-95`의 다음 흐름에서 비롯된다:

```go
// 토큰 검증 실패 시
http.Error(w, "Unauthorized", http.StatusUnauthorized)
return
// upgrader.Upgrade()에 도달하지 않음 → WebSocket 핸드셰이크 미완료
```

이는 client 측에서 다음 형태로 관찰된다:

- `onerror` 이벤트 발생 (브라우저는 보안상 HTTP status code 를 JS에 직접 노출하지 않음).
- 직후 `onclose` 이벤트 with `event.wasClean === false`, `event.code === 1006`, `event.reason === ''`.

**클라이언트 인식 규약**

- "인증 실패로 추정되는 close" = "직전 connect 시도가 토큰을 포함하여 발송되었음에도 `1006/abnormal closure`로 즉시 종료되었고, `onmessage` 이벤트가 한 번도 발생하지 않은 경우".
- 이 조건이 일치하는 close 발생 시:
  - `scheduleReconnect()` 호출 금지.
  - 상태를 `disconnected` 로 surface 하고, 내부 플래그 `lastFailureWasAuth = true` 를 기록.
  - 다음 connect 시도는 [E1]의 토큰 변경 트리거 또는 사용자 명시 액션에 의해서만 가능.
- 일반 네트워크 장애로 인한 close(예: 토큰을 정상 동반하여 이미 한 번이라도 메시지를 주고받은 후 `1006`)는 기존 reconnect 정책을 그대로 따른다 (auth-failure 와 명확히 구분).

### 2.5 [O1] (Optional) 토큰 회전 시 무중단 재연결

가능하면, refresh 등으로 `tokens.access_token` 값이 회전된 경우 `WSClient` singleton 인스턴스의 정체성을 유지하면서 새 토큰으로 매끄럽게 reconnect를 수행하여, 사용자에게 추가 액션(페이지 새로고침, 재로그인)을 요구하지 않아야 한다.

**상세 규약 — 본 SPEC의 acceptance scope 에 정식 포함**

- "Optional" EARS 분류는 유지되나(시스템 핵심 기능이 아닌 사용성 향상이라는 의미), acceptance 시나리오에는 정식으로 포함되며 [UB1]의 무한 재시도 차단 정책과 충돌하지 않도록 구현되어야 한다.
- 토큰 회전 트리거: `useAuthStore` 의 `tokens.access_token` 값이 `null/undefined → string`, 또는 `string A → string B (A !== B)` 로 전이.
- 회전 검출 시 `WSClient` 인스턴스를 **재생성하지 않고** `disconnect()` → `connect()` (신규 URL 평가) 순으로 호출하여, 외부 코드(`useWebSocket()` 훅 사용처)가 보유한 client 참조의 안정성을 보장한다.
- 회전으로 인한 reconnect 는 [UB1] 의 `lastFailureWasAuth` 플래그를 자동 클리어한다 (auth 상태가 명시적으로 변경되었으므로).

---

## 3. 트레이서빌리티 표

각 EARS 모듈을 영향 파일 및 acceptance 시나리오에 매핑한다.

| 모듈 ID | 요약 | 영향 파일 | 신규/수정 | 관련 acceptance |
|---------|------|-----------|-----------|-----------------|
| U1 | 핸드셰이크 토큰 전달 정책 | `web/src/services/ws/wsClient.ts` | 수정 | AC-1, AC-3 |
| S1 | 연결 게이트 매트릭스 | `web/src/hooks/useWebSocket.ts` | 수정 | AC-1, AC-2, AC-3, AC-4 |
| E1 | authStore 변경 트리거 | `web/src/hooks/useWebSocket.ts` | 수정 | AC-4, AC-5 |
| UB1 | 인증 실패 시 무한 재연결 차단 | `web/src/services/ws/wsClient.ts` | 수정 | AC-6 |
| O1 | 토큰 회전 시 무중단 reconnect | `web/src/hooks/useWebSocket.ts` (+ `wsClient.ts` 보조) | 수정 | AC-5 |
| 회귀 방지 | 기존 visibility-pause 동작 보존 | `web/src/services/ws/wsClient.ts` | 보존 | AC-7 |
| 회귀 방지 | StrictMode 이중 마운트 안전성 보존 | `web/src/hooks/useWebSocket.ts` | 보존 | AC-8 |
| 신규 단위 테스트 | wsClient 동작 검증 | `web/src/services/ws/wsClient.test.ts` | 신규 | AC-1, AC-3, AC-6 |
| 신규 통합 테스트 | Provider/hook 동작 검증 | `web/src/hooks/useWebSocket.test.tsx` | 신규 | AC-2, AC-4, AC-5, AC-8 |

---

## 4. 인증 실패 close code 매핑 (상세)

본 절은 [UB1] 구현 시 클라이언트가 "인증 실패"를 어떻게 식별하는지 정의한다. 표준화된 별도 close code(예: `4401`)가 서버에서 사용되지 않기 때문에 본 절이 필요하다.

### 4.1 서버 거동 (변경 없음, 참고용)

`internal/api/handler/websocket.go:55-95` 흐름:

1. `h.authEnabled && h.jwtSvc != nil` → 토큰 추출 (Authorization header → `?token=` query).
2. 토큰 없음 / blacklist / `ValidateToken()` 실패 중 하나라도 발생 시 `http.Error(w, "Unauthorized", http.StatusUnauthorized)` 후 `return`.
3. 이 경로에서는 `upgrader.Upgrade(w, r, nil)`이 호출되지 **않으므로** WebSocket close frame은 송신되지 않으며, 응답은 일반 HTTP 401 본문 `Unauthorized\n`.

### 4.2 클라이언트 관찰 결과

브라우저 표준 `WebSocket` API 거동:

- HTTP 401 응답 수신 → `WebSocket.readyState === CLOSED`로 전이.
- `onerror` 이벤트 발생 (event 객체에 status code 정보 없음 — 보안상 노출 금지).
- 직후 `onclose` 이벤트 with:
  - `event.wasClean === false`
  - `event.code === 1006` (Abnormal Closure)
  - `event.reason === ''`
- `onopen` / `onmessage` 는 한 번도 발생하지 않음.

### 4.3 클라이언트 식별 휴리스틱 ([UB1] 구현용)

`WSClient` 내부에 다음 상태를 유지한다:

- `lastConnectAttemptHadToken: boolean` — 직전 `connect()` 시도 시 `?token=` 가 URL에 포함되었는지.
- `connectionEverEstablished: boolean` — 해당 connect 시도 동안 `onopen` 이 한 번이라도 발생했는지.

`onclose` 핸들러에서 다음 조건이 모두 참이면 "인증 실패"로 판정:

- `event.code === 1006`
- `event.wasClean === false`
- `connectionEverEstablished === false`
- `lastConnectAttemptHadToken === true` (토큰을 보냈는데도 즉시 거절된 상황 — 토큰이 위조/만료/blacklist)

추가 케이스: `lastConnectAttemptHadToken === false` 인데 1006이 발생 = `basic_auth: true` 인 서버에 토큰 없이 연결을 시도한 경우. 이 경우도 동일하게 "인증 실패"로 판정하여 무한 재시도를 차단한다.

판정 결과가 "인증 실패"이면:

- `scheduleReconnect()` 호출하지 않음.
- 내부 플래그 `lastFailureWasAuth = true` 기록.
- 상태를 `disconnected` 로 emit (필요 시 향후 별도 `auth_failed` 상태 도입 검토 — 본 SPEC 범위 밖).

판정 결과가 "정상 close 또는 일반 네트워크 장애" 이면 기존 reconnect 정책 유지.

---

## 5. 비기능 요구사항

- **성능**: connect 시점의 URL 재계산 및 authStore 상태 조회는 모두 O(1) 동기 작업. 추가 네트워크 오버헤드 없음.
- **보안**: 토큰은 query parameter 로 전달되므로 서버 access log 에 기록될 수 있다. 이는 기존 서버 설계의 의도된 fallback (`internal/api/handler/websocket.go:64-67` 주석 "WebSocket 클라이언트용 폴백")을 따르는 것이며, 본 SPEC 의 수정 범위가 아니다. 향후 access log 마스킹은 별도 SPEC 으로 다룰 수 있다.
- **신뢰성**: 인증 실패 시 무한 재시도를 차단함으로써 서버 부하 및 로그 폭주를 방지한다.
- **테스트 가능성**: `WSClient` 는 `WebSocket` 글로벌을 직접 참조하지 않고 주입 가능한 형태로 유지되거나, Vitest 환경에서 `globalThis.WebSocket` 모킹이 가능해야 한다.
- **호환성**: React 19 + StrictMode 환경에서 Provider 이중 마운트 시에도 정확히 한 개의 활성 WS 연결만 유지.

---

## 6. 가정 및 제약

- `authStore` 는 이미 `authEnabled: boolean | null`, `isAuthenticated: boolean`, `tokens: { access_token: string } | null` 필드를 노출한다 (검증 완료, `web/src/stores/authStore.ts`).
- `authStore` 의 토큰 회전은 기존 `authService` refresh 흐름이 담당하며, 본 SPEC 은 그 결과(스토어 값 변경)에만 반응한다.
- 서버는 `Authorization: Bearer` 헤더와 `?token=` 쿼리 둘 다 지원하지만, 브라우저 `WebSocket` API 제약으로 클라이언트는 query 만 사용한다.
- 신규 npm dependency 도입은 금지. 단, jsdom 환경의 WebSocket 모킹을 위한 `mock-socket` 도입 여부는 plan 단계의 검토 항목으로 분리한다.
- 본 SPEC 은 `feature/SPEC-DASHBOARD-001` 브랜치 위에서 후속 작업으로 진행되며, 별도 브랜치 분기는 하지 않는다.
