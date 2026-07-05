// useTargetGating 단위 테스트 (SPEC-REMOTE-001 M8, REQ-J05/J09).

import { renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { LOCAL_TARGET } from '@/lib/remote/target';
import type { ManagedNode } from '@/types/remote';

const useManagedNodesMock = vi.hoisted(() => vi.fn());
const useRemoteModeMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useRemote', () => ({
  useManagedNodes: useManagedNodesMock,
  useRemoteMode: useRemoteModeMock,
}));

import { useTargetGating } from './useTargetGating';

function node(o: Partial<ManagedNode> = {}): ManagedNode {
  return {
    instance_id: 'node-a',
    hostname: 'gw-1',
    version: '',
    status: 'approved',
    online: true,
    last_seen: 0,
    ...o,
  };
}

beforeEach(() => {
  useManagedNodesMock.mockReset().mockReturnValue({ data: [] });
  useRemoteModeMock.mockReset().mockReturnValue({ data: { mode: 'server' } });
});

describe('useTargetGating — 로컬', () => {
  it('로컬 타깃은 항상 제어 가능하며 노드 쿼리를 발행하지 않는다', () => {
    const { result } = renderHook(() => useTargetGating(LOCAL_TARGET));
    expect(result.current.isRemote).toBe(false);
    expect(result.current.nodeReady).toBe(true);
    expect(result.current.canControl()).toBe(true);
    expect(result.current.canControl(false)).toBe(true);
    // 로컬이면 비활성으로 노드 쿼리를 막는다.
    expect(useManagedNodesMock).toHaveBeenCalledWith(undefined, false);
  });
});

describe('useTargetGating — 원격', () => {
  it('승인+온라인 노드면 nodeReady=true, 자원 online 평가', () => {
    useManagedNodesMock.mockReturnValue({ data: [node()] });
    const { result } = renderHook(() =>
      useTargetGating({ type: 'remote', instanceId: 'node-a' }),
    );
    expect(result.current.isRemote).toBe(true);
    expect(result.current.nodeReady).toBe(true);
    expect(result.current.nodeLabel).toBe('gw-1');
    expect(result.current.canControl(true)).toBe(true);
    expect(result.current.canControl(false)).toBe(false);
  });

  it('오프라인 노드면 제어 불가', () => {
    useManagedNodesMock.mockReturnValue({ data: [node({ online: false })] });
    const { result } = renderHook(() =>
      useTargetGating({ type: 'remote', instanceId: 'node-a' }),
    );
    expect(result.current.nodeReady).toBe(false);
    expect(result.current.canControl(true)).toBe(false);
  });

  it('미승인 노드면 제어 불가', () => {
    useManagedNodesMock.mockReturnValue({ data: [node({ status: 'pending' })] });
    const { result } = renderHook(() =>
      useTargetGating({ type: 'remote', instanceId: 'node-a' }),
    );
    expect(result.current.nodeReady).toBe(false);
  });

  it('존재하지 않는 노드면 제어 불가', () => {
    useManagedNodesMock.mockReturnValue({ data: [] });
    const { result } = renderHook(() =>
      useTargetGating({ type: 'remote', instanceId: 'ghost' }),
    );
    expect(result.current.nodeReady).toBe(false);
    expect(result.current.nodeLabel).toBe('ghost');
  });
});
