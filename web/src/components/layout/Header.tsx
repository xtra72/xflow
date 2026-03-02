// 헤더 컴포넌트.
// 페이지 제목, 사용자 정보, WebSocket 연결 상태, 테마 토글, 로그아웃을 표시한다.

import { LogOut, Monitor, Moon, Sun } from 'lucide-react';
import { useLocation } from 'react-router';

import { useAuth } from '@/hooks/useAuth';
import { useTheme } from '@/hooks/useTheme';
import { useWebSocket } from '@/hooks/useWebSocket';
import { cn } from '@/lib/utils/cn';
import type { ConnectionState } from '@/services/ws/wsClient';

/** 라우트 경로에 따른 페이지 제목 매핑 */
const PAGE_TITLES: Record<string, string> = {
  '/': '대시보드',
  '/flows': '플로우',
  '/monitoring': '모니터링',
  '/settings': '설정',
};

/** WebSocket 연결 상태에 따른 표시 색상 */
const CONNECTION_STYLES: Record<ConnectionState, { dot: string; label: string }> = {
  connected: {
    dot: 'bg-green-500',
    label: '연결됨',
  },
  connecting: {
    dot: 'bg-yellow-500 animate-pulse',
    label: '연결 중',
  },
  reconnecting: {
    dot: 'bg-yellow-500 animate-pulse',
    label: '재연결 중',
  },
  disconnected: {
    dot: 'bg-red-500',
    label: '연결 끊김',
  },
};

/** 테마에 따른 아이콘 컴포넌트 매핑 */
const THEME_ICONS = {
  light: Sun,
  dark: Moon,
  system: Monitor,
} as const;

/**
 * 앱 상단 헤더.
 * 현재 페이지 제목, 사용자 정보, 연결 상태, 테마 및 로그아웃 버튼을 표시한다.
 */
export default function Header() {
  const location = useLocation();
  const { user, logout } = useAuth();
  const { theme, toggleTheme } = useTheme();
  const { state: wsState } = useWebSocket();

  // 현재 라우트에서 페이지 제목 결정
  const pageTitle = derivePageTitle(location.pathname);

  // 현재 테마에 맞는 아이콘
  const ThemeIcon = THEME_ICONS[theme];
  const connectionStyle = CONNECTION_STYLES[wsState];

  return (
    <header className="flex h-(--header-height) shrink-0 items-center justify-between border-b border-gray-200 bg-white px-6 dark:border-gray-700 dark:bg-gray-800">
      {/* 페이지 제목 */}
      <h1 className="text-lg font-semibold text-gray-900 dark:text-white">{pageTitle}</h1>

      {/* 우측 액션 영역 */}
      <div className="flex items-center gap-4">
        {/* WebSocket 연결 상태 */}
        <div className="flex items-center gap-2" title={`WebSocket: ${connectionStyle.label}`}>
          <span
            className={cn('inline-block h-2 w-2 rounded-full', connectionStyle.dot)}
            aria-hidden="true"
          />
          <span className="text-xs text-gray-500 dark:text-gray-400">{connectionStyle.label}</span>
        </div>

        {/* 테마 토글 */}
        <button
          type="button"
          onClick={toggleTheme}
          className={cn(
            'rounded-md p-2 text-gray-500 transition-colors',
            'hover:bg-gray-100 hover:text-gray-700',
            'dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200',
          )}
          aria-label={`테마 변경 (현재: ${theme})`}
        >
          <ThemeIcon className="h-5 w-5" aria-hidden="true" />
        </button>

        {/* 사용자 정보 */}
        {user && (
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-gray-700 dark:text-gray-300">
              {user.name}
            </span>
            <span
              className={cn(
                'rounded-full px-2 py-0.5 text-xs font-medium',
                user.role === 'admin'
                  ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                  : user.role === 'editor'
                    ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
                    : 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300',
              )}
            >
              {user.role}
            </span>
          </div>
        )}

        {/* 로그아웃 버튼 */}
        <button
          type="button"
          onClick={() => void logout()}
          className={cn(
            'rounded-md p-2 text-gray-500 transition-colors',
            'hover:bg-gray-100 hover:text-gray-700',
            'dark:text-gray-400 dark:hover:bg-gray-700 dark:hover:text-gray-200',
          )}
          aria-label="로그아웃"
        >
          <LogOut className="h-5 w-5" aria-hidden="true" />
        </button>
      </div>
    </header>
  );
}

/**
 * 라우트 경로에서 페이지 제목을 추출한다.
 * 에디터 경로(/editor/:flowId)의 경우 '에디터'를 반환한다.
 */
function derivePageTitle(pathname: string): string {
  // 정적 경로 매핑에서 확인
  if (pathname in PAGE_TITLES) {
    return PAGE_TITLES[pathname]!;
  }

  // 에디터 경로 패턴 (/editor/:flowId)
  if (pathname.startsWith('/editor/')) {
    return '에디터';
  }

  return 'XFlow';
}
