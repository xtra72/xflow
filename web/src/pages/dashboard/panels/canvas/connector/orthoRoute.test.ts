// 직각 경로의 장애물 회피 (SPEC-CANVAS-017 M1 · REQ-01 · REQ-02 · REQ-06).
//
// **이 파일이 지키는 것은 불변식 둘이다**: 모든 구간이 한 축 위이고(K1), 넓힌 장애물의
// **안**을 지나지 않는다(K2). 나머지는 그 둘 위에서 "짧고 덜 꺾인다" 를 잰다.
//
// @spec SPEC-CANVAS-017

import { describe, expect, it } from 'vitest';

import type { CanvasBox } from '../canvasGeometry';
import {
  ORTHO_MARGIN,
  ORTHO_MAX_OBSTACLES,
  ORTHO_TURN_COST,
  orthoRoute,
} from './orthoRoute';

interface Pt {
  x: number;
  y: number;
}

/** 넓힌 장애물. 판정이 쓰는 그 상자를 시험도 같은 식으로 얻는다. */
function inflated(b: CanvasBox): CanvasBox {
  return {
    x: b.x - ORTHO_MARGIN,
    y: b.y - ORTHO_MARGIN,
    w: b.w + ORTHO_MARGIN * 2,
    h: b.h + ORTHO_MARGIN * 2,
  };
}

/** 구간이 상자의 **안**을 지나는가 — 경계는 안이 아니다. */
function segmentEntersBox(a: Pt, b: Pt, box: CanvasBox): boolean {
  const steps = 200;
  for (let i = 1; i < steps; i += 1) {
    const t = i / steps;
    const x = a.x + (b.x - a.x) * t;
    const y = a.y + (b.y - a.y) * t;
    if (x > box.x && x < box.x + box.w && y > box.y && y < box.y + box.h) return true;
  }
  return false;
}

function assertOrthogonal(path: readonly Pt[]): void {
  for (let i = 1; i < path.length; i += 1) {
    const a = path[i - 1]!;
    const b = path[i]!;
    expect(a.x === b.x || a.y === b.y, `${JSON.stringify(a)}→${JSON.stringify(b)}`).toBe(true);
  }
}

function assertClear(path: readonly Pt[], obstacles: readonly CanvasBox[]): void {
  for (const box of obstacles.map(inflated)) {
    for (let i = 1; i < path.length; i += 1) {
      expect(
        segmentEntersBox(path[i - 1]!, path[i]!, box),
        `${JSON.stringify(path[i - 1])}→${JSON.stringify(path[i])} vs ${JSON.stringify(box)}`,
      ).toBe(false);
    }
  }
}

function turns(path: readonly Pt[]): number {
  let n = 0;
  for (let i = 2; i < path.length; i += 1) {
    const horizontalBefore = path[i - 2]!.y === path[i - 1]!.y;
    const horizontalNow = path[i - 1]!.y === path[i]!.y;
    if (horizontalBefore !== horizontalNow) n += 1;
  }
  return n;
}

// --- ① 가로막은 상자를 피한다 (REQ-01 · K1 · K2) ---------------------------

describe('가로막은 상자를 피한다 (REQ-01)', () => {
  /** 두 끝 사이를 정확히 가로막는 상자. */
  const WALL: CanvasBox = { x: 150, y: 50, w: 100, h: 200 };
  const A: Pt = { x: 100, y: 150 };
  const B: Pt = { x: 300, y: 150 };

  it('길을 찾고, 그 길이 상자 안을 지나지 않는다', () => {
    const path = orthoRoute(A, B, [WALL]);
    expect(path).toBeDefined();
    assertOrthogonal(path!);
    assertClear(path!, [WALL]);
  });

  it('두 끝을 **그대로** 지난다', () => {
    const path = orthoRoute(A, B, [WALL])!;
    expect(path[0]).toEqual(A);
    expect(path.at(-1)).toEqual(B);
  });

  it('곧은 길(015 의 세 구간)은 그 상자를 **실제로 가로지른다** — 회피가 필요한 장면이다', () => {
    // 이 단언이 없으면 위 둘이 "애초에 막히지 않은 장면" 에서도 통과한다.
    const naive: Pt[] = [A, { x: 200, y: A.y }, { x: 200, y: B.y }, B];
    const box = inflated(WALL);
    const crossed = naive.some((_, i) => i > 0 && segmentEntersBox(naive[i - 1]!, naive[i]!, box));
    expect(crossed).toBe(true);
  });

  it('위로 돌든 아래로 돌든 **상자 밖**으로 돈다', () => {
    const path = orthoRoute(A, B, [WALL])!;
    const box = inflated(WALL);
    const detour = path.some((p) => p.y <= box.y || p.y >= box.y + box.h);
    expect(detour).toBe(true);
  });
});

// --- ② 짧고 덜 꺾인다 (REQ-02) ----------------------------------------------

describe('짧고 덜 꺾인다 (REQ-02)', () => {
  it('계단이 나오지 않는다 — 꺾임이 손에 꼽는다', () => {
    // 벌점이 없으면 같은 길이의 계단이 최적과 동점이 되어 열 번 꺾어 간다.
    const path = orthoRoute({ x: 0, y: 0 }, { x: 400, y: 300 }, [
      { x: 150, y: 100, w: 60, h: 60 },
    ])!;
    expect(turns(path)).toBeLessThanOrEqual(3);
  });

  it('벌점이 길이와 **같은 저울**에 있다', () => {
    // 상수인 것이 §결정 2 다 — 캔버스 크기에 비례하면 큰 판에서 꺾임이 공짜가 된다.
    expect(ORTHO_TURN_COST).toBeGreaterThan(0);
    expect(ORTHO_TURN_COST).toBeGreaterThan(ORTHO_MARGIN);
  });

  it('가까운 쪽으로 돈다 — 벽의 위아래 가운데 짧은 쪽이다', () => {
    // 끝이 벽의 위쪽에 치우쳐 있으면 위로 도는 길이 짧다.
    const wall: CanvasBox = { x: 150, y: 0, w: 100, h: 300 };
    const path = orthoRoute({ x: 100, y: 20 }, { x: 300, y: 20 }, [wall])!;
    const box = inflated(wall);
    // 도는 자리가 상자 위쪽 밖이다.
    expect(path.some((p) => p.y <= box.y)).toBe(true);
    expect(path.every((p) => p.y < box.y + box.h)).toBe(true);
  });
});

// --- ③ 폴백 (REQ-06 · K5) ---------------------------------------------------

describe('막히거나 클 때는 **부재**다 — 부르는 쪽이 폴백한다 (REQ-06)', () => {
  it('장애물이 없으면 부재다 — 015 의 길로 간다 (K3)', () => {
    expect(orthoRoute({ x: 0, y: 0 }, { x: 100, y: 100 }, [])).toBeUndefined();
  });

  it('상한을 넘으면 부재다', () => {
    const many: CanvasBox[] = Array.from({ length: ORTHO_MAX_OBSTACLES + 1 }, (_, i) => ({
      x: i * 30,
      y: 0,
      w: 10,
      h: 10,
    }));
    expect(orthoRoute({ x: 0, y: 500 }, { x: 900, y: 500 }, many)).toBeUndefined();
  });

  it('사방이 막히면 부재다 — 던지지 않는다', () => {
    // 출발점을 상자 넷이 에워싼다. 길이 없으면 조용히 부재이고, 부르는 쪽이 015 로 간다.
    const A: Pt = { x: 100, y: 100 };
    const ring: CanvasBox[] = [
      { x: 60, y: 40, w: 80, h: 20 },
      { x: 60, y: 140, w: 80, h: 20 },
      { x: 40, y: 60, w: 20, h: 80 },
      { x: 140, y: 60, w: 20, h: 80 },
    ];
    expect(() => orthoRoute(A, { x: 400, y: 400 }, ring)).not.toThrow();
  });
});

// --- ④ 여러 장애물 (K1 · K2 를 장면 여럿에서) --------------------------------

describe('여러 장애물에서도 불변식이 선다', () => {
  const SCENES: readonly (readonly [Pt, Pt, CanvasBox[]])[] = [
    [
      { x: 20, y: 20 },
      { x: 380, y: 280 },
      [
        { x: 120, y: 60, w: 80, h: 80 },
        { x: 220, y: 160, w: 90, h: 70 },
      ],
    ],
    [
      { x: 380, y: 20 },
      { x: 20, y: 280 },
      [
        { x: 100, y: 100, w: 200, h: 40 },
        { x: 40, y: 180, w: 60, h: 60 },
      ],
    ],
    [
      { x: 50, y: 150 },
      { x: 350, y: 150 },
      [
        { x: 120, y: 60, w: 40, h: 180 },
        { x: 220, y: 100, w: 40, h: 180 },
      ],
    ],
  ];

  it.each(SCENES.map((s, i) => [i, s] as const))('장면 %i', (_i, [a, b, boxes]) => {
    const path = orthoRoute(a, b, boxes);
    expect(path, '길을 찾아야 한다').toBeDefined();
    assertOrthogonal(path!);
    assertClear(path!, boxes);
    expect(path![0]).toEqual(a);
    expect(path!.at(-1)).toEqual(b);
  });
});
