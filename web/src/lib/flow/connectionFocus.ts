// 연결 포커스 — 노드 선택 시 연결을 강조하고 나머지를 흐리게 하는 헬퍼.
//
// "연결 포커스" 뷰 토글이 켜지고 정확히 한 노드가 선택되면, 선택 노드로부터
// 지정한 단계(depth) 이내로 연결된 노드/엣지만 강조하고 그 외는 흐리게
// (opacity 낮춤) 렌더한다. 이 모듈은 그 "연결 집합"을 계산하는 순수 함수만
// 제공한다(렌더링/스토어 의존성 없음).
//
// 가상(virtual) 엣지도 연결로 간주한다 — 가상 와이어가 숨겨져 있어도 논리적
// 연결이므로 포커스 강조 대상에 포함한다(Feature 2 요구사항).
//
// 단계(depth) 는 선택 노드로부터의 hop 수다. depth=1 이면 선택 노드 + 직접
// 이웃(기존 동작), depth=2 면 이웃의 이웃까지, depth 가 충분히 크면 같은
// 연결 컴포넌트 전체를 포함한다. depth<=0 이면 선택 노드 자신만 담는다.

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
 * 선택된 노드로부터 `depth` hop 이내로 연결된 모든 노드 id 집합을 계산하는
 * 순수 헬퍼. 무방향(undirected) 인접 그래프 위에서 BFS 로 탐색한다.
 *
 * - 반환 집합에는 선택 노드 자신이 항상 포함된다(고립 노드여도 자신만 담긴다).
 * - source/target 양방향 모두를 연결로 본다(무방향 그래프).
 * - 가상(virtual) 엣지도 연결로 간주한다(숨겨진 와이어라도 논리적 연결).
 * - selectedNodeId 가 null 이면 빈 집합을 반환한다(포커스 대상 없음).
 * - depth=1 은 선택 노드 + 직접 이웃(기존 1-hop 동작과 동일).
 * - depth<=0 이면 선택 노드 자신만 담는다(이웃 미포함).
 * - depth 가 충분히 크면 같은 연결 컴포넌트 전체를 담는다. 사이클이 있어도
 *   방문 집합으로 중복 없이 종료한다.
 *
 * @param edges          스토어의 전체 엣지 목록.
 * @param selectedNodeId 현재 단일 선택된 노드 id(없으면 null).
 * @param depth          포함할 최대 hop 수(기본 1). 정수가 아니면 내림한다.
 */
export function getConnectedNodeIds(
  edges: Edge[],
  selectedNodeId: string | null,
  depth = 1,
): Set<string> {
  const connected = new Set<string>();
  if (selectedNodeId === null) return connected;

  // 선택 노드 자신을 항상 포함한다(고립 노드 → 자신만 담김).
  connected.add(selectedNodeId);

  // depth<=0 이면 이웃 탐색 없이 선택 노드만 반환한다.
  const maxDepth = Math.floor(depth);
  if (maxDepth <= 0) return connected;

  // 무방향 인접 리스트를 구성한다(source↔target 양방향, 자기 루프는 무시).
  const adjacency = new Map<string, string[]>();
  const addNeighbor = (from: string, to: string): void => {
    if (from === to) return;
    const list = adjacency.get(from);
    if (list) {
      list.push(to);
    } else {
      adjacency.set(from, [to]);
    }
  };
  for (const edge of edges) {
    addNeighbor(edge.source, edge.target);
    addNeighbor(edge.target, edge.source);
  }

  // 선택 노드에서 시작하는 BFS. 각 레벨을 한 hop 으로 세어 maxDepth 까지 확장한다.
  let frontier: string[] = [selectedNodeId];
  for (let hop = 0; hop < maxDepth && frontier.length > 0; hop += 1) {
    const next: string[] = [];
    for (const nodeId of frontier) {
      for (const neighbor of adjacency.get(nodeId) ?? []) {
        if (!connected.has(neighbor)) {
          connected.add(neighbor);
          next.push(neighbor);
        }
      }
    }
    frontier = next;
  }

  return connected;
}
