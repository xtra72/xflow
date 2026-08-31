// 경계 채우기 구간.

import { describe, expect, it } from 'vitest';

import { resolveFillBand, type AxisBound } from './thresholdFill';

const domain = [0, 100] as readonly [AxisBound, AxisBound];

describe('resolveFillBand', () => {
  it('경계 이하는 축 바닥부터 경계까지', () => {
    expect(resolveFillBand({ value: 30, fill_direction: 'below' }, domain)).toEqual({
      y1: 0,
      y2: 30,
    });
  });

  it('경계 이상은 경계부터 축 꼭대기까지', () => {
    expect(resolveFillBand({ value: 30, fill_direction: 'above' }, domain)).toEqual({
      y1: 30,
      y2: 100,
    });
  });

  it('구간이 도메인을 넘지 않는다 — 넘으면 recharts 가 통째로 버린다', () => {
    const band = resolveFillBand({ value: 30, fill_direction: 'below' }, domain)!;
    expect(band.y1).toBeGreaterThanOrEqual(0);
    expect(band.y2).toBeLessThanOrEqual(100);
  });

  it('명시 구간(fill_to)은 양끝을 정렬한다', () => {
    expect(resolveFillBand({ value: 60, fill_to: 20 }, domain)).toEqual({ y1: 20, y2: 60 });
  });

  it('명시 구간도 도메인 안으로 좁힌다', () => {
    expect(resolveFillBand({ value: 500, fill_to: -500 }, domain)).toEqual({ y1: 0, y2: 100 });
  });

  it('채우지 않는 경계는 구간이 없다', () => {
    expect(resolveFillBand({ value: 30 }, domain)).toBeUndefined();
  });

  it('경계가 축 위쪽 밖이면 이하 채우기는 축 전체다', () => {
    expect(resolveFillBand({ value: 500, fill_direction: 'below' }, domain)).toEqual({
      y1: 0,
      y2: 100,
    });
  });

  it('경계가 축 아래 밖이면 이하 채우기는 그리지 않는다 — 두께가 0 이다', () => {
    expect(resolveFillBand({ value: -50, fill_direction: 'below' }, domain)).toBeUndefined();
  });

  it('경계가 축 위쪽 밖이면 이상 채우기는 그리지 않는다', () => {
    expect(resolveFillBand({ value: 500, fill_direction: 'above' }, domain)).toBeUndefined();
  });

  it('도메인이 auto 면 넉넉한 구간을 준다 — 호출부가 잘라 낸다', () => {
    const band = resolveFillBand({ value: 30, fill_direction: 'below' }, ['auto', 'auto'])!;
    expect(band.y2).toBe(30);
    expect(band.y1).toBeLessThan(0);
    // 1e9 같은 극단값은 쓰지 않는다.
    expect(band.y1).toBeGreaterThan(-1e9);
  });

  it('한쪽만 auto 여도 아는 쪽은 도메인을 지킨다', () => {
    const band = resolveFillBand({ value: 30, fill_direction: 'above' }, [0, 'auto'])!;
    expect(band.y1).toBe(30);
    expect(band.y2).toBeGreaterThan(30);
  });
});
