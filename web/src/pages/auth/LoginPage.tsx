// 로그인 페이지 컴포넌트.
// 인증 API 미구현 상태 — /login 접속 시 자동으로 메인 페이지로 리다이렉트한다.
//
// TODO(SPEC-AUTH-001): 인증 API 구현 후 이메일/비밀번호 로그인 폼 복원

import { Navigate, useSearchParams } from 'react-router';

/**
 * 로그인 페이지.
 * 인증 API가 구현될 때까지 returnUrl 또는 '/'로 즉시 리다이렉트한다.
 *
 * TODO(SPEC-AUTH-001): 인증 API 구현 후 로그인 폼 복원
 * - 이전 구현: git show 6d77c67:web/src/pages/auth/LoginPage.tsx
 */
export default function LoginPage() {
  const [searchParams] = useSearchParams();
  const returnUrl = searchParams.get('returnUrl') || '/';
  return <Navigate to={returnUrl} replace />;
}
