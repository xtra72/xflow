// 대시보드 메인 페이지.
// 플로우 현황, 시스템 메트릭, 에이전트 상태를 위젯 형태로 표시하고
// WebSocket을 통해 실시간 업데이트를 수신한다.
// react-grid-layout으로 패널 드래그/리사이즈를 지원한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import GridLayout from 'react-grid-layout';
import { useNavigate } from 'react-router';
import {
  Pencil,
  RefreshCw,
  X,
  Settings,
  Star,
  ChevronDown,
  ChevronRight,
  Plus,
  Sun,
  Moon,
  Monitor,
  Paintbrush,
  Palette as PaletteIcon,
  Save,
  Check,
  Grid3X3,
  ChevronUp,
  Clock,
} from 'lucide-react';

import 'react-grid-layout/css/styles.css';
import 'react-resizable/css/styles.css';

import { useFlows, useWebSocket } from '@/hooks';
import { useTranslation } from '@/lib/i18n';
import { useDashboardSyncStatus } from '@/contexts/DashboardSyncContext';
import { isRemoteTarget, LOCAL_TARGET, type ResourceTarget } from '@/lib/remote/target';
import { getMetrics } from '@/services/api/monitorService';
import { useAuthStore } from '@/stores/authStore';
import {
  useUIStore,
  type DashboardLayoutItem,
  type PanelConfig,
  type ThemeMode,
} from '@/stores/uiStore';
import { WS_MESSAGE_TYPES } from '@/services/ws/wsHandlers';
import type { FlowInfo } from '@/types/flow';

import { renderDashboardPanel } from './renderDashboardPanel';
import RemoteDashboardView from './RemoteDashboardView';

/** 그리드 설정 */
const GRID_MARGIN: [number, number] = [16, 16];

/**
 * 대시보드 페이지 — 로컬/원격 디스패처 (SPEC-REMOTE-001 M10, 그룹 L, REQ-L09/L10).
 *
 * target 이 원격이면 읽기 전용 RemoteDashboardView 로, 로컬(미지정)이면 기존
 * LocalDashboardView 로 라우팅한다. 디스패처 자체는 훅을 호출하지 않으므로 두 뷰는
 * 각자 독립된 훅 트리를 가진다(Rules of Hooks 안전). 로컬 경로는 회귀 없이 동일하다.
 */
export default function DashboardPage({
  target = LOCAL_TARGET,
}: {
  /** 자원 타깃(미지정=로컬). 노드 대시보드가 remote 타깃을 주입한다(REQ-L10). */
  target?: ResourceTarget;
}): React.JSX.Element {
  return isRemoteTarget(target) ? (
    <RemoteDashboardView target={target} />
  ) : (
    <LocalDashboardView />
  );
}

/** 대시보드 자동 갱신 주기 옵션 (초 단위). 라벨은 t('dashboard.refreshOption') 로 렌더. */
const REFRESH_INTERVALS = [
  { value: 5 },
  { value: 10 },
  { value: 15 },
  { value: 30 },
  { value: 60 },
] as const;

/** 테마 모드 옵션 (Pencil 디자인 매칭). labelKey/descKey 는 i18n 키. */
const THEME_OPTIONS: { value: ThemeMode; labelKey: string; descKey: string; icon: React.ReactNode; iconColor: string }[] = [
  { value: 'system', labelKey: 'dashboard.theme.system', descKey: 'dashboard.themeDesc.system', icon: <Monitor className="h-4 w-4" />, iconColor: 'text-blue-500' },
  { value: 'day', labelKey: 'dashboard.theme.day', descKey: 'dashboard.themeDesc.day', icon: <Sun className="h-4 w-4" />, iconColor: 'text-amber-500' },
  { value: 'night', labelKey: 'dashboard.theme.night', descKey: 'dashboard.themeDesc.night', icon: <Moon className="h-4 w-4" />, iconColor: 'text-indigo-500' },
  { value: 'custom', labelKey: 'dashboard.theme.custom', descKey: 'dashboard.themeDesc.custom', icon: <Paintbrush className="h-4 w-4" />, iconColor: 'text-violet-500' },
];

/** 로컬 대시보드 뷰 — 기존 DashboardPage 본문(편집/sync 포함). 회귀 없이 동일하다. */
function LocalDashboardView() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  // SPEC-DASHBOARD-001 v0.2.0: 서버 snapshot 동기화 상태(읽기 전용).
  // 실제 동기화 훅(useDashboardSync)은 AppLayout 이 호출한다 — 패널 추가/설정
  // 라우트로 이동해도 구독이 끊기지 않아야 하기 때문(AppLayout 주석 참조).
  const { pendingSync } = useDashboardSyncStatus();

  // SPEC-DASHBOARD-001 v0.2.0: 활성 스코프 (탭) + 사용자 역할.
  const activeDashboardScope = useUIStore((s) => s.activeDashboardScope);
  const setActiveDashboardScope = useUIStore((s) => s.setActiveDashboardScope);
  const userRole = useAuthStore((s) => s.user?.role);
  const isAdmin = userRole === 'admin';
  /** 공유 탭이면서 admin 이 아닌 경우 편집 컨트롤 사전 비활성화 (AC-4 UX). */
  const sharedReadOnly = activeDashboardScope === 'shared' && !isAdmin;

  // UI store
  const refreshInterval = useUIStore((s) => s.dashboardRefreshInterval);
  const setRefreshInterval = useUIStore((s) => s.setDashboardRefreshInterval);

  const dashboardPages = useUIStore((s) => s.dashboardPages);
  const activeDashboardId = useUIStore((s) => s.activeDashboardId);
  const setActiveDashboard = useUIStore((s) => s.setActiveDashboard);
  const addDashboardPage = useUIStore((s) => s.addDashboardPage);
  const setDefaultDashboardPage = useUIStore((s) => s.setDefaultDashboardPage);
  const renameDashboardPage = useUIStore((s) => s.renameDashboardPage);
  const activePage = useUIStore((s) =>
    s.dashboardPages.find((p) => p.id === s.activeDashboardId),
  );
  const layout = activePage?.layout ?? [];
  const panels = activePage?.panels ?? [];
  const setLayout = useUIStore((s) => s.setDashboardLayout);
  const editMode = useUIStore((s) => s.dashboardEditMode);
  const setEditMode = useUIStore((s) => s.setDashboardEditMode);
  const removePanel = useUIStore((s) => s.removePanel);
  const updatePanelConfig = useUIStore((s) => s.updatePanelConfig);
  const updatePanelTitle = useUIStore((s) => s.updatePanelTitle);
  const theme = useUIStore((s) => s.theme);
  const setTheme = useUIStore((s) => s.setTheme);
  const gridCols = useUIStore((s) => s.dashboardGridCols);
  const setGridCols = useUIStore((s) => s.setDashboardGridCols);
  const showGridLines = useUIStore((s) => s.dashboardShowGridLines);
  const setShowGridLines = useUIStore((s) => s.setDashboardShowGridLines);
  const refreshMs = refreshInterval * 1000;

  // 대시보드 드롭다운 열기/닫기
  const [dashboardDropdownOpen, setDashboardDropdownOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);
  // 인라인 이름 편집 상태
  const [editingPageId, setEditingPageId] = useState<string | null>(null);
  const [editingName, setEditingName] = useState('');

  // 테마 드롭다운
  const [themeDropdownOpen, setThemeDropdownOpen] = useState(false);
  const themeDropdownRef = useRef<HTMLDivElement>(null);
  // 그리드 드롭다운
  const [gridDropdownOpen, setGridDropdownOpen] = useState(false);
  const gridDropdownRef = useRef<HTMLDivElement>(null);
  // 갱신 주기 드롭다운 (일반 모드 헤더)
  const [refreshDropdownOpen, setRefreshDropdownOpen] = useState(false);
  const refreshDropdownRef = useRef<HTMLDivElement>(null);
  // 패널 생성/설정: 모달 대신 딥링크 라우트로 이동한다(편집 상태는 uiStore 로 보존).
  const navigate = useNavigate();

  // 컨테이너 너비 측정 (그리드 영역) — 콜백 ref로 조건부 렌더링 대응
  const containerRef = useRef<HTMLDivElement>(null);
  const gridRef = useRef<HTMLDivElement>(null);
  const gridObserverRef = useRef<ResizeObserver | null>(null);
  const [containerWidth, setContainerWidth] = useState(0);

  const gridCallbackRef = useCallback((node: HTMLDivElement | null) => {
    // 이전 observer 해제
    if (gridObserverRef.current) {
      gridObserverRef.current.disconnect();
      gridObserverRef.current = null;
    }
    gridRef.current = node;
    if (!node) return;

    setContainerWidth(node.clientWidth);
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width);
      }
    });
    observer.observe(node);
    gridObserverRef.current = observer;
  }, []);

  // 언마운트 시 observer 해제
  useEffect(() => {
    return () => {
      if (gridObserverRef.current) {
        gridObserverRef.current.disconnect();
      }
    };
  }, []);

  // 칼럼 폭에 맞춘 동적 행 높이 (정사각형 셀)
  const gridRowHeight = containerWidth > 0
    ? Math.round((containerWidth - GRID_MARGIN[0] * (gridCols - 1)) / gridCols)
    : 45;

  // 드롭다운 외부 클릭 닫기
  useEffect(() => {
    if (!dashboardDropdownOpen && !themeDropdownOpen && !gridDropdownOpen && !refreshDropdownOpen) return;
    const handleClickOutside = (e: MouseEvent) => {
      if (dashboardDropdownOpen && dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setDashboardDropdownOpen(false);
      }
      if (themeDropdownOpen && themeDropdownRef.current && !themeDropdownRef.current.contains(e.target as Node)) {
        setThemeDropdownOpen(false);
      }
      if (gridDropdownOpen && gridDropdownRef.current && !gridDropdownRef.current.contains(e.target as Node)) {
        setGridDropdownOpen(false);
      }
      if (refreshDropdownOpen && refreshDropdownRef.current && !refreshDropdownRef.current.contains(e.target as Node)) {
        setRefreshDropdownOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [dashboardDropdownOpen, themeDropdownOpen, gridDropdownOpen, refreshDropdownOpen]);

  // 데이터 로드
  const {
    data: flowsData,
    isLoading: flowsLoading,
    error: flowsError,
  } = useFlows();

  const {
    data: metrics,
    isLoading: metricsLoading,
  } = useQuery({
    queryKey: ['monitor', 'metrics'],
    queryFn: getMetrics,
    refetchInterval: refreshMs,
  });

  const flows: FlowInfo[] = flowsData?.data ?? [];
  const isLoading = flowsLoading || metricsLoading;

  // WebSocket 실시간 업데이트
  const { state: wsState, client: wsClient } = useWebSocket();

  useEffect(() => {
    if (wsState !== 'connected' || !wsClient) return;

    const handleFlowStatus = () => {
      queryClient.invalidateQueries({ queryKey: ['flows'] });
    };
    const handleFlowMetrics = () => {
      queryClient.invalidateQueries({ queryKey: ['monitor', 'metrics'] });
    };
    const handleAgentStatus = () => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
    };
    const handleSystemEvent = () => {
      queryClient.invalidateQueries({ queryKey: ['flows'] });
      queryClient.invalidateQueries({ queryKey: ['monitor', 'metrics'] });
      queryClient.invalidateQueries({ queryKey: ['agents'] });
    };

    wsClient.on(WS_MESSAGE_TYPES.FLOW_STATUS, handleFlowStatus);
    wsClient.on(WS_MESSAGE_TYPES.FLOW_METRICS, handleFlowMetrics);
    wsClient.on(WS_MESSAGE_TYPES.AGENT_STATUS, handleAgentStatus);
    wsClient.on(WS_MESSAGE_TYPES.SYSTEM_EVENT, handleSystemEvent);

    return () => {
      wsClient.off(WS_MESSAGE_TYPES.FLOW_STATUS, handleFlowStatus);
      wsClient.off(WS_MESSAGE_TYPES.FLOW_METRICS, handleFlowMetrics);
      wsClient.off(WS_MESSAGE_TYPES.AGENT_STATUS, handleAgentStatus);
      wsClient.off(WS_MESSAGE_TYPES.SYSTEM_EVENT, handleSystemEvent);
    };
  }, [wsState, wsClient, queryClient]);

  const handleRefresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['flows'] });
    queryClient.invalidateQueries({ queryKey: ['monitor', 'metrics'] });
    queryClient.invalidateQueries({ queryKey: ['agents'] });
  }, [queryClient]);

  /** 레이아웃 변경 핸들러 */
  const handleLayoutChange = useCallback(
    (newLayout: DashboardLayoutItem[]) => {
      setLayout(newLayout);
    },
    [setLayout],
  );

  /** 패널 공통 config/title 변경 콜백 생성 */
  const configChangeFor = useCallback(
    (panelId: string) => (c: Record<string, unknown>) => updatePanelConfig(panelId, c),
    [updatePanelConfig],
  );
  const titleChangeFor = useCallback(
    (panelId: string) => (t: string) => updatePanelTitle(panelId, t),
    [updatePanelTitle],
  );

  /** 인라인 이름 편집 시작 */
  const startRename = (pageId: string, currentName: string) => {
    setEditingPageId(pageId);
    setEditingName(currentName);
  };

  /** 인라인 이름 편집 완료 */
  const finishRename = () => {
    if (editingPageId && editingName.trim()) {
      renameDashboardPage(editingPageId, editingName.trim());
    }
    setEditingPageId(null);
    setEditingName('');
  };

  /** 패널 타입에 따라 적절한 위젯 컴포넌트를 렌더링(공유 렌더러 재사용 — REQ-L09). */
  const renderPanel = (panel: PanelConfig, flowsList: FlowInfo[], metricsData: typeof metrics) =>
    renderDashboardPanel(panel, flowsList, metricsData, refreshMs, (panelId) => ({
      onConfigChange: configChangeFor(panelId),
      onTitleChange: titleChangeFor(panelId),
    }));

  const hasError = flowsError;

  // 현재 활성 대시보드 이름
  const activePageName = activePage?.name ?? t('dashboard.fallbackName');

  return (
    <div className="-m-6 flex flex-1 flex-col" ref={containerRef}>
      {/* SPEC-DASHBOARD-001 v0.2.0: 공유/내 대시보드 탭 토글 (헤더 위) */}
      <div
        role="tablist"
        aria-label={t('dashboard.scope.aria')}
        className="flex h-9 shrink-0 items-center gap-1 border-b border-(--color-border-default) bg-(--color-bg-surface) px-6"
      >
        <button
          type="button"
          role="tab"
          aria-selected={activeDashboardScope === 'shared'}
          aria-label={t('dashboard.scope.sharedAria')}
          onClick={() => setActiveDashboardScope('shared')}
          className={`inline-flex h-7 items-center rounded-md px-3 text-[12px] font-medium transition-colors ${
            activeDashboardScope === 'shared'
              ? 'bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400'
              : 'text-(--color-text-muted) hover:bg-(--color-bg-elevated)'
          }`}
        >
          {t('dashboard.scope.shared')}
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeDashboardScope === 'mine'}
          aria-label={t('dashboard.scope.mineAria')}
          onClick={() => setActiveDashboardScope('mine')}
          className={`inline-flex h-7 items-center rounded-md px-3 text-[12px] font-medium transition-colors ${
            activeDashboardScope === 'mine'
              ? 'bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400'
              : 'text-(--color-text-muted) hover:bg-(--color-bg-elevated)'
          }`}
        >
          {t('dashboard.scope.mine')}
        </button>
        {/* 동기화 인디케이터 + 읽기 전용 뱃지 */}
        <div className="ml-auto flex items-center gap-3">
          {sharedReadOnly && (
            <span
              className="text-[11px] text-(--color-text-muted)"
              title={t('dashboard.scope.adminOnly')}
            >
              {t('dashboard.scope.readOnly')}
            </span>
          )}
          {pendingSync && (
            <span
              className="text-[11px] text-(--color-text-muted)"
              aria-live="polite"
            >
              {t('dashboard.scope.syncing')}
            </span>
          )}
        </div>
      </div>

      {/* ── 헤더 바 (Pencil: 56px, 흰색, border-bottom) ── */}
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-(--color-border-default) bg-(--color-bg-surface) px-6">
        {editMode ? (
          /* ── 편집 모드 헤더 좌측: 타이틀 + 앰버 뱃지 ── */
          <div className="flex items-center gap-3">
            <span className="text-base font-semibold text-(--color-text-primary)">
              {activePageName}
            </span>
            <span className="inline-flex items-center rounded-md bg-amber-50 px-2.5 py-1 text-[11px] font-semibold text-amber-500 dark:bg-amber-900/30 dark:text-amber-400">
              {t('dashboard.editMode')}
            </span>
          </div>
        ) : (
          /* ── 일반 모드 헤더 좌측: 대시보드 셀렉터 ── */
          <div className="relative" ref={dropdownRef}>
            <button
              type="button"
              onClick={() => setDashboardDropdownOpen((prev) => !prev)}
              className="inline-flex items-center gap-1.5 rounded-lg p-1 text-base font-semibold text-(--color-text-primary) transition-colors hover:bg-(--color-bg-elevated)"
            >
              {activePageName}
              <ChevronDown className={`h-4 w-4 text-(--color-text-muted) transition-transform ${dashboardDropdownOpen ? 'rotate-180' : ''}`} />
            </button>

            {/* 대시보드 드롭다운 목록 (Pencil: Dx9JA) */}
            {dashboardDropdownOpen && (
              <div className="absolute left-0 top-full z-50 mt-1 w-[220px] overflow-hidden rounded-[10px] border border-(--color-border-default) bg-(--color-bg-surface) shadow-lg">
                <div className="flex flex-col gap-0.5 p-1.5">
                  {dashboardPages.map((page) => (
                    <div
                      key={page.id}
                      className={`flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors hover:bg-(--color-bg-elevated) ${
                        page.id === activeDashboardId ? 'bg-(--color-bg-elevated) font-medium' : ''
                      }`}
                    >
                      {editingPageId === page.id ? (
                        <input
                          type="text"
                          value={editingName}
                          onChange={(e) => setEditingName(e.target.value)}
                          onBlur={finishRename}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') finishRename();
                            if (e.key === 'Escape') { setEditingPageId(null); setEditingName(''); }
                          }}
                          className="flex-1 rounded border border-blue-400 bg-(--color-bg-surface) px-1 py-0.5 text-sm outline-none focus:ring-1 focus:ring-blue-400"
                          autoFocus
                        />
                      ) : (
                        <button
                          type="button"
                          onClick={() => { setActiveDashboard(page.id); setDashboardDropdownOpen(false); }}
                          className="flex-1 truncate text-left text-(--color-text-primary)"
                        >
                          {page.name}
                        </button>
                      )}
                      <button
                        type="button"
                        onClick={() => setDefaultDashboardPage(page.id)}
                        disabled={sharedReadOnly}
                        aria-disabled={sharedReadOnly}
                        className={`shrink-0 p-0.5 transition-colors ${
                          sharedReadOnly
                            ? 'cursor-not-allowed opacity-40'
                            : page.isDefault
                            ? 'text-yellow-500'
                            : 'text-(--color-text-muted) hover:text-yellow-400'
                        }`}
                        title={sharedReadOnly ? t('dashboard.scope.adminOnly') : page.isDefault ? t('dashboard.header.defaultTitle') : t('dashboard.header.setDefault')}
                      >
                        <Star className={`h-3.5 w-3.5 ${page.isDefault ? 'fill-current' : ''}`} />
                      </button>
                      <button
                        type="button"
                        onClick={() => startRename(page.id, page.name)}
                        disabled={sharedReadOnly}
                        aria-disabled={sharedReadOnly}
                        className={`shrink-0 p-0.5 transition-colors ${
                          sharedReadOnly
                            ? 'cursor-not-allowed text-(--color-text-muted) opacity-40'
                            : 'text-(--color-text-muted) hover:text-(--color-text-primary)'
                        }`}
                        title={sharedReadOnly ? t('dashboard.scope.adminOnly') : t('dashboard.header.rename')}
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  ))}
                </div>
                <div className="border-t border-(--color-border-default)">
                  <button
                    type="button"
                    onClick={() => { addDashboardPage(t('dashboard.header.newDashboardName')); setDashboardDropdownOpen(false); }}
                    disabled={sharedReadOnly}
                    aria-disabled={sharedReadOnly}
                    title={sharedReadOnly ? t('dashboard.scope.adminOnly') : undefined}
                    className={`flex w-full items-center justify-center gap-2 px-4 py-2.5 text-sm transition-colors ${
                      sharedReadOnly
                        ? 'cursor-not-allowed text-(--color-text-muted) opacity-40'
                        : 'text-blue-600 hover:bg-(--color-bg-elevated) dark:text-blue-400'
                    }`}
                  >
                    <Plus className="h-3.5 w-3.5" />
                    {t('dashboard.addDashboard')}
                  </button>
                </div>
              </div>
            )}
          </div>
        )}

        {/* 헤더 우측 */}
        <div className="flex items-center gap-2">
          {editMode ? (
            /* ── 편집 모드 우측: 테마 + 패널추가 + 취소 + 저장 ── */
            <>
              {/* 테마 셀렉터 버튼 (Pencil: SO2aH) */}
              <div className="relative" ref={themeDropdownRef}>
                <button
                  type="button"
                  onClick={() => setThemeDropdownOpen((prev) => !prev)}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-(--color-border-default) bg-(--color-bg-elevated) px-3.5 py-2 text-[13px] font-medium text-(--color-text-muted) transition-colors hover:bg-(--color-border-default)"
                >
                  <PaletteIcon className="h-3.5 w-3.5" />
                  {t('dashboard.header.theme')}
                  <ChevronDown className="h-3 w-3 text-(--color-text-muted)" />
                </button>

                {/* 테마 드롭다운 (Pencil: vs5tn) */}
                {themeDropdownOpen && (
                  <div className="absolute right-0 top-full z-50 mt-1 w-[280px] overflow-hidden rounded-[10px] border border-(--color-border-default) bg-(--color-bg-surface) shadow-lg">
                    <div className="px-4 py-2.5">
                      <span className="text-[13px] font-semibold text-(--color-text-primary)">{t('dashboard.header.themeTitle')}</span>
                    </div>
                    <div className="h-px bg-(--color-border-default)" />
                    <div className="flex flex-col gap-0.5 p-1.5">
                      {THEME_OPTIONS.map((opt) => (
                        <button
                          key={opt.value}
                          type="button"
                          onClick={() => { setTheme(opt.value); setThemeDropdownOpen(false); }}
                          className={`flex items-center justify-between rounded-md px-3 py-2.5 transition-colors ${
                            theme === opt.value
                              ? 'bg-blue-50 dark:bg-blue-900/20'
                              : 'hover:bg-(--color-bg-elevated)'
                          }`}
                        >
                          <div className="flex items-center gap-2.5">
                            <span className={opt.iconColor}>{opt.icon}</span>
                            <div className="flex flex-col items-start gap-0.5">
                              <span className={`text-[13px] font-medium ${theme === opt.value ? 'text-blue-600 dark:text-blue-400' : 'text-(--color-text-primary)'}`}>
                                {t(opt.labelKey)}
                              </span>
                              <span className="text-[11px] text-(--color-text-muted)">{t(opt.descKey)}</span>
                            </div>
                          </div>
                          {theme === opt.value ? (
                            <Check className="h-3.5 w-3.5 text-blue-500" />
                          ) : opt.value === 'custom' ? (
                            <ChevronRight className="h-3.5 w-3.5 text-(--color-text-muted)" />
                          ) : (
                            <span className="h-3.5 w-3.5" />
                          )}
                        </button>
                      ))}
                    </div>
                  </div>
                )}
              </div>

              {/* 그리드 설정 드롭다운 */}
              <div className="relative" ref={gridDropdownRef}>
                <button
                  type="button"
                  onClick={() => setGridDropdownOpen((prev) => !prev)}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-(--color-border-default) bg-(--color-bg-elevated) px-3.5 py-2 text-[13px] font-medium text-(--color-text-muted) transition-colors hover:bg-(--color-border-default)"
                >
                  <Grid3X3 className="h-3.5 w-3.5" />
                  {t('dashboard.header.grid')}
                  <ChevronDown className="h-3 w-3 text-(--color-text-muted)" />
                </button>

                {gridDropdownOpen && (
                  <div className="absolute right-0 top-full z-50 mt-1 w-[200px] overflow-hidden rounded-[10px] border border-(--color-border-default) bg-(--color-bg-surface) shadow-lg">
                    <div className="px-4 py-2.5">
                      <span className="text-[13px] font-semibold text-(--color-text-primary)">{t('dashboard.grid.title')}</span>
                    </div>
                    <div className="h-px bg-(--color-border-default)" />
                    <div className="flex flex-col gap-3 p-3">
                      {/* 칼럼 수 입력 */}
                      <div className="flex flex-col gap-1.5">
                        <span className="text-[11px] font-medium text-(--color-text-muted)">{t('dashboard.grid.colsLabel')}</span>
                        <div className="flex">
                          <div className="flex flex-1 items-center justify-center rounded-l-lg border border-(--color-border-default) bg-(--color-bg-surface)">
                            <input
                              type="number"
                              min={4}
                              max={100}
                              value={gridCols}
                              onChange={(e) => setGridCols(Number(e.target.value))}
                              className="w-full bg-transparent py-1.5 text-center text-sm font-semibold text-(--color-text-primary) outline-none"
                            />
                          </div>
                          <div className="flex flex-col">
                            <button
                              type="button"
                              onClick={() => setGridCols(gridCols + 1)}
                              disabled={gridCols >= 100}
                              className="flex h-[18px] w-7 items-center justify-center rounded-tr-md border border-l-0 border-(--color-border-default) bg-(--color-bg-elevated) text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-40"
                            >
                              <ChevronUp className="h-3 w-3" />
                            </button>
                            <button
                              type="button"
                              onClick={() => setGridCols(gridCols - 1)}
                              disabled={gridCols <= 4}
                              className="flex h-[18px] w-7 items-center justify-center rounded-br-md border border-l-0 border-t-0 border-(--color-border-default) bg-(--color-bg-elevated) text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-40"
                            >
                              <ChevronDown className="h-3 w-3" />
                            </button>
                          </div>
                        </div>
                      </div>

                      {/* 그리드 라인 표시 토글 */}
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-1.5">
                          <Grid3X3 className="h-3.5 w-3.5 text-(--color-text-muted)" />
                          <span className="text-xs font-medium text-(--color-text-secondary)">{t('dashboard.grid.showGridLines')}</span>
                        </div>
                        <button
                          type="button"
                          onClick={() => setShowGridLines(!showGridLines)}
                          className={`relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors ${showGridLines ? 'bg-blue-500' : 'bg-(--color-border-default)'}`}
                          aria-label={t('dashboard.grid.showGridLinesAria')}
                        >
                          <span className={`inline-block h-4 w-4 rounded-full bg-white shadow-sm transition-transform ${showGridLines ? 'translate-x-[18px]' : 'translate-x-0.5'}`} />
                        </button>
                      </div>
                    </div>
                  </div>
                )}
              </div>

              {/* 패널 추가 (Pencil: aSF19) */}
              <button
                type="button"
                onClick={() => navigate('/panels/new')}
                className="inline-flex items-center gap-1.5 rounded-lg border border-(--color-border-default) bg-(--color-bg-elevated) px-3.5 py-2 text-[13px] font-medium text-(--color-text-muted) transition-colors hover:bg-(--color-border-default)"
              >
                <Plus className="h-3.5 w-3.5" />
                {t('dashboard.addPanel')}
              </button>

              {/* 취소 (Pencil: m5m4p) */}
              <button
                type="button"
                onClick={() => setEditMode(false)}
                className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-default) px-4 py-2 text-[13px] font-medium text-(--color-text-primary) transition-colors hover:bg-(--color-bg-elevated)"
              >
                <X className="h-3.5 w-3.5 text-(--color-text-muted)" />
                {t('common.cancel')}
              </button>

              {/* 저장 (Pencil: NjZlk) */}
              <button
                type="button"
                onClick={() => setEditMode(false)}
                className="inline-flex items-center gap-1.5 rounded-md bg-blue-500 px-4 py-2 text-[13px] font-medium text-white transition-colors hover:bg-blue-600"
              >
                <Save className="h-3.5 w-3.5" />
                {t('common.save')}
              </button>
            </>
          ) : (
            /* ── 일반 모드 우측: WS 상태 + 새로고침 + 편집 + 구분선 + 테마 + 사용자 + 로그아웃 ── */
            <>
              <div className="flex items-center gap-4">
                {/* WebSocket 연결 상태 */}
                <span className="inline-flex items-center gap-1.5">
                  <span className={`h-2 w-2 rounded-full ${wsState === 'connected' ? 'bg-green-500' : 'bg-gray-400'}`} />
                  <span className={`text-xs ${wsState === 'connected' ? 'text-green-500' : 'text-(--color-text-muted)'}`}>
                    {wsState === 'connected' ? t('dashboard.header.connected') : t('dashboard.offline')}
                  </span>
                </span>

                {/* 갱신 주기 셀렉터 — 개인/뷰 설정이므로 편집 권한과 무관하게 항상 사용 가능 */}
                <div className="relative" ref={refreshDropdownRef}>
                  <button
                    type="button"
                    onClick={() => setRefreshDropdownOpen((prev) => !prev)}
                    className="inline-flex items-center gap-1.5 rounded-lg border border-(--color-border-default) bg-(--color-bg-elevated) px-2.5 py-1.5 text-[13px] font-medium text-(--color-text-muted) transition-colors hover:bg-(--color-border-default)"
                    aria-label={t('dashboard.header.refreshAria')}
                    aria-haspopup="listbox"
                    aria-expanded={refreshDropdownOpen}
                  >
                    <Clock className="h-3.5 w-3.5" />
                    {refreshInterval}{t('dashboard.header.refreshSuffix')}
                    <ChevronDown className="h-3 w-3 text-(--color-text-muted)" />
                  </button>

                  {refreshDropdownOpen && (
                    <div
                      role="listbox"
                      aria-label={t('dashboard.header.refreshSelectAria')}
                      className="absolute right-0 top-full z-50 mt-1 w-[160px] overflow-hidden rounded-[10px] border border-(--color-border-default) bg-(--color-bg-surface) shadow-lg"
                    >
                      <div className="px-4 py-2.5">
                        <span className="text-[13px] font-semibold text-(--color-text-primary)">{t('dashboard.header.refreshTitle')}</span>
                      </div>
                      <div className="h-px bg-(--color-border-default)" />
                      <div className="flex flex-col gap-0.5 p-1.5">
                        {REFRESH_INTERVALS.map((opt) => {
                          const isSelected = refreshInterval === opt.value;
                          return (
                            <button
                              key={opt.value}
                              type="button"
                              role="option"
                              aria-selected={isSelected}
                              onClick={() => { setRefreshInterval(opt.value); setRefreshDropdownOpen(false); }}
                              className={`flex items-center justify-between rounded-md px-3 py-2 transition-colors ${
                                isSelected
                                  ? 'bg-blue-50 dark:bg-blue-900/20'
                                  : 'hover:bg-(--color-bg-elevated)'
                              }`}
                            >
                              <span className={`text-[13px] font-medium ${isSelected ? 'text-blue-600 dark:text-blue-400' : 'text-(--color-text-primary)'}`}>
                                {t('dashboard.refreshOption').replace('{value}', String(opt.value))}
                              </span>
                              {isSelected && <Check className="h-3.5 w-3.5 text-blue-500" />}
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  )}
                </div>

                {/* 새로고침 */}
                <button
                  type="button"
                  onClick={handleRefresh}
                  disabled={isLoading}
                  className="text-(--color-text-muted) transition-colors hover:text-(--color-text-primary) disabled:opacity-50"
                  aria-label={t('dashboard.header.refresh')}
                >
                  <RefreshCw className={`h-4 w-4 ${isLoading ? 'animate-spin' : ''}`} />
                </button>

                {/* 편집 모드 진입 — sharedReadOnly 면 비활성화 (AC-4) */}
                <button
                  type="button"
                  onClick={() => setEditMode(true)}
                  disabled={sharedReadOnly}
                  aria-disabled={sharedReadOnly}
                  title={sharedReadOnly ? t('dashboard.scope.adminOnly') : t('dashboard.editLayout')}
                  className={`transition-colors ${
                    sharedReadOnly
                      ? 'cursor-not-allowed text-(--color-text-muted) opacity-40'
                      : 'text-(--color-text-muted) hover:text-(--color-text-primary)'
                  }`}
                  aria-label={t('dashboard.editLayout')}
                >
                  <Pencil className="h-4 w-4" />
                </button>
              </div>
            </>
          )}
        </div>
      </header>

      {/* ── 콘텐츠 영역 ── */}
      <div className={`flex-1 overflow-auto ${editMode ? 'bg-(--color-bg-elevated)' : ''}`}>
        {/* 에러 배너 */}
        {hasError && (
          <div className="mx-6 mt-4 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400">
            {t('dashboard.loadError')}
          </div>
        )}

        {/* 로딩 스켈레톤 */}
        {isLoading && !flowsData && !metrics ? (
          <div className="space-y-4 p-6">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div className="h-96 animate-pulse rounded-lg bg-(--color-bg-elevated)" />
              <div className="h-96 animate-pulse rounded-lg bg-(--color-bg-elevated)" />
            </div>
            <div className="h-48 animate-pulse rounded-lg bg-(--color-bg-elevated)" />
          </div>
        ) : (
          <div className="relative p-6" ref={gridCallbackRef}>
            {/* 편집 모드 그리드 가이드 라인 (세로 + 가로 통합) */}
            {editMode && showGridLines && (
              <div
                className="pointer-events-none absolute inset-6 z-0"
                style={{
                  display: 'grid',
                  gridTemplateColumns: `repeat(${gridCols}, 1fr)`,
                  gridAutoRows: `${gridRowHeight}px`,
                  gap: `${GRID_MARGIN[1]}px ${GRID_MARGIN[0]}px`,
                }}
              >
                {Array.from({ length: gridCols * Math.ceil(((gridRef.current?.clientHeight ?? 800) - 48) / (gridRowHeight + GRID_MARGIN[1])) }).map((_, i) => (
                  <div
                    key={i}
                    className="border border-dashed border-slate-200 dark:border-slate-700"
                  />
                ))}
              </div>
            )}

            {containerWidth <= 0 ? null : <GridLayout
              layout={layout}
              width={containerWidth}
              gridConfig={{
                cols: gridCols,
                rowHeight: gridRowHeight,
                margin: GRID_MARGIN,
                containerPadding: [0, 0],
              }}
              dragConfig={{
                enabled: editMode,
                handle: '.dashboard-drag-handle',
              }}
              resizeConfig={{
                enabled: editMode,
                handles: ['se'],
              }}
              onLayoutChange={(newLayout) => handleLayoutChange(newLayout as DashboardLayoutItem[])}
            >
              {panels.map((panel) => {
                const panelColor = panel.config?.panelColor as string | undefined;
                return (
                  <div
                    key={panel.id}
                    className={`relative flex flex-col overflow-hidden ${editMode ? 'rounded-lg ring-2 ring-blue-300 dark:ring-blue-600' : ''}`}
                    style={panelColor ? { '--panel-accent': panelColor, borderLeft: `4px solid ${panelColor}`, borderTop: `2px solid ${panelColor}` } as React.CSSProperties : undefined}
                  >
                    {editMode && <DragHandle />}
                    {editMode && (
                      <div className="absolute right-1 top-1 z-10 flex gap-1">
                        <button
                          type="button"
                          onClick={() => navigate(`/panels/${panel.id}/settings`)}
                          className="rounded-full bg-(--color-bg-elevated) p-0.5 text-(--color-text-muted) shadow transition-colors hover:bg-(--color-bg-surface) hover:text-(--color-text-primary)"
                          aria-label={t('dashboard.settings.title')}
                          title={t('dashboard.settings.title')}
                        >
                          <Settings className="h-3.5 w-3.5" />
                        </button>
                        <button
                          type="button"
                          onClick={() => removePanel(panel.id)}
                          className="rounded-full bg-red-500 p-0.5 text-white shadow transition-colors hover:bg-red-600"
                          aria-label={t('dashboard.settings.deletePanelAria')}
                          title={t('dashboard.settings.deletePanelAria')}
                        >
                          <X className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    )}
                    {renderPanel(panel, flows, metrics)}
                  </div>
                );
              })}
            </GridLayout>}
          </div>
        )}
      </div>

    </div>
  );
}

/** 편집 모드 드래그 핸들 */
function DragHandle() {
  return (
    <div className="dashboard-drag-handle flex h-6 cursor-grab items-center justify-center rounded-t-lg bg-(--color-bg-elevated)/80 active:cursor-grabbing">
      <div className="flex gap-1">
        <span className="h-1 w-1 rounded-full bg-(--color-text-muted)" />
        <span className="h-1 w-1 rounded-full bg-(--color-text-muted)" />
        <span className="h-1 w-1 rounded-full bg-(--color-text-muted)" />
      </div>
    </div>
  );
}
