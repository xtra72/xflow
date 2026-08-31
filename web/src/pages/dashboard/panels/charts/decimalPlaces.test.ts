// 값 표기 소수점 자릿수 규칙 테스트.
//
// 잠그는 것은 셋이다:
//   - 미지정은 기본값(2)이고, 0 은 "지움"이 아니라 정수 표기 지정이다
//   - 범위를 벗어난 config(음수 · 100 · NaN · 문자열)가 `toFixed` 를 던지지 않는다
//   - 축·눈금이 쓰는 "명시 여부" 판정이 기본값과 구분된다

import { describe, expect, it } from 'vitest';

import {
  DEFAULT_DECIMAL_PLACES,
  MAX_DECIMAL_PLACES,
  formatDecimal,
  hasExplicitDecimalPlaces,
  readDecimalPlaces,
} from './decimalPlaces';

describe('readDecimalPlaces', () => {
  it('미지정이면 기본값이다', () => {
    expect(readDecimalPlaces(undefined)).toBe(DEFAULT_DECIMAL_PLACES);
    expect(readDecimalPlaces({})).toBe(DEFAULT_DECIMAL_PLACES);
    expect(readDecimalPlaces({ decimal_places: undefined })).toBe(DEFAULT_DECIMAL_PLACES);
  });

  it('0 은 지움이 아니라 정수 표기 지정이다', () => {
    // `?? 2` 로 읽으면 0 이 살아남지만, `|| 2` 로 읽으면 조용히 2 가 된다.
    expect(readDecimalPlaces({ decimal_places: 0 })).toBe(0);
  });

  it('지정한 자릿수를 그대로 쓴다', () => {
    expect(readDecimalPlaces({ decimal_places: 3 })).toBe(3);
  });

  it('범위를 벗어난 값은 죈다 (손으로 편집한 config 가 toFixed 를 던지지 않게)', () => {
    expect(readDecimalPlaces({ decimal_places: -1 })).toBe(0);
    expect(readDecimalPlaces({ decimal_places: 999 })).toBe(MAX_DECIMAL_PLACES);
    expect(readDecimalPlaces({ decimal_places: 2.7 })).toBe(2);
  });

  it('수가 아닌 값은 기본값으로 떨어진다', () => {
    expect(readDecimalPlaces({ decimal_places: '3' })).toBe(DEFAULT_DECIMAL_PLACES);
    expect(readDecimalPlaces({ decimal_places: NaN })).toBe(DEFAULT_DECIMAL_PLACES);
    expect(readDecimalPlaces({ decimal_places: null })).toBe(DEFAULT_DECIMAL_PLACES);
  });
});

describe('hasExplicitDecimalPlaces', () => {
  it('미지정과 명시 지정을 가른다', () => {
    // 축 눈금은 이 판정으로 "기본값은 따르지 않고 명시 설정만 따른다" 를 구현한다.
    expect(hasExplicitDecimalPlaces(undefined)).toBe(false);
    expect(hasExplicitDecimalPlaces({})).toBe(false);
    expect(hasExplicitDecimalPlaces({ decimal_places: undefined })).toBe(false);
    expect(hasExplicitDecimalPlaces({ decimal_places: 0 })).toBe(true);
    expect(hasExplicitDecimalPlaces({ decimal_places: 2 })).toBe(true);
  });

  it('수가 아닌 값은 명시로 보지 않는다', () => {
    expect(hasExplicitDecimalPlaces({ decimal_places: '2' })).toBe(false);
    expect(hasExplicitDecimalPlaces({ decimal_places: NaN })).toBe(false);
  });
});

describe('formatDecimal', () => {
  it('지정 자릿수로 끊는다', () => {
    expect(formatDecimal(21.533333333333335, 2)).toBe('21.53');
    expect(formatDecimal(21.5, 0)).toBe('22');
    expect(formatDecimal(10, 2)).toBe('10.00');
  });

  it('음수도 같은 규칙이다', () => {
    expect(formatDecimal(-3.14159, 3)).toBe('-3.142');
  });

  it('유한수가 아니면 원문을 남긴다 (값 없음과 구별할 수 있도록)', () => {
    expect(formatDecimal(NaN, 2)).toBe('NaN');
    expect(formatDecimal(Infinity, 2)).toBe('Infinity');
  });

  it('범위를 벗어난 자릿수도 죈다 — toFixed 가 던지지 않는다', () => {
    expect(() => formatDecimal(1, -5)).not.toThrow();
    expect(() => formatDecimal(1, 1e6)).not.toThrow();
    expect(formatDecimal(1, 999)).toBe((1).toFixed(MAX_DECIMAL_PLACES));
  });
});
