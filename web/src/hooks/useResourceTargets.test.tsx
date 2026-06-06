// useFlowsTarget / useAgentsTarget / useDevicesTarget 단위 테스트
// (SPEC-REMOTE-001 M8, REQ-J09/J10).
//
// 검증:
//   - 로컬 타깃: 로컬 목록 훅 위임, isRemote=false, 반환 형태 동일.
//   - 원격 타깃: 미러(useNodeMirror) 조회 → 목록 아이템 매핑, isRemote=true.

import { renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { LOCAL_TARGET } from '@/lib/remote/target';
import type { MirroredResource } from '@/types/remote';

const useFlowsMock = vi.hoisted(() => vi.fn());
const useAgentsMock = vi.hoisted(() => vi.fn());
const useDevicesRealtimeMock = vi.hoisted(() => vi.fn());
const useNodeMirrorMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useFlow', () => ({ useFlows: useFlowsMock }));
vi.mock('@/hooks/useAgent', () => ({ useAgents: useAgentsMock }));
vi.mock('@/hooks/useDevice', () => ({ useDevicesRealtime: useDevicesRealtimeMock }));
vi.mock('@/hooks/useRemote', () => ({ useNodeMirror: useNodeMirrorMock }));

import {
  useAgentsTarget,
  useDevicesTarget,
  useFlowsTarget,
} from './useResourceTargets';

function makeMirror(o: Partial<MirroredResource> = {}): MirroredResource {
  return {
    id: 'r-1',
    source_instance_id: 'node-a',
    name: 'Res 1',
    kind: 'flow',
    updated_at: 0,
    online: true,
    ...o,
  };
}

const emptyQuery = { data: undefined, isLoading: false, error: null, refetch: vi.fn() };

beforeEach(() => {
  useFlowsMock.mockReset().mockReturnValue(emptyQuery);
  useAgentsMock.mockReset().mockReturnValue(emptyQuery);
  useDevicesRealtimeMock.mockReset().mockReturnValue(emptyQuery);
  useNodeMirrorMock.mockReset().mockReturnValue({ data: [], isLoading: false, error: null, refetch: vi.fn() });
});

describe('useFlowsTarget', () => {
  it('로컬 타깃은 useFlows 를 위임하고 isRemote=false 다', () => {
    useFlowsMock.mockReturnValue({
      data: { data: [{ id: 'f1', name: 'F1', status: 'running', node_count: 2 }], total: 1 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    const { result } = renderHook(() => useFlowsTarget(LOCAL_TARGET));
    expect(result.current.isRemote).toBe(false);
    expect(result.current.data?.data[0]!.id).toBe('f1');
    // 원격 미러는 비활성으로 호출된다.
    expect(useNodeMirrorMock).toHaveBeenCalledWith('', 'flow', false);
  });

  it('원격 타깃은 미러를 FlowInfo 로 매핑하고 isRemote=true 다', () => {
    useNodeMirrorMock.mockReturnValue({
      data: [
        makeMirror({
          id: 'f-r',
          name: 'Remote Flow',
          status: 'running',
          definition: JSON.stringify({ nodes: [{ id: 'n1' }, { id: 'n2' }] }),
        }),
      ],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    const { result } = renderHook(() =>
      useFlowsTarget({ type: 'remote', instanceId: 'node-a' }),
    );
    expect(result.current.isRemote).toBe(true);
    expect(useNodeMirrorMock).toHaveBeenCalledWith('node-a', 'flow', true);
    const flow = result.current.data!.data[0]!;
    expect(flow.id).toBe('f-r');
    expect(flow.name).toBe('Remote Flow');
    expect(flow.node_count).toBe(2);
    // 로컬 훅 데이터는 사용되지 않는다.
  });
});

describe('useAgentsTarget', () => {
  it('원격 타깃은 미러를 AgentInfo 로 매핑한다(type 추출)', () => {
    useNodeMirrorMock.mockReturnValue({
      data: [
        makeMirror({
          id: 'a-r',
          name: 'Remote Agent',
          kind: 'agent',
          definition: JSON.stringify({ type: 'mqtt-client' }),
        }),
      ],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    const { result } = renderHook(() =>
      useAgentsTarget({ type: 'remote', instanceId: 'node-a' }),
    );
    expect(useNodeMirrorMock).toHaveBeenCalledWith('node-a', 'agent', true);
    expect(result.current.data!.data[0]!.type).toBe('mqtt-client');
  });
});

describe('useDevicesTarget', () => {
  it('원격 타깃은 미러를 DeviceInfo 로 매핑하고 online 을 보존한다', () => {
    useNodeMirrorMock.mockReturnValue({
      data: [makeMirror({ id: 'd-r', name: 'Dev', kind: 'device', online: false })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    const { result } = renderHook(() =>
      useDevicesTarget({ type: 'remote', instanceId: 'node-a' }),
    );
    expect(useNodeMirrorMock).toHaveBeenCalledWith('node-a', 'device', true);
    const dev = result.current.data!.data[0]!;
    expect(dev.id).toBe('d-r');
    expect(dev.online).toBe(false);
  });

  it('로컬 타깃은 useDevicesRealtime 를 위임한다', () => {
    useDevicesRealtimeMock.mockReturnValue({
      data: { data: [{ id: 'd1', name: 'Local', online: true }], total: 1 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    const { result } = renderHook(() => useDevicesTarget(LOCAL_TARGET));
    expect(result.current.isRemote).toBe(false);
    expect(result.current.data!.data[0]!.id).toBe('d1');
  });
});
