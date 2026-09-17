// 직각 경로가 피할 것들 — **셋이 함께 읽는 한 목록** (SPEC-CANVAS-017 M2 · §결정 3).
//
// 그리는 쪽(`drawElement`) · 잡는 쪽(`canvasHitTest`) · 점 편집(`connectorEdit`) 셋이 같은
// 목록을 봐야 한다(REQ-04). 셋이 저마다 거르면 "그려진 선과 잡히는 선이 다르다" 가
// 시작된다 — 011 M6·M7 이 `connectorPath` 를 한 함수로 둔 그 근거가 장애물에도 그대로 걸린다.
//
// ## 무엇이 빠지는가
//
//   - **연결선** — 상자가 없다(`outlineAabb` 의 인자가 `OutlinedNode` 인 것이 그 금지다).
//   - **`visible: false`** — 보이지 않는 도형을 피해 도는 선은 사용자에게 이유 없는 우회다.
//   - **두 끝 도형**(REQ-03) — 빼지 않으면 앵커가 제 도형의 경계에 있으므로 **선이 출발조차
//     못 한다.** 011 A12 가 "중심 앵커에서 물러나지 않는다 — 도형에 가려지는 대가는 숨기지
//     않는다" 로 그 자리를 이미 정해 두었다.
//
// 상자는 **돌아간 도형의 축-나란 상자**다 — 014 가 세운 `outlineAabb` 를 그대로 쓰므로
// 회전을 아는 두 번째 산술이 생기지 않는다.
//
// @spec SPEC-CANVAS-017 REQ-03 · REQ-04

import type { CanvasProjection, PxBox } from '../canvasGeometry';
import { outlineAabb } from '../canvasOutline';
import { isConnector, type ConnectorElement, type ConnectorEnd } from './connectorTypes';
import { isAttachedEnd } from './resolveConnector';
import { isGroup, type CanvasNode } from '../group/groupTypes';

/**
 * 이 끝이 가리키는 요소 id — 자유 끝이면 부재다.
 *
 * 가르는 일은 `isAttachedEnd` 한 함수가 한다(011 AC-45). 여기서 `'el' in` 을 적으면 그
 * 판정이 둘이 되고 출시된 가드가 곧바로 운다 — **013 이 이미 한 번 밟은 자리**이며, 그
 * 되풀이 자체가 그 가드의 값이다.
 *
 * **타입 이름을 그대로 적는다.** `ConnectorElement['from']` 으로 적으면 "`ConnectorEnd` 를
 * 들이는 파일이 몇인가" 를 세는 가드가 이 파일을 **지나친다** — 그것은 자리를 줄이는 것이
 * 아니라 세는 일을 피하는 것이고, 013 이 다섯째를 세어 적으며 이미 고른 판단이다.
 */
function endHost(end: ConnectorEnd): string | undefined {
  return isAttachedEnd(end) ? end.el : undefined;
}

/**
 * 이 연결선이 피해야 할 상자들(**스테이지 로컬 px**).
 *
 * px 인 것에 뜻이 있다 — `connectorPath` 가 점을 px 로 다루므로 그 자리에서 견주면 환산이
 * 끼어들지 않는다. 축척이 하나이므로(014 A1) 어느 공간에서 재도 같은 길이 나온다.
 */
export interface ConnectorRouting {
  /**
   * 피할 상자 **전부** — 두 끝 도형도 든다 (SPEC-CANVAS-018 REQ-01).
   *
   * 017 은 두 끝 도형을 여기서 뺐다. 근거("빼지 않으면 앵커가 제 도형의 경계에 있어 선이
   * 출발조차 못 한다")는 맞았고 **처분이 틀렸다** — 출발하지 못하는 것은 앵커 바로 옆 한
   * 구간의 문제인데 도형 **전체**를 뺐고, 그 결과 경로가 끝 도형의 변에 붙어 내려왔다.
   * 018 은 도형을 되돌려 놓고 그 한 구간만 따로 허용한다(`hosts`).
   */
  obstacles: PxBox[];
  /** 두 끝이 앉은 도형의 상자. 자유 끝이면 부재다 — 나갈 도형이 없다. */
  hosts: { from?: PxBox; to?: PxBox };
}

export function connectorObstacles(
  connector: ConnectorElement,
  nodes: readonly CanvasNode[],
  proj: CanvasProjection,
  textWidths: Readonly<Record<string, number>>,
): ConnectorRouting {
  const fromId = endHost(connector.from);
  const toId = endHost(connector.to);
  const hosts: { from?: PxBox; to?: PxBox } = {};
  const obstacles = nodes.flatMap((node): PxBox[] => {
    if (isConnector(node)) return [];
    // 그룹의 `style` 은 선택 필드라 `visible` 판정이 갈린다 — 그리는 쪽이 그룹 자신의
    // `visible` 을 보지 않는 것과 같은 자리이므로 여기서도 부품 기준을 만들지 않는다.
    if (!isGroup(node) && node.style.visible === false) return [];
    const box = outlineAabb(node, proj, textWidths);
    // 끝 도형은 **장애물이면서 동시에** 나갈 상자다. 빼지 않는 것이 018 의 전부다.
    if (node.id === fromId) hosts.from = box;
    if (node.id === toId) hosts.to = box;
    return [box];
  });
  return { obstacles, hosts };
}
