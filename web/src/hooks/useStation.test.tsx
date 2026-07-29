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
  EMPTY_REQUIRED,
  parseDelimitedRows,
  useAddAirpurifierDevice,
  useAddPlace,
  useAddStation,
  useAirpurifierDevices,
  useBulkAddPlaces,
  useBulkAddPlacesTop,
  useBulkAddStations,
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

  it('add_device 는 device_id/name 없이 station/place/index/group_id 만 params 로 보내고 응답을 반환한다', async () => {
    execAgentMock.mockResolvedValueOnce({ status: 'ok', device_id: 'uuid-1', name: 'st01:PL-A:003', source: 'bridge' });

    const { result } = renderHook(() => useAddAirpurifierDevice('agent-1'), { wrapper });
    result.current.mutate({ station: 'st01', place: 'PL-A', index: 3, group_id: 'g1' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'add_device',
      params: { station: 'st01', place: 'PL-A', index: 3, group_id: 'g1' },
    });
    // 백엔드가 생성/계산한 device_id + name 이 결과로 반환된다(성공 토스트에 사용).
    expect(result.current.data).toEqual({
      status: 'ok',
      device_id: 'uuid-1',
      name: 'st01:PL-A:003',
      source: 'bridge',
    });
  });

  it('set_device 는 device_id(UUID)로 대상 지정 + 편집 필드를 params 로 보낸다', async () => {
    execAgentMock.mockResolvedValueOnce({ status: 'ok' });

    const { result } = renderHook(() => useSetAirpurifierDevice('agent-1'), { wrapper });
    result.current.mutate({ device_id: 'uuid-1', station: 'st02', place: 'PL-B', index: 5, group_id: 'g2' });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'set_device',
      params: { device_id: 'uuid-1', station: 'st02', place: 'PL-B', index: 5, group_id: 'g2' },
    });
  });
});

describe('parseDelimitedRows', () => {
  it('개행 분리 + 빈 줄 스킵 + 셀 trim, 원본 줄번호 보존', () => {
    const rows = parseDelimitedRows('ST-1, line-2 , 강남 , 5\n\n   \nST-2,line-2,을지로,2');
    expect(rows).toHaveLength(2);
    expect(rows[0]).toEqual({ line: 1, cells: ['ST-1', 'line-2', '강남', '5'], raw: 'ST-1, line-2 , 강남 , 5' });
    // 빈 줄(2,3)을 건너뛰어 다음 데이터 행의 원본 줄번호는 4.
    expect(rows[1]?.line).toBe(4);
    expect(rows[1]?.cells).toEqual(['ST-2', 'line-2', '을지로', '2']);
  });

  it('TAB 이 있으면 tab-split, 없으면 comma-split (줄마다 자동 감지) + \\r 제거', () => {
    const rows = parseDelimitedRows('ST-1\tline-2\t강남\t5\r\nST-2,line-2,을지로,2');
    expect(rows[0]?.cells).toEqual(['ST-1', 'line-2', '강남', '5']);
    expect(rows[1]?.cells).toEqual(['ST-2', 'line-2', '을지로', '2']);
  });
});

describe('useBulkAddStations', () => {
  it('행마다 add_station 을 params 로 호출하고 order 를 정수 파싱한다 (빈 값→0)', async () => {
    execAgentMock.mockResolvedValue({ status: 'ok' });

    const { result } = renderHook(() => useBulkAddStations('agent-1'), { wrapper });
    const res = await result.current.mutateAsync('ST-1,line-2,강남,5\nST-2');

    expect(res).toEqual({ total: 2, ok: 2, failed: [] });
    expect(execAgentMock).toHaveBeenNthCalledWith(1, 'agent-1', {
      command: 'add_station',
      params: { station: 'ST-1', line: 'line-2', display_name: '강남', order: 5 },
    });
    // ST-2: 나머지 컬럼 없음 → 빈 문자열/0.
    expect(execAgentMock).toHaveBeenNthCalledWith(2, 'agent-1', {
      command: 'add_station',
      params: { station: 'ST-2', line: '', display_name: '', order: 0 },
    });
  });

  it('best-effort: 빈 station 행은 호출 없이 실패로 집계, 개별 오류는 계속 진행', async () => {
    // 행1 성공, 행2(빈 station) 스킵-실패, 행3 백엔드 오류.
    execAgentMock
      .mockResolvedValueOnce({ status: 'ok' })
      .mockRejectedValueOnce(new Error('duplicate'));

    const { result } = renderHook(() => useBulkAddStations('agent-1'), { wrapper });
    const res = await result.current.mutateAsync('ST-1,line-2\n,line-2,이름\nST-3');

    expect(res.total).toBe(3);
    expect(res.ok).toBe(1);
    expect(res.failed).toHaveLength(2);
    // 빈 station 행(원본 2행)은 sentinel 사유, execAgent 미호출.
    expect(res.failed[0]).toEqual({ line: 2, input: ',line-2,이름', reason: EMPTY_REQUIRED });
    // 백엔드 오류(원본 3행)는 메시지 원문.
    expect(res.failed[1]).toEqual({ line: 3, input: 'ST-3', reason: 'duplicate' });
    // execAgent 는 유효 행(ST-1, ST-3) 2회만 호출.
    expect(execAgentMock).toHaveBeenCalledTimes(2);
  });
});

describe('useBulkAddPlaces', () => {
  it('행마다 add_place 를 station+params 로 호출한다', async () => {
    execAgentMock.mockResolvedValue({ status: 'ok' });

    const { result } = renderHook(() => useBulkAddPlaces('agent-1'), { wrapper });
    const res = await result.current.mutateAsync({ station: 'ST-1', text: 'PL-A,승강장 A,1\nPL-B' });

    expect(res).toEqual({ total: 2, ok: 2, failed: [] });
    expect(execAgentMock).toHaveBeenNthCalledWith(1, 'agent-1', {
      command: 'add_place',
      params: { station: 'ST-1', place: 'PL-A', display_name: '승강장 A', order: 1 },
    });
    expect(execAgentMock).toHaveBeenNthCalledWith(2, 'agent-1', {
      command: 'add_place',
      params: { station: 'ST-1', place: 'PL-B', display_name: '', order: 0 },
    });
  });

  it('빈 place 행은 실패로 집계하고 execAgent 를 호출하지 않는다', async () => {
    const { result } = renderHook(() => useBulkAddPlaces('agent-1'), { wrapper });
    const res = await result.current.mutateAsync({ station: 'ST-1', text: ',이름,1' });

    expect(res).toEqual({ total: 1, ok: 0, failed: [{ line: 1, input: ',이름,1', reason: EMPTY_REQUIRED }] });
    expect(execAgentMock).not.toHaveBeenCalled();
  });
});

describe('useBulkAddPlacesTop', () => {
  it('행마다 station+place 를 포함해 add_place 를 호출한다 (station,place,display_name,order)', async () => {
    execAgentMock.mockResolvedValue({ status: 'ok' });

    const { result } = renderHook(() => useBulkAddPlacesTop('agent-1'), { wrapper });
    const res = await result.current.mutateAsync('st01,PL-A,승강장 A,1\nst02,PL-B');

    expect(res).toEqual({ total: 2, ok: 2, failed: [] });
    expect(execAgentMock).toHaveBeenNthCalledWith(1, 'agent-1', {
      command: 'add_place',
      params: { station: 'st01', place: 'PL-A', display_name: '승강장 A', order: 1 },
    });
    // 나머지 컬럼 없음 → 빈 문자열/0.
    expect(execAgentMock).toHaveBeenNthCalledWith(2, 'agent-1', {
      command: 'add_place',
      params: { station: 'st02', place: 'PL-B', display_name: '', order: 0 },
    });
  });

  it('station 또는 place 가 비면 EMPTY_REQUIRED 로 실패, best-effort 로 계속 진행', async () => {
    // 행1 성공, 행2(빈 station) 스킵, 행3(빈 place) 스킵, 행4 백엔드 오류.
    execAgentMock
      .mockResolvedValueOnce({ status: 'ok' })
      .mockRejectedValueOnce(new Error('station not found'));

    const { result } = renderHook(() => useBulkAddPlacesTop('agent-1'), { wrapper });
    const res = await result.current.mutateAsync('st01,PL-A\n,PL-X\nst02,\nst03,PL-Z');

    expect(res.total).toBe(4);
    expect(res.ok).toBe(1);
    expect(res.failed).toEqual([
      { line: 2, input: ',PL-X', reason: EMPTY_REQUIRED },
      { line: 3, input: 'st02,', reason: EMPTY_REQUIRED },
      { line: 4, input: 'st03,PL-Z', reason: 'station not found' },
    ]);
    // 유효 행(st01, st03) 2회만 호출.
    expect(execAgentMock).toHaveBeenCalledTimes(2);
  });
});
