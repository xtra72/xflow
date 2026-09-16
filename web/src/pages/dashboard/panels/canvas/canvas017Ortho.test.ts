// 직각 경로가 실제로 도형을 피하고, 잡는 쪽도 같은 길을 본다 (SPEC-CANVAS-017 M2·M3).
//
// **이 파일이 막는 실패는 둘이다.** 그려진 선이 도형을 가로지르는 것(REQ-01)과, 그려진
// 선과 잡히는 선이 갈라지는 것(REQ-04) — 뒤의 것은 002 가 위험 R1 로 이름 적은 그 부류다.
//
// @spec SPEC-CANVAS-017

import { describe, expect, it } from 'vitest';

import { parseCanvasConfig, type CanvasSize } from './canvasConfig';
import type { CanvasProjection, PxBox, PxPoint } from './canvasGeometry';
import { drawElements, type DrawContext2D } from './drawElement';
import { hitTest } from './canvasHitTest';
import { connectorObstacles } from './connector/connectorObstacles';
import { isConnector, type ConnectorElement } from './connector/connectorTypes';
import { ORTHO_MARGIN } from './connector/orthoRoute';
import type { CanvasNode } from './group/groupTypes';

/** 축척이 하나이고 1:1 이다 — 시험이 환산을 들고 다니지 않는다. */
const CANVAS: CanvasSize = { width: 400, height: 300 };
const PROJ: CanvasProjection = { stage: { width: 400, height: 300 }, canvas: CANVAS };

/**
 * 두 도형 사이를 **정확히 가로막는** 벽 하나.
 *
 * 왼쪽 상자의 `e` 앵커(60,150)에서 오른쪽 상자의 `w` 앵커(340,150)로 가는 곧은 길이
 * 이 벽의 한가운데를 지난다.
 */
const SCENE: readonly Record<string, unknown>[] = [
  { id: 'a', kind: 'rect', geometry: { x: 20, y: 130, w: 40, h: 40 }, style: { fill: '#111' } },
  { id: 'b', kind: 'rect', geometry: { x: 340, y: 130, w: 40, h: 40 }, style: { fill: '#222' } },
  { id: 'wall', kind: 'rect', geometry: { x: 170, y: 60, w: 60, h: 180 }, style: { fill: '#333' } },
  {
    id: 'c1',
    kind: 'connector',
    from: { el: 'a', a: 'e' },
    to: { el: 'b', a: 'w' },
    route: 'ortho',
    style: { stroke: '#f0f', strokeWidth: 2 },
  },
];

function scene(nodes: readonly Record<string, unknown>[] = SCENE): CanvasNode[] {
  return parseCanvasConfig({ canvas: { ...CANVAS }, elements: nodes.map((n) => ({ ...n })) }).elements;
}

function connectorOf(nodes: readonly CanvasNode[]): ConnectorElement {
  const found = nodes.find((n): n is ConnectorElement => isConnector(n));
  if (found === undefined) throw new Error('연결선이 없다');
  return found;
}

// --- 기록 스텁 -------------------------------------------------------------

type Recorded = [string, ...unknown[]];

function makeRecorder(): DrawContext2D & { calls: Recorded[] } {
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
    measureText: (text: string) => ({ width: text.length * 10 }),
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    globalAlpha: 1,
    font: '',
    textAlign: 'left' as CanvasTextAlign,
    textBaseline: 'alphabetic' as CanvasTextBaseline,
  };
}

/** 그려진 연결선의 점 목록 — `moveTo`/`lineTo` 만 뽑는다. */
function drawnLine(nodes: readonly CanvasNode[]): PxPoint[] {
  const ctx = makeRecorder();
  drawElements(ctx, nodes, {}, {}, PROJ);
  const out: PxPoint[] = [];
  let seen = false;
  for (const [op, x, y] of ctx.calls) {
    // 연결선은 배열 마지막이라 마지막 `moveTo` 이후가 그 선이다.
    if (op === 'moveTo') {
      out.length = 0;
      out.push({ x: x as number, y: y as number });
      seen = true;
    } else if (op === 'lineTo' && seen) {
      out.push({ x: x as number, y: y as number });
    }
  }
  return out;
}

function entersBox(a: PxPoint, b: PxPoint, box: PxBox): boolean {
  for (let i = 1; i < 200; i += 1) {
    const t = i / 200;
    const x = a.x + (b.x - a.x) * t;
    const y = a.y + (b.y - a.y) * t;
    if (x > box.x && x < box.x + box.w && y > box.y && y < box.y + box.h) return true;
  }
  return false;
}

// --- ① 그린 선이 도형을 가로지르지 않는다 (REQ-01) --------------------------

describe('그린 선이 벽을 가로지르지 않는다 (REQ-01)', () => {
  it('모든 구간이 한 축 위이고 벽 밖이다', () => {
    const nodes = scene();
    const line = drawnLine(nodes);
    expect(line.length).toBeGreaterThan(2);
    const wall: PxBox = { x: 170 - ORTHO_MARGIN, y: 60 - ORTHO_MARGIN, w: 60 + ORTHO_MARGIN * 2, h: 180 + ORTHO_MARGIN * 2 };
    for (let i = 1; i < line.length; i += 1) {
      const a = line[i - 1]!;
      const b = line[i]!;
      expect(a.x === b.x || a.y === b.y, '한 축 위여야 한다').toBe(true);
      expect(entersBox(a, b, wall), `${JSON.stringify(a)}→${JSON.stringify(b)}`).toBe(false);
    }
  });

  it('벽이 없으면 **015 의 길 그대로**다 (K3)', () => {
    // 장애물이 없으면 격자를 세우지 않고 곧바로 폴백한다 — 017 이전의 그림이 그대로다.
    //
    // 이 장면의 두 앵커는 `y` 가 같으므로 015 가 내는 길도 **곧은 두 점**이다(모서리를
    // 끼우면 길이 0 인 구간이 생긴다 — 015 K2). 넷을 기대했다가 둘을 본 것이 첫 판이었고,
    // 둘이 맞다.
    const noWall = SCENE.filter((n) => n.id !== 'wall');
    const line = drawnLine(scene(noWall));
    expect(line).toEqual([
      { x: 60, y: 150 },
      { x: 340, y: 150 },
    ]);
  });

  it('두 앵커가 어긋나 있으면 015 는 **세 구간**을 낸다 — 폴백이 그 길이다', () => {
    // 위 단언이 "언제나 두 점" 으로 읽히지 않게 짝을 둔다. 폴백은 015 의 산술 그대로다.
    const staggered = SCENE.filter((n) => n.id !== 'wall').map((n) =>
      n.id === 'b' ? { ...n, geometry: { x: 340, y: 40, w: 40, h: 40 } } : n,
    );
    expect(drawnLine(scene(staggered))).toHaveLength(4);
  });

  it('벽이 생기면 그림이 **실제로 달라진다**', () => {
    const noWall = drawnLine(scene(SCENE.filter((n) => n.id !== 'wall')));
    const withWall = drawnLine(scene());
    expect(JSON.stringify(withWall)).not.toBe(JSON.stringify(noWall));
  });
});

// --- ② 두 끝 도형은 장애물이 아니다 (REQ-03) --------------------------------

describe('두 끝 도형은 장애물이 아니다 (REQ-03)', () => {
  it('목록에 `a` 도 `b` 도 없다', () => {
    const nodes = scene();
    const boxes = connectorObstacles(connectorOf(nodes), nodes, PROJ, {});
    // 벽 하나만 남는다 — 연결선은 상자가 없고 두 끝 도형은 빠진다.
    expect(boxes).toHaveLength(1);
    expect(boxes[0]!.x).toBe(170);
  });

  it('보이지 않는 도형도 빠진다', () => {
    const hidden = SCENE.map((n) =>
      n.id === 'wall' ? { ...n, style: { fill: '#333', visible: false } } : n,
    );
    const nodes = scene(hidden);
    expect(connectorObstacles(connectorOf(nodes), nodes, PROJ, {})).toHaveLength(0);
  });
});

// --- ③ 그리는 쪽과 잡는 쪽이 같은 길을 본다 (REQ-04 · K4) -------------------

describe('그린 자리에서 잡힌다 (REQ-04 · K4)', () => {
  it('돌아간 구간의 **한가운데**가 잡힌다', () => {
    // 잡는 쪽이 장애물을 모르면 이 자리는 선에서 멀어 잡히지 않는다 — 그 어긋남이
    // 002 위험 R1 이며, 예외도 경고도 없이 화면에서만 드러난다.
    const nodes = scene();
    const line = drawnLine(nodes);
    // 가장 긴 구간의 한가운데를 고른다.
    let best = { at: { x: 0, y: 0 }, len: -1 };
    for (let i = 1; i < line.length; i += 1) {
      const a = line[i - 1]!;
      const b = line[i]!;
      const len = Math.abs(b.x - a.x) + Math.abs(b.y - a.y);
      if (len > best.len) best = { at: { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 }, len };
    }
    expect(best.len).toBeGreaterThan(0);
    expect(hitTest(nodes, best.at, PROJ, {})).toEqual({ nodeId: 'c1' });
  });

  it('곧은 길이었다면 지났을 자리는 **잡히지 않는다**', () => {
    // 벽 한가운데 — 015 의 길이라면 잉크였을 자리다. 회피 뒤에는 비어 있어야 한다.
    const nodes = scene();
    expect(hitTest(nodes, { x: 200, y: 150 }, PROJ, {})).toEqual({ nodeId: 'wall' });
  });
});
