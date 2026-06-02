// 가져오기 파서 테스트 (flow-management).
//
// 검증 대상:
//  - regenerateDefinitionIds: 노드/엣지 id 재생성 + source/target 재작성
//    - 같은 정의를 두 번 가져오면 노드/엣지 id 집합이 서로소(disjoint)
//    - 엣지가 여전히 동일한 논리적 source→target 노드를 연결
//    - sourceHandle/targetHandle(포트 이름) 은 변경되지 않음
//    - agent_ref(에이전트 이름 바인딩) 는 보존
//  - collectSecretFieldNames: export 힌트 + 스키마 sensitive 필드 합집합
//  - extractRequiredAgents: sensitive_fields 힌트 추출

import { describe, expect, it } from 'vitest';

import {
  regenerateDefinitionIds,
  collectSecretFieldNames,
  extractRequiredAgents,
  type RequiredAgent,
} from './importParser';

/** UUID v4 형식(대략) 검증용 정규식. */
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** 두 노드/엣지 사이를 잇는 wires 를 가진 샘플 플로우 정의. */
function sampleDefinition(): Record<string, unknown> {
  return {
    nodes: [
      { id: 'node-a', type: 'custom', data: { label: 'A' } },
      { id: 'node-b', type: 'custom', data: { label: 'B' }, agent_ref: { agent_name: 'mqtt-1', agent_id: 'srv-1' } },
    ],
    wires: [
      {
        id: 'xy-edge__node-aout-node-bin',
        source: 'node-a',
        target: 'node-b',
        sourceHandle: 'out',
        targetHandle: 'in',
        wire_type: 'simple',
      },
    ],
  };
}

describe('regenerateDefinitionIds', () => {
  it('모든 노드 id 를 새 UUID 로 교체한다', () => {
    const out = regenerateDefinitionIds(sampleDefinition());
    const nodes = out.nodes as Array<Record<string, unknown>>;
    expect(nodes).toHaveLength(2);
    for (const n of nodes) {
      expect(n.id).toMatch(UUID_RE);
    }
  });

  it('엣지 source/target 을 새 노드 id 로 재작성하고 엣지 id 를 UUID 로 교체한다', () => {
    const out = regenerateDefinitionIds(sampleDefinition());
    const nodes = out.nodes as Array<Record<string, unknown>>;
    const wires = out.wires as Array<Record<string, unknown>>;
    const newAId = nodes[0]!.id;
    const newBId = nodes[1]!.id;

    expect(wires).toHaveLength(1);
    const edge = wires[0]!;
    // 엣지 id 는 새 UUID (파생 xy-edge__ 가 아님)
    expect(edge.id).toMatch(UUID_RE);
    expect(String(edge.id)).not.toContain('xy-edge__');
    // source/target 은 재작성된 노드 id 를 가리킨다
    expect(edge.source).toBe(newAId);
    expect(edge.target).toBe(newBId);
    // 포트 핸들(이름) 은 변경되지 않는다
    expect(edge.sourceHandle).toBe('out');
    expect(edge.targetHandle).toBe('in');
  });

  it('agent_ref(에이전트 이름 바인딩) 는 보존한다', () => {
    const out = regenerateDefinitionIds(sampleDefinition());
    const nodes = out.nodes as Array<Record<string, unknown>>;
    const agentRef = nodes[1]!.agent_ref as Record<string, unknown>;
    expect(agentRef.agent_name).toBe('mqtt-1');
    expect(agentRef.agent_id).toBe('srv-1');
  });

  it('원본 정의를 변경하지 않는다(순수 함수)', () => {
    const def = sampleDefinition();
    regenerateDefinitionIds(def);
    const nodes = def.nodes as Array<Record<string, unknown>>;
    expect(nodes[0]!.id).toBe('node-a');
    expect(nodes[1]!.id).toBe('node-b');
  });

  it('같은 정의를 두 번 가져오면 노드/엣지 id 집합이 서로소이고 엣지는 동일 논리 연결을 유지한다', () => {
    const first = regenerateDefinitionIds(sampleDefinition());
    const second = regenerateDefinitionIds(sampleDefinition());

    const firstNodes = first.nodes as Array<Record<string, unknown>>;
    const secondNodes = second.nodes as Array<Record<string, unknown>>;
    const firstWires = first.wires as Array<Record<string, unknown>>;
    const secondWires = second.wires as Array<Record<string, unknown>>;

    // 노드 id 집합 서로소
    const firstNodeIds = new Set(firstNodes.map((n) => n.id as string));
    const secondNodeIds = new Set(secondNodes.map((n) => n.id as string));
    for (const id of secondNodeIds) {
      expect(firstNodeIds.has(id)).toBe(false);
    }

    // 엣지 id 집합 서로소
    const firstEdgeIds = new Set(firstWires.map((e) => e.id as string));
    const secondEdgeIds = new Set(secondWires.map((e) => e.id as string));
    for (const id of secondEdgeIds) {
      expect(firstEdgeIds.has(id)).toBe(false);
    }

    // 두 가져오기 모두 엣지는 첫 번째 노드 → 두 번째 노드를 연결한다
    // (논리적 source→target 보존). 핸들도 동일.
    const firstEdge = firstWires[0]!;
    expect(firstEdge.source).toBe(firstNodes[0]!.id);
    expect(firstEdge.target).toBe(firstNodes[1]!.id);

    const secondEdge = secondWires[0]!;
    expect(secondEdge.source).toBe(secondNodes[0]!.id);
    expect(secondEdge.target).toBe(secondNodes[1]!.id);
    expect(secondEdge.sourceHandle).toBe('out');
    expect(secondEdge.targetHandle).toBe('in');
  });

  it('edges 키(에디터 저장 포맷) 도 지원한다', () => {
    const def: Record<string, unknown> = {
      nodes: [
        { id: 'n1' },
        { id: 'n2' },
      ],
      edges: [
        { id: 'xy-edge__n1a-n2b', source: 'n1', target: 'n2', sourceHandle: 'a', targetHandle: 'b' },
      ],
    };
    const out = regenerateDefinitionIds(def);
    const nodes = out.nodes as Array<Record<string, unknown>>;
    const edges = out.edges as Array<Record<string, unknown>>;
    expect(edges[0]!.source).toBe(nodes[0]!.id);
    expect(edges[0]!.target).toBe(nodes[1]!.id);
    expect(edges[0]!.id).toMatch(UUID_RE);
  });

  it('nodes 가 없으면 그대로 반환한다', () => {
    const def: Record<string, unknown> = { foo: 'bar' };
    const out = regenerateDefinitionIds(def);
    expect(out.foo).toBe('bar');
  });
});

describe('collectSecretFieldNames', () => {
  it('export 힌트(sensitiveFields) 를 모두 포함한다', () => {
    const agent: RequiredAgent = {
      name: 'mqtt-1',
      type: 'mqtt-client',
      config: {},
      sensitiveFields: ['username', 'password'],
    };
    const result = collectSecretFieldNames(agent, []);
    expect(result).toEqual(['username', 'password']);
  });

  it('스키마 sensitive 필드 중 값이 비어 있는 것만 포함한다', () => {
    const agent: RequiredAgent = {
      name: 'influx-1',
      type: 'influxdb',
      config: { token: 'existing-token' }, // token 은 값이 있으므로 제외
    };
    // 스키마 sensitive 필드: token(값 있음 -> 제외)
    const result = collectSecretFieldNames(agent, ['token']);
    expect(result).toEqual([]);
  });

  it('값이 비어 있는 스키마 sensitive 필드는 포함한다', () => {
    const agent: RequiredAgent = {
      name: 'mqtt-1',
      type: 'mqtt-client',
      config: { username: '', password: undefined },
    };
    const result = collectSecretFieldNames(agent, ['username', 'password']);
    expect(result).toEqual(['username', 'password']);
  });

  it('힌트와 스키마 필드를 합집합으로 처리하고 중복을 제거한다', () => {
    const agent: RequiredAgent = {
      name: 'mqtt-1',
      type: 'mqtt-client',
      config: {},
      sensitiveFields: ['password'],
    };
    const result = collectSecretFieldNames(agent, ['username', 'password']);
    // password(힌트) + username(스키마, 빈 값) — password 는 중복 제거
    expect(result).toEqual(['password', 'username']);
  });
});

describe('extractRequiredAgents — sensitive_fields 힌트', () => {
  it('required_agents 의 sensitive_fields 를 추출한다', () => {
    const flowData: Record<string, unknown> = {
      name: 'flow-1',
      required_agents: [
        { name: 'mqtt-1', type: 'mqtt-client', sensitive_fields: ['username', 'password'] },
        { name: 'logger-1', type: 'logger' },
      ],
    };
    const result = extractRequiredAgents(flowData);
    expect(result).toHaveLength(2);
    expect(result[0]!.sensitiveFields).toEqual(['username', 'password']);
    expect(result[1]!.sensitiveFields).toBeUndefined();
  });
});
