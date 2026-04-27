// 에러 경계 컴포넌트.
// 하위 컴포넌트 트리에서 발생하는 렌더링 에러를 포착하여
// 앱 전체가 크래시되지 않도록 대체 UI를 표시한다.

import { Component, type ErrorInfo, type ReactNode } from 'react';

import { useTranslation } from '@/lib/i18n';

interface ErrorBoundaryProps {
  children: ReactNode;
  /** 에러 발생 시 표시할 커스텀 UI. 미지정 시 기본 폴백 사용. */
  fallback?: ReactNode;
}

interface ErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
}

/** 에러 경계 기본 폴백 UI. 함수 컴포넌트로 분리하여 useTranslation 훅 사용. */
function ErrorFallbackUI({ error, onReload }: { error: Error | null; onReload: () => void }) {
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

/**
 * React 에러 경계.
 * 자식 트리의 렌더링 에러를 포착하고 대체 UI를 보여준다.
 */
export default class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    // 에러 로깅 (추후 외부 모니터링 서비스 연동 가능)
    console.error('[ErrorBoundary] 렌더링 에러 발생:', error);
    console.error('[ErrorBoundary] 컴포넌트 스택:', errorInfo.componentStack);
  }

  /** 에러 상태 초기화 후 페이지 새로고침 */
  private handleReload = () => {
    this.setState({ hasError: false, error: null });
    window.location.reload();
  };

  render() {
    if (this.state.hasError) {
      // 커스텀 fallback이 있으면 사용
      if (this.props.fallback) {
        return this.props.fallback;
      }

      // 기본 폴백 UI (함수 컴포넌트를 사용하여 i18n 지원)
      return <ErrorFallbackUI error={this.state.error} onReload={this.handleReload} />;
    }

    return this.props.children;
  }
}
