import { describe, it, expect } from 'vitest';

import { buildGapOverlay, gapSeriesKey, GAP_SERIES_PREFIX } from './gapDash';

const K = 'temp';
const G = gapSeriesKey(K);

/** 값 배열을 차트 행으로 만든다. null 은 결측이다. */
function rowsOf(values: Array<number | null>): Array<Record<string, unknown>> {
  return values.map((v, i) => ({ timestamp: i * 1000, [K]: v }));
}

describe('buildGapOverlay', () => {
  it('임계 미만 결측은 손대지 않는다', () => {
    const { rows, gapKeys } = buildGapOverlay(rowsOf([1, null, 3]), [K], 2);
    expect(gapKeys).toEqual([]);
    expect(rows.every((r) => r[G] === undefined)).toBe(true);
  });

  it('임계 이상 결측 구간에 보간 덧그림을 만든다', () => {
    // 0..4 중 1,2,3 결측. 양 끝은 10 과 50.
    const { rows, gapKeys } = buildGapOverlay(rowsOf([10, null, null, null, 50]), [K], 3);
    expect(gapKeys).toEqual([G]);
    // 양 끝은 실측값 그대로여야 원래 선과 이어져 보인다.
    expect(rows[0]![G]).toBe(10);
    expect(rows[4]![G]).toBe(50);
    // 내부는 선형 보간 — 구간이 4 개이므로 10씩 증가한다.
    expect(rows[1]![G]).toBe(20);
    expect(rows[2]![G]).toBe(30);
    expect(rows[3]![G]).toBe(40);
  });

  it('구간이 둘이면 각각 독립적으로 그려진다 (실측 구간에 덧그림이 없다)', () => {
    // 결측: 1,2 와 6,7. 실측 3,4,5 중 **가운데 4** 는 어느 구간에도 닿지 않는다.
    const { rows } = buildGapOverlay(
      rowsOf([0, null, null, 30, 40, 50, null, null, 80]),
      [K],
      2,
    );
    // 3 과 5 는 각 구간의 끝점이라 값이 있고, 4 는 비어 있어야 한다 —
    // 여기까지 덧그림이 이어지면 실측 구간 위에 가짜 점선이 겹친다.
    expect(rows[3]![G]).toBe(30);
    expect(rows[5]![G]).toBe(50);
    expect(rows[4]![G]).toBeUndefined();
    // 두 구간 모두 만들어졌다.
    expect(rows[1]![G]).toBeCloseTo(10);
    expect(rows[6]![G]).toBeCloseTo(60);
  });

  it('선두 결측은 건너뛴다 — 이을 상대가 없다', () => {
    const { rows, gapKeys } = buildGapOverlay(rowsOf([null, null, 30]), [K], 2);
    expect(gapKeys).toEqual([]);
    expect(rows.every((r) => r[G] === undefined)).toBe(true);
  });

  it('말미 결측도 건너뛴다', () => {
    const { rows, gapKeys } = buildGapOverlay(rowsOf([10, null, null]), [K], 2);
    expect(gapKeys).toEqual([]);
    expect(rows.every((r) => r[G] === undefined)).toBe(true);
  });

  it('임계 0 이하면 아무것도 하지 않고 원본을 그대로 돌려준다', () => {
    const input = rowsOf([1, null, null, 4]);
    const { rows, gapKeys } = buildGapOverlay(input, [K], 0);
    expect(rows).toBe(input);
    expect(gapKeys).toEqual([]);
  });

  it('원본 행을 변경하지 않는다', () => {
    const input = rowsOf([1, null, null, 4]);
    buildGapOverlay(input, [K], 2);
    expect(input.every((r) => r[G] === undefined)).toBe(true);
  });

  it('덧그림 키는 원래 키와 겹치지 않는다', () => {
    expect(gapSeriesKey('a')).toBe(`${GAP_SERIES_PREFIX}a`);
    expect(gapSeriesKey('a').startsWith(GAP_SERIES_PREFIX)).toBe(true);
  });

  it('시리즈가 여럿이면 결측이 있는 시리즈만 덧그림을 얻는다', () => {
    const rows = [
      { timestamp: 0, a: 1, b: 1 },
      { timestamp: 1, a: null, b: 2 },
      { timestamp: 2, a: null, b: 3 },
      { timestamp: 3, a: 4, b: 4 },
    ];
    const out = buildGapOverlay(rows, ['a', 'b'], 2);
    expect(out.gapKeys).toEqual([gapSeriesKey('a')]);
  });
});
