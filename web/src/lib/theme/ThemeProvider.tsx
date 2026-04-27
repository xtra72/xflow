// 테마 관리를 활성화하는 래퍼 컴포넌트.
// useTheme 훅을 호출하여 dark 클래스 토글, 시스템 설정 감지, localStorage 저장 등을 수행한다.

import type { ReactNode } from 'react';

import { useTheme } from '@/hooks/useTheme';

interface ThemeProviderProps {
  children: ReactNode;
}

/**
 * 컴포넌트 트리 최상단에 배치하여 테마 관리 로직을 활성화한다.
 * useTheme 훅이 모든 실제 작업(dark 클래스 토글, OS 설정 감지, localStorage 저장)을 수행하므로
 * 이 컴포넌트는 해당 훅을 호출하고 자식을 렌더링하는 역할만 담당한다.
 */
export function ThemeProvider({ children }: ThemeProviderProps) {
  // 테마 관리 로직 활성화 (dark 클래스 토글, 시스템 설정 감지, localStorage 저장)
  useTheme();

  return <>{children}</>;
}
