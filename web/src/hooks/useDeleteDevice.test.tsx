// useDeleteDevice 훅 테스트.
//
// agentService.execAgent 와 deviceService.deleteMetadata 를 mock 하여
// remove_device → deleteMetadata 오케스트레이션을 검증한다:
//   - remove_device 를 { command:'remove_device', params:{ device_id } } 로 호출
//   - 성공 후 동일 UUID 로 deleteMetadata 호출
//   - deleteMetadata 실패(404 등)는 삼켜서 mutation 은 성공으로 처리
//   - deviceId 없이 address 만 있으면 remove_device 의 address fallback 사용

import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const execAgentMock = vi.hoisted(() => vi.fn());
const deleteMetadataMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/agentService', () => ({
  execAgent: execAgentMock,
}));

vi.mock('@/services/api/deviceService', () => ({
  deleteMetadata: deleteMetadataMock,
}));

// useDevice 모듈은 WebSocket 훅/핸들러를 import 하므로 부작용 없는 스텁으로 대체한다.
vi.mock('@/hooks/useWebSocket', () => ({
  useWebSocket: () => ({ client: null }),
}));
vi.mock('@/services/ws/wsHandlers', () => ({
  onDeviceStatus: () => () => {},
}));

import { useDeleteDevice } from './useDevice';

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  execAgentMock.mockReset();
  deleteMetadataMock.mockReset();
});

describe('useDeleteDevice', () => {
  it('remove_device 를 device_id 파라미터로 호출하고 성공 후 deleteMetadata 를 호출한다', async () => {
    execAgentMock.mockResolvedValueOnce({ result: {} });
    deleteMetadataMock.mockResolvedValueOnce(undefined);

    const { result } = renderHook(() => useDeleteDevice(), { wrapper });

    result.current.mutate({ agentId: 'agent-1', deviceId: 'uuid-42' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'remove_device',
      params: { device_id: 'uuid-42' },
    });
    expect(deleteMetadataMock).toHaveBeenCalledWith('uuid-42');
  });

  it('deleteMetadata 실패(404 등)는 삼켜서 mutation 은 성공으로 처리한다', async () => {
    execAgentMock.mockResolvedValueOnce({ result: {} });
    deleteMetadataMock.mockRejectedValueOnce(new Error('not found'));

    const { result } = renderHook(() => useDeleteDevice(), { wrapper });

    result.current.mutate({ agentId: 'agent-1', deviceId: 'uuid-42' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.isError).toBe(false);
  });

  it('remove_device 실패는 mutation 을 reject 하고 deleteMetadata 를 호출하지 않는다', async () => {
    execAgentMock.mockRejectedValueOnce(new Error('unsupported'));

    const { result } = renderHook(() => useDeleteDevice(), { wrapper });

    result.current.mutate({ agentId: 'agent-1', deviceId: 'uuid-42' });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(deleteMetadataMock).not.toHaveBeenCalled();
  });

  it('deviceId 가 없으면 address fallback 으로 remove_device 를 호출하고 deleteMetadata 는 건너뛴다', async () => {
    execAgentMock.mockResolvedValueOnce({ result: {} });

    const { result } = renderHook(() => useDeleteDevice(), { wrapper });

    result.current.mutate({ agentId: 'agent-1', address: '20.00.01' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'remove_device',
      params: { address: '20.00.01' },
    });
    expect(deleteMetadataMock).not.toHaveBeenCalled();
  });
});
