// 연결선 렌더 (SPEC-CANVAS-011 M6).
//
// ## 이 파일이 **가장 먼저** 서야 하는 이유
//
// M4 는 `walkDrawables` 가 `CanvasElement` 를 낸다는 사실 하나로 `drawElement.ts` 를
// 연결선으로부터 가려 두었다. 그 가림막이 컴파일러가 이 파일에 대해 할 수 있는 말의
// 전부였다 — 순회가 연결선을 내기 시작하는 순간 그 말이 사라진다. 그리기가 연결선에
// 대해 **아무것도 하지 않아도** 타입 오류도, 기존 단언의 실패도 없다.
//
// 그래서 M6 의 첫 줄은 구현이 아니라 이 단언이다: 연결선 하나를 그리면 **캔버스 호출이
// 실제로 난다.** 침묵이 초록으로 보이지 않게 하는 자가 이것 하나뿐이다.
//
// ## 그리기 성질은 **그리기 기록**으로 잰다
//
// 001 의 기록 스텁을 그대로 쓴다(jsdom canvas 없이 렌더 경로 전량을 관찰한다).
// 히트로 재면 엉뚱한 자로 재는 것이다.
//
// @spec SPEC-CANVAS-011 REQ-04 · AC-16 · AC-46~AC-52

import { describe, expect, it } from 'vitest';

import type { CanvasElement } from './canvasConfig';
import { projectPoint, type CanvasProjection } from './canvasGeometry';
import { drawElements, type DrawContext2D } from './drawElement';
import type { CanvasNode, GroupElement } from './group/groupTypes';
import type { ConnectorElement, ConnectorRoute } from './connector/connectorTypes';

// --- 기록 스텁 -------------------------------------------------------------

const CHAR_WIDTH = 10;

type Recorded = [string, ...unknown[]];

interface Recorder extends DrawContext2D {
  readonly calls: Recorded[];
}

/**
 * 속성 **대입도** 기록한다 — 001 의 스텁이 그러하듯이(`drawElement.test.ts`).
 *
 * 평범한 필드로 두면 `ctx.strokeStyle = ...` 이 기록에 남지 않고, 그러면 "색을 지어내지
 * 않는다" 도 "저술을 덮는다" 도 **재는 척만 하는 단언**이 된다. 네 갈래의 기록이 같은지
 * 보는 AC-50 도 이름만 비교하게 되어 "곡선만 색이 다르다" 를 놓친다.
 */
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
    stroke: rec('stroke'),
    fill: rec('fill'),
    fillText: rec('fillText'),
    clearRect: rec('clearRect'),
    fillRect: rec('fillRect'),
    measureText: (text: string) => ({ width: text.length * CHAR_WIDTH }),
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

/** 주어진 이름의 호출을 **모두** 뽑는다(인자 포함). */
function allOf(ctx: Recorder, op: string): unknown[][] {
  return ctx.calls.filter((c) => c[0] === op).map((c) => c.slice(1));
}

/**
 * 연결선만이 낼 수 있는 호출을 뽑는다.
 *
 * 아래 고정 입력의 도형 둘은 **채우기만** 하므로(`fill` 저술뿐) 이 이름들을 하나도 내지
 * 않는다. 그래서 이 목록이 비었다는 것은 곧 "연결선이 아무것도 그리지 않았다" 이며,
 * `ctx.calls` 전체를 비었다고 단언하는 것보다 정확하다 — 배열에 도형이 함께 있어야
 * 끝점이 풀리는지를 잴 수 있기 때문이다.
 */
function connectorOps(ctx: Recorder): string[] {
  const only = ['moveTo', 'lineTo', 'bezierCurveTo', 'stroke', 'strokeStyle', 'lineWidth'];
  return ops(ctx).filter((name) => only.includes(name));
}

// --- 고정 입력 -------------------------------------------------------------

/** 스테이지 400×200 · 캔버스 500×400 — 두 축의 축척이 서로 다르다(0.8 과 0.5). */
const PROJ: CanvasProjection = {
  stage: { width: 400, height: 200 },
  canvas: { width: 500, height: 400 },
};

/** 캔버스 좌표를 px 로 옮긴다 — 시험이 제 나름의 산술을 두지 않는다. */
function px(x: number, y: number): [number, number] {
  const p = projectPoint({ x, y }, PROJ);
  return [p.x, p.y];
}

const STROKE = { stroke: '#f0f', strokeWidth: 3 } as const;

/** 좌상 100×100 상자 — `e` 앵커가 캔버스 (100, 50) 이다. */
function boxA(): CanvasElement {
  return { id: 'A', kind: 'rect', style: { fill: '#111' }, geometry: { x: 0, y: 0, w: 100, h: 100 } };
}

/** 우하 100×100 상자 — `w` 앵커가 캔버스 (300, 250) 이다. */
function boxB(): CanvasElement {
  return {
    id: 'B',
    kind: 'rect',
    style: { fill: '#222' },
    geometry: { x: 300, y: 200, w: 100, h: 100 },
  };
}

function link(over: Partial<ConnectorElement> = {}): ConnectorElement {
  return {
    id: 'c1',
    kind: 'connector',
    from: { el: 'A', a: 'e' },
    to: { el: 'B', a: 'w' },
    route: 'straight',
    style: { ...STROKE },
    ...over,
  };
}

function draw(nodes: readonly CanvasNode[]): Recorder {
  const ctx = makeRecorder();
  drawElements(ctx, nodes, {}, {}, PROJ);
  return ctx;
}

// --- 침묵을 막는 자 ---------------------------------------------------------

describe('연결선은 **그려진다** — 순회가 낸다고 그림이 나는 것은 아니다', () => {
  it('두 도형을 잇는 연결선이 캔버스 호출을 낸다', () => {
    // M4 가 순회에서 연결선을 빼 두는 동안 `drawElement.ts` 는 연결선을 **볼 수 없었다**.
    // 그 가림막을 M6 이 걷어내는데, 걷어낸 자리에 그리기가 없어도 컴파일러는 침묵한다.
    // 이 단언이 그 침묵을 깬다 — 연결선만 있는 배열에서 잉크가 나야 한다.
    const ctx = draw([boxA(), boxB(), link()]);
    const [ax, ay] = px(100, 50);
    const [bx, by] = px(300, 250);

    expect(allOf(ctx, 'moveTo')).toContainEqual([ax, ay]);
    expect(allOf(ctx, 'lineTo')).toContainEqual([bx, by]);
    expect(ops(ctx)).toContain('stroke');
  });
});

// --- 네 갈래 (AC-46 · AC-47 · AC-50) ----------------------------------------

describe('`route` 는 **그리기만** 가른다', () => {
  it('AC-46: 중간점 없는 직선은 `moveTo` 한 번과 `lineTo` 한 번이다', () => {
    const ctx = draw([boxA(), boxB(), link()]);
    expect(allOf(ctx, 'moveTo')).toEqual([px(100, 50)]);
    expect(allOf(ctx, 'lineTo')).toEqual([px(300, 250)]);
    expect(ops(ctx)).not.toContain('bezierCurveTo');
  });

  it('AC-47: 중간점 둘인 꺾은 선은 `lineTo` 가 세 번이다', () => {
    const ctx = draw([
      boxA(),
      boxB(),
      link({ route: 'elbow', points: [{ x: 200, y: 50 }, { x: 200, y: 250 }] }),
    ]);
    expect(allOf(ctx, 'moveTo')).toEqual([px(100, 50)]);
    expect(allOf(ctx, 'lineTo')).toEqual([px(200, 50), px(200, 250), px(300, 250)]);
  });

  it('AC-50: 중간점이 없으면 네 갈래의 **호출 기록이 동일하다**', () => {
    // 속성 설정(`strokeStyle` · `lineWidth` · `globalAlpha`)까지 포함한 전체 기록을 잰다 —
    // 이름만 같고 값이 다르면 "곡선만 색이 다르다" 가 지나간다.
    const routes: ConnectorRoute[] = ['straight', 'elbow', 'curve', 'free'];
    const records = routes.map((route) => draw([boxA(), boxB(), link({ route })]).calls);
    const [first] = records;
    for (const record of records) expect(record).toEqual(first);
    // 그 동일한 기록이 **실제로 직선**인지도 함께 잰다(넷이 똑같이 아무것도 안 그려도
    // 위 단언은 통과한다).
    expect(first?.map((c) => c[0])).toContain('lineTo');
  });

  it('꺾은 선과 자유선은 같은 점에서 **같은 그림**이다 — 다른 것은 출처뿐이다', () => {
    const points = [{ x: 200, y: 50 }, { x: 200, y: 250 }];
    const elbow = draw([boxA(), boxB(), link({ route: 'elbow', points })]).calls;
    const free = draw([boxA(), boxB(), link({ route: 'free', points })]).calls;
    expect(free).toEqual(elbow);
  });
});

// --- 곡선 (AC-48 · AC-49) ---------------------------------------------------

describe('곡선은 **새 명령을 만들지 않는다**', () => {
  it('AC-48: 제어점이 `A + ⅔(P−A)` 와 `B + ⅔(P−B)` 다', () => {
    // 세 점 A-P-B 의 2차 베지어. 제어점 P 는 두 끝의 어느 쪽에도 치우치지 않게 골랐다 —
    // 대칭이면 두 제어점의 식이 뒤바뀌어도 시험이 통과한다.
    const ctx = draw([
      boxA(),
      boxB(),
      link({ route: 'curve', points: [{ x: 150, y: 350 }] }),
    ]);
    const [ax, ay] = px(100, 50);
    const [qx, qy] = px(150, 350);
    const [bx, by] = px(300, 250);
    const two = 2 / 3;

    expect(allOf(ctx, 'moveTo')).toEqual([[ax, ay]]);
    expect(allOf(ctx, 'bezierCurveTo')).toEqual([
      [
        ax + two * (qx - ax),
        ay + two * (qy - ay),
        bx + two * (qx - bx),
        by + two * (qy - by),
        bx,
        by,
      ],
    ]);
    // 2차를 3차가 **정확히** 표현하므로 조각이 하나뿐이고 `lineTo` 는 나지 않는다.
    expect(ops(ctx)).not.toContain('lineTo');
  });

  it('AC-49: `DrawContext2D` 에 `quadraticCurveTo` 가 없다', () => {
    // 007 이 가드로 고정한 표면이다. 스텁은 이 인터페이스를 **구조적으로** 만족하므로,
    // 2차 명령이 더해지는 순간 이 파일의 스텁이 컴파일되지 않는다 — 아래 단언은 그
    // 컴파일 시각의 사실을 런타임에서도 값으로 남긴다.
    const ctx: DrawContext2D = makeRecorder();
    expect('quadraticCurveTo' in ctx).toBe(false);
    for (const absent of ['quadraticCurveTo', 'arc', 'arcTo', 'setLineDash']) {
      expect(Object.keys(ctx)).not.toContain(absent);
    }
  });

  it('중간점 둘인 곡선은 이웃한 두 제어점의 **중점**에서 이어진다', () => {
    // SPEC 을 글자대로 읽으면 중간점마다 "양옆 두 점을 끝으로 하는" 2차라 구간이 겹치고
    // 선이 되돌아간다. 011 이 고른 일반화는 중간점을 전부 제어점으로 보고, 곡선 위의
    // 점을 이웃한 두 제어점의 중점으로 둔다 — 중간점 하나에서 AC-48 로 줄어든다.
    const m1 = { x: 150, y: 350 };
    const m2 = { x: 260, y: 40 };
    const ctx = draw([boxA(), boxB(), link({ route: 'curve', points: [m1, m2] })]);
    const segs = allOf(ctx, 'bezierCurveTo');
    expect(segs).toHaveLength(2);
    const joint = px((m1.x + m2.x) / 2, (m1.y + m2.y) / 2);
    // 첫 조각은 중점에서 끝나고 둘째 조각은 끝점에서 끝난다 — 두 조각이 이어지고
    // 어느 구간도 두 번 그려지지 않는다.
    expect(segs[0]?.slice(4)).toEqual(joint);
    expect(segs[1]?.slice(4)).toEqual(px(300, 250));
    expect(ops(ctx)).not.toContain('lineTo');
  });
});

// --- 배열 자리 (AC-51) -----------------------------------------------------

describe('그리기 순서는 **배열 순서 하나뿐이다** (AC-51)', () => {
  it('연결선이 앞 도형 뒤 · 뒤 도형 앞에 그려진다', () => {
    const before: CanvasElement = {
      id: 'back',
      kind: 'rect',
      style: { fill: '#111' },
      geometry: { x: 0, y: 0, w: 10, h: 10 },
    };
    const after: CanvasElement = {
      id: 'front',
      kind: 'ellipse',
      style: { fill: '#222' },
      geometry: { x: 0, y: 0, w: 10, h: 10 },
    };
    const ctx = draw([boxA(), boxB(), before, link(), after]);
    const names = ops(ctx);
    expect(names.indexOf('rect')).toBeLessThan(names.indexOf('stroke'));
    expect(names.indexOf('stroke')).toBeLessThan(names.indexOf('ellipse'));
  });

  it('배열 자리를 바꾸면 그림의 앞뒤가 **따라 바뀐다** — 별도 층이 없다', () => {
    const shape: CanvasElement = {
      id: 'z',
      kind: 'ellipse',
      style: { fill: '#333' },
      geometry: { x: 0, y: 0, w: 10, h: 10 },
    };
    const underneath = ops(draw([boxA(), boxB(), link(), shape]));
    const above = ops(draw([boxA(), boxB(), shape, link()]));
    expect(underneath.indexOf('stroke')).toBeLessThan(underneath.indexOf('ellipse'));
    expect(above.indexOf('ellipse')).toBeLessThan(above.lastIndexOf('stroke'));
  });

  it('그룹 부품의 연속을 **끊지 않는다** — 연결선은 그룹에 담기지 않는다 (A8)', () => {
    // 연결선이 그룹 바로 앞에 서도 부품 넷은 여전히 연달아 그려진다. 끊기면 그룹 상자
    // 캐시가 부품마다 다시 나고, 그 결함은 "어떤 부품만 자리가 다르다" 로만 보인다.
    const grp: GroupElement = {
      id: 'grp-1',
      kind: 'group',
      geometry: { x: 0, y: 0, w: 200, h: 200 },
      parts: [
        { id: 'p1', kind: 'rect', style: { fill: '#1' }, geometry: { x: 0, y: 0, w: 100, h: 100 } },
        { id: 'p2', kind: 'rect', style: { fill: '#2' }, geometry: { x: 0, y: 0, w: 100, h: 100 } },
      ],
    };
    const ctx = draw([boxA(), boxB(), link(), grp]);
    const names = ops(ctx);
    const rects = names.reduce<number[]>((at, name, i) => {
      if (name === 'rect') at.push(i);
      return at;
    }, []);
    // boxA · boxB · p1 · p2 — 뒤의 둘(부품)이 **붙어** 있다.
    expect(rects).toHaveLength(4);
    expect(rects[3]! - rects[2]!).toBe(rects[1]! - rects[0]!);
    // 그리고 그 둘은 연결선의 잉크 **뒤**에 온다.
    expect(names.indexOf('stroke')).toBeLessThan(rects[2]!);
  });
});

// --- 끊긴 연결 (AC-52) -----------------------------------------------------

describe('끊긴 연결은 **그려지지 않는다** (AC-52)', () => {
  it('가리킨 요소가 없으면 캔버스 호출이 하나도 나지 않는다', () => {
    // 연결선만 있는 배열이라 그림이 나면 그것은 전부 연결선의 것이다. 길이 0 인 선도,
    // 아무것도 따르지 않는 `beginPath` 도 아니다 — 부재는 그릴 수 없다.
    const ctx = draw([link()]);
    expect(ctx.calls).toEqual([]);
  });

  it('한쪽 끝만 풀려도 그리지 않는다 — 반쪽 선이 남지 않는다', () => {
    // 한쪽이 풀렸으니 "어딘가에 붙은 선" 을 그릴 수는 있다. 그리지 **않는** 것이 요점이다 —
    // 그런 선은 사용자가 긋지 않은 선이다.
    const ctx = draw([boxA(), link()]);
    expect(connectorOps(ctx)).toEqual([]);
  });

  it('없는 앵커 이름도 끊긴 연결이다 — 중심으로 떨어뜨리지 않는다', () => {
    const ctx = draw([boxA(), boxB(), link({ to: { el: 'B', a: 'no-such-anchor' } })]);
    expect(connectorOps(ctx)).toEqual([]);
  });

  it('예외가 나지 않고 **이웃은 그대로 그려진다**', () => {
    const shape: CanvasElement = {
      id: 'z',
      kind: 'ellipse',
      style: { fill: '#333' },
      geometry: { x: 0, y: 0, w: 10, h: 10 },
    };
    const ctx = draw([link({ from: { el: 'ghost', a: 'e' } }), shape]);
    expect(ops(ctx)).toContain('ellipse');
    expect(ops(ctx)).not.toContain('stroke');
  });
});

// --- 중심 앵커 (AC-16 · A12) -------------------------------------------------

describe('중심 앵커에 붙은 선은 **잘리지 않는다** (AC-16)', () => {
  it('끝점이 도형 중심이며 경계로 물러나지 않는다', () => {
    // M3 에서 미룬 단언이다 — 자르는가 아닌가는 **그리는** 성질이라 그릴 법이 서야 잰다.
    const ctx = draw([
      boxA(),
      boxB(),
      link({ from: { el: 'A', a: 'c' }, to: { el: 'B', a: 'c' } }),
    ]);
    // A 의 중심은 (50, 50), B 의 중심은 (350, 250) 이다.
    expect(allOf(ctx, 'moveTo')).toEqual([px(50, 50)]);
    expect(allOf(ctx, 'lineTo')).toEqual([px(350, 250)]);
  });

  it('도형을 키워도 끝점은 **여전히 중심**이다 — 경계 산술이 끼어들지 않는다', () => {
    const wide: CanvasElement = {
      id: 'A',
      kind: 'rect',
      style: { fill: '#111' },
      geometry: { x: 0, y: 0, w: 400, h: 40 },
    };
    const ctx = draw([wide, boxB(), link({ from: { el: 'A', a: 'c' }, to: { el: 'B', a: 'c' } })]);
    expect(allOf(ctx, 'moveTo')).toEqual([px(200, 20)]);
  });
});

// --- 장부와 저술 -------------------------------------------------------------

describe('연결선은 장부에도 저술에도 제 자리를 넘지 않는다', () => {
  it('글자 폭 장부에 **키를 더하지 않는다**', () => {
    // 이 장부는 `measureText` 를 지난 사실만 나른다(002). 연결선 키가 들면 받는 쪽이
    // "이 선에도 잡을 수 있는 글자 상자가 있다" 고 잘못 읽는다.
    const ctx = makeRecorder();
    const txt: CanvasElement = {
      id: 't1',
      kind: 'text',
      style: { fill: '#000' },
      geometry: { x: 10, y: 10 },
      text: 'abcd',
    };
    const widths = drawElements(ctx, [boxA(), boxB(), txt, link()], {}, {}, PROJ);
    expect(Object.keys(widths)).toEqual(['t1']);
  });

  it('`style` 이 아예 없어도 예외가 나지 않고 **색을 지어내지도 않는다**', () => {
    const bare = link();
    delete bare.style;
    const ctx = draw([boxA(), boxB(), bare]);
    // 경로는 세우되 잉크는 없다 — 001 의 "기본 색을 지어내지 않는다" 그대로다.
    expect(ops(ctx)).toContain('moveTo');
    expect(ops(ctx)).not.toContain('stroke');
    expect(ops(ctx)).not.toContain('strokeStyle');
  });

  it('선은 **채우지 않는다** — 열린 경로의 fill 은 뜻이 없다', () => {
    const ctx = draw([boxA(), boxB(), link({ style: { ...STROKE, fill: '#0f0' } })]);
    // 도형 둘이 fill 을 쓰므로 횟수로 잰다: 연결선이 채웠다면 셋이 된다.
    expect(allOf(ctx, 'fill')).toHaveLength(2);
  });

  it('`visible:false` 는 아무것도 그리지 않는다 — `save` 조차 하지 않는다', () => {
    // 도형 둘을 **반드시** 함께 둔다. 빼면 끝점이 풀리지 않아 끊긴 연결로 떨어지고,
    // 그때도 기록은 비어 있으므로 이 단언이 `visible` 이 아니라 AC-52 를 재게 된다.
    const hidden = draw([boxA(), boxB(), link({ style: { ...STROKE, visible: false } })]);
    const shown = draw([boxA(), boxB(), link({ style: { ...STROKE } })]);
    expect(connectorOps(hidden)).toEqual([]);
    expect(connectorOps(shown)).not.toEqual([]);
  });

  it('그리다 던져도 프레임이 무너지지 않고 `restore` 가 짝을 맞춘다', () => {
    // 진짜 context 는 좌표가 손상돼도 던지지 않지만, 이 층이 내건 규율은 "손상된 것 하나가
    // 프레임 전체를 지우지 않는다" 이다(001 REQ-05). 규율을 말로만 두지 않는다.
    const ctx = makeRecorder();
    const shape: CanvasElement = {
      id: 'z',
      kind: 'ellipse',
      style: { fill: '#333' },
      geometry: { x: 0, y: 0, w: 10, h: 10 },
    };
    const broken: DrawContext2D = {
      ...ctx,
      lineTo() {
        throw new Error('boom');
      },
    };
    expect(() =>
      drawElements(broken, [boxA(), boxB(), link(), shape], {}, {}, PROJ),
    ).not.toThrow();
    // 뒤의 도형은 그대로 그려졌고, `save`/`restore` 가 짝을 이룬다.
    expect(ops(ctx)).toContain('ellipse');
    expect(allOf(ctx, 'save')).toHaveLength(allOf(ctx, 'restore').length);
  });

  it('`styles` 맵의 항목이 저술을 **덮는다** — 캐스케이드의 자리가 하나다', () => {
    const ctx = makeRecorder();
    drawElements(
      ctx,
      [boxA(), boxB(), link()],
      { c1: { stroke: '#0a0', strokeWidth: 7 } },
      {},
      PROJ,
    );
    expect(allOf(ctx, 'strokeStyle')).toEqual([['#0a0']]);
    expect(allOf(ctx, 'lineWidth')).toEqual([[7]]);
  });
});

// --- 문구 상자에 붙은 끝점 ---------------------------------------------------

describe('문구에 붙은 끝점은 **직전 프레임**의 폭 장부를 지난다', () => {
  it('폭을 주면 그 상자의 앵커로 풀린다', () => {
    // 문구의 윤곽 상자는 잰 폭에서 나온다. 이번 프레임이 쌓는 중인 장부를 쓰면 같은
    // 연결선이 배열의 어디에 있느냐에 따라 끝점이 달라지므로, 직전 프레임의 장부를 받는다.
    const txt: CanvasElement = {
      id: 't1',
      kind: 'text',
      style: { fill: '#000' },
      geometry: { x: 100, y: 100 },
      text: 'abcd',
    };
    const withLedger = makeRecorder();
    drawElements(
      withLedger,
      [txt, boxB(), link({ from: { el: 't1', a: 'c' } })],
      {},
      {},
      PROJ,
      { t1: 40 },
    );
    const without = makeRecorder();
    drawElements(without, [txt, boxB(), link({ from: { el: 't1', a: 'c' } })], {}, {}, PROJ);

    // 둘 다 그려지되 **끝점이 다르다** — 장부가 실제로 상자에 닿는다는 사실이다.
    expect(allOf(withLedger, 'moveTo')).toHaveLength(1);
    expect(allOf(without, 'moveTo')).toHaveLength(1);
    expect(allOf(withLedger, 'moveTo')).not.toEqual(allOf(without, 'moveTo'));
  });

  it('장부를 넘기지 않아도 예외가 나지 않는다 — 부재는 빈 장부다 (REQ-09)', () => {
    const ctx = draw([boxA(), boxB(), link()]);
    expect(ops(ctx)).toContain('stroke');
  });
});
