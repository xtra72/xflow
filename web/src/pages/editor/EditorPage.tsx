// React Flow 기반 플로우 에디터 페이지.
// 노드 팔레트, 캔버스, 속성 패널로 구성된 3컬럼 레이아웃을 제공한다.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useBlocker, useLocation, useNavigate, useParams } from 'react-router';
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
import { FlowAreaNode } from '@/components/flow/FlowAreaNode';
import { FlowBoundaryNode } from '@/components/flow/FlowBoundaryNode';
import { FlowPortPanel } from '@/components/flow/FlowPortPanel';
import { NodeContextMenu } from '@/components/flow/NodeContextMenu';
import { NodePalette } from '@/components/palette/NodePalette';
import { ConfirmDialog } from '@/components/property/ConfirmDialog';
import { EdgePropertyPanel } from '@/components/property/EdgePropertyPanel';
import { PropertyPanel } from '@/components/property/PropertyPanel';
import { RuntimeStatsContext } from '@/contexts/RuntimeStatsContext';
import { useFlowStatus } from '@/hooks/useFlow';
import { useRemoteNodeDetail } from '@/hooks/useRemote';
import { useEditorFlowTarget } from '@/hooks/useEditorFlowTarget';
import { useResizable } from '@/hooks/useResizable';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { remoteEditErrorMessage } from '@/lib/remote/editError';
import { resolveRemoteNodeLabel } from '@/lib/remote/nodeLabel';
import { LOCAL_TARGET, type ResourceTarget } from '@/lib/remote/target';
import { TargetProvider } from '@/lib/remote/TargetContext';
import {
  getFlowNodes,
  getFlowTaps,
} from '@/services/api/flowService';
import { useEditorStore } from '@/stores/editorStore';
import { useTapStore } from '@/stores/tapStore';
import { useUIStore } from '@/stores/uiStore';
import type { NodeTypeInfo } from '@/types/node';
import { computePortsForNode, getConfigSchema } from '@/config/nodeSchemas';
import { generateUUID } from '@/lib/utils/uuid';
import {
  FLOW_AREA_NODE_TYPE,
  FLOW_BOUNDARY_NODE_TYPE,
  parseFlowPortsFromConfig,
  serializeFlowDefinition,
} from '@/lib/flow/boundary';
import {
  pushBackStack,
  readBackStack,
  SUBFLOW_BACK_STATE_KEY,
} from '@/lib/flow/subflowNav';
import {
  aggregateFlowNodeStats,
  buildRuntimeStatsMap,
} from '@/lib/flow/subflowNamespace';

/** React Flow에 등록할 커스텀 노드 타입 맵 */
const nodeTypes = {
  custom: CustomNode,
  // SPEC-SUBFLOW-001: 플로우 레벨 경계 포트 합성 노드(좌 입력 / 우 출력).
  [FLOW_BOUNDARY_NODE_TYPE]: FlowBoundaryNode,
  // SPEC-SUBFLOW-001 M4: 실제 노드를 감싸는 영역(바운딩 박스) 배경 표시 노드.
  [FLOW_AREA_NODE_TYPE]: FlowAreaNode,
};

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
  // 로컬 편집: /editor/:flowId. 원격 편집(SPEC-REMOTE-001 M7, REQ-I08):
  //   /admin/remote/nodes/:instanceId/flows/:flowId/edit (기존 수정)
  //   /admin/remote/nodes/:instanceId/flows/new          (신규 생성)
  const { flowId, instanceId } = useParams<{
    flowId: string;
    instanceId: string;
  }>();
  const reactFlowInstance = useReactFlow();
  // 서브플로우 네비게이션(들어가기) 용 라우터 훅.
  // 백 스택은 location.state.subflowBack(string[]) 으로만 운반한다.
  const navigate = useNavigate();
  const location = useLocation();

  // 원격 신규 생성 모드: 라우트가 .../flows/new 이면 flowId 가 'new' 또는 부재.
  const isRemote = !!instanceId;
  const isNewRemoteFlow = isRemote && (flowId === undefined || flowId === 'new');
  // 'new' 플레이스홀더는 실제 자원 id 가 아니므로 하이드레이션/저장에서 제외한다.
  const effectiveFlowId = isNewRemoteFlow ? undefined : flowId;

  // 원격 편집 대상 노드의 사람이 읽을 수 있는 호스트명 해석(REQ: 원격 편집기
  // 시각 구분). 원격 모드일 때만 노드 상세를 조회하며, 조회 실패/지연이 편집을
  // 막지 않도록 폴백(단축 instanceId)을 둔다 — 호스트명은 표시 전용이다.
  const { data: remoteNodeDetail } = useRemoteNodeDetail(
    instanceId ?? '',
    isRemote,
  );
  // 호스트명 우선, 없으면 단축 instanceId(앞 8자 + 생략부호)로 폴백한다.
  // 원시 UUID 전체는 본문에 노출하지 않는다(툴팁/접근성에만 노출).
  const remoteHostname = resolveRemoteNodeLabel(
    remoteNodeDetail?.hostname,
    instanceId,
  );

  // 통합 툴바(EditorToolbar)에 넘길 자원 타깃. 원격이면 노드 instanceId 를 담은
  // 원격 타깃, 아니면 로컬 싱글턴(참조 안정). 라이프사이클/상태/저장 라우팅에 사용.
  const toolbarTarget = useMemo<ResourceTarget>(
    () => (isRemote && instanceId ? { type: 'remote', instanceId } : LOCAL_TARGET),
    [isRemote, instanceId],
  );

  // 플로우 데이터 소스/저장 대상(로컬 PUT vs 원격 PATCH/POST 구분).
  const flowTarget = useEditorFlowTarget({
    flowId: effectiveFlowId,
    instanceId,
    isNew: isNewRemoteFlow,
  });
  const flowData = flowTarget.flowData;
  const isLoading = flowTarget.isLoading;
  const error = flowTarget.error;

  // 플로우 런타임 상태 (5초 간격 폴링) — 로컬 편집에만 해당한다.
  // 원격 편집은 노드 측 상태이며 서버에 로컬 /flows/{id}/status 가 없으므로
  // 빈 id 로 호출해 쿼리를 비활성화한다(원격 편집기에는 런타임 통계 미표시).
  const { data: flowStatus } = useFlowStatus(isRemote ? '' : (flowId ?? ''));
  const isFlowRunning = !isRemote && flowStatus?.status === 'running';

  // 노드 출력 tap(관찰) 상태 복원.
  //
  // tap 은 서버 측에서도 런타임/인메모리 전용이므로, 실행 중인 로컬 플로우를
  // 열 때 현재 서버 tap 목록을 1회 조회해 캔버스의 눈 인디케이터에 반영한다.
  // 실행이 멈추거나 플로우를 떠나면 클라이언트 미러를 초기화한다(핸들러 누수
  // 방지를 위해 출력 버퍼와 관찰 집합을 모두 비운다).
  const setTappedNodeIds = useTapStore((s) => s.setTappedNodeIds);
  const resetTapStore = useTapStore((s) => s.reset);
  useEffect(() => {
    if (isRemote || !flowId || !isFlowRunning) return;
    let cancelled = false;
    getFlowTaps(flowId)
      .then((taps) => {
        if (!cancelled) setTappedNodeIds(taps.node_ids);
      })
      .catch(() => {
        // 플로우 미실행/조회 실패 — 복원 없이 진행한다(빈 상태 유지).
      });
    return () => {
      cancelled = true;
    };
  }, [isRemote, flowId, isFlowRunning, setTappedNodeIds]);

  // 에디터를 떠나거나 플로우를 전환하면 tap 스토어를 초기화한다.
  useEffect(() => {
    return () => {
      resetTapStore();
    };
  }, [flowId, resetTapStore]);

  // 런타임 노드 정보 폴링 (로컬 플로우가 열려 있으면 항상, 3초 간격) — 로컬 전용.
  //
  // isFlowRunning 으로 게이팅하지 않는다. 백엔드가 서브플로우 통계를 기존
  // GET /flows/{id}/nodes 응답에 접어 넣기 때문이다: 플로우 X 가 단독 배포되지
  // 않고 부모 안에서만 서브플로우로 실행되면, getFlowNodes(X) 는 de-namespace 된
  // per-original-node LIVE 통계를 반환한다. 따라서 서브플로우를 단독으로 열어도
  // (그 자신은 isFlowRunning=false) 이 단일 쿼리가 통계를 채운다. 정지/미참조
  // 플로우에 대해서는 백엔드가 빈 배열([])을 싸게 반환하므로, 열린 플로우당
  // 3초마다 요청 1개의 비용으로 메인 플로우와 서브플로우를 균일하게 처리한다.
  const { data: runtimeNodes } = useQuery({
    queryKey: ['flows', flowId, 'nodes'],
    queryFn: () => getFlowNodes(flowId!),
    enabled: !isRemote && !!flowId,
    refetchInterval: 3000,
  });

  // 현재 캔버스에 표시된 flow-node(서브플로우 참조 노드) id 목록 (Fix 1 집계 대상).
  // data.nodeType === 'flow-node' 로 식별한다. 셀렉터에서 id 만 추출해 불필요한
  // 재계산을 줄인다(노드 위치 변경 등에는 반응하지 않도록 정렬 후 join 비교).
  const flowNodeIdsKey = useEditorStore((s) =>
    s.nodes
      .filter((n) => (n.data as Record<string, unknown>)?.nodeType === 'flow-node')
      .map((n) => n.id)
      .sort()
      .join(' '),
  );
  const flowNodeIds = useMemo(
    () => (flowNodeIdsKey === '' ? [] : flowNodeIdsKey.split(' ')),
    [flowNodeIdsKey],
  );

  // 런타임 통계 맵 구성 (nodeId → { inMessages, outMessages, state, ports }).
  //
  // 단계:
  //  1) getFlowNodes 결과로 exact-id 기준 기본 맵을 만든다(일반 노드는 그대로 매핑).
  //     백엔드가 서브플로우 통계를 이 응답에 접어 넣으므로(원본 노드 id 키),
  //     서브플로우 단독 뷰에서도 별도 병합 없이 기본 맵이 이미 채워져 있다.
  //  2) Fix 1 — 표시된 각 flow-node 에 대해 subflow_<id>_* 자식 통계를 합산해
  //     flow-node 자신의 id 로 올린다(부모 뷰에서 0 으로 보이던 문제 해결).
  //     부모 뷰에서는 getFlowNodes(parent) 가 네임스페이스 노드를 반환하므로 유효하다.
  const runtimeStatsMap = useMemo(() => {
    // 1) 기본 맵(exact id). runtimeNodes 가 아직 없으면 빈 맵에서 시작한다.
    const map = buildRuntimeStatsMap(runtimeNodes ?? []);
    // 2) Fix 1 — flow-node 위로 자식 네임스페이스 노드 통계 집계.
    for (const flowNodeId of flowNodeIds) {
      const agg = aggregateFlowNodeStats(flowNodeId, runtimeNodes ?? []);
      if (agg !== undefined) {
        map[flowNodeId] = agg;
      }
    }
    return map;
  }, [runtimeNodes, flowNodeIds]);

  // 에디터 그리드 스냅 설정 (v0.18.4)
  const editorSnapToGrid = useUIStore((s) => s.editorSnapToGrid);
  const editorSnapGridSize = useUIStore((s) => s.editorSnapGridSize);
  // 원격 편집 저장 피드백(토스트) 용.
  const addNotification = useUIStore((s) => s.addNotification);
  const { t } = useTranslation();

  // 에디터 스토어
  const nodes = useEditorStore((s) => s.nodes);
  const edges = useEditorStore((s) => s.edges);
  const flowInputs = useEditorStore((s) => s.flowInputs);
  const flowOutputs = useEditorStore((s) => s.flowOutputs);
  const selectedNodeId = useEditorStore((s) => s.selectedNodeId);
  const selectedEdgeId = useEditorStore((s) => s.selectedEdgeId);
  const isDirty = useEditorStore((s) => s.isDirty);
  const onNodesChange = useEditorStore((s) => s.onNodesChange);
  const onEdgesChange = useEditorStore((s) => s.onEdgesChange);
  const onConnect = useEditorStore((s) => s.onConnect);
  const loadFlow = useEditorStore((s) => s.loadFlow);
  const addNode = useEditorStore((s) => s.addNode);
  const removeNode = useEditorStore((s) => s.removeNode);
  const duplicateNodes = useEditorStore((s) => s.duplicateNodes);
  const selectNode = useEditorStore((s) => s.selectNode);
  const selectEdge = useEditorStore((s) => s.selectEdge);
  const setHighlightedLinkName = useEditorStore((s) => s.setHighlightedLinkName);
  const undo = useEditorStore((s) => s.undo);
  const redo = useEditorStore((s) => s.redo);
  const setDirty = useEditorStore((s) => s.setDirty);
  const setCurrentFlowId = useEditorStore((s) => s.setCurrentFlowId);
  const resetEditor = useEditorStore((s) => s.resetEditor);

  // 노드 우클릭 컨텍스트 메뉴 상태 (위치 + 대상 노드 id).
  const [contextMenu, setContextMenu] = useState<{
    x: number;
    y: number;
    nodeId: string;
  } | null>(null);

  const closeContextMenu = useCallback(() => setContextMenu(null), []);

  // SPEC-SUBFLOW-001: 플로우 포트 관리 패널 표시 토글(뷰 전용 로컬 상태).
  const [showPortPanel, setShowPortPanel] = useState(false);

  // --- 플로우 데이터 로딩 ---
  // hydrationKey 당 1회만 hydrate 한다. 저장 후 invalidateQueries 로 인한 백그라운드
  // 재조회가 에디터 상태를 덮어쓰거나 isDirty 를 되살려 저장 버튼 빨간점이
  // 사라지지 않는 문제를 막는다. 원격 편집은 노드/플로우 조합으로 키를 구성한다.
  const hydrationKey = isRemote
    ? `remote:${instanceId}:${isNewRemoteFlow ? 'new' : (effectiveFlowId ?? '')}`
    : (flowId ?? '');
  const hydratedFlowIdRef = useRef<string | null>(null);

  useEffect(() => {
    if (!flowData) return;
    if (hydratedFlowIdRef.current === hydrationKey) return;

    // config 또는 definition에서 노드/엣지 파싱
    const source =
      (flowData.config as Record<string, unknown>) ?? {};
    const rawNodes = (source.nodes as Node[]) ?? [];
    const rawEdges = (source.edges as Edge[]) ?? [];
    // SPEC-SUBFLOW-001: 정의 최상위 inputs/outputs(플로우 레벨 포트)를 파싱한다.
    // 백엔드는 이를 config.inputs / config.outputs 로 방출한다(REQ-SUBFLOW-A07).
    // loadFlow 가 이 포트들로부터 합성 경계 노드를 만들어 렌더한다.
    const { flowInputs: loadedInputs, flowOutputs: loadedOutputs } =
      parseFlowPortsFromConfig(source);

    // 서버 로딩 전용 액션: nodes/edges/플로우 포트 교체 + isDirty=false + 히스토리 초기화
    loadFlow(rawNodes, rawEdges, loadedInputs, loadedOutputs);
    // 노드 카드 라이브 제어(output ON/OFF 등) 가 현재 플로우를 식별하도록
    // 편집 중인 flowId 를 스토어에 보관한다 (dirty/undo 에 영향 없음).
    // 원격 편집은 라이브 제어 대상이 아니므로 null 로 둔다.
    setCurrentFlowId(isRemote ? null : (flowId ?? null));
    hydratedFlowIdRef.current = hydrationKey;

    // flowId 변경(들어가기/돌아가기/플로우 전환) 시 새 플로우를 전체보기로 맞춘다.
    // <ReactFlow fitView> 는 최초 마운트에만 동작하므로, 재하이드레이션 시에는
    // 이전 플로우의 뷰포트(확대 상태)가 남아 일부만 확대돼 보인다. loadFlow 로
    // 교체된 노드가 측정된 뒤 fitView 하도록 약간의 지연을 둔다.
    const fitTimer = setTimeout(() => {
      reactFlowInstance.fitView({ padding: 0.15, duration: 200 });
    }, 150);
    return () => clearTimeout(fitTimer);
  }, [
    flowId,
    isRemote,
    hydrationKey,
    flowData,
    loadFlow,
    setCurrentFlowId,
    reactFlowInstance,
  ]);

  // hydrationKey 변경(또는 언마운트) 시 에디터를 초기화해 다음 대상이 다시
  // hydrate 되도록 한다.
  useEffect(() => {
    return () => {
      hydratedFlowIdRef.current = null;
      resetEditor();
    };
  }, [hydrationKey, resetEditor]);

  // --- 저장 핸들러 ---
  // 로컬: PUT /flows/{id}. 원격: PATCH(기존)/POST(신규) → 노드 명령 경유(REQ-I08).
  // 신규 원격 플로우는 저장 성공 후 노드 채번 id 로 edit 라우트로 이동한다.
  const { save: saveFlow, isRemote: isRemoteTarget } = flowTarget;
  const handleSave = useCallback(() => {
    // 로컬은 flowId 가 있어야 하고, 원격 신규는 flowId 없이도 저장(생성)한다.
    if (!isDirty) return;
    if (!isRemoteTarget && !flowId) return;

    // SPEC-SUBFLOW-001: 합성 경계 노드를 nodes 에서 제외하고(REQ-SUBFLOW-B04),
    // 센티넬 경계 와이어를 포함한 모든 엣지를 보존하며, 플로우 레벨 포트를
    // 정의 최상위 inputs/outputs 로 기록한다(REQ-SUBFLOW-A07).
    const definition = serializeFlowDefinition(nodes, edges, flowInputs, flowOutputs);
    const flowName = flowData?.name ?? '';

    void saveFlow(definition, flowName)
      .then((res) => {
        setDirty(false);
        // 원격 편집은 명령 전파 결과이므로 성공 토스트로 피드백한다(REQ-I10).
        if (isRemoteTarget) {
          addNotification({ type: 'success', message: t('remote.edit.saveSuccess') });
        }
        // 원격 신규 생성: 노드 채번 id 로 edit 라우트로 교체 이동한다.
        if (isRemoteTarget && isNewRemoteFlow && res.id) {
          navigate(`/admin/remote/nodes/${instanceId}/flows/${res.id}/edit`, {
            replace: true,
          });
        }
      })
      .catch((err: unknown) => {
        // 실패 시 dirty 를 유지해 사용자가 재시도할 수 있게 한다.
        // 원격 편집 실패는 503/504/502/404 의미별 메시지로 토스트한다(REQ-I11).
        if (isRemoteTarget) {
          addNotification({
            type: 'error',
            message: remoteEditErrorMessage(err, t),
          });
        }
      });
  }, [
    isDirty,
    isRemoteTarget,
    isNewRemoteFlow,
    flowId,
    instanceId,
    nodes,
    edges,
    flowInputs,
    flowOutputs,
    flowData,
    saveFlow,
    setDirty,
    navigate,
    addNotification,
    t,
  ]);

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

      // 입력 필드(INPUT/TEXTAREA/contentEditable) 위에서는 복사/붙여넣기를
      // 네이티브 동작에 맡긴다.
      const target = e.target as HTMLElement;
      const inEditableField =
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable;

      // Ctrl/Cmd + C: 선택된 노드를 클립보드에 복사
      if (isMod && e.key === 'c' && !inEditableField) {
        const state = useEditorStore.getState();
        // node.selected === true 인 노드 우선, 없으면 selectedNodeId fallback.
        const selectedIds = state.nodes
          .filter((n) => n.selected)
          .map((n) => n.id);
        const ids =
          selectedIds.length > 0
            ? selectedIds
            : state.selectedNodeId
              ? [state.selectedNodeId]
              : [];
        if (ids.length > 0) {
          e.preventDefault();
          state.copyToClipboard(ids);
        }
        return;
      }

      // Ctrl/Cmd + V: 클립보드 노드를 붙여넣기
      if (isMod && e.key === 'v' && !inEditableField) {
        const state = useEditorStore.getState();
        if (state.clipboard.length > 0) {
          e.preventDefault();
          state.pasteClipboard();
        }
        return;
      }

      // Delete/Backspace: 선택된 노드 또는 엣지 삭제
      if (e.key === 'Delete' || e.key === 'Backspace') {
        // 입력 필드에서는 동작하지 않도록 방지 (위에서 계산한 inEditableField 재사용)
        if (inEditableField) {
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

  // --- 브라우저 이탈 경고 (새로고침/탭 닫기/창 닫기) ---
  // 변경 사항이 있을 때만 브라우저 기본 이탈 확인 대화상자를 띄운다.
  useEffect(() => {
    if (!isDirty) return;

    const handleBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = '';
    };

    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => window.removeEventListener('beforeunload', handleBeforeUnload);
  }, [isDirty]);

  // --- 라우터 이탈 차단 (미저장 변경 시) ---
  // 플로우 전환 선택기 / 사이드바 / 뒤로 가기 등으로 다른 경로로 이동하려 할 때,
  // 변경 사항이 있으면 useBlocker 로 이동을 막고 확인 다이얼로그를 띄운다.
  //
  // - isDirty 가 false 면(저장 직후 또는 변경 없음) 차단하지 않는다.
  // - 같은 경로(같은 flowId 재진입 등)면 pathname 이 변하지 않으므로 차단하지 않는다.
  const blocker = useBlocker(
    ({ currentLocation, nextLocation }) =>
      isDirty && currentLocation.pathname !== nextLocation.pathname,
  );

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
      closeContextMenu();
    },
    [selectNode, closeContextMenu],
  );

  const handleEdgeClick = useCallback(
    (_event: React.MouseEvent, edge: { id: string }) => {
      selectEdge(edge.id);
      closeContextMenu();
    },
    [selectEdge, closeContextMenu],
  );

  const handlePaneClick = useCallback(() => {
    selectNode(null);
    selectEdge(null);
    // SPEC-LINK-001: 빈 캔버스 클릭 시 가상 링크 하이라이트도 해제한다.
    setHighlightedLinkName(null);
    closeContextMenu();
  }, [selectNode, selectEdge, setHighlightedLinkName, closeContextMenu]);

  // --- 노드 우클릭 컨텍스트 메뉴 ---
  const handleNodeContextMenu = useCallback(
    (e: React.MouseEvent, node: Node) => {
      e.preventDefault();
      setContextMenu({ x: e.clientX, y: e.clientY, nodeId: node.id });
    },
    [],
  );

  // 컨텍스트 메뉴의 "복제" 실행.
  // 우클릭한 노드가 현재 다중 선택에 포함되면 선택된 노드 전체를 복제하고,
  // 아니면 해당 노드만 복제한다.
  const handleDuplicateFromMenu = useCallback(() => {
    if (!contextMenu) return;
    const currentNodes = useEditorStore.getState().nodes;
    const target = currentNodes.find((n) => n.id === contextMenu.nodeId);
    const selectedIds = currentNodes.filter((n) => n.selected).map((n) => n.id);

    const ids =
      target?.selected && selectedIds.length > 0
        ? selectedIds
        : [contextMenu.nodeId];

    duplicateNodes(ids);
    closeContextMenu();
  }, [contextMenu, duplicateNodes, closeContextMenu]);

  // 컨텍스트 메뉴의 "삭제" 실행.
  const handleDeleteFromMenu = useCallback(() => {
    if (!contextMenu) return;
    removeNode(contextMenu.nodeId);
    closeContextMenu();
  }, [contextMenu, removeNode, closeContextMenu]);

  // 컨텍스트 메뉴의 "들어가기" 실행 — 참조 플로우를 에디터에서 연다.
  // 현재(부모) 플로우 id 를 백 스택에 쌓아 돌아가기에서 복원할 수 있게 한다.
  // 미저장 변경 시 이동 차단은 useBlocker 가 중앙에서 처리하므로 여기서는
  // navigate 만 호출한다(blocker 가 pathname 변경을 감지해 확인 다이얼로그를 띄움).
  const handleEnterSubflow = useCallback(
    (refFlowId: string) => {
      const currentStack = readBackStack(location.state);
      navigate(`/editor/${refFlowId}`, {
        state: {
          [SUBFLOW_BACK_STATE_KEY]: pushBackStack(currentStack, flowId ?? ''),
        },
      });
      closeContextMenu();
    },
    [location.state, navigate, flowId, closeContextMenu],
  );

  // 우클릭 대상 노드가 참조 플로우가 지정된 flow-node 면 들어갈 참조 플로우 id 를,
  // 아니면 null 을 돌려준다. null 이면 컨텍스트 메뉴에서 "들어가기" 를 숨긴다.
  const contextMenuRefFlowId = useMemo<string | null>(() => {
    if (!contextMenu) return null;
    const target = nodes.find((n) => n.id === contextMenu.nodeId);
    if (!target) return null;
    const data = target.data as Record<string, unknown>;
    if (data.nodeType !== 'flow-node') return null;
    const refFlowId = data.flow_id;
    return typeof refFlowId === 'string' && refFlowId !== '' ? refFlowId : null;
  }, [contextMenu, nodes]);

  // --- MiniMap 노드 색상 ---
  // SPEC-SUBFLOW-001: 영역 표시 노드(__flow_area__)는 실제 노드 전체를 덮는 큰
  // 사각형이라, 미니맵에서 채우면 실제 노드들이 가려진다. 영역은 투명, 경계 포트는
  // 옅은 파랑으로 구분해 실제 노드(회색)가 미니맵에 보이도록 한다.
  const miniMapNodeColor = useCallback((node: Node) => {
    if (node.type === FLOW_AREA_NODE_TYPE) return 'transparent';
    if (node.type === FLOW_BOUNDARY_NODE_TYPE) return '#93c5fd';
    return '#6b7280';
  }, []);

  // 영역 표시 노드는 미니맵에서 테두리도 투명 처리해 완전히 가려지지 않게 한다.
  const miniMapNodeStrokeColor = useCallback((node: Node) => {
    if (node.type === FLOW_AREA_NODE_TYPE) return 'transparent';
    return 'transparent';
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
    // 타깃 인지 공급(SPEC-REMOTE-001): 속성 패널/폼(서브플로우 flow_picker, "포트
    // 갱신")이 prop-drilling 없이 현재 편집 타깃(로컬 | 원격 노드)을 읽도록 에디터
    // 콘텐츠 전체를 감싼다. 로컬 편집은 LOCAL_TARGET 이라 useTargetContext 기본값과
    // 동일 — 기존 로컬 동작은 회귀하지 않는다.
    <TargetProvider target={toolbarTarget}>
    <div className="flex h-full">
      {/* 왼쪽: 노드 팔레트 (240px 고정) */}
      <NodePalette />

      {/* 가운데: 툴바 + 캔버스 */}
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* 상단 툴바 — 로컬/원격 모두 동일한 통합 EditorToolbar 를 사용한다
            (SPEC-REMOTE-001 M8). 원격은 툴바에 target 을 넘겨 라이프사이클(시작/중지/
            배포)을 그룹 D 명령으로 라우팅하고(재시작은 노드 미지원 → 비활성+툴팁),
            노드 식별(호스트명 배지) + "노드로 돌아가기" 링크를 툴바 내부
            (RemoteTitleBlock)에 통합한다. 과거의 별도 RemoteEditorBanner 는 헤더
            ("원격 · {hostname}")·툴바 배지·캔버스 액센트 링과 중복되어 제거했다. */}
        {isRemote ? (
          <div className="flex items-center border-b border-(--color-border-default) bg-gray-50 px-3 py-1.5 dark:bg-gray-900/50">
            <EditorToolbar
              flowId={effectiveFlowId ?? ''}
              target={toolbarTarget}
              nodeLabel={remoteHostname}
              nodeTitle={instanceId ?? ''}
              flowName={flowData?.name ?? ''}
              onSave={handleSave}
              isSaving={flowTarget.isSaving}
              showPortPanel={showPortPanel}
              onTogglePortPanel={() => setShowPortPanel((v) => !v)}
            />
          </div>
        ) : (
          flowId && (
            <div className="flex items-center border-b border-(--color-border-default) bg-gray-50 px-3 py-1.5 dark:bg-gray-900/50">
              <EditorToolbar
                flowId={flowId}
                showPortPanel={showPortPanel}
                onTogglePortPanel={() => setShowPortPanel((v) => !v)}
              />
            </div>
          )
        )}

        {/* React Flow 캔버스.
            원격 모드에서는 캔버스 영역 전체에 옅은 violet 인셋 링을 더해
            "원격 노드를 편집 중"임이 표면 전체에서 읽히게 한다(로컬은 적용 안 함). */}
        <div
          className={cn(
            'relative flex-1',
            isRemote &&
              'ring-1 ring-inset ring-violet-300/60 dark:ring-violet-700/50',
          )}
          data-testid={isRemote ? 'remote-editor-canvas' : undefined}
        >
          {/* SPEC-SUBFLOW-001 M4: 플로우 포트 관리 패널(캔버스 좌상단 오버레이).
              떠 있는 토글 버튼은 제거하고, 토글 트리거는 제어판(EditorToolbar)으로
              이동했다. 패널 자체는 토글이 켜졌을 때만 오버레이로 렌더한다. */}
          {showPortPanel && (
            <div className="absolute left-3 top-3 z-10">
              <FlowPortPanel onClose={() => setShowPortPanel(false)} />
            </div>
          )}

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
            onNodeContextMenu={handleNodeContextMenu}
            onEdgeClick={handleEdgeClick}
            onPaneClick={handlePaneClick}
            onDragOver={handleDragOver}
            onDrop={handleDrop}
            fitView
            deleteKeyCode={null}
            snapToGrid={editorSnapToGrid}
            snapGrid={[editorSnapGridSize, editorSnapGridSize]}
            // 우측 하단 "React Flow" attribution 링크 숨김 (xyflow MIT — 제거 허용).
            proOptions={{ hideAttribution: true }}
            className="bg-gray-50 dark:bg-gray-950"
          >
            <MiniMap
              nodeColor={miniMapNodeColor}
              nodeStrokeColor={miniMapNodeStrokeColor}
              maskColor="rgba(0, 0, 0, 0.1)"
              className="!bg-white dark:!bg-gray-900 !border-gray-200 dark:!border-gray-700"
            />
            <Controls className="!border-(--color-border-default) !bg-(--color-bg-elevated) !shadow-sm" />
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

      {/* 노드 우클릭 컨텍스트 메뉴 */}
      {contextMenu && (
        <NodeContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          // 참조 플로우가 지정된 flow-node 에서만 "들어가기" 를 노출한다.
          onEnter={
            contextMenuRefFlowId
              ? () => handleEnterSubflow(contextMenuRefFlowId)
              : undefined
          }
          onDuplicate={handleDuplicateFromMenu}
          onDelete={handleDeleteFromMenu}
          onClose={closeContextMenu}
        />
      )}

      {/* 미저장 변경 시 이탈 경고 다이얼로그.
          - 확인("이동") → blocker.proceed() 로 이동 진행.
          - 취소/닫기 → blocker.reset() 로 현재 페이지 유지. */}
      <ConfirmDialog
        isOpen={blocker.state === 'blocked'}
        onClose={() => blocker.reset?.()}
        onConfirm={() => blocker.proceed?.()}
        title="저장하지 않은 변경사항"
        message="저장하지 않은 변경사항이 있습니다. 저장하지 않고 이동하시겠습니까?"
        confirmLabel="이동"
        cancelLabel="취소"
        variant="danger"
      />
    </div>
    </TargetProvider>
  );
}
