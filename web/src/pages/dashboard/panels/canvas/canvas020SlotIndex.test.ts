// 자리 번호는 **논리 구간**이다 (SPEC-CANVAS-020).
//
// 신고: "꺾임점 추가시 다른 라인에 대해서는 꺾임점 추가 안됨." 직각 연결선에서 첫 구간
// 말고는 눌러도 아무 일이 없었다. 그려진 명령의 **차례**를 자리로 쓴 탓인데, 015 가 점
// 사이에 모서리를 끼우면서 차례와 구간이 갈렸다.
//
// @spec SPEC-CANVAS-020

import { describe, expect, it } from 'vitest';

import { parseCanvasConfig, type CanvasSize } from './canvasConfig';
import type { CanvasProjection } from './canvasGeometry';
import { connectorDrawn, connectorPath } from './connector/connectorPath';
import { connectorObstacles } from './connector/connectorObstacles';
import { resolveConnector } from './connector/resolveConnector';
import { connectorPointGestureAt, insertPointAt } from './connector/connectorEdit';
import { isConnector, type ConnectorElement, type ConnectorRoute } from './connector/connectorTypes';
import type { CanvasNode } from './group/groupTypes';
import type { CanvasPoint } from './canvasGeometry';

const CANVAS: CanvasSize = { width: 400, height: 300 };
/** 축척이 하나이고 1:1 이다 — 시험이 환산을 들고 다니지 않는다. */
const PROJ: CanvasProjection = { stage: { width: 400, height: 300 }, canvas: CANVAS };

/**
 * 신고된 그 장면. 그려지는 모양은 `(120,65) → (175,65) → (175,205) → (230,205)` 다 —
 * 가로 · 세로 · 가로 **세 구간**.
 */
function scene(route: ConnectorRoute, points?: readonly { x: number; y: number }[]): CanvasNode[] {
  return parseCanvasConfig({
    canvas: { ...CANVAS },
    elements: [
      { id: 'a', kind: 'rect', geometry: { x: 40, y: 40, w: 80, h: 50 }, style: {} },
      { id: 'b', kind: 'rect', geometry: { x: 230, y: 180, w: 80, h: 50 }, style: {} },
      {
        id: 'c1',
        kind: 'connector',
        from: { el: 'a', a: 'e' },
        to: { el: 'b', a: 'w' },
        route,
        ...(points === undefined ? {} : { points }),
      },
    ],
  }).elements;
}

function connectorOf(els: readonly CanvasNode[]): ConnectorElement {
  const c = els.find((n) => n.id === 'c1');
  if (c === undefined || !isConnector(c)) throw new Error('연결선이 없다');
  return c;
}

/** 해석된 점 목록 — 없으면 시험의 장면이 잘못 지어진 것이므로 그 자리에서 끊는다. */
function resolved(els: readonly CanvasNode[]): readonly CanvasPoint[] {
  const points = resolveConnector(connectorOf(els), els, PROJ, {});
  if (points === undefined) throw new Error('연결이 끊겼다');
  return points;
}

/** 한 자리를 눌렀을 때 나는 자리 번호와, 그것이 **실제로 먹히는지**. */
function press(
  els: readonly CanvasNode[],
  at: { x: number; y: number },
): { slot: number | undefined; taken: boolean } {
  const c = connectorOf(els);
  const gesture = connectorPointGestureAt(
    resolved(els),
    c.route,
    at,
    PROJ,
    9,
    connectorObstacles(c, els, PROJ, {}),
  );
  if (gesture === undefined || gesture.kind !== 'insert') return { slot: undefined, taken: false };
  const next = insertPointAt(c, gesture.index, gesture.at);
  // **`insertPointAt` 은 범위 밖 자리를 받으면 받은 것을 그대로 돌려준다.** 그 동일성이
  // 곧 "조용히 아무 일도 없었다" 이며, 신고된 형상이 바로 그것이다.
  return { slot: gesture.index, taken: next !== c };
}

// --- ① 신고된 그 장면 (REQ-01) ----------------------------------------------

describe('직각 선의 모든 구간이 점을 받는다 (REQ-01)', () => {
  /** 구간 셋 위의 다섯 자리 — 첫 가로 · 세로 셋 · 마지막 가로. */
  const ON_INK = [
    { at: { x: 150, y: 65 }, 구간: '첫 가로' },
    { at: { x: 175, y: 100 }, 구간: '가운데 세로(위)' },
    { at: { x: 175, y: 150 }, 구간: '가운데 세로(가운데)' },
    { at: { x: 175, y: 190 }, 구간: '가운데 세로(아래)' },
    { at: { x: 200, y: 205 }, 구간: '마지막 가로' },
  ] as const;

  it.each(ON_INK)('$구간 — 점이 찍힌다', ({ at }) => {
    expect(press(scene('ortho'), at).taken).toBe(true);
  });

  it('**다섯 자리가 모두** 먹는다 — 세어서 적는다', () => {
    const taken = ON_INK.filter(({ at }) => press(scene('ortho'), at).taken);
    expect(taken).toHaveLength(ON_INK.length);
  });

  it('점이 하나 있어도 네 자리가 모두 먹는다', () => {
    const els = scene('ortho', [{ x: 175, y: 150 }]);
    const ats = [
      { x: 150, y: 65 },
      { x: 175, y: 120 },
      { x: 175, y: 180 },
      { x: 205, y: 205 },
    ];
    expect(ats.filter((at) => press(els, at).taken)).toHaveLength(4);
  });
});

// --- ② 자리 번호의 뜻 (REQ-02) ----------------------------------------------

describe('자리 번호는 논리 구간이다 (REQ-02 · K1)', () => {
  it('점이 없으면 **구간이 하나**이므로 어디를 눌러도 0 이다', () => {
    // 그려진 구간은 셋이지만 논리 구간은 `from → to` 하나다. 그려진 차례(0·1·2)를 쓰면
    // 1 과 2 가 범위 밖이 되고, 그것이 신고된 결함이었다.
    const slots = [
      { x: 150, y: 65 },
      { x: 175, y: 150 },
      { x: 200, y: 205 },
    ].map((at) => press(scene('ortho'), at).slot);
    expect(slots).toEqual([0, 0, 0]);
  });

  it('점이 하나면 앞뒤가 0 과 1 로 갈린다', () => {
    const els = scene('ortho', [{ x: 175, y: 150 }]);
    expect(press(els, { x: 150, y: 65 }).slot).toBe(0);
    expect(press(els, { x: 205, y: 205 }).slot).toBe(1);
  });

  it('어떤 자리를 눌러도 번호가 `0 … points.length` 안이다 (K1)', () => {
    for (const points of [undefined, [{ x: 175, y: 150 }], [{ x: 160, y: 65 }, { x: 175, y: 190 }]]) {
      const els = scene('ortho', points);
      const max = points?.length ?? 0;
      for (let y = 66; y < 205; y += 7) {
        const slot = press(els, { x: 175, y }).slot;
        if (slot === undefined) continue;
        expect(slot).toBeGreaterThanOrEqual(0);
        expect(slot).toBeLessThanOrEqual(max);
      }
    }
  });
});

// --- ③ 다른 갈래는 그대로 (REQ-04 · K3) --------------------------------------

describe('직각이 아닌 갈래는 한 글자도 바뀌지 않는다 (REQ-04 · K3)', () => {
  it.each(['straight', 'elbow', 'curve'] as const)('%s — 주인 목록이 항등이다', (route) => {
    const els = scene(route, [{ x: 170, y: 90 }]);
    const c = connectorOf(els);
    const { commands, owner } = connectorDrawn(
      resolved(els),
      route,
      PROJ,
      connectorObstacles(c, els, PROJ, {}),
    );
    // 명령 `k` 의 주인은 `k` 다 — 점 하나가 명령 하나이므로 차례가 곧 구간이다.
    expect(owner).toEqual(commands.slice(1).map((_cmd, i) => i));
  });

  it.each(['straight', 'curve'] as const)('%s — 누른 자리의 번호가 종전과 같다', (route) => {
    const els = scene(route, [{ x: 170, y: 90 }]);
    // 0.1.0 이전에 관측한 값이다(같은 장면·같은 자리). 바뀌면 고친 것보다 잃은 것이 많다.
    expect(press(els, { x: 140, y: 74 }).slot).toBe(0);
    expect(press(els, { x: 205, y: 160 }).slot).toBe(1);
  });
});

// --- ④ 그림은 바뀌지 않는다 (REQ-05 · K2) ------------------------------------

describe('connectorPath 의 출력은 019 와 같다 (REQ-05 · K2)', () => {
  it.each(['straight', 'elbow', 'curve', 'ortho'] as const)(
    '%s — `connectorPath` 는 `connectorDrawn` 의 명령 그대로다',
    (route) => {
      for (const points of [undefined, [{ x: 175, y: 150 }]]) {
        const els = scene(route, points);
        const c = connectorOf(els);
        const pts = resolved(els);
        const routing = connectorObstacles(c, els, PROJ, {});
        expect(connectorPath(pts, route, PROJ, routing)).toEqual(
          connectorDrawn(pts, route, PROJ, routing).commands,
        );
      }
    },
  );

  it('직각 · 점 없음의 명령이 018 이 못 박은 네 점 그대로다', () => {
    const els = scene('ortho');
    const c = connectorOf(els);
    const cmds = connectorPath(
      resolved(els),
      'ortho',
      PROJ,
      connectorObstacles(c, els, PROJ, {}),
    );
    expect(cmds.map((cmd) => (cmd.c === 'C' ? 'C' : `${cmd.x},${cmd.y}`))).toEqual([
      '120,65',
      '175,65',
      '175,205',
      '229.99999999999997,205',
    ]);
  });

  it('019 의 고정이 살아 있다 — 주인을 실어도 경로가 흔들리지 않는다', () => {
    const els = parseCanvasConfig({
      canvas: { ...CANVAS },
      elements: [
        { id: 'a', kind: 'rect', geometry: { x: 40, y: 40, w: 80, h: 50 }, style: {} },
        { id: 'b', kind: 'rect', geometry: { x: 230, y: 180, w: 80, h: 50 }, style: {} },
        {
          id: 'c1',
          kind: 'connector',
          from: { el: 'a', a: 'e' },
          to: { el: 'b', a: 'w' },
          route: 'ortho',
          ortho_split: 200,
        },
      ],
    }).elements;
    const c = connectorOf(els);
    const cmds = connectorPath(
      resolved(els),
      'ortho',
      PROJ,
      connectorObstacles(c, els, PROJ, {}),
      c.ortho_split,
    );
    expect(cmds.map((cmd) => (cmd.c === 'C' ? 'C' : cmd.x))).toEqual([
      120,
      200,
      200,
      229.99999999999997,
    ]);
  });
});

// --- ⑤ 한 길에서 난다 (REQ-03 · K4) -----------------------------------------

describe('그리는 쪽과 자리 번호가 같은 함수를 지난다 (REQ-03 · K4)', () => {
  it('주인 목록은 명령 목록과 **언제나 나란하다**', () => {
    for (const route of ['straight', 'elbow', 'curve', 'ortho'] as const) {
      for (const points of [undefined, [{ x: 175, y: 150 }], [{ x: 160, y: 65 }, { x: 175, y: 190 }]]) {
        const els = scene(route, points);
        const c = connectorOf(els);
        const { commands, owner } = connectorDrawn(
          resolved(els),
          route,
          PROJ,
          connectorObstacles(c, els, PROJ, {}),
        );
        expect(owner).toHaveLength(Math.max(commands.length - 1, 0));
      }
    }
  });

  it('주인은 **오름차순**이다 — 구간은 되돌아가지 않는다', () => {
    const els = scene('ortho', [{ x: 175, y: 150 }]);
    const c = connectorOf(els);
    const { owner } = connectorDrawn(
      resolved(els),
      'ortho',
      PROJ,
      connectorObstacles(c, els, PROJ, {}),
    );
    expect(owner).toEqual([...owner].sort((l, r) => l - r));
  });

  it('접기가 논리 꼭짓점을 삼키면 **살아남은 점의 주인**이 실린다 (§결정 3)', () => {
    // 사용자 점 (150,65) 는 첫 가로 구간 위에 **그대로 놓인다.** 그러면 `(120,65)` ·
    // `(150,65)` · 그다음 모서리가 한 직선에 서고, `collapseCollinear` 가 가운데를 삼킨다.
    //
    // 삼켜진 것은 구간 0 과 1 의 **경계**였다. 살아남은 점의 주인을 쓰므로 목록은 통째로
    // 1 이 되고, 그 구간을 누른 누름은 **한 자리 늦은** 번호를 받는다 — 새 점이 기존 점보다
    // 뒤에 놓인다. 둘이 한 직선 위에 있으므로 그려지는 그림은 사실상 같다.
    //
    // 접기를 멈추면 017 K3(017 이전과 바이트 동일)이 깨지므로 그림이 같은 쪽을 골랐다.
    // 고른 것을 **값으로** 적어 둔다 — 적지 않으면 다음 사람이 이것을 결함으로 읽는다.
    const els = scene('ortho', [{ x: 150, y: 65 }]);
    const c = connectorOf(els);
    const { commands, owner } = connectorDrawn(
      resolved(els),
      'ortho',
      PROJ,
      connectorObstacles(c, els, PROJ, {}),
    );
    // 삼켜졌다 — 여섯 점이 아니라 넷이다.
    expect(commands).toHaveLength(4);
    expect(owner).toEqual([1, 1, 1]);
    // 그래도 **범위 밖으로는 나가지 않는다**(K1): 점이 하나이므로 1 이 마지막 자리다.
    expect(Math.max(...owner)).toBeLessThanOrEqual(c.points?.length ?? 0);
  });

  it('빈 목록이 들어오면 둘 다 비어 있다', () => {
    const empty = connectorDrawn([], 'ortho', PROJ);
    expect(empty.commands).toEqual([]);
    expect(empty.owner).toEqual([]);
  });
});
