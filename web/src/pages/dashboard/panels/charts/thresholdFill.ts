// 경계 채우기 구간 산출.
//
// recharts 의 `ReferenceArea` 는 기본값이 `ifOverflow: 'discard'` 라, 양 끝 중
// 하나라도 축 도메인 밖이면 **통째로 그리지 않는다**. 종전에는 `경계 이하` 를
// `y1 = -1e9`, `경계 이상` 을 `y2 = 1e9` 로 표현했는데, 그 값이 언제나 도메인
// 밖이라 두 모드가 화면에 아예 나오지 않았다.
//
// 그래서 채울 구간을 **축 도메인 안으로 좁혀서** 넘긴다. 도메인을 모를 때
// (`'auto'`)를 대비해 호출부는 `ifOverflow="hidden"` 도 함께 준다 — 그때는
// 버리는 대신 그림 영역에서 잘라 낸다.

/** 채울 구간. 축 도메인 안으로 좁혀진 값이다. */
export interface FillBand {
  y1: number;
  y2: number;
}

/** 축 도메인 — recharts 규약을 그대로 쓴다(수치이거나 `'auto'`). */
export type AxisBound = number | 'auto';

/** 도메인을 모를 때 쓰는 대체 경계. 화면 밖으로 충분히 크되 1e9 처럼 극단은 피한다. */
const FALLBACK_SPAN = 1e6;

export interface ThresholdLike {
  value: number;
  /** `below` = 경계 이하, `above` = 경계 이상. 없으면 `fill_to` 로 구간을 만든다. */
  fill_direction?: 'below' | 'above';
  /** 명시 구간의 반대쪽 끝. */
  fill_to?: number;
}

/**
 * 경계 하나가 채울 구간을 구한다.
 *
 * 채우지 않는 경계(`fill_direction` 도 `fill_to` 도 없음)는 `undefined` 다.
 *
 * 반환값은 **축 도메인 안으로 좁혀진다**. 도메인 밖으로 나가면 recharts 가
 * 그리기를 포기하기 때문이다 — 이 좁히기가 이 함수의 존재 이유다.
 */
export function resolveFillBand(
  t: ThresholdLike,
  domain: readonly [AxisBound, AxisBound],
): FillBand | undefined {
  const [lo, hi] = domain;
  // 도메인을 모르면 넉넉한 대체값을 쓴다. 호출부의 `ifOverflow="hidden"` 이 받아 준다.
  const bottom = typeof lo === 'number' && Number.isFinite(lo) ? lo : t.value - FALLBACK_SPAN;
  const top = typeof hi === 'number' && Number.isFinite(hi) ? hi : t.value + FALLBACK_SPAN;

  let y1: number;
  let y2: number;
  if (t.fill_direction === 'below') {
    y1 = bottom;
    y2 = t.value;
  } else if (t.fill_direction === 'above') {
    y1 = t.value;
    y2 = top;
  } else if (t.fill_to != null) {
    y1 = Math.min(t.value, t.fill_to);
    y2 = Math.max(t.value, t.fill_to);
  } else {
    return undefined;
  }

  // 도메인 안으로 좁힌다. 좁힌 뒤 두께가 0 이하면 보이지 않으므로 그리지 않는다 —
  // 경계가 축 밖에 있는 경우가 그렇다.
  const clampedLo = Math.max(Math.min(y1, y2), bottom);
  const clampedHi = Math.min(Math.max(y1, y2), top);
  if (!(clampedHi > clampedLo)) return undefined;
  return { y1: clampedLo, y2: clampedHi };
}
