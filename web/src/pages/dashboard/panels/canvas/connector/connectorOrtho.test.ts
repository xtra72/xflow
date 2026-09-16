// 직각 꺾은선 (SPEC-CANVAS-015 M1 · REQ-01 · REQ-03 · REQ-05).
//
// **이 파일이 지키는 것은 불변식 하나다**: 이 갈래가 내는 모든 구간은 수평이거나 수직이다.
// 그 사실을 한 자리에서 값으로 재고, 나머지 넷의 그림이 **바이트 동일**함도 함께 잰다 —
// 갈래 하나를 더하면서 넷을 건드리지 않았다는 증거다(K5).
//
// @spec SPEC-CANVAS-015 REQ-01 · REQ-05

import { describe, expect, it } from 'vitest';

import type { CanvasPoint, CanvasProjection } from '../canvasGeometry';
import { connectorPath, type ConnectorPathCommand } from './connectorPath';
import { CONNECTOR_ROUTES, isConnectorRoute, type ConnectorRoute } from './connectorTypes';

/** 축척이 하나다 — px 와 캔버스 단위가 1:1 이라 시험이 환산을 들고 다니지 않는다. */
const PROJ: CanvasProjection = { stage: { width: 400, height: 400 }, canvas: { width: 400, height: 400 } };

/** 구간 끝점 목록으로 편다. 직각 갈래에는 `C` 가 나지 않으므로 `M`·`L` 뿐이다. */
function points(cmds: readonly ConnectorPathCommand[]): { x: number; y: number }[] {
  return cmds.map((c) => {
    if (c.c === 'C') throw new Error('직각에 곡선 명령이 났다');
    return { x: c.x, y: c.y };
  });
}

/** 이웃한 두 점이 한 축 위인가 — 수평이거나 수직이다. */
function axisAligned(a: { x: number; y: number }, b: { x: number; y: number }): boolean {
  return a.x === b.x || a.y === b.y;
}

function ortho(pts: readonly CanvasPoint[]): { x: number; y: number }[] {
  return points(connectorPath(pts, 'ortho', PROJ));
}

// --- ① 모든 구간이 직각이다 (REQ-01 · K1) ----------------------------------

describe('모든 구간이 수평이거나 수직이다 (K1)', () => {
  /** 비대칭 장면들 — 가로가 긴 것 · 세로가 긴 것 · 음수 방향 · 점을 든 것. */
  const SCENES: readonly (readonly CanvasPoint[])[] = [
    [
      { x: 10, y: 20 },
      { x: 300, y: 250 },
    ],
    [
      { x: 300, y: 250 },
      { x: 10, y: 20 },
    ],
    [
      { x: 20, y: 10 },
      { x: 60, y: 380 },
    ],
    [
      { x: 10, y: 10 },
      { x: 120, y: 90 },
      { x: 300, y: 40 },
    ],
    [
      { x: -50, y: -30 },
      { x: 17, y: 211 },
    ],
  ];

  it.each(SCENES.map((s, i) => [i, s] as const))('장면 %i 의 모든 구간이 한 축 위다', (_i, pts) => {
    const out = ortho(pts);
    expect(out.length).toBeGreaterThan(1);
    for (let i = 1; i < out.length; i += 1) {
      expect(axisAligned(out[i - 1]!, out[i]!), `${JSON.stringify(out[i - 1])}→${JSON.stringify(out[i])}`).toBe(
        true,
      );
    }
  });

  it('두 끝은 **그대로** 지난다 — 점을 옮겨 저장하지 않는다 (K4 · §결정 3)', () => {
    for (const pts of SCENES) {
      const out = ortho(pts);
      expect(out[0]).toEqual({ x: pts[0]!.x, y: pts[0]!.y });
      expect(out.at(-1)).toEqual({ x: pts.at(-1)!.x, y: pts.at(-1)!.y });
    }
  });

  it('중간점을 **지난다** (REQ-03)', () => {
    const mid = { x: 120, y: 90 };
    const out = ortho([{ x: 10, y: 10 }, mid, { x: 300, y: 40 }]);
    expect(out).toContainEqual(mid);
  });
});

// --- ② 가운데서 꺾는다 (§결정 1) --------------------------------------------

describe('가운데서 꺾는다 (§결정 1)', () => {
  it('가로가 길면 **가운데 x** 에서 꺾는다', () => {
    const out = ortho([
      { x: 0, y: 0 },
      { x: 200, y: 60 },
    ]);
    // 가로 → 세로 → 가로. 모서리 둘의 x 가 가운데(100)다.
    expect(out).toEqual([
      { x: 0, y: 0 },
      { x: 100, y: 0 },
      { x: 100, y: 60 },
      { x: 200, y: 60 },
    ]);
  });

  it('세로가 길면 **가운데 y** 에서 꺾는다', () => {
    const out = ortho([
      { x: 0, y: 0 },
      { x: 60, y: 200 },
    ]);
    expect(out).toEqual([
      { x: 0, y: 0 },
      { x: 0, y: 100 },
      { x: 60, y: 100 },
      { x: 60, y: 200 },
    ]);
  });

  it('두 끝이 **대칭**이다 — L 이 아닌 이유다', () => {
    // 한 번만 꺾는 L 은 두 끝 가운데 한쪽에 붙으므로 같은 두 점을 이어도 어느 쪽에
    // 붙느냐로 그림이 달라진다. 가운데서 꺾으면 뒤집어 이어도 같은 선이 나온다.
    const a = ortho([
      { x: 0, y: 0 },
      { x: 200, y: 60 },
    ]);
    const b = ortho([
      { x: 200, y: 60 },
      { x: 0, y: 0 },
    ]);
    expect([...b].reverse()).toEqual(a);
  });
});

// --- ③ 길이 0 인 구간을 내지 않는다 (REQ-05 · K2) ---------------------------

describe('길이 0 인 구간을 내지 않는다 (K2 · REQ-05)', () => {
  it.each([
    ['수평', [{ x: 10, y: 50 }, { x: 300, y: 50 }]],
    ['수직', [{ x: 50, y: 10 }, { x: 50, y: 300 }]],
  ] as const)('이미 %s 이면 모서리가 없다', (_label, pts) => {
    const out = ortho(pts);
    expect(out).toHaveLength(2);
  });

  it('어느 장면에서도 같은 점이 잇달아 나지 않는다', () => {
    const out = ortho([
      { x: 0, y: 0 },
      { x: 200, y: 60 },
      { x: 200, y: 200 },
    ]);
    for (let i = 1; i < out.length; i += 1) {
      expect(out[i]).not.toEqual(out[i - 1]);
    }
  });
});

// --- ④ 넷째까지는 한 글자도 바뀌지 않았다 (K5) ------------------------------

describe('넷째까지의 갈래가 **바이트 동일**하다 (K5)', () => {
  const pts: CanvasPoint[] = [
    { x: 10, y: 20 },
    { x: 120, y: 90 },
    { x: 300, y: 250 },
  ];

  it.each(['straight', 'elbow', 'free'] as const)('`%s` 는 여전히 곧은 폴리라인이다', (route) => {
    // 셋은 015 이전과 같은 길을 지난다 — 점을 그대로 잇는다.
    expect(connectorPath(pts, route, PROJ)).toEqual([
      { c: 'M', x: 10, y: 20 },
      { c: 'L', x: 120, y: 90 },
      { c: 'L', x: 300, y: 250 },
    ]);
  });

  it('`curve` 는 여전히 곡선 명령을 낸다', () => {
    const out = connectorPath(pts, 'curve', PROJ);
    expect(out.some((c) => c.c === 'C')).toBe(true);
  });

  it('직각만 갈린다 — 같은 점으로 다른 그림이 난다', () => {
    expect(connectorPath(pts, 'ortho', PROJ)).not.toEqual(connectorPath(pts, 'elbow', PROJ));
  });
});

// --- ⑤ 어휘가 한 자리에 있다 (파서 결함의 재발 방지) ------------------------

describe('그리는 법 목록이 한 자리다', () => {
  it('다섯이고 그 다섯이다', () => {
    expect([...CONNECTOR_ROUTES].sort()).toEqual(['curve', 'elbow', 'free', 'ortho', 'straight']);
  });

  it.each(CONNECTOR_ROUTES)('`%s` 를 판별이 받아들인다', (route) => {
    expect(isConnectorRoute(route)).toBe(true);
  });

  it.each([undefined, null, '', 'Ortho', 'ORTHO', 'zigzag', 0, {}])('%o 를 거절한다', (bad) => {
    expect(isConnectorRoute(bad)).toBe(false);
  });

  it('타입과 목록이 같은 집합이다 — 여섯째가 생기면 여기가 운다', () => {
    const seen: Record<ConnectorRoute, true> = {
      straight: true,
      elbow: true,
      ortho: true,
      curve: true,
      free: true,
    };
    expect(Object.keys(seen).sort()).toEqual([...CONNECTOR_ROUTES].sort());
  });
});
