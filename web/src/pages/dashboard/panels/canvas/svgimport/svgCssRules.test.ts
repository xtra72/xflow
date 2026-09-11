// SVG CSS 규칙 파서 시험 (SPEC-CANVAS-007, defect A 수정).

import { describe, expect, it } from 'vitest';

import { getCSSPropertiesForElement, parseCSSRules, type CSSRules } from './svgCssRules';

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

    const props = getCSSPropertiesForElement('rect', 'red', rules);
    // element 선택자 먼저, 그다음 class 선택자가 덮는다
    expect(props['fill']).toBe('#ff0000');
    expect(props['stroke']!).toBe('#000');
  });

  it('element 선택자만 적용한다', () => {
    const rules: CSSRules = {
      'circle': { 'fill': '#145a32' },
    };

    const props = getCSSPropertiesForElement('circle', undefined, rules);
    expect(props['fill']).toBe('#145a32');
  });

  it('class 속성이 없으면 class 선택자를 무시한다', () => {
    const rules: CSSRules = {
      'rect': { 'fill': '#c0392b' },
      '.red': { 'fill': '#ff0000' },
    };

    const props = getCSSPropertiesForElement('rect', undefined, rules);
    expect(props['fill']).toBe('#c0392b');
  });

  it('여러 class 를 처리한다', () => {
    const rules: CSSRules = {
      '.red': { 'fill': '#ff0000' },
      '.bold': { 'stroke-width': '5' },
    };

    const props = getCSSPropertiesForElement('rect', 'red bold', rules);
    expect(props['fill']).toBe('#ff0000');
    expect(props['stroke-width']).toBe('5');
  });

  it('선택자가 대소문자를 구분하지 않는다', () => {
    const css = 'RECT { fill: red; } .RED { stroke: blue; }';
    const rules = parseCSSRules(css);

    const props = getCSSPropertiesForElement('Rect', 'Red', rules);
    expect(props['fill']).toBe('red');
    expect(props['stroke']).toBe('blue');
  });
});
