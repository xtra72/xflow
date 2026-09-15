// 연결선 히트 판정 (SPEC-CANVAS-011 M7).
//
// ## 이 파일이 겨누는 함정
//
// 008 의 경로 시험이 적어 둔 **D5** 가 여기서도 그대로 걸린다: 잉크 **위만** 찍는 시험은
// 바운딩 박스 판정으로도 전부 초록이다. 그래서 여기서 잰다고 말할 수 있는 것은 **상자 안의
// 빈 곳이 빗나가는가**이며(AC-54), 그 한 줄이 REQ-07-b 의 유일한 가드다.
//
// 그 하나만으로는 부족한 자리가 곡선이다. 곡선을 **현(弦)으로** 재는 구현도 상자 판정은
// 아니므로 AC-54 를 지나간다. 그것을 가르는 자는 둘이다:
//
//   - 곡선의 **불룩한 정점**(현에서 150px 떨어진 자리)이 잡힌다 — 현으로 재면 빗나간다.
//   - **현 위의 점**(잉크에서 134px 떨어진 자리)이 잡히지 않는다 — 현으로 재면 잡힌다.
//
// 둘을 함께 두는 것이 요점이다. 앞의 하나만 두면 "전부 잡는" 구현이, 뒤의 하나만 두면
// "아무것도 안 잡는" 구현이 지나간다.
//
// ## 고정 입력이 기본값 모양이 아니다
//
// 축척 **가로 1.6 · 세로 1.5** — 단위 축척도 아니고 두 축이 같지도 않다. 집기 여유는
// **화면 양**이므로 축척이 무엇이든 6px 이어야 하며, 아래 경계 시험들이 그것을 고정한다.
//
// 끝점은 대부분 **자유 끝점**이다. 앵커 산술(M2·M3)을 지나면 이 파일이 재는 것이 히트인지
// 앵커인지 흐려진다 — 참조가 실제로 풀리는지는 `resolveConnector.test.ts` 와 아래 z-order ·
// 끊긴 연결 절이 따로 잰다.
//
// DOM 을 쓰지 않으므로 jsdom 없이 돈다.
//
// @spec SPEC-CANVAS-011 REQ-07-b · AC-53~AC-56

import { describe, expect, it } from 'vitest';

import type { CanvasElement, PointGeometry } from './canvasConfig';
import type { CanvasProjection, PxPoint } from './canvasGeometry';
import { HIT_TOLERANCE_PX, hitTest } from './canvasHitTest';
import type { ConnectorElement } from './connector/connectorTypes';
import { drawElements, type DrawContext2D } from './drawElement';
import type { CanvasNode } from './group/groupTypes';

// --- 고정 입력 -------------------------------------------------------------

/** 축척 가로 1.6 · 세로 1.5. 두 축이 다른 것이 이 픽스처의 요점이다. */
const PROJ: CanvasProjection = {
  stage: { width: 800, height: 600 },
  canvas: { width: 500, height: 400 },
};

const NO_WIDTHS: Record<string, number> = {};

/**
 * 캔버스 단위 → 스테이지 px. **시험이 스스로 센다** — 투영을 구현에서 빌려 오면 투영이
 * 통째로 틀려도 시험이 함께 틀려 초록이 된다.
 */
function toPx(x: number, y: number): PxPoint {
  return { x: x * 1.6, y: y * 1.5 };
}

const at = (x: number, y: number): PxPoint => ({ x, y });

function link(over: Partial<ConnectorElement> = {}): ConnectorElement {
  return {
    id: 'c1',
    kind: 'connector',
    from: { x: 0, y: 0 },
    to: { x: 200, y: 200 },
    route: 'straight',
    style: { stroke: '#f0f', strokeWidth: 2 },
    ...over,
  };
}

function rect(id: string, x: number, y: number, w: number, h: number): CanvasElement {
  return { id, kind: 'rect', style: { fill: '#111' }, geometry: { x, y, w, h } };
}

function pick(nodes: readonly CanvasNode[], point: PxPoint): string | undefined {
  return hitTest(nodes, point, PROJ, NO_WIDTHS)?.nodeId;
}

/**
 * 크게 꺾인 연결선 — `(0,0) → (200,0) → (200,200)`.
 *
 * px 로는 `(0,0) → (320,0) → (320,300)` 이고, 그 윤곽 상자 `320×300` 가운데 잉크가 지나는
 * 곳은 위 변과 오른쪽 변뿐이다. 나머지 **거의 전부가 빈 공간**이며 AC-54 가 거기서 잰다.
 */
function elbow(over: Partial<ConnectorElement> = {}): ConnectorElement {
  const points: PointGeometry[] = [{ x: 200, y: 0 }];
  return link({ route: 'elbow', points, to: { x: 200, y: 200 }, ...over });
}

/**
 * 아래로 불룩한 곡선 — 끝 `(0,0)`·`(200,0)`, 제어점 `(100,200)`.
 *
 * px 로 옮기면 2차 베지어 `P0(0,0) · Q(160,300) · P2(320,0)` 이고, `x(t) = 320t` 라
 * 가로는 t 에 선형이다. 정점(t=0.5)은 `(160, 150)` — **현에서 150px** 떨어져 있다.
 */
function curve(over: Partial<ConnectorElement> = {}): ConnectorElement {
  const points: PointGeometry[] = [{ x: 100, y: 200 }];
  return link({ route: 'curve', points, to: { x: 200, y: 0 }, ...over });
}

/** 곡선의 정점(px). 시험이 2차 베지어를 직접 셈한 값이다. */
const CURVE_APEX = at(160, 150);

// --- AC-53: 잉크 가까이에서 잡힌다 -------------------------------------------

describe('연결선은 **잉크와의 거리**로 잡힌다 (AC-53)', () => {
  it('직선 위를 누르면 잡힌다', () => {
    expect(pick([link()], toPx(100, 100))).toBe('c1');
  });

  it('여유 안쪽은 잡히고 여유 밖은 잡히지 않는다 — 경계가 **화면 양**이다', () => {
    // 꺾은 선의 위 변은 px 에서 y=0 의 가로 선분이라 이탈 거리를 눈으로 셀 수 있다.
    const nodes = [elbow()];
    expect(HIT_TOLERANCE_PX).toBe(6);
    expect(pick(nodes, at(160, 0))).toBe('c1');
    expect(pick(nodes, at(160, 4))).toBe('c1');
    expect(pick(nodes, at(160, -4))).toBe('c1');
    expect(pick(nodes, at(160, 10))).toBeUndefined();
    expect(pick(nodes, at(160, -10))).toBeUndefined();
  });

  it('꺾이는 자리와 두 끝도 잡힌다 — 중간점이 잉크에서 빠지지 않는다', () => {
    const nodes = [elbow()];
    expect(pick(nodes, at(320, 0))).toBe('c1');
    expect(pick(nodes, at(0, 0))).toBe('c1');
    expect(pick(nodes, at(320, 300))).toBe('c1');
    // 오른쪽 세로 변의 한가운데 — 두 번째 선분이 판정에 참여한다는 사실이다.
    expect(pick(nodes, at(320, 150))).toBe('c1');
  });

  it('두께가 여유보다 두꺼우면 **제 두께만큼** 잡힌다 — `line` 과 같은 식이다', () => {
    // 임계는 `max(두께/2, 여유)` 다. 40px 선은 제 가장자리(20px)까지 잡혀야 그림과 손이 맞는다.
    const thick = elbow({ style: { stroke: '#f0f', strokeWidth: 40 } });
    expect(pick([thick], at(160, 15))).toBe('c1');
    expect(pick([thick], at(160, 25))).toBeUndefined();
    // 같은 자리가 얇은 선에서는 빗나간다 — 두께가 실제로 판정에 참여한다.
    expect(pick([elbow()], at(160, 15))).toBeUndefined();
  });

  it('저술이 아예 없어도 잡히고 던지지 않는다 — `style` 은 있을 수도 없을 수도 있다', () => {
    // 연결선은 `style` 이 **없을 수 있는** 유일한 노드다. 요소와 같은 줄로 가시성을 보면
    // 그 자리에서 던진다.
    const bare = link();
    delete bare.style;
    expect(() => pick([bare], toPx(100, 100))).not.toThrow();
    expect(pick([bare], toPx(100, 100))).toBe('c1');
  });
});

// --- AC-54: 상자 안이라고 잡히지 않는다 ---------------------------------------

describe('상자 안이라고 잡히지 않는다 (AC-54 · REQ-07-b)', () => {
  /** 꺾은 선의 px 윤곽 상자. 시험이 직접 센다. */
  const BOX = { left: 0, right: 320, top: 0, bottom: 300 };

  /** 상자 안이면서 **잉크에서 먼** 자리들. 꺾임의 안쪽 삼각 지대가 전부 여기다. */
  const INSIDE_BUT_FAR: PxPoint[] = [
    at(20, 280),
    at(160, 150),
    at(80, 220),
    at(300, 290),
  ];

  it('고른 자리들이 실제로 **상자 안**이다 — 그물이 성기지 않다', () => {
    // 이 단언이 없으면 아래 "잡히지 않는다" 가 그저 "멀리서 눌렀다" 로 지나간다.
    for (const p of INSIDE_BUT_FAR) {
      expect(p.x >= BOX.left && p.x <= BOX.right, `x ${p.x}`).toBe(true);
      expect(p.y >= BOX.top && p.y <= BOX.bottom, `y ${p.y}`).toBe(true);
    }
  });

  it('그 자리들이 하나도 잡히지 않는다', () => {
    const nodes = [elbow()];
    for (const p of INSIDE_BUT_FAR) {
      expect(pick(nodes, p), `${p.x},${p.y}`).toBeUndefined();
    }
  });

  it('상자 안의 빈 곳에서는 **뒤에 놓인 요소**가 잡힌다', () => {
    // 상자로 잡으면 이 도형은 영원히 잡히지 않는다 — `hitsGroup` 이 밸브 심볼에 대해 적어
    // 둔 그 결함과 같은 부류다.
    const behind = rect('r1', 0, 100, 60, 60);
    expect(pick([behind, elbow()], toPx(30, 130))).toBe('r1');
  });
});

// --- 곡선은 평탄화를 지난다 (AC-55 의 행동 쪽) ---------------------------------

describe('곡선은 **그려진 그 곡선**으로 잡힌다', () => {
  it('불룩한 정점이 잡힌다 — 현으로 재면 150px 빗나간다', () => {
    expect(pick([curve()], CURVE_APEX)).toBe('c1');
  });

  it('정점 바깥쪽 여유 안은 잡히고 그 밖은 잡히지 않는다', () => {
    const nodes = [curve()];
    expect(pick(nodes, at(CURVE_APEX.x, CURVE_APEX.y + 4))).toBe('c1');
    expect(pick(nodes, at(CURVE_APEX.x, CURVE_APEX.y + 12))).toBeUndefined();
  });

  it('**오목한 안쪽**은 잡히지 않는다 — 현 위의 점이 그 자리다', () => {
    // `(160, 2)` 는 두 끝을 잇는 현에서 2px 이지만 진짜 곡선에서는 134px 떨어져 있다.
    // 현으로 재는 구현이 여기서 빨개진다.
    const nodes = [curve()];
    expect(pick(nodes, at(160, 2))).toBeUndefined();
    expect(pick(nodes, at(160, 60))).toBeUndefined();
    expect(pick(nodes, at(160, 100))).toBeUndefined();
  });

  it('그 오목 지역이 곡선의 **상자 안**이다 — 상자 판정도 함께 빨개진다', () => {
    // 곡선의 px 상자는 `x∈[0,320] · y∈[0,150]` 이다(정점이 아래 끝).
    for (const p of [at(160, 2), at(160, 60), at(160, 100)]) {
      expect(p.x >= 0 && p.x <= 320).toBe(true);
      expect(p.y >= 0 && p.y <= 150).toBe(true);
    }
  });

  it('중간점이 없는 `curve` 는 직선과 **같은 자리에서** 잡힌다 (AC-50 의 짝)', () => {
    // 그리는 쪽이 네 갈래의 호출 기록이 같다고 단언한 그 성질을, 잡는 쪽에서도 잰다.
    const straight = link({ route: 'straight' });
    const noMid = link({ route: 'curve' });
    for (const p of [toPx(100, 100), at(160, 153), at(200, 20)]) {
      expect(pick([noMid], p), `${p.x},${p.y}`).toBe(pick([straight], p));
    }
  });
});

// --- 퇴화한 연결선 -----------------------------------------------------------

describe('두 끝이 같은 자리인 연결선도 잡힌다 — 길이 0 선분', () => {
  const degenerate = link({ from: { x: 50, y: 50 }, to: { x: 50, y: 50 } });

  it('그 점 자신이 잡힌다 — `NaN` 으로 떨어지지 않는다', () => {
    // 길이 0 갈래가 없으면 `0/0 = NaN` 이 되고 모든 비교가 거짓이 되어 "아무것도 맞지 않음"
    // 과 구분되지 않는다. 그래서 **잡힌다**는 쪽으로 잰다.
    expect(pick([degenerate], toPx(50, 50))).toBe('c1');
  });

  it('여유 밖은 여전히 잡히지 않고 던지지도 않는다', () => {
    expect(() => pick([degenerate], at(80, 90))).not.toThrow();
    expect(pick([degenerate], at(80, 90))).toBeUndefined();
  });
});

// --- AC-56: 끊긴 연결은 잡히지 않는다 -----------------------------------------

describe('끊긴 연결은 잡히지 않고 던지지 않는다 (AC-56)', () => {
  it('가리킨 요소가 없으면 잡히지 않는다', () => {
    const broken = link({ from: { el: 'ghost', a: 'e' }, to: { x: 200, y: 200 } });
    expect(() => pick([broken], toPx(100, 100))).not.toThrow();
    expect(pick([broken], toPx(100, 100))).toBeUndefined();
  });

  it('없는 앵커 이름도 끊긴 연결이다', () => {
    const host = rect('A', 0, 0, 100, 100);
    const broken = link({ from: { el: 'A', a: 'no-such-anchor' }, to: { x: 200, y: 200 } });
    // 도형 자신은 그대로 잡힌다 — 끊긴 것은 선뿐이다.
    expect(pick([host, broken], toPx(50, 50))).toBe('A');
    expect(pick([host, broken], toPx(150, 150))).toBeUndefined();
  });

  it('끊긴 연결이 **뒤에 놓인 요소를 가리지 않는다**', () => {
    // 잡히지 않는 것과 "이벤트를 먹는다" 는 다르다. 순회는 그대로 다음 노드로 간다.
    const behind = rect('r1', 50, 50, 100, 100);
    const broken = link({ from: { el: 'ghost', a: 'e' } });
    expect(pick([behind, broken], toPx(100, 100))).toBe('r1');
  });
});

// --- `visible:false` ---------------------------------------------------------

describe('그리지 않는 선은 잡히지 않는다', () => {
  it('`visible:false` 연결선은 잡히지 않는다', () => {
    const hidden = link({ style: { stroke: '#f0f', strokeWidth: 2, visible: false } });
    expect(pick([hidden], toPx(100, 100))).toBeUndefined();
  });

  it('그 자리에서 **뒤에 놓인 요소**가 잡힌다', () => {
    const behind = rect('r1', 50, 50, 100, 100);
    const hidden = link({ style: { stroke: '#f0f', strokeWidth: 2, visible: false } });
    expect(pick([behind, hidden], toPx(100, 100))).toBe('r1');
  });
});

// --- z-order: 그리는 순서와 잡는 순서가 **같은 하나**다 -------------------------

/** 호출 이름만 모으는 최소 스텁 — 이 절이 재는 것은 순서뿐이다. */
function makeOrderRecorder(): DrawContext2D & { readonly ops: string[] } {
  const ops: string[] = [];
  const rec =
    (op: string) =>
    (): void => {
      ops.push(op);
    };
  return {
    ops,
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
    measureText: (text: string) => ({ width: text.length * 10 }),
    clearRect: rec('clearRect'),
    fillRect: rec('fillRect'),
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    globalAlpha: 1,
    font: '',
    textAlign: 'left',
    textBaseline: 'alphabetic',
  };
}

describe('겹친 자리의 승자는 **배열 자리**가 정한다 — 종류가 아니다', () => {
  // 도형의 px 상자는 `(0,0)-(320,300)` 이고 연결선의 잉크는 그 대각선이다. 아래 지점은
  // 둘 **모두**에 맞으므로 승자를 가르는 것이 배열 자리뿐이다.
  const overlap = toPx(100, 100);
  const shape = (): CanvasElement => rect('r1', 0, 0, 200, 200);

  it('연결선이 도형보다 **뒤**에 있으면 연결선이 잡힌다', () => {
    expect(pick([shape(), link()], overlap)).toBe('c1');
  });

  it('연결선이 도형보다 **앞**에 있으면 도형이 잡힌다', () => {
    expect(pick([link(), shape()], overlap)).toBe('r1');
  });

  it('그 승자가 **나중에 그려진 쪽**이다 — 두 층이 같은 배열을 본다', () => {
    // 층을 건너 재는 자리다. 어느 한쪽만 초록인 구현(그리기는 배열 순, 잡기는 정방향
    // 순회)이 여기서만 빨개진다.
    for (const nodes of [[shape(), link()], [link(), shape()]] as const) {
      const ctx = makeOrderRecorder();
      drawElements(ctx, nodes, {}, {}, PROJ);
      const drawnLast = ctx.ops.lastIndexOf('rect') > ctx.ops.lastIndexOf('stroke') ? 'r1' : 'c1';
      expect(pick(nodes, overlap)).toBe(drawnLast);
    }
  });

  it('연결선 둘이 겹치면 **뒤의 것**이 잡힌다', () => {
    const under = link({ id: 'under' });
    const over = link({ id: 'over' });
    expect(pick([under, over], overlap)).toBe('over');
    expect(pick([over, under], overlap)).toBe('under');
  });
});

// --- 참조로 붙은 끝점 ---------------------------------------------------------

describe('붙은 끝점은 도형을 따라간다 — 잡는 자리도 함께 옮겨 앉는다', () => {
  it('도형을 옮기면 잡히던 자리가 바뀐다', () => {
    // 참조가 실제로 풀린다는 사실을 히트 쪽에서 한 번 더 못박는다. 끝점을 좌표로 적어 둔
    // 구현은 도형을 옮겨도 선이 옛 자리에 남으며, 그 어긋남은 화면에서만 보인다.
    const near = rect('A', 0, 0, 100, 100);
    const far = rect('A', 200, 200, 100, 100);
    const attached = link({ from: { el: 'A', a: 'c' }, to: { x: 400, y: 300 } });

    // A 의 중심은 각각 (50,50) 과 (250,250) — 두 선은 서로 다른 자리를 지난다.
    // `(190,150)` 은 첫 선 위의 점이고(기울기 5/7), 둘째 선의 시작점보다 앞쪽이다.
    const onFirst = toPx(190, 150);
    expect(pick([near, attached], onFirst)).toBe('c1');
    expect(pick([far, attached], onFirst)).toBeUndefined();
  });
});
