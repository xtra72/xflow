// 연결 포커스 — 노드 선택 시 연결을 강조하고 나머지를 흐리게 하는 헬퍼.
//
// "연결 포커스" 뷰 토글이 켜지고 정확히 한 노드가 선택되면, 선택 노드로부터
// 지정한 단계(depth) 이내로 연결된 노드/엣지만 강조하고 그 외는 흐리게
// (opacity 낮춤) 렌더한다. 이 모듈은 그 "연결 집합"을 계산하는 순수 함수만
// 제공한다(렌더링/스토어 의존성 없음).
//
// 방향성(directional) 탐색: 선택 노드의 조상(상류)/자손(하류) 사슬만 강조하고,
// 단지 같은 하류 타깃을 공유하는 형제(sibling) 처럼 순수 조상/자손이 아닌
// 노드는 제외한다. 선택 노드로부터 서로 독립적인 두 방향을 각각 depth hop 까지
// 탐색한다:
//   - 하류(downstream, 출력 방향): 현재 노드가 source 인 엣지를 따라 target 으로
//     이동한다(선택 노드의 출력을 정방향으로 따라간다).
//   - 상류(upstream, 입력 방향): 현재 노드가 target 인 엣지를 따라 source 로
//     이동한다(선택 노드의 입력을 역방향으로 따라간다).
// 한 경로 안에서 방향을 섞지 않는다 — 하류 탐색은 항상 정방향만, 상류 탐색은
// 항상 역방향만 따라간다. 따라서 형제 관계(타깃/소스를 공유하지만 선택 노드의
// 순수 조상/자손이 아닌 노드) 로만 연결된 노드는 제외된다.
//
// 가상(virtual) 엣지도 탐색 대상에 포함한다 — 가상 와이어가 숨겨져 있어도
// 논리적 연결이므로 포커스 탐색에 포함한다.
//
// 단계(depth) 는 선택 노드로부터의 hop 수다. depth=1 이면 선택 노드 + 직접
// 상류/하류 이웃, depth=2 면 각 방향으로 한 hop 씩 더 확장한다. depth<=0 이면
// 선택 노드 자신만 담고 따라간 엣지는 없다. 사이클은 방문 집합으로 종료하며,
// 자기 자신으로의 루프 엣지는 건너뛴다.

import type { Edge } from '@xyflow/react';

/**
 * 주어진 엣지가 특정 노드에 닿는지(source 또는 target) 여부.
 *
 * 가상/비가상 구분 없이 source 또는 target 이 nodeId 와 일치하면 true.
 */
export function isEdgeConnectedToNode(edge: Edge, nodeId: string): boolean {
  return edge.source === nodeId || edge.target === nodeId;
}

/**
 * 방향성 연결 탐색 결과.
 *
 * - `nodeIds`: 선택 노드 + 상류/하류로 도달한 모든 노드 id(선택 노드 항상 포함).
 * - `edgeIds`: 상류/하류 BFS 중 따라간(traversed) 모든 엣지 id.
 */
export interface ConnectedElements {
  nodeIds: Set<string>;
  edgeIds: Set<string>;
}

/**
 * 한 방향으로 BFS 를 수행해 도달 노드와 따라간 엣지를 누적하는 내부 헬퍼.
 *
 * @param edges        전체 엣지 목록.
 * @param start        탐색 시작 노드(선택 노드).
 * @param maxDepth     최대 hop 수(>=1 보장은 호출부 책임).
 * @param forward      true 면 하류(source→target), false 면 상류(target→source).
 * @param nodeIds      도달 노드를 누적하는 집합(start 는 이미 포함된 상태로 전달).
 * @param edgeIds      따라간 엣지를 누적하는 집합.
 */
function traverseDirection(
  edges: Edge[],
  start: string,
  maxDepth: number,
  forward: boolean,
  nodeIds: Set<string>,
  edgeIds: Set<string>,
): void {
  // 방향별 인접 리스트: forward 면 source→[{neighbor:target, edgeId}],
  // backward 면 target→[{neighbor:source, edgeId}]. 자기 루프는 제외한다.
  const adjacency = new Map<string, { neighbor: string; edgeId: string }[]>();
  for (const edge of edges) {
    if (edge.source === edge.target) continue; // 자기 루프 무시
    const from = forward ? edge.source : edge.target;
    const neighbor = forward ? edge.target : edge.source;
    const list = adjacency.get(from);
    const item = { neighbor, edgeId: edge.id };
    if (list) {
      list.push(item);
    } else {
      adjacency.set(from, [item]);
    }
  }

  // 방향별 방문 집합(선택 노드 기준 독립 탐색). 시작 노드는 방문 처리한다.
  const visited = new Set<string>([start]);
  let frontier: string[] = [start];
  for (let hop = 0; hop < maxDepth && frontier.length > 0; hop += 1) {
    const next: string[] = [];
    for (const nodeId of frontier) {
      for (const { neighbor, edgeId } of adjacency.get(nodeId) ?? []) {
        // 따라간 엣지는 항상 기록한다(사이클로 인해 이웃이 이미 방문돼도 동일).
        edgeIds.add(edgeId);
        if (!visited.has(neighbor)) {
          visited.add(neighbor);
          nodeIds.add(neighbor);
          next.push(neighbor);
        }
      }
    }
    frontier = next;
  }
}

/**
 * 선택 노드로부터 `depth` hop 이내로 방향성 연결된 노드/엣지 집합을 계산하는
 * 순수 헬퍼.
 *
 * 하류(source→target) 와 상류(target→source) 두 방향을 서로 독립적으로 각각
 * depth hop 까지 탐색한다(한 경로 안에서 방향 혼합 없음). 형제 관계로만 연결된
 * 노드는 제외된다.
 *
 * - 반환 `nodeIds` 에는 선택 노드 자신이 항상 포함된다(고립 노드여도 자신만 담김).
 * - 반환 `edgeIds` 에는 상류/하류 탐색 중 따라간 엣지 id 가 모두 담긴다.
 * - 가상(virtual) 엣지도 탐색 대상에 포함한다(숨겨진 와이어라도 논리적 연결).
 * - selectedNodeId 가 null 이면 빈 집합들을 반환한다(포커스 대상 없음).
 * - depth=1 은 선택 노드 + 직접 상류/하류 이웃.
 * - depth<=0 이면 선택 노드 자신만 담고 edgeIds 는 비어 있다.
 * - 사이클이 있어도 방문 집합으로 중복 없이 종료한다.
 *
 * @param edges          스토어의 전체 엣지 목록.
 * @param selectedNodeId 현재 단일 선택된 노드 id(없으면 null).
 * @param depth          포함할 최대 hop 수(기본 1). 정수가 아니면 내림한다.
 */
export function getConnectedElements(
  edges: Edge[],
  selectedNodeId: string | null,
  depth = 1,
): ConnectedElements {
  const nodeIds = new Set<string>();
  const edgeIds = new Set<string>();
  if (selectedNodeId === null) return { nodeIds, edgeIds };

  // 선택 노드 자신을 항상 포함한다(고립 노드 → 자신만 담김).
  nodeIds.add(selectedNodeId);

  // depth<=0 이면 이웃 탐색 없이 선택 노드만 반환한다(따라간 엣지 없음).
  const maxDepth = Math.floor(depth);
  if (maxDepth <= 0) return { nodeIds, edgeIds };

  // 하류/상류를 독립적으로 탐색한다(각각 선택 노드에서 시작).
  traverseDirection(edges, selectedNodeId, maxDepth, true, nodeIds, edgeIds);
  traverseDirection(edges, selectedNodeId, maxDepth, false, nodeIds, edgeIds);

  return { nodeIds, edgeIds };
}

/**
 * `getConnectedElements` 의 thin wrapper. 방향성 연결 노드 id 집합만 반환한다.
 *
 * 노드 단위 흐림 처리(CustomNode) 처럼 엣지 집합이 필요 없는 호출부를 위한
 * 편의 함수다. 방향성 의미는 `getConnectedElements` 와 동일하다.
 */
export function getConnectedNodeIds(
  edges: Edge[],
  selectedNodeId: string | null,
  depth = 1,
): Set<string> {
  return getConnectedElements(edges, selectedNodeId, depth).nodeIds;
}
