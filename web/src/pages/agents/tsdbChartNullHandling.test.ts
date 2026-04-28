// applyNullHandling 단위 테스트.
// 4가지 모드(gap/previous/value/interpolate) 의 변환 동작을 검증한다.
//
// @spec SPEC-WEB-005

import { describe, expect, it } from 'vitest';

import {
  applyNullHandling,
  type ChartRow,
} from './tsdbChartNullHandling';

const COLS = ['a', 'b'] as const;

function makeRows(): ChartRow[] {
  return [
    { bucketStartMs: 0, a: 10, b: null },
    { bucketStartMs: 60_000, a: null, b: 20 },
    { bucketStartMs: 120_000, a: 30, b: null },
    { bucketStartMs: 180_000, a: null, b: 40 },
  ];
}

describe('applyNullHandling - gap', () => {
  it('변환 없이 동일 참조를 반환', () => {
    const rows = makeRows();
    const out = applyNullHandling(rows, COLS, 'gap', 0);
    expect(out).toBe(rows);
  });
});

describe('applyNullHandling - value', () => {
  it('모든 null 을 fillValue 로 대체', () => {
    const rows = makeRows();
    const out = applyNullHandling(rows, COLS, 'value', -1);
    expect(out[0]).toEqual({ bucketStartMs: 0, a: 10, b: -1 });
    expect(out[1]).toEqual({ bucketStartMs: 60_000, a: -1, b: 20 });
    expect(out[2]).toEqual({ bucketStartMs: 120_000, a: 30, b: -1 });
    expect(out[3]).toEqual({ bucketStartMs: 180_000, a: -1, b: 40 });
  });

  it('fillValue=0 도 정상 적용 (falsy 값 안전)', () => {
    const out = applyNullHandling(makeRows(), COLS, 'value', 0);
    expect(out[0]?.b).toBe(0);
    expect(out[1]?.a).toBe(0);
  });

  it('입력 배열을 변경하지 않음 (불변)', () => {
    const rows = makeRows();
    applyNullHandling(rows, COLS, 'value', 999);
    expect(rows[0]?.b).toBeNull();
  });
});

describe('applyNullHandling - previous', () => {
  it('컬럼별 마지막 알려진 값으로 forward-fill', () => {
    const out = applyNullHandling(makeRows(), COLS, 'previous', 0);
    // a: 10, null→10, 30, null→30
    expect(out.map((r) => r.a)).toEqual([10, 10, 30, 30]);
    // b: null(시작), 20, null→20, 40
    expect(out.map((r) => r.b)).toEqual([null, 20, 20, 40]);
  });

  it('leading null 은 알려진 값이 없으면 그대로 둔다', () => {
    const rows: ChartRow[] = [
      { bucketStartMs: 0, x: null },
      { bucketStartMs: 1, x: null },
      { bucketStartMs: 2, x: 5 },
    ];
    const out = applyNullHandling(rows, ['x'], 'previous', 0);
    expect(out[0]?.x).toBeNull();
    expect(out[1]?.x).toBeNull();
    expect(out[2]?.x).toBe(5);
  });
});

describe('applyNullHandling - interpolate', () => {
  it('양쪽으로 알려진 값이 있으면 인덱스 기반 선형 보간', () => {
    const rows: ChartRow[] = [
      { bucketStartMs: 0, x: 10 },
      { bucketStartMs: 1, x: null },
      { bucketStartMs: 2, x: null },
      { bucketStartMs: 3, x: null },
      { bucketStartMs: 4, x: 50 },
    ];
    const out = applyNullHandling(rows, ['x'], 'interpolate', 0);
    // 10 → 50 사이를 4등분: 10, 20, 30, 40, 50
    expect(out.map((r) => r.x)).toEqual([10, 20, 30, 40, 50]);
  });

  it('알려진 값이 인접하면 보간 없이 그대로', () => {
    const rows: ChartRow[] = [
      { bucketStartMs: 0, x: 10 },
      { bucketStartMs: 1, x: 20 },
      { bucketStartMs: 2, x: null },
      { bucketStartMs: 3, x: 40 },
    ];
    const out = applyNullHandling(rows, ['x'], 'interpolate', 0);
    expect(out.map((r) => r.x)).toEqual([10, 20, 30, 40]);
  });

  it('trailing null (다음 알려진 값 없음) 은 forward-fill 로 대체', () => {
    const rows: ChartRow[] = [
      { bucketStartMs: 0, x: 10 },
      { bucketStartMs: 1, x: null },
      { bucketStartMs: 2, x: null },
    ];
    const out = applyNullHandling(rows, ['x'], 'interpolate', 0);
    expect(out.map((r) => r.x)).toEqual([10, 10, 10]);
  });

  it('leading null (이전 알려진 값 없음) 은 그대로 유지', () => {
    const rows: ChartRow[] = [
      { bucketStartMs: 0, x: null },
      { bucketStartMs: 1, x: null },
      { bucketStartMs: 2, x: 30 },
    ];
    const out = applyNullHandling(rows, ['x'], 'interpolate', 0);
    expect(out[0]?.x).toBeNull();
    expect(out[1]?.x).toBeNull();
    expect(out[2]?.x).toBe(30);
  });

  it('전 컬럼이 null 이면 변경 없음', () => {
    const rows: ChartRow[] = [
      { bucketStartMs: 0, x: null },
      { bucketStartMs: 1, x: null },
    ];
    const out = applyNullHandling(rows, ['x'], 'interpolate', 0);
    expect(out.map((r) => r.x)).toEqual([null, null]);
  });

  it('컬럼별로 독립적으로 보간된다', () => {
    const rows: ChartRow[] = [
      { bucketStartMs: 0, a: 0, b: 100 },
      { bucketStartMs: 1, a: null, b: 200 },
      { bucketStartMs: 2, a: 4, b: null },
      { bucketStartMs: 3, a: null, b: 400 },
      { bucketStartMs: 4, a: 8, b: null },
    ];
    const out = applyNullHandling(rows, ['a', 'b'], 'interpolate', 0);
    // a: 0, ?, 4, ?, 8 → 0, 2, 4, 6, 8
    expect(out.map((r) => r.a)).toEqual([0, 2, 4, 6, 8]);
    // b: 100, 200, ?, 400, ? → 100, 200, 300, 400, 400 (마지막은 forward-fill)
    expect(out.map((r) => r.b)).toEqual([100, 200, 300, 400, 400]);
  });
});

describe('applyNullHandling - edge cases', () => {
  it('빈 행 배열은 그대로 반환', () => {
    expect(applyNullHandling([], COLS, 'previous', 0)).toEqual([]);
    expect(applyNullHandling([], COLS, 'value', 5)).toEqual([]);
    expect(applyNullHandling([], COLS, 'interpolate', 0)).toEqual([]);
  });

  it('빈 컬럼 목록은 그대로 반환', () => {
    const rows = makeRows();
    expect(applyNullHandling(rows, [], 'previous', 0)).toBe(rows);
  });

  it('NaN/Infinity 도 알려지지 않은 값으로 취급된다 (interpolate)', () => {
    const rows: ChartRow[] = [
      { bucketStartMs: 0, x: 10 },
      // null 만 채우는 것이 아니라 비유한 수도 무시되어야 안전.
      { bucketStartMs: 1, x: NaN as unknown as number },
      { bucketStartMs: 2, x: 30 },
    ];
    const out = applyNullHandling(rows, ['x'], 'interpolate', 0);
    expect(out[1]?.x).toBe(20);
  });
});
