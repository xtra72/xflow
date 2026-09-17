// 파선이 실제로 그려지는가 (SPEC-CANVAS-012 M3 · AC-13~AC-18).
//
// ## 이 파일이 제 스텁을 따로 두는 이유
//
// `setLineDash` 는 `DrawContext2D` 의 **선택적** 멤버다(012 §결정 2). 선택적인 것의 대가는
// "구현하지 않은 스텁에서 파선이 조용히 지나간다" 이고, 그래서 이 파일의 스텁은 그 멤버를
// **갖춘다.** 갖추지 않으면 아래 단언이 전부 `undefined` 를 보고도 초록이 된다.
//
// 그 사실 자체를 첫 시험이 못박는다 — 훗날 누군가 스텁에서 그 줄을 지우면, 파선이 아니라
// **파선을 재는 일**이 먼저 죽기 때문이다.
//
// 고정 입력은 기본값 모양이 아니다(시험 규율 D2·D3): 두께 3 은 기본 1 이 아니고, 무늬가
// 두께를 타는지 보려면 1 이면 안 된다(`[3, 2]` 와 `[3w, 2w]` 가 구분되지 않는다).
//
// @spec SPEC-CANVAS-012 REQ-04

import { describe, expect, it } from 'vitest';

import type { CanvasElement } from './canvasConfig';
import type { CanvasProjection } from './canvasGeometry';
import { drawElements, type DrawContext2D } from './drawElement';
import type { ConnectorElement } from './connector/connectorTypes';
import type { CanvasNode } from './group/groupTypes';
import { dashPattern } from './strokeDash';

// --- 기록 스텁 -------------------------------------------------------------

type Recorded = [string, ...unknown[]];

interface Recorder extends DrawContext2D {
  readonly calls: Recorded[];
}

function makeRecorder(): Recorder {
  const calls: Recorded[] = [];
  const rec =
    (op: string) =>
    (...args: unknown[]): void => {
      calls.push([op, ...args]);
    };
  let fillStyle: string | CanvasGradient | CanvasPattern = '';
  let strokeStyle: string | CanvasGradient | CanvasPattern = '';
  let lineWidth = 1;
  let globalAlpha = 1;
  let font = '';
  let textAlign: CanvasTextAlign = 'left';
  let textBaseline: CanvasTextBaseline = 'alphabetic';

  return {
    calls,
    save: rec('save'),
    restore: rec('restore'),
    setTransform: rec('setTransform'),
    beginPath: rec('beginPath'),
    rect: rec('rect'),
    ellipse: rec('ellipse'),
    moveTo: rec('moveTo'),
    lineTo: rec('lineTo'),
    closePath: rec('closePath'),
    bezierCurveTo: rec('bezierCurveTo'),
    // **이 한 줄이 이 파일의 전제다.** 지우면 아래 단언이 전부 재는 척만 한다.
    setLineDash: rec('setLineDash'),
    stroke: rec('stroke'),
    fill: rec('fill'),
    fillText: rec('fillText'),
    clearRect: rec('clearRect'),
    fillRect: rec('fillRect'),
    measureText: (text: string) => ({ width: text.length * 10 }),
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

/** 주어진 이름의 호출을 모두 뽑는다(인자 포함). */
function allOf(ctx: Recorder, op: string): unknown[][] {
  return ctx.calls.filter((c) => c[0] === op).map((c) => c.slice(1));
}

/** 호출 이름만 뽑는다(순서 단언용). */
function ops(ctx: Recorder): string[] {
  return ctx.calls.map((c) => c[0]);
}

// --- 고정 입력 -------------------------------------------------------------

/** 두 축의 축척이 서로 다르다(0.8 과 0.5) — 한 축만 맞는 결함을 감추지 않는다. */
const PROJ: CanvasProjection = {
  stage: { width: 400, height: 200 },
  canvas: { width: 500, height: 400 },
};

/** 기본 두께(1)가 아니다 — 무늬가 두께를 타는지 보려면 1 이면 안 된다. */
const WIDTH = 3;

function rect(id: string, style: CanvasElement['style']): CanvasElement {
  return { id, kind: 'rect', style, geometry: { x: 31, y: 57, w: 140, h: 90 } };
}

function link(style: ConnectorElement['style']): ConnectorElement {
  return {
    id: 'c1',
    kind: 'connector',
    from: { x: 10, y: 20 },
    to: { x: 300, y: 250 },
    route: 'straight',
    style,
  };
}

function draw(nodes: readonly CanvasNode[]): Recorder {
  const ctx = makeRecorder();
  drawElements(ctx, nodes, {}, {}, PROJ);
  return ctx;
}

// --- 스텁 전제 (AC-18) ------------------------------------------------------

describe('스텁이 그 멤버를 갖춘다 (AC-18)', () => {
  it('`setLineDash` 를 기록한다 — 없으면 아래 단언이 전부 재는 척만 한다', () => {
    const ctx = makeRecorder();
    expect(typeof ctx.setLineDash).toBe('function');
  });

  it('인터페이스에서는 **선택적**이다 (AC-13)', () => {
    // 선택적이므로 그 멤버 **없이도** 구조적으로 만족한다 — 출시된 스텁 공장 아홉이
    // 컴파일되는 근거이고, 그것이 §결정 2 가 산 것의 전부다. 이 대입이 곧 그 증명이다.
    const withoutDash: DrawContext2D = makeRecorderWithoutDash();
    expect('setLineDash' in withoutDash).toBe(false);
  });
});

/** `setLineDash` 를 **빼고** 만든 스텁 — 선택성의 증거다. */
function makeRecorderWithoutDash(): DrawContext2D {
  const full = makeRecorder();
  const { setLineDash: _omitted, ...rest } = full;
  void _omitted;
  return rest;
}

// --- 무늬 산술 (AC-14 · AC-15) ---------------------------------------------

describe('무늬가 두께를 탄다 (AC-14)', () => {
  it('`dash` · 두께 4 에서 [12, 8] 이 걸린다', () => {
    const ctx = draw([rect('el-1', { stroke: '#f0f', strokeWidth: 4, strokeDash: 'dash' })]);
    expect(allOf(ctx, 'setLineDash')).toEqual([[[12, 8]]]);
  });

  it.each(['dash', 'dot', 'dashDot'] as const)(
    '`%s` 가 잎 모듈의 표와 **같은 배열**을 건다',
    (name) => {
      // 시험이 제 나름의 산술을 두지 않는다 — 두면 표가 바뀔 때 둘이 갈라지고, 그때
      // 어느 쪽이 참인지 화면이 답하지 못한다.
      const ctx = draw([rect('el-1', { stroke: '#f0f', strokeWidth: WIDTH, strokeDash: name })]);
      expect(allOf(ctx, 'setLineDash')).toEqual([[dashPattern(name, WIDTH)]]);
    },
  );

  it('연결선도 **같은 한 자리**를 지난다 (§결정 6)', () => {
    const ctx = draw([link({ stroke: '#0ff', strokeWidth: WIDTH, strokeDash: 'dot' })]);
    expect(allOf(ctx, 'setLineDash')).toEqual([[dashPattern('dot', WIDTH)]]);
  });
});

describe('실선 (AC-15)', () => {
  it.each([
    ['`solid`', 'solid' as const],
    ['미지정', undefined],
  ])('%s 는 빈 배열이다', (_label, dash) => {
    const ctx = draw([rect('el-1', { stroke: '#f0f', strokeWidth: WIDTH, strokeDash: dash })]);
    expect(allOf(ctx, 'setLineDash')).toEqual([[[]]]);
  });
});

// --- 새지 않는다 (AC-16 · AC-17) -------------------------------------------

describe('무늬가 요소 사이로 새지 않는다 (AC-16)', () => {
  it('파선 다음 실선은 빈 배열을 받는다', () => {
    const ctx = draw([
      rect('el-1', { stroke: '#f0f', strokeWidth: WIDTH, strokeDash: 'dash' }),
      rect('el-2', { stroke: '#0f0', strokeWidth: WIDTH }),
    ]);
    expect(allOf(ctx, 'setLineDash')).toEqual([[dashPattern('dash', WIDTH)], [[]]]);
  });

  it('무늬는 `save`/`restore` **안**에서 걸린다 — canvas 상태의 일부다', () => {
    const ctx = draw([rect('el-1', { stroke: '#f0f', strokeWidth: WIDTH, strokeDash: 'dash' })]);
    const names = ops(ctx);
    const save = names.indexOf('save');
    const dash = names.indexOf('setLineDash');
    const restore = names.lastIndexOf('restore');
    expect(save).toBeGreaterThanOrEqual(0);
    expect(dash).toBeGreaterThan(save);
    expect(dash).toBeLessThan(restore);
  });
});

describe('칠하지 않을 선에는 무늬도 걸지 않는다 (AC-17)', () => {
  it('선 색이 없으면 아무것도 걸지 않는다', () => {
    const ctx = draw([rect('el-1', { fill: '#111', strokeWidth: WIDTH, strokeDash: 'dash' })]);
    expect(allOf(ctx, 'setLineDash')).toEqual([]);
    expect(ops(ctx)).not.toContain('stroke');
  });

  it('두께가 0 이면 아무것도 걸지 않는다', () => {
    const ctx = draw([rect('el-1', { stroke: '#f0f', strokeWidth: 0, strokeDash: 'dash' })]);
    expect(allOf(ctx, 'setLineDash')).toEqual([]);
    expect(ops(ctx)).not.toContain('stroke');
  });
});
