// Line Chart 패널 (REQ-M4-05).
// x축=timestamp, y축=display_field.
// multi_series_field 가 지정되면 label 값별로 line 을 분리한다.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Pause, Play } from 'lucide-react';
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

import {
  getByPath,
  type ChartEntry,
  type LineChartPanelConfig,
  type TimeWindowMode,
  type YAxisMode,
} from './chartChannelTypes';
import { ConnectionStatusIcon } from './ConnectionStatusIcon';
import {
  formatTimeShort,
  formatTimestamp,
  toNumber,
} from './chartChannelUtils';
import { useChartChannel } from './useChartChannel';

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
    display_field: (config.display_field as string) ?? 'value',
    max_points: (config.max_points as number) ?? DEFAULT_MAX_POINTS,
    y_min: config.y_min as number | undefined,
    y_max: config.y_max as number | undefined,
    y_axis_mode: config.y_axis_mode as YAxisMode | undefined,
    y_axis_padding_pct: config.y_axis_padding_pct as number | undefined,
    time_window_mode: config.time_window_mode as TimeWindowMode | undefined,
    recent_window_sec: config.recent_window_sec as number | undefined,
    fixed_start_ms: config.fixed_start_ms as number | undefined,
    fixed_end_ms: config.fixed_end_ms as number | undefined,
    time_window_refresh_ms: config.time_window_refresh_ms as number | undefined,
    smooth: (config.smooth as boolean) ?? false,
    multi_series_field: config.multi_series_field as string | undefined,
  };
}

function clamp(n: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(hi, n));
}

export default function LineChartPanel({ panelId: _panelId, config }: LineChartPanelProps) {
  const cfg = parseConfig(config);
  const { entries, status, closedReason, errorReason } = useChartChannel(
    cfg.channel_name || undefined,
    { maxPoints: cfg.max_points ?? DEFAULT_MAX_POINTS },
  );

  // 시간 윈도우 설정
  const timeWindowMode: TimeWindowMode = cfg.time_window_mode ?? 'points';
  const recentWindowSec = cfg.recent_window_sec ?? DEFAULT_RECENT_WINDOW_SEC;
  const refreshMs = clamp(
    cfg.time_window_refresh_ms ?? DEFAULT_REFRESH_MS,
    MIN_REFRESH_MS,
    MAX_REFRESH_MS,
  );

  // 일시정지: 클릭 시점의 entries 와 now 를 스냅샷으로 보관
  const [pauseSnapshot, setPauseSnapshot] = useState<
    { entries: ChartEntry[]; now: number } | null
  >(null);
  const isPaused = pauseSnapshot !== null;

  // 'recent' 모드에서 현재 시각을 주기적으로 갱신 (슬라이딩 윈도우). 일시정지 중에는 정지.
  const [now, setNow] = useState<number>(() => Date.now());
  useEffect(() => {
    if (timeWindowMode !== 'recent' || isPaused) return;
    const id = window.setInterval(() => setNow(Date.now()), refreshMs);
    return () => window.clearInterval(id);
  }, [timeWindowMode, refreshMs, isPaused]);

  // 일시정지 시 사용할 effective 값
  const effectiveEntries = pauseSnapshot ? pauseSnapshot.entries : entries;
  const effectiveNow = pauseSnapshot ? pauseSnapshot.now : now;

  const togglePause = useCallback(() => {
    setPauseSnapshot((prev) =>
      prev ? null : { entries: [...entries], now: Date.now() },
    );
  }, [entries]);

  // 시간 윈도우 적용 — entries 를 [start, end] 범위로 필터링
  const filteredEntries = useMemo(() => {
    if (timeWindowMode === 'recent') {
      const start = effectiveNow - recentWindowSec * 1000;
      return effectiveEntries.filter(
        (e) => e.timestamp >= start && e.timestamp <= effectiveNow,
      );
    }
    if (timeWindowMode === 'fixed') {
      const start = cfg.fixed_start_ms ?? Number.NEGATIVE_INFINITY;
      const end = cfg.fixed_end_ms ?? Number.POSITIVE_INFINITY;
      return effectiveEntries.filter(
        (e) => e.timestamp >= start && e.timestamp <= end,
      );
    }
    return effectiveEntries; // 'points': 시간 기반 필터링 없음
  }, [
    effectiveEntries,
    timeWindowMode,
    recentWindowSec,
    effectiveNow,
    cfg.fixed_start_ms,
    cfg.fixed_end_ms,
  ]);

  const { chartData, seriesKeys } = useMemo(() => {
    const displayField = cfg.display_field ?? 'value';
    const seriesField = cfg.multi_series_field;

    if (!seriesField) {
      // 단일 시리즈: [{ timestamp, value }]
      const data = filteredEntries.map((e) => ({
        timestamp: e.timestamp,
        value: toNumber(getByPath(e, displayField)),
      }));
      return { chartData: data, seriesKeys: ['value'] };
    }

    // 다중 시리즈: [{ timestamp, [series1]: v, [series2]: v, ... }]
    const rows = new Map<number, Record<string, number | null | unknown>>();
    const seen = new Set<string>();
    for (const e of filteredEntries) {
      const sRaw = getByPath(e, seriesField);
      const s = sRaw == null ? 'default' : String(sRaw);
      seen.add(s);
      const v = toNumber(getByPath(e, displayField));
      if (!rows.has(e.timestamp)) {
        rows.set(e.timestamp, { timestamp: e.timestamp });
      }
      const row = rows.get(e.timestamp)!;
      row[s] = v;
    }
    const data = Array.from(rows.values()).sort(
      (a, b) => (a.timestamp as number) - (b.timestamp as number),
    );
    return { chartData: data, seriesKeys: Array.from(seen) };
  }, [filteredEntries, cfg.display_field, cfg.multi_series_field]);

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

  return (
    <div className="relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4 ring-1 ring-(--color-border-default)">
      {/* 상단 우측: 일시정지 토글 + 연결 상태 아이콘 */}
      <div className="absolute right-3 top-3 z-10 flex items-center gap-1">
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

      <div className="mb-2 truncate pr-14 text-xs font-medium text-(--color-text-muted)">
        {cfg.channel_name || '채널 미지정'}
      </div>

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
            {seriesKeys.map((key, i) => (
              <Line
                key={key}
                type={lineType}
                dataKey={key}
                stroke={SERIES_COLORS[i % SERIES_COLORS.length]}
                strokeWidth={2}
                dot={false}
                isAnimationActive={false}
                connectNulls
              />
            ))}
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
