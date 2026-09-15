// 고정 앵커 아홉 — 선이 붙는 자리 (SPEC-CANVAS-011 M3).
//
// 자리는 윤곽 상자의 네 모서리 · 네 변 가운데 · **중심** 아홉이다. 앞의 여덟은 8핸들의
// 이름을 **그대로** 쓰고, 011 이 새로 짓는 이름은 중심(`c`) 하나뿐이다(AC-11).
//
// ## 저장하지 않는다 (A1 · AC-14)
//
// 아홉은 전부 `outlineBox` 에서 **파생**한다. 저장하면 도형을 늘리는 순간 앵커가 옛 자리에
// 남고, 그 어긋남은 예외도 경고도 없이 **화면에서만** 드러난다. 파생시키면 그 상태가
// 표현 불가능하다 — 이 모듈에 쓰기 경로가 한 줄도 없는 것이 그 불변식의 전부다.
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

import { unprojectPoint, type CanvasPoint, type CanvasProjection } from '../canvasGeometry';
import { BOX_HANDLE_FACTORS, BOX_HANDLE_IDS } from '../canvasEditGeometry';
import { outlineBox } from '../canvasOutline';
import type { CanvasNode } from '../group/groupTypes';

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
 * 노드의 고정 앵커 아홉을 **캔버스 단위**로 낸다.
 *
 * 차례는 `FIXED_ANCHOR_IDS` 그대로다 — `Map` 이 삽입 순서를 지키므로 순회하는 쪽이
 * 제 나름의 정렬을 두지 않아도 된다.
 */
export function anchorPoints(
  node: CanvasNode,
  proj: CanvasProjection,
  textWidths: Readonly<Record<string, number>>,
): Map<FixedAnchorId, CanvasPoint> {
  // 상자는 여기서 **한 번** 나온다(AC-33).
  const box = outlineBox(node, proj, textWidths);
  const at = (fx: number, fy: number): CanvasPoint =>
    unprojectPoint({ x: box.x + box.w * fx, y: box.y + box.h * fy }, proj);

  const points = new Map<FixedAnchorId, CanvasPoint>();
  for (const id of BOX_HANDLE_IDS) {
    const [fx, fy] = BOX_HANDLE_FACTORS[id];
    points.set(id, at(fx, fy));
  }
  points.set(ANCHOR_CENTER_ID, at(CENTER_FACTOR[0], CENTER_FACTOR[1]));
  return points;
}
