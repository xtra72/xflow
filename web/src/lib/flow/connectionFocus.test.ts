// 연결 포커스 헬퍼 테스트(방향성 탐색).
//
// 하류(source→target)/상류(target→source) 독립 탐색, 형제 노드 제외, 가상 엣지
// 포함, 따라간 엣지(edgeIds) 정확성, depth 1/2, 사이클, 선택 노드 자신 포함,
// null 선택 → 빈 집합을 검증한다.

import { describe, expect, it } from 'vitest';
import type { Edge } from '@xyflow/react';

import {
  getConnectedElements,
  getConnectedNodeIds,
  isEdgeConnectedToNode,
} from './connectionFocus';

/** 테스트용 엣지 생성 헬퍼(최상위 virtual 속성 포함). */
function makeEdge(partial: Partial<Edge> & Record<string, unknown>): Edge {
  return {
    id: 'e',
    source: 's',
    target: 't',
    ...partial,
  } as Edge;
}

/** Set 을 정렬된 배열로 변환(비교 편의). */
function sorted(set: Set<string>): string[] {
  return [...set].sort();
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

describe('getConnectedElements — 기본/경계', () => {
  it('selectedNodeId 가 null 이면 빈 집합들을 반환한다', () => {
    const edges = [makeEdge({ source: 'A', target: 'B' })];
    const { nodeIds, edgeIds } = getConnectedElements(edges, null);
    expect(nodeIds.size).toBe(0);
    expect(edgeIds.size).toBe(0);
  });

  it('선택 노드 자신을 항상 포함한다(엣지 없음)', () => {
    const { nodeIds, edgeIds } = getConnectedElements([], 'A');
    expect([...nodeIds]).toEqual(['A']);
    expect(edgeIds.size).toBe(0);
  });

  it('고립 노드(연결 없음)는 자신만 담는다', () => {
    const edges = [makeEdge({ id: 'e1', source: 'B', target: 'C' })];
    const { nodeIds, edgeIds } = getConnectedElements(edges, 'A');
    expect([...nodeIds]).toEqual(['A']);
    expect(edgeIds.size).toBe(0);
  });

  it('depth=0 이면 선택 노드 자신만 담고 edgeIds 는 비어 있다', () => {
    const edges = [makeEdge({ id: 'e1', source: 'A', target: 'B' })];
    const { nodeIds, edgeIds } = getConnectedElements(edges, 'A', 0);
    expect([...nodeIds]).toEqual(['A']);
    expect(edgeIds.size).toBe(0);
  });

  it('음수 depth 도 선택 노드 자신만 담는다', () => {
    const edges = [makeEdge({ id: 'e1', source: 'A', target: 'B' })];
    const { nodeIds, edgeIds } = getConnectedElements(edges, 'A', -3);
    expect([...nodeIds]).toEqual(['A']);
    expect(edgeIds.size).toBe(0);
  });

  it('자기 자신으로의 루프 엣지는 건너뛴다(자신만, edgeIds 비어 있음)', () => {
    const edges = [makeEdge({ id: 'e1', source: 'A', target: 'A' })];
    const { nodeIds, edgeIds } = getConnectedElements(edges, 'A', 3);
    expect([...nodeIds]).toEqual(['A']);
    expect(edgeIds.size).toBe(0);
  });

  it('selectedNodeId 가 null 이면 depth 와 무관하게 빈 집합', () => {
    const edges = [makeEdge({ id: 'e1', source: 'A', target: 'B' })];
    const { nodeIds } = getConnectedElements(edges, null, 5);
    expect(nodeIds.size).toBe(0);
  });
});

describe('getConnectedElements — 방향성(하류/상류) depth=1', () => {
  it('하류만: 선택 노드가 source 인 엣지의 target 을 포함한다', () => {
    const edges = [makeEdge({ id: 'e1', source: 'A', target: 'B' })];
    const { nodeIds, edgeIds } = getConnectedElements(edges, 'A', 1);
    expect(sorted(nodeIds)).toEqual(['A', 'B']);
    expect(sorted(edgeIds)).toEqual(['e1']);
  });

  it('상류만: 선택 노드가 target 인 엣지의 source 를 포함한다', () => {
    const edges = [makeEdge({ id: 'e1', source: 'X', target: 'A' })];
    const { nodeIds, edgeIds } = getConnectedElements(edges, 'A', 1);
    expect(sorted(nodeIds)).toEqual(['A', 'X']);
    expect(sorted(edgeIds)).toEqual(['e1']);
  });

  it('상류와 하류를 모두 각각 따라간다', () => {
    const edges = [
      makeEdge({ id: 'e1', source: 'A', target: 'B' }), // 하류
      makeEdge({ id: 'e2', source: 'X', target: 'A' }), // 상류
    ];
    const { nodeIds, edgeIds } = getConnectedElements(edges, 'A', 1);
    expect(sorted(nodeIds)).toEqual(['A', 'B', 'X']);
    expect(sorted(edgeIds)).toEqual(['e1', 'e2']);
  });

  it('형제 노드 제외: A→B→C, D→B 에서 B 선택 시 형제 D 의 하류는 따라가지 않는다', () => {
    // B 선택: 하류 = C(B→C), 상류 = A(A→B), D(D→B).
    // D 는 상류 노드로 포함되지만, D 의 하류(D→B 외) 는 더 따라가지 않는다.
    const edges = [
      makeEdge({ id: 'ab', source: 'A', target: 'B' }),
      makeEdge({ id: 'bc', source: 'B', target: 'C' }),
      makeEdge({ id: 'db', source: 'D', target: 'B' }),
    ];
    const { nodeIds, edgeIds } = getConnectedElements(edges, 'B', 1);
    expect(sorted(nodeIds)).toEqual(['A', 'B', 'C', 'D']);
    expect(sorted(edgeIds)).toEqual(['ab', 'bc', 'db']);
  });

  it('A→B, A→G 에서 B 선택 시 형제 출력 G 는 제외한다', () => {
    // B 선택: 상류 A(A→B). A 의 다른 출력 G(A→G) 는 하류 엣지이므로 상류
    // 탐색에서 따라가지 않는다 → G 제외, ag 엣지도 제외.
    const edges = [
      makeEdge({ id: 'ab', source: 'A', target: 'B' }),
      makeEdge({ id: 'ag', source: 'A', target: 'G' }),
    ];
    const { nodeIds, edgeIds } = getConnectedElements(edges, 'B', 1);
    expect(sorted(nodeIds)).toEqual(['A', 'B']);
    expect(nodeIds.has('G')).toBe(false);
    expect(sorted(edgeIds)).toEqual(['ab']);
    expect(edgeIds.has('ag')).toBe(false);
  });

  it('depth=1 은 직접 상류/하류 이웃만 담는다(2-hop 제외)', () => {
    const edges = [
      makeEdge({ id: 'e1', source: 'A', target: 'B' }),
      makeEdge({ id: 'e2', source: 'B', target: 'C' }), // 2-hop 하류
    ];
    const { nodeIds, edgeIds } = getConnectedElements(edges, 'A', 1);
    expect(sorted(nodeIds)).toEqual(['A', 'B']);
    expect(nodeIds.has('C')).toBe(false);
    expect(sorted(edgeIds)).toEqual(['e1']);
  });

  it('depth 기본값(미지정) 은 depth=1 과 동일하다', () => {
    const edges = [
      makeEdge({ id: 'e1', source: 'A', target: 'B' }),
      makeEdge({ id: 'e2', source: 'B', target: 'C' }),
    ];
    const def = getConnectedElements(edges, 'A');
    const one = getConnectedElements(edges, 'A', 1);
    expect(sorted(def.nodeIds)).toEqual(sorted(one.nodeIds));
    expect(sorted(def.edgeIds)).toEqual(sorted(one.edgeIds));
    expect(sorted(def.nodeIds)).toEqual(['A', 'B']);
  });

  it('같은 이웃으로의 다중 엣지는 노드는 한 번, 엣지는 모두 담는다', () => {
    const edges = [
      makeEdge({ id: 'e1', source: 'A', target: 'B' }),
      makeEdge({ id: 'e2', source: 'A', target: 'B', virtual: true }),
    ];
    const { nodeIds, edgeIds } = getConnectedElements(edges, 'A', 1);
    expect(sorted(nodeIds)).toEqual(['A', 'B']);
    expect(sorted(edgeIds)).toEqual(['e1', 'e2']);
  });
});

describe('getConnectedElements — depth 다중 hop', () => {
  // A → B → C → D 직선 체인(모두 정방향).
  const chain = [
    makeEdge({ id: 'e1', source: 'A', target: 'B' }),
    makeEdge({ id: 'e2', source: 'B', target: 'C' }),
    makeEdge({ id: 'e3', source: 'C', target: 'D' }),
  ];

  it('depth=2 는 하류로 2-hop 까지 담는다(따라간 엣지 정확)', () => {
    const { nodeIds, edgeIds } = getConnectedElements(chain, 'A', 2);
    expect(sorted(nodeIds)).toEqual(['A', 'B', 'C']);
    expect(nodeIds.has('D')).toBe(false);
    expect(sorted(edgeIds)).toEqual(['e1', 'e2']);
  });

  it('depth=3 은 체인 끝(D) 까지 담고 모든 엣지를 따라간다', () => {
    const { nodeIds, edgeIds } = getConnectedElements(chain, 'A', 3);
    expect(sorted(nodeIds)).toEqual(['A', 'B', 'C', 'D']);
    expect(sorted(edgeIds)).toEqual(['e1', 'e2', 'e3']);
  });

  it('중간 노드 선택 시 상류/하류 각각 depth 만큼 확장한다', () => {
    // B 선택 depth=2: 상류 A(1-hop). 하류 C(1-hop), D(2-hop).
    const { nodeIds, edgeIds } = getConnectedElements(chain, 'B', 2);
    expect(sorted(nodeIds)).toEqual(['A', 'B', 'C', 'D']);
    expect(sorted(edgeIds)).toEqual(['e1', 'e2', 'e3']);
  });

  it('상류 다중 hop: 역방향 체인을 source 쪽으로 따라간다', () => {
    // C→B→A: A 의 상류는 없고, C 선택 시 상류 B(1-hop), A(2-hop).
    const edges = [
      makeEdge({ id: 'e1', source: 'B', target: 'A' }),
      makeEdge({ id: 'e2', source: 'C', target: 'B' }),
    ];
    expect(sorted(getConnectedElements(edges, 'C', 1).nodeIds)).toEqual([
      'B',
      'C',
    ]);
    const two = getConnectedElements(edges, 'C', 2);
    expect(sorted(two.nodeIds)).toEqual(['A', 'B', 'C']);
    expect(sorted(two.edgeIds)).toEqual(['e1', 'e2']);
  });

  it('다른 컴포넌트의 노드는 depth 가 커도 포함하지 않는다', () => {
    const edges = [
      ...chain,
      makeEdge({ id: 'x1', source: 'X', target: 'Y' }),
    ];
    const { nodeIds } = getConnectedElements(edges, 'A', 99);
    expect(nodeIds.has('X')).toBe(false);
    expect(nodeIds.has('Y')).toBe(false);
  });

  it('비정수 depth 는 내림 처리한다(2.9 → 2 hop)', () => {
    const { nodeIds } = getConnectedElements(chain, 'A', 2.9);
    expect(sorted(nodeIds)).toEqual(['A', 'B', 'C']);
  });

  it('가상 엣지를 통한 다중 hop 도 방향대로 따라간다', () => {
    const edges = [
      makeEdge({ id: 'e1', source: 'A', target: 'B', virtual: true }),
      makeEdge({ id: 'e2', source: 'B', target: 'C', virtual: true }),
    ];
    const { nodeIds, edgeIds } = getConnectedElements(edges, 'A', 2);
    expect(sorted(nodeIds)).toEqual(['A', 'B', 'C']);
    expect(sorted(edgeIds)).toEqual(['e1', 'e2']);
  });
});

describe('getConnectedElements — depth=Infinity(전체)', () => {
  // A → B → C → D 직선 체인.
  const chain = [
    makeEdge({ id: 'e1', source: 'A', target: 'B' }),
    makeEdge({ id: 'e2', source: 'B', target: 'C' }),
    makeEdge({ id: 'e3', source: 'C', target: 'D' }),
  ];

  it('체인 시작 A 를 depth=Infinity 로 선택하면 하류 전체 {A,B,C,D} 를 담는다', () => {
    const { nodeIds, edgeIds } = getConnectedElements(
      chain,
      'A',
      Number.POSITIVE_INFINITY,
    );
    expect(sorted(nodeIds)).toEqual(['A', 'B', 'C', 'D']);
    expect(sorted(edgeIds)).toEqual(['e1', 'e2', 'e3']);
  });

  it('중간 노드 B 를 depth=Infinity 로 선택하면 상류/하류 전체 체인을 담는다', () => {
    const { nodeIds, edgeIds } = getConnectedElements(
      chain,
      'B',
      Number.POSITIVE_INFINITY,
    );
    expect(sorted(nodeIds)).toEqual(['A', 'B', 'C', 'D']);
    expect(sorted(edgeIds)).toEqual(['e1', 'e2', 'e3']);
  });

  it('depth=Infinity 도 형제(다른 컴포넌트) 는 포함하지 않는다', () => {
    const edges = [
      ...chain,
      makeEdge({ id: 'x1', source: 'X', target: 'Y' }),
    ];
    const { nodeIds } = getConnectedElements(
      edges,
      'A',
      Number.POSITIVE_INFINITY,
    );
    expect(sorted(nodeIds)).toEqual(['A', 'B', 'C', 'D']);
    expect(nodeIds.has('X')).toBe(false);
    expect(nodeIds.has('Y')).toBe(false);
  });

  it('depth=Infinity 도 사이클에서 무한 루프 없이 종료한다', () => {
    const cycle = [
      makeEdge({ id: 'e1', source: 'A', target: 'B' }),
      makeEdge({ id: 'e2', source: 'B', target: 'C' }),
      makeEdge({ id: 'e3', source: 'C', target: 'A' }),
    ];
    const { nodeIds, edgeIds } = getConnectedElements(
      cycle,
      'A',
      Number.POSITIVE_INFINITY,
    );
    expect(sorted(nodeIds)).toEqual(['A', 'B', 'C']);
    expect(sorted(edgeIds)).toEqual(['e1', 'e2', 'e3']);
  });
});

describe('getConnectedElements — 사이클', () => {
  it('하류 사이클이 있어도 무한 루프 없이 종료한다', () => {
    // 삼각형 A→B→C→A.
    const cycle = [
      makeEdge({ id: 'e1', source: 'A', target: 'B' }),
      makeEdge({ id: 'e2', source: 'B', target: 'C' }),
      makeEdge({ id: 'e3', source: 'C', target: 'A' }),
    ];
    const { nodeIds, edgeIds } = getConnectedElements(cycle, 'A', 99);
    // 하류로 B, C 도달. 상류로도 C(C→A), B(B→C... 상류는 target→source) 도달.
    expect(sorted(nodeIds)).toEqual(['A', 'B', 'C']);
    // 모든 엣지가 어느 한 방향에서든 따라가진다.
    expect(sorted(edgeIds)).toEqual(['e1', 'e2', 'e3']);
  });

  it('사이클 엣지여도 따라간 엣지(edgeIds) 로 기록된다(하류만 1-hop)', () => {
    const cycle = [
      makeEdge({ id: 'e1', source: 'A', target: 'B' }),
      makeEdge({ id: 'e2', source: 'B', target: 'A' }),
    ];
    // A 선택 depth=1: 하류 e1(→B), 상류 e2(B→A 의 source B). nodeIds {A,B}.
    const { nodeIds, edgeIds } = getConnectedElements(cycle, 'A', 1);
    expect(sorted(nodeIds)).toEqual(['A', 'B']);
    expect(sorted(edgeIds)).toEqual(['e1', 'e2']);
  });
});

describe('getConnectedNodeIds — thin wrapper', () => {
  it('getConnectedElements 의 nodeIds 와 동일한 결과를 반환한다', () => {
    const edges = [
      makeEdge({ id: 'e1', source: 'A', target: 'B' }),
      makeEdge({ id: 'e2', source: 'X', target: 'A' }),
    ];
    const viaWrapper = getConnectedNodeIds(edges, 'A', 1);
    const viaFull = getConnectedElements(edges, 'A', 1).nodeIds;
    expect(sorted(viaWrapper)).toEqual(sorted(viaFull));
    expect(sorted(viaWrapper)).toEqual(['A', 'B', 'X']);
  });

  it('null 선택 시 빈 집합', () => {
    expect(getConnectedNodeIds([], null).size).toBe(0);
  });
});
