// 팝오버 배치 산술 — 순수 함수.
//
// 오늘 이 산술은 `ColorSwatchButton.place` 안에 있고, **jsdom 에 레이아웃이 없어서
// 컴포넌트로는 잴 수가 없다**(`getBoundingClientRect` 가 전부 0). 컴포넌트 밖으로
// 꺼내는 것이 이 파일의 전부이며, 규칙 자체는 오늘 것을 그대로 잇는다.
//
// @spec SPEC-COLOR-001 §명세 · 팝오버 배치 (M4)

/**
 * 팝오버 폭. 옛 `ColorSwatchButton` 은 156 이었다 — 한 행 8칸 팔레트 기준이다.
 * 통합 팔레트는 한 행이 10칸이라 폭을 다시 잡는다(칸 20 + 사이 4 → 10칸 236 + 여백).
 */
export const POPOVER_WIDTH = 248;

/** 트리거와 팝오버 사이 틈. */
export const POPOVER_GAP = 4;

/** 화면 가장자리에서 남기는 여백. */
export const POPOVER_MARGIN = 8;

/**
 * 높이를 재지 못했을 때 쓰는 추정값.
 *
 * `offsetHeight` 가 `0` 이면 "높이가 0" 이 아니라 **"재지 못했다"** 는 뜻이다 —
 * 레이아웃이 없는 환경(jsdom)과 첫 렌더가 그렇다. 0 을 그대로 믿으면 뒤집기 판정이
 * 항상 "안 넘침" 으로 떨어진다.
 */
export const POPOVER_FALLBACK_HEIGHT = 320;

/** 트리거의 화면 좌표 — `DOMRect` 에서 쓰는 네 값만. */
export interface TriggerRect {
  readonly top: number;
  readonly bottom: number;
  readonly right: number;
}

export interface PlacementInput {
  readonly trigger: TriggerRect;
  /** 실제로 잰 팝오버 높이. `0`(또는 미측정)이면 폴백을 쓴다. */
  readonly popoverHeight: number;
  readonly viewportHeight: number;
}

export interface Placement {
  readonly top: number;
  readonly left: number;
  /** 위로 뒤집었는가 — 시험이 "뒤집었다" 를 좌표 산수 없이 단언할 수 있게 낸다. */
  readonly flipped: boolean;
}

/**
 * 아래로 열되 화면을 넘으면 위로 뒤집고, 좌우는 화면 안에 가둔다.
 *
 * 우측 정렬이 기본인 것은 색 자리 다수가 표 행의 오른쪽 끝이기 때문이다. 왼쪽으로
 * 넘치면 여백까지만 가둔다 — 넘친 채로 두면 팔레트 왼쪽 칸을 누를 수 없다.
 */
export function placePopover(input: PlacementInput): Placement {
  const { trigger, popoverHeight, viewportHeight } = input;
  const height = popoverHeight > 0 ? popoverHeight : POPOVER_FALLBACK_HEIGHT;

  const below = trigger.bottom + POPOVER_GAP;
  const overflows = below + height > viewportHeight - POPOVER_MARGIN;
  const raw = overflows ? trigger.top - height - POPOVER_GAP : below;

  return {
    top: Math.max(POPOVER_MARGIN, raw),
    left: Math.max(POPOVER_MARGIN, trigger.right - POPOVER_WIDTH),
    flipped: overflows,
  };
}
