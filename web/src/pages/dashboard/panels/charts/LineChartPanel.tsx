// Line Chart 패널 (REQ-M4-05).
// x축=timestamp, y축=display_field.
// multi_series_field 가 지정되면 label 값별로 line 을 분리한다.

import { Fragment, useCallback, useEffect, useMemo, useState } from 'react';
import { Download, Pause, Play, TrendingUp } from 'lucide-react';
import {
  CartesianGrid,
  Line,
  Area,
  Bar,
  ComposedChart,
  ReferenceArea,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { ChartTooltipContent } from './ChartTooltipContent';
import { SingleSeriesTooltipContent } from './SingleSeriesTooltipContent';
import { resolveXAxisHeight, resolveYAxisWidth, yTickSampleValues } from './axisSize';
import { resolveFillBand } from './thresholdFill';
import { CandleShape } from './CandleShape';
import { candleRows } from './candle';
import { useCandleSeriesData } from './useCandleSeriesData';
import {
  effectiveStacked,
  hasGapDash,
  readGraphStyle,
  resolveSeriesStyle,
  type GraphStyle,
} from './graphStyle';
import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

import {
  buildEnumLabelMap,
  formatEnumValue,
  DEFAULT_CHART_LEGEND_FONT_SIZE,
  resolveAxisFont,
  SERIES_PALETTE,
  STROKE_DASHARRAY,
  THRESHOLD_DEFAULT_COLORS,
  type AxisFontStyle,
  type ChannelRefConfig,
  type LegendConfig,
  type LineChartPanelConfig,
  type StoreSourceConfig,
  type TimeWindowMode,
  type YAxisDataType,
  type YEnumLabel,
  type YThreshold,
  type TooltipConfig,
  type YAxisMode,
} from './chartChannelTypes';
import { ChartLegend } from './ChartLegend';
import { ConnectionStatusIcon } from './ConnectionStatusIcon';
import {
  computeNiceTimeTicks,
  formatTimeShort,
  formatTimestamp,
} from './chartChannelUtils';
import { chartDataToCsv, downloadCsv } from './csvExport';
import {
  buildGapOverlay,
  GAP_DASHARRAY,
  gapDotSeriesKey,
  gapSeriesKey,
} from './gapDash';
import {
  isPanelSeriesActive,
  panelSourceWindowMs,
  resolvePanelSourceBinding,
} from './panelDataSource';
import {
  formatDecimal,
  hasExplicitDecimalPlaces,
  readDecimalPlaces,
} from './decimalPlaces';
import { axisUnitLabel, formatTickValue, isAutoScaledUnit } from './unitOptions';
import { resolveGroupPageDisplay, resolvePanelSeriesDisplay } from './panelSeriesStatus';
import {
  readChartXRange,
  type SeriesRange,
} from './seriesRange';
import { usePanelSeriesData } from './usePanelSeriesData';
import { usePanelTitleStyle, usePanelTitleVisible } from '../../panelChromeContext';
import { usePanelEditMode } from '../PanelEditToggle';
import { ChartDragLayer } from '../../ChartDragLayer';
import { PanelEditGrid } from '../../PanelEditGrid';
import { PanelAlignToolbar } from '../../PanelAlignToolbar';
import { usePanelElementEdit } from '../../usePanelElementEdit';
import {
  PanelResizeHandle,
  PANEL_EDIT_OUTLINE_CLASS,
  PANEL_SELECTED_OUTLINE_CLASS,
} from '../../PanelDragLayer';
import { LEGEND_OFFSET_SAFETY_LIMIT, clampStoredLegendOffset } from './legendOverlay';
import {
  panelBoxTransform,
  PANEL_OFFSET_LIMIT,
  PANEL_SIZE_MAX,
  PANEL_SIZE_MIN,
  readPanelOffset,
  readPanelSize,
} from './panelGeometry';
import { resolveFontSize } from './textStyle';

interface LineChartPanelProps {
  panelId: string;
  title?: string;
  config: Record<string, unknown>;
  /**
   * 배치 편집으로 바뀐 값을 쓸 콜백. 없으면 편집 모드에 들어갈 수 없다 —
   * 범례를 끌어도 저장할 곳이 없기 때문이다(파이 패널과 같은 규칙).
   */
  onConfigChange?: (config: Record<string, unknown>) => void;
  /** 설정 미리보기처럼 **항상** 편집인 자리인가(토글을 감춘다). */
  forceEdit?: boolean;
}

const DEFAULT_MAX_POINTS = 100;

// 버퍼 크기는 X축 범위가 정한다(`chartXRangePoints`) — 갯수 방식이면 그 값,
// 최근 기간이면 1Hz 가정으로 기간의 2배. 종전 `resolveMaxPoints` 규약을 그대로
// 옮겼으므로 저장된 패널의 버퍼 크기는 변하지 않는다.

const DEFAULT_REFRESH_MS = 1000;
const MIN_REFRESH_MS = 200;
const MAX_REFRESH_MS = 60_000;
const DEFAULT_Y_PAD_PCT = 5;
const MAX_Y_PAD_PCT = 50;

/** multi-series 색상 팔레트 */
// 시리즈 자동 색상 팔레트(설정 편집기와 공유). @see chartChannelTypes.SERIES_PALETTE
const SERIES_COLORS = SERIES_PALETTE;

function parseConfig(config: Record<string, unknown>): LineChartPanelConfig {
  return {
    channel_name: (config.channel_name as string) ?? '',
    channels: config.channels as ChannelRefConfig[] | undefined,
    display_field: (config.display_field as string) ?? 'value',
    max_points: (config.max_points as number) ?? DEFAULT_MAX_POINTS,
    x_label: config.x_label as string | undefined,
    x_label_font: config.x_label_font as AxisFontStyle | undefined,
    x_tick_font: config.x_tick_font as AxisFontStyle | undefined,
    y_label_font: config.y_label_font as AxisFontStyle | undefined,
    y_tick_font: config.y_tick_font as AxisFontStyle | undefined,
    y_axis_type: config.y_axis_type as YAxisDataType | undefined,
    y_enum_labels: config.y_enum_labels as YEnumLabel[] | undefined,
    y_min: config.y_min as number | undefined,
    y_max: config.y_max as number | undefined,
    y_axis_mode: config.y_axis_mode as YAxisMode | undefined,
    y_axis_padding_pct: config.y_axis_padding_pct as number | undefined,
    y_label: config.y_label as string | undefined,
    y_unit: config.y_unit as string | undefined,
    y_thresholds: config.y_thresholds as YThreshold[] | undefined,
    graph_style: config.graph_style as GraphStyle | undefined,
    stacked: config.stacked as boolean | undefined,
    decimal_places: config.decimal_places as number | undefined,
    tooltip: config.tooltip as TooltipConfig | undefined,
    x_range: config.x_range as SeriesRange | undefined,
    time_window_mode: config.time_window_mode as TimeWindowMode | undefined,
    recent_window_sec: config.recent_window_sec as number | undefined,
    fixed_start_ms: config.fixed_start_ms as number | undefined,
    fixed_end_ms: config.fixed_end_ms as number | undefined,
    time_window_refresh_ms: config.time_window_refresh_ms as number | undefined,
    legend: config.legend as LegendConfig | undefined,
    gap_dash_threshold: config.gap_dash_threshold as number | undefined,
    smooth: (config.smooth as boolean) ?? false,
    multi_series_field: config.multi_series_field as string | undefined,
  };
}


/**
 * 차트의 최신 timestamp 행에서 critical 임계 초과 여부 판정.
 * 다중 시리즈 시 어느 한 시리즈라도 critical 임계 위면 true.
 */
function isCriticalBreached(
  rows: Array<Record<string, unknown>>,
  seriesKeys: string[],
  thresholds: YThreshold[] | undefined,
): boolean {
  if (!thresholds || thresholds.length === 0 || rows.length === 0) return false;
  const criticals = thresholds.filter((t) => t.severity === 'critical');
  if (criticals.length === 0) return false;
  const last = rows[rows.length - 1]!;
  for (const key of seriesKeys) {
    const v = last[key];
    if (typeof v !== 'number' || !Number.isFinite(v)) continue;
    for (const t of criticals) {
      if (v >= t.value) return true;
    }
  }
  return false;
}

function clamp(n: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(hi, n));
}




/** 공용 편집 표면이 다루는 요소. 라인은 플롯 상자와 범례 둘이다. */
type LineElementKind = 'plot' | 'legend';
const LINE_ELEMENT_KINDS: readonly LineElementKind[] = ['plot', 'legend'];

export default function LineChartPanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange,
  forceEdit,
}: LineChartPanelProps) {
  const showTitle = usePanelTitleVisible();
  const titleStyle = usePanelTitleStyle();
  // 범례·그림 상자 끌어 옮기기 — 파이 패널과 같은 두 겹 게이팅(콜백 유무 + 편집 모드).
  const edit = usePanelEditMode({
    canEdit: !!onConfigChange,
    forced: forceEdit,
    testId: 'line-chart-edit-toggle',
    below: showTitle,
  });
  const { t } = useTranslation();
  const cfg = parseConfig(config);
  // 시간창 표시(brush 범위)에만 쓰는 원본 store 블록. 조회 자체는 usePanelSeriesData 가 한다.
  const storeSource = config.store_source as StoreSourceConfig | undefined;
  // SPEC-TSDB-002 §2.3 [U3]: 소스 판정은 `panelDataSource` 계약이 소유한다. 패널은
  // `data_source` 를 직접 비교하지 않는다 — 소스 종류가 늘어도 이 지점이 종류만큼
  // 곱해지지 않게 하기 위함이다(UB1-1).
  // `isStore` 는 "채널이 아닌 시리즈 소스가 활성인가" 를 뜻한다. store 와 tsdb 가 둘 다
  // 이 조건을 만족하며, 이름만 store 시절의 것이 남아 있다(호출부 무변경의 대가).
  // 표시 문구(비어 있음 / 조회 중)는 **종류**를 알아야 하므로 단일 축 해석기를 그대로 쓴다.
  const sourceBinding = resolvePanelSourceBinding(config);
  // 활성 판정은 소스 **목록 전체**를 본다 — 단일 축 해석기로 판정하면 목록 쪽 인스턴스에만
  // 시리즈가 있는 패널이 비활성으로 보인다.
  const isStore = isPanelSeriesActive(config);
  // X축 고정 창 — 활성 소스의 블록에서 읽는다(store / tsdb / sysmetrics 공통 필드).
  const sourceWindowMs = panelSourceWindowMs(config);
  // X축 범위(구간 · 최근 · 포인트)를 한 곳에서 읽는다. 구 `time_window_mode` 계열은
  // `readChartXRange` 안에서 폴백으로 해석되므로, 여기부터는 새 어휘만 쓴다.
  const xRange = useMemo(
    () => readChartXRange(cfg),
    // parseConfig 가 매 렌더 새 객체를 만들므로 원시 필드로 memo 한다.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [
      cfg.x_range,
      cfg.time_window_mode,
      cfg.max_points,
      cfg.recent_window_sec,
      cfg.fixed_start_ms,
      cfg.fixed_end_ms,
    ],
  );
  /**
   * 시리즈축 페이지 커서(SPEC-TSDB-004 §2.7).
   *
   * config 가 아니라 패널 로컬 상태다 — 페이지를 넘길 때마다 대시보드 config 가
   * 저장되면 안 된다. 커서는 패널당 하나이며 모든 group by 항목에 같이 적용된다.
   */
  const [groupPageIndex, setGroupPageIndex] = useState(0);
  const storeResult = usePanelSeriesData(config, {
    tsdbOptions: { groupPage: groupPageIndex },
  });
  const onGroupPageChange = useCallback((p: number) => setGroupPageIndex(p), []);

  const status = storeResult.status;
  const closedReason = storeResult.closedReason;
  const errorReason = storeResult.errorReason;

  // SPEC-TSDB-002 §2.14 [S2]: 네 상태(빈 선택 / 빈 결과 / 부분 실패 / 전체 실패)를 서로
  // 구분해 표시한다. 판정은 `panelSeriesStatus` 가 소유하며 패널은 그리기만 한다.
  const seriesDisplay = resolvePanelSeriesDisplay(sourceBinding, storeResult);
  // SPEC-TSDB-004 §2.7: 시리즈축 페이지네이션 상태. group by 항목이 없으면
  // show=false 이므로 기존 패널의 렌더는 한 줄도 바뀌지 않는다.
  const groupPage = resolveGroupPageDisplay(storeResult.groups);
  // 빈 선택 안내는 **되돌아갈 채널이 없을 때만** 띄운다. 채널이 설정된 패널은 시리즈
  // 소스가 비활성이어도 채널 데이터를 그대로 그리므로(하위 호환, §2.4), 그 위에 안내를
  // 얹으면 정상 렌더를 오류처럼 보이게 한다.
  const hasChannelFallback =
    (cfg.channel_name ?? '') !== '' || (cfg.channels?.length ?? 0) > 0;
  const showEmptySelection =
    seriesDisplay.state === 'empty-selection' && !hasChannelFallback;

  // 최근 범위에서만 X축 끝(now)이 전진한다 — 그 주기.
  const isRelativeRange = xRange.mode === 'relative';
  const refreshMs = clamp(
    cfg.time_window_refresh_ms ?? DEFAULT_REFRESH_MS,
    MIN_REFRESH_MS,
    MAX_REFRESH_MS,
  );

  // 'recent' 모드에서 현재 시각을 주기적으로 갱신
  const [now, setNow] = useState<number>(() => Date.now());

  // 일시정지: 클릭 시점의 chartData/seriesKeys/now 를 스냅샷으로 보관
  // (단일/다중 모드 공통: 최종 렌더 데이터 동결 방식)
  const [pauseSnapshot, setPauseSnapshot] = useState<
    {
      chartData: Array<Record<string, unknown>>;
      seriesKeys: string[];
      now: number;
    } | null
  >(null);
  const isPaused = pauseSnapshot !== null;

  // recent 모드 또는 store 모드(설정 윈도우로 X축 고정)에서 현재 시각을 주기 갱신해
  // X축 도메인 끝(now)이 계속 전진하도록 한다.
  useEffect(() => {
    if ((!isRelativeRange && !isStore) || isPaused) return;
    const id = window.setInterval(() => setNow(Date.now()), refreshMs);
    return () => window.clearInterval(id);
  }, [isRelativeRange, isStore, refreshMs, isPaused]);

  // raw 데이터 계산 (모드별 분기)
  const {
    chartData: rawChartData,
    seriesKeys: rawSeriesKeys,
    booleanKeys,
  } = useMemo(() => {
    if (isStore) {
      const rows = new Map<number, Record<string, unknown>>();
      const seen: string[] = [];
      for (const [name, seriesArr] of storeResult.seriesEntries) {
        if (!seen.includes(name)) seen.push(name);
        for (const e of seriesArr) {
          if (!rows.has(e.timestamp)) {
            rows.set(e.timestamp, { timestamp: e.timestamp });
          }
          // store 엔트리 value 는 number|null. null 은 connectNulls 로 이어진다.
          rows.get(e.timestamp)![name] =
            typeof e.value === 'number' ? e.value : null;
        }
      }
      const data = Array.from(rows.values()).sort(
        (a, b) => (a.timestamp as number) - (b.timestamp as number),
      );
      // store 값은 number|null 로 집계됨(boolean 은 storeChartValue 로 1/0 변환).
      // data_type='boolean' 시리즈는 storeResult.booleanSeries 로 true/false 표시한다.
      const boolKeys = new Set(
        seen.filter((name) => storeResult.booleanSeries.has(name)),
      );
      return { chartData: data, seriesKeys: seen, booleanKeys: boolKeys };
    }

    // 채널이 소스에서 빠진 뒤로 시리즈는 언제나 위 분기에서 나온다. 비활성 소스는
    // 빈 차트이며, 안내는 `seriesDisplay` 가 따로 띄운다.
    return { chartData: [], seriesKeys: [], booleanKeys: new Set<string>() };
    // 채널 경로가 사라지면서 이 파생이 읽는 것은 store 결과뿐이다 — 표시 필드·다중
    // 시리즈 필드·구간은 채널 payload 를 가르던 축이라 더 이상 여기에 들어오지 않는다.
  }, [isStore, storeResult.seriesEntries, storeResult.booleanSeries]);

  // 일시정지 시 스냅샷 사용
  const rawOrPaused = pauseSnapshot ? pauseSnapshot.chartData : rawChartData;
  const seriesKeys = pauseSnapshot ? pauseSnapshot.seriesKeys : rawSeriesKeys;
  const effectiveNow = pauseSnapshot ? pauseSnapshot.now : now;

  // 결측 구간 점선 덧그림 (SPEC-TSDB-004 §2.19).
  //
  // 임계가 없으면 `buildGapOverlay` 가 입력을 그대로 돌려주므로, 저장된
  // 대시보드는 계산도 그림도 종전과 같다.
  const gapThreshold = cfg.gap_dash_threshold ?? 0;
  // 버킷 간격 — **행이 없는 결측**을 찾는 데 쓴다. 빈 구간 처리가 기본값
  // (`채우지 않음`)이면 서버가 빈 버킷을 아예 보내지 않아 행이 통째로 없고,
  // null 을 세는 방식으로는 하나도 찾을 수 없다. 채널 모드는 버킷 개념이
  // 없으므로 0(행 인덱스 기준)으로 둔다.
  //
  // 소스 블록은 `parseConfig` 가 옮기지 않으므로(허용 목록 방식) **원본 config**
  // 에서 읽는다. `cfg` 에서 읽으면 타입은 통과하지만 값이 항상 undefined 라
  // 판정이 조용히 꺼진다 — 실제로 그렇게 한 번 놓쳤다.
  const gapIntervalMs =
    ((config.tsdb_source as { interval_ms?: number } | undefined)?.interval_ms ??
      storeSource?.interval_ms ??
      0) || 0;
  const { rows: chartData, gapKeys } = useMemo(
    () => buildGapOverlay(rawOrPaused, seriesKeys, gapThreshold, gapIntervalMs),
    [rawOrPaused, seriesKeys, gapThreshold, gapIntervalMs],
  );
  // 그래프 스타일 — 패널 기본값. 시리즈가 개별로 덮어쓴다.
  const panelGraphStyle = readGraphStyle(cfg.graph_style);
  const panelStacked = cfg.stacked === true;
  // 캔들이 하나라도 쓰이면 OHLC 를 따로 조회한다. 패널 기본이 캔들이거나,
  // 시리즈 하나라도 캔들로 덮어썼으면 참이다.
  // 시리즈별 덮어쓰기는 시리즈 소스(store · tsdb)에만 있다 — 채널 모드는 패널 값만 본다.
  const anyCandle =
    panelGraphStyle === 'candle' ||
    (isStore &&
      [...storeResult.seriesStyles.values()].some((st) => st.graph_style === 'candle'));
  const candleSeries = useCandleSeriesData(config, anyCandle);

  const gapKeySet = useMemo(() => new Set(gapKeys), [gapKeys]);

  /**
   * 캔들 축 값을 차트 행에 얹는다.
   *
   * 캔들은 버킷마다 네 값을 쓰므로 행에 축별 키를 더 싣는다(`__o__`·`__h__`…).
   * 원래 시리즈 키에는 몸통 범위 `[아래, 위]` 를 넣어 Bar 가 그 구간에 막대를
   * 세우고, 모양 함수가 꼬리를 마저 그린다. 캔들이 없으면 원본을 그대로 둔다.
   */
  const chartDataWithCandles = useMemo(() => {
    if (candleSeries.size === 0) return chartData;
    const byTs = new Map<number, Record<string, unknown>>();
    for (const row of chartData) {
      const ts = row.timestamp as number;
      byTs.set(ts, { ...row });
    }
    for (const [name, candles] of candleSeries) {
      for (const row of candleRows(name, candles)) {
        const ts = row.timestamp as number;
        const target = byTs.get(ts);
        if (target) Object.assign(target, row);
        else byTs.set(ts, row);
      }
    }
    return Array.from(byTs.values()).sort(
      (a, b) => (a.timestamp as number) - (b.timestamp as number),
    );
  }, [chartData, candleSeries]);

  // 렌더 중인 시리즈가 모두 boolean 이면 Y축/툴팁을 true/false 로 표시한다.
  // (혼합 시엔 숫자 축을 유지하되 boolean 시리즈 값만 툴팁에서 true/false 로 표기)
  const allBoolean =
    seriesKeys.length > 0 && seriesKeys.every((k) => booleanKeys.has(k));

  const togglePause = useCallback(() => {
    setPauseSnapshot((prev) =>
      prev
        ? null
        : {
            chartData: rawChartData as Array<Record<string, unknown>>,
            seriesKeys: rawSeriesKeys,
            now: Date.now(),
          },
    );
  }, [rawChartData, rawSeriesKeys]);

  const handleExportCsv = useCallback(
    (rows: Array<Record<string, unknown>>, keys: string[]) => {
      const csv = chartDataToCsv(
        rows.map((r) => r as { timestamp: number; [k: string]: unknown }),
        keys,
        // boolean 시리즈는 CSV 에도 true/false 로 내보낸다.
        booleanKeys,
      );
      // 종전에는 채널 이름을 파일 이름으로 썼다. 채널이 없어졌으므로 패널 제목을 쓴다.
      const baseName = (title ?? '').trim() || 'chart';
      const ts = new Date().toISOString().replace(/[:.]/g, '-');
      downloadCsv(csv, `${baseName}-${ts}.csv`);
    },
    [title, booleanKeys],
  );

  // X축 도메인
  // recent 모드: 항상 설정된 윈도우 크기(now - windowSec ~ now) 로 고정.
  // 데이터가 윈도우보다 짧으면 좌측에 빈 공간이 생기지만, X축 크기 자체는
  // 사용자가 설정한 시간 범위를 일관되게 유지한다 (스케일 안정성 우선).
  const xDomain = useMemo<[number | 'dataMin', number | 'dataMax']>(() => {
    if (xRange.mode === 'relative') {
      const w = xRange.window_ms ?? 0;
      if (w > 0) return [effectiveNow - w, effectiveNow];
    }
    if (xRange.mode === 'absolute') {
      // 한쪽만 지정해도 축은 그려야 한다 — 빈 쪽은 현재 시각으로 닫는다.
      const end = xRange.end_ms ?? Date.now();
      const start = xRange.start_ms ?? end;
      return [start, end];
    }
    // 시리즈 소스(기본): X축을 **활성 소스의** 시간 윈도우로 고정한다. 데이터가 윈도우보다
    // 짧아도 X축 범위는 설정값을 일관되게 유지한다(스케일 안정성 우선).
    //
    // 창을 어느 블록에서 읽을지는 계약이 정한다(`panelSourceWindowMs`). 종전에는
    // `store_source` 를 하드코딩해 store 가 아닌 소스에서는 축이 데이터 범위로 떨어졌고,
    // 라이브로 쌓는 sysmetrics 는 점이 0~1개인 동안 축이 한 점으로 접혀 선이 보이지 않았다.
    if (isStore && sourceWindowMs !== undefined) {
      return [effectiveNow - sourceWindowMs, effectiveNow];
    }
    return ['dataMin', 'dataMax'];
  }, [xRange, effectiveNow, isStore, sourceWindowMs]);

  // X축 도메인이 고정 수치 범위인지(recent/fixed/store 윈도우) — 데이터 오버플로 클립 여부 결정.
  const xDomainFixed =
    typeof xDomain[0] === 'number' && typeof xDomain[1] === 'number';

  // X축 nice ticks
  const xTicks = useMemo(() => {
    if (typeof xDomain[0] === 'number' && typeof xDomain[1] === 'number') {
      return computeNiceTimeTicks(xDomain[0], xDomain[1]);
    }
    if (chartData.length >= 2) {
      const first = chartData[0] as Record<string, unknown>;
      const last = chartData[chartData.length - 1] as Record<string, unknown>;
      const s = first.timestamp as number | undefined;
      const e = last.timestamp as number | undefined;
      if (typeof s === 'number' && typeof e === 'number' && e > s) {
        return computeNiceTimeTicks(s, e);
      }
    }
    return undefined;
  }, [xDomain, chartData]);

  /**
   * 숫자형 Y축 눈금 포맷터.
   *
   * 소수 자릿수도 단위도 없으면 `undefined` 를 돌려준다 — Recharts 기본 표기를
   * 그대로 두기 위해서다. 여기서 항상 포맷터를 주면 아무 설정도 안 한 패널의
   * 눈금 글자가 조용히 바뀐다.
   *
   * **자릿수의 기본값(2)은 축에 적용하지 않는다.** 눈금은 값 읽기가 아니라 눈금자이고
   * Recharts 가 이미 보기 좋은 수(0 · 25 · 50)를 고르는데, 거기에 기본값을 걸면 아무
   * 설정도 안 한 패널의 축이 `0.00 · 25.00 · 50.00` 이 된다. 사용자가 자릿수를 **직접
   * 지정하면** 그때는 축도 따라간다 — 축과 툴팁이 같은 값을 다른 자릿수로 보여 주지
   * 않아야 한다는 규약은 그대로다(`decimalPlaces.ts` 머리말).
   */
  const yNumberFormatter = useMemo(() => {
    const dec = hasExplicitDecimalPlaces(config) ? readDecimalPlaces(config) : undefined;
    const unit = cfg.y_unit;
    if (dec === undefined && !unit) return undefined;
    // 눈금은 눈금자다 — 단위를 붙이고 자릿수는 그 눈금의 크기가 정한다.
    // 사용자가 자릿수를 명시했으면(`dec`) 그 값이 이긴다.
    return (v: number): string => formatTickValue(v, unit, dec);
  }, [config, cfg.y_unit]);


  // 툴팁 — 미지정이 종전 동작(켬 + 전체 시리즈).
  const tooltipEnabled = cfg.tooltip?.enabled !== false;
  const tooltipSingle = cfg.tooltip?.single === true;

  // Y축 도메인
  const yAxisMode: YAxisMode = cfg.y_axis_mode ?? 'auto';
  const yPadPct = clamp(cfg.y_axis_padding_pct ?? DEFAULT_Y_PAD_PCT, 0, MAX_Y_PAD_PCT);

  // 열거형 Y축(패널 단위): y_axis_type==='enum' + 유효 매핑이 있으면 값→라벨로 표시한다.
  // enum 이 우선하며, 미설정 boolean 시리즈는 아래 boolAxis 자동 처리로 폴백한다.
  const enumMap = useMemo(
    () => buildEnumLabelMap(cfg.y_enum_labels),
    [cfg.y_enum_labels],
  );
  const enumMode = cfg.y_axis_type === 'enum' && enumMap.size > 0;
  // 눈금은 매핑된 값들을 오름차순으로 사용한다.
  const enumTicks = useMemo(
    () => (enumMode ? [...enumMap.keys()].sort((a, b) => a - b) : undefined),
    [enumMode, enumMap],
  );

  // boolean 시리즈: Y축을 [0,1] 두 눈금(false/true)으로 고정한다. boolean 값은 0/1 이
  // 유일한 의미이므로 수동 Y축 범위(manual)도 적용하지 않는다.
  // 단, 명시적 열거형(enumMode)이 설정되면 그쪽이 우선하므로 boolAxis 는 끈다.
  const boolAxis = allBoolean && !enumMode;

  /**
   * 툴팁 값 표기.
   *
   * 소수 자릿수는 **Y축 눈금과 같은 설정**을 쓴다. 축은 12.35 인데 툴팁은
   * 12.3456 이면 같은 값을 두 자리로 읽게 된다.
   *
   * 순서가 있다 — boolean 시리즈는 1/0 이 아니라 true/false 로 읽어야 하고,
   * 열거형 축은 값 자체가 라벨이므로 자릿수 개념이 없다. 자릿수는 그 둘에
   * 해당하지 않는 **숫자에만** 적용한다. 다만 열거형 축에서 매핑에 없는 값은
   * 숫자로 떨어지므로(`formatEnumValue` 의 폴백) 거기에도 자릿수를 적용한다.
   */
  const formatTooltipValue = (value: unknown, name: string): React.ReactNode => {
    const dec = readDecimalPlaces(config);
    const isNum = typeof value === 'number' && Number.isFinite(value);

    if (enumMode && isNum) {
      const label = enumMap.get(value as number);
      return label ?? formatDecimal(value as number, dec);
    }
    if (booleanKeys.has(name)) return value === 1 ? 'true' : 'false';
    // 축과 **같은 배율**로 접어 표기한다. 접미사는 붙이지 않는다 — 단위는 축 라벨이 한 번만
    // 말한다는 규칙(`formatTickValue` 머리말)을 툴팁도 따른다. 종전에는 원값을 그대로
    // 찍어 축은 `137`(KB) 인데 값은 `137355.20`(B) 로 같은 점이 두 자리로 읽혔다.
    if (isNum) return formatTickValue(value as number, cfg.y_unit, dec);
    // 숫자가 아니면 그대로 — 문자열·null 은 Recharts 가 알아서 표기한다.
    return value as React.ReactNode;
  };
  // 값 표기 자릿수는 기본값이 있으므로 툴팁 포맷터는 **항상** 필요하다. 종전에는
  // 자릿수 미지정 + enum/boolean 아님이면 포맷터를 주지 않아 원값(21.533333333333335)이
  // 그대로 나왔다.
  const tooltipNeedsFormatter = true;
  /**
   * 그려진 값들의 최소·최대. 유효한 값이 하나도 없으면 undefined.
   *
   * 같은 주사를 세 곳이 따로 돌고 있었다 — 자동 여백 도메인, 자동 환산 배율의 기준값,
   * 그리고 축 폭 표본. 한 번만 돌고 나눠 쓴다.
   */
  const dataExtent = useMemo<[number, number] | undefined>(() => {
    let minV = Number.POSITIVE_INFINITY;
    let maxV = Number.NEGATIVE_INFINITY;
    for (const row of chartData) {
      for (const k of seriesKeys) {
        const v = row[k as keyof typeof row];
        if (typeof v === 'number' && Number.isFinite(v)) {
          if (v < minV) minV = v;
          if (v > maxV) maxV = v;
        }
      }
    }
    return Number.isFinite(minV) && Number.isFinite(maxV) ? [minV, maxV] : undefined;
  }, [chartData, seriesKeys]);

  const yDomain = useMemo<[number | 'auto', number | 'auto']>(() => {
    // 열거형 축: 매핑된 최소/최대 값 ±0.5 여백으로 고정(모든 눈금이 보이도록).
    if (enumMode && enumTicks && enumTicks.length > 0) {
      const lo = enumTicks[0]!;
      const hi = enumTicks[enumTicks.length - 1]!;
      return [lo - 0.5, hi + 0.5];
    }
    if (boolAxis) {
      return [-0.1, 1.1];
    }
    if (yAxisMode === 'manual') {
      return [cfg.y_min ?? 'auto', cfg.y_max ?? 'auto'];
    }
    if (yAxisMode === 'auto_padded') {
      if (!dataExtent) return ['auto', 'auto'];
      const [minV, maxV] = dataExtent;
      const range = maxV - minV || Math.abs(maxV) || 1;
      const pad = (range * yPadPct) / 100;
      return [minV - pad, maxV + pad];
    }
    // 'auto'
    return ['auto', 'auto'];
  }, [enumMode, enumTicks, boolAxis, yAxisMode, cfg.y_min, cfg.y_max, yPadPct, dataExtent]);

  // 글로벌 smooth fallback (하위 호환)
  const globalSmooth = cfg.smooth ?? false;
  const legendCfg: LegendConfig = (cfg.legend as LegendConfig | undefined) ?? {};
  // 저장된 변위는 성긴 상한으로만 죈다 — 정확한 죄기는 요소 크기를 알아야 하는데
  // 그 값은 레이아웃 후에만 나온다(끌 때는 드래그 레이어가 정확히 죈다).
  const legendOffsetX = clampStoredLegendOffset(legendCfg.offset_x);
  const legendOffsetY = clampStoredLegendOffset(legendCfg.offset_y);

  // 그림 상자의 크기·자리 — 파이·게이지와 **같은 규칙**(패널 대비 백분율)을 쓴다.
  // 기본값(100%, 0, 0)이면 transform 자체가 없어 저장된 대시보드의 그림이 변하지 않는다.
  const plotSize = readPanelSize(config.plot_size) ?? PANEL_SIZE_MAX;
  const plotOffsetX = readPanelOffset(config.plot_offset_x);
  const plotOffsetY = readPanelOffset(config.plot_offset_y);

  /**
   * 공용 편집 표면이 다루는 요소 — 플롯 상자와 범례 둘이다.
   *
   * 드래그 계산은 `ChartDragLayer` 가 계속 소유한다. 범례는 흐름 안에 남아 "지금 그려진
   * 자리에서 상자 밖으로 나가지 않는 만큼" 으로 죄고(`clampInlineLegendOffset`), 재는
   * 상자와 변위를 얹는 상자가 두 겹이라 공용 레이어가 대신할 수 없다.
   */
  const {
    selection,
    setSelection,
    snap,
    setSnap,
    boundsRef,
    align,
    reset,
  } = usePanelElementEdit<LineElementKind>({
    kinds: LINE_ELEMENT_KINDS,
    enabled: edit.active,
    // 상한은 읽는 쪽이 정한다 — 그림 상자는 `readPanelOffset` 의 ±40, 범례는
    // `clampStoredLegendOffset` 의 ±50. 정렬이 더 느슨하게 죄면 맞춘 자리가
    // 다음에 읽힐 때 되돌아간다.
    offsets: {
      plot: { x: plotOffsetX, y: plotOffsetY, limit: PANEL_OFFSET_LIMIT },
      legend: {
        x: clampStoredLegendOffset(legendCfg.offset_x),
        y: clampStoredLegendOffset(legendCfg.offset_y),
        limit: LEGEND_OFFSET_SAFETY_LIMIT,
      },
    },
    writeOffsets: (patches) => {
      const next: Record<string, unknown> = {};
      for (const p of patches) {
        if (p.kind === 'plot') {
          next.plot_offset_x = p.x;
          next.plot_offset_y = p.y;
        } else {
          next.legend = { ...legendCfg, offset_x: p.x, offset_y: p.y };
        }
      }
      onConfigChange?.(next);
    },
  });
  /** 요소 상자에 붙는 편집 표식 — 정렬이 상자를 찾고, 누르면 선택된다. */
  const editProps = (kind: LineElementKind) =>
    edit.active
      ? {
          'data-panel-drag': kind,
          onPointerDown: () =>
            setSelection(selection.has(kind) ? selection : new Set<LineElementKind>([kind])),
        }
      : {};
  const outlineOf = (kind: LineElementKind): string =>
    edit.active
      ? selection.has(kind)
        ? PANEL_SELECTED_OUTLINE_CLASS
        : PANEL_EDIT_OUTLINE_CLASS
      : '';
  const plotTransform = panelBoxTransform(plotSize, plotOffsetX, plotOffsetY);

  // 축 폰트(레이블/눈금) — 미지정 필드는 기본값(size 10, #9ca3af, normal)으로 폴백.
  const xTickFont = resolveAxisFont(cfg.x_tick_font);
  const xLabelFont = resolveAxisFont(cfg.x_label_font);
  const yTickFont = resolveAxisFont(cfg.y_tick_font);
  const yLabelFont = resolveAxisFont(cfg.y_label_font);

  /**
   * 축 폭·높이 — 내용에서 산출한다.
   *
   * 종전에는 Y축이 56px 고정이라, 축 제목 글꼴을 키우거나 눈금에 단위·소수를
   * 붙이면 제목이 잘렸다. 눈금 글자와 제목이 같은 고정 폭을 나눠 쓰고 있었기
   * 때문이다. 이제 둘을 **더해서** 잡는다.
   *
   * 눈금 표본은 실제로 찍힐 문자열을 만든다 — 도메인 양끝을 그대로 포맷터에
   * 통과시키고, 열거형·불리언 축은 라벨 자체를 쓴다. 추정이라 정확하진 않지만
   * "무엇이 커지면 폭도 큰다" 는 관계는 지켜진다.
   */
  const yTickSamples = useMemo(() => {
    if (enumMode) return [...enumMap.values()];
    if (boolAxis) return ['false', 'true'];
    const fmt = yNumberFormatter ?? ((v: number) => String(v));
    // 표본 선택은 순수 함수가 맡는다 — 자동 축에서 표본이 비어 폭이 주저앉던 결함의
    // 자리이므로, 전수 검증이 가능한 형태로 떼어 둔다.
    return yTickSampleValues(yDomain, dataExtent).map(fmt);
  }, [enumMode, enumMap, boolAxis, yNumberFormatter, yDomain, dataExtent]);

  // 축 라벨의 단위는 **축이 실제로 접은 배율**이다(`axisUnitLabel`). 자동 환산 단위의
  // 저장값(`auto:bytes`)을 그대로 붙이면 눈금은 KB 인데 라벨은 규칙 이름을 말한다.
  // 배율은 가장 큰 눈금이 정하므로 도메인 상한을 기준으로 삼는다.
  // 배율 기준값. 도메인 상한이 수면 그대로 쓰고, 'auto' 면 실제 데이터 최대값을 찾는다 —
  // 자동 축에서는 Recharts 가 상한을 정하므로 여기서는 알 수 없다. 스캔은 자동 환산
  // 단위일 때만 한다(다른 단위에서는 라벨이 저장값 그대로라 기준값이 필요 없다).
  const yScaleReference = useMemo<number | undefined>(() => {
    if (!isAutoScaledUnit(cfg.y_unit)) return undefined;
    if (typeof yDomain[1] === 'number') return yDomain[1];
    return dataExtent?.[1];
  }, [cfg.y_unit, yDomain, dataExtent]);
  const yUnitLabel = axisUnitLabel(cfg.y_unit, yScaleReference);
  const yAxisTitle =
    [cfg.y_label, yUnitLabel ? `(${yUnitLabel})` : ''].filter(Boolean).join(' ') || undefined;

  const yAxisWidth = resolveYAxisWidth({
    tickTexts: yTickSamples,
    tickFontSize: yTickFont.fontSize,
    label: yAxisTitle,
    labelFontSize: yLabelFont.fontSize,
  });
  const xAxisHeight = resolveXAxisHeight({
    tickFontSize: xTickFont.fontSize,
    label: cfg.x_label,
    labelFontSize: xLabelFont.fontSize,
  });


  const criticalBreached = useMemo(
    () =>
      isCriticalBreached(
        chartData as Array<Record<string, unknown>>,
        seriesKeys,
        cfg.y_thresholds,
      ),
    [chartData, seriesKeys, cfg.y_thresholds],
  );

  const containerClass = criticalBreached
    ? 'relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4 ring-2 ring-red-500 animate-pulse'
    : 'relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4 ring-1 ring-(--color-border-default)';

  return (
    // 경보 상태(critical 임계 초과)를 다는 자리다. 드래그 레이어가 본문을 한 겹 더
    // 감싸므로 부모 관계로 이 요소를 찾으면 배치가 바뀔 때마다 깨진다 — 표식을 둔다.
    <div className={containerClass} data-testid="line-chart-root">
      {/* 상단 우측: CSV 다운로드 + 일시정지 토글 + 연결 상태 아이콘 */}
      <div className="absolute right-3 top-3 z-10 flex items-center gap-1">
        <button
          type="button"
          onClick={() => handleExportCsv(chartData as Array<Record<string, unknown>>, seriesKeys)}
          data-testid="line-chart-csv-button"
          className="flex h-6 w-6 items-center justify-center rounded text-(--color-text-muted) hover:bg-(--color-bg-hover) hover:text-(--color-text-default)"
          aria-label={t('dashboard.chart.exportCsv')}
          title={t('dashboard.chart.exportCsv')}
        >
          <Download className="h-3.5 w-3.5" />
        </button>
        <button
          type="button"
          onClick={togglePause}
          data-testid="line-chart-pause-button"
          className="flex h-6 w-6 items-center justify-center rounded text-(--color-text-muted) hover:bg-(--color-bg-hover) hover:text-(--color-text-default)"
          aria-label={isPaused ? t('dashboard.chart.resume') : t('dashboard.chart.pause')}
          title={isPaused ? t('dashboard.chart.resume') : t('dashboard.chart.pause')}
        >
          {isPaused ? <Play className="h-3.5 w-3.5" /> : <Pause className="h-3.5 w-3.5" />}
        </button>
        <ConnectionStatusIcon status={status} />
      </div>

      {showTitle && (
      <div className="mb-2 flex shrink-0 items-center gap-2 pr-24">
        <TrendingUp className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
        <span className="truncate text-sm font-semibold text-(--color-text-primary)" style={titleStyle}>
          {title || t('dashboard.settings.preview.channelUnset')}
        </span>
      </div>
      )}

      {/* 채널 상태는 범례(Legend)에 통합 — 별도 배지 불필요 */}

      {isPaused && (
        <div
          data-testid="line-chart-pause-badge"
          className="absolute left-3 top-3 z-10 rounded bg-amber-500/90 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-white shadow"
        >
          PAUSED
        </div>
      )}

      {edit.toggle}

      <ChartDragLayer
        enabled={edit.active}
        snap={snap}
        selection={selection}
        onSelectionChange={setSelection}
        legend={{
          offsetX: legendOffsetX,
          offsetY: legendOffsetY,
          onChange: ({ x, y }) =>
            onConfigChange?.({
              legend: {
                ...((config.legend as Record<string, unknown>) ?? {}),
                offset_x: x,
                offset_y: y,
              },
            }),
          // 범례는 글자 덩어리라 손잡이가 글자 크기(px)를 바꾼다.
          size: resolveFontSize(legendCfg.font_size) ?? DEFAULT_CHART_LEGEND_FONT_SIZE,
          onResize: (font_size) =>
            onConfigChange?.({
              legend: { ...((config.legend as Record<string, unknown>) ?? {}), font_size },
            }),
        }}
        plot={{
          offsetX: plotOffsetX,
          offsetY: plotOffsetY,
          onChange: ({ x, y }) => onConfigChange?.({ plot_offset_x: x, plot_offset_y: y }),
          // 그림은 글자가 아니므로 손잡이가 백분율(`plot_size`)을 바꾼다.
          size: readPanelSize(config.plot_size) ?? PANEL_SIZE_MAX,
          sizeRange: { min: PANEL_SIZE_MIN, max: PANEL_SIZE_MAX },
          onResize: (plot_size) => onConfigChange?.({ plot_size }),
        }}
      >
      <div
        className={cn(
          'min-h-0 flex-1 flex',
          legendCfg.position === 'left' && 'flex-row-reverse',
          legendCfg.position === 'right' && 'flex-row',
          (!legendCfg.position || legendCfg.position === 'bottom') && 'flex-col',
        )}
        data-testid="line-chart-container"
        // 범례를 끌 수 있는 범위 — 차트와 범례가 함께 들어 있는 본문이다.
        data-chart-legend-bounds=""
        // 공용 정렬이 상자를 재는 기준이기도 하다(같은 영역이므로 표식을 함께 단다).
        data-panel-bounds=""
        ref={boundsRef}
      >
        {/* 배치 그리드와 중심 표식 — 요소 뒤에 깔리고 포인터를 받지 않는다. */}
        <PanelEditGrid enabled={edit.active} />
        <div
          className={`relative min-h-0 min-w-0 flex-1 ${outlineOf('plot')}`}
          data-testid="line-chart-plot"
          // 끌어 옮길 대상 표식 — 범례와 같은 레이어가 둘을 구분해 잡는다.
          data-chart-plot-area=""
          {...editProps('plot')}
          style={{ transform: plotTransform }}
        >
        {edit.active && selection.has('plot') && (
          <PanelResizeHandle kind="plot" enabled label={t('dashboard.chart.plotSize')} />
        )}
        <ResponsiveContainer width="100%" height="100%">
          <ComposedChart
            data={
              chartDataWithCandles.length > 0
                ? chartDataWithCandles
                : [{ timestamp: Date.now() }]
            }
            margin={{ top: 8, right: 16, left: 0, bottom: 0 }}
          >
            <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
            <XAxis
              dataKey="timestamp"
              type="number"
              domain={xDomain}
              allowDataOverflow={xDomainFixed}
              ticks={xTicks}
              tickFormatter={(v: number) => formatTimeShort(v)}
              tick={{ fontSize: xTickFont.fontSize, fill: xTickFont.fill, fontWeight: xTickFont.fontWeight }}
              stroke="#9ca3af"
              // insideBottom 은 y = 축상단 + height - offset (verticalAnchor 'end') 로 계산되므로
              // offset 을 양수로 주어 텍스트를 축 하단(=SVG 경계)에서 위로 띄워 잘림을 방지한다.
              label={cfg.x_label ? { value: cfg.x_label, position: 'insideBottom', offset: 6, style: { textAnchor: 'middle', fontSize: xLabelFont.fontSize, fill: xLabelFont.fill, fontWeight: xLabelFont.fontWeight } } : undefined}
              height={xAxisHeight}
            />
            <YAxis
              domain={yDomain}
              tick={{ fontSize: yTickFont.fontSize, fill: yTickFont.fill, fontWeight: yTickFont.fontWeight }}
              stroke="#9ca3af"
              width={yAxisWidth}
              label={
                yAxisTitle
                  ? {
                      value: yAxisTitle,
                      angle: -90,
                      position: 'insideLeft',
                      style: {
                        fontSize: yLabelFont.fontSize,
                        fill: yLabelFont.fill,
                        fontWeight: yLabelFont.fontWeight,
                      },
                    }
                  : undefined
              }
              // 열거형 축은 매핑된 값 눈금을 라벨로, boolean 축은 0/1 을 false/true 로 표시한다.
              ticks={enumMode ? enumTicks : boolAxis ? [0, 1] : undefined}
              tickFormatter={
                enumMode
                  ? (v: number) => formatEnumValue(v, enumMap)
                  : boolAxis
                    ? (v: number) => (v === 1 ? 'true' : v === 0 ? 'false' : '')
                    : yNumberFormatter
              }
            />
            {tooltipEnabled && (
            <Tooltip
              // 단일 값: 커서에 가장 가까운 라인 하나만 보여 준다.
              //
              // `shared={false}` 로는 되지 않는다 — v3 의 LineChart 는 허용 툴팁
              // 이벤트 타입이 `['axis']` 뿐이라 그 프롭이 무시된다. 축 모드를
              // 그대로 두고 payload 를 좁히는 것이 실제로 동작하는 유일한 길이다.
              // 두 모드 모두 자체 내용을 쓴다 — 레이블 왼쪽 · 값 오른쪽 정렬은
              // recharts 기본 내용(`이름 : 값` 한 줄)으로는 만들 수 없다.
              content={tooltipSingle ? SingleSeriesTooltipContent : ChartTooltipContent}
              // 열거형 라벨 · boolean true/false · 소수 자릿수를 한 함수가 정한다.
              formatter={
                tooltipNeedsFormatter
                  ? (value, name) => [formatTooltipValue(value, String(name)), name]
                  : undefined
              }
              labelFormatter={(v) => {
                const n = typeof v === 'number' ? v : Number(v);
                return Number.isFinite(n) ? formatTimestamp(n) : String(v ?? '');
              }}
              // 다크모드에서 흰색 배경 + 흰색 시간 라벨로 인해 타임이 보이지 않던 문제 해결.
              // CSS 변수로 surface 배경 + primary 텍스트 색상 적용.
              contentStyle={{
                fontSize: '0.75rem',
                backgroundColor: 'var(--color-bg-surface)',
                border: '1px solid var(--color-border-default)',
                borderRadius: '6px',
                color: 'var(--color-text-primary)',
              }}
              labelStyle={{ color: 'var(--color-text-primary)' }}
              itemStyle={{ color: 'var(--color-text-primary)' }}
            />
            )}
            {cfg.y_thresholds?.map((t, i) => {
              const color = t.color ?? THRESHOLD_DEFAULT_COLORS[t.severity ?? 'info'];
              return (
                <ReferenceLine
                  key={`th-${i}`}
                  y={t.value}
                  stroke={color}
                  strokeDasharray="4 2"
                  label={
                    t.label
                      ? { value: t.label, position: 'right', fontSize: 10, fill: color }
                      : undefined
                  }
                />
              );
            })}
            {cfg.y_thresholds?.map((t, i) => {
              // 채울 구간을 축 도메인 안으로 좁힌다. recharts 의 ReferenceArea 는
              // 기본값이 `ifOverflow: 'discard'` 라, 한 끝이라도 도메인 밖이면
              // 통째로 그리지 않는다 — 종전의 ±1e9 표현이 그래서 안 보였다.
              const band = resolveFillBand(t, yDomain);
              if (!band) return null;
              // 선과 같은 색 규칙을 쓴다. 색을 지정하지 않은 경계도 채워져야 한다.
              const fill = t.color ?? THRESHOLD_DEFAULT_COLORS[t.severity ?? 'info'];
              return (
                <ReferenceArea
                  key={`fill-${i}`}
                  y1={band.y1}
                  y2={band.y2}
                  fill={fill}
                  fillOpacity={0.1}
                  strokeOpacity={0}
                  // 도메인을 모르는 경우(auto)의 안전망 — 버리는 대신 잘라 낸다.
                  ifOverflow="hidden"
                />
              );
            })}
            {seriesKeys.map((key, i) => {
              let stroke = SERIES_COLORS[i % SERIES_COLORS.length]!;
              let strokeDasharray: string | undefined;
              let strokeWidth = 2;
              let lineSmooth = globalSmooth;

              if (isStore) {
                // SPEC-WEB-005: store 시리즈는 seriesStyles(시리즈 표시 이름 기준)에서
                // per-line 스타일을 적용한다. 미지정 필드는 기본값을 유지한다.
                const st = storeResult.seriesStyles.get(key);
                if (st) {
                  if (st.color) stroke = st.color;
                  if (st.stroke_width) strokeWidth = st.stroke_width;
                  if (st.smooth != null) lineSmooth = st.smooth;
                  const dash = STROKE_DASHARRAY[st.stroke_style ?? 'solid'];
                  if (dash) strokeDasharray = dash;
                }
              }
              // 이 시리즈의 실제 모양. 시리즈 지정이 없으면 패널 기본값을 따른다.
              const seriesStyle: GraphStyle = resolveSeriesStyle(
                isStore ? storeResult.seriesStyles.get(key)?.graph_style : undefined,
                panelGraphStyle,
              );
              // 스택킹은 영역·바에서만 뜻이 있다. 같은 stackId 를 공유해야 쌓인다.
              const stackId = effectiveStacked(seriesStyle, panelStacked) ? 'stack' : undefined;
              // 결측 점선은 라인·영역에서만 그린다 — 바에 그으면 없는 막대를 잇는 선이 된다.
              const gapKey = gapSeriesKey(key);
              const hasGap = gapKeySet.has(gapKey) && hasGapDash(seriesStyle);
              const common = {
                dataKey: key,
                isAnimationActive: false,
                stackId,
              } as const;
              return (
                <Fragment key={key}>
                  {seriesStyle === 'candle' ? (
                    // 캔들은 몸통 구간에 Bar 를 세우고 모양 함수가 꼬리를 마저 그린다.
                    // 스택킹은 붙지 않는다 — 네 값이 한 덩어리라 쌓을 수 없다.
                    <Bar
                      dataKey={key}
                      isAnimationActive={false}
                      shape={(p: object) => (
                        <CandleShape {...p} seriesKey={key} />
                      )}
                    />
                  ) : seriesStyle === 'bar' ? (
                    <Bar {...common} fill={stroke} />
                  ) : seriesStyle === 'area' ? (
                    <Area
                      {...common}
                      type={lineSmooth ? 'monotone' : 'linear'}
                      stroke={stroke}
                      strokeWidth={strokeWidth}
                      strokeDasharray={strokeDasharray}
                      fill={stroke}
                      fillOpacity={0.25}
                      dot={false}
                      connectNulls={gapThreshold <= 0}
                    />
                  ) : (
                  <Line
                    {...common}
                    type={lineSmooth ? 'monotone' : 'linear'}
                    stroke={stroke}
                    strokeWidth={strokeWidth}
                    strokeDasharray={strokeDasharray}
                    dot={false}
                    // 점선 표기를 켠 패널에서는 결측에서 선을 **끊는다** — 끊지 않으면
                    // 덧그림 점선 아래에 실선이 그대로 남아 둘이 겹친다.
                    connectNulls={gapThreshold <= 0}
                  />
                  )}
                  {hasGap && (
                    <Line
                      type={lineSmooth ? 'monotone' : 'linear'}
                      dataKey={gapKey}
                      stroke={stroke}
                      strokeWidth={strokeWidth}
                      strokeDasharray={GAP_DASHARRAY}
                      dot={false}
                      isAnimationActive={false}
                      connectNulls={false}
                      // 범례·툴팁에는 넣지 않는다 — 새 시리즈가 아니라 같은
                      // 시리즈의 결측 구간 표기다.
                      legendType="none"
                      tooltipType="none"
                    />
                  )}
                  {hasGap && (
                    // 실선이 끊기고 점선이 시작되는 자리를 점으로 짚는다.
                    // 선은 그리지 않는다 — 이미 두 라인이 그 구간을 덮고 있다.
                    <Line
                      type="linear"
                      dataKey={gapDotSeriesKey(key)}
                      stroke="none"
                      dot={{ r: 3, fill: stroke, strokeWidth: 0 }}
                      activeDot={false}
                      isAnimationActive={false}
                      connectNulls={false}
                      legendType="none"
                      tooltipType="none"
                    />
                  )}
                </Fragment>
              );
            })}
          </ComposedChart>
        </ResponsiveContainer>
        </div>

        {/* 범례 — recharts 바깥, CSS flex로 배치 */}
        <ChartLegend
          seriesKeys={seriesKeys}
          seriesColors={seriesKeys.map((key, i) => {
            if (isStore) {
              return (
                storeResult.seriesStyles.get(key)?.color ??
                SERIES_COLORS[i % SERIES_COLORS.length]!
              );
            }
            return SERIES_COLORS[i % SERIES_COLORS.length]!;
          })}
          legendCfg={legendCfg}
          chartData={chartData}
          edit={
            edit.active
              ? {
                  props: editProps('legend'),
                  outline: outlineOf('legend'),
                  handle: selection.has('legend') ? (
                    <PanelResizeHandle
                      kind="legend"
                      enabled
                      label={t('dashboard.chart.legendFontSize')}
                    />
                  ) : undefined,
                }
              : undefined
          }
          // 범례 마지막값도 툴팁과 동일하게 표시한다 — 같은 값이 범례와 툴팁에서
          // 다른 자릿수로 읽히면 어느 쪽이 맞는지 알 수 없다. 종전에는 1자리 고정이었다.
          // enum 축이면 라벨로, boolean 시리즈면 true/false, 그 외는 설정 자릿수.
          formatValue={(key: string, v: number) => {
            if (enumMode) return formatEnumValue(v, enumMap);
            if (booleanKeys.has(key)) return v === 1 ? 'true' : v === 0 ? 'false' : String(v);
            // 툴팁·축과 같은 배율 — 셋이 한 값을 말해야 한다.
            return formatTickValue(v, cfg.y_unit, readDecimalPlaces(config));
          }}
        />
      </div>
      </ChartDragLayer>

      {/* 정렬 툴바 — 편집 중에만. */}
      <PanelAlignToolbar
        enabled={edit.active}
        snap={snap}
        onSnapChange={setSnap}
        onAlign={align}
        onReset={reset}
      />

      {/* 부분 실패 배지 — 성공 시리즈는 그대로 렌더하고 실패 개수만 알린다(§2.14 · §2.19).
          오버레이가 아니라 배지인 이유: 남은 시리즈는 정상이므로 화면을 덮으면 안 된다.
          `role="status"` 로 스크린리더에 실패 개수를 전달한다(§4.7 접근성). */}
      {seriesDisplay.state === 'partial-failure' && (
        <div
          role="status"
          data-testid="panel-series-partial-badge"
          className="absolute right-2 top-2 rounded-full bg-amber-500/90 px-2 py-0.5 text-[10px] font-medium text-white"
        >
          {t('dashboard.chart.seriesPartialFailure').replace(
            '{count}',
            String(seriesDisplay.failureCount),
          )}
        </div>
      )}

      {/* 빈 선택 안내 — 오류가 아니라 "아직 고르지 않았다" 다(§2.14). */}
      {showEmptySelection && (
        <div
          data-testid="panel-series-empty-selection"
          className="absolute inset-0 flex items-center justify-center rounded-2xl p-4 text-center text-xs text-(--color-text-muted)"
        >
          {t('dashboard.chart.seriesEmptySelection')}
        </div>
      )}

      {/* 그룹 페이지 바 — **넘길 페이지가 있거나 절단 경고가 있을 때만** 그린다.
          그림 위에 겹치는 표기는 조작할 것이나 알릴 것이 있을 때만 값을 한다.
          판정은 resolveGroupPageDisplay 가 소유한다(§2.7.2). */}
      {groupPage.show && (
        <div
          data-testid="line-chart-group-page"
          className="absolute bottom-1 right-2 flex items-center gap-1 text-[10px] text-(--color-text-muted)"
        >
          <span data-testid="line-chart-group-page-status">
            {t('dashboard.chart.tsdbGroupPageStatus')
              .replace('{total}', String(groupPage.total))
              .replace('{page}', String(groupPage.page + 1))
              .replace('{pageCount}', String(groupPage.pageCount))}
          </span>
          {groupPage.pageCount > 1 && (
            <>
              <button
                type="button"
                data-testid="line-chart-group-page-prev"
                aria-label={t('dashboard.chart.tsdbGroupPagePrev')}
                disabled={!groupPage.canPrev}
                aria-disabled={!groupPage.canPrev}
                onClick={() => onGroupPageChange?.(Math.max(0, groupPage.page - 1))}
                className="rounded px-1 disabled:opacity-40"
              >
                {'<'}
              </button>
              <button
                type="button"
                data-testid="line-chart-group-page-next"
                aria-label={t('dashboard.chart.tsdbGroupPageNext')}
                disabled={!groupPage.canNext}
                aria-disabled={!groupPage.canNext}
                onClick={() => onGroupPageChange?.(groupPage.page + 1)}
                className="rounded px-1 disabled:opacity-40"
              >
                {'>'}
              </button>
            </>
          )}
          {groupPage.truncated && (
            <span
              data-testid="line-chart-group-page-truncated"
              role="alert"
              className="text-amber-500"
              title={t('dashboard.chart.tsdbGroupPageTruncated')}
            >
              !
            </span>
          )}
        </div>
      )}

      {(status === 'closed' || status === 'error') && (
        <div
          className="absolute inset-0 flex items-center justify-center rounded-2xl bg-black/50 p-4 text-center text-sm text-white"
          data-testid="line-chart-overlay"
        >
          {status === 'closed'
            ? `Channel closed: ${closedReason ?? 'unknown'}`
            : seriesDisplay.backendMismatch
              ? // §2.18 · UB2-7: 백엔드 불일치는 일반 조회 실패와 사유가 다르다. 원문
                // 오류 메시지 대신 "무엇을 고쳐야 하는가" 를 말하는 문구를 쓴다.
                t('dashboard.chart.dataSourceTsdbBackendMismatch')
              : `Error: ${errorReason ?? 'unknown'}`}
        </div>
      )}
    </div>
  );
}
