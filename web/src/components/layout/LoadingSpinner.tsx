// 전체 화면 로딩 스피너 컴포넌트.
// Suspense fallback 및 라우트 전환 시 사용한다.

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

interface LoadingSpinnerProps {
  /** 추가 CSS 클래스 */
  className?: string;
  /** 스피너 크기 (px 기준 Tailwind 클래스) */
  size?: 'sm' | 'md' | 'lg';
}

const sizeClasses = {
  sm: 'h-5 w-5',
  md: 'h-8 w-8',
  lg: 'h-12 w-12',
} as const;

/**
 * 중앙 정렬된 로딩 스피너.
 * Suspense fallback이나 비동기 라우트 전환 시 표시된다.
 */
export default function LoadingSpinner({ className, size = 'md' }: LoadingSpinnerProps) {
  const { t } = useTranslation();

  return (
    <div className={cn('flex min-h-screen items-center justify-center', className)}>
      <div
        className={cn(
          'animate-spin rounded-full border-2 border-gray-300 border-t-blue-600',
          'dark:border-gray-600 dark:border-t-blue-400',
          sizeClasses[size],
        )}
        role="status"
        aria-label={t('common.loading')}
      >
        <span className="sr-only">{t('common.loading')}</span>
      </div>
    </div>
  );
}
