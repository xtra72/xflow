// 손으로 쓴 작은 CSS 규칙 파서 (SPEC-CANVAS-007).
//
// **왜 손으로 쓰는가.** 브라우저에게 CSS 를 물으려면 문서를 살아 있는 문서에 붙이고
// `getComputedStyle` 을 불러야 한다. 그 길은 불변식 K3 이 닫아 둔 길이다(붙이지 않는다 →
// 스크립트가 돌 수 없고 외부 자원을 가져오지 않는다). 그래서 이 파서가 있다. 이 파일은
// 문자열만 다루며 DOM 을 **한 마디도** 알지 못한다(불변식 K7).
//
// **지원하는 선택자는 홑마디 셋뿐이다**: `rect` · `.red` · `#myid`.
// **지원하지 않는다**: cascade · specificity · 의사 클래스 · 복합 선택자 · `@media` ·
// 상속 · `!important`.
//
// **cascade 도 specificity 도 없다는 것이 이 파서를 존재할 수 있게 한 단순함이다.**
// 가중치를 셈하는 대신 **적용 순서**를 하나 정해 두고 뒤에 놓인 것이 앞을 덮는다:
//
//     element  →  .class  →  #id            (뒤가 이긴다)
//
// 그 순서는 진짜 CSS 의 specificity 순서(0,0,1 < 0,1,0 < 1,0,0)와 **결과가 같아 보이지만**
// 같은 것이 아니다 — 여기에는 가중치 산술도, 출처(author/user/UA)도, `!important` 도 없다.
// 같은 선택자가 두 번 나오면 `parseCSSRules` 안에서 **뒤에 적힌 것이 표에 남는다.**
//
// **선택자와 class·id 를 양쪽 모두 소문자로 접는다.** SVG 는 XML 이라 class 와 id 가
// 본디 대소문자를 가리지만, 접기를 한쪽만 하면 표가 두 벌의 이름 규칙을 갖게 되고
// `.RED` 규칙이 `class="RED"` 에 닿지 못하는 자리가 생긴다. 접기의 값은 `#Foo` 와 `#foo`
// 가 한 칸을 다툰다는 것이고, 그 대가는 이 크기의 파서에서 치를 만하다.

/** 파싱된 CSS 규칙: 선택자 → 속성들. 선택자는 **소문자로 접혀** 있다. */
export type CSSRules = Record<string, Record<string, string>>;

/** 규칙 한 덩이 — `선택자 { 선언들 }`. 닫는 중괄호가 있어야 잡힌다. */
const RULE_BLOCK = /([^{]+)\s*\{\s*([^}]*)\s*\}/g;

/**
 * 이 파서가 **읽을 수 있는** 선택자의 꼴 — 홑마디 하나.
 *
 * `parseCSSRules` 와 `countUnsupportedCssRules` 가 **같은 술어를 쓴다**. 둘이 제 검사를
 * 따로 들면 한쪽만 고친 변경이 "적용하지 않으면서 보고도 하지 않는" 조용한 구멍을 연다.
 */
function isSupportedSelector(selector: string): boolean {
  return /^[a-z0-9#.][a-z0-9#._-]*$/i.test(selector);
}

/** 주석을 걷는다 — 주석 안의 중괄호가 덩이 가르기를 어지럽히지 않도록 먼저 한다. */
function stripCssComments(cssText: string): string {
  return cssText.replace(/\/\*[\s\S]*?\*\//g, '');
}

/**
 * CSS 텍스트에서 규칙을 파싱한다.
 *
 * - element 선택자: `rect { fill: red }`
 * - class 선택자: `.red { fill: blue }`
 * - ID 선택자: `#myid { fill: green }`
 *
 * 읽지 못한 선택자는 **조용히 건너뛰지 않는다** — 그 개수는 `countUnsupportedCssRules` 가
 * 세어 화면의 보고에 오른다.
 */
export function parseCSSRules(cssText: string): CSSRules {
  const rules: CSSRules = {};
  const css = stripCssComments(cssText);

  for (const match of css.matchAll(RULE_BLOCK)) {
    const selector = match[1]?.trim().toLowerCase() ?? '';
    const declarations = match[2] ?? '';

    if (!isSupportedSelector(selector)) continue; // 복합 선택자는 건너뜀

    // 속성 파싱
    const props: Record<string, string> = {};
    for (const decl of declarations.split(';')) {
      const colonIdx = decl.indexOf(':');
      if (colonIdx !== -1) {
        const propName = decl.slice(0, colonIdx).trim().toLowerCase();
        const propValue = decl.slice(colonIdx + 1).trim();
        if (propName && propValue) {
          props[propName] = propValue;
        }
      }
    }

    if (Object.keys(props).length > 0) {
      rules[selector] = props;
    }
  }

  return rules;
}

/**
 * **적용하지 못한** 규칙의 수 — 화면의 버림 보고가 드는 수다.
 *
 * 세는 것은 **선택자를 읽지 못한 덩이**뿐이다. 적용된 규칙을 함께 세면 보고가 거의 모든
 * 파일에서 울려 아무것도 말하지 않게 되고(위험 R7), 쓴 것을 버렸다고 말하는 거짓이 된다.
 *
 * 닫는 중괄호가 없어 덩이로 잡히지도 못한 꼬리는 **한 줄로 센다** — 브라우저는 닫히지 않은
 * CSS 도 읽으므로 그 자리는 "규칙이 없다" 가 아니라 "우리가 읽지 못했다" 이다. 꼬리에
 * `{` 가 있을 때만 세어, 마지막 덩이 뒤에 남은 `}` 조각이 한 줄을 더 만들지 않게 한다.
 */
export function countUnsupportedCssRules(cssText: string): number {
  const css = stripCssComments(cssText);
  let count = 0;
  let consumed = 0;

  for (const match of css.matchAll(RULE_BLOCK)) {
    if (!isSupportedSelector(match[1]?.trim().toLowerCase() ?? '')) count += 1;
    consumed = (match.index ?? 0) + match[0].length;
  }

  if (css.slice(consumed).includes('{')) count += 1;
  return count;
}

/**
 * 태그명 · class · id 에 기반해 CSS 속성을 얻는다.
 *
 * 적용 순서는 **element → .class → #id** 이고 뒤가 앞을 덮는다(머리말의 순서 표).
 * 여기서 나온 값은 호출부에서 다시 **인라인 표현 속성에게 덮인다** — 이 함수는 그
 * 마지막 한 겹을 알지 못한다.
 *
 * `idAttr` 이 **선택 인자가 아닌 것에 뜻이 있다.** 기본값을 주면 id 를 넘기지 않은 호출부가
 * 조용히 컴파일되고, 규칙표에 들어간 `#id` 를 아무도 읽지 않는 자리가 다시 생긴다 —
 * 이 함수가 실제로 물렸던 결함이 꼭 그 꼴이었다. 필수 인자는 그 결함을 컴파일러의 일로
 * 만든다.
 */
export function getCSSPropertiesForElement(
  tagName: string,
  classAttr: string | undefined,
  idAttr: string | undefined,
  rules: CSSRules,
): Record<string, string> {
  const result: Record<string, string> = {};

  // element 선택자 적용
  const tagSelector = tagName.toLowerCase();
  if (rules[tagSelector]) {
    Object.assign(result, rules[tagSelector]);
  }

  // class 선택자 적용 (element 선택자를 덮는다)
  if (classAttr) {
    for (const className of classAttr.split(/\s+/)) {
      const classSelector = `.${className.toLowerCase()}`;
      if (rules[classSelector]) {
        Object.assign(result, rules[classSelector]);
      }
    }
  }

  // id 선택자 적용 (class 선택자를 덮는다) — 요소는 id 를 하나만 갖는다.
  //
  // **다듬지 않는다**(`trim` 없음). 처음에는 방어로 `trim()` 을 두었으나 그 가지를 무는
  // 시험이 없었고, 세우려 보니 세워서는 안 되는 것이었다: 같은 문서의 id 를 `<use>` 는
  // `svgDocument` 의 id 표에서 **속성 값 그대로** 찾는다. 여기서만 다듬으면 한 파일 안에
  // id 를 맞추는 규칙이 두 벌이 되어, `#x` 규칙은 닿는데 `<use href="#x">` 는 빗나가는
  // 자리가 생긴다. 소문자 접기만 두 쪽에서 같은 규칙으로 한다.
  if (idAttr) {
    const idSelector = `#${idAttr.toLowerCase()}`;
    if (rules[idSelector]) {
      Object.assign(result, rules[idSelector]);
    }
  }

  return result;
}
