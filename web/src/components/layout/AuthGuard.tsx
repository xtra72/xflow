// 인증 가드 컴포넌트.
// 인증되지 않은 사용자를 /login 페이지로 리다이렉트한다.
// 인증된 사용자 또는 인증 비활성화 시 자식 라우트(Outlet)를 렌더링한다.

import { useEffect } from 'react';
import { Navigate, Outlet, useLocation } from 'react-router';
import { Loader2 } from 'lucide-react';

import { useAuth } from '@/hooks/useAuth';

/**
 * 보호된 라우트를 위한 인증 가드.
 * 앱 시작 시 서버 인증 상태를 확인하고,
 * 인증이 활성화된 경우 미인증 사용자를 로그인 페이지로 리다이렉트한다.
 */
export default function AuthGuard() {
  const { isAuthenticated, authEnabled, isLoading, initialize } = useAuth();
  const location = useLocation();

  useEffect(() => {
    void initialize();
  }, [initialize]);

  // 인증 상태 확인 중 — 로딩 표시
  if (authEnabled === null || isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-(--color-bg-base)">
        <Loader2 className="h-8 w-8 animate-spin text-(--color-text-muted)" />
      </div>
    );
  }

  // 인증 비활성화 — 모든 접근 허용
  if (!authEnabled) {
    return <Outlet />;
  }

  // 인증 활성화 + 미인증 — 로그인 페이지로 리다이렉트
  if (!isAuthenticated) {
    const returnUrl = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/login?returnUrl=${returnUrl}`} replace />;
  }

  // 인증됨 — 자식 라우트 렌더링
  return <Outlet />;
}
