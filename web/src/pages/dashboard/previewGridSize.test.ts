// 미리보기 리사이즈의 그리드 단위 변환 검증.
//
// 화면 px 를 그대로 그리드 단위로 쓰면 축소된 미리보기에서 한 칸을 끌었는데 여러 칸이
// 움직인다. 배율 환산이 이 테스트가 잠그는 핵심이다.

import { describe, expect, it } from 'vitest';

import { GRID_MARGIN_PX, unitsToPx } from './gridGeometry';
import {
  DEFAULT_MAX_H,
  clampGridSize,
  resizeGridSize,
  sameGridSize,
  unitsFromPx,
} from './previewGridSize';

const CELL = 100;

describe('unitsToPx / unitsFromPx', () => {
  it('단위와 픽셀을 서로 되돌린다', () => {
    for (const u of [1, 2, 5, 12]) {
      expect(unitsFromPx(unitsToPx(u, CELL), CELL)).toBeCloseTo(u, 10);
    }
  });

  it('마진이 단위 수에 비례해 끼어든다', () => {
    expect(unitsToPx(1, CELL)).toBe(CELL);
    expect(unitsToPx(3, CELL)).toBe(3 * CELL + GRID_MARGIN_PX * 2);
  });

  it('셀 크기를 모르면 되돌릴 수 없다', () => {
    expect(unitsFromPx(100, 0)).toBeUndefined();
  });
});

describe('clampGridSize', () => {
  it('정수로 반올림한다', () => {
    expect(clampGridSize({ w: 2.4, h: 3.6 }, { cols: 12 })).toEqual({ w: 2, h: 4 });
  });

  it('칼럼 수를 넘는 폭을 막는다', () => {
    expect(clampGridSize({ w: 99, h: 2 }, { cols: 12 })).toEqual({ w: 12, h: 2 });
  });

  it('최소 크기를 지킨다', () => {
    expect(clampGridSize({ w: 1, h: 1 }, { cols: 12, minW: 3, minH: 2 })).toEqual({ w: 3, h: 2 });
  });

  it('높이 상한으로 무한히 긴 패널을 막는다', () => {
    expect(clampGridSize({ w: 2, h: 9999 }, { cols: 12 }).h).toBe(DEFAULT_MAX_H);
  });

  it('minW 가 cols 보다 커도 모순된 결과를 내지 않는다', () => {
    expect(clampGridSize({ w: 5, h: 1 }, { cols: 2, minW: 4 })).toEqual({ w: 4, h: 1 });
  });

  it('최소 크기를 0 으로 줘도 한 칸 아래로는 내려가지 않는다', () => {
    // 0 칸짜리 패널은 화면에서 사라지고, 사라지면 다시 잡아 키울 손잡이도 없다.
    expect(clampGridSize({ w: 1, h: 1 }, { cols: 12, minW: 0, minH: 0 })).toEqual({ w: 1, h: 1 });
  });

  it('칼럼 수를 아직 모르면(0) 최소 폭으로 떨어진다 — 폭 0 인 패널을 만들지 않는다', () => {
    // 대시보드를 한 번도 열지 않아 칼럼 수가 0 인 상태. 상한을 0 으로 읽으면 폭도 0 이 된다.
    expect(clampGridSize({ w: 5, h: 2 }, { cols: 0, minW: 3 })).toEqual({ w: 3, h: 2 });
  });

  it('높이 상한이 0 이면 최소 높이로 떨어진다', () => {
    expect(clampGridSize({ w: 2, h: 9 }, { cols: 12, maxH: 0 })).toEqual({ w: 2, h: 1 });
  });

  it('크기가 수가 아니면 최소 크기로 본다 — NaN 이 그대로 새어 나가지 않는다', () => {
    expect(clampGridSize({ w: Number.NaN, h: Number.NaN }, { cols: 12, minW: 2, minH: 3 })).toEqual(
      { w: 2, h: 3 },
    );
  });
});

describe('resizeGridSize', () => {
  // 4×3 패널을 절반 배율(scale 0.5)로 보여주는 미리보기 상자.
  const start = { w: 4, h: 3 };
  const boxW = unitsToPx(4, CELL) * 0.5;
  const boxH = unitsToPx(3, CELL) * 0.5;
  const base = { start, boxW, boxH, cell: CELL, bounds: { cols: 12 } };

  it('끌지 않으면 크기가 그대로다', () => {
    expect(resizeGridSize({ ...base, dxPx: 0, dyPx: 0 })).toEqual({ w: 4, h: 3 });
  });

  it('축소 배율을 반영한다 — 화면 px 를 그대로 쓰지 않는다', () => {
    // 화면에서 한 칸 폭의 절반(=대시보드 한 칸)만큼 끌면 폭이 1 늘어난다.
    const oneCellOnScreen = unitsToPx(1, CELL) * 0.5;
    expect(resizeGridSize({ ...base, dxPx: oneCellOnScreen, dyPx: 0 }).w).toBe(5);

    // 같은 거리를 배율 없이 해석하면 2 칸이 늘어난다 — 그 오류를 막는 것이 요점.
    expect(resizeGridSize({ ...base, dxPx: oneCellOnScreen, dyPx: 0 }).w).not.toBe(6);
  });

  it('세로도 같은 방식으로 환산한다', () => {
    const oneCellOnScreen = unitsToPx(1, CELL) * 0.5;
    expect(resizeGridSize({ ...base, dxPx: 0, dyPx: oneCellOnScreen }).h).toBe(4);
  });

  it('음수 방향으로 끌면 줄어들고 최소치에서 멈춘다', () => {
    expect(resizeGridSize({ ...base, dxPx: -boxW * 2, dyPx: -boxH * 2 })).toEqual({ w: 1, h: 1 });
  });

  it('칼럼 수를 넘겨 끌 수 없다', () => {
    expect(resizeGridSize({ ...base, dxPx: boxW * 10, dyPx: 0, bounds: { cols: 6 } }).w).toBe(6);
  });

  it('셀 미측정이면 상자 비례로 근사한다', () => {
    // 배율을 모르므로 상자 폭이 2배가 되면 단위도 2배로 본다.
    const noCell = { ...base, cell: 0 };
    expect(resizeGridSize({ ...noCell, dxPx: boxW, dyPx: 0 }).w).toBe(8);
  });

  it('드래그 거리가 수가 아니면 크기를 바꾸지 않는다 — 조용히 최소치로 떨어지지 않는다', () => {
    expect(resizeGridSize({ ...base, dxPx: Number.NaN, dyPx: Number.NaN })).toEqual({
      w: 4,
      h: 3,
    });
  });

  it('상자 크기가 0 이면(측정 전) 크기를 바꾸지 않는다', () => {
    expect(resizeGridSize({ ...base, boxW: 0, boxH: 0, dxPx: 500, dyPx: 500 })).toEqual({
      w: 4,
      h: 3,
    });
  });
});

describe('sameGridSize', () => {
  it('값이 같으면 같다고 본다', () => {
    expect(sameGridSize({ w: 2, h: 3 }, { w: 2, h: 3 })).toBe(true);
    expect(sameGridSize({ w: 2, h: 3 }, { w: 2, h: 4 })).toBe(false);
  });

  it('null 은 null 하고만 같다', () => {
    expect(sameGridSize(null, null)).toBe(true);
    expect(sameGridSize(null, { w: 1, h: 1 })).toBe(false);
  });
});
