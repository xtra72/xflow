// 차트 글자 스타일 — 글꼴·크기·색.
//
// 글꼴은 **토큰**으로 저장하고 렌더 시점에 스택으로 편다. CSS 스택 문자열을 그대로
// 저장하면 (a) config 에 긴 문자열이 박혀 나중에 대체 글꼴을 고칠 수 없고, (b) 사용자가
// 시스템에 없는 글꼴 이름을 적어 조용히 기본값으로 떨어지는 일이 생긴다.
//
// 미지정은 **상속**이다. 기본값을 채워 넣으면 저장된 패널의 글자가 조용히 바뀐다.

/** 고를 수 있는 글꼴. 실제 글꼴 파일을 싣지 않고 시스템 글꼴만 쓴다. */
export type ChartFontFamily = 'sans' | 'serif' | 'mono';

/**
 * 토큰 → CSS `font-family` 스택.
 *
 * 각 스택은 `ui-*` 우선이라 OS 기본 글꼴을 먼저 쓰고, 없으면 널리 깔린 이름으로
 * 내려간다. 마지막 총칭(sans-serif 등)은 어느 환경에서도 해석되는 안전망이다.
 */
export const FONT_FAMILY_STACKS: Record<ChartFontFamily, string> = {
  sans: 'ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif',
  serif: 'ui-serif, Georgia, "Times New Roman", serif',
  mono: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
};

/** 설정 화면의 선택지 순서 — 토큰과 라벨 i18n 키. */
export const FONT_FAMILY_OPTIONS: ReadonlyArray<{ value: ChartFontFamily; labelKey: string }> = [
  { value: 'sans', labelKey: 'dashboard.chart.fontSans' },
  { value: 'serif', labelKey: 'dashboard.chart.fontSerif' },
  { value: 'mono', labelKey: 'dashboard.chart.fontMono' },
];

export function isChartFontFamily(v: unknown): v is ChartFontFamily {
  return typeof v === 'string' && v in FONT_FAMILY_STACKS;
}

/**
 * 글꼴 토큰을 CSS 스택으로 편다. 미지정·인식 불가 값은 `undefined` — 속성을 붙이지
 * 않아 상위에서 상속된다(인식 불가 값에 임의 글꼴을 씌우지 않는다).
 */
export function resolveFontFamily(v: unknown): string | undefined {
  return isChartFontFamily(v) ? FONT_FAMILY_STACKS[v] : undefined;
}

/**
 * 글자 크기(px). 양수가 아니면 미지정으로 본다 — 0이나 음수를 그대로 쓰면 글자가
 * 사라져 되돌릴 수단이 화면에서 없어진다.
 */
export function resolveFontSize(v: unknown): number | undefined {
  return typeof v === 'number' && Number.isFinite(v) && v > 0 ? v : undefined;
}

/**
 * 글자색(`#rgb` / `#rrggbb`). 형식이 아니면 미지정 — 임의 문자열을 `fill` 에 그대로
 * 넘기면 브라우저가 조용히 검정으로 떨어뜨려 어두운 배경에서 글자가 사라진다.
 */
export function resolveFontColor(v: unknown): string | undefined {
  return typeof v === 'string' && /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.test(v.trim())
    ? v.trim()
    : undefined;
}
