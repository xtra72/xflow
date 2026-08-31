// 축 크기 산출.
//
// 브라우저 밖에서는 글자 폭을 실제로 잴 수 없어 추정을 쓴다. 여기서 고정하는
// 것은 정확한 픽셀이 아니라 **관계** 다 — 무엇이 커지면 무엇이 따라 커지는가.

import { describe, expect, it } from 'vitest';

import {
  estimateTextWidth,
  MIN_Y_AXIS_WIDTH,
  resolveXAxisHeight,
  resolveYAxisWidth,
} from './axisSize';

describe('estimateTextWidth', () => {
  it('글자 수에 비례한다', () => {
    const one = estimateTextWidth('1', 10);
    expect(estimateTextWidth('123', 10)).toBeCloseTo(one * 3);
  });

  it('글꼴 크기에 비례한다', () => {
    expect(estimateTextWidth('123', 20)).toBeCloseTo(estimateTextWidth('123', 10) * 2);
  });

  it('한글은 숫자보다 넓다 — 열거형 축 라벨이 여기 걸린다', () => {
    expect(estimateTextWidth('정지', 12)).toBeGreaterThan(estimateTextWidth('12', 12));
  });

  it('빈 문자열은 0', () => {
    expect(estimateTextWidth('', 12)).toBe(0);
  });
});

describe('resolveYAxisWidth', () => {
  const base = { tickTexts: ['32.00'], tickFontSize: 12, labelFontSize: 12 };

  it('제목이 있으면 그만큼 더 넓다 — 제목 자리를 눈금에서 뺏지 않는다', () => {
    const without = resolveYAxisWidth(base);
    const withLabel = resolveYAxisWidth({ ...base, label: '온도' });
    expect(withLabel).toBeGreaterThan(without);
  });

  it('제목 글꼴을 키우면 폭이 따라 는다 — 디자인 배지로 키운 경우', () => {
    const small = resolveYAxisWidth({ ...base, label: '온도', labelFontSize: 10 });
    const large = resolveYAxisWidth({ ...base, label: '온도', labelFontSize: 24 });
    expect(large).toBeGreaterThan(small);
  });

  it('눈금이 길어지면 폭이 따라 는다 — 단위·소수 자릿수를 붙인 경우', () => {
    const short = resolveYAxisWidth({ ...base, label: '온도' });
    const long = resolveYAxisWidth({ ...base, tickTexts: ['-1234.567kW'], label: '온도' });
    expect(long).toBeGreaterThan(short);
  });

  it('가장 긴 눈금이 폭을 정한다', () => {
    const a = resolveYAxisWidth({ ...base, tickTexts: ['1', '1234567', '12'] });
    const b = resolveYAxisWidth({ ...base, tickTexts: ['1234567'] });
    expect(a).toBe(b);
  });

  it('열거형 라벨도 자리를 얻는다', () => {
    const num = resolveYAxisWidth({ ...base, tickTexts: ['1'] });
    const enumLabel = resolveYAxisWidth({ ...base, tickTexts: ['비상정지'] });
    expect(enumLabel).toBeGreaterThan(num);
  });

  it('눈금이 없어도 최소 폭은 지킨다', () => {
    expect(resolveYAxisWidth({ ...base, tickTexts: [] })).toBeGreaterThanOrEqual(MIN_Y_AXIS_WIDTH);
  });

  it('종전 고정값(56)이 모자라던 조합에서 더 넓게 잡는다', () => {
    // 제목 글꼴 20 + 단위 붙은 눈금 — 잘림이 보고된 모양.
    const w = resolveYAxisWidth({
      tickTexts: ['1234.56kW'],
      tickFontSize: 12,
      label: '순시전력',
      labelFontSize: 20,
    });
    expect(w).toBeGreaterThan(56);
  });

  it('정수 픽셀을 돌려준다', () => {
    expect(Number.isInteger(resolveYAxisWidth({ ...base, label: '온도' }))).toBe(true);
  });
});

describe('resolveXAxisHeight', () => {
  it('제목이 있으면 더 높다', () => {
    const without = resolveXAxisHeight({ tickFontSize: 12, labelFontSize: 12 });
    const withLabel = resolveXAxisHeight({ tickFontSize: 12, label: '시간', labelFontSize: 12 });
    expect(withLabel).toBeGreaterThan(without);
  });

  it('글꼴을 키우면 따라 높아진다', () => {
    const small = resolveXAxisHeight({ tickFontSize: 12, label: '시간', labelFontSize: 10 });
    const large = resolveXAxisHeight({ tickFontSize: 12, label: '시간', labelFontSize: 28 });
    expect(large).toBeGreaterThan(small);
  });

  it('눈금 글꼴만 키워도 높아진다', () => {
    const small = resolveXAxisHeight({ tickFontSize: 10, labelFontSize: 12 });
    const large = resolveXAxisHeight({ tickFontSize: 28, labelFontSize: 12 });
    expect(large).toBeGreaterThan(small);
  });

  it('최소 높이를 지킨다', () => {
    expect(resolveXAxisHeight({ tickFontSize: 1, labelFontSize: 1 })).toBeGreaterThanOrEqual(30);
  });
});
