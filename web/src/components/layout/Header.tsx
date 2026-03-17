// 헤더 컴포넌트.
// 페이지 제목, 사용자 정보, WebSocket 연결 상태, 테마 선택, 로그아웃을 표시한다.

import { useState } from 'react';
import { LogOut } from 'lucide-react';
import { useLocation } from 'react-router';

import { useAuth } from '@/hooks/useAuth';
import { useTranslation } from '@/lib/i18n';
import { useWebSocket } from '@/hooks/useWebSocket';
import { cn } from '@/lib/utils/cn';
import { ThemeSelector } from '@/components/theme/ThemeSelector';
import { ThemeEditorModal } from '@/components/theme/ThemeEditorModal';
import type { ConnectionState } from '@/services/ws/wsClient';

/** 라우트 경로에 따른 페이지 제목 번역 키 매핑 */
const PAGE_TITLE_KEYS: Record<string, string> = {
  '/': 'nav.dashboard',
  '/flows': 'nav.flows',
  '/monitoring': 'nav.monitoring',
  '/settings': 'nav.settings',
};

/** WebSocket 연결 상태에 따른 표시 색상 */
const CONNECTION_STYLES: Record<ConnectionState, { dot: string; labelKey: string }> = {
  connected: {
    dot: 'bg-green-500',
    labelKey: 'status.connected',
  },
  connecting: {
    dot: 'bg-yellow-500 animate-pulse',
    labelKey: 'status.connecting',
  },
  reconnecting: {
    dot: 'bg-yellow-500 animate-pulse',
    labelKey: 'status.reconnecting',
  },
  disconnected: {
    dot: 'bg-red-500',
    labelKey: 'status.disconnected',
  },
};

/**
 * 앱 상단 헤더.
 * 현재 페이지 제목, 사용자 정보, 연결 상태, 테마 및 로그아웃 버튼을 표시한다.
 */
export default function Header() {
  const { t } = useTranslation();
  const location = useLocation();
  const { user, logout } = useAuth();
  const { state: wsState } = useWebSocket();
  const [editorOpen, setEditorOpen] = useState(false);

  // 현재 라우트에서 페이지 제목 결정
  const pageTitle = derivePageTitle(location.pathname, t);
  const connectionStyle = CONNECTION_STYLES[wsState];

  return (
    <header className="flex h-(--header-height) shrink-0 items-center justify-between border-b border-(--color-border-default) bg-(--color-bg-surface) px-6">
      {/* 페이지 제목 */}
      <h1 className="text-lg font-semibold text-(--color-text-primary)">{pageTitle}</h1>

      {/* 우측 액션 영역 */}
      <div className="flex items-center gap-4">
        {/* WebSocket 연결 상태 */}
        <div className="flex items-center gap-2" title={`WebSocket: ${t(connectionStyle.labelKey)}`}>
          <span
            className={cn('inline-block h-2 w-2 rounded-full', connectionStyle.dot)}
            aria-hidden="true"
          />
          <span className="text-xs text-(--color-text-muted)">{t(connectionStyle.labelKey)}</span>
        </div>

        {/* 테마 선택 */}
        <ThemeSelector onOpenEditor={() => setEditorOpen(true)} />

        {/* 커스텀 테마 에디터 */}
        <ThemeEditorModal isOpen={editorOpen} onClose={() => setEditorOpen(false)} />

        {/* 사용자 정보 */}
        {user && (
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-(--color-text-secondary)">
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
            'rounded-md p-2 text-(--color-text-muted) transition-colors',
            'hover:bg-(--color-bg-sunken) hover:text-(--color-text-secondary)',
          )}
          aria-label={t('auth.logout')}
        >
          <LogOut className="h-5 w-5" aria-hidden="true" />
        </button>
      </div>
    </header>
  );
}

/**
 * 라우트 경로에서 페이지 제목을 추출한다.
 * 에디터 경로(/editor/:flowId)의 경우 번역된 '에디터'를 반환한다.
 */
function derivePageTitle(pathname: string, t: (key: string) => string): string {
  // 정적 경로 매핑에서 확인
  if (pathname in PAGE_TITLE_KEYS) {
    return t(PAGE_TITLE_KEYS[pathname]!);
  }

  // 에디터 경로 패턴 (/editor/:flowId)
  if (pathname.startsWith('/editor/')) {
    return t('nav.editor');
  }

  return 'XFlow';
}
