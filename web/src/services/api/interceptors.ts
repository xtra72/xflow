import type { AxiosInstance, AxiosRequestConfig, InternalAxiosRequestConfig } from 'axios';

import type { AuthTokens } from '@/types/auth';

// Flag + queue used to serialize concurrent 401 refresh attempts.
let isRefreshing = false;
let pendingQueue: Array<{
  resolve: (token: string) => void;
  reject: (error: unknown) => void;
}> = [];

function processPendingQueue(token: string | null, error: unknown = null): void {
  for (const entry of pendingQueue) {
    if (token) {
      entry.resolve(token);
    } else {
      entry.reject(error);
    }
  }
  pendingQueue = [];
}

/**
 * Attach Authorization: Bearer header from the auth store.
 * The auth store is imported lazily (dynamic import) to avoid
 * circular dependency issues between the store and the API client.
 */
export function setupAuthInterceptor(instance: AxiosInstance): void {
  instance.interceptors.request.use(async (config: InternalAxiosRequestConfig) => {
    // Lazy import to break circular dependency (store -> apiClient -> store).
    const { useAuthStore } = await import('@/stores/authStore');
    const token = useAuthStore.getState().tokens?.access_token;
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  });
}

/**
 * Intercept 401 responses and attempt silent token refresh.
 *
 * - Only one refresh request runs at a time; concurrent 401s are queued
 *   and replayed once the refresh succeeds.
 * - If the refresh itself fails the user is logged out.
 */
export function setupRefreshInterceptor(instance: AxiosInstance): void {
  instance.interceptors.response.use(undefined, async (error) => {
    const originalRequest = error.config as AxiosRequestConfig & { _retry?: boolean };

    // Only handle 401 and avoid infinite retry loops.
    if (error.response?.status !== 401 || originalRequest._retry) {
      throw error;
    }

    const { useAuthStore } = await import('@/stores/authStore');
    const refreshToken = useAuthStore.getState().tokens?.refresh_token;

    // No refresh token available -- force logout.
    if (!refreshToken) {
      useAuthStore.getState().logout();
      throw error;
    }

    // If a refresh is already in flight, queue this request.
    if (isRefreshing) {
      return new Promise<string>((resolve, reject) => {
        pendingQueue.push({ resolve, reject });
      }).then((token) => {
        if (originalRequest.headers) {
          (originalRequest.headers as Record<string, string>)['Authorization'] =
            `Bearer ${token}`;
        }
        originalRequest._retry = true;
        return instance(originalRequest);
      });
    }

    isRefreshing = true;
    originalRequest._retry = true;

    try {
      // Call the refresh endpoint directly (not through intercepted client).
      const response = await instance.post<AuthTokens>('/auth/refresh', {
        refresh_token: refreshToken,
      });

      const tokens = response.data;
      useAuthStore.getState().setTokens(tokens);

      // Replay queued requests with the new access token.
      processPendingQueue(tokens.access_token);

      // Retry the original request.
      if (originalRequest.headers) {
        (originalRequest.headers as Record<string, string>)['Authorization'] =
          `Bearer ${tokens.access_token}`;
      }
      return instance(originalRequest);
    } catch (refreshError) {
      processPendingQueue(null, refreshError);
      useAuthStore.getState().logout();
      throw refreshError;
    } finally {
      isRefreshing = false;
    }
  });
}

/**
 * Apply all interceptors to the given axios instance.
 */
export function setupInterceptors(instance: AxiosInstance): void {
  setupAuthInterceptor(instance);
  setupRefreshInterceptor(instance);
}
