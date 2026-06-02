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
}

interface EditorState {
  nodes: Node[];
  edges: Edge[];
  selectedNodeId: string | null;
  selectedEdgeId: string | null;
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
}

interface EditorActions {
  loadFlow: (nodes: Node[], edges: Edge[]) => void;
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
  undo: () => void;
  redo: () => void;
  clearHistory: () => void;
  setDirty: (dirty: boolean) => void;
  /** 현재 편집 중인 플로우 ID 설정 (hydration 시). dirty/undo 에 영향 없음. */
  setCurrentFlowId: (flowId: string | null) => void;
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
  };
  const stack = [...state.undoStack, entry];
  if (stack.length > MAX_HISTORY) {
    stack.shift();
  }
  return { undoStack: stack, redoStack: [] };
}

export const useEditorStore = create<EditorState & EditorActions>()((set) => ({
  // State
  nodes: [],
  edges: [],
  selectedNodeId: null,
  selectedEdgeId: null,
  isDirty: false,
  undoStack: [],
  redoStack: [],
  currentFlowId: null,
  clipboard: [],

  // Actions

  // Server-load only: replace nodes/edges WITHOUT marking the editor dirty
  // and reset history. Use this when hydrating from server data (initial load
  // or post-save refetch) so the unsaved indicator does not turn back on.
  loadFlow: (nodes, edges) =>
    set({
      nodes,
      edges,
      isDirty: false,
      undoStack: [],
      redoStack: [],
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
      // 'dimensions'(노드 측정) 와 'select'(선택) 변경은 사용자 편집이 아니므로
      // isDirty 를 만들지 않는다. React Flow 는 마운트/렌더 시 노드를 측정하며
      // 'dimensions' 변경을 emit 하는데, 이를 dirty 로 처리하면 플로우 로드 직후
      // 편집 없이도 무조건 "수정됨" 으로 표시되는 버그가 발생한다(미저장 가드 오작동).
      const meaningful = changes.some(
        (c) => c.type !== 'dimensions' && c.type !== 'select',
      );
      return {
        nodes: applyNodeChanges(changes, state.nodes),
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
      const edgeWithMeta = {
        ...connection,
        id: generateUUID(),
        name: wireName,
        wire_type: 'simple',
        mode: 'bypass',
        buffer_size: 0,
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
    set((state) => ({
      ...pushUndo(state),
      nodes: state.nodes.filter((n) => n.id !== nodeId),
      edges: state.edges.filter(
        (e) => e.source !== nodeId && e.target !== nodeId,
      ),
      selectedNodeId: state.selectedNodeId === nodeId ? null : state.selectedNodeId,
      isDirty: true,
    })),

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

  undo: () =>
    set((state) => {
      if (state.undoStack.length === 0) return state;

      const undoStack = [...state.undoStack];
      const entry = undoStack.pop()!;

      // Push current state onto redo stack before restoring.
      const redoEntry: HistoryEntry = {
        nodes: state.nodes,
        edges: state.edges,
      };
      const redoStack = [...state.redoStack, redoEntry];
      if (redoStack.length > MAX_HISTORY) {
        redoStack.shift();
      }

      return {
        nodes: entry.nodes,
        edges: entry.edges,
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
      };
      const undoStack = [...state.undoStack, undoEntry];
      if (undoStack.length > MAX_HISTORY) {
        undoStack.shift();
      }

      return {
        nodes: entry.nodes,
        edges: entry.edges,
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

  resetEditor: () =>
    set({
      nodes: [],
      edges: [],
      selectedNodeId: null,
      selectedEdgeId: null,
      isDirty: false,
      undoStack: [],
      redoStack: [],
      currentFlowId: null,
      clipboard: [],
    }),
}));
