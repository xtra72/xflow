// 사이드바 하단 사용자 메뉴 (비밀번호 변경 · 로그아웃).
//
// 이전에는 Header 우측 상단에만 있었다. 그런데 Header 는 대시보드 라우트에서
// null 을 반환하므로(대시보드가 자체 헤더를 쓴다), 메뉴가 대시보드 하나만 남은
// 사용자는 로그아웃할 방법이 없었다 — 다른 화면으로 이동해야 했는데 갈 곳이
// 없기 때문이다. 사이드바는 어떤 권한 조합에서도 항상 렌더되므로 여기로 옮긴다.
//
// @spec SPEC-AUTH-006

import { ChevronUp, Key, LogOut } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';

import { useAuth } from '@/hooks/useAuth';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import ChangePasswordDialog from '@/pages/auth/ChangePasswordDialog';

interface SidebarUserMenuProps {
  /** 사이드바 접힘 상태 — 접히면 이니셜만 보인다. */
  collapsed: boolean;
}

/** 역할 뱃지 색상. 역할 이름은 표시 용도로만 쓴다(게이팅 아님 — AC-10). */
function roleBadgeClass(role: string): string {
  if (role === 'admin') {
    return 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400';
  }
  if (role === 'editor') {
    return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400';
  }
  return 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300';
}

export default function SidebarUserMenu({ collapsed }: SidebarUserMenuProps) {
  const { t } = useTranslation();
  const { user, authEnabled, logout } = useAuth();
  const [open, setOpen] = useState(false);
  const [passwordDialogOpen, setPasswordDialogOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', onPointerDown);
    return () => document.removeEventListener('mousedown', onPointerDown);
  }, [open]);

  if (!user) return null;

  // 인증 비활성 배포에는 로그아웃할 세션이 없다 — 사용자 정보만 표시한다.
  if (!authEnabled) {
    return (
      <div
        className={cn(
          'flex items-center gap-2 px-2 py-2 text-sm text-(--color-text-secondary)',
          collapsed && 'justify-center',
        )}
        data-testid="sidebar-user-static"
      >
        {collapsed ? (
          <span className="text-xs font-semibold">{user.name.slice(0, 2)}</span>
        ) : (
          <>
            <span className="truncate font-medium">{user.name}</span>
            <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium', roleBadgeClass(user.role))}>
              {user.role}
            </span>
          </>
        )}
      </div>
    );
  }

  return (
    <div className="relative" ref={menuRef}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        aria-haspopup="true"
        aria-label={t('auth.userMenu')}
        title={collapsed ? user.name : undefined}
        data-testid="sidebar-user-menu-trigger"
        className={cn(
          'flex w-full items-center gap-2 rounded-md px-2 py-2 text-sm transition-colors',
          'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
          collapsed && 'justify-center',
        )}
      >
        {collapsed ? (
          <span className="text-xs font-semibold">{user.name.slice(0, 2)}</span>
        ) : (
          <>
            <span className="truncate font-medium">{user.name}</span>
            <span className={cn('shrink-0 rounded-full px-2 py-0.5 text-xs font-medium', roleBadgeClass(user.role))}>
              {user.role}
            </span>
            <ChevronUp
              className={cn('ml-auto h-4 w-4 shrink-0 transition-transform', !open && 'rotate-180')}
              aria-hidden="true"
            />
          </>
        )}
      </button>

      {open && (
        // 사이드바 하단이므로 위쪽으로 펼친다.
        <div
          className="absolute bottom-full left-0 z-50 mb-1 w-48 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) py-1 shadow-lg"
          data-testid="sidebar-user-menu"
        >
          <button
            type="button"
            onClick={() => {
              setOpen(false);
              setPasswordDialogOpen(true);
            }}
            className="flex w-full items-center gap-2 px-3 py-2 text-sm text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
          >
            <Key className="h-4 w-4" aria-hidden="true" />
            {t('auth.changePassword')}
          </button>
          <hr className="my-1 border-(--color-border-default)" />
          <button
            type="button"
            onClick={() => {
              setOpen(false);
              void logout();
            }}
            className="flex w-full items-center gap-2 px-3 py-2 text-sm text-red-600 hover:bg-(--color-bg-elevated) dark:text-red-400"
          >
            <LogOut className="h-4 w-4" aria-hidden="true" />
            {t('auth.logout')}
          </button>
        </div>
      )}

      <ChangePasswordDialog
        open={passwordDialogOpen}
        onClose={() => setPasswordDialogOpen(false)}
      />
    </div>
  );
}
