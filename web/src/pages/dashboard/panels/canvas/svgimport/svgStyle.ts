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
// **`light-dark()` 와 `var()` 는 풀어서 읽는다** — 그 둘을 모르면 손에 쥔 색을 버리게 된다.
// 지금의 draw.io 는 도형마다 표현 속성과 `style` 속성을 **함께** 내보내며(`fill="#f5f5f5"`
// 와 `style="fill: light-dark(rgb(245,245,245), rgb(26,26,26))"`), `style` 이 이기는 것은
// 옳지만 이긴 값을 읽지 못하면 모든 도형이 씨앗 색으로 나온다. 그래서 이 층은 (ㄱ) 두 함수
// 표기를 정적으로 풀고(`resolveCssWideValue`), (ㄴ) 그러고도 읽지 못한 칠 선언은 **같은
// 요소의 표현 속성으로 되돌린다**(`collectStyleAtoms`). 우선순위를 뒤집는 것이 아니라,
// 이긴 값이 쓸 수 없을 때 이미 가진 값을 쓰는 것이다.
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

/**
 * 글자에만 뜻이 있는 속성들 — **칠 축과 갈라 둔다**(결함 B 정정).
 *
 * `INHERITED_STYLE_PROPS` 에 얹지 않는 이유는 그 목록이 든 뜻에 있다: 그것은 **칠 속성의
 * 상속**이고 그 목록의 여섯은 도형이 읽는 것 전부다. 활자를 그 안에 섞으면 "opacity 는
 * 상속되지 않는다" 를 지키는 목록이 "글자 크기도 상속된다" 를 함께 지게 되어, 한 목록이 두
 * 축을 책임진다 — 이 파일이 `visibility` 를 그 목록에 넣지 않은 것과 **같은 판단**이다.
 */
export const TEXT_STYLE_PROPS = [
  'font-size',
  'font-weight',
  'font-family',
  'font-style',
  'text-anchor',
  'letter-spacing',
  'writing-mode',
  'text-decoration',
  'dominant-baseline',
  'alignment-baseline',
  'textlength',
] as const;

/**
 * 그 가운데 **사양이 상속으로 정한** 것들.
 *
 * `dominant-baseline` · `alignment-baseline` · `text-decoration` · `textLength` 는 상속되지
 * 않는다(SVG 1.1 각 속성의 "Inherited: no"). 넷을 함께 내려보내면 `<g dominant-baseline="hanging">`
 * 안의 모든 글자가 어긋남으로 보고되어, 사양이 걸지 않는 성질을 화면이 말한다.
 */
export const INHERITED_TEXT_PROPS = [
  'font-size',
  'font-weight',
  'font-family',
  'font-style',
  'text-anchor',
  'letter-spacing',
  'writing-mode',
] as const;

/** 한 요소가 스스로 말한 활자 — 표현 속성 + `style` 속성. **`style` 이 이긴다**(위 규율 그대로). */
export function collectTextAtoms(attrs: AttrBag): Record<string, string> {
  const out: Record<string, string> = {};
  for (const name of TEXT_STYLE_PROPS) {
    const value = attrs[name];
    if (value !== undefined && value.trim() !== '') out[name] = value.trim();
  }
  const inline = parseStyleAttribute(attrs['style']);
  for (const name of TEXT_STYLE_PROPS) {
    const value = inline[name];
    if (value !== undefined && value.trim() !== '') out[name] = value.trim();
  }
  return out;
}

/** 활자의 상속. 목록이 다를 뿐 `inheritStyleAtoms` 와 **같은 형상**이다. */
export function inheritTextAtoms(parent: StyleAtoms, own: StyleAtoms): Record<string, string> {
  const inherited: Record<string, string> = {};
  for (const name of INHERITED_TEXT_PROPS) {
    const value = parent[name];
    if (value !== undefined) inherited[name] = value;
  }
  return { ...inherited, ...own };
}

/**
 * 표현 속성으로 **되돌릴 수 있는** 축 — 칠 둘뿐이다.
 *
 * 되돌림의 판정이 `normalizePaint` 인 까닭에 이 목록도 그 함수가 읽는 축과 같아야 한다.
 * `stop-color` 는 여기 없다: 그 값은 `normalizePaint` 를 지나지 않고
 * `foldAlphaIntoColor` 로 바로 가므로 같은 판정을 쓸 수 없다.
 */
const PAINT_PROP_NAMES = ['fill', 'stroke'] as const;

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

// --- CSS 넓은 값 풀기 ----------------------------------------------------

const CSS_VALUE_CALL = /^([a-z-]+)\(([\s\S]*)\)$/i;

/** 되풀이 상한 — 서로를 가리키는 대체값에서 멈춘다. */
const CSS_VALUE_MAX_DEPTH = 4;

/**
 * **최상위 쉼표에서만** 자른 인자 목록. 괄호가 맞지 않으면 `undefined`.
 *
 * `light-dark(rgb(245, 245, 245), rgb(26, 26, 26))` 의 쉼표는 넷인데 인자는 둘이다 —
 * `split(',')` 은 `rgb(245` 같은 조각을 내고, 그 조각은 색으로도 아닌 것으로도 읽히지
 * 않는다. 괄호 깊이를 세는 것이 이 함수의 전부다.
 *
 * **따옴표는 세지 않는다.** 칠 값에 따옴표가 드는 표기는 SVG 에서 쓰이지 않으며, 세는
 * 순간 이 함수가 작은 CSS 토크나이저가 되어 이 파일이 지켜 온 경계(머리말 — 선택자
 * 엔진을 들이지 않는다)를 넘는다.
 */
function splitTopLevelArgs(body: string): string[] | undefined {
  const parts: string[] = [];
  let depth = 0;
  let start = 0;
  for (let i = 0; i < body.length; i += 1) {
    const ch = body[i];
    if (ch === '(') depth += 1;
    else if (ch === ')') {
      depth -= 1;
      // 여는 괄호보다 닫는 괄호가 먼저다 — 바깥 `(…)` 가 한 호출이 아니었다는 뜻이다.
      if (depth < 0) return undefined;
    } else if (ch === ',' && depth === 0) {
      parts.push(body.slice(start, i).trim());
      start = i + 1;
    }
  }
  if (depth !== 0) return undefined;
  parts.push(body.slice(start).trim());
  return parts;
}

function resolveCssWide(raw: string, depth: number): string {
  const value = raw.trim();
  if (depth >= CSS_VALUE_MAX_DEPTH) return value;
  const call = CSS_VALUE_CALL.exec(value);
  if (call === null) return value;
  const name = (call[1] ?? '').toLowerCase();
  if (name !== 'light-dark' && name !== 'var') return value;
  const args = splitTopLevelArgs(call[2] ?? '');
  if (args === undefined) return value;
  if (name === 'light-dark') {
    // 인자가 둘이 아니면 브라우저도 그 선언을 버린다 — 우리도 읽지 못한 것으로 둔다.
    if (args.length !== 2) return value;
    return resolveCssWide(args[0] ?? '', depth + 1);
  }
  // `var(--x, F)` — 첫 인자는 사용자 지정 속성이어야 하고, 대체값이 있어야 읽을 것이 있다.
  if (args.length < 2 || !(args[0] ?? '').startsWith('--')) return value;
  const fallback = args.slice(1).join(', ');
  if (fallback === '') return value;
  return resolveCssWide(fallback, depth + 1);
}

/**
 * `light-dark()` 와 `var()` 를 **정적으로** 푼다. 읽지 못하는 표기는 그대로 돌려준다.
 *
 * **고르는 쪽마다 근거가 다르다.**
 *   - `light-dark(A, B)` → `A`. 가져온 그림은 밝은 바탕에 대고 그려진 것이며(같은 파일의
 *     바탕 `<rect>` 가 `#ffffff` 를 든다), 이 층에는 읽을 수 있는 색 구성이 없다
 *     (`prefers-color-scheme` 은 살아 있는 문서의 것이고 불변식 K3 이 그것을 막는다).
 *     둘 중 하나를 골라야 한다면 **저자가 기본으로 본 쪽**이다.
 *   - `var(--x, F)` → `F`. 사용자 지정 속성의 실제 값은 살아 있는 캐스케이드에만 있고,
 *     선언된 대체값은 저자가 "이 변수가 없으면 이 색" 이라고 **적어 둔** 값이다.
 *   - `var(--x)` → 그대로. 대체값이 없으면 우리가 아는 것이 없으므로, 모르는 채로 두어
 *     `normalizePaint` 의 `unsupported` 와 그 보고(`paintUnresolved`)에 닿게 한다.
 *     모르는 것을 짐작해 색으로 통과시키는 것이 이 층에서 가장 나쁜 실패다(아래
 *     `normalizePaint` 머리말).
 */
export function resolveCssWideValue(raw: string): string {
  return resolveCssWide(raw, 0);
}

/**
 * `style="fill:red;stroke:none"` → 이름/값 표.
 *
 * 값 안의 콜론(`url(a:b)`)을 지키기 위해 **첫 콜론에서만** 자른다.
 *
 * 값은 `resolveCssWideValue` 를 지난다 — 칠뿐 아니라 활자 축도 같은 표기를 들 수 있고
 * (`font-size: var(--fs, 12px)`), 푸는 자리를 둘로 나누면 규칙이 두 벌이 된다.
 */
export function parseStyleAttribute(raw: string | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  if (raw === undefined) return out;
  for (const chunk of raw.split(';')) {
    const colon = chunk.indexOf(':');
    if (colon === -1) continue;
    const name = chunk.slice(0, colon).trim().toLowerCase();
    const value = resolveCssWideValue(chunk.slice(colon + 1));
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
  const presented: Record<string, string> = {};
  for (const name of STYLE_PROP_NAMES) {
    const value = attrs[name];
    if (value !== undefined && value.trim() !== '') presented[name] = value.trim();
  }
  const declared = parseStyleAttribute(attrs['style']);
  const merged: Record<string, string> = { ...presented, ...declared };

  // **읽지 못한 선언이 읽을 수 있는 속성을 가리지 않는다.**
  //
  // 우선순위는 그대로다 — `style` 이 이긴다. 되돌리는 것은 이긴 값이 **쓸 수 없을 때**
  // 뿐이고, 그때의 선택지는 둘이다: 씨앗 색으로 떨어지거나(`resolveStyle` 의
  // `unsupported` 가지), 같은 요소가 이미 든 색을 쓰거나. 앞을 고르면 `fill="#f5f5f5"`
  // 를 손에 쥐고도 도형을 파랗게 칠한다.
  //
  // **`none` 은 쓸 수 없는 값이 아니다.** `style="fill:none"` 은 `fill="red"` 를 이겨야
  // 하고 `url(#g)` 도 마찬가지다 — `normalizePaint` 가 그 둘을 따로 된 갈래로 읽으므로,
  // 되돌리는 판정을 `unsupported` 하나에 걸면 그 구별이 공짜로 따라온다.
  //
  // **표현 속성 쪽도 못 읽는 경우는 따로 가르지 않는다.** 되돌리든 말든 `unsupported` 는
  // 한 바구니라 `resolveStyle` 이 똑같이 씨앗 색과 `paintUnresolved` 를 낸다 — 그것을
  // 가르는 가지는 어떤 시험으로도 구별되지 않는(실측: 그 가지를 지워도 62 시험이 모두
  // 초록인) 죽은 분기다. 되돌림은 보고를 지우는 길이 아니라 **보고할 일이 없게** 만드는
  // 길이고, 읽을 값이 없으면 보고는 그대로 선다.
  for (const name of PAINT_PROP_NAMES) {
    const inline = declared[name];
    const attr = presented[name];
    if (inline === undefined || attr === undefined) continue;
    if (normalizePaint(inline).kind !== 'unsupported') continue;
    merged[name] = attr;
  }
  return merged;
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

/**
 * SVG `fill` 의 **초기값**(사양 §11.3). 우리가 고른 색이 아니라 사양이 정한 값이다 —
 * 그래서 씨앗 색(`SEED_COLOR`)과 섞이지 않게 이름을 따로 둔다. 둘을 한 상수로 합치면
 * "사양대로의 값" 과 "우리가 고른 값" 이 한 이름을 나눠 쓰게 되고, 그 순간 `pathSeedStyle`
 * 을 쓰는 자리와 이 자리를 구별할 근거가 사라진다.
 */
const SVG_INITIAL_FILL = '#000000';

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

  // **`fill` 을 말하지 않은 채 `stroke` 만 말한 도형은 검게 채워진다**(SVG 1.1 §11.3 —
  // `fill` 의 초기값이 `black` 이다). `<path d="…" stroke="red"/>` 는 브라우저에서 검은 속에
  // 빨간 테를 두른 도형으로 그려지며, 그것이 **사용자가 원본 도구에서 본 그림**이다.
  //
  // **기각 — 칠하지 않고 근사로 보고한다.** 그 안은 "우리는 이 칠을 하지 않았습니다" 를
  // 말할 뿐 그림을 고치지 않는다. 사양이 정한 값을 세 줄로 적을 수 있는데 보고로 대신하는
  // 것은 고칠 수 있는 어긋남을 **설명으로 바꾸는 일**이고, 게다가 사용자가 **적은 적 없는**
  // 칠에 대한 알림이라 잡음이 된다(위험 R7 — 읽히지 않는 보고는 침묵과 같다).
  //
  // **보고에 올리지 않는 이유**: 이것은 근사가 아니라 **사양대로의 값**이다. 옳게 그린 것을
  // 근사로 말하면 사용자가 미리보기의 정확함을 의심하게 된다.
  //
  // 그리고 이 규칙은 **아무 칠도 말하지 않은 도형에는 걸리지 않는다** — 그쪽은 008 의
  // `pathSeedStyle` 이 서며(REQ-06), 그 결정은 SPEC 이 사양과 **의도적으로** 갈라선 자리다
  // (가져온 도형이 카탈로그 도형과 같은 씨앗 색을 입어야 목록에서 구별되지 않는다).
  const fillSource = fillRaw ?? (strokeRaw !== undefined ? SVG_INITIAL_FILL : undefined);
  const fill = resolvePaint(fillSource, parseOpacity(atoms['fill-opacity']) ?? 1);
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

/**
 * 같은 갈래·같은 사유의 항목을 하나로 합친다. **개수는 더한다.**
 *
 * 순회 중에는 항목이 하나씩 발행되는 편이 싸다(합칠 자리를 찾느라 목록을 훑지 않는다).
 * 합치는 일은 화면에 닿기 직전 한 번이면 되고, 그 자리를 여기 두는 것은 **항목을 만드는
 * 층이 항목을 합치는 층과 같아야** 두 곳이 갈라지지 않기 때문이다.
 *
 * 발행 순서를 지킨다 — 같은 사유가 처음 나온 자리에 합계가 선다. 순서가 흔들리면 화면의
 * 보고 줄 차례가 파일마다 달라지고, 그것은 시험이 잡기 어려운 잡음이다.
 */
export function mergeNotes(notes: readonly ImportNote[]): ImportNote[] {
  const order: string[] = [];
  const byKey = new Map<string, { kind: ImportNote['kind']; reason: ImportNoteReason; count: number }>();
  for (const item of notes) {
    if (item.count <= 0) continue;
    const key = `${item.kind}:${item.reason}`;
    const found = byKey.get(key);
    if (found === undefined) {
      order.push(key);
      byKey.set(key, { kind: item.kind, reason: item.reason, count: item.count });
    } else {
      found.count += item.count;
    }
  }
  return order.flatMap((key) => {
    const found = byKey.get(key);
    return found === undefined ? [] : [{ kind: found.kind, reason: found.reason, count: found.count }];
  });
}
