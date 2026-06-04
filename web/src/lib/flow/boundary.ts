// SPEC-SUBFLOW-001 그룹 A/B: 플로우 레벨 경계 포트(boundary port) 합성 유틸.
//
// 플로우 레벨 입출력 포트는 노드가 아닌 플로우 레벨 엔티티이다(REQ-SUBFLOW-B04).
// 캔버스에서는 이를 두 개의 "합성(synthetic) 경계 노드" 로 렌더한다.
//
//   - __flow_input__  : 좌측에 고정. 핸들(출력) = 플로우 입력 포트들.
//                       사용자가 `__flow_input__`.X → 내부 노드 입력 으로 와이어한다.
//   - __flow_output__ : 우측에 고정. 핸들(입력) = 플로우 출력 포트들.
//                       사용자가 내부 노드 출력 → `__flow_output__`.Y 로 와이어한다.
//
// 핸들 id = 플로우 포트 "이름" 이므로, 경계 노드로 연결된 엣지의
// sourceHandle / targetHandle = 포트 이름 = 백엔드 SourcePort / TargetPort 이다.
// 경계 노드를 엔드포인트로 갖는 엣지는 일반 엣지로 영속되며(센티넬 id 를 운반),
// 두 합성 노드만 저장 `nodes` 배열에서 제외된다(로드 시 포트로부터 재구성).
//
// 백엔드 규약(pkg/flow/boundary.go)과 일치:
//   FlowInputBoundaryID  = "__flow_input__"
//   FlowOutputBoundaryID = "__flow_output__"

import type { Edge, Node } from '@xyflow/react';

/** 플로우 입력 포트 경계 센티넬 노드 ID (백엔드 FlowInputBoundaryID 와 동일). */
export const FLOW_INPUT_BOUNDARY_ID = '__flow_input__';

/** 플로우 출력 포트 경계 센티넬 노드 ID (백엔드 FlowOutputBoundaryID 와 동일). */
export const FLOW_OUTPUT_BOUNDARY_ID = '__flow_output__';

/** React Flow 에 등록하는 합성 경계 노드의 노드 타입 키. */
export const FLOW_BOUNDARY_NODE_TYPE = 'flow-boundary';

/** 경계 노드 배치용 기본 좌표/간격(실제 노드가 없을 때 폴백). */
const BOUNDARY_GAP_X = 260;
const DEFAULT_INPUT_X = -BOUNDARY_GAP_X;
const DEFAULT_OUTPUT_X = 400;
const DEFAULT_Y = 0;

/**
 * 플로우 레벨 포트 정의(에디터 상태).
 * 방향은 소속 목록(flowInputs / flowOutputs)으로 결정되므로 여기서는 보관하지 않는다.
 */
export interface FlowPortDef {
  /** 안정적 식별자(이름 변경에도 불변). */
  id: string;
  /** 표시·연결 이름. flow-node 핸들 이름 및 와이어 핸들 id 로 사용. */
  name: string;
}

/** 경계 노드의 React Flow data 페이로드. */
export interface FlowBoundaryNodeData extends Record<string, unknown> {
  /** 경계 방향: 입력 경계(좌) 또는 출력 경계(우). */
  direction: 'input' | 'output';
  /** 이 경계가 노출하는 포트 이름 배열(핸들). */
  ports: string[];
}

/** 주어진 노드 ID 가 합성 경계 센티넬인지 판별한다. */
export function isBoundaryNodeId(id: string | null | undefined): boolean {
  return id === FLOW_INPUT_BOUNDARY_ID || id === FLOW_OUTPUT_BOUNDARY_ID;
}

/** 노드가 합성 경계 노드인지 판별한다(저장 제외 대상). */
export function isBoundaryNode(node: Node): boolean {
  return isBoundaryNodeId(node.id);
}

/**
 * 엣지가 주어진 방향/포트 이름의 경계 와이어를 엔드포인트로 갖는지 판별한다.
 *
 * - input 방향: source === __flow_input__ && sourceHandle === portName
 * - output 방향: target === __flow_output__ && targetHandle === portName
 */
export function edgeUsesBoundaryPort(
  edge: Edge,
  direction: 'input' | 'output',
  portName: string,
): boolean {
  if (direction === 'input') {
    return (
      edge.source === FLOW_INPUT_BOUNDARY_ID &&
      edge.sourceHandle === portName
    );
  }
  return (
    edge.target === FLOW_OUTPUT_BOUNDARY_ID && edge.targetHandle === portName
  );
}

/** 노드 배열에서 합성 경계 노드를 제외한 "실제(real)" 노드만 반환한다. */
export function realNodesOnly(nodes: Node[]): Node[] {
  return nodes.filter((n) => !isBoundaryNode(n));
}

/**
 * 정의 최상위 inputs/outputs(맵 배열) 에서 FlowPortDef 배열을 파싱한다.
 *
 * 각 항목은 {id,name,direction} 형태이며, 비문자열/빈 이름은 안전하게 걸러낸다.
 * id 가 없으면 빈 문자열로 두지 않고 이름 기반 임시 id 를 부여한다(로드 안정성).
 * 중복 id 는 뒤따르는 항목을 건너뛴다.
 */
export function parseFlowPorts(raw: unknown): FlowPortDef[] {
  if (!Array.isArray(raw)) return [];
  const seenId = new Set<string>();
  const ports: FlowPortDef[] = [];
  for (const item of raw) {
    if (item == null || typeof item !== 'object') continue;
    const rec = item as Record<string, unknown>;
    const name = typeof rec.name === 'string' ? rec.name.trim() : '';
    if (name === '') continue;
    let id = typeof rec.id === 'string' ? rec.id.trim() : '';
    if (id === '') id = `port-${name}`;
    if (seenId.has(id)) continue;
    seenId.add(id);
    ports.push({ id, name });
  }
  return ports;
}

/**
 * 플로우 상세 config(또는 정의) 에서 입력/출력 플로우 포트를 파싱한다.
 * 백엔드는 정의 최상위 inputs/outputs 를 flowToReactFlowConfig 의 config.inputs /
 * config.outputs 로 방출한다(REQ-SUBFLOW-A07).
 */
export function parseFlowPortsFromConfig(config: unknown): {
  flowInputs: FlowPortDef[];
  flowOutputs: FlowPortDef[];
} {
  const src = (config ?? {}) as Record<string, unknown>;
  return {
    flowInputs: parseFlowPorts(src.inputs),
    flowOutputs: parseFlowPorts(src.outputs),
  };
}

/** FlowPortDef 배열을 정의 최상위 직렬화용 {id,name,direction} 배열로 변환한다. */
function flowPortsToDefs(
  ports: FlowPortDef[],
  direction: 'input' | 'output',
): Array<{ id: string; name: string; direction: 'input' | 'output' }> {
  return ports.map((p) => ({ id: p.id, name: p.name, direction }));
}

/**
 * 실제 노드들의 경계 상자에서 경계 노드 배치 좌표를 계산한다.
 *
 * 입력 경계는 좌측 바깥, 출력 경계는 우측 바깥에 둔다. 실제 노드가 없으면 기본값.
 */
function computeBoundaryPositions(realNodes: Node[]): {
  input: { x: number; y: number };
  output: { x: number; y: number };
} {
  if (realNodes.length === 0) {
    return {
      input: { x: DEFAULT_INPUT_X, y: DEFAULT_Y },
      output: { x: DEFAULT_OUTPUT_X, y: DEFAULT_Y },
    };
  }
  let minX = Infinity;
  let maxX = -Infinity;
  let sumY = 0;
  for (const n of realNodes) {
    minX = Math.min(minX, n.position.x);
    maxX = Math.max(maxX, n.position.x);
    sumY += n.position.y;
  }
  const avgY = sumY / realNodes.length;
  return {
    input: { x: minX - BOUNDARY_GAP_X, y: avgY },
    output: { x: maxX + BOUNDARY_GAP_X, y: avgY },
  };
}

/** 단일 경계 노드를 생성한다(이전 위치가 있으면 그 위치를 보존). */
function makeBoundaryNode(
  id: string,
  direction: 'input' | 'output',
  ports: FlowPortDef[],
  position: { x: number; y: number },
): Node<FlowBoundaryNodeData> {
  return {
    id,
    type: FLOW_BOUNDARY_NODE_TYPE,
    position,
    // 경계 노드는 고정·비선택·비삭제 — 일반 노드 편집/삭제 대상에서 제외한다.
    draggable: false,
    selectable: false,
    deletable: false,
    data: {
      direction,
      ports: ports.map((p) => p.name),
    },
  };
}

/**
 * 플로우 포트 목록으로부터 두 합성 경계 노드(__flow_input__ / __flow_output__)를 만든다.
 *
 * prevNodes 에 동일 경계 노드가 있으면 그 위치를 보존해 포트 편집 시 노드가
 * 튀지 않도록 한다(없으면 실제 노드 경계 상자로 계산).
 */
export function buildBoundaryNodes(
  flowInputs: FlowPortDef[],
  flowOutputs: FlowPortDef[],
  realNodes: Node[],
  prevNodes: Node[] = [],
): [Node<FlowBoundaryNodeData>, Node<FlowBoundaryNodeData>] {
  const positions = computeBoundaryPositions(realNodes);
  const prevById = new Map(prevNodes.map((n) => [n.id, n] as const));
  const prevInput = prevById.get(FLOW_INPUT_BOUNDARY_ID);
  const prevOutput = prevById.get(FLOW_OUTPUT_BOUNDARY_ID);

  return [
    makeBoundaryNode(
      FLOW_INPUT_BOUNDARY_ID,
      'input',
      flowInputs,
      prevInput?.position ?? positions.input,
    ),
    makeBoundaryNode(
      FLOW_OUTPUT_BOUNDARY_ID,
      'output',
      flowOutputs,
      prevOutput?.position ?? positions.output,
    ),
  ];
}

/**
 * 실제 노드 + 플로우 포트로부터 "렌더용 노드 배열"을 만든다.
 *
 * realNodes 는 경계 노드를 포함하지 않은 순수 노드 배열이어야 한다.
 * 경계 노드는 해당 방향에 포트가 1개 이상 있을 때만 추가한다(포트가 없는 플로우는
 * 빈 경계 카드로 캔버스를 어지럽히지 않고, 노드 수 의미도 보존). 포트는 포트 관리
 * 패널에서 추가하면 즉시 경계 노드가 나타난다.
 * prevNodes 로 이전 경계 노드 위치를 보존한다.
 */
export function withBoundaryNodes(
  realNodes: Node[],
  flowInputs: FlowPortDef[],
  flowOutputs: FlowPortDef[],
  prevNodes: Node[] = [],
): Node[] {
  const real = realNodesOnly(realNodes);
  const [inputNode, outputNode] = buildBoundaryNodes(
    flowInputs,
    flowOutputs,
    real,
    prevNodes,
  );
  const result = [...real];
  if (flowInputs.length > 0) result.push(inputNode);
  if (flowOutputs.length > 0) result.push(outputNode);
  return result;
}

/**
 * React Flow 상태(nodes/edges + 플로우 포트)를 백엔드 저장/내보내기용 정의로 직렬화한다.
 *
 * - nodes: 합성 경계 노드를 제외한 실제 노드만(REQ-SUBFLOW-B04).
 * - edges: 센티넬 경계 와이어를 포함한 모든 엣지를 보존(경계 연결은 일반 엣지로 영속).
 * - inputs/outputs: 플로우 레벨 포트를 정의 최상위에 {id,name,direction} 으로 기록
 *   (REQ-SUBFLOW-A07). 백엔드가 round-trip 한다.
 */
export function serializeFlowDefinition(
  nodes: Node[],
  edges: Edge[],
  flowInputs: FlowPortDef[],
  flowOutputs: FlowPortDef[],
): Record<string, unknown> {
  return {
    nodes: realNodesOnly(nodes),
    edges,
    inputs: flowPortsToDefs(flowInputs, 'input'),
    outputs: flowPortsToDefs(flowOutputs, 'output'),
  };
}

/**
 * 새 플로우 포트의 기본 이름을 계산한다(in1, in2, … / out1, out2, …).
 * existingNames 안에서 유일해지는 가장 작은 양의 정수 접미사를 사용한다.
 */
export function nextFlowPortName(
  direction: 'input' | 'output',
  existingNames: Iterable<string>,
): string {
  const prefix = direction === 'input' ? 'in' : 'out';
  const taken = new Set(existingNames);
  let n = 1;
  while (taken.has(`${prefix}${n}`)) n += 1;
  return `${prefix}${n}`;
}
