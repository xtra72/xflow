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
// ## 끝을 **가르는** 일도 여기 산다 (M12)
//
// 참조를 푸는 일만이 아니라 "이 끝이 요소에 붙었는가" 를 묻는 일도 이 파일의 몫이다
// (`isAttachedEnd`). 그 갈림이 흩어지면 푸는 쪽과 말하는 쪽이 서로 다른 답을 낼 수 있고,
// 가드가 소스에서 그것을 세고 있다. 그 위에 얹힌 `connectorTargets` 는 **푸는 것이 아니라
// 세는 것**이라 노드 목록도 투영도 받지 않는다 — 지우기 경로(M12)가 그 가벼움을 쓴다.
//
// @spec SPEC-CANVAS-011 REQ-03-a · REQ-08

import type { CanvasPoint, CanvasProjection } from '../canvasGeometry';
import type { CanvasNode, OutlinedNode } from '../group/groupTypes';
import { anchorPoints, type AnchorId } from './anchors';
import { isConnector, type ConnectorElement, type ConnectorEnd } from './connectorTypes';

/**
 * 이 끝이 **요소에 붙어 있는가** — 붙은 끝과 자유 끝을 가르는 **유일한 자리**다.
 *
 * 011 은 그 갈림을 한 자리에 묶기로 했고 가드가 소스에서 그것을 세고 있다(`'el' in` 이
 * 적힌 제품 파일이 이 파일 하나다). 판정을 함수로 **내보내는 것**이 그 규율을 지키면서
 * 다른 층이 끝을 말할 수 있게 하는 길이다: 목록 행이 "시작은 어디에 붙었는가" 를 말하려면
 * 그 갈림이 필요한데, 거기서 `'el' in` 을 한 번 더 적으면 판정이 둘이 되고 그 둘은
 * 언젠가 갈라진다 — `isConnector` 가 `kind` 판정에 대해 세운 그 규율과 같은 자다.
 *
 * 좁히는 타입을 `ConnectorEnd` 의 갈래 그대로 적는 것에도 뜻이 있다. 부르는 쪽은 이
 * 함수를 지나는 것만으로 `el`·`a` 를 읽을 수 있으므로 자료형 이름을 따로 들일 필요가 없다.
 */
export function isAttachedEnd(end: ConnectorEnd): end is { el: string; a: string } {
  return 'el' in end;
}

/**
 * 이 연결선이 **붙어 있는 요소 id** 들 — 없거나(양 끝이 자유) 하나이거나 둘이다.
 *
 * 푸는 것이 아니라 **가리키는 이름을 세는** 함수다. 그래서 노드 목록도 투영도 받지 않고,
 * 지워진 요소를 참조하는 연결선을 걷어내는 쪽(`canvasEditArrange.removeNodesWithConnectors`)
 * 이 이것만으로 답을 얻는다 — 지우는 길에서 상자를 재게 하면 지우기가 글자 폭 장부를
 * 요구하게 되고, 목록 편집기에는 그런 것이 없다.
 *
 * 같은 요소를 두 끝이 가리키면 그 id 가 **두 번** 들어온다. 부르는 쪽이 집합으로 받으므로
 * 접지 않는다 — 여기서 접으면 "몇 자리가 붙어 있는가" 를 묻는 다음 사람이 답을 잃는다.
 */
export function connectorTargets(connector: ConnectorElement): readonly string[] {
  const out: string[] = [];
  for (const end of [connector.from, connector.to]) {
    if (isAttachedEnd(end)) out.push(end.el);
  }
  return out;
}

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
    if (!isAttachedEnd(end)) return { x: end.x, y: end.y };
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
