// 패널 상세 설정 다이얼로그.
// 편집 모드에서 패널별 설정(타이틀, 색상, 컬럼/필드 가시성, 타입별 설정)을 관리한다.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ArrowLeft, ArrowUpDown, Check, ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Fan, Gauge, Maximize2, Minimize2, Minus, Pipette, Plus, Power, Snowflake, Thermometer, Trash2, X } from 'lucide-react';
import {
  CartesianGrid,
  Line,
  LineChart,
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
import { useTranslation, type TranslationFn } from '@/lib/i18n';

import { useAgents } from '@/hooks/useAgent';
import {
  SysResourceSelector,
  type SysResourceKind,
} from '@/components/property/SysResourceSelector';
import {
  PANEL_SIZE_MAX,
  PANEL_SIZE_MIN,
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
  resolveItemOptions,
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
import { getDeviceDisplayName, getDeviceTypeLabel, getPropertyLabel } from '@/lib/utils/deviceLabels';
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
} from './panels/charts/chartChannelTypes';
import { ChartLegend } from './panels/charts/ChartLegend';
import { buildPreviewSeries } from './panels/charts/previewSeries';
// SPEC-HEATMAP-PANEL-001: 히트맵 설정 섹션(store 태그 + 센서 좌표 + 상하한 + 색상표 + IDW).
import {
  parseHeatmapConfig,
  DEFAULT_CONTOUR_LEVEL_COUNT,
  DEFAULT_LEGEND_TICK_COUNT,
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
import { resolvePanelSourceBinding } from './panels/charts/panelDataSource';
// "채널이 아닌 활성 소스인가" 판정 — 소스 종류가 늘어도 식이 그대로다.
import { isPanelSeriesSource } from './panels/charts/usePanelSeriesData';
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
} from './ChartPanelSections';
import { PanelSettingsDataSource } from './PanelSettingsDataSource';
import { useDraftPanelConfig } from './useDraftPanelConfig';
import { useDebouncedValue } from './useDebouncedValue';
import { usePanelSettingsRatio } from './usePanelSettingsRatio';
import { SECTION_CATALOG, findItemMeta } from '@/pages/monitoring/monitoringCatalog';
import { interfaceColors } from '@/pages/monitoring/networkSeries';
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
  const layoutItem = activePage?.layout.find((l) => l.i === panelId) ?? null;
  const gridCell = gridCellSize(gridWidth, gridCols);
  const panelAspect = panelPixelAspect(layoutItem?.w ?? 0, layoutItem?.h ?? 0, gridCell);

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
  }, [storePanel, draftConfig, draftTitle, updatePanelConfig, updatePanelTitle]);

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
    // 인식 불가 문자열도 계약이 channel 로 접으므로 함께 이관된다(§2.17-2).
    if (resolvePanelSourceBinding(draftConfig).kind !== 'channel') return;
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
  // 디바운스에서 **빼는** 필드 — 데이터 조회에 전혀 관여하지 않고 그리기만 바꾸는 값들이다.
  //
  // 디바운스를 둔 이유는 "잦은 편집이 재조회를 부르는 것" 을 막기 위해서다. 조회와
  // 무관한 값까지 200ms 늦추면, 값 글자를 끌 때 미리보기가 200ms 계단으로 따라와
  // 드래그가 뚝뚝 끊긴다. 조회 축이 아닌 값은 즉시 반영한다.
  const livePreviewConfig = useMemo(() => {
    const live: Record<string, unknown> = { ...debouncedConfig };
    for (const key of PREVIEW_LIVE_KEYS) {
      if (key in draftConfig) live[key] = draftConfig[key];
      else delete live[key];
    }
    return live;
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

  // 미리보기 줌 배율 (0.5 ~ 2.0). 버튼 / Ctrl+휠 / 더블클릭 리셋 으로 조절.
  const PREVIEW_ZOOM_MIN = 0.5;
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
                <TitleSection panel={panel} onTitleChange={(v) => handleTitleChange(v)} />
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
                <ColumnsSection<DeviceListColumnKey>
                  allColumns={[...ALL_DEVICE_COLUMNS]}
                  labels={Object.fromEntries(ALL_DEVICE_COLUMNS.map((k) => [k, t(DEVICE_COLUMN_LABELS[k])])) as Record<DeviceListColumnKey, string>}
                  visibleColumns={(panel.config?.visibleColumns as DeviceListColumnKey[]) ?? [...ALL_DEVICE_COLUMNS]}
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
  const previewSourceBinding = resolvePanelSourceBinding(previewChartConfig);
  // 소스 종류를 **열거하지 않는다**. 'store' 만 보던 때는 TSDB 패널이, 'store'|'tsdb' 만
  // 보던 때는 sysmetrics 패널이 영원히 합성 미리보기에 머물렀다 — 종류가 늘 때마다 이
  // 자리를 고쳐야 하는 것이 결함의 원인이었다. 계약의 술어(`isPanelSeriesSource` =
  // 채널이 아니고 활성)를 그대로 쓰면 다음 종류에서 같은 일이 반복되지 않는다.
  //
  // previewRealData 가 거짓이면 실제 렌더를 쓰지 않고 합성 미리보기로 내려간다.
  const isPreviewStoreActive = previewRealData && isPanelSeriesSource(previewSourceBinding);
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
  // 채널 모드는 종전대로 미리보기가 없다. 실제 조회를 끈 경우(previewRealData 해제)도 같다 —
  // 그 토글의 의미가 "실제 질의를 내지 않는다" 이므로 대신 보여줄 합성 화면이 없다.
  const isSeriesSourcePreview =
    previewSourceBinding.kind !== 'channel' && previewRealData;
  const isStoreStatPreview = previewRenderPanel.type === 'stat' && isSeriesSourcePreview;
  // gauge: 값 소스 판정의 단일 정본(`resolveGaugeValueSource`)을 그대로 쓴다. 레거시 경로가
  // 이기는 동안에는 합성 샘플값 미니 프리뷰가 그대로 남는다(레거시는 실제 값이 없을 수 있다).
  const isStoreGaugePreview =
    previewRenderPanel.type === 'gauge' &&
    resolveGaugeValueSource(gaugeValueSourceFlags(previewChartConfig)) === 'store-source';
  // 종횡비 보존 미리보기(라운드 게이지, 작은 accent device/ac/hvac): 높이를 채우고 폭은
  // 종횡비로 파생한다. previewZoom(0.5~2.0)이 곱해진다(±/Ctrl+휠/더블클릭).
  const previewFitStyle = (aspect: string): React.CSSProperties => ({
    height: `${100 * previewZoom}%`,
    maxWidth: '100%',
    maxHeight: '100%',
    aspectRatio: aspect,
  });
  // FILL 미리보기(heatmap/차트/리스트/리소스/로그/modbus): 종횡비를 무시하고 fit 컨테이너를
  // 가로·세로 모두 채운다. 영역 비율은 사용자가 경계 드래그(leftWidth/previewRatio)로 조절한다.
  // zoom=1.0 → 100%×100%. previewZoom 이 곱해진다(±/Ctrl+휠/더블클릭).
  const previewFillStyle = (): React.CSSProperties => ({
    width: `${100 * previewZoom}%`,
    height: `${100 * previewZoom}%`,
  });
  // FIT 미리보기(실측 contain): fit 컨테이너 실측(W×H)과 패널 종횡비 r 로 "가장 큰 종횡비
  // 보존 박스"를 px 로 계산한다(CSS transferred-size 불확실성 제거). W/H>=r → 높이 바운드
  // (높이 가득 + 좌우 여백), 아니면 폭 바운드(폭 가득 + 상하 여백). previewZoom 곱함.
  // 측정 불가(0, jsdom/초기)면 CSS previewFitStyle 로 폴백(테스트 안정 + 초기 페인트).
  const parseAspectRatio = (aspect: string): number => {
    const parts = aspect.split('/').map((s) => parseFloat(s.trim()));
    const a = parts[0] ?? NaN;
    const b = parts[1] ?? NaN;
    return Number.isFinite(a) && Number.isFinite(b) && b !== 0 ? a / b : 1;
  };
  const measuredFitStyle = (aspect: string): React.CSSProperties => {
    const { w, h } = fitSize;
    if (w <= 0 || h <= 0) return previewFitStyle(aspect); // 측정 불가 → CSS 폴백
    const r = parseAspectRatio(aspect);
    let boxW: number;
    let boxH: number;
    if (w / h >= r) {
      // 영역이 더 넓다 → 높이 바운드: 높이 가득, 폭은 종횡비로 파생(좌우 여백).
      boxH = h;
      boxW = h * r;
    } else {
      // 영역이 더 좁다 → 폭 바운드: 폭 가득, 높이는 종횡비로 파생(상하 여백).
      boxW = w;
      boxH = w / r;
    }
    return { width: `${boxW * previewZoom}px`, height: `${boxH * previewZoom}px` };
  };
  const previewSlot = (
    // fit 컨테이너: 남은 미리보기 영역을 세로로 가득(min-h-0 flex-1) 차지하고 자식을 양축
    // 가운데 정렬한다. ref 로 실측하여 fit 모드가 종횡비 보존 contain 을 결정론적으로 계산한다.
    // 크롬 옵션은 draft config 로 전파해 타이틀 바 토글이 미리보기에 즉시 반영되게 한다.
    <PanelChromeProvider config={panel.config}>
    <div
      ref={setFitContainer}
      className="flex min-h-0 flex-1 items-center justify-center overflow-hidden"
    >
            {/*
              FILL 유형(heatmap/차트/리스트/리소스/로그/modbus)은 previewFillStyle 로 fit 컨테이너를
              가득 채우고(영역 비율은 드래그로 조절), 종횡비가 중요한 미니(게이지/accent)는
              previewFitStyle 로 종횡비를 보존한다. previewZoom 이 곱해진다(±/Ctrl+휠/더블클릭).
              wheel 핸들러는 개별 wrapper 에 부여한다 (Ctrl+휠 으로만 동작하므로 기본 스크롤 보존).
            */}
            {(panel.type === 'device' || panel.type === 'ac-control' || panel.type === 'hvac-control') && (
              <div
                style={previewFitStyle('3 / 2')}
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
                style={previewFillMode === 'fill' ? previewFillStyle() : measuredFitStyle('3 / 2')}
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
                style={previewFillMode === 'fill' ? previewFillStyle() : measuredFitStyle('3 / 2')}
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
                style={previewFillMode === 'fill' ? previewFillStyle() : measuredFitStyle('4 / 3')}
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
            {panel.type === 'monitor-network' && (
              <div
                style={previewFillMode === 'fill' ? previewFillStyle() : measuredFitStyle('4 / 3')}
                onWheel={handlePreviewWheel}
              >
                <MonitorNetworkMiniPreview
                  panel={panel}
                  selectedGroup={selectedGroup}
                  onSelectGroup={setSelectedGroup}
                  effectiveColor={effectiveColor}
                  panelColor={panelColor}
                />
              </div>
            )}
            {SYSMETRICS_PANEL_TYPES.has(panel.type) && (
              <div
                style={previewFillMode === 'fill' ? previewFillStyle() : measuredFitStyle('4 / 3')}
                onWheel={handlePreviewWheel}
              >
                <SysMetricsMiniPreview
                  panel={panel}
                  selectedGroup={selectedGroup}
                  onSelectGroup={setSelectedGroup}
                  effectiveColor={effectiveColor}
                  panelColor={panelColor}
                />
              </div>
            )}
            {panel.type === 'monitor-stats' && (
              <div
                style={previewFillMode === 'fill' ? previewFillStyle() : measuredFitStyle('4 / 3')}
                onWheel={handlePreviewWheel}
              >
                <MonitorStatsMiniPreview
                  panel={panel}
                  selectedGroup={selectedGroup}
                  onSelectGroup={setSelectedGroup}
                  effectiveColor={effectiveColor}
                  panelColor={panelColor}
                />
              </div>
            )}
            {panel.type === 'monitor-metrics' && (
              <div
                style={previewFillMode === 'fill' ? previewFillStyle() : measuredFitStyle('4 / 3')}
                onWheel={handlePreviewWheel}
              >
                <MonitorMetricsMiniPreview
                  panel={panel}
                  selectedGroup={selectedGroup}
                  onSelectGroup={setSelectedGroup}
                  effectiveColor={effectiveColor}
                  panelColor={panelColor}
                />
              </div>
            )}
            {panel.type === 'logs' && (
              <div
                style={previewFillMode === 'fill' ? previewFillStyle() : measuredFitStyle('3 / 2')}
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
                // Store 실데이터 경로에서는 실제 GaugePanel 이 게이지 배열을 그리므로 세로를
                // 채울 flex 컨테이너가 필요하다(라인 차트 프리뷰와 같은 이유). 미니 프리뷰는
                // 자체 h-full 이라 두 경로 모두 안전하다.
                className="flex min-h-0 flex-col"
                data-testid="gauge-preview-wrapper"
                style={
                  isStoreGaugePreview && previewFillMode === 'fill'
                    ? previewFillStyle()
                    : previewFitStyle('1 / 1')
                }
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
                    <GaugeMiniPreview panel={previewRenderPanel} />
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
                style={previewFillMode === 'fill' ? previewFillStyle() : measuredFitStyle('3 / 2')}
                onWheel={handlePreviewWheel}
              >
                <StatPanel
                  panelId={previewRenderPanel.id}
                  title={previewRenderPanel.title}
                  config={previewRenderPanel.config ?? {}}
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
                style={previewFillMode === 'fill' ? previewFillStyle() : measuredFitStyle('16 / 9')}
                onWheel={handlePreviewWheel}
              >
                <BarChartPanel
                  panelId={previewRenderPanel.id}
                  title={previewRenderPanel.title}
                  config={previewRenderPanel.config ?? {}}
                />
              </div>
            )}
            {previewRenderPanel.type === 'table' && isSeriesSourcePreview && (
              <div
                // 다른 실패널 미리보기와 같은 이유로 flex 컨테이너여야 한다 — 테이블 패널
                // 루트가 flex-1 이라 plain block 안에서는 높이가 콘텐츠로 붕괴한다.
                className="flex min-h-0 flex-col"
                data-testid="table-preview-wrapper"
                style={previewFillMode === 'fill' ? previewFillStyle() : measuredFitStyle('3 / 2')}
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
                style={previewFillMode === 'fill' ? previewFillStyle() : measuredFitStyle('4 / 3')}
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
                style={previewFillMode === 'fill' ? previewFillStyle() : measuredFitStyle('16 / 9')}
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
                  />
                ) : (
                  <LineChartMiniPreview panel={previewRenderPanel} />
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
                style={previewFillMode === 'fill' ? previewFillStyle() : measuredFitStyle('3 / 2')}
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
                // fit 모드는 **이 패널의 실제 대시보드 비율**로 그린다(레이아웃 미상이면 3:2 폴백).
                // 히트맵은 도면 종횡비로 스테이지를 레터박스하므로(stage.ts), 미리보기 비율이
                // 실제와 다르면 여백이 얼마나 생길지 확인할 방법이 없다.
                style={
                  previewFillMode === 'fill'
                    ? previewFillStyle()
                    : measuredFitStyle(panelAspect !== undefined ? `${panelAspect} / 1` : '3 / 2')
                }
                onWheel={handlePreviewWheel}
              >
                <HeatmapPanel
                  panelId={panel.id}
                  title={panel.title}
                  config={panel.config}
                  onConfigChange={(c) => handleConfigChange(c)}
                  forcePlacement
                />
                {/* 실제 대시보드에서 이 패널이 차지할 영역을 점선으로 표시한다. 채움(fill)
                    모드는 미리보기 영역을 가로·세로로 모두 채우므로 실제 비율과 다르고,
                    그 상태에서는 도면이 대시보드에서 어디까지 보일지 알 수 없다. */}
                {panelAspect !== undefined && previewFillMode === 'fill' && (
                  <PanelAreaOutline aspect={panelAspect} />
                )}
              </div>
            )}
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
        {isPanelSeriesSource(previewSourceBinding) && (
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
              className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
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
              {/* 위치(4모서리). */}
              <div>
                <label className="mb-1 block text-[11px] text-(--color-text-muted)">
                  {t('dashboard.settings.heatmapLegendPosition')}
                </label>
                <select
                  value={legendCfg?.position ?? 'bottom-right'}
                  data-testid="heatmap-legend-position"
                  onChange={(e) =>
                    // 모서리를 다시 고르면 드래그로 저장된 자유 위치(offset)를 버린다 — 남겨두면
                    // offset 이 우선하므로 select 를 바꿔도 범례가 움직이지 않아 고장으로 보인다.
                    setLegend({
                      position: e.target.value as LegendConfig['position'],
                      offset: undefined,
                    })
                  }
                  className={inputCls}
                >
                  <option value="top-left">
                    {t('dashboard.settings.heatmapLegendPositionTopLeft')}
                  </option>
                  <option value="top-right">
                    {t('dashboard.settings.heatmapLegendPositionTopRight')}
                  </option>
                  <option value="bottom-left">
                    {t('dashboard.settings.heatmapLegendPositionBottomLeft')}
                  </option>
                  <option value="bottom-right">
                    {t('dashboard.settings.heatmapLegendPositionBottomRight')}
                  </option>
                </select>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-2">
              {/* 크기(S/M/L). */}
              <div>
                <label className="mb-1 block text-[11px] text-(--color-text-muted)">
                  {t('dashboard.settings.heatmapLegendSize')}
                </label>
                <select
                  value={legendCfg?.size ?? 'md'}
                  data-testid="heatmap-legend-size"
                  onChange={(e) => setLegend({ size: e.target.value as LegendConfig['size'] })}
                  className={inputCls}
                >
                  <option value="sm">{t('dashboard.settings.heatmapLegendSizeSm')}</option>
                  <option value="md">{t('dashboard.settings.heatmapLegendSizeMd')}</option>
                  <option value="lg">{t('dashboard.settings.heatmapLegendSizeLg')}</option>
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
          className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
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
          className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
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
          className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
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
          className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
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
          className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
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
              className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
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
                  className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
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

/** 시스템 통계 패널 악센트 그룹 — 값은 i18n 키 */
const MONITOR_STATS_ACCENT_LABEL_KEYS: Record<string, string> = {
  _base: 'dashboard.settings.accent.base',
  header: 'dashboard.settings.accent.header',
  label: 'dashboard.settings.accent.statLabel',
  value: 'dashboard.settings.accent.statValue',
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
/**
 * 미리보기에서 **디바운스 없이** 즉시 반영할 config 키.
 *
 * 조건은 하나다 — 데이터 조회에 관여하지 않고 그리기만 바꾸는 값인가. 현재값의 크기와
 * 자리가 그렇다. 끌어서 옮기는 조작은 손이 움직이는 동안 그림이 따라와야 어디에 놓일지
 * 보이므로, 여기에 없으면 드래그가 200ms 계단으로 끊긴다.
 */
const PREVIEW_LIVE_KEYS = [
  'value_scale',
  'value_offset_x',
  'value_offset_y',
  'gauge_size',
  'gauge_offset_x',
  'gauge_offset_y',
  'threshold_legend_offset_x',
  'threshold_legend_offset_y',
] as const;

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
            className="h-3.5 w-3.5 rounded border-gray-300"
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
            config.gauge_offset_x ||
            config.gauge_offset_y) ? (
            <button
              type="button"
              onClick={() =>
                onConfigChange({
                  gauge_size: undefined,
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
              onConfigChange({ value_scale: undefined, value_offset_x: undefined, value_offset_y: undefined })
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
                className="h-4 w-4 rounded-sm border border-gray-200 dark:border-gray-600"
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
                className="h-4 w-4 rounded-sm border border-gray-200 dark:border-gray-600"
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
        channels,
        channelName,
        globalSmooth,
        strokeDasharray: STROKE_DASHARRAY,
        palette: PREVIEW_FALLBACK_PALETTE,
        sampleName: t('dashboard.settings.preview.sample'),
        channelFallbackName: (i) =>
          t('dashboard.settings.preview.channelFallback').replace('{index}', String(i)),
      }),
    [config.data_source, config.tsdb_source, storeSource, channels, channelName, globalSmooth, t],
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
          {isStoreMode
            ? t('dashboard.settings.preview.seriesCount').replace('{count}', String(storeSeriesCount))
            : isMultiMode
              ? t('dashboard.settings.preview.channelCount').replace('{count}', String(channels.length))
              : channelName || t('dashboard.settings.preview.channelUnset')}
        </span>
      </div>
      <div
        className={cn(
          'flex min-h-0 flex-1',
          isLegendVert ? 'flex-row' : 'flex-col',
          legendPos === 'left' ? 'flex-row-reverse' : '',
        )}
      >
        <div className="min-h-0 min-w-0 flex-1">
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
        {/* 범례 — 실제 패널과 같은 컴포넌트/배치. recharts 내장 Legend 를 쓰면 구분선·여백과
            범례 옵션(이름/선/마지막 값)이 실제 렌더와 달라진다. */}
        <ChartLegend
          seriesKeys={series.map((s) => s.key)}
          seriesColors={series.map((s) => s.color)}
          isMultiMode={isMultiMode}
          legendCfg={legendCfg}
          chartData={data}
          formatValue={(_key, v) => (enumMode ? formatEnumValue(v, enumMap) : v.toFixed(1))}
        />
      </div>
    </div>
  );
}

/**
 * 미리보기 위에 "실제 대시보드에서 이 패널이 차지할 영역"을 점선 사각형으로 겹쳐 보여준다.
 *
 * 왜 필요한가: 채움(fill) 모드는 미리보기 영역을 가로·세로 모두 채우므로 대시보드에서의
 * 실제 종횡비와 다르다. 히트맵은 도면 종횡비로 스테이지를 레터박스하므로(stage.ts), 실제
 * 비율을 모르면 대시보드에서 도면이 어디까지 보이고 여백이 얼마나 생길지 확인할 방법이 없다.
 * 맞춤(fit) 모드는 미리보기 자체가 실제 비율이므로 이 오버레이를 그리지 않는다.
 *
 * 순수 표시용이다 — 포인터 이벤트를 받지 않아 마커 드래그 배치를 방해하지 않는다.
 */
function PanelAreaOutline({ aspect }: { aspect: number }): React.ReactElement {
  const { t } = useTranslation();
  return (
    <div
      data-testid="panel-area-outline"
      aria-hidden="true"
      className="pointer-events-none absolute inset-0 flex items-center justify-center"
    >
      <div
        className="relative max-h-full max-w-full border border-dashed border-blue-400/70"
        style={{ aspectRatio: `${aspect} / 1`, width: '100%', height: '100%' }}
      >
        <span className="absolute right-0 top-0 bg-blue-400/80 px-1 py-px text-[9px] leading-tight text-white">
          {t('dashboard.settings.preview.panelArea')}
        </span>
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
      {/* 필드 카드 - 각 카드가 독립 악센트 그룹 */}
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

/**
 * 시스템 통계 패널 미니 프리뷰.
 *
 * 실제 패널과 같은 규칙으로 그린다 — 열 개수 상한을 반영하고, 색 영역은 패널이
 * 실제로 소비하는 그룹(_base / header / label / value)만 노출한다. 패널이 쓰지 않는
 * 그룹을 프리뷰에 두면 색을 골라도 아무 일이 없는 빈 약속이 된다.
 */
/**
 * sysmetrics 패널 미니 프리뷰.
 *
 * 세 패널이 한 컴포넌트를 쓴다. 셋의 차이는 "무엇을 몇 칸으로 그리는가" 뿐이고,
 * 색 영역(_base / header / label / value)은 같기 때문이다. 패널마다 따로 만들면
 * 색 영역 규약이 조금씩 갈라진다.
 *
 * 실제 데이터를 부르지 않는다 — 설정 화면의 미리보기는 배치와 색을 보는 자리이고,
 * 여기서 폴링을 걸면 설정을 여는 것만으로 요청이 늘어난다.
 */
function SysMetricsMiniPreview({
  panel,
  selectedGroup,
  onSelectGroup,
  effectiveColor,
  panelColor,
}: {
  panel: PanelConfig;
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

  const config = panel.config as Record<string, unknown> | undefined;
  const panelOptions = readPanelOptions(config, panel.type);

  // 그릴 칸 목록. 패널 유형마다 축이 다르다 — 시스템은 항목, 네트워크는 채널,
  // 스토리지는 마운트(고르지 않았으면 합계 한 줄)다.
  const cells: { key: string; label: string; kind: SysMetricValueKind }[] = (() => {
    const targets = SYSMETRICS_STYLE_TARGETS[panel.type];
    if (targets) {
      // 시스템 패널은 옛 그룹 키를 값 키로 옮겨 대조한다. 옮기지 않으면 저장된
      // 대시보드(그룹 키)와 값 카탈로그의 교집합이 비어 미리보기가 빈 채로 나온다.
      const items =
        panel.type === 'sysmetrics-system'
          ? normalizeSystemItems(config?.items)
          : Array.isArray(config?.items)
            ? (config.items as string[])
            : undefined;
      const shown = items ? targets.filter((x) => items.includes(x.key)) : targets;
      return shown.map((x) => ({ key: x.key, label: t(x.labelKey), kind: x.kind }));
    }
    const mounts = Array.isArray(config?.mountpoints) ? (config.mountpoints as string[]) : [];
    if (mounts.length > 0) {
      return mounts.map((m) => ({ key: m, label: m, kind: 'ratio' as SysMetricValueKind }));
    }
    return [{ key: 'total', label: t('sysmetrics.storage.total'), kind: 'ratio' as SysMetricValueKind }];
  })();

  const cols = readMaxCols(config, 2, cells.length);

  return (
    <div className="h-full w-full overflow-hidden rounded-xl border border-(--color-border-default) text-xs">
      <div className={zone('_base', 'flex items-center gap-1.5 rounded-t-xl px-3 py-1.5')} onClick={() => onSelectGroup('_base')}>
        {tag('_base', t('dashboard.settings.preview.tagBase'))}
        <span className="h-2.5 w-2.5 rounded-full border border-gray-300 dark:border-gray-600"
          style={panelColor ? { backgroundColor: panelColor } : { background: 'conic-gradient(from 0deg, #ef4444, #f59e0b, #10b981, #3b82f6, #8b5cf6, #ef4444)' }} />
        <span className="text-[10px] text-(--color-text-muted)">{panelColor ?? t('dashboard.settings.accent.default')}</span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      <div className={zone('header', 'px-3 py-2')} onClick={() => onSelectGroup('header')}>
        {tag('header', t('dashboard.settings.preview.tagHeader'))}
        <span className="text-sm font-semibold text-(--color-text-primary)" style={effectiveColor('header') ? { color: effectiveColor('header')! } : undefined}>
          {panel.title || t(`dashboard.panelTypes.${panel.type === 'sysmetrics-system' ? 'sysmetricsSystem' : panel.type === 'sysmetrics-network' ? 'sysmetricsNetwork' : 'sysmetricsStorage'}`)}
        </span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {cells.length === 0 ? (
        <p className="p-4 text-center text-[10px] text-(--color-text-muted)">{t('monitoring.emptyPanel')}</p>
      ) : (
        <div className="grid gap-2 p-3" style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }} data-testid="sysmetrics-preview-grid" data-cols={cols}>
          {cells.slice(0, 6).map((cell) => {
            const style = resolveItemOptions(config, cell.key, cell.kind, panel.type).style;
            return (
              <div key={cell.key} data-testid={`sysmetrics-preview-${cell.key}`} data-style={style} className="rounded-md border border-(--color-border-default) p-2">
                <span className={zone('label', 'block truncate text-[9px]')} onClick={(e) => { e.stopPropagation(); onSelectGroup('label'); }}
                  style={effectiveColor('label') ? { color: effectiveColor('label')! } : undefined}>
                  {cell.label}
                </span>
                <div className={zone('value', 'mt-1')} onClick={(e) => { e.stopPropagation(); onSelectGroup('value'); }}>
                  {tag('value', t('dashboard.settings.preview.tagValue'))}
                  <SysMetricsPreviewShape style={style} color={effectiveColor('value')} />
                </div>
              </div>
            );
          })}
        </div>
      )}
      {panelOptions.legend !== 'none' && (
        <p className="px-3 pb-2 text-[8px] text-(--color-text-muted)">
          {t('sysmetrics.settings.legend')}: {t(`sysmetrics.legend.${panelOptions.legend}`)}
        </p>
      )}
    </div>
  );
}

/** 미리보기 칸의 모양 — 스타일마다 다른 형태를 작게 흉내 낸다. */
function SysMetricsPreviewShape({ style, color }: { style: SysMetricsStyle; color?: string }) {
  const stroke = color ?? '#0ea5e9';

  if (style === 'tile') {
    return <span className="block text-sm font-semibold text-(--color-text-primary)" style={color ? { color } : undefined}>42%</span>;
  }
  if (style === 'progress') {
    return (
      <div className="h-1.5 w-full overflow-hidden rounded bg-(--color-bg-sunken)">
        <div className="h-full rounded" style={{ width: '62%', backgroundColor: stroke }} />
      </div>
    );
  }
  if (style === 'gauge') {
    return (
      <svg viewBox="0 0 40 40" className="h-8 w-full">
        <circle cx={20} cy={20} r={14} fill="none" stroke="#e2e8f0" strokeWidth={6} />
        <circle cx={20} cy={20} r={14} fill="none" stroke={stroke} strokeWidth={6}
          strokeDasharray={`${2 * Math.PI * 14 * 0.62} ${2 * Math.PI * 14}`}
          transform="rotate(-90 20 20)" />
      </svg>
    );
  }
  const points = [3, 7, 5, 9, 6, 8, 5, 7, 4, 8];
  if (style === 'bar') {
    return (
      <svg viewBox="0 0 100 28" preserveAspectRatio="none" className="h-7 w-full">
        {points.map((v, i) => (
          <rect key={i} x={i * 10 + 1} y={28 - v * 2.6} width={8} height={v * 2.6} fill={stroke} />
        ))}
      </svg>
    );
  }
  const line = points.map((v, i) => `${(i / 9) * 100},${28 - v * 2.6}`).join(' ');
  return (
    <svg viewBox="0 0 100 28" preserveAspectRatio="none" className="h-7 w-full">
      {style === 'area' && <polygon points={`0,28 ${line} 100,28`} fill={stroke} opacity={0.25} />}
      <polyline points={line} fill="none" stroke={stroke} strokeWidth={2} vectorEffect="non-scaling-stroke" />
    </svg>
  );
}

function MonitorStatsMiniPreview({
  panel,
  selectedGroup,
  onSelectGroup,
  effectiveColor,
  panelColor,
}: {
  panel: PanelConfig;
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

  // 프리뷰도 실제 패널과 같은 열 상한을 따른다(좁을 때 자동 축소는 실제 폭에서만 일어난다).
  const allItems = readPanelItems('stats', panel.config);
  const cols = readMaxCols(panel.config, 3, allItems.length);
  const items = allItems.slice(0, 6);
  const sample: Record<string, string> = {
    cpuUsage: '0.0%', memoryUsage: '62.8%', goRoutines: '128', heapAlloc: '12.3 MB',
    memSys: '64.0 MB', uptime: '1h 2m', totalFlows: '12', runningFlows: '5',
    logsReceived: '1,204', eventsReceived: '37', wsState: 'CONNECTED',
  };

  return (
    <div className="h-full w-full overflow-hidden rounded-xl border border-(--color-border-default) text-xs">
      <div className={zone('_base', 'flex items-center gap-1.5 rounded-t-xl px-3 py-1.5')} onClick={() => onSelectGroup('_base')}>
        {tag('_base', t('dashboard.settings.preview.tagBase'))}
        <span className="h-2.5 w-2.5 rounded-full border border-gray-300 dark:border-gray-600"
          style={panelColor ? { backgroundColor: panelColor } : { background: 'conic-gradient(from 0deg, #ef4444, #f59e0b, #10b981, #3b82f6, #8b5cf6, #ef4444)' }} />
        <span className="text-[10px] text-(--color-text-muted)">{panelColor ?? t('dashboard.settings.accent.default')}</span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      <div className={zone('header', 'px-3 py-2')} onClick={() => onSelectGroup('header')}>
        {tag('header', t('dashboard.settings.preview.tagHeader'))}
        <span className="text-sm font-semibold text-(--color-text-primary)" style={effectiveColor('header') ? { color: effectiveColor('header')! } : undefined}>
          {panel.title || t('dashboard.panelTypes.monitorStats')}
        </span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {items.length === 0 ? (
        <p className="p-4 text-center text-[10px] text-(--color-text-muted)">{t('monitoring.emptyPanel')}</p>
      ) : (
        <div className="grid gap-2 p-3" style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}>
          {items.map((key) => (
            <div key={key} className="rounded-md border border-(--color-border-default) p-2">
              <div className={zone('label', 'rounded px-1')} onClick={() => onSelectGroup('label')}>
                {tag('label', t('dashboard.settings.accent.statLabel'))}
                <span className="block truncate text-[9px] text-(--color-text-muted)" style={effectiveColor('label') ? { color: effectiveColor('label')! } : undefined}>
                  {t(findItemMeta('stats', key)?.labelKey ?? key)}
                </span>
              </div>
              <div className={zone('value', 'mt-0.5 rounded px-1')} onClick={() => onSelectGroup('value')}>
                {tag('value', t('dashboard.settings.accent.statValue'))}
                <span className="block truncate text-sm font-bold text-(--color-text-primary)" style={effectiveColor('value') ? { color: effectiveColor('value')! } : undefined}>
                  {sample[key] ?? '-'}
                </span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

/**
 * 실시간 메트릭 패널 미니 프리뷰.
 *
 * 채널별 색 영역은 리소스 패널과 같은 그룹 이름(cpu/memory/throughput/errorRate)을 쓴다 —
 * 실제 패널이 `accentColor(channel)` 로 선 색을 읽으므로 프리뷰에서 고른 색이 그대로 반영된다.
 */
function MonitorMetricsMiniPreview({
  panel,
  selectedGroup,
  onSelectGroup,
  effectiveColor,
  panelColor,
}: {
  panel: PanelConfig;
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

  const items = readPanelItems('metrics', panel.config);
  const cols = readMaxCols(panel.config, 2, items.length);
  const DEFAULT_COLORS: Record<string, string> = {
    cpu: '#3b82f6', memory: '#8b5cf6', throughput: '#10b981', errorRate: '#ef4444',
  };
  // 선 모양만 흉내 내는 고정 표본 — 실데이터를 끌어오면 설정 화면이 스트림에 묶인다.
  const SPARK = [4, 7, 5, 9, 6, 8, 5, 7, 4, 8];

  return (
    <div className="h-full w-full overflow-hidden rounded-xl border border-(--color-border-default) text-xs">
      <div className={zone('_base', 'flex items-center gap-1.5 rounded-t-xl px-3 py-1.5')} onClick={() => onSelectGroup('_base')}>
        {tag('_base', t('dashboard.settings.preview.tagBase'))}
        <span className="h-2.5 w-2.5 rounded-full border border-gray-300 dark:border-gray-600"
          style={panelColor ? { backgroundColor: panelColor } : { background: 'conic-gradient(from 0deg, #ef4444, #f59e0b, #10b981, #3b82f6, #8b5cf6, #ef4444)' }} />
        <span className="text-[10px] text-(--color-text-muted)">{panelColor ?? t('dashboard.settings.accent.default')}</span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      <div className={zone('header', 'px-3 py-2')} onClick={() => onSelectGroup('header')}>
        {tag('header', t('dashboard.settings.preview.tagHeader'))}
        <span className="text-sm font-semibold text-(--color-text-primary)" style={effectiveColor('header') ? { color: effectiveColor('header')! } : undefined}>
          {panel.title || t('dashboard.panelTypes.monitorMetrics')}
        </span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {items.length === 0 ? (
        <p className="p-4 text-center text-[10px] text-(--color-text-muted)">{t('monitoring.emptyPanel')}</p>
      ) : (
        <div className="grid gap-2 p-3" style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}>
          {items.map((key) => {
            const lineColor = effectiveColor(key) ?? DEFAULT_COLORS[key] ?? '#3b82f6';
            return (
              <div key={key} className={zone(key, 'rounded-md border border-(--color-border-default) p-2')} onClick={() => onSelectGroup(key)}>
                {tag(key, key)}
                <span className="block truncate text-[9px] text-(--color-text-muted)">
                  {t(findItemMeta('metrics', key)?.labelKey ?? key)}
                </span>
                {/* 실제 패널은 라인 차트다. 프리뷰가 막대면 색만 맞고 모양이 달라
                    "미리보기와 출력이 다르다"는 어긋남이 생긴다. */}
                <svg viewBox="0 0 100 28" preserveAspectRatio="none" className="mt-1 h-7 w-full">
                  <polyline
                    points={SPARK.map((v, i) => `${(i / (SPARK.length - 1)) * 100},${28 - v * 2.6}`).join(' ')}
                    fill="none"
                    stroke={lineColor}
                    strokeWidth={2}
                    vectorEffect="non-scaling-stroke"
                  />
                </svg>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

/**
 * 네트워크 패널 미니 프리뷰.
 *
 * 채널마다 차트 하나, 선택한 인터페이스마다 선 하나 — 실제 패널과 같은 구조로 그린다.
 */
function MonitorNetworkMiniPreview({
  panel,
  selectedGroup,
  onSelectGroup,
  effectiveColor,
  panelColor,
}: {
  panel: PanelConfig;
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

  const items = readPanelItems('network', panel.config);
  const cols = readMaxCols(panel.config, 2, items.length);
  const chosen = readInterfaces(panel.config);
  const lines = chosen.length > 0 ? chosen : ['total'];
  const colors = interfaceColors(lines);
  const SHAPES = [
    [3, 7, 5, 9, 6, 8, 5, 7, 4, 8],
    [6, 4, 8, 5, 9, 6, 7, 5, 8, 6],
  ];

  return (
    <div className="h-full w-full overflow-hidden rounded-xl border border-(--color-border-default) text-xs">
      <div className={zone('_base', 'flex items-center gap-1.5 rounded-t-xl px-3 py-1.5')} onClick={() => onSelectGroup('_base')}>
        {tag('_base', t('dashboard.settings.preview.tagBase'))}
        <span className="h-2.5 w-2.5 rounded-full border border-gray-300 dark:border-gray-600"
          style={panelColor ? { backgroundColor: panelColor } : { background: 'conic-gradient(from 0deg, #ef4444, #f59e0b, #10b981, #3b82f6, #8b5cf6, #ef4444)' }} />
        <span className="text-[10px] text-(--color-text-muted)">{panelColor ?? t('dashboard.settings.accent.default')}</span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      <div className={zone('header', 'px-3 py-2')} onClick={() => onSelectGroup('header')}>
        {tag('header', t('dashboard.settings.preview.tagHeader'))}
        <span className="text-sm font-semibold text-(--color-text-primary)" style={effectiveColor('header') ? { color: effectiveColor('header')! } : undefined}>
          {panel.title || t('dashboard.panelTypes.monitorNetwork')}
        </span>
      </div>
      <div className="border-t border-gray-100 dark:border-gray-700" />
      {items.length === 0 ? (
        <p className="p-4 text-center text-[10px] text-(--color-text-muted)">{t('monitoring.emptyPanel')}</p>
      ) : (
        <div className="grid gap-2 p-3" style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}>
          {items.slice(0, 4).map((key) => (
            <div key={key} className="rounded-md border border-(--color-border-default) p-2">
              <span className="block truncate text-[9px] text-(--color-text-muted)">
                {t(findItemMeta('network', key)?.labelKey ?? key)}
              </span>
              <svg viewBox="0 0 100 28" preserveAspectRatio="none" className="mt-1 h-7 w-full">
                {lines.map((name, li) => (
                  <polyline
                    key={name}
                    points={SHAPES[li % SHAPES.length]!.map((v, i) => `${(i / 9) * 100},${28 - v * 2.6}`).join(' ')}
                    fill="none"
                    stroke={colors[name]}
                    strokeWidth={2}
                    vectorEffect="non-scaling-stroke"
                  />
                ))}
              </svg>
              {lines.length > 1 && (
                <div className="mt-1 flex flex-wrap gap-1">
                  {lines.map((name) => (
                    <span key={name} className="flex items-center gap-0.5 text-[8px] text-(--color-text-muted)">
                      <span className="inline-block h-1.5 w-1.5 rounded-full" style={{ backgroundColor: colors[name] }} />
                      {name}
                    </span>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
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
