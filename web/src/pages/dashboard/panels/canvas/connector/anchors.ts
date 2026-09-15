// 고정 앵커 아홉 — 선이 붙는 자리 (SPEC-CANVAS-011 M3).
//
// 자리는 윤곽 상자의 네 모서리 · 네 변 가운데 · **중심** 아홉이다. 앞의 여덟은 8핸들의
// 이름을 **그대로** 쓰고, 011 이 새로 짓는 이름은 중심(`c`) 하나뿐이다(AC-11).
//
// ## 고정 아홉은 저장하지 않는다 (A1 · AC-14)
//
// 아홉은 전부 `outlineBox` 에서 **파생**한다. 저장하면 도형을 늘리는 순간 앵커가 옛 자리에
// 남고, 그 어긋남은 예외도 경고도 없이 **화면에서만** 드러난다. 파생시키면 그 상태가
// 표현 불가능하다.
//
// M3' 가 더한 `addAnchorAt` · `removeAnchor` 는 그 불변식을 깨지 않는다 — 둘이 만지는
// 필드는 `anchors` 하나뿐이고, 아홉의 이름도 비율도 거기 닿지 않는다. **저장되는 것은
// 사용자가 고른 자리이고(A10), 파생되는 것은 여전히 아홉이다.** 그 둘을 한 파일에 두는
// 까닭은 임의 앵커의 로컬 격자가 고정 아홉이 쓰는 **그 상자** 위에 서기 때문이다. 갈라
// 두면 상자를 두 벌 재게 되고, 그것이 AC-33 이 금지한 바로 그것이다.
//
// ## 산술을 새로 적지 않는다 (AC-10)
//
// 여덟 자리의 비율은 `canvasEditGeometry` 의 `BOX_HANDLE_FACTORS` 를 **들여와서** 쓴다.
// 베껴 적으면 두 표가 생기고, 그중 하나가 바뀌는 날 "손잡이가 선 자리와 선이 붙는 자리가
// 다르다" 가 시작된다 — 002 가 위험 R1 로 이름 적어 둔 그 부류이며, 화면에서만 보인다.
//
// ## 상자를 **한 번만** 잰다 (AC-33)
//
// `outlineBox` 호출이 이 파일에 하나뿐이다. 두 번 재면 M3' 의 임의 앵커가 고정 아홉과
// 다른 상자에서 나올 수 있고, 그 갈라짐은 **크기를 바꾼 뒤에야** 보인다.
//
// ## 종류별 갈래가 없다 (A2)
//
// `switch (node.kind)` 가 이 파일에 없다. 선도 문구도 아홉을 낸다 — 그 둘의 상자 갈래는
// 이미 `outlineBox` 안에 있으므로 여기서 다시 가를 일이 없고, 가르기 시작하면 "이 도형만
// 붙는 자리가 다르다" 가 시작된다. 별의 진짜 꼭지점이 필요한 손에게는 임의 앵커(M3')가
// 있다.
//
// ## 부품은 앵커를 내지 않는다 (A3)
//
// 이 함수의 인자는 **최상위 노드**다. 부품은 그룹 로컬 격자에 살고 최상위 순회에 나오지
// 않으므로 여기 닿을 길이 없으며, 그룹은 부품이 몇이든 **제 상자**로 아홉을 낸다
// (`outlineBox` 의 `group` 갈래가 그것을 이미 정했다). 그래서 런타임 부품 검사를 두지
// 않는다 — 없는 입력을 막는 검사는 죽은 코드이고, 죽은 코드는 다음 사람에게 "부품이 여기
// 올 수 있다" 고 거짓말한다.
//
// ## 좌표는 **캔버스 단위**다
//
// 이 코드베이스는 캔버스 단위로 저술하고 그리는 자리에서만 투영한다. 앵커도 그 규율을
// 따른다 — 연결선의 자유 끝점이 캔버스 단위 정수이므로(M4), 붙은 끝점이 px 면 한 자료형
// 안에 두 공간이 섞인다. 상자는 px 로 나오므로 아홉 자리를 px 에서 셈한 뒤 `unprojectPoint`
// 로 되돌린다. 되돌림은 역투영 **한 자리**를 지나므로 8핸들과 비트 단위로 같은 값이 나온다
// (AC-10). 다만 그 값을 다시 투영해 오는 왕복은 마지막 비트가 갈릴 수 있으며, 그 사실을
// 시험이 감추지 않는다.
//
// **정수화하지 않는다.** `unprojectPoint` 가 적어 둔 대로 정수화는 쓰는 쪽의 몫이고,
// 이 값들은 아무도 쓰지 않는다(파생이지 저장이 아니다 — A1).
//
// **이 모듈은 DOM 을 모른다.** 투영과 글자 폭 장부만 받는다.
//
// @spec SPEC-CANVAS-011 REQ-02

import { isBoxGeometry, type PointGeometry } from '../canvasConfig';
import {
  unprojectBox,
  unprojectPoint,
  type CanvasBox,
  type CanvasPoint,
  type CanvasProjection,
} from '../canvasGeometry';
import { BOX_HANDLE_FACTORS, BOX_HANDLE_IDS } from '../canvasEditGeometry';
import { outlineBox } from '../canvasOutline';
import { toAbsolutePointExact, toLocalPoint, widenDegenerateBox } from '../group/groupCoords';
import type { CanvasNode } from '../group/groupTypes';
import { ANCHOR_SNAP_LOCAL, type CustomAnchor } from './anchorTypes';

/** 중심 앵커의 이름. 011 이 새로 짓는 자리 이름은 **이 하나뿐**이다(AC-11). */
export const ANCHOR_CENTER_ID = 'c';

/**
 * 고정 아홉의 이름. 여덟은 `BOX_HANDLE_IDS` 에서 **파생**시키고 중심 하나를 더한다 —
 * 아홉을 손으로 적으면 8핸들의 이름이 바뀌는 날 두 목록이 조용히 갈라진다.
 */
export type FixedAnchorId = (typeof BOX_HANDLE_IDS)[number] | typeof ANCHOR_CENTER_ID;

/** 아홉의 차례. 여덟은 8핸들의 순서(북서 → 서) 그대로이고 중심이 맨 뒤에 온다. */
export const FIXED_ANCHOR_IDS: readonly FixedAnchorId[] = [...BOX_HANDLE_IDS, ANCHOR_CENTER_ID];

/**
 * 중심의 비율. `BOX_HANDLE_FACTORS` 의 값과 **같은 형상**으로 적는다 — 아홉째 줄이지
 * 다른 종류의 값이 아니다.
 */
const CENTER_FACTOR: readonly [number, number] = [0.5, 0.5];

/**
 * 앵커 한 자리의 이름 — 고정 아홉이거나 임의 앵커의 id 다.
 *
 * 합집합이 타입으로는 `string` 으로 접히지만, 이름을 두는 것이 뜻을 남긴다: 연결선의
 * 끝점이 가리키는 것은 **자리 이름**이지 아무 문자열이 아니다(M4 의 `ConnectorEnd`).
 */
export type AnchorId = FixedAnchorId | string;

/**
 * 노드의 앵커 자리를 **캔버스 단위**로 낸다 — 고정 아홉 + 임의 앵커.
 *
 * 차례는 `FIXED_ANCHOR_IDS` 뒤에 저장된 임의 앵커 순서다 — `Map` 이 삽입 순서를 지키므로
 * 순회하는 쪽이 제 나름의 정렬을 두지 않아도 된다.
 *
 * ## 상자는 여전히 **한 번만** 잰다 (AC-33)
 *
 * 임의 앵커의 로컬 → 캔버스 환산에 상자가 필요하지만, 그 상자는 고정 아홉이 쓴 **그
 * 상자**를 역투영한 것이다. 다시 재면 두 벌이 갈라질 수 있고 그 갈라짐은 크기를 바꾼
 * 뒤에야 보인다.
 *
 * ## 퇴화 상자를 여기서 넓히지 **않는다**
 *
 * 폭이나 높이가 0 인 상자에서 `toAbsolutePointExact` 는 곱셈만 지나므로 NaN 을 내지 않고 모든
 * 임의 앵커를 상자 모서리로 접는다 — 그런데 고정 아홉도 같은 상자에서 **똑같이** 접힌다.
 * 여기서만 넓히면 퇴화한 도형에서 임의 앵커만 윤곽 밖으로 벌어져 나가, 보이는 상자와 선이
 * 붙는 자리가 갈린다(위험 R1). 넓히기가 필요한 쪽은 나누기를 지나는 **쓰는 경로**뿐이다.
 *
 * ## 이름이 부딪히면 고정이 이긴다
 *
 * 임의 앵커의 id 는 사용자·도구가 짓는 문자열이라 고정 아홉의 이름과 같을 수 있다. 그때
 * 고정을 덮어쓰면 아홉 중 하나가 조용히 다른 자리를 가리키고, 그 어긋남은 이미 그 이름에
 * 붙어 있던 연결선에서만 드러난다. 먼저 온 것이 이기는 파서 규율과 같은 방향이다.
 */
export function anchorPoints(
  node: CanvasNode,
  proj: CanvasProjection,
  textWidths: Readonly<Record<string, number>>,
): Map<AnchorId, CanvasPoint> {
  // 상자는 여기서 **한 번** 나온다(AC-33).
  const box = outlineBox(node, proj, textWidths);
  const at = (fx: number, fy: number): CanvasPoint =>
    unprojectPoint({ x: box.x + box.w * fx, y: box.y + box.h * fy }, proj);

  const points = new Map<AnchorId, CanvasPoint>();
  for (const id of BOX_HANDLE_IDS) {
    const [fx, fy] = BOX_HANDLE_FACTORS[id];
    points.set(id, at(fx, fy));
  }
  points.set(ANCHOR_CENTER_ID, at(CENTER_FACTOR[0], CENTER_FACTOR[1]));

  const custom = node.anchors;
  if (custom !== undefined && custom.length > 0) {
    // 같은 상자를 캔버스 단위로 되돌린 것이다 — 두 번째 측정이 아니다.
    const canvasBox = unprojectBox(box, proj);
    for (const anchor of custom) {
      if (points.has(anchor.id)) continue;
      points.set(anchor.id, toAbsolutePointExact({ x: anchor.x, y: anchor.y }, canvasBox));
    }
  }
  return points;
}

// --- 임의 앵커를 더하고 뺀다 (M3' · A10) ------------------------------------
//
// 고정 아홉에 쓰기 경로가 없다는 A1 은 그대로다 — 아래 둘이 만지는 필드는 `anchors` 하나
// 뿐이고, 아홉의 이름도 비율도 여기 닿지 않는다. 저장되는 것은 **사용자가 고른 자리**이고
// (A10), 파생되는 것은 여전히 아홉이다.
//
// 둘 다 **순수 함수**다. 새 노드를 내고 받은 노드를 건드리지 않는다 — 도구(M3'b)가 무엇을
// 하든 이 산술은 화면도 상태도 모른다.

/** 상자형 노드의 캔버스 단위 상자. 상자형이 아니면 부재다(A11). */
function anchorBox(node: CanvasNode): CanvasBox | undefined {
  const geo = node.geometry;
  if (!isBoxGeometry(geo)) return undefined;
  return { x: geo.x, y: geo.y, w: geo.w, h: geo.h };
}

/**
 * 경로 요소면 허용 오차 안의 가장 가까운 **경로 명령 좌표**로 당긴다(REQ-02'-b · AC-21).
 *
 * 경로 명령과 임의 앵커가 **같은 격자에 살기 때문에** 환산 없이 견줄 수 있고, 그래서
 * 당겨진 좌표가 명령의 좌표와 **정확히** 같다. 환산이 한 번이라도 끼면 반올림이 그
 * "정확히" 를 표현 불가능하게 만든다.
 *
 * 제어점(`C` 의 `x1·y1 · x2·y2`)은 보지 않는다 — 곡선 위의 점이 아니므로 거기 붙이면
 * 앵커가 그려지는 윤곽에서 떨어져 앉는다. `Z` 는 좌표가 없어 저절로 빠진다.
 */
function snapToPathVertex(node: CanvasNode, local: PointGeometry): PointGeometry {
  if (!('path' in node)) return local;
  const limit = ANCHOR_SNAP_LOCAL * ANCHOR_SNAP_LOCAL;
  let best = local;
  let bestDistance = limit;
  for (const cmd of node.path) {
    if (!('x' in cmd)) continue;
    const dx = cmd.x - local.x;
    const dy = cmd.y - local.y;
    const distance = dx * dx + dy * dy;
    if (distance <= bestDistance) {
      bestDistance = distance;
      best = { x: cmd.x, y: cmd.y };
    }
  }
  return best;
}

/**
 * 캔버스 단위 자리 하나에 임의 앵커를 더한 **새 노드**. 설 수 없으면 받은 노드 그대로다.
 *
 * 아무 일도 하지 않는 경우 셋:
 *   1. 상자형이 아니다(선·문구 — A11 · AC-24).
 *   2. `id` 가 정체성이 없다(빈 문자열).
 *   3. 그 `id` 가 이미 있다 — **먼저 온 것이 이긴다**(파서와 같은 규율).
 *
 * 퇴화한 상자는 `widenDegenerateBox` 로 먼저 편다. `toLocalPoint` 는 상자변으로 **나누는**
 * 쪽이라 0 인 축에서 모든 자리가 한 점으로 내려앉고, 그 결함은 예외 없이 저장 왕복을
 * 견디며 화면으로만 드러난다(004 가 같은 함정에서 같은 결론에 도달했다).
 */
export function addAnchorAt<T extends CanvasNode>(node: T, at: CanvasPoint, id: string): T {
  const box = anchorBox(node);
  if (box === undefined) return node;
  if (id.trim() === '') return node;

  const existing = node.anchors ?? [];
  if (existing.some((anchor) => anchor.id === id)) return node;

  const local = toLocalPoint({ x: at.x, y: at.y }, widenDegenerateBox(box));
  const snapped = snapToPathVertex(node, local);
  const anchor: CustomAnchor = { id, x: snapped.x, y: snapped.y };
  // 제네릭 노드에 필드 하나를 얹는 자리다. `anchors` 는 모든 갈래에서 옵셔널이므로
  // 얹은 결과는 여전히 `T` 이며, 다른 필드는 펼침이 그대로 옮긴다.
  return { ...node, anchors: [...existing, anchor] } as T;
}

/**
 * 임의 앵커 하나를 뺀 **새 노드**. 없는 id 면 받은 노드 그대로다.
 *
 * 마지막 하나를 빼면 **키를 지운다.** `[]` 를 남기면 "쓴 적 없음" 과 "다 지웠음" 이
 * 구분되지 않고, 저장 왕복에 없던 `anchors` 키가 생긴다(AC-25). 파서가 빈 목록을 부재로
 * 떨어뜨리는 것과 같은 규율이며, 두 규율이 갈리면 화면에서는 보이지 않는 차이가 config
 * 에만 남는다.
 */
export function removeAnchor<T extends CanvasNode>(node: T, id: string): T {
  const existing = node.anchors;
  if (existing === undefined) return node;

  const kept = existing.filter((anchor) => anchor.id !== id);
  if (kept.length === existing.length) return node;
  if (kept.length === 0) {
    const { anchors: _dropped, ...rest } = node;
    return rest as T;
  }
  return { ...node, anchors: kept } as T;
}
