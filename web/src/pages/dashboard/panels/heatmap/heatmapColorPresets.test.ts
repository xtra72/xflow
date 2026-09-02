// heatmapColorPresets — gradient 프리셋 상수 검증.
// @spec SPEC-PANEL-SETTINGS-001 (T8, REQ-10)

import { describe, expect, it } from 'vitest';

import { clonePresetStops, HEATMAP_COLOR_PRESETS } from './heatmapColorPresets';
import { DEFAULT_COLOR_TABLE } from './idw';

describe('HEATMAP_COLOR_PRESETS', () => {
  it('4~5종(기본 포함 5종) + 예상 id 집합', () => {
    expect(HEATMAP_COLOR_PRESETS.length).toBeGreaterThanOrEqual(4);
    expect(HEATMAP_COLOR_PRESETS.length).toBeLessThanOrEqual(5);
    expect(HEATMAP_COLOR_PRESETS.map((p) => p.id)).toEqual([
      'default',
      'viridis',
      'turbo',
      'warm',
      'cool',
    ]);
  });

  it('각 프리셋은 0..1 을 오름차순으로 커버하는 유효 ColorStop 배열', () => {
    for (const p of HEATMAP_COLOR_PRESETS) {
      expect(p.stops.length).toBeGreaterThanOrEqual(2);
      expect(p.stops[0]!.stop).toBe(0);
      expect(p.stops[p.stops.length - 1]!.stop).toBe(1);
      for (let i = 1; i < p.stops.length; i++) {
        expect(p.stops[i]!.stop).toBeGreaterThan(p.stops[i - 1]!.stop);
      }
      for (const s of p.stops) {
        expect(s.color).toMatch(/^#[0-9a-fA-F]{3,8}$/);
      }
    }
  });

  it('default 프리셋은 기존 DEFAULT_COLOR_TABLE 을 재사용한다', () => {
    const def = HEATMAP_COLOR_PRESETS.find((p) => p.id === 'default')!;
    expect(def.stops).toBe(DEFAULT_COLOR_TABLE);
  });

  it('clonePresetStops 는 값이 같은 새 배열/객체를 반환한다(원본 비공유)', () => {
    const viridis = HEATMAP_COLOR_PRESETS.find((p) => p.id === 'viridis')!;
    const cloned = clonePresetStops(viridis);
    expect(cloned).toEqual(viridis.stops);
    expect(cloned).not.toBe(viridis.stops);
    expect(cloned[0]).not.toBe(viridis.stops[0]);
  });
});
