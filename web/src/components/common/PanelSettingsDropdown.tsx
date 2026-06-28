// 패널 헤더에 배치되는 설정 드롭다운.
// 타이틀 편집 + 표시 항목 선택 + 패널 컬러 기능을 제공한다.
// Portal을 사용해 부모 overflow에 영향받지 않는다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Settings } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

export interface ColumnOption<T extends string> {
  key: T;
  label: string;
}

/** 패널 컬러 프리셋 */
const PANEL_COLORS = [
  '#3b82f6', // blue
  '#8b5cf6', // violet
  '#06b6d4', // cyan
  '#10b981', // emerald
  '#f59e0b', // amber
  '#ef4444', // red
  '#ec4899', // pink
  '#6b7280', // gray
];

interface PanelSettingsDropdownProps<T extends string> {
  title: string;
  onTitleChange: (title: string) => void;
  /** 표시 항목 컬럼 설정 (없으면 표시 항목 섹션 숨김) */
  columns?: ColumnOption<T>[];
  visibleColumns?: T[];
  onColumnsChange?: (columns: T[]) => void;
  /** 패널 컬러 */
  panelColor?: string;
  onPanelColorChange?: (color: string | undefined) => void;
  /** 추가 설정 항목 (드롭다운 하단에 렌더링) */
  children?: React.ReactNode;
}

export default function PanelSettingsDropdown<T extends string>({
  title,
  onTitleChange,
  columns,
  visibleColumns,
  onColumnsChange,
  panelColor,
  onPanelColorChange,
  children,
}: PanelSettingsDropdownProps<T>) {
  const { t } = useTranslation();
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
    if (!visibleColumns || !onColumnsChange) return;
    if (visibleColumns.includes(key)) {
      if (visibleColumns.length <= 1) return;
      onColumnsChange(visibleColumns.filter((k) => k !== key));
    } else {
      onColumnsChange([...visibleColumns, key]);
    }
  };

  const hasColumns = columns && columns.length > 0 && visibleColumns && onColumnsChange;

  return (
    <>
      <button
        ref={buttonRef}
        type="button"
        onClick={() => setOpen(!open)}
        className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
        style={panelColor ? { color: panelColor } : undefined}
        aria-label={t('panel.settings.aria')}
      >
        <Settings className="h-4 w-4" />
      </button>

      {open &&
        createPortal(
          <div
            ref={menuRef}
            className="fixed z-50 w-56 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) py-2 shadow-lg"
            style={{ top: pos.top, left: pos.left }}
          >
            {/* 타이틀 편집 */}
            <div className="px-3 pb-2">
              <label className="mb-1 block text-xs font-medium text-(--color-text-muted)">
                {t('panel.settings.title')}
              </label>
              <input
                type="text"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                onBlur={handleTitleBlur}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') handleTitleBlur();
                }}
                className="w-full rounded border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
              />
            </div>

            {/* 표시 항목 (columns가 있을 때만) */}
            {hasColumns && (
              <>
                <div className="my-1 border-t border-(--color-border-default)" />
                <div className="px-3 pt-1">
                  <span className="mb-2 block text-xs font-medium text-(--color-text-muted)">
                    {t('panel.settings.visibleColumns')}
                  </span>
                  {columns.map((col) => {
                    const checked = visibleColumns.includes(col.key);
                    const isLast = checked && visibleColumns.length <= 1;
                    return (
                      <label
                        key={col.key}
                        className={`flex items-center gap-2 rounded px-1 py-1 text-sm ${
                          isLast ? 'cursor-not-allowed opacity-50' : 'cursor-pointer hover:bg-(--color-bg-elevated)'
                        }`}
                      >
                        <input
                          type="checkbox"
                          checked={checked}
                          disabled={isLast}
                          onChange={() => handleToggle(col.key)}
                          className="h-3.5 w-3.5 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
                        />
                        <span className="text-(--color-text-secondary)">{col.label}</span>
                      </label>
                    );
                  })}
                </div>
              </>
            )}

            {/* 추가 설정 항목 */}
            {children}

            {/* 패널 컬러 */}
            {onPanelColorChange && (
              <>
                <div className="my-1 border-t border-(--color-border-default)" />
                <div className="px-3 pt-1 pb-1">
                  <span className="mb-2 block text-xs font-medium text-(--color-text-muted)">
                    {t('panel.settings.panelColor')}
                  </span>
                  <div className="flex flex-wrap gap-1.5">
                    {PANEL_COLORS.map((color) => (
                      <button
                        key={color}
                        type="button"
                        onClick={() => onPanelColorChange(color)}
                        className={`h-5 w-5 rounded-full border-2 transition-transform hover:scale-110 ${
                          panelColor === color ? 'border-white ring-2 ring-blue-500' : 'border-transparent'
                        }`}
                        style={{ backgroundColor: color }}
                        aria-label={color}
                      />
                    ))}
                  </div>
                  {panelColor && (
                    <button
                      type="button"
                      onClick={() => onPanelColorChange(undefined)}
                      className="mt-1.5 w-full rounded px-2 py-0.5 text-xs text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated)"
                    >
                      {t('panel.settings.reset')}
                    </button>
                  )}
                </div>
              </>
            )}
          </div>,
          document.body,
        )}
    </>
  );
}
