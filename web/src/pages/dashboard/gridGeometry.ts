// 대시보드 그리드 기하 순수 함수.
//
// 대시보드는 react-grid-layout 위에 "정사각형 셀"로 그려진다 — 행 높이를 칼럼 폭과 같게
// 잡기 때문이다(DashboardPage: rowHeight = (containerWidth - M(cols-1)) / cols). 따라서
// 패널의 실제 픽셀 크기는 셀 크기 C 와 마진 M 으로 결정된다:
//
//   px(u) = u·C + M·(u−1)      (u = 그리드 단위 폭 또는 높이)
//
// 패널의 픽셀 종횡비는 px(w)/px(h) 이며, 그리드 단위 비 w/h 와 **같지 않다**(마진이 단위 수에
// 비례해 끼어들기 때문). 셀이 작고 단위 수 차이가 클수록 오차가 커져(예: C=50, 1×8 → 0.098 vs
// 0.125) 히트맵 레터박스 예측이 어긋난다. 그래서 근사 대신 이 식을 쓴다.
//
// DOM 에 의존하지 않도록 크기를 인자로 받는다(stage.ts / placement.ts 와 동일 규율).

/** 그리드 마진(px). 양축 동일 — DashboardPage / RemoteDashboardView 가 공유한다. */
export const GRID_MARGIN_PX = 16;

/**
 * 컨테이너 폭과 칼럼 수로 정사각형 셀 한 변(px)을 구한다.
 * 측정 전(폭 0)이거나 칼럼 수가 부적절하면 0 — 호출부가 "셀 크기 미상"으로 폴백한다.
 */
export function gridCellSize(containerWidth: number, cols: number): number {
  if (!(containerWidth > 0) || !(cols > 0)) return 0;
  return Math.round((containerWidth - GRID_MARGIN_PX * (cols - 1)) / cols);
}

/** 그리드 단위 u 가 차지하는 픽셀 길이(마진 포함). */
function unitsToPx(u: number, cell: number): number {
  return u * cell + GRID_MARGIN_PX * (u - 1);
}

/**
 * 패널의 실제 픽셀 종횡비(폭/높이). 단위가 부적절하면 undefined.
 *
 * `cell` 이 0(미측정)이면 마진을 무시한 근사 w/h 를 돌려준다 — 대시보드를 한 번도 열지 않고
 * 설정 화면에 직접 진입한 경우다. 정확도는 떨어지지만 값이 없는 것보다 낫다.
 */
export function panelPixelAspect(w: number, h: number, cell: number): number | undefined {
  if (!(w > 0) || !(h > 0)) return undefined;
  if (!(cell > 0)) return w / h;
  return unitsToPx(w, cell) / unitsToPx(h, cell);
}

/**
 * 폭 `w`(그리드 단위)를 고정한 채 목표 종횡비 `aspect`(폭/높이)를 내는 높이(그리드 단위)를 구한다.
 *
 *   px(h) = px(w) / aspect  →  h·C + M(h−1) = target  →  h = (target + M) / (C + M)
 *
 * 그리드는 정수 단위이므로 반올림하며 최소 1(또는 `minH`)을 보장한다. 반올림 때문에 여백이
 * 완전히 0 이 되지는 않고 "한 칸 이내"로 줄어든다.
 */
export function gridHeightForAspect(
  w: number,
  aspect: number,
  cell: number,
  minH = 1,
): number | undefined {
  if (!(w > 0) || !Number.isFinite(aspect) || aspect <= 0) return undefined;
  const floor = Math.max(1, Math.trunc(minH) || 1);
  if (!(cell > 0)) return Math.max(floor, Math.round(w / aspect));
  const target = unitsToPx(w, cell) / aspect;
  return Math.max(floor, Math.round((target + GRID_MARGIN_PX) / (cell + GRID_MARGIN_PX)));
}
