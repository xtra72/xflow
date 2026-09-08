// 타일 격자 배치 — 순수 계산.
//
// 패널 안에 격자를 두고 타일마다 자리(x, y)와 크기(w, h)를 준다. 종전에는 타일이 흐름
// 배치라 "오류만 크게" 나 "가동시간을 오른쪽 끝에" 같은 것을 만들 수 없었다.
//
// 좌표는 **1부터** 센다. CSS grid 의 줄 번호와 같아, 화면에 보이는 수와 저장된 수가
// 어긋나지 않는다.

/** 격자 크기. */
export interface TileGrid {
  rows: number;
  cols: number;
}

/** 타일 한 장의 자리와 크기. */
export interface TileArea {
  /** 1부터 세는 시작 열 */
  x: number;
  /** 1부터 세는 시작 행 */
  y: number;
  /** 쓰는 열 수(>=1) */
  w: number;
  /** 쓰는 행 수(>=1) */
  h: number;
}

export const MIN_TILE_GRID = 1;
/**
 * 격자 한 변의 최대. 24 를 넘기면 칸이 글자보다 좁아져 어느 타일이 어디 있는지 알 수 없다.
 */
export const MAX_TILE_GRID = 24;

function clampInt(value: unknown, min: number, max: number, fallback: number): number {
  // null / undefined / 빈 문자열은 "값 없음"이라 기본으로 돌아간다. Number(null) 이 0 인
  // 탓에 이것을 가르지 않으면, 저장되지 않은 값이 조용히 최솟값으로 눌린다.
  if (value === null || value === undefined || value === '') return fallback;
  const n = Math.round(Number(value));
  if (!Number.isFinite(n)) return fallback;
  return Math.min(Math.max(n, min), Math.max(min, max));
}

/** 저장된 격자 크기를 읽는다. 손상·미설정 값은 기본으로 떨어진다. */
export function readTileGrid(raw: unknown, fallback: TileGrid): TileGrid {
  const v = (raw ?? {}) as Partial<TileGrid>;
  return {
    rows: clampInt(v.rows, MIN_TILE_GRID, MAX_TILE_GRID, fallback.rows),
    cols: clampInt(v.cols, MIN_TILE_GRID, MAX_TILE_GRID, fallback.cols),
  };
}

/** 자리와 크기를 격자 안으로 가둔다. */
export function clampTileArea(raw: Partial<TileArea> | undefined, grid: TileGrid): TileArea {
  const x = clampInt(raw?.x, 1, grid.cols, 1);
  const y = clampInt(raw?.y, 1, grid.rows, 1);
  return {
    x,
    y,
    // 시작 자리에서 격자 끝까지가 쓸 수 있는 최대다.
    w: clampInt(raw?.w, 1, grid.cols - x + 1, 1),
    h: clampInt(raw?.h, 1, grid.rows - y + 1, 1),
  };
}

/**
 * 타일을 칸 단위로 옮긴다 — 크기는 그대로 두고 격자 안에 가둔다.
 *
 * 격자 끝에 부딪히면 그 자리에 멈춘다. 끌다가 크기까지 줄어들면 손을 떼기 전에는 무엇이
 * 놓일지 알 수 없다.
 */
export function moveTile(area: TileArea, grid: TileGrid, dx: number, dy: number): TileArea {
  return {
    ...area,
    x: clampInt(area.x + dx, 1, grid.cols - area.w + 1, area.x),
    y: clampInt(area.y + dy, 1, grid.rows - area.h + 1, area.y),
  };
}

/** 오른쪽 아래 모서리를 끌어 크기를 바꾼다 — 시작 자리는 그대로다. */
export function resizeTile(area: TileArea, grid: TileGrid, dw: number, dh: number): TileArea {
  return {
    ...area,
    w: clampInt(area.w + dw, 1, grid.cols - area.x + 1, area.w),
    h: clampInt(area.h + dh, 1, grid.rows - area.y + 1, area.h),
  };
}

/** 두 자리가 겹치는지(직사각형 교차). */
function overlaps(a: TileArea, b: TileArea): boolean {
  return a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h;
}

/** 겹치지 않는 첫 자리. 격자가 다 차면 맨 아래에 이어 붙인다. */
function firstFreeSlot(
  placed: TileArea[],
  grid: TileGrid,
  size: { w: number; h: number },
): TileArea {
  for (let y = 1; y <= grid.rows - size.h + 1; y += 1) {
    for (let x = 1; x <= grid.cols - size.w + 1; x += 1) {
      const cand: TileArea = { x, y, ...size };
      if (!placed.some((p) => overlaps(p, cand))) return cand;
    }
  }
  // 격자 밖 — 마지막 줄 아래로. 잘리는 것이 사라지는 것보다 낫다.
  const bottom = placed.reduce((max, p) => Math.max(max, p.y + p.h), 1);
  return { x: 1, y: bottom, ...size };
}

/**
 * 아직 자리를 정하지 않은 타일을 채워 넣는다.
 *
 * 위에서 아래로, 왼쪽에서 오른쪽으로 훑어 **먼저 비는 자리**에 놓는다. 격자가 다 차면
 * 아래로 이어 붙인다 — 자리가 없다고 타일을 지우면 사용자가 고른 항목이 조용히 사라진다.
 *
 * 저장된 자리가 있으면 그대로 쓰되 격자 안으로 가둔다. 격자를 줄였을 때 밖으로 나간
 * 타일이 화면에서 사라지지 않게 하기 위함이다.
 */
export function placeTiles<T extends string>(
  items: readonly T[],
  saved: Record<string, Partial<TileArea>> | undefined,
  grid: TileGrid,
  defaultSize: { w: number; h: number },
): Record<T, TileArea> {
  const out = {} as Record<T, TileArea>;
  const placed: TileArea[] = [];

  // 저장된 자리를 먼저 차지한다 — 자동 배치가 그 위에 겹쳐 놓지 않도록.
  for (const item of items) {
    const own = saved?.[item];
    if (own && (own.x !== undefined || own.y !== undefined)) {
      const area = clampTileArea(own, grid);
      out[item] = area;
      placed.push(area);
    }
  }

  for (const item of items) {
    if (out[item] !== undefined) continue;
    const size = clampTileArea({ x: 1, y: 1, ...defaultSize }, grid);
    const area = firstFreeSlot(placed, grid, { w: size.w, h: size.h });
    out[item] = area;
    placed.push(area);
  }
  return out;
}
