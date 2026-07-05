// 서브플로우 런타임 통계 네임스페이스 유틸 테스트.
// 접두사 생성(백엔드 규약 복제), 부모 뷰 집계(Fix 1), 서브플로우 단독 뷰 병합(Fix 2)을 검증한다.

import { describe, expect, it } from 'vitest';

import type { FlowNodeInfo, PortInfo, SubflowNodeStat } from '@/types/flow';

import {
  aggregateFlowNodeStats,
  buildRuntimeStatsMap,
  mergeSubflowStats,
  subflowNodeIdPrefix,
} from './subflowNamespace';

/** 테스트용 PortInfo 빌더(불필요 필드 기본값). */
function port(partial: Partial<PortInfo> & { name: string; direction: string }): PortInfo {
  return {
    id: partial.name,
    name: partial.name,
    direction: partial.direction,
    connected: partial.connected ?? false,
    messages: partial.messages ?? 0,
    delivered: partial.delivered ?? 0,
    throughput: partial.throughput ?? '0.000',
    active_for: partial.active_for ?? '',
  };
}

/** 테스트용 FlowNodeInfo 빌더. */
function node(node_id: string, state: string, ports: PortInfo[]): FlowNodeInfo {
  return { node_id, name: node_id, type: 'test', state, ports };
}

describe('subflowNodeIdPrefix — 백엔드 규약 복제', () => {
  it('"subflow_<flowNodeId>_" 형태의 접두사를 만든다', () => {
    expect(subflowNodeIdPrefix('F')).toBe('subflow_F_');
    expect(subflowNodeIdPrefix('flow-node-1')).toBe('subflow_flow-node-1_');
  });

  it('빈 flowNodeId 도 토큰 + "_" 만으로 결합한다(백엔드 SubflowNodeIDPrefix 와 동형)', () => {
    expect(subflowNodeIdPrefix('')).toBe('subflow__');
  });

  it('중첩 접두사도 단순 문자열 결합으로 매칭 가능하다', () => {
    // 부모 F 안의 서브플로우 G 노드는 subflow_F_subflow_G_inner 로 평탄화된다.
    const nested = 'subflow_F_subflow_G_inner';
    expect(nested.startsWith(subflowNodeIdPrefix('F'))).toBe(true);
  });
});

describe('aggregateFlowNodeStats — Fix 1 부모 뷰 집계', () => {
  it('flow-node 의 subflow_<id>_* 자식 노드 통계를 모두 합산한다', () => {
    const runtimeNodes: FlowNodeInfo[] = [
      // flow-node "F" 의 내부 노드 두 개(네임스페이스).
      node('subflow_F_a', 'running', [
        port({ name: 'in', direction: 'input', messages: 10 }),
        port({ name: 'out', direction: 'output', messages: 8, delivered: 7 }),
      ]),
      node('subflow_F_b', 'running', [
        port({ name: 'out', direction: 'output', messages: 5, delivered: 5 }),
      ]),
      // 다른 flow-node "G" 의 내부 노드 — 합산에 포함되면 안 됨.
      node('subflow_G_x', 'running', [
        port({ name: 'out', direction: 'output', messages: 99, delivered: 99 }),
      ]),
      // 일반 노드 — 무관.
      node('plain', 'running', [port({ name: 'out', direction: 'output', messages: 1 })]),
    ];

    const agg = aggregateFlowNodeStats('F', runtimeNodes);
    expect(agg).toBeDefined();
    // input messages: 10. output messages: 8 + 5 = 13.
    expect(agg?.inMessages).toBe(10);
    expect(agg?.outMessages).toBe(13);
    // 포트 이름 단위 합산: out = messages 13 / delivered 12.
    const outPort = agg?.ports.find((p) => p.name === 'out' && p.direction === 'output');
    expect(outPort?.messages).toBe(13);
    expect(outPort?.delivered).toBe(12);
    const inPort = agg?.ports.find((p) => p.name === 'in' && p.direction === 'input');
    expect(inPort?.messages).toBe(10);
  });

  it('자식이 하나라도 running 이면 집계 state 는 running 이다', () => {
    const runtimeNodes: FlowNodeInfo[] = [
      node('subflow_F_a', 'stopped', [port({ name: 'out', direction: 'output' })]),
      node('subflow_F_b', 'running', [port({ name: 'out', direction: 'output' })]),
    ];
    expect(aggregateFlowNodeStats('F', runtimeNodes)?.state).toBe('running');
  });

  it('일치하는 자식이 없으면 undefined 를 반환한다(통계 미표시 유지)', () => {
    const runtimeNodes: FlowNodeInfo[] = [
      node('subflow_G_x', 'running', [port({ name: 'out', direction: 'output' })]),
      node('plain', 'running', []),
    ];
    expect(aggregateFlowNodeStats('F', runtimeNodes)).toBeUndefined();
  });

  it('접두사가 정확히 "subflow_F_" 인 노드만 매칭한다(부분 일치 오인 방지)', () => {
    const runtimeNodes: FlowNodeInfo[] = [
      // "subflow_FF_*" 는 flow-node "F" 의 자식이 아니다.
      node('subflow_FF_a', 'running', [port({ name: 'out', direction: 'output', messages: 7 })]),
    ];
    expect(aggregateFlowNodeStats('F', runtimeNodes)).toBeUndefined();
  });
});

describe('mergeSubflowStats — Fix 2 서브플로우 단독 뷰 병합', () => {
  it('own(getFlowNodes)이 노드를 갖지 않으면 subflow-stats 로 채운다', () => {
    // 서브플로우가 단독 배포되지 않아 own 은 비어 있고, 부모에서 역집계된 통계만 있다.
    const ownMap = buildRuntimeStatsMap([]);
    const subflowStats: SubflowNodeStat[] = [
      {
        node_id: 'inner1',
        processed: 5,
        errors: 0,
        ports: [
          port({ name: 'in', direction: 'input', messages: 5 }),
          port({ name: 'out', direction: 'output', messages: 5, delivered: 4 }),
        ],
      },
    ];

    const merged = mergeSubflowStats(ownMap, subflowStats);
    const inner1 = merged.inner1;
    expect(inner1).toBeDefined();
    expect(inner1?.inMessages).toBe(5);
    expect(inner1?.outMessages).toBe(5);
    const outPort = inner1?.ports.find((p) => p.name === 'out');
    expect(outPort?.delivered).toBe(4);
  });

  it('같은 노드 id 가 양쪽에 있으면 own 을 우선한다(own takes precedence)', () => {
    const ownNodes: FlowNodeInfo[] = [
      node('inner1', 'running', [port({ name: 'out', direction: 'output', messages: 100, delivered: 100 })]),
    ];
    const ownMap = buildRuntimeStatsMap(ownNodes);
    const subflowStats: SubflowNodeStat[] = [
      {
        node_id: 'inner1',
        processed: 5,
        errors: 0,
        ports: [port({ name: 'out', direction: 'output', messages: 5, delivered: 5 })],
      },
    ];

    const merged = mergeSubflowStats(ownMap, subflowStats);
    // own 의 100 이 유지되어야 한다(subflow-stats 의 5 로 덮어쓰지 않음).
    expect(merged.inner1?.outMessages).toBe(100);
  });

  it('own 과 subflow-stats 노드를 합집합으로 포함한다', () => {
    const ownNodes: FlowNodeInfo[] = [
      node('ownOnly', 'running', [port({ name: 'out', direction: 'output', messages: 1 })]),
    ];
    const ownMap = buildRuntimeStatsMap(ownNodes);
    const subflowStats: SubflowNodeStat[] = [
      { node_id: 'subflowOnly', processed: 2, errors: 0, ports: [] },
    ];

    const merged = mergeSubflowStats(ownMap, subflowStats);
    expect(Object.keys(merged).sort()).toEqual(['ownOnly', 'subflowOnly']);
  });
});
