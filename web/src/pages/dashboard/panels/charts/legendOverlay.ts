// 그림 위에 **겹쳐 뜨는 범례**의 자리 계산 — 파이 범례와 게이지 임계값 범례가 공유한다.
//
// 오프셋은 **담는 상자 대비 백분율**이다. 픽셀로 두면 미리보기와 실제 패널의 크기가
// 달라 같은 값이 다른 자리를 가리키고, 좁은 패널에서는 범례가 밖으로 나가 보이지 않는다.
//
// 한때 게이지 값 글자만 SVG viewBox 좌표를 썼으나, 도형 밖 오버레이가 되면서 이 규약으로
// 합쳐졌다 — 이제 패널의 모든 떠 있는 요소가 같은 축을 쓴다.
//
// 파일 끝의 `clampInlineLegendOffset` / `inlineLegendOffsetStyle` 은 **겹쳐 뜨지 않는**
// 라인 차트 범례 몫이다. 백분율 오프셋이라는 저장 규약은 같고, 죄기 기준만 다르다.

import type { CSSProperties } from 'react';

/** 범례를 붙일 변. */
export type LegendAnchor = 'top' | 'bottom' | 'left' | 'right';

/**
 * 겹쳐 뜨는 범례의 배치 스타일 — **기준 자리 + 끌어 옮긴 오프셋**.
 *
 * 오프셋을 **백분율**로 받는 이유가 이 함수의 존재 이유다. 픽셀로 두면 미리보기와 실제
 * 패널의 폭이 달라 같은 값이 다른 자리를 가리키고, 좁은 패널에서는 범례가 아예 밖으로
 * 나가 보이지 않는다(설정 미리보기 ≈1870px vs 대시보드 패널 ≈1500px 이하).
 *
 * CSS `transform: translate(%)` 를 쓰지 않는 이유: transform 의 백분율은 **요소 자신의**
 * 크기 기준이다. 범례는 컨테이너보다 훨씬 작으므로 `translate(10%)` 는 "컨테이너 폭의
 * 10%" 가 아니라 "범례 폭의 10%" 가 된다. `left`/`top`/`bottom` 의 백분율은 담는
 * 상자 기준이므로 이쪽을 쓴다. transform 은 가운데 정렬 되물림에만 남긴다.
 *
 * 부호는 **화면 방향**으로 통일한다 — x 가 크면 오른쪽, y 가 크면 아래. 그래서 오른쪽·
 * 아래에 붙인 축은 값을 뒤집어 넣는다(`right`/`bottom` 은 반대 방향으로 자라므로).
 */
export function legendOverlayStyle(
  anchor: LegendAnchor,
  xPercent: number,
  yPercent: number,
): CSSProperties {
  switch (anchor) {
    case 'top':
      return {
        left: `calc(50% + ${xPercent}%)`,
        top: `${yPercent}%`,
        transform: 'translateX(-50%)',
      };
    case 'bottom':
      return {
        left: `calc(50% + ${xPercent}%)`,
        bottom: `${-yPercent}%`,
        transform: 'translateX(-50%)',
      };
    case 'left':
      return {
        left: `${xPercent}%`,
        top: `calc(50% + ${yPercent}%)`,
        transform: 'translateY(-50%)',
      };
    case 'right':
      return {
        right: `${-xPercent}%`,
        top: `calc(50% + ${yPercent}%)`,
        transform: 'translateY(-50%)',
      };
  }
}

/** 오프셋이 움직일 수 있는 범위(백분율). */
export interface LegendOffsetBounds {
  minX: number;
  maxX: number;
  minY: number;
  maxY: number;
}

/**
 * 끌 수 있는 **범위**를 범례와 담는 상자의 실제 크기에서 구한다.
 *
 * 고정 상한(±40% 같은 수)으로 죄면 두 방향에서 틀린다 — 작은 범례는 가장자리에 닿기도
 * 전에 멈추고(오른쪽에 여백이 남는데 더 못 간다), 큰 범례는 상한 안에서도 밖으로 나간다.
 * 범례 모서리가 상자 가장자리에 **정확히 닿는** 지점이 자연스러운 한계다.
 *
 * 축마다 기준이 다르다.
 *   - 가운데 정렬 축(위·아래 배치의 가로, 좌·우 배치의 세로): 중심이 기준이므로
 *     `±50%(1 − 크기비)`.
 *   - 붙인 축(위 배치의 세로 등): 그 변이 0 이므로 한쪽은 0, 다른 쪽은 `100%(1 − 크기비)`.
 *
 * 상자를 잴 수 없거나 범례가 상자보다 크면 움직일 여지가 없으므로 0으로 고정한다 —
 * 죌 수 없는 값을 그대로 두면 범례가 화면 밖으로 사라진다.
 */
export function legendOffsetBounds(
  anchor: LegendAnchor,
  container: { width: number; height: number },
  legend: { width: number; height: number },
): LegendOffsetBounds {
  // `-0` 을 만들지 않는다 — 그대로 CSS 로 나가면 `calc(50% + -0%)` 같은 문자열이 되고,
  // 테스트에서도 `+0` 과 다른 값으로 잡혀 의도를 흐린다.
  const neg = (v: number): number => (v === 0 ? 0 : -v);
  const centered = (span: number, size: number): number => {
    if (!(span > 0) || !(size > 0) || size >= span) return 0;
    return 50 * (1 - size / span);
  };
  const anchored = (span: number, size: number): number => {
    if (!(span > 0) || !(size > 0) || size >= span) return 0;
    return 100 * (1 - size / span);
  };

  switch (anchor) {
    case 'top': {
      const x = centered(container.width, legend.width);
      return { minX: neg(x), maxX: x, minY: 0, maxY: anchored(container.height, legend.height) };
    }
    case 'bottom': {
      const x = centered(container.width, legend.width);
      return { minX: neg(x), maxX: x, minY: neg(anchored(container.height, legend.height)), maxY: 0 };
    }
    case 'left': {
      const y = centered(container.height, legend.height);
      return { minX: 0, maxX: anchored(container.width, legend.width), minY: neg(y), maxY: y };
    }
    case 'right': {
      const y = centered(container.height, legend.height);
      return { minX: neg(anchored(container.width, legend.width)), maxX: 0, minY: neg(y), maxY: y };
    }
  }
}

/** 오프셋 한 쌍을 {@link legendOffsetBounds} 범위로 죈다. */
export function clampLegendOffsets(
  anchor: LegendAnchor,
  x: number,
  y: number,
  container: { width: number; height: number },
  legend: { width: number; height: number },
): { x: number; y: number } {
  const b = legendOffsetBounds(anchor, container, legend);
  const fit = (v: number, lo: number, hi: number): number =>
    Number.isFinite(v) ? Math.min(Math.max(v, lo), hi) : 0;
  return { x: fit(x, b.minX, b.maxX), y: fit(y, b.minY, b.maxY) };
}

/**
 * 저장된 오프셋의 **안전 상한**(렌더 시점).
 *
 * 끌 때는 실제 크기로 정확히 죄지만(위), 저장된 값은 그 뒤 패널 크기가 바뀌거나 손으로
 * 고쳐질 수 있다. 그때도 범례가 통째로 사라지지 않도록 성긴 상한만 둔다 — 정확한 죄기는
 * 요소 크기를 알아야 하는데 그 값은 레이아웃 후에만 나온다.
 */
export const LEGEND_OFFSET_SAFETY_LIMIT = 50;

export function clampStoredLegendOffset(v: unknown): number {
  const n = typeof v === 'number' && Number.isFinite(v) ? v : 0;
  return Math.min(Math.max(n, -LEGEND_OFFSET_SAFETY_LIMIT), LEGEND_OFFSET_SAFETY_LIMIT);
}

/**
 * 흐름 안에 남은 채로 **밀어서 옮기는** 범례의 오프셋 죄기.
 *
 * 라인 차트 범례는 파이와 달리 겹쳐 뜨지 않는다 — 차트와 자리를 나눠 가지는 형제이며,
 * 구분선과 마지막 값 칸이 있는 표에 가깝다. 그것을 절대 배치로 띄우면 저장된 모든
 * 대시보드에서 차트가 커지고 범례가 그림 위를 덮는다. 그래서 자리는 흐름이 정하고,
 * 끌어 옮긴 값은 그 자리에서의 **상대 변위**(`position: relative` 의 left/top)로만 얹는다.
 *
 * 그래서 죄기 기준도 다르다. 붙인 변이 없으므로 앵커별 규칙(`legendOffsetBounds`)이
 * 성립하지 않는다. 대신 **지금 그려진 자리**에서 담는 상자 밖으로 나가지 않는 범위를
 * 구한다: 이미 적용된 오프셋(`base`)에, 아직 남은 여백만큼만 더 갈 수 있다.
 *
 * 상자를 잴 수 없으면 움직이지 않는다 — 죌 수 없는 값을 통과시키면 범례가 사라진다.
 */
export function clampInlineLegendOffset(
  base: { x: number; y: number },
  next: { x: number; y: number },
  container: { left: number; top: number; width: number; height: number },
  legend: { left: number; top: number; width: number; height: number },
): { x: number; y: number } {
  if (!(container.width > 0) || !(container.height > 0)) return { x: base.x, y: base.y };
  const pct = (px: number, span: number): number => (px / span) * 100;
  // 지금 자리에서 남은 여백. 왼쪽으로는 상자 왼끝까지, 오른쪽으로는 상자 오른끝까지.
  const minX = base.x + pct(container.left - legend.left, container.width);
  const maxX = base.x + pct(container.left + container.width - (legend.left + legend.width), container.width);
  const minY = base.y + pct(container.top - legend.top, container.height);
  const maxY = base.y + pct(container.top + container.height - (legend.top + legend.height), container.height);
  const fit = (v: number, fallback: number, lo: number, hi: number): number => {
    if (!Number.isFinite(v)) return fallback;
    // 범례가 상자보다 크면 lo > hi 가 된다 — 움직일 여지가 없으므로 제자리에 둔다.
    if (lo > hi) return fallback;
    return Math.min(Math.max(v, lo), hi);
  };
  return { x: fit(next.x, base.x, minX, maxX), y: fit(next.y, base.y, minY, maxY) };
}

/** 흐름 배치 범례의 상대 변위 스타일. 0,0 이면 스타일을 붙이지 않는다(저장된 그림 유지). */
export function inlineLegendOffsetStyle(x: number, y: number): CSSProperties | undefined {
  if (x === 0 && y === 0) return undefined;
  return { position: 'relative', left: `${x}%`, top: `${y}%` };
}
