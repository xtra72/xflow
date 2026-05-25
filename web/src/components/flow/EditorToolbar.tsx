// 플로우 에디터 상단 툴바 컴포넌트.
// 저장, 배포, 실행 제어, 실행 취소/다시 실행 버튼과 플로우 상태 배지를 제공한다.

import {
  Grid3x3,
  Play,
  Redo2,
  RotateCcw,
  Rocket,
  Save,
  Square,
  Undo2,
} from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import {
  useFlowStatus,
  useUpdateFlow,
  useDeployFlow,
  useStartFlow,
  useStopFlow,
  useRestartFlow,
} from '@/hooks/useFlow';
import { useEditorStore } from '@/stores/editorStore';
import { useUIStore } from '@/stores/uiStore';
import type { FlowStatus } from '@/types/flow';

/** 상태별 배지 스타일 매핑 */
const STATUS_BADGE_STYLES: Record<FlowStatus, string> = {
  stored: 'bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400',
  loaded: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300',
  running: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  stopped: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/40 dark:text-yellow-300',
  error: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300',
};

/** 상태별 한국어 라벨 */
const STATUS_LABELS: Record<FlowStatus, string> = {
  stored: '저장됨',
  loaded: '탑재됨',
  running: '실행 중',
  stopped: '중지됨',
  error: '오류',
};

interface EditorToolbarProps {
  /** 현재 편집 중인 플로우 ID */
  flowId: string;
}

/**
 * 에디터 툴바 컴포넌트.
 * 플로우의 저장, 배포, 실행 제어와 실행 취소/다시 실행 기능을 제공한다.
 */
export function EditorToolbar({ flowId }: EditorToolbarProps) {
  const addNotification = useUIStore((s) => s.addNotification);

  // 에디터 상태
  const nodes = useEditorStore((s) => s.nodes);
  const edges = useEditorStore((s) => s.edges);
  const isDirty = useEditorStore((s) => s.isDirty);
  const undoStack = useEditorStore((s) => s.undoStack);
  const redoStack = useEditorStore((s) => s.redoStack);
  const undo = useEditorStore((s) => s.undo);
  const redo = useEditorStore((s) => s.redo);
  const setDirty = useEditorStore((s) => s.setDirty);

  // 에디터 그리드 스냅 (v0.18.4)
  const editorSnapToGrid = useUIStore((s) => s.editorSnapToGrid);
  const toggleEditorSnapToGrid = useUIStore((s) => s.toggleEditorSnapToGrid);

  // 플로우 상태 조회 (5초 간격 폴링)
  const { data: statusInfo } = useFlowStatus(flowId);
  const currentStatus = (statusInfo?.status as FlowStatus) ?? 'stored';

  // 뮤테이션 훅
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
        return '플로우를 찾을 수 없습니다. 서버를 재시작했거나 플로우가 삭제되었을 수 있습니다.';
      }
    }
    return err instanceof Error ? err.message : '알 수 없는 오류';
  };

  // 저장 처리
  const handleSave = () => {
    updateFlow.mutate(
      {
        id: flowId,
        req: {
          definition: { nodes, edges } as Record<string, unknown>,
        },
      },
      {
        onSuccess: () => {
          setDirty(false);
          addNotification({ type: 'success', message: '플로우가 저장되었습니다' });
        },
        onError: (err) =>
          addNotification({ type: 'error', message: `저장 실패: ${errorMsg(err)}` }),
      },
    );
  };

  // 배포 처리
  const handleDeploy = () => {
    deployFlow.mutate(flowId, {
      onSuccess: () =>
        addNotification({ type: 'success', message: '플로우가 배포되었습니다' }),
      onError: (err) =>
        addNotification({ type: 'error', message: `배포 실패: ${errorMsg(err)}` }),
    });
  };

  // 시작 처리
  const handleStart = () => {
    startFlow.mutate(flowId, {
      onSuccess: () =>
        addNotification({ type: 'success', message: '플로우가 시작되었습니다' }),
      onError: (err) =>
        addNotification({ type: 'error', message: `시작 실패: ${errorMsg(err)}` }),
    });
  };

  // 중지 처리
  const handleStop = () => {
    stopFlow.mutate(flowId, {
      onSuccess: () =>
        addNotification({ type: 'success', message: '플로우가 중지되었습니다' }),
      onError: (err) =>
        addNotification({ type: 'error', message: `중지 실패: ${errorMsg(err)}` }),
    });
  };

  // 재시작 처리
  const handleRestart = () => {
    restartFlow.mutate(flowId, {
      onSuccess: () =>
        addNotification({ type: 'success', message: '플로우가 재시작되었습니다' }),
      onError: (err) =>
        addNotification({ type: 'error', message: `재시작 실패: ${errorMsg(err)}` }),
    });
  };

  return (
    <div className="flex items-center gap-1 rounded-lg border border-zinc-200 bg-white px-2 py-1 shadow-sm dark:border-zinc-700 dark:bg-zinc-900">
      {/* 저장 */}
      <ToolbarButton
        icon={Save}
        label="저장"
        onClick={handleSave}
        disabled={!isDirty || isMutating}
        badge={isDirty}
      />

      {/* 배포 */}
      <ToolbarButton
        icon={Rocket}
        label="배포"
        onClick={handleDeploy}
        disabled={isRunning || isMutating}
      />

      <Separator />

      {/* 실행 제어 */}
      <ToolbarButton
        icon={Play}
        label="시작"
        onClick={handleStart}
        disabled={isRunning || isMutating}
      />
      <ToolbarButton
        icon={Square}
        label="중지"
        onClick={handleStop}
        disabled={!isRunning || isMutating}
      />
      <ToolbarButton
        icon={RotateCcw}
        label="재시작"
        onClick={handleRestart}
        disabled={!isRunning || isMutating}
      />

      <Separator />

      {/* 실행 취소 / 다시 실행 */}
      <ToolbarButton
        icon={Undo2}
        label="실행 취소"
        onClick={undo}
        disabled={undoStack.length === 0}
      />
      <ToolbarButton
        icon={Redo2}
        label="다시 실행"
        onClick={redo}
        disabled={redoStack.length === 0}
      />

      <Separator />

      {/* 그리드 스냅 토글 (v0.18.4) */}
      <ToolbarButton
        icon={Grid3x3}
        label={editorSnapToGrid ? '그리드 스냅 끄기' : '그리드 스냅 켜기'}
        onClick={toggleEditorSnapToGrid}
        active={editorSnapToGrid}
      />

      <Separator />

      {/* 플로우 상태 배지 */}
      <span
        className={cn(
          'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium',
          STATUS_BADGE_STYLES[currentStatus],
        )}
      >
        {STATUS_LABELS[currentStatus]}
      </span>
    </div>
  );
}

// ---- 내부 컴포넌트 ----

interface ToolbarButtonProps {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  onClick: () => void;
  disabled?: boolean;
  /** 저장 버튼의 변경 표시 점 */
  badge?: boolean;
  /** 토글 활성화 상태 (v0.18.4) */
  active?: boolean;
}

/** 툴바 버튼 내부 컴포넌트 */
function ToolbarButton({ icon: Icon, label, onClick, disabled, badge, active }: ToolbarButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={label}
      aria-pressed={active}
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
