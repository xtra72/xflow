// 헤더 컴포넌트.
// 페이지 제목, 사용자 정보, WebSocket 연결 상태, 테마 선택, 로그아웃을 표시한다.
// 대시보드 라우트('/')에서는 대시보드 선택/관리 컨트롤을 표시한다.
// 디바이스 라우트('/devices')에서는 디바이스 추가 버튼을 표시한다.
//
// SPEC-WEB-006 v0.1.0 (M4): 우측 액션에 UpdateAvailableBadge 통합.
//   - useSystemVersion 으로 60초 폴링된 update_available/latest_version 사용.
//   - 비관리자(admin 이외)에서는 disabled.
//   - false→true 전이 시 useUpdateAvailableNotification 이 info 토스트 발화.
//   - 클릭 시 /admin/system 라우트로 이동 (Phase F 의 라우팅 등록 후 동작).
//
// @spec SPEC-WEB-006 v0.1.0 (M4)

import { useEffect, useRef, useState } from 'react';
import { ChevronDown, Key, LogOut, Pencil, Plus, Trash2, Star } from 'lucide-react';
import { useLocation, useNavigate } from 'react-router';

import { useAuth } from '@/hooks/useAuth';
import { useRemoteNodeDetail } from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { useWebSocket } from '@/hooks/useWebSocket';
import { cn } from '@/lib/utils/cn';
import { resolveRemoteNodeLabel } from '@/lib/remote/nodeLabel';
import { useSystemVersion } from '@/services/api/systemUpdate';
import { useUIStore } from '@/stores/uiStore';
import { ThemeSelector } from '@/components/theme/ThemeSelector';
import { ThemeEditorModal } from '@/components/theme/ThemeEditorModal';
import { UpdateAvailableBadge } from '@/components/system/UpdateAvailableBadge';
import CreateDashboardDialog from '@/pages/dashboard/CreateDashboardDialog';
import ChangePasswordDialog from '@/pages/auth/ChangePasswordDialog';
import type { ConnectionState } from '@/services/ws/wsClient';

/**
 * 원격 노드 컨텍스트 경로에서 instanceId 를 추출한다.
 *
 * 원격 노드 하위 경로(`/admin/remote/nodes/:instanceId/...`, 원격 플로우 편집기
 * 포함)에서만 instanceId 를 돌려준다. 그 외 경로는 null 을 반환한다.
 */
function extractRemoteInstanceId(pathname: string): string | null {
  const match = pathname.match(/^\/admin\/remote\/nodes\/([^/]+)/);
  return match ? decodeURIComponent(match[1]!) : null;
}

/** 라우트 경로에 따른 페이지 제목 번역 키 매핑 */
const PAGE_TITLE_KEYS: Record<string, string> = {
  '/': 'nav.dashboard',
  '/flows': 'nav.flows',
  '/agents': 'nav.agents',
  '/devices': 'nav.devices',
  '/nodes': 'nav.nodes',
  '/agent-types': 'nav.agentTypes',
  '/monitoring': 'nav.monitoring',
  '/settings': 'nav.settings',
};

/** WebSocket 연결 상태에 따른 표시 색상 */
const CONNECTION_STYLES: Record<ConnectionState, { dot: string; labelKey: string }> = {
  connected: {
    dot: 'bg-green-500',
    labelKey: 'status.connected',
  },
  connecting: {
    dot: 'bg-yellow-500 animate-pulse',
    labelKey: 'status.connecting',
  },
  reconnecting: {
    dot: 'bg-yellow-500 animate-pulse',
    labelKey: 'status.reconnecting',
  },
  disconnected: {
    dot: 'bg-red-500',
    labelKey: 'status.disconnected',
  },
};

/**
 * 앱 상단 헤더.
 * 현재 페이지 제목, 사용자 정보, 연결 상태, 테마 및 로그아웃 버튼을 표시한다.
 */
export default function Header() {
  const { t } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();
  const { user, authEnabled, logout } = useAuth();
  const { state: wsState } = useWebSocket();

  // SPEC-WEB-006 (M4): 시스템 버전 폴링 결과 — UpdateAvailableBadge 가 소비.
  // useSystemVersion 은 React Query cache 로 dedupe 되므로 다른 곳에서도
  // 호출되어 있으면 추가 네트워크 요청은 발생하지 않는다.
  const { data: versionInfo } = useSystemVersion();
  // 관리자만 시스템 업데이트 페이지 접근 가능. authEnabled=false 일 때는
  // 인증 자체가 비활성이므로 항상 활성화한다 (단일-사용자 dev 모드).
  const updateBadgeDisabled = authEnabled ? user?.role !== 'admin' : false;
  const [editorOpen, setEditorOpen] = useState(false);
  const [passwordDialogOpen, setPasswordDialogOpen] = useState(false);
  const [userMenuOpen, setUserMenuOpen] = useState(false);
  const userMenuRef = useRef<HTMLDivElement>(null);

  // 원격 노드 컨텍스트 제목 — 원격 노드 하위 경로(원격 플로우 편집기 포함)에서는
  // 사이드바 브랜드("XFlow")와 중복되는 폴백 대신 대상 노드 이름을 보여준다.
  // 호스트명은 노드 상세에서 해석하고, 미조회/지연 시 단축 instanceId 로 폴백한다
  // (원시 UUID 전체는 제목에 노출하지 않음). 상세 조회는 렌더를 막지 않는다.
  const remoteInstanceId = extractRemoteInstanceId(location.pathname);
  const { data: remoteNodeDetail } = useRemoteNodeDetail(
    remoteInstanceId ?? '',
    remoteInstanceId !== null,
  );
  const remoteTitle = remoteInstanceId
    ? `${t('remote.header.titlePrefix')} · ${resolveRemoteNodeLabel(
        remoteNodeDetail?.hostname,
        remoteInstanceId,
      )}`
    : null;

  // 대시보드 관리 상태
  const isDashboardRoute = location.pathname === '/';
  const dashboardPages = useUIStore((s) => s.dashboardPages);
  const activeDashboardId = useUIStore((s) => s.activeDashboardId);
  const setActiveDashboard = useUIStore((s) => s.setActiveDashboard);
  const removeDashboardPage = useUIStore((s) => s.removeDashboardPage);
  const setDefaultDashboardPage = useUIStore((s) => s.setDefaultDashboardPage);
  const renameDashboardPage = useUIStore((s) => s.renameDashboardPage);

  const activePage = isDashboardRoute
    ? dashboardPages.find((p) => p.id === activeDashboardId)
    : undefined;
  const isOnlyPage = dashboardPages.length <= 1;
  const isDefaultPage = activePage?.isDefault ?? false;

  const [createOpen, setCreateOpen] = useState(false);
  const [renaming, setRenaming] = useState(false);
  const [renameName, setRenameName] = useState('');
  const renameInputRef = useRef<HTMLInputElement>(null);

  // 사용자 메뉴 외부 클릭 시 닫기
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (userMenuRef.current && !userMenuRef.current.contains(e.target as Node)) {
        setUserMenuOpen(false);
      }
    };
    if (userMenuOpen) {
      document.addEventListener('mousedown', handleClickOutside);
    }
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [userMenuOpen]);

  // 이름 편집 모드 활성화 시 포커스
  useEffect(() => {
    if (renaming) {
      requestAnimationFrame(() => {
        renameInputRef.current?.focus();
        renameInputRef.current?.select();
      });
    }
  }, [renaming]);

  /** 이름 편집 시작 */
  const handleStartRename = () => {
    setRenameName(activePage?.name ?? '');
    setRenaming(true);
  };

  /** 이름 편집 확정 */
  const handleConfirmRename = () => {
    const trimmed = renameName.trim();
    if (trimmed && trimmed !== activePage?.name) {
      renameDashboardPage(activeDashboardId, trimmed);
    }
    setRenaming(false);
  };

  /** 이름 편집 키보드 핸들러 */
  const handleRenameKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleConfirmRename();
    } else if (e.key === 'Escape') {
      setRenaming(false);
    }
  };

  /** 대시보드 삭제 */
  const handleDelete = () => {
    if (isOnlyPage) return;
    const confirmed = window.confirm(
      t('header.dashboard.deleteConfirm').replace('{name}', activePage?.name ?? ''),
    );
    if (confirmed) {
      removeDashboardPage(activeDashboardId);
    }
  };

  // 현재 라우트에서 페이지 제목 결정. 원격 노드 컨텍스트에서는 노드 이름을
  // 우선 사용한다(폴백 'XFlow' 가 사이드바 브랜드와 중복되지 않도록).
  const pageTitle = remoteTitle ?? derivePageTitle(location.pathname, t);
  const connectionStyle = CONNECTION_STYLES[wsState];

  // 대시보드 라우트에서는 DashboardPage가 자체 헤더를 렌더링하므로 앱 헤더를 숨긴다.
  if (isDashboardRoute) return null;

  return (
    <header className="flex h-(--header-height) shrink-0 items-center justify-between border-b border-(--color-border-default) bg-(--color-bg-surface) px-6">
      {/* 좌측: 페이지 제목 또는 대시보드 관리 */}
      {isDashboardRoute ? (
        <div className="flex items-center gap-2">
          {/* 대시보드 선택 / 이름 편집 */}
          {renaming ? (
            <input
              ref={renameInputRef}
              type="text"
              value={renameName}
              onChange={(e) => setRenameName(e.target.value)}
              onBlur={handleConfirmRename}
              onKeyDown={handleRenameKeyDown}
              className="rounded-md border border-blue-500 bg-(--color-bg-surface) px-2 py-1 text-sm font-semibold text-(--color-text-primary) outline-none ring-1 ring-blue-500"
              aria-label={t('header.dashboard.renameAria')}
            />
          ) : (
            <select
              value={activeDashboardId}
              onChange={(e) => setActiveDashboard(e.target.value)}
              className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1 text-sm font-semibold text-(--color-text-primary)"
              aria-label={t('header.dashboard.selectAria')}
            >
              {dashboardPages.map((page) => (
                <option key={page.id} value={page.id}>
                  {page.isDefault ? `\u2605 ${page.name}` : page.name}
                </option>
              ))}
            </select>
          )}

          {/* 이름 편집 */}
          <button
            type="button"
            onClick={handleStartRename}
            disabled={renaming}
            className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-40"
            aria-label={t('header.dashboard.renameAria')}
            title={t('header.dashboard.renameAria')}
          >
            <Pencil className="h-3.5 w-3.5" />
          </button>

          {/* 대시보드 추가 */}
          <button
            type="button"
            onClick={() => setCreateOpen(true)}
            className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated)"
            aria-label={t('header.dashboard.add')}
            title={t('header.dashboard.add')}
          >
            <Plus className="h-3.5 w-3.5" />
          </button>

          {/* 대시보드 삭제 */}
          <button
            type="button"
            onClick={handleDelete}
            disabled={isOnlyPage}
            className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
            aria-label={t('header.dashboard.delete')}
            title={t('header.dashboard.delete')}
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>

          {/* 기본 대시보드 설정 */}
          <button
            type="button"
            onClick={() => !isDefaultPage && setDefaultDashboardPage(activeDashboardId)}
            disabled={isDefaultPage}
            className={cn(
              'rounded-md p-1 transition-colors',
              isDefaultPage
                ? 'text-yellow-500 dark:text-yellow-400'
                : 'text-(--color-text-muted) hover:bg-(--color-bg-elevated)',
              'disabled:cursor-not-allowed disabled:opacity-40',
            )}
            aria-label={t('header.dashboard.setDefault')}
            title={t('header.dashboard.setDefault')}
          >
            <Star className={cn('h-3.5 w-3.5', isDefaultPage && 'fill-current')} />
          </button>
        </div>
      ) : (
        <h1 className="text-lg font-semibold text-(--color-text-primary)">{pageTitle}</h1>
      )}

      {/* 우측 액션 영역 */}
      <div className="flex items-center gap-4">
        {/* WebSocket 연결 상태 */}
        <div className="flex items-center gap-2" title={`WebSocket: ${t(connectionStyle.labelKey)}`}>
          <span
            className={cn('inline-block h-2 w-2 rounded-full', connectionStyle.dot)}
            aria-hidden="true"
          />
          <span className="text-xs text-(--color-text-muted)">{t(connectionStyle.labelKey)}</span>
        </div>

        {/* SPEC-WEB-006 (M4): 시스템 업데이트 가용 배지 */}
        <UpdateAvailableBadge
          available={versionInfo?.update_available ?? false}
          latestVersion={versionInfo?.latest_version ?? null}
          disabled={updateBadgeDisabled}
          onClick={() => {
            // Phase F 에서 라우트 등록 후 정상 동작. 미등록 상태에서는
            // 단순 navigate 호출이 noop 처리된다.
            navigate('/admin/system');
          }}
        />

        {/* 테마 선택 */}
        <ThemeSelector onOpenEditor={() => setEditorOpen(true)} />

        {/* 커스텀 테마 에디터 */}
        <ThemeEditorModal isOpen={editorOpen} onClose={() => setEditorOpen(false)} />

        {/* 사용자 메뉴 (인증 활성화 시만 표시) */}
        {authEnabled && user ? (
          <div className="relative" ref={userMenuRef}>
            <button
              type="button"
              onClick={() => setUserMenuOpen(!userMenuOpen)}
              className={cn(
                'flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors',
                'text-(--color-text-secondary) hover:bg-(--color-bg-sunken)',
              )}
              aria-expanded={userMenuOpen}
              aria-haspopup="true"
            >
              <span className="font-medium">{user.name}</span>
              <span
                className={cn(
                  'rounded-full px-2 py-0.5 text-xs font-medium',
                  user.role === 'admin'
                    ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                    : user.role === 'editor'
                      ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
                      : 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300',
                )}
              >
                {user.role}
              </span>
              <ChevronDown className="h-4 w-4" aria-hidden="true" />
            </button>

            {/* 드롭다운 메뉴 */}
            {userMenuOpen && (
              <div className="absolute right-0 top-full z-50 mt-1 w-48 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) py-1 shadow-lg">
                <button
                  type="button"
                  onClick={() => {
                    setUserMenuOpen(false);
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
                    setUserMenuOpen(false);
                    void logout();
                  }}
                  className="flex w-full items-center gap-2 px-3 py-2 text-sm text-red-600 hover:bg-(--color-bg-elevated) dark:text-red-400"
                >
                  <LogOut className="h-4 w-4" aria-hidden="true" />
                  {t('auth.logout')}
                </button>
              </div>
            )}
          </div>
        ) : user ? (
          /* 인증 비활성화 시 기존 사용자 정보 표시 */
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-(--color-text-secondary)">
              {user.name}
            </span>
            <span
              className={cn(
                'rounded-full px-2 py-0.5 text-xs font-medium',
                user.role === 'admin'
                  ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                  : user.role === 'editor'
                    ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
                    : 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300',
              )}
            >
              {user.role}
            </span>
          </div>
        ) : null}
      </div>
      {/* 대시보드 생성 다이얼로그 */}
      {isDashboardRoute && (
        <CreateDashboardDialog
          open={createOpen}
          onClose={() => setCreateOpen(false)}
        />
      )}

      {/* 비밀번호 변경 다이얼로그 */}
      <ChangePasswordDialog
        open={passwordDialogOpen}
        onClose={() => setPasswordDialogOpen(false)}
      />
    </header>
  );
}

/**
 * 라우트 경로에서 페이지 제목을 추출한다.
 * 에디터 경로(/editor/:flowId)의 경우 번역된 '플로우'(nav.editor)를 반환한다.
 */
function derivePageTitle(pathname: string, t: (key: string) => string): string {
  // 정적 경로 매핑에서 확인
  if (pathname in PAGE_TITLE_KEYS) {
    return t(PAGE_TITLE_KEYS[pathname]!);
  }

  // 에디터 경로 패턴 (/editor/:flowId)
  if (pathname.startsWith('/editor/')) {
    return t('nav.editor');
  }

  return 'XFlow';
}
