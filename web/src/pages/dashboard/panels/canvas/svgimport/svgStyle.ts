// 스타일 추출 (SPEC-CANVAS-007 M4).
//
// **`ElementStyle` 은 정확히 아홉 축인데 007 이 쓰는 것은 다섯이다**(실측
// `canvasConfig.ts:190–204`): `fill` · `stroke` · `strokeWidth` · `opacity` · `visible`.
// 나머지 넷(`fontSize` · `fontWeight` · `textColor` · `align`)은 **글자 축**이며 007 은
// `<text>` 를 받지 않으므로 원리적으로 쓰이지 않는다. 이 분해가 §결정 2 의 `<text>` 거절과
// 같은 사실의 두 얼굴이다.
//
// **알파를 색 문자열에 접는다 — 근사가 아니라 정확한 옮김이다.** SVG 는 `opacity` ·
// `fill-opacity` · `stroke-opacity` 셋을 따로 갖고 `ElementStyle` 은 `opacity` 하나만
// 갖는다. 그런데 `paintFill` 이 `ctx.fillStyle = style.fill` 로 **문자열을 그대로 넘기므로**
// (실측 178–182행) `rgba()` 와 8자리 hex 가 살아서 지나간다.
// ```
// style.fill    ← 색에 (fill-opacity)   를 접은 rgba/8자리 hex
// style.stroke  ← 색에 (stroke-opacity) 를 접은 rgba/8자리 hex
// style.opacity ← 요소 수준 opacity 그대로
// ```
// **셋을 곱해 하나로 뭉개는 안을 기각한다** — 채움과 선의 불투명도가 다른 도형에서 둘 중
// 하나가 조용히 틀어진다.
//
// **접을 수 없는 색 표기가 있다는 사실은 감추지 않는다.** 이름 색(`red`)에 알파를 접으려면
// 이름→RGB 표(148 항목)가 필요하고, 그것은 신규 코드로 이 SPEC 의 크기가 아니다. 그 경우
// **알파를 잃고 근사로 보고한다**(`opacityUnfoldable`). hex 와 `rgb()` 는 정확히 접힌다 —
// 실사용 파일의 압도적 다수가 그 둘이다.
//
// **`<style>` 안의 CSS 를 적용하지 않는 것은 게으름이 아니라 경계의 귀결이다.** 캐스케이드를
// 옳게 적용하는 길은 `getComputedStyle`(요소가 **살아 있는 문서 안에** 있어야 한다 → 불변식
// K3 이 깨진다. 게다가 jsdom 에는 레이아웃이 없어 시험에서 서지 않는다) 또는 선택자 엔진을
// 들이는 것(신규 의존성이거나 수백 줄, 특이도·상속·`!important` 까지 따라온다)뿐이다.
//
// **`opacity` 는 상속되지 않는다.** SVG 의 그룹 불투명도는 "자식들을 따로 그린 뒤 그 결과
// 전체에 알파를 곱한다" 이지 상속이 아니다. 007 은 대안(그룹 노드)이 없으므로 **자식마다
// 곱하고 그 사실을 근사로 보고한다** — 겹친 자리가 진해지는 성질을 감추지 않는다.
//
// @spec SPEC-CANVAS-007 REQ-05 · AC-08

import type { ElementStyle } from '../canvasConfig';

import { parseLength } from './svgPathData';
import type { AttrBag } from './svgShapes';
import type { ImportNote, ImportNoteReason } from './svgImportTypes';

/** 이름 → 값. 표현 속성과 `style` 속성이 이미 합쳐진 뒤의 모습이다. */
export type StyleAtoms = Readonly<Record<string, string>>;

/**
 * SVG 사양이 **상속으로 정한** 속성 가운데 007 이 읽는 것.
 *
 * `opacity` 가 여기 없는 것이 이 목록의 요점이다(위 머리말). `display` 도 없다 — CSS 에서
 * 상속되지 않으며, 007 은 그것을 **순회의 가지치기**로 다룬다(`isHidden`).
 */
export const INHERITED_STYLE_PROPS = [
  'fill',
  'stroke',
  'stroke-width',
  'fill-rule',
  'fill-opacity',
  'stroke-opacity',
] as const;

/** 표현 속성으로도 읽는 스타일 이름 전부(상속 여부와 무관하다). */
const STYLE_PROP_NAMES = [
  ...INHERITED_STYLE_PROPS,
  'opacity',
  'display',
  'visibility',
  'stroke-dasharray',
  'filter',
  'clip-path',
  'mask',
  'stop-color',
  'stop-opacity',
] as const;

/**
 * `style="fill:red;stroke:none"` → 이름/값 표.
 *
 * 값 안의 콜론(`url(a:b)`)을 지키기 위해 **첫 콜론에서만** 자른다.
 */
export function parseStyleAttribute(raw: string | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  if (raw === undefined) return out;
  for (const chunk of raw.split(';')) {
    const colon = chunk.indexOf(':');
    if (colon === -1) continue;
    const name = chunk.slice(0, colon).trim().toLowerCase();
    const value = chunk.slice(colon + 1).trim();
    if (name !== '' && value !== '') out[name] = value;
  }
  return out;
}

/**
 * 한 요소가 스스로 말한 스타일 — 표현 속성 + `style` 속성.
 *
 * **`style` 속성이 이긴다**(CSS 캐스케이드에서 인라인 선언의 우선순위가 표현 속성보다
 * 높다). 이 순서가 뒤집히면 `fill="red" style="fill:blue"` 인 도형이 빨갛게 나온다.
 */
export function collectStyleAtoms(attrs: AttrBag): Record<string, string> {
  const out: Record<string, string> = {};
  for (const name of STYLE_PROP_NAMES) {
    const value = attrs[name];
    if (value !== undefined && value.trim() !== '') out[name] = value.trim();
  }
  return { ...out, ...parseStyleAttribute(attrs['style']) };
}

/**
 * 조상에서 물려받은 것과 자신이 말한 것을 합친다.
 *
 * **상속 목록에 든 것만 내려온다.** 이것은 CSS 엔진이 아니라 트리 순회의 부수 효과이며
 * `transform` 을 쌓는 것과 같은 층이다.
 */
export function inheritStyleAtoms(
  parent: StyleAtoms,
  own: StyleAtoms,
): Record<string, string> {
  const inherited: Record<string, string> = {};
  for (const name of INHERITED_STYLE_PROPS) {
    const value = parent[name];
    if (value !== undefined) inherited[name] = value;
  }
  return { ...inherited, ...own };
}

/**
 * 이 도형이 그려지지 않는가 — `display:none` 또는 `visibility:hidden`.
 *
 * **`visible: false` 로 가져오는 안을 기각한다.** 그렇게 하면 사용자가 볼 수 없는 요소가
 * 요소 상한과 명령 상한을 먹는다 — 보이는 그림이 상한에 걸려 거절되었는데 원인이 화면에
 * 없는 도형인 상황이 생긴다. **만들지 않고 개수를 보고한다.**
 */
export function isHidden(atoms: StyleAtoms): boolean {
  return atoms['display'] === 'none' || atoms['visibility'] === 'hidden';
}

// --- 색과 알파 ----------------------------------------------------------

/** 칠 하나의 갈래. `'ref'` 만 문서 층의 도움(그라디언트 조회)을 요구한다. */
export type Paint =
  | { readonly kind: 'none' }
  | { readonly kind: 'color'; readonly value: string }
  | { readonly kind: 'ref'; readonly id: string }
  | { readonly kind: 'unsupported' };

const HEX_COLOR = /^#(?:[0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})$/i;
const FUNCTIONAL_COLOR = /^(?:rgb|rgba|hsl|hsla)\(/i;
const NAMED_COLOR = /^[a-z]+$/i;
const URL_REF = /^url\(\s*(['"]?)#([^)'"\s]+)\1\s*\)$/i;

/**
 * 칠 문자열 하나를 갈래로 나눈다.
 *
 * **모르는 표기를 색으로 통과시키지 않는 것이 요점이다.** `ctx.fillStyle = '알 수 없는 것'`
 * 은 예외를 내지 않고 **대입이 무시되어 직전 색이 그대로 쓰인다** — 도형이 엉뚱한 색으로
 * 조용히 그려지는, 이 층에서 가장 나쁜 실패 형상이다. `currentColor` 가 정확히 그런
 * 표기다(문맥 색이 007 에 없다).
 */
export function normalizePaint(raw: string | undefined): Paint {
  if (raw === undefined) return { kind: 'unsupported' };
  const value = raw.trim();
  if (value === '') return { kind: 'unsupported' };
  const lower = value.toLowerCase();
  if (lower === 'none' || lower === 'transparent') return { kind: 'none' };
  const ref = URL_REF.exec(value);
  if (ref !== null) return { kind: 'ref', id: ref[2] ?? '' };
  if (lower === 'currentcolor' || lower === 'inherit' || lower === 'context-fill') {
    return { kind: 'unsupported' };
  }
  if (HEX_COLOR.test(value) || FUNCTIONAL_COLOR.test(value) || NAMED_COLOR.test(value)) {
    return { kind: 'color', value };
  }
  return { kind: 'unsupported' };
}

/** `0..1` 로 죈 불투명도. 읽을 수 없으면 `undefined`(= 지정하지 않음). */
export function parseOpacity(raw: string | undefined): number | undefined {
  if (raw === undefined) return undefined;
  const trimmed = raw.trim();
  const percent = trimmed.endsWith('%');
  const value = parseLength(percent ? trimmed.slice(0, -1) : trimmed);
  if (value === undefined) return undefined;
  const ratio = percent ? value / 100 : value;
  return Math.min(1, Math.max(0, ratio));
}

function hexPair(component: string): number {
  return Number.parseInt(component.length === 1 ? component + component : component, 16);
}

function foldHex(value: string, alpha: number): string | undefined {
  const body = value.slice(1);
  const short = body.length === 3 || body.length === 4;
  const step = short ? 1 : 2;
  if (body.length !== 3 && body.length !== 4 && body.length !== 6 && body.length !== 8) {
    return undefined;
  }
  const r = hexPair(body.slice(0, step));
  const g = hexPair(body.slice(step, step * 2));
  const b = hexPair(body.slice(step * 2, step * 3));
  const existing = body.length === 4 || body.length === 8 ? hexPair(body.slice(step * 3)) / 255 : 1;
  return `rgba(${r}, ${g}, ${b}, ${round3(existing * alpha)})`;
}

/** `rgb(…)`/`rgba(…)` 의 성분. 퍼센트 표기도 받는다. */
function foldRgb(value: string, alpha: number): string | undefined {
  const open = value.indexOf('(');
  const close = value.lastIndexOf(')');
  if (open === -1 || close === -1) return undefined;
  const parts = value
    .slice(open + 1, close)
    .split(/[\s,/]+/)
    .map((part) => part.trim())
    .filter((part) => part !== '');
  if (parts.length !== 3 && parts.length !== 4) return undefined;
  const channels: number[] = [];
  for (const part of parts.slice(0, 3)) {
    const percent = part.endsWith('%');
    const n = parseLength(percent ? part.slice(0, -1) : part);
    if (n === undefined) return undefined;
    channels.push(Math.min(255, Math.max(0, Math.round(percent ? (n / 100) * 255 : n))));
  }
  const existing = parts.length === 4 ? (parseOpacity(parts[3]) ?? 1) : 1;
  return `rgba(${channels[0]}, ${channels[1]}, ${channels[2]}, ${round3(existing * alpha)})`;
}

function round3(value: number): number {
  return Math.round(value * 1000) / 1000;
}

/**
 * 색 문자열에 알파를 접는다. **접을 수 없으면 그 사실을 값으로 말한다.**
 *
 * 알파가 1 이면 접을 것이 없으므로 어떤 표기든 그대로 지나간다(`exact: true`) — 이름 색이
 * 대다수의 파일에서 아무 손실 없이 사는 이유다.
 */
export function foldAlphaIntoColor(
  color: string,
  alpha: number,
): { readonly value: string; readonly exact: boolean } {
  if (!(alpha < 1)) return { value: color, exact: true };
  const folded = color.startsWith('#') ? foldHex(color, alpha) : foldRgb(color, alpha);
  if (folded !== undefined) return { value: folded, exact: true };
  // 이름 색 등 — 표를 들이지 않는 대신 알파를 잃고 보고한다(위 머리말).
  return { value: color, exact: false };
}

// --- 산출 --------------------------------------------------------------

export interface ResolveStyleOptions {
  /** `url(#id)` 를 단색으로 푸는 콜백(첫 `stop-color`). 문서 층이 넘긴다. */
  readonly resolvePaintRef?: (id: string) => string | undefined;
  /** 선 두께에 곱할 배율 — 변환 `√|det|` × (상자 ÷ viewBox). 호출부가 합성해 넘긴다. */
  readonly strokeScale?: number;
  /** 조상 `<g opacity>` 의 누적 곱. 상속이 아니라 **자식별 곱**이다(근사). */
  readonly groupOpacity?: number;
  /** 변환이 비균등이라 `√|det|` 가 근사인가. */
  readonly nonUniformStroke?: boolean;
  /** 스타일을 말하지 않은 도형의 씨앗 색(008 `SEED_COLOR`). */
  readonly fallbackColor?: string;
}

export interface ResolvedImportStyle {
  readonly style: ElementStyle;
  readonly notes: readonly ImportNote[];
  /** SVG 가 칠을 한 마디라도 말했는가 — 아니면 008 의 `pathSeedStyle` 이 선다. */
  readonly hasOwnStyle: boolean;
  /** `fill-rule="evenodd"` 인가 — 감김 무리 뒤집기(M7)의 입력. */
  readonly evenOdd: boolean;
}

function note(kind: ImportNote['kind'], reason: ImportNoteReason): ImportNote {
  return { kind, reason, count: 1 };
}

/**
 * 속성 자루 하나를 `ElementStyle` 과 보고 항목들로.
 *
 * **보고를 여기서 발행한다** — 스타일이 근사/버림을 아는 유일한 층이기 때문이다. 문서
 * 층은 개수를 합칠 뿐이다.
 */
export function resolveStyle(atoms: StyleAtoms, options: ResolveStyleOptions = {}): ResolvedImportStyle {
  const notes: ImportNote[] = [];
  const strokeScale = options.strokeScale ?? 1;
  const fallbackColor = options.fallbackColor;

  /** 칠 하나를 색 문자열로 — 참조는 풀고, 모르는 표기는 씨앗으로 떨어뜨린다. */
  const resolvePaint = (raw: string | undefined, opacity: number): string | undefined => {
    const paint = normalizePaint(raw);
    let color: string | undefined;
    switch (paint.kind) {
      case 'none':
        return undefined;
      case 'color':
        color = paint.value;
        break;
      case 'ref': {
        const resolved = options.resolvePaintRef?.(paint.id);
        color = resolved ?? fallbackColor;
        notes.push(note('approximated', resolved === undefined ? 'paintUnresolved' : 'gradientToSolid'));
        break;
      }
      case 'unsupported':
        if (raw === undefined) return undefined;
        color = fallbackColor;
        notes.push(note('approximated', 'paintUnresolved'));
        break;
    }
    if (color === undefined) return undefined;
    const folded = foldAlphaIntoColor(color, opacity);
    if (!folded.exact) notes.push(note('approximated', 'opacityUnfoldable'));
    return folded.value;
  };

  const fillRaw = atoms['fill'];
  const strokeRaw = atoms['stroke'];
  const hasOwnStyle = fillRaw !== undefined || strokeRaw !== undefined;

  const style: { -readonly [K in keyof ElementStyle]: ElementStyle[K] } = {};

  const fill = resolvePaint(fillRaw, parseOpacity(atoms['fill-opacity']) ?? 1);
  if (fill !== undefined) style.fill = fill;

  const stroke = resolvePaint(strokeRaw, parseOpacity(atoms['stroke-opacity']) ?? 1);
  if (stroke !== undefined) {
    style.stroke = stroke;
    // **선이 있으면 두께를 반드시 적는다.** SVG 의 기본 두께는 1 **사용자 단위**이고
    // 렌더측 기본은 1 **캔버스 px** 이라, 비워 두면 축척만큼 어긋난다.
    const width = parseLength(atoms['stroke-width']) ?? 1;
    style.strokeWidth = Math.max(0, width * strokeScale);
    if (options.nonUniformStroke === true) {
      notes.push(note('approximated', 'nonUniformStrokeScale'));
    }
  }

  const own = parseOpacity(atoms['opacity']) ?? 1;
  const group = options.groupOpacity ?? 1;
  const opacity = own * group;
  if (opacity < 1) style.opacity = round3(opacity);
  if (group < 1) notes.push(note('approximated', 'groupOpacityPerChild'));

  const evenOdd = atoms['fill-rule']?.trim().toLowerCase() === 'evenodd';
  // **성공 여부와 무관하게 언제나 근사로 보고한다.** 스스로 교차하거나 서로 교차하는 부분
  // 경로에서는 두 규칙이 원리적으로 다르며, 파서가 "이번엔 정확했다" 를 판정하려 드는 것보다
  // 사용자가 미리보기에서 확인하게 두는 편이 싸고 정직하다(위험 R9).
  if (evenOdd) notes.push(note('approximated', 'evenOddWinding'));

  for (const [name, reason] of [
    ['stroke-dasharray', 'dashArrayDropped'],
    ['filter', 'filterDropped'],
    ['clip-path', 'clipPathDropped'],
    ['mask', 'maskDropped'],
  ] as const) {
    const value = atoms[name]?.trim().toLowerCase();
    if (value !== undefined && value !== '' && value !== 'none') {
      notes.push(note('dropped', reason));
    }
  }

  return { style, notes, hasOwnStyle, evenOdd };
}
