// Bar Chart 패널 (REQ-M4-06).
// 두 모드:
//  - category: label_field 별 최신 값을 막대로 표시
//  - time_bin: bin_sec 간격으로 시간 bin 별 집계 (count/sum/avg)

import { useMemo } from 'react';
import { BarChart3 } from 'lucide-react';
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

import {
  getByPath,
  type BarChartPanelConfig,
  type StoreSourceConfig,
} from './chartChannelTypes';
import { ConnectionStatusIcon } from './ConnectionStatusIcon';
import {
  aggregateByTimeBin,
  formatTimeShort,
  toNumber,
} from './chartChannelUtils';
import { useChartChannel } from './useChartChannel';
import { useStoreChartData } from './useStoreChartData';
import { usePanelTitleVisible } from '../../panelChromeContext';

interface BarChartPanelProps {
  panelId: string;
  title?: string;
  config: Record<string, unknown>;
}

const DEFAULT_MAX_POINTS = 20;

function parseConfig(config: Record<string, unknown>): BarChartPanelConfig {
  return {
    channel_name: (config.channel_name as string) ?? '',
    display_field: (config.display_field as string) ?? 'value',
    label_field: (config.label_field as string) ?? 'labels.name',
    mode: (config.mode as BarChartPanelConfig['mode']) ?? 'category',
    bin_sec: (config.bin_sec as number) ?? 60,
    agg_func: (config.agg_func as BarChartPanelConfig['agg_func']) ?? 'avg',
    max_points: (config.max_points as number) ?? DEFAULT_MAX_POINTS,
  };
}

/** category 모드: label_field 값별 최신 값 추출 */
function buildCategoryData(
  entries: ReturnType<typeof useChartChannel>['entries'],
  labelField: string,
  displayField: string,
): Array<{ label: string; value: number }> {
  // 마지막으로 본 label -> 값 기록 (map 순서 = 삽입 순서)
  const latest = new Map<string, number>();
  for (const e of entries) {
    const lRaw = getByPath(e, labelField);
    const label = lRaw == null ? 'unknown' : String(lRaw);
    const v = toNumber(getByPath(e, displayField));
    latest.set(label, v);
  }
  return Array.from(latest.entries()).map(([label, value]) => ({ label, value }));
}

export default function BarChartPanel({ panelId: _panelId, title, config }: BarChartPanelProps) {
  const showTitle = usePanelTitleVisible();
  const cfg = parseConfig(config);
  // SPEC-WEB-005: data_source 에 따라 Store 소스 또는 채널 소스를 사용한다(공존).
  const storeSource = config.store_source as StoreSourceConfig | undefined;
  const isStore =
    config.data_source === 'store' && (storeSource?.series?.length ?? 0) > 0;

  const channelRes = useChartChannel(
    isStore ? undefined : cfg.channel_name || undefined,
    { maxPoints: Math.max(cfg.max_points ?? DEFAULT_MAX_POINTS, 100) },
  );
  const storeRes = useStoreChartData(isStore ? storeSource : undefined, isStore);

  const { entries, status, closedReason, errorReason } = isStore
    ? storeRes
    : channelRes;

  const chartData = useMemo(() => {
    const displayField = cfg.display_field ?? 'value';
    if (cfg.mode === 'time_bin') {
      const out = aggregateByTimeBin(
        entries,
        displayField,
        cfg.bin_sec ?? 60,
        cfg.agg_func ?? 'avg',
      );
      const max = cfg.max_points ?? DEFAULT_MAX_POINTS;
      const trimmed = out.length > max ? out.slice(-max) : out;
      return trimmed.map((b) => ({ label: formatTimeShort(b.binStart), value: b.value }));
    }
    const cat = buildCategoryData(entries, cfg.label_field ?? 'labels.name', displayField);
    const max = cfg.max_points ?? DEFAULT_MAX_POINTS;
    return cat.length > max ? cat.slice(-max) : cat;
  }, [entries, cfg.mode, cfg.display_field, cfg.label_field, cfg.bin_sec, cfg.agg_func, cfg.max_points]);

  return (
    <div className="relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4 ring-1 ring-(--color-border-default)">
      <div className="absolute right-3 top-3 z-10">
        <ConnectionStatusIcon status={status} />
      </div>

      {showTitle && (
        <div className="mb-1 flex shrink-0 items-center gap-2 pr-6">
          <BarChart3 className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
          <span className="truncate text-sm font-semibold text-(--color-text-primary)">
            {title || cfg.channel_name || '채널 미지정'}
          </span>
        </div>
      )}
      <div className="mb-2 truncate pr-6 text-xs font-medium text-(--color-text-muted)">
        {cfg.channel_name || '채널 미지정'} · {cfg.mode}
      </div>

      <div className="min-h-0 flex-1" data-testid="bar-chart-container">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={chartData} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
            <XAxis dataKey="label" tick={{ fontSize: 10 }} stroke="#9ca3af" />
            <YAxis tick={{ fontSize: 10 }} stroke="#9ca3af" width={50} />
            <Tooltip contentStyle={{ fontSize: '0.75rem' }} />
            <Bar dataKey="value" fill="#3b82f6" isAnimationActive={false} />
          </BarChart>
        </ResponsiveContainer>
      </div>

      {(status === 'closed' || status === 'error') && (
        <div
          className="absolute inset-0 flex items-center justify-center rounded-2xl bg-black/50 p-4 text-center text-sm text-white"
          data-testid="bar-chart-overlay"
        >
          {status === 'closed'
            ? `Channel closed: ${closedReason ?? 'unknown'}`
            : `Error: ${errorReason ?? 'unknown'}`}
        </div>
      )}
    </div>
  );
}
