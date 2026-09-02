// 캔들용 OHLC 조회.
//
// 조회 1회는 집계 하나만 돌려준다. 캔들은 버킷마다 시가·고가·저가·종가가 동시에
// 필요하므로, **같은 소스를 집계만 바꿔 네 번** 조회하고 버킷 기준으로 병합한다.
// 서버 질의 형태를 늘리지 않는 대신 조회가 네 배가 되므로, 캔들이 실제로 쓰일
// 때만(`enabled`) 돌린다.
//
// 훅은 조건부로 호출할 수 없다. 그래서 네 개(소스 종류까지 여덟 개)를 항상 호출하고,
// 쓰지 않는 쪽에는 `undefined` 를 넘겨 idle 로 둔다 — `usePanelSeriesData` 가 store/tsdb
// 를 다루는 방식과 같은 규약이다.

import { useMemo } from 'react';

import { buildCandles, OHLC_AGGREGATIONS, type Candle, type OhlcAggregation } from './candle';
import type { ChartEntry, StoreSourceConfig, TsdbSourceConfig } from './chartChannelTypes';
import { resolvePanelSourceBinding } from './panelDataSource';
import { useStoreChartData } from './useStoreChartData';
import { useTsdbChartData } from './useTsdbChartData';

/** 시리즈 표시 이름 → 그 시리즈의 캔들 목록. */
export type CandleSeries = Map<string, Candle[]>;

const EMPTY: CandleSeries = new Map();

/** 시리즈별 (버킷 → 값) 맵으로 접는다. 숫자가 아닌 값은 버린다. */
function toBucketMap(entries: readonly ChartEntry[]): Map<number, number> {
  const out = new Map<number, number>();
  for (const e of entries) {
    if (typeof e.value === 'number' && Number.isFinite(e.value)) out.set(e.timestamp, e.value);
  }
  return out;
}

/**
 * 캔들 시리즈를 조회한다.
 *
 * `enabled` 가 거짓이면 어떤 조회도 나가지 않는다(빈 맵). 소스가 채널이면 버킷
 * 개념이 없어 캔들을 만들 수 없으므로 역시 조회하지 않는다.
 */
export function useCandleSeriesData(
  config: Record<string, unknown>,
  enabled: boolean,
): CandleSeries {
  const binding = resolvePanelSourceBinding(config);
  const storeOn = enabled && binding.kind === 'store' && binding.active;
  const tsdbOn = enabled && binding.kind === 'tsdb' && binding.active;

  const storeBase = config.store_source as StoreSourceConfig | undefined;
  const tsdbBase = config.tsdb_source as TsdbSourceConfig | undefined;

  // 집계만 바꾼 사본. 원본 config 는 건드리지 않는다.
  const storeAt = (agg: OhlcAggregation): StoreSourceConfig | undefined =>
    storeOn && storeBase ? { ...storeBase, aggregation: agg } : undefined;
  const tsdbAt = (agg: OhlcAggregation): TsdbSourceConfig | undefined =>
    tsdbOn && tsdbBase ? { ...tsdbBase, aggregation: agg } : undefined;

  // 훅은 조건부 호출이 불가하므로 여덟 개를 항상 부른다. 진 쪽은 idle 이다.
  const sFirst = useStoreChartData(storeAt('first'), storeOn);
  const sMax = useStoreChartData(storeAt('max'), storeOn);
  const sMin = useStoreChartData(storeAt('min'), storeOn);
  const sLast = useStoreChartData(storeAt('last'), storeOn);
  const tFirst = useTsdbChartData(tsdbAt('first'), tsdbOn);
  const tMax = useTsdbChartData(tsdbAt('max'), tsdbOn);
  const tMin = useTsdbChartData(tsdbAt('min'), tsdbOn);
  const tLast = useTsdbChartData(tsdbAt('last'), tsdbOn);

  const byAgg = storeOn
    ? { first: sFirst, max: sMax, min: sMin, last: sLast }
    : { first: tFirst, max: tMax, min: tMin, last: tLast };

  return useMemo(() => {
    if (!storeOn && !tsdbOn) return EMPTY;
    // 시가 조회에 나온 시리즈만 후보다 — 네 값이 모두 있어야 캔들이 된다.
    const out: CandleSeries = new Map();
    for (const [name, firstEntries] of byAgg.first.seriesEntries) {
      const maps = {
        first: toBucketMap(firstEntries),
        max: toBucketMap(byAgg.max.seriesEntries.get(name) ?? []),
        min: toBucketMap(byAgg.min.seriesEntries.get(name) ?? []),
        last: toBucketMap(byAgg.last.seriesEntries.get(name) ?? []),
      } satisfies Record<OhlcAggregation, Map<number, number>>;
      const candles = buildCandles(maps);
      if (candles.length > 0) out.set(name, candles);
    }
    return out;
    // 네 결과 객체가 모두 바뀔 때만 다시 만든다.
  }, [storeOn, tsdbOn, byAgg.first, byAgg.max, byAgg.min, byAgg.last]);
}

export { OHLC_AGGREGATIONS };
