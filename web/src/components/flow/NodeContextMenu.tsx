// 노드 우클릭 컨텍스트 메뉴.
// 화면 좌표(x/y)에 고정 위치로 표시되는 작은 팝업 메뉴이며,
// "복제" / "삭제" 액션을 제공한다. 외부 클릭 / Escape / 액션 실행 시 닫힌다.

import { useEffect, useRef } from 'react';
import { Copy, Trash2 } from 'lucide-react';

import { cn } from '@/lib/utils/cn';

interface NodeContextMenuProps {
  /** 메뉴 표시 화면 좌표 (clientX/clientY) */
  x: number;
  y: number;
  /** 복제 실행 콜백 */
  onDuplicate: () => void;
  /** 삭제 실행 콜백 */
  onDelete: () => void;
  /** 메뉴 닫기 콜백 (외부 클릭 / Escape) */
  onClose: () => void;
}

/**
 * 노드 컨텍스트 메뉴 팝업.
 * 부모(EditorPageInner)가 열림 상태와 위치를 관리하고, 본 컴포넌트는
 * 위치 렌더링과 외부 클릭/Escape 감지만 담당한다.
 */
export function NodeContextMenu({
  x,
  y,
  onDuplicate,
  onDelete,
  onClose,
}: NodeContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);

  // 외부 클릭 및 Escape 로 메뉴를 닫는다.
  useEffect(() => {
    function handlePointerDown(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose();
      }
    }
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        e.stopPropagation();
        onClose();
      }
    }
    // capture 단계로 등록해 다른 핸들러보다 먼저 닫힘을 감지한다.
    window.addEventListener('mousedown', handlePointerDown, true);
    window.addEventListener('keydown', handleKeyDown, true);
    return () => {
      window.removeEventListener('mousedown', handlePointerDown, true);
      window.removeEventListener('keydown', handleKeyDown, true);
    };
  }, [onClose]);

  return (
    <div
      ref={menuRef}
      role="menu"
      style={{ top: y, left: x }}
      className={cn(
        'fixed z-50 min-w-[140px] overflow-hidden rounded-md border py-1',
        'border-(--color-border-default) bg-(--color-bg-elevated) shadow-lg',
      )}
    >
      <MenuItem icon={Copy} label="복제" onClick={onDuplicate} />
      <MenuItem icon={Trash2} label="삭제" danger onClick={onDelete} />
    </div>
  );
}

interface MenuItemProps {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  onClick: () => void;
  /** 위험 액션(삭제) 강조 */
  danger?: boolean;
}

/** 컨텍스트 메뉴 항목 */
function MenuItem({ icon: Icon, label, onClick, danger }: MenuItemProps) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onClick}
      className={cn(
        'flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm transition-colors',
        danger
          ? 'text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/30'
          : 'text-(--color-text-primary) hover:bg-zinc-100 dark:hover:bg-zinc-700',
      )}
    >
      <Icon className="h-4 w-4 shrink-0" />
      <span>{label}</span>
    </button>
  );
}
