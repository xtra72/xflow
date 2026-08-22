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

/**
 * 항목 수에 대한 **열 수 상한**을 돌려준다 — `ceil(sqrt(N))`.
 *
 * 실제 열 수는 `min(ceil(sqrt(N)), 패널 폭이 허용하는 최대 열)` 이며(§2.4), 뒤쪽 항은
 * CSS `auto-fit` + `minmax` 가 렌더 시점에 결정한다. 여기서는 앞쪽 항만 계산한다 —
 * jsdom 에는 레이아웃이 없으므로 폭 기반 판정을 JS 로 흉내 내면 테스트가 거짓말을 한다.
 */
export function autoColumnCount(count: number): number {
  if (!Number.isFinite(count) || count <= 1) return 1;
  return Math.ceil(Math.sqrt(count));
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
