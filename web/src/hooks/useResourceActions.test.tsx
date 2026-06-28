// useFlowActionsTarget / useAgentActionsTarget 라우팅 테스트
// (SPEC-REMOTE-001 M8, 그룹 J).
//
// 검증 항목:
//   - 로컬 타깃: 액션이 로컬 뮤테이션 훅으로 라우팅된다(start/stop/restart/...).
//   - 원격 타깃: 라이프사이클이 useSendCommand({domain, action, args:{id}})로,
//     삭제가 M7 편집 훅(useDeleteRemoteFlow/useDeleteRemoteAgent)으로 라우팅된다.
//   - 원격 미지원 액션(flow restart/undeploy, agent enable/disable)은 supports=false
//     이며 perform 시 reject 한다.

import { renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { LOCAL_TARGET, type ResourceTarget } from '@/lib/remote/target';

// ---- 로컬/원격 뮤테이션 훅 모킹 ----
// 각 훅은 mutateAsync 스파이를 가진 안정 객체를 반환한다.

function makeMutation() {
  return { mutateAsync: vi.fn().mockResolvedValue(undefined), isPending: false };
}

const startFlow = makeMutation();
const stopFlow = makeMutation();
const restartFlow = makeMutation();
const deployFlow = makeMutation();
const undeployFlow = makeMutation();
const deleteFlow = makeMutation();

const startAgent = makeMutation();
const stopAgent = makeMutation();
const restartAgent = makeMutation();
const enableAgent = makeMutation();
const disableAgent = makeMutation();
const deleteAgent = makeMutation();

const sendCommand = makeMutation();
const deleteRemoteFlow = makeMutation();
const deleteRemoteAgent = makeMutation();

vi.mock('@/hooks/useFlow', () => ({
  useStartFlow: () => startFlow,
  useStopFlow: () => stopFlow,
  useRestartFlow: () => restartFlow,
  useDeployFlow: () => deployFlow,
  useUndeployFlow: () => undeployFlow,
  useDeleteFlow: () => deleteFlow,
}));

vi.mock('@/hooks/useAgent', () => ({
  useStartAgent: () => startAgent,
  useStopAgent: () => stopAgent,
  useRestartAgent: () => restartAgent,
  useEnableAgent: () => enableAgent,
  useDisableAgent: () => disableAgent,
  useDeleteAgent: () => deleteAgent,
}));

vi.mock('@/hooks/useRemote', () => ({
  useSendCommand: () => sendCommand,
  useDeleteRemoteFlow: () => deleteRemoteFlow,
  useDeleteRemoteAgent: () => deleteRemoteAgent,
}));

import { useAgentActionsTarget, useFlowActionsTarget } from './useResourceActions';

const REMOTE: ResourceTarget = { type: 'remote', instanceId: 'node-1' };

beforeEach(() => {
  for (const m of [
    startFlow, stopFlow, restartFlow, deployFlow, undeployFlow, deleteFlow,
    startAgent, stopAgent, restartAgent, enableAgent, disableAgent, deleteAgent,
    sendCommand, deleteRemoteFlow, deleteRemoteAgent,
  ]) {
    m.mutateAsync.mockClear();
  }
});

describe('useFlowActionsTarget — 로컬', () => {
  it('start/stop/restart/deploy/delete 가 로컬 훅으로 라우팅된다', async () => {
    const { result } = renderHook(() => useFlowActionsTarget(LOCAL_TARGET));
    await result.current.perform('start', 'f1');
    await result.current.perform('stop', 'f1');
    await result.current.perform('restart', 'f1');
    await result.current.perform('deploy', 'f1');
    await result.current.perform('delete', 'f1');

    expect(startFlow.mutateAsync).toHaveBeenCalledWith('f1');
    expect(stopFlow.mutateAsync).toHaveBeenCalledWith('f1');
    expect(restartFlow.mutateAsync).toHaveBeenCalledWith('f1');
    expect(deployFlow.mutateAsync).toHaveBeenCalledWith('f1');
    expect(deleteFlow.mutateAsync).toHaveBeenCalledWith('f1');
    expect(sendCommand.mutateAsync).not.toHaveBeenCalled();
    expect(result.current.supports('restart')).toBe(true);
    expect(result.current.supports('undeploy')).toBe(true);
  });
});

describe('useFlowActionsTarget — 원격', () => {
  it('start 가 sendCommand{domain:flow, action:start, args:{id}} 로 라우팅된다', async () => {
    const { result } = renderHook(() => useFlowActionsTarget(REMOTE));
    await result.current.perform('start', 'f9');
    expect(sendCommand.mutateAsync).toHaveBeenCalledWith({
      instanceID: 'node-1',
      req: { domain: 'flow', action: 'start', args: { id: 'f9' } },
    });
    expect(startFlow.mutateAsync).not.toHaveBeenCalled();
  });

  it('deploy 가 sendCommand action:deploy 로 라우팅된다', async () => {
    const { result } = renderHook(() => useFlowActionsTarget(REMOTE));
    await result.current.perform('deploy', 'f9');
    expect(sendCommand.mutateAsync).toHaveBeenCalledWith({
      instanceID: 'node-1',
      req: { domain: 'flow', action: 'deploy', args: { id: 'f9' } },
    });
  });

  it('delete 가 M7 useDeleteRemoteFlow 로 라우팅된다(sendCommand 미사용)', async () => {
    const { result } = renderHook(() => useFlowActionsTarget(REMOTE));
    await result.current.perform('delete', 'f9');
    expect(deleteRemoteFlow.mutateAsync).toHaveBeenCalledWith({
      instanceID: 'node-1',
      flowID: 'f9',
    });
    expect(sendCommand.mutateAsync).not.toHaveBeenCalled();
  });

  it('restart/undeploy 는 원격 미지원(supports=false, perform 시 reject)', async () => {
    const { result } = renderHook(() => useFlowActionsTarget(REMOTE));
    expect(result.current.supports('restart')).toBe(false);
    expect(result.current.supports('undeploy')).toBe(false);
    await expect(result.current.perform('restart', 'f9')).rejects.toThrow();
    expect(sendCommand.mutateAsync).not.toHaveBeenCalled();
  });
});

describe('useAgentActionsTarget — 원격', () => {
  it('start/stop/restart 가 sendCommand{domain:agent} 로 라우팅된다', async () => {
    const { result } = renderHook(() => useAgentActionsTarget(REMOTE));
    await result.current.perform('start', 'a1');
    await result.current.perform('stop', 'a1');
    await result.current.perform('restart', 'a1');
    expect(sendCommand.mutateAsync).toHaveBeenNthCalledWith(1, {
      instanceID: 'node-1',
      req: { domain: 'agent', action: 'start', args: { id: 'a1' } },
    });
    expect(sendCommand.mutateAsync).toHaveBeenNthCalledWith(3, {
      instanceID: 'node-1',
      req: { domain: 'agent', action: 'restart', args: { id: 'a1' } },
    });
  });

  it('delete 가 M7 useDeleteRemoteAgent 로 라우팅된다', async () => {
    const { result } = renderHook(() => useAgentActionsTarget(REMOTE));
    await result.current.perform('delete', 'a1');
    expect(deleteRemoteAgent.mutateAsync).toHaveBeenCalledWith({
      instanceID: 'node-1',
      agentID: 'a1',
    });
  });

  it('enable/disable 는 원격 미지원(supports=false, perform 시 reject)', async () => {
    const { result } = renderHook(() => useAgentActionsTarget(REMOTE));
    expect(result.current.supports('enable')).toBe(false);
    expect(result.current.supports('disable')).toBe(false);
    await expect(result.current.perform('enable', 'a1')).rejects.toThrow();
    expect(sendCommand.mutateAsync).not.toHaveBeenCalled();
  });
});

describe('useAgentActionsTarget — 로컬', () => {
  it('enable/disable 가 로컬 훅으로 라우팅된다', async () => {
    const { result } = renderHook(() => useAgentActionsTarget(LOCAL_TARGET));
    await result.current.perform('enable', 'a1');
    await result.current.perform('disable', 'a1');
    expect(enableAgent.mutateAsync).toHaveBeenCalledWith('a1');
    expect(disableAgent.mutateAsync).toHaveBeenCalledWith('a1');
  });
});
