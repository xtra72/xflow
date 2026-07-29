// 패널 상세 설정 다이얼로그.
// 편집 모드에서 패널별 설정(타이틀, 색상, 컬럼/메트릭 가시성, 타입별 설정)을 관리한다.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ArrowUpDown, Check, ChevronLeft, ChevronRight, Fan, Gauge, Minus, Pipette, Plus, Power, Snowflake, Thermometer, Trash2, X } from 'lucide-react';
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ReferenceArea,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

import { useAgents } from '@/hooks/useAgent';
import { useStations, useAirpurifierDevices } from '@/hooks/useStation';
import { useDevices, useDeviceRealtime } from '@/hooks/useDevice';
import { useFlows } from '@/hooks/useFlow';
import { listChartChannels, type ChartChannelSummary } from '@/services/api/charts';
import { listStoreKeys } from '@/services/api/storeService';
import { resolveStoreAgentName } from './panels/charts/storeAgentResolve';
import GaugePanel, { type GaugeType } from './panels/GaugePanel';
import { getDeviceDisplayName, getDeviceTypeLabel, getPropertyLabel } from '@/lib/utils/deviceLabels';
import {
  buildEnumLabelMap,
  formatEnumValue,
  resolveAxisFont,
  STROKE_DASHARRAY,
  THRESHOLD_DEFAULT_COLORS,
  type AxisFontStyle,
  type ChannelRefConfig,
  type YThreshold,
  type YAxisMode,
  type YAxisDataType,
  type YEnumLabel,
} from './panels/charts/chartChannelTypes';
import {
  ChartChannelSection,
  StoreSourceSection,
  StatChartSection,
  LineChartSection,
  BarChartSection,
  PieChartSection,
  TableChartSection,
} from './ChartPanelSections';
import AcControlStyleSection from './AcControlStyleSection';
import AcControlThresholdsSection from './AcControlThresholdsSection';
import type { ValueColorConfig } from './panels/acControlColors';
import {
  useUIStore,
  type PanelConfig,
  type FlowColumnKey,
  type AgentColumnKey,
  type DeviceColumnKey,
  type MetricKey,
  ALL_FLOW_COLUMNS,
  ALL_AGENT_COLUMNS,
  ALL_DEVICE_COLUMNS,
  ALL_METRIC_KEYS,
} from '@/stores/uiStore';

// ---- 차트 패널 공통 (SPEC-CHART-001 M5) ----

/** 차트 계열 패널 타입 집합 (REQ-M5-03) */
const CHART_PANEL_TYPES = new Set(['stat', 'line-chart', 'bar-chart', 'pie-chart', 'table']);

/** 패널 색상 프리셋 */
const COLOR_PRESETS = [
  '#3b82f6', // blue
  '#10b981', // emerald
  '#f59e0b', // amber
  '#ef4444', // red
  '#8b5cf6', // violet
  '#ec4899', // pink
  '#06b6d4', // cyan
  '#f97316', // orange
  '#64748b', // slate
  '#0f172a', // dark
];

/** 컬럼 라벨 i18n 키 매핑 (기존 dashboard.col.* 재사용) */
const FLOW_COLUMN_LABEL_KEYS: Record<FlowColumnKey, string> = {
  name: 'dashboard.col.name',
  status: 'dashboard.col.status',
  node_count: 'dashboard.col.nodeCount',
  updated_at: 'dashboard.col.updatedAt',
  actions: 'dashboard.col.actions',
};

const AGENT_COLUMN_LABEL_KEYS: Record<AgentColumnKey, string> = {
  name: 'dashboard.col.name',
  type: 'dashboard.col.type',
  status: 'dashboard.col.status',
  uptime: 'dashboard.col.uptime',
  messages: 'dashboard.col.messages',
  actions: 'dashboard.col.actions',
};

const DEVICE_COLUMN_LABEL_KEYS: Record<DeviceColumnKey, string> = {
  name: 'dashboard.col.name',
  type: 'dashboard.col.type',
  status: 'dashboard.col.status',
  agent: 'dashboard.col.agent',
  last_seen: 'dashboard.col.lastSeen',
};

const METRIC_LABEL_KEYS: Record<MetricKey, string> = {
  cpu: 'dashboard.settings.metricLabels.cpu',
  memory: 'dashboard.settings.metricLabels.memory',
  throughput: 'dashboard.settings.metricLabels.throughput',
  errorRate: 'dashboard.settings.metricLabels.errorRate',
};

interface PanelSettingsDialogProps {
  panelId: string | null;
  onClose: () => void;
}

/** 패널 상세 설정 모달 */
export default function PanelSettingsDialog({ panelId, onClose }: PanelSettingsDialogProps) {
  const { t } = useTranslation();
  const activePage = useUIStore((s) =>
    s.dashboardPages.find((p) => p.id === s.activeDashboardId),
  );
  const updatePanelConfig = useUIStore((s) => s.updatePanelConfig);
  const updatePanelTitle = useUIStore((s) => s.updatePanelTitle);

  const storePanel = activePage?.panels.find((p) => p.id === panelId) ?? null;

  // 드래프트: 적용 버튼 전까지 변경을 로컬에 보관
  // panelId 가 바뀔 때만 초기화 — storePanel.config 변경 시 리셋하지 않음
  const [draftConfig, setDraftConfig] = useState<Record<string, unknown>>(() => storePanel?.config ?? {});
  const [draftTitle, setDraftTitle] = useState<string>(() => storePanel?.title ?? '');
  const prevPanelIdRef = useRef(panelId);
  useEffect(() => {
    if (prevPanelIdRef.current !== panelId) {
      prevPanelIdRef.current = panelId;
      setDraftConfig(storePanel?.config ?? {});
      setDraftTitle(storePanel?.title ?? '');
    }
  }, [panelId, storePanel?.config, storePanel?.title]);

  const handleConfigChange = useCallback(
    (patch: Record<string, unknown>) => {
      setDraftConfig((prev) => ({ ...prev, ...patch }));
    },
    [],
  );

  const handleTitleChange = useCallback((title: string) => {
    setDraftTitle(title);
  }, []);

  const handleApply = useCallback(() => {
    if (!storePanel) return;
    updatePanelConfig(storePanel.id, draftConfig);
    updatePanelTitle(storePanel.id, draftTitle);
  }, [storePanel, draftConfig, draftTitle, updatePanelConfig, updatePanelTitle]);

  const handleApplyAndClose = useCallback(() => {
    handleApply();
    onClose();
  }, [handleApply, onClose]);

  // 드래프트를 반영한 가상 패널 (미리보기 + 설정 컴포넌트용)
  const panel = useMemo(
    () =>
      storePanel
        ? { ...storePanel, config: draftConfig, title: draftTitle }
        : null,
    [storePanel, draftConfig, draftTitle],
  );

  // 악센트 그룹 선택 상태 (좌측 컬럼에 컨트롤 표시용)
  const [selectedGroup, setSelectedGroup] = useState<string | null>(null);

  // 악센트 관련 파생값
  const accentElements = (panel?.config?.accentElements as Record<string, string | boolean>) ?? {};
  const panelColor = panel?.config?.panelColor as string | undefined;
  const effectiveColor = (group: string): string | undefined => {
    if (accentElements[group] === false) return undefined;
    const val = accentElements[group];
    if (typeof val === 'string') return val;
    return panelColor;
  };
  const getSubProp = (group: string, prop: string): string | undefined => {
    const val = accentElements[`${group}.${prop}`];
    return typeof val === 'string' ? val : undefined;
  };

  // 패널 변경 시 선택 그룹 초기화
  useEffect(() => {
    setSelectedGroup(null);
  }, [panelId]);

  // 미리보기 패널 접기 상태 (localStorage 영속)
  const [previewCollapsed, setPreviewCollapsed] = useState<boolean>(() => {
    if (typeof window === 'undefined') return false;
    return window.localStorage.getItem('panelSettings.previewCollapsed') === '1';
  });
  useEffect(() => {
    if (typeof window === 'undefined') return;
    window.localStorage.setItem(
      'panelSettings.previewCollapsed',
      previewCollapsed ? '1' : '0',
    );
  }, [previewCollapsed]);

  // 미리보기 줌 배율 (0.5 ~ 2.0). 버튼 / Ctrl+휠 / 더블클릭 리셋 으로 조절.
  const PREVIEW_ZOOM_MIN = 0.5;
  const PREVIEW_ZOOM_MAX = 2.0;
  const PREVIEW_ZOOM_STEP = 0.1;
  const [previewZoom, setPreviewZoom] = useState<number>(1.0);
  const zoomIn = useCallback(
    () => setPreviewZoom((z) => Math.min(PREVIEW_ZOOM_MAX, Math.round((z + PREVIEW_ZOOM_STEP) * 10) / 10)),
    [],
  );
  const zoomOut = useCallback(
    () => setPreviewZoom((z) => Math.max(PREVIEW_ZOOM_MIN, Math.round((z - PREVIEW_ZOOM_STEP) * 10) / 10)),
    [],
  );
  const zoomReset = useCallback(() => setPreviewZoom(1.0), []);
  const handlePreviewWheel = useCallback(
    (e: React.WheelEvent<HTMLDivElement>) => {
      // Ctrl/Meta + 휠 만 줌으로 처리 (일반 스크롤 보존).
      if (!e.ctrlKey && !e.metaKey) return;
      e.preventDefault();
      if (e.deltaY < 0) zoomIn();
      else if (e.deltaY > 0) zoomOut();
    },
    [zoomIn, zoomOut],
  );

  // 좌측 컬럼 너비 (px) - 드래그 리사이저로 조절, localStorage 영속
  const LEFT_MIN = 240;
  const LEFT_MAX = 800;
  const [leftWidth, setLeftWidth] = useState<number>(() => {
    if (typeof window === 'undefined') return 360;
    const stored = window.localStorage.getItem('panelSettings.leftWidth');
    const n = stored ? parseInt(stored, 10) : NaN;
    return Number.isFinite(n) ? Math.max(LEFT_MIN, Math.min(LEFT_MAX, n)) : 360;
  });
  useEffect(() => {
    if (typeof window === 'undefined') return;
    window.localStorage.setItem('panelSettings.leftWidth', String(leftWidth));
  }, [leftWidth]);

  // 드래그 상태 — mousemove/mouseup 은 window 에 부착
  const [isDragging, setIsDragging] = useState(false);
  useEffect(() => {
    if (!isDragging) return;
    const onMove = (e: MouseEvent) => {
      // 컬럼 배치: [미리보기 (flex-1)] [splitter] [설정 (leftWidth, 우측 고정폭)]
      // 설정 컬럼이 우측에 고정되므로 너비는 다이얼로그 우측 가장자리 기준으로 계산한다.
      //   - 스플리터를 오른쪽으로 드래그 → 마우스 X 증가 → rect.right - clientX 감소
      //     → leftWidth(=설정 폭) 감소 → 미리보기 영역이 넓어짐 (직관에 일치)
      //   - 스플리터를 왼쪽으로 드래그 → 설정 폭 증가
      const dialog = document.querySelector(
        '[data-panel-settings-content]',
      ) as HTMLElement | null;
      if (!dialog) return;
      const rect = dialog.getBoundingClientRect();
      const next = Math.max(
        LEFT_MIN,
        Math.min(LEFT_MAX, rect.right - e.clientX),
      );
      setLeftWidth(next);
    };
    const onUp = () => setIsDragging(false);
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
    return () => {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };
  }, [isDragging]);

  // 악센트 라벨 키 결정 (값은 i18n 키, 렌더 시 t() 로 변환)
  const accentLabelKeys = panel?.type === 'device' || panel?.type === 'ac-control' || panel?.type === 'hvac-control' || panel?.type === 'properties-grid'
    ? ACCENT_ELEMENT_LABEL_KEYS
    : panel?.type === 'resource'
    ? RESOURCE_ACCENT_LABEL_KEYS
    : panel?.type === 'logs'
    ? LOG_ACCENT_LABEL_KEYS
    : panel?.type === 'gauge'
    ? GAUGE_ACCENT_LABEL_KEYS
    : LIST_ACCENT_LABEL_KEYS;

  // ESC 키로 닫기
  useEffect(() => {
    if (!panel) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [panel, onClose]);

  // 배경 클릭 시 닫기
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget) onClose();
    },
    [onClose],
  );

  if (!panel) return null;

  // SPEC-WEB-005: 차트 패널이면 데이터 소스 섹션을 좌측 프리뷰 아래에 넓게 배치한다.
  // 프리뷰가 접혀도 데이터 소스 섹션은 좌측 영역에 계속 노출된다.
  const isChartPanel = CHART_PANEL_TYPES.has(panel.type);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="panel-settings-dialog-title"
    >
      <div className="mx-4 flex h-[min(900px,95vh)] w-full max-w-[min(1400px,95vw)] flex-col rounded-2xl bg-(--color-bg-surface) shadow-xl">
        {/* 헤더 */}
        <div className="flex shrink-0 items-center justify-between px-5 pt-4 pb-3">
          <h2
            id="panel-settings-dialog-title"
            className="text-base font-semibold text-(--color-text-primary)"
          >
            {t('dashboard.settings.title')}
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={t('dashboard.settings.closeAria')}
          >
            <X className="h-4.5 w-4.5" />
          </button>
        </div>

        <div className="border-t border-(--color-border-default)" />

        {/* 설정 내용 - 2컬럼 레이아웃 (드래그 리사이저)
         *
         * 컬럼 배치 (Grafana 패턴):
         *   [미리보기 (flex-1)] [splitter] [설정 (leftWidth)]
         * 미리보기를 좌측에 두면 시선이 자연스럽게 미리보기 → 설정으로 흐르고,
         * 설정 패널은 사이드바처럼 우측에 고정된다.
         *
         * CSS flexbox `order` 로 시각적 순서를 제어한다 (JSX 가독성과 분리).
         *   - 미리보기 / expand 버튼: order-1
         *   - splitter: order-2
         *   - 설정: order-3
         */}
        <div
          data-panel-settings-content
          className="relative flex min-h-0 flex-1 gap-3 px-5 pb-5 pt-4"
        >
          <div
            style={{
              // 차트 패널은 접힘 상태에서도 좌측 데이터 소스 영역이 남으므로
              // 설정 컬럼을 고정 폭으로 유지한다. 그 외에는 접힘 시 전체 폭.
              width:
                previewCollapsed && !isChartPanel ? '100%' : `${leftWidth}px`,
            }}
            className={cn(
              'order-3 shrink-0 space-y-0.5 overflow-y-auto pr-1',
              previewCollapsed && !isChartPanel && 'flex-1',
            )}
          >
            {/*
              공통: 타이틀 / 디바이스 — "패널 옵션" CollapsibleSection 으로 그룹화 (Grafana 패턴).
              디바이스 필드는 패널 타입별 조건부.
            */}
            <CollapsibleSection title={t('dashboard.settings.panelOptions')}>
              <div className="space-y-3">
                <TitleSection panel={panel} onTitleChange={(v) => handleTitleChange(v)} />
                {(panel.type === 'device' || panel.type === 'ac-control' || panel.type === 'hvac-control' || panel.type === 'properties-grid') && (
                  <DeviceSection
                    panel={panel}
                    onConfigChange={(c) => handleConfigChange(c)}
                  />
                )}
              </div>
            </CollapsibleSection>

            {/* 타입별 설정 */}
            {panel.type === 'flows' && (
              <CollapsibleSection title={t('dashboard.settings.columns')}>
                <ColumnsSection<FlowColumnKey>
                  allColumns={[...ALL_FLOW_COLUMNS]}
                  labels={Object.fromEntries(ALL_FLOW_COLUMNS.map((k) => [k, t(FLOW_COLUMN_LABEL_KEYS[k])])) as Record<FlowColumnKey, string>}
                  visibleColumns={(panel.config?.visibleColumns as FlowColumnKey[]) ?? [...ALL_FLOW_COLUMNS]}
                  onChange={(cols) => updatePanelConfig(panel.id, { visibleColumns: cols })}
                />
              </CollapsibleSection>
            )}
            {panel.type === 'agents' && (
              <CollapsibleSection title={t('dashboard.settings.columns')}>
                <ColumnsSection<AgentColumnKey>
                  allColumns={[...ALL_AGENT_COLUMNS]}
                  labels={Object.fromEntries(ALL_AGENT_COLUMNS.map((k) => [k, t(AGENT_COLUMN_LABEL_KEYS[k])])) as Record<AgentColumnKey, string>}
                  visibleColumns={(panel.config?.visibleColumns as AgentColumnKey[]) ?? [...ALL_AGENT_COLUMNS]}
                  onChange={(cols) => updatePanelConfig(panel.id, { visibleColumns: cols })}
                />
              </CollapsibleSection>
            )}
            {panel.type === 'devices' && (
              <CollapsibleSection title={t('dashboard.settings.columns')}>
                <ColumnsSection<DeviceColumnKey>
                  allColumns={[...ALL_DEVICE_COLUMNS]}
                  labels={Object.fromEntries(ALL_DEVICE_COLUMNS.map((k) => [k, t(DEVICE_COLUMN_LABEL_KEYS[k])])) as Record<DeviceColumnKey, string>}
                  visibleColumns={(panel.config?.visibleColumns as DeviceColumnKey[]) ?? [...ALL_DEVICE_COLUMNS]}
                  onChange={(cols) => updatePanelConfig(panel.id, { visibleColumns: cols })}
                />
              </CollapsibleSection>
            )}
            {panel.type === 'resource' && (
              <CollapsibleSection title={t('dashboard.settings.resource')}>
                <ResourceSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}
            {panel.type === 'logs' && (
              <CollapsibleSection title={t('dashboard.settings.logs')}>
                <LogsSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}
            {panel.type === 'gauge' && (
              <CollapsibleSection title={t('dashboard.settings.gauge')}>
                <GaugeSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}
            {panel.type === 'properties-grid' && (
              <CollapsibleSection title={t('dashboard.settings.propertiesGrid')}>
                <PropertiesGridSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}
            {/* SPEC-FACILITY-DASHBOARD-001 M5: 설비 패널 3종 (에이전트 + 라인/역사/기기) */}
            {(panel.type === 'facility-line' ||
              panel.type === 'facility-station' ||
              panel.type === 'facility-device') && (
              <CollapsibleSection title={t('dashboard.settings.facility')} defaultOpen={true}>
                <FacilitySection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}

            {/*
              차트 패널 공통: channel_name (line-chart 는 channels 로 통합됨).
              data_source === 'store' 인 경우에도 채널 설정은 유지된다(공존, 하위 호환).
              데이터 소스 섹션(StoreSourceSection)은 좌측 프리뷰 아래로 이동했다(SPEC-WEB-005).
            */}
            {CHART_PANEL_TYPES.has(panel.type) && panel.type !== 'line-chart' && (
              <CollapsibleSection title={t('dashboard.settings.channel')}>
                <ChartChannelSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}

            {/* 차트 타입별 세부 설정 (SPEC-CHART-001 §4.2.2 / REQ-M5-03) */}
            {panel.type === 'stat' && (
              <CollapsibleSection title={t('dashboard.settings.statSettings')}>
                <StatChartSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}
            {panel.type === 'line-chart' && (
              <CollapsibleSection title={t('dashboard.settings.lineChartSettings')}>
                <LineChartSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}
            {panel.type === 'bar-chart' && (
              <CollapsibleSection title={t('dashboard.settings.barChartSettings')}>
                <BarChartSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}
            {panel.type === 'pie-chart' && (
              <CollapsibleSection title={t('dashboard.settings.pieChartSettings')}>
                <PieChartSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}
            {panel.type === 'table' && (
              <CollapsibleSection title={t('dashboard.settings.tableSettings')}>
                <TableChartSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}

            {/*
              임계값 섹션 (ac-control 전용) — 스타일 섹션 위에 배치.
              구간(range) 기반 색상 매핑: 빈 입력은 -∞/+∞ 의미.
            */}
            {panel.type === 'ac-control' && (
              <CollapsibleSection title={t('dashboard.settings.thresholds')} defaultOpen={true}>
                <AcControlThresholdsSection
                  config={panel.config?.valueColor as ValueColorConfig | undefined}
                  onChange={(next) => handleConfigChange({ valueColor: next })}
                />
              </CollapsibleSection>
            )}

            {/*
              스타일 섹션:
              - ac-control 패널은 통합 스타일 섹션 (모든 항목을 항상 표시)을 사용한다.
              - 그 외 패널은 미리보기에서 그룹 선택 시 표시되는 AccentGroupControls 를 사용한다.
            */}
            {panel.type === 'ac-control' ? (
              <CollapsibleSection title={t('dashboard.settings.style')} defaultOpen={true}>
                <AcControlStyleSection
                  panelColor={panelColor}
                  accentElements={accentElements}
                  config={panel.config ?? {}}
                  onPanelColorChange={(c) => handleConfigChange({ panelColor: c })}
                  onAccentChange={(elements) => handleConfigChange({ accentElements: elements })}
                  onConfigChange={(patch) => handleConfigChange(patch)}
                />
              </CollapsibleSection>
            ) : selectedGroup ? (
              <CollapsibleSection title={t('dashboard.settings.style')} defaultOpen={true}>
                <AccentGroupControls
                  selected={selectedGroup}
                  labelKeys={accentLabelKeys}
                  accentElements={accentElements}
                  panelColor={panelColor}
                  onChange={(elements) => handleConfigChange({ accentElements: elements })}
                  onPanelColorChange={(color) => handleConfigChange({ panelColor: color })}
                />
              </CollapsibleSection>
            ) : null}

          </div>

          {/*
            미리보기 펼치기 버튼: collapsed 시 좌측에 표시 (order-1).
            ChevronRight 는 "오른쪽으로 펼쳐서 미리보기를 보여준다" 의미.
          */}
          {previewCollapsed && (
            <button
              type="button"
              onClick={() => setPreviewCollapsed(false)}
              data-testid="panel-settings-preview-expand"
              aria-label={t('dashboard.settings.previewExpandAria')}
              title={t('dashboard.settings.previewExpandAria')}
              className="order-1 flex w-6 shrink-0 cursor-pointer items-center justify-center rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) text-(--color-text-muted) hover:bg-(--color-bg-hover) hover:text-(--color-text-default)"
            >
              <ChevronRight className="h-4 w-4" />
            </button>
          )}

          {/* 드래그 리사이저 (collapsed 가 아닐 때만) — order-2 */}
          {!previewCollapsed && (
            <div
              role="separator"
              aria-orientation="vertical"
              aria-label={t('dashboard.settings.splitterAria')}
              data-testid="panel-settings-splitter"
              onMouseDown={(e) => {
                e.preventDefault();
                setIsDragging(true);
              }}
              className={cn(
                'group relative order-2 -mx-1 flex w-2 shrink-0 cursor-col-resize items-center justify-center',
                isDragging && 'bg-blue-500/20',
              )}
            >
              <div
                className={cn(
                  'h-12 w-0.5 rounded-full bg-(--color-border-default) transition-colors',
                  'group-hover:bg-blue-400',
                  isDragging && 'bg-blue-500',
                )}
              />
            </div>
          )}

          {/* 우측 컬럼: 프리뷰 + 악센트 컨트롤 */}
          {/*
            justify-center 사용 시, 하단 AccentGroupControls 가 조건부 렌더링되며
            미리보기 박스가 위/아래로 이동하는 문제가 있어 justify-start 로 변경.
            미리보기는 항상 같은 위치 (상단) 에 고정되고, 악센트 컨트롤은 그 아래에 추가된다.
            미리보기 자체는 mx-auto + zoom 으로 가운데 정렬되며 사용자가 ± 버튼이나
            Ctrl+휠 로 확대/축소 할 수 있다.
          */}
          {(!previewCollapsed || isChartPanel) && (
          <div className="order-1 flex min-w-0 flex-1 flex-col items-stretch justify-start gap-3 overflow-y-auto">
            {/*
              프리뷰 영역(툴바 + 미리보기 블록)은 접히면 숨긴다. 차트 패널의
              데이터 소스 섹션은 이 아래에 별도로 항상 노출된다(SPEC-WEB-005).
            */}
            {!previewCollapsed && (
            <>
            <div className="flex shrink-0 items-center justify-between gap-2">
              <label className="text-xs font-medium text-(--color-text-muted)">{t('dashboard.settings.previewLabel')}</label>
              <div className="flex items-center gap-1">
                {/* 줌 컨트롤 */}
                <button
                  type="button"
                  onClick={zoomOut}
                  disabled={previewZoom <= PREVIEW_ZOOM_MIN + 1e-6}
                  data-testid="panel-settings-preview-zoom-out"
                  aria-label={t('dashboard.settings.zoomOutAria')}
                  title={t('dashboard.settings.zoomOutTitle')}
                  className="flex h-5 w-5 items-center justify-center rounded text-(--color-text-muted) hover:bg-(--color-bg-hover) hover:text-(--color-text-default) disabled:opacity-40"
                >
                  <Minus className="h-3 w-3" />
                </button>
                <button
                  type="button"
                  onClick={zoomReset}
                  data-testid="panel-settings-preview-zoom-reset"
                  aria-label={t('dashboard.settings.zoomResetAria')}
                  title={t('dashboard.settings.zoomResetTitle')}
                  className="min-w-10 rounded px-1 text-[10px] font-medium tabular-nums text-(--color-text-muted) hover:bg-(--color-bg-hover) hover:text-(--color-text-default)"
                >
                  {Math.round(previewZoom * 100)}%
                </button>
                <button
                  type="button"
                  onClick={zoomIn}
                  disabled={previewZoom >= PREVIEW_ZOOM_MAX - 1e-6}
                  data-testid="panel-settings-preview-zoom-in"
                  aria-label={t('dashboard.settings.zoomInAria')}
                  title={t('dashboard.settings.zoomInTitle')}
                  className="flex h-5 w-5 items-center justify-center rounded text-(--color-text-muted) hover:bg-(--color-bg-hover) hover:text-(--color-text-default) disabled:opacity-40"
                >
                  <Plus className="h-3 w-3" />
                </button>
                <div className="mx-1 h-3 w-px bg-(--color-border-default)" />
                <button
                  type="button"
                  onClick={() => setPreviewCollapsed(true)}
                  data-testid="panel-settings-preview-collapse"
                  aria-label={t('dashboard.settings.previewCollapseAria')}
                  title={t('dashboard.settings.previewCollapseAria')}
                  className="flex h-5 w-5 items-center justify-center rounded text-(--color-text-muted) hover:bg-(--color-bg-hover) hover:text-(--color-text-default)"
                >
                  <ChevronLeft className="h-3.5 w-3.5" />
                </button>
              </div>
            </div>
            {/*
              각 미리보기를 패널 유형별 기본 그리드 비율과 일치하는 wrapper 로 감싸서
              크기 비율을 고정한다. mx-auto 로 가운데 정렬되고, previewZoom 으로
              버튼/Ctrl+휠 확대축소가 가능하다 (인라인 width 가 베이스 max 에 zoom 곱한 값).
              wheel 핸들러는 패널 유형별 분기 바깥의 wrapper(아래) 가 아니라
              개별 wrapper 에 부여한다 (Ctrl+휠 으로만 동작하므로 기본 스크롤은 보존).
            */}
            {(panel.type === 'device' || panel.type === 'ac-control' || panel.type === 'hvac-control') && (
              <div
                className="mx-auto"
                style={{ width: `${28 * previewZoom}rem`, maxWidth: '100%', aspectRatio: '3 / 2' }}
                onWheel={handlePreviewWheel}
              >
                <NasaMiniPreview
                  selectedGroup={selectedGroup}
                  onSelectGroup={setSelectedGroup}
                  effectiveColor={effectiveColor}
                  getSubProp={getSubProp}
                  panelColor={panelColor}
                />
              </div>
            )}
            {panel.type === 'properties-grid' && (
              <div
                className="mx-auto"
                style={{ width: `${28 * previewZoom}rem`, maxWidth: '100%', aspectRatio: '3 / 2' }}
                onWheel={handlePreviewWheel}
              >
                <GridMiniPreview
                  selectedGroup={selectedGroup}
                  onSelectGroup={setSelectedGroup}
                  effectiveColor={effectiveColor}
                  panelColor={panelColor}
                  gridCols={(panel.config?.gridCols as number | undefined) ?? 3}
                />
              </div>
            )}
            {(panel.type === 'flows' || panel.type === 'agents' || panel.type === 'devices') && (
              <div
                className="mx-auto"
                style={{ width: `${28 * previewZoom}rem`, maxWidth: '100%', aspectRatio: '3 / 2' }}
                onWheel={handlePreviewWheel}
              >
                <ListMiniPreview
                  selectedGroup={selectedGroup}
                  onSelectGroup={setSelectedGroup}
                  effectiveColor={effectiveColor}
                  panelColor={panelColor}
                  variant={panel.type as 'flows' | 'agents' | 'devices'}
                />
              </div>
            )}
            {panel.type === 'resource' && (
              <div
                className="mx-auto"
                style={{ width: `${28 * previewZoom}rem`, maxWidth: '100%', aspectRatio: '4 / 3' }}
                onWheel={handlePreviewWheel}
              >
                <ResourceMiniPreview
                  selectedGroup={selectedGroup}
                  onSelectGroup={setSelectedGroup}
                  effectiveColor={effectiveColor}
                  panelColor={panelColor}
                />
              </div>
            )}
            {panel.type === 'logs' && (
              <div
                className="mx-auto"
                style={{ width: `${28 * previewZoom}rem`, maxWidth: '100%', aspectRatio: '3 / 2' }}
                onWheel={handlePreviewWheel}
              >
                <LogMiniPreview
                  selectedGroup={selectedGroup}
                  onSelectGroup={setSelectedGroup}
                  effectiveColor={effectiveColor}
                  panelColor={panelColor}
                />
              </div>
            )}
            {panel.type === 'gauge' && (
              <div
                className="mx-auto"
                style={{ width: `${24 * previewZoom}rem`, maxWidth: '100%', aspectRatio: '1 / 1' }}
                onWheel={handlePreviewWheel}
              >
                <GaugeMiniPreview panel={panel} />
              </div>
            )}
            {panel.type === 'line-chart' && (
              <div
                className="mx-auto"
                style={{ width: `${42 * previewZoom}rem`, maxWidth: '100%', aspectRatio: '16 / 9' }}
                onWheel={handlePreviewWheel}
              >
                <LineChartMiniPreview panel={panel} />
              </div>
            )}
            {/* 악센트 그룹 컨트롤은 좌측 컬럼으로 이동되었음 (스타일 섹션) */}
            </>
            )}

            {/*
              데이터 소스 섹션(채널/Store 토글 + Store 테이블/필터 + 선택 시리즈) —
              SPEC-WEB-005: 넓은 좌측 공간을 활용해 프리뷰 아래에 배치한다. 프리뷰가
              접혀도 차트 패널이면 이 섹션은 계속 노출된다.
            */}
            {isChartPanel && (
              <div data-testid="panel-settings-data-source" className="shrink-0">
                <CollapsibleSection title={t('dashboard.settings.dataSource')}>
                  <StoreSourceSection
                    panel={panel}
                    onConfigChange={(c) => handleConfigChange(c)}
                  />
                </CollapsibleSection>
              </div>
            )}
          </div>
          )}
        </div>

        {/* 하단: 적용 / 취소 */}
        <div className="flex shrink-0 items-center justify-end gap-2 border-t border-(--color-border-default) px-5 py-3">
          <button
            type="button"
            onClick={onClose}
            className="rounded-md px-4 py-1.5 text-sm font-medium text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated)"
          >
            {t('common.cancel')}
          </button>
          <button
            type="button"
            onClick={handleApplyAndClose}
            className="rounded-md bg-blue-600 px-4 py-1.5 text-sm font-medium text-white transition-colors hover:bg-blue-700"
          >
            {t('dashboard.settings.apply')}
          </button>
        </div>
      </div>
    </div>
  );
}

/**
 * 접기/펼치기 가능한 설정 섹션 wrapper (Grafana 스타일).
 *
 * Grafana 패널 옵션 패턴을 따른다:
 *   - 둘러싸는 박스/배경 없음 (플랫)
 *   - 섹션 사이는 하단 테두리 1px 로만 구분
 *   - 헤더는 chevron(왼쪽) + 굵은 제목, 펼침 시 chevron 회전
 *   - 본문은 들여쓰기 없이 padding 만 사용
 *
 * native `<details>` 사용으로 키보드/스크린리더 접근성 보장.
 */
function CollapsibleSection({
  title,
  defaultOpen = true,
  children,
}: {
  title: string;
  defaultOpen?: boolean;
  children: React.ReactNode;
}) {
  return (
    <details
      open={defaultOpen}
      className="group border-b border-(--color-border-default) [&[open]>summary>svg]:rotate-90 last:border-b-0"
    >
      <summary className="flex cursor-pointer select-none items-center gap-1.5 px-1 py-2 text-xs font-semibold text-(--color-text-primary) marker:hidden [&::-webkit-details-marker]:hidden hover:text-blue-500">
        <ChevronRight className="h-3.5 w-3.5 text-(--color-text-muted) transition-transform" />
        <span>{title}</span>
      </summary>
      <div className="px-1 pb-3">
        {children}
      </div>
    </details>
  );
}

/** 타이틀 입력 섹션 */
function TitleSection({
  panel,
  onTitleChange,
}: {
  panel: PanelConfig;
  onTitleChange: (title: string) => void;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState(panel.title);

  useEffect(() => {
    setDraft(panel.title);
  }, [panel.title]);

  const handleBlur = () => {
    const trimmed = draft.trim();
    if (trimmed && trimmed !== panel.title) {
      onTitleChange(trimmed);
    } else {
      setDraft(panel.title);
    }
  };

  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
        {t('dashboard.settings.titleLabel')}
      </label>
      <input
        type="text"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={handleBlur}
        onKeyDown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
        className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
      />
    </div>
  );
}

/** 색상 선택 섹션 */
/** 컬럼 가시성 토글 섹션 (flows, agents 공용) */
function ColumnsSection<T extends string>({
  allColumns,
  labels,
  visibleColumns,
  onChange,
}: {
  allColumns: T[];
  labels: Record<T, string>;
  visibleColumns: T[];
  onChange: (cols: T[]) => void;
}) {
  const { t } = useTranslation();
  const toggle = (key: T) => {
    if (visibleColumns.includes(key)) {
      if (visibleColumns.length > 1) {
        onChange(visibleColumns.filter((c) => c !== key));
      }
    } else {
      onChange([...visibleColumns, key]);
    }
  };

  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
        {t('dashboard.settings.visibleColumns')}
      </label>
      <div className="space-y-1">
        {allColumns.map((key) => (
          <label
            key={key}
            className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-(--color-bg-elevated)"
          >
            <input
              type="checkbox"
              checked={visibleColumns.includes(key)}
              onChange={() => toggle(key)}
              className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
            />
            <span className="text-sm text-(--color-text-primary)">{labels[key]}</span>
          </label>
        ))}
      </div>
    </div>
  );
}

/** 리소스 패널 전용 설정 (메트릭 + 그리드 열 수) */
function ResourceSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const visibleMetrics = (panel.config?.visibleMetrics as MetricKey[]) ?? [...ALL_METRIC_KEYS];
  const gridCols = (panel.config?.gridCols as number | undefined) ?? visibleMetrics.length;

  const toggleMetric = (key: MetricKey) => {
    if (visibleMetrics.includes(key)) {
      if (visibleMetrics.length > 1) {
        onConfigChange({ visibleMetrics: visibleMetrics.filter((m) => m !== key) });
      }
    } else {
      onConfigChange({ visibleMetrics: [...visibleMetrics, key] });
    }
  };

  return (
    <>
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.visibleMetrics')}
        </label>
        <div className="space-y-1">
          {ALL_METRIC_KEYS.map((key) => (
            <label
              key={key}
              className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-(--color-bg-elevated)"
            >
              <input
                type="checkbox"
                checked={visibleMetrics.includes(key)}
                onChange={() => toggleMetric(key)}
                className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              <span className="text-sm text-(--color-text-primary)">{t(METRIC_LABEL_KEYS[key])}</span>
            </label>
          ))}
        </div>
      </div>
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.columnCount')}
        </label>
        <div className="flex gap-1">
          {[1, 2, 3, 4].map((n) => (
            <button
              key={n}
              type="button"
              onClick={() => onConfigChange({ gridCols: n })}
              className={`flex-1 rounded px-3 py-1.5 text-sm font-medium transition-colors ${
                gridCols === n
                  ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                  : 'bg-(--color-bg-elevated) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)/80'
              }`}
            >
              {n}
            </button>
          ))}
        </div>
      </div>
    </>
  );
}

/** 디바이스 제어 패널 전용 설정 (디바이스 선택) */
function DeviceSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const currentDeviceId = panel.config?.deviceId as string | undefined;
  const { data: devicesData, isLoading } = useDevices();
  const allDevices = devicesData?.data ?? [];

  // ac-control/hvac-control 패널은 실외기 (HVACR.ODU, 레거시 outdoor) 를 제외한다 (v0.18.3).
  const devices = (panel.type === 'ac-control' || panel.type === 'hvac-control')
    ? allDevices.filter((d) => d.type !== 'HVACR.ODU' && d.type !== 'outdoor')
    : allDevices;

  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
        {t('dashboard.settings.device')}
      </label>
      {isLoading ? (
        <div className="flex items-center justify-center py-4">
          <div className="h-5 w-5 animate-spin rounded-full border-2 border-(--color-border-strong) border-t-blue-600" />
        </div>
      ) : devices.length === 0 ? (
        <p className="py-2 text-sm text-(--color-text-muted)">{t('dashboard.settings.noDevices')}</p>
      ) : (
        <select
          value={currentDeviceId ?? ''}
          onChange={(e) => onConfigChange({ deviceId: e.target.value || undefined })}
          className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        >
          <option value="">{t('dashboard.settings.selectDevice')}</option>
          {devices.map((device) => (
            <option key={device.uid ?? device.id} value={device.id}>
              {getDeviceDisplayName(device)} ({getDeviceTypeLabel(device.type)})
            </option>
          ))}
        </select>
      )}
    </div>
  );
}

/**
 * 설비 패널 전용 설정 (SPEC-FACILITY-DASHBOARD-001 M5).
 *
 * 에이전트(airpurifier)를 고르고, 패널 타입에 따라 라인/역사/기기 대상을 고른다.
 * 대상 조회는 기존 useStations / useAirpurifierDevices 를 재사용한다(UB-001).
 * 변경은 onConfigChange({ agentId }) / ({ deviceId | station | line }) 로 config 에만 기록한다.
 */
function FacilitySection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const agentId = (panel.config?.agentId as string | undefined) ?? '';

  const { data: agentsResult } = useAgents();
  const airAgents = useMemo(
    () => (agentsResult?.data ?? []).filter((a) => a.type === 'airpurifier'),
    [agentsResult],
  );

  const { data: stations, isLoading: stationsLoading } = useStations(agentId);
  const { data: devices, isLoading: devicesLoading } = useAirpurifierDevices(agentId);

  // 패널 타입별 대상 키 / 현재 값 / i18n 라벨.
  const targetKey: 'deviceId' | 'station' | 'line' =
    panel.type === 'facility-device' ? 'deviceId' : panel.type === 'facility-station' ? 'station' : 'line';
  const currentTarget = (panel.config?.[targetKey] as string | undefined) ?? '';
  const targetLabelKey =
    panel.type === 'facility-device'
      ? 'dashboard.settings.device'
      : panel.type === 'facility-station'
        ? 'dashboard.settings.station'
        : 'dashboard.settings.line';
  const targetPlaceholderKey =
    panel.type === 'facility-device'
      ? 'dashboard.settings.selectDevice'
      : panel.type === 'facility-station'
        ? 'dashboard.settings.selectStation'
        : 'dashboard.settings.selectLine';

  const targetOptions: { value: string; label: string }[] = useMemo(() => {
    if (panel.type === 'facility-device') {
      return (devices ?? []).map((d) => ({ value: d.device_id, label: d.name || d.device_id }));
    }
    if (panel.type === 'facility-station') {
      return (stations ?? []).map((s) => ({ value: s.station, label: s.display_name || s.station }));
    }
    const lines = Array.from(new Set((stations ?? []).map((s) => s.line).filter(Boolean)));
    return lines.map((l) => ({ value: l, label: l }));
  }, [panel.type, devices, stations]);

  const targetLoading = panel.type === 'facility-device' ? devicesLoading : stationsLoading;

  return (
    <div className="space-y-3">
      {/* 에이전트 선택 */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.agent')}
        </label>
        <select
          value={agentId}
          onChange={(e) => onConfigChange({ agentId: e.target.value })}
          className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        >
          <option value="">{t('dashboard.settings.selectAgent')}</option>
          {airAgents.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </select>
      </div>

      {/* 대상(라인/역사/기기) 선택 */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t(targetLabelKey)}
        </label>
        <select
          value={currentTarget}
          onChange={(e) => onConfigChange({ [targetKey]: e.target.value })}
          disabled={!agentId || targetLoading}
          className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 disabled:opacity-60"
        >
          <option value="">{t(targetPlaceholderKey)}</option>
          {targetOptions.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </div>
    </div>
  );
}

/** 속성 그리드 패널 전용 설정 (열 수 + 표시 항목) */
function PropertiesGridSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const gridCols = (panel.config?.gridCols as number | undefined) ?? 3;
  const visibleProperties = (panel.config?.visibleProperties as string[] | undefined) ?? [];
  const deviceId = panel.config?.deviceId as string | undefined;
  const { data: deviceData } = useDeviceRealtime(deviceId ?? '');
  const allKeys = deviceData?.state?.properties ? Object.keys(deviceData.state.properties) : [];
  const protocol = deviceData?.protocol ?? '';
  const type = deviceData?.type ?? '';

  const toggleProperty = (key: string) => {
    if (visibleProperties.includes(key)) {
      onConfigChange({ visibleProperties: visibleProperties.filter((k) => k !== key) });
    } else {
      onConfigChange({ visibleProperties: [...visibleProperties, key] });
    }
  };

  const isAllSelected = visibleProperties.length === 0;

  return (
    <>
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.columnCount')}
        </label>
        <div className="flex gap-1">
          {[1, 2, 3, 4, 5, 6].map((n) => (
            <button
              key={n}
              type="button"
              onClick={() => onConfigChange({ gridCols: n })}
              className={`flex-1 rounded px-3 py-1.5 text-sm font-medium transition-colors ${
                gridCols === n
                  ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                  : 'bg-(--color-bg-elevated) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)/80'
              }`}
            >
              {n}
            </button>
          ))}
        </div>
      </div>
      {allKeys.length > 0 && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.settings.visibleColumns')}
          </label>
          <div className="space-y-1">
            <label
              className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-(--color-bg-elevated)"
            >
              <input
                type="checkbox"
                checked={isAllSelected}
                onChange={() => onConfigChange({ visibleProperties: [] })}
                className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              <span className="text-sm font-medium text-(--color-text-primary)">{t('dashboard.settings.selectAll')}</span>
            </label>
            {allKeys.map((key) => (
              <label
                key={key}
                className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-(--color-bg-elevated)"
              >
                <input
                  type="checkbox"
                  checked={isAllSelected || visibleProperties.includes(key)}
                  onChange={() => {
                    if (isAllSelected) {
                      // "전체" 해제 → 이 항목만 제외
                      onConfigChange({ visibleProperties: allKeys.filter((k) => k !== key) });
                    } else {
                      toggleProperty(key);
                    }
                  }}
                  className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                />
                <span className="text-sm text-(--color-text-primary)">{getPropertyLabel(key, protocol, type)}</span>
              </label>
            ))}
          </div>
        </div>
      )}
    </>
  );
}

/** 로그 패널 전용 설정 (최대 줄 수) */
function LogsSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const maxLines = (panel.config?.maxLines as number) || 100;
  const [draft, setDraft] = useState(String(maxLines));

  useEffect(() => {
    setDraft(String(maxLines));
  }, [maxLines]);

  const handleBlur = () => {
    const n = parseInt(draft, 10);
    if (!isNaN(n) && n > 0 && n !== maxLines) {
      onConfigChange({ maxLines: n });
    } else {
      setDraft(String(maxLines));
    }
  };

  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
        {t('dashboard.settings.maxLines')}
      </label>
      <input
        type="number"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={handleBlur}
        onKeyDown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
        min={10}
        max={10000}
        className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
      />
    </div>
  );
}

/** 악센트 적용 요소 그룹 (디바이스 리모컨) — 값은 i18n 키 */
const ACCENT_ELEMENT_LABEL_KEYS: Record<string, string> = {
  _base: 'dashboard.settings.accent.base',
  temperature: 'dashboard.settings.accent.temperature',
  controls: 'dashboard.settings.accent.controls',
  labels: 'dashboard.settings.accent.labels',
  borders: 'dashboard.settings.accent.borders',
  indicators: 'dashboard.settings.accent.indicators',
};

/** 리스트 패널 (flows/agents/devices) 악센트 그룹 — 값은 i18n 키 */
const LIST_ACCENT_LABEL_KEYS: Record<string, string> = {
  _base: 'dashboard.settings.accent.base',
  header: 'dashboard.settings.accent.header',
  badges: 'dashboard.settings.accent.badges',
  table: 'dashboard.settings.accent.table',
};

/** 리소스 패널 악센트 그룹 — 값은 i18n 키 */
const RESOURCE_ACCENT_LABEL_KEYS: Record<string, string> = {
  _base: 'dashboard.settings.accent.base',
  header: 'dashboard.settings.accent.header',
  cpu: 'dashboard.settings.accent.cpuCard',
  memory: 'dashboard.settings.accent.memoryCard',
  throughput: 'dashboard.settings.accent.throughputCard',
  errorRate: 'dashboard.settings.accent.errorRateCard',
};

/** 로그 패널 악센트 그룹 — 값은 i18n 키 */
const LOG_ACCENT_LABEL_KEYS: Record<string, string> = {
  _base: 'dashboard.settings.accent.base',
  header: 'dashboard.settings.accent.header',
  levels: 'dashboard.settings.accent.levels',
  timestamp: 'dashboard.settings.accent.timestamp',
  source: 'dashboard.settings.accent.source',
};

/** 게이지 패널 악센트 그룹 — 값은 i18n 키 */
const GAUGE_ACCENT_LABEL_KEYS: Record<string, string> = {
  _base: 'dashboard.settings.accent.base',
  header: 'dashboard.settings.accent.header',
  arc: 'dashboard.settings.accent.arc',
  value: 'dashboard.settings.accent.value',
};

/** 게이지 단위 옵션 — labelKey/unitLabelKey 는 i18n 키. 키가 없으면 value 를 그대로 표시. */
const UNIT_OPTIONS: { labelKey: string; units: { value: string; labelKey?: string }[] }[] = [
  {
    labelKey: 'dashboard.settings.unitGroups.ratio',
    units: [
      { value: '%', labelKey: 'dashboard.settings.units.percent' },
      { value: '‰', labelKey: 'dashboard.settings.units.permille' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.temperature',
    units: [
      { value: '°C', labelKey: 'dashboard.settings.units.celsius' },
      { value: '°F', labelKey: 'dashboard.settings.units.fahrenheit' },
      { value: 'K', labelKey: 'dashboard.settings.units.kelvin' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.electric',
    units: [
      { value: 'V', labelKey: 'dashboard.settings.units.volt' },
      { value: 'A', labelKey: 'dashboard.settings.units.ampere' },
      { value: 'W', labelKey: 'dashboard.settings.units.watt' },
      { value: 'kW', labelKey: 'dashboard.settings.units.kilowatt' },
      { value: 'kWh', labelKey: 'dashboard.settings.units.kilowattHour' },
      { value: 'Ω', labelKey: 'dashboard.settings.units.ohm' },
      { value: 'Hz', labelKey: 'dashboard.settings.units.hertz' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.pressureFlow',
    units: [
      { value: 'Pa', labelKey: 'dashboard.settings.units.pascal' },
      { value: 'kPa' },
      { value: 'bar', labelKey: 'dashboard.settings.units.bar' },
      { value: 'psi' },
      { value: 'L/min', labelKey: 'dashboard.settings.units.litersPerMin' },
      { value: 'm³/h' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.speedRotation',
    units: [
      { value: 'm/s', labelKey: 'dashboard.settings.units.meterPerSec' },
      { value: 'km/h' },
      { value: 'rpm', labelKey: 'dashboard.settings.units.rpm' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.weightVolume',
    units: [
      { value: 'kg', labelKey: 'dashboard.settings.units.kilogram' },
      { value: 'L', labelKey: 'dashboard.settings.units.liter' },
      { value: 'mL', labelKey: 'dashboard.settings.units.milliliter' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.length',
    units: [
      { value: 'mm', labelKey: 'dashboard.settings.units.millimeter' },
      { value: 'cm', labelKey: 'dashboard.settings.units.centimeter' },
      { value: 'm', labelKey: 'dashboard.settings.units.meter' },
    ],
  },
  {
    labelKey: 'dashboard.settings.unitGroups.etc',
    units: [
      { value: 'dB', labelKey: 'dashboard.settings.units.decibel' },
      { value: 'lux', labelKey: 'dashboard.settings.units.lux' },
      { value: 'ppm' },
      { value: '', labelKey: 'dashboard.settings.units.none' },
    ],
  },
];

const GAUGE_TYPE_META: { type: GaugeType; labelKey: string; icon: string }[] = [
  { type: 'simple', labelKey: 'dashboard.settings.gaugeTypes.simple', icon: 'O' },
  { type: 'half', labelKey: 'dashboard.settings.gaugeTypes.half', icon: 'U' },
  { type: 'multi-ring', labelKey: 'dashboard.settings.gaugeTypes.multiRing', icon: '(O)' },
  { type: 'needle', labelKey: 'dashboard.settings.gaugeTypes.needle', icon: '>' },
  { type: 'needle-rainbow', labelKey: 'dashboard.settings.gaugeTypes.needleRainbow', icon: '>>' },
  { type: 'vertical-bar', labelKey: 'dashboard.settings.gaugeTypes.verticalBar', icon: '|' },
  { type: 'half-rainbow', labelKey: 'dashboard.settings.gaugeTypes.halfRainbow', icon: 'U+' },
];

/** 데이터 소스 바인딩 */
interface DataSourceBinding {
  sourceType: 'resource' | 'flow' | 'chart-emitter' | 'store';
  resource?: string;
  flowId?: string;
  dataField?: string;
  /** chart-emitter 소스 전용: 활성 chart 채널 이름 */
  channelName?: string;
  /** chart-emitter / store 소스: 값 추출 경로 (기본 "value", dot-path 지원) */
  displayField?: string;
  /**
   * store 소스 전용: Store 에이전트의 안정적 ID(정본). @spec SPEC-WEB-006
   * Store API 는 이름 주소이지만 이름은 변경될 수 있으므로 불변 ID 를 정본으로
   * 저장하고, 조회 시 이 id 로 현재 이름을 해석해 호출한다. 구 config 하위호환을
   * 위해 옵셔널이며, 부재 시 `storeAgent`(이름)를 그대로 사용한다.
   */
  storeAgentId?: string;
  /**
   * store 소스 전용: Store 에이전트 이름.
   * `storeAgentId` 가 있으면 표시용 스냅샷 + 하위호환 폴백. @spec SPEC-WEB-006
   */
  storeAgent?: string;
  /** store 소스 전용: Store 키 */
  storeKey?: string;
  /** store 소스 전용: Store 네임스페이스 (기본 "default") */
  storeNamespace?: string;
}

/** 연속 컬러 테마 프리셋 — labelKey 는 i18n 키 */
const COLOR_THEME_PRESETS = [
  { id: 'green-red', labelKey: 'dashboard.settings.colorThemes.greenRed', colors: ['#10b981', '#f59e0b', '#ef4444'] },
  { id: 'blue-purple', labelKey: 'dashboard.settings.colorThemes.bluePurple', colors: ['#3b82f6', '#8b5cf6', '#a855f7'] },
  { id: 'cyan-blue', labelKey: 'dashboard.settings.colorThemes.cyanBlue', colors: ['#06b6d4', '#3b82f6', '#1e40af'] },
];

/** 서브 속성용 색상 프리셋 (흰/검 포함) */
const SUB_COLOR_PRESETS = ['#ffffff', '#000000', ...COLOR_PRESETS];

/** 라운드 프리셋 — labelKey 는 i18n 키 */
const RADIUS_PRESETS = [
  { value: '0', labelKey: 'dashboard.settings.radiusPresets.sharp' },
  { value: '4', labelKey: 'dashboard.settings.radiusPresets.small' },
  { value: '8', labelKey: 'dashboard.settings.radiusPresets.medium' },
  { value: '9999', labelKey: 'dashboard.settings.radiusPresets.round' },
];

/** 서브 속성 색상 팔레트 행 */
function SubColorRow({
  label,
  color,
  onColorChange,
}: {
  label: string;
  color: string | undefined;
  onColorChange: (c: string | undefined) => void;
}) {
  const { t } = useTranslation();
  return (
    <div>
      <div className="mb-1 flex items-center gap-1.5">
        <span className="text-[11px] text-(--color-text-muted)">{label}</span>
        {color && (
          <span
            className="h-2.5 w-2.5 rounded-full border border-gray-200 dark:border-gray-600"
            style={{ backgroundColor: color }}
          />
        )}
      </div>
      <div className="space-y-1.5">
        <div className="flex items-center gap-1.5">
          {SUB_COLOR_PRESETS.map((c) => (
            <button
              key={c}
              type="button"
              onClick={() => onColorChange(c)}
              className={cn(
                'relative h-5 w-5 rounded-full transition-transform hover:scale-110',
                c === '#ffffff' && color !== c && 'ring-1 ring-gray-200 dark:ring-gray-600',
              )}
              style={{ backgroundColor: c }}
              aria-label={`${label} ${c}`}
            >
              {color === c && <Check className={cn('absolute inset-0 m-auto h-3 w-3 drop-shadow', c === '#ffffff' ? 'text-gray-700' : 'text-white')} />}
            </button>
          ))}
          <button
            type="button"
            onClick={() => onColorChange(undefined)}
            className={cn(
              'flex h-5 w-5 items-center justify-center rounded-full border-2 transition-transform hover:scale-110',
              !color
                ? 'border-blue-500 bg-(--color-bg-surface) text-(--color-text-secondary)'
                : 'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-muted)',
            )}
            title={t('dashboard.settings.accent.default')}
          >
            <X className="h-2.5 w-2.5" />
          </button>
        </div>
        <div className="flex items-center gap-1.5">
          <label className="relative flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded-md bg-(--color-bg-elevated) transition-colors hover:bg-(--color-border-default)">
            <Pipette className="h-3.5 w-3.5 text-(--color-text-muted)" />
            <input
              type="color"
              value={color ?? '#3b82f6'}
              onChange={(e) => onColorChange(e.target.value)}
              className="absolute inset-0 cursor-pointer opacity-0"
            />
          </label>
          <div className="flex h-6 items-center gap-px rounded-md bg-(--color-bg-elevated) px-1.5 text-[11px] font-mono text-(--color-text-secondary)">
            <span className="text-(--color-text-muted)">#</span>
            <input type="text" value={(color ?? '#3b82f6').replace('#', '').toUpperCase()}
              onChange={(e) => { const v = e.target.value.replace(/[^0-9a-fA-F]/g, '').slice(0, 6); if (v.length === 6) onColorChange(`#${v}`); }}
              className="w-14 bg-transparent text-center outline-none" maxLength={6} />
            <span className="mx-1 h-3 w-px bg-(--color-border-default)" />
            <span className="text-(--color-text-muted)">100%</span>
          </div>
        </div>
      </div>
    </div>
  );
}

/** 범용 악센트 그룹 컨트롤 패널 — 체크박스 + 색상 팔레트 */
function AccentGroupControls({
  selected,
  labelKeys,
  accentElements,
  panelColor,
  onChange,
  onPanelColorChange,
}: {
  selected: string;
  labelKeys: Record<string, string>;
  accentElements: Record<string, string | boolean>;
  panelColor: string | undefined;
  onChange: (elements: Record<string, string | boolean>) => void;
  onPanelColorChange?: (color: string | undefined) => void;
}) {
  const { t } = useTranslation();
  // 선택된 그룹의 표시 라벨 (키 → 번역)
  const selectedLabel = labelKeys[selected] ? t(labelKeys[selected]!) : selected;
  // _base 그룹은 panelColor를 직접 제어
  const isBase = selected === '_base';
  const isEnabled = isBase ? true : accentElements[selected] !== false;
  const gc = isBase ? panelColor : (typeof accentElements[selected] === 'string' ? (accentElements[selected] as string) : undefined);

  // 서브 속성 접근
  const getSubProp = (group: string, prop: string): string | undefined => {
    const val = accentElements[`${group}.${prop}`];
    return typeof val === 'string' ? val : undefined;
  };
  const setSubProp = (group: string, prop: string, value: string | undefined) => {
    const key = `${group}.${prop}`;
    if (value === undefined) {
      const next = { ...accentElements };
      delete next[key];
      onChange(next);
    } else {
      onChange({ ...accentElements, [key]: value });
    }
  };
  const toggle = () => {
    if (isBase) return; // _base는 항상 활성
    onChange({ ...accentElements, [selected]: isEnabled ? false : true });
  };
  const setColor = (color: string | undefined) => {
    if (isBase) {
      onPanelColorChange?.(color);
    } else {
      onChange({ ...accentElements, [selected]: color ?? true });
    }
  };

  const hasSubProps = selected === 'labels';

  return (
    <div className="mt-3 rounded-lg border border-(--color-border-default) bg-(--color-bg-elevated) p-3">
      <div className="mb-2 flex items-center gap-2">
        {!isBase && <input type="checkbox" checked={isEnabled} onChange={toggle} className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500" />}
        <span className="text-sm font-medium text-(--color-text-primary)">{selectedLabel}</span>
        {isEnabled && hasSubProps ? (
          <div className="ml-auto flex gap-1">
            {getSubProp('labels', 'bg') && <span className="h-3 w-3 rounded-sm border border-white/50" style={{ backgroundColor: getSubProp('labels', 'bg') }} title={t('dashboard.settings.accent.background')} />}
            {getSubProp('labels', 'text') && <span className="h-3 w-3 rounded-sm border border-white/50" style={{ backgroundColor: getSubProp('labels', 'text') }} title={t('dashboard.settings.accent.text')} />}
          </div>
        ) : isEnabled && gc ? (
          <span className="ml-auto h-3.5 w-3.5 rounded-full border border-white/50" style={{ backgroundColor: gc }} />
        ) : isEnabled ? (
          <span className="ml-auto text-[10px] text-(--color-text-muted)">{t('dashboard.settings.accent.panelColor')}</span>
        ) : null}
      </div>

      {isEnabled && hasSubProps ? (
        <div className="space-y-3">
          <SubColorRow label={t('dashboard.settings.accent.background')} color={getSubProp('labels', 'bg')} onColorChange={(c) => setSubProp('labels', 'bg', c)} />
          <SubColorRow label={t('dashboard.settings.accent.text')} color={getSubProp('labels', 'text')} onColorChange={(c) => setSubProp('labels', 'text', c)} />
          <div>
            <span className="mb-1 block text-[11px] text-(--color-text-muted)">{t('dashboard.settings.accent.radius')}</span>
            <div className="flex gap-1">
              {RADIUS_PRESETS.map((r) => (
                <button key={r.value} type="button"
                  onClick={() => setSubProp('labels', 'radius', getSubProp('labels', 'radius') === r.value ? undefined : r.value)}
                  className={cn('rounded px-2.5 py-1 text-[11px] font-medium transition-colors',
                    getSubProp('labels', 'radius') === r.value ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                      : 'bg-(--color-bg-surface) text-(--color-text-secondary) hover:bg-(--color-bg-surface)/80')}
                >{t(r.labelKey)}</button>
              ))}
            </div>
          </div>
        </div>
      ) : isEnabled ? (
        <div className="space-y-1.5">
          <div className="flex items-center gap-1.5">
            {COLOR_PRESETS.map((color) => (
              <button key={color} type="button" onClick={() => setColor(color)}
                className="relative h-5 w-5 rounded-full transition-transform hover:scale-110"
                style={{ backgroundColor: color }} aria-label={`${selectedLabel} ${color}`}>
                {gc === color && <Check className="absolute inset-0 m-auto h-3 w-3 text-white drop-shadow" />}
              </button>
            ))}
            <button type="button" onClick={() => setColor(undefined)}
              className={cn('flex h-5 w-5 items-center justify-center rounded-full border-2 transition-transform hover:scale-110',
                !gc ? 'border-blue-500 bg-(--color-bg-surface) text-(--color-text-secondary)' : 'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-muted)')}
              title={t('dashboard.settings.accent.panelColor')}><X className="h-2.5 w-2.5" /></button>
          </div>
          <div className="flex items-center gap-1.5">
            <label className="relative flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded-md bg-(--color-bg-elevated) transition-colors hover:bg-(--color-border-default)">
              <Pipette className="h-3.5 w-3.5 text-(--color-text-muted)" />
              <input type="color" value={gc ?? panelColor ?? '#3b82f6'} onChange={(e) => setColor(e.target.value)} className="absolute inset-0 cursor-pointer opacity-0" />
            </label>
            <div className="flex h-6 items-center gap-px rounded-md bg-(--color-bg-elevated) px-1.5 text-[11px] font-mono text-(--color-text-secondary)">
              <span className="text-(--color-text-muted)">#</span>
              <input type="text" value={(gc ?? panelColor ?? '#3b82f6').replace('#', '').toUpperCase()}
                onChange={(e) => { const v = e.target.value.replace(/[^0-9a-fA-F]/g, '').slice(0, 6); if (v.length === 6) setColor(`#${v}`); }}
                className="w-14 bg-transparent text-center outline-none" maxLength={6} />
              <span className="mx-1 h-3 w-px bg-(--color-border-default)" />
              <span className="text-(--color-text-muted)">100%</span>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

/** 게이지 패널 전용 설정 섹션 */
function GaugeSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const gaugeType = (config.gaugeType as GaugeType) ?? 'simple';
  const min = (config.min as number) ?? 0;
  const max = (config.max as number) ?? 100;
  const unit = (config.unit as string) ?? '%';
  const dataSources = (config.dataSources as DataSourceBinding[]) ?? [{ sourceType: 'resource', resource: 'cpu' }];
  const colorMode = (config.colorMode as 'individual' | 'continuous') ?? 'individual';
  const colorTheme = (config.colorTheme as string) ?? 'green-red';
  const thresholds = (config.thresholds as { name: string; color: string; from: number; to: number }[]) ?? [
    { name: t('dashboard.settings.gaugeSection.thresholdNormal'), color: '#10b981', from: 0, to: 60 },
    { name: t('dashboard.settings.gaugeSection.thresholdCaution'), color: '#f59e0b', from: 60, to: 80 },
    { name: t('dashboard.settings.gaugeSection.thresholdDanger'), color: '#ef4444', from: 80, to: 100 },
  ];

  // 플로우 목록 (데이터 소스 선택용)
  const { data: flowsData } = useFlows();
  const flows = flowsData?.data ?? [];

  // 활성 chart-emitter 채널 목록 (마운트 시 1회 조회)
  const [chartChannels, setChartChannels] = useState<ChartChannelSummary[]>([]);
  useEffect(() => {
    let cancelled = false;
    listChartChannels()
      .then((result) => {
        if (!cancelled) setChartChannels(result);
      })
      .catch(() => {
        // 목록 조회 실패 시 빈 목록으로 유지 (수동 입력 경로는 없음 — dead config 방지)
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // 로컬 드래프트
  const [minDraft, setMinDraft] = useState(String(min));
  const [maxDraft, setMaxDraft] = useState(String(max));
  const [unitDraft, setUnitDraft] = useState(unit);

  useEffect(() => { setMinDraft(String(min)); }, [min]);
  useEffect(() => { setMaxDraft(String(max)); }, [max]);
  useEffect(() => { setUnitDraft(unit); }, [unit]);

  const commitRange = () => {
    const nMin = parseFloat(minDraft);
    const nMax = parseFloat(maxDraft);
    if (!isNaN(nMin) && !isNaN(nMax)) {
      onConfigChange({ min: nMin, max: nMax });
    }
  };

  const updateDataSource = (index: number, patch: Partial<DataSourceBinding>) => {
    const next = dataSources.map((ds, i) => (i === index ? { ...ds, ...patch } : ds));
    onConfigChange({ dataSources: next });
  };

  const addDataSource = () => {
    onConfigChange({ dataSources: [...dataSources, { sourceType: 'resource' as const, resource: 'cpu' }] });
  };

  const removeDataSource = (index: number) => {
    if (dataSources.length <= 1) return;
    onConfigChange({ dataSources: dataSources.filter((_, i) => i !== index) });
  };

  const updateThreshold = (index: number, patch: Partial<{ name: string; color: string; from: number; to: number }>) => {
    const next = thresholds.map((t, i) => (i === index ? { ...t, ...patch } : t));
    onConfigChange({ thresholds: next });
  };

  const addThreshold = () => {
    const lastTo = thresholds.length > 0 ? thresholds[thresholds.length - 1]!.to : 0;
    onConfigChange({
      thresholds: [...thresholds, { name: '', color: '#64748b', from: lastTo, to: max }],
    });
  };

  const removeThreshold = (index: number) => {
    onConfigChange({ thresholds: thresholds.filter((_, i) => i !== index) });
  };

  return (
    <div className="space-y-4">
      {/* A. 게이지 유형 */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.gaugeSection.type')}
        </label>
        <div className="grid grid-cols-4 gap-1">
          {GAUGE_TYPE_META.map(({ type, labelKey }) => (
            <button
              key={type}
              type="button"
              onClick={() => onConfigChange({ gaugeType: type })}
              className={cn(
                'flex flex-col items-center gap-0.5 rounded-md px-1 py-1.5 text-[10px] font-medium transition-colors',
                gaugeType === type
                  ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                  : 'bg-(--color-bg-elevated) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)/80',
              )}
            >
              <GaugeTypeIcon type={type} size={18} active={gaugeType === type} />
              <span className="leading-tight">{t(labelKey)}</span>
            </button>
          ))}
        </div>
      </div>

      {/* B. 값 범위 */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.gaugeSection.valueRange')}
        </label>
        <div className="flex items-center gap-2">
          <input
            type="number"
            value={minDraft}
            onChange={(e) => setMinDraft(e.target.value)}
            onBlur={commitRange}
            onKeyDown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2.5 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
            placeholder={t('dashboard.settings.gaugeSection.min')}
          />
          <span className="shrink-0 text-xs text-(--color-text-muted)">~</span>
          <input
            type="number"
            value={maxDraft}
            onChange={(e) => setMaxDraft(e.target.value)}
            onBlur={commitRange}
            onKeyDown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2.5 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
            placeholder={t('dashboard.settings.gaugeSection.max')}
          />
        </div>
      </div>

      {/* C. 단위 */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.gaugeSection.unit')}
        </label>
        <div className="flex gap-2">
          <select
            value={UNIT_OPTIONS.some((g) => g.units.some((u) => u.value === unitDraft)) ? unitDraft : '__custom__'}
            onChange={(e) => {
              const v = e.target.value;
              if (v === '__custom__') return;
              setUnitDraft(v);
              if (v !== unit) onConfigChange({ unit: v });
            }}
            className="flex-1 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500"
          >
            {UNIT_OPTIONS.map((group) => (
              <optgroup key={group.labelKey} label={t(group.labelKey)}>
                {group.units.map((u) => (
                  <option key={u.value} value={u.value}>{u.labelKey ? t(u.labelKey) : u.value}</option>
                ))}
              </optgroup>
            ))}
            <option value="__custom__">{t('dashboard.settings.gaugeSection.custom')}</option>
          </select>
          <input
            type="text"
            value={unitDraft}
            onChange={(e) => setUnitDraft(e.target.value)}
            onBlur={() => { if (unitDraft !== unit) onConfigChange({ unit: unitDraft }); }}
            onKeyDown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
            className="w-20 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500"
            placeholder={t('dashboard.settings.gaugeSection.customInput')}
          />
        </div>
      </div>

      {/* D. 값 지정 (데이터 소스) */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.gaugeSection.valueBinding')}
        </label>
        <div className="space-y-1.5">
          {dataSources.map((ds, idx) => (
            <div key={idx} className="space-y-1">
              <div className="flex items-center gap-1.5">
                <select
                  value={ds.sourceType}
                  onChange={(e) => {
                    const nextType = e.target.value as DataSourceBinding['sourceType'];
                    updateDataSource(idx, {
                      sourceType: nextType,
                      resource: undefined,
                      flowId: undefined,
                      dataField: undefined,
                      channelName: undefined,
                      displayField: undefined,
                      storeAgent: undefined,
                      storeKey: undefined,
                      storeNamespace: undefined,
                    });
                  }}
                  data-testid={`gauge-source-type-${idx}`}
                  className="w-[72px] shrink-0 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1.5 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
                >
                  <option value="resource">{t('dashboard.settings.gaugeSection.sourceResource')}</option>
                  <option value="flow">{t('dashboard.settings.gaugeSection.sourceFlow')}</option>
                  <option value="chart-emitter">{t('dashboard.settings.gaugeSection.sourceChart')}</option>
                  <option value="store">{t('dashboard.settings.gaugeSection.sourceStore')}</option>
                </select>
                {ds.sourceType === 'resource' && (
                  <select
                    value={ds.resource ?? 'cpu'}
                    onChange={(e) => updateDataSource(idx, { resource: e.target.value })}
                    className="min-w-0 flex-1 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1.5 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
                  >
                    <option value="cpu">{t('dashboard.settings.gaugeSection.resourceCpu')}</option>
                    <option value="memory">{t('dashboard.settings.gaugeSection.resourceMemory')}</option>
                  </select>
                )}
                {ds.sourceType === 'flow' && (
                  <select
                    value={ds.flowId ?? ''}
                    onChange={(e) => updateDataSource(idx, { flowId: e.target.value || undefined })}
                    className="min-w-0 flex-1 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1.5 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
                  >
                    <option value="">{t('dashboard.settings.gaugeSection.selectFlow')}</option>
                    {flows.map((f) => (
                      <option key={f.id} value={f.id}>{f.name || f.id}</option>
                    ))}
                  </select>
                )}
                {ds.sourceType === 'chart-emitter' && (
                  <select
                    value={ds.channelName ?? ''}
                    onChange={(e) => updateDataSource(idx, { channelName: e.target.value || undefined })}
                    data-testid={`gauge-channel-select-${idx}`}
                    className="min-w-0 flex-1 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1.5 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
                  >
                    <option value="">{t('dashboard.settings.gaugeSection.selectChannel')}</option>
                    {chartChannels.map((c) => (
                      <option key={c.name} value={c.name}>
                        {c.name}
                      </option>
                    ))}
                    {/* 현재 저장된 채널이 목록에 없으면 (비활성 등) 선택 상태 유지 */}
                    {ds.channelName && !chartChannels.some((c) => c.name === ds.channelName) && (
                      <option value={ds.channelName}>{t('dashboard.settings.gaugeSection.channelInactive').replace('{name}', ds.channelName)}</option>
                    )}
                  </select>
                )}
                {ds.sourceType === 'store' && (
                  <StoreSourceSelector
                    ds={ds}
                    onChange={(patch) => updateDataSource(idx, patch)}
                  />
                )}
                {gaugeType === 'multi-ring' && dataSources.length > 1 && (
                  <button
                    type="button"
                    onClick={() => removeDataSource(idx)}
                    className="shrink-0 rounded p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-red-500"
                    aria-label={t('dashboard.settings.gaugeSection.deleteSourceAria')}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                )}
              </div>
              {/* chart-emitter / store 선택 시 displayField 입력 */}
              {(ds.sourceType === 'chart-emitter' || ds.sourceType === 'store') && (
                <input
                  type="text"
                  value={ds.displayField ?? ''}
                  onChange={(e) => updateDataSource(idx, { displayField: e.target.value || undefined })}
                  placeholder={t('dashboard.settings.gaugeSection.displayFieldPlaceholder')}
                  data-testid={`gauge-display-field-${idx}`}
                  className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1 text-[11px] text-(--color-text-primary) outline-none focus:border-blue-500"
                />
              )}
            </div>
          ))}
          {gaugeType === 'multi-ring' && (
            <button
              type="button"
              onClick={addDataSource}
              className="flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
            >
              <Plus className="h-3 w-3" />
              {t('dashboard.settings.gaugeSection.addSource')}
            </button>
          )}
        </div>
      </div>

      {/* E. 임계값 및 컬러 설정 */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.gaugeSection.thresholdAndColor')}
        </label>
        {/* 모드 토글 */}
        <div className="mb-2 flex gap-1">
          <button
            type="button"
            onClick={() => onConfigChange({ colorMode: 'individual' })}
            className={cn(
              'flex-1 rounded-md px-2 py-1.5 text-xs font-medium transition-colors',
              colorMode === 'individual'
                ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                : 'bg-(--color-bg-elevated) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)/80',
            )}
          >
            {t('dashboard.settings.gaugeSection.modeIndividual')}
          </button>
          <button
            type="button"
            onClick={() => onConfigChange({ colorMode: 'continuous' })}
            className={cn(
              'flex-1 rounded-md px-2 py-1.5 text-xs font-medium transition-colors',
              colorMode === 'continuous'
                ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                : 'bg-(--color-bg-elevated) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)/80',
            )}
          >
            {t('dashboard.settings.gaugeSection.modeContinuous')}
          </button>
        </div>

        {/*
          임계값 영역(파이) 표시 옵션 — 게이지 내부에 임계값 색상을 부채꼴로 채움.
          기본 동작: thresholds 가 1개 이상이면 ON, 사용자가 명시적으로 해제 가능.
          체크 상태는 다음 규칙으로 계산한다:
            - 명시적으로 false → false (사용자가 해제)
            - 명시적으로 true 또는 미설정+thresholds 존재 → true
        */}
        <label className="mb-2 flex items-center gap-2 text-xs text-(--color-text-secondary)">
          <input
            type="checkbox"
            checked={
              config.showThresholdZones === false
                ? false
                : config.showThresholdZones === true || thresholds.length > 0
            }
            onChange={(e) => onConfigChange({ showThresholdZones: e.target.checked })}
            className="h-4 w-4 rounded border-(--color-border-default) text-blue-600 focus:ring-blue-500"
            data-testid="gauge-show-threshold-zones"
          />
          <span>{t('dashboard.settings.gaugeSection.showThresholdZones')}</span>
        </label>

        {colorMode === 'individual' ? (
          <div className="space-y-1.5">
            {thresholds.map((th, idx) => (
              <div key={idx} className="flex w-full items-center gap-1.5">
                <label className="relative flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded-md transition-colors hover:opacity-80">
                  <span
                    className="h-4 w-4 rounded-sm border border-gray-200 dark:border-gray-600"
                    style={{ backgroundColor: th.color }}
                  />
                  <input
                    type="color"
                    value={th.color}
                    onChange={(e) => updateThreshold(idx, { color: e.target.value })}
                    className="absolute inset-0 cursor-pointer opacity-0"
                  />
                </label>
                <input
                  type="text"
                  value={th.name}
                  onChange={(e) => updateThreshold(idx, { name: e.target.value })}
                  placeholder={t('dashboard.settings.gaugeSection.thresholdNamePlaceholder')}
                  className="min-w-0 flex-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-[11px] text-(--color-text-primary) outline-none focus:border-blue-500"
                />
                <input
                  type="number"
                  value={th.from}
                  onChange={(e) => updateThreshold(idx, { from: parseFloat(e.target.value) || 0 })}
                  className="min-w-0 flex-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-1 text-center text-[11px] text-(--color-text-primary) outline-none focus:border-blue-500"
                />
                <span className="shrink-0 text-[10px] text-(--color-text-muted)">~</span>
                <input
                  type="number"
                  value={th.to}
                  onChange={(e) => updateThreshold(idx, { to: parseFloat(e.target.value) || 0 })}
                  className="min-w-0 flex-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-1 text-center text-[11px] text-(--color-text-primary) outline-none focus:border-blue-500"
                />
                <button
                  type="button"
                  onClick={() => removeThreshold(idx)}
                  className="shrink-0 rounded p-0.5 text-(--color-text-muted) transition-colors hover:text-red-500"
                  aria-label={t('dashboard.settings.gaugeSection.deleteThresholdAria')}
                >
                  <X className="h-3 w-3" />
                </button>
              </div>
            ))}
            <button
              type="button"
              onClick={addThreshold}
              className="flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
            >
              <Plus className="h-3 w-3" />
              {t('dashboard.settings.gaugeSection.addThreshold')}
            </button>
          </div>
        ) : (
          <div className="flex gap-1.5">
            {COLOR_THEME_PRESETS.map((theme) => (
              <button
                key={theme.id}
                type="button"
                onClick={() => onConfigChange({ colorTheme: theme.id })}
                className={cn(
                  'flex flex-1 flex-col items-center gap-1 rounded-md px-2 py-2 transition-colors',
                  colorTheme === theme.id
                    ? 'ring-2 ring-blue-500 ring-inset bg-blue-50/50 dark:bg-blue-900/20'
                    : 'bg-(--color-bg-elevated) hover:bg-(--color-bg-elevated)/80',
                )}
              >
                <div className="flex gap-0.5">
                  {theme.colors.map((c, i) => (
                    <span key={i} className="h-3 w-3 rounded-full" style={{ backgroundColor: c }} />
                  ))}
                </div>
                <span className="text-[9px] text-(--color-text-muted) leading-tight">{t(theme.labelKey)}</span>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

/** 게이지 유형 아이콘 (SVG 미니 아이콘) */
function GaugeTypeIcon({ type, size = 18, active }: { type: GaugeType; size?: number; active?: boolean }) {
  const color = active ? '#2563eb' : '#94a3b8';
  const s = size;
  const c = s / 2;
  const r = s / 2 - 2;

  switch (type) {
    case 'simple':
      return (
        <svg width={s} height={s} viewBox={`0 0 ${s} ${s}`}>
          <circle cx={c} cy={c} r={r} fill="none" stroke={color} strokeWidth={2.5} strokeDasharray={`${r * Math.PI * 1.5} ${r * Math.PI * 2}`} strokeLinecap="round" />
        </svg>
      );
    case 'half':
      return (
        <svg width={s} height={s * 0.65} viewBox={`0 0 ${s} ${s * 0.65}`}>
          <path d={`M 2 ${s * 0.6} A ${r} ${r} 0 0 1 ${s - 2} ${s * 0.6}`} fill="none" stroke={color} strokeWidth={2.5} strokeLinecap="round" />
        </svg>
      );
    case 'multi-ring':
      return (
        <svg width={s} height={s} viewBox={`0 0 ${s} ${s}`}>
          <circle cx={c} cy={c} r={r} fill="none" stroke={color} strokeWidth={1.5} strokeDasharray={`${r * Math.PI * 1.2} 100`} opacity={0.4} />
          <circle cx={c} cy={c} r={r - 3} fill="none" stroke={color} strokeWidth={1.5} strokeDasharray={`${(r - 3) * Math.PI * 1.5} 100`} />
        </svg>
      );
    case 'needle':
      return (
        <svg width={s} height={s} viewBox={`0 0 ${s} ${s}`}>
          <circle cx={c} cy={c} r={r} fill="none" stroke={color} strokeWidth={1.5} opacity={0.3} />
          <line x1={c} y1={c} x2={c + r * 0.7} y2={c - r * 0.3} stroke={color} strokeWidth={1.5} strokeLinecap="round" />
          <circle cx={c} cy={c} r={1.5} fill={color} />
        </svg>
      );
    case 'needle-rainbow':
      return (
        <svg width={s} height={s} viewBox={`0 0 ${s} ${s}`}>
          <path d={`M ${c - r} ${c} A ${r} ${r} 0 0 1 ${c + r} ${c}`} fill="none" stroke="#10b981" strokeWidth={2} />
          <path d={`M ${c - r * 0.7} ${c - r * 0.7} A ${r} ${r} 0 0 1 ${c + r} ${c}`} fill="none" stroke="#f59e0b" strokeWidth={2} />
          <line x1={c} y1={c} x2={c + r * 0.5} y2={c - r * 0.5} stroke={color} strokeWidth={1.5} strokeLinecap="round" />
          <circle cx={c} cy={c} r={1.5} fill={color} />
        </svg>
      );
    case 'vertical-bar':
      return (
        <svg width={s} height={s} viewBox={`0 0 ${s} ${s}`}>
          <rect x={s * 0.3} y={2} width={s * 0.4} height={s - 4} rx={2} fill="none" stroke={color} strokeWidth={1.5} opacity={0.3} />
          <rect x={s * 0.3} y={s * 0.4} width={s * 0.4} height={s * 0.6 - 2} rx={2} fill={color} opacity={0.7} />
        </svg>
      );
    case 'half-rainbow':
      return (
        <svg width={s} height={s * 0.65} viewBox={`0 0 ${s} ${s * 0.65}`}>
          <path d={`M 2 ${s * 0.6} A ${r} ${r} 0 0 1 ${s * 0.4} ${s * 0.1}`} fill="none" stroke="#10b981" strokeWidth={2.5} strokeLinecap="round" />
          <path d={`M ${s * 0.4} ${s * 0.1} A ${r} ${r} 0 0 1 ${s - 2} ${s * 0.6}`} fill="none" stroke="#ef4444" strokeWidth={2.5} strokeLinecap="round" />
        </svg>
      );
    default:
      return <Gauge className="text-current" style={{ width: s, height: s }} />;
  }
}

/** 게이지 미니 프리뷰 (우측 컬럼) */
/** Store 데이터 소스 선택 (에이전트 → 키) */
function StoreSourceSelector({
  ds,
  onChange,
}: {
  ds: DataSourceBinding;
  onChange: (patch: Partial<DataSourceBinding>) => void;
}) {
  const { t } = useTranslation();
  const { data: agentsResult } = useAgents();
  const storeAgents = useMemo(
    () => (agentsResult?.data ?? []).filter((a) => a.type === 'store'),
    [agentsResult],
  );

  // SPEC-WEB-006: 셀렉트는 agent id 기준. 저장된 storeAgentId 로 현재 에이전트를
  // 찾고(구 config 는 이름으로 매칭 시도), 키 조회는 해석된 현재 이름으로 한다.
  const selectedAgent = useMemo(
    () =>
      ds.storeAgentId
        ? storeAgents.find((a) => a.id === ds.storeAgentId)
        : storeAgents.find((a) => a.name === ds.storeAgent),
    [storeAgents, ds.storeAgentId, ds.storeAgent],
  );
  const selectValue = selectedAgent?.id ?? ds.storeAgentId ?? '';
  const resolvedAgentName = resolveStoreAgentName(
    ds.storeAgentId,
    ds.storeAgent ?? '',
    agentsResult?.data,
  );

  // 선택된 에이전트의 키 목록
  const [keys, setKeys] = useState<string[]>([]);
  const [keysLoading, setKeysLoading] = useState(false);

  useEffect(() => {
    if (!resolvedAgentName) {
      setKeys([]);
      return;
    }
    let cancelled = false;
    setKeysLoading(true);
    listStoreKeys(resolvedAgentName, ds.storeNamespace ?? 'default')
      .then((result) => {
        if (!cancelled) setKeys(result);
      })
      .catch(() => {
        if (!cancelled) setKeys([]);
      })
      .finally(() => {
        if (!cancelled) setKeysLoading(false);
      });
    return () => { cancelled = true; };
  }, [resolvedAgentName, ds.storeNamespace]);

  return (
    <div className="flex min-w-0 flex-1 flex-col gap-1">
      <div className="flex gap-1">
        <select
          value={selectValue}
          onChange={(e) => {
            const id = e.target.value;
            if (!id) {
              onChange({ storeAgentId: undefined, storeAgent: undefined, storeKey: undefined });
              return;
            }
            const agent = storeAgents.find((a) => a.id === id);
            // storeAgentId(정본) + storeAgent(현재 이름 스냅샷) 저장, 키 초기화.
            onChange({ storeAgentId: id, storeAgent: agent?.name, storeKey: undefined });
          }}
          className="min-w-0 flex-1 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1.5 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
        >
          <option value="">{t('dashboard.settings.gaugeSection.selectStore')}</option>
          {storeAgents.map((a: { id: string; name: string }) => (
            <option key={a.id} value={a.id}>{a.name}</option>
          ))}
          {/* 저장된 에이전트가 목록에 없으면(비활성/삭제) 저장된 이름으로 선택 유지 */}
          {selectValue && !selectedAgent && (
            <option value={selectValue}>{ds.storeAgent || selectValue}</option>
          )}
        </select>
        <select
          value={ds.storeKey ?? ''}
          onChange={(e) => onChange({ storeKey: e.target.value || undefined })}
          disabled={keysLoading || !selectValue}
          className="min-w-0 flex-1 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1.5 text-xs text-(--color-text-primary) outline-none focus:border-blue-500 disabled:opacity-60"
        >
          <option value="">
            {keysLoading
              ? t('dashboard.settings.gaugeSection.keyLoading')
              : keys.length === 0
                ? t('dashboard.settings.gaugeSection.keyEmpty')
                : t('dashboard.settings.gaugeSection.selectKey')}
          </option>
          {keys.map((k) => (
            <option key={k} value={k}>{k}</option>
          ))}
          {ds.storeKey && !keys.includes(ds.storeKey) && (
            <option value={ds.storeKey}>{t('dashboard.settings.gaugeSection.keyCurrent').replace('{key}', ds.storeKey)}</option>
          )}
        </select>
      </div>
    </div>
  );
}

function GaugeMiniPreview({ panel }: { panel: PanelConfig }) {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const min = (config.min as number) ?? 0;
  const max = (config.max as number) ?? 100;
  const range = max - min;
  const sampleValue = Math.round(min + range * 0.65);
  const previewConfig = { ...config, value: sampleValue };

  return (
    <div
      className="flex h-full w-full flex-col rounded-xl border border-(--color-border-default) bg-(--color-bg-elevated) p-3"
      style={{ overflow: 'hidden' }}
    >
      <span className="mb-1 text-center text-[10px] font-medium text-(--color-text-muted)">
        {t('dashboard.settings.gaugeSection.previewSample').replace('{value}', String(sampleValue))}
      </span>
      <div className="min-h-0 flex-1">
        <GaugePanel
          panelId="__preview__"
          title=""
          config={previewConfig}
          onConfigChange={() => {}}
          onTitleChange={() => {}}
        />
      </div>
    </div>
  );
}

/**
 * Line-chart 미니 프리뷰. 실제 chart-emitter 채널 구독 없이
 * 사인파 기반 샘플 데이터를 합성하여 사용자 config (channels/color/
 * thresholds/smooth/y_axis_mode 등) 변경을 즉시 시각화한다.
 */
const PREVIEW_FALLBACK_PALETTE = [
  '#3b82f6',
  '#10b981',
  '#f59e0b',
  '#ef4444',
  '#8b5cf6',
  '#06b6d4',
];

function LineChartMiniPreview({ panel }: { panel: PanelConfig }) {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const rawChannels = config.channels as ChannelRefConfig[] | undefined;
  const channels = useMemo(() => rawChannels ?? [], [rawChannels]);
  const isMultiMode = channels.length > 0;
  const globalSmooth = (config.smooth as boolean | undefined) ?? false;
  const yAxisMode = (config.y_axis_mode as YAxisMode | undefined) ?? 'auto';
  const yMin = config.y_min as number | undefined;
  const yMax = config.y_max as number | undefined;
  const xLabel = (config.x_label as string | undefined) ?? '';
  const yLabel = (config.y_label as string | undefined) ?? '';
  const yUnit = (config.y_unit as string | undefined) ?? '';
  const xTickFont = resolveAxisFont(config.x_tick_font as AxisFontStyle | undefined);
  const xLabelFont = resolveAxisFont(config.x_label_font as AxisFontStyle | undefined);
  const yTickFont = resolveAxisFont(config.y_tick_font as AxisFontStyle | undefined);
  const yLabelFont = resolveAxisFont(config.y_label_font as AxisFontStyle | undefined);
  const yAxisType = (config.y_axis_type as YAxisDataType | undefined) ?? 'numeric';
  const rawEnumLabels = config.y_enum_labels as YEnumLabel[] | undefined;
  const enumMap = useMemo(() => buildEnumLabelMap(rawEnumLabels), [rawEnumLabels]);
  const enumMode = yAxisType === 'enum' && enumMap.size > 0;
  const enumTicks = useMemo(
    () => (enumMode ? [...enumMap.keys()].sort((a, b) => a - b) : undefined),
    [enumMode, enumMap],
  );
  const rawThresholds = config.y_thresholds as YThreshold[] | undefined;
  const thresholds = useMemo(() => rawThresholds ?? [], [rawThresholds]);
  const channelName = (config.channel_name as string | undefined) ?? '';
  const legendCfg = (config.legend as Record<string, unknown> | undefined) ?? {};
  const legendPos = (legendCfg.position as string | undefined) ?? 'bottom';

  const series = useMemo(() => {
    if (isMultiMode) {
      return channels.map((c, i) => ({
        key: c.alias ?? (c.name || t('dashboard.settings.preview.channelFallback').replace('{index}', String(i + 1))),
        color: c.color ?? PREVIEW_FALLBACK_PALETTE[i % PREVIEW_FALLBACK_PALETTE.length]!,
        smooth: c.smooth ?? globalSmooth,
        strokeWidth: c.stroke_width ?? 2,
        strokeDasharray: c.stroke_style ? STROKE_DASHARRAY[c.stroke_style] : '',
      }));
    }
    return [{
      key: channelName || t('dashboard.settings.preview.sample'),
      color: PREVIEW_FALLBACK_PALETTE[0]!,
      smooth: globalSmooth,
      strokeWidth: 2,
      strokeDasharray: '',
    }];
  }, [isMultiMode, channels, channelName, globalSmooth, t]);

  const data = useMemo(() => {
    const points = 30;
    const rows: Array<Record<string, number>> = [];
    // 열거형 미리보기: 각 시리즈가 매핑된 값들을 계단식으로 순회하도록 합성한다.
    if (enumMode && enumTicks && enumTicks.length > 0) {
      for (let i = 0; i < points; i++) {
        const row: Record<string, number> = { t: i };
        series.forEach((s, idx) => {
          const step = Math.floor(i / Math.max(1, Math.floor(points / enumTicks.length)));
          row[s.key] = enumTicks[(step + idx) % enumTicks.length]!;
        });
        rows.push(row);
      }
      return rows;
    }
    for (let i = 0; i < points; i++) {
      const row: Record<string, number> = { t: i };
      series.forEach((s, idx) => {
        const phase = (idx * Math.PI) / 3;
        row[s.key] = 50 + 30 * Math.sin((i / points) * Math.PI * 2 + phase);
      });
      rows.push(row);
    }
    return rows;
  }, [series, enumMode, enumTicks]);

  const yDomain = useMemo<[number | 'auto', number | 'auto']>(() => {
    if (enumMode && enumTicks && enumTicks.length > 0) {
      return [enumTicks[0]! - 0.5, enumTicks[enumTicks.length - 1]! + 0.5];
    }
    if (yAxisMode === 'manual') return [yMin ?? 'auto', yMax ?? 'auto'];
    return [0, 100];
  }, [enumMode, enumTicks, yAxisMode, yMin, yMax]);

  const yAxisLabel = yLabel || yUnit
    ? { value: [yLabel, yUnit].filter(Boolean).join(' '), angle: -90, position: 'insideLeft' as const, style: { fontSize: yLabelFont.fontSize, fill: yLabelFont.fill, fontWeight: yLabelFont.fontWeight } }
    : undefined;

  return (
    <div
      className="flex h-full w-full flex-col overflow-hidden rounded-xl border border-(--color-border-default) bg-(--color-bg-elevated) p-3"
    >
      <div className="mb-1 flex items-center justify-between">
        <span className="text-[10px] font-medium text-(--color-text-muted)">
          {t('dashboard.settings.preview.label')}
        </span>
        <span className="text-[10px] text-(--color-text-muted)">
          {isMultiMode ? t('dashboard.settings.preview.channelCount').replace('{count}', String(channels.length)) : channelName || t('dashboard.settings.preview.channelUnset')}
        </span>
      </div>
      <div className="min-h-0 flex-1">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={data} margin={{ top: 8, right: 16, left: yAxisLabel ? 16 : 0, bottom: xLabel ? 20 : 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
            <XAxis
              dataKey="t"
              tick={{ fontSize: xTickFont.fontSize, fill: xTickFont.fill, fontWeight: xTickFont.fontWeight }}
              stroke="#9ca3af"
              height={xLabel ? 40 : undefined}
              label={xLabel ? { value: xLabel, position: 'insideBottom', offset: 6, style: { textAnchor: 'middle', fontSize: xLabelFont.fontSize, fill: xLabelFont.fill, fontWeight: xLabelFont.fontWeight } } : undefined}
            />
            <YAxis
              domain={yDomain}
              tick={{ fontSize: yTickFont.fontSize, fill: yTickFont.fill, fontWeight: yTickFont.fontWeight }}
              stroke="#9ca3af"
              width={yAxisLabel ? 48 : 36}
              label={yAxisLabel}
              ticks={enumMode ? enumTicks : undefined}
              tickFormatter={
                enumMode
                  ? (v: number) => formatEnumValue(v, enumMap)
                  : yUnit
                    ? (v: number) => `${v}${yUnit}`
                    : undefined
              }
            />
            <Tooltip
              contentStyle={{ fontSize: '0.7rem' }}
              formatter={
                enumMode
                  ? (value, name) => [
                      typeof value === 'number' ? formatEnumValue(value, enumMap) : value,
                      name,
                    ]
                  : undefined
              }
            />
            {series.length > 1 && (
              <Legend
                wrapperStyle={{ fontSize: '0.7rem' }}
                verticalAlign={legendPos === 'left' || legendPos === 'right' ? 'middle' : 'bottom'}
                align={legendPos === 'left' ? 'left' : legendPos === 'right' ? 'right' : 'center'}
                layout={legendPos === 'left' || legendPos === 'right' ? 'vertical' : 'horizontal'}
              />
            )}
            {thresholds.map((t, i) => {
              const color = t.color ?? THRESHOLD_DEFAULT_COLORS[t.severity ?? 'info'];
              return (
                <ReferenceLine key={`th-${i}`} y={t.value} stroke={color} strokeDasharray="4 2" />
              );
            })}
            {thresholds
              .filter((t) => t.fill_direction)
              .map((t, i) => (
                <ReferenceArea
                  key={`fill-${i}`}
                  y1={t.fill_direction === 'below' ? -1e9 : t.value}
                  y2={t.fill_direction === 'below' ? t.value : 1e9}
                  fill={t.color}
                  fillOpacity={0.1}
                  strokeOpacity={0}
                />
              ))}
            {series.map((s) => (
              <Line
                key={s.key}
                type={s.smooth ? 'monotone' : 'linear'}
                dataKey={s.key}
                stroke={s.color}
                strokeWidth={s.strokeWidth}
                strokeDasharray={s.strokeDasharray || undefined}
                dot={false}
                isAnimationActive={false}
              />
            ))}
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

/** Samsung HVACR-01 리모컨 미니 프리뷰 - 클릭으로 악센트 그룹 선택 */
function NasaMiniPreview({
  selectedGroup,
  onSelectGroup,
  effectiveColor,
  getSubProp,
  panelColor,
}: {
  selectedGroup: string | null;
  onSelectGroup: (group: string) => void;
  effectiveColor: (group: string) => string | undefined;
  getSubProp: (group: string, prop: string) => string | undefined;
  panelColor: string | undefined;
}) {
  const { t } = useTranslation();
  const zoneClass = (group: string, extra?: string) =>
    cn(
      'cursor-pointer transition-all relative',
      selectedGroup === group
        ? 'ring-2 ring-inset ring-blue-500/70 bg-blue-50/30 dark:bg-blue-900/15'
        : 'hover:bg-blue-50/20 dark:hover:bg-blue-900/10',
      extra,
    );

  const tag = (group: string, label: string) =>
    selectedGroup === group ? (
      <span className="pointer-events-none absolute right-1 top-0.5 rounded bg-blue-500 px-1 py-px text-[8px] font-medium leading-tight text-white">
        {label}
      </span>
    ) : null;

  const divider = (
    <div
      className={cn(
        'mx-3 cursor-pointer border-t transition-all',
        selectedGroup === 'borders' ? 'border-blue-500 border-t-2' : 'border-gray-100 dark:border-gray-700',
      )}
      style={selectedGroup !== 'borders' && effectiveColor('borders') ? { borderColor: `${effectiveColor('borders')}30` } : undefined}
      onClick={() => onSelectGroup('borders')}
    />
  );

  const labelBg = getSubProp('labels', 'bg');
  const labelText = getSubProp('labels', 'text') ?? effectiveColor('labels');
  const labelRadius = getSubProp('labels', 'radius');

  return (
    <div
      className={cn(
        'h-full w-full overflow-hidden rounded-2xl text-xs',
        selectedGroup === 'borders' ? 'ring-2 ring-blue-500' : 'ring-1 ring-(--color-border-default)',
      )}
      style={selectedGroup !== 'borders' && effectiveColor('borders') ? { boxShadow: `inset 0 0 0 1px ${effectiveColor('borders')}40` } : undefined}
    >
      {/* 전체 색상 - _base */}
      <div
        className={zoneClass('_base', 'flex items-center gap-1.5 px-3 py-1')}
        onClick={() => onSelectGroup('_base')}
      >
        {tag('_base', t('dashboard.settings.preview.tagBase'))}
        <span
          className="h-2.5 w-2.5 rounded-full border border-gray-300 dark:border-gray-600"
          style={panelColor ? { backgroundColor: panelColor } : { background: 'conic-gradient(from 0deg, #ef4444, #f59e0b, #10b981, #3b82f6, #8b5cf6, #ef4444)' }}
        />
        <span className="text-[10px] text-(--color-text-muted)">{panelColor ?? t('dashboard.settings.accent.default')}</span>
      </div>

      {/* 헤더: 아이콘+타이틀 | 상태뱃지+전원 — indicators */}
      <div className={zoneClass('indicators')} onClick={() => onSelectGroup('indicators')}>
        {tag('indicators', t('dashboard.settings.preview.tagIndicators'))}
        <div className="flex items-center justify-between px-3 py-1.5">
          <div className="flex items-center gap-1.5">
            <Snowflake
              className="h-3.5 w-3.5 text-blue-500"
              style={effectiveColor('indicators') ? { color: effectiveColor('indicators')! } : undefined}
            />
            <span className="text-[11px] font-bold text-(--color-text-primary)">{t('dashboard.settings.preview.livingRoom')}</span>
          </div>
          <div className="flex items-center gap-1.5">
            <span
              className="inline-flex items-center gap-1 rounded-full bg-blue-50 px-1.5 py-0.5 text-[9px] font-medium text-blue-500 dark:bg-blue-900/30 dark:text-blue-400"
              style={effectiveColor('indicators') ? { color: effectiveColor('indicators')!, backgroundColor: `${effectiveColor('indicators')}15` } : undefined}
            >
              <span className="h-1 w-1 rounded-full bg-blue-500" style={effectiveColor('indicators') ? { backgroundColor: effectiveColor('indicators')! } : undefined} />
              {t('dashboard.settings.preview.operating')}
            </span>
            <span
              className="flex h-5 w-5 items-center justify-center rounded-md bg-blue-500 text-white"
              style={effectiveColor('indicators') ? { backgroundColor: effectiveColor('indicators')! } : undefined}
            >
              <Power className="h-3 w-3" />
            </span>
          </div>
        </div>
      </div>

      {/* 현재 온도 — temperature */}
      <div className={zoneClass('temperature')} onClick={() => onSelectGroup('temperature')}>
        {tag('temperature', t('dashboard.settings.preview.tagTemperature'))}
        <div className="flex flex-col items-center py-2">
          <div className="flex items-end">
            <span
              className="text-3xl font-light text-blue-600"
              style={effectiveColor('temperature') ? { color: effectiveColor('temperature')! } : undefined}
            >23.7</span>
            <span
              className="text-sm text-blue-600"
              style={effectiveColor('temperature') ? { color: effectiveColor('temperature')! } : undefined}
            >°C</span>
          </div>
          <span
            className="text-[9px] text-blue-300"
            style={effectiveColor('temperature') ? { color: `${effectiveColor('temperature')}60` } : undefined}
          >{t('dashboard.settings.preview.currentTemp')}</span>
        </div>
        {/* 설정 온도 */}
        <div className="flex items-center justify-center gap-1.5 pb-2">
          <Thermometer className="h-3 w-3 text-(--color-text-muted)" style={effectiveColor('temperature') ? { color: effectiveColor('temperature')! } : undefined} />
          <span className="flex h-4.5 w-4.5 items-center justify-center rounded bg-(--color-bg-elevated)"><Minus className="h-2.5 w-2.5 text-(--color-text-muted)" /></span>
          <span className="text-[10px] font-semibold text-(--color-text-primary)">{t('dashboard.settings.preview.setLabel')}</span>
          <span className="flex h-4.5 w-4.5 items-center justify-center rounded bg-(--color-bg-elevated)"><Plus className="h-2.5 w-2.5 text-(--color-text-muted)" /></span>
        </div>
      </div>

      {divider}

      {/* 모드 선택 (5버튼) — labels */}
      <div className={zoneClass('labels')} onClick={() => onSelectGroup('labels')}>
        {tag('labels', t('dashboard.settings.preview.tagModeLabel'))}
        <div className="flex gap-1 px-3 py-2">
          {[
            { label: t('dashboard.settings.preview.modeCooling'), icon: <Snowflake className="h-3 w-3" />, active: true },
            { label: t('dashboard.settings.preview.modeHeating'), active: false },
            { label: t('dashboard.settings.preview.modeAuto'), active: false },
            { label: t('dashboard.settings.preview.modeDehumidify'), active: false },
            { label: t('dashboard.settings.preview.modeFan'), active: false },
          ].map(({ label, icon, active }) => (
            <span
              key={label}
              className={cn(
                'flex flex-1 flex-col items-center justify-center gap-0.5 py-1 text-[8px] font-medium',
                labelRadius == null && 'rounded-lg',
                active && !labelBg && !labelText
                  ? 'bg-blue-600 text-white'
                  : active ? 'text-white' : 'text-(--color-text-muted) ring-1 ring-(--color-border-default)',
              )}
              style={{
                ...(active && labelBg ? { backgroundColor: labelBg } : {}),
                ...(active && labelText ? { color: labelText } : {}),
                ...(labelRadius != null ? { borderRadius: `${labelRadius}px` } : {}),
              }}
            >
              {icon}
              {label}
            </span>
          ))}
        </div>
      </div>

      {/* 풍량 — controls */}
      <div className={zoneClass('controls')} onClick={() => onSelectGroup('controls')}>
        {tag('controls', t('dashboard.settings.preview.tagControls'))}
        <div className="flex items-center gap-1.5 px-3 py-1.5">
          <Fan
            className="h-3 w-3 shrink-0 text-blue-600"
            style={effectiveColor('controls') ? { color: effectiveColor('controls')! } : undefined}
          />
          <span
            className="text-[10px] font-semibold text-blue-600"
            style={effectiveColor('controls') ? { color: effectiveColor('controls')! } : undefined}
          >{t('dashboard.settings.preview.airflow')}</span>
          {[t('dashboard.settings.preview.fanAuto'), t('dashboard.settings.preview.fanLow'), t('dashboard.settings.preview.fanMid'), t('dashboard.settings.preview.fanHigh')].map((s, i) => (
            <span
              key={s}
              className={cn(
                'flex-1 rounded-md py-0.5 text-center text-[9px] font-medium',
                i === 0
                  ? 'bg-blue-50 text-blue-600 ring-1 ring-blue-500 dark:bg-blue-900/30'
                  : 'text-(--color-text-muted) ring-1 ring-(--color-border-default)',
              )}
              style={i === 0 && effectiveColor('controls') ? { color: effectiveColor('controls')!, borderColor: effectiveColor('controls')!, backgroundColor: `${effectiveColor('controls')}10` } : undefined}
            >
              {s}
            </span>
          ))}
        </div>
      </div>

      {divider}

      {/* 하단: 스윙 + 필터 — indicators */}
      <div className={zoneClass('indicators', 'rounded-b-2xl')} onClick={() => onSelectGroup('indicators')}>
        <div className="flex items-center gap-3 px-3 py-1.5">
          <span
            className="flex items-center gap-0.5 text-[10px] text-(--color-text-muted)"
            style={effectiveColor('indicators') ? { color: effectiveColor('indicators')! } : undefined}
          >
            <ArrowUpDown className="h-3 w-3" />
            {t('dashboard.settings.preview.swingOn')}
          </span>
          <span className="flex items-center gap-0.5 text-[10px] text-amber-500">
            {t('dashboard.settings.preview.filterNormal')}
          </span>
        </div>
      </div>
    </div>
  );
}

/** 리스트 패널 미니 프리뷰 (flows / agents / devices) */
function ListMiniPreview({
  selectedGroup,
  onSelectGroup,
  effectiveColor,
  panelColor,
  variant,
}: {
  selectedGroup: string | null;
  onSelectGroup: (g: string) => void;
  effectiveColor: (g: string) => string | undefined;
  panelColor: string | undefined;
  variant: 'flows' | 'agents' | 'devices';
}) {
  const { t } = useTranslation();
  const zone = (group: string, extra?: string) =>
    cn('cursor-pointer transition-all relative',
      selectedGroup === group ? 'ring-2 ring-inset ring-blue-500/70 bg-blue-50/30 dark:bg-blue-900/15' : 'hover:bg-blue-50/20 dark:hover:bg-blue-900/10', extra);
  const tag = (group: string, label: string) =>
    selectedGroup === group ? <span className="pointer-events-none absolute right-1 top-0.5 rounded bg-blue-500 px-1 py-px text-[8px] font-medium leading-tight text-white">{label}</span> : null;

  const titles = {
    flows: t('dashboard.settings.preview.flowsTitle'),
    agents: t('dashboard.settings.preview.agentsTitle'),
    devices: t('dashboard.settings.preview.devicesTitle'),
  };
  const badgeLabels = variant === 'flows'
    ? [{ l: t('dashboard.settings.preview.flowsRunning'), c: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' }, { l: t('dashboard.settings.preview.flowsStopped'), c: 'bg-gray-100 text-gray-500 dark:bg-gray-700/30 dark:text-gray-400' }]
    : variant === 'agents'
    ? [{ l: t('dashboard.settings.preview.agentsTotal'), c: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400' }, { l: t('dashboard.settings.preview.agentsActive'), c: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' }]
    : [{ l: t('dashboard.settings.preview.devicesTotal'), c: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400' }, { l: t('dashboard.settings.preview.devicesOnline'), c: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400' }];

  return (
    <div className="h-full w-full overflow-hidden rounded-xl border border-(--color-border-default) text-xs">
      {/* 전체 색상 - _base */}
      <div className={zone('_base', 'flex items-center gap-1.5 rounded-t-xl px-3 py-1.5')} onClick={() => onSelectGroup('_base')}>
        {tag('_base', t('dashboard.settings.preview.tagBase'))}
        <span className="h-2.5 w-2.5 rounded-full border border-gray-300 dark:border-gray-600"
          style={panelColor ? { backgroundColor: panelColor } : { background: 'conic-gradient(from 0deg, #ef4444, #f59e0b, #10b981, #3b82f6, #8b5cf6, #ef4444)' }} />
        <span className="text-[10px] text-(--color-text-muted)">{panelColor ?? t('dashboard.settings.accent.default')}</span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 타이틀 */}
      <div className={zone('header', 'px-3 py-2')} onClick={() => onSelectGroup('header')}>
        {tag('header', t('dashboard.settings.preview.tagHeader'))}
        <span className="text-sm font-semibold text-(--color-text-primary)" style={effectiveColor('header') ? { color: effectiveColor('header')! } : undefined}>
          {titles[variant]}
        </span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 요약 배지 */}
      <div className={zone('badges', 'flex gap-1.5 px-3 py-2')} onClick={() => onSelectGroup('badges')}>
        {tag('badges', t('dashboard.settings.preview.tagBadges'))}
        {badgeLabels.map((b) => (
          <span key={b.l} className={cn('rounded-full px-1.5 py-0.5 text-[10px] font-medium', b.c)}
            style={effectiveColor('badges') ? { backgroundColor: `${effectiveColor('badges')}20`, color: effectiveColor('badges')! } : undefined}>
            {b.l}
          </span>
        ))}
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 테이블 헤더 */}
      <div className={zone('table', 'px-3 py-2')} onClick={() => onSelectGroup('table')}>
        {tag('table', t('dashboard.settings.preview.tagTableHeader'))}
        <div className="flex gap-4 text-[10px] font-medium uppercase tracking-wider text-(--color-text-muted)"
          style={effectiveColor('table') ? { color: effectiveColor('table')! } : undefined}>
          <span className="flex-1">{t('dashboard.settings.preview.colName')}</span><span>{t('dashboard.settings.preview.colStatus')}</span><span>{t('dashboard.settings.preview.colUpdated')}</span>
        </div>
      </div>
      {/* 더미 행 */}
      <div className="border-t border-gray-100 px-3 py-1.5 dark:border-gray-700">
        <div className="flex gap-4 text-[10px] text-(--color-text-muted)">
          <span className="flex-1 text-blue-500">sample-1</span><span>●</span><span>{t('dashboard.settings.preview.ago2min')}</span>
        </div>
      </div>
      <div className="border-t border-gray-100 px-3 py-1.5 dark:border-gray-700">
        <div className="flex gap-4 text-[10px] text-(--color-text-muted)">
          <span className="flex-1 text-blue-500">sample-2</span><span>○</span><span>{t('dashboard.settings.preview.ago5min')}</span>
        </div>
      </div>
    </div>
  );
}

/** 리소스 패널 미니 프리뷰 */
/** 속성 그리드 미니 프리뷰 */
function GridMiniPreview({
  selectedGroup,
  onSelectGroup,
  effectiveColor,
  panelColor,
  gridCols,
}: {
  selectedGroup: string | null;
  onSelectGroup: (g: string) => void;
  effectiveColor: (g: string) => string | undefined;
  panelColor: string | undefined;
  gridCols: number;
}) {
  const { t } = useTranslation();
  const zone = (group: string, extra?: string) =>
    cn('cursor-pointer transition-all relative',
      selectedGroup === group ? 'ring-2 ring-inset ring-blue-500/70 bg-blue-50/30 dark:bg-blue-900/15' : 'hover:bg-blue-50/20 dark:hover:bg-blue-900/10', extra);
  const tag = (group: string, label: string) =>
    selectedGroup === group ? <span className="pointer-events-none absolute right-1 top-0.5 rounded bg-blue-500 px-1 py-px text-[8px] font-medium leading-tight text-white">{label}</span> : null;

  const items = [
    { key: 'power', label: t('dashboard.settings.preview.propPower'), value: 'ON' },
    { key: 'mode', label: t('dashboard.settings.preview.propMode'), value: 'cooling' },
    { key: 'target_temperature', label: t('dashboard.settings.preview.propTargetTemp'), value: '24°C' },
    { key: 'current_temperature', label: t('dashboard.settings.preview.propCurrentTemp'), value: '25.5°C' },
    { key: 'fan_speed', label: t('dashboard.settings.preview.propFanSpeed'), value: 'auto' },
    { key: 'valve_open', label: t('dashboard.settings.preview.propValve'), value: 'ON' },
  ];

  const cols = Math.min(gridCols, 3);
  const colClass = cols === 1 ? 'grid-cols-1' : cols === 2 ? 'grid-cols-2' : 'grid-cols-3';

  return (
    <div className="h-full w-full overflow-hidden rounded-xl border border-(--color-border-default) text-xs">
      {/* 전체 색상 */}
      <div className={zone('_base', 'flex items-center gap-1.5 rounded-t-xl px-3 py-1.5')} onClick={() => onSelectGroup('_base')}>
        {tag('_base', t('dashboard.settings.preview.tagBase'))}
        <span className="h-2.5 w-2.5 rounded-full border border-gray-300 dark:border-gray-600"
          style={panelColor ? { backgroundColor: panelColor } : { background: 'conic-gradient(from 0deg, #ef4444, #f59e0b, #10b981, #3b82f6, #8b5cf6, #ef4444)' }} />
        <span className="text-[10px] text-(--color-text-muted)">{panelColor ?? t('dashboard.settings.accent.default')}</span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 인디케이터 + 라벨 */}
      <div className={zone('labels', 'flex items-center gap-2 px-3 py-2')} onClick={() => onSelectGroup('labels')}>
        {tag('labels', t('dashboard.settings.preview.tagLabels'))}
        <span className="h-2 w-2 rounded-full bg-green-500" style={effectiveColor('indicators') ? { backgroundColor: effectiveColor('indicators')! } : undefined} />
        <span className="text-sm font-medium text-(--color-text-primary)" style={effectiveColor('labels') ? { color: effectiveColor('labels')! } : undefined}>
          {t('dashboard.settings.preview.propertiesGrid')}
        </span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 속성 카드 그리드 */}
      <div className={cn('grid gap-2 p-3', colClass)}>
        {items.slice(0, Math.min(items.length, cols * 2)).map((item) => (
          <div
            key={item.key}
            className={zone('borders', 'rounded-lg border border-(--color-border-default) px-2 py-1.5')}
            onClick={() => onSelectGroup('borders')}
            style={effectiveColor('borders') ? { borderColor: `${effectiveColor('borders')}30` } : undefined}
          >
            {tag('borders', t('dashboard.settings.preview.tagBorders'))}
            <p className="text-[10px] text-(--color-text-muted)" style={effectiveColor('labels') ? { color: effectiveColor('labels')! } : undefined}>
              {item.label}
            </p>
            <p className="mt-0.5 text-xs font-medium text-(--color-text-primary)">{item.value}</p>
          </div>
        ))}
      </div>
    </div>
  );
}

function ResourceMiniPreview({
  selectedGroup,
  onSelectGroup,
  effectiveColor,
  panelColor,
}: {
  selectedGroup: string | null;
  onSelectGroup: (g: string) => void;
  effectiveColor: (g: string) => string | undefined;
  panelColor: string | undefined;
}) {
  const { t } = useTranslation();
  const zone = (group: string, extra?: string) =>
    cn('cursor-pointer transition-all relative',
      selectedGroup === group ? 'ring-2 ring-inset ring-blue-500/70 bg-blue-50/30 dark:bg-blue-900/15' : 'hover:bg-blue-50/20 dark:hover:bg-blue-900/10', extra);
  const tag = (group: string, label: string) =>
    selectedGroup === group ? <span className="pointer-events-none absolute right-1 top-0.5 rounded bg-blue-500 px-1 py-px text-[8px] font-medium leading-tight text-white">{label}</span> : null;

  const metrics = [
    { key: 'cpu', label: 'CPU', value: '45.2%', defaultColor: '#3b82f6' },
    { key: 'memory', label: 'MEM', value: '62.8%', defaultColor: '#8b5cf6' },
    { key: 'throughput', label: 'MSG/S', value: '1,234', defaultColor: '#10b981' },
    { key: 'errorRate', label: 'ERR', value: '0.3%', defaultColor: '#ef4444' },
  ];

  return (
    <div className="h-full w-full overflow-hidden rounded-xl border border-(--color-border-default) text-xs">
      {/* 전체 색상 - _base */}
      <div className={zone('_base', 'flex items-center gap-1.5 rounded-t-xl px-3 py-1.5')} onClick={() => onSelectGroup('_base')}>
        {tag('_base', t('dashboard.settings.preview.tagBase'))}
        <span className="h-2.5 w-2.5 rounded-full border border-gray-300 dark:border-gray-600"
          style={panelColor ? { backgroundColor: panelColor } : { background: 'conic-gradient(from 0deg, #ef4444, #f59e0b, #10b981, #3b82f6, #8b5cf6, #ef4444)' }} />
        <span className="text-[10px] text-(--color-text-muted)">{panelColor ?? t('dashboard.settings.accent.default')}</span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 타이틀 */}
      <div className={zone('header', 'px-3 py-2')} onClick={() => onSelectGroup('header')}>
        {tag('header', t('dashboard.settings.preview.tagHeader'))}
        <span className="text-sm font-semibold text-(--color-text-primary)" style={effectiveColor('header') ? { color: effectiveColor('header')! } : undefined}>
          {t('dashboard.settings.preview.processResource')}
        </span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 메트릭 카드 - 각 카드가 독립 악센트 그룹 */}
      <div className="grid grid-cols-2 gap-2 p-3">
        {metrics.map((m) => {
          const cardColor = effectiveColor(m.key) ?? m.defaultColor;
          return (
            <div
              key={m.key}
              className={zone(m.key, 'rounded-md border border-(--color-border-default) p-2 text-center')}
              onClick={() => onSelectGroup(m.key)}
            >
              {tag(m.key, m.label)}
              <div className="mb-1 inline-flex items-center gap-1">
                <span className="text-[10px] text-(--color-text-muted)" style={effectiveColor(m.key) ? { color: cardColor } : undefined}>{m.label}</span>
              </div>
              <div className="text-base font-bold">
                <span style={{ color: cardColor }}>{m.value}</span>
              </div>
              <div className="mt-1 flex items-end gap-px h-4">
                {[3,5,4,6,8,7,5,6].map((h, i) => (
                  <div key={i} className="flex-1 rounded-sm" style={{ height: `${h * 2}px`, backgroundColor: cardColor, opacity: 0.6 }} />
                ))}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

/** 로그 패널 미니 프리뷰 */
function LogMiniPreview({
  selectedGroup,
  onSelectGroup,
  effectiveColor,
  panelColor,
}: {
  selectedGroup: string | null;
  onSelectGroup: (g: string) => void;
  effectiveColor: (g: string) => string | undefined;
  panelColor: string | undefined;
}) {
  const { t } = useTranslation();
  const zone = (group: string, extra?: string) =>
    cn('cursor-pointer transition-all relative',
      selectedGroup === group ? 'ring-2 ring-inset ring-blue-500/70 bg-blue-50/30 dark:bg-blue-900/15' : 'hover:bg-blue-50/20 dark:hover:bg-blue-900/10', extra);
  const tag = (group: string, label: string) =>
    selectedGroup === group ? <span className="pointer-events-none absolute right-1 top-0.5 rounded bg-blue-500 px-1 py-px text-[8px] font-medium leading-tight text-white">{label}</span> : null;

  const rows = [
    { time: '14:23:01', level: 'INFO', lvCls: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400', src: 'mqtt', msg: 'connected to broker' },
    { time: '14:23:05', level: 'WARN', lvCls: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400', src: 'flow', msg: 'retry attempt 3' },
    { time: '14:23:08', level: 'ERR', lvCls: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400', src: 'samsung_hvacr01', msg: 'timeout on device A1' },
  ];

  return (
    <div className="h-full w-full overflow-hidden rounded-xl border border-(--color-border-default) text-xs">
      {/* 전체 색상 - _base */}
      <div className={zone('_base', 'flex items-center gap-1.5 rounded-t-xl px-3 py-1.5')} onClick={() => onSelectGroup('_base')}>
        {tag('_base', t('dashboard.settings.preview.tagBase'))}
        <span className="h-2.5 w-2.5 rounded-full border border-gray-300 dark:border-gray-600"
          style={panelColor ? { backgroundColor: panelColor } : { background: 'conic-gradient(from 0deg, #ef4444, #f59e0b, #10b981, #3b82f6, #8b5cf6, #ef4444)' }} />
        <span className="text-[10px] text-(--color-text-muted)">{panelColor ?? t('dashboard.settings.accent.default')}</span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 타이틀 */}
      <div className={zone('header', 'px-3 py-2')} onClick={() => onSelectGroup('header')}>
        {tag('header', t('dashboard.settings.preview.tagHeader'))}
        <span className="text-sm font-semibold text-(--color-text-primary)" style={effectiveColor('header') ? { color: effectiveColor('header')! } : undefined}>
          {t('dashboard.settings.preview.systemLog')}
        </span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {/* 로그 행 */}
      <div className="bg-(--color-bg-sunken) font-mono">
        {rows.map((r, i) => (
          <div key={i} className="flex items-center gap-1.5 border-b border-(--color-border-subtle) px-2 py-1">
            <span className={zone('timestamp', 'shrink-0 text-[9px]')} onClick={(e) => { e.stopPropagation(); onSelectGroup('timestamp'); }}>
              {i === 0 && tag('timestamp', t('dashboard.settings.preview.tagTimestamp'))}
              <span style={effectiveColor('timestamp') ? { color: effectiveColor('timestamp')! } : undefined} className="text-(--color-text-muted)">{r.time}</span>
            </span>
            <span className={zone('levels', 'shrink-0')} onClick={(e) => { e.stopPropagation(); onSelectGroup('levels'); }}>
              {i === 0 && tag('levels', t('dashboard.settings.preview.tagLevels'))}
              <span className={cn('rounded px-1 py-px text-[8px] font-semibold', r.lvCls)}
                style={effectiveColor('levels') ? { backgroundColor: `${effectiveColor('levels')}20`, color: effectiveColor('levels')! } : undefined}>
                {r.level}
              </span>
            </span>
            <span className={zone('source', 'shrink-0')} onClick={(e) => { e.stopPropagation(); onSelectGroup('source'); }}>
              {i === 0 && tag('source', t('dashboard.settings.preview.tagSource'))}
              <span className="text-[9px] text-purple-600 dark:text-purple-400" style={effectiveColor('source') ? { color: effectiveColor('source')! } : undefined}>{r.src}</span>
            </span>
            <span className="flex-1 truncate text-[9px] text-(--color-text-primary)">{r.msg}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
