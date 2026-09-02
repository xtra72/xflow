// textStyle 순수 모듈 테스트 — 글꼴 토큰·크기·색 해석.

import { describe, it, expect } from 'vitest';

import {
  FONT_FAMILY_OPTIONS,
  FONT_FAMILY_STACKS,
  isChartFontFamily,
  resolveFontColor,
  resolveFontFamily,
  resolveFontSize,
} from './textStyle';

describe('resolveFontFamily', () => {
  it('토큰을 CSS 스택으로 편다', () => {
    expect(resolveFontFamily('mono')).toBe(FONT_FAMILY_STACKS.mono);
    expect(resolveFontFamily('serif')).toContain('Georgia');
  });

  it('미지정·인식 불가 값은 undefined — 임의 글꼴을 씌우지 않고 상속한다', () => {
    expect(resolveFontFamily(undefined)).toBeUndefined();
    expect(resolveFontFamily('comic')).toBeUndefined();
    expect(resolveFontFamily(12)).toBeUndefined();
  });

  it('모든 스택이 총칭 글꼴로 끝난다 — 어느 환경에서도 해석될 안전망', () => {
    for (const stack of Object.values(FONT_FAMILY_STACKS)) {
      expect(stack).toMatch(/(sans-serif|serif|monospace)$/);
    }
  });

  it('설정 선택지와 스택이 1:1 이다 — 고를 수 있으나 못 그리는 값이 없도록', () => {
    expect(FONT_FAMILY_OPTIONS.map((o) => o.value).sort()).toEqual(
      Object.keys(FONT_FAMILY_STACKS).sort(),
    );
  });
});

describe('isChartFontFamily', () => {
  it('토큰만 통과시킨다', () => {
    expect(isChartFontFamily('sans')).toBe(true);
    expect(isChartFontFamily('cursive')).toBe(false);
  });
});

describe('resolveFontSize', () => {
  it('양수만 받는다 — 0·음수는 글자를 지워 되돌릴 수단을 없앤다', () => {
    expect(resolveFontSize(14)).toBe(14);
    expect(resolveFontSize(0)).toBeUndefined();
    expect(resolveFontSize(-3)).toBeUndefined();
    expect(resolveFontSize(Number.NaN)).toBeUndefined();
    expect(resolveFontSize('14')).toBeUndefined();
  });
});

describe('resolveFontColor', () => {
  it('hex 색만 받는다', () => {
    expect(resolveFontColor('#fff')).toBe('#fff');
    expect(resolveFontColor('#A1B2C3')).toBe('#A1B2C3');
    expect(resolveFontColor('  #123456  ')).toBe('#123456');
  });

  it('그 외는 미지정 — 임의 문자열은 브라우저가 조용히 검정으로 떨어뜨린다', () => {
    expect(resolveFontColor('red')).toBeUndefined();
    expect(resolveFontColor('rgb(1,2,3)')).toBeUndefined();
    expect(resolveFontColor(undefined)).toBeUndefined();
  });
});
