// 테마 관리 훅. 시스템 설정 감지, data-theme 속성, dark 클래스 호환, 커스텀 CSS 변수 적용을 담당한다.

import { useCallback, useEffect, useMemo } from 'react';

import { useUIStore } from '@/stores/uiStore';
import { ALL_TOKEN_VARS, DAY_PRESET } from '@/lib/theme/tokens';

/**
 * 현재 테마, 해석된 테마, 테마 제어 기능을 제공한다.
 * `<html>` 요소에 data-theme 속성과 dark 클래스를 관리하고,
 * 커스텀 테마 선택 시 인라인 CSS 변수를 적용한다.
 */
export function useTheme() {
  const theme = useUIStore((s) => s.theme);
  const setTheme = useUIStore((s) => s.setTheme);
  const customThemeTokens = useUIStore((s) => s.customThemeTokens);

  // 시스템 설정 기반 해석된 테마
  const resolvedTheme = useMemo((): 'day' | 'night' | 'custom' => {
    if (theme === 'system') {
      if (typeof window === 'undefined') return 'day';
      return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'night' : 'day';
    }
    return theme === 'custom' ? 'custom' : theme;
  }, [theme]);

  // data-theme 속성 + dark 클래스 적용
  useEffect(() => {
    const root = document.documentElement;

    // data-theme 속성 설정
    if (theme === 'custom') {
      root.dataset.theme = 'custom';
    } else {
      root.dataset.theme = resolvedTheme;
    }

    // dark 클래스 호환성 유지 (마이그레이션 기간)
    if (resolvedTheme === 'night') {
      root.classList.add('dark');
    } else {
      root.classList.remove('dark');
    }
  }, [theme, resolvedTheme]);

  // Custom 테마 인라인 CSS 변수 적용
  useEffect(() => {
    const root = document.documentElement;
    if (theme === 'custom') {
      // 커스텀 토큰 적용 (없는 토큰은 Day 프리셋 기본값 사용)
      for (const varName of ALL_TOKEN_VARS) {
        const value = customThemeTokens[varName] ?? DAY_PRESET[varName] ?? '';
        root.style.setProperty(varName, value);
      }
    } else {
      // Custom 아닌 경우 인라인 스타일 제거
      for (const varName of ALL_TOKEN_VARS) {
        root.style.removeProperty(varName);
      }
    }
  }, [theme, customThemeTokens]);

  // System 테마일 때 OS 설정 변경 감지
  useEffect(() => {
    if (theme !== 'system') return;

    const mql = window.matchMedia('(prefers-color-scheme: dark)');
    const handler = (e: MediaQueryListEvent) => {
      const root = document.documentElement;
      if (e.matches) {
        root.dataset.theme = 'night';
        root.classList.add('dark');
      } else {
        root.dataset.theme = 'day';
        root.classList.remove('dark');
      }
    };

    mql.addEventListener('change', handler);
    return () => mql.removeEventListener('change', handler);
  }, [theme]);

  /**
   * 테마 순환 토글: system -> day -> night -> system
   * @deprecated M15-4에서 ThemeSelector 드롭다운으로 대체 예정. 임시 호환용.
   */
  const toggleTheme = useCallback(() => {
    const order = ['system', 'day', 'night'] as const;
    const idx = order.indexOf(theme as (typeof order)[number]);
    const next = idx === -1 ? 'system' : order[(idx + 1) % order.length]!;
    setTheme(next);
  }, [theme, setTheme]);

  return { theme, resolvedTheme, setTheme, toggleTheme };
}
