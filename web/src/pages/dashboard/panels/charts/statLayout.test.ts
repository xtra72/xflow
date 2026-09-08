// statLayout 단위 테스트.
//
// @spec SPEC-CHART-004 AC-06 / AC-11 / AC-12 / AC-14

import { describe, it, expect } from 'vitest';

import {
  FONT_SIZE_MAX,
  FONT_SIZE_MIN,
  clampFontSize,
  readStatLayout,
  readStatLayoutFontSize,
  STAT_OFFSET_LIMIT,
  writeStatLayout,
} from './statLayout';

describe('clampFontSize (AC-14)', () => {
  it('범위 안의 수는 그대로다', () => {
    expect(clampFontSize(50)).toBe(50);
    expect(clampFontSize(FONT_SIZE_MIN)).toBe(FONT_SIZE_MIN);
    expect(clampFontSize(FONT_SIZE_MAX)).toBe(FONT_SIZE_MAX);
  });

  it('범위를 벗어나면 죈다', () => {
    expect(clampFontSize(1)).toBe(FONT_SIZE_MIN);
    expect(clampFontSize(500)).toBe(FONT_SIZE_MAX);
  });

  it('수가 아니면 undefined 다 — 0 이나 음수를 그대로 쓰면 글자가 사라진다', () => {
    for (const bad of ['x', null, undefined, NaN, Infinity, {}]) {
      expect(clampFontSize(bad)).toBeUndefined();
    }
  });
});

describe('readStatLayout — 크기 폴백 (AC-11 / AC-12)', () => {
  it('미지정이면 기본 px × 배율이다', () => {
    // 본값 36 · 보조 줄 14 가 기본이고 배율 기본은 1.
    expect(readStatLayout({}, 'value').fontSize).toBe(36);
    expect(readStatLayout({}, 'delta').fontSize).toBe(14);
    expect(readStatLayout({}, 'stats').fontSize).toBe(14);
  });

  it('배율만 있으면 곱한다', () => {
    const cfg = { value_scale: 2, sub_value_scale: 1.5 };
    expect(readStatLayout(cfg, 'value').fontSize).toBe(72);
    expect(readStatLayout(cfg, 'delta').fontSize).toBe(21);
    expect(readStatLayout(cfg, 'stats').fontSize).toBe(21);
  });

  it('AC-11: font_size 는 배율을 이기고 곱해지지 않는다', () => {
    const cfg = { value_scale: 2, value_layout: { font_size: 50 } };
    // 50 이다 — 72(36×2) 도 100(50×2) 도 아니다.
    expect(readStatLayout(cfg, 'value').fontSize).toBe(50);
    // 지정하지 않은 요소는 폴백 그대로.
    expect(readStatLayout(cfg, 'delta').fontSize).toBe(14);
  });

  it('font_size 도 6~160 으로 죈다', () => {
    expect(readStatLayout({ value_layout: { font_size: 1 } }, 'value').fontSize).toBe(FONT_SIZE_MIN);
    expect(readStatLayout({ value_layout: { font_size: 900 } }, 'value').fontSize).toBe(FONT_SIZE_MAX);
  });

  it('비수치 font_size 는 폴백으로 떨어진다', () => {
    expect(readStatLayout({ value_scale: 2, value_layout: { font_size: 'x' } }, 'value').fontSize).toBe(72);
  });

  it('타일 기본 px 은 24 다 — 같은 value 요소라도 기준이 다르다', () => {
    expect(readStatLayout({ value_scale: 2 }, 'value', { tile: true }).fontSize).toBe(48);
    // 타일이 1개뿐이면 단일 출력과 크기를 맞춘다(SPEC-CHART-002 §2.4 N=1 외형 보존).
    expect(readStatLayout({ value_scale: 2 }, 'value', { tile: true, single: true }).fontSize).toBe(72);
  });
});

describe('readStatLayout — 오프셋 (AC-08)', () => {
  it('미지정이면 0 이다', () => {
    const r = readStatLayout({}, 'value');
    expect(r.offsetX).toBe(0);
    expect(r.offsetY).toBe(0);
  });

  it('지정한 값을 읽고 ±50 으로 죈다', () => {
    const r = readStatLayout({ value_layout: { offset_x: 10, offset_y: -60 } }, 'value');
    expect(r.offsetX).toBe(10);
    expect(r.offsetY).toBe(-STAT_OFFSET_LIMIT);
    // 게이지·파이(±40)보다 넓다 — 작은 글자를 패널 어느 모서리에든 놓아야 한다.
    expect(STAT_OFFSET_LIMIT).toBe(50);
  });

  it('비수치는 0 이다', () => {
    const r = readStatLayout({ value_layout: { offset_x: 'x', offset_y: null } }, 'value');
    expect(r.offsetX).toBe(0);
    expect(r.offsetY).toBe(0);
  });

  it('layout 이 객체가 아니면 전부 기본값이다', () => {
    for (const bad of [null, 'x', 3, [], true]) {
      const r = readStatLayout({ value_layout: bad }, 'value');
      expect(r.offsetX).toBe(0);
      expect(r.fontSize).toBe(36);
    }
  });
});

describe('readStatLayout — 글자 스타일', () => {
  it('글꼴 토큰을 CSS 스택으로 편다', () => {
    expect(readStatLayout({ value_layout: { font_family: 'serif' } }, 'value').fontFamily).toContain(
      'serif',
    );
    expect(readStatLayout({}, 'value').fontFamily).toBeUndefined();
  });

  it('색은 #rgb / #rrggbb 만 받는다 — 임의 문자열은 무시한다', () => {
    expect(readStatLayout({ value_layout: { font_color: '#123456' } }, 'value').fontColor).toBe(
      '#123456',
    );
    expect(readStatLayout({ value_layout: { font_color: 'red' } }, 'value').fontColor).toBeUndefined();
  });

  it('굵기는 normal / bold 만 받는다', () => {
    expect(readStatLayout({ value_layout: { font_weight: 'bold' } }, 'value').fontWeight).toBe('bold');
    expect(readStatLayout({ value_layout: { font_weight: 'heavy' } }, 'value').fontWeight).toBeUndefined();
  });

  it('변화량은 글자색을 갖지 않는다 — 방향별 색이 delta_display 소유다 (D4)', () => {
    expect(
      readStatLayout({ delta_layout: { font_color: '#123456' } }, 'delta').fontColor,
    ).toBeUndefined();
  });
});

describe('writeStatLayout', () => {
  it('요소별 키에만 부분 갱신한다', () => {
    expect(writeStatLayout({}, 'value', { offset_x: 10 })).toEqual({
      value_layout: { offset_x: 10 },
    });
  });

  it('기존 값을 보존하며 덮어쓴다', () => {
    const cfg = { value_layout: { offset_x: 10, font_size: 50 } };
    expect(writeStatLayout(cfg, 'value', { offset_y: -5 })).toEqual({
      value_layout: { offset_x: 10, font_size: 50, offset_y: -5 },
    });
  });

  it('다른 요소의 키는 인자에 담지 않는다 (AC-07)', () => {
    const cfg = { value_layout: { offset_x: 10 }, delta_layout: { offset_x: 5 } };
    const patch = writeStatLayout(cfg, 'value', { offset_x: 20 });
    expect(patch).toEqual({ value_layout: { offset_x: 20 } });
    expect(patch).not.toHaveProperty('delta_layout');
  });

  it('모든 하위 필드가 비면 키 자체를 지운다', () => {
    const cfg = { value_layout: { offset_x: 10 } };
    expect(writeStatLayout(cfg, 'value', { offset_x: undefined })).toEqual({
      value_layout: undefined,
    });
  });
});

describe('readStatLayoutFontSize — 직접 지정 여부만 본다 (AC-30)', () => {
  it('지정되지 않았으면 undefined 다 — 폴백을 채우지 않는다', () => {
    // readStatLayout 은 36 을 돌려주지만, 이쪽은 "직접 지정했는가" 를 답한다.
    expect(readStatLayout({}, 'value').fontSize).toBe(36);
    expect(readStatLayoutFontSize({}, 'value')).toBeUndefined();
    expect(readStatLayoutFontSize({ value_scale: 2 }, 'value')).toBeUndefined();
  });

  it('지정된 값을 죄어서 돌려준다', () => {
    expect(readStatLayoutFontSize({ value_layout: { font_size: 50 } }, 'value')).toBe(50);
    expect(readStatLayoutFontSize({ delta_layout: { font_size: 900 } }, 'delta')).toBe(FONT_SIZE_MAX);
    expect(readStatLayoutFontSize({ stats_layout: { font_size: 'x' } }, 'stats')).toBeUndefined();
  });
});
