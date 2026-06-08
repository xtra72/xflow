// AcControlPanel 원격 명령 라우팅 테스트 (SPEC-REMOTE-001 M10, 그룹 L, REQ-L08).
//
// 검증:
//   - 원격 target 하에서 전원 토글이 그룹 D 명령(sendCommand {domain:'device',
//     action:'execute', args:{id, command:'set_power', ...}})으로 라우팅된다.
//   - 디바이스 상태 read 는 useDeviceDetailTarget(device.state)로 취득한다.

import { render, screen, fireEvent } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const sendCommandMutateMock = vi.hoisted(() => vi.fn());
const executeMutateMock = vi.hoisted(() => vi.fn());
const useDeviceDetailTargetMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useRemote', () => ({
  useSendCommand: () => ({ mutate: sendCommandMutateMock, isPending: false, error: null }),
}));
vi.mock('@/hooks/useDevice', () => ({
  useExecuteCommand: () => ({ mutate: executeMutateMock, isPending: false, error: null }),
  useDeviceRealtime: () => ({ data: undefined, isLoading: false }),
}));
vi.mock('@/hooks/useDetailTargets', () => ({
  useDeviceDetailTarget: useDeviceDetailTargetMock,
}));
vi.mock('@/hooks/useTargetGating', () => ({
  useTargetGating: () => ({
    isRemote: true,
    nodeReady: true,
    nodeLabel: 'host',
    canControl: () => true,
  }),
}));
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { TargetProvider } from '@/lib/remote/TargetContext';

import AcControlPanel from './AcControlPanel';

function deviceWithControl() {
  return {
    id: 'dev-1',
    uid: 'dev-1',
    name: 'AC',
    type: 'indoor',
    protocol: 'lgap',
    agent_name: 'a',
    source: 'config',
    online: true,
    last_seen: '',
    capabilities: ['set_power', 'set_mode'],
    state: {
      online: true,
      ready: true,
      last_seen: '',
      error_count: 0,
      properties: { power: true, current_temperature: 24, target_temperature: 24, mode: 'cool' },
    },
  };
}

beforeEach(() => {
  sendCommandMutateMock.mockReset();
  executeMutateMock.mockReset();
  useDeviceDetailTargetMock.mockReset().mockReturnValue({
    data: deviceWithControl(),
    isLoading: false,
    error: null,
  });
});

describe('AcControlPanel — 원격 명령 라우팅', () => {
  it('전원 토글이 그룹 D 명령(execute)으로 라우팅된다(로컬 execute 미호출)', () => {
    render(
      <TargetProvider target={{ type: 'remote', instanceId: 'node-1' }}>
        <AcControlPanel panelId="p" title="AC" config={{ deviceId: 'dev-1' }} />
      </TargetProvider>,
    );

    // 전원 끄기 버튼(현재 ON) 클릭.
    fireEvent.click(screen.getByLabelText('전원 끄기'));

    expect(sendCommandMutateMock).toHaveBeenCalledWith({
      instanceID: 'node-1',
      req: {
        domain: 'device',
        action: 'execute',
        args: { id: 'dev-1', command: 'set_power', params: { power: false } },
      },
    });
    expect(executeMutateMock).not.toHaveBeenCalled();
  });

  it('원격 상태 read 에 useDeviceDetailTarget 을 사용한다(device.state)', () => {
    render(
      <TargetProvider target={{ type: 'remote', instanceId: 'node-1' }}>
        <AcControlPanel panelId="p" title="AC" config={{ deviceId: 'dev-1' }} />
      </TargetProvider>,
    );
    expect(useDeviceDetailTargetMock).toHaveBeenCalled();
  });
});
