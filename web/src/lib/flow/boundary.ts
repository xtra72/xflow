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

/**
 * SPEC-SUBFLOW-001 M4: 실제 노드를 감싸는 "영역(바운딩 박스) 표시" 합성 노드 ID.
 *
 * 경계 포트가 붙는 영역(실제 노드 전체를 감싸는 사각형)을 시각적으로 보여주는
 * 배경 노드의 센티넬 ID 이다. 경계 노드와 마찬가지로 비-노드 엔티티이므로
 * 저장 `nodes` 에서 제외되고(realNodesOnly), 바운딩 박스 계산에서도 제외된다
 * (피드백 루프 방지). 비선택·비삭제·비드래그이며 다른 노드 아래(z<0)에 렌더한다.
 */
export const FLOW_AREA_NODE_ID = '__flow_area__';

/** React Flow 에 등록하는 영역 표시 합성 노드의 노드 타입 키. */
export const FLOW_AREA_NODE_TYPE = 'flow-area';

/**
 * 경계 노드를 바운딩 박스 바깥에 둘 때의 수평 간격(GAP).
 * 입력 경계는 minX-GAP, 출력 경계는 maxX+GAP 에 배치한다.
 */
const BOUNDARY_GAP_X = 80;

/**
 * 경계 노드의 너비(px). makeBoundaryNode 의 node.width 와 동일해야 한다.
 *
 * 입력 경계 노드는 핸들이 오른쪽 면에 붙으므로, x 를 좌측으로 이 너비만큼 더
 * 밀어야(minX - GAP - WIDTH) 노드의 오른쪽 면이 minX 에서 GAP 만큼 떨어진다.
 * 두 값이 어긋나면 입력 경계가 가장 왼쪽 노드와 겹치므로 같은 상수를 공유한다.
 */
const BOUNDARY_WIDTH = 140;

/** 영역(바운딩 박스) 사각형이 실제 노드를 감쌀 때의 여백(padding). */
const AREA_PADDING = 24;

/** 노드 크기 측정값이 없을 때 사용하는 기본 너비/높이(px). */
const DEFAULT_NODE_WIDTH = 180;
const DEFAULT_NODE_HEIGHT = 80;

/** 경계 노드 높이 측정값이 없을 때 사용하는 기본 높이(px, 수직 중앙 정렬 보정용). */
const DEFAULT_BOUNDARY_HEIGHT = 80;

/**
 * 실제 노드가 없을 때 경계 노드를 두는 폴백 좌표/간격.
 *
 * 입력 폴백은 경계 노드 너비만큼 좌측으로 더 민 값으로, 박스 기반 배치
 * (minX - GAP - WIDTH) 와 동일한 "오른쪽 면이 GAP 떨어진다" 규칙을 따른다.
 */
const DEFAULT_INPUT_X = -260 - BOUNDARY_WIDTH;
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

/** 영역(바운딩 박스) 표시 노드의 React Flow data 페이로드. */
export interface FlowAreaNodeData extends Record<string, unknown> {
  /** 영역 사각형의 너비(px). */
  width: number;
  /** 영역 사각형의 높이(px). */
  height: number;
}

/** 주어진 노드 ID 가 합성 경계 센티넬인지 판별한다(두 포트 경계만). */
export function isBoundaryNodeId(id: string | null | undefined): boolean {
  return id === FLOW_INPUT_BOUNDARY_ID || id === FLOW_OUTPUT_BOUNDARY_ID;
}

/**
 * 주어진 노드 ID 가 합성(비-노드) 센티넬인지 판별한다.
 * 두 경계 포트 노드 + 영역 표시 노드 = 저장/바운딩박스 제외 대상.
 */
export function isSyntheticNodeId(id: string | null | undefined): boolean {
  return isBoundaryNodeId(id) || id === FLOW_AREA_NODE_ID;
}

/** 노드가 합성 경계 노드인지 판별한다(포트 경계 2종). */
export function isBoundaryNode(node: Node): boolean {
  return isBoundaryNodeId(node.id);
}

/** 노드가 합성 노드(경계 2종 + 영역)인지 판별한다(저장/바운딩박스 제외 대상). */
export function isSyntheticNode(node: Node): boolean {
  return isSyntheticNodeId(node.id);
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

/**
 * 노드 배열에서 합성 노드(경계 2종 + 영역)를 제외한 "실제(real)" 노드만 반환한다.
 * 저장 직렬화·바운딩 박스 계산 모두 이 결과만 사용한다(합성 노드 피드백 루프 방지).
 */
export function realNodesOnly(nodes: Node[]): Node[] {
  return nodes.filter((n) => !isSyntheticNode(n));
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

/** 노드 하나를 감싸는 바운딩 박스 계산용 측정 크기를 구한다(미측정 시 기본값). */
function getNodeSize(node: Node): { width: number; height: number } {
  const measured = node.measured;
  const width = node.width ?? measured?.width ?? DEFAULT_NODE_WIDTH;
  const height = node.height ?? measured?.height ?? DEFAULT_NODE_HEIGHT;
  return { width, height };
}

/** 실제 노드들을 감싸는 바운딩 박스. */
export interface NodesBoundingBox {
  minX: number;
  minY: number;
  maxX: number;
  maxY: number;
  width: number;
  height: number;
  centerX: number;
  centerY: number;
}

/**
 * 실제 노드 전체를 감싸는 바운딩 박스를 계산한다.
 *
 * - 합성 노드(경계 2종 + 영역)는 내부에서 제외한다(피드백 루프 방지).
 * - 각 노드는 position(좌상단) + 측정 크기(width/height 또는 measured, 미측정 시 기본값)로
 *   우하단을 구한다.
 * - 제외 후 실제 노드가 없으면 null 을 반환한다(폴백은 호출부에서 처리).
 */
export function computeNodesBoundingBox(nodes: Node[]): NodesBoundingBox | null {
  const real = realNodesOnly(nodes);
  if (real.length === 0) return null;

  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const n of real) {
    const { width, height } = getNodeSize(n);
    minX = Math.min(minX, n.position.x);
    minY = Math.min(minY, n.position.y);
    maxX = Math.max(maxX, n.position.x + width);
    maxY = Math.max(maxY, n.position.y + height);
  }

  return {
    minX,
    minY,
    maxX,
    maxY,
    width: maxX - minX,
    height: maxY - minY,
    centerX: (minX + maxX) / 2,
    centerY: (minY + maxY) / 2,
  };
}

/**
 * 바운딩 박스로부터 두 경계 노드 배치 좌표를 도출한다(항상 파생, 수동 배치 아님).
 *
 * - 입력 경계: x = minX - GAP - WIDTH (좌측 바깥). 입력 경계 노드는 핸들이 오른쪽
 *   면에 붙으므로 노드 너비만큼 더 좌측으로 밀어야 오른쪽 면이 minX 에서 GAP 만큼
 *   떨어진다(가장 왼쪽 노드와 겹치지 않게). 출력 경계: x = maxX + GAP (우측 바깥,
 *   핸들이 왼쪽 면에 붙으므로 왼쪽 면이 maxX 에서 GAP 떨어져 겹치지 않음).
 * - y 는 박스의 수직 중앙(centerY)에서 경계 노드 높이의 절반을 빼 시각적으로 중앙 정렬.
 *   경계 노드 높이 측정값(prevHeights)이 있으면 사용하고, 없으면 기본 높이로 보정.
 * - 박스가 null(실제 노드 없음)이면 고정 폴백 좌표를 사용한다(빈 플로우에서도 표시).
 */
export function computeBoundaryPositions(
  bbox: NodesBoundingBox | null,
  prevHeights: { input?: number; output?: number } = {},
): {
  input: { x: number; y: number };
  output: { x: number; y: number };
} {
  if (bbox === null) {
    return {
      input: { x: DEFAULT_INPUT_X, y: DEFAULT_Y },
      output: { x: DEFAULT_OUTPUT_X, y: DEFAULT_Y },
    };
  }
  const inputH = prevHeights.input ?? DEFAULT_BOUNDARY_HEIGHT;
  const outputH = prevHeights.output ?? DEFAULT_BOUNDARY_HEIGHT;
  return {
    input: {
      x: bbox.minX - BOUNDARY_GAP_X - BOUNDARY_WIDTH,
      y: bbox.centerY - inputH / 2,
    },
    output: { x: bbox.maxX + BOUNDARY_GAP_X, y: bbox.centerY - outputH / 2 },
  };
}

/** 단일 경계 노드를 생성한다(파생 좌표 사용). */
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
    // FIX: fitView 타이밍 이슈 — 경계 노드의 width/height 를 명시해 fitView 가
    // 측정 전에 정확한 바운딩 박스를 계산하도록 한다(오프스크린 배치 방지).
    // 실제 렌더 크기는 component 에서 결정되지만, fitView 는 이 값을 사용한다.
    // computeBoundaryPositions 의 입력 경계 좌측 시프트와 어긋나지 않도록
    // 동일한 BOUNDARY_WIDTH 상수를 공유한다(min-w-[120px] + padding 추정값).
    width: BOUNDARY_WIDTH,
    height: ports.length === 0 ? 60 : 60 + ports.length * 40,  // 포트 개수에 따라 높이 조정
    // FIX(핸들 연결 불가 결함, React Flow v12.10.1 소스 검증 — NATIVE 경로):
    // 핸들에서 와이어를 시작할 수도, 핸들로 와이어를 드롭할 수도 없던 결함의 실제
    // 원인은 노드 래퍼(.react-flow__node)의 pointer-events 였다. React Flow v12.10.1
    // NodeWrapper(index.mjs:2114/2135/2231) 의 계산:
    //   isSelectable     = !!(node.selectable || (elementsSelectable && undefined))
    //   hasPointerEvents = isSelectable || isDraggable || onClick
    //                      || onMouseEnter || onMouseMove || onMouseLeave
    //   style: { ..., pointerEvents: hasPointerEvents ? 'all' : 'none', ...node.style }
    // 이전 수정은 node-level style.pointerEvents:'all'(...node.style)로 'none' 을
    // 덮어쓰려 했으나 실전에서 적용되지 않았다(외부 래퍼/클래스 등으로 인라인 override 가
    // 이기지 못함). 대신 NATIVE 플래그를 사용한다: selectable:true 면 isSelectable=true →
    // hasPointerEvents=true 가 되어 React Flow 가 래퍼에 직접 pointer-events:'all' 을
    // 부여한다(스타일 핵 불필요, 항상 우선). 이것이 핸들을 살리는 1차 수정이다.
    selectable: true,
    // 위치는 onNodesChange 에서 항상 실제 노드 바운딩 박스로부터 재파생되므로 드래그
    // 금지여도 무방하다(선택해도 움직이지 않음). 삭제도 금지(포트는 포트 패널에서만 관리).
    draggable: false,
    deletable: false,
    // belt-and-suspenders: 래퍼 인라인 pointerEvents 도 유지한다(무해, native 가 주효).
    // width 는 fitView 가 측정 전에 정확한 박스를 잡도록 node-level width 와 동일하게 유지.
    style: { width: BOUNDARY_WIDTH, pointerEvents: 'all' },
    // connectable 은 명시적으로 true 로 유지한다(핸들이 연결을 시작/수신할 수 있음 보장).
    connectable: true,
    data: {
      direction,
      ports: ports.map((p) => p.name),
    },
  };
}

/**
 * 바운딩 박스로부터 영역(배경) 표시 노드를 만든다(실제 노드가 없으면 null).
 *
 * 사각형은 박스를 AREA_PADDING 만큼 바깥으로 키워 노드를 살짝 감싸며,
 * 다른 노드 아래(zIndex<0)에 렌더하고 비선택·비삭제·비드래그이다.
 */
export function buildAreaNode(
  bbox: NodesBoundingBox | null,
): Node<FlowAreaNodeData> | null {
  if (bbox === null) return null;
  const width = bbox.width + AREA_PADDING * 2;
  const height = bbox.height + AREA_PADDING * 2;
  return {
    id: FLOW_AREA_NODE_ID,
    type: FLOW_AREA_NODE_TYPE,
    position: { x: bbox.minX - AREA_PADDING, y: bbox.minY - AREA_PADDING },
    // FIX: 영역 노드는 node-level width/height 가 반드시 있어야 한다. data 에만
    // 크기를 두면 React Flow 가 노드 래퍼 크기를 잡지 못해 점선 사각형이 렌더되지
    // 않는다(경계 노드가 width/height 추가 전까지 보이지 않던 것과 동일한 원인).
    // style 에도 동일 크기를 넣어 측정 전에도 래퍼가 정확한 크기를 갖게 한다.
    width,
    height,
    // FIX(연결 차단 결함): 영역 노드는 실제 노드 전체를 덮는 거대한 사각형이므로,
    // 노드 래퍼(.react-flow__node)가 그 영역 위에서 포인터 이벤트를 가로채면
    // 경계 포트 ↔ 내부 노드 사이의 연결 드래그가 시작/종료되지 못한다.
    // 내부 div 의 pointerEvents:'none' 만으로는 래퍼가 여전히 이벤트를 잡으므로,
    // node-level style 에 pointerEvents:'none' 을 넣어 React Flow 가 이를 래퍼에
    // 직접 적용하게 한다(래퍼·내부 모두 투명 — belt and suspenders).
    style: { width, height, pointerEvents: 'none' },
    // 영역 노드는 항상 nodes 배열 맨 앞에 위치하므로(렌더 순서상 아래) 음수 z 없이도
    // 다른 노드 아래에 깔린다. 음수 zIndex 는 React Flow 에서 스택 컨텍스트 클리핑으로
    // 보이지 않게 될 수 있어 0 으로 둔다.
    zIndex: 0,
    draggable: false,
    selectable: false,
    deletable: false,
    // 영역 노드는 어떤 연결에도 절대 참여하지 않는다(핸들 없음 + 명시적 비연결).
    connectable: false,
    data: { width, height },
  };
}

/**
 * 플로우 포트 목록 + 실제 노드 바운딩 박스로부터 두 합성 경계 노드를 만든다.
 *
 * 경계 노드 위치는 항상 바운딩 박스에서 파생한다(수동 배치 아님). prevNodes 에
 * 이전 경계 노드가 있으면 그 측정 높이만 읽어 수직 중앙 정렬 보정에 사용한다.
 */
export function buildBoundaryNodes(
  flowInputs: FlowPortDef[],
  flowOutputs: FlowPortDef[],
  realNodes: Node[],
  prevNodes: Node[] = [],
): [Node<FlowBoundaryNodeData>, Node<FlowBoundaryNodeData>] {
  const bbox = computeNodesBoundingBox(realNodes);
  const prevById = new Map(prevNodes.map((n) => [n.id, n] as const));
  const positions = computeBoundaryPositions(bbox, {
    input: prevById.get(FLOW_INPUT_BOUNDARY_ID)?.measured?.height,
    output: prevById.get(FLOW_OUTPUT_BOUNDARY_ID)?.measured?.height,
  });

  return [
    makeBoundaryNode(FLOW_INPUT_BOUNDARY_ID, 'input', flowInputs, positions.input),
    makeBoundaryNode(
      FLOW_OUTPUT_BOUNDARY_ID,
      'output',
      flowOutputs,
      positions.output,
    ),
  ];
}

/**
 * 실제 노드 + 플로우 포트로부터 "렌더용 노드 배열"을 만든다.
 *
 * realNodes 는 합성 노드를 포함하지 않은 순수 노드 배열이어야 한다(방어적으로 다시 필터).
 * - 영역 표시 노드: 플로우 포트가 1개 이상 있고(=경계 UI 가 의미 있을 때) 실제 노드가
 *   1개 이상이면 바운딩 박스로부터 만들어 맨 앞(배경)에 둔다. 포트가 전혀 없으면
 *   경계 UI 자체가 없으므로 영역도 그리지 않아 "포트 없음 → 합성 노드 없음" 불변식을
 *   유지한다(노드 수 의미 보존).
 * - 경계 노드: 해당 방향에 포트가 1개 이상 있을 때만 추가한다(빈 경계 카드로 캔버스를
 *   어지럽히지 않고 노드 수 의미도 보존).
 * 경계/영역 위치는 항상 현재 실제 노드 바운딩 박스에서 파생되며, prevNodes 로는
 * 경계 노드의 측정 높이만 보정에 사용한다(수동 배치 보존 아님).
 */
export function withBoundaryNodes(
  realNodes: Node[],
  flowInputs: FlowPortDef[],
  flowOutputs: FlowPortDef[],
  prevNodes: Node[] = [],
): Node[] {
  const real = realNodesOnly(realNodes);
  const bbox = computeNodesBoundingBox(real);
  const [inputNode, outputNode] = buildBoundaryNodes(
    flowInputs,
    flowOutputs,
    real,
    prevNodes,
  );
  // 포트가 하나도 없으면 경계/영역 합성 노드를 전혀 만들지 않는다.
  const hasAnyPort = flowInputs.length > 0 || flowOutputs.length > 0;
  const areaNode = hasAnyPort ? buildAreaNode(bbox) : null;

  // 영역 노드를 맨 앞에 두어 다른 노드보다 먼저(아래에) 렌더되게 한다(zIndex 도 음수).
  const result: Node[] = areaNode ? [areaNode, ...real] : [...real];
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
