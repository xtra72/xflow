// ChartChannelClient 단위 테스트.
// WebSocket 을 인젝션 가능한 factory 로 대체해 테스트한다.

import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';

import {
  ChartChannelClient,
  type ChartChannelClientOptions,
  type ChartServerMessage,
  type ChartConnectionStatus,
} from './chartChannel';

// --- MockWebSocket ---

class MockWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;

  readyState = MockWebSocket.CONNECTING;
  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: string }) => void) | null = null;
  onclose: ((ev?: { code?: number; reason?: string }) => void) | null = null;
  onerror: (() => void) | null = null;

  readonly url: string;
  closeCalled = false;
  closeArgs: Array<[number?, string?]> = [];

  // 생성된 모든 인스턴스 추적 (reconnect 검증용)
  static instances: MockWebSocket[] = [];

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
  }

  open() {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.();
  }

  emit(msg: ChartServerMessage) {
    this.onmessage?.({ data: JSON.stringify(msg) });
  }

  emitRaw(data: string) {
    this.onmessage?.({ data });
  }

  fireError() {
    this.onerror?.();
  }

  fireClose(code?: number, reason?: string) {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.({ code, reason });
  }

  close(code?: number, reason?: string) {
    this.closeCalled = true;
    this.closeArgs.push([code, reason]);
    this.readyState = MockWebSocket.CLOSED;
    // 실제 브라우저처럼 close 이후 onclose 호출은 안 함 (disconnect 에서 수동 트리거 안함).
  }

  static reset() {
    MockWebSocket.instances = [];
  }
}

function buildOpts(
  overrides: Partial<ChartChannelClientOptions> = {},
): { opts: ChartChannelClientOptions; onMessage: ReturnType<typeof vi.fn>; onStatus: ReturnType<typeof vi.fn> } {
  const onMessage = vi.fn();
  const onStatus = vi.fn();
  const opts: ChartChannelClientOptions = {
    url: 'ws://localhost/ws/chart/test',
    onMessage,
    onStatus,
    createSocket: (u) => new MockWebSocket(u) as unknown as WebSocket,
    backoff: [10, 20, 40],
    ...overrides,
  };
  return { opts, onMessage, onStatus };
}

describe('ChartChannelClient', () => {
  beforeEach(() => {
    MockWebSocket.reset();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('connect() 시 WebSocket 을 생성하고 status=connecting 통지', () => {
    const { opts, onStatus } = buildOpts();
    const client = new ChartChannelClient(opts);

    client.connect();

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0]!.url).toBe('ws://localhost/ws/chart/test');
    expect(onStatus).toHaveBeenCalledWith('connecting', undefined);
  });

  it('socket.onopen 시 status=connected', () => {
    const { opts, onStatus } = buildOpts();
    const client = new ChartChannelClient(opts);
    client.connect();

    MockWebSocket.instances[0]!.open();

    expect(onStatus).toHaveBeenCalledWith('connected', undefined);
    expect(client.isConnected()).toBe(true);
  });

  it('chart.backfill 메시지를 파싱해 onMessage 로 전달', () => {
    const { opts, onMessage } = buildOpts();
    const client = new ChartChannelClient(opts);
    client.connect();
    MockWebSocket.instances[0]!.open();

    MockWebSocket.instances[0]!.emit({
      type: 'chart.backfill',
      channel: 'test',
      entries: [{ timestamp: 100, value: 1 }],
      backfilled_count: 1,
    });

    expect(onMessage).toHaveBeenCalledTimes(1);
    const call = onMessage.mock.calls[0]![0] as { type: string; backfilled_count: number };
    expect(call.type).toBe('chart.backfill');
    expect(call.backfilled_count).toBe(1);
  });

  it('chart.append 메시지 전달', () => {
    const { opts, onMessage } = buildOpts();
    const client = new ChartChannelClient(opts);
    client.connect();
    MockWebSocket.instances[0]!.open();

    MockWebSocket.instances[0]!.emit({
      type: 'chart.append',
      channel: 'test',
      entry: { timestamp: 200, value: 2 },
    });

    expect(onMessage).toHaveBeenCalledTimes(1);
  });

  it('잘못된 JSON 은 무시 (콜백 호출 안 됨)', () => {
    const { opts, onMessage } = buildOpts();
    const client = new ChartChannelClient(opts);
    client.connect();
    MockWebSocket.instances[0]!.open();

    MockWebSocket.instances[0]!.emitRaw('{not json');

    expect(onMessage).not.toHaveBeenCalled();
  });

  it('chart.closed 메시지 수신 시 onStatus(closed) 통지, 재연결 스케줄 안 함', () => {
    const { opts, onStatus } = buildOpts();
    const client = new ChartChannelClient(opts);
    client.connect();
    MockWebSocket.instances[0]!.open();

    MockWebSocket.instances[0]!.emit({
      type: 'chart.closed',
      channel: 'test',
      reason: 'flow_undeployed',
    });

    // closed 상태 통지
    expect(onStatus).toHaveBeenCalledWith('closed', 'flow_undeployed');

    // 소켓 자체 close 가 일어나도 재연결 시도 안 함
    MockWebSocket.instances[0]!.fireClose();
    vi.advanceTimersByTime(100);
    expect(MockWebSocket.instances).toHaveLength(1);
  });

  it('chart.error 수신 시 onStatus(error)', () => {
    const { opts, onStatus } = buildOpts();
    const client = new ChartChannelClient(opts);
    client.connect();
    MockWebSocket.instances[0]!.open();

    MockWebSocket.instances[0]!.emit({
      type: 'chart.error',
      channel: 'test',
      reason: 'channel_not_found',
    });

    expect(onStatus).toHaveBeenCalledWith('error', 'channel_not_found');
  });

  it('예기치 않은 close 시 exponential backoff 로 재연결 스케줄', () => {
    const { opts, onStatus } = buildOpts({ backoff: [100, 200, 400] });
    const client = new ChartChannelClient(opts);
    client.connect();
    MockWebSocket.instances[0]!.open();

    // 서버가 예기치 않게 연결을 끊음 -- open 된 적이 있었으므로 retryCount 는 이미 0
    MockWebSocket.instances[0]!.fireClose();

    expect(onStatus).toHaveBeenCalledWith('disconnected', undefined);
    // 첫 backoff = 100ms
    expect(MockWebSocket.instances).toHaveLength(1);
    vi.advanceTimersByTime(100);
    expect(MockWebSocket.instances).toHaveLength(2);

    // 두번째 재연결 대상은 아직 open 되기 전에 즉시 실패 -> 두번째 backoff = 200ms (retryCount=1)
    MockWebSocket.instances[1]!.fireClose();
    vi.advanceTimersByTime(150);
    expect(MockWebSocket.instances).toHaveLength(2);
    vi.advanceTimersByTime(60);
    expect(MockWebSocket.instances).toHaveLength(3);
  });

  it('disconnect() 가 대기 중인 재연결을 취소', () => {
    const { opts } = buildOpts({ backoff: [1000] });
    const client = new ChartChannelClient(opts);
    client.connect();
    MockWebSocket.instances[0]!.open();
    MockWebSocket.instances[0]!.fireClose();

    // 재연결 대기 중
    client.disconnect();
    vi.advanceTimersByTime(2000);

    expect(MockWebSocket.instances).toHaveLength(1);
  });

  it('disconnect() 가 OPEN 상태 소켓을 정상 close', () => {
    const { opts } = buildOpts();
    const client = new ChartChannelClient(opts);
    client.connect();
    MockWebSocket.instances[0]!.open();

    client.disconnect();

    expect(MockWebSocket.instances[0]!.closeCalled).toBe(true);
  });

  it('disconnect() 는 idempotent (두 번 호출해도 에러 없음)', () => {
    const { opts } = buildOpts();
    const client = new ChartChannelClient(opts);
    client.connect();
    MockWebSocket.instances[0]!.open();

    client.disconnect();
    expect(() => client.disconnect()).not.toThrow();
  });

  it('socket error 발생 시 onStatus(error) 호출', () => {
    const { opts, onStatus } = buildOpts();
    const client = new ChartChannelClient(opts);
    client.connect();
    MockWebSocket.instances[0]!.fireError();

    expect(onStatus).toHaveBeenCalledWith('error', undefined);
  });

  it('maxReconnectAttempts 초과 시 더이상 재연결 안 함', () => {
    const { opts } = buildOpts({ backoff: [10], maxReconnectAttempts: 2 });
    const client = new ChartChannelClient(opts);
    client.connect();

    // 3번 실패 -> 2번만 재연결 시도 가능
    for (let i = 0; i < 5; i++) {
      MockWebSocket.instances[MockWebSocket.instances.length - 1]!.fireClose();
      vi.advanceTimersByTime(20);
    }

    // 최초 1 + 재시도 2 = 3개
    expect(MockWebSocket.instances.length).toBeLessThanOrEqual(3);
  });

  it('backoff 배열이 소진되면 마지막 값 반복 사용', () => {
    const { opts } = buildOpts({ backoff: [10, 20] });
    const client = new ChartChannelClient(opts);
    client.connect();

    MockWebSocket.instances[0]!.fireClose();
    vi.advanceTimersByTime(10);
    expect(MockWebSocket.instances).toHaveLength(2);

    MockWebSocket.instances[1]!.fireClose();
    vi.advanceTimersByTime(20);
    expect(MockWebSocket.instances).toHaveLength(3);

    // 3번째 실패 - 마지막 값 20ms 반복
    MockWebSocket.instances[2]!.fireClose();
    vi.advanceTimersByTime(20);
    expect(MockWebSocket.instances).toHaveLength(4);
  });

  it('onStatus 전 이전 status 와 동일하면 중복 통지 안 해도 됨 (허용)', () => {
    // behavior spec: status 변경은 최소 한번은 호출되면 됨. 과도 호출 검증 불필요.
    const { opts, onStatus } = buildOpts();
    const client = new ChartChannelClient(opts);
    client.connect();
    MockWebSocket.instances[0]!.open();

    const statusValues = onStatus.mock.calls.map((c) => c[0] as ChartConnectionStatus);
    expect(statusValues).toContain('connecting');
    expect(statusValues).toContain('connected');
  });

  it('isConnected() 는 OPEN 상태일 때만 true', () => {
    const { opts } = buildOpts();
    const client = new ChartChannelClient(opts);
    expect(client.isConnected()).toBe(false);

    client.connect();
    expect(client.isConnected()).toBe(false);

    MockWebSocket.instances[0]!.open();
    expect(client.isConnected()).toBe(true);

    client.disconnect();
    expect(client.isConnected()).toBe(false);
  });
});
