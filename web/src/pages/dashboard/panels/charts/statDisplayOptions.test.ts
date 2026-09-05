// statDisplayOptions 단위 테스트.
//
// @spec SPEC-CHART-003 AC-08 / AC-12 / AC-18 / AC-25

import { describe, it, expect } from 'vitest';

import {
  DEFAULT_DELTA_COLORS,
  readDeltaColors,
  readDeltaEnabled,
  readSubValueScale,
  readWindowStats,
} from './statDisplayOptions';

describe('readDeltaEnabled — 경로 의존 기본값 (AC-08 / spec.md §5 D2)', () => {
  it('미지정 + legacy → true (레거시 경로는 원래 항상 그렸다)', () => {
    expect(readDeltaEnabled({}, 'legacy')).toBe(true);
    expect(readDeltaEnabled({ delta_display: {} }, 'legacy')).toBe(true);
  });

  it('미지정 + tile → false (타일 경로는 원래 한 번도 그리지 않았다)', () => {
    expect(readDeltaEnabled({}, 'tile')).toBe(false);
    expect(readDeltaEnabled({ delta_display: {} }, 'tile')).toBe(false);
  });

  it('명시 true 는 두 경로에서 똑같이 켠다', () => {
    const cfg = { delta_display: { enabled: true } };
    expect(readDeltaEnabled(cfg, 'legacy')).toBe(true);
    expect(readDeltaEnabled(cfg, 'tile')).toBe(true);
  });

  it('명시 false 는 두 경로에서 똑같이 끈다', () => {
    const cfg = { delta_display: { enabled: false } };
    expect(readDeltaEnabled(cfg, 'legacy')).toBe(false);
    expect(readDeltaEnabled(cfg, 'tile')).toBe(false);
  });

  it('delta_display 가 객체가 아니면 미지정으로 취급한다', () => {
    // 손으로 편집한 config 가 경로 기본값을 무너뜨리지 않아야 한다.
    for (const bad of [null, 'x', 3, [], true]) {
      expect(readDeltaEnabled({ delta_display: bad }, 'legacy')).toBe(true);
      expect(readDeltaEnabled({ delta_display: bad }, 'tile')).toBe(false);
    }
  });

  it('enabled 가 불리언이 아니면 미지정으로 취급한다', () => {
    for (const bad of ['true', 1, 0, null]) {
      expect(readDeltaEnabled({ delta_display: { enabled: bad } }, 'legacy')).toBe(true);
      expect(readDeltaEnabled({ delta_display: { enabled: bad } }, 'tile')).toBe(false);
    }
  });
});

describe('readDeltaColors (AC-12)', () => {
  it('미지정은 현행 기본색이다', () => {
    expect(readDeltaColors({})).toEqual(DEFAULT_DELTA_COLORS);
    expect(DEFAULT_DELTA_COLORS.up).toBe('#10b981');
    expect(DEFAULT_DELTA_COLORS.down).toBe('#f43f5e');
    expect(DEFAULT_DELTA_COLORS.flat).toBe('var(--color-text-muted)');
  });

  it('지정한 색만 덮고 나머지는 기본값을 유지한다', () => {
    expect(readDeltaColors({ delta_display: { up_color: '#123456' } })).toEqual({
      up: '#123456',
      down: DEFAULT_DELTA_COLORS.down,
      flat: DEFAULT_DELTA_COLORS.flat,
    });
  });

  it('세 색을 모두 지정하면 모두 반영한다', () => {
    expect(
      readDeltaColors({
        delta_display: { up_color: '#111111', down_color: '#222222', flat_color: '#333333' },
      }),
    ).toEqual({ up: '#111111', down: '#222222', flat: '#333333' });
  });

  it('문자열이 아닌 색은 무시하고 기본값을 쓴다', () => {
    expect(readDeltaColors({ delta_display: { up_color: 42, down_color: null } })).toEqual(
      DEFAULT_DELTA_COLORS,
    );
  });
});

describe('readWindowStats — 고정 순서 (AC-18)', () => {
  it('미지정이면 빈 배열이다(기본은 아무것도 그리지 않음)', () => {
    expect(readWindowStats({})).toEqual([]);
    expect(readWindowStats({ window_stats: {} })).toEqual([]);
  });

  it('켠 항목만 반환한다', () => {
    expect(readWindowStats({ window_stats: { avg: true } })).toEqual(['avg']);
    expect(readWindowStats({ window_stats: { max: true } })).toEqual(['max']);
    expect(readWindowStats({ window_stats: { min: true } })).toEqual(['min']);
  });

  it('순서는 설정 기재 순서가 아니라 avg → max → min 고정이다', () => {
    // min 을 먼저 적었지만 결과는 고정 순서를 따른다.
    expect(readWindowStats({ window_stats: { min: true, avg: true } })).toEqual(['avg', 'min']);
    expect(readWindowStats({ window_stats: { min: true, max: true, avg: true } })).toEqual([
      'avg',
      'max',
      'min',
    ]);
  });

  it('false 와 비불리언은 꺼진 것으로 본다', () => {
    expect(readWindowStats({ window_stats: { avg: false, max: 'true', min: 1 } })).toEqual([]);
  });

  it('window_stats 가 객체가 아니면 빈 배열이다', () => {
    for (const bad of [null, 'x', 3, [], true]) {
      expect(readWindowStats({ window_stats: bad })).toEqual([]);
    }
  });
});

describe('readSubValueScale (AC-25)', () => {
  it('미지정은 1 이다', () => {
    expect(readSubValueScale({})).toBe(1);
  });

  it('readValueScale 과 같은 클램프를 쓴다(하한 0.3 · 상한 3)', () => {
    expect(readSubValueScale({ sub_value_scale: 0.1 })).toBe(0.3);
    expect(readSubValueScale({ sub_value_scale: 5 })).toBe(3);
    expect(readSubValueScale({ sub_value_scale: 1.5 })).toBe(1.5);
  });

  it('비수치는 1 이다', () => {
    for (const bad of ['x', null, undefined, NaN, Infinity, {}]) {
      expect(readSubValueScale({ sub_value_scale: bad })).toBe(1);
    }
  });

  it('본값 배율과 독립이다 — value_scale 은 읽지 않는다 (AC-26)', () => {
    expect(readSubValueScale({ value_scale: 2 })).toBe(1);
  });
});
