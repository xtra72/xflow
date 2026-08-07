// IDW 보간 + 값→색 매핑 순수 함수 단위 테스트 (SPEC-HEATMAP-PANEL-001 T4).
// AC-01(다중 센서 보간, 센서점 픽셀=원본값), AC-02(상하한 clamp), AC-03(0-거리 안전) 커버.

import { describe, it, expect } from 'vitest';

import { interpolateIDW, mapValueToColor, DEFAULT_COLOR_TABLE, type IdwPoint } from './idw';

/** Float32Array 전체가 유한값(NaN/Infinity 없음)인지 확인한다. */
function allFinite(grid: Float32Array): boolean {
  for (let i = 0; i < grid.length; i++) {
    if (!Number.isFinite(grid[i])) return false;
  }
  return true;
}

describe('interpolateIDW', () => {
  it('AC-01: 다중 센서를 보간해 격자 전체를 유한값으로 채운다', () => {
    const points: IdwPoint[] = [
      { x: 0.1, y: 0.1, value: 20 },
      { x: 0.9, y: 0.1, value: 24 },
      { x: 0.5, y: 0.9, value: 22 },
    ];
    const grid = interpolateIDW(points, 8, 8, 2);
    expect(grid.length).toBe(64);
    expect(allFinite(grid)).toBe(true);
    // 모든 보간값은 센서값 범위[20, 24] 안에 있어야 한다(IDW 가중 평균 특성).
    for (let i = 0; i < grid.length; i++) {
      expect(grid[i]!).toBeGreaterThanOrEqual(20 - 1e-6);
      expect(grid[i]!).toBeLessThanOrEqual(24 + 1e-6);
    }
  });

  it('AC-01: 센서점과 일치하는 픽셀은 해당 센서 원본 값을 가진다', () => {
    // 3x3 격자에서 중앙 픽셀(1,1) 샘플 좌표 = ((1+0.5)/3, (1+0.5)/3) = (0.5, 0.5).
    const points: IdwPoint[] = [
      { x: 0.5, y: 0.5, value: 42 }, // 중앙 픽셀과 정확히 일치.
      { x: 0.1, y: 0.1, value: 10 },
    ];
    const grid = interpolateIDW(points, 3, 3, 2);
    const centerIdx = 1 * 3 + 1;
    expect(grid[centerIdx]).toBe(42);
  });

  it('AC-03: 0-거리 픽셀에서 NaN/Infinity 없이 원본 값을 반환한다', () => {
    const points: IdwPoint[] = [{ x: 0.5, y: 0.5, value: 25 }];
    const grid = interpolateIDW(points, 3, 3, 2);
    expect(allFinite(grid)).toBe(true);
    // 단일 센서라 모든 픽셀이 그 값(중앙은 정확 일치, 그 외는 가중 평균=단일값).
    for (let i = 0; i < grid.length; i++) {
      expect(grid[i]).toBe(25);
    }
  });

  it('AC-03: 같은 좌표에 겹친 두 센서는 평균한다(분모 예외 없음)', () => {
    const points: IdwPoint[] = [
      { x: 0.5, y: 0.5, value: 20 },
      { x: 0.5, y: 0.5, value: 30 },
    ];
    const grid = interpolateIDW(points, 3, 3, 2);
    const centerIdx = 1 * 3 + 1;
    expect(grid[centerIdx]).toBe(25); // (20+30)/2
    expect(allFinite(grid)).toBe(true);
  });

  it('빈 입력은 예외 없이 전부 0 인 격자를 반환한다(no NaN, AC-E1)', () => {
    const grid = interpolateIDW([], 4, 4, 2);
    expect(grid.length).toBe(16);
    expect(allFinite(grid)).toBe(true);
    for (let i = 0; i < grid.length; i++) expect(grid[i]).toBe(0);
  });

  it('gridW/gridH 가 0 이하면 길이 0 배열을 반환한다', () => {
    expect(interpolateIDW([{ x: 0.5, y: 0.5, value: 1 }], 0, 4, 2).length).toBe(0);
    expect(interpolateIDW([{ x: 0.5, y: 0.5, value: 1 }], 4, -1, 2).length).toBe(0);
  });

  it('power 가 클수록 가까운 센서가 더 지배한다', () => {
    // 픽셀(0,0) 샘플 = (0.25, 0.25). 가까운 센서(0.2,0.2,value=100) vs 먼 센서(0.9,0.9,value=0).
    const points: IdwPoint[] = [
      { x: 0.2, y: 0.2, value: 100 },
      { x: 0.9, y: 0.9, value: 0 },
    ];
    const low = interpolateIDW(points, 2, 2, 1)[0]!;
    const high = interpolateIDW(points, 2, 2, 4)[0]!;
    // power 가 클수록 가까운 센서(100)에 더 가까워진다.
    expect(high).toBeGreaterThan(low);
    expect(high).toBeLessThanOrEqual(100);
  });
});

describe('mapValueToColor', () => {
  const bounds = { min: 18, max: 26 };
  const table = [
    { stop: 0, color: '#0000ff' }, // min 색(파랑)
    { stop: 1, color: '#ff0000' }, // max 색(빨강)
  ];

  it('AC-02: min 이하 값은 min 색으로 clamp 된다', () => {
    expect(mapValueToColor(10, bounds, table)).toEqual([0, 0, 255, 255]);
    expect(mapValueToColor(18, bounds, table)).toEqual([0, 0, 255, 255]);
  });

  it('AC-02: max 이상 값은 max 색으로 clamp 된다', () => {
    expect(mapValueToColor(30, bounds, table)).toEqual([255, 0, 0, 255]);
    expect(mapValueToColor(26, bounds, table)).toEqual([255, 0, 0, 255]);
  });

  it('AC-02: 구간 중간 값은 정지점 색을 선형 보간한다', () => {
    // 중앙 t=0.5 → 파랑↔빨강 중간 = [128, 0, 128].
    expect(mapValueToColor(22, bounds, table)).toEqual([128, 0, 128, 255]);
  });

  it('퇴화 범위(max<=min)는 t=0(첫 색)으로 처리한다(분모 0 방어)', () => {
    expect(mapValueToColor(5, { min: 20, max: 20 }, table)).toEqual([0, 0, 255, 255]);
  });

  it('빈 color_table 은 DEFAULT_COLOR_TABLE 로 폴백한다', () => {
    const rgba = mapValueToColor(18, bounds, []);
    const [r, g, b] = rgba;
    // 기본 gradient 의 첫 정지점(#2166ac)과 일치.
    expect([r, g, b]).toEqual([0x21, 0x66, 0xac]);
    expect(rgba[3]).toBe(255);
  });

  it("3자리 hex('#rgb')도 파싱한다", () => {
    const t3 = [
      { stop: 0, color: '#00f' },
      { stop: 1, color: '#f00' },
    ];
    expect(mapValueToColor(10, bounds, t3)).toEqual([0, 0, 255, 255]);
    expect(mapValueToColor(30, bounds, t3)).toEqual([255, 0, 0, 255]);
  });

  it('DEFAULT_COLOR_TABLE 은 0 과 1 정지점을 포함한다', () => {
    expect(DEFAULT_COLOR_TABLE[0]!.stop).toBe(0);
    expect(DEFAULT_COLOR_TABLE[DEFAULT_COLOR_TABLE.length - 1]!.stop).toBe(1);
  });
});
