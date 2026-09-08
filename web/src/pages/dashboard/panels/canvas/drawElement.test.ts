// drawElement 렌더 층 테스트 (SPEC-CANVAS-001 T4).
//
// jsdom canvas 는 `node-canvas` 없이는 2D context 를 주지 않는다. 그래서 여기서는 **진짜
// canvas 를 쓰지 않는다** — `DrawContext2D` 를 만족하는 기록 스텁을 넘겨 호출 순서·인자·
// 설정된 속성을 관찰한다. 그리기는 부수효과라 결과를 볼 방법이 호출 기록뿐이고, 최소
// 구조 인터페이스가 그 기록을 1급 관찰 대상으로 만든 것이 T4 설계의 요점이다.

import { describe, it, expect } from 'vitest';

import type {
  CanvasElement,
  EllipseElement,
  LineElement,
  RectElement,
  TextElement,
} from './canvasConfig';
import type { StageSize } from './canvasGeometry';
import type { ResolvedStyle } from './canvasRules';
import {
  clearSurface,
  drawElement,
  drawElements,
  DEFAULT_FONT_FAMILY,
  TEXT_BASELINE,
  type DrawContext2D,
} from './drawElement';

// --- 기록 스텁 -----------------------------------------------------------

/** 한 글자 폭(px). measureText 대체값이라 정렬 계산이 정수로 떨어진다. */
const CHAR_WIDTH = 10;

type Recorded = [string, ...unknown[]];

interface Recorder extends DrawContext2D {
  readonly calls: Recorded[];
}

function makeRecorder(): Recorder {
  const calls: Recorded[] = [];
  let fillStyle: string | CanvasGradient | CanvasPattern = '';
  let strokeStyle: string | CanvasGradient | CanvasPattern = '';
  let lineWidth = 1;
  let globalAlpha = 1;
  let font = '';
  let textAlign: CanvasTextAlign = 'start';
  let textBaseline: CanvasTextBaseline = 'alphabetic';

  return {
    calls,
    save() {
      calls.push(['save']);
    },
    restore() {
      calls.push(['restore']);
    },
    setTransform(a, b, c, d, e, f) {
      calls.push(['setTransform', a, b, c, d, e, f]);
    },
    beginPath() {
      calls.push(['beginPath']);
    },
    rect(x, y, w, h) {
      calls.push(['rect', x, y, w, h]);
    },
    ellipse(x, y, rx, ry, rotation, start, end) {
      calls.push(['ellipse', x, y, rx, ry, rotation, start, end]);
    },
    moveTo(x, y) {
      calls.push(['moveTo', x, y]);
    },
    lineTo(x, y) {
      calls.push(['lineTo', x, y]);
    },
    stroke() {
      calls.push(['stroke']);
    },
    fill() {
      calls.push(['fill']);
    },
    fillText(text, x, y) {
      calls.push(['fillText', text, x, y]);
    },
    measureText(text) {
      calls.push(['measureText', text]);
      return { width: text.length * CHAR_WIDTH };
    },
    clearRect(x, y, w, h) {
      calls.push(['clearRect', x, y, w, h]);
    },
    fillRect(x, y, w, h) {
      calls.push(['fillRect', x, y, w, h]);
    },
    get fillStyle() {
      return fillStyle;
    },
    set fillStyle(v) {
      fillStyle = v;
      calls.push(['fillStyle', v]);
    },
    get strokeStyle() {
      return strokeStyle;
    },
    set strokeStyle(v) {
      strokeStyle = v;
      calls.push(['strokeStyle', v]);
    },
    get lineWidth() {
      return lineWidth;
    },
    set lineWidth(v) {
      lineWidth = v;
      calls.push(['lineWidth', v]);
    },
    get globalAlpha() {
      return globalAlpha;
    },
    set globalAlpha(v) {
      globalAlpha = v;
      calls.push(['globalAlpha', v]);
    },
    get font() {
      return font;
    },
    set font(v) {
      font = v;
      calls.push(['font', v]);
    },
    get textAlign() {
      return textAlign;
    },
    set textAlign(v) {
      textAlign = v;
      calls.push(['textAlign', v]);
    },
    get textBaseline() {
      return textBaseline;
    },
    set textBaseline(v) {
      textBaseline = v;
      calls.push(['textBaseline', v]);
    },
  };
}

/** 호출 이름만 뽑는다(순서 단언용). */
function ops(ctx: Recorder): string[] {
  return ctx.calls.map((c) => c[0]);
}

/** 특정 호출의 인자를 뽑는다(첫 번째 것). */
function argsOf(ctx: Recorder, op: string): unknown[] | undefined {
  return ctx.calls.find((c) => c[0] === op)?.slice(1);
}

// --- 고정 입력 -----------------------------------------------------------

/** 200×100 스테이지 — 정규화 좌표가 정수 px 로 떨어진다. */
const STAGE: StageSize = { width: 200, height: 100 };

function rectEl(over: Partial<RectElement> = {}): RectElement {
  return { id: 'r1', kind: 'rect', geometry: { x: 0.1, y: 0.2, w: 0.5, h: 0.4 }, style: {}, ...over };
}
function ellipseEl(over: Partial<EllipseElement> = {}): EllipseElement {
  return { id: 'e1', kind: 'ellipse', geometry: { x: 0.1, y: 0.2, w: 0.5, h: 0.4 }, style: {}, ...over };
}
function lineEl(over: Partial<LineElement> = {}): LineElement {
  return { id: 'l1', kind: 'line', geometry: { x1: 0, y1: 0, x2: 1, y2: 1 }, style: {}, ...over };
}
function textEl(over: Partial<TextElement> = {}): TextElement {
  return { id: 't1', kind: 'text', geometry: { x: 0.5, y: 0.5 }, style: {}, ...over };
}

const EMPTY: ResolvedStyle = {};

describe('drawElement — 도형 4종', () => {
  it('rect 는 투영된 px 사각형 경로를 그리고 채움·선을 적용한다', () => {
    const ctx = makeRecorder();
    drawElement(ctx, rectEl(), { fill: '#ff0000', stroke: '#000000', strokeWidth: 3 }, undefined, STAGE);

    // 0.1*200=20, 0.2*100=20, 0.5*200=100, 0.4*100=40.
    expect(argsOf(ctx, 'rect')).toEqual([20, 20, 100, 40]);
    expect(ops(ctx)).toEqual([
      'save',
      'globalAlpha',
      'beginPath',
      'rect',
      'fillStyle',
      'fill',
      'strokeStyle',
      'lineWidth',
      'stroke',
      'restore',
    ]);
    expect(argsOf(ctx, 'fillStyle')).toEqual(['#ff0000']);
    expect(argsOf(ctx, 'lineWidth')).toEqual([3]);
  });

  it('ellipse 는 중심+반지름으로 그린다(ellipseParams 규약)', () => {
    const ctx = makeRecorder();
    drawElement(ctx, ellipseEl(), { fill: '#00ff00' }, undefined, STAGE);

    // 박스 (20,20,100,40) → 중심 (70,40), 반지름 (50,20).
    expect(argsOf(ctx, 'ellipse')).toEqual([70, 40, 50, 20, 0, 0, Math.PI * 2]);
    expect(ops(ctx)).toContain('fill');
  });

  it('line 은 두 끝점을 잇고 채우지 않는다(열린 경로의 fill 은 뜻이 없다)', () => {
    const ctx = makeRecorder();
    drawElement(ctx, lineEl(), { fill: '#ff0000', stroke: '#0000ff', strokeWidth: 2 }, undefined, STAGE);

    expect(argsOf(ctx, 'moveTo')).toEqual([0, 0]);
    expect(argsOf(ctx, 'lineTo')).toEqual([200, 100]);
    expect(ops(ctx)).toContain('stroke');
    expect(ops(ctx)).not.toContain('fill');
  });

  it('text 는 실측 폭으로 좌측 끝 원점을 정하고 고정 정렬·기준선으로 그린다', () => {
    const ctx = makeRecorder();
    drawElement(
      ctx,
      textEl({ style: {} }),
      { textColor: '#111111', align: 'center', fontSize: 20, fontWeight: 'bold' },
      'ab',
      STAGE,
    );

    expect(argsOf(ctx, 'font')).toEqual([`bold 20px ${DEFAULT_FONT_FAMILY}`]);
    expect(argsOf(ctx, 'textAlign')).toEqual(['left']);
    expect(argsOf(ctx, 'textBaseline')).toEqual([TEXT_BASELINE]);
    expect(argsOf(ctx, 'measureText')).toEqual(['ab']);
    // 기준점 (100,50), 폭 2*10=20, center → 좌측 끝 x = 100 - 10 = 90.
    expect(argsOf(ctx, 'fillText')).toEqual(['ab', 90, 50]);
  });

  it('text 의 기본 글꼴은 굵기 없는 기본 크기다', () => {
    const ctx = makeRecorder();
    drawElement(ctx, textEl(), { textColor: '#111111' }, 'x', STAGE);
    expect(argsOf(ctx, 'font')).toEqual([`14px ${DEFAULT_FONT_FAMILY}`]);
    // align 미지정 = left → 원점이 기준점 그대로.
    expect(argsOf(ctx, 'fillText')).toEqual(['x', 100, 50]);
  });
});

describe('drawElement — 스타일 규율', () => {
  it('visible:false 면 save/restore 조차 하지 않고 아무것도 그리지 않는다', () => {
    const ctx = makeRecorder();
    drawElement(ctx, rectEl(), { fill: '#ff0000', visible: false }, 'label', STAGE);
    expect(ctx.calls).toHaveLength(0);
  });

  it('fill 이 없으면 채우지 않고 stroke 가 없으면 긋지 않는다(기본색을 지어내지 않는다)', () => {
    const ctx = makeRecorder();
    drawElement(ctx, rectEl(), EMPTY, undefined, STAGE);
    expect(ops(ctx)).toEqual(['save', 'globalAlpha', 'beginPath', 'rect', 'restore']);
  });

  it('strokeWidth 가 0 이면 stroke 색이 있어도 긋지 않는다', () => {
    const ctx = makeRecorder();
    drawElement(ctx, rectEl(), { stroke: '#000000', strokeWidth: 0 }, undefined, STAGE);
    expect(ops(ctx)).not.toContain('stroke');
  });

  it('strokeWidth 미지정은 기본 두께로 긋는다', () => {
    const ctx = makeRecorder();
    drawElement(ctx, rectEl(), { stroke: '#000000' }, undefined, STAGE);
    expect(argsOf(ctx, 'lineWidth')).toEqual([1]);
  });

  it('opacity 는 globalAlpha 로 적용되며 미지정·손상 값은 1 로 떨어진다', () => {
    const half = makeRecorder();
    drawElement(half, rectEl(), { opacity: 0.25 }, undefined, STAGE);
    expect(argsOf(half, 'globalAlpha')).toEqual([0.25]);

    const missing = makeRecorder();
    drawElement(missing, rectEl(), EMPTY, undefined, STAGE);
    expect(argsOf(missing, 'globalAlpha')).toEqual([1]);

    const broken = makeRecorder();
    drawElement(broken, rectEl(), { opacity: Number.NaN }, undefined, STAGE);
    expect(argsOf(broken, 'globalAlpha')).toEqual([1]);

    const over = makeRecorder();
    drawElement(over, rectEl(), { opacity: 4 }, undefined, STAGE);
    expect(argsOf(over, 'globalAlpha')).toEqual([1]);

    const under = makeRecorder();
    drawElement(under, rectEl(), { opacity: -1 }, undefined, STAGE);
    expect(argsOf(under, 'globalAlpha')).toEqual([0]);
  });

  it('한 요소의 스타일이 다음 요소로 새지 않는다(save/restore 로 갇힌다)', () => {
    const ctx = makeRecorder();
    drawElements(
      ctx,
      [rectEl({ id: 'a' }), rectEl({ id: 'b' })],
      { a: { fill: '#ff0000', opacity: 0.5 }, b: {} },
      {},
      STAGE,
    );
    expect(ops(ctx)).toEqual([
      'save',
      'globalAlpha',
      'beginPath',
      'rect',
      'fillStyle',
      'fill',
      'restore',
      // 두 번째 요소는 자기 스타일만 세운다 — fillStyle/fill 이 없다.
      'save',
      'globalAlpha',
      'beginPath',
      'rect',
      'restore',
    ]);
    // 두 번째 요소의 globalAlpha 는 첫 요소의 0.5 를 물려받지 않는다.
    expect(ctx.calls.filter((c) => c[0] === 'globalAlpha').map((c) => c[1])).toEqual([0.5, 1]);
  });
});

describe('drawElement — 도형 라벨', () => {
  it('도형에 붙은 문구는 labelAnchor 에 그려진다(rect 는 중심)', () => {
    const ctx = makeRecorder();
    drawElement(ctx, rectEl(), { fill: '#ff0000', textColor: '#ffffff' }, 'ab', STAGE);
    // rect 중심 (70,40), 폭 20, align 미지정(left) → 원점 (70,40).
    expect(argsOf(ctx, 'fillText')).toEqual(['ab', 70, 40]);
  });

  it('line 라벨은 중점에 그려진다', () => {
    const ctx = makeRecorder();
    drawElement(ctx, lineEl(), { stroke: '#000000', textColor: '#ffffff' }, 'ab', STAGE);
    expect(argsOf(ctx, 'fillText')).toEqual(['ab', 100, 50]);
  });

  it('도형 라벨은 textColor 가 없으면 그리지 않는다(도형 fill 로 폴백하면 제 색에 묻힌다)', () => {
    const ctx = makeRecorder();
    drawElement(ctx, rectEl(), { fill: '#ff0000' }, 'ab', STAGE);
    expect(ops(ctx)).not.toContain('fillText');
  });

  it('text 요소는 textColor 가 없으면 fill 을 글자색으로 쓴다(채울 도형이 없다)', () => {
    const ctx = makeRecorder();
    drawElement(ctx, textEl(), { fill: '#123456' }, 'ab', STAGE);
    expect(argsOf(ctx, 'fillStyle')).toEqual(['#123456']);
    expect(ops(ctx)).toContain('fillText');
  });

  it('문구가 없거나 빈 문자열이면 글자를 그리지 않는다', () => {
    const none = makeRecorder();
    drawElement(none, textEl(), { textColor: '#000000' }, undefined, STAGE);
    expect(ops(none)).not.toContain('fillText');

    const empty = makeRecorder();
    drawElement(empty, textEl(), { textColor: '#000000' }, '', STAGE);
    expect(ops(empty)).not.toContain('fillText');
  });
});

describe('drawElement — 견고성', () => {
  it('알 수 없는 kind 는 예외 없이 건너뛴다', () => {
    const ctx = makeRecorder();
    const malformed = {
      id: 'x',
      kind: 'diamond',
      geometry: { x: 0.5, y: 0.5 },
      style: {},
    } as unknown as CanvasElement;
    expect(() => drawElement(ctx, malformed, EMPTY, undefined, STAGE)).not.toThrow();
    expect(ops(ctx)).toEqual(['save', 'globalAlpha', 'restore']);
  });

  it('context 가 그리다 던져도 전파하지 않고 상태를 되돌린다(AC-E4 마지막 프레임 보존)', () => {
    const ctx = makeRecorder();
    ctx.rect = () => {
      throw new Error('boom');
    };
    expect(() => drawElement(ctx, rectEl(), { fill: '#ff0000' }, undefined, STAGE)).not.toThrow();
    expect(ops(ctx).at(-1)).toBe('restore');
  });
});

describe('drawElements', () => {
  it('배열 순서대로 그린다(뒤가 위 — 001 의 유일한 z-order)', () => {
    const ctx = makeRecorder();
    drawElements(
      ctx,
      [textEl({ id: 'first' }), textEl({ id: 'second' })],
      { first: { textColor: '#000000' }, second: { textColor: '#000000' } },
      { first: 'A', second: 'B' },
      STAGE,
    );
    const drawn = ctx.calls.filter((c) => c[0] === 'fillText').map((c) => c[1]);
    expect(drawn).toEqual(['A', 'B']);
  });

  it('맵에 항목이 없으면 요소의 기본 스타일·기본 문구로 떨어진다(AC-E2/AC-E3)', () => {
    const ctx = makeRecorder();
    drawElements(ctx, [textEl({ style: { textColor: '#abcdef' }, text: 'base' })], {}, {}, STAGE);
    expect(argsOf(ctx, 'fillStyle')).toEqual(['#abcdef']);
    expect(argsOf(ctx, 'fillText')).toEqual(['base', 100, 50]);
  });

  it('요소가 0개면 아무 호출도 하지 않는다(AC-E1)', () => {
    const ctx = makeRecorder();
    drawElements(ctx, [], {}, {}, STAGE);
    expect(ctx.calls).toHaveLength(0);
  });
});

describe('clearSurface', () => {
  it('항등 변환으로 되돌린 뒤 백킹 버퍼 전체를 지운다', () => {
    const ctx = makeRecorder();
    clearSurface(ctx, { width: 200, height: 160, scale: 2 });
    expect(ops(ctx)).toEqual(['setTransform', 'globalAlpha', 'clearRect']);
    expect(argsOf(ctx, 'setTransform')).toEqual([1, 0, 0, 1, 0, 0]);
    expect(argsOf(ctx, 'clearRect')).toEqual([0, 0, 200, 160]);
  });

  it('배경색이 있으면 지운 뒤 그 색으로 칠한다', () => {
    const ctx = makeRecorder();
    clearSurface(ctx, { width: 200, height: 160, scale: 2 }, '#101010');
    expect(ops(ctx)).toEqual(['setTransform', 'globalAlpha', 'clearRect', 'fillStyle', 'fillRect']);
    expect(argsOf(ctx, 'fillRect')).toEqual([0, 0, 200, 160]);
  });
});
