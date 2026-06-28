// SPEC-SUBFLOW-001 그룹 C: flow-node 참조 플로우 포트 비정규화 유틸 테스트.

import { describe, expect, it, vi, beforeEach } from 'vitest';

import type { FlowInfo } from '@/types/flow';

// 원격 포트 해석은 매니저 로컬 GET 이 아니라 대상 노드의 flow READ 프록시
// (getRemoteFlow) 에서 정의를 가져와야 한다(SPEC-REMOTE-001). getFlow 와
// getRemoteFlow 를 모두 스파이로 격리해 어느 경로를 탔는지 검증한다.
const getFlowMock = vi.fn<(id: string) => Promise<FlowInfo>>();
const getRemoteFlowMock = vi.fn<(instanceId: string, flowId: string) => Promise<FlowInfo>>();
vi.mock('@/services/api/flowService', () => ({
  getFlow: (id: string) => getFlowMock(id),
}));
vi.mock('@/services/api/remoteService', () => ({
  getRemoteFlow: (instanceId: string, flowId: string) =>
    getRemoteFlowMock(instanceId, flowId),
}));

import {
  buildRemoteFlowRef,
  extractFlowNodePorts,
  extractPortNames,
  parseRemoteFlowRef,
  resolveFlowNodePorts,
  resolveRemoteFlowNodePorts,
} from './subflowPorts';

beforeEach(() => {
  getFlowMock.mockReset();
  getRemoteFlowMock.mockReset();
});

describe('extractPortNames — 플로우 레벨 포트 맵 → 이름 배열', () => {
  it('포트 맵 배열에서 name 만 추출한다', () => {
    const names = extractPortNames([
      { id: 'p1', name: 'in1', direction: 'input' },
      { id: 'p2', name: 'in2', direction: 'input' },
    ]);
    expect(names).toEqual(['in1', 'in2']);
  });

  it('빈 이름/공백/중복/비문자열을 걸러내고 trim 한다', () => {
    const names = extractPortNames([
      { id: 'p1', name: 'in1' },
      { id: 'p2', name: '' },
      { id: 'p3', name: '   ' },
      { id: 'p4', name: 'in1' },
      { id: 'p5', name: '  in2  ' },
      { id: 'p6', name: 42 },
      { id: 'p7' },
    ]);
    expect(names).toEqual(['in1', 'in2']);
  });

  it('배열이 아니면 빈 배열을 반환한다', () => {
    expect(extractPortNames(undefined)).toEqual([]);
    expect(extractPortNames(null)).toEqual([]);
    expect(extractPortNames('in1')).toEqual([]);
    expect(extractPortNames({})).toEqual([]);
  });
});

describe('extractFlowNodePorts — 플로우 상세 → flow-node 표시 포트', () => {
  /** config.inputs/outputs 가 있는 플로우 상세 픽스처. */
  function makeFlow(overrides?: Partial<FlowInfo>): FlowInfo {
    return {
      id: 'flow-abc',
      name: '온도 정규화 플로우',
      status: 'stored',
      node_count: 3,
      config: {
        nodes: [],
        edges: [],
        inputs: [
          { id: 'fi1', name: 'temp_in', direction: 'input' },
          { id: 'fi2', name: 'unit_in', direction: 'input' },
        ],
        outputs: [{ id: 'fo1', name: 'celsius_out', direction: 'output' }],
      },
      ...overrides,
    };
  }

  it('config.inputs/outputs 에서 포트 이름과 flow_name 을 추출한다 (REQ-SUBFLOW-A07)', () => {
    const resolved = extractFlowNodePorts(makeFlow());
    expect(resolved.input_ports).toEqual(['temp_in', 'unit_in']);
    expect(resolved.output_ports).toEqual(['celsius_out']);
    expect(resolved.flow_name).toBe('온도 정규화 플로우');
  });

  it('포트가 없는 플로우는 빈 배열을 반환한다', () => {
    const resolved = extractFlowNodePorts(
      makeFlow({ config: { nodes: [], edges: [], inputs: [], outputs: [] } }),
    );
    expect(resolved.input_ports).toEqual([]);
    expect(resolved.output_ports).toEqual([]);
    expect(resolved.flow_name).toBe('온도 정규화 플로우');
  });

  it('config 가 없어도 안전하게 빈 포트를 반환한다', () => {
    const resolved = extractFlowNodePorts(
      makeFlow({ config: undefined }),
    );
    expect(resolved.input_ports).toEqual([]);
    expect(resolved.output_ports).toEqual([]);
  });
});

describe('buildRemoteFlowRef / parseRemoteFlowRef — remote:// 정규화 참조 (그룹 RU)', () => {
  it('buildRemoteFlowRef 는 remote://{instanceId}/{flowId} 를 만든다 (백엔드 규약 동형)', () => {
    expect(buildRemoteFlowRef('node-a', 'flow-1')).toBe('remote://node-a/flow-1');
  });

  it('parseRemoteFlowRef 는 정규화 참조를 첫 / 에서 분할해 파싱한다', () => {
    expect(parseRemoteFlowRef('remote://node-a/flow-1')).toEqual({
      instanceId: 'node-a',
      flowId: 'flow-1',
    });
  });

  it('build → parse 라운드트립이 동일 값을 보존한다', () => {
    const ref = parseRemoteFlowRef(buildRemoteFlowRef('edge-01', 'abc-123'));
    expect(ref).toEqual({ instanceId: 'edge-01', flowId: 'abc-123' });
  });

  it('flowId 에 / 가 포함되면 첫 구분자 기준으로만 분할한다(나머지는 flowId)', () => {
    expect(parseRemoteFlowRef('remote://node-a/grp/flow-1')).toEqual({
      instanceId: 'node-a',
      flowId: 'grp/flow-1',
    });
  });

  it('스킴이 없는 평문 id 는 null(로컬 참조로 처리)', () => {
    expect(parseRemoteFlowRef('flow-1')).toBeNull();
    expect(parseRemoteFlowRef('')).toBeNull();
  });

  it('instanceId 또는 flowId 가 비면 null', () => {
    expect(parseRemoteFlowRef('remote://node-a/')).toBeNull();
    expect(parseRemoteFlowRef('remote:///flow-1')).toBeNull();
    expect(parseRemoteFlowRef('remote://node-a')).toBeNull();
    expect(parseRemoteFlowRef('remote://')).toBeNull();
  });
});

describe('resolveFlowNodePorts / resolveRemoteFlowNodePorts — 타깃별 정의 소스', () => {
  const remoteFlow: FlowInfo = {
    id: 'rf-1',
    name: '노드 플로우',
    status: 'running',
    node_count: 1,
    config: {
      inputs: [{ id: 'i1', name: 'temp_in', direction: 'input' }],
      outputs: [{ id: 'o1', name: 'out1', direction: 'output' }],
    },
  };

  it('로컬: getFlow(매니저 로컬) 에서 포트를 해석한다', async () => {
    getFlowMock.mockResolvedValue(remoteFlow);
    const resolved = await resolveFlowNodePorts('rf-1');
    expect(getFlowMock).toHaveBeenCalledWith('rf-1');
    expect(getRemoteFlowMock).not.toHaveBeenCalled();
    expect(resolved.input_ports).toEqual(['temp_in']);
    expect(resolved.output_ports).toEqual(['out1']);
  });

  it('원격: getRemoteFlow(대상 노드 프록시) 에서 포트를 해석한다 (로컬 getFlow 미호출)', async () => {
    getRemoteFlowMock.mockResolvedValue(remoteFlow);
    const resolved = await resolveRemoteFlowNodePorts('node-a', 'rf-1');
    expect(getRemoteFlowMock).toHaveBeenCalledWith('node-a', 'rf-1');
    expect(getFlowMock).not.toHaveBeenCalled();
    expect(resolved.input_ports).toEqual(['temp_in']);
    expect(resolved.output_ports).toEqual(['out1']);
    expect(resolved.flow_name).toBe('노드 플로우');
  });
});
