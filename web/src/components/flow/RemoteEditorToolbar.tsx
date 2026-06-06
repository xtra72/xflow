// 원격 플로우 편집 툴바 (SPEC-REMOTE-001 M7, 그룹 I, REQ-I08/I10).
//
// 로컬 EditorToolbar 는 deploy/start/stop 등 로컬 /flows/{id}/* 라이프사이클
// 엔드포인트를 호출하므로 원격(미러) 플로우 편집에는 부적합하다(노드 측 자원이며
// 서버에 로컬 라이프사이클이 없음). 본 툴바는 원격 편집에 필요한 것만 제공한다:
//   - 저장(부모 EditorPage 의 handleSave 위임 — PATCH/POST 명령 전파)
//   - 실행 취소 / 다시 실행
//   - 플로우 포트 빠른 추가 + 포트 패널 토글
//   - 뷰 토글(그리드 스냅 / 가상 와이어 / 연결 포커스)
//   - 원격 편집 대상 노드 배지(로컬 vs 원격 구분 — REQ-I08)
//
// 라이프사이클/배포 컨트롤은 의도적으로 제외한다(원격 편집은 정의 편집만 지원).

import {
  ArrowLeftToLine,
  ArrowRightToLine,
  Eye,
  EyeOff,
  Focus,
  Grid3x3,
  PanelLeft,
  Redo2,
  Save,
  Server,
  Undo2,
} from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useEditorStore } from '@/stores/editorStore';
import { useUIStore } from '@/stores/uiStore';

interface RemoteEditorToolbarProps {
  /** 원격 편집 대상 노드 표시명(또는 instance_id). */
  nodeLabel: string;
  /** 편집 중인 플로우 이름(신규는 빈 문자열). */
  flowName: string;
  /** 신규 생성 모드 여부(저장 시 POST). */
  isNew: boolean;
  /** 저장 진행 중 여부(버튼 비활성). */
  isSaving: boolean;
  /** 저장 콜백(EditorPage.handleSave 위임). */
  onSave: () => void;
  /** 포트 패널 열림 여부. */
  showPortPanel: boolean;
  /** 포트 패널 토글 콜백. */
  onTogglePortPanel: () => void;
}

/**
 * 원격 플로우 편집 전용 툴바.
 */
export function RemoteEditorToolbar({
  nodeLabel,
  flowName,
  isNew,
  isSaving,
  onSave,
  showPortPanel,
  onTogglePortPanel,
}: RemoteEditorToolbarProps): React.JSX.Element {
  const { t } = useTranslation();

  const isDirty = useEditorStore((s) => s.isDirty);
  const undoStack = useEditorStore((s) => s.undoStack);
  const redoStack = useEditorStore((s) => s.redoStack);
  const undo = useEditorStore((s) => s.undo);
  const redo = useEditorStore((s) => s.redo);
  const addFlowInput = useEditorStore((s) => s.addFlowInput);
  const addFlowOutput = useEditorStore((s) => s.addFlowOutput);

  const editorSnapToGrid = useUIStore((s) => s.editorSnapToGrid);
  const toggleEditorSnapToGrid = useUIStore((s) => s.toggleEditorSnapToGrid);
  const showVirtualWires = useEditorStore((s) => s.showVirtualWires);
  const toggleShowVirtualWires = useEditorStore((s) => s.toggleShowVirtualWires);
  const focusConnectionsOnSelect = useEditorStore((s) => s.focusConnectionsOnSelect);
  const toggleFocusConnections = useEditorStore((s) => s.toggleFocusConnections);

  return (
    <div
      className="flex items-center gap-1 rounded-lg border border-zinc-200 bg-white px-2 py-1 shadow-sm dark:border-zinc-700 dark:bg-zinc-900"
      data-testid="remote-editor-toolbar"
    >
      {/* 원격 편집 대상 노드 배지(로컬 vs 원격 구분 — REQ-I08). */}
      <span
        className="inline-flex items-center gap-1 rounded-md bg-violet-100 px-2 py-0.5 text-xs font-medium text-violet-700 dark:bg-violet-900/40 dark:text-violet-300"
        title={t('remote.editor.targetNode')}
        data-testid="remote-editor-node-badge"
      >
        <Server className="h-3 w-3" aria-hidden="true" />
        {nodeLabel}
      </span>

      <span className="max-w-[200px] truncate px-1 text-sm font-semibold text-zinc-800 dark:text-zinc-100">
        {flowName || (isNew ? t('remote.editor.newFlow') : t('remote.editor.untitled'))}
      </span>

      <Separator />

      {/* 저장 — 원격은 PATCH(기존)/POST(신규) 명령 전파(REQ-I01/I02). */}
      <ToolbarButton
        icon={Save}
        label={t('remote.editor.save')}
        onClick={onSave}
        disabled={!isDirty || isSaving}
        badge={isDirty}
        testId="remote-editor-save"
      />

      <Separator />

      <ToolbarButton
        icon={Undo2}
        label={t('remote.editor.undo')}
        onClick={undo}
        disabled={undoStack.length === 0}
      />
      <ToolbarButton
        icon={Redo2}
        label={t('remote.editor.redo')}
        onClick={redo}
        disabled={redoStack.length === 0}
      />

      <Separator />

      <ToolbarButton
        icon={ArrowRightToLine}
        label={t('remote.editor.addInputPort')}
        onClick={addFlowInput}
      />
      <ToolbarButton
        icon={ArrowLeftToLine}
        label={t('remote.editor.addOutputPort')}
        onClick={addFlowOutput}
      />
      <ToolbarButton
        icon={PanelLeft}
        label={t('remote.editor.portPanel')}
        onClick={onTogglePortPanel}
        active={showPortPanel}
      />

      <Separator />

      <ToolbarButton
        icon={Grid3x3}
        label={t('remote.editor.snapGrid')}
        onClick={toggleEditorSnapToGrid}
        active={editorSnapToGrid}
      />
      <ToolbarButton
        icon={showVirtualWires ? Eye : EyeOff}
        label={t('remote.editor.virtualWires')}
        onClick={toggleShowVirtualWires}
        active={showVirtualWires}
      />
      <ToolbarButton
        icon={Focus}
        label={t('remote.editor.focusConnections')}
        onClick={toggleFocusConnections}
        active={focusConnectionsOnSelect}
      />
    </div>
  );
}

interface ToolbarButtonProps {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  onClick: () => void;
  disabled?: boolean;
  badge?: boolean;
  active?: boolean;
  testId?: string;
}

/** 툴바 버튼(로컬 EditorToolbar 와 동일 스타일). */
function ToolbarButton({
  icon: Icon,
  label,
  onClick,
  disabled,
  badge,
  active,
  testId,
}: ToolbarButtonProps): React.JSX.Element {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={label}
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

/** 수직 구분선. */
function Separator(): React.JSX.Element {
  return <div className="mx-0.5 h-5 w-px bg-zinc-200 dark:bg-zinc-700" />;
}
