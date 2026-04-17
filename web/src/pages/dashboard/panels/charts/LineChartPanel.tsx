// Line Chart 패널 (REQ-M4-05).
// x축=timestamp, y축=display_field.
// multi_series_field 가 지정되면 label 값별로 line 을 분리한다.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Download, Pause, Play } from 'lucide-react';
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

import {
  getByPath,
  THRESHOLD_DEFAULT_COLORS,
  type ChannelRefConfig,
  type ChartEntry,
  type LineChartPanelConfig,
  type TimeWindowMode,
  type YThreshold,
  type YAxisMode,
} from './chartChannelTypes';
import { ConnectionStatusIcon } from './ConnectionStatusIcon';
import {
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
    y_min: config.y_min as number | undefined,
    y_max: config.y_max as number | undefined,
    y_axis_mode: config.y_axis_mode as YAxisMode | undefined,
    y_axis_padding_pct: config.y_axis_padding_pct as number | undefined,
    y_thresholds: config.y_thresholds as YThreshold[] | undefined,
    time_window_mode: config.time_window_mode as TimeWindowMode | undefined,
    recent_window_sec: config.recent_window_sec as number | undefined,
    fixed_start_ms: config.fixed_start_ms as number | undefined,
    fixed_end_ms: config.fixed_end_ms as number | undefined,
    time_window_refresh_ms: config.time_window_refresh_ms as number | undefined,
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

function thresholdColor(t: YThreshold): string {
  if (t.color) return t.color;
  return THRESHOLD_DEFAULT_COLORS[t.severity ?? 'info'];
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

export default function LineChartPanel({ panelId: _panelId, config }: LineChartPanelProps) {
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

  const lineType = cfg.smooth ? 'monotone' : 'linear';

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

      <div className="mb-2 truncate pr-24 text-xs font-medium text-(--color-text-muted)">
        {isMultiMode
          ? cfg
              .channels!.map((c) => c.alias ?? c.name)
              .filter((n) => !!n)
              .join(', ') || '다채널 미지정'
          : cfg.channel_name || '채널 미지정'}
      </div>

      {isMultiMode && (
        <div className="mb-2 flex flex-wrap gap-1.5">
          {channelStates.map(({ ref, state }) => {
            const label = ref.alias ?? (ref.name || '(미지정)');
            const dotColor = ref.color ?? '#9ca3af';
            const title =
              state.status === 'closed' && state.closedReason
                ? `${state.status}: ${state.closedReason}`
                : state.status === 'error' && state.errorReason
                  ? `${state.status}: ${state.errorReason}`
                  : state.status;
            return (
              <span
                key={ref.name || label}
                data-testid={`line-chart-channel-status-${ref.name || label}`}
                data-status={state.status}
                title={title}
                className="inline-flex items-center gap-1 rounded-full bg-(--color-bg-elevated) px-1.5 py-0.5 text-[10px] text-(--color-text-muted) ring-1 ring-(--color-border-default)"
              >
                <span
                  aria-hidden="true"
                  className="inline-block h-2 w-2 rounded-full"
                  style={{ backgroundColor: dotColor }}
                />
                <span className="max-w-[8rem] truncate">{label}</span>
                <ConnectionStatusIcon status={state.status} className="h-3 w-3" />
              </span>
            );
          })}
        </div>
      )}

      {isPaused && (
        <div
          data-testid="line-chart-pause-badge"
          className="absolute left-3 top-3 z-10 rounded bg-amber-500/90 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-white shadow"
        >
          PAUSED
        </div>
      )}

      <div className="min-h-0 flex-1" data-testid="line-chart-container">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={chartData} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
            <XAxis
              dataKey="timestamp"
              type="number"
              domain={xDomain}
              allowDataOverflow={timeWindowMode !== 'points'}
              tickFormatter={(v: number) => formatTimeShort(v)}
              tick={{ fontSize: 10 }}
              stroke="#9ca3af"
            />
            <YAxis domain={yDomain} tick={{ fontSize: 10 }} stroke="#9ca3af" width={50} />
            <Tooltip
              labelFormatter={(v) => {
                const n = typeof v === 'number' ? v : Number(v);
                return Number.isFinite(n) ? formatTimestamp(n) : String(v ?? '');
              }}
              contentStyle={{ fontSize: '0.75rem' }}
            />
            {seriesKeys.length > 1 && <Legend wrapperStyle={{ fontSize: '0.75rem' }} />}
            {cfg.y_thresholds?.map((t, i) => (
              <ReferenceLine
                key={`th-${i}`}
                y={t.value}
                stroke={thresholdColor(t)}
                strokeDasharray="4 2"
                label={
                  t.label
                    ? {
                        value: t.label,
                        position: 'right',
                        fontSize: 10,
                        fill: thresholdColor(t),
                      }
                    : undefined
                }
              />
            ))}
            {seriesKeys.map((key, i) => {
              // 다채널 모드: alias 기준 색상 매칭 (alias::label 도 alias 부분으로 lookup)
              let stroke = SERIES_COLORS[i % SERIES_COLORS.length];
              if (isMultiMode) {
                const baseKey = key.includes('::') ? key.split('::')[0]! : key;
                const ref = cfg.channels!.find(
                  (c) => (c.alias ?? c.name) === baseKey,
                );
                if (ref?.color) stroke = ref.color;
              }
              return (
                <Line
                  key={key}
                  type={lineType}
                  dataKey={key}
                  stroke={stroke}
                  strokeWidth={2}
                  dot={false}
                  isAnimationActive={false}
                  connectNulls
                />
              );
            })}
          </LineChart>
        </ResponsiveContainer>
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
