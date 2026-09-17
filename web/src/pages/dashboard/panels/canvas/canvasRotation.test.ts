// 회전 각도의 산술과 저장 규율 (SPEC-CANVAS-014 M1 · AC-01~AC-07).
//
// **이 파일이 지키는 것 가운데 하나는 새 기능이 아니라 옛 그림이다.** 각도 0 에서 저장이
// 바이트 동일하고 AABB 가 받은 상자 그대로여야 014 이전의 모든 화면이 흔들리지 않는다.
//
// @spec SPEC-CANVAS-014 REQ-01 · REQ-08

import { describe, expect, it } from 'vitest';

import type { CanvasBox } from './canvasGeometry';
import {
  FULL_TURN_DEGREES,
  ROTATION_SNAP_DEGREES,
  boxCenter,
  isRotated,
  normalizeDegrees,
  rotatePoint,
  rotatedAabb,
  storableDegrees,
  toRadians,
} from './canvasRotation';

/** 기본값 모양이 아니다 — 원점 ≠ 0 · `x ≠ y` · 비정사각(시험 규율 D2·D3). */
const BOX: CanvasBox = { x: 31, y: 57, w: 140, h: 90 };

describe('정규화 (AC-02)', () => {
  it.each([
    [0, 0],
    [30, 30],
    [359, 359],
    [360, 0],
    [450, 90],
    [-90, 270],
    [-360, 0],
    [720 + 45, 45],
  ])('%o → %o', (input, expected) => {
    expect(normalizeDegrees(input)).toBe(expected);
  });

  it('소수는 정수로 반올림된다 — 저장에 부동소수를 들이지 않는다', () => {
    expect(normalizeDegrees(30.4)).toBe(30);
    expect(normalizeDegrees(30.6)).toBe(31);
  });

  it.each([Number.NaN, Number.POSITIVE_INFINITY, '30', null, undefined, {}])(
    '손상된 %o 는 미지정이다 (AC-03)',
    (bad) => {
      expect(normalizeDegrees(bad)).toBeUndefined();
    },
  );
});

describe('저장 규율 — 0 은 키를 만들지 않는다 (AC-04 · §결정 7)', () => {
  it.each([0, 360, -360, 720])('%o 는 저장되지 않는다', (v) => {
    expect(storableDegrees(v)).toBeUndefined();
  });

  it('0 이 아닌 각도는 저장된다', () => {
    expect(storableDegrees(30)).toBe(30);
    expect(storableDegrees(-90)).toBe(270);
  });

  it('산술용과 저장용이 **갈라져 있다**', () => {
    // 산술은 0 을 값으로 다뤄야 하고(각도를 더한 결과가 0 일 수 있다) 저장은 부재로
    // 다뤄야 한다. 한 함수가 둘을 겸하면 산술 쪽에서 매번 `?? 0` 을 적게 된다.
    expect(normalizeDegrees(0)).toBe(0);
    expect(storableDegrees(0)).toBeUndefined();
  });
});

describe('`isRotated` — 미지정과 0 은 같은 그림이다', () => {
  it.each([undefined, 0, 360, -360])('%o 는 돌지 않았다', (v) => {
    expect(isRotated(v)).toBe(false);
  });

  it.each([1, 90, 359, -1])('%o 는 돌았다', (v) => {
    expect(isRotated(v)).toBe(true);
  });
});

describe('점 회전 — 양수는 시계 방향이다', () => {
  const pivot = { x: 0, y: 0 };

  it('90° 에서 (10, 0) 이 (0, 10) 으로 간다', () => {
    // 화면 좌표는 y 가 아래다. 오른쪽에 있던 점이 **아래**로 간다 — 013 의
    // `transformLocalPoint` 가 쓴 그 약속과 같다. 부호가 뒤집히면 여기가 운다.
    const out = rotatePoint({ x: 10, y: 0 }, pivot, 90);
    expect(out.x).toBeCloseTo(0, 10);
    expect(out.y).toBeCloseTo(10, 10);
  });

  it('부호를 뒤집으면 되돌아온다 — 함수가 하나인 근거다', () => {
    const p = { x: 37, y: -13 };
    const there = rotatePoint(p, pivot, 47);
    const back = rotatePoint(there, pivot, -47);
    expect(back.x).toBeCloseTo(p.x, 9);
    expect(back.y).toBeCloseTo(p.y, 9);
  });

  it('축 위의 점은 움직이지 않는다', () => {
    const c = { x: 5, y: 7 };
    const out = rotatePoint(c, c, 123);
    expect(out.x).toBeCloseTo(5, 10);
    expect(out.y).toBeCloseTo(7, 10);
  });

  it('네 번 90° 면 제자리다', () => {
    let p = { x: 12, y: -5 };
    for (let i = 0; i < 4; i += 1) p = rotatePoint(p, pivot, 90);
    expect(p.x).toBeCloseTo(12, 9);
    expect(p.y).toBeCloseTo(-5, 9);
  });
});

describe('축-나란 상자 (AC-06 · AC-07 · §결정 2)', () => {
  it('각도 0 이면 **받은 상자 그대로**다 (K3)', () => {
    // 014 이전의 모든 화면이 "OBB 와 AABB 가 같은 수" 위에 서 있다. 여기서 부동소수
    // 왕복을 한 번이라도 태우면 그 등식이 마지막 자리에서 깨진다.
    expect(rotatedAabb(BOX, undefined)).toEqual(BOX);
    expect(rotatedAabb(BOX, 0)).toEqual(BOX);
    expect(rotatedAabb(BOX, 360)).toEqual(BOX);
  });

  it('45° 돌아간 정사각형의 AABB 가 `√2` 배다 (AC-07)', () => {
    const square: CanvasBox = { x: 0, y: 0, w: 100, h: 100 };
    const out = rotatedAabb(square, 45);
    expect(out.w).toBeCloseTo(100 * Math.SQRT2, 6);
    expect(out.h).toBeCloseTo(100 * Math.SQRT2, 6);
    // 가운데는 움직이지 않는다 — 축이 상자 가운데이기 때문이다.
    expect(out.x + out.w / 2).toBeCloseTo(50, 6);
    expect(out.y + out.h / 2).toBeCloseTo(50, 6);
  });

  it('90° 에서 가로·세로가 맞바뀐다', () => {
    const out = rotatedAabb(BOX, 90);
    expect(out.w).toBeCloseTo(BOX.h, 6);
    expect(out.h).toBeCloseTo(BOX.w, 6);
  });

  it('AABB 는 언제나 원래 상자를 **덮는다**', () => {
    for (const deg of [1, 17, 45, 90, 137, 200, 271, 359]) {
      const out = rotatedAabb(BOX, deg);
      const c = boxCenter(BOX);
      expect(out.x + out.w / 2, String(deg)).toBeCloseTo(c.x, 6);
      expect(out.y + out.h / 2, String(deg)).toBeCloseTo(c.y, 6);
      // 덮는 상자는 원래 상자의 대각선보다 작을 수 없다.
      const diag = Math.hypot(BOX.w, BOX.h);
      expect(Math.max(out.w, out.h), String(deg)).toBeLessThanOrEqual(diag + 1e-6);
    }
  });
});

describe('상수', () => {
  it('한 바퀴가 360 이다 — 013 의 `(θ+90)×4 ≡ θ` 가 서는 근거다', () => {
    expect(FULL_TURN_DEGREES).toBe(360);
    expect(normalizeDegrees(30 + 90 * 4)).toBe(30);
  });

  it('Shift 눈금은 격자 간격과 **다른 축**이다', () => {
    expect(ROTATION_SNAP_DEGREES).toBe(15);
    expect(FULL_TURN_DEGREES % ROTATION_SNAP_DEGREES).toBe(0);
  });

  it('라디안 환산이 맞다', () => {
    expect(toRadians(180)).toBeCloseTo(Math.PI, 12);
  });
});
