// SPEC-SUBFLOW-001 그룹 C: flow-node 참조 플로우 포트 비정규화 유틸 테스트.

import { describe, expect, it } from 'vitest';

import type { FlowInfo } from '@/types/flow';
import { extractFlowNodePorts, extractPortNames } from './subflowPorts';

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
