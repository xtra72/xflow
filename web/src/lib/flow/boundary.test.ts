// SPEC-SUBFLOW-001 그룹 A/B: 플로우 경계 포트 합성 유틸 테스트.

import { describe, expect, it } from 'vitest';
import type { Edge, Node } from '@xyflow/react';

import {
  FLOW_AREA_NODE_ID,
  FLOW_AREA_NODE_TYPE,
  FLOW_BOUNDARY_NODE_TYPE,
  FLOW_INPUT_BOUNDARY_ID,
  FLOW_OUTPUT_BOUNDARY_ID,
  buildAreaNode,
  buildBoundaryNodes,
  computeBoundaryPositions,
  computeNodesBoundingBox,
  edgeUsesBoundaryPort,
  isBoundaryNode,
  isBoundaryNodeId,
  isSyntheticNode,
  isSyntheticNodeId,
  nextFlowPortName,
  parseFlowPorts,
  parseFlowPortsFromConfig,
  realNodesOnly,
  serializeFlowDefinition,
  withBoundaryNodes,
  type FlowAreaNodeData,
  type FlowPortDef,
} from './boundary';

const realNode = (id: string, x = 0, y = 0): Node => ({
  id,
  type: 'custom',
  position: { x, y },
  data: { label: id },
});

/** 측정 크기(width/height)를 가진 실제 노드 헬퍼(바운딩 박스 계산 테스트용). */
const sizedNode = (
  id: string,
  x: number,
  y: number,
  width: number,
  height: number,
): Node => ({
  id,
  type: 'custom',
  position: { x, y },
  width,
  height,
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
    // 영역 노드는 경계 노드가 아니다(포트 경계 2종만 isBoundaryNode).
    expect(isBoundaryNode(realNode(FLOW_AREA_NODE_ID))).toBe(false);
  });

  it('isSyntheticNodeId/isSyntheticNode 는 경계 2종 + 영역 노드를 판정한다', () => {
    expect(isSyntheticNodeId(FLOW_INPUT_BOUNDARY_ID)).toBe(true);
    expect(isSyntheticNodeId(FLOW_OUTPUT_BOUNDARY_ID)).toBe(true);
    expect(isSyntheticNodeId(FLOW_AREA_NODE_ID)).toBe(true);
    expect(isSyntheticNodeId('n1')).toBe(false);
    expect(isSyntheticNodeId(null)).toBe(false);
    expect(isSyntheticNode(realNode(FLOW_AREA_NODE_ID))).toBe(true);
    expect(isSyntheticNode(realNode('n1'))).toBe(false);
  });

  it('realNodesOnly 는 경계 노드 + 영역 노드를 제외한다', () => {
    const nodes = [
      realNode('n1'),
      realNode(FLOW_INPUT_BOUNDARY_ID),
      realNode(FLOW_AREA_NODE_ID),
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

  it('경계 노드는 비-드래그/비-삭제이며 selectable:true(핸들 연결용 native pointer-events)이다', () => {
    // selectable:true 는 React Flow v12 가 노드 래퍼에 native pointer-events:'all' 을
    // 부여하게 해 핸들 연결을 살리는 1차 수정이다(isSelectable→hasPointerEvents).
    // 드래그/삭제는 여전히 금지(위치는 바운딩 박스에서 재파생, 포트는 포트 패널 관리).
    // 실제 "선택 상태"(node.selected) 는 onNodesChange 의 'select' 필터로 차단된다.
    const [inputNode, outputNode] = buildBoundaryNodes(inputs, outputs, []);
    expect(inputNode.draggable).toBe(false);
    expect(inputNode.selectable).toBe(true);
    expect(inputNode.deletable).toBe(false);
    expect(outputNode.draggable).toBe(false);
    expect(outputNode.selectable).toBe(true);
    expect(outputNode.deletable).toBe(false);
  });

  it('경계 노드는 명시적으로 connectable 이다(연결 차단 결함 회귀 가드)', () => {
    // 핸들이 연결을 시작/수신할 수 있도록 경계 노드(입력/출력 모두)는
    // connectable:true 를 명시한다(selectable:true 와 함께 양방향 연결을 보장).
    const [inputNode, outputNode] = buildBoundaryNodes(inputs, outputs, []);
    expect(inputNode.connectable).toBe(true);
    expect(outputNode.connectable).toBe(true);
  });

  it('경계 노드는 selectable:true + style.pointerEvents:all 로 래퍼 포인터를 살린다(핸들 연결 불가 회귀 가드)', () => {
    // React Flow v12.10.1 NodeWrapper(index.mjs:2135):
    //   hasPointerEvents = isSelectable || isDraggable || onClick || onMouse*
    // isSelectable(2114) = !!(node.selectable || ...). 따라서 selectable:true 면
    // hasPointerEvents=true 가 되어 React Flow 가 래퍼에 native pointer-events:'all' 을
    // 직접 부여한다(스타일 핵에 의존하지 않는 신뢰할 수 있는 NATIVE 경로). connectable 은
    // 이 계산에 미포함이므로 connectable:true 만으로는 복구되지 않는다(이전 결함의 원인).
    // style.pointerEvents:'all' 은 무해한 belt-and-suspenders 로 함께 유지한다.
    // 입력/출력 경계 모두에 적용되어야 양방향 연결이 가능하다.
    const [inputNode, outputNode] = buildBoundaryNodes(inputs, outputs, []);
    expect(inputNode.selectable).toBe(true);
    expect(inputNode.style).toMatchObject({ pointerEvents: 'all' });
    expect(outputNode.selectable).toBe(true);
    expect(outputNode.style).toMatchObject({ pointerEvents: 'all' });
  });

  it('경계 노드 위치는 항상 실제 노드 바운딩 박스에서 파생한다(수동 배치 보존 아님)', () => {
    // 이전 경계 노드 위치가 있어도 보존하지 않고 박스에서 다시 계산한다.
    const prev: Node[] = [
      {
        id: FLOW_INPUT_BOUNDARY_ID,
        type: FLOW_BOUNDARY_NODE_TYPE,
        position: { x: -999, y: 123 },
        data: { direction: 'input', ports: [] },
      },
    ];
    const node = sizedNode('n1', 50, 50, 100, 60);
    const [inputNode, outputNode] = buildBoundaryNodes(
      inputs,
      outputs,
      [node],
      prev,
    );
    // bbox: minX=50, maxX=150, centerY=80. GAP=80, 경계 너비 140, 기본 경계 높이 80.
    // 입력은 핸들이 오른쪽 면이므로 minX-GAP-WIDTH=50-80-140=-170, y=80-40=40.
    // 출력은 maxX+GAP=150+80=230.
    expect(inputNode.position).toEqual({ x: 50 - 80 - 140, y: 80 - 40 });
    expect(outputNode.position).toEqual({ x: 150 + 80, y: 80 - 40 });
  });

  it('이전 경계 노드의 측정 높이로 수직 중앙 정렬을 보정한다', () => {
    const prev: Node[] = [
      {
        id: FLOW_INPUT_BOUNDARY_ID,
        type: FLOW_BOUNDARY_NODE_TYPE,
        position: { x: 0, y: 0 },
        measured: { width: 120, height: 200 },
        data: { direction: 'input', ports: [] },
      },
    ];
    const node = sizedNode('n1', 0, 0, 100, 100);
    const [inputNode] = buildBoundaryNodes(inputs, outputs, [node], prev);
    // bbox centerY=50, 입력 경계 측정 높이 200 → y=50-100=-50.
    expect(inputNode.position.y).toBe(-50);
  });

  it('withBoundaryNodes 는 영역 노드 + 실제 노드 + 경계 노드 2개를 반환한다', () => {
    const result = withBoundaryNodes([realNode('n1')], inputs, outputs);
    expect(result).toHaveLength(4);
    expect(result.map((n) => n.id)).toEqual([
      FLOW_AREA_NODE_ID,
      'n1',
      FLOW_INPUT_BOUNDARY_ID,
      FLOW_OUTPUT_BOUNDARY_ID,
    ]);
  });

  it('withBoundaryNodes 의 영역 노드는 node-level width/height 로 크기를 가진다', () => {
    const result = withBoundaryNodes(
      [sizedNode('n1', 0, 0, 100, 60)],
      inputs,
      outputs,
    );
    const area = result.find((n) => n.id === FLOW_AREA_NODE_ID)!;
    // padding 24 → size (100+48, 60+48) = 148 x 108. data 와 node-level 이 일치한다.
    expect(area.width).toBe(148);
    expect(area.height).toBe(108);
    expect(area.data as FlowAreaNodeData).toEqual({ width: 148, height: 108 });
  });

  it('withBoundaryNodes 는 입력으로 들어온 합성 노드를 중복 추가하지 않는다', () => {
    const withExisting: Node[] = [
      realNode('n1'),
      {
        id: FLOW_AREA_NODE_ID,
        type: FLOW_AREA_NODE_TYPE,
        position: { x: 0, y: 0 },
        data: { width: 10, height: 10 },
      },
      {
        id: FLOW_INPUT_BOUNDARY_ID,
        type: FLOW_BOUNDARY_NODE_TYPE,
        position: { x: 0, y: 0 },
        data: { direction: 'input', ports: [] },
      },
    ];
    const result = withBoundaryNodes(withExisting, inputs, outputs);
    expect(result.filter((n) => n.id === FLOW_INPUT_BOUNDARY_ID)).toHaveLength(1);
    expect(result.filter((n) => n.id === FLOW_AREA_NODE_ID)).toHaveLength(1);
  });

  it('실제 노드가 없으면 영역 노드를 추가하지 않고 경계 노드만 폴백 배치한다', () => {
    const result = withBoundaryNodes([], inputs, outputs);
    expect(result.map((n) => n.id)).toEqual([
      FLOW_INPUT_BOUNDARY_ID,
      FLOW_OUTPUT_BOUNDARY_ID,
    ]);
    expect(result.some((n) => n.id === FLOW_AREA_NODE_ID)).toBe(false);
  });
});

describe('boundary - 바운딩 박스 계산(computeNodesBoundingBox)', () => {
  it('여러 노드를 감싸는 박스를 position + 측정 크기로 계산한다', () => {
    const box = computeNodesBoundingBox([
      sizedNode('n1', 0, 0, 100, 50),
      sizedNode('n2', 200, 100, 80, 40),
    ]);
    expect(box).toEqual({
      minX: 0,
      minY: 0,
      maxX: 280, // 200 + 80
      maxY: 140, // 100 + 40
      width: 280,
      height: 140,
      centerX: 140,
      centerY: 70,
    });
  });

  it('단일 노드면 그 노드 하나만 감싼다', () => {
    const box = computeNodesBoundingBox([sizedNode('n1', 10, 20, 100, 60)]);
    expect(box).toMatchObject({
      minX: 10,
      minY: 20,
      maxX: 110,
      maxY: 80,
      centerX: 60,
      centerY: 50,
    });
  });

  it('측정 크기가 없으면 기본 크기(180x80)로 박스를 계산한다', () => {
    const box = computeNodesBoundingBox([realNode('n1', 0, 0)]);
    expect(box).toMatchObject({ minX: 0, minY: 0, maxX: 180, maxY: 80 });
  });

  it('실제 노드가 없으면 null(폴백 신호)을 반환한다', () => {
    expect(computeNodesBoundingBox([])).toBeNull();
  });

  it('합성 노드(경계/영역)는 박스에서 제외한다(피드백 루프 방지)', () => {
    const box = computeNodesBoundingBox([
      sizedNode('n1', 0, 0, 100, 50),
      // 박스를 벗어난 좌표의 합성 노드들 — 무시되어야 한다.
      { ...sizedNode(FLOW_INPUT_BOUNDARY_ID, -9999, -9999, 50, 50) },
      { ...sizedNode(FLOW_OUTPUT_BOUNDARY_ID, 9999, 9999, 50, 50) },
      { ...sizedNode(FLOW_AREA_NODE_ID, -5000, -5000, 50, 50) },
    ]);
    expect(box).toMatchObject({ minX: 0, minY: 0, maxX: 100, maxY: 50 });
  });

  it('합성 노드만 있으면 null 을 반환한다', () => {
    const box = computeNodesBoundingBox([
      realNode(FLOW_INPUT_BOUNDARY_ID),
      realNode(FLOW_AREA_NODE_ID),
    ]);
    expect(box).toBeNull();
  });
});

describe('boundary - 경계 위치 파생(computeBoundaryPositions)', () => {
  it('입력은 좌측(minX-GAP), 출력은 우측(maxX+GAP), 둘 다 수직 중앙 정렬', () => {
    const box = computeNodesBoundingBox([sizedNode('n1', 100, 0, 100, 100)])!;
    const pos = computeBoundaryPositions(box);
    // minX=100, maxX=200, centerY=50, GAP=80, 경계 너비 140, 기본 경계 높이 80 → y=50-40=10.
    // 입력은 핸들이 오른쪽 면이므로 minX-GAP-WIDTH=100-80-140=-120.
    expect(pos.input).toEqual({ x: 100 - 80 - 140, y: 10 });
    expect(pos.output).toEqual({ x: 200 + 80, y: 10 });
  });

  it('경계 노드 측정 높이를 받으면 그 절반만큼 위로 보정한다', () => {
    const box = computeNodesBoundingBox([sizedNode('n1', 0, 0, 100, 100)])!;
    const pos = computeBoundaryPositions(box, { input: 40, output: 120 });
    // centerY=50 → input y=50-20=30, output y=50-60=-10.
    expect(pos.input.y).toBe(30);
    expect(pos.output.y).toBe(-10);
  });

  it('박스가 null(실제 노드 없음)이면 고정 폴백 좌표를 사용한다', () => {
    const pos = computeBoundaryPositions(null);
    // 입력 폴백도 경계 너비(140)만큼 좌측으로 더 민다(오른쪽 면 기준 정렬).
    expect(pos.input).toEqual({ x: -260 - 140, y: 0 });
    expect(pos.output).toEqual({ x: 400, y: 0 });
  });
});

describe('boundary - 영역 노드 생성(buildAreaNode)', () => {
  it('박스를 여백만큼 키운 영역 노드를 만들고 비선택/비삭제/z=0 이다', () => {
    const box = computeNodesBoundingBox([sizedNode('n1', 0, 0, 100, 60)])!;
    const area = buildAreaNode(box)!;
    expect(area.id).toBe(FLOW_AREA_NODE_ID);
    expect(area.type).toBe(FLOW_AREA_NODE_TYPE);
    // padding 24 → position (-24,-24), size (100+48, 60+48).
    expect(area.position).toEqual({ x: -24, y: -24 });
    expect(area.data as FlowAreaNodeData).toEqual({ width: 148, height: 108 });
    // FIX: data 뿐 아니라 node-level width/height(+ style) 가 있어야 React Flow 가
    // 노드 래퍼 크기를 잡아 점선 사각형을 렌더한다.
    expect(area.width).toBe(148);
    expect(area.height).toBe(108);
    expect(area.style).toMatchObject({ width: 148, height: 108 });
    expect(area.selectable).toBe(false);
    expect(area.deletable).toBe(false);
    expect(area.draggable).toBe(false);
    // FIX(연결 차단 결함 회귀 가드): 영역 노드 래퍼(.react-flow__node)가 포인터
    // 이벤트를 가로채 경계 포트 ↔ 내부 노드 연결 드래그를 막던 결함을 방지한다.
    // node-level style.pointerEvents 가 'none' 이어야 래퍼까지 포인터 투명해진다.
    expect(area.style).toMatchObject({ pointerEvents: 'none' });
    // 영역 노드는 어떤 연결에도 참여하지 않는다(명시적 비연결).
    expect(area.connectable).toBe(false);
    // 영역 노드는 nodes 배열 맨 앞(렌더 순서상 아래)에 두어 다른 노드 아래에 깔리며,
    // 음수 zIndex 는 React Flow 스택 컨텍스트 클리핑으로 안 보일 수 있어 0 으로 둔다.
    expect(area.zIndex).toBe(0);
  });

  it('박스가 null 이면 영역 노드를 만들지 않는다(null)', () => {
    expect(buildAreaNode(null)).toBeNull();
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
