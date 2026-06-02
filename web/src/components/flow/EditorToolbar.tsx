// 플로우 에디터 상단 툴바 컴포넌트.
// 저장, 배포, 실행 제어, 실행 취소/다시 실행 버튼과 플로우 상태 배지를 제공한다.

import { useEffect, useRef, useState } from 'react';
import {
  Grid3x3,
  Pencil,
  Play,
  Redo2,
  RotateCcw,
  Rocket,
  Save,
  Settings,
  Square,
  Undo2,
} from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import {
  useFlow,
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
import { FlowSettingsDialog } from './FlowSettingsDialog';

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
  // 2026-05-31: 플로우 표시 설정 모달 상태
  const [settingsOpen, setSettingsOpen] = useState(false);

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
      {/* 플로우 이름 (클릭하여 인라인 편집) */}
      <FlowNameEditor flowId={flowId} />

      <Separator />

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

      {/* 2026-05-31: 플로우 표시 설정 모달 */}
      <ToolbarButton
        icon={Settings}
        label="플로우 표시 설정"
        onClick={() => setSettingsOpen(true)}
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

      <FlowSettingsDialog
        isOpen={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        flowId={flowId}
      />
    </div>
  );
}

// ---- 내부 컴포넌트 ----

interface FlowNameEditorProps {
  /** 현재 편집 중인 플로우 ID */
  flowId: string;
}

/**
 * 플로우 이름 인라인 편집 컴포넌트.
 *
 * - 기본은 텍스트로 이름을 표시하고, 클릭하면 input 으로 전환된다.
 * - Enter / blur 시 trim 후 저장(useUpdateFlow). 빈 값은 무시하고 원래 이름 복원.
 * - Escape 는 저장 없이 취소.
 * - 편집 중에는 keydown 전파를 막아 에디터 단축키(Delete/Ctrl+Z 등) 가
 *   실행되지 않게 한다.
 */
function FlowNameEditor({ flowId }: FlowNameEditorProps) {
  const { data: flowData } = useFlow(flowId);
  const updateFlow = useUpdateFlow();
  const name = flowData?.name ?? '';

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);
  // blur 와 Enter / Escape 가 중복 저장되지 않도록 커밋 여부를 추적한다.
  const committedRef = useRef(false);

  // 편집 진입 시 input 에 포커스하고 전체 선택한다.
  useEffect(() => {
    if (editing) {
      inputRef.current?.focus();
      inputRef.current?.select();
    }
  }, [editing]);

  const startEditing = () => {
    setDraft(name);
    committedRef.current = false;
    setEditing(true);
  };

  const commit = () => {
    if (committedRef.current) return;
    committedRef.current = true;

    const trimmed = draft.trim();
    // 빈 값이거나 변경이 없으면 저장하지 않고 원래 이름으로 되돌린다.
    if (trimmed && trimmed !== name) {
      updateFlow.mutate({ id: flowId, req: { name: trimmed } });
    }
    setEditing(false);
  };

  const cancel = () => {
    committedRef.current = true;
    setEditing(false);
  };

  if (editing) {
    return (
      <input
        ref={inputRef}
        type="text"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        // 에디터 전역 단축키가 발동하지 않도록 keydown 전파를 차단한다.
        onKeyDown={(e) => {
          e.stopPropagation();
          if (e.key === 'Enter') {
            e.preventDefault();
            commit();
          } else if (e.key === 'Escape') {
            e.preventDefault();
            cancel();
          }
        }}
        aria-label="플로우 이름"
        className={cn(
          'max-w-[220px] rounded-md border border-blue-400 bg-white px-2 py-0.5',
          'text-sm font-semibold text-zinc-900 outline-none',
          'focus:ring-1 focus:ring-blue-400',
          'dark:bg-zinc-800 dark:text-zinc-100',
        )}
      />
    );
  }

  return (
    <button
      type="button"
      onClick={startEditing}
      title="플로우 이름 (클릭하여 변경)"
      aria-label="플로우 이름 (클릭하여 변경)"
      className={cn(
        'group inline-flex items-center gap-1 rounded-md px-2 py-0.5',
        'max-w-[220px] text-sm font-semibold transition-colors',
        'text-zinc-800 hover:bg-zinc-100 dark:text-zinc-100 dark:hover:bg-zinc-800',
      )}
    >
      <span className="truncate">{name || '이름 없음'}</span>
      <Pencil className="h-3 w-3 shrink-0 text-zinc-400 opacity-0 transition-opacity group-hover:opacity-100" />
    </button>
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
