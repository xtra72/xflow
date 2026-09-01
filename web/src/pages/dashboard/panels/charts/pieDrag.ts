// 파이 패널에서 **끌어 옮긴 오프셋**을 다루는 순수 함수들.
//
// 두 대상의 좌표계가 다르다.
//   - **범례**는 HTML 요소라 픽셀이 그대로 오프셋이 된다(CSS transform).
//   - **파이 중심**은 recharts 가 `cx`/`cy` 를 백분율로 받는다 — 그 환산과 죄기는
//     게이지와 공유하므로 `panelGeometry` 가 소유한다.
//
// 게이지의 `panels/gauge/valueDrag.ts` 도 같은 이유로 자기 좌표계(SVG viewBox)를 쓴다 —
// 좌표계를 하나로 뭉개면 어느 한쪽이 패널 크기에 따라 과하게/굼뜨게 움직인다.

/**
 * 오프셋을 패널 밖으로 완전히 나가지 않을 만큼 죈다.
 *
 * 상한은 기준 변의 절반이다 — 그보다 멀리 밀면 범례가 패널 밖으로 나가 다시 잡을 수
 * 없다(끌어서 옮긴 것을 끌어서 되돌릴 수 없게 된다).
 *
 * 기준 변을 잴 수 없으면(레이아웃 전이라 0) 오프셋을 0으로 돌린다 — 죌 수 없는 값을
 * 그대로 두면 범례가 사라진 것처럼 보인다.
 */
export function clampLegendOffset(value: number, span: number): number {
  if (!Number.isFinite(value)) return 0;
  if (!Number.isFinite(span) || span <= 0) return 0;
  const limit = span / 2;
  return Math.min(Math.max(value, -limit), limit);
}

/**
 * 범례의 CSS transform — **기준 자리 보정 + 끌어 옮긴 오프셋**을 하나로 합친다.
 *
 * 범례는 차트 위에 겹쳐 뜨므로 절대 배치를 쓰는데, 가운데 정렬은 `left: 50%` 만으로는
 * 되지 않고 자기 폭의 절반을 되물려야 한다(`translate(-50%)`). 그 보정과 드래그
 * 오프셋이 **같은 transform 속성**을 두고 다투므로 한 곳에서 합쳐야 한다 — 따로 쓰면
 * 뒤에 선언한 쪽이 앞을 지워 범례가 기준 자리에서 튄다.
 *
 * - 하단: 가로만 되물린다(세로는 아래에 붙인다).
 * - 좌·우: 세로만 되물린다(가로는 옆에 붙인다).
 */
export function legendTransform(
  position: 'bottom' | 'left' | 'right',
  x: number,
  y: number,
): string {
  return position === 'bottom'
    ? `translate(calc(-50% + ${x}px), ${y}px)`
    : `translate(${x}px, calc(-50% + ${y}px))`;
}
