# SPEC-AUTH-006 구현 계획

## 1. 작업 분해

### M1 — 권한 컨텍스트

| # | 작업 | 산출물 |
|---|------|--------|
| 1.1 | `UserRole` 3종 유니온 → `string` 완화, `User.permissions` 추가 | `web/src/types/auth.ts` |
| 1.2 | authStore 에 `permissions: Set<string>` 보관 + 로그인/복원 시 채움 | `web/src/stores/authStore.ts` |
| 1.3 | `hasPermission` / `hasAnyPermission` + 폴백(인증 비활성·구버전 서버) | `web/src/hooks/usePermission.ts` (신규) |
| 1.4 | 단위 테스트 | `usePermission.test.ts` (신규) |

폴백 규칙(한 곳에 모아 둔다):

```
authEnabled === false        → 항상 true
permissions 필드 없음(구버전) → 항상 true
그 외                        → 집합 조회
```

`UserRole` 완화는 타입 넓히기이므로 기존 사용처는 컴파일이 유지된다. 다만 `roles.includes(user.role)` 같은 코드가 남아 있으면 의미가 무너지므로 M3 에서 함께 걷어낸다.

### M2 — 관리 화면과 API 클라이언트

| # | 작업 | 산출물 |
|---|------|--------|
| 2.1 | 사용자/역할/권한 API 클라이언트 | `web/src/services/api/userService.ts` (신규) |
| 2.2 | 사용자 관리 페이지(목록·등록·역할변경·비밀번호 재설정·삭제) | `web/src/pages/admin/UsersPage.tsx` (신규) |
| 2.3 | 역할 관리 페이지(목록·생성·권한 행렬 체크박스·삭제) | `web/src/pages/admin/RolesPage.tsx` (신규) |
| 2.4 | 409 잠금 방지 응답의 사용자 안내 매핑 | 2.2 / 2.3 내부 |
| 2.5 | i18n 키(ko/en) | `web/src/lib/i18n/{ko,en}.json` |

권한 행렬 UI: 세로축 리소스(12종), 가로축 액션(read/create/update/delete/execute). 해당 리소스에 없는 액션은 셀을 비운다.

### M3 — 메뉴·라우트 게이팅 전환

| # | 작업 | 산출물 |
|---|------|--------|
| 3.1 | `NavItem.roles` → `NavItem.permission`, `hasAccess` 를 권한 판정으로 교체 | `web/src/components/layout/Sidebar.tsx` |
| 3.2 | 메뉴 ↔ 권한 매핑 적용(spec.md §2.3 표) + 사용자/역할 관리 메뉴 추가 | 동일 |
| 3.3 | `AuthGuard.requireRole` → `requirePermission` | `web/src/components/layout/AuthGuard.tsx` |
| 3.4 | 라우트에 권한 지정 + `/admin/users`, `/admin/roles` 등록 | `web/src/router.tsx` |
| 3.5 | 권한 없는 진입 시 대시보드 리다이렉트 + 안내 | `AuthGuard.tsx` |

### M4 — 액션 컨트롤 게이팅

| # | 작업 | 산출물 |
|---|------|--------|
| 4.1 | 권한 기반 비활성 버튼 래퍼(`aria-disabled` + 사유 툴팁) | `web/src/components/common/PermissionButton.tsx` (신규) |
| 4.2 | 에이전트 화면 액션 게이팅 | `web/src/pages/agents/` |
| 4.3 | 디바이스 화면 액션 게이팅 | `web/src/pages/devices/` |
| 4.4 | 플로우 화면 액션 게이팅 | `web/src/pages/flows/` |

먼저 각 화면의 액션 컨트롤을 전수 조사해 목록을 만든 뒤 게이팅한다. 조사 없이 눈에 띄는 버튼만 처리하면 누락이 남는다.

---

## 2. 기술 스택

기존 스택만 사용한다 — React 19, TypeScript, zustand, TanStack Query, Tailwind. **새 의존성 없음.**

---

## 3. 위험 분석

| 위험 | 영향 | 완화 |
|------|------|------|
| SPEC-AUTH-005 미완료 상태에서 착수 | `permissions` 가 없어 검증 불가 | 폴백(전원 허용)으로 개발은 진행, 통합 검증은 005 완료 후 |
| `UserRole` 완화 후 잔존 역할 비교 코드 | 커스텀 역할이 메뉴에서 사라짐 | 전환 후 `role ===` / `roles.includes` 잔존 여부 전수 grep 검증(AC-10) |
| 액션 컨트롤 누락 | 권한 없는 사용자가 버튼을 눌러 403을 받음 | M4 전수 조사 + 서버가 실제 차단하므로 보안 사고는 아님 |
| 게이팅 과잉 | 정상 사용자가 기능을 못 찾음 | 액션은 숨김이 아니라 비활성 + 사유 표시 |
| 인증 비활성 배포 회귀 | 전 메뉴 소실 | 폴백 규칙 단위 테스트(AC-08) |

---

## 4. 구현 순서

M1 → M3 → M2 → M4

M3(게이팅 전환)을 M2(관리 화면)보다 앞에 두는 이유: 관리 화면 자체가 `user.read` / `role.read` 로 게이팅되어야 하므로 게이팅 기반이 먼저 있어야 한다. M1 은 둘 모두의 전제다.

---

## 5. 검증 방법

```bash
cd web && npx tsc --noEmit && npx eslint src && npx vitest run && npm run build
```

전체 완료 시 acceptance.md 의 AC-01~AC-10 을 검증한다. 통합 검증(실제 권한 응답 기반)은 SPEC-AUTH-005 완료를 전제로 한다.
