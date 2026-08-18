// gridGeometry 순수 함수 테스트 — 패널 픽셀 종횡비 / 목표 비율 역산.

import { describe, it, expect } from 'vitest';

import {
  GRID_MARGIN_PX,
  gridCellSize,
  gridHeightForAspect,
  panelPixelAspect,
} from './gridGeometry';

describe('gridCellSize', () => {
  it('정사각형 셀 = (컨테이너 폭 − 마진×(칼럼−1)) / 칼럼', () => {
    // 10칼럼, 마진 16 → 폭 1144 는 셀 100 이 되도록 고른 값(1000 + 16×9).
    expect(gridCellSize(1144, 10)).toBe(100);
  });

  it('미측정(폭 0)이거나 칼럼 수가 0 이면 0', () => {
    expect(gridCellSize(0, 10)).toBe(0);
    expect(gridCellSize(1144, 0)).toBe(0);
  });
});

describe('panelPixelAspect', () => {
  it('마진을 포함해 계산한다 — 단위 비 w/h 와 다르다', () => {
    // 6×4, 셀 100: 폭 = 600 + 16×5 = 680, 높이 = 400 + 16×3 = 448.
    expect(panelPixelAspect(6, 4, 100)).toBeCloseTo(680 / 448, 10);
    expect(panelPixelAspect(6, 4, 100)).not.toBeCloseTo(6 / 4, 3);
  });

  it('정사각 단위(w === h)면 마진이 상쇄되어 정확히 1', () => {
    expect(panelPixelAspect(5, 5, 37)).toBe(1);
  });

  it('셀이 작을수록 단위 비와의 괴리가 커진다(근사가 부적절한 이유)', () => {
    // 1×8, 셀 50: 폭 50, 높이 400 + 16×7 = 512 → 0.0977 (단위 비 0.125 와 28% 차이).
    expect(panelPixelAspect(1, 8, 50)).toBeCloseTo(50 / 512, 10);
  });

  it('셀 미상(0)이면 마진을 무시한 근사 w/h 로 폴백한다', () => {
    expect(panelPixelAspect(6, 4, 0)).toBe(1.5);
  });

  it('단위가 0 이하이면 undefined', () => {
    expect(panelPixelAspect(0, 4, 100)).toBeUndefined();
    expect(panelPixelAspect(6, 0, 100)).toBeUndefined();
  });
});

describe('gridHeightForAspect', () => {
  it('역산한 높이는 목표 비율을 (반올림 오차 내에서) 재현한다', () => {
    const cell = 100;
    const aspect = 16 / 9;
    const h = gridHeightForAspect(8, aspect, cell)!;
    const got = panelPixelAspect(8, h, cell)!;
    // 정수 단위 반올림 때문에 정확히 같지는 않다 — 한 칸 이내로만 벗어난다.
    expect(Math.abs(got - aspect)).toBeLessThan(aspect * 0.12);
  });

  it('panelPixelAspect 와 왕복 일치한다(정사각 도면 → 정사각 패널)', () => {
    expect(gridHeightForAspect(6, 1, 100)).toBe(6);
  });

  it('세로로 긴 도면이면 높이가 폭보다 커진다', () => {
    const h = gridHeightForAspect(4, 0.5, 100)!;
    expect(h).toBeGreaterThan(4);
  });

  it('최소 높이(minH) 아래로는 내려가지 않는다', () => {
    expect(gridHeightForAspect(2, 100, 100, 3)).toBe(3);
  });

  it('셀 미상(0)이면 마진 무시 근사로 역산한다', () => {
    expect(gridHeightForAspect(6, 1.5, 0)).toBe(4);
  });

  it('비율이 부적절하면 undefined', () => {
    expect(gridHeightForAspect(6, 0, 100)).toBeUndefined();
    expect(gridHeightForAspect(6, Number.NaN, 100)).toBeUndefined();
    expect(gridHeightForAspect(0, 1.5, 100)).toBeUndefined();
  });

  it('마진 상수는 대시보드 그리드와 동일한 16px', () => {
    expect(GRID_MARGIN_PX).toBe(16);
  });
});
