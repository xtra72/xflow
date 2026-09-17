// 직각 선의 가운데 구간을 잡아 옮긴다 — 저장과 경로 (SPEC-CANVAS-019 M1 · M2).
//
// 몸짓(손잡이)은 `canvas019SplitHandle.test.tsx` 가 진다. 이 파일은 **값으로 잴 수 있는
// 것**만 본다: 저장 규율과 그려진 길.
//
// @spec SPEC-CANVAS-019 REQ-03 · REQ-04 · REQ-06

import { describe, expect, it } from 'vitest';

import { parseCanvasConfig, type CanvasSize } from './canvasConfig';
import type { CanvasProjection, PxPoint } from './canvasGeometry';
import { drawElements, type DrawContext2D } from './drawElement';
import { isConnector } from './connector/connectorTypes';
import type { CanvasNode } from './group/groupTypes';

const CANVAS: CanvasSize = { width: 400, height: 300 };
const PROJ: CanvasProjection = { stage: { width: 400, height: 300 }, canvas: CANVAS };

function scene(rightY: number, split?: number): CanvasNode[] {
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
        ...(split !== undefined ? { ortho_split: split } : {}),
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

/**
 * **SPEC-CANVAS-021 이 저장 형상을 넓혔다** — 수 하나에서 **구간과 나란한 목록**으로.
 *
 * 019 의 단언을 지우지 않고 그대로 두되, 읽는 자리에서 한 겹을 벗긴다. 019 가 다루는
 * 장면은 논리 구간이 **하나뿐**이므로 목록의 첫 자리가 곧 019 의 그 수다 — 019 가 물은
 * 질문("왕복하는가 · 0 도 값인가 · 손상되면 버리는가")은 한 글자도 달라지지 않는다.
 *
 * 019 가 적은 **수 하나도 그대로 읽힌다**(`parseOrthoSplits` 가 `[그 수]` 로 읽는다) —
 * 이 파일이 장면을 수 하나로 짓는데도 통과하는 것이 그 사실의 증거다.
 */
function splitOf(nodes: readonly CanvasNode[]): number | undefined {
  const c = nodes.find((n) => isConnector(n));
  if (c === undefined || !isConnector(c)) return undefined;
  const first = c.ortho_split?.[0];
  return typeof first === 'number' ? first : undefined;
}

// --- ① 저장 (REQ-06) --------------------------------------------------------

describe('저장 규율 (REQ-06)', () => {
  it('왕복한다', () => {
    expect(splitOf(scene(180, 175))).toBe(175);
  });

  it('0 도 값이다 — 좌표이지 "없음" 이 아니다', () => {
    // 각도(014)와 갈리는 자리다. 저쪽은 0 이 "돌지 않음" 이지만 여기 0 은 **캔버스 왼쪽
    // 끝**이라는 자리이고, 지우면 사용자가 거기 둔 선이 자동으로 튄다.
    expect(splitOf(scene(180, 0))).toBe(0);
  });

  it.each([Number.NaN, Number.POSITIVE_INFINITY, '175', null, {}])(
    '손상된 %o 는 키를 버린다 — 요소는 살아 있다',
    (bad) => {
      const nodes = parseCanvasConfig({
        canvas: { ...CANVAS },
        elements: [
          {
            id: 'c1',
            kind: 'connector',
            from: { el: 'a', a: 'e' },
            to: { el: 'b', a: 'w' },
            route: 'ortho',
            ortho_split: bad,
          },
        ],
      }).elements;
      expect(nodes).toHaveLength(1);
      expect(splitOf(nodes)).toBeUndefined();
    },
  );

  it('직각이 아니면 키를 만들지 않는다', () => {
    // 다른 갈래에는 "가운데 구간" 이 없으므로 그 수가 뜻을 갖지 못한다.
    const nodes = parseCanvasConfig({
      canvas: { ...CANVAS },
      elements: [
        {
          id: 'c1',
          kind: 'connector',
          from: { el: 'a', a: 'e' },
          to: { el: 'b', a: 'w' },
          route: 'elbow',
          ortho_split: 175,
        },
      ],
    }).elements;
    expect(splitOf(nodes)).toBeUndefined();
  });
});

// --- ② 경로가 그 자리를 쓴다 (REQ-03 · REQ-04 · K2) -------------------------

describe('경로가 고른 자리를 쓴다 (REQ-03)', () => {
  it('가운데 세로선이 고른 x 에 선다', () => {
    const path = line(scene(180, 190));
    expect(path).toHaveLength(4);
    expect(path[1]!.x).toBeCloseTo(190, 6);
    expect(path[2]!.x).toBeCloseTo(190, 6);
  });

  it('없으면 018 과 **바이트 동일**하다 (K2)', () => {
    expect(line(scene(180))).toEqual(line(scene(180, undefined)));
  });

  it('모든 구간이 여전히 한 축 위다 (K1)', () => {
    const path = line(scene(180, 190));
    for (let i = 1; i < path.length; i += 1) {
      expect(path[i - 1]!.x === path[i]!.x || path[i - 1]!.y === path[i]!.y).toBe(true);
    }
  });
});

describe('상자를 옮겨도 그 자리를 지킨다 (REQ-04)', () => {
  it('가운데 x 가 그대로이고 양옆만 늘고 준다', () => {
    // **이것이 점 둘을 기각한 이유다.** 점으로 적었다면 상자를 옮기는 순간 저장된 점이
    // 낡아 첫 구간이 수평을 잃고, 018 이 그 사이에 꺾임을 끼워 넣는다.
    const before = line(scene(120, 190));
    const after = line(scene(220, 190));
    expect(before.map((p) => p.x)).toEqual(after.map((p) => p.x));
    expect(before[1]!.x).toBeCloseTo(190, 6);
    expect(after[1]!.x).toBeCloseTo(190, 6);
    // 세로만 움직였다.
    expect(after[2]!.y).not.toBeCloseTo(before[2]!.y, 6);
  });

  it('고정하면 **피하지 않는다** (§결정 2)', () => {
    // 사용자가 그 자리를 골랐다. 거기서 다시 피해 돌면 옮긴 자리가 지켜지지 않는다.
    const walled = parseCanvasConfig({
      canvas: { ...CANVAS },
      elements: [
        { id: 'a', kind: 'rect', geometry: { x: 40, y: 40, w: 80, h: 50 }, style: {} },
        { id: 'b', kind: 'rect', geometry: { x: 230, y: 180, w: 80, h: 50 }, style: {} },
        { id: 'w', kind: 'rect', geometry: { x: 180, y: 60, w: 30, h: 180 }, style: {} },
        {
          id: 'c1',
          kind: 'connector',
          from: { el: 'a', a: 'e' },
          to: { el: 'b', a: 'w' },
          route: 'ortho',
          ortho_split: 195,
          style: { stroke: '#4a8', strokeWidth: 2 },
        },
      ],
    }).elements;
    const path = line(walled);
    // 벽 한가운데를 고르고도 그 자리에 선다 — 라우터가 돌지 않았다는 뜻이다.
    expect(path).toHaveLength(4);
    expect(path[1]!.x).toBeCloseTo(195, 6);
  });
});
