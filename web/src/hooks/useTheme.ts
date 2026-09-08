// 테마 관리 훅.
// 모드(system/day/night) 해석, <html> data-theme 속성, 사용자 팔레트 오버라이드
// 적용을 담당한다.

import { useCallback, useEffect, useMemo, useSyncExternalStore } from 'react';

import { useUIStore } from '@/stores/uiStore';
import { ALL_TOKEN_VARS, type PresetId } from '@/lib/theme/tokens';

/** OS 다크 모드 선호 여부를 구독한다. SSR·테스트 환경(matchMedia 없음)에서는 false. */
function subscribePrefersDark(onChange: () => void): () => void {
  if (typeof window === 'undefined' || !window.matchMedia) return () => {};
  const mql = window.matchMedia('(prefers-color-scheme: dark)');
  mql.addEventListener('change', onChange);
  return () => mql.removeEventListener('change', onChange);
}

function getPrefersDark(): boolean {
  if (typeof window === 'undefined' || !window.matchMedia) return false;
  return window.matchMedia('(prefers-color-scheme: dark)').matches;
}

/**
 * 현재 테마 모드, 해석된 팔레트, 테마 제어 기능을 제공한다.
 *
 * `<html>` 에 data-theme 속성(day|night)과 dark 클래스를 관리하고,
 * 해당 팔레트에 사용자 오버라이드가 있으면 인라인 CSS 변수로 덮어쓴다.
 * 오버라이드가 없는 토큰은 index.css 의 기본 팔레트 값을 그대로 쓴다.
 */
export function useTheme() {
  const theme = useUIStore((s) => s.theme);
  const setTheme = useUIStore((s) => s.setTheme);
  const themeOverrides = useUIStore((s) => s.themeOverrides);

  // OS 설정 변경에 반응한다(system 모드가 아닐 때는 결과에 영향이 없다).
  const prefersDark = useSyncExternalStore(subscribePrefersDark, getPrefersDark, () => false);

  /** 실제로 화면에 적용되는 팔레트 */
  const resolvedPreset = useMemo((): PresetId => {
    if (theme === 'system') return prefersDark ? 'night' : 'day';
    return theme;
  }, [theme, prefersDark]);

  // data-theme 속성 + dark 클래스 적용.
  // dark 클래스는 아직 남아 있는 Tailwind `dark:` 변형과의 호환을 위해 유지한다.
  useEffect(() => {
    const root = document.documentElement;
    root.dataset.theme = resolvedPreset;
    root.classList.toggle('dark', resolvedPreset === 'night');
  }, [resolvedPreset]);

  // 사용자 오버라이드를 인라인 CSS 변수로 적용한다.
  // 매번 전체 토큰을 지우고 오버라이드된 것만 다시 설정하므로,
  // 팔레트를 전환하거나 초기화해도 이전 값이 남지 않는다.
  useEffect(() => {
    const root = document.documentElement;
    const overrides = themeOverrides[resolvedPreset] ?? {};
    for (const cssVar of ALL_TOKEN_VARS) {
      const value = overrides[cssVar];
      if (value) root.style.setProperty(cssVar, value);
      else root.style.removeProperty(cssVar);
    }
  }, [resolvedPreset, themeOverrides]);

  /** 테마 순환 토글: system -> day -> night -> system */
  const toggleTheme = useCallback(() => {
    const order = ['system', 'day', 'night'] as const;
    const idx = order.indexOf(theme);
    setTheme(order[(idx + 1) % order.length]!);
  }, [theme, setTheme]);

  return { theme, resolvedPreset, setTheme, toggleTheme };
}
