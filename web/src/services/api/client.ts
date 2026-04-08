import axios, { type AxiosInstance, type AxiosRequestConfig } from 'axios';

import { APIError, type APIResponse, type PaginationMeta } from '@/types/api';

// Axios instance configured for the XFlow backend API.
// Vite dev server proxy handles forwarding /api requests to the Go backend.
const apiClient: AxiosInstance = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Response interceptor: unwrap the API envelope.
// Successful responses have their `data.data` extracted so callers
// receive the domain payload directly. Error responses are converted
// to APIError instances.
apiClient.interceptors.response.use(
  (response) => {
    // 204 No Content 등 빈 body 응답은 envelope 파싱 없이 통과
    if (response.status === 204 || response.data == null) {
      return response;
    }

    const body = response.data as APIResponse;

    if (!body.success) {
      const err = body.error;
      throw new APIError(
        err?.code ?? 'UNKNOWN',
        err?.message ?? 'Unknown API error',
        response.status,
        err?.details,
      );
    }

    // Attach pagination meta to the response so list helpers can access it.
    // We store it on a non-standard property that our helpers read.
    (response as unknown as Record<string, unknown>)['_meta'] = body.meta;

    // Replace response.data with the unwrapped payload.
    response.data = body.data;
    return response;
  },
  (error) => {
    if (axios.isAxiosError(error) && error.response) {
      const body = error.response.data as APIResponse | undefined;
      if (body?.error) {
        throw new APIError(
          body.error.code,
          body.error.message,
          error.response.status,
          body.error.details,
        );
      }
    }
    throw error;
  },
);

// ---- Typed request helpers ----

/**
 * GET request returning typed data.
 */
export async function get<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
  const response = await apiClient.get<T>(url, config);
  return response.data;
}

/**
 * GET request for paginated list endpoints.
 * Returns both the data array and the total count from pagination metadata.
 */
export async function getList<T>(
  url: string,
  config?: AxiosRequestConfig,
): Promise<{ data: T[]; total: number }> {
  const response = await apiClient.get<T[]>(url, config);
  const meta = (response as unknown as Record<string, unknown>)['_meta'] as
    | { pagination?: PaginationMeta }
    | undefined;
  return {
    data: response.data,
    total: meta?.pagination?.total ?? 0,
  };
}

/**
 * POST request returning typed data.
 */
export async function post<T>(
  url: string,
  data?: unknown,
  config?: AxiosRequestConfig,
): Promise<T> {
  const response = await apiClient.post<T>(url, data, config);
  return response.data;
}

/**
 * PUT request returning typed data.
 */
export async function put<T>(
  url: string,
  data?: unknown,
  config?: AxiosRequestConfig,
): Promise<T> {
  const response = await apiClient.put<T>(url, data, config);
  return response.data;
}

/**
 * DELETE request returning void.
 */
export async function del(url: string, config?: AxiosRequestConfig): Promise<void> {
  await apiClient.delete(url, config);
}

export { apiClient };
