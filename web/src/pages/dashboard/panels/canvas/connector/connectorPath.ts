// 연결선의 **화면 위 모양** — 점 목록과 `route` 에서 경로 명령 하나를 낸다
// (SPEC-CANVAS-011 M6 · M7).
//
// ## 왜 명령 목록인가
//
// 그리는 쪽(M6)과 잡는 쪽(M7)이 **같은 곡선**을 봐야 한다. 둘이 저마다 "곡선이면
// `curveSegments`, 아니면 폴리라인" 을 적으면 그 갈래가 두 벌이 되고, 한쪽만 고쳐지는 날
// "그려진 선과 잡히는 선이 다르다" 가 시작된다 — 002 가 위험 R1 로 이름 적어 둔 그 결함이며,
// 예외도 경고도 없이 **화면에서만** 드러난다. M5 가 참조 푸는 자리를 하나로 못박은 것과 같은
// 규율을 **모양**에 대해 한 번 더 적용한 것이 이 모듈이다.
//
// 명령 목록을 고른 것은 그것이 **두 소비자가 모두 읽을 수 있는 유일한 형상**이기 때문이다.
// 그리는 쪽은 명령마다 대응하는 context 호출을 내고(`M`→`moveTo` · `L`→`lineTo` ·
// `C`→`bezierCurveTo`), 잡는 쪽은 008 의 `flattenPath` 에 그대로 넘긴다. 008 이 경로를 위해
// 지은 평탄화를 011 이 **두 번째 호출자**로 쓰는 자리가 바로 여기이며, 그래서 베지어 거리
// 산술이 새로 적히지 않는다(AC-55).
//
// ## `Z` 가 없는 것이 **타입에** 적혀 있다
//
// 연결선은 두 자리를 잇는 **열린** 선이다. 닫히지 않으므로 안쪽이라는 개념이 없고, 그래서
// 잡는 쪽도 내부 판정(`isInsidePath`)을 부르지 않는다. 그 사실을 산문이 아니라 반환 타입에
// 적어 두면 닫힘 명령을 더하는 일이 문법으로 막힌다.
//
// ## 투영이 여기 **한 번** 있다
//
// 점 목록은 캔버스 단위로 들어오고(M5 의 `resolveConnector`), 나가는 명령은 스테이지 로컬
// px 다. 곡선을 **투영한 뒤에** 펴는 것이 M6 이 고른 순서이며(투영은 축마다 상수를 곱하는
// 아핀 변환이라 어느 쪽에서 펴도 같은 곡선이지만, 이쪽이면 AC-48 이 재는 제어점이 곧 그려진
// 좌표다), 두 소비자가 이 함수를 지나므로 그 순서가 두 벌이 되지 않는다.
//
// **이 모듈은 DOM 을 모른다.** 점과 숫자뿐이라 jsdom 없이 전량 단위 시험된다.
//
// @spec SPEC-CANVAS-011 REQ-04 · REQ-07-b · AC-55

import {
  projectPoint,
  type CanvasPoint,
  type CanvasProjection,
  type PxPathCommand,
  type PxPoint,
} from '../canvasGeometry';
import { curveSegments } from './connectorCurve';
// 장애물 회피의 산술은 잎 모듈이 소유한다(SPEC-CANVAS-017). 점과 상자와 숫자뿐이다.
import { orthoPathClear, orthoRoute, orthoStub } from './orthoRoute';
import { splitAt, type OrthoSplit } from './orthoSplits';
import type { ConnectorRoute } from './connectorTypes';
import type { ConnectorRouting } from './connectorObstacles';

/**
 * 연결선이 낼 수 있는 경로 명령 — `Z` 를 **뺀** 008 의 어휘 그대로다.
 *
 * 008 의 `PxPathCommand` 를 좁혀서 쓰는 것이지 새 어휘를 짓는 것이 아니다. 좁힌 배열은
 * 넓은 배열이 필요한 자리(`flattenPath`)에 그대로 들어가므로 변환이 끼어들지 않는다.
 */
export type ConnectorPathCommand = Exclude<PxPathCommand, { c: 'Z' }>;

/**
 * 해석된 점 목록(`[시작, …중간점, 끝]`, **캔버스 단위**)을 스테이지 로컬 px 경로 명령으로 편다.
 *
 * - 점이 하나도 없으면 **빈 목록**이다. 그리는 쪽은 그때 아무 호출도 내지 않고, 잡는 쪽은
 *   아무것도 잡지 않는다 — 없는 선을 그릴 수도 잡을 수도 없다.
 * - `straight` · `elbow` · `free` 는 셋이 **같은 길**을 지난다. 다른 것은 점이 어디서
 *   왔는가 뿐이며(사람이 찍었는가, 손이 그은 궤적인가) 그 출처는 모양에 닿지 않는다.
 * - `ortho` 도 **그 길을 지난다**(SPEC-CANVAS-015). 갈래를 세우는 대신 점 목록에 모서리를
 *   끼워 넣으므로, 폴리라인 갈래는 넓어진 목록을 **모른 채** 그대로 편다.
 * - `curve` 만 갈라지되, 중간점이 없으면 `curveSegments` 가 빈 목록을 내므로 **그 갈래도
 *   같은 길로 떨어진다**(AC-50). 네 갈래가 같아야 한다는 REQ-04-b 가 조건문이 아니라
 *   **구조**로 지켜지는 자리다.
 */
/**
 * 두 점을 **직각으로만** 잇는 중간 모서리 (SPEC-CANVAS-015 §결정 1).
 *
 * ## 가운데서 꺾는다
 *
 * 두 점을 직각으로 잇는 길은 여럿이다. 015 는 **긴 축을 반으로 갈라** 세 구간으로 잇는다:
 *
 *     |dx| ≥ |dy| 이면  가로 → 세로 → 가로   (가운데 x 에서 꺾는다)
 *     |dx| <  |dy| 이면  세로 → 가로 → 세로   (가운데 y 에서 꺾는다)
 *
 * **한 번만 꺾는 L 을 기각한다.** L 은 두 끝 가운데 **한쪽에 붙어** 꺾이므로 같은 두 점을
 * 이어도 어느 쪽에 붙느냐로 그림이 달라지고, 그 선택에 근거가 없다. 가운데서 꺾으면 두 끝이
 * 대칭이라 "왜 저쪽으로 돌았는가" 라는 물음이 생기지 않는다.
 *
 * **긴 축을 고르는 것**에도 이유가 있다. 좌우로 먼 두 점을 세로부터 꺾으면 선이 도형 위로
 * 올라탔다 내려온다 — 사람이 손으로 그을 때 하지 않는 모양이다.
 *
 * ## 길이 0 인 구간을 내지 않는다 (K2)
 *
 * 두 점이 이미 한 축 위에 있으면 모서리가 **없다** — 그 자리에서 가운데를 꺾으면 같은 점이
 * 둘 생기고, 그 빈 구간은 잡는 쪽의 거리 산술에서 0 으로 나누는 자리가 된다.
 */
function orthoCorners(a: PxPoint, b: PxPoint, axis?: OrthoAxis): PxPoint[] {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  // 이미 한 축 위다 — 곧은 구간 하나로 족하다.
  if (dx === 0 || dy === 0) return [];
  // **축이 정해져 있으면 그것을 따른다**(SPEC-CANVAS-018 §안정한 모양).
  //
  // 015 는 축을 `|dx|` 와 `|dy|` 로 골랐다. 그 규칙은 상자를 세로로 옮기다 `|dy|` 가
  // `|dx|` 를 넘는 순간 **가로-세로-가로에서 세로-가로-세로로 뒤집힌다** — 사용자에게는
  // "세로 선만 길어져야 하는데 모양이 통째로 바뀐다" 로 보인다(신고 2026-09-17).
  //
  // 축은 **앵커가 나가는 쪽**에서 와야 한다. `e` 앵커는 가로로 나가므로 그 선은 가로로
  // 시작해야 하고, 그 사실은 상자를 어디로 옮기든 변하지 않는다.
  if (axis === 'lr') {
    const mid = a.x + dx / 2;
    return [
      { x: mid, y: a.y },
      { x: mid, y: b.y },
    ];
  }
  if (axis === 'tb') {
    const mid = a.y + dy / 2;
    return [
      { x: a.x, y: mid },
      { x: b.x, y: mid },
    ];
  }
  if (axis === 'lr-tb') return [{ x: b.x, y: a.y }];
  if (axis === 'tb-lr') return [{ x: a.x, y: b.y }];
  if (Math.abs(dx) >= Math.abs(dy)) {
    const mid = a.x + dx / 2;
    return [
      { x: mid, y: a.y },
      { x: mid, y: b.y },
    ];
  }
  const mid = a.y + dy / 2;
  return [
    { x: a.x, y: mid },
    { x: b.x, y: mid },
  ];
}

/**
 * 가운데를 어떻게 꺾을 것인가 — **앵커가 나가는 쪽**에서 온다 (SPEC-CANVAS-018).
 *
 * `lr` 은 가로-세로-가로, `tb` 는 세로-가로-세로, 섞이면 한 번만 꺾는다. 다리가 없으면
 * 부재이고, 그때 015 의 `|dx|` vs `|dy|` 규칙이 그대로 선다(자유 끝이 그 자리다).
 */
type OrthoAxis = 'lr' | 'tb' | 'lr-tb' | 'tb-lr';

/** 다리의 방향에서 축을 읽는다. 다리가 가로면 그 끝의 선도 가로로 시작한다. */
function cornerAxis(
  from: PxPoint,
  exitA: PxPoint | undefined,
  to: PxPoint,
  exitB: PxPoint | undefined,
): OrthoAxis | undefined {
  const a = exitA === undefined ? undefined : exitA.y === from.y ? 'lr' : 'tb';
  const b = exitB === undefined ? undefined : exitB.y === to.y ? 'lr' : 'tb';
  if (a === undefined || b === undefined) return a ?? b;
  if (a === b) return a;
  return a === 'lr' ? 'lr-tb' : 'tb-lr';
}

/**
 * 고정된 가운데 자리로 꺾는다 (SPEC-CANVAS-019 REQ-03).
 *
 * `orthoCorners` 의 가운데 갈래와 **같은 모양**이고 가운데 값만 사용자가 고른 것이다 —
 * 두 함수를 갈라 두면 자동일 때와 고정일 때의 모양이 갈라진다.
 */
function orthoSplitCorners(a: PxPoint, b: PxPoint, axis: 'lr' | 'tb', at: number): PxPoint[] {
  if (axis === 'lr') {
    return [
      { x: at, y: a.y },
      { x: at, y: b.y },
    ];
  }
  return [
    { x: a.x, y: at },
    { x: b.x, y: at },
  ];
}

/** 아무것도 피하지 않는 자리 — 015 의 길로 간다. */
const EMPTY_ROUTING: ConnectorRouting = { obstacles: [], hosts: {} };

/**
 * 그려진 점과, 점마다 **그것을 낳은 논리 구간의 번호** (SPEC-CANVAS-020 §결정 2).
 *
 * 두 목록은 언제나 같은 길이다. 나란함을 타입으로 묶어 두는 것에 뜻이 있다 — 따로 돌려
 * 주면 한쪽만 접거나 한쪽만 거르는 날이 온다.
 */
interface OwnedPath {
  readonly points: readonly PxPoint[];
  readonly owner: readonly number[];
  /**
   * 논리 구간마다 그린 폴리라인 — **접기 전**의 것이며 양 끝을 포함한다.
   *
   * 접힌 목록에서 구간을 되찾을 수는 없다: 접기는 한 직선 위의 가운데 점을 지우고, 지워진
   * 것이 논리 꼭짓점이면 그려진 구간 하나가 **두 논리 구간에 걸친다**. 그 구간에 손잡이를
   * 세우면 어느 구간의 고정값을 건드리는지 말할 수 없다(021 REQ-04 가 답해야 하는 그것).
   */
  readonly runs: readonly (readonly PxPoint[])[];
}

/**
 * 점 하나가 명령 하나인 갈래의 주인 목록.
 *
 * `i - 1` 인 것에 뜻이 있다 — 점 `i` 에 **닿는** 명령은 구간 `i-1`(`px[i-1] → px[i]`)을
 * 그린다. 첫 점은 `M` 이라 어느 명령의 도착점도 아니지만, 목록의 길이를 점과 맞춰 두면
 * `slice(1)` 한 번으로 명령과 나란해진다 — 직각 갈래가 쓰는 그 셈과 같다.
 */
function identity(px: readonly PxPoint[]): number[] {
  return px.map((_point, i) => Math.max(i - 1, 0));
}

/** 점 하나가 명령 하나인 갈래의 구간별 경로 — 이웃한 두 점이 곧 한 구간이다. */
function pairs(px: readonly PxPoint[]): PxPoint[][] {
  const out: PxPoint[][] = [];
  for (let i = 0; i + 1 < px.length; i += 1) {
    const a = px[i];
    const b = px[i + 1];
    if (a !== undefined && b !== undefined) out.push([a, b]);
  }
  return out;
}

const EMPTY_DRAWN: ConnectorDrawn = { commands: [], owner: [], runs: [] };

/**
 * 직각 경로 하나를 편다 — **다리 · 길 · 다리** (SPEC-CANVAS-018 §결정 2).
 *
 * `앵커 → 다리끝 → …길… → 다리끝 → 앵커`. 가운데 길은 **모든 도형**을 피하므로 끝 도형의
 * 변에 붙지 않는다(REQ-01). 017 이 세운 격자와 값은 한 글자도 바뀌지 않았다 — 바뀐 것은
 * 무엇을 장애물로 보는가와 **어디서 어디로 찾는가** 둘뿐이다.
 *
 * 다리는 **양 끝의 바깥쪽**에만 난다. 사용자가 찍은 중간점 사이에는 나갈 도형이 없다.
 */
function orthoPath(
  px: readonly PxPoint[],
  routing: ConnectorRouting,
  splits: readonly OrthoSplit[] | undefined,
  proj: CanvasProjection,
): OwnedPath {
  /**
   * 구간 하나의 고정값을 px 로 옮긴다 (SPEC-CANVAS-021).
   *
   * 저장은 **캔버스 단위**이고 여기는 px 다. 축을 아는 자리에서 한 번만 옮긴다 — 축마다
   * 배율이 다를 수 있으므로(형상상 같지만 — 014 A1) 좌표를 통째로 투영해 고른다.
   */
  const pinOf = (index: number): PxPoint | undefined => {
    const at = splitAt(splits, index);
    return at === undefined ? undefined : projectPoint({ x: at, y: at }, proj);
  };
  const out: PxPoint[] = [];
  // **점마다 그것을 낳은 논리 구간의 번호**(SPEC-CANVAS-020 §결정 2). 같은 순환에서 함께
  // 실으므로 새로 계산할 것이 없다 — 따로 계산하면 그 둘이 갈리는 날 아무도 울지 않는다.
  const owner: number[] = [];
  let owns = 0;
  const push = (p: PxPoint): void => {
    const last = out.at(-1);
    // 같은 자리를 두 번 싣지 않는다 — 길이 0 인 구간은 잡는 쪽의 거리 산술에서 0 으로
    // 나누는 자리가 된다(015 K2).
    if (last === undefined || last.x !== p.x || last.y !== p.y) {
      out.push(p);
      owner.push(owns);
    }
  };

  px.forEach((point, i) => {
    // 구간 `i-1` 은 `px[i-1] → px[i]` 다. 첫 점(`i === 0`)은 첫 구간의 시작이므로 0 이다.
    owns = Math.max(i - 1, 0);
    const prev = px[i - 1];
    if (prev === undefined) {
      push(point);
      return;
    }
    // 첫 구간의 시작과 마지막 구간의 끝에만 다리가 난다.
    const fromHost = i === 1 ? routing.hosts.from : undefined;
    const toHost = i === px.length - 1 ? routing.hosts.to : undefined;
    const exitA = fromHost !== undefined ? orthoStub(prev, fromHost, point) : undefined;
    const exitB = toHost !== undefined ? orthoStub(point, toHost, prev) : undefined;
    const a = exitA ?? prev;
    const b = exitB ?? point;

    if (exitA !== undefined) push(exitA);
    // **015 의 가운데 꺾기를 먼저 묻는다**(018 §안정한 모양). 그 길이 비어 있으면 그것을
    // 쓴다 — 두 끝에서 대칭이라 한쪽을 옮겨도 가운데 자리가 그대로이기 때문이다(REQ-04).
    //
    // 라우터는 **막혔을 때만** 돈다. 먼저 묻지 않으면, 맨해튼 거리에서 같은 값인 여러
    // 계단 가운데 아무것이나 골라 상자를 조금 옮길 때마다 모양이 통째로 바뀐다.
    const axis = cornerAxis(prev, exitA, point, exitB);
    // **사용자가 고른 자리가 있으면 그것을 쓴다**(019 REQ-03 · §결정 2). 라우터를 돌리지
    // 않는 것이 요점이다 — 거기서 다시 피해 돌면 옮긴 자리가 지켜지지 않는다.
    //
    // **구간마다 제 고정값을 읽는다**(021). 019 는 `px.length === 2`(논리 구간이 하나)일
    // 때만 고정을 허용했다 — 수가 하나뿐이라 어느 구간의 것인지 말할 수 없었기 때문이다.
    // 목록이 되면서 그 제약이 사라진다: 자리가 곧 구간 번호다.
    const projected = pinOf(owns);
    const pinned =
      projected !== undefined && (axis === 'lr' || axis === 'tb')
        ? orthoSplitCorners(a, b, axis, axis === 'lr' ? projected.x : projected.y)
        : undefined;
    const midway = pinned ?? orthoCorners(a, b, axis);
    const plain = [a, ...midway, b];
    const routed =
      pinned !== undefined || orthoPathClear(plain, routing.obstacles)
        ? undefined
        : orthoRoute(a, b, routing.obstacles);
    // 되돌아온 목록은 두 끝을 **포함한다** — 양 끝은 아래에서 따로 싣는다.
    if (routed !== undefined) for (const p of routed.slice(1, -1)) push(p);
    else for (const p of midway) push(p);
    if (exitB !== undefined) push(exitB);
    push(point);
  });
  return collapseCollinear(out, owner);
}

/**
 * 한 직선 위에 늘어선 점을 **접는다**.
 *
 * 다리와 격자는 같은 축 위의 점을 여럿 낸다 — 그림은 같지만 그 점들은 **잡는 구간**을
 * 쪼개고, 그러면 선 위의 더블클릭이 "몇 번째 조각인가" 를 다르게 답한다. 접어 두면
 * 피할 것이 없을 때 015 와 **바이트 동일**한 목록이 되어, 017 이전의 그림이 그대로임을
 * 값으로 보일 수 있다(K3).
 */
/** 세 점이 한 축 위에 나란한가 — 접기의 판정 하나. */
function inLine(prev: PxPoint, last: PxPoint, next: PxPoint): boolean {
  return (prev.x === last.x && last.x === next.x) || (prev.y === last.y && last.y === next.y);
}

/**
 * 주인이 같은 점끼리 묶는다 — 앞 묶음의 끝점을 시작으로 이어 붙여 **양 끝을 포함**한다.
 *
 * 묶음마다 **그 안에서** 접는다. 다리(`orthoStub`)와 격자가 같은 축 위에 점을 여럿 내므로,
 * 접지 않으면 Z 하나가 점 여섯으로 보이고 "구간이 Z 인가" 를 묻는 쪽이 답을 얻지 못한다.
 *
 * 묶음 **안에서만** 접는 것이 요점이다. 전체를 접으면 논리 꼭짓점이 삼켜져 한 구간이 두
 * 논리 구간에 걸치고, 그러면 그 구간의 손잡이가 어느 고정값을 건드리는지 말할 수 없다.
 */
function runsOf(points: readonly PxPoint[], owner: readonly number[]): PxPoint[][] {
  const out: PxPoint[][] = [];
  points.forEach((point, i) => {
    const own = owner[i] ?? 0;
    while (out.length <= own) {
      // 새 묶음은 **앞 묶음의 마지막 점**에서 시작한다 — 구간은 꼭짓점을 나눠 갖는다.
      const prev = out.at(-1)?.at(-1);
      out.push(prev === undefined ? [] : [prev]);
    }
    const run = out[own];
    if (run === undefined) return;
    const last = run.at(-1);
    const prev = run.at(-2);
    if (last !== undefined && prev !== undefined && inLine(prev, last, point)) {
      run[run.length - 1] = point;
      return;
    }
    run.push(point);
  });
  return out;
}

function collapseCollinear(points: readonly PxPoint[], owner: readonly number[]): OwnedPath {
  const out: PxPoint[] = [];
  const kept: number[] = [];
  points.forEach((point, i) => {
    const last = out.at(-1);
    const prev = out.at(-2);
    const owns = owner[i] ?? 0;
    if (last !== undefined && prev !== undefined && inLine(prev, last, point)) {
      // **삼킨 점의 주인은 버리고 살아남은 점의 주인을 쓴다**(020 §결정 3). 삼켜진 것이
      // 논리 꼭짓점이었다면 그 자리를 누른 누름은 한 자리 늦은 번호를 받는다 — 두 점이
      // 한 직선 위에 있으므로 그려지는 그림은 사실상 같다. 접기를 멈추면 017 K3(바이트
      // 동일)이 깨지므로 그림이 같은 쪽을 고르고, 그 한계를 SPEC 에 적어 두었다.
      out[out.length - 1] = point;
      kept[kept.length - 1] = owns;
      return;
    }
    out.push(point);
    kept.push(owns);
  });
  return { points: out, owner: kept, runs: runsOf(points, owner) };
}

/**
 * 그려진 명령과, 명령마다 **그것을 낳은 논리 구간의 번호** (SPEC-CANVAS-020 §결정 1).
 *
 * `owner` 는 `commands.slice(1)` 과 나란하다 — 명령 `k` 는 `path[k-1] → path[k]` 를
 * 그리므로 `path[k]` 의 주인이 그 명령의 주인이다.
 */
export interface ConnectorDrawn {
  readonly commands: readonly ConnectorPathCommand[];
  readonly owner: readonly number[];
  /** 논리 구간마다 그린 폴리라인 — 접기 전, 양 끝 포함 (021 REQ-04 가 읽는다). */
  readonly runs: readonly (readonly PxPoint[])[];
}

/**
 * 그리는 쪽과 **자리 번호가 같은 함수를 지나게 하는** 한 자리 (020 REQ-03 · K4).
 *
 * 그리는 쪽은 `connectorPath` 로 명령만 받아 가고, 점을 찍는 쪽은 여기서 주인까지 받아
 * 간다. 나란한 두 함수로 나누면 그 둘이 갈리는 날 아무도 울지 않는다 — 017 이 장애물
 * 목록에 대해 세운 K4 와 같은 문장이다.
 */
export function connectorDrawn(
  points: readonly CanvasPoint[],
  route: ConnectorRoute,
  proj: CanvasProjection,
  routing: ConnectorRouting = EMPTY_ROUTING,
  splits?: readonly OrthoSplit[],
): ConnectorDrawn {
  const px = points.map((point) => projectPoint(point, proj));
  const start = px[0];
  // `resolveConnector` 는 늘 점을 둘 이상 내지만 그 사실은 타입에 없다. 손으로 지은 목록이
  // 들어와도 던지지 않는 쪽을 고른다 — 빈 목록에서 `M` 을 지어내면 없는 자리에 선이 선다.
  if (start === undefined) return EMPTY_DRAWN;

  const out: ConnectorPathCommand[] = [{ c: 'M', x: start.x, y: start.y }];
  // **직각은 점 사이에 모서리를 끼운다**(SPEC-CANVAS-015 REQ-01). 곡선과 달리 갈래를 따로
  // 세우지 않고 **점 목록을 넓혀** 같은 길로 보낸다 — 아래 폴리라인 갈래가 그대로 쓰이므로
  // 그리는 쪽도 잡는 쪽도 새 어휘를 배우지 않는다(§결정 2 · K3).
  // **장애물이 있으면 그것을 피해 돈다**(SPEC-CANVAS-017 REQ-01). 길을 찾지 못하거나
  // 피할 것이 없으면 015 의 세 구간으로 떨어진다 — **선이 사라지는 갈래는 없다**(K5).
  //
  // 장애물은 **px 로 받는다.** 투영이 각도를 보존하고 축척이 하나이므로(014 A1) 캔버스
  // 단위에서 돌리든 px 에서 돌리든 같은 길이지만, 여기서 점이 이미 px 이라 그 자리에서
  // 재는 것이 환산 하나를 덜 지난다.
  //
  // **주인 목록은 직각에서만 따로 난다**(020 REQ-04 · K3). 다른 갈래는 점 하나가 명령
  // 하나이므로 주인이 곧 차례이고, 그 항등이 "다른 갈래는 한 글자도 바뀌지 않는다" 를
  // 코드 한 줄 없이 지킨다.
  const drawn: OwnedPath =
    route === 'ortho' ? orthoPath(px, routing, splits, proj) : { points: px, owner: identity(px), runs: pairs(px) };
  const segments = route === 'curve' ? curveSegments(px) : [];
  if (segments.length === 0) {
    for (const point of drawn.points.slice(1)) out.push({ c: 'L', x: point.x, y: point.y });
    return { commands: out, owner: drawn.owner.slice(1), runs: drawn.runs };
  }
  for (const seg of segments) {
    out.push({
      c: 'C',
      x1: seg.c1.x,
      y1: seg.c1.y,
      x2: seg.c2.x,
      y2: seg.c2.y,
      x: seg.to.x,
      y: seg.to.y,
    });
  }
  // 곡선은 구간 하나가 명령 하나다 — 차례가 곧 주인이다.
  return { commands: out, owner: segments.map((_seg, i) => i), runs: drawn.runs };
}

/**
 * 그려진 명령만. `connectorDrawn` 의 얇은 껍데기이며 **출력은 019 와 바이트 동일**하다
 * (020 REQ-05 · K2).
 */
export function connectorPath(
  points: readonly CanvasPoint[],
  route: ConnectorRoute,
  proj: CanvasProjection,
  routing: ConnectorRouting = EMPTY_ROUTING,
  splits?: readonly OrthoSplit[],
): readonly ConnectorPathCommand[] {
  return connectorDrawn(points, route, proj, routing, splits).commands;
}
