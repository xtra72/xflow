// useChartChannel 훅 테스트.
// createClient 를 인젝션해 실제 WebSocket 없이 검증한다.

import { describe, it, expect, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import { useChartChannel } from './useChartChannel';
import type {
  ChartChannelClientOptions,
  ChartServerMessage,
} from '@/services/ws/chartChannel';

interface MockClientHandle {
  opts: ChartChannelClientOptions;
  connectCalls: number;
  disconnectCalls: number;
  emit: (msg: ChartServerMessage) => void;
  setStatus: (s: string, detail?: string) => void;
}

function buildClientFactory(): {
  factory: (opts: ChartChannelClientOptions) => {
    connect: () => void;
    disconnect: () => void;
    isConnected: () => boolean;
  };
  getHandle: () => MockClientHandle;
} {
  let handle: MockClientHandle | null = null;

  const factory = (opts: ChartChannelClientOptions) => {
    const h: MockClientHandle = {
      opts,
      connectCalls: 0,
      disconnectCalls: 0,
      emit: (msg) => opts.onMessage(msg),
      setStatus: (s, detail) =>
        opts.onStatus(s as Parameters<typeof opts.onStatus>[0], detail),
    };
    handle = h;
    return {
      connect: () => {
        h.connectCalls += 1;
      },
      disconnect: () => {
        h.disconnectCalls += 1;
      },
      isConnected: () => false,
    };
  };

  return {
    factory,
    getHandle: () => {
      if (!handle) throw new Error('no client created yet');
      return handle;
    },
  };
}

describe('useChartChannel', () => {
  it('channelName 미지정 시 idle 상태, 빈 entries', () => {
    const { result } = renderHook(() => useChartChannel(undefined));
    expect(result.current.status).toBe('idle');
    expect(result.current.entries).toEqual([]);
  });

  it('채널명 지정 시 client.connect() 호출', () => {
    const { factory, getHandle } = buildClientFactory();
    renderHook(() => useChartChannel('test1', { createClient: factory }));
    expect(getHandle().connectCalls).toBe(1);
  });

  it('chart.backfill 수신 시 entries 가 세팅됨', () => {
    const { factory, getHandle } = buildClientFactory();
    const { result } = renderHook(() =>
      useChartChannel('test1', { createClient: factory }),
    );

    act(() => {
      getHandle().emit({
        type: 'chart.backfill',
        channel: 'test1',
        entries: [
          { timestamp: 1, value: 10 },
          { timestamp: 2, value: 20 },
        ],
        backfilled_count: 2,
      });
    });

    expect(result.current.entries).toHaveLength(2);
    expect(result.current.entries[0]!.value).toBe(10);
  });

  it('chart.append 수신 시 entries 뒤에 붙음', () => {
    const { factory, getHandle } = buildClientFactory();
    const { result } = renderHook(() =>
      useChartChannel('test1', { createClient: factory }),
    );

    act(() => {
      getHandle().emit({
        type: 'chart.backfill',
        channel: 'test1',
        entries: [{ timestamp: 1, value: 10 }],
        backfilled_count: 1,
      });
    });
    act(() => {
      getHandle().emit({
        type: 'chart.append',
        channel: 'test1',
        entry: { timestamp: 2, value: 20 },
      });
    });

    expect(result.current.entries).toHaveLength(2);
    expect(result.current.entries[1]!.value).toBe(20);
  });

  it('maxPoints 초과 시 오래된 entry 를 잘라냄', () => {
    const { factory, getHandle } = buildClientFactory();
    const { result } = renderHook(() =>
      useChartChannel('test1', { maxPoints: 3, createClient: factory }),
    );

    act(() => {
      getHandle().emit({
        type: 'chart.backfill',
        channel: 'test1',
        entries: [
          { timestamp: 1, value: 1 },
          { timestamp: 2, value: 2 },
          { timestamp: 3, value: 3 },
          { timestamp: 4, value: 4 },
        ],
        backfilled_count: 4,
      });
    });

    // 최근 3개만 남아야 함 (tail)
    expect(result.current.entries).toHaveLength(3);
    expect(result.current.entries[0]!.value).toBe(2);
    expect(result.current.entries[2]!.value).toBe(4);

    // append 후에도 tail 유지
    act(() => {
      getHandle().emit({
        type: 'chart.append',
        channel: 'test1',
        entry: { timestamp: 5, value: 5 },
      });
    });

    expect(result.current.entries).toHaveLength(3);
    expect(result.current.entries[0]!.value).toBe(3);
    expect(result.current.entries[2]!.value).toBe(5);
  });

  it('chart.closed 수신 시 status="closed", closedReason 설정', () => {
    const { factory, getHandle } = buildClientFactory();
    const { result } = renderHook(() =>
      useChartChannel('test1', { createClient: factory }),
    );

    act(() => {
      getHandle().setStatus('closed', 'flow_undeployed');
      getHandle().emit({
        type: 'chart.closed',
        channel: 'test1',
        reason: 'flow_undeployed',
      });
    });

    expect(result.current.status).toBe('closed');
    expect(result.current.closedReason).toBe('flow_undeployed');
  });

  it('chart.error 수신 시 status="error", errorReason 설정', () => {
    const { factory, getHandle } = buildClientFactory();
    const { result } = renderHook(() =>
      useChartChannel('test1', { createClient: factory }),
    );

    act(() => {
      getHandle().setStatus('error', 'channel_not_found');
      getHandle().emit({
        type: 'chart.error',
        channel: 'test1',
        reason: 'channel_not_found',
      });
    });

    expect(result.current.status).toBe('error');
    expect(result.current.errorReason).toBe('channel_not_found');
  });

  it('status 전이 반영 (connecting -> connected)', () => {
    const { factory, getHandle } = buildClientFactory();
    const { result } = renderHook(() =>
      useChartChannel('test1', { createClient: factory }),
    );

    act(() => {
      getHandle().setStatus('connecting');
    });
    expect(result.current.status).toBe('connecting');

    act(() => {
      getHandle().setStatus('connected');
    });
    expect(result.current.status).toBe('connected');
  });

  it('unmount 시 client.disconnect() 호출', () => {
    const { factory, getHandle } = buildClientFactory();
    const { unmount } = renderHook(() =>
      useChartChannel('test1', { createClient: factory }),
    );

    unmount();
    expect(getHandle().disconnectCalls).toBe(1);
  });

  it('channelName 변경 시 기존 client 를 disconnect 하고 새로 connect', () => {
    const { factory } = buildClientFactory();
    const disconnectSpy = vi.fn();
    const connectSpy = vi.fn();

    const trackingFactory = (opts: ChartChannelClientOptions) => {
      // 실제 팩토리 이용하여 onMessage/onStatus 만 유지
      const inner = factory(opts);
      return {
        connect: () => {
          connectSpy();
          inner.connect();
        },
        disconnect: () => {
          disconnectSpy();
          inner.disconnect();
        },
        isConnected: () => false,
      };
    };

    const { rerender } = renderHook(
      ({ ch }: { ch: string | undefined }) =>
        useChartChannel(ch, { createClient: trackingFactory }),
      { initialProps: { ch: 'a' } },
    );

    expect(connectSpy).toHaveBeenCalledTimes(1);

    rerender({ ch: 'b' });

    expect(disconnectSpy).toHaveBeenCalledTimes(1);
    expect(connectSpy).toHaveBeenCalledTimes(2);
  });
});
