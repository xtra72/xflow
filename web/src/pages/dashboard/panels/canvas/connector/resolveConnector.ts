// 끝점 해석 — 연결선의 두 끝이 **어디인가**를 내는 한 함수 (SPEC-CANVAS-011 M5).
//
// ## 왜 한 함수인가
//
// 그리는 쪽(M6) · 잡는 쪽(M7) · 손잡이를 세우는 쪽(M9)이 전부 이 하나를 지난다. 셋이
// 저마다 참조를 풀면 "그려진 자리와 잡히는 자리가 다르다" 가 시작된다 — 002 가 위험 R1
// 으로 이름 적어 둔 그 결함이며, 예외도 경고도 없이 **화면에서만** 드러난다.
//
// ## `undefined` 는 **끊긴 연결**이다 (REQ-08)
//
// 빈 배열도 반쪽 목록도 내지 않는다. 반쪽 목록은 소비자가 **그릴 수 있는 형상**이라
// "한쪽 끝이 어딘가에 붙은 선" 이 화면에 남고, 그것은 사용자가 긋지 않은 선이다.
// 부재는 그릴 수 없으므로 세 소비자가 모두 아무것도 하지 않게 된다.
//
// ## 복합 키는 **찾지 못해서** 풀리지 않는다 (A3 · AC-44)
//
// `grp-1/body` 같은 부품 키를 여기서 **쪼개지 않는다.** 최상위 목록에 그 id 를 가진 노드가
// 없으므로 아래 조회에서 저절로 떨어진다. 이것은 게으름이 아니라 **선택**이다:
//
//   - 구분자 리터럴(`'/'`)이 적히는 자리는 `group/frameKey.ts` 하나뿐이어야 한다
//     (009 AC-04 이 소스에서 그것을 붙들고 있다).
//   - 부품 검사를 명시로 두면 "부품은 여기 올 수 있다" 는 거짓말이 코드에 남고, 그 다음
//     사람은 부품을 **받아 주는** 갈래를 자연스럽게 더한다.
//
// 그러니 이 파일에 부품 판별을 더하지 말 것. 막는 것은 조회이지 검사가 아니다.
//
// ## 연결선끼리는 **타입으로** 이어지지 않는다 (A6 · REQ-03-b)
//
// `anchorPoints` 의 인자가 `OutlinedNode` 이므로 연결선을 넘기는 것이 컴파일되지 않는다.
// 아래 조회가 `isConnector` 로 좁히는 한 줄이 그 사실의 런타임 짝이며, 그래서 고리를 막는
// 검사가 해석 · 그리기 · 히트 셋에 각각 필요하지 않다.
//
// ## 없는 앵커 이름도 **끊긴 연결**이다 (REQ-02'-e)
//
// 요소는 있는데 그 이름의 자리가 없는 경우(임의 앵커를 뺐다), 중심이나 아무 고정 앵커로
// 떨어뜨리지 **않는다.** 떨어뜨리면 사용자가 그어 둔 선이 예고 없이 다른 자리로 옮겨
// 앉고, 그 이동은 되돌릴 손잡이가 없다. 끊겼다고 말하면 사용자가 다시 붙일 수 있다.
//
// ## 상자를 노드마다 **한 번만** 잰다 (AC-33)
//
// `anchorPoints` 호출이 이 파일에 하나뿐이고, 그 앞의 장부가 같은 노드를 두 번 재지 않게
// 한다 — 두 끝이 같은 요소를 가리킬 때가 그 자리다. 장부는 **재는 횟수를 줄일 뿐** 새로
// 재는 자리를 만들지 않는다. 이 파일에 `outlineBox` 가 없는 것이 그 사실이다.
//
// **좌표는 캔버스 단위다.** `anchorPoints` 와 자유 끝점(M4)이 이미 그 공간에 있으므로
// 여기서 공간이 섞이지 않는다. 투영은 앵커 자리를 얻는 데만 쓰인다.
//
// **이 모듈은 DOM 도 React 도 모른다.** 노드 목록과 투영과 글자 폭 장부만 받는다.
//
// @spec SPEC-CANVAS-011 REQ-03-a · REQ-08

import type { CanvasPoint, CanvasProjection } from '../canvasGeometry';
import type { CanvasNode, OutlinedNode } from '../group/groupTypes';
import { anchorPoints, type AnchorId } from './anchors';
import { isConnector, type ConnectorElement, type ConnectorEnd } from './connectorTypes';

/**
 * 연결선의 점 목록을 **캔버스 단위**로 낸다 — `[시작, …중간점, 끝]`.
 *
 * 끊긴 연결이면 `undefined` 다(REQ-08). 끊기는 경우 넷:
 *   1. 가리킨 id 의 노드가 목록에 없다(지워졌다 — AC-43).
 *   2. 그 id 가 **부품 복합 키**다. 최상위에 없으므로 1 과 같은 길로 떨어진다(AC-44).
 *   3. 가리킨 노드가 연결선이다(A6).
 *   4. 요소는 있으나 그 이름의 앵커 자리가 없다(REQ-02'-e).
 *
 * 반환하는 점은 전부 **새 객체**다. 중간점을 저장값 그대로 내보내면 소비자가 손잡이를
 * 끌 때 config 를 제자리에서 고치게 되고, 그 변경은 되돌리기 장부를 지나지 않는다.
 */
export function resolveConnector(
  connector: ConnectorElement,
  nodes: readonly CanvasNode[],
  proj: CanvasProjection,
  textWidths: Readonly<Record<string, number>>,
): readonly CanvasPoint[] | undefined {
  // 노드마다 한 벌. 두 끝이 같은 요소를 가리켜도 상자는 한 번만 난다(AC-33).
  const measured = new Map<string, ReadonlyMap<AnchorId, CanvasPoint>>();
  const anchorsOf = (node: OutlinedNode): ReadonlyMap<AnchorId, CanvasPoint> => {
    const seen = measured.get(node.id);
    if (seen !== undefined) return seen;
    const points = anchorPoints(node, proj, textWidths);
    measured.set(node.id, points);
    return points;
  };

  const resolveEnd = (end: ConnectorEnd): CanvasPoint | undefined => {
    // 자유 끝점은 제 좌표 그대로다 — 다른 요소가 무엇을 하든 움직이지 않는다(AC-42).
    if (!('el' in end)) return { x: end.x, y: end.y };
    const node = nodes.find((candidate) => candidate.id === end.el);
    if (node === undefined || isConnector(node)) return undefined;
    return anchorsOf(node).get(end.a);
  };

  const from = resolveEnd(connector.from);
  if (from === undefined) return undefined;
  const to = resolveEnd(connector.to);
  if (to === undefined) return undefined;

  // 중간점은 차례 그대로 지나간다 — 이 함수는 경로를 **해석**할 뿐 다시 짜지 않는다.
  const mid = connector.points ?? [];
  return [from, ...mid.map((point) => ({ x: point.x, y: point.y })), to];
}
