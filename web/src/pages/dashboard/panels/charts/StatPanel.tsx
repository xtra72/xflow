// Stat 패널 (REQ-M4-04).
// 최신 entry 의 display_field 값을 큰 숫자로 표시하고,
// 직전 entry 와의 delta (절대값 + 화살표) 를 부가 표시한다.
//
// SPEC-CHART-002: Store 모드 + `series_reduce` 지정 시에는 시리즈당 대표값 타일 1개를
// 그리는 다중 출력 경로로 갈린다. 분기는 아래 `derived` useMemo 진입부 한 곳뿐이며,
// `series_reduce` 부재는 "기본값 last" 가 아니라 **레거시 경로**를 뜻한다(§2.9 [S1]).

import { useMemo } from 'react';
import { clsx } from 'clsx';
import { Hash } from 'lucide-react';

import {
  getByPath,
  type ChartEntry,
  type StatPanelConfig,
} from './chartChannelTypes';
import { ConnectionStatusIcon } from './ConnectionStatusIcon';
import {
  formatNumber,
  pickThresholdColor,
  toNumber,
} from './chartChannelUtils';
import { reduceAllSeries, type ReducedSeries } from './seriesReduce';
import { SeriesTileGrid } from './SeriesTileGrid';
import { useChartChannel } from './useChartChannel';
import { type StoreSeriesStyle } from './useStoreChartData';
import { resolvePanelSourceBinding } from './panelDataSource';
import { isPanelSeriesSource, usePanelSeriesData } from './usePanelSeriesData';
import { usePanelTitleVisible } from '../../panelChromeContext';

interface StatPanelProps {
  panelId: string;
  title?: string;
  config: Record<string, unknown>;
}

/** 시리즈 축이 없는 경로(채널 모드)에서 쓰는 빈 기본값 — 매 렌더 새 객체를 만들지 않는다. */
const EMPTY_SERIES_ENTRIES: ReadonlyMap<string, ChartEntry[]> = new Map();
const EMPTY_SERIES_STYLES: ReadonlyMap<string, StoreSeriesStyle> = new Map();
const EMPTY_SERIES_NAMES: readonly string[] = [];

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
    // SPEC-CHART-002 — 유무가 곧 렌더 경로 스위치다. 기본값을 채우지 않는다.
    series_reduce: config.series_reduce as StatPanelConfig['series_reduce'],
    multi_output_limit: config.multi_output_limit as number | undefined,
  };
}

/** 레거시(단일 값) 경로의 파생 결과. */
interface LegacyDerived {
  mode: 'legacy';
  currentValue: number | undefined;
  deltaText: string;
  arrow: string;
  color: string | undefined;
}

/** 다중 출력(대표값 타일) 경로의 파생 결과. */
interface ReduceDerived {
  mode: 'reduce';
  tiles: ReducedSeries[];
}

export default function StatPanel({ panelId: _panelId, title, config }: StatPanelProps) {
  const showTitle = usePanelTitleVisible();
  const cfg = parseConfig(config);
  // SPEC-WEB-005: data_source === 'store' 면 Store 소스에서, 그 외에는 기존 채널에서
  // 데이터를 가져온다. 두 훅 모두 항상 호출하고(React 규칙) 비활성 쪽은 idle 로 유지한다.
  // SPEC-TSDB-002 §2.3 [U3]: 소스 판정은 `panelDataSource` 계약이 소유한다. 패널은
  // `data_source` 를 직접 비교하지 않는다 — 소스 종류가 늘어도 이 지점이 종류만큼
  // 곱해지지 않게 하기 위함이다(UB1-1).
  // `isStore` 는 "채널이 아닌 시리즈 소스가 활성인가" 를 뜻한다. M3 시점에는 store 만
  // 그 조건을 만족하며, tsdb 는 M6 에서 같은 이름을 통해 합류한다.
  const isStore = isPanelSeriesSource(resolvePanelSourceBinding(config));

  const channelRes = useChartChannel(
    isStore ? undefined : cfg.channel_name || undefined,
    { maxPoints: Math.max(cfg.max_points ?? 2, 2) },
  );
  const storeRes = usePanelSeriesData(config);

  const { entries, status, closedReason, errorReason } = isStore
    ? storeRes
    : channelRes;

  // 시리즈 축은 Store 경로에만 존재한다. 채널 경로에서는 빈 기본값을 쓴다.
  const seriesEntries = storeRes.seriesEntries ?? EMPTY_SERIES_ENTRIES;
  const seriesNames = storeRes.seriesNames ?? EMPTY_SERIES_NAMES;
  const seriesStyles = storeRes.seriesStyles ?? EMPTY_SERIES_STYLES;

  // SPEC-CHART-002 §2.9 [S1] — 데이터 파생의 유일한 분기점.
  const isReduceMode = isStore && cfg.series_reduce !== undefined;

  const derived = useMemo<LegacyDerived | ReduceDerived>(() => {
    if (isReduceMode && cfg.series_reduce !== undefined) {
      // 다중 출력 경로: 시리즈 순서 그대로 대표값 1개씩. 보조 delta 줄은 없다(OQ5).
      return {
        mode: 'reduce',
        tiles: reduceAllSeries(seriesEntries, seriesNames, seriesStyles, cfg.series_reduce),
      };
    }

    // --- 레거시 경로(변경 금지 — 특성화 CH-01~CH-04 가 지킨다) ---
    if (entries.length === 0) {
      return {
        mode: 'legacy',
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
    return { mode: 'legacy', currentValue: v, deltaText: dText, arrow: a, color: c };
  }, [
    isReduceMode,
    cfg.series_reduce,
    seriesEntries,
    seriesNames,
    seriesStyles,
    entries,
    cfg.display_field,
    cfg.decimal_places,
    cfg.threshold_color_rules,
  ]);

  const decimals = cfg.decimal_places ?? 2;
  // 다중 출력 경로에서 시리즈가 0개면 기존 빈 상태(—)를 그대로 보여준다(§2.4).
  const tiles = derived.mode === 'reduce' ? derived.tiles : null;
  const showTiles = tiles !== null && tiles.length > 0;
  const legacy = derived.mode === 'legacy' ? derived : null;
  const currentValue = legacy?.currentValue;
  const deltaText = legacy?.deltaText ?? '';
  const arrow = legacy?.arrow ?? '';
  const color = legacy?.color;
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
      {showTitle && (
        <div className="mb-1 flex shrink-0 items-center gap-2 pr-6">
          <Hash className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
          <span className="truncate text-sm font-semibold text-(--color-text-primary)">
            {title || cfg.channel_name || '채널 미지정'}
          </span>
        </div>
      )}

      {/* 값 영역 */}
      {showTiles ? (
        <div className="flex min-h-0 flex-1 flex-col justify-center" data-testid="stat-tiles">
          <SeriesTileGrid
            items={tiles}
            limit={cfg.multi_output_limit}
            itemKey={(tile, i) => `${i}:${tile.name}`}
            renderItem={(tile) => (
              <StatTile tile={tile} cfg={cfg} decimals={decimals} single={tiles.length === 1} />
            )}
          />
        </div>
      ) : (
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
      )}

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

/**
 * 대표값 타일 1개 — 시리즈 라벨 + 값(+단위).
 *
 * 색상 축 분리(§2.6): **값 숫자 색**은 기존 `threshold_color_rules` 가 계속 소유하고,
 * **시리즈 색**은 라벨에만 적용한다. 시리즈 색이 없으면 기본 라벨색을 쓴다(팔레트 폴백
 * 없음 — 팔레트는 bar/pie 의 채움색 규칙이다).
 */
function StatTile({
  tile,
  cfg,
  decimals,
  single,
}: {
  tile: ReducedSeries;
  cfg: StatPanelConfig;
  decimals: number;
  /** 타일이 1개뿐이면 값 글자 크기를 기존 단일 출력과 맞춘다(§2.4 — N=1 외형 보존). */
  single: boolean;
}) {
  const hasValue = tile.value !== undefined && Number.isFinite(tile.value);
  const valueColor = hasValue
    ? pickThresholdColor(tile.value!, cfg.threshold_color_rules)
    : undefined;
  return (
    <>
      <span
        data-testid="stat-tile-label"
        title={tile.name}
        className={clsx(
          'w-full truncate text-center text-xs font-medium',
          !tile.color && 'text-(--color-text-muted)',
        )}
        style={tile.color ? { color: tile.color } : undefined}
      >
        {tile.name}
      </span>
      <span
        data-testid="stat-tile-value"
        className={clsx(
          'w-full truncate text-center font-bold tabular-nums',
          single ? 'text-4xl' : 'text-2xl',
          !valueColor && 'text-(--color-text-primary)',
        )}
        style={valueColor ? { color: valueColor } : undefined}
      >
        {hasValue ? formatNumber(tile.value!, decimals) : '—'}
        {hasValue && cfg.unit ? (
          <span className={clsx('ml-1 font-medium', single ? 'text-xl' : 'text-sm')}>
            {cfg.unit}
          </span>
        ) : null}
      </span>
    </>
  );
}
