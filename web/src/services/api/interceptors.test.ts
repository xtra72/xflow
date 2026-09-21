// 401 처리 경로의 계약 (@SPEC:SPEC-AUTH-EXPIRY-001).
//
// 이 파일이 없던 동안 갱신 인터셉터는 한 번도 불리지 않았다. client.ts 의 봉투
// 인터셉터가 먼저 등록되어 axios 에러를 APIError 로 바꿔 던졌고, 갱신 인터셉터의
// 문턱(`error.response?.status`)은 그 APIError 를 알아보지 못했다. 시험이 없어
// 아무도 몰랐다 — 그래서 여기서 고정하는 것은 "등록 순서" 그 자체다.
//
// 주의: client.ts 를 import 하는 것만으로 인터셉터가 운영과 같은 순서로 붙는다.
// 이 시험은 그 배선을 그대로 쓴다(별도 인스턴스를 만들면 검증 대상이 사라진다).

import { beforeEach, describe, expect, it } from 'vitest';
import type { InternalAxiosRequestConfig } from 'axios';

import { apiClient, get } from './client';
import { useAuthStore } from '@/stores/authStore';

/** 서버가 실제로 싣는 401 봉투(dto.NewErrorResponse)로 거부한다. */
function reject401(config: InternalAxiosRequestConfig) {
  return Object.assign(new Error('Request failed with status code 401'), {
    isAxiosError: true,
    config,
    response: {
      data: { success: false, error: { code: 'UNAUTHORIZED', message: '만료된 토큰입니다' } },
      status: 401,
      statusText: 'Unauthorized',
      headers: {},
      config,
    },
    toJSON: () => ({}),
  });
}

function ok(config: InternalAxiosRequestConfig, data: unknown) {
  return { data: { success: true, data }, status: 200, statusText: 'OK', headers: {}, config };
}

let calls: string[];

beforeEach(() => {
  calls = [];
  useAuthStore.setState({
    tokens: { access_token: 'expired', refresh_token: 'refresh-ok', expires_at: 1 },
    isAuthenticated: true,
    user: null,
  });
});

describe('세션 만료 — 401 이 갱신 인터셉터에 닿는다', () => {
  it('401 을 받으면 갱신하고 원 요청을 재시도한다', async () => {
    apiClient.defaults.adapter = async (config) => {
      calls.push(`${String(config.method).toUpperCase()} ${config.url}`);
      if (config.url === '/auth/refresh') {
        return ok(config, { access_token: 'fresh', refresh_token: 'r2', expires_at: 2 });
      }
      // 첫 호출만 401, 갱신 뒤 재시도는 성공.
      if (calls.filter((c) => c.endsWith('/agents')).length === 1) throw reject401(config);
      return ok(config, [{ id: 'a1' }]);
    };

    await expect(get('/agents')).resolves.toEqual([{ id: 'a1' }]);
    expect(calls).toEqual(['GET /agents', 'POST /auth/refresh', 'GET /agents']);
    // 새 토큰이 스토어에 반영되어 이후 요청과 WS 가 함께 쓴다.
    expect(useAuthStore.getState().tokens?.access_token).toBe('fresh');
  });

  it('재시도에는 새 액세스 토큰이 실린다', async () => {
    const authHeaders: Array<string | undefined> = [];
    apiClient.defaults.adapter = async (config) => {
      calls.push(`${config.url}`);
      if (config.url === '/auth/refresh') {
        return ok(config, { access_token: 'fresh', refresh_token: 'r2', expires_at: 2 });
      }
      authHeaders.push(config.headers?.Authorization as string | undefined);
      if (calls.filter((c) => c === '/agents').length === 1) throw reject401(config);
      return ok(config, []);
    };

    await get('/agents');
    expect(authHeaders[0]).toBe('Bearer expired');
    expect(authHeaders[1]).toBe('Bearer fresh');
  });

  it('갱신이 실패하면 로그아웃한다 — AuthGuard 가 로그인 화면으로 보낸다', async () => {
    apiClient.defaults.adapter = async (config) => {
      calls.push(`${config.url}`);
      throw reject401(config); // 갱신 요청도 401
    };

    await expect(get('/agents')).rejects.toThrow();
    const state = useAuthStore.getState();
    expect(state.isAuthenticated).toBe(false);
    expect(state.tokens).toBeNull();
  });

  it('갱신 토큰이 없으면 갱신을 시도하지 않고 즉시 로그아웃한다', async () => {
    useAuthStore.setState({
      tokens: { access_token: 'expired', refresh_token: '', expires_at: 1 },
      isAuthenticated: true,
    });
    apiClient.defaults.adapter = async (config) => {
      calls.push(`${config.url}`);
      throw reject401(config);
    };

    await expect(get('/agents')).rejects.toThrow();
    expect(calls).toEqual(['/agents']);
    expect(useAuthStore.getState().isAuthenticated).toBe(false);
  });

  it('동시에 터진 401 들에 대해 갱신은 한 번만 돈다', async () => {
    apiClient.defaults.adapter = async (config) => {
      calls.push(`${config.url}`);
      if (config.url === '/auth/refresh') {
        return ok(config, { access_token: 'fresh', refresh_token: 'r2', expires_at: 2 });
      }
      const seen = calls.filter((c) => c === config.url).length;
      if (seen === 1) throw reject401(config); // 각 경로의 첫 호출만 401
      return ok(config, []);
    };

    await Promise.all([get('/agents'), get('/devices'), get('/flows')]);
    expect(calls.filter((c) => c === '/auth/refresh')).toHaveLength(1);
  });
});
