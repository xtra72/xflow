// 히트맵 전용 이름있는 gradient 색상 프리셋(SPEC-PANEL-SETTINGS-001 T8).
//
// @spec SPEC-PANEL-SETTINGS-001 (REQ-10/REQ-11)
//
// 각 프리셋은 heatmap config 의 `color_table`(ColorStop[]) 형식과 호환된다. 프리셋 선택은
// heatmap draft config 의 color_table 로 반영되어 미리보기를 갱신한다. 프리셋/커스텀 UI 는
// heatmap 옵션 섹션에서만 노출된다(REQ-12: 차트 5종 미노출).
//
// 기본(default) 프리셋은 기존 `DEFAULT_COLOR_TABLE`(idw.ts) 를 재사용한다(중복 방지).

import type { ColorStop } from './heatmapConfig';
import { DEFAULT_COLOR_TABLE } from './idw';

/** 이름있는 색상 프리셋. */
export interface HeatmapColorPreset {
  /** 안정적 식별자(testid/선택 상태 키). */
  id: string;
  /** 표시명(범용 colormap 고유명 — 로케일 불변). */
  name: string;
  /** 정규화 값(0..1) → 색 정지점 배열. */
  stops: ColorStop[];
}

/**
 * gradient 색상 프리셋 목록(총 5종): 기존 기본 + Viridis / Turbo / Warm / Cool.
 * 각 프리셋은 stop 0..1 을 오름차순으로 커버한다.
 */
export const HEATMAP_COLOR_PRESETS: readonly HeatmapColorPreset[] = [
  { id: 'default', name: 'Default', stops: DEFAULT_COLOR_TABLE },
  {
    id: 'viridis',
    name: 'Viridis',
    stops: [
      { stop: 0, color: '#440154' },
      { stop: 0.25, color: '#3b528b' },
      { stop: 0.5, color: '#21918c' },
      { stop: 0.75, color: '#5ec962' },
      { stop: 1, color: '#fde725' },
    ],
  },
  {
    id: 'turbo',
    name: 'Turbo',
    stops: [
      { stop: 0, color: '#30123b' },
      { stop: 0.25, color: '#28bceb' },
      { stop: 0.5, color: '#a4fc3c' },
      { stop: 0.75, color: '#fb8022' },
      { stop: 1, color: '#7a0403' },
    ],
  },
  {
    id: 'warm',
    name: 'Warm',
    stops: [
      { stop: 0, color: '#400000' },
      { stop: 0.25, color: '#8b0000' },
      { stop: 0.5, color: '#ff4500' },
      { stop: 0.75, color: '#ff8c00' },
      { stop: 1, color: '#ffd700' },
    ],
  },
  {
    id: 'cool',
    name: 'Cool',
    stops: [
      { stop: 0, color: '#08306b' },
      { stop: 0.25, color: '#2171b5' },
      { stop: 0.5, color: '#4292c6' },
      { stop: 0.75, color: '#6baed6' },
      { stop: 1, color: '#c6dbef' },
    ],
  },
];

/**
 * 프리셋의 정지점을 얕은 복제하여 반환한다(config 에 프리셋 원본 참조를 공유하지 않도록).
 * 커스텀 편집이 프리셋 상수를 변형하지 않게 한다.
 */
export function clonePresetStops(preset: HeatmapColorPreset): ColorStop[] {
  return preset.stops.map((s) => ({ ...s }));
}
