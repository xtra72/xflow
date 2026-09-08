// 미리보기 리사이즈의 그리드 단위 변환 — 순수 함수.
//
// 미리보기 상자는 대시보드의 패널을 **축소해서** 보여준다. 그래서 화면에서 끈 픽셀
// 거리를 그대로 그리드 단위로 바꿀 수 없고, "화면 px → 대시보드 px → 그리드 단위"의
// 두 단계를 거쳐야 한다. 이 변환을 DOM 없이 검증할 수 있도록 분리한다
// (gridGeometry.ts 와 같은 규율).

import { GRID_MARGIN_PX, unitsToPx } from './gridGeometry';

/**
 * 대시보드 픽셀 길이를 그리드 단위로 되돌린다(unitsToPx 의 역).
 *
 *   px = u·C + M(u−1)  →  u = (px + M) / (C + M)
 *
 * 셀 크기를 모르면(0) 되돌릴 수 없으므로 undefined.
 */
export function unitsFromPx(px: number, cell: number): number | undefined {
  if (!(cell > 0)) return undefined;
  return (px + GRID_MARGIN_PX) / (cell + GRID_MARGIN_PX);
}

/** 그리드 크기(단위). */
export interface GridSize {
  w: number;
  h: number;
}

/** 크기 제한. `cols` 는 대시보드 칼럼 수 — 폭은 이를 넘을 수 없다. */
export interface GridSizeBounds {
  cols: number;
  minW?: number;
  minH?: number;
  /** 높이 상한. 무한히 긴 패널을 만들지 않기 위한 안전장치. */
  maxH?: number;
}

/** 높이 상한 기본값 — 대시보드에서 스크롤 없이 다루기 어려운 크기를 넘지 않게 한다. */
export const DEFAULT_MAX_H = 50;

/** 그리드 크기를 정수로 맞추고 한계 안으로 가둔다. */
export function clampGridSize(size: GridSize, bounds: GridSizeBounds): GridSize {
  const minW = Math.max(1, Math.trunc(bounds.minW ?? 1) || 1);
  const minH = Math.max(1, Math.trunc(bounds.minH ?? 1) || 1);
  const maxW = Math.max(minW, Math.trunc(bounds.cols) || minW);
  const maxH = Math.max(minH, Math.trunc(bounds.maxH ?? DEFAULT_MAX_H) || minH);

  const w = Math.min(maxW, Math.max(minW, Math.round(size.w) || minW));
  const h = Math.min(maxH, Math.max(minH, Math.round(size.h) || minH));
  return { w, h };
}

/** resizeGridSize 입력. */
export interface ResizeInput {
  /** 드래그 시작 시점의 그리드 크기 */
  start: GridSize;
  /** 화면에서 끈 거리(px) */
  dxPx: number;
  dyPx: number;
  /** 드래그 시작 시점의 미리보기 상자 화면 크기(px) */
  boxW: number;
  boxH: number;
  /** 대시보드의 셀 한 변(px). 0 이면 미측정 — 상자 비례로 근사한다. */
  cell: number;
  bounds: GridSizeBounds;
}

/**
 * 미리보기 상자를 끈 거리를 새 그리드 크기로 바꾼다.
 *
 * 미리보기 상자(boxW×boxH)는 `start` 크기의 패널을 축소해 보여주는 것이므로,
 * 화면 px 와 대시보드 px 사이의 배율은 `boxW / unitsToPx(start.w, cell)` 이다.
 * 이 배율로 드래그 거리를 대시보드 px 로 되돌린 뒤 그리드 단위로 환산한다.
 *
 * 셀 크기를 모르면(대시보드를 한 번도 열지 않은 경우) 마진을 무시하고 상자를
 * 단위 수로 나눈 근사 배율을 쓴다 — 정확하진 않지만 끌리기는 한다.
 */
export function resizeGridSize(input: ResizeInput): GridSize {
  const { start, dxPx, dyPx, boxW, boxH, cell, bounds } = input;

  const next = (
    startUnits: number,
    boxPx: number,
    deltaPx: number,
  ): number => {
    // 값이 하나라도 성립하지 않으면 크기를 바꾸지 않는다. NaN 을 그대로 흘리면
    // clamp 가 조용히 최소 크기(1×1)로 떨어뜨려, 계산이 깨진 것을 "사용자가 아주
    // 작게 끌었다"와 구분할 수 없게 된다.
    if (!Number.isFinite(deltaPx)) return startUnits;
    if (!(boxPx > 0) || !(startUnits > 0)) return startUnits;

    if (cell > 0) {
      const scale = boxPx / unitsToPx(startUnits, cell);
      if (!(scale > 0)) return startUnits;
      const dashboardPx = (boxPx + deltaPx) / scale;
      return unitsFromPx(dashboardPx, cell) ?? startUnits;
    }

    // 셀 미측정 폴백 — 상자를 단위 수로 나눈 근사 배율.
    return ((boxPx + deltaPx) * startUnits) / boxPx;
  };

  return clampGridSize(
    { w: next(start.w, boxW, dxPx), h: next(start.h, boxH, dyPx) },
    bounds,
  );
}

/** 두 그리드 크기가 같은지. */
export function sameGridSize(a: GridSize | null, b: GridSize | null): boolean {
  if (a === null || b === null) return a === b;
  return a.w === b.w && a.h === b.h;
}
