// SVG style CSS 가져오기 시험 (SPEC-CANVAS-007, defect A).
//
// 색이 <style> 스타일시트에 정의된 SVG 는 인라인 속성이 없어서 hasOwnStyle 이 false 가
// 되고, 그 결과 씨앗 색(파랑)이 입혀진다. 이 시험은 그 결함을 재고, 수정하면 CSS 규칙에서
// 색을 꺼낼 수 있음을 보인다.

import { describe, expect, it } from 'vitest';

import { SEED_COLOR } from '../canvasElementFactory';
import { collectStyleAtoms, resolveStyle } from './svgStyle';

describe('SVG <style> 색 가져오기 (defect A)', () => {
  it('CSS 클래스 선택자로 정의된 색은 현재 CSS 파싱 없이 스킵된다 — hasOwnStyle === false', () => {
    // <style>.red { fill: #c0392b; } ... <rect class="red" ... />
    // 인라인 속성이 없으므로 hasOwnStyle 이 false 가 되어야 한다.
    const attrs = { class: 'red' };
    const atoms = collectStyleAtoms(attrs);

    // class 속성은 수집되지 않는다 — CSS 파싱이 없기 때문이다.
    expect(atoms['fill']).toBeUndefined();
    expect(atoms['stroke']).toBeUndefined();

    const resolved = resolveStyle(atoms, {
      resolvePaintRef: () => undefined,
      strokeScale: 1,
      groupOpacity: 1,
      nonUniformStroke: false,
      fallbackColor: SEED_COLOR,
    });

    // hasOwnStyle === false 이므로 호출자가 씨앗 색을 쓸 것이다.
    expect(resolved.hasOwnStyle).toBe(false);
  });

  it('인라인 fill 속성이 있으면 hasOwnStyle === true', () => {
    // <rect fill="#c0392b" ...
    const attrs = { fill: '#c0392b' };
    const atoms = collectStyleAtoms(attrs);

    expect(atoms['fill']).toBe('#c0392b');

    const resolved = resolveStyle(atoms, {
      resolvePaintRef: () => undefined,
      strokeScale: 1,
      groupOpacity: 1,
      nonUniformStroke: false,
      fallbackColor: SEED_COLOR,
    });

    // 인라인 fill 이 있으므로 hasOwnStyle === true.
    expect(resolved.hasOwnStyle).toBe(true);
    expect(resolved.style.fill).toBe('#c0392b');
  });

  it('CSS 클래스 선택자로 정의된 도형은 씨앗 색으로 렌더된다 (현재 동작)', () => {
    // <style>.red { fill: #c0392b; } ... <rect class="red" .../>
    // 색이 CSS 에만 있고 인라인이 없다.
    const attrs = { class: 'red' };
    const atoms = collectStyleAtoms(attrs);

    // CSS 를 파싱하지 않으므로 atoms 는 비었다.
    expect(Object.keys(atoms).length).toBe(0);

    // 빈 atoms 로 resolveStyle 을 호출하면 hasOwnStyle === false
    const resolved = resolveStyle(atoms, {
      resolvePaintRef: () => undefined,
      strokeScale: 1,
      groupOpacity: 1,
      nonUniformStroke: false,
      fallbackColor: SEED_COLOR,
    });

    expect(resolved.hasOwnStyle).toBe(false);
    // fill 이 없으면 결과에도 fill 이 없다.
    expect(resolved.style.fill).toBeUndefined();

    // canvasElementFactory.ts:386 에서:
    // style: shape.hasOwnStyle ? { ...shape.style } : { ...pathSeedStyle(path), ...shape.style }
    // hasOwnStyle === false 이므로 pathSeedStyle 이 쓰인다. 즉, 씨앗 색(파랑)이 입는다.
  });
});
