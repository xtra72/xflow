// 커스텀 테마 에디터 모달.
// 카테고리별 시맨틱 토큰 컬러 피커와 라이브 프리뷰를 제공한다.

import { useState, useEffect, useCallback } from 'react';
import { X } from 'lucide-react';

import { TOKEN_CATEGORIES, DAY_PRESET, ALL_TOKEN_VARS } from '@/lib/theme/tokens';
import type { ThemeTokens } from '@/lib/theme/tokens';
import { useTranslation } from '@/lib/i18n';
import { useUIStore } from '@/stores/uiStore';
import { ColorTokenInput } from '@/components/theme/ColorTokenInput';
import { cn } from '@/lib/utils/cn';

interface ThemeEditorModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export function ThemeEditorModal({ isOpen, onClose }: ThemeEditorModalProps) {
  const { t } = useTranslation();
  const customThemeTokens = useUIStore((s) => s.customThemeTokens);
  const setCustomThemeTokens = useUIStore((s) => s.setCustomThemeTokens);

  // 편집 중인 임시 토큰 상태
  const [editTokens, setEditTokens] = useState<ThemeTokens>({});

  // 모달 열릴 때 현재 커스텀 토큰으로 초기화 (없으면 Day 프리셋)
  useEffect(() => {
    if (isOpen) {
      const initial: ThemeTokens = {};
      for (const v of ALL_TOKEN_VARS) {
        initial[v] = customThemeTokens[v] ?? DAY_PRESET[v] ?? '';
      }
      setEditTokens(initial);
    }
  }, [isOpen, customThemeTokens]);

  // 라이브 프리뷰: 편집 중 토큰을 <html> 인라인 스타일에 즉시 반영
  useEffect(() => {
    if (!isOpen) return;
    const root = document.documentElement;
    for (const [varName, value] of Object.entries(editTokens)) {
      if (value) root.style.setProperty(varName, value);
    }
  }, [isOpen, editTokens]);

  const handleTokenChange = useCallback((cssVar: string, value: string) => {
    setEditTokens((prev) => ({ ...prev, [cssVar]: value }));
  }, []);

  const handleSave = useCallback(() => {
    setCustomThemeTokens(editTokens);
    onClose();
  }, [editTokens, setCustomThemeTokens, onClose]);

  const handleCancel = useCallback(() => {
    // 임시 인라인 스타일 제거 (useTheme이 다시 적용)
    const root = document.documentElement;
    for (const v of ALL_TOKEN_VARS) {
      root.style.removeProperty(v);
    }
    // 기존 커스텀 토큰 복원
    for (const [varName, value] of Object.entries(customThemeTokens)) {
      if (value) root.style.setProperty(varName, value);
    }
    onClose();
  }, [customThemeTokens, onClose]);

  const handleReset = useCallback(() => {
    const dayTokens: ThemeTokens = {};
    for (const v of ALL_TOKEN_VARS) {
      dayTokens[v] = DAY_PRESET[v] ?? '';
    }
    setEditTokens(dayTokens);
  }, []);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* 백드롭 */}
      <div
        className="absolute inset-0 bg-black/50"
        onClick={handleCancel}
        role="presentation"
      />

      {/* 모달 */}
      <div
        className={cn(
          'relative z-10 max-h-[80vh] w-full max-w-lg overflow-y-auto rounded-xl',
          'border border-gray-200 bg-white p-6 shadow-2xl',
          'dark:border-gray-700 dark:bg-gray-800',
        )}
      >
        {/* 헤더 */}
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white">
            {t('dashboard.theme.editTitle')}
          </h2>
          <button
            type="button"
            onClick={handleCancel}
            className="rounded-md p-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* 토큰 카테고리별 편집 */}
        {TOKEN_CATEGORIES.map((cat) => (
          <div key={cat.category} className="mb-4">
            <h3 className="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
              {cat.label}
            </h3>
            <div className="grid grid-cols-2 gap-2">
              {cat.tokens.map((token) => (
                <ColorTokenInput
                  key={token.cssVar}
                  label={token.label}
                  value={editTokens[token.cssVar] ?? ''}
                  onChange={(v) => handleTokenChange(token.cssVar, v)}
                />
              ))}
            </div>
          </div>
        ))}

        {/* 액션 버튼 */}
        <div className="mt-6 flex items-center justify-between border-t border-gray-200 pt-4 dark:border-gray-700">
          <button
            type="button"
            onClick={handleReset}
            className="rounded-md px-3 py-1.5 text-sm text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
          >
            {t('dashboard.theme.reset')}
          </button>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={handleCancel}
              className={cn(
                'rounded-md border border-gray-300 px-4 py-1.5 text-sm',
                'text-gray-700 hover:bg-gray-50',
                'dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700',
              )}
            >
              {t('common.cancel')}
            </button>
            <button
              type="button"
              onClick={handleSave}
              className="rounded-md bg-blue-600 px-4 py-1.5 text-sm text-white hover:bg-blue-700"
            >
              {t('common.save')}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
