// Line Chart 패널 (REQ-M4-05).
// x축=timestamp, y축=display_field.
// multi_series_field 가 지정되면 label 값별로 line 을 분리한다.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Download, Pause, Play, TrendingUp } from 'lucide-react';
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
import { cn } from '@/lib/utils/cn';

import {
  getByPath,
  STROKE_DASHARRAY,
  THRESHOLD_DEFAULT_COLORS,
  type ChannelRefConfig,
  type ChartEntry,
  type LegendConfig,
  type LineChartPanelConfig,
  type TimeWindowMode,
  type YThreshold,
  type YAxisMode,
} from './chartChannelTypes';
import { ConnectionStatusIcon } from './ConnectionStatusIcon';
import {
  computeNiceTimeTicks,
  formatTimeShort,
  formatTimestamp,
  toNumber,
} from './chartChannelUtils';
import type { ChartConnectionStatus } from '@/services/ws/chartChannel';
import { chartDataToCsv, downloadCsv } from './csvExport';
import { useChartChannel } from './useChartChannel';
import { useChartChannels, type ChannelState } from './useChartChannels';

interface LineChartPanelProps {
  panelId: string;
  title?: string;
  config: Record<string, unknown>;
}

const DEFAULT_MAX_POINTS = 100;
const DEFAULT_RECENT_WINDOW_SEC = 600;
const DEFAULT_REFRESH_MS = 1000;
const MIN_REFRESH_MS = 200;
const MAX_REFRESH_MS = 60_000;
const DEFAULT_Y_PAD_PCT = 5;
const MAX_Y_PAD_PCT = 50;

/** multi-series 색상 팔레트 */
const SERIES_COLORS = [
  '#3b82f6',
  '#10b981',
  '#f59e0b',
  '#ef4444',
  '#8b5cf6',
  '#06b6d4',
  '#ec4899',
  '#84cc16',
];

function parseConfig(config: Record<string, unknown>): LineChartPanelConfig {
  return {
    channel_name: (config.channel_name as string) ?? '',
    channels: config.channels as ChannelRefConfig[] | undefined,
    display_field: (config.display_field as string) ?? 'value',
    max_points: (config.max_points as number) ?? DEFAULT_MAX_POINTS,
    x_label: config.x_label as string | undefined,
    y_min: config.y_min as number | undefined,
    y_max: config.y_max as number | undefined,
    y_axis_mode: config.y_axis_mode as YAxisMode | undefined,
    y_axis_padding_pct: config.y_axis_padding_pct as number | undefined,
    y_label: config.y_label as string | undefined,
    y_unit: config.y_unit as string | undefined,
    y_thresholds: config.y_thresholds as YThreshold[] | undefined,
    time_window_mode: config.time_window_mode as TimeWindowMode | undefined,
    recent_window_sec: config.recent_window_sec as number | undefined,
    fixed_start_ms: config.fixed_start_ms as number | undefined,
    fixed_end_ms: config.fixed_end_ms as number | undefined,
    time_window_refresh_ms: config.time_window_refresh_ms as number | undefined,
    legend: config.legend as LegendConfig | undefined,
    smooth: (config.smooth as boolean) ?? false,
    multi_series_field: config.multi_series_field as string | undefined,
  };
}

/** 채널 상태들의 status 를 통합 — 가장 심각한 상태가 우세. */
function aggregateStatus(states: ChartConnectionStatus[]): ChartConnectionStatus {
  if (states.length === 0) return 'idle';
  const order: ChartConnectionStatus[] = [
    'error',
    'closed',
    'disconnected',
    'connecting',
    'connected',
    'idle',
  ];
  for (const s of order) {
    if (states.includes(s)) return s;
  }
  return states[0]!;
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

/** 시간 윈도우 모드에 따라 entries 를 필터링 */
function filterByTimeWindow(
  entries: ChartEntry[],
  mode: TimeWindowMode,
  now: number,
  windowSec: number,
  startMs: number | undefined,
  endMs: number | undefined,
): ChartEntry[] {
  if (mode === 'recent') {
    const start = now - windowSec * 1000;
    return entries.filter((e) => e.timestamp >= start && e.timestamp <= now);
  }
  if (mode === 'fixed') {
    const s = startMs ?? Number.NEGATIVE_INFINITY;
    const e = endMs ?? Number.POSITIVE_INFINITY;
    return entries.filter((x) => x.timestamp >= s && x.timestamp <= e);
  }
  return entries;
}

interface NormalizedChannel {
  ref: ChannelRefConfig;
  state: ChannelState;
}

function CustomLegend({
  seriesKeys,
  seriesColors,
  channelStates,
  isMultiMode,
  legendCfg,
  chartData,
}: {
  seriesKeys: string[];
  seriesColors: string[];
  channelStates: NormalizedChannel[];
  isMultiMode: boolean;
  legendCfg: LegendConfig;
  chartData: Array<Record<string, unknown>>;
}): React.ReactElement | null {
  if (seriesKeys.length === 0) return null;
  const isVert = legendCfg.position === 'left' || legendCfg.position === 'right';
  const showName = legendCfg.show_name !== false;
  const showLine = legendCfg.show_line !== false;
  const showLastValue = legendCfg.show_last_value === true;

  // 각 시리즈별 마지막 유효 값 (역순 탐색)
  const lastValues = useMemo(() => {
    if (!showLastValue || chartData.length === 0) return {};
    const result: Record<string, number | undefined> = {};
    for (const key of seriesKeys) {
      for (let i = chartData.length - 1; i >= 0; i--) {
        const v = chartData[i]![key as keyof (typeof chartData)[0]];
        if (typeof v === 'number' && Number.isFinite(v)) {
          result[key] = v;
          break;
        }
      }
    }
    return result;
  }, [showLastValue, chartData, seriesKeys]);

  return (
    <div
      className={cn(
        'flex shrink-0 text-[11px]',
        isVert
          ? 'min-w-fit flex-col justify-center gap-y-1 border-l border-(--color-border-default) py-2 pl-3 pr-2'
          : 'flex-wrap justify-center gap-x-4 gap-y-1 border-t border-(--color-border-default) py-1.5 px-2',
      )}
      data-testid="line-chart-legend"
    >
      {seriesKeys.map((key, i) => {
        const baseKey = key.includes('::') ? key.split('::')[0]! : key;
        const chState = isMultiMode
          ? channelStates.find((c) => (c.ref.alias ?? c.ref.name) === baseKey)
          : channelStates[0];
        const st = chState?.state.status ?? 'idle';
        const statusDot =
          st === 'connected'
            ? 'bg-emerald-400'
            : st === 'error'
              ? 'bg-rose-400'
              : 'bg-gray-400';
        const lastVal = lastValues[key];
        const lastStr = lastVal !== undefined ? lastVal.toFixed(1) : '—';
        return (
          <span
            key={key}
            data-testid={`line-chart-channel-status-${chState?.ref.name ?? key}`}
            data-status={st}
            className={cn(
              'inline-flex items-center gap-1',
              isVert && showLastValue && 'w-full',
            )}
          >
            {showLine && (
              <span
                className="inline-block h-0.5 w-3 shrink-0 rounded-full"
                style={{ backgroundColor: seriesColors[i] }}
              />
            )}
            {showName && (
              <span className="shrink-0 whitespace-nowrap text-(--color-text-primary)">{key}</span>
            )}
            {showLastValue && (
              <span className={cn(
                'shrink-0 whitespace-nowrap font-mono text-[10px] text-(--color-text-muted)',
                isVert && 'ml-auto text-right',
              )}>
                {lastStr}
              </span>
            )}
            <span
              className={`inline-block h-1.5 w-1.5 shrink-0 rounded-full ${statusDot}`}
              title={st}
            />
          </span>
        );
      })}
    </div>
  );
}

export default function LineChartPanel({ panelId: _panelId, title, config }: LineChartPanelProps) {
  const cfg = parseConfig(config);
  const isMultiMode = (cfg.channels?.length ?? 0) > 0;

  // 두 hook 모두 항상 호출 (React hook 규칙). 비활성 모드는 idle 상태로 유지.
  const singleResult = useChartChannel(
    isMultiMode ? undefined : cfg.channel_name || undefined,
    { maxPoints: cfg.max_points ?? DEFAULT_MAX_POINTS },
  );
  const multiResult = useChartChannels(
    isMultiMode ? cfg.channels! : [],
    { maxPoints: cfg.max_points ?? DEFAULT_MAX_POINTS },
  );

  // 모드별 채널 정규화
  const channelStates: NormalizedChannel[] = useMemo(
    () =>
      isMultiMode
        ? cfg.channels!.map((ref) => ({
            ref,
            state: multiResult.channels.get(ref.name) ?? {
              entries: [],
              status: 'connecting' as ChartConnectionStatus,
            },
          }))
        : [
            {
              ref: {
                name: cfg.channel_name || '',
                display_field: cfg.display_field,
              },
              state: {
                entries: singleResult.entries,
                status: singleResult.status,
                closedReason: singleResult.closedReason,
                errorReason: singleResult.errorReason,
              },
            },
          ],
    [
      isMultiMode,
      cfg.channels,
      cfg.channel_name,
      cfg.display_field,
      multiResult.channels,
      singleResult.entries,
      singleResult.status,
      singleResult.closedReason,
      singleResult.errorReason,
    ],
  );

  // 통합 상태 (가장 심각한 status 우세)
  const status = aggregateStatus(channelStates.map((c) => c.state.status));
  const closedReason = channelStates.find((c) => c.state.closedReason)?.state
    .closedReason;
  const errorReason = channelStates.find((c) => c.state.errorReason)?.state
    .errorReason;

  // 시간 윈도우 설정
  const timeWindowMode: TimeWindowMode = cfg.time_window_mode ?? 'points';
  const recentWindowSec = cfg.recent_window_sec ?? DEFAULT_RECENT_WINDOW_SEC;
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

  useEffect(() => {
    if (timeWindowMode !== 'recent' || isPaused) return;
    const id = window.setInterval(() => setNow(Date.now()), refreshMs);
    return () => window.clearInterval(id);
  }, [timeWindowMode, refreshMs, isPaused]);

  // raw 데이터 계산 (모드별 분기)
  const { chartData: rawChartData, seriesKeys: rawSeriesKeys } = useMemo(() => {
    const seriesField = cfg.multi_series_field;
    const filterArgs = [
      timeWindowMode,
      now,
      recentWindowSec,
      cfg.fixed_start_ms,
      cfg.fixed_end_ms,
    ] as const;

    if (isMultiMode) {
      // 다채널: 채널마다 alias 기반 시리즈 키 (multi_series_field 시 alias::label)
      const rows = new Map<number, Record<string, unknown>>();
      const seen = new Set<string>();
      for (const { ref, state } of channelStates) {
        const filtered = filterByTimeWindow(state.entries, ...filterArgs);
        const baseKey = ref.alias ?? ref.name;
        const channelField = ref.display_field ?? cfg.display_field ?? 'value';
        for (const e of filtered) {
          let key: string;
          if (seriesField) {
            const sRaw = getByPath(e, seriesField);
            const s = sRaw == null ? 'default' : String(sRaw);
            key = `${baseKey}::${s}`;
          } else {
            key = baseKey;
          }
          seen.add(key);
          const v = toNumber(getByPath(e, channelField));
          if (!rows.has(e.timestamp)) {
            rows.set(e.timestamp, { timestamp: e.timestamp });
          }
          rows.get(e.timestamp)![key] = v;
        }
      }
      const data = Array.from(rows.values()).sort(
        (a, b) => (a.timestamp as number) - (b.timestamp as number),
      );
      return { chartData: data, seriesKeys: Array.from(seen) };
    }

    // 단일 채널 (기존 동작 유지)
    const filtered = filterByTimeWindow(
      channelStates[0]!.state.entries,
      ...filterArgs,
    );
    const displayField = cfg.display_field ?? 'value';
    if (!seriesField) {
      const data = filtered.map((e) => ({
        timestamp: e.timestamp,
        value: toNumber(getByPath(e, displayField)),
      }));
      return { chartData: data, seriesKeys: ['value'] };
    }
    const rows = new Map<number, Record<string, unknown>>();
    const seen = new Set<string>();
    for (const e of filtered) {
      const sRaw = getByPath(e, seriesField);
      const s = sRaw == null ? 'default' : String(sRaw);
      seen.add(s);
      const v = toNumber(getByPath(e, displayField));
      if (!rows.has(e.timestamp)) {
        rows.set(e.timestamp, { timestamp: e.timestamp });
      }
      rows.get(e.timestamp)![s] = v;
    }
    const data = Array.from(rows.values()).sort(
      (a, b) => (a.timestamp as number) - (b.timestamp as number),
    );
    return { chartData: data, seriesKeys: Array.from(seen) };
  }, [
    isMultiMode,
    channelStates,
    cfg.display_field,
    cfg.multi_series_field,
    timeWindowMode,
    now,
    recentWindowSec,
    cfg.fixed_start_ms,
    cfg.fixed_end_ms,
  ]);

  // 일시정지 시 스냅샷 사용
  const chartData = pauseSnapshot ? pauseSnapshot.chartData : rawChartData;
  const seriesKeys = pauseSnapshot ? pauseSnapshot.seriesKeys : rawSeriesKeys;
  const effectiveNow = pauseSnapshot ? pauseSnapshot.now : now;

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
      );
      const baseName =
        isMultiMode && cfg.channels && cfg.channels.length > 0
          ? cfg.channels.map((c) => c.alias ?? c.name).join('_')
          : cfg.channel_name || 'chart';
      const ts = new Date().toISOString().replace(/[:.]/g, '-');
      downloadCsv(csv, `${baseName}-${ts}.csv`);
    },
    [cfg.channel_name, cfg.channels, isMultiMode],
  );

  // X축 도메인
  const xDomain = useMemo<[number | 'dataMin', number | 'dataMax']>(() => {
    if (timeWindowMode === 'recent') {
      return [effectiveNow - recentWindowSec * 1000, effectiveNow];
    }
    if (timeWindowMode === 'fixed') {
      const end = cfg.fixed_end_ms ?? Date.now();
      const start = cfg.fixed_start_ms ?? end;
      return [start, end];
    }
    return ['dataMin', 'dataMax'];
  }, [
    timeWindowMode,
    effectiveNow,
    recentWindowSec,
    cfg.fixed_start_ms,
    cfg.fixed_end_ms,
  ]);

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

  // Y축 도메인
  const yAxisMode: YAxisMode = cfg.y_axis_mode ?? 'auto';
  const yPadPct = clamp(cfg.y_axis_padding_pct ?? DEFAULT_Y_PAD_PCT, 0, MAX_Y_PAD_PCT);
  const yDomain = useMemo<[number | 'auto', number | 'auto']>(() => {
    if (yAxisMode === 'manual') {
      return [cfg.y_min ?? 'auto', cfg.y_max ?? 'auto'];
    }
    if (yAxisMode === 'auto_padded') {
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
      if (!Number.isFinite(minV) || !Number.isFinite(maxV)) {
        return ['auto', 'auto'];
      }
      const range = maxV - minV || Math.abs(maxV) || 1;
      const pad = (range * yPadPct) / 100;
      return [minV - pad, maxV + pad];
    }
    // 'auto'
    return ['auto', 'auto'];
  }, [yAxisMode, cfg.y_min, cfg.y_max, yPadPct, chartData, seriesKeys]);

  // 글로벌 smooth fallback (하위 호환)
  const globalSmooth = cfg.smooth ?? false;
  const legendCfg: LegendConfig = (cfg.legend as LegendConfig | undefined) ?? {};

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
    <div className={containerClass}>
      {/* 상단 우측: CSV 다운로드 + 일시정지 토글 + 연결 상태 아이콘 */}
      <div className="absolute right-3 top-3 z-10 flex items-center gap-1">
        <button
          type="button"
          onClick={() => handleExportCsv(chartData as Array<Record<string, unknown>>, seriesKeys)}
          data-testid="line-chart-csv-button"
          className="flex h-6 w-6 items-center justify-center rounded text-(--color-text-muted) hover:bg-(--color-bg-hover) hover:text-(--color-text-default)"
          aria-label="CSV 내보내기"
          title="CSV 내보내기"
        >
          <Download className="h-3.5 w-3.5" />
        </button>
        <button
          type="button"
          onClick={togglePause}
          data-testid="line-chart-pause-button"
          className="flex h-6 w-6 items-center justify-center rounded text-(--color-text-muted) hover:bg-(--color-bg-hover) hover:text-(--color-text-default)"
          aria-label={isPaused ? '재개' : '일시정지'}
          title={isPaused ? '재개' : '일시정지'}
        >
          {isPaused ? <Play className="h-3.5 w-3.5" /> : <Pause className="h-3.5 w-3.5" />}
        </button>
        <ConnectionStatusIcon status={status} />
      </div>

      <div className="mb-2 flex shrink-0 items-center gap-2 pr-24">
        <TrendingUp className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
        <span className="truncate text-sm font-semibold text-(--color-text-primary)">
          {title || (isMultiMode
            ? cfg
                .channels!.map((c) => c.alias ?? c.name)
                .filter((n) => !!n)
                .join(', ') || '채널 미지정'
            : cfg.channel_name || '채널 미지정')}
        </span>
      </div>

      {/* 채널 상태는 범례(Legend)에 통합 — 별도 배지 불필요 */}

      {isPaused && (
        <div
          data-testid="line-chart-pause-badge"
          className="absolute left-3 top-3 z-10 rounded bg-amber-500/90 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-white shadow"
        >
          PAUSED
        </div>
      )}

      <div
        className={cn(
          'min-h-0 flex-1 flex',
          legendCfg.position === 'left' && 'flex-row-reverse',
          legendCfg.position === 'right' && 'flex-row',
          (!legendCfg.position || legendCfg.position === 'bottom') && 'flex-col',
        )}
        data-testid="line-chart-container"
      >
        <div className="min-h-0 min-w-0 flex-1">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart
            data={chartData.length > 0 ? chartData : [{ timestamp: Date.now() }]}
            margin={{ top: 8, right: 16, left: 0, bottom: 0 }}
          >
            <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
            <XAxis
              dataKey="timestamp"
              type="number"
              domain={xDomain}
              allowDataOverflow={timeWindowMode !== 'points'}
              ticks={xTicks}
              tickFormatter={(v: number) => formatTimeShort(v)}
              tick={{ fontSize: 10 }}
              stroke="#9ca3af"
              label={cfg.x_label ? { value: cfg.x_label, position: 'insideBottomRight', offset: -4, style: { fontSize: 10, fill: '#9ca3af' } } : undefined}
              height={cfg.x_label ? 40 : 30}
            />
            <YAxis
              domain={yDomain}
              tick={{ fontSize: 10 }}
              stroke="#9ca3af"
              width={cfg.y_label || cfg.y_unit ? 56 : 50}
              label={cfg.y_label || cfg.y_unit ? { value: [cfg.y_label, cfg.y_unit ? `(${cfg.y_unit})` : ''].filter(Boolean).join(' '), angle: -90, position: 'insideLeft', style: { fontSize: 10, fill: '#9ca3af' } } : undefined}
              tickFormatter={cfg.y_unit ? (v: number) => `${v}${cfg.y_unit}` : undefined}
            />
            <Tooltip
              labelFormatter={(v) => {
                const n = typeof v === 'number' ? v : Number(v);
                return Number.isFinite(n) ? formatTimestamp(n) : String(v ?? '');
              }}
              contentStyle={{ fontSize: '0.75rem' }}
            />
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
            {cfg.y_thresholds
              ?.filter((t) => t.fill_direction || t.fill_to != null)
              .map((t, i) => {
                let y1: number;
                let y2: number;
                if (t.fill_direction === 'below') {
                  y1 = -1e9;
                  y2 = t.value;
                } else if (t.fill_direction === 'above') {
                  y1 = t.value;
                  y2 = 1e9;
                } else {
                  y1 = Math.min(t.value, t.fill_to!);
                  y2 = Math.max(t.value, t.fill_to!);
                }
                return (
                  <ReferenceArea
                    key={`fill-${i}`}
                    y1={y1}
                    y2={y2}
                    fill={t.color}
                    fillOpacity={0.1}
                    strokeOpacity={0}
                  />
                );
              })}
            {seriesKeys.map((key, i) => {
              let stroke = SERIES_COLORS[i % SERIES_COLORS.length]!;
              let strokeDasharray: string | undefined;
              let strokeWidth = 2;
              let lineSmooth = globalSmooth;

              if (isMultiMode) {
                const baseKey = key.includes('::') ? key.split('::')[0]! : key;
                const ref = cfg.channels!.find(
                  (c) => (c.alias ?? c.name) === baseKey,
                );
                if (ref) {
                  if (ref.color) stroke = ref.color;
                  if (ref.stroke_width) strokeWidth = ref.stroke_width;
                  if (ref.smooth != null) lineSmooth = ref.smooth;
                  const style = ref.stroke_style ?? 'solid';
                  const dash = STROKE_DASHARRAY[style];
                  if (dash) strokeDasharray = dash;
                }
              }
              return (
                <Line
                  key={key}
                  type={lineSmooth ? 'monotone' : 'linear'}
                  dataKey={key}
                  stroke={stroke}
                  strokeWidth={strokeWidth}
                  strokeDasharray={strokeDasharray}
                  dot={false}
                  isAnimationActive={false}
                  connectNulls
                />
              );
            })}
          </LineChart>
        </ResponsiveContainer>
        </div>

        {/* 범례 — recharts 바깥, CSS flex로 배치 */}
        <CustomLegend
          seriesKeys={seriesKeys}
          seriesColors={seriesKeys.map((key, i) => {
            if (isMultiMode) {
              const baseKey = key.includes('::') ? key.split('::')[0]! : key;
              const ref = cfg.channels!.find(
                (c) => (c.alias ?? c.name) === baseKey,
              );
              return ref?.color ?? SERIES_COLORS[i % SERIES_COLORS.length]!;
            }
            return SERIES_COLORS[i % SERIES_COLORS.length]!;
          })}
          channelStates={channelStates}
          isMultiMode={isMultiMode}
          legendCfg={legendCfg}
          chartData={chartData}
        />
      </div>

      {(status === 'closed' || status === 'error') && (
        <div
          className="absolute inset-0 flex items-center justify-center rounded-2xl bg-black/50 p-4 text-center text-sm text-white"
          data-testid="line-chart-overlay"
        >
          {status === 'closed'
            ? `Channel closed: ${closedReason ?? 'unknown'}`
            : `Error: ${errorReason ?? 'unknown'}`}
        </div>
      )}
    </div>
  );
}
