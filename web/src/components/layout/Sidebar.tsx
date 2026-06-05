// 사이드바 네비게이션 컴포넌트.
// 메뉴 항목, 접기/펼치기 토글, RBAC 기반 메뉴 필터링을 제공한다.
// 그룹 메뉴(하위 항목 포함)를 지원한다.

import { useState } from 'react';
import {
  Blocks,
  BookOpen,
  Bot,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Boxes,
  HardDrive,
  LayoutDashboard,
  Monitor,
  Network,
  Settings,
  Workflow,
} from 'lucide-react';
import { NavLink, useLocation } from 'react-router';

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
}

/** 그룹 메뉴 정의 (하위 항목 포함) */
interface NavGroup {
  /** 번역 키 */
  labelKey: string;
  /** lucide-react 아이콘 컴포넌트 */
  icon: React.ComponentType<{ className?: string }>;
  /** 하위 메뉴 항목 */
  children: NavItem[];
  /** 접근 가능한 역할 목록. 미지정 시 모든 역할 허용. */
  roles?: UserRole[];
}

type NavEntry = NavItem | NavGroup;

/** NavEntry가 그룹인지 판별 */
function isNavGroup(entry: NavEntry): entry is NavGroup {
  return 'children' in entry;
}

/** 메뉴 항목 목록 */
const NAV_ENTRIES: NavEntry[] = [
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
    labelKey: 'nav.agents',
    path: '/agents',
    icon: Bot,
  },
  {
    labelKey: 'nav.devices',
    path: '/devices',
    icon: HardDrive,
  },
  {
    labelKey: 'nav.monitoring',
    path: '/monitoring',
    icon: Monitor,
  },
  // 참고 그룹 메뉴
  {
    labelKey: 'nav.reference',
    icon: BookOpen,
    children: [
      {
        labelKey: 'nav.nodes',
        path: '/nodes',
        icon: Blocks,
      },
      {
        labelKey: 'nav.agentTypes',
        path: '/agent-types',
        icon: Bot,
      },
    ],
  },
  // SPEC-REMOTE-001 M5: admin 전용 원격 관리 그룹.
  {
    labelKey: 'nav.remote',
    icon: Network,
    roles: ['admin'],
    children: [
      {
        labelKey: 'nav.remoteNodes',
        path: '/admin/remote',
        icon: Network,
        roles: ['admin'],
      },
      {
        labelKey: 'nav.remoteResources',
        path: '/admin/remote/resources',
        icon: Boxes,
        roles: ['admin'],
      },
    ],
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
 * 접기/펼치기 토글, 활성 메뉴 하이라이트, RBAC 필터링, 그룹 메뉴를 지원한다.
 */
export default function Sidebar() {
  const { t } = useTranslation();
  const { user } = useAuth();
  const location = useLocation();
  const sidebarCollapsed = useUIStore((s) => s.sidebarCollapsed);
  const toggleSidebar = useUIStore((s) => s.toggleSidebar);

  // 그룹의 하위 항목 중 현재 활성인 경로가 있는 그룹을 자동 펼침
  const activeGroupKey = NAV_ENTRIES.find(
    (entry) =>
      isNavGroup(entry) &&
      entry.children.some((child) => location.pathname === child.path),
  );
  const [openGroups, setOpenGroups] = useState<Set<string>>(
    () => new Set(activeGroupKey ? [activeGroupKey.labelKey] : []),
  );

  const toggleGroup = (key: string) => {
    setOpenGroups((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  /** 역할 기반 필터링 */
  const hasAccess = (roles?: UserRole[]) => {
    if (!roles) return true;
    return user?.role ? roles.includes(user.role) : false;
  };

  // 사용자 역할에 따른 메뉴 필터링
  const filteredEntries = NAV_ENTRIES.filter((entry) => {
    if (!hasAccess(entry.roles)) return false;
    // 그룹의 경우 접근 가능한 하위 항목이 하나라도 있으면 표시
    if (isNavGroup(entry)) {
      return entry.children.some((child) => hasAccess(child.roles));
    }
    return true;
  });

  return (
    <aside
      className={cn(
        'flex h-screen flex-col border-r border-(--color-border-default) bg-(--color-bg-surface) transition-[width] duration-200',
        sidebarCollapsed ? 'w-(--sidebar-collapsed-width)' : 'w-(--sidebar-width)',
      )}
    >
      {/* 로고 영역 */}
      <div className="flex h-(--header-height) items-center border-b border-(--color-border-default) px-4">
        {!sidebarCollapsed && (
          <span className="text-lg font-bold text-(--color-text-primary)">XFlow</span>
        )}
        {sidebarCollapsed && (
          <span className="text-lg font-bold text-(--color-text-primary)">X</span>
        )}
      </div>

      {/* 네비게이션 메뉴 */}
      <nav className="flex-1 space-y-1 overflow-y-auto px-2 py-3" aria-label={t('nav.sidebarNav')}>
        {filteredEntries.map((entry) => {
          if (isNavGroup(entry)) {
            return (
              <NavGroupItem
                key={entry.labelKey}
                group={entry}
                isOpen={openGroups.has(entry.labelKey)}
                onToggle={() => toggleGroup(entry.labelKey)}
                collapsed={sidebarCollapsed}
                t={t}
                userRole={user?.role}
              />
            );
          }

          const Icon = entry.icon;
          const isRoot = entry.path === '/';

          return (
            <NavLink
              key={entry.path}
              to={entry.path}
              end={isRoot}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
                  'hover:bg-(--color-bg-elevated)',
                  isActive
                    ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
                    : 'text-(--color-text-secondary)',
                  sidebarCollapsed && 'justify-center px-2',
                )
              }
              title={sidebarCollapsed ? t(entry.labelKey) : undefined}
            >
              <Icon className="h-5 w-5 shrink-0" aria-hidden="true" />
              {!sidebarCollapsed && <span>{t(entry.labelKey)}</span>}
            </NavLink>
          );
        })}
      </nav>

      {/* 접기/펼치기 토글 버튼 */}
      <div className="border-t border-(--color-border-default) p-2">
        <button
          type="button"
          onClick={toggleSidebar}
          className={cn(
            'flex w-full items-center justify-center rounded-md p-2 text-(--color-text-muted) transition-colors',
            'hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary)',
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

// ---- 그룹 메뉴 컴포넌트 ----

interface NavGroupItemProps {
  group: NavGroup;
  isOpen: boolean;
  onToggle: () => void;
  collapsed: boolean;
  t: (key: string) => string;
  userRole?: UserRole;
}

/** 그룹 메뉴 (접기/펼치기 가능한 하위 항목 포함) */
function NavGroupItem({ group, isOpen, onToggle, collapsed, t, userRole }: NavGroupItemProps) {
  const Icon = group.icon;
  const location = useLocation();

  // 하위 항목 중 활성인 것이 있는지 확인
  const hasActiveChild = group.children.some(
    (child) => location.pathname === child.path,
  );

  // 접근 가능한 하위 항목만 필터링
  const visibleChildren = group.children.filter((child) => {
    if (!child.roles) return true;
    return userRole ? child.roles.includes(userRole) : false;
  });

  // 사이드바가 접힌 상태에서는 첫 번째 하위 항목 경로로 직접 이동
  if (collapsed) {
    const firstChild = visibleChildren[0];
    if (!firstChild) return null;

    return (
      <NavLink
        to={firstChild.path}
        className={({ isActive }) =>
          cn(
            'flex items-center justify-center rounded-md px-2 py-2 text-sm font-medium transition-colors',
            'hover:bg-(--color-bg-elevated)',
            isActive
              ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
              : 'text-(--color-text-secondary)',
          )
        }
        title={t(group.labelKey)}
      >
        <Icon className="h-5 w-5 shrink-0" aria-hidden="true" />
      </NavLink>
    );
  }

  return (
    <div>
      {/* 그룹 헤더 */}
      <button
        type="button"
        onClick={onToggle}
        className={cn(
          'flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
          'hover:bg-(--color-bg-elevated)',
          hasActiveChild
            ? 'text-blue-700 dark:text-blue-400'
            : 'text-(--color-text-secondary)',
        )}
      >
        <Icon className="h-5 w-5 shrink-0" aria-hidden="true" />
        <span className="flex-1 text-left">{t(group.labelKey)}</span>
        <ChevronDown
          className={cn(
            'h-4 w-4 shrink-0 transition-transform duration-200',
            !isOpen && '-rotate-90',
          )}
          aria-hidden="true"
        />
      </button>

      {/* 하위 항목 */}
      {isOpen && (
        <div className="ml-4 mt-0.5 space-y-0.5 border-l border-(--color-border-default) pl-3">
          {visibleChildren.map((child) => {
            const ChildIcon = child.icon;
            return (
              <NavLink
                key={child.path}
                to={child.path}
                className={({ isActive }) =>
                  cn(
                    'flex items-center gap-3 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
                    'hover:bg-(--color-bg-elevated)',
                    isActive
                      ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
                      : 'text-(--color-text-muted)',
                  )
                }
              >
                <ChildIcon className="h-4 w-4 shrink-0" aria-hidden="true" />
                <span>{t(child.labelKey)}</span>
              </NavLink>
            );
          })}
        </div>
      )}
    </div>
  );
}
