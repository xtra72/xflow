import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import ErrorBoundary from '@/components/layout/ErrorBoundary';
import { WebSocketProvider } from '@/hooks/useWebSocket';
import { I18nProvider } from '@/lib/i18n';
import { ThemeProvider } from '@/lib/theme/ThemeProvider';
import Router from '@/router';

// 대시보드 전역 QueryClient. 적절한 기본값을 설정한다.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 30 * 1000,
    },
    mutations: {
      retry: false,
    },
  },
});

/**
 * 루트 애플리케이션 컴포넌트.
 * I18nProvider > ErrorBoundary > ThemeProvider > QueryClientProvider > Router 순서로 감싼다.
 * ErrorBoundary가 I18nProvider 안에 있어야 ErrorFallbackUI에서 useTranslation을 사용할 수 있다.
 */
export default function App() {
  return (
    <I18nProvider>
      <ErrorBoundary>
        <ThemeProvider>
          <QueryClientProvider client={queryClient}>
            <WebSocketProvider>
              <Router />
            </WebSocketProvider>
          </QueryClientProvider>
        </ThemeProvider>
      </ErrorBoundary>
    </I18nProvider>
  );
}
