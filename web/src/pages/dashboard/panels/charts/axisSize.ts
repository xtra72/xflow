// 축이 차지할 폭·높이 — 내용에서 산출한다.
//
// 종전에는 Y축 폭이 56px 고정이었다. 그 안에 **눈금 글자와 회전된 축 제목이
// 함께** 들어가야 하는데, 둘 중 하나만 커져도 넘친다:
//
//   - 축 제목 글꼴을 키우면(디자인 배지) 제목이 잘린다
//   - 단위를 붙이거나 소수 자릿수를 늘리면 눈금이 길어져 제목을 밀어낸다
//   - 열거형 축의 라벨("비상정지")은 숫자보다 훨씬 넓다
//
// 브라우저 밖에서는 글자 폭을 실제로 잴 수 없으므로 **글꼴 크기 기반 추정**을 쓴다.
// 추정이라 넉넉히 잡는다 — 조금 넓은 축은 눈에 거슬리는 정도지만, 좁은 축은 글자를
// 자른다. 둘 중 하나를 틀려야 한다면 넓은 쪽이 덜 나쁘다.

/** 글자 하나의 대략적인 폭(글꼴 크기 대비). */
const CHAR_WIDTH_RATIO = {
  /** 숫자·라틴 문자. */
  narrow: 0.62,
  /** 한글·한자 등 전각. */
  wide: 1.0,
} as const;

/** 축선과 눈금 표시가 먹는 여백(px). */
const AXIS_GUTTER = 10;
/** 회전된 축 제목이 먹는 가로 폭 = 글꼴 크기 × 이 값. */
const ROTATED_LABEL_BAND = 1.6;

/** 최소 폭 — 눈금이 없어도 축선이 붙을 자리는 남긴다. */
export const MIN_Y_AXIS_WIDTH = 32;

/**
 * 전각 폭으로 볼 글자 — 한글 자모 · CJK · 한글 음절 · 전각 기호.
 */
const WIDE_CHAR = /[\u1100-\u11FF\u3000-\u9FFF\uAC00-\uD7AF\uFF00-\uFFEF]/;

/** 문자열의 대략적인 픽셀 폭. 전각 문자를 두 배 폭으로 센다. */
export function estimateTextWidth(text: string, fontSize: number): number {
  let units = 0;
  for (const ch of text) {
    // 한글·CJK·전각 기호는 넓다. 그 밖(숫자·라틴·기호)은 좁게 본다.
    // 코드포인트로 적는다 — 전각 공백처럼 눈에 안 보이는 글자를 문자로 넣으면
    // 소스에서 구분되지 않는다.
    units += WIDE_CHAR.test(ch) ? CHAR_WIDTH_RATIO.wide : CHAR_WIDTH_RATIO.narrow;
  }
  return units * fontSize;
}

export interface YAxisWidthInput {
  /** 눈금에 실제로 찍힐 글자들의 표본. 가장 긴 것이 폭을 정한다. */
  tickTexts: readonly string[];
  /** 눈금 글꼴 크기(px). */
  tickFontSize: number;
  /** 축 제목. 없으면 제목 자리를 잡지 않는다. */
  label?: string;
  /** 축 제목 글꼴 크기(px). */
  labelFontSize: number;
}

/**
 * Y축 폭을 정한다.
 *
 * 폭 = 눈금 글자 + 축 여백 + (제목이 있으면) 회전된 제목 띠.
 *
 * 제목 띠를 **더하는** 것이 핵심이다. 종전처럼 고정 폭 안에 둘을 함께 밀어 넣으면
 * 눈금이 길어질수록 제목이 밖으로 밀려 잘린다.
 */
export function resolveYAxisWidth(input: YAxisWidthInput): number {
  const { tickTexts, tickFontSize, label, labelFontSize } = input;

  let tickWidth = 0;
  for (const t of tickTexts) {
    const w = estimateTextWidth(t, tickFontSize);
    if (w > tickWidth) tickWidth = w;
  }

  // 최소 폭은 **눈금 영역**에만 건다. 전체에 걸면 제목 몫까지 삼켜, 눈금이 짧을 때
  // "제목을 넣었는데 폭이 그대로" 가 된다 — 잘림 보고가 나온 자리가 여기다.
  const tickArea = Math.max(MIN_Y_AXIS_WIDTH, tickWidth + AXIS_GUTTER);
  const labelBand = label ? labelFontSize * ROTATED_LABEL_BAND : 0;
  return Math.ceil(tickArea + labelBand);
}

/** 최소 높이 — 눈금만 있을 때. */
export const MIN_X_AXIS_HEIGHT = 30;

/**
 * X축 높이를 정한다.
 *
 * 가로 제목이라 글자 폭이 아니라 **줄 높이**가 문제다. 눈금 한 줄 + 제목 한 줄이
 * 들어가야 하며, 글꼴을 키우면 같이 커져야 한다.
 */
export function resolveXAxisHeight(input: {
  tickFontSize: number;
  label?: string;
  labelFontSize: number;
}): number {
  const { tickFontSize, label, labelFontSize } = input;
  // 줄 높이는 글꼴 크기의 약 1.5배. 눈금 줄 + 축선 여백은 항상 잡는다.
  const tickLine = tickFontSize * 1.5 + AXIS_GUTTER;
  const labelLine = label ? labelFontSize * 1.6 : 0;
  return Math.max(MIN_X_AXIS_HEIGHT, Math.ceil(tickLine + labelLine));
}
