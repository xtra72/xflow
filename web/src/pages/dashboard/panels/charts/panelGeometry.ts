// 패널 안에서 **그림 하나를 키우고 옮기는** 규칙 — 파이와 게이지가 공유한다.
//
// 두 패널의 그림은 그리는 방식이 전혀 다르지만(파이는 recharts 가 `cx`/`cy`/반지름을
// 받고, 게이지는 6종의 SVG 가 제 viewBox 로 그린다) **크기와 자리를 다루는 규칙은 같다**.
// 규칙을 각자 두면 같은 슬라이더가 패널마다 다른 범위를 갖거나, 한쪽만 상한을 잃는다.
//
// 크기·오프셋을 모두 **백분율**로 두는 이유: 픽셀로 두면 패널 크기를 바꿀 때 상대 위치와
// 비율이 어긋난다. 백분율은 패널이 커지든 작아지든 같은 그림을 준다.

/**
 * 중심 오프셋의 상한(백분율 포인트).
 *
 * 40 을 넘기면 그림이 영역 가장자리에 붙어 절반 이상이 잘린다 — 되돌릴 수는 있지만
 * 그 상태에서 무엇을 보고 있는지 알 수 없다.
 */
export const PANEL_OFFSET_LIMIT = 40;

/** 그림 크기(%) 허용 범위. 너무 작으면 안 보이고, 100 을 넘으면 영역 밖이다. */
/**
 * 막대 굵기(px)의 범위와 손잡이 시작값.
 *
 * `bar_size` 를 두지 않으면 recharts 가 칸 폭에 맞춰 자동으로 정하므로, 지금 화면에
 * 그려진 굵기를 코드가 알 수 없다. 손잡이를 처음 끌 때는 이 기본값에서 시작하며 —
 * 그 순간 굵기가 한 번 튄다. 자동값을 읽어 올 방법이 없어 받아들인 절충이다.
 */
export const BAR_SIZE_MIN = 2;
export const BAR_SIZE_MAX = 200;
export const BAR_SIZE_DEFAULT = 24;

/** 막대 굵기를 읽는다. 범위 밖이거나 수가 아니면 `undefined`(= 자동). */
export function readBarSize(v: unknown): number | undefined {
  return typeof v === 'number' && Number.isFinite(v) && v >= BAR_SIZE_MIN && v <= BAR_SIZE_MAX
    ? v
    : undefined;
}

export const PANEL_SIZE_MIN = 20;
export const PANEL_SIZE_MAX = 100;

/** 픽셀 이동량을 기준 변 대비 백분율로 환산한다. 기준 변을 잴 수 없으면 0. */
export function pixelsToPercent(px: number, span: number): number {
  if (!Number.isFinite(px) || !Number.isFinite(span) || span <= 0) return 0;
  return (px / span) * 100;
}

/** 백분율 오프셋을 ±limit 로 죈다. */
export function clampPercentOffset(value: number, limit = PANEL_OFFSET_LIMIT): number {
  if (!Number.isFinite(value)) return 0;
  return Math.min(Math.max(value, -limit), limit);
}

/**
 * 그림 크기(%)를 읽는다. 범위를 벗어나면 미지정으로 본다 — 0 이하는 그림이 사라지고,
 * 100 을 넘으면 영역 밖으로 나가 어느 쪽도 화면에서 되돌릴 수 없다.
 */
export function readPanelSize(v: unknown): number | undefined {
  return typeof v === 'number' && Number.isFinite(v) && v >= PANEL_SIZE_MIN && v <= PANEL_SIZE_MAX
    ? v
    : undefined;
}

/** 오프셋(백분율)을 읽는다. 수가 아니면 0 — 구 config 에는 이 키가 없다. */
export function readPanelOffset(v: unknown): number {
  return clampPercentOffset(typeof v === 'number' && Number.isFinite(v) ? v : 0);
}

/**
 * 그림 상자의 CSS transform — 옮긴 뒤 키운다.
 *
 * 순서가 뜻을 정한다. CSS transform 목록은 오른쪽부터 요소에 적용되므로
 * `translate(...) scale(...)` 은 "줄인 뒤 옮긴다" 가 되고, translate 의 백분율은 요소의
 * **원래** 크기를 기준으로 하므로 오프셋이 크기에 휘둘리지 않는다 — 크기를 줄여도
 * "패널 폭의 10% 만큼 오른쪽" 이라는 뜻이 유지된다.
 *
 * 기본값(크기 100%, 오프셋 0)이면 `undefined` — 불필요한 transform 을 남기지 않는다
 * (transform 이 걸리면 브라우저가 별도 레이어를 만들어 렌더 비용이 붙는다).
 */
export function panelBoxTransform(
  sizePercent: number,
  offsetXPercent: number,
  offsetYPercent: number,
  /**
   * 세로 배율. 생략하면 가로와 같다(종전 동작 — 저장된 설정이 그대로 동작한다).
   *
   * 축을 나누는 이유는 **사각형 도형** 때문이다. 세로바 게이지와 바 차트 그림 영역은
   * 사각형이라 "가늘고 길게" 나 "굵고 짧게" 가 뜻을 갖는데, 배율이 하나뿐이면 그
   * 모양을 만들 수단이 없다. 원형 게이지처럼 비율이 뜻을 갖는 도형은 이 값을 주지
   * 않으면 되므로, 한 함수로 둘 다 표현된다.
   */
  sizeYPercent?: number,
): string | undefined {
  const scale = sizePercent / 100;
  const scaleY = sizeYPercent === undefined ? scale : sizeYPercent / 100;
  if (scale === 1 && scaleY === 1 && !offsetXPercent && !offsetYPercent) return undefined;
  // 두 축이 같으면 한 값으로 적는다 — 같은 뜻을 두 벌로 쓰면 diff 와 스냅샷이 흔들린다.
  const s = scale === scaleY ? `${scale}` : `${scale}, ${scaleY}`;
  return `translate(${offsetXPercent}%, ${offsetYPercent}%) scale(${s})`;
}
