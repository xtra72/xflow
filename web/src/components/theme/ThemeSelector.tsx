// 테마 선택 드롭다운 컴포넌트.
// Header에서 기존 3-cycle 토글을 대체한다.

import { useState, useRef, useEffect } from 'react';
import { Monitor, Sun, Moon, Palette, Check, Settings2 } from 'lucide-react';

import { useTheme } from '@/hooks/useTheme';
import { cn } from '@/lib/utils/cn';
import type { ThemeMode } from '@/stores/uiStore';

interface ThemeOption {
  value: ThemeMode;
  label: string;
  icon: typeof Sun;
}

const THEME_OPTIONS: ThemeOption[] = [
  { value: 'system', label: 'System', icon: Monitor },
  { value: 'day', label: 'Day', icon: Sun },
  { value: 'night', label: 'Night', icon: Moon },
  { value: 'custom', label: 'Custom', icon: Palette },
];

interface ThemeSelectorProps {
  /** 커스텀 테마 에디터 열기 콜백 */
  onOpenEditor?: () => void;
}

export function ThemeSelector({ onOpenEditor }: ThemeSelectorProps) {
  const { theme, setTheme } = useTheme();
  const [isOpen, setIsOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // 외부 클릭 감지
  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [isOpen]);

  // ESC 키 감지
  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setIsOpen(false);
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [isOpen]);

  const currentOption = THEME_OPTIONS.find((o) => o.value === theme) ?? THEME_OPTIONS[0]!;
  const TriggerIcon = currentOption.icon;

  return (
    <div ref={containerRef} className="relative">
      {/* 트리거 버튼 */}
      <button
        type="button"
        onClick={() => setIsOpen(!isOpen)}
        className={cn(
          'rounded-md p-2 text-gray-500 transition-colors',
          'hover:bg-gray-100 hover:text-gray-700',
          'dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200',
        )}
        aria-label="테마 선택"
        aria-expanded={isOpen}
      >
        <TriggerIcon className="h-5 w-5" aria-hidden="true" />
      </button>

      {/* 드롭다운 */}
      {isOpen && (
        <div className="absolute right-0 top-full z-50 mt-1 w-40 rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-gray-700 dark:bg-gray-800">
          {THEME_OPTIONS.map((option) => {
            const Icon = option.icon;
            const isActive = theme === option.value;
            return (
              <button
                key={option.value}
                type="button"
                onClick={() => {
                  setTheme(option.value);
                  setIsOpen(false);
                }}
                className={cn(
                  'flex w-full items-center gap-3 px-3 py-2 text-sm transition-colors',
                  isActive
                    ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
                    : 'text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-700',
                )}
              >
                <Icon className="h-4 w-4" aria-hidden="true" />
                <span className="flex-1 text-left">{option.label}</span>
                {isActive && <Check className="h-4 w-4" aria-hidden="true" />}
                {option.value === 'custom' && (
                  <button
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation();
                      setIsOpen(false);
                      onOpenEditor?.();
                    }}
                    className="rounded p-0.5 hover:bg-gray-200 dark:hover:bg-gray-600"
                    aria-label="커스텀 테마 편집"
                  >
                    <Settings2 className="h-3.5 w-3.5" aria-hidden="true" />
                  </button>
                )}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
