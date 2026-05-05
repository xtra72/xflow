// Pie Chart 패널 (REQ-M4-07).
// 최근 max_points 항목을 label_field 별로 그룹화한 값의 비율을 파이로 표시.

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

import { type PiePanelConfig } from './chartChannelTypes';
import { ConnectionStatusIcon } from './ConnectionStatusIcon';
import { aggregateByLabel } from './chartChannelUtils';
import { useChartChannel } from './useChartChannel';

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

function parseConfig(config: Record<string, unknown>): PiePanelConfig {
  return {
    channel_name: (config.channel_name as string) ?? '',
    display_field: (config.display_field as string) ?? 'value',
    label_field: (config.label_field as string) ?? 'labels.name',
    agg_func: (config.agg_func as PiePanelConfig['agg_func']) ?? 'sum',
    show_legend: (config.show_legend as boolean | undefined) ?? true,
    show_percentage: (config.show_percentage as boolean | undefined) ?? true,
    max_points: (config.max_points as number) ?? DEFAULT_MAX_POINTS,
  };
}

export default function PieChartPanel({ panelId: _panelId, title, config }: PieChartPanelProps) {
  const cfg = parseConfig(config);
  const { entries, status, closedReason, errorReason } = useChartChannel(
    cfg.channel_name || undefined,
    { maxPoints: Math.max(cfg.max_points ?? DEFAULT_MAX_POINTS, 100) },
  );

  const chartData = useMemo(() => {
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
    return out.map((r) => ({ name: r.label, value: r.value }));
  }, [entries, cfg.label_field, cfg.display_field, cfg.agg_func, cfg.max_points]);

  return (
    <div className="relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4 ring-1 ring-(--color-border-default)">
      <div className="absolute right-3 top-3 z-10">
        <ConnectionStatusIcon status={status} />
      </div>

      <div className="mb-1 flex shrink-0 items-center gap-2 pr-6">
        <PieChartIcon className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
        <span className="truncate text-sm font-semibold text-(--color-text-primary)">
          {title || cfg.channel_name || '채널 미지정'}
        </span>
      </div>

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
              {chartData.map((_, i) => (
                <Cell key={i} fill={PIE_COLORS[i % PIE_COLORS.length]} />
              ))}
            </Pie>
            <Tooltip contentStyle={{ fontSize: '0.75rem' }} />
            {cfg.show_legend && <Legend wrapperStyle={{ fontSize: '0.75rem' }} />}
          </PieChart>
        </ResponsiveContainer>
      </div>

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
