// useSetDeviceReport 훅 테스트.
//
// agentService.execAgent 를 mock 하여 set_device 호출과 낙관적 캐시 업데이트를 검증한다:
//   - set_device 를 { command:'set_device', params:{ device_id, report_enabled } } 로 호출
//   - 낙관적 업데이트: ['devices'] 캐시의 해당 디바이스 report_enabled 즉시 반영
//   - 실패 시 낙관적 업데이트 롤백 후 mutation 은 error 로 처리

import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { DeviceInfo } from '@/types/device';

const execAgentMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/agentService', () => ({
  execAgent: execAgentMock,
}));

vi.mock('@/services/api/deviceService', () => ({}));

// useDevice 모듈은 WebSocket 훅/핸들러를 import 하므로 부작용 없는 스텁으로 대체한다.
vi.mock('@/hooks/useWebSocket', () => ({
  useWebSocket: () => ({ client: null }),
}));
vi.mock('@/services/ws/wsHandlers', () => ({
  onDeviceStatus: () => () => {},
}));

import { useSetDeviceReport } from './useDevice';

function makeDevice(overrides: Partial<DeviceInfo> = {}): DeviceInfo {
  return {
    id: 'uuid-9',
    uid: 'uuid-9',
    name: '거실 에어컨',
    type: 'indoor',
    protocol: 'lgap',
    agent_name: 'agent-a',
    source: 'config',
    online: true,
    last_seen: new Date().toISOString(),
    capabilities: [],
    report_enabled: true,
    ...overrides,
  };
}

let queryClient: QueryClient;

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  execAgentMock.mockReset();
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
});

describe('useSetDeviceReport', () => {
  it('set_device 를 device_id + report_enabled 파라미터로 호출한다', async () => {
    execAgentMock.mockResolvedValueOnce({ result: {} });

    const { result } = renderHook(() => useSetDeviceReport(), { wrapper });

    result.current.mutate({ agentId: 'agent-1', deviceId: 'uuid-9', reportEnabled: false });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'set_device',
      params: { device_id: 'uuid-9', report_enabled: false },
    });
  });

  it('낙관적으로 ["devices"] 캐시의 해당 디바이스 report_enabled 를 즉시 갱신한다', async () => {
    // 응답을 지연시켜 낙관적 업데이트 시점(onMutate)을 관찰한다.
    let resolveExec: (v: unknown) => void = () => {};
    execAgentMock.mockReturnValueOnce(
      new Promise((res) => {
        resolveExec = res;
      }),
    );

    queryClient.setQueryData(['devices', undefined], {
      data: [makeDevice({ report_enabled: true })],
    });

    const { result } = renderHook(() => useSetDeviceReport(), { wrapper });

    result.current.mutate({ agentId: 'agent-1', deviceId: 'uuid-9', reportEnabled: false });

    // onMutate 낙관적 패치가 적용될 때까지 대기.
    await waitFor(() => {
      const cached = queryClient.getQueryData<{ data: DeviceInfo[] }>(['devices', undefined]);
      expect(cached?.data?.[0]?.report_enabled).toBe(false);
    });

    resolveExec({ result: {} });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });

  it('실패 시 낙관적 업데이트를 롤백하고 mutation 은 error 로 처리한다', async () => {
    execAgentMock.mockRejectedValueOnce(new Error('unsupported'));

    queryClient.setQueryData(['devices', undefined], {
      data: [makeDevice({ report_enabled: true })],
    });

    const { result } = renderHook(() => useSetDeviceReport(), { wrapper });

    result.current.mutate({ agentId: 'agent-1', deviceId: 'uuid-9', reportEnabled: false });

    await waitFor(() => expect(result.current.isError).toBe(true));

    // 롤백으로 원래 값(true)이 복원된다.
    const cached = queryClient.getQueryData<{ data: DeviceInfo[] }>(['devices', undefined]);
    expect(cached?.data?.[0]?.report_enabled).toBe(true);
  });
});
