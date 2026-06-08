// useFlowsTarget / useAgentsTarget / useDevicesTarget 단위 테스트
// (SPEC-REMOTE-001 M8, REQ-J04/J09/J10).
//
// 검증:
//   - 로컬 타깃: 로컬 목록 훅 위임, isRemote=false, 반환 형태 동일.
//   - 원격 타깃(노드 ready): 라이브 목록(useNodeLiveList) 조회 → FULL 타입 그대로
//     반환(connected/uptime/stats 등 런타임 필드 포함), isRemote=true.
//   - 원격 타깃 라이브 실패: 미러(useNodeMirror) 매핑으로 폴백.
//   - 원격 타깃 노드 미-ready: 라이브 비활성 → 미러 폴백.

import { renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { LOCAL_TARGET } from '@/lib/remote/target';
import type { MirroredResource } from '@/types/remote';

const useFlowsMock = vi.hoisted(() => vi.fn());
const useAgentsMock = vi.hoisted(() => vi.fn());
const useDevicesRealtimeMock = vi.hoisted(() => vi.fn());
const useNodeMirrorMock = vi.hoisted(() => vi.fn());
const useNodeLiveListMock = vi.hoisted(() => vi.fn());
const useTargetGatingMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useFlow', () => ({ useFlows: useFlowsMock }));
vi.mock('@/hooks/useAgent', () => ({ useAgents: useAgentsMock }));
vi.mock('@/hooks/useDevice', () => ({ useDevicesRealtime: useDevicesRealtimeMock }));
vi.mock('@/hooks/useRemote', () => ({
  useNodeMirror: useNodeMirrorMock,
  useNodeLiveList: useNodeLiveListMock,
}));
vi.mock('@/hooks/useTargetGating', () => ({ useTargetGating: useTargetGatingMock }));

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
/** 비활성/빈 라이브 쿼리(노드 미-ready 또는 미발행 상태). */
const emptyLive = { data: undefined, isLoading: false, error: null, refetch: vi.fn() };

beforeEach(() => {
  useFlowsMock.mockReset().mockReturnValue(emptyQuery);
  useAgentsMock.mockReset().mockReturnValue(emptyQuery);
  useDevicesRealtimeMock.mockReset().mockReturnValue(emptyQuery);
  useNodeMirrorMock.mockReset().mockReturnValue({ data: [], isLoading: false, error: null, refetch: vi.fn() });
  useNodeLiveListMock.mockReset().mockReturnValue(emptyLive);
  // 기본: 노드 ready(승인+온라인). 개별 테스트에서 override.
  useTargetGatingMock.mockReset().mockReturnValue({
    isRemote: true,
    nodeReady: true,
    nodeLabel: 'gw-1',
    canControl: () => true,
  });
});

describe('useFlowsTarget', () => {
  it('로컬 타깃은 useFlows 를 위임하고 isRemote=false 다', () => {
    useTargetGatingMock.mockReturnValue({
      isRemote: false,
      nodeReady: true,
      nodeLabel: undefined,
      canControl: () => true,
    });
    useFlowsMock.mockReturnValue({
      data: { data: [{ id: 'f1', name: 'F1', status: 'running', node_count: 2 }], total: 1 },
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    const { result } = renderHook(() => useFlowsTarget(LOCAL_TARGET));
    expect(result.current.isRemote).toBe(false);
    expect(result.current.data?.data[0]!.id).toBe('f1');
    // 로컬이면 라이브/미러 모두 비활성으로 호출된다.
    expect(useNodeLiveListMock).toHaveBeenCalledWith('', 'flow', false);
  });

  it('원격 타깃(ready)은 라이브 목록을 FULL FlowInfo 로 반환한다', () => {
    useNodeLiveListMock.mockReturnValue({
      data: [
        { id: 'f-r', name: 'Remote Flow', status: 'running', node_count: 3, uptime: '5m' },
      ],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    const { result } = renderHook(() =>
      useFlowsTarget({ type: 'remote', instanceId: 'node-a' }),
    );
    expect(result.current.isRemote).toBe(true);
    expect(useNodeLiveListMock).toHaveBeenCalledWith('node-a', 'flow', true);
    const flow = result.current.data!.data[0]!;
    expect(flow.id).toBe('f-r');
    expect(flow.node_count).toBe(3);
    expect(flow.uptime).toBe('5m');
  });

  it('라이브가 에러면 미러 매핑으로 폴백한다', () => {
    useNodeLiveListMock.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('502'),
      refetch: vi.fn(),
    });
    useNodeMirrorMock.mockReturnValue({
      data: [
        makeMirror({
          id: 'f-m',
          name: 'Mirror Flow',
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
    // 라이브 에러 → 미러 발행 + 매핑 결과.
    expect(useNodeMirrorMock).toHaveBeenCalledWith('node-a', 'flow', true);
    const flow = result.current.data!.data[0]!;
    expect(flow.id).toBe('f-m');
    expect(flow.node_count).toBe(2);
  });
});

describe('useAgentsTarget', () => {
  it('원격 타깃(ready)은 라이브 목록의 런타임 필드를 그대로 노출한다', () => {
    useNodeLiveListMock.mockReturnValue({
      data: [
        {
          id: 'a-r',
          name: 'Remote Agent',
          type: 'mqtt-client',
          status: 'running',
          connected: true,
          uptime: '12m',
          stats: { messages_in: 100, messages_out: 50 },
        },
      ],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    const { result } = renderHook(() =>
      useAgentsTarget({ type: 'remote', instanceId: 'node-a' }),
    );
    expect(useNodeLiveListMock).toHaveBeenCalledWith('node-a', 'agent', true);
    const agent = result.current.data!.data[0]!;
    expect(agent.type).toBe('mqtt-client');
    expect(agent.connected).toBe(true);
    expect(agent.uptime).toBe('12m');
    expect(agent.stats?.messages_in).toBe(100);
  });

  it('노드 미-ready 면 라이브 비활성, 미러 폴백', () => {
    useTargetGatingMock.mockReturnValue({
      isRemote: true,
      nodeReady: false,
      nodeLabel: 'gw-1',
      canControl: () => false,
    });
    useNodeMirrorMock.mockReturnValue({
      data: [
        makeMirror({
          id: 'a-m',
          name: 'Mirror Agent',
          kind: 'agent',
          definition: JSON.stringify({ type: 'socket' }),
        }),
      ],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    const { result } = renderHook(() =>
      useAgentsTarget({ type: 'remote', instanceId: 'node-a' }),
    );
    // 라이브는 비활성(enabled=false), 미러는 폴백으로 활성.
    expect(useNodeLiveListMock).toHaveBeenCalledWith('node-a', 'agent', false);
    expect(useNodeMirrorMock).toHaveBeenCalledWith('node-a', 'agent', true);
    expect(result.current.data!.data[0]!.type).toBe('socket');
  });
});

describe('useDevicesTarget', () => {
  it('원격 타깃(ready)은 라이브 디바이스 목록을 그대로 반환한다', () => {
    useNodeLiveListMock.mockReturnValue({
      data: [
        {
          id: 'd-r',
          name: 'Dev',
          type: 'sensor',
          protocol: 'modbus',
          agent_name: 'a1',
          source: 'config',
          online: true,
          last_seen: '2026-01-01T00:00:00Z',
          capabilities: [],
        },
      ],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    const { result } = renderHook(() =>
      useDevicesTarget({ type: 'remote', instanceId: 'node-a' }),
    );
    expect(useNodeLiveListMock).toHaveBeenCalledWith('node-a', 'device', true);
    const dev = result.current.data!.data[0]!;
    expect(dev.id).toBe('d-r');
    expect(dev.online).toBe(true);
  });

  it('로컬 타깃은 useDevicesRealtime 를 위임한다', () => {
    useTargetGatingMock.mockReturnValue({
      isRemote: false,
      nodeReady: true,
      nodeLabel: undefined,
      canControl: () => true,
    });
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
