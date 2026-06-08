// 상세 타깃 훅 단위 테스트 (SPEC-REMOTE-001 M8, REQ-J09/J11).
//
// 검증:
//   - 로컬 타깃: 로컬 상세 훅 위임(원격 서비스 미호출).
//   - 원격 타깃: READ 프록시 GET 호출(정적), SSE 스트림(라이브) + 폴백.
//   - 라이브: 스트림 프레임 우선, shouldFallback 시 폴링 결과.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// ---- 의존 mock ----
const useLocalAgentMock = vi.hoisted(() => vi.fn());
const useLocalAgentStatsMock = vi.hoisted(() => vi.fn());
const useLocalDeviceRealtimeMock = vi.hoisted(() => vi.fn());
const useLocalFlowStatusMock = vi.hoisted(() => vi.fn());
const useLocalFlowNodesMock = vi.hoisted(() => vi.fn());
const useRemoteStreamMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useAgent', () => ({
  useAgent: useLocalAgentMock,
  useAgentStats: useLocalAgentStatsMock,
}));
vi.mock('@/hooks/useDevice', () => ({
  useDeviceRealtime: useLocalDeviceRealtimeMock,
}));
vi.mock('@/hooks/useFlow', () => ({
  useFlowStatus: useLocalFlowStatusMock,
  useFlowNodes: useLocalFlowNodesMock,
}));
vi.mock('@/hooks/useRemoteStream', () => ({
  useRemoteStream: useRemoteStreamMock,
}));

const getRemoteFlowStatusMock = vi.hoisted(() => vi.fn());
const getRemoteFlowNodesMock = vi.hoisted(() => vi.fn());
const getRemoteAgentMock = vi.hoisted(() => vi.fn());
const getRemoteAgentStatsMock = vi.hoisted(() => vi.fn());
const getRemoteDeviceMock = vi.hoisted(() => vi.fn());
const getRemoteDeviceStateMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/remoteService', () => ({
  getRemoteFlowStatus: getRemoteFlowStatusMock,
  getRemoteFlowNodes: getRemoteFlowNodesMock,
  getRemoteAgent: getRemoteAgentMock,
  getRemoteAgentStats: getRemoteAgentStatsMock,
  getRemoteDevice: getRemoteDeviceMock,
  getRemoteDeviceState: getRemoteDeviceStateMock,
  remoteStreamUrl: (instanceID: string, kind: { domain: string; action: string }, id: string) =>
    `/sse/${instanceID}/${kind.domain}/${id}/${kind.action}`,
}));

import {
  useAgentDetailTarget,
  useAgentStatsTarget,
  useDeviceDetailTarget,
  useFlowStatusTarget,
} from './useDetailTargets';

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

const localIdle = { data: undefined, isLoading: false, error: null };

beforeEach(() => {
  useLocalAgentMock.mockReset().mockReturnValue(localIdle);
  useLocalAgentStatsMock.mockReset().mockReturnValue(localIdle);
  useLocalDeviceRealtimeMock.mockReset().mockReturnValue(localIdle);
  useLocalFlowStatusMock.mockReset().mockReturnValue(localIdle);
  useLocalFlowNodesMock.mockReset().mockReturnValue(localIdle);
  useRemoteStreamMock.mockReset().mockReturnValue({
    data: undefined,
    status: 'idle',
    shouldFallback: false,
  });
  [
    getRemoteFlowStatusMock,
    getRemoteFlowNodesMock,
    getRemoteAgentMock,
    getRemoteAgentStatsMock,
    getRemoteDeviceMock,
    getRemoteDeviceStateMock,
  ].forEach((m) => m.mockReset());
});

describe('useFlowStatusTarget', () => {
  it('로컬 타깃은 useFlowStatus 를 위임하고 원격 서비스를 호출하지 않는다', () => {
    useLocalFlowStatusMock.mockReturnValue({
      data: { status: 'running' },
      isLoading: false,
      error: null,
    });
    const { result } = renderHook(
      () => useFlowStatusTarget({ type: 'local' }, 'f1'),
      { wrapper },
    );
    expect(result.current.data).toEqual({ status: 'running' });
    expect(getRemoteFlowStatusMock).not.toHaveBeenCalled();
    // 로컬 훅은 실제 flowId 로 호출된다.
    expect(useLocalFlowStatusMock).toHaveBeenCalledWith('f1');
  });

  it('원격 타깃은 getRemoteFlowStatus 를 호출한다', async () => {
    getRemoteFlowStatusMock.mockResolvedValue({ status: 'stopped' });
    const { result } = renderHook(
      () => useFlowStatusTarget({ type: 'remote', instanceId: 'node-a' }, 'f1'),
      { wrapper },
    );
    await waitFor(() => expect(result.current.data).toEqual({ status: 'stopped' }));
    expect(getRemoteFlowStatusMock).toHaveBeenCalledWith('node-a', 'f1');
    // 로컬 훅은 빈 id 로 비활성 호출된다.
    expect(useLocalFlowStatusMock).toHaveBeenCalledWith('');
  });
});

describe('useAgentDetailTarget', () => {
  it('원격 타깃은 getRemoteAgent 를 호출한다', async () => {
    getRemoteAgentMock.mockResolvedValue({ id: 'a1', name: 'Remote', type: 'mqtt', status: 'running' });
    const { result } = renderHook(
      () => useAgentDetailTarget({ type: 'remote', instanceId: 'node-a' }, 'a1', 'full'),
      { wrapper },
    );
    await waitFor(() => expect(result.current.data?.name).toBe('Remote'));
    expect(getRemoteAgentMock).toHaveBeenCalledWith('node-a', 'a1');
  });
});

describe('useAgentStatsTarget — 스트림 우선, 폴백', () => {
  it('원격: 스트림 프레임을 우선 노출한다', () => {
    useRemoteStreamMock.mockReturnValue({
      data: { messages_in: 5, messages_out: 0, error_count: 0 },
      status: 'open',
      shouldFallback: false,
    });
    const { result } = renderHook(
      () => useAgentStatsTarget({ type: 'remote', instanceId: 'node-a' }, 'a1'),
      { wrapper },
    );
    expect(result.current.data).toEqual({ messages_in: 5, messages_out: 0, error_count: 0 });
    // 스트림이 살아있으면 폴링하지 않는다.
    expect(getRemoteAgentStatsMock).not.toHaveBeenCalled();
  });

  it('원격: shouldFallback 이면 폴링 결과를 노출한다', async () => {
    useRemoteStreamMock.mockReturnValue({
      data: undefined,
      status: 'closed',
      shouldFallback: true,
    });
    getRemoteAgentStatsMock.mockResolvedValue({ messages_in: 9, messages_out: 1, error_count: 0 });
    const { result } = renderHook(
      () => useAgentStatsTarget({ type: 'remote', instanceId: 'node-a' }, 'a1'),
      { wrapper },
    );
    await waitFor(() => expect(result.current.data?.messages_in).toBe(9));
    expect(getRemoteAgentStatsMock).toHaveBeenCalledWith('node-a', 'a1');
  });

  it('로컬: useAgentStats 를 위임한다', () => {
    useLocalAgentStatsMock.mockReturnValue({
      data: { messages_in: 1, messages_out: 2, error_count: 0 },
      isLoading: false,
      error: null,
    });
    const { result } = renderHook(
      () => useAgentStatsTarget({ type: 'local' }, 'a1'),
      { wrapper },
    );
    expect(result.current.data?.messages_in).toBe(1);
    expect(getRemoteAgentStatsMock).not.toHaveBeenCalled();
  });
});

describe('useDeviceDetailTarget — get + state 병합', () => {
  it('원격: get(정적) + state(스트림) 을 병합해 properties 를 갱신한다', async () => {
    getRemoteDeviceMock.mockResolvedValue({
      id: 'd1',
      name: 'Dev',
      commands: [],
      state: { properties: { power: false } },
    });
    useRemoteStreamMock.mockReturnValue({
      data: { state: { properties: { power: true, temp: 22 } } },
      status: 'open',
      shouldFallback: false,
    });
    const { result } = renderHook(
      () => useDeviceDetailTarget({ type: 'remote', instanceId: 'node-a' }, 'd1'),
      { wrapper },
    );
    await waitFor(() => expect(result.current.data?.name).toBe('Dev'));
    expect(result.current.data?.state?.properties).toEqual({ power: true, temp: 22 });
    expect(getRemoteDeviceMock).toHaveBeenCalledWith('node-a', 'd1');
  });

  it('로컬: useDeviceRealtime 를 위임한다', () => {
    useLocalDeviceRealtimeMock.mockReturnValue({
      data: { id: 'd1', name: 'Local', state: { properties: {} } },
      isLoading: false,
      error: null,
    });
    const { result } = renderHook(
      () => useDeviceDetailTarget({ type: 'local' }, 'd1'),
      { wrapper },
    );
    expect(result.current.data?.name).toBe('Local');
    expect(getRemoteDeviceMock).not.toHaveBeenCalled();
  });
});
