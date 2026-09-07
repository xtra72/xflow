// 미리보기 스테이지 기하 — 순수 함수.
//
// 미리보기는 패널을 **대시보드에서의 실제 픽셀 크기 그대로** 그린 뒤 통째로 축소해
// 보여준다. 이렇게 해야 큰 패널의 레이아웃(칼럼이 몇 개 보이는지, 글자가 어디서
// 줄바꿈되는지)이 대시보드와 같아진다. 미리보기 크기에 맞춰 다시 레이아웃하면
// 반응형 재배치가 일어나 대시보드와 다른 화면이 된다.
//
// DOM 에 의존하지 않도록 크기를 인자로 받는다(gridGeometry.ts 와 같은 규율).

import { GRID_MARGIN_PX, unitsToPx } from './gridGeometry';

/** 셀 크기를 알 수 없을 때(대시보드 미방문) 쓰는 기준 셀 한 변(px). */
export const NOMINAL_CELL_PX = 100;

/** 미리보기 채움 방식. fit=전체가 보이도록 축소, fill=영역을 채우도록 확대. */
export type PreviewFillMode = 'fill' | 'fit';

export interface PreviewStageInput {
  /** 패널의 그리드 크기(단위) */
  w: number;
  h: number;
  /** 대시보드의 셀 한 변(px). 0 이면 미측정 — NOMINAL_CELL_PX 로 대체한다. */
  cell: number;
  /** 미리보기 영역의 실측 크기(px) */
  areaW: number;
  areaH: number;
  mode: PreviewFillMode;
  /** 사용자 줌 배율(0.5~2.0) */
  zoom: number;
}

export interface PreviewStage {
  /** 대시보드에서의 패널 픽셀 크기 — 패널을 이 크기로 렌더한 뒤 축소한다. */
  pxW: number;
  pxH: number;
  /**
   * 대시보드 픽셀 → 화면 픽셀 배율. 축별로 나뉜다.
   *
   * `fit` 은 두 축이 **줌 값 그대로**다 — 100% 가 대시보드와 1:1 이라는 뜻이고,
   * 영역보다 큰 패널은 넘치므로 줌을 낮춰 전체를 본다.
   * `fill` 은 축마다 배율이 달라 영역을 정확히 채운다(그만큼 늘어난다). 축을 묶어
   * 확대하면 한쪽이 넘쳐 우하단 손잡이가 화면 밖으로 나가므로 나눈다.
   */
  scaleX: number;
  scaleY: number;
  /** 화면에 실제로 차지하는 크기(px) */
  screenW: number;
  screenH: number;
  /** 화면 기준 셀 크기와 간격 — 그리드 라인을 이 값으로 그린다(축별). */
  cellW: number;
  cellH: number;
  gapX: number;
  gapY: number;
}

/**
 * 패널의 그리드 크기와 미리보기 영역으로 스테이지 기하를 구한다.
 *
 * - `fit`: 두 축에 줌 값을 그대로 쓴다. **100% 는 대시보드와 같은 크기(1:1)** 이므로
 *   실제 크기를 그대로 확인할 수 있고, 영역보다 큰 패널은 줌을 낮춰 전체를 본다.
 * - `fill`: 축마다 배율을 달리해 영역을 정확히 채운다. 형태는 늘어나지만 영역을
 *   남김없이 쓰므로 세부를 크게 볼 수 있고, 우하단 크기 조절 손잡이가 항상 영역
 *   안(모서리)에 놓인다.
 *
 * 줌은 두 경우 모두 배율에 곱해진다 — 줌 100% 를 넘기면 의도적으로 넘칠 수 있다.
 *
 * 크기를 알 수 없으면(영역 미실측, 그리드 단위 0) null — 호출부가 렌더를 미룬다.
 */
/**
 * 패널 전체가 영역 안에 들어가는 배율.
 *
 * 맞춤 모드의 100% 는 대시보드 1:1 이라 큰 패널은 넘친다. 이 값이 "전부 보이는"
 * 배율이므로 줌 하한을 여기까지 열어 준다 — 그러지 않으면 큰 패널의 전체를 볼
 * 방법이 없다.
 */
export function computeFitScale(
  input: Pick<PreviewStageInput, 'w' | 'h' | 'cell' | 'areaW' | 'areaH'>,
): number | null {
  if (!(input.w > 0) || !(input.h > 0)) return null;
  if (!(input.areaW > 0) || !(input.areaH > 0)) return null;

  const cell = input.cell > 0 ? input.cell : NOMINAL_CELL_PX;
  const pxW = unitsToPx(input.w, cell);
  const pxH = unitsToPx(input.h, cell);
  if (!(pxW > 0) || !(pxH > 0)) return null;

  const scale = Math.min(input.areaW / pxW, input.areaH / pxH);
  return scale > 0 && Number.isFinite(scale) ? scale : null;
}

export function computePreviewStage(input: PreviewStageInput): PreviewStage | null {
  const { w, h, mode, zoom } = input;
  if (!(w > 0) || !(h > 0)) return null;
  if (!(input.areaW > 0) || !(input.areaH > 0)) return null;

  const cell = input.cell > 0 ? input.cell : NOMINAL_CELL_PX;
  const pxW = unitsToPx(w, cell);
  const pxH = unitsToPx(h, cell);
  if (!(pxW > 0) || !(pxH > 0)) return null;

  const byWidth = input.areaW / pxW;
  const byHeight = input.areaH / pxH;
  const scaleX = mode === 'fill' ? byWidth * zoom : zoom;
  const scaleY = mode === 'fill' ? byHeight * zoom : zoom;
  if (!(scaleX > 0) || !(scaleY > 0)) return null;
  if (!Number.isFinite(scaleX) || !Number.isFinite(scaleY)) return null;

  return {
    pxW,
    pxH,
    scaleX,
    scaleY,
    screenW: pxW * scaleX,
    screenH: pxH * scaleY,
    cellW: cell * scaleX,
    cellH: cell * scaleY,
    gapX: GRID_MARGIN_PX * scaleX,
    gapY: GRID_MARGIN_PX * scaleY,
  };
}

/**
 * 패널을 열거나 모드를 바꿀 때의 시작 줌 배율.
 *
 * 맞춤 100% 는 대시보드 1:1 이므로 큰 패널을 그대로 열면 가운데 일부만 보인다 —
 * 카드 테두리도 제목도 화면 밖이라 패널 모양을 알 수 없다. 그래서 처음에는 전체가
 * 보이는 배율에서 시작한다.
 *
 * 영역보다 작은 패널은 1 을 넘기지 않는다. 확대해 띄우면 "100% = 실제 크기" 라는
 * 약속이 깨져, 미리보기에서 큰 글자가 대시보드에서 작아지는 혼란이 생긴다.
 *
 * 채움 모드는 100% 가 이미 영역에 꼭 맞으므로 1 이다.
 */
export function initialPreviewZoom(mode: PreviewFillMode, fitScale: number | null): number {
  if (mode === 'fill' || fitScale === null) return 1;
  return Math.min(1, fitScale);
}
