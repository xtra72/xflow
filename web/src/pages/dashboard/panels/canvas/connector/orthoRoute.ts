// 직각 경로가 도형을 피한다 — 후보 격자 위의 최단 경로 (SPEC-CANVAS-017 M1).
//
// ## 왜 격자인가
//
// 직각 경로의 **최적해는 반드시 이 격자 위에 있다.** 장애물 변에서 `MARGIN` 만큼 떨어진
// 선과 두 끝의 좌표 말고 다른 자리를 지날 이유가 없기 때문이며, 그래서 연속 공간을 뒤지지
// 않고도 최적을 얻는다. 후보 축은 이렇게 모은다:
//
//     xs = { A.x, B.x } ∪ { 상자.left − M, 상자.right + M }
//     ys = { A.y, B.y } ∪ { 상자.top  − M, 상자.bottom + M }
//
// 두 끝은 제 좌표가 후보에 들어 있으므로 **저절로 노드**다.
//
// ## 값은 길이 + 꺾임 벌점
//
// 길이만 재면 같은 길이의 **계단**이 최적과 동점이 되어, 한 번 꺾어 갈 길을 열 번 꺾어
// 간다. 벌점은 **캔버스 크기에 비례하지 않는 상수**다 — 비례하게 두면 큰 캔버스에서 꺾임이
// 공짜가 되고 작은 캔버스에서는 길이가 공짜가 된다.
//
// ## 막히면 사라지지 않는다
//
// 길을 찾지 못하거나 장애물이 상한을 넘으면 **부재**를 돌려준다. 부르는 쪽은 그때 015 의
// 세 구간 길로 떨어지며, **선이 사라지는 갈래는 없다**(K5).
//
// **이 모듈은 DOM 도 투영도 모른다.** 점과 상자와 숫자뿐이다.
//
// @spec SPEC-CANVAS-017 REQ-01 · REQ-02 · REQ-06

import type { CanvasBox } from '../canvasGeometry';

/** 장애물 변에서 선이 떨어져 지나는 거리(캔버스 단위). 0 이면 선이 도형에 붙어 스친다. */
export const ORTHO_MARGIN = 8;

/**
 * 꺾임 한 번의 값(캔버스 단위 길이로 환산).
 *
 * 길이와 같은 저울에 얹되 **상수**다(§결정 2). 12 는 `ORTHO_MARGIN` 보다 조금 큰 값이라,
 * "한 칸 돌아가기" 와 "한 번 더 꺾기" 가 맞붙을 때 **덜 꺾는 쪽**이 이긴다.
 */
export const ORTHO_TURN_COST = 12;

/**
 * 장애물 수 상한. 넘으면 부재를 돌려준다(폴백).
 *
 * 격자 노드는 대략 `(2N+2)²` 이므로 N=24 에서 2,500 노드다 — 포인터 사건당 한 번 도는
 * 다익스트라로 감당할 수 있는 크기이고, 그 위로는 화면이 멈춘 것처럼 보인다.
 */
export const ORTHO_MAX_OBSTACLES = 24;

interface Pt {
  readonly x: number;
  readonly y: number;
}

/** 상자를 `ORTHO_MARGIN` 만큼 넓힌다 — 선이 변에 붙어 스치지 않게 하는 여유다. */
function inflate(box: CanvasBox): CanvasBox {
  return {
    x: box.x - ORTHO_MARGIN,
    y: box.y - ORTHO_MARGIN,
    w: box.w + ORTHO_MARGIN * 2,
    h: box.h + ORTHO_MARGIN * 2,
  };
}

/**
 * 축에 나란한 구간이 상자의 **안**을 지나는가.
 *
 * **엄격한 부등호**인 것이 요점이다. 후보 축이 넓힌 상자의 변 위에 정확히 놓이므로,
 * 경계를 "안" 으로 읽으면 그 선들이 통째로 쓸 수 없게 되어 길이 없어진다.
 */
function crosses(a: Pt, b: Pt, box: CanvasBox): boolean {
  const left = box.x;
  const right = box.x + box.w;
  const top = box.y;
  const bottom = box.y + box.h;
  if (a.y === b.y) {
    if (a.y <= top || a.y >= bottom) return false;
    const lo = Math.min(a.x, b.x);
    const hi = Math.max(a.x, b.x);
    return hi > left && lo < right;
  }
  if (a.x <= left || a.x >= right) return false;
  const lo = Math.min(a.y, b.y);
  const hi = Math.max(a.y, b.y);
  return hi > top && lo < bottom;
}

/** 중복을 없애고 정렬한다 — 격자 축이 두 벌이 되면 같은 노드가 둘이 된다. */
function axis(values: readonly number[]): number[] {
  return [...new Set(values)].sort((p, q) => p - q);
}

/**
 * 두 점을 잇는 **가장 짧고 덜 꺾이는** 직각 경로. 막혔거나 너무 크면 **부재**다.
 *
 * 돌려주는 목록은 시작점과 끝점을 **포함한다**. 이웃한 두 점은 언제나 한 축 위다(K1).
 */
export function orthoRoute(
  from: Pt,
  to: Pt,
  obstacles: readonly CanvasBox[],
): Pt[] | undefined {
  if (obstacles.length === 0 || obstacles.length > ORTHO_MAX_OBSTACLES) return undefined;
  const boxes = obstacles.map(inflate);

  const xs = axis([from.x, to.x, ...boxes.flatMap((b) => [b.x, b.x + b.w])]);
  const ys = axis([from.y, to.y, ...boxes.flatMap((b) => [b.y, b.y + b.h])]);
  const xi = new Map(xs.map((v, i) => [v, i]));
  const yi = new Map(ys.map((v, i) => [v, i]));

  const startX = xi.get(from.x);
  const startY = yi.get(from.y);
  const goalX = xi.get(to.x);
  const goalY = yi.get(to.y);
  // 두 끝의 좌표는 축에 넣었으므로 여기서 빌 수 없다. 그래도 단언하지 않는 것은, 훗날
  // 축을 거르는 코드가 생겼을 때 조용히 깨지는 대신 폴백으로 떨어지게 하기 위해서다.
  if (startX === undefined || startY === undefined || goalX === undefined || goalY === undefined) {
    return undefined;
  }

  const cols = xs.length;
  const rows = ys.length;
  const nodeCount = cols * rows;
  // 방향을 상태에 넣는다 — 꺾임 벌점은 **어느 방향으로 들어왔는가**에 달려 있다.
  // 0 = 가로로 들어옴, 1 = 세로로 들어옴, 2 = 출발(아직 방향이 없다).
  const stateCount = nodeCount * 3;
  const dist = new Float64Array(stateCount).fill(Number.POSITIVE_INFINITY);
  const prev = new Int32Array(stateCount).fill(-1);

  const stateOf = (cx: number, cy: number, dir: number): number => (cy * cols + cx) * 3 + dir;
  const start = stateOf(startX, startY, 2);
  dist[start] = 0;

  // 노드 수가 수천 규모라 이진 힙 없이 **선형 추출**로 충분하다 — 자료구조 하나를 더
  // 들이는 값보다 이 함수가 작게 남는 값이 크다.
  const done = new Uint8Array(stateCount);
  for (;;) {
    let best = -1;
    let bestDist = Number.POSITIVE_INFINITY;
    for (let i = 0; i < stateCount; i += 1) {
      if (done[i] === 0 && dist[i]! < bestDist) {
        bestDist = dist[i]!;
        best = i;
      }
    }
    if (best < 0) break;
    done[best] = 1;

    const dir = best % 3;
    const node = (best - dir) / 3;
    const cx = node % cols;
    const cy = (node - cx) / cols;
    if (cx === goalX && cy === goalY) break;

    for (const [dx, dy] of [
      [1, 0],
      [-1, 0],
      [0, 1],
      [0, -1],
    ] as const) {
      const nx = cx + dx;
      const ny = cy + dy;
      if (nx < 0 || ny < 0 || nx >= cols || ny >= rows) continue;
      const a = { x: xs[cx]!, y: ys[cy]! };
      const b = { x: xs[nx]!, y: ys[ny]! };
      if (boxes.some((box) => crosses(a, b, box))) continue;

      const nextDir = dx === 0 ? 1 : 0;
      const turn = dir === 2 || dir === nextDir ? 0 : ORTHO_TURN_COST;
      const step = Math.abs(b.x - a.x) + Math.abs(b.y - a.y);
      const target = stateOf(nx, ny, nextDir);
      const candidate = bestDist + step + turn;
      if (candidate < dist[target]!) {
        dist[target] = candidate;
        prev[target] = best;
      }
    }
  }

  // 목적지의 세 상태 가운데 가장 싼 것에서 되짚는다.
  let end = -1;
  let endDist = Number.POSITIVE_INFINITY;
  for (let dir = 0; dir < 3; dir += 1) {
    const state = stateOf(goalX, goalY, dir);
    if (dist[state]! < endDist) {
      endDist = dist[state]!;
      end = state;
    }
  }
  if (end < 0 || !Number.isFinite(endDist)) return undefined;

  const out: Pt[] = [];
  for (let at = end; at >= 0; at = prev[at]!) {
    const dir = at % 3;
    const node = (at - dir) / 3;
    const cx = node % cols;
    const cy = (node - cx) / cols;
    const point = { x: xs[cx]!, y: ys[cy]! };
    const head = out[0];
    // 같은 자리를 두 번 싣지 않는다 — 출발 상태와 첫 이동이 같은 노드를 가리킨다.
    if (head === undefined || head.x !== point.x || head.y !== point.y) out.unshift(point);
  }
  return out.length >= 2 ? out : undefined;
}
