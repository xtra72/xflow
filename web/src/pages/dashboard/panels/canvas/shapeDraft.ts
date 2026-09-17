// 선을 그어 짓는 도형 — 꼭짓점 목록에서 `path` 요소의 재료로 (SPEC-CANVAS-022 M1).
//
// ## 새 기하를 들이지 않는다
//
// 윤곽의 모양은 `connectorPath` 가 낸다(K3). 015~021 이 직각에 쏟은 판단 — 축을 앵커에서
// 읽고, 막히면 돌고, 고정하면 피하지 않는다 — 이 도형 윤곽에서도 **저절로** 같아지는 것이
// 그 선택의 값이다. 여기서 다시 그리면 그 판단들이 한 벌 더 생기고, 두 벌은 갈린다.
//
// 로컬 정규화도 마찬가지다. `toLocalCommands` 는 SVG 가져오기가 쓰는 그 함수이며,
// "사용자 단위 → 경로 로컬 정수" 라는 나눗셈이 이 패널에서 나타나는 **유일한 자리**다.
//
// ## 이 모듈은 DOM 도 React 도 모른다
//
// 상태 칸(찍은 점들 · 지금 손의 자리)은 오버레이가 든다. 여기는 값에서 값을 낸다.
//
// @spec SPEC-CANVAS-022 REQ-03 · REQ-06 · REQ-07

import {
  MIN_ELEMENT_EXTENT,
  coordinate,
  type BoxGeometry,
  type CanvasSize,
  type PointGeometry,
} from './canvasConfig';
import type { CanvasProjection } from './canvasGeometry';
import { connectorPath } from './connector/connectorPath';
import type { ConnectorRoute } from './connector/connectorTypes';
import { toLocalCommands } from './svgimport/svgImportPlan';
import type { PathCommand } from './shapes/pathTypes';

/** 윤곽을 닫으려면 꼭짓점이 **셋**은 있어야 한다 — 둘을 닫은 것은 겹친 선이다. */
export const SHAPE_MIN_VERTICES = 3;

/**
 * 캔버스 단위 = px 인 투영.
 *
 * `connectorPath` 는 투영을 받아 px 를 낸다. 화면 축척을 지나고 되돌아오면 왕복 오차가
 * 정수 죔에 섞이고, 그 오차는 "그린 자리와 만들어진 자리가 조금 다르다" 로 보인다
 * (§결정 3). 무대 크기를 캔버스 크기로 두면 그 왕복이 아예 없다.
 */
export function identityProjection(canvas: CanvasSize): CanvasProjection {
  return { stage: { width: canvas.width, height: canvas.height }, canvas };
}

/**
 * 꼭짓점 목록을 **캔버스 단위 명령**으로 (§결정 2 · §결정 3).
 *
 * 닫는 점은 목록에 넣지 않는다 — `Z` 가 그 일을 하고, 시작점을 한 번 더 실으면 길이 0 인
 * 구간이 하나 생겨 잡는 쪽의 거리 산술에서 0 으로 나누는 자리가 된다(015 K2).
 */
export function draftCommands(
  vertices: readonly PointGeometry[],
  route: ConnectorRoute,
  canvas: CanvasSize,
): PathCommand[] {
  const proj = identityProjection(canvas);
  const open = connectorPath(vertices, route, proj);
  // 명령 어휘가 이미 같다 — `Z` 하나가 차이다.
  return open.map((cmd): PathCommand => ({ ...cmd }));
}

/** 명령이 닿는 모든 점 — 곡선은 **제어점까지** 감싼다(§결정 4). */
function commandPoints(commands: readonly PathCommand[]): { x: number; y: number }[] {
  const out: { x: number; y: number }[] = [];
  for (const cmd of commands) {
    if (cmd.c === 'Z') continue;
    if (cmd.c === 'C') {
      out.push({ x: cmd.x1, y: cmd.y1 }, { x: cmd.x2, y: cmd.y2 });
    }
    out.push({ x: cmd.x, y: cmd.y });
  }
  return out;
}

/**
 * 윤곽을 감싸는 상자. **너비·높이가 0 이 되지 않는다**(REQ-07 · K5).
 *
 * 0 축은 `toLocalCommands` 의 나눗셈에서 0 으로 나누는 자리이고, 그 결과(NaN)는 그 요소를
 * 화면에서 지운다 — 한 점에 모아 찍은 윤곽이 **보이지 않는 요소**가 되는 형상이다.
 */
export function draftBox(commands: readonly PathCommand[]): BoxGeometry {
  const points = commandPoints(commands);
  const first = points[0];
  if (first === undefined) {
    return { x: 0, y: 0, w: MIN_ELEMENT_EXTENT, h: MIN_ELEMENT_EXTENT };
  }
  let minX = first.x;
  let minY = first.y;
  let maxX = first.x;
  let maxY = first.y;
  for (const point of points) {
    minX = Math.min(minX, point.x);
    minY = Math.min(minY, point.y);
    maxX = Math.max(maxX, point.x);
    maxY = Math.max(maxY, point.y);
  }
  const x = coordinate(minX, 0);
  const y = coordinate(minY, 0);
  return {
    x,
    y,
    w: Math.max(MIN_ELEMENT_EXTENT, coordinate(maxX, 0) - x),
    h: Math.max(MIN_ELEMENT_EXTENT, coordinate(maxY, 0) - y),
  };
}

/** 지어진 도형의 재료 — 상자와 그 상자 로컬의 닫힌 명령. */
export interface ShapeDraft {
  readonly geometry: BoxGeometry;
  readonly path: PathCommand[];
}

/**
 * 꼭짓점 목록을 **닫힌 도형의 재료**로 (REQ-03 · REQ-06).
 *
 * 꼭짓점이 `SHAPE_MIN_VERTICES` 미만이면 `undefined` 다 — 던지지 않는 쪽을 고른다. 부르는
 * 쪽이 이미 세었더라도 여기서 한 번 더 막아야, 그 셈이 두 벌이 되는 날 조용히 겹친 선이
 * 요소로 남지 않는다.
 */
export function shapeDraftOf(
  vertices: readonly PointGeometry[],
  route: ConnectorRoute,
  canvas: CanvasSize,
): ShapeDraft | undefined {
  if (vertices.length < SHAPE_MIN_VERTICES) return undefined;
  const commands = draftCommands(vertices, route, canvas);
  if (commands.length === 0) return undefined;
  const geometry = draftBox(commands);
  const local = toLocalCommands(commands, {
    minX: geometry.x,
    minY: geometry.y,
    width: geometry.w,
    height: geometry.h,
  });
  // **`Z` 를 여기서 붙인다.** 이 한 줄이 "선" 과 "도형" 을 가르며, `closedSeedStyle` 이
  // 읽는 것도 이 명령이다 — 붙이는 자리와 읽는 자리가 하나씩이다.
  return { geometry, path: [...local, { c: 'Z' }] };
}
