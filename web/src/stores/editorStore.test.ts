// editorStore 의 저장/로딩 dirty 플래그 동작 테스트.
// 버그: 저장 후 서버 재조회로 데이터가 다시 로드되면 isDirty 가 true 로
// 되돌아가 저장 버튼 빨간점이 사라지지 않는다. loadFlow 는 서버 로딩 전용
// 액션으로, nodes/edges 를 교체하되 dirty 를 만들지 않아야 한다.

import { beforeEach, describe, expect, it } from 'vitest';
import type { Edge, Node } from '@xyflow/react';

import { useEditorStore } from './editorStore';

const sampleNodes: Node[] = [
  { id: 'n1', type: 'custom', position: { x: 0, y: 0 }, data: { label: 'A' } },
];
const sampleEdges: Edge[] = [
  { id: 'e1', source: 'n1', target: 'n2' },
];

/** 각 테스트 전에 스토어를 초기 상태로 되돌린다. */
function resetStore(): void {
  useEditorStore.getState().resetEditor();
}

describe('editorStore - 저장/로딩 dirty 플래그', () => {
  beforeEach(() => {
    resetStore();
  });

  describe('loadFlow (서버 로딩 전용)', () => {
    it('nodes/edges 를 교체하고 isDirty 를 false 로 유지한다', () => {
      const { loadFlow } = useEditorStore.getState();
      loadFlow(sampleNodes, sampleEdges);

      const state = useEditorStore.getState();
      expect(state.nodes).toEqual(sampleNodes);
      expect(state.edges).toEqual(sampleEdges);
      expect(state.isDirty).toBe(false);
    });

    it('undo/redo 히스토리를 초기화한다 (로딩은 편집 이력이 아니다)', () => {
      const store = useEditorStore.getState();
      // 편집으로 히스토리를 쌓는다.
      store.addNode({
        id: 'n0',
        type: 'custom',
        position: { x: 1, y: 1 },
        data: {},
      });
      expect(useEditorStore.getState().undoStack.length).toBeGreaterThan(0);

      useEditorStore.getState().loadFlow(sampleNodes, sampleEdges);

      const after = useEditorStore.getState();
      expect(after.undoStack).toHaveLength(0);
      expect(after.redoStack).toHaveLength(0);
    });

    it('저장 직후 서버 재조회를 흉내내도 isDirty 가 false 로 유지된다', () => {
      const store = useEditorStore.getState();

      // 1) 사용자가 노드를 추가해 편집 → dirty
      store.addNode({
        id: 'n9',
        type: 'custom',
        position: { x: 5, y: 5 },
        data: {},
      });
      expect(useEditorStore.getState().isDirty).toBe(true);

      // 2) 저장 성공 → setDirty(false)
      useEditorStore.getState().setDirty(false);
      expect(useEditorStore.getState().isDirty).toBe(false);

      // 3) invalidateQueries 로 인한 재조회 → 동일 데이터 재로딩
      useEditorStore.getState().loadFlow(sampleNodes, sampleEdges);

      // 빨간점이 다시 켜지면 안 된다.
      expect(useEditorStore.getState().isDirty).toBe(false);
    });
  });

  describe('setNodes/setEdges (사용자 편집)', () => {
    it('setNodes 는 사용자 편집이므로 isDirty 를 true 로 만든다', () => {
      useEditorStore.getState().setDirty(false);
      useEditorStore.getState().setNodes(sampleNodes);
      expect(useEditorStore.getState().isDirty).toBe(true);
    });

    it('setEdges 는 사용자 편집이므로 isDirty 를 true 로 만든다', () => {
      useEditorStore.getState().setDirty(false);
      useEditorStore.getState().setEdges(sampleEdges);
      expect(useEditorStore.getState().isDirty).toBe(true);
    });
  });

  describe('onConnect (엣지 생성)', () => {
    it('새 엣지 id 는 UUID 로 부여된다 (xy-edge 파생 id 미사용)', () => {
      const twoNodes: Node[] = [
        { id: 'n1', type: 'custom', position: { x: 0, y: 0 }, data: { label: 'A' } },
        { id: 'n2', type: 'custom', position: { x: 200, y: 0 }, data: { label: 'B' } },
      ];
      useEditorStore.getState().loadFlow(twoNodes, []);
      useEditorStore.getState().onConnect({
        source: 'n1',
        target: 'n2',
        sourceHandle: 'out',
        targetHandle: 'in',
      });
      const edges = useEditorStore.getState().edges;
      expect(edges).toHaveLength(1);
      const edge = edges[0];
      if (!edge) throw new Error('엣지가 생성되지 않았습니다');
      // UUID v4 형식이며 React Flow 파생 'xy-edge__' 형식이 아니어야 한다.
      expect(edge.id.startsWith('xy-edge__')).toBe(false);
      expect(edge.id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i);
      // 메타데이터 보존 확인.
      expect((edge as Record<string, unknown>).wire_type).toBe('simple');
    });
  });

  describe('currentFlowId (라이브 제어용 식별자)', () => {
    it('초기값은 null 이다', () => {
      expect(useEditorStore.getState().currentFlowId).toBeNull();
    });

    it('setCurrentFlowId 가 값을 설정한다', () => {
      useEditorStore.getState().setCurrentFlowId('flow-42');
      expect(useEditorStore.getState().currentFlowId).toBe('flow-42');
    });

    it('setCurrentFlowId 는 isDirty / 히스토리에 영향을 주지 않는다', () => {
      useEditorStore.getState().setDirty(false);
      useEditorStore.getState().setCurrentFlowId('flow-7');
      const state = useEditorStore.getState();
      expect(state.isDirty).toBe(false);
      expect(state.undoStack).toHaveLength(0);
      expect(state.redoStack).toHaveLength(0);
    });

    it('resetEditor 가 currentFlowId 를 null 로 초기화한다', () => {
      useEditorStore.getState().setCurrentFlowId('flow-99');
      expect(useEditorStore.getState().currentFlowId).toBe('flow-99');

      useEditorStore.getState().resetEditor();
      expect(useEditorStore.getState().currentFlowId).toBeNull();
    });
  });
});
