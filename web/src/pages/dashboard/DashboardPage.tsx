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
  Trash2,
} from 'lucide-react';

import 'react-grid-layout/css/styles.css';
import 'react-resizable/css/styles.css';

import { useFlows, useWebSocket } from '@/hooks';
import { usePermission } from '@/hooks/usePermission';
import { useTranslation } from '@/lib/i18n';
import { useDashboardSyncStatus } from '@/contexts/DashboardSyncContext';
import { isRemoteTarget, LOCAL_TARGET, type ResourceTarget } from '@/lib/remote/target';
import { getMetrics } from '@/services/api/monitorService';
import {
  useUIStore,
  type DashboardLayoutItem,
  type PanelConfig,
  type ThemeMode,
} from '@/stores/uiStore';
import { WS_MESSAGE_TYPES } from '@/services/ws/wsHandlers';
import type { FlowInfo } from '@/types/flow';

import { dashboardAccessByUid } from './dashboardAccess';
import DashboardSettingsSelector from './DashboardSettingsSelector';
import DragHandle from './DragHandle';
import { GRID_MARGIN_PX } from './gridGeometry';
import { useCreateDashboard } from './useCreateDashboard';
import { useDashboardMutations } from './useDashboardMutations';
import { renderDashboardPanel } from './renderDashboardPanel';
import RemoteDashboardView from './RemoteDashboardView';

/** 그리드 설정. 마진은 gridGeometry 와 공유한다 — 패널 설정 화면이 같은 식으로 종횡비를 역산한다. */
const GRID_MARGIN: [number, number] = [GRID_MARGIN_PX, GRID_MARGIN_PX];

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

  // SPEC-DASHBOARD-004 M6: 대시보드 단위 인가 판정 (spec.md §2.11 S2).
  //
  // 목록 응답이 항목마다 실어 보내는 can_edit / can_grant / can_delete 를 그대로
  // 읽는다 — 판정 규칙(§2.2)은 서버가 소유하고 프론트는 재구현하지 않는다.
  // 컨트롤은 **숨기지 않고 비활성 + 사유 툴팁**으로 둔다(SPEC-AUTH-006 §4.2):
  // 사라진 버튼은 "기능이 없다"로 읽히지만, 비활성 버튼은 관리자에게 권한을
  // 요청할 여지를 남긴다.
  const dashboards = useUIStore((s) => s.dashboards);
  const accessOf = useCallback(
    (uid: string) => dashboardAccessByUid(dashboards, uid),
    [dashboards],
  );

  // 생성은 대시보드 단위가 아니라 **전역 권한** 판정이다(spec.md §2.7 E1).
  // 아직 존재하지 않는 대시보드에는 can_* 를 붙일 대상이 없기 때문이다.
  // 역할 이름 비교(isAdmin)는 쓰지 않는다(spec.md §2.14 #2).
  const { hasPermission } = usePermission();
  const canCreate = hasPermission('dashboard.create');
  const { create: createDashboardOnServer, isCreating } = useCreateDashboard();

  // 이름 변경·기본 지정·삭제는 서버에 반영해야 영속된다 — `name` 과 `is_default`
  // 는 컬럼이 되었고 본문 PUT 은 `{ payload }` 만 보내므로, 로컬 스토어만 바꾸면
  // 사용자가 새로고침에서 변경을 잃는다.
  const { rename, setDefault, remove: removeDashboard } = useDashboardMutations();

  // UI store
  const refreshInterval = useUIStore((s) => s.dashboardRefreshInterval);
  const setRefreshInterval = useUIStore((s) => s.setDashboardRefreshInterval);

  const dashboardPages = useUIStore((s) => s.dashboardPages);
  const activeDashboardId = useUIStore((s) => s.activeDashboardId);
  const setActiveDashboard = useUIStore((s) => s.setActiveDashboard);
  const activePage = useUIStore((s) =>
    s.dashboardPages.find((p) => p.id === s.activeDashboardId),
  );
  // 활성 대시보드의 판정 — 레이아웃 편집·패널 추가/삭제의 게이트다.
  const activeAccess = accessOf(activeDashboardId);
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
  const setGridWidth = useUIStore((s) => s.setDashboardGridWidth);
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

  // 실측 폭을 store 에 게시한다 — 패널 설정 화면(다른 라우트)은 그리드를 렌더하지 않으므로
  // 스스로 잴 수 없고, 이 값 없이는 "이 패널이 실제로 몇 대 몇인지"를 알 수 없다.
  useEffect(() => {
    setGridWidth(containerWidth);
  }, [containerWidth, setGridWidth]);

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

  /** 인라인 이름 편집 완료 — 서버 PATCH{name} 으로 영속한다.
   *
   * 편집 시작 후 목록 재조회로 권한을 잃는 경우가 있으므로 확정 시점에 다시
   * 판정한다. 서버가 거부하면 훅이 낙관적 반영을 되돌리고 사유를 알린다. */
  const finishRename = () => {
    if (editingPageId && editingName.trim() && accessOf(editingPageId).canEdit) {
      void rename(editingPageId, editingName.trim());
    }
    setEditingPageId(null);
    setEditingName('');
  };

  /** 대시보드 삭제 — 되돌릴 수 없으므로 확인을 받고 서버에 요청한다. */
  const handleDeleteDashboard = (uid: string, name: string) => {
    if (!accessOf(uid).canDelete) return;
    const confirmed = window.confirm(
      t('header.dashboard.deleteConfirm').replace('{name}', name),
    );
    if (!confirmed) return;
    void removeDashboard(uid);
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
    <div className="-m-6 flex flex-1 flex-col" ref={containerRef} data-testid="dashboard-root">
      {/* 공유/내 대시보드 스코프 바는 제거했다(SPEC-AUTH-006 후속) —
          대시보드 관리는 별도 메뉴로 분리 예정이다. 같은 행에 있던 저장 상태
          표시는 아래 헤더 바 우측으로 옮겨 정보가 사라지지 않게 했다. */}
      {/* ── 헤더 바 (Pencil: 56px, 흰색, border-bottom) ── */}
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-(--color-border-default) bg-(--color-bg-surface) px-6">
        {editMode ? (
          /* ── 편집 모드 헤더 좌측: 대시보드 셀렉터 + 관리 컨트롤 + 앰버 뱃지 ──
             별도 '대시보드 관리' 화면을 없애고 관리 기능을 이 안으로 옮겼다.
             목록은 볼 수 있는 대시보드 **전부**(비활성 포함)를 싣는다. */
          <DashboardSettingsSelector activeName={activePageName} />
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
                  {dashboardPages.map((page) => {
                    // 행마다 그 대시보드의 판정을 쓴다 — 활성 대시보드 하나로
                    // 목록 전체를 잠그면 편집 가능한 대시보드까지 함께 잠긴다.
                    const rowAccess = accessOf(page.id);
                    return (
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
                      {/* 기본 지정은 서버가 PATCH{is_default} 를 grant 인가로
                          검사한다(spec.md §2.3) — can_grant 로 게이팅한다. */}
                      <button
                        type="button"
                        onClick={() => void setDefault(page.id)}
                        disabled={!rowAccess.canGrant}
                        aria-disabled={!rowAccess.canGrant}
                        data-testid={`dashboard-set-default-${page.id}`}
                        className={`shrink-0 p-0.5 transition-colors ${
                          !rowAccess.canGrant
                            ? 'cursor-not-allowed opacity-40'
                            : page.isDefault
                            ? 'text-yellow-500'
                            : 'text-(--color-text-muted) hover:text-yellow-400'
                        }`}
                        title={!rowAccess.canGrant ? t('dashboard.gate.grantDenied') : page.isDefault ? t('dashboard.header.defaultTitle') : t('dashboard.header.setDefault')}
                      >
                        <Star className={`h-3.5 w-3.5 ${page.isDefault ? 'fill-current' : ''}`} />
                      </button>
                      {/* 이름 변경은 서버가 PATCH{name} 를 edit 인가로
                          검사한다(spec.md §2.3) — can_edit 으로 게이팅한다. */}
                      <button
                        type="button"
                        onClick={() => startRename(page.id, page.name)}
                        disabled={!rowAccess.canEdit}
                        aria-disabled={!rowAccess.canEdit}
                        data-testid={`dashboard-rename-${page.id}`}
                        className={`shrink-0 p-0.5 transition-colors ${
                          !rowAccess.canEdit
                            ? 'cursor-not-allowed text-(--color-text-muted) opacity-40'
                            : 'text-(--color-text-muted) hover:text-(--color-text-primary)'
                        }`}
                        title={!rowAccess.canEdit ? t('dashboard.gate.editDenied') : t('dashboard.header.rename')}
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </button>
                      {/* 삭제는 서버가 DELETE 를 delete 인가로 검사한다
                          (spec.md §2.3) — can_delete 로 게이팅한다. 활성
                          대시보드를 지우면 M5 의 폴백 사슬이 착지점을 정한다. */}
                      <button
                        type="button"
                        onClick={() => handleDeleteDashboard(page.id, page.name)}
                        disabled={!rowAccess.canDelete}
                        aria-disabled={!rowAccess.canDelete}
                        data-testid={`dashboard-delete-${page.id}`}
                        className={`shrink-0 p-0.5 transition-colors ${
                          !rowAccess.canDelete
                            ? 'cursor-not-allowed text-(--color-text-muted) opacity-40'
                            : 'text-(--color-text-muted) hover:text-red-500'
                        }`}
                        title={!rowAccess.canDelete ? t('dashboard.gate.deleteDenied') : t('header.dashboard.delete')}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                    );
                  })}
                </div>
                <div className="border-t border-(--color-border-default)">
                  {/* 생성은 전역 dashboard.create 판정이다(spec.md §2.7 E1).
                      로컬 스토어가 아니라 서버 POST 로 만들고 서버가 발급한
                      uid 를 그대로 쓴다 — uid 를 지어내면 서버 행과 어긋난다. */}
                  <button
                    type="button"
                    onClick={() => {
                      void createDashboardOnServer(t('dashboard.header.newDashboardName'));
                      setDashboardDropdownOpen(false);
                    }}
                    disabled={!canCreate || isCreating}
                    aria-disabled={!canCreate || isCreating}
                    data-testid="dashboard-add"
                    title={canCreate ? undefined : t('dashboard.gate.createDenied')}
                    className={`flex w-full items-center justify-center gap-2 px-4 py-2.5 text-sm transition-colors ${
                      !canCreate || isCreating
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
          {/* 저장 진행 표시 — 스코프 바가 사라지면서 이리로 옮겼다. 레이아웃
              변경이 저장 중인지 사용자가 알 수 있어야 한다. */}
          {pendingSync && (
            <span
              className="text-[11px] text-(--color-text-muted)"
              aria-live="polite"
              data-testid="dashboard-syncing"
            >
              {t('dashboard.scope.syncing')}
            </span>
          )}
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

              {/* 패널 추가 (Pencil: aSF19) — 편집 모드 진입 후 권한을 잃는
                  경우(목록 재조회로 can_edit 이 false 로 바뀜)가 있으므로
                  여기서도 판정한다(spec.md §2.11 은 패널 추가를 명시한다). */}
              <button
                type="button"
                onClick={() => navigate('/panels/new')}
                disabled={!activeAccess.canEdit}
                aria-disabled={!activeAccess.canEdit}
                data-testid="dashboard-add-panel"
                title={activeAccess.canEdit ? undefined : t('dashboard.gate.editDenied')}
                className="inline-flex items-center gap-1.5 rounded-lg border border-(--color-border-default) bg-(--color-bg-elevated) px-3.5 py-2 text-[13px] font-medium text-(--color-text-muted) transition-colors hover:bg-(--color-border-default) disabled:cursor-not-allowed disabled:opacity-40"
              >
                <Plus className="h-3.5 w-3.5" />
                {/* dashboard.addPanel 은 하위 키를 가진 네임스페이스 객체이므로
                    그대로 넘기면 번역이 잡히지 않고 키 문자열이 그대로 노출된다. */}
                {t('dashboard.addPanel.title')}
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

                {/* 레이아웃 편집 진입 — 활성 대시보드의 can_edit 게이트
                    (spec.md §2.11 S2). 숨기지 않고 비활성 + 사유 툴팁이다. */}
                <button
                  type="button"
                  onClick={() => setEditMode(true)}
                  disabled={!activeAccess.canEdit}
                  aria-disabled={!activeAccess.canEdit}
                  data-testid="dashboard-edit-mode"
                  title={activeAccess.canEdit ? t('dashboard.editLayout') : t('dashboard.gate.editDenied')}
                  className={`transition-colors ${
                    !activeAccess.canEdit
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

        {/* 편집 불가 상시 안내 배너는 두지 않는다(사용자 요청).
            사유는 비활성 컨트롤의 툴팁(dashboard.gate.*Denied)이 계속 전달한다. */}

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
                // 레이아웃 편집도 can_edit 게이트를 탄다(spec.md §2.11) —
                // 편집 중 권한을 잃으면 드래그가 즉시 멈춰야 한다.
                enabled: editMode && activeAccess.canEdit,
                handle: '.dashboard-drag-handle',
              }}
              resizeConfig={{
                enabled: editMode && activeAccess.canEdit,
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
                      <div className="absolute right-1 top-1 z-20 flex gap-1">
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
                          disabled={!activeAccess.canEdit}
                          aria-disabled={!activeAccess.canEdit}
                          className="rounded-full bg-red-500 p-0.5 text-white shadow transition-colors hover:bg-red-600 disabled:cursor-not-allowed disabled:opacity-40"
                          aria-label={t('dashboard.settings.deletePanelAria')}
                          title={activeAccess.canEdit ? t('dashboard.settings.deletePanelAria') : t('dashboard.gate.editDenied')}
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

