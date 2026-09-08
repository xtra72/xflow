// SPEC-SUBFLOW-001 그룹 B: 플로우 레벨 포트 관리 패널.
//
// 플로우 입력/출력 포트를 추가·이름 변경·삭제하는 패널이다(REQ-SUBFLOW-B03).
// 편집은 editorStore 의 flowInputs/flowOutputs 를 갱신하며, 경계 노드 핸들이
// 즉시 재구성되고 dirty 로 표시된다(REQ-SUBFLOW-B05). 포트 삭제 시 그 포트를
// 쓰던 경계 와이어도 함께 정리된다(스토어 removeFlowPort, REQ-SUBFLOW-A04).

import { useEffect, useRef, useState } from 'react';
import { ArrowLeftToLine, ArrowRightToLine, Pencil, Trash2, X } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useEditorStore } from '@/stores/editorStore';
import type { FlowPortDef } from '@/lib/flow/boundary';

interface FlowPortPanelProps {
  /** 닫기 버튼 콜백. */
  onClose: () => void;
}

/**
 * 플로우 포트 관리 패널.
 * 입력 포트 / 출력 포트 섹션으로 나뉘며, 각 섹션에 포트 추가 버튼과
 * 포트별 인라인 이름 편집 + 삭제 버튼을 제공한다.
 */
export function FlowPortPanel({ onClose }: FlowPortPanelProps) {
  const { t } = useTranslation();
  const flowInputs = useEditorStore((s) => s.flowInputs);
  const flowOutputs = useEditorStore((s) => s.flowOutputs);
  // 포트 추가는 에디터 툴바의 빠른 추가 버튼을 사용하므로 여기서는 제거됨
  const renameFlowPort = useEditorStore((s) => s.renameFlowPort);
  const removeFlowPort = useEditorStore((s) => s.removeFlowPort);

  return (
    <div
      className={cn(
        'flex w-64 flex-col rounded-lg border border-zinc-200 bg-(--color-bg-surface) shadow-lg',
        'dark:border-zinc-700 dark:bg-zinc-900',
      )}
      role="dialog"
      aria-label={t('editor.port.panelAria')}
    >
      {/* 헤더 */}
      <div className="flex items-center justify-between border-b border-zinc-200 px-3 py-2 dark:border-zinc-700">
        <span className="text-sm font-semibold text-zinc-800 dark:text-zinc-100">
          {t('editor.toolbar.portPanel')}
        </span>
        <button
          type="button"
          onClick={onClose}
          title={t('common.close')}
          aria-label={t('editor.port.closeAria')}
          className="rounded p-0.5 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-zinc-800"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      <div className="flex flex-col gap-3 p-3">
        <PortSection
          title={t('editor.port.inputSection')}
          icon={ArrowRightToLine}
          accent="input"
          ports={flowInputs}
          onRename={(id, name) => renameFlowPort('input', id, name)}
          onRemove={(id) => removeFlowPort('input', id)}
        />
        <PortSection
          title={t('editor.port.outputSection')}
          icon={ArrowLeftToLine}
          accent="output"
          ports={flowOutputs}
          onRename={(id, name) => renameFlowPort('output', id, name)}
          onRemove={(id) => removeFlowPort('output', id)}
        />
      </div>
    </div>
  );
}

interface PortSectionProps {
  title: string;
  icon: React.ComponentType<{ className?: string }>;
  accent: 'input' | 'output';
  ports: FlowPortDef[];
  onRename: (id: string, name: string) => void;
  onRemove: (id: string) => void;
}

/** 단일 방향(입력/출력) 포트 섹션. */
function PortSection({
  title,
  icon: Icon,
  accent,
  ports,
  onRename,
  onRemove,
}: PortSectionProps) {
  const { t } = useTranslation();
  const accentText =
    accent === 'input'
      ? 'text-blue-600 dark:text-blue-300'
      : 'text-emerald-600 dark:text-emerald-300';

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center justify-between">
        <span
          className={cn(
            'flex items-center gap-1 text-xs font-semibold uppercase tracking-wide',
            accentText,
          )}
        >
          <Icon className="h-3 w-3 shrink-0" />
          {title}
        </span>
        {/* 포트 추가 버튼은 제거됨 — 에디터 툴바의 빠른 추가 버튼으로 통합됨 */}
      </div>

      {ports.length === 0 ? (
        <div className="px-1 py-0.5 text-xs italic text-zinc-400">
          {t('editor.port.empty')}
        </div>
      ) : (
        <ul className="flex flex-col gap-1">
          {ports.map((port) => (
            <PortRow
              key={port.id}
              port={port}
              onRename={(name) => onRename(port.id, name)}
              onRemove={() => onRemove(port.id)}
            />
          ))}
        </ul>
      )}
    </div>
  );
}

interface PortRowProps {
  port: FlowPortDef;
  onRename: (name: string) => void;
  onRemove: () => void;
}

/**
 * 단일 포트 행: 이름 표시 + 인라인 편집 + 삭제.
 * 이름 클릭 또는 연필 버튼으로 편집 모드 진입, Enter/blur 저장, Escape 취소.
 */
function PortRow({ port, onRename, onRemove }: PortRowProps) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(port.name);
  const inputRef = useRef<HTMLInputElement>(null);
  const committedRef = useRef(false);

  useEffect(() => {
    if (editing) {
      inputRef.current?.focus();
      inputRef.current?.select();
    }
  }, [editing]);

  const startEditing = () => {
    setDraft(port.name);
    committedRef.current = false;
    setEditing(true);
  };

  const commit = () => {
    if (committedRef.current) return;
    committedRef.current = true;
    const trimmed = draft.trim();
    // 빈 값이거나 변경이 없으면 저장하지 않는다(스토어도 동일 가드).
    if (trimmed && trimmed !== port.name) {
      onRename(trimmed);
    }
    setEditing(false);
  };

  const cancel = () => {
    committedRef.current = true;
    setEditing(false);
  };

  if (editing) {
    return (
      <li>
        <input
          ref={inputRef}
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          // 에디터 전역 단축키(Delete/Ctrl+Z 등) 발동을 막는다.
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
          aria-label={t('editor.port.nameAria')}
          className={cn(
            'w-full rounded border border-blue-400 bg-(--color-bg-surface) px-1.5 py-0.5 text-xs',
            'text-zinc-900 outline-none focus:ring-1 focus:ring-blue-400',
            'dark:bg-zinc-800 dark:text-zinc-100',
          )}
        />
      </li>
    );
  }

  return (
    <li
      className={cn(
        'group flex items-center justify-between gap-1 rounded px-1.5 py-1',
        'bg-zinc-50 text-xs text-zinc-700 dark:bg-zinc-800 dark:text-zinc-200',
      )}
    >
      <button
        type="button"
        onClick={startEditing}
        title={t('editor.port.rename')}
        className="flex-1 truncate text-left"
      >
        {port.name}
      </button>
      <div className="flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
        <button
          type="button"
          onClick={startEditing}
          title={t('editor.port.rename')}
          aria-label={t('editor.port.renameAria').replace('{name}', port.name)}
          className="rounded p-0.5 text-zinc-400 hover:bg-zinc-200 hover:text-zinc-700 dark:hover:bg-zinc-700"
        >
          <Pencil className="h-3 w-3" />
        </button>
        <button
          type="button"
          onClick={onRemove}
          title={t('common.delete')}
          aria-label={t('editor.port.deleteAria').replace('{name}', port.name)}
          className="rounded p-0.5 text-zinc-400 hover:bg-red-100 hover:text-red-600 dark:hover:bg-red-900/40"
        >
          <Trash2 className="h-3 w-3" />
        </button>
      </div>
    </li>
  );
}
