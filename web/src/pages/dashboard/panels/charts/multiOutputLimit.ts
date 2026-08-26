// 다중 출력 그리드의 순수 계산 — 열 수 상한 / 표시 상한 적용.
//
// 컴포넌트 파일(SeriesTileGrid.tsx)에서 분리했다. 컴포넌트 파일이 컴포넌트만
// 내보내야 Fast Refresh 가 동작하기 때문이다(react-refresh/only-export-components).
//
// @spec SPEC-CHART-002 §2.4

/**
 * 다중 출력 표시 개수 기본 상한. @spec SPEC-CHART-002 §2.4 / §4.6 / OQ1
 *
 * 시리즈 **선택** 상한(`STORE_SERIES_LIMIT` = 48)과 다른 축이다 — 선택 상한은 조회 부하를,
 * 이 상한은 가독성을 보호한다. 48개를 조회하되 12개만 그리는 상태는 정상이다.
 */
export const DEFAULT_MULTI_OUTPUT_LIMIT = 12;

/** 타일 최소 폭(px). 이 폭이 확보되지 않으면 열 수가 자동으로 줄어든다(§2.4). */
export const MIN_TILE_WIDTH_PX = 120;

/** 타일 간격(px). 열 폭 계산에 들어가므로 상수로 고정한다. */
export const TILE_GAP_PX = 8;

/** 타일 배열의 기본 행 수. 지정하지 않으면 한 줄에 늘어놓는다. */
export const DEFAULT_TILE_ROWS = 1;

/**
 * 행 수 상한. 표시 상한 기본값(12)과 같은 값이다 — 12개를 12행에 하나씩 놓는 것이
 * "행을 늘리는" 방향의 끝이고, 그보다 많은 행은 놓을 타일이 없어 의미가 없다.
 */
export const MAX_TILE_ROWS = 12;

/**
 * 행 수 설정을 정규화한다. 비정상 값(미지정 · 비유한 · 1 미만)은 기본값으로 되돌리고
 * 상한을 넘으면 잘라낸다.
 *
 * `0` 을 "행 없음" 으로 해석하지 않는 이유는 표시 상한과 같다(`applyMultiOutputLimit`) —
 * 사용자가 실수로 패널을 비울 수 있고, 그 상태는 "값이 없다" 와 화면에서 구분되지 않는다.
 */
export function normalizeTileRows(rows?: number): number {
  const raw = rows === undefined ? Number.NaN : Math.trunc(rows);
  if (!Number.isFinite(raw) || raw < 1) return DEFAULT_TILE_ROWS;
  return Math.min(raw, MAX_TILE_ROWS);
}

/**
 * 항목 수와 목표 행 수로 **열 수 상한**을 돌려준다 — `ceil(N / rows)`.
 *
 * 실제 열 수는 `min(ceil(N / rows), 패널 폭이 허용하는 최대 열)` 이며, 뒤쪽 항은 CSS
 * `auto-fit` + `minmax` 가 렌더 시점에 결정한다. 여기서는 앞쪽 항만 계산한다 — jsdom 에는
 * 레이아웃이 없으므로 폭 기반 판정을 JS 로 흉내 내면 테스트가 거짓말을 한다.
 *
 * 그래서 행 수는 **목표이지 상한이 아니다**. 폭이 좁아 타일 최소 폭(`MIN_TILE_WIDTH_PX`)을
 * 확보하지 못하면 열이 줄고 행이 목표보다 늘어난다. 반대로 하면 시리즈가 많고 패널이 좁을
 * 때 타일이 수십 px 로 짜부라져 값을 읽을 수 없다.
 */
export function tileColumnCount(count: number, rows?: number): number {
  if (!Number.isFinite(count) || count <= 1) return 1;
  return Math.max(1, Math.ceil(count / normalizeTileRows(rows)));
}

/** 상한 적용 결과. `truncated` 는 잘려 나간 개수(0 이면 잘림 없음). */
export interface MultiOutputSlice<T> {
  visible: T[];
  truncated: number;
}

/**
 * 표시 상한을 적용한다. 잘림은 **순서상 뒤에서부터**이며 정렬을 바꾸지 않는다(UB1-11).
 *
 * `limit` 이 미지정이거나 정상 범위를 벗어나면(0 이하 / 비유한) 기본 상한으로 되돌린다 —
 * `multi_output_limit: 0` 을 "아무것도 그리지 않는다" 로 해석하면 사용자가 패널을 실수로
 * 비울 수 있고, 그 상태는 "값이 없다" 와 화면에서 구분되지 않는다.
 */
export function applyMultiOutputLimit<T>(
  items: readonly T[],
  limit?: number,
): MultiOutputSlice<T> {
  const raw = limit === undefined ? Number.NaN : Math.trunc(limit);
  const max = Number.isFinite(raw) && raw > 0 ? raw : DEFAULT_MULTI_OUTPUT_LIMIT;
  if (items.length <= max) return { visible: [...items], truncated: 0 };
  return { visible: items.slice(0, max), truncated: items.length - max };
}
