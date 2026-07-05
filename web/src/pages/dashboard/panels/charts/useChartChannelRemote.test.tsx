// useChartChannel 원격 target 배선 테스트 (SPEC-REMOTE-001 M10, 그룹 L, REQ-L07).
//
// 검증: TargetProvider(remote) 하에서 useChartChannel 이 RemoteChartChannelClient 를
// 원격 SSE URL 로 생성한다(별도 WS 경로 미신설). 로컬(TargetProvider 없음)에서는
// 기존 ChartChannelClient 를 사용한다(회귀 없음).

import { renderHook } from '@testing-library/react';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const remoteCtorMock = vi.hoisted(() => vi.fn());
const localCtorMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/ws/remoteChartChannel', () => ({
  RemoteChartChannelClient: class {
    constructor(...args: unknown[]) {
      remoteCtorMock(...args);
    }
    connect() {}
    disconnect() {}
    isConnected() {
      return false;
    }
  },
}));
vi.mock('@/services/ws/chartChannel', () => ({
  ChartChannelClient: class {
    constructor(...args: unknown[]) {
      localCtorMock(...args);
    }
    connect() {}
    disconnect() {}
    isConnected() {
      return false;
    }
  },
}));

import { TargetProvider } from '@/lib/remote/TargetContext';
import { LOCAL_TARGET } from '@/lib/remote/target';

import { useChartChannel } from './useChartChannel';

function remoteWrapper(instanceId: string) {
  return ({ children }: { children: ReactNode }) => (
    <TargetProvider target={{ type: 'remote', instanceId }}>{children}</TargetProvider>
  );
}

function localWrapper() {
  return ({ children }: { children: ReactNode }) => (
    <TargetProvider target={LOCAL_TARGET}>{children}</TargetProvider>
  );
}

beforeEach(() => {
  remoteCtorMock.mockReset();
  localCtorMock.mockReset();
});

describe('useChartChannel — 원격 target 배선', () => {
  it('원격 target 이면 RemoteChartChannelClient 를 SSE URL 로 생성한다', () => {
    renderHook(() => useChartChannel('temp'), { wrapper: remoteWrapper('node-1') });

    expect(remoteCtorMock).toHaveBeenCalledTimes(1);
    expect(localCtorMock).not.toHaveBeenCalled();
    // 2번째 인자 = 원격 차트 SSE base URL(채널명 포함).
    const sseUrl = remoteCtorMock.mock.calls[0]![1] as string;
    expect(sseUrl).toBe('/api/v1/remote/nodes/node-1/charts/temp/stream');
  });

  it('로컬 target 이면 ChartChannelClient(WS)를 사용한다(회귀 없음)', () => {
    renderHook(() => useChartChannel('temp'), { wrapper: localWrapper() });

    expect(localCtorMock).toHaveBeenCalledTimes(1);
    expect(remoteCtorMock).not.toHaveBeenCalled();
  });
});
