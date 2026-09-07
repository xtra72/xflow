// 패널 상세 설정 다이얼로그.
// 편집 모드에서 패널별 설정(타이틀, 색상, 컬럼/필드 가시성, 타입별 설정)을 관리한다.

import { useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import { QueryClientContext, useQuery } from '@tanstack/react-query';

import { inertQueryClient } from '@/hooks/inertQueryClient';
import { getMetrics } from '@/services/api/monitorService';
import * as flowService from '@/services/api/flowService';
import type { FlowInfo } from '@/types/flow';
import { ArrowLeft, Check, ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Gauge, Maximize2, Minimize2, Minus, Pipette, Plus, RotateCcw, Trash2, X } from 'lucide-react';
import {
  Area,
  Bar,
  CartesianGrid,
  ComposedChart,
  Line,
  ReferenceArea,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

import {
  ALL_DEVICE_COLUMNS,
  DEVICE_COLUMN_LABELS,
  type DeviceListColumnKey,
} from '@/hooks/useDeviceColumns';
import { cn } from '@/lib/utils/cn';
import { PANEL_COLORS } from './panelColorPresets';
import { useTranslation, type TranslationFn } from '@/lib/i18n';

import { useAgents } from '@/hooks/useAgent';
import {
  SysResourceSelector,
  type SysResourceKind,
} from '@/components/property/SysResourceSelector';
import {
  panelBoxTransform,
  PANEL_SIZE_MAX,
  PANEL_SIZE_MIN,
  readPanelOffset,
  readPanelSize,
} from './panels/charts/panelGeometry';
import { FONT_FAMILY_OPTIONS } from './panels/charts/textStyle';
import {
  DEFAULT_TRACK_FILL,
  defaultGaugeThresholds,
  GAUGE_COLOR_THEMES,
  GAUGE_TYPES,
  HALF_RAINBOW_DIRECTIONS,
  readBaseColor,
  readHalfRainbowDirection,
} from '@/pages/dashboard/panels/gauge/gaugeShapes';
import {
  readValueScale,
  VALUE_SCALE_MAX,
  VALUE_SCALE_MIN,
} from '@/pages/dashboard/panels/charts/valueScale';
import {
  DEFAULT_SYSTEM_FIELDS,
  SYSTEM_FIELDS,
  normalizeSystemItems,
} from '@/pages/dashboard/panels/sysmetrics/sysMetricsFields';
import {
  MAX_ITEM_HEIGHT,
  MIN_ITEM_HEIGHT,
  hasOverride,
  readAllOverrides,
  TILE_ALIGNS,
  TILE_TEXT_SIZES,
  TILE_TEXT_WEIGHTS,
  optionFieldsFor,
  readPanelOptions,
  readStyle,
  stylesFor,
  stylesForPanel,
  withItemOverride,
  SYSMETRICS_COUNTER_MODES,
  type SysMetricValueKind,
  type SysMetricsStyle,
} from '@/pages/dashboard/panels/sysmetrics/sysMetricsItemOptions';
import { useStations, useXsfmDevices } from '@/hooks/useStation';
import { useGroups } from '@/hooks/useGroups';
import { useDevices, useDeviceRealtime } from '@/hooks/useDevice';
import {
  gaugeValueSourceFlags,
  resolveGaugeValueSource,
} from './panels/charts/gaugeLegacyBinding';
import GaugePanel, { type GaugeType } from './panels/GaugePanel';
import { isAxisSplitGauge } from './panels/gauge/gaugeAxis';
import { readVBarSize, VBAR_SIZE_MAX, VBAR_SIZE_MIN } from './panels/gauge/gaugeShapes';
// SPEC-MODBUS-012: MODBUS Gateway 패널 설정 섹션 + 프리뷰(설정 다이얼로그 내 실제 패널 렌더).
import { useModbusListDevices, formatUnitLabel } from './panels/modbus/useModbusData';
import ModbusRealDevicesPanel from './panels/modbus/ModbusRealDevicesPanel';
import ModbusVirtualDevicesPanel from './panels/modbus/ModbusVirtualDevicesPanel';
import ModbusSharedRegistersPanel from './panels/modbus/ModbusSharedRegistersPanel';
import ModbusDeviceRegistersPanel from './panels/modbus/ModbusDeviceRegistersPanel';
import ModbusBusStatsPanel from './panels/modbus/ModbusBusStatsPanel';
import ModbusSummaryStatsPanel from './panels/modbus/ModbusSummaryStatsPanel';
import {
  REGISTER_AREA_LABELS,
  REGISTER_AREA_ORDER,
  resolveAreaColumns,
  type RegisterArea,
} from './panels/modbus/registerCellState';
import {
  getDeviceDisplayName,
  getDeviceTypeLabel,
  defaultUnitOf,
  getPropertyLabel,
  isDerivedPropertyKey,
  listDisplayableProperties,
  PROPERTY_GROUPS,
  propertyGroupOf,
} from '@/lib/utils/deviceLabels';
import {
  buildEnumLabelMap,
  buildDefaultStoreSource,
  formatEnumValue,
  resolveAxisFont,
  STROKE_DASHARRAY,
  THRESHOLD_DEFAULT_COLORS,
  type AxisFontStyle,
  type ChannelRefConfig,
  type StoreSourceConfig,
  type YThreshold,
  type YAxisMode,
  type YAxisDataType,
  type YEnumLabel,
  type LegendConfig as ChartLegendConfig,
  type TsdbSourceConfig,
  type SysmetricsSourceConfig,
} from './panels/charts/chartChannelTypes';
import { ChartLegend } from './panels/charts/ChartLegend';
import { ChartDragLayer } from './ChartDragLayer';
import { clampStoredLegendOffset } from './panels/charts/legendOverlay';
import { CandleShape } from './panels/charts/CandleShape';
import { candleRows } from './panels/charts/candle';
import { readGraphStyle } from './panels/charts/graphStyle';
import { PanelEditContext } from './panelEditContext';
import {
  DEVICE_BADGES,
  DEVICE_BADGE_DEFAULT,
  DEVICE_BADGE_LABEL_KEYS,
  type DeviceBadge,
} from './panels/PropertiesGridPanel';
import { renderDashboardPanel } from './renderDashboardPanel';
import { computeFitScale, computePreviewStage, initialPreviewZoom } from './previewStage';
import { PanelGridBackdrop } from './PanelGridBackdrop';
import {
  AGENT_BADGES,
  AGENT_BADGE_LABEL_KEYS,
  MESSAGE_GRID_DEFAULT,
  MESSAGE_GROUPS,
  MESSAGE_GROUP_LABEL_KEYS,
  MESSAGE_TILES,
  MESSAGE_TILE_SETTING_LABEL_KEYS,
  MESSAGE_TILE_SIZE,
  STAT_GRID_DEFAULT,
  STAT_TILES,
  STAT_TILE_LABEL_KEYS,
  STAT_TILE_SIZE,
  type AgentBadge,
  type MessageGroup,
  type MessageTile,
  type StatTile,
} from './panels/AgentStatusPanel';
import {
  readTileDesign,
  readTileFont,
  readTileItems,
  type TileDesign,
} from './panels/tileSelection';
import {
  MAX_TILE_GRID,
  MIN_TILE_GRID,
  moveTile,
  placeTiles,
  readTileGrid,
  resizeTile,
  type TileArea,
  type TileGrid,
} from './panels/tileLayout';
import {
  readSummaryItems,
  SUMMARY_ITEMS,
  type SummaryItem,
} from './panels/listPanelStyle';
import {
  CARD_ELEMENTS,
  MAX_CARD_DIV,
  moveArea,
  resizeArea,
  VALUE_RULE_OPS,
  MIN_CARD_DIV,
  moveToPosition,
  readPropertiesGridStyle,
  moveWithinGroup,
  PROPERTY_GROUP_AREA_KEY,
  setGroupSelection,
  PROPERTY_GROUP_GRID,
  PROPERTY_GROUP_GRID_KEY,
  PROPERTY_GROUP_LABEL_KEYS,
  PROPERTY_TILE_SIZE,
  readPropertyOverride,
  resolveCardLayout,
  resolveTileLabel,
  type CardArea,
  type CardAreas,
  type CardElement,
  type CardGrid,
  type PropertiesGridStyle,
  type PropertyOverride,
  type ValueColorRule,
  type ValueRuleOp,
} from './panels/propertiesGridStyle';
import { PanelResizeOverlay } from './PanelResizeOverlay';
import { sameGridSize, type GridSize } from './previewGridSize';
import { buildPreviewSeries } from './panels/charts/previewSeries';
import { chartLayoutResetPatch, isChartLayoutDirty } from './panels/charts/chartLayout';
import { mergeLivePreviewConfig } from './previewLiveKeys';
// SPEC-HEATMAP-PANEL-001: 히트맵 설정 섹션(store 태그 + 센서 좌표 + 상하한 + 색상표 + IDW).
import {
  parseHeatmapConfig,
  DEFAULT_CONTOUR_LEVEL_COUNT,
  DEFAULT_LEGEND_TICK_COUNT,
  MAX_LEGEND_FONT_SIZE,
  MIN_LEGEND_FONT_SIZE,
  type ColorStop,
  type ContourConfig,
  type FloorPlanLayer,
  type LegendConfig,
} from './panels/heatmap/heatmapConfig';
import { MIN_GRID_RESOLUTION, MAX_GRID_RESOLUTION } from './panels/heatmap/HeatmapCanvas';
// 히트맵 프리뷰(설정 다이얼로그 내 실제 패널 렌더 — MODBUS 프리뷰 선례와 동일 방식).
import HeatmapPanel from './panels/heatmap/HeatmapPanel';
import BarChartPanel from './panels/charts/BarChartPanel';
import LineChartPanel from './panels/charts/LineChartPanel';
import PieChartPanel from './panels/charts/PieChartPanel';
import StatPanel from './panels/charts/StatPanel';
import TablePanel from './panels/charts/TablePanel';
import { isPanelSeriesActive } from './panels/charts/panelDataSource';
// "채널이 아닌 활성 소스인가" 판정 — 소스 종류가 늘어도 식이 그대로다.
import {
  clonePresetStops,
  HEATMAP_COLOR_PRESETS,
} from './panels/heatmap/heatmapColorPresets';
// SPEC-HEATMAP-PANEL-002: 도면 이미지 첨부(data-URL) + 크기 상한 검증.
import {
  readImageAsDataUrl,
  readImageNaturalSize,
  assertImageSizeUnderLimit,
  DEFAULT_MAX_IMAGE_BYTES,
  ImageSizeLimitError,
} from './panels/heatmap/imageAsset';
import { uploadDashboardAsset } from '@/services/api/dashboardAssetService';
import { useFloorPlanSources } from './panels/heatmap/useFloorPlanSources';
import { useFloorPlanAspect } from './panels/heatmap/useFloorPlanAspect';
import { gridCellSize, gridHeightForAspect, panelPixelAspect } from './gridGeometry';
import { PanelChromeProvider } from './PanelChromeProvider';
import type { PanelTitleFont } from './panelChromeContext';
import ColorSwatchButton from './colorSwatchPalette';
import { COLOR_PALETTE } from './colorPalette';
import {
  StatChartSection,
  LineChartSection,
  BarChartSection,
  PieChartSection,
  TableChartSection,
  TileRowsField,
  DecimalPlacesField,
  UnitField,
  TextStyleFields,
  DesignPopover,
} from './ChartPanelSections';
import { PanelSettingsDataSource } from './PanelSettingsDataSource';
import { useDraftPanelConfig } from './useDraftPanelConfig';
import { useDebouncedValue } from './useDebouncedValue';
import { usePanelSettingsRatio } from './usePanelSettingsRatio';
import { SECTION_CATALOG } from '@/pages/monitoring/monitoringCatalog';
import { useNetworkStats } from '@/services/api/monitorService';
import type { MonitorSectionKey } from '@/pages/monitoring/monitoringLayout';
import {
  WINDOW_SEC_OPTIONS,
  joinDuration,
  maxColsOptions,
  readInterfaces,
  readMaxCols,
  readPanelItems,
  readRefreshMs,
  readWindowSec,
  splitDuration,
} from './panels/monitor/monitorPanelConfig';
import AcControlStyleSection from './AcControlStyleSection';
import AcControlThresholdsSection from './AcControlThresholdsSection';
import type { ValueColorConfig } from './panels/acControlColors';
import {
  useUIStore,
  type PanelConfig,
  type FlowColumnKey,
  type AgentColumnKey,
  type MetricKey,
  ALL_FLOW_COLUMNS,
  ALL_AGENT_COLUMNS,
  ALL_METRIC_KEYS,
} from '@/stores/uiStore';

// ---- 차트 패널 공통 (SPEC-CHART-001 M5) ----

/** 차트 계열 패널 타입 집합 (REQ-M5-03) */
const CHART_PANEL_TYPES = new Set(['stat', 'graph-chart', 'bar-chart', 'pie-chart', 'table']);

// 단일 `channel_name` 편집 섹션은 없어졌다. 차트 계열 전체가 시리즈 소스(store/tsdb)로
// 일원화되어(`SERIES_SOURCE_PANEL_TYPES`) 채널 이름을 편집할 자리가 필요 없다.

/**
 * 채널 모드를 더 이상 기본으로 두지 않는 패널 타입 — 설정 진입 시 store 로 자동 이관된다.
 *
 * 신규 패널은 이미 store 기본값으로 태어나지만(`uiStore.createDefaultPanel`), 그 이전에
 * 만들어진 패널은 `data_source` 가 없거나 `'channel'` 이다. 채널 편집 섹션을 없앤 뒤 그
 * 패널들을 그대로 두면 채널 이름을 고칠 수단이 사라진 채 채널 모드에 갇힌다.
 *
 * **게이지는 제외한다.** 이관의 목적은 "편집 수단을 잃은 패널을 구제" 인데, 게이지는
 * 애초에 이 채널 섹션의 대상이 아니었다 — 게이지의 채널 바인딩은 레거시 `dataSources[]`
 * 편집기(`sourceType: 'chart-emitter'`)에 있고 그것은 그대로 남아 있으므로 갇히지 않는다.
 * 반대로 게이지를 넣으면 SPEC-CHART-002 §2.8 [E2] "저장된 config 를 자동으로 조용히 다시
 * 쓰지 않는다" 를 깨고(AC-20), §4.5 되돌리기 경로까지 흔든다 — 얻는 것 없이 계약만 잃는다.
 */
const SERIES_SOURCE_PANEL_TYPES: ReadonlySet<string> = new Set([
  'stat',
  'bar-chart',
  'pie-chart',
  // 테이블도 채널 편집 섹션이 사라졌으므로 같은 구제 대상이다 — 이관하지 않으면
  // 기존 채널 모드 테이블이 채널 이름을 고칠 수단 없이 채널 모드에 갇힌다.
  'table',
  // 채널이 패널 소스에서 완전히 빠지면서 나머지 둘도 같은 처지가 됐다. 게이지를 종전에
  // 제외한 이유(레거시 `dataSources[]` 로 채널을 물고 있어 이 섹션의 대상이 아니었다)는
  // 더 이상 구제를 미룰 근거가 되지 못한다 — 그 레거시 경로 자체가 사라졌다.
  'graph-chart',
  'gauge',
]);

/** SPEC-MODBUS-012: MODBUS Gateway 패널 6종 집합(설정 섹션/프리뷰 분기용). */
const MODBUS_PANEL_TYPES = new Set([
  'modbus-real-devices',
  'modbus-virtual-devices',
  'modbus-shared-registers',
  'modbus-device-registers',
  'modbus-bus-stats',
  'modbus-summary-stats',
]);

// 단일 config.columns(그리드 열 수) 컨트롤을 노출하는 MODBUS 패널 (SPEC-MODBUS-012).
// summary/bus 는 타일·미니차트 행당 개수를 단일 열 수로 제어한다.
const MODBUS_COLUMNS_PANEL_TYPES = new Set(['modbus-summary-stats', 'modbus-bus-stats']);

// 영역별 config.areaColumns(4영역 각각의 셀 열 수) 컨트롤을 노출하는 레지스터 맵 패널 (SPEC-MODBUS-012).
// coils/discrete_inputs/holding_registers/input_registers 4개 입력을 각각 제공한다.
const MODBUS_AREA_COLUMNS_PANEL_TYPES = new Set([
  'modbus-shared-registers',
  'modbus-device-registers',
]);

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

  // 이 패널이 대시보드에서 실제로 갖는 픽셀 종횡비. fit 미리보기가 이 비율을 쓰면 미리보기의
  // 여백(히트맵 레터박스)이 대시보드에서 보게 될 여백과 같아진다 — 하드코딩된 3:2 로는 원리적으로
  // 확인할 수 없던 부분이다. 그리드 폭 미측정(대시보드 미방문) 시 gridGeometry 가 근사로 폴백한다.
  const gridCols = useUIStore((s) => s.dashboardGridCols);
  const gridWidth = useUIStore((s) => s.dashboardGridWidth);
  const showGridLines = useUIStore((s) => s.dashboardShowGridLines);
  const setDashboardLayout = useUIStore((s) => s.setDashboardLayout);
  const layoutItem = activePage?.layout.find((l) => l.i === panelId) ?? null;
  const gridCell = gridCellSize(gridWidth, gridCols);

  // 패널 크기(그리드 단위)도 config 와 같은 draft 규율을 따른다 — 미리보기에서 끌면
  // draft 에만 쌓이고, 저장 버튼이 대시보드 레이아웃으로 커밋하며, 취소하면 사라진다.
  // 크기는 config 가 아니라 `activePage.layout` 에 있으므로 별도 상태로 둔다.
  // useMemo 로 고정한다 — 매 렌더 새 객체를 만들면 handleApply 의 의존성이 계속 바뀐다.
  const committedSize = useMemo<GridSize | null>(
    () => (layoutItem !== null ? { w: layoutItem.w, h: layoutItem.h } : null),
    [layoutItem],
  );
  const [draftSize, setDraftSize] = useState<GridSize | null>(null);
  useEffect(() => {
    // 다른 패널로 넘어가면 이전 패널의 draft 크기를 들고 가지 않는다.
    setDraftSize(null);
  }, [panelId]);
  const effectiveSize = draftSize ?? committedSize;

  const panelAspect = panelPixelAspect(
    effectiveSize?.w ?? 0,
    effectiveSize?.h ?? 0,
    gridCell,
  );

  // 미리보기가 **실제 패널**을 그리므로 대시보드와 같은 데이터를 넘겨야 한다
  // (flows 목록 · 리소스 메트릭 · 폴링 주기). Provider 가 없는 테스트에서도
  // 렌더가 깨지지 않도록 비활성 클라이언트로 떨어진다(`inertQueryClient` 주석 참조).
  const queryClient = useContext(QueryClientContext);
  const previewRefreshMs = useUIStore((s) => s.dashboardRefreshInterval) * 1000;
  const { data: previewFlowsData } = useQuery(
    {
      queryKey: ['flows', undefined],
      queryFn: () => flowService.getFlows(),
      enabled: queryClient !== undefined,
    },
    queryClient ?? inertQueryClient(),
  );
  const { data: previewMetrics } = useQuery(
    {
      queryKey: ['monitor', 'metrics'],
      queryFn: getMetrics,
      refetchInterval: previewRefreshMs,
      enabled: queryClient !== undefined,
    },
    queryClient ?? inertQueryClient(),
  );
  const previewFlows: FlowInfo[] = previewFlowsData?.data ?? [];

  // draft(편집 중) / committed(저장) 분리 — T9(REQ-14). panelId 전환 시에만 draft 재초기화
  // (같은 패널에서 외부 committed 변경이 편집 중 draft 를 덮어쓰지 않음 — 기존 동작 보존).
  const committedConfig = useMemo(() => storePanel?.config ?? {}, [storePanel?.config]);
  const committedTitle = storePanel?.title ?? '';
  const { draftConfig, draftTitle, patchConfig, setTitle } = useDraftPanelConfig(
    committedConfig,
    committedTitle,
    panelId ?? '',
  );

  const handleConfigChange = patchConfig;
  const handleTitleChange = setTitle;

  const handleApply = useCallback(() => {
    if (!storePanel) return;
    // 저장 = draft → committed 승격.
    updatePanelConfig(storePanel.id, draftConfig);
    updatePanelTitle(storePanel.id, draftTitle);

    // 크기는 레이아웃에 있으므로 따로 커밋한다. 바뀐 게 없으면 손대지 않는다 —
    // 레이아웃을 통째로 다시 쓰면 다른 패널의 위치까지 건드릴 수 있다.
    if (draftSize && !sameGridSize(draftSize, committedSize) && activePage) {
      setDashboardLayout(
        activePage.layout.map((item) =>
          item.i === storePanel.id ? { ...item, w: draftSize.w, h: draftSize.h } : item,
        ),
      );
    }
  }, [
    storePanel,
    draftConfig,
    draftTitle,
    updatePanelConfig,
    updatePanelTitle,
    draftSize,
    committedSize,
    activePage,
    setDashboardLayout,
  ]);

  const handleApplyAndClose = useCallback(() => {
    handleApply();
    onClose();
  }, [handleApply, onClose]);

  // 드래프트를 반영한 가상 패널 (옵션/데이터소스 편집 — 즉시 반영).
  const panel = useMemo(
    () =>
      storePanel
        ? { ...storePanel, config: draftConfig, title: draftTitle }
        : null,
    [storePanel, draftConfig, draftTitle],
  );

  // 목록형 패널(플로우 현황 · 에이전트 현황)의 자리별 글자 모양.
  const tableHeaderFont = (panel?.config?.table_header_font as PanelTitleFont | undefined) ?? {};
  const tableCellFont = (panel?.config?.table_cell_font as PanelTitleFont | undefined) ?? {};
  const badgeFont = (panel?.config?.badge_font as PanelTitleFont | undefined) ?? {};

  // 속성 그리드 카드의 조각별 글자 모양(항목명 · 값 · 갱신 시각).

  /** 컬럼 설정 옆에 접어 두는 디자인 팝오버 — 목록형 패널이 공유한다. */
  const columnsDesignPopover = (testId: string) => (
    <DesignPopover testId={testId}>
      <TextStyleFields
        label={t('dashboard.settings.listPanel.tableHeaderStyle')}
        family={tableHeaderFont.family}
        size={tableHeaderFont.size}
        color={tableHeaderFont.color}
        weight={tableHeaderFont.weight ?? 'inherit'}
        sizePlaceholder={t('dashboard.chart.inherit')}
        testIdPrefix="table-header-font"
        onChange={(patch) =>
          handleConfigChange({ table_header_font: mergeFont(tableHeaderFont, patch) })
        }
      />
      <TextStyleFields
        label={t('dashboard.settings.listPanel.tableCellStyle')}
        family={tableCellFont.family}
        size={tableCellFont.size}
        color={tableCellFont.color}
        weight={tableCellFont.weight ?? 'inherit'}
        sizePlaceholder={t('dashboard.chart.inherit')}
        testIdPrefix="table-cell-font"
        onChange={(patch) =>
          handleConfigChange({ table_cell_font: mergeFont(tableCellFont, patch) })
        }
      />
    </DesignPopover>
  );

  /**
   * 요약 배지 설정(표시 여부 + 디자인) — 목록형 패널이 공유한다.
   *
   * `extra` 는 패널마다 다른 항목(에이전트 현황의 타일 목록 등)을 같은 접이 섹션 안에
   * 끼워 넣는 자리다. 따로 섹션을 만들면 같은 배지를 두 자리에서 고치게 된다.
   */
  const summaryBadgeSection = (extra?: React.ReactNode) => (
    <CollapsibleSection title={t('dashboard.settings.listPanel.summaryBadges')}>
      <div className="flex items-center gap-1.5">
        <label className="flex items-center gap-2 text-xs text-(--color-text-secondary)">
          <input
            type="checkbox"
            data-testid="show-summary-badges"
            checked={panel?.config?.showSummaryBadges !== false}
            onChange={(e) =>
              // 기본은 표시다 — 끌 때만 config 에 남긴다.
              handleConfigChange({ showSummaryBadges: e.target.checked ? undefined : false })
            }
            className="h-3.5 w-3.5 accent-blue-600"
          />
          {t('dashboard.settings.listPanel.showSummaryBadges')}
        </label>
        {/* 감춘 배지에는 걸 곳이 없으므로 표시할 때만 낸다(타이틀 디자인과 같은 규칙). */}
        {panel?.config?.showSummaryBadges !== false && (
          <DesignPopover testId="badge-design">
            <TextStyleFields
              label={t('dashboard.settings.listPanel.badgeStyle')}
              family={badgeFont.family}
              size={badgeFont.size}
              color={badgeFont.color}
              weight={badgeFont.weight ?? 'inherit'}
              sizePlaceholder={t('dashboard.chart.inherit')}
              testIdPrefix="badge-font"
              onChange={(patch) => handleConfigChange({ badge_font: mergeFont(badgeFont, patch) })}
            />
          </DesignPopover>
        )}
      </div>
      {panel?.config?.showSummaryBadges !== false && extra}
    </CollapsibleSection>
  );

  // 타이틀 글자 모양 — 모든 패널 공통 크롬 옵션(`panelChromeContext`).
  const titleFont = (panel?.config?.title_font as PanelTitleFont | undefined) ?? {};

  // 라인 차트 배치(그림 상자 크기·자리 + 범례 자리) — 미리보기에서 끌어 고치는 값들이다.
  // 판정과 되돌리기는 순수 모듈이 소유한다(`chartLayout.ts`).
  const chartConfig = panel?.config;
  const layoutDirty = isChartLayoutDirty(chartConfig);
  const resetChartLayout = useCallback(
    () => patchConfig(chartLayoutResetPatch(chartConfig)),
    [chartConfig, patchConfig],
  );

  // 통계/게이지/바/파이의 채널 모드 → store 자동 이관.
  //
  // 이 4종은 채널 편집 섹션을 노출하지 않으므로(`CHANNEL_SECTION_PANEL_TYPES`) 채널 모드에
  // 남겨 두면 채널 이름을 고칠 수단이 없는 상태가 된다. 설정을 여는 시점에 시리즈 소스로
  // 옮겨 편집 가능한 상태로 만든다.
  //
  // **draft 만 바꾼다.** 커밋은 저장 버튼이 하므로(`handleApply`), 설정을 열었다가 저장
  // 없이 닫으면 패널은 그대로 남고 취소 버튼으로도 되돌아간다. 스토어를 직접 건드리면
  // 화면만 열어도 대시보드가 변경된 것으로 표시된다.
  //
  // `store_source` 는 **있으면 보존한다** — 채널 모드로 되돌려 둔 패널에도 예전 store 설정이
  // 남아 있을 수 있고, 그것을 기본형으로 덮으면 사용자가 고른 시리즈가 조용히 사라진다.
  //
  // `channel_name` 도 지우지 않는다. 갓 이관된 store 소스는 시리즈가 없어 비활성이므로
  // (`isStoreSourceActive` false) 패널은 채널 경로로 폴백해 **이관 직후에도 종전과 같은 값을
  // 계속 그린다**. 시리즈를 고르는 순간 store 로 넘어간다.
  useEffect(() => {
    if (!storePanel || !SERIES_SOURCE_PANEL_TYPES.has(storePanel.type)) return;
    // 채널이 소스에서 빠진 뒤로 판정은 **저장값 원문**을 본다. 계약이 이미 store 로 접어
    // 주므로 `kind` 로는 이관 대상을 가릴 수 없다 — config 에 남은 옛 값이 그대로 있는지가
    // 기준이다. 부재도 대상이다(그 시절의 기본이 채널이었다).
    const raw = draftConfig.data_source;
    if (raw !== undefined && raw !== null && raw !== 'channel') return;
    patchConfig({
      data_source: 'store',
      store_source:
        (draftConfig.store_source as StoreSourceConfig | undefined) ??
        buildDefaultStoreSource(),
    });
  }, [storePanel, draftConfig, patchConfig]);

  // 라이브 미리보기용 디바운스 config — 실 store 데이터 패널(heatmap/line/gauge/modbus)의
  // 잦은 편집 재렌더/재조회를 억제한다(T9/AC-13/R3). 옵션 편집은 즉시(panel), 미리보기는
  // 디바운스(previewPanel)로 분리한다.
  const debouncedConfig = useDebouncedValue(draftConfig, 200);
  // 디바운스에서 **빼는** 필드 — 목록과 병합 규칙은 `previewLiveKeys.ts` 가 소유한다
  // (끌어 옮기는 값을 새로 만들 때 목록에 넣는 것을 잊는 일이 반복됐다).
  const livePreviewConfig = useMemo(() => {
    return mergeLivePreviewConfig(debouncedConfig, draftConfig);
  }, [debouncedConfig, draftConfig]);
  const previewPanel = useMemo(
    () =>
      storePanel
        ? { ...storePanel, config: livePreviewConfig, title: draftTitle }
        : null,
    [storePanel, livePreviewConfig, draftTitle],
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

  // 미리보기 채움 모드: 'fill'(영역을 가로·세로 모두 채움) / 'fit'(종횡비 보존 + 최대 맞춤).
  // 공간 채우는 패널(heatmap/차트/리스트/리소스/로그/modbus)에만 적용되며, 게이지/accent
  // 미니는 항상 종횡비를 보존한다. localStorage 로 영속(기본 'fill' = 기존 동작).
  const [previewFillMode, setPreviewFillMode] = useState<'fill' | 'fit'>(() => {
    if (typeof window === 'undefined') return 'fill';
    return window.localStorage.getItem('panelSettings.previewFillMode') === 'fit'
      ? 'fit'
      : 'fill';
  });
  useEffect(() => {
    if (typeof window === 'undefined') return;
    window.localStorage.setItem('panelSettings.previewFillMode', previewFillMode);
  }, [previewFillMode]);

  // fit 모드 전용 실측: fit 컨테이너의 실제 px 크기(W×H)를 ResizeObserver 로 재어, 종횡비
  // 보존 contain 박스를 결정론적으로 계산한다(CSS transferred-size 불확실성 제거). 콜백 ref 로
  // 마운트/언마운트(접힘 토글) + 영역 리사이즈(경계 드래그)에 자동 재측정된다.
  const [fitSize, setFitSize] = useState<{ w: number; h: number }>({ w: 0, h: 0 });
  const fitRoRef = useRef<ResizeObserver | null>(null);
  const setFitContainer = useCallback((el: HTMLDivElement | null) => {
    if (fitRoRef.current) {
      fitRoRef.current.disconnect();
      fitRoRef.current = null;
    }
    if (!el || typeof ResizeObserver === 'undefined') return;
    const measure = (): void => setFitSize({ w: el.clientWidth, h: el.clientHeight });
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    fitRoRef.current = ro;
  }, []);

  // 패널 전체가 영역에 들어가는 배율. 맞춤 모드의 100% 는 대시보드 1:1 이라
  // 큰 패널은 넘치므로, 줌 하한을 이 값까지 열어 두어야 전체를 볼 수 있다.
  const previewFitScale = computeFitScale({
    w: effectiveSize?.w ?? 0,
    h: effectiveSize?.h ?? 0,
    cell: gridCell,
    areaW: fitSize.w,
    areaH: fitSize.h,
  });

  // 미리보기 줌 배율. 맞춤 모드의 100% 는 대시보드와 같은 크기다.
  // 채움 모드는 100% 가 이미 영역에 꼭 맞으므로 기본 하한(0.5)이면 충분하다.
  const PREVIEW_ZOOM_MIN =
    previewFillMode === 'fit' && previewFitScale !== null
      ? Math.min(0.5, previewFitScale)
      : 0.5;
  const PREVIEW_ZOOM_MAX = 2.0;
  const PREVIEW_ZOOM_STEP = 0.1;
  const [previewZoom, setPreviewZoom] = useState<number>(1.0);
  /**
   * 미리보기에 **실제 데이터**를 쓸지.
   *
   * 기본 참 — 종전 동작(활성 소스면 실제 패널 렌더)을 그대로 둔다. 끄면 합성
   * 미리보기로 내려가 조회 없이 스타일만 확인할 수 있다. 데이터가 아직 없거나
   * 폴링 비용을 피하고 싶을 때 필요하다.
   */
  const [previewRealData, setPreviewRealData] = useState(true);
  const zoomIn = useCallback(
    () => setPreviewZoom((z) => Math.min(PREVIEW_ZOOM_MAX, Math.round((z + PREVIEW_ZOOM_STEP) * 10) / 10)),
    [],
  );
  const zoomOut = useCallback(
    () => setPreviewZoom((z) => Math.max(PREVIEW_ZOOM_MIN, Math.round((z - PREVIEW_ZOOM_STEP) * 10) / 10)),
    [PREVIEW_ZOOM_MIN],
  );
  // 리셋은 100% — 대시보드와 같은 크기다(큰 패널은 넘친다).
  const zoomReset = useCallback(() => setPreviewZoom(1.0), []);

  /**
   * 패널을 열거나 모드를 바꿀 때의 시작 배율.
   *
   * 맞춤 100% 는 대시보드 1:1 이라, 큰 패널을 그대로 열면 가운데 일부만 보인다 —
   * 카드 테두리도 제목도 화면 밖이라 패널 모양을 알 수 없다. 그래서 **처음에는
   * 전체가 보이는 배율**로 시작한다. 영역보다 작은 패널은 1:1 그대로 둔다(확대해
   * 띄우지 않는다 — 100% 가 실제 크기라는 약속을 깨지 않기 위함).
   *
   * 채움 모드는 100% 가 이미 영역에 꼭 맞으므로 1 이다.
   */
  const autoZoomKeyRef = useRef<string | null>(null);
  useEffect(() => {
    const key = `${panelId ?? ''}|${previewFillMode}`;
    if (autoZoomKeyRef.current === key) return;
    // 영역 실측 전에는 배율을 알 수 없다 — 실측되면 이 효과가 다시 돈다.
    if (previewFitScale === null) return;
    autoZoomKeyRef.current = key;
    setPreviewZoom(initialPreviewZoom(previewFillMode, previewFitScale));
    // previewFitScale 은 크기 조절 중에도 바뀐다. 위 key 가드가 (패널, 모드) 조합당
    // 한 번만 적용되게 막아, 손잡이를 끄는 도중 배율이 튀지 않는다.
  }, [panelId, previewFillMode, previewFitScale]);
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

  // 2경계 비율(옵션 컬럼 폭 + 미리보기 높이 비율) — 패널별 localStorage 영속/복원 (T2/T3).
  // 손상/부재 값은 기본 비율로 폴백한다(usePanelSettingsRatio 내부, AC-03 edge).
  const { ratio, setRatio } = usePanelSettingsRatio(panelId ?? '');
  const leftWidth = ratio.optionsWidth;
  const previewRatio = ratio.previewRatio;

  // 좌우(세로) 경계 드래그 — 우측 옵션 컬럼 폭 조절. mousemove/mouseup 은 window 에 부착.
  const [isDragging, setIsDragging] = useState(false);
  useEffect(() => {
    if (!isDragging) return;
    const onMove = (e: MouseEvent) => {
      // [미리보기 (flex-1)] [splitter] [설정 (leftWidth, 우측 고정폭)].
      // 설정 컬럼이 우측 고정이므로 폭 = 다이얼로그 우측 가장자리 - 마우스 X (setRatio 가 클램프).
      const dialog = document.querySelector(
        '[data-panel-settings-content]',
      ) as HTMLElement | null;
      if (!dialog) return;
      const rect = dialog.getBoundingClientRect();
      setRatio({ optionsWidth: rect.right - e.clientX });
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
  }, [isDragging, setRatio]);

  // 상하(가로) 경계 드래그 — 좌측 컬럼 내 미리보기(상단) 높이 비율 조절.
  const [previewDragging, setPreviewDragging] = useState(false);
  useEffect(() => {
    if (!previewDragging) return;
    const onMove = (e: MouseEvent) => {
      const region = document.querySelector(
        '[data-testid="panel-settings-preview"]',
      ) as HTMLElement | null;
      if (!region) return;
      const rect = region.getBoundingClientRect();
      if (rect.height <= 0) return;
      setRatio({ previewRatio: (e.clientY - rect.top) / rect.height });
    };
    const onUp = () => setPreviewDragging(false);
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
    document.body.style.cursor = 'row-resize';
    document.body.style.userSelect = 'none';
    return () => {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };
  }, [previewDragging, setRatio]);

  // 악센트 라벨 키 결정 (값은 i18n 키, 렌더 시 t() 로 변환)
  const accentLabelKeys = panel?.type === 'device' || panel?.type === 'ac-control' || panel?.type === 'hvac-control' || panel?.type === 'properties-grid'
    ? ACCENT_ELEMENT_LABEL_KEYS
    : panel?.type === 'resource'
    ? RESOURCE_ACCENT_LABEL_KEYS
    : panel?.type === 'monitor-stats'
    ? MONITOR_STATS_ACCENT_LABEL_KEYS
    // 실시간 메트릭은 리소스 패널과 채널 구성이 같아 악센트 그룹도 그대로 쓴다.
    : panel?.type === 'monitor-metrics'
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

  if (!panel) return null;

  // SPEC-WEB-005: 차트 패널이면 데이터 소스 섹션을 좌측 프리뷰 아래에 넓게 배치한다.
  // 프리뷰가 접혀도 데이터 소스 섹션은 좌측 영역에 계속 노출된다.
  const isChartPanel = CHART_PANEL_TYPES.has(panel.type);
  // 히트맵도 store 데이터 소스를 쓰므로 차트 패널과 동일하게 프리뷰 아래 배치/레이아웃을 적용한다.
  // (CHART_PANEL_TYPES 자체에는 넣지 않는다 — 차트 전용 채널/타입 분기 오염 방지.)
  //
  // SPEC-CHART-002 §2.3 [U3] (M4.1): 게이지도 공용 Store 데이터 소스 surface 를 받는다.
  // heatmap 과 같은 이유로 CHART_PANEL_TYPES 에는 **넣지 않는다** — 그 집합은 데이터소스
  // 노출 외에 차트 전용 채널/타입 분기를 구동하며 게이지는 그 분기의 대상이 아니다(UB1-9).
  const dataSourceBelowPreview =
    isChartPanel || panel.type === 'heatmap' || panel.type === 'gauge';

  // @spec SPEC-PANEL-SETTINGS-001 (T1): 3분할 셸 슬롯 구성.
  // 기존 옵션/미리보기/데이터소스 편집 서브트리를 셸 영역으로 이관한다(편집 로직 보존).
  const optionsSlot = (
    <>
            {/*
              공통: 타이틀 / 디바이스 — "패널 옵션" CollapsibleSection 으로 그룹화 (Grafana 패턴).
              디바이스 필드는 패널 타입별 조건부.
            */}
            <CollapsibleSection title={t('dashboard.settings.panelOptions')}>
              <div className="space-y-3">
                <TitleSection
                  panel={panel}
                  onTitleChange={(v) => handleTitleChange(v)}
                  design={
                    /* 타이틀을 감춘 패널에는 걸 곳이 없으므로 표시할 때만 낸다.
                       미지정은 각 패널이 쓰던 모양 그대로다. */
                    panel.config?.showTitle !== false ? (
                      <DesignPopover testId="panel-title-design">
                        <TextStyleFields
                          label={t('dashboard.settings.titleTextStyle')}
                          family={titleFont.family}
                          size={titleFont.size}
                          color={titleFont.color}
                          weight={titleFont.weight ?? 'inherit'}
                          sizePlaceholder={t('dashboard.chart.inherit')}
                          testIdPrefix="panel-title-font"
                          onChange={(patch) => {
                            const next = { ...titleFont, ...patch };
                            // 전부 비면 필드를 지운다 — 빈 객체가 남으면 "설정했다" 로 읽힌다.
                            const empty = Object.values(next).every((v) => v === undefined);
                            handleConfigChange({ title_font: empty ? undefined : next });
                          }}
                        />
                      </DesignPopover>
                    ) : null
                  }
                />
                {/* 타이틀 바 표시(모든 패널 공통). 기본 표시 — 명시적으로 끌 때만 config 에 남긴다. */}
                <label className="flex items-center gap-2 text-xs text-(--color-text-secondary)">
                  <input
                    type="checkbox"
                    data-testid="panel-show-title"
                    checked={panel.config?.showTitle !== false}
                    onChange={(e) =>
                      handleConfigChange({ showTitle: e.target.checked ? undefined : false })
                    }
                    className="h-3.5 w-3.5 accent-blue-600"
                  />
                  {t('dashboard.settings.showTitleBar')}
                </label>
                {/*
                  패널 색상 — 타입과 무관한 패널 속성이라 공통 옵션에 둔다.
                  종전에는 스타일 섹션의 악센트 그룹 `_base` 가 이 값을 편집했는데,
                  (a) 그룹처럼 보이지만 실은 panelColor 를 직접 쓰는 예외였고
                  (b) 스타일 섹션이 없는 5종(flows·agents·devices·properties-grid·
                  agent-status)에서는 편집할 방법이 아예 없었다.
                */}
                <PanelColorRow
                  panelColor={panelColor}
                  onChange={(color) => handleConfigChange({ panelColor: color })}
                />
                {(panel.type === 'device' || panel.type === 'ac-control' || panel.type === 'hvac-control' || panel.type === 'properties-grid') && (
                  <DeviceSection
                    panel={panel}
                    onConfigChange={(c) => handleConfigChange(c)}
                  />
                )}
              </div>
            </CollapsibleSection>

            {/*
              타입별 설정.

              컬럼 표시 여부도 다른 옵션과 같이 **draft** 에 쌓아야 한다. 스토어에 직접
              쓰면(updatePanelConfig) draft 는 옛 값을 그대로 들고 있어 미리보기가 바뀌지
              않고, 저장 버튼이 그 옛 draft 를 커밋하면서 방금 한 변경이 되돌아간다.
            */}
            {panel.type === 'flows' && (
              <CollapsibleSection title={t('dashboard.settings.columns')}>
                <ColumnsSection<FlowColumnKey>
                  allColumns={[...ALL_FLOW_COLUMNS]}
                  labels={Object.fromEntries(ALL_FLOW_COLUMNS.map((k) => [k, t(FLOW_COLUMN_LABEL_KEYS[k])])) as Record<FlowColumnKey, string>}
                  visibleColumns={(panel.config?.visibleColumns as FlowColumnKey[]) ?? [...ALL_FLOW_COLUMNS]}
                  onChange={(cols) => handleConfigChange({ visibleColumns: cols })}
                  design={columnsDesignPopover('flow-columns-design')}
                />
              </CollapsibleSection>
            )}
            {panel.type === 'flows' && summaryBadgeSection()}
            {panel.type === 'agents' && (
              <CollapsibleSection title={t('dashboard.settings.columns')}>
                <ColumnsSection<AgentColumnKey>
                  allColumns={[...ALL_AGENT_COLUMNS]}
                  labels={Object.fromEntries(ALL_AGENT_COLUMNS.map((k) => [k, t(AGENT_COLUMN_LABEL_KEYS[k])])) as Record<AgentColumnKey, string>}
                  visibleColumns={(panel.config?.visibleColumns as AgentColumnKey[]) ?? [...ALL_AGENT_COLUMNS]}
                  onChange={(cols) => handleConfigChange({ visibleColumns: cols })}
                  design={columnsDesignPopover('agent-columns-design')}
                />
              </CollapsibleSection>
            )}
            {panel.type === 'agents' &&
              summaryBadgeSection(
                <TileListEditor<SummaryItem>
                  all={SUMMARY_ITEMS}
                  labelKeys={SUMMARY_TILE_LABEL_KEYS}
                  items={readSummaryItems(panel.config)}
                  styles={panel.config?.summaryStyles as Record<string, unknown> | undefined}
                  testIdPrefix="summary-tile"
                  onItemsChange={(items) => handleConfigChange({ summaryItems: items })}
                  onStylesChange={(styles) => handleConfigChange({ summaryStyles: styles })}
                />,
              )}
            {panel.type === 'devices' && (
              <CollapsibleSection title={t('dashboard.settings.columns')}>
                <ColumnsSection<DeviceListColumnKey>
                  allColumns={[...ALL_DEVICE_COLUMNS]}
                  labels={Object.fromEntries(ALL_DEVICE_COLUMNS.map((k) => [k, t(DEVICE_COLUMN_LABELS[k])])) as Record<DeviceListColumnKey, string>}
                  visibleColumns={(panel.config?.visibleColumns as DeviceListColumnKey[]) ?? [...ALL_DEVICE_COLUMNS]}
                  onChange={(cols) => handleConfigChange({ visibleColumns: cols })}
                  design={columnsDesignPopover('device-columns-design')}
                />
              </CollapsibleSection>
            )}
            {panel.type === 'devices' && summaryBadgeSection()}
            {panel.type === 'resource' && (
              <CollapsibleSection title={t('dashboard.settings.resource')}>
                <ResourceSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}
            {panel.type in MONITOR_PANEL_SECTION && (
              <CollapsibleSection title={t('dashboard.settings.monitor')}>
                <MonitorItemsSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}
            {SYSMETRICS_PANEL_TYPES.has(panel.type) && (
              <CollapsibleSection title={t('dashboard.settings.sysmetrics')}>
                <SysMetricsSection
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
              panel.type === 'facility-group' ||
              panel.type === 'facility-device') && (
              <CollapsibleSection title={t('dashboard.settings.facility')} defaultOpen={true}>
                <FacilitySection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}

            {/* SPEC-MODBUS-012: MODBUS Gateway 패널 6종 (게이트웨이 에이전트 + 가상 레지스터 맵은 unit) */}
            {MODBUS_PANEL_TYPES.has(panel.type) && (
              <CollapsibleSection title={t('dashboard.settings.modbusGateway')} defaultOpen={true}>
                <ModbusSettingsSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}

            {/* SPEC-DASHBOARD-002: 에이전트 상태 패널 (전체 타입 에이전트 재선택) */}
            {panel.type === 'agent-status' && (
              <CollapsibleSection title={t('dashboard.settings.agent')} defaultOpen={true}>
                <AgentStatusSettingsSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
                {/* 타일 목록 — 고를 것과 타일별 모양. 다이어그램 뷰에는 타일이 없다. */}
                {panel.config?.viewMode !== 'diagram' && (
                  <>
                    {/*
                      배지 — 무엇을 낼지 고르고 항목마다 모양을 정한다. 글자 설정은
                      표시 체크 옆에 둔다: 감춘 배지에는 걸 곳이 없으므로 표시할 때만 낸다.
                      색을 비워 두면 상태별 의미색(초록·빨강)이 그대로 산다.
                    */}
                    <div className="mt-3">
                      <div className="flex items-center gap-1.5">
                        <label className="flex items-center gap-2 text-xs text-(--color-text-secondary)">
                          <input
                            type="checkbox"
                            data-testid="agent-status-show-badges"
                            checked={panel.config?.showBadges !== false}
                            onChange={(e) =>
                              // 기본은 표시다 — 끌 때만 config 에 남긴다.
                              handleConfigChange({ showBadges: e.target.checked ? undefined : false })
                            }
                            className="h-3.5 w-3.5 accent-blue-600"
                          />
                          {t('dashboard.settings.propertiesGridOpt.showBadges')}
                        </label>
                        {panel.config?.showBadges !== false && (
                          <DesignPopover testId="badge-design">
                            <TextStyleFields
                              label={t('dashboard.settings.listPanel.badgeStyle')}
                              family={badgeFont.family}
                              size={badgeFont.size}
                              color={badgeFont.color}
                              weight={badgeFont.weight ?? 'inherit'}
                              align={badgeFont.align ?? 'inherit'}
                              sizePlaceholder={t('dashboard.chart.inherit')}
                              testIdPrefix="badge-font"
                              onChange={(patch) =>
                                handleConfigChange({ badge_font: mergeFont(badgeFont, patch) })
                              }
                            />
                          </DesignPopover>
                        )}
                      </div>
                      {panel.config?.showBadges !== false && (
                        <TileListEditor<AgentBadge>
                          all={AGENT_BADGES}
                          labelKeys={AGENT_BADGE_LABEL_KEYS}
                          items={readTileItems(panel.config?.badgeItems, AGENT_BADGES)}
                          styles={panel.config?.badgeStyles as Record<string, unknown> | undefined}
                          testIdPrefix="agent-badge"
                          onItemsChange={(items) => handleConfigChange({ badgeItems: items })}
                          onDesignChange={(item, next) =>
                            handleConfigChange({
                              badgeStyles: {
                                ...((panel.config?.badgeStyles as Record<string, unknown>) ?? {}),
                                [item]: next,
                              },
                            })
                          }
                        />
                      )}
                    </div>

                    {/* 공통 속성 — 모든 타일에 함께 걸린다. 타일별 설정이 이 위를 덮는다. */}
                    <div className="mt-3 flex items-center gap-1.5">
                      <span className="text-xs font-medium text-(--color-text-muted)">
                        {t('dashboard.settings.propertiesGridOpt.card')}
                      </span>
                      <DesignPopover testId="agent-status-common">
                        <TileDesignFields
                          testIdPrefix="agent-status-common"
                          design={{
                            label_font: panel.config?.tileLabelFont,
                            value_font: panel.config?.tileValueFont,
                            bg: panel.config?.tileBg as string | undefined,
                          }}
                          onChange={(next) =>
                            handleConfigChange({
                              tileLabelFont: next.label_font,
                              tileValueFont: next.value_font,
                              tileBg: next.bg,
                            })
                          }
                          onReset={() =>
                            handleConfigChange({
                              tileLabelFont: undefined,
                              tileValueFont: undefined,
                              tileBg: undefined,
                            })
                          }
                        />
                      </DesignPopover>
                    </div>

                    <div className="mt-3">
                      {/*
                        레이아웃(행·열 + 배치)은 팝업으로 연다 — 설정 컬럼에 격자를 늘
                        펼쳐 두면 아래 목록이 한 화면에서 밀려난다.
                      */}
                      <div className="flex items-center gap-1.5">
                        <span className="text-xs font-medium text-(--color-text-muted)">
                          {t('dashboard.settings.agentStatusOpt.statTiles')}
                        </span>
                        <DesignPopover
                          testId="stat-layout-popup"
                          label={t('dashboard.settings.agentStatusOpt.layout')}
                          width={384}
                        >
                          <TileGridEditor<StatTile>
                            grid={readTileGrid(panel.config?.statGrid, STAT_GRID_DEFAULT)}
                            items={readTileItems(panel.config?.statTiles, STAT_TILES)}
                            areas={placeTiles(
                              readTileItems(panel.config?.statTiles, STAT_TILES),
                              panel.config?.statTileAreas as Record<string, Partial<TileArea>> | undefined,
                              readTileGrid(panel.config?.statGrid, STAT_GRID_DEFAULT),
                              STAT_TILE_SIZE,
                            )}
                            labelKeys={STAT_TILE_LABEL_KEYS}
                            testIdPrefix="stat-layout"
                            onGridChange={(g) => handleConfigChange({ statGrid: g })}
                            onAreasChange={(areas) => handleConfigChange({ statTileAreas: areas })}
                            onMove={(item, dir) => {
                              const list = readTileItems(panel.config?.statTiles, STAT_TILES);
                              handleConfigChange({
                                statTiles: moveToPosition(list, item, list.indexOf(item) + 1 + dir),
                              });
                            }}
                            onReset={() =>
                              handleConfigChange({ statGrid: undefined, statTileAreas: undefined })
                            }
                          />
                        </DesignPopover>
                      </div>
                      <TileListEditor<StatTile>
                        all={STAT_TILES}
                        labelKeys={STAT_TILE_LABEL_KEYS}
                        items={readTileItems(panel.config?.statTiles, STAT_TILES)}
                        testIdPrefix="stat-tile"
                        styles={panel.config?.statTileStyles as Record<string, unknown> | undefined}
                        onItemsChange={(items) => handleConfigChange({ statTiles: items })}
                        onDesignChange={(item, next) =>
                          handleConfigChange({
                            statTileStyles: {
                              ...((panel.config?.statTileStyles as Record<string, unknown>) ?? {}),
                              [item]: next,
                            },
                          })
                        }
                      />
                    </div>
                    <div className="mt-3">
                      <div className="flex items-center gap-1.5">
                        <span className="text-xs font-medium text-(--color-text-muted)">
                          {t('dashboard.settings.agentStatusOpt.messageTiles')}
                        </span>
                        {/*
                          격자에 놓는 단위는 **묶음 카드**(외부/내부)다 — 카드 한 장이 값
                          셋을 담는다. 디자인도 카드 단위다: 타이틀은 묶음 이름, 값 글자와
                          색 규칙은 그 카드 안 값들에 함께 걸린다.
                        */}
                        <DesignPopover
                          testId="message-layout-popup"
                          label={t('dashboard.settings.agentStatusOpt.layout')}
                          width={384}
                        >
                          <TileGridEditor<MessageGroup>
                            grid={readTileGrid(panel.config?.messageGrid, MESSAGE_GRID_DEFAULT)}
                            items={readTileItems(panel.config?.messageGroups, MESSAGE_GROUPS)}
                            areas={placeTiles(
                              readTileItems(panel.config?.messageGroups, MESSAGE_GROUPS),
                              panel.config?.messageTileAreas as Record<string, Partial<TileArea>> | undefined,
                              readTileGrid(panel.config?.messageGrid, MESSAGE_GRID_DEFAULT),
                              MESSAGE_TILE_SIZE,
                            )}
                            labelKeys={MESSAGE_GROUP_LABEL_KEYS}
                            testIdPrefix="message-layout"
                            onGridChange={(g) => handleConfigChange({ messageGrid: g })}
                            onAreasChange={(areas) => handleConfigChange({ messageTileAreas: areas })}
                            onMove={(item, dir) => {
                              const list = readTileItems(panel.config?.messageGroups, MESSAGE_GROUPS);
                              handleConfigChange({
                                messageGroups: moveToPosition(list, item, list.indexOf(item) + 1 + dir),
                              });
                            }}
                            onReset={() =>
                              handleConfigChange({
                                messageGrid: undefined,
                                messageTileAreas: undefined,
                              })
                            }
                          />
                        </DesignPopover>
                      </div>
                      <TileListEditor<MessageTile>
                        all={MESSAGE_TILES}
                        labelKeys={MESSAGE_TILE_SETTING_LABEL_KEYS}
                        items={readTileItems(panel.config?.messageTiles, MESSAGE_TILES)}
                        testIdPrefix="message-tile"
                        styles={panel.config?.messageTileStyles as Record<string, unknown> | undefined}
                        onItemsChange={(items) => handleConfigChange({ messageTiles: items })}
                        onDesignChange={(item, next) =>
                          handleConfigChange({
                            messageTileStyles: {
                              ...((panel.config?.messageTileStyles as Record<string, unknown>) ?? {}),
                              [item]: next,
                            },
                          })
                        }
                      />
                    </div>
                  </>
                )}
              </CollapsibleSection>
            )}

            {/* SPEC-HEATMAP-PANEL-001 (MVP): 히트맵 설정 섹션. STAGE 1 은 자리표시 안내만 렌더한다
                (전체 에디터 — store 태그/센서 좌표/색상표/IDW — 는 STAGE 2 T7 에서 채운다). */}
            {panel.type === 'heatmap' && (
              <CollapsibleSection title={t('dashboard.settings.heatmap')} defaultOpen={true}>
                <HeatmapSettingsSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}

            {/* 데이터 소스 섹션(StoreSourceSection)은 좌측 프리뷰 아래에 있다(SPEC-WEB-005). */}
            {/* 차트 타입별 세부 설정 (SPEC-CHART-001 §4.2.2 / REQ-M5-03) */}
            {panel.type === 'stat' && (
              <CollapsibleSection title={t('dashboard.settings.statSettings')}>
                <StatChartSection
                  panel={panel}
                  onConfigChange={(c) => handleConfigChange(c)}
                />
              </CollapsibleSection>
            )}
            {panel.type === 'graph-chart' && (
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
              - 그 외 패널은 그룹 목록에서 고른 뒤 AccentGroupControls 로 색을 정한다.
              - 목록형 패널(플로우 현황·에이전트 현황·디바이스 목록)은 내지 않는다 —
                타이틀·컬럼·요약 배지의 모양을 각자의 디자인 팝오버가 갖게 되면서
                남은 항목이 없다. 하나뿐인 항목을 위해
                고르기 → 편집 두 단계를 남겨 두면 빈 껍데기가 된다.
              - 에이전트 상태도 내지 않는다 — 이 패널은 accentElements 를 **읽지 않아**
                그룹을 골라 색을 정해도 화면이 바뀌지 않는 죽은 컨트롤이었다.
              - 디바이스 상태(속성 그리드)도 내지 않는다 — 카드 조각별 디자인이 색을
                갖게 되면서 악센트 labels/borders 와 자리가 겹친다.
              - 통계도 내지 않는다 — 그룹 4개 중 header/badges/table 은 StatPanel 이
                읽지 않는 죽은 컨트롤이었고, 살아 있던 `_base`(= panelColor)는 패널
                옵션으로 올라갔다(SPEC-CHART-003 §5 D1). 남는 항목이 없다.
            */}
            {panel.type === 'flows' ||
            panel.type === 'agents' ||
            panel.type === 'devices' ||
            panel.type === 'properties-grid' ||
            panel.type === 'stat' ||
            panel.type === 'agent-status' ? null : panel.type === 'ac-control' ? (
              <CollapsibleSection title={t('dashboard.settings.style')} defaultOpen={true}>
                <AcControlStyleSection
                  accentElements={accentElements}
                  config={panel.config ?? {}}
                  onAccentChange={(elements) => handleConfigChange({ accentElements: elements })}
                  onConfigChange={(patch) => handleConfigChange(patch)}
                />
              </CollapsibleSection>
            ) : (
              <CollapsibleSection title={t('dashboard.settings.style')} defaultOpen={true}>
                {/*
                  악센트 그룹 고르기. 종전에는 미리보기의 목업 영역을 클릭했는데,
                  미리보기가 실제 패널로 바뀌면서 클릭 가능한 영역 지도가 사라졌다.
                  그룹 이름표는 타입별로 이미 정의되어 있어(accentLabelKeys) 그대로 쓴다.
                */}
                <AccentGroupPicker
                  labelKeys={accentLabelKeys}
                  selected={selectedGroup}
                  effectiveColor={effectiveColor}
                  onSelect={setSelectedGroup}
                />
                {selectedGroup && (
                  <AccentGroupControls
                    selected={selectedGroup}
                    labelKeys={accentLabelKeys}
                    accentElements={accentElements}
                    inheritedColor={panelColor}
                    onChange={(elements) => handleConfigChange({ accentElements: elements })}
                  />
                )}
              </CollapsibleSection>
            )}

    </>
  );
  // line/gauge/modbus 미리보기는 디바운스된 previewPanel 로 렌더한다(잦은 편집 재조회 억제).
  // (panel 은 non-null 로 좁혀졌으므로 previewPanel 부재 시 panel 로 폴백해 항상 non-null.)
  // 히트맵은 예외 — 시각 설정(도면 배경/색상/IDW)은 재조회를 트리거하지 않으므로 즉시 draft
  // config(panel)로 렌더해 도면 배경이 지연 없이 반영된다(아래 heatmap 렌더 블록 참고).
  const previewRenderPanel = previewPanel ?? panel;
  // SPEC-CHART-002 M6.3 — stat / gauge 도 같은 선례를 따른다: 신규 경로가 실제로 값을 낼 수
  // 있을 때만 **실제 패널**을 draft config 로 렌더하고, 그 밖에는 기존 미리보기를 유지한다.
  // 판정 규칙은 각 패널이 스스로 쓰는 규칙과 같아야 한다 — 미리보기와 실제 렌더가 서로 다른
  // 조건으로 갈리면 "설정 화면에서는 보이는데 대시보드에서는 안 보인다" 가 된다.
  const previewChartConfig = previewRenderPanel.config ?? {};
  // SPEC-TSDB-002 §2.3 [U3]: 두 미리보기 게이트의 **소스 항**을 계약에 위임한다.
  // 두 게이트는 같은 `previewRenderPanel.config` 에서 파생하므로 바인딩도 하나면 된다.
  // **부가 조건은 각 게이트에 그대로 남는다** — 패널 타입, 라인의 `tag_filters` 대안,
  // stat 의 `series_reduce` 요구는 소스 종류와 무관한 게이트 고유 조건이다.
  // 활성 판정은 소스 **목록 전체**를 본다. 단일 축 해석기는 최상위 `store_source` 만 보므로,
  // 시리즈 선택이 `sources[i].store_source` 에 쓰이는 지금은 고른 뒤에도 게이트가 거짓으로
  // 남아 미리보기가 합성 샘플에 머물렀다.
  // 소스 종류를 **열거하지 않는다**. 'store' 만 보던 때는 TSDB 패널이, 'store'|'tsdb' 만
  // 보던 때는 sysmetrics 패널이 영원히 합성 미리보기에 머물렀다 — 종류가 늘 때마다 이
  // 자리를 고쳐야 하는 것이 결함의 원인이었다. 계약의 술어(`isPanelSeriesSource` =
  // 채널이 아니고 활성)를 그대로 쓰면 다음 종류에서 같은 일이 반복되지 않는다.
  //
  // previewRealData 가 거짓이면 실제 렌더를 쓰지 않고 합성 미리보기로 내려간다.
  const isPreviewStoreActive = previewRealData && isPanelSeriesActive(previewChartConfig);
  // Store 라인 차트에서 실제 데이터 미리보기를 쓸지 판정한다. 시리즈가 하나도 선택되지
  // 않았거나 채널 모드면 실제 패널은 빈 상태만 보여주므로, 스타일을 확인할 수 있는
  // 합성 미니 프리뷰를 유지한다.
  const isStoreLinePreview =
    previewRenderPanel.type === 'graph-chart' && isPreviewStoreActive;
  // stat / bar / pie 라이브 미리보기 게이트.
  //
  // 이 3종에는 합성 미니 프리뷰가 없다. 그래서 "실패널을 렌더하지 않는다" = "미리보기 영역이
  // 빈 화면" 이며, 시리즈 소스를 고르고도 아무것도 안 보이는 상태가 된다. 판정을 **소스 종류
  // 축 하나**로 좁혀, 채널이 아닌 소스를 고른 순간부터 실패널을 그린다.
  //
  // 시리즈 미선택도 렌더한다 — 패널이 스스로 빈 상태를 표시하며, 그것이 대시보드에서 보게 될
  // 실제 모습이다(히트맵 미리보기와 같은 방식). 조회는 소스가 비활성이면 idle 이므로
  // (`usePanelSeriesData`) 빈 시리즈로 요청이 나가지도 않는다.
  //
  // `series_reduce` 는 게이트에서 빠졌다. StatPanel·BarChartPanel·PieChartPanel 은 대표값
  // 없이도(레거시 경로) 시리즈 소스 데이터를 그리므로, 대표값 유무로 미리보기를 끄면 실제
  // 렌더와 미리보기가 서로 다른 조건으로 갈린다 — "설정에서는 안 보이는데 대시보드에서는
  // 보인다" 가 된다. 게이지는 예외로 남는다(합성 미니 프리뷰가 있고, 대표값이 값 소스
  // 진리표의 축 자체다 — `resolveGaugeValueSource`).
  //
  // 실제 조회를 끈 경우(previewRealData 해제)는 미리보기가 없다 — 그 토글의 의미가
  // "실제 질의를 내지 않는다" 이므로 대신 보여줄 화면이 없다.
  //
  // 소스 항은 사라졌다. 종전 조건은 "채널이 아닐 것" 이었는데 채널이 빠지면서 언제나
  // 참이다 — 시리즈 미선택 패널도 종전처럼 **실패널의 빈 상태**를 그린다(빈 화면이 아니다).
  const isSeriesSourcePreview = previewRealData;
  const isStoreStatPreview = previewRenderPanel.type === 'stat' && isSeriesSourcePreview;
  // gauge: 값 소스 판정의 단일 정본(`resolveGaugeValueSource`)을 그대로 쓴다. 레거시 경로가
  // 이기는 동안에는 합성 샘플값 미니 프리뷰가 그대로 남는다(레거시는 실제 값이 없을 수 있다).
  const isStoreGaugePreview =
    previewRenderPanel.type === 'gauge' &&
    resolveGaugeValueSource(gaugeValueSourceFlags(previewChartConfig)) === 'store-source';
  // 종횡비 보존 미리보기(라운드 게이지, 작은 accent device/ac/hvac): 높이를 채우고 폭은
  // 종횡비로 파생한다. previewZoom(0.5~2.0)이 곱해진다(±/Ctrl+휠/더블클릭).
  /** 모든 미리보기는 스테이지 상자를 그대로 채운다 — 사이징 정본은 스테이지 한 곳이다. */
  const PREVIEW_CHILD_STYLE: React.CSSProperties = { width: '100%', height: '100%' };

  // 미리보기 스테이지 — 패널을 대시보드에서의 실제 픽셀 크기로 렌더한 뒤 통째로
  // 축소한다. 미리보기 크기에 맞춰 다시 레이아웃하면 반응형 재배치가 일어나
  // 대시보드와 다른 화면이 되기 때문이다(previewStage.ts 주석 참조).
  const previewStage = computePreviewStage({
    w: effectiveSize?.w ?? 0,
    h: effectiveSize?.h ?? 0,
    cell: gridCell,
    areaW: fitSize.w,
    areaH: fitSize.h,
    mode: previewFillMode,
    zoom: previewZoom,
  });
  const previewBox =
    previewStage !== null ? { w: previewStage.screenW, h: previewStage.screenH } : null;

  const previewSlot = (
    // fit 컨테이너: 남은 미리보기 영역을 세로로 가득(min-h-0 flex-1) 차지하고 자식을 양축
    // 가운데 정렬한다. ref 로 실측하여 fit 모드가 종횡비 보존 contain 을 결정론적으로 계산한다.
    // 크롬 옵션은 draft config 로 전파해 타이틀 바 토글이 미리보기에 즉시 반영되게 한다.
    <PanelChromeProvider config={panel.config}>
    <div
      ref={setFitContainer}
      className="relative flex min-h-0 flex-1 items-center justify-center overflow-hidden"
    >
      {/* 실제 패널처럼 크기를 조절한다. 그리드(칼럼 수·셀·마진)와 가이드 라인 표시는
          현재 대시보드 설정을 그대로 쓴다. 레이아웃이 없는 패널(대시보드 미배치)은
          기준 크기가 없으므로 손잡이를 내지 않는다. */}
      {effectiveSize !== null && panelAspect !== undefined && (
        <PanelResizeOverlay
          size={effectiveSize}
          box={previewBox}
          aspect={panelAspect}
          cols={gridCols}
          cell={gridCell}
          minW={layoutItem?.minW}
          minH={layoutItem?.minH}
          onChange={setDraftSize}
        />
      )}

      {/*
        그리드 가이드 — 대시보드와 같이 패널 **뒤**에 깔린다. 표시 여부는 대시보드
        설정을 따른다. 위에 그리면 패널의 실제 모양(카드 배경·테두리)이 가려진다.
      */}
      {showGridLines && previewStage !== null && (
        <PanelGridBackdrop
          areaW={fitSize.w}
          areaH={fitSize.h}
          boxW={previewStage.screenW}
          boxH={previewStage.screenH}
          cellW={previewStage.cellW}
          cellH={previewStage.cellH}
          gapX={previewStage.gapX}
          gapY={previewStage.gapY}
        />
      )}

      {/*
        스테이지 — 패널을 대시보드에서의 실제 픽셀 크기로 렌더한 뒤 통째로 축소한다.
        타입마다 다른 상자 크기를 쓰면 프레임·그리드와 어긋나므로 사이징은 여기 한 곳이
        정본이고, 각 미리보기는 이 상자를 100% 로 채운다.

        실측 전(첫 페인트)에는 영역을 그대로 채운다 — 배율을 알 수 없기 때문이다.
      */}
      <div
        data-testid="preview-stage"
        className="relative flex min-h-0 shrink-0 flex-col overflow-hidden"
        style={
          previewStage !== null
            ? {
                width: `${previewStage.pxW}px`,
                height: `${previewStage.pxH}px`,
                transform: `scale(${previewStage.scaleX}, ${previewStage.scaleY})`,
                transformOrigin: 'center',
              }
            : previewFillMode === 'fill'
              ? // 실측 전이라도 채움은 영역을 채운다 — 모드의 뜻을 그대로 지킨다.
                { width: '100%', height: '100%' }
              : {
                  // 맞춤은 배율을 몰라도 비율은 안다. 영역을 통째로 늘이면 첫 페인트가
                  // 찌그러져 보이므로 패널 비율을 지킨다.
                  height: '100%',
                  maxWidth: '100%',
                  maxHeight: '100%',
                  aspectRatio: `${panelAspect ?? 1.5} / 1`,
                }
        }
        onWheel={handlePreviewWheel}
      >
            {/*
              실제 패널 미리보기 — 대시보드와 **같은 렌더러**(renderDashboardPanel)를 쓴다.

              종전에는 타입별로 손으로 그린 미니 목업을 그렸다. 목업은 컬럼도 값도 실제와
              달라(가짜 행 sample-1/sample-2, 실제엔 없는 컬럼) 미리보기를 보고 판단할 수
              없었다. 목업이 겸하던 "영역 클릭 → 악센트 그룹 선택"은 스타일 섹션의 그룹
              목록으로 옮겼다 — 실제 패널은 이미 accentElements 를 읽어 색을 칠하므로
              고른 색은 이 미리보기에 그대로 나타난다.

              채움(fill)은 영역을 가득 채우고, 맞춤(fit)은 패널의 그리드 비율을 지킨다.
              wheel 핸들러는 wrapper 에 부여한다(Ctrl+휠 로만 동작 — 기본 스크롤 보존).
            */}
            {REAL_PANEL_PREVIEW_TYPES.has(panel.type) && (
              // 대시보드에서의 실제 픽셀 크기로 렌더한 뒤 transform 으로 축소한다.
              // 작은 상자에 다시 레이아웃하면 반응형 재배치가 일어나 칼럼 수·줄바꿈이
              // 대시보드와 달라진다 — 그러면 미리보기의 의미가 없다.
              //
              // 영역 실측 전(첫 페인트)에는 스테이지를 만들 수 없으므로 종횡비만 맞춰
              // 그린다. 실측되는 즉시 위 경로로 넘어간다.
              <div
                data-testid="real-panel-preview"
                className="flex min-h-0 shrink-0 flex-col overflow-hidden"
                style={PREVIEW_CHILD_STYLE}
                onWheel={handlePreviewWheel}
              >
                {/*
                  미리보기 안에서만 직접 조작을 켠다 — 카드를 끌어 옮기는 동작은 대시보드에
                  놓인 패널에서는 패널 자체를 끄는 동작과 부딪힌다.
                */}
                <PanelEditContext.Provider value={true}>
                  {renderDashboardPanel(
                    previewRenderPanel,
                    previewFlows,
                    previewMetrics as Record<string, unknown> | undefined,
                    previewRefreshMs,
                    () => ({ onConfigChange: patchConfig, onTitleChange: setTitle }),
                  )}
                </PanelEditContext.Provider>
              </div>
            )}
            {panel.type === 'gauge' && (
              <div
                // Store 실데이터 경로에서는 실제 GaugePanel 이 게이지 배열을 그리므로 세로를
                // 채울 flex 컨테이너가 필요하다(라인 차트 프리뷰와 같은 이유). 미니 프리뷰는
                // 자체 h-full 이라 두 경로 모두 안전하다.
                className="flex min-h-0 flex-col"
                data-testid="gauge-preview-wrapper"
                style={PREVIEW_CHILD_STYLE}
                onWheel={handlePreviewWheel}
              >
                {/* 값 글자를 끌어 자리를 잡는다. 두 미리보기 경로(실 패널 · 미니)가 같은
                    레이어를 쓰므로 어느 쪽이 떠 있어도 조작이 같다. */}
                {/* 드래그 배치는 패널 자신이 갖는다 — 미리보기는 항상 편집(`forceEdit`). */}
                <>
                  {isStoreGaugePreview ? (
                    // Store 소스 + 대표값 지정: 합성 샘플값이 아니라 **실제 패널**을 draft
                    // config 로 렌더한다(§2.11 [O1] / M6.3, 라인 차트 isStoreLinePreview 선례).
                    <GaugePanel
                      panelId={previewRenderPanel.id}
                      title={previewRenderPanel.title}
                      config={previewRenderPanel.config ?? {}}
                      onConfigChange={patchConfig}
                      onTitleChange={() => {}}
                      forceEdit
                    />
                  ) : (
                    <GaugeMiniPreview
                      panel={previewRenderPanel}
                      onConfigChange={patchConfig}
                    />
                  )}
                </>
              </div>
            )}
            {/*
              stat / bar / pie 라이브 미리보기. 이 3종에는 합성 미니 프리뷰가 없으므로 실패널을
              draft config 로 렌더한다(히트맵·MODBUS 미리보기와 같은 방식 — 패널이 자체 빈
              상태를 표시하므로 blank 가 되지 않는다). 게이트는 `isSeriesSourcePreview` 하나다.
            */}
            {isStoreStatPreview && (
              <div
                className="flex min-h-0 flex-col"
                data-testid="stat-preview-wrapper"
                style={PREVIEW_CHILD_STYLE}
                onWheel={handlePreviewWheel}
              >
                <StatPanel
                  panelId={previewRenderPanel.id}
                  title={previewRenderPanel.title}
                  config={previewRenderPanel.config ?? {}}
                  // 미리보기에서 요소를 직접 옮기고 크기·글자 스타일을 바꾼다
                  // (SPEC-CHART-004). 게이지와 같은 형태다.
                  onConfigChange={patchConfig}
                  forceEdit
                />
              </div>
            )}
            {previewRenderPanel.type === 'bar-chart' && isSeriesSourcePreview && (
              <div
                // 패널 루트가 flex-1 로 부모 높이를 채우므로 wrapper 가 flex 여야 한다
                // (라인 차트 프리뷰와 같은 이유 — plain block 이면 flex-1 이 no-op 이 되어
                // 콘텐츠 높이로 축소되고 차트 영역이 0-height 로 붕괴한다).
                className="flex min-h-0 flex-col"
                data-testid="bar-chart-preview-wrapper"
                style={PREVIEW_CHILD_STYLE}
                onWheel={handlePreviewWheel}
              >
                <BarChartPanel
                  panelId={previewRenderPanel.id}
                  title={previewRenderPanel.title}
                  config={previewRenderPanel.config ?? {}}
                  // 미리보기에서 그림·범례를 직접 옮긴다(SPEC-CHART-005).
                  onConfigChange={patchConfig}
                  forceEdit
                />
              </div>
            )}
            {previewRenderPanel.type === 'table' && isSeriesSourcePreview && (
              <div
                // 다른 실패널 미리보기와 같은 이유로 flex 컨테이너여야 한다 — 테이블 패널
                // 루트가 flex-1 이라 plain block 안에서는 높이가 콘텐츠로 붕괴한다.
                className="flex min-h-0 flex-col"
                data-testid="table-preview-wrapper"
                style={PREVIEW_CHILD_STYLE}
                onWheel={handlePreviewWheel}
              >
                <TablePanel
                  panelId={previewRenderPanel.id}
                  title={previewRenderPanel.title}
                  // 폭 조절은 시각 설정이라 재조회를 트리거하지 않는다 — 디바운스된
                  // previewPanel 이 아니라 draft(panel)로 렌더해야 드래그가 지연 없이
                  // 따라온다(히트맵 센서 배치와 같은 이유).
                  config={panel.config ?? {}}
                  onColumnsChange={(columns) => handleConfigChange({ columns })}
                />
              </div>
            )}
            {previewRenderPanel.type === 'pie-chart' && isSeriesSourcePreview && (
              <div
                className="flex min-h-0 flex-col"
                data-testid="pie-chart-preview-wrapper"
                // 파이는 정사각에 가까운 편이 실제 배치를 가늠하기 좋다(게이지 1:1 과 차트
                // 16:9 사이). 채움 모드에서는 종횡비를 무시하고 영역을 가득 채운다.
                style={PREVIEW_CHILD_STYLE}
                onWheel={handlePreviewWheel}
              >
                {/* 드래그 배치는 패널 자신이 갖는다(대시보드와 같은 구현). 미리보기는
                    `forceEdit` 로 토글 없이 항상 편집이다 — 끌 수 있다는 사실이 화면
                    맥락으로 이미 드러나 있고, 토글이 미리보기를 가린다.
                    config 는 디바운스된 previewPanel 이 아니라 draft(panel)를 쓴다 —
                    시각 설정이라 재조회를 트리거하지 않고, 드래그가 지연 없이 따라온다. */}
                <PieChartPanel
                  panelId={previewRenderPanel.id}
                  title={previewRenderPanel.title}
                  config={panel.config ?? {}}
                  onConfigChange={patchConfig}
                  forceEdit
                />
              </div>
            )}
            {panel.type === 'graph-chart' && (
              <div
                // 실제 LineChartPanel 을 렌더할 때 높이를 물려주려면 flex 컨테이너여야 한다
                // (패널 루트가 flex-1 로 부모 높이를 채운다). 미니 프리뷰는 자체 h-full 이라
                // 두 경로 모두 안전하다.
                className="flex min-h-0 flex-col"
                data-testid="line-chart-preview-wrapper"
                style={PREVIEW_CHILD_STYLE}
                onWheel={handlePreviewWheel}
              >
                {isStoreLinePreview ? (
                  // Store 모드 + 시리즈 선택됨: 합성 데이터가 아니라 **실제 패널**을 draft
                  // config 로 렌더한다. 실제 조회 결과·축·범례·옵션이 그대로 보인다
                  // (히트맵 미리보기와 같은 방식). 시리즈 미선택/채널 모드는 기존 미니
                  // 프리뷰가 스타일 확인용 합성 데이터를 그린다.
                  <LineChartPanel
                    panelId={previewRenderPanel.id}
                    title={previewRenderPanel.title}
                    config={previewRenderPanel.config ?? {}}
                    // 미리보기에서 범례를 끌어 배치한다(파이·히트맵과 같은 규칙).
                    onConfigChange={patchConfig}
                    forceEdit
                  />
                ) : (
                  <LineChartMiniPreview panel={previewRenderPanel} onConfigChange={patchConfig} />
                )}
              </div>
            )}
            {/*
              SPEC-MODBUS-012: MODBUS 패널 프리뷰. 실제 패널 컴포넌트를 draft config 로 렌더한다.
              패널이 미설정/원격/로딩/에러/빈 상태를 자체 ModbusNotice 로 표시하므로 blank 가 되지 않는다.
              (TargetContext 는 기본 LOCAL_TARGET, react-query 는 앱 전역 Provider 를 사용한다.)
            */}
            {MODBUS_PANEL_TYPES.has(panel.type) && (
              <div
                style={PREVIEW_CHILD_STYLE}
                onWheel={handlePreviewWheel}
              >
                <ModbusPanelPreview panel={previewRenderPanel} />
              </div>
            )}
            {/*
              SPEC-HEATMAP-PANEL: 히트맵 프리뷰. 실제 HeatmapPanel 을 즉시 draft config(panel)로
              렌더한다 — 도면 배경/색상/IDW 등 시각 설정은 store 재조회를 트리거하지 않으므로
              debounce 없이 즉시 반영해야 한다(도면 이미지 첨부 즉시 배경 표시). store 데이터
              재조회는 HeatmapPanel 내부 useStoreChartData 의 pollKey(agent/namespace/window/
              interval/aggregation/series·tags)로 스로틀되며, 시각 필드는 pollKey 에 포함되지
              않아 재조회 폭주가 없다(shallow-merge 로 store_source 참조도 안정적).
              데이터/좌표 미설정 시 패널이 자체 빈상태 안내를 표시하므로 blank 가 되지 않는다.
              onConfigChange(draft writer) + forcePlacement 로 프리뷰에서 센서 마커 드래그 배치를
              활성화한다(대시보드 편집모드에 의존하지 않음). 드래그 → sensor_positions 를 draft 에
              쓰고 프리뷰가 재렌더된다. @spec SPEC-PANEL-SETTINGS-001 (heatmap 시리즈 위치)
            */}
            {panel.type === 'heatmap' && (
              <div
                // flex 컨테이너로 지정 — HeatmapPanel 의 flex-1 루트가 부모(이 wrapper)의 높이를
                // 채우려면 부모가 flex 여야 한다. plain block 이면 flex-1 이 no-op 이 되어 패널이
                // '콘텐츠 높이'로 축소된다 → 데이터 0개일 때 canvas 가 없어 0-height 로 붕괴하고
                // 배경/마커가 안 보인다("완전히 빈 영역" 버그). 높이는 style(previewFillStyle/
                // measuredFitStyle)이 제공하고, flex-col 로 flex-1 이 그 높이를 채운다.
                data-testid="heatmap-preview-wrapper"
                className="relative flex min-h-0 flex-col"
                // 스테이지가 이 패널의 실제 대시보드 크기를 잡아 주므로 여기서는 채우기만 한다.
                // 히트맵은 도면 종횡비로 스테이지를 레터박스하므로(stage.ts), 상자 비율이
                // 실제와 다르면 여백이 얼마나 생길지 확인할 방법이 없다.
                style={PREVIEW_CHILD_STYLE}
                onWheel={handlePreviewWheel}
              >
                <HeatmapPanel
                  panelId={panel.id}
                  title={panel.title}
                  config={panel.config}
                  onConfigChange={(c) => handleConfigChange(c)}
                  forcePlacement
                />
              </div>
            )}
      </div>
    </div>
    </PanelChromeProvider>
  );
  const dataSourceSlot = dataSourceBelowPreview ? (
    <div data-testid="panel-settings-data-source" className="shrink-0">
      <CollapsibleSection title={t('dashboard.settings.dataSource')}>
        {/* Store/TSDB 토글 + 기존 편집기 + 공용 StoreEntryTable 선택 surface.
            @spec SPEC-PANEL-SETTINGS-001 (T4/T6/T7) */}
        <PanelSettingsDataSource
          panel={panel}
          onConfigChange={(c) => handleConfigChange(c)}
        />
        {/* 실제 데이터 적용 여부 — **데이터 소스 설정** 안에 둔다.
            미리보기 영역에 두면 "보기 방식" 처럼 읽히지만, 실제로는 소스에
            질의를 낼지 말지를 정하는 조회 옵션이다. 소스가 활성일 때만
            의미가 있으므로 그때만 노출한다. */}
        {isPanelSeriesActive(previewChartConfig) && (
            <label
              data-testid="preview-real-data-toggle"
              className="mt-2 flex items-center gap-1 text-xs text-(--color-text-muted)"
            >
              <input
                type="checkbox"
                checked={previewRealData}
                onChange={(e) => setPreviewRealData(e.target.checked)}
              />
              <span>{t('dashboard.settings.preview.previewRealData')}</span>
            </label>
          )}
      </CollapsibleSection>
    </div>
  ) : null;


  // 모달 → 페이지: 배경 오버레이 제거, AppLayout 콘텐츠 영역을 채우는 전체화면
  // 페이지 컨테이너로 렌더한다. 내부 2컬럼 레이아웃/스크롤/하단 고정 푸터는 그대로 유지된다.
  return (
    <div className="flex h-full w-full flex-col">
      <div
        className="flex h-full w-full flex-col overflow-hidden rounded-2xl border border-(--color-border-default) bg-(--color-bg-surface)"
        aria-labelledby="panel-settings-dialog-title"
      >
        {/* 헤더 */}
        <div className="flex shrink-0 items-center justify-between px-5 pt-4 pb-3">
          <div className="flex items-center gap-2">
            {/* 페이지 뒤로가기: 대시보드로 복귀 */}
            <button
              type="button"
              onClick={onClose}
              className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary)"
              aria-label={t('dashboard.settings.closeAria')}
            >
              <ArrowLeft className="h-4.5 w-4.5" />
            </button>
            <h2
              id="panel-settings-dialog-title"
              className="text-base font-semibold text-(--color-text-primary)"
            >
              {t('dashboard.settings.title')}
            </h2>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary)"
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
        <PanelSettingsShell
          options={optionsSlot}
          preview={previewSlot}
          dataSource={dataSourceSlot}
          previewCollapsed={previewCollapsed}
          setPreviewCollapsed={setPreviewCollapsed}
          previewFillMode={previewFillMode}
          setPreviewFillMode={setPreviewFillMode}
          dataSourceBelowPreview={dataSourceBelowPreview}
          leftWidth={leftWidth}
          isDragging={isDragging}
          setIsDragging={setIsDragging}
          previewRatio={previewRatio}
          previewSplitterDragging={previewDragging}
          onPreviewSplitterMouseDown={() => setPreviewDragging(true)}
          previewZoom={previewZoom}
          zoomIn={zoomIn}
          zoomOut={zoomOut}
          zoomReset={zoomReset}
          previewZoomMin={PREVIEW_ZOOM_MIN}
          previewZoomMax={PREVIEW_ZOOM_MAX}
          onResetLayout={
            panel?.type === 'graph-chart' && layoutDirty ? resetChartLayout : undefined
          }
          t={t}
        />

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
            data-testid="panel-settings-apply"
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
  design,
}: {
  panel: PanelConfig;
  onTitleChange: (title: string) => void;
  /** 글자 모양 배지. 축·범례와 같은 자리에 접는다 — 자주 고치는 것은 제목 글자뿐이다. */
  design?: React.ReactNode;
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
      <div className="mb-1.5 flex items-center gap-1.5">
        <label className="block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.titleLabel')}
        </label>
        {design}
      </div>
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
  design,
}: {
  allColumns: T[];
  labels: Record<T, string>;
  visibleColumns: T[];
  onChange: (cols: T[]) => void;
  /**
   * 라벨 옆에 접어 두는 디자인 팝오버. 타이틀과 같은 자리·같은 조작이라
   * "디자인은 설정 옆에 접혀 있다"를 한 번만 배우면 된다.
   */
  design?: React.ReactNode;
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
      <div className="mb-1.5 flex items-center gap-1.5">
        <label className="text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.visibleColumns')}
        </label>
        {design}
      </div>
      <div className="space-y-1">
        {allColumns.map((key) => (
          <label
            key={key}
            className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-(--color-bg-elevated)"
          >
            <input
              type="checkbox"
              data-testid={`column-toggle-${key}`}
              checked={visibleColumns.includes(key)}
              onChange={() => toggle(key)}
              className="h-4 w-4 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500"
            />
            <span className="text-sm text-(--color-text-primary)">{labels[key]}</span>
          </label>
        ))}
      </div>
    </div>
  );
}

/** 리소스 패널 전용 설정 (필드 + 그리드 열 수) */
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
                className="h-4 w-4 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500"
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
 * 에이전트(xsfm)를 고르고, 패널 타입에 따라 라인/역사/기기 대상을 고른다.
 * 대상 조회는 기존 useStations / useXsfmDevices 를 재사용한다(UB-001).
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
    () => (agentsResult?.data ?? []).filter((a) => a.type === 'xsfm'),
    [agentsResult],
  );

  const { data: stations, isLoading: stationsLoading } = useStations(agentId);
  const { data: devices, isLoading: devicesLoading } = useXsfmDevices(agentId);
  const { data: groups, isLoading: groupsLoading } = useGroups(agentId);

  // 패널 타입별 대상 키 / 현재 값 / i18n 라벨. 그룹(facility-group)은 groupId 로 단일 그룹을 지정한다.
  const targetKey: 'deviceId' | 'station' | 'line' | 'groupId' =
    panel.type === 'facility-device'
      ? 'deviceId'
      : panel.type === 'facility-station'
        ? 'station'
        : panel.type === 'facility-group'
          ? 'groupId'
          : 'line';
  const currentTarget = (panel.config?.[targetKey] as string | undefined) ?? '';
  const targetLabelKey =
    panel.type === 'facility-device'
      ? 'dashboard.settings.device'
      : panel.type === 'facility-station'
        ? 'dashboard.settings.station'
        : panel.type === 'facility-group'
          ? 'dashboard.settings.group'
          : 'dashboard.settings.line';
  const targetPlaceholderKey =
    panel.type === 'facility-device'
      ? 'dashboard.settings.selectDevice'
      : panel.type === 'facility-station'
        ? 'dashboard.settings.selectStation'
        : panel.type === 'facility-group'
          ? 'dashboard.settings.selectGroup'
          : 'dashboard.settings.selectLine';

  const targetOptions: { value: string; label: string }[] = useMemo(() => {
    if (panel.type === 'facility-device') {
      return (devices ?? []).map((d) => ({ value: d.device_id, label: d.name || d.device_id }));
    }
    if (panel.type === 'facility-station') {
      return (stations ?? []).map((s) => ({ value: s.station, label: s.display_name || s.station }));
    }
    if (panel.type === 'facility-group') {
      return (groups ?? []).map((g) => ({
        value: g.id,
        label: `${g.name} · ${t(`dashboard.facility.group.type.${g.type}`)} · ${g.member_count}`,
      }));
    }
    const lines = Array.from(new Set((stations ?? []).map((s) => s.line).filter(Boolean)));
    return lines.map((l) => ({ value: l, label: l }));
  }, [panel.type, devices, stations, groups, t]);

  const targetLoading =
    panel.type === 'facility-device'
      ? devicesLoading
      : panel.type === 'facility-group'
        ? groupsLoading
        : stationsLoading;

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

      {/* 대상(라인/그룹/기기) 선택 */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t(targetLabelKey)}
        </label>
        <select
          value={currentTarget}
          data-testid="facility-target-select"
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

      {/* 라인 패널 전용 표시 옵션(SPEC-FACILITY-DASHBOARD-001 후속, config-only UB-003). */}
      {panel.type === 'facility-line' && (
        <FacilityLineDisplayOptions panel={panel} onConfigChange={onConfigChange} />
      )}

      {/* 역사/그룹 패널 공용 표시 옵션(config-only UB-003). 그룹 패널은 역사 패널을 일반화한 것이라 동일 옵션. */}
      {(panel.type === 'facility-station' || panel.type === 'facility-group') && (
        <FacilityStationDisplayOptions panel={panel} onConfigChange={onConfigChange} />
      )}
    </div>
  );
}

/**
 * 에이전트 상태 패널 전용 설정 (SPEC-DASHBOARD-002).
 *
 * AddPanelDialog 의 AgentStatusAgentStep 을 미러링한다: 전체 타입(필터 없음) 에이전트를
 * 재선택 → config.agentId. 변경은 onConfigChange 로 draftConfig 에만 기록한다(적용 전까지 미반영).
 */
function AgentStatusSettingsSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const agentId = (panel.config?.agentId as string | undefined) ?? '';
  // 출력 형식(SPEC-DASHBOARD-003 REQ-06): 미설정/미인식 값은 기본 'tile'(하위호환).
  const viewMode = panel.config?.viewMode === 'diagram' ? 'diagram' : 'tile';

  const { data: agentsResult } = useAgents();
  // 타입 필터 없음 — 전체 연결 에이전트를 제시한다(모든 타입 대상).
  const agents = useMemo(() => agentsResult?.data ?? [], [agentsResult]);

  return (
    <div className="space-y-3">
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.agent')}
        </label>
        <select
          value={agentId}
          data-testid="agent-status-settings-select"
          onChange={(e) => onConfigChange({ agentId: e.target.value })}
          className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        >
          <option value="">{t('dashboard.settings.selectAgent')}</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name} ({a.type})
            </option>
          ))}
        </select>
      </div>

      {/* 출력 형식 선택기(REQ-06): agent-picker <select> 미러링. */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.agentStatus.viewMode')}
        </label>
        <select
          value={viewMode}
          data-testid="agent-status-viewmode-select"
          onChange={(e) => onConfigChange({ viewMode: e.target.value })}
          className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        >
          <option value="tile">{t('dashboard.agentStatus.viewModeTile')}</option>
          <option value="diagram">{t('dashboard.agentStatus.viewModeDiagram')}</option>
        </select>
      </div>
    </div>
  );
}

/**
 * 히트맵 패널 전용 설정 (SPEC-HEATMAP-PANEL-001 T7).
 *
 * 히트맵 고유 편집 블록(센서 좌표/상하한/색상표/IDW/도면/등고선/범례):
 *   - 센서 좌표(sensor_positions) — 라이브 시리즈별 x/y(0..1) 입력 + 미배치 센서 노출(AC-E2).
 *   - value_bounds min/max — 미설정 시 자동(센서값 범위).
 *   - color_table — colorSwatchPalette 재사용한 정지점 편집.
 *   - IDW power / grid_resolution.
 *
 * 데이터 소스(store 태그/에이전트) 편집 UI 는 차트 패널과 동일하게 좌측 프리뷰 아래의
 * 공용 StoreSourceSection 으로 일원화했다(중복 렌더 제거). 여기서는 미배치 센서 노출용으로
 * store 폴링(storeResult)만 읽기 전용으로 재사용한다.
 *
 * onConfigChange 는 최상위 얕은 병합이므로 중첩 필드(sensor_positions/color_table/idw)는
 * 전체 객체를 다시 전달한다.
 */
function HeatmapSettingsSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const cfg = parseHeatmapConfig(config);

  // 센서 좌표(sensor_positions x/y) 편집은 데이터 소스의 선택된 시리즈 영역으로 이동했다
  // (PanelStoreSelectTable + 프리뷰 마커 드래그). 여기(패널 옵션)에서는 시각 옵션만 편집한다.
  // @spec SPEC-PANEL-SETTINGS-001 (heatmap 시리즈 위치)

  // value_bounds — 두 입력 모두 비면 undefined(자동)로 되돌린다.
  const rawBounds = config.value_bounds as { min?: number; max?: number } | undefined;
  const setBound = (key: 'min' | 'max', value: number | undefined) => {
    const merged: { min?: number; max?: number } = { ...(rawBounds ?? {}), [key]: value };
    const empty = merged.min === undefined && merged.max === undefined;
    onConfigChange({ value_bounds: empty ? undefined : merged });
  };

  // color_table — 정지점 배열 편집(빈 배열이면 undefined = 기본 gradient 폴백).
  const colorTable: ColorStop[] = Array.isArray(config.color_table)
    ? (config.color_table as ColorStop[])
    : [];
  const commitColorTable = (next: ColorStop[]) => {
    onConfigChange({ color_table: next.length > 0 ? next : undefined });
  };
  const addColorStop = () => {
    const stop = colorTable.length === 0 ? 0 : 1;
    commitColorTable([...colorTable, { stop, color: COLOR_PALETTE[0]! }]);
  };
  const updateColorStop = (idx: number, patch: Partial<ColorStop>) => {
    commitColorTable(colorTable.map((s, i) => (i === idx ? { ...s, ...patch } : s)));
  };
  const removeColorStop = (idx: number) => {
    commitColorTable(colorTable.filter((_, i) => i !== idx));
  };

  // IDW — power / grid_resolution(성능 가드: MIN..MAX clamp).
  const setIdw = (patch: { power?: number; grid_resolution?: number }) => {
    onConfigChange({ idw: { ...cfg.idw, ...patch } });
  };

  // SPEC-002: 도면 이미지(data-URL) / 불투명도 / fit / 에디터 옵션. 다중 레이어로 확장됐다.
  const [imgWarning, setImgWarning] = useState<string | null>(null);
  const layers = cfg.floor_plans;
  // 썸네일도 패널과 같은 해석 경로를 쓴다(자산 id → data-URL, 레거시 인라인 이미지는 그대로).
  const layerSources = useFloorPlanSources(layers);

  // "도면 비율에 맞추기" — 패널 높이(그리드 단위)를 기준 도면 종횡비에 맞춰 레터박스 여백을
  // 원인부터 없앤다. 레이아웃은 패널 config 가 아니라 대시보드 레이아웃 상태이므로 draft 를 거치지
  // 않고 즉시 반영된다(저장/취소 버튼과 무관 — 버튼 title 로 명시).
  const gridCols = useUIStore((s) => s.dashboardGridCols);
  const gridWidth = useUIStore((s) => s.dashboardGridWidth);
  const layout = useUIStore(
    (s) => s.dashboardPages.find((p) => p.id === s.activeDashboardId)?.layout,
  );
  const setDashboardLayout = useUIStore((s) => s.setDashboardLayout);
  const baseAspect = useFloorPlanAspect(layers[0], layerSources[0]);
  const layoutItem = layout?.find((l) => l.i === panel.id);
  const targetH =
    baseAspect !== undefined && layoutItem
      ? gridHeightForAspect(
          layoutItem.w,
          baseAspect,
          gridCellSize(gridWidth, gridCols),
          layoutItem.minH ?? 1,
        )
      : undefined;
  const canMatchRatio = layoutItem !== undefined && targetH !== undefined && targetH !== layoutItem.h;
  const matchPlanRatio = () => {
    if (!layout || !layoutItem || targetH === undefined) return;
    setDashboardLayout(layout.map((l) => (l.i === panel.id ? { ...l, h: targetH } : l)));
  };

  /**
   * 레이어 배열을 통째로 쓴다. 구 단일 `floor_plan` 도 함께 지워 두 표현이 공존하지 않게 한다 —
   * 남겨두면 파서가 배열을 우선하므로 조용히 무시되는 죽은 필드가 config 에 계속 남는다.
   */
  const commitLayers = (next: FloorPlanLayer[]) => {
    onConfigChange({ floor_plans: next, floor_plan: undefined });
  };
  const patchLayer = (idx: number, patch: Partial<FloorPlanLayer>) => {
    commitLayers(layers.map((l, i) => (i === idx ? { ...l, ...patch } : l)));
  };
  const removeLayer = (idx: number) => {
    setImgWarning(null);
    commitLayers(layers.filter((_, i) => i !== idx));
  };
  /** 레이어 순서 이동(그리기 순서 = 배열 순서, 뒤가 위). 범위를 벗어나면 no-op. */
  const moveLayer = (idx: number, delta: number) => {
    const to = idx + delta;
    if (to < 0 || to >= layers.length) return;
    const next = [...layers];
    const [moved] = next.splice(idx, 1);
    next.splice(to, 0, moved!);
    commitLayers(next);
  };

  // 자산 분리 이전에 저장된 인라인 이미지 레이어. 이 상태의 패널은 대시보드 snapshot 이
  // 256KB 를 넘겨 **저장 자체가 실패**하므로, 사용자가 명시적으로 옮길 수 있게 노출한다.
  // 자동으로 옮기지 않는 이유: 설정 화면을 여는 것만으로 config 를 고쳐 쓰면 사용자가
  // 의도하지 않은 변경이 draft 에 섞인다.
  const inlineLayerCount = layers.filter((l) => !l.asset_id && l.image).length;
  const [migrating, setMigrating] = useState(false);
  const migrateInlineLayers = async () => {
    setImgWarning(null);
    setMigrating(true);
    try {
      const next = await Promise.all(
        layers.map(async (l) => {
          if (l.asset_id || !l.image) return l;
          const asset = await uploadDashboardAsset(l.image);
          // image 는 제거한다 — 남겨두면 snapshot 크기가 그대로라 저장이 여전히 실패한다.
          const { image: _dropped, ...rest } = l;
          return { ...rest, asset_id: asset.id };
        }),
      );
      commitLayers(next);
    } catch {
      setImgWarning(t('dashboard.settings.heatmapImageReadError'));
    } finally {
      setMigrating(false);
    }
  };

  // 파일 첨부 → data-URL 인코딩 → 8MB 상한 검증(AC-E3). 초과 시 저장하지 않고 경고를 띄운다.
  // 원본 크기(natural_*)를 함께 저장한다 — 기준 레이어의 종횡비가 패널 스테이지 비율을 정하므로
  // 첫 페인트부터 정확하려면 config 에 있어야 한다(없으면 패널이 이미지를 로드해 실측한다).
  const onPickImage = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = ''; // 동일 파일 재선택 허용.
    if (!file) return;
    setImgWarning(null);
    try {
      const dataUrl = await readImageAsDataUrl(file);
      assertImageSizeUnderLimit(dataUrl, DEFAULT_MAX_IMAGE_BYTES);
      const natural = await readImageNaturalSize(dataUrl);
      // 이미지 바이트는 자산 API 로 올리고 config 에는 id 만 남긴다. data-URL 을 config 에
      // 박으면 대시보드 snapshot(256KB 상한)을 넘겨 저장 자체가 실패한다.
      const asset = await uploadDashboardAsset(dataUrl);
      const layer: FloorPlanLayer = {
        asset_id: asset.id,
        x: 0,
        y: 0,
        w: 1,
        h: 1,
        opacity: 1,
        fit: 'contain',
        ...natural,
      };
      commitLayers([...layers, layer]);
    } catch (err) {
      setImgWarning(
        err instanceof ImageSizeLimitError
          ? t('dashboard.settings.heatmapImageTooLarge')
          : t('dashboard.settings.heatmapImageReadError'),
      );
    }
  };
  const setOpacity = (v: number) => onConfigChange({ heatmap_opacity: v });
  // 에디터 옵션(snap/marker_size). 둘 다 비면 editor 를 undefined 로 되돌린다.
  const setEditor = (patch: { snap?: number; marker_size?: number }) => {
    const next: { snap?: number; marker_size?: number } = { ...(cfg.editor ?? {}), ...patch };
    if (next.snap === undefined) delete next.snap;
    if (next.marker_size === undefined) delete next.marker_size;
    const empty = next.snap === undefined && next.marker_size === undefined;
    onConfigChange({ editor: empty ? undefined : next });
  };

  // SPEC-003: 등고선 설정(additive — contour 키만 병합). 기존 필드/렌더 경로 무변경.
  const contourCfg = cfg.contour;
  const setContour = (patch: Partial<ContourConfig>) => {
    const base: ContourConfig = contourCfg ?? {
      enabled: false,
      level_count: DEFAULT_CONTOUR_LEVEL_COUNT,
      line: {},
      labels: false,
    };
    onConfigChange({ contour: { ...base, ...patch } });
  };
  // 선 스타일 부분 병합(색/두께/dash).
  const setContourLine = (patch: Partial<ContourConfig['line']>) => {
    const line = { ...(contourCfg?.line ?? {}), ...patch };
    if (line.color === undefined) delete line.color;
    if (line.width === undefined) delete line.width;
    if (line.dash === undefined || line.dash.length === 0) delete line.dash;
    setContour({ line });
  };

  // 색표 범례 설정(additive — legend 키만 병합). 기존 필드/렌더 경로 무변경.
  const legendCfg = cfg.legend;
  const setLegend = (patch: Partial<LegendConfig>) => {
    const base: LegendConfig = legendCfg ?? {
      enabled: false,
      orientation: 'vertical',
      position: 'bottom-right',
      size: 'md',
      tick_count: DEFAULT_LEGEND_TICK_COUNT,
    };
    onConfigChange({ legend: { ...base, ...patch } });
  };
  // 쉼표 구분 숫자열 → number[](유한만). 비면 undefined.
  const parseNumberList = (text: string): number[] | undefined => {
    const nums = text
      .split(',')
      .map((s) => Number(s.trim()))
      .filter((n) => Number.isFinite(n));
    return nums.length > 0 ? nums : undefined;
  };

  const inputCls =
    'w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500';

  return (
    <div className="space-y-4">
      {/* 센서 좌표(sensor_positions) 편집은 데이터 소스의 선택된 시리즈 영역 + 프리뷰 마커
          드래그로 이동했다(패널 옵션에서 제거). 여기서는 시각 옵션만 편집한다. */}

      {/* 3) 표시 상하한(value_bounds). 비우면 자동(센서값 범위). */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.heatmapValueBounds')}
        </label>
        <div className="flex items-center gap-2">
          <input
            type="number"
            value={rawBounds?.min !== undefined ? String(rawBounds.min) : ''}
            data-testid="heatmap-bounds-min"
            placeholder={t('dashboard.settings.gaugeSection.min')}
            onChange={(e) =>
              setBound('min', e.target.value === '' ? undefined : Number(e.target.value))
            }
            className={inputCls}
          />
          <span className="shrink-0 text-xs text-(--color-text-muted)">~</span>
          <input
            type="number"
            value={rawBounds?.max !== undefined ? String(rawBounds.max) : ''}
            data-testid="heatmap-bounds-max"
            placeholder={t('dashboard.settings.gaugeSection.max')}
            onChange={(e) =>
              setBound('max', e.target.value === '' ? undefined : Number(e.target.value))
            }
            className={inputCls}
          />
        </div>
      </div>

      {/* 4) 색상표(color_table). gradient 프리셋 + 정지점(0..1) 색상 스와치. 비우면 기본 gradient. */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.heatmapColorTable')}
        </label>
        {/* gradient 프리셋 선택(heatmap 전용, T8/REQ-10/11). 선택 시 draft color_table 반영. */}
        <div className="mb-2" data-testid="heatmap-color-presets">
          <span className="mb-1 block text-[11px] font-medium text-(--color-text-muted)">
            {t('dashboard.settings.heatmapColorPreset')}
          </span>
          <div className="flex flex-wrap gap-1.5">
            {HEATMAP_COLOR_PRESETS.map((preset) => (
              <button
                key={preset.id}
                type="button"
                data-testid={`heatmap-preset-${preset.id}`}
                onClick={() => commitColorTable(clonePresetStops(preset))}
                title={preset.name}
                className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-2 py-1 text-[11px] font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-secondary)"
              >
                <span
                  aria-hidden="true"
                  className="h-3 w-8 shrink-0 rounded-sm border border-(--color-border-default)"
                  style={{
                    backgroundImage: `linear-gradient(to right, ${preset.stops
                      .map((s) => `${s.color} ${Math.round(s.stop * 100)}%`)
                      .join(', ')})`,
                  }}
                />
                {preset.name}
              </button>
            ))}
          </div>
        </div>
        <div className="space-y-1.5">
          {colorTable.map((stop, idx) => (
            <div key={idx} className="flex items-center gap-1.5">
              <ColorSwatchButton
                color={stop.color}
                onChange={(c) => updateColorStop(idx, { color: c ?? COLOR_PALETTE[0]! })}
                ariaLabel={t('dashboard.settings.heatmapColorStopAria').replace('{index}', String(idx + 1))}
              />
              <input
                type="number"
                min={0}
                max={1}
                step={0.05}
                value={String(stop.stop)}
                data-testid={`heatmap-colorstop-${idx}`}
                onChange={(e) => updateColorStop(idx, { stop: Number(e.target.value) })}
                className="w-20 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
              />
              <button
                type="button"
                onClick={() => removeColorStop(idx)}
                className="shrink-0 rounded p-0.5 text-(--color-text-muted) transition-colors hover:text-red-500"
                aria-label={t('dashboard.settings.heatmapRemoveColorStopAria')}
              >
                <X className="h-3 w-3" />
              </button>
            </div>
          ))}
          <button
            type="button"
            onClick={addColorStop}
            data-testid="heatmap-add-colorstop"
            className="flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
          >
            <Plus className="h-3 w-3" />
            {t('dashboard.settings.heatmapAddColorStop')}
          </button>
        </div>
      </div>

      {/* 5) IDW 파라미터(power / grid_resolution). */}
      <div className="grid grid-cols-2 gap-2">
        <div>
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.settings.heatmapIdwPower')}
          </label>
          <input
            type="number"
            min={1}
            step={0.5}
            value={String(cfg.idw.power)}
            data-testid="heatmap-idw-power"
            onChange={(e) => {
              const v = Number(e.target.value);
              if (Number.isFinite(v) && v > 0) setIdw({ power: v });
            }}
            className={inputCls}
          />
        </div>
        <div>
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.settings.heatmapGridResolution')}
          </label>
          <input
            type="number"
            min={MIN_GRID_RESOLUTION}
            max={MAX_GRID_RESOLUTION}
            step={1}
            value={String(cfg.idw.grid_resolution)}
            data-testid="heatmap-idw-grid"
            onChange={(e) => {
              const v = Math.trunc(Number(e.target.value));
              if (Number.isFinite(v) && v > 0) {
                setIdw({
                  grid_resolution: Math.max(
                    MIN_GRID_RESOLUTION,
                    Math.min(MAX_GRID_RESOLUTION, v),
                  ),
                });
              }
            }}
            className={inputCls}
          />
        </div>
      </div>

      {/* 6) SPEC-002: 도면 이미지 배경(첨부/미리보기/제거) + fit. */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.heatmapFloorPlan')}
        </label>
        {/* 레이어 목록: 배열 순서가 곧 그리기 순서(위 항목이 아래에 깔림). 첫 항목이 기준 도면
            이며 패널 스테이지의 종횡비를 정한다 — 그래서 순서 이동이 단순 z-order 이상의 의미를
            갖고, 안내 문구로 명시한다. */}
        {layers.length > 0 && (
          <ul className="mb-1.5 space-y-1.5" data-testid="heatmap-floorplan-layers">
            {layers.map((layer, idx) => (
              <li
                key={idx}
                data-testid={`heatmap-floorplan-layer-${idx}`}
                className="rounded-md border border-(--color-border-default) p-1.5"
              >
                <div className="flex items-center gap-2">
                  <img
                    src={layerSources[idx] || undefined}
                    alt=""
                    data-testid={idx === 0 ? 'heatmap-floorplan-preview' : `heatmap-floorplan-preview-${idx}`}
                    className="h-14 w-20 shrink-0 rounded border border-(--color-border-default) object-cover"
                  />
                  <div className="min-w-0 flex-1">
                    <span className="block text-[11px] font-medium text-(--color-text-secondary)">
                      {idx === 0
                        ? t('dashboard.settings.heatmapLayerBase')
                        : t('dashboard.settings.heatmapLayerNth').replace('{n}', String(idx + 1))}
                    </span>
                    {/* fit(박스 안 맞춤). 기준 레이어는 스테이지와 종횡비가 같아 사실상 무의미하지만
                        일관성을 위해 동일하게 노출한다. */}
                    <select
                      value={layer.fit}
                      data-testid={`heatmap-floorplan-fit-${idx}`}
                      onChange={(e) => {
                        const v = e.target.value;
                        patchLayer(idx, {
                          fit: v === 'cover' || v === 'fill' ? v : 'contain',
                        });
                      }}
                      className="mt-1 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
                    >
                      <option value="contain">{t('dashboard.settings.heatmapFitContain')}</option>
                      <option value="fill">{t('dashboard.settings.heatmapFitFill')}</option>
                      <option value="cover">{t('dashboard.settings.heatmapFitCover')}</option>
                    </select>
                  </div>
                  <div className="flex shrink-0 flex-col gap-0.5">
                    <button
                      type="button"
                      data-testid={`heatmap-floorplan-up-${idx}`}
                      aria-label={t('dashboard.settings.heatmapLayerMoveUp')}
                      title={t('dashboard.settings.heatmapLayerMoveUp')}
                      disabled={idx === 0}
                      onClick={() => moveLayer(idx, -1)}
                      className="rounded p-0.5 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-hover) disabled:opacity-30"
                    >
                      <ChevronUp className="h-3.5 w-3.5" />
                    </button>
                    <button
                      type="button"
                      data-testid={`heatmap-floorplan-down-${idx}`}
                      aria-label={t('dashboard.settings.heatmapLayerMoveDown')}
                      title={t('dashboard.settings.heatmapLayerMoveDown')}
                      disabled={idx === layers.length - 1}
                      onClick={() => moveLayer(idx, 1)}
                      className="rounded p-0.5 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-hover) disabled:opacity-30"
                    >
                      <ChevronDown className="h-3.5 w-3.5" />
                    </button>
                    <button
                      type="button"
                      data-testid={idx === 0 ? 'heatmap-floorplan-remove' : `heatmap-floorplan-remove-${idx}`}
                      aria-label={t('dashboard.settings.heatmapRemoveImage')}
                      title={t('dashboard.settings.heatmapRemoveImage')}
                      onClick={() => removeLayer(idx)}
                      className="rounded p-0.5 text-red-600 transition-colors hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
                {/* 위치/크기(스테이지 정규화 0..1) + 불투명도. 기준 도면 위 어디에 얼마만큼
                    얹을지를 숫자로 지정한다. 기본 {0,0,1,1} 이 스테이지를 가득 채운다. */}
                <div className="mt-1.5 grid grid-cols-5 gap-1">
                  {(['x', 'y', 'w', 'h'] as const).map((axis) => (
                    <label key={axis} className="flex flex-col gap-0.5">
                      <span className="text-[10px] text-(--color-text-muted)">
                        {t(`dashboard.settings.heatmapLayer${axis.toUpperCase()}`)}
                      </span>
                      <input
                        type="number"
                        min={axis === 'w' || axis === 'h' ? 0.01 : 0}
                        max={1}
                        step={0.05}
                        value={String(layer[axis])}
                        data-testid={`heatmap-floorplan-${axis}-${idx}`}
                        onChange={(e) => {
                          const v = Number(e.target.value);
                          if (!Number.isFinite(v)) return;
                          patchLayer(idx, { [axis]: v } as Partial<FloorPlanLayer>);
                        }}
                        className="rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
                      />
                    </label>
                  ))}
                  <label className="flex flex-col gap-0.5">
                    <span className="text-[10px] text-(--color-text-muted)">
                      {t('dashboard.settings.heatmapLayerOpacity')}
                    </span>
                    <input
                      type="number"
                      min={0}
                      max={1}
                      step={0.05}
                      value={String(layer.opacity)}
                      data-testid={`heatmap-floorplan-opacity-${idx}`}
                      onChange={(e) => {
                        const v = Number(e.target.value);
                        if (!Number.isFinite(v)) return;
                        patchLayer(idx, { opacity: v });
                      }}
                      className="rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
                    />
                  </label>
                </div>
              </li>
            ))}
          </ul>
        )}
        <label className="inline-flex cursor-pointer items-center gap-1 rounded-md border border-dashed border-(--color-border-default) px-2 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)">
          <Plus className="h-3 w-3" />
          {layers.length === 0
            ? t('dashboard.settings.heatmapAttachImage')
            : t('dashboard.settings.heatmapAddLayer')}
          <input
            type="file"
            accept="image/*"
            data-testid="heatmap-floorplan-input"
            onChange={onPickImage}
            className="hidden"
          />
        </label>
        {layers.length > 0 && (
          <p className="mt-1 text-[11px] text-(--color-text-muted)">
            {t('dashboard.settings.heatmapLayerHint')}
          </p>
        )}
        {/* 스테이지 맞춤: 여백(contain) / 잘림(cover) / 왜곡(stretch) 중 무엇을 감수할지.
            아래 "도면 비율에 맞추기"가 여백을 원인부터 없애는 길이고, 이건 패널 크기를 그대로 둔 채
            여백만 없애는 길이다. cover 는 잘린 영역의 센서를 배치 편집에서 잡을 수 없다. */}
        {layers.length > 0 && (
          <div className="mt-1.5">
            <label className="mb-1 block text-[11px] text-(--color-text-muted)">
              {t('dashboard.settings.heatmapStageFit')}
            </label>
            <select
              value={cfg.stage_fit ?? 'contain'}
              data-testid="heatmap-stage-fit"
              onChange={(e) => {
                const v = e.target.value;
                // 기본값(contain)은 undefined 로 지워 config 에 죽은 필드를 남기지 않는다.
                onConfigChange({ stage_fit: v === 'cover' || v === 'stretch' ? v : undefined });
              }}
              className="rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
            >
              <option value="contain">{t('dashboard.settings.heatmapStageFitContain')}</option>
              <option value="cover">{t('dashboard.settings.heatmapStageFitCover')}</option>
              <option value="stretch">{t('dashboard.settings.heatmapStageFitStretch')}</option>
            </select>
            <p data-testid="heatmap-stage-fit-hint" className="mt-1 text-[11px] text-(--color-text-muted)">
              {t(
                cfg.stage_fit === 'cover'
                  ? 'dashboard.settings.heatmapStageFitCoverHint'
                  : cfg.stage_fit === 'stretch'
                    ? 'dashboard.settings.heatmapStageFitStretchHint'
                    : 'dashboard.settings.heatmapStageFitContainHint',
              )}
            </p>
          </div>
        )}
        {/* 패널 크기 ↔ 도면 종횡비 정렬. 스테이지가 도면 비율로 레터박스되므로(stage.ts) 둘이
            어긋난 만큼이 그대로 좌우/상하 여백이 된다. 그리드는 정수 단위라 반올림 후 여백은
            "한 칸 이내"로 남는다. */}
        {layers.length > 0 && layoutItem && (
          <div className="mt-1.5 flex flex-wrap items-center gap-2" data-testid="heatmap-match-ratio-row">
            <button
              type="button"
              data-testid="heatmap-match-ratio"
              disabled={!canMatchRatio}
              title={t('dashboard.settings.heatmapMatchPanelRatioTitle')}
              onClick={matchPlanRatio}
              className="inline-flex items-center gap-1 rounded-md border border-(--color-border-default) px-2 py-1 text-[11px] font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-40"
            >
              {t('dashboard.settings.heatmapMatchPanelRatio')}
            </button>
            <span data-testid="heatmap-match-ratio-hint" className="text-[11px] text-(--color-text-muted)">
              {t(
                targetH === undefined
                  ? 'dashboard.settings.heatmapMatchPanelRatioUnknown'
                  : canMatchRatio
                    ? 'dashboard.settings.heatmapMatchPanelRatioHint'
                    : 'dashboard.settings.heatmapMatchPanelRatioDone',
              )
                .replace('{w}', String(layoutItem.w))
                .replace('{h}', String(layoutItem.h))
                .replace('{targetH}', String(targetH ?? layoutItem.h))}
            </span>
          </div>
        )}
        {/* 인라인 이미지 이관 안내 — 이 상태에서는 대시보드 저장이 413 으로 실패한다. */}
        {inlineLayerCount > 0 && (
          <div
            data-testid="heatmap-floorplan-inline-warning"
            className="mt-1.5 rounded-md border border-amber-300 bg-amber-50 px-2 py-1.5 text-[11px] text-amber-800 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-300"
          >
            <p>
              {t('dashboard.settings.heatmapInlineImageWarning').replace(
                '{count}',
                String(inlineLayerCount),
              )}
            </p>
            <button
              type="button"
              data-testid="heatmap-floorplan-migrate"
              disabled={migrating}
              onClick={() => void migrateInlineLayers()}
              className="mt-1 inline-flex items-center gap-1 rounded-md border border-amber-400 px-2 py-1 font-medium transition-colors hover:bg-amber-100 disabled:opacity-50 dark:hover:bg-amber-900/40"
            >
              {t('dashboard.settings.heatmapMigrateInlineImage')}
            </button>
          </div>
        )}
        {imgWarning && (
          <p
            data-testid="heatmap-floorplan-warning"
            className="mt-1 text-[11px] text-red-600 dark:text-red-400"
          >
            {imgWarning}
          </p>
        )}
      </div>

      {/* 6-1) 값 표기 자릿수.
          히트맵에는 값 읽기(툴팁·타일)가 없고 범례 눈금과 등고선 라벨만 있다. 둘 다
          눈금자이므로 **비워 두면 종전 표기**(범례 1자리 / 등고선 정수)를 그대로 쓰고,
          자릿수를 직접 넣었을 때만 그 값을 따른다. */}
      <DecimalPlacesField
        config={config}
        onConfigChange={onConfigChange}
        testId="heatmap-decimal-places"
      />

      {/* 7) SPEC-002: 히트맵 합성 불투명도(0..1). */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.heatmapOpacity')}{' '}
          <span className="tabular-nums text-(--color-text-secondary)">
            {cfg.heatmap_opacity.toFixed(2)}
          </span>
        </label>
        <input
          type="range"
          min={0}
          max={1}
          step={0.05}
          value={cfg.heatmap_opacity}
          data-testid="heatmap-opacity"
          onChange={(e) => setOpacity(Number(e.target.value))}
          className="w-full"
        />
      </div>

      {/* 8) SPEC-002: 배치 에디터 옵션(스냅/마커 크기) + 미배치 안내. 실제 드래그 편집은 패널의
          편집 토글에서 수행한다(런타임 상태). */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.heatmapEditor')}
        </label>
        <p className="mb-1.5 text-[11px] text-(--color-text-muted)">
          {t('dashboard.settings.heatmapEditHint')}
        </p>
        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className="mb-1 block text-[11px] text-(--color-text-muted)">
              {t('dashboard.settings.heatmapSnap')}
            </label>
            <input
              type="number"
              min={0}
              max={1}
              step={0.05}
              value={cfg.editor?.snap !== undefined ? String(cfg.editor.snap) : ''}
              data-testid="heatmap-editor-snap"
              placeholder="0"
              onChange={(e) =>
                setEditor({ snap: e.target.value === '' ? undefined : Number(e.target.value) })
              }
              className={inputCls}
            />
          </div>
          <div>
            <label className="mb-1 block text-[11px] text-(--color-text-muted)">
              {t('dashboard.settings.heatmapMarkerSize')}
            </label>
            <input
              type="number"
              min={4}
              step={1}
              value={cfg.editor?.marker_size !== undefined ? String(cfg.editor.marker_size) : ''}
              data-testid="heatmap-editor-marker-size"
              placeholder="16"
              onChange={(e) =>
                setEditor({
                  marker_size: e.target.value === '' ? undefined : Number(e.target.value),
                })
              }
              className={inputCls}
            />
          </div>
        </div>
      </div>

      {/* 9) SPEC-003: 등고선(contour lines) 오버레이 — enabled/레벨/선 스타일/라벨. additive. */}
      <div>
        <label className="mb-1.5 flex items-center gap-2 text-xs font-medium text-(--color-text-muted)">
          <input
            type="checkbox"
            checked={contourCfg?.enabled ?? false}
            data-testid="heatmap-contour-enabled"
            onChange={(e) => setContour({ enabled: e.target.checked })}
          />
          {t('dashboard.settings.heatmapContour')}
        </label>

        {(contourCfg?.enabled ?? false) && (
          <div className="mt-2 space-y-2 border-l-2 border-(--color-border-default) pl-2">
            {/* 레벨 개수 vs 명시 등치값(설정 시 개수보다 우선). */}
            <div className="grid grid-cols-2 gap-2">
              <div>
                <label className="mb-1 block text-[11px] text-(--color-text-muted)">
                  {t('dashboard.settings.heatmapContourLevelCount')}
                </label>
                <input
                  type="number"
                  min={1}
                  step={1}
                  value={String(contourCfg?.level_count ?? DEFAULT_CONTOUR_LEVEL_COUNT)}
                  data-testid="heatmap-contour-level-count"
                  onChange={(e) => {
                    const n = Number(e.target.value);
                    if (Number.isFinite(n) && n > 0) setContour({ level_count: Math.trunc(n) });
                  }}
                  className={inputCls}
                />
              </div>
              <div>
                <label className="mb-1 block text-[11px] text-(--color-text-muted)">
                  {t('dashboard.settings.heatmapContourLevels')}
                </label>
                <input
                  type="text"
                  value={contourCfg?.levels?.join(', ') ?? ''}
                  data-testid="heatmap-contour-levels"
                  placeholder="20, 24"
                  onChange={(e) => setContour({ levels: parseNumberList(e.target.value) })}
                  className={inputCls}
                />
              </div>
            </div>

            {/* 선 스타일: 색 / 두께 / dash. */}
            <div className="flex items-center gap-2">
              <span className="text-[11px] text-(--color-text-muted)">
                {t('dashboard.settings.heatmapContourLineColor')}
              </span>
              <ColorSwatchButton
                color={contourCfg?.line.color}
                onChange={(color) => setContourLine({ color })}
                ariaLabel={t('dashboard.settings.heatmapContourLineColorAria')}
              />
            </div>
            <div className="grid grid-cols-2 gap-2">
              <div>
                <label className="mb-1 block text-[11px] text-(--color-text-muted)">
                  {t('dashboard.settings.heatmapContourLineWidth')}
                </label>
                <input
                  type="number"
                  min={0.5}
                  step={0.5}
                  value={contourCfg?.line.width !== undefined ? String(contourCfg.line.width) : ''}
                  data-testid="heatmap-contour-line-width"
                  placeholder="1"
                  onChange={(e) =>
                    setContourLine({
                      width: e.target.value === '' ? undefined : Number(e.target.value),
                    })
                  }
                  className={inputCls}
                />
              </div>
              <div>
                <label className="mb-1 block text-[11px] text-(--color-text-muted)">
                  {t('dashboard.settings.heatmapContourLineDash')}
                </label>
                <input
                  type="text"
                  value={contourCfg?.line.dash?.join(', ') ?? ''}
                  data-testid="heatmap-contour-line-dash"
                  placeholder="4, 2"
                  onChange={(e) => setContourLine({ dash: parseNumberList(e.target.value) })}
                  className={inputCls}
                />
              </div>
            </div>

            {/* 등치값 라벨 토글. */}
            <label className="flex items-center gap-2 text-[11px] text-(--color-text-muted)">
              <input
                type="checkbox"
                checked={contourCfg?.labels ?? false}
                data-testid="heatmap-contour-labels"
                onChange={(e) => setContour({ labels: e.target.checked })}
              />
              {t('dashboard.settings.heatmapContourLabels')}
            </label>
          </div>
        )}
      </div>

      {/* 10) 값→색 색표(colorbar) 범례 — enabled/방향/위치/크기/눈금 개수. additive. */}
      <div>
        <label className="mb-1.5 flex items-center gap-2 text-xs font-medium text-(--color-text-muted)">
          <input
            type="checkbox"
            checked={legendCfg?.enabled ?? false}
            data-testid="heatmap-legend-enabled"
            onChange={(e) => setLegend({ enabled: e.target.checked })}
          />
          {t('dashboard.settings.heatmapLegend')}
        </label>

        {(legendCfg?.enabled ?? false) && (
          <div className="mt-2 space-y-2 border-l-2 border-(--color-border-default) pl-2">
            {/* 자리와 크기는 여기에 없다 — 미리보기에서 범례를 끌어 옮기고, 오른쪽 아래
                손잡이를 끌어 키운다. 화면을 보면서 맞추는 일을 설정 창의 드롭박스로 밀어내면
                고른 값이 화면에서 어떻게 보일지 매번 상상해야 한다. */}
            <p className="text-[11px] leading-tight text-(--color-text-muted)">
              {t('dashboard.settings.heatmapLegendDragHint')}
            </p>

            <div className="grid grid-cols-2 gap-2">
              {/* 방향(가로/세로). */}
              <div>
                <label className="mb-1 block text-[11px] text-(--color-text-muted)">
                  {t('dashboard.settings.heatmapLegendOrientation')}
                </label>
                <select
                  value={legendCfg?.orientation ?? 'vertical'}
                  data-testid="heatmap-legend-orientation"
                  onChange={(e) =>
                    setLegend({ orientation: e.target.value as LegendConfig['orientation'] })
                  }
                  className={inputCls}
                >
                  <option value="vertical">
                    {t('dashboard.settings.heatmapLegendOrientationVertical')}
                  </option>
                  <option value="horizontal">
                    {t('dashboard.settings.heatmapLegendOrientationHorizontal')}
                  </option>
                </select>
              </div>
              {/* 눈금 개수(2..10). */}
              <div>
                <label className="mb-1 block text-[11px] text-(--color-text-muted)">
                  {t('dashboard.settings.heatmapLegendTickCount')}
                </label>
                <input
                  type="number"
                  min={2}
                  max={10}
                  step={1}
                  value={String(legendCfg?.tick_count ?? DEFAULT_LEGEND_TICK_COUNT)}
                  data-testid="heatmap-legend-tick-count"
                  onChange={(e) => {
                    const n = Number(e.target.value);
                    if (Number.isFinite(n) && n > 0) setLegend({ tick_count: Math.trunc(n) });
                  }}
                  className={inputCls}
                />
              </div>
            </div>

            <div className="grid grid-cols-2 gap-2">
              {/* 글자 크기(px). 미지정이면 막대 크기에서 파생된 값이 쓰이므로 placeholder 로
                  "자동" 을 알린다 — 0 이나 기본 숫자를 채우면 지정 여부를 구분할 수 없다. */}
              <div>
                <label className="mb-1 block text-[11px] text-(--color-text-muted)">
                  {t('dashboard.settings.heatmapLegendFontSize')}
                </label>
                <input
                  type="number"
                  min={MIN_LEGEND_FONT_SIZE}
                  max={MAX_LEGEND_FONT_SIZE}
                  step={1}
                  value={legendCfg?.font_size !== undefined ? String(legendCfg.font_size) : ''}
                  placeholder={t('dashboard.settings.heatmapLegendFontSizeAuto')}
                  data-testid="heatmap-legend-font-size"
                  onChange={(e) => {
                    const raw = e.target.value.trim();
                    if (raw === '') {
                      setLegend({ font_size: undefined });
                      return;
                    }
                    const n = Number(raw);
                    if (Number.isFinite(n)) setLegend({ font_size: Math.trunc(n) });
                  }}
                  className={inputCls}
                />
              </div>
              {/* 글자 색. 체크를 끄면 지정을 지워 테마 보조색으로 돌아간다 — 색 입력만 두면
                  한 번 고른 색을 "안 고른 상태" 로 되돌릴 방법이 없다(게이지 바늘색과 같은 규칙). */}
              <div>
                <label className="mb-1 block text-[11px] text-(--color-text-muted)">
                  {t('dashboard.settings.heatmapLegendFontColor')}
                </label>
                <div className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={legendCfg?.font_color !== undefined}
                    data-testid="heatmap-legend-font-color-enabled"
                    onChange={(e) =>
                      setLegend({ font_color: e.target.checked ? '#334155' : undefined })
                    }
                  />
                  {legendCfg?.font_color !== undefined && (
                    <input
                      type="color"
                      value={legendCfg.font_color}
                      data-testid="heatmap-legend-font-color"
                      onChange={(e) => setLegend({ font_color: e.target.value })}
                      className="h-7 w-10 cursor-pointer rounded border border-(--color-border-default) bg-transparent"
                    />
                  )}
                </div>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

/**
 * MODBUS Gateway 패널 전용 설정 (SPEC-MODBUS-012).
 *
 * AddPanelDialog 의 ModbusAgentStep 을 미러링한다:
 *   - modbus-gateway 에이전트를 필터해 재선택 → config.agentId.
 *   - modbus-device-registers 만 대상 unit 2차 선택 → config.unitId(선택 에이전트에 list_devices 조회).
 * 변경은 onConfigChange 로 draftConfig 에만 기록한다(적용 버튼 전까지 스토어 미반영).
 */
function ModbusSettingsSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const agentId = (panel.config?.agentId as string | undefined) ?? '';
  const needsUnit = panel.type === 'modbus-device-registers';
  const currentUnitId = panel.config?.unitId as number | undefined;
  // 단일 그리드 열 수(config.columns) — summary/bus 만. 미설정/0 은 자동(반응형/wrap).
  const needsColumns = MODBUS_COLUMNS_PANEL_TYPES.has(panel.type);
  const currentColumns = panel.config?.columns as number | undefined;
  // 영역별 셀 열 수(config.areaColumns) — 레지스터 맵 패널(shared/device) 만.
  const needsAreaColumns = MODBUS_AREA_COLUMNS_PANEL_TYPES.has(panel.type);
  // 표시/편집 기준값: 구 단일 columns 마이그레이션을 반영한 영역별 값(resolveAreaColumns).
  const areaColumns = resolveAreaColumns(panel.config);
  // 영역 카드 배치 열 수(config.areaLayoutColumns) — 레지스터 맵 패널 전용. 4개 영역 카드의 외곽 배치.
  // 미설정/0 은 반응형 기본(1→2열). 영역별 셀 열 수(areaColumns)와는 독립적인 별개 설정이다.
  const currentAreaLayoutColumns = panel.config?.areaLayoutColumns as number | undefined;

  const { data: agentsResult } = useAgents();
  const gatewayAgents = useMemo(
    () => (agentsResult?.data ?? []).filter((a) => a.type === 'modbus-gateway'),
    [agentsResult],
  );

  // 대상 unit 목록은 가상 레지스터 맵 패널에서만 선택 에이전트에 list_devices 로 조회한다.
  const { devices } = useModbusListDevices(agentId, needsUnit && agentId.length > 0);

  return (
    <div className="space-y-3">
      {/* 에이전트 선택(modbus-gateway 만). 변경 시 unit 은 초기화한다. */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.agent')}
        </label>
        <select
          value={agentId}
          data-testid="modbus-settings-agent-select"
          onChange={(e) =>
            onConfigChange(
              needsUnit
                ? { agentId: e.target.value, unitId: undefined }
                : { agentId: e.target.value },
            )
          }
          className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        >
          <option value="">{t('dashboard.settings.selectAgent')}</option>
          {gatewayAgents.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </select>
      </div>

      {/* 대상 unit 선택(가상 디바이스 레지스터 맵 전용). */}
      {needsUnit && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.modbus.selectUnit')}
          </label>
          <select
            value={currentUnitId !== undefined ? String(currentUnitId) : ''}
            data-testid="modbus-settings-unit-select"
            onChange={(e) =>
              onConfigChange({ unitId: e.target.value === '' ? undefined : Number(e.target.value) })
            }
            disabled={!agentId}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 disabled:opacity-60"
          >
            <option value="">{t('dashboard.modbus.selectUnitPlaceholder')}</option>
            {devices.map((d) => (
              <option key={d.unit_id} value={String(d.unit_id)}>
                {formatUnitLabel(d.unit_id)} · {d.name || formatUnitLabel(d.unit_id)}
              </option>
            ))}
            {/* 저장된 unit 이 현재 목록에 없어도(에이전트 미실행 등) 선택 상태를 유지한다. */}
            {currentUnitId !== undefined &&
              !devices.some((d) => d.unit_id === currentUnitId) && (
                <option value={String(currentUnitId)}>{formatUnitLabel(currentUnitId)}</option>
              )}
          </select>
        </div>
      )}

      {/* 단일 그리드 열 수(config.columns) — summary/bus 전용. 비움/0 = 자동(반응형/wrap). */}
      {needsColumns && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.settings.modbusColumns')}
          </label>
          <input
            type="number"
            min={1}
            max={24}
            value={currentColumns !== undefined ? String(currentColumns) : ''}
            data-testid="modbus-settings-columns-input"
            onChange={(e) =>
              onConfigChange({
                columns: e.target.value === '' ? undefined : Number(e.target.value),
              })
            }
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
          />
          <p className="mt-1 text-[11px] text-(--color-text-muted)">
            {t('dashboard.settings.modbusColumnsHint')}
          </p>
        </div>
      )}

      {/* 영역별 셀 열 수(config.areaColumns) — 레지스터 맵 패널 전용. 4영역 각각 비움/0 = 자동(wrap). */}
      {needsAreaColumns && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.settings.modbusAreaColumns')}
          </label>
          <div className="space-y-2">
            {REGISTER_AREA_ORDER.map((area) => {
              const value = areaColumns[area];
              return (
                <div key={area} className="flex items-center gap-2">
                  <span className="min-w-0 flex-1 truncate text-xs text-(--color-text-secondary)">
                    {REGISTER_AREA_LABELS[area]}
                  </span>
                  <input
                    type="number"
                    min={1}
                    max={24}
                    value={value !== undefined ? String(value) : ''}
                    data-testid={`modbus-settings-area-columns-input-${area}`}
                    onChange={(e) => {
                      // onConfigChange 는 최상위 얕은 병합이므로 areaColumns 전체를 다시 전달한다.
                      // resolveAreaColumns 로 마이그레이션 시드된 다른 영역 값도 함께 보존한다.
                      const next: Partial<Record<RegisterArea, number>> = { ...areaColumns };
                      if (e.target.value === '') delete next[area];
                      else next[area] = Number(e.target.value);
                      onConfigChange({ areaColumns: next });
                    }}
                    className="w-20 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
                  />
                </div>
              );
            })}
          </div>
          <p className="mt-1 text-[11px] text-(--color-text-muted)">
            {t('dashboard.settings.modbusColumnsHint')}
          </p>
        </div>
      )}

      {/* 영역 카드 배치 열 수(config.areaLayoutColumns) — 레지스터 맵 패널 전용. 4개 영역 카드의 외곽 배치. */}
      {/* 비움/0 = 자동(반응형 1→2열). 영역별 셀 열 수(위)와는 독립적인 별개 설정이다. */}
      {needsAreaColumns && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.settings.modbusAreaLayoutColumns')}
          </label>
          <input
            type="number"
            min={1}
            max={4}
            value={currentAreaLayoutColumns !== undefined ? String(currentAreaLayoutColumns) : ''}
            data-testid="modbus-settings-area-layout-columns-input"
            onChange={(e) =>
              onConfigChange({
                areaLayoutColumns: e.target.value === '' ? undefined : Number(e.target.value),
              })
            }
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
          />
          <p className="mt-1 text-[11px] text-(--color-text-muted)">
            {t('dashboard.settings.modbusAreaLayoutColumnsHint')}
          </p>
        </div>
      )}
    </div>
  );
}

/**
 * MODBUS 패널 프리뷰 — 실제 패널 컴포넌트를 draft config 로 렌더한다(SPEC-MODBUS-012).
 * 각 패널이 미설정/원격/로딩/에러/빈 상태를 자체 ModbusNotice 로 표시하므로 프리뷰가 blank 가 되지 않는다.
 */
function ModbusPanelPreview({ panel }: { panel: PanelConfig }) {
  const config = panel.config ?? {};
  const common = { title: panel.title, config };
  return (
    <div className="h-full w-full overflow-hidden rounded-xl border border-(--color-border-default)">
      {panel.type === 'modbus-real-devices' && <ModbusRealDevicesPanel {...common} />}
      {panel.type === 'modbus-virtual-devices' && <ModbusVirtualDevicesPanel {...common} />}
      {panel.type === 'modbus-shared-registers' && <ModbusSharedRegistersPanel {...common} />}
      {panel.type === 'modbus-device-registers' && <ModbusDeviceRegistersPanel {...common} />}
      {panel.type === 'modbus-bus-stats' && <ModbusBusStatsPanel {...common} />}
      {panel.type === 'modbus-summary-stats' && <ModbusSummaryStatsPanel {...common} />}
    </div>
  );
}

/** config 의 nodeSize 값을 5단계 키로 정규화(레거시 sm/md/lg → '1'/'2'/'3', 기본 '2'). */
function normalizeNodeSize(value: unknown): '1' | '2' | '3' | '4' | '5' {
  switch (value) {
    case '1':
    case '2':
    case '3':
    case '4':
    case '5':
      return value;
    case 'sm':
      return '1';
    case 'md':
      return '2';
    case 'lg':
      return '3';
    default:
      return '2';
  }
}

/**
 * 라인 패널 표시 옵션(config-only, UB-003 — 스냅샷 스키마 변경 없음).
 *   - showStationStatus: "역사별 간략 상태" 섹션 표시(기본 true).
 *   - showLineStats: "라인 통계" 섹션 표시(기본 true).
 *   - offlineAsOff: 오프라인을 꺼짐으로 표시(기본 false).
 *   - nodeSize: 라인도 역 정보 크기 5단계 '1'~'5'(기본 '2', 레벨3 = 기존 lg).
 *   - stationsPerRow: 1줄당 역사 수(0 = 자동, nodeSize 기반 폴백; 1 이상 지정 시 직접 제어).
 */
function FacilityLineDisplayOptions({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const showStationStatus = (panel.config?.showStationStatus as boolean | undefined) ?? true;
  const showLineStats = (panel.config?.showLineStats as boolean | undefined) ?? true;
  const offlineAsOff = (panel.config?.offlineAsOff as boolean | undefined) ?? false;
  const nodeSize = normalizeNodeSize(panel.config?.nodeSize);
  const stationsPerRow = (panel.config?.stationsPerRow as number | undefined) ?? 0;
  const sizeOptions: { value: '1' | '2' | '3' | '4' | '5'; labelKey: string }[] = [
    { value: '1', labelKey: 'dashboard.settings.nodeSize1' },
    { value: '2', labelKey: 'dashboard.settings.nodeSize2' },
    { value: '3', labelKey: 'dashboard.settings.nodeSize3' },
    { value: '4', labelKey: 'dashboard.settings.nodeSize4' },
    { value: '5', labelKey: 'dashboard.settings.nodeSize5' },
  ];

  return (
    <>
      <label className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-(--color-bg-elevated)">
        <input
          type="checkbox"
          data-testid="facility-line-show-station-status"
          checked={showStationStatus}
          onChange={(e) => onConfigChange({ showStationStatus: e.target.checked })}
          className="h-4 w-4 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500"
        />
        <span className="text-sm font-medium text-(--color-text-primary)">
          {t('dashboard.settings.showStationStatus')}
        </span>
      </label>

      <label className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-(--color-bg-elevated)">
        <input
          type="checkbox"
          data-testid="facility-line-show-line-stats"
          checked={showLineStats}
          onChange={(e) => onConfigChange({ showLineStats: e.target.checked })}
          className="h-4 w-4 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500"
        />
        <span className="text-sm font-medium text-(--color-text-primary)">
          {t('dashboard.settings.showLineStats')}
        </span>
      </label>

      <label className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-(--color-bg-elevated)">
        <input
          type="checkbox"
          data-testid="facility-line-offline-as-off"
          checked={offlineAsOff}
          onChange={(e) => onConfigChange({ offlineAsOff: e.target.checked })}
          className="h-4 w-4 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500"
        />
        <span className="text-sm font-medium text-(--color-text-primary)">
          {t('dashboard.settings.offlineAsOff')}
        </span>
      </label>

      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.nodeSize')}
        </label>
        <select
          value={nodeSize}
          data-testid="facility-line-node-size"
          onChange={(e) => onConfigChange({ nodeSize: e.target.value })}
          className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        >
          {sizeOptions.map((o) => (
            <option key={o.value} value={o.value}>
              {t(o.labelKey)}
            </option>
          ))}
        </select>
      </div>

      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.stationsPerRow')}
        </label>
        <input
          type="number"
          min={0}
          max={20}
          step={1}
          value={stationsPerRow}
          data-testid="facility-line-stations-per-row"
          // 0 = 자동(nodeSize 기반 폴백). 1 이상이면 1줄당 역사 수를 직접 제어한다.
          onChange={(e) => onConfigChange({ stationsPerRow: Number(e.target.value) })}
          className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        />
      </div>
    </>
  );
}

/**
 * 역사/그룹 패널 공용 표시 옵션(config-only, UB-003). 그룹 패널은 역사 패널을 일반화한 것이라
 * 동일 옵션을 쓴다. testid 접두사는 panel.type 을 따른다(facility-station-* / facility-group-*).
 *   - showStats: "통계"(StatTiles) 섹션 표시(기본 true).
 *   - deviceLabelMode: 개별 기기 라벨(placeIndex=위치+번호 기본 / name=기기 이름).
 *   - offlineAsOff: 오프라인을 꺼짐으로 표시(기본 false, 라인 패널과 동일 옵션).
 */
function FacilityStationDisplayOptions({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const showStats = (panel.config?.showStats as boolean | undefined) ?? true;
  const deviceLabelMode = (panel.config?.deviceLabelMode as string | undefined) ?? 'placeIndex';
  const offlineAsOff = (panel.config?.offlineAsOff as boolean | undefined) ?? false;
  // testid 접두사: 역사=facility-station, 그룹=facility-group.
  const idp = panel.type;

  return (
    <>
      <label className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-(--color-bg-elevated)">
        <input
          type="checkbox"
          data-testid={`${idp}-show-stats`}
          checked={showStats}
          onChange={(e) => onConfigChange({ showStats: e.target.checked })}
          className="h-4 w-4 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500"
        />
        <span className="text-sm font-medium text-(--color-text-primary)">
          {t('dashboard.settings.showStats')}
        </span>
      </label>

      {/* 오프라인을 꺼짐으로 표시(item 2, 라인 패널과 동일한 i18n 키 재사용). */}
      <label className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-(--color-bg-elevated)">
        <input
          type="checkbox"
          data-testid={`${idp}-offline-as-off`}
          checked={offlineAsOff}
          onChange={(e) => onConfigChange({ offlineAsOff: e.target.checked })}
          className="h-4 w-4 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500"
        />
        <span className="text-sm font-medium text-(--color-text-primary)">
          {t('dashboard.settings.offlineAsOff')}
        </span>
      </label>

      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.deviceLabelMode')}
        </label>
        <select
          value={deviceLabelMode}
          data-testid={`${idp}-device-label-mode`}
          onChange={(e) => onConfigChange({ deviceLabelMode: e.target.value })}
          className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        >
          <option value="placeIndex">{t('dashboard.settings.deviceLabelModePlaceIndex')}</option>
          <option value="name">{t('dashboard.settings.deviceLabelModeName')}</option>
        </select>
      </div>
    </>
  );
}

/**
 * 값에 따른 색 규칙 편집기.
 *
 * 항목 카드(디바이스 현황)와 타일(에이전트 상태)이 같은 규칙을 쓴다. 각자 두면 "위에서
 * 먼저 맞는 것이 이긴다" 같은 규칙이 조용히 갈라진다.
 */
function ValueColorRules({
  rules,
  testIdPrefix,
  onChange,
}: {
  rules: ValueColorRule[];
  testIdPrefix: string;
  onChange: (rules: ValueColorRule[]) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="mt-2 border-t border-(--color-border-subtle) pt-2">
      <div className="mb-1.5 flex items-center justify-between gap-2">
        <span className="text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.propertiesGridOpt.valueColors')}
        </span>
        <button
          type="button"
          data-testid={`${testIdPrefix}-add`}
          onClick={() => onChange([...rules, { op: 'gte', value: '', color: '#ef4444' }])}
          className="rounded border border-(--color-border-default) px-1.5 py-0.5 text-[11px] text-(--color-text-secondary) hover:border-(--color-border-strong)"
        >
          {t('common.add')}
        </button>
      </div>
      {/* 위에서부터 먼저 맞는 규칙이 이긴다 — 순서가 곧 우선순위다. */}
      {rules.map((rule, index) => (
        <div key={index} className="mb-1 flex items-center gap-1">
          <select
            value={rule.op}
            data-testid={`${testIdPrefix}-op-${index}`}
            aria-label={t('dashboard.settings.propertiesGridOpt.valueColors')}
            onChange={(e) =>
              onChange(rules.map((r, i) => (i === index ? { ...r, op: e.target.value as ValueRuleOp } : r)))
            }
            className="w-20 shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-0.5 text-xs text-(--color-text-primary)"
          >
            {VALUE_RULE_OPS.map((op) => (
              <option key={op} value={op}>
                {t(`dashboard.settings.propertiesGridOpt.op.${op}`)}
              </option>
            ))}
          </select>
          <input
            type="text"
            value={rule.value}
            data-testid={`${testIdPrefix}-value-${index}`}
            onChange={(e) => onChange(rules.map((r, i) => (i === index ? { ...r, value: e.target.value } : r)))}
            className="min-w-0 flex-1 rounded border border-(--color-border-default) bg-(--color-bg-sunken) px-1.5 py-0.5 text-xs text-(--color-text-primary)"
          />
          <input
            type="color"
            value={rule.color}
            data-testid={`${testIdPrefix}-color-${index}`}
            aria-label={t('dashboard.settings.propertiesGridOpt.valueColors')}
            onChange={(e) => onChange(rules.map((r, i) => (i === index ? { ...r, color: e.target.value } : r)))}
            className="h-6 w-7 shrink-0 cursor-pointer rounded border border-(--color-border-default) bg-transparent p-0"
          />
          <button
            type="button"
            data-testid={`${testIdPrefix}-remove-${index}`}
            aria-label={t('common.delete')}
            onClick={() => onChange(rules.filter((_, i) => i !== index))}
            className="h-6 w-6 shrink-0 rounded border border-(--color-border-default) text-xs text-(--color-text-muted) hover:bg-(--color-bg-elevated)"
          >
            x
          </button>
        </div>
      ))}
    </div>
  );
}

/**
 * 항목별 세부 설정 편집기 — 글자 덮어쓰기 + 값에 따른 색.
 *
 * 카드 전체 설정만으로는 "온도만 크게" 나 "임계값을 넘으면 빨갛게" 를 만들 수 없다.
 * 지정하지 않은 항목은 카드 전체 설정을 그대로 따르므로, 비어 있는 동안은 이 설정이
 * 없는 것과 같다.
 */
function PropertyOverrideFields({
  propertyKey,
  override,
  base,
  fallbackLabel,
  withUnit,
  onChange,
}: {
  propertyKey: string;
  override: PropertyOverride;
  /** 단위 칸을 낼지. 디바이스가 보고하는 값(상태 정보)에만 낸다. */
  withUnit?: boolean;
  /** 이름을 정하지 않았을 때 보이는 기본 이름. */
  fallbackLabel: string;
  /** 카드 전체 배치 — 따로 잡지 않은 항목이 따르는 값. */
  base: Pick<PropertiesGridStyle, 'cardGrid' | 'areas'>;
  onChange: (next: PropertyOverride) => void;
}) {
  const { t } = useTranslation();
  const rules = override.valueColors ?? [];
  // 따로 잡은 값이 하나라도 있으면 "이 항목만" 상태다 — 켬/끔 플래그를 따로 두면
  // 플래그와 데이터가 어긋날 수 있다.
  const ownLayout =
    override.cardAreas !== undefined ||
    override.cardRows !== undefined ||
    override.cardCols !== undefined;
  const layout = resolveCardLayout(base, override);

  const setFont = (field: 'label_font' | 'value_font' | 'time_font') => (patch: Partial<PanelTitleFont>) => {
    const current = (override[field] as PanelTitleFont | undefined) ?? {};
    onChange({ ...override, [field]: mergeFont(current, patch) });
  };

  const setRules = (next: ValueColorRule[]) => onChange({ ...override, valueColors: next });

  return (
    <>
      {/*
        타일 이름. 기본 이름은 프로토콜과 종류에서 나오는데, 같은 값을 다르게 부르는
        현장이 있다 — 그때 이름만 바꿀 수단이 없으면 항목을 다시 만들 수도 없다.
      */}
      <label className="mb-1.5 flex items-center gap-1.5">
        <span className="shrink-0 text-xs text-(--color-text-muted)">
          {t('dashboard.settings.propertiesGridOpt.tileName')}
        </span>
        <input
          type="text"
          value={(override.label as string | undefined) ?? ''}
          data-testid={`property-name-${propertyKey}`}
          placeholder={fallbackLabel}
          onChange={(e) => onChange({ ...override, label: e.target.value || undefined })}
          className="min-w-0 flex-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-0.5 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
        />
      </label>
      {/*
        단위 — 수로 오는 값에 붙인다. 기본 정보(사람이 적어 둔 값)에는 붙일 단위가 없다.

        단위를 정하면 키 이름으로 짐작한 단위(온도의 °C)나 원래 단위(RSSI 의 dBm)를
        건너뛴다 — 그러지 않으면 "26.4°C K" 처럼 단위가 둘 붙는다. 빈칸의 흐린 글자가
        지금 붙는 단위를 알려 준다.
      */}
      {withUnit && (
        <label className="mb-1.5 flex items-center gap-1.5">
          <span className="shrink-0 text-xs text-(--color-text-muted)">
            {t('dashboard.settings.propertiesGridOpt.unit')}
          </span>
          <input
            type="text"
            value={(override.unit as string | undefined) ?? ''}
            data-testid={`property-unit-${propertyKey}`}
            placeholder={defaultUnitOf(propertyKey) ?? t('dashboard.settings.propertiesGridOpt.unitHint')}
            onChange={(e) => onChange({ ...override, unit: e.target.value || undefined })}
            className="min-w-0 flex-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-0.5 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
          />
        </label>
      )}
      {(['label_font', 'value_font', 'time_font'] as const).map((field) => (
        <TextStyleFields
          key={field}
          label={t(
            field === 'label_font'
              ? 'dashboard.settings.propertiesGridOpt.labelStyle'
              : field === 'value_font'
                ? 'dashboard.settings.propertiesGridOpt.valueStyle'
                : 'dashboard.settings.propertiesGridOpt.timeStyle',
          )}
          family={(override[field] as PanelTitleFont | undefined)?.family}
          size={(override[field] as PanelTitleFont | undefined)?.size}
          color={(override[field] as PanelTitleFont | undefined)?.color}
          weight={(override[field] as PanelTitleFont | undefined)?.weight ?? 'inherit'}
          align={(override[field] as PanelTitleFont | undefined)?.align ?? 'inherit'}
          sizePlaceholder={t('dashboard.chart.inherit')}
          testIdPrefix={`property-override-${propertyKey}-${field}`}
          onChange={setFont(field)}
        />
      ))}

      {/*
        이 항목만 쓰는 카드 배치. 끄면 카드 전체 배치를 그대로 따른다 — 항목 수만큼
        배치를 관리하게 만들지 않으려고 기본은 꺼짐이다.
      */}
      <div className="mt-2 border-t border-(--color-border-subtle) pt-2">
        <label className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-(--color-text-muted)">
          <input
            type="checkbox"
            data-testid={`property-own-layout-${propertyKey}`}
            checked={ownLayout}
            onChange={(e) =>
              onChange(
                e.target.checked
                  ? // 켜는 순간 전체 배치를 그대로 복사해 둔다 — 빈 상태에서 시작하면
                    // 방금까지 보이던 배치가 사라진 것처럼 보인다.
                    { ...override, cardRows: layout.grid.rows, cardCols: layout.grid.cols, cardAreas: layout.areas }
                  : { ...override, cardRows: undefined, cardCols: undefined, cardAreas: undefined },
              )
            }
            className="h-3.5 w-3.5 accent-blue-600"
          />
          {t('dashboard.settings.propertiesGridOpt.ownLayout')}
        </label>
        {ownLayout && (
          <>
            <div className="mb-1.5 flex items-center gap-1.5">
              <span className="text-[11px] text-(--color-text-secondary)">
                {t('dashboard.settings.propertiesGridOpt.rows')}
              </span>
              <input
                type="number"
                min={MIN_CARD_DIV}
                max={MAX_CARD_DIV}
                value={layout.grid.rows}
                data-testid={`property-card-rows-${propertyKey}`}
                aria-label={t('dashboard.settings.propertiesGridOpt.rows')}
                onChange={(e) => onChange({ ...override, cardRows: Number(e.target.value) })}
                className={cardNumberInputClass}
              />
              <span className="text-[11px] text-(--color-text-secondary)">
                {t('dashboard.settings.propertiesGridOpt.cols')}
              </span>
              <input
                type="number"
                min={MIN_CARD_DIV}
                max={MAX_CARD_DIV}
                value={layout.grid.cols}
                data-testid={`property-card-cols-${propertyKey}`}
                aria-label={t('dashboard.settings.propertiesGridOpt.cols')}
                onChange={(e) => onChange({ ...override, cardCols: Number(e.target.value) })}
                className={cardNumberInputClass}
              />
            </div>
            <CardLayoutEditor
              testId={`property-card-${propertyKey}`}
              grid={layout.grid}
              areas={layout.areas}
              fontOf={(element) => (override[CARD_ELEMENT_FONT_KEYS[element]] as PanelTitleFont | undefined) ?? {}}
              onAreasChange={(areas) => onChange({ ...override, cardAreas: areas })}
              onFontChange={(element, font) =>
                onChange({ ...override, [CARD_ELEMENT_FONT_KEYS[element]]: font })
              }
            />
          </>
        )}
      </div>

      <ValueColorRules
        testIdPrefix={`property-rule-${propertyKey}`}
        rules={rules}
        onChange={setRules}
      />

      {/*
        항목별 배경색. 정하지 않으면 공통 배경을 따른다 — 좁은 쪽이 이기는 것이 글자
        설정과 같은 규칙이다.
      */}
      <div className="mt-2 flex items-center gap-1.5 border-t border-(--color-border-subtle) pt-2">
        <span className="flex-1 text-xs text-(--color-text-muted)">
          {t('dashboard.settings.propertiesGridOpt.tileBackground')}
        </span>
        <input
          type="color"
          value={(override.bg as string | undefined) ?? '#1f2937'}
          data-testid={`property-bg-${propertyKey}`}
          aria-label={t('dashboard.settings.propertiesGridOpt.tileBackground')}
          onChange={(e) => onChange({ ...override, bg: e.target.value })}
          className="h-7 w-7 shrink-0 cursor-pointer rounded border border-(--color-border-default) bg-transparent p-0"
        />
        <button
          type="button"
          data-testid={`property-bg-reset-${propertyKey}`}
          aria-label={t('dashboard.chart.fontColorReset')}
          title={t('dashboard.chart.fontColorReset')}
          onClick={() => onChange({ ...override, bg: undefined })}
          className="h-7 w-7 shrink-0 rounded border border-(--color-border-default) text-xs text-(--color-text-muted) hover:bg-(--color-bg-elevated)"
        >
          x
        </button>
      </div>
    </>
  );
}

/** 카드 배치 숫자 입력의 공통 모양. */
const cardNumberInputClass =
  'w-14 shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-0.5 text-center text-xs text-(--color-text-primary) outline-none focus:border-blue-500';

/** 카드 조각 → 이름표 i18n 키. */
const CARD_ELEMENT_LABEL_KEYS: Record<CardElement, string> = {
  label: 'dashboard.settings.propertiesGridOpt.labelStyle',
  value: 'dashboard.settings.propertiesGridOpt.valueStyle',
  time: 'dashboard.settings.propertiesGridOpt.timeStyle',
};

/** 속성 그리드 패널 전용 설정 (열 수 · 배치 · 표시 항목) */
/** 카드 디자인 팝업의 조각별 라벨 — 배치 편집기의 라벨과 달리 "…글자"로 읽힌다. */
const CARD_ELEMENT_STYLE_LABEL_KEYS: Record<CardElement, string> = {
  label: 'dashboard.settings.propertiesGridOpt.labelStyle',
  value: 'dashboard.settings.propertiesGridOpt.valueStyle',
  time: 'dashboard.settings.propertiesGridOpt.timeStyle',
};

/** 기존 테스트·문서가 참조하는 테스트 아이디를 그대로 유지한다. */
const CARD_ELEMENT_TESTID_PREFIX: Record<CardElement, string> = {
  label: 'property-label-font',
  value: 'property-value-font',
  time: 'property-time-font',
};

/** 카드 조각별 글자 설정이 config 에 쓰이는 키. */
const CARD_ELEMENT_FONT_KEYS: Record<CardElement, 'label_font' | 'value_font' | 'time_font'> = {
  label: 'label_font',
  value: 'value_font',
  time: 'time_font',
};

/**
 * 카드 배치 편집기 — 끌어서 옮기고, 모서리를 끌어 칸 수를 바꾸고, 우클릭으로 그 조각의
 * 글자 설정을 연다.
 *
 * 숫자 입력만 있던 종전에는 "값을 위쪽 두 칸에" 를 만들려면 네 칸을 머릿속으로 계산해
 * 채워야 했다. 아래 숫자 입력은 그대로 둔다 — 눈으로 맞추는 길과 정확히 찍는 길은
 * 서로를 대신하지 못한다.
 */
function CardLayoutEditor({
  grid,
  areas,
  fontOf,
  onAreasChange,
  onFontChange,
  testId = 'card-layout-editor',
}: {
  grid: CardGrid;
  areas: CardAreas;
  /** 조각의 현재 글자 설정. 전체 설정과 항목별 설정 어느 쪽에도 붙일 수 있게 주입받는다. */
  fontOf: (element: CardElement) => PanelTitleFont;
  onAreasChange: (areas: CardAreas) => void;
  onFontChange: (element: CardElement, font: PanelTitleFont | undefined) => void;
  testId?: string;
}) {
  const { t } = useTranslation();
  const gridRef = useRef<HTMLDivElement>(null);
  const [menuFor, setMenuFor] = useState<CardElement | null>(null);
  // 끌기 시작 시점의 상태. 매 프레임 원본 영역에 누적 변위를 더해야 조금씩 밀리는
  // 오차가 쌓이지 않는다.
  const dragRef = useRef<{
    element: CardElement;
    mode: 'move' | 'resize';
    x: number;
    y: number;
    area: CardArea;
  } | null>(null);
  const [dragging, setDragging] = useState<CardElement | null>(null);

  const beginDrag = (element: CardElement, mode: 'move' | 'resize') => (e: React.MouseEvent) => {
    // 우클릭은 메뉴용이라 끌기를 시작하지 않는다.
    if (e.button !== 0) return;
    e.preventDefault();
    e.stopPropagation();
    dragRef.current = { element, mode, x: e.clientX, y: e.clientY, area: areas[element] };
    setDragging(element);
  };

  useEffect(() => {
    if (!dragging) return;
    const move = (e: MouseEvent): void => {
      const start = dragRef.current;
      const box = gridRef.current?.getBoundingClientRect();
      if (!start || !box || box.width <= 0 || box.height <= 0) return;
      // 칸 하나의 크기로 나눠 칸 단위 변위를 얻는다. 반올림이라 칸의 절반을 넘겨야 옮겨진다.
      const dCol = Math.round((e.clientX - start.x) / (box.width / grid.cols));
      const dRow = Math.round((e.clientY - start.y) / (box.height / grid.rows));
      const next =
        start.mode === 'move'
          ? moveArea(start.area, grid, dRow, dCol)
          : resizeArea(start.area, grid, dRow, dCol);
      const current = areas[start.element];
      if (
        next.row === current.row &&
        next.col === current.col &&
        next.rowSpan === current.rowSpan &&
        next.colSpan === current.colSpan
      ) {
        return;
      }
      onAreasChange({ ...areas, [start.element]: next });
    };
    const end = (): void => {
      dragRef.current = null;
      setDragging(null);
    };
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', end);
    return () => {
      window.removeEventListener('mousemove', move);
      window.removeEventListener('mouseup', end);
    };
  }, [dragging, areas, grid, onAreasChange]);

  // 바깥을 누르거나 Esc 를 치면 우클릭 메뉴를 닫는다.
  useEffect(() => {
    if (!menuFor) return;
    const close = (e: Event): void => {
      if (e instanceof KeyboardEvent && e.key !== 'Escape') return;
      if (e.type === 'mousedown' && gridRef.current?.contains(e.target as Node)) return;
      setMenuFor(null);
    };
    document.addEventListener('mousedown', close);
    document.addEventListener('keydown', close);
    return () => {
      document.removeEventListener('mousedown', close);
      document.removeEventListener('keydown', close);
    };
  }, [menuFor]);

  return (
    <div className="mb-2">
      <div
        ref={gridRef}
        data-testid={testId}
        className="grid aspect-[3/2] w-full gap-0.5 rounded-md border border-(--color-border-default) bg-(--color-bg-sunken) p-1"
        style={{
          gridTemplateRows: `repeat(${grid.rows}, minmax(0, 1fr))`,
          gridTemplateColumns: `repeat(${grid.cols}, minmax(0, 1fr))`,
        }}
      >
        {/* 빈 칸 바탕 — 어디에 놓을 수 있는지 보이게 한다. */}
        {Array.from({ length: grid.rows * grid.cols }, (_, i) => (
          <div
            key={`cell-${i}`}
            aria-hidden
            className="rounded-sm border border-dashed border-(--color-border-subtle)"
            style={{ gridRow: Math.floor(i / grid.cols) + 1, gridColumn: (i % grid.cols) + 1 }}
          />
        ))}
        {CARD_ELEMENTS.map((element) => {
          const area = areas[element];
          return (
            <div
              key={element}
              data-testid={`${testId === 'card-layout-editor' ? 'card-block' : testId}-${element}`}
              role="button"
              tabIndex={0}
              title={t('dashboard.settings.propertiesGridOpt.dragHint')}
              onMouseDown={beginDrag(element, 'move')}
              onContextMenu={(e) => {
                e.preventDefault();
                setMenuFor(element);
              }}
              className={cn(
                'relative flex select-none items-center justify-center rounded-sm border text-[11px] transition-colors',
                dragging === element
                  ? 'cursor-grabbing border-blue-500 bg-blue-500/25'
                  : 'cursor-grab border-blue-500/60 bg-blue-500/15 hover:bg-blue-500/25',
              )}
              style={{
                gridRow: `${area.row} / span ${area.rowSpan}`,
                gridColumn: `${area.col} / span ${area.colSpan}`,
              }}
            >
              <span className="truncate px-1 text-(--color-text-primary)">
                {t(CARD_ELEMENT_LABEL_KEYS[element])}
              </span>
              {/* 오른쪽 아래 모서리 — 칸 수를 바꾸는 손잡이. */}
              <span
                data-testid={`${testId === 'card-layout-editor' ? 'card-block' : testId}-${element}-resize`}
                onMouseDown={beginDrag(element, 'resize')}
                className="absolute bottom-0 right-0 h-2.5 w-2.5 cursor-se-resize rounded-br-sm bg-blue-500/70"
              />
              {menuFor === element && (
                <div
                  data-testid={`${testId === 'card-layout-editor' ? 'card-block' : testId}-${element}-design`}
                  onMouseDown={(e) => e.stopPropagation()}
                  onContextMenu={(e) => e.preventDefault()}
                  className="absolute left-0 top-full z-30 mt-1 w-64 space-y-1.5 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) p-2.5 text-left shadow-lg"
                >
                  <TextStyleFields
                    label={t(CARD_ELEMENT_LABEL_KEYS[element])}
                    family={fontOf(element).family}
                    size={fontOf(element).size}
                    color={fontOf(element).color}
                    weight={fontOf(element).weight ?? 'inherit'}
                    align={fontOf(element).align ?? 'inherit'}
                    sizePlaceholder={t('dashboard.chart.inherit')}
                    testIdPrefix={`${testId === 'card-layout-editor' ? 'card-block' : testId}-${element}-font`}
                    onChange={(patch) => onFontChange(element, mergeFont(fontOf(element), patch))}
                  />
                </div>
              )}
            </div>
          );
        })}
      </div>
      <p className="mt-1 text-[10px] text-(--color-text-muted)">
        {t('dashboard.settings.propertiesGridOpt.dragHint')}
      </p>
    </div>
  );
}

/**
 * 타일 격자 배치 편집기 — 격자 크기와 타일마다의 자리·크기.
 *
 * 배치 방식은 그래픽과 표 중 하나를 고른다(디바이스 현황의 카드 배치와 같은 규칙). 눈으로
 * 맞추는 길과 정확히 찍는 길은 서로를 대신하지 못하지만, 둘을 한꺼번에 펼치면 같은 값이
 * 두 벌 보여 어느 쪽이 정본인지 흐려진다.
 */
function TileGridEditor<T extends string>({
  grid,
  areas,
  items,
  labelKeys,
  styles,
  testIdPrefix,
  onGridChange,
  onAreasChange,
  onMove,
  onReset,
  onStylesChange,
  rawLabels,
  renderDesign,
}: {
  grid: TileGrid;
  areas: Record<T, TileArea>;
  items: readonly T[];
  labelKeys: Record<T, string>;
  /** 타일별 디자인. 넘기면 표에 디자인 팝업이 붙는다. */
  styles?: Record<string, unknown> | undefined;
  testIdPrefix: string;
  onGridChange: (grid: TileGrid) => void;
  onAreasChange: (areas: Record<string, TileArea>) => void;
  /** 차례 바꾸기(위=-1, 아래=+1). 넘기지 않으면 ↑↓ 를 쓸 수 없다. */
  onMove?: (item: T, direction: -1 | 1) => void;
  /** 격자와 자리를 미설정으로 되돌린다. 넘기지 않으면 초기화를 내지 않는다. */
  onReset?: () => void;
  onStylesChange?: (styles: Record<string, unknown>) => void;
  /** `labelKeys` 가 i18n 키가 아니라 이미 번역된 글자일 때. */
  rawLabels?: boolean;
  /** 표의 디자인 팝업 내용을 갈아끼운다. 넘기지 않으면 타일 기본 디자인을 쓴다. */
  renderDesign?: (item: T) => React.ReactNode;
}) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<'graphic' | 'table'>('graphic');
  const label = (item: T): string => (rawLabels ? labelKeys[item] : t(labelKeys[item]));
  const gridRef = useRef<HTMLDivElement>(null);
  const dragRef = useRef<{ item: T; mode: 'move' | 'resize'; x: number; y: number; area: TileArea } | null>(null);
  const [dragging, setDragging] = useState<T | null>(null);

  const beginDrag = (item: T, kind: 'move' | 'resize') => (e: React.MouseEvent) => {
    if (e.button !== 0) return;
    e.preventDefault();
    e.stopPropagation();
    const area = areas[item];
    if (!area) return;
    dragRef.current = { item, mode: kind, x: e.clientX, y: e.clientY, area };
    setDragging(item);
  };

  useEffect(() => {
    if (!dragging) return;
    const move = (e: MouseEvent): void => {
      const start = dragRef.current;
      const box = gridRef.current?.getBoundingClientRect();
      if (!start || !box || box.width <= 0 || box.height <= 0) return;
      // 칸 하나의 크기로 나눠 칸 단위 변위를 얻는다. 반올림이라 칸의 절반을 넘겨야 움직인다.
      const dx = Math.round((e.clientX - start.x) / (box.width / grid.cols));
      const dy = Math.round((e.clientY - start.y) / (box.height / grid.rows));
      const next =
        start.mode === 'move'
          ? moveTile(start.area, grid, dx, dy)
          : resizeTile(start.area, grid, dx, dy);
      const cur = areas[start.item];
      if (cur && next.x === cur.x && next.y === cur.y && next.w === cur.w && next.h === cur.h) return;
      onAreasChange({ ...areas, [start.item]: next });
    };
    const end = (): void => {
      dragRef.current = null;
      setDragging(null);
    };
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', end);
    return () => {
      window.removeEventListener('mousemove', move);
      window.removeEventListener('mouseup', end);
    };
  }, [dragging, areas, grid, onAreasChange]);

  const patch = (item: T, field: keyof TileArea, value: number) => {
    const area = areas[item];
    if (!area) return;
    // 표에서도 격자 밖으로 나가지 않게 같은 규칙으로 가둔다.
    const next =
      field === 'x' || field === 'y'
        ? moveTile(area, grid, field === 'x' ? value - area.x : 0, field === 'y' ? value - area.y : 0)
        : resizeTile(area, grid, field === 'w' ? value - area.w : 0, field === 'h' ? value - area.h : 0);
    onAreasChange({ ...areas, [item]: next });
  };

  return (
    <div className="space-y-2">
      {/* 격자 크기 */}
      <div className="flex items-center gap-2">
        <label className="text-xs text-(--color-text-secondary)">
          {t('dashboard.settings.propertiesGridOpt.rows')}
        </label>
        <input
          type="number"
          min={MIN_TILE_GRID}
          max={MAX_TILE_GRID}
          value={grid.rows}
          data-testid={`${testIdPrefix}-rows`}
          aria-label={t('dashboard.settings.propertiesGridOpt.rows')}
          onChange={(e) => onGridChange({ ...grid, rows: Number(e.target.value) })}
          className={cardNumberInputClass}
        />
        <label className="text-xs text-(--color-text-secondary)">
          {t('dashboard.settings.propertiesGridOpt.cols')}
        </label>
        <input
          type="number"
          min={MIN_TILE_GRID}
          max={MAX_TILE_GRID}
          value={grid.cols}
          data-testid={`${testIdPrefix}-cols`}
          aria-label={t('dashboard.settings.propertiesGridOpt.cols')}
          onChange={(e) => onGridChange({ ...grid, cols: Number(e.target.value) })}
          className={cardNumberInputClass}
        />
        <div className="ml-auto flex items-center gap-0.5 rounded border border-(--color-border-default) p-0.5">
          {(['graphic', 'table'] as const).map((m) => (
            <button
              key={m}
              type="button"
              data-testid={`${testIdPrefix}-mode-${m}`}
              aria-pressed={mode === m}
              onClick={() => setMode(m)}
              className={cn(
                'rounded px-1.5 py-0.5 text-[11px] transition-colors',
                mode === m
                  ? 'bg-blue-600 text-white'
                  : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
              )}
            >
              {t(
                m === 'graphic'
                  ? 'dashboard.settings.propertiesGridOpt.layoutGraphic'
                  : 'dashboard.settings.propertiesGridOpt.layoutTable',
              )}
            </button>
          ))}
        </div>
      </div>

      {mode === 'graphic' ? (
        <div
          ref={gridRef}
          data-testid={`${testIdPrefix}-canvas`}
          className="grid aspect-[2/1] w-full gap-0.5 rounded-md border border-(--color-border-default) bg-(--color-bg-sunken) p-1"
          style={{
            gridTemplateRows: `repeat(${grid.rows}, minmax(0, 1fr))`,
            gridTemplateColumns: `repeat(${grid.cols}, minmax(0, 1fr))`,
          }}
        >
          {/* 빈 칸 바탕 — 어디에 놓을 수 있는지 보이게 한다. */}
          {Array.from({ length: grid.rows * grid.cols }, (_, i) => (
            <div
              key={`cell-${i}`}
              aria-hidden
              className="rounded-sm border border-dashed border-(--color-border-subtle)"
              style={{ gridRow: Math.floor(i / grid.cols) + 1, gridColumn: (i % grid.cols) + 1 }}
            />
          ))}
          {items.map((item) => {
            const area = areas[item];
            if (!area) return null;
            return (
              <div
                key={item}
                data-testid={`${testIdPrefix}-tile-${item}`}
                role="button"
                tabIndex={0}
                title={t('dashboard.settings.propertiesGridOpt.dragHint')}
                onMouseDown={beginDrag(item, 'move')}
                className={cn(
                  'relative flex select-none items-center justify-center overflow-hidden rounded-sm border text-[10px] transition-colors',
                  dragging === item
                    ? 'cursor-grabbing border-blue-500 bg-blue-500/25'
                    : 'cursor-grab border-blue-500/60 bg-blue-500/15 hover:bg-blue-500/25',
                )}
                style={{
                  gridColumn: `${area.x} / span ${area.w}`,
                  gridRow: `${area.y} / span ${area.h}`,
                }}
              >
                <span className="truncate px-1 text-(--color-text-primary)">{label(item)}</span>
                <span
                  data-testid={`${testIdPrefix}-tile-${item}-resize`}
                  onMouseDown={beginDrag(item, 'resize')}
                  className="absolute bottom-0 right-0 h-2.5 w-2.5 cursor-se-resize rounded-br-sm bg-blue-500/70"
                />
              </div>
            );
          })}
        </div>
      ) : (
        <div className="space-y-1.5">
          <div className="flex items-center gap-1 text-[10px] text-(--color-text-muted)">
            <span className="w-12 shrink-0" />
            <span className="w-16 shrink-0" />
            {(['x', 'y', 'w', 'h'] as const).map((f) => (
              <span key={f} className="w-10 text-center uppercase">
                {f}
              </span>
            ))}
          </div>
          {items.map((item, index) => {
            const area = areas[item];
            if (!area) return null;
            const design = readTileDesign(styles, item);
            const font = (field: 'label_font' | 'value_font') =>
              (design[field] as PanelTitleFont | undefined) ?? {};
            const patchDesign = (next: TileDesign) =>
              onStylesChange?.({ ...(styles ?? {}), [item]: next });
            return (
              <div key={item} className="flex items-center gap-1">
                {/*
                  차례 바꾸기. 순번을 숫자로 찍던 종전에는 "3번을 1번으로" 를 머릿속으로
                  계산해야 했고, 자리를 눈으로 보며 고치는 이 표와도 어긋났다.
                */}
                <div className="flex w-12 shrink-0 gap-0.5">
                  <button
                    type="button"
                    data-testid={`${testIdPrefix}-up-${item}`}
                    aria-label={t('dashboard.settings.propertiesGridOpt.moveUp')}
                    title={t('dashboard.settings.propertiesGridOpt.moveUp')}
                    disabled={index === 0 || !onMove}
                    onClick={() => onMove?.(item, -1)}
                    className="h-5 w-5 rounded border border-(--color-border-default) text-[10px] text-(--color-text-muted) disabled:opacity-30 hover:bg-(--color-bg-elevated)"
                  >
                    ↑
                  </button>
                  <button
                    type="button"
                    data-testid={`${testIdPrefix}-down-${item}`}
                    aria-label={t('dashboard.settings.propertiesGridOpt.moveDown')}
                    title={t('dashboard.settings.propertiesGridOpt.moveDown')}
                    disabled={index === items.length - 1 || !onMove}
                    onClick={() => onMove?.(item, 1)}
                    className="h-5 w-5 rounded border border-(--color-border-default) text-[10px] text-(--color-text-muted) disabled:opacity-30 hover:bg-(--color-bg-elevated)"
                  >
                    ↓
                  </button>
                </div>
                <span className="w-16 shrink-0 truncate text-xs text-(--color-text-secondary)">
                  {label(item)}
                </span>
                {(['x', 'y', 'w', 'h'] as const).map((f) => (
                  <input
                    key={f}
                    type="number"
                    min={1}
                    max={f === 'x' || f === 'w' ? grid.cols : grid.rows}
                    value={area[f]}
                    data-testid={`${testIdPrefix}-area-${item}-${f}`}
                    aria-label={`${label(item)} ${f}`}
                    onChange={(e) => patch(item, f, Number(e.target.value))}
                    className="w-10 shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-0.5 text-center text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
                  />
                ))}
                {(onStylesChange || renderDesign) && (
                  <DesignPopover testId={`${testIdPrefix}-design-${item}`}>
                    {renderDesign?.(item)}
                    {/* 타이틀과 값을 따로 정한다 — 값만 크게 두는 것이 이 타일의 쓰임새다. */}
                    {!renderDesign && (
                    <>
                    <TextStyleFields
                      label={t('dashboard.settings.agentStatusOpt.tileLabel')}
                      family={font('label_font').family}
                      size={font('label_font').size}
                      color={font('label_font').color}
                      weight={font('label_font').weight ?? 'inherit'}
                      align={font('label_font').align ?? 'inherit'}
                      sizePlaceholder={t('dashboard.chart.inherit')}
                      testIdPrefix={`${testIdPrefix}-label-font-${item}`}
                      onChange={(p) =>
                        patchDesign({ ...design, label_font: mergeFont(font('label_font'), p) })
                      }
                    />
                    <TextStyleFields
                      label={t('dashboard.settings.agentStatusOpt.tileValue')}
                      family={font('value_font').family}
                      size={font('value_font').size}
                      color={font('value_font').color}
                      weight={font('value_font').weight ?? 'inherit'}
                      align={font('value_font').align ?? 'inherit'}
                      sizePlaceholder={t('dashboard.chart.inherit')}
                      testIdPrefix={`${testIdPrefix}-value-font-${item}`}
                      onChange={(p) =>
                        patchDesign({ ...design, value_font: mergeFont(font('value_font'), p) })
                      }
                    />
                    <ValueColorRules
                      testIdPrefix={`${testIdPrefix}-rule-${item}`}
                      rules={design.valueColors ?? []}
                      onChange={(rules) => patchDesign({ ...design, valueColors: rules })}
                    />
                    <div className="mt-1.5 flex items-center gap-1.5">
                      <span className="flex-1 text-xs text-(--color-text-muted)">
                        {t('dashboard.settings.listPanel.tileBackground')}
                      </span>
                      <input
                        type="color"
                        value={design.bg ?? '#3b82f6'}
                        data-testid={`${testIdPrefix}-bg-${item}`}
                        aria-label={t('dashboard.settings.listPanel.tileBackground')}
                        onChange={(e) => patchDesign({ ...design, bg: e.target.value })}
                        className="h-7 w-7 shrink-0 cursor-pointer rounded border border-(--color-border-default) bg-transparent p-0"
                      />
                      <button
                        type="button"
                        data-testid={`${testIdPrefix}-bg-reset-${item}`}
                        aria-label={t('dashboard.chart.fontColorReset')}
                        title={t('dashboard.chart.fontColorReset')}
                        onClick={() => patchDesign({ ...design, bg: undefined })}
                        className="h-7 w-7 shrink-0 rounded border border-(--color-border-default) text-xs text-(--color-text-muted) hover:bg-(--color-bg-elevated)"
                      >
                        x
                      </button>
                    </div>
                    </>
                    )}
                  </DesignPopover>
                )}
              </div>
            );
          })}
        </div>
      )}
      <p className="text-[10px] text-(--color-text-muted)">
        {t('dashboard.settings.propertiesGridOpt.dragHint')}
      </p>
      {/*
        초기화 — 격자와 자리를 한꺼번에 미설정으로 되돌린다. 타일을 여기저기 옮겨 놓은
        뒤에는 처음 배치를 손으로 되짚을 수 없다.
      */}
      {onReset && (
        <button
          type="button"
          data-testid={`${testIdPrefix}-reset`}
          onClick={onReset}
          className="w-full rounded border border-(--color-border-default) py-1 text-xs text-(--color-text-secondary) transition-colors hover:border-(--color-border-strong)"
        >
          {t('dashboard.settings.propertiesGridOpt.reset')}
        </button>
      )}
    </div>
  );
}

/**
 * 타일 선택 목록.
 *
 * `styles` 를 넘기면 한 줄에 순번과 디자인 팝업까지 낸다(요약 배지처럼 레이아웃 팝업이
 * 없는 자리). `onDesignChange` 를 넘기면 줄마다 타일 디자인 팝업을 낸다 — 켜고 끄는
 * 것과 같은 줄에 있어야 어느 타일의 설정인지 헷갈리지 않는다(디바이스 상태와 같은 규칙).
 */
function TileListEditor<T extends string>({
  all,
  labelKeys,
  items,
  styles,
  testIdPrefix,
  onItemsChange,
  onStylesChange,
  onDesignChange,
}: {
  all: readonly T[];
  labelKeys: Record<T, string>;
  /** 지금 고른 타일과 순서. */
  items: T[];
  /** 타일별 글자·배경 설정. */
  styles?: Record<string, unknown> | undefined;
  testIdPrefix: string;
  onItemsChange: (items: T[]) => void;
  /** 넘기면 한 줄에 순번 + 옛 형태 디자인 팝업을 낸다(요약 배지 전용). */
  onStylesChange?: (styles: Record<string, unknown>) => void;
  /** 넘기면 한 줄에 타일 디자인(타이틀·값·값 색·배경) 팝업을 낸다. */
  onDesignChange?: (item: T, design: TileDesign) => void;
}) {
  const { t } = useTranslation();
  const withOrder = onStylesChange !== undefined;

  const toggle = (item: T) =>
    onItemsChange(items.includes(item) ? items.filter((i) => i !== item) : [...items, item]);

  const patchFont = (item: T, next: Record<string, unknown>) =>
    onStylesChange?.({ ...(styles ?? {}), [item]: next });

  return (
    <div className="mt-2 space-y-1">
      {/* 한꺼번에 켜고 끄는 버튼 — 항목이 많으면 하나씩 누르는 것이 현실적이지 않다. */}
      <div className="mb-1.5 flex items-center gap-1">
        <button
          type="button"
          data-testid={`${testIdPrefix}-select-all`}
          onClick={() => onItemsChange([...all])}
          className="rounded border border-(--color-border-default) px-1.5 py-0.5 text-[11px] text-(--color-text-secondary) transition-colors hover:border-(--color-border-strong)"
        >
          {t('dashboard.settings.selectAll')}
        </button>
        <button
          type="button"
          data-testid={`${testIdPrefix}-clear-all`}
          onClick={() => onItemsChange([])}
          className="rounded border border-(--color-border-default) px-1.5 py-0.5 text-[11px] text-(--color-text-secondary) transition-colors hover:border-(--color-border-strong)"
        >
          {t('dashboard.settings.propertiesGridOpt.clearAll')}
        </button>
      </div>
      {all.map((item) => {
        const order = items.indexOf(item);
        const font = readTileFont(styles, item) as PanelTitleFont & { bg?: string };
        const design = readTileDesign(styles, item);
        return (
          <div key={item} className="flex items-center gap-2">
            <label className="flex min-w-0 flex-1 cursor-pointer items-center gap-2">
              <input
                type="checkbox"
                data-testid={`${testIdPrefix}-${item}`}
                checked={order >= 0}
                onChange={() => toggle(item)}
                className="h-3.5 w-3.5 shrink-0 accent-blue-600"
              />
              <span className="truncate text-xs text-(--color-text-primary)">
                {t(labelKeys[item])}
              </span>
            </label>
            {/* 자리를 잡을 수 없는(꺼진) 타일에는 설정을 낼 이유가 없다. */}
            {onDesignChange && (
              <div className={order >= 0 ? undefined : 'invisible'}>
                <DesignPopover testId={`${testIdPrefix}-design-${item}`}>
                  <TileDesignFields
                    testIdPrefix={`${testIdPrefix}-${item}`}
                    design={design}
                    onChange={(next) => onDesignChange(item, next)}
                  />
                </DesignPopover>
              </div>
            )}
            {withOrder && (
              <>
                {/* 자리는 늘 잡는다 — 체크할 때마다 행 높이가 바뀌면 목록이 흔들린다. */}
                <div className={order >= 0 ? undefined : 'invisible'}>
                  <input
                    type="number"
                    min={1}
                    max={Math.max(1, items.length)}
                    value={order >= 0 ? order + 1 : 1}
                    disabled={order < 0}
                    data-testid={`${testIdPrefix}-order-${item}`}
                    aria-label={t('dashboard.settings.propertiesGridOpt.order')}
                    onChange={(e) => onItemsChange(moveToPosition(items, item, Number(e.target.value)))}
                    className="w-14 shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-0.5 text-center text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
                  />
                </div>
                <DesignPopover testId={`${testIdPrefix}-design-${item}`}>
                  <TextStyleFields
                    label={t(labelKeys[item])}
                    family={font.family}
                    size={font.size}
                    color={font.color}
                    weight={font.weight ?? 'inherit'}
                    align={font.align ?? 'inherit'}
                    sizePlaceholder={t('dashboard.chart.inherit')}
                    testIdPrefix={`${testIdPrefix}-font-${item}`}
                    onChange={(patch) => patchFont(item, { ...font, ...mergeFont(font, patch) })}
                  />
                  <div className="mt-1.5 flex items-center gap-1.5">
                    <span className="flex-1 text-xs text-(--color-text-muted)">
                      {t('dashboard.settings.listPanel.tileBackground')}
                    </span>
                    <input
                      type="color"
                      value={font.bg ?? '#3b82f6'}
                      data-testid={`${testIdPrefix}-bg-${item}`}
                      aria-label={t('dashboard.settings.listPanel.tileBackground')}
                      onChange={(e) => patchFont(item, { ...font, bg: e.target.value })}
                      className="h-7 w-7 shrink-0 cursor-pointer rounded border border-(--color-border-default) bg-transparent p-0"
                    />
                    <button
                      type="button"
                      data-testid={`${testIdPrefix}-bg-reset-${item}`}
                      aria-label={t('dashboard.chart.fontColorReset')}
                      title={t('dashboard.chart.fontColorReset')}
                      onClick={() => patchFont(item, { ...font, bg: undefined })}
                      className="h-7 w-7 shrink-0 rounded border border-(--color-border-default) text-xs text-(--color-text-muted) hover:bg-(--color-bg-elevated)"
                    >
                      x
                    </button>
                  </div>
                </DesignPopover>
              </>
            )}
          </div>
        );
      })}
    </div>
  );
}

/**
 * 타일 디자인 입력 묶음 — 타이틀 글자 · 값 글자 · 값에 따른 색 · 배경색.
 *
 * 공통 설정과 타일별 설정이 같은 입력을 쓴다. 두 벌을 두면 "공통에서 되는 것이 타일별에서
 * 안 되는" 자리가 조용히 생긴다.
 */
function TileDesignFields({
  design,
  testIdPrefix,
  onChange,
  onReset,
}: {
  design: TileDesign;
  testIdPrefix: string;
  onChange: (design: TileDesign) => void;
  onReset?: () => void;
}) {
  const { t } = useTranslation();
  const font = (field: 'label_font' | 'value_font') =>
    (design[field] as PanelTitleFont | undefined) ?? {};

  return (
    <>
      {(['label_font', 'value_font'] as const).map((field) => (
        <TextStyleFields
          key={field}
          label={t(
            field === 'label_font'
              ? 'dashboard.settings.agentStatusOpt.tileLabel'
              : 'dashboard.settings.agentStatusOpt.tileValue',
          )}
          family={font(field).family}
          size={font(field).size}
          color={font(field).color}
          weight={font(field).weight ?? 'inherit'}
          align={font(field).align ?? 'inherit'}
          sizePlaceholder={t('dashboard.chart.inherit')}
          testIdPrefix={`${testIdPrefix}-${field}`}
          onChange={(patch) => onChange({ ...design, [field]: mergeFont(font(field), patch) })}
        />
      ))}
      <ValueColorRules
        testIdPrefix={`${testIdPrefix}-rule`}
        rules={design.valueColors ?? []}
        onChange={(rules) => onChange({ ...design, valueColors: rules })}
      />
      <div className="mt-1.5 flex items-center gap-1.5">
        <span className="flex-1 text-xs text-(--color-text-muted)">
          {t('dashboard.settings.propertiesGridOpt.tileBackground')}
        </span>
        <input
          type="color"
          value={design.bg ?? '#1f2937'}
          data-testid={`${testIdPrefix}-bg`}
          aria-label={t('dashboard.settings.propertiesGridOpt.tileBackground')}
          onChange={(e) => onChange({ ...design, bg: e.target.value })}
          className="h-7 w-7 shrink-0 cursor-pointer rounded border border-(--color-border-default) bg-transparent p-0"
        />
        <button
          type="button"
          data-testid={`${testIdPrefix}-bg-reset`}
          aria-label={t('dashboard.chart.fontColorReset')}
          title={t('dashboard.chart.fontColorReset')}
          onClick={() => onChange({ ...design, bg: undefined })}
          className="h-7 w-7 shrink-0 rounded border border-(--color-border-default) text-xs text-(--color-text-muted) hover:bg-(--color-bg-elevated)"
        >
          x
        </button>
      </div>
      {onReset && (
        <div className="mt-2 border-t border-(--color-border-subtle) pt-2">
          <button
            type="button"
            data-testid={`${testIdPrefix}-reset`}
            onClick={onReset}
            className="w-full rounded border border-(--color-border-default) py-1 text-xs text-(--color-text-secondary) transition-colors hover:border-(--color-border-strong)"
          >
            {t('dashboard.settings.propertiesGridOpt.reset')}
          </button>
        </div>
      )}
    </>
  );
}

/** 요약 타일 이름 — 패널이 그리는 라벨과 같은 키를 쓴다. */
const SUMMARY_TILE_LABEL_KEYS: Record<SummaryItem, string> = {
  total: 'dashboard.panel.total',
  active: 'dashboard.panel.active',
  inactive: 'dashboard.panel.inactive',
};

/**
 * 카드 디자인 팝업 본문 — 분할 · 배치 · 조각별 글자.
 *
 * 배치는 그래픽 편집기와 표 중 하나를 고른다. 눈으로 맞추는 길과 정확히 찍는 길은 서로를
 * 대신하지 못하지만, 둘을 한꺼번에 펼쳐 두면 같은 값을 두 벌 보여 주게 되어 어느 쪽이
 * 정본인지 흐려진다. 고른 방식은 팝업이 열려 있는 동안만 기억한다 — 이것은 패널 데이터가
 * 아니라 보는 방식이라 저장해서 다른 사람 화면까지 바꿀 이유가 없다.
 */
function CardDesignPopoverBody({
  design,
  config,
  onConfigChange,
}: {
  design: PropertiesGridStyle;
  config: Record<string, unknown> | undefined;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<'graphic' | 'table'>('graphic');

  const fontOf = (element: CardElement): PanelTitleFont =>
    (config?.[CARD_ELEMENT_FONT_KEYS[element]] as PanelTitleFont | undefined) ?? {};

  return (
    <div className="space-y-2.5">
      {/* 카드 분할 — 카드 한 장을 나누는 수. 대시보드 격자(열 수)와 다른 축이다. */}
      <div>
        <span className="mb-1 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.propertiesGridOpt.cardDivision')}
        </span>
        <div className="flex items-center gap-2">
          <label htmlFor="card-rows" className="text-xs text-(--color-text-secondary)">
            {t('dashboard.settings.propertiesGridOpt.rows')}
          </label>
          <input
            id="card-rows"
            type="number"
            min={MIN_CARD_DIV}
            max={MAX_CARD_DIV}
            value={design.cardGrid.rows}
            data-testid="card-rows"
            onChange={(e) => onConfigChange({ cardRows: Number(e.target.value) })}
            className={cardNumberInputClass}
          />
          <label htmlFor="card-cols" className="text-xs text-(--color-text-secondary)">
            {t('dashboard.settings.propertiesGridOpt.cols')}
          </label>
          <input
            id="card-cols"
            type="number"
            min={MIN_CARD_DIV}
            max={MAX_CARD_DIV}
            value={design.cardGrid.cols}
            data-testid="card-cols"
            onChange={(e) => onConfigChange({ cardCols: Number(e.target.value) })}
            className={cardNumberInputClass}
          />
        </div>
      </div>

      {/*
        배치 — 조각마다 시작 행·열과 쓸 칸 수를 정한다. 영역이 겹치면 먼저 오는 조각이
        갖고 뒤 조각은 빈 칸으로 밀린다.
      */}
      <div className="border-t border-(--color-border-subtle) pt-2">
        <div className="mb-1.5 flex items-center justify-between gap-2">
          <span className="text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.settings.propertiesGridOpt.layout')}
          </span>
          <div className="flex items-center gap-0.5 rounded border border-(--color-border-default) p-0.5">
            {(['graphic', 'table'] as const).map((m) => (
              <button
                key={m}
                type="button"
                data-testid={`card-layout-mode-${m}`}
                aria-pressed={mode === m}
                onClick={() => setMode(m)}
                className={cn(
                  'rounded px-1.5 py-0.5 text-[11px] transition-colors',
                  mode === m
                    ? 'bg-blue-600 text-white'
                    : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
                )}
              >
                {t(
                  m === 'graphic'
                    ? 'dashboard.settings.propertiesGridOpt.layoutGraphic'
                    : 'dashboard.settings.propertiesGridOpt.layoutTable',
                )}
              </button>
            ))}
          </div>
        </div>
        {mode === 'graphic' ? (
          <CardLayoutEditor
            grid={design.cardGrid}
            areas={design.areas}
            fontOf={fontOf}
            onAreasChange={(areas) => onConfigChange({ cardAreas: areas })}
            onFontChange={(element, font) =>
              onConfigChange({ [CARD_ELEMENT_FONT_KEYS[element]]: font })
            }
          />
        ) : (
          <div className="space-y-1.5">
            <div className="flex items-center gap-1.5 text-[10px] text-(--color-text-muted)">
              <span className="w-16 shrink-0" />
              <span className="w-14 text-center">{t('dashboard.settings.propertiesGridOpt.startRow')}</span>
              <span className="w-14 text-center">{t('dashboard.settings.propertiesGridOpt.startCol')}</span>
              <span className="w-14 text-center">{t('dashboard.settings.propertiesGridOpt.rowSpan')}</span>
              <span className="w-14 text-center">{t('dashboard.settings.propertiesGridOpt.colSpan')}</span>
            </div>
            {CARD_ELEMENTS.map((element) => {
              const area = design.areas[element];
              const patch = (field: 'row' | 'col' | 'rowSpan' | 'colSpan', value: number) =>
                onConfigChange({
                  cardAreas: { ...design.areas, [element]: { ...area, [field]: value } },
                });
              return (
                <div key={element} className="flex items-center gap-1.5">
                  <span className="w-16 shrink-0 truncate text-xs text-(--color-text-secondary)">
                    {t(CARD_ELEMENT_LABEL_KEYS[element])}
                  </span>
                  {(
                    [
                      ['row', area.row, design.cardGrid.rows],
                      ['col', area.col, design.cardGrid.cols],
                      ['rowSpan', area.rowSpan, design.cardGrid.rows],
                      ['colSpan', area.colSpan, design.cardGrid.cols],
                    ] as const
                  ).map(([field, value, max]) => (
                    <input
                      key={field}
                      type="number"
                      min={1}
                      max={max}
                      value={value}
                      data-testid={`card-area-${element}-${field}`}
                      aria-label={`${t(CARD_ELEMENT_LABEL_KEYS[element])} ${field}`}
                      onChange={(e) => patch(field, Number(e.target.value))}
                      className={cardNumberInputClass}
                    />
                  ))}
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* 조각별 글자 — 타일 안 세 조각에 함께 걸린다(항목별 덮어쓰기는 표시 항목 쪽). */}
      <div className="border-t border-(--color-border-subtle) pt-2">
        <span className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.propertiesGridOpt.cardDesign')}
        </span>
        {CARD_ELEMENTS.map((element) => (
          <TextStyleFields
            key={element}
            label={t(CARD_ELEMENT_STYLE_LABEL_KEYS[element])}
            family={fontOf(element).family}
            size={fontOf(element).size}
            color={fontOf(element).color}
            weight={fontOf(element).weight ?? 'inherit'}
            align={fontOf(element).align ?? 'inherit'}
            sizePlaceholder={t('dashboard.chart.inherit')}
            testIdPrefix={CARD_ELEMENT_TESTID_PREFIX[element]}
            onChange={(patch) =>
              onConfigChange({
                [CARD_ELEMENT_FONT_KEYS[element]]: mergeFont(fontOf(element), patch),
              })
            }
          />
        ))}
        {/*
          타일 배경색. 정하지 않으면 패널 기본 배경이 그대로 산다 — 테마를 바꿔도 함께
          따라가는 값이라, 굳이 고정할 이유가 없으면 두지 않는 편이 낫다.
        */}
        <div className="mt-1.5 flex items-center gap-1.5">
          <span className="flex-1 text-xs text-(--color-text-muted)">
            {t('dashboard.settings.propertiesGridOpt.tileBackground')}
          </span>
          <input
            type="color"
            value={(config?.tileBg as string | undefined) ?? '#1f2937'}
            data-testid="properties-grid-tile-bg"
            aria-label={t('dashboard.settings.propertiesGridOpt.tileBackground')}
            onChange={(e) => onConfigChange({ tileBg: e.target.value })}
            className="h-7 w-7 shrink-0 cursor-pointer rounded border border-(--color-border-default) bg-transparent p-0"
          />
          <button
            type="button"
            data-testid="properties-grid-tile-bg-reset"
            aria-label={t('dashboard.chart.fontColorReset')}
            title={t('dashboard.chart.fontColorReset')}
            onClick={() => onConfigChange({ tileBg: undefined })}
            className="h-7 w-7 shrink-0 rounded border border-(--color-border-default) text-xs text-(--color-text-muted) hover:bg-(--color-bg-elevated)"
          >
            x
          </button>
        </div>
      </div>

      {/*
        초기화 — 분할·배치·글자·배경을 한꺼번에 미설정으로 되돌린다. 하나씩 되돌리려면
        어느 값이 기본이었는지 기억해야 하는데, 그것을 화면이 알려 주지 않는다.
      */}
      <div className="border-t border-(--color-border-subtle) pt-2">
        <button
          type="button"
          data-testid="properties-grid-card-reset"
          onClick={() =>
            onConfigChange({
              cardRows: undefined,
              cardCols: undefined,
              cardAreas: undefined,
              label_font: undefined,
              value_font: undefined,
              time_font: undefined,
              tileBg: undefined,
            })
          }
          className="w-full rounded border border-(--color-border-default) py-1 text-xs text-(--color-text-secondary) transition-colors hover:border-(--color-border-strong)"
        >
          {t('dashboard.settings.propertiesGridOpt.reset')}
        </button>
      </div>
    </div>
  );
}

function PropertiesGridSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const design = readPropertiesGridStyle(panel.config);
  const visibleProperties = (panel.config?.visibleProperties as string[] | undefined) ?? [];
  const deviceId = panel.config?.deviceId as string | undefined;
  const { data: deviceData } = useDeviceRealtime(deviceId ?? '');
  const protocol = deviceData?.protocol ?? '';
  const type = deviceData?.type ?? '';

  // **디바이스 종류가 표현할 수 있는** 속성을 나열한다. 종전에는 지금까지 보고된 키만
  // 골라, 아직 값이 오지 않은 속성은 켜 둘 방법이 없었고 값이 처음 도착하는 순간
  // 갑자기 나타났다.
  // 패널이 카드를 만드는 그 경로를 그대로 쓴다 — 여기서 고를 수 있는 항목과 화면에
  // 뜨는 카드가 갈라질 수 없다. 종전에는 프로토콜 라벨표로 목록을 채워, 이 디바이스가
  // 보고하지 않는 항목이나 전용 섹션이 그리는 키까지 고를 수 있었다.
  const reportedKeys = listDisplayableProperties(
    deviceData
      ? {
          properties: deviceData.state?.properties,
          id: deviceData.id,
          metadata: deviceData.metadata,
        }
      : undefined,
  );

  // 고른 항목 중 디바이스가 아직 보고하지 않은 것도 목록에 낸다. 그러지 않으면 첫 통신
  // 전에는 상태 정보 그룹이 통째로 사라져, 화면에는 카드가 보이는데 그것을 끌 자리가
  // 없다. 파생 항목은 모두 열거되므로 목록에 없다면 존재하지 않는 키다(제외).
  const allKeys = [
    ...reportedKeys,
    ...visibleProperties.filter((k) => !reportedKeys.includes(k) && !isDerivedPropertyKey(k)),
  ];

  // 표시 목록이 비어 있으면 "디바이스가 보고하는 속성 전부" 라는 뜻이다(파생 카드 제외).
  // 그룹별로 켜고 끄려면 그 뜻을 실제 목록으로 펴 두어야 한다 — 비어 있는 채로 한 항목만
  // 끄면 나머지가 함께 사라진다.
  const shownKeys =
    visibleProperties.length > 0 ? visibleProperties : allKeys.filter((k) => !isDerivedPropertyKey(k));

  return (
    <>
      {/*
        배지 — 무엇을 낼지 고르고 항목마다 모양을 정한다. 타이틀 옆에 글자로 붙어 있던
        프로토콜이 여기로 왔다.
      */}
      <div className="mt-3">
        <label className="flex items-center gap-2 text-xs text-(--color-text-secondary)">
          <input
            type="checkbox"
            data-testid="properties-grid-show-badges"
            checked={panel.config?.showBadges !== false}
            onChange={(e) =>
              // 기본은 표시다 — 끌 때만 config 에 남긴다.
              onConfigChange({ showBadges: e.target.checked ? undefined : false })
            }
            className="h-3.5 w-3.5 accent-blue-600"
          />
          {t('dashboard.settings.propertiesGridOpt.showBadges')}
        </label>
        {panel.config?.showBadges !== false && (
          <TileListEditor<DeviceBadge>
            all={DEVICE_BADGES}
            labelKeys={DEVICE_BADGE_LABEL_KEYS}
            items={readTileItems(panel.config?.badgeItems, DEVICE_BADGES, DEVICE_BADGE_DEFAULT)}
            styles={panel.config?.badgeStyles as Record<string, unknown> | undefined}
            testIdPrefix="device-badge"
            onItemsChange={(items) => onConfigChange({ badgeItems: items })}
            onDesignChange={(item, next) =>
              onConfigChange({
                badgeStyles: {
                  ...((panel.config?.badgeStyles as Record<string, unknown>) ?? {}),
                  [item]: next,
                },
              })
            }
          />
        )}
      </div>

      {/*
        타일 한 장에 관한 설정을 한 무리로 모은다. 종전에는 분할 · 배치 · 글자가 각각
        따로 놓여, 같은 타일을 고치는데 세 자리를 오가야 했다.

        격자 열 수는 여기 없다 — 그룹마다 제 격자를 가지므로 패널 전체의 열 수를 따로
        정할 자리가 없어졌다.
      */}
      <div className="mt-3">
        <div className="mb-1.5 flex items-center gap-1.5">
          <span className="text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.settings.propertiesGridOpt.card')}
          </span>
          <DesignPopover testId="properties-grid-card" width={384}>
            <CardDesignPopoverBody
              design={design}
              config={panel.config}
              onConfigChange={onConfigChange}
            />
          </DesignPopover>
        </div>
        {/* 자주 켜고 끄는 것이라 팝업 밖에 둔다. */}
        <label className="flex items-center gap-2 text-xs text-(--color-text-secondary)">
          <input
            type="checkbox"
            data-testid="properties-grid-show-updated"
            checked={design.showUpdatedAt}
            onChange={(e) =>
              // 기본은 표시다 — 끌 때만 config 에 남긴다.
              onConfigChange({ showUpdatedAt: e.target.checked ? undefined : false })
            }
            className="h-3.5 w-3.5 accent-blue-600"
          />
          {t('dashboard.settings.propertiesGridOpt.showUpdatedAt')}
        </label>
      </div>

      {/*
        표시 항목을 그룹별로 나눈다. 한 목록에 스무 개를 넘게 늘어놓으면 어느 것이 어느
        그룹인지 알 수 없고, 그룹마다 격자가 따로인데 목록만 한 줄이면 고른 것이 어디에
        놓이는지도 읽히지 않는다.
      */}
      {PROPERTY_GROUPS.map((group) => {
        const groupKeys = allKeys.filter((k) => propertyGroupOf(k) === group);
        if (groupKeys.length === 0) return null;
        const shown = groupKeys.filter((k) => shownKeys.includes(k));
        const grid = readTileGrid(panel.config?.[PROPERTY_GROUP_GRID_KEY[group]], PROPERTY_GROUP_GRID[group]);
        return (
          <div key={group} className="mt-3">
            <div className="mb-1.5 flex items-center gap-1.5">
              <span className="text-xs font-medium text-(--color-text-muted)">
                {t(PROPERTY_GROUP_LABEL_KEYS[group])}
              </span>
              {/* 고른 것이 없으면 놓을 자리도 없다 — 그때는 레이아웃을 내지 않는다. */}
              {shown.length > 0 && (
                <DesignPopover
                  testId={`properties-grid-${group}-layout`}
                  label={t('dashboard.settings.propertiesGridOpt.layout')}
                  width={384}
                >
                  <TileGridEditor<string>
                    grid={grid}
                    items={shown}
                    areas={placeTiles(
                      shown,
                      panel.config?.[PROPERTY_GROUP_AREA_KEY[group]] as
                        | Record<string, Partial<TileArea>>
                        | undefined,
                      grid,
                      PROPERTY_TILE_SIZE,
                    )}
                    labelKeys={Object.fromEntries(
                      shown.map((k) => [
                        k,
                        resolveTileLabel(
                          readPropertyOverride(panel.config, k),
                          getPropertyLabel(k, protocol, type),
                        ),
                      ]),
                    )}
                    rawLabels
                    testIdPrefix={`properties-grid-${group}`}
                    onGridChange={(g) => onConfigChange({ [PROPERTY_GROUP_GRID_KEY[group]]: g })}
                    onAreasChange={(areas) => onConfigChange({ [PROPERTY_GROUP_AREA_KEY[group]]: areas })}
                    onMove={(key, direction) =>
                      onConfigChange({
                        visibleProperties: moveWithinGroup(shownKeys, key, direction),
                      })
                    }
                    onReset={() =>
                      onConfigChange({
                        [PROPERTY_GROUP_GRID_KEY[group]]: undefined,
                        [PROPERTY_GROUP_AREA_KEY[group]]: undefined,
                      })
                    }
                  />
                </DesignPopover>
              )}
              <div className="ml-auto flex items-center gap-1">
                <button
                  type="button"
                  data-testid={`properties-grid-${group}-select-all`}
                  onClick={() =>
                    onConfigChange({
                      visibleProperties: setGroupSelection(shownKeys, groupKeys, true),
                    })
                  }
                  className="rounded border border-(--color-border-default) px-1.5 py-0.5 text-[11px] text-(--color-text-secondary) transition-colors hover:border-(--color-border-strong)"
                >
                  {t('dashboard.settings.selectAll')}
                </button>
                <button
                  type="button"
                  data-testid={`properties-grid-${group}-clear-all`}
                  onClick={() =>
                    onConfigChange({
                      visibleProperties: setGroupSelection(shownKeys, groupKeys, false),
                    })
                  }
                  className="rounded border border-(--color-border-default) px-1.5 py-0.5 text-[11px] text-(--color-text-secondary) transition-colors hover:border-(--color-border-strong)"
                >
                  {t('dashboard.settings.propertiesGridOpt.clearAll')}
                </button>
              </div>
            </div>
            <div className="space-y-1">
              {groupKeys.map((key) => (
                <div
                  key={key}
                  className="flex items-center gap-2 rounded-md px-2 py-1 transition-colors hover:bg-(--color-bg-elevated)"
                >
                  <label className="flex min-w-0 flex-1 cursor-pointer items-center gap-2">
                    <input
                      type="checkbox"
                      data-testid={`property-toggle-${key}`}
                      checked={shown.includes(key)}
                      onChange={() =>
                        onConfigChange({
                          visibleProperties: shown.includes(key)
                            ? shownKeys.filter((k) => k !== key)
                            : [...shownKeys, key],
                        })
                      }
                      className="h-4 w-4 shrink-0 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500"
                    />
                    <span className="truncate text-sm text-(--color-text-primary)">
                      {resolveTileLabel(
                        readPropertyOverride(panel.config, key),
                        getPropertyLabel(key, protocol, type),
                      )}
                    </span>
                  </label>
                  {/*
                    항목별 타일 디자인. 자리는 레이아웃 팝업이 갖지만 **모양은 여기서**
                    정한다 — 켜고 끄는 것과 같은 줄에 있어야 어느 항목의 설정인지 헷갈리지
                    않는다. 자리를 잡을 수 없는(꺼진) 항목에는 낼 이유가 없다.
                  */}
                  <div className={shown.includes(key) ? undefined : 'invisible'}>
                    <DesignPopover testId={`property-override-${key}`}>
                      <PropertyOverrideFields
                        propertyKey={key}
                        override={readPropertyOverride(panel.config, key)}
                        base={design}
                        fallbackLabel={getPropertyLabel(key, protocol, type)}
                        // 기본 정보는 사람이 적어 둔 값이라 붙일 단위가 없다.
                        withUnit={group !== 'basic'}
                        onChange={(next) =>
                          onConfigChange({
                            propertyOverrides: {
                              ...((panel.config?.propertyOverrides as Record<string, unknown>) ?? {}),
                              [key]: next,
                            },
                          })
                        }
                      />
                    </DesignPopover>
                  </div>
                </div>
              ))}
            </div>
          </div>
        );
      })}

    </>
  );
}

/** 로그 패널 전용 설정 (최대 줄 수) */
/** 열 개수·갱신 주기 설정이 의미 있는 모니터링 섹션 (로그/이벤트는 단일 뷰라 제외) */
const GRID_MONITOR_SECTIONS = new Set<MonitorSectionKey>(['stats', 'metrics', 'network']);

/** 표시 구간 설정이 있는 섹션 (통계는 시점 값이라 구간 개념이 없다) */
const WINDOWED_MONITOR_SECTIONS = new Set<MonitorSectionKey>(['metrics', 'network']);

/** 모니터링 패널 타입 → 항목 어휘의 섹션 키 */
const MONITOR_PANEL_SECTION: Record<string, MonitorSectionKey> = {
  'monitor-stats': 'stats',
  'monitor-metrics': 'metrics',
  'monitor-network': 'network',
  'monitor-logs': 'logs',
  'monitor-events': 'events',
};

/** sysmetrics 패널 타입 → 고를 대상 축. system 은 대상을 고르지 않는다(항상 종합). */
const SYSMETRICS_TARGET_KIND: Record<string, SysResourceKind | undefined> = {
  'sysmetrics-system': undefined,
  'sysmetrics-network': 'interfaces',
  'sysmetrics-storage': 'mountpoints',
};

/** sysmetrics 패널 타입 집합 (설정 섹션 표시 조건) */
const SYSMETRICS_PANEL_TYPES = new Set(Object.keys(SYSMETRICS_TARGET_KIND));

/**
 * 미리보기를 **실제 패널**로 그리는 타입.
 *
 * 차트 계열(stat/bar/pie/table/graph-chart/gauge/heatmap)은 이미 각자의 분기에서
 * 실패널을 그리며 미리보기 안 드래그 편집(범례 배치·값 자리)을 얹고 있으므로 여기
 * 목록에 넣지 않는다. 이 집합은 종전에 손으로 그린 목업을 쓰던 타입들이다.
 */
const REAL_PANEL_PREVIEW_TYPES = new Set<string>([
  'flows',
  'agents',
  'devices',
  'resource',
  'logs',
  'monitor-network',
  'monitor-stats',
  'monitor-metrics',
  'properties-grid',
  'device',
  'ac-control',
  'hvac-control',
  'outdoor-control',
  // 종전에는 미리보기 분기가 아예 없어 설정을 열면 빈 영역만 보이던 타입들이다.
  // renderDashboardPanel 이 모두 처리하므로 목록에 넣기만 하면 대시보드와 같은 화면이 된다.
  'agent-status',
  'monitor-logs',
  'monitor-events',
  'facility-device',
  'facility-station',
  'facility-line',
  'facility-group',
  'facility-schedule',
  'trigger-config',
  // text·custom-control 은 렌더러가 자리표시자를 그린다 — 대시보드에서 보이는 것과 같다.
  'text',
  'custom-control',
  ...SYSMETRICS_PANEL_TYPES,
]);

/** 패널 타입별 표시 항목 카탈로그 (켜고 끄는 항목). */
const SYSMETRICS_ITEM_CATALOG: Record<string, { key: string; labelKey: string }[]> = {
  // 시스템 패널의 항목은 **값 하나**다 — 그룹으로 묶으면 메모리 사용량만 크게 보는
  // 것이 불가능하고, 값마다 스타일·정렬·색을 정할 자리도 없다.
  'sysmetrics-system': SYSTEM_FIELDS.map((f) => ({ key: f.key, labelKey: f.labelKey })),
  'sysmetrics-storage': [
    { key: 'usage', labelKey: 'sysmetrics.storage.usage' },
    { key: 'used', labelKey: 'sysmetrics.storage.used' },
    { key: 'free', labelKey: 'sysmetrics.storage.free' },
    { key: 'total', labelKey: 'sysmetrics.storage.capacity' },
  ],
};

/**
 * 스타일을 고를 수 있는 대상 목록.
 *
 * 항목 켜기/끄기(`items`)와는 축이 다르다 — 스토리지는 마운트마다 스타일을 고르고,
 * 네트워크는 채널마다 고른다. 그래서 카탈로그를 따로 둔다. 마운트 목록은 설정된
 * 대상에서 나오므로 런타임에 만든다.
 */
const SYSMETRICS_STYLE_TARGETS: Record<
  string,
  { key: string; labelKey: string; kind: SysMetricValueKind; counterMode: boolean }[]
> = {
  // `counterMode` 는 "증가량/누적값을 고를 수 있는가" 다. 누적 카운터인 값에만 뜻이
  // 있으므로 카탈로그의 `rate` 를 그대로 따른다 — 상태값(비율·용량)에 이 선택을 내면
  // 골라도 아무 일도 일어나지 않는다.
  'sysmetrics-system': SYSTEM_FIELDS.map((f) => ({
    key: f.key,
    labelKey: f.labelKey,
    kind: f.kind,
    counterMode: f.rate,
  })),
  // 네트워크 전용 패널은 채널 넷을 한 축에 겹쳐 그리므로 채널마다 자릿수가 다른 값을
  // 섞을 수 없다 — 이 패널은 증가량으로 고정이다.
  'sysmetrics-network': [
    { key: 'bytes_recv', labelKey: 'sysmetrics.channels.rxBytes', kind: 'counter', counterMode: false },
    { key: 'bytes_sent', labelKey: 'sysmetrics.channels.txBytes', kind: 'counter', counterMode: false },
    { key: 'packets_recv', labelKey: 'sysmetrics.channels.rxPackets', kind: 'counter', counterMode: false },
    { key: 'packets_sent', labelKey: 'sysmetrics.channels.txPackets', kind: 'counter', counterMode: false },
  ],
};

/** 범례 위치 선택지 */
const LEGEND_CHOICES = ['none', 'top', 'bottom', 'right'] as const;

/** 게이지 모양 선택지 — 게이지 패널과 같은 목록을 쓴다. */
const GAUGE_TYPE_CHOICES = GAUGE_TYPES;

/** 표시 구간 선택지(초) */
const WINDOW_CHOICES = [60, 300, 600, 1_800, 3_600] as const;

/**
 * sysmetrics 패널 전용 설정 — 에이전트 · 대상 · 표시 항목 · 갱신 주기.
 *
 * 대상 선택이 이 패널군의 핵심이다. **비워 두면 종합**이고 고르면 개별이므로,
 * "종합"과 "개별"을 별도 패널 유형으로 나누지 않아도 된다. 그래서 빈 선택을
 * 오류로 다루지 않고 안내 문구로 설명한다.
 */
function SysMetricsSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const { data: agentsResult } = useAgents();
  const agents = useMemo(
    () => (agentsResult?.data ?? []).filter((a) => a.type === 'sysmetrics'),
    [agentsResult],
  );

  const config = panel.config as Record<string, unknown> | undefined;
  const agentId = typeof config?.agent_id === 'string' ? config.agent_id : '';
  const targetKind = SYSMETRICS_TARGET_KIND[panel.type];
  const catalog = SYSMETRICS_ITEM_CATALOG[panel.type] ?? [];
  // 시스템 패널은 옛 그룹 키를 값 키로 옮겨 읽는다(저장된 대시보드 호환).
  const selectedItems =
    panel.type === 'sysmetrics-system'
      ? (normalizeSystemItems(config?.items) ?? DEFAULT_SYSTEM_FIELDS)
      : Array.isArray(config?.items)
        ? (config.items as unknown[]).filter((v): v is string => typeof v === 'string')
        : catalog.map((i) => i.key);
  const refreshMs = readRefreshMs(config, 5_000);
  const duration = splitDuration(refreshMs);

  // 패널 기본 표시 옵션 — 항목이 따로 고르지 않으면 이 값을 따른다.
  const panelOptions = readPanelOptions(config, panel.type);
  const colOptions = maxColsOptions(Math.max(selectedItems.length, 1));
  const maxCols = readMaxCols(config, 2, selectedItems.length);

  // 스타일을 고를 수 있는 대상. 스토리지는 설정된 마운트가 곧 대상이라 런타임에 만든다.
  const styleTargets =
    SYSMETRICS_STYLE_TARGETS[panel.type] ??
    (panel.type === 'sysmetrics-storage'
      ? (Array.isArray(config?.mountpoints) ? (config.mountpoints as string[]) : []).map((m) => ({
          key: m,
          labelKey: m,
          kind: 'ratio' as SysMetricValueKind,
          // 스토리지는 그 시점 용량이라 환산할 것이 없다.
          counterMode: false,
        }))
      : []);
  const overrides = readAllOverrides(config);

  /** 항목별 덮어쓰기를 갱신한다. 값이 undefined 면 "패널을 따름"으로 되돌린다. */
  const patchItem = (key: string, patch: Record<string, unknown>) => {
    onConfigChange({ itemOptions: withItemOverride(config, key, patch) });
  };

  const toggleItem = (key: string) => {
    onConfigChange({
      items: selectedItems.includes(key)
        ? selectedItems.filter((k) => k !== key)
        : // 카탈로그 순서를 유지해야 타일 배치가 체크 순서에 따라 흔들리지 않는다.
          catalog.filter((i) => i.key === key || selectedItems.includes(i.key)).map((i) => i.key),
    });
  };

  return (
    <div className="space-y-4">
      {/* 에이전트: 이름이 아니라 ID 를 정본으로 저장한다(리네임에도 연결이 유지된다). */}
      <div>
        <label
          htmlFor="sysmetrics-agent-select"
          className="mb-1.5 block text-xs font-medium text-(--color-text-muted)"
        >
          {t('dashboard.settings.agent')}
        </label>
        <select
          id="sysmetrics-agent-select"
          data-testid="sysmetrics-settings-agent"
          value={agentId}
          onChange={(e) => {
            const next = agents.find((a) => a.id === e.target.value);
            onConfigChange({ agent_id: e.target.value, agent_name: next?.name ?? '' });
          }}
          className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary)"
        >
          <option value="">{t('dashboard.settings.selectAgent')}</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </select>
      </div>

      {/* 대상: 비우면 종합. 목록은 호스트가 실제로 가진 것에서 고른다. */}
      {targetKind && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('sysmetrics.settings.targets')}
          </label>
          <p className="mb-1.5 text-xs text-(--color-text-muted)">
            {t('sysmetrics.settings.targetsHint')}
          </p>
          <SysResourceSelector
            kind={targetKind}
            value={config?.[targetKind]}
            onChange={(next) => onConfigChange({ [targetKind]: next })}
          />
        </div>
      )}

      {/* 표시 항목 (네트워크는 채널이 고정이라 없다) */}
      {catalog.length > 0 && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.settings.monitorItems')}
          </label>
          <div className="flex flex-wrap gap-2">
            {catalog.map((item) => (
              <button
                key={item.key}
                type="button"
                data-testid={`sysmetrics-item-${item.key}`}
                data-selected={selectedItems.includes(item.key) ? 'true' : 'false'}
                onClick={() => toggleItem(item.key)}
                className={cn(
                  'rounded-md border px-2 py-1 text-xs transition-colors',
                  selectedItems.includes(item.key)
                    ? 'border-blue-500 bg-blue-500/10 text-blue-600 dark:text-blue-300'
                    : 'border-(--color-border-default) text-(--color-text-secondary)',
                )}
              >
                {t(item.labelKey)}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* 갱신 주기 */}
      <div>
        <label
          htmlFor="sysmetrics-refresh"
          className="mb-1.5 block text-xs font-medium text-(--color-text-muted)"
        >
          {t('dashboard.settings.refreshInterval')}
        </label>
        <input
          id="sysmetrics-refresh"
          data-testid="sysmetrics-settings-refresh"
          type="number"
          min={1}
          value={duration.seconds + duration.minutes * 60 + duration.hours * 3_600}
          onChange={(e) => {
            const seconds = Number(e.target.value);
            onConfigChange({ refreshMs: joinDuration(0, 0, Number.isFinite(seconds) ? seconds : 5) });
          }}
          className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary)"
        />
      </div>

      {/* 열 개수 상한 — 실제 열 수는 패널 폭에 따라 이보다 줄어든다. */}
      <div>
        <label
          htmlFor="sysmetrics-max-cols"
          className="mb-1.5 block text-xs font-medium text-(--color-text-muted)"
        >
          {t('dashboard.settings.maxColumns')}
        </label>
        <select
          id="sysmetrics-max-cols"
          data-testid="sysmetrics-settings-maxcols"
          value={maxCols}
          onChange={(e) => onConfigChange({ maxCols: Number(e.target.value) })}
          className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary)"
        >
          {colOptions.map((n) => (
            <option key={n} value={n}>
              {n}
            </option>
          ))}
        </select>
      </div>

      {/* 패널 기본 표시 옵션 — 항목이 덮어쓰지 않으면 이 값을 따른다. */}
      <div className="rounded-md border border-(--color-border-default) p-3">
        <p className="mb-2 text-xs font-medium text-(--color-text-muted)">
          {t('sysmetrics.settings.panelDefaults')}
        </p>
        <SysMetricsOptionFields
          testIdPrefix="sysmetrics-panel"
          styles={stylesForPanel(panel.type)}
          value={panelOptions}
          effectiveStyle={panelOptions.style}
          // 패널 기본값은 누적 카운터를 담는 패널에서만 뜻이 있다.
          showCounterMode={styleTargets.some((target) => target.counterMode)}
          onChange={(patch) => onConfigChange(patch)}
        />
      </div>

      {/* 항목별 덮어쓰기 */}
      {styleTargets.length > 0 && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('sysmetrics.settings.perItem')}
          </label>
          <p className="mb-2 text-xs text-(--color-text-muted)">
            {t('sysmetrics.settings.perItemHint')}
          </p>
          <div className="space-y-2">
            {styleTargets.map((target) => {
              const override = overrides[target.key] ?? {};
              const overridden = hasOverride(config, target.key);
              return (
                <details
                  key={target.key}
                  data-testid={`sysmetrics-item-options-${target.key}`}
                  data-overridden={overridden ? 'true' : 'false'}
                  className="rounded-md border border-(--color-border-default) p-2"
                >
                  <summary className="cursor-pointer text-xs text-(--color-text-secondary)">
                    {target.labelKey.startsWith('sysmetrics.') ? t(target.labelKey) : target.labelKey}
                    {overridden && (
                      <span className="ml-2 rounded bg-blue-500/10 px-1 text-[10px] text-blue-600 dark:text-blue-300">
                        {t('sysmetrics.settings.overridden')}
                      </span>
                    )}
                  </summary>
                  <div className="mt-2">
                    <SysMetricsOptionFields
                      testIdPrefix={`sysmetrics-item-${target.key}`}
                      styles={stylesFor(target.kind)}
                      value={{
                        style: override.style as SysMetricsStyle | undefined,
                        counterMode: override.counterMode as string | undefined,
                        height: override.height as number | undefined,
                        legend: override.legend as string | undefined,
                        windowSec: override.windowSec as number | undefined,
                        smooth: override.smooth as boolean | undefined,
                        stacked: override.stacked as boolean | undefined,
                        gaugeType: override.gaugeType as string | undefined,
                        min: override.min as number | undefined,
                        max: override.max as number | undefined,
                        align: override.align as string | undefined,
                        valueSize: override.valueSize as string | undefined,
                        valueColor: override.valueColor as string | undefined,
                        labelSize: override.labelSize as string | undefined,
                        labelWeight: override.labelWeight as string | undefined,
                      }}
                      placeholder={panelOptions}
                      // 항목이 실제로 그려질 스타일 — 덮어쓰지 않았으면 패널을 따른다.
                      effectiveStyle={readStyle(override.style, panelOptions.style)}
                      showCounterMode={target.counterMode}
                      onChange={(patch) => patchItem(target.key, patch)}
                    />
                  </div>
                </details>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

/**
 * 표시 옵션 입력 묶음 — 패널 기본값과 항목별 덮어쓰기가 같은 폼을 쓴다.
 *
 * `placeholder` 가 있으면 항목별 모드다: 빈 값이 "패널을 따름"을 뜻하고, 선택지에
 * 그 뜻의 항목을 하나 더 둔다. 되돌릴 방법이 없으면 한 번 고른 값에 갇힌다.
 */
function SysMetricsOptionFields({
  testIdPrefix,
  styles,
  value,
  placeholder,
  effectiveStyle,
  showCounterMode = false,
  onChange,
}: {
  testIdPrefix: string;
  styles: SysMetricsStyle[];
  value: {
    style?: SysMetricsStyle;
    counterMode?: string;
    height?: number;
    legend?: string;
    windowSec?: number;
    smooth?: boolean;
    stacked?: boolean;
    gaugeType?: string;
    min?: number;
    max?: number;
    align?: string;
    valueSize?: string;
    valueColor?: string;
    labelSize?: string;
    labelWeight?: string;
  };
  placeholder?: {
    style: SysMetricsStyle;
    counterMode: string;
    height?: number;
    legend: string;
    windowSec: number;
    smooth: boolean;
    stacked: boolean;
    gaugeType: string;
    min: number;
    max: number;
    align: string;
    valueSize: string;
    valueColor?: string;
    labelSize: string;
    labelWeight: string;
  };
  /** 실제로 그려질 스타일. 이 스타일에서 뜻이 없는 입력 칸은 감춘다. */
  effectiveStyle: SysMetricsStyle;
  /** 누적 카운터의 표시 방식(증가량/누적값) 선택을 낼 것인가. */
  showCounterMode?: boolean;
  onChange: (patch: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const inherit = placeholder !== undefined;
  // 타일은 차트가 아니라 높이·범례·구간이 아무것도 하지 않는다. 그런데도 칸을 보여
  // 주면 바꿔 놓고 왜 안 변하는지 찾아 헤매게 된다.
  const fields = optionFieldsFor(effectiveStyle);
  const cls =
    'w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1 text-xs text-(--color-text-primary)';

  return (
    <div className="grid grid-cols-2 gap-2">
      <div>
        <label className="mb-1 block text-[11px] text-(--color-text-muted)">
          {t('sysmetrics.settings.style')}
        </label>
        <select
          data-testid={`${testIdPrefix}-style`}
          value={value.style ?? ''}
          onChange={(e) => onChange({ style: e.target.value || undefined })}
          className={cls}
        >
          {inherit && <option value="">{t('sysmetrics.settings.inherit')}</option>}
          {styles.map((st) => (
            <option key={st} value={st}>
              {t(`sysmetrics.styles.${st}`)}
            </option>
          ))}
        </select>
      </div>

      {showCounterMode && (
      <div>
        <label className="mb-1 block text-[11px] text-(--color-text-muted)">
          {t('sysmetrics.settings.counterMode')}
        </label>
        <select
          data-testid={`${testIdPrefix}-counter-mode`}
          value={value.counterMode ?? ''}
          onChange={(e) => onChange({ counterMode: e.target.value || undefined })}
          className={cls}
        >
          {inherit && <option value="">{t('sysmetrics.settings.inherit')}</option>}
          {SYSMETRICS_COUNTER_MODES.map((mode) => (
            <option key={mode} value={mode}>
              {t(`sysmetrics.counterModes.${mode}`)}
            </option>
          ))}
        </select>
      </div>
      )}

      {fields.height && (
      <div>
        <label className="mb-1 block text-[11px] text-(--color-text-muted)">
          {t('sysmetrics.settings.height')}
        </label>
        <input
          data-testid={`${testIdPrefix}-height`}
          type="number"
          min={MIN_ITEM_HEIGHT}
          max={MAX_ITEM_HEIGHT}
          value={value.height ?? ''}
          // 비우면 칸을 채운다. 값이 있으면 그 높이에 고정된다.
          placeholder={placeholder?.height ? String(placeholder.height) : t('sysmetrics.settings.heightFill')}
          onChange={(e) => {
            const n = Number(e.target.value);
            onChange({ height: e.target.value === '' || !Number.isFinite(n) ? undefined : n });
          }}
          className={cls}
        />
      </div>
      )}

      {fields.legend && (
      <div>
        <label className="mb-1 block text-[11px] text-(--color-text-muted)">
          {t('sysmetrics.settings.legend')}
        </label>
        <select
          data-testid={`${testIdPrefix}-legend`}
          value={value.legend ?? ''}
          onChange={(e) => onChange({ legend: e.target.value || undefined })}
          className={cls}
        >
          {inherit && <option value="">{t('sysmetrics.settings.inherit')}</option>}
          {LEGEND_CHOICES.map((pos) => (
            <option key={pos} value={pos}>
              {t(`sysmetrics.legend.${pos}`)}
            </option>
          ))}
        </select>
      </div>
      )}

      {fields.window && (
      <div>
        <label className="mb-1 block text-[11px] text-(--color-text-muted)">
          {t('sysmetrics.settings.window')}
        </label>
        <select
          data-testid={`${testIdPrefix}-window`}
          value={value.windowSec ?? ''}
          onChange={(e) => onChange({ windowSec: e.target.value ? Number(e.target.value) : undefined })}
          className={cls}
        >
          {inherit && <option value="">{t('sysmetrics.settings.inherit')}</option>}
          {WINDOW_CHOICES.map((sec) => (
            <option key={sec} value={sec}>
              {sec >= 3_600 ? `${sec / 3_600}h` : sec >= 60 ? `${sec / 60}m` : `${sec}s`}
            </option>
          ))}
        </select>
      </div>
      )}

      {/* 곡선·누적 — 차트 패널과 같은 이름·같은 노출 규칙 */}
      {fields.smooth && (
        <label className="col-span-2 flex cursor-pointer items-center gap-1.5 text-[11px] text-(--color-text-muted)">
          <input
            type="checkbox"
            data-testid={`${testIdPrefix}-smooth`}
            checked={value.smooth ?? placeholder?.smooth ?? false}
            onChange={(e) => onChange({ smooth: e.target.checked || undefined })}
          />
          <span>{t('dashboard.chart.curve')}</span>
        </label>
      )}
      {fields.stacked && (
        <label className="col-span-2 flex cursor-pointer items-center gap-1.5 text-[11px] text-(--color-text-muted)">
          <input
            type="checkbox"
            data-testid={`${testIdPrefix}-stacked`}
            checked={value.stacked ?? placeholder?.stacked ?? false}
            onChange={(e) => onChange({ stacked: e.target.checked || undefined })}
          />
          <span>{t('dashboard.chart.stacked')}</span>
        </label>
      )}

      {/* 게이지 모양·눈금 — 게이지 패널과 같은 키(gaugeType/min/max/unit) */}
      {fields.gauge && (
        <>
          <div className="col-span-2">
            <label className="mb-1 block text-[11px] text-(--color-text-muted)">
              {t('dashboard.settings.gaugeType')}
            </label>
            <select
              data-testid={`${testIdPrefix}-gauge-type`}
              value={value.gaugeType ?? ''}
              onChange={(e) => onChange({ gaugeType: e.target.value || undefined })}
              className={cls}
            >
              {inherit && <option value="">{t('sysmetrics.settings.inherit')}</option>}
              {GAUGE_TYPE_CHOICES.map((g) => (
                <option key={g} value={g}>
                  {t(`dashboard.settings.gaugeTypes.${g}`)}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="mb-1 block text-[11px] text-(--color-text-muted)">
              {t('dashboard.settings.min')}
            </label>
            <input
              data-testid={`${testIdPrefix}-min`}
              type="number"
              value={value.min ?? ''}
              placeholder={placeholder ? String(placeholder.min) : undefined}
              onChange={(e) => {
                const n = Number(e.target.value);
                onChange({ min: e.target.value === '' || !Number.isFinite(n) ? undefined : n });
              }}
              className={cls}
            />
          </div>
          <div>
            <label className="mb-1 block text-[11px] text-(--color-text-muted)">
              {t('dashboard.settings.max')}
            </label>
            <input
              data-testid={`${testIdPrefix}-max`}
              type="number"
              value={value.max ?? ''}
              placeholder={placeholder ? String(placeholder.max) : undefined}
              onChange={(e) => {
                const n = Number(e.target.value);
                onChange({ max: e.target.value === '' || !Number.isFinite(n) ? undefined : n });
              }}
              className={cls}
            />
          </div>
        </>
      )}

      {/* 타일 표시 — 정렬·글자·색. 타일·진행 막대에서만 뜻이 있다. */}
      {fields.tile && (
        <>
          <div>
            <label className="mb-1 block text-[11px] text-(--color-text-muted)">
              {t('sysmetrics.settings.align')}
            </label>
            <select
              data-testid={`${testIdPrefix}-align`}
              value={value.align ?? ''}
              onChange={(e) => onChange({ align: e.target.value || undefined })}
              className={cls}
            >
              {inherit && <option value="">{t('sysmetrics.settings.inherit')}</option>}
              {TILE_ALIGNS.map((a) => (
                <option key={a} value={a}>
                  {t(`sysmetrics.align.${a}`)}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="mb-1 block text-[11px] text-(--color-text-muted)">
              {t('sysmetrics.settings.valueSize')}
            </label>
            <select
              data-testid={`${testIdPrefix}-value-size`}
              value={value.valueSize ?? ''}
              onChange={(e) => onChange({ valueSize: e.target.value || undefined })}
              className={cls}
            >
              {inherit && <option value="">{t('sysmetrics.settings.inherit')}</option>}
              {TILE_TEXT_SIZES.map((sz) => (
                <option key={sz} value={sz}>
                  {t(`sysmetrics.textSize.${sz}`)}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="mb-1 block text-[11px] text-(--color-text-muted)">
              {t('sysmetrics.settings.valueColor')}
            </label>
            <div className="flex items-center gap-1">
              <input
                type="color"
                data-testid={`${testIdPrefix}-value-color`}
                value={value.valueColor ?? placeholder?.valueColor ?? '#3b82f6'}
                onChange={(e) => onChange({ valueColor: e.target.value })}
                className="h-7 w-9 cursor-pointer rounded border border-(--color-border-default)"
              />
              {/* 색을 지운다 = 기본 글자색으로 되돌린다. 되돌릴 방법이 없으면 갇힌다. */}
              <button
                type="button"
                data-testid={`${testIdPrefix}-value-color-clear`}
                onClick={() => onChange({ valueColor: undefined })}
                className="rounded border border-(--color-border-default) px-1.5 py-0.5 text-[10px] text-(--color-text-secondary)"
              >
                {t('sysmetrics.settings.clearColor')}
              </button>
            </div>
          </div>

          <div>
            <label className="mb-1 block text-[11px] text-(--color-text-muted)">
              {t('sysmetrics.settings.labelSize')}
            </label>
            <select
              data-testid={`${testIdPrefix}-label-size`}
              value={value.labelSize ?? ''}
              onChange={(e) => onChange({ labelSize: e.target.value || undefined })}
              className={cls}
            >
              {inherit && <option value="">{t('sysmetrics.settings.inherit')}</option>}
              {TILE_TEXT_SIZES.map((sz) => (
                <option key={sz} value={sz}>
                  {t(`sysmetrics.textSize.${sz}`)}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="mb-1 block text-[11px] text-(--color-text-muted)">
              {t('sysmetrics.settings.labelWeight')}
            </label>
            <select
              data-testid={`${testIdPrefix}-label-weight`}
              value={value.labelWeight ?? ''}
              onChange={(e) => onChange({ labelWeight: e.target.value || undefined })}
              className={cls}
            >
              {inherit && <option value="">{t('sysmetrics.settings.inherit')}</option>}
              {TILE_TEXT_WEIGHTS.map((w) => (
                <option key={w} value={w}>
                  {t(`sysmetrics.textWeight.${w}`)}
                </option>
              ))}
            </select>
          </div>
        </>
      )}
    </div>
  );
}

/**
 * 모니터링 패널 전용 설정 — 표시 항목 다중 선택.
 *
 * 항목 어휘와 검증은 모니터링 페이지와 공유한다(monitoringCatalog / monitoringLayout).
 * 항목을 모두 끄는 것도 정상 상태로 허용하며, 그때 패널은 빈 상태 안내를 보여준다.
 */
function MonitorItemsSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  // 모니터링 패널이 아닌 타입으로 들어오면 그릴 것이 없다(호출부 분기가 이미 막지만
  // 타입 수준에서도 좁혀 둔다).
  const section: MonitorSectionKey | undefined = MONITOR_PANEL_SECTION[panel.type];
  const catalog = section ? SECTION_CATALOG[section] : [];
  const selected = section ? readPanelItems(section, panel.config) : [];
  // 기본값은 패널 컴포넌트와 같은 값을 써야 설정 화면과 실제 렌더가 어긋나지 않는다.
  const colOptions = maxColsOptions(selected.length);
  const maxCols = readMaxCols(panel.config, section === 'metrics' ? 2 : 3, selected.length);
  const refreshMs = readRefreshMs(panel.config, section === 'metrics' ? 1_000 : 5_000);
  const duration = splitDuration(refreshMs);
  const windowSec = readWindowSec(panel.config, 300);
  const chosenIfaces = readInterfaces(panel.config);
  // 인터페이스 목록은 거의 변하지 않는다. 설정 화면에서 잦은 폴링은 낭비라 크게 벌린다.
  const { data: netStats } = useNetworkStats(60_000);
  const netInterfaces = ['total', ...(netStats?.interfaces.map((i) => i.name) ?? [])];

  if (!section) return null;

  const toggle = (key: string) => {
    onConfigChange({
      items: selected.includes(key)
        ? selected.filter((k) => k !== key)
        : // 카탈로그 순서를 유지해야 패널 배치가 체크 순서에 따라 흔들리지 않는다.
          catalog.filter((item) => item.key === key || selected.includes(item.key)).map((i) => i.key),
    });
  };

  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
        {t('dashboard.settings.monitorItems')}
      </label>
      <div className="space-y-1">
        {catalog.map((item) => (
          <label
            key={item.key}
            className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-(--color-bg-elevated)"
          >
            <input
              type="checkbox"
              data-testid={`monitor-item-toggle-${item.key}`}
              checked={selected.includes(item.key)}
              onChange={() => toggle(item.key)}
              className="h-4 w-4 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500"
            />
            <span className="text-sm text-(--color-text-primary)">{t(item.labelKey)}</span>
          </label>
        ))}
      </div>

      {/* 열 개수 상한 — 통계/메트릭처럼 격자로 놓는 패널에만 의미가 있다. */}
      {GRID_MONITOR_SECTIONS.has(section) && (
        <div className="mt-3">
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.settings.maxColumns')}
          </label>
          <div className="flex flex-wrap gap-1">
            {colOptions.map((n) => (
              <button
                key={n}
                type="button"
                data-testid={`monitor-maxcols-${n}`}
                onClick={() => onConfigChange({ maxCols: n })}
                className={cn(
                  'flex-1 rounded-md border px-2 py-1 text-xs font-medium transition-colors',
                  maxCols === n
                    ? 'border-blue-500 bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
                    : 'border-(--color-border-default) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
                )}
              >
                {n}
              </button>
            ))}
          </div>
          <p className="mt-1 text-[10px] leading-relaxed text-(--color-text-muted)">
            {t('dashboard.settings.maxColumnsHint')}
          </p>
        </div>
      )}

      {/* 갱신 주기 — 시/분/초로 직접 정한다. */}
      {GRID_MONITOR_SECTIONS.has(section) && (
        <div className="mt-3">
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.settings.refreshInterval')}
          </label>
          <div className="flex items-center gap-2">
            {(
              [
                ['hours', duration.hours, 23, 'dashboard.settings.unitHour'],
                ['minutes', duration.minutes, 59, 'dashboard.settings.unitMinute'],
                ['seconds', duration.seconds, 59, 'dashboard.settings.unitSecond'],
              ] as const
            ).map(([field, value, max, unitKey]) => (
              <div key={field} className="flex flex-1 items-center gap-1">
                <input
                  type="number"
                  min={0}
                  max={max}
                  value={value}
                  data-testid={`monitor-refresh-${field}`}
                  onChange={(e) => {
                    const n = Number(e.target.value);
                    const next = { ...duration, [field]: Number.isFinite(n) ? n : 0 };
                    onConfigChange({
                      refreshMs: joinDuration(next.hours, next.minutes, next.seconds),
                    });
                  }}
                  className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-400 focus:outline-none"
                />
                <span className="shrink-0 text-[10px] text-(--color-text-muted)">{t(unitKey)}</span>
              </div>
            ))}
          </div>
          <p className="mt-1 text-[10px] leading-relaxed text-(--color-text-muted)">
            {t(
              section === 'stats'
                ? 'dashboard.settings.refreshIntervalStatsHint'
                : section === 'network'
                  ? 'dashboard.settings.refreshIntervalNetworkHint'
                  : 'dashboard.settings.refreshIntervalMetricsHint',
            )}
          </p>
        </div>
      )}

      {/* 표시 구간 */}
      {WINDOWED_MONITOR_SECTIONS.has(section) && (
        <div className="mt-3">
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.settings.displayWindow')}
          </label>
          <div className="flex flex-wrap gap-1">
            {WINDOW_SEC_OPTIONS.map((sec) => (
              <button
                key={sec}
                type="button"
                data-testid={`monitor-window-${sec}`}
                onClick={() => onConfigChange({ windowSec: sec })}
                className={cn(
                  'flex-1 rounded-md border px-2 py-1 text-xs font-medium transition-colors',
                  windowSec === sec
                    ? 'border-blue-500 bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
                    : 'border-(--color-border-default) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
                )}
              >
                {sec >= 60 ? `${sec / 60}${t('dashboard.settings.unitMinute')}` : `${sec}${t('dashboard.settings.unitSecond')}`}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* 단위시간 — 네트워크 rate 계열 전용 */}
      {section === 'network' && (
        <div className="mt-3">
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.settings.unitTime')}
          </label>
          <div className="flex gap-1">
            {(['sec', 'min', 'hour'] as const).map((u) => (
              <button
                key={u}
                type="button"
                data-testid={`monitor-unittime-${u}`}
                onClick={() => onConfigChange({ unitTime: u })}
                className={cn(
                  'flex-1 rounded-md border px-2 py-1 text-xs font-medium transition-colors',
                  (panel.config?.unitTime ?? 'sec') === u
                    ? 'border-blue-500 bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
                    : 'border-(--color-border-default) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
                )}
              >
                {t(`dashboard.settings.unitTime_${u}`)}
              </button>
            ))}
          </div>
          <p className="mt-1 text-[10px] leading-relaxed text-(--color-text-muted)">
            {t('dashboard.settings.unitTimeHint')}
          </p>
        </div>
      )}

      {/* 인터페이스 다중 선택 — 고른 만큼 차트에 선이 겹친다. */}
      {section === 'network' && (
        <div className="mt-3">
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.settings.netInterface')}
          </label>
          <div className="max-h-40 space-y-1 overflow-y-auto rounded-md border border-(--color-border-default) p-1">
            {netInterfaces.map((name) => (
              <label
                key={name}
                className="flex cursor-pointer items-center gap-2 rounded px-2 py-1 transition-colors hover:bg-(--color-bg-elevated)"
              >
                <input
                  type="checkbox"
                  data-testid={`monitor-iface-${name}`}
                  checked={chosenIfaces.includes(name)}
                  onChange={() =>
                    onConfigChange({
                      interfaces: chosenIfaces.includes(name)
                        ? chosenIfaces.filter((n) => n !== name)
                        : [...chosenIfaces, name],
                    })
                  }
                  className="h-4 w-4 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500"
                />
                <span className="text-sm text-(--color-text-primary)">
                  {name === 'total' ? t('dashboard.settings.netInterfaceAll') : name}
                </span>
              </label>
            ))}
          </div>
          <p className="mt-1 text-[10px] leading-relaxed text-(--color-text-muted)">
            {t('dashboard.settings.netInterfaceHint')}
          </p>
        </div>
      )}

    </div>
  );
}

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
  temperature: 'dashboard.settings.accent.temperature',
  controls: 'dashboard.settings.accent.controls',
  labels: 'dashboard.settings.accent.labels',
  borders: 'dashboard.settings.accent.borders',
  indicators: 'dashboard.settings.accent.indicators',
};

/** 리스트 패널 (flows/agents/devices) 악센트 그룹 — 값은 i18n 키 */
/**
 * 글자 모양 패치를 병합한다. 전부 비면 필드를 **지운다** — 빈 객체가 남으면
 * "설정했다"로 읽혀, 되돌렸는데도 설정된 것처럼 보인다(타이틀 디자인과 같은 규칙).
 */
function mergeFont(
  current: PanelTitleFont,
  patch: Partial<PanelTitleFont>,
): PanelTitleFont | undefined {
  const next = { ...current, ...patch };
  return Object.values(next).every((v) => v === undefined) ? undefined : next;
}

const LIST_ACCENT_LABEL_KEYS: Record<string, string> = {
  header: 'dashboard.settings.accent.header',
  badges: 'dashboard.settings.accent.badges',
  table: 'dashboard.settings.accent.table',
};

/** 리소스 패널 악센트 그룹 — 값은 i18n 키 */
const RESOURCE_ACCENT_LABEL_KEYS: Record<string, string> = {
  header: 'dashboard.settings.accent.header',
  cpu: 'dashboard.settings.accent.cpuCard',
  memory: 'dashboard.settings.accent.memoryCard',
  throughput: 'dashboard.settings.accent.throughputCard',
  errorRate: 'dashboard.settings.accent.errorRateCard',
};

/** 시스템 통계 패널 악센트 그룹 — 값은 i18n 키 */
const MONITOR_STATS_ACCENT_LABEL_KEYS: Record<string, string> = {
  header: 'dashboard.settings.accent.header',
  label: 'dashboard.settings.accent.statLabel',
  value: 'dashboard.settings.accent.statValue',
};

/** 로그 패널 악센트 그룹 — 값은 i18n 키 */
const LOG_ACCENT_LABEL_KEYS: Record<string, string> = {
  header: 'dashboard.settings.accent.header',
  levels: 'dashboard.settings.accent.levels',
  timestamp: 'dashboard.settings.accent.timestamp',
  source: 'dashboard.settings.accent.source',
};

/** 게이지 패널 악센트 그룹 — 값은 i18n 키 */
const GAUGE_ACCENT_LABEL_KEYS: Record<string, string> = {
  header: 'dashboard.settings.accent.header',
  arc: 'dashboard.settings.accent.arc',
  value: 'dashboard.settings.accent.value',
};

/** 게이지 단위 옵션 — labelKey/unitLabelKey 는 i18n 키. 키가 없으면 value 를 그대로 표시. */
/** 니들(바늘)이 있는 유형 — 니들 색 설정을 노출하는 자리다. */
const NEEDLE_GAUGE_TYPES: readonly GaugeType[] = ['needle', 'needle-rainbow', 'half-rainbow'];

const GAUGE_TYPE_META: { type: GaugeType; labelKey: string; icon: string }[] = [
  { type: 'simple', labelKey: 'dashboard.settings.gaugeTypes.simple', icon: 'O' },
  { type: 'half', labelKey: 'dashboard.settings.gaugeTypes.half', icon: 'U' },
  { type: 'needle', labelKey: 'dashboard.settings.gaugeTypes.needle', icon: '>' },
  { type: 'needle-rainbow', labelKey: 'dashboard.settings.gaugeTypes.needleRainbow', icon: '>>' },
  { type: 'vertical-bar', labelKey: 'dashboard.settings.gaugeTypes.verticalBar', icon: '|' },
  { type: 'half-rainbow', labelKey: 'dashboard.settings.gaugeTypes.halfRainbow', icon: 'U+' },
];

/** 연속 컬러 테마 프리셋 — labelKey 는 i18n 키 */
// 색 목록은 게이지 렌더와 **같은 정본**(GAUGE_COLOR_THEMES)에서 온다. 종전에는 여기
// 하드코딩이라, 미리보기 점 세 개는 테마 색으로 바뀌는데 게이지는 그대로였다.
const COLOR_THEME_PRESETS = [
  { id: 'green-red', labelKey: 'dashboard.settings.colorThemes.greenRed' },
  { id: 'blue-purple', labelKey: 'dashboard.settings.colorThemes.bluePurple' },
  { id: 'cyan-blue', labelKey: 'dashboard.settings.colorThemes.cyanBlue' },
].map((t) => ({ ...t, colors: GAUGE_COLOR_THEMES[t.id] ?? [] }));

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
            className="h-2.5 w-2.5 rounded-full border border-(--color-border-default)"
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
                c === '#ffffff' && color !== c && 'ring-1 ring-(--color-border-strong)',
              )}
              style={{ backgroundColor: c }}
              aria-label={`${label} ${c}`}
            >
              {color === c && <Check className={cn('absolute inset-0 m-auto h-3 w-3 drop-shadow', c === '#ffffff' ? 'text-(--color-text-secondary)' : 'text-white')} />}
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
/**
 * 악센트 그룹 선택기.
 *
 * 미리보기가 실제 패널이 되면서 "영역을 클릭해 고르기"가 불가능해졌다. 대신 그룹을
 * 목록으로 늘어놓고 고르게 한다. 각 항목은 현재 적용된 색을 점으로 보여주므로,
 * 어느 그룹에 색이 지정되어 있는지 목록만 보고 알 수 있다.
 */
/**
 * 패널 색상 한 줄 — 스와치 목록 + 초기화. @spec SPEC-CHART-003 §2.1 [U1-2 / U1-3]
 *
 * `config.panelColor` 만 쓰고 `config.accentElements` 는 읽지도 쓰지도 않는다 —
 * 편집 입구만 사라지고 저장된 값은 그대로 남아야 한다(U1-4). 다른 패널 타입으로
 * 바꿨을 때 종전 악센트 설정이 되살아나야 하기 때문이다.
 */
function PanelColorRow({
  panelColor,
  onChange,
}: {
  panelColor: string | undefined;
  onChange: (color: string | undefined) => void;
}) {
  const { t } = useTranslation();
  return (
    <div data-testid="panel-color-row">
      <span className="mb-2 block text-xs font-medium text-(--color-text-muted)">
        {t('dashboard.settings.accent.panelColor')}
      </span>
      <div className="flex flex-wrap items-center gap-1.5">
        {PANEL_COLORS.map((color) => (
          <button
            key={color}
            type="button"
            data-testid={`panel-color-${color}`}
            aria-label={color}
            aria-pressed={panelColor === color}
            onClick={() => onChange(color)}
            className={cn(
              'h-5 w-5 rounded-full border-2 transition-transform hover:scale-110',
              panelColor === color ? 'border-white ring-2 ring-blue-500' : 'border-transparent',
            )}
            style={{ backgroundColor: color }}
          />
        ))}
        {panelColor && (
          <button
            type="button"
            data-testid="panel-color-reset"
            onClick={() => onChange(undefined)}
            className="ml-1 rounded px-2 py-0.5 text-xs text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated)"
          >
            {t('dashboard.settings.panelColorReset')}
          </button>
        )}
      </div>
    </div>
  );
}

function AccentGroupPicker({
  labelKeys,
  selected,
  effectiveColor,
  onSelect,
}: {
  labelKeys: Record<string, string>;
  selected: string | null;
  effectiveColor: (group: string) => string | undefined;
  onSelect: (group: string | null) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="mb-3 flex flex-wrap gap-1.5" data-testid="accent-group-picker">
      {Object.entries(labelKeys).map(([group, labelKey]) => {
        const isSelected = group === selected;
        const color = effectiveColor(group);
        return (
          <button
            key={group}
            type="button"
            data-testid={`accent-group-${group}`}
            aria-pressed={isSelected}
            // 이미 고른 항목을 다시 누르면 선택을 푼다 — 색 편집을 접는 수단이다.
            onClick={() => onSelect(isSelected ? null : group)}
            className={cn(
              'flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs transition-colors',
              isSelected
                ? 'border-blue-500 bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-300'
                : 'border-(--color-border-default) text-(--color-text-secondary) hover:border-(--color-border-strong)',
            )}
          >
            <span
              aria-hidden="true"
              className="h-2.5 w-2.5 shrink-0 rounded-full border border-(--color-border-strong)"
              style={color ? { backgroundColor: color } : undefined}
            />
            {t(labelKey)}
          </button>
        );
      })}
    </div>
  );
}

function AccentGroupControls({
  selected,
  labelKeys,
  accentElements,
  inheritedColor,
  onChange,
}: {
  selected: string;
  labelKeys: Record<string, string>;
  accentElements: Record<string, string | boolean>;
  /**
   * 색을 지정하지 않은 그룹이 물려받는 패널 색상 — **표시용**이다.
   * 편집은 패널 옵션의 패널 색상이 소유한다(여기서 쓰면 두 자리가 한 값을 다툰다).
   */
  inheritedColor: string | undefined;
  onChange: (elements: Record<string, string | boolean>) => void;
}) {
  const { t } = useTranslation();
  // 선택된 그룹의 표시 라벨 (키 → 번역)
  const selectedLabel = labelKeys[selected] ? t(labelKeys[selected]!) : selected;
  // 모든 그룹이 accentElements 를 쓴다 — panelColor 를 직접 쓰던 `_base` 예외는
  // 패널 옵션의 패널 색상으로 이관되면서 사라졌다.
  const isEnabled = accentElements[selected] !== false;
  const gc = typeof accentElements[selected] === 'string' ? (accentElements[selected] as string) : undefined;

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
    onChange({ ...accentElements, [selected]: isEnabled ? false : true });
  };
  const setColor = (color: string | undefined) => {
    onChange({ ...accentElements, [selected]: color ?? true });
  };

  const hasSubProps = selected === 'labels';

  return (
    <div className="mt-3 rounded-lg border border-(--color-border-default) bg-(--color-bg-elevated) p-3">
      <div className="mb-2 flex items-center gap-2">
        <input type="checkbox" checked={isEnabled} onChange={toggle} className="h-4 w-4 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500" />
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
              <input type="color" value={gc ?? inheritedColor ?? '#3b82f6'} onChange={(e) => setColor(e.target.value)} className="absolute inset-0 cursor-pointer opacity-0" />
            </label>
            <div className="flex h-6 items-center gap-px rounded-md bg-(--color-bg-elevated) px-1.5 text-[11px] font-mono text-(--color-text-secondary)">
              <span className="text-(--color-text-muted)">#</span>
              <input type="text" value={(gc ?? inheritedColor ?? '#3b82f6').replace('#', '').toUpperCase()}
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
/**
 * 게이지 패널 전용 설정 — 유형 / 값 범위 / 단위 / 임계값·컬러.
 *
 * 값을 어디서 가져올지는 여기서 정하지 않는다. 다른 차트 패널과 같은 공용
 * 데이터 소스 섹션(`PanelSettingsDataSource`)이 담당한다 — 게이지만 별도의
 * "값 지정" 편집기를 두면 같은 일을 하는 자리가 둘이 되어, 어느 쪽이 이기는지
 * 사용자가 알 수 없다.
 *
 * 레거시 `config.dataSources[]` 는 **읽는 경로가 그대로 살아 있다**
 * (`gaugeLegacyBinding.resolveGaugeValueSource`). 이미 그 방식으로 묶인 게이지는
 * 계속 같은 값을 그리며, 공용 데이터 소스를 설정하면 그쪽이 이긴다.
 */
/**
 * 게이지 설정의 글자 스타일 3칸 — 글꼴 · 크기 · 색.
 *
 * `prefix` 로 config 키를 만든다(`caption` → `caption_font_family` …). 시리즈 이름과
 * 임계값 범례가 같은 컨트롤을 쓰므로 두 곳의 조작이 갈리지 않는다. 셋 다 비우면 상속.
 */
function GaugeTextStyleFields({
  prefix,
  config,
  onConfigChange,
  testIdPrefix,
  label,
}: {
  prefix: string;
  config: Record<string, unknown>;
  onConfigChange: (patch: Record<string, unknown>) => void;
  testIdPrefix: string;
  label: string;
}): React.ReactElement {
  const { t } = useTranslation();
  const familyKey = `${prefix}_font_family`;
  const sizeKey = `${prefix}_font_size`;
  const colorKey = `${prefix}_font_color`;
  const color = config[colorKey] as string | undefined;
  return (
    <>
      <select
        value={(config[familyKey] as string | undefined) ?? ''}
        onChange={(e) => onConfigChange({ [familyKey]: e.target.value || undefined })}
        data-testid={`${testIdPrefix}-family`}
        aria-label={`${label} ${t('dashboard.chart.fontFamily')}`}
        className="min-w-0 flex-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
      >
        <option value="">{t('dashboard.chart.inherit')}</option>
        {FONT_FAMILY_OPTIONS.map((o) => (
          <option key={o.value} value={o.value}>
            {t(o.labelKey)}
          </option>
        ))}
      </select>
      <input
        type="number"
        min={6}
        max={40}
        value={(config[sizeKey] as number | undefined) ?? ''}
        placeholder={t('dashboard.chart.inherit')}
        onChange={(e) => {
          const v = e.target.value;
          onConfigChange({ [sizeKey]: v === '' ? undefined : parseInt(v, 10) || undefined });
        }}
        data-testid={`${testIdPrefix}-size`}
        aria-label={`${label} ${t('dashboard.chart.fontSize')}`}
        className="w-14 shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-center text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
      />
      <input
        type="color"
        value={color ?? '#9ca3af'}
        onChange={(e) => onConfigChange({ [colorKey]: e.target.value })}
        data-testid={`${testIdPrefix}-color`}
        aria-label={`${label} ${t('dashboard.chart.fontColor')}`}
        className="h-7 w-7 shrink-0 cursor-pointer rounded border border-(--color-border-default) bg-transparent p-0"
      />
      {color !== undefined && (
        <button
          type="button"
          onClick={() => onConfigChange({ [colorKey]: undefined })}
          data-testid={`${testIdPrefix}-color-reset`}
          aria-label={`${label} ${t('dashboard.chart.fontColorReset')}`}
          title={t('dashboard.chart.fontColorReset')}
          className="h-7 w-7 shrink-0 rounded border border-(--color-border-default) text-xs text-(--color-text-muted) hover:bg-(--color-bg-elevated)"
        >
          x
        </button>
      )}
    </>
  );
}

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
  const halfRainbowDirection = readHalfRainbowDirection(config.half_rainbow_direction);
  const baseColor = readBaseColor(config.base_color);
  const needleColor = typeof config.needle_color === 'string' ? config.needle_color : '';
  const valueScale = readValueScale(config.value_scale);
  // 슬라이더는 "미지정" 을 표현할 수 없으므로 기본값(가득)을 그대로 보여 준다.
  const gaugeSize = readPanelSize(config.gauge_size) ?? PANEL_SIZE_MAX;
  // 세로바만 사각형이라 높이를 따로 잡을 수 있다. 원형·반원·바늘은 비율 자체가 값을
  // 읽는 규약의 일부라(각도로 읽는다) 축을 나누면 값을 잘못 읽게 된다.
  const axisSplitGauge = isAxisSplitGauge(config);
  // 세로바의 폭·높이는 게이지 상자의 배율이 아니라 **도형의 치수**다(글자를 함께
  // 누르지 않기 위해서다). 그래서 `gauge_size`(상자 배율)와 나란히 둘 수 있다.
  const vbarWidth = readVBarSize(config.gauge_bar_width);
  const vbarHeight = readVBarSize(config.gauge_bar_height);
  const min = (config.min as number) ?? 0;
  const max = (config.max as number) ?? 100;
  const unit = (config.unit as string) ?? '%';
  const colorMode = (config.colorMode as 'individual' | 'continuous') ?? 'individual';
  const colorTheme = (config.colorTheme as string) ?? 'green-red';
  // 기본 임계값은 게이지와 **같은 함수**에서 온다. 종전에는 여기 하드코딩(0/60/80/100)이라
  // 편집기에는 세 줄이 뜨는데 게이지는 0개로 읽었고, 값 범위를 바꿔도 경계가 따라오지 않았다.
  // 이름만 이 화면의 로케일 문구로 덮는다 — 구간과 색은 정본이 정한다.
  const defaultNames = [
    t('dashboard.settings.gaugeSection.thresholdNormal'),
    t('dashboard.settings.gaugeSection.thresholdCaution'),
    t('dashboard.settings.gaugeSection.thresholdDanger'),
  ];
  const thresholds = (config.thresholds as { name: string; color: string; from: number; to: number }[]) ??
    defaultGaugeThresholds(min, max).map((th, i) => ({ ...th, name: defaultNames[i] ?? '' }));

  // 로컬 드래프트
  const [minDraft, setMinDraft] = useState(String(min));
  const [maxDraft, setMaxDraft] = useState(String(max));

  useEffect(() => { setMinDraft(String(min)); }, [min]);
  useEffect(() => { setMaxDraft(String(max)); }, [max]);

  const commitRange = () => {
    const nMin = parseFloat(minDraft);
    const nMax = parseFloat(maxDraft);
    if (!isNaN(nMin) && !isNaN(nMax)) {
      onConfigChange({ min: nMin, max: nMax });
    }
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

      {/* A-3.4. 시리즈 이름(타일 캡션) — 다중 시리즈에서 각 타일 아래(또는 위)에 붙는
          이름이다. 글자 스타일은 파이 범례와 같은 어휘를 쓴다. */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.gaugeSection.captionStyle')}
        </label>
        <div className="flex items-center gap-1.5">
          <select
            value={(config.caption_position as string) === 'top' ? 'top' : 'bottom'}
            onChange={(e) => onConfigChange({ caption_position: e.target.value })}
            data-testid="gauge-caption-position"
            aria-label={t('dashboard.settings.gaugeSection.captionPosition')}
            className="rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
          >
            <option value="bottom">{t('dashboard.settings.gaugeSection.captionBottom')}</option>
            <option value="top">{t('dashboard.settings.gaugeSection.captionTop')}</option>
          </select>
          <GaugeTextStyleFields
            prefix="caption"
            config={config}
            onConfigChange={onConfigChange}
            testIdPrefix="gauge-caption"
            label={t('dashboard.settings.gaugeSection.captionStyle')}
          />
        </div>
      </div>

      {/* A-3.6. 임계값 범례 — 색이 무엇을 뜻하는지 화면에 남긴다. 설정을 열지 않고는
          구간 색의 의미를 알 수 없기 때문이다. */}
      <div>
        <label className="flex cursor-pointer items-center gap-2 text-xs text-(--color-text-secondary)">
          <input
            type="checkbox"
            checked={config.show_threshold_legend === true}
            onChange={(e) => onConfigChange({ show_threshold_legend: e.target.checked })}
            data-testid="gauge-threshold-legend-show"
            className="h-3.5 w-3.5 rounded border-(--color-border-strong)"
          />
          {t('dashboard.settings.gaugeSection.thresholdLegend')}
        </label>
        {config.show_threshold_legend === true && (
          <div className="mt-1.5 flex items-center gap-1.5">
            <select
              value={
                (config.threshold_legend_position as string) === 'top' ? 'top' : 'bottom'
              }
              onChange={(e) => onConfigChange({ threshold_legend_position: e.target.value })}
              data-testid="gauge-threshold-legend-position"
              aria-label={t('dashboard.settings.gaugeSection.captionPosition')}
              className="rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
            >
              <option value="bottom">{t('dashboard.settings.gaugeSection.captionBottom')}</option>
              <option value="top">{t('dashboard.settings.gaugeSection.captionTop')}</option>
            </select>
            <select
              value={
                config.threshold_legend_orientation === 'vertical' ? 'vertical' : 'horizontal'
              }
              onChange={(e) => onConfigChange({ threshold_legend_orientation: e.target.value })}
              data-testid="gauge-threshold-legend-orientation"
              aria-label={t('dashboard.settings.gaugeSection.thresholdLegendOrientation')}
              className="rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
            >
              <option value="horizontal">
                {t('dashboard.settings.gaugeSection.orientationHorizontal')}
              </option>
              <option value="vertical">
                {t('dashboard.settings.gaugeSection.orientationVertical')}
              </option>
            </select>
            <GaugeTextStyleFields
              prefix="threshold_legend"
              config={config}
              onConfigChange={onConfigChange}
              testIdPrefix="gauge-threshold-legend"
              label={t('dashboard.settings.gaugeSection.thresholdLegend')}
            />
          </div>
        )}
        {config.show_threshold_legend === true && (
          <p className="mt-1 text-[11px] leading-snug text-(--color-text-muted)">
            {t('dashboard.settings.gaugeSection.thresholdLegendDragHint')}
          </p>
        )}
        {/* 끌어 옮긴 자리를 되돌리는 유일한 출구다 — 드래그는 미리보기에서만 된다. */}
        {config.show_threshold_legend === true &&
        (config.threshold_legend_offset_x || config.threshold_legend_offset_y) ? (
          <button
            type="button"
            data-testid="gauge-threshold-legend-reset-offset"
            onClick={() =>
              onConfigChange({
                threshold_legend_offset_x: undefined,
                threshold_legend_offset_y: undefined,
              })
            }
            className="mt-1.5 rounded border border-(--color-border-default) px-2 py-1 text-xs text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
          >
            {t('dashboard.chart.legendResetOffset')}
          </button>
        ) : null}
      </div>

      {/* A-3.5. 게이지 크기·위치 — 그림 전체를 줄이고 옮긴다. 값(A-4)과 다른 축이다:
          값은 게이지 **안에서의** 자리이고, 이것은 패널 안에서의 게이지 자리다.
          파이와 같은 어휘(패널 대비 백분율)를 쓴다. */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.gaugeSection.gaugeSize')}
        </label>
        <div className="flex items-center gap-2">
          <input
            type="range"
            min={PANEL_SIZE_MIN}
            max={PANEL_SIZE_MAX}
            step={1}
            value={gaugeSize}
            onChange={(e) => onConfigChange({ gauge_size: Number(e.target.value) })}
            data-testid="gauge-body-size"
            className="flex-1"
          />
          <span className="w-10 shrink-0 text-right text-xs tabular-nums text-(--color-text-muted)">
            {gaugeSize}%
          </span>
          {/* 크기·자리를 함께 되돌린다 — 둘은 같은 조작(끌기·슬라이더)으로 어긋나므로
              따로 되돌리면 한쪽이 남아 왜 제자리가 아닌지 알 수 없다. */}
          {(config.gauge_size !== undefined ||
            config.gauge_bar_width !== undefined ||
            config.gauge_bar_height !== undefined ||
            config.gauge_offset_x ||
            config.gauge_offset_y) ? (
            <button
              type="button"
              onClick={() =>
                onConfigChange({
                  gauge_size: undefined,
                  gauge_bar_width: undefined,
                  gauge_bar_height: undefined,
                  gauge_offset_x: undefined,
                  gauge_offset_y: undefined,
                })
              }
              data-testid="gauge-body-reset"
              className="shrink-0 rounded-md bg-(--color-bg-elevated) px-2 py-1 text-xs text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)/80"
            >
              {t('dashboard.settings.gaugeSection.valueReset')}
            </button>
          ) : null}
        </div>
        {/* 세로바에만 폭·높이 칸이 나온다 — 사각형이라야 두 축이 따로 뜻을 갖는다.
            원형·반원·바늘은 각도로 값을 읽으므로 찌그러뜨리면 오독한다. */}
        {axisSplitGauge ? (
          <>
            <div className="mt-2">
              <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
                {t('dashboard.settings.gaugeSection.barWidth')}
              </label>
              <div className="flex items-center gap-2">
                <input
                  type="range"
                  min={VBAR_SIZE_MIN}
                  max={VBAR_SIZE_MAX}
                  step={1}
                  value={vbarWidth}
                  onChange={(e) => onConfigChange({ gauge_bar_width: Number(e.target.value) })}
                  data-testid="gauge-bar-width"
                  aria-label={t('dashboard.settings.gaugeSection.barWidth')}
                  className="flex-1"
                />
                <span className="w-10 shrink-0 text-right text-xs tabular-nums text-(--color-text-muted)">
                  {vbarWidth}%
                </span>
              </div>
            </div>
            <div className="mt-2">
              <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
                {t('dashboard.settings.gaugeSection.barHeight')}
              </label>
              <div className="flex items-center gap-2">
                <input
                  type="range"
                  min={VBAR_SIZE_MIN}
                  max={VBAR_SIZE_MAX}
                  step={1}
                  value={vbarHeight}
                  onChange={(e) => onConfigChange({ gauge_bar_height: Number(e.target.value) })}
                  data-testid="gauge-bar-height"
                  aria-label={t('dashboard.settings.gaugeSection.barHeight')}
                  className="flex-1"
                />
                <span className="w-10 shrink-0 text-right text-xs tabular-nums text-(--color-text-muted)">
                  {vbarHeight}%
                </span>
              </div>
            </div>
          </>
        ) : null}
        <p className="mt-1 text-[11px] leading-snug text-(--color-text-muted)">
          {t('dashboard.settings.gaugeSection.gaugeDragHint')}
        </p>
      </div>

      {/* A-4. 현재값 크기·위치 — 유형마다 기본 크기·자리가 달라 **배율과 변위**로 둔다.
          유형을 바꿔도 "조금 크게, 조금 위로" 라는 뜻이 유지된다. 위치는 미리보기에서
          값 글자를 끌어서도 잡을 수 있다(같은 config 를 쓴다). */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.gaugeSection.valueSize')}
        </label>
        <div className="flex items-center gap-2">
          <input
            type="range"
            min={VALUE_SCALE_MIN}
            max={VALUE_SCALE_MAX}
            step={0.05}
            value={valueScale}
            onChange={(e) => onConfigChange({ value_scale: Number(e.target.value) })}
            data-testid="gauge-value-scale"
            className="flex-1"
          />
          <span className="w-10 shrink-0 text-right text-xs tabular-nums text-(--color-text-muted)">
            {valueScale.toFixed(2)}
          </span>
          <button
            type="button"
            onClick={() =>
              onConfigChange({
                value_scale: undefined,
                value_pos_x: undefined,
                value_pos_y: undefined,
                // 값이 도형 안에 있던 시절의 viewBox 단위 키. 남아 있으면 초기화가
                // 반쪽이 되므로 함께 지운다.
                value_offset_x: undefined,
                value_offset_y: undefined,
              })
            }
            data-testid="gauge-value-reset"
            className="shrink-0 rounded-md bg-(--color-bg-elevated) px-2 py-1 text-xs text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)/80"
          >
            {t('dashboard.settings.gaugeSection.valueReset')}
          </button>
        </div>
        <p className="mt-1 text-[11px] leading-snug text-(--color-text-muted)">
          {t('dashboard.settings.gaugeSection.valueDragHint')}
        </p>
      </div>

      {/* A-3. 니들 색 — 니들이 있는 유형에서만 뜻이 있다. 비우면 본문 글자색을 따라
          다크·라이트 양쪽에서 보인다(종전 동작). */}
      {NEEDLE_GAUGE_TYPES.includes(gaugeType) && (
        <div className="flex items-center gap-2">
          <label className="flex items-center gap-2 text-xs text-(--color-text-secondary)">
            <input
              type="checkbox"
              checked={needleColor !== ''}
              onChange={(e) =>
                onConfigChange({ needle_color: e.target.checked ? '#ef4444' : '' })
              }
              className="h-4 w-4 rounded border-(--color-border-default) text-blue-600 focus:ring-blue-500"
              data-testid="gauge-needle-color-enabled"
            />
            <span>{t('dashboard.settings.gaugeSection.needleColor')}</span>
          </label>
          {needleColor !== '' && (
            <label className="relative flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded-md transition-colors hover:opacity-80">
              <span
                className="h-4 w-4 rounded-sm border border-(--color-border-default)"
                style={{ backgroundColor: needleColor }}
              />
              <input
                type="color"
                value={needleColor}
                onChange={(e) => onConfigChange({ needle_color: e.target.value })}
                data-testid="gauge-needle-color"
                className="absolute inset-0 cursor-pointer opacity-0"
              />
            </label>
          )}
        </div>
      )}

      {/* A-2. 반원 방향 — 반원 RB 에서만 뜻이 있다. 다른 타입에서는 컨트롤 자체를
          내린다(있는데 아무 효과가 없는 칸이 가장 헷갈린다). 방향에 따라 캔버스
          배치가 함께 바뀌므로 어느 쪽을 골라도 잘리지 않는다. */}
      {gaugeType === 'half-rainbow' && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.settings.gaugeSection.halfRainbowDirection')}
          </label>
          <div className="flex gap-1">
            {HALF_RAINBOW_DIRECTIONS.map((dir) => (
              <button
                key={dir}
                type="button"
                onClick={() => onConfigChange({ half_rainbow_direction: dir })}
                data-testid={`gauge-half-rainbow-direction-${dir}`}
                aria-pressed={halfRainbowDirection === dir}
                className={cn(
                  'flex-1 rounded-md px-2 py-1.5 text-xs font-medium transition-colors',
                  halfRainbowDirection === dir
                    ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                    : 'bg-(--color-bg-elevated) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)/80',
                )}
              >
                {t(`dashboard.settings.gaugeSection.directions.${dir}`)}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* B~C-3. 값 표기 — 범위·단위·자릿수는 한 묶음이다.
          셋을 각각 한 줄씩 세로로 늘어놓으면 설정 하나가 한 화면을 넘기고, 자릿수처럼
          한 자리 수를 넣는 칸까지 폭을 다 써서 무엇을 넣는 칸인지 흐려진다. */}
      <div className="grid grid-cols-2 gap-3">
      {/* B. 값 범위 — min ~ max 두 칸이라 한 줄을 다 쓴다. */}
      <div className="col-span-2">
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

      {/* C. 단위 — 목록·직접 입력 모두 다른 차트와 같은 컨트롤을 쓴다. */}
      <div className="col-span-2 min-w-0">
        <UnitField
          value={unit}
          onChange={(v) => onConfigChange({ unit: v })}
          testId="gauge-unit"
          label="dashboard.settings.gaugeSection.unit"
        />
      </div>

      {/* C-3. 값 표기 자릿수 — 게이지 가운데 숫자에 적용된다. 눈금 라벨은 눈금자이므로
          종전대로 정수 반올림을 유지한다.

          단위와 같은 행에 두었더니 "직접 입력" 을 고를 때 나타나는 입력칸이 자릿수 칸을
          밀어냈다. 자릿수를 한 행 아래로 내려 두 설정이 서로 폭을 다투지 않게 한다. */}
      <div className="col-span-2 min-w-0 sm:col-span-1">
        <DecimalPlacesField
          config={config}
          onConfigChange={onConfigChange}
          testId="gauge-decimal-places"
        />
      </div>
      </div>

      {/* C-2. 다중 출력 배열 — 시리즈가 2개 이상일 때 게이지를 몇 행으로 늘어놓을지. */}
      <TileRowsField
        value={config.tile_rows as number | undefined}
        onChange={(tile_rows) => onConfigChange({ tile_rows })}
      />

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

        {/* 기본색 — 임계 구간 테두리 **안쪽**을 채우는 색. 비우면 채우지 않아
            테두리만 남는다. 미지정과 "채우지 않음" 은 다른 뜻이라 체크박스로 나눈다. */}
        <div className="mb-2 flex items-center gap-2">
          <label className="flex items-center gap-2 text-xs text-(--color-text-secondary)">
            <input
              type="checkbox"
              checked={baseColor !== ''}
              onChange={(e) =>
                onConfigChange({ base_color: e.target.checked ? DEFAULT_TRACK_FILL : '' })
              }
              className="h-4 w-4 rounded border-(--color-border-default) text-blue-600 focus:ring-blue-500"
              data-testid="gauge-base-color-enabled"
            />
            <span>{t('dashboard.settings.gaugeSection.baseColor')}</span>
          </label>
          {baseColor !== '' && (
            <label className="relative flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded-md transition-colors hover:opacity-80">
              <span
                className="h-4 w-4 rounded-sm border border-(--color-border-default)"
                style={{ backgroundColor: baseColor }}
              />
              <input
                type="color"
                value={baseColor}
                onChange={(e) => onConfigChange({ base_color: e.target.value })}
                data-testid="gauge-base-color"
                className="absolute inset-0 cursor-pointer opacity-0"
              />
            </label>
          )}
        </div>

        {colorMode === 'individual' ? (
          <div className="space-y-1.5">
            {thresholds.map((th, idx) => (
              <div key={idx} className="flex w-full items-center gap-1.5">
                <label className="relative flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded-md transition-colors hover:opacity-80">
                  <span
                    className="h-4 w-4 rounded-sm border border-(--color-border-default)"
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

/**
 * 합성 샘플값으로 그리는 게이지 미리보기 — 레거시 경로에는 실제 값이 없을 수 있다.
 *
 * **배치 편집은 여기서도 켠다.** 종전에는 `onConfigChange` 로 빈 함수를 넘기고
 * `forceEdit` 도 주지 않아, 설정 화면에서 값 글자·게이지 상자를 끌 수 없었다(대시보드에
 * 놓인 같은 패널에서는 됐다 — 실제로 그렇게 보고됐다). 값이 합성이라는 것과 **자리를
 * 옮길 수 있다는 것은 다른 축**이다: 배치는 데이터와 무관한 시각 설정이다.
 */
function GaugeMiniPreview({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  /** config 를 쓸 콜백. 없으면 종전처럼 보기 전용이다. */
  onConfigChange?: (patch: Record<string, unknown>) => void;
}) {
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
      {/*
        **flex 컨테이너여야 한다.** 평범한 블록이면 패널 뿌리의 `flex-1` 이 아무 뜻도
        갖지 못해 높이가 내용으로 정해지고, 게이지 내용의 높이는 SVG 의 고유 비율이라
        유형마다 달라진다 — 반원(240×140)에서는 패널이 미리보기 상자보다 짧아져
        그리드가 일부만 덮이고, 도형의 이동 범위가 위로 치우치며, 아래쪽 띠에는
        닿지 못했다. 다른 패널 미리보기들이 쓰는 것과 같은 상자다.
      */}
      <div className="flex min-h-0 flex-1 flex-col">
        <GaugePanel
          panelId="__preview__"
          title=""
          config={previewConfig}
          onConfigChange={onConfigChange ?? (() => {})}
          onTitleChange={() => {}}
          // 미리보기는 항상 편집이다 — 토글은 감춘다(실 패널 경로와 같은 규칙).
          forceEdit={onConfigChange !== undefined}
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

function LineChartMiniPreview({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  /** 범례를 끌어 옮긴 값을 쓸 곳. 실제 패널 미리보기와 같은 조작을 여기서도 준다. */
  onConfigChange?: (config: Record<string, unknown>) => void;
}) {
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
  const legendCfg = (config.legend as ChartLegendConfig | undefined) ?? {};
  const legendPos = legendCfg.position ?? 'bottom';
  const isLegendVert = legendPos === 'left' || legendPos === 'right';

  // 미리보기 시리즈 산출은 순수 함수(buildPreviewSeries)에 위임한다 — store 모드에서
  // 선택된 시리즈와 패널의 시리즈 이름 형식을 실제 렌더와 같은 규칙으로 반영한다.
  const isStoreMode = (config.data_source as string | undefined) === 'store';
  const storeSource = config.store_source as StoreSourceConfig | undefined;
  const storeSeriesCount = storeSource?.series?.length ?? 0;

  const series = useMemo(
    () =>
      buildPreviewSeries({
        dataSource: config.data_source as string | undefined,
        storeSource,
        tsdbSource: config.tsdb_source as TsdbSourceConfig | undefined,
        // sysmetrics 소스를 넘기지 않으면 소스 판정이 channel 로 떨어져 미리보기가
        // sample 한 줄로 퇴화한다 — 시스템 지표 패널에서 스타일 변화가 보이지 않던 자리다.
        sysmetricsSource: config.sysmetrics_source as SysmetricsSourceConfig | undefined,
        channels,
        channelName,
        globalSmooth,
        panelGraphStyle: readGraphStyle(config.graph_style),
        strokeDasharray: STROKE_DASHARRAY,
        palette: PREVIEW_FALLBACK_PALETTE,
        sampleName: t('dashboard.settings.preview.sample'),
        channelFallbackName: (i) =>
          t('dashboard.settings.preview.channelFallback').replace('{index}', String(i)),
      }),
    [
      config.data_source,
      config.tsdb_source,
      config.sysmetrics_source,
      config.graph_style,
      storeSource,
      channels,
      channelName,
      globalSmooth,
      t,
    ],
  );

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
        const wave = (n: number): number => 50 + 30 * Math.sin((n / points) * Math.PI * 2 + phase);
        if (s.graphStyle === 'candle') {
          // 캔들은 값 하나로 그릴 수 없다 — 시·고·저·종 네 값이 있어야 몸통과 꼬리가 선다.
          // 합성 파형의 이웃 두 점을 시가·종가로 삼고 꼬리를 붙인다. 행 키는 실제 렌더와
          // 같은 규칙(`candleRows`)을 따라야 모양 함수가 값을 찾는다.
          const open = wave(i);
          const close = wave(i + 1);
          const body = Math.abs(close - open) || 1;
          candleRows(s.key, [
            {
              timestamp: i,
              open,
              close,
              high: Math.max(open, close) + body * 0.6,
              low: Math.min(open, close) - body * 0.6,
            },
          ]).forEach((r) => {
            for (const [k, v] of Object.entries(r)) {
              if (k !== 'timestamp') (row as Record<string, unknown>)[k] = v;
            }
          });
          return;
        }
        row[s.key] = wave(i);
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
          {isStoreMode
            ? t('dashboard.settings.preview.seriesCount').replace('{count}', String(storeSeriesCount))
            : isMultiMode
              ? t('dashboard.settings.preview.channelCount').replace('{count}', String(channels.length))
              : channelName || t('dashboard.settings.preview.channelUnset')}
        </span>
      </div>
      <ChartDragLayer
        legend={{
          offsetX: clampStoredLegendOffset(legendCfg.offset_x),
          offsetY: clampStoredLegendOffset(legendCfg.offset_y),
          onChange: ({ x, y }) =>
            onConfigChange?.({ legend: { ...legendCfg, offset_x: x, offset_y: y } }),
        }}
        plot={{
          offsetX: readPanelOffset(config.plot_offset_x),
          offsetY: readPanelOffset(config.plot_offset_y),
          onChange: ({ x, y }) => onConfigChange?.({ plot_offset_x: x, plot_offset_y: y }),
        }}
      >
      <div
        className={cn(
          'flex min-h-0 flex-1',
          isLegendVert ? 'flex-row' : 'flex-col',
          legendPos === 'left' ? 'flex-row-reverse' : '',
        )}
        // 범례를 끌 수 있는 범위 — 실제 패널과 같은 표식이다.
        data-chart-legend-bounds=""
      >
        <div
          className="min-h-0 min-w-0 flex-1"
          // 실제 패널과 같은 표식·같은 변환 — 미리보기에서 끈 자리가 대시보드와 달라지면
          // 미리보기가 제 일을 못 한다.
          data-chart-plot-area=""
          style={{
            transform: panelBoxTransform(
              readPanelSize(config.plot_size) ?? PANEL_SIZE_MAX,
              readPanelOffset(config.plot_offset_x),
              readPanelOffset(config.plot_offset_y),
            ),
          }}
        >
        <ResponsiveContainer width="100%" height="100%">
          <ComposedChart data={data} margin={{ top: 8, right: 16, left: yAxisLabel ? 16 : 0, bottom: xLabel ? 20 : 0 }}>
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
            {/* 모양은 실제 패널과 같은 분기다 — 미리보기가 라인으로만 그리면
                "스타일을 바꿔도 샘플이 그대로" 가 된다. */}
            {series.map((s) => {
              const common = {
                key: s.key,
                dataKey: s.key,
                isAnimationActive: false,
              } as const;
              if (s.graphStyle === 'candle') {
                return (
                  <Bar
                    {...common}
                    shape={(p: object) => <CandleShape {...p} seriesKey={s.key} color={s.color} />}
                  />
                );
              }
              if (s.graphStyle === 'bar') return <Bar {...common} fill={s.color} />;
              if (s.graphStyle === 'area') {
                return (
                  <Area
                    {...common}
                    type={s.smooth ? 'monotone' : 'linear'}
                    stroke={s.color}
                    strokeWidth={s.strokeWidth}
                    strokeDasharray={s.strokeDasharray || undefined}
                    fill={s.color}
                    fillOpacity={0.25}
                    dot={false}
                  />
                );
              }
              return (
                <Line
                  {...common}
                  type={s.smooth ? 'monotone' : 'linear'}
                  stroke={s.color}
                  strokeWidth={s.strokeWidth}
                  strokeDasharray={s.strokeDasharray || undefined}
                  dot={false}
                />
              );
            })}
          </ComposedChart>
        </ResponsiveContainer>
        </div>
        {/* 범례 — 실제 패널과 같은 컴포넌트/배치. recharts 내장 Legend 를 쓰면 구분선·여백과
            범례 옵션(이름/선/마지막 값)이 실제 렌더와 달라진다. */}
        <ChartLegend
          seriesKeys={series.map((s) => s.key)}
          seriesColors={series.map((s) => s.color)}
          legendCfg={legendCfg}
          chartData={data}
          formatValue={(_key, v) => (enumMode ? formatEnumValue(v, enumMap) : v.toFixed(1))}
        />
      </div>
      </ChartDragLayer>
    </div>
  );
}




/** 리소스 패널 미니 프리뷰 */

/**
 * 시스템 통계 패널 미니 프리뷰.
 *
 * 실제 패널과 같은 규칙으로 그린다 — 열 개수 상한을 반영하고, 색 영역은 패널이
 * 실제로 소비하는 그룹(_base / header / label / value)만 노출한다. 패널이 쓰지 않는
 * 그룹을 프리뷰에 두면 색을 골라도 아무 일이 없는 빈 약속이 된다.
 */





// ---- 3분할 설정 셸 (T1/T2) ----
// @spec SPEC-PANEL-SETTINGS-001 (REQ-01/REQ-02)
// 다이얼로그 본문을 미리보기(좌상단)·데이터소스(좌하단)·옵션(우측) 3영역으로 배치하는 셸.
// 2경계 드래그 리사이즈: 좌우(좌측 컬럼↔옵션) + 상하(미리보기↔데이터소스). 비율 영속은
// 상위(usePanelSettingsRatio)가 담당하고, 셸은 값(leftWidth/previewRatio)과 드래그 개시
// 콜백(setIsDragging/onPreviewSplitterMouseDown)만 받는다.
function PanelSettingsShell({
  options,
  preview,
  dataSource,
  previewCollapsed,
  setPreviewCollapsed,
  previewFillMode,
  setPreviewFillMode,
  dataSourceBelowPreview,
  leftWidth,
  isDragging,
  setIsDragging,
  previewRatio,
  previewSplitterDragging,
  onPreviewSplitterMouseDown,
  previewZoom,
  zoomIn,
  zoomOut,
  zoomReset,
  previewZoomMin: PREVIEW_ZOOM_MIN,
  previewZoomMax: PREVIEW_ZOOM_MAX,
  onResetLayout,
  t,
}: {
  options: React.ReactNode;
  preview: React.ReactNode;
  dataSource: React.ReactNode;
  previewCollapsed: boolean;
  setPreviewCollapsed: React.Dispatch<React.SetStateAction<boolean>>;
  previewFillMode: 'fill' | 'fit';
  setPreviewFillMode: React.Dispatch<React.SetStateAction<'fill' | 'fit'>>;
  dataSourceBelowPreview: boolean;
  leftWidth: number;
  isDragging: boolean;
  setIsDragging: React.Dispatch<React.SetStateAction<boolean>>;
  previewRatio: number;
  previewSplitterDragging: boolean;
  onPreviewSplitterMouseDown: () => void;
  previewZoom: number;
  zoomIn: () => void;
  zoomOut: () => void;
  zoomReset: () => void;
  previewZoomMin: number;
  previewZoomMax: number;
  /** 배치(그림 상자·범례 자리)를 되돌린다. 되돌릴 것이 없으면 `undefined` — 버튼을 내지 않는다. */
  onResetLayout?: () => void;
  t: TranslationFn;
}) {
  // 상하 경계는 미리보기 + 데이터소스가 함께 존재할 때만(접힘 아님 + 차트/heatmap) 노출.
  const showHSplit = !previewCollapsed && dataSourceBelowPreview;
  return (
        <div
          data-panel-settings-content
          className="relative flex min-h-0 flex-1 gap-3 px-5 pb-5 pt-4"
        >
          <div
            data-testid="panel-settings-options"
            style={{
              // 차트/히트맵 패널은 접힘 상태에서도 좌측 데이터 소스 영역이 남으므로
              // 설정 컬럼을 고정 폭으로 유지한다. 그 외에는 접힘 시 전체 폭.
              width:
                previewCollapsed && !dataSourceBelowPreview ? '100%' : `${leftWidth}px`,
            }}
            className={cn(
              'order-3 shrink-0 space-y-0.5 overflow-y-auto pr-1',
              previewCollapsed && !dataSourceBelowPreview && 'flex-1',
            )}
          >
            {options}
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
            미리보기 자체는 fit 컨테이너가 남은 높이를 채우며 양축 가운데 정렬하고, 사용자가
            ± 버튼이나 Ctrl+휠 로 확대/축소 할 수 있다.
          */}
          {(!previewCollapsed || dataSourceBelowPreview) && (
          <div
            data-testid="panel-settings-preview"
            className={cn(
              // min-h-0: 세로 방향으로 자식(미리보기 블록 → fit 컨테이너)이 남은 높이를 온전히
              // 받도록 한다(누락 시 flex-1/height:100% 체인이 콘텐츠 높이로 붕괴 → 미리보기 짧아짐).
              'order-1 flex min-h-0 min-w-0 flex-1 flex-col items-stretch justify-start',
              showHSplit ? 'gap-0 overflow-hidden' : 'gap-3 overflow-y-auto',
            )}
          >
            {/*
              프리뷰 영역(툴바 + 미리보기 블록)은 접히면 숨긴다. 차트/히트맵 패널의
              데이터 소스 섹션은 이 아래에 별도로 항상 노출된다(SPEC-WEB-005).
              상하 경계(showHSplit)일 때 미리보기는 previewRatio 높이를 차지한다.
            */}
            {!previewCollapsed && (
            <div
              className={cn('flex flex-col gap-3', showHSplit ? 'min-h-0 overflow-hidden' : 'min-h-0 flex-1')}
              style={showHSplit ? { flexBasis: `${previewRatio * 100}%`, flexGrow: 0, flexShrink: 0 } : undefined}
            >
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
                {/* 채움/맞춤 토글: fill(영역 채움) ↔ fit(종횡비 보존 최대 맞춤). */}
                <button
                  type="button"
                  onClick={() => setPreviewFillMode((m) => (m === 'fill' ? 'fit' : 'fill'))}
                  data-testid="panel-settings-preview-fill-toggle"
                  aria-label={
                    previewFillMode === 'fill'
                      ? t('dashboard.settings.previewModeFillAria')
                      : t('dashboard.settings.previewModeFitAria')
                  }
                  title={
                    previewFillMode === 'fill'
                      ? t('dashboard.settings.previewModeFillAria')
                      : t('dashboard.settings.previewModeFitAria')
                  }
                  className="flex h-5 w-5 items-center justify-center rounded text-(--color-text-muted) hover:bg-(--color-bg-hover) hover:text-(--color-text-default)"
                >
                  {previewFillMode === 'fill' ? (
                    <Maximize2 className="h-3 w-3" />
                  ) : (
                    <Minimize2 className="h-3 w-3" />
                  )}
                </button>
                {/* 배치 초기화 — 미리보기에서 끌어 옮긴 결과를 한 번에 되돌린다.
                    그림 상자와 범례는 **같은 화면에서 같은 조작(끌기)** 으로 어긋나므로
                    한 버튼이 둘을 함께 되돌린다. 따로 두면 한쪽이 남아 왜 제자리가
                    아닌지 알 수 없다. 되돌릴 것이 없으면 버튼을 내지 않는다. */}
                {onResetLayout && (
                  <>
                    <button
                      type="button"
                      onClick={onResetLayout}
                      data-testid="panel-settings-preview-reset-layout"
                      aria-label={t('dashboard.settings.previewResetLayout')}
                      title={t('dashboard.settings.previewResetLayout')}
                      className="flex h-5 w-5 items-center justify-center rounded text-(--color-text-muted) hover:bg-(--color-bg-hover) hover:text-(--color-text-default)"
                    >
                      <RotateCcw className="h-3 w-3" />
                    </button>
                  </>
                )}
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
            {/* preview(previewSlot) 자체가 fit 컨테이너(ref 실측 + flex-1 + 가운데 정렬)다. */}
            {preview}
            {/* 악센트 그룹 컨트롤은 좌측 컬럼으로 이동되었음 (스타일 섹션) */}
            </div>
            )}

            {/* 상하(가로) 경계 드래그 리사이저 (미리보기↔데이터소스). */}
            {showHSplit && (
              <div
                role="separator"
                aria-orientation="horizontal"
                aria-label={t('dashboard.settings.previewSplitterAria')}
                data-testid="panel-settings-preview-splitter"
                onMouseDown={(e) => {
                  e.preventDefault();
                  onPreviewSplitterMouseDown();
                }}
                className={cn(
                  'group relative -my-1 flex h-2 shrink-0 cursor-row-resize items-center justify-center',
                  previewSplitterDragging && 'bg-blue-500/20',
                )}
              >
                <div
                  className={cn(
                    'h-0.5 w-12 rounded-full bg-(--color-border-default) transition-colors group-hover:bg-blue-400',
                    previewSplitterDragging && 'bg-blue-500',
                  )}
                />
              </div>
            )}

            {dataSource && (
              <div className={cn(showHSplit ? 'min-h-0 flex-1 overflow-y-auto' : 'shrink-0')}>
                {dataSource}
              </div>
            )}
          </div>
          )}
        </div>
  );
}
