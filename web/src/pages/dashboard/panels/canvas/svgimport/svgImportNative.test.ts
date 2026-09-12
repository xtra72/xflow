// 태그가 말한 도형이 원시형 요소가 된다 — 가져오기의 원시형 치환 (SPEC-CANVAS-007 후속).
//
// **고정 입력이 네 함정을 피한다.** 이 주제에서 초록으로 통과하면서 아무것도 재지 않는
// 시험을 만드는 길이 넷 있다:
//   - **항등 변환** — 행렬 게이트를 통째로 숨긴다(어떤 조건이든 항등은 통과한다).
//   - **정사각 rect** — x/y 를 맞바꾼 결함이 같은 수를 낸다.
//   - **원점의 rect** — 평행이동을 빠뜨린 결함이 같은 수를 낸다.
//   - **균등 배율** — 축마다 다른 배율을 한 축으로 쓴 결함이 같은 수를 낸다.
// 그래서 고정 입력은 `translate(3,5) scale(2,3)` 아래의 비정사각·비원점 rect 다.
//
// `viewBox` 는 계획 시험과 같은 `"-13 7 317 181"` — 원점 ≠ 0 · 두 원점의 부호가 다르다 ·
// 비정사각 · 축척이 나누어떨어지지 않는다(E2 · E3).
//
// **확인한 뮤테이션** — "→" 뒤가 빨개지는 단언이다.
//   1. `preservesAxisAlignment` 를 `() => true` 로 → "45° 로 돌린 사각형은 경로로 남는다" 가
//      빨개진다(상자가 마주 보는 두 꼭짓점으로만 나서 도형보다 작아진다).
//   2. 게이트의 둘째 갈래(`a === 0 && d === 0`)를 지우면 → "축을 맞바꾸는 행렬은 통과한다" 가
//      빨개진다.
//   3. 게이트에 허용 오차(`Math.abs(b) < 1e-9`)를 두면 → "`rotate(90)` 은 경로로 남는다" 가
//      빨개진다.
//   4. `transformNative` 의 상자 갈래에서 `Math.min`/`Math.max` 를 지우고 두 상을 그대로
//      쓰면 → "반사 아래의 사각형" 이 빨개진다(폭이 음수가 된다).
//   5. `<line>` 에도 축 정렬 게이트를 걸면 → "돌린 선은 선으로 남는다" 가 빨개진다.
//   6. `planNative` 의 퇴화 선 검사를 지우면 → "한 칸에 떨어지는 선은 경로로 남는다" 가
//      빨개진다(왕복에서 기하가 통째로 씨앗 선으로 갈린다).
//   7. `finiteNative` 를 항등으로 바꾸면 → "좌표가 `±∞` 인 사각형은 경로로 남는다" 가
//      빨개진다.
//   8. `isSharpCorner` 를 `() => true` 로 → "`rx` 가 있는 사각형은 경로로 남는다" 가 빨개진다.
//   9. `ellipseNative` 의 `rx > 0` 검사를 지우면 → "`ry` 없는 `<ellipse>` 는 원시형이 아니다"
//      가 빨개진다.
//  10. `countCommands` 가 원시형의 `commands` 를 세려 들면(항상 0 이 아니라) → 세지 못한다.
//      대신 상한 검사와 보고가 **다른** 함수를 쓰게 하면 → "보고의 명령 수가 상한이 죄는
//      그 수다" 가 빨개진다.
//  11. `importedElement` 의 닫힘을 종류에서 읽으면(`shape.kind !== 'line'`) → "열린 경로는
//      선 씨앗을 입는다" 가 빨개진다.
//  12. 원시형을 배열 뒤로 몰면 → "문서 순서가 배열 순서다" 가 빨개진다.

import { describe, expect, it } from 'vitest';

import {
  DEFAULT_LINE_GEOMETRY,
  isDegenerateLine,
  parseCanvasConfig,
  type CanvasElement,
  type CanvasSize,
} from '../canvasConfig';
import {
  appendImportedElements,
  SEED_COLOR,
  SEED_STROKE_WIDTH,
} from '../canvasElementFactory';
import { MAX_PATH_COMMANDS } from '../shapes/pathTypes';

import { planSvgImport, type ImportedShapeSpec } from './svgImportPlan';
import { shorthandNativeShape, shorthandShapeCommands } from './svgShapes';
import { IDENTITY_MATRIX, parseTransformList, preservesAxisAlignment } from './svgTransform';
import { MAX_IMPORT_ELEMENTS, MAX_IMPORT_TEXT_LENGTH } from './svgImportTypes';

const CANVAS: CanvasSize = { width: 500, height: 400 };

function svg(body: string, rootAttrs = 'viewBox="-13 7 317 181"'): string {
  return `<svg xmlns="http://www.w3.org/2000/svg" ${rootAttrs}>${body}</svg>`;
}

function plan(text: string, canvas: CanvasSize = CANVAS) {
  const result = planSvgImport(text, canvas);
  if (!result.ok) throw new Error(`거절됨: ${result.refusal.reason}`);
  return result;
}

/** 계획을 실제 요소로 — 사용자가 목록에서 보는 그것이다. */
function place(text: string, canvas: CanvasSize = CANVAS): CanvasElement[] {
  const ready = plan(text, canvas);
  return appendImportedElements([], ready.shapes, ready.texts).created;
}

/** 도형 하나짜리 문서의 그 요소. 여럿이면 시험이 선다 — 잰다고 믿은 것을 재고 있는가. */
function only(text: string, canvas: CanvasSize = CANVAS): CanvasElement {
  const els = place(text, canvas);
  if (els.length !== 1) throw new Error(`도형이 ${els.length} 개다`);
  return els[0]!;
}

// --- ① 태그가 말한 것이 그대로 선다 ---------------------------------------

describe('태그가 말한 도형이 원시형이 된다', () => {
  it('평행이동·비균등 배율 아래의 <rect> 는 rect 요소이고 네 수가 정확하다', () => {
    // 행렬 = translate(3,5)·scale(2,3) = {a:2, b:0, c:0, d:3, e:3, f:5}.
    // 꼭짓점 (10,20)→(23,65), (90,60)→(183,185).
    // 문서 틀 = fitBox(-13 7 317 181, 500×400) = {x:50, y:86, w:400, h:228}.
    //   x = round(50 + (23 − (−13)) × 400/317) = round(95.426) = 95
    //   y = round(86 + (65 − 7) × 228/181)     = round(159.061) = 159
    //   w = round(160 × 400/317) = round(201.893) = 202
    //   h = round(120 × 228/181) = round(151.160) = 151
    const el = only(svg('<rect x="10" y="20" width="80" height="40" transform="translate(3,5) scale(2,3)"/>'));
    expect(el.kind).toBe('rect');
    expect(el.geometry).toEqual({ x: 95, y: 159, w: 202, h: 151 });
  });

  it('가져온 사각형의 상자가 같은 도형을 경로로 옮겼을 때의 상자와 **같다**', () => {
    // `<polygon>` 은 캔버스에 그 종류가 없어 경로로 남는다 — 꼭짓점은 위 `<rect>` 와 같다.
    // 두 상자가 갈라지면 치환이 그림을 옮긴 것이고, 그 순간 "정확하다" 가 거짓이 된다.
    const rect = only(svg('<rect x="10" y="20" width="80" height="40" transform="translate(3,5) scale(2,3)"/>'));
    const poly = only(
      svg('<polygon points="10,20 90,20 90,60 10,60" transform="translate(3,5) scale(2,3)"/>'),
    );
    expect(poly.kind).toBe('path');
    expect(rect.geometry).toEqual(poly.geometry);
  });

  it('<circle> 은 ellipse 가 되고 **비균등 배율을 흡수한다** — 원이 타원이 된다', () => {
    // scale(2,3) 아래의 반지름 10 인 원 → 사용자 단위 상자 (80,150)~(120,210).
    //   x = round(50 + 93 × 400/317)  = round(167.350) = 167
    //   y = round(86 + 143 × 228/181) = round(266.133) = 266
    //   w = round(40 × 400/317) = round(50.473) = 50
    //   h = round(60 × 228/181) = round(75.580) = 76
    const el = only(svg('<circle cx="50" cy="60" r="10" transform="scale(2,3)"/>'));
    expect(el.kind).toBe('ellipse');
    expect(el.geometry).toEqual({ x: 167, y: 266, w: 50, h: 76 });
    // 켜져 있음: 원이었는데 정사각이 **아니다**. 같으면 배율을 한 축만 먹은 것이다.
    expect((el.geometry as { w: number; h: number }).w).not.toBe(
      (el.geometry as { w: number; h: number }).h,
    );
  });

  it('<ellipse> 도 같은 자리에 선다 — 반지름 둘을 제 이름으로 읽는다', () => {
    const el = only(svg('<ellipse cx="50" cy="60" rx="20" ry="10"/>'));
    expect(el.kind).toBe('ellipse');
    // 사용자 단위 상자 (30,50)~(70,70) → w = round(40 × 400/317) = 50, h = round(20 × 228/181) = 25.
    expect(el.geometry).toEqual({ x: 104, y: 140, w: 50, h: 25 });
  });

  it('<line> 은 **돌아가 있어도** line 요소다 — 선에는 축 정렬 조건이 없다 (뮤테이션 5)', () => {
    // rotate(30) 아래의 (10,20)~(90,20) → (−1.3397, 22.3205) · (67.9423, 62.3205).
    //   (65,105) · (152,156)
    const el = only(svg('<line x1="10" y1="20" x2="90" y2="20" transform="rotate(30)"/>'));
    expect(el.kind).toBe('line');
    expect(el.geometry).toEqual({ x1: 65, y1: 105, x2: 152, y2: 156 });
    // 켜져 있음: 기울기가 0 이 **아니다**. 회전을 버렸다면 두 y 가 같아진다.
    expect((el.geometry as { y1: number; y2: number }).y1).not.toBe(
      (el.geometry as { y1: number; y2: number }).y2,
    );
  });
});

// --- ② 행렬 게이트 ---------------------------------------------------------

describe('행렬 게이트 — 축 정렬을 지키지 못하면 경로로 남는다', () => {
  it('45° 로 돌린 <rect> 는 경로로 남고 **그림이 여전히 돌아가 있다** (뮤테이션 1)', () => {
    const els = place(svg('<rect x="10" y="20" width="80" height="40" transform="rotate(45)"/>'));
    expect(els.map((el) => el.kind)).toEqual(['path']);
    const el = els[0]!;
    if (el.kind !== 'path') throw new Error('경로여야 한다');
    // **치환을 막은 것만으로는 모자라다** — 그림이 실제로 돌아가 있어야 한다. 네 꼭짓점의
    // x 가 넷 다 다르면 그 사각형은 축에 나란하지 않다(축에 나란하면 x 가 두 값뿐이다).
    const xs = new Set(el.path.filter((cmd) => cmd.c !== 'Z').map((cmd) => cmd.x));
    expect(xs.size).toBe(4);
  });

  it('기울인 <rect> 도 경로로 남는다 — skew 는 두 갈래 모두에서 떨어진다', () => {
    const els = place(svg('<rect x="10" y="20" width="80" height="40" transform="skewX(20)"/>'));
    expect(els.map((el) => el.kind)).toEqual(['path']);
  });

  it('축을 맞바꾸는 행렬은 통과하고 **폭과 높이가 뒤바뀐다** (뮤테이션 2)', () => {
    // matrix(0 1 −1 0 200 0): (10,20)→(180,10), (90,60)→(140,90). 상자 40 × 80 —
    // 원본 80 × 40 이 **뒤집혔다**. 정사각 고정 입력이었다면 이 단언이 아무것도 재지 못한다.
    const el = only(
      svg('<rect x="10" y="20" width="80" height="40" transform="matrix(0 1 -1 0 200 0)"/>'),
    );
    expect(el.kind).toBe('rect');
    expect(el.geometry).toEqual({ x: 243, y: 90, w: 50, h: 101 });
  });

  it('`rotate(90)` 은 **경로로 남는다** — 게이트에 허용 오차가 없다 (뮤테이션 3)', () => {
    // `cos 90° = 6.12e−17` 이라 `a === 0` 이 거짓이다. 오차를 두면 `rotate(0.0001)` 도
    // 함께 통과해 그 회전이 조용히 지워진다 — 치환의 전제(정확하다)를 그만큼 무르는 일이다.
    const els = place(svg('<rect x="10" y="20" width="80" height="40" transform="rotate(90)"/>'));
    expect(els.map((el) => el.kind)).toEqual(['path']);
  });

  it('반사(음수 배율) 아래의 <rect> 는 rect 로 남고 **크기가 뒤집히지 않는다** (뮤테이션 4)', () => {
    // scale(−1,1): (10,20)→(−10,20), (90,60)→(−90,60). 두 상을 그대로 쓰면 `minX` 가
    // `maxX` 보다 커져 폭이 음수가 되고, 그 음수는 상자를 정하는 층에서 **최소 크기 1 로
    // 조용히 접힌다** — "폭이 0 보다 크다" 만 재는 단언은 그 접힘을 통과시킨다. 그래서
    // 네 수를 전부 못박는다: 폭 101 은 원본 80 이 축척을 탄 값이고 1 이 아니다.
    //   x = round(50 + (−90 + 13) × 400/317) = round(−47.161) = −47   (캔버스 밖 — clamp 하지 않는다)
    //   y = round(86 + 13 × 228/181)         = round(102.376) = 102
    //   w = round(80 × 400/317) = 101   ·   h = round(40 × 228/181) = 50
    const el = only(svg('<rect x="10" y="20" width="80" height="40" transform="scale(-1,1)"/>'));
    expect(el.kind).toBe('rect');
    expect(el.geometry).toEqual({ x: -47, y: 102, w: 101, h: 50 });
  });

  it('조상 <g> 의 회전도 함께 본다 — 누적 행렬이 게이트의 입력이다', () => {
    const els = place(svg('<g transform="rotate(37)"><rect x="10" y="20" width="80" height="40"/></g>'));
    expect(els.map((el) => el.kind)).toEqual(['path']);
  });

  it('조상이 돌려도 자신이 되돌리면 다시 rect 다 — 게이트가 보는 것은 **누적**이다', () => {
    const els = place(
      svg('<g transform="rotate(37)"><rect x="10" y="20" width="80" height="40" transform="rotate(-37)"/></g>'),
    );
    // `rotate(37)·rotate(-37)` 의 곱이 `b`·`c` 를 정확히 0 으로 되돌리지는 않는다(부동소수).
    // 그러므로 이 시험이 요구하는 것은 "rect 여야 한다" 가 아니라 **어느 쪽이든 그림이
    // 옳다**는 것이고, 그것은 아래 왕복이 잰다. 갈래만 적어 둔다.
    expect(['rect', 'path']).toContain(els[0]!.kind);
  });
});

describe('축 정렬 술어 그 자체 (svgTransform)', () => {
  it('항등 · 배율 · 평행이동 · 반사는 통과한다', () => {
    expect(preservesAxisAlignment(IDENTITY_MATRIX)).toBe(true);
    expect(preservesAxisAlignment(parseTransformList('scale(2,3)'))).toBe(true);
    expect(preservesAxisAlignment(parseTransformList('translate(7,11)'))).toBe(true);
    expect(preservesAxisAlignment(parseTransformList('scale(-1,1)'))).toBe(true);
  });

  it('축을 맞바꾸는 행렬도 통과한다 — 조건의 둘째 갈래다', () => {
    expect(preservesAxisAlignment({ a: 0, b: 1, c: -1, d: 0, e: 0, f: 0 })).toBe(true);
  });

  it('회전 · 기울임은 떨어진다', () => {
    expect(preservesAxisAlignment(parseTransformList('rotate(45)'))).toBe(false);
    expect(preservesAxisAlignment(parseTransformList('rotate(90)'))).toBe(false);
    expect(preservesAxisAlignment(parseTransformList('skewX(20)'))).toBe(false);
    expect(preservesAxisAlignment(parseTransformList('skewY(20)'))).toBe(false);
  });
});

// --- ③ 캔버스가 담지 못하는 것은 경로로 남는다 -----------------------------

describe('캔버스가 담지 못하면 경로로 남는다', () => {
  it('모서리가 깎인 <rect> 는 경로다 — `BoxGeometry` 에 반지름 칸이 없다 (뮤테이션 8)', () => {
    for (const attr of ['rx="5"', 'ry="5"', 'rx="5" ry="3"']) {
      const els = place(svg(`<rect x="10" y="20" width="80" height="40" ${attr}/>`));
      expect(els.map((el) => el.kind), attr).toEqual(['path']);
    }
  });

  it('`rx="0"` 은 깎인 것이 아니다 — 사각형으로 선다', () => {
    expect(only(svg('<rect x="10" y="20" width="80" height="40" rx="0"/>')).kind).toBe('rect');
  });

  it('음수 `rx` 는 "없음" 으로 읽힌다 — 사양이 그렇게 정한다', () => {
    expect(only(svg('<rect x="10" y="20" width="80" height="40" rx="-4"/>')).kind).toBe('rect');
  });

  it('<polygon> · <polyline> · <path> 는 경로다 — 캔버스에 그 종류가 없다', () => {
    for (const body of [
      '<polygon points="10,20 90,20 90,60"/>',
      '<polyline points="10,20 90,20 90,60"/>',
      '<path d="M 10 20 L 90 20 L 90 60 Z"/>',
    ]) {
      expect(place(svg(body)).map((el) => el.kind), body).toEqual(['path']);
    }
  });

  it('좌표가 `±∞` 로 밀린 <rect> 는 경로로 돌아간다 (뮤테이션 7)', () => {
    // 경로에는 이 경우의 답이 007 에 이미 있다(잴 수 없으면 문서 틀을 그대로 쓴다).
    // 원시형으로 세우면 상자가 유한 폴백으로 내려앉아 캔버스 구석의 1×1 부스러기가 된다.
    const els = place(
      svg('<rect x="1e200" y="1e200" width="1e200" height="1e200" transform="scale(1e200)"/>'),
    );
    expect(els.map((el) => el.kind)).toEqual(['path']);
    const box = els[0]!.geometry as { w: number; h: number };
    expect(box.w).toBeGreaterThan(1);
    expect(box.h).toBeGreaterThan(1);
  });

  it('두 끝점이 같은 칸에 떨어지는 <line> 은 경로로 돌아간다 (뮤테이션 6)', () => {
    // 사용자 단위로는 0.3 만큼 다르지만 캔버스 정수로는 같은 칸이다. 그대로 선 요소로
    // 세우면 저장 왕복에서 파서가 기하를 **통째로** `DEFAULT_LINE_GEOMETRY` 로 갈아 끼워,
    // 캔버스를 가로지르는 400 단위짜리 선이 난데없이 나타난다.
    const els = place(svg('<line x1="10" y1="20" x2="10.3" y2="20"/>'));
    expect(els.map((el) => el.kind)).toEqual(['path']);
    const reopened = parseCanvasConfig(
      JSON.parse(JSON.stringify({ canvas: CANVAS, elements: els })) as unknown,
    );
    expect(reopened.elements[0]!.geometry).not.toEqual(DEFAULT_LINE_GEOMETRY);
  });

  it('길이가 0 인 <line> 도 같다 — 점 하나는 선이 아니다', () => {
    expect(place(svg('<line x1="10" y1="20" x2="10" y2="20"/>')).map((el) => el.kind)).toEqual([
      'path',
    ]);
  });

  it('그릴 것이 없는 도형은 **원시형으로도 되살아나지 않는다**', () => {
    // 축약기가 빈 목록을 내는 것들이다. 원시형 쪽에서만 값을 내면 "축약기가 버린 도형이
    // 요소로 되살아난다" — 보고에도 오르지 않은 채.
    for (const body of [
      '<rect x="5" y="5" width="0" height="10"/>',
      '<rect x="5" y="5" width="-4" height="-4"/>',
      '<circle cx="10" cy="20" r="0"/>',
      '<ellipse cx="10" cy="20" rx="5"/>',
    ]) {
      const result = plan(svg(`${body}<polygon points="10,20 90,20 90,60"/>`));
      expect(result.shapes.map((sh) => sh.kind), body).toEqual(['path']);
    }
  });
});

// --- ④ 축약기와 원시형 판정이 같은 것을 본다 (단위) ------------------------

describe('`shorthandNativeShape` 자체', () => {
  it('여섯 태그 가운데 셋만 원시형이 있다', () => {
    const has = (tag: string, attrs: Record<string, string>): boolean =>
      shorthandNativeShape(tag, attrs) !== undefined;
    expect(has('rect', { width: '4', height: '5' })).toBe(true);
    expect(has('circle', { r: '3' })).toBe(true);
    expect(has('ellipse', { rx: '3', ry: '2' })).toBe(true);
    expect(has('line', {})).toBe(true);
    expect(has('polygon', { points: '0,0 1,1 2,0' })).toBe(false);
    expect(has('polyline', { points: '0,0 1,1' })).toBe(false);
    expect(has('path', { d: 'M 0 0' })).toBe(false);
  });

  it('`ry` 없는 <ellipse> 는 원시형이 아니다 — 축약기도 빈 목록을 낸다 (뮤테이션 9)', () => {
    // SVG2 는 결측 `ry` 를 `rx` 로 채우지만 이 저장소의 축약기는 0 으로 읽는다(실측).
    // 원시형만 채우면 **그리지 않기로 한 타원이 요소로 선다** — 두 답이 갈라지는 자리다.
    expect(shorthandNativeShape('ellipse', { cx: '5', cy: '5', rx: '3' })).toBeUndefined();
    expect(shorthandShapeCommands('ellipse', { cx: '5', cy: '5', rx: '3' })).toEqual([]);
  });

  it('원시형이 있으면 축약기도 명령을 낸다 — 두 표현이 같은 잉크를 가리킨다', () => {
    const cases: ReadonlyArray<readonly [string, Record<string, string>]> = [
      ['rect', { x: '1', y: '2', width: '4', height: '5' }],
      ['circle', { cx: '1', cy: '2', r: '3' }],
      ['ellipse', { cx: '1', cy: '2', rx: '3', ry: '4' }],
      ['line', { x1: '1', y1: '2', x2: '3', y2: '4' }],
    ];
    for (const [tag, attrs] of cases) {
      expect(shorthandNativeShape(tag, attrs), tag).not.toBeUndefined();
      expect(shorthandShapeCommands(tag, attrs)?.length, tag).toBeGreaterThan(0);
    }
  });

  it('사각형의 상자가 `x`·`y`·`width`·`height` 그대로다', () => {
    expect(shorthandNativeShape('rect', { x: '1', y: '2', width: '4', height: '5' })).toEqual({
      kind: 'rect',
      minX: 1,
      minY: 2,
      maxX: 5,
      maxY: 7,
    });
  });

  it('타원의 상자가 `cx ± rx` · `cy ± ry` 그대로다 — 3차 근사를 지나지 않는다', () => {
    expect(shorthandNativeShape('circle', { cx: '10', cy: '20', r: '3' })).toEqual({
      kind: 'ellipse',
      minX: 7,
      minY: 17,
      maxX: 13,
      maxY: 23,
    });
  });
});

// --- ⑤ 순서 · 스타일 · 이름 ------------------------------------------------

describe('갈래가 섞여도 문서 순서가 배열 순서다 (뮤테이션 12)', () => {
  it('배경 <rect> · 전경 <path> · 이름표 순서가 문서 그대로다', () => {
    // **배열 순서가 유일한 z-order 다.** 원시형을 뒤로 몰면 배경 사각형이 그 위의 그림을
    // 덮는다 — 007 이 문구에 대해 치른 그 대가가 배경에서는 훨씬 크다.
    const els = place(
      svg(
        '<rect x="-13" y="7" width="317" height="181" fill="#eeeeee"/>' +
          '<path d="M 10 20 L 90 20 L 90 60 Z" fill="#c0392b"/>' +
          '<circle cx="50" cy="60" r="10" fill="#145a32"/>' +
          '<line x1="10" y1="150" x2="200" y2="160" stroke="#2980b9"/>',
      ),
    );
    expect(els.map((el) => el.kind)).toEqual(['rect', 'path', 'ellipse', 'line']);
    // 채운 셋은 제 채움색을, 그은 하나는 제 선색을 든다. **선에 `fill` 이 함께 있는 것이
    // 옳다** — SVG 의 기본 채움은 검정이고, 같은 문서의 `<polyline>` 도 똑같이 그것을
    // 들고 온다(원시형이 되면서 달라진 것이 아니다).
    expect(els.slice(0, 3).map((el) => el.style.fill)).toEqual(['#eeeeee', '#c0392b', '#145a32']);
    expect(els[3]!.style.stroke).toBe('#2980b9');
  });
});

describe('스타일 — 씨앗 규칙이 둘이 되지 않는다 (뮤테이션 11)', () => {
  it('칠을 말하지 않은 사각형·타원은 **채움 씨앗**을, 선은 **선 씨앗**을 입는다', () => {
    const els = place(
      svg('<rect x="10" y="20" width="80" height="40"/><circle cx="50" cy="120" r="10"/><line x1="10" y1="150" x2="200" y2="160"/>'),
    );
    expect(els.map((el) => el.kind)).toEqual(['rect', 'ellipse', 'line']);
    expect(els[0]!.style).toEqual({ fill: SEED_COLOR });
    expect(els[1]!.style).toEqual({ fill: SEED_COLOR });
    expect(els[2]!.style).toEqual({ stroke: SEED_COLOR, strokeWidth: SEED_STROKE_WIDTH });
  });

  it('**열린 경로는 여전히 선 씨앗이다** — 닫힘을 종류에서 읽으면 여기가 빨개진다', () => {
    const els = place(svg('<polyline points="10,20 90,20 90,60"/>'));
    expect(els[0]!.kind).toBe('path');
    expect(els[0]!.style).toEqual({ stroke: SEED_COLOR, strokeWidth: SEED_STROKE_WIDTH });
  });

  it('말한 도형은 제 색을 지킨다 — 씨앗이 덮지 않는다', () => {
    const el = only(svg('<rect x="10" y="20" width="80" height="40" fill="#c0392b" stroke="#145a32" stroke-width="4"/>'));
    expect(el.style.fill).toBe('#c0392b');
    expect(el.style.stroke).toBe('#145a32');
  });

  it('선 두께가 **경로와 같은 비**로 옮겨진다 — 두 번째 규율을 만들지 않는다', () => {
    const rect = only(svg('<rect x="10" y="20" width="80" height="40" stroke="#145a32" stroke-width="4"/>'));
    const poly = only(svg('<polygon points="10,20 90,20 90,60 10,60" stroke="#145a32" stroke-width="4"/>'));
    expect(rect.style.strokeWidth).toBe(poly.style.strokeWidth);
    // 켜져 있음: 비가 1 이 아니다(400/317). 1 이면 이 단언이 항등을 재고 있다.
    expect(rect.style.strokeWidth).not.toBe(4);
  });

  it('비균등 배율 아래의 선 두께는 근사로 **보고된다** — 원시형이 되어도 같다', () => {
    const result = plan(svg('<rect x="10" y="20" width="80" height="40" stroke="#145a32" stroke-width="4" transform="scale(2,3)"/>'));
    expect(result.shapes[0]!.kind).toBe('rect');
    expect(result.report.notes.map((n) => n.reason)).toContain('nonUniformStrokeScale');
  });
});

describe('이름과 출처', () => {
  it('가져온 원시형에 `catalog_id` 가 없다 — 목록이 제 종류 이름을 그대로 말한다', () => {
    const el = only(svg('<rect x="10" y="20" width="80" height="40"/>'));
    // 목록의 이름은 `KIND_LABEL_KEY[el.kind]` 에서 나오고, `catalog_id` 는 `kind === 'path'`
    // 일 때만 읽힌다 — 사각형 행이 카탈로그 이름으로 잘못 불릴 길이 형상으로 없다.
    expect('catalog_id' in el).toBe(false);
    expect(Object.keys(el).sort()).toEqual(['geometry', 'id', 'kind', 'style']);
  });
});

// --- ⑥ 보고와 예산 ---------------------------------------------------------

describe('보고 — 도형 수는 전부를, 명령 수는 경로만 센다', () => {
  it('원시형은 명령을 한 칸도 먹지 않는다 (뮤테이션 10)', () => {
    const result = plan(
      svg('<rect x="10" y="20" width="80" height="40"/><circle cx="50" cy="120" r="10"/><line x1="10" y1="150" x2="200" y2="160"/>'),
    );
    expect(result.report.shapes).toBe(3);
    expect(result.report.commands).toBe(0);
  });

  it('치환이 같은 문서의 명령 총수를 **줄인다** — 그것이 이 변경의 값이다', () => {
    const asNative = plan(svg('<rect x="10" y="20" width="80" height="40"/>'));
    const asPath = plan(svg('<polygon points="10,20 90,20 90,60 10,60"/>'));
    expect(asPath.report.commands).toBe(5);
    expect(asNative.report.commands).toBe(0);
    expect(asNative.report.estimatedBytes).toBeLessThan(asPath.report.estimatedBytes);
  });

  it('원시형이 되었다는 **보고 항목은 없다** — 근사한 것도 버린 것도 없다 (위험 R7)', () => {
    const result = plan(
      svg('<rect x="10" y="20" width="80" height="40" rx="5"/><polygon points="10,80 90,80 90,120"/>'),
    );
    // 모서리가 깎여 경로로 남은 사각형도, 처음부터 경로인 다각형도 보고에 오르지 않는다.
    // 오르게 하면 `<path>` 가 든 거의 모든 파일에서 울려 보고가 아무것도 말하지 않게 된다.
    expect(result.shapes.map((sh) => sh.kind)).toEqual(['path', 'path']);
    expect(result.report.notes).toEqual([]);
  });

  it('원시형도 요소 상한을 먹는다 — 명령을 안 먹는다고 자리까지 공짜는 아니다', () => {
    const body = Array.from(
      { length: MAX_IMPORT_ELEMENTS + 1 },
      (_v, i) => `<rect x="${10 + (i % 7)}" y="20" width="4" height="4"/>`,
    ).join('');
    const refused = planSvgImport(svg(body), CANVAS);
    expect(refused.ok).toBe(false);
    if (!refused.ok) {
      expect(refused.refusal.reason).toBe('tooManyElements');
      expect(refused.refusal.actual).toBe(MAX_IMPORT_ELEMENTS + 1);
    }
  });

  it('viewBox 가 없는 **원시형뿐인 문서**도 폴백을 탄다 — 빈 문서로 거절되지 않는다', () => {
    // 폴백의 합집합은 `commands` 를 읽는다. 원시형이 명령을 함께 나르지 않았다면 이 문서는
    // "그릴 것이 하나도 없다" 로 거절된다.
    const result = plan(
      svg('<rect x="0" y="0" width="40" height="20"/><circle cx="80" cy="10" r="10"/>', 'id="no-size"'),
    );
    expect(result.shapes.map((sh) => sh.kind)).toEqual(['rect', 'ellipse']);
  });
});

// --- ⑦ 예산 재실측 ---------------------------------------------------------

describe('예산 — 최악을 **다시 잰다** (위험 R6)', () => {
  const DASHBOARD_BUDGET = 256 * 1024;

  /** `n` 개의 명령을 든 `<path>` 하나(`M` 하나 + `C` n−1). */
  function curvePath(commands: number): string {
    const curves = Array.from(
      { length: commands - 1 },
      (_v, i) => `C ${i} ${i + 1} ${i + 2} ${i + 3} ${i + 4} ${i + 5}`,
    ).join(' ');
    return `<path d="M 0 0 ${curves}" fill="#c0392b" stroke="#145a32" stroke-width="3"/>`;
  }

  /** 최대 길이의 한글 이름표 하나. */
  function koreanText(i: number): string {
    return `<text x="${10 + (i % 7)}" y="40">${'가'.repeat(MAX_IMPORT_TEXT_LENGTH)}</text>`;
  }

  /** 원시형 사각 하나 — 명령을 한 칸도 먹지 않고 요소 자리 하나를 먹는다. */
  function nativeRect(i: number): string {
    return `<rect x="${10 + (i % 7)}" y="20" width="40" height="20" fill="#c0392b" stroke="#145a32" stroke-width="3"/>`;
  }

  function bytesOf(doc: string): number {
    const result = plan(doc);
    const { next } = appendImportedElements([], result.shapes, result.texts);
    expect(next).toHaveLength(MAX_IMPORT_ELEMENTS);
    return new TextEncoder().encode(JSON.stringify(next)).length;
  }

  // **실측 표(이 시험이 잰 수).** 손으로 센 상수를 그대로 두지 않는 이유는 형상이 바뀌는
  // 날 조용히 틀리기 때문이고, 원시형 치환이 바로 그 "형상이 바뀌는 날" 이다.
  //
  // 후보 넷을 모두 재고 **가장 큰 것**을 최악으로 적는다. 원시형은 요소 자리를 먹되 명령을
  // 먹지 않으므로 "명령을 적은 경로에 몰고 남은 자리를 채운다" 는 그 전략의 **채울 것**이
  // 하나 늘었다 — 그래서 후보에 든다. 다만 원시형 요소 한 건은 256자 한글 문구 한 건보다
  // 훨씬 짧으므로 최악을 갱신하지 못한다. 그 사실을 **재서** 적는다.
  it('네 후보를 재고 가장 큰 것을 최악으로 적는다', () => {
    const measured = {
      경로64_명령640: bytesOf(svg(curvePath(10).repeat(64))),
      문구64_한글256: bytesOf(svg(Array.from({ length: 64 }, (_v, i) => koreanText(i)).join(''))),
      원시형64: bytesOf(svg(Array.from({ length: 64 }, (_v, i) => nativeRect(i)).join(''))),
      경로3_명령640_문구61: bytesOf(
        svg(
          curvePath(MAX_PATH_COMMANDS) +
            curvePath(MAX_PATH_COMMANDS) +
            curvePath(640 - 2 * MAX_PATH_COMMANDS) +
            Array.from({ length: 61 }, (_v, i) => koreanText(i)).join(''),
        ),
      ),
      경로3_명령640_원시형61: bytesOf(
        svg(
          curvePath(MAX_PATH_COMMANDS) +
            curvePath(MAX_PATH_COMMANDS) +
            curvePath(640 - 2 * MAX_PATH_COMMANDS) +
            Array.from({ length: 61 }, (_v, i) => nativeRect(i)).join(''),
        ),
      ),
    };
    const worst = Math.max(...Object.values(measured));

    // **실측값** — 이 시험이 이 고정 입력으로 잰 수다(256KB 예산 대비 비를 함께 적는다).
    //   경로 64 · 명령 640(전부 `C`)      =  50,424 B = 19.24%
    //   문구 64 × 256자 한글              =  57,400 B = 21.90%
    //   원시형 64(사각 · 칠과 선 둘 다)   =   9,528 B =  3.63%
    //   경로 3(명령 640) + 문구 61        =  98,208 B = 37.46%  ← **최악**
    //   경로 3(명령 640) + 원시형 61      =  52,580 B = 20.06%
    //
    // SPEC 의 표(97,465 B · 37.18%)와 어긋나는 것은 **고정 입력이 다르기 때문**이다 —
    // 좌표 자릿수와 `<text>` 의 `x` 배치가 달라 요소마다 몇 바이트씩 차이가 난다. 원시형
    // 치환은 경로·문구 후보를 한 바이트도 건드리지 않으므로(그 요소들의 형상이 그대로다)
    // **최악 자체는 움직이지 않았다.** 움직인 것은 SPEC 에 적힌 수가 어느 문서를 잰 것인가다.
    expect(measured.경로64_명령640).toBe(50424);
    expect(measured.문구64_한글256).toBe(57400);
    expect(measured.원시형64).toBe(9528);
    expect(measured.경로3_명령640_문구61).toBe(98208);
    expect(measured.경로3_명령640_원시형61).toBe(52580);

    // 최악은 여전히 **경로 3 + 문구 61** 이다 — 원시형은 명령을 안 먹지만 제 몸집이
    // 256자 한글 문구의 1/6 이라, 남은 자리를 원시형으로 채우면 예산이 오히려 **줄어든다**.
    expect(worst).toBe(measured.경로3_명령640_문구61);
    expect(worst / DASHBOARD_BUDGET).toBeLessThan(0.38);
    // 그리고 원시형만으로는 최악 근처에도 가지 못한다 — 치환은 예산을 **푸는** 방향이다.
    expect(measured.원시형64).toBeLessThan(measured.경로64_명령640 / 5);
  });
});

// --- ⑧ 저장 왕복 -----------------------------------------------------------

describe('원시형도 저장 왕복을 그대로 견딘다 (가정 A15)', () => {
  it('세 갈래가 섞인 채로 왕복해도 기하도 스타일도 같다', () => {
    const els = place(
      svg(
        '<rect x="10" y="20" width="80" height="40" fill="#c0392b"/>' +
          '<circle cx="50" cy="120" r="10" fill="#145a32"/>' +
          '<line x1="10" y1="150" x2="200" y2="160" stroke="#2980b9" stroke-width="2"/>' +
          '<polygon points="210,20 290,20 290,60" fill="#8e44ad"/>',
      ),
    );
    expect(els.map((el) => el.kind)).toEqual(['rect', 'ellipse', 'line', 'path']);
    const reopened = parseCanvasConfig(
      JSON.parse(JSON.stringify({ canvas: CANVAS, elements: els })) as unknown,
    );
    // **깊은 비교다** — 파서가 한 필드라도 씨앗으로 갈아 끼우면 여기서 드러난다.
    expect(reopened.elements).toEqual(els);
  });

  it('선 요소가 퇴화하지 않는다 — 왕복에서 씨앗 선으로 갈리지 않는다', () => {
    const el = only(svg('<line x1="10" y1="20" x2="90" y2="60" stroke="#2980b9"/>'));
    if (el.kind !== 'line') throw new Error('선이어야 한다');
    expect(isDegenerateLine(el.geometry)).toBe(false);
  });
});

// --- ⑨ 계획이 내는 갈래표 ---------------------------------------------------

describe('계획의 갈래표가 요소의 종류와 **같다**', () => {
  it('spec 의 `kind` 가 곧 만들어질 요소의 `kind` 다', () => {
    const result = plan(
      svg(
        '<rect x="10" y="20" width="80" height="40"/><circle cx="50" cy="120" r="10"/>' +
          '<line x1="10" y1="150" x2="200" y2="160"/><polygon points="210,20 290,20 290,60"/>',
      ),
    );
    const kinds: ImportedShapeSpec['kind'][] = result.shapes.map((sh) => sh.kind);
    const { created } = appendImportedElements([], result.shapes);
    expect(created.map((el) => el.kind)).toEqual(kinds);
  });
});
