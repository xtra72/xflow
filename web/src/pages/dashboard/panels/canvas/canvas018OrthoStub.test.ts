// 직각 선이 끝 도형에도 붙지 않는다 (SPEC-CANVAS-018 · 사용자 신고 2026-09-17).
//
// ## 신고된 장면을 그대로 세운다
//
// 왼쪽 상자의 `e` 앵커에서 오른쪽 상자의 `w` 앵커로 직각으로 이은 뒤, **오른쪽 상자를
// 내린다.** 가운데 세로 구간만 길어져야 하는데 017 에서는 세로선이 오른쪽 상자의 **왼쪽
// 변에 붙어** 내려왔다 — 017 §결정 3 이 두 끝 도형을 장애물에서 통째로 뺐기 때문이다.
//
// @spec SPEC-CANVAS-018 REQ-01 · REQ-02 · REQ-04

import { describe, expect, it } from 'vitest';

import { parseCanvasConfig, type CanvasSize } from './canvasConfig';
import type { CanvasProjection, PxBox, PxPoint } from './canvasGeometry';
import { drawElements, type DrawContext2D } from './drawElement';
import { isConnector, type ConnectorElement } from './connector/connectorTypes';
import { ORTHO_MARGIN, orthoStub } from './connector/orthoRoute';
import { connectorObstacles } from './connector/connectorObstacles';
import type { CanvasNode } from './group/groupTypes';

const CANVAS: CanvasSize = { width: 400, height: 300 };
const PROJ: CanvasProjection = { stage: { width: 400, height: 300 }, canvas: CANVAS };

/** 오른쪽 상자의 세로 자리만 바꾼다 — 신고된 두 배치가 이 인자 하나로 갈린다. */
function scene(rightY: number): CanvasNode[] {
  return parseCanvasConfig({
    canvas: { ...CANVAS },
    elements: [
      { id: 'a', kind: 'rect', geometry: { x: 40, y: 40, w: 80, h: 50 }, style: { fill: '#39f' } },
      { id: 'b', kind: 'rect', geometry: { x: 230, y: rightY, w: 80, h: 50 }, style: { fill: '#39f' } },
      {
        id: 'c1',
        kind: 'connector',
        from: { el: 'a', a: 'e' },
        to: { el: 'b', a: 'w' },
        route: 'ortho',
        style: { stroke: '#4a8', strokeWidth: 2 },
      },
    ],
  }).elements;
}

function makeRecorder(): DrawContext2D & { calls: [string, ...unknown[]][] } {
  const calls: [string, ...unknown[]][] = [];
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
    measureText: (t: string) => ({ width: t.length * 10 }),
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    globalAlpha: 1,
    font: '',
    textAlign: 'left' as CanvasTextAlign,
    textBaseline: 'alphabetic' as CanvasTextBaseline,
  };
}

function line(nodes: readonly CanvasNode[]): PxPoint[] {
  const ctx = makeRecorder();
  drawElements(ctx, nodes, {}, {}, PROJ);
  const out: PxPoint[] = [];
  let seen = false;
  for (const [op, x, y] of ctx.calls) {
    if (op === 'moveTo') {
      out.length = 0;
      out.push({ x: x as number, y: y as number });
      seen = true;
    } else if (op === 'lineTo' && seen) out.push({ x: x as number, y: y as number });
  }
  return out;
}

function connectorOf(nodes: readonly CanvasNode[]): ConnectorElement {
  const found = nodes.find((n): n is ConnectorElement => isConnector(n));
  if (found === undefined) throw new Error('연결선이 없다');
  return found;
}

/** 넓힌 상자의 **안**을 지나는가. 다리는 이 판정에서 빠진다(아래 §다리). */
function entersBox(a: PxPoint, b: PxPoint, box: PxBox): boolean {
  for (let i = 1; i < 200; i += 1) {
    const t = i / 200;
    const x = a.x + (b.x - a.x) * t;
    const y = a.y + (b.y - a.y) * t;
    if (x > box.x && x < box.x + box.w && y > box.y && y < box.y + box.h) return true;
  }
  return false;
}

// --- ① 신고된 결함 (REQ-01 · REQ-04) ----------------------------------------

describe('상자를 내려도 세로 구간만 길어진다 (REQ-04 · 신고된 결함)', () => {
  it('두 배치가 **같은 모양**이고 세로만 길어진다', () => {
    // 신고된 장면은 처음부터 어긋나 있다(첫 그림의 두 상자가 이미 대각이다). 같은 높이로
    // 두면 곧은 선이라 "모양이 같다" 가 아무것도 재지 않는다 — 첫 판에서 그랬다.
    const before = line(scene(100));
    const after = line(scene(180));
    // 꼭짓점 수가 같다 — 모양이 바뀌지 않았다는 뜻이다.
    expect(after).toHaveLength(before.length);
    // 가로 자리(꺾이는 x)가 그대로다.
    expect(after.map((p) => p.x)).toEqual(before.map((p) => p.x));
    // 세로만 움직였다.
    expect(after.some((p, i) => p.y !== before[i]!.y)).toBe(true);
  });

  it('**끝 도형의 변에 붙지 않는다** (REQ-01)', () => {
    // 017 의 결함이 정확히 이것이었다 — 세로선이 오른쪽 상자의 왼쪽 변을 타고 내려왔다.
    const nodes = scene(180);
    const path = line(nodes);
    const right: PxBox = {
      x: 230 - ORTHO_MARGIN,
      y: 180 - ORTHO_MARGIN,
      w: 80 + ORTHO_MARGIN * 2,
      h: 50 + ORTHO_MARGIN * 2,
    };
    // 마지막 구간은 **다리**이므로 상자 안을 지나는 것이 정상이다(REQ-02). 그 앞의
    // 구간들은 전부 바깥이어야 한다.
    for (let i = 1; i < path.length - 1; i += 1) {
      expect(entersBox(path[i - 1]!, path[i]!, right), `구간 ${i}`).toBe(false);
    }
  });

  it('모든 구간이 여전히 한 축 위다 (K1)', () => {
    for (const y of [40, 100, 180, 230]) {
      const path = line(scene(y));
      for (let i = 1; i < path.length; i += 1) {
        const a = path[i - 1]!;
        const b = path[i]!;
        expect(a.x === b.x || a.y === b.y, `y=${y} 구간 ${i}`).toBe(true);
      }
    }
  });
});

// --- ② 다리 (REQ-02 · REQ-03 · REQ-05) --------------------------------------

describe('나가는 다리 (REQ-02 · REQ-03 · REQ-05)', () => {
  const HOST: PxBox = { x: 100, y: 100, w: 100, h: 60 };

  it('상대가 오른쪽이면 오른쪽 변으로 나간다 (REQ-03)', () => {
    const exit = orthoStub({ x: 150, y: 130 }, HOST, { x: 400, y: 130 });
    expect(exit).toEqual({ x: 200 + ORTHO_MARGIN, y: 130 });
  });

  it('상대가 아래로 더 멀면 아래 변으로 나간다', () => {
    const exit = orthoStub({ x: 150, y: 130 }, HOST, { x: 160, y: 400 });
    expect(exit).toEqual({ x: 150, y: 160 + ORTHO_MARGIN });
  });

  it('다리는 **한 축**으로만 난다 — 상자 안에서 꺾이지 않는다 (§결정 1)', () => {
    const anchor = { x: 150, y: 130 };
    const exit = orthoStub(anchor, HOST, { x: 400, y: 400 })!;
    // 좌표 하나만 바뀐다.
    expect(exit.x === anchor.x || exit.y === anchor.y).toBe(true);
  });

  it('이미 상자 밖이면 다리가 없다 (REQ-05)', () => {
    // 길이 0 인 구간을 만들지 않는다.
    expect(orthoStub({ x: 400, y: 130 }, HOST, { x: 0, y: 130 })).toBeUndefined();
  });
});

// --- ③ 017 의 나머지는 그대로다 ----------------------------------------------

describe('017 이 세운 것은 그대로다', () => {
  it('세 소비자가 보는 목록을 **한 함수**가 낸다 (017 K4)', () => {
    const nodes = scene(100);
    const routing = connectorObstacles(connectorOf(nodes), nodes, PROJ, {});
    // 두 끝 도형이 장애물이면서 동시에 다리를 낼 상자다.
    expect(routing.obstacles).toHaveLength(2);
    expect(routing.hosts.from).toBeDefined();
    expect(routing.hosts.to).toBeDefined();
  });

  it('피할 것이 없으면 015 의 길 그대로다 (K3)', () => {
    // 두 앵커가 같은 `y` 이고 사이에 아무것도 없으면 **곧은 두 점**이다 — 다리가 낸
    // 같은 축 위의 점은 접힌다(`collapseCollinear`).
    const path = line(scene(40));
    expect(path).toHaveLength(2);
    // 투영이 부동소수를 지나므로 정확 비교는 쓸 수 없다(`0.8 - 0.2 !== 0.6`).
    expect(path[0]!.x).toBeCloseTo(120, 6);
    expect(path[0]!.y).toBeCloseTo(65, 6);
    expect(path[1]!.x).toBeCloseTo(230, 6);
    expect(path[1]!.y).toBeCloseTo(65, 6);
  });
});
