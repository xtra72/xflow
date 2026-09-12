// 손으로 쓴 작은 CSS 규칙 파서 (SPEC-CANVAS-007).
//
// **왜 손으로 쓰는가.** 브라우저에게 CSS 를 물으려면 문서를 살아 있는 문서에 붙이고
// `getComputedStyle` 을 불러야 한다. 그 길은 불변식 K3 이 닫아 둔 길이다(붙이지 않는다 →
// 스크립트가 돌 수 없고 외부 자원을 가져오지 않는다). 그래서 이 파서가 있다. 이 파일은
// 문자열만 다루며 DOM 을 **한 마디도** 알지 못한다(불변식 K7).
//
// **지원하는 선택자는 홑마디의 이어 붙임이다**: element 이름 하나(없어도 된다) 뒤에
// `.class` 와 `#id` 마디가 **몇이든 · 어느 차례로든** 붙은 꼴 — `rect` · `.red` · `#myid` ·
// `rect.cls-2` · `.a.b` · `rect#top` · `#a.b`. 마디가 **전부** 맞아야 규칙이 선다.
// `rect.cls-N` 은 일러스트레이터와 잉크스케이프가 실제로 뱉는 꼴이라 변두리가 아니다.
//
// **지원하지 않는다**: 결합자(`g > rect` · `g .a` · `a + b` · `a ~ b`) · 의사 클래스 ·
// 속성 선택자 · `@media` · 쉼표 목록 · cascade · 상속 · `!important`. 그것들은 표에 담기지
// 않고 **한 줄씩 세어져** 화면의 버림 보고에 오른다(`countUnsupportedCssRules`).
//
// **cascade 도 specificity 도 없다는 것이 이 파서를 존재할 수 있게 한 단순함이다.**
// 가중치를 셈하는 대신 **적용 순서**를 하나 정해 두고 뒤에 놓인 것이 앞을 덮는다:
//
//     element  →  .class  →  #id            (뒤가 이긴다)
//     같은 칸 안에서는 **원문 순서** — 뒤에 적힌 규칙이 앞을 덮는다.
//
// 복합 선택자는 **제 마디 가운데 가장 센 종류의 칸**에 선다(`#` 이 있으면 id 칸, 없고 `.` 이
// 있으면 class 칸, 둘 다 없으면 element 칸). 마디를 **세지 않는다** — `rect.a.b` 와 `.a` 는
// 같은 칸에 서고 둘의 승부는 원문 순서가 가른다. 칸이 셋뿐이고 산술이 없다는 뜻에서 이것은
// specificity 가 **아니다**. 진짜 specificity 는 (1,0,0) > (0,1,1) 같은 셈을 하고 출처와
// `!important` 를 가리지만, 여기에는 그 어느 것도 없다.
//
// **왜 순수한 원문 순서가 아닌가.** "맞는 규칙을 모두, 원문 순서로, 뒤가 이긴다" 는 더
// 단순하지만 지금 맞는 답을 틀리게 만든다 — 일러스트레이터가 뱉는 `.cls-1{…}` 뭉치 뒤에
// 손으로 `rect{…}` 한 줄을 더하면 그 순간 `rect` 가 `.cls-1` 을 덮어 색이 뒤집힌다.
// 브라우저는 그 자리에서 `.cls-1` 을 세운다. 칸 셋을 지키면 홑마디의 답이 **한 자리도**
// 달라지지 않고, 복합만 새로 닿는다.
//
// **선택자와 class·id 를 양쪽 모두 소문자로 접는다.** SVG 는 XML 이라 class 와 id 가
// 본디 대소문자를 가리지만, 접기를 한쪽만 하면 표가 두 벌의 이름 규칙을 갖게 되고
// `.RED` 규칙이 `class="RED"` 에 닿지 못하는 자리가 생긴다. 접기의 값은 `#Foo` 와 `#foo`
// 가 한 칸을 다툰다는 것이고, 그 대가는 이 크기의 파서에서 치를 만하다.

/**
 * 파싱된 CSS 규칙: 선택자 → 속성들. 선택자는 **소문자로 접혀** 있다.
 *
 * **키의 차례가 뜻을 나른다** — `getCSSPropertiesForElement` 이 같은 칸 안의 승부를 이
 * 차례로 가르므로, 이 표는 "선택자 집합" 이 아니라 **원문 순서를 실은 목록**이다. JS 객체는
 * 정수꼴이 아닌 문자열 키의 넣은 차례를 지키고, 이 파서의 선택자는 글자·`.`·`#` 로 시작해
 * 정수꼴이 될 수 없다. 같은 선택자가 두 번 오면 **자리는 처음 것이, 값은 나중 것이** 남는다.
 */
export type CSSRules = Record<string, Record<string, string>>;

/** 규칙 한 덩이 — `선택자 { 선언들 }`. 닫는 중괄호가 있어야 잡힌다. */
const RULE_BLOCK = /([^{]+)\s*\{\s*([^}]*)\s*\}/g;

/**
 * 이 파서가 **읽을 수 있는** 선택자의 꼴 — 홑마디의 이어 붙임.
 *
 * 받아들이는 마디는 셋이고, element 이름은 **맨 앞에 한 번만** 올 수 있다:
 *
 *   - element 이름 `[a-z][a-z0-9-]*` — XML 이름은 글자로 시작한다. 이음표를 남기는 것은
 *     `font-face` · `missing-glyph` · `color-profile` 이 실제 SVG element 이름이기
 *     때문이고, 밑줄과 앞자리 숫자를 버리는 것은 그런 SVG element 이름이 **없어** 그 칸이
 *     요소와 영영 맞지 않기 때문이다.
 *   - class `\.[a-z0-9_-]+` — class 속성은 CDATA 라 `_foo` · `-foo` · `9foo` 가 실제로
 *     오고, `classAttr.split(/\s+/)` 이 그것을 그대로 이름으로 만든다. 그래서 이름의 첫
 *     글자를 element 만큼 죄지 않는다.
 *   - id `#[a-z0-9_-]+` — 같은 이유.
 *
 * **이름이 비면 받지 않는다**(`+`, `*` 가 아니다). 홑점 `.` 하나는 `class=" "` 가 만드는
 * 빈 이름과 맞아 **칠을 하고 있었다**(실측). 어느 브라우저도 `.` 을 선택자로 읽지 않으므로
 * (파싱 오류 → 규칙 통째 버림) 그 칠은 아무도 따라 하지 않는 칠이었고, 이제 브라우저와 같이
 * 버리되 **말없이 버리지 않는다** — 아래 셈이 그 한 줄을 든다. 마디 이름이 빌 수 없다는
 * 이 한 가지가 그 구멍을 계속 닫아 둔다: `class=" "` 가 만드는 빈 이름은 어떤 마디와도
 * 맞지 못한다.
 *
 * **띄어쓰기도 그 밖의 글자도 들어오지 못한다는 것이 이 술어의 전부다.** 자리를 띄운
 * `g .a` · 결합자 `>` `+` `~` · 쉼표 목록 · `:hover` · `[fill]` · `@media` 는 모두 이
 * 그물 밖이고, 밖에 있다는 사실이 곧 **아래 셈이 그것을 말한다**는 뜻이다.
 *
 * **가짓수를 두 갈래로 적은 까닭.** "element 이름은 선택, 마디는 0개 이상" 을 한 줄로 적으면
 * **빈 선택자**까지 받는다(`   {fill:red}` 이 실제로 만드는 꼴이다). 두 갈래는 "element 로
 * 시작하거나, 마디로 시작하거나" 를 적어 어느 쪽이든 **적어도 한 마디**를 요구한다.
 *
 * `parseCSSRules` 와 `countUnsupportedCssRules` 가 **같은 술어를 쓴다**. 둘이 제 검사를
 * 따로 들면 한쪽만 고친 변경이 "적용하지 않으면서 보고도 하지 않는" 조용한 구멍을 연다.
 * 그 공유는 시험의 맞걸림 성질(담김 ⟺ 세지 않음)이 지킨다 — 한쪽만 넓히면 거기서 빨개진다.
 */
const SUPPORTED_SELECTOR = /^(?:[a-z][a-z0-9-]*(?:[.#][a-z0-9_-]+)*|(?:[.#][a-z0-9_-]+)+)$/i;

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
 * - 이어 붙인 복합 선택자: `rect.red { fill: teal }` · `.a.b` · `rect#top`
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

    if (!isSupportedSelector(selector)) continue; // 결합자·의사 클래스·쉼표 목록은 건너뜀

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

// --- 선택자 맞춰 보기 -----------------------------------------------------

/** 선택자에서 **마디가 시작되는 자리**. 그 앞이 element 이름이다(없을 수 있다). */
const FIRST_PART = /[.#]/;

/** `.a#b.c` → `['.a', '#b', '.c']`. 앞이 마디로 시작할 때만 부른다 — 빈 조각이 나지 않는다. */
const PART_SPLIT = /(?=[.#])/;

/**
 * 선택자가 서는 **칸** — element(0) · class(1) · id(2). 뒤 칸이 앞 칸을 덮는다.
 *
 * **마디를 세지 않는다.** 가장 센 종류 하나만 본다: `#` 이 있으면 id 칸, 없고 `.` 이 있으면
 * class 칸, 둘 다 없으면 element 칸. 그래서 `rect.a.b` 와 `.a` 가 같은 칸에 서고, 둘의
 * 승부는 표의 차례(= 원문 순서)가 가른다. 이것이 specificity 가 **아닌** 지점이다.
 */
function selectorTier(selector: string): 0 | 1 | 2 {
  if (selector.includes('#')) return 2;
  if (selector.includes('.')) return 1;
  return 0;
}

/**
 * 선택자의 **모든 마디**가 이 요소에 맞는가.
 *
 * - element 이름이 있으면 태그와 같아야 한다. 없으면 태그를 묻지 않는다.
 * - `.name` 은 class 집합에 있어야 한다. 같은 이름이 두 번 와도(`.a.a`) 집합은 한 번만
 *   묻으므로 `class="a"` 가 맞는다 — 브라우저도 그렇게 읽는다(`.a.a` 는 `.a` 와 **맞는
 *   범위가 같고** specificity 만 다른데, 그 셈이 여기에는 없다).
 * - `#name` 은 요소의 id 와 같아야 한다. 요소는 id 를 하나만 가지므로 `#a#b` 같은 선택자는
 *   표에 담기되 **어느 요소와도 맞지 않는다** — 브라우저와 같은 답이라 구멍이 아니다.
 */
function selectorMatches(
  selector: string,
  tagName: string,
  classes: ReadonlySet<string>,
  id: string | undefined,
): boolean {
  const cut = selector.search(FIRST_PART);
  const element = cut === -1 ? selector : selector.slice(0, cut);
  if (element !== '' && element !== tagName) return false;
  if (cut === -1) return true;
  for (const part of selector.slice(cut).split(PART_SPLIT)) {
    const name = part.slice(1);
    if (part.startsWith('.') ? !classes.has(name) : name !== id) return false;
  }
  return true;
}

/**
 * 태그명 · class · id 에 기반해 CSS 속성을 얻는다.
 *
 * 맞는 규칙을 **모두** 모아 칸 순서(element → .class → #id)로, 같은 칸 안에서는 **원문
 * 순서**로 덮어쓴다(머리말의 순서 표). 여기서 나온 값은 호출부에서 다시 **인라인 표현
 * 속성에게 덮인다** — 이 함수는 그 마지막 한 겹을 알지 못한다.
 *
 * **class 를 집합으로 든다.** 규칙마다 class 목록을 다시 훑는 대신 한 번 만들어 두고
 * 마디마다 묻는다. 겹쳐 적은 class(`class="a a"`)가 한 칸으로 줄고, `.a.a` 같은 선택자가
 * 자연히 맞는 것도 여기서 나온다. `class=" "` 가 만드는 **빈 이름**은 집합에 남지만
 * 마디 이름이 빌 수 없어(`SUPPORTED_SELECTOR`) 어떤 마디와도 맞지 않는다 — 걸러 내는
 * 가지를 따로 두지 않는 까닭이 그것이다(그 가지는 아무 시험도 물지 못한다).
 *
 * `idAttr` 이 **선택 인자가 아닌 것에 뜻이 있다.** 기본값을 주면 id 를 넘기지 않은 호출부가
 * 조용히 컴파일되고, 규칙표에 들어간 `#id` 를 아무도 읽지 않는 자리가 다시 생긴다 —
 * 이 함수가 실제로 물렸던 결함이 꼭 그 꼴이었다. 필수 인자는 그 결함을 컴파일러의 일로
 * 만든다.
 *
 * **id 를 다듬지 않는다**(`trim` 없음). 처음에는 방어로 `trim()` 을 두었으나 그 가지를 무는
 * 시험이 없었고, 세우려 보니 세워서는 안 되는 것이었다: 같은 문서의 id 를 `<use>` 는
 * `svgDocument` 의 id 표에서 **속성 값 그대로** 찾는다. 여기서만 다듬으면 한 파일 안에
 * id 를 맞추는 규칙이 두 벌이 되어, `#x` 규칙은 닿는데 `<use href="#x">` 는 빗나가는
 * 자리가 생긴다.
 *
 * **소문자 접기는 두 쪽이 같지 않다 — 이 짝이 아직 고르지 못한 자리다.** 여기서는 선택자와
 * `idAttr` 을 양쪽 다 접지만, `svgDocument.indexIds` 는 id 표를 **속성 값 그대로** 키잉하고
 * `<use>` 도 그대로 찾는다. 그래서 `#Foo` 규칙은 `id="foo"` 에 닿는데 `<use href="#Foo">` 는
 * 빗나간다 — 브라우저는 SVG 가 XML 이라 **둘 다** 빗나간다. 복합 선택자 넓히기는 이 자리를
 * **건드리지 않는다**: 고치려면 두 쪽을 함께 옮겨야 하고(한쪽만 옮기면 `<use>` 나 CSS 중
 * 하나가 조용히 어긋난다), 그것은 이 결함과 다른 결함이다.
 */
export function getCSSPropertiesForElement(
  tagName: string,
  classAttr: string | undefined,
  idAttr: string | undefined,
  rules: CSSRules,
): Record<string, string> {
  const tag = tagName.toLowerCase();
  const classes = new Set((classAttr ?? '').toLowerCase().split(/\s+/));
  const id = idAttr?.toLowerCase();

  // 칸마다 따로 모은 뒤 칸 순서로 붓는다. 한 번에 붓지 않는 것은 표의 차례가 원문 순서일 뿐
  // 칸 순서는 아니기 때문이다 — `#top` 이 `rect` 보다 먼저 적힌 문서에서 그 둘이 갈린다.
  const byTier: [
    Record<string, string>[],
    Record<string, string>[],
    Record<string, string>[],
  ] = [[], [], []];
  for (const [selector, props] of Object.entries(rules)) {
    if (selectorMatches(selector, tag, classes, id)) byTier[selectorTier(selector)].push(props);
  }

  const result: Record<string, string> = {};
  for (const tier of byTier) {
    for (const props of tier) Object.assign(result, props);
  }
  return result;
}
