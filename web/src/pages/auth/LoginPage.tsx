// 로그인 페이지 컴포넌트.
// 사용자명/비밀번호 입력 폼을 제공하며 인증 API를 호출한다.

import { useState } from 'react';
import { useNavigate } from 'react-router';
import { Eye, EyeOff, LogIn } from 'lucide-react';

import { useAuth } from '@/hooks/useAuth';
import { useTranslation } from '@/lib/i18n';

/** 로그인 성공 후 이동할 경로. 대시보드는 인증만 요구하므로 항상 도달 가능하다. */
const DASHBOARD_PATH = '/';

/**
 * 로그인 페이지.
 *
 * 사용자명과 비밀번호를 입력받아 인증 후 **항상 대시보드로** 이동한다.
 * 세션 만료 직전에 보던 화면으로 되돌아가지 않는다 — 로그인 직후의 시작점은
 * 언제나 대시보드 하나로 고정한다.
 */
export default function LoginPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();

  const { login, isLoading } = useAuth();

  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState('');

  /** 폼 제출 핸들러 */
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    if (!username.trim()) {
      setError(t('auth.usernameRequired'));
      return;
    }
    if (!password) {
      setError(t('auth.passwordRequired'));
      return;
    }

    try {
      await login(username.trim(), password);
      navigate(DASHBOARD_PATH, { replace: true });
    } catch {
      setError(t('auth.loginError'));
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-(--color-bg-base) p-4">
      <div className="w-full max-w-sm">
        {/* 헤더 */}
        <div className="mb-8 text-center">
          <h1 className="text-2xl font-bold text-(--color-text-primary)">
            {t('auth.loginTitle')}
          </h1>
          <p className="mt-2 text-sm text-(--color-text-muted)">
            {t('auth.loginSubtitle')}
          </p>
        </div>

        {/* 로그인 폼 */}
        <form onSubmit={(e) => void handleSubmit(e)} className="space-y-4">
          {/* 에러 메시지 */}
          {error && (
            <div
              className="rounded-md border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400"
              role="alert"
            >
              {error}
            </div>
          )}

          {/* 사용자명 입력 */}
          <div>
            <label
              htmlFor="username"
              className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('auth.username')}
            </label>
            <input
              id="username"
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              disabled={isLoading}
              autoComplete="username"
              autoFocus
              className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-3 py-2 text-sm text-(--color-text-primary) placeholder:text-(--color-text-muted) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
              placeholder={t('auth.username')}
            />
          </div>

          {/* 비밀번호 입력 */}
          <div>
            <label
              htmlFor="password"
              className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('auth.password')}
            </label>
            <div className="relative">
              <input
                id="password"
                type={showPassword ? 'text' : 'password'}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                disabled={isLoading}
                autoComplete="current-password"
                className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-3 py-2 pr-10 text-sm text-(--color-text-primary) placeholder:text-(--color-text-muted) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
                placeholder={t('auth.passwordPlaceholder')}
              />
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-(--color-text-muted) hover:text-(--color-text-secondary)"
                aria-label={showPassword ? t('auth.hidePassword') : t('auth.showPassword')}
                tabIndex={-1}
              >
                {showPassword ? (
                  <EyeOff className="h-4 w-4" aria-hidden="true" />
                ) : (
                  <Eye className="h-4 w-4" aria-hidden="true" />
                )}
              </button>
            </div>
          </div>

          {/* 로그인 버튼 */}
          <button
            type="submit"
            disabled={isLoading}
            className="flex w-full items-center justify-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {isLoading ? (
              <>
                <span className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                {t('auth.loggingIn')}
              </>
            ) : (
              <>
                <LogIn className="h-4 w-4" aria-hidden="true" />
                {t('auth.loginButton')}
              </>
            )}
          </button>
        </form>
      </div>
    </div>
  );
}
