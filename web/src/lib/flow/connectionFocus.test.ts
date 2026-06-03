// 연결 포커스 헬퍼 테스트.
// 1-hop 양방향 연결, 가상 엣지 포함, 선택 노드 자신 포함, 고립 노드 →
// 자신만, null 선택 → 빈 집합을 검증한다.

import { describe, expect, it } from 'vitest';
import type { Edge } from '@xyflow/react';

import { getConnectedNodeIds, isEdgeConnectedToNode } from './connectionFocus';

/** 테스트용 엣지 생성 헬퍼(최상위 virtual 속성 포함). */
function makeEdge(partial: Partial<Edge> & Record<string, unknown>): Edge {
  return {
    id: 'e',
    source: 's',
    target: 't',
    ...partial,
  } as Edge;
}

describe('isEdgeConnectedToNode', () => {
  it('source 또는 target 이 노드와 일치하면 true', () => {
    const edge = makeEdge({ source: 'A', target: 'B' });
    expect(isEdgeConnectedToNode(edge, 'A')).toBe(true);
    expect(isEdgeConnectedToNode(edge, 'B')).toBe(true);
    expect(isEdgeConnectedToNode(edge, 'C')).toBe(false);
  });

  it('가상/비가상 구분 없이 닿기만 하면 true', () => {
    const virtual = makeEdge({ source: 'A', target: 'B', virtual: true });
    expect(isEdgeConnectedToNode(virtual, 'A')).toBe(true);
    expect(isEdgeConnectedToNode(virtual, 'B')).toBe(true);
  });
});

describe('getConnectedNodeIds', () => {
  it('selectedNodeId 가 null 이면 빈 집합을 반환한다', () => {
    const edges = [makeEdge({ source: 'A', target: 'B' })];
    const result = getConnectedNodeIds(edges, null);
    expect(result.size).toBe(0);
  });

  it('선택 노드 자신을 항상 포함한다', () => {
    const result = getConnectedNodeIds([], 'A');
    expect(result.has('A')).toBe(true);
    expect(result.size).toBe(1);
  });

  it('고립 노드(연결 없음)는 자신만 담는다', () => {
    const edges = [makeEdge({ id: 'e1', source: 'B', target: 'C' })];
    const result = getConnectedNodeIds(edges, 'A');
    expect([...result]).toEqual(['A']);
  });

  it('선택 노드가 source 인 엣지의 target 을 포함한다(정방향 1-hop)', () => {
    const edges = [makeEdge({ id: 'e1', source: 'A', target: 'B' })];
    const result = getConnectedNodeIds(edges, 'A');
    expect(result.has('A')).toBe(true);
    expect(result.has('B')).toBe(true);
    expect(result.size).toBe(2);
  });

  it('선택 노드가 target 인 엣지의 source 를 포함한다(역방향 1-hop)', () => {
    const edges = [makeEdge({ id: 'e1', source: 'X', target: 'A' })];
    const result = getConnectedNodeIds(edges, 'A');
    expect(result.has('A')).toBe(true);
    expect(result.has('X')).toBe(true);
    expect(result.size).toBe(2);
  });

  it('양방향 이웃을 모두 포함한다', () => {
    const edges = [
      makeEdge({ id: 'e1', source: 'A', target: 'B' }),
      makeEdge({ id: 'e2', source: 'X', target: 'A' }),
      makeEdge({ id: 'e3', source: 'B', target: 'C' }), // 2-hop — 제외되어야 함
    ];
    const result = getConnectedNodeIds(edges, 'A');
    expect([...result].sort()).toEqual(['A', 'B', 'X']);
    expect(result.has('C')).toBe(false);
  });

  it('가상 엣지도 연결로 포함한다', () => {
    const edges = [
      makeEdge({ id: 'e1', source: 'A', target: 'V', virtual: true }),
    ];
    const result = getConnectedNodeIds(edges, 'A');
    expect(result.has('V')).toBe(true);
  });

  it('자기 자신으로의 루프 엣지여도 자신만 담는다', () => {
    const edges = [makeEdge({ id: 'e1', source: 'A', target: 'A' })];
    const result = getConnectedNodeIds(edges, 'A');
    expect([...result]).toEqual(['A']);
  });

  it('같은 이웃으로의 다중 엣지는 중복 없이 한 번만 담는다', () => {
    const edges = [
      makeEdge({ id: 'e1', source: 'A', target: 'B' }),
      makeEdge({ id: 'e2', source: 'A', target: 'B', virtual: true }),
    ];
    const result = getConnectedNodeIds(edges, 'A');
    expect([...result].sort()).toEqual(['A', 'B']);
  });
});
