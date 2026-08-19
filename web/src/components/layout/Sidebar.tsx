// 사이드바 네비게이션 컴포넌트.
// 메뉴 항목, 접기/펼치기 토글, 권한 기반 메뉴 필터링을 제공한다.
// 그룹 메뉴(하위 항목 포함)를 지원한다.
//
// SPEC-AUTH-006 S1 (M3.1, M3.2): 역할 이름 열거(`roles: ['admin']`)를 권한 키
//   (`permission: 'remote.read'`)로 교체했다. 커스텀 역할이 생기면 역할 이름
//   열거는 유지할 수 없다 — 커스텀 역할이 어떤 메뉴에도 접근하지 못한다.

import { useState } from 'react';
import {
  Blocks,
  BookOpen,
  Bot,
  CalendarClock,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  HardDrive,
  Layers,
  LayoutDashboard,
  LayoutList,
  Monitor,
  Network,
  Package,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
  UserPlus,
  Users,
  Workflow,
} from 'lucide-react';
import { NavLink, useLocation } from 'react-router';

import { usePermission } from '@/hooks/usePermission';
import { useRemoteMode } from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import SidebarUserMenu from '@/components/layout/SidebarUserMenu';
import { useUIStore } from '@/stores/uiStore';

/** 네비게이션 메뉴 항목 정의 */
interface NavItem {
  /** 번역 키 */
  labelKey: string;
  /** 라우트 경로 */
  path: string;
  /** lucide-react 아이콘 컴포넌트 */
  icon: React.ComponentType<{ className?: string }>;
  /**
   * 데이터 권한 키(`<resource>.<action>`). 메뉴 축 도입 이전 역할의 폴백 판정에
   * 쓰인다. 미지정 시 인증만으로 접근 가능.
   */
  permission?: string;
  /**
   * 메뉴 노출 키(`nav.<menu>`) — SPEC-AUTH-006 E2.
   * 역할이 `nav.*` 를 하나라도 보유하면 이 키로 판정하고, 하나도 없으면
   * `permission` 으로 폴백한다(기존 배포 회귀 방지).
   */
  navPermission?: string;
}

/** 그룹 메뉴 정의 (하위 항목 포함) */
interface NavGroup {
  /** 번역 키 */
  labelKey: string;
  /** lucide-react 아이콘 컴포넌트 */
  icon: React.ComponentType<{ className?: string }>;
  /** 하위 메뉴 항목 */
  children: NavItem[];
  /** 데이터 권한 키(폴백용). 미지정 시 인증만으로 접근 가능. */
  permission?: string;
  /** 메뉴 노출 키(`nav.<menu>`) — SPEC-AUTH-006 E2. */
  navPermission?: string;
}

type NavEntry = NavItem | NavGroup;

/** NavEntry가 그룹인지 판별 */
function isNavGroup(entry: NavEntry): entry is NavGroup {
  return 'children' in entry;
}

/**
 * 메뉴 항목 목록.
 *
 * 권한 매핑은 SPEC-AUTH-006 spec.md §2.3 표를 그대로 옮긴 것이다.
 * 대시보드는 인증만 요구하므로 permission 을 지정하지 않는다 — 권한이 0개인
 * 사용자도 빈 사이드바가 아니라 대시보드 항목은 남는다.
 */
const NAV_ENTRIES: NavEntry[] = [
  {
    labelKey: 'nav.dashboard',
    path: '/',
    icon: LayoutDashboard,
  },
  // SPEC-DASHBOARD-004 M7 7.3: 대시보드 관리 메뉴.
  //   `nav.dashboard` 는 **이 관리 항목에만** 건다. 바로 위 대시보드 항목은
  //   permission/navPermission 미지정 상태를 유지한다 — 대시보드를 *보는* 것은
  //   인증만 요구하며, 여기에 키를 걸면 권한 0개 사용자가 빈 사이드바를 보게 되어
  //   catalog.go 가 명시한 원칙을 위반한다(spec.md §2.5).
  //
  //   데이터 권한(`permission`)은 지정하지 않는다. 메뉴 노출 판정은 `nav.*` 만
  //   보므로(SPEC-AUTH-006 E2, 커밋 c26079c9) 여기에 데이터 키를 적으면 존재하지
  //   않는 폴백이 있는 것처럼 읽힌다.
  {
    labelKey: 'nav.dashboardAdmin',
    path: '/dashboards/admin',
    icon: LayoutList,
    navPermission: 'nav.dashboard',
  },
  {
    labelKey: 'nav.flows',
    path: '/flows',
    icon: Workflow,
    permission: 'flow.read',

    navPermission: 'nav.flow',
  },
  {
    labelKey: 'nav.agents',
    path: '/agents',
    icon: Bot,
    permission: 'agent.read',

    navPermission: 'nav.agent',
  },
  {
    labelKey: 'nav.devices',
    path: '/devices',
    icon: HardDrive,
    permission: 'device.read',

    navPermission: 'nav.device',
  },
  {
    labelKey: 'nav.monitoring',
    path: '/monitoring',
    icon: Monitor,
    permission: 'monitoring.read',

    navPermission: 'nav.monitoring',
  },
  // SPEC-SCHEDULE-VIEW-001 M5: 스케줄 뷰.
  {
    labelKey: 'nav.schedules',
    path: '/schedules',
    icon: CalendarClock,
    permission: 'schedule.read',

    navPermission: 'nav.schedule',
  },
  // 참고 그룹 메뉴 — 노드 타입 / 에이전트 타입 카탈로그.
  {
    labelKey: 'nav.reference',
    icon: BookOpen,
    children: [
      {
        labelKey: 'nav.nodes',
        path: '/nodes',
        icon: Blocks,
        permission: 'node.read',

        navPermission: 'nav.node',
      },
      {
        labelKey: 'nav.agentTypes',
        path: '/agent-types',
        icon: Bot,
        permission: 'node.read',

        navPermission: 'nav.node',
      },
    ],
  },
  // SPEC-REMOTE-001 M9 (그룹 K, REQ-K11): 원격 관리 그룹.
  //   노드 관리(운영) + 등록 관리(온보딩) — 기존 관리 노드 + 원격 노드 제어 대체.
  //   원격 하위 화면은 세분 게이팅 없이 `remote.*` 단일 키로만 다룬다
  //   (spec.md §1.3 비범위) — 서버 라우트도 조회는 remote.read 로 통일돼 있다.
  {
    labelKey: 'nav.remote',
    icon: Network,
    permission: 'remote.read',

    navPermission: 'nav.remote',
    children: [
      {
        labelKey: 'nav.nodeManagement',
        path: '/admin/remote',
        icon: SlidersHorizontal,
        permission: 'remote.read',

        navPermission: 'nav.remote',
      },
      {
        labelKey: 'nav.groupManagement',
        path: '/admin/remote/groups',
        icon: Layers,
        permission: 'remote.read',

        navPermission: 'nav.remote',
      },
      {
        labelKey: 'nav.enrollmentManagement',
        path: '/admin/remote/enrollment',
        icon: UserPlus,
        permission: 'remote.read',

        navPermission: 'nav.remote',
      },
      {
        labelKey: 'nav.releaseStore',
        path: '/admin/remote/releases',
        icon: Package,
        permission: 'remote.read',

        navPermission: 'nav.remote',
      },
    ],
  },
  // SPEC-AUTH-006 M3.2: 사용자/역할 관리 메뉴. 페이지 구현은 M2 가 채운다.
  {
    labelKey: 'nav.users',
    path: '/admin/users',
    icon: Users,
    permission: 'user.read',

    navPermission: 'nav.user',
  },
  {
    labelKey: 'nav.roles',
    path: '/admin/roles',
    icon: ShieldCheck,
    permission: 'role.read',

    navPermission: 'nav.role',
  },
  {
    labelKey: 'nav.settings',
    path: '/settings',
    icon: Settings,
    permission: 'system.read',

    navPermission: 'nav.system',
  },
];

/**
 * 앱 사이드바 네비게이션.
 * 접기/펼치기 토글, 활성 메뉴 하이라이트, RBAC 필터링, 그룹 메뉴를 지원한다.
 */
/** 원격 관리 그룹 식별용 라벨 키 (server 모드에서만 노출). */
const REMOTE_GROUP_LABEL_KEY = 'nav.remote';

export default function Sidebar() {
  const { t } = useTranslation();
  const { canSeeMenu } = usePermission();
  // 원격 관리 그룹은 server 모드에서만 노출한다. 로딩 중/비 server 모드면 숨긴다.
  const { data: remoteMode } = useRemoteMode();
  const isRemoteServer = remoteMode?.mode === 'server';
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

  /** 권한 기반 필터링. permission 미지정 항목은 인증만으로 접근 가능하다. */
  // 메뉴 노출은 nav.* 만 본다. 데이터 권한(permission)은 판정에 쓰지 않는다 —
  // 대시보드가 읽어야 하는 데이터와 메뉴 노출은 별개 축이기 때문이다.
  const hasAccess = (entry: { navPermission?: string }) =>
    entry.navPermission ? canSeeMenu(entry.navPermission) : true;

  // 사용자 권한에 따른 메뉴 필터링
  const filteredEntries = NAV_ENTRIES.filter((entry) => {
    if (!hasAccess(entry)) return false;
    // 원격 관리 그룹은 remote.read 권한 + server 모드를 모두 충족할 때만 노출한다.
    if (isNavGroup(entry) && entry.labelKey === REMOTE_GROUP_LABEL_KEY && !isRemoteServer) {
      return false;
    }
    // 그룹의 경우 접근 가능한 하위 항목이 하나라도 있으면 표시 (기존 동작 유지)
    if (isNavGroup(entry)) {
      return entry.children.some((child) => hasAccess(child));
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
        {/* 사용자 메뉴 — 대시보드만 남은 사용자도 로그아웃할 수 있어야 하므로
            항상 렌더되는 사이드바 하단에 둔다(SPEC-AUTH-006). */}
        <SidebarUserMenu collapsed={sidebarCollapsed} />

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
}

/** 그룹 메뉴 (접기/펼치기 가능한 하위 항목 포함) */
function NavGroupItem({ group, isOpen, onToggle, collapsed, t }: NavGroupItemProps) {
  const Icon = group.icon;
  const location = useLocation();
  const { canSeeMenu } = usePermission();

  // 하위 항목 중 활성인 것이 있는지 확인
  const hasActiveChild = group.children.some(
    (child) => location.pathname === child.path,
  );

  // 접근 가능한 하위 항목만 필터링 (상위 필터와 동일한 메뉴 축 판정)
  const visibleChildren = group.children.filter((child) =>
    child.navPermission ? canSeeMenu(child.navPermission) : true,
  );

  // 사이드바가 접힌 상태에서는 그룹의 각 하위 항목을 개별 아이콘으로 렌더한다
  // (접힘에서도 모든 항목 접근 가능 — 예: 노드 관리/등록 관리).
  if (collapsed) {
    if (visibleChildren.length === 0) return null;

    return (
      <>
        {visibleChildren.map((child) => {
          const ChildIcon = child.icon;
          return (
            <NavLink
              key={child.path}
              to={child.path}
              end
              className={({ isActive }) =>
                cn(
                  'flex items-center justify-center rounded-md px-2 py-2 text-sm font-medium transition-colors',
                  'hover:bg-(--color-bg-elevated)',
                  isActive
                    ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
                    : 'text-(--color-text-secondary)',
                )
              }
              title={t(child.labelKey)}
            >
              <ChildIcon className="h-5 w-5 shrink-0" aria-hidden="true" />
            </NavLink>
          );
        })}
      </>
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
                end
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
