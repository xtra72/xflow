// RemoteChartChannelClient 테스트 (SPEC-REMOTE-001 M10, 그룹 L, REQ-L07).
//
// 검증: SSE EventSource 로 chart.backfill/chart.append 프레임을 onMessage 로
// 전달하고, chart.closed 는 종단(재연결 금지)으로 처리하며, disconnect 시
// EventSource 를 close 한다(teardown — REQ-J08b).

import { beforeEach, describe, expect, it, vi } from 'vitest';

import { RemoteChartChannelClient, type EventSourceLike } from './remoteChartChannel';
import type {
  ChartChannelClientOptions,
  ChartServerMessage,
  ChartConnectionStatus,
} from './chartChannel';

vi.mock('@/stores', () => ({
  getAuthState: () => ({ tokens: { access_token: 'tok-1' } }),
}));

interface FakeSource extends EventSourceLike {
  url: string;
  listeners: Record<string, ((ev: MessageEvent) => void) | undefined>;
  closed: boolean;
  emit: (type: string, data?: string) => void;
}

function buildFactory(): {
  factory: (url: string) => EventSourceLike;
  get: () => FakeSource;
} {
  let src: FakeSource | null = null;
  const factory = (url: string): EventSourceLike => {
    const s: FakeSource = {
      url,
      listeners: {},
      closed: false,
      onerror: null,
      onopen: null,
      addEventListener(type, listener) {
        this.listeners[type] = listener;
      },
      close() {
        this.closed = true;
      },
      emit(type, data) {
        this.listeners[type]?.({ data } as MessageEvent);
      },
    };
    src = s;
    return s;
  };
  return {
    factory,
    get: () => {
      if (!src) throw new Error('no source');
      return src;
    },
  };
}

function buildOpts(): {
  opts: ChartChannelClientOptions;
  messages: ChartServerMessage[];
  statuses: ChartConnectionStatus[];
} {
  const messages: ChartServerMessage[] = [];
  const statuses: ChartConnectionStatus[] = [];
  return {
    messages,
    statuses,
    opts: {
      url: 'ws://ignored/ws/chart/x',
      onMessage: (m) => messages.push(m),
      onStatus: (s) => statuses.push(s),
    },
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('RemoteChartChannelClient', () => {
  it('connect 시 SSE URL 에 token 을 부착한다', () => {
    const { factory, get } = buildFactory();
    const { opts } = buildOpts();
    const client = new RemoteChartChannelClient(
      opts,
      '/api/v1/remote/nodes/node-1/charts/temp/stream',
      factory,
    );
    client.connect();
    expect(get().url).toBe(
      '/api/v1/remote/nodes/node-1/charts/temp/stream?token=tok-1',
    );
  });

  it('chart.backfill / chart.append 프레임을 onMessage 로 전달한다', () => {
    const { factory, get } = buildFactory();
    const { opts, messages } = buildOpts();
    const client = new RemoteChartChannelClient(opts, '/base', factory);
    client.connect();

    get().emit(
      'message',
      JSON.stringify({ type: 'chart.backfill', channel: 'temp', entries: [], backfilled_count: 0 }),
    );
    get().emit(
      'message',
      JSON.stringify({ type: 'chart.append', channel: 'temp', entry: { timestamp: 1, value: 2 } }),
    );

    expect(messages.map((m) => m.type)).toEqual(['chart.backfill', 'chart.append']);
  });

  it('chart.closed 수신 시 closed 상태 전이 + EventSource close(재연결 금지)', () => {
    const { factory, get } = buildFactory();
    const { opts, statuses } = buildOpts();
    const client = new RemoteChartChannelClient(opts, '/base', factory);
    client.connect();

    get().emit('message', JSON.stringify({ type: 'chart.closed', channel: 'x', reason: 'done' }));

    expect(statuses).toContain('closed');
    expect(get().closed).toBe(true);
  });

  it('disconnect 는 EventSource 를 close 한다(teardown)', () => {
    const { factory, get } = buildFactory();
    const { opts } = buildOpts();
    const client = new RemoteChartChannelClient(opts, '/base', factory);
    client.connect();
    client.disconnect();
    expect(get().closed).toBe(true);
  });
});
