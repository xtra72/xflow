// 연결 포커스 — 노드 선택 시 직접 연결을 강조하고 나머지를 흐리게 하는 헬퍼.
//
// "연결 포커스" 뷰 토글이 켜지고 정확히 한 노드가 선택되면, 선택 노드와
// 1-hop(직접) 연결된 노드/엣지만 강조하고 그 외는 흐리게(opacity 낮춤) 렌더한다.
// 이 모듈은 그 "연결 집합"을 계산하는 순수 함수만 제공한다(렌더링/스토어 의존성 없음).
//
// 가상(virtual) 엣지도 연결로 간주한다 — 가상 와이어가 숨겨져 있어도 논리적
// 연결이므로 포커스 강조 대상에 포함한다(Feature 2 요구사항).

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
 * 선택된 노드와 1-hop(직접) 연결된 모든 노드 id 집합을 계산하는 순수 헬퍼.
 *
 * - 반환 집합에는 선택 노드 자신이 항상 포함된다(고립 노드여도 자신만 담긴다).
 * - source/target 양방향 모두를 1-hop 연결로 본다.
 * - 가상(virtual) 엣지도 연결로 간주한다(숨겨진 와이어라도 논리적 연결).
 * - selectedNodeId 가 null 이면 빈 집합을 반환한다(포커스 대상 없음).
 *
 * @param edges          스토어의 전체 엣지 목록.
 * @param selectedNodeId 현재 단일 선택된 노드 id(없으면 null).
 */
export function getConnectedNodeIds(
  edges: Edge[],
  selectedNodeId: string | null,
): Set<string> {
  const connected = new Set<string>();
  if (selectedNodeId === null) return connected;

  // 선택 노드 자신을 항상 포함한다(고립 노드 → 자신만 담김).
  connected.add(selectedNodeId);

  for (const edge of edges) {
    if (edge.source === selectedNodeId) {
      connected.add(edge.target);
    }
    if (edge.target === selectedNodeId) {
      connected.add(edge.source);
    }
  }

  return connected;
}
