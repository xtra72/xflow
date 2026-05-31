// React Flow 기반 플로우 에디터 페이지.
// 노드 팔레트, 캔버스, 속성 패널로 구성된 3컬럼 레이아웃을 제공한다.

import { useCallback, useEffect, useMemo, useRef } from 'react';
import { useParams } from 'react-router';
import {
  ReactFlow,
  MiniMap,
  Controls,
  Background,
  BackgroundVariant,
  ReactFlowProvider,
  useReactFlow,
  type Node,
  type Edge,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { useQuery } from '@tanstack/react-query';
import { AlertTriangle, Loader2 } from 'lucide-react';

import { CustomNode } from '@/components/flow/CustomNode';
import { CustomEdge } from '@/components/flow/CustomEdge';
import { DebugPanel } from '@/components/flow/DebugPanel';
import { EditorToolbar } from '@/components/flow/EditorToolbar';
import { NodePalette } from '@/components/palette/NodePalette';
import { EdgePropertyPanel } from '@/components/property/EdgePropertyPanel';
import { PropertyPanel } from '@/components/property/PropertyPanel';
import {
  RuntimeStatsContext,
  type NodeRuntimeStats,
} from '@/contexts/RuntimeStatsContext';
import { useFlow, useFlowStatus, useUpdateFlow } from '@/hooks/useFlow';
import { useResizable } from '@/hooks/useResizable';
import { getFlowNodes } from '@/services/api/flowService';
import { useEditorStore } from '@/stores/editorStore';
import { useUIStore } from '@/stores/uiStore';
import type { NodeTypeInfo } from '@/types/node';
import { computePortsForNode, getConfigSchema } from '@/config/nodeSchemas';
import { generateUUID } from '@/lib/utils/uuid';

/** React Flow에 등록할 커스텀 노드 타입 맵 */
const nodeTypes = { custom: CustomNode };

/** React Flow에 등록할 커스텀 엣지 타입 맵 */
const edgeTypes = { custom: CustomEdge };

/**
 * 에디터 페이지 외부 래퍼.
 * ReactFlowProvider로 감싸야 내부 컴포넌트에서 useReactFlow를 사용할 수 있다.
 */
export default function EditorPage() {
  return (
    <ReactFlowProvider>
      <EditorPageInner />
    </ReactFlowProvider>
  );
}

/**
 * 에디터 페이지 내부 컴포넌트.
 * React Flow 캔버스, 드래그 앤 드롭, 키보드 단축키, 플로우 로딩을 처리한다.
 */
function EditorPageInner() {
  const { flowId } = useParams<{ flowId: string }>();
  const reactFlowInstance = useReactFlow();

  // 플로우 데이터 조회
  const { data: flowData, isLoading, error } = useFlow(flowId ?? '');
  const updateFlow = useUpdateFlow();

  // 플로우 런타임 상태 (5초 간격 폴링)
  const { data: flowStatus } = useFlowStatus(flowId ?? '');
  const isFlowRunning = flowStatus?.status === 'running';

  // 런타임 노드 정보 폴링 (플로우 실행 중일 때만, 3초 간격)
  const { data: runtimeNodes } = useQuery({
    queryKey: ['flows', flowId, 'nodes'],
    queryFn: () => getFlowNodes(flowId!),
    enabled: !!flowId && isFlowRunning,
    refetchInterval: 3000,
  });

  // 런타임 통계 맵 구성 (nodeId → { inMessages, outMessages, state })
  const runtimeStatsMap = useMemo(() => {
    if (!runtimeNodes || !isFlowRunning) return {};
    const map: Record<string, NodeRuntimeStats> = {};
    for (const node of runtimeNodes) {
      const inMessages = (node.ports ?? [])
        .filter((p) => p.direction === 'input')
        .reduce((sum, p) => sum + p.messages, 0);
      const outMessages = (node.ports ?? [])
        .filter((p) => p.direction === 'output')
        .reduce((sum, p) => sum + p.messages, 0);
      map[node.node_id] = { inMessages, outMessages, state: node.state };
    }
    return map;
  }, [runtimeNodes, isFlowRunning]);

  // 에디터 그리드 스냅 설정 (v0.18.4)
  const editorSnapToGrid = useUIStore((s) => s.editorSnapToGrid);
  const editorSnapGridSize = useUIStore((s) => s.editorSnapGridSize);

  // 에디터 스토어
  const nodes = useEditorStore((s) => s.nodes);
  const edges = useEditorStore((s) => s.edges);
  const selectedNodeId = useEditorStore((s) => s.selectedNodeId);
  const selectedEdgeId = useEditorStore((s) => s.selectedEdgeId);
  const isDirty = useEditorStore((s) => s.isDirty);
  const onNodesChange = useEditorStore((s) => s.onNodesChange);
  const onEdgesChange = useEditorStore((s) => s.onEdgesChange);
  const onConnect = useEditorStore((s) => s.onConnect);
  const loadFlow = useEditorStore((s) => s.loadFlow);
  const addNode = useEditorStore((s) => s.addNode);
  const removeNode = useEditorStore((s) => s.removeNode);
  const selectNode = useEditorStore((s) => s.selectNode);
  const selectEdge = useEditorStore((s) => s.selectEdge);
  const undo = useEditorStore((s) => s.undo);
  const redo = useEditorStore((s) => s.redo);
  const setDirty = useEditorStore((s) => s.setDirty);
  const resetEditor = useEditorStore((s) => s.resetEditor);

  // --- 플로우 데이터 로딩 ---
  // flowId 당 1회만 hydrate 한다. 저장 후 invalidateQueries 로 인한 백그라운드
  // 재조회가 에디터 상태를 덮어쓰거나 isDirty 를 되살려 저장 버튼 빨간점이
  // 사라지지 않는 문제를 막는다.
  const hydratedFlowIdRef = useRef<string | null>(null);

  useEffect(() => {
    if (!flowData) return;
    if (hydratedFlowIdRef.current === flowId) return;

    // config 또는 definition에서 노드/엣지 파싱
    const source =
      (flowData.config as Record<string, unknown>) ?? {};
    const rawNodes = (source.nodes as Node[]) ?? [];
    const rawEdges = (source.edges as Edge[]) ?? [];

    // 서버 로딩 전용 액션: nodes/edges 교체 + isDirty=false + 히스토리 초기화
    loadFlow(rawNodes, rawEdges);
    hydratedFlowIdRef.current = flowId ?? null;
  }, [flowId, flowData, loadFlow]);

  // flowId 변경(또는 언마운트) 시 에디터를 초기화해 다음 flowId 가 다시
  // hydrate 되도록 한다.
  useEffect(() => {
    return () => {
      hydratedFlowIdRef.current = null;
      resetEditor();
    };
  }, [flowId, resetEditor]);

  // --- 저장 핸들러 ---
  const handleSave = useCallback(() => {
    if (!flowId || !isDirty) return;

    updateFlow.mutate(
      {
        id: flowId,
        req: {
          definition: { nodes, edges } as Record<string, unknown>,
        },
      },
      { onSuccess: () => setDirty(false) },
    );
  }, [flowId, isDirty, nodes, edges, updateFlow, setDirty]);

  // --- 키보드 단축키 ---
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      const isMod = e.metaKey || e.ctrlKey;

      // Ctrl/Cmd + S: 저장
      if (isMod && e.key === 's') {
        e.preventDefault();
        handleSave();
        return;
      }

      // Ctrl/Cmd + Shift + Z: 다시 실행
      if (isMod && e.shiftKey && e.key === 'z') {
        e.preventDefault();
        redo();
        return;
      }

      // Ctrl/Cmd + Z: 실행 취소
      if (isMod && e.key === 'z') {
        e.preventDefault();
        undo();
        return;
      }

      // Delete/Backspace: 선택된 노드 또는 엣지 삭제
      if (e.key === 'Delete' || e.key === 'Backspace') {
        // 입력 필드에서는 동작하지 않도록 방지
        const target = e.target as HTMLElement;
        if (
          target.tagName === 'INPUT' ||
          target.tagName === 'TEXTAREA' ||
          target.isContentEditable
        ) {
          return;
        }

        const currentSelectedNodeId = useEditorStore.getState().selectedNodeId;
        const currentSelectedEdgeId = useEditorStore.getState().selectedEdgeId;

        if (currentSelectedNodeId) {
          e.preventDefault();
          removeNode(currentSelectedNodeId);
        } else if (currentSelectedEdgeId) {
          e.preventDefault();
          reactFlowInstance.deleteElements({
            edges: [{ id: currentSelectedEdgeId }],
          });
          selectEdge(null);
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleSave, undo, redo, removeNode, selectEdge, reactFlowInstance]);

  // --- 브라우저 이탈 경고 (변경 사항이 있을 때) ---
  useEffect(() => {
    if (!isDirty) return;

    function handleBeforeUnload(e: BeforeUnloadEvent) {
      e.preventDefault();
    }

    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => window.removeEventListener('beforeunload', handleBeforeUnload);
  }, [isDirty]);

  // --- 드래그 앤 드롭 핸들러 ---
  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
  }, []);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();

      const raw = e.dataTransfer.getData('application/xflow-node');
      if (!raw) return;

      let nodeType: NodeTypeInfo;
      try {
        nodeType = JSON.parse(raw) as NodeTypeInfo;
      } catch {
        return;
      }

      // 화면 좌표를 플로우 캔버스 좌표로 변환
      const position = reactFlowInstance.screenToFlowPosition({
        x: e.clientX,
        y: e.clientY,
      });

      const newNode: Node = {
        // v0.18.12: 노드 id 를 UUID v4 로 생성 (이전: `${type}-${Date.now()}`).
        // generateUUID 는 secure context 외부 (HTTP 환경) 에서도 안전한 fallback 보유.
        id: generateUUID(),
        type: 'custom',
        position,
        data: {
          label: nodeType.type,
          nodeType: nodeType.type,
          category: nodeType.category,
          ports: computePortsForNode(nodeType.type),
          config_schema: nodeType.type === 'bridge' ? undefined : getConfigSchema(nodeType.type),
          status: 'draft',
        },
      };

      addNode(newNode);
    },
    [reactFlowInstance, addNode],
  );

  // --- 노드/엣지 클릭 핸들러 ---
  const handleNodeClick = useCallback(
    (_event: React.MouseEvent, node: Node) => {
      selectNode(node.id);
    },
    [selectNode],
  );

  const handleEdgeClick = useCallback(
    (_event: React.MouseEvent, edge: { id: string }) => {
      selectEdge(edge.id);
    },
    [selectEdge],
  );

  const handlePaneClick = useCallback(() => {
    selectNode(null);
    selectEdge(null);
  }, [selectNode, selectEdge]);

  // --- MiniMap 노드 색상 ---
  const miniMapNodeColor = useCallback(() => {
    return '#6b7280';
  }, []);

  // --- 선택된 노드 또는 엣지가 있으면 속성 패널 표시 ---
  const showPropertyPanel = selectedNodeId !== null;
  const showEdgePanel = selectedEdgeId !== null && selectedNodeId === null;

  // --- 속성 패널 리사이즈 ---
  const { width: panelWidth, isDragging, handleMouseDown: onResizeStart } = useResizable({
    storageKey: 'xflow-property-panel-width',
    defaultWidth: 300,
    minWidth: 240,
    maxWidth: 600,
    side: 'left',
  });

  // --- 기본 엣지 옵션 (모든 새 엣지에 적용) ---
  const defaultEdgeOptions = useMemo(
    () => ({
      type: 'custom',
    }),
    [],
  );

  // --- 로딩 상태 ---
  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="flex flex-col items-center gap-3">
          <Loader2 className="h-8 w-8 animate-spin text-blue-500" />
          <p className="text-sm text-(--color-text-muted)">
            플로우를 불러오는 중...
          </p>
        </div>
      </div>
    );
  }

  // --- 오류 상태 ---
  if (error) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="flex flex-col items-center gap-3 text-center">
          <AlertTriangle className="h-8 w-8 text-red-500" />
          <p className="text-sm font-medium text-(--color-text-primary)">
            플로우를 불러올 수 없습니다
          </p>
          <p className="max-w-md text-xs text-(--color-text-muted)">
            {error instanceof Error ? error.message : '알 수 없는 오류가 발생했습니다'}
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full">
      {/* 왼쪽: 노드 팔레트 (240px 고정) */}
      <NodePalette />

      {/* 가운데: 툴바 + 캔버스 */}
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* 상단 툴바 */}
        {flowId && (
          <div className="flex items-center border-b border-(--color-border-default) bg-gray-50 px-3 py-1.5 dark:bg-gray-900/50">
            <EditorToolbar flowId={flowId} />
          </div>
        )}

        {/* React Flow 캔버스 */}
        <div className="flex-1">
          <RuntimeStatsContext.Provider value={runtimeStatsMap}>
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            edgeTypes={edgeTypes}
            defaultEdgeOptions={defaultEdgeOptions}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            onNodeClick={handleNodeClick}
            onEdgeClick={handleEdgeClick}
            onPaneClick={handlePaneClick}
            onDragOver={handleDragOver}
            onDrop={handleDrop}
            fitView
            deleteKeyCode={null}
            snapToGrid={editorSnapToGrid}
            snapGrid={[editorSnapGridSize, editorSnapGridSize]}
            className="bg-gray-50 dark:bg-gray-950"
          >
            <MiniMap
              nodeColor={miniMapNodeColor}
              maskColor="rgba(0, 0, 0, 0.1)"
              className="!bg-white dark:!bg-gray-900 !border-gray-200 dark:!border-gray-700"
            />
            <Controls className="!border-gray-200 !bg-white !shadow-sm dark:!border-gray-700 dark:!bg-gray-900" />
            <Background
              variant={BackgroundVariant.Dots}
              gap={editorSnapGridSize}
              size={1}
              color="#d1d5db"
            />
          </ReactFlow>
          </RuntimeStatsContext.Provider>
        </div>

        {/* 하단: 디버그 출력 패널 */}
        <DebugPanel />
      </div>

      {/* 오른쪽: 속성 패널 (리사이즈 가능, 노드 또는 엣지 선택 시 표시) */}
      {(showPropertyPanel || showEdgePanel) && (
        <>
          {/* 리사이즈 핸들 */}
          <div
            onMouseDown={onResizeStart}
            className={`w-1 shrink-0 cursor-col-resize transition-colors hover:bg-blue-400
              ${isDragging ? 'bg-blue-500' : 'bg-transparent'}`}
          />
          {showPropertyPanel
            ? <PropertyPanel width={panelWidth} />
            : <EdgePropertyPanel width={panelWidth} />
          }
        </>
      )}

      {/* 드래그 중 iframe/캔버스 위에서도 이벤트 캡처 */}
      {isDragging && <div className="fixed inset-0 z-50 cursor-col-resize" />}
    </div>
  );
}
