// 플로우 에디터 상단 툴바 컴포넌트.
// 저장, 배포, 실행 제어, 실행 취소/다시 실행 버튼과 플로우 상태 배지를 제공한다.
//
// 로컬/원격 통합(SPEC-REMOTE-001 M8): `target` 프롭이 없거나 로컬이면 기존
// 동작과 바이트 단위로 동일하다. 원격 타깃이면 동일한 메뉴 형태(저장/배포/시작/
// 중지/재시작 + 상태 배지)를 그대로 노출하되, 라이프사이클은 그룹 D 명령으로
// 라우팅하고(useFlowActionsTarget), 노드 미지원 액션(재시작)은 숨기지 않고
// 비활성+툴팁으로 둔다(메뉴 형태 일치). 상태 배지는 원격 쿼리 프록시
// (useFlowStatusTarget)에서 가져온다. 저장은 EditorPage 가 소유하므로 onSave 로
// 위임한다. 원격 라이프사이클은 노드 승인+온라인일 때만 활성화한다(useTargetGating).

import { useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router';
import {
  ArrowLeft,
  ArrowLeftToLine,
  ArrowRightToLine,
  Check,
  ChevronDown,
  CornerUpLeft,
  Eye,
  EyeOff,
  Focus,
  Grid3x3,
  Layers,
  Minus,
  PanelLeft,
  Plus,
  Play,
  Redo2,
  RotateCcw,
  Rocket,
  Save,
  Server,
  Settings,
  Square,
  Undo2,
} from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import {
  useFlow,
  useFlows,
  useFlowStatus,
  useUpdateFlow,
  useDeployFlow,
  useStartFlow,
  useStopFlow,
  useRestartFlow,
} from '@/hooks/useFlow';
import { useFlowStatusTarget } from '@/hooks/useDetailTargets';
import { useFlowActionsTarget, type FlowAction } from '@/hooks/useResourceActions';
import { useTargetGating } from '@/hooks/useTargetGating';
import { useTranslation } from '@/lib/i18n';
import { remoteEditErrorMessage } from '@/lib/remote/editError';
import { LOCAL_TARGET, isRemoteTarget, type ResourceTarget } from '@/lib/remote/target';
import {
  FOCUS_DEPTH_ALL,
  FOCUS_DEPTH_MAX,
  FOCUS_DEPTH_MIN,
  useEditorStore,
} from '@/stores/editorStore';
import { useUIStore } from '@/stores/uiStore';
import { serializeFlowDefinition } from '@/lib/flow/boundary';
import {
  popBackStack,
  readBackStack,
  SUBFLOW_BACK_STATE_KEY,
} from '@/lib/flow/subflowNav';
import type { FlowStatus } from '@/types/flow';
import { FieldHelp } from '@/components/property/FieldHelp';
import { FlowSettingsDialog } from './FlowSettingsDialog';

/** 상태별 배지 스타일 매핑 */
const STATUS_BADGE_STYLES: Record<FlowStatus, string> = {
  stored: 'bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400',
  loaded: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300',
  running: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  stopped: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/40 dark:text-yellow-300',
  error: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300',
};

/** 상태별 i18n 키 매핑(렌더 시 t() 로 해석 — 컴포넌트 밖에서 t() 호출 금지) */
const STATUS_LABEL_KEYS: Record<FlowStatus, string> = {
  stored: 'status.stored',
  loaded: 'status.loaded',
  running: 'status.running',
  stopped: 'status.stopped',
  error: 'status.error',
};

interface EditorToolbarProps {
  /** 현재 편집 중인 플로우 ID (원격 신규 생성 시 빈 문자열 가능). */
  flowId: string;
  /**
   * SPEC-SUBFLOW-001 M4: 플로우 포트 관리 패널이 열려 있는지 여부(뷰 전용).
   * 패널 자체는 EditorPage 가 오버레이로 렌더하고, 토글 트리거만 제어판(툴바)에 둔다.
   */
  showPortPanel: boolean;
  /** 플로우 포트 패널 열림/닫힘 토글 콜백. */
  onTogglePortPanel: () => void;
  /**
   * 자원 타깃(SPEC-REMOTE-001 M8). 미지정/로컬이면 기존 동작과 동일하다.
   * 원격이면 라이프사이클/상태/저장을 타깃 인지 경로로 라우팅한다.
   */
  target?: ResourceTarget;
  /** 원격 대상 노드 표시명(호스트명 — 배지 본문). 로컬은 미사용. */
  nodeLabel?: string;
  /** 원격 대상 노드 원본 instanceId(배지 툴팁에만 노출). 로컬은 미사용. */
  nodeTitle?: string;
  /** 원격 편집 중인 플로우 이름(원격은 useFlow 를 쓰지 않음). 로컬은 미사용. */
  flowName?: string;
  /**
   * 저장 콜백. 원격 편집은 EditorPage 가 PATCH/POST 명령 전파를 소유하므로
   * 반드시 주입한다. 로컬은 미지정 시 내부 PUT 저장 핸들러를 사용한다(불변).
   */
  onSave?: () => void;
  /** 외부 저장 진행 상태(원격). 로컬은 미사용. */
  isSaving?: boolean;
}

/**
 * 에디터 툴바 컴포넌트.
 * 플로우의 저장, 배포, 실행 제어와 실행 취소/다시 실행 기능을 제공한다.
 */
export function EditorToolbar({
  flowId,
  showPortPanel,
  onTogglePortPanel,
  target = LOCAL_TARGET,
  nodeLabel,
  nodeTitle,
  flowName,
  onSave,
  isSaving,
}: EditorToolbarProps) {
  const { t } = useTranslation();
  const remote = isRemoteTarget(target);
  const addNotification = useUIStore((s) => s.addNotification);
  // 2026-05-31: 플로우 설정 모달 상태(이름·설명 편집 + 표시 토글)
  const [settingsOpen, setSettingsOpen] = useState(false);

  // 에디터 상태
  const nodes = useEditorStore((s) => s.nodes);
  const edges = useEditorStore((s) => s.edges);
  const flowInputs = useEditorStore((s) => s.flowInputs);
  const flowOutputs = useEditorStore((s) => s.flowOutputs);
  const isDirty = useEditorStore((s) => s.isDirty);
  const undoStack = useEditorStore((s) => s.undoStack);
  const redoStack = useEditorStore((s) => s.redoStack);
  const undo = useEditorStore((s) => s.undo);
  const redo = useEditorStore((s) => s.redo);
  const setDirty = useEditorStore((s) => s.setDirty);
  // SPEC-SUBFLOW-001 M4: 패널을 열지 않고도 제어판에서 바로 플로우 포트를 추가한다.
  const addFlowInput = useEditorStore((s) => s.addFlowInput);
  const addFlowOutput = useEditorStore((s) => s.addFlowOutput);

  // 에디터 그리드 스냅 (v0.18.4)
  const editorSnapToGrid = useUIStore((s) => s.editorSnapToGrid);
  const toggleEditorSnapToGrid = useUIStore((s) => s.toggleEditorSnapToGrid);

  // 뷰 전용 표시 토글 (가상 와이어 표시 / 연결 포커스)
  const showVirtualWires = useEditorStore((s) => s.showVirtualWires);
  const toggleShowVirtualWires = useEditorStore((s) => s.toggleShowVirtualWires);
  const focusConnectionsOnSelect = useEditorStore(
    (s) => s.focusConnectionsOnSelect,
  );
  const toggleFocusConnections = useEditorStore((s) => s.toggleFocusConnections);
  const focusDepth = useEditorStore((s) => s.focusDepth);
  const setFocusDepth = useEditorStore((s) => s.setFocusDepth);

  // 플로우 상태 조회 (5초 간격 폴링) — 로컬 전용. 원격은 빈 id 로 비활성화하고
  // 상태/라이프사이클을 RemoteFlowControls 가 타깃 인지 경로로 처리한다.
  const { data: statusInfo } = useFlowStatus(remote ? '' : flowId);
  const currentStatus = (statusInfo?.status as FlowStatus) ?? 'stored';

  // 뮤테이션 훅 (로컬 전용 — 원격은 RemoteFlowControls 로 위임).
  const updateFlow = useUpdateFlow();
  const deployFlow = useDeployFlow();
  const startFlow = useStartFlow();
  const stopFlow = useStopFlow();
  const restartFlow = useRestartFlow();

  const isRunning = currentStatus === 'running';
  const isMutating =
    updateFlow.isPending ||
    deployFlow.isPending ||
    startFlow.isPending ||
    stopFlow.isPending ||
    restartFlow.isPending;

  // 에러 메시지 추출 헬퍼
  const errorMsg = (err: unknown): string => {
    if (err && typeof err === 'object' && 'code' in err) {
      const apiErr = err as { code: string; message: string };
      if (apiErr.code === 'NOT_FOUND') {
        return t('editor.toast.flowNotFound');
      }
    }
    return err instanceof Error ? err.message : t('error.unknownError');
  };

  // 저장 처리
  const handleSave = () => {
    updateFlow.mutate(
      {
        id: flowId,
        req: {
          // SPEC-SUBFLOW-001: 합성 경계 노드 제외 + 플로우 레벨 inputs/outputs 기록
          // + 센티넬 경계 와이어 보존(EditorPage.handleSave 와 동일 직렬화 경로).
          definition: serializeFlowDefinition(
            nodes,
            edges,
            flowInputs,
            flowOutputs,
          ),
        },
      },
      {
        onSuccess: () => {
          setDirty(false);
          addNotification({ type: 'success', message: t('editor.toast.saved') });
        },
        onError: (err) =>
          addNotification({
            type: 'error',
            message: t('editor.toast.saveFailed').replace('{error}', errorMsg(err)),
          }),
      },
    );
  };

  // 배포 처리
  const handleDeploy = () => {
    deployFlow.mutate(flowId, {
      onSuccess: () =>
        addNotification({ type: 'success', message: t('editor.toast.deployed') }),
      onError: (err) =>
        addNotification({
          type: 'error',
          message: t('editor.toast.deployFailed').replace('{error}', errorMsg(err)),
        }),
    });
  };

  // 시작 처리
  const handleStart = () => {
    startFlow.mutate(flowId, {
      onSuccess: () =>
        addNotification({ type: 'success', message: t('editor.toast.started') }),
      onError: (err) =>
        addNotification({
          type: 'error',
          message: t('editor.toast.startFailed').replace('{error}', errorMsg(err)),
        }),
    });
  };

  // 중지 처리
  const handleStop = () => {
    stopFlow.mutate(flowId, {
      onSuccess: () =>
        addNotification({ type: 'success', message: t('editor.toast.stopped') }),
      onError: (err) =>
        addNotification({
          type: 'error',
          message: t('editor.toast.stopFailed').replace('{error}', errorMsg(err)),
        }),
    });
  };

  // 재시작 처리
  const handleRestart = () => {
    restartFlow.mutate(flowId, {
      onSuccess: () =>
        addNotification({ type: 'success', message: t('editor.toast.restarted') }),
      onError: (err) =>
        addNotification({
          type: 'error',
          message: t('editor.toast.restartFailed').replace('{error}', errorMsg(err)),
        }),
    });
  };

  return (
    <div className="flex items-center gap-1 rounded-lg border border-zinc-200 bg-(--color-bg-surface) px-2 py-1 shadow-sm dark:border-zinc-700 dark:bg-zinc-900">
      {/* 서브플로우 "돌아가기" — 들어가기로 진입한 플로우에서만(백 스택 비어있지 않을 때)
          표시되며, 직전(부모) 플로우로 되돌아간다. 중첩(A→B→C) 을 지원한다. */}
      <SubflowBackButton />

      {/* 제목 블록 — 로컬: 플로우 이름(읽기 전용) + 전환 선택기. 원격: 대상 노드
          배지(호스트명) + 플로우 이름(원격은 로컬 useFlow 를 쓰지 않음). */}
      {remote ? (
        <RemoteTitleBlock
          instanceId={isRemoteTarget(target) ? target.instanceId : ''}
          nodeLabel={nodeLabel ?? ''}
          nodeTitle={nodeTitle}
          flowName={flowName ?? ''}
        />
      ) : (
        <div className="flex items-center gap-0.5">
          <FlowTitle flowId={flowId} />
          <FlowSwitcher flowId={flowId} />
        </div>
      )}

      <Separator />

      {/* 저장 + 배포 + 실행 제어(시작/중지/재시작) — 로컬/원격 모두 동일한 메뉴
          형태를 유지한다. 원격은 명령 경로로 라우팅하며 노드 미지원 액션(재시작)은
          숨기지 않고 비활성+툴팁으로 둔다. */}
      {remote ? (
        <RemoteFlowControls
          target={target}
          flowId={flowId}
          isDirty={isDirty}
          isSaving={isSaving ?? false}
          onSave={onSave}
        />
      ) : (
        <>
          {/* 저장 */}
          <ToolbarButton
            icon={Save}
            label={t('editor.save')}
            onClick={onSave ?? handleSave}
            disabled={!isDirty || isMutating}
            badge={isDirty}
          />

          {/* 배포 */}
          <ToolbarButton
            icon={Rocket}
            label={t('editor.deploy')}
            onClick={handleDeploy}
            disabled={isRunning || isMutating}
          />

          <Separator />

          {/* 실행 제어 */}
          <ToolbarButton
            icon={Play}
            label={t('editor.start')}
            onClick={handleStart}
            disabled={isRunning || isMutating}
          />
          <ToolbarButton
            icon={Square}
            label={t('editor.stop')}
            onClick={handleStop}
            disabled={!isRunning || isMutating}
          />
          <ToolbarButton
            icon={RotateCcw}
            label={t('editor.restart')}
            onClick={handleRestart}
            disabled={!isRunning || isMutating}
          />
        </>
      )}

      <Separator />

      {/* 실행 취소 / 다시 실행 */}
      <ToolbarButton
        icon={Undo2}
        label={t('editor.undo')}
        onClick={undo}
        disabled={undoStack.length === 0}
      />
      <ToolbarButton
        icon={Redo2}
        label={t('editor.redo')}
        onClick={redo}
        disabled={redoStack.length === 0}
      />

      <Separator />

      {/* SPEC-SUBFLOW-001 M4: 플로우 포트 빠른 추가 — 패널을 열지 않고도
          제어판에서 입력/출력 포트를 바로 추가한다. 이름 변경·삭제는 플로우 포트
          패널에서 계속 처리한다. */}
      <ToolbarButton
        icon={ArrowRightToLine}
        label={t('editor.toolbar.addInputPort')}
        onClick={addFlowInput}
      />
      <ToolbarButton
        icon={ArrowLeftToLine}
        label={t('editor.toolbar.addOutputPort')}
        onClick={addFlowOutput}
      />

      {/* SPEC-SUBFLOW-001 M4: 플로우 포트 관리 패널 토글 — 기존 좌상단 떠 있는
          버튼을 제거하고 모든 플로우 포트 진입점을 제어판(툴바)으로 통합한다.
          패널 자체는 EditorPage 가 오버레이로 렌더한다(뷰 전용 토글 상태). */}
      <ToolbarButton
        icon={PanelLeft}
        label={t('editor.toolbar.portPanel')}
        onClick={onTogglePortPanel}
        active={showPortPanel}
      />

      <Separator />

      {/* 그리드 스냅 토글 (v0.18.4) */}
      <ToolbarButton
        icon={Grid3x3}
        label={editorSnapToGrid ? t('editor.toolbar.snapOff') : t('editor.toolbar.snapOn')}
        onClick={toggleEditorSnapToGrid}
        active={editorSnapToGrid}
      />

      {/* 가상 와이어 표시 토글 — 켜면 가상 와이어 선을 일반 연결선처럼 그린다. */}
      <ToolbarButton
        icon={showVirtualWires ? Eye : EyeOff}
        label={showVirtualWires ? t('editor.toolbar.virtualWiresHide') : t('editor.toolbar.virtualWiresShow')}
        onClick={toggleShowVirtualWires}
        active={showVirtualWires}
      />

      {/* 연결 포커스 토글 — 켜고 노드를 선택하면 연결만 강조하고 나머지를 흐리게. */}
      <ToolbarButton
        icon={Focus}
        label={t('editor.toolbar.focusConnections')}
        onClick={toggleFocusConnections}
        active={focusConnectionsOnSelect}
      />

      {/* 연결 단계(depth) 컴팩트 스테퍼 — 포커스가 켜졌을 때만 활성화한다.
          선택 노드로부터 몇 hop 까지 강조할지 1~5 또는 전체(무제한) 로 조절한다. */}
      <FocusDepthStepper
        depth={focusDepth}
        onChange={setFocusDepth}
        enabled={focusConnectionsOnSelect}
      />

      {/* 2026-05-31: 플로우 설정 모달(이름·설명 편집 + 표시 토글) — 이름·설명
          편집은 명령 경로 밖이라 원격 편집에서는 노출하지 않는다(로컬 전용). */}
      {!remote && (
        <ToolbarButton
          icon={Settings}
          label={t('editor.toolbar.flowSettings')}
          onClick={() => setSettingsOpen(true)}
        />
      )}

      <Separator />

      {/* 플로우 상태 배지 — 로컬은 로컬 폴링 상태, 원격은 쿼리 프록시 상태를
          소스로 한다(메뉴 형태 유지를 위해 항상 배지를 렌더). */}
      {remote ? (
        <RemoteStatusBadge target={target} flowId={flowId} />
      ) : (
        <span
          className={cn(
            'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium',
            STATUS_BADGE_STYLES[currentStatus],
          )}
        >
          {t(STATUS_LABEL_KEYS[currentStatus])}
        </span>
      )}

      {/* 플로우 설정 모달은 로컬 편집에만 의미가 있다(원격은 이름·표시 토글이
          명령 경로 밖). 원격에서는 설정 버튼도 렌더하지 않으므로 모달도 생략한다. */}
      {!remote && (
        <FlowSettingsDialog
          isOpen={settingsOpen}
          onClose={() => setSettingsOpen(false)}
          flowId={flowId}
        />
      )}
    </div>
  );
}

// ---- 내부 컴포넌트 ----

interface FlowTitleProps {
  /** 현재 편집 중인 플로우 ID */
  flowId: string;
}

/**
 * 플로우 이름(읽기 전용) + 설명 도움말(?) 표시 컴포넌트.
 *
 * - 이름은 더 이상 인라인 편집하지 않는다. 이름·설명 변경은 "플로우 설정"
 *   모달(FlowSettingsDialog)에서만 처리한다.
 * - 설명(description)이 있으면 이름 뒤에 FieldHelp(?) 아이콘을 표시하고,
 *   클릭하면 설명 팝오버를 보여준다. 설명이 없으면 아이콘을 숨긴다.
 */
function FlowTitle({ flowId }: FlowTitleProps) {
  const { t } = useTranslation();
  const { data: flowData } = useFlow(flowId);
  const name = flowData?.name ?? '';
  const description = flowData?.description?.trim() ?? '';

  return (
    <span className="inline-flex items-center gap-1">
      <span
        title={name || t('editor.title.untitled')}
        aria-label={t('editor.title.ariaLabel')}
        className={cn(
          'max-w-[220px] truncate px-2 py-0.5 text-sm font-semibold',
          'text-zinc-800 dark:text-zinc-100',
        )}
      >
        {name || t('editor.title.untitled')}
      </span>
      {description && (
        <FieldHelp text={description} describedById={`flow-desc-${flowId}`} />
      )}
    </span>
  );
}

interface FlowSwitcherProps {
  /** 현재 편집 중인 플로우 ID */
  flowId: string;
}

/**
 * 플로우 전환 선택기.
 *
 * - chevron 버튼을 클릭하면 전체 플로우 목록이 드롭다운으로 열린다.
 * - 현재 플로우는 체크 표시로 구분된다.
 * - 다른 플로우를 선택하면 `/editor/{id}` 로 이동한다. 이동 차단(미저장 변경
 *   경고)은 EditorPage 의 useBlocker 가 중앙에서 처리하므로 여기서는 단순히
 *   navigate 만 호출한다.
 * - 바깥 클릭 / Escape / 선택 시 메뉴를 닫는다.
 */
function FlowSwitcher({ flowId }: FlowSwitcherProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data: flowsResult } = useFlows();
  const flows = flowsResult?.data ?? [];

  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // 바깥 클릭 / Escape 로 메뉴 닫기.
  useEffect(() => {
    if (!open) return;

    const handlePointerDown = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false);
    };

    document.addEventListener('mousedown', handlePointerDown);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('mousedown', handlePointerDown);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [open]);

  // 다른 플로우 선택 시 이동. 같은 플로우면 메뉴만 닫는다.
  const handleSelect = (id: string) => {
    setOpen(false);
    if (id !== flowId) {
      navigate(`/editor/${id}`);
    }
  };

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        title={t('editor.switcher.switchTo')}
        aria-label={t('editor.switcher.switchTo')}
        aria-haspopup="listbox"
        aria-expanded={open}
        className={cn(
          'inline-flex items-center justify-center rounded-md p-1',
          'text-(--color-text-muted) transition-colors',
          'hover:bg-(--color-bg-elevated) hover:text-(--color-text-primary)',
        )}
      >
        <ChevronDown
          className={cn('h-3.5 w-3.5 transition-transform', open && 'rotate-180')}
        />
      </button>

      {open && (
        <div
          role="listbox"
          aria-label={t('editor.switcher.listLabel')}
          className={cn(
            'absolute left-0 top-full z-50 mt-1 max-h-72 w-60 overflow-y-auto',
            'rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) py-1 shadow-lg',
          )}
        >
          {flows.length === 0 ? (
            <div className="px-3 py-2 text-xs text-(--color-text-muted)">
              {t('editor.switcher.empty')}
            </div>
          ) : (
            flows.map((flow) => {
              const isCurrent = flow.id === flowId;
              return (
                <button
                  key={flow.id}
                  type="button"
                  role="option"
                  aria-selected={isCurrent}
                  onClick={() => handleSelect(flow.id)}
                  className={cn(
                    'flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm transition-colors',
                    'hover:bg-(--color-bg-surface)',
                    isCurrent
                      ? 'font-semibold text-(--color-text-primary)'
                      : 'text-(--color-text-secondary)',
                  )}
                >
                  <Check
                    className={cn(
                      'h-3.5 w-3.5 shrink-0',
                      isCurrent ? 'text-blue-500' : 'opacity-0',
                    )}
                  />
                  <span className="truncate">{flow.name || t('editor.title.untitled')}</span>
                </button>
              );
            })
          )}
        </div>
      )}
    </div>
  );
}

/**
 * 서브플로우 "돌아가기" 버튼.
 *
 * - 백 스택(location.state.subflowBack)이 비어 있지 않을 때만(들어가기로 진입한
 *   플로우에서만) 표시된다. 비어 있으면 아무것도 렌더하지 않는다.
 * - 클릭하면 직전(부모) 플로우 id 를 pop 하여 `/editor/{prev}` 로 이동하고,
 *   남은 스택을 새 location.state 로 넘겨 중첩(A→B→C) 복원을 유지한다.
 * - 미저장 변경 시 이동 차단은 EditorPage 의 useBlocker 가 중앙에서 처리하므로
 *   여기서는 navigate 만 호출한다.
 */
function SubflowBackButton() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const backStack = readBackStack(location.state);

  // 들어가기로 진입하지 않았으면(스택 비어있음) 버튼을 숨긴다.
  if (backStack.length === 0) return null;

  const handleBack = () => {
    const { prev, rest } = popBackStack(backStack);
    if (!prev) return;
    navigate(`/editor/${prev}`, {
      state: { [SUBFLOW_BACK_STATE_KEY]: rest },
    });
  };

  return (
    <>
      <ToolbarButton icon={CornerUpLeft} label={t('editor.toolbar.back')} onClick={handleBack} />
      <Separator />
    </>
  );
}

interface FocusDepthStepperProps {
  /** 현재 연결 단계(1~FOCUS_DEPTH_MAX 또는 FOCUS_DEPTH_ALL=전체). */
  depth: number;
  /** 단계 변경 콜백(스토어에서 클램프됨). */
  onChange: (depth: number) => void;
  /** 포커스 토글이 켜져 있어 컨트롤을 활성화할지 여부. */
  enabled: boolean;
}

/**
 * 연결 단계(depth) 컴팩트 스테퍼.
 *
 * - "표시 단계" 레이어 아이콘 + 현재 값 + 증감 버튼으로 구성한다.
 * - 단계 순서는 1, 2, 3, 4, 5, 전체(FOCUS_DEPTH_ALL=Infinity) 이다.
 *   `+` 가 5 에서 한 번 더 눌리면 전체로, `-` 가 전체에서 눌리면 5 로 돌아간다.
 * - 전체일 때는 숫자 대신 "전체" 라벨을 보이고, `+` 버튼을 비활성화한다.
 * - enabled(포커스 ON) 일 때만 상호작용 가능하며, OFF 면 흐리게 비활성화한다.
 * - 하한(1) 에서는 `-`, 상한(전체) 에서는 `+` 가 비활성화된다.
 *   (실제 클램프/센티넬 처리는 스토어 setFocusDepth 가 보장한다.)
 */
function FocusDepthStepper({ depth, onChange, enabled }: FocusDepthStepperProps) {
  const { t } = useTranslation();
  const isAll = depth === FOCUS_DEPTH_ALL;
  const atMin = depth <= FOCUS_DEPTH_MIN;
  const atMax = isAll;

  // `-`: 전체면 유한 상한(5) 으로, 그 외에는 한 단계 줄인다.
  const decrement = () => onChange(isAll ? FOCUS_DEPTH_MAX : depth - 1);
  // `+`: 유한 상한(5) 에서는 전체로, 그 외에는 한 단계 늘린다.
  const increment = () =>
    onChange(depth >= FOCUS_DEPTH_MAX ? FOCUS_DEPTH_ALL : depth + 1);

  const label = isAll ? t('editor.focusDepth.all') : String(depth);
  const title = isAll
    ? t('editor.focusDepth.titleAll')
    : t('editor.focusDepth.title').replace(/\{depth\}/g, String(depth));

  return (
    <div
      title={title}
      aria-label={title}
      className={cn(
        'inline-flex items-center gap-0.5 rounded-md border px-1 py-0.5',
        'border-zinc-200 dark:border-zinc-700',
        enabled
          ? 'text-zinc-600 dark:text-zinc-300'
          : 'pointer-events-none opacity-40',
      )}
    >
      <Layers className="h-3.5 w-3.5 shrink-0 text-zinc-400" />
      <button
        type="button"
        onClick={decrement}
        disabled={!enabled || atMin}
        aria-label={t('editor.focusDepth.decrease')}
        className={cn(
          'inline-flex h-4 w-4 items-center justify-center rounded',
          'hover:bg-zinc-100 dark:hover:bg-zinc-800',
          'disabled:pointer-events-none disabled:opacity-30',
        )}
      >
        <Minus className="h-3 w-3" />
      </button>
      <span
        className={cn(
          'text-center text-xs font-medium tabular-nums',
          // "전체" 라벨이 들어갈 너비를 확보한다.
          isAll ? 'min-w-[1.75rem]' : 'min-w-[0.75rem]',
        )}
        aria-live="polite"
      >
        {label}
      </span>
      <button
        type="button"
        onClick={increment}
        disabled={!enabled || atMax}
        aria-label={t('editor.focusDepth.increase')}
        className={cn(
          'inline-flex h-4 w-4 items-center justify-center rounded',
          'hover:bg-zinc-100 dark:hover:bg-zinc-800',
          'disabled:pointer-events-none disabled:opacity-30',
        )}
      >
        <Plus className="h-3 w-3" />
      </button>
    </div>
  );
}

interface ToolbarButtonProps {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  onClick: () => void;
  disabled?: boolean;
  /** 저장 버튼의 변경 표시 점 */
  badge?: boolean;
  /** 토글 활성화 상태 (v0.18.4) */
  active?: boolean;
  /** 툴팁 오버라이드(미지정 시 label). 원격 미지원/게이트 안내에 사용. */
  title?: string;
  /** 테스트 식별자(선택). */
  testId?: string;
}

/** 툴바 버튼 내부 컴포넌트 */
function ToolbarButton({ icon: Icon, label, onClick, disabled, badge, active, title, testId }: ToolbarButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title ?? label}
      aria-label={label}
      aria-pressed={active}
      data-testid={testId}
      className={cn(
        'relative inline-flex items-center justify-center rounded-md p-1.5',
        'transition-colors duration-100',
        active
          ? 'bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900/40 dark:text-blue-300 dark:hover:bg-blue-900/60'
          : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100',
        'disabled:pointer-events-none disabled:opacity-40',
      )}
    >
      <Icon className="h-4 w-4" />
      {badge && (
        <span className="absolute -right-0.5 -top-0.5 h-1.5 w-1.5 rounded-full bg-orange-500" />
      )}
    </button>
  );
}

/** 수직 구분선 */
function Separator() {
  return <div className="mx-0.5 h-5 w-px bg-zinc-200 dark:bg-zinc-700" />;
}

// ---- 원격 타깃 전용 서브컴포넌트 ----
//
// 원격 전용 훅(useFlowActionsTarget/useTargetGating/useFlowStatusTarget)을 이
// 서브컴포넌트로 스코프해, 로컬 편집기는 원격 훅을 전혀 마운트하지 않게 한다
// (로컬 동작/테스트 불변). 부모 EditorToolbar 는 remote 일 때만 이들을 렌더한다.

interface RemoteTitleBlockProps {
  /** 대상 노드 instanceId(노드로 돌아가기 딥링크 구성에 사용). */
  instanceId: string;
  /** 대상 노드 표시명(호스트명 — 배지 본문). */
  nodeLabel: string;
  /** 원본 instanceId(배지 툴팁에만 노출). */
  nodeTitle?: string;
  /** 편집 중인 플로우 이름. */
  flowName: string;
}

/**
 * 원격 편집기에서 "노드로 돌아가기" 가 향하는 노드 관리 화면 딥링크를 만든다.
 *
 * 단순히 `/admin/remote` 로 가면 노드 관리 페이지가 선택/탭을 잃고 새로 마운트돼
 * 사용자가 노드와 플로우 탭을 다시 골라야 한다. 대신 직전 단계(선택된 노드 +
 * 플로우 탭)를 정확히 복원하도록 `?node={instanceId}&tab=flows` 로 이동한다.
 * 사용자는 플로우를 편집 중이었으므로 flows 탭으로 되돌린다.
 */
function remoteBackHref(instanceId: string): string {
  if (!instanceId) return '/admin/remote';
  return `/admin/remote?node=${encodeURIComponent(instanceId)}&tab=flows`;
}

/**
 * 원격 제목 블록 — 노드로 돌아가기 링크 + 대상 노드 배지(호스트명) + 플로우 이름.
 *
 * RemoteEditorToolbar 의 노드 배지 시각 언어(violet + Server 아이콘)를 계승한다.
 * 원시 instanceId(UUID)는 본문이 아닌 title(툴팁)로만 노출한다.
 *
 * "노드로 돌아가기"(ArrowLeft) 링크는 과거 RemoteEditorBanner 가 제공하던 유일한
 * 유용 요소로, 중복 배너를 제거하면서 이 제어판으로 이관했다. 노드 배지 왼쪽에
 * 컴팩트한 링크 형태로 배치하며, 선택 노드 + 플로우 탭을 복원하는 노드 관리
 * 딥링크(remoteBackHref)로 이동한다(직전 단계로 정확히 복귀).
 * 로컬 편집기에는 RemoteTitleBlock 자체가 렌더되지 않으므로 영향이 없다.
 */
function RemoteTitleBlock({ instanceId, nodeLabel, nodeTitle, flowName }: RemoteTitleBlockProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  return (
    <div className="flex min-w-0 items-center gap-1">
      <button
        type="button"
        onClick={() => navigate(remoteBackHref(instanceId))}
        title={t('remote.editor.backToNode')}
        aria-label={t('remote.editor.backToNode')}
        data-testid="remote-editor-back"
        className={cn(
          'inline-flex shrink-0 items-center justify-center rounded-md p-1.5',
          'text-violet-600 transition-colors duration-100',
          'hover:bg-violet-100 hover:text-violet-800',
          'dark:text-violet-300 dark:hover:bg-violet-900/40 dark:hover:text-violet-100',
        )}
      >
        <ArrowLeft className="h-4 w-4" aria-hidden="true" />
      </button>
      <span
        className="inline-flex shrink-0 items-center gap-1 rounded-md bg-violet-100 px-2 py-0.5 text-xs font-medium text-violet-700 dark:bg-violet-900/40 dark:text-violet-300"
        title={nodeTitle ? `${t('remote.editor.targetNode')} (${nodeTitle})` : t('remote.editor.targetNode')}
        data-testid="remote-editor-node-badge"
      >
        <Server className="h-3 w-3" aria-hidden="true" />
        {nodeLabel}
      </span>
      <span
        className="max-w-[200px] truncate px-1 text-sm font-semibold text-zinc-800 dark:text-zinc-100"
        title={flowName || t('remote.editor.untitled')}
      >
        {flowName || t('remote.editor.untitled')}
      </span>
    </div>
  );
}

interface RemoteFlowControlsProps {
  /** 원격 타깃(노드 instanceId 포함). */
  target: ResourceTarget;
  /** 편집 중인 플로우 ID(원격 신규는 빈 문자열). */
  flowId: string;
  /** 미저장 변경 여부(저장 버튼 활성). */
  isDirty: boolean;
  /** 저장 진행 여부(저장 버튼 비활성). */
  isSaving: boolean;
  /** 저장 콜백(EditorPage 의 PATCH/POST 명령 전파 위임). */
  onSave?: () => void;
}

/**
 * 원격 저장 + 배포 + 실행 제어(시작/중지/재시작).
 *
 * 로컬 EditorToolbar 의 메뉴 형태를 그대로 유지하되 라이프사이클을 그룹 D 명령
 * (useFlowActionsTarget)으로 라우팅한다. 노드 미지원 액션(재시작)은 숨기지 않고
 * 비활성+미지원 툴팁으로 둔다. 노드가 승인+온라인이 아니면 게이트 안내와 함께
 * 비활성화한다(useTargetGating). 신규(빈 flowId)는 라이프사이클을 모두 비활성화한다.
 */
function RemoteFlowControls({
  target,
  flowId,
  isDirty,
  isSaving,
  onSave,
}: RemoteFlowControlsProps) {
  const { t } = useTranslation();
  const actions = useFlowActionsTarget(target);
  const gating = useTargetGating(target);
  const status = useFlowStatusTarget(target, flowId);
  const addNotification = useUIStore((s) => s.addNotification);

  const currentStatus = (status.data?.status as FlowStatus | undefined) ?? 'stored';
  const nodeControllable = gating.canControl();
  // 신규(아직 채번 전) 플로우는 라이프사이클 대상 id 가 없으므로 모두 비활성화한다.
  const hasId = flowId !== '';

  const canStart =
    hasId && (currentStatus === 'stored' || currentStatus === 'loaded' || currentStatus === 'stopped');
  const canStop = hasId && currentStatus === 'running';
  const canDeploy = hasId && currentStatus === 'stored';

  // 액션 버튼의 disabled/툴팁을 계산한다(FlowActionMenu 와 동일 정책).
  //  - 미지원 액션(재시작): 항상 비활성 + 미지원 안내.
  //  - 노드 미제어(오프라인/미승인): 비활성 + 게이트 안내.
  const stateFor = (action: FlowAction, baseTitle: string): { disabled: boolean; title: string } => {
    if (!actions.supports(action)) {
      return { disabled: true, title: t('remote.edit.unsupportedOnRemote') };
    }
    if (!nodeControllable) {
      return { disabled: true, title: t('remote.edit.actionGateHint') };
    }
    return { disabled: false, title: baseTitle };
  };

  const run = async (action: FlowAction) => {
    try {
      await actions.perform(action, flowId);
    } catch (err) {
      addNotification({ type: 'error', message: remoteEditErrorMessage(err, t) });
    }
  };

  const deploy = stateFor('deploy', t('editor.deploy'));
  const start = stateFor('start', t('editor.start'));
  const stop = stateFor('stop', t('editor.stop'));
  const restart = stateFor('restart', t('editor.restart'));

  return (
    <>
      {/* 저장 — 원격은 PATCH(기존)/POST(신규) 명령 전파(EditorPage 소유). */}
      <ToolbarButton
        icon={Save}
        label={t('editor.save')}
        onClick={() => onSave?.()}
        disabled={!isDirty || isSaving}
        badge={isDirty}
        testId="remote-editor-save"
      />

      {/* 배포 */}
      <ToolbarButton
        icon={Rocket}
        label={t('editor.deploy')}
        title={deploy.title}
        onClick={() => void run('deploy')}
        disabled={!canDeploy || deploy.disabled || (actions.pending.deploy ?? false)}
      />

      <Separator />

      {/* 실행 제어 */}
      <ToolbarButton
        icon={Play}
        label={t('editor.start')}
        title={start.title}
        onClick={() => void run('start')}
        disabled={!canStart || start.disabled || (actions.pending.start ?? false)}
      />
      <ToolbarButton
        icon={Square}
        label={t('editor.stop')}
        title={stop.title}
        onClick={() => void run('stop')}
        disabled={!canStop || stop.disabled || (actions.pending.stop ?? false)}
      />
      {/* 재시작 — 원격 노드 미지원: 숨기지 않고 비활성+툴팁으로 메뉴 형태 일치. */}
      <ToolbarButton
        icon={RotateCcw}
        label={t('editor.restart')}
        title={restart.title}
        onClick={() => void run('restart')}
        disabled
      />
    </>
  );
}

interface RemoteStatusBadgeProps {
  /** 원격 타깃. */
  target: ResourceTarget;
  /** 플로우 ID(빈 문자열이면 상태 미조회 → 중립 표시). */
  flowId: string;
}

/**
 * 원격 플로우 상태 배지 — 쿼리 프록시(useFlowStatusTarget)에서 상태를 가져온다.
 *
 * 로컬 /flows/{id}/status 를 호출하지 않는다(원격은 404). 상태 미확정/미조회 시
 * 'stored'(저장됨) 로 중립 표시해 배지 레이아웃을 항상 유지한다.
 */
function RemoteStatusBadge({ target, flowId }: RemoteStatusBadgeProps) {
  const { t } = useTranslation();
  const status = useFlowStatusTarget(target, flowId);
  const currentStatus = (status.data?.status as FlowStatus | undefined) ?? 'stored';
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium',
        STATUS_BADGE_STYLES[currentStatus],
      )}
      data-testid="remote-editor-status-badge"
    >
      {t(STATUS_LABEL_KEYS[currentStatus])}
    </span>
  );
}
