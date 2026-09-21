import axios, { type AxiosInstance, type AxiosRequestConfig } from 'axios';

import { APIError, type APIResponse, type PaginationMeta } from '@/types/api';

import { ALREADY_UNWRAPPED, setupInterceptors } from './interceptors';

// Axios instance configured for the XFlow backend API.
// Vite dev server proxy handles forwarding /api requests to the Go backend.
//
// 주의: 인스턴스 기본 Content-Type 을 강제하지 않는다. axios v1 은 객체 본문에는
// 자동으로 application/json 을 설정하고, FormData 본문에는 multipart/form-data 와
// boundary 를 브라우저가 부여한다. 기본값으로 application/json 을 고정하면 axios 의
// transformRequest 가 FormData 를 JSON 으로 직렬화해버려(멀티파트 boundary 누락)
// 파일 업로드(릴리스 자산 등)가 서버에서 파싱 실패한다.
const apiClient: AxiosInstance = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
});

// 인증/갱신 인터셉터를 아래 봉투 인터셉터보다 **먼저** 붙인다. 순서가 곧 동작이다.
//
// axios 응답 인터셉터는 등록 순서대로 돈다. 아래 봉투 인터셉터는 실패 응답을
// APIError 로 바꿔 던지는데, APIError 에는 response 도 config 도 없다. 그러므로
// 갱신 인터셉터가 뒤에 붙으면 401 을 알아보지도(response.status) 원 요청을
// 재시도하지도(config) 못하고 그대로 흘려보낸다 — 실제로 그 순서였고(main.tsx 가
// 앱 시작 시 붙였다), 그래서 자동 토큰 갱신과 만료 시 로그아웃이 한 번도 동작하지
// 않았다. 세션이 만료되면 화면은 에러만 뿌린 채 로그인으로 가지 않았다.
//
// 등록을 이 파일로 옮긴 것은 그 순서를 코드가 스스로 보장하게 하기 위해서다.
// 모듈 적재 시점에 둘이 한자리에서 순서대로 붙으므로, 호출 지점이 달라져 순서가
// 뒤집히는 일이 생기지 않는다.
setupInterceptors(apiClient);

// Response interceptor: unwrap the API envelope.
// Successful responses have their `data.data` extracted so callers
// receive the domain payload directly. Error responses are converted
// to APIError instances.
apiClient.interceptors.response.use(
  (response) => {
    // 401 갱신 후 재시도된 응답은 자기 체인에서 이미 봉투가 벗겨진 채 이 자리로
    // 되돌아온다. 한 번 더 벗기면 payload 를 봉투로 오인해 APIError 를 던진다.
    if ((response as unknown as Record<PropertyKey, unknown>)[ALREADY_UNWRAPPED]) {
      return response;
    }

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
 * PATCH request returning typed data.
 */
export async function patch<T>(
  url: string,
  data?: unknown,
  config?: AxiosRequestConfig,
): Promise<T> {
  const response = await apiClient.patch<T>(url, data, config);
  return response.data;
}

/**
 * DELETE request returning void.
 */
export async function del(url: string, config?: AxiosRequestConfig): Promise<void> {
  await apiClient.delete(url, config);
}

/**
 * DELETE request returning typed data.
 *
 * 204 No Content 응답에서는 envelope 인터셉터가 빈 body 를 통과시키므로
 * `response.data` 는 undefined 가 될 수 있다. 호출자는 응답 본문이 항상
 * 존재하는 엔드포인트에서만 이 헬퍼를 사용해야 한다.
 */
export async function delWith<T>(url: string, config?: AxiosRequestConfig): Promise<T> {
  const response = await apiClient.delete<T>(url, config);
  return response.data;
}

export { apiClient };
