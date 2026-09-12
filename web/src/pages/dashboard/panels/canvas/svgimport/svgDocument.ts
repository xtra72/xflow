// 문서 순회 — **007 에서 DOM 을 아는 유일한 모듈** (SPEC-CANVAS-007 M5).
//
// **이 파일이 하나인 것이 아키텍처 진술이다**(불변식 K7). 여기서 `DOMParser` 를 부르고
// 트리를 훑고 속성을 긁어 **평평한 `ImportedShape[]`** 를 낸다. 그 아래로는 문자열과
// 숫자뿐이다. 그래서 (a) 산술 결함이 DOM 거동으로 위장하지 못하고, (b) 훗날 다른 벡터
// 형식이 들어와도 축약 산술이 그대로 쓰이며, (c) jsdom 이 SVG 를 어떻게 다루든 시험의
// 대부분이 흔들리지 않는다.
//
// **파싱한 문서를 살아 있는 문서에 붙이지 않는다**(불변식 K3). `appendChild` ·
// `insertBefore` · `replaceWith` · `importNode` · `adoptNode` 가 이 파일에 **하나도 없다**.
// 그 형상이 값을 셋 준다: 스크립트 실행 가능성이 형상으로 닫히고, `getComputedStyle` 로
// 가는 길이 닫히며(그래서 `<style>` 을 **손으로 쓴 파서**로 읽는 것이 게으름이 아니라 이
// 경계의 귀결이다 — `svgCssRules.ts`), 붙지 않은 문서는 외부 자원을 가져오지 않는다.
//
// **SVG 기하 DOM API 를 하나도 쓰지 않는다**(불변식 K4). `getBBox` · `getTotalLength` ·
// `getPointAtLength` · `pathSegList` · `getCTM` 은 jsdom 에 **없고**(측정: `getBBox` 와
// `SVGPathElement` 둘 다 `undefined`), `getComputedStyle` 은 살아 있는 문서를 요구한다.
// 이것은 편의가 아니라 **가능성의 경계**다 — 속성 읽기와 트리 순회만 쓴다.
//
// **감춤은 부모에서 자식으로 흘러야 한다.** `<g visibility="hidden">` 안의 도형은 그려지지
// 않는데, 자기 속성만 보는 판정은 그 사실을 놓쳐 **감춘 그림을 통째로 가져온다.** 그래서
// 순회가 두 상태를 함께 나른다:
//   - `display:none` — **뒤집을 수 없다.** 그 아래 전부가 사라진다(CSS 에서 상속되지 않지만
//     하위 트리를 렌더 트리에서 제거하므로 결과가 상속처럼 보인다).
//   - `visibility` — **상속되고 자식이 뒤집을 수 있다**(`visibility="visible"`). 그래서
//     이 값은 물려받은 값과 자기 값 가운데 **자기 값이 이긴다**.
//   `visibility` 를 `INHERITED_STYLE_PROPS` 에 넣지 않는 이유가 여기 있다 — 그 목록은
//   **칠 속성**의 상속이고, 감춤은 순회의 가지치기다. 두 축을 한 목록에 섞으면 감춤이
//   스타일 병합의 부수 효과가 되어 뒤집기 규칙을 잃는다.
//
// **감춘 하위 트리에서는 개수만 센다.** 안쪽의 `<text>`·`<image>`·필터를 함께 보고하면
// 보고가 잡음이 되고(위험 R7), 그리지 않기로 한 그림의 미지원 목록은 사용자가 할 일이
// 없는 정보다.
//
// @spec SPEC-CANVAS-007 REQ-02 · REQ-04 · REQ-07 · AC-03 · AC-07 · AC-E9 · AC-E10

import { SEED_COLOR } from '../canvasElementFactory';
import type { PathCommand } from '../shapes/pathTypes';

import {
  MAX_USE_DEPTH,
  type ImportNote,
  type ImportNoteKind,
  type ImportNoteReason,
  type ImportRefusal,
  type ImportedNative,
  type ImportedShape,
  type ImportedText,
} from './svgImportTypes';
import { parseLength, parseNumberList, parseSvgPathData } from './svgPathData';
import {
  isShorthandShapeTag,
  shorthandNativeShape,
  shorthandShapeCommands,
  type AttrBag,
} from './svgShapes';
import {
  collectStyleAtoms,
  collectTextAtoms,
  TEXT_STYLE_PROPS,
  foldAlphaIntoColor,
  inheritStyleAtoms,
  inheritTextAtoms,
  isHidden,
  mergeNotes,
  parseOpacity,
  resolveStyle,
  type StyleAtoms,
} from './svgStyle';
import {
  hasTemplateToken,
  normalizeSvgText,
  preservesSpace,
  readAnchorCoord,
  resolveTextStyle,
  truncateImportText,
} from './svgText';
import {
  countUnsupportedCssRules,
  parseCSSRules,
  getCSSPropertiesForElement,
  type CSSRules,
} from './svgCssRules';
import {
  applyMatrix,
  determinant,
  IDENTITY_MATRIX,
  isSimilarity,
  multiplyMatrix,
  parseTransformList,
  preservesAxisAlignment,
  strokeScaleOf,
  transformCommands,
  type Matrix2x3,
} from './svgTransform';

const SVG_NAMESPACE = 'http://www.w3.org/2000/svg';

/**
 * 안으로 **내려가지 않는** 요소들.
 *
 * **버림이 아니다** — SVG 사양이 이들을 직접 그리지 않으므로 건너뛰는 것이 곧 옳은
 * 렌더다. `<use>` 가 가리키면 그때 내용이 산다. 그리지 않는 것을 버렸다고 말하면 보고가
 * 잡음이 되고, 잡음이 섞인 보고는 읽히지 않으며, 읽히지 않는 보고는 침묵과 같다.
 */
const NON_RENDERED_TAGS = new Set([
  'defs',
  'symbol',
  'clippath',
  'mask',
  'marker',
  'pattern',
  'metadata',
  'title',
  'desc',
  'script',
  'style',
  'lineargradient',
  'radialgradient',
  'filter',
]);

/**
 * 안으로 내려가는 그릇. `<a>` 는 SVG 사양의 컨테이너이며 도구가 실제로 도형을 감싼다.
 *
 * **`<switch>` 를 여기 넣지 않는다.** 그것은 그릇이되 **조건부 그릇**이다 — 브라우저는 직속
 * 자식 가운데 `requiredFeatures`·`requiredExtensions`·`systemLanguage` 가 모두 참인 **첫
 * 하나만** 그린다. 전부 내려가면 대안 N개를 겹쳐 그려 원본에 없는 그림이 되고, "첫 하나"
 * 를 고르려면 조건부 처리 속성 평가가 필요한데 그것은 구멍 메우기가 아니라 기능이다.
 * 그래서 `<switch>` 는 들어가지 않고 **보고한다**(아래 `hasDrawableDescendant`).
 */
const CONTAINER_TAGS = new Set(['g', 'a']);

/** `viewBox` 또는 그 폴백이 정한 사용자 좌표 상자. */
export interface ViewBox {
  readonly minX: number;
  readonly minY: number;
  readonly width: number;
  readonly height: number;
}

/** 문서 층이 낸 것 전부. 좌표는 **사용자 단위**이며 변환은 이미 녹아 있다. */
export interface SvgDocumentRead {
  readonly shapes: readonly ImportedShape[];
  /**
   * 문구들. **도형과 갈라 둔다** — 둘이 나르는 것이 다르고(명령 대 글자), 무엇보다 `shapes`
   * 를 합집합으로 넓히면 이 층을 이미 쓰고 있는 자리들이 `commands` 를 읽지 못한다.
   *
   * 배열 순서는 문서 순서다. 다만 요소가 될 때는 **도형 전부의 뒤**에 붙으므로, 문서에서
   * 문구보다 나중에 그려지던 도형이 있었으면 z-order 가 바뀐다(그 사실이 `textOrderChanged`
   * 로 보고에 오른다).
   */
  readonly texts: readonly ImportedText[];
  readonly notes: readonly ImportNote[];
  /** 문서가 스스로 말한 `viewBox`. 없으면 계획 층이 폴백을 탄다. */
  readonly viewBox: ViewBox | undefined;
  /** `width`/`height` 속성 — **단위 없는 수 또는 `px` 만** 크기로 읽는다. */
  readonly size: { readonly width: number; readonly height: number } | undefined;
}

/** 문서를 읽은 결과. **예외가 아니라 값으로 실패가 돌아온다**(REQ-07). */
export type SvgDocumentOutcome =
  | { readonly ok: true; readonly document: SvgDocumentRead }
  | { readonly ok: false; readonly refusal: ImportRefusal };

/** 문자열 → 문서를 읽는 함수의 형상. 계획 층이 이것을 **주입 가능하게** 받는다. */
export type SvgDocumentReader = (text: string) => SvgDocumentOutcome;

// --- 속성 읽기 ----------------------------------------------------------

/** 요소 하나의 속성 자루. 이름은 **소문자로 눕힌다**(`viewBox` 는 예외 없이 그대로 쓴다). */
function attrBag(el: Element): Record<string, string | undefined> {
  const out: Record<string, string> = {};
  const list = el.attributes;
  for (let i = 0; i < list.length; i += 1) {
    const attr = list.item(i);
    if (attr === null) continue;
    out[attr.name.toLowerCase()] = attr.value;
  }
  return out;
}

/** SVG 네임스페이스의 요소인가 — 아니면 미지 네임스페이스(잉크스케이프의 `sodipodi:*` 등)다. */
function isSvgElement(el: Element): boolean {
  return el.namespaceURI === SVG_NAMESPACE || el.namespaceURI === null;
}

function tagOf(el: Element): string {
  return (el.localName !== '' ? el.localName : el.nodeName).toLowerCase();
}

/** `walk` 가 닿았더라면 **그리거나 보고했을** 태그. 그릇(`g`·`a`)은 스스로 그리지 않는다. */
function isDrawableTag(tag: string): boolean {
  if (isShorthandShapeTag(tag)) return true;
  return (
    tag === 'path' ||
    tag === 'text' ||
    tag === 'use' ||
    tag === 'image' ||
    tag === 'foreignobject' ||
    tag === 'svg'
  );
}

/**
 * 이 요소 **안에** 그렸을 것이 하나라도 있는가 — 들어가지 않은 그릇을 보고할지 가르는 유일한
 * 조건이다.
 *
 * **"요소 자식이 있는가" 를 따로 묻지 않는다.** 그릴 것이 있으면 요소 자식은 반드시 있으므로
 * 두 조건을 함께 두면 뒤의 것이 앞의 것을 삼켜 **물지 않는 가드**가 된다. 글자 노드만 든
 * `<madeUpTag>글자</madeUpTag>` 와 주석만 든 요소는 요소 자식이 **아예 없으므로** 여기서
 * 자연히 거짓이 된다 — 진짜 잎이고, 잃은 것이 없다.
 *
 * **`NON_RENDERED_TAGS` 아래는 세지 않는다.** `<defs>` 안의 `<rect>` 는 들어갔더라도 그리지
 * 않았을 것이므로, 그것을 근거로 "버렸다" 고 말하면 보고가 거짓이 된다.
 *
 * **미지 네임스페이스 자식도 지난다** — 그릇이 겹쳐 있을 수 있고(`<foo:a><bar:b><text/>`),
 * 안쪽의 `<text>` 는 기본 네임스페이스를 물려받아 여전히 SVG 의 글자다.
 */
function hasDrawableDescendant(el: Element): boolean {
  for (const child of Array.from(el.children)) {
    if (isSvgElement(child)) {
      const tag = tagOf(child);
      if (NON_RENDERED_TAGS.has(tag)) continue;
      if (isDrawableTag(tag)) return true;
    }
    if (hasDrawableDescendant(child)) return true;
  }
  return false;
}

// --- 파싱 실패 판정 ------------------------------------------------------

/**
 * 파싱이 실패했는가 — **둘 다** 본다.
 *
 * jsdom 에서 깨진 문서는 루트가 `parsererror` 였다(측정). **브라우저에서는 문서 안에
 * `<parsererror>` 가 끼는 형태가 알려져 있으나 본 SPEC 은 그것을 실측하지 않았다**
 * (가정 A9 — 신뢰도 낮음, 미검증). 하나만 보는 코드는 실측하지 않은 쪽에서 조용히
 * 통과하고, 그때 산출은 "루트가 svg 가 아닌 문서에서 도형 0개" 가 아니라 **깨진 문서에서
 * 지어낸 그림**이 된다. 그래서 둘 다 본다.
 */
function parseFailure(doc: Document): boolean {
  if (doc.getElementsByTagName('parsererror').length > 0) return true;
  const root = doc.documentElement as Element | null;
  if (root === null) return true;
  return tagOf(root) !== 'svg';
}

// --- 순회 --------------------------------------------------------------

/**
 * 이 문맥이 감춰져 있는가 — `display:none` 하위 트리이거나 유효 `visibility` 가
 * `hidden` 이다. **부모의 감춤을 자식이 되살릴 수 있으므로 끈끈한 불린을 따로 들지
 * 않는다** — 그 불린을 들면 `visibility="visible"` 이 영영 통하지 않는다.
 */
function isHiddenContext(ctx: WalkContext): boolean {
  return ctx.displayNone || ctx.visibility === 'hidden';
}

/**
 * 원시 도형의 좌표에 행렬을 녹인다. **못 녹이면 `undefined`** — 그 도형은 경로로 남는다.
 *
 * 두 갈래가 서로 다른 조건을 지나는 것이 이 함수의 요점이다.
 *
 * **선에는 조건이 없다.** `LineGeometry` 는 두 끝점을 그대로 나르고 아핀은 선분을 언제나
 * 선분으로 옮기므로, 돌아간 `<line>` 도 · 기울어진 `<line>` 도 캔버스 선으로 **정확히**
 * 선다. 상자에 물리는 축 정렬 조건을 선에까지 걸면, 걸 이유가 없는 곳에 걸어 **돌아간
 * 선을 전부 경로로** 떨어뜨리게 된다.
 *
 * **상자에는 조건이 있다** — `preservesAxisAlignment`(그 주석이 조건과 근거를 적는다).
 * 통과했으면 마주 보는 두 꼭짓점의 상(像)이 곧 결과 상자다: 두 변의 상이 각각 축에
 * 나란하므로 옮겨진 도형은 그 두 점이 마주 보는 축 정렬 상자와 **정확히 같다**. 반사도
 * 축 맞바꿈도 `min`/`max` 가 함께 흡수하므로 갈래를 더 가르지 않는다.
 *
 * **타원의 상자도 같은 두 점으로 난다.** 타원의 극점은 두 반축 끝이고 그 넷의 상은
 * 옮겨진 상자의 네 변에 닿는다 — 조건을 통과한 행렬에서만 참인 성질이며, 그래서 이
 * 산술이 게이트 뒤에 있다.
 */
function transformNative(native: ImportedNative, m: Matrix2x3): ImportedNative | undefined {
  if (native.kind === 'line') {
    const from = applyMatrix(m, native.x1, native.y1);
    const to = applyMatrix(m, native.x2, native.y2);
    return finiteNative({ kind: 'line', x1: from.x, y1: from.y, x2: to.x, y2: to.y });
  }
  if (!preservesAxisAlignment(m)) return undefined;
  const a = applyMatrix(m, native.minX, native.minY);
  const b = applyMatrix(m, native.maxX, native.maxY);
  return finiteNative({
    kind: native.kind,
    minX: Math.min(a.x, b.x),
    minY: Math.min(a.y, b.y),
    maxX: Math.max(a.x, b.x),
    maxY: Math.max(a.y, b.y),
  });
}

/**
 * 좌표 넷이 모두 유한한 원시형만 통과시킨다. **아니면 그 도형은 경로로 남는다.**
 *
 * `transform="scale(1e200)"` 처럼 좌표를 `±∞` 로 미는 문서가 실재한다(`|det|` 이 0 이
 * 아니므로 퇴화 검사를 통과한다). 그 좌표로 원시형을 세우면 상자를 정하는 층이 유한 폴백
 * 으로 내려앉아 **캔버스 구석의 1×1 부스러기**가 나온다.
 *
 * 경로에는 이 경우에 대한 답이 007 에 이미 있다 — 잴 바운딩 박스가 없으면 **문서 틀을
 * 그대로** 쓴다(`viewBoxBounds` 폴백). 요소를 잃지도, 구석으로 찌부러뜨리지도 않는 그 답을
 * 원시형 쪽에 다시 적는 대신 **경로로 돌려보낸다** — 그러면 답은 계속 한 곳에 있다.
 *
 * 경로의 부분 폴백(좌표 하나가 망가져도 나머지로 상자를 잰다)과 달리 여기는 **전부 아니면
 * 전무**다. 원시형의 네 수는 도형 하나를 통째로 정하므로 하나가 망가지면 나머지 셋으로
 * 세울 수 있는 도형이 없다.
 */
function finiteNative(native: ImportedNative): ImportedNative | undefined {
  const numbers =
    native.kind === 'line'
      ? [native.x1, native.y1, native.x2, native.y2]
      : [native.minX, native.minY, native.maxX, native.maxY];
  return numbers.every((n) => Number.isFinite(n)) ? native : undefined;
}

interface WalkContext {
  /** 조상까지 누적한 변환. **조상이 바깥이다.** */
  readonly matrix: Matrix2x3;
  /** 조상에서 물려받은 **칠** 속성들(감춤은 여기 없다 — 위 머리말). */
  readonly atoms: StyleAtoms;
  /**
   * 조상에서 물려받은 **활자** 속성들. 칠과 **다른 자루**인 것이 뜻이다 — 상속 목록이
   * 서로 다르고(`INHERITED_TEXT_PROPS`), 한 자루에 섞으면 "opacity 는 상속되지 않는다" 를
   * 지키는 목록이 활자까지 책임지게 된다(`svgStyle` §TEXT_STYLE_PROPS).
   */
  readonly textAtoms: StyleAtoms;
  /** 물려받은 `xml:space`. XML 에서 상속되므로 루트의 `preserve` 가 아래로 흐른다. */
  readonly preserveSpace: boolean;
  /** 물려받은 `visibility`. 자식이 `visible` 로 **뒤집을 수 있다**. */
  readonly visibility: string | undefined;
  /** `display:none` 하위 트리인가 — **뒤집을 수 없다**. */
  readonly displayNone: boolean;
  /** 조상 `<g opacity>` 의 누적 곱. 상속이 아니라 **자식별 곱**이다(근사). */
  readonly opacity: number;
  /** `<use>` 전개 깊이. */
  readonly depth: number;
  /** 지금 전개 중인 `<use>` 대상 id 집합 — **순환 검출**. */
  readonly expanding: ReadonlySet<string>;
}

class DocumentWalker {
  private readonly shapes: ImportedShape[] = [];
  private readonly texts: ImportedText[] = [];
  private readonly notes: ImportNote[] = [];
  private readonly byId = new Map<string, Element>();
  private hiddenShapes = 0;
  private cssRules: CSSRules = {};
  /**
   * 문구를 하나라도 세운 뒤에 도형이 또 섰는가 — 그 순간 z-order 가 바뀐다.
   *
   * 문구 **하나하나**가 추월당했는지를 따로 세지 않는 것에 뜻이 있다: 요소가 될 때 문구는
   * 전부 뒤로 가므로, 마지막 문구보다 나중에 선 도형이 하나라도 있으면 **그 시점까지의 문구
   * 전부**가 그 도형 위로 올라선다. 그래서 세어야 하는 것은 "추월당한 문구 수" 이고,
   * 그 수는 도형이 설 때의 문구 개수다.
   */
  private overtakenTexts = 0;

  constructor(private readonly root: Element) {
    this.indexIds(root);
    this.collectStyleTags(root);
  }

  /**
   * id 표를 **`<defs>` 안까지 포함해** 먼저 만든다.
   *
   * `document.getElementById` 를 쓰지 않는 것은 XML 문서에서 그 함수가 DTD 없이는 id 를
   * 알지 못하기 때문이다 — jsdom 에서도 브라우저에서도 `null` 이 돌아온다. 속성을 직접
   * 읽는 것이 두 환경에서 같은 답을 내는 유일한 길이다.
   *
   * **처음 만난 것이 이긴다** — SVG 사양이 중복 id 를 오류로 두고 브라우저는 문서 순서상
   * 앞의 것을 쓴다.
   */
  private indexIds(el: Element): void {
    const id = el.getAttribute('id');
    if (id !== null && id !== '' && !this.byId.has(id)) this.byId.set(id, el);
    for (const child of Array.from(el.children)) this.indexIds(child);
  }

  /**
   * SVG 문서의 모든 `<style>` 태그에서 CSS 규칙을 **한 표로** 모은다.
   *
   * **지원**: element 선택자 · `.class` · `#id`, 그리고 그것들을 **이어 붙인** 복합
   * 선택자(`rect.cls-2` · `.a.b` · `rect#top`) — 모두 `descend` 가 실제로 읽는다
   * (`getCSSPropertiesForElement`). 표에만 들어가고 아무도 읽지 않는 선택자를 두지
   * 않는 것이 이 짝의 규율이다.
   * **미지원**: cascade · specificity · 결합자(`g > rect` · `g .a`) · 의사 클래스 ·
   * 속성 선택자 · 쉼표 목록 · `@media` — 그 개수는 `walk` 가 버림 보고에 올린다.
   *
   * 문서 순서대로 덮어쓴다(`Object.assign`) — 뒤에 선 `<style>` 이 앞을 이긴다.
   * **키의 차례는 처음 본 자리에 남는다**(`CSSRules` 머리말): 같은 칸 안의 승부를
   * `getCSSPropertiesForElement` 이 그 차례로 가르므로 이 `Object.assign` 은 값만 바꾸고
   * 자리는 옮기지 않는다.
   */
  private collectStyleTags(el: Element): void {
    if (isSvgElement(el) && tagOf(el) === 'style') {
      const cssText = el.textContent ?? '';
      const parsed = parseCSSRules(cssText);
      Object.assign(this.cssRules, parsed);
    }
    for (const child of Array.from(el.children)) this.collectStyleTags(child);
  }

  private note(kind: ImportNoteKind, reason: ImportNoteReason, count = 1): void {
    if (count > 0) this.notes.push({ kind, reason, count });
  }

  /**
   * `url(#g)` → 참조된 그라디언트의 **첫 `<stop stop-color>`**.
   *
   * 그라디언트를 단색으로 떨어뜨리는 것은 근사이며 보고에 오른다(그 보고는 `resolveStyle`
   * 이 발행한다). 첫 stop 을 고르는 것은 "가장 덜 틀린 하나" 이지 평균이 아니다 — 평균색은
   * 원본 어디에도 없는 색이라, 사용자가 미리보기에서 그것을 원본과 견줄 수 없다.
   */
  private readonly resolvePaintRef = (id: string): string | undefined => {
    const target = this.byId.get(id);
    if (target === undefined) return undefined;
    const stop = this.firstStop(target);
    if (stop === undefined) return undefined;
    const atoms = collectStyleAtoms(attrBag(stop));
    const color = atoms['stop-color'];
    if (color === undefined || color.trim() === '') return undefined;
    const alpha = parseOpacity(atoms['stop-opacity']) ?? 1;
    return foldAlphaIntoColor(color.trim(), alpha).value;
  };

  private firstStop(el: Element): Element | undefined {
    for (const child of Array.from(el.children)) {
      if (tagOf(child) === 'stop') return child;
      const nested = this.firstStop(child);
      if (nested !== undefined) return nested;
    }
    return undefined;
  }

  /**
   * 명령 목록 하나를 요소 후보로 세운다. 감춘 하위 트리에서는 **개수만** 는다.
   *
   * `native` 가 있으면 그 도형은 경로가 아니라 캔버스 원시형이 될 수 있다. 그 판정을
   * 여기서 하는 이유는 **행렬을 아는 층이 여기뿐**이기 때문이다 — 축약기는 행렬을 모르고
   * (모듈 머리말 K7), 계획 층은 행렬이 이미 좌표에 녹은 뒤를 받는다.
   */
  private emit(
    commands: readonly PathCommand[],
    atoms: StyleAtoms,
    ctx: WalkContext,
    hidden: boolean,
    native?: ImportedNative,
  ): void {
    if (commands.length === 0) return;
    if (hidden) {
      this.hiddenShapes += 1;
      return;
    }
    // `|det| = 0` 이면 그 도형은 한 점 또는 한 선으로 찌부러져 그려질 것이 없다.
    if (determinant(ctx.matrix) === 0) {
      this.note('dropped', 'degenerateTransformDropped');
      return;
    }
    const resolved = resolveStyle(atoms, {
      resolvePaintRef: this.resolvePaintRef,
      strokeScale: strokeScaleOf(ctx.matrix),
      groupOpacity: ctx.opacity,
      nonUniformStroke: !isSimilarity(ctx.matrix),
      fallbackColor: SEED_COLOR,
    });
    this.notes.push(...resolved.notes);
    // **변환은 여기서 좌표에 녹는다 — 호는 이미 3차가 되어 있다**(불변식 K2). 호 변수를
    // 든 자료 구조가 이 층에 도달할 수 없는 것이 그 불변식의 형상 판정이다.
    const transformed = transformCommands(commands, ctx.matrix);
    // **원시형도 같은 행렬을 같은 자리에서 지난다.** 둘을 다른 자리에서 옮기면 한쪽만
    // 고쳐질 수 있고, 그때 "경로로 보면 여기 있는데 사각형으로 보면 저기 있다" 가 된다.
    const placed = native === undefined ? undefined : transformNative(native, ctx.matrix);
    this.shapes.push({
      commands: transformed,
      style: resolved.style,
      closed: transformed.some((cmd) => cmd.c === 'Z'),
      hasOwnStyle: resolved.hasOwnStyle,
      evenOdd: resolved.evenOdd,
      ...(placed === undefined ? {} : { native: placed }),
    });
    // 지금까지 선 문구들은 **이 도형 아래에서** 그려지던 것이다. 요소가 될 때 문구는 전부
    // 도형 뒤(=위)로 가므로, 그 수가 곧 z-order 가 뒤집힌 문구의 수다.
    this.overtakenTexts = this.texts.length;
  }

  /**
   * `<text>` 하나를 문구 후보로 세운다.
   *
   * **`emit` 과 갈라 둔 것에 뜻이 있다.** 도형은 명령 목록을 나르고 문구는 한 점과 글자를
   * 나르며, 둘이 거치는 판정이 다르다 — 문구에는 감김도 `fill-rule` 도 없고, 대신 공백
   * 규칙 · 글자 수 상한 · 템플릿 토큰이 있다. 한 함수에 담으면 두 갈래가 `if` 로 갈라지고
   * 그 `if` 가 곧 두 번째 규칙이 된다.
   *
   * **그리지 않는 것은 세지 않는다.** 빈 `<text>` 는 SVG 에서도 아무것도 그리지 않으므로
   * 보고에 오르지 않는다(`<defs>` 를 버림으로 세지 않는 것과 같은 규율).
   */
  private emitText(el: Element, attrs: AttrBag, ctx: WalkContext, own: StyleAtoms, hidden: boolean): void {
    if (hasTextPathChild(el)) {
      // 길 위의 글자 — 곧은 한 줄로 펴면 문서가 말한 적 없는 배치가 된다. 옮기지 못한다.
      if (!hidden) this.note('dropped', 'textDropped');
      return;
    }
    const text = normalizeSvgText(el.textContent ?? '', ctx.preserveSpace);
    if (text === '') return;
    if (hidden) {
      this.hiddenShapes += 1;
      return;
    }
    if (determinant(ctx.matrix) === 0) {
      this.note('dropped', 'degenerateTransformDropped');
      return;
    }
    const resolved = resolveTextStyle(own, ctx.textAtoms, {
      resolvePaintRef: this.resolvePaintRef,
      groupOpacity: ctx.opacity,
      fallbackColor: SEED_COLOR,
      fontScale: strokeScaleOf(ctx.matrix),
      nonUniform: !isSimilarity(ctx.matrix),
    });
    if (resolved.style.textColor === undefined && resolved.hasOwnStyle) {
      // `fill="none"` — 이 패널은 글자에 테를 두르지 않으므로 어느 쪽도 요소가 서지 않지만,
      // **테가 있었으면 보이던 글자를 못 옮긴 것**이고 없었으면 문서에서도 감춰져 있었다.
      if (resolved.strokePainted) this.note('dropped', 'textDropped');
      else this.hiddenShapes += 1;
      return;
    }
    this.notes.push(...resolved.notes);
    // 글자마다 자리를 준 문서(`x="10 20 30"`)는 첫 수에 한 줄로 선다 — 그 배치도 활자다.
    const ax = readAnchorCoord(attrs['x']);
    const ay = readAnchorCoord(attrs['y']);
    if (resolved.typographyIgnored || ax.perGlyph || ay.perGlyph) {
      this.note('approximated', 'textFontIgnored');
    }
    this.note('dropped', 'tspanDropped', countOwnTspans(el));
    const cut = truncateImportText(text);
    if (cut.truncated) this.note('dropped', 'textTruncated');
    if (hasTemplateToken(cut.text)) this.note('approximated', 'textTemplateToken');
    const anchor = applyMatrix(ctx.matrix, ax.value, ay.value);
    this.texts.push({
      x: anchor.x,
      y: anchor.y,
      text: cut.text,
      style: resolved.style,
      fontSizeUserUnits: resolved.fontSizeUserUnits,
      hasOwnStyle: resolved.hasOwnStyle,
    });
  }

  /** 한 요소의 순회 문맥 — 변환 누적 · 칠 상속 · 감춤 전파 · 그룹 불투명도. */
  private descend(attrs: AttrBag, parent: WalkContext, tagName?: string): { ctx: WalkContext; own: StyleAtoms; hidden: boolean } {
    const ownAtoms = collectStyleAtoms(attrs);
    // `<style>` 규칙을 먼저 깔고 **인라인 표현 속성이 그 위를 덮는다.** 규칙끼리의 순서는
    // 칸(element → .class → #id) 이 먼저고 같은 칸 안에서는 원문 순서이며, 맞는 규칙을
    // 고르고 그 순서를 가르는 일은 모두 `svgCssRules` 의 몫이다.
    const cssProps = getCSSPropertiesForElement(
      tagName ?? '',
      attrs['class'],
      attrs['id'],
      this.cssRules,
    );
    const ownAtomsWithCSS = { ...cssProps, ...ownAtoms };
    const own = inheritStyleAtoms(parent.atoms, ownAtomsWithCSS);
    const ownVisibility = ownAtoms['visibility'];
    // **자기 값이 물려받은 값을 이긴다** — `visibility="visible"` 로 뒤집을 수 있다.
    const visibility = ownVisibility ?? parent.visibility;
    const displayNone = parent.displayNone || own['display'] === 'none';
    // `isHidden` 을 그대로 쓰되 **유효 `visibility`** 를 물려 넣는다. 두 판정이 갈라지면
    // 감춤 규칙이 두 벌이 된다.
    const hidden = displayNone || isHidden({ ...own, visibility: visibility ?? '' });
    const opacityOwn = parseOpacity(own['opacity']) ?? 1;
    // 활자도 CSS 규칙을 받는다 — 인라인 속성이 규칙을 덮는 순서는 칠과 **같다**.
    const ownText = collectTextAtoms(attrs);
    const space = attrs['xml:space'];
    return {
      ctx: {
        matrix: multiplyMatrix(parent.matrix, parseTransformList(attrs['transform'])),
        atoms: own,
        textAtoms: inheritTextAtoms(parent.textAtoms, { ...pickText(cssProps), ...ownText }),
        preserveSpace: space === undefined ? parent.preserveSpace : preservesSpace(space),
        visibility,
        displayNone,
        opacity: parent.opacity * opacityOwn,
        depth: parent.depth,
        expanding: parent.expanding,
      },
      own,
      hidden,
    };
  }

  private walkChildren(el: Element, ctx: WalkContext): void {
    for (const child of Array.from(el.children)) this.walk(child, ctx);
  }

  private walk(el: Element, parent: WalkContext): void {
    if (!isSvgElement(el)) {
      // 미지 네임스페이스 — 그 자신은 버림이 아니다(그려지지 않는다). 다만 **안에 그릴 것을
      // 품고 있었다면** 그 하위 트리째 사라진 것이고, 그 사실은 말해야 한다(아래 `default:`
      // 와 같은 판정). 잉크스케이프가 거의 모든 파일에 내보내는 `<sodipodi:namedview>` 는
      // `<inkscape:grid>` 만 품으므로 여기서 조용하다 — 그 침묵이 위험 R7 의 값이다.
      if (!isHiddenContext(parent) && hasDrawableDescendant(el)) {
        this.note('dropped', 'unenteredContainerDropped');
      }
      return;
    }
    const tag = tagOf(el);
    if (NON_RENDERED_TAGS.has(tag)) {
      // `<style>` 만 예외로 보고에 오른다. **오르는 것은 적용하지 못한 규칙뿐이다** —
      // 규칙들은 `collectStyleTags` 가 이미 읽어 그림에 닿았고, 쓴 것을 "버렸다" 고 말하는
      // 보고는 침묵보다 나쁘다. 적용된 것까지 세면 `<style>` 이 든 거의 모든 파일에서 한 줄이
      // 울려 보고가 아무것도 말하지 않게 된다(위험 R7).
      if (tag === 'style' && !isHiddenContext(parent)) {
        this.note('dropped', 'styleRuleDropped', countUnsupportedCssRules(el.textContent ?? ''));
      }
      return;
    }

    const attrs = attrBag(el);
    const { ctx, own, hidden } = this.descend(attrs, parent, tag);

    if (CONTAINER_TAGS.has(tag)) {
      this.walkChildren(el, ctx);
      return;
    }

    if (tag === 'use') {
      this.expandUse(attrs, ctx, hidden);
      return;
    }

    if (tag === 'path') {
      this.emit(parseSvgPathData(attrs['d'] ?? ''), own, ctx, hidden);
      return;
    }

    if (isShorthandShapeTag(tag)) {
      // 명령과 원시형을 **함께** 낸다. 어느 쪽이 요소가 되는지는 `emit` 이 행렬을 보고
      // 정하고, 원시형이 서지 못하면 명령이 그대로 남는다.
      this.emit(
        shorthandShapeCommands(tag, attrs) ?? [],
        own,
        ctx,
        hidden,
        shorthandNativeShape(tag, attrs),
      );
      return;
    }

    if (tag === 'text') {
      // **감춤 판정보다 앞이다.** 글자도 그려지는 것이므로 감췄으면 도형과 **같이** 개수로
      // 센다 — 아래 가지의 "감춘 하위 트리의 미지원 내용은 보고하지 않는다" 는 그리지
      // 못하는 것들의 규칙이고, 글자는 이제 그 무리가 아니다.
      this.emitText(el, attrs, ctx, own, hidden);
      return;
    }

    if (hidden) return; // 감춘 하위 트리의 미지원 내용은 보고하지 않는다(위 머리말).

    switch (tag) {
      case 'image':
        this.note('dropped', 'imageDropped');
        return;
      case 'foreignobject':
        this.note('dropped', 'foreignObjectDropped');
        return;
      case 'svg':
        this.note('dropped', 'nestedSvgDropped');
        return;
      default:
        // 모르는 SVG 요소 — **잎이면** 그려지지 않으므로 보고하지 않는다(위험 R7). 그 전제가
        // 참인 것은 잎일 때뿐이다: 안에 도형이나 글자를 품고 있었다면 하위 트리 전부가
        // 그림에서도 보고에서도 사라지고, 그 조합(도형은 들어왔고 · 글자는 없고 · 버림 칸은
        // 비었고)이 사용자가 실제로 본 화면이다.
        //
        // **들어가지 않고 말만 한다.** SVG 1.1 의 렌더 모델은 아는 그릇과 아는 도형만 그리므로
        // 모르는 요소의 하위 트리는 브라우저에서도 그려지지 않는다 — 들어가면 브라우저가
        // 그리지 않는 것을 우리가 그리게 되어, 이 층이 지켜 온 "브라우저가 그리는 것을
        // 그린다" 가 깨진다. `<switch>` 도 같은 문으로 온다(위 `CONTAINER_TAGS` 머리말).
        if (hasDrawableDescendant(el)) this.note('dropped', 'unenteredContainerDropped');
        return;
    }
  }

  /**
   * `<use href="#id">` 를 같은 문서 안에서 전개한다.
   *
   * **깊이 상한과 순환 검출을 둘 다 둔다.** 순환이 아닌 깊은 사슬(도구가 낸 중첩 심볼)도
   * 멈춰야 하고, 깊이 상한만으로는 상한 안에서 도는 순환의 비용이 지수로 커진다.
   *
   * 외부 문서 참조(`href="other.svg#id"`)는 **버림**이다 — 가져오면 "파일이 브라우저를
   * 떠나지 않는다" 가 깨지고, 네트워크 요청이 일어난다.
   */
  private expandUse(attrs: AttrBag, ctx: WalkContext, hidden: boolean): void {
    const href = attrs['href'] ?? attrs['xlink:href'];
    if (href === undefined || href.trim() === '') return;
    const target = href.trim();
    if (!target.startsWith('#')) {
      if (!hidden) this.note('dropped', 'externalRefDropped');
      return;
    }
    const id = target.slice(1);
    if (ctx.depth >= MAX_USE_DEPTH || ctx.expanding.has(id)) {
      if (!hidden) this.note('dropped', 'useDepthDropped');
      return;
    }
    const node = this.byId.get(id);
    if (node === undefined) return; // 가리키는 것이 없다 — 지어낼 그림이 없다.

    if (!hidden && (attrs['width'] !== undefined || attrs['height'] !== undefined)) {
      // `width`/`height` 는 `<symbol>`/중첩 `<svg>` 에만 뜻이 있다. 무시하고 보고한다.
      this.note('approximated', 'useSizeIgnored');
    }

    const x = parseLength(attrs['x']) ?? 0;
    const y = parseLength(attrs['y']) ?? 0;
    const nested: WalkContext = {
      ...ctx,
      // `x`/`y` 는 `translate(x, y)` 로 접어 넣는다 — 사양이 그렇게 정의한다.
      matrix: multiplyMatrix(ctx.matrix, { a: 1, b: 0, c: 0, d: 1, e: x, f: y }),
      depth: ctx.depth + 1,
      expanding: new Set([...ctx.expanding, id]),
    };
    const targetTag = tagOf(node);
    // `<symbol>`/중첩 `<svg>` 자체는 그려지지 않는다 — 그 **내용**이 산다.
    if (targetTag === 'symbol' || targetTag === 'svg') {
      this.walkChildren(node, nested);
      return;
    }
    this.walk(node, nested);
  }

  /** 루트에서 시작해 전부 훑는다. 루트의 `transform` 은 **적용하지 않는다**. */
  run(): {
    shapes: readonly ImportedShape[];
    texts: readonly ImportedText[];
    notes: readonly ImportNote[];
  } {
    const attrs = attrBag(this.root);
    if (attrs['transform'] !== undefined && attrs['transform'].trim() !== '') {
      // SVG 1.1 은 최외곽 `<svg>` 의 `transform` 을 정의하지 않는다. 적용하지 않고 보고한다.
      this.note('approximated', 'rootTransformIgnored');
    }
    const preserve = attrs['preserveaspectratio']?.trim();
    if (preserve !== undefined && preserve !== '' && preserve !== 'xMidYMid meet') {
      // 007 에는 뷰포트가 없어 그 속성이 할 일이 없다 — 상자의 종횡비를 문서에 맞추는 것이
      // 이미 그 속성이 하려던 일이다. 읽지 않고 보고한다.
      this.note('approximated', 'preserveAspectRatioIgnored');
    }
    const rootAtoms = collectStyleAtoms(attrs);
    const rootCtx: WalkContext = {
      matrix: IDENTITY_MATRIX,
      atoms: rootAtoms,
      textAtoms: collectTextAtoms(attrs),
      preserveSpace: preservesSpace(attrs['xml:space']),
      visibility: rootAtoms['visibility'],
      displayNone: rootAtoms['display'] === 'none',
      opacity: parseOpacity(rootAtoms['opacity']) ?? 1,
      depth: 0,
      expanding: new Set<string>(),
    };
    this.walkChildren(this.root, rootCtx);
    if (this.hiddenShapes > 0) this.note('dropped', 'hiddenDropped', this.hiddenShapes);
    // **끝에서 한 번만 센다.** 순회 중에 세면 같은 문구가 도형마다 다시 올라 개수가 부풀고,
    // 그 수는 "몇 개의 문구가 올라섰는가" 가 아니라 "몇 번 추월당했는가" 가 된다.
    this.note('approximated', 'textOrderChanged', this.overtakenTexts);
    return { shapes: this.shapes, texts: this.texts, notes: mergeNotes(this.notes) };
  }
}

/**
 * `<text>` 안에 `<textPath>` 가 있는가 — 있으면 그 글자는 **곧은 한 줄이 아니다**.
 *
 * 자손까지 본다. 도구는 `<text><tspan><textPath>` 처럼 한 겹 더 싸서 내기도 하고, 겉의
 * 자식만 보는 판정은 그때 길 위의 글자를 곧게 펴서 들여온다 — 문서가 말한 적 없는 배치다.
 */
function hasTextPathChild(el: Element): boolean {
  for (const child of Array.from(el.children)) {
    if (tagOf(child) === 'textpath') return true;
    if (hasTextPathChild(child)) return true;
  }
  return false;
}

/** `<tspan>` 이 제 것을 말할 때 무시해도 좋은 속성. 둘 다 **그림에 영향을 주지 않는다**. */
const TSPAN_HARMLESS_ATTRS = new Set(['id', 'xml:space']);

/**
 * 제 자리나 제 스타일을 말한 `<tspan>` 의 수.
 *
 * **속성 이름 목록을 열거하지 않는다.** `x`·`dy`·`fill`·`font-size`·`class`… 를 늘어놓으면
 * 그 목록에 없는 속성이 조용히 통과하고(예: 도구가 내는 `baseline-shift`), 목록의 어느
 * 항목이 실제로 무는지도 알 수 없다. 뒤집어 적는다 — **그림에 영향을 주지 않는 둘 말고
 * 무엇이든 말했으면** 우리가 버린 것이 있다. 맨 `<tspan>`(묶기만 하는 흔한 형태)은 아무것도
 * 잃지 않으므로 세지 않는다.
 */
function countOwnTspans(el: Element): number {
  let count = 0;
  for (const child of Array.from(el.children)) {
    if (tagOf(child) === 'tspan') {
      const list = child.attributes;
      for (let i = 0; i < list.length; i += 1) {
        const attr = list.item(i);
        if (attr !== null && !TSPAN_HARMLESS_ATTRS.has(attr.name.toLowerCase())) {
          count += 1;
          break;
        }
      }
    }
    count += countOwnTspans(child);
  }
  return count;
}

/** 자루에서 활자 속성만 고른다 — CSS 규칙이 낸 표에서 활자 축을 갈라 내는 자리다. */
function pickText(props: Readonly<Record<string, string>>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const name of TEXT_STYLE_PROPS) {
    const value = props[name];
    if (value !== undefined && value.trim() !== '') out[name] = value.trim();
  }
  return out;
}

// --- viewBox 와 크기 ----------------------------------------------------

/** `viewBox="minX minY w h"` — 네 수가 유한하고 두 치수가 양수일 때만 성립한다. */
function readViewBox(attrs: AttrBag): ViewBox | undefined {
  const raw = attrs['viewbox'];
  if (raw === undefined) return undefined;
  const numbers = parseNumberList(raw);
  if (numbers.length < 4) return undefined;
  const [minX, minY, width, height] = numbers as [number, number, number, number];
  if (![minX, minY, width, height].every((n) => Number.isFinite(n))) return undefined;
  if (!(width > 0) || !(height > 0)) return undefined;
  return { minX, minY, width, height };
}

/** `width`/`height` 속성 — **단위 없는 수 또는 `px` 만**. `%` 는 크기가 아니다. */
function readSize(attrs: AttrBag): { width: number; height: number } | undefined {
  const width = parseLength(attrs['width']);
  const height = parseLength(attrs['height']);
  if (width === undefined || height === undefined) return undefined;
  if (!(width > 0) || !(height > 0)) return undefined;
  return { width, height };
}

// --- 입구 --------------------------------------------------------------

/**
 * 문자열 하나를 `ImportedShape[]` 로. **007 에서 `DOMParser` 를 부르는 유일한 자리다.**
 *
 * 예외를 밖으로 던지지 않는다(REQ-07) — `DOMParser` 자체가 던지는 환경(구형 · 정책 차단)도
 * 값으로 실패가 돌아온다.
 */
export function readSvgDocument(text: string): SvgDocumentOutcome {
  let doc: Document;
  try {
    doc = new DOMParser().parseFromString(text, 'image/svg+xml');
  } catch {
    return { ok: false, refusal: { reason: 'notSvg', actual: 0, limit: 0 } };
  }
  if (parseFailure(doc)) {
    return { ok: false, refusal: { reason: 'notSvg', actual: 0, limit: 0 } };
  }
  const root = doc.documentElement;
  const attrs = attrBag(root);
  const { shapes, texts, notes } = new DocumentWalker(root).run();
  return {
    ok: true,
    document: { shapes, texts, notes, viewBox: readViewBox(attrs), size: readSize(attrs) },
  };
}
