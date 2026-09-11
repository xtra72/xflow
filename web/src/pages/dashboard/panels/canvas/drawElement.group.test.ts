// 그룹 렌더 — 2단 순회와 상자 안 투영 (SPEC-CANVAS-004 M3).
//
// 001 의 `DrawContext2D` 기록 스텁을 그대로 쓴다(jsdom canvas 없이 그리기 경로 전량을
// 관찰한다). **그리기 성질은 그리기 기록으로** 잰다 — 히트로 재면 엉뚱한 자로 재는 것이다.
//
// 고정 입력은 시험 규율이 요구하는 형상이다:
//   - **E-A** 부품 넷, 서로 다른 상자, 하나는 가장자리에 닿지 않는다.
//   - **E-B** 부품마다 다른 상자(전부 같으면 축척 결함이 보이지 않는다).
//   - **E-C** 그룹 상자 `317 × 181`(EXTENT 를 나누어떨어뜨리지 않는다).
//   - **E-D** 원점 `(73, 41)` — 두 축이 다르고 0 이 아니다.
//   - **E-L** 문구 부품이 하나 있다(넷째 프레임 표면이 자지 않도록).
//
// @spec SPEC-CANVAS-004 REQ-03 · AC-04 · AC-E1 · AC-E4

import { describe, expect, it } from 'vitest';

import type { CanvasElement } from './canvasConfig';
import { projectBox, projectBoxIn, projectPointIn, type CanvasProjection } from './canvasGeometry';
import { drawElements, type DrawContext2D } from './drawElement';
import { GROUP_LOCAL_EXTENT, type CanvasNode, type GroupElement } from './group/groupTypes';

// --- 기록 스텁 -------------------------------------------------------------

const CHAR_WIDTH = 10;

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
    stroke: rec('stroke'),
    fill: rec('fill'),
    fillText: rec('fillText'),
    clearRect: rec('clearRect'),
    fillRect: rec('fillRect'),
    measureText: (text: string) => ({ width: text.length * CHAR_WIDTH }),
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    globalAlpha: 1,
    font: '',
    textAlign: 'left' as CanvasTextAlign,
    textBaseline: 'alphabetic' as CanvasTextBaseline,
  };
}

// --- 고정 입력 -------------------------------------------------------------

/** 스테이지 400×200 · 캔버스 500×400 — 두 축의 축척이 서로 다르다(0.8 과 0.5). */
const PROJ: CanvasProjection = {
  stage: { width: 400, height: 200 },
  canvas: { width: 500, height: 400 },
};

const GROUP_GEO = { x: 73, y: 41, w: 317, h: 181 } as const;

function parts(): CanvasElement[] {
  return [
    // 가장자리에 닿는다.
    { id: 'body', kind: 'rect', style: { fill: '#111' }, geometry: { x: 0, y: 0, w: 3000, h: 2000 } },
    // **안쪽에 떠 있다** — 어느 좌표도 0 도 EXTENT 도 아니다(E-A).
    {
      id: 'stem',
      kind: 'ellipse',
      style: { fill: '#222' },
      geometry: { x: 4100, y: 3300, w: 1700, h: 900 },
    },
    {
      id: 'pipe',
      kind: 'line',
      style: { stroke: '#333', strokeWidth: 2 },
      geometry: { x1: 1200, y1: 7400, x2: 8800, y2: 6100 },
    },
    // 문구 부품(E-L).
    {
      id: 'label',
      kind: 'text',
      style: { fill: '#444' },
      geometry: { x: 5000, y: 9200 },
      text: 'abcd',
    },
  ];
}

function group(over: Partial<GroupElement> = {}): GroupElement {
  return { id: 'grp-1', kind: 'group', geometry: { ...GROUP_GEO }, parts: parts(), ...over };
}

function rect(id: string): CanvasElement {
  return { id, kind: 'rect', style: { fill: '#aaa' }, geometry: { x: 0, y: 0, w: 10, h: 10 } };
}

function ops(ctx: Recorder): string[] {
  return ctx.calls.map((c) => c[0]);
}

// --- 2단 그리기 순서 (AC-E4) ----------------------------------------------

describe('그리기 순서는 2단이다 (AC-E4)', () => {
  it('rect-A → 부품들 → rect-B 순으로 그린다', () => {
    const ctx = makeRecorder();
    const nodes: CanvasNode[] = [
      rect('rect-A'),
      { ...group(), parts: [rect('p1'), rect('p2')] },
      rect('rect-B'),
    ];
    drawElements(ctx, nodes, {}, {}, PROJ);
    // `rect` 호출의 순서가 곧 그리기 순서다. 자리로 구분한다(부품은 그룹 상자 안이다).
    const boxes = ctx.calls.filter((c) => c[0] === 'rect').map((c) => Number(c[1]));
    expect(boxes).toHaveLength(4);
    // rect-A · rect-B 는 캔버스 원점 기준(0 → 0px), 부품 둘은 그룹 원점 기준이다.
    const groupPx = projectBox(GROUP_GEO, PROJ);
    expect(boxes[0]).toBe(0);
    expect(boxes[1]).toBeCloseTo(groupPx.x, 10);
    expect(boxes[2]).toBeCloseTo(groupPx.x, 10);
    expect(boxes[3]).toBe(0);
  });

  it('빈 그룹은 아무것도 그리지 않되 뒤 요소를 막지도 않는다 (AC-E1)', () => {
    const ctx = makeRecorder();
    drawElements(ctx, [group({ parts: [] }), rect('after')], {}, {}, PROJ);
    expect(ctx.calls.filter((c) => c[0] === 'rect')).toHaveLength(1);
    expect(ops(ctx)).toContain('restore');
  });
});

// --- 상자 안 투영 ----------------------------------------------------------

describe('부품은 그룹의 px 상자 안에 그려진다 (REQ-03)', () => {
  it('상자 부품이 `projectBoxIn` 의 자리에 선다', () => {
    const ctx = makeRecorder();
    drawElements(ctx, [group()], {}, {}, PROJ);
    const box = projectBox(GROUP_GEO, PROJ);
    const expected = projectBoxIn({ x: 0, y: 0, w: 3000, h: 2000 }, box);
    const call = ctx.calls.find((c) => c[0] === 'rect');
    expect(call?.slice(1)).toEqual([expected.x, expected.y, expected.w, expected.h]);
  });

  it('안쪽에 떠 있는 타원이 **원점과 축척을 둘 다** 지난다 (E-A · E-D)', () => {
    const ctx = makeRecorder();
    drawElements(ctx, [group()], {}, {}, PROJ);
    const box = projectBox(GROUP_GEO, PROJ);
    const inner = projectBoxIn({ x: 4100, y: 3300, w: 1700, h: 900 }, box);
    const call = ctx.calls.find((c) => c[0] === 'ellipse');
    expect(Number(call?.[1])).toBeCloseTo(inner.x + inner.w / 2, 10);
    expect(Number(call?.[2])).toBeCloseTo(inner.y + inner.h / 2, 10);
    // 원점을 잊은 구현은 여기서 그룹 원점만큼 어긋난다.
    expect(Number(call?.[1])).toBeGreaterThan(box.x);
  });

  it('선 부품의 두 끝점이 모두 상자 안이다', () => {
    const ctx = makeRecorder();
    drawElements(ctx, [group()], {}, {}, PROJ);
    const box = projectBox(GROUP_GEO, PROJ);
    const move = ctx.calls.find((c) => c[0] === 'moveTo');
    const line = ctx.calls.find((c) => c[0] === 'lineTo');
    for (const call of [move, line]) {
      expect(Number(call?.[1])).toBeGreaterThanOrEqual(box.x);
      expect(Number(call?.[1])).toBeLessThanOrEqual(box.x + box.w);
      expect(Number(call?.[2])).toBeGreaterThanOrEqual(box.y);
      expect(Number(call?.[2])).toBeLessThanOrEqual(box.y + box.h);
    }
  });

  it('문구 부품의 기준점도 상자 안이다 (E-L)', () => {
    const ctx = makeRecorder();
    drawElements(ctx, [group()], {}, {}, PROJ);
    const box = projectBox(GROUP_GEO, PROJ);
    const anchor = projectPointIn({ x: 5000, y: 9200 }, box);
    const call = ctx.calls.find((c) => c[0] === 'fillText');
    expect(call?.[1]).toBe('abcd');
    // 정렬이 left 라 원점이 곧 기준점이다.
    expect(Number(call?.[2])).toBeCloseTo(anchor.x, 10);
    expect(Number(call?.[3])).toBeCloseTo(anchor.y, 10);
  });

  it('경로 부품은 **두 겹의 로컬 격자**를 합성한다(섞지 않는다)', () => {
    const ctx = makeRecorder();
    const pathPart: CanvasElement = {
      id: 'ink',
      kind: 'path',
      style: { stroke: '#000', strokeWidth: 1 },
      geometry: { x: 2000, y: 2000, w: 4000, h: 4000 },
      path: [
        { c: 'M', x: 0, y: 0 },
        { c: 'L', x: GROUP_LOCAL_EXTENT, y: GROUP_LOCAL_EXTENT },
      ],
    };
    drawElements(ctx, [group({ parts: [pathPart] })], {}, {}, PROJ);
    const box = projectBox(GROUP_GEO, PROJ);
    const partBox = projectBoxIn(pathPart.geometry, box);
    const move = ctx.calls.find((c) => c[0] === 'moveTo');
    const line = ctx.calls.find((c) => c[0] === 'lineTo');
    // 명령 로컬 0 은 부품 상자의 좌상단, EXTENT 는 그 우하단이다.
    expect(Number(move?.[1])).toBeCloseTo(partBox.x, 10);
    expect(Number(move?.[2])).toBeCloseTo(partBox.y, 10);
    expect(Number(line?.[1])).toBeCloseTo(partBox.x + partBox.w, 10);
    expect(Number(line?.[2])).toBeCloseTo(partBox.y + partBox.h, 10);
  });

  it('비균등 그룹 상자에서 부품이 함께 일그러진다 (A7)', () => {
    const ctx = makeRecorder();
    const square: CanvasElement = {
      id: 'sq',
      kind: 'rect',
      style: { fill: '#000' },
      geometry: { x: 0, y: 0, w: 5000, h: 5000 },
    };
    drawElements(ctx, [group({ parts: [square] })], {}, {}, PROJ);
    const call = ctx.calls.find((c) => c[0] === 'rect');
    // 캔버스 단위로 정사각인 부품이 px 에서는 직사각형이다.
    expect(Number(call?.[3])).not.toBeCloseTo(Number(call?.[4]), 6);
  });
});

// --- 프레임 키 넷째 표면 (AC-04 · E-L) ------------------------------------

describe('글자 폭 장부가 복합 키를 쓴다 — 넷째 표면 (AC-04)', () => {
  it('부품 문구는 `그룹id/부품id`, 최상위 문구는 `el.id` 다', () => {
    const ctx = makeRecorder();
    const topText: CanvasElement = {
      id: 'txt-1',
      kind: 'text',
      style: { fill: '#000' },
      geometry: { x: 10, y: 10 },
      text: 'xy',
    };
    const widths = drawElements(ctx, [topText, group()], {}, {}, PROJ);
    expect(Object.keys(widths).sort()).toEqual(['grp-1/label', 'txt-1']);
    // 폴백이 아니라 **실측 폭**이다(문구 길이로 구분되는 값을 쓴다).
    expect(widths['grp-1/label']).toBe(4 * CHAR_WIDTH);
    expect(widths['txt-1']).toBe(2 * CHAR_WIDTH);
  });

  it('도형 부품의 **라벨 자리**도 상자 안이다 (`labelAnchorIn`)', () => {
    // 이 시험이 없으면 `labelAnchorIn` 이 원점을 잊어도, 아예 `labelAnchor` 로 되돌아가도
    // 아무 시험이 울지 않는다 — 라벨은 폭 장부에 담기지 않아 넷째 표면이 잡아 주지 못하고,
    // 문구 **부품**은 `labelAnchor` 가 아니라 `projectPointIn` 을 지나기 때문이다.
    const ctx = makeRecorder();
    const labelled: CanvasElement = {
      id: 'body',
      kind: 'rect',
      style: { textColor: '#000' },
      // 상자 가운데가 아니라 **오른쪽 아래 구석**에 둔다 — 가운데에 두면 상자 중심과
      // 그룹 중심이 겹쳐 원점 결함이 절반만 드러난다.
      geometry: { x: 6000, y: 7000, w: 2000, h: 1000 },
      text: 'ab',
    };
    drawElements(ctx, [group({ parts: [labelled] })], {}, {}, PROJ);
    const box = projectBox(GROUP_GEO, PROJ);
    const partBox = projectBoxIn(labelled.geometry, box);
    const call = ctx.calls.find((c) => c[0] === 'fillText');
    expect(call?.[1]).toBe('ab');
    // 라벨은 부품 상자의 **중심**에 앉고(001 의 `labelAnchor`), 정렬이 left 라 원점이
    // 곧 기준점이다.
    expect(Number(call?.[2])).toBeCloseTo(partBox.x + partBox.w / 2, 10);
    expect(Number(call?.[3])).toBeCloseTo(partBox.y + partBox.h / 2, 10);
    // 그룹 원점을 잊으면 여기서 상자 바깥으로 나간다.
    expect(Number(call?.[2])).toBeGreaterThan(box.x);
    expect(Number(call?.[3])).toBeGreaterThan(box.y);
  });

  it('도형 부품의 라벨 폭은 담기지 않는다 — 001 의 규율 그대로다', () => {
    const ctx = makeRecorder();
    const labelled: CanvasElement = {
      id: 'body',
      kind: 'rect',
      style: { textColor: '#000' },
      geometry: { x: 0, y: 0, w: 100, h: 100 },
      text: 'hello',
    };
    const widths = drawElements(ctx, [group({ parts: [labelled] })], {}, {}, PROJ);
    expect(widths).toEqual({});
  });

  it('같은 부품 id 를 가진 그룹 둘이 섞이지 않는다 (E-I)', () => {
    const ctx = makeRecorder();
    const text: CanvasElement = {
      id: 'label',
      kind: 'text',
      style: { fill: '#000' },
      geometry: { x: 0, y: 0 },
      text: 'ab',
    };
    const widths = drawElements(
      ctx,
      [
        { ...group({ parts: [text] }), id: 'grp-A' },
        { ...group({ parts: [{ ...text, text: 'abcdef' }] }), id: 'grp-B' },
      ],
      {},
      {},
      PROJ,
    );
    expect(widths).toEqual({ 'grp-A/label': 20, 'grp-B/label': 60 });
  });
});

// --- 상자 장부는 그룹마다 다시 잰다 (뮤테이션 M3-10 이 드러낸 자리) ----------

describe('나란한 그룹 둘은 **제 상자**를 쓴다', () => {
  it('두 그룹의 id 가 같아도 뒤 그룹의 부품이 앞 그룹의 상자로 그려지지 않는다', () => {
    // 파서는 최상위 id 중복을 걸러 내지만(먼저 온 것이 이긴다) 이 모듈은 손으로 지은
    // 배열도 받는다(미리보기). 상자 장부를 **id** 로 비교하면 여기서 뒤 그룹의 부품이
    // 앞 그룹의 상자 안에 앉고, 그 어긋남은 예외도 빈 화면도 아닌 조용한 어긋남이다.
    const ctx = makeRecorder();
    const only: CanvasElement = {
      id: 'body',
      kind: 'rect',
      style: { fill: '#111' },
      geometry: { x: 0, y: 0, w: GROUP_LOCAL_EXTENT, h: GROUP_LOCAL_EXTENT },
    };
    const first = { ...group({ parts: [only] }), id: 'dup' };
    const second = {
      ...group({ parts: [only] }),
      id: 'dup',
      geometry: { x: 200, y: 150, w: 133, h: 97 },
    };
    drawElements(ctx, [first, second], {}, {}, PROJ);

    const drawn = ctx.calls.filter((c) => c[0] === 'rect');
    expect(drawn).toHaveLength(2);
    const a = projectBox(first.geometry, PROJ);
    const b = projectBox(second.geometry, PROJ);
    // 두 상자가 서로 다르다는 것부터 잰다 — 같으면 이 시험이 아무것도 재지 않는다.
    expect([b.x, b.y, b.w, b.h]).not.toEqual([a.x, a.y, a.w, a.h]);
    expect(drawn[0]?.slice(1)).toEqual([a.x, a.y, a.w, a.h]);
    expect(drawn[1]?.slice(1)).toEqual([b.x, b.y, b.w, b.h]);
  });

  it('한 그룹의 부품들은 여전히 **같은 상자**를 본다', () => {
    const ctx = makeRecorder();
    const box = projectBox(GROUP_GEO, PROJ);
    drawElements(ctx, [group()], {}, {}, PROJ);
    const rects = ctx.calls.filter((c) => c[0] === 'rect');
    expect(rects).toHaveLength(1);
    expect(rects[0]?.slice(1)).toEqual([
      projectBoxIn(parts()[0]!.geometry as never, box).x,
      projectBoxIn(parts()[0]!.geometry as never, box).y,
      projectBoxIn(parts()[0]!.geometry as never, box).w,
      projectBoxIn(parts()[0]!.geometry as never, box).h,
    ]);
  });
});

describe('스타일·문구 맵도 복합 키로 읽는다 — 첫째·둘째 표면 (AC-04)', () => {
  it('부품 스타일이 `그룹id/부품id` 로 들어온다', () => {
    const ctx = makeRecorder();
    drawElements(ctx, [group()], { 'grp-1/body': { fill: '#f00' } }, {}, PROJ);
    expect(ctx.calls.some((c) => c[0] === 'fill')).toBe(true);
    // 평평한 키(`body`)로 들어온 스타일은 **무시된다** — 그래야 다른 그룹의 같은 이름
    // 부품이 서로의 스타일을 훔치지 않는다.
    const flat = makeRecorder();
    drawElements(flat, [group()], { body: { visible: false } }, {}, PROJ);
    expect(flat.calls.filter((c) => c[0] === 'rect')).toHaveLength(1);
  });

  it('부품 문구가 `그룹id/부품id` 로 들어온다', () => {
    const ctx = makeRecorder();
    drawElements(ctx, [group()], {}, { 'grp-1/label': 'replaced' }, PROJ);
    expect(ctx.calls.find((c) => c[0] === 'fillText')?.[1]).toBe('replaced');
  });

  it('최상위 원소는 002 와 **바이트 동일한** 키를 쓴다 (G11)', () => {
    const ctx = makeRecorder();
    const top = rect('rect-1');
    drawElements(ctx, [top], { 'rect-1': { visible: false } }, {}, PROJ);
    expect(ctx.calls.filter((c) => c[0] === 'rect')).toHaveLength(0);
  });
});

// --- 견고성 ---------------------------------------------------------------

describe('그룹 렌더가 예외를 던지지 않는다 (REQ-05)', () => {
  it('손상된 그룹·부품에서도 나머지를 계속 그린다', () => {
    const ctx = makeRecorder();
    const broken = {
      id: 'bad',
      kind: 'group',
      geometry: { x: Number.NaN, y: 0, w: Number.POSITIVE_INFINITY, h: 0 },
      parts: parts(),
    } as GroupElement;
    expect(() => drawElements(ctx, [broken, rect('after')], {}, {}, PROJ)).not.toThrow();
    const numbers = ctx.calls.flatMap((c) => c.slice(1)).filter((v) => typeof v === 'number');
    expect(numbers.filter((n) => Number.isNaN(n))).toEqual([]);
  });

  it('`DrawContext2D` 에 `drawImage` 를 더하지 않았다 — 기록 스텁이 그 증인이다', () => {
    const ctx = makeRecorder();
    expect('drawImage' in ctx).toBe(false);
    // 스텁에 없는 멤버를 렌더가 부르면 예외가 난다. 즉 이 시험이 초록인 것 자체가
    // "그룹 렌더가 새 context 멤버를 쓰지 않는다" 의 증거다.
    expect(() => drawElements(ctx, [group()], {}, {}, PROJ)).not.toThrow();
  });
});
