// Stat 패널 (REQ-M4-04).
// 최신 entry 의 display_field 값을 큰 숫자로 표시하고,
// 직전 entry 와의 delta (절대값 + 화살표) 를 부가 표시한다.

import { useMemo } from 'react';
import { clsx } from 'clsx';
import { Hash } from 'lucide-react';

import {
  getByPath,
  type StatPanelConfig,
  type StoreSourceConfig,
} from './chartChannelTypes';
import { ConnectionStatusIcon } from './ConnectionStatusIcon';
import {
  formatNumber,
  pickThresholdColor,
  toNumber,
} from './chartChannelUtils';
import { useChartChannel } from './useChartChannel';
import { useStoreChartData } from './useStoreChartData';

interface StatPanelProps {
  panelId: string;
  title?: string;
  config: Record<string, unknown>;
}

function parseConfig(config: Record<string, unknown>): StatPanelConfig {
  return {
    channel_name: (config.channel_name as string) ?? '',
    display_field: (config.display_field as string) ?? 'value',
    unit: config.unit as string | undefined,
    decimal_places: config.decimal_places as number | undefined,
    threshold_color_rules: config.threshold_color_rules as
      | Array<{ min: number; color: string }>
      | undefined,
    max_points: (config.max_points as number) ?? 2,
  };
}

export default function StatPanel({ panelId: _panelId, title, config }: StatPanelProps) {
  const cfg = parseConfig(config);
  // SPEC-WEB-005: data_source === 'store' 면 Store 소스에서, 그 외에는 기존 채널에서
  // 데이터를 가져온다. 두 훅 모두 항상 호출하고(React 규칙) 비활성 쪽은 idle 로 유지한다.
  const storeSource = config.store_source as StoreSourceConfig | undefined;
  const isStore =
    config.data_source === 'store' && (storeSource?.series?.length ?? 0) > 0;

  const channelRes = useChartChannel(
    isStore ? undefined : cfg.channel_name || undefined,
    { maxPoints: Math.max(cfg.max_points ?? 2, 2) },
  );
  const storeRes = useStoreChartData(isStore ? storeSource : undefined, isStore);

  const { entries, status, closedReason, errorReason } = isStore
    ? storeRes
    : channelRes;

  const { currentValue, deltaText, arrow, color } = useMemo(() => {
    if (entries.length === 0) {
      return {
        currentValue: undefined as number | undefined,
        deltaText: '',
        arrow: '',
        color: undefined as string | undefined,
      };
    }
    const last = entries[entries.length - 1]!;
    const prev = entries.length > 1 ? entries[entries.length - 2] : undefined;
    const v = toNumber(getByPath(last, cfg.display_field ?? 'value'));
    const pv = prev
      ? toNumber(getByPath(prev, cfg.display_field ?? 'value'))
      : NaN;

    let delta = NaN;
    if (Number.isFinite(v) && Number.isFinite(pv)) {
      delta = v - pv;
    }

    let a = '→';
    if (Number.isFinite(delta)) {
      if (delta > 0) a = '↑';
      else if (delta < 0) a = '↓';
    }

    const dec = cfg.decimal_places ?? 2;
    const dText = Number.isFinite(delta)
      ? `${delta > 0 ? '+' : ''}${formatNumber(delta, dec)}`
      : '';

    const c = Number.isFinite(v) ? pickThresholdColor(v, cfg.threshold_color_rules) : undefined;
    return { currentValue: v, deltaText: dText, arrow: a, color: c };
  }, [entries, cfg.display_field, cfg.decimal_places, cfg.threshold_color_rules]);

  const decimals = cfg.decimal_places ?? 2;
  const hasValue = currentValue !== undefined && Number.isFinite(currentValue);

  return (
    <div
      className={clsx(
        'relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4',
        'ring-1 ring-(--color-border-default)',
      )}
    >
      {/* 상단 우측: 연결 상태 아이콘 */}
      <div className="absolute right-3 top-3">
        <ConnectionStatusIcon status={status} />
      </div>

      {/* 헤더: 아이콘 + 타이틀 */}
      <div className="mb-1 flex shrink-0 items-center gap-2 pr-6">
        <Hash className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
        <span className="truncate text-sm font-semibold text-(--color-text-primary)">
          {title || cfg.channel_name || '채널 미지정'}
        </span>
      </div>

      {/* 값 영역 */}
      <div className="flex min-h-0 flex-1 flex-col items-center justify-center">
        <div
          className={clsx('text-4xl font-bold tabular-nums', !color && 'text-(--color-text-primary)')}
          style={color ? { color } : undefined}
          data-testid="stat-value"
        >
          {hasValue ? formatNumber(currentValue!, decimals) : '—'}
          {hasValue && cfg.unit ? (
            <span className="ml-1 text-xl font-medium">{cfg.unit}</span>
          ) : null}
        </div>
        {deltaText && (
          <div
            className={clsx(
              'mt-1 text-sm',
              arrow === '↑' && 'text-emerald-500',
              arrow === '↓' && 'text-rose-500',
              arrow === '→' && 'text-gray-400',
            )}
            data-testid="stat-delta"
          >
            {arrow} {deltaText}
          </div>
        )}
      </div>

      {/* closed / error 오버레이 */}
      {(status === 'closed' || status === 'error') && (
        <div
          className="absolute inset-0 flex items-center justify-center rounded-2xl bg-black/50 p-4 text-center text-sm text-white"
          data-testid="stat-overlay"
        >
          {status === 'closed'
            ? `Channel closed: ${closedReason ?? 'unknown'}`
            : `Error: ${errorReason ?? 'unknown'}`}
        </div>
      )}
    </div>
  );
}
