// 간단한 CSS 규칙 파서 (SPEC-CANVAS-007, defect A 수정).
//
// SVG <style> 요소에서 CSS 규칙을 파싱한다.
// 지원: element 선택자, .class 선택자, #id 선택자
// 미지원: cascade, specificity, 의사 클래스, 복합 선택자

/** 파싱된 CSS 규칙: 선택자 → 속성들 */
export type CSSRules = Record<string, Record<string, string>>;

/**
 * CSS 텍스트에서 규칙을 파싱한다.
 *
 * - element 선택자: `rect { fill: red }`
 * - class 선택자: `.red { fill: blue }`
 * - ID 선택자: `#myid { fill: green }`
 *
 * **지원하지 않음**: cascade, specificity, 의사 클래스, 복합 선택자
 * **거동**: "마지막 규칙이 이긴다" (간단한 순서대로 적용)
 */
export function parseCSSRules(cssText: string): CSSRules {
  const rules: CSSRules = {};

  // 주석 제거
  const css = cssText.replace(/\/\*[\s\S]*?\*\//g, '');

  // 규칙 블록 분리
  const ruleMatches = css.matchAll(/([^{]+)\s*\{\s*([^}]*)\s*\}/g);

  for (const match of ruleMatches) {
    const selector = match[1]?.trim().toLowerCase() ?? '';
    const declarations = match[2] ?? '';

    // element, .class, #id 같은 단순 선택자만 처리
    if (!selector.match(/^[a-z0-9#.][a-z0-9#._-]*$/i)) {
      continue; // 복합 선택자는 건너뜀
    }

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
 * 태그명과 class 속성에 기반해 CSS 속성을 얻는다.
 *
 * - element 선택자 적용
 * - class 선택자 적용 (element 선택자를 덮는다)
 */
export function getCSSPropertiesForElement(
  tagName: string,
  classAttr: string | undefined,
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

  return result;
}
