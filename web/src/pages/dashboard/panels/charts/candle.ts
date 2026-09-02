// 캔들 — 버킷마다 시가·고가·저가·종가(OHLC).
//
// 조회 1회는 집계 하나만 돌려준다(`aggregation: min|max|average|...`). 캔들은 네
// 값이 동시에 필요하므로 **같은 구간을 집계만 바꿔 네 번** 조회하고 버킷 기준으로
// 병합한다. 서버 질의 형태를 늘리지 않는 대신 조회가 네 배가 되므로, 캔들을 고른
// 시리즈에만 적용한다.
//
// 캔들은 버킷 개념이 있는 소스(Store · TSDB)에서만 뜻이 있다. 채널(실시간)은
// 버킷이 없어 시·고·저·종을 정의할 수 없다.

/** OHLC 를 만드는 데 쓰는 집계 4종. 순서가 곧 o·h·l·c 다. */
export const OHLC_AGGREGATIONS = ['first', 'max', 'min', 'last'] as const;
export type OhlcAggregation = (typeof OHLC_AGGREGATIONS)[number];

/** 한 버킷의 캔들. 네 값이 모두 있어야 그릴 수 있다. */
export interface Candle {
  timestamp: number;
  open: number;
  high: number;
  low: number;
  close: number;
}

/** 캔들 시리즈의 행 키 — 원래 시리즈 키에 축 접미사를 붙인다. */
export const CANDLE_SUFFIX = {
  open: '__o__',
  high: '__h__',
  low: '__l__',
  close: '__c__',
} as const;

export function candleKey(seriesKey: string, axis: keyof typeof CANDLE_SUFFIX): string {
  return `${seriesKey}${CANDLE_SUFFIX[axis]}`;
}

/**
 * 집계별 타임라인 네 벌을 버킷 기준으로 병합해 캔들을 만든다.
 *
 * 네 값이 **모두** 있는 버킷만 캔들이 된다. 하나라도 비면 그 버킷은 건너뛴다 —
 * 없는 값을 다른 값으로 메우면 실제로 재지 않은 몸통·꼬리를 그리게 된다.
 *
 * 고가 < 저가처럼 앞뒤가 뒤집힌 버킷도 버린다. 집계가 서로 다른 조회에서 왔으므로
 * 폴링 타이밍에 따라 어긋난 조합이 섞일 수 있고, 그대로 그리면 뒤집힌 캔들이 된다.
 */
export function buildCandles(
  byAggregation: Readonly<Record<OhlcAggregation, ReadonlyMap<number, number>>>,
): Candle[] {
  const first = byAggregation.first;
  const out: Candle[] = [];
  for (const [ts, open] of first) {
    const high = byAggregation.max.get(ts);
    const low = byAggregation.min.get(ts);
    const close = byAggregation.last.get(ts);
    if (high === undefined || low === undefined || close === undefined) continue;
    if (!Number.isFinite(open) || !Number.isFinite(high)) continue;
    if (!Number.isFinite(low) || !Number.isFinite(close)) continue;
    // 고가가 저가보다 낮으면 서로 다른 시점의 조회가 섞인 것이다.
    if (high < low) continue;
    out.push({ timestamp: ts, open, high, low, close });
  }
  out.sort((a, b) => a.timestamp - b.timestamp);
  return out;
}

/** 캔들이 오르막인지(종가 ≥ 시가). 몸통 색을 가른다. */
export function isBullish(c: Pick<Candle, 'open' | 'close'>): boolean {
  return c.close >= c.open;
}

/**
 * 캔들을 차트 행에 얹는다.
 *
 * recharts 에 캔들 프리미티브가 없어 `Bar` 의 커스텀 모양으로 그린다. Bar 는
 * 하나의 dataKey 로 높이를 잡으므로, 몸통 범위 `[min(o,c), max(o,c)]` 를 값으로
 * 주고 꼬리(고가·저가)는 모양 함수가 함께 읽도록 같은 행에 실어 둔다.
 */
export function candleRows(seriesKey: string, candles: readonly Candle[]): Array<Record<string, unknown>> {
  return candles.map((c) => ({
    timestamp: c.timestamp,
    [candleKey(seriesKey, 'open')]: c.open,
    [candleKey(seriesKey, 'high')]: c.high,
    [candleKey(seriesKey, 'low')]: c.low,
    [candleKey(seriesKey, 'close')]: c.close,
    // Bar 가 읽는 값 — 몸통의 아래·위. 범위 배열을 주면 recharts 가 그 구간에 막대를 세운다.
    [seriesKey]: [Math.min(c.open, c.close), Math.max(c.open, c.close)] as [number, number],
  }));
}

/** 오르막·내리막 기본색. 시리즈 색을 지정하면 그 색이 몸통을 채운다. */
export const CANDLE_UP_COLOR = '#ef4444';
export const CANDLE_DOWN_COLOR = '#3b82f6';

/**
 * 값 → 픽셀 y 변환기를 몸통 상자에서 역산한다.
 *
 * 몸통의 값 범위 `[lo, hi]` 가 픽셀 `[y+height, y]` 에 대응한다. 두 값이 같으면
 * (높이 0 인 몸통) 기울기를 구할 수 없어 `undefined` 를 돌려준다 — 그 경우 꼬리는
 * 그리지 않는다. 없는 축척으로 그은 선은 거짓이다.
 */
export function makeValueToPixel(
  lo: number,
  hi: number,
  y: number,
  height: number,
): ((v: number) => number) | undefined {
  if (!(hi > lo)) return undefined;
  const scale = height / (hi - lo);
  return (v: number) => y + (hi - v) * scale;
}
