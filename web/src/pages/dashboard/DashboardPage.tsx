// 대시보드 메인 페이지.
// 플로우 현황, 시스템 메트릭, 에이전트 상태를 위젯 형태로 표시하고
// WebSocket을 통해 실시간 업데이트를 수신한다.
// react-grid-layout으로 패널 드래그/리사이즈를 지원한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import GridLayout from 'react-grid-layout';
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
  Info,
  Check,
  Grid3X3,
  ChevronUp,
  BarChart3,
  PieChart,
  LineChart,
  Gauge,
  Type,
  Table,
  Gamepad2,
  Hash,
} from 'lucide-react';

import 'react-grid-layout/css/styles.css';
import 'react-resizable/css/styles.css';

import { useFlows, useWebSocket } from '@/hooks';
import { getMetrics } from '@/services/api/monitorService';
import {
  useUIStore,
  type DashboardLayoutItem,
  type PanelConfig,
  type PanelType,
  type ThemeMode,
} from '@/stores/uiStore';
import { WS_MESSAGE_TYPES } from '@/services/ws/wsHandlers';
import type { FlowInfo } from '@/types/flow';

import AgentPanel from './panels/AgentPanel';
import DevicePanel from './panels/DevicePanel';
import FlowPanel from './panels/FlowPanel';
import LogPanel from './panels/LogPanel';
import SingleDevicePanel from './panels/SingleDevicePanel';
import AcControlPanel from './panels/AcControlPanel';
import GaugePanel from './panels/GaugePanel';
import PropertiesGridPanel from './panels/PropertiesGridPanel';
import HvacControlPanel from './panels/HvacControlPanel';
import OutdoorControlPanel from './panels/OutdoorControlPanel';
import StatPanel from './panels/charts/StatPanel';
import LineChartPanel from './panels/charts/LineChartPanel';
import BarChartPanel from './panels/charts/BarChartPanel';
import PieChartPanel from './panels/charts/PieChartPanel';
import TablePanel from './panels/charts/TablePanel';
import ResourceWidget from './widgets/ResourceWidget';
import AddPanelDialog from './AddPanelDialog';
import PanelSettingsDialog from './PanelSettingsDialog';

/** 그리드 설정 */
const GRID_MARGIN: [number, number] = [16, 16];

/** 패널 타입별 아이콘 매핑 */
const PANEL_TYPE_ICONS: Partial<Record<PanelType, React.ReactNode>> = {
  stat: <Hash className="h-6 w-6 text-(--color-text-muted)" />,
  gauge: <Gauge className="h-6 w-6 text-(--color-text-muted)" />,
  'line-chart': <LineChart className="h-6 w-6 text-(--color-text-muted)" />,
  'bar-chart': <BarChart3 className="h-6 w-6 text-(--color-text-muted)" />,
  'pie-chart': <PieChart className="h-6 w-6 text-(--color-text-muted)" />,
  text: <Type className="h-6 w-6 text-(--color-text-muted)" />,
  table: <Table className="h-6 w-6 text-(--color-text-muted)" />,
  'custom-control': <Gamepad2 className="h-6 w-6 text-(--color-text-muted)" />,
};

/** 테마 모드 라벨 (Pencil 디자인 매칭) */
const THEME_OPTIONS: { value: ThemeMode; label: string; desc: string; icon: React.ReactNode; iconColor: string }[] = [
  { value: 'system', label: '시스템', desc: 'OS 설정에 따라 자동 전환', icon: <Monitor className="h-4 w-4" />, iconColor: 'text-blue-500' },
  { value: 'day', label: '데이', desc: '밝은 배경, 어두운 텍스트', icon: <Sun className="h-4 w-4" />, iconColor: 'text-amber-500' },
  { value: 'night', label: '나이트', desc: '어두운 배경, 밝은 텍스트', icon: <Moon className="h-4 w-4" />, iconColor: 'text-indigo-500' },
  { value: 'custom', label: '커스텀', desc: '사용자 정의 색상 테마', icon: <Paintbrush className="h-4 w-4" />, iconColor: 'text-violet-500' },
];

/** 대시보드 페이지 컴포넌트 */
export default function DashboardPage() {
  const queryClient = useQueryClient();

  // UI store
  const refreshInterval = useUIStore((s) => s.dashboardRefreshInterval);

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
  // 패널 추가 다이얼로그
  const [addPanelOpen, setAddPanelOpen] = useState(false);
  // 패널 설정 다이얼로그 (열린 패널 ID)
  const [settingsPanelId, setSettingsPanelId] = useState<string | null>(null);

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
    if (!dashboardDropdownOpen && !themeDropdownOpen && !gridDropdownOpen) return;
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
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [dashboardDropdownOpen, themeDropdownOpen, gridDropdownOpen]);

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

  /** 패널 타입에 따라 적절한 위젯 컴포넌트를 렌더링 */
  const renderPanel = (panel: PanelConfig, flowsList: FlowInfo[], metricsData: typeof metrics) => {
    const onCfg = configChangeFor(panel.id);
    const onTitle = titleChangeFor(panel.id);

    switch (panel.type) {
      case 'flows':
        return <FlowPanel flows={flowsList} panelConfig={panel} />;
      case 'agents':
        return <AgentPanel panelConfig={panel} />;
      case 'resource':
        return <ResourceWidget metrics={metricsData} panelConfig={panel} />;
      case 'devices':
        return (
          <DevicePanel
            panelId={panel.id}
            title={panel.title}
            config={panel.config}
            refreshMs={refreshMs}
            onConfigChange={onCfg}
            onTitleChange={onTitle}
          />
        );
      case 'device':
        return (
          <SingleDevicePanel
            panelId={panel.id}
            title={panel.title}
            config={panel.config}
            onConfigChange={onCfg}
            onTitleChange={onTitle}
          />
        );
      case 'logs':
        return (
          <LogPanel
            panelId={panel.id}
            title={panel.title}
            config={panel.config}
            onConfigChange={onCfg}
            onTitleChange={onTitle}
          />
        );
      case 'ac-control':
        return (
          <AcControlPanel
            panelId={panel.id}
            title={panel.title}
            config={panel.config}
            onConfigChange={onCfg}
            onTitleChange={onTitle}
          />
        );
      case 'hvac-control':
        return (
          <HvacControlPanel
            panelId={panel.id}
            title={panel.title}
            config={panel.config}
            onConfigChange={onCfg}
            onTitleChange={onTitle}
          />
        );
      case 'outdoor-control':
        return (
          <OutdoorControlPanel
            panelId={panel.id}
            title={panel.title}
            config={panel.config}
            onConfigChange={onCfg}
            onTitleChange={onTitle}
          />
        );
      case 'gauge':
        return (
          <GaugePanel
            panelId={panel.id}
            title={panel.title}
            config={panel.config}
            onConfigChange={onCfg}
            onTitleChange={onTitle}
          />
        );
      case 'properties-grid':
        return (
          <PropertiesGridPanel
            panelId={panel.id}
            title={panel.title}
            config={panel.config}
            onConfigChange={onCfg}
            onTitleChange={onTitle}
          />
        );
      // SPEC-CHART-001 M4: 5종 차트 패널
      case 'stat':
        return <StatPanel panelId={panel.id} config={panel.config} />;
      case 'line-chart':
        return <LineChartPanel panelId={panel.id} config={panel.config} />;
      case 'bar-chart':
        return <BarChartPanel panelId={panel.id} config={panel.config} />;
      case 'pie-chart':
        return <PieChartPanel panelId={panel.id} config={panel.config} />;
      case 'table':
        return <TablePanel panelId={panel.id} config={panel.config} />;
      // 잔여 플레이스홀더 패널 타입들 (text, custom-control)
      case 'text':
      case 'custom-control':
        return (
          <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-2 rounded-lg bg-(--color-bg-surface) p-6 shadow">
            {PANEL_TYPE_ICONS[panel.type] ?? null}
            <span className="text-sm font-medium text-(--color-text-primary)">{panel.title}</span>
            <span className="text-xs text-(--color-text-muted)">{panel.type}</span>
          </div>
        );
      default:
        return (
          <div className="flex min-h-0 flex-1 items-center justify-center rounded-lg bg-(--color-bg-surface) p-6 shadow">
            <span className="text-sm text-(--color-text-muted)">{panel.title}</span>
          </div>
        );
    }
  };

  const hasError = flowsError;

  // 현재 활성 대시보드 이름
  const activePageName = activePage?.name ?? '대시보드';

  return (
    <div className="-m-6 flex flex-1 flex-col" ref={containerRef}>
      {/* ── 헤더 바 (Pencil: 56px, 흰색, border-bottom) ── */}
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-(--color-border-default) bg-(--color-bg-surface) px-6">
        {editMode ? (
          /* ── 편집 모드 헤더 좌측: 타이틀 + 앰버 뱃지 ── */
          <div className="flex items-center gap-3">
            <span className="text-base font-semibold text-(--color-text-primary)">
              {activePageName}
            </span>
            <span className="inline-flex items-center rounded-md bg-amber-50 px-2.5 py-1 text-[11px] font-semibold text-amber-500 dark:bg-amber-900/30 dark:text-amber-400">
              편집 모드
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
                        className={`shrink-0 p-0.5 transition-colors ${page.isDefault ? 'text-yellow-500' : 'text-(--color-text-muted) hover:text-yellow-400'}`}
                        title={page.isDefault ? '기본 대시보드' : '기본 대시보드로 설정'}
                      >
                        <Star className={`h-3.5 w-3.5 ${page.isDefault ? 'fill-current' : ''}`} />
                      </button>
                      <button
                        type="button"
                        onClick={() => startRename(page.id, page.name)}
                        className="shrink-0 p-0.5 text-(--color-text-muted) transition-colors hover:text-(--color-text-primary)"
                        title="이름 변경"
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  ))}
                </div>
                <div className="border-t border-(--color-border-default)">
                  <button
                    type="button"
                    onClick={() => { addDashboardPage('새 대시보드'); setDashboardDropdownOpen(false); }}
                    className="flex w-full items-center justify-center gap-2 px-4 py-2.5 text-sm text-blue-600 transition-colors hover:bg-(--color-bg-elevated) dark:text-blue-400"
                  >
                    <Plus className="h-3.5 w-3.5" />
                    새 대시보드 추가
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
                  테마
                  <ChevronDown className="h-3 w-3 text-(--color-text-muted)" />
                </button>

                {/* 테마 드롭다운 (Pencil: vs5tn) */}
                {themeDropdownOpen && (
                  <div className="absolute right-0 top-full z-50 mt-1 w-[280px] overflow-hidden rounded-[10px] border border-(--color-border-default) bg-(--color-bg-surface) shadow-lg">
                    <div className="px-4 py-2.5">
                      <span className="text-[13px] font-semibold text-(--color-text-primary)">대시보드 테마</span>
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
                                {opt.label}
                              </span>
                              <span className="text-[11px] text-(--color-text-muted)">{opt.desc}</span>
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
                  그리드
                  <ChevronDown className="h-3 w-3 text-(--color-text-muted)" />
                </button>

                {gridDropdownOpen && (
                  <div className="absolute right-0 top-full z-50 mt-1 w-[200px] overflow-hidden rounded-[10px] border border-(--color-border-default) bg-(--color-bg-surface) shadow-lg">
                    <div className="px-4 py-2.5">
                      <span className="text-[13px] font-semibold text-(--color-text-primary)">그리드 설정</span>
                    </div>
                    <div className="h-px bg-(--color-border-default)" />
                    <div className="flex flex-col gap-3 p-3">
                      {/* 칼럼 수 입력 */}
                      <div className="flex flex-col gap-1.5">
                        <span className="text-[11px] font-medium text-(--color-text-muted)">칼럼 수 (4 - 100)</span>
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
                          <span className="text-xs font-medium text-(--color-text-secondary)">그리드 라인 표시</span>
                        </div>
                        <button
                          type="button"
                          onClick={() => setShowGridLines(!showGridLines)}
                          className={`relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors ${showGridLines ? 'bg-blue-500' : 'bg-(--color-border-default)'}`}
                          aria-label="그리드 라인 표시 토글"
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
                onClick={() => setAddPanelOpen(true)}
                className="inline-flex items-center gap-1.5 rounded-lg border border-(--color-border-default) bg-(--color-bg-elevated) px-3.5 py-2 text-[13px] font-medium text-(--color-text-muted) transition-colors hover:bg-(--color-border-default)"
              >
                <Plus className="h-3.5 w-3.5" />
                패널 추가
              </button>

              {/* 취소 (Pencil: m5m4p) */}
              <button
                type="button"
                onClick={() => setEditMode(false)}
                className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-default) px-4 py-2 text-[13px] font-medium text-(--color-text-primary) transition-colors hover:bg-(--color-bg-elevated)"
              >
                <X className="h-3.5 w-3.5 text-(--color-text-muted)" />
                취소
              </button>

              {/* 저장 (Pencil: NjZlk) */}
              <button
                type="button"
                onClick={() => setEditMode(false)}
                className="inline-flex items-center gap-1.5 rounded-md bg-blue-500 px-4 py-2 text-[13px] font-medium text-white transition-colors hover:bg-blue-600"
              >
                <Save className="h-3.5 w-3.5" />
                저장
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
                    {wsState === 'connected' ? '연결됨' : '오프라인'}
                  </span>
                </span>

                {/* 새로고침 */}
                <button
                  type="button"
                  onClick={handleRefresh}
                  disabled={isLoading}
                  className="text-(--color-text-muted) transition-colors hover:text-(--color-text-primary) disabled:opacity-50"
                  aria-label="새로고침"
                >
                  <RefreshCw className={`h-4 w-4 ${isLoading ? 'animate-spin' : ''}`} />
                </button>

                {/* 편집 모드 진입 */}
                <button
                  type="button"
                  onClick={() => setEditMode(true)}
                  className="text-(--color-text-muted) transition-colors hover:text-(--color-text-primary)"
                  aria-label="레이아웃 편집"
                >
                  <Pencil className="h-4 w-4" />
                </button>
              </div>
            </>
          )}
        </div>
      </header>

      {/* ── 콘텐츠 영역 ── */}
      <div className={`flex-1 overflow-auto ${editMode ? 'bg-slate-50 dark:bg-slate-900' : ''}`}>
        {/* 에러 배너 */}
        {hasError && (
          <div className="mx-6 mt-4 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400">
            데이터를 불러오는 중 오류가 발생했습니다. 새로고침을 시도해주세요.
          </div>
        )}

        {/* 편집 모드 인포 배너 (Pencil: iN5W7) */}
        {editMode && (
          <div className="mx-6 mt-4 flex h-9 items-center gap-2 rounded-lg bg-blue-50 px-4 dark:bg-blue-900/20">
            <Info className="h-3.5 w-3.5 shrink-0 text-blue-500" />
            <span className="text-[11px] text-blue-500">
              {gridCols}칸 그리드 | 타이틀 바 드래그로 이동 | 우측 하단 모서리로 크기 조절 | 최소 2칸, 최대 {gridCols}칸
            </span>
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
                          onClick={() => setSettingsPanelId(panel.id)}
                          className="rounded-full bg-(--color-bg-elevated) p-0.5 text-(--color-text-muted) shadow transition-colors hover:bg-(--color-bg-surface) hover:text-(--color-text-primary)"
                          aria-label="패널 설정"
                          title="패널 설정"
                        >
                          <Settings className="h-3.5 w-3.5" />
                        </button>
                        <button
                          type="button"
                          onClick={() => removePanel(panel.id)}
                          className="rounded-full bg-red-500 p-0.5 text-white shadow transition-colors hover:bg-red-600"
                          aria-label="패널 삭제"
                          title="패널 삭제"
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

      {/* 패널 추가 다이얼로그 */}
      <AddPanelDialog
        open={addPanelOpen}
        onClose={() => setAddPanelOpen(false)}
      />

      {/* 패널 설정 다이얼로그 */}
      <PanelSettingsDialog
        panelId={settingsPanelId}
        onClose={() => setSettingsPanelId(null)}
      />

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
