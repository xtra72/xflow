// 자유선의 궤적 모듈 (SPEC-CANVAS-011 M11 · REQ-06 · AC-74~AC-77).
//
// ## 이 파일이 겨누는 실패
//
// 간소화는 **개수로만 재면 언제나 속일 수 있다.** "끝점 둘만 남긴다" 는 어떤 궤적에서도
// 점 수를 줄이므로 AC-75 를 통과하지만, 사용자가 그린 곡선을 직선 하나로 바꿔 놓는다.
// 그래서 이 파일의 중심 단언은 개수가 아니라 **기하 성질**이다 — 버려진 점은 전부 남은
// 폴리라인에서 허용 오차 안에 있다(아래 §모양을 지킨다).
//
// 그 성질을 재는 자(`strayFromPolyline`)는 제품 코드를 부르지 않고 여기서 따로 적는다.
// 제품의 거리 함수를 그대로 빌려 쓰면 그 함수가 틀렸을 때 시험도 함께 틀린다.
//
// **jsdom 이 필요 없다.** 점과 숫자뿐이다.
//
// @spec SPEC-CANVAS-011 REQ-06 · AC-74 · AC-75 · AC-76 · AC-77

import { describe, expect, it } from 'vitest';

import { parseCanvasConfig, type PointGeometry } from '../canvasConfig';
import { appendConnector } from '../canvasElementFactory';
import { HIT_TOLERANCE_PX } from '../canvasHitTest';
import { MAX_PATH_COMMANDS } from '../shapes/pathTypes';
import { MAX_CONNECTOR_POINTS, isConnector, type ConnectorRoute } from './connectorTypes';
import {
  FREEHAND_TOLERANCE,
  ROUTE_TRACES_TRAIL,
  freehandPoints,
  simplifyTrail,
  takeFreehandSample,
} from './freehand';

// --- 재는 자 ---------------------------------------------------------------

/** 점–선분 거리. **제품 코드를 부르지 않는다**(파일 머리말). */
function strayFromSegment(p: PointGeometry, a: PointGeometry, b: PointGeometry): number {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  const spanSq = dx * dx + dy * dy;
  if (spanSq === 0) return Math.hypot(p.x - a.x, p.y - a.y);
  const t = Math.min(1, Math.max(0, ((p.x - a.x) * dx + (p.y - a.y) * dy) / spanSq));
  return Math.hypot(p.x - (a.x + t * dx), p.y - (a.y + t * dy));
}

/** 점이 폴리라인에서 벗어난 거리 — 모든 구간 가운데 가장 가까운 것. */
function strayFromPolyline(p: PointGeometry, poly: readonly PointGeometry[]): number {
  const head = poly[0];
  if (head === undefined) return Number.POSITIVE_INFINITY;
  let best = Math.hypot(p.x - head.x, p.y - head.y);
  for (let i = 1; i < poly.length; i += 1) {
    const a = poly[i - 1];
    const b = poly[i];
    if (a === undefined || b === undefined) continue;
    best = Math.min(best, strayFromSegment(p, a, b));
  }
  return best;
}

/** 궤적 전체가 줄인 선에서 오차 안에 있는가 — 가장 멀리 벗어난 한 점의 거리. */
function worstStray(
  trail: readonly PointGeometry[],
  poly: readonly PointGeometry[],
): number {
  return trail.reduce((worst, p) => Math.max(worst, strayFromPolyline(p, poly)), 0);
}

// --- 고정 입력 -------------------------------------------------------------

/** 표본 목록을 받는 쪽에 차례로 먹인다 — 실제 몸짓이 하는 그대로. */
function collect(samples: readonly { x: number; y: number }[]): readonly PointGeometry[] {
  let trail: readonly PointGeometry[] = [];
  for (const at of samples) trail = takeFreehandSample(trail, at);
  return trail;
}

/** 손이 그은 원호 — 곡률이 있어 직선 하나로는 오차 안에 들 수 없다. */
function arc(count: number, radius = 200): PointGeometry[] {
  return Array.from({ length: count }, (_u, i) => {
    const t = (i / (count - 1)) * (Math.PI / 2);
    return { x: Math.round(radius * Math.sin(t)), y: Math.round(radius * (1 - Math.cos(t))) };
  });
}

/**
 * **간소화로는 줄지 않는** 궤적 — 사건마다 오차의 여덟 배(24)로 꺾는다.
 *
 * 상한을 재는 시험은 이 모양이라야 뜻이 있다. 완만한 궤적을 쓰면 허용 오차가 먼저 점을
 * 줄여 버려, 상한을 통째로 걷어내도 시험이 초록으로 남는다(실측: 완만한 600 점은 54 점
 * 으로 접힌다). 아래 §전제 시험이 이 성질을 **매번 다시 확인한다** — 고정 입력이 언젠가
 * 완만해지면 그 줄이 먼저 운다.
 */
function zigzag(count: number): PointGeometry[] {
  return Array.from({ length: count }, (_u, i) => ({ x: i * 3, y: (i % 2) * 24 }));
}

/** 거의 직선인 궤적 — 흔들림이 허용 오차 아래다. */
function nearlyStraight(count: number): PointGeometry[] {
  return Array.from({ length: count }, (_u, i) => ({
    x: i * 5,
    y: i % 2 === 0 ? 0 : 2,
  }));
}

// --- 상수 ------------------------------------------------------------------

describe('상한은 008 의 명령 상한에서 **파생**한다 (REQ-06)', () => {
  it('두 수가 같다 — 베껴 적은 두 번째 상한이 없다', () => {
    // 갈리면 저술에서는 통과한 궤적이 저장 왕복에서 조용히 잘린다. 파서가 같은 이름을
    // 보는지는 `canvas011Freehand.test.tsx` 의 왕복 시험이 값으로 잰다.
    expect(MAX_CONNECTOR_POINTS).toBe(MAX_PATH_COMMANDS);
  });

  it('그 수가 008 이 적은 256 그대로다', () => {
    expect(MAX_CONNECTOR_POINTS).toBe(256);
  });
});

describe('허용 오차는 집기 여유에서 **파생**한다', () => {
  it('집기 여유의 절반이다 — 숫자를 새로 적지 않았다', () => {
    expect(FREEHAND_TOLERANCE).toBe(HIT_TOLERANCE_PX / 2);
  });

  it('집기 반경보다 **작다** — 간소화가 선을 손 밑에서 빼내지 못한다', () => {
    // 이 부등호가 뒤집히면 "분명히 여기 그었는데 잡히지 않는" 자리가 표현 가능해진다.
    expect(FREEHAND_TOLERANCE).toBeLessThan(HIT_TOLERANCE_PX);
    expect(FREEHAND_TOLERANCE).toBeGreaterThan(0);
  });
});

describe('궤적을 받는 갈래는 **자유선 하나**다 (REQ-06)', () => {
  it('표가 **다섯** 갈래를 총망라한다 (015)', () => {
    const routes: ConnectorRoute[] = ['straight', 'elbow', 'ortho', 'curve', 'free'];
    expect(Object.keys(ROUTE_TRACES_TRAIL).sort()).toEqual([...routes].sort());
  });

  it('`free` 만 참이다', () => {
    expect(ROUTE_TRACES_TRAIL.free).toBe(true);
    expect(ROUTE_TRACES_TRAIL.straight).toBe(false);
    expect(ROUTE_TRACES_TRAIL.elbow).toBe(false);
    expect(ROUTE_TRACES_TRAIL.curve).toBe(false);
    // 직각도 손으로 긋는 갈래가 아니다 — 궤적에서 점을 뽑는 일은 자유선 하나의 몫이고,
    // 직각은 두 끝에서 모서리를 **지어내는** 갈래다(015).
    expect(ROUTE_TRACES_TRAIL.ortho).toBe(false);
  });
});

// --- 받기 ------------------------------------------------------------------

describe('궤적을 받는다', () => {
  it('표본이 차례대로 쌓인다', () => {
    expect(collect([{ x: 0, y: 0 }, { x: 10, y: 5 }, { x: 20, y: 30 }])).toEqual([
      { x: 0, y: 0 },
      { x: 10, y: 5 },
      { x: 20, y: 30 },
    ]);
  });

  it('**정수로 죈다** — 파서의 그 반올림과 같은 규칙이다', () => {
    expect(collect([{ x: 10.4, y: 5.6 }, { x: -0.5, y: 2.49 }])).toEqual([
      { x: 10, y: 6 },
      { x: -0, y: 2 },
    ]);
  });

  it('유한하지 않은 자리는 0 으로 떨어진다 — 던지지 않는다', () => {
    expect(collect([{ x: Number.NaN, y: Number.POSITIVE_INFINITY }])).toEqual([{ x: 0, y: 0 }]);
  });

  it('직전과 **같은 정수 자리**는 받지 않는다 — 손이 멈춰 있어도 자라지 않는다', () => {
    const trail = collect([
      { x: 10, y: 10 },
      { x: 10.2, y: 9.8 },
      { x: 10.4, y: 10.1 },
      { x: 11, y: 10 },
    ]);
    expect(trail).toEqual([{ x: 10, y: 10 }, { x: 11, y: 10 }]);
  });

  it('받을 것이 없으면 **같은 배열**을 돌려준다 — 새 배열을 흘리지 않는다', () => {
    const trail = takeFreehandSample([], { x: 3, y: 4 });
    expect(takeFreehandSample(trail, { x: 3.2, y: 4.1 })).toBe(trail);
  });
});

describe('상한에 닿으면 **더 받지 않되 던지지 않는다** (AC-76 · AC-77)', () => {
  /** 정수 자리가 전부 다른 아주 긴 궤적 — 상한의 네 배를 먹인다. */
  const LONG = Array.from({ length: MAX_CONNECTOR_POINTS * 4 }, (_u, i) => ({ x: i, y: 0 }));

  it('쌓인 표본이 상한을 넘지 않는다', () => {
    expect(collect(LONG)).toHaveLength(MAX_CONNECTOR_POINTS);
  });

  it('상한에 닿은 뒤에도 **예외 없이** 계속 받아 준다', () => {
    let trail = collect(LONG);
    expect(() => {
      for (let i = 0; i < 100; i += 1) trail = takeFreehandSample(trail, { x: 9000 + i, y: 7 });
    }).not.toThrow();
    // 몸짓은 이어졌고 궤적만 그대로다 — 끊긴 흔적이 남지 않는다.
    expect(trail).toHaveLength(MAX_CONNECTOR_POINTS);
  });

  it('상한 뒤의 표본은 **앞을 밀어내지 않는다** — 앞에서부터 살린다', () => {
    const trail = collect(LONG);
    expect(trail[0]).toEqual({ x: 0, y: 0 });
    expect(trail[MAX_CONNECTOR_POINTS - 1]).toEqual({ x: MAX_CONNECTOR_POINTS - 1, y: 0 });
  });
});

// --- 간소화 ----------------------------------------------------------------

describe('모양을 지킨다 — 버려진 점은 전부 오차 안이다 (REQ-06)', () => {
  // **이 절이 이 파일의 중심이다.** 개수만 재면 "끝점 둘만 남긴다" 가 통과한다.
  it.each([3, 8, 40, 200])('점 %i 개짜리 원호에서 가장 먼 벗어남이 오차 이하다', (count) => {
    const trail = arc(count);
    const kept = simplifyTrail(trail, FREEHAND_TOLERANCE);
    expect(worstStray(trail, kept)).toBeLessThanOrEqual(FREEHAND_TOLERANCE + 1e-9);
  });

  it.each([0.5, 1, 3, 12])('오차 %f 로도 그 약속이 참이다', (tolerance) => {
    const trail = arc(120);
    expect(worstStray(trail, simplifyTrail(trail, tolerance))).toBeLessThanOrEqual(
      tolerance + 1e-9,
    );
  });

  it('오차가 클수록 점이 적어진다 — 줄이는 눈금이 실제로 그 눈금이다', () => {
    const trail = arc(120);
    const fine = simplifyTrail(trail, 0.5).length;
    const coarse = simplifyTrail(trail, 12).length;
    expect(coarse).toBeLessThan(fine);
    expect(fine).toBeLessThanOrEqual(trail.length);
  });

  it('**되짚어 온 궤적**에서도 참이다 — 무한 직선이 아니라 선분까지를 잰다', () => {
    // 직선 거리로 재는 고전 구현이 틀리는 그 자리다: 가운데 점은 두 끝점이 이루는 직선
    // 에서는 1 밖에 안 떨어졌지만, **선분**에서는 90 이나 떨어져 있다. 직선으로 재면 그
    // 점이 버려지고 사용자가 지나간 적 없는 자리에 선이 남는다.
    const trail: PointGeometry[] = [
      { x: 0, y: 0 },
      { x: 100, y: 1 },
      { x: 10, y: 0 },
    ];
    const kept = simplifyTrail(trail, FREEHAND_TOLERANCE);
    expect(kept).toHaveLength(3);
    expect(worstStray(trail, kept)).toBeLessThanOrEqual(FREEHAND_TOLERANCE + 1e-9);
  });

  it('꺾인 자리는 남는다 — ㄱ 자가 직선으로 접히지 않는다', () => {
    const trail: PointGeometry[] = [
      { x: 0, y: 0 },
      { x: 50, y: 0 },
      { x: 100, y: 0 },
      { x: 100, y: 50 },
      { x: 100, y: 100 },
    ];
    expect(simplifyTrail(trail, FREEHAND_TOLERANCE)).toEqual([
      { x: 0, y: 0 },
      { x: 100, y: 0 },
      { x: 100, y: 100 },
    ]);
  });
});

describe('거의 직선인 궤적은 **뚜렷이** 줄어든다 (AC-75)', () => {
  it('사건 40 개가 점 둘로 접힌다', () => {
    const trail = nearlyStraight(40);
    const kept = simplifyTrail(trail, FREEHAND_TOLERANCE);
    expect(kept).toHaveLength(2);
    // 그러면서도 모양 약속은 그대로다 — 접힌 것이 옳은 이유로 접혔다.
    expect(worstStray(trail, kept)).toBeLessThanOrEqual(FREEHAND_TOLERANCE + 1e-9);
  });

  it('곡선 궤적도 사건 수보다 한참 적어진다', () => {
    const trail = arc(200);
    expect(simplifyTrail(trail, FREEHAND_TOLERANCE).length).toBeLessThan(trail.length / 4);
  });
});

describe('줄이는 일이 지켜야 할 나머지', () => {
  it('점이 둘 이하면 그대로다 — 다만 **새 객체**로 나온다', () => {
    const trail: PointGeometry[] = [{ x: 1, y: 2 }, { x: 3, y: 4 }];
    const kept = simplifyTrail(trail, FREEHAND_TOLERANCE);
    expect(kept).toEqual(trail);
    expect(kept[0]).not.toBe(trail[0]);
  });

  it('빈 궤적은 빈 목록이다', () => {
    expect(simplifyTrail([], FREEHAND_TOLERANCE)).toEqual([]);
  });

  it('양 끝은 언제나 남는다 — 선이 시작하고 끝나는 자리가 달라지지 않는다', () => {
    const trail = arc(60);
    const kept = simplifyTrail(trail, 1000);
    expect(kept).toHaveLength(2);
    expect(kept[0]).toEqual(trail[0]);
    expect(kept[1]).toEqual(trail[trail.length - 1]);
  });

  it('점을 **늘리지 않는다** — 그래서 상한이 받는 자리 하나로 족하다', () => {
    for (const count of [0, 1, 2, 5, 120]) {
      const trail = arc(Math.max(count, 2)).slice(0, count);
      expect(simplifyTrail(trail, FREEHAND_TOLERANCE).length).toBeLessThanOrEqual(trail.length);
    }
  });

  it('받은 궤적을 **고치지 않는다**', () => {
    const trail = arc(30);
    const before = JSON.stringify(trail);
    simplifyTrail(trail, FREEHAND_TOLERANCE);
    expect(JSON.stringify(trail)).toBe(before);
  });
});

// --- 중간점으로 옮기기 -----------------------------------------------------

describe('끝점과 겹치는 앞뒤 표본은 **중간점이 아니다**', () => {
  const FROM = { x: 0, y: 0 };
  const TO = { x: 300, y: 0 };

  it('놓은 자리와 같은 마지막 표본이 걷힌다 — 손잡이가 겹치지 않는다', () => {
    // `pointerup` 은 대개 마지막 `pointermove` 와 같은 좌표로 온다. 그 표본을 그대로
    // 적으면 끝점 손잡이 밑에 중간점 손잡이가 선다.
    const trail: PointGeometry[] = [
      { x: 100, y: 60 },
      { x: 200, y: 60 },
      { x: 300, y: 0 },
    ];
    const points = freehandPoints(trail, FROM, TO);
    expect(points).not.toContainEqual({ x: 300, y: 0 });
    expect(points.length).toBeGreaterThan(0);
  });

  it('누른 자리와 같은 첫 표본도 걷힌다', () => {
    const trail: PointGeometry[] = [
      { x: 0, y: 0 },
      { x: 150, y: 80 },
      { x: 280, y: 5 },
    ];
    // 끝 쪽 표본(280,5)은 놓은 자리에서 20 남짓 떨어져 있어 **남는다** — 걷어내는 문턱은
    // 허용 오차이지 "마지막이면 무조건" 이 아니다.
    expect(freehandPoints(trail, FROM, TO)).toEqual([
      { x: 150, y: 80 },
      { x: 280, y: 5 },
    ]);
  });

  it('궤적이 통째로 끝점 언저리면 중간점이 하나도 나지 않는다', () => {
    expect(freehandPoints([{ x: 1, y: 1 }, { x: 299, y: 1 }], FROM, TO)).toEqual([]);
  });

  it('빈 궤적은 빈 중간점이다 — 움직임 없는 몸짓이 점을 지어내지 않는다', () => {
    expect(freehandPoints([], FROM, TO)).toEqual([]);
  });

  it('허용 오차를 **그 상수로** 묶는다 — 부르는 쪽이 고르지 않는다', () => {
    const trail = arc(80).map((p) => ({ x: p.x + 500, y: p.y + 500 }));
    const far = { x: -9000, y: -9000 };
    expect(freehandPoints(trail, far, far)).toEqual(simplifyTrail(trail, FREEHAND_TOLERANCE));
  });
});

// --- 상한이 실제로 죄는 자리 (AC-76) ---------------------------------------
//
// **이 절은 몸짓 시험에서 내려온 것이다.** 같은 성질을 오버레이 위에서 재면 포인터 사건
// 600 개가 React 렌더 600 번을 부르고, 그 한 시험이 전체 스위트 부하에서 5초 천장에
// 부딪힌다(실측 7418ms). 여기서는 같은 600 점이 마이크로초다. 몸짓 층에는 **배선이
// 이어졌는가** 만 남기고(`canvas011Freehand.test.tsx`), 상한 자체의 성질은 이 자리가 든다.

describe('상한이 **실제로 죄는** 자리다 (AC-76)', () => {
  const RAW = zigzag(600);
  /** 궤적에서 아주 먼 두 끝 — 끝점 걷어냄이 개수에 끼어들지 않게 한다. */
  const FAR = { x: -90000, y: -90000 };

  it('전제: 이 궤적은 **간소화로 줄지 않는다** — 줄이는 것이 오차가 아님을 먼저 못박는다', () => {
    // 이 줄이 빨개지면 아래 두 시험은 상한을 재는 것이 아니라 오차를 재는 것이 된다.
    expect(simplifyTrail(RAW, FREEHAND_TOLERANCE)).toHaveLength(RAW.length);
  });

  it('받는 자리가 궤적을 상한에서 끊는다', () => {
    expect(collect(RAW)).toHaveLength(MAX_CONNECTOR_POINTS);
  });

  it('저장되는 점이 상한 아래이고, **상한이 줄였다**', () => {
    const points = freehandPoints(collect(RAW), FAR, FAR);
    expect(points.length).toBeLessThanOrEqual(MAX_CONNECTOR_POINTS);
    // 하한이 없으면 상한을 걷어내도 위 단언이 초록으로 남는다.
    expect(points.length).toBeGreaterThan(MAX_CONNECTOR_POINTS / 2);
    expect(RAW.length).toBeGreaterThan(MAX_CONNECTOR_POINTS);
  });
});

describe('저술의 상한과 파싱의 상한이 **같은 수**다', () => {
  it('상한을 채운 목록이 저장 왕복에서 잘리지 않는다', () => {
    // 두 상한이 갈리면 여기서 길이가 준다. 소스 텍스트가 아니라 **왕복 결과**로 잰다.
    const points = freehandPoints(collect(zigzag(600)), { x: -90000, y: -90000 }, {
      x: -90000,
      y: -90000,
    });
    expect(points.length).toBeGreaterThan(MAX_CONNECTOR_POINTS / 2);

    const { created } = appendConnector([], { el: 'a', a: 'e' }, { el: 'b', a: 'w' }, 'free', points);
    const restored = parseCanvasConfig(
      JSON.parse(JSON.stringify({ canvas: { width: 500, height: 400 }, elements: [created] })) as unknown,
    );
    expect(restored.elements.find(isConnector)?.points).toEqual(points);
  });
});
