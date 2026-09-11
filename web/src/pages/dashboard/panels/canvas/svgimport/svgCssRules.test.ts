// SVG CSS 규칙 파서 시험 (SPEC-CANVAS-007).
//
// **파싱 단언만으로는 아무것도 보장되지 않는다.** 아래 "ID 선택자를 파싱한다" 는 규칙이
// **표에 들어가는지**만 재므로, 표를 읽는 자리가 하나도 없어도 초록이다 — 결함 A 가 꼭 그
// 형상으로 한 배포를 살아남았다. 그래서 파싱 단언 옆에는 언제나 **적용 단언**이 서야 하고,
// 그림에 닿았는지를 끝까지 재는 것은 `svgDocument.test.ts` 의 몫이다.

import { describe, expect, it } from 'vitest';

import {
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
