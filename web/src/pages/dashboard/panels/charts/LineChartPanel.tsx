// Line Chart 패널 (REQ-M4-05).
// x축=timestamp, y축=display_field.
// multi_series_field 가 지정되면 label 값별로 line 을 분리한다.

import { useMemo } from 'react';
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

import { getByPath, type LineChartPanelConfig } from './chartChannelTypes';
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
    smooth: (config.smooth as boolean) ?? false,
    multi_series_field: config.multi_series_field as string | undefined,
  };
}

export default function LineChartPanel({ panelId: _panelId, config }: LineChartPanelProps) {
  const cfg = parseConfig(config);
  const { entries, status, closedReason, errorReason } = useChartChannel(
    cfg.channel_name || undefined,
    { maxPoints: cfg.max_points ?? DEFAULT_MAX_POINTS },
  );

  const { chartData, seriesKeys } = useMemo(() => {
    const displayField = cfg.display_field ?? 'value';
    const seriesField = cfg.multi_series_field;

    if (!seriesField) {
      // 단일 시리즈: [{ timestamp, value }]
      const data = entries.map((e) => ({
        timestamp: e.timestamp,
        value: toNumber(getByPath(e, displayField)),
      }));
      return { chartData: data, seriesKeys: ['value'] };
    }

    // 다중 시리즈: [{ timestamp, [series1]: v, [series2]: v, ... }]
    const rows = new Map<number, Record<string, number | null | unknown>>();
    const seen = new Set<string>();
    for (const e of entries) {
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
  }, [entries, cfg.display_field, cfg.multi_series_field]);

  const yDomain: [number | 'auto', number | 'auto'] = [
    cfg.y_min ?? 'auto',
    cfg.y_max ?? 'auto',
  ];
  const lineType = cfg.smooth ? 'monotone' : 'linear';

  return (
    <div className="relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4 ring-1 ring-(--color-border-default)">
      {/* 상단 우측: 연결 상태 아이콘 */}
      <div className="absolute right-3 top-3 z-10">
        <ConnectionStatusIcon status={status} />
      </div>

      <div className="mb-2 truncate pr-6 text-xs font-medium text-(--color-text-muted)">
        {cfg.channel_name || '채널 미지정'}
      </div>

      <div className="min-h-0 flex-1" data-testid="line-chart-container">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={chartData} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
            <XAxis
              dataKey="timestamp"
              type="number"
              domain={['dataMin', 'dataMax']}
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
