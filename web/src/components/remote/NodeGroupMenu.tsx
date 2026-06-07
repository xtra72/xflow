// 노드 그룹 배정/해제 메뉴 (SPEC-REMOTE-001 M9, 그룹 K, REQ-K02/K12).
//
// 디렉토리 뷰의 노드 행에서 그룹을 배정/변경/해제하는 affordance 이다.
//   - 기존 그룹 라벨 목록에서 선택 → setGroup.
//   - 새 그룹 이름 입력(자유 입력) → 암묵적 그룹 생성(setGroup).
//   - "전체로 이동"(해제) → clearGroup.
//
// 그룹은 서버 운영 메타데이터이므로(A13) 노드로 명령을 전파하지 않는다(그룹 D 비경유).

import { useEffect, useRef, useState } from 'react';
import { Check, FolderInput, Plus } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

interface NodeGroupMenuProps {
  /** 현재 그룹 라벨(빈 문자열 = "전체"). */
  currentGroup: string;
  /** 배정 후보 그룹 라벨 목록("전체" 제외, distinct). */
  groupNames: string[];
  /** 진행 중 여부(중복 클릭 방지). */
  pending: boolean;
  /** 그룹 배정 핸들러(기존 또는 신규 라벨). */
  onAssign: (groupName: string) => void;
  /** 그룹 해제 핸들러("전체" 환원). */
  onClear: () => void;
}

/** 노드 그룹 배정/해제 드롭다운 메뉴. */
export function NodeGroupMenu({
  currentGroup,
  groupNames,
  pending,
  onAssign,
  onClear,
}: NodeGroupMenuProps): React.JSX.Element {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [newName, setNewName] = useState('');
  const containerRef = useRef<HTMLDivElement>(null);

  // 외부 클릭 시 닫기.
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent): void => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  const close = (): void => {
    setOpen(false);
    setNewName('');
  };

  const handleAssign = (groupName: string): void => {
    onAssign(groupName);
    close();
  };

  const handleCreate = (): void => {
    const trimmed = newName.trim();
    if (!trimmed) return;
    onAssign(trimmed);
    close();
  };

  const handleClear = (): void => {
    onClear();
    close();
  };

  return (
    <div ref={containerRef} className="relative shrink-0">
      <button
        type="button"
        disabled={pending}
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={t('remote.group.menuLabel')}
        title={t('remote.group.menuLabel')}
        data-testid="node-group-menu-button"
        className="rounded-md p-1 text-(--color-text-muted) opacity-60 transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary) group-hover:opacity-100 disabled:cursor-not-allowed disabled:opacity-40"
      >
        <FolderInput className="h-4 w-4" aria-hidden="true" />
      </button>

      {open && (
        <div
          role="menu"
          data-testid="node-group-menu"
          className="absolute right-0 z-20 mt-1 w-56 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-1 shadow-lg"
        >
          <p className="px-2 py-1 text-[10px] font-semibold uppercase tracking-wider text-(--color-text-muted)">
            {t('remote.group.moveTo')}
          </p>

          {/* "전체"로 이동(해제) */}
          <button
            type="button"
            role="menuitem"
            onClick={handleClear}
            data-testid="node-group-clear"
            className="flex w-full items-center justify-between gap-2 rounded px-2 py-1.5 text-left text-sm text-(--color-text-primary) transition-colors hover:bg-(--color-bg-elevated)"
          >
            <span>{t('remote.group.all')}</span>
            {currentGroup === '' && <Check className="h-3.5 w-3.5 text-blue-600" aria-hidden="true" />}
          </button>

          {/* 기존 그룹 목록 */}
          {groupNames.map((name) => (
            <button
              key={name}
              type="button"
              role="menuitem"
              onClick={() => handleAssign(name)}
              data-testid="node-group-option"
              data-group={name}
              className="flex w-full items-center justify-between gap-2 rounded px-2 py-1.5 text-left text-sm text-(--color-text-primary) transition-colors hover:bg-(--color-bg-elevated)"
            >
              <span className="truncate">{name}</span>
              {currentGroup === name && (
                <Check className="h-3.5 w-3.5 shrink-0 text-blue-600" aria-hidden="true" />
              )}
            </button>
          ))}

          {/* 새 그룹 입력 */}
          <div className="mt-1 flex items-center gap-1 border-t border-(--color-border-default) px-2 pt-2">
            <input
              type="text"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  handleCreate();
                }
              }}
              placeholder={t('remote.group.newPlaceholder')}
              aria-label={t('remote.group.newLabel')}
              data-testid="node-group-new-input"
              className="min-w-0 flex-1 rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
            />
            <button
              type="button"
              onClick={handleCreate}
              disabled={!newName.trim()}
              aria-label={t('remote.group.create')}
              title={t('remote.group.create')}
              data-testid="node-group-new-submit"
              className={cn(
                'shrink-0 rounded bg-blue-600 p-1 text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-40 dark:bg-blue-500 dark:hover:bg-blue-600',
              )}
            >
              <Plus className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
