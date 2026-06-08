// 노드 디스플레이 해상도 오버라이드 뮤테이션 훅 테스트 (SPEC-REMOTE-001 M12).
//
// remoteService 를 mock 하여 set/clear 뮤테이션이 올바른 서비스 함수를 호출하고,
// 성공 시 노드-상세(['remote','nodes',id]) + 노드 목록(['remote','nodes']) 쿼리를
// 무효화하는지 검증한다. 상세 prefix 무효화는 고정 캔버스(DashboardCanvas)가 같은
// 캐시를 공유하므로 새 EFFECTIVE 해상도로 재렌더되게 한다(캔버스 코드 변경 없음).

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { createElement, type ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const setRemoteNodeDisplayMock = vi.hoisted(() => vi.fn());
const clearRemoteNodeDisplayMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/remoteService', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/services/api/remoteService')>();
  return {
    ...actual,
    setRemoteNodeDisplay: setRemoteNodeDisplayMock,
    clearRemoteNodeDisplay: clearRemoteNodeDisplayMock,
  };
});

import { useClearNodeDisplay, useSetNodeDisplay } from './useRemote';

function makeWrapper(): {
  wrapper: (p: { children: ReactNode }) => React.JSX.Element;
  client: QueryClient;
} {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }): React.JSX.Element =>
    createElement(QueryClientProvider, { client }, children);
  return { wrapper, client };
}

beforeEach(() => {
  setRemoteNodeDisplayMock.mockReset();
  clearRemoteNodeDisplayMock.mockReset();
});

describe('useSetNodeDisplay', () => {
  it('setRemoteNodeDisplay 를 W×H 로 호출하고 성공 시 노드-상세+목록 쿼리를 무효화한다', async () => {
    setRemoteNodeDisplayMock.mockResolvedValue(undefined);
    const { wrapper, client } = makeWrapper();
    const spy = vi.spyOn(client, 'invalidateQueries');

    const { result } = renderHook(() => useSetNodeDisplay(), { wrapper });
    await result.current.mutateAsync({
      instanceID: 'node-a',
      width: 1600,
      height: 900,
    });

    expect(setRemoteNodeDisplayMock).toHaveBeenCalledWith('node-a', 1600, 900);
    await waitFor(() => {
      // 노드 prefix(상세 ['remote','nodes',id,'detail'] 포함) + 전체 목록 무효화.
      expect(spy).toHaveBeenCalledWith({ queryKey: ['remote', 'nodes', 'node-a'] });
      expect(spy).toHaveBeenCalledWith({ queryKey: ['remote', 'nodes'] });
    });
  });
});

describe('useClearNodeDisplay', () => {
  it('clearRemoteNodeDisplay 를 호출하고 성공 시 노드-상세+목록 쿼리를 무효화한다', async () => {
    clearRemoteNodeDisplayMock.mockResolvedValue(undefined);
    const { wrapper, client } = makeWrapper();
    const spy = vi.spyOn(client, 'invalidateQueries');

    const { result } = renderHook(() => useClearNodeDisplay(), { wrapper });
    await result.current.mutateAsync('node-a');

    expect(clearRemoteNodeDisplayMock).toHaveBeenCalledWith('node-a');
    await waitFor(() => {
      expect(spy).toHaveBeenCalledWith({ queryKey: ['remote', 'nodes', 'node-a'] });
      expect(spy).toHaveBeenCalledWith({ queryKey: ['remote', 'nodes'] });
    });
  });
});
