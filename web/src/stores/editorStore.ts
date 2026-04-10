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

const MAX_HISTORY = 50;

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
}

interface EditorActions {
  setNodes: (nodes: Node[]) => void;
  setEdges: (edges: Edge[]) => void;
  onNodesChange: (changes: NodeChange[]) => void;
  onEdgesChange: (changes: EdgeChange[]) => void;
  onConnect: (connection: Connection) => void;
  addNode: (node: Node) => void;
  removeNode: (nodeId: string) => void;
  updateNodeData: (nodeId: string, data: Record<string, unknown>) => void;
  updateEdgeData: (edgeId: string, data: Record<string, unknown>) => void;
  selectNode: (nodeId: string | null) => void;
  selectEdge: (edgeId: string | null) => void;
  undo: () => void;
  redo: () => void;
  clearHistory: () => void;
  setDirty: (dirty: boolean) => void;
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

  // Actions
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
    set((state) => ({
      nodes: applyNodeChanges(changes, state.nodes),
      isDirty: true,
    })),

  onEdgesChange: (changes) =>
    set((state) => ({
      edges: applyEdgeChanges(changes, state.edges),
      isDirty: true,
    })),

  onConnect: (connection) =>
    set((state) => {
      const srcNode = state.nodes.find((n) => n.id === connection.source);
      const tgtNode = state.nodes.find((n) => n.id === connection.target);
      const srcNodeName = (srcNode?.data?.label as string) || srcNode?.id || 'unknown';
      const tgtNodeName = (tgtNode?.data?.label as string) || tgtNode?.id || 'unknown';
      const srcPort = connection.sourceHandle || 'default';
      const tgtPort = connection.targetHandle || 'default';
      const wireName = `${srcNodeName}.${srcPort}_to_${tgtNodeName}.${tgtPort}`;

      const edgeWithMeta = {
        ...connection,
        name: wireName,
        wire_type: 'simple',
        mode: 'bypass',
        buffer_size: 0,
      };

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

  resetEditor: () =>
    set({
      nodes: [],
      edges: [],
      selectedNodeId: null,
      selectedEdgeId: null,
      isDirty: false,
      undoStack: [],
      redoStack: [],
    }),
}));
