// 단일 값 툴팁의 라인 선택 판정.

import { describe, expect, it } from 'vitest';

import { cursorValueAt, pickNearestSeries } from './tooltipSingle';

const plot = { y: 10, height: 100 };
const domain = [0, 100];

describe('cursorValueAt — 커서 픽셀을 값으로', () => {
  it('플롯 위쪽 끝은 도메인 최댓값', () => {
    expect(cursorValueAt(10, plot, domain)).toBe(100);
  });

  it('플롯 아래쪽 끝은 도메인 최솟값', () => {
    expect(cursorValueAt(110, plot, domain)).toBe(0);
  });

  it('가운데는 중간값 — 픽셀은 아래로, 값은 위로 늘어난다', () => {
    expect(cursorValueAt(60, plot, domain)).toBe(50);
  });

  it('음수 도메인도 같은 식으로 다룬다', () => {
    expect(cursorValueAt(60, plot, [-50, 50])).toBe(0);
  });

  it('커서를 모르면 판정하지 않는다', () => {
    expect(cursorValueAt(undefined, plot, domain)).toBeUndefined();
  });

  it('플롯 높이가 0 이면 판정하지 않는다', () => {
    expect(cursorValueAt(10, { y: 0, height: 0 }, domain)).toBeUndefined();
  });

  it('도메인이 숫자가 아니면 판정하지 않는다 — 범주형 축', () => {
    expect(cursorValueAt(60, plot, ['a', 'b'])).toBeUndefined();
  });

  it('도메인 양끝이 같으면 판정하지 않는다 — 나눌 수 없다', () => {
    expect(cursorValueAt(60, plot, [5, 5])).toBeUndefined();
  });

  it('도메인이 없으면 판정하지 않는다', () => {
    expect(cursorValueAt(60, plot, undefined)).toBeUndefined();
  });
});

describe('pickNearestSeries — 가장 가까운 라인 하나', () => {
  const items = [{ v: 10 }, { v: 50 }, { v: 90 }];
  const valueOf = (i: { v: unknown }) => i.v;

  it('커서 값에 가장 가까운 것만 남긴다', () => {
    expect(pickNearestSeries(items, valueOf, 48)).toEqual([{ v: 50 }]);
    expect(pickNearestSeries(items, valueOf, 12)).toEqual([{ v: 10 }]);
    expect(pickNearestSeries(items, valueOf, 1000)).toEqual([{ v: 90 }]);
  });

  it('동률이면 앞선 항목 — 범례 순서와 같아 예측 가능하다', () => {
    expect(pickNearestSeries([{ v: 0 }, { v: 20 }], valueOf, 10)).toEqual([{ v: 0 }]);
  });

  it('항목이 하나면 그대로', () => {
    expect(pickNearestSeries([{ v: 7 }], valueOf, 999)).toEqual([{ v: 7 }]);
  });

  it('커서 값을 모르면 전부 남긴다 — 임의로 고르면 조용한 오답이다', () => {
    expect(pickNearestSeries(items, valueOf, undefined)).toEqual(items);
  });

  it('숫자 항목이 하나도 없으면 전부 남긴다', () => {
    const nonNum = [{ v: null }, { v: 'x' }];
    expect(pickNearestSeries(nonNum, valueOf, 10)).toEqual(nonNum);
  });

  it('숫자가 아닌 항목은 후보에서 뺀다', () => {
    const mixed = [{ v: null }, { v: 50 }, { v: 'x' }];
    expect(pickNearestSeries(mixed, valueOf, 48)).toEqual([{ v: 50 }]);
  });

  it('빈 payload 는 빈 배열', () => {
    expect(pickNearestSeries([], valueOf, 10)).toEqual([]);
    expect(pickNearestSeries(undefined, valueOf, 10)).toEqual([]);
  });
});
