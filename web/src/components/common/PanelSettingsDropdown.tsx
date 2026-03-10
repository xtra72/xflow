// 패널 헤더에 배치되는 설정 드롭다운.
// 타이틀 편집 + 표시 항목 선택 기능을 제공한다.
// Portal을 사용해 부모 overflow에 영향받지 않는다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Settings } from 'lucide-react';

export interface ColumnOption<T extends string> {
  key: T;
  label: string;
}

interface PanelSettingsDropdownProps<T extends string> {
  title: string;
  onTitleChange: (title: string) => void;
  columns: ColumnOption<T>[];
  visibleColumns: T[];
  onColumnsChange: (columns: T[]) => void;
}

export default function PanelSettingsDropdown<T extends string>({
  title,
  onTitleChange,
  columns,
  visibleColumns,
  onColumnsChange,
}: PanelSettingsDropdownProps<T>) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(title);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ top: 0, left: 0 });

  // 메뉴 위치 계산
  const updatePosition = useCallback(() => {
    if (!buttonRef.current) return;
    const rect = buttonRef.current.getBoundingClientRect();
    const menuWidth = 224; // w-56 = 14rem = 224px
    setPos({
      top: rect.bottom + 4,
      left: rect.right - menuWidth,
    });
  }, []);

  // 열릴 때 위치 계산 + 스크롤/리사이즈 추적
  useEffect(() => {
    if (!open) return;
    updatePosition();

    window.addEventListener('scroll', updatePosition, true);
    window.addEventListener('resize', updatePosition);
    return () => {
      window.removeEventListener('scroll', updatePosition, true);
      window.removeEventListener('resize', updatePosition);
    };
  }, [open, updatePosition]);

  // 외부 클릭 닫기
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as Node;
      if (
        buttonRef.current?.contains(target) ||
        menuRef.current?.contains(target)
      ) return;
      setOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  // title prop 변경 시 draft 동기화
  useEffect(() => {
    setDraft(title);
  }, [title]);

  const handleTitleBlur = () => {
    const trimmed = draft.trim();
    if (trimmed) {
      onTitleChange(trimmed);
    } else {
      setDraft(title);
    }
  };

  const handleToggle = (key: T) => {
    if (visibleColumns.includes(key)) {
      if (visibleColumns.length <= 1) return;
      onColumnsChange(visibleColumns.filter((k) => k !== key));
    } else {
      onColumnsChange([...visibleColumns, key]);
    }
  };

  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        onClick={() => setOpen(!open)}
        className="rounded-md p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-700 dark:hover:text-gray-300"
        aria-label="패널 설정"
      >
        <Settings className="h-4 w-4" />
      </button>

      {open &&
        createPortal(
          <div
            ref={menuRef}
            className="fixed z-50 w-56 rounded-md border border-gray-200 bg-white py-2 shadow-lg dark:border-gray-600 dark:bg-gray-800"
            style={{ top: pos.top, left: pos.left }}
          >
            {/* 타이틀 편집 */}
            <div className="px-3 pb-2">
              <label className="mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400">
                타이틀
              </label>
              <input
                type="text"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                onBlur={handleTitleBlur}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') handleTitleBlur();
                }}
                className="w-full rounded border border-gray-300 bg-white px-2 py-1 text-sm text-gray-900 focus:border-blue-500 focus:outline-none dark:border-gray-600 dark:bg-gray-700 dark:text-white"
              />
            </div>

            <div className="my-1 border-t border-gray-200 dark:border-gray-700" />

            {/* 표시 항목 */}
            <div className="px-3 pt-1">
              <span className="mb-2 block text-xs font-medium text-gray-500 dark:text-gray-400">
                표시 항목
              </span>
              {columns.map((col) => {
                const checked = visibleColumns.includes(col.key);
                const isLast = checked && visibleColumns.length <= 1;
                return (
                  <label
                    key={col.key}
                    className={`flex items-center gap-2 rounded px-1 py-1 text-sm ${
                      isLast ? 'cursor-not-allowed opacity-50' : 'cursor-pointer hover:bg-gray-50 dark:hover:bg-gray-700'
                    }`}
                  >
                    <input
                      type="checkbox"
                      checked={checked}
                      disabled={isLast}
                      onChange={() => handleToggle(col.key)}
                      className="h-3.5 w-3.5 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                    />
                    <span className="text-gray-700 dark:text-gray-300">{col.label}</span>
                  </label>
                );
              })}
            </div>
          </div>,
          document.body,
        )}
    </>
  );
}
