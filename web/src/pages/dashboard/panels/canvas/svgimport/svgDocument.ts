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
// 가는 길이 닫히며(그래서 `<style>` CSS 미지원이 게으름이 아니라 이 경계의 귀결이다),
// 붙지 않은 문서는 외부 자원을 가져오지 않는다.
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
  type ImportedShape,
} from './svgImportTypes';
import { parseLength, parseNumberList, parseSvgPathData } from './svgPathData';
import { isShorthandShapeTag, shorthandShapeCommands, type AttrBag } from './svgShapes';
import {
  collectStyleAtoms,
  foldAlphaIntoColor,
  inheritStyleAtoms,
  isHidden,
  mergeNotes,
  parseOpacity,
  resolveStyle,
  type StyleAtoms,
} from './svgStyle';
import { parseCSSRules, getCSSPropertiesForElement, type CSSRules } from './svgCssRules';
import {
  determinant,
  IDENTITY_MATRIX,
  isSimilarity,
  multiplyMatrix,
  parseTransformList,
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

/** 안으로 내려가는 그릇. `<a>` 는 SVG 사양의 컨테이너이며 도구가 실제로 도형을 감싼다. */
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

interface WalkContext {
  /** 조상까지 누적한 변환. **조상이 바깥이다.** */
  readonly matrix: Matrix2x3;
  /** 조상에서 물려받은 **칠** 속성들(감춤은 여기 없다 — 위 머리말). */
  readonly atoms: StyleAtoms;
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
  private readonly notes: ImportNote[] = [];
  private readonly byId = new Map<string, Element>();
  private hiddenShapes = 0;
  private cssRules: CSSRules = {};

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
   * SVG 문서의 모든 `<style>` 태그에서 CSS 규칙을 수집한다 (defect A 수정).
   *
   * **지원**: element 선택자, .class 선택자, #id 선택자
   * **미지원**: cascade, specificity, 의사 클래스, 복합 선택자
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

  /** 명령 목록 하나를 요소 후보로 세운다. 감춘 하위 트리에서는 **개수만** 는다. */
  private emit(
    commands: readonly PathCommand[],
    atoms: StyleAtoms,
    ctx: WalkContext,
    hidden: boolean,
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
    this.shapes.push({
      commands: transformed,
      style: resolved.style,
      closed: transformed.some((cmd) => cmd.c === 'Z'),
      hasOwnStyle: resolved.hasOwnStyle,
      evenOdd: resolved.evenOdd,
    });
  }

  /** 한 요소의 순회 문맥 — 변환 누적 · 칠 상속 · 감춤 전파 · 그룹 불투명도. */
  private descend(attrs: AttrBag, parent: WalkContext, tagName?: string): { ctx: WalkContext; own: StyleAtoms; hidden: boolean } {
    const ownAtoms = collectStyleAtoms(attrs);
    // CSS 규칙 적용 (defect A 수정) — 인라인 속성이 CSS 규칙을 덮는다.
    const classAttr = attrs['class'];
    const cssProps = getCSSPropertiesForElement(tagName ?? '', classAttr, this.cssRules);
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
    return {
      ctx: {
        matrix: multiplyMatrix(parent.matrix, parseTransformList(attrs['transform'])),
        atoms: own,
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
    if (!isSvgElement(el)) return; // 미지 네임스페이스 — 버림이 아니다(그려지지 않는다).
    const tag = tagOf(el);
    if (NON_RENDERED_TAGS.has(tag)) {
      // `<style>` 만 예외로 보고에 오른다 — 그 규칙들이 **그림에 영향을 준다**.
      if (tag === 'style' && !isHiddenContext(parent)) {
        this.note('dropped', 'styleRuleDropped', countCssRules(el.textContent ?? ''));
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
      this.emit(shorthandShapeCommands(tag, attrs) ?? [], own, ctx, hidden);
      return;
    }

    if (hidden) return; // 감춘 하위 트리의 미지원 내용은 보고하지 않는다(위 머리말).

    switch (tag) {
      case 'text':
        this.note('dropped', 'textDropped');
        return;
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
        // 모르는 SVG 요소 — 그려지지 않으므로 보고하지 않는다(위험 R7).
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
  run(): { shapes: readonly ImportedShape[]; notes: readonly ImportNote[] } {
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
      visibility: rootAtoms['visibility'],
      displayNone: rootAtoms['display'] === 'none',
      opacity: parseOpacity(rootAtoms['opacity']) ?? 1,
      depth: 0,
      expanding: new Set<string>(),
    };
    this.walkChildren(this.root, rootCtx);
    if (this.hiddenShapes > 0) this.note('dropped', 'hiddenDropped', this.hiddenShapes);
    return { shapes: this.shapes, notes: mergeNotes(this.notes) };
  }
}

/** `<style>` 안의 규칙 수를 센다 — 보고는 "규칙이 있습니다" 가 아니라 "규칙 N개" 다. */
function countCssRules(css: string): number {
  let count = 0;
  for (const ch of css) if (ch === '}') count += 1;
  // 닫는 중괄호가 없어도 내용이 있으면 규칙 하나로 센다(닫히지 않은 CSS 도 브라우저는 읽는다).
  return count > 0 ? count : css.trim() === '' ? 0 : 1;
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
  const { shapes, notes } = new DocumentWalker(root).run();
  return {
    ok: true,
    document: { shapes, notes, viewBox: readViewBox(attrs), size: readSize(attrs) },
  };
}
