---
id: SPEC-AUTH-006
title: 사용자·역할 관리 화면 및 권한 기반 UI 게이팅
version: 0.1.0
status: draft
created: 2026-08-18
updated: 2026-08-18
author: xtra
priority: high
domain: auth
related_specs:
  - SPEC-AUTH-005
  - SPEC-AUTH-002
  - SPEC-WEB-005
lifecycle_level: spec-first
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 0.1.0 | 2026-08-18 | xtra | 최초 작성 — SPEC-AUTH-005 가 제공하는 권한 API를 소비해 사용자·역할 관리 화면을 추가하고, 고정 3역할 전제로 짜인 기존 UI 게이팅을 권한 키 기반으로 전환한다. |

---

## 1. 개요 (Overview)

### 1.1 목적

[SPEC-AUTH-005](../SPEC-AUTH-005/spec.md) 가 서버에 도입한 사용자·역할·권한 체계를 웹 대시보드에서 사용할 수 있게 한다.

1. **관리 화면**: 사용자 등록·역할 부여·비밀번호 재설정·삭제, 역할 생성·권한 조합 화면을 추가한다.
2. **게이팅 전환**: 현재 `role === 'admin'` 문자열 비교로 짜인 메뉴·라우트 게이팅을 **권한 키 기반**으로 바꾼다. 커스텀 역할이 생기면 역할 이름 열거는 성립하지 않는다.
3. **액션 게이팅**: 에이전트·디바이스·플로우 화면의 등록·수정·삭제·실행 버튼을 권한에 따라 숨기거나 비활성화한다.

UI 게이팅은 **보조 수단**이다. 실제 차단은 서버(SPEC-AUTH-005)가 수행하며, 본 SPEC은 "할 수 없는 일을 보여주지 않는" 사용성 계층이다.

### 1.2 배경

#### 1.2.1 이미 있는 게이팅 (재사용 대상)

프론트엔드에는 고정 3역할 전제의 RBAC 뼈대가 이미 있다.

| 자산 | 위치 | 현재 동작 |
|------|------|-----------|
| `UserRole = 'admin' \| 'editor' \| 'viewer'` | `web/src/types/auth.ts` | 3종 고정 유니온 |
| 사이드바 역할 필터 (`hasAccess`) | `web/src/components/layout/Sidebar.tsx:196` | `roles?: UserRole[]` 미지정이면 전원 허용 |
| 라우트 가드 | `web/src/router.tsx:196` | `<AuthGuard requireRole="admin" />` 로 `/admin` 보호 |
| 인증 스토어 | `web/src/stores/authStore.ts` | `user`, `tokens` 보유. 권한 개념 없음 |

즉 "메뉴가 무방비"인 상태는 아니다. 문제는 **표현력**이다 — 고정 3역할만 표현할 수 있고, 리소스별 기능 단위 구분이 불가능하며(`admin` 여부만 봄), 사용자 관리 화면 자체가 없다.

#### 1.2.2 전환이 필요한 이유

SPEC-AUTH-005 가 역할 CRUD를 도입하면 `operator` 같은 커스텀 역할이 생긴다. 현재 코드는 `roles: ['admin']` 처럼 역할 이름을 하드코딩하므로 커스텀 역할은 어떤 메뉴에도 접근할 수 없다. 역할 이름이 아니라 **권한 키**로 판단해야 한다.

### 1.3 비범위 (Out of Scope)

- 서버 측 권한 강제 — SPEC-AUTH-005
- 사용자 프로필·아바타·이메일 등 계정 정보 확장
- 권한 변경 이력 조회 화면(감사 로그 UI)
- 원격 노드(remote) 하위 화면의 세분 게이팅 — `remote.*` 단일 키로만 다룬다
- 다국어 확장: 신규 문구는 기존과 동일하게 ko/en 두 로케일만 추가한다

---

## 2. EARS 요구사항

### 2.1 [U1] (Ubiquitous) 권한 컨텍스트

시스템은 로그인 성공 및 세션 복원 시 `GET /api/v1/auth/me` 응답의 `permissions` 배열을 인증 스토어에 보관하고, 다음 판정 수단을 제공한다.

- `hasPermission(key: string): boolean` — 단일 권한 보유 여부
- `hasAnyPermission(keys: string[]): boolean` — 하나라도 보유
- 서버 인증이 비활성(`authEnabled === false`)이면 **모든 판정이 true** 를 반환한다. 서버가 검사하지 않는 상태에서 UI만 잠그면 기존 배포가 사용 불가가 된다.

`UserRole` 타입은 3종 유니온에서 문자열로 완화한다. 역할 이름은 더 이상 게이팅 판단에 쓰이지 않으며 표시 용도로만 남는다.

### 2.2 [U2] (Ubiquitous) 사용자·역할 관리 화면

시스템은 다음 두 화면을 제공한다.

**사용자 관리** (`/admin/users`, `user.read` 필요)

- 사용자 목록: username, 역할, 생성일, 수정일
- 등록: username, 비밀번호, 역할 선택(`role.read` 로 가져온 역할 목록)
- 역할 변경(인라인), 비밀번호 재설정, 삭제
- 서버가 409로 거부하는 잠금 방지 케이스(마지막 관리자 삭제·강등, 자기 자신 삭제)는 원인을 사용자 언어로 안내한다

**역할 관리** (`/admin/roles`, `role.read` 필요)

- 역할 목록: 이름, 설명, 빌트인 여부, 권한 개수
- 생성·수정: 권한 카탈로그(`GET /api/v1/permissions`)를 리소스×액션 행렬 체크박스로 표시
- 빌트인 역할은 수정·삭제 컨트롤을 비활성화하고 이유를 표시

### 2.3 [S1] (State-Driven) 메뉴·라우트 게이팅

**While** 사용자가 로그인한 상태인 동안, **the system shall** 권한이 없는 메뉴 항목을 숨기고 권한이 없는 경로로의 직접 진입을 차단한다.

- `NavItem` / `NavGroup` 의 `roles?: UserRole[]` 를 `permission?: string` 으로 교체한다. 그룹은 접근 가능한 하위 항목이 하나라도 있을 때만 표시한다(기존 동작 유지).
- `AuthGuard` 의 `requireRole` 을 `requirePermission` 으로 교체한다. 권한이 없으면 대시보드로 리다이렉트하고 안내를 표시한다(빈 화면 금지).
- 메뉴 ↔ 권한 매핑:

| 메뉴 | 권한 |
|------|------|
| 대시보드 | (없음 — 인증만) |
| 플로우 | `flow.read` |
| 에이전트 | `agent.read` |
| 디바이스 | `device.read` |
| 모니터링 | `monitoring.read` |
| 스케줄 | `schedule.read` |
| 노드 타입 / 에이전트 타입 | `node.read` |
| 관리 그룹(원격) | `remote.read` |
| 사용자 관리 | `user.read` |
| 역할 관리 | `role.read` |
| 설정 | `system.read` |

### 2.4 [E1] (Event-driven) 액션 컨트롤 게이팅

**When** 에이전트·디바이스·플로우 화면이 렌더될 때, **the system shall** 각 액션 컨트롤을 해당 권한 보유 여부에 따라 노출한다.

| 컨트롤 | 권한 |
|--------|------|
| 등록/추가 버튼 | `<resource>.create` |
| 수정/설정 버튼 | `<resource>.update` |
| 삭제 버튼 | `<resource>.delete` |
| 시작·정지·재시작·활성/비활성 | `<resource>.execute` |

**숨김이 아니라 비활성화**를 기본으로 한다. 버튼이 사라지면 사용자는 기능이 없는 것으로 오해하지만, 비활성 + 툴팁("권한이 필요합니다")은 관리자에게 요청할 여지를 남긴다. 단, 화면 전체가 권한 밖인 경우(메뉴/라우트)는 숨김을 적용한다.

### 2.5 [UB1] (Unwanted-Behavior) 게이팅 우회와 상태 불일치

1. 권한이 없는 경로를 URL로 직접 입력해도 화면이 렌더되지 않는다.
2. 서버가 403을 반환하면 UI는 그 사실을 사용자 언어로 안내하고, 무한 재시도를 하지 않는다.
3. 관리자가 자신의 역할을 변경한 경우, 다음 `auth/me` 갱신 시점에 UI 권한이 갱신된다. 갱신 전까지 UI가 허용해 보이더라도 서버가 거부하므로 보안 경계는 유지된다.
4. `permissions` 가 응답에 없는 구버전 서버에 접속하면(하위 호환) 모든 판정을 true 로 폴백해 기존 동작을 유지한다.

---

## 3. 트레이서빌리티 표

| 요구사항 | 대상 파일 | 검증 |
|----------|-----------|------|
| U1 권한 컨텍스트 | `web/src/stores/authStore.ts`, `web/src/types/auth.ts`, `web/src/hooks/usePermission.ts`(신규) | AC-01, AC-02 |
| U2 사용자 관리 화면 | `web/src/pages/admin/UsersPage.tsx`(신규) | AC-03 |
| U2 역할 관리 화면 | `web/src/pages/admin/RolesPage.tsx`(신규) | AC-04 |
| U2 API 클라이언트 | `web/src/services/api/userService.ts`(신규) | AC-03, AC-04 |
| S1 메뉴 게이팅 | `web/src/components/layout/Sidebar.tsx` | AC-05 |
| S1 라우트 게이팅 | `web/src/router.tsx`, `web/src/components/layout/AuthGuard.tsx` | AC-06 |
| E1 액션 게이팅 | `web/src/pages/agents/`, `web/src/pages/devices/`, `web/src/pages/flows/` | AC-07 |
| UB1 폴백·오류 처리 | `web/src/stores/authStore.ts`, `web/src/services/api/client.ts` | AC-08, AC-09 |

---

## 4. 설계 결정

### 4.1 역할 이름이 아닌 권한 키로 판단

커스텀 역할이 생기는 순간 `roles: ['admin']` 같은 열거는 유지 불가능해진다. 권한 키로 판단하면 새 역할이 추가돼도 UI 코드를 고칠 필요가 없다.

### 4.2 액션 컨트롤은 비활성, 화면은 숨김

액션 버튼을 숨기면 "기능이 없다"로 읽히고, 화면(메뉴)을 비활성으로 남기면 접근할 수 없는 항목이 목록을 채운다. 층위에 따라 다르게 처리한다.

### 4.3 인증 비활성·구버전 서버에서는 전원 허용

`authEnabled === false` 이거나 `permissions` 필드가 없는 응답을 만나면 모든 권한 판정을 true 로 둔다. 서버가 검사하지 않는 상태에서 UI만 잠그는 것은 보안 이득 없이 회귀만 만든다.

---

## 5. 비기능 요구사항

| 항목 | 기준 |
|------|------|
| 하위 호환 | `permissions` 미제공 서버에서 기존과 동일하게 동작 |
| 접근성 | 비활성 버튼은 `aria-disabled` 와 사유 툴팁을 제공 |
| 국제화 | 신규 문구는 ko/en 두 로케일 모두 추가 |
| 테스트 | 신규 컴포넌트·훅 단위 테스트, 게이팅 로직 커버리지 85% 이상 |
| 성능 | 권한 판정은 메모리 집합 조회로 렌더당 추가 네트워크 요청 없음 |

---

## 6. 가정 및 제약

1. SPEC-AUTH-005 가 먼저 완료되어 `/auth/me` 가 `permissions` 를 제공한다. 미완료 상태에서는 폴백(전원 허용)으로 동작하므로 구현 착수는 가능하나 검증은 불가하다.
2. 권한 목록은 로그인/세션 복원 시 1회 조회하며, 실시간 푸시 갱신은 하지 않는다.
3. 사용자 수는 한 화면에 목록으로 표시 가능한 규모(수십 명)로 가정한다. 페이지네이션은 범위 밖이다.
4. 기존 `/admin/*` 라우트의 `requireRole="admin"` 은 `requirePermission` 으로 대체되며, 빌트인 `admin` 역할은 전체 권한을 가지므로 기존 관리자 경험은 변하지 않는다.
