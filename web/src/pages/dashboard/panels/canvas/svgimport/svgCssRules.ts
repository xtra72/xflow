// 손으로 쓴 작은 CSS 규칙 파서 (SPEC-CANVAS-007).
//
// **왜 손으로 쓰는가.** 브라우저에게 CSS 를 물으려면 문서를 살아 있는 문서에 붙이고
// `getComputedStyle` 을 불러야 한다. 그 길은 불변식 K3 이 닫아 둔 길이다(붙이지 않는다 →
// 스크립트가 돌 수 없고 외부 자원을 가져오지 않는다). 그래서 이 파서가 있다. 이 파일은
// 문자열만 다루며 DOM 을 **한 마디도** 알지 못한다(불변식 K7).
//
// **지원하는 선택자는 홑마디 셋뿐이다**: `rect` · `.red` · `#myid`.
// **지원하지 않는다**: cascade · specificity · 의사 클래스 · `@media` · 상속 ·
// `!important` · 그리고 **복합 선택자 — 자리를 띄운 `g .a` 든 붙여 쓴 `rect.red` 든 똑같이**.
// 붙여 쓴 쪽을 한동안 통과시켜 표에만 넣어 두었던 것이 이 파일이 물렸던 결함이다.
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
 * 받아들이는 꼴은 `getCSSPropertiesForElement` 이 **열쇠로 만들 수 있는 셋뿐**이다:
 *
 *   - element 이름 `[a-z][a-z0-9-]*` — XML 이름은 글자로 시작한다. 이음표를 남기는 것은
 *     `font-face` · `missing-glyph` · `color-profile` 이 실제 SVG element 이름이기
 *     때문이고, 밑줄과 앞자리 숫자를 버리는 것은 그런 SVG element 이름이 **없어** 그 칸이
 *     열쇠와 영영 맞지 않기 때문이다.
 *   - class `\.[a-z0-9_-]+` — class 속성은 CDATA 라 `_foo` · `-foo` · `9foo` 가 실제로
 *     오고, `classAttr.split(/\s+/)` 이 그것을 그대로 열쇠로 만든다. 그래서 이름의 첫
 *     글자를 element 만큼 죄지 않는다.
 *   - id `#[a-z0-9_-]+` — 같은 이유.
 *
 * **이름이 비면 받지 않는다**(`+`, `*` 가 아니다). 홑점 `.` 하나는 `class=" "` 가 만드는
 * 빈 이름과 맞아 **칠을 하고 있었다**(실측). 어느 브라우저도 `.` 을 선택자로 읽지 않으므로
 * (파싱 오류 → 규칙 통째 버림) 그 칠은 아무도 따라 하지 않는 칠이었고, 이제 브라우저와 같이
 * 버리되 **말없이 버리지 않는다** — 아래 셈이 그 한 줄을 든다.
 *
 * **`.` 과 `#` 이 글자 칸 안에 없다는 것이 이 술어의 전부다.** 예전 술어는 둘을 글자 칸에
 * 넣어 두어 `rect.red` · `.a.b` · `rect#top` 같은 **붙여 쓴 복합 선택자**를 통과시켰다.
 * 그런 선택자는 표에 칸을 얻지만 위의 열쇠 셋 중 어느 것도 되지 못해 **아무도 열지 않는다** —
 * 적용되지도, 보고되지도 않는 조용한 자리였다. 좁히기로 적용이 달라지는 것은 없고(닿은 적이
 * 없다), 달라지는 것은 **아래 셈이 그것을 말하게 된다**는 것뿐이다.
 *
 * **cascade 도 specificity 도 여기에 없다**(머리말). 가중치를 셈하지 않기에 이 파서는 문서를
 * 살아 있는 문서에 붙이지 않고도 설 수 있었고(불변식 K3), 복합 선택자를 받아들이려면 바로
 * 그 가중치가 필요해진다 — 좁히기는 그 단순함을 지키는 쪽이다.
 *
 * `parseCSSRules` 와 `countUnsupportedCssRules` 가 **같은 술어를 쓴다**. 둘이 제 검사를
 * 따로 들면 한쪽만 고친 변경이 "적용하지 않으면서 보고도 하지 않는" 조용한 구멍을 연다.
 */
const SUPPORTED_SELECTOR = /^(?:[a-z][a-z0-9-]*|[.#][a-z0-9_-]+)$/i;

function isSupportedSelector(selector: string): boolean {
  // 두 호출부가 이미 소문자로 접어 넘기므로 `i` 표는 **덧댄 것**이다 — 접지 않는 셋째
  // 호출부가 생겨도 술어가 조용히 달라지지 않도록 남긴다.
  return SUPPORTED_SELECTOR.test(selector);
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
  // 자리가 생긴다.
  //
  // **소문자 접기는 두 쪽이 같지 않다 — 이 짝이 아직 고르지 못한 자리다.** 여기서는 선택자와
  // `idAttr` 을 양쪽 다 접지만, `svgDocument.indexIds` 는 id 표를 **속성 값 그대로** 키잉하고
  // `<use>` 도 그대로 찾는다. 그래서 `#Foo` 규칙은 `id="foo"` 에 닿는데 `<use href="#Foo">` 는
  // 빗나간다 — 브라우저는 SVG 가 XML 이라 **둘 다** 빗나간다. 복합 선택자 좁히기는 이 자리를
  // **건드리지 않는다**: 고치려면 두 쪽을 함께 옮겨야 하고(한쪽만 옮기면 `<use>` 나 CSS 중
  // 하나가 조용히 어긋난다), 그것은 이 결함과 다른 결함이다.
  if (idAttr) {
    const idSelector = `#${idAttr.toLowerCase()}`;
    if (rules[idSelector]) {
      Object.assign(result, rules[idSelector]);
    }
  }

  return result;
}
