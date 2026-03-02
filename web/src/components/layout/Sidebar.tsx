// 사이드바 네비게이션 컴포넌트.
// 메뉴 항목, 접기/펼치기 토글, RBAC 기반 메뉴 필터링을 제공한다.

import {
  ChevronLeft,
  ChevronRight,
  LayoutDashboard,
  Monitor,
  Settings,
  Workflow,
} from 'lucide-react';
import { NavLink } from 'react-router';

import { useAuth } from '@/hooks/useAuth';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';
import type { UserRole } from '@/types/auth';

/** 네비게이션 메뉴 항목 정의 */
interface NavItem {
  /** 번역 키 */
  labelKey: string;
  /** 라우트 경로 */
  path: string;
  /** lucide-react 아이콘 컴포넌트 */
  icon: React.ComponentType<{ className?: string }>;
  /** 접근 가능한 역할 목록. 미지정 시 모든 역할 허용. */
  roles?: UserRole[];
  /** 특정 라우트에서만 표시할지 여부 */
  matchPath?: string;
}

/** 메뉴 항목 목록 */
const NAV_ITEMS: NavItem[] = [
  {
    labelKey: 'nav.dashboard',
    path: '/',
    icon: LayoutDashboard,
  },
  {
    labelKey: 'nav.flows',
    path: '/flows',
    icon: Workflow,
  },
  {
    labelKey: 'nav.monitoring',
    path: '/monitoring',
    icon: Monitor,
  },
  {
    labelKey: 'nav.settings',
    path: '/settings',
    icon: Settings,
    roles: ['admin'],
  },
];

/**
 * 앱 사이드바 네비게이션.
 * 접기/펼치기 토글, 활성 메뉴 하이라이트, RBAC 필터링을 지원한다.
 */
export default function Sidebar() {
  const { t } = useTranslation();
  const { user } = useAuth();
  const sidebarCollapsed = useUIStore((s) => s.sidebarCollapsed);
  const toggleSidebar = useUIStore((s) => s.toggleSidebar);

  // 사용자 역할에 따른 메뉴 필터링
  const filteredItems = NAV_ITEMS.filter((item) => {
    // 역할 제한이 없으면 모두 표시
    if (!item.roles) return true;
    // 사용자 역할이 허용 목록에 있는지 확인
    return user?.role ? item.roles.includes(user.role) : false;
  });

  // 에디터 라우트에 있는 경우 에디터 항목은 별도로 표시하지 않음
  // (에디터는 /flows에서 진입하므로 사이드바에 직접 노출하지 않는다)

  return (
    <aside
      className={cn(
        'flex h-screen flex-col border-r border-gray-200 bg-white transition-[width] duration-200',
        'dark:border-gray-700 dark:bg-gray-800',
        sidebarCollapsed ? 'w-(--sidebar-collapsed-width)' : 'w-(--sidebar-width)',
      )}
    >
      {/* 로고 영역 */}
      <div className="flex h-(--header-height) items-center border-b border-gray-200 px-4 dark:border-gray-700">
        {!sidebarCollapsed && (
          <span className="text-lg font-bold text-gray-900 dark:text-white">XFlow</span>
        )}
        {sidebarCollapsed && (
          <span className="text-lg font-bold text-gray-900 dark:text-white">X</span>
        )}
      </div>

      {/* 네비게이션 메뉴 */}
      <nav className="flex-1 space-y-1 overflow-y-auto px-2 py-3" aria-label={t('nav.sidebarNav')}>
        {filteredItems.map((item) => {
          const Icon = item.icon;
          // end 옵션: '/' 경로는 정확히 일치할 때만 활성 표시
          const isRoot = item.path === '/';

          return (
            <NavLink
              key={item.path}
              to={item.path}
              end={isRoot}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
                  'hover:bg-gray-100 dark:hover:bg-gray-700',
                  isActive
                    ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
                    : 'text-gray-700 dark:text-gray-300',
                  sidebarCollapsed && 'justify-center px-2',
                )
              }
              title={sidebarCollapsed ? t(item.labelKey) : undefined}
            >
              <Icon className="h-5 w-5 shrink-0" aria-hidden="true" />
              {!sidebarCollapsed && <span>{t(item.labelKey)}</span>}
            </NavLink>
          );
        })}
      </nav>

      {/* 접기/펼치기 토글 버튼 */}
      <div className="border-t border-gray-200 p-2 dark:border-gray-700">
        <button
          type="button"
          onClick={toggleSidebar}
          className={cn(
            'flex w-full items-center justify-center rounded-md p-2 text-gray-500 transition-colors',
            'hover:bg-gray-100 hover:text-gray-700',
            'dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200',
          )}
          aria-label={sidebarCollapsed ? t('nav.expandSidebar') : t('nav.collapseSidebar')}
        >
          {sidebarCollapsed ? (
            <ChevronRight className="h-5 w-5" aria-hidden="true" />
          ) : (
            <ChevronLeft className="h-5 w-5" aria-hidden="true" />
          )}
        </button>
      </div>
    </aside>
  );
}
