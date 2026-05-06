// 인증 가드 컴포넌트.
// 인증되지 않은 사용자를 /login 페이지로 리다이렉트한다.
// 인증된 사용자 또는 인증 비활성화 시 자식 라우트(Outlet)를 렌더링한다.
//
// SPEC-WEB-006 v0.1.0 (M1, M11): 옵션 `requireRole` 추가.
//   - 미지정: 기존 동작 그대로 (auth 통과 시 Outlet 렌더).
//   - 지정: 사용자의 role 이 일치할 때만 Outlet 렌더, 불일치 시 ForbiddenPage.
//   - authEnabled=false 일 때는 단일 사용자 dev 모드로 간주하여 role 검사 우회.
//
// @spec SPEC-WEB-006 v0.1.0 (M1, M11)

import { useEffect } from 'react';
import { Navigate, Outlet, useLocation } from 'react-router';
import { Loader2 } from 'lucide-react';

import { useAuth } from '@/hooks/useAuth';
import { ForbiddenPage } from '@/pages/system/ForbiddenPage';
import type { UserRole } from '@/types/auth';

interface AuthGuardProps {
  /**
   * 추가 권한 검증을 수행할 역할(들). 단일 또는 복수.
   *
   * - 미지정: 인증만 통과하면 Outlet 렌더 (기존 동작).
   * - 지정: 인증 + role 일치 모두 충족해야 Outlet 렌더.
   * - 인증 비활성(authEnabled=false): role 검사를 건너뛴다 (dev 모드 단일 사용자).
   *
   * @example
   *   <AuthGuard requireRole="admin" />        // admin 만
   *   <AuthGuard requireRole={['admin','editor']} />  // admin 또는 editor
   */
  requireRole?: UserRole | readonly UserRole[];
}

/**
 * 보호된 라우트를 위한 인증 가드.
 * 앱 시작 시 서버 인증 상태를 확인하고,
 * 인증이 활성화된 경우 미인증 사용자를 로그인 페이지로 리다이렉트한다.
 * `requireRole` 이 지정되면 추가로 사용자 역할을 검증한다.
 */
export default function AuthGuard({
  requireRole,
}: AuthGuardProps = {}): React.JSX.Element {
  const { isAuthenticated, authEnabled, isLoading, initialize, user } =
    useAuth();
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

  // 인증 비활성화 — 모든 접근 허용 (role 검사도 우회)
  if (!authEnabled) {
    return <Outlet />;
  }

  // 인증 활성화 + 미인증 — 로그인 페이지로 리다이렉트
  if (!isAuthenticated) {
    const returnUrl = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/login?returnUrl=${returnUrl}`} replace />;
  }

  // 인증됨 — role 검사 필요 시 추가 검증
  if (requireRole && !hasRequiredRole(user?.role, requireRole)) {
    return <ForbiddenPage />;
  }

  // 모든 검증 통과 — 자식 라우트 렌더링
  return <Outlet />;
}

/**
 * 사용자의 role 이 요구되는 role(s) 중 하나와 일치하는지 검사한다.
 *
 * - role 이 undefined (예: user 가 아직 미로딩) 인 경우 false 반환.
 * - 단일 string 또는 readonly array 둘 다 지원한다.
 */
function hasRequiredRole(
  role: UserRole | undefined,
  required: UserRole | readonly UserRole[],
): boolean {
  if (!role) return false;
  if (typeof required === 'string') {
    return role === required;
  }
  return required.includes(role);
}
