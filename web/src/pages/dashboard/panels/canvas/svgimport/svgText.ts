// 글자의 순수 산술 — 공백 정규화 · 정렬 · 크기 · 템플릿 토큰 (SPEC-CANVAS-007 · 결함 B 정정).
//
// **이 모듈은 DOM 을 모른다.** 문서 층이 트리에서 긁어 온 **문자열과 속성 자루**만 받는다.
// 글자를 다루는 산술을 여기 모아 두는 이유는 `svgDocument` 가 이미 한 몫을 하고 있어서가
// 아니라, 이 규칙들이 **jsdom 없이 재어져야** 하기 때문이다 — 공백 규칙도 정렬 대응도
// 크기 환산도 전부 문자열 → 값이며, 그 시험이 DOM 을 세우는 순간 규칙이 렌더 거동으로
// 위장한다(`canvasHitTest` · `canvasEditGeometry` 와 같은 규율).
//
// **여기서 가장 잘 오해되는 자리는 세로 기준이다.** SVG 의 `y` 는 글자가 **올라서는 기준선**
// 이고, 이 패널의 문구 요소는 `textBaseline = 'middle'` 로 그려져 기준점이 **글자 상자의 세로
// 가운데**다(실측 `drawElement.TEXT_BASELINE`). 두 뜻이 다르므로 옮기면 글자가 약 0.3em 만큼
// 올라앉는다.
//
// **그 어긋남을 보정하지 않는다.** 보정하려면 `y − 0.3 × fontSize` 를 써야 하는데 `fontSize`
// 는 **px** 이고 기하는 **캔버스 단위**다(실측: `drawElement` 가 `style.fontSize` 를 축척
// 없이 그대로 `ctx.font` 에 넣는다). 두 단위를 섞은 보정은 축척 1 에서만 참이고 사용자가
// 패널 크기를 바꾸는 순간 거짓이 된다 — `svgImportBox` §결정 1 이 상자를 잉크까지 넓히지
// 않기로 한 것과 **같은 이유로** 지킬 수 없는 약속을 좌표에 새기지 않는다.
//
// @spec SPEC-CANVAS-007 REQ-04 · REQ-05

import type { ElementAlign, ElementFontWeight, ElementStyle } from '../canvasConfig';

import type { ImportNote } from './svgImportTypes';
import { MAX_IMPORT_TEXT_LENGTH } from './svgImportTypes';
import { parseLength, parseNumberList } from './svgPathData';
import { resolveStyle, type ResolveStyleOptions, type StyleAtoms } from './svgStyle';

/**
 * `font-size` 의 초기값(사용자 단위).
 *
 * CSS 의 초기값 `medium` 이 브라우저에서 16px 이고, SVG 문서 안에서 그 16 은 **사용자 단위**
 * 로 읽힌다. 패널의 기본 글자 크기(`DEFAULT_FONT_SIZE` = 14 **px**)를 쓰지 않는 이유가 여기
 * 있다 — 그 값은 축척을 지나지 않는 화면 양이라, 작은 `viewBox` 의 문서에서 글자만 통째로
 * 커진다. 사양이 정한 값을 문서의 축척에 태우는 쪽이 **사용자가 원본 도구에서 본 그림**이다.
 */
export const SVG_INITIAL_FONT_SIZE = 16;

/**
 * 옮기지 못하는 활자 속성들. 하나라도 말했으면 문구마다 `textFontIgnored` 가 한 번 오른다.
 *
 * 이름을 **목록으로** 적는 것에 뜻이 있다 — 속성마다 사유를 따로 두면 보고가 여섯 줄로
 * 갈라지는데, 사용자가 그 여섯에 대해 할 일은 하나다(패널의 글꼴 칸에서 다시 고른다).
 * 반대로 크기의 죔(`textSizeClamped`)은 갈라 둔다: 그쪽은 **크기 칸**에서 고칠 일이다.
 */
export const UNCARRIED_TEXT_PROPS = [
  'font-family',
  'font-style',
  'letter-spacing',
  'text-decoration',
  'writing-mode',
  'textlength',
] as const;

/** 패널이 이미 그렇게 그리는 세로 기준 — 이 값이면 `dominant-baseline` 은 어긋남이 아니다. */
const CARRIED_BASELINES = new Set(['middle', 'central']);

/**
 * `xml:space` 규칙으로 글자를 한 줄로 편다(SVG 1.1 §10.15).
 *
 * - `default`: 줄바꿈을 **지우고**, 탭을 공백으로 바꾸고, 앞뒤 공백을 떼고, 이어진 공백을
 *   하나로 줄인다.
 * - `preserve`: 줄바꿈과 탭을 공백으로 바꾸고 **그 밖은 그대로 둔다**.
 *
 * 이 함수가 없으면 예쁘게 찍어 낸 문서(줄바꿈과 들여쓰기가 든 `<text>`)의 글자가 앞뒤
 * 공백을 단 채 config 에 실린다 — `fillText` 는 줄바꿈을 그리지 않으므로 화면에서는 왼쪽에
 * 빈 자리만 생기고, 사용자는 원인을 볼 수 없다. **실측**: jsdom 의 `textContent` 는
 * `"\n  앞 가운데 뒤\t끝\n"` 을 그대로 돌려준다.
 */
export function normalizeSvgText(raw: string, preserve: boolean): string {
  if (preserve) return raw.replaceAll('\n', ' ').replaceAll('\r', ' ').replaceAll('\t', ' ');
  return raw
    .replaceAll('\n', '')
    .replaceAll('\r', '')
    .replaceAll('\t', ' ')
    .replace(/ +/g, ' ')
    .trim();
}

/** `xml:space="preserve"` 인가. 그 밖의 값(`default`·부재·오타)은 전부 기본 규칙이다. */
export function preservesSpace(value: string | undefined): boolean {
  return value?.trim() === 'preserve';
}

/**
 * `text-anchor` → 이 패널의 정렬.
 *
 * `start`(그리고 부재)는 **적지 않는다** — 렌더 층의 기본이 이미 `'left'` 이고
 * (`drawElement`: `style.align ?? 'left'`), 기본값을 적어 두면 "지정 안 함" 과 "우연히
 * 기본값" 이 config 에서 구분되지 않는다(`canvasConfig` §ElementStyle 이 세운 규율).
 *
 * 오른쪽에서 왼쪽으로 쓰는 문서(`direction: rtl`)에서는 `start` 가 오른쪽이지만, 이 패널에
 * 쓰기 방향이라는 축이 없으므로 그 문서는 `writing-mode`·`direction` 과 함께
 * `textFontIgnored` 로 보고된다.
 */
export function alignFromTextAnchor(raw: string | undefined): ElementAlign | undefined {
  switch (raw?.trim().toLowerCase()) {
    case 'middle':
      return 'center';
    case 'end':
      return 'right';
    default:
      return undefined;
  }
}

/**
 * `font-weight` → **굵은가 아닌가**. 수치 굵기는 600 이상을 굵다고 본다(CSS 의 통용 경계).
 *
 * **`'normal'` 을 적지 않는다.** 렌더 층의 기본이 이미 보통 굵기이므로, 문서가 `normal` 을
 * 말했다고 그 값을 config 에 적으면 "지정 안 함" 과 "우연히 기본값" 이 구분되지 않는다 —
 * 위 `alignFromTextAnchor` 가 `start` 를 적지 않는 것과 **같은 판단**이다. 상속을 덮는 힘은
 * 그대로다: 조상이 `bold` 여도 자기가 `normal` 이면 자기 값이 이겨 굵기가 적히지 않는다.
 */
export function fontWeightFrom(raw: string | undefined): ElementFontWeight | undefined {
  const value = raw?.trim().toLowerCase();
  if (value === 'bold' || value === 'bolder') return 'bold';
  const numeric = Number(value);
  if (value !== undefined && value !== '' && Number.isFinite(numeric) && numeric >= 600) {
    return 'bold';
  }
  return undefined;
}

/** `font-size` 읽기의 결과. **읽지 못한 것과 말하지 않은 것을 가른다** — 전자만 보고에 오른다. */
export interface FontSizeRead {
  /** 사용자 단위 크기. 말하지 않았거나 읽지 못했으면 사양의 초기값이다. */
  readonly userUnits: number;
  /** 단위를 읽지 못했는가(`12pt` · `1.5em` · `120%`). */
  readonly unreadable: boolean;
}

/**
 * `font-size` 를 사용자 단위로. **단위 없는 수와 `px` 만 읽는다** — `parseLength` 와 같은
 * 규율이며, 그 함수를 그대로 쓰는 것이 요점이다(길이를 읽는 두 번째 규칙을 만들지 않는다).
 *
 * 0 이하는 읽지 못한 것으로 본다 — 크기 0 은 화면에서 글자가 사라진 상태이고, 그것을
 * 그대로 옮기면 사용자가 다시 잡을 수 없는 요소가 된다(`resizeFontSize` 가 같은 판단을 한다).
 */
export function readFontSize(raw: string | undefined): FontSizeRead {
  if (raw === undefined || raw.trim() === '') {
    return { userUnits: SVG_INITIAL_FONT_SIZE, unreadable: false };
  }
  const parsed = parseLength(raw);
  if (parsed === undefined || !(parsed > 0)) {
    return { userUnits: SVG_INITIAL_FONT_SIZE, unreadable: true };
  }
  return { userUnits: parsed, unreadable: false };
}

/** 기준점 한 축을 읽은 결과. **목록이었는지를 값으로 말한다** — 그 사실이 보고에 오른다. */
export interface AnchorCoord {
  readonly value: number;
  /** 수가 둘 이상이었는가 — 글자마다 자리를 준 문서이며, 그 배치는 옮기지 못한다. */
  readonly perGlyph: boolean;
}

/**
 * `<text x>` · `<text y>` 한 축. **목록일 수 있다**(`x="10 20 30"` — 글자마다의 자리).
 *
 * `parseLength` 를 그대로 쓰지 않는 이유가 실측에 있다: 그 함수는 단위를 죄느라 목록에서
 * `undefined` 를 내고, 그러면 기준점이 조용히 **0** 이 된다 — 잉크스케이프가 자간을 준
 * 글자에 실제로 그 형상을 내므로, 그 문서의 글자가 전부 문서 원점에 쌓인다. 그래서 여기서는
 * 수 목록으로 읽고 **첫 수**를 쓴다(사양이 첫 글자의 자리로 정한 그 수다).
 *
 * 목록의 나머지는 버린다 — 한 요소는 한 자리에 선다. 그 사실은 `textFontIgnored` 로 오른다.
 */
export function readAnchorCoord(raw: string | undefined): AnchorCoord {
  const numbers = parseNumberList(raw ?? '').filter((n) => Number.isFinite(n));
  const first = numbers[0];
  if (first === undefined) return { value: 0, perGlyph: false };
  return { value: first, perGlyph: numbers.length > 1 };
}

/**
 * 문서가 **옮기지 못할 활자**를 말했는가.
 *
 * `dominant-baseline` · `alignment-baseline` 은 값을 본다 — 도구가 흔히 내는
 * `middle`·`central` 은 이 패널이 **이미 그렇게 그리므로** 어긋남이 아니고, 그것까지
 * 보고하면 옳게 그린 것을 근사라고 말하는 잡음이 된다(위험 R7).
 */
export function ignoresTypography(
  atoms: Readonly<Record<string, string | undefined>>,
  fontSizeUnreadable: boolean,
): boolean {
  if (fontSizeUnreadable) return true;
  for (const name of UNCARRIED_TEXT_PROPS) {
    const value = atoms[name]?.trim();
    if (value !== undefined && value !== '' && value.toLowerCase() !== 'none') return true;
  }
  for (const name of ['dominant-baseline', 'alignment-baseline'] as const) {
    const value = atoms[name]?.trim().toLowerCase();
    if (value !== undefined && value !== '' && value !== 'auto' && !CARRIED_BASELINES.has(value)) {
      return true;
    }
  }
  return false;
}

/**
 * 패널이 치환할 토큰이 문구 안에 있는가.
 *
 * **문구를 고치지 않는다.** `{value}` 를 망가뜨려 놓으면 config 에 문서가 말한 적 없는
 * 문자열이 실리고, 사용자는 패널 어디에서도 원문을 되찾을 수 없다. 그대로 싣고 보고한다 —
 * 문구 칸은 사용자가 바로 고칠 수 있는 자리이므로, 보고를 읽은 사용자에게 할 일이 있다.
 *
 * 셋만 본다. 그 밖의 `{foo}` 는 `renderTextTemplate` 이 **원문 그대로 두므로**(실측 ·
 * 의도된 동작) 어긋남이 아니다.
 */
export function hasTemplateToken(text: string): boolean {
  return /\{(?:value|name|unit)\}/.test(text);
}

/** 자른 결과. **잘랐는지를 값으로 말한다** — 길이 비교를 호출부가 다시 하지 않는다. */
export interface TruncatedText {
  readonly text: string;
  readonly truncated: boolean;
}

/**
 * 글자 수 상한으로 자른다. 넘지 않으면 **같은 문자열을 그대로** 돌려준다.
 *
 * 자른 자리에 말줄임표를 넣지 않는다 — 그 세 점은 문서가 말한 적 없는 글자이고, 잘렸다는
 * 사실은 보고가 말한다.
 */
export function truncateImportText(text: string): TruncatedText {
  if (text.length <= MAX_IMPORT_TEXT_LENGTH) return { text, truncated: false };
  return { text: text.slice(0, MAX_IMPORT_TEXT_LENGTH), truncated: true };
}


// --- 스타일 ------------------------------------------------------------

/** 글자 스타일을 푼 결과. **크기를 `style` 밖에 둔다**(아래 머리말). */
export interface ResolvedTextStyle {
  /** `textColor` · `opacity` · `fontWeight` · `align`. **`fontSize` 는 여기 없다.** */
  readonly style: ElementStyle;
  /** **사용자 단위** 글자 크기. px 가 아니므로 `ElementStyle` 에 실을 수 없다. */
  readonly fontSizeUserUnits: number;
  readonly notes: readonly ImportNote[];
  /** SVG 가 칠을 한 마디라도 말했는가 — 아니면 002 의 문구 씨앗 색이 선다. */
  readonly hasOwnStyle: boolean;
  /** 옮기지 못한 활자를 말했는가(`textFontIgnored` 의 입력). */
  readonly typographyIgnored: boolean;
  /**
   * 글자에 **테**가 그려지고 있었는가.
   *
   * 이 값이 있어야 `fill="none"` 인 글자의 두 갈래가 갈린다 — 테도 없으면 문서에서도
   * 보이지 않던 글자(감춤)이고, 테가 있으면 **보이던 글자를 옮기지 못한 것**(버림)이다.
   * 이 패널은 글자에 테를 두르지 않으므로(`paintText` 는 `fillText` 하나다) 두 경우 모두
   * 요소가 서지 않지만, 사용자가 읽어야 하는 문장이 서로 다르다.
   */
  readonly strokePainted: boolean;
}

/**
 * 칠 자루와 활자 자루를 문구 요소의 스타일로.
 *
 * **칠은 `resolveStyle` 을 그대로 쓴다** — 그라디언트 풀기 · 알파 접기 · 씨앗 폴백의 규칙이
 * 도형과 문구에서 갈라지면 같은 색이 종류에 따라 다르게 옮겨진다. 그 함수가 낸 `fill` 을
 * **`textColor` 로 옮겨 싣는 것**이 이 함수가 하는 일의 절반이다: 렌더 층은 문구를
 * `textColor ?? fill` 로 칠하므로 `fill` 그대로도 그려지지만, 그 자리에 값을 두면 "문구
 * 요소인데 채움색을 갖는" 형상이 되어 편집기의 글자색 칸과 config 가 어긋난다.
 *
 * **선은 싣지 않는다.** 이 패널은 글자에 테를 두르지 않으므로(`paintText` 는 `fillText` 하나다)
 * `stroke`/`strokeWidth` 를 실으면 그리지 않는 값이 config 를 먹는다. 다만 `stroke` 만 말한
 * 글자가 **검게 칠해지는** 사양의 규칙은 `resolveStyle` 안에 그대로 살아 있다.
 *
 * **크기는 `style` 밖으로 낸다.** 여기서 나오는 수는 **사용자 단위**이고 `ElementStyle.fontSize`
 * 는 **px** 다. 두 단위를 한 필드에 담으면 계획 층이 그것을 px 로 읽어 축척을 두 번 곱하거나
 * 한 번도 곱하지 않는 길이 열린다.
 */
export function resolveTextStyle(
  paintAtoms: StyleAtoms,
  textAtoms: StyleAtoms,
  options: ResolveStyleOptions & { readonly fontScale?: number; readonly nonUniform?: boolean } = {},
): ResolvedTextStyle {
  const base = resolveStyle(paintAtoms, options);
  const style: { -readonly [K in keyof ElementStyle]: ElementStyle[K] } = {};
  if (base.style.fill !== undefined) style.textColor = base.style.fill;
  if (base.style.opacity !== undefined) style.opacity = base.style.opacity;
  const weight = fontWeightFrom(textAtoms['font-weight']);
  if (weight !== undefined) style.fontWeight = weight;
  const align = alignFromTextAnchor(textAtoms['text-anchor']);
  if (align !== undefined) style.align = align;
  const size = readFontSize(textAtoms['font-size']);
  return {
    style,
    fontSizeUserUnits: size.userUnits * (options.fontScale ?? 1),
    notes: base.notes,
    hasOwnStyle: base.hasOwnStyle,
    typographyIgnored:
      ignoresTypography(textAtoms, size.unreadable) || options.nonUniform === true,
    strokePainted: base.style.stroke !== undefined,
  };
}
