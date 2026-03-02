// 로그인 페이지 컴포넌트.
// 이메일/비밀번호 인증 폼과 클라이언트 측 유효성 검사를 제공한다.

import { useState, type FormEvent } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { Eye, EyeOff, Loader2 } from 'lucide-react';

import { useAuth } from '@/hooks/useAuth';
import { APIError } from '@/types/api';
import { cn } from '@/lib/utils/cn';

// 간단한 이메일 형식 정규식
const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

interface FieldErrors {
  email?: string;
  password?: string;
}

/**
 * 로그인 페이지.
 * 이메일/비밀번호 폼, 유효성 검사, 서버 에러 표시,
 * 인증 성공 시 returnUrl 또는 '/'로 리다이렉트한다.
 */
export default function LoginPage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { isAuthenticated, isLoading, login } = useAuth();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [serverError, setServerError] = useState('');

  // 이미 인증된 사용자는 홈으로 리다이렉트
  if (isAuthenticated) {
    // navigate는 렌더링 중 호출하면 안 되므로 useEffect 대신
    // 조건부 렌더링으로 처리하고, 실제 리다이렉트는 아래에서 수행
    const returnUrl = searchParams.get('returnUrl') || '/';
    navigate(returnUrl, { replace: true });
    return null;
  }

  /** 클라이언트 측 유효성 검사 */
  function validate(): boolean {
    const errors: FieldErrors = {};

    if (!email.trim()) {
      errors.email = '이메일을 입력해 주세요.';
    } else if (!EMAIL_REGEX.test(email)) {
      errors.email = '올바른 이메일 형식이 아닙니다.';
    }

    if (!password) {
      errors.password = '비밀번호를 입력해 주세요.';
    }

    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  }

  /** 폼 제출 핸들러 */
  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setServerError('');

    if (!validate()) return;

    try {
      await login(email, password);
      const returnUrl = searchParams.get('returnUrl') || '/';
      navigate(returnUrl, { replace: true });
    } catch (error) {
      // 서버 에러 메시지 표시 후 비밀번호 필드 초기화
      if (error instanceof APIError) {
        setServerError(error.message);
      } else {
        setServerError('로그인 중 오류가 발생했습니다. 다시 시도해 주세요.');
      }
      setPassword('');
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-gray-50 px-4 dark:bg-gray-900">
      <div className="w-full max-w-md space-y-8">
        {/* 브랜딩 */}
        <div className="text-center">
          <h1 className="text-3xl font-bold tracking-tight text-gray-900 dark:text-white">
            XFlow
          </h1>
          <p className="mt-2 text-sm text-gray-600 dark:text-gray-400">
            대시보드에 로그인하세요
          </p>
        </div>

        {/* 카드 */}
        <div className="rounded-lg bg-white p-8 shadow dark:bg-gray-800">
          {/* 서버 에러 배너 */}
          {serverError && (
            <div
              role="alert"
              className="mb-6 rounded-md bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/30 dark:text-red-400"
            >
              {serverError}
            </div>
          )}

          <form onSubmit={handleSubmit} noValidate className="space-y-5">
            {/* 이메일 필드 */}
            <div>
              <label
                htmlFor="login-email"
                className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                이메일
              </label>
              <input
                id="login-email"
                type="email"
                autoComplete="email"
                required
                value={email}
                onChange={(e) => {
                  setEmail(e.target.value);
                  if (fieldErrors.email) setFieldErrors((prev) => ({ ...prev, email: undefined }));
                }}
                aria-invalid={!!fieldErrors.email}
                aria-describedby={fieldErrors.email ? 'email-error' : undefined}
                className={cn(
                  'block w-full rounded-md border px-3 py-2 text-sm shadow-sm',
                  'placeholder:text-gray-400',
                  'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
                  'dark:bg-gray-700 dark:text-white dark:border-gray-600 dark:placeholder:text-gray-500',
                  fieldErrors.email
                    ? 'border-red-500 focus:ring-red-500 focus:border-red-500'
                    : 'border-gray-300 dark:border-gray-600',
                )}
                placeholder="name@example.com"
              />
              {fieldErrors.email && (
                <p id="email-error" className="mt-1 text-sm text-red-500" role="alert">
                  {fieldErrors.email}
                </p>
              )}
            </div>

            {/* 비밀번호 필드 */}
            <div>
              <label
                htmlFor="login-password"
                className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
              >
                비밀번호
              </label>
              <div className="relative">
                <input
                  id="login-password"
                  type={showPassword ? 'text' : 'password'}
                  autoComplete="current-password"
                  required
                  value={password}
                  onChange={(e) => {
                    setPassword(e.target.value);
                    if (fieldErrors.password)
                      setFieldErrors((prev) => ({ ...prev, password: undefined }));
                  }}
                  aria-invalid={!!fieldErrors.password}
                  aria-describedby={fieldErrors.password ? 'password-error' : undefined}
                  className={cn(
                    'block w-full rounded-md border px-3 py-2 pr-10 text-sm shadow-sm',
                    'placeholder:text-gray-400',
                    'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
                    'dark:bg-gray-700 dark:text-white dark:border-gray-600 dark:placeholder:text-gray-500',
                    fieldErrors.password
                      ? 'border-red-500 focus:ring-red-500 focus:border-red-500'
                      : 'border-gray-300 dark:border-gray-600',
                  )}
                  placeholder="비밀번호 입력"
                />
                {/* 비밀번호 표시/숨기기 토글 */}
                <button
                  type="button"
                  onClick={() => setShowPassword((prev) => !prev)}
                  className="absolute inset-y-0 right-0 flex items-center pr-3 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                  aria-label={showPassword ? '비밀번호 숨기기' : '비밀번호 표시'}
                >
                  {showPassword ? (
                    <EyeOff className="h-4 w-4" aria-hidden="true" />
                  ) : (
                    <Eye className="h-4 w-4" aria-hidden="true" />
                  )}
                </button>
              </div>
              {fieldErrors.password && (
                <p id="password-error" className="mt-1 text-sm text-red-500" role="alert">
                  {fieldErrors.password}
                </p>
              )}
            </div>

            {/* 로그인 버튼 */}
            <button
              type="submit"
              disabled={isLoading}
              className={cn(
                'flex w-full items-center justify-center rounded-md px-4 py-2 text-sm font-medium text-white',
                'bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2',
                'dark:focus:ring-offset-gray-800',
                'disabled:cursor-not-allowed disabled:opacity-50',
                'transition-colors',
              )}
            >
              {isLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                  로그인 중...
                </>
              ) : (
                '로그인'
              )}
            </button>
          </form>
        </div>
      </div>
    </div>
  );
}
