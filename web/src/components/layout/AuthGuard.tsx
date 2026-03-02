// 인증 가드 컴포넌트.
// 인증되지 않은 사용자를 /login 페이지로 리다이렉트한다.
// 인증된 사용자는 자식 라우트(Outlet)를 렌더링한다.

import { Navigate, Outlet, useLocation } from 'react-router';

import { useAuth } from '@/hooks/useAuth';
import LoadingSpinner from '@/components/layout/LoadingSpinner';

/**
 * 보호된 라우트를 위한 인증 가드.
 * 미인증 시 현재 경로를 returnUrl 파라미터로 포함하여 로그인 페이지로 리다이렉트한다.
 */
export default function AuthGuard() {
  const { isAuthenticated, isLoading } = useAuth();
  const location = useLocation();

  // 인증 상태 확인 중에는 로딩 스피너 표시
  if (isLoading) {
    return <LoadingSpinner />;
  }

  // 미인증 시 로그인 페이지로 리다이렉트 (현재 경로를 returnUrl로 전달)
  if (!isAuthenticated) {
    const returnUrl = location.pathname + location.search;
    return <Navigate to={`/login?returnUrl=${encodeURIComponent(returnUrl)}`} replace />;
  }

  return <Outlet />;
}
