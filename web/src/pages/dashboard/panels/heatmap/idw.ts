// IDW(Inverse Distance Weighting) 보간 + 값→색 매핑 순수 함수 (SPEC-HEATMAP-PANEL-001 T4).
//
// DOM 의존이 없는 순수 로직이라 단위 테스트로 필수 커버한다(품질 게이트). 렌더
// (HeatmapCanvas)와 분리해 보간 알고리즘을 격리한다.
//
// 좌표계: 센서점 좌표와 격자 픽셀 샘플 좌표 모두 정규화(0..1) 공간이다. 픽셀 (px,py)는
// 정규화 좌표 ((px+0.5)/gridW, (py+0.5)/gridH) 를 샘플링하며, 격자 인덱스는
// idx = py*gridW + px 이다.
//
// @spec SPEC-HEATMAP-PANEL-001

import type { ColorStop } from './heatmapConfig';

/** 배치된 센서 1개(정규화 좌표 + 판독값). */
export interface IdwPoint {
  /** 정규화 x(0..1 권장). */
  x: number;
  /** 정규화 y(0..1 권장). */
  y: number;
  /** 센서 판독값. */
  value: number;
}

/** 미지정 색상표 폴백용 기본 gradient(파랑→시안→초록→노랑→빨강). */
export const DEFAULT_COLOR_TABLE: ColorStop[] = [
  { stop: 0, color: '#2166ac' },
  { stop: 0.25, color: '#67a9cf' },
  { stop: 0.5, color: '#7fbf7b' },
  { stop: 0.75, color: '#fdae61' },
  { stop: 1, color: '#d73027' },
];

/**
 * IDW 보간으로 grid(gridW×gridH) 픽셀마다 배치 센서점들의 가중 평균 온도를 계산한다.
 *
 * - 빈 입력(points 0개)은 예외 없이 0 으로 채운 Float32Array 를 반환한다(no NaN, AC-E1).
 * - gridW/gridH 가 0 이하면 길이 0 배열을 반환한다.
 * - 어떤 픽셀이 센서점과 정확히 일치하면(0-거리) 분모 예외(NaN/Infinity) 없이 그 센서의
 *   원본 값을 그대로 사용한다(AC-03). 같은 위치에 여러 센서가 겹치면 평균한다.
 * - weight = 1 / dist^power. dist^2(d2) 와 pow(d2, power/2) 로 계산해 sqrt 를 피한다.
 *
 * @returns 길이 gridW*gridH 의 Float32Array(행 우선, idx = py*gridW + px).
 */
export function interpolateIDW(
  points: IdwPoint[],
  gridW: number,
  gridH: number,
  power: number,
): Float32Array {
  if (gridW <= 0 || gridH <= 0) return new Float32Array(0);
  const out = new Float32Array(gridW * gridH);
  if (points.length === 0) return out; // 빈 입력: 전부 0(no NaN).

  const halfPower = power / 2; // pow(d2, power/2) === dist^power

  for (let py = 0; py < gridH; py++) {
    const ny = (py + 0.5) / gridH;
    for (let px = 0; px < gridW; px++) {
      const nx = (px + 0.5) / gridW;

      let wsum = 0;
      let vsum = 0;
      // 0-거리(픽셀=센서점) 겹침 처리: 겹친 센서값을 누적 평균한다(분모 예외 방지).
      let exactSum = 0;
      let exactCount = 0;

      for (let i = 0; i < points.length; i++) {
        const p = points[i]!;
        const dx = nx - p.x;
        const dy = ny - p.y;
        const d2 = dx * dx + dy * dy;
        if (d2 === 0) {
          exactSum += p.value;
          exactCount++;
          continue;
        }
        const w = 1 / Math.pow(d2, halfPower);
        wsum += w;
        vsum += w * p.value;
      }

      const idx = py * gridW + px;
      if (exactCount > 0) {
        out[idx] = exactSum / exactCount;
      } else {
        out[idx] = wsum > 0 ? vsum / wsum : 0;
      }
    }
  }

  return out;
}

/** 값을 [lo, hi] 로 clamp 한다. */
function clamp(value: number, lo: number, hi: number): number {
  if (value < lo) return lo;
  if (value > hi) return hi;
  return value;
}

/** hex 색('#rrggbb' 또는 '#rgb')을 [r,g,b](0..255)로 파싱한다. 실패 시 검정. */
function parseHexColor(hex: string): [number, number, number] {
  let h = hex.trim().replace(/^#/, '');
  if (h.length === 3) {
    // '#rgb' → '#rrggbb'
    h = h[0]! + h[0]! + h[1]! + h[1]! + h[2]! + h[2]!;
  }
  if (h.length !== 6 || !/^[0-9a-fA-F]{6}$/.test(h)) return [0, 0, 0];
  const r = parseInt(h.slice(0, 2), 16);
  const g = parseInt(h.slice(2, 4), 16);
  const b = parseInt(h.slice(4, 6), 16);
  return [r, g, b];
}

/** 두 RGB 를 t(0..1)로 선형 보간한다. */
function lerpColor(
  a: [number, number, number],
  b: [number, number, number],
  t: number,
): [number, number, number] {
  return [
    Math.round(a[0] + (b[0] - a[0]) * t),
    Math.round(a[1] + (b[1] - a[1]) * t),
    Math.round(a[2] + (b[2] - a[2]) * t),
  ];
}

/**
 * 값을 상·하한으로 clamp 한 뒤 정규화(0..1)하여 색상표로 매핑한 RGBA 를 반환한다(REQ-05).
 *
 * - min 이하는 min 색(첫 정지점), max 이상은 max 색(마지막 정지점)으로 clamp 된다(AC-02).
 * - max<=min(퇴화 범위)이면 t=0 으로 처리한다(분모 0 방어).
 * - colorTable 이 비어 있으면 DEFAULT_COLOR_TABLE 로 폴백한다.
 * - 정지점 사이는 인접 두 색을 선형 보간한다.
 *
 * @returns [r, g, b, a] (각 0..255, a=255 불투명).
 */
export function mapValueToColor(
  value: number,
  bounds: { min: number; max: number },
  colorTable: ColorStop[],
): [number, number, number, number] {
  const table = colorTable.length > 0 ? colorTable : DEFAULT_COLOR_TABLE;
  // stop 오름차순 정렬(원본 불변).
  const stops = [...table].sort((a, b) => a.stop - b.stop);

  const { min, max } = bounds;
  const clamped = clamp(value, min, max);
  const t = max > min ? clamp((clamped - min) / (max - min), 0, 1) : 0;

  // 경계: t 가 첫/마지막 정지점 바깥이면 끝 색을 사용한다.
  const first = stops[0]!;
  const last = stops[stops.length - 1]!;
  if (t <= first.stop) {
    const [r, g, b] = parseHexColor(first.color);
    return [r, g, b, 255];
  }
  if (t >= last.stop) {
    const [r, g, b] = parseHexColor(last.color);
    return [r, g, b, 255];
  }

  // t 를 포함하는 구간을 찾아 인접 두 색을 보간한다.
  for (let i = 0; i < stops.length - 1; i++) {
    const lo = stops[i]!;
    const hi = stops[i + 1]!;
    if (t >= lo.stop && t <= hi.stop) {
      const span = hi.stop - lo.stop;
      const localT = span > 0 ? (t - lo.stop) / span : 0;
      const [r, g, b] = lerpColor(parseHexColor(lo.color), parseHexColor(hi.color), localT);
      return [r, g, b, 255];
    }
  }

  // 도달 불가(위 경계 처리로 커버). 안전 폴백: 마지막 색.
  const [r, g, b] = parseHexColor(last.color);
  return [r, g, b, 255];
}
