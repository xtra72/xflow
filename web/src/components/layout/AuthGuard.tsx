// 인증 가드 컴포넌트.
// 인증되지 않은 사용자를 /login 페이지로 리다이렉트한다.
// 인증된 사용자 또는 인증 비활성화 시 자식 라우트(Outlet)를 렌더링한다.
//
// SPEC-WEB-006 v0.1.0 (M1, M11): 옵션 `requireRole` 로 admin 라우트를 보호했다.
//
// SPEC-AUTH-006 S1 (M3.3, M3.5): `requireRole` 을 `requirePermission` 으로 교체.
//   - 미지정: 기존 동작 그대로 (auth 통과 시 Outlet 렌더).
//   - 지정: 권한 보유 시에만 Outlet 렌더.
//   - 권한 없음: ForbiddenPage 를 렌더하지 않고 대시보드로 리다이렉트 + 안내
//     (AC-06 "빈 화면이 아니다"). 막다른 화면 대신 즉시 갈 곳을 준다.
//   - authEnabled=false / 구버전 서버 폴백은 usePermission 이 흡수한다.
//
// @spec SPEC-WEB-006 v0.1.0 (M1, M11)
// @spec SPEC-AUTH-006 v0.1.0 (M3.3, M3.5)

import { useEffect } from 'react';
import { Navigate, Outlet, useLocation } from 'react-router';
import { Loader2 } from 'lucide-react';

import { useAuth } from '@/hooks/useAuth';
import { usePermission } from '@/hooks/usePermission';
import { useTranslation } from '@/lib/i18n';
import { useUIStore } from '@/stores/uiStore';

/** 권한 부족 시 되돌려 보낼 경로. 대시보드는 인증만 요구하므로 항상 도달 가능하다. */
const FALLBACK_PATH = '/';

interface AuthGuardProps {
  /**
   * 추가로 요구할 권한 키(`<resource>.<action>`).
   *
   * - 미지정: 인증만 통과하면 Outlet 렌더 (기존 동작).
   * - 지정: 인증 + 권한 보유 모두 충족해야 Outlet 렌더.
   * - 인증 비활성(authEnabled=false)이거나 구버전 서버(permissions 미제공)면
   *   usePermission 의 폴백이 true 를 반환하므로 검사를 통과한다.
   *
   * @example
   *   <AuthGuard requirePermission="user.read" />
   */
  requirePermission?: string;
}

/**
 * 보호된 라우트를 위한 인증 가드.
 * 앱 시작 시 서버 인증 상태를 확인하고,
 * 인증이 활성화된 경우 미인증 사용자를 로그인 페이지로 리다이렉트한다.
 * `requirePermission` 이 지정되면 추가로 권한을 검증한다.
 */
export default function AuthGuard({
  requirePermission,
}: AuthGuardProps = {}): React.JSX.Element {
  const { isAuthenticated, authEnabled, isLoading, initialize } = useAuth();
  const { hasPermission, isPermissionUnavailable } = usePermission();
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);
  const location = useLocation();

  useEffect(() => {
    void initialize();
  }, [initialize]);

  // 인증 상태 확인이 끝나고, 인증된 사용자가 권한 부족으로 막힌 경우에만 참이다.
  // 로딩/미인증 단계에서 안내를 띄우면 로그인 리다이렉트와 겹쳐 소음이 된다.
  const denied =
    authEnabled === true &&
    !isLoading &&
    isAuthenticated &&
    requirePermission !== undefined &&
    !hasPermission(requirePermission);

  // UB1.2: 안내는 1회만 띄우고 재시도 루프를 만들지 않는다.
  // 권한 조회 실패와 권한 부족은 사용자에게 다른 안내를 준다 — 전자는 재시도가
  // 의미 있고, 후자는 관리자에게 요청해야 한다.
  useEffect(() => {
    if (!denied) return;
    addNotification({
      type: 'warning',
      message: isPermissionUnavailable
        ? t('auth.permissionUnavailable')
        : t('auth.permissionDenied'),
    });
  }, [denied, isPermissionUnavailable, addNotification, t]);

  // 인증 상태 확인 중 — 로딩 표시
  if (authEnabled === null || isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-(--color-bg-base)">
        <Loader2 className="h-8 w-8 animate-spin text-(--color-text-muted)" />
      </div>
    );
  }

  // 인증 비활성화 — 모든 접근 허용 (권한 검사도 우회)
  if (!authEnabled) {
    return <Outlet />;
  }

  // 인증 활성화 + 미인증 — 로그인 페이지로 리다이렉트
  if (!isAuthenticated) {
    const returnUrl = encodeURIComponent(location.pathname + location.search);
    return <Navigate to={`/login?returnUrl=${returnUrl}`} replace />;
  }

  // 인증됨 + 권한 부족 — 대시보드로 되돌려 보낸다.
  // 이미 대시보드에 있다면 리다이렉트가 자기 자신을 향해 반복되므로 그대로
  // 렌더한다. 대시보드 라우트에는 requirePermission 을 걸지 않으므로 실제로는
  // 도달하지 않지만, 향후 잘못된 설정이 무한 루프가 되지 않도록 막아 둔다.
  if (denied && location.pathname !== FALLBACK_PATH) {
    return <Navigate to={FALLBACK_PATH} replace />;
  }

  // 모든 검증 통과 — 자식 라우트 렌더링
  return <Outlet />;
}
