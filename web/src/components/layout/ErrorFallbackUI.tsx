// 에러 경계 기본 폴백 UI.
//
// 컴포넌트 파일이 컴포넌트만 내보내야 Fast Refresh 가 동작하므로, 클래스 컴포넌트를
// 내보내는 ErrorBoundary.tsx 에서 이 함수 컴포넌트를 분리했다
// (react-refresh/only-export-components).

import { useTranslation } from '@/lib/i18n';

/** 에러 경계 기본 폴백 UI. 함수 컴포넌트로 분리하여 useTranslation 훅 사용. */
export function ErrorFallbackUI({ error, onReload }: { error: Error | null; onReload: () => void }) {
  const { t } = useTranslation();

  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-(--color-bg-primary) px-4">
      <div className="text-center">
        <h2 className="text-xl font-semibold text-(--color-text-primary)">
          {t('error.somethingWentWrong')}
        </h2>
        <p className="mt-2 text-sm text-(--color-text-muted)">
          {error?.message || t('error.unknownError')}
        </p>
      </div>
      <button
        type="button"
        onClick={onReload}
        className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 dark:focus:ring-offset-gray-900"
      >
        {t('error.reload')}
      </button>
    </div>
  );
}
