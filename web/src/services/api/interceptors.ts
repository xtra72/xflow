import type { AxiosInstance, AxiosRequestConfig, InternalAxiosRequestConfig } from 'axios';

import type { AuthTokens } from '@/types/auth';

/** 토큰 갱신 엔드포인트. 이 요청의 401 은 갱신으로 되받지 않는다(재귀 차단). */
const REFRESH_URL = '/auth/refresh';

/**
 * 재시도 응답에 찍는 표식 — 봉투를 두 번 벗기지 않기 위한 것이다.
 *
 * 갱신 후 재시도한 요청은 자기 체인을 한 바퀴 돌며 봉투 인터셉터를 이미 지났다.
 * 그런데 이 인터셉터의 거부 핸들러가 값을 반환하면 axios 는 **원 요청의** 체인을
 * 이어서 돌린다 — 뒤따르는 봉투 인터셉터가 벗겨진 payload 를 다시 봉투로 보고
 * `success` 가 없다며 APIError('UNKNOWN') 를 던진다. client.ts 가 이 표식을 보고
 * 그 두 번째 통과를 건너뛴다.
 */
export const ALREADY_UNWRAPPED = Symbol('xflow.envelope.unwrapped');

/** 이미 인터셉터가 붙은 인스턴스 — 중복 등록 방지. */
const installed = new WeakSet<AxiosInstance>();

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

/** 재시도 응답에 ALREADY_UNWRAPPED 표식을 찍어 되돌려 준다. */
function markUnwrapped<T>(response: T): T {
  (response as Record<PropertyKey, unknown>)[ALREADY_UNWRAPPED] = true;
  return response;
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
    const originalRequest = error.config as
      | (AxiosRequestConfig & { _retry?: boolean })
      | undefined;

    // 401 판정은 원본 axios 에러에서만 가능하다 — 이 인터셉터가 봉투 인터셉터보다
    // **먼저** 등록되어야 하는 까닭이다(client.ts 참조). 뒤에 붙으면 봉투가 던지는
    // APIError 에는 response 도 config 도 없어, 401 을 알아보지도 원 요청을
    // 재시도하지도 못한 채 그대로 통과한다 — 자동 갱신과 만료 로그아웃이 한 번도
    // 동작하지 않던 결함이 정확히 그 자리였다.
    if (error.response?.status !== 401 || !originalRequest || originalRequest._retry) {
      throw error;
    }

    // 갱신 요청 자신의 401 은 여기서 되받지 않는다 — 되받으면 갱신이 갱신을 부른다.
    if (originalRequest.url === REFRESH_URL) {
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
      }).then(async (token) => {
        if (originalRequest.headers) {
          (originalRequest.headers as Record<string, string>)['Authorization'] =
            `Bearer ${token}`;
        }
        originalRequest._retry = true;
        return markUnwrapped(await instance(originalRequest));
      });
    }

    isRefreshing = true;
    originalRequest._retry = true;

    try {
      // Call the refresh endpoint directly (not through intercepted client).
      const response = await instance.post<AuthTokens>(REFRESH_URL, {
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
      return markUnwrapped(await instance(originalRequest));
    } catch (refreshError) {
      processPendingQueue(null, refreshError);
      // logout() 이 isAuthenticated 를 내리면 AuthGuard 가 /login 으로 보낸다.
      // 모든 앱 라우트가 AuthGuard 아래에 있으므로 이 한 줄이 전이를 만든다.
      // 종전의 window.location.href 는 전체 새로고침이라 SPA 상태를 통째로 버렸다.
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
  // 두 번 붙으면 401 한 번에 갱신이 두 번 돈다. 호출 지점이 옮겨 다닌 이력이 있어
  // 멱등하게 둔다(client.ts 가 부르고, 옛 호출 지점이 남아 있어도 무해하도록).
  if (installed.has(instance)) return;
  installed.add(instance);
  setupAuthInterceptor(instance);
  setupRefreshInterceptor(instance);
}
