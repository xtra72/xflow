// 단일 값 툴팁 — 커서에 가장 가까운 라인 하나만 고른다.
//
// recharts 의 `<Tooltip shared={false}>` 로는 되지 않는다. v3 의 LineChart 는
// 허용 툴팁 이벤트 타입이 `['axis']` 뿐이라 `shared` 가 무시되고, 결국 축 위의
// 모든 시리즈가 그대로 나온다. 그래서 축 모드는 그대로 두고 **payload 를 좁힌다**.
//
// 어느 라인인가는 커서의 세로 위치로 정한다 — 사용자가 "이 선" 이라고 여기는
// 것은 가리키고 있는 높이의 선이다. 판정은 순수 함수로 떼어 두어 마우스 없이
// 전수 검증한다.

/** 플롯 영역(픽셀). recharts `usePlotArea()` 결과의 필요한 부분만. */
export interface PlotBox {
  y: number;
  height: number;
}

/**
 * 커서의 픽셀 Y 를 **데이터 값**으로 되돌린다.
 *
 * 되돌릴 수 없으면 `undefined` — 커서를 모르거나, 플롯 높이가 0 이거나, Y축
 * 도메인이 숫자가 아닌 경우다. 이때는 "가장 가까운" 을 정의할 수 없다.
 */
export function cursorValueAt(
  cursorY: number | undefined,
  plot: PlotBox | undefined,
  yDomain: readonly unknown[] | undefined,
): number | undefined {
  if (cursorY === undefined || !plot || !(plot.height > 0)) return undefined;
  if (!yDomain || yDomain.length < 2) return undefined;

  const min = yDomain[0];
  const max = yDomain[yDomain.length - 1];
  if (typeof min !== 'number' || typeof max !== 'number') return undefined;
  if (!Number.isFinite(min) || !Number.isFinite(max) || min === max) return undefined;

  // 픽셀은 위에서 아래로 늘고 값은 아래에서 위로 는다 — 그래서 뒤집는다.
  const ratio = (cursorY - plot.y) / plot.height;
  return max - ratio * (max - min);
}

/**
 * 커서 값에 가장 가까운 항목 하나만 남긴다.
 *
 * 판정할 수 없으면(커서 값을 모르거나 숫자 항목이 하나도 없으면) **원본을 그대로**
 * 돌려준다. 임의로 하나를 고르면 가리키지 않은 선의 값이 나와, 사용자는 그것이
 * 가리킨 선의 값이라고 읽는다 — 조용한 오답이다. 전부 보여 주는 쪽이 낫다.
 */
export function pickNearestSeries<T>(
  payload: readonly T[] | undefined,
  valueOf: (item: T) => unknown,
  cursorValue: number | undefined,
): readonly T[] {
  if (!payload || payload.length <= 1) return payload ?? [];
  if (cursorValue === undefined) return payload;

  let best: T | undefined;
  let bestDist = Number.POSITIVE_INFINITY;
  for (const item of payload) {
    const v = valueOf(item);
    if (typeof v !== 'number' || !Number.isFinite(v)) continue;
    const d = Math.abs(v - cursorValue);
    // 동률이면 앞선 항목을 남긴다 — 시리즈 순서가 곧 범례 순서라 예측 가능하다.
    if (d < bestDist) {
      bestDist = d;
      best = item;
    }
  }
  return best === undefined ? payload : [best];
}
