// 곡선 조각 산술 (SPEC-CANVAS-011 M6).
//
// 순수 모듈이라 DOM 도 투영도 없이 값만 잰다. 그리기 쪽의 시험
// (`drawElement.connector.test.ts`)은 **호출 기록**을 재고, 이 파일은 그 기록에 들어가는
// **수**를 잰다 — 둘이 같은 사실을 다른 자로 재는 것이 층을 건너는 시험의 값이다.
//
// @spec SPEC-CANVAS-011 REQ-04 · AC-48 · AC-50

import { describe, expect, it } from 'vitest';

import { curveSegments, type CurvePoint } from './connectorCurve';

const A: CurvePoint = { x: 0, y: 0 };
const B: CurvePoint = { x: 90, y: 30 };

/** 2차 제어점 `q` 에 대한 3차 제어점 — 시험이 제 식을 따로 적는다(구현을 베끼지 않는다). */
function cubicControl(end: CurvePoint, q: CurvePoint): CurvePoint {
  return { x: (end.x + 2 * q.x) / 3, y: (end.y + 2 * q.y) / 3 };
}

describe('중간점이 없으면 **조각도 없다** (AC-50)', () => {
  it('두 점이면 빈 목록이다 — 호출자가 꺾은 선으로 떨어진다', () => {
    expect(curveSegments([A, B])).toEqual([]);
  });

  it('점이 하나거나 아예 없어도 빈 목록이다', () => {
    expect(curveSegments([A])).toEqual([]);
    expect(curveSegments([])).toEqual([]);
  });

  it('빈 목록과 "직선 한 조각" 은 다르다 — 후자면 `bezierCurveTo` 가 난다', () => {
    // 중간점 없는 `curve` 가 `straight` 와 호출 기록이 갈리지 않는 근거가 이 한 줄이다.
    expect(curveSegments([A, B])).toHaveLength(0);
  });
});

describe('중간점 하나는 **2차 하나**로 줄어든다 (AC-48)', () => {
  const P: CurvePoint = { x: 15, y: 120 };

  it('제어점이 `A + ⅔(P−A)` 와 `B + ⅔(P−B)` 다', () => {
    const [seg, ...rest] = curveSegments([A, P, B]);
    expect(rest).toEqual([]);
    expect(seg?.c1).toEqual(cubicControl(A, P));
    expect(seg?.c2).toEqual(cubicControl(B, P));
    expect(seg?.to).toEqual(B);
  });

  it('두 제어점이 **서로 다르다** — 식이 뒤바뀌면 드러난다', () => {
    const [seg] = curveSegments([A, P, B]);
    expect(seg?.c1).not.toEqual(seg?.c2);
  });

  it('제어점이 A·P 를 잇는 선분 위에 있다 — ⅔ 지점이다', () => {
    const [seg] = curveSegments([A, P, B]);
    // (c1 − A) 가 (P − A) 의 정확히 2/3 배다.
    expect(seg?.c1.x).toBeCloseTo(A.x + (2 / 3) * (P.x - A.x), 12);
    expect(seg?.c1.y).toBeCloseTo(A.y + (2 / 3) * (P.y - A.y), 12);
  });
});

describe('중간점이 둘 이상이면 **중점에서 이어진다**', () => {
  const m1: CurvePoint = { x: 30, y: 90 };
  const m2: CurvePoint = { x: 60, y: -30 };
  const m3: CurvePoint = { x: 75, y: 45 };

  it('조각 수가 중간점 수와 같다', () => {
    expect(curveSegments([A, m1, B])).toHaveLength(1);
    expect(curveSegments([A, m1, m2, B])).toHaveLength(2);
    expect(curveSegments([A, m1, m2, m3, B])).toHaveLength(3);
  });

  it('앞 조각은 이웃한 두 제어점의 중점에서 끝나고 마지막만 끝점에서 끝난다', () => {
    const segs = curveSegments([A, m1, m2, m3, B]);
    expect(segs[0]?.to).toEqual({ x: (m1.x + m2.x) / 2, y: (m1.y + m2.y) / 2 });
    expect(segs[1]?.to).toEqual({ x: (m2.x + m3.x) / 2, y: (m2.y + m3.y) / 2 });
    expect(segs[2]?.to).toEqual(B);
  });

  it('조각들이 **끊기지 않는다** — 앞 조각의 끝이 뒤 조각의 시작이다', () => {
    // 시작점은 인자로 넘어가지 않으므로(직전 조각의 끝이다) 제어점으로 잰다: `c1` 은
    // 시작점과 제어점의 ⅔ 지점이므로, 시작점을 거꾸로 구해 앞 조각의 끝과 맞춘다.
    const segs = curveSegments([A, m1, m2, m3, B]);
    const controls = [m1, m2, m3];
    let from: CurvePoint = A;
    segs.forEach((seg, i) => {
      const q = controls[i]!;
      // c1 = from + ⅔(q − from)  ⇒  from = (3·c1 − 2·q)
      expect(3 * seg.c1.x - 2 * q.x).toBeCloseTo(from.x, 9);
      expect(3 * seg.c1.y - 2 * q.y).toBeCloseTo(from.y, 9);
      from = seg.to;
    });
    expect(from).toEqual(B);
  });

  it('어느 구간도 **두 번 그려지지 않는다** — 글자대로의 읽기와 갈리는 자리다', () => {
    // 글자대로 읽으면 m₁ 의 2차가 `A→m₂` 를, m₂ 의 2차가 `m₁→B` 를 덮어 `m₁..m₂` 가 두 번
    // 그려지고 선이 되돌아간다. 011 의 일반화에서는 조각의 끝이 **단조롭게 전진**한다.
    const segs = curveSegments([A, m1, m2, m3, B]);
    const ends = segs.map((s) => s.to);
    expect(new Set(ends.map((p) => `${p.x},${p.y}`)).size).toBe(ends.length);
  });
});

describe('단위를 모른다', () => {
  it('좌표를 축마다 상수배 해도 같은 배율의 조각이 난다 — 아핀 보존', () => {
    // 그리는 쪽은 **투영한 뒤에** 부른다. 그래도 같은 곡선이라는 사실이 여기 값으로 있다.
    const P: CurvePoint = { x: 15, y: 120 };
    const scale = (p: CurvePoint): CurvePoint => ({ x: p.x * 0.8, y: p.y * 0.5 });
    const [plain] = curveSegments([A, P, B]);
    const [scaled] = curveSegments([A, P, B].map(scale));
    expect(scaled?.c1.x).toBeCloseTo((plain?.c1.x ?? 0) * 0.8, 9);
    expect(scaled?.c1.y).toBeCloseTo((plain?.c1.y ?? 0) * 0.5, 9);
    expect(scaled?.to).toEqual(scale(B));
  });
});
