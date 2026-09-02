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
  pickThresholdColor,
  toNumber,
} from './chartChannelUtils';
import { formatValueWithUnit, scaleValueUnit } from './unitOptions';
import { readValueScale } from './valueScale';

/**
 * 배율 없이 쓰던 Tailwind 글자 크기(px). 배율을 걸려면 수가 필요하므로 클래스에서
 * 꺼내 상수로 둔다 — `text-4xl` 36 · `text-2xl` 24 · `text-xl` 20 · `text-sm` 14.
 *
 * 값과 단위가 **함께** 커져야 한다. 값만 키우면 단위가 상대적으로 작아져 두 글자의
 * 균형이 배율마다 달라진다.
 */
const STAT_VALUE_PX = { value: 36, unit: 20 } as const;
const STAT_TILE_PX = { value: 24, unit: 14 } as const;
import { reduceAllSeries, type ReducedSeries } from './seriesReduce';
import { SeriesTileGrid } from './SeriesTileGrid';
import { type StoreSeriesStyle } from './useStoreChartData';
import { isPanelSeriesActive } from './panelDataSource';
// 값 표기 자릿수는 차트 계열 공용 규칙을 따른다(범위를 벗어난 config 도 여기서 걸린다).
import { readDecimalPlaces } from './decimalPlaces';
import { usePanelSeriesData } from './usePanelSeriesData';
import { usePanelTitleStyle, usePanelTitleVisible } from '../../panelChromeContext';

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
    tile_rows: config.tile_rows as number | undefined,
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
  const titleStyle = usePanelTitleStyle();
  const cfg = parseConfig(config);
  // SPEC-WEB-005: data_source === 'store' 면 Store 소스에서, 그 외에는 기존 채널에서
  // 데이터를 가져온다. 두 훅 모두 항상 호출하고(React 규칙) 비활성 쪽은 idle 로 유지한다.
  // SPEC-TSDB-002 §2.3 [U3]: 소스 판정은 `panelDataSource` 계약이 소유한다. 패널은
  // `data_source` 를 직접 비교하지 않는다 — 소스 종류가 늘어도 이 지점이 종류만큼
  // 곱해지지 않게 하기 위함이다(UB1-1).
  // `isStore` 는 "채널이 아닌 시리즈 소스가 활성인가" 를 뜻한다. M3 시점에는 store 만
  // 그 조건을 만족하며, tsdb 는 M6 에서 같은 이름을 통해 합류한다.
  // 활성 판정은 소스 **목록 전체**를 본다 — 단일 축 해석기로 판정하면 목록 쪽 인스턴스에만
  // 시리즈가 있는 패널이 비활성으로 보인다.
  const isStore = isPanelSeriesActive(config);

  const storeRes = usePanelSeriesData(config);

  const { entries, status, closedReason, errorReason } = storeRes;

  const seriesEntries = storeRes.seriesEntries ?? EMPTY_SERIES_ENTRIES;
  const seriesNames = storeRes.seriesNames ?? EMPTY_SERIES_NAMES;
  const seriesStyles = storeRes.seriesStyles ?? EMPTY_SERIES_STYLES;

  // SPEC-CHART-002 §2.9 [S1] — 데이터 파생의 유일한 분기점.
  const isReduceMode = isStore && cfg.series_reduce !== undefined;

  // 자릿수를 useMemo **밖에서** 계산해 의존성으로 건다. 안에서 `config` 를 읽으면
  // config 객체 전체가 의존성이 되어, 무관한 키가 바뀔 때마다 파생이 다시 돈다.
  const decimals = readDecimalPlaces(config);
  // 현재값 글자 크기 배율. 게이지와 **같은 config 키**를 쓴다 — 패널 유형을 바꿔도
  // "조금 크게" 라는 뜻이 유지된다.
  const valueScale = readValueScale(config.value_scale);

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

    const dec = decimals;
    // 증감도 값과 같은 단위 규칙을 따른다 — 본값은 `1.2GB` 인데 증감만 원시 바이트로
    // 나오면 두 수가 같은 축인지 알 수 없다. 자동 단위는 증감의 크기에 맞춰 접힌다.
    const dText = Number.isFinite(delta)
      ? `${delta > 0 ? '+' : ''}${formatValueWithUnit(delta, dec, cfg.unit)}`
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
    decimals,
    // 증감 표기가 단위 규칙을 따르므로 단위도 재계산 축이다 — 빼면 단위를 바꿔도
    // 증감만 옛 표기에 멈춘다.
    cfg.unit,
    cfg.threshold_color_rules,
  ]);

  // 다중 출력 경로에서 시리즈가 0개면 기존 빈 상태(—)를 그대로 보여준다(§2.4).
  const tiles = derived.mode === 'reduce' ? derived.tiles : null;
  const showTiles = tiles !== null && tiles.length > 0;
  const legacy = derived.mode === 'legacy' ? derived : null;
  const currentValue = legacy?.currentValue;
  const deltaText = legacy?.deltaText ?? '';
  const arrow = legacy?.arrow ?? '';
  const color = legacy?.color;
  const hasValue = currentValue !== undefined && Number.isFinite(currentValue);
  // 자동 데이터 량은 값의 크기가 접미사를 정한다 — 값과 단위를 함께 계산해야
  // `1.21` 옆에 저장값(`auto:bytes`)이 붙는 사고가 나지 않는다.
  const shownUnit = hasValue ? scaleValueUnit(currentValue!, decimals, cfg.unit).suffix : '';

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
          <span className="truncate text-sm font-semibold text-(--color-text-primary)" style={titleStyle}>
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
            rows={cfg.tile_rows}
            itemKey={(tile, i) => `${i}:${tile.name}`}
            renderItem={(tile) => (
              <StatTile
                tile={tile}
                cfg={cfg}
                decimals={decimals}
                valueScale={valueScale}
                single={tiles.length === 1}
              />
            )}
          />
        </div>
      ) : (
        <div className="flex min-h-0 flex-1 flex-col items-center justify-center">
          <div
            className={clsx('font-bold tabular-nums', !color && 'text-(--color-text-primary)')}
            style={{ ...(color ? { color } : {}), fontSize: STAT_VALUE_PX.value * valueScale }}
            data-testid="stat-value"
          >
            {hasValue ? scaleValueUnit(currentValue!, decimals, cfg.unit).text : '—'}
            {hasValue && shownUnit ? (
              <span
                className="ml-1 font-medium"
                style={{ fontSize: STAT_VALUE_PX.unit * valueScale }}
              >
                {shownUnit}
              </span>
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
  valueScale,
  single,
}: {
  tile: ReducedSeries;
  cfg: StatPanelConfig;
  decimals: number;
  /** 현재값 글자 크기 배율(기본 1). */
  valueScale: number;
  /** 타일이 1개뿐이면 값 글자 크기를 기존 단일 출력과 맞춘다(§2.4 — N=1 외형 보존). */
  single: boolean;
}) {
  const hasValue = tile.value !== undefined && Number.isFinite(tile.value);
  // 타일마다 값이 달라 접히는 자리도 다르다 — 타일별로 단위를 정한다.
  const shown = hasValue
    ? scaleValueUnit(tile.value!, decimals, cfg.unit)
    : { text: '', suffix: '' };
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
          !valueColor && 'text-(--color-text-primary)',
        )}
        style={{
          ...(valueColor ? { color: valueColor } : {}),
          fontSize: (single ? STAT_VALUE_PX.value : STAT_TILE_PX.value) * valueScale,
        }}
      >
        {hasValue ? shown.text : '—'}
        {hasValue && shown.suffix ? (
          <span
            className="ml-1 font-medium"
            style={{
              fontSize: (single ? STAT_VALUE_PX.unit : STAT_TILE_PX.unit) * valueScale,
            }}
          >
            {shown.suffix}
          </span>
        ) : null}
      </span>
    </>
  );
}
