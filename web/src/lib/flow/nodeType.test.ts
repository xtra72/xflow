// normalizeNodeType 단위 테스트.
//
// 12개 옛 `_` HVAC 타입이 canonical `-` 표기로 정규화되고, 그 외 타입은
// 변경 없이 통과하는지 검증한다.

import { describe, expect, it } from 'vitest';

import { normalizeNodeType } from './nodeType';

describe('normalizeNodeType', () => {
  const cases: Array<[string, string]> = [
    ['samsung_hvacr01', 'samsung-hvacr01'],
    ['samsung_hvacr01_status', 'samsung-hvacr01-status'],
    ['samsung_hvacr01_control', 'samsung-hvacr01-control'],
    ['lg_hvacr01', 'lg-hvacr01'],
    ['lg_hvacr01_status', 'lg-hvacr01-status'],
    ['lg_hvacr01_control', 'lg-hvacr01-control'],
    ['lg_hvacr02', 'lg-hvacr02'],
    ['lg_hvacr02_status', 'lg-hvacr02-status'],
    ['lg_hvacr02_control', 'lg-hvacr02-control'],
    ['century_hvacr01', 'century-hvacr01'],
    ['century_hvacr01_status', 'century-hvacr01-status'],
    ['century_hvacr01_control', 'century-hvacr01-control'],
  ];

  it.each(cases)('옛 `_` 타입 %s 를 canonical %s 로 매핑한다', (input, expected) => {
    expect(normalizeNodeType(input)).toBe(expected);
  });

  it('이미 canonical 인 `-` 타입은 그대로 반환한다', () => {
    expect(normalizeNodeType('samsung-hvacr01-status')).toBe('samsung-hvacr01-status');
    expect(normalizeNodeType('lg-hvacr02')).toBe('lg-hvacr02');
    expect(normalizeNodeType('century-hvacr01-control')).toBe('century-hvacr01-control');
  });

  it('HVAC 가 아닌 타입은 변경 없이 통과한다', () => {
    for (const t of ['filter', 'transform', 'switch', 'bridge', 'flow-node', 'mqtt-subscriber']) {
      expect(normalizeNodeType(t)).toBe(t);
    }
  });

  it('빈 문자열과 미등록 타입도 안전하게 그대로 반환한다', () => {
    expect(normalizeNodeType('')).toBe('');
    expect(normalizeNodeType('unknown_node_type')).toBe('unknown_node_type');
  });
});
