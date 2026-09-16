// 선 위의 중간점을 **끼워 넣고 빼는** 산술 (SPEC-CANVAS-011 M10).
//
// ## 도구 층은 몸짓만 안다
//
// 오버레이가 아는 것은 하나다: 선택된 연결선 위로 **두 번째 누름**이 왔다(009 의
// `isSecondPress` — 판정은 그 하나뿐이다, AC-72). 그 누름이 점을 더하는 것인지 빼는
// 것인지, 더한다면 **목록의 어느 자리**인지는 자리의 산술이고 그 산술은 여기 산다.
// `connector/anchors.ts` 의 `anchorGestureAt` 이 앵커에 대해 세운 그 규율 그대로이며,
// 갈라 두면 오버레이가 제 손으로 선의 모양을 다시 지어 "그려진 선과 점이 붙는 선이
// 다르다" 가 시작된다(002 위험 R1).
//
// ## 끼워 넣는 자리는 **눌린 구간의 뒤**다 (REQ-05-a)
//
// 목록 끝에 붙이면 선이 스스로를 가로지른다. 그 증상은 예외도 경고도 없이 **화면에서만**
// 드러나므로, 자리를 정하는 일이 이 모듈의 본체다.
//
// `elbow` · `free` 에서는 그리는 선이 곧 점 목록이라 구간을 그대로 세면 된다. **`curve`
// 는 다르다** — 그려진 곡선은 제어 다각형에서 한참 떨어져 지날 수 있고, 그래서 다각형과의
// 거리로 자리를 고르면 **보이는 곡선을 눌렀는데 엉뚱한 자리**에 점이 든다. 그 어긋남 역시
// 화면에서만 보인다(이 모듈의 시험이 그 배치를 하나 들고 있다).
//
// 그러므로 재는 대상은 언제나 **그려진 잉크**다. 잉크는 그리는 쪽·잡는 쪽이 보는 그
// `connectorPath` 에서 나오고, 곡선은 008 의 `flattenPath` 로 편다 — 히트 층(M7)이 이미
// 지난 그 길이며, 베지어 거리 산술이 여기 새로 적히지 않는다(AC-55).
//
// ## 평탄화가 **출처를 잃지 않게** 한다
//
// `flattenPath` 는 명령 목록을 받아 폴리라인을 내지만 **어느 점이 어느 명령에서 났는지는
// 말하지 않는다.** 목록을 통째로 넘기면 곡선 조각의 경계가 그 폴리라인 안에서 사라지고,
// 가장 가까운 변을 찾아도 그 변이 몇 번째 조각의 것인지 되물을 수단이 없다.
//
// 그래서 **명령마다 따로 편다.** 조각 하나를 펴서 그 조각의 자리를 붙여 두고, 조각들을
// 훑어 최소를 고른다. 출처는 순회 자신이 들고 있으므로 잃을 자리가 없다.
//
// ## 곡선 한 조각은 **자리 둘**에 걸친다
//
// `curveSegments` 는 중간점 하나마다 3차 조각 하나를 낸다(M6). 그 조각은 제어점 `Pk` 의
// 앞뒤로 걸쳐 있다 — 시작은 `P(k-1)`·`Pk` 의 중점이고 끝은 `Pk`·`P(k+1)` 의 중점이다.
// 그러니 조각 `k` 의 **앞 절반**은 구간 `(Pk-1, Pk)` 의 것이고 **뒤 절반**은 구간
// `(Pk, Pk+1)` 의 것이다. 나누는 자리는 그 조각의 **호 길이 중간**이다.
//
// 호 길이를 쓰는 것에 뜻이 있다. 매개변수 중간(`t=0.5`)을 쓰려면 3차를 한 번 더 셈해야
// 하고, 그 순간 베지어 산술이 사는 자리가 하나 는다(AC-55 가 금지한 그것이다). 호 길이는
// 이미 펴 놓은 폴리라인의 변 길이만으로 나오며, 무엇보다 **손이 보는 중간**이 그것이다.
//
// 중간점이 하나인 곡선(`A-P-B`)에서 이 규칙은 곧 "누른 자리가 A 쪽이면 P 앞, B 쪽이면
// P 뒤" 다 — SPEC 이 값으로 못박아 둔 그 경우에서 규칙이 상식과 같다.
//
// ## 쓰는 두 함수는 `anchors.ts` 의 그 둘과 같은 모양이다
//
// `insertPointAt` · `removePointAt` 은 **새 연결선을 돌려준다**(배열을 받지 않는다) —
// `addAnchorAt` · `removeAnchor` 가 노드에 대해 하는 그대로이며, 할 일이 없으면 받은
// 것을 그대로 돌려준다. 배열에 옮기는 일은 오버레이의 `map` 한 줄이다.
//
// ## 점을 싣는 일은 **갈래를 함께 옮긴다** (사용자 신고 2026-09-16)
//
// 직선 위의 두 번째 누름은 종전에 아무 말 없이 아무 일도 하지 않았다. 지금은 다른 셋과
// 같은 뜻이고, 점이 드는 그 순간 `route` 가 점을 들 수 있는 갈래로 **함께** 갈아 끼워진다
// (`insertPointAt` → `routeHosting`). 한 객체 안에서 일어나므로 "직선인데 점을 든" 중간
// 상태가 존재할 자리가 없다 — REQ-04 가 지키는 것이 그 상태의 부재다.
//
// **이 모듈은 DOM 도 React 도 모른다.** 점과 숫자와 순수 함수뿐이라 jsdom 없이 전량
// 단위 시험된다.
//
// @spec SPEC-CANVAS-011 REQ-05 · REQ-05-a · REQ-05-b · AC-67 · AC-68 · AC-71

import { coordinate, type PointGeometry } from '../canvasConfig';
import {
  closestPointOnSegment,
  projectPoint,
  unprojectPoint,
  type CanvasPoint,
  type CanvasProjection,
  type PxBox,
  type PxPoint,
} from '../canvasGeometry';
import { FLATTEN_TOLERANCE_PX, flattenPath } from '../shapes/pathFlatten';
import { connectorPath } from './connectorPath';
import { routeHosting, type ConnectorElement, type ConnectorRoute } from './connectorTypes';

// --- 한 누름이 뜻하는 것 ---------------------------------------------------

/**
 * 선 위의 두 번째 누름이 뜻하는 것.
 *
 * `undefined`(아무 뜻도 없음)와 두 갈래뿐이고 `refuse` 가 없다 — 앵커 쪽은 "여기엔 놓을
 * 수 없다" 는 사실을 화면이 말해야 했지만(A11 의 선·텍스트), 여기서 뜻이 서지 않는 경우는
 * **끊긴 연결** 하나뿐이고 그것은 사용자가 이미 보고 있는 사실이다: 끊긴 선은 애초에
 * 그려지지도 않는다(REQ-08).
 *
 * **직선이 이 목록에서 빠진 것이 M10 의 고침이다.** 종전에는 직선도 "아무 뜻 없음" 이었고
 * 화면은 그 거절을 말하지 않았다 — 도구 넷 가운데 첫째가, 즉 사람이 가장 먼저 긋는 선이
 * 말없이 꺾이지 않았다(사용자 신고 2026-09-16). 지금 직선 위의 두 번째 누름은 다른 셋과
 * **같은 뜻**이며, 그 선이 점을 들 수 있는 갈래로 갈아 끼워지는 일은 점을 싣는 쪽
 * (`insertPointAt`)의 그 한 줄에서 **함께** 일어난다.
 */
export type ConnectorPointGesture =
  | { kind: 'insert'; index: number; at: CanvasPoint }
  | { kind: 'remove'; index: number };

/**
 * 화면의 한 점(스테이지 px)이 뜻하는 중간점 몸짓.
 *
 * 순서가 규칙의 전부다: **오차 안에 중간점이 있으면 빼기**(REQ-05-b · AC-71) → **아니면
 * 그 구간의 뒤에 끼워 넣기**(REQ-05-a · AC-68). 앵커 쪽 순서와 같은 이유로 **빼기가
 * 먼저다** — 점 위를 눌렀는데 그 곁에 점이 하나 더 생기면 사용자는 지우려던 손으로 늘리게
 * 된다.
 *
 * ## `route` 로 **거르지 않는다** (M10 의 고침)
 *
 * 종전에는 이 함수의 첫 줄이 직선을 걸러 냈다. 그 줄이 사라진 자리에 다른 갈래가 서지
 * 않는 것이 요점이다 — 직선의 해석된 목록은 `[시작, 끝]` 둘뿐이라 뺄 중간점이 없고,
 * 그래서 **아래 두 줄이 글자 하나 바뀌지 않고** 직선에 대해 옳게 돈다: 빼기는 찾을 것이
 * 없어 지나가고, 끼워 넣기는 그 하나뿐인 구간의 뒤(색인 0)를 낸다.
 *
 * `route` 는 여전히 인자다 — 자리를 고를 때 **그려진 잉크**를 봐야 하고(아래 `nearestSlot`),
 * 곡선은 제어 다각형과 다른 자리를 지나기 때문이다.
 *
 * ## 오차는 인자이고, 그 값은 앵커의 것을 그대로 쓴다
 *
 * 세 번째 눈금을 만들지 않는다. 오버레이가 넘기는 `ANCHOR_PICK_SLOP_PX` 는 "그려지는 점의
 * 반지름 + 더블클릭이 이미 허락한 손 떨림" 이고, 그 셈은 연결선 손잡이에도 글자 그대로
 * 옳다 — 손잡이도 점이고 판정도 같은 더블클릭이다. 여기에 숫자를 적으면 그리는 크기를
 * 바꾼 날 판정만 옛 값에 남는다.
 *
 * ## 끊긴 연결은 **인자의 부재**로 들어온다 (REQ-08)
 *
 * `resolveConnector` 가 내는 값을 그대로 받으므로, 끊긴 선은 `undefined` 로 여기 닿아
 * 아무 뜻도 없이 돌아간다. 부르는 쪽에 "끊겼으면 부르지 말라" 는 갈래를 두지 않는 것이
 * 요점이다 — 그 갈래는 소비자마다 한 줄씩 늘고, 한 곳에서 빠지는 날 없는 선 위에서
 * 예외가 난다.
 *
 * ## 누름이 잉크에서 얼마나 떨어졌는지는 **보지 않는다**
 *
 * 이 함수를 부르기 전에 히트 층이 이미 "이 누름은 이 선의 잉크다" 라고 답했다
 * (`canvasHitTest` — 잉크와의 거리, REQ-07-b). 여기서 그 거리를 한 번 더 재면 두 자가
 * 생기고, 둘의 여유가 다른 날 "잡히기는 하는데 점이 생기지 않는" 자리가 띠처럼 남는다.
 */
export function connectorPointGestureAt(
  points: readonly CanvasPoint[] | undefined,
  route: ConnectorRoute,
  at: PxPoint,
  proj: CanvasProjection,
  slopPx: number,
  obstacles: readonly PxBox[] = [],
): ConnectorPointGesture | undefined {
  // 점이 둘이 되지 못하면 구간이 없다 — 손으로 지은 목록이 들어와도 던지지 않는다
  // (`connectorPath` 가 빈 목록에 대해 하는 그 선택과 같다).
  if (points === undefined || points.length < 2) return undefined;

  const found = midpointAt(points, at, proj, slopPx);
  if (found !== undefined) return { kind: 'remove', index: found };

  const slot = nearestSlot(points, route, at, proj, obstacles);
  if (slot === undefined) return undefined;
  return { kind: 'insert', index: slot.index, at: unprojectPoint(slot.at, proj) };
}

/**
 * 오차 안에 있는 **중간점의 저장 배열 자리**. 없으면 부재다.
 *
 * 견주는 일을 px 에서 한다 — 손잡이가 그려지는 자리가 px 이고 오차도 px 이므로, 캔버스
 * 단위로 옮겨 재면 축척이 가로·세로로 다른 화면에서 "보이는 원" 이 판정에서는 타원이 된다
 * (`anchorGestureAt` 이 같은 이유로 같은 공간을 골랐다).
 *
 * 양 끝은 보지 않는다. 끝점은 뺄 수 있는 것이 아니라 **갈아 끼우는** 것이고(REQ-07-a),
 * 여기서 받아 주면 두 끝이 없는 연결선이 표현 가능해진다.
 *
 * 동점은 **뒤에 온 것이 이긴다**(`<=`) — 겹쳐 놓인 두 점 가운데 사용자가 보는 것은
 * 나중에 그려진 위쪽 손잡이다(앵커 쪽의 그 규칙과 같다).
 */
function midpointAt(
  points: readonly CanvasPoint[],
  at: PxPoint,
  proj: CanvasProjection,
  slopPx: number,
): number | undefined {
  let best: number | undefined;
  let bestDistance = slopPx * slopPx;
  for (let i = 1; i + 1 < points.length; i += 1) {
    const point = points[i];
    if (point === undefined) continue;
    const px = projectPoint(point, proj);
    const dx = px.x - at.x;
    const dy = px.y - at.y;
    const distance = dx * dx + dy * dy;
    if (distance <= bestDistance) {
      bestDistance = distance;
      // 해석된 목록의 `i` 는 저장 배열에서 `i - 1` 이다 — 앞에 시작 끝점이 하나 있다.
      // 그 사실을 `connectorHandleAt` 과 여기 둘이 읽지만, 읽는 값은 **같은 목록**이다.
      best = i - 1;
    }
  }
  return best;
}

// --- 그려진 잉크의 조각들 --------------------------------------------------

/**
 * 그려진 선의 **한 조각**과 그 출처.
 *
 * `index` 는 이 조각의 (앞 절반의) 자리다 — 저장 배열에 끼워 넣을 색인이며, 구간
 * `(points[index], points[index + 1])` 의 뒤를 뜻한다(REQ-05-a).
 *
 * `halved` 가 참이면 이 조각은 **자리 둘**에 걸친다(곡선의 3차 한 조각 — 머리말 §곡선 한
 * 조각은 자리 둘에 걸친다). 뒤 절반은 `index + 1` 이다.
 */
interface DrawnPiece {
  points: readonly PxPoint[];
  index: number;
  halved: boolean;
}

/**
 * 그려진 선을 **명령마다 하나씩**의 폴리라인으로 편다.
 *
 * 목록을 통째로 `flattenPath` 에 넘기지 않는 것이 이 함수의 전부다 — 통째로 펴면 조각의
 * 경계가 사라져 "이 변이 몇 번째 조각의 것인가" 를 되물을 수단이 없다(머리말 §평탄화가
 * 출처를 잃지 않게 한다).
 *
 * `route` 를 여기서 가르지 않는다. 모양을 정하는 자리는 `connectorPath` **하나**이고
 * (AC-55 의 그 가드), 이 함수는 그것이 낸 명령의 **종류**만 본다 — 폴리라인이면 조각
 * 하나가 자리 하나이고, 3차면 자리 둘에 걸친다.
 */
function drawnPieces(
  points: readonly CanvasPoint[],
  route: ConnectorRoute,
  proj: CanvasProjection,
  obstacles: readonly PxBox[] = [],
): readonly DrawnPiece[] {
  // **그리는 쪽과 같은 장애물 목록을 본다**(017 REQ-04 · K4). 다르면 선 위를 눌렀는데
  // 다른 조각이 답하고, 점이 엉뚱한 자리에 끼워진다.
  const cmds = connectorPath(points, route, proj, obstacles);
  const head = cmds[0];
  if (head === undefined || head.c === 'C') return [];

  const out: DrawnPiece[] = [];
  let from: PxPoint = { x: head.x, y: head.y };
  cmds.slice(1).forEach((cmd, index) => {
    if (cmd.c === 'C') {
      // 시작점을 `M` 하나로 앞에 세워 **이 조각만** 편다. `flattenPath` 는 부분 경로마다
      // 하나씩 내므로 목록은 언제나 한 벌이다.
      const flat = flattenPath(
        [{ c: 'M', x: from.x, y: from.y }, cmd],
        FLATTEN_TOLERANCE_PX,
      )[0];
      out.push({ points: flat?.points ?? [from], index, halved: true });
      from = { x: cmd.x, y: cmd.y };
      return;
    }
    const to = { x: cmd.x, y: cmd.y };
    out.push({ points: [from, to], index, halved: false });
    from = to;
  });
  return out;
}

/** 폴리라인 한 벌의 길이. 호 길이 중간을 찾는 데만 쓴다. */
function polylineLength(points: readonly PxPoint[]): number {
  let total = 0;
  for (let i = 0; i + 1 < points.length; i += 1) {
    const a = points[i];
    const b = points[i + 1];
    if (a === undefined || b === undefined) continue;
    total += Math.hypot(b.x - a.x, b.y - a.y);
  }
  return total;
}

/** 끼워 넣을 자리와, 그 자리의 **선 위의 점**(px). */
interface SlotHit {
  index: number;
  at: PxPoint;
}

/**
 * 누른 자리에 가장 가까운 **잉크 위의 점**과 그 점이 속한 자리.
 *
 * 조각을 훑으며 변마다 가장 가까운 자리를 얻는다. 그 자리를 내는 산술은
 * `canvasGeometry.closestPointOnSegment` **하나**이며, 히트 층의 거리 판정이 지나는 그
 * 함수다 — 잡히는 자리와 점이 놓이는 자리가 두 벌로 갈라질 수 없다(위험 R1).
 *
 * 동점은 **앞선 조각이 이긴다**(`<`). 이음매의 점은 두 조각에 함께 속하고, 앞의 것을
 * 고르면 새 점이 그 이음매 앞에 선다 — 어느 쪽이든 선의 모양은 같으나 규칙이 하나여야
 * 한다.
 */
function nearestSlot(
  points: readonly CanvasPoint[],
  route: ConnectorRoute,
  at: PxPoint,
  proj: CanvasProjection,
  obstacles: readonly PxBox[] = [],
): SlotHit | undefined {
  let best: SlotHit | undefined;
  let bestDistance = Number.POSITIVE_INFINITY;

  for (const piece of drawnPieces(points, route, proj, obstacles)) {
    const total = polylineLength(piece.points);
    let travelled = 0;
    for (let i = 0; i + 1 < piece.points.length; i += 1) {
      const a = piece.points[i];
      const b = piece.points[i + 1];
      if (a === undefined || b === undefined) continue;
      const on = closestPointOnSegment({ x1: a.x, y1: a.y, x2: b.x, y2: b.y }, at);
      const dx = on.x - at.x;
      const dy = on.y - at.y;
      const distance = dx * dx + dy * dy;
      if (distance < bestDistance) {
        bestDistance = distance;
        // 이 조각 안에서 얼마나 지나왔는가 — 호 길이의 절반을 넘겼으면 뒤 절반이다.
        const arc = travelled + Math.hypot(on.x - a.x, on.y - a.y);
        const late = piece.halved && total > 0 && arc * 2 > total;
        best = { index: late ? piece.index + 1 : piece.index, at: on };
      }
      travelled += Math.hypot(b.x - a.x, b.y - a.y);
    }
  }
  return best;
}

// --- 쓰기 ------------------------------------------------------------------

/**
 * 중간점 하나를 `index` 자리에 끼워 넣은 **새 연결선**. 자리가 범위 밖이면 받은 것 그대로다.
 *
 * 좌표는 **캔버스 단위 정수**로 죈다 — 파서의 그 함수(`coordinate`)를 지난다.
 * `freeConnectorEnd` · `moveConnectorPoint` 가 끝점과 허리에 대해 쓰는 바로 그 자이며,
 * 저술하는 쪽과 읽어 들이는 쪽이 다른 규칙으로 죄면 **저장 왕복에 값이 달라진다**:
 * 파서는 `Math.round` 를 지나므로, 여기서 죄지 않고 소수를 실으면 다음에 파일을 읽는
 * 순간 사용자가 찍어 둔 꺾임이 반 칸 옮겨 앉는다(AC-36 이 금지한 그것이다).
 *
 * 끝에 붙이는 것(`index === points.length`)은 범위 안이다 — 마지막 구간을 누른 누름이
 * 그 자리를 낸다(REQ-05-a 의 "구간의 뒤" 가 마지막 구간에서 뜻하는 값이다).
 *
 * ## `route` 를 **함께** 갈아 끼운다 (M10 · 사용자 신고 2026-09-16)
 *
 * 점을 싣는 일과 갈래를 올리는 일이 **객체 하나**에서 일어나는 것이 이 함수의 절반이다.
 * 두 줄로 나누면 그 사이에 `straight` 이면서 `points` 를 든 연결선이 **표현 가능해지고**,
 * 그 상태에서 누가 한 번 그리면 REQ-04 가 거짓인 그림이 화면에 남는다. 승격이 값을 만드는
 * 그 표현 안에 있으므로 중간 상태가 존재할 자리가 없다.
 *
 * 무엇으로 올라가는지는 여기서 정하지 않는다 — `routeHosting` 한 자리이며, 만드는 쪽과
 * 읽어 들이는 쪽이 지나는 그 함수다(`connectorTypes.ts`).
 */
export function insertPointAt(
  connector: ConnectorElement,
  index: number,
  at: CanvasPoint,
): ConnectorElement {
  const points = connector.points ?? [];
  if (index < 0 || index > points.length) return connector;
  const inserted: PointGeometry = { x: coordinate(at.x, 0), y: coordinate(at.y, 0) };
  const next = [...points.slice(0, index), inserted, ...points.slice(index)];
  return { ...connector, route: routeHosting(connector.route, next.length), points: next };
}

/**
 * 중간점 하나를 뺀 **새 연결선**. 없는 자리면 받은 것 그대로다 (REQ-05-b · AC-71).
 *
 * 마지막 하나를 빼면 **키를 지운다.** `[]` 를 남기면 "쓴 적 없음" 과 "다 지웠음" 이
 * 구분되지 않고, 저장 왕복에 없던 `points` 키가 생긴다 — `removeAnchor` 가 `anchors` 에
 * 대해 지키는 그 규율이고, `appendConnector` 가 애초에 빈 배열을 심지 않는 그 이유다.
 *
 * 그리고 그 규율에는 **화면의 뜻**이 있다: 점을 전부 뺀 곡선은 휠 자리가 없으므로
 * `curveSegments` 가 빈 목록을 내고, 그래서 그 선은 **직선으로 돌아온다**(AC-50).
 * "빼면 직선으로 돌아온다" 는 사용자의 말이 여기서 코드 한 줄도 없이 성립한다.
 *
 * **`route` 는 건드리지 않는다.** 위 `insertPointAt` 의 승격에는 짝이 없다 — 되돌릴 값을
 * 알 수 없고, 되돌려도 화면이 달라지지 않기 때문이다(근거는 `routeHosting` 의 주석).
 * 그래서 이 함수가 내는 "직선으로 돌아온 그림" 은 `route` 가 아니라 **점이 없다는 사실**
 * 에서 나온다.
 */
export function removePointAt(connector: ConnectorElement, index: number): ConnectorElement {
  const points = connector.points;
  if (points === undefined) return connector;
  if (index < 0 || index >= points.length) return connector;
  const kept = points.filter((_point, i) => i !== index);
  if (kept.length === 0) {
    const { points: _dropped, ...rest } = connector;
    return rest;
  }
  return { ...connector, points: kept };
}
