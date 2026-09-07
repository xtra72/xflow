// 타일 격자 배치 — 순수 계산 검증.

import { describe, it, expect } from 'vitest';

import {
  clampTileArea,
  MAX_TILE_GRID,
  moveTile,
  placeTiles,
  readTileGrid,
  resizeTile,
  type TileArea,
} from './tileLayout';

const GRID = { rows: 4, cols: 8 };

describe('readTileGrid', () => {
  it('미설정이면 기본을 쓴다', () => {
    expect(readTileGrid(undefined, GRID)).toEqual(GRID);
    expect(readTileGrid({}, GRID)).toEqual(GRID);
  });

  it('저장된 값을 쓴다', () => {
    expect(readTileGrid({ rows: 2, cols: 6 }, GRID)).toEqual({ rows: 2, cols: 6 });
  });

  it('한쪽만 있으면 나머지는 기본을 따른다', () => {
    expect(readTileGrid({ cols: 12 }, GRID)).toEqual({ rows: 4, cols: 12 });
  });

  it('손상 값은 기본으로 떨어진다', () => {
    expect(readTileGrid({ rows: 'x', cols: null }, GRID)).toEqual(GRID);
  });

  it('한계 밖은 가둔다 — 칸이 글자보다 좁아지면 읽을 수 없다', () => {
    expect(readTileGrid({ rows: 0, cols: 999 }, GRID)).toEqual({ rows: 1, cols: MAX_TILE_GRID });
  });
});

describe('clampTileArea', () => {
  it('격자 안이면 그대로', () => {
    expect(clampTileArea({ x: 2, y: 2, w: 2, h: 2 }, GRID)).toEqual({ x: 2, y: 2, w: 2, h: 2 });
  });

  it('크기는 시작 자리에서 격자 끝까지로 묶인다', () => {
    expect(clampTileArea({ x: 7, y: 3, w: 9, h: 9 }, GRID)).toEqual({ x: 7, y: 3, w: 2, h: 2 });
  });

  it('미설정은 좌상단 1x1', () => {
    expect(clampTileArea(undefined, GRID)).toEqual({ x: 1, y: 1, w: 1, h: 1 });
  });
});

describe('moveTile', () => {
  const area: TileArea = { x: 1, y: 1, w: 2, h: 2 };

  it('칸 단위로 옮기고 크기는 그대로 둔다', () => {
    expect(moveTile(area, GRID, 2, 1)).toEqual({ x: 3, y: 2, w: 2, h: 2 });
  });

  it('격자 끝에 부딪히면 멈춘다 — 밖으로 나가거나 줄어들지 않는다', () => {
    expect(moveTile(area, GRID, 99, 99)).toEqual({ x: 7, y: 3, w: 2, h: 2 });
    expect(moveTile(area, GRID, -99, -99)).toEqual({ x: 1, y: 1, w: 2, h: 2 });
  });
});

describe('resizeTile', () => {
  it('시작 자리는 그대로 두고 크기만 바꾼다', () => {
    expect(resizeTile({ x: 2, y: 2, w: 2, h: 1 }, GRID, 1, 1)).toEqual({ x: 2, y: 2, w: 3, h: 2 });
  });

  it('크기는 시작 자리에서 격자 끝까지로 묶인다', () => {
    expect(resizeTile({ x: 7, y: 3, w: 1, h: 1 }, GRID, 9, 9)).toEqual({ x: 7, y: 3, w: 2, h: 2 });
    expect(resizeTile({ x: 1, y: 1, w: 2, h: 2 }, GRID, -9, -9)).toEqual({ x: 1, y: 1, w: 1, h: 1 });
  });
});

describe('placeTiles', () => {
  const items = ['a', 'b', 'c', 'd', 'e'] as const;

  it('8x4 격자에 2x2 타일을 왼쪽에서 오른쪽으로 채운다', () => {
    const got = placeTiles(items, undefined, GRID, { w: 2, h: 2 });
    expect(got.a).toEqual({ x: 1, y: 1, w: 2, h: 2 });
    expect(got.b).toEqual({ x: 3, y: 1, w: 2, h: 2 });
    expect(got.d).toEqual({ x: 7, y: 1, w: 2, h: 2 });
    // 한 줄(8칸)에 넷이 들어가고 다섯째는 다음 줄로 내려간다.
    expect(got.e).toEqual({ x: 1, y: 3, w: 2, h: 2 });
  });

  it('8x2 격자에 4x2 타일 둘이 딱 맞는다', () => {
    const got = placeTiles(['x', 'y'] as const, undefined, { rows: 2, cols: 8 }, { w: 4, h: 2 });
    expect(got.x).toEqual({ x: 1, y: 1, w: 4, h: 2 });
    expect(got.y).toEqual({ x: 5, y: 1, w: 4, h: 2 });
  });

  it('저장된 자리가 자동 배치보다 먼저다 — 그 위에 겹쳐 놓지 않는다', () => {
    const got = placeTiles(items, { c: { x: 1, y: 1, w: 2, h: 2 } }, GRID, { w: 2, h: 2 });
    expect(got.c).toEqual({ x: 1, y: 1, w: 2, h: 2 });
    expect(got.a).toEqual({ x: 3, y: 1, w: 2, h: 2 });
  });

  it('격자를 줄이면 밖으로 나간 타일을 안으로 들인다 — 사라지면 안 된다', () => {
    const got = placeTiles(['a'] as const, { a: { x: 7, y: 3, w: 2, h: 2 } }, { rows: 2, cols: 4 }, { w: 2, h: 2 });
    expect(got.a.x).toBeLessThanOrEqual(4);
    expect(got.a.y).toBeLessThanOrEqual(2);
  });

  it('격자가 다 차면 아래로 이어 붙인다 — 자리가 없다고 타일을 지우지 않는다', () => {
    const small = { rows: 1, cols: 2 };
    const got = placeTiles(['a', 'b'] as const, undefined, small, { w: 2, h: 1 });
    expect(got.a).toEqual({ x: 1, y: 1, w: 2, h: 1 });
    expect(got.b.y).toBeGreaterThan(1);
  });

  it('고른 항목마다 자리를 준다', () => {
    const got = placeTiles(items, undefined, GRID, { w: 2, h: 2 });
    expect(Object.keys(got).sort()).toEqual([...items].sort());
  });
});
