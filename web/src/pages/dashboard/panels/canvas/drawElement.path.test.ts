// 경로 요소 렌더 테스트 (SPEC-CANVAS-008 M3).
//
// jsdom 은 2D context 를 주지 않으므로 `drawElement.test.ts` 와 같은 수법을 쓴다 —
// `DrawContext2D` 를 만족하는 기록 스텁을 넘겨 **호출 순서와 인자**를 관찰한다. 그
// 인터페이스의 최소성이 있어서 렌더 경로 전량이 jsdom 없이 검증되므로, 이 파일은 그
// 최소성 자체도 함께 잰다(AC-E2).
//
// 이 파일이 정면으로 겨누는 함정은 **D4** 다: `fill()` 은 부분 경로를 암묵적으로 닫으므로
// **채운 도형만 보는 시험은 `closePath` 가 기록되지 않아도 통과한다.** 그래서 닫힘은 선으로
// 재고, **열린 도형을 하나 함께 넣어** 거기엔 `closePath` 가 **없음**을 단언한다. 뒤쪽이
// 없으면 "언제나 닫는다" 는 결함이 그대로 통과한다.
//
// 고정 입력은 `canvasGeometry.path.test.ts` 와 같다(축척 0.5 · 비정사각 · 원점 ≠ 0).
//
// @spec SPEC-CANVAS-008 REQ-01 · REQ-02 · AC-02 · AC-E2

import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { describe, expect, it } from 'vitest';

import type { PathElement } from './canvasConfig';
import { projectBox, type CanvasProjection } from './canvasGeometry';
import type { ResolvedStyle } from './canvasRules';
import { drawElement, drawElements, type DrawContext2D } from './drawElement';
import { PATH_LOCAL_EXTENT, type PathCommand } from './shapes/pathTypes';

// --- 기록 스텁 -----------------------------------------------------------

const CHAR_WIDTH = 10;

type Recorded = [string, ...unknown[]];

interface Recorder extends DrawContext2D {
  readonly calls: Recorded[];
}

function makeRecorder(): Recorder {
  const calls: Recorded[] = [];
  return {
    calls,
    save() {
      calls.push(['save']);
    },
    restore() {
      calls.push(['restore']);
    },
    setTransform() {},
    beginPath() {
      calls.push(['beginPath']);
    },
    rect(x, y, w, h) {
      calls.push(['rect', x, y, w, h]);
    },
    ellipse(x, y, rx, ry) {
      calls.push(['ellipse', x, y, rx, ry]);
    },
    moveTo(x, y) {
      calls.push(['moveTo', x, y]);
    },
    lineTo(x, y) {
      calls.push(['lineTo', x, y]);
    },
    closePath() {
      calls.push(['closePath']);
    },
    bezierCurveTo(cp1x, cp1y, cp2x, cp2y, x, y) {
      calls.push(['bezierCurveTo', cp1x, cp1y, cp2x, cp2y, x, y]);
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
      return { width: text.length * CHAR_WIDTH };
    },
    clearRect() {},
    fillRect() {},
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    globalAlpha: 1,
    font: '',
    textAlign: 'left',
    textBaseline: 'middle',
  };
}

/** 호출 이름만 뽑는다(순서 확인용). */
const names = (r: Recorder): string[] => r.calls.map((c) => c[0]);

// --- 고정 입력 -----------------------------------------------------------

const PROJ: CanvasProjection = {
  stage: { width: 250, height: 200 },
  canvas: { width: 500, height: 400 },
};

const BOX = { x: 37, y: 61, w: 160, h: 90 } as const;

/** 비대칭 · 닫힘 · 다각형. 직각삼각형이라 x·y 를 뒤바꾸면 모양이 달라진다. */
const RIGHT_TRIANGLE: readonly PathCommand[] = [
  { c: 'M', x: 0, y: PATH_LOCAL_EXTENT },
  { c: 'L', x: PATH_LOCAL_EXTENT, y: PATH_LOCAL_EXTENT },
  { c: 'L', x: 0, y: 0 },
  { c: 'Z' },
];

/** **열림** · 곡선. `Z` 가 없으므로 `closePath` 가 기록되면 안 된다(D4). */
const ARROW_CURVED: readonly PathCommand[] = [
  { c: 'M', x: 0, y: 5000 },
  { c: 'C', x1: 3000, y1: 1000, x2: 7000, y2: 9000, x: PATH_LOCAL_EXTENT, y: 5000 },
];

function pathElement(path: readonly PathCommand[], over: Partial<PathElement> = {}): PathElement {
  return { id: 'p1', kind: 'path', geometry: { ...BOX }, path: [...path], style: {}, ...over };
}

const FILLED: ResolvedStyle = { fill: '#3b82f6' };
const STROKED: ResolvedStyle = { stroke: '#1e40af', strokeWidth: 2 };

// --- 시험 ---------------------------------------------------------------

describe('drawElement — 경로는 명령 그대로 그려진다 (AC-02)', () => {
  it('닫힌 다각형: beginPath → moveTo → lineTo… → closePath → fill 순서다', () => {
    const ctx = makeRecorder();
    drawElement(ctx, pathElement(RIGHT_TRIANGLE), FILLED, undefined, PROJ);
    expect(names(ctx)).toEqual([
      'save',
      'beginPath',
      'moveTo',
      'lineTo',
      'lineTo',
      'closePath',
      'fill',
      'restore',
    ]);
  });

  it('각 인자가 **상자 안으로 투영된** px 다 — 원점 항과 축척 항을 둘 다 쓴다', () => {
    const ctx = makeRecorder();
    drawElement(ctx, pathElement(RIGHT_TRIANGLE), FILLED, undefined, PROJ);
    // 상자는 {x:18.5, y:30.5, w:80, h:45}. 원점 항을 빼면 (0,45)·(80,45)·(0,0) 이 나오고,
    // 축척 항을 빼면 세 점이 모두 좌상단에 겹친다. 둘 중 하나만 빠져도 여기서 갈린다.
    expect(ctx.calls.filter((c) => c[0] === 'moveTo' || c[0] === 'lineTo')).toEqual([
      ['moveTo', 18.5, 75.5],
      ['lineTo', 98.5, 75.5],
      ['lineTo', 18.5, 30.5],
    ]);
  });

  it('곡선은 `bezierCurveTo` 여섯 인자를 순서 그대로 받는다', () => {
    const ctx = makeRecorder();
    drawElement(ctx, pathElement(ARROW_CURVED), STROKED, undefined, PROJ);
    expect(ctx.calls.find((c) => c[0] === 'bezierCurveTo')).toEqual([
      'bezierCurveTo',
      42.5,
      35,
      74.5,
      71,
      98.5,
      53,
    ]);
  });

  it('그린 점들의 바깥 상자가 `projectBox` 의 상자와 같다 — 경로는 스테이지를 다시 재지 않는다', () => {
    const ctx = makeRecorder();
    // 격자를 가득 채우는 사각형이라, 그린 극값이 곧 상자여야 한다.
    drawElement(
      ctx,
      pathElement([
        { c: 'M', x: 0, y: 0 },
        { c: 'L', x: PATH_LOCAL_EXTENT, y: 0 },
        { c: 'L', x: PATH_LOCAL_EXTENT, y: PATH_LOCAL_EXTENT },
        { c: 'L', x: 0, y: PATH_LOCAL_EXTENT },
        { c: 'Z' },
      ]),
      FILLED,
      undefined,
      PROJ,
    );
    const pts = ctx.calls
      .filter((c) => c[0] === 'moveTo' || c[0] === 'lineTo')
      .map((c) => [c[1] as number, c[2] as number] as const);
    const xs = pts.map(([x]) => x);
    const ys = pts.map(([, y]) => y);
    const box = projectBox(BOX, PROJ);
    expect({
      x: Math.min(...xs),
      y: Math.min(...ys),
      w: Math.max(...xs) - Math.min(...xs),
      h: Math.max(...ys) - Math.min(...ys),
    }).toEqual(box);
  });
});

describe('drawElement — 닫힘은 **선**으로 잰다 (시험 규율 D4)', () => {
  it('닫힌 도형을 선으로만 그려도 `closePath` 가 기록된다', () => {
    const ctx = makeRecorder();
    drawElement(ctx, pathElement(RIGHT_TRIANGLE), STROKED, undefined, PROJ);
    // 마지막 정점에서 첫 정점으로 `lineTo` 로 돌아가면 그 자리는 이음이 아니라 두
    // 끝점이라, 두께 2px 이상에서 뾰족한 꼭짓점에 홈이 파인다.
    expect(names(ctx)).toContain('closePath');
    expect(names(ctx).indexOf('closePath')).toBeLessThan(names(ctx).indexOf('stroke'));
  });

  it('**열린** 도형에는 `closePath` 가 없다 — "언제나 닫는다" 를 여기서 잡는다', () => {
    const ctx = makeRecorder();
    drawElement(ctx, pathElement(ARROW_CURVED), STROKED, undefined, PROJ);
    expect(names(ctx)).not.toContain('closePath');
    expect(names(ctx)).toContain('bezierCurveTo');
  });

  it('`Z` 가 여럿이면 그만큼 닫는다 — 부분 경로 여럿이 살아 있다', () => {
    const ctx = makeRecorder();
    drawElement(
      ctx,
      pathElement([
        { c: 'M', x: 0, y: 0 },
        { c: 'L', x: 5000, y: 5000 },
        { c: 'Z' },
        { c: 'M', x: 6000, y: 6000 },
        { c: 'L', x: PATH_LOCAL_EXTENT, y: PATH_LOCAL_EXTENT },
        { c: 'Z' },
      ]),
      FILLED,
      undefined,
      PROJ,
    );
    expect(names(ctx).filter((n) => n === 'closePath')).toHaveLength(2);
    // 경로는 **한 번만** 시작한다 — 부분 경로마다 `beginPath` 를 부르면 앞의 것이 지워진다.
    expect(names(ctx).filter((n) => n === 'beginPath')).toHaveLength(1);
  });
});

describe('drawElement — 001 의 칠하기 규율이 경로에도 그대로 걸린다', () => {
  it('`fill` 이 없으면 `fill()` 이 불리지 않는다 — 기본 색을 지어내지 않는다', () => {
    const ctx = makeRecorder();
    drawElement(ctx, pathElement(RIGHT_TRIANGLE), STROKED, undefined, PROJ);
    expect(names(ctx)).not.toContain('fill');
  });

  it('`stroke` 가 없거나 두께가 0 이면 `stroke()` 가 불리지 않는다', () => {
    for (const style of [FILLED, { fill: '#fff', stroke: '#000', strokeWidth: 0 }]) {
      const ctx = makeRecorder();
      drawElement(ctx, pathElement(RIGHT_TRIANGLE), style, undefined, PROJ);
      expect(names(ctx)).not.toContain('stroke');
    }
  });

  it('`visible: false` 는 `save`/`restore` 조차 하지 않는다', () => {
    const ctx = makeRecorder();
    drawElement(ctx, pathElement(RIGHT_TRIANGLE), { ...FILLED, visible: false }, undefined, PROJ);
    expect(ctx.calls).toEqual([]);
  });

  it('라벨은 상자의 **중심**에 붙는다 — 좌상단이 아니다', () => {
    const ctx = makeRecorder();
    drawElement(ctx, pathElement(RIGHT_TRIANGLE), { ...FILLED, textColor: '#111' }, 'ON', PROJ);
    // 중심은 (58.5, 53). `labelAnchor` 가 `default:` 로 떨어지면 (18.5, 30.5) 가 나온다.
    expect(ctx.calls.find((c) => c[0] === 'fillText')).toEqual(['fillText', 'ON', 58.5, 53]);
  });

  it('`catalog_id` 를 지워도 그림이 완전하다 — 렌더가 그 값을 읽지 않는다', () => {
    const withId = makeRecorder();
    const without = makeRecorder();
    drawElement(withId, pathElement(RIGHT_TRIANGLE, { catalog_id: 'star5' }), FILLED, undefined, PROJ);
    drawElement(without, pathElement(RIGHT_TRIANGLE), FILLED, undefined, PROJ);
    expect(without.calls).toEqual(withId.calls);
  });

  it('drawElements 는 경로를 배열 순서대로 그리고 글자 폭 장부에 담지 않는다', () => {
    const ctx = makeRecorder();
    const widths = drawElements(
      ctx,
      [pathElement(RIGHT_TRIANGLE), pathElement(ARROW_CURVED, { id: 'p2' })],
      {},
      {},
      PROJ,
    );
    // 경로에는 잡을 수 있는 글자 상자가 없다 — 담기면 히트 테스트가 잘못 읽는다.
    expect(widths).toEqual({});
    expect(names(ctx).filter((n) => n === 'beginPath')).toHaveLength(2);
  });
});

describe('DrawContext2D 는 정확히 둘 늘었다 (AC-E2 · 불변식 J1)', () => {
  /** 인터페이스 본문에서 주석을 걷어낸 코드만 남긴다. */
  function interfaceBody(): string {
    const source = readFileSync(join(__dirname, 'drawElement.ts'), 'utf-8');
    const start = source.indexOf('export interface DrawContext2D {');
    expect(start, 'DrawContext2D 선언을 찾지 못했다').toBeGreaterThanOrEqual(0);
    const end = source.indexOf('\n}', start);
    return source
      .slice(start, end)
      .split('\n')
      .filter((line) => !/^\s*(\/\/|\*|\/\*)/.test(line))
      .join('\n');
  }

  it('멤버 목록이 **008 이전 + closePath + bezierCurveTo** 와 정확히 같다', () => {
    const members = interfaceBody()
      .split('\n')
      .map((line) => /^\s{2}([A-Za-z][A-Za-z0-9]*)\s*[(:]/.exec(line)?.[1])
      .filter((name): name is string => name !== undefined);
    expect([...members].sort()).toEqual(
      [
        // 008 이전의 21
        'save',
        'restore',
        'setTransform',
        'beginPath',
        'rect',
        'ellipse',
        'moveTo',
        'lineTo',
        'stroke',
        'fill',
        'fillText',
        'measureText',
        'clearRect',
        'fillRect',
        'fillStyle',
        'strokeStyle',
        'lineWidth',
        'globalAlpha',
        'font',
        'textAlign',
        'textBaseline',
        // 008 이 더한 둘
        'closePath',
        'bezierCurveTo',
      ].sort(),
    );
  });

  it('비동기·콜백·이미지·폰트를 타입에 담은 멤버가 하나도 없다 (형상 판정)', () => {
    // 이름 열거가 아니다 — 이름 열거는 개명 한 번에 무장 해제된다. `drawImage` 를 더하면
    // `Image` 가, `loadFont(): Promise<void>` 를 더하면 `Promise` 가 여기서 걸린다.
    expect(interfaceBody()).not.toMatch(/Promise|=>|Image|Font|await/);
  });
});
