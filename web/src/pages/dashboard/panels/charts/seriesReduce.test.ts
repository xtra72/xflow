// 구간 대표값(series_reduce) 순수 로직 테스트.
//
// SPEC-CHART-002 §2.2 의 7종 정의(max/avg/min/last/sum/count/delta)를
// 7개 엣지 클래스 — 정상 / 표본 0 / 전부 null / 비수치 혼합 / 단일 표본 /
// 불리언 / 음수 — 로 전수 검증한다.
//
// 인수 조건: acceptance.md AC-03 ~ AC-07.
// @spec SPEC-CHART-002

import { describe, it, expect } from 'vitest';

import type { ChartEntry, SeriesReduceFunc } from './chartChannelTypes';
import {
  lastSampleDelta,
  reduceAllSeries,
  reduceSeries,
  SERIES_REDUCE_FUNCS,
} from './seriesReduce';
import type { StoreSeriesStyle } from './useStoreChartData';

/** 버킷 간격이 균일한 시리즈 타임라인을 만든다(공통 픽스처 F1 과 같은 형상). */
function series(values: readonly unknown[]): ChartEntry[] {
  return values.map((value, i) => ({
    timestamp: 1_000 + i * 60_000,
    value,
    labels: { name: 'temp' },
  }));
}

// --- 공통 픽스처 F1 (acceptance.md) — interval_ms 균일, 5버킷 ---
const F1_ROOM1 = series([20, 22, 26, 24, 21]);
const F1_ROOM2 = series([18, null, 19, 19, 23]);
const F1_ROOM3 = series([null, null, null, null, null]);

/** 대표값 7종을 한 번에 계산해 표로 비교하기 위한 헬퍼. */
function reduceAll(entries: ChartEntry[]): Record<SeriesReduceFunc, number | undefined> {
  return {
    max: reduceSeries(entries, 'max'),
    avg: reduceSeries(entries, 'avg'),
    min: reduceSeries(entries, 'min'),
    last: reduceSeries(entries, 'last'),
    sum: reduceSeries(entries, 'sum'),
    count: reduceSeries(entries, 'count'),
    delta: reduceSeries(entries, 'delta'),
  };
}

describe('SERIES_REDUCE_FUNCS', () => {
  it('사용자 선택지는 7종이며 first 는 포함되지 않는다', () => {
    expect(SERIES_REDUCE_FUNCS).toEqual(['max', 'avg', 'min', 'last', 'sum', 'count', 'delta']);
    expect(SERIES_REDUCE_FUNCS).not.toContain('first');
  });
});

describe('reduceSeries', () => {
  // AC-03
  it('max/avg/min/last/sum/count/delta 가 F1 기대치와 일치한다', () => {
    expect(reduceAll(F1_ROOM1)).toEqual({
      max: 26,
      avg: 22.6,
      min: 20,
      last: 21,
      sum: 113,
      count: 5,
      delta: 1,
    });
    expect(reduceAll(F1_ROOM2)).toEqual({
      max: 23,
      avg: 19.75,
      min: 18,
      last: 23,
      sum: 79,
      count: 4,
      delta: 5,
    });
    expect(reduceAll(F1_ROOM3)).toEqual({
      max: undefined,
      avg: undefined,
      min: undefined,
      last: undefined,
      sum: undefined,
      count: 0,
      delta: undefined,
    });
  });

  // AC-03
  it('null 버킷은 표본에서 제외된다 (temp.room2 avg = 19.75)', () => {
    // 5버킷 중 1개가 null 이므로 분모는 5 가 아니라 4 다.
    expect(reduceSeries(F1_ROOM2, 'count')).toBe(4);
    expect(reduceSeries(F1_ROOM2, 'avg')).toBe(19.75);
    // null 을 0 으로 취급했다면 avg 는 79/5 = 15.8 이 된다.
    expect(reduceSeries(F1_ROOM2, 'avg')).not.toBe(15.8);
  });

  it('last 는 배열 순서가 아니라 timestamp 가 가장 큰 표본을 고른다', () => {
    const outOfOrder: ChartEntry[] = [
      { timestamp: 3_000, value: 7 },
      { timestamp: 1_000, value: 1 },
      { timestamp: 2_000, value: 5 },
    ];
    expect(reduceSeries(outOfOrder, 'last')).toBe(7);
    // delta 도 같은 규칙(last − first)을 쓴다.
    expect(reduceSeries(outOfOrder, 'delta')).toBe(6);
  });

  it('timestamp 가 동률이면 last 는 배열상 뒤 항목을 고른다', () => {
    const tie: ChartEntry[] = [
      { timestamp: 1_000, value: 10 },
      { timestamp: 1_000, value: 20 },
    ];
    expect(reduceSeries(tie, 'last')).toBe(20);
  });
});

describe('빈 윈도우', () => {
  // AC-04
  it('count 는 0 을 반환한다', () => {
    expect(reduceSeries(F1_ROOM3, 'count')).toBe(0);
  });

  // AC-04 / UB1-4
  it('sum 은 0 이 아니라 undefined 를 반환한다', () => {
    // 대시보드에서 0 은 "측정했고 결과가 0" 이라는 강한 주장이다(§4.3).
    expect(reduceSeries(F1_ROOM3, 'sum')).toBeUndefined();
  });

  // AC-04
  it('max/avg/min/last/delta 는 undefined 를 반환한다', () => {
    for (const fn of ['max', 'avg', 'min', 'last', 'delta'] as const) {
      expect(reduceSeries(F1_ROOM3, fn)).toBeUndefined();
    }
  });

  // AC-04 (표본 0개 클래스 — 빈 배열)
  it('빈 배열 입력도 전(全) null 윈도우와 동일하게 취급한다', () => {
    expect(reduceAll([])).toEqual(reduceAll(F1_ROOM3));
    expect(reduceSeries([], 'count')).toBe(0);
  });

  it('undefined 입력도 빈 윈도우와 동일하게 취급한다', () => {
    // seriesEntries.get(name) 이 미스일 때 호출자가 별도 분기를 두지 않아도 되게 한다.
    expect(reduceSeries(undefined, 'count')).toBe(0);
    expect(reduceSeries(undefined, 'sum')).toBeUndefined();
  });
});

describe('단일 표본', () => {
  const one = series([42]);

  it('delta 를 제외한 6종은 그 표본 값(또는 개수)을 반환한다', () => {
    expect(reduceAll(one)).toEqual({
      max: 42,
      avg: 42,
      min: 42,
      last: 42,
      sum: 42,
      count: 1,
      delta: undefined,
    });
  });

  it('null 이 섞여 있어도 유효 표본이 1개면 delta 는 undefined 다', () => {
    expect(reduceSeries(series([null, 42, null]), 'count')).toBe(1);
    expect(reduceSeries(series([null, 42, null]), 'delta')).toBeUndefined();
  });
});

describe('delta', () => {
  // AC-05
  it('표본 2개 이상이면 last - first 를 반환한다', () => {
    expect(reduceSeries(series([10, 15, 30]), 'delta')).toBe(20);
    expect(reduceSeries(F1_ROOM1, 'delta')).toBe(1);
  });

  // AC-05
  it('표본 1개면 undefined 를 반환한다 (변화량 미정의)', () => {
    expect(reduceSeries(series([7]), 'delta')).toBeUndefined();
  });

  // AC-05
  it('감소 구간에서 음수를 반환한다', () => {
    expect(reduceSeries(series([30, 20, 5]), 'delta')).toBe(-25);
  });
});

describe('비수치 값', () => {
  // AC-06 — 문자열과 숫자가 섞인 타임라인
  const mixed = series([10, 'abc', 20, null, 30]);

  it('문자열 표본은 제외되고 count 는 수치 표본 수를 센다', () => {
    expect(reduceSeries(mixed, 'count')).toBe(3);
    expect(reduceSeries(mixed, 'sum')).toBe(60);
    expect(reduceSeries(mixed, 'avg')).toBe(20);
  });

  // AC-06
  it('avg === sum / count 항등이 성립한다', () => {
    const sum = reduceSeries(mixed, 'sum');
    const count = reduceSeries(mixed, 'count');
    const avg = reduceSeries(mixed, 'avg');
    expect(sum).toBeDefined();
    expect(count).toBe(3);
    expect(avg).toBe(sum! / count!);
  });

  // AC-06
  it('전부 문자열이면 count 0 + 나머지 undefined', () => {
    expect(reduceAll(series(['a', 'b', 'c']))).toEqual({
      max: undefined,
      avg: undefined,
      min: undefined,
      last: undefined,
      sum: undefined,
      count: 0,
      delta: undefined,
    });
  });

  it('NaN / Infinity / 객체 / 숫자 모양 문자열은 표본이 아니다', () => {
    // Number.isFinite 방어(§5 Secured). '3' 을 3 으로 강제 변환하지 않는다(UB1-5).
    const junk = series([NaN, Infinity, -Infinity, { a: 1 }, '3', undefined]);
    expect(reduceSeries(junk, 'count')).toBe(0);
    expect(reduceSeries(junk, 'sum')).toBeUndefined();
  });

  it('원시 boolean 값은 표본이 아니다 (1/0 변환은 store 변환 계층의 책임)', () => {
    // useStoreChartData 에 도달하기 전 store.ts 가 boolean → 1/0 으로 정규화한다.
    // 정규화되지 않은 raw boolean 이 흘러들어오면 조용히 1 로 세지 않고 제외한다.
    expect(reduceSeries(series([true, false, true]), 'count')).toBe(0);
  });
});

describe('불리언 시리즈', () => {
  // AC-07 — data_type='boolean' 시리즈는 1/0 으로 도착한다(§2.2, 가정 6-2).
  const bool = series([1, 0, 1, 1]);

  it('avg 는 duty ratio 를 반환한다 (0.75)', () => {
    expect(reduceAll(bool)).toEqual({
      max: 1,
      avg: 0.75,
      min: 0,
      last: 1,
      sum: 3,
      count: 4,
      delta: 0,
    });
  });

  // AC-07
  it('delta 는 -1 / 0 / 1 중 하나다', () => {
    expect(reduceSeries(series([0, 0, 1]), 'delta')).toBe(1); // 거짓 → 참
    expect(reduceSeries(series([1, 1, 0]), 'delta')).toBe(-1); // 참 → 거짓
    expect(reduceSeries(series([1, 0, 1]), 'delta')).toBe(0); // 변화 없음
  });

  it('max 는 한 번이라도 참이면 1, min 은 항상 참일 때만 1 이다', () => {
    expect(reduceSeries(series([0, 0, 1, 0]), 'max')).toBe(1);
    expect(reduceSeries(series([0, 0, 0, 0]), 'max')).toBe(0);
    expect(reduceSeries(series([1, 1, 1]), 'min')).toBe(1);
    expect(reduceSeries(series([1, 0, 1]), 'min')).toBe(0);
  });
});

describe('음수 값', () => {
  const neg = series([-5, -10, 3]);

  it('음수를 포함한 표본을 그대로 접는다', () => {
    expect(reduceAll(neg)).toEqual({
      max: 3,
      avg: -4,
      min: -10,
      last: 3,
      sum: -12,
      count: 3,
      delta: 8,
    });
  });

  it('전부 음수면 max 도 음수다 (0 으로 clamp 하지 않는다)', () => {
    expect(reduceSeries(series([-5, -10, -1]), 'max')).toBe(-1);
    expect(reduceSeries(series([-5, -10, -1]), 'sum')).toBe(-16);
  });

  it('-0 은 0 으로 취급한다', () => {
    expect(reduceSeries(series([-0]), 'sum')).toBe(0);
  });
});

// --- reduceAllSeries — 시리즈 순서 보존 + 색상 결합 ---

describe('reduceAllSeries', () => {
  const seriesEntries = new Map<string, ChartEntry[]>([
    ['temp.room1', F1_ROOM1],
    ['temp.room2', F1_ROOM2],
    ['temp.room3', F1_ROOM3],
  ]);
  const seriesNames = ['temp.room1', 'temp.room2', 'temp.room3'];
  const seriesStyles = new Map<string, StoreSeriesStyle>([
    ['temp.room1', { color: '#ff0000' }],
    ['temp.room3', { stroke_width: 3 }],
  ]);

  it('seriesNames 순서를 그대로 보존한다', () => {
    const out = reduceAllSeries(seriesEntries, seriesNames, seriesStyles, 'max');
    expect(out.map((s) => s.name)).toEqual(['temp.room1', 'temp.room2', 'temp.room3']);
  });

  it('값 크기로 재정렬하지 않는다 (UB1-11)', () => {
    // room2(23) 가 room1(26) 보다 작지만 순서는 config 순서를 따른다.
    const out = reduceAllSeries(seriesEntries, seriesNames, seriesStyles, 'max');
    expect(out.map((s) => s.value)).toEqual([26, 23, undefined]);
  });

  it('시리즈 색상을 결합하고, 미지정이면 undefined 로 남긴다', () => {
    // 팔레트 폴백은 패널별로 다르므로(§2.6) 여기서 채우지 않는다.
    const out = reduceAllSeries(seriesEntries, seriesNames, seriesStyles, 'last');
    expect(out[0]).toEqual({ name: 'temp.room1', value: 21, color: '#ff0000' });
    expect(out[1]?.color).toBeUndefined();
    expect(out[2]?.color).toBeUndefined();
  });

  it('대표값이 undefined 인 시리즈도 슬롯을 유지한다', () => {
    const out = reduceAllSeries(seriesEntries, seriesNames, seriesStyles, 'avg');
    expect(out).toHaveLength(3);
    expect(out[2]).toEqual({ name: 'temp.room3', value: undefined, color: undefined });
  });

  it('count 는 빈 시리즈에서도 0 을 낸다', () => {
    const out = reduceAllSeries(seriesEntries, seriesNames, seriesStyles, 'count');
    expect(out.map((s) => s.value)).toEqual([5, 4, 0]);
  });

  it('타임라인이 없는 이름은 빈 윈도우로 취급한다', () => {
    const out = reduceAllSeries(new Map(), ['ghost'], new Map(), 'max');
    expect(out).toEqual([{ name: 'ghost', value: undefined, color: undefined }]);
  });

  it('표시 이름이 중복되면 출력도 하나로 합쳐진다 (가정 6-6)', () => {
    // useStoreChartData 가 같은 이름의 타임라인을 이미 하나로 병합하므로
    // 같은 이름을 두 번 렌더하면 같은 값이 두 번 나온다.
    const out = reduceAllSeries(seriesEntries, ['temp.room1', 'temp.room1'], seriesStyles, 'max');
    expect(out).toHaveLength(1);
    expect(out[0]?.name).toBe('temp.room1');
  });

  it('시리즈 0개는 빈 배열이다 (오류가 아니다)', () => {
    expect(reduceAllSeries(seriesEntries, [], seriesStyles, 'max')).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// SPEC-CHART-003 — 직전 표본 대비 변화량.
// ---------------------------------------------------------------------------
describe('lastSampleDelta (SPEC-CHART-003 AC-14)', () => {
  const e = (timestamp: number, value: unknown) => ({ timestamp, value });

  it('마지막 표본에서 직전 표본을 뺀다 — 구간 시작 대비가 아니다', () => {
    // 20 22 26 24 21 → last(21) − prev(24) = -3. first 대비였다면 +1 이다.
    const entries = [e(1, 20), e(2, 22), e(3, 26), e(4, 24), e(5, 21)];
    expect(lastSampleDelta(entries)).toBe(-3);
    expect(reduceSeries(entries, 'delta')).toBe(1);
  });

  it('비수치·null 은 표본이 아니므로 건너뛴다', () => {
    // 18 null 19 19 23 → 표본은 18 19 19 23, 직전은 19 → +4.
    expect(
      lastSampleDelta([e(1, 18), e(2, null), e(3, 19), e(4, 19), e(5, 23)]),
    ).toBe(4);
  });

  it('표본이 2개 미만이면 undefined 다(0 이 아니다)', () => {
    expect(lastSampleDelta([])).toBeUndefined();
    expect(lastSampleDelta([e(1, 5)])).toBeUndefined();
    expect(lastSampleDelta([e(1, null), e(2, 'x')])).toBeUndefined();
    expect(lastSampleDelta(undefined)).toBeUndefined();
  });

  it('변화가 없으면 0 이다', () => {
    expect(lastSampleDelta([e(1, 7), e(2, 7)])).toBe(0);
  });

  it('timestamp 정렬이 깨져 있어도 시각으로 판정한다', () => {
    // 배열 순서는 뒤섞였지만 마지막은 t=5(21), 직전은 t=4(24).
    expect(lastSampleDelta([e(5, 21), e(1, 20), e(4, 24), e(2, 22)])).toBe(-3);
  });
});
