// @spec SPEC-COLOR-001 §결정 3 (M4) — AC-05 · 불변식 I5

import { describe, expect, it } from 'vitest';

import { LEGACY_UNION, PALETTE_ROWS, UNIFIED_PALETTE } from './palette';

describe('UNIFIED_PALETTE', () => {
  it('28칸이다', () => {
    expect(UNIFIED_PALETTE).toHaveLength(28);
  });

  it('모두 6자리 소문자 hex 다 — 팔레트에 알파는 없다', () => {
    for (const c of UNIFIED_PALETTE) {
      expect(c).toMatch(/^#[0-9a-f]{6}$/);
    }
  });

  it('중복이 없다', () => {
    expect(new Set(UNIFIED_PALETTE).size).toBe(UNIFIED_PALETTE.length);
  });

  it('행 구조가 팔레트와 같은 색을 같은 차례로 담는다', () => {
    expect(PALETTE_ROWS.flat()).toEqual([...UNIFIED_PALETTE]);
    expect(PALETTE_ROWS.map((r) => r.length)).toEqual([8, 10, 10]);
  });
});

describe('기존 여섯 배열의 색을 한 색도 잃지 않는다 (I5)', () => {
  it('합집합이 14색이다', () => {
    expect(LEGACY_UNION).toHaveLength(14);
    expect(new Set(LEGACY_UNION).size).toBe(14);
  });

  it.each(LEGACY_UNION)('%s 가 통합 팔레트에 있다', (color) => {
    expect(UNIFIED_PALETTE).toContain(color);
  });

  it('합집합이 실제 기존 배열들과 일치한다 — 표를 베끼지 않고 다시 합친다', () => {
    // 실측한 여섯 배열(gradient 제외). `SUB_COLOR_PRESETS` 는 흰+검+COLOR_PRESETS 이고
    // `LOG_COLOR_PRESETS` 는 `PANEL_COLORS` 의 완전한 사본이다.
    const PANEL_COLORS = [
      '#3b82f6',
      '#8b5cf6',
      '#06b6d4',
      '#10b981',
      '#f59e0b',
      '#ef4444',
      '#ec4899',
      '#6b7280',
    ];
    const COLOR_PRESETS = [
      '#3b82f6',
      '#10b981',
      '#f59e0b',
      '#ef4444',
      '#8b5cf6',
      '#ec4899',
      '#06b6d4',
      '#f97316',
      '#64748b',
      '#0f172a',
    ];
    const SUB_COLOR_PRESETS = ['#ffffff', '#000000', ...COLOR_PRESETS];
    const LOG_COLOR_PRESETS = [...PANEL_COLORS];
    const COLOR_PALETTE = [...COLOR_PRESETS.slice(0, 9), '#94a3b8'];

    const union = new Set([
      ...PANEL_COLORS,
      ...COLOR_PRESETS,
      ...SUB_COLOR_PRESETS,
      ...LOG_COLOR_PRESETS,
      ...COLOR_PALETTE,
    ]);
    expect(union.size).toBe(14);
    expect(new Set(LEGACY_UNION)).toEqual(union);
  });
});
