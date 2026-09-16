// 돌아간 것이 그려지고 **같은 자리에서 잡힌다** (SPEC-CANVAS-014 M3~M5 · AC-10~AC-18).
//
// ## 이 파일이 막는 실패
//
// 그려진 자리와 잡히는 자리가 갈라지면 사용자는 **보이는 도형을 눌러도 잡지 못하고 빈 곳을
// 눌러 엉뚱한 것을 잡는다** — 002 가 위험 R1 로 이름 적어 둔 그것이며, 014 에서 가장 나쁜
// 실패다. 그래서 잉크 위 점이 잡히는 것(AC-15)과 **빈 모서리가 안 잡히는 것**(AC-16)을
// 짝으로 잰다. 앞의 것만 있으면 "축-나란 상자로 잡는" 구현이 초록으로 지나간다.
//
// ## 고정 입력이 비대칭이고 각도가 비직각이다
//
// 대칭 도형과 90° 배수만으로는 **역회전의 부호 결함이 통째로 지나간다**(AC-18). 긴 막대와
// 37° 를 함께 쓴다.
//
// @spec SPEC-CANVAS-014 REQ-01 · REQ-03

import { describe, expect, it } from 'vitest';

import type { CanvasElement, CanvasSize } from './canvasConfig';
import type { CanvasProjection, PxPoint } from './canvasGeometry';
import { drawElements, type DrawContext2D } from './drawElement';
import { hitTest } from './canvasHitTest';
import type { CanvasNode, GroupElement } from './group/groupTypes';
import { rotatePoint, rotatedAabb } from './canvasRotation';

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
    // **이 두 줄이 이 파일의 전제다** (AC-11). 지우면 아래 단언이 전부 재는 척만 한다.
    translate: rec('translate'),
    rotate: rec('rotate'),
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
    },
    get strokeStyle() {
      return strokeStyle;
    },
    set strokeStyle(v) {
      strokeStyle = v;
    },
    get lineWidth() {
      return lineWidth;
    },
    set lineWidth(v) {
      lineWidth = v;
    },
    get globalAlpha() {
      return globalAlpha;
    },
    set globalAlpha(v) {
      globalAlpha = v;
    },
    get font() {
      return font;
    },
    set font(v) {
      font = v;
    },
    get textAlign() {
      return textAlign;
    },
    set textAlign(v) {
      textAlign = v;
    },
    get textBaseline() {
      return textBaseline;
    },
    set textBaseline(v) {
      textBaseline = v;
    },
  };
}

function ops(ctx: Recorder): string[] {
  return ctx.calls.map((c) => c[0]);
}

function allOf(ctx: Recorder, op: string): unknown[][] {
  return ctx.calls.filter((c) => c[0] === op).map((c) => c.slice(1));
}

// --- 고정 입력 -------------------------------------------------------------

const CANVAS: CanvasSize = { width: 400, height: 400 };
/** 축척이 하나다(A1) — 두 축 모두 1.0. 각도가 투영을 지나 보존된다. */
const PROJ: CanvasProjection = { stage: { width: 400, height: 400 }, canvas: CANVAS };

/** **긴 막대**다. 정사각형은 회전 결함을 통째로 감춘다. */
const BAR = { x: 100, y: 190, w: 200, h: 20 } as const;
/** **비직각**이다. 90° 배수는 부호 결함을 감춘다. */
const DEG = 37;

function bar(rotation?: number): CanvasElement {
  return {
    id: 'b1',
    kind: 'rect',
    style: { fill: '#112233' },
    geometry: { ...BAR },
    ...(rotation !== undefined ? { rotation } : {}),
  } as CanvasElement;
}

function draw(nodes: readonly CanvasNode[]): Recorder {
  const ctx = makeRecorder();
  drawElements(ctx, nodes, {}, {}, PROJ);
  return ctx;
}

const center: PxPoint = { x: BAR.x + BAR.w / 2, y: BAR.y + BAR.h / 2 };

// --- ① 인터페이스와 스텁 (AC-10 · AC-11) ------------------------------------

describe('인터페이스 (AC-10 · AC-11)', () => {
  it('`translate`·`rotate` 가 **선택적**이다', () => {
    // 선택적이므로 그 둘 **없이도** 구조적으로 만족한다 — 출시된 스텁 공장 아홉이
    // 컴파일되는 근거이고, 그것이 §결정 5 가 산 것의 전부다.
    const full = makeRecorder();
    const { translate: _t, rotate: _r, ...rest } = full;
    void _t;
    void _r;
    const without: DrawContext2D = rest;
    expect('translate' in without).toBe(false);
    expect('rotate' in without).toBe(false);
  });

  it('이 파일의 스텁은 그 둘을 갖춘다', () => {
    const ctx = makeRecorder();
    expect(typeof ctx.translate).toBe('function');
    expect(typeof ctx.rotate).toBe('function');
  });
});

// --- ② 그리기 (AC-12 · AC-13 · AC-14) ---------------------------------------

describe('그리기 (AC-12 · AC-13 · AC-14)', () => {
  it('축이 상자 가운데이고 차례가 `translate → rotate → translate` 다 (AC-12)', () => {
    const ctx = draw([bar(90)]);
    expect(allOf(ctx, 'translate')).toEqual([
      [center.x, center.y],
      [-center.x, -center.y],
    ]);
    const names = ops(ctx);
    const t1 = names.indexOf('translate');
    const r = names.indexOf('rotate');
    const t2 = names.lastIndexOf('translate');
    expect(t1).toBeLessThan(r);
    expect(r).toBeLessThan(t2);
    // 라디안으로 들어간다.
    expect(allOf(ctx, 'rotate')).toEqual([[Math.PI / 2]]);
  });

  it('각도 0 이면 회전 호출이 **하나도 없다** (AC-13)', () => {
    // 014 이전의 호출 기록이 바이트 동일해야 한다 — 출시된 렌더 시험 다수가 그 기록을 센다.
    const ctx = draw([bar()]);
    expect(ops(ctx)).not.toContain('translate');
    expect(ops(ctx)).not.toContain('rotate');
  });

  it('각도가 요소 사이로 새지 않는다 (AC-14)', () => {
    const second: CanvasElement = {
      id: 'b2',
      kind: 'rect',
      style: { fill: '#445566' },
      geometry: { x: 0, y: 0, w: 10, h: 10 },
    };
    const ctx = draw([bar(DEG), second]);
    // 회전은 첫 요소의 `save`/`restore` 안에서만 일어난다.
    const names = ops(ctx);
    const lastRotate = names.lastIndexOf('rotate');
    const secondRect = names.lastIndexOf('rect');
    expect(lastRotate).toBeLessThan(secondRect);
    expect(allOf(ctx, 'rotate')).toHaveLength(1);
  });

  it('회전은 `save`/`restore` **안**에서 일어난다', () => {
    const ctx = draw([bar(DEG)]);
    const names = ops(ctx);
    expect(names.indexOf('save')).toBeLessThan(names.indexOf('rotate'));
    expect(names.indexOf('rotate')).toBeLessThan(names.lastIndexOf('restore'));
  });
});

describe('돌아간 그룹은 부품 묶음 **전체**를 감싼다', () => {
  function group(rotation?: number): GroupElement {
    return {
      id: 'g1',
      kind: 'group',
      geometry: { x: 50, y: 50, w: 100, h: 100 },
      parts: [
        { id: 'a', kind: 'rect', style: { fill: '#111' }, geometry: { x: 0, y: 0, w: 3000, h: 2000 } },
        { id: 'b', kind: 'rect', style: { fill: '#222' }, geometry: { x: 5000, y: 0, w: 3000, h: 2000 } },
      ],
      ...(rotation !== undefined ? { rotation } : {}),
    };
  }

  it('부품이 둘이어도 회전은 **한 번**이다', () => {
    // 부품마다 걸면 부품이 저마다 제 가운데를 축으로 돌아 그룹이 흩어진다.
    const ctx = draw([group(DEG)]);
    expect(allOf(ctx, 'rotate')).toHaveLength(1);
    // 축은 그룹 상자의 가운데다.
    expect(allOf(ctx, 'translate')[0]).toEqual([100, 100]);
  });

  it('그룹이 끝나면 닫힌다 — 다음 요소가 남의 각도로 그려지지 않는다', () => {
    const after: CanvasElement = {
      id: 'z',
      kind: 'rect',
      style: { fill: '#999' },
      geometry: { x: 300, y: 300, w: 10, h: 10 },
    };
    const ctx = draw([group(DEG), after]);
    const names = ops(ctx);
    // 마지막 `rect`(바깥 요소)가 그려지기 전에 `restore` 가 났다.
    expect(names.lastIndexOf('restore')).toBeGreaterThan(names.lastIndexOf('rect'));
    expect(allOf(ctx, 'rotate')).toHaveLength(1);
  });

  it('각도 0 인 그룹은 회전 호출이 없다', () => {
    const ctx = draw([group()]);
    expect(ops(ctx)).not.toContain('rotate');
  });
});

// --- ③ 보이는 자리 = 잡히는 자리 (AC-15 · AC-16 · AC-18 · K1) ---------------

describe('보이는 자리에서 잡힌다 (AC-15 · AC-16 · REQ-03)', () => {
  const nodes = [bar(DEG)];

  /** 돌지 않은 좌표계의 점을 **보이는 자리**로 옮긴다(그리는 쪽이 하는 그 변환이다). */
  const seen = (p: PxPoint): PxPoint => rotatePoint(p, center, DEG);

  it('잉크 위의 점이 잡힌다 (AC-15)', () => {
    // 막대의 한쪽 끝 근처 — 돌기 전 좌표로 (120, 200) 은 확실히 잉크 안이다.
    const p = seen({ x: 120, y: 200 });
    expect(hitTest(nodes, p, PROJ, {})).toEqual({ nodeId: 'b1' });
  });

  it('돌아간 상자의 **빈 모서리**는 잡히지 않는다 (AC-16)', () => {
    // 축-나란 상자 **안**이지만 잉크 **밖**인 자리다. 이 짝이 REQ-03 의 전부다 — 위
    // 단언만 있으면 "축-나란 상자로 잡는" 구현이 초록으로 지나간다.
    //
    // 모서리를 손으로 적지 않고 `rotatedAabb` 에서 구한다. 손으로 적으면 각도나 상자를
    // 고칠 때 그 수가 조용히 낡아 **잉크 위의 점**이 되고, 그러면 이 시험이 반대의 것을
    // 재면서 초록으로 남는다(첫 판에서 실제로 그랬다).
    const aabb = rotatedAabb({ x: BAR.x, y: BAR.y, w: BAR.w, h: BAR.h }, DEG);
    const corner: PxPoint = { x: aabb.x + aabb.w, y: aabb.y };
    // 그 점이 정말 축-나란 상자 안이다(경계 위다).
    expect(corner.x).toBeLessThanOrEqual(aabb.x + aabb.w + 1e-9);
    expect(corner.y).toBeGreaterThanOrEqual(aabb.y - 1e-9);
    expect(hitTest(nodes, corner, PROJ, {})).toBeUndefined();

    // **그리고 그 자리는 돌지 않았다면 잡혔을 자리다** — 이 짝단언이 없으면 위 단언이
    // "아무 데서나 안 잡힌다" 와 구분되지 않는다.
    expect(hitTest([bar()], { x: 150, y: 200 }, PROJ, {})).toEqual({ nodeId: 'b1' });
  });

  it('돌기 전에는 잡히던 자리가 돌고 나면 안 잡힌다', () => {
    // 막대 오른쪽 끝(290, 200)은 돌지 않았으면 잉크다. 37° 돌면 그 자리는 비어야 한다.
    const p: PxPoint = { x: 290, y: 200 };
    expect(hitTest([bar()], p, PROJ, {})).toEqual({ nodeId: 'b1' });
    expect(hitTest([bar(DEG)], p, PROJ, {})).toBeUndefined();
  });

  it('**역회전의 부호가 맞다** (AC-18)', () => {
    // 부호가 뒤집히면 잉크가 반대쪽으로 간 것처럼 읽힌다. 비대칭(긴 막대)과 비직각(37°)
    // 이라 그 결함이 여기서 드러난다 — 정사각형이나 90° 였으면 통째로 지나간다.
    const inkRight = seen({ x: 280, y: 200 });
    const wrongSign = rotatePoint({ x: 280, y: 200 }, center, -DEG);
    expect(hitTest(nodes, inkRight, PROJ, {})).toEqual({ nodeId: 'b1' });
    expect(hitTest(nodes, wrongSign, PROJ, {})).toBeUndefined();
  });

  it('돌아간 그룹의 부품이 보이는 자리에서 잡힌다', () => {
    const g: GroupElement = {
      id: 'g1',
      kind: 'group',
      geometry: { x: 100, y: 100, w: 200, h: 40 },
      rotation: DEG,
      parts: [
        { id: 'a', kind: 'rect', style: { fill: '#111' }, geometry: { x: 0, y: 0, w: 10000, h: 10000 } },
      ],
    };
    const gCenter: PxPoint = { x: 200, y: 120 };
    const p = rotatePoint({ x: 130, y: 115 }, gCenter, DEG);
    expect(hitTest([g], p, PROJ, {})).toEqual({ nodeId: 'g1', partId: 'a' });
  });
});
