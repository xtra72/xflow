// useChartChannels 다중 채널 구독 훅 테스트.

import { describe, it, expect } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import { useChartChannels, type ChannelRef } from './useChartChannels';
import type {
  ChartChannelClientOptions,
  ChartServerMessage,
} from '@/services/ws/chartChannel';

interface ClientLog {
  url: string;
  connectCalls: number;
  disconnectCalls: number;
  emit: (msg: ChartServerMessage) => void;
  setStatus: (s: string, detail?: string) => void;
}

function makeFactoryRegistry() {
  const logs = new Map<string, ClientLog>();
  const factory = (opts: ChartChannelClientOptions) => {
    const log: ClientLog = {
      url: opts.url,
      connectCalls: 0,
      disconnectCalls: 0,
      emit: (msg) => opts.onMessage(msg),
      setStatus: (s, detail) =>
        opts.onStatus(s as Parameters<typeof opts.onStatus>[0], detail),
    };
    logs.set(opts.url, log);
    return {
      connect: () => {
        log.connectCalls += 1;
      },
      disconnect: () => {
        log.disconnectCalls += 1;
      },
      isConnected: () => false,
    };
  };
  return { factory, logs };
}

const wsBase = 'ws://test';

describe('useChartChannels', () => {
  it('빈 refs: 빈 결과 + 클라이언트 미생성', () => {
    const { factory, logs } = makeFactoryRegistry();
    const { result } = renderHook(() =>
      useChartChannels([], { wsBaseUrl: wsBase, createClient: factory }),
    );
    expect(result.current.channels.size).toBe(0);
    expect(logs.size).toBe(0);
  });

  it('refs 개수만큼 채널 구독', () => {
    const { factory, logs } = makeFactoryRegistry();
    const refs: ChannelRef[] = [{ name: 'a' }, { name: 'b' }, { name: 'c' }];
    renderHook(() =>
      useChartChannels(refs, { wsBaseUrl: wsBase, createClient: factory }),
    );
    expect(logs.size).toBe(3);
    expect(logs.get('ws://test/ws/chart/a')!.connectCalls).toBe(1);
    expect(logs.get('ws://test/ws/chart/b')!.connectCalls).toBe(1);
    expect(logs.get('ws://test/ws/chart/c')!.connectCalls).toBe(1);
  });

  it('각 채널 별 메시지가 독립 entries 로 누적', () => {
    const { factory, logs } = makeFactoryRegistry();
    const refs: ChannelRef[] = [{ name: 'a' }, { name: 'b' }];
    const { result } = renderHook(() =>
      useChartChannels(refs, { wsBaseUrl: wsBase, createClient: factory }),
    );

    act(() => {
      logs.get('ws://test/ws/chart/a')!.emit({
        type: 'chart.backfill',
        channel: 'a',
        entries: [{ timestamp: 1, value: 100 }],
        backfilled_count: 1,
      });
      logs.get('ws://test/ws/chart/b')!.emit({
        type: 'chart.backfill',
        channel: 'b',
        entries: [
          { timestamp: 1, value: 200 },
          { timestamp: 2, value: 201 },
        ],
        backfilled_count: 2,
      });
    });

    expect(result.current.channels.get('a')!.entries).toHaveLength(1);
    expect(result.current.channels.get('a')!.entries[0]!.value).toBe(100);
    expect(result.current.channels.get('b')!.entries).toHaveLength(2);
  });

  it('refs 추가 시 기존 채널은 그대로 두고 신규만 connect', () => {
    const { factory, logs } = makeFactoryRegistry();
    const { rerender } = renderHook(
      ({ refs }: { refs: ChannelRef[] }) =>
        useChartChannels(refs, { wsBaseUrl: wsBase, createClient: factory }),
      { initialProps: { refs: [{ name: 'a' }] } },
    );
    expect(logs.size).toBe(1);

    rerender({ refs: [{ name: 'a' }, { name: 'b' }] });
    expect(logs.size).toBe(2);
    expect(logs.get('ws://test/ws/chart/a')!.disconnectCalls).toBe(0);
    expect(logs.get('ws://test/ws/chart/b')!.connectCalls).toBe(1);
  });

  it('refs 제거 시 해당 채널만 disconnect', () => {
    const { factory, logs } = makeFactoryRegistry();
    const { rerender, result } = renderHook(
      ({ refs }: { refs: ChannelRef[] }) =>
        useChartChannels(refs, { wsBaseUrl: wsBase, createClient: factory }),
      { initialProps: { refs: [{ name: 'a' }, { name: 'b' }] as ChannelRef[] } },
    );
    rerender({ refs: [{ name: 'a' }] });
    expect(logs.get('ws://test/ws/chart/a')!.disconnectCalls).toBe(0);
    expect(logs.get('ws://test/ws/chart/b')!.disconnectCalls).toBe(1);
    expect(result.current.channels.has('b')).toBe(false);
  });

  it('unmount 시 모든 채널 disconnect', () => {
    const { factory, logs } = makeFactoryRegistry();
    const { unmount } = renderHook(() =>
      useChartChannels([{ name: 'a' }, { name: 'b' }] as ChannelRef[], {
        wsBaseUrl: wsBase,
        createClient: factory,
      }),
    );
    unmount();
    expect(logs.get('ws://test/ws/chart/a')!.disconnectCalls).toBe(1);
    expect(logs.get('ws://test/ws/chart/b')!.disconnectCalls).toBe(1);
  });

  it('maxPoints 가 채널마다 적용', () => {
    const { factory, logs } = makeFactoryRegistry();
    const { result } = renderHook(() =>
      useChartChannels([{ name: 'a' }] as ChannelRef[], {
        wsBaseUrl: wsBase,
        createClient: factory,
        maxPoints: 2,
      }),
    );
    act(() => {
      logs.get('ws://test/ws/chart/a')!.emit({
        type: 'chart.backfill',
        channel: 'a',
        entries: [
          { timestamp: 1, value: 1 },
          { timestamp: 2, value: 2 },
          { timestamp: 3, value: 3 },
        ],
        backfilled_count: 3,
      });
    });
    expect(result.current.channels.get('a')!.entries).toHaveLength(2);
    expect(result.current.channels.get('a')!.entries[0]!.value).toBe(2);
  });

  it('빈 이름 ref 는 무시', () => {
    const { factory, logs } = makeFactoryRegistry();
    renderHook(() =>
      useChartChannels(
        [{ name: '' }, { name: 'a' }, { name: '   ' }] as ChannelRef[],
        { wsBaseUrl: wsBase, createClient: factory },
      ),
    );
    expect(logs.size).toBe(1);
    expect(logs.has('ws://test/ws/chart/a')).toBe(true);
  });

  it('상태 변경 반영', () => {
    const { factory, logs } = makeFactoryRegistry();
    const { result } = renderHook(() =>
      useChartChannels([{ name: 'a' }] as ChannelRef[], {
        wsBaseUrl: wsBase,
        createClient: factory,
      }),
    );
    act(() => {
      logs.get('ws://test/ws/chart/a')!.setStatus('connected');
    });
    expect(result.current.channels.get('a')!.status).toBe('connected');

    act(() => {
      logs.get('ws://test/ws/chart/a')!.setStatus('closed', 'flow_undeployed');
    });
    expect(result.current.channels.get('a')!.status).toBe('closed');
    expect(result.current.channels.get('a')!.closedReason).toBe('flow_undeployed');
  });
});
