// 파이 조각 라벨·범례 셀의 **문구 조립**.
//
// 순수 모듈로 두는 이유: 조각 라벨은 SVG `<text>` 안에서만 그려지고 범례는 HTML 로
// 그려지는데, 둘이 같은 값을 다르게 적으면(한쪽은 `45%`, 다른 쪽은 `45.00%`) 같은
// 조각을 두 번 읽어야 한다. 문구 규칙을 한 곳에 두고 두 화면이 함께 쓴다.
//
// 비율과 값은 **다른 축**이다. 비율은 전체 대비 비중이라 단위가 없고 정수 %로 적고,
// 값은 소스의 단위를 그대로 쓴다 — 비중 옆에 단위를 붙이면 두 수가 같은 뜻처럼 보인다.

import { formatValueWithUnit } from './unitOptions';

/** 조각 하나를 적을 때 고를 수 있는 표기. */
export interface PieTextOptions {
  showPercentage: boolean;
  showValue: boolean;
  /** 값 표기 자릿수(차트 계열 공용 규칙). */
  decimals: number;
  unit?: string;
}

/**
 * 조각 라벨을 적을 최소 비중(%) 기본값.
 *
 * recharts 는 조각 라벨끼리 겹치는지 보지 않는다 — 각 라벨은 제 조각의 중간각에서
 * 바깥으로 밀려 나올 뿐이라, 작은 조각이 몇 개 붙어 있으면 라벨이 같은 자리에 쌓인다.
 * 겹친 라벨은 둘 다 못 읽으므로, 작은 조각은 아예 적지 않고 범례로 보낸다.
 *
 * 5%는 라벨 두 줄(비율 + 값)이 서로 밀려나지 않는 최소 간격에서 잡았다. 0으로 두면
 * 종전처럼 전부 적는다.
 */
export const DEFAULT_PIE_LABEL_MIN_PERCENT = 5;

/** 이 조각에 라벨을 적을 것인가. `minPercent` 는 0~100 이고 `percent` 는 0~1 이다. */
export function isPieLabelVisible(percent: number, minPercent: number): boolean {
  if (!Number.isFinite(percent)) return false;
  if (!(minPercent > 0)) return true;
  return percent * 100 >= minPercent;
}

/** 비율(0~1)을 정수 %로 적는다 — 종전 조각 라벨과 같은 규칙. */
export function formatPiePercent(percent: number): string {
  return `${Math.round((Number.isFinite(percent) ? percent : 0) * 100)}%`;
}

/**
 * 조각 라벨 문구. 아무것도 켜지 않았으면 빈 문자열이다 — 빈 `<text>` 를 그리면
 * 조각 위에 보이지 않는 요소만 남는다.
 *
 * 비율과 값을 함께 켜도 **한 줄**로 잇는다. 두 줄로 쌓으면 라벨이 차지하는 세로
 * 높이가 두 배가 되어, 나란한 조각의 라벨과 훨씬 쉽게 겹친다.
 */
export function pieSliceLabelText(
  percent: number,
  value: number,
  opts: PieTextOptions,
): string {
  const parts: string[] = [];
  if (opts.showPercentage) parts.push(formatPiePercent(percent));
  if (opts.showValue) parts.push(formatValueWithUnit(value, opts.decimals, opts.unit));
  return parts.join(' ');
}

/**
 * 조각 **안쪽**의 라벨 자리.
 *
 * 바깥에 적으면 지시선 길이 + 글자 폭만큼 차트 영역을 넘어가 패널 경계에서 잘린다
 * (`96% 418.65GB` 처럼 값까지 적으면 특히). 안쪽은 조각이 곧 경계이므로 잘릴 수 없다.
 *
 * 반지름의 60% 지점에 놓는다 — 중심에 가까우면 좁은 조각에서 이웃과 붙고, 가장자리에
 * 가까우면 글자가 조각 밖으로 삐져나온다.
 */
export function insideLabelPoint(geom: {
  cx: number;
  cy: number;
  innerRadius: number;
  outerRadius: number;
  midAngle: number;
}): { x: number; y: number } {
  const r = geom.innerRadius + (geom.outerRadius - geom.innerRadius) * 0.6;
  // SVG 는 y축이 아래로 자라므로 각도의 부호를 뒤집는다(recharts 관용구와 동일).
  const rad = (-geom.midAngle * Math.PI) / 180;
  return { x: geom.cx + r * Math.cos(rad), y: geom.cy + r * Math.sin(rad) };
}

/**
 * 조각 색 위에서 읽히는 글자색.
 *
 * 팔레트에는 밝은 색(라임·주황)과 어두운 색(파랑·보라)이 섞여 있어 한 색으로 고정하면
 * 절반은 읽히지 않는다. sRGB 상대 휘도(WCAG 정의)로 갈라 흰색/검정을 고른다.
 * 색을 읽을 수 없으면 `currentColor` 로 두어 종전처럼 테마 글자색을 따른다.
 */
export function sliceLabelTextColor(fill: string | undefined): string {
  const m = /^#?([0-9a-f]{6})$/i.exec(fill?.trim() ?? '');
  if (!m) return 'currentColor';
  const n = parseInt(m[1]!, 16);
  const channel = (v: number): number => {
    const c = v / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  const luminance =
    0.2126 * channel((n >> 16) & 0xff) +
    0.7152 * channel((n >> 8) & 0xff) +
    0.0722 * channel(n & 0xff);
  return luminance > 0.45 ? '#111827' : '#ffffff';
}

/** 범례 한 줄에 적을 값 문구. 끄면 `undefined` — 칸 자체를 만들지 않는다. */
export function pieLegendValueText(
  value: number,
  opts: Pick<PieTextOptions, 'decimals' | 'unit'>,
): string {
  return formatValueWithUnit(value, opts.decimals, opts.unit);
}

/**
 * 조각 값들에서 각 조각의 비율(0~1)을 구한다.
 *
 * 합이 0이면(모든 값이 0) 비율을 정의할 수 없다 — 0으로 나눈 `NaN` 을 그대로 적으면
 * 라벨이 `NaN%` 가 되므로 0으로 돌린다.
 */
export function piePercents(values: readonly number[]): number[] {
  const total = values.reduce((a, b) => a + b, 0);
  if (!(total > 0)) return values.map(() => 0);
  return values.map((v) => v / total);
}
