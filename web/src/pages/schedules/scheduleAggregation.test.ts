// SPEC-SCHEDULE-VIEW-001 M5 — scheduleAggregation 순수 함수.
// 교차-플로우 노드 추출(AC-7/AC-18) / agent 그룹키 + 미지정 버킷(AC-8) / origin 보존.

import { describe, it, expect } from 'vitest';

import type { FlowInfo } from '@/types/flow';

import {
  collectTriggerNodeEntries,
  deriveDownstreamAgents,
  groupEntriesByAgent,
  nodeLevelAgent,
  resolveScheduleAgent,
  type ScheduleNodeEntry,
} from './scheduleAggregation';

function flow(id: string, name: string, nodes: unknown[]): FlowInfo {
  return { id, name, status: 'stored', node_count: nodes.length, config: { nodes } };
}

/** nodes + edges 를 갖는 플로우(엣지 유도 검증용). */
function wiredFlow(nodes: unknown[], edges: unknown[]): FlowInfo {
  return { id: 'f', name: 'F', status: 'stored', node_count: nodes.length, config: { nodes, edges } };
}

describe('collectTriggerNodeEntries — 플로우 정의에서 trigger 노드 추출', () => {
  it('nodeType==="trigger" 노드의 schedules/baseline/origin 을 추출한다', () => {
    const f = flow('flow-1', '플로우 1', [
      { id: 't1', data: { nodeType: 'trigger', label: '트리거 1', agentId: 'ag-x', schedules: [{ type: 'interval', value: '1s', agent_id: 'ag-x' }] } },
      { id: 'n2', data: { nodeType: 'output' } }, // 비-trigger 제외
    ]);
    const entries = collectTriggerNodeEntries(f);
    expect(entries).toHaveLength(1);
    const e = entries[0]!;
    expect(e.flowId).toBe('flow-1');
    expect(e.nodeId).toBe('t1');
    expect(e.nodeName).toBe('트리거 1');
    expect(e.schedules).toHaveLength(1);
    expect(e.nodeConfig.agentId).toBe('ag-x'); // baseline 보존
  });

  it('schedules 배열만 있어도(nodeType 부재) 스케줄 보유 노드로 본다', () => {
    const f = flow('flow-2', 'F2', [{ id: 'x', data: { schedules: [] } }]);
    const entries = collectTriggerNodeEntries(f);
    expect(entries).toHaveLength(1);
    expect(entries[0]!.schedules).toEqual([]);
    expect(entries[0]!.nodeName).toBe('x'); // label 부재 → id 폴백
  });

  it('config 없음/노드 배열 없음이면 빈 결과', () => {
    expect(collectTriggerNodeEntries({ id: 'a', name: 'a', status: 'stored', node_count: 0 })).toEqual([]);
  });
});

describe('resolveScheduleAgent / nodeLevelAgent — 그룹 키 결정(명시 필드)', () => {
  it('schedule.agent_id 우선, 없으면 nodeConfig.agentId, 둘 다 없으면 null', () => {
    expect(resolveScheduleAgent({ agent_id: 'ag-s' }, { agentId: 'ag-n' })).toBe('ag-s');
    expect(resolveScheduleAgent({}, { agentId: 'ag-n' })).toBe('ag-n');
    expect(resolveScheduleAgent({}, {})).toBeNull();
    expect(nodeLevelAgent({ agentId: 'ag-n' })).toBe('ag-n');
    expect(nodeLevelAgent({})).toBeNull();
  });
});

describe('groupEntriesByAgent — agent 그룹 + 미지정 버킷(AC-7/AC-8)', () => {
  const mkEntry = (
    flowId: string,
    nodeId: string,
    schedules: Record<string, unknown>[],
    nodeConfig: Record<string, unknown> = {},
  ): ScheduleNodeEntry => ({
    flowId,
    flowName: flowId,
    nodeId,
    nodeName: nodeId,
    nodeConfig,
    schedules,
    derivedAgentIds: [],
  });

  it('여러 플로우의 스케줄을 declared agent_id 로 묶고, agent 없는 스케줄은 미지정(null) 버킷', () => {
    const entries = [
      mkEntry('flow-1', 't1', [{ agent_id: 'ag-x' }, { agent_id: 'ag-x' }]),
      mkEntry('flow-2', 't2', [{ /* agent 없음 */ }]),
      mkEntry('flow-2', 't3', [{ agent_id: 'ag-y' }]),
    ];
    const groups = groupEntriesByAgent(entries);
    const byKey = new Map(groups.map((g) => [g.key, g]));

    expect(byKey.has('ag-x')).toBe(true);
    expect(byKey.has('ag-y')).toBe(true);
    expect(byKey.has(null)).toBe(true); // 미지정 버킷 존재(드롭 없음)

    // 교차 플로우: ag-x=flow-1/t1, ag-y=flow-2/t3, 미지정=flow-2/t2
    expect(byKey.get('ag-x')!.nodes.map((n) => n.nodeId)).toEqual(['t1']);
    expect(byKey.get(null)!.nodes.map((n) => `${n.flowId}:${n.nodeId}`)).toEqual(['flow-2:t2']);
  });

  it('스케줄이 없는 노드는 node-level agentId 로(없으면 미지정) 배치해 추가 진입점을 남긴다', () => {
    const groups = groupEntriesByAgent([mkEntry('f', 'empty', [], { agentId: 'ag-z' })]);
    expect(groups).toHaveLength(1);
    expect(groups[0]!.key).toBe('ag-z');
    expect(groups[0]!.nodes[0]!.nodeId).toBe('empty');
  });

  it('혼합 agent 노드는 각 agent 그룹에 편입된다(드롭 없음)', () => {
    const groups = groupEntriesByAgent([
      mkEntry('f', 't', [{ agent_id: 'ag-a' }, { agent_id: 'ag-b' }]),
    ]);
    const keys = groups.map((g) => g.key).sort();
    expect(keys).toEqual(['ag-a', 'ag-b']);
  });
});

describe('deriveDownstreamAgents — 엣지 하류 제어 노드에서 실행 에이전트 유도', () => {
  it('단일 하류 제어 노드 → [agent_ref]', () => {
    const f = wiredFlow(
      [
        { id: 't', data: { nodeType: 'trigger' } },
        { id: 'c', data: { nodeType: 'xsfm-control', agent_ref: 'ag-1' } },
      ],
      [{ source: 't', target: 'c' }],
    );
    expect(deriveDownstreamAgents(f, 't')).toEqual(['ag-1']);
  });

  it('필터를 거친 다중 홉(trigger→filter→control) → [agent]', () => {
    const f = wiredFlow(
      [
        { id: 't', data: { nodeType: 'trigger' } },
        { id: 'flt', data: { nodeType: 'filter' } },
        { id: 'c', data: { nodeType: 'xsfm-control', agent_ref: 'ag-2' } },
      ],
      [
        { source: 't', target: 'flt', sourceHandle: 'out', targetHandle: 'in' },
        { source: 'flt', target: 'c' },
      ],
    );
    expect(deriveDownstreamAgents(f, 't')).toEqual(['ag-2']);
  });

  it('서로 다른 agent_ref 를 갖는 두 제어 노드로 팬아웃 → [a, b](모호)', () => {
    const f = wiredFlow(
      [
        { id: 't', data: { nodeType: 'trigger' } },
        { id: 'c1', data: { nodeType: 'xsfm-control', agent_ref: 'ag-a' } },
        { id: 'c2', data: { nodeType: 'samsung-hvacr01-control', agent_ref: 'ag-b' } },
      ],
      [
        { source: 't', target: 'c1' },
        { source: 't', target: 'c2' },
      ],
    );
    expect(deriveDownstreamAgents(f, 't').sort()).toEqual(['ag-a', 'ag-b']);
  });

  it('미배선 trigger(하류 제어 노드 없음) → []', () => {
    const f = wiredFlow(
      [
        { id: 't', data: { nodeType: 'trigger' } },
        { id: 'o', data: { nodeType: 'output' } },
      ],
      [{ source: 't', target: 'o' }],
    );
    expect(deriveDownstreamAgents(f, 't')).toEqual([]);
  });

  it('agent_ref 부재 시 agent_id 로 폴백, 동일 agent 중복 제거', () => {
    const f = wiredFlow(
      [
        { id: 't', data: { nodeType: 'trigger' } },
        { id: 'c1', data: { nodeType: 'xsfm-control', agent_id: 'ag-x' } },
        { id: 'c2', data: { nodeType: 'xsfm-control', agent_ref: 'ag-x' } },
      ],
      [
        { source: 't', target: 'c1' },
        { source: 'c1', target: 'c2' },
      ],
    );
    expect(deriveDownstreamAgents(f, 't')).toEqual(['ag-x']);
  });

  it('잘못된(누락 source/target) 엣지·사이클을 안전하게 처리한다(예외 없음)', () => {
    const f = wiredFlow(
      [
        { id: 't', data: { nodeType: 'trigger' } },
        { id: 'c', data: { nodeType: 'xsfm-control', agent_ref: 'ag-1' } },
      ],
      [
        { source: 't' }, // target 누락 → 무시
        { target: 'c' }, // source 누락 → 무시
        { source: 't', target: 'c' },
        { source: 'c', target: 't' }, // 사이클 → 무한 루프 방지
        null,
      ],
    );
    expect(deriveDownstreamAgents(f, 't')).toEqual(['ag-1']);
  });

  it('config/edges 부재면 빈 결과', () => {
    expect(
      deriveDownstreamAgents({ id: 'a', name: 'a', status: 'stored', node_count: 0 }, 't'),
    ).toEqual([]);
  });

  it('collectTriggerNodeEntries 가 trigger 노드마다 derivedAgentIds 를 채운다', () => {
    const f = wiredFlow(
      [
        { id: 't', data: { nodeType: 'trigger', schedules: [] } },
        { id: 'c', data: { nodeType: 'xsfm-control', agent_ref: 'ag-1' } },
      ],
      [{ source: 't', target: 'c' }],
    );
    const entries = collectTriggerNodeEntries(f);
    expect(entries).toHaveLength(1);
    expect(entries[0]!.derivedAgentIds).toEqual(['ag-1']);
  });
});
