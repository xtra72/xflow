// thresholdLegend 순수 모듈 테스트 — 임계값 색의 뜻을 화면에 남기는 문구 규칙.

import { describe, it, expect } from 'vitest';

import { thresholdLegendItems, thresholdLegendLabel } from './thresholdLegend';

const entry = (over: Partial<Parameters<typeof thresholdLegendLabel>[0]> = {}) => ({
  name: '',
  color: '#10b981',
  from: 0,
  to: 60,
  ...over,
});

describe('thresholdLegendLabel', () => {
  it('이름이 있으면 이름을 쓴다 — 사용자가 붙인 이름이 곧 그 색의 뜻이다', () => {
    expect(thresholdLegendLabel(entry({ name: '정상' }), undefined)).toBe('정상');
  });

  it('공백뿐인 이름은 이름이 없는 것으로 본다', () => {
    expect(thresholdLegendLabel(entry({ name: '   ' }), undefined)).toBe('0~60');
  });

  it('이름이 없으면 범위를 적는다 — 기본 3구간은 이름이 비어 있다', () => {
    expect(thresholdLegendLabel(entry(), undefined)).toBe('0~60');
  });

  it('자릿수를 지정하면 두 끝에 함께 적용된다', () => {
    expect(thresholdLegendLabel(entry({ from: 0, to: 60 }), undefined, 2)).toBe('0.00~60.00');
  });

  it('자릿수 미지정이면 두 끝의 자릿수를 맞춘다 — 한 줄 안에서 갈리지 않게', () => {
    // 눈금 규칙은 값마다 자릿수를 따로 고른다(0 → 2자리, 60 → 0자리). 그대로 두면
    // `0.00~60` 이 되어 범위가 아니라 서로 다른 두 값처럼 읽힌다.
    expect(thresholdLegendLabel(entry({ from: 0, to: 60 }), undefined)).toBe('0~60');
    expect(thresholdLegendLabel(entry({ from: 0, to: 0.5 }), undefined)).toBe('0.00~0.50');
  });

  it('범위를 읽을 수 없으면 빈 문구 — 색만 남는 줄을 만들지 않는다', () => {
    expect(thresholdLegendLabel(entry({ from: Number.NaN }), undefined)).toBe('');
  });
});

describe('thresholdLegendItems', () => {
  it('구간마다 색과 문구를 만든다', () => {
    expect(
      thresholdLegendItems(
        [
          { name: '정상', color: '#10b981', from: 0, to: 60 },
          { name: '위험', color: '#ef4444', from: 80, to: 100 },
        ],
        undefined,
      ),
    ).toEqual([
      { color: '#10b981', label: '정상' },
      { color: '#ef4444', label: '위험' },
    ]);
  });

  it('문구가 빈 줄은 버린다 — 아무것도 알려주지 않으면서 자리만 차지한다', () => {
    const items = thresholdLegendItems(
      [
        { name: '', color: '#10b981', from: Number.NaN, to: 60 },
        { name: '위험', color: '#ef4444', from: 80, to: 100 },
      ],
      undefined,
    );
    expect(items).toHaveLength(1);
    expect(items[0]!.label).toBe('위험');
  });

  it('구간이 없으면 빈 목록', () => {
    expect(thresholdLegendItems([], undefined)).toEqual([]);
  });
});
