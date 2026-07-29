// useXsfmControl 훅 테스트 (SPEC-FACILITY-DASHBOARD-001 B1).
//
// agentService.execAgent 를 mock 하여 제어 명령이 셀렉터(하나) + 값을 params 로 전송하는지,
// fan-out 응답이 타입가드로 판별되는지 검증한다.

import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const execAgentMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/agentService', () => ({
  execAgent: execAgentMock,
}));

import {
  isFanOutResponse,
  useXsfmControl,
  type ControlResponse,
} from './useXsfmControl';

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

describe('useXsfmControl', () => {
  it('set_power 를 station 셀렉터 + power 로 params 에 담아 전송한다', async () => {
    execAgentMock.mockResolvedValueOnce({
      selector: { type: 'station', value: 'ST-1' },
      results: [{ device_id: 'd1', status: 'ok' }],
      status: 'ok',
    });

    const { result } = renderHook(() => useXsfmControl('agent-1'), { wrapper });
    result.current.setPower.mutate({ station: 'ST-1', power: true });

    await waitFor(() => expect(result.current.setPower.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'set_power',
      params: { station: 'ST-1', power: true },
    });
  });

  it('set_fan_speed 를 line 셀렉터 + fan_speed 로 전송한다', async () => {
    execAgentMock.mockResolvedValueOnce({
      selector: { type: 'line', value: '2호선' },
      results: [{ device_id: 'd1', status: 'ok' }],
      status: 'ok',
    });

    const { result } = renderHook(() => useXsfmControl('agent-1'), { wrapper });
    result.current.setFanSpeed.mutate({ line: '2호선', fan_speed: 2 });

    await waitFor(() => expect(result.current.setFanSpeed.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'set_fan_speed',
      params: { line: '2호선', fan_speed: 2 },
    });
  });

  it('set_multiple 를 device_id 단일 대상 + power/fan_speed 로 전송한다', async () => {
    execAgentMock.mockResolvedValueOnce({ device_id: 'ap-1', status: 'ok' });

    const { result } = renderHook(() => useXsfmControl('agent-1'), { wrapper });
    result.current.setMultiple.mutate({ device_id: 'ap-1', power: true, fan_speed: 3 });

    await waitFor(() => expect(result.current.setMultiple.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'set_multiple',
      params: { device_id: 'ap-1', power: true, fan_speed: 3 },
    });
  });

  it('fan-out 응답을 타입화해 멤버별 status 를 반환한다(partial)', async () => {
    execAgentMock.mockResolvedValueOnce({
      selector: { type: 'line', value: '2호선' },
      results: [
        { device_id: 'd1', status: 'ok' },
        { device_id: 'd2', status: 'timeout', error: 'ErrControlTimeout' },
      ],
      excluded: ['d9'],
      status: 'partial',
    });

    const { result } = renderHook(() => useXsfmControl('agent-1'), { wrapper });
    result.current.setPower.mutate({ line: '2호선', power: false });

    await waitFor(() => expect(result.current.setPower.isSuccess).toBe(true));
    const res = result.current.setPower.data as ControlResponse;
    expect(isFanOutResponse(res)).toBe(true);
    if (isFanOutResponse(res)) {
      expect(res.status).toBe('partial');
      expect(res.results).toHaveLength(2);
      expect(res.results[1]?.status).toBe('timeout');
      expect(res.excluded).toEqual(['d9']);
    }
  });
});

describe('isFanOutResponse', () => {
  it('단일 device_id 응답은 fan-out 이 아니다', () => {
    const single: ControlResponse = { device_id: 'ap-1', status: 'ok' };
    expect(isFanOutResponse(single)).toBe(false);
  });
});
