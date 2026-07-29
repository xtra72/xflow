// useStation 훅 테스트 (SPEC-AIRPURIFIER-001 Wave 2).
//
// agentService.execAgent 를 mock 하여 airpurifier 명령이 표준 exec 계약대로 인자를
// `params` 아래에 중첩해 전송하는지, list_* 응답이 올바르게 파싱되는지 검증한다
// (samsung/modbus 와 동일 계약; 백엔드가 params 로부터 내부 필드를 backfill).

import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const execAgentMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/agentService', () => ({
  execAgent: execAgentMock,
}));

import {
  useAddAirpurifierDevice,
  useAddPlace,
  useAddStation,
  useAirpurifierDevices,
  useRemovePlace,
  useSetAirpurifierDevice,
  useStations,
} from './useStation';

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

describe('useStations', () => {
  it('list_stations 응답의 stations 를 파싱하고 places 누락을 방어한다', async () => {
    execAgentMock.mockResolvedValueOnce({
      status: 'ok',
      stations: [
        { station: 'ST-1', line: '2호선', display_name: '시청', order: 1, places: [{ place: 'p1', display_name: '승강장', order: 1 }] },
        { station: 'ST-2', line: '2호선', display_name: '을지로', order: 2 },
      ],
    });

    const { result } = renderHook(() => useStations('agent-1'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', { command: 'list_stations' });
    expect(result.current.data).toHaveLength(2);
    expect(result.current.data?.[0]?.places).toHaveLength(1);
    expect(result.current.data?.[1]?.places).toEqual([]);
  });
});

describe('useAddStation', () => {
  it('add_station 인자를 params 아래에 중첩해 전송한다', async () => {
    execAgentMock.mockResolvedValueOnce({ status: 'ok' });

    const { result } = renderHook(() => useAddStation('agent-1'), { wrapper });
    result.current.mutate({ station: 'ST-1', line: '2호선', display_name: '시청', order: 1 });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'add_station',
      params: { station: 'ST-1', line: '2호선', display_name: '시청', order: 1 },
    });
  });
});

describe('useAddPlace / useRemovePlace', () => {
  it('add_place 인자를 params 아래에 중첩해 전송한다', async () => {
    execAgentMock.mockResolvedValueOnce({ status: 'ok' });

    const { result } = renderHook(() => useAddPlace('agent-1'), { wrapper });
    result.current.mutate({ station: 'ST-1', place: 'p1', display_name: '승강장', order: 2 });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'add_place',
      params: { station: 'ST-1', place: 'p1', display_name: '승강장', order: 2 },
    });
  });

  it('remove_place 인자를 params 아래에 중첩해 전송한다', async () => {
    execAgentMock.mockResolvedValueOnce({ status: 'ok' });

    const { result } = renderHook(() => useRemovePlace('agent-1'), { wrapper });
    result.current.mutate({ station: 'ST-1', place: 'p1' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'remove_place',
      params: { station: 'ST-1', place: 'p1' },
    });
  });
});

describe('airpurifier devices', () => {
  it('list_devices 응답의 devices 를 파싱한다', async () => {
    execAgentMock.mockResolvedValueOnce({
      status: 'ok',
      devices: [
        { device_id: 'ap-101', name: '대합실', group_id: '', station: 'ST-1', place: 'p1', index: 1, online: true, power: false, fan_speed: 0, source: 'bridge' },
      ],
    });

    const { result } = renderHook(() => useAirpurifierDevices('agent-1'), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', { command: 'list_devices' });
    expect(result.current.data?.[0]?.device_id).toBe('ap-101');
  });

  it('add_device 인자를 params 아래에 중첩해 전송한다', async () => {
    execAgentMock.mockResolvedValueOnce({ status: 'ok' });

    const { result } = renderHook(() => useAddAirpurifierDevice('agent-1'), { wrapper });
    result.current.mutate({ device_id: 'ap-101', name: '대합실', station: 'ST-1', place: 'p1', index: 3, group_id: 'g1' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'add_device',
      params: { device_id: 'ap-101', name: '대합실', station: 'ST-1', place: 'p1', index: 3, group_id: 'g1' },
    });
  });

  it('set_device 인자를 params 아래에 중첩해 전송한다', async () => {
    execAgentMock.mockResolvedValueOnce({ status: 'ok' });

    const { result } = renderHook(() => useSetAirpurifierDevice('agent-1'), { wrapper });
    result.current.mutate({ device_id: 'ap-101', name: '변경' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'set_device',
      params: { device_id: 'ap-101', name: '변경' },
    });
  });
});
