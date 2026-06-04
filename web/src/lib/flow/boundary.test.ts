// SPEC-SUBFLOW-001 그룹 A/B: 플로우 경계 포트 합성 유틸 테스트.

import { describe, expect, it } from 'vitest';
import type { Edge, Node } from '@xyflow/react';

import {
  FLOW_BOUNDARY_NODE_TYPE,
  FLOW_INPUT_BOUNDARY_ID,
  FLOW_OUTPUT_BOUNDARY_ID,
  buildBoundaryNodes,
  edgeUsesBoundaryPort,
  isBoundaryNode,
  isBoundaryNodeId,
  nextFlowPortName,
  parseFlowPorts,
  parseFlowPortsFromConfig,
  realNodesOnly,
  serializeFlowDefinition,
  withBoundaryNodes,
  type FlowPortDef,
} from './boundary';

const realNode = (id: string, x = 0, y = 0): Node => ({
  id,
  type: 'custom',
  position: { x, y },
  data: { label: id },
});

describe('boundary - 센티넬 식별', () => {
  it('isBoundaryNodeId 는 두 센티넬만 true 로 판정한다', () => {
    expect(isBoundaryNodeId(FLOW_INPUT_BOUNDARY_ID)).toBe(true);
    expect(isBoundaryNodeId(FLOW_OUTPUT_BOUNDARY_ID)).toBe(true);
    expect(isBoundaryNodeId('n1')).toBe(false);
    expect(isBoundaryNodeId(null)).toBe(false);
    expect(isBoundaryNodeId(undefined)).toBe(false);
  });

  it('isBoundaryNode 는 노드 id 로 경계 노드를 판정한다', () => {
    expect(isBoundaryNode(realNode(FLOW_INPUT_BOUNDARY_ID))).toBe(true);
    expect(isBoundaryNode(realNode('n1'))).toBe(false);
  });

  it('realNodesOnly 는 경계 노드를 제외한다', () => {
    const nodes = [
      realNode('n1'),
      realNode(FLOW_INPUT_BOUNDARY_ID),
      realNode(FLOW_OUTPUT_BOUNDARY_ID),
      realNode('n2'),
    ];
    const real = realNodesOnly(nodes);
    expect(real.map((n) => n.id)).toEqual(['n1', 'n2']);
  });
});

describe('boundary - 포트 파싱', () => {
  it('parseFlowPorts 는 {id,name,direction} 배열에서 id/name 을 추출한다', () => {
    const ports = parseFlowPorts([
      { id: 'p1', name: 'in1', direction: 'input' },
      { id: 'p2', name: 'in2', direction: 'input' },
    ]);
    expect(ports).toEqual([
      { id: 'p1', name: 'in1' },
      { id: 'p2', name: 'in2' },
    ]);
  });

  it('빈 이름/비문자열/중복 id 는 걸러내고, id 없으면 이름 기반 임시 id 를 부여한다', () => {
    const ports = parseFlowPorts([
      { name: 'a' },
      { id: 'x', name: '' },
      { id: 'dup', name: 'b' },
      { id: 'dup', name: 'c' },
      'not-an-object',
    ]);
    expect(ports).toEqual([
      { id: 'port-a', name: 'a' },
      { id: 'dup', name: 'b' },
    ]);
  });

  it('parseFlowPortsFromConfig 는 config.inputs / config.outputs 를 읽는다', () => {
    const { flowInputs, flowOutputs } = parseFlowPortsFromConfig({
      inputs: [{ id: 'i1', name: 'in1', direction: 'input' }],
      outputs: [{ id: 'o1', name: 'out1', direction: 'output' }],
    });
    expect(flowInputs).toEqual([{ id: 'i1', name: 'in1' }]);
    expect(flowOutputs).toEqual([{ id: 'o1', name: 'out1' }]);
  });

  it('inputs/outputs 가 없으면 빈 배열을 반환한다', () => {
    expect(parseFlowPortsFromConfig({})).toEqual({
      flowInputs: [],
      flowOutputs: [],
    });
    expect(parseFlowPortsFromConfig(undefined)).toEqual({
      flowInputs: [],
      flowOutputs: [],
    });
  });
});

describe('boundary - 경계 노드 생성', () => {
  const inputs: FlowPortDef[] = [
    { id: 'i1', name: 'in1' },
    { id: 'i2', name: 'in2' },
  ];
  const outputs: FlowPortDef[] = [{ id: 'o1', name: 'out1' }];

  it('두 경계 노드를 센티넬 id/타입으로 만들고 핸들 포트를 포함한다', () => {
    const [inputNode, outputNode] = buildBoundaryNodes(
      inputs,
      outputs,
      [realNode('n1')],
    );
    expect(inputNode.id).toBe(FLOW_INPUT_BOUNDARY_ID);
    expect(inputNode.type).toBe(FLOW_BOUNDARY_NODE_TYPE);
    expect(inputNode.data).toMatchObject({
      direction: 'input',
      ports: ['in1', 'in2'],
    });
    expect(outputNode.id).toBe(FLOW_OUTPUT_BOUNDARY_ID);
    expect(outputNode.data).toMatchObject({
      direction: 'output',
      ports: ['out1'],
    });
  });

  it('경계 노드는 비-드래그/비-선택/비-삭제이다', () => {
    const [inputNode] = buildBoundaryNodes(inputs, outputs, []);
    expect(inputNode.draggable).toBe(false);
    expect(inputNode.selectable).toBe(false);
    expect(inputNode.deletable).toBe(false);
  });

  it('이전 경계 노드 위치를 보존한다(포트 편집 시 튀지 않음)', () => {
    const prev: Node[] = [
      {
        id: FLOW_INPUT_BOUNDARY_ID,
        type: FLOW_BOUNDARY_NODE_TYPE,
        position: { x: -999, y: 123 },
        data: { direction: 'input', ports: [] },
      },
    ];
    const [inputNode] = buildBoundaryNodes(inputs, outputs, [realNode('n1', 50, 50)], prev);
    expect(inputNode.position).toEqual({ x: -999, y: 123 });
  });

  it('withBoundaryNodes 는 실제 노드 + 경계 노드 2개를 반환한다', () => {
    const result = withBoundaryNodes([realNode('n1')], inputs, outputs);
    expect(result).toHaveLength(3);
    expect(result.map((n) => n.id)).toEqual([
      'n1',
      FLOW_INPUT_BOUNDARY_ID,
      FLOW_OUTPUT_BOUNDARY_ID,
    ]);
  });

  it('withBoundaryNodes 는 입력으로 들어온 경계 노드를 중복 추가하지 않는다', () => {
    const withExisting: Node[] = [
      realNode('n1'),
      {
        id: FLOW_INPUT_BOUNDARY_ID,
        type: FLOW_BOUNDARY_NODE_TYPE,
        position: { x: 0, y: 0 },
        data: { direction: 'input', ports: [] },
      },
    ];
    const result = withBoundaryNodes(withExisting, inputs, outputs);
    const inputCount = result.filter(
      (n) => n.id === FLOW_INPUT_BOUNDARY_ID,
    ).length;
    expect(inputCount).toBe(1);
  });
});

describe('boundary - 경계 와이어 식별', () => {
  it('입력 경계 와이어를 source/sourceHandle 로 판정한다', () => {
    const edge: Edge = {
      id: 'e1',
      source: FLOW_INPUT_BOUNDARY_ID,
      sourceHandle: 'in1',
      target: 'n1',
      targetHandle: 'in',
    };
    expect(edgeUsesBoundaryPort(edge, 'input', 'in1')).toBe(true);
    expect(edgeUsesBoundaryPort(edge, 'input', 'in2')).toBe(false);
    expect(edgeUsesBoundaryPort(edge, 'output', 'in1')).toBe(false);
  });

  it('출력 경계 와이어를 target/targetHandle 로 판정한다', () => {
    const edge: Edge = {
      id: 'e2',
      source: 'n1',
      sourceHandle: 'out',
      target: FLOW_OUTPUT_BOUNDARY_ID,
      targetHandle: 'out1',
    };
    expect(edgeUsesBoundaryPort(edge, 'output', 'out1')).toBe(true);
    expect(edgeUsesBoundaryPort(edge, 'output', 'out2')).toBe(false);
  });
});

describe('boundary - 직렬화(저장)', () => {
  const inputs: FlowPortDef[] = [{ id: 'i1', name: 'in1' }];
  const outputs: FlowPortDef[] = [{ id: 'o1', name: 'out1' }];

  const nodes: Node[] = [
    realNode('n1'),
    {
      id: FLOW_INPUT_BOUNDARY_ID,
      type: FLOW_BOUNDARY_NODE_TYPE,
      position: { x: 0, y: 0 },
      data: { direction: 'input', ports: ['in1'] },
    },
    {
      id: FLOW_OUTPUT_BOUNDARY_ID,
      type: FLOW_BOUNDARY_NODE_TYPE,
      position: { x: 0, y: 0 },
      data: { direction: 'output', ports: ['out1'] },
    },
  ];

  const edges: Edge[] = [
    {
      id: 'e1',
      source: FLOW_INPUT_BOUNDARY_ID,
      sourceHandle: 'in1',
      target: 'n1',
      targetHandle: 'in',
    },
    {
      id: 'e2',
      source: 'n1',
      sourceHandle: 'out',
      target: FLOW_OUTPUT_BOUNDARY_ID,
      targetHandle: 'out1',
    },
  ];

  it('합성 경계 노드를 nodes 에서 제외한다', () => {
    const def = serializeFlowDefinition(nodes, edges, inputs, outputs);
    const outNodes = def.nodes as Node[];
    expect(outNodes.map((n) => n.id)).toEqual(['n1']);
  });

  it('센티넬 경계 와이어를 포함한 모든 엣지를 보존한다', () => {
    const def = serializeFlowDefinition(nodes, edges, inputs, outputs);
    expect((def.edges as Edge[]).map((e) => e.id)).toEqual(['e1', 'e2']);
  });

  it('플로우 레벨 포트를 정의 최상위 inputs/outputs 로 {id,name,direction} 기록한다', () => {
    const def = serializeFlowDefinition(nodes, edges, inputs, outputs);
    expect(def.inputs).toEqual([
      { id: 'i1', name: 'in1', direction: 'input' },
    ]);
    expect(def.outputs).toEqual([
      { id: 'o1', name: 'out1', direction: 'output' },
    ]);
  });

  it('round-trip: 직렬화한 inputs/outputs 를 다시 파싱하면 동일 포트가 복원된다', () => {
    const def = serializeFlowDefinition(nodes, edges, inputs, outputs);
    const { flowInputs, flowOutputs } = parseFlowPortsFromConfig(def);
    expect(flowInputs).toEqual(inputs);
    expect(flowOutputs).toEqual(outputs);
  });
});

describe('boundary - 기본 포트 이름', () => {
  it('입력은 in1, in2 … 로 증가한다', () => {
    expect(nextFlowPortName('input', [])).toBe('in1');
    expect(nextFlowPortName('input', ['in1'])).toBe('in2');
    expect(nextFlowPortName('input', ['in1', 'in2'])).toBe('in3');
  });

  it('출력은 out1, out2 … 로 증가하며 빈 자리를 채운다', () => {
    expect(nextFlowPortName('output', [])).toBe('out1');
    expect(nextFlowPortName('output', ['out2'])).toBe('out1');
  });
});
