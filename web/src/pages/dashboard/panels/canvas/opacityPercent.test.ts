// 불투명도 백분율 환산 (SPEC-CANVAS-012 M5 · AC-24~AC-27).

import { describe, expect, it } from 'vitest';

import {
  OPACITY_PERCENT_MAX,
  opacityToPercentInput,
  percentInputToOpacity,
} from './opacityPercent';

describe('저장 → 칸 (AC-24)', () => {
  it.each([
    [0, 0],
    [0.25, 25],
    [0.5, 50],
    [1, 100],
  ])('%o 이 %o 로 보인다', (stored, shown) => {
    expect(opacityToPercentInput(stored)).toBe(shown);
  });

  it('미지정은 **빈 칸**이다 — 0 이 아니다', () => {
    // 0 으로 보이면 "지정하지 않음" 과 "완전히 투명함" 이 구분되지 않는다.
    expect(opacityToPercentInput(undefined)).toBe('');
  });

  it.each([Number.NaN, Number.POSITIVE_INFINITY])('손상값 %o 도 빈 칸이다', (bad) => {
    expect(opacityToPercentInput(bad)).toBe('');
  });

  it('범위 밖 저장값은 **그려지는 수**로 보인다', () => {
    // 규칙 표가 종전에 죄지 않았으므로 1.5 가 남아 있을 수 있다. `resolveAlpha` 가 1 로
    // 죄어 그리므로, 칸이 150 을 보이면 그려지지 않는 수를 말하게 된다.
    expect(opacityToPercentInput(1.5)).toBe(100);
    expect(opacityToPercentInput(-0.2)).toBe(0);
  });
});

describe('칸 → 저장 (AC-25 · AC-27)', () => {
  it.each([
    ['0', 0],
    ['25', 0.25],
    ['50', 0.5],
    ['100', 1],
  ])('%s 이 %o 로 저장된다', (typed, stored) => {
    expect(percentInputToOpacity(typed)).toBe(stored);
  });

  it.each(['', '   ', 'abc'])('%o 는 미지정이다 — 키를 지운다', (raw) => {
    expect(percentInputToOpacity(raw)).toBeUndefined();
  });

  it.each([
    ['-5', 0],
    ['120', 1],
  ])('범위 밖 %s 은 %o 으로 죈다', (typed, stored) => {
    expect(percentInputToOpacity(typed)).toBe(stored);
  });

  it('소수 백분율은 정수로 반올림한 뒤 나눈다 — `step` 에 기대지 않는다', () => {
    // 붙여넣기와 IME 는 브라우저의 `step` 검증을 지나간다.
    expect(percentInputToOpacity('33.4')).toBe(0.33);
    expect(percentInputToOpacity('33.6')).toBe(0.34);
  });
});

describe('왕복 (AC-26)', () => {
  it.each([0, 0.25, 0.5, 0.75, 1])('%o 이 칸을 지나 그대로 돌아온다', (stored) => {
    // **한 방향만 재면 환산이 한쪽만 맞는 경우를 놓친다.**
    const shown = opacityToPercentInput(stored);
    expect(percentInputToOpacity(String(shown))).toBe(stored);
  });

  it('정수 백분율 전량이 왕복한다', () => {
    for (let pct = 0; pct <= OPACITY_PERCENT_MAX; pct += 1) {
      const stored = percentInputToOpacity(String(pct));
      expect(stored, String(pct)).toBeDefined();
      expect(opacityToPercentInput(stored), String(pct)).toBe(pct);
    }
  });
});
