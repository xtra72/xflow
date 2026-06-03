// editorStore 의 저장/로딩 dirty 플래그 동작 테스트.
// 버그: 저장 후 서버 재조회로 데이터가 다시 로드되면 isDirty 가 true 로
// 되돌아가 저장 버튼 빨간점이 사라지지 않는다. loadFlow 는 서버 로딩 전용
// 액션으로, nodes/edges 를 교체하되 dirty 를 만들지 않아야 한다.

import { beforeEach, describe, expect, it } from 'vitest';
import type { Edge, EdgeChange, Node, NodeChange } from '@xyflow/react';

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

    it('새 엣지 기본값은 큐(buffer) 모드 + 용량 100 이다', () => {
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
      const edge = useEditorStore.getState().edges[0];
      if (!edge) throw new Error('엣지가 생성되지 않았습니다');
      expect((edge as Record<string, unknown>).mode).toBe('buffer');
      expect((edge as Record<string, unknown>).buffer_size).toBe(100);
    });
  });

  describe('updateEdgeData (엣지 큐 설정 편집)', () => {
    function seedSingleEdge(): string {
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
      const edge = useEditorStore.getState().edges[0];
      if (!edge) throw new Error('엣지가 생성되지 않았습니다');
      return edge.id;
    }

    it('mode/buffer_size 를 갱신하고 dirty 로 표시한다', () => {
      const edgeId = seedSingleEdge();
      // loadFlow 직후 dirty=false 상태에서 onConnect 가 dirty 를 만들므로 먼저 초기화.
      useEditorStore.getState().setDirty(false);

      useEditorStore.getState().updateEdgeData(edgeId, {
        mode: 'drop_oldest',
        buffer_size: 50,
      });

      const edge = useEditorStore.getState().edges[0]!;
      expect((edge as Record<string, unknown>).mode).toBe('drop_oldest');
      expect((edge as Record<string, unknown>).buffer_size).toBe(50);
      // 다른 엣지 메타데이터(name/wire_type)는 보존된다.
      expect((edge as Record<string, unknown>).wire_type).toBe('simple');
      expect(useEditorStore.getState().isDirty).toBe(true);
    });

    it('무버퍼(bypass) 로 전환 시 buffer_size 0 을 반영한다', () => {
      const edgeId = seedSingleEdge();
      useEditorStore.getState().updateEdgeData(edgeId, {
        mode: 'bypass',
        buffer_size: 0,
      });
      const edge = useEditorStore.getState().edges[0]!;
      expect((edge as Record<string, unknown>).mode).toBe('bypass');
      expect((edge as Record<string, unknown>).buffer_size).toBe(0);
    });
  });

  describe('SPEC-LINK-001 가상 링크', () => {
    function seedSingleEdge(): string {
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
      const edge = useEditorStore.getState().edges[0];
      if (!edge) throw new Error('엣지가 생성되지 않았습니다');
      return edge.id;
    }

    it('onConnect 로 생성한 새 엣지는 virtual=false 기본값을 가진다', () => {
      seedSingleEdge();
      const edge = useEditorStore.getState().edges[0]!;
      expect((edge as Record<string, unknown>).virtual).toBe(false);
    });

    it('updateEdgeData 로 virtual 을 토글하고 dirty 로 표시한다', () => {
      const edgeId = seedSingleEdge();
      useEditorStore.getState().setDirty(false);

      useEditorStore.getState().updateEdgeData(edgeId, { virtual: true });

      const edge = useEditorStore.getState().edges[0]!;
      expect((edge as Record<string, unknown>).virtual).toBe(true);
      // 다른 메타데이터는 보존된다.
      expect((edge as Record<string, unknown>).mode).toBe('buffer');
      expect(useEditorStore.getState().isDirty).toBe(true);
    });

    it('updateEdgeData 로 링크 이름(name)을 편집하고 dirty 로 표시한다', () => {
      const edgeId = seedSingleEdge();
      useEditorStore.getState().setDirty(false);

      useEditorStore.getState().updateEdgeData(edgeId, { name: 'sensor' });

      const edge = useEditorStore.getState().edges[0]!;
      expect((edge as Record<string, unknown>).name).toBe('sensor');
      expect(useEditorStore.getState().isDirty).toBe(true);
    });

    it('setHighlightedLinkName 은 이름을 설정하고 dirty 를 만들지 않는다', () => {
      seedSingleEdge();
      useEditorStore.getState().setDirty(false);

      useEditorStore.getState().setHighlightedLinkName('sensor');

      expect(useEditorStore.getState().highlightedLinkName).toBe('sensor');
      // 하이라이트는 표시 전용이므로 dirty 가 되지 않는다.
      expect(useEditorStore.getState().isDirty).toBe(false);
    });

    it('setHighlightedLinkName 에 같은 이름을 다시 주면 토글 해제(null)된다', () => {
      seedSingleEdge();
      useEditorStore.getState().setHighlightedLinkName('sensor');
      useEditorStore.getState().setHighlightedLinkName('sensor');
      expect(useEditorStore.getState().highlightedLinkName).toBeNull();
    });

    it('loadFlow 는 하이라이트를 초기화한다', () => {
      seedSingleEdge();
      useEditorStore.getState().setHighlightedLinkName('sensor');
      useEditorStore.getState().loadFlow(sampleNodes, sampleEdges);
      expect(useEditorStore.getState().highlightedLinkName).toBeNull();
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

describe('editorStore - onNodesChange/onEdgesChange dirty 처리', () => {
  const oneNode: Node[] = [
    { id: 'n1', type: 'custom', position: { x: 0, y: 0 }, data: { label: 'A' } },
  ];

  beforeEach(() => {
    useEditorStore.getState().resetEditor();
  });

  it("'dimensions'(측정) 변경은 isDirty 를 만들지 않는다 (로드 직후 수정됨 표시 버그 방지)", () => {
    useEditorStore.getState().loadFlow(oneNode, []);
    expect(useEditorStore.getState().isDirty).toBe(false);

    useEditorStore.getState().onNodesChange([
      { id: 'n1', type: 'dimensions', dimensions: { width: 120, height: 60 } } as NodeChange,
    ]);
    expect(useEditorStore.getState().isDirty).toBe(false);
  });

  it("'select'(선택) 변경은 isDirty 를 만들지 않는다", () => {
    useEditorStore.getState().loadFlow(oneNode, []);
    useEditorStore.getState().onNodesChange([
      { id: 'n1', type: 'select', selected: true } as NodeChange,
    ]);
    expect(useEditorStore.getState().isDirty).toBe(false);
  });

  it("'position'(이동) 같은 실제 편집 변경은 isDirty 를 true 로 만든다", () => {
    useEditorStore.getState().loadFlow(oneNode, []);
    useEditorStore.getState().onNodesChange([
      { id: 'n1', type: 'position', position: { x: 50, y: 50 }, dragging: false } as NodeChange,
    ]);
    expect(useEditorStore.getState().isDirty).toBe(true);
  });

  it("edge 'select' 변경은 isDirty 를 만들지 않는다", () => {
    useEditorStore.getState().loadFlow(oneNode, [
      { id: 'e1', source: 'n1', target: 'n1' },
    ]);
    useEditorStore.getState().onEdgesChange([
      { id: 'e1', type: 'select', selected: true } as EdgeChange,
    ]);
    expect(useEditorStore.getState().isDirty).toBe(false);
  });
});

describe('editorStore - 뷰 전용 표시 토글 (showVirtualWires / focusConnectionsOnSelect)', () => {
  const oneNode: Node[] = [
    { id: 'n1', type: 'custom', position: { x: 0, y: 0 }, data: { label: 'A' } },
  ];

  beforeEach(() => {
    useEditorStore.getState().resetEditor();
    // resetEditor 는 뷰 토글/단계를 의도적으로 보존하므로(플로우 무관 선호),
    // 테스트 간 격리를 위해 명시적으로 기본값으로 되돌린다.
    useEditorStore.setState({
      showVirtualWires: false,
      focusConnectionsOnSelect: false,
      focusDepth: 1,
    });
  });

  it('두 토글의 초기값은 false 이다 (기존 동작 유지)', () => {
    const state = useEditorStore.getState();
    expect(state.showVirtualWires).toBe(false);
    expect(state.focusConnectionsOnSelect).toBe(false);
  });

  it('toggleShowVirtualWires 는 값을 뒤집는다', () => {
    useEditorStore.getState().toggleShowVirtualWires();
    expect(useEditorStore.getState().showVirtualWires).toBe(true);
    useEditorStore.getState().toggleShowVirtualWires();
    expect(useEditorStore.getState().showVirtualWires).toBe(false);
  });

  it('toggleFocusConnections 는 값을 뒤집는다', () => {
    useEditorStore.getState().toggleFocusConnections();
    expect(useEditorStore.getState().focusConnectionsOnSelect).toBe(true);
    useEditorStore.getState().toggleFocusConnections();
    expect(useEditorStore.getState().focusConnectionsOnSelect).toBe(false);
  });

  it('토글은 isDirty / undo / redo 스택에 영향을 주지 않는다 (뷰 전용)', () => {
    useEditorStore.getState().setDirty(false);

    useEditorStore.getState().toggleShowVirtualWires();
    useEditorStore.getState().toggleFocusConnections();

    const state = useEditorStore.getState();
    expect(state.isDirty).toBe(false);
    expect(state.undoStack).toHaveLength(0);
    expect(state.redoStack).toHaveLength(0);
  });

  it('loadFlow 는 뷰 토글을 초기화하지 않는다 (플로우와 무관한 사용자 선호)', () => {
    useEditorStore.getState().toggleShowVirtualWires();
    useEditorStore.getState().toggleFocusConnections();

    useEditorStore.getState().loadFlow(oneNode, []);

    const state = useEditorStore.getState();
    expect(state.showVirtualWires).toBe(true);
    expect(state.focusConnectionsOnSelect).toBe(true);
  });

  describe('focusDepth (연결 단계, 뷰 전용)', () => {
    it('초기값은 1 이다 (기존 1-hop 동작)', () => {
      expect(useEditorStore.getState().focusDepth).toBe(1);
    });

    it('setFocusDepth 는 값을 설정한다', () => {
      useEditorStore.getState().setFocusDepth(3);
      expect(useEditorStore.getState().focusDepth).toBe(3);
    });

    it('하한(1) 미만은 1 로 클램프한다', () => {
      useEditorStore.getState().setFocusDepth(0);
      expect(useEditorStore.getState().focusDepth).toBe(1);
      useEditorStore.getState().setFocusDepth(-5);
      expect(useEditorStore.getState().focusDepth).toBe(1);
    });

    it('상한(5) 초과는 5 로 클램프한다', () => {
      useEditorStore.getState().setFocusDepth(6);
      expect(useEditorStore.getState().focusDepth).toBe(5);
      useEditorStore.getState().setFocusDepth(99);
      expect(useEditorStore.getState().focusDepth).toBe(5);
    });

    it('비정수는 내림 후 클램프한다', () => {
      useEditorStore.getState().setFocusDepth(2.9);
      expect(useEditorStore.getState().focusDepth).toBe(2);
    });

    it('NaN/Infinity 는 안전하게 하한(1) 으로 처리한다', () => {
      useEditorStore.getState().setFocusDepth(Number.NaN);
      expect(useEditorStore.getState().focusDepth).toBe(1);
      useEditorStore.getState().setFocusDepth(Number.POSITIVE_INFINITY);
      expect(useEditorStore.getState().focusDepth).toBe(5);
    });

    it('setFocusDepth 는 isDirty / undo / redo 에 영향을 주지 않는다 (뷰 전용)', () => {
      useEditorStore.getState().setDirty(false);

      useEditorStore.getState().setFocusDepth(4);

      const state = useEditorStore.getState();
      expect(state.isDirty).toBe(false);
      expect(state.undoStack).toHaveLength(0);
      expect(state.redoStack).toHaveLength(0);
    });

    it('loadFlow 는 focusDepth 를 초기화하지 않는다 (플로우와 무관한 사용자 선호)', () => {
      useEditorStore.getState().setFocusDepth(3);

      useEditorStore.getState().loadFlow(oneNode, []);

      expect(useEditorStore.getState().focusDepth).toBe(3);
    });
  });
});
