// 캔들 조립 판정.

import { describe, expect, it } from 'vitest';

import { buildCandles, candleKey, candleRows, isBullish, type OhlcAggregation } from './candle';

/** 집계별 (버킷 → 값) 맵을 만든다. */
function agg(
  first: Array<[number, number]>,
  max: Array<[number, number]>,
  min: Array<[number, number]>,
  last: Array<[number, number]>,
): Record<OhlcAggregation, Map<number, number>> {
  return { first: new Map(first), max: new Map(max), min: new Map(min), last: new Map(last) };
}

describe('buildCandles', () => {
  it('네 집계를 버킷 기준으로 병합한다', () => {
    const got = buildCandles(agg([[10, 1]], [[10, 5]], [[10, 0]], [[10, 4]]));
    expect(got).toEqual([{ timestamp: 10, open: 1, high: 5, low: 0, close: 4 }]);
  });

  it('시각 오름차순으로 정렬한다', () => {
    const got = buildCandles(
      agg(
        [
          [20, 2],
          [10, 1],
        ],
        [
          [20, 5],
          [10, 5],
        ],
        [
          [20, 0],
          [10, 0],
        ],
        [
          [20, 4],
          [10, 4],
        ],
      ),
    );
    expect(got.map((c) => c.timestamp)).toEqual([10, 20]);
  });

  it('하나라도 빠진 버킷은 버린다 — 메우면 재지 않은 꼬리를 그리게 된다', () => {
    // 고가만 없다.
    expect(buildCandles(agg([[10, 1]], [], [[10, 0]], [[10, 4]]))).toEqual([]);
    // 종가만 없다.
    expect(buildCandles(agg([[10, 1]], [[10, 5]], [[10, 0]], []))).toEqual([]);
  });

  it('숫자가 아닌 값이 섞이면 그 버킷을 버린다', () => {
    expect(buildCandles(agg([[10, NaN]], [[10, 5]], [[10, 0]], [[10, 4]]))).toEqual([]);
  });

  it('고가 < 저가인 버킷은 버린다 — 서로 다른 시점의 조회가 섞인 것이다', () => {
    expect(buildCandles(agg([[10, 1]], [[10, 0]], [[10, 5]], [[10, 4]]))).toEqual([]);
  });

  it('고가 = 저가는 유효하다 — 값이 변하지 않은 버킷', () => {
    const got = buildCandles(agg([[10, 3]], [[10, 3]], [[10, 3]], [[10, 3]]));
    expect(got).toHaveLength(1);
  });

  it('빈 입력은 빈 결과', () => {
    expect(buildCandles(agg([], [], [], []))).toEqual([]);
  });
});

describe('isBullish', () => {
  it('종가가 시가 이상이면 오르막', () => {
    expect(isBullish({ open: 1, close: 2 })).toBe(true);
    expect(isBullish({ open: 2, close: 2 })).toBe(true);
  });

  it('종가가 시가보다 낮으면 내리막', () => {
    expect(isBullish({ open: 2, close: 1 })).toBe(false);
  });
});

describe('candleRows', () => {
  it('축별 값과 몸통 범위를 한 행에 싣는다', () => {
    const [row] = candleRows('temp', [{ timestamp: 10, open: 1, high: 5, low: 0, close: 4 }]);
    expect(row!.timestamp).toBe(10);
    expect(row![candleKey('temp', 'high')]).toBe(5);
    expect(row![candleKey('temp', 'low')]).toBe(0);
    // 몸통은 시가·종가 사이 — 방향과 무관하게 [아래, 위] 다.
    expect(row!['temp']).toEqual([1, 4]);
  });

  it('내리막 캔들도 몸통은 [아래, 위] 순이다', () => {
    const [row] = candleRows('temp', [{ timestamp: 10, open: 4, high: 5, low: 0, close: 1 }]);
    expect(row!['temp']).toEqual([1, 4]);
  });

  it('축 키는 원래 키와 겹치지 않는다', () => {
    expect(candleKey('temp', 'open')).not.toBe('temp');
    const keys = (['open', 'high', 'low', 'close'] as const).map((a) => candleKey('temp', a));
    expect(new Set(keys).size).toBe(4);
  });
});
