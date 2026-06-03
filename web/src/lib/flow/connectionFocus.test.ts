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

  it('depth 기본값(미지정) 은 1-hop 과 동일하다', () => {
    const edges = [
      makeEdge({ id: 'e1', source: 'A', target: 'B' }),
      makeEdge({ id: 'e2', source: 'B', target: 'C' }), // 2-hop
    ];
    const withDefault = getConnectedNodeIds(edges, 'A');
    const withOne = getConnectedNodeIds(edges, 'A', 1);
    expect([...withDefault].sort()).toEqual(['A', 'B']);
    expect([...withOne].sort()).toEqual([...withDefault].sort());
  });
});

describe('getConnectedNodeIds — depth(BFS 다중 hop)', () => {
  // A - B - C - D 직선 체인.
  const chain = [
    makeEdge({ id: 'e1', source: 'A', target: 'B' }),
    makeEdge({ id: 'e2', source: 'B', target: 'C' }),
    makeEdge({ id: 'e3', source: 'C', target: 'D' }),
  ];

  it('depth=1 은 선택 노드 + 직접 이웃만 담는다', () => {
    const result = getConnectedNodeIds(chain, 'A', 1);
    expect([...result].sort()).toEqual(['A', 'B']);
  });

  it('depth=2 는 2-hop 이웃까지 담는다', () => {
    const result = getConnectedNodeIds(chain, 'A', 2);
    expect([...result].sort()).toEqual(['A', 'B', 'C']);
    expect(result.has('D')).toBe(false);
  });

  it('depth=3 은 3-hop 이웃까지 담는다(체인 끝)', () => {
    const result = getConnectedNodeIds(chain, 'A', 3);
    expect([...result].sort()).toEqual(['A', 'B', 'C', 'D']);
  });

  it('역방향(B→A) 엣지도 무방향으로 따라가며 다중 hop 탐색한다', () => {
    const edges = [
      makeEdge({ id: 'e1', source: 'B', target: 'A' }), // A 의 이웃 B
      makeEdge({ id: 'e2', source: 'C', target: 'B' }), // B 의 이웃 C
    ];
    expect([...getConnectedNodeIds(edges, 'A', 1)].sort()).toEqual(['A', 'B']);
    expect([...getConnectedNodeIds(edges, 'A', 2)].sort()).toEqual([
      'A',
      'B',
      'C',
    ]);
  });

  it('depth 가 충분히 크면 같은 연결 컴포넌트 전체를 담는다', () => {
    const result = getConnectedNodeIds(chain, 'A', 99);
    expect([...result].sort()).toEqual(['A', 'B', 'C', 'D']);
  });

  it('다른 컴포넌트의 노드는 depth 가 커도 포함하지 않는다', () => {
    const edges = [
      ...chain,
      makeEdge({ id: 'x1', source: 'X', target: 'Y' }), // 분리된 컴포넌트
    ];
    const result = getConnectedNodeIds(edges, 'A', 99);
    expect(result.has('X')).toBe(false);
    expect(result.has('Y')).toBe(false);
  });

  it('depth=0 이면 선택 노드 자신만 담는다(이웃 미포함)', () => {
    const result = getConnectedNodeIds(chain, 'A', 0);
    expect([...result]).toEqual(['A']);
  });

  it('음수 depth 도 선택 노드 자신만 담는다', () => {
    const result = getConnectedNodeIds(chain, 'A', -3);
    expect([...result]).toEqual(['A']);
  });

  it('사이클이 있어도 무한 루프 없이 종료하고 중복 없이 담는다', () => {
    // 삼각형 사이클 A-B-C-A.
    const cycle = [
      makeEdge({ id: 'e1', source: 'A', target: 'B' }),
      makeEdge({ id: 'e2', source: 'B', target: 'C' }),
      makeEdge({ id: 'e3', source: 'C', target: 'A' }),
    ];
    const result = getConnectedNodeIds(cycle, 'A', 99);
    expect([...result].sort()).toEqual(['A', 'B', 'C']);
    expect(result.size).toBe(3);
  });

  it('사이클을 통한 짧은 경로(2 hop)도 정확히 반영한다', () => {
    // 사각형 A-B-C-D-A: A 에서 D 는 1 hop(직접) 으로도 닿는다.
    const square = [
      makeEdge({ id: 'e1', source: 'A', target: 'B' }),
      makeEdge({ id: 'e2', source: 'B', target: 'C' }),
      makeEdge({ id: 'e3', source: 'C', target: 'D' }),
      makeEdge({ id: 'e4', source: 'D', target: 'A' }),
    ];
    // depth=1: A 의 직접 이웃은 B, D.
    expect([...getConnectedNodeIds(square, 'A', 1)].sort()).toEqual([
      'A',
      'B',
      'D',
    ]);
    // depth=2: C 까지 전체 포함.
    expect([...getConnectedNodeIds(square, 'A', 2)].sort()).toEqual([
      'A',
      'B',
      'C',
      'D',
    ]);
  });

  it('비정수 depth 는 내림 처리한다(2.9 → 2 hop)', () => {
    const result = getConnectedNodeIds(chain, 'A', 2.9);
    expect([...result].sort()).toEqual(['A', 'B', 'C']);
  });

  it('가상 엣지를 통한 다중 hop 도 따라간다', () => {
    const edges = [
      makeEdge({ id: 'e1', source: 'A', target: 'B', virtual: true }),
      makeEdge({ id: 'e2', source: 'B', target: 'C', virtual: true }),
    ];
    expect([...getConnectedNodeIds(edges, 'A', 2)].sort()).toEqual([
      'A',
      'B',
      'C',
    ]);
  });

  it('selectedNodeId 가 null 이면 depth 와 무관하게 빈 집합', () => {
    expect(getConnectedNodeIds(chain, null, 5).size).toBe(0);
  });
});
