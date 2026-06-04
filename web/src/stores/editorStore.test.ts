// editorStore 의 저장/로딩 dirty 플래그 동작 테스트.
// 버그: 저장 후 서버 재조회로 데이터가 다시 로드되면 isDirty 가 true 로
// 되돌아가 저장 버튼 빨간점이 사라지지 않는다. loadFlow 는 서버 로딩 전용
// 액션으로, nodes/edges 를 교체하되 dirty 를 만들지 않아야 한다.

import { beforeEach, describe, expect, it } from 'vitest';
import type { Edge, EdgeChange, Node, NodeChange } from '@xyflow/react';

import {
  FOCUS_DEPTH_ALL,
  clampFocusDepth,
  nextDuplicateLabel,
  useEditorStore,
} from './editorStore';
import {
  FLOW_AREA_NODE_ID,
  FLOW_INPUT_BOUNDARY_ID,
  FLOW_OUTPUT_BOUNDARY_ID,
  isBoundaryNode,
  realNodesOnly,
} from '@/lib/flow/boundary';

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

    it('NaN 은 안전하게 하한(1) 으로 처리한다', () => {
      useEditorStore.getState().setFocusDepth(Number.NaN);
      expect(useEditorStore.getState().focusDepth).toBe(1);
    });

    it('-Infinity 는 하한(1) 으로 클램프한다', () => {
      useEditorStore.getState().setFocusDepth(Number.NEGATIVE_INFINITY);
      expect(useEditorStore.getState().focusDepth).toBe(1);
    });

    it('FOCUS_DEPTH_ALL(=+Infinity) 은 전체 센티넬로 그대로 유지한다', () => {
      useEditorStore.getState().setFocusDepth(FOCUS_DEPTH_ALL);
      expect(useEditorStore.getState().focusDepth).toBe(FOCUS_DEPTH_ALL);
      expect(useEditorStore.getState().focusDepth).toBe(
        Number.POSITIVE_INFINITY,
      );
    });

    it('전체에서 다시 유한 값으로 되돌릴 수 있다', () => {
      useEditorStore.getState().setFocusDepth(FOCUS_DEPTH_ALL);
      useEditorStore.getState().setFocusDepth(5);
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

  describe('clampFocusDepth (순수 함수)', () => {
    it('유한 값은 [1, 5] 로 클램프한다', () => {
      expect(clampFocusDepth(0)).toBe(1);
      expect(clampFocusDepth(-5)).toBe(1);
      expect(clampFocusDepth(1)).toBe(1);
      expect(clampFocusDepth(3)).toBe(3);
      expect(clampFocusDepth(5)).toBe(5);
      expect(clampFocusDepth(6)).toBe(5);
      expect(clampFocusDepth(99)).toBe(5);
    });

    it('유한 비정수는 내림 후 클램프한다', () => {
      expect(clampFocusDepth(2.9)).toBe(2);
      expect(clampFocusDepth(4.1)).toBe(4);
    });

    it('NaN 은 하한(1) 으로 처리한다', () => {
      expect(clampFocusDepth(Number.NaN)).toBe(1);
    });

    it('-Infinity 는 하한(1) 으로 처리한다', () => {
      expect(clampFocusDepth(Number.NEGATIVE_INFINITY)).toBe(1);
    });

    it('FOCUS_DEPTH_ALL(=+Infinity) 은 5 로 내리지 않고 그대로 통과시킨다', () => {
      expect(clampFocusDepth(FOCUS_DEPTH_ALL)).toBe(FOCUS_DEPTH_ALL);
      expect(clampFocusDepth(Number.POSITIVE_INFINITY)).toBe(
        Number.POSITIVE_INFINITY,
      );
    });
  });
});

// ---------------------------------------------------------------------------
// SPEC-SUBFLOW-001 그룹 A/B: 플로우 레벨 포트 액션 + 경계 노드 합성
// ---------------------------------------------------------------------------

describe('editorStore - 플로우 레벨 포트(SPEC-SUBFLOW-001)', () => {
  beforeEach(() => {
    resetStore();
  });

  describe('포트 추가/이름/삭제 (dirty + undo)', () => {
    it('addFlowInput / addFlowOutput 는 고유 id + 기본 이름 포트를 추가하고 dirty 로 만든다', () => {
      const s = useEditorStore.getState();
      s.addFlowInput();
      s.addFlowInput();
      s.addFlowOutput();

      const state = useEditorStore.getState();
      expect(state.flowInputs.map((p) => p.name)).toEqual(['in1', 'in2']);
      expect(state.flowOutputs.map((p) => p.name)).toEqual(['out1']);
      // id 는 고유해야 한다.
      const ids = state.flowInputs.map((p) => p.id);
      expect(new Set(ids).size).toBe(ids.length);
      expect(state.isDirty).toBe(true);
    });

    it('포트 추가는 undo 스택에 쌓이고 undo 로 되돌릴 수 있다', () => {
      const s = useEditorStore.getState();
      s.addFlowInput();
      expect(useEditorStore.getState().undoStack.length).toBeGreaterThan(0);

      useEditorStore.getState().undo();
      expect(useEditorStore.getState().flowInputs).toHaveLength(0);

      useEditorStore.getState().redo();
      expect(useEditorStore.getState().flowInputs).toHaveLength(1);
    });

    it('renameFlowPort 는 id 를 유지한 채 이름만 갱신한다', () => {
      const s = useEditorStore.getState();
      s.addFlowInput();
      const portId = useEditorStore.getState().flowInputs[0]!.id;

      useEditorStore.getState().renameFlowPort('input', portId, '센서입력');

      const port = useEditorStore.getState().flowInputs[0]!;
      expect(port.id).toBe(portId);
      expect(port.name).toBe('센서입력');
    });

    it('renameFlowPort 는 빈 이름/동일 이름이면 dirty 를 만들지 않는다', () => {
      const s = useEditorStore.getState();
      s.addFlowInput();
      const portId = useEditorStore.getState().flowInputs[0]!.id;
      // 로드처럼 dirty 를 끈 뒤 무의미 rename 을 시도한다.
      useEditorStore.getState().setDirty(false);

      useEditorStore.getState().renameFlowPort('input', portId, '   ');
      expect(useEditorStore.getState().isDirty).toBe(false);

      useEditorStore.getState().renameFlowPort('input', portId, 'in1');
      expect(useEditorStore.getState().isDirty).toBe(false);
    });

    it('removeFlowPort 는 포트를 제거하고 dirty 로 만든다', () => {
      const s = useEditorStore.getState();
      s.addFlowOutput();
      const portId = useEditorStore.getState().flowOutputs[0]!.id;
      useEditorStore.getState().setDirty(false);

      useEditorStore.getState().removeFlowPort('output', portId);
      expect(useEditorStore.getState().flowOutputs).toHaveLength(0);
      expect(useEditorStore.getState().isDirty).toBe(true);
    });
  });

  describe('loadFlow 가 포트로부터 경계 노드를 만든다', () => {
    it('flowInputs/flowOutputs 로부터 두 경계 노드를 렌더용 nodes 에 넣는다', () => {
      useEditorStore.getState().loadFlow(
        [{ id: 'n1', type: 'custom', position: { x: 0, y: 0 }, data: {} }],
        [],
        [{ id: 'i1', name: 'in1' }],
        [{ id: 'o1', name: 'out1' }],
      );

      const state = useEditorStore.getState();
      const ids = state.nodes.map((n) => n.id);
      expect(ids).toContain(FLOW_INPUT_BOUNDARY_ID);
      expect(ids).toContain(FLOW_OUTPUT_BOUNDARY_ID);
      expect(ids).toContain('n1');
      expect(state.isDirty).toBe(false);

      const inputBoundary = state.nodes.find(
        (n) => n.id === FLOW_INPUT_BOUNDARY_ID,
      )!;
      expect((inputBoundary.data as { ports: string[] }).ports).toEqual(['in1']);
    });

    it('loadFlow 를 2-인자로 호출하면(기존 호출부) 포트가 없으므로 경계 노드도 없다', () => {
      useEditorStore.getState().loadFlow(
        [{ id: 'n1', type: 'custom', position: { x: 0, y: 0 }, data: {} }],
        [],
      );
      const state = useEditorStore.getState();
      expect(state.flowInputs).toEqual([]);
      expect(state.flowOutputs).toEqual([]);
      // 포트가 없으면 경계 노드를 추가하지 않아 기존 노드 수 의미를 보존한다.
      const boundary = state.nodes.filter(isBoundaryNode);
      expect(boundary).toHaveLength(0);
      expect(state.nodes).toHaveLength(1);
    });

    it('포트 편집 시 경계 노드 핸들이 재구성된다', () => {
      useEditorStore.getState().loadFlow([], [], [], []);
      useEditorStore.getState().addFlowInput();

      const inputBoundary = useEditorStore
        .getState()
        .nodes.find((n) => n.id === FLOW_INPUT_BOUNDARY_ID)!;
      expect((inputBoundary.data as { ports: string[] }).ports).toEqual(['in1']);
    });
  });

  describe('경계 노드 비-노드 불변식 (REQ-SUBFLOW-B04)', () => {
    it('removeNode 는 경계 노드를 삭제하지 않는다', () => {
      useEditorStore
        .getState()
        .loadFlow([], [], [{ id: 'i1', name: 'in1' }], []);

      useEditorStore.getState().removeNode(FLOW_INPUT_BOUNDARY_ID);

      const ids = useEditorStore.getState().nodes.map((n) => n.id);
      expect(ids).toContain(FLOW_INPUT_BOUNDARY_ID);
    });

    it('onNodesChange 의 remove 변경에서 경계 노드를 걸러낸다', () => {
      useEditorStore
        .getState()
        .loadFlow(
          [{ id: 'n1', type: 'custom', position: { x: 0, y: 0 }, data: {} }],
          [],
          [{ id: 'i1', name: 'in1' }],
          [],
        );

      useEditorStore.getState().onNodesChange([
        { type: 'remove', id: FLOW_INPUT_BOUNDARY_ID },
        { type: 'remove', id: 'n1' },
      ]);

      const ids = useEditorStore.getState().nodes.map((n) => n.id);
      expect(ids).toContain(FLOW_INPUT_BOUNDARY_ID);
      expect(ids).not.toContain('n1');
    });
  });

  describe('포트 삭제 시 센티넬 와이어 정리 (REQ-SUBFLOW-A04)', () => {
    it('삭제된 입력 포트를 쓰던 경계 와이어를 함께 제거한다', () => {
      useEditorStore.getState().loadFlow(
        [{ id: 'n1', type: 'custom', position: { x: 0, y: 0 }, data: {} }],
        [
          {
            id: 'e1',
            source: FLOW_INPUT_BOUNDARY_ID,
            sourceHandle: 'in1',
            target: 'n1',
            targetHandle: 'in',
          },
          { id: 'e2', source: 'n1', target: 'n1', sourceHandle: 'out' },
        ],
        [{ id: 'i1', name: 'in1' }],
        [],
      );

      const portId = useEditorStore.getState().flowInputs[0]!.id;
      useEditorStore.getState().removeFlowPort('input', portId);

      const edgeIds = useEditorStore.getState().edges.map((e) => e.id);
      expect(edgeIds).not.toContain('e1'); // 경계 와이어 제거됨
      expect(edgeIds).toContain('e2'); // 일반 와이어 보존
    });

    it('renameFlowPort 는 경계 와이어의 sourceHandle 을 새 이름으로 갱신한다', () => {
      useEditorStore.getState().loadFlow(
        [{ id: 'n1', type: 'custom', position: { x: 0, y: 0 }, data: {} }],
        [
          {
            id: 'e1',
            source: FLOW_INPUT_BOUNDARY_ID,
            sourceHandle: 'in1',
            target: 'n1',
            targetHandle: 'in',
          },
        ],
        [{ id: 'i1', name: 'in1' }],
        [],
      );

      const portId = useEditorStore.getState().flowInputs[0]!.id;
      useEditorStore.getState().renameFlowPort('input', portId, 'sensor');

      const edge = useEditorStore.getState().edges[0]!;
      expect(edge.sourceHandle).toBe('sensor');
    });
  });

  describe('onConnect 센티넬 엣지 정확성 (REQ-SUBFLOW-D02 / 그룹 B)', () => {
    it('경계 노드로 연결하면 센티넬 source + 포트 이름 핸들 엣지를 만든다', () => {
      useEditorStore
        .getState()
        .loadFlow(
          [{ id: 'n1', type: 'custom', position: { x: 0, y: 0 }, data: {} }],
          [],
          [{ id: 'i1', name: 'in1' }],
          [],
        );

      // __flow_input__.in1 → n1.in 연결(사용자가 경계 노드에서 내부 노드로 와이어).
      useEditorStore.getState().onConnect({
        source: FLOW_INPUT_BOUNDARY_ID,
        sourceHandle: 'in1',
        target: 'n1',
        targetHandle: 'in',
      });

      const edge = useEditorStore
        .getState()
        .edges.find((e) => e.source === FLOW_INPUT_BOUNDARY_ID)!;
      expect(edge).toBeDefined();
      expect(edge.sourceHandle).toBe('in1');
      expect(edge.target).toBe('n1');
      expect(edge.targetHandle).toBe('in');
    });

    it('내부 노드 출력 → 출력 경계로 연결하면 센티넬 target + 포트 이름 핸들 엣지를 만든다', () => {
      useEditorStore
        .getState()
        .loadFlow(
          [{ id: 'n1', type: 'custom', position: { x: 0, y: 0 }, data: {} }],
          [],
          [],
          [{ id: 'o1', name: 'out1' }],
        );

      // n1.out → __flow_output__.out1 연결(내부 노드에서 출력 경계로 와이어).
      useEditorStore.getState().onConnect({
        source: 'n1',
        sourceHandle: 'out',
        target: FLOW_OUTPUT_BOUNDARY_ID,
        targetHandle: 'out1',
      });

      const edge = useEditorStore
        .getState()
        .edges.find((e) => e.target === FLOW_OUTPUT_BOUNDARY_ID)!;
      expect(edge).toBeDefined();
      expect(edge.source).toBe('n1');
      expect(edge.sourceHandle).toBe('out');
      expect(edge.targetHandle).toBe('out1');
    });
  });

  describe('직렬화: 저장은 경계 노드 제외 + inputs/outputs 기록 + 센티넬 엣지 보존', () => {
    it('realNodesOnly 로 경계 노드를 거른 결과만 저장 대상이 된다', () => {
      useEditorStore.getState().loadFlow(
        [{ id: 'n1', type: 'custom', position: { x: 0, y: 0 }, data: {} }],
        [],
        [{ id: 'i1', name: 'in1' }],
        [{ id: 'o1', name: 'out1' }],
      );

      const { nodes } = useEditorStore.getState();
      expect(realNodesOnly(nodes).map((n) => n.id)).toEqual(['n1']);
    });
  });

  describe('resetEditor 는 플로우 포트를 비운다', () => {
    it('포트가 있어도 reset 후 비어 있다', () => {
      const s = useEditorStore.getState();
      s.addFlowInput();
      s.addFlowOutput();
      useEditorStore.getState().resetEditor();

      const state = useEditorStore.getState();
      expect(state.flowInputs).toEqual([]);
      expect(state.flowOutputs).toEqual([]);
    });
  });
});

describe('editorStore - 경계/영역 동적 배치 (SPEC-SUBFLOW-001 M4)', () => {
  beforeEach(() => {
    useEditorStore.getState().resetEditor();
  });

  /** 측정 크기를 가진 실제 노드 헬퍼. */
  const sized = (id: string, x: number, y: number): Node => ({
    id,
    type: 'custom',
    position: { x, y },
    width: 100,
    height: 60,
    data: { label: id },
  });

  it('포트가 있으면 실제 노드 바운딩 박스로부터 영역 노드가 생성된다', () => {
    useEditorStore.getState().loadFlow(
      [sized('n1', 0, 0), sized('n2', 300, 0)],
      [],
      [{ id: 'i1', name: 'in1' }],
      [],
    );
    const area = useEditorStore
      .getState()
      .nodes.find((n) => n.id === FLOW_AREA_NODE_ID);
    expect(area).toBeDefined();
  });

  it('포트가 없으면 영역 노드를 만들지 않는다(합성 노드 없음 불변식)', () => {
    useEditorStore.getState().loadFlow([sized('n1', 0, 0)], []);
    const ids = useEditorStore.getState().nodes.map((n) => n.id);
    expect(ids).not.toContain(FLOW_AREA_NODE_ID);
    expect(ids).toEqual(['n1']);
  });

  it('노드를 옮기면 경계 노드 위치가 새 바운딩 박스에서 다시 파생된다', () => {
    useEditorStore.getState().loadFlow(
      [sized('n1', 0, 0)],
      [],
      [{ id: 'i1', name: 'in1' }],
      [{ id: 'o1', name: 'out1' }],
    );

    const before = useEditorStore.getState();
    const inputBefore = before.nodes.find((n) => n.id === FLOW_INPUT_BOUNDARY_ID)!;
    const outputBefore = before.nodes.find(
      (n) => n.id === FLOW_OUTPUT_BOUNDARY_ID,
    )!;

    // n1 을 오른쪽으로 크게 이동 → maxX 증가 → 출력 경계 x 가 커져야 한다.
    useEditorStore.getState().onNodesChange([
      {
        id: 'n1',
        type: 'position',
        position: { x: 500, y: 0 },
        dragging: false,
      } as NodeChange,
    ]);

    const after = useEditorStore.getState();
    const inputAfter = after.nodes.find((n) => n.id === FLOW_INPUT_BOUNDARY_ID)!;
    const outputAfter = after.nodes.find(
      (n) => n.id === FLOW_OUTPUT_BOUNDARY_ID,
    )!;

    expect(inputAfter.position.x).toBeGreaterThan(inputBefore.position.x);
    expect(outputAfter.position.x).toBeGreaterThan(outputBefore.position.x);
  });

  it('영역 노드는 실제 노드 이동 시 함께 갱신된다', () => {
    useEditorStore.getState().loadFlow(
      [sized('n1', 0, 0)],
      [],
      [{ id: 'i1', name: 'in1' }],
      [],
    );
    const areaBefore = useEditorStore
      .getState()
      .nodes.find((n) => n.id === FLOW_AREA_NODE_ID)!;

    useEditorStore.getState().onNodesChange([
      {
        id: 'n1',
        type: 'position',
        position: { x: 400, y: 200 },
        dragging: false,
      } as NodeChange,
    ]);

    const areaAfter = useEditorStore
      .getState()
      .nodes.find((n) => n.id === FLOW_AREA_NODE_ID)!;
    expect(areaAfter.position.x).toBeGreaterThan(areaBefore.position.x);
    expect(areaAfter.position.y).toBeGreaterThan(areaBefore.position.y);
  });

  it('합성 노드는 저장 직렬화에서 제외된다(영역 노드 포함)', () => {
    useEditorStore.getState().loadFlow(
      [sized('n1', 0, 0)],
      [],
      [{ id: 'i1', name: 'in1' }],
      [],
    );
    const { nodes } = useEditorStore.getState();
    // 영역 노드가 렌더 nodes 에는 있지만 realNodesOnly 로 거른 결과에는 없다.
    expect(nodes.some((n) => n.id === FLOW_AREA_NODE_ID)).toBe(true);
    expect(realNodesOnly(nodes).map((n) => n.id)).toEqual(['n1']);
  });

  // REGRESSION TEST: 포트 추가 후 경계/영역 노드가 렌더용 nodes 에 포함되어야 한다
  describe('addFlowInput/addFlowOutput 후 경계 노드 및 영역 노드 렌더링 (Issue: 포트 추가 후 노드 사라짐)', () => {
    it('addFlowInput 후 렌더용 nodes 에 영역(__flow_area__) + 입력 경계(__flow_input__) 노드가 포함되어야 한다', () => {
      // 초기: 실제 노드만 로드 (포트 없음 → 경계 노드 없음)
      useEditorStore.getState().loadFlow([sized('n1', 0, 0)], []);
      expect(useEditorStore.getState().nodes).toHaveLength(1);
      expect(useEditorStore.getState().flowInputs).toHaveLength(0);

      // 입력 포트 추가
      useEditorStore.getState().addFlowInput();

      // 확인: nodes 에 경계 + 영역 노드 포함?
      const { nodes, flowInputs } = useEditorStore.getState();
      expect(flowInputs).toHaveLength(1);
      expect(flowInputs[0]?.name).toBe('in1');

      // 버그: 다음 두 조건 모두 참이어야 함
      const areaNodeExists = nodes.some((n) => n.id === FLOW_AREA_NODE_ID);
      expect(areaNodeExists).toBe(true); // *** 이것이 false 면 버그 1: 영역 노드 미렌더링 ***

      const inputBoundaryExists = nodes.some((n) => n.id === FLOW_INPUT_BOUNDARY_ID);
      expect(inputBoundaryExists).toBe(true); // *** 이것이 false 면 버그 2: 경계 포트 미렌더링 ***

      // 실제 노드는 여전히 present
      const realNodeExists = nodes.some((n) => n.id === 'n1');
      expect(realNodeExists).toBe(true);
    });

    it('addFlowOutput 후 렌더용 nodes 에 영역(__flow_area__) + 출력 경계(__flow_output__) 노드가 포함되어야 한다', () => {
      useEditorStore.getState().loadFlow([sized('n1', 0, 0)], []);
      useEditorStore.getState().addFlowOutput();

      const { nodes, flowOutputs } = useEditorStore.getState();
      expect(flowOutputs).toHaveLength(1);

      const areaNodeExists = nodes.some((n) => n.id === FLOW_AREA_NODE_ID);
      expect(areaNodeExists).toBe(true); // *** 이것이 false 면 버그 1 ***

      const outputBoundaryExists = nodes.some((n) => n.id === FLOW_OUTPUT_BOUNDARY_ID);
      expect(outputBoundaryExists).toBe(true); // *** 이것이 false 면 버그 2 ***
    });
  });
});
