// 차트 시간축 유틸.
//
// 데이터 포인트가 시각을 epoch ms 로 들고 있으므로, 축은 "모인 데이터 범위"가 아니라
// "표시 구간" 자체를 그린다. 시작 직후 축이 좁았다가 데이터가 쌓이며 넓어지는 현상을
// 없애기 위한 것이다 — 구간은 처음부터 끝까지 고정되고, 선이 오른쪽부터 채워진다.

import type { MetricDataPoint } from './MetricsChart';

/** epoch ms 를 HH:MM:SS 로 포맷 (축 눈금·툴팁 공용) */
export function formatClock(ts: number): string {
  if (!Number.isFinite(ts)) return '';
  const d = new Date(ts);
  const h = String(d.getHours()).padStart(2, '0');
  const m = String(d.getMinutes()).padStart(2, '0');
  const s = String(d.getSeconds()).padStart(2, '0');
  return `${h}:${m}:${s}`;
}

/**
 * 표시 구간에 해당하는 X축 도메인 `[시작, 끝]` 을 만든다.
 *
 * 끝은 가장 최근 표본이다 — `Date.now()` 를 쓰면 렌더할 때마다 축이 미세하게
 * 흔들리고, 데이터가 끊긴 동안에도 축만 계속 흘러간다.
 *
 * @param seriesList 한 차트에 겹쳐 그리는 계열들 (단일 계열이면 하나)
 * @param windowMs 표시 구간 길이(ms)
 * @param fallbackNow 표본이 하나도 없을 때 쓸 기준 시각 (기본 현재 시각)
 */
export function windowDomain(
  seriesList: readonly MetricDataPoint[][],
  windowMs: number,
  fallbackNow: number = Date.now(),
): [number, number] {
  let end = Number.NEGATIVE_INFINITY;
  for (const series of seriesList) {
    const last = series[series.length - 1];
    if (last && last.ts > end) end = last.ts;
  }
  if (!Number.isFinite(end)) end = fallbackNow;

  const span = windowMs > 0 ? windowMs : 1_000;
  return [end - span, end];
}

/**
 * 표시 구간 밖의 오래된 포인트를 잘라 낸다.
 *
 * 개수가 아니라 시각으로 자른다 — 폴링 주기가 바뀌거나 표본이 걸러진 구간이 있으면
 * 개수 기준 자르기는 실제 구간과 어긋난다.
 */
export function sliceToWindow(
  points: MetricDataPoint[],
  domain: readonly [number, number],
): MetricDataPoint[] {
  const [start] = domain;
  // 대부분의 호출에서 전부 남으므로, 자를 것이 없으면 원본을 그대로 돌려준다.
  if (points.length === 0 || points[0]!.ts >= start) return points;
  return points.filter((p) => p.ts >= start);
}
