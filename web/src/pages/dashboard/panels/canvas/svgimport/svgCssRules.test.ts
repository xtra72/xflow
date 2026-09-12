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

  it('복합 선택자는 건너뜬다', () => {
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

describe('읽는 선택자의 꼴 — 홑마디 셋뿐 (조용한 자리 둘 (가))', () => {
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
  ];

  const REJECTED = [
    'rect.red', // 붙여 쓴 복합 — 열쇠를 만들 수 없어 아무도 읽지 못한다
    '.a.b',
    'rect#top',
    '#a.b',
    '.a#b',
    'g .b', // 자리 결합자
    'rect > path',
    'rect + path',
    'rect ~ path',
    'rect, circle', // 쉼표 목록 — 이 파서의 밖이다
    '*',
    '.a:hover', // 의사 클래스
    '[fill]', // 속성 선택자
    '.', // 이름 없는 홑점 — 브라우저도 읽지 못한다
    '#',
    '1abc', // XML element 이름은 숫자로 시작하지 못한다
    'foo_bar', // SVG 에 밑줄을 쓰는 element 이름이 없다
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
    const css = '.a{fill:red}rect.b{fill:blue}#c{fill:green}.d.e{fill:teal}';
    expect(Object.keys(parseCSSRules(css)).sort()).toEqual(['#c', '.a']);
    expect(countUnsupportedCssRules(css)).toBe(2);
  });
});
