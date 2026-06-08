// 원격 자원 편집 뮤테이션 훅 테스트 (SPEC-REMOTE-001 M7, REQ-I01~I04).
//
// remoteService 를 mock 하여 각 뮤테이션 훅이 올바른 서비스 함수를 호출하고,
// 성공 시 관련 ['remote', ...] 쿼리를 무효화하는지 검증한다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import { createElement, type ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const createRemoteFlowMock = vi.hoisted(() => vi.fn());
const updateRemoteFlowMock = vi.hoisted(() => vi.fn());
const deleteRemoteFlowMock = vi.hoisted(() => vi.fn());
const createRemoteAgentMock = vi.hoisted(() => vi.fn());
const updateRemoteAgentMock = vi.hoisted(() => vi.fn());
const deleteRemoteAgentMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/remoteService', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/services/api/remoteService')>();
  return {
    ...actual,
    createRemoteFlow: createRemoteFlowMock,
    updateRemoteFlow: updateRemoteFlowMock,
    deleteRemoteFlow: deleteRemoteFlowMock,
    createRemoteAgent: createRemoteAgentMock,
    updateRemoteAgent: updateRemoteAgentMock,
    deleteRemoteAgent: deleteRemoteAgentMock,
  };
});

import {
  useCreateRemoteAgent,
  useCreateRemoteFlow,
  useDeleteRemoteAgent,
  useDeleteRemoteFlow,
  useUpdateRemoteAgent,
  useUpdateRemoteFlow,
} from './useRemote';

function makeWrapper(): { wrapper: (p: { children: ReactNode }) => React.JSX.Element; client: QueryClient } {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }): React.JSX.Element =>
    createElement(QueryClientProvider, { client }, children);
  return { wrapper, client };
}

beforeEach(() => {
  createRemoteFlowMock.mockReset();
  updateRemoteFlowMock.mockReset();
  deleteRemoteFlowMock.mockReset();
  createRemoteAgentMock.mockReset();
  updateRemoteAgentMock.mockReset();
  deleteRemoteAgentMock.mockReset();
});

describe('useCreateRemoteFlow', () => {
  it('createRemoteFlow 를 호출하고 성공 시 미러 쿼리를 무효화한다', async () => {
    createRemoteFlowMock.mockResolvedValue({ id: 'f-new', name: 'x', status: '' });
    const { wrapper, client } = makeWrapper();
    const spy = vi.spyOn(client, 'invalidateQueries');

    const { result } = renderHook(() => useCreateRemoteFlow(), { wrapper });
    await result.current.mutateAsync({
      instanceID: 'node-a',
      req: { name: 'x', definition: {} },
    });

    expect(createRemoteFlowMock).toHaveBeenCalledWith('node-a', {
      name: 'x',
      definition: {},
    });
    await waitFor(() => {
      expect(spy).toHaveBeenCalledWith({ queryKey: ['remote', 'nodes', 'node-a'] });
      expect(spy).toHaveBeenCalledWith({ queryKey: ['remote', 'flows'] });
    });
  });
});

describe('useUpdateRemoteFlow', () => {
  it('updateRemoteFlow 를 호출한다', async () => {
    updateRemoteFlowMock.mockResolvedValue({ id: 'f-1', name: 'x', status: '' });
    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useUpdateRemoteFlow(), { wrapper });
    await result.current.mutateAsync({
      instanceID: 'node-a',
      flowID: 'f-1',
      req: { definition: { nodes: [] } },
    });
    expect(updateRemoteFlowMock).toHaveBeenCalledWith('node-a', 'f-1', {
      definition: { nodes: [] },
    });
  });
});

describe('useDeleteRemoteFlow', () => {
  it('deleteRemoteFlow 를 호출한다', async () => {
    deleteRemoteFlowMock.mockResolvedValue(undefined);
    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useDeleteRemoteFlow(), { wrapper });
    await result.current.mutateAsync({ instanceID: 'node-a', flowID: 'f-1' });
    expect(deleteRemoteFlowMock).toHaveBeenCalledWith('node-a', 'f-1');
  });
});

describe('useCreateRemoteAgent / useUpdateRemoteAgent / useDeleteRemoteAgent', () => {
  it('createRemoteAgent 를 호출하고 agents 쿼리를 무효화한다', async () => {
    createRemoteAgentMock.mockResolvedValue({ id: 'a-new', name: 'a', status: '' });
    const { wrapper, client } = makeWrapper();
    const spy = vi.spyOn(client, 'invalidateQueries');
    const { result } = renderHook(() => useCreateRemoteAgent(), { wrapper });
    await result.current.mutateAsync({
      instanceID: 'node-a',
      req: { name: 'a', type: 'mqtt', config: {} },
    });
    expect(createRemoteAgentMock).toHaveBeenCalledWith('node-a', {
      name: 'a',
      type: 'mqtt',
      config: {},
    });
    await waitFor(() => {
      expect(spy).toHaveBeenCalledWith({ queryKey: ['remote', 'agents'] });
    });
  });

  it('updateRemoteAgent 를 호출한다', async () => {
    updateRemoteAgentMock.mockResolvedValue({ id: 'a-1', name: 'a', status: '' });
    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useUpdateRemoteAgent(), { wrapper });
    await result.current.mutateAsync({
      instanceID: 'node-a',
      agentID: 'a-1',
      req: { config: { host: 'h' } },
    });
    expect(updateRemoteAgentMock).toHaveBeenCalledWith('node-a', 'a-1', {
      config: { host: 'h' },
    });
  });

  it('deleteRemoteAgent 를 호출한다', async () => {
    deleteRemoteAgentMock.mockResolvedValue(undefined);
    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useDeleteRemoteAgent(), { wrapper });
    await result.current.mutateAsync({ instanceID: 'node-a', agentID: 'a-1' });
    expect(deleteRemoteAgentMock).toHaveBeenCalledWith('node-a', 'a-1');
  });
});
