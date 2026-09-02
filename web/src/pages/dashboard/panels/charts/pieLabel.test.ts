// pieLabel 순수 모듈 테스트 — 조각 라벨·범례가 공유하는 문구 규칙.

import { describe, it, expect } from 'vitest';

import {
  DEFAULT_PIE_LABEL_MIN_PERCENT,
  formatPiePercent,
  isPieLabelVisible,
  pieLegendValueText,
  piePercents,
  pieSliceLabelText,
  insideLabelPoint,
  sliceLabelTextColor,
} from './pieLabel';

describe('isPieLabelVisible', () => {
  it('기준 미만의 조각은 적지 않는다 — 겹친 라벨은 둘 다 못 읽는다', () => {
    // 보고된 화면: 1% · 3% 조각의 라벨이 같은 자리에 쌓였다.
    expect(isPieLabelVisible(0.01, DEFAULT_PIE_LABEL_MIN_PERCENT)).toBe(false);
    expect(isPieLabelVisible(0.03, DEFAULT_PIE_LABEL_MIN_PERCENT)).toBe(false);
    expect(isPieLabelVisible(0.96, DEFAULT_PIE_LABEL_MIN_PERCENT)).toBe(true);
  });

  it('경계는 포함한다', () => {
    expect(isPieLabelVisible(0.05, 5)).toBe(true);
  });

  it('0이면 모두 적는다 — 종전 동작으로 되돌리는 출구', () => {
    expect(isPieLabelVisible(0.001, 0)).toBe(true);
  });

  it('비율이 수가 아니면 적지 않는다', () => {
    expect(isPieLabelVisible(Number.NaN, 5)).toBe(false);
  });
});

describe('formatPiePercent', () => {
  it('정수 %로 적는다 — 종전 조각 라벨과 같은 규칙', () => {
    expect(formatPiePercent(0.4512)).toBe('45%');
    expect(formatPiePercent(1)).toBe('100%');
    expect(formatPiePercent(0)).toBe('0%');
  });

  it('수가 아니면 0% — NaN% 가 조각에 적히지 않는다', () => {
    expect(formatPiePercent(Number.NaN)).toBe('0%');
    expect(formatPiePercent(Number.POSITIVE_INFINITY)).toBe('0%');
  });
});

describe('piePercents', () => {
  it('합 대비 비중을 돌려준다', () => {
    expect(piePercents([3, 1])).toEqual([0.75, 0.25]);
  });

  it('합이 0이면 모두 0 — 0으로 나눈 NaN 을 흘리지 않는다', () => {
    expect(piePercents([0, 0])).toEqual([0, 0]);
    expect(piePercents([])).toEqual([]);
  });
});

describe('pieSliceLabelText', () => {
  const base = { decimals: 1, unit: 'kW' };

  it('둘 다 끄면 빈 문자열 — 빈 <text> 를 그리지 않는다', () => {
    expect(
      pieSliceLabelText(0.5, 12.34, { ...base, showPercentage: false, showValue: false }),
    ).toBe('');
  });

  it('비율만 켜면 비율만', () => {
    expect(
      pieSliceLabelText(0.5, 12.34, { ...base, showPercentage: true, showValue: false }),
    ).toBe('50%');
  });

  it('값만 켜면 자릿수·단위를 적용한 값만', () => {
    expect(
      pieSliceLabelText(0.5, 12.34, { ...base, showPercentage: false, showValue: true }),
    ).toBe('12.3kW');
  });

  it('둘 다 켜면 한 줄로 잇는다 — 두 줄은 세로 높이가 두 배라 이웃 라벨과 겹친다', () => {
    expect(
      pieSliceLabelText(0.5, 12.34, { ...base, showPercentage: true, showValue: true }),
    ).toBe('50% 12.3kW');
  });

  it('비율에는 단위를 붙이지 않는다 — 비중과 값은 다른 축이다', () => {
    const text = pieSliceLabelText(0.5, 12.34, {
      ...base,
      showPercentage: true,
      showValue: false,
    });
    expect(text).not.toContain('kW');
  });
});

describe('insideLabelPoint', () => {
  const geom = { cx: 100, cy: 100, innerRadius: 0, outerRadius: 50, midAngle: 0 };

  it('반지름의 60% 지점에 놓는다 — 조각 안이라 패널 경계에서 잘리지 않는다', () => {
    // midAngle 0 = 오른쪽(3시). 60% x 50 = 30.
    const p = insideLabelPoint(geom);
    expect(p.x).toBeCloseTo(130);
    expect(p.y).toBeCloseTo(100);
  });

  it('SVG 좌표에 맞춰 각도의 부호를 뒤집는다 — 90도는 위쪽이다', () => {
    const p = insideLabelPoint({ ...geom, midAngle: 90 });
    expect(p.x).toBeCloseTo(100);
    expect(p.y).toBeCloseTo(70);
  });

  it('도넛(innerRadius > 0)이면 두 반지름 사이에 놓는다', () => {
    const p = insideLabelPoint({ ...geom, innerRadius: 30, outerRadius: 50 });
    // 30 + (50-30)*0.6 = 42
    expect(p.x).toBeCloseTo(142);
  });
});

describe('sliceLabelTextColor', () => {
  it('어두운 조각 위에는 흰 글자', () => {
    expect(sliceLabelTextColor('#3b82f6')).toBe('#ffffff');
    expect(sliceLabelTextColor('#8b5cf6')).toBe('#ffffff');
  });

  it('밝은 조각 위에는 검은 글자 — 한 색으로 고정하면 절반은 읽히지 않는다', () => {
    expect(sliceLabelTextColor('#84cc16')).toBe('#111827');
  });

  it('색을 읽을 수 없으면 테마 글자색을 따른다', () => {
    expect(sliceLabelTextColor(undefined)).toBe('currentColor');
    expect(sliceLabelTextColor('red')).toBe('currentColor');
  });
});

describe('pieLegendValueText', () => {
  it('값 표기 규칙은 조각 라벨과 같다', () => {
    expect(pieLegendValueText(12.34, { decimals: 1, unit: 'kW' })).toBe('12.3kW');
    expect(pieLegendValueText(12.34, { decimals: 0 })).toBe('12');
  });
});
