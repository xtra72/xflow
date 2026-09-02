// 미리보기에서 현재값을 끌어 옮길 때의 **좌표 환산**.
//
// 게이지는 SVG `viewBox` 좌표로 그려지고 화면에는 패널 크기에 맞춰 축소·확대되어
// 나온다. 그래서 포인터가 움직인 **픽셀** 거리를 그대로 오프셋에 더하면, 패널이 작을수록
// 값이 과하게 움직이고 클수록 굼뜨다. 픽셀을 viewBox 단위로 되돌린 뒤 더해야 한다.
//
// 순수 함수로 두는 이유: 환산이 틀리면 "조금 끌었는데 값이 화면 밖으로 날아간다" 로
// 나타나는데, 그것을 브라우저 없이 잠글 수 있어야 한다.

/** `viewBox="minX minY w h"` 를 폭·높이로 읽는다. 형식이 아니면 `null`. */
export function parseViewBox(raw: string | null | undefined): { w: number; h: number } | null {
  if (!raw) return null;
  const parts = raw.trim().split(/[\s,]+/).map(Number);
  if (parts.length !== 4 || parts.some((n) => !Number.isFinite(n))) return null;
  const [, , w, h] = parts as [number, number, number, number];
  return w > 0 && h > 0 ? { w, h } : null;
}

/**
 * 화면 픽셀 이동량을 viewBox 이동량으로 환산한다.
 *
 * SVG 의 기본 `preserveAspectRatio`(=meet)는 **가로·세로 같은 배율**로 축소하고 남는
 * 쪽에 여백을 둔다. 그래서 배율은 두 축 비율 중 작은 쪽 하나다 — 축마다 따로 계산하면
 * 여백이 있는 축에서 이동량이 부풀려진다.
 *
 * 배율을 구할 수 없으면(레이아웃 전이라 크기가 0) 이동 없음을 돌려준다. 0 으로 나눠
 * `Infinity` 를 오프셋에 더하면 값이 화면에서 사라진다.
 */
export function pixelsToViewBox(
  dxPx: number,
  dyPx: number,
  rect: { width: number; height: number },
  viewBox: { w: number; h: number },
): { dx: number; dy: number } {
  const scale = Math.min(rect.width / viewBox.w, rect.height / viewBox.h);
  if (!Number.isFinite(scale) || scale <= 0) return { dx: 0, dy: 0 };
  return { dx: dxPx / scale, dy: dyPx / scale };
}

/**
 * 오프셋을 캔버스 밖으로 나가지 않을 만큼 죈다.
 *
 * 상한은 viewBox 한 변의 절반이다 — 그보다 멀리 밀면 값이 캔버스 밖으로 완전히
 * 나가 다시 잡을 수 없다(끌어서 옮긴 것을 끌어서 되돌릴 수 없게 된다).
 */
export function clampOffset(value: number, span: number): number {
  const limit = span / 2;
  return Math.min(Math.max(value, -limit), limit);
}
