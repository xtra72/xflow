// Pie Chart 패널 (REQ-M4-07).
// 최근 max_points 항목을 label_field 별로 그룹화한 값의 비율을 파이로 표시.
//
// SPEC-CHART-002: Store 모드 + `series_reduce` 지정 시에는 **시리즈당 조각 1개**를 그리는
// 다중 출력 경로로 갈린다. 이때 `label_field` · `agg_func` · `max_points` 는 무시된다
// (§2.9 [S1]). 분기는 아래 `derived` useMemo 진입부 한 곳뿐이다.

import { useMemo } from 'react';
import { PieChart as PieChartIcon } from 'lucide-react';
import {
  Cell,
  Legend,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
} from 'recharts';

import {
  pickSeriesColor,
  type ChartEntry,
  type PiePanelConfig,
} from './chartChannelTypes';
import { ConnectionStatusIcon } from './ConnectionStatusIcon';
import { aggregateByLabel } from './chartChannelUtils';
import { reduceAllSeries } from './seriesReduce';
import { applyMultiOutputLimit, MultiOutputTruncationNotice } from './SeriesTileGrid';
import { useChartChannel } from './useChartChannel';
import { type StoreSeriesStyle } from './useStoreChartData';
import { resolvePanelSourceBinding } from './panelDataSource';
import { isPanelSeriesSource, usePanelSeriesData } from './usePanelSeriesData';
import { usePanelTitleVisible } from '../../panelChromeContext';
import { useTranslation } from '@/lib/i18n';

interface PieChartPanelProps {
  panelId: string;
  title?: string;
  config: Record<string, unknown>;
}

const DEFAULT_MAX_POINTS = 20;

const PIE_COLORS = [
  '#3b82f6',
  '#10b981',
  '#f59e0b',
  '#ef4444',
  '#8b5cf6',
  '#06b6d4',
  '#ec4899',
  '#84cc16',
  '#f97316',
  '#14b8a6',
];

/** 시리즈 축이 없는 경로(채널 모드)에서 쓰는 빈 기본값 — 매 렌더 새 객체를 만들지 않는다. */
const EMPTY_SERIES_ENTRIES: ReadonlyMap<string, ChartEntry[]> = new Map();
const EMPTY_SERIES_STYLES: ReadonlyMap<string, StoreSeriesStyle> = new Map();
const EMPTY_SERIES_NAMES: readonly string[] = [];

function parseConfig(config: Record<string, unknown>): PiePanelConfig {
  return {
    channel_name: (config.channel_name as string) ?? '',
    display_field: (config.display_field as string) ?? 'value',
    label_field: (config.label_field as string) ?? 'labels.name',
    agg_func: (config.agg_func as PiePanelConfig['agg_func']) ?? 'sum',
    show_legend: (config.show_legend as boolean | undefined) ?? true,
    show_percentage: (config.show_percentage as boolean | undefined) ?? true,
    max_points: (config.max_points as number) ?? DEFAULT_MAX_POINTS,
    // SPEC-CHART-002 — 유무가 곧 렌더 경로 스위치다. 기본값을 채우지 않는다.
    series_reduce: config.series_reduce as PiePanelConfig['series_reduce'],
    multi_output_limit: config.multi_output_limit as number | undefined,
  };
}

/** 파이 조각 1개. `fill` 은 다중 출력 경로에서만 채워진다. */
interface PieSlice {
  name: string;
  value: number;
  fill?: string;
}

export default function PieChartPanel({ panelId: _panelId, title, config }: PieChartPanelProps) {
  const { t } = useTranslation();
  const showTitle = usePanelTitleVisible();
  const cfg = parseConfig(config);
  // SPEC-WEB-005: data_source 에 따라 Store 소스 또는 채널 소스를 사용한다(공존).
  // SPEC-TSDB-002 §2.3 [U3]: 소스 판정은 `panelDataSource` 계약이 소유한다. 패널은
  // `data_source` 를 직접 비교하지 않는다 — 소스 종류가 늘어도 이 지점이 종류만큼
  // 곱해지지 않게 하기 위함이다(UB1-1).
  // `isStore` 는 "채널이 아닌 시리즈 소스가 활성인가" 를 뜻한다. M3 시점에는 store 만
  // 그 조건을 만족하며, tsdb 는 M6 에서 같은 이름을 통해 합류한다.
  const isStore = isPanelSeriesSource(resolvePanelSourceBinding(config));

  const channelRes = useChartChannel(
    isStore ? undefined : cfg.channel_name || undefined,
    { maxPoints: Math.max(cfg.max_points ?? DEFAULT_MAX_POINTS, 100) },
  );
  const storeRes = usePanelSeriesData(config);

  const { entries, status, closedReason, errorReason } = isStore
    ? storeRes
    : channelRes;

  // 시리즈 축은 Store 경로에만 존재한다.
  const seriesEntries = storeRes.seriesEntries ?? EMPTY_SERIES_ENTRIES;
  const seriesNames = storeRes.seriesNames ?? EMPTY_SERIES_NAMES;
  const seriesStyles = storeRes.seriesStyles ?? EMPTY_SERIES_STYLES;

  // SPEC-CHART-002 §2.9 [S1] — 데이터 파생의 유일한 분기점.
  const isReduceMode = isStore && cfg.series_reduce !== undefined;

  const derived = useMemo<{
    slices: PieSlice[];
    truncated: number;
    negativeOmitted: number;
    reduce: boolean;
  }>(() => {
    if (isReduceMode && cfg.series_reduce !== undefined) {
      // 다중 출력: 조각 = 시리즈, 채움색 = 시리즈 색 ?? 팔레트(§2.6).
      // 팔레트 인덱스는 **필터 전** 시리즈 순서를 쓴다 — 생략된 조각 때문에 나머지 색이
      // 밀려나면 폴링마다 색이 바뀐다.
      const slices: PieSlice[] = [];
      let negativeOmitted = 0;
      reduceAllSeries(seriesEntries, seriesNames, seriesStyles, cfg.series_reduce).forEach(
        (r, i) => {
          // 값 없음: 조각 생략(0 과 구분되지 않는 조각을 그리지 않는다).
          if (r.value === undefined) return;
          // 음수: 파이는 음수를 표현할 수 없다. 절댓값(부호 소실)도 0 clamp(사유 은폐)도
          // 아닌 **생략 + 사유 안내**를 택한다(§4.4 / OQ6).
          if (r.value < 0) {
            negativeOmitted += 1;
            return;
          }
          slices.push({ name: r.name, value: r.value, fill: r.color ?? pickSeriesColor(i) });
        },
      );
      const { visible, truncated } = applyMultiOutputLimit(slices, cfg.multi_output_limit);
      return { slices: visible, truncated, negativeOmitted, reduce: true };
    }

    // --- 레거시 경로(변경 금지 — 특성화 CH-08~CH-10 이 지킨다) ---
    const recent =
      entries.length > (cfg.max_points ?? DEFAULT_MAX_POINTS)
        ? entries.slice(-(cfg.max_points ?? DEFAULT_MAX_POINTS))
        : entries;
    const out = aggregateByLabel(
      recent,
      cfg.label_field ?? 'labels.name',
      cfg.display_field ?? 'value',
      cfg.agg_func ?? 'sum',
    );
    return {
      slices: out.map((r) => ({ name: r.label, value: r.value })),
      truncated: 0,
      negativeOmitted: 0,
      reduce: false,
    };
  }, [
    isReduceMode,
    cfg.series_reduce,
    cfg.multi_output_limit,
    seriesEntries,
    seriesNames,
    seriesStyles,
    entries,
    cfg.label_field,
    cfg.display_field,
    cfg.agg_func,
    cfg.max_points,
  ]);

  const chartData = derived.slices;

  return (
    <div className="relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4 ring-1 ring-(--color-border-default)">
      <div className="absolute right-3 top-3 z-10">
        <ConnectionStatusIcon status={status} />
      </div>

      {showTitle && (
        <div className="mb-1 flex shrink-0 items-center gap-2 pr-6">
          <PieChartIcon className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
          <span className="truncate text-sm font-semibold text-(--color-text-primary)">
            {title || cfg.channel_name || '채널 미지정'}
          </span>
        </div>
      )}

      <div className="min-h-0 flex-1" data-testid="pie-chart-container">
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={chartData}
              dataKey="value"
              nameKey="name"
              cx="50%"
              cy="50%"
              outerRadius="80%"
              label={
                cfg.show_percentage
                  ? ({ percent }: { percent?: number }) =>
                      `${((percent ?? 0) * 100).toFixed(0)}%`
                  : undefined
              }
              isAnimationActive={false}
            >
              {/*
                레거시 경로는 기존 PIE_COLORS 순환 배정을 그대로 쓴다(CH-09).
                다중 출력 경로에서만 시리즈 색(미지정 시 팔레트)으로 대체한다.
              */}
              {chartData.map((slice, i) => (
                <Cell
                  key={i}
                  fill={derived.reduce ? slice.fill : PIE_COLORS[i % PIE_COLORS.length]}
                />
              ))}
            </Pie>
            <Tooltip contentStyle={{ fontSize: '0.75rem' }} />
            {cfg.show_legend && <Legend wrapperStyle={{ fontSize: '0.75rem' }} />}
          </PieChart>
        </ResponsiveContainer>
      </div>

      {/* 음수 대표값으로 생략된 조각의 사유 안내(§4.4 — 조각이 사라진 이유가 화면에 남는다). */}
      {derived.negativeOmitted > 0 && (
        <p
          data-testid="pie-negative-omitted"
          className="mt-2 shrink-0 text-center text-xs text-(--color-text-muted)"
        >
          {t('dashboard.chart.pieNegativeOmitted').replace(
            '{count}',
            String(derived.negativeOmitted),
          )}
        </p>
      )}

      <MultiOutputTruncationNotice truncated={derived.truncated} />

      {(status === 'closed' || status === 'error') && (
        <div
          className="absolute inset-0 flex items-center justify-center rounded-2xl bg-black/50 p-4 text-center text-sm text-white"
          data-testid="pie-chart-overlay"
        >
          {status === 'closed'
            ? `Channel closed: ${closedReason ?? 'unknown'}`
            : `Error: ${errorReason ?? 'unknown'}`}
        </div>
      )}
    </div>
  );
}
