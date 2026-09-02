// marching squares 등고선(등치선) 순수 알고리즘 (SPEC-HEATMAP-PANEL-003 T1).
//
// MVP idw.ts 가 계산한 스칼라 격자(Float32Array, gridW×gridH, 행 우선)를 입력으로,
// 사용자가 지정한 등치값(iso-value)마다 등치선 세그먼트를 산출한다. **재보간하지 않는다**
// (REQ-01/AC-E5) — 전달받은 격자만 소비한다. DOM 의존이 없는 순수 로직이라 골든 케이스
// 단위 테스트로 필수 커버한다(품질 게이트).
//
// 좌표계(R5, idw.ts 와 정합): 격자점 (gx,gy)의 정규화 좌표는 셀-중심 규약
//   ((gx+0.5)/gridW, (gy+0.5)/gridH) 이다(히트맵 blit 과 동일 정합). 산출 세그먼트 좌표는
//   모두 정규화(0..1) 공간이며, ContourLayer 의 viewBox 0..1 에 직접 매핑된다.
//
// @spec SPEC-HEATMAP-PANEL-003

/** 등치선 세그먼트 1개(정규화 0..1 좌표의 선분). */
export interface Segment {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}

/** 등치 레벨 개수 기본값(REQ-05). */
export const DEFAULT_CONTOUR_LEVEL_COUNT = 5;

/** 유한 숫자인지 확인한다(NaN/Infinity 방어). */
function isFinite(v: number): boolean {
  return Number.isFinite(v);
}

/**
 * 등치 레벨(iso-value) 목록을 산출한다(REQ-05, AC-03/AC-E3).
 *
 * - `explicit`(명시 값 목록)이 있으면 그 값을 우선 사용하고 `count`를 무시한다.
 * - `explicit`이 없으면 (min, max) 구간을 `count`등분한 내부 경계값을 산출한다(끝점 제외).
 *   끝점(min/max)에서의 등치선은 퇴화(빈/전체)하므로 내부값만 낸다.
 * - 두 경우 모두 **범위 밖(min<level<max 아님) 레벨은 필터**한다(AC-E3): 격자 범위를 벗어난
 *   등치값은 잘못된(빈/전체) 선을 유발하므로 제외한다.
 * - 퇴화 범위(max<=min)·비유한 범위·count<=0 은 빈 배열로 방어한다.
 */
export function resolveLevels(
  bounds: { min: number; max: number },
  count?: number,
  explicit?: number[],
): number[] {
  const { min, max } = bounds;
  // 퇴화/비유한 범위: 등치선 불가.
  if (!isFinite(min) || !isFinite(max) || max <= min) return [];

  // explicit 우선: count 무시, 범위 밖/비유한 값 필터(AC-03/AC-E3).
  if (explicit && explicit.length > 0) {
    return explicit.filter((v) => isFinite(v) && v > min && v < max);
  }

  // count 균등 분할: (min,max) 내부를 count 등분한 경계값(끝점 제외).
  const n = count ?? 0;
  if (!isFinite(n) || n <= 0) return [];
  const c = Math.trunc(n);
  const levels: number[] = [];
  for (let i = 1; i <= c; i++) {
    levels.push(min + ((max - min) * i) / (c + 1));
  }
  return levels;
}

/** 격자점 (gx,gy)의 정규화 x 좌표(셀-중심 규약, R5). */
function nx(gx: number, gridW: number): number {
  return (gx + 0.5) / gridW;
}

/** 격자점 (gx,gy)의 정규화 y 좌표(셀-중심 규약, R5). */
function ny(gy: number, gridH: number): number {
  return (gy + 0.5) / gridH;
}

/**
 * 등치값(level) 교차 위치 보간 파라미터 t 를 구한다(선형 보간).
 * v0/v1 은 교차 시 서로 다름이 보장되므로(한쪽 >= level, 다른쪽 < level) 분모 0 이 아니다.
 */
function edgeT(v0: number, v1: number, level: number): number {
  return (level - v0) / (v1 - v0);
}

/**
 * 단일 등치값(level)에 대해 격자에 marching squares 를 적용해 세그먼트 집합을 산출한다(REQ-01).
 *
 * - 셀은 인접 4격자점(TL/TR/BR/BL)으로 구성되며 case 0..15 로 분기한다.
 * - 셀 모서리에서 **선형 보간**으로 교차점을 구한다(정규화 좌표).
 * - saddle(케이스 5/10)은 **셀 4코너 평균(중앙값) 기준**으로 결정적으로 분기한다(AC-E1):
 *   중앙값 >= level 이면 "안(inside)"이 연결된 것으로 보고 두 바깥 코너를 각각 격리하고,
 *   중앙값 < level 이면 두 안 코너를 각각 격리한다. 끊기거나 교차하는 선을 만들지 않는다.
 * - 빈 격자/1행·1열 격자(셀 없음)는 빈 배열을 반환한다.
 *
 * @param grid 스칼라 격자(행 우선, idx = gy*gridW + gx). idw.ts 출력 재사용(재보간 없음).
 */
export function computeContours(
  grid: Float32Array,
  gridW: number,
  gridH: number,
  level: number,
): Segment[] {
  const segments: Segment[] = [];
  // 셀이 성립하지 않는 격자(빈/1행/1열)·비유한 level 은 방어적으로 빈 배열.
  if (gridW < 2 || gridH < 2) return segments;
  if (!isFinite(level)) return segments;
  if (grid.length < gridW * gridH) return segments;

  for (let gy = 0; gy < gridH - 1; gy++) {
    for (let gx = 0; gx < gridW - 1; gx++) {
      // 셀 4코너 값(TL/TR/BR/BL).
      const a = grid[gy * gridW + gx]!; // TL
      const b = grid[gy * gridW + (gx + 1)]!; // TR
      const c = grid[(gy + 1) * gridW + (gx + 1)]!; // BR
      const d = grid[(gy + 1) * gridW + gx]!; // BL

      // "안(inside)" = 값 >= level. case 비트: TL=8, TR=4, BR=2, BL=1.
      const tl = a >= level;
      const tr = b >= level;
      const br = c >= level;
      const bl = d >= level;
      const caseIndex =
        (tl ? 8 : 0) | (tr ? 4 : 0) | (br ? 2 : 0) | (bl ? 1 : 0);
      if (caseIndex === 0 || caseIndex === 15) continue; // 전부 밖/전부 안: 선 없음.

      // 셀 격자 좌표(정규화).
      const xL = nx(gx, gridW);
      const xR = nx(gx + 1, gridW);
      const yT = ny(gy, gridH);
      const yB = ny(gy + 1, gridH);

      // 셀 4모서리 교차점(필요 시 계산). 정규화 좌표.
      const topPt = () => ({ x: xL + edgeT(a, b, level) * (xR - xL), y: yT }); // TL-TR
      const rightPt = () => ({ x: xR, y: yT + edgeT(b, c, level) * (yB - yT) }); // TR-BR
      const bottomPt = () => ({ x: xL + edgeT(d, c, level) * (xR - xL), y: yB }); // BL-BR
      const leftPt = () => ({ x: xL, y: yT + edgeT(a, d, level) * (yB - yT) }); // TL-BL

      const push = (
        p: { x: number; y: number },
        q: { x: number; y: number },
      ) => {
        segments.push({ x1: p.x, y1: p.y, x2: q.x, y2: q.y });
      };

      switch (caseIndex) {
        // 단일 코너 격리(1개 세그먼트).
        case 1: // BL
          push(leftPt(), bottomPt());
          break;
        case 2: // BR
          push(bottomPt(), rightPt());
          break;
        case 4: // TR
          push(topPt(), rightPt());
          break;
        case 8: // TL
          push(topPt(), leftPt());
          break;
        // 단일 코너만 바깥(3개 안) — 위 케이스의 여집합.
        case 14: // ~BL
          push(leftPt(), bottomPt());
          break;
        case 13: // ~BR
          push(bottomPt(), rightPt());
          break;
        case 11: // ~TR
          push(topPt(), rightPt());
          break;
        case 7: // ~TL
          push(topPt(), leftPt());
          break;
        // 인접 두 코너(행/열) — 관통 1개 세그먼트.
        case 3: // BL+BR (하단 행)
        case 12: // TL+TR (상단 행)
          push(leftPt(), rightPt());
          break;
        case 6: // TR+BR (우측 열)
        case 9: // TL+BL (좌측 열)
          push(topPt(), bottomPt());
          break;
        // saddle(대각) — 중앙값 기준 결정적 분기(AC-E1).
        case 5: {
          // TR+BL 안(high). 중앙값>=level: high 연결 → 바깥 코너(TL,BR) 격리.
          const center = (a + b + c + d) / 4;
          if (center >= level) {
            push(topPt(), leftPt()); // TL 격리
            push(rightPt(), bottomPt()); // BR 격리
          } else {
            push(topPt(), rightPt()); // TR 격리
            push(leftPt(), bottomPt()); // BL 격리
          }
          break;
        }
        case 10: {
          // TL+BR 안(high). 중앙값>=level: high 연결 → 바깥 코너(TR,BL) 격리.
          const center = (a + b + c + d) / 4;
          if (center >= level) {
            push(topPt(), rightPt()); // TR 격리
            push(leftPt(), bottomPt()); // BL 격리
          } else {
            push(topPt(), leftPt()); // TL 격리
            push(rightPt(), bottomPt()); // BR 격리
          }
          break;
        }
        default:
          break;
      }
    }
  }

  return segments;
}

/**
 * 세그먼트 집합을 SVG path `d` 문자열로 변환한다. 각 세그먼트는 `M x1 y1 L x2 y2`.
 * 빈 입력은 빈 문자열을 반환한다.
 */
export function segmentsToPath(segments: Segment[]): string {
  return segments.map((s) => `M ${s.x1} ${s.y1} L ${s.x2} ${s.y2}`).join(' ');
}
