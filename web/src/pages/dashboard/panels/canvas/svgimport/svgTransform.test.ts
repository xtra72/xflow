// 변환 시험 (SPEC-CANVAS-007 M3 · AC-03 · AC-04).
//
// **E1 이 이 파일의 고정 입력을 정한다.** 항등 변환(속성이 아예 없거나 `translate(0,0)`)은
// **변환 층 전부**를 감춘다 — 합성 순서도, 조상 누적도, `√|det|` 도, 호와의 순서(K2)도
// 어느 것도 관측되지 않는다. 그리고 **`translate` 만 쓴 고정 입력도 마찬가지다**:
// 평행이동끼리는 교환법칙이 성립하므로 순서를 뒤집어도 같은 답이 나온다.
//
// 그래서 고정 입력은 **깊이 3 중첩에 세 종류의 변환**을 쓴다:
//   바깥 `translate(7,11)` · 중간 `rotate(30, 4, 9)` · 안쪽 `scale(2, 0.5)`.
// 셋이 다 있어야 순서가 보이고, 안쪽이 비균등이라 `√|det|` 근사도 켜진다.
//
// **확인한 뮤테이션(E12)** — 각각 어느 단언이 빨개지는지 시험 이름에 적었다.
//   1. `multiplyMatrix` 의 안팎을 뒤집으면 → "왼쪽이 바깥이다" 와 "깊이 3 누적" 이
//      빨개진다. **`translate` 만 있는 고정 입력에서는 빨개지지 않는다.**
//   2. 각도를 라디안으로 읽으면(`DEG_TO_RAD` 제거) → `rotate` 단언이 빨개진다.
//   3. `rotate(a, cx, cy)` 의 중심 되돌림을 지우면 → 중심 있는 회전 단언이 빨개진다.
//   4. `isSimilarity` 를 언제나 `true` 로 두면 → "비균등이 근사로 보고된다" 가 빨개진다.
//   5. `strokeScaleOf` 를 `|det|` 로(제곱근 제거) 두면 → 두께 배율 단언이 빨개진다.
//   6. `A` 축약을 변환 뒤로 옮기면 → AC-04 의 "두 순서가 다르다" 가 빨개진다.

import fs from 'node:fs';
import path from 'node:path';

import { describe, expect, it } from 'vitest';

import type { PathCommand } from '../shapes/pathTypes';

import { arcToCubics } from './svgArc';
import {
  applyMatrix,
  determinant,
  IDENTITY_MATRIX,
  isSimilarity,
  multiplyMatrix,
  parseTransformList,
  strokeScaleOf,
  transformCommands,
  type Matrix2x3,
} from './svgTransform';

// --- E1 고정 입력 -------------------------------------------------------

const OUTER = 'translate(7,11)';
const MIDDLE = 'rotate(30, 4, 9)';
const INNER = 'scale(2, 0.5)';

/** 조상 누적 — 조상이 바깥이다. */
function nested(): Matrix2x3 {
  return multiplyMatrix(
    multiplyMatrix(parseTransformList(OUTER), parseTransformList(MIDDLE)),
    parseTransformList(INNER),
  );
}

describe('행렬 기본', () => {
  it('항등은 점을 움직이지 않는다', () => {
    expect(applyMatrix(IDENTITY_MATRIX, 3, 5)).toEqual({ x: 3, y: 5 });
  });

  it('translate · scale · rotate 가 각각 제 뜻대로 점을 옮긴다', () => {
    expect(applyMatrix(parseTransformList('translate(7,11)'), 3, 5)).toEqual({ x: 10, y: 16 });
    expect(applyMatrix(parseTransformList('scale(2,0.5)'), 3, 5)).toEqual({ x: 6, y: 2.5 });
    const rotated = applyMatrix(parseTransformList('rotate(90)'), 1, 0);
    expect(rotated.x).toBeCloseTo(0, 12);
    expect(rotated.y).toBeCloseTo(1, 12);
  });

  it('각도는 도(degree)다 — 라디안으로 읽으면 30° 가 1719° 가 된다 (뮤테이션 2)', () => {
    const m = parseTransformList('rotate(30)');
    expect(m.a).toBeCloseTo(Math.cos(Math.PI / 6), 12);
    expect(m.b).toBeCloseTo(Math.sin(Math.PI / 6), 12);
  });

  it('rotate(a, cx, cy) 는 그 점을 고정점으로 둔다 (뮤테이션 3)', () => {
    const m = parseTransformList('rotate(30, 4, 9)');
    const fixed = applyMatrix(m, 4, 9);
    expect(fixed.x).toBeCloseTo(4, 12);
    expect(fixed.y).toBeCloseTo(9, 12);
  });

  it('scale 인자가 하나면 두 축에 같이 걸린다', () => {
    expect(parseTransformList('scale(3)')).toEqual(parseTransformList('scale(3,3)'));
  });

  it('translate 인자가 하나면 ty = 0 이다', () => {
    expect(parseTransformList('translate(5)')).toEqual(parseTransformList('translate(5,0)'));
  });

  it('skewX 와 skewY 가 서로 다른 축을 기울인다', () => {
    const sx = parseTransformList('skewX(45)');
    const sy = parseTransformList('skewY(45)');
    expect(applyMatrix(sx, 0, 1).x).toBeCloseTo(1, 12);
    expect(applyMatrix(sx, 0, 1).y).toBeCloseTo(1, 12);
    expect(applyMatrix(sy, 1, 0).x).toBeCloseTo(1, 12);
    expect(applyMatrix(sy, 1, 0).y).toBeCloseTo(1, 12);
  });

  it('matrix(a b c d e f) 는 그대로 실린다', () => {
    expect(parseTransformList('matrix(1 2 3 4 5 6)')).toEqual({ a: 1, b: 2, c: 3, d: 4, e: 5, f: 6 });
  });
});

describe('합성 순서 — 왼쪽이 바깥이다 [E1] (뮤테이션 1)', () => {
  it('한 속성 안의 두 함수에서 오른쪽이 점에 먼저 적용된다', () => {
    // `translate(10,0) scale(2)` 에 (1,0) → 배율 먼저 (2,0) → 옮김 (12,0).
    // 순서를 뒤집으면 (1+10)*2 = (22,0) 이다 — 두 답이 다른 고정 입력이라야 순서가 보인다.
    const m = parseTransformList('translate(10,0) scale(2)');
    expect(applyMatrix(m, 1, 0)).toEqual({ x: 12, y: 0 });
  });

  it('translate 만 있는 고정 입력에서는 순서가 관측되지 않는다 — 그 사실을 시험이 적는다', () => {
    const forward = parseTransformList('translate(10,0) translate(0,5)');
    const backward = parseTransformList('translate(0,5) translate(10,0)');
    expect(applyMatrix(forward, 1, 1)).toEqual(applyMatrix(backward, 1, 1));
  });

  it('깊이 3 중첩이 M_바깥 · M_중간 · M_안쪽 를 적용한다 [E1]', () => {
    // 독립으로 손으로 푼 값이다(코드로 다시 합성하지 않는다):
    //   scale(2,0.5) : (3,5) → (6, 2.5)
    //   rotate(30° about (4,9)) : (6,2.5) → (4 + 2cos30 + 6.5sin30, 9 + 2sin30 − 6.5cos30)
    //                                     = (8.982050807568877, 4.370834875401148)
    //   translate(7,11) : → (15.982050807568877, 15.370834875401148)
    // 리터럴을 15 유효숫자로 줄인 것은 `no-loss-of-precision` 때문이다 — 17 자리는
    // 런타임에 정밀도를 잃고, 그 사실을 lint 가 오류로 잡는다.
    const p = applyMatrix(nested(), 3, 5);
    expect(p.x).toBeCloseTo(15.9820508075689, 9);
    expect(p.y).toBeCloseTo(15.3708348754011, 9);
  });
});

describe('선 두께 배율 (AC-03)', () => {
  it('비균등 배율의 두께 배율은 √|det| 다 (뮤테이션 5)', () => {
    const m = parseTransformList(INNER); // scale(2, 0.5) → det = 1
    expect(determinant(m)).toBeCloseTo(1, 12);
    expect(strokeScaleOf(m)).toBeCloseTo(1, 12);
    const bigger = parseTransformList('scale(4, 1)');
    expect(strokeScaleOf(bigger)).toBeCloseTo(2, 12);
  });

  it('닮음 변환은 정확한 옮김이라 보고할 것이 없다 (뮤테이션 4)', () => {
    expect(isSimilarity(parseTransformList('rotate(30) scale(3)'))).toBe(true);
    expect(isSimilarity(parseTransformList('translate(7,11)'))).toBe(true);
    expect(isSimilarity(parseTransformList('scale(-2)'))).toBe(true); // 반사도 닮음이다
  });

  it('비균등 배율과 기울임은 닮음이 아니다 — 근사로 보고할 자리다 (뮤테이션 4)', () => {
    expect(isSimilarity(parseTransformList(INNER))).toBe(false);
    expect(isSimilarity(parseTransformList('skewX(20)'))).toBe(false);
    expect(isSimilarity(nested())).toBe(false);
  });

  it('퇴화 행렬은 |det| = 0 이다 — 그릴 것이 없다', () => {
    expect(determinant(parseTransformList('scale(0, 3)'))).toBe(0);
    expect(strokeScaleOf(parseTransformList('matrix(1 2 2 4 0 0)'))).toBe(0);
  });
});

describe('좌표에 녹인다 (REQ-02)', () => {
  it('M · L · C 의 모든 좌표가 옮겨지고 Z 는 그대로다', () => {
    const commands: PathCommand[] = [
      { c: 'M', x: 1, y: 0 },
      { c: 'L', x: 3, y: 0 },
      { c: 'C', x1: 4, y1: 0, x2: 5, y2: 2, x: 6, y: 4 },
      { c: 'Z' },
    ];
    const moved = transformCommands(commands, parseTransformList('translate(10,0) scale(2)'));
    expect(moved).toEqual([
      { c: 'M', x: 12, y: 0 },
      { c: 'L', x: 16, y: 0 },
      { c: 'C', x1: 18, y1: 0, x2: 20, y2: 4, x: 22, y: 8 },
      { c: 'Z' },
    ]);
  });

  it('아핀은 3차 베지어를 3차 베지어로 정확히 옮긴다 — 곡선 위 점이 옮겨진 곡선 위에 있다', () => {
    const m = nested();
    const curve: PathCommand[] = [{ c: 'C', x1: 10, y1: 0, x2: 20, y2: 30, x: 30, y: 10 }];
    const movedCurve = transformCommands(curve, m)[0];
    if (movedCurve?.c !== 'C') throw new Error('expected cubic');
    const t = 0.37;
    const bezier = (p0: number, p1: number, p2: number, p3: number): number => {
      const u = 1 - t;
      return u * u * u * p0 + 3 * u * u * t * p1 + 3 * u * t * t * p2 + t * t * t * p3;
    };
    const before = { x: bezier(0, 10, 20, 30), y: bezier(0, 0, 30, 10) };
    const expected = applyMatrix(m, before.x, before.y);
    const origin = applyMatrix(m, 0, 0);
    const actual = {
      x: bezier(origin.x, movedCurve.x1, movedCurve.x2, movedCurve.x),
      y: bezier(origin.y, movedCurve.y1, movedCurve.y2, movedCurve.y),
    };
    expect(actual.x).toBeCloseTo(expected.x, 9);
    expect(actual.y).toBeCloseTo(expected.y, 9);
  });
});

describe('AC-04 — A 축약이 transform 보다 먼저다 (불변식 K2 · 뮤테이션 6)', () => {
  /** 틀린 순서: 끝점을 먼저 옮기고, **같은** `rx`/`ry`/`φ` 로 호를 만든다. */
  function arcAfterTransform(m: Matrix2x3): PathCommand[] {
    const from = applyMatrix(m, 0, 0);
    const to = applyMatrix(m, 60, 20);
    return arcToCubics(from.x, from.y, 50, 30, 0, 0, 1, to.x, to.y);
  }
  /** 옳은 순서: 호를 3차로 바꾼 뒤 아핀을 적용한다. */
  function arcBeforeTransform(m: Matrix2x3): PathCommand[] {
    return transformCommands(arcToCubics(0, 0, 50, 30, 0, 0, 1, 60, 20), m);
  }

  it('비균등 scale 아래에서 두 순서가 서로 다른 곡선을 낸다 — 시험이 둘 다 계산한다', () => {
    const m = parseTransformList(INNER);
    expect(arcBeforeTransform(m)).not.toEqual(arcAfterTransform(m));
  });

  it('기울어진 g 안에서도 두 순서가 다르다 — 화면에서만 드러나는 부류다', () => {
    const m = parseTransformList('skewX(25)');
    expect(arcBeforeTransform(m)).not.toEqual(arcAfterTransform(m));
  });

  it('평행이동만 있으면 두 순서가 같다 — 이 고정 입력에서는 K2 가 관측되지 않는다', () => {
    // **이 사실이 K2 가 조용한 이유다.** `translate` 만 쓰는 문서로는 순서 결함이 절대
    // 드러나지 않는다 — E1 이 지목한 그 기본값이 여기서도 층 전체를 감춘다.
    //
    // **닮음도 강체도 여기 들지 못한다.** `scale(3)` 은 끝점 사이 거리를 3배로 벌리면서
    // `rx`/`ry` 는 그대로 두므로 틀린 순서 쪽이 반지름 보정(`√Λ`)에 걸리고, `rotate(30)`
    // 은 타원의 축을 함께 돌리지 않으므로 `φ` 가 어긋난다. 두 순서가 정말로 같아지는
    // 것은 **평행이동뿐**이다.
    const m = parseTransformList('translate(7,11)');
    const before = arcBeforeTransform(m);
    const after = arcAfterTransform(m);
    expect(before).toHaveLength(after.length);
    before.forEach((cmd, i) => {
      const other = after[i];
      if (cmd.c !== 'C' || other?.c !== 'C') throw new Error('expected cubics');
      expect(cmd.x).toBeCloseTo(other.x, 6);
      expect(cmd.y).toBeCloseTo(other.y, 6);
    });
  });

  it('형상 판정 — 호 모듈이 변환 행렬을 참조하지 않는다', () => {
    const source = fs.readFileSync(path.join(__dirname, 'svgArc.ts'), 'utf8');
    expect(source).not.toContain('Matrix2x3');
    expect(source).not.toContain('svgTransform');
  });
});

describe('견고성 — 손상된 transform (REQ-07 · AC-E4)', () => {
  const corrupt = [
    'rotate(',
    'matrix(1,2,3)',
    'translate(1,2,3)',
    'banana(1)',
    'scale()',
    'rotate(30, 4)',
    '((((',
    'translate(NaN, 2)',
  ];

  for (const raw of corrupt) {
    it(`${JSON.stringify(raw)} 이 예외 없이 값으로 돌아온다`, () => {
      let m: Matrix2x3 | undefined;
      expect(() => {
        m = parseTransformList(raw);
      }).not.toThrow();
      expect(Number.isFinite(m?.a)).toBe(true);
    });
  }

  it('속성이 없으면 항등이다', () => {
    expect(parseTransformList(undefined)).toEqual(IDENTITY_MATRIX);
    expect(parseTransformList('')).toEqual(IDENTITY_MATRIX);
  });

  it('앞이 성하고 뒤가 손상되면 앞까지를 살린다', () => {
    // 오타 하나로 도형을 통째로 잃는 쪽을 기각한 자리(머리말).
    expect(parseTransformList('translate(7,11) rotate(')).toEqual(parseTransformList('translate(7,11)'));
  });
});
