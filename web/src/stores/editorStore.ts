// Flow editor state management with undo/redo history.

import { create } from 'zustand';
import {
  addEdge,
  applyEdgeChanges,
  applyNodeChanges,
  type Connection,
  type Edge,
  type EdgeChange,
  type Node,
  type NodeChange,
} from '@xyflow/react';

import { generateUUID } from '@/lib/utils/uuid';
import {
  edgeUsesBoundaryPort,
  isSyntheticNodeId,
  nextFlowPortName,
  realNodesOnly,
  withBoundaryNodes,
  type FlowPortDef,
} from '@/lib/flow/boundary';

const MAX_HISTORY = 50;

/** 복제 시 라벨에 붙는 offset (원본과 겹치지 않도록 새 노드를 비껴 배치). */
const DUPLICATE_OFFSET = { x: 32, y: 32 } as const;

/** `-복제<숫자>` 접미사 패턴 (라벨 stem 추출용). */
const DUPLICATE_SUFFIX_RE = /-복제\d+$/;

/**
 * 복제 노드의 라벨을 계산하는 순수 헬퍼.
 *
 * - baseLabel 에서 후행 `-복제<숫자>` 접미사를 제거해 stem 을 구한다.
 *   (이미 복제된 노드를 다시 복제할 때 원본 stem 을 재사용하기 위함)
 * - `${stem}-복제${n}` 형태로, existingLabels 안에서 유일해지는 가장 작은
 *   양의 정수 n 을 찾아 반환한다.
 *
 * 예) "filter" + {} → "filter-복제1"
 *     "filter" + {"filter-복제1"} → "filter-복제2"
 *     "filter-복제1" + {"filter-복제1"} → "filter-복제2" (stem = "filter")
 */
export function nextDuplicateLabel(
  baseLabel: string,
  existingLabels: Set<string>,
): string {
  const stem = baseLabel.replace(DUPLICATE_SUFFIX_RE, '');
  let n = 1;
  let candidate = `${stem}-복제${n}`;
  while (existingLabels.has(candidate)) {
    n += 1;
    candidate = `${stem}-복제${n}`;
  }
  return candidate;
}

/**
 * 주어진 노드들을 복제해 새 노드 배열을 만드는 순수 헬퍼.
 *
 * - 각 노드는 새 UUID id 를 받고, data 는 깊은 복제 후 label 만 교체한다.
 * - 라벨은 existingLabels 와 "이번에 생성된 라벨들" 양쪽에 대해 유일하도록
 *   순차적으로 계산한다 (다중 복제 시 복제1, 복제2 … 로 증가).
 * - position 은 모든 복제본에 동일한 delta 를 더해 상대 위치를 보존한다.
 * - 복제본은 selected=true 로 표시한다 (호출부에서 원본은 해제).
 */
/** 노드 배열에서 문자열 label 집합을 수집한다. */
function collectLabels(nodes: Node[]): Set<string> {
  const labels = new Set<string>();
  for (const n of nodes) {
    if (typeof n.data?.label === 'string') {
      labels.add(n.data.label);
    }
  }
  return labels;
}

function buildDuplicateNodes(
  sources: Node[],
  existingLabels: Set<string>,
): Node[] {
  // 생성 과정에서 누적되는 라벨까지 고려해 유일성을 유지한다.
  const usedLabels = new Set(existingLabels);
  return sources.map((src) => {
    const baseLabel =
      typeof src.data?.label === 'string' ? src.data.label : src.id;
    const newLabel = nextDuplicateLabel(baseLabel, usedLabels);
    usedLabels.add(newLabel);

    // data 를 깊은 복제해 원본과 참조를 공유하지 않도록 한다.
    const clonedData = structuredClone(src.data ?? {});

    return {
      ...src,
      id: generateUUID(),
      type: src.type ?? 'custom',
      position: {
        x: src.position.x + DUPLICATE_OFFSET.x,
        y: src.position.y + DUPLICATE_OFFSET.y,
      },
      data: { ...clonedData, label: newLabel },
      selected: true,
    };
  });
}

interface HistoryEntry {
  nodes: Node[];
  edges: Edge[];
  // SPEC-SUBFLOW-001: 플로우 레벨 포트도 편집 이력에 포함한다(포트 추가/이름/삭제 undo).
  flowInputs: FlowPortDef[];
  flowOutputs: FlowPortDef[];
}

interface EditorState {
  nodes: Node[];
  edges: Edge[];
  /**
   * SPEC-SUBFLOW-001 그룹 A/B: 플로우 레벨 입력/출력 포트(노드가 아닌 플로우 레벨 엔티티).
   *
   * 이 두 배열이 플로우 포트의 단일 진실 공급원(source of truth)이며, 캔버스의 두
   * 합성 경계 노드(__flow_input__ / __flow_output__)는 이 값으로부터 파생되어
   * `nodes` 배열에 함께 렌더된다(withBoundaryNodes). 저장 시에는 경계 노드를
   * `nodes` 에서 제외하고 이 포트 목록을 정의 최상위 inputs/outputs 로 기록한다.
   *
   * 포트의 실제 편집(추가/이름/삭제)만 dirty + pushUndo 로 처리한다(REQ-SUBFLOW-B05).
   */
  flowInputs: FlowPortDef[];
  flowOutputs: FlowPortDef[];
  selectedNodeId: string | null;
  selectedEdgeId: string | null;
  /**
   * SPEC-LINK-001: 현재 하이라이트된 가상 링크 이름(그룹 식별자).
   * 사용자가 가상 링크 배지를 클릭하면 같은 이름 그룹의 숨겨진 와이어 선을
   * 일시적으로 표시하고 상대 배지를 강조한다. UI 표시 전용 상태이므로
   * pushUndo / isDirty 로직에 절대 포함하지 않는다.
   */
  highlightedLinkName: string | null;
  isDirty: boolean;
  undoStack: HistoryEntry[];
  redoStack: HistoryEntry[];
  /**
   * 현재 에디터가 편집 중인 플로우 ID.
   * 노드 카드의 라이브 제어(예: output ON/OFF) 가 실행 중 플로우에 즉시
   * 적용되도록 configureNode 호출 시 사용한다. 편집 이력(undo/redo) 이나
   * 저장 변경 상태(isDirty) 와 무관한 식별자이므로 pushUndo / dirty 로직에
   * 절대 포함하지 않는다.
   */
  currentFlowId: string | null;
  /**
   * 복사/붙여넣기용 클립보드 스냅샷.
   * 편집 이력(undo/redo) 이나 저장 변경 상태(isDirty) 와 무관하므로
   * pushUndo / dirty 로직에 절대 포함하지 않는다.
   */
  clipboard: Node[];
  /**
   * 가상 와이어 표시 토글(뷰 전용).
   *
   * - false(기본): 가상 와이어 선을 숨기고 포트별 컴팩트 링크 인디케이터를 보인다(기존 동작).
   * - true: 가상 와이어를 일반 연결선처럼 그리고, 중복되는 컴팩트 인디케이터는 숨긴다.
   *
   * 라우팅/저장 데이터/엔진 동작에 전혀 영향을 주지 않는 순수 표시 상태이므로
   * pushUndo / isDirty 로직에 절대 포함하지 않으며, 플로우 로드/리셋 시에도
   * 유지한다(뷰 설정은 플로우와 무관하게 사용자 선호로 본다).
   */
  showVirtualWires: boolean;
  /**
   * 연결 포커스 토글(뷰 전용).
   *
   * - true 이고 정확히 한 노드가 선택되면, 선택 노드와 1-hop 연결된 노드/엣지만
   *   강조하고 그 외는 흐리게(opacity) 렌더한다.
   * - false 이거나 선택 노드가 없으면(또는 다중 선택) 일반 렌더(흐림 없음).
   *
   * showVirtualWires 와 동일하게 순수 표시 상태이므로 pushUndo / isDirty 에
   * 포함하지 않고, 플로우 로드/리셋 시에도 유지한다.
   */
  focusConnectionsOnSelect: boolean;
  /**
   * 연결 포커스 단계(depth, 뷰 전용).
   *
   * 포커스 모드에서 선택 노드로부터 몇 hop 까지 강조할지 결정한다.
   * - 1(기본): 선택 노드 + 직접 이웃(기존 동작).
   * - 2~FOCUS_DEPTH_MAX: 이웃의 이웃까지 단계적으로 확장.
   * - FOCUS_DEPTH_ALL(=Infinity): 선택 노드의 전체 방향성 연결 체인을 강조(무제한 hop).
   * - 유한 값은 [FOCUS_DEPTH_MIN, FOCUS_DEPTH_MAX] 범위로 클램프하며,
   *   FOCUS_DEPTH_ALL 은 그대로 유지한다.
   *
   * showVirtualWires / focusConnectionsOnSelect 와 동일하게 순수 표시 상태이므로
   * pushUndo / isDirty 에 포함하지 않고, 플로우 로드/리셋 시에도 유지한다.
   */
  focusDepth: number;
}

/** 연결 포커스 단계의 최소/최대(유한) 한계. */
export const FOCUS_DEPTH_MIN = 1;
export const FOCUS_DEPTH_MAX = 5;

/**
 * 연결 포커스 단계의 "전체(무제한)" 센티넬.
 *
 * focusDepth 가 이 값이면 선택 노드로부터 도달 가능한 전체 방향성 연결 체인을
 * 강조한다(hop 수 제한 없음). getConnectedElements 의 BFS 는 frontier 가 빌
 * 때까지 진행하며, 방문 집합이 사이클을 막는다. 유한 단계(1~FOCUS_DEPTH_MAX)
 * 와 구별하기 위해 Infinity 를 센티넬로 사용한다.
 */
export const FOCUS_DEPTH_ALL = Number.POSITIVE_INFINITY;

/**
 * 연결 포커스 단계를 유효한 값으로 클램프한다.
 *
 * - FOCUS_DEPTH_ALL(=+Infinity) 은 "전체" 센티넬이므로 그대로 통과시킨다
 *   (상한 5 로 내리지 않는다).
 * - NaN 은 안전하게 하한(MIN) 으로 처리한다.
 * - -Infinity 는 하한(MIN) 으로 처리한다.
 * - 그 외 유한 값은 [MIN, MAX] 범위로 클램프하고, 비정수는 내림한다(예: 2.9 → 2).
 */
export function clampFocusDepth(n: number): number {
  // "전체" 센티넬은 클램프 없이 그대로 유지한다.
  if (n === FOCUS_DEPTH_ALL) return FOCUS_DEPTH_ALL;
  if (Number.isNaN(n)) return FOCUS_DEPTH_MIN;
  if (n >= FOCUS_DEPTH_MAX) return FOCUS_DEPTH_MAX;
  if (n <= FOCUS_DEPTH_MIN) return FOCUS_DEPTH_MIN;
  // 유한한 중간값만 남으므로 내림으로 정수화한다(예: 2.9 → 2).
  return Math.floor(n);
}

interface EditorActions {
  /**
   * 서버 로딩 전용: nodes/edges/플로우 포트를 교체하고 dirty=false, 히스토리 초기화.
   * flowInputs/flowOutputs 로부터 합성 경계 노드를 만들어 nodes 에 함께 넣는다.
   * (flowInputs/flowOutputs 미지정 시 빈 배열 — 기존 호출부 호환)
   */
  loadFlow: (
    nodes: Node[],
    edges: Edge[],
    flowInputs?: FlowPortDef[],
    flowOutputs?: FlowPortDef[],
  ) => void;
  /** SPEC-SUBFLOW-001: 플로우 입력 포트를 추가한다(고유 id + 기본 이름, dirty+undo). */
  addFlowInput: () => void;
  /** SPEC-SUBFLOW-001: 플로우 출력 포트를 추가한다(고유 id + 기본 이름, dirty+undo). */
  addFlowOutput: () => void;
  /**
   * SPEC-SUBFLOW-001: 플로우 포트 이름을 변경한다(id 불변 → 와이어 연속성 보존, dirty+undo).
   * 경계 와이어의 핸들 id(=포트 이름) 도 함께 갱신해 연결을 유지한다.
   */
  renameFlowPort: (
    direction: 'input' | 'output',
    id: string,
    name: string,
  ) => void;
  /**
   * SPEC-SUBFLOW-001: 플로우 포트를 삭제한다(dirty+undo).
   * 해당 포트를 엔드포인트로 쓰던 경계 와이어(센티넬 엣지)도 함께 제거한다.
   */
  removeFlowPort: (direction: 'input' | 'output', id: string) => void;
  setNodes: (nodes: Node[]) => void;
  setEdges: (edges: Edge[]) => void;
  onNodesChange: (changes: NodeChange[]) => void;
  onEdgesChange: (changes: EdgeChange[]) => void;
  onConnect: (connection: Connection) => void;
  addNode: (node: Node) => void;
  removeNode: (nodeId: string) => void;
  /**
   * 주어진 기존 노드들을 복제한다 (새 UUID/라벨/offset/선택, pushUndo, isDirty=true).
   * 비어 있으면 아무 동작도 하지 않는다.
   */
  duplicateNodes: (nodeIds: string[]) => void;
  /** 주어진 노드들을 깊은 복제해 클립보드에 스냅샷한다 (dirty/undo 영향 없음). */
  copyToClipboard: (nodeIds: string[]) => void;
  /**
   * 클립보드 스냅샷 노드들을 캔버스에 복제한다
   * (새 UUID/라벨/offset/선택, pushUndo, isDirty=true). 비어 있으면 no-op.
   */
  pasteClipboard: () => void;
  updateNodeData: (nodeId: string, data: Record<string, unknown>) => void;
  updateEdgeData: (edgeId: string, data: Record<string, unknown>) => void;
  selectNode: (nodeId: string | null) => void;
  selectEdge: (edgeId: string | null) => void;
  /**
   * SPEC-LINK-001: 가상 링크 하이라이트 그룹 설정.
   * 같은 이름을 다시 설정하면 토글 해제(null)된다. dirty/undo 영향 없음.
   */
  setHighlightedLinkName: (name: string | null) => void;
  undo: () => void;
  redo: () => void;
  clearHistory: () => void;
  setDirty: (dirty: boolean) => void;
  /** 현재 편집 중인 플로우 ID 설정 (hydration 시). dirty/undo 에 영향 없음. */
  setCurrentFlowId: (flowId: string | null) => void;
  /** 가상 와이어 표시 토글 (뷰 전용, dirty/undo 무관). */
  toggleShowVirtualWires: () => void;
  /** 연결 포커스 토글 (뷰 전용, dirty/undo 무관). */
  toggleFocusConnections: () => void;
  /**
   * 연결 포커스 단계 설정 (뷰 전용, dirty/undo 무관).
   * 유한 값은 [1, FOCUS_DEPTH_MAX] 로 클램프하고, FOCUS_DEPTH_ALL(전체) 은 그대로 둔다.
   */
  setFocusDepth: (depth: number) => void;
  resetEditor: () => void;
}

/**
 * Push the current nodes/edges onto the undo stack (capped at MAX_HISTORY)
 * and clear the redo stack since a new change invalidates future history.
 */
function pushUndo(state: EditorState): Pick<EditorState, 'undoStack' | 'redoStack'> {
  const entry: HistoryEntry = {
    nodes: state.nodes,
    edges: state.edges,
    flowInputs: state.flowInputs,
    flowOutputs: state.flowOutputs,
  };
  const stack = [...state.undoStack, entry];
  if (stack.length > MAX_HISTORY) {
    stack.shift();
  }
  return { undoStack: stack, redoStack: [] };
}

/**
 * 현재 state.nodes(실제 노드 + 경계 노드) 와 플로우 포트로부터 렌더용 노드 배열을
 * 재구성한다. 실제 노드는 보존하고 경계 노드만 포트 기준으로 다시 만든다(이전
 * 경계 노드 위치는 보존). 플로우 포트 편집 시 경계 노드 핸들을 갱신하는 데 사용한다.
 */
function rebuildRenderedNodes(
  state: Pick<EditorState, 'nodes' | 'flowInputs' | 'flowOutputs'>,
  flowInputs: FlowPortDef[],
  flowOutputs: FlowPortDef[],
): Node[] {
  return withBoundaryNodes(
    realNodesOnly(state.nodes),
    flowInputs,
    flowOutputs,
    state.nodes,
  );
}

export const useEditorStore = create<EditorState & EditorActions>()((set) => ({
  // State
  nodes: [],
  edges: [],
  flowInputs: [],
  flowOutputs: [],
  selectedNodeId: null,
  selectedEdgeId: null,
  highlightedLinkName: null,
  isDirty: false,
  undoStack: [],
  redoStack: [],
  currentFlowId: null,
  clipboard: [],
  // 뷰 전용 표시 토글 — 기본값 false(기존 동작과 100% 동일하게 렌더).
  showVirtualWires: false,
  focusConnectionsOnSelect: false,
  // 연결 포커스 단계 — 기본 1(직접 이웃만, 기존 동작과 동일).
  focusDepth: FOCUS_DEPTH_MIN,

  // Actions

  // Server-load only: replace nodes/edges WITHOUT marking the editor dirty
  // and reset history. Use this when hydrating from server data (initial load
  // or post-save refetch) so the unsaved indicator does not turn back on.
  loadFlow: (nodes, edges, flowInputs = [], flowOutputs = []) =>
    set({
      // 서버 nodes 는 경계 노드를 포함하지 않으므로(저장 시 제외됨), 플로우 포트로부터
      // 합성 경계 노드를 만들어 렌더용 nodes 에 함께 넣는다(REQ-SUBFLOW-B01).
      nodes: withBoundaryNodes(realNodesOnly(nodes), flowInputs, flowOutputs),
      edges,
      flowInputs,
      flowOutputs,
      isDirty: false,
      undoStack: [],
      redoStack: [],
      highlightedLinkName: null,
    }),

  // 플로우 포트 추가/이름/삭제는 실제 편집이므로 dirty + pushUndo 로 처리한다
  // (노드 편집과 동일, REQ-SUBFLOW-A02/B05). 편집 후 경계 노드 핸들을 재구성한다.
  addFlowInput: () =>
    set((state) => {
      const existing = new Set(state.flowInputs.map((p) => p.name));
      const name = nextFlowPortName('input', existing);
      const flowInputs = [...state.flowInputs, { id: generateUUID(), name }];
      return {
        ...pushUndo(state),
        flowInputs,
        nodes: rebuildRenderedNodes(state, flowInputs, state.flowOutputs),
        isDirty: true,
      };
    }),

  addFlowOutput: () =>
    set((state) => {
      const existing = new Set(state.flowOutputs.map((p) => p.name));
      const name = nextFlowPortName('output', existing);
      const flowOutputs = [...state.flowOutputs, { id: generateUUID(), name }];
      return {
        ...pushUndo(state),
        flowOutputs,
        nodes: rebuildRenderedNodes(state, state.flowInputs, flowOutputs),
        isDirty: true,
      };
    }),

  renameFlowPort: (direction, id, name) =>
    set((state) => {
      const trimmed = name.trim();
      if (trimmed === '') return state;

      const list = direction === 'input' ? state.flowInputs : state.flowOutputs;
      const target = list.find((p) => p.id === id);
      // 대상이 없거나 이름이 그대로면 dirty 를 만들지 않는다.
      if (!target || target.name === trimmed) return state;
      const oldName = target.name;

      const updated = list.map((p) =>
        p.id === id ? { ...p, name: trimmed } : p,
      );
      const flowInputs = direction === 'input' ? updated : state.flowInputs;
      const flowOutputs = direction === 'output' ? updated : state.flowOutputs;

      // 핸들 id(=포트 이름) 가 바뀌므로 경계 와이어의 sourceHandle/targetHandle 도
      // 갱신해 연결 연속성을 보존한다(REQ-SUBFLOW-A03).
      const edges = state.edges.map((e) => {
        if (direction === 'input' && edgeUsesBoundaryPort(e, 'input', oldName)) {
          return { ...e, sourceHandle: trimmed };
        }
        if (
          direction === 'output' &&
          edgeUsesBoundaryPort(e, 'output', oldName)
        ) {
          return { ...e, targetHandle: trimmed };
        }
        return e;
      });

      return {
        ...pushUndo(state),
        flowInputs,
        flowOutputs,
        edges,
        nodes: rebuildRenderedNodes(state, flowInputs, flowOutputs),
        isDirty: true,
      };
    }),

  removeFlowPort: (direction, id) =>
    set((state) => {
      const list = direction === 'input' ? state.flowInputs : state.flowOutputs;
      const target = list.find((p) => p.id === id);
      if (!target) return state;
      const portName = target.name;

      const updated = list.filter((p) => p.id !== id);
      const flowInputs = direction === 'input' ? updated : state.flowInputs;
      const flowOutputs = direction === 'output' ? updated : state.flowOutputs;

      // 삭제된 포트를 엔드포인트로 쓰던 경계 와이어(센티넬 엣지)를 제거한다
      // (REQ-SUBFLOW-A04 — 기존 포트 제거 시 dangling 와이어 정리).
      const edges = state.edges.filter(
        (e) => !edgeUsesBoundaryPort(e, direction, portName),
      );

      return {
        ...pushUndo(state),
        flowInputs,
        flowOutputs,
        edges,
        nodes: rebuildRenderedNodes(state, flowInputs, flowOutputs),
        isDirty: true,
      };
    }),

  setNodes: (nodes) =>
    set((state) => ({
      ...pushUndo(state),
      nodes,
      isDirty: true,
    })),

  setEdges: (edges) =>
    set((state) => ({
      ...pushUndo(state),
      edges,
      isDirty: true,
    })),

  onNodesChange: (changes) =>
    set((state) => {
      // 합성 노드(경계 2종 + 영역)는 비-노드 엔티티이므로 일반 노드 삭제(remove)
      // 대상에서 제외한다(REQ-SUBFLOW-B04). 합성 노드는 draggable/selectable=false
      // 이지만, 방어적으로 remove 변경도 걸러낸다.
      const filtered = changes.filter(
        (c) => !(c.type === 'remove' && isSyntheticNodeId(c.id)),
      );
      // 'dimensions'(노드 측정) 와 'select'(선택) 변경은 사용자 편집이 아니므로
      // isDirty 를 만들지 않는다. React Flow 는 마운트/렌더 시 노드를 측정하며
      // 'dimensions' 변경을 emit 하는데, 이를 dirty 로 처리하면 플로우 로드 직후
      // 편집 없이도 무조건 "수정됨" 으로 표시되는 버그가 발생한다(미저장 가드 오작동).
      const meaningful = filtered.some(
        (c) => c.type !== 'dimensions' && c.type !== 'select',
      );

      const applied = applyNodeChanges(filtered, state.nodes);

      // SPEC-SUBFLOW-001 M4: 실제 노드의 위치(position)·크기(dimensions)·삭제(remove)가
      // 바뀌면 경계 노드/영역 노드를 현재 실제 노드 바운딩 박스에서 다시 파생한다
      // (노드를 옮기면 경계 포트와 영역 사각형이 함께 따라온다). 합성 노드는 실제
      // 노드 바운딩 박스에서만 파생되고 자신은 박스에서 제외되므로 피드백 루프가 없다.
      const geometryChanged = filtered.some(
        (c) =>
          c.type === 'position' ||
          c.type === 'dimensions' ||
          c.type === 'remove' ||
          c.type === 'add',
      );
      const nextNodes = geometryChanged
        ? withBoundaryNodes(
            realNodesOnly(applied),
            state.flowInputs,
            state.flowOutputs,
            applied,
          )
        : applied;

      return {
        nodes: nextNodes,
        isDirty: meaningful ? true : state.isDirty,
      };
    }),

  onEdgesChange: (changes) =>
    set((state) => {
      // 'select'(선택) 변경은 사용자 편집이 아니므로 dirty 에서 제외한다.
      const meaningful = changes.some((c) => c.type !== 'select');
      return {
        edges: applyEdgeChanges(changes, state.edges),
        isDirty: meaningful ? true : state.isDirty,
      };
    }),

  onConnect: (connection) =>
    set((state) => {
      const srcNode = state.nodes.find((n) => n.id === connection.source);
      const tgtNode = state.nodes.find((n) => n.id === connection.target);
      const srcNodeName = (srcNode?.data?.label as string) || srcNode?.id || 'unknown';
      const tgtNodeName = (tgtNode?.data?.label as string) || tgtNode?.id || 'unknown';
      const srcPort = connection.sourceHandle || 'default';
      const tgtPort = connection.targetHandle || 'default';
      const wireName = `${srcNodeName}.${srcPort}_to_${tgtNodeName}.${tgtPort}`;

      // 새 엣지 id 는 UUID 로 부여한다. addEdge 가 id 없는 connection 에는
      // `xy-edge__<source><sourceHandle>-<target>...` 형태의 파생 id 를
      // 생성하는데, 이는 동일 source/target 재연결 시 충돌하고 가져오기
      // id 재생성과도 일관되지 않는다. id 를 명시하면 addEdge 가 이 값을 유지한다.
      // Edge 로 단언해 addEdge 의 제네릭이 Edge[] 로 해석되도록 한다(메타데이터는 추가 속성).
      // 새 엣지 기본값: 큐(버퍼) 모드 + 용량 100.
      // 무버퍼(bypass)는 송신 측이 수신 처리 속도에 동기로 묶여 백프레셔가
      // 즉시 전파되므로, 기본은 가득 차면 대기하는 큐(buffer) 100 으로 둔다.
      // 이후 엣지 속성 패널에서 용량/모드를 개별 조정할 수 있다.
      // SPEC-LINK-001: 새 엣지는 기본 비가상(`virtual: false`) 으로 생성한다.
      // 가상화는 표시 전용 플래그이며 엔진 라우팅에 영향을 주지 않는다. 사용자가
      // 속성 패널에서 토글하기 전까지는 기존과 동일한 연결선으로 렌더된다.
      const edgeWithMeta = {
        ...connection,
        id: generateUUID(),
        name: wireName,
        wire_type: 'simple',
        mode: 'buffer',
        buffer_size: 100,
        virtual: false,
      } as Edge;

      return {
        ...pushUndo(state),
        edges: addEdge(edgeWithMeta, state.edges),
        isDirty: true,
      };
    }),

  addNode: (node) =>
    set((state) => ({
      ...pushUndo(state),
      nodes: [...state.nodes, node],
      isDirty: true,
    })),

  removeNode: (nodeId) =>
    set((state) => {
      // 합성 노드(경계 2종 + 영역)는 일반 노드 삭제로 제거할 수 없다(REQ-SUBFLOW-B04).
      // 포트는 포트 관리 패널의 removeFlowPort 로만 제거한다.
      if (isSyntheticNodeId(nodeId)) return state;
      return {
        ...pushUndo(state),
        nodes: state.nodes.filter((n) => n.id !== nodeId),
        edges: state.edges.filter(
          (e) => e.source !== nodeId && e.target !== nodeId,
        ),
        selectedNodeId:
          state.selectedNodeId === nodeId ? null : state.selectedNodeId,
        isDirty: true,
      };
    }),

  duplicateNodes: (nodeIds) =>
    set((state) => {
      const idSet = new Set(nodeIds);
      const sources = state.nodes.filter((n) => idSet.has(n.id));
      if (sources.length === 0) return state;

      // 현재 모든 노드 라벨을 기준으로 유일한 복제 라벨을 계산한다.
      const existingLabels = collectLabels(state.nodes);
      const duplicates = buildDuplicateNodes(sources, existingLabels);

      return {
        ...pushUndo(state),
        // 원본은 선택 해제하고 복제본만 선택 상태로 추가한다.
        nodes: [
          ...state.nodes.map((n) =>
            idSet.has(n.id) ? { ...n, selected: false } : n,
          ),
          ...duplicates,
        ],
        isDirty: true,
      };
    }),

  // 클립보드는 식별자성 스냅샷일 뿐이므로 dirty / undo 스택을 건드리지 않는다.
  copyToClipboard: (nodeIds) =>
    set((state) => {
      const idSet = new Set(nodeIds);
      const snapshot = state.nodes
        .filter((n) => idSet.has(n.id))
        .map((n) => structuredClone(n));
      return { clipboard: snapshot };
    }),

  pasteClipboard: () =>
    set((state) => {
      if (state.clipboard.length === 0) return state;

      const existingLabels = collectLabels(state.nodes);
      const duplicates = buildDuplicateNodes(state.clipboard, existingLabels);

      return {
        ...pushUndo(state),
        // 기존 선택을 해제하고 붙여넣은 노드만 선택한다.
        nodes: [
          ...state.nodes.map((n) =>
            n.selected ? { ...n, selected: false } : n,
          ),
          ...duplicates,
        ],
        isDirty: true,
        // 클립보드는 그대로 유지해 반복 붙여넣기를 허용한다.
      };
    }),

  updateNodeData: (nodeId, data) =>
    set((state) => ({
      ...pushUndo(state),
      nodes: state.nodes.map((n) =>
        n.id === nodeId ? { ...n, data: { ...n.data, ...data } } : n,
      ),
      isDirty: true,
    })),

  updateEdgeData: (edgeId, data) =>
    set((state) => ({
      ...pushUndo(state),
      edges: state.edges.map((e) =>
        e.id === edgeId ? { ...e, ...data } : e,
      ),
      isDirty: true,
    })),

  selectNode: (nodeId) =>
    set({ selectedNodeId: nodeId, selectedEdgeId: null }),

  selectEdge: (edgeId) =>
    set({ selectedEdgeId: edgeId, selectedNodeId: null }),

  // 같은 이름을 다시 설정하면 토글 해제한다. 표시 전용이므로 dirty/undo 무관.
  setHighlightedLinkName: (name) =>
    set((state) => ({
      highlightedLinkName: state.highlightedLinkName === name ? null : name,
    })),

  undo: () =>
    set((state) => {
      if (state.undoStack.length === 0) return state;

      const undoStack = [...state.undoStack];
      const entry = undoStack.pop()!;

      // Push current state onto redo stack before restoring.
      const redoEntry: HistoryEntry = {
        nodes: state.nodes,
        edges: state.edges,
        flowInputs: state.flowInputs,
        flowOutputs: state.flowOutputs,
      };
      const redoStack = [...state.redoStack, redoEntry];
      if (redoStack.length > MAX_HISTORY) {
        redoStack.shift();
      }

      return {
        nodes: entry.nodes,
        edges: entry.edges,
        flowInputs: entry.flowInputs,
        flowOutputs: entry.flowOutputs,
        undoStack,
        redoStack,
        isDirty: true,
      };
    }),

  redo: () =>
    set((state) => {
      if (state.redoStack.length === 0) return state;

      const redoStack = [...state.redoStack];
      const entry = redoStack.pop()!;

      // Push current state onto undo stack before restoring.
      const undoEntry: HistoryEntry = {
        nodes: state.nodes,
        edges: state.edges,
        flowInputs: state.flowInputs,
        flowOutputs: state.flowOutputs,
      };
      const undoStack = [...state.undoStack, undoEntry];
      if (undoStack.length > MAX_HISTORY) {
        undoStack.shift();
      }

      return {
        nodes: entry.nodes,
        edges: entry.edges,
        flowInputs: entry.flowInputs,
        flowOutputs: entry.flowOutputs,
        undoStack,
        redoStack,
        isDirty: true,
      };
    }),

  clearHistory: () =>
    set({ undoStack: [], redoStack: [] }),

  setDirty: (dirty) =>
    set({ isDirty: dirty }),

  // currentFlowId 는 식별자일 뿐이므로 dirty 나 undo 스택을 건드리지 않는다.
  setCurrentFlowId: (flowId) =>
    set({ currentFlowId: flowId }),

  // 뷰 전용 토글 — 표시 상태만 뒤집고 dirty/undo 스택을 건드리지 않는다.
  toggleShowVirtualWires: () =>
    set((state) => ({ showVirtualWires: !state.showVirtualWires })),

  toggleFocusConnections: () =>
    set((state) => ({
      focusConnectionsOnSelect: !state.focusConnectionsOnSelect,
    })),

  // 뷰 전용 — 유한 값은 [1, FOCUS_DEPTH_MAX] 로 클램프하고 FOCUS_DEPTH_ALL(전체) 은
  // 그대로 둔다. dirty/undo 를 건드리지 않는다.
  setFocusDepth: (depth) =>
    set({ focusDepth: clampFocusDepth(depth) }),

  resetEditor: () =>
    set({
      nodes: [],
      edges: [],
      flowInputs: [],
      flowOutputs: [],
      selectedNodeId: null,
      selectedEdgeId: null,
      highlightedLinkName: null,
      isDirty: false,
      undoStack: [],
      redoStack: [],
      currentFlowId: null,
      clipboard: [],
    }),
}));
