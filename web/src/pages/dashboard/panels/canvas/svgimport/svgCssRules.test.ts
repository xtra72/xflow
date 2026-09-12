// SVG CSS 규칙 파서 시험 (SPEC-CANVAS-007).
//
// **파싱 단언만으로는 아무것도 보장되지 않는다.** 아래 "ID 선택자를 파싱한다" 는 규칙이
// **표에 들어가는지**만 재므로, 표를 읽는 자리가 하나도 없어도 초록이다 — 결함 A 가 꼭 그
// 형상으로 한 배포를 살아남았다. 그래서 파싱 단언 옆에는 언제나 **적용 단언**이 서야 하고,
// 그림에 닿았는지를 끝까지 재는 것은 `svgDocument.test.ts` 의 몫이다.

import { describe, expect, it } from 'vitest';

import {
  countUnsupportedCssRules,
  getCSSPropertiesForElement,
  parseCSSRules,
  type CSSRules,
} from './svgCssRules';

describe('SVG CSS 규칙 파싱', () => {
  it('element 선택자를 파싱한다', () => {
    const css = 'rect { fill: #c0392b; stroke: #145a32; }';
    const rules = parseCSSRules(css);

    expect(rules['rect']).toBeDefined();
    expect(rules['rect']!['fill']).toBe('#c0392b');
    expect(rules['rect']!['stroke']).toBe('#145a32');
  });

  it('class 선택자를 파싱한다', () => {
    const css = '.red { fill: #c0392b; } .blue { stroke: #145a32; }';
    const rules = parseCSSRules(css);

    expect(rules['.red']).toBeDefined();
    expect(rules['.red']!['fill']).toBe('#c0392b');
    expect(rules['.blue']).toBeDefined();
    expect(rules['.blue']!['stroke']).toBe('#145a32');
  });

  it('ID 선택자를 파싱한다', () => {
    const css = '#myid { opacity: 0.5; }';
    const rules = parseCSSRules(css);

    expect(rules['#myid']).toBeDefined();
    expect(rules['#myid']!['opacity']).toBe('0.5');
  });

  it('주석을 제거한다', () => {
    const css = '/* 주석 */ rect { fill: red; } /* 또 다른 주석 */';
    const rules = parseCSSRules(css);

    expect(rules['rect']).toBeDefined();
    expect(rules['rect']!['fill']).toBe('red');
  });

  it('결합자를 쓴 선택자는 건너뜬다', () => {
    const css = 'rect > path { fill: red; } g path { stroke: blue; }';
    const rules = parseCSSRules(css);

    // 복합 선택자는 무시된다
    expect(rules['rect > path']).toBeUndefined();
    expect(rules['g path']).toBeUndefined();
  });

  it('마지막 규칙이 이긴다', () => {
    const css = 'rect { fill: red; } rect { fill: blue; }';
    const rules = parseCSSRules(css);

    expect(rules['rect']!['fill']).toBe('blue');
  });

  it('요소별로 CSS 속성을 얻는다', () => {
    const rules: CSSRules = {
      'rect': { 'fill': '#c0392b' },
      '.red': { 'fill': '#ff0000', 'stroke': '#000' },
    };

    const props = getCSSPropertiesForElement('rect', 'red', undefined, rules);
    // element 선택자 먼저, 그다음 class 선택자가 덮는다
    expect(props['fill']).toBe('#ff0000');
    expect(props['stroke']!).toBe('#000');
  });

  it('element 선택자만 적용한다', () => {
    const rules: CSSRules = {
      'circle': { 'fill': '#145a32' },
    };

    const props = getCSSPropertiesForElement('circle', undefined, undefined, rules);
    expect(props['fill']).toBe('#145a32');
  });

  it('class 속성이 없으면 class 선택자를 무시한다', () => {
    const rules: CSSRules = {
      'rect': { 'fill': '#c0392b' },
      '.red': { 'fill': '#ff0000' },
    };

    const props = getCSSPropertiesForElement('rect', undefined, undefined, rules);
    expect(props['fill']).toBe('#c0392b');
  });

  it('여러 class 를 처리한다', () => {
    const rules: CSSRules = {
      '.red': { 'fill': '#ff0000' },
      '.bold': { 'stroke-width': '5' },
    };

    const props = getCSSPropertiesForElement('rect', 'red bold', undefined, rules);
    expect(props['fill']).toBe('#ff0000');
    expect(props['stroke-width']).toBe('5');
  });

  it('선택자가 대소문자를 구분하지 않는다', () => {
    const css = 'RECT { fill: red; } .RED { stroke: blue; }';
    const rules = parseCSSRules(css);

    const props = getCSSPropertiesForElement('Rect', 'Red', undefined, rules);
    expect(props['fill']).toBe('red');
    expect(props['stroke']).toBe('blue');
  });
});

describe('#id 선택자는 표에 들어갈 뿐 아니라 **읽힌다**', () => {
  const RULES: CSSRules = {
    'rect': { 'fill': '#111111' },
    '.mid': { 'fill': '#222222' },
    '#top': { 'fill': '#333333' },
  };

  it('id 를 넘기면 #id 규칙이 잡힌다', () => {
    expect(getCSSPropertiesForElement('circle', undefined, 'top', RULES)['fill']).toBe('#333333');
  });

  it('#id 가 .class 를 덮고 .class 가 element 를 덮는다 — 적용 순서가 이 파서의 전부다', () => {
    expect(getCSSPropertiesForElement('rect', 'mid', 'top', RULES)['fill']).toBe('#333333');
    expect(getCSSPropertiesForElement('rect', 'mid', undefined, RULES)['fill']).toBe('#222222');
    expect(getCSSPropertiesForElement('rect', undefined, undefined, RULES)['fill']).toBe('#111111');
  });

  it('id 가 어느 규칙과도 맞지 않으면 아래 겹이 그대로 남는다', () => {
    expect(getCSSPropertiesForElement('rect', 'mid', 'nope', RULES)['fill']).toBe('#222222');
  });

  it('id 도 대소문자를 접는다 — class 와 같은 규칙이다', () => {
    const rules = parseCSSRules('#TOP { fill: #333333; }');
    expect(getCSSPropertiesForElement('rect', undefined, 'Top', rules)['fill']).toBe('#333333');
  });
});

describe('적용하지 못한 규칙만 센다 (결함 B)', () => {
  it('읽은 선택자는 세지 않는다', () => {
    expect(countUnsupportedCssRules('rect{fill:red}.a{fill:blue}#b{fill:green}')).toBe(0);
  });

  it('복합 선택자는 하나씩 센다', () => {
    expect(countUnsupportedCssRules('.a{fill:red}g .b{fill:blue}rect > path{fill:green}')).toBe(2);
  });

  it('빈 텍스트도 공백뿐인 텍스트도 0 이다', () => {
    expect(countUnsupportedCssRules('')).toBe(0);
    expect(countUnsupportedCssRules('   \n  ')).toBe(0);
  });

  it('주석뿐인 텍스트는 0 이다 — 주석 안의 중괄호가 한 줄을 만들지 않는다', () => {
    expect(countUnsupportedCssRules('/* .a { fill: red } */')).toBe(0);
  });

  it('닫히지 않은 꼬리는 한 줄로 센다', () => {
    expect(countUnsupportedCssRules('.a{fill:red')).toBe(1);
    expect(countUnsupportedCssRules('.a{fill:red}.b{fill:blue')).toBe(1);
  });

  it('마지막 덩이 뒤에 남은 `}` 조각은 한 줄을 더 만들지 않는다', () => {
    // `@media` 한 덩이는 **하나**로 센다 — 바깥 선택자가 읽히지 않아 1, 남은 `}` 는 0.
    expect(countUnsupportedCssRules('@media print { .a { fill: red } }')).toBe(1);
  });
});

describe('읽는 선택자의 꼴 — 홑마디의 이어 붙임 (조용한 자리 둘 (가))', () => {
  /** 규칙 한 덩이가 표에 담겼는가. */
  function stored(css: string): boolean {
    return Object.keys(parseCSSRules(css)).length === 1;
  }

  const ACCEPTED = [
    'rect', // element
    'circle',
    'font-face', // 이음표를 쓰는 SVG element 이름이 실제로 있다
    'lineargradient', // 소문자로 접힌 뒤의 꼴
    '.red', // class
    '.cls-1',
    '._foo', // class 속성은 CDATA 라 밑줄·이음표·숫자로 시작할 수 있다
    '.-foo',
    '.9foo',
    '#top', // id
    '#top_1',
    // 이어 붙인 복합 — 일러스트레이터·잉크스케이프가 실제로 뱉는 꼴이다.
    'rect.red',
    '.a.b',
    'rect#top',
    '#a.b',
    '.a#b',
    'font-face.a', // 이음표 element 뒤에도 마디가 붙는다
    'rect.a.b.c', // 마디 수에 상한이 없다
    '#a#b', // 표에는 담기되 어느 요소와도 맞지 않는다 — 브라우저와 같은 답이다
  ];

  const REJECTED = [
    'g .b', // 자리 결합자
    'rect > path',
    'rect.a > .b', // 복합을 받아들여도 결합자는 여전히 밖이다
    'rect .a', // 띄어쓰기 하나가 이어 붙임과 자리 결합자를 가른다
    'rect + path',
    'rect ~ path',
    'rect, circle', // 쉼표 목록 — 이 파서의 밖이다
    'rect.a, circle', // 복합이 섞인 쉼표 목록도 밖이다
    '*',
    '.a:hover', // 의사 클래스
    'rect.a:hover',
    '[fill]', // 속성 선택자
    'rect.a[fill]',
    '.', // 이름 없는 홑점 — 브라우저도 읽지 못한다
    '#',
    'rect.', // 마디 이름이 비면 이어 붙임도 받지 않는다
    '.a.', // 뒤따르는 빈 마디도 같다
    '1abc', // XML element 이름은 숫자로 시작하지 못한다
    'foo_bar', // SVG 에 밑줄을 쓰는 element 이름이 없다
    'foo_bar.a', // element 이름의 규칙은 마디가 붙어도 느슨해지지 않는다
  ];

  for (const sel of ACCEPTED) {
    it(`\`${sel}\` 는 표에 담기고 세지 않는다`, () => {
      const css = `${sel}{fill:#c0392b}`;
      expect(stored(css)).toBe(true);
      expect(parseCSSRules(css)[sel]!['fill']).toBe('#c0392b');
      expect(countUnsupportedCssRules(css)).toBe(0);
    });
  }

  for (const sel of REJECTED) {
    it(`\`${sel}\` 는 표에 담기지 않고 한 줄로 센다`, () => {
      const css = `${sel}{fill:#c0392b}`;
      expect(parseCSSRules(css)).toEqual({});
      expect(countUnsupportedCssRules(css)).toBe(1);
    });
  }

  it('담기는 것과 세지 않는 것은 **같은 술어**를 본다 — 한쪽만 고친 변경이 여기서 빨개진다', () => {
    // 둘이 제 검사를 따로 들면 "적용하지 않으면서 보고도 하지 않는" 구멍이 다시 열린다.
    // 선언이 하나라도 성한 덩이에 대해 **담김 ⟺ 세지 않음** 이 성립해야 한다.
    // (선언이 비거나 망가진 덩이는 이 맞걸림의 밖이다 — 담기지도 세지도 않는다.)
    for (const sel of [...ACCEPTED, ...REJECTED]) {
      const css = `${sel}{fill:#c0392b}`;
      expect({ sel, stored: stored(css), counted: countUnsupportedCssRules(css) }).toEqual({
        sel,
        stored: ACCEPTED.includes(sel),
        counted: ACCEPTED.includes(sel) ? 0 : 1,
      });
    }
  });

  it('여러 덩이가 섞여도 읽은 것만 담기고 읽지 못한 것만 세진다', () => {
    const css = '.a{fill:red}rect.b{fill:blue}#c{fill:green}g .d{fill:teal}';
    expect(Object.keys(parseCSSRules(css)).sort()).toEqual(['#c', '.a', 'rect.b']);
    expect(countUnsupportedCssRules(css)).toBe(1);
  });
});

// --- 이어 붙인 복합 선택자의 맞춰 보기와 적용 순서 ---------------------------
//
// **여기 있는 것은 표가 아니라 적용이다.** 표에 칸이 생기는지는 위 블록이 재고, 이 블록은
// `getCSSPropertiesForElement` 이 실제로 그 칸을 연다는 것을 잰다. 그림까지 닿는지는
// `svgCssCompound.test.ts` 가 문서 → 계획 → 요소 → 그린 기록 끝까지 몰아 잰다.

describe('이어 붙인 복합 선택자는 **마디가 전부 맞아야** 선다', () => {
  const RULES = parseCSSRules(
    'rect.red{fill:#c0392b}.a.b{fill:#145a32}rect#top{fill:#8e44ad}#a.b{fill:#2980b9}',
  );

  it('`rect.red` 는 태그와 class 가 둘 다 맞을 때만 선다', () => {
    expect(getCSSPropertiesForElement('rect', 'red', undefined, RULES)['fill']).toBe('#c0392b');
    // 태그가 어긋난다.
    expect(getCSSPropertiesForElement('circle', 'red', undefined, RULES)['fill']).toBeUndefined();
    // class 가 어긋난다.
    expect(getCSSPropertiesForElement('rect', 'blue', undefined, RULES)['fill']).toBeUndefined();
    // class 가 아예 없다.
    expect(
      getCSSPropertiesForElement('rect', undefined, undefined, RULES)['fill'],
    ).toBeUndefined();
  });

  it('`.a.b` 는 class 둘을 **함께** 요구한다 — 하나만으로는 서지 않는다', () => {
    expect(getCSSPropertiesForElement('rect', 'a b', undefined, RULES)['fill']).toBe('#145a32');
    expect(getCSSPropertiesForElement('rect', 'b a', undefined, RULES)['fill']).toBe('#145a32');
    expect(getCSSPropertiesForElement('rect', 'a', undefined, RULES)['fill']).toBeUndefined();
    expect(getCSSPropertiesForElement('rect', 'b', undefined, RULES)['fill']).toBeUndefined();
  });

  it('`rect#top` 과 `#a.b` 는 id 마디까지 맞아야 선다', () => {
    expect(getCSSPropertiesForElement('rect', undefined, 'top', RULES)['fill']).toBe('#8e44ad');
    expect(getCSSPropertiesForElement('circle', undefined, 'top', RULES)['fill']).toBeUndefined();
    expect(getCSSPropertiesForElement('rect', 'b', 'a', RULES)['fill']).toBe('#2980b9');
    expect(getCSSPropertiesForElement('rect', undefined, 'a', RULES)['fill']).toBeUndefined();
  });

  it('겹쳐 적은 마디(`.a.a`)는 class 하나로 맞는다 — 집합으로 묻기 때문이다', () => {
    const rules = parseCSSRules('.a.a{fill:#c0392b}');
    expect(getCSSPropertiesForElement('rect', 'a', undefined, rules)['fill']).toBe('#c0392b');
    // 겹쳐 적은 class 속성도 같은 자리에서 맞는다.
    expect(getCSSPropertiesForElement('rect', 'a a', undefined, rules)['fill']).toBe('#c0392b');
  });

  it('id 둘을 요구하는 `#a#b` 는 담기되 어느 요소와도 맞지 않는다', () => {
    const rules = parseCSSRules('#a#b{fill:#c0392b}');
    // 켜져 있음을 먼저 못박는다 — 표에 칸이 없으면 아래가 공허하다.
    expect(Object.keys(rules)).toEqual(['#a#b']);
    expect(getCSSPropertiesForElement('rect', undefined, 'a', rules)['fill']).toBeUndefined();
    expect(getCSSPropertiesForElement('rect', undefined, 'b', rules)['fill']).toBeUndefined();
  });

  it('`class=" "` 가 만드는 빈 이름은 어떤 마디와도 맞지 않는다 (홑점 구멍은 닫힌 채다)', () => {
    // 빈 이름은 집합에 남지만 마디 이름이 빌 수 없어 열리지 않는다.
    expect(parseCSSRules('.{fill:#c0392b}')).toEqual({});
    expect(
      getCSSPropertiesForElement('rect', ' ', undefined, parseCSSRules('.a{fill:#c0392b}'))['fill'],
    ).toBeUndefined();
  });
});

describe('적용 순서 — 칸(element → class → id) 이 먼저, 같은 칸 안에서는 원문 순서', () => {
  it('복합은 **제 마디 가운데 가장 센 종류**의 칸에 선다 — 마디를 세지 않는다', () => {
    // `rect.a.b` 는 마디가 셋이지만 `.a` 와 **같은 칸**이다. 마디를 셌다면(specificity)
    // `rect.a.b` 가 뒤에 오든 앞에 오든 이겼을 것이다 — 여기서는 원문 순서가 가른다.
    const later = parseCSSRules('rect.a.b{fill:#c0392b}.a{fill:#145a32}');
    expect(getCSSPropertiesForElement('rect', 'a b', undefined, later)['fill']).toBe('#145a32');
    const earlier = parseCSSRules('.a{fill:#145a32}rect.a.b{fill:#c0392b}');
    expect(getCSSPropertiesForElement('rect', 'a b', undefined, earlier)['fill']).toBe('#c0392b');
  });

  it('id 칸은 원문에서 앞서도 class·element 칸을 덮는다 — 순수 원문 순서가 아니다', () => {
    // 순수 원문 순서였다면 뒤의 `rect` 가 이겨 색이 뒤집힌다. 칸이 셋이라 그러지 않는다.
    const rules = parseCSSRules('#top{fill:#8e44ad}.a{fill:#145a32}rect{fill:#c0392b}');
    expect(getCSSPropertiesForElement('rect', 'a', 'top', rules)['fill']).toBe('#8e44ad');
    expect(getCSSPropertiesForElement('rect', 'a', undefined, rules)['fill']).toBe('#145a32');
  });

  it('복합이 낀 칸 싸움도 칸이 먼저다 — `rect.a` 는 `#top` 을 덮지 못한다', () => {
    const rules = parseCSSRules('#top{fill:#8e44ad}rect.a{fill:#c0392b}');
    expect(getCSSPropertiesForElement('rect', 'a', 'top', rules)['fill']).toBe('#8e44ad');
  });

  it('맞는 규칙이 여럿이면 속성별로 겹쳐진다 — 덮지 않는 속성은 살아남는다', () => {
    const rules = parseCSSRules('rect{stroke:#111111}rect.a{fill:#c0392b}');
    const props = getCSSPropertiesForElement('rect', 'a', undefined, rules);
    expect(props).toEqual({ 'stroke': '#111111', 'fill': '#c0392b' });
  });
});
