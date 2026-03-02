// 인증 가드 컴포넌트.
// 인증되지 않은 사용자를 /login 페이지로 리다이렉트한다.
// 인증된 사용자는 자식 라우트(Outlet)를 렌더링한다.

import { Outlet } from 'react-router';

/**
 * 보호된 라우트를 위한 인증 가드.
 * 미인증 시 현재 경로를 returnUrl 파라미터로 포함하여 로그인 페이지로 리다이렉트한다.
 *
 * TODO(SPEC-AUTH-001): 인증 API 구현 후 인증 검사 복원
 */
export default function AuthGuard() {
  // 인증 API 미구현 상태 — 모든 접근 허용
  return <Outlet />;
}
