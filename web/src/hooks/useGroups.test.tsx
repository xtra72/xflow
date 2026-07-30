// useGroups 훅 테스트 (SPEC-XSFM-GROUP-001 Module 6, M6).
//
// agentService.execAgent 를 mock 하여 그룹 CRUD 명령이 표준 exec 계약(command + params)으로
// 전송되는지, list_groups 응답이 정규화되는지 검증한다. AC 3.1~3.4(명령 계약) + 6.2 를 뒷받침한다.

import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const execAgentMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/agentService', () => ({
  execAgent: execAgentMock,
}));

import {
  isCustomGroup,
  useAddGroup,
  useGroups,
  useRemoveGroup,
  useSetGroup,
  type Group,
} from './useGroups';

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

describe('useGroups', () => {
  it('list_groups 응답을 그룹 배열로 반환하고 members/member_count 를 정규화한다', async () => {
    execAgentMock.mockResolvedValueOnce({
      status: 'ok',
      groups: [
        { id: 'station:ST-1', name: '강남역', type: 'station', member_count: 2, members: ['d1', 'd2'] },
        // member_count 누락 → members 길이로 폴백. members 누락 → [] 방어.
        { id: 'custom:floor2', name: '2층', type: 'custom', members: ['d3'] },
        { id: 'line:L1', name: 'L1', type: 'line', member_count: 0 },
      ],
    });

    const { result } = renderHook(() => useGroups('agent-1'), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(execAgentMock).toHaveBeenCalledWith('agent-1', { command: 'list_groups' });
    const groups = result.current.data as Group[];
    expect(groups).toHaveLength(3);
    expect(groups[1]).toMatchObject({ id: 'custom:floor2', member_count: 1, members: ['d3'] });
    expect(groups[2]).toMatchObject({ id: 'line:L1', member_count: 0, members: [] });
  });

  it('빈 응답이면 빈 배열을 반환한다', async () => {
    execAgentMock.mockResolvedValueOnce({ status: 'ok' });
    const { result } = renderHook(() => useGroups('agent-1'), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it('useAddGroup 은 add_group 을 { name, members } params 로 전송하고 group_id 를 반환한다', async () => {
    execAgentMock.mockResolvedValueOnce({ status: 'ok', group_id: 'custom:2층' });
    const { result } = renderHook(() => useAddGroup('agent-1'), { wrapper });
    result.current.mutate({ name: '2층', members: ['d1'] });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'add_group',
      params: { name: '2층', members: ['d1'] },
    });
    expect(result.current.data).toEqual({ status: 'ok', group_id: 'custom:2층' });
  });

  it('useSetGroup 은 set_group 을 부분 갱신 params 로 전송한다(생략 필드 미포함)', async () => {
    execAgentMock.mockResolvedValueOnce({ status: 'ok', group_id: 'custom:x' });
    const { result } = renderHook(() => useSetGroup('agent-1'), { wrapper });
    // name 생략, members 만 갱신 → params 에 name 키가 없어야 한다(백엔드가 보존).
    result.current.mutate({ group_id: 'custom:x', members: ['d1', 'd2'] });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'set_group',
      params: { group_id: 'custom:x', members: ['d1', 'd2'] },
    });
    const [, req] = execAgentMock.mock.calls[0]!;
    expect('name' in (req as { params: object }).params).toBe(false);
  });

  it('useRemoveGroup 은 remove_group 을 { group_id } params 로 전송한다', async () => {
    execAgentMock.mockResolvedValueOnce({ status: 'ok', group_id: 'custom:x' });
    const { result } = renderHook(() => useRemoveGroup('agent-1'), { wrapper });
    result.current.mutate('custom:x');
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(execAgentMock).toHaveBeenCalledWith('agent-1', {
      command: 'remove_group',
      params: { group_id: 'custom:x' },
    });
  });
});

describe('isCustomGroup', () => {
  it('custom 타입만 편집 가능(true), station/line 은 읽기 전용(false)', () => {
    expect(isCustomGroup({ type: 'custom' })).toBe(true);
    expect(isCustomGroup({ type: 'station' })).toBe(false);
    expect(isCustomGroup({ type: 'line' })).toBe(false);
  });
});
