// 꺾임·곡률 편집의 산술 (SPEC-CANVAS-011 M10 · REQ-05 · AC-67 · AC-68 · AC-71).
//
// ## 이 파일이 겨누는 결함
//
// M10 이 여는 실패 형상은 하나이고, 그것은 **화면에서만** 드러난다: 새 점이 목록의 엉뚱한
// 자리에 들어 선이 스스로를 가로지른다. 배열은 멀쩡하고 예외도 없다 — 점 수도 맞고 좌표도
// 선 위이며, 다만 차례가 틀렸다.
//
// 그 차례를 틀리게 만드는 길이 둘 있다.
//
//   ① **끝에 붙인다.** SPEC 이 REQ-05-a 로 곧장 금지한 그것이다.
//   ② **제어 다각형으로 잰다.** `curve` 에서만 나타난다. 그려진 곡선은 제어점들을 잇는
//      다각형에서 한참 떨어져 지날 수 있으므로, 다각형과의 거리로 자리를 고르면 **보이는
//      곡선을 눌렀는데** 다른 구간의 뒤에 점이 든다. ①과 달리 이 길은 그럴듯해 보이고,
//      `elbow` 에서는 두 재기가 **같은 답**을 내므로 곡선 배치 없이는 잡히지 않는다.
//
// 그래서 이 파일은 ②를 위해 배치 하나를 들고 있다(아래 §제어 다각형으로 재면 틀린다).
// 그 배치에서 다각형 재기는 `0` 을, 잉크 재기는 `1` 을 낸다 — 시험이 **두 답을 모두**
// 적어 두므로, 구현이 다각형으로 돌아가는 날 그 자리가 곧바로 빨개진다.
//
// ## 투영은 1:1 이다
//
// 스테이지와 캔버스가 같은 크기라 px 값과 캔버스 단위가 글자 그대로 같다. 자리를 겨누는
// 시험에서 축척까지 함께 재면 빨개진 이유가 둘이 되고, 그때 다음 사람은 어느 쪽이 틀렸는지
// 를 먼저 가려내야 한다. 축척이 섞이는 자리는 오버레이 시험이 따로 붙든다.
//
// @spec SPEC-CANVAS-011 REQ-05 · REQ-05-a · REQ-05-b

import { describe, expect, it } from 'vitest';

import type { CanvasSize } from '../canvasConfig';
import {
  closestPointOnSegment,
  type CanvasPoint,
  type CanvasProjection,
  type PxPoint,
  type StageSize,
} from '../canvasGeometry';
import {
  connectorPointGestureAt,
  insertPointAt,
  removePointAt,
} from './connectorEdit';
import { connectorPath } from './connectorPath';
import { CONNECTOR_KIND, type ConnectorElement } from './connectorTypes';

// --- 고정 입력 -------------------------------------------------------------

const STAGE: StageSize = { width: 500, height: 400 };
const CANVAS: CanvasSize = { width: 500, height: 400 };
const PROJ: CanvasProjection = { stage: STAGE, canvas: CANVAS };

/** 앵커 쪽에서 오는 그 오차 — 오버레이가 넘기는 값과 같다(`ANCHOR_PICK_SLOP_PX` = 9). */
const SLOP = 9;

function point(x: number, y: number): CanvasPoint {
  return { x, y };
}

function connector(over: Partial<ConnectorElement> = {}): ConnectorElement {
  return {
    id: 'c-1',
    kind: CONNECTOR_KIND,
    from: { el: 'r1', a: 'e' },
    to: { el: 'r2', a: 'w' },
    route: 'elbow',
    style: { stroke: '#3b82f6', strokeWidth: 2 },
    ...over,
  };
}

/**
 * **011 이 기각한 재기** — 점 목록(곡선에서는 제어 다각형)과의 거리로만 자리를 고른다.
 *
 * 시험 안에 두는 것에 뜻이 있다. 이 함수가 내는 답과 구현이 내는 답이 **갈리는 배치**를
 * 하나 들고 있어야, "잉크로 잰다" 가 말이 아니라 값으로 고정된다. 재는 산술은 구현과 같은
 * 것(`closestPointOnSegment`)을 쓴다 — 갈리는 것은 **무엇을 재는가**이지 어떻게 재는가가
 * 아니라는 사실을 그렇게 적는다.
 */
function polygonSlot(points: readonly CanvasPoint[], at: PxPoint): number {
  let best = 0;
  let bestDistance = Number.POSITIVE_INFINITY;
  for (let i = 0; i + 1 < points.length; i += 1) {
    const a = points[i]!;
    const b = points[i + 1]!;
    const on = closestPointOnSegment({ x1: a.x, y1: a.y, x2: b.x, y2: b.y }, at);
    const distance = Math.hypot(on.x - at.x, on.y - at.y);
    if (distance < bestDistance) {
      bestDistance = distance;
      best = i;
    }
  }
  return best;
}

/** 2차 베지어 한 점 — 시험이 **곡선 위의 자리**를 겨누는 데 쓴다(구현을 부르지 않는다). */
function quadraticAt(a: CanvasPoint, q: CanvasPoint, b: CanvasPoint, t: number): PxPoint {
  const u = 1 - t;
  return {
    x: u * u * a.x + 2 * t * u * q.x + t * t * b.x,
    y: u * u * a.y + 2 * t * u * q.y + t * t * b.y,
  };
}

// --- AC-68: 끼워 넣는 자리가 눌린 구간의 뒤다 --------------------------------

describe('끼워 넣는 자리가 **눌린 구간의 뒤**다 (AC-68 · REQ-05-a)', () => {
  /** SPEC 이 글자로 적은 그 선 — 점 A-B-C(시작 · 중간점 하나 · 끝). */
  const A = point(100, 100);
  const B = point(300, 100);
  const C = point(300, 300);
  const ABC = [A, B, C];

  it('A-B 구간을 누르면 새 점이 **A 와 B 사이**에 든다', () => {
    const gesture = connectorPointGestureAt(ABC, 'elbow', { x: 150, y: 100 }, PROJ, SLOP);
    expect(gesture).toEqual({ kind: 'insert', index: 0, at: { x: 150, y: 100 } });

    // 자리는 저장 배열의 색인이다 — 그 배열에 실제로 그렇게 든다.
    const next = insertPointAt(connector({ points: [{ x: B.x, y: B.y }] }), 0, { x: 150, y: 100 });
    expect(next.points).toEqual([
      { x: 150, y: 100 },
      { x: 300, y: 100 },
    ]);
  });

  it('B-C 구간을 누르면 **B 뒤**에 든다 — 같은 선, 다른 구간', () => {
    const gesture = connectorPointGestureAt(ABC, 'elbow', { x: 300, y: 200 }, PROJ, SLOP);
    expect(gesture).toEqual({ kind: 'insert', index: 1, at: { x: 300, y: 200 } });
  });

  it('중간점 셋인 선의 **가운데 구간**도 그 구간의 뒤다', () => {
    // 해석된 목록 [시작, A, B, C, 끝] 에서 A-B 구간은 저장 배열의 1 번 자리다.
    const points = [point(0, 0), point(100, 0), point(200, 0), point(300, 0), point(400, 0)];
    const gesture = connectorPointGestureAt(points, 'elbow', { x: 150, y: 0 }, PROJ, SLOP);
    expect(gesture).toEqual({ kind: 'insert', index: 1, at: { x: 150, y: 0 } });

    const next = insertPointAt(
      connector({
        points: [
          { x: 100, y: 0 },
          { x: 200, y: 0 },
          { x: 300, y: 0 },
        ],
      }),
      1,
      { x: 150, y: 0 },
    );
    expect(next.points).toEqual([
      { x: 100, y: 0 },
      { x: 150, y: 0 },
      { x: 200, y: 0 },
      { x: 300, y: 0 },
    ]);
  });

  it('**첫 구간**을 누르면 맨 앞에 든다', () => {
    const points = [point(0, 0), point(100, 0), point(200, 0)];
    expect(connectorPointGestureAt(points, 'elbow', { x: 40, y: 0 }, PROJ, SLOP)).toMatchObject({
      kind: 'insert',
      index: 0,
    });
  });

  it('**마지막 구간**을 누르면 맨 뒤에 든다 — 그리고 그 자리는 범위 안이다', () => {
    const points = [point(0, 0), point(100, 0), point(200, 0)];
    const gesture = connectorPointGestureAt(points, 'elbow', { x: 160, y: 0 }, PROJ, SLOP);
    expect(gesture).toMatchObject({ kind: 'insert', index: 1 });

    // 끝에 붙이는 자리(`index === points.length`)가 거절되면 마지막 구간을 누른 손이
    // 아무 일도 하지 못한다.
    const next = insertPointAt(connector({ points: [{ x: 100, y: 0 }] }), 1, point(160, 0));
    expect(next.points).toEqual([
      { x: 100, y: 0 },
      { x: 160, y: 0 },
    ]);
  });

  it('중간점이 **하나도 없는** 선에도 구간은 있다 — 두 끝 사이 하나다', () => {
    const points = [point(100, 100), point(300, 100)];
    const gesture = connectorPointGestureAt(points, 'elbow', { x: 200, y: 100 }, PROJ, SLOP);
    expect(gesture).toEqual({ kind: 'insert', index: 0, at: { x: 200, y: 100 } });

    const next = insertPointAt(connector(), 0, point(200, 100));
    expect(next.points).toEqual([{ x: 200, y: 100 }]);
  });
});

// --- 곡선: 제어 다각형으로 재면 틀린다 ---------------------------------------

describe('곡선은 **그려진 잉크**로 잰다 — 제어 다각형이 아니다 (REQ-05-a)', () => {
  // 제어 다각형이 그려진 곡선에서 멀리 떨어지는 배치다.
  //
  //   A(100,100) ─────────────── P(300,100)
  //                                   │
  //                                 B(300,120)
  //
  // 곡선은 A 에서 출발해 P 쪽으로 붙었다가 B 로 꺾여 오르며, **P 근처를 지나지 않는다.**
  // 누르는 자리는 그 곡선 위의 t=0.6 지점 — 호 길이로는 이미 중간(≈t 0.31)을 한참 지났다.
  const A = point(100, 100);
  const P = point(300, 100);
  const B = point(300, 120);
  const CURVE = [A, P, B];
  const PRESS = quadraticAt(A, P, B, 0.6);

  it('누른 자리는 그려진 곡선 **위**다 — 전제를 시험이 스스로 세운다', () => {
    // 잉크는 그리는 쪽·잡는 쪽이 보는 그 명령 목록에서 나온다. 누름이 잉크에서 떨어져
    // 있으면 아래 단언들은 "곡선을 눌렀다" 를 재는 것이 아니게 된다.
    const cmds = connectorPath(CURVE, 'curve', PROJ);
    expect(cmds.map((c) => c.c)).toEqual(['M', 'C']);
    const gesture = connectorPointGestureAt(CURVE, 'curve', PRESS, PROJ, SLOP);
    expect(gesture?.kind).toBe('insert');
    if (gesture?.kind !== 'insert') return;
    expect(Math.hypot(gesture.at.x - PRESS.x, gesture.at.y - PRESS.y)).toBeLessThan(0.5);
  });

  it('잉크로 재면 **P 의 뒤**(1)다 — 손이 지나온 자리가 그것이다', () => {
    expect(connectorPointGestureAt(CURVE, 'curve', PRESS, PROJ, SLOP)).toMatchObject({
      kind: 'insert',
      index: 1,
    });
  });

  it('제어 다각형으로 재면 **P 의 앞**(0)이다 — 두 답이 실제로 갈린다', () => {
    // 이 단언이 이 파일의 이유다. 다각형 재기는 누른 자리에서 A-P 변까지 7.2, P-B 변까지
    // 32 를 재어 `0` 을 고른다 — 손은 곡선의 끝자락을 눌렀는데 점은 **시작 쪽**에 든다.
    expect(polygonSlot(CURVE, PRESS)).toBe(0);
    const gesture = connectorPointGestureAt(CURVE, 'curve', PRESS, PROJ, SLOP);
    expect(gesture).toMatchObject({ kind: 'insert' });
    if (gesture?.kind !== 'insert') return;
    expect(gesture.index).not.toBe(polygonSlot(CURVE, PRESS));
  });

  it('곡선의 **앞자락**을 누르면 P 의 앞(0)이다 — 규칙이 한쪽으로 치우치지 않았다', () => {
    const early = quadraticAt(A, P, B, 0.1);
    expect(connectorPointGestureAt(CURVE, 'curve', early, PROJ, SLOP)).toMatchObject({
      kind: 'insert',
      index: 0,
    });
  });

  it('`elbow` 에서는 두 재기가 **같은 답**이다 — 그래서 곡선 배치가 필요했다', () => {
    // 꺾은 선에서는 그려진 잉크가 곧 점 목록이므로 ②의 결함이 나타날 자리가 없다.
    const at = { x: 250, y: 100 };
    const gesture = connectorPointGestureAt(CURVE, 'elbow', at, PROJ, SLOP);
    expect(gesture).toMatchObject({ kind: 'insert', index: polygonSlot(CURVE, at) });
  });

  it('제어점 **둘**인 곡선도 조각마다 제 자리를 안다', () => {
    // 해석된 목록 [시작, P1, P2, 끝] — 3차 조각은 둘이고 자리는 셋이다.
    const points = [point(100, 300), point(200, 100), point(300, 100), point(400, 300)];
    // 첫 조각의 앞자락 → 0, 둘째 조각의 뒷자락 → 2. 가운데(두 조각의 이음매 근처)는 1 이다.
    const head = connectorPointGestureAt(points, 'curve', { x: 120, y: 260 }, PROJ, SLOP);
    const tail = connectorPointGestureAt(points, 'curve', { x: 380, y: 260 }, PROJ, SLOP);
    expect(head).toMatchObject({ kind: 'insert', index: 0 });
    expect(tail).toMatchObject({ kind: 'insert', index: 2 });
  });
});

// --- AC-71: 중간점 더블클릭이 그 점을 뺀다 -----------------------------------

describe('오차 안의 중간점은 **빼기**다 (AC-71 · REQ-05-b)', () => {
  const points = [point(0, 0), point(100, 0), point(200, 0), point(300, 0)];

  it('점 위를 누르면 그 점의 **저장 배열 자리**를 낸다', () => {
    expect(connectorPointGestureAt(points, 'elbow', { x: 100, y: 0 }, PROJ, SLOP)).toEqual({
      kind: 'remove',
      index: 0,
    });
    expect(connectorPointGestureAt(points, 'elbow', { x: 200, y: 0 }, PROJ, SLOP)).toEqual({
      kind: 'remove',
      index: 1,
    });
  });

  it('오차 **안**이면 정확히 누르지 않아도 빼기다 — 앵커와 같은 여유다', () => {
    expect(connectorPointGestureAt(points, 'elbow', { x: 100, y: 8 }, PROJ, SLOP)).toEqual({
      kind: 'remove',
      index: 0,
    });
  });

  it('오차 **밖**이면 그 자리에 새 점이 든다 — 곁에 놓을 길이 남는다', () => {
    const gesture = connectorPointGestureAt(points, 'elbow', { x: 100, y: 12 }, PROJ, SLOP);
    expect(gesture).toMatchObject({ kind: 'insert' });
  });

  it('**끝점은 빼지 않는다** — 갈아 끼우는 것이지 없애는 것이 아니다 (REQ-07-a)', () => {
    expect(connectorPointGestureAt(points, 'elbow', { x: 0, y: 0 }, PROJ, SLOP)).toMatchObject({
      kind: 'insert',
    });
    expect(connectorPointGestureAt(points, 'elbow', { x: 300, y: 0 }, PROJ, SLOP)).toMatchObject({
      kind: 'insert',
    });
  });
});

// --- 뜻이 서지 않는 경우 -----------------------------------------------------

describe('뜻이 서지 않는 누름은 **아무것도 아니다** — 예외가 아니다', () => {
  it('직선은 중간점을 들지 않는다 (REQ-04)', () => {
    const points = [point(100, 100), point(300, 100)];
    expect(connectorPointGestureAt(points, 'straight', { x: 200, y: 100 }, PROJ, SLOP)).toBeUndefined();
  });

  it('`free` 는 든다 — 점의 출처가 다를 뿐 고치는 법은 같다', () => {
    const points = [point(100, 100), point(300, 100)];
    expect(connectorPointGestureAt(points, 'free', { x: 200, y: 100 }, PROJ, SLOP)).toMatchObject({
      kind: 'insert',
    });
  });

  it('**끊긴 연결**은 목록이 부재다 (REQ-08)', () => {
    // `resolveConnector` 가 내는 값을 그대로 받는다 — 부르는 쪽에 갈래를 두지 않는다.
    expect(connectorPointGestureAt(undefined, 'elbow', { x: 0, y: 0 }, PROJ, SLOP)).toBeUndefined();
  });

  it('점이 둘이 못 되면 구간이 없다 — 손으로 지은 목록도 던지지 않는다', () => {
    expect(connectorPointGestureAt([], 'elbow', { x: 0, y: 0 }, PROJ, SLOP)).toBeUndefined();
    expect(
      connectorPointGestureAt([point(10, 10)], 'elbow', { x: 0, y: 0 }, PROJ, SLOP),
    ).toBeUndefined();
  });
});

// --- 저장되는 값 -------------------------------------------------------------

describe('끼워 넣은 점은 **캔버스 단위 정수**이고 선 위에 있다 (A5 · AC-36)', () => {
  it('소수 자리를 누르면 정수로 죄인다 — 저장 왕복에 값이 옮겨 앉지 않는다', () => {
    // 기울어진 구간이라 가장 가까운 자리가 정수로 떨어지지 않는다.
    const points = [point(100, 100), point(200, 150)];
    const gesture = connectorPointGestureAt(points, 'elbow', { x: 150.4, y: 130 }, PROJ, SLOP);
    expect(gesture?.kind).toBe('insert');
    if (gesture?.kind !== 'insert') return;

    const next = insertPointAt(connector(), gesture.index, gesture.at);
    const stored = next.points?.[0];
    expect(stored).toBeDefined();
    expect(Number.isInteger(stored?.x)).toBe(true);
    expect(Number.isInteger(stored?.y)).toBe(true);
  });

  it('죄고 나서도 **선 위**다 — 어긋남은 반 칸의 대각선(≈0.71) 이하다', () => {
    const a = point(100, 100);
    const b = point(200, 150);
    const gesture = connectorPointGestureAt([a, b], 'elbow', { x: 150.4, y: 130 }, PROJ, SLOP);
    if (gesture?.kind !== 'insert') throw new Error('끼워 넣기가 아니다');
    const next = insertPointAt(connector(), gesture.index, gesture.at);
    const stored = next.points![0]!;
    const on = closestPointOnSegment({ x1: a.x, y1: a.y, x2: b.x, y2: b.y }, stored);
    expect(Math.hypot(on.x - stored.x, on.y - stored.y)).toBeLessThanOrEqual(Math.SQRT1_2);
  });
});

// --- 쓰기 두 함수 -------------------------------------------------------------

describe('`insertPointAt` · `removePointAt` 는 **새 연결선**을 낸다', () => {
  it('범위 밖 자리는 받은 것을 그대로 돌려준다 — 없는 자리를 지어내지 않는다', () => {
    const c = connector({ points: [{ x: 10, y: 10 }] });
    expect(insertPointAt(c, -1, point(0, 0))).toBe(c);
    expect(insertPointAt(c, 2, point(0, 0))).toBe(c);
    expect(removePointAt(c, -1)).toBe(c);
    expect(removePointAt(c, 1)).toBe(c);
    // 점을 쓴 적 없는 선은 뺄 것이 없다.
    const bare = connector();
    expect(removePointAt(bare, 0)).toBe(bare);
  });

  it('빼도 이웃은 **같은 객체 그대로** 지나간다', () => {
    const kept = { x: 20, y: 20 };
    const c = connector({ points: [{ x: 10, y: 10 }, kept] });
    expect(removePointAt(c, 0).points?.[0]).toBe(kept);
  });

  it('마지막 하나를 빼면 **키를 지운다** — 빈 배열을 남기지 않는다 (AC-25)', () => {
    const c = connector({ points: [{ x: 10, y: 10 }] });
    const next = removePointAt(c, 0);
    expect('points' in next).toBe(false);
  });

  it('점을 전부 뺀 곡선은 **직선으로 돌아온다** (AC-50)', () => {
    // 사용자의 말("빼면 직선으로 돌아온다")이 코드 한 줄도 없이 성립한다는 사실을 잰다:
    // 점이 없으면 `curveSegments` 가 빈 목록을 내고 네 갈래가 같은 그림이 된다.
    const ends = [point(100, 100), point(300, 300)];
    const c = connector({ route: 'curve', points: [{ x: 300, y: 100 }] });
    const bent = connectorPath([ends[0]!, { x: 300, y: 100 }, ends[1]!], c.route, PROJ);
    expect(bent.map((cmd) => cmd.c)).toEqual(['M', 'C']);

    const flat = removePointAt(c, 0);
    expect(flat.points).toBeUndefined();
    expect(connectorPath(ends, flat.route, PROJ).map((cmd) => cmd.c)).toEqual(['M', 'L']);
  });

  it('원본을 제자리에서 고치지 않는다 — 되돌리기 장부를 지나지 않는 변경이 없다', () => {
    const points = [{ x: 10, y: 10 }];
    const c = connector({ points });
    insertPointAt(c, 0, point(5, 5));
    removePointAt(c, 0);
    expect(c.points).toBe(points);
    expect(points).toEqual([{ x: 10, y: 10 }]);
  });
});
