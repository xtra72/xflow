// SPEC-WEB-006 v0.1.0 (M2, M3, M5, M6, M7) — System Update API Hooks 테스트.
//
// SPEC-UPDATE-001 v0.1.0 의 5 REST endpoint 를 소비하는 클라이언트 + React Query
// 훅에 대한 사양 테스트.
//
// 모든 HTTP 호출은 `./client` 의 `get`/`post` 헬퍼를 통해 envelope 가 이미
// 풀린 형태로 들어온다. 인터셉터가 4xx/5xx 를 `APIError` 로 변환하므로
// 테스트는 그 가정을 그대로 따른다.
//
// SPEC-UPDATE-002 v0.1.0 (M9, M10, M14):
//   - OperationStatus 11-state 확장 (restarting, health_checking 추가)
//   - ApplyRequest 신규 필드 (target, auto_restart)
//   - Target enum (xflowd | xflow-agent | xflow)
//
// @spec SPEC-WEB-006 v0.1.0
// @spec SPEC-UPDATE-001 v0.1.0
// @spec SPEC-UPDATE-002 v0.1.0 (M9, M10, M14)

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { type PropsWithChildren } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const getMock = vi.hoisted(() => vi.fn());
const postMock = vi.hoisted(() => vi.fn());
const putMock = vi.hoisted(() => vi.fn());

vi.mock('./client', () => ({
  get: getMock,
  post: postMock,
  put: putMock,
}));

import { APIError } from '@/types/api';

import {
  fetchChannelInfo,
  fetchSystemVersion,
  fetchUpdateStatus,
  postUpdateApply,
  postUpdateCheck,
  postUpdateRollback,
  putChannel,
  useChangeChannel,
  useChannelInfo,
  useSystemVersion,
  useUpdateApply,
  useUpdateCheck,
  useUpdateRollback,
  useUpdateStatus,
  type ApplyResponse,
  type ChangeChannelResponse,
  type ChannelInfo,
  type CheckResult,
  type OperationStatus,
  type RollbackResponse,
  type UpdateOperation,
  type VersionInfo,
} from './systemUpdate';

// ─────────────────────────────────────────────────────────────────────
// Pure API functions
// ─────────────────────────────────────────────────────────────────────

describe('fetchSystemVersion', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
  });

  it('GET /system/version 을 호출하고 VersionInfo 를 그대로 반환한다', async () => {
    const payload: VersionInfo = {
      version: 'v0.3.0',
      commit: '6b9c531',
      build_date: '2026-05-04T12:34:56Z',
      go_version: 'go1.22.5',
      channel: 'stable',
      update_available: false,
      latest_version: null,
    };
    getMock.mockResolvedValueOnce(payload);

    const result = await fetchSystemVersion();

    expect(getMock).toHaveBeenCalledWith('/system/version');
    expect(result).toEqual(payload);
  });

  it('401 응답은 APIError 로 전파된다', async () => {
    const err = new APIError('UNAUTHORIZED', 'unauthorized', 401);
    getMock.mockRejectedValueOnce(err);

    await expect(fetchSystemVersion()).rejects.toBe(err);
  });
});

describe('postUpdateCheck', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
  });

  it('POST /system/update/check 를 호출하고 CheckResult 를 반환한다', async () => {
    const payload: CheckResult = {
      current: 'v0.3.0',
      latest: 'v0.4.0',
      available: true,
      channel: 'stable',
      release_url: 'https://github.com/xtra72/xflow/releases/tag/v0.4.0',
      published_at: '2026-05-05T08:00:00Z',
    };
    postMock.mockResolvedValueOnce(payload);

    const result = await postUpdateCheck();

    expect(postMock).toHaveBeenCalledWith('/system/update/check');
    expect(result).toEqual(payload);
  });

  it('503 (네트워크/채널 오류) 는 APIError 로 전파된다', async () => {
    const err = new APIError('CHANNEL_UNAVAILABLE', 'channel unreachable', 503);
    postMock.mockRejectedValueOnce(err);

    await expect(postUpdateCheck()).rejects.toBe(err);
  });
});

describe('postUpdateApply', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
  });

  it('version + force 가 명시되면 본문 그대로 전송한다', async () => {
    const payload: ApplyResponse = {
      operation_id: 'upd-001',
      status: 'starting',
      from_version: 'v0.3.0',
      to_version: 'v0.4.0',
    };
    postMock.mockResolvedValueOnce(payload);

    const result = await postUpdateApply({ version: 'v0.4.0', force: true });

    expect(postMock).toHaveBeenCalledWith('/system/update/apply', {
      version: 'v0.4.0',
      force: true,
    });
    expect(result).toEqual(payload);
  });

  it('빈 body 는 빈 객체로 전송된다 (백엔드가 latest, force=false 기본값 적용)', async () => {
    const payload: ApplyResponse = {
      operation_id: 'upd-002',
      status: 'starting',
      from_version: 'v0.3.0',
      to_version: 'v0.4.0',
    };
    postMock.mockResolvedValueOnce(payload);

    const result = await postUpdateApply({});

    expect(postMock).toHaveBeenCalledWith('/system/update/apply', {});
    expect(result).toEqual(payload);
  });

  it('409 (이미 진행 중) 는 status 와 함께 APIError 로 전파된다', async () => {
    const err = new APIError(
      'UPDATE_IN_PROGRESS',
      'another update operation in progress',
      409,
    );
    postMock.mockRejectedValueOnce(err);

    await expect(postUpdateApply({})).rejects.toMatchObject({
      status: 409,
      code: 'UPDATE_IN_PROGRESS',
    });
  });

  // ───────────────────────────────────────────────────────────────────
  // SPEC-UPDATE-002 v0.1.0 (M9, M10, M14) — auto_restart + target
  // ───────────────────────────────────────────────────────────────────

  it('auto_restart=true 가 본문에 포함되어 전송된다 (M-1 in-process restart)', async () => {
    const payload: ApplyResponse = {
      operation_id: 'upd-ar',
      status: 'starting',
      from_version: 'v0.3.0',
      to_version: 'v0.4.0',
    };
    postMock.mockResolvedValueOnce(payload);

    await postUpdateApply({ auto_restart: true });

    expect(postMock).toHaveBeenCalledWith('/system/update/apply', {
      auto_restart: true,
    });
  });

  it('target=xflow-agent 가 본문에 포함되어 전송된다 (M9 multi-binary)', async () => {
    const payload: ApplyResponse = {
      operation_id: 'upd-agent',
      status: 'starting',
      from_version: 'v0.3.0',
      to_version: 'v0.4.0',
    };
    postMock.mockResolvedValueOnce(payload);

    await postUpdateApply({ target: 'xflow-agent' });

    expect(postMock).toHaveBeenCalledWith('/system/update/apply', {
      target: 'xflow-agent',
    });
  });

  it('target + auto_restart + version + force 동시 전송 (M9 + M10 + M14)', async () => {
    const payload: ApplyResponse = {
      operation_id: 'upd-full',
      status: 'starting',
      from_version: 'v0.3.0',
      to_version: 'v0.4.0',
    };
    postMock.mockResolvedValueOnce(payload);

    await postUpdateApply({
      version: 'v0.4.0',
      force: false,
      auto_restart: true,
      target: 'xflowd',
    });

    expect(postMock).toHaveBeenCalledWith('/system/update/apply', {
      version: 'v0.4.0',
      force: false,
      auto_restart: true,
      target: 'xflowd',
    });
  });

  it('target=xflow 의 경우 CLI 이므로 auto_restart 미설정으로 전송 가능 (Scenario 11)', async () => {
    const payload: ApplyResponse = {
      operation_id: 'upd-cli',
      status: 'starting',
      from_version: 'v0.3.0',
      to_version: 'v0.4.0',
    };
    postMock.mockResolvedValueOnce(payload);

    await postUpdateApply({ target: 'xflow' });

    expect(postMock).toHaveBeenCalledWith('/system/update/apply', {
      target: 'xflow',
    });
  });

  it('400 (의존성 매트릭스 위반) 은 APIError 로 전파된다 (M11 Scenario 12)', async () => {
    const err = new APIError(
      'UPDATE_INCOMPATIBLE_VERSION',
      'incompatible: xflow-agent v1.0.0 requires xflowd >=v1.0.0',
      400,
    );
    postMock.mockRejectedValueOnce(err);

    await expect(
      postUpdateApply({ target: 'xflow-agent' }),
    ).rejects.toMatchObject({
      status: 400,
      code: 'UPDATE_INCOMPATIBLE_VERSION',
    });
  });
});

describe('postUpdateRollback', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
  });

  it('POST /system/update/rollback 을 호출하고 RollbackResponse 를 반환한다', async () => {
    const payload: RollbackResponse = {
      from_version: 'v0.4.0',
      to_version: 'v0.3.0',
      rolled_back_at: '2026-05-05T11:00:00Z',
    };
    postMock.mockResolvedValueOnce(payload);

    const result = await postUpdateRollback();

    expect(postMock).toHaveBeenCalledWith('/system/update/rollback');
    expect(result).toEqual(payload);
  });

  it('409 (백업 없음) 는 APIError 로 전파된다', async () => {
    const err = new APIError('NO_BACKUP', 'no backup available', 409);
    postMock.mockRejectedValueOnce(err);

    await expect(postUpdateRollback()).rejects.toBe(err);
  });
});

describe('fetchUpdateStatus', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
  });

  it.each<OperationStatus>([
    'idle',
    'starting',
    'checking',
    'downloading',
    'verifying',
    'applying',
    'ready_to_restart',
    'restarting',
    'health_checking',
    'completed',
    'failed',
  ])('status=%s 인 응답을 그대로 반환한다 (11-state machine)', async (status) => {
    const payload: UpdateOperation = {
      operation_id: 'upd-99',
      status,
      from_version: 'v0.3.0',
      to_version: 'v0.4.0',
      started_at: '2026-05-05T10:00:00Z',
      completed_at: status === 'completed' ? '2026-05-05T10:05:00Z' : null,
      error: status === 'failed' ? 'apply failed' : null,
    };
    getMock.mockResolvedValueOnce(payload);

    const result = await fetchUpdateStatus();

    expect(getMock).toHaveBeenCalledWith('/system/update/status');
    expect(result.status).toBe(status);
    expect(result).toEqual(payload);
  });
});

// ─────────────────────────────────────────────────────────────────────
// React Query hooks
// ─────────────────────────────────────────────────────────────────────

function buildWrapper(client?: QueryClient) {
  const queryClient =
    client ??
    new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
          // 백그라운드 폴링이 테스트 종료 후에도 계속되는 것을 방지.
          refetchOnWindowFocus: false,
        },
        mutations: { retry: false },
      },
    });

  function Wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  }

  return { Wrapper, queryClient };
}

// ─────────────────────────────────────────────────────────────────────
// SPEC-UPDATE-002 v0.1.0 (M6, M7) — Channel API
// ─────────────────────────────────────────────────────────────────────

describe('fetchChannelInfo (SPEC-UPDATE-002 M6)', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
    putMock.mockReset();
  });

  it('GET /system/update/channel 을 호출하고 ChannelInfo 를 그대로 반환한다', async () => {
    const payload: ChannelInfo = {
      current: 'stable',
      available: ['stable', 'beta', 'nightly'],
    };
    getMock.mockResolvedValueOnce(payload);

    const result = await fetchChannelInfo();

    expect(getMock).toHaveBeenCalledWith('/system/update/channel');
    expect(result).toEqual(payload);
  });

  it('401 응답은 APIError 로 전파된다', async () => {
    const err = new APIError('UNAUTHORIZED', 'unauthorized', 401);
    getMock.mockRejectedValueOnce(err);

    await expect(fetchChannelInfo()).rejects.toBe(err);
  });
});

describe('putChannel (SPEC-UPDATE-002 M7)', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
    putMock.mockReset();
  });

  it('PUT /system/update/channel 호출 시 본문을 그대로 전송하고 응답을 반환한다', async () => {
    const payload: ChangeChannelResponse = {
      previous: 'stable',
      current: 'beta',
      check_result: {
        current: 'v0.3.0',
        latest: 'v0.4.0-beta.1',
        available: true,
        channel: 'beta',
        release_url: 'https://github.com/xtra72/xflow/releases/tag/v0.4.0-beta.1',
        published_at: '2026-05-05T08:00:00Z',
      },
      message: 'yaml 영구 저장은 CLI 사용',
    };
    putMock.mockResolvedValueOnce(payload);

    const result = await putChannel({ channel: 'beta' });

    expect(putMock).toHaveBeenCalledWith('/system/update/channel', {
      channel: 'beta',
    });
    expect(result).toEqual(payload);
  });

  it('400 (잘못된 채널) 은 APIError 로 전파된다', async () => {
    const err = new APIError(
      'CHANNEL_INVALID',
      'invalid channel',
      400,
    );
    putMock.mockRejectedValueOnce(err);

    await expect(putChannel({ channel: 'beta' })).rejects.toMatchObject({
      status: 400,
      code: 'CHANNEL_INVALID',
    });
  });
});

describe('useChannelInfo (SPEC-UPDATE-002 M6)', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
    putMock.mockReset();
  });

  it('GET /system/update/channel 결과를 data 로 노출한다', async () => {
    const payload: ChannelInfo = {
      current: 'stable',
      available: ['stable', 'beta', 'nightly'],
    };
    getMock.mockResolvedValue(payload);

    const { Wrapper } = buildWrapper();
    const { result } = renderHook(() => useChannelInfo(), { wrapper: Wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(getMock).toHaveBeenCalledWith('/system/update/channel');
    expect(result.current.data).toEqual(payload);
  });
});

describe('useChangeChannel (SPEC-UPDATE-002 M7)', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
    putMock.mockReset();
  });

  it('mutateAsync 시 PUT 을 트리거하고 응답을 반환한다', async () => {
    const payload: ChangeChannelResponse = {
      previous: 'stable',
      current: 'beta',
      check_result: null,
      message: 'yaml 영구 저장은 CLI 사용',
    };
    putMock.mockResolvedValueOnce(payload);

    const { Wrapper } = buildWrapper();
    const { result } = renderHook(() => useChangeChannel(), {
      wrapper: Wrapper,
    });

    const data = await result.current.mutateAsync({ channel: 'beta' });

    expect(putMock).toHaveBeenCalledWith('/system/update/channel', {
      channel: 'beta',
    });
    expect(data).toEqual(payload);
  });

  it('성공 시 system/version 과 system/update/channel 쿼리를 무효화한다', async () => {
    const payload: ChangeChannelResponse = {
      previous: 'stable',
      current: 'beta',
      check_result: null,
    };
    putMock.mockResolvedValueOnce(payload);

    const { Wrapper, queryClient } = buildWrapper();
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

    const { result } = renderHook(() => useChangeChannel(), {
      wrapper: Wrapper,
    });

    await result.current.mutateAsync({ channel: 'beta' });

    await waitFor(() => {
      // version 과 channel 두 query 모두 무효화 되어야 한다.
      const calls = invalidateSpy.mock.calls.map((c) => c[0]);
      const hasVersion = calls.some(
        (arg) =>
          Array.isArray((arg as { queryKey?: unknown[] }).queryKey) &&
          ((arg as { queryKey: unknown[] }).queryKey).includes('version'),
      );
      const hasChannel = calls.some(
        (arg) =>
          Array.isArray((arg as { queryKey?: unknown[] }).queryKey) &&
          ((arg as { queryKey: unknown[] }).queryKey).includes('channel'),
      );
      expect(hasVersion).toBe(true);
      expect(hasChannel).toBe(true);
    });
  });
});

describe('useSystemVersion', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
  });

  it('성공 응답을 data 로 노출한다', async () => {
    const payload: VersionInfo = {
      version: 'v0.3.0',
      commit: '6b9c531',
      build_date: '2026-05-04T12:34:56Z',
      go_version: 'go1.22.5',
      channel: 'stable',
      update_available: true,
      latest_version: 'v0.4.0',
    };
    getMock.mockResolvedValue(payload);

    const { Wrapper } = buildWrapper();
    const { result } = renderHook(() => useSystemVersion(), { wrapper: Wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual(payload);
  });

  it('refetchInterval 이 60_000ms 로 설정되어 있다', async () => {
    // 60s 간격을 직접 검증하기 어렵기 때문에, 옵션을 외부로 노출하기보다
    // 호출 빈도를 통해 간접 검증. fake timers 로 60s 경과 시 한 번 더 호출되는지 확인.
    vi.useFakeTimers();
    const payload: VersionInfo = {
      version: 'v0.3.0',
      commit: 'abc1234',
      build_date: '2026-05-04T00:00:00Z',
      go_version: 'go1.22.5',
      channel: 'stable',
      update_available: false,
      latest_version: null,
    };
    getMock.mockResolvedValue(payload);

    const { Wrapper } = buildWrapper();
    renderHook(() => useSystemVersion(), { wrapper: Wrapper });

    // 첫 호출은 마운트 직후 즉시.
    await vi.waitFor(() => expect(getMock).toHaveBeenCalledTimes(1));

    // 60s 미만에는 추가 호출 없음.
    await vi.advanceTimersByTimeAsync(30_000);
    expect(getMock).toHaveBeenCalledTimes(1);

    // 60s 경과 → 두 번째 호출.
    await vi.advanceTimersByTimeAsync(31_000);
    await vi.waitFor(() => expect(getMock).toHaveBeenCalledTimes(2));

    vi.useRealTimers();
  });
});

describe('useUpdateCheck', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
  });

  it('mutateAsync 호출 시 POST /system/update/check 를 트리거한다', async () => {
    const payload: CheckResult = {
      current: 'v0.3.0',
      latest: 'v0.4.0',
      available: true,
      channel: 'stable',
      release_url: 'https://x',
      published_at: '2026-05-05T08:00:00Z',
    };
    postMock.mockResolvedValueOnce(payload);

    const { Wrapper } = buildWrapper();
    const { result } = renderHook(() => useUpdateCheck(), { wrapper: Wrapper });

    const data = await result.current.mutateAsync();

    expect(postMock).toHaveBeenCalledWith('/system/update/check');
    expect(data).toEqual(payload);
  });
});

describe('useUpdateApply', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
  });

  it('변수로 받은 ApplyRequest 를 그대로 백엔드로 전달한다', async () => {
    const payload: ApplyResponse = {
      operation_id: 'upd-007',
      status: 'starting',
      from_version: 'v0.3.0',
      to_version: 'v0.4.0',
    };
    postMock.mockResolvedValueOnce(payload);

    const { Wrapper } = buildWrapper();
    const { result } = renderHook(() => useUpdateApply(), { wrapper: Wrapper });

    const data = await result.current.mutateAsync({ version: 'v0.4.0', force: false });

    expect(postMock).toHaveBeenCalledWith('/system/update/apply', {
      version: 'v0.4.0',
      force: false,
    });
    expect(data).toEqual(payload);
  });
});

describe('useUpdateRollback', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
  });

  it('mutateAsync 시 POST /system/update/rollback 을 트리거한다', async () => {
    const payload: RollbackResponse = {
      from_version: 'v0.4.0',
      to_version: 'v0.3.0',
      rolled_back_at: '2026-05-05T11:00:00Z',
    };
    postMock.mockResolvedValueOnce(payload);

    const { Wrapper } = buildWrapper();
    const { result } = renderHook(() => useUpdateRollback(), { wrapper: Wrapper });

    const data = await result.current.mutateAsync();

    expect(postMock).toHaveBeenCalledWith('/system/update/rollback');
    expect(data).toEqual(payload);
  });
});

describe('useUpdateStatus', () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
  });

  it('enabled=false 이면 호출되지 않는다', () => {
    const { Wrapper } = buildWrapper();
    renderHook(() => useUpdateStatus({ enabled: false }), { wrapper: Wrapper });

    expect(getMock).not.toHaveBeenCalled();
  });

  it('enabled=true 이면 GET /system/update/status 를 호출한다', async () => {
    const payload: UpdateOperation = {
      operation_id: 'upd-999',
      status: 'downloading',
      from_version: 'v0.3.0',
      to_version: 'v0.4.0',
      started_at: '2026-05-05T10:00:00Z',
      completed_at: null,
      error: null,
    };
    getMock.mockResolvedValue(payload);

    const { Wrapper } = buildWrapper();
    const { result } = renderHook(
      () => useUpdateStatus({ enabled: true }),
      { wrapper: Wrapper },
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(getMock).toHaveBeenCalledWith('/system/update/status');
    expect(result.current.data?.status).toBe('downloading');
  });

  it('refetchInterval 이 1_000ms 로 설정되어 있다 (활성 작업 모니터링)', async () => {
    vi.useFakeTimers();
    const payload: UpdateOperation = {
      operation_id: 'upd-000',
      status: 'downloading',
      from_version: 'v0.3.0',
      to_version: 'v0.4.0',
      started_at: '2026-05-05T10:00:00Z',
      completed_at: null,
      error: null,
    };
    getMock.mockResolvedValue(payload);

    const { Wrapper } = buildWrapper();
    renderHook(() => useUpdateStatus({ enabled: true }), { wrapper: Wrapper });

    await vi.waitFor(() => expect(getMock).toHaveBeenCalledTimes(1));

    await vi.advanceTimersByTimeAsync(1_100);
    await vi.waitFor(() => expect(getMock).toHaveBeenCalledTimes(2));

    vi.useRealTimers();
  });
});
