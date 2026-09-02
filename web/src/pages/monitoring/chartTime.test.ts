// 차트 시간축 유틸 테스트.
//
// 핵심 계약: 축은 "모인 데이터 범위"가 아니라 "표시 구간"을 그린다. 이 계약이
// 깨지면 시작 직후 축이 좁았다가 데이터가 쌓이며 넓어진다.

import { describe, it, expect } from 'vitest';

import { formatClock, sliceToWindow, windowDomain } from './chartTime';
import type { MetricDataPoint } from './MetricsChart';

/** 로컬 시간 기준 epoch ms */
function at(h: number, m: number, s = 0): number {
  return new Date(2026, 0, 15, h, m, s).getTime();
}

function pt(ts: number, value = 1): MetricDataPoint {
  return { ts, value };
}

describe('formatClock', () => {
  it('epoch 을 HH:MM:SS 로 그린다', () => {
    expect(formatClock(at(9, 5, 3))).toBe('09:05:03');
  });

  it('비정상 값은 빈 문자열 (축 눈금이 NaN 으로 새지 않게)', () => {
    expect(formatClock(Number.NaN)).toBe('');
    expect(formatClock(Number.POSITIVE_INFINITY)).toBe('');
  });
});

describe('windowDomain', () => {
  const WINDOW = 300_000; // 5분

  it('표본이 하나뿐이어도 구간 전체를 그린다', () => {
    // 이것이 이번 수정의 핵심 — 데이터 범위(0)가 아니라 구간(5분)이 축 폭이다.
    const [start, end] = windowDomain([[pt(at(12, 0))]], WINDOW);

    expect(end).toBe(at(12, 0));
    expect(end - start).toBe(WINDOW);
  });

  it('표본이 없으면 기준 시각을 끝으로 삼는다', () => {
    const now = at(12, 0);
    const [start, end] = windowDomain([[]], WINDOW, now);

    expect(end).toBe(now);
    expect(end - start).toBe(WINDOW);
  });

  it('끝은 가장 최근 표본이다 (현재 시각이 아니라)', () => {
    // 현재 시각을 쓰면 데이터가 끊긴 동안에도 축만 계속 흘러간다.
    const now = at(13, 0);
    const [, end] = windowDomain([[pt(at(12, 0)), pt(at(12, 1))]], WINDOW, now);

    expect(end).toBe(at(12, 1));
  });

  it('여러 계열 중 가장 최근 표본을 끝으로 삼는다', () => {
    const [, end] = windowDomain(
      [[pt(at(12, 0))], [pt(at(12, 5))], []],
      WINDOW,
    );

    expect(end).toBe(at(12, 5));
  });

  it('구간이 커지면 축도 그만큼 넓어진다', () => {
    const short = windowDomain([[pt(at(12, 0))]], 60_000);
    const long = windowDomain([[pt(at(12, 0))]], 3_600_000);

    expect(short[1] - short[0]).toBe(60_000);
    expect(long[1] - long[0]).toBe(3_600_000);
    // 끝점은 같고 시작만 밀린다.
    expect(short[1]).toBe(long[1]);
  });

  it('구간이 0 이하면 최소 폭을 준다 (축이 무너지지 않게)', () => {
    const [start, end] = windowDomain([[pt(at(12, 0))]], 0);
    expect(end - start).toBeGreaterThan(0);
  });
});

describe('sliceToWindow', () => {
  const domain: [number, number] = [at(12, 0), at(12, 5)];

  it('구간 밖의 오래된 포인트를 버린다', () => {
    const points = [pt(at(11, 59)), pt(at(12, 1)), pt(at(12, 3))];

    expect(sliceToWindow(points, domain).map((p) => p.ts)).toEqual([at(12, 1), at(12, 3)]);
  });

  it('경계 시각은 남긴다', () => {
    const points = [pt(at(12, 0)), pt(at(12, 1))];

    expect(sliceToWindow(points, domain)).toHaveLength(2);
  });

  it('자를 것이 없으면 원본 배열을 그대로 돌려준다', () => {
    const points = [pt(at(12, 1))];

    expect(sliceToWindow(points, domain)).toBe(points);
  });

  it('빈 배열은 그대로', () => {
    const points: MetricDataPoint[] = [];
    expect(sliceToWindow(points, domain)).toBe(points);
  });
});
