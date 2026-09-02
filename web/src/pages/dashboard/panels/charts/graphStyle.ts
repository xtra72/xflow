// 그래프 스타일 축 — 시리즈를 어떤 모양으로 그릴지.
//
// 패널이 기본값을 갖고, 시리즈가 개별로 덮어쓴다. 온도는 라인·가동량은 바처럼
// 한 차트 안에서 섞어 그릴 수 있어야 하기 때문이다. 스타일별 제약(스택킹 가능
// 여부, 선 모양이 뜻을 갖는지)도 한 곳에서 답한다 — 렌더러와 설정 UI가 서로
// 다른 답을 내면 "설정은 켰는데 그림은 안 바뀐다" 가 된다.

/** 시리즈를 그리는 모양. */
export type GraphStyle = 'line' | 'area' | 'bar' | 'candle';

export const GRAPH_STYLES: readonly GraphStyle[] = ['line', 'area', 'bar', 'candle'];

/** 패널 기본 스타일. 지정이 없으면 종전 동작(라인)이다. */
export const DEFAULT_GRAPH_STYLE: GraphStyle = 'line';

/** 문자열을 스타일로 읽는다. 모르는 값은 기본값으로 떨어뜨린다. */
export function readGraphStyle(v: unknown, fallback: GraphStyle = DEFAULT_GRAPH_STYLE): GraphStyle {
  return (GRAPH_STYLES as readonly string[]).includes(v as string) ? (v as GraphStyle) : fallback;
}

/**
 * 시리즈 하나의 실제 스타일. 시리즈 지정이 없으면 패널 기본값을 따른다.
 *
 * 시리즈 값이 `undefined` 인 것과 `'line'` 인 것은 다르다 — 전자는 "패널을 따름"
 * 이라 패널 기본값을 바꾸면 같이 바뀌고, 후자는 고정이다.
 */
export function resolveSeriesStyle(
  seriesStyle: unknown,
  panelStyle: GraphStyle,
): GraphStyle {
  if (seriesStyle === undefined || seriesStyle === null || seriesStyle === '') return panelStyle;
  return readGraphStyle(seriesStyle, panelStyle);
}

/**
 * 스택킹이 뜻을 갖는 스타일인가.
 *
 * 라인은 쌓아도 겹친 선이 될 뿐 누적으로 읽히지 않는다 — 누적을 보려면 영역으로
 * 바꿔야 한다. 캔들은 시가·고가·저가·종가가 한 덩어리라 쌓을 수 없다.
 */
export function isStackable(style: GraphStyle): boolean {
  return style === 'area' || style === 'bar';
}

/** 선 모양(실선·파선·점선)과 곡선이 뜻을 갖는 스타일인가. */
export function hasStrokeStyle(style: GraphStyle): boolean {
  return style === 'line' || style === 'area';
}

/**
 * 결측 점선 덧그림이 뜻을 갖는 스타일인가.
 *
 * 바·캔들은 값이 없으면 막대가 서지 않아 결측이 이미 눈에 보인다. 그 위에 점선을
 * 더 그으면 없는 막대를 잇는 선이 되어 오히려 지어낸 그림이 된다.
 */
export function hasGapDash(style: GraphStyle): boolean {
  return style === 'line' || style === 'area';
}

/**
 * 캔들은 버킷마다 네 값(시·고·저·종)이 필요해 집계 소스에서만 그릴 수 있다.
 * 채널(실시간) 모드는 버킷 개념이 없어 지원하지 않는다.
 */
export function requiresBuckets(style: GraphStyle): boolean {
  return style === 'candle';
}

/**
 * 스택킹을 실제로 적용할지. 켜져 있어도 스타일이 받쳐 주지 않으면 무시한다.
 * 같은 판정을 렌더러와 설정 UI가 공유해야 둘이 어긋나지 않는다.
 */
export function effectiveStacked(style: GraphStyle, stacked: boolean | undefined): boolean {
  return stacked === true && isStackable(style);
}
