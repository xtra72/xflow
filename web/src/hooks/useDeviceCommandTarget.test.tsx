// useDeviceCommandTarget 테스트 (SPEC-REMOTE-001 M10, 그룹 L, REQ-L08).
//
// 검증:
//   - 로컬 target: useExecuteCommand(POST /devices/{id}/execute)로 라우팅한다.
//   - 원격 target: 그룹 D 명령(sendCommand {domain:'device', action:'execute',
//     args:{id, command, params}})으로 라우팅한다(프록시 비경유 — REQ-J03).

import { renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const executeMutateMock = vi.hoisted(() => vi.fn());
const sendCommandMutateMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useDevice', () => ({
  useExecuteCommand: () => ({
    mutate: executeMutateMock,
    isPending: false,
    error: null,
  }),
}));
vi.mock('@/hooks/useRemote', () => ({
  useSendCommand: () => ({
    mutate: sendCommandMutateMock,
    isPending: false,
    error: null,
  }),
}));

import { LOCAL_TARGET } from '@/lib/remote/target';

import { useDeviceCommandTarget } from './useDeviceCommandTarget';

beforeEach(() => {
  executeMutateMock.mockReset();
  sendCommandMutateMock.mockReset();
});

describe('useDeviceCommandTarget', () => {
  it('로컬 target 이면 useExecuteCommand 로 라우팅한다', () => {
    const { result } = renderHook(() => useDeviceCommandTarget(LOCAL_TARGET));
    result.current.execute('dev-1', 'set_power', { power: true });
    expect(executeMutateMock).toHaveBeenCalledWith({
      id: 'dev-1',
      req: { command: 'set_power', params: { power: true } },
    });
    expect(sendCommandMutateMock).not.toHaveBeenCalled();
    expect(result.current.isRemote).toBe(false);
  });

  it('원격 target 이면 그룹 D 명령(execute)으로 라우팅한다', () => {
    const { result } = renderHook(() =>
      useDeviceCommandTarget({ type: 'remote', instanceId: 'node-1' }),
    );
    result.current.execute('dev-1', 'set_mode', { mode: 'cool' });
    expect(sendCommandMutateMock).toHaveBeenCalledWith({
      instanceID: 'node-1',
      req: {
        domain: 'device',
        action: 'execute',
        args: { id: 'dev-1', command: 'set_mode', params: { mode: 'cool' } },
      },
    });
    expect(executeMutateMock).not.toHaveBeenCalled();
    expect(result.current.isRemote).toBe(true);
  });

  it('deviceId 가 비면 명령을 보내지 않는다', () => {
    const { result } = renderHook(() =>
      useDeviceCommandTarget({ type: 'remote', instanceId: 'node-1' }),
    );
    result.current.execute(undefined, 'set_power', { power: true });
    expect(sendCommandMutateMock).not.toHaveBeenCalled();
  });
});
