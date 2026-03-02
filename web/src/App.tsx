import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

// Shared QueryClient instance with sensible defaults for the dashboard.
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
 * Root application component.
 * Provides React Query context to the entire component tree.
 * Router and layout shell will be added in Milestone 2.
 */
export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <div className="flex min-h-screen items-center justify-center">
        <h1 className="text-2xl font-semibold">XFlow Dashboard</h1>
      </div>
    </QueryClientProvider>
  );
}
