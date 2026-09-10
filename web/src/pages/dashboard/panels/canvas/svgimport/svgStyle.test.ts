// 스타일 추출 시험 (SPEC-CANVAS-007 M4 · AC-08).
//
// **E8 이 이 파일의 고정 입력을 정한다.** 스타일이 없거나 씨앗 색(`#3b82f6`)과 같은 도형은
// "읽었는가" 를 구분할 수 없게 만들고, `fill-opacity` 와 `stroke-opacity` 가 **같은 값**인
// 고정 입력은 "셋을 곱해 하나로 뭉갠" 결함을 그대로 통과시킨다. 그래서 고정 입력은
// 씨앗과 다른 색(`#c0392b` · `#145a32`)에 **서로 다른 알파**(0.6 · 0.9)를 든다.
//
// **확인한 뮤테이션(E12)**
//   1. 알파 접기를 `style.opacity` 곱셈으로 바꾸면 → "두 알파가 서로 다르게 살아남는다" 가
//      빨개진다.
//   2. `INHERITED_STYLE_PROPS` 에 `opacity` 를 넣으면 → "opacity 는 상속되지 않는다" 가
//      빨개진다.
//   3. `collectStyleAtoms` 의 합치는 순서를 뒤집으면(표현 속성이 이김) → "style 속성이
//      이긴다" 가 빨개진다.
//   4. `normalizePaint` 가 모르는 표기를 색으로 통과시키면 → "currentColor 는 씨앗으로
//      떨어진다" 가 빨개진다.
//   5. `isHidden` 에서 `visibility` 를 빼면 → "감춘 도형" 이 빨개진다.
//   6. `evenOdd` 보고를 "성공하면 생략" 으로 바꾸면 → "언제나 근사로 보고한다" 가 빨개진다.
//   7. 선이 있을 때 `strokeWidth` 를 비우면 → "선이 있으면 두께를 반드시 적는다" 가
//      빨개진다.

import fs from 'node:fs';
import path from 'node:path';

import { describe, expect, it } from 'vitest';

import { SEED_COLOR } from '../canvasElementFactory';

import {
  collectStyleAtoms,
  foldAlphaIntoColor,
  INHERITED_STYLE_PROPS,
  inheritStyleAtoms,
  isHidden,
  normalizePaint,
  parseOpacity,
  parseStyleAttribute,
  resolveStyle,
} from './svgStyle';

/** E8 의 고정 입력 — 씨앗과 다른 색, 서로 다른 알파. */
const FIXTURE = {
  fill: '#c0392b',
  'fill-opacity': '0.6',
  stroke: '#145a32',
  'stroke-opacity': '0.9',
  'stroke-width': '3',
} as const;

function reasons(notes: readonly { reason: string }[]): string[] {
  return notes.map((n) => n.reason);
}

describe('style 속성 파싱 (뮤테이션 3)', () => {
  it('이름과 값을 첫 콜론에서만 자른다 — 값 안의 콜론이 살아야 한다', () => {
    expect(parseStyleAttribute('fill:url(#a);stroke:red')).toEqual({
      fill: 'url(#a)',
      stroke: 'red',
    });
  });

  it('빈 조각과 콜론 없는 조각은 버린다', () => {
    expect(parseStyleAttribute('fill:red;;banana;stroke:')).toEqual({ fill: 'red' });
    expect(parseStyleAttribute(undefined)).toEqual({});
  });

  it('style 속성이 표현 속성을 이긴다 (뮤테이션 3)', () => {
    // 뒤집히면 `fill="red" style="fill:blue"` 인 도형이 빨갛게 나온다.
    const atoms = collectStyleAtoms({ fill: 'red', style: 'fill:blue' });
    expect(atoms['fill']).toBe('blue');
  });

  it('빈 문자열 속성은 말하지 않은 것으로 읽는다', () => {
    expect(collectStyleAtoms({ fill: '   ' })).toEqual({});
  });
});

describe('상속 — opacity 는 상속되지 않는다 (뮤테이션 2)', () => {
  it('사양이 상속으로 정한 여섯만 내려온다', () => {
    expect([...INHERITED_STYLE_PROPS]).toEqual([
      'fill',
      'stroke',
      'stroke-width',
      'fill-rule',
      'fill-opacity',
      'stroke-opacity',
    ]);
  });

  it('중첩 g 의 fill 은 자식에게 내려오고 opacity 는 내려오지 않는다', () => {
    const parent = { fill: '#c0392b', opacity: '0.5', filter: 'url(#f)' };
    const child = inheritStyleAtoms(parent, {});
    expect(child['fill']).toBe('#c0392b');
    expect(child['opacity']).toBeUndefined();
    // 상속되지 않는 것은 `opacity` 만이 아니다 — `filter` 도 조상에 머문다.
    expect(child['filter']).toBeUndefined();
  });

  it('자식이 말한 것이 물려받은 것을 덮는다', () => {
    expect(inheritStyleAtoms({ fill: 'red' }, { fill: 'blue' })['fill']).toBe('blue');
  });
});

describe('칠 갈래 나누기 (뮤테이션 4)', () => {
  it('none 과 transparent 는 칠하지 않는다', () => {
    expect(normalizePaint('none')).toEqual({ kind: 'none' });
    expect(normalizePaint(' TRANSPARENT ')).toEqual({ kind: 'none' });
  });

  it('url(#id) 는 참조다 — 따옴표가 있어도 같다', () => {
    expect(normalizePaint('url(#grad)')).toEqual({ kind: 'ref', id: 'grad' });
    expect(normalizePaint("url('#grad')")).toEqual({ kind: 'ref', id: 'grad' });
  });

  it('hex · rgb() · 이름 색은 색이다', () => {
    expect(normalizePaint('#c0392b')).toEqual({ kind: 'color', value: '#c0392b' });
    expect(normalizePaint('rgb(1,2,3)')).toEqual({ kind: 'color', value: 'rgb(1,2,3)' });
    expect(normalizePaint('rebeccapurple')).toEqual({ kind: 'color', value: 'rebeccapurple' });
  });

  it('모르는 표기를 색으로 통과시키지 않는다 (뮤테이션 4)', () => {
    // `ctx.fillStyle = '알 수 없는 것'` 은 예외를 내지 않고 **대입이 무시되어 직전 색이
    // 그대로 쓰인다** — 도형이 엉뚱한 색으로 조용히 그려지는 이 층 최악의 실패 형상이다.
    expect(normalizePaint('currentColor')).toEqual({ kind: 'unsupported' });
    expect(normalizePaint('inherit')).toEqual({ kind: 'unsupported' });
    expect(normalizePaint('#12345')).toEqual({ kind: 'unsupported' });
    expect(normalizePaint('')).toEqual({ kind: 'unsupported' });
  });
});

describe('알파를 색에 접는다 (뮤테이션 1)', () => {
  it('알파가 1 이면 어떤 표기든 그대로 지나간다', () => {
    expect(foldAlphaIntoColor('rebeccapurple', 1)).toEqual({ value: 'rebeccapurple', exact: true });
  });

  it('6자리 hex 에 정확히 접힌다', () => {
    expect(foldAlphaIntoColor('#c0392b', 0.6)).toEqual({ value: 'rgba(192, 57, 43, 0.6)', exact: true });
  });

  it('3자리 hex 를 두 배로 펴서 접는다', () => {
    expect(foldAlphaIntoColor('#f00', 0.5)).toEqual({ value: 'rgba(255, 0, 0, 0.5)', exact: true });
  });

  it('8자리 hex 의 기존 알파와 곱해진다', () => {
    expect(foldAlphaIntoColor('#ff000080', 0.5)).toEqual({ value: 'rgba(255, 0, 0, 0.251)', exact: true });
  });

  it('rgb() 와 rgba() 에 접힌다 — 퍼센트 성분도 읽는다', () => {
    expect(foldAlphaIntoColor('rgb(10, 20, 30)', 0.25)).toEqual({
      value: 'rgba(10, 20, 30, 0.25)',
      exact: true,
    });
    expect(foldAlphaIntoColor('rgba(10,20,30,0.5)', 0.5)).toEqual({
      value: 'rgba(10, 20, 30, 0.25)',
      exact: true,
    });
    expect(foldAlphaIntoColor('rgb(100%, 0%, 0%)', 1)).toEqual({ value: 'rgb(100%, 0%, 0%)', exact: true });
    expect(foldAlphaIntoColor('rgb(100%, 0%, 0%)', 0.4)).toEqual({
      value: 'rgba(255, 0, 0, 0.4)',
      exact: true,
    });
  });

  it('이름 색에는 접을 수 없다 — 알파를 잃고 그 사실을 값으로 말한다', () => {
    // 이름→RGB 표(148 항목)를 들이지 않는 대가다. 감추지 않고 보고한다.
    expect(foldAlphaIntoColor('rebeccapurple', 0.5)).toEqual({
      value: 'rebeccapurple',
      exact: false,
    });
  });
});

describe('불투명도 읽기', () => {
  it('0..1 로 죈다', () => {
    expect(parseOpacity('0.6')).toBe(0.6);
    expect(parseOpacity('-1')).toBe(0);
    expect(parseOpacity('2')).toBe(1);
    expect(parseOpacity('50%')).toBe(0.5);
  });

  it('읽을 수 없으면 지정하지 않은 것이다', () => {
    expect(parseOpacity('banana')).toBeUndefined();
    expect(parseOpacity(undefined)).toBeUndefined();
  });
});

describe('감춤 — 만들지 않고 보고한다 (뮤테이션 5)', () => {
  it('display:none 과 visibility:hidden 을 둘 다 본다', () => {
    expect(isHidden({ display: 'none' })).toBe(true);
    expect(isHidden({ visibility: 'hidden' })).toBe(true);
    expect(isHidden({ display: 'inline', visibility: 'visible' })).toBe(false);
    expect(isHidden({})).toBe(false);
  });
});

describe('AC-08 — 아홉 축에 다섯이 앉는다', () => {
  it('두 알파가 서로 다르게 살아남는다 — 하나로 뭉개지지 않았다 [E8] (뮤테이션 1)', () => {
    const { style } = resolveStyle(FIXTURE);
    expect(style.fill).toBe('rgba(192, 57, 43, 0.6)');
    expect(style.stroke).toBe('rgba(20, 90, 50, 0.9)');
    // 같은 값이었다면 "셋을 곱해 뭉갠" 결함이 그대로 통과했을 것이다.
    expect(style.fill).not.toBe(style.stroke);
  });

  it('style.opacity 는 요소 수준 opacity 그대로다', () => {
    expect(resolveStyle({ ...FIXTURE, opacity: '0.4' }).style.opacity).toBe(0.4);
    // 채움·선 알파는 색에 접혔으므로 opacity 축을 건드리지 않는다.
    expect(resolveStyle(FIXTURE).style.opacity).toBeUndefined();
  });

  it('선이 있으면 두께를 반드시 적고 배율이 곱해진다 (뮤테이션 7)', () => {
    // SVG 의 기본 두께는 1 **사용자 단위**이고 렌더측 기본은 1 **캔버스 px** 이라,
    // 비워 두면 축척만큼 어긋난다.
    expect(resolveStyle(FIXTURE, { strokeScale: 2 }).style.strokeWidth).toBe(6);
    expect(resolveStyle({ stroke: 'red' }, { strokeScale: 0.5 }).style.strokeWidth).toBe(0.5);
  });

  it('선이 없으면 두께도 없다 — 미지정 필드를 만들어 채우지 않는다', () => {
    const { style } = resolveStyle({ fill: 'red' });
    expect(style.strokeWidth).toBeUndefined();
    expect(style.stroke).toBeUndefined();
  });

  it('fill="none" 은 채우지 않는다', () => {
    expect(resolveStyle({ fill: 'none', stroke: 'red' }).style.fill).toBeUndefined();
  });

  it('글자 축 넷은 한 번도 대입되지 않는다(형상 판정)', () => {
    const { style } = resolveStyle({ ...FIXTURE, 'font-size': '20', 'text-anchor': 'middle' });
    expect(style.fontSize).toBeUndefined();
    expect(style.fontWeight).toBeUndefined();
    expect(style.textColor).toBeUndefined();
    expect(style.align).toBeUndefined();

    const source = fs.readFileSync(path.join(__dirname, 'svgStyle.ts'), 'utf8');
    for (const axis of ['fontSize', 'fontWeight', 'textColor', 'align']) {
      expect(source).not.toContain(`style.${axis} =`);
    }
  });
});

describe('그라디언트와 모르는 색은 근사로 보고된다', () => {
  it('url(#g) 는 참조된 첫 stop 색이 되고 근사로 오른다', () => {
    const resolved = resolveStyle(
      { fill: 'url(#g)' },
      { resolvePaintRef: (id) => (id === 'g' ? '#8e44ad' : undefined) },
    );
    expect(resolved.style.fill).toBe('#8e44ad');
    expect(reasons(resolved.notes)).toEqual(['gradientToSolid']);
  });

  it('못 읽는 참조는 씨앗 색이 되고 그 사실이 따로 보고된다', () => {
    const resolved = resolveStyle({ fill: 'url(#missing)' }, { fallbackColor: SEED_COLOR });
    expect(resolved.style.fill).toBe(SEED_COLOR);
    expect(reasons(resolved.notes)).toEqual(['paintUnresolved']);
  });

  it('currentColor 는 씨앗으로 떨어지고 근사로 오른다 (뮤테이션 4)', () => {
    const resolved = resolveStyle({ fill: 'currentColor' }, { fallbackColor: SEED_COLOR });
    expect(resolved.style.fill).toBe(SEED_COLOR);
    expect(reasons(resolved.notes)).toEqual(['paintUnresolved']);
  });

  it('접을 수 없는 알파가 보고된다', () => {
    const resolved = resolveStyle({ fill: 'rebeccapurple', 'fill-opacity': '0.5' });
    expect(resolved.style.fill).toBe('rebeccapurple');
    expect(reasons(resolved.notes)).toEqual(['opacityUnfoldable']);
  });

  it('비균등 배율의 선 두께가 근사로 오른다', () => {
    const resolved = resolveStyle({ stroke: 'red' }, { strokeScale: 1, nonUniformStroke: true });
    expect(reasons(resolved.notes)).toEqual(['nonUniformStrokeScale']);
  });

  it('그룹 opacity 는 자식마다 곱해지고 근사로 오른다', () => {
    const resolved = resolveStyle({ fill: 'red', opacity: '0.5' }, { groupOpacity: 0.5 });
    expect(resolved.style.opacity).toBe(0.25);
    expect(reasons(resolved.notes)).toEqual(['groupOpacityPerChild']);
  });
});

describe('evenodd 는 언제나 근사로 보고된다 (뮤테이션 6 · 위험 R9)', () => {
  it('fill-rule="evenodd" 가 값과 보고 둘 다를 낸다', () => {
    const resolved = resolveStyle({ fill: 'red', 'fill-rule': 'evenodd' });
    expect(resolved.evenOdd).toBe(true);
    expect(reasons(resolved.notes)).toEqual(['evenOddWinding']);
  });

  it('nonzero 는 값도 보고도 없다', () => {
    const resolved = resolveStyle({ fill: 'red', 'fill-rule': 'nonzero' });
    expect(resolved.evenOdd).toBe(false);
    expect(resolved.notes).toEqual([]);
  });

  it('상속으로 내려온 fill-rule 도 읽힌다', () => {
    const atoms = inheritStyleAtoms({ 'fill-rule': 'evenodd' }, { fill: 'red' });
    expect(resolveStyle(atoms).evenOdd).toBe(true);
  });
});

describe('떨어지는 것들 (REQ-05)', () => {
  it('dasharray · filter · clip-path · mask 가 버림으로 오른다', () => {
    const resolved = resolveStyle({
      fill: 'red',
      'stroke-dasharray': '4 2',
      filter: 'url(#blur)',
      'clip-path': 'url(#c)',
      mask: 'url(#m)',
    });
    expect(reasons(resolved.notes)).toEqual([
      'dashArrayDropped',
      'filterDropped',
      'clipPathDropped',
      'maskDropped',
    ]);
    expect(resolved.notes.every((n) => n.kind === 'dropped')).toBe(true);
  });

  it('none 인 filter 는 그림에 영향을 주지 않으므로 보고하지 않는다', () => {
    // 보고에는 **그림에 실제로 영향을 준 것만** 오른다 — 잡음이 섞인 보고는 읽히지 않고,
    // 읽히지 않는 보고는 침묵과 같다(위험 R7).
    expect(resolveStyle({ fill: 'red', filter: 'none', 'stroke-dasharray': 'none' }).notes).toEqual([]);
  });

  it('linecap · linejoin · miterlimit 는 보고하지 않는다 — 두께 2px 에서 눈에 띄지 않는다', () => {
    const resolved = resolveStyle({
      stroke: 'red',
      'stroke-linecap': 'round',
      'stroke-linejoin': 'bevel',
      'stroke-miterlimit': '2',
    });
    expect(resolved.notes).toEqual([]);
  });
});

describe('씨앗 판정 (REQ-06)', () => {
  it('SVG 가 칠을 말하지 않으면 hasOwnStyle 이 거짓이다 — pathSeedStyle 이 선다', () => {
    expect(resolveStyle({}).hasOwnStyle).toBe(false);
    expect(resolveStyle({ opacity: '0.5' }).hasOwnStyle).toBe(false);
    expect(resolveStyle({ fill: 'none' }).hasOwnStyle).toBe(true);
    expect(resolveStyle({ stroke: 'red' }).hasOwnStyle).toBe(true);
  });
});

describe('견고성 (REQ-07)', () => {
  it('손상된 값에 예외가 없다', () => {
    const corrupt: Record<string, string>[] = [
      { fill: 'rgb(' },
      { fill: '#' },
      { 'fill-opacity': 'banana', fill: '#fff' },
      { 'stroke-width': '1e999', stroke: 'red' },
      { style: ':::' },
      { fill: 'rgb(1,2)' },
    ];
    for (const atoms of corrupt) {
      expect(() => resolveStyle(collectStyleAtoms(atoms))).not.toThrow();
    }
  });

  it('비유한 두께는 SVG 기본 1 로 떨어진다', () => {
    expect(resolveStyle({ stroke: 'red', 'stroke-width': '1e999' }).style.strokeWidth).toBe(1);
  });

  it('음수 두께는 0 이다 — 선 없음을 뜻한다', () => {
    expect(resolveStyle({ stroke: 'red', 'stroke-width': '-4' }).style.strokeWidth).toBe(0);
  });
});

// --- SVG 의 `fill` 초기값 (M11 · 결정 1) -----------------------------------

describe('`stroke` 만 말한 도형은 **검게 채워진다** — SVG 초기값 (M11 결정 1)', () => {
  it('`<path stroke="red"/>` 가 검은 채움을 든다 — 브라우저가 그리는 그 그림이다', () => {
    // SVG 1.1 §11.3: `fill` 의 초기값은 `black` 이다. 그래서 `fill` 을 말하지 않고 `stroke`
    // 만 말한 도형은 브라우저에서 **검은 속에 테를 두른** 모습으로 그려진다. 칠하지 않으면
    // 우리 화면만 원본과 달라지고, 그 어긋남은 미리보기에서 사용자가 제 눈 탓으로 돌린다.
    const { style } = resolveStyle({ stroke: 'red' });
    expect(style.fill).toBe('#000000');
    expect(style.stroke).toBe('red');
  });

  it('`fill-opacity` 도 그 초기값에 접힌다 — 반쯤 비치는 검정이다', () => {
    expect(resolveStyle({ stroke: 'red', 'fill-opacity': '0.5' }).style.fill).toBe(
      'rgba(0, 0, 0, 0.5)',
    );
  });

  it('**보고에 오르지 않는다** — 근사가 아니라 사양대로의 값이다', () => {
    // 옳게 그린 것을 근사로 말하면 사용자가 미리보기의 정확함을 의심하게 되고, 게다가
    // 사용자가 **적은 적 없는** 칠에 대한 알림이라 잡음이 된다(위험 R7).
    expect(resolveStyle({ stroke: 'red' }).notes).toEqual([]);
    expect(resolveStyle({ stroke: 'red', 'stroke-width': '3' }).notes).toEqual([]);
  });

  it('`fill="none"` 을 **명시**하면 그대로 채우지 않는다 — 초기값이 그것을 덮지 않는다', () => {
    expect(resolveStyle({ fill: 'none', stroke: 'red' }).style.fill).toBeUndefined();
  });

  it('상속받은 `fill` 이 있으면 그것이 이긴다 — 초기값이 조상을 덮지 않는다', () => {
    const inherited = inheritStyleAtoms({ fill: '#c0392b' }, { stroke: 'red' });
    expect(resolveStyle(inherited).style.fill).toBe('#c0392b');
  });

  it('**아무 칠도 말하지 않은 도형에는 걸리지 않는다** — 거기서는 `pathSeedStyle` 이 선다', () => {
    // SPEC 이 사양과 의도적으로 갈라선 자리다(REQ-06): 가져온 도형이 카탈로그 도형과 같은
    // 씨앗 색을 입어야 목록에서 구별되지 않는다. 이 규칙이 그 결정을 삼키면 안 된다.
    const bare = resolveStyle({});
    expect(bare.hasOwnStyle).toBe(false);
    expect(bare.style.fill).toBeUndefined();
    // `stroke` 아닌 축만 말한 경우도 같다 — `hasOwnStyle` 의 정의가 칠 두 축이기 때문이다.
    expect(resolveStyle({ opacity: '0.5' }).style.fill).toBeUndefined();
    expect(resolveStyle({ 'stroke-width': '3' }).style.fill).toBeUndefined();
  });

  it('`stroke` 의 초기값은 `none` 이라 반대 방향은 일어나지 않는다', () => {
    // 사양이 두 축에 다른 초기값을 주므로 대칭이 아니다. 대칭으로 만들면 채움만 말한
    // 도형에 있지도 않은 테가 생긴다.
    const { style } = resolveStyle({ fill: '#c0392b' });
    expect(style.stroke).toBeUndefined();
    expect(style.strokeWidth).toBeUndefined();
  });
});
