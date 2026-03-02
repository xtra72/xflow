import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import ErrorBoundary from '@/components/layout/ErrorBoundary';
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
 * ErrorBoundary > QueryClientProvider > Router 순서로 감싼다.
 */
export default function App() {
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <Router />
      </QueryClientProvider>
    </ErrorBoundary>
  );
}
