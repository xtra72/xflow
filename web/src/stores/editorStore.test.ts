// editorStore 의 저장/로딩 dirty 플래그 동작 테스트.
// 버그: 저장 후 서버 재조회로 데이터가 다시 로드되면 isDirty 가 true 로
// 되돌아가 저장 버튼 빨간점이 사라지지 않는다. loadFlow 는 서버 로딩 전용
// 액션으로, nodes/edges 를 교체하되 dirty 를 만들지 않아야 한다.

import { beforeEach, describe, expect, it } from 'vitest';
import type { Edge, Node } from '@xyflow/react';

import { nextDuplicateLabel, useEditorStore } from './editorStore';

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

  describe('nextDuplicateLabel (복제 라벨 계산 헬퍼)', () => {
    it('"filter" + {} → "filter-복제1"', () => {
      expect(nextDuplicateLabel('filter', new Set())).toBe('filter-복제1');
    });

    it('"filter" + {"filter-복제1"} → "filter-복제2"', () => {
      expect(nextDuplicateLabel('filter', new Set(['filter-복제1']))).toBe(
        'filter-복제2',
      );
    });

    it('"filter-복제1" 은 stem 을 "filter" 로 벗겨 "filter-복제1" 존재 시 "filter-복제2"', () => {
      expect(
        nextDuplicateLabel('filter-복제1', new Set(['filter-복제1'])),
      ).toBe('filter-복제2');
    });

    it('연속된 복제 라벨이 있으면 비어있는 가장 작은 n 을 찾는다', () => {
      expect(
        nextDuplicateLabel(
          'filter',
          new Set(['filter-복제1', 'filter-복제2']),
        ),
      ).toBe('filter-복제3');
    });
  });

  describe('duplicateNodes (노드 복제)', () => {
    const sourceNode: Node = {
      id: 'src-1',
      type: 'custom',
      position: { x: 100, y: 50 },
      selected: true,
      data: {
        label: 'filter',
        nodeType: 'filter',
        category: 'transform',
        ports: [{ id: 'in', direction: 'input' }],
        config: { threshold: 5 },
        status: 'draft',
        output_enabled: true,
      },
    };

    it('새 노드는 다른 UUID id, "<base>-복제1" 라벨, offset 위치, selected=true 를 가진다', () => {
      useEditorStore.getState().loadFlow([sourceNode], []);
      useEditorStore.getState().duplicateNodes(['src-1']);

      const nodes = useEditorStore.getState().nodes;
      expect(nodes).toHaveLength(2);

      const dup = nodes.find((n) => n.id !== 'src-1');
      if (!dup) throw new Error('복제 노드가 생성되지 않았습니다');

      // 새 UUID (원본과 다른 id, UUID 형식)
      expect(dup.id).not.toBe('src-1');
      // 라벨
      expect(dup.data.label).toBe('filter-복제1');
      // offset 위치 (+32, +32)
      expect(dup.position).toEqual({ x: 132, y: 82 });
      // 복제본은 선택, 원본은 해제
      expect(dup.selected).toBe(true);
      const original = nodes.find((n) => n.id === 'src-1');
      expect(original?.selected).toBe(false);
    });

    it('label 외의 data 속성은 동일하게 유지된다', () => {
      useEditorStore.getState().loadFlow([sourceNode], []);
      useEditorStore.getState().duplicateNodes(['src-1']);

      const dup = useEditorStore
        .getState()
        .nodes.find((n) => n.id !== 'src-1');
      if (!dup) throw new Error('복제 노드가 생성되지 않았습니다');

      expect(dup.data.nodeType).toBe('filter');
      expect(dup.data.category).toBe('transform');
      expect(dup.data.config).toEqual({ threshold: 5 });
      expect(dup.data.status).toBe('draft');
      expect(dup.data.output_enabled).toBe(true);
      // 깊은 복제이므로 config 참조가 분리되어야 한다.
      expect(dup.data.config).not.toBe(sourceNode.data.config);
    });

    it('undo 히스토리를 쌓고 isDirty 를 true 로 만든다', () => {
      useEditorStore.getState().loadFlow([sourceNode], []);
      expect(useEditorStore.getState().isDirty).toBe(false);

      useEditorStore.getState().duplicateNodes(['src-1']);

      const state = useEditorStore.getState();
      expect(state.isDirty).toBe(true);
      expect(state.undoStack.length).toBeGreaterThan(0);
    });

    it('빈 배열이면 no-op 이다', () => {
      useEditorStore.getState().loadFlow([sourceNode], []);
      useEditorStore.getState().duplicateNodes([]);
      expect(useEditorStore.getState().nodes).toHaveLength(1);
      expect(useEditorStore.getState().isDirty).toBe(false);
    });

    it('다중 선택 복제 시 상대 위치를 동일 delta 로 보존한다', () => {
      const nodeA: Node = {
        id: 'a',
        type: 'custom',
        position: { x: 0, y: 0 },
        data: { label: 'a' },
      };
      const nodeB: Node = {
        id: 'b',
        type: 'custom',
        position: { x: 100, y: 200 },
        data: { label: 'b' },
      };
      useEditorStore.getState().loadFlow([nodeA, nodeB], []);
      useEditorStore.getState().duplicateNodes(['a', 'b']);

      const nodes = useEditorStore.getState().nodes;
      const dupA = nodes.find((n) => n.data.label === 'a-복제1');
      const dupB = nodes.find((n) => n.data.label === 'b-복제1');
      if (!dupA || !dupB) throw new Error('복제 노드가 누락되었습니다');

      // 두 복제본 모두 동일 delta(+32,+32) 적용 → 상대 위치 유지.
      expect(dupA.position).toEqual({ x: 32, y: 32 });
      expect(dupB.position).toEqual({ x: 132, y: 232 });
    });
  });

  describe('copyToClipboard / pasteClipboard', () => {
    const sourceNode: Node = {
      id: 'src-1',
      type: 'custom',
      position: { x: 10, y: 10 },
      data: { label: 'filter', nodeType: 'filter' },
    };

    it('붙여넣기는 "-복제1" 노드를 만들고, 두 번째 붙여넣기는 "-복제2" 로 증가한다', () => {
      useEditorStore.getState().loadFlow([sourceNode], []);
      useEditorStore.getState().copyToClipboard(['src-1']);

      // 1차 붙여넣기 → -복제1
      useEditorStore.getState().pasteClipboard();
      let labels = useEditorStore
        .getState()
        .nodes.map((n) => n.data.label as string);
      expect(labels).toContain('filter-복제1');

      // 2차 붙여넣기 → 현재 라벨 기준으로 -복제2
      useEditorStore.getState().pasteClipboard();
      labels = useEditorStore
        .getState()
        .nodes.map((n) => n.data.label as string);
      expect(labels).toContain('filter-복제2');
      expect(useEditorStore.getState().nodes).toHaveLength(3);
    });

    it('클립보드는 undo/dirty 의 영향을 받지 않는다', () => {
      useEditorStore.getState().loadFlow([sourceNode], []);
      useEditorStore.getState().copyToClipboard(['src-1']);

      // copyToClipboard 자체는 dirty / undo 를 만들지 않는다.
      expect(useEditorStore.getState().isDirty).toBe(false);
      expect(useEditorStore.getState().undoStack).toHaveLength(0);
      expect(useEditorStore.getState().clipboard).toHaveLength(1);

      // 붙여넣기 후 undo 해도 클립보드는 유지된다.
      useEditorStore.getState().pasteClipboard();
      expect(useEditorStore.getState().isDirty).toBe(true);
      useEditorStore.getState().undo();
      expect(useEditorStore.getState().clipboard).toHaveLength(1);
    });

    it('빈 클립보드 붙여넣기는 no-op 이다', () => {
      useEditorStore.getState().loadFlow([sourceNode], []);
      useEditorStore.getState().pasteClipboard();
      expect(useEditorStore.getState().nodes).toHaveLength(1);
      expect(useEditorStore.getState().isDirty).toBe(false);
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
