// 플로우 에디터 상단 툴바 컴포넌트.
// 저장, 배포, 실행 제어, 실행 취소/다시 실행 버튼과 플로우 상태 배지를 제공한다.

import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router';
import {
  ArrowLeftToLine,
  ArrowRightToLine,
  Check,
  ChevronDown,
  Eye,
  EyeOff,
  Focus,
  Grid3x3,
  Layers,
  Minus,
  Pencil,
  Plus,
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
  useFlows,
  useFlowStatus,
  useUpdateFlow,
  useDeployFlow,
  useStartFlow,
  useStopFlow,
  useRestartFlow,
} from '@/hooks/useFlow';
import {
  FOCUS_DEPTH_ALL,
  FOCUS_DEPTH_MAX,
  FOCUS_DEPTH_MIN,
  useEditorStore,
} from '@/stores/editorStore';
import { useUIStore } from '@/stores/uiStore';
import { serializeFlowDefinition } from '@/lib/flow/boundary';
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
      {/* 플로우 이름 (클릭하여 인라인 편집) + 다른 플로우로 전환하는 선택기 */}
      <div className="flex items-center gap-0.5">
        <FlowNameEditor flowId={flowId} />
        <FlowSwitcher flowId={flowId} />
      </div>

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

      {/* SPEC-SUBFLOW-001 M4: 플로우 포트 빠른 추가 — 패널을 열지 않고도
          제어판에서 입력/출력 포트를 바로 추가한다. 이름 변경·삭제는 플로우 포트
          패널에서 계속 처리한다. */}
      <ToolbarButton
        icon={ArrowRightToLine}
        label="입력 포트 추가"
        onClick={addFlowInput}
      />
      <ToolbarButton
        icon={ArrowLeftToLine}
        label="출력 포트 추가"
        onClick={addFlowOutput}
      />

      <Separator />

      {/* 그리드 스냅 토글 (v0.18.4) */}
      <ToolbarButton
        icon={Grid3x3}
        label={editorSnapToGrid ? '그리드 스냅 끄기' : '그리드 스냅 켜기'}
        onClick={toggleEditorSnapToGrid}
        active={editorSnapToGrid}
      />

      {/* 가상 와이어 표시 토글 — 켜면 가상 와이어 선을 일반 연결선처럼 그린다. */}
      <ToolbarButton
        icon={showVirtualWires ? Eye : EyeOff}
        label={showVirtualWires ? '가상 와이어 숨김' : '가상 와이어 표시'}
        onClick={toggleShowVirtualWires}
        active={showVirtualWires}
      />

      {/* 연결 포커스 토글 — 켜고 노드를 선택하면 연결만 강조하고 나머지를 흐리게. */}
      <ToolbarButton
        icon={Focus}
        label="연결 포커스"
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
        title="다른 플로우로 전환"
        aria-label="다른 플로우로 전환"
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
          aria-label="플로우 목록"
          className={cn(
            'absolute left-0 top-full z-50 mt-1 max-h-72 w-60 overflow-y-auto',
            'rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) py-1 shadow-lg',
          )}
        >
          {flows.length === 0 ? (
            <div className="px-3 py-2 text-xs text-(--color-text-muted)">
              플로우가 없습니다
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
                  <span className="truncate">{flow.name || '이름 없음'}</span>
                </button>
              );
            })
          )}
        </div>
      )}
    </div>
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
  const isAll = depth === FOCUS_DEPTH_ALL;
  const atMin = depth <= FOCUS_DEPTH_MIN;
  const atMax = isAll;

  // `-`: 전체면 유한 상한(5) 으로, 그 외에는 한 단계 줄인다.
  const decrement = () => onChange(isAll ? FOCUS_DEPTH_MAX : depth - 1);
  // `+`: 유한 상한(5) 에서는 전체로, 그 외에는 한 단계 늘린다.
  const increment = () =>
    onChange(depth >= FOCUS_DEPTH_MAX ? FOCUS_DEPTH_ALL : depth + 1);

  const label = isAll ? '전체' : String(depth);
  const title = isAll
    ? '연결 단계: 전체 (선택 노드의 전체 연결 체인 강조)'
    : `연결 단계: ${depth}단계 (선택 노드로부터 ${depth} hop 이내 강조)`;

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
        aria-label="연결 단계 줄이기"
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
        aria-label="연결 단계 늘리기"
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
