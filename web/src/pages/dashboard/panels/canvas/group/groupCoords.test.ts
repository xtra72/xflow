// 그룹 로컬 좌표 변환 (SPEC-CANVAS-004 M2).
//
// 재는 것은 넷이다:
//   ① **왕복의 양쪽** — 상한 안에서 정수까지 정확하고, 상한 밖에서 오차가 상한 이하다
//      (시험 규율 E-G: 한쪽만 재면 상한을 넘긴 구현도 통과한다).
//   ② **원점과 길이의 차이** — 자리에서는 원점을 빼고 길이에서는 빼지 않는다(E-D).
//   ③ **퇴화 상자 넓히기가 `toLocal` 앞에서** 일어난다 — 부품이 모서리가 아니라
//      **가운데**로 간다(E-H · AC-E10).
//   ④ 새 나눗셈이 없다 — 격자 상수가 이 모듈에 글자로 나타나지 않는다(G2).
//
// 고정 입력은 그룹 상자 `317 × 181` 에 원점 `(73, 41)`(E-B · E-C · E-D)이고, 음수 원점
// 상자를 따로 둔다.
//
// @spec SPEC-CANVAS-004 REQ-07 · AC-14 · AC-E10

import { describe, expect, it } from 'vitest';

import { MIN_ELEMENT_EXTENT, type BoxGeometry, type LineGeometry, type PointGeometry } from '../canvasConfig';
import type { CanvasBox } from '../canvasGeometry';
import {
  toAbsoluteBox,
  toAbsoluteGeometry,
  toAbsoluteLine,
  toAbsolutePointRounded,
  toLocalBox,
  toLocalGeometry,
  toLocalLine,
  toLocalPoint,
  widenDegenerateBox,
} from './groupCoords';
import { GROUP_LOCAL_EXTENT } from './groupTypes';

const BOX: CanvasBox = { x: 73, y: 41, w: 317, h: 181 };
const NEG_BOX: CanvasBox = { x: -120, y: -37, w: 317, h: 181 };

/** 왕복 오차 상한 — `상자변 ÷ (2 × EXTENT)` 캔버스 단위(가정 A15). */
function roundTripBound(span: number): number {
  return span / (2 * GROUP_LOCAL_EXTENT);
}

// --- ① 왕복 — 양쪽을 다 단언한다 (E-G · AC-14) -----------------------------

describe('묶기 → 풀기 왕복 (AC-14 · 가정 A15)', () => {
  it('상자변이 상한(10 000) 이하이면 **정수까지 정확**하다', () => {
    // 기본 캔버스 500×400 을 덮는 자리들 + 상자 밖(캔버스 밖 저술은 합법이다).
    const geos: BoxGeometry[] = [];
    for (let x = 60; x <= 400; x += 7) {
      for (let y = 30; y <= 230; y += 11) {
        geos.push({ x, y, w: 13, h: 29 });
      }
    }
    geos.push({ x: -50, y: -20, w: 5, h: 5 }, { x: 900, y: 700, w: 3, h: 3 });
    for (const geo of geos) {
      expect(toAbsoluteBox(toLocalBox(geo, BOX), BOX), JSON.stringify(geo)).toEqual(geo);
    }
    // 상한 경계(변이 정확히 10 000)에서도 정확하다.
    const edge: CanvasBox = { x: 0, y: 0, w: GROUP_LOCAL_EXTENT, h: GROUP_LOCAL_EXTENT };
    for (let x = 0; x <= GROUP_LOCAL_EXTENT; x += 137) {
      const geo: BoxGeometry = { x, y: GROUP_LOCAL_EXTENT - x, w: 11, h: 13 };
      expect(toAbsoluteBox(toLocalBox(geo, edge), edge), `edge ${x}`).toEqual(geo);
    }
  });

  it('상자변이 `MAX_CANVAS_DIMENSION`(100 000)이면 **정확하지 않고**, 오차가 상한 이하다', () => {
    const huge: CanvasBox = { x: 0, y: 0, w: 100000, h: 100000 };
    const bound = roundTripBound(100000);
    expect(bound).toBe(5);
    let worst = 0;
    let inexact = 0;
    for (let x = 0; x <= 100000; x += 137) {
      const geo: BoxGeometry = { x, y: x, w: 11, h: 13 };
      const back = toAbsoluteBox(toLocalBox(geo, huge), huge);
      const dx = Math.abs(back.x - geo.x);
      worst = Math.max(worst, dx);
      if (dx !== 0) inexact += 1;
    }
    // 상한을 **넘는** 입력이 실제로 있다 — 없으면 이 시험은 아무것도 재지 않는다.
    expect(inexact).toBeGreaterThan(0);
    expect(worst).toBeGreaterThan(0);
    expect(worst).toBeLessThanOrEqual(bound);
  });

  it('음수 원점에서도 왕복이 정확하다 (E-D)', () => {
    for (let x = -200; x <= 150; x += 9) {
      const geo: BoxGeometry = { x, y: -30 + x, w: 7, h: 9 };
      expect(toAbsoluteBox(toLocalBox(geo, NEG_BOX), NEG_BOX), `x=${x}`).toEqual(geo);
    }
  });

  it('선·점도 왕복한다', () => {
    const line: LineGeometry = { x1: 90, y1: 60, x2: 300, y2: 180 };
    expect(toAbsoluteLine(toLocalLine(line, BOX), BOX)).toEqual(line);
    const point: PointGeometry = { x: 231, y: 131 };
    expect(toAbsolutePointRounded(toLocalPoint(point, BOX), BOX)).toEqual(point);
  });
});

// --- ② 원점과 길이 (E-D) ---------------------------------------------------

describe('자리에서는 원점을 빼고 길이에서는 빼지 않는다 (E-D)', () => {
  it('상자를 가득 덮는 부품은 로컬 `0..EXTENT` 가 된다', () => {
    expect(toLocalBox({ x: 73, y: 41, w: 317, h: 181 }, BOX)).toEqual({
      x: 0,
      y: 0,
      w: GROUP_LOCAL_EXTENT,
      h: GROUP_LOCAL_EXTENT,
    });
  });

  it('원점에 놓인 부품의 로컬 자리는 0 이고 **길이는 0 이 아니다**', () => {
    const local = toLocalBox({ x: 73, y: 41, w: 158.5, h: 90.5 }, BOX);
    expect(local.x).toBe(0);
    expect(local.y).toBe(0);
    // 길이에서 원점을 빼는 구현이라면 여기서 음수가 나온다.
    expect(local.w).toBe(5000);
    expect(local.h).toBe(5000);
  });

  it('안쪽에 떠 있는 부품은 두 축에서 서로 다른 로컬 값을 얻는다 (E-A · E-B)', () => {
    // 73 + 0.41×317 = 202.97 → 203 / 41 + 0.33×181 = 100.73 → 101
    const local = toLocalBox({ x: 203, y: 101, w: 54, h: 16 }, BOX);
    expect(local.x).toBe(Math.round(((203 - 73) / 317) * GROUP_LOCAL_EXTENT));
    expect(local.y).toBe(Math.round(((101 - 41) / 181) * GROUP_LOCAL_EXTENT));
    // 두 축의 값이 다르다 — 정사각 고정 입력이 감추던 자리다.
    expect(local.x).not.toBe(local.y);
    expect(local.w).not.toBe(local.h);
  });

  it('선의 **두 끝점 모두** 원점을 뺀다', () => {
    const local = toLocalLine({ x1: 73, y1: 41, x2: 390, y2: 222 }, BOX);
    expect(local).toEqual({ x1: 0, y1: 0, x2: GROUP_LOCAL_EXTENT, y2: GROUP_LOCAL_EXTENT });
  });

  it('형상별 갈래가 종류를 제대로 고른다', () => {
    expect(toLocalGeometry({ x: 73, y: 41, w: 317, h: 181 } as BoxGeometry, BOX)).toEqual(
      toLocalBox({ x: 73, y: 41, w: 317, h: 181 }, BOX),
    );
    expect(toLocalGeometry({ x1: 73, y1: 41, x2: 390, y2: 222 } as LineGeometry, BOX)).toEqual(
      toLocalLine({ x1: 73, y1: 41, x2: 390, y2: 222 }, BOX),
    );
    expect(toLocalGeometry({ x: 231, y: 131 } as PointGeometry, BOX)).toEqual(
      toLocalPoint({ x: 231, y: 131 }, BOX),
    );
    expect(toAbsoluteGeometry({ x: 5000, y: 5000 } as PointGeometry, BOX)).toEqual(
      toAbsolutePointRounded({ x: 5000, y: 5000 }, BOX),
    );
  });
});

// --- ③ 퇴화 상자 (E-H · AC-E10) -------------------------------------------

describe('퇴화 상자 넓히기는 `toLocal` **앞에서** 일어난다 (E-H · AC-E10)', () => {
  it('높이 0 인 상자를 넓히면 두 변이 최소 크기 이상이고 가운데가 보존된다', () => {
    const flat: CanvasBox = { x: 40, y: 200, w: 260, h: 0 };
    const wide = widenDegenerateBox(flat);
    expect(wide.w).toBe(260);
    expect(wide.h).toBeGreaterThanOrEqual(MIN_ELEMENT_EXTENT);
    // 가운데가 그대로다 — 한쪽만 넓히면 여기서 반 칸 어긋난다.
    expect(wide.y + wide.h / 2).toBe(flat.y + flat.h / 2);
    // 넓힌 변이 정수이고 짝수라 가운데도 정수다.
    expect(Number.isInteger(wide.y)).toBe(true);
    expect(wide.h % 2).toBe(0);
  });

  it('넓힌 상자 위에서 부품이 **모서리가 아니라 가운데**로 간다', () => {
    const flat: CanvasBox = { x: 40, y: 200, w: 260, h: 0 };
    const wide = widenDegenerateBox(flat);
    // 가로선 둘: 둘 다 y = 200 에 있다.
    const local = toLocalLine({ x1: 40, y1: 200, x2: 300, y2: 200 }, wide);
    expect(local.x1).toBe(0);
    expect(local.x2).toBe(GROUP_LOCAL_EXTENT);
    // **이 두 줄이 이 시험의 전부다.** 넓히기를 `toLocal` 뒤로 옮기면 여기가 0 이 된다.
    expect(local.y1).toBe(GROUP_LOCAL_EXTENT / 2);
    expect(local.y2).toBe(GROUP_LOCAL_EXTENT / 2);
  });

  it('넓힌 상자에서 왕복도 성립한다 — 되돌리면 원래 y 다', () => {
    const flat: CanvasBox = { x: 40, y: 200, w: 260, h: 0 };
    const wide = widenDegenerateBox(flat);
    const line: LineGeometry = { x1: 40, y1: 200, x2: 300, y2: 200 };
    expect(toAbsoluteLine(toLocalLine(line, wide), wide)).toEqual(line);
  });

  it('두 변 모두 0 이어도 가운데가 보존된다', () => {
    const dot: CanvasBox = { x: 17, y: 29, w: 0, h: 0 };
    const wide = widenDegenerateBox(dot);
    expect(wide).toEqual({ x: 16, y: 28, w: 2, h: 2 });
    expect(toLocalPoint({ x: 17, y: 29 }, wide)).toEqual({
      x: GROUP_LOCAL_EXTENT / 2,
      y: GROUP_LOCAL_EXTENT / 2,
    });
  });

  it('멀쩡한 상자는 건드리지 않는다', () => {
    expect(widenDegenerateBox(BOX)).toEqual(BOX);
    expect(widenDegenerateBox({ x: 0, y: 0, w: MIN_ELEMENT_EXTENT, h: MIN_ELEMENT_EXTENT })).toEqual({
      x: 0,
      y: 0,
      w: MIN_ELEMENT_EXTENT,
      h: MIN_ELEMENT_EXTENT,
    });
  });

  it('음수 크기 상자도 최소 크기 이상으로 넓힌다', () => {
    const wide = widenDegenerateBox({ x: 10, y: 10, w: -5, h: -5 });
    expect(wide.w).toBeGreaterThanOrEqual(MIN_ELEMENT_EXTENT);
    expect(wide.h).toBeGreaterThanOrEqual(MIN_ELEMENT_EXTENT);
  });
});

// --- ④ 방어 ---------------------------------------------------------------

describe('손상 입력이 NaN 을 흘리지 않는다', () => {
  it('비유한 기하·상자에서도 유한 정수가 나온다', () => {
    const local = toLocalBox(
      { x: Number.NaN, y: 41, w: Number.POSITIVE_INFINITY, h: 181 },
      { x: 73, y: Number.NaN, w: 317, h: 0 },
    );
    expect(Object.values(local).every(Number.isFinite)).toBe(true);
    const abs = toAbsoluteBox(local, { x: Number.NaN, y: 0, w: -1, h: 181 });
    expect(Object.values(abs).every(Number.isFinite)).toBe(true);
  });
});
