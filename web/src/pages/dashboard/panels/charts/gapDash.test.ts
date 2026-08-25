import { describe, it, expect } from 'vitest';

import {
  buildGapOverlay,
  gapDotSeriesKey,
  gapSeriesKey,
  GAP_SERIES_PREFIX,
} from './gapDash';

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

// ===== 행 자체가 없는 결측 (보고된 결함) =====
//
// `빈 구간 처리 = 채우지 않음`(기본값)이면 서버가 **빈 버킷을 아예 보내지 않는다**.
// 그러면 차트 행에 null 이 남지 않고 행이 통째로 없어서, null 만 보는 판정은
// 결측을 하나도 찾지 못한다. 시간 간격으로 봐야 한다.

describe('buildGapOverlay — 행이 없는 결측', () => {
  /** 주어진 타임스탬프에만 행이 있는 데이터. */
  function sparse(points: Array<[number, number]>): Array<Record<string, unknown>> {
    return points.map(([t, v]) => ({ timestamp: t, [K]: v }));
  }

  it('간격을 주면 빠진 버킷 수로 판정한다', () => {
    // 1분 간격인데 0분과 10분만 있다 → 사이 9개가 빠졌다.
    const { rows, gapKeys } = buildGapOverlay(
      sparse([
        [0, 10],
        [600_000, 50],
      ]),
      [K],
      2,
      60_000,
    );
    expect(gapKeys).toEqual([G]);
    expect(rows[0]![G]).toBe(10);
    expect(rows[1]![G]).toBe(50);
  });

  it('간격만큼만 떨어져 있으면 결측이 아니다', () => {
    const { gapKeys } = buildGapOverlay(
      sparse([
        [0, 10],
        [60_000, 20],
        [120_000, 30],
      ]),
      [K],
      2,
      60_000,
    );
    expect(gapKeys).toEqual([]);
  });

  it('임계 미만으로 빠진 것은 잇지 않는다', () => {
    // 1개만 빠졌는데 임계가 2다.
    const { gapKeys } = buildGapOverlay(
      sparse([
        [0, 10],
        [120_000, 30],
      ]),
      [K],
      2,
      60_000,
    );
    expect(gapKeys).toEqual([]);
  });

  it('간격을 주지 않으면 종전대로 행 인덱스로 센다', () => {
    // 행이 붙어 있으므로 인덱스 기준으로는 결측이 없다.
    const { gapKeys } = buildGapOverlay(
      sparse([
        [0, 10],
        [600_000, 50],
      ]),
      [K],
      2,
    );
    expect(gapKeys).toEqual([]);
  });

  it('행이 있는 결측과 없는 결측이 섞여도 각각 처리한다', () => {
    const rows = [
      { timestamp: 0, [K]: 0 },
      { timestamp: 60_000, [K]: null },
      { timestamp: 120_000, [K]: null },
      { timestamp: 180_000, [K]: 30 },
      // 3분 → 13분: 행 없이 9개 빠짐
      { timestamp: 780_000, [K]: 80 },
    ];
    const out = buildGapOverlay(rows, [K], 2, 60_000);
    expect(out.gapKeys).toEqual([G]);
    // 앞 구간(행이 있는 null)의 보간
    expect(out.rows[1]![G]).toBe(10);
    expect(out.rows[2]![G]).toBe(20);
    // 뒤 구간(행이 없는 결측)의 양 끝
    expect(out.rows[3]![G]).toBe(30);
    expect(out.rows[4]![G]).toBe(80);
  });
});

// ===== 경계 점 표시 (SPEC-TSDB-004 §2.19) =====

describe('buildGapOverlay — 실선/점선 경계 점', () => {
  const D = gapDotSeriesKey(K);

  it('결측 구간의 양 끝에만 점을 찍는다', () => {
    // 1,2,3 결측. 경계는 0(마지막 실측)과 4(다음 실측)다.
    const { rows } = buildGapOverlay(rowsOf([10, null, null, null, 50]), [K], 3);
    expect(rows[0]![D]).toBe(10);
    expect(rows[4]![D]).toBe(50);
    // 구간 내부에는 점이 없다 — 그 자리에는 잰 값이 없다.
    expect(rows[1]![D]).toBeUndefined();
    expect(rows[2]![D]).toBeUndefined();
    expect(rows[3]![D]).toBeUndefined();
  });

  it('결측이 없으면 점도 없다', () => {
    const { rows } = buildGapOverlay(rowsOf([1, 2, 3]), [K], 2);
    expect(rows.every((r) => r[D] === undefined)).toBe(true);
  });

  it('임계 미만 결측에는 점을 찍지 않는다', () => {
    const { rows } = buildGapOverlay(rowsOf([1, null, 3]), [K], 2);
    expect(rows.every((r) => r[D] === undefined)).toBe(true);
  });

  it('구간이 둘이면 경계가 넷이다', () => {
    const { rows } = buildGapOverlay(
      rowsOf([0, null, null, 30, 40, 50, null, null, 80]),
      [K],
      2,
    );
    const marked = rows.map((r, i) => (r[D] !== undefined ? i : -1)).filter((i) => i >= 0);
    expect(marked).toEqual([0, 3, 5, 8]);
  });

  it('행이 없는 결측에서도 경계에 점을 찍는다', () => {
    const { rows } = buildGapOverlay(
      [
        { timestamp: 0, [K]: 10 },
        { timestamp: 600_000, [K]: 50 },
      ],
      [K],
      2,
      60_000,
    );
    expect(rows[0]![D]).toBe(10);
    expect(rows[1]![D]).toBe(50);
  });

  it('점 키는 선 키와도 원래 키와도 겹치지 않는다', () => {
    expect(gapDotSeriesKey('a')).not.toBe(gapSeriesKey('a'));
    expect(gapDotSeriesKey('a')).not.toBe('a');
    expect(gapDotSeriesKey('a').startsWith(GAP_SERIES_PREFIX)).toBe(false);
  });
});
